package sidecar_controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	crdv1beta2 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta2"
	v1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	groupsnapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumegroupsnapshot/v1beta2"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

type fakeContentLister struct {
}

func (f *fakeContentLister) List(selector labels.Selector) (ret []*v1.VolumeSnapshotContent, err error) {
	return nil, nil
}
func (f *fakeContentLister) Get(name string) (*v1.VolumeSnapshotContent, error) {
	return &v1.VolumeSnapshotContent{}, nil
}

func TestDeleteCSIGroupSnapshotOperation(t *testing.T) {
	ctrl := &csiSnapshotSideCarController{
		contentLister: &fakeContentLister{},
		handler:       &csiHandler{},
		eventRecorder: &record.FakeRecorder{},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("deleteCSIGroupSnapshotOperation() panicked with: %v", r)
		}
	}()
	err := ctrl.deleteCSIGroupSnapshotOperation(nil)
	if err == nil {
		t.Errorf("expected deleteCSIGroupSnapshotOperation to return error when groupsnapshotContent is nil: %v", err)
	}
	gsc := crdv1beta2.VolumeGroupSnapshotContent{
		Status: &crdv1beta2.VolumeGroupSnapshotContentStatus{
			VolumeSnapshotInfoList: []crdv1beta2.VolumeSnapshotInfo{
				{
					VolumeHandle:   "test-pv",
					SnapshotHandle: "test-vsc",
				},
			},
		},
	}
	err = ctrl.deleteCSIGroupSnapshotOperation(&gsc)
	if err == nil {
		t.Errorf("expected deleteCSIGroupSnapshotOperation to return error when groupsnapshotContent is empty: %v", err)
	}
}

// fakeGroupSnapshotHandler implements Handler for group snapshot tests.
type fakeGroupSnapshotHandler struct {
	createGroupSnapshotErr    error
	createGroupSnapshotResult func() (driverName, groupSnapshotID string, snapshots []*csi.Snapshot, creationTime time.Time, readyToUse bool, err error)
	getGroupSnapshotStatusErr error
	getGroupSnapshotStatus    func() (readyToUse bool, creationTime time.Time, err error)
	deleteGroupSnapshotErr    error
}

func (f *fakeGroupSnapshotHandler) CreateSnapshot(_ *v1.VolumeSnapshotContent, _, _ map[string]string) (string, string, time.Time, int64, bool, error) {
	return "", "", time.Time{}, 0, false, errors.NewServiceUnavailable("not implemented")
}
func (f *fakeGroupSnapshotHandler) DeleteSnapshot(_ *v1.VolumeSnapshotContent, _ map[string]string) error {
	return errors.NewServiceUnavailable("not implemented")
}
func (f *fakeGroupSnapshotHandler) GetSnapshotStatus(_ *v1.VolumeSnapshotContent, _ map[string]string) (bool, time.Time, int64, string, error) {
	return false, time.Time{}, 0, "", errors.NewServiceUnavailable("not implemented")
}
func (f *fakeGroupSnapshotHandler) CreateGroupSnapshot(_ *crdv1beta2.VolumeGroupSnapshotContent, _ map[string]string, _ map[string]string) (string, string, []*csi.Snapshot, time.Time, bool, error) {
	if f.createGroupSnapshotErr != nil {
		return "", "", nil, time.Time{}, false, f.createGroupSnapshotErr
	}
	if f.createGroupSnapshotResult != nil {
		return f.createGroupSnapshotResult()
	}
	return "driver", "group-id", nil, time.Now(), true, nil
}
func (f *fakeGroupSnapshotHandler) GetGroupSnapshotStatus(_ *crdv1beta2.VolumeGroupSnapshotContent, _ []string, _ map[string]string) (bool, time.Time, error) {
	if f.getGroupSnapshotStatusErr != nil {
		return false, time.Time{}, f.getGroupSnapshotStatusErr
	}
	if f.getGroupSnapshotStatus != nil {
		return f.getGroupSnapshotStatus()
	}
	return true, time.Now(), nil
}
func (f *fakeGroupSnapshotHandler) DeleteGroupSnapshot(_ *crdv1beta2.VolumeGroupSnapshotContent, _ []string, _ map[string]string) error {
	return f.deleteGroupSnapshotErr
}

