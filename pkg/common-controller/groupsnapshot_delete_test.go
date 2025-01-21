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

package common_controller

import (
	"testing"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

func TestDeleteGroupSnapshotSync(t *testing.T) {
	tests := []controllerTest{
		{
			name: "2-1 - group snapshot have been deleted, but no content was present - no op",
			initialGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithGroupSnapshotClass(classGold).
				WithDeletionTimestamp(timeNowMetav1).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithStatusReadyToUse(false).
				BuildArray(),
			expectedGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithGroupSnapshotClass(classGold).
				WithDeletionTimestamp(timeNowMetav1).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithStatusReadyToUse(false).
				BuildArray(),
			initialGroupContents:  nil,
			expectedGroupContents: nil,
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  true,
		},
		{
			name: "2-2 - dynamic group snapshot have been deleted, was not provisioned, no-op",
			initialGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithGroupSnapshotClass(classGold).
				WithDeletionTimestamp(timeNowMetav1).
				WithNilStatus().
				BuildArray(),
			expectedGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithGroupSnapshotClass(classGold).
				WithDeletionTimestamp(timeNowMetav1).
				WithNilStatus().
				BuildArray(),
			initialGroupContents:  nil,
			expectedGroupContents: nil,
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: nil,
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  true,
		},
		{
			name: "2-3 - pre-provisioned group snapshot have been deleted, retention policy set to retain - set 'being deleted' annotation",
			initialGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithGroupSnapshotClass(classGold).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithDeletionTimestamp(timeNowMetav1).
				WithStatusReadyToUse(false).
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			expectedGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithGroupSnapshotClass(classGold).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithDeletionTimestamp(timeNowMetav1).
				WithStatusReadyToUse(false).
				BuildArray(),
			initialGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-group-snapuid1-1").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClassName(classGold).
				WithDesiredVolumeHandles("1-pv-handle6-1", "2-pv-handle6-1").
				WithDeletionPolicy(crdv1.VolumeSnapshotContentRetain).
				BuildArray(),
			expectedGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-group-snapuid1-1").
				WithAnnotation(utils.AnnVolumeGroupSnapshotBeingDeleted, "yes").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClassName(classGold).
				WithDesiredVolumeHandles("1-pv-handle6-1", "2-pv-handle6-1").
				WithDeletionPolicy(crdv1.VolumeSnapshotContentRetain).
				BuildArray(),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  true,
		},
		{
			name: "2-4 - pre-provisioned snapshot have been deleted, retention policy set to delete - volume snapshot content will be deleted",
			initialGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithGroupSnapshotClass(classGold).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithDeletionTimestamp(timeNowMetav1).
				WithStatusReadyToUse(false).
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			expectedGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithGroupSnapshotClass(classGold).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithDeletionTimestamp(timeNowMetav1).
				WithStatusReadyToUse(false).
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			initialGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-group-snapuid1-1").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClassName(classGold).
				WithDesiredVolumeHandles("1-pv-handle6-1", "2-pv-handle6-1").
				WithDeletionPolicy(crdv1.VolumeSnapshotContentDelete).
				BuildArray(),
			expectedGroupContents: nil,
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  true,
		},
	}
	runSyncTests(t, tests, nil, groupSnapshotClasses)
}
