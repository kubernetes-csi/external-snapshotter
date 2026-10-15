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
	versioned "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	informers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	core "k8s.io/client-go/testing"
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

func TestPVCFinalizerRemovedAfterSnapshotDeletion(t *testing.T) {
	tests := []controllerTest{
		{
			name: "single_snapshot_current_caches",
			initialSnapshots: withSnapshotFinalizers(
				newSnapshotArray("snapshot-1", "snapshot-uid-1", "source", "", classSilver, "", &False, nil, nil, nil, false, false, nil),
				utils.VolumeSnapshotAsSourceFinalizer,
			),
			initialClaims:     newClaimArray("source", "pvc-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty),
			expectedSnapshots: nosnapshots,
			expectSuccess:     true,
			test:              testPVCFinalizerRemovalAfterDeletion(false, false),
		},
		{
			name: "single_snapshot_delayed_pvc_update",
			initialSnapshots: withSnapshotFinalizers(
				newSnapshotArray("snapshot-1", "snapshot-uid-1", "source", "", classSilver, "", &False, nil, nil, nil, false, false, nil),
				utils.VolumeSnapshotAsSourceFinalizer,
			),
			initialClaims:     newClaimArray("source", "pvc-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty),
			expectedSnapshots: nosnapshots,
			expectSuccess:     true,
			test:              testPVCFinalizerRemovalAfterDeletion(true, false),
		},
		{
			name: "two_snapshots_current_caches",
			initialSnapshots: withSnapshotFinalizers([]*crdv1.VolumeSnapshot{
				newSnapshot("snapshot-1", "snapshot-uid-1", "source", "", classSilver, "", &False, nil, nil, nil, false, false, nil),
				newSnapshot("snapshot-2", "snapshot-uid-2", "source", "", classSilver, "", &False, nil, nil, nil, false, false, nil),
			}, utils.VolumeSnapshotAsSourceFinalizer),
			initialClaims:     newClaimArray("source", "pvc-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty),
			expectedSnapshots: nosnapshots,
			expectSuccess:     true,
			test:              testPVCFinalizerRemovalAfterDeletion(false, false),
		},
		{
			name: "two_snapshots_delayed_snapshot_deletes",
			initialSnapshots: withSnapshotFinalizers([]*crdv1.VolumeSnapshot{
				newSnapshot("snapshot-1", "snapshot-uid-1", "source", "", classSilver, "", &False, nil, nil, nil, false, false, nil),
				newSnapshot("snapshot-2", "snapshot-uid-2", "source", "", classSilver, "", &False, nil, nil, nil, false, false, nil),
			}, utils.VolumeSnapshotAsSourceFinalizer),
			initialClaims:     newClaimArray("source", "pvc-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty),
			expectedSnapshots: nosnapshots,
			expectSuccess:     true,
			test:              testPVCFinalizerRemovalAfterDeletion(false, true),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runSyncTests(t, []controllerTest{test}, snapshotClasses, nil)
		})
	}
}

// testPVCFinalizerRemovalAfterDeletion controls watch delivery independently of
// the shared reactor's API storage. Both races are reproducible without workers.
func testPVCFinalizerRemovalAfterDeletion(delayPVCUpdate, delaySnapshotDelete bool) testCall {
	return func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
		ctx := context.Background()
		finalized := make(map[string]*crdv1.VolumeSnapshot, len(test.initialSnapshots))
		ctrl.clientset.(*fake.Clientset).PrependReactor("update", "volumesnapshots", func(action core.Action) (bool, runtime.Object, error) {
			handled, obj, err := reactor.React(action)
			if err != nil {
				return handled, obj, err
			}
			updated := obj.(*crdv1.VolumeSnapshot)
			if updated.DeletionTimestamp != nil && len(updated.Finalizers) == 0 {
				// Finalization removes API state before Update returns. Keep the
				// deleted object for assertions and independently delayed delivery.
				reactor.deleteSnapshotEvent(updated)
				finalized[updated.Name] = updated.DeepCopy()
			}
			return handled, obj, nil
		})
		pvc := test.initialClaims[0]
		pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
		if err := pvcIndexer.Add(pvc.DeepCopy()); err != nil {
			return err
		}
		ctrl.pvcLister = corelisters.NewPersistentVolumeClaimLister(pvcIndexer)

		// Write protection through the controller, not directly into the fixture.
		if err := ctrl.ensurePVCFinalizer(test.initialSnapshots[0]); err != nil {
			return err
		}
		protectedPVC, err := ctrl.client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !slices.Contains(protectedPVC.Finalizers, utils.PVCFinalizer) {
			return fmt.Errorf("source PVC was not protected before snapshot deletion")
		}
		if !delayPVCUpdate {
			if err := pvcIndexer.Update(protectedPVC.DeepCopy()); err != nil {
				return err
			}
		}

		// Mark every snapshot for deletion before reconciling any of them.
		// No content exists, so deletion does not need a sidecar operation.
		snapshots := make([]*crdv1.VolumeSnapshot, 0, len(test.initialSnapshots))
		for _, initialSnapshot := range test.initialSnapshots {
			snapshot := initialSnapshot.DeepCopy()
			snapshot.DeletionTimestamp = &timeNowMetav1
			snapshot, err = ctrl.clientset.SnapshotV1().VolumeSnapshots(snapshot.Namespace).Update(ctx, snapshot, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			if err := ctrl.snapshotIndexer.Add(snapshot.DeepCopy()); err != nil {
				return err
			}
			snapshots = append(snapshots, snapshot)
		}

		for i, snapshot := range snapshots {
			if err := ctrl.syncSnapshot(ctx, snapshot); err != nil {
				return err
			}
			finalizedSnapshot, found := finalized[snapshot.Name]
			if !found {
				return fmt.Errorf("snapshot %s was not finalized during reconciliation", snapshot.Name)
			}
			snapshots[i] = finalizedSnapshot
			if !delaySnapshotDelete {
				if err := ctrl.snapshotIndexer.Delete(finalizedSnapshot); err != nil {
					return err
				}
				if err := ctrl.syncSnapshotByKey(utils.SnapshotKey(snapshot)); err != nil {
					return err
				}
			}
		}

		// Deliver delayed PVC and snapshot events before checking for a leak.
		pvc, err = ctrl.client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if err := pvcIndexer.Update(pvc.DeepCopy()); err != nil {
			return err
		}
		if delaySnapshotDelete {
			for _, snapshot := range snapshots {
				if err := ctrl.snapshotIndexer.Delete(snapshot); err != nil {
					return err
				}
				if err := ctrl.syncSnapshotByKey(utils.SnapshotKey(snapshot)); err != nil {
					return err
				}
			}
		}

		// runSyncTests checks that no snapshots remain. Check the PVC through
		// the same API client as a consumer, since the runner does not compare PVCs.
		pvc, err = ctrl.client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if slices.Contains(pvc.Finalizers, utils.PVCFinalizer) {
			return fmt.Errorf("PVC %s/%s still has finalizer %q after all %d snapshots were deleted and delayed informer events were delivered", pvc.Namespace, pvc.Name, utils.PVCFinalizer, len(snapshots))
		}
		return nil
	}
}

// TestPVCFinalizerRemovedThroughSnapshotInformer exercises the registered informer
// callbacks, workqueue, and concurrent workers without starting unrelated informers.
func TestPVCFinalizerRemovedThroughSnapshotInformer(t *testing.T) {
	tests := []struct {
		name          string
		snapshotCount int
	}{
		{name: "single_snapshot", snapshotCount: 1},
		{name: "two_snapshots", snapshotCount: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), wait.ForeverTestTimeout)
			defer cancel()
			client := &fake.Clientset{}
			kubeClient := &kubefake.Clientset{}
			factory := informers.NewSharedInformerFactory(client, utils.NoResyncPeriodFunc())

			// The generated informer does not forward the fake client's WatchList
			// capability. Use the per-informer adapter instead of a global feature gate.
			informer := factory.InformerFor(&crdv1.VolumeSnapshot{}, func(client versioned.Interface, resync time.Duration) cache.SharedIndexInformer {
				listWatch := &cache.ListWatch{
					ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
						return client.SnapshotV1().VolumeSnapshots(testNamespace).List(ctx, options)
					},
					WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
						return client.SnapshotV1().VolumeSnapshots(testNamespace).Watch(ctx, options)
					},
				}
				return cache.NewSharedIndexInformer(
					cache.ToListWatcherWithWatchListSemantics(listWatch, client),
					&crdv1.VolumeSnapshot{}, resync,
					cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
				)
			})
			ctrl, err := newTestController(kubeClient, client, factory, t, controllerTest{})
			if err != nil {
				t.Fatal(err)
			}
			reactor := newSnapshotReactor(kubeClient, client, ctrl, nil, nil, nil)
			var workers sync.WaitGroup
			t.Cleanup(func() {
				ctrl.snapshotQueue.ShutDown()
				ctrl.contentQueue.ShutDown()
				ctrl.groupSnapshotQueue.ShutDown()
				ctrl.groupSnapshotContentQueue.ShutDown()
				workers.Wait()
				ctrl.eventBroadcaster.Shutdown()
			})

			pvc := newClaim("shared-claim", "claim-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty, true)
			reactor.claims[pvc.Name] = pvc
			pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
			if err := pvcIndexer.Add(pvc.DeepCopy()); err != nil {
				t.Fatal(err)
			}
			ctrl.pvcLister = corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
			class := newSnapshotClass(classSilver, "class-uid", mockDriverName, false)
			class.Namespace = ""
			if err := factory.Snapshot().V1().VolumeSnapshotClasses().Informer().GetIndexer().Add(class); err != nil {
				t.Fatal(err)
			}
			for i := range test.snapshotCount {
				name := fmt.Sprintf("snapshot-%d", i)
				snapshot := newSnapshot(name, name, pvc.Name, "", classSilver, "", &False, nil, nil, nil, false, false, &timeNowMetav1)
				snapshot.Finalizers = []string{utils.VolumeSnapshotAsSourceFinalizer}
				reactor.snapshots[name] = snapshot
			}
			watcher := watch.NewRaceFreeFake()
			client.AddWatchReactor("volumesnapshots", func(core.Action) (bool, watch.Interface, error) {
				return true, watcher, nil
			})
			finalized := make(chan *crdv1.VolumeSnapshot, test.snapshotCount)
			client.PrependReactor("update", "volumesnapshots", func(action core.Action) (bool, runtime.Object, error) {
				handled, obj, err := reactor.React(action)
				if err != nil {
					return handled, obj, err
				}
				updated := obj.(*crdv1.VolumeSnapshot)
				if updated.DeletionTimestamp != nil && len(updated.Finalizers) == 0 {
					// Delete API state immediately. No watcher is attached to the
					// reactor; delivery waits until all finalization workers finish.
					reactor.deleteSnapshotEvent(updated)
					finalized <- updated.DeepCopy()
				}
				return handled, obj, nil
			})

			informerStopped := make(chan struct{})
			go func() {
				defer close(informerStopped)
				informer.RunWithContext(ctx)
			}()
			t.Cleanup(func() {
				cancel()
				<-informerStopped
			})
			if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
				t.Fatal("snapshot informer did not sync")
			}

			for _, phase := range []string{"finalization", "deletion"} {
				// The real informer callbacks must enqueue each snapshot. Workers
				// start only after the full batch arrives, making ordering explicit.
				if err := wait.PollUntilContextCancel(ctx, time.Millisecond, true, func(context.Context) (bool, error) {
					return ctrl.snapshotQueue.Len() == test.snapshotCount, nil
				}); err != nil {
					t.Fatalf("%s: informer did not enqueue %d snapshots: %v", phase, test.snapshotCount, err)
				}
				done := make(chan struct{}, test.snapshotCount)
				for range test.snapshotCount {
					workers.Add(1)
					go func() {
						defer workers.Done()
						ctrl.snapshotWorker()
						done <- struct{}{}
					}()
				}
				// Worker completion includes workqueue.Done and the complete delete
				// callback, not just removal of the controller-store entry.
				for range test.snapshotCount {
					select {
					case <-done:
					case <-ctx.Done():
						t.Fatalf("%s: snapshot worker did not finish", phase)
					}
				}
				if phase == "finalization" {
					remaining, err := client.SnapshotV1().VolumeSnapshots(testNamespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						t.Fatal(err)
					}
					if len(remaining.Items) != 0 {
						t.Fatalf("expected all API snapshots to be finalized, got %d remaining", len(remaining.Items))
					}
					for range test.snapshotCount {
						select {
						case snapshot := <-finalized:
							watcher.Delete(snapshot)
						case <-ctx.Done():
							t.Fatal("snapshot finalization event missing")
						}
					}
				}
			}
			updatedPVC, err := kubeClient.CoreV1().PersistentVolumeClaims(testNamespace).Get(ctx, pvc.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(updatedPVC.Finalizers, utils.PVCFinalizer) {
				t.Fatalf("PVC %s/%s still has finalizer %q after all %d API snapshots were deleted, informer deletion events were delivered, and workers completed", pvc.Namespace, pvc.Name, utils.PVCFinalizer, test.snapshotCount)
			}
		})
	}
}

