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
	"testing"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

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
