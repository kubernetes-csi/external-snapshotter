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
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/glog"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	clientset "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	"github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned/fake"
	informers "github.com/kubernetes-csi/external-snapshotter/pkg/client/informers/externalversions"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/pkg/client/listers/volumesnapshot/v1alpha1"
	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storagev1beta1 "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

// This is a unit test framework for snapshot controller.
// It fills the controller with test snapshots/contents and can simulate these
// scenarios:
// 1) Call syncSnapshot/syncContent once.
// 2) Call syncSnapshot/syncContent several times (both simulating "snapshot/content
//    modified" events and periodic sync), until the controller settles down and
//    does not modify anything.
// 3) Simulate almost real API server/etcd and call add/update/delete
//    content/snapshot.
// In all these scenarios, when the test finishes, the framework can compare
// resulting snapshots/contents with list of expected snapshots/contents and report
// differences.

// controllerTest contains a single controller test input.
// Each test has initial set of contents and snapshots that are filled into the
// controller before the test starts. The test then contains a reference to
// function to call as the actual test. Available functions are:
//   - testSyncSnapshot - calls syncSnapshot on the first snapshot in initialSnapshots.
//   - testSyncSnapshotError - calls syncSnapshot on the first snapshot in initialSnapshots
//                          and expects an error to be returned.
//   - testSyncContent - calls syncContent on the first content in initialContents.
//   - any custom function for specialized tests.
// The test then contains list of contents/snapshots that are expected at the end
// of the test and list of generated events.
type controllerTest struct {
	// Name of the test, for logging
	name string
	// Initial content of controller content cache.
	initialContents []*crdv1.VolumeSnapshotContent
	// Expected content of controller content cache at the end of the test.
	expectedContents []*crdv1.VolumeSnapshotContent
	// Initial content of controller snapshot cache.
	initialSnapshots []*crdv1.VolumeSnapshot
	// Expected content of controller snapshot cache at the end of the test.
	expectedSnapshots []*crdv1.VolumeSnapshot
	// Initial content of controller volume cache.
	initialVolumes []*v1.PersistentVolume
	// Initial content of controller claim cache.
	initialClaims []*v1.PersistentVolumeClaim
	// Initial content of controller StorageClass cache.
	initialStorageClasses []*storagev1.StorageClass
	// Initial content of controller Secret cache.
	initialSecrets []*v1.Secret
	// Expected events - any event with prefix will pass, we don't check full
	// event message.
	expectedEvents []string
	// Errors to produce on matching action
	errors []reactorError
	// List of expected CSI Create snapshot calls
	expectedCreateCalls []createCall
	// List of expected CSI Delete snapshot calls
	expectedDeleteCalls []deleteCall
	// List of expected CSI list snapshot calls
	expectedListCalls []listCall
	// Function to call as the test.
	test testCall
}

type testCall func(ctrl *csiSnapshotController, reactor *snapshotReactor, test controllerTest) error

const testNamespace = "default"
const mockDriverName = "csi-mock-plugin"

var versionConflictError = errors.New("VersionError")
var nocontents []*crdv1.VolumeSnapshotContent
var nosnapshots []*crdv1.VolumeSnapshot
var noevents = []string{}
var noerrors = []reactorError{}

// snapshotReactor is a core.Reactor that simulates etcd and API server. It
// stores:
// - Latest version of snapshots contents saved by the controller.
// - Queue of all saves (to simulate "content/snapshot updated" events). This queue
//   contains all intermediate state of an object - e.g. a snapshot.VolumeName
//   is updated first and snapshot.Phase second. This queue will then contain both
//   updates as separate entries.
// - Number of changes since the last call to snapshotReactor.syncAll().
// - Optionally, content and snapshot fake watchers which should be the same ones
//   used by the controller. Any time an event function like deleteContentEvent
//   is called to simulate an event, the reactor's stores are updated and the
//   controller is sent the event via the fake watcher.
// - Optionally, list of error that should be returned by reactor, simulating
//   etcd / API server failures. These errors are evaluated in order and every
//   error is returned only once. I.e. when the reactor finds matching
//   reactorError, it return appropriate error and removes the reactorError from
//   the list.
type snapshotReactor struct {
	secrets              map[string]*v1.Secret
	storageClasses       map[string]*storagev1.StorageClass
	volumes              map[string]*v1.PersistentVolume
	claims               map[string]*v1.PersistentVolumeClaim
	contents             map[string]*crdv1.VolumeSnapshotContent
	snapshots            map[string]*crdv1.VolumeSnapshot
	changedObjects       []interface{}
	changedSinceLastSync int
	ctrl                 *csiSnapshotController
	fakeContentWatch     *watch.FakeWatcher
	fakeSnapshotWatch    *watch.FakeWatcher
	lock                 sync.Mutex
	errors               []reactorError
}

// reactorError is an error that is returned by test reactor (=simulated
// etcd+/API server) when an action performed by the reactor matches given verb
// ("get", "update", "create", "delete" or "*"") on given resource
// ("volumesnapshotcontents", "volumesnapshots" or "*").
type reactorError struct {
	verb     string
	resource string
	error    error
}

