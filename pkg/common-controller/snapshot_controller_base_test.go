/*
Copyright 2026 The Kubernetes Authors.

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

package common_controller

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	informers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/metrics"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

// shutdownTrackingBroadcaster counts Shutdown calls on a real broadcaster.
type shutdownTrackingBroadcaster struct {
	record.EventBroadcaster
	shutdownCalls atomic.Int32
}

func (b *shutdownTrackingBroadcaster) Shutdown() {
	b.shutdownCalls.Add(1)
	b.EventBroadcaster.Shutdown()
}

func neverReady() bool { return false }

func newRunTestController() *csiSnapshotCommonController {
	// The snapshot fake has no NewClientset because apply configurations are not generated.
	client := fake.NewSimpleClientset()
	kubeClient := kubefake.NewClientset()
	informerFactory := informers.NewSharedInformerFactory(client, utils.NoResyncPeriodFunc())
	coreFactory := coreinformers.NewSharedInformerFactory(kubeClient, utils.NoResyncPeriodFunc())
	newRateLimiter := func() workqueue.TypedRateLimiter[string] {
		return workqueue.NewTypedItemExponentialFailureRateLimiter[string](1*time.Millisecond, 1*time.Minute)
	}

	return NewCSISnapshotCommonController(
		client,
		kubeClient,
		informerFactory.Snapshot().V1().VolumeSnapshots(),
		informerFactory.Snapshot().V1().VolumeSnapshotContents(),
		informerFactory.Snapshot().V1().VolumeSnapshotClasses(),
		nil,
		nil,
		nil,
		coreFactory.Core().V1().PersistentVolumeClaims(),
		coreFactory.Core().V1().PersistentVolumes(),
		nil,
		metrics.NewMetricsManager(),
		60*time.Second,
		newRateLimiter(),
		newRateLimiter(),
		newRateLimiter(),
		newRateLimiter(),
		false,
		false,
		false,
	)
}

func TestRunShutsDownEventBroadcaster(t *testing.T) {
	tests := []struct {
		name         string
		snapshotSync func() bool
	}{
		{
			name:         "stop after caches synced",
			snapshotSync: alwaysReady,
		},
		{
			name:         "cache sync failure",
			snapshotSync: neverReady,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := newRunTestController()
			broadcaster := &shutdownTrackingBroadcaster{EventBroadcaster: ctrl.eventBroadcaster}
			ctrl.eventBroadcaster = broadcaster

			ctrl.snapshotListerSynced = test.snapshotSync
			ctrl.contentListerSynced = alwaysReady
			ctrl.classListerSynced = alwaysReady
			ctrl.pvcListerSynced = alwaysReady
			ctrl.pvListerSynced = alwaysReady

			// WaitForCacheSync checks the synced funcs once before it looks at stopCh, so a closed stopCh is deterministic.
			stopCh := make(chan struct{})
			close(stopCh)

			done := make(chan struct{})
			go func() {
				defer close(done)
				ctrl.Run(1, stopCh, &sync.WaitGroup{})
			}()

			select {
			case <-done:
			case <-time.After(wait.ForeverTestTimeout):
				t.Fatal("Run did not return after stopCh was closed")
			}

			if got := broadcaster.shutdownCalls.Load(); got != 1 {
				t.Errorf("expected event broadcaster Shutdown to be called once, got %d", got)
			}
		})
	}
}