func TestPVCFinalizerCleanupRetries(t *testing.T) {
	tests := []struct {
		name               string
		verb               string
		replacementSource  string
		expectedFinalizers []string
	}{
		{
			name:               "get_failure",
			verb:               "get",
			expectedFinalizers: []string{"example.com/other-protection"},
		},
		{
			name:               "update_failure",
			verb:               "update",
			expectedFinalizers: []string{"example.com/other-protection"},
		},
		{
			name:               "name_reused_same_source",
			verb:               "get",
			replacementSource:  "source",
			expectedFinalizers: []string{utils.PVCFinalizer, "example.com/other-protection"},
		},
		{
			name:               "name_reused_different_source",
			verb:               "get",
			replacementSource:  "new-source",
			expectedFinalizers: []string{"example.com/other-protection"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), wait.ForeverTestTimeout)
			defer cancel()
			client, kubeClient := &fake.Clientset{}, &kubefake.Clientset{}
			ctrl, err := newTestController(kubeClient, client, nil, t, controllerTest{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctrl.snapshotQueue.ShutDown()
				ctrl.contentQueue.ShutDown()
				ctrl.groupSnapshotQueue.ShutDown()
				ctrl.groupSnapshotContentQueue.ShutDown()
				ctrl.eventBroadcaster.Shutdown()
			})
			cleanupError := reactorError{test.verb, "persistentvolumeclaims", fmt.Errorf("injected cleanup failure")}
			reactor := newSnapshotReactor(kubeClient, client, ctrl, nil, nil, []reactorError{cleanupError})
			pvc := newClaim("source", "claim-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty, true)
			pvc.Finalizers = append(pvc.Finalizers, "example.com/other-protection")
			reactor.claims[pvc.Name] = pvc
			pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
			if err := pvcIndexer.Add(pvc.DeepCopy()); err != nil {
				t.Fatal(err)
			}
			ctrl.pvcLister = corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
			deleted := newSnapshot("deleted", "old-uid", pvc.Name, "", classSilver, "", &False, nil, nil, nil, false, false, &timeNowMetav1)
			// Only the controller store retains this deleted snapshot's source.
			if err := ctrl.snapshotStore.Add(deleted); err != nil {
				t.Fatal(err)
			}
			key := utils.SnapshotKey(deleted)
			ctrl.snapshotQueue.Add(key)
			ctrl.snapshotWorker()
			if ctrl.snapshotQueue.NumRequeues(key) == 0 {
				t.Fatal("failed cleanup was not scheduled for retry")
			}
			stored, err := ctrl.getSnapshotFromStore(key)
			if err != nil {
				t.Fatal(err)
			}
			if stored == nil || stored.UID != deleted.UID || stored.Spec.Source.PersistentVolumeClaimName == nil || *stored.Spec.Source.PersistentVolumeClaimName != pvc.Name {
				t.Fatalf("failed cleanup lost the deleted snapshot's source: %#v", stored)
			}

			if len(test.replacementSource) != 0 {
				// No class is installed: its error-status update must not
				// overwrite the old source while cleanup is still failing.
				replacement := newSnapshot(deleted.Name, "new-uid", test.replacementSource, "", classSilver, "", &False, nil, nil, nil, false, false, nil)
				replacement.ResourceVersion = "2"
				reactor.snapshots[replacement.Name] = replacement
				if err := ctrl.snapshotIndexer.Add(replacement.DeepCopy()); err != nil {
					t.Fatal(err)
				}
				if test.replacementSource != pvc.Name {
					reactor.claims[test.replacementSource] = newClaim(test.replacementSource, "new-claim-uid", "1Gi", "new-volume", v1.ClaimBound, &classEmpty, true)
					reactor.errors = append(reactor.errors, cleanupError)
					if err := ctrl.syncSnapshotByKey(key); err == nil {
						t.Fatal("replacement did not propagate old-source cleanup failure")
					}
					stored, err = ctrl.getSnapshotFromStore(key)
					if err != nil {
						t.Fatal(err)
					}
					if stored == nil || stored.UID != deleted.UID {
						t.Fatalf("replacement overwrote the pending old source: %#v", stored)
					}
					unchanged, err := client.SnapshotV1().VolumeSnapshots(replacement.Namespace).Get(ctx, replacement.Name, metav1.GetOptions{})
					if err != nil {
						t.Fatal(err)
					}
					if unchanged.Status != nil && unchanged.Status.Error != nil {
						t.Fatal("replacement class reconciliation ran before old-source cleanup succeeded")
					}
				}
			}

			if err := wait.PollUntilContextCancel(ctx, time.Millisecond, true, func(context.Context) (bool, error) {
				return ctrl.snapshotQueue.Len() != 0, nil
			}); err != nil {
				t.Fatalf("rate-limited retry was not delivered: %v", err)
			}
			ctrl.snapshotWorker()
			updated, err := kubeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(updated.Finalizers, test.expectedFinalizers) {
				t.Fatalf("PVC finalizers=%v, want %v", updated.Finalizers, test.expectedFinalizers)
			}
			stored, err = ctrl.getSnapshotFromStore(key)
			if err != nil {
				t.Fatal(err)
			}
			if len(test.replacementSource) == 0 {
				if stored != nil || ctrl.snapshotQueue.NumRequeues(key) != 0 {
					t.Fatalf("successful cleanup retained source or retry state: snapshot=%#v, retries=%d", stored, ctrl.snapshotQueue.NumRequeues(key))
				}
			} else {
				if stored == nil || stored.UID != "new-uid" || stored.Status == nil || stored.Status.Error == nil {
					t.Fatalf("replacement did not reconcile its missing class after the cleanup check: %#v", stored)
				}
				if test.replacementSource != pvc.Name {
					replacementPVC, err := kubeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, test.replacementSource, metav1.GetOptions{})
					if err != nil {
						t.Fatal(err)
					}
					if !slices.Contains(replacementPVC.Finalizers, utils.PVCFinalizer) {
						t.Fatal("cleanup removed the replacement PVC's protection")
					}
				}
			}
		})
	}
}

