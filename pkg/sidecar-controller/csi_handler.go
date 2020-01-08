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

package sidecar_controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/pkg/snapshotter"
	"github.com/kubernetes-csi/external-snapshotter/pkg/utils"
)

// Handler is responsible for handling VolumeSnapshot events from informer.
type Handler interface {
	CreateSnapshot(content *crdv1.VolumeSnapshotContent, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, time.Time, int64, bool, error)
	DeleteSnapshot(content *crdv1.VolumeSnapshotContent, snapshotterCredentials map[string]string) error
	GetSnapshotStatus(content *crdv1.VolumeSnapshotContent) (bool, time.Time, int64, error)
}

// csiHandler is a handler that calls CSI to create/delete volume snapshot.
type csiHandler struct {
	snapshotter            snapshotter.Snapshotter
	timeout                time.Duration
	snapshotNamePrefix     string
	snapshotNameUUIDLength int
}

// NewCSIHandler returns a handler which includes the csi connection and Snapshot name details
func NewCSIHandler(
	snapshotter snapshotter.Snapshotter,
	timeout time.Duration,
	snapshotNamePrefix string,
	snapshotNameUUIDLength int,
) Handler {
	return &csiHandler{
		snapshotter:            snapshotter,
		timeout:                timeout,
		snapshotNamePrefix:     snapshotNamePrefix,
		snapshotNameUUIDLength: snapshotNameUUIDLength,
	}
}

func (handler *csiHandler) CreateSnapshot(content *crdv1.VolumeSnapshotContent, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, time.Time, int64, bool, error) {

	ctx, cancel := context.WithTimeout(context.Background(), handler.timeout)
	defer cancel()

	if content.Spec.VolumeSnapshotRef.UID == "" {
		return "", "", time.Time{}, 0, false, fmt.Errorf("cannot create snapshot. Snapshot content %s not bound to a snapshot", content.Name)
	}

	if content.Spec.Source.VolumeHandle == nil {
		return "", "", time.Time{}, 0, false, fmt.Errorf("cannot create snapshot. Volume handle not found in snapshot content %s", content.Name)
	}

	snapshotName, err := makeSnapshotName(handler.snapshotNamePrefix, string(content.Spec.VolumeSnapshotRef.UID), handler.snapshotNameUUIDLength)
	if err != nil {
		return "", "", time.Time{}, 0, false, err
	}
	newParameters, err := utils.RemovePrefixedParameters(parameters)
	if err != nil {
		return "", "", time.Time{}, 0, false, fmt.Errorf("failed to remove CSI Parameters of prefixed keys: %v", err)
	}
	return handler.snapshotter.CreateSnapshot(ctx, snapshotName, *content.Spec.Source.VolumeHandle, newParameters, snapshotterCredentials)
}

func (handler *csiHandler) DeleteSnapshot(content *crdv1.VolumeSnapshotContent, snapshotterCredentials map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), handler.timeout)
	defer cancel()

	var snapshotHandle string
	var err error
	if content.Status != nil && content.Status.SnapshotHandle != nil {
		snapshotHandle = *content.Status.SnapshotHandle
	} else if content.Spec.Source.SnapshotHandle != nil {
		snapshotHandle = *content.Spec.Source.SnapshotHandle
	} else {
		return fmt.Errorf("failed to delete snapshot content %s: snapshotHandle is missing", content.Name)
	}

	err = handler.snapshotter.DeleteSnapshot(ctx, snapshotHandle, snapshotterCredentials)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot content %s: %q", content.Name, err)
	}

	return nil
}

func (handler *csiHandler) GetSnapshotStatus(content *crdv1.VolumeSnapshotContent) (bool, time.Time, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), handler.timeout)
	defer cancel()

	var snapshotHandle string
	var err error
	if content.Status != nil && content.Status.SnapshotHandle != nil {
		snapshotHandle = *content.Status.SnapshotHandle
	} else if content.Spec.Source.SnapshotHandle != nil {
		snapshotHandle = *content.Spec.Source.SnapshotHandle
	} else {
		return false, time.Time{}, 0, fmt.Errorf("failed to list snapshot for content %s: snapshotHandle is missing", content.Name)
	}

	csiSnapshotStatus, timestamp, size, err := handler.snapshotter.GetSnapshotStatus(ctx, snapshotHandle)
	if err != nil {
		return false, time.Time{}, 0, fmt.Errorf("failed to list snapshot for content %s: %q", content.Name, err)
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
