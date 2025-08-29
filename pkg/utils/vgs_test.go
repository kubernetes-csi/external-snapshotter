/*
Copyright 2024 The Kubernetes Authors.

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

package utils

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
)

func TestIsVolumeSnapshotGroupMember(t *testing.T) {
	testCases := []struct {
		name                     string
		snapshot                 *crdv1.VolumeSnapshot
		expected                 bool
		expectedParentObjectName string
		expectedIndexKey         string
	}{
		{
			name:                     "nil snapshot",
			snapshot:                 nil,
			expected:                 false,
			expectedParentObjectName: "",
			expectedIndexKey:         "",
		},
		{
			name:                     "without ownership",
			snapshot:                 &crdv1.VolumeSnapshot{},
			expected:                 false,
			expectedParentObjectName: "",
			expectedIndexKey:         "",
		},
		{
			name: "with a different ownership",
			snapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Name:       "test",
							Kind:       "Deployment",
						},
					},
				},
			},
			expected:                 false,
			expectedParentObjectName: "",
			expectedIndexKey:         "",
		},
		{
			name: "with wrong ownership",
			snapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "test/v1beta2",
							Kind:       "VolumeGroupSnapshot",
							Name:       "vgs",
						},
					},
				},
			},
			expected:                 false,
			expectedParentObjectName: "",
			expectedIndexKey:         "",
		},
		{
			name: "with correct ownership",
			snapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vs",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "groupsnapshot.storage.k8s.io/v1beta2",
							Kind:       "VolumeGroupSnapshot",
							Name:       "vgs",
						},
					},
				},
			},
			expected:                 true,
			expectedParentObjectName: "vgs",
			expectedIndexKey:         "default^vgs",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := IsVolumeGroupSnapshotMember(test.snapshot)
			if result != test.expected {
				t.Errorf("IsVolumeSnapshotGroupMember(s) = %v WANT %v", result, test.expected)
			}

			resultParentObject := getVolumeGroupSnapshotParentObjectName(test.snapshot)
			if resultParentObject != test.expectedParentObjectName {
				t.Errorf("GetVolumeGroupSnapshotParentObjectName(s) = %v WANTED %v", resultParentObject, test.expectedParentObjectName)
			}

			resultKey := VolumeSnapshotParentGroupKeyFunc(test.snapshot)
			if resultKey != test.expectedIndexKey {
				t.Errorf("VolumeSnapshotParentGroupKeyFunc(s) = %v WANTED %v", resultKey, test.expectedIndexKey)
			}
		})
	}
}

func TestNeedToAddVolumeGroupSnapshotOwnership(t *testing.T) {
	testCases := []struct {
		name     string
		snapshot *crdv1.VolumeSnapshot
		expected bool
	}{
		{
			name:     "nil snapshot",
			snapshot: nil,
			expected: false,
		},
		{
			name: "snapshot with nil status",
			snapshot: &crdv1.VolumeSnapshot{
				Status: nil,
			},
			expected: false,
		},
		{
			name: "independent snapshot",
			snapshot: &crdv1.VolumeSnapshot{
				Status: &crdv1.VolumeSnapshotStatus{
					VolumeGroupSnapshotName: nil,
				},
			},
			expected: false,
		},
		{
			name: "snapshot with empty volume group snapshot name",
			snapshot: &crdv1.VolumeSnapshot{
				Status: &crdv1.VolumeSnapshotStatus{
					VolumeGroupSnapshotName: ptr.To(""),
				},
			},
			expected: false,
		},
		{
			name: "snapshot with empty ownership metadata",
			snapshot: &crdv1.VolumeSnapshot{
				Status: &crdv1.VolumeSnapshotStatus{
					VolumeGroupSnapshotName: ptr.To("vgs"),
				},
			},
			expected: true,
		},
		{
			name: "snapshot missing the group ownership but having other ownerships",
			snapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "v1",
							Kind:       "Deployment",
							Name:       "test",
						},
					},
				},
				Status: &crdv1.VolumeSnapshotStatus{
					VolumeGroupSnapshotName: ptr.To("vgs"),
				},
			},
			expected: true,
		},
		{
			name: "snapshot with the ownership already set",
			snapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "groupsnapshot.storage.k8s.io/v1beta2",
							Kind:       "VolumeGroupSnapshot",
							Name:       "vgs",
						},
					},
				},
				Status: &crdv1.VolumeSnapshotStatus{
					VolumeGroupSnapshotName: ptr.To("vgs"),
				},
			},
			expected: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := NeedToAddVolumeGroupSnapshotOwnership(test.snapshot)
			if result != test.expected {
				t.Errorf("NeedToAddVolumeGroupSnapshotOwnership(s) = %v WANT %v", result, test.expected)
			}
		})
	}
}