func TestPVCFinalizerProtectionUntilOtherSnapshotReady(t *testing.T) {
	tests := []struct {
		name                           string
		initialReady                   bool
		terminating                    bool
		expectedFinalReady             bool
		expectedProtectedAfterDeletion bool
	}{
		{
			name:                           "unready_peer_becomes_ready",
			expectedFinalReady:             true,
			expectedProtectedAfterDeletion: true,
		},
		{
			name:                           "ready_peer",
			initialReady:                   true,
			expectedFinalReady:             true,
			expectedProtectedAfterDeletion: false,
		},
		{
			name:                           "terminating_unready_peer",
			terminating:                    true,
			expectedFinalReady:             false,
			expectedProtectedAfterDeletion: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deleted := newSnapshot("deleted", "deleted-uid", "source", "", classSilver, "", &False, nil, nil, nil, false, false, &timeNowMetav1)
			peer := newSnapshot("peer", "peer-uid", "source", "", classSilver, "peer-content", &test.initialReady, nil, nil, nil, false, true, nil)
			if test.terminating {
				peer.DeletionTimestamp = &timeNowMetav1
			}
			readyPeer := peer.DeepCopy()
			readyPeer.Status.ReadyToUse = &True
			expectedPeer := peer.DeepCopy()
			expectedPeer.Status.ReadyToUse = &test.expectedFinalReady
			content := newContent("peer-content", "peer-uid", peer.Name, "snapshot-handle", classSilver, "", "volume", crdv1.VolumeSnapshotContentDelete, nil, nil, true, true)
			runSyncTests(t, []controllerTest{{
				name:              test.name,
				initialSnapshots:  []*crdv1.VolumeSnapshot{deleted, peer},
				expectedSnapshots: []*crdv1.VolumeSnapshot{expectedPeer},
				initialContents:   []*crdv1.VolumeSnapshotContent{content},
				expectedContents:  []*crdv1.VolumeSnapshotContent{content},
				initialClaims:     newClaimArrayFinalizer("source", "claim-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty),
				expectSuccess:     true,
				test: func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, _ controllerTest) error {
					t.Cleanup(func() {
						ctrl.snapshotQueue.ShutDown()
						ctrl.contentQueue.ShutDown()
						ctrl.groupSnapshotQueue.ShutDown()
						ctrl.groupSnapshotContentQueue.ShutDown()
						ctrl.eventBroadcaster.Shutdown()
					})
					ctx := context.Background()
					reactor.deleteSnapshotEvent(deleted)
					if err := ctrl.snapshotIndexer.Add(peer.DeepCopy()); err != nil {
						return err
					}
					if err := ctrl.syncSnapshotByKey(utils.SnapshotKey(deleted)); err != nil {
						return err
					}
					pvc, err := ctrl.client.CoreV1().PersistentVolumeClaims(peer.Namespace).Get(ctx, "source", metav1.GetOptions{})
					if err != nil {
						return err
					}
					if protected := slices.Contains(pvc.Finalizers, utils.PVCFinalizer); protected != test.expectedProtectedAfterDeletion {
						return fmt.Errorf("PVC protected=%v, want %v", protected, test.expectedProtectedAfterDeletion)
					}
					if test.initialReady || test.terminating {
						return nil
					}

					// Reconcile the remaining snapshot's readiness update.
					updated, err := ctrl.clientset.SnapshotV1().VolumeSnapshots(peer.Namespace).UpdateStatus(ctx, readyPeer, metav1.UpdateOptions{})
					if err != nil {
						return err
					}
					if err := ctrl.snapshotIndexer.Update(updated); err != nil {
						return err
					}
					if err := ctrl.syncSnapshot(ctx, updated); err != nil {
						return err
					}
					pvc, err = ctrl.client.CoreV1().PersistentVolumeClaims(peer.Namespace).Get(ctx, "source", metav1.GetOptions{})
					if err != nil {
						return err
					}
					if slices.Contains(pvc.Finalizers, utils.PVCFinalizer) {
						return fmt.Errorf("PVC remained protected after its last unready snapshot became ready")
					}
					return nil
				},
			}}, snapshotClasses, nil)
		})
	}
}

