package sidecar_controller

import (
	"testing"

	crdv1beta2 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta2"

	v1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"k8s.io/apimachinery/pkg/labels"
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