// React is a callback called by fake kubeClient from the controller.
// In other words, every snapshot/content change performed by the controller ends
// here.
// This callback checks versions of the updated objects and refuse those that
// are too old (simulating real etcd).
// All updated objects are stored locally to keep track of object versions and
// to evaluate test results.
// All updated objects are also inserted into changedObjects queue and
// optionally sent back to the controller via its watchers.
func (r *snapshotReactor) React(action core.Action) (handled bool, ret runtime.Object, err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	glog.V(4).Infof("reactor got operation %q on %q", action.GetVerb(), action.GetResource())

	// Inject error when requested
	err = r.injectReactError(action)
	if err != nil {
		return true, nil, err
	}

	// Test did not request to inject an error, continue simulating API server.
	switch {
	case action.Matches("create", "volumesnapshotcontents"):
		obj := action.(core.UpdateAction).GetObject()
		content := obj.(*crdv1.VolumeSnapshotContent)

		// check the content does not exist
		_, found := r.contents[content.Name]
		if found {
			return true, nil, fmt.Errorf("cannot create content %s: content already exists", content.Name)
		}

		// Store the updated object to appropriate places.
		r.contents[content.Name] = content
		r.changedObjects = append(r.changedObjects, content)
		r.changedSinceLastSync++
		glog.V(5).Infof("created content %s", content.Name)
		return true, content, nil

	case action.Matches("update", "volumesnapshotcontents"):
		obj := action.(core.UpdateAction).GetObject()
		content := obj.(*crdv1.VolumeSnapshotContent)

		// Check and bump object version
		storedVolume, found := r.contents[content.Name]
		if found {
			storedVer, _ := strconv.Atoi(storedVolume.ResourceVersion)
			requestedVer, _ := strconv.Atoi(content.ResourceVersion)
			if storedVer != requestedVer {
				return true, obj, versionConflictError
			}
			// Don't modify the existing object
			content = content.DeepCopy()
			content.ResourceVersion = strconv.Itoa(storedVer + 1)
		} else {
			return true, nil, fmt.Errorf("cannot update content %s: content not found", content.Name)
		}

		// Store the updated object to appropriate places.
		r.contents[content.Name] = content
		r.changedObjects = append(r.changedObjects, content)
		r.changedSinceLastSync++
		glog.V(4).Infof("saved updated content %s", content.Name)
		return true, content, nil

	case action.Matches("update", "volumesnapshots"):
		obj := action.(core.UpdateAction).GetObject()
		snapshot := obj.(*crdv1.VolumeSnapshot)

		// Check and bump object version
		storedSnapshot, found := r.snapshots[snapshot.Name]
		if found {
			storedVer, _ := strconv.Atoi(storedSnapshot.ResourceVersion)
			requestedVer, _ := strconv.Atoi(snapshot.ResourceVersion)
			if storedVer != requestedVer {
				return true, obj, versionConflictError
			}
			// Don't modify the existing object
			snapshot = snapshot.DeepCopy()
			snapshot.ResourceVersion = strconv.Itoa(storedVer + 1)
		} else {
			return true, nil, fmt.Errorf("cannot update snapshot %s: snapshot not found", snapshot.Name)
		}

		// Store the updated object to appropriate places.
		r.snapshots[snapshot.Name] = snapshot
		r.changedObjects = append(r.changedObjects, snapshot)
		r.changedSinceLastSync++
		glog.V(4).Infof("saved updated snapshot %s", snapshot.Name)
		return true, snapshot, nil

	case action.Matches("get", "volumesnapshotcontents"):
		name := action.(core.GetAction).GetName()
		content, found := r.contents[name]
		if found {
			glog.V(4).Infof("GetVolume: found %s", content.Name)
			return true, content, nil
		} else {
			glog.V(4).Infof("GetVolume: content %s not found", name)
			return true, nil, fmt.Errorf("cannot find content %s", name)
		}

	case action.Matches("get", "volumesnapshots"):
		name := action.(core.GetAction).GetName()
		snapshot, found := r.snapshots[name]
		if found {
			glog.V(4).Infof("GetSnapshot: found %s", snapshot.Name)
			return true, snapshot, nil
		} else {
			glog.V(4).Infof("GetSnapshot: content %s not found", name)
			return true, nil, fmt.Errorf("cannot find snapshot %s", name)
		}

	case action.Matches("delete", "volumesnapshotcontents"):
		name := action.(core.DeleteAction).GetName()
		glog.V(4).Infof("deleted content %s", name)
		_, found := r.contents[name]
		if found {
			delete(r.contents, name)
			r.changedSinceLastSync++
			return true, nil, nil
		} else {
			return true, nil, fmt.Errorf("cannot delete content %s: not found", name)
		}

	case action.Matches("delete", "volumesnapshots"):
		name := action.(core.DeleteAction).GetName()
		glog.V(4).Infof("deleted snapshot %s", name)
		_, found := r.contents[name]
		if found {
			delete(r.snapshots, name)
			r.changedSinceLastSync++
			return true, nil, nil
		} else {
			return true, nil, fmt.Errorf("cannot delete snapshot %s: not found", name)
		}

	case action.Matches("get", "persistentvolumes"):
		name := action.(core.GetAction).GetName()
		volume, found := r.volumes[name]
		if found {
			glog.V(4).Infof("GetVolume: found %s", volume.Name)
			return true, volume, nil
		} else {
			glog.V(4).Infof("GetVolume: volume %s not found", name)
			return true, nil, fmt.Errorf("cannot find volume %s", name)
		}

	case action.Matches("get", "persistentvolumeclaims"):
		name := action.(core.GetAction).GetName()
		claim, found := r.claims[name]
		if found {
			glog.V(4).Infof("GetClaim: found %s", claim.Name)
			return true, claim, nil
		} else {
			glog.V(4).Infof("GetClaim: claim %s not found", name)
			return true, nil, fmt.Errorf("cannot find claim %s", name)
		}

	case action.Matches("get", "storageclasses"):
		name := action.(core.GetAction).GetName()
		storageClass, found := r.storageClasses[name]
		if found {
			glog.V(4).Infof("GetStorageClass: found %s", storageClass.Name)
			return true, storageClass, nil
		} else {
			glog.V(4).Infof("GetStorageClass: storageClass %s not found", name)
			return true, nil, fmt.Errorf("cannot find storageClass %s", name)
		}

	case action.Matches("get", "secrets"):
		name := action.(core.GetAction).GetName()
		secret, found := r.secrets[name]
		if found {
			glog.V(4).Infof("GetSecret: found %s", secret.Name)
			return true, secret, nil
		} else {
			glog.V(4).Infof("GetSecret: secret %s not found", name)
			return true, nil, fmt.Errorf("cannot find secret %s", name)
		}
	}

	return false, nil, nil
}

