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

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/client/v6/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

var (
	// SnapshotV1GVR is GroupVersionResource for v1 VolumeSnapshots
	SnapshotV1GVR = metav1.GroupVersionResource{Group: volumesnapshotv1.GroupName, Version: "v1", Resource: "volumesnapshots"}
	// SnapshotContentV1GVR is GroupVersionResource for v1 VolumeSnapshotContents
	SnapshotContentV1GVR = metav1.GroupVersionResource{Group: volumesnapshotv1.GroupName, Version: "v1", Resource: "volumesnapshotcontents"}
	// SnapshotContentV1GVR is GroupVersionResource for v1 VolumeSnapshotContents
	SnapshotClassV1GVR = metav1.GroupVersionResource{Group: volumesnapshotv1.GroupName, Version: "v1", Resource: "volumesnapshotclasses"}
)

type SnapshotAdmitter interface {
	Admit(v1.AdmissionReview) *v1.AdmissionResponse
}

type admitter struct {
	lister storagelisters.VolumeSnapshotClassLister
}

func NewSnapshotAdmitter(lister storagelisters.VolumeSnapshotClassLister) SnapshotAdmitter {
	return &admitter{
		lister: lister,
	}
}

// Add a label {"added-label": "yes"} to the object
func (a admitter) Admit(ar v1.AdmissionReview) *v1.AdmissionResponse {
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
	case SnapshotV1GVR:
		snapshot := &volumesnapshotv1.VolumeSnapshot{}
		if _, _, err := deserializer.Decode(raw, nil, snapshot); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		oldSnapshot := &volumesnapshotv1.VolumeSnapshot{}
		if _, _, err := deserializer.Decode(oldRaw, nil, oldSnapshot); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		return decideSnapshotV1(snapshot, oldSnapshot, isUpdate)
	case SnapshotContentV1GVR:
		snapcontent := &volumesnapshotv1.VolumeSnapshotContent{}
		if _, _, err := deserializer.Decode(raw, nil, snapcontent); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		oldSnapcontent := &volumesnapshotv1.VolumeSnapshotContent{}
		if _, _, err := deserializer.Decode(oldRaw, nil, oldSnapcontent); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		return decideSnapshotContentV1(snapcontent, oldSnapcontent, isUpdate)
	case SnapshotClassV1GVR:
		snapClass := &volumesnapshotv1.VolumeSnapshotClass{}
		if _, _, err := deserializer.Decode(raw, nil, snapClass); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		oldSnapClass := &volumesnapshotv1.VolumeSnapshotClass{}
		if _, _, err := deserializer.Decode(oldRaw, nil, oldSnapClass); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		return decideSnapshotClassV1(snapClass, oldSnapClass, a.lister)
	default:
		err := fmt.Errorf("expect resource to be %s, %s, or %s, but found %v",
			SnapshotV1GVR, SnapshotContentV1GVR, SnapshotClassV1GVR, ar.Request.Resource)
		klog.Error(err)
		return toV1AdmissionResponse(err)
	}
}

func decideSnapshotV1(snapshot, oldSnapshot *volumesnapshotv1.VolumeSnapshot, isUpdate bool) *v1.AdmissionResponse {
	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	if isUpdate {
		// if it is an UPDATE and oldSnapshot is valid, check immutable fields
		if err := checkSnapshotImmutableFieldsV1(snapshot, oldSnapshot); err != nil {
			reviewResponse.Allowed = false
			reviewResponse.Result.Message = err.Error()
			return reviewResponse
		}
	}
	// Enforce strict validation for CREATE requests. Immutable checks don't apply for CREATE requests.
	// Enforce strict validation for UPDATE requests where old is valid and passes immutability check.
	if err := ValidateV1Snapshot(snapshot); err != nil {
		reviewResponse.Allowed = false
		reviewResponse.Result.Message = err.Error()
	}
	return reviewResponse
}

func decideSnapshotContentV1(snapcontent, oldSnapcontent *volumesnapshotv1.VolumeSnapshotContent, isUpdate bool) *v1.AdmissionResponse {
	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	if isUpdate {
		// if it is an UPDATE and oldSnapcontent is valid, check immutable fields
		if err := checkSnapshotContentImmutableFieldsV1(snapcontent, oldSnapcontent); err != nil {
			reviewResponse.Allowed = false
			reviewResponse.Result.Message = err.Error()
			return reviewResponse
		}
	}
	// Enforce strict validation for all CREATE requests. Immutable checks don't apply for CREATE requests.
	// Enforce strict validation for UPDATE requests where old is valid and passes immutability check.
	if err := ValidateV1SnapshotContent(snapcontent); err != nil {
		reviewResponse.Allowed = false
		reviewResponse.Result.Message = err.Error()
	}
	return reviewResponse
}

func decideSnapshotClassV1(snapClass, oldSnapClass *volumesnapshotv1.VolumeSnapshotClass, lister storagelisters.VolumeSnapshotClassLister) *v1.AdmissionResponse {
	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	// Only Validate when a new snapClass is being set as a default.
	if snapClass.Annotations[utils.IsDefaultSnapshotClassAnnotation] != "true" {
		return reviewResponse
	}

	// If Old snapshot class has this, then we can assume that it was validated if driver is the same.
	if oldSnapClass.Annotations[utils.IsDefaultSnapshotClassAnnotation] == "true" && oldSnapClass.Driver == snapClass.Driver {
		return reviewResponse
	}

	ret, err := lister.List(labels.Everything())
	if err != nil {
		reviewResponse.Allowed = false
		reviewResponse.Result.Message = err.Error()
		return reviewResponse
	}

	for _, snapshotClass := range ret {
		if snapshotClass.Annotations[utils.IsDefaultSnapshotClassAnnotation] != "true" {
			continue
		}
		if snapshotClass.Driver == snapClass.Driver {
			reviewResponse.Allowed = false
			reviewResponse.Result.Message = fmt.Sprintf("default snapshot class: %v already exists for driver: %v", snapshotClass.Name, snapClass.Driver)
			return reviewResponse
		}
	}

	return reviewResponse
}

func strPtrDereference(s *string) string {
	if s == nil {
		return "<nil string pointer>"
	}
	return *s
}

func checkSnapshotImmutableFieldsV1(snapshot, oldSnapshot *volumesnapshotv1.VolumeSnapshot) error {
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

func checkSnapshotContentImmutableFieldsV1(snapcontent, oldSnapcontent *volumesnapshotv1.VolumeSnapshotContent) error {
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

	if preventVolumeModeConversion {
		if !reflect.DeepEqual(snapcontent.Spec.SourceVolumeMode, oldSnapcontent.Spec.SourceVolumeMode) {
			return fmt.Errorf("Spec.SourceVolumeMode is immutable but was changed from %v to %v", *oldSnapcontent.Spec.SourceVolumeMode, *snapcontent.Spec.SourceVolumeMode)
		}
	}

	return nil
}