// TestCreateGroupSnapshotErrorPath tests createGroupSnapshot when createGroupSnapshotWrapper returns error.
func TestCreateGroupSnapshotErrorPath(t *testing.T) {
	content := newGroupSnapshotContent(
		"create-err", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(newVolumeGroupSnapshotClass("class-a", mockDriverName))
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		contentStore:              cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		groupSnapshotContentStore: store,
		groupSnapshotClassLister:  groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer),
		handler:                   &fakeGroupSnapshotHandler{createGroupSnapshotErr: fmt.Errorf("csi create failed")},
		eventRecorder:             record.NewFakeRecorder(10),
	}

	err := ctrl.createGroupSnapshot(content)
	if err == nil {
		t.Error("createGroupSnapshot expected error when handler returns error")
	}
	// Error path should call updateGroupSnapshotContentErrorStatusWithEvent
	updated, _ := client.GroupsnapshotV1beta2().VolumeGroupSnapshotContents().Get(context.Background(), "create-err", metav1.GetOptions{})
	if updated.Status == nil || updated.Status.Error == nil {
		t.Error("expected error status to be set on content after createGroupSnapshot failure")
	}
}

// TestCheckandUpdateGroupSnapshotContentStatusErrorPath tests checkandUpdateGroupSnapshotContentStatus when operation fails.
func TestCheckandUpdateGroupSnapshotContentStatusErrorPath(t *testing.T) {
	content := newGroupSnapshotContentWithHandles(
		"check-err", "uid", "snap", testNamespace,
		"group-h", []string{"snap-1"}, "class-a",
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(newVolumeGroupSnapshotClass("class-a", mockDriverName))
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		contentStore:              cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		groupSnapshotContentStore: store,
		groupSnapshotClassLister:  groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer),
		handler:                   &fakeGroupSnapshotHandler{getGroupSnapshotStatusErr: fmt.Errorf("get status failed")},
		eventRecorder:             record.NewFakeRecorder(10),
	}

	err := ctrl.checkandUpdateGroupSnapshotContentStatus(content)
	if err == nil {
		t.Error("checkandUpdateGroupSnapshotContentStatus expected error when GetGroupSnapshotStatus fails")
	}
	updated, _ := client.GroupsnapshotV1beta2().VolumeGroupSnapshotContents().Get(context.Background(), "check-err", metav1.GetOptions{})
	if updated.Status == nil || updated.Status.Error == nil {
		t.Error("expected error status to be set after checkandUpdateGroupSnapshotContentStatus failure")
	}
}

// TestDeleteCSIGroupSnapshotOperationSuccess tests the success path: DeleteGroupSnapshot succeeds,
// clearGroupSnapshotContentStatus, then updateGroupSnapshotContentInInformerCache.
func TestDeleteCSIGroupSnapshotOperationSuccess(t *testing.T) {
	content := newGroupSnapshotContent(
		"delete-ok", "uid", "snap", testNamespace,
		"group-h", "class-a", []string{"vol-1"},
		v1.VolumeSnapshotContentDelete, nil, true, &timeNowMetav1,
	)
	content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
		VolumeGroupSnapshotHandle: ptrString("group-handle-1"),
		VolumeSnapshotInfoList: []crdv1beta2.VolumeSnapshotInfo{
			{VolumeHandle: "vol-1", SnapshotHandle: "snap-1"},
		},
	}
	metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingDeleted, "yes")
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	_ = store.Add(content)
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(newVolumeGroupSnapshotClass("class-a", mockDriverName))
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		groupSnapshotContentStore: store,
		groupSnapshotClassLister:  groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer),
		handler:                   &fakeGroupSnapshotHandler{}, // DeleteGroupSnapshot returns nil
		eventRecorder:             record.NewFakeRecorder(10),
	}
	err := ctrl.deleteCSIGroupSnapshotOperation(content)
	if err != nil {
		t.Fatalf("deleteCSIGroupSnapshotOperation failed: %v", err)
	}
	updated, _ := client.GroupsnapshotV1beta2().VolumeGroupSnapshotContents().Get(context.Background(), "delete-ok", metav1.GetOptions{})
	if updated.Status != nil && updated.Status.VolumeGroupSnapshotHandle != nil {
		t.Error("expected VolumeGroupSnapshotHandle to be cleared after successful delete")
	}
}