// injectReactError returns an error when the test requested given action to
// fail. nil is returned otherwise.
func (r *snapshotReactor) injectReactError(action core.Action) error {
	if len(r.errors) == 0 {
		// No more errors to inject, everything should succeed.
		return nil
	}

	for i, expected := range r.errors {
		glog.V(4).Infof("trying to match %q %q with %q %q", expected.verb, expected.resource, action.GetVerb(), action.GetResource())
		if action.Matches(expected.verb, expected.resource) {
			// That's the action we're waiting for, remove it from injectedErrors
			r.errors = append(r.errors[:i], r.errors[i+1:]...)
			glog.V(4).Infof("reactor found matching error at index %d: %q %q, returning %v", i, expected.verb, expected.resource, expected.error)
			return expected.error
		}
	}
	return nil
}

// checkContents compares all expectedContents with set of contents at the end of
// the test and reports differences.
func (r *snapshotReactor) checkContents(expectedContents []*crdv1.VolumeSnapshotContent) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	expectedMap := make(map[string]*crdv1.VolumeSnapshotContent)
	gotMap := make(map[string]*crdv1.VolumeSnapshotContent)
	// Clear any ResourceVersion from both sets
	for _, v := range expectedContents {
		// Don't modify the existing object
		v := v.DeepCopy()
		v.ResourceVersion = ""
		if v.Spec.VolumeSnapshotRef != nil {
			v.Spec.VolumeSnapshotRef.ResourceVersion = ""
		}
		if v.Spec.PersistentVolumeRef != nil {
			v.Spec.PersistentVolumeRef.ResourceVersion = ""
		}
		if v.Spec.CSI != nil {
			v.Spec.CSI.CreationTime = nil
		}
		expectedMap[v.Name] = v
	}
	for _, v := range r.contents {
		// We must clone the content because of golang race check - it was
		// written by the controller without any locks on it.
		v := v.DeepCopy()
		v.ResourceVersion = ""
		if v.Spec.VolumeSnapshotRef != nil {
			v.Spec.VolumeSnapshotRef.ResourceVersion = ""
		}
		if v.Spec.PersistentVolumeRef != nil {
			v.Spec.PersistentVolumeRef.ResourceVersion = ""
		}
		if v.Spec.CSI != nil {
			v.Spec.CSI.CreationTime = nil
		}
		gotMap[v.Name] = v
	}
	if !reflect.DeepEqual(expectedMap, gotMap) {
		// Print ugly but useful diff of expected and received objects for
		// easier debugging.
		return fmt.Errorf("content check failed [A-expected, B-got]: %s", diff.ObjectDiff(expectedMap, gotMap))
	}
	return nil
}

// checkSnapshots compares all expectedSnapshots with set of snapshots at the end of the
// test and reports differences.
func (r *snapshotReactor) checkSnapshots(expectedSnapshots []*crdv1.VolumeSnapshot) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	expectedMap := make(map[string]*crdv1.VolumeSnapshot)
	gotMap := make(map[string]*crdv1.VolumeSnapshot)
	for _, c := range expectedSnapshots {
		// Don't modify the existing object
		c = c.DeepCopy()
		c.ResourceVersion = ""
		if c.Status.Error != nil {
			c.Status.Error.Time = metav1.Time{}
		}
		expectedMap[c.Name] = c
	}
	for _, c := range r.snapshots {
		// We must clone the snapshot because of golang race check - it was
		// written by the controller without any locks on it.
		c = c.DeepCopy()
		c.ResourceVersion = ""
		if c.Status.Error != nil {
			c.Status.Error.Time = metav1.Time{}
		}
		gotMap[c.Name] = c
	}
	if !reflect.DeepEqual(expectedMap, gotMap) {
		// Print ugly but useful diff of expected and received objects for
		// easier debugging.
		return fmt.Errorf("snapshot check failed [A-expected, B-got result]: %s", diff.ObjectDiff(expectedMap, gotMap))
	}
	return nil
}

