/*
Copyright 2019 The Kubernetes Authors.

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
	"github.com/kubernetes-csi/external-snapshotter/v7/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"testing"
)

// Test single call to ensurePVCFinalizer, checkandRemovePVCFinalizer, addSnapshotFinalizer, removeSnapshotFinalizer
// expecting finalizers to be added or removed
func TestSnapshotFinalizer(t *testing.T) {
	tests := []controllerTest{
		{
			name:             "1-1 - successful add PVC finalizer",
			initialSnapshots: newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
			initialClaims:    newClaimArray("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			test:             testAddPVCFinalizer,
			expectSuccess:    true,
		},
		{
			name:             "1-2 - won't add PVC finalizer; already added",
			initialSnapshots: newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
			initialClaims:    newClaimArrayFinalizer("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			test:             testAddPVCFinalizer,
			expectSuccess:    false,
		},
		{
			name:             "1-3 - successful remove PVC finalizer",
			initialSnapshots: newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
			initialClaims:    newClaimArrayFinalizer("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			test:             testRemovePVCFinalizer,
			expectSuccess:    true,
		},
		{
			name:             "1-4 - won't remove PVC finalizer; already removed",
			initialSnapshots: newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
			initialClaims:    newClaimArray("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			test:             testRemovePVCFinalizer,
			expectSuccess:    false,
		},
		{
			name:             "1-5 - won't remove PVC finalizer; PVC in-use",
			initialSnapshots: newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
			initialClaims:    newClaimArray("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			test:             testRemovePVCFinalizer,
			expectSuccess:    false,
		},
		{
			name:             "2-1 - successful add Snapshot finalizer",
			initialSnapshots: newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", classSilver, "", &False, nil, nil, nil, false, false, nil),
			initialClaims:    newClaimArray("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			test:             testAddSnapshotFinalizer,
			expectSuccess:    true,
		},
		{
			name:             "2-2 - successful add single Snapshot finalizer with patch",
			initialSnapshots: withSnapshotFinalizers(newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", classSilver, "", &False, nil, nil, nil, false, false, nil), utils.VolumeSnapshotBoundFinalizer),
			initialClaims:    newClaimArray("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			test:             testAddSingleSnapshotFinalizer,
			expectSuccess:    true,
		},
		{
			name:             "2-3 - successful remove Snapshot finalizer",
			initialSnapshots: newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
			initialClaims:    newClaimArray("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			test:             testRemoveSnapshotFinalizer,
			expectSuccess:    true,
		},
	}
	runFinalizerTests(t, tests, snapshotClasses)
}
