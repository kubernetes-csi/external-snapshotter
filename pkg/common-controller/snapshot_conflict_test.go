/*
Copyright 2026 The Kubernetes Authors.

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
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func newConflictTestController(
	t *testing.T,
	test controllerTest,
) (*csiSnapshotCommonController, *snapshotReactor) {
	t.Helper()

	kubeClient := &kubefake.Clientset{}
	client := &fake.Clientset{}

	ctrl, err := newTestController(kubeClient, client, nil, t, test)
	if err != nil {
		t.Fatalf("construct controller failed: %v", err)
	}

	reactor := newSnapshotReactor(kubeClient, client, ctrl, nil, nil, test.errors)
	for _, snapshot := range test.initialSnapshots {
		ctrl.snapshotStore.Add(snapshot)
		reactor.snapshots[snapshot.Name] = snapshot
	}
	for _, content := range test.initialContents {
		ctrl.contentStore.Add(content)
		reactor.contents[content.Name] = content
	}

	pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, claim := range test.initialClaims {
		reactor.claims[claim.Name] = claim
		pvcIndexer.Add(claim)
	}
	ctrl.pvcLister = corelisters.NewPersistentVolumeClaimLister(pvcIndexer)

	return ctrl, reactor
}

func newConflict(resource, name string) error {
	return apierrs.NewConflict(schema.GroupResource{Resource: resource}, name, fmt.Errorf("resource version conflict"))
}

func TestEnsurePVCFinalizerConflictIsRetriedAndSucceeds(t *testing.T) {
	test := controllerTest{
		name:             "ensure pvc finalizer retries on conflict",
		initialSnapshots: newSnapshotArray("snap-conflict-1", "snapuid-conflict-1", "claim-conflict-1", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
		initialClaims:    newClaimArray("claim-conflict-1", "pvcuid-conflict-1", "1Gi", "volume-conflict-1", v1.ClaimBound, &classEmpty),
		errors: []reactorError{
			{"update", "persistentvolumeclaims", newConflict("persistentvolumeclaims", "claim-conflict-1")},
		},
	}
	ctrl, reactor := newConflictTestController(t, test)

	err := ctrl.ensurePVCFinalizer(test.initialSnapshots[0])
	if err != nil {
		t.Fatalf("expected retry to succeed after one conflict, got: %v", err)
	}

	claim := reactor.claims["claim-conflict-1"]
	if claim == nil {
		t.Fatalf("expected claim to exist in reactor store")
	}
	if !slices.Contains(claim.Finalizers, "snapshot.storage.kubernetes.io/pvc-as-source-protection") {
		t.Fatalf("expected pvc finalizer to be present after retry, got: %v", claim.Finalizers)
	}
}

func TestUpdateSnapshotStatusConflictIsRetriedAndSucceeds(t *testing.T) {
	created := time.Now()
	createdNano := created.UnixNano()
	size := int64(1)
	ready := true
	test := controllerTest{
		name:             "update snapshot status retries on conflict",
		initialSnapshots: newSnapshotArray("snap-conflict-2", "snapuid-conflict-2", "claim-conflict-2", "", classSilver, "", &False, nil, nil, nil, false, true, nil),
		initialContents:  newContentArrayWithReadyToUse("snapcontent-snapuid-conflict-2", "snapuid-conflict-2", "snap-conflict-2", "sid-conflict-2", classSilver, "", "pv-handle-conflict-2", deletionPolicy, &createdNano, &size, &ready, false),
		errors: []reactorError{
			{"update", "volumesnapshots", newConflict("volumesnapshots", "snap-conflict-2")},
		},
	}
	ctrl, _ := newConflictTestController(t, test)

	snapshot, err := ctrl.updateSnapshotStatus(test.initialSnapshots[0], test.initialContents[0])
	if err != nil {
		t.Fatalf("expected retry to succeed after one conflict, got: %v", err)
	}
	if snapshot.Status == nil || snapshot.Status.BoundVolumeSnapshotContentName == nil || *snapshot.Status.BoundVolumeSnapshotContentName != "snapcontent-snapuid-conflict-2" {
		t.Fatalf("expected snapshot status to be updated, got: %#v", snapshot.Status)
	}
}

func TestAddSnapshotFinalizerConflictIsRetriedAndSucceeds(t *testing.T) {
	test := controllerTest{
		name:             "add snapshot finalizer retries on conflict",
		initialSnapshots: newSnapshotArray("snap-conflict-3", "snapuid-conflict-3", "claim-conflict-3", "", classSilver, "", &False, nil, nil, nil, false, false, nil),
		errors: []reactorError{
			{"update", "volumesnapshots", newConflict("volumesnapshots", "snap-conflict-3")},
		},
	}
	ctrl, reactor := newConflictTestController(t, test)

	err := ctrl.addSnapshotFinalizer(test.initialSnapshots[0], true, true)
	if err != nil {
		t.Fatalf("expected retry to succeed after one conflict, got: %v", err)
	}
	snapshot := reactor.snapshots["snap-conflict-3"]
	if snapshot == nil {
		t.Fatalf("expected snapshot to exist in reactor store")
	}
	if !slices.Contains(snapshot.Finalizers, "snapshot.storage.kubernetes.io/volumesnapshot-as-source-protection") ||
		!slices.Contains(snapshot.Finalizers, "snapshot.storage.kubernetes.io/volumesnapshot-bound-protection") {
		t.Fatalf("expected both snapshot finalizers to be present after retry, got: %v", snapshot.Finalizers)
	}
}