// checkEvents compares all expectedEvents with events generated during the test
// and reports differences.
func checkEvents(t *testing.T, expectedEvents []string, ctrl *csiSnapshotController) error {
	var err error

	// Read recorded events - wait up to 1 minute to get all the expected ones
	// (just in case some goroutines are slower with writing)
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

	fakeRecorder := ctrl.eventRecorder.(*record.FakeRecorder)
	gotEvents := []string{}
	finished := false
	for len(gotEvents) < len(expectedEvents) && !finished {
		select {
		case event, ok := <-fakeRecorder.Events:
			if ok {
				glog.V(5).Infof("event recorder got event %s", event)
				gotEvents = append(gotEvents, event)
			} else {
				glog.V(5).Infof("event recorder finished")
				finished = true
			}
		case _, _ = <-timer.C:
			glog.V(5).Infof("event recorder timeout")
			finished = true
		}
	}

	// Evaluate the events
	for i, expected := range expectedEvents {
		if len(gotEvents) <= i {
			t.Errorf("Event %q not emitted", expected)
			err = fmt.Errorf("Events do not match")
			continue
		}
		received := gotEvents[i]
		if !strings.HasPrefix(received, expected) {
			t.Errorf("Unexpected event received, expected %q, got %q", expected, received)
			err = fmt.Errorf("Events do not match")
		}
	}
	for i := len(expectedEvents); i < len(gotEvents); i++ {
		t.Errorf("Unexpected event received: %q", gotEvents[i])
		err = fmt.Errorf("Events do not match")
	}
	return err
}

// popChange returns one recorded updated object, either *crdv1.VolumeSnapshotContent
// or *crdv1.VolumeSnapshot. Returns nil when there are no changes.
func (r *snapshotReactor) popChange() interface{} {
	r.lock.Lock()
	defer r.lock.Unlock()

	if len(r.changedObjects) == 0 {
		return nil
	}

	// For debugging purposes, print the queue
	for _, obj := range r.changedObjects {
		switch obj.(type) {
		case *crdv1.VolumeSnapshotContent:
			vol, _ := obj.(*crdv1.VolumeSnapshotContent)
			glog.V(4).Infof("reactor queue: %s", vol.Name)
		case *crdv1.VolumeSnapshot:
			snapshot, _ := obj.(*crdv1.VolumeSnapshot)
			glog.V(4).Infof("reactor queue: %s", snapshot.Name)
		}
	}

	// Pop the first item from the queue and return it
	obj := r.changedObjects[0]
	r.changedObjects = r.changedObjects[1:]
	return obj
}

// syncAll simulates the controller periodic sync of contents and snapshot. It
// simply adds all these objects to the internal queue of updates. This method
// should be used when the test manually calls syncSnapshot/syncContent. Test that
// use real controller loop (ctrl.Run()) will get periodic sync automatically.
func (r *snapshotReactor) syncAll() {
	r.lock.Lock()
	defer r.lock.Unlock()

	for _, c := range r.snapshots {
		r.changedObjects = append(r.changedObjects, c)
	}
	for _, v := range r.contents {
		r.changedObjects = append(r.changedObjects, v)
	}
	r.changedSinceLastSync = 0
}

func (r *snapshotReactor) getChangeCount() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.changedSinceLastSync
}

// waitForIdle waits until all tests, controllers and other goroutines do their
// job and no new actions are registered for 10 milliseconds.
func (r *snapshotReactor) waitForIdle() {
	r.ctrl.runningOperations.WaitForCompletion()
	// Check every 10ms if the controller does something and stop if it's
	// idle.
	oldChanges := -1
	for {
		time.Sleep(10 * time.Millisecond)
		changes := r.getChangeCount()
		if changes == oldChanges {
			// No changes for last 10ms -> controller must be idle.
			break
		}
		oldChanges = changes
	}
}

// waitTest waits until all tests, controllers and other goroutines do their
// job and list of current contents/snapshots is equal to list of expected
// contents/snapshots (with ~10 second timeout).
func (r *snapshotReactor) waitTest(test controllerTest) error {
	// start with 10 ms, multiply by 2 each step, 10 steps = 10.23 seconds
	backoff := wait.Backoff{
		Duration: 10 * time.Millisecond,
		Jitter:   0,
		Factor:   2,
		Steps:    10,
	}
	err := wait.ExponentialBackoff(backoff, func() (done bool, err error) {
		// Finish all operations that are in progress
		r.ctrl.runningOperations.WaitForCompletion()

		// Return 'true' if the reactor reached the expected state
		err1 := r.checkSnapshots(test.expectedSnapshots)
		err2 := r.checkContents(test.expectedContents)
		if err1 == nil && err2 == nil {
			return true, nil
		}
		return false, nil
	})
	return err
}

// deleteContentEvent simulates that a content has been deleted in etcd and
// the controller receives 'content deleted' event.
func (r *snapshotReactor) deleteContentEvent(content *crdv1.VolumeSnapshotContent) {
	r.lock.Lock()
	defer r.lock.Unlock()

	// Remove the content from list of resulting contents.
	delete(r.contents, content.Name)

	// Generate deletion event. Cloned content is needed to prevent races (and we
	// would get a clone from etcd too).
	if r.fakeContentWatch != nil {
		r.fakeContentWatch.Delete(content.DeepCopy())
	}
}

// deleteSnapshotEvent simulates that a snapshot has been deleted in etcd and the
// controller receives 'snapshot deleted' event.
func (r *snapshotReactor) deleteSnapshotEvent(snapshot *crdv1.VolumeSnapshot) {
	r.lock.Lock()
	defer r.lock.Unlock()

	// Remove the snapshot from list of resulting snapshots.
	delete(r.snapshots, snapshot.Name)

	// Generate deletion event. Cloned content is needed to prevent races (and we
	// would get a clone from etcd too).
	if r.fakeSnapshotWatch != nil {
		r.fakeSnapshotWatch.Delete(snapshot.DeepCopy())
	}
}

