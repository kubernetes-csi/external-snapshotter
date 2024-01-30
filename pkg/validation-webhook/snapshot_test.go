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
	"encoding/json"
	"fmt"
	"testing"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/client/v7/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v7/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAdmitVolumeSnapshotV1(t *testing.T) {
	pvcname := "pvcname1"
	mutatedField := "changed-immutable-field"
	contentname := "snapcontent1"
	volumeSnapshotClassName := "volume-snapshot-class-1"
	emptyVolumeSnapshotClassName := ""

	testCases := []struct {
		name              string
		volumeSnapshot    *volumesnapshotv1.VolumeSnapshot
		oldVolumeSnapshot *volumesnapshotv1.VolumeSnapshot
		shouldAdmit       bool
		msg               string
		operation         v1.Operation
	}{
		{
			name:              "Delete: new and old are nil. Should admit",
			volumeSnapshot:    nil,
			oldVolumeSnapshot: nil,
			shouldAdmit:       true,
			operation:         v1.Delete,
		},
		{
			name: "Create: old is nil and new is valid",
			volumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			oldVolumeSnapshot: nil,
			shouldAdmit:       true,
			operation:         v1.Create,
		},
		{
			name: "Update: old is valid and new is invalid",
			volumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
					VolumeSnapshotClassName: &emptyVolumeSnapshotClassName,
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: false,
			operation:   v1.Update,
			msg:         "Spec.VolumeSnapshotClassName must not be the empty string",
		},
		{
			name: "Update: old is valid and new is valid",
			volumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
					VolumeSnapshotClassName: &volumeSnapshotClassName,
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: true,
			operation:   v1.Update,
		},
		{
			name: "Update: old is valid and new is valid but changes immutable field spec.source",
			volumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &mutatedField,
					},
					VolumeSnapshotClassName: &volumeSnapshotClassName,
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: false,
			operation:   v1.Update,
			msg:         fmt.Sprintf("Spec.Source.VolumeSnapshotContentName is immutable but was changed from %s to %s", contentname, mutatedField),
		},
		{
			name: "Update: old is invalid and new is valid",
			volumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						PersistentVolumeClaimName: &pvcname,
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: false,
			operation:   v1.Update,
			msg:         fmt.Sprintf("Spec.Source.PersistentVolumeClaimName is immutable but was changed from %s to <nil string pointer>", pvcname),
		},
		{
			// will be handled by schema validation
			name: "Update: old is invalid and new is invalid",
			volumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
						PersistentVolumeClaimName: &pvcname,
					},
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1.VolumeSnapshot{
				Spec: volumesnapshotv1.VolumeSnapshotSpec{
					Source: volumesnapshotv1.VolumeSnapshotSource{
						PersistentVolumeClaimName: &pvcname,
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: true,
			operation:   v1.Update,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := tc.volumeSnapshot
			raw, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			oldSnapshot := tc.oldVolumeSnapshot
			oldRaw, err := json.Marshal(oldSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			review := v1.AdmissionReview{
				Request: &v1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: raw,
					},
					OldObject: runtime.RawExtension{
						Raw: oldRaw,
					},
					Resource:  SnapshotV1GVR,
					Operation: tc.operation,
				},
			}
			sa := NewSnapshotAdmitter(nil)
			response := sa.Admit(review)
			shouldAdmit := response.Allowed
			msg := response.Result.Message

			expectedResponse := tc.shouldAdmit
			expectedMsg := tc.msg

			if shouldAdmit != expectedResponse {
				t.Errorf("expected \"%v\" to equal \"%v\"", shouldAdmit, expectedResponse)
			}
			if msg != expectedMsg {
				t.Errorf("expected \"%v\" to equal \"%v\"", msg, expectedMsg)
			}
		})
	}
}

