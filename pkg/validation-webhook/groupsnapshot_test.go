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
	"encoding/json"
	"fmt"
	"testing"

	volumegroupsnapshotv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"
	groupsnapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v6/listers/volumegroupsnapshot/v1alpha1"
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

type fakeGroupSnapshotLister struct {
	values []*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass
}

func (f *fakeGroupSnapshotLister) List(selector labels.Selector) (ret []*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass, err error) {
	return f.values, nil
}

func (f *fakeGroupSnapshotLister) Get(name string) (*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass, error) {
	for _, v := range f.values {
		if v.Name == name {
			return v, nil
		}
	}
	return nil, nil
}

func TestAdmitVolumeGroupSnapshotV1Alpha1(t *testing.T) {
	selector := metav1.LabelSelector{MatchLabels: map[string]string{
		"group": "A",
	}}
	mutatedField := "changed-immutable-field"
	contentname := "groupsnapcontent1"
	emptyVolumeGroupSnapshotClassName := ""

	testCases := []struct {
		name                   string
		volumeGroupSnapshot    *volumegroupsnapshotv1alpha1.VolumeGroupSnapshot
		oldVolumeGroupSnapshot *volumegroupsnapshotv1alpha1.VolumeGroupSnapshot
		shouldAdmit            bool
		msg                    string
		operation              v1.Operation
	}{
		{
			name:                   "Delete: new and old are nil. Should admit",
			volumeGroupSnapshot:    nil,
			oldVolumeGroupSnapshot: nil,
			shouldAdmit:            true,
			operation:              v1.Delete,
		},
		{
			name: "Create: old is nil and new is valid, with contentname",
			volumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
					},
				},
			},
			oldVolumeGroupSnapshot: nil,
			shouldAdmit:            true,
			msg:                    "",
			operation:              v1.Create,
		},
		{
			name: "Create: old is nil and new is valid, with selector",
			volumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						Selector: selector,
					},
				},
			},
			oldVolumeGroupSnapshot: nil,
			shouldAdmit:            true,
			msg:                    "",
			operation:              v1.Create,
		},
		{
			name: "Update: old is valid and new is invalid",
			volumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
					},
					VolumeGroupSnapshotClassName: &emptyVolumeGroupSnapshotClassName,
				},
			},
			oldVolumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: false,
			operation:   v1.Update,
			msg:         "Spec.VolumeGroupSnapshotClassName must not be the empty string",
		},
		{
			name: "Update: old is valid and new is valid",
			volumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
					},
				},
			},
			oldVolumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: true,
			operation:   v1.Update,
		},
		{
			name: "Update: old is valid and new is valid but changes immutable field spec.source",
			volumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &mutatedField,
					},
				},
			},
			oldVolumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: false,
			operation:   v1.Update,
			msg:         fmt.Sprintf("Spec.Source.VolumeGroupSnapshotContentName is immutable but was changed from %s to %s", contentname, mutatedField),
		},
		{
			name: "Update: old is invalid and new is valid",
			volumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
					},
				},
			},
			oldVolumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
						Selector:                       selector,
					},
				},
			},
			shouldAdmit: false,
			operation:   v1.Update,
			msg:         fmt.Sprintf("Spec.Source.Selector is immutable but was changed from %v to %v", selector, metav1.LabelSelector{}),
		},
		{
			// will be handled by schema validation
			name: "Update: old is invalid and new is invalid",
			volumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
						Selector:                       selector,
					},
				},
			},
			oldVolumeGroupSnapshot: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshot{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotSource{
						VolumeGroupSnapshotContentName: &contentname,
						Selector:                       selector,
					},
				},
			},
			shouldAdmit: true,
			operation:   v1.Update,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			groupSnapshot := tc.volumeGroupSnapshot
			raw, err := json.Marshal(groupSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			oldGroupSnapshot := tc.oldVolumeGroupSnapshot
			oldRaw, err := json.Marshal(oldGroupSnapshot)
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
					Resource:  GroupSnapshotV1Alpha1GVR,
					Operation: tc.operation,
				},
			}
			sa := NewGroupSnapshotAdmitter(nil)
			response := sa.Admit(review)
			shouldAdmit := response.Allowed
			msg := response.Result.Message

			expectedResponse := tc.shouldAdmit
			expectedMsg := tc.msg

			if shouldAdmit != expectedResponse {
				t.Errorf("expected \"%v\" to equal \"%v\": %v", shouldAdmit, expectedResponse, msg)
			}
			if msg != expectedMsg {
				t.Errorf("expected \"%v\" to equal \"%v\"", msg, expectedMsg)
			}
		})
	}
}

