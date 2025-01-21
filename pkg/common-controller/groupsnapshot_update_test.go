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

	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

func TestUpdateGroupSnapshotSync(t *testing.T) {
	tests := []controllerTest{
		{
			name: "4-1 - successful pre-provisioned group snapshot but not ready, no-op",
			initialGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClass(classGold).
				WithTargetContentName("groupsnapcontent-snapuid1-1").
				WithStatusReadyToUse(false).
				WithStatusBoundContentName("groupsnapcontent-snapuid1-1").
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			expectedGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClass(classGold).
				WithTargetContentName("groupsnapcontent-snapuid1-1").
				WithStatusReadyToUse(false).
				WithStatusBoundContentName("groupsnapcontent-snapuid1-1").
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			initialGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-snapuid1-1").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithDeletionPolicy(deletionPolicy).
				WithGroupSnapshotClassName(classGold).
				WithTargetGroupSnapshotHandle("group-snapshot-handle").
				BuildArray(),
			expectedGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-snapuid1-1").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithDeletionPolicy(deletionPolicy).
				WithGroupSnapshotClassName(classGold).
				WithTargetGroupSnapshotHandle("group-snapshot-handle").
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
			name: "4-2 - successful dynamically-provisioned create group snapshot with group snapshot class gold but not ready, no-op",
			initialGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClass(classGold).
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithStatusReadyToUse(false).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			expectedGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClass(classGold).
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithStatusReadyToUse(false).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			initialGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-group-snapuid1-1").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithDeletionPolicy(deletionPolicy).
				WithGroupSnapshotClassName(classGold).
				WithDesiredVolumeHandles("1-pv-handle6-1", "2-pv-handle6-1").
				BuildArray(),
			expectedGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-group-snapuid1-1").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithDeletionPolicy(deletionPolicy).
				WithGroupSnapshotClassName(classGold).
				WithDesiredVolumeHandles("1-pv-handle6-1", "2-pv-handle6-1").
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
			name: "4-3 - group snapshot, setting it ready",
			initialGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClass(classGold).
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithStatusReadyToUse(false).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			expectedGroupSnapshots: NewVolumeGroupSnapshotBuilder("group-snap-1-1", "group-snapuid1-1").
				WithGroupSnapshotClass(classGold).
				WithSelectors(map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}).
				WithStatusReadyToUse(true).
				WithStatusBoundContentName("groupsnapcontent-group-snapuid1-1").
				WithFinalizers(utils.VolumeGroupSnapshotBoundFinalizer).
				BuildArray(),
			initialGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-group-snapuid1-1").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithDeletionPolicy(deletionPolicy).
				WithGroupSnapshotClassName(classGold).
				WithDesiredVolumeHandles("1-pv-handle6-1", "2-pv-handle6-1").
				WithStatus().
				BuildArray(),
			expectedGroupContents: NewVolumeGroupSnapshotContentBuilder("groupsnapcontent-group-snapuid1-1").
				WithBoundGroupSnapshot("group-snap-1-1", "group-snapuid1-1").
				WithDeletionPolicy(deletionPolicy).
				WithGroupSnapshotClassName(classGold).
				WithDesiredVolumeHandles("1-pv-handle6-1", "2-pv-handle6-1").
				WithStatus().
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
	}
	runSyncTests(t, tests, nil, groupSnapshotClasses)
}