// addContentEvent simulates that a content has been added in etcd and the
// controller receives 'content added' event.
func (r *snapshotReactor) addContentEvent(content *crdv1.VolumeSnapshotContent) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.contents[content.Name] = content
	// Generate event. No cloning is needed, this snapshot is not stored in the
	// controller cache yet.
	if r.fakeContentWatch != nil {
		r.fakeContentWatch.Add(content)
	}
}

// modifyContentEvent simulates that a content has been modified in etcd and the
// controller receives 'content modified' event.
func (r *snapshotReactor) modifyContentEvent(content *crdv1.VolumeSnapshotContent) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.contents[content.Name] = content
	// Generate deletion event. Cloned content is needed to prevent races (and we
	// would get a clone from etcd too).
	if r.fakeContentWatch != nil {
		r.fakeContentWatch.Modify(content.DeepCopy())
	}
}

// addSnapshotEvent simulates that a snapshot has been deleted in etcd and the
// controller receives 'snapshot added' event.
func (r *snapshotReactor) addSnapshotEvent(snapshot *crdv1.VolumeSnapshot) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.snapshots[snapshot.Name] = snapshot
	// Generate event. No cloning is needed, this snapshot is not stored in the
	// controller cache yet.
	if r.fakeSnapshotWatch != nil {
		r.fakeSnapshotWatch.Add(snapshot)
	}
}

func newSnapshotReactor(kubeClient *kubefake.Clientset, client *fake.Clientset, ctrl *csiSnapshotController, fakeVolumeWatch, fakeClaimWatch *watch.FakeWatcher, errors []reactorError) *snapshotReactor {
	reactor := &snapshotReactor{
		secrets:           make(map[string]*v1.Secret),
		storageClasses:    make(map[string]*storagev1.StorageClass),
		volumes:           make(map[string]*v1.PersistentVolume),
		claims:            make(map[string]*v1.PersistentVolumeClaim),
		contents:          make(map[string]*crdv1.VolumeSnapshotContent),
		snapshots:         make(map[string]*crdv1.VolumeSnapshot),
		ctrl:              ctrl,
		fakeContentWatch:  fakeVolumeWatch,
		fakeSnapshotWatch: fakeClaimWatch,
		errors:            errors,
	}

	client.AddReactor("create", "volumesnapshotcontents", reactor.React)
	client.AddReactor("update", "volumesnapshotcontents", reactor.React)
	client.AddReactor("update", "volumesnapshots", reactor.React)
	client.AddReactor("get", "volumesnapshotcontents", reactor.React)
	client.AddReactor("get", "volumesnapshots", reactor.React)
	client.AddReactor("delete", "volumesnapshotcontents", reactor.React)
	client.AddReactor("delete", "volumesnapshots", reactor.React)
	kubeClient.AddReactor("get", "persistentvolumeclaims", reactor.React)
	kubeClient.AddReactor("get", "persistentvolumes", reactor.React)
	kubeClient.AddReactor("get", "storageclasses", reactor.React)
	kubeClient.AddReactor("get", "secrets", reactor.React)

	return reactor
}

func alwaysReady() bool { return true }

func newTestController(kubeClient kubernetes.Interface, clientset clientset.Interface,
	informerFactory informers.SharedInformerFactory, t *testing.T, test controllerTest) (*csiSnapshotController, error) {
	if informerFactory == nil {
		informerFactory = informers.NewSharedInformerFactory(clientset, NoResyncPeriodFunc())
	}

	// Construct controller
	csiConnection := &fakeCSIConnection{
		t:           t,
		listCalls:   test.expectedListCalls,
		createCalls: test.expectedCreateCalls,
		deleteCalls: test.expectedDeleteCalls,
	}

	ctrl := NewCSISnapshotController(
		clientset,
		kubeClient,
		mockDriverName,
		informerFactory.Volumesnapshot().V1alpha1().VolumeSnapshots(),
		informerFactory.Volumesnapshot().V1alpha1().VolumeSnapshotContents(),
		informerFactory.Volumesnapshot().V1alpha1().VolumeSnapshotClasses(),
		3,
		5*time.Millisecond,
		csiConnection,
		5*time.Millisecond,
		60*time.Second,
		"snapshot",
		-1,
	)

	ctrl.eventRecorder = record.NewFakeRecorder(1000)

	ctrl.contentListerSynced = alwaysReady
	ctrl.snapshotListerSynced = alwaysReady
	ctrl.classListerSynced = alwaysReady

	return ctrl, nil
}

// newContent returns a new content with given attributes
func newContent(name, className, snapshotHandle, volumeUID, volumeName, boundToSnapshotUID, boundToSnapshotName string, size *resource.Quantity, creationTime *int64) *crdv1.VolumeSnapshotContent {
	content := crdv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: "1",
		},
		Spec: crdv1.VolumeSnapshotContentSpec{
			VolumeSnapshotSource: crdv1.VolumeSnapshotSource{
				CSI: &crdv1.CSIVolumeSnapshotSource{
					RestoreSize:    size,
					Driver:         mockDriverName,
					SnapshotHandle: snapshotHandle,
					CreationTime:   creationTime,
				},
			},
			VolumeSnapshotClassName: &className,
			PersistentVolumeRef: &v1.ObjectReference{
				Kind:       "PersistentVolume",
				APIVersion: "v1",
				UID:        types.UID(volumeUID),
				Name:       volumeName,
			},
		},
	}
	if boundToSnapshotName != "" {
		content.Spec.VolumeSnapshotRef = &v1.ObjectReference{
			Kind:       "VolumeSnapshot",
			APIVersion: "snapshot.storage.k8s.io/v1alpha1",
			UID:        types.UID(boundToSnapshotUID),
			Namespace:  testNamespace,
			Name:       boundToSnapshotName,
		}
	}

	return &content
}

