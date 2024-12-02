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

package common_controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	sysruntime "runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	crdv1beta1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	snapshotscheme "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/scheme"
	informers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions"
	groupstoragelisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumegroupsnapshot/v1beta1"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/metrics"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	corelisters "k8s.io/client-go/listers/core/v1"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	klog "k8s.io/klog/v2"
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
//     and expects an error to be returned.
//   - testSyncContent - calls syncContent on the first content in initialContents.
//   - any custom function for specialized tests.
//
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
	// Initial content of controller content cache.
	initialGroupSnapshots []*crdv1beta1.VolumeGroupSnapshot
	// Expected content of controller content cache at the end of the test.
	expectedGroupSnapshots []*crdv1beta1.VolumeGroupSnapshot
	// Initial content of controller content cache.
	initialGroupContents []*crdv1beta1.VolumeGroupSnapshotContent
	// Expected content of controller content cache at the end of the test.
	expectedGroupContents []*crdv1beta1.VolumeGroupSnapshotContent
	// Initial content of controller volume cache.
	initialVolumes []*v1.PersistentVolume
	// Initial content of controller claim cache.
	initialClaims []*v1.PersistentVolumeClaim
	// Initial content of controller Secret cache.
	initialSecrets []*v1.Secret
	// Expected events - any event with prefix will pass, we don't check full
	// event message.
	expectedEvents []string
	// Errors to produce on matching action
	errors []reactorError
	// Function to call as the test.
	test          testCall
	expectSuccess bool
}

type testCall func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error

const (
	testNamespace  = "default"
	mockDriverName = "csi-mock-plugin"
)

var (
	errVersionConflict = errors.New("VersionError")
	nocontents         []*crdv1.VolumeSnapshotContent
	nogroupcontents    []*crdv1beta1.VolumeGroupSnapshotContent
	nosnapshots        []*crdv1.VolumeSnapshot
	nogroupsnapshots   []*crdv1beta1.VolumeGroupSnapshot
	noevents           = []string{}
	noerrors           = []reactorError{}
)

// snapshotReactor is a core.Reactor that simulates etcd and API server. It
// stores:
//   - Latest version of snapshots contents saved by the controller.
//   - Queue of all saves (to simulate "content/snapshot updated" events). This queue
//     contains all intermediate state of an object - e.g. a snapshot.VolumeName
//     is updated first and snapshot.Phase second. This queue will then contain both
//     updates as separate entries.
//   - Number of changes since the last call to snapshotReactor.syncAll().
//   - Optionally, content and snapshot fake watchers which should be the same ones
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
	volumes              map[string]*v1.PersistentVolume
	claims               map[string]*v1.PersistentVolumeClaim
	contents             map[string]*crdv1.VolumeSnapshotContent
	snapshots            map[string]*crdv1.VolumeSnapshot
	snapshotClasses      map[string]*crdv1.VolumeSnapshotClass
	groupContents        map[string]*crdv1beta1.VolumeGroupSnapshotContent
	groupSnapshots       map[string]*crdv1beta1.VolumeGroupSnapshot
	groupSnapshotClasses map[string]*crdv1beta1.VolumeGroupSnapshotClass
	changedObjects       []interface{}
	changedSinceLastSync int
	ctrl                 *csiSnapshotCommonController
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

// testError is an error returned from a test that marks a test as failed even
// though the test case itself expected a common error (such as API error)
type testError string

func (t testError) Error() string {
	return string(t)
}

var _ error = testError("foo")

func isTestError(err error) bool {
	_, ok := err.(testError)
	return ok
}

func withClaimLabels(pvcs []*v1.PersistentVolumeClaim, labels map[string]string) []*v1.PersistentVolumeClaim {
	for i := range pvcs {
		if pvcs[i].ObjectMeta.Labels == nil {
			pvcs[i].ObjectMeta.Labels = make(map[string]string)
		}
		for k, v := range labels {
			pvcs[i].ObjectMeta.Labels[k] = v
		}
	}
	return pvcs
}

func withVolumesCSIDriverName(pvs []*v1.PersistentVolume, driverName string) []*v1.PersistentVolume {
	for i := range pvs {
		if pvs[i].Spec.CSI == nil {
			pvs[i].Spec.CSI = &v1.CSIPersistentVolumeSource{}
		}
		pvs[i].Spec.CSI.Driver = driverName
	}
	return pvs
}

func withVolumesLocalPath(pvs []*v1.PersistentVolume, path string) []*v1.PersistentVolume {
	for i := range pvs {
		pvs[i].Spec.CSI = nil
		pvs[i].Spec.Local = &v1.LocalVolumeSource{
			Path: path,
		}
	}
	return pvs
}

func withSnapshotFinalizers(snapshots []*crdv1.VolumeSnapshot, finalizers ...string) []*crdv1.VolumeSnapshot {
	for i := range snapshots {
		for _, f := range finalizers {
			snapshots[i].ObjectMeta.Finalizers = append(snapshots[i].ObjectMeta.Finalizers, f)
		}
	}
	return snapshots
}

func withGroupSnapshotFinalizers(groupSnapshots []*crdv1beta1.VolumeGroupSnapshot, finalizers ...string) []*crdv1beta1.VolumeGroupSnapshot {
	for i := range groupSnapshots {
		for _, f := range finalizers {
			groupSnapshots[i].ObjectMeta.Finalizers = append(groupSnapshots[i].ObjectMeta.Finalizers, f)
		}
	}
	return groupSnapshots
}

