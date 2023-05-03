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
	"github.com/container-storage-interface/spec/lib/go/csi"
	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/group_snapshotter"
	"strings"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/snapshotter"
)

// Handler is responsible for handling VolumeSnapshot events from informer.
type Handler interface {
	CreateSnapshot(content *crdv1.VolumeSnapshotContent, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, time.Time, int64, bool, error)
	DeleteSnapshot(content *crdv1.VolumeSnapshotContent, snapshotterCredentials map[string]string) error
	GetSnapshotStatus(content *crdv1.VolumeSnapshotContent, snapshotterListCredentials map[string]string) (bool, time.Time, int64, error)
	CreateGroupSnapshot(content *crdv1alpha1.VolumeGroupSnapshotContent, volumeIDs []string, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, []*csi.Snapshot, time.Time, bool, error)
	GetGroupSnapshotStatus(content *crdv1alpha1.VolumeGroupSnapshotContent, snapshotterListCredentials map[string]string) (bool, time.Time, error)
}

// csiHandler is a handler that calls CSI to create/delete volume snapshot.
type csiHandler struct {
	snapshotter                 snapshotter.Snapshotter
	groupSnapshotter            group_snapshotter.GroupSnapshotter
	timeout                     time.Duration
	snapshotNamePrefix          string
	snapshotNameUUIDLength      int
	groupSnapshotNamePrefix     string
	groupSnapshotNameUUIDLength int
}

// NewCSIHandler returns a handler which includes the csi connection and Snapshot name details
func NewCSIHandler(
	snapshotter snapshotter.Snapshotter,
	groupSnapshotter group_snapshotter.GroupSnapshotter,
	timeout time.Duration,
	snapshotNamePrefix string,
	snapshotNameUUIDLength int,
	groupSnapshotNamePrefix string,
	groupSnapshotNameUUIDLength int,
) Handler {
	return &csiHandler{
		snapshotter:                 snapshotter,
		groupSnapshotter:            groupSnapshotter,
		timeout:                     timeout,
		snapshotNamePrefix:          snapshotNamePrefix,
		snapshotNameUUIDLength:      snapshotNameUUIDLength,
		groupSnapshotNamePrefix:     groupSnapshotNamePrefix,
		groupSnapshotNameUUIDLength: groupSnapshotNameUUIDLength,
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
	return handler.snapshotter.CreateSnapshot(ctx, snapshotName, *content.Spec.Source.VolumeHandle, parameters, snapshotterCredentials)
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

func (handler *csiHandler) GetSnapshotStatus(content *crdv1.VolumeSnapshotContent, snapshotterListCredentials map[string]string) (bool, time.Time, int64, error) {
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

	csiSnapshotStatus, timestamp, size, err := handler.snapshotter.GetSnapshotStatus(ctx, snapshotHandle, snapshotterListCredentials)
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

func (handler *csiHandler) CreateGroupSnapshot(content *crdv1alpha1.VolumeGroupSnapshotContent, volumeIDs []string, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, []*csi.Snapshot, time.Time, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), handler.timeout)
	defer cancel()

	if content.Spec.VolumeGroupSnapshotRef.UID == "" {
		return "", "", nil, time.Time{}, false, fmt.Errorf("cannot create group snapshot. Group snapshot content %s not bound to a group snapshot", content.Name)
	}

	if len(volumeIDs) == 0 {
		return "", "", nil, time.Time{}, false, fmt.Errorf("cannot create group snapshot. PVCs to be snapshotted not found in group snapshot content %s", content.Name)
	}

	groupSnapshotName, err := handler.makeGroupSnapshotName(string(content.Spec.VolumeGroupSnapshotRef.UID))
	if err != nil {
		return "", "", nil, time.Time{}, false, err
	}
	return handler.groupSnapshotter.CreateGroupSnapshot(ctx, groupSnapshotName, volumeIDs, parameters, snapshotterCredentials)
}

func (handler *csiHandler) GetGroupSnapshotStatus(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent, snapshotterListCredentials map[string]string) (bool, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), handler.timeout)
	defer cancel()

	var groupSnapshotHandle string
	var err error
	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.VolumeGroupSnapshotHandle != nil {
		groupSnapshotHandle = *groupSnapshotContent.Status.VolumeGroupSnapshotHandle
	} else if groupSnapshotContent.Spec.Source.VolumeGroupSnapshotHandle != nil {
		groupSnapshotHandle = *groupSnapshotContent.Spec.Source.VolumeGroupSnapshotHandle
	} else {
		return false, time.Time{}, fmt.Errorf("failed to list group snapshot for group snapshot content %s: groupSnapshotHandle is missing", groupSnapshotContent.Name)
	}

	csiSnapshotStatus, timestamp, err := handler.groupSnapshotter.GetGroupSnapshotStatus(ctx, groupSnapshotHandle, snapshotterListCredentials)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to list group snapshot for group snapshot content %s: %q", groupSnapshotContent.Name, err)
	}

	return csiSnapshotStatus, timestamp, nil
}

func (handler *csiHandler) makeGroupSnapshotName(groupSnapshotUID string) (string, error) {
	if len(groupSnapshotUID) == 0 {
		return "", fmt.Errorf("group snapshot object is missing UID")
	}
	if handler.groupSnapshotNameUUIDLength == -1 {
		return fmt.Sprintf("%s-%s", handler.groupSnapshotNamePrefix, groupSnapshotUID), nil
	}

	return fmt.Sprintf("%s-%s", handler.groupSnapshotNamePrefix, strings.Replace(groupSnapshotUID, "-", "", -1)[0:handler.groupSnapshotNameUUIDLength]), nil
}
