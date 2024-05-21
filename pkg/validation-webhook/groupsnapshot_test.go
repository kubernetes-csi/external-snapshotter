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
	"testing"

	volumegroupsnapshotv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1alpha1"
	groupsnapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumegroupsnapshot/v1alpha1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/admission/v1"
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
