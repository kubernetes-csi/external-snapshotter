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
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-1-1", "group-snapuid1-1", nil,
					"groupsnapcontent-snapuid1-1", classGold, "groupsnapcontent-snapuid1-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-1-1", "group-snapuid1-1", nil,
					"groupsnapcontent-snapuid1-1", classGold, "groupsnapcontent-snapuid1-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "", classGold, nil,
				"group-snapshot-handle", deletionPolicy, nil, false, false,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "", classGold, nil,
				"group-snapshot-handle", deletionPolicy, nil, false, false,
			),
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
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-1-1", "group-snapuid1-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid1-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-1-1", "group-snapuid1-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid1-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "snapshot-handle", classGold, []string{
					"1-pv-handle6-1",
					"2-pv-handle6-1",
				}, "", deletionPolicy, nil, false, false,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "snapshot-handle", classGold, []string{
					"1-pv-handle6-1",
					"2-pv-handle6-1",
				}, "", deletionPolicy, nil, false, false,
			),
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
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-1-1", "group-snapuid1-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid1-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-1-1", "group-snapuid1-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid1-1", &True, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "snapshot-handle", classGold, []string{
					"1-pv-handle6-1",
					"2-pv-handle6-1",
				}, "", deletionPolicy, nil, false, true,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "snapshot-handle", classGold, []string{
					"1-pv-handle6-1",
					"2-pv-handle6-1",
				}, "", deletionPolicy, nil, false, true,
			),
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
