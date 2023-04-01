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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	"google.golang.org/grpc"
	klog "k8s.io/klog/v2"
	"time"
)

// GroupSnapshotter implements CreateGroupSnapshot/DeleteGroupSnapshot operations against a remote CSI driver.
type GroupSnapshotter interface {
	// CreateGroupSnapshot creates a snapshot for a volume
	CreateGroupSnapshot(ctx context.Context, groupSnapshotName string, volumeIDs []string, parameters map[string]string, snapshotterCredentials map[string]string) (driverName string, snapshotId string, snapshots []*csi.Snapshot, timestamp time.Time, readyToUse bool, err error)

	// DeleteGroupSnapshot deletes a snapshot from a volume
	DeleteGroupSnapshot(ctx context.Context, snapshotID string, snapshotterCredentials map[string]string) (err error)

	// GetGroupSnapshotStatus returns if a snapshot is ready to use, creation time, and restore size.
	GetGroupSnapshotStatus(ctx context.Context, snapshotID string, snapshotterListCredentials map[string]string) (bool, time.Time, error)
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
	klog.V(5).Infof("CSI CreateSnapshot: %s", groupSnapshotName)
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

	klog.V(5).Infof("CSI CreateSnapshot: %s driver name [%s] snapshot ID [%s] time stamp [%v] snapshots [%v] readyToUse [%v]", groupSnapshotName, driverName, rsp.GroupSnapshot.GroupSnapshotId, rsp.GroupSnapshot.CreationTime, rsp.GroupSnapshot.Snapshots, rsp.GroupSnapshot.ReadyToUse)
	creationTime, err := ptypes.Timestamp(rsp.GroupSnapshot.CreationTime)
	if err != nil {
		return "", "", nil, time.Time{}, false, err
	}
	return driverName, rsp.GroupSnapshot.GroupSnapshotId, rsp.GroupSnapshot.Snapshots, creationTime, rsp.GroupSnapshot.ReadyToUse, nil

}

func (gs *groupSnapshot) DeleteGroupSnapshot(ctx context.Context, snapshotID string, snapshotterCredentials map[string]string) error {
	// TODO: Implement DeleteGroupSnapshot
	return nil
}

func (gs *groupSnapshot) GetGroupSnapshotStatus(ctx context.Context, snapshotID string, snapshotterListCredentials map[string]string) (bool, time.Time, error) {
	// TODO: Implement GetGroupSnapshotStatus
	return true, time.Now(), nil
}
