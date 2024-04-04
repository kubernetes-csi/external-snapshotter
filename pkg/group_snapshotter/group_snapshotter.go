/*
Copyright 2023 The Kubernetes Authors.

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

package group_snapshotter

import (
	"context"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	"google.golang.org/grpc"
	klog "k8s.io/klog/v2"
)

// GroupSnapshotter implements CreateGroupSnapshot/DeleteGroupSnapshot operations against a CSI driver.
type GroupSnapshotter interface {
	// CreateGroupSnapshot creates a group snapshot for multiple volumes
	CreateGroupSnapshot(ctx context.Context, groupSnapshotName string, volumeIDs []string, parameters map[string]string, snapshotterCredentials map[string]string) (driverName string, groupSnapshotId string, snapshots []*csi.Snapshot, timestamp time.Time, readyToUse bool, err error)

	// DeleteGroupSnapshot deletes a group snapshot of multiple volumes
	DeleteGroupSnapshot(ctx context.Context, groupSnapshotID string, snapshotIDs []string, snapshotterCredentials map[string]string) (err error)

	// GetGroupSnapshotStatus returns if a group snapshot is ready to use, its creation time, etc
	GetGroupSnapshotStatus(ctx context.Context, groupSnapshotID string, snapshotIDs []string, snapshotterCredentials map[string]string) (bool, time.Time, error)
}

type groupSnapshot struct {
	conn *grpc.ClientConn
}

func NewGroupSnapshotter(conn *grpc.ClientConn) GroupSnapshotter {
	return &groupSnapshot{
		conn: conn,
	}
}

func (gs *groupSnapshot) CreateGroupSnapshot(ctx context.Context, groupSnapshotName string, volumeIDs []string, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, []*csi.Snapshot, time.Time, bool, error) {
	klog.V(5).Infof("CSI CreateGroupSnapshot: %s", groupSnapshotName)
	client := csi.NewGroupControllerClient(gs.conn)

	driverName, err := csirpc.GetDriverName(ctx, gs.conn)
	if err != nil {
		return "", "", nil, time.Time{}, false, err
	}

	req := csi.CreateVolumeGroupSnapshotRequest{
		Name:            groupSnapshotName,
		SourceVolumeIds: volumeIDs,
		Secrets:         snapshotterCredentials,
		Parameters:      parameters,
	}

	rsp, err := client.CreateVolumeGroupSnapshot(ctx, &req)
	if err != nil {
		return "", "", nil, time.Time{}, false, err
	}

	klog.V(5).Infof("CSI CreateGroupSnapshot: %s driver name [%s] group snapshot ID [%s] time stamp [%v] snapshots [%v] readyToUse [%v]", groupSnapshotName, driverName, rsp.GroupSnapshot.GroupSnapshotId, rsp.GroupSnapshot.CreationTime, rsp.GroupSnapshot.Snapshots, rsp.GroupSnapshot.ReadyToUse)
	creationTime := rsp.GroupSnapshot.CreationTime.AsTime()
	return driverName, rsp.GroupSnapshot.GroupSnapshotId, rsp.GroupSnapshot.Snapshots, creationTime, rsp.GroupSnapshot.ReadyToUse, nil

}

func (gs *groupSnapshot) DeleteGroupSnapshot(ctx context.Context, groupSnapshotID string, snapshotIds []string, snapshotterCredentials map[string]string) error {
	client := csi.NewGroupControllerClient(gs.conn)

	req := csi.DeleteVolumeGroupSnapshotRequest{
		Secrets:         snapshotterCredentials,
		GroupSnapshotId: groupSnapshotID,
		SnapshotIds:     snapshotIds,
	}

	_, err := client.DeleteVolumeGroupSnapshot(ctx, &req)
	if err != nil {
		return err
	}

	return nil
}

func (gs *groupSnapshot) GetGroupSnapshotStatus(ctx context.Context, groupSnapshotID string, snapshotIds []string, snapshotterCredentials map[string]string) (bool, time.Time, error) {
	klog.V(5).Infof("CSI GetGroupSnapshotStatus: %s", groupSnapshotID)
	client := csi.NewGroupControllerClient(gs.conn)

	req := csi.GetVolumeGroupSnapshotRequest{
		Secrets:         snapshotterCredentials,
		GroupSnapshotId: groupSnapshotID,
		SnapshotIds:     snapshotIds,
	}

	rsp, err := client.GetVolumeGroupSnapshot(ctx, &req)
	if err != nil {
		return false, time.Time{}, err
	}

	klog.V(5).Infof("CSI GetGroupSnapshot: group snapshot ID [%s] time stamp [%v] snapshots [%v] readyToUse [%v]", rsp.GroupSnapshot.GroupSnapshotId, rsp.GroupSnapshot.CreationTime, rsp.GroupSnapshot.Snapshots, rsp.GroupSnapshot.ReadyToUse)
	return rsp.GroupSnapshot.ReadyToUse, rsp.GroupSnapshot.CreationTime.AsTime(), nil
}
