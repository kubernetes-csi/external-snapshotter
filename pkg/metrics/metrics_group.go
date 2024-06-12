/*
Copyright 2024 The Kubernetes Authors.

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

package metrics

import (
	"time"
)

const (
	// CreateGroupSnapshotOperationName is the operation that tracks how long the controller takes to create a groupsnapshot.
	// Specifically, the operation metric is emitted based on the following timestamps:
	// - Start_time: controller notices the first time that there is a new VolumeGroupSnapshot CR to dynamically provision a groupsnapshot
	// - End_time:   controller notices that the CR has a status with CreationTime field set to be non-nil
	CreateGroupSnapshotOperationName = "CreateGroupSnapshot"

	// CreateGroupSnapshotAndReadyOperationName is the operation that tracks how long the controller takes to create a groupsnapshot and for it to be ready.
	// Specifically, the operation metric is emitted based on the following timestamps:
	// - Start_time: controller notices the first time that there is a new VolumeGroupSnapshot CR(both dynamic and pre-provisioned cases)
	// - End_time:   controller notices that the CR has a status with Ready To Use field set to be true
	CreateGroupSnapshotAndReadyOperationName = "CreateGroupSnapshotAndReady"

	// DeleteGroupSnapshotOperationName is the operation that tracks how long a groupsnapshot deletion takes.
	// Specifically, the operation metric is emitted based on the following timestamps:
	// - Start_time: controller notices the first time that there is a deletion timestamp placed on the VolumeGroupSnapshot CR and the CR is ready to be deleted.
	// Note that if the CR is being used by a PVC for rehydration, the controller should *NOT* set the start_time.
	// - End_time: controller removed all finalizers on the VolumeGroupSnapshot CR such that the CR is ready to be removed in the API server.
	DeleteGroupSnapshotOperationName = "DeleteGroupSnapshot"
	// DynamicGroupSnapshotType represents a groupsnapshot that is being dynamically provisioned
	DynamicGroupSnapshotType = snapshotProvisionType("dynamic")
	// PreProvisionedGroupSnapshotType represents a groupsnapshot that is pre-provisioned
	PreProvisionedGroupSnapshotType = snapshotProvisionType("pre-provisioned")
)

// RecordVolumeGroupMetrics emits operation metrics
func (opMgr *operationMetricsManager) RecordVolumeGroupSnapshotMetrics(opKey OperationKey, opStatus OperationStatus, driverName string) {
	opMgr.mu.Lock()
	defer opMgr.mu.Unlock()
	opVal, exists := opMgr.cache[opKey]
	if !exists {
		// the operation has not been cached, return directly
		return
	}
	status := string(SnapshotStatusTypeUnknown)
	if opStatus != nil {
		status = opStatus.String()
	}

	// if we do not know the driverName while recording metrics,
	// refer to the cached version instead.
	if driverName == "" || driverName == unknownDriverName {
		driverName = opVal.Driver
	}

	operationDuration := time.Since(opVal.startTime).Seconds()
	opMgr.opLatencyMetrics.WithLabelValues(driverName, opKey.Name, opVal.SnapshotType, status).Observe(operationDuration)

	// Report cancel metrics if we are deleting an unfinished VolumeGroupSnapshot
	if opKey.Name == DeleteGroupSnapshotOperationName {
		// check if we have a CreateGroupSnapshot operation pending for this
		createKey := NewOperationKey(CreateGroupSnapshotOperationName, opKey.ResourceID)
		obj, exists := opMgr.cache[createKey]
		if exists {
			// record a cancel metric if found
			opMgr.recordCancelMetricLocked(obj, createKey, operationDuration)
		}

		// check if we have a CreateGroupSnapshotAndReady operation pending for this
		createAndReadyKey := NewOperationKey(CreateGroupSnapshotAndReadyOperationName, opKey.ResourceID)
		obj, exists = opMgr.cache[createAndReadyKey]
		if exists {
			// record a cancel metric if found
			opMgr.recordCancelMetricLocked(obj, createAndReadyKey, operationDuration)
		}
	}

	delete(opMgr.cache, opKey)
	opMgr.opInFlight.Set(float64(len(opMgr.cache)))
}
