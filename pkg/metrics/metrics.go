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
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/types"
	k8smetrics "k8s.io/component-base/metrics"
	"k8s.io/klog"
)

const (
	opStatusUnknown      = "Unknown"
	labelDriverName      = "driver_name"
	labelOperationName   = "operation_name"
	labelOperationStatus = "operation_status"
	subSystem            = "snapshot_controller"
	metricName           = "operation_total_seconds"
	metricHelpMsg        = "Total number of seconds spent by the controller on an operation from end to end"
)

// OperationStatus is the interface type for representing an operation's execution
// status, with the nil value representing an "Unknown" status of the operation.
type OperationStatus interface {
	String() string
}

var metricBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 30, 60, 120, 300, 600}

type MetricsManager interface {
	// StartMetricsEndpoint starts the metrics endpoint at the specified addr/pattern for
	// metrics managed by this MetricsManager. It spawns a goroutine to listen to
	// and serve HTTP requests received on addr/pattern.
	// If the "pattern" is empty (i.e., ""), no endpoint will be started.
	// An error will be returned if there is any.
	StartMetricsEndpoint(pattern, addr string, logger promhttp.Logger, wg *sync.WaitGroup) (*http.Server, error)

	// OperationStart takes in an operation and caches its start time.
	// if the operation already exists, it's an no-op.
	OperationStart(op Operation)

	// DropOperation removes an operation from cache.
	// if the operation does not exist, it's an no-op.
	DropOperation(op Operation)

	// RecordMetrics records a metric point. Note that it will be an no-op if an
	// operation has NOT been marked "Started" previously via invoking "OperationStart".
	// Invoking of RecordMetrics effectively removes the cached entry.
	// op - the operation which the metric is associated with.
	// status - the operation status, if not specified, i.e., status == nil, an
	//          "Unknown" status of the passed-in operation is assumed.
	RecordMetrics(op Operation, status OperationStatus)
}

// Operation is a structure which holds information to identify a snapshot
// related operation
type Operation struct {
	// the name of the operation, for example: "CreateSnapshot", "DeleteSnapshot"
	Name string
	// the name of the driver which executes the operation
	Driver string
	// the resource UID to which the operation has been executed against
	ResourceID types.UID
}

type operationMetricsManager struct {
	// cache is a concurrent-safe map which stores start timestamps for all
	// ongoing operations.
	// key is an Operation
	// value is the timestamp of the start time of the operation
	cache sync.Map

	// registry is a wrapper around Prometheus Registry
	registry k8smetrics.KubeRegistry

	// opLatencyMetrics is a Histogram metrics
	opLatencyMetrics *k8smetrics.HistogramVec
}

func NewMetricsManager() MetricsManager {
	mgr := &operationMetricsManager{
		cache: sync.Map{},
	}
	mgr.init()
	return mgr
}

func (opMgr *operationMetricsManager) OperationStart(op Operation) {
	opMgr.cache.LoadOrStore(op, time.Now())
}

func (opMgr *operationMetricsManager) DropOperation(op Operation) {
	opMgr.cache.Delete(op)
}

func (opMgr *operationMetricsManager) RecordMetrics(op Operation, status OperationStatus) {
	obj, exists := opMgr.cache.Load(op)
	if !exists {
		// the operation has not been cached, return directly
		return
	}
	ts, ok := obj.(time.Time)
	if !ok {
		// the cached item is not a time.Time, should NEVER happen, clean and return
		klog.Errorf("Invalid cache entry for key %v", op)
		opMgr.cache.Delete(op)
		return
	}
	strStatus := opStatusUnknown
	if status != nil {
		strStatus = status.String()
	}
	duration := time.Since(ts).Seconds()
	opMgr.opLatencyMetrics.WithLabelValues(op.Driver, op.Name, strStatus).Observe(duration)
	opMgr.cache.Delete(op)
}

func (opMgr *operationMetricsManager) init() {
	opMgr.registry = k8smetrics.NewKubeRegistry()
	opMgr.opLatencyMetrics = k8smetrics.NewHistogramVec(
		&k8smetrics.HistogramOpts{
			Subsystem: subSystem,
			Name:      metricName,
			Help:      metricHelpMsg,
			Buckets:   metricBuckets,
		},
		[]string{labelDriverName, labelOperationName, labelOperationStatus},
	)
	opMgr.registry.MustRegister(opMgr.opLatencyMetrics)
}

func (opMgr *operationMetricsManager) StartMetricsEndpoint(pattern, addr string, logger promhttp.Logger, wg *sync.WaitGroup) (*http.Server, error) {
	if addr == "" {
		return nil, fmt.Errorf("metrics endpoint will not be started as endpoint address is not specified")
	}
	// start listening
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on address[%s], error[%v]", addr, err)
	}
	mux := http.NewServeMux()
	mux.Handle(pattern, k8smetrics.HandlerFor(
		opMgr.registry,
		k8smetrics.HandlerOpts{
			ErrorLog:      logger,
			ErrorHandling: k8smetrics.ContinueOnError,
		}))
	srv := &http.Server{Addr: l.Addr().String(), Handler: mux}
	// start serving the endpoint
	go func() {
		defer wg.Done()
		if err := srv.Serve(l); err != http.ErrServerClosed {
			klog.Fatalf("failed to start endpoint at:%s/%s, error: %v", addr, pattern, err)
		}
	}()
	return srv, nil
}
