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
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	cmg "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"k8s.io/apimachinery/pkg/types"
)

var (
	statusMap map[int]string = map[int]string{
		0: "Success",
		1: "Failure",
		2: "Unknown",
	}
)

const (
	httpPattern            = "/metrics"
	addr                   = "localhost:0"
	processStartTimeMetric = "process_start_time_seconds"
)

type fakeOpStatus struct {
	statusCode int
}

func (s *fakeOpStatus) String() string {
	if str, ok := statusMap[s.statusCode]; ok {
		return str
	}
	return "Unknown"
}

func initMgr() (MetricsManager, *http.Server) {
	mgr := NewMetricsManager()
	mux := http.NewServeMux()
	err := mgr.PrepareMetricsPath(mux, httpPattern, nil)
	if err != nil {
		log.Fatalf("failed to start serving [%v]", err)
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on address[%s], error[%v]", addr, err)
	}
	srv := &http.Server{Addr: l.Addr().String(), Handler: mux}
	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			log.Fatalf("failed to start endpoint at:%s/%s, error: %v", addr, httpPattern, err)
		}
	}()

	return mgr, srv
}

func shutdown(srv *http.Server) {
	if err := srv.Shutdown(context.Background()); err != nil {
		panic(err)
	}
}

func TestNew(t *testing.T) {
	mgr, srv := initMgr()
	defer shutdown(srv)
	if mgr == nil {
		t.Errorf("failed testing new")
	}
}

func TestDropNonExistingOperation(t *testing.T) {
	mgr, srv := initMgr()
	defer shutdown(srv)
	op := OperationKey{
		Name:       "drop-non-existing-operation-should-be-noop",
		ResourceID: types.UID("uid"),
	}
	mgr.DropOperation(op)
}

func TestRecordMetricsForNonExistingOperation(t *testing.T) {
	mgr, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv)
	opKey := OperationKey{
		Name:       "no-metrics-should-be-recorded-as-operation-did-not-start",
		ResourceID: types.UID("uid"),
	}
	mgr.RecordMetrics(opKey, nil, "driver")
	rsp, err := http.Get(srvAddr)
	if err != nil || rsp.StatusCode != http.StatusOK {
		t.Errorf("failed to get response from server %v, %v", err, rsp)
	}
	r, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Errorf("failed to read response body %v", err)
	}
	if strings.Contains(string(r), opKey.Name) {
		t.Errorf("found metric should have been dropped for operation [%s] [%s]", opKey.Name, string(r))
	}
}

func TestDropOperation(t *testing.T) {
	mgr, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv)
	opKey := OperationKey{
		Name:       "should-have-been-dropped",
		ResourceID: types.UID("uid"),
	}
	opVal := NewOperationValue("driver", DynamicSnapshotType)
	mgr.OperationStart(opKey, opVal)
	mgr.DropOperation(opKey)
	time.Sleep(300 * time.Millisecond)
	rsp, err := http.Get(srvAddr)
	if err != nil || rsp.StatusCode != http.StatusOK {
		t.Errorf("failed to get response from server %v, %v", err, rsp)
	}
	r, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Errorf("failed to read response body %v", err)
	}
	if strings.Contains(string(r), opKey.Name) {
		t.Errorf("found metric should have been dropped for operation [%s] [%s]", opKey.Name, string(r))
	}
	// re-add with a different name
	opKey.Name = "should-have-been-added"
	mgr.OperationStart(opKey, opVal)
	time.Sleep(300 * time.Millisecond)
	opStatus := &fakeOpStatus{
		statusCode: 0,
	}
	mgr.RecordMetrics(opKey, opStatus, "driver")
	expected :=
		`# HELP snapshot_controller_operation_total_seconds [ALPHA] Total number of seconds spent by the controller on an operation from end to end
# TYPE snapshot_controller_operation_total_seconds histogram
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="0.25"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type=""} 0.3
snapshot_controller_operation_total_seconds_count{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",snapshot_type=""} 1
`

	if err := verifyMetric(expected, srvAddr); err != nil {
		t.Errorf("failed testing[%v]", err)
	}
}

func TestUnknownStatus(t *testing.T) {
	mgr, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv)
	opKey := OperationKey{
		Name:       "unknown-status-operation",
		ResourceID: types.UID("uid"),
	}
	mgr.OperationStart(opKey, NewOperationValue("driver", DynamicSnapshotType))
	// should create a Unknown data point with latency ~300ms
	time.Sleep(300 * time.Millisecond)
	mgr.RecordMetrics(opKey, nil, "driver")
	expected :=
		`# HELP snapshot_controller_operation_total_seconds [ALPHA] Total number of seconds spent by the controller on an operation from end to end
# TYPE snapshot_controller_operation_total_seconds histogram
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="0.25"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type=""} 0.3
snapshot_controller_operation_total_seconds_count{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",snapshot_type=""} 1
`

	if err := verifyMetric(expected, srvAddr); err != nil {
		t.Errorf("failed testing[%v]", err)
	}
}