func newContentArray(name, className, snapshotHandle, volumeUID, volumeName, boundToSnapshotUID, boundToSnapshotName string, size *resource.Quantity, creationTime *int64) []*crdv1.VolumeSnapshotContent {
	return []*crdv1.VolumeSnapshotContent{
		newContent(name, className, snapshotHandle, volumeUID, volumeName, boundToSnapshotUID, boundToSnapshotName, size, creationTime),
	}
}

func newSnapshot(name, className, boundToContent, snapshotUID, claimName string, ready bool, err *storagev1beta1.VolumeError, creationTime *metav1.Time, size *resource.Quantity) *crdv1.VolumeSnapshot {
	snapshot := crdv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       testNamespace,
			UID:             types.UID(snapshotUID),
			ResourceVersion: "1",
			SelfLink:        "/apis/snapshot.storage.k8s.io/v1alpha1/namespaces/" + testNamespace + "/volumesnapshots/" + name,
		},
		Spec: crdv1.VolumeSnapshotSpec{
			Source: &crdv1.TypedLocalObjectReference{
				Name: claimName,
				Kind: "PersistentVolumeClaim",
			},
			VolumeSnapshotClassName: &className,
			SnapshotContentName:     boundToContent,
		},
		Status: crdv1.VolumeSnapshotStatus{
			CreationTime: creationTime,
			Ready:        ready,
			Error:        err,
			RestoreSize:  size,
		},
	}

	return &snapshot
}

func newSnapshotArray(name, className, boundToContent, snapshotUID, claimName string, ready bool, err *storagev1beta1.VolumeError, creationTime *metav1.Time, size *resource.Quantity) []*crdv1.VolumeSnapshot {
	return []*crdv1.VolumeSnapshot{
		newSnapshot(name, className, boundToContent, snapshotUID, claimName, ready, err, creationTime, size),
	}
}

// newClaim returns a new claim with given attributes
func newClaim(name, claimUID, capacity, boundToVolume string, phase v1.PersistentVolumeClaimPhase, class *string) *v1.PersistentVolumeClaim {
	claim := v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       testNamespace,
			UID:             types.UID(claimUID),
			ResourceVersion: "1",
			SelfLink:        "/api/v1/namespaces/" + testNamespace + "/persistentvolumeclaims/" + name,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce, v1.ReadOnlyMany},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceName(v1.ResourceStorage): resource.MustParse(capacity),
				},
			},
			VolumeName:       boundToVolume,
			StorageClassName: class,
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}

	// Bound claims must have proper Status.
	if phase == v1.ClaimBound {
		claim.Status.AccessModes = claim.Spec.AccessModes
		// For most of the tests it's enough to copy claim's requested capacity,
		// individual tests can adjust it using withExpectedCapacity()
		claim.Status.Capacity = claim.Spec.Resources.Requests
	}

	return &claim
}

// newClaimArray returns array with a single claim that would be returned by
// newClaim() with the same parameters.
func newClaimArray(name, claimUID, capacity, boundToVolume string, phase v1.PersistentVolumeClaimPhase, class *string) []*v1.PersistentVolumeClaim {
	return []*v1.PersistentVolumeClaim{
		newClaim(name, claimUID, capacity, boundToVolume, phase, class),
	}
}

// newVolume returns a new volume with given attributes
func newVolume(name, volumeUID, volumeHandle, capacity, boundToClaimUID, boundToClaimName string, phase v1.PersistentVolumePhase, reclaimPolicy v1.PersistentVolumeReclaimPolicy, class string, annotations ...string) *v1.PersistentVolume {
	volume := v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: "1",
			UID:             types.UID(volumeUID),
			SelfLink:        "/api/v1/persistentvolumes/" + name,
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse(capacity),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       mockDriverName,
					VolumeHandle: volumeHandle,
				},
			},
			AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce, v1.ReadOnlyMany},
			PersistentVolumeReclaimPolicy: reclaimPolicy,
			StorageClassName:              class,
		},
		Status: v1.PersistentVolumeStatus{
			Phase: phase,
		},
	}

	if boundToClaimName != "" {
		volume.Spec.ClaimRef = &v1.ObjectReference{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
			UID:        types.UID(boundToClaimUID),
			Namespace:  testNamespace,
			Name:       boundToClaimName,
		}
	}

	return &volume
}

// newVolumeArray returns array with a single volume that would be returned by
// newVolume() with the same parameters.
func newVolumeArray(name, volumeUID, volumeHandle, capacity, boundToClaimUID, boundToClaimName string, phase v1.PersistentVolumePhase, reclaimPolicy v1.PersistentVolumeReclaimPolicy, class string) []*v1.PersistentVolume {
	return []*v1.PersistentVolume{
		newVolume(name, volumeUID, volumeHandle, capacity, boundToClaimUID, boundToClaimName, phase, reclaimPolicy, class),
	}
}

func newVolumeError(message string) *storagev1beta1.VolumeError {
	return &storagev1beta1.VolumeError{
		Time:    metav1.Time{},
		Message: message,
	}
}

