/*
Copyright 2023 The Kubernetes Authors.

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

	volumegroupsnapshotv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumegroupsnapshot/v1alpha1"
	groupsnapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v7/listers/volumegroupsnapshot/v1alpha1"
	"github.com/kubernetes-csi/external-snapshotter/v7/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

var (
	// GroupSnapshotV1Alpha1GVR is GroupVersionResource for v1alpha1 VolumeGroupSnapshots
	GroupSnapshotV1Alpha1GVR = metav1.GroupVersionResource{Group: volumegroupsnapshotv1alpha1.GroupName, Version: "v1alpha1", Resource: "volumegroupsnapshots"}
	// GroupSnapshotContentV1Apha1GVR is GroupVersionResource for v1alpha1 VolumeGroupSnapshotContents
	GroupSnapshotContentV1Apha1GVR = metav1.GroupVersionResource{Group: volumegroupsnapshotv1alpha1.GroupName, Version: "v1alpha1", Resource: "volumegroupsnapshotcontents"}
	// GroupSnapshotClassV1Apha1GVR is GroupVersionResource for v1alpha1 VolumeGroupSnapshotClasses
	GroupSnapshotClassV1Apha1GVR = metav1.GroupVersionResource{Group: volumegroupsnapshotv1alpha1.GroupName, Version: "v1alpha1", Resource: "volumegroupsnapshotclasses"}
)

type GroupSnapshotAdmitter interface {
	Admit(v1.AdmissionReview) *v1.AdmissionResponse
}

type groupSnapshotAdmitter struct {
	lister groupsnapshotlisters.VolumeGroupSnapshotClassLister
}

func NewGroupSnapshotAdmitter(lister groupsnapshotlisters.VolumeGroupSnapshotClassLister) GroupSnapshotAdmitter {
	return &groupSnapshotAdmitter{
		lister: lister,
	}
}

// Add a label {"added-label": "yes"} to the object
func (a groupSnapshotAdmitter) Admit(ar v1.AdmissionReview) *v1.AdmissionResponse {
	klog.V(2).Info("admitting volumegroupsnapshots volumegroupsnapshotcontents " +
		"or volumegroupsnapshotclasses")

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
	case GroupSnapshotV1Alpha1GVR:
		groupSnapshot := &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{}
		if _, _, err := deserializer.Decode(raw, nil, groupSnapshot); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		oldGroupSnapshot := &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{}
		if _, _, err := deserializer.Decode(oldRaw, nil, oldGroupSnapshot); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		return decideGroupSnapshotV1Alpha1(groupSnapshot, oldGroupSnapshot, isUpdate)
	case GroupSnapshotContentV1Apha1GVR:
		groupSnapContent := &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent{}
		if _, _, err := deserializer.Decode(raw, nil, groupSnapContent); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		oldGroupSnapContent := &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent{}
		if _, _, err := deserializer.Decode(oldRaw, nil, oldGroupSnapContent); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		return decideGroupSnapshotContentV1Alpha1(groupSnapContent, oldGroupSnapContent, isUpdate)
	case GroupSnapshotClassV1Apha1GVR:
		groupSnapClass := &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{}
		if _, _, err := deserializer.Decode(raw, nil, groupSnapClass); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		oldGroupSnapClass := &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{}
		if _, _, err := deserializer.Decode(oldRaw, nil, oldGroupSnapClass); err != nil {
			klog.Error(err)
			return toV1AdmissionResponse(err)
		}
		return decideGroupSnapshotClassV1Alpha1(groupSnapClass, oldGroupSnapClass, a.lister)
	default:
		err := fmt.Errorf("expect resource to be %s, %s, or %s, but found %v",
			GroupSnapshotV1Alpha1GVR, GroupSnapshotContentV1Apha1GVR,
			GroupSnapshotClassV1Apha1GVR, ar.Request.Resource)
		klog.Error(err)
		return toV1AdmissionResponse(err)
	}
}

func decideGroupSnapshotV1Alpha1(groupSnapshot, oldGroupSnapshot *volumegroupsnapshotv1alpha1.VolumeGroupSnapshot, isUpdate bool) *v1.AdmissionResponse {
	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	if isUpdate {
		// if it is an UPDATE and oldGroupSnapshot is valid, check immutable fields
		if err := checkGroupSnapshotImmutableFieldsV1Alpha1(groupSnapshot, oldGroupSnapshot); err != nil {
			reviewResponse.Allowed = false
			reviewResponse.Result.Message = err.Error()
			return reviewResponse
		}
	}
	// Enforce strict validation for CREATE requests. Immutable checks don't apply for CREATE requests.
	// Enforce strict validation for UPDATE requests where old is valid and passes immutability check.
	if err := ValidateV1Alpha1GroupSnapshot(groupSnapshot); err != nil {
		reviewResponse.Allowed = false
		reviewResponse.Result.Message = err.Error()
	}
	return reviewResponse
}

func decideGroupSnapshotContentV1Alpha1(groupSnapcontent, oldGroupSnapcontent *volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent, isUpdate bool) *v1.AdmissionResponse {
	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	if isUpdate {
		// if it is an UPDATE and oldGroupSnapcontent is valid, check immutable fields
		if err := checkGroupSnapshotContentImmutableFieldsV1Alpha1(groupSnapcontent, oldGroupSnapcontent); err != nil {
			reviewResponse.Allowed = false
			reviewResponse.Result.Message = err.Error()
			return reviewResponse
		}
	}
	// Enforce strict validation for all CREATE requests. Immutable checks don't apply for CREATE requests.
	// Enforce strict validation for UPDATE requests where old is valid and passes immutability check.
	if err := ValidateV1Alpha1GroupSnapshotContent(groupSnapcontent); err != nil {
		reviewResponse.Allowed = false
		reviewResponse.Result.Message = err.Error()
	}
	return reviewResponse
}

func decideGroupSnapshotClassV1Alpha1(groupSnapClass, oldGroupSnapClass *volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass, lister groupsnapshotlisters.VolumeGroupSnapshotClassLister) *v1.AdmissionResponse {
	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	// Only Validate when a new group snapshot class is being set as a default.
	if groupSnapClass.Annotations[utils.IsDefaultGroupSnapshotClassAnnotation] != "true" {
		return reviewResponse
	}

	// If the old group snapshot class has this, then we can assume that it was validated if driver is the same.
	if oldGroupSnapClass.Annotations[utils.IsDefaultGroupSnapshotClassAnnotation] == "true" && oldGroupSnapClass.Driver == groupSnapClass.Driver {
		return reviewResponse
	}

	ret, err := lister.List(labels.Everything())
	if err != nil {
		reviewResponse.Allowed = false
		reviewResponse.Result.Message = err.Error()
		return reviewResponse
	}

	for _, groupSnapshotClass := range ret {
		if groupSnapshotClass.Annotations[utils.IsDefaultGroupSnapshotClassAnnotation] != "true" {
			continue
		}
		if groupSnapshotClass.Driver == groupSnapClass.Driver {
			reviewResponse.Allowed = false
			reviewResponse.Result.Message = fmt.Sprintf("default group snapshot class: %v already exists for driver: %v", groupSnapshotClass.Name, groupSnapClass.Driver)
			return reviewResponse
		}
	}

	return reviewResponse
}

func checkGroupSnapshotImmutableFieldsV1Alpha1(groupSnapshot, oldGroupSnapshot *volumegroupsnapshotv1alpha1.VolumeGroupSnapshot) error {
	if groupSnapshot == nil {
		return fmt.Errorf("VolumeGroupSnapshot is nil")
	}
	if oldGroupSnapshot == nil {
		return fmt.Errorf("old VolumeGroupSnapshot is nil")
	}

	source := groupSnapshot.Spec.Source
	oldSource := oldGroupSnapshot.Spec.Source

	if !reflect.DeepEqual(source.Selector, oldSource.Selector) {
		return fmt.Errorf("Spec.Source.Selector is immutable but was changed from %s to %s", oldSource.Selector, source.Selector)
	}
	if !reflect.DeepEqual(source.VolumeGroupSnapshotContentName, oldSource.VolumeGroupSnapshotContentName) {
		return fmt.Errorf("Spec.Source.VolumeGroupSnapshotContentName is immutable but was changed from %s to %s", strPtrDereference(oldSource.VolumeGroupSnapshotContentName), strPtrDereference(source.VolumeGroupSnapshotContentName))
	}

	return nil
}

func checkGroupSnapshotContentImmutableFieldsV1Alpha1(groupSnapcontent, oldGroupSnapcontent *volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent) error {
	if groupSnapcontent == nil {
		return fmt.Errorf("VolumeGroupSnapshotContent is nil")
	}
	if oldGroupSnapcontent == nil {
		return fmt.Errorf("old VolumeGroupSnapshotContent is nil")
	}

	source := groupSnapcontent.Spec.Source
	oldSource := oldGroupSnapcontent.Spec.Source

	if !reflect.DeepEqual(source.GroupSnapshotHandles, oldSource.GroupSnapshotHandles) {
		return fmt.Errorf("Spec.Source.GroupSnapshotHandles is immutable but was changed from %s to %s", oldSource.GroupSnapshotHandles, source.GroupSnapshotHandles)
	}
	if !reflect.DeepEqual(source.VolumeHandles, oldSource.VolumeHandles) {
		return fmt.Errorf("Spec.Source.VolumeHandles is immutable but was changed from %v to %v", oldSource.VolumeHandles, source.VolumeHandles)
	}

	ref := groupSnapcontent.Spec.VolumeGroupSnapshotRef
	oldRef := oldGroupSnapcontent.Spec.VolumeGroupSnapshotRef

	if ref.Name != oldRef.Name {
		return fmt.Errorf("Spec.VolumeGroupSnapshotRef.Name is immutable but was changed from %s to %s", oldRef.Name, ref.Name)
	}
	if ref.Namespace != oldRef.Namespace {
		return fmt.Errorf("Spec.VolumeGroupSnapshotRef.Namespace is immutable but was changed from %s to %s", oldRef.Namespace, ref.Namespace)
	}

	return nil
}
