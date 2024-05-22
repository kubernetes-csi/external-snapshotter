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

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

var (
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
	klog.V(2).Info("admitting volumesnapshotclasses")

	reviewResponse := &v1.AdmissionResponse{
		Allowed: true,
		Result:  &metav1.Status{},
	}

	// Admit requests other than Update and Create
	if !(ar.Request.Operation == v1.Update || ar.Request.Operation == v1.Create) {
		return reviewResponse
	}

	raw := ar.Request.Object.Raw
	oldRaw := ar.Request.OldObject.Raw

	deserializer := codecs.UniversalDeserializer()
	switch ar.Request.Resource {
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
		err := fmt.Errorf("expect resource to be %s, but found %v",
			SnapshotClassV1GVR, ar.Request.Resource)
		klog.Error(err)
		return toV1AdmissionResponse(err)
	}
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