func testSyncSnapshot(ctrl *csiSnapshotController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.syncSnapshot(test.initialSnapshots[0])
}

func testSyncSnapshotError(ctrl *csiSnapshotController, reactor *snapshotReactor, test controllerTest) error {
	err := ctrl.syncSnapshot(test.initialSnapshots[0])

	if err != nil {
		return nil
	}
	return fmt.Errorf("syncSnapshot succeeded when failure was expected")
}

func testSyncContent(ctrl *csiSnapshotController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.syncContent(test.initialContents[0])
}

var (
	classEmpty         string = ""
	classGold          string = "gold"
	classSilver        string = "silver"
	classNonExisting   string = "non-existing"
	defaultClass       string = "default-class"
	emptySecretClass   string = "empty-secret-class"
	invalidSecretClass string = "invalid-secret-class"
	validSecretClass   string = "valid-secret-class"
	sameDriver         string = "sameDriver"
	diffDriver         string = "diffDriver"
)

// wrapTestWithInjectedOperation returns a testCall that:
// - starts the controller and lets it run original testCall until
//   scheduleOperation() call. It blocks the controller there and calls the
//   injected function to simulate that something is happening when the
//   controller waits for the operation lock. Controller is then resumed and we
//   check how it behaves.
func wrapTestWithInjectedOperation(toWrap testCall, injectBeforeOperation func(ctrl *csiSnapshotController, reactor *snapshotReactor)) testCall {

	return func(ctrl *csiSnapshotController, reactor *snapshotReactor, test controllerTest) error {
		// Inject a hook before async operation starts
		glog.V(4).Infof("reactor:injecting call")
		injectBeforeOperation(ctrl, reactor)

		// Run the tested function (typically syncSnapshot/syncContent) in a
		// separate goroutine.
		var testError error
		var testFinished int32

		go func() {
			testError = toWrap(ctrl, reactor, test)
			// Let the "main" test function know that syncContent has finished.
			atomic.StoreInt32(&testFinished, 1)
		}()

		// Wait for the controller to finish the test function.
		for atomic.LoadInt32(&testFinished) == 0 {
			time.Sleep(time.Millisecond * 10)
		}

		return testError
	}
}

func evaluateTestResults(ctrl *csiSnapshotController, reactor *snapshotReactor, test controllerTest, t *testing.T) {
	// Evaluate results
	if err := reactor.checkSnapshots(test.expectedSnapshots); err != nil {
		t.Errorf("Test %q: %v", test.name, err)

	}
	if err := reactor.checkContents(test.expectedContents); err != nil {
		t.Errorf("Test %q: %v", test.name, err)
	}

	if err := checkEvents(t, test.expectedEvents, ctrl); err != nil {
		t.Errorf("Test %q: %v", test.name, err)
	}
}

// Test single call to syncSnapshot and syncContent methods.
// For all tests:
// 1. Fill in the controller with initial data
// 2. Call the tested function (syncSnapshot/syncContent) via
//    controllerTest.testCall *once*.
// 3. Compare resulting contents and snapshots with expected contents and snapshots.
func runSyncTests(t *testing.T, tests []controllerTest, snapshotClasses []*crdv1.VolumeSnapshotClass) {
	for _, test := range tests {
		glog.V(4).Infof("starting test %q", test.name)

		// Initialize the controller
		kubeClient := &kubefake.Clientset{}
		client := &fake.Clientset{}

		ctrl, err := newTestController(kubeClient, client, nil, t, test)
		if err != nil {
			t.Fatalf("Test %q construct persistent content failed: %v", test.name, err)
		}

		reactor := newSnapshotReactor(kubeClient, client, ctrl, nil, nil, test.errors)
		for _, snapshot := range test.initialSnapshots {
			ctrl.snapshotStore.Add(snapshot)
			reactor.snapshots[snapshot.Name] = snapshot
		}
		for _, content := range test.initialContents {
			ctrl.contentStore.Add(content)
			reactor.contents[content.Name] = content
		}
		for _, claim := range test.initialClaims {
			reactor.claims[claim.Name] = claim
		}
		for _, volume := range test.initialVolumes {
			reactor.volumes[volume.Name] = volume
		}
		for _, storageClass := range test.initialStorageClasses {
			reactor.storageClasses[storageClass.Name] = storageClass
		}
		for _, secret := range test.initialSecrets {
			reactor.secrets[secret.Name] = secret
		}

		// Inject classes into controller via a custom lister.
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		for _, class := range snapshotClasses {
			indexer.Add(class)
		}
		ctrl.classLister = storagelisters.NewVolumeSnapshotClassLister(indexer)

		// Run the tested functions
		err = test.test(ctrl, reactor, test)
		if err != nil {
			t.Errorf("Test %q failed: %v", test.name, err)
		}

		// Wait for the target state
		err = reactor.waitTest(test)
		if err != nil {
			t.Errorf("Test %q failed: %v", test.name, err)
		}

		evaluateTestResults(ctrl, reactor, test, t)
	}
}

func getSize(size int64) *resource.Quantity {
	return resource.NewQuantity(size, resource.BinarySI)
}

func emptySecret() *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "emptysecret",
			Namespace: "default",
		},
	}
}

func secret() *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"foo": []byte("bar"),
		},
	}
}

type listCall struct {
	snapshotID string
	// information to return
	status     *csi.SnapshotStatus
	createTime int64
	size       int64
	err        error
}

