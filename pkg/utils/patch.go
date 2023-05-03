package utils

import (
	"context"
	"encoding/json"
	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
