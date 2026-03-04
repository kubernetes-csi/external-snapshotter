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

package sidecar_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	crdv1beta2 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta2"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/group_snapshotter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fakeGroupSnapshotter implements group_snapshotter.GroupSnapshotter for testing csiHandler group snapshot methods.
type fakeGroupSnapshotter struct {
	createGroupSnapshotResult func() (driverName, groupSnapshotID string, snapshots []*csi.Snapshot, timestamp time.Time, readyToUse bool, err error)
	deleteGroupSnapshotErr    error
	getGroupSnapshotStatus    func() (readyToUse bool, timestamp time.Time, err error)
}

func (f *fakeGroupSnapshotter) CreateGroupSnapshot(ctx context.Context, groupSnapshotName string, volumeIDs []string, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, []*csi.Snapshot, time.Time, bool, error) {
	if f.createGroupSnapshotResult != nil {
		return f.createGroupSnapshotResult()
	}
	return "driver", "group-snap-id", nil, time.Now(), true, nil
}

func (f *fakeGroupSnapshotter) DeleteGroupSnapshot(ctx context.Context, groupSnapshotID string, snapshotIDs []string, snapshotterCredentials map[string]string) error {
	return f.deleteGroupSnapshotErr
}

func (f *fakeGroupSnapshotter) GetGroupSnapshotStatus(ctx context.Context, groupSnapshotID string, snapshotIDs []string, snapshotterCredentials map[string]string) (bool, time.Time, error) {
	if f.getGroupSnapshotStatus != nil {
		return f.getGroupSnapshotStatus()
	}
	return true, time.Now(), nil
}

func newCSIHandlerWithFakeGroupSnapshotter(f group_snapshotter.GroupSnapshotter) Handler {
	return NewCSIHandler(
		nil,
		f,
		5*time.Second,
		"snapshot",
		8,
		"group-snapshot",
		8,
	)
}

// TestCreateGroupSnapshot validates CreateGroupSnapshot error paths and success path.
func TestCreateGroupSnapshot(t *testing.T) {
	handler := newCSIHandlerWithFakeGroupSnapshotter(&fakeGroupSnapshotter{})

	t.Run("empty VolumeGroupSnapshotRef.UID", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-1"},
			Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
				VolumeGroupSnapshotRef: corev1.ObjectReference{UID: ""},
				Source:                 crdv1beta2.VolumeGroupSnapshotContentSource{VolumeHandles: []string{"vol-1"}},
			},
		}
		_, _, _, _, _, err := handler.CreateGroupSnapshot(content, nil, nil)
		if err == nil {
			t.Fatal("expected error when UID is empty")
		}
		if !contains(err.Error(), "not bound to a group snapshot") {
			t.Errorf("expected 'not bound' error, got: %v", err)
		}
	})

	t.Run("empty VolumeHandles", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-2"},
			Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
				VolumeGroupSnapshotRef: corev1.ObjectReference{UID: "uid-123"},
				Source:                 crdv1beta2.VolumeGroupSnapshotContentSource{VolumeHandles: nil},
			},
		}
		_, _, _, _, _, err := handler.CreateGroupSnapshot(content, nil, nil)
		if err == nil {
			t.Fatal("expected error when VolumeHandles is empty")
		}
		if !contains(err.Error(), "PVCs to be snapshotted not found") {
			t.Errorf("expected 'PVCs to be snapshotted' error, got: %v", err)
		}
	})

	t.Run("makeGroupSnapshotName error with empty UID", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-3"},
			Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
				VolumeGroupSnapshotRef: corev1.ObjectReference{UID: ""},
				Source:                 crdv1beta2.VolumeGroupSnapshotContentSource{VolumeHandles: []string{"vol-1"}},
			},
		}
		_, _, _, _, _, err := handler.CreateGroupSnapshot(content, nil, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("success", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-4"},
			Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
				VolumeGroupSnapshotRef: corev1.ObjectReference{UID: "12345678-1234"},
				Source:                 crdv1beta2.VolumeGroupSnapshotContentSource{VolumeHandles: []string{"vol-1", "vol-2"}},
			},
		}
		driverName, groupID, snapshots, ts, ready, err := handler.CreateGroupSnapshot(content, map[string]string{"key": "val"}, nil)
		if err != nil {
			t.Fatalf("CreateGroupSnapshot failed: %v", err)
		}
		if driverName != "driver" || groupID != "group-snap-id" {
			t.Errorf("unexpected driverName=%q groupID=%q", driverName, groupID)
		}
		if ts.IsZero() {
			t.Error("expected non-zero timestamp")
		}
		if !ready {
			t.Error("expected readyToUse true")
		}
		_ = snapshots
	})

	t.Run("makeGroupSnapshotName with UUIDLength -1", func(t *testing.T) {
		h := NewCSIHandler(nil, &fakeGroupSnapshotter{}, 5*time.Second, "snap", 8, "grp-snap", -1)
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-5"},
			Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
				VolumeGroupSnapshotRef: corev1.ObjectReference{UID: "my-uid-with-dashes"},
				Source:                 crdv1beta2.VolumeGroupSnapshotContentSource{VolumeHandles: []string{"vol-1"}},
			},
		}
		_, groupID, _, _, _, err := h.CreateGroupSnapshot(content, nil, nil)
		if err != nil {
			t.Fatalf("CreateGroupSnapshot failed: %v", err)
		}
		if groupID != "group-snap-id" {
			t.Errorf("unexpected groupID: %q", groupID)
		}
	})
}

