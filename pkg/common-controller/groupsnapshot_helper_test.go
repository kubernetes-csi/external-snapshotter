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
	"context"
	"fmt"
	"testing"

	crdv1beta2 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta2"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

// helperSetup is shared infrastructure for direct unit-tests of the group-snapshot helper methods.
// It wires together a minimal controller and the fake-client reactor so that helper calls go
// through the same reactor plumbing as the broader framework tests.
type helperSetup struct {
	ctrl    *csiSnapshotCommonController
	reactor *snapshotReactor
	client  *fake.Clientset
	kube    *kubefake.Clientset
}

// newHelperSetup creates a minimal controller backed by fake clients and a pre-wired reactor.
func newHelperSetup(t *testing.T) *helperSetup {
	t.Helper()
	kube := &kubefake.Clientset{}
	client := &fake.Clientset{}
	ctrl, err := newTestController(kube, client, nil, t, controllerTest{})
	if err != nil {
		t.Fatalf("failed to create test controller: %v", err)
	}
	reactor := newSnapshotReactor(kube, client, ctrl, nil, nil, nil)
	return &helperSetup{ctrl: ctrl, reactor: reactor, client: client, kube: kube}
}

// --- object builder helpers ---

// makeTestGroupSnapshotContent returns a VolumeGroupSnapshotContent ready for tests.
func makeTestGroupSnapshotContent(
	name, driver, snapshotNamespace string,
	groupHandle string,
	policy crdv1.DeletionPolicy,
) *crdv1beta2.VolumeGroupSnapshotContent {
	return &crdv1beta2.VolumeGroupSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
			Driver:         driver,
			DeletionPolicy: policy,
			VolumeGroupSnapshotRef: v1.ObjectReference{
				Name:      "test-gs",
				Namespace: snapshotNamespace,
			},
		},
		Status: &crdv1beta2.VolumeGroupSnapshotContentStatus{
			VolumeGroupSnapshotHandle: &groupHandle,
		},
	}
}

// makeTestGroupSnapshot returns a VolumeGroupSnapshot ready for tests.
func makeTestGroupSnapshot(name, namespace string, uid types.UID) *crdv1beta2.VolumeGroupSnapshot {
	return &crdv1beta2.VolumeGroupSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
	}
}

// makeCSIPersistentVolume returns a PV backed by the given CSI driver and volume handle.
func makeCSIPersistentVolume(pvName, driver, volumeHandle, pvcName, pvcNamespace string) *v1.PersistentVolume {
	mode := v1.PersistentVolumeFilesystem
	return &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: pvName},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       driver,
					VolumeHandle: volumeHandle,
				},
			},
			ClaimRef: &v1.ObjectReference{
				Name:      pvcName,
				Namespace: pvcNamespace,
			},
			VolumeMode: &mode,
		},
	}
}

// --- reactor builder helpers ---

// alreadyExistsReactor returns a PrependReactor that makes a Create call return AlreadyExists.
func alreadyExistsReactor(resource string) func(core.Action) (bool, runtime.Object, error) {
	return func(action core.Action) (bool, runtime.Object, error) {
		obj, _ := action.(core.CreateAction)
		name := ""
		if obj != nil {
			if meta, ok := obj.GetObject().(metav1.Object); ok {
				name = meta.GetName()
			}
		}
		return true, nil, apierrs.NewAlreadyExists(
			schema.GroupResource{Group: "snapshot.storage.k8s.io", Resource: resource},
			name,
		)
	}
}

// genericErrorReactor returns a PrependReactor that always fails with the given message.
func genericErrorReactor(msg string) func(core.Action) (bool, runtime.Object, error) {
	return func(_ core.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("%s", msg)
	}
}

// --- TestCreateOrGetVolumeSnapshotContent ---

