// Copyright 2018 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhook

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mattbaird/jsonpatch"
	//"github.com/pkg/errors"
	//"github.com/sirupsen/logrus"
	"github.com/golang/glog"

	admv1beta1 "k8s.io/api/admission/v1beta1"
	admregv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	kubeclientset "k8s.io/client-go/kubernetes"

	snapshotv1alpha1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	clientset "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
)

const (
	webhookConfigName   = "snapshot-webhook-config"
	snapshotWebhookName = "snapshot-init.snapshot.storage.k8s.io"
	// The admission webhook must be exposed in the following service,
	// this is mainly for the server certificate.
	// serviceName is the name of the admission webhook service.
	//serviceName = "snapshot-admission-webhook-svc"

	IsDefaultSnapshotClassAnnotation = "snapshot.storage.kubernetes.io/is-default-class"
)

var supportedSourceKinds = sets.NewString(string("PersistentVolumeClaim"))
var supportedDataSourceAPIGroups = sets.NewString(string(""))

// CreateConfiguration creates MutatingWebhookConfiguration and registeres the
// webhook admission controller with the kube-apiserver
func CreateConfiguration(clientset kubeclientset.Interface, serviceName, serviceNamespace string) error {
	failurePolicy := admregv1beta1.Fail
	config := &admregv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookConfigName,
		},
		Webhooks: []admregv1beta1.Webhook{
			// Webhook for initializing Snapshots.
			{
				Name: snapshotWebhookName,
				ClientConfig: admregv1beta1.WebhookClientConfig{
					Service: &admregv1beta1.ServiceReference{
						Name:      serviceName,
						Namespace: serviceNamespace,
					},
					CABundle: caCert,
				},
				Rules: []admregv1beta1.RuleWithOperations{
					{
						Operations: []admregv1beta1.OperationType{
							admregv1beta1.Create,
						},
						Rule: admregv1beta1.Rule{
							APIGroups:   []string{snapshotv1alpha1.GroupName},
							APIVersions: []string{"v1alpha1"},
							Resources:   []string{"volumesnapshots"},
						},
					},
				},
				FailurePolicy: &failurePolicy,
			},
		},
	}
	glog.V(4).Infof("Creating MutatingWebhookConfigurations %q", config.Name)
	if _, err := clientset.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(config); err != nil {
		if apierrors.IsAlreadyExists(err) {
			glog.V(2).Infof("MutatingWebhookConfigurations %q already exists; use the existing one", config.Name)
			return nil
		}
		return fmt.Errorf("failed to create MutatingWebhookConfigurations %q: %v", config.Name, err)
	}
	return nil
}