// TestUpdateGroupSnapshotContentStatusExistingStatus tests updateGroupSnapshotContentStatus when Status already exists.
func TestUpdateGroupSnapshotContentStatusExistingStatus(t *testing.T) {
	content := newGroupSnapshotContent(
		"update-existing", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
		VolumeGroupSnapshotHandle: ptrString("old-handle"),
		ReadyToUse:                ptr(false),
		CreationTime:              &metav1.Time{Time: metav1.Now().Time},
	}
	client := fake.NewSimpleClientset(content)
	ctrl := &csiSnapshotSideCarController{clientset: client}
	handle := "new-handle"
	ready := true
	created := metav1.NewTime(metav1.Now().Time)
	snapList := []*csi.Snapshot{
		{SnapshotId: "snap-1", SourceVolumeId: "vol-1", SizeBytes: 1024, ReadyToUse: true},
	}
	got, err := ctrl.updateGroupSnapshotContentStatus(content, handle, ready, created, snapList)
	if err != nil {
		t.Fatalf("updateGroupSnapshotContentStatus failed: %v", err)
	}
	if got.Status == nil || len(got.Status.VolumeSnapshotInfoList) == 0 {
		t.Error("expected VolumeSnapshotInfoList to be set when snapshotList provided")
	}
}

// TestUpdateGroupSnapshotContentStatusNoUpdate tests when nothing changes (updated=false).
func TestUpdateGroupSnapshotContentStatusNoUpdate(t *testing.T) {
	handle := "same-handle"
	ready := true
	created := metav1.NewTime(metav1.Now().Time)
	content := newGroupSnapshotContent(
		"no-update", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
		VolumeGroupSnapshotHandle: &handle,
		ReadyToUse:                &ready,
		CreationTime:              &created,
	}
	client := fake.NewSimpleClientset(content)
	ctrl := &csiSnapshotSideCarController{clientset: client}
	got, err := ctrl.updateGroupSnapshotContentStatus(content, handle, ready, created, nil)
	if err != nil {
		t.Fatalf("updateGroupSnapshotContentStatus failed: %v", err)
	}
	if got != content && (got.Status == nil || got.Status.VolumeGroupSnapshotHandle == nil) {
		t.Error("expected same object when no update needed")
	}
}

// TestCheckandUpdateGroupSnapshotContentStatusOperationSuccess tests GroupSnapshotHandles path when GetGroupSnapshotStatus succeeds.
func TestCheckandUpdateGroupSnapshotContentStatusOperationSuccess(t *testing.T) {
	content := newGroupSnapshotContentWithHandles(
		"check-ok", "uid", "snap", testNamespace,
		"group-h", []string{"snap-1"}, "", // no class so no credentials path
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	content.Spec.VolumeGroupSnapshotClassName = nil
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		groupSnapshotContentStore: store,
		handler:                   &fakeGroupSnapshotHandler{}, // GetGroupSnapshotStatus returns (true, time.Now(), nil)
		eventRecorder:             record.NewFakeRecorder(10),
	}
	got, err := ctrl.checkandUpdateGroupSnapshotContentStatusOperation(content)
	if err != nil {
		t.Fatalf("checkandUpdateGroupSnapshotContentStatusOperation failed: %v", err)
	}
	if got.Status == nil || got.Status.VolumeGroupSnapshotHandle == nil {
		t.Error("expected status to be updated with group snapshot handle")
	}
}

