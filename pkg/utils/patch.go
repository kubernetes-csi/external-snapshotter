package utils

import (
	"context"
	"encoding/json"
	"fmt"

	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
)

// PatchOp represents a json patch operation
type PatchOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// PatchVolumeSnapshotContent patches a volume snapshot content object
func PatchVolumeSnapshotContent(
	existingSnapshotContent *crdv1.VolumeSnapshotContent,
	patch []PatchOp,
	client clientset.Interface,
	subresources ...string,
) (*crdv1.VolumeSnapshotContent, error) {
	data, err := json.Marshal(patch)
	if nil != err {
		return existingSnapshotContent, err
	}

	newSnapshotContent, err := client.SnapshotV1().VolumeSnapshotContents().Patch(context.TODO(), existingSnapshotContent.Name, types.JSONPatchType, data, metav1.PatchOptions{}, subresources...)
	if err != nil {
		return existingSnapshotContent, err
	}

	return newSnapshotContent, nil
}

// PatchVolumeSnapshot patches a volume snapshot object
func PatchVolumeSnapshot(
	existingSnapshot *crdv1.VolumeSnapshot,
	patch []PatchOp,
	client clientset.Interface,
	subresources ...string,
) (*crdv1.VolumeSnapshot, error) {
	data, err := json.Marshal(patch)
	if nil != err {
		return existingSnapshot, err
	}

	newSnapshot, err := client.SnapshotV1().VolumeSnapshots(existingSnapshot.Namespace).Patch(context.TODO(), existingSnapshot.Name, types.JSONPatchType, data, metav1.PatchOptions{}, subresources...)
	if err != nil {
		return existingSnapshot, err
	}

	return newSnapshot, nil
}

// PatchVolumeGroupSnapshotContent patches a volume group snapshot content object
func PatchVolumeGroupSnapshotContent(
	existingGroupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent,
	patch []PatchOp,
	client clientset.Interface,
	subresources ...string,
) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	data, err := json.Marshal(patch)
	if nil != err {
		return existingGroupSnapshotContent, err
	}

	newGroupSnapshotContent, err := client.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().Patch(context.TODO(), existingGroupSnapshotContent.Name, types.JSONPatchType, data, metav1.PatchOptions{}, subresources...)
	if err != nil {
		return existingGroupSnapshotContent, err
	}

	return newGroupSnapshotContent, nil
}

func PatchPersistentVolumeClaim(client kubernetes.Interface, oldPVC, newPVC *v1.PersistentVolumeClaim, addResourceVersionCheck bool) (*v1.PersistentVolumeClaim, error) {
	patchBytes, err := GetPVCPatchData(oldPVC, newPVC, addResourceVersionCheck)
	if err != nil {
		return oldPVC, fmt.Errorf("can't patch status of PVC %s as generate path data failed: %v", PVCKey(oldPVC), err)
	}
	updatedClaim, updateErr := client.CoreV1().PersistentVolumeClaims(oldPVC.Namespace).
		Patch(context.TODO(), oldPVC.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{}, "status")
	if updateErr != nil {
		return oldPVC, fmt.Errorf("can't patch status of  PVC %s with %v", PVCKey(oldPVC), updateErr)
	}

	return updatedClaim, nil
}

// PVCKey returns an unique key of a PVC object,
func PVCKey(pvc *v1.PersistentVolumeClaim) string {
	return fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
}

func GetPVCPatchData(oldPVC, newPVC *v1.PersistentVolumeClaim, addResourceVersionCheck bool) ([]byte, error) {
	patchBytes, err := GetPatchData(oldPVC, newPVC)
	if err != nil {
		return patchBytes, err
	}

	if addResourceVersionCheck {
		patchBytes, err = addResourceVersion(patchBytes, oldPVC.ResourceVersion)
		if err != nil {
			return nil, fmt.Errorf("apply ResourceVersion to patch data failed: %v", err)
		}
	}
	return patchBytes, nil
}

func GetPatchData(oldObj, newObj interface{}) ([]byte, error) {
	oldData, err := json.Marshal(oldObj)
	if err != nil {
		return nil, fmt.Errorf("marshal old object failed: %v", err)
	}
	newData, err := json.Marshal(newObj)
	if err != nil {
		return nil, fmt.Errorf("marshal new object failed: %v", err)
	}
	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, oldObj)
	if err != nil {
		return nil, fmt.Errorf("CreateTwoWayMergePatch failed: %v", err)
	}
	return patchBytes, nil
}

func addResourceVersion(patchBytes []byte, resourceVersion string) ([]byte, error) {
	var patchMap map[string]interface{}
	err := json.Unmarshal(patchBytes, &patchMap)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling patch with %v", err)
	}
	u := unstructured.Unstructured{Object: patchMap}
	a, err := meta.Accessor(&u)
	if err != nil {
		return nil, fmt.Errorf("error creating accessor with  %v", err)
	}
	a.SetResourceVersion(resourceVersion)
	versionBytes, err := json.Marshal(patchMap)
	if err != nil {
		return nil, fmt.Errorf("error marshalling json patch with %v", err)
	}
	return versionBytes, nil
}