func TestAdmitVolumeGroupSnapshotContentV1Alpha1(t *testing.T) {
	volumeHandle := "volumeHandle1"
	modifiedField := "modified-field"
	groupSnapshotHandle := "groupsnapshotHandle1"
	volumeGroupSnapshotClassName := "volume-snapshot-class-1"
	validContent := &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent{
		Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContentSpec{
			Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContentSource{
				VolumeGroupSnapshotHandle: &groupSnapshotHandle,
			},
			VolumeGroupSnapshotRef: core_v1.ObjectReference{
				Name:      "group-snapshot-ref",
				Namespace: "default-ns",
			},
			VolumeGroupSnapshotClassName: &volumeGroupSnapshotClassName,
		},
	}
	invalidContent := &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent{
		Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContentSpec{
			Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContentSource{
				VolumeGroupSnapshotHandle: &groupSnapshotHandle,
				PersistentVolumeNames:     []string{volumeHandle},
			},
			VolumeGroupSnapshotRef: core_v1.ObjectReference{
				Name:      "",
				Namespace: "default-ns",
			}},
	}

	testCases := []struct {
		name                string
		groupSnapContent    *volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent
		oldGroupSnapContent *volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent
		shouldAdmit         bool
		msg                 string
		operation           v1.Operation
	}{
		{
			name:                "Delete: both new and old are nil",
			groupSnapContent:    nil,
			oldGroupSnapContent: nil,
			shouldAdmit:         true,
			operation:           v1.Delete,
		},
		{
			name:                "Create: old is nil and new is valid",
			groupSnapContent:    validContent,
			oldGroupSnapContent: nil,
			shouldAdmit:         true,
			operation:           v1.Create,
		},
		{
			name:                "Update: old is valid and new is invalid",
			groupSnapContent:    invalidContent,
			oldGroupSnapContent: validContent,
			shouldAdmit:         false,
			operation:           v1.Update,
			msg:                 fmt.Sprintf("Spec.Source.PersistentVolumeNames is immutable but was changed from %s to %s", []string{}, []string{volumeHandle}),
		},
		{
			name:                "Update: old is valid and new is valid",
			groupSnapContent:    validContent,
			oldGroupSnapContent: validContent,
			shouldAdmit:         true,
			operation:           v1.Update,
		},
		{
			name: "Update: old is valid and new is valid but modifies immutable field",
			groupSnapContent: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContentSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContentSource{
						VolumeGroupSnapshotHandle: &modifiedField,
					},
					VolumeGroupSnapshotRef: core_v1.ObjectReference{
						Name:      "snapshot-ref",
						Namespace: "default-ns",
					},
				},
			},
			oldGroupSnapContent: validContent,
			shouldAdmit:         false,
			operation:           v1.Update,
			msg:                 fmt.Sprintf("Spec.Source.VolumeGroupSnapshotHandle is immutable but was changed from %s to %s", groupSnapshotHandle, modifiedField),
		},
		{
			name: "Update: old is valid and new is valid but modifies immutable ref",
			groupSnapContent: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContent{
				Spec: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContentSpec{
					Source: volumegroupsnapshotv1alpha1.VolumeGroupSnapshotContentSource{
						VolumeGroupSnapshotHandle: &groupSnapshotHandle,
					},
					VolumeGroupSnapshotRef: core_v1.ObjectReference{
						Name:      modifiedField,
						Namespace: "default-ns",
					},
				},
			},
			oldGroupSnapContent: validContent,
			shouldAdmit:         false,
			operation:           v1.Update,
			msg: fmt.Sprintf("Spec.VolumeGroupSnapshotRef.Name is immutable but was changed from %s to %s",
				validContent.Spec.VolumeGroupSnapshotRef.Name, modifiedField),
		},
		{
			name:                "Update: old is invalid and new is valid",
			groupSnapContent:    validContent,
			oldGroupSnapContent: invalidContent,
			shouldAdmit:         false,
			operation:           v1.Update,
			msg:                 fmt.Sprintf("Spec.Source.PersistentVolumeNames is immutable but was changed from %s to %s", []string{volumeHandle}, []string{}),
		},
		{
			name:                "Update: old is invalid and new is invalid",
			groupSnapContent:    invalidContent,
			oldGroupSnapContent: invalidContent,
			shouldAdmit:         false,
			operation:           v1.Update,
			msg:                 "both Spec.VolumeGroupSnapshotRef.Name =  and Spec.VolumeGroupSnapshotRef.Namespace = default-ns must be set",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			groupSnapContent := tc.groupSnapContent
			raw, err := json.Marshal(groupSnapContent)
			if err != nil {
				t.Fatal(err)
			}
			oldGroupSnapContent := tc.oldGroupSnapContent
			oldRaw, err := json.Marshal(oldGroupSnapContent)
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
					Resource:  GroupSnapshotContentV1Apha1GVR,
					Operation: tc.operation,
				},
			}
			sa := NewGroupSnapshotAdmitter(nil)
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

