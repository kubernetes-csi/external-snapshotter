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
	"errors"

	v1 "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"

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
			// defualt snapshot class name should be set
			name:                  "1-1 - default snapshot class name should be set",
			initialContents:       nocontents,
			initialSnapshots:      newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "content1-1", "", "content1-1", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots:     newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "content1-1", defaultClass, "content1-1", &True, nil, nil, nil, false, true, nil),
			initialClaims:         newClaimArray("claim1-1", "pvc-uid1-1", "1Gi", "volume1-1", v1.ClaimBound, &sameDriver),
			initialVolumes:        newVolumeArray("volume1-1", "pv-uid1-1", "pv-handle1-1", "1Gi", "pvc-uid1-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialStorageClasses: []*storage.StorageClass{sameDriverStorageClass},
			expectedEvents:        noevents,
			errors:                noerrors,
			test:                  testUpdateSnapshotClass,
		},
		{
			// snapshot class name already set
			name:                  "1-2 - snapshot class name already set",
			initialContents:       nocontents,
			initialSnapshots:      newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "content1-1", defaultClass, "content1-1", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots:     newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "content1-1", defaultClass, "content1-1", &True, nil, nil, nil, false, true, nil),
			initialClaims:         newClaimArray("claim1-1", "pvc-uid1-1", "1Gi", "volume1-1", v1.ClaimBound, &sameDriver),
			initialVolumes:        newVolumeArray("volume1-1", "pv-uid1-1", "pv-handle1-1", "1Gi", "pvc-uid1-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialStorageClasses: []*storage.StorageClass{sameDriverStorageClass},
			expectedEvents:        noevents,
			errors:                noerrors,
			test:                  testUpdateSnapshotClass,
		},
		{
			// default snapshot class not found
			name:                  "1-3 - snapshot class name not found",
			initialContents:       nocontents,
			initialSnapshots:      newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "content1-1", "missing-class", "content1-1", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots:     newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "content1-1", "missing-class", "content1-1", &False, nil, nil, newVolumeError("Failed to get snapshot class with error failed to retrieve snapshot class missing-class from the informer: \"volumesnapshotclass.snapshot.storage.k8s.io \\\"missing-class\\\" not found\""), false, true, nil),
			initialClaims:         newClaimArray("claim1-1", "pvc-uid1-1", "1Gi", "volume1-1", v1.ClaimBound, &sameDriver),
			initialVolumes:        newVolumeArray("volume1-1", "pv-uid1-1", "pv-handle1-1", "1Gi", "pvc-uid1-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialStorageClasses: []*storage.StorageClass{sameDriverStorageClass},
			expectedEvents:        []string{"Warning GetSnapshotClassFailed"},
			errors:                noerrors,
			test:                  testUpdateSnapshotClass,
		},
		{
			// failed to get snapshot class from name
			name:                  "1-4 - snapshot update with default class name failed because storageclass not found",
			initialContents:       nocontents,
			initialSnapshots:      newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "content1-1", "", "content1-1", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots:     newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "content1-1", "", "content1-1", &False, nil, nil, newVolumeError("Failed to set default snapshot class with error mock update error"), false, true, nil),
			initialClaims:         newClaimArray("claim1-1", "pvc-uid1-1", "1Gi", "volume1-1", v1.ClaimBound, &sameDriver),
			initialVolumes:        newVolumeArray("volume1-1", "pv-uid1-1", "pv-handle1-1", "1Gi", "pvc-uid1-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialStorageClasses: []*storage.StorageClass{sameDriverStorageClass},
			expectedEvents:        []string{"Warning SetDefaultSnapshotClassFailed"},
			errors: []reactorError{
				{"get", "storageclasses", errors.New("mock update error")},
			},
			test: testUpdateSnapshotClass,
		},
	}

	runUpdateSnapshotClassTests(t, tests, snapshotClasses)
}