// TestDeleteGroupSnapshot validates DeleteGroupSnapshot error paths and success path.
func TestDeleteGroupSnapshot(t *testing.T) {
	handler := newCSIHandlerWithFakeGroupSnapshotter(&fakeGroupSnapshotter{})

	t.Run("empty snapshotIDs", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-del-1"},
			Status:     &crdv1beta2.VolumeGroupSnapshotContentStatus{VolumeGroupSnapshotHandle: ptrString("handle-1")},
		}
		err := handler.DeleteGroupSnapshot(content, nil, nil)
		if err == nil {
			t.Fatal("expected error when snapshotIDs is empty")
		}
		if !contains(err.Error(), "No snapshots found") {
			t.Errorf("expected 'No snapshots' error, got: %v", err)
		}
	})

	t.Run("handle from Status", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-del-2"},
			Status:     &crdv1beta2.VolumeGroupSnapshotContentStatus{VolumeGroupSnapshotHandle: ptrString("handle-from-status")},
		}
		err := handler.DeleteGroupSnapshot(content, []string{"snap-1"}, nil)
		if err != nil {
			t.Errorf("DeleteGroupSnapshot failed: %v", err)
		}
	})

	t.Run("handle from GroupSnapshotHandles", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-del-3"},
			Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
				Source: crdv1beta2.VolumeGroupSnapshotContentSource{
					GroupSnapshotHandles: &crdv1beta2.GroupSnapshotHandles{
						VolumeGroupSnapshotHandle: "handle-from-spec",
						VolumeSnapshotHandles:     []string{"snap-1"},
					},
				},
			},
		}
		err := handler.DeleteGroupSnapshot(content, []string{"snap-1"}, nil)
		if err != nil {
			t.Errorf("DeleteGroupSnapshot failed: %v", err)
		}
	})

	t.Run("missing handle", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-del-4"},
			Spec:       crdv1beta2.VolumeGroupSnapshotContentSpec{Source: crdv1beta2.VolumeGroupSnapshotContentSource{}},
		}
		err := handler.DeleteGroupSnapshot(content, []string{"snap-1"}, nil)
		if err == nil {
			t.Fatal("expected error when handle is missing")
		}
		if !contains(err.Error(), "groupsnapshotHandle is missing") {
			t.Errorf("expected 'groupsnapshotHandle is missing', got: %v", err)
		}
	})

	t.Run("groupSnapshotter returns error", func(t *testing.T) {
		handlerErr := newCSIHandlerWithFakeGroupSnapshotter(&fakeGroupSnapshotter{deleteGroupSnapshotErr: errors.New("driver error")})
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-del-5"},
			Status:     &crdv1beta2.VolumeGroupSnapshotContentStatus{VolumeGroupSnapshotHandle: ptrString("h")},
		}
		err := handlerErr.DeleteGroupSnapshot(content, []string{"s1"}, nil)
		if err == nil {
			t.Fatal("expected error from driver")
		}
		if !contains(err.Error(), "driver error") {
			t.Errorf("expected driver error, got: %v", err)
		}
	})
}

