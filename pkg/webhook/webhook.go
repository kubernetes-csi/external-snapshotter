/*
Copyright 2025 The Kubernetes Authors.

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
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

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

var CmdWebhook = &cobra.Command{
	Use:   "conversion-webhook",
	Short: "Starts a HTTPS server to perform conversion between v1beta1 and v1beta2 VolumeGroupSnapshot API",
	Args:  cobra.MaximumNArgs(0),
	Run:   main,
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
		true, "Prevents an unauthorised user from modifying the volume mode when creating a PVC from an existing VolumeSnapshot.")
	CmdWebhook.Flags().BoolVar(&enableVolumeGroupSnapshotWebhook, "enable-volume-group-snapshot-webhook",
		false, "Enables webhook for VolumeGroupSnapshotClass.")
}

func startServer(
	ctx context.Context,
	tlsConfig *tls.Config,
	cw *CertWatcher,
) error {
	go func() {
		klog.Info("Starting certificate watcher")
		if err := cw.Start(ctx); err != nil {
			klog.Errorf("certificate watcher error: %v", err)
		}
	}()

	fmt.Println("Starting conversion webhook server")

	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/convert", func(w http.ResponseWriter, req *http.Request) { serve(w, req, convertGroupSnapshotCRD) })

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

	if err := startServer(ctx, tlsConfig, cw); err != nil {
		klog.Fatalf("server stopped: %v", err)
	}
}