func TestAdmitVolumeGroupSnapshotClassV1Alpha1(t *testing.T) {
	testCases := []struct {
		name              string
		groupSnapClass    *volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass
		oldGroupSnapClass *volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass
		shouldAdmit       bool
		msg               string
		operation         v1.Operation
		lister            groupsnapshotlisters.VolumeGroupSnapshotClassLister
	}{
		{
			name: "new default for group snapshot class with no existing group snapshot classes",
			groupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultGroupSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldGroupSnapClass: nil,
			shouldAdmit:       true,
			msg:               "",
			operation:         v1.Create,
			lister:            &fakeGroupSnapshotLister{values: []*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{}},
		},
		{
			name: "new default for group snapshot class for  with existing default group snapshot class with different drivers",
			groupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultGroupSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldGroupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{},
			shouldAdmit:       true,
			msg:               "",
			operation:         v1.Create,
			lister: &fakeGroupSnapshotLister{values: []*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							utils.IsDefaultGroupSnapshotClassAnnotation: "true",
						},
					},
					Driver: "existing.test.csi.io",
				},
			}},
		},
		{
			name: "new default for group snapshot class with existing default group snapshot class same driver",
			groupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultGroupSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldGroupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{},
			shouldAdmit:       false,
			msg:               "default group snapshot class: driver-a already exists for driver: test.csi.io",
			operation:         v1.Create,
			lister: &fakeGroupSnapshotLister{values: []*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "driver-a",
						Annotations: map[string]string{
							utils.IsDefaultGroupSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
			}},
		},
		{
			name: "default for group snapshot class with existing default group snapshot class same driver update",
			groupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultGroupSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldGroupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultGroupSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			shouldAdmit: true,
			msg:         "",
			operation:   v1.Update,
			lister: &fakeGroupSnapshotLister{values: []*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							utils.IsDefaultGroupSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
			}},
		},
		{
			name: "new group snapshot for group snapshot class with existing default group snapshot class same driver",
			groupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Driver:     "test.csi.io",
			},
			oldGroupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{},
			shouldAdmit:       true,
			msg:               "",
			operation:         v1.Create,
			lister: &fakeGroupSnapshotLister{values: []*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							utils.IsDefaultGroupSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
			}},
		},
		{
			name: "new group snapshot for group snapshot class with existing group snapshot default classes",
			groupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultGroupSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			oldGroupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{},
			shouldAdmit:       false,
			msg:               "default group snapshot class: driver-is-default already exists for driver: test.csi.io",
			operation:         v1.Create,
			lister: &fakeGroupSnapshotLister{[]*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "driver-is-default",
						Annotations: map[string]string{
							utils.IsDefaultGroupSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							utils.IsDefaultGroupSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
			}},
		},
		{
			name: "update group snapshot class to new driver with existing default group snapshot classes",
			groupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultGroupSnapshotClassAnnotation: "true",
					},
				},
				Driver: "driver.test.csi.io",
			},
			oldGroupSnapClass: &volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.IsDefaultGroupSnapshotClassAnnotation: "true",
					},
				},
				Driver: "test.csi.io",
			},
			shouldAdmit: false,
			msg:         "default group snapshot class: driver-test-default already exists for driver: driver.test.csi.io",
			operation:   v1.Update,
			lister: &fakeGroupSnapshotLister{values: []*volumegroupsnapshotv1alpha1.VolumeGroupSnapshotClass{
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "driver-is-default",
						Annotations: map[string]string{
							utils.IsDefaultGroupSnapshotClassAnnotation: "true",
						},
					},
					Driver: "test.csi.io",
				},
				{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name: "driver-test-default",
						Annotations: map[string]string{
							utils.IsDefaultGroupSnapshotClassAnnotation: "true",
						},
					},
					Driver: "driver.test.csi.io",
				},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			groupSnapContent := tc.groupSnapClass
			raw, err := json.Marshal(groupSnapContent)
			if err != nil {
				t.Fatal(err)
			}
			oldGroupSnapClass := tc.oldGroupSnapClass
			oldRaw, err := json.Marshal(oldGroupSnapClass)
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
					Resource:  GroupSnapshotClassV1Apha1GVR,
					Operation: tc.operation,
				},
			}
			sa := NewGroupSnapshotAdmitter(tc.lister)
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