func TestPVCFinalizerCleanupAPIBoundaries(t *testing.T) {
	tests := []struct {
		name               string
		verb               string
		failure            string
		withFinalizer      bool
		expectedFinalizers []string
		expectNoUpdate     bool
	}{
		{
			name:               "already_unprotected",
			withFinalizer:      false,
			expectedFinalizers: []string{"example.com/other-protection"},
			expectNoUpdate:     true,
		},
		{name: "get_not_found", verb: "get", failure: "not_found", withFinalizer: true},
		{name: "update_not_found", verb: "update", failure: "not_found", withFinalizer: true},
		{
			name:               "update_conflict_preserves_concurrent_finalizer",
			verb:               "update",
			failure:            "conflict",
			withFinalizer:      true,
			expectedFinalizers: []string{"example.com/other-protection", "example.com/concurrent-protection"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			pvc := newClaim("source", "claim-uid", "1Gi", "volume", v1.ClaimBound, &classEmpty, test.withFinalizer)
			pvc.Finalizers = append(pvc.Finalizers, "example.com/other-protection")
			client := kubefake.NewClientset(pvc)
			ctrl := &csiSnapshotCommonController{client: client}
			injected := false
			if len(test.failure) != 0 {
				client.PrependReactor(test.verb, "persistentvolumeclaims", func(core.Action) (bool, runtime.Object, error) {
					if injected {
						return false, nil, nil
					}
					injected = true
					resource := v1.SchemeGroupVersion.WithResource("persistentvolumeclaims")
					if test.failure == "conflict" {
						concurrent := pvc.DeepCopy()
						concurrent.ResourceVersion = "2"
						concurrent.Finalizers = append(concurrent.Finalizers, "example.com/concurrent-protection")
						if err := client.Tracker().Update(resource, concurrent, pvc.Namespace); err != nil {
							t.Fatal(err)
						}
						return true, nil, apierrs.NewConflict(resource.GroupResource(), pvc.Name, fmt.Errorf("concurrent PVC update"))
					}
					if err := client.Tracker().Delete(resource, pvc.Namespace, pvc.Name); err != nil {
						t.Fatal(err)
					}
					return true, nil, apierrs.NewNotFound(resource.GroupResource(), pvc.Name)
				})
			}

			if err := ctrl.removePVCFinalizer(pvc); err != nil {
				t.Fatal(err)
			}
			if len(test.failure) != 0 && !injected {
				t.Fatal("cleanup did not encounter the injected API failure")
			}
			if test.failure == "not_found" {
				if _, err := client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{}); !apierrs.IsNotFound(err) {
					t.Fatalf("cleanup of a deleted PVC must succeed without recreating it, got %v", err)
				}
				return
			}
			updated, err := client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(updated.Finalizers, test.expectedFinalizers) {
				t.Fatalf("cleanup must preserve other controllers' finalizers: got %v, want %v", updated.Finalizers, test.expectedFinalizers)
			}
			if test.expectNoUpdate {
				for _, action := range client.Actions() {
					if action.GetVerb() == "update" {
						t.Fatal("already-unprotected PVC caused an unnecessary update")
					}
				}
			}
		})
	}
}