// TestCheckandUpdateGroupSnapshotContentStatusSuccess tests the outer function success path (store update).
func TestCheckandUpdateGroupSnapshotContentStatusSuccess(t *testing.T) {
	content := newGroupSnapshotContentWithHandles(
		"check-status-ok", "uid", "snap", testNamespace,
		"group-h", []string{"snap-1"}, "",
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	content.Spec.VolumeGroupSnapshotClassName = nil
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		groupSnapshotContentStore: store,
		handler:                   &fakeGroupSnapshotHandler{},
		eventRecorder:             record.NewFakeRecorder(10),
	}
	err := ctrl.checkandUpdateGroupSnapshotContentStatus(content)
	if err != nil {
		t.Fatalf("checkandUpdateGroupSnapshotContentStatus failed: %v", err)
	}
	_, found, _ := store.GetByKey("check-status-ok")
	if !found {
		t.Error("expected content to be in store after successful checkandUpdate")
	}
}

// TestCreateGroupSnapshotWrapperSuccess tests full success path: setAnn, CreateGroupSnapshot, updateStatus, removeAnn.
func TestCreateGroupSnapshotWrapperSuccess(t *testing.T) {
	content := newGroupSnapshotContent(
		"wrapper-ok", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	class := newVolumeGroupSnapshotClass("class-a", mockDriverName)
	class.Parameters = nil
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(class)
	client := fake.NewSimpleClientset(content)
	contentStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	groupStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		contentStore:              contentStore,
		groupSnapshotContentStore: groupStore,
		groupSnapshotClassLister:  groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer),
		handler:                   &fakeGroupSnapshotHandler{},
		eventRecorder:             record.NewFakeRecorder(10),
		extraCreateMetadata:       true,
	}
	got, err := ctrl.createGroupSnapshotWrapper(content)
	if err != nil {
		t.Fatalf("createGroupSnapshotWrapper failed: %v", err)
	}
	if got.Status == nil || got.Status.VolumeGroupSnapshotHandle == nil {
		t.Error("expected status to be set after successful create")
	}
	if metav1.HasAnnotation(got.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated) {
		t.Error("expected AnnVolumeGroupSnapshotBeingCreated to be removed after success")
	}
}

// TestCreateGroupSnapshotWrapperFinalError tests that on a final CSI error the annotation is removed.
func TestCreateGroupSnapshotWrapperFinalError(t *testing.T) {
	content := newGroupSnapshotContent(
		"wrapper-final-err", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	class := newVolumeGroupSnapshotClass("class-a", mockDriverName)
	class.Parameters = nil
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(class)
	client := fake.NewSimpleClientset(content)
	contentStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	groupStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		contentStore:              contentStore,
		groupSnapshotContentStore: groupStore,
		groupSnapshotClassLister:  groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer),
		handler:                   &fakeGroupSnapshotHandler{createGroupSnapshotErr: status.Error(codes.InvalidArgument, "invalid request")},
		eventRecorder:             record.NewFakeRecorder(10),
	}
	_, err := ctrl.createGroupSnapshotWrapper(content)
	if err == nil {
		t.Fatal("expected createGroupSnapshotWrapper to fail")
	}
	updated, _ := client.GroupsnapshotV1beta2().VolumeGroupSnapshotContents().Get(context.Background(), "wrapper-final-err", metav1.GetOptions{})
	if metav1.HasAnnotation(updated.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated) {
		t.Error("expected AnnVolumeGroupSnapshotBeingCreated to be removed on final error")
	}
}

// TestCreateGroupSnapshotSuccess tests createGroupSnapshot when wrapper succeeds.
func TestCreateGroupSnapshotSuccess(t *testing.T) {
	content := newGroupSnapshotContent(
		"create-ok", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		v1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	class := newVolumeGroupSnapshotClass("class-a", mockDriverName)
	class.Parameters = nil
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(class)
	client := fake.NewSimpleClientset(content)
	contentStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	groupStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		contentStore:              contentStore,
		groupSnapshotContentStore: groupStore,
		groupSnapshotClassLister:  groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer),
		handler:                   &fakeGroupSnapshotHandler{},
		eventRecorder:             record.NewFakeRecorder(10),
	}
	err := ctrl.createGroupSnapshot(content)
	if err != nil {
		t.Fatalf("createGroupSnapshot failed: %v", err)
	}
}
