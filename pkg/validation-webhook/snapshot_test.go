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

	volumesnapshotv1beta1 "github.com/kubernetes-csi/external-snapshotter/client/v3/apis/volumesnapshot/v1beta1"
	v1 "k8s.io/api/admission/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAdmitVolumeSnapshot(t *testing.T) {
	pvcname := "pvcname1"
	mutatedField := "changed-immutable-field"
	contentname := "snapcontent1"
	volumeSnapshotClassName := "volume-snapshot-class-1"
	emptyVolumeSnapshotClassName := ""
	invalidErrorMsg := fmt.Sprintf("only one of Spec.Source.PersistentVolumeClaimName = %s and Spec.Source.VolumeSnapshotContentName = %s should be set", pvcname, contentname)

	testCases := []struct {
		name              string
		volumeSnapshot    *volumesnapshotv1beta1.VolumeSnapshot
		oldVolumeSnapshot *volumesnapshotv1beta1.VolumeSnapshot
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
			name: "Create: old is nil and new is invalid",
			volumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
						PersistentVolumeClaimName: &pvcname,
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			oldVolumeSnapshot: nil,
			shouldAdmit:       false,
			operation:         v1.Create,
			msg:               invalidErrorMsg,
		},
		{
			name: "Create: old is nil and new is valid",
			volumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
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
			volumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
					VolumeSnapshotClassName: &emptyVolumeSnapshotClassName,
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
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
			volumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
					VolumeSnapshotClassName: &volumeSnapshotClassName,
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: true,
			operation:   v1.Update,
		},
		{
			name: "Update: old is valid and new is valid but changes immutable field spec.source",
			volumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &mutatedField,
					},
					VolumeSnapshotClassName: &volumeSnapshotClassName,
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
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
			volumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
						PersistentVolumeClaimName: &pvcname,
						VolumeSnapshotContentName: &contentname,
					},
				},
			},
			shouldAdmit: true,
			operation:   v1.Update,
		},
		{
			name: "Update: old is invalid and new is invalid",
			volumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
						VolumeSnapshotContentName: &contentname,
						PersistentVolumeClaimName: &pvcname,
					},
				},
			},
			oldVolumeSnapshot: &volumesnapshotv1beta1.VolumeSnapshot{
				Spec: volumesnapshotv1beta1.VolumeSnapshotSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotSource{
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
					Resource:  SnapshotV1Beta1GVR,
					Operation: tc.operation,
				},
			}
			response := admitSnapshot(review)
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
func TestAdmitVolumeSnapshotContent(t *testing.T) {
	volumeHandle := "volumeHandle1"
	modifiedField := "modified-field"
	snapshotHandle := "snapshotHandle1"
	volumeSnapshotClassName := "volume-snapshot-class-1"
	validContent := &volumesnapshotv1beta1.VolumeSnapshotContent{
		Spec: volumesnapshotv1beta1.VolumeSnapshotContentSpec{
			Source: volumesnapshotv1beta1.VolumeSnapshotContentSource{
				SnapshotHandle: &snapshotHandle,
			},
			VolumeSnapshotRef: core_v1.ObjectReference{
				Name:      "snapshot-ref",
				Namespace: "default-ns",
			},
			VolumeSnapshotClassName: &volumeSnapshotClassName,
		},
	}
	invalidContent := &volumesnapshotv1beta1.VolumeSnapshotContent{
		Spec: volumesnapshotv1beta1.VolumeSnapshotContentSpec{
			Source: volumesnapshotv1beta1.VolumeSnapshotContentSource{
				SnapshotHandle: &snapshotHandle,
				VolumeHandle:   &volumeHandle,
			},
			VolumeSnapshotRef: core_v1.ObjectReference{
				Name:      "",
				Namespace: "default-ns",
			},
		},
	}
	invalidErrorMsg := fmt.Sprintf("only one of Spec.Source.VolumeHandle = %s and Spec.Source.SnapshotHandle = %s should be set", volumeHandle, snapshotHandle)
	testCases := []struct {
		name                     string
		volumeSnapshotContent    *volumesnapshotv1beta1.VolumeSnapshotContent
		oldVolumeSnapshotContent *volumesnapshotv1beta1.VolumeSnapshotContent
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
			name:                     "Create: old is nil and new is invalid",
			volumeSnapshotContent:    invalidContent,
			oldVolumeSnapshotContent: nil,
			shouldAdmit:              false,
			operation:                v1.Create,
			msg:                      invalidErrorMsg,
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
			volumeSnapshotContent: &volumesnapshotv1beta1.VolumeSnapshotContent{
				Spec: volumesnapshotv1beta1.VolumeSnapshotContentSpec{
					Source: volumesnapshotv1beta1.VolumeSnapshotContentSource{
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
			shouldAdmit:              true,
			operation:                v1.Update,
		},
		{
			name:                     "Update: old is invalid and new is invalid",
			volumeSnapshotContent:    invalidContent,
			oldVolumeSnapshotContent: invalidContent,
			shouldAdmit:              true,
			operation:                v1.Update,
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
					Resource:  SnapshotContentV1Beta1GVR,
					Operation: tc.operation,
				},
			}
			response := admitSnapshot(review)
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
