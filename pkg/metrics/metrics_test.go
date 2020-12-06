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
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
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
	httpPattern = "/metrics"
	addr        = "localhost:0"
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

func initMgr() (MetricsManager, *sync.WaitGroup, *http.Server) {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	mgr := NewMetricsManager()
	srv, err := mgr.StartMetricsEndpoint(httpPattern, addr, nil, wg)
	if err != nil {
		log.Fatalf("failed to start serving [%v]", err)
	}
	return mgr, wg, srv
}

func shutdown(srv *http.Server, wg *sync.WaitGroup) {
	if err := srv.Shutdown(context.Background()); err != nil {
		panic(err)
	}
	wg.Wait()
}

func TestNew(t *testing.T) {
	mgr, wg, srv := initMgr()
	defer shutdown(srv, wg)
	if mgr == nil {
		t.Errorf("failed testing new")
	}
}

func TestDropNonExistingOperation(t *testing.T) {
	mgr, wg, srv := initMgr()
	defer shutdown(srv, wg)
	op := Operation{
		Name:       "drop-non-existing-operation-should-be-noop",
		Driver:     "driver",
		ResourceID: types.UID("uid"),
	}
	mgr.DropOperation(op)
}

func TestRecordMetricsForNonExistingOperation(t *testing.T) {
	mgr, wg, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv, wg)
	op := Operation{
		Name:       "no-metrics-should-be-recorded-as-operation-did-not-start",
		Driver:     "driver",
		ResourceID: types.UID("uid"),
	}
	mgr.RecordMetrics(op, nil)
	rsp, err := http.Get(srvAddr)
	if err != nil || rsp.StatusCode != http.StatusOK {
		t.Errorf("failed to get response from server %v, %v", err, rsp)
	}
	r, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		t.Errorf("failed to read response body %v", err)
	}
	if strings.Contains(string(r), op.Name) {
		t.Errorf("found metric should have been dropped for operation [%s] [%s]", op.Name, string(r))
	}
}

func TestDropOperation(t *testing.T) {
	mgr, wg, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv, wg)
	op := Operation{
		Name:       "should-have-been-dropped",
		Driver:     "driver",
		ResourceID: types.UID("uid"),
	}
	mgr.OperationStart(op)
	mgr.DropOperation(op)
	time.Sleep(300 * time.Millisecond)
	rsp, err := http.Get(srvAddr)
	if err != nil || rsp.StatusCode != http.StatusOK {
		t.Errorf("failed to get response from server %v, %v", err, rsp)
	}
	r, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		t.Errorf("failed to read response body %v", err)
	}
	if strings.Contains(string(r), op.Name) {
		t.Errorf("found metric should have been dropped for operation [%s] [%s]", op.Name, string(r))
	}
	// re-add with a different name
	op.Name = "should-have-been-added"
	mgr.OperationStart(op)
	time.Sleep(300 * time.Millisecond)
	opStatus := &fakeOpStatus{
		statusCode: 0,
	}
	mgr.RecordMetrics(op, opStatus)
	expected :=
		`# HELP snapshot_controller_operation_total_seconds [ALPHA] Total number of seconds spent by the controller on an operation from end to end
# TYPE snapshot_controller_operation_total_seconds histogram
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="0.25"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="should-have-been-added",operation_status="Success",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver",operation_name="should-have-been-added",operation_status="Success"} 0.3
snapshot_controller_operation_total_seconds_count{driver_name="driver",operation_name="should-have-been-added",operation_status="Success"} 1
`

	if err := verifyMetric(expected, srvAddr); err != nil {
		t.Errorf("failed testing[%v]", err)
	}
}