func TestCreateOrGetVolumeSnapshotContent(t *testing.T) {
	t.Run("creates new content successfully", func(t *testing.T) {
		h := newHelperSetup(t)

		vsc := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "content-new"},
			Spec: crdv1.VolumeSnapshotContentSpec{
				Driver:         mockDriverName,
				DeletionPolicy: deletionPolicy,
			},
		}
		result, err := h.ctrl.createOrGetVolumeSnapshotContent(context.Background(), vsc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Name != "content-new" {
			t.Errorf("name = %q, want %q", result.Name, "content-new")
		}
		if _, found := h.reactor.contents["content-new"]; !found {
			t.Error("content not stored in reactor after creation")
		}
	})

	t.Run("returns existing content on AlreadyExists", func(t *testing.T) {
		h := newHelperSetup(t)

		existing := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "content-existing",
				ResourceVersion: "5",
			},
		}
		h.reactor.contents["content-existing"] = existing
		h.client.PrependReactor("create", "volumesnapshotcontents",
			alreadyExistsReactor("volumesnapshotcontents"))

		vsc := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "content-existing"},
		}
		result, err := h.ctrl.createOrGetVolumeSnapshotContent(context.Background(), vsc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ResourceVersion != "5" {
			t.Errorf("ResourceVersion = %q, want %q (expected existing object)", result.ResourceVersion, "5")
		}
	})

	t.Run("propagates non-AlreadyExists create error", func(t *testing.T) {
		h := newHelperSetup(t)
		h.client.PrependReactor("create", "volumesnapshotcontents",
			genericErrorReactor("simulated-create-failure"))

		vsc := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "content-fail"},
		}
		_, err := h.ctrl.createOrGetVolumeSnapshotContent(context.Background(), vsc)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

// --- TestCreateOrGetVolumeSnapshot ---

func TestCreateOrGetVolumeSnapshot(t *testing.T) {
	t.Run("creates new snapshot with non-empty UID", func(t *testing.T) {
		h := newHelperSetup(t)

		vs := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snap-new",
				Namespace: testNamespace,
			},
		}
		result, err := h.ctrl.createOrGetVolumeSnapshot(context.Background(), vs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Name != "snap-new" {
			t.Errorf("name = %q, want %q", result.Name, "snap-new")
		}
		if result.UID == "" {
			t.Error("reactor should have assigned a non-empty UID to the new snapshot")
		}
		if _, found := h.reactor.snapshots["snap-new"]; !found {
			t.Error("snapshot not stored in reactor after creation")
		}
	})

	t.Run("returns existing snapshot on AlreadyExists", func(t *testing.T) {
		h := newHelperSetup(t)

		existing := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "snap-existing",
				Namespace:       testNamespace,
				UID:             "existing-uid",
				ResourceVersion: "7",
			},
		}
		h.reactor.snapshots["snap-existing"] = existing
		h.client.PrependReactor("create", "volumesnapshots",
			alreadyExistsReactor("volumesnapshots"))

		vs := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snap-existing",
				Namespace: testNamespace,
			},
		}
		result, err := h.ctrl.createOrGetVolumeSnapshot(context.Background(), vs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.UID != "existing-uid" {
			t.Errorf("UID = %q, want %q", result.UID, "existing-uid")
		}
	})

	t.Run("propagates non-AlreadyExists create error", func(t *testing.T) {
		h := newHelperSetup(t)
		h.client.PrependReactor("create", "volumesnapshots",
			genericErrorReactor("simulated-create-failure"))

		vs := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snap-fail",
				Namespace: testNamespace,
			},
		}
		_, err := h.ctrl.createOrGetVolumeSnapshot(context.Background(), vs)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

// --- TestBindSnapshotContentToSnapshot ---

func TestBindSnapshotContentToSnapshot(t *testing.T) {
	t.Run("patches content UID with snapshot UID", func(t *testing.T) {
		h := newHelperSetup(t)

		content := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "content-bind"},
			Spec: crdv1.VolumeSnapshotContentSpec{
				VolumeSnapshotRef: v1.ObjectReference{
					Kind:      "VolumeSnapshot",
					Name:      "snap-bind",
					Namespace: testNamespace,
				},
				Driver:         mockDriverName,
				DeletionPolicy: deletionPolicy,
			},
		}
		h.reactor.contents["content-bind"] = content

		snapshot := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snap-bind",
				Namespace: testNamespace,
				UID:       "snap-uid-bind",
			},
		}

		if err := h.ctrl.bindSnapshotContentToSnapshot(content, snapshot); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		stored := h.reactor.contents["content-bind"]
		if stored.Spec.VolumeSnapshotRef.UID != "snap-uid-bind" {
			t.Errorf("VolumeSnapshotRef.UID = %q, want %q",
				stored.Spec.VolumeSnapshotRef.UID, "snap-uid-bind")
		}
	})

	t.Run("errors when content not found in reactor", func(t *testing.T) {
		h := newHelperSetup(t)

		content := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "content-missing"},
		}
		snapshot := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name: "snap-x",
				UID:  "uid-x",
			},
		}
		if err := h.ctrl.bindSnapshotContentToSnapshot(content, snapshot); err == nil {
			t.Fatal("expected error for missing content, got nil")
		}
	})
}

