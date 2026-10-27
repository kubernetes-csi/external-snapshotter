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
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotfake "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
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

func TestSingleSnapshotDeletionRemovesPVCFinalizer(t *testing.T) {
	testSnapshotDeletionRemovesPVCFinalizer(t, 1)
}

func TestTwentyConcurrentSnapshotDeletionsRemovePVCFinalizer(t *testing.T) {
	testSnapshotDeletionRemovesPVCFinalizer(t, 20)
}

func testSnapshotDeletionRemovesPVCFinalizer(t *testing.T, snapshotCount int) {
	t.Helper()

	const claimName = "shared-claim"
	pvc := newClaim(claimName, "claim-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty, true)

	deletionTimestamp := metav1.Now()
	ready := false
	snapshots := make([]*crdv1.VolumeSnapshot, 0, snapshotCount)
	snapshotObjects := make([]runtime.Object, 0, snapshotCount)
	for i := 0; i < snapshotCount; i++ {
		snapshot := newSnapshot(
			fmt.Sprintf("snapshot-%d", i),
			fmt.Sprintf("snapshot-uid-%d", i),
			claimName,
			"",
			classSilver,
			"",
			&ready,
			nil,
			nil,
			nil,
			false,
			false,
			&deletionTimestamp,
		)
		snapshot.Finalizers = []string{utils.VolumeSnapshotAsSourceFinalizer}
		snapshots = append(snapshots, snapshot)
		snapshotObjects = append(snapshotObjects, snapshot)
	}

	snapshotClient := snapshotfake.NewSimpleClientset(snapshotObjects...)
	kubeClient := kubefake.NewSimpleClientset(pvc)

	indexers := cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}
	snapshotIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, indexers)
	pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, indexers)
	if err := pvcIndexer.Add(pvc); err != nil {
		t.Fatalf("add PVC to indexer: %v", err)
	}

	ctrl := &csiSnapshotCommonController{
		clientset:      snapshotClient,
		client:         kubeClient,
		snapshotStore:  cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		snapshotLister: storagelisters.NewVolumeSnapshotLister(snapshotIndexer),
		pvcLister:      corelisters.NewPersistentVolumeClaimLister(pvcIndexer),
	}
	for _, snapshot := range snapshots {
		if err := snapshotIndexer.Add(snapshot); err != nil {
			t.Fatalf("add snapshot to indexer: %v", err)
		}
		if err := ctrl.snapshotStore.Add(snapshot); err != nil {
			t.Fatalf("add snapshot to controller store: %v", err)
		}
	}

	// Model one worker per deleting snapshot. The informer cache intentionally
	// retains all snapshots until every worker has made its finalizer decision,
	// which is a valid ordering when a namespace deletion removes them together.
	var wg sync.WaitGroup
	errors := make(chan error, snapshotCount)
	for _, snapshot := range snapshots {
		wg.Add(1)
		go func(snapshot *crdv1.VolumeSnapshot) {
			defer wg.Done()
			if err := ctrl.removeSnapshotFinalizer(snapshot, true, false, false); err != nil {
				errors <- err
			}
		}(snapshot)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("remove snapshot finalizer: %v", err)
	}
	for _, snapshot := range snapshots {
		updatedSnapshot, err := snapshotClient.SnapshotV1().VolumeSnapshots(testNamespace).Get(
			context.Background(), snapshot.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get snapshot %s: %v", snapshot.Name, err)
		}
		if slices.Contains(updatedSnapshot.Finalizers, utils.VolumeSnapshotAsSourceFinalizer) {
			t.Fatalf("snapshot %s still has source finalizer", snapshot.Name)
		}
	}

	// Deliver the informer deletions after the API server accepted all
	// finalizer removals. No VolumeSnapshot remains to trigger PVC cleanup.
	for _, snapshot := range snapshots {
		if err := snapshotIndexer.Delete(snapshot); err != nil {
			t.Fatalf("delete snapshot from indexer: %v", err)
		}
	}

	updatedPVC, err := kubeClient.CoreV1().PersistentVolumeClaims(testNamespace).Get(
		context.Background(), claimName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PVC: %v", err)
	}
	if slices.Contains(updatedPVC.Finalizers, utils.PVCFinalizer) {
		t.Errorf("PVC %s still has %s after all %d snapshots were deleted concurrently",
			claimName, utils.PVCFinalizer, snapshotCount)
	}
}
