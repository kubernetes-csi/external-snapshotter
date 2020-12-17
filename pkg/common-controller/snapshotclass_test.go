/*
Copyright 2018 The Kubernetes Authors.

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
	v1 "k8s.io/api/core/v1"
	"testing"
)

// Test single call to checkAndUpdateSnapshotClass.
// 1. Fill in the controller with initial data
// 2. Call the tested function checkAndUpdateSnapshotClass via
//    controllerTest.testCall *once*.
// 3. Compare resulting snapshotclass.
func TestUpdateSnapshotClass(t *testing.T) {
	tests := []controllerTest{
		{
			// default snapshot class name should be set
			name:              "1-1 - default snapshot class name should be set",
			initialContents:   nocontents,
			initialSnapshots:  newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "", "", "", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "", defaultClass, "", &True, nil, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim1-1", "pvc-uid1-1", "1Gi", "volume1-1", v1.ClaimBound, &sameDriver),
			initialVolumes:    newVolumeArray("volume1-1", "pv-uid1-1", "pv-handle1-1", "1Gi", "pvc-uid1-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, sameDriver),
			expectedEvents:    noevents,
			errors:            noerrors,
			test:              testUpdateSnapshotClass,
		},
		{
			// snapshot class name already set
			name:              "1-2 - snapshot class name already set",
			initialContents:   nocontents,
			initialSnapshots:  newSnapshotArray("snap1-2", "snapuid1-2", "claim1-2", "", defaultClass, "", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap1-2", "snapuid1-2", "claim1-2", "", defaultClass, "", &True, nil, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim1-2", "pvc-uid1-2", "1Gi", "volume1-2", v1.ClaimBound, &sameDriver),
			initialVolumes:    newVolumeArray("volume1-2", "pv-uid1-2", "pv-handle1-2", "1Gi", "pvc-uid1-2", "claim1-2", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, sameDriver),
			expectedEvents:    noevents,
			errors:            noerrors,
			test:              testUpdateSnapshotClass,
		},
		{
			// default snapshot class not found
			name:              "1-3 - snapshot class name not found",
			initialContents:   nocontents,
			initialSnapshots:  newSnapshotArray("snap1-3", "snapuid1-3", "claim1-3", "", "missing-class", "", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap1-3", "snapuid1-3", "claim1-3", "", "missing-class", "", &True, nil, nil, newVolumeError("Failed to get snapshot class with error volumesnapshotclass.snapshot.storage.k8s.io \"missing-class\" not found"), false, true, nil),
			initialClaims:     newClaimArray("claim1-3", "pvc-uid1-3", "1Gi", "volume1-3", v1.ClaimBound, &sameDriver),
			initialVolumes:    newVolumeArray("volume1-3", "pv-uid1-3", "pv-handle1-3", "1Gi", "pvc-uid1-3", "claim1-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, sameDriver),
			expectedEvents:    []string{"Warning GetSnapshotClassFailed"},
			errors:            noerrors,
			test:              testUpdateSnapshotClass,
		},
		{
			// PVC does not exist
			name:              "1-5 - snapshot update with default class name failed because PVC was not found",
			initialContents:   nocontents,
			initialSnapshots:  newSnapshotArray("snap1-5", "snapuid1-5", "claim1-5", "", "", "", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap1-5", "snapuid1-5", "claim1-5", "", "", "", &True, nil, nil, newVolumeError("Failed to set default snapshot class with error failed to retrieve PVC claim1-5 from the lister: \"persistentvolumeclaim \\\"claim1-5\\\" not found\""), false, true, nil),
			initialClaims:     nil,
			initialVolumes:    nil,
			expectedEvents:    []string{"Warning SetDefaultSnapshotClassFailed"},
			errors:            noerrors,
			test:              testUpdateSnapshotClass,
		},
	}

	runUpdateSnapshotClassTests(t, tests, snapshotClasses)
}
