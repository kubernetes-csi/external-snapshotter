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

package group_snapshotter

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeCSIServer implements Identity and GroupController for in-process tests.
type fakeCSIServer struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedGroupControllerServer

	driverName       string
	getPluginInfoErr error

	createGroupSnapshotResp *csi.CreateVolumeGroupSnapshotResponse
	createGroupSnapshotErr  error

	deleteGroupSnapshotErr error

	getGroupSnapshotResp *csi.GetVolumeGroupSnapshotResponse
	getGroupSnapshotErr  error
}

func (f *fakeCSIServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	if f.getPluginInfoErr != nil {
		return nil, f.getPluginInfoErr
	}
	name := f.driverName
	if name == "" {
		name = "test-driver"
	}
	return &csi.GetPluginInfoResponse{Name: name}, nil
}

func (f *fakeCSIServer) CreateVolumeGroupSnapshot(ctx context.Context, req *csi.CreateVolumeGroupSnapshotRequest) (*csi.CreateVolumeGroupSnapshotResponse, error) {
	if f.createGroupSnapshotErr != nil {
		return nil, f.createGroupSnapshotErr
	}
	if f.createGroupSnapshotResp != nil {
		return f.createGroupSnapshotResp, nil
	}
	// default success response
	ts := timestamppb.New(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	return &csi.CreateVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: "group-snap-123",
			Snapshots:       []*csi.Snapshot{{SnapshotId: "snap-1"}, {SnapshotId: "snap-2"}},
			CreationTime:    ts,
			ReadyToUse:      true,
		},
	}, nil
}

func (f *fakeCSIServer) DeleteVolumeGroupSnapshot(ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest) (*csi.DeleteVolumeGroupSnapshotResponse, error) {
	if f.deleteGroupSnapshotErr != nil {
		return nil, f.deleteGroupSnapshotErr
	}
	return &csi.DeleteVolumeGroupSnapshotResponse{}, nil
}

func (f *fakeCSIServer) GetVolumeGroupSnapshot(ctx context.Context, req *csi.GetVolumeGroupSnapshotRequest) (*csi.GetVolumeGroupSnapshotResponse, error) {
	if f.getGroupSnapshotErr != nil {
		return nil, f.getGroupSnapshotErr
	}
	if f.getGroupSnapshotResp != nil {
		return f.getGroupSnapshotResp, nil
	}
	ts := timestamppb.New(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))
	return &csi.GetVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: req.GroupSnapshotId,
			CreationTime:    ts,
			ReadyToUse:      true,
		},
	}, nil
}

func (f *fakeCSIServer) GroupControllerGetCapabilities(ctx context.Context, req *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	return &csi.GroupControllerGetCapabilitiesResponse{}, nil
}

func startFakeCSI(t *testing.T, fake *fakeCSIServer) (conn *grpc.ClientConn, cleanup func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	csi.RegisterIdentityServer(srv, fake)
	csi.RegisterGroupControllerServer(srv, fake)
	go func() {
		_ = srv.Serve(lis)
	}()
	addr := lis.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err = grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		srv.Stop()
		_ = lis.Close()
		t.Fatalf("failed to dial: %v", err)
	}
	cleanup = func() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return conn, cleanup
}

func TestNewGroupSnapshotter(t *testing.T) {
	conn, cleanup := startFakeCSI(t, &fakeCSIServer{})
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	if gs == nil {
		t.Fatal("NewGroupSnapshotter returned nil")
	}
}

func TestCreateGroupSnapshot_Success(t *testing.T) {
	fake := &fakeCSIServer{driverName: "my-driver"}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	driverName, groupID, snapshots, creationTime, ready, err := gs.CreateGroupSnapshot(ctx, "my-name", []string{"vol-1", "vol-2"}, map[string]string{"key": "val"}, nil)
	if err != nil {
		t.Fatalf("CreateGroupSnapshot: %v", err)
	}
	if driverName != "my-driver" {
		t.Errorf("driverName = %q, want my-driver", driverName)
	}
	if groupID != "group-snap-123" {
		t.Errorf("groupSnapshotId = %q, want group-snap-123", groupID)
	}
	if len(snapshots) != 2 {
		t.Errorf("len(snapshots) = %d, want 2", len(snapshots))
	}
	if creationTime.Year() != 2024 || creationTime.Month() != 1 {
		t.Errorf("creationTime = %v", creationTime)
	}
	if !ready {
		t.Error("readyToUse = false, want true")
	}
}

