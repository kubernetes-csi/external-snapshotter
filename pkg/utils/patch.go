package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumegroupsnapshot/v1alpha1"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v7/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubernetes "k8s.io/client-go/kubernetes"
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

// PatchVolumeGroupSnapshot patches a volume group snapshot object
func PatchVolumeGroupSnapshot(
	existingGroupSnapshot *crdv1alpha1.VolumeGroupSnapshot,
	patch []PatchOp,
	client clientset.Interface,
	subresources ...string,
) (*crdv1alpha1.VolumeGroupSnapshot, error) {
	data, err := json.Marshal(patch)
	if nil != err {
		return existingGroupSnapshot, err
	}

	newGroupSnapshot, err := client.GroupsnapshotV1alpha1().VolumeGroupSnapshots(existingGroupSnapshot.Namespace).Patch(context.TODO(), existingGroupSnapshot.Name, types.JSONPatchType, data, metav1.PatchOptions{}, subresources...)
	if err != nil {
		return existingGroupSnapshot, err
	}

	return newGroupSnapshot, nil
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

// Remove one or more finalizers from an object
// if finalizers is not empty, only the specified finalizers will be removed
func PatchRemoveFinalizers(object metav1.Object, client clientset.Interface, finalizers ...string) (metav1.Object, error) {
	data, err := PatchOpsBytesToRemoveFinalizers(object, finalizers...)
	if err != nil {
		return nil, err
	}
	switch object.(type) {
	case *crdv1.VolumeSnapshot:
		obj, err := client.SnapshotV1().VolumeSnapshots(object.GetNamespace()).Patch(context.TODO(), object.GetName(), types.JSONPatchType, data, metav1.PatchOptions{})
		if len(obj.Finalizers) == 0 {
			// to satisfy some tests that requires nil rather than []string{}
			obj.Finalizers = nil
		}
		return obj, err
	case *crdv1alpha1.VolumeGroupSnapshot:
		obj, err := client.GroupsnapshotV1alpha1().VolumeGroupSnapshots(object.GetNamespace()).Patch(context.TODO(), object.GetName(), types.JSONPatchType, data, metav1.PatchOptions{})
		if len(obj.Finalizers) == 0 {
			// to satisfy some tests that requires nil rather than []string{}
			obj.Finalizers = nil
		}
		return obj, err
	default:
		return nil, errors.New("PatchRemoveFinalizers: unsupported object type")
	}
}

func PatchRemoveFinalizersCorev1(object metav1.Object, client kubernetes.Interface, finalizers ...string) (metav1.Object, error) {
	data, err := PatchOpsBytesToRemoveFinalizers(object, finalizers...)
	if err != nil {
		return nil, err
	}
	obj, err := client.CoreV1().Pods(object.GetNamespace()).Patch(context.TODO(), object.GetName(), types.JSONPatchType, data, metav1.PatchOptions{})
	if len(obj.Finalizers) == 0 {
		// to satisfy some tests that requires nil rather than []string{}
		obj.Finalizers = nil
	}
	return obj, err
}

func PatchOpsToRemoveFinalizers(object metav1.Object, finalizers ...string) []PatchOp {
	patches := []PatchOp{}
	if len(finalizers) == 0 {
		return patches
	}
	// remove only the specified finalizers
	// get index of all finalizers to be removed
	// sort the indexes in descending order
	// remove the finalizers in descending order
	// this is to avoid the need to adjust the index after each removal
	indexes := []int{}
	for _, finalizer := range finalizers {
		for i, f := range object.GetFinalizers() {
			if f == finalizer {
				indexes = append(indexes, i)
			}
		}
	}
	slices.Sort(indexes)
	slices.Reverse(indexes)
	for _, i := range indexes {
		patches = append(patches, PatchOp{
			Op:   "remove",
			Path: "/metadata/finalizers/" + fmt.Sprint(i),
		})
	}
	return patches
}

func PatchOpsBytesToRemoveFinalizers(object metav1.Object, finalizers ...string) ([]byte, error) {
	patches := PatchOpsToRemoveFinalizers(object, finalizers...)
	return json.Marshal(patches)
}

func PatchOpsToAddFinalizers(object metav1.Object, finalizers ...string) []PatchOp {
	patches := []PatchOp{}
	for _, finalizer := range finalizers {
		patches = append(patches, PatchOp{
			Op:    "add",
			Path:  "/metadata/finalizers/-",
			Value: finalizer,
		})
	}
	return patches
}

func PatchOpsBytesToAddFinalizers(object metav1.Object, finalizers ...string) ([]byte, error) {
	patches := PatchOpsToAddFinalizers(object, finalizers...)
	return json.Marshal(patches)
}