func TestAdmitVolumeSnapshotContentV1(t *testing.T) {
	volumeHandle := "volumeHandle1"
	modifiedField := "modified-field"
	snapshotHandle := "snapshotHandle1"
	volumeSnapshotClassName := "volume-snapshot-class-1"
	validContent := &volumesnapshotv1.VolumeSnapshotContent{
		Spec: volumesnapshotv1.VolumeSnapshotContentSpec{
			Source: volumesnapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: &snapshotHandle,
			},
			VolumeSnapshotRef: core_v1.ObjectReference{
				Name:      "snapshot-ref",
				Namespace: "default-ns",
			},
			VolumeSnapshotClassName: &volumeSnapshotClassName,
		},
	}
	invalidContent := &volumesnapshotv1.VolumeSnapshotContent{
		Spec: volumesnapshotv1.VolumeSnapshotContentSpec{
			Source: volumesnapshotv1.VolumeSnapshotContentSource{
				SnapshotHandle: &snapshotHandle,
				VolumeHandle:   &volumeHandle,
			},
			VolumeSnapshotRef: core_v1.ObjectReference{
				Name:      "",
				Namespace: "default-ns",
			},
		},
	}

	testCases := []struct {
		name                     string
		volumeSnapshotContent    *volumesnapshotv1.VolumeSnapshotContent
		oldVolumeSnapshotContent *volumesnapshotv1.VolumeSnapshotContent
		shouldAdmit              bool
		msg                      string
		operation                v1.Operation
	}{
		{
			name:                     "Delete: both new and old are nil",
			volumeSnapshotContent:    nil,
			oldVolumeSnapshotContent: nil,
			shouldAdmit:              true,
			operation:                v1.Delete,
		},
		{
			name:                     "Create: old is nil and new is valid",
			volumeSnapshotContent:    validContent,
			oldVolumeSnapshotContent: nil,
			shouldAdmit:              true,
			operation:                v1.Create,
		},
		{
			name:                     "Update: old is valid and new is invalid",
			volumeSnapshotContent:    invalidContent,
			oldVolumeSnapshotContent: validContent,
			shouldAdmit:              false,
			operation:                v1.Update,
			msg:                      fmt.Sprintf("Spec.Source.VolumeHandle is immutable but was changed from %s to %s", strPtrDereference(nil), volumeHandle),
		},
		{
			name:                     "Update: old is valid and new is valid",
			volumeSnapshotContent:    validContent,
			oldVolumeSnapshotContent: validContent,
			shouldAdmit:              true,
			operation:                v1.Update,
		},
		{
			name: "Update: old is valid and new is valid but modifies immutable field",
			volumeSnapshotContent: &volumesnapshotv1.VolumeSnapshotContent{
				Spec: volumesnapshotv1.VolumeSnapshotContentSpec{
					Source: volumesnapshotv1.VolumeSnapshotContentSource{
						SnapshotHandle: &modifiedField,
					},
					VolumeSnapshotRef: core_v1.ObjectReference{
						Name:      "snapshot-ref",
						Namespace: "default-ns",
					},
				},
			},
			oldVolumeSnapshotContent: validContent,
			shouldAdmit:              false,
			operation:                v1.Update,
			msg:                      fmt.Sprintf("Spec.Source.SnapshotHandle is immutable but was changed from %s to %s", snapshotHandle, modifiedField),
		},
		{
			name:                     "Update: old is invalid and new is valid",
			volumeSnapshotContent:    validContent,
			oldVolumeSnapshotContent: invalidContent,
			shouldAdmit:              false,
			operation:                v1.Update,
			msg:                      fmt.Sprintf("Spec.Source.VolumeHandle is immutable but was changed from %s to <nil string pointer>", volumeHandle),
		},
		{
			name:                     "Update: old is invalid and new is invalid",
			volumeSnapshotContent:    invalidContent,
			oldVolumeSnapshotContent: invalidContent,
			shouldAdmit:              false,
			operation:                v1.Update,
			msg:                      "both Spec.VolumeSnapshotRef.Name =  and Spec.VolumeSnapshotRef.Namespace = default-ns must be set",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snapshotContent := tc.volumeSnapshotContent
			raw, err := json.Marshal(snapshotContent)
			if err != nil {
				t.Fatal(err)
			}
			oldSnapshotContent := tc.oldVolumeSnapshotContent
			oldRaw, err := json.Marshal(oldSnapshotContent)
			if err != nil {
				t.Fatal(err)
			}
			review := v1.AdmissionReview{
				Request: &v1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: raw,
					},
					OldObject: runtime.RawExtension{
						Raw: oldRaw,
					},
					Resource:  SnapshotContentV1GVR,
					Operation: tc.operation,
				},
			}
			sa := NewSnapshotAdmitter(nil)
			response := sa.Admit(review)
			shouldAdmit := response.Allowed
			msg := response.Result.Message

			expectedResponse := tc.shouldAdmit
			expectedMsg := tc.msg

			if shouldAdmit != expectedResponse {
				t.Errorf("expected \"%v\" to equal \"%v\"", shouldAdmit, expectedResponse)
			}
			if msg != expectedMsg {
				t.Errorf("expected \"%v\" to equal \"%v\"", msg, expectedMsg)
			}
		})
	}
}

type fakeSnapshotLister struct {
	values []*volumesnapshotv1.VolumeSnapshotClass
}

func (f *fakeSnapshotLister) List(selector labels.Selector) (ret []*volumesnapshotv1.VolumeSnapshotClass, err error) {
	return f.values, nil
}

func (f *fakeSnapshotLister) Get(name string) (*volumesnapshotv1.VolumeSnapshotClass, error) {
	for _, v := range f.values {
		if v.Name == name {
			return v, nil
		}
	}
	return nil, nil
}