func TestCreateGroupSnapshot_GetDriverNameError(t *testing.T) {
	fake := &fakeCSIServer{getPluginInfoErr: errors.New("identity error")}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	_, _, _, _, _, err := gs.CreateGroupSnapshot(ctx, "name", []string{"vol-1"}, nil, nil)
	if err == nil {
		t.Fatal("expected error from GetDriverName")
	}
	if !strings.Contains(err.Error(), "identity") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateGroupSnapshot_CreateRPCError(t *testing.T) {
	fake := &fakeCSIServer{createGroupSnapshotErr: status.Error(codes.Internal, "create failed")}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	_, _, _, _, _, err := gs.CreateGroupSnapshot(ctx, "name", []string{"vol-1"}, nil, nil)
	if err == nil {
		t.Fatal("expected error from CreateVolumeGroupSnapshot")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestCreateGroupSnapshot_CustomResponse(t *testing.T) {
	ts := timestamppb.New(time.Date(2025, 6, 1, 12, 30, 0, 0, time.UTC))
	fake := &fakeCSIServer{
		driverName: "custom-driver",
		createGroupSnapshotResp: &csi.CreateVolumeGroupSnapshotResponse{
			GroupSnapshot: &csi.VolumeGroupSnapshot{
				GroupSnapshotId: "custom-id",
				Snapshots:       []*csi.Snapshot{{SnapshotId: "s1"}},
				CreationTime:    ts,
				ReadyToUse:      false,
			},
		},
	}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	driverName, groupID, snapshots, creationTime, ready, err := gs.CreateGroupSnapshot(ctx, "name", []string{"v1"}, nil, nil)
	if err != nil {
		t.Fatalf("CreateGroupSnapshot: %v", err)
	}
	if driverName != "custom-driver" {
		t.Errorf("driverName = %q", driverName)
	}
	if groupID != "custom-id" {
		t.Errorf("groupSnapshotId = %q", groupID)
	}
	if len(snapshots) != 1 || snapshots[0].SnapshotId != "s1" {
		t.Errorf("snapshots = %v", snapshots)
	}
	if creationTime.Year() != 2025 || creationTime.Month() != 6 {
		t.Errorf("creationTime = %v", creationTime)
	}
	if ready {
		t.Error("readyToUse = true, want false")
	}
}

func TestDeleteGroupSnapshot_Success(t *testing.T) {
	fake := &fakeCSIServer{}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	err := gs.DeleteGroupSnapshot(ctx, "group-id", []string{"snap-1", "snap-2"}, map[string]string{"secret": "val"})
	if err != nil {
		t.Fatalf("DeleteGroupSnapshot: %v", err)
	}
}

func TestDeleteGroupSnapshot_Error(t *testing.T) {
	fake := &fakeCSIServer{deleteGroupSnapshotErr: status.Error(codes.NotFound, "not found")}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	err := gs.DeleteGroupSnapshot(ctx, "group-id", []string{"snap-1"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestGetGroupSnapshotStatus_Success(t *testing.T) {
	fake := &fakeCSIServer{}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	ready, creationTime, err := gs.GetGroupSnapshotStatus(ctx, "group-id", []string{"snap-1"}, nil)
	if err != nil {
		t.Fatalf("GetGroupSnapshotStatus: %v", err)
	}
	if !ready {
		t.Error("ready = false, want true")
	}
	if creationTime.Year() != 2024 {
		t.Errorf("creationTime = %v", creationTime)
	}
}

func TestGetGroupSnapshotStatus_CustomResponse(t *testing.T) {
	ts := timestamppb.New(time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
	fake := &fakeCSIServer{
		getGroupSnapshotResp: &csi.GetVolumeGroupSnapshotResponse{
			GroupSnapshot: &csi.VolumeGroupSnapshot{
				GroupSnapshotId: "gid",
				CreationTime:    ts,
				ReadyToUse:      false,
			},
		},
	}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	ready, creationTime, err := gs.GetGroupSnapshotStatus(ctx, "gid", []string{"s1"}, nil)
	if err != nil {
		t.Fatalf("GetGroupSnapshotStatus: %v", err)
	}
	if ready {
		t.Error("ready = true, want false")
	}
	if creationTime.Year() != 2026 || creationTime.Month() != 3 {
		t.Errorf("creationTime = %v", creationTime)
	}
}

func TestGetGroupSnapshotStatus_Error(t *testing.T) {
	fake := &fakeCSIServer{getGroupSnapshotErr: status.Error(codes.NotFound, "group snapshot not found")}
	conn, cleanup := startFakeCSI(t, fake)
	defer cleanup()
	gs := NewGroupSnapshotter(conn)
	ctx := context.Background()

	ready, creationTime, err := gs.GetGroupSnapshotStatus(ctx, "group-id", []string{"snap-1"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
	if ready {
		t.Error("ready should be false on error")
	}
	if !creationTime.IsZero() {
		t.Errorf("creationTime should be zero on error, got %v", creationTime)
	}
}