func TestUnknownStatus(t *testing.T) {
	mgr, wg, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv, wg)
	op := Operation{
		Name:       "unknown-status-operation",
		Driver:     "driver",
		ResourceID: types.UID("uid"),
	}
	mgr.OperationStart(op)
	// should create a Unknown data point with latency ~300ms
	time.Sleep(300 * time.Millisecond)
	mgr.RecordMetrics(op, nil)
	expected :=
		`# HELP snapshot_controller_operation_total_seconds [ALPHA] Total number of seconds spent by the controller on an operation from end to end
# TYPE snapshot_controller_operation_total_seconds histogram
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="0.25"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown"} 0.3
snapshot_controller_operation_total_seconds_count{driver_name="driver",operation_name="unknown-status-operation",operation_status="Unknown"} 1
`

	if err := verifyMetric(expected, srvAddr); err != nil {
		t.Errorf("failed testing[%v]", err)
	}
}

func TestRecordMetrics(t *testing.T) {
	mgr, wg, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv, wg)
	// add an operation
	op := Operation{
		Name:       "op1",
		Driver:     "driver1",
		ResourceID: types.UID("uid1"),
	}
	mgr.OperationStart(op)
	// should create a Success data point with latency ~ 1100ms
	time.Sleep(1100 * time.Millisecond)
	success := &fakeOpStatus{
		statusCode: 0,
	}
	mgr.RecordMetrics(op, success)

	// add another operation metric
	op.Name = "op2"
	op.Driver = "driver2"
	op.ResourceID = types.UID("uid2")
	mgr.OperationStart(op)
	// should create a Failure data point with latency ~ 100ms
	time.Sleep(100 * time.Millisecond)
	failure := &fakeOpStatus{
		statusCode: 1,
	}
	mgr.RecordMetrics(op, failure)

	expected :=
		`# HELP snapshot_controller_operation_total_seconds [ALPHA] Total number of seconds spent by the controller on an operation from end to end
# TYPE snapshot_controller_operation_total_seconds histogram
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="0.25"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="0.5"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="op1",operation_status="Success",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver1",operation_name="op1",operation_status="Success"} 1.1
snapshot_controller_operation_total_seconds_count{driver_name="driver1",operation_name="op1",operation_status="Success"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="op2",operation_status="Failure",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver2",operation_name="op2",operation_status="Failure"} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver2",operation_name="op2",operation_status="Failure"} 1
`
	if err := verifyMetric(expected, srvAddr); err != nil {
		t.Errorf("failed testing [%v]", err)
	}
}

func TestConcurrency(t *testing.T) {
	mgr, wg, srv := initMgr()
	srvAddr := "http://" + srv.Addr + httpPattern
	defer shutdown(srv, wg)
	success := &fakeOpStatus{
		statusCode: 0,
	}
	failure := &fakeOpStatus{
		statusCode: 1,
	}
	ops := []struct {
		op               Operation
		desiredLatencyMs time.Duration
		status           OperationStatus
		drop             bool
	}{
		{
			Operation{
				Name:       "success1",
				Driver:     "driver1",
				ResourceID: types.UID("uid1"),
			},
			100,
			success,
			false,
		},
		{
			Operation{
				Name:       "success2",
				Driver:     "driver2",
				ResourceID: types.UID("uid2"),
			},
			100,
			success,
			false,
		},
		{
			Operation{
				Name:       "failure1",
				Driver:     "driver3",
				ResourceID: types.UID("uid3"),
			},
			100,
			failure,
			false,
		},
		{
			Operation{
				Name:       "failure2",
				Driver:     "driver4",
				ResourceID: types.UID("uid4"),
			},
			100,
			failure,
			false,
		},
		{
			Operation{
				Name:       "unknown",
				Driver:     "driver5",
				ResourceID: types.UID("uid5"),
			},
			100,
			nil,
			false,
		},
		{
			Operation{
				Name:       "drop-from-cache",
				Driver:     "driver6",
				ResourceID: types.UID("uid6"),
			},
			100,
			nil,
			true,
		},
	}

	for i := range ops {
		mgr.OperationStart(ops[i].op)
	}
	// add an extra operation which should remain in the cache
	remaining := Operation{
		Name:       "remaining-in-cache",
		Driver:     "driver7",
		ResourceID: types.UID("uid7"),
	}
	mgr.OperationStart(remaining)

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
				mgr.RecordMetrics(ops[i].op, ops[i].status)
			}
		}(i)
	}
	wgMetrics.Wait()

	// validate
	expected :=
		`# HELP snapshot_controller_operation_total_seconds [ALPHA] Total number of seconds spent by the controller on an operation from end to end
# TYPE snapshot_controller_operation_total_seconds histogram
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver1",operation_name="success1",operation_status="Success",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver1",operation_name="success1",operation_status="Success"} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver1",operation_name="success1",operation_status="Success"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver2",operation_name="success2",operation_status="Success",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver2",operation_name="success2",operation_status="Success"} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver2",operation_name="success2",operation_status="Success"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver3",operation_name="failure1",operation_status="Failure",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver3",operation_name="failure1",operation_status="Failure"} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver3",operation_name="failure1",operation_status="Failure"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver4",operation_name="failure2",operation_status="Failure",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver4",operation_name="failure2",operation_status="Failure"} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver4",operation_name="failure2",operation_status="Failure"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="0.1"} 0
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="0.25"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="0.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="1"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="2.5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="5"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="10"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="15"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="30"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="60"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="120"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="300"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="600"} 1
snapshot_controller_operation_total_seconds_bucket{driver_name="driver5",operation_name="unknown",operation_status="Unknown",le="+Inf"} 1
snapshot_controller_operation_total_seconds_sum{driver_name="driver5",operation_name="unknown",operation_status="Unknown"} 0.1
snapshot_controller_operation_total_seconds_count{driver_name="driver5",operation_name="unknown",operation_status="Unknown"} 1
`
	if err := verifyMetric(expected, srvAddr); err != nil {
		t.Errorf("failed testing [%v]", err)
	}
}

