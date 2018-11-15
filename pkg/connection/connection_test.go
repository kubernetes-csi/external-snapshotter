/*
Copyright 2018 The Kubernetes Authors.

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

package connection

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes"
	"github.com/kubernetes-csi/csi-test/driver"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	driverName = "foo/bar"
)

func createMockServer(t *testing.T) (*gomock.Controller, *driver.MockCSIDriver, *driver.MockIdentityServer, *driver.MockControllerServer, CSIConnection, error) {
	// Start the mock server
	mockController := gomock.NewController(t)
	identityServer := driver.NewMockIdentityServer(mockController)
	controllerServer := driver.NewMockControllerServer(mockController)
	drv := driver.NewMockCSIDriver(&driver.MockCSIDriverServers{
		Identity:   identityServer,
		Controller: controllerServer,
	})
	drv.Start()

	// Create a client connection to it
	addr := drv.Address()
	csiConn, err := New(addr, 10)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return mockController, drv, identityServer, controllerServer, csiConn, nil
}

func TestGetPluginInfo(t *testing.T) {
	tests := []struct {
		name        string
		output      *csi.GetPluginInfoResponse
		injectError bool
		expectError bool
	}{
		{
			name: "success",
			output: &csi.GetPluginInfoResponse{
				Name:          "csi/example",
				VendorVersion: "0.3.0",
				Manifest: map[string]string{
					"hello": "world",
				},
			},
			expectError: false,
		},
		{
			name:        "gRPC error",
			output:      nil,
			injectError: true,
			expectError: true,
		},
		{
			name: "empty name",
			output: &csi.GetPluginInfoResponse{
				Name: "",
			},
			expectError: true,
		},
	}

	mockController, driver, identityServer, _, csiConn, err := createMockServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer mockController.Finish()
	defer driver.Stop()
	defer csiConn.Close()

	for _, test := range tests {

		in := &csi.GetPluginInfoRequest{}

		out := test.output
		var injectedErr error = nil
		if test.injectError {
			injectedErr = fmt.Errorf("mock error")
		}

		// Setup expectation
		identityServer.EXPECT().GetPluginInfo(gomock.Any(), in).Return(out, injectedErr).Times(1)

		name, err := csiConn.GetDriverName(context.Background())
		if test.expectError && err == nil {
			t.Errorf("test %q: Expected error, got none", test.name)
		}
		if !test.expectError && err != nil {
			t.Errorf("test %q: got error: %v", test.name, err)
		}
		if err == nil && name != "csi/example" {
			t.Errorf("got unexpected name: %q", name)
		}
	}
}

func TestSupportsControllerCreateSnapshot(t *testing.T) {
	tests := []struct {
		name         string
		output       *csi.ControllerGetCapabilitiesResponse
		injectError  bool
		expectError  bool
		expectResult bool
	}{
		{
			name: "success",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
							},
						},
					},
				},
			},
			expectError:  false,
			expectResult: true,
		},
		{
			name:         "gRPC error",
			output:       nil,
			injectError:  true,
			expectError:  true,
			expectResult: false,
		},
		{
			name: "no create snapshot",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
							},
						},
					},
				},
			},
			expectError:  false,
			expectResult: false,
		},
		{
			name: "empty capability",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: nil,
					},
				},
			},
			expectError:  false,
			expectResult: false,
		},
		{
			name: "no capabilities",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{},
			},
			expectError:  false,
			expectResult: false,
		},
	}

	mockController, driver, _, controllerServer, csiConn, err := createMockServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer mockController.Finish()
	defer driver.Stop()
	defer csiConn.Close()

	for _, test := range tests {

		in := &csi.ControllerGetCapabilitiesRequest{}

		out := test.output
		var injectedErr error = nil
		if test.injectError {
			injectedErr = fmt.Errorf("mock error")
		}

		// Setup expectation
		controllerServer.EXPECT().ControllerGetCapabilities(gomock.Any(), in).Return(out, injectedErr).Times(1)

		ok, err := csiConn.SupportsControllerCreateSnapshot(context.Background())
		if test.expectError && err == nil {
			t.Errorf("test %q: Expected error, got none", test.name)
		}
		if !test.expectError && err != nil {
			t.Errorf("test %q: got error: %v", test.name, err)
		}
		if err == nil && test.expectResult != ok {
			t.Errorf("test fail expected result %t but got %t\n", test.expectResult, ok)
		}
	}
}

func TestSupportsControllerListSnapshots(t *testing.T) {
	tests := []struct {
		name         string
		output       *csi.ControllerGetCapabilitiesResponse
		injectError  bool
		expectError  bool
		expectResult bool
	}{
		{
			name: "success",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
							},
						},
					},
				},
			},
			expectError:  false,
			expectResult: true,
		},
		{
			name: "have create_delete_snapshot but no list snapshot ",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
							},
						},
					},
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
							},
						},
					},
				},
			},
			expectError:  false,
			expectResult: false,
		},
		{
			name:         "gRPC error",
			output:       nil,
			injectError:  true,
			expectError:  true,
			expectResult: false,
		},
		{
			name: "no list snapshot",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: &csi.ControllerServiceCapability_Rpc{
							Rpc: &csi.ControllerServiceCapability_RPC{
								Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
							},
						},
					},
				},
			},
			expectError:  false,
			expectResult: false,
		},
		{
			name: "empty capability",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{
					{
						Type: nil,
					},
				},
			},
			expectError:  false,
			expectResult: false,
		},
		{
			name: "no capabilities",
			output: &csi.ControllerGetCapabilitiesResponse{
				Capabilities: []*csi.ControllerServiceCapability{},
			},
			expectError:  false,
			expectResult: false,
		},
	}

	mockController, driver, _, controllerServer, csiConn, err := createMockServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer mockController.Finish()
	defer driver.Stop()
	defer csiConn.Close()

	for _, test := range tests {

		in := &csi.ControllerGetCapabilitiesRequest{}

		out := test.output
		var injectedErr error = nil
		if test.injectError {
			injectedErr = fmt.Errorf("mock error")
		}

		// Setup expectation
		controllerServer.EXPECT().ControllerGetCapabilities(gomock.Any(), in).Return(out, injectedErr).Times(1)

		ok, err := csiConn.SupportsControllerListSnapshots(context.Background())
		if test.expectError && err == nil {
			t.Errorf("test %q: Expected error, got none", test.name)
		}
		if !test.expectError && err != nil {
			t.Errorf("test %q: got error: %v", test.name, err)
		}
		if err == nil && test.expectResult != ok {
			t.Errorf("test fail expected result %t but got %t\n", test.expectResult, ok)
		}
	}
}

func TestCreateSnapshot(t *testing.T) {
	defaultName := "snapshot-test"
	defaultID := "testid"
	createTimestamp := ptypes.TimestampNow()
	createTime, err := ptypes.Timestamp(createTimestamp)
	if err != nil {
		t.Fatalf("Failed to convert timestamp to time: %v", err)
	}

	createSecrets := map[string]string{"foo": "bar"}
	defaultParameter := map[string]string{
		"param1": "value1",
		"param2": "value2",
	}

	csiVolume := FakeCSIVolume()
	volumeWithoutCSI := FakeVolume()

	defaultRequest := &csi.CreateSnapshotRequest{
		Name:           defaultName,
		SourceVolumeId: csiVolume.Spec.CSI.VolumeHandle,
	}

	attributesRequest := &csi.CreateSnapshotRequest{
		Name:           defaultName,
		Parameters:     defaultParameter,
		SourceVolumeId: csiVolume.Spec.CSI.VolumeHandle,
	}

	secretsRequest := &csi.CreateSnapshotRequest{
		Name:           defaultName,
		SourceVolumeId: csiVolume.Spec.CSI.VolumeHandle,
		Secrets:        createSecrets,
	}

	defaultResponse := &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     defaultID,
			SizeBytes:      1000,
			SourceVolumeId: csiVolume.Spec.CSI.VolumeHandle,
			CreationTime:   createTimestamp,
			ReadyToUse:     true,
		},
	}

	pluginInfoOutput := &csi.GetPluginInfoResponse{
		Name:          driverName,
		VendorVersion: "0.3.0",
		Manifest: map[string]string{
			"hello": "world",
		},
	}

	type snapshotResult struct {
		driverName string
		snapshotId string
		timestamp  int64
		size       int64
		readyToUse bool
	}

	result := &snapshotResult{
		size:       1000,
		driverName: driverName,
		snapshotId: defaultID,
		timestamp:  createTime.UnixNano(),
		readyToUse: true,
	}

	tests := []struct {
		name         string
		snapshotName string
		volume       *v1.PersistentVolume
		parameters   map[string]string
		secrets      map[string]string
		input        *csi.CreateSnapshotRequest
		output       *csi.CreateSnapshotResponse
		injectError  codes.Code
		expectError  bool
		expectResult *snapshotResult
	}{
		{
			name:         "success",
			snapshotName: defaultName,
			volume:       csiVolume,
			input:        defaultRequest,
			output:       defaultResponse,
			expectError:  false,
			expectResult: result,
		},
		{
			name:         "attributes",
			snapshotName: defaultName,
			volume:       csiVolume,
			parameters:   defaultParameter,
			input:        attributesRequest,
			output:       defaultResponse,
			expectError:  false,
			expectResult: result,
		},
		{
			name:         "secrets",
			snapshotName: defaultName,
			volume:       csiVolume,
			secrets:      createSecrets,
			input:        secretsRequest,
			output:       defaultResponse,
			expectError:  false,
			expectResult: result,
		},
		{
			name:         "fail for volume without csi source",
			snapshotName: defaultName,
			volume:       volumeWithoutCSI,
			input:        nil,
			output:       nil,
			expectError:  true,
		},
		{
			name:         "gRPC transient error",
			snapshotName: defaultName,
			volume:       csiVolume,
			input:        defaultRequest,
			output:       nil,
			injectError:  codes.DeadlineExceeded,
			expectError:  true,
		},
		{
			name:         "gRPC final error",
			snapshotName: defaultName,
			volume:       csiVolume,
			input:        defaultRequest,
			output:       nil,
			injectError:  codes.NotFound,
			expectError:  true,
		},
	}

	mockController, driver, identityServer, controllerServer, csiConn, err := createMockServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer mockController.Finish()
	defer driver.Stop()
	defer csiConn.Close()

	for _, test := range tests {
		in := test.input
		out := test.output
		var injectedErr error = nil
		if test.injectError != codes.OK {
			injectedErr = status.Error(test.injectError, fmt.Sprintf("Injecting error %d", test.injectError))
		}

		// Setup expectation
		if in != nil {
			identityServer.EXPECT().GetPluginInfo(gomock.Any(), gomock.Any()).Return(pluginInfoOutput, nil).Times(1)
			controllerServer.EXPECT().CreateSnapshot(gomock.Any(), in).Return(out, injectedErr).Times(1)
		}

		driverName, snapshotId, timestamp, size, readyToUse, err := csiConn.CreateSnapshot(context.Background(), test.snapshotName, test.volume, test.parameters, test.secrets)
		if test.expectError && err == nil {
			t.Errorf("test %q: Expected error, got none", test.name)
		}
		if !test.expectError && err != nil {
			t.Errorf("test %q: got error: %v", test.name, err)
		}
		if test.expectResult != nil {
			if driverName != test.expectResult.driverName {
				t.Errorf("test %q: expected driverName: %q, got: %q", test.name, test.expectResult.driverName, driverName)
			}

			if snapshotId != test.expectResult.snapshotId {
				t.Errorf("test %q: expected snapshotId: %v, got: %v", test.name, test.expectResult.snapshotId, snapshotId)
			}

			if timestamp != test.expectResult.timestamp {
				t.Errorf("test %q: expected create time: %v, got: %v", test.name, test.expectResult.timestamp, timestamp)
			}

			if size != test.expectResult.size {
				t.Errorf("test %q: expected size: %v, got: %v", test.name, test.expectResult.size, size)
			}

			if !reflect.DeepEqual(readyToUse, test.expectResult.readyToUse) {
				t.Errorf("test %q: expected readyToUse: %v, got: %v", test.name, test.expectResult.readyToUse, readyToUse)
			}
		}
	}
}

func TestDeleteSnapshot(t *testing.T) {
	defaultID := "testid"
	secrets := map[string]string{"foo": "bar"}

	defaultRequest := &csi.DeleteSnapshotRequest{
		SnapshotId: defaultID,
	}

	secretsRequest := &csi.DeleteSnapshotRequest{
		SnapshotId: defaultID,
		Secrets:    secrets,
	}

	tests := []struct {
		name        string
		snapshotID  string
		secrets     map[string]string
		input       *csi.DeleteSnapshotRequest
		output      *csi.DeleteSnapshotResponse
		injectError codes.Code
		expectError bool
	}{
		{
			name:        "success",
			snapshotID:  defaultID,
			input:       defaultRequest,
			output:      &csi.DeleteSnapshotResponse{},
			expectError: false,
		},
		{
			name:        "secrets",
			snapshotID:  defaultID,
			secrets:     secrets,
			input:       secretsRequest,
			output:      &csi.DeleteSnapshotResponse{},
			expectError: false,
		},
		{
			name:        "gRPC transient error",
			snapshotID:  defaultID,
			input:       defaultRequest,
			output:      nil,
			injectError: codes.DeadlineExceeded,
			expectError: true,
		},
		{
			name:        "gRPC final error",
			snapshotID:  defaultID,
			input:       defaultRequest,
			output:      nil,
			injectError: codes.NotFound,
			expectError: true,
		},
	}

	mockController, driver, _, controllerServer, csiConn, err := createMockServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer mockController.Finish()
	defer driver.Stop()
	defer csiConn.Close()

	for _, test := range tests {
		in := test.input
		out := test.output
		var injectedErr error = nil
		if test.injectError != codes.OK {
			injectedErr = status.Error(test.injectError, fmt.Sprintf("Injecting error %d", test.injectError))
		}

		// Setup expectation
		if in != nil {
			controllerServer.EXPECT().DeleteSnapshot(gomock.Any(), in).Return(out, injectedErr).Times(1)
		}

		err := csiConn.DeleteSnapshot(context.Background(), test.snapshotID, test.secrets)
		if test.expectError && err == nil {
			t.Errorf("test %q: Expected error, got none", test.name)
		}
		if !test.expectError && err != nil {
			t.Errorf("test %q: got error: %v", test.name, err)
		}
	}

}

func TestGetSnapshotStatus(t *testing.T) {
	defaultID := "testid"
	size := int64(1000)
	createTimestamp := ptypes.TimestampNow()
	createTime, err := ptypes.Timestamp(createTimestamp)
	if err != nil {
		t.Fatalf("Failed to convert timestamp to time: %v", err)
	}

	defaultRequest := &csi.ListSnapshotsRequest{
		SnapshotId: defaultID,
	}

	defaultResponse := &csi.ListSnapshotsResponse{
		Entries: []*csi.ListSnapshotsResponse_Entry{
			{
				Snapshot: &csi.Snapshot{
					SnapshotId:     defaultID,
					SizeBytes:      size,
					SourceVolumeId: "volumeid",
					CreationTime:   createTimestamp,
					ReadyToUse:     true,
				},
			},
		},
	}

	tests := []struct {
		name           string
		snapshotID     string
		input          *csi.ListSnapshotsRequest
		output         *csi.ListSnapshotsResponse
		injectError    codes.Code
		expectError    bool
		expectReady    bool
		expectCreateAt int64
		expectSize     int64
	}{
		{
			name:           "success",
			snapshotID:     defaultID,
			input:          defaultRequest,
			output:         defaultResponse,
			expectError:    false,
			expectReady:    true,
			expectCreateAt: createTime.UnixNano(),
			expectSize:     size,
		},
		{
			name:        "gRPC transient error",
			snapshotID:  defaultID,
			input:       defaultRequest,
			output:      nil,
			injectError: codes.DeadlineExceeded,
			expectError: true,
		},
		{
			name:        "gRPC final error",
			snapshotID:  defaultID,
			input:       defaultRequest,
			output:      nil,
			injectError: codes.NotFound,
			expectError: true,
		},
	}

	mockController, driver, _, controllerServer, csiConn, err := createMockServer(t)
	if err != nil {
		t.Fatal(err)
	}
	defer mockController.Finish()
	defer driver.Stop()
	defer csiConn.Close()

	for _, test := range tests {
		in := test.input
		out := test.output
		var injectedErr error = nil
		if test.injectError != codes.OK {
			injectedErr = status.Error(test.injectError, fmt.Sprintf("Injecting error %d", test.injectError))
		}

		// Setup expectation
		if in != nil {
			controllerServer.EXPECT().ListSnapshots(gomock.Any(), in).Return(out, injectedErr).Times(1)
		}

		ready, createTime, size, err := csiConn.GetSnapshotStatus(context.Background(), test.snapshotID)
		if test.expectError && err == nil {
			t.Errorf("test %q: Expected error, got none", test.name)
		}
		if !test.expectError && err != nil {
			t.Errorf("test %q: got error: %v", test.name, err)
		}
		if test.expectReady != ready {
			t.Errorf("test %q: expected status: %v, got: %v", test.name, test.expectReady, ready)
		}
		if test.expectCreateAt != createTime {
			t.Errorf("test %q: expected createTime: %v, got: %v", test.name, test.expectCreateAt, createTime)
		}
		if test.expectSize != size {
			t.Errorf("test %q: expected size: %v, got: %v", test.name, test.expectSize, size)
		}
	}
}

func FakeCSIVolume() *v1.PersistentVolume {
	volume := v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fake-csi-volume",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef: &v1.ObjectReference{
				Kind:       "PersistentVolumeClaim",
				APIVersion: "v1",
				UID:        types.UID("uid123"),
				Namespace:  "default",
				Name:       "test-claim",
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       driverName,
					VolumeHandle: "foo",
				},
			},
			StorageClassName: "default",
		},
		Status: v1.PersistentVolumeStatus{
			Phase: v1.VolumeBound,
		},
	}

	return &volume
}

func FakeVolume() *v1.PersistentVolume {
	volume := v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fake-csi-volume",
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef: &v1.ObjectReference{
				Kind:       "PersistentVolumeClaim",
				APIVersion: "v1",
				UID:        types.UID("uid123"),
				Namespace:  "default",
				Name:       "test-claim",
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				GCEPersistentDisk: &v1.GCEPersistentDiskVolumeSource{},
			},
			StorageClassName: "default",
		},
		Status: v1.PersistentVolumeStatus{
			Phase: v1.VolumeBound,
		},
	}

	return &volume
}