func GetTLSConfig() *tls.Config {
	sCert, err := tls.X509KeyPair(serverCert, serverKey)
	if err != nil {
		glog.Fatal(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
	}
}

// AdmitSnapshot performs admission checks and mutations on VolumeSnapshots.
func AdmitSnapshot(writer http.ResponseWriter, req *http.Request, clientset kubeclientset.Interface, snapClient clientset.Interface) {
	glog.Infof("admit snapshot ")
	review := &admv1beta1.AdmissionReview{}
	if err := json.NewDecoder(req.Body).Decode(review); err != nil {
		glog.Infof("Failed to decode Admit request: req = %+v, err = %s", *req, err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	response := admitSnapshot(review, clientset, snapClient)
	glog.V(5).Infof("Processed admission review: %+v", review)
	sendResponse(writer, response)
}

func admitSnapshot(review *admv1beta1.AdmissionReview, clientset kubeclientset.Interface, snapClient clientset.Interface) *admv1beta1.AdmissionReview {
	review.Response = &admv1beta1.AdmissionResponse{}

	// Verify that the request is indeed a Snapshot object.
	resource := metav1.GroupVersionResource{
		Group:    snapshotv1alpha1.GroupName,
		Version:  "v1alpha1",
		Resource: "volumesnapshots",
	}
	if review.Request.Resource != resource {
		review.Response.Result = &metav1.Status{
			Reason:  metav1.StatusReasonInvalid,
			Message: fmt.Sprintf("unexpected resource %+v in VolumeSnapshot admission", review.Request.Resource),
		}
		return review
	}

	// Decode the request
	snapshot := &snapshotv1alpha1.VolumeSnapshot{}
	if err := json.Unmarshal(review.Request.Object.Raw, snapshot); err != nil {
		review.Response.Result = &metav1.Status{
			Reason:  metav1.StatusReasonInvalid,
			Message: fmt.Sprintf("failed to decode VolumeSnapshot object %s/%s", review.Request.Namespace, review.Request.Name),
		}
		return review
	}

	// Validate the Snapshot object and set default snapshot class if not specified
	newSnapshot, changed, err := validateSnapshot(snapshot, clientset, snapClient)
	if err != nil {
		review.Response.Result = &metav1.Status{
			Reason:  metav1.StatusReasonInvalid,
			Message: err.Error(),
		}
		return review
	}

	if changed {
		glog.V(5).Infof("Patched Snapshot %s/%s: +%v", snapshot.Namespace, snapshot.Name, newSnapshot)
		patch, err := createPatch(review.Request.Object.Raw, newSnapshot)
		if err != nil {
			review.Response.Result = &metav1.Status{
				Reason:  metav1.StatusReasonInternalError,
				Message: fmt.Sprintf("failed to create patch for snapshot %s/%s", snapshot.Namespace, snapshot.Name),
			}
			return review
		}
		patchType := admv1beta1.PatchTypeJSONPatch
		review.Response.Patch = patch
		review.Response.PatchType = &patchType
	}

	review.Response.Allowed = true
	return review
}

func createPatch(old []byte, newObj interface{}) ([]byte, error) {
	new, err := json.Marshal(newObj)
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.CreatePatch(old, new)
	if err != nil {
		return nil, err
	}
	return json.Marshal(patch)
}

// IsDefaultAnnotation returns a boolean if
// the annotation is set
func IsDefaultAnnotation(obj metav1.ObjectMeta) bool {
	if obj.Annotations[IsDefaultSnapshotClassAnnotation] == "true" {
		return true
	}

	return false
}

func getDefaultSnapshotClassName(driver string, snapClient clientset.Interface) (string, error) {
	list, err := snapClient.VolumesnapshotV1alpha1().VolumeSnapshotClasses().List(metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	defaultClasses := []snapshotv1alpha1.VolumeSnapshotClass{}

	for _, class := range list.Items {
		if IsDefaultAnnotation(class.ObjectMeta) && class.Snapshotter == driver {
			defaultClasses = append(defaultClasses, class)
			glog.V(5).Infof("get defaultClass added: %s", class.Name)
		}
	}
	if len(defaultClasses) == 0 {
		return "", fmt.Errorf("cannot find default snapshot class")
	}
	if len(defaultClasses) > 1 {
		glog.V(4).Infof("get DefaultClass %d defaults found", len(defaultClasses))
		return "", fmt.Errorf("%d default snapshot classes were found", len(defaultClasses))
	}
	return defaultClasses[0].Name, nil
}

func getSnapshotClassNameFromContent(content *snapshotv1alpha1.VolumeSnapshotContent, snapClient clientset.Interface) (string, error) {
	if content.Spec.VolumeSnapshotClassName != nil {
		return *content.Spec.VolumeSnapshotClassName, nil
	}
	if content.Spec.VolumeSnapshotSource.CSI == nil {
		return "", fmt.Errorf("VolumeSNapshotContent does not has CSI volume source")
	}
	return getDefaultSnapshotClassName(content.Spec.VolumeSnapshotSource.CSI.Driver, snapClient)
}

// validateSnapshot checks whether the values set on the VolumeSnapshot object are valid.
// also set the snapshot class if it is not specified
func validateSnapshot(snapshot *snapshotv1alpha1.VolumeSnapshot, clientset kubeclientset.Interface, snapClient clientset.Interface) (*snapshotv1alpha1.VolumeSnapshot, bool, error) {
	if snapshot.Spec.Source != nil {
		source := snapshot.Spec.Source
		if len(source.Name) == 0 {
			return nil, false, fmt.Errorf("Snapshot.Spec.Source.Name can not be empty")
		} else if !supportedSourceKinds.Has(string(source.Kind)) {
			return nil, false, fmt.Errorf("Snapshot.Spec.Source.Kind exepct %v, got %q", supportedSourceKinds, source.Kind)
		}
		pvc, err := validateVolume(snapshot, clientset)
		if err != nil {
			return nil, false, err
		}
		exist, driver, err := validateSnapshotClass(snapshot, pvc, clientset, snapClient)
		if err != nil {
			return nil, false, err
		}
		if exist {
			return snapshot, false, nil
		}
		defaultClassName, err := getDefaultSnapshotClassName(driver, snapClient)
		if err != nil {
			return nil, false, err
		}
		snapshotClone := snapshot.DeepCopy()
		snapshotClone.Spec.VolumeSnapshotClassName = &defaultClassName
		return snapshotClone, true, nil
	}

	// if source is not specified, the content name must be set
	if snapshot.Spec.SnapshotContentName == "" {
		return nil, false, fmt.Errorf("cannot set Snapshot.Spec.Source to nil and snapshot.Spec.SnapshotContentName to empty at the same time.")
	}
	if snapshot.Spec.VolumeSnapshotClassName == nil {
		content, err := snapClient.VolumesnapshotV1alpha1().VolumeSnapshotContents().Get(snapshot.Spec.SnapshotContentName, metav1.GetOptions{})
		if err != nil {
			return nil, false, err
		}
		className, err := getSnapshotClassNameFromContent(content, snapClient)
		if err != nil {
			return nil, false, fmt.Errorf("cannot get default storage class %v", err)
		}
		snapshotClone := snapshot.DeepCopy()
		snapshotClone.Spec.VolumeSnapshotClassName = &className
		return snapshotClone, true, nil
	}
	return nil, false, nil
}

func validateVolume(snapshot *snapshotv1alpha1.VolumeSnapshot, clientset kubeclientset.Interface) (*v1.PersistentVolumeClaim, error) {
	pvcName := snapshot.Spec.Source.Name
	if pvcName == "" {
		return nil, fmt.Errorf("the PVC name is not specified in snapshot %s/%s", snapshot.Namespace, snapshot.Name)
	}
	pvc, err := clientset.CoreV1().PersistentVolumeClaims(snapshot.Namespace).Get(pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PVC %s from the API server: %q", pvcName, err)
	}
	if pvc.Status.Phase != v1.ClaimBound {
		return nil, fmt.Errorf("the PVC %s is not yet bound to a PV, will not attempt to take a snapshot", pvc.Name)
	}
	pvName := pvc.Spec.VolumeName
	_, err = clientset.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PV %s from the API server: %q", pvName, err)
	}

	glog.V(5).Infof("getVolumeFromVolumeSnapshot: snapshot [%s] PV name [%s]", snapshot.Name, pvName)

	return pvc, nil
}

func validateSnapshotClass(snapshot *snapshotv1alpha1.VolumeSnapshot, pvc *v1.PersistentVolumeClaim, clientset kubeclientset.Interface, snapClient clientset.Interface) (bool, string, error) {
	storageClassName := pvc.Spec.StorageClassName
	if storageClassName == nil || len(*storageClassName) == 0 {
		return false, "", fmt.Errorf("fail to get storage class from the pvc source")
	}

	storageClass, err := clientset.StorageV1().StorageClasses().Get(*storageClassName, metav1.GetOptions{})
	if err != nil {
		return false, "", fmt.Errorf("failed to retrieve StorageClass %q from the API server: %v", storageClassName, err)
	}

	snapshotClassName := snapshot.Spec.VolumeSnapshotClassName
	if snapshotClassName == nil || len(*snapshotClassName) == 0 {
		return false, storageClass.Provisioner, nil
	}
	snapshotClass, err := snapClient.VolumesnapshotV1alpha1().VolumeSnapshotClasses().Get(*snapshotClassName, metav1.GetOptions{})
	if err != nil {
		return false, "", fmt.Errorf("failed to retrieve VolumeSnapshotClass %q from the API server: %v", snapshotClassName, err)
	}

	if storageClass.Provisioner != snapshotClass.Snapshotter {
		return true, storageClass.Provisioner, fmt.Errorf("the snapshotter driver specified in snapshot class does not match the driver from the volume's provisioner")
	}
	return true, storageClass.Provisioner, nil
}

// sendResponse sends the response using the writer.
func sendResponse(writer http.ResponseWriter, response interface{}) {
	b, err := json.Marshal(response)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.WriteHeader(http.StatusOK)
	writer.Write(b)
}