// --- TestBindSnapshotToSnapshotContent ---

func TestBindSnapshotToSnapshotContent(t *testing.T) {
	t.Run("patches snapshot status with content name", func(t *testing.T) {
		h := newHelperSetup(t)

		snapshot := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snap-status",
				Namespace: testNamespace,
				UID:       "uid-status",
			},
		}
		h.reactor.snapshots["snap-status"] = snapshot

		if err := h.ctrl.bindSnapshotToSnapshotContent(snapshot, "content-target"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		stored := h.reactor.snapshots["snap-status"]
		if stored.Status == nil || stored.Status.BoundVolumeSnapshotContentName == nil {
			t.Fatal("status or BoundVolumeSnapshotContentName is nil after patch")
		}
		if *stored.Status.BoundVolumeSnapshotContentName != "content-target" {
			t.Errorf("BoundVolumeSnapshotContentName = %q, want %q",
				*stored.Status.BoundVolumeSnapshotContentName, "content-target")
		}
	})

	t.Run("errors when snapshot not found in reactor", func(t *testing.T) {
		h := newHelperSetup(t)

		snapshot := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snap-missing",
				Namespace: testNamespace,
			},
		}
		if err := h.ctrl.bindSnapshotToSnapshotContent(snapshot, "content-target"); err == nil {
			t.Fatal("expected error for missing snapshot, got nil")
		}
	})
}

// --- TestUpdateVolumeSnapshotContentStatus ---

func TestUpdateVolumeSnapshotContentStatus(t *testing.T) {
	readyToUse := true
	creationTime := int64(1234567890)
	restoreSize := int64(1073741824)
	groupHandle := "grp-handle-1"

	t.Run("patches content with all status fields", func(t *testing.T) {
		h := newHelperSetup(t)

		content := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "content-status"},
		}
		h.reactor.contents["content-status"] = content

		snapshotInfo := crdv1beta2.VolumeSnapshotInfo{
			SnapshotHandle: "snap-handle-1",
			CreationTime:   &creationTime,
			ReadyToUse:     &readyToUse,
			RestoreSize:    &restoreSize,
		}
		groupContent := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-1"},
			Status: &crdv1beta2.VolumeGroupSnapshotContentStatus{
				VolumeGroupSnapshotHandle: &groupHandle,
			},
		}

		if err := h.ctrl.updateVolumeSnapshotContentStatus(content, snapshotInfo, groupContent); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		stored := h.reactor.contents["content-status"]
		if stored.Status == nil {
			t.Fatal("status is nil after patch")
		}
		if stored.Status.SnapshotHandle == nil || *stored.Status.SnapshotHandle != "snap-handle-1" {
			t.Errorf("SnapshotHandle = %v, want %q", stored.Status.SnapshotHandle, "snap-handle-1")
		}
		if stored.Status.VolumeGroupSnapshotHandle == nil || *stored.Status.VolumeGroupSnapshotHandle != groupHandle {
			t.Errorf("VolumeGroupSnapshotHandle = %v, want %q",
				stored.Status.VolumeGroupSnapshotHandle, groupHandle)
		}
		if stored.Status.ReadyToUse == nil || !*stored.Status.ReadyToUse {
			t.Errorf("ReadyToUse = %v, want true", stored.Status.ReadyToUse)
		}
		if stored.Status.CreationTime == nil || *stored.Status.CreationTime != creationTime {
			t.Errorf("CreationTime = %v, want %d", stored.Status.CreationTime, creationTime)
		}
		if stored.Status.RestoreSize == nil || *stored.Status.RestoreSize != restoreSize {
			t.Errorf("RestoreSize = %v, want %d", stored.Status.RestoreSize, restoreSize)
		}
	})

	t.Run("errors when content not found in reactor", func(t *testing.T) {
		h := newHelperSetup(t)

		content := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "content-missing-status"},
		}
		snapshotInfo := crdv1beta2.VolumeSnapshotInfo{SnapshotHandle: "x"}
		groupContent := &crdv1beta2.VolumeGroupSnapshotContent{
			Status: &crdv1beta2.VolumeGroupSnapshotContentStatus{
				VolumeGroupSnapshotHandle: &groupHandle,
			},
		}

		if err := h.ctrl.updateVolumeSnapshotContentStatus(content, snapshotInfo, groupContent); err == nil {
			t.Fatal("expected error for missing content, got nil")
		}
	})
}

