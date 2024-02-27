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
	"testing"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
		// TODO: Handle conflict errors, currently failing
		// {
		// 	name:             "2-4 - successful remove Snapshot finalizer after update conflict",
		// 	initialSnapshots: newSnapshotArray("snap2-4", "snapuid2-4", "claim2-4", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
		// 	initialClaims:    newClaimArray("claim2-4", "pvc-uid2-4", "1Gi", "volume2-4", v1.ClaimBound, &classEmpty),
		// 	test:             testRemoveSnapshotFinalizerAfterUpdateConflict,
		// 	expectSuccess:    true,
		// 	errors: []reactorError{
		// 		{"update", "volumesnapshots", errors.NewConflict(crdv1.Resource("volumesnapshots"), "snap2-4", nil)},
		// 	},
		// },
		{
			name:             "2-5 - unsuccessful remove Snapshot finalizer after update non-conflict error",
			initialSnapshots: newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
			initialClaims:    newClaimArray("claim2-5", "pvc-uid2-5", "1Gi", "volume2-5", v1.ClaimBound, &classEmpty),
			test:             testRemoveSnapshotFinalizerAfterUpdateConflict,
			expectSuccess:    false,
			errors: []reactorError{
				{"update", "volumesnapshots", errors.NewForbidden(crdv1.Resource("volumesnapshots"), "snap2-5", nil)},
			},
		},
		{
			name:             "2-6 - unsuccessful remove Snapshot finalizer after update conflict and get fails",
			initialSnapshots: newSnapshotArray("snap2-6", "snapuid2-6", "claim2-6", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
			initialClaims:    newClaimArray("claim2-6", "pvc-uid2-6", "1Gi", "volume2-6", v1.ClaimBound, &classEmpty),
			test:             testRemoveSnapshotFinalizerAfterUpdateConflict,
			expectSuccess:    false,
			errors: []reactorError{
				{"update", "volumesnapshots", errors.NewConflict(crdv1.Resource("volumesnapshots"), "snap2-6", nil)},
				{"get", "volumesnapshots", errors.NewServerTimeout(crdv1.Resource("volumesnapshots"), "get", 10)},
			},
		},
	}
	runFinalizerTests(t, tests, snapshotClasses)
}