func TestAdmitVolumeSnapshotClassV1(t *testing.T) {
	testCases := []struct {
		name                   string
		volumeSnapshotClass    *volumesnapshotv1.VolumeSnapshotClass
		oldVolumeSnapshotClass *volumesnapshotv1.VolumeSnapshotClass
		shouldAdmit            bool
		msg                    string
		operation              v1.Operation
		lister                 storagelisters.VolumeSnapshotClassLister
	}{
		{
			name: "new default for class with no existing classes",
			volumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldVolumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{},
			shouldAdmit:            true,
			msg:                    "",
			operation:              v1.Create,
			lister:                 &fakeSnapshotLister{values: []*volumesnapshotv1.VolumeSnapshotClass{}},
		},
		{
			name: "new default for class for  with existing default class different drivers",
			volumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldVolumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{},
			shouldAdmit:            true,
			msg:                    "",
			operation:              v1.Create,
			lister: &fakeSnapshotLister{values: []*volumesnapshotv1.VolumeSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							utils.IsDefaultSnapshotClassAnnotation: "true",
						},
					},
					Driver: "existing.test.csi.io",
				},
			}},
		},
		{
			name: "new default for class with existing default class same driver",
			volumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldVolumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{},
			shouldAdmit:            false,
			msg:                    "default snapshot class: driver-a already exists for driver: test.csi.io",
			operation:              v1.Create,
			lister: &fakeSnapshotLister{values: []*volumesnapshotv1.VolumeSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "driver-a",
						Annotations: map[string]string{
							utils.IsDefaultSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
			}},
		},
		{
			name: "default for class with existing default class same driver update",
			volumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldVolumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			shouldAdmit: true,
			msg:         "",
			operation:   v1.Update,
			lister: &fakeSnapshotLister{values: []*volumesnapshotv1.VolumeSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							utils.IsDefaultSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
			}},
		},
		{
			name: "new snapshot for class with existing default class same driver",
			volumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Driver:     "test.csi.io",
			},
			oldVolumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{},
			shouldAdmit:            true,
			msg:                    "",
			operation:              v1.Create,
			lister: &fakeSnapshotLister{values: []*volumesnapshotv1.VolumeSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							utils.IsDefaultSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
			}},
		},
		{
			name: "new snapshot for class with existing default classes",
			volumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldVolumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{},
			shouldAdmit:            false,
			msg:                    "default snapshot class: driver-is-default already exists for driver: test.csi.io",
			operation:              v1.Create,
			lister: &fakeSnapshotLister{values: []*volumesnapshotv1.VolumeSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "driver-is-default",
						Annotations: map[string]string{
							utils.IsDefaultSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							utils.IsDefaultSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
			}},
		},
		{
			name: "update snapshot class to new driver with existing default classes",
			volumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultSnapshotClassAnnotation: "true",
					},
				},
				Driver: "driver.test.csi.io",
			},
			oldVolumeSnapshotClass: &volumesnapshotv1.VolumeSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			shouldAdmit: false,
			msg:         "default snapshot class: driver-test-default already exists for driver: driver.test.csi.io",
			operation:   v1.Update,
			lister: &fakeSnapshotLister{values: []*volumesnapshotv1.VolumeSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "driver-is-default",
						Annotations: map[string]string{
							utils.IsDefaultSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "driver-test-default",
						Annotations: map[string]string{
							utils.IsDefaultSnapshotClassAnnotation: "true",
						},
					},
					Driver: "driver.test.csi.io",
				},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snapshotContent := tc.volumeSnapshotClass
			raw, err := json.Marshal(snapshotContent)
			if err != nil {
				t.Fatal(err)
			}
			oldSnapshotClass := tc.oldVolumeSnapshotClass
			oldRaw, err := json.Marshal(oldSnapshotClass)
			if err != nil {
				t.Fatal(err)
			}
			review := v1.AdmissionReview{
				Request: &v1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: raw,
					},
					OldObject: runtime.RawExtension{
						Raw: oldRaw,
					},
					Resource:  SnapshotClassV1GVR,
					Operation: tc.operation,
				},
			}
			sa := NewSnapshotAdmitter(tc.lister)
			response := sa.Admit(review)

			shouldAdmit := response.Allowed
			msg := response.Result.Message

			expectedResponse := tc.shouldAdmit
			expectedMsg := tc.msg

			if shouldAdmit != expectedResponse {
				t.Errorf("expected \"%v\" to equal \"%v\"", shouldAdmit, expectedResponse)
			}
			if msg != expectedMsg {
				t.Errorf("expected \"%v\" to equal \"%v\"", msg, expectedMsg)
			}
		})
	}
}
