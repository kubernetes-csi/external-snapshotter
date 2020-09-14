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
	"fmt"
	"reflect"

	volumesnapshotv1beta1 "github.com/kubernetes-csi/external-snapshotter/client/v3/apis/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/v3/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

var (
	// SnapshotV1Beta1GVR is GroupVersionResource for volumesnapshots
	SnapshotV1Beta1GVR = metav1.GroupVersionResource{Group: volumesnapshotv1beta1.GroupName, Version: "v1beta1", Resource: "volumesnapshots"}
	// SnapshotContentV1Beta1GVR is GroupVersionResource for volumesnapshotcontents
	SnapshotContentV1Beta1GVR = metav1.GroupVersionResource{Group: volumesnapshotv1beta1.GroupName, Version: "v1beta1", Resource: "volumesnapshotcontents"}
)

// Add a label {"added-label": "yes"} to the object
func admitSnapshot(ar v1.AdmissionReview) *v1.AdmissionResponse {
	klog.V(2).Info("admitting volumesnapshots or volumesnapshotcontents")

	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	// Admit requests other than Update and Create
	if !(ar.Request.Operation == v1.Update || ar.Request.Operation == v1.Create) {
		return reviewResponse
	}
	isUpdate := ar.Request.Operation == v1.Update

	raw := ar.Request.Object.Raw
	oldRaw := ar.Request.OldObject.Raw

	deserializer := codecs.UniversalDeserializer()
	switch ar.Request.Resource {
	case SnapshotV1Beta1GVR:
		snapshot := &volumesnapshotv1beta1.VolumeSnapshot{}
		if _, _, err := deserializer.Decode(raw, nil, snapshot); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		oldSnapshot := &volumesnapshotv1beta1.VolumeSnapshot{}
		if _, _, err := deserializer.Decode(oldRaw, nil, oldSnapshot); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		return decideSnapshot(snapshot, oldSnapshot, isUpdate)
	case SnapshotContentV1Beta1GVR:
		snapcontent := &volumesnapshotv1beta1.VolumeSnapshotContent{}
		if _, _, err := deserializer.Decode(raw, nil, snapcontent); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		oldSnapcontent := &volumesnapshotv1beta1.VolumeSnapshotContent{}
		if _, _, err := deserializer.Decode(oldRaw, nil, oldSnapcontent); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		return decideSnapshotContent(snapcontent, oldSnapcontent, isUpdate)
	default:
		err := fmt.Errorf("expect resource to be %s or %s", SnapshotV1Beta1GVR, SnapshotContentV1Beta1GVR)
		klog.Error(err)
		return toV1AdmissionResponse(err)
	}
}

func decideSnapshot(snapshot, oldSnapshot *volumesnapshotv1beta1.VolumeSnapshot, isUpdate bool) *v1.AdmissionResponse {
	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	if isUpdate {
		// if it is an UPDATE and oldSnapshot is not valid, then don't enforce strict validation
		// This allows no-op updates to occur on snapshot resources which fail strict validation
		// Which allows the remover of finalizers and therefore deletion of this object
		// Don't rely on the pointers to be nil, because the deserialization method will convert it to
		// The empty struct value. Instead check the operation type.
		if err := utils.ValidateSnapshot(oldSnapshot); err != nil {
			return reviewResponse
		}

		// if it is an UPDATE and oldSnapshot is valid, check immutable fields
		if err := checkSnapshotImmutableFields(snapshot, oldSnapshot); err != nil {
			reviewResponse.Allowed = false
			reviewResponse.Result.Message = err.Error()
			return reviewResponse
		}
	}
	// Enforce strict validation for CREATE requests. Immutable checks don't apply for CREATE requests.
	// Enforce strict validation for UPDATE requests where old is valid and passes immutability check.
	if err := utils.ValidateSnapshot(snapshot); err != nil {
		reviewResponse.Allowed = false
		reviewResponse.Result.Message = err.Error()
	}
	return reviewResponse
}

func decideSnapshotContent(snapcontent, oldSnapcontent *volumesnapshotv1beta1.VolumeSnapshotContent, isUpdate bool) *v1.AdmissionResponse {
	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	if isUpdate {
		// if it is an UPDATE and oldSnapcontent is not valid, then don't enforce strict validation
		// This allows no-op updates to occur on snapshot resources which fail strict validation
		// Which allows the remover of finalizers and therefore deletion of this object
		// Don't rely on the pointers to be nil, because the deserialization method will convert it to
		// The empty struct value. Instead check the operation type.
		if err := utils.ValidateSnapshotContent(oldSnapcontent); err != nil {
			return reviewResponse
		}

		// if it is an UPDATE and oldSnapcontent is valid, check immutable fields
		if err := checkSnapshotContentImmutableFields(snapcontent, oldSnapcontent); err != nil {
			reviewResponse.Allowed = false
			reviewResponse.Result.Message = err.Error()
			return reviewResponse
		}
	}
	// Enforce strict validation for all CREATE requests. Immutable checks don't apply for CREATE requests.
	// Enforce strict validation for UPDATE requests where old is valid and passes immutability check.
	if err := utils.ValidateSnapshotContent(snapcontent); err != nil {
		reviewResponse.Allowed = false
		reviewResponse.Result.Message = err.Error()
	}
	return reviewResponse
}

func strPtrDereference(s *string) string {
	if s == nil {
		return "<nil string pointer>"
	}
	return *s
}
func checkSnapshotImmutableFields(snapshot, oldSnapshot *volumesnapshotv1beta1.VolumeSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("VolumeSnapshot is nil")
	}
	if oldSnapshot == nil {
		return fmt.Errorf("old VolumeSnapshot is nil")
	}

	source := snapshot.Spec.Source
	oldSource := oldSnapshot.Spec.Source

	if !reflect.DeepEqual(source.PersistentVolumeClaimName, oldSource.PersistentVolumeClaimName) {
		return fmt.Errorf("Spec.Source.PersistentVolumeClaimName is immutable but was changed from %s to %s", strPtrDereference(oldSource.PersistentVolumeClaimName), strPtrDereference(source.PersistentVolumeClaimName))
	}
	if !reflect.DeepEqual(source.VolumeSnapshotContentName, oldSource.VolumeSnapshotContentName) {
		return fmt.Errorf("Spec.Source.VolumeSnapshotContentName is immutable but was changed from %s to %s", strPtrDereference(oldSource.VolumeSnapshotContentName), strPtrDereference(source.VolumeSnapshotContentName))
	}

	return nil
}

func checkSnapshotContentImmutableFields(snapcontent, oldSnapcontent *volumesnapshotv1beta1.VolumeSnapshotContent) error {
	if snapcontent == nil {
		return fmt.Errorf("VolumeSnapshotContent is nil")
	}
	if oldSnapcontent == nil {
		return fmt.Errorf("old VolumeSnapshotContent is nil")
	}

	source := snapcontent.Spec.Source
	oldSource := oldSnapcontent.Spec.Source

	if !reflect.DeepEqual(source.VolumeHandle, oldSource.VolumeHandle) {
		return fmt.Errorf("Spec.Source.VolumeHandle is immutable but was changed from %s to %s", strPtrDereference(oldSource.VolumeHandle), strPtrDereference(source.VolumeHandle))
	}
	if !reflect.DeepEqual(source.SnapshotHandle, oldSource.SnapshotHandle) {
		return fmt.Errorf("Spec.Source.SnapshotHandle is immutable but was changed from %s to %s", strPtrDereference(oldSource.SnapshotHandle), strPtrDereference(source.SnapshotHandle))
	}
	return nil
}