// assertReactorStateAfterIndividualSnapshot checks that the fake reactor holds the expected
// VolumeSnapshot and VolumeSnapshotContent after createIndividualSnapshotForGroupSnapshot succeeds:
// objects exist, bidirectional binding (ref UID, bound content name), status fields, and optional
// PVC / volume-mode / deletion-secret annotations.
func assertReactorStateAfterIndividualSnapshot(
	t *testing.T,
	h *helperSetup,
	groupUID types.UID,
	volumeHandle string,
	info crdv1beta2.VolumeSnapshotInfo,
	groupHandle string,
	wantPVC string,
	wantSecret *v1.SecretReference,
) {
	t.Helper()

	snapName := getSnapshotNameForVolumeGroupSnapshotContent(string(groupUID), volumeHandle)
	contentName := getSnapshotContentNameForVolumeGroupSnapshotContent(string(groupUID), volumeHandle)

	contents, err := h.client.SnapshotV1().VolumeSnapshotContents().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list VolumeSnapshotContents: %v", err)
	}
	if len(contents.Items) != 1 {
		t.Fatalf("expected 1 VolumeSnapshotContent to be created, got %d", len(contents.Items))
	}
	content := &contents.Items[0]

	if content.Name != contentName {
		t.Errorf("VSC name = %q, want %q", content.Name, contentName)
	}

	snapshots, err := h.client.SnapshotV1().VolumeSnapshots(testNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list VolumeSnapshots: %v", err)
	}
	if len(snapshots.Items) != 1 {
		t.Fatalf("expected 1 VolumeSnapshot to be created, got %d", len(snapshots.Items))
	}
	snap := &snapshots.Items[0]

	if snap.Name != snapName {
		t.Errorf("VS name = %q, want %q", snap.Name, snapName)
	}

	expectedSnapUID := types.UID(snapName + "-uid")
	if snap.UID != expectedSnapUID {
		t.Errorf("VolumeSnapshot.UID = %q, want %q (reactor assigns name+\"-uid\")", snap.UID, expectedSnapUID)
	}
	if content.Spec.VolumeSnapshotRef.Name != snapName {
		t.Errorf("VSC.Spec.VolumeSnapshotRef.Name = %q, want %q", content.Spec.VolumeSnapshotRef.Name, snapName)
	}
	if content.Spec.VolumeSnapshotRef.Namespace != testNamespace {
		t.Errorf("VSC.Spec.VolumeSnapshotRef.Namespace = %q, want %q", content.Spec.VolumeSnapshotRef.Namespace, testNamespace)
	}
	if content.Spec.VolumeSnapshotRef.UID != snap.UID {
		t.Errorf("VSC.Spec.VolumeSnapshotRef.UID = %q, want %q (bound to snapshot)", content.Spec.VolumeSnapshotRef.UID, snap.UID)
	}
	if snap.Status == nil || snap.Status.BoundVolumeSnapshotContentName == nil {
		t.Fatal("VolumeSnapshot status or BoundVolumeSnapshotContentName is nil after bind")
	}
	if *snap.Status.BoundVolumeSnapshotContentName != contentName {
		t.Errorf("BoundVolumeSnapshotContentName = %q, want %q", *snap.Status.BoundVolumeSnapshotContentName, contentName)
	}

	if content.Status == nil {
		t.Fatal("VolumeSnapshotContent.Status is nil after updateVolumeSnapshotContentStatus")
	}
	if content.Status.SnapshotHandle == nil || *content.Status.SnapshotHandle != info.SnapshotHandle {
		t.Errorf("Status.SnapshotHandle = %v, want %q", content.Status.SnapshotHandle, info.SnapshotHandle)
	}
	if content.Status.VolumeGroupSnapshotHandle == nil || *content.Status.VolumeGroupSnapshotHandle != groupHandle {
		t.Errorf("Status.VolumeGroupSnapshotHandle = %v, want %q", content.Status.VolumeGroupSnapshotHandle, groupHandle)
	}
	if content.Status.CreationTime == nil || *content.Status.CreationTime != *info.CreationTime {
		t.Errorf("Status.CreationTime = %v, want %v", content.Status.CreationTime, *info.CreationTime)
	}
	if content.Status.RestoreSize == nil || *content.Status.RestoreSize != *info.RestoreSize {
		t.Errorf("Status.RestoreSize = %v, want %v", content.Status.RestoreSize, *info.RestoreSize)
	}
	if content.Status.ReadyToUse == nil || *content.Status.ReadyToUse != *info.ReadyToUse {
		t.Errorf("Status.ReadyToUse = %v, want %v", content.Status.ReadyToUse, *info.ReadyToUse)
	}

	if got := content.Annotations[utils.VolumeGroupSnapshotHandleAnnotation]; got != groupHandle {
		t.Errorf("annotation %s = %q, want %q", utils.VolumeGroupSnapshotHandleAnnotation, got, groupHandle)
	}

	if snap.Spec.Source.PersistentVolumeClaimName == nil || *snap.Spec.Source.PersistentVolumeClaimName != wantPVC {
		t.Errorf("VolumeSnapshot PVC name = %v, want %q", snap.Spec.Source.PersistentVolumeClaimName, wantPVC)
	}
	if content.Spec.Source.VolumeHandle == nil || *content.Spec.Source.VolumeHandle != volumeHandle {
		t.Errorf("VSC source volume handle = %v, want %q", content.Spec.Source.VolumeHandle, volumeHandle)
	}
	if wantPVC != "" {
		if content.Spec.SourceVolumeMode == nil || *content.Spec.SourceVolumeMode != v1.PersistentVolumeFilesystem {
			t.Errorf("VSC.Spec.SourceVolumeMode = %v, want PersistentVolumeFilesystem", content.Spec.SourceVolumeMode)
		}
	} else if content.Spec.SourceVolumeMode != nil {
		t.Errorf("VSC.Spec.SourceVolumeMode = %v, want nil when no PV", content.Spec.SourceVolumeMode)
	}

	if wantSecret != nil {
		if got := content.Annotations[utils.AnnDeletionSecretRefName]; got != wantSecret.Name {
			t.Errorf("annotation %s = %q, want %q", utils.AnnDeletionSecretRefName, got, wantSecret.Name)
		}
		if got := content.Annotations[utils.AnnDeletionSecretRefNamespace]; got != wantSecret.Namespace {
			t.Errorf("annotation %s = %q, want %q", utils.AnnDeletionSecretRefNamespace, got, wantSecret.Namespace)
		}
	}
}