func TestRecordMetrics(t *testing.T) {
	mgr, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv)
	// add an operation
	opKey := OperationKey{
		Name:       "op1",
		ResourceID: types.UID("uid1"),
	}
	opVal := NewOperationValue("driver", DynamicSnapshotType)
	mgr.OperationStart(opKey, opVal)
	// should create a Success data point with latency ~ 1100ms
	time.Sleep(1100 * time.Millisecond)
	success := &fakeOpStatus{
		statusCode: 0,
	}
	mgr.RecordMetrics(opKey, success, "driver")

	// add another operation metric
	opKey.Name = "op2"
	opKey.ResourceID = types.UID("uid2")
	mgr.OperationStart(opKey, opVal)
	// should create a Failure data point with latency ~ 100ms
	time.Sleep(100 * time.Millisecond)
	failure := &fakeOpStatus{
		statusCode: 1,
	}
	mgr.RecordMetrics(opKey, failure, "driver")

	expected :=
		`# HELP snapshot_controller_operation_total_seconds [ALPHA] Total number of seconds spent by the controller on an operation from end to end
# TYPE snapshot_controller_operation_total_seconds histogram
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="0.25"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="0.5"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type=""} 1.1
snapshot_controller_operation_total_seconds_count{driver_name="driver1",operation_name="op1",operation_status="Success",snapshot_type=""} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type=""} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver2",operation_name="op2",operation_status="Failure",snapshot_type=""} 1
`
	if err := verifyMetric(expected, srvAddr); err != nil {
		t.Errorf("failed testing [%v]", err)
	}
}

func TestConcurrency(t *testing.T) {
	mgr, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv)
	success := &fakeOpStatus{
		statusCode: 0,
	}
	failure := &fakeOpStatus{
		statusCode: 1,
	}
	ops := []struct {
		op               OperationKey
		desiredLatencyMs time.Duration
		status           OperationStatus
		drop             bool
	}{
		{
			OperationKey{
				Name:       "success1",
				ResourceID: types.UID("uid1"),
			},
			100,
			success,
			false,
		},
		{
			OperationKey{
				Name:       "success2",
				ResourceID: types.UID("uid2"),
			},
			100,
			success,
			false,
		},
		{
			OperationKey{
				Name:       "failure1",
				ResourceID: types.UID("uid3"),
			},
			100,
			failure,
			false,
		},
		{
			OperationKey{
				Name:       "failure2",
				ResourceID: types.UID("uid4"),
			},
			100,
			failure,
			false,
		},
		{
			OperationKey{
				Name:       "unknown",
				ResourceID: types.UID("uid5"),
			},
			100,
			nil,
			false,
		},
		{
			OperationKey{
				Name:       "drop-from-cache",
				ResourceID: types.UID("uid6"),
			},
			100,
			nil,
			true,
		},
	}

	for i := range ops {
		mgr.OperationStart(ops[i].op, OperationValue{
			Driver:       fmt.Sprintf("driver%v", i),
			SnapshotType: string(DynamicSnapshotType),
		})
	}
	// add an extra operation which should remain in the cache
	remaining := OperationKey{
		Name:       "remaining-in-cache",
		ResourceID: types.UID("uid7"),
	}
	mgr.OperationStart(remaining, OperationValue{
		Driver:       "driver7",
		SnapshotType: string(DynamicSnapshotType),
	})

	var wgMetrics sync.WaitGroup

	for i := range ops {
		wgMetrics.Add(1)
		go func(i int) {
			defer wgMetrics.Done()
			if ops[i].desiredLatencyMs > 0 {
				time.Sleep(ops[i].desiredLatencyMs * time.Millisecond)
			}
			if ops[i].drop {
				mgr.DropOperation(ops[i].op)
			} else {
				mgr.RecordMetrics(ops[i].op, ops[i].status, fmt.Sprintf("driver%v", i))
			}
		}(i)
	}
	wgMetrics.Wait()

	// validate
	expected :=
		`# HELP snapshot_controller_operation_total_seconds [ALPHA] Total number of seconds spent by the controller on an operation from end to end
# TYPE snapshot_controller_operation_total_seconds histogram
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type=""} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver1",operation_name="success1",operation_status="Success",snapshot_type=""} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type=""} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver2",operation_name="success2",operation_status="Success",snapshot_type=""} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type=""} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver3",operation_name="failure1",operation_status="Failure",snapshot_type=""} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type=""} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver4",operation_name="failure2",operation_status="Failure",snapshot_type=""} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type="",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type=""} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver5",operation_name="unknown",operation_status="Unknown",snapshot_type=""} 1
`
	if err := verifyMetric(expected, srvAddr); err != nil {
		t.Errorf("failed testing [%v]", err)
	}
}

