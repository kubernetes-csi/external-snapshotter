/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	clientset "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	groupsnapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v6/listers/volumegroupsnapshot/v1alpha1"
	snapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v6/listers/volumesnapshot/v1"
	"github.com/spf13/cobra"

	informers "github.com/kubernetes-csi/external-snapshotter/client/v6/informers/externalversions"
	v1 "k8s.io/api/admission/v1"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var (
	certFile                         string
	keyFile                          string
	kubeconfigFile                   string
	port                             int
	preventVolumeModeConversion      bool
	enableVolumeGroupSnapshotWebhook bool
)

// CmdWebhook is used by Cobra.
var CmdWebhook = &cobra.Command{
	Use:   "validation-webhook",
	Short: "Starts a HTTPS server, uses ValidatingAdmissionWebhook to perform ratcheting validation on VolumeSnapshot and VolumeSnapshotContent",
	Long: `Starts a HTTPS server, uses ValidatingAdmissionWebhook to perform ratcheting validation on VolumeSnapshot and VolumeSnapshotContent.
After deploying it to Kubernetes cluster, the Administrator needs to create a ValidatingWebhookConfiguration
in the Kubernetes cluster to register remote webhook admission controllers. Phase one of https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/177-volume-snapshot/tighten-validation-webhook-crd.md`,
	Args: cobra.MaximumNArgs(0),
	Run:  main,
}

func init() {
	CmdWebhook.Flags().StringVar(&certFile, "tls-cert-file", "",
		"File containing the x509 Certificate for HTTPS. (CA cert, if any, concatenated after server cert). Required.")
	CmdWebhook.Flags().StringVar(&keyFile, "tls-private-key-file", "",
		"File containing the x509 private key matching --tls-cert-file. Required.")
	CmdWebhook.Flags().IntVar(&port, "port", 443,
		"Secure port that the webhook listens on")
	CmdWebhook.MarkFlagRequired("tls-cert-file")
	CmdWebhook.MarkFlagRequired("tls-private-key-file")
	// Add optional flag for kubeconfig
	CmdWebhook.Flags().StringVar(&kubeconfigFile, "kubeconfig", "", "kubeconfig file to use for volumesnapshotclasses")
	CmdWebhook.Flags().BoolVar(&preventVolumeModeConversion, "prevent-volume-mode-conversion",
		false, "Prevents an unauthorised user from modifying the volume mode when creating a PVC from an existing VolumeSnapshot.")
	CmdWebhook.Flags().BoolVar(&enableVolumeGroupSnapshotWebhook, "enable-volume-group-snapshot-webhook",
		false, "Enables webhook for VolumeGroupSnapshot, VolumeGroupSnapshotContent and VolumeGroupSnapshotClass.")
}

// admitv1beta1Func handles a v1beta1 admission
type admitv1beta1Func func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

// admitv1beta1Func handles a v1 admission
type admitv1Func func(v1.AdmissionReview) *v1.AdmissionResponse

// admitHandler is a handler, for both validators and mutators, that supports multiple admission review versions
type admitHandler struct {
	SnapshotAdmitter
}

func newDelegateToV1AdmitHandler(sa SnapshotAdmitter) admitHandler {
	return admitHandler{
		SnapshotAdmitter: sa,
	}
}

func delegateV1beta1AdmitToV1(f admitv1Func) admitv1beta1Func {
	return func(review v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
		in := v1.AdmissionReview{Request: convertAdmissionRequestToV1(review.Request)}
		out := f(in)
		return convertAdmissionResponseToV1beta1(out)
	}
}