func withPVCFinalizer(pvc *v1.PersistentVolumeClaim) *v1.PersistentVolumeClaim {
	pvc.ObjectMeta.Finalizers = append(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer)
	return pvc
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

	case action.Matches("create", "volumegroupsnapshotcontents"):
		obj := action.(core.UpdateAction).GetObject()
		content := obj.(*crdv1beta1.VolumeGroupSnapshotContent)

		// check the content does not exist
		_, found := r.contents[content.Name]
		if found {
			return true, nil, fmt.Errorf("cannot create content %s: content already exists", content.Name)
		}

		// Store the updated object to appropriate places.
		r.groupContents[content.Name] = content
		r.changedObjects = append(r.changedObjects, content)
		r.changedSinceLastSync++
		klog.V(5).Infof("created group content %s", content.Name)
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

	case action.Matches("update", "volumegroupsnapshotcontents"):
		obj := action.(core.UpdateAction).GetObject()
		content := obj.(*crdv1beta1.VolumeGroupSnapshotContent)

		// Check and bump object version
		storedVolume, found := r.contents[content.Name]
		if found {
			storedVer, _ := strconv.Atoi(storedVolume.ResourceVersion)
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
		r.groupContents[content.Name] = content
		r.changedObjects = append(r.changedObjects, content)
		r.changedSinceLastSync++
		klog.V(4).Infof("saved updated group content %s", content.Name)
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

	case action.Matches("patch", "volumegroupsnapshotcontents"):
		content := &crdv1beta1.VolumeGroupSnapshotContent{}
		action := action.(core.PatchAction)

		// Check and bump object version
		storedGroupSnapshotContent, found := r.groupContents[action.GetName()]
		if found {
			// Apply patch
			storedGroupSnapshotBytes, err := json.Marshal(storedGroupSnapshotContent)
			if err != nil {
				return true, nil, err
			}
			contentPatch, err := jsonpatch.DecodePatch(action.GetPatch())
			if err != nil {
				return true, nil, err
			}

			modified, err := contentPatch.Apply(storedGroupSnapshotBytes)
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
			return true, nil, fmt.Errorf("cannot update group snapshot content %s: group snapshot content not found", action.GetName())
		}

		// Store the updated object to appropriate places.
		r.groupContents[content.Name] = content
		r.changedObjects = append(r.changedObjects, content)
		r.changedSinceLastSync++
		klog.V(4).Infof("saved updated group content %s", content.Name)
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
				return true, obj, errVersionConflict
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
		klog.V(4).Infof("saved updated snapshot %s", snapshot.Name)
		return true, snapshot, nil

	case action.Matches("update", "volumegroupsnapshots"):
		obj := action.(core.UpdateAction).GetObject()
		groupSnapshot := obj.(*crdv1beta1.VolumeGroupSnapshot)

		// Check and bump object version
		storedGroupSnapshot, found := r.groupSnapshots[groupSnapshot.Name]
		if found {
			storedVer, _ := strconv.Atoi(storedGroupSnapshot.ResourceVersion)
			requestedVer, _ := strconv.Atoi(groupSnapshot.ResourceVersion)
			if storedVer != requestedVer {
				return true, obj, errVersionConflict
			}
			// Don't modify the existing object
			groupSnapshot = groupSnapshot.DeepCopy()
			groupSnapshot.ResourceVersion = strconv.Itoa(storedVer + 1)
		} else {
			return true, nil, fmt.Errorf("cannot update group snapshot %s: snapshot not found", groupSnapshot.Name)
		}

		// Store the updated object to appropriate places.
		r.groupSnapshots[groupSnapshot.Name] = groupSnapshot
		r.changedObjects = append(r.changedObjects, groupSnapshot)
		r.changedSinceLastSync++
		klog.V(4).Infof("saved updated snapshot %s", groupSnapshot.Name)
		return true, groupSnapshot, nil

	case action.Matches("patch", "volumesnapshots"):
		action := action.(core.PatchAction)
		// Check and bump object version
		storedSnapshot, found := r.snapshots[action.GetName()]
		if found {
			// Apply patch
			storedSnapshotBytes, err := json.Marshal(storedSnapshot)
			if err != nil {
				return true, nil, err
			}
			snapPatch, err := jsonpatch.DecodePatch(action.GetPatch())
			if err != nil {
				return true, nil, err
			}

			modified, err := snapPatch.Apply(storedSnapshotBytes)
			if err != nil {
				return true, nil, err
			}

			err = json.Unmarshal(modified, storedSnapshot)
			if err != nil {
				return true, nil, err
			}

			storedVer, _ := strconv.Atoi(storedSnapshot.ResourceVersion)
			storedSnapshot.ResourceVersion = strconv.Itoa(storedVer + 1)
		} else {
			return true, nil, fmt.Errorf("cannot update snapshot %s: snapshot not found", action.GetName())
		}

		// Store the updated object to appropriate places.
		r.snapshots[storedSnapshot.Name] = storedSnapshot
		r.changedObjects = append(r.changedObjects, storedSnapshot)
		r.changedSinceLastSync++

		klog.V(4).Infof("saved updated snapshot %s", storedSnapshot.Name)
		return true, storedSnapshot, nil

	case action.Matches("patch", "volumegroupsnapshots"):
		action := action.(core.PatchAction)
		// Check and bump object version
		storedGroupSnapshot, found := r.groupSnapshots[action.GetName()]
		if found {
			// Apply patch
			storedGroupSnapshotBytes, err := json.Marshal(storedGroupSnapshot)
			if err != nil {
				return true, nil, err
			}
			groupSnapPatch, err := jsonpatch.DecodePatch(action.GetPatch())
			if err != nil {
				return true, nil, err
			}

			modified, err := groupSnapPatch.Apply(storedGroupSnapshotBytes)
			if err != nil {
				return true, nil, err
			}

			err = json.Unmarshal(modified, storedGroupSnapshot)
			if err != nil {
				return true, nil, err
			}

			storedVer, _ := strconv.Atoi(storedGroupSnapshot.ResourceVersion)
			storedGroupSnapshot.ResourceVersion = strconv.Itoa(storedVer + 1)
		} else {
			return true, nil, fmt.Errorf("cannot update group snapshot %s: snapshot not found", action.GetName())
		}

		// Store the updated object to appropriate places.
		r.groupSnapshots[storedGroupSnapshot.Name] = storedGroupSnapshot
		r.changedObjects = append(r.changedObjects, storedGroupSnapshot)
		r.changedSinceLastSync++

		klog.V(4).Infof("saved updated group snapshot %s", storedGroupSnapshot.Name)
		return true, storedGroupSnapshot, nil

	case action.Matches("get", "volumesnapshotcontents"):
		name := action.(core.GetAction).GetName()
		content, found := r.contents[name]
		if found {
			klog.V(4).Infof("GetVolumeSnapshotContent: found %s", content.Name)
			return true, content, nil
		}
		klog.V(4).Infof("GetVolumeSnapshotContent: content %s not found", name)
		return true, nil, fmt.Errorf("cannot find content %s", name)

	case action.Matches("get", "volumegroupsnapshotcontents"):
		name := action.(core.GetAction).GetName()
		content, found := r.groupContents[name]
		if found {
			klog.V(4).Infof("GetVolumeGroupSnapshotContent: found %s", content.Name)
			return true, content, nil
		}
		klog.V(4).Infof("GetVolumeGroupSnapshotContent: content %s not found", name)
		return true, nil, fmt.Errorf("cannot find content %s", name)

	case action.Matches("get", "volumesnapshots"):
		name := action.(core.GetAction).GetName()
		snapshot, found := r.snapshots[name]
		if found {
			klog.V(4).Infof("GetVolumeSnapshot: found %s", snapshot.Name)
			return true, snapshot, nil
		}
		klog.V(4).Infof("GetVolumeSnapshot: content %s not found", name)
		return true, nil, fmt.Errorf("cannot find snapshot %s", name)

	case action.Matches("get", "volumegroupsnapshots"):
		name := action.(core.GetAction).GetName()
		groupSnapshot, found := r.groupSnapshots[name]
		if found {
			klog.V(4).Infof("GetVolumeGroupSnapshot: found %s", groupSnapshot.Name)
			return true, groupSnapshot, nil
		}
		klog.V(4).Infof("GetVolumeGroupSnapshot: content %s not found", name)
		return true, nil, fmt.Errorf("cannot find snapshot %s", name)

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

	case action.Matches("delete", "volumegroupsnapshotcontents"):
		name := action.(core.DeleteAction).GetName()
		klog.V(4).Infof("deleted group content %s", name)
		_, found := r.groupContents[name]
		if found {
			delete(r.groupContents, name)
			r.changedSinceLastSync++
			return true, nil, nil
		}
		return true, nil, fmt.Errorf("cannot delete group snapshot content %s: not found", name)

	case action.Matches("delete", "volumesnapshots"):
		name := action.(core.DeleteAction).GetName()
		klog.V(4).Infof("deleted snapshot %s", name)
		_, found := r.snapshots[name]
		if found {
			delete(r.snapshots, name)
			r.changedSinceLastSync++
			return true, nil, nil
		}
		return true, nil, fmt.Errorf("cannot delete snapshot %s: not found", name)

	case action.Matches("delete", "volumegroupsnapshots"):
		name := action.(core.DeleteAction).GetName()
		klog.V(4).Infof("deleted volume group snapshot %s", name)
		_, found := r.groupSnapshots[name]
		if found {
			delete(r.groupSnapshots, name)
			r.changedSinceLastSync++
			return true, nil, nil
		}
		return true, nil, fmt.Errorf("cannot delete group snapshot %s: not found", name)

	case action.Matches("get", "persistentvolumes"):
		name := action.(core.GetAction).GetName()
		volume, found := r.volumes[name]
		if found {
			klog.V(4).Infof("GetVolume: found %s", volume.Name)
			return true, volume, nil
		}
		klog.V(4).Infof("GetVolume: volume %s not found", name)
		return true, nil, fmt.Errorf("cannot find volume %s", name)

	case action.Matches("get", "persistentvolumeclaims"):
		name := action.(core.GetAction).GetName()
		claim, found := r.claims[name]
		if found {
			klog.V(4).Infof("GetClaim: found %s", claim.Name)
			return true, claim, nil
		}
		klog.V(4).Infof("GetClaim: claim %s not found", name)
		return true, nil, fmt.Errorf("cannot find claim %s", name)

	case action.Matches("list", "persistentvolumeclaims"):
		matchingLabels := action.(core.ListAction).GetListRestrictions().Labels
		var result []v1.PersistentVolumeClaim
		for _, claim := range r.claims {
			if matchingLabels.Matches(labels.Set(claim.Labels)) {
				result = append(result, *claim)
			}
		}
		klog.V(4).Infof("ListClaim: found %v", result)
		return true, &v1.PersistentVolumeClaimList{
			Items: result,
		}, nil

	case action.Matches("update", "persistentvolumeclaims"):
		obj := action.(core.UpdateAction).GetObject()
		claim := obj.(*v1.PersistentVolumeClaim)

		// Check and bump object version
		storedClaim, found := r.claims[claim.Name]
		if found {
			storedVer, _ := strconv.Atoi(storedClaim.ResourceVersion)
			requestedVer, _ := strconv.Atoi(claim.ResourceVersion)
			if storedVer != requestedVer {
				return true, obj, errVersionConflict
			}
			// Don't modify the existing object
			claim = claim.DeepCopy()
			claim.ResourceVersion = strconv.Itoa(storedVer + 1)
		} else {
			return true, nil, fmt.Errorf("cannot update claim %s: claim not found", claim.Name)
		}

		// Store the updated object to appropriate places.
		r.claims[claim.Name] = claim
		r.changedObjects = append(r.changedObjects, claim)
		r.changedSinceLastSync++
		klog.V(4).Infof("saved updated claim %s", claim.Name)
		return true, claim, nil

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

// checkGroupContents compares all expectedGroupContents with set of contents at the end of
// the test and reports differences.
func (r *snapshotReactor) checkGroupContents(expectedGroupContents []*crdv1beta1.VolumeGroupSnapshotContent) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	expectedMap := make(map[string]*crdv1beta1.VolumeGroupSnapshotContent)
	gotMap := make(map[string]*crdv1beta1.VolumeGroupSnapshotContent)
	// Clear any ResourceVersion from both sets
	for _, v := range expectedGroupContents {
		// Don't modify the existing object
		v := v.DeepCopy()
		v.ResourceVersion = ""
		sort.Strings(v.Spec.Source.VolumeHandles)
		if v.Status != nil {
			v.Status.CreationTime = nil
		}
		expectedMap[v.Name] = v
	}
	for _, v := range r.groupContents {
		// We must clone the content because of golang race check - it was
		// written by the controller without any locks on it.
		v := v.DeepCopy()
		v.ResourceVersion = ""
		sort.Strings(v.Spec.Source.VolumeHandles)
		if v.Status != nil {
			v.Status.CreationTime = nil
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
		if c.Status != nil && c.Status.Error != nil {
			c.Status.Error.Time = &metav1.Time{}
		}
		expectedMap[c.Name] = c
	}
	for _, c := range r.snapshots {
		// We must clone the snapshot because of golang race check - it was
		// written by the controller without any locks on it.
		c = c.DeepCopy()
		c.ResourceVersion = ""
		if c.Status != nil && c.Status.Error != nil {
			c.Status.Error.Time = &metav1.Time{}
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

// checkGroupSnapshots compares all expectedGroupSnapshots with set of snapshots at the end of the
// test and reports differences.
func (r *snapshotReactor) checkGroupSnapshots(expectedGroupSnapshots []*crdv1beta1.VolumeGroupSnapshot) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	expectedMap := make(map[string]*crdv1beta1.VolumeGroupSnapshot)
	gotMap := make(map[string]*crdv1beta1.VolumeGroupSnapshot)
	for _, c := range expectedGroupSnapshots {
		// Don't modify the existing object
		c = c.DeepCopy()
		c.ResourceVersion = ""
		if c.Status != nil && c.Status.Error != nil {
			c.Status.Error.Time = &metav1.Time{}
		}
		expectedMap[c.Name] = c
	}
	for _, c := range r.groupSnapshots {
		// We must clone the snapshot because of golang race check - it was
		// written by the controller without any locks on it.
		c = c.DeepCopy()
		c.ResourceVersion = ""
		if c.Status != nil && c.Status.Error != nil {
			c.Status.Error.Time = &metav1.Time{}
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
func checkEvents(t *testing.T, expectedEvents []string, ctrl *csiSnapshotCommonController) error {
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
		case *crdv1.VolumeSnapshot:
			snapshot, _ := obj.(*crdv1.VolumeSnapshot)
			klog.V(4).Infof("reactor queue: %s", snapshot.Name)
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
	for _, pvc := range r.claims {
		r.changedObjects = append(r.changedObjects, pvc)
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
		// Return 'true' if the reactor reached the expected state
		err1 := r.checkSnapshots(test.expectedSnapshots)
		err2 := r.checkContents(test.expectedContents)
		err3 := r.checkGroupSnapshots(test.expectedGroupSnapshots)
		err4 := r.checkGroupContents(test.expectedGroupContents)
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
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

// addSnapshotEvent simulates that a snapshot has been created in etcd and the
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

func newSnapshotReactor(kubeClient *kubefake.Clientset, client *fake.Clientset, ctrl *csiSnapshotCommonController, fakeVolumeWatch, fakeClaimWatch *watch.FakeWatcher, errors []reactorError) *snapshotReactor {
	reactor := &snapshotReactor{
		secrets:              make(map[string]*v1.Secret),
		volumes:              make(map[string]*v1.PersistentVolume),
		claims:               make(map[string]*v1.PersistentVolumeClaim),
		snapshotClasses:      make(map[string]*crdv1.VolumeSnapshotClass),
		contents:             make(map[string]*crdv1.VolumeSnapshotContent),
		snapshots:            make(map[string]*crdv1.VolumeSnapshot),
		groupSnapshotClasses: make(map[string]*crdv1beta1.VolumeGroupSnapshotClass),
		groupContents:        make(map[string]*crdv1beta1.VolumeGroupSnapshotContent),
		groupSnapshots:       make(map[string]*crdv1beta1.VolumeGroupSnapshot),
		ctrl:                 ctrl,
		fakeContentWatch:     fakeVolumeWatch,
		fakeSnapshotWatch:    fakeClaimWatch,
		errors:               errors,
	}

	client.AddReactor("create", "volumesnapshotcontents", reactor.React)
	client.AddReactor("create", "volumegroupsnapshotcontents", reactor.React)
	client.AddReactor("update", "volumesnapshotcontents", reactor.React)
	client.AddReactor("update", "volumegroupsnapshotcontents", reactor.React)
	client.AddReactor("update", "volumesnapshots", reactor.React)
	client.AddReactor("update", "volumegroupsnapshots", reactor.React)
	client.AddReactor("patch", "volumesnapshotcontents", reactor.React)
	client.AddReactor("patch", "volumegroupsnapshotcontents", reactor.React)
	client.AddReactor("patch", "volumesnapshots", reactor.React)
	client.AddReactor("patch", "volumegroupsnapshots", reactor.React)
	client.AddReactor("update", "volumesnapshotclasses", reactor.React)
	client.AddReactor("update", "volumegroupsnapshotclasses", reactor.React)
	client.AddReactor("get", "volumesnapshotcontents", reactor.React)
	client.AddReactor("get", "volumegroupsnapshotcontents", reactor.React)
	client.AddReactor("get", "volumesnapshots", reactor.React)
	client.AddReactor("get", "volumegroupsnapshots", reactor.React)
	client.AddReactor("get", "volumesnapshotclasses", reactor.React)
	client.AddReactor("get", "volumegroupsnapshotclasses", reactor.React)
	client.AddReactor("delete", "volumesnapshotcontents", reactor.React)
	client.AddReactor("delete", "volumegroupsnapshotcontents", reactor.React)
	client.AddReactor("delete", "volumesnapshots", reactor.React)
	client.AddReactor("delete", "volumegroupsnapshots", reactor.React)
	client.AddReactor("delete", "volumesnapshotclasses", reactor.React)
	client.AddReactor("delete", "volumegroupsnapshotclasses", reactor.React)
	kubeClient.AddReactor("get", "persistentvolumeclaims", reactor.React)
	kubeClient.AddReactor("list", "persistentvolumeclaims", reactor.React)
	kubeClient.AddReactor("update", "persistentvolumeclaims", reactor.React)
	kubeClient.AddReactor("get", "persistentvolumes", reactor.React)
	kubeClient.AddReactor("get", "secrets", reactor.React)

	return reactor
}

func alwaysReady() bool { return true }

func newTestController(kubeClient kubernetes.Interface, clientset clientset.Interface,
	informerFactory informers.SharedInformerFactory, t *testing.T, test controllerTest) (*csiSnapshotCommonController, error) {
	if informerFactory == nil {
		informerFactory = informers.NewSharedInformerFactory(clientset, utils.NoResyncPeriodFunc())
	}

	coreFactory := coreinformers.NewSharedInformerFactory(kubeClient, utils.NoResyncPeriodFunc())
	metricsManager := metrics.NewMetricsManager()
	mux := http.NewServeMux()
	metricsManager.PrepareMetricsPath(mux, "/metrics", nil)
	go func() {
		err := http.ListenAndServe("localhost:0", mux)
		if err != nil {
			t.Errorf("failed to prepare metrics path: %v", err)
		}
	}()

	ctrl := NewCSISnapshotCommonController(
		clientset,
		kubeClient,
		informerFactory.Snapshot().V1().VolumeSnapshots(),
		informerFactory.Snapshot().V1().VolumeSnapshotContents(),
		informerFactory.Snapshot().V1().VolumeSnapshotClasses(),
		informerFactory.Groupsnapshot().V1beta1().VolumeGroupSnapshots(),
		informerFactory.Groupsnapshot().V1beta1().VolumeGroupSnapshotContents(),
		informerFactory.Groupsnapshot().V1beta1().VolumeGroupSnapshotClasses(),
		coreFactory.Core().V1().PersistentVolumeClaims(),
		coreFactory.Core().V1().PersistentVolumes(),
		nil,
		metricsManager,
		60*time.Second,
		workqueue.NewItemExponentialFailureRateLimiter(1*time.Millisecond, 1*time.Minute),
		workqueue.NewItemExponentialFailureRateLimiter(1*time.Millisecond, 1*time.Minute),
		workqueue.NewItemExponentialFailureRateLimiter(1*time.Millisecond, 1*time.Minute),
		workqueue.NewItemExponentialFailureRateLimiter(1*time.Millisecond, 1*time.Minute),
		false,
		false,
		true,
	)

	ctrl.eventRecorder = record.NewFakeRecorder(1000)

	ctrl.contentListerSynced = alwaysReady
	ctrl.snapshotListerSynced = alwaysReady
	ctrl.classListerSynced = alwaysReady
	ctrl.pvcListerSynced = alwaysReady

	return ctrl, nil
}

func newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, creationTime, size *int64,
	withFinalizer bool, withStatus bool) *crdv1.VolumeSnapshotContent {
	ready := true
	content := crdv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:            contentName,
			ResourceVersion: "1",
		},
		Spec: crdv1.VolumeSnapshotContentSpec{
			Driver:         mockDriverName,
			DeletionPolicy: deletionPolicy,
		},
	}

	if withStatus {
		content.Status = &crdv1.VolumeSnapshotContentStatus{
			CreationTime: creationTime,
			RestoreSize:  size,
			ReadyToUse:   &ready,
		}
	}

	if withStatus && snapshotHandle != "" {
		content.Status.SnapshotHandle = &snapshotHandle
	}

	if snapshotClassName != "" {
		content.Spec.VolumeSnapshotClassName = &snapshotClassName
	}

	if volumeHandle != "" {
		content.Spec.Source.VolumeHandle = &volumeHandle
	}

	if desiredSnapshotHandle != "" {
		content.Spec.Source.SnapshotHandle = &desiredSnapshotHandle
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

func newGroupSnapshotContent(groupSnapshotContentName, boundToGroupSnapshotUID, boundToGroupSnapshotName, groupSnapshotHandle, groupSnapshotClassName string, desiredVolumeHandles []string, targetVolumeGroupSnapshotHandle string,
	deletionPolicy crdv1.DeletionPolicy, creationTime *int64,
	withFinalizer bool, withStatus bool) *crdv1beta1.VolumeGroupSnapshotContent {
	ready := true
	content := crdv1beta1.VolumeGroupSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:            groupSnapshotContentName,
			ResourceVersion: "1",
		},
		Spec: crdv1beta1.VolumeGroupSnapshotContentSpec{
			Driver:         mockDriverName,
			DeletionPolicy: deletionPolicy,
		},
	}

	if withStatus {
		content.Status = &crdv1beta1.VolumeGroupSnapshotContentStatus{
			CreationTime: creationTime,
			ReadyToUse:   &ready,
		}
	}

	if withStatus && groupSnapshotHandle != "" {
		content.Status.VolumeGroupSnapshotHandle = &groupSnapshotHandle
	}

	if groupSnapshotClassName != "" {
		content.Spec.VolumeGroupSnapshotClassName = &groupSnapshotClassName
	}

	if targetVolumeGroupSnapshotHandle != "" {
		content.Spec.Source.GroupSnapshotHandles = &crdv1beta1.GroupSnapshotHandles{
			VolumeGroupSnapshotHandle: targetVolumeGroupSnapshotHandle,
		}
	}

	if len(desiredVolumeHandles) != 0 {
		content.Spec.Source.VolumeHandles = desiredVolumeHandles
	}

	if boundToGroupSnapshotName != "" {
		content.Spec.VolumeGroupSnapshotRef = v1.ObjectReference{
			Kind:            "VolumeGroupSnapshot",
			APIVersion:      "groupsnapshot.storage.k8s.io/v1beta1",
			UID:             types.UID(boundToGroupSnapshotUID),
			Namespace:       testNamespace,
			Name:            boundToGroupSnapshotName,
			ResourceVersion: "1",
		}
	}

	if withFinalizer {
		return withGroupContentFinalizer(&content)
	}
	return &content
}

func newGroupSnapshotContentArray(groupSnapshotContentName, boundToGroupSnapshotUID, boundToGroupSnapshotSnapshotName, groupSnapshotHandle, groupSnapshotClassName string, desiredVolumeHandles []string, volumeGroupHandle string,
	deletionPolicy crdv1.DeletionPolicy, creationTime *int64,
	withFinalizer bool, withStatus bool) []*crdv1beta1.VolumeGroupSnapshotContent {
	return []*crdv1beta1.VolumeGroupSnapshotContent{
		newGroupSnapshotContent(groupSnapshotContentName, boundToGroupSnapshotUID, boundToGroupSnapshotSnapshotName, groupSnapshotHandle, groupSnapshotClassName, desiredVolumeHandles, volumeGroupHandle,
			deletionPolicy, creationTime,
			withFinalizer, withStatus),
	}
}

func withContentAnnotations(contents []*crdv1.VolumeSnapshotContent, annotations map[string]string) []*crdv1.VolumeSnapshotContent {
	for i := range contents {
		if contents[i].ObjectMeta.Annotations == nil {
			contents[i].ObjectMeta.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			contents[i].ObjectMeta.Annotations[k] = v
		}
	}
	return contents
}

func withContentSpecSnapshotClassName(contents []*crdv1.VolumeSnapshotContent, volumeSnapshotClassName *string) []*crdv1.VolumeSnapshotContent {
	for i := range contents {
		contents[i].Spec.VolumeSnapshotClassName = volumeSnapshotClassName
	}
	return contents
}

func withContentFinalizer(content *crdv1.VolumeSnapshotContent) *crdv1.VolumeSnapshotContent {
	content.ObjectMeta.Finalizers = append(content.ObjectMeta.Finalizers, utils.VolumeSnapshotContentFinalizer)
	return content
}

func withGroupContentFinalizer(content *crdv1beta1.VolumeGroupSnapshotContent) *crdv1beta1.VolumeGroupSnapshotContent {
	content.ObjectMeta.Finalizers = append(content.ObjectMeta.Finalizers, utils.VolumeGroupSnapshotContentFinalizer)
	return content
}

func withGroupContentAnnotations(contents []*crdv1beta1.VolumeGroupSnapshotContent, annotations map[string]string) []*crdv1beta1.VolumeGroupSnapshotContent {
	for i := range contents {
		if contents[i].ObjectMeta.Annotations == nil {
			contents[i].ObjectMeta.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			contents[i].ObjectMeta.Annotations[k] = v
		}
	}
	return contents
}

func newContentArray(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, size, creationTime *int64,
	withFinalizer bool) []*crdv1.VolumeSnapshotContent {
	return []*crdv1.VolumeSnapshotContent{
		newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, creationTime, size, withFinalizer, true),
	}
}

func newContentArrayNoStatus(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, size, creationTime *int64,
	withFinalizer bool, withStatus bool) []*crdv1.VolumeSnapshotContent {
	return []*crdv1.VolumeSnapshotContent{
		newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, creationTime, size, withFinalizer, withStatus),
	}
}

func newContentArrayWithReadyToUse(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, creationTime, size *int64, readyToUse *bool,
	withFinalizer bool) []*crdv1.VolumeSnapshotContent {
	content := newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, creationTime, size, withFinalizer, true)
	content.Status.ReadyToUse = readyToUse
	return []*crdv1.VolumeSnapshotContent{
		content,
	}
}

func newContentWithUnmatchDriverArray(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, size, creationTime *int64,
	withFinalizer bool) []*crdv1.VolumeSnapshotContent {
	content := newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, size, creationTime, withFinalizer, true)
	content.Spec.Driver = "fake"
	return []*crdv1.VolumeSnapshotContent{
		content,
	}
}

func newContentArrayWithError(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle string,
	deletionPolicy crdv1.DeletionPolicy, size, creationTime *int64,
	withFinalizer bool, snapshotErr *crdv1.VolumeSnapshotError) []*crdv1.VolumeSnapshotContent {
	content := newContent(contentName, boundToSnapshotUID, boundToSnapshotName, snapshotHandle, snapshotClassName, desiredSnapshotHandle, volumeHandle, deletionPolicy, size, creationTime, withFinalizer, true)
	ready := false
	content.Status.ReadyToUse = &ready
	content.Status.Error = snapshotErr
	return []*crdv1.VolumeSnapshotContent{
		content,
	}
}

func newSnapshot(
	snapshotName, snapshotUID, pvcName, targetContentName, snapshotClassName, boundContentName string,
	readyToUse *bool, creationTime *metav1.Time, restoreSize *resource.Quantity,
	err *crdv1.VolumeSnapshotError, nilStatus bool, withAllFinalizers bool, deletionTimestamp *metav1.Time) *crdv1.VolumeSnapshot {
	snapshot := crdv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:              snapshotName,
			Namespace:         testNamespace,
			UID:               types.UID(snapshotUID),
			ResourceVersion:   "1",
			SelfLink:          "/apis/snapshot.storage.k8s.io/v1/namespaces/" + testNamespace + "/volumesnapshots/" + snapshotName,
			DeletionTimestamp: deletionTimestamp,
		},
		Spec: crdv1.VolumeSnapshotSpec{
			VolumeSnapshotClassName: nil,
		},
	}

	if !nilStatus {
		snapshot.Status = &crdv1.VolumeSnapshotStatus{
			CreationTime: creationTime,
			ReadyToUse:   readyToUse,
			Error:        err,
			RestoreSize:  restoreSize,
		}
	}

	if boundContentName != "" {
		snapshot.Status.BoundVolumeSnapshotContentName = &boundContentName
	}

	if snapshotClassName != "" {
		snapshot.Spec.VolumeSnapshotClassName = &snapshotClassName
	}

	if pvcName != "" {
		snapshot.Spec.Source.PersistentVolumeClaimName = &pvcName
	}
	if targetContentName != "" {
		snapshot.Spec.Source.VolumeSnapshotContentName = &targetContentName
	}
	if withAllFinalizers {
		return withSnapshotFinalizers([]*crdv1.VolumeSnapshot{&snapshot}, utils.VolumeSnapshotAsSourceFinalizer, utils.VolumeSnapshotBoundFinalizer)[0]
	}
	return &snapshot
}