func TestInFlightMetric(t *testing.T) {
	inFlightCheckInterval = time.Millisecond * 50

	mgr, srv := initMgr()
	defer shutdown(srv)
	srvAddr := "http://" + srv.Addr + httpPattern

	//  Start first operation, should be 1
	opKey := OperationKey{
		Name:       "leaked",
		ResourceID: types.UID("uid"),
	}
	opVal := NewOperationValue("driver", "test")
	mgr.OperationStart(opKey, opVal)
	time.Sleep(500 * time.Millisecond)

	if err := verifyInFlightMetric(`snapshot_controller_operations_in_flight 1`, srvAddr); err != nil {
		t.Errorf("failed testing [%v]", err)
	}

	//  Start second operation, should be 2
	opKey = OperationKey{
		Name:       "leaked2",
		ResourceID: types.UID("uid"),
	}
	opVal = NewOperationValue("driver2", "test2")
	mgr.OperationStart(opKey, opVal)
	time.Sleep(500 * time.Millisecond)

	if err := verifyInFlightMetric(`snapshot_controller_operations_in_flight 2`, srvAddr); err != nil {
		t.Errorf("failed testing [%v]", err)
	}

	//  Record, should be down to 1
	mgr.RecordMetrics(opKey, nil, "driver")
	time.Sleep(500 * time.Millisecond)

	if err := verifyInFlightMetric(`snapshot_controller_operations_in_flight 1`, srvAddr); err != nil {
		t.Errorf("failed testing [%v]", err)
	}

	//  Start 50 operations, should be 51
	for i := 0; i < 50; i++ {
		opKey := OperationKey{
			Name:       fmt.Sprintf("op%d", i),
			ResourceID: types.UID("uid%d"),
		}
		opVal := NewOperationValue("driver1", "test")
		mgr.OperationStart(opKey, opVal)
	}
	time.Sleep(500 * time.Millisecond)

	if err := verifyInFlightMetric(`snapshot_controller_operations_in_flight 51`, srvAddr); err != nil {
		t.Errorf("failed testing [%v]", err)
	}
}

func verifyInFlightMetric(expected string, srvAddr string) error {
	rsp, err := http.Get(srvAddr)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get response from serve: %s", http.StatusText(rsp.StatusCode))
	}
	r, err := io.ReadAll(rsp.Body)
	if err != nil {
		return err
	}

	if !strings.Contains(string(r), expected) {
		return fmt.Errorf("failed, not equal")
	}

	return nil
}

func verifyMetric(expected, srvAddr string) error {
	rsp, err := http.Get(srvAddr)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get response from serve: %s", http.StatusText(rsp.StatusCode))
	}
	r, err := io.ReadAll(rsp.Body)
	if err != nil {
		return err
	}

	format := expfmt.ResponseFormat(rsp.Header)
	gotReader := strings.NewReader(string(r))
	gotDecoder := expfmt.NewDecoder(gotReader, format)
	expectedReader := strings.NewReader(expected)
	expectedDecoder := expfmt.NewDecoder(expectedReader, format)

	gotMfs := []*cmg.MetricFamily{}
	expectedMfs := []*cmg.MetricFamily{}
	for {
		gotMf := &cmg.MetricFamily{}
		gotErr := gotDecoder.Decode(gotMf)
		expectedMf := &cmg.MetricFamily{}
		if expectedErr := expectedDecoder.Decode(expectedMf); expectedErr != nil {
			// return correctly if both are EOF
			if expectedErr == io.EOF && gotErr == io.EOF {
				break
			} else {
				return err
			}
		}
		gotMfs = append(gotMfs, gotMf)
		expectedMfs = append(expectedMfs, expectedMf)
	}

	if !containsMetrics(expectedMfs, gotMfs) {
		return fmt.Errorf("failed testing, expected\n%s\n, got\n%s\n", expected, string(r))
	}

	return nil
}

