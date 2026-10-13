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
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotfake "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	snapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/metrics"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientfeatures "k8s.io/client-go/features"
	coreinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
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
	featureGates, ok := clientfeatures.FeatureGates().(interface {
		clientfeatures.Gates
		Set(clientfeatures.Feature, bool) error
	})
	if !ok {
		t.Fatal("client-go feature gates cannot be changed for the fake informer test")
	}
	watchListEnabled := featureGates.Enabled(clientfeatures.WatchListClient)
	if err := featureGates.Set(clientfeatures.WatchListClient, false); err != nil {
		t.Fatalf("disable WatchListClient for fake informers: %v", err)
	}
	t.Cleanup(func() {
		if err := featureGates.Set(clientfeatures.WatchListClient, watchListEnabled); err != nil {
			t.Errorf("restore WatchListClient: %v", err)
		}
	})

	const (
		claimName = "shared-claim"
		workers   = 10
	)
	pvc := newClaim(claimName, "claim-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty, true)

	deletionTimestamp := metav1.Now()
	ready := false
	snapshots := make([]*crdv1.VolumeSnapshot, 0, snapshotCount)
	snapshotClass := newSnapshotClass(classSilver, "class-uid", mockDriverName, false)
	snapshotClass.Namespace = ""
	snapshotObjects := []runtime.Object{snapshotClass}
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

	// Removing the last finalizer from an object with a deletion timestamp makes
	// the API server delete it. Hold those delete watch events until all workers
	// have completed their updates, then deliver the events through the informer.
	finalizersRemoved := make(chan string, snapshotCount)
	snapshotClient.Fake.PrependReactor("update", "volumesnapshots", func(action clienttesting.Action) (bool, runtime.Object, error) {
		updatedSnapshot := action.(clienttesting.UpdateAction).GetObject().(*crdv1.VolumeSnapshot)
		if updatedSnapshot.DeletionTimestamp == nil || slices.Contains(updatedSnapshot.Finalizers, utils.VolumeSnapshotAsSourceFinalizer) {
			return false, nil, nil
		}
		finalizersRemoved <- updatedSnapshot.Name
		return true, updatedSnapshot.DeepCopy(), nil
	})

	snapshotInformerFactory := snapshotinformers.NewSharedInformerFactory(snapshotClient, utils.NoResyncPeriodFunc())
	coreInformerFactory := coreinformers.NewSharedInformerFactory(kubeClient, utils.NoResyncPeriodFunc())
	rateLimiter := func() workqueue.TypedRateLimiter[string] {
		return workqueue.NewTypedItemExponentialFailureRateLimiter[string](time.Millisecond, time.Minute)
	}
	ctrl := NewCSISnapshotCommonController(
		snapshotClient,
		kubeClient,
		snapshotInformerFactory.Snapshot().V1().VolumeSnapshots(),
		snapshotInformerFactory.Snapshot().V1().VolumeSnapshotContents(),
		snapshotInformerFactory.Snapshot().V1().VolumeSnapshotClasses(),
		snapshotInformerFactory.Groupsnapshot().V1().VolumeGroupSnapshots(),
		snapshotInformerFactory.Groupsnapshot().V1().VolumeGroupSnapshotContents(),
		snapshotInformerFactory.Groupsnapshot().V1().VolumeGroupSnapshotClasses(),
		coreInformerFactory.Core().V1().PersistentVolumeClaims(),
		coreInformerFactory.Core().V1().PersistentVolumes(),
		nil,
		metrics.NewMetricsManager(),
		utils.NoResyncPeriodFunc(),
		rateLimiter(),
		rateLimiter(),
		rateLimiter(),
		rateLimiter(),
		false,
		false,
		false,
	)
	ctrl.eventRecorder = record.NewFakeRecorder(snapshotCount)

	stopCh := make(chan struct{})
	snapshotInformerFactory.Start(stopCh)
	coreInformerFactory.Start(stopCh)
	var workerWG sync.WaitGroup
	controllerStopped := make(chan struct{})
	go func() {
		defer close(controllerStopped)
		ctrl.Run(workers, stopCh, &workerWG)
	}()
	t.Cleanup(func() {
		close(stopCh)
		<-controllerStopped
		workerWG.Wait()
	})

	removedSnapshots := make(map[string]struct{}, snapshotCount)
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for len(removedSnapshots) < snapshotCount {
		select {
		case snapshotName := <-finalizersRemoved:
			removedSnapshots[snapshotName] = struct{}{}
		case <-timer.C:
			t.Fatalf("timed out after %d of %d snapshot finalizers were removed", len(removedSnapshots), snapshotCount)
		}
	}

	for _, snapshot := range snapshots {
		if err := snapshotClient.SnapshotV1().VolumeSnapshots(testNamespace).Delete(
			context.Background(), snapshot.Name, metav1.DeleteOptions{}); err != nil {
			t.Fatalf("deliver deletion for snapshot %s: %v", snapshot.Name, err)
		}
	}
	if err := wait.PollUntilContextTimeout(context.Background(), 10*time.Millisecond, 10*time.Second, true,
		func(context.Context) (bool, error) {
			cachedSnapshots, err := ctrl.snapshotLister.VolumeSnapshots(testNamespace).List(labels.Everything())
			return err == nil && len(cachedSnapshots) == 0 && len(ctrl.snapshotStore.List()) == 0, err
		}); err != nil {
		t.Fatalf("wait for snapshot deletion events: %v", err)
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
