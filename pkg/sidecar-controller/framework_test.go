/*
Copyright 2019 The Kubernetes Authors.
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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	snapshotscheme "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/scheme"
	informers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	klog "k8s.io/klog/v2"
)

// This is a unit test framework for snapshot sidecar controller.
// It fills the controller with test contents and can simulate these
// scenarios:
// 1) Call syncContent once.
// 2) Call syncContent several times (both simulating "content
//    modified" events and periodic sync), until the controller settles down and
//    does not modify anything.
// 3) Simulate almost real API server/etcd and call add/update/delete
//    content.
// In all these scenarios, when the test finishes, the framework can compare
// resulting contents with list of expected contents and report
// differences.

// controllerTest contains a single controller test input.
// Each test has initial set of contents that are filled into the
// controller before the test starts. The test then contains a reference to
// function to call as the actual test. Available functions are:
//   - testSyncContent - calls syncContent on the first content in initialContents.
//   - any custom function for specialized tests.
//
// The test then contains list of contents that are expected at the end
// of the test and list of generated events.
type controllerTest struct {
	// Name of the test, for logging
	name string
	// Initial content of controller content cache.
	initialContents []*crdv1.VolumeSnapshotContent
	// Expected content of controller content cache at the end of the test.
	expectedContents []*crdv1.VolumeSnapshotContent
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
	test          testCall
	expectSuccess bool
	expectRequeue bool
}

type testCall func(ctrl *csiSnapshotSideCarController, reactor *snapshotReactor, test controllerTest) (requeue bool, err error)

const (
	testNamespace  = "default"
	mockDriverName = "csi-mock-plugin"
)

var (
	errVersionConflict = errors.New("VersionError")
	nocontents         []*crdv1.VolumeSnapshotContent
	noevents           = []string{}
	noerrors           = []reactorError{}
)

// snapshotReactor is a core.Reactor that simulates etcd and API server. It
// stores:
//   - Latest version of snapshots contents saved by the controller.
//   - Queue of all saves (to simulate "content updated" events). This queue
//     contains all intermediate state of an object. This queue will then contain both
//     updates as separate entries.
//   - Number of changes since the last call to snapshotReactor.syncAll().
//   - Optionally, content watcher which should be the same ones
//     used by the controller. Any time an event function like deleteContentEvent
//     is called to simulate an event, the reactor's stores are updated and the
//     controller is sent the event via the fake watcher.
//   - Optionally, list of error that should be returned by reactor, simulating
//     etcd / API server failures. These errors are evaluated in order and every
//     error is returned only once. I.e. when the reactor finds matching
//     reactorError, it return appropriate error and removes the reactorError from
//     the list.
type snapshotReactor struct {
	secrets              map[string]*v1.Secret
	snapshotClasses      map[string]*crdv1.VolumeSnapshotClass
	contents             map[string]*crdv1.VolumeSnapshotContent
	changedObjects       []interface{}
	changedSinceLastSync int
	ctrl                 *csiSnapshotSideCarController
	fakeContentWatch     *watch.FakeWatcher
	lock                 sync.Mutex
	errors               []reactorError
}

// reactorError is an error that is returned by test reactor (=simulated
// etcd+/API server) when an action performed by the reactor matches given verb
// ("get", "update", "create", "delete" or "*"") on given resource
// ("volumesnapshotcontents" or "*").
type reactorError struct {
	verb     string
	resource string
	error    error
}

func withContentFinalizer(content *crdv1.VolumeSnapshotContent) *crdv1.VolumeSnapshotContent {
	content.ObjectMeta.Finalizers = append(content.ObjectMeta.Finalizers, utils.VolumeSnapshotContentFinalizer)
	return content
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

	klog.V(4).Infof("reactor got operation %q on %q", action.GetVerb(), action.GetResource())

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
		klog.V(5).Infof("created content %s", content.Name)
		return true, content, nil

	case action.Matches("update", "volumesnapshotcontents"):
		obj := action.(core.UpdateAction).GetObject()
		content := obj.(*crdv1.VolumeSnapshotContent)

		// Check and bump object version
		storedContent, found := r.contents[content.Name]
		if found {
			storedVer, _ := strconv.Atoi(storedContent.ResourceVersion)
			requestedVer, _ := strconv.Atoi(content.ResourceVersion)
			if storedVer != requestedVer {
				return true, obj, errVersionConflict
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
		klog.V(4).Infof("saved updated content %s", content.Name)
		return true, content, nil

	case action.Matches("patch", "volumesnapshotcontents"):
		content := &crdv1.VolumeSnapshotContent{}
		action := action.(core.PatchAction)

		// Check and bump object version
		storedSnapshotContent, found := r.contents[action.GetName()]
		if found {
			// Apply patch
			storedSnapshotBytes, err := json.Marshal(storedSnapshotContent)
			if err != nil {
				return true, nil, err
			}
			contentPatch, err := jsonpatch.DecodePatch(action.GetPatch())
			if err != nil {
				return true, nil, err
			}

			modified, err := contentPatch.Apply(storedSnapshotBytes)
			if err != nil {
				return true, nil, err
			}

			err = json.Unmarshal(modified, content)
			if err != nil {
				return true, nil, err
			}

			storedVer, _ := strconv.Atoi(content.ResourceVersion)
			content.ResourceVersion = strconv.Itoa(storedVer + 1)
		} else {
			return true, nil, fmt.Errorf("cannot update snapshot content %s: snapshot content not found", action.GetName())
		}

		// Store the updated object to appropriate places.
		r.contents[content.Name] = content
		r.changedObjects = append(r.changedObjects, content)
		r.changedSinceLastSync++
		klog.V(4).Infof("saved updated content %s", content.Name)
		return true, content, nil

	case action.Matches("get", "volumesnapshotcontents"):
		name := action.(core.GetAction).GetName()
		content, found := r.contents[name]
		if found {
			klog.V(4).Infof("GetVolume: found %s", content.Name)
			return true, content, nil
		}
		klog.V(4).Infof("GetVolume: content %s not found", name)
		return true, nil, fmt.Errorf("cannot find content %s", name)

	case action.Matches("delete", "volumesnapshotcontents"):
		name := action.(core.DeleteAction).GetName()
		klog.V(4).Infof("deleted content %s", name)
		_, found := r.contents[name]
		if found {
			delete(r.contents, name)
			r.changedSinceLastSync++
			return true, nil, nil
		}
		return true, nil, fmt.Errorf("cannot delete content %s: not found", name)

	case action.Matches("get", "secrets"):
		name := action.(core.GetAction).GetName()
		secret, found := r.secrets[name]
		if found {
			klog.V(4).Infof("GetSecret: found %s", secret.Name)
			return true, secret, nil
		}
		klog.V(4).Infof("GetSecret: secret %s not found", name)
		return true, nil, fmt.Errorf("cannot find secret %s", name)

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
		klog.V(4).Infof("trying to match %q %q with %q %q", expected.verb, expected.resource, action.GetVerb(), action.GetResource())
		if action.Matches(expected.verb, expected.resource) {
			// That's the action we're waiting for, remove it from injectedErrors
			r.errors = append(r.errors[:i], r.errors[i+1:]...)
			klog.V(4).Infof("reactor found matching error at index %d: %q %q, returning %v", i, expected.verb, expected.resource, expected.error)
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
		v.Spec.VolumeSnapshotRef.ResourceVersion = ""
		if v.Status != nil {
			v.Status.CreationTime = nil
		}
		if v.Status.Error != nil {
			v.Status.Error.Time = &metav1.Time{}
		}
		expectedMap[v.Name] = v
	}
	for _, v := range r.contents {
		// We must clone the content because of golang race check - it was
		// written by the controller without any locks on it.
		v := v.DeepCopy()
		v.ResourceVersion = ""
		v.Spec.VolumeSnapshotRef.ResourceVersion = ""
		if v.Status != nil {
			v.Status.CreationTime = nil
			if v.Status.Error != nil {
				v.Status.Error.Time = &metav1.Time{}
			}
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

// checkEvents compares all expectedEvents with events generated during the test
// and reports differences.
func checkEvents(t *testing.T, expectedEvents []string, ctrl *csiSnapshotSideCarController) error {
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
				klog.V(5).Infof("event recorder got event %s", event)
				gotEvents = append(gotEvents, event)
			} else {
				klog.V(5).Infof("event recorder finished")
				finished = true
			}
		case _, _ = <-timer.C:
			klog.V(5).Infof("event recorder timeout")
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
			klog.V(4).Infof("reactor queue: %s", vol.Name)
		}
	}

	// Pop the first item from the queue and return it
	obj := r.changedObjects[0]
	r.changedObjects = r.changedObjects[1:]
	return obj
}

// syncAll simulates the controller periodic sync of contents. It
// simply adds all these objects to the internal queue of updates. This method
// should be used when the test manually calls syncContent. Test that
// use real controller loop (ctrl.Run()) will get periodic sync automatically.
func (r *snapshotReactor) syncAll() {
	r.lock.Lock()
	defer r.lock.Unlock()

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
		// Return 'true' if the reactor reached the expected state
		err1 := r.checkContents(test.expectedContents)
		if err1 == nil {
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

func newSnapshotReactor(kubeClient *kubefake.Clientset, client *fake.Clientset, ctrl *csiSnapshotSideCarController, fakeVolumeWatch, fakeClaimWatch *watch.FakeWatcher, errors []reactorError) *snapshotReactor {
	reactor := &snapshotReactor{
		secrets:          make(map[string]*v1.Secret),
		snapshotClasses:  make(map[string]*crdv1.VolumeSnapshotClass),
		contents:         make(map[string]*crdv1.VolumeSnapshotContent),
		ctrl:             ctrl,
		fakeContentWatch: fakeVolumeWatch,
		errors:           errors,
	}

	client.AddReactor("create", "volumesnapshotcontents", reactor.React)
	client.AddReactor("update", "volumesnapshotcontents", reactor.React)
	client.AddReactor("patch", "volumesnapshotcontents", reactor.React)
	client.AddReactor("get", "volumesnapshotcontents", reactor.React)
	client.AddReactor("delete", "volumesnapshotcontents", reactor.React)
	kubeClient.AddReactor("get", "secrets", reactor.React)

	return reactor
}

func alwaysReady() bool { return true }

func newTestController(kubeClient kubernetes.Interface, clientset clientset.Interface,
	informerFactory informers.SharedInformerFactory, t *testing.T, test controllerTest) (*csiSnapshotSideCarController, error) {
	if informerFactory == nil {
		informerFactory = informers.NewSharedInformerFactory(clientset, utils.NoResyncPeriodFunc())
	}

	// Construct controller
	fakeSnapshot := &fakeSnapshotter{
		t:           t,
		listCalls:   test.expectedListCalls,
		createCalls: test.expectedCreateCalls,
		deleteCalls: test.expectedDeleteCalls,
	}

	ctrl := NewCSISnapshotSideCarController(
		clientset,
		kubeClient,
		mockDriverName,
		informerFactory.Snapshot().V1().VolumeSnapshotContents(),
		informerFactory.Snapshot().V1().VolumeSnapshotClasses(),
		fakeSnapshot,
		nil, // TODO: Replace with fake group snapshotter
		5*time.Millisecond,
		60*time.Second,
		"snapshot",
		-1,
		"groupsnapshot",
		-1,
		true,
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](1*time.Millisecond, 1*time.Minute),
		false,
		informerFactory.Groupsnapshot().V1beta2().VolumeGroupSnapshotContents(),
		informerFactory.Groupsnapshot().V1beta2().VolumeGroupSnapshotClasses(),
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](1*time.Millisecond, 1*time.Minute),
	)

	ctrl.eventRecorder = record.NewFakeRecorder(1000)

	ctrl.contentListerSynced = alwaysReady
	ctrl.classListerSynced = alwaysReady

	return ctrl, nil
}

func newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, creationTime, size *int64,
	withFinalizer bool, deletionTime *metav1.Time) *crdv1.VolumeSnapshotContent {
	var annotations map[string]string

	content := crdv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:              contentName,
			ResourceVersion:   "1",
			DeletionTimestamp: deletionTime,
			Annotations:       annotations,
		},
		Spec: crdv1.VolumeSnapshotContentSpec{
			Driver:         mockDriverName,
			DeletionPolicy: deletionPolicy,
		},
		Status: &crdv1.VolumeSnapshotContentStatus{
			CreationTime: creationTime,
			RestoreSize:  size,
		},
	}
	if deletionTime != nil {
		metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeSnapshotBeingDeleted, "yes")
	}

	if snapshotHandle != "" {
		content.Status.SnapshotHandle = &snapshotHandle
	}

	if snapshotClassName != "" {
		content.Spec.VolumeSnapshotClassName = &snapshotClassName
	}

	if volumeHandle != "" {
		content.Spec.Source = crdv1.VolumeSnapshotContentSource{
			VolumeHandle: &volumeHandle,
		}
	} else if desiredSnapshotHandle != "" {
		content.Spec.Source = crdv1.VolumeSnapshotContentSource{
			SnapshotHandle: &desiredSnapshotHandle,
		}
	}

	if boundToSnapshotName != "" {
		content.Spec.VolumeSnapshotRef = v1.ObjectReference{
			Kind:       "VolumeSnapshot",
			APIVersion: "snapshot.storage.k8s.io/v1",
			UID:        types.UID(boundToSnapshotUID),
			Namespace:  testNamespace,
			Name:       boundToSnapshotName,
		}
	}

	if withFinalizer {
		return withContentFinalizer(&content)
	}
	return &content
}

func newContentArray(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, size, creationTime *int64,
	withFinalizer bool) []*crdv1.VolumeSnapshotContent {
	return []*crdv1.VolumeSnapshotContent{
		newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, creationTime, size, withFinalizer, nil),
	}
}

func newContentArrayWithReadyToUse(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, creationTime, size *int64, readyToUse *bool,
	withFinalizer bool) []*crdv1.VolumeSnapshotContent {
	content := newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, creationTime, size, withFinalizer, nil)
	content.Status.ReadyToUse = readyToUse
	return []*crdv1.VolumeSnapshotContent{
		content,
	}
}

func newContentWithVolumeGroupSnapshotHandle(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, volumeGroupSnapshotHandle, snapshotClassName, desiredSnapshotHandle,
	volumeHandle string, deletionPolicy crdv1.DeletionPolicy, creationTime, size *int64, withFinalizer bool, deletionTime *metav1.Time) []*crdv1.VolumeSnapshotContent {
	content := newContentArrayWithDeletionTimestamp(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, creationTime, size, withFinalizer, deletionTime)
	for _, c := range content {
		c.Status.VolumeGroupSnapshotHandle = &volumeGroupSnapshotHandle
	}
	return content
}

func newContentArrayWithDeletionTimestamp(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, size, creationTime *int64,
	withFinalizer bool, deletionTime *metav1.Time) []*crdv1.VolumeSnapshotContent {
	return []*crdv1.VolumeSnapshotContent{
		newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, creationTime, size, withFinalizer, deletionTime),
	}
}

func withContentStatus(content []*crdv1.VolumeSnapshotContent, status *crdv1.VolumeSnapshotContentStatus) []*crdv1.VolumeSnapshotContent {
	for i := range content {
		content[i].Status = status
	}

	return content
}

func withContentAnnotations(content []*crdv1.VolumeSnapshotContent, annotations map[string]string) []*crdv1.VolumeSnapshotContent {
	for i := range content {
		content[i].ObjectMeta.Annotations = annotations
	}

	return content
}

func testSyncContent(ctrl *csiSnapshotSideCarController, reactor *snapshotReactor, test controllerTest) (bool, error) {
	return ctrl.syncContent(test.initialContents[0])
}

func testSyncContentError(ctrl *csiSnapshotSideCarController, reactor *snapshotReactor, test controllerTest) (bool, error) {
	requeue, err := ctrl.syncContent(test.initialContents[0])
	if err != nil {
		return requeue, nil
	}
	return requeue, fmt.Errorf("syncSnapshotContent succeeded when failure was expected")
}

var (
	classEmpty         string
	classGold          = "gold"
	classSilver        = "silver"
	classNonExisting   = "non-existing"
	defaultClass       = "default-class"
	emptySecretClass   = "empty-secret-class"
	invalidSecretClass = "invalid-secret-class"
	validSecretClass   = "valid-secret-class"
	sameDriver         = "sameDriver"
	diffDriver         = "diffDriver"
	noClaim            = ""
	noBoundUID         = ""
	noVolume           = ""
)

// wrapTestWithInjectedOperation returns a testCall that:
//   - starts the controller and lets it run original testCall until
//     scheduleOperation() call. It blocks the controller there and calls the
//     injected function to simulate that something is happening when the
//     controller waits for the operation lock. Controller is then resumed and we
//     check how it behaves.
func wrapTestWithInjectedOperation(toWrap testCall, injectBeforeOperation func(ctrl *csiSnapshotSideCarController, reactor *snapshotReactor)) testCall {
	return func(ctrl *csiSnapshotSideCarController, reactor *snapshotReactor, test controllerTest) (bool, error) {
		// Inject a hook before async operation starts
		klog.V(4).Infof("reactor:injecting call")
		injectBeforeOperation(ctrl, reactor)

		// Run the tested function (typically syncContent) in a
		// separate goroutine.
		var testError error
		var requeue bool
		var testFinished int32

		go func() {
			requeue, testError = toWrap(ctrl, reactor, test)
			// Let the "main" test function know that syncContent has finished.
			atomic.StoreInt32(&testFinished, 1)
		}()

		// Wait for the controller to finish the test function.
		for atomic.LoadInt32(&testFinished) == 0 {
			time.Sleep(time.Millisecond * 10)
		}

		return requeue, testError
	}
}

func evaluateTestResults(ctrl *csiSnapshotSideCarController, reactor *snapshotReactor, test controllerTest, t *testing.T) {
	// Evaluate results
	if test.expectedContents != nil {
		if err := reactor.checkContents(test.expectedContents); err != nil {
			t.Errorf("Test %q: %v", test.name, err)
		}
	}

	if err := checkEvents(t, test.expectedEvents, ctrl); err != nil {
		t.Errorf("Test %q: %v", test.name, err)
	}
}

// Test single call to syncContent methods.
// For all tests:
//  1. Fill in the controller with initial data
//  2. Call the tested function (syncContent) via
//     controllerTest.testCall *once*.
//  3. Compare resulting contents and snapshots with expected contents and snapshots.
func runSyncContentTests(t *testing.T, tests []controllerTest, snapshotClasses []*crdv1.VolumeSnapshotClass) {
	snapshotscheme.AddToScheme(scheme.Scheme)
	for _, test := range tests {
		klog.V(4).Infof("starting test %q", test.name)

		// Initialize the controller
		kubeClient := &kubefake.Clientset{}
		client := &fake.Clientset{}

		ctrl, err := newTestController(kubeClient, client, nil, t, test)
		if err != nil {
			t.Fatalf("Test %q construct persistent content failed: %v", test.name, err)
		}

		reactor := newSnapshotReactor(kubeClient, client, ctrl, nil, nil, test.errors)
		for _, content := range test.initialContents {
			if ctrl.isDriverMatch(test.initialContents[0]) {
				ctrl.contentStore.Add(content)
				reactor.contents[content.Name] = content
			}
		}

		for _, secret := range test.initialSecrets {
			reactor.secrets[secret.Name] = secret
		}

		// Inject classes into controller via a custom lister.
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		for _, class := range snapshotClasses {
			indexer.Add(class)
			reactor.snapshotClasses[class.Name] = class
		}
		ctrl.classLister = storagelisters.NewVolumeSnapshotClassLister(indexer)

		// Run the tested functions
		requeue, err := test.test(ctrl, reactor, test)
		if test.expectSuccess && err != nil {
			t.Errorf("Test %q failed: %v", test.name, err)
		}
		if !test.expectSuccess && err == nil {
			t.Errorf("Test %q failed: expected error, got nil", test.name)
		}
		if !test.expectSuccess && err == nil {
			t.Errorf("Test %q failed: expected error, got nil", test.name)
		}
		// requeue has meaning only when err == nil. A snapshot content is automatically requeued on error
		if err == nil && requeue != test.expectRequeue {
			t.Errorf("Test %q expected requeue %t, got %t", test.name, test.expectRequeue, requeue)
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

func secretAnnotations() map[string]string {
	return map[string]string{
		utils.AnnDeletionSecretRefName:      "secret",
		utils.AnnDeletionSecretRefNamespace: "default",
	}
}

func emptyNamespaceSecretAnnotations() map[string]string {
	return map[string]string{
		utils.AnnDeletionSecretRefName:      "name",
		utils.AnnDeletionSecretRefNamespace: "",
	}
}

// this refers to emptySecret(), which is missing data.
func emptyDataSecretAnnotations() map[string]string {
	return map[string]string{
		utils.AnnDeletionSecretRefName:      "emptysecret",
		utils.AnnDeletionSecretRefNamespace: "default",
	}
}

type listCall struct {
	snapshotID string
	secrets    map[string]string
	// information to return
	readyToUse      bool
	createTime      time.Time
	size            int64
	err             error
	groupSnapshotID string
}

type deleteCall struct {
	snapshotID string
	secrets    map[string]string
	err        error
}

type createCall struct {
	// expected request parameter
	snapshotName string
	volumeHandle string
	parameters   map[string]string
	secrets      map[string]string
	// information to return
	driverName   string
	snapshotId   string
	creationTime time.Time
	size         int64
	readyToUse   bool
	err          error
}

// Fake SnapShotter implementation that check that Attach/Detach is called
// with the right parameters and it returns proper error code and metadata.
type fakeSnapshotter struct {
	createCalls       []createCall
	createCallCounter int
	deleteCalls       []deleteCall
	deleteCallCounter int
	listCalls         []listCall
	listCallCounter   int
	t                 *testing.T
}

func (f *fakeSnapshotter) CreateSnapshot(ctx context.Context, snapshotName string, volumeHandle string, parameters map[string]string, snapshotterCredentials map[string]string) (string, string, time.Time, int64, bool, error) {
	if f.createCallCounter >= len(f.createCalls) {
		f.t.Errorf("Unexpected CSI Create Snapshot call: snapshotName=%s, volumeHandle=%v, index: %d, calls: %+v", snapshotName, volumeHandle, f.createCallCounter, f.createCalls)
		return "", "", time.Time{}, 0, false, fmt.Errorf("unexpected call")
	}
	call := f.createCalls[f.createCallCounter]
	f.createCallCounter++

	var err error
	if call.snapshotName != snapshotName {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotName=%s, volumeHandle=%s, expected snapshotName: %s", snapshotName, volumeHandle, call.snapshotName)
		err = fmt.Errorf("unexpected create snapshot call")
	}

	if call.volumeHandle != volumeHandle {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotName=%s, volumeHandle=%s, expected volumeHandle: %s", snapshotName, volumeHandle, call.volumeHandle)
		err = fmt.Errorf("unexpected create snapshot call")
	}

	if !reflect.DeepEqual(call.parameters, parameters) && !(len(call.parameters) == 0 && len(parameters) == 0) {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotName=%s, volumeHandle=%s, expected parameters %+v, got %+v", snapshotName, volumeHandle, call.parameters, parameters)
		err = fmt.Errorf("unexpected create snapshot call")
	}

	if !reflect.DeepEqual(call.secrets, snapshotterCredentials) && !(len(call.secrets) == 0 && len(snapshotterCredentials) == 0) {
		f.t.Errorf("Wrong CSI Create Snapshot call: snapshotName=%s, volumeHandle=%s, expected secrets %+v, got %+v", snapshotName, volumeHandle, call.secrets, snapshotterCredentials)
		err = fmt.Errorf("unexpected create snapshot call")
	}

	if err != nil {
		return "", "", time.Time{}, 0, false, fmt.Errorf("unexpected call")
	}
	return call.driverName, call.snapshotId, call.creationTime, call.size, call.readyToUse, call.err
}

func (f *fakeSnapshotter) DeleteSnapshot(ctx context.Context, snapshotID string, snapshotterCredentials map[string]string) error {
	if f.deleteCallCounter >= len(f.deleteCalls) {
		f.t.Errorf("Unexpected CSI Delete Snapshot call: snapshotID=%s, index: %d, calls: %+v", snapshotID, f.createCallCounter, f.createCalls)
		return fmt.Errorf("unexpected DeleteSnapshot call")
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

func (f *fakeSnapshotter) GetSnapshotStatus(ctx context.Context, snapshotID string, snapshotterListCredentials map[string]string) (bool, time.Time, int64, string, error) {
	if f.listCallCounter >= len(f.listCalls) {
		f.t.Errorf("Unexpected CSI list Snapshot call: snapshotID=%s, index: %d, calls: %+v", snapshotID, f.createCallCounter, f.createCalls)
		return false, time.Time{}, 0, "", fmt.Errorf("unexpected call")
	}
	call := f.listCalls[f.listCallCounter]
	f.listCallCounter++

	var err error
	if call.snapshotID != snapshotID {
		f.t.Errorf("Wrong CSI List Snapshot call: snapshotID=%s, expected snapshotID: %s", snapshotID, call.snapshotID)
		err = fmt.Errorf("unexpected List snapshot call")
	}

	if !reflect.DeepEqual(call.secrets, snapshotterListCredentials) {
		f.t.Errorf("Wrong CSI List Snapshot call: snapshotID=%s, expected secrets %+v, got %+v", snapshotID, call.secrets, snapshotterListCredentials)
		err = fmt.Errorf("unexpected List Snapshot call")
	}

	if err != nil {
		return false, time.Time{}, 0, "", fmt.Errorf("unexpected call")
	}

	return call.readyToUse, call.createTime, call.size, call.groupSnapshotID, call.err
}

func newSnapshotError(message string) *crdv1.VolumeSnapshotError {
	return &crdv1.VolumeSnapshotError{
		Time:    &metav1.Time{},
		Message: &message,
	}
}

func toStringPointer(str string) *string { return &str }
