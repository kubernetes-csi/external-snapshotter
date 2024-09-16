/*
Copyright 2021 The Kubernetes Authors.

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

package driver

import (
	"net"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"

	"github.com/kubernetes-csi/csi-test/v5/utils"
)

type MockCSIDriverServers struct {
	Controller       *MockControllerServer
	Identity         *MockIdentityServer
	Node             *MockNodeServer
	SnapshotMetadata *MockSnapshotMetadataServer
}

type MockCSIDriver struct {
	CSIDriver
	conn *grpc.ClientConn
}

func NewMockCSIDriver(servers *MockCSIDriverServers) *MockCSIDriver {
	return &MockCSIDriver{
		CSIDriver: CSIDriver{
			servers: &CSIDriverServers{
				Controller: struct {
					csi.UnsafeControllerServer
					*MockControllerServer
				}{MockControllerServer: servers.Controller},
				Node: struct {
					csi.UnsafeNodeServer
					*MockNodeServer
				}{MockNodeServer: servers.Node},
				Identity: struct {
					csi.UnsafeIdentityServer
					*MockIdentityServer
				}{MockIdentityServer: servers.Identity},
				SnapshotMetadata: struct {
					csi.UnsafeSnapshotMetadataServer
					*MockSnapshotMetadataServer
				}{MockSnapshotMetadataServer: servers.SnapshotMetadata},
			},
		},
	}
}

// StartOnAddress starts a new gRPC server listening on given address.
func (m *MockCSIDriver) StartOnAddress(network, address string) error {
	l, err := net.Listen(network, address)
	if err != nil {
		return err
	}

	if err := m.CSIDriver.Start(l); err != nil {
		l.Close()
		return err
	}

	return nil
}

// Start starts a new gRPC server listening on a random TCP loopback port.
func (m *MockCSIDriver) Start() error {
	// Listen on a port assigned by the net package
	return m.StartOnAddress("tcp", "127.0.0.1:0")
}

func (m *MockCSIDriver) Nexus() (*grpc.ClientConn, error) {
	// Start server
	err := m.Start()
	if err != nil {
		return nil, err
	}

	// Create a client connection
	m.conn, err = utils.Connect(m.Address(), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return m.conn, nil
}

func (m *MockCSIDriver) Close() {
	m.conn.Close()
	m.server.Stop()
}
