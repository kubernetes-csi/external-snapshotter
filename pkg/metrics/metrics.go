/*
Copyright 2020 The Kubernetes Authors.

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
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/types"
	k8smetrics "k8s.io/component-base/metrics"
)

const (
	labelDriverName               = "driver_name"
	labelOperationName            = "operation_name"
	labelOperationStatus          = "operation_status"
	labelSnapshotType             = "snapshot_type"
	subSystem                     = "snapshot_controller"
	operationLatencyMetricName    = "operation_total_seconds"
	operationLatencyMetricHelpMsg = "Total number of seconds spent by the controller on an operation"
	operationInFlightName         = "operations_in_flight"
	operationInFlightHelpMsg      = "Total number of operations in flight"
	unknownDriverName             = "unknown"

	// CreateSnapshotOperationName is the operation that tracks how long the controller takes to create a snapshot.
	// Specifically, the operation metric is emitted based on the following timestamps:
	// - Start_time: controller notices the first time that there is a new VolumeSnapshot CR to dynamically provision a snapshot
	// - End_time:   controller notices that the CR has a status with CreationTime field set to be non-nil
	CreateSnapshotOperationName = "CreateSnapshot"

	// CreateSnapshotAndReadyOperationName is the operation that tracks how long the controller takes to create a snapshot and for it to be ready.
	// Specifically, the operation metric is emitted based on the following timestamps:
	// - Start_time: controller notices the first time that there is a new VolumeSnapshot CR(both dynamic and pre-provisioned cases)
	// - End_time:   controller notices that the CR has a status with Ready field set to be true
	CreateSnapshotAndReadyOperationName = "CreateSnapshotAndReady"

	// DeleteSnapshotOperationName is the operation that tracks how long a snapshot deletion takes.
	// Specifically, the operation metric is emitted based on the following timestamps:
	// - Start_time: controller notices the first time that there is a deletion timestamp placed on the VolumeSnapshot CR and the CR is ready to be deleted. Note that if the CR is being used by a PVC for rehydration, the controller should *NOT* set the start_time.
	// - End_time: controller removed all finalizers on the VolumeSnapshot CR such that the CR is ready to be removed in the API server.
	DeleteSnapshotOperationName = "DeleteSnapshot"

	// DynamicSnapshotType represents a snapshot that is being dynamically provisioned
	DynamicSnapshotType = snapshotProvisionType("dynamic")
	// PreProvisionedSnapshotType represents a snapshot that is pre-provisioned
	PreProvisionedSnapshotType = snapshotProvisionType("pre-provisioned")

	// SnapshotStatusTypeUnknown represents that the status is unknown
	SnapshotStatusTypeUnknown snapshotStatusType = "unknown"
	// Success and Cancel are statuses for operation time (operation_total_seconds) as seen by snapshot controller
	// SnapshotStatusTypeSuccess represents that a CreateSnapshot, CreateSnapshotAndReady,
	// or DeleteSnapshot has finished successfully.
	// Individual reconciliations (reconciliation_total_seconds) also use this status.
	SnapshotStatusTypeSuccess snapshotStatusType = "success"
	// SnapshotStatusTypeCancel represents that a CreateSnapshot, CreateSnapshotAndReady,
	// or DeleteSnapshot has been deleted before finishing.
	SnapshotStatusTypeCancel snapshotStatusType = "cancel"
)

var (
	inFlightCheckInterval = 30 * time.Second
)

// OperationStatus is the interface type for representing an operation's execution
// status, with the nil value representing an "Unknown" status of the operation.
type OperationStatus interface {
	String() string
}

var metricBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 30, 60, 120, 300, 600}

type MetricsManager interface {
	// PrepareMetricsPath prepares the metrics path the specified pattern for
	// metrics managed by this MetricsManager.
	// If the "pattern" is empty (i.e., ""), it will not be registered.
	// An error will be returned if there is any.
	PrepareMetricsPath(mux *http.ServeMux, pattern string, logger promhttp.Logger) error

	// OperationStart takes in an operation and caches its start time.
	// if the operation already exists, it's an no-op.
	OperationStart(key OperationKey, val OperationValue)

	// DropOperation removes an operation from cache.
	// if the operation does not exist, it's an no-op.
	DropOperation(op OperationKey)

	// RecordMetrics records a metric point. Note that it will be an no-op if an
	// operation has NOT been marked "Started" previously via invoking "OperationStart".
	// Invoking of RecordMetrics effectively removes the cached entry.
	// op - the operation which the metric is associated with.
	// status - the operation status, if not specified, i.e., status == nil, an
	//          "Unknown" status of the passed-in operation is assumed.
	RecordMetrics(op OperationKey, status OperationStatus, driverName string)

	// RecordVolumeGroupSnapshotMetrics records a metric for operations related to
	// VolumeGroupSnapshot
	RecordVolumeGroupSnapshotMetrics(op OperationKey, status OperationStatus, driverName string)

	// GetRegistry() returns the metrics.KubeRegistry used by this metrics manager.
	GetRegistry() k8smetrics.KubeRegistry
}

// OperationKey is a structure which holds information to
// uniquely identify a snapshot related operation
type OperationKey struct {
	// Name is the name of the operation, for example: "CreateSnapshot", "DeleteSnapshot"
	Name string
	// ResourceID is the resource UID to which the operation has been executed against
	ResourceID types.UID
}

// OperationValue is a structure which holds operation metadata
type OperationValue struct {
	// Driver is the driver name which executes the operation
	Driver string
	// SnapshotType represents the snapshot type, for example: "dynamic", "pre-provisioned"
	SnapshotType string

	// startTime is the time when the operation first started
	startTime time.Time
}

// NewOperationKey initializes a new OperationKey
func NewOperationKey(name string, snapshotUID types.UID) OperationKey {
	return OperationKey{
		Name:       name,
		ResourceID: snapshotUID,
	}
}

// NewOperationValue initializes a new OperationValue
func NewOperationValue(driver string, snapshotType snapshotProvisionType) OperationValue {
	if driver == "" {
		driver = unknownDriverName
	}

	return OperationValue{
		Driver:       driver,
		SnapshotType: string(snapshotType),
	}
}

type operationMetricsManager struct {
	// cache is a concurrent-safe map which stores start timestamps for all
	// ongoing operations.
	// key is an Operation
	// value is the timestamp of the start time of the operation
	cache map[OperationKey]OperationValue

	// mutex for protecting cache from concurrent access
	mu sync.Mutex

	// registry is a wrapper around Prometheus Registry
	registry k8smetrics.KubeRegistry

	// opLatencyMetrics is a Histogram metrics for operation time per request
	opLatencyMetrics *k8smetrics.HistogramVec

	// opInFlight is a Gauge metric for the number of operations in flight
	opInFlight *k8smetrics.Gauge
}

// NewMetricsManager creates a new MetricsManager instance
func NewMetricsManager() MetricsManager {
	mgr := &operationMetricsManager{
		cache: make(map[OperationKey]OperationValue),
	}
	mgr.init()
	return mgr
}

// OperationStart starts a new operation
func (opMgr *operationMetricsManager) OperationStart(key OperationKey, val OperationValue) {
	opMgr.mu.Lock()
	defer opMgr.mu.Unlock()

	if _, exists := opMgr.cache[key]; !exists {
		val.startTime = time.Now()
		opMgr.cache[key] = val
	}
	opMgr.opInFlight.Set(float64(len(opMgr.cache)))
}

// OperationStart drops an operation
func (opMgr *operationMetricsManager) DropOperation(op OperationKey) {
	opMgr.mu.Lock()
	defer opMgr.mu.Unlock()
	delete(opMgr.cache, op)
	opMgr.opInFlight.Set(float64(len(opMgr.cache)))
}

// RecordMetrics emits operation metrics
func (opMgr *operationMetricsManager) RecordMetrics(opKey OperationKey, opStatus OperationStatus, driverName string) {
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

	// Report cancel metrics if we are deleting an unfinished VolumeSnapshot
	if opKey.Name == DeleteSnapshotOperationName {
		// check if we have a CreateSnapshot operation pending for this
		createKey := NewOperationKey(CreateSnapshotOperationName, opKey.ResourceID)
		obj, exists := opMgr.cache[createKey]
		if exists {
			// record a cancel metric if found
			opMgr.recordCancelMetricLocked(obj, createKey, operationDuration)
		}

		// check if we have a CreateSnapshotAndReady operation pending for this
		createAndReadyKey := NewOperationKey(CreateSnapshotAndReadyOperationName, opKey.ResourceID)
		obj, exists = opMgr.cache[createAndReadyKey]
		if exists {
			// record a cancel metric if found
			opMgr.recordCancelMetricLocked(obj, createAndReadyKey, operationDuration)
		}
	}

	delete(opMgr.cache, opKey)
	opMgr.opInFlight.Set(float64(len(opMgr.cache)))
}

// recordCancelMetric records a metric for a create operation that hasn't finished
// This function must be called with opMgr mutex locked (to prevent recursive locks).
func (opMgr *operationMetricsManager) recordCancelMetricLocked(val OperationValue, key OperationKey, duration float64) {
	// record a cancel metric if found

	opMgr.opLatencyMetrics.WithLabelValues(
		val.Driver,
		key.Name,
		val.SnapshotType,
		string(SnapshotStatusTypeCancel),
	).Observe(duration)
	delete(opMgr.cache, key)
}

func (opMgr *operationMetricsManager) init() {
	opMgr.registry = k8smetrics.NewKubeRegistry()
	k8smetrics.RegisterProcessStartTime(opMgr.registry.Register)
	opMgr.opLatencyMetrics = k8smetrics.NewHistogramVec(
		&k8smetrics.HistogramOpts{
			Subsystem: subSystem,
			Name:      operationLatencyMetricName,
			Help:      operationLatencyMetricHelpMsg,
			Buckets:   metricBuckets,
		},
		[]string{labelDriverName, labelOperationName, labelSnapshotType, labelOperationStatus},
	)
	opMgr.registry.MustRegister(opMgr.opLatencyMetrics)
	opMgr.opInFlight = k8smetrics.NewGauge(
		&k8smetrics.GaugeOpts{
			Subsystem: subSystem,
			Name:      operationInFlightName,
			Help:      operationInFlightHelpMsg,
		},
	)
	opMgr.registry.MustRegister(opMgr.opInFlight)

	// While we always maintain the number of operations in flight
	// for every metrics operation start/finish, if any are leaked,
	// this scheduled routine will catch any leaked operations.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go opMgr.scheduleOpsInFlightMetric(ctx)
}

func (opMgr *operationMetricsManager) scheduleOpsInFlightMetric(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			for range time.NewTicker(inFlightCheckInterval).C {
				func() {
					opMgr.mu.Lock()
					defer opMgr.mu.Unlock()
					opMgr.opInFlight.Set(float64(len(opMgr.cache)))
				}()
			}
		}
	}
}

func (opMgr *operationMetricsManager) PrepareMetricsPath(mux *http.ServeMux, pattern string, logger promhttp.Logger) error {
	mux.Handle(pattern, k8smetrics.HandlerFor(
		opMgr.registry,
		k8smetrics.HandlerOpts{
			ErrorLog:      logger,
			ErrorHandling: k8smetrics.ContinueOnError,
		}))

	return nil
}

func (opMgr *operationMetricsManager) GetRegistry() k8smetrics.KubeRegistry {
	return opMgr.registry
}

// snapshotProvisionType represents which kind of snapshot a metric is
type snapshotProvisionType string

// snapshotStatusType represents the type of snapshot status to report
type snapshotStatusType string

// SnapshotOperationStatus represents the status for a snapshot controller operation
type SnapshotOperationStatus struct {
	status snapshotStatusType
}

// NewSnapshotOperationStatus returns a new SnapshotOperationStatus
func NewSnapshotOperationStatus(status snapshotStatusType) SnapshotOperationStatus {
	return SnapshotOperationStatus{
		status: status,
	}
}

func (sos SnapshotOperationStatus) String() string {
	return string(sos.status)
}