// --- TestCreateIndividualSnapshotForGroupSnapshot ---

func TestCreateIndividualSnapshotForGroupSnapshot(t *testing.T) {
	groupHandle := "grp-handle-x"
	readyToUse := true
	creationTime := int64(1000)
	restoreSize := int64(2000)

	newGSC := func(driver, ns string) *crdv1beta2.VolumeGroupSnapshotContent {
		return makeTestGroupSnapshotContent("gsc-1", driver, ns, groupHandle, deletionPolicy)
	}
	newGS := func(ns string) *crdv1beta2.VolumeGroupSnapshot {
		return makeTestGroupSnapshot("gs-1", ns, "gs-uid-1")
	}
	newInfo := func(volHandle string) crdv1beta2.VolumeSnapshotInfo {
		return crdv1beta2.VolumeSnapshotInfo{
			VolumeHandle:   volHandle,
			SnapshotHandle: "snap-handle-x",
			CreationTime:   &creationTime,
			ReadyToUse:     &readyToUse,
			RestoreSize:    &restoreSize,
		}
	}

	t.Run("full success with PV found in indexer", func(t *testing.T) {
		h := newHelperSetup(t)

		pv := makeCSIPersistentVolume("pv-1", mockDriverName, "vol-with-pv", "pvc-1", testNamespace)
		if err := h.ctrl.pvIndexer.Add(pv); err != nil {
			t.Fatalf("failed to add PV to indexer: %v", err)
		}

		info := newInfo("vol-with-pv")
		err := h.ctrl.createIndividualSnapshotForGroupSnapshot(
			context.Background(),
			info,
			newGSC(mockDriverName, testNamespace),
			newGS(testNamespace),
			nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertReactorStateAfterIndividualSnapshot(t, h, "gs-uid-1", "vol-with-pv", info, groupHandle, "pvc-1", nil)
	})

	t.Run("full success when no PV in indexer", func(t *testing.T) {
		h := newHelperSetup(t)

		info := newInfo("vol-no-pv")
		err := h.ctrl.createIndividualSnapshotForGroupSnapshot(
			context.Background(),
			info,
			newGSC(mockDriverName, testNamespace),
			newGS(testNamespace),
			nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertReactorStateAfterIndividualSnapshot(t, h, "gs-uid-1", "vol-no-pv", info, groupHandle, "", nil)
	})

	t.Run("error when snapshot is returned with empty UID", func(t *testing.T) {
		h := newHelperSetup(t)

		// Override create volumesnapshots to return the snapshot without assigning a UID.
		h.client.PrependReactor("create", "volumesnapshots",
			func(action core.Action) (bool, runtime.Object, error) {
				obj := action.(core.CreateAction).GetObject().(*crdv1.VolumeSnapshot)
				return true, obj, nil
			})

		err := h.ctrl.createIndividualSnapshotForGroupSnapshot(
			context.Background(),
			newInfo("vol-empty-uid"),
			newGSC(mockDriverName, testNamespace),
			newGS(testNamespace),
			nil,
		)
		if err == nil {
			t.Fatal("expected error for empty UID, got nil")
		}
	})

	t.Run("error propagated from createOrGetVolumeSnapshotContent", func(t *testing.T) {
		h := newHelperSetup(t)
		h.client.PrependReactor("create", "volumesnapshotcontents",
			genericErrorReactor("content-create-failure"))

		err := h.ctrl.createIndividualSnapshotForGroupSnapshot(
			context.Background(),
			newInfo("vol-content-err"),
			newGSC(mockDriverName, testNamespace),
			newGS(testNamespace),
			nil,
		)
		if err == nil {
			t.Fatal("expected error propagated from content creation, got nil")
		}
	})

	t.Run("error propagated from createOrGetVolumeSnapshot", func(t *testing.T) {
		h := newHelperSetup(t)
		h.client.PrependReactor("create", "volumesnapshots",
			genericErrorReactor("snapshot-create-failure"))

		err := h.ctrl.createIndividualSnapshotForGroupSnapshot(
			context.Background(),
			newInfo("vol-snap-err"),
			newGSC(mockDriverName, testNamespace),
			newGS(testNamespace),
			nil,
		)
		if err == nil {
			t.Fatal("expected error propagated from snapshot creation, got nil")
		}
	})

	t.Run("success with secret reference set", func(t *testing.T) {
		h := newHelperSetup(t)

		secret := &v1.SecretReference{
			Name:      "my-secret",
			Namespace: testNamespace,
		}

		info := newInfo("vol-with-secret")
		err := h.ctrl.createIndividualSnapshotForGroupSnapshot(
			context.Background(),
			info,
			newGSC(mockDriverName, testNamespace),
			newGS(testNamespace),
			secret,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertReactorStateAfterIndividualSnapshot(t, h, "gs-uid-1", "vol-with-secret", info, groupHandle, "", secret)
	})
}