func newGroupSnapshot(
	groupSnapshotName, groupSnapshotUID string, selectors map[string]string, targetContentName, groupSnapshotClassName, boundContentName string,
	readyToUse *bool, creationTime *metav1.Time,
	err *crdv1.VolumeSnapshotError, nilStatus bool, withAllFinalizers bool, deletionTimestamp *metav1.Time) *crdv1beta1.VolumeGroupSnapshot {
	groupSnapshot := crdv1beta1.VolumeGroupSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:              groupSnapshotName,
			Namespace:         testNamespace,
			UID:               types.UID(groupSnapshotUID),
			ResourceVersion:   "1",
			SelfLink:          "/apis/groupsnapshot.storage.k8s.io/v1beta1/namespaces/" + testNamespace + "/volumesnapshots/" + groupSnapshotName,
			DeletionTimestamp: deletionTimestamp,
		},
		Spec: crdv1beta1.VolumeGroupSnapshotSpec{
			VolumeGroupSnapshotClassName: nil,
		},
	}

	if len(selectors) > 0 {
		groupSnapshot.Spec.Source.Selector = &metav1.LabelSelector{
			MatchLabels: selectors,
		}
	}

	if !nilStatus {
		groupSnapshot.Status = &crdv1beta1.VolumeGroupSnapshotStatus{
			CreationTime: creationTime,
			ReadyToUse:   readyToUse,
			Error:        err,
		}

		if boundContentName != "" {
			groupSnapshot.Status.BoundVolumeGroupSnapshotContentName = &boundContentName
		}
	}

	if groupSnapshotClassName != "" {
		groupSnapshot.Spec.VolumeGroupSnapshotClassName = &groupSnapshotClassName
	}

	if targetContentName != "" {
		groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName = &targetContentName
	}
	if withAllFinalizers {
		return withGroupSnapshotFinalizers([]*crdv1beta1.VolumeGroupSnapshot{&groupSnapshot}, utils.VolumeGroupSnapshotContentFinalizer, utils.VolumeGroupSnapshotBoundFinalizer)[0]
	}
	return &groupSnapshot
}