// TestGetGroupSnapshotStatus validates GetGroupSnapshotStatus error paths and success path.
func TestGetGroupSnapshotStatus(t *testing.T) {
	handler := newCSIHandlerWithFakeGroupSnapshotter(&fakeGroupSnapshotter{})

	t.Run("empty snapshotIDs", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-status-1"},
			Status:     &crdv1beta2.VolumeGroupSnapshotContentStatus{VolumeGroupSnapshotHandle: ptrString("h")},
		}
		_, _, err := handler.GetGroupSnapshotStatus(content, nil, nil)
		if err == nil {
			t.Fatal("expected error when snapshotIDs is empty")
		}
		if !contains(err.Error(), "No snapshots found") {
			t.Errorf("expected 'No snapshots' error, got: %v", err)
		}
	})

	t.Run("handle from Status", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-status-2"},
			Status:     &crdv1beta2.VolumeGroupSnapshotContentStatus{VolumeGroupSnapshotHandle: ptrString("handle-status")},
		}
		ready, ts, err := handler.GetGroupSnapshotStatus(content, []string{"snap-1"}, nil)
		if err != nil {
			t.Fatalf("GetGroupSnapshotStatus failed: %v", err)
		}
		if !ready || ts.IsZero() {
			t.Errorf("ready=%v ts=%v", ready, ts)
		}
	})

	t.Run("handle from GroupSnapshotHandles", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-status-3"},
			Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
				Source: crdv1beta2.VolumeGroupSnapshotContentSource{
					GroupSnapshotHandles: &crdv1beta2.GroupSnapshotHandles{
						VolumeGroupSnapshotHandle: "handle-spec",
						VolumeSnapshotHandles:     []string{"snap-1"},
					},
				},
			},
		}
		ready, _, err := handler.GetGroupSnapshotStatus(content, []string{"snap-1"}, nil)
		if err != nil {
			t.Fatalf("GetGroupSnapshotStatus failed: %v", err)
		}
		if !ready {
			t.Error("expected ready true")
		}
	})

	t.Run("missing handle", func(t *testing.T) {
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-status-4"},
			Spec:       crdv1beta2.VolumeGroupSnapshotContentSpec{Source: crdv1beta2.VolumeGroupSnapshotContentSource{}},
		}
		_, _, err := handler.GetGroupSnapshotStatus(content, []string{"snap-1"}, nil)
		if err == nil {
			t.Fatal("expected error when handle is missing")
		}
		if !contains(err.Error(), "groupSnapshotHandle is missing") {
			t.Errorf("expected 'groupSnapshotHandle is missing', got: %v", err)
		}
	})

	t.Run("groupSnapshotter returns error", func(t *testing.T) {
		handlerErr := newCSIHandlerWithFakeGroupSnapshotter(&fakeGroupSnapshotter{
			getGroupSnapshotStatus: func() (bool, time.Time, error) { return false, time.Time{}, errors.New("driver list error") },
		})
		content := &crdv1beta2.VolumeGroupSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: "gsc-status-5"},
			Status:     &crdv1beta2.VolumeGroupSnapshotContentStatus{VolumeGroupSnapshotHandle: ptrString("h")},
		}
		_, _, err := handlerErr.GetGroupSnapshotStatus(content, []string{"s1"}, nil)
		if err == nil {
			t.Fatal("expected error from driver")
		}
		if !contains(err.Error(), "driver list error") {
			t.Errorf("expected driver error, got: %v", err)
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
