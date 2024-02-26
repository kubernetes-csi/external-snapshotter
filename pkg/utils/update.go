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
func UpdateRemoveFinalizers(object metav1.Object, client clientset.Interface, finalizers ...string) (metav1.Object, error) {
	object.SetFinalizers(RemoveStrings(object.GetFinalizers(), finalizers...))
	switch object.(type) {
	case *crdv1.VolumeSnapshot:
		obj, err := client.SnapshotV1().VolumeSnapshots(object.GetNamespace()).Update(context.TODO(), object.(*crdv1.VolumeSnapshot), metav1.UpdateOptions{})
		if err != nil && apierrors.IsConflict(err) {
			obj, err = client.SnapshotV1().VolumeSnapshots(object.GetNamespace()).Get(context.TODO(), object.GetName(), metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return UpdateRemoveFinalizers(obj, client, finalizers...)
		}
		if obj != nil && len(obj.GetFinalizers()) == 0 {
			// to satisfy some tests that requires nil rather than []string{}
			obj.SetFinalizers(nil)
		}
		return obj, err
	case *crdv1alpha1.VolumeGroupSnapshot:
		obj, err := client.GroupsnapshotV1alpha1().VolumeGroupSnapshots(object.GetNamespace()).Update(context.TODO(), object.(*crdv1alpha1.VolumeGroupSnapshot), metav1.UpdateOptions{})
		if err != nil && apierrors.IsConflict(err) {
			obj, err = client.GroupsnapshotV1alpha1().VolumeGroupSnapshots(object.GetNamespace()).Get(context.TODO(), object.GetName(), metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return UpdateRemoveFinalizers(obj, client, finalizers...)
		}
		if obj != nil && len(obj.GetFinalizers()) == 0 {
			// to satisfy some tests that requires nil rather than []string{}
			obj.SetFinalizers(nil)
		}
		return obj, err
	default:
		return nil, errors.New("UpdateRemoveFinalizers: unsupported object type")
	}
}

func UpdateRemoveFinalizersCoreV1(object metav1.Object, client kubernetes.Interface, finalizers ...string) (metav1.Object, error) {
	object.SetFinalizers(RemoveStrings(object.GetFinalizers(), finalizers...))
	pvc := object.(*corev1.PersistentVolumeClaim)
	pvc, err := client.CoreV1().PersistentVolumeClaims(object.GetNamespace()).Update(context.TODO(), pvc, metav1.UpdateOptions{})
	// if out of date, get the object and retry
	if err != nil && apierrors.IsConflict(err) {
		pvc, err = client.CoreV1().PersistentVolumeClaims(object.GetNamespace()).Get(context.TODO(), pvc.GetName(), metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return UpdateRemoveFinalizersCoreV1(pvc, client, finalizers...)
	}
	if pvc != nil && len(pvc.GetFinalizers()) == 0 {
		// to satisfy some tests that requires nil rather than []string{}
		pvc.SetFinalizers(nil)
	}
	return pvc, err
}