func newGroupSnapshotArray(
	groupSnapshotName, groupSnapshotUID string, selectors map[string]string, targetContentName, groupSnapshotClassName, boundContentName string,
	readyToUse *bool, creationTime *metav1.Time,
	err *crdv1.VolumeSnapshotError, nilStatus bool, withAllFinalizers bool, deletionTimestamp *metav1.Time) []*crdv1beta1.VolumeGroupSnapshot {
	return []*crdv1beta1.VolumeGroupSnapshot{
		newGroupSnapshot(groupSnapshotName, groupSnapshotUID, selectors, targetContentName, groupSnapshotClassName, boundContentName, readyToUse, creationTime, err, nilStatus, withAllFinalizers, deletionTimestamp),
	}
}

func newSnapshotArray(
	groupSnapshotName, groupSnapshotUID, pvcName, targetContentName, groupSnapshotClassName, boundContentName string,
	readyToUse *bool, creationTime *metav1.Time, restoreSize *resource.Quantity,
	err *crdv1.VolumeSnapshotError, nilStatus bool, withAllFinalizers bool, deletionTimestamp *metav1.Time) []*crdv1.VolumeSnapshot {
	return []*crdv1.VolumeSnapshot{
		newSnapshot(groupSnapshotName, groupSnapshotUID, pvcName, targetContentName, groupSnapshotClassName, boundContentName, readyToUse, creationTime, restoreSize, err, nilStatus, withAllFinalizers, deletionTimestamp),
	}
}