func verifyMetric(expected, srvAddr string) error {
	rsp, err := http.Get(srvAddr)
	if err != nil {
		return err
	}
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get response from serve: %s", http.StatusText(rsp.StatusCode))
	}
	r, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return err
	}
	format := expfmt.ResponseFormat(rsp.Header)
	reader := strings.NewReader(string(r))
	decoder := expfmt.NewDecoder(reader, format)
	mf := &cmg.MetricFamily{}
	if err := decoder.Decode(mf); err != nil {
		return err
	}
	reader = strings.NewReader(expected)
	decoder = expfmt.NewDecoder(reader, format)
	expectedMf := &cmg.MetricFamily{}
	if err := decoder.Decode(expectedMf); err != nil {
		return err
	}
	if !containsMetrics(expectedMf, mf) {
		return fmt.Errorf("failed testing, expected\n%s\n, got\n%s\n", expected, string(r))
	}
	return nil
}

func containsMetrics(expected, got *cmg.MetricFamily) bool {
	if (got.Name == nil || *(got.Name) != *(expected.Name)) ||
		(got.Type == nil || *(got.Type) != *(expected.Type)) ||
		(got.Help == nil || *(got.Help) != *(expected.Help)) {
		return false
	}

	numRecords := len(expected.Metric)
	if len(got.Metric) < numRecords {
		return false
	}
	matchCount := 0
	for i := 0; i < len(got.Metric); i++ {
		for j := 0; j < numRecords; j++ {
			// labels should be the same
			if !reflect.DeepEqual(got.Metric[i].Label, expected.Metric[j].Label) {
				continue
			}
			gh := got.Metric[i].Histogram
			eh := expected.Metric[j].Histogram
			if gh == nil {
				continue
			}
			if !reflect.DeepEqual(gh.Bucket, eh.Bucket) {
				continue
			}
			// this is a sum record, expecting a latency which is more than the
			// expected one. If the sum is smaller than expected, it will be considered
			// as NOT a match
			if gh.SampleSum == nil || *(gh.SampleSum) < *(eh.SampleSum) {
				continue
			}
			if gh.SampleCount == nil || *(gh.SampleCount) != *(eh.SampleCount) {
				continue
			}
			// this is a match
			matchCount = matchCount + 1
			break
		}
	}
	return matchCount == numRecords
}
