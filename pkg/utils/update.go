package utils

import (
	"context"
	"errors"

	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumegroupsnapshot/v1alpha1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v7/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Remove one or more finalizers from an object
// if finalizers is not empty, only the specified finalizers will be removed
// If update fails due to out of date, it will call get on the object and retry removing the finalizers
func UpdateRemoveFinalizers[O metav1.Object](
	object O,
	updateFunc func(context.Context, O, metav1.UpdateOptions) (O, error),
	getFunc func(context.Context, string, metav1.GetOptions) (O, error),
	finalizers ...string) (O, error) {
	for success := false; !success; {
		object.SetFinalizers(RemoveStrings(object.GetFinalizers(), finalizers...))
		updatedObject, err := updateFunc(context.TODO(), object, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsConflict(err) {
				object, err = getFunc(context.TODO(), object.GetName(), metav1.GetOptions{})
				if err != nil {
					return object, err
				}
				// retry removing finalizers
				continue
			} else {
				// return error if it's not a conflict
				return object, err
			}
		}
		success = true
		object = updatedObject
	}
	if len(object.GetFinalizers()) == 0 {
		// to satisfy some tests that requires nil rather than []string{}
		object.SetFinalizers(nil)
	}
	return object, nil
}

func UpdateRemoveFinalizersSnapshots(object metav1.Object, client clientset.Interface, finalizers ...string) (metav1.Object, error) {
	switch object.(type) {
	case *crdv1.VolumeSnapshot:
		return UpdateRemoveFinalizers[*crdv1.VolumeSnapshot](
			object.(*crdv1.VolumeSnapshot),
			client.SnapshotV1().VolumeSnapshots(object.GetNamespace()).Update,
			client.SnapshotV1().VolumeSnapshots(object.GetNamespace()).Get,
			finalizers...)
	case *crdv1alpha1.VolumeGroupSnapshot:
		return UpdateRemoveFinalizers[*crdv1alpha1.VolumeGroupSnapshot](
			object.(*crdv1alpha1.VolumeGroupSnapshot),
			client.GroupsnapshotV1alpha1().VolumeGroupSnapshots(object.GetNamespace()).Update,
			client.GroupsnapshotV1alpha1().VolumeGroupSnapshots(object.GetNamespace()).Get,
			finalizers...)
	default:
		return nil, errors.New("UpdateRemoveFinalizersSnapshots: unsupported object type")
	}
}

func UpdateRemoveFinalizersCoreV1(object metav1.Object, client kubernetes.Interface, finalizers ...string) (metav1.Object, error) {
	switch object.(type) {
	case *corev1.PersistentVolumeClaim:
		return UpdateRemoveFinalizers[*corev1.PersistentVolumeClaim](
			object.(*corev1.PersistentVolumeClaim),
			client.CoreV1().PersistentVolumeClaims(object.GetNamespace()).Update,
			client.CoreV1().PersistentVolumeClaims(object.GetNamespace()).Get,
			finalizers...)
	default:
		return nil, errors.New("UpdateRemoveFinalizersCoreV1: unsupported object type")
	}
}