// sortMfs, sorts metric families in alphabetical order by type.
// currently only supports counter and histogram
func sortMfs(mfs []*cmg.MetricFamily) []*cmg.MetricFamily {
	var sortedMfs []*cmg.MetricFamily

	// Sort first by type
	sort.Slice(mfs, func(i, j int) bool {
		return *mfs[i].Type < *mfs[j].Type
	})

	// Next, sort by length of name
	sort.Slice(mfs, func(i, j int) bool {
		return len(*mfs[i].Name) < len(*mfs[j].Name)
	})

	return sortedMfs
}

func containsMetrics(expectedMfs, gotMfs []*cmg.MetricFamily) bool {
	if len(gotMfs) != len(expectedMfs) {
		fmt.Printf("Not same length: expected and got metrics families: %v vs. %v\n", len(expectedMfs), len(gotMfs))
		return false
	}

	// sort metric families for deterministic comparison.
	sortedExpectedMfs := sortMfs(expectedMfs)
	sortedGotMfs := sortMfs(gotMfs)

	// compare expected vs. sorted actual metrics
	for k, got := range sortedGotMfs {
		matchCount := 0
		expected := sortedExpectedMfs[k]

		if (got.Name == nil || *(got.Name) != *(expected.Name)) ||
			(got.Type == nil || *(got.Type) != *(expected.Type)) ||
			(got.Help == nil || *(got.Help) != *(expected.Help)) {
			fmt.Printf("invalid header info: got: %v, expected: %v\n", *got.Name, *expected.Name)
			fmt.Printf("invalid header info: got: %v, expected: %v\n", *got.Type, *expected.Type)
			fmt.Printf("invalid header info: got: %v, expected: %v\n", *got.Help, *expected.Help)
			return false
		}

		numRecords := len(expected.Metric)
		if len(got.Metric) < numRecords {
			fmt.Printf("Not the same number of records: got.Metric: %v, numRecords: %v\n", len(got.Metric), numRecords)
			return false
		}
		for i := 0; i < len(got.Metric); i++ {
			for j := 0; j < numRecords; j++ {
				if got.Metric[i].Histogram == nil && expected.Metric[j].Histogram != nil ||
					got.Metric[i].Histogram != nil && expected.Metric[j].Histogram == nil {
					fmt.Printf("got metric and expected metric histogram type mismatch")
					return false
				}

				// labels should be the same
				if !reflect.DeepEqual(got.Metric[i].Label, expected.Metric[j].Label) {
					continue
				}

				// metric type specific checks
				switch {
				case got.Metric[i].Histogram != nil && expected.Metric[j].Histogram != nil:
					gh := got.Metric[i].Histogram
					eh := expected.Metric[j].Histogram
					if gh == nil || eh == nil {
						continue
					}
					if !reflect.DeepEqual(gh.Bucket, eh.Bucket) {
						fmt.Println("got and expected histogram bucket not equal")
						continue
					}

					// this is a sum record, expecting a latency which is more than the
					// expected one. If the sum is smaller than expected, it will be considered
					// as NOT a match
					if gh.SampleSum == nil || eh.SampleSum == nil || *(gh.SampleSum) < *(eh.SampleSum) {
						fmt.Println("difference in sample sum")
						continue
					}
					if gh.SampleCount == nil || eh.SampleCount == nil || *(gh.SampleCount) != *(eh.SampleCount) {
						fmt.Println("difference in sample count")
						continue
					}

				case got.Metric[i].Counter != nil && expected.Metric[j].Counter != nil:
					gc := got.Metric[i].Counter
					ec := expected.Metric[j].Counter
					if gc.Value == nil || *(gc.Value) != *(ec.Value) {
						fmt.Println("difference in counter values")
						continue
					}
				}

				// this is a match
				matchCount = matchCount + 1
				break
			}
		}

		if matchCount != numRecords {
			fmt.Printf("matchCount %v, numRecords %v\n", matchCount, numRecords)
			return false
		}
	}

	return true
}

func TestProcessStartTimeMetricExist(t *testing.T) {
	mgr, srv := initMgr()
	defer shutdown(srv)
	metricsFamilies, err := mgr.GetRegistry().Gather()
	if err != nil {
		t.Fatalf("Error fetching metrics: %v", err)
	}

	for _, metricsFamily := range metricsFamilies {
		if metricsFamily.GetName() == processStartTimeMetric {
			return
		}
		m := metricsFamily.GetMetric()
		if m[0].GetGauge().GetValue() <= 0 {
			t.Fatalf("Expected non zero timestamp for process start time")
		}
	}

	t.Fatalf("Metrics does not contain %v. Scraped content: %v", processStartTimeMetric, metricsFamilies)
}