// serve handles the http portion of a request prior to handing to an admit
// function
func serve(w http.ResponseWriter, r *http.Request, admit admitHandler) {
	var body []byte
	if r.Body == nil {
		msg := "Expected request body to be non-empty"
		klog.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		msg := fmt.Sprintf("Request could not be decoded: %v", err)
		klog.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
	}
	body = data

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		msg := fmt.Sprintf("contentType=%s, expect application/json", contentType)
		klog.Errorf(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	klog.V(2).Info(fmt.Sprintf("handling request: %s", body))

	deserializer := codecs.UniversalDeserializer()
	obj, gvk, err := deserializer.Decode(body, nil, nil)
	if err != nil {
		msg := fmt.Sprintf("Request could not be decoded: %v", err)
		klog.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	var responseObj runtime.Object
	switch *gvk {
	case v1beta1.SchemeGroupVersion.WithKind("AdmissionReview"):
		requestedAdmissionReview, ok := obj.(*v1beta1.AdmissionReview)
		if !ok {
			msg := fmt.Sprintf("Expected v1beta1.AdmissionReview but got: %T", obj)
			klog.Errorf(msg)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		responseAdmissionReview := &v1beta1.AdmissionReview{}
		responseAdmissionReview.SetGroupVersionKind(*gvk)
		responseAdmissionReview.Response = delegateV1beta1AdmitToV1(admit.Admit)(*requestedAdmissionReview)
		responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID
		responseObj = responseAdmissionReview
	case v1.SchemeGroupVersion.WithKind("AdmissionReview"):
		requestedAdmissionReview, ok := obj.(*v1.AdmissionReview)
		if !ok {
			msg := fmt.Sprintf("Expected v1.AdmissionReview but got: %T", obj)
			klog.Errorf(msg)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		responseAdmissionReview := &v1.AdmissionReview{}
		responseAdmissionReview.SetGroupVersionKind(*gvk)
		responseAdmissionReview.Response = admit.Admit(*requestedAdmissionReview)
		responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID
		responseObj = responseAdmissionReview
	default:
		msg := fmt.Sprintf("Unsupported group version kind: %v", gvk)
		klog.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	klog.V(2).Info(fmt.Sprintf("sending response: %v", responseObj))
	respBytes, err := json.Marshal(responseObj)
	if err != nil {
		klog.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		klog.Error(err)
	}
}

type serveSnapshotWebhook struct {
	lister snapshotlisters.VolumeSnapshotClassLister
}

func (s serveSnapshotWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serve(w, r, newDelegateToV1AdmitHandler(NewSnapshotAdmitter(s.lister)))
}

type serveGroupSnapshotWebhook struct {
	lister groupsnapshotlisters.VolumeGroupSnapshotClassLister
}

func (s serveGroupSnapshotWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serve(w, r, newDelegateToV1AdmitHandler(NewGroupSnapshotAdmitter(s.lister)))
}

func startServer(
	ctx context.Context,
	tlsConfig *tls.Config,
	cw *CertWatcher,
	vscLister snapshotlisters.VolumeSnapshotClassLister,
	vgscLister groupsnapshotlisters.VolumeGroupSnapshotClassLister,
) error {
	go func() {
		klog.Info("Starting certificate watcher")
		if err := cw.Start(ctx); err != nil {
			klog.Errorf("certificate watcher error: %v", err)
		}
	}()
	// Pipe through the informer at some point here.
	snapshotWebhook := serveSnapshotWebhook{
		lister: vscLister,
	}

	fmt.Println("Starting webhook server")
	mux := http.NewServeMux()
	mux.Handle("/volumesnapshot", snapshotWebhook)

	if enableVolumeGroupSnapshotWebhook {
		groupSnapshotWebhook := serveGroupSnapshotWebhook{
			lister: vgscLister,
		}
		mux.Handle("/volumegroupsnapshot", groupSnapshotWebhook)
	}

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) { w.Write([]byte("ok")) })
	srv := &http.Server{
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	// listener is always closed by srv.Serve
	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), tlsConfig)
	if err != nil {
		return err
	}

	return srv.Serve(listener)
}

func main(cmd *cobra.Command, args []string) {
	// Create new cert watcher
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel() // stops certwatcher
	cw, err := NewCertWatcher(certFile, keyFile)
	if err != nil {
		klog.Fatalf("failed to initialize new cert watcher: %v", err)
	}
	tlsConfig := &tls.Config{
		GetCertificate: cw.GetCertificate,
	}

	// Create an indexer.
	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(kubeconfigFile)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}
	snapClient, err := clientset.NewForConfig(config)
	if err != nil {
		klog.Errorf("Error building snapshot clientset: %s", err.Error())
		os.Exit(1)
	}

	factory := informers.NewSharedInformerFactory(snapClient, 0)
	snapshotLister := factory.Snapshot().V1().VolumeSnapshotClasses().Lister()
	var groupSnapshotLister groupsnapshotlisters.VolumeGroupSnapshotClassLister
	if enableVolumeGroupSnapshotWebhook {
		groupSnapshotLister = factory.Groupsnapshot().V1alpha1().VolumeGroupSnapshotClasses().Lister()
	}

	// Start the informers
	factory.Start(ctx.Done())
	// wait for the caches to sync
	factory.WaitForCacheSync(ctx.Done())

	if err := startServer(ctx, tlsConfig, cw, snapshotLister, groupSnapshotLister); err != nil {
		klog.Fatalf("server stopped: %v", err)
	}
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