func newSnapshotClass(snapshotClassName, snapshotClassUID, driverName string, isDefaultClass bool) *crdv1.VolumeSnapshotClass {
	sc := &crdv1.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            snapshotClassName,
			Namespace:       testNamespace,
			UID:             types.UID(snapshotClassUID),
			ResourceVersion: "1",
			SelfLink:        "/apis/snapshot.storage.k8s.io/v1/namespaces/" + testNamespace + "/volumesnapshotclasses/" + snapshotClassName,
		},
		Driver: driverName,
	}
	if isDefaultClass {
		sc.Annotations = make(map[string]string)
		sc.Annotations[utils.IsDefaultSnapshotClassAnnotation] = "true"
	}
	return sc
}

func newSnapshotClassArray(snapshotClassName, snapshotClassUID, driverName string, isDefaultClass bool) []*crdv1.VolumeSnapshotClass {
	return []*crdv1.VolumeSnapshotClass{
		newSnapshotClass(snapshotClassName, snapshotClassUID, driverName, isDefaultClass),
	}
}

// newClaim returns a new claim with given attributes
func newClaim(name, claimUID, capacity, boundToVolume string, phase v1.PersistentVolumeClaimPhase, class *string, bFinalizer bool) *v1.PersistentVolumeClaim {
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
			Resources: v1.VolumeResourceRequirements{
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

	if bFinalizer {
		return withPVCFinalizer(&claim)
	}
	return &claim
}

// newClaimArray returns array with a single claim that would be returned by
// newClaim() with the same parameters.
func newClaimArray(name, claimUID, capacity, boundToVolume string, phase v1.PersistentVolumeClaimPhase, class *string) []*v1.PersistentVolumeClaim {
	return []*v1.PersistentVolumeClaim{
		newClaim(name, claimUID, capacity, boundToVolume, phase, class, false),
	}
}

// newClaimCoupleArray returns array with two claims that would be returned by
// newClaim() with "1-" and "2-" as prefix for names and UID.
func newClaimCoupleArray(name, claimUID, capacity, boundToVolume string, phase v1.PersistentVolumeClaimPhase, class *string) []*v1.PersistentVolumeClaim {
	pre1 := func(s string) string {
		if len(s) == 0 {
			return s
		}
		return fmt.Sprintf("1-%s", s)
	}
	pre2 := func(s string) string {
		if len(s) == 0 {
			return s
		}
		return fmt.Sprintf("2-%s", s)
	}
	return []*v1.PersistentVolumeClaim{
		newClaim(pre1(name), pre1(claimUID), capacity, pre1(boundToVolume), phase, class, false),
		newClaim(pre2(name), pre2(claimUID), capacity, pre2(boundToVolume), phase, class, false),
	}
}

// newClaimArrayFinalizer returns array with a single claim that would be returned by
// newClaim() with the same parameters plus finalizer.
func newClaimArrayFinalizer(name, claimUID, capacity, boundToVolume string, phase v1.PersistentVolumeClaimPhase, class *string) []*v1.PersistentVolumeClaim {
	return []*v1.PersistentVolumeClaim{
		newClaim(name, claimUID, capacity, boundToVolume, phase, class, true),
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

// newVolumeCoupleArray returns array with two volumes that would be returned by
// newVolume() with the same parameters, adding "1-" and "2-" as prefix to their
// names and UIDs.
func newVolumeCoupleArray(name, volumeUID, volumeHandle, capacity, boundToClaimUID, boundToClaimName string, phase v1.PersistentVolumePhase, reclaimPolicy v1.PersistentVolumeReclaimPolicy, class string) []*v1.PersistentVolume {
	pre1 := func(s string) string {
		if len(s) == 0 {
			return s
		}
		return fmt.Sprintf("1-%s", s)
	}
	pre2 := func(s string) string {
		if len(s) == 0 {
			return s
		}
		return fmt.Sprintf("2-%s", s)
	}
	return []*v1.PersistentVolume{
		newVolume(pre1(name), pre1(volumeUID), pre1(volumeHandle), capacity, pre1(boundToClaimUID), pre1(boundToClaimName), phase, reclaimPolicy, class),
		newVolume(pre2(name), pre2(volumeUID), pre2(volumeHandle), capacity, pre2(boundToClaimUID), pre2(boundToClaimName), phase, reclaimPolicy, class),
	}
}

func newVolumeError(message string) *crdv1.VolumeSnapshotError {
	return &crdv1.VolumeSnapshotError{
		Time:    &metav1.Time{},
		Message: &message,
	}
}

func testSyncSnapshot(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.syncSnapshot(context.TODO(), test.initialSnapshots[0])
}

func testSyncGroupSnapshot(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.syncGroupSnapshot(context.TODO(), test.initialGroupSnapshots[0])
}

func testSyncSnapshotError(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	err := ctrl.syncSnapshot(context.TODO(), test.initialSnapshots[0])
	if err != nil {
		return nil
	}
	return fmt.Errorf("syncSnapshot succeeded when failure was expected")
}

func testUpdateSnapshotErrorStatus(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	snapshot, err := ctrl.updateSnapshotStatus(test.initialSnapshots[0], test.initialContents[0])
	if err != nil {
		return fmt.Errorf("update snapshot status failed: %v", err)
	}
	var expected, got *crdv1.VolumeSnapshotError
	if test.initialContents[0].Status != nil {
		expected = test.initialContents[0].Status.Error
	}
	if snapshot.Status != nil {
		got = snapshot.Status.Error
	}
	if expected == nil && got != nil {
		return fmt.Errorf("update snapshot status failed: expected nil but got: %v", got)
	}
	if expected != nil && got == nil {
		return fmt.Errorf("update snapshot status failed: expected: %v but got nil", expected)
	}
	if expected != nil && got != nil && !reflect.DeepEqual(expected, got) {
		return fmt.Errorf("update snapshot status failed [A-expected, B-got]: %s", diff.ObjectDiff(expected, got))
	}
	return nil
}

func testSyncContent(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.syncContent(test.initialContents[0])
}

func testSyncContentError(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	err := ctrl.syncContent(test.initialContents[0])
	if err != nil {
		return nil
	}
	return fmt.Errorf("syncContent succeeded when failure was expected")
}

func testAddPVCFinalizer(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.ensurePVCFinalizer(test.initialSnapshots[0])
}

func testRemovePVCFinalizer(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.checkandRemovePVCFinalizer(test.initialSnapshots[0], false)
}

func testAddSnapshotFinalizer(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.addSnapshotFinalizer(test.initialSnapshots[0], true, true)
}

func testAddSingleSnapshotFinalizer(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.addSnapshotFinalizer(test.initialSnapshots[0], false, true)
}

func testRemoveSnapshotFinalizer(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	return ctrl.removeSnapshotFinalizer(test.initialSnapshots[0], true, true, false)
}

func testUpdateSnapshotClass(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	snap, err := ctrl.checkAndUpdateSnapshotClass(test.initialSnapshots[0])
	// syncSnapshotByKey expects that checkAndUpdateSnapshotClass always returns a snapshot
	if snap == nil {
		return testError(fmt.Sprintf("checkAndUpdateSnapshotClass returned nil snapshot on error: %v", err))
	}
	return err
}

func testNewSnapshotContentCreation(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	if err := ctrl.syncUnreadySnapshot(test.initialSnapshots[0]); err != nil {
		return fmt.Errorf("syncUnreadySnapshot failed: %v", err)
	}

	return nil
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
func wrapTestWithInjectedOperation(toWrap testCall, injectBeforeOperation func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor)) testCall {
	return func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
		// Inject a hook before async operation starts
		klog.V(4).Infof("reactor:injecting call")
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

func evaluateTestResults(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest, t *testing.T) {
	// Evaluate results
	if err := reactor.checkSnapshots(test.expectedSnapshots); err != nil {
		t.Errorf("Test %q: %v", test.name, err)
	}
	if err := reactor.checkContents(test.expectedContents); err != nil {
		t.Errorf("Test %q: %v", test.name, err)
	}
	if err := reactor.checkGroupSnapshots(test.expectedGroupSnapshots); err != nil {
		t.Errorf("Test %q: %v", test.name, err)
	}
	if err := reactor.checkGroupContents(test.expectedGroupContents); err != nil {
		t.Errorf("Test %q: %v", test.name, err)
	}

	if err := checkEvents(t, test.expectedEvents, ctrl); err != nil {
		t.Errorf("Test %q: %v", test.name, err)
	}
}

// Test single call to syncSnapshot and syncContent methods.
// For all tests:
//  1. Fill in the controller with initial data
//  2. Call the tested function (syncSnapshot/syncContent) via
//     controllerTest.testCall *once*.
//  3. Compare resulting contents and snapshots with expected contents and snapshots.
func runSyncTests(t *testing.T, tests []controllerTest, snapshotClasses []*crdv1.VolumeSnapshotClass, groupSnapshotClasses []*crdv1beta1.VolumeGroupSnapshotClass) {
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
		for _, snapshot := range test.initialSnapshots {
			ctrl.snapshotStore.Add(snapshot)
			reactor.snapshots[snapshot.Name] = snapshot
		}
		for _, content := range test.initialContents {
			ctrl.contentStore.Add(content)
			reactor.contents[content.Name] = content
		}
		for _, groupsnapshot := range test.initialGroupSnapshots {
			ctrl.groupSnapshotStore.Add(groupsnapshot)
			reactor.groupSnapshots[groupsnapshot.Name] = groupsnapshot
		}
		for _, groupcontent := range test.initialGroupContents {
			ctrl.groupSnapshotContentStore.Add(groupcontent)
			reactor.groupContents[groupcontent.Name] = groupcontent
		}

		pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		for _, claim := range test.initialClaims {
			reactor.claims[claim.Name] = claim
			pvcIndexer.Add(claim)
		}
		ctrl.pvcLister = corelisters.NewPersistentVolumeClaimLister(pvcIndexer)

		for _, volume := range test.initialVolumes {
			reactor.volumes[volume.Name] = volume
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

		// Inject group snapshot classes into the controller
		groupIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		for _, groupClass := range groupSnapshotClasses {
			groupIndexer.Add(groupClass)
		}
		ctrl.groupSnapshotClassLister = groupstoragelisters.NewVolumeGroupSnapshotClassLister(groupIndexer)

		// Run the tested functions
		err = test.test(ctrl, reactor, test)
		if test.expectSuccess && err != nil {
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

// This tests that finalizers are added or removed from a PVC or Snapshot
func runFinalizerTests(t *testing.T, tests []controllerTest, snapshotClasses []*crdv1.VolumeSnapshotClass) {
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
		for _, snapshot := range test.initialSnapshots {
			ctrl.snapshotStore.Add(snapshot)
			reactor.snapshots[snapshot.Name] = snapshot
		}
		for _, content := range test.initialContents {
			ctrl.contentStore.Add(content)
			reactor.contents[content.Name] = content
		}

		pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		for _, claim := range test.initialClaims {
			reactor.claims[claim.Name] = claim
			pvcIndexer.Add(claim)
		}
		ctrl.pvcLister = corelisters.NewPersistentVolumeClaimLister(pvcIndexer)

		for _, volume := range test.initialVolumes {
			reactor.volumes[volume.Name] = volume
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

		// Verify Finalizer tests results
		evaluateFinalizerTests(ctrl, reactor, test, t)
	}
}

// Evaluate Finalizer tests results
func evaluateFinalizerTests(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest, t *testing.T) {
	// Evaluate results
	bHasPVCFinalizer := false
	bHasSnapshotFinalizer := false
	name := sysruntime.FuncForPC(reflect.ValueOf(test.test).Pointer()).Name()
	index := strings.LastIndex(name, ".")
	if index == -1 {
		t.Errorf("Test %q: failed to test finalizer - invalid test call name [%s]", test.name, name)
		return
	}
	names := []rune(name)
	funcName := string(names[index+1 : len(name)])
	klog.V(4).Infof("test %q: Finalizer test func name: [%s]", test.name, funcName)

	if strings.Contains(funcName, "PVCFinalizer") {
		if funcName == "testAddPVCFinalizer" {
			for _, pvc := range reactor.claims {
				if test.initialClaims[0].Name == pvc.Name {
					if !slices.Contains(test.initialClaims[0].ObjectMeta.Finalizers, utils.PVCFinalizer) && slices.Contains(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer) {
						klog.V(4).Infof("test %q succeeded. PVCFinalizer is added to PVC %s", test.name, pvc.Name)
						bHasPVCFinalizer = true
					}
					break
				}
			}
			if test.expectSuccess && !bHasPVCFinalizer {
				t.Errorf("Test %q: failed to add finalizer to PVC %s", test.name, test.initialClaims[0].Name)
			}
		}
		bHasPVCFinalizer = true
		if funcName == "testRemovePVCFinalizer" {
			for _, pvc := range reactor.claims {
				if test.initialClaims[0].Name == pvc.Name {
					if slices.Contains(test.initialClaims[0].ObjectMeta.Finalizers, utils.PVCFinalizer) && !slices.Contains(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer) {
						klog.V(4).Infof("test %q succeeded. PVCFinalizer is removed from PVC %s", test.name, pvc.Name)
						bHasPVCFinalizer = false
					}
					break
				}
			}
			if test.expectSuccess && bHasPVCFinalizer {
				t.Errorf("Test %q: failed to remove finalizer from PVC %s", test.name, test.initialClaims[0].Name)
			}
		}
	} else {
		if funcName == "testAddSnapshotFinalizer" {
			for _, snapshot := range reactor.snapshots {
				if test.initialSnapshots[0].Name == snapshot.Name {
					if !slices.Contains(test.initialSnapshots[0].ObjectMeta.Finalizers, utils.VolumeSnapshotBoundFinalizer) &&
						slices.Contains(snapshot.ObjectMeta.Finalizers, utils.VolumeSnapshotBoundFinalizer) &&
						!slices.Contains(test.initialSnapshots[0].ObjectMeta.Finalizers, utils.VolumeSnapshotAsSourceFinalizer) &&
						slices.Contains(snapshot.ObjectMeta.Finalizers, utils.VolumeSnapshotAsSourceFinalizer) {
						klog.V(4).Infof("test %q succeeded. Finalizers are added to snapshot %s", test.name, snapshot.Name)
						bHasSnapshotFinalizer = true
					}
					break
				}
			}
			if test.expectSuccess && !bHasSnapshotFinalizer {
				t.Errorf("Test %q: failed to add finalizer to Snapshot %s. Finalizers: %s", test.name, test.initialSnapshots[0].Name, test.initialSnapshots[0].GetFinalizers())
			}
		}
		bHasSnapshotFinalizer = true
		if funcName == "testRemoveSnapshotFinalizer" {
			for _, snapshot := range reactor.snapshots {
				if test.initialSnapshots[0].Name == snapshot.Name {
					if slices.Contains(test.initialSnapshots[0].ObjectMeta.Finalizers, utils.VolumeSnapshotBoundFinalizer) &&
						!slices.Contains(snapshot.ObjectMeta.Finalizers, utils.VolumeSnapshotBoundFinalizer) &&
						slices.Contains(test.initialSnapshots[0].ObjectMeta.Finalizers, utils.VolumeSnapshotAsSourceFinalizer) &&
						!slices.Contains(snapshot.ObjectMeta.Finalizers, utils.VolumeSnapshotAsSourceFinalizer) {

						klog.V(4).Infof("test %q succeeded. SnapshotFinalizer is removed from Snapshot %s", test.name, snapshot.Name)
						bHasSnapshotFinalizer = false
					}
					break
				}
			}
			if test.expectSuccess && bHasSnapshotFinalizer {
				t.Errorf("Test %q: failed to remove finalizer from snapshot %s", test.name, test.initialSnapshots[0].Name)
			}
		}
	}
}

// This tests that a snapshotclass is updated or not
func runUpdateSnapshotClassTests(t *testing.T, tests []controllerTest, snapshotClasses []*crdv1.VolumeSnapshotClass) {
	snapshotscheme.AddToScheme(scheme.Scheme)
	for _, test := range tests {
		klog.V(4).Infof("starting test %q", test.name)

		// Initialize the controller
		kubeClient := &kubefake.Clientset{}
		client := &fake.Clientset{}

		ctrl, err := newTestController(kubeClient, client, nil, t, test)
		if err != nil {
			t.Fatalf("Test %q construct test controller failed: %v", test.name, err)
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

		pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		for _, claim := range test.initialClaims {
			reactor.claims[claim.Name] = claim
			pvcIndexer.Add(claim)
		}
		ctrl.pvcLister = corelisters.NewPersistentVolumeClaimLister(pvcIndexer)

		for _, volume := range test.initialVolumes {
			reactor.volumes[volume.Name] = volume
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
		err = test.test(ctrl, reactor, test)
		if err != nil && isTestError(err) {
			t.Errorf("Test %q failed: %v", test.name, err)
		}
		if test.expectSuccess && err != nil {
			t.Errorf("Test %q failed: %v", test.name, err)
		}

		// Verify UpdateSnapshotClass tests results
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
