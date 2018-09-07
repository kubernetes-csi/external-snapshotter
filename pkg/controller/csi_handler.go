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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	"github.com/kubernetes-csi/external-snapshotter/pkg/connection"
	"k8s.io/api/core/v1"
)

// Handler is responsible for handling VolumeSnapshot events from informer.
type Handler interface {
	CreateSnapshot(snapshot *crdv1.VolumeSnapshot, volume *v1.PersistentVolume, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, int64, int64, *csi.SnapshotStatus, error)
	DeleteSnapshot(content *crdv1.VolumeSnapshotContent, snapshotterCredentials map[string]string) error
	GetSnapshotStatus(content *crdv1.VolumeSnapshotContent) (*csi.SnapshotStatus, int64, int64, error)
}

// csiHandler is a handler that calls CSI to create/delete volume snapshot.
type csiHandler struct {
	csiConnection          connection.CSIConnection
	timeout                time.Duration
	snapshotNamePrefix     string
	snapshotNameUUIDLength int
}

func NewCSIHandler(
	csiConnection connection.CSIConnection,
	timeout time.Duration,
	snapshotNamePrefix string,
	snapshotNameUUIDLength int,
) Handler {
	return &csiHandler{
		csiConnection:          csiConnection,
		timeout:                timeout,
		snapshotNamePrefix:     snapshotNamePrefix,
		snapshotNameUUIDLength: snapshotNameUUIDLength,
	}
}

func (handler *csiHandler) CreateSnapshot(snapshot *crdv1.VolumeSnapshot, volume *v1.PersistentVolume, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, int64, int64, *csi.SnapshotStatus, error) {

	ctx, cancel := context.WithTimeout(context.Background(), handler.timeout)
	defer cancel()

	snapshotName, err := makeSnapshotName(handler.snapshotNamePrefix, string(snapshot.UID), handler.snapshotNameUUIDLength)
	if err != nil {
		return "", "", 0, 0, nil, err
	}
	return handler.csiConnection.CreateSnapshot(ctx, snapshotName, volume, parameters, snapshotterCredentials)
}

func (handler *csiHandler) DeleteSnapshot(content *crdv1.VolumeSnapshotContent, snapshotterCredentials map[string]string) error {
	if content.Spec.CSI == nil {
		return fmt.Errorf("CSISnapshot not defined in spec")
	}
	ctx, cancel := context.WithTimeout(context.Background(), handler.timeout)
	defer cancel()

	err := handler.csiConnection.DeleteSnapshot(ctx, content.Spec.CSI.SnapshotHandle, snapshotterCredentials)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot data %s: %q", content.Name, err)
	}

	return nil
}

func (handler *csiHandler) GetSnapshotStatus(content *crdv1.VolumeSnapshotContent) (*csi.SnapshotStatus, int64, int64, error) {
	if content.Spec.CSI == nil {
		return nil, 0, 0, fmt.Errorf("CSISnapshot not defined in spec")
	}
	ctx, cancel := context.WithTimeout(context.Background(), handler.timeout)
	defer cancel()

	csiSnapshotStatus, timestamp, size, err := handler.csiConnection.GetSnapshotStatus(ctx, content.Spec.CSI.SnapshotHandle)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to list snapshot data %s: %q", content.Name, err)
	}
	return csiSnapshotStatus, timestamp, size, nil

}

func makeSnapshotName(prefix, snapshotUID string, snapshotNameUUIDLength int) (string, error) {
	// create persistent name based on a volumeNamePrefix and volumeNameUUIDLength
	// of PVC's UID
	if len(snapshotUID) == 0 {
		return "", fmt.Errorf("Corrupted snapshot object, it is missing UID")
	}
	if snapshotNameUUIDLength == -1 {
		// Default behavior is to not truncate or remove dashes
		return fmt.Sprintf("%s-%s", prefix, snapshotUID), nil
	}
	return fmt.Sprintf("%s-%s", prefix, strings.Replace(snapshotUID, "-", "", -1)[0:snapshotNameUUIDLength]), nil
}