type deleteCall struct {
	snapshotID string
	secrets    map[string]string
	err        error
}

type createCall struct {
	// expected request parameter
	snapshotName string
	volume       *v1.PersistentVolume
	parameters   map[string]string
	secrets      map[string]string
	// information to return
	driverName string
	snapshotId string
	timestamp  int64
	size       int64
	status     *csi.SnapshotStatus
	err        error
}

// Fake CSIConnection implementation that check that Attach/Detach is called
// with the right parameters and it returns proper error code and metadata.
type fakeCSIConnection struct {
	createCalls       []createCall
	createCallCounter int
	deleteCalls       []deleteCall
	deleteCallCounter int
	listCalls         []listCall
	listCallCounter   int
	t                 *testing.T
}

func (f *fakeCSIConnection) GetDriverName(ctx context.Context) (string, error) {
	return mockDriverName, nil
}

func (f *fakeCSIConnection) SupportsControllerCreateSnapshot(ctx context.Context) (bool, error) {
	return false, fmt.Errorf("Not implemented")
}

func (f *fakeCSIConnection) SupportsControllerListSnapshots(ctx context.Context) (bool, error) {
	return false, fmt.Errorf("Not implemented")
}

func (f *fakeCSIConnection) CreateSnapshot(ctx context.Context, snapshotName string, volume *v1.PersistentVolume, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, int64, int64, *csi.SnapshotStatus, error) {
	if f.createCallCounter >= len(f.createCalls) {
		f.t.Errorf("Unexpected CSI Create Snapshot call: snapshotName=%s, volume=%v, index: %d, calls: %+v", snapshotName, volume.Name, f.createCallCounter, f.createCalls)
		return "", "", 0, 0, nil, fmt.Errorf("unexpected call")
	}
	call := f.createCalls[f.createCallCounter]
	f.createCallCounter++

	var err error
	if call.snapshotName != snapshotName {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotName=%s, volume=%s, expected snapshotName: %s", snapshotName, volume.Name, call.snapshotName)
		err = fmt.Errorf("unexpected create snapshot call")
	}

	if !reflect.DeepEqual(call.volume, volume) {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotName=%s, volume=%s, diff %s", snapshotName, volume.Name, diff.ObjectDiff(call.volume, volume))
		err = fmt.Errorf("unexpected create snapshot call")
	}

	if !reflect.DeepEqual(call.parameters, parameters) {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotName=%s, volume=%s, expected parameters %+v, got %+v", snapshotName, volume.Name, call.parameters, parameters)
		err = fmt.Errorf("unexpected create snapshot call")
	}

	if !reflect.DeepEqual(call.secrets, snapshotterCredentials) {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotName=%s, volume=%s, expected secrets %+v, got %+v", snapshotName, volume.Name, call.secrets, snapshotterCredentials)
		err = fmt.Errorf("unexpected create snapshot call")
	}

	if err != nil {
		return "", "", 0, 0, nil, fmt.Errorf("unexpected call")
	}

	return call.driverName, call.snapshotId, call.timestamp, call.size, call.status, call.err
}

func (f *fakeCSIConnection) DeleteSnapshot(ctx context.Context, snapshotID string, snapshotterCredentials map[string]string) error {
	if f.deleteCallCounter >= len(f.deleteCalls) {
		f.t.Errorf("Unexpected CSI Delete Snapshot call: snapshotID=%s, index: %d, calls: %+v", snapshotID, f.createCallCounter, f.createCalls)
		return fmt.Errorf("unexpected call")
	}
	call := f.deleteCalls[f.deleteCallCounter]
	f.deleteCallCounter++

	var err error
	if call.snapshotID != snapshotID {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotID=%s, expected snapshotID: %s", snapshotID, call.snapshotID)
		err = fmt.Errorf("unexpected Delete snapshot call")
	}

	if !reflect.DeepEqual(call.secrets, snapshotterCredentials) {
		f.t.Errorf("Wrong CSI Delete Snapshot call: snapshotID=%s, expected secrets %+v, got %+v", snapshotID, call.secrets, snapshotterCredentials)
		err = fmt.Errorf("unexpected Delete Snapshot call")
	}

	if err != nil {
		return fmt.Errorf("unexpected call")
	}

	return call.err
}

func (f *fakeCSIConnection) GetSnapshotStatus(ctx context.Context, snapshotID string) (*csi.SnapshotStatus, int64, int64, error) {
	if f.listCallCounter >= len(f.listCalls) {
		f.t.Errorf("Unexpected CSI list Snapshot call: snapshotID=%s, index: %d, calls: %+v", snapshotID, f.createCallCounter, f.createCalls)
		return nil, 0, 0, fmt.Errorf("unexpected call")
	}
	call := f.listCalls[f.listCallCounter]
	f.listCallCounter++

	var err error
	if call.snapshotID != snapshotID {
		f.t.Errorf("Wrong CSI List Snapshot call: snapshotID=%s, expected snapshotID: %s", snapshotID, call.snapshotID)
		err = fmt.Errorf("unexpected List snapshot call")
	}

	if err != nil {
		return nil, 0, 0, fmt.Errorf("unexpected call")
	}

	return call.status, call.createTime, call.size, call.err
}

func (f *fakeCSIConnection) Close() error {
	return fmt.Errorf("Not implemented")
}

func (f *fakeCSIConnection) Probe(ctx context.Context) error {
	return nil
}
