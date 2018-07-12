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
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	"k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1beta1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	ref "k8s.io/client-go/tools/reference"
	"k8s.io/kubernetes/pkg/util/goroutinemap"
	"k8s.io/kubernetes/pkg/util/goroutinemap/exponentialbackoff"
)

// ==================================================================
// PLEASE DO NOT ATTEMPT TO SIMPLIFY THIS CODE.
// KEEP THE SPACE SHUTTLE FLYING.
// ==================================================================

// Design:
//
// The fundamental key to this design is the bi-directional "pointer" between
// VolumeSnapshots and VolumeSnapshotContents, which is represented here
// as snapshot.Spec.SnapshotContentName and content.Spec.VolumeSnapshotRef.
// The bi-directionality is complicated to manage in a transactionless system, but
// without it we can't ensure sane behavior in the face of different forms of
// trouble.  For example, a rogue HA controller instance could end up racing
// and making multiple bindings that are indistinguishable, resulting in
// potential data loss.
//
// This controller is designed to work in active-passive high availability
// mode. It *could* work also in active-active HA mode, all the object
// transitions are designed to cope with this, however performance could be
// lower as these two active controllers will step on each other toes
// frequently.
//
// This controller supports both dynamic snapshot creation and pre-bound snapshot.
// In pre-bound mode, objects are created with pre-defined pointers: a VolumeSnapshot
// points to a specific VolumeSnapshotContent and the VolumeSnapshotContent also
// points back for this VolumeSnapshot.
//
// The dynamic snapshot creation is multi-step process: first controller triggers
// snapshot creation though csi volume plugin which should return a snapshot after
// it is created succesfully (however, the snapshot might not be ready to use yet if
// there is an uploading phase). The creationTimestamp will be updated according to
// VolumeSnapshot, and then a VolumeSnapshotContent object is created to represent
// this snapshot. After that, the controller will keep checking the snapshot status
// though csi snapshot calls. When the snapshot is ready to use, the controller set
// the status "Bound" to true to indicate the snapshot is bound and ready to use.
// If the createtion failed for any reason, the Error status is set accordingly.
// In alpha version, the controller not retry to create the snapshot after it failed.
// In the future version, a retry policy will be added.

const pvcKind = "PersistentVolumeClaim"

const IsDefaultSnapshotClassAnnotation = "snapshot.storage.kubernetes.io/is-default-class"

// syncContent deals with one key off the queue.  It returns false when it's time to quit.
func (ctrl *CSISnapshotController) syncContent(content *crdv1.VolumeSnapshotContent) error {
	glog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]", content.Name)

	// VolumeSnapshotContent is not bind to any VolumeSnapshot, this case rare and we just return err
	if content.Spec.VolumeSnapshotRef == nil {
		// content is not bind
		glog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: VolumeSnapshotContent is not bound to any VolumeSnapshot", content.Name)
		return fmt.Errorf("volumeSnapshotContent %s is not bound to any VolumeSnapshot", content.Name)
	} else {
		glog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: content is bound to snapshot %s", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
		// The VolumeSnapshotContent is reserved for a VolumeSNapshot;
		// that VolumeSnapshot has not yet been bound to this VolumeSnapshotCent; the VolumeSnapshot sync will handle it.
		if content.Spec.VolumeSnapshotRef.UID == "" {
			glog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: VolumeSnapshotContent is pre-bound to VolumeSnapshot %s", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
			return nil
		}
		// Get the VolumeSnapshot by _name_
		var vs *crdv1.VolumeSnapshot
		vsName := snapshotRefKey(content.Spec.VolumeSnapshotRef)
		obj, found, err := ctrl.snapshotStore.GetByKey(vsName)
		if err != nil {
			return err
		}
		if !found {
			glog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: vs %s not found", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
			// Fall through with vs = nil
		} else {
			var ok bool
			vs, ok = obj.(*crdv1.VolumeSnapshot)
			if !ok {
				return fmt.Errorf("cannot convert object from vs cache to vs %q!?: %#v", content.Name, obj)
			}
			glog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: vs %s found", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
		}
		if vs != nil && vs.UID != content.Spec.VolumeSnapshotRef.UID {
			// The vs that the content was pointing to was deleted, and another
			// with the same name created.
			glog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: content %s has different UID, the old one must have been deleted", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
			// Treat the volume as bound to a missing claim.
			vs = nil
		}
		if vs == nil {
			ctrl.deleteSnapshotContent(content)
		}
	}
	return nil
}

// syncSnapshot is the main controller method to decide what to do with a snapshot.
// It's invoked by appropriate cache.Controller callbacks when a snapshot is
// created, updated or periodically synced. We do not differentiate between
// these events.
// For easier readability, it is split into syncCompleteSnapshot and syncUncompleteSnapshot
func (ctrl *CSISnapshotController) syncSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	glog.V(4).Infof("synchonizing VolumeSnapshot[%s]: %s", snapshotKey(snapshot), getSnapshotStatusForLogging(snapshot))

	if !snapshot.Status.Ready {
		return ctrl.syncUnboundSnapshot(snapshot)
	} else {
		return ctrl.syncBoundSnapshot(snapshot)
	}
}

// syncCompleteSnapshot checks the snapshot which has been bound to snapshot content succesfully before.
// If there is any problem with the binding (e.g., snapshot points to a non-exist snapshot content), update the snapshot status and emit event.
func (ctrl *CSISnapshotController) syncBoundSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	if snapshot.Spec.SnapshotContentName == "" {
		if _, err := ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotLost", "Bound snapshot has lost reference to VolumeSnapshotContent"); err != nil {
			return err
		}
		return nil
	}
	obj, found, err := ctrl.contentStore.GetByKey(snapshot.Spec.SnapshotContentName)
	if err != nil {
		return err
	}
	if !found {
		if _, err = ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotLost", "Bound snapshot has lost reference to VolumeSnapshotContent"); err != nil {
			return err
		}
		return nil
	} else {
		content, ok := obj.(*crdv1.VolumeSnapshotContent)
		if !ok {
			return fmt.Errorf("Cannot convert object from snapshot content store to VolumeSnapshotContent %q!?: %#v", snapshot.Spec.SnapshotContentName, obj)
		}

		glog.V(4).Infof("syncCompleteSnapshot[%s]: VolumeSnapshotContent %q found", snapshotKey(snapshot), content.Name)
		if !IsSnapshotBound(snapshot, content) {
			// snapshot is bound but content is not bound to snapshot correctly
			if _, err = ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotMisbound", "VolumeSnapshotContent is not bound to the VolumeSnapshot correctly"); err != nil {
				return err
			}
			return nil
		}
		// Snapshot is correctly bound.
		if _, err = ctrl.updateSnapshotBoundWithEvent(snapshot, v1.EventTypeNormal, "SnapshotBound", "Snapshot is bound to its VolumeSnapshotContent"); err != nil {
			return err
		}
		return nil
	}
}

func (ctrl *CSISnapshotController) syncUnboundSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	uniqueSnapshotName := snapshotKey(snapshot)
	glog.V(4).Infof("syncSnapshot %s", uniqueSnapshotName)

	// Snsapshot has errors during its creation. Controller will not try to fix it. Nothing to do.
	if snapshot.Status.Error != nil {
		//return nil
	}

	if snapshot.Spec.SnapshotContentName != "" {
		contentObj, found, err := ctrl.contentStore.GetByKey(snapshot.Spec.SnapshotContentName)
		if err != nil {
			return err
		}
		if !found {
			// snapshot is bound to a non-existing content.
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotLost", fmt.Sprintf("Snapshot has lost reference to VolumeSnapshotContent, %v", err))
			return fmt.Errorf("snapshot %s is bound to a non-existing content %s", uniqueSnapshotName, snapshot.Spec.SnapshotContentName)
		}
		content, ok := contentObj.(*crdv1.VolumeSnapshotContent)
		if !ok {
			return fmt.Errorf("expected volume snapshot content, got %+v", contentObj)
		}

		if err := ctrl.bindSnapshotContent(snapshot, content); err != nil {
			// snapshot is bound but content is not bound to snapshot correctly
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotBoundFailed", fmt.Sprintf("Snapshot failed to bind VolumeSnapshotContent, %v", err))
			return fmt.Errorf("snapshot %s is bound, but VolumeSnapshotContent %s is not bound to the VolumeSnapshot correctly, %v", uniqueSnapshotName, content.Name, err)
		}

		// snapshot is already bound correctly, check the status and update if it is ready.
		glog.V(4).Infof("Check and update snapshot %s status", uniqueSnapshotName)
		if err = ctrl.checkandUpdateSnapshotStatus(snapshot, content); err != nil {
			return err
		}
		return nil
	} else { // snapshot.Spec.SnapshotContentName == nil
		if ContentObj := ctrl.getMatchSnapshotContent(snapshot); ContentObj != nil {
			glog.V(4).Infof("Find VolumeSnapshotContent object %s for snapshot %s", ContentObj.Name, uniqueSnapshotName)
			newSnapshot, err := ctrl.bindandUpdateVolumeSnapshot(ContentObj, snapshot)
			if err != nil {
				return err
			}
			glog.V(4).Infof("bindandUpdateVolumeSnapshot %v", newSnapshot)
			return nil
		} else if snapshot.Status.Error == nil { // Try to create snapshot if no error status is set
			if err := ctrl.createSnapshot(snapshot); err != nil {
				ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotCreationFailed", fmt.Sprintf("Failed to create snapshot with error %v", err))
				return err
			}
			return nil
		}
		return nil
	}
}

// getMatchSnapshotContent looks up VolumeSnapshotContent for a VolumeSnapshot named snapshotName
func (ctrl *CSISnapshotController) getMatchSnapshotContent(vs *crdv1.VolumeSnapshot) *crdv1.VolumeSnapshotContent {
	var snapshotDataObj *crdv1.VolumeSnapshotContent
	var found bool

	objs := ctrl.contentStore.List()
	for _, obj := range objs {
		content := obj.(*crdv1.VolumeSnapshotContent)
		if content.Spec.VolumeSnapshotRef != nil &&
			content.Spec.VolumeSnapshotRef.Name == vs.Name &&
			content.Spec.VolumeSnapshotRef.Namespace == vs.Namespace &&
			content.Spec.VolumeSnapshotRef.UID == vs.UID {
			found = true
			snapshotDataObj = content
			break
		}
	}

	if !found {
		glog.V(4).Infof("No VolumeSnapshotContent for VolumeSnapshot %s found", snapshotKey(vs))
		return nil
	}

	return snapshotDataObj
}

// deleteSnapshotContent starts delete action.
func (ctrl *CSISnapshotController) deleteSnapshotContent(content *crdv1.VolumeSnapshotContent) {
	operationName := fmt.Sprintf("delete-%s[%s]", content.Name, string(content.UID))
	glog.V(4).Infof("Snapshotter is about to delete volume snapshot and the operation named %s", operationName)
	ctrl.scheduleOperation(operationName, func() error {
		return ctrl.DeleteSnapshotContentOperation(content)
	})
}

// scheduleOperation starts given asynchronous operation on given volume. It
// makes sure the operation is already not running.
func (ctrl *CSISnapshotController) scheduleOperation(operationName string, operation func() error) {
	glog.V(4).Infof("scheduleOperation[%s]", operationName)

	err := ctrl.runningOperations.Run(operationName, operation)
	if err != nil {
		switch {
		case goroutinemap.IsAlreadyExists(err):
			glog.V(4).Infof("operation %q is already running, skipping", operationName)
		case exponentialbackoff.IsExponentialBackoff(err):
			glog.V(4).Infof("operation %q postponed due to exponential backoff", operationName)
		default:
			glog.Errorf("error scheduling operation %q: %v", operationName, err)
		}
	}
}

func (ctrl *CSISnapshotController) storeSnapshotUpdate(vs interface{}) (bool, error) {
	return storeObjectUpdate(ctrl.snapshotStore, vs, "vs")
}

func (ctrl *CSISnapshotController) storeContentUpdate(content interface{}) (bool, error) {
	return storeObjectUpdate(ctrl.contentStore, content, "content")
}

// createSnapshot starts new asynchronous operation to create snapshot data for snapshot
func (ctrl *CSISnapshotController) createSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	glog.V(4).Infof("createSnapshot[%s]: started", snapshotKey(snapshot))
	opName := fmt.Sprintf("create-%s[%s]", snapshotKey(snapshot), string(snapshot.UID))
	ctrl.scheduleOperation(opName, func() error {
		snapshotObj, err := ctrl.createSnapshotOperation(snapshot)
		if err != nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotCreationFailed", fmt.Sprintf("Failed to create snapshot: %v", err))
			glog.Errorf("createSnapshot [%s]: error occurred in createSnapshotOperation: %v", opName, err)
			return err
		}
		_, updateErr := ctrl.storeSnapshotUpdate(snapshotObj)
		if updateErr != nil {
			// We will get an "snapshot update" event soon, this is not a big error
			glog.V(4).Infof("createSnapshot [%s]: cannot update internal cache: %v", snapshotKey(snapshotObj), updateErr)
		}

		return nil
	})
	return nil
}

func (ctrl *CSISnapshotController) checkandUpdateSnapshotStatus(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) error {
	glog.V(5).Infof("checkandUpdateSnapshotStatus[%s] started", snapshotKey(snapshot))
	opName := fmt.Sprintf("check-%s[%s]", snapshotKey(snapshot), string(snapshot.UID))
	ctrl.scheduleOperation(opName, func() error {
		snapshotObj, err := ctrl.checkandUpdateSnapshotStatusOperation(snapshot, content)
		if err != nil {
			glog.Errorf("checkandUpdateSnapshotStatus [%s]: error occured %v", snapshotKey(snapshot), err)
			return err
		}
		_, updateErr := ctrl.storeSnapshotUpdate(snapshotObj)
		if updateErr != nil {
			// We will get an "snapshot update" event soon, this is not a big error
			glog.V(4).Infof("checkandUpdateSnapshotStatus [%s]: cannot update internal cache: %v", snapshotKey(snapshotObj), updateErr)
		}

		return nil
	})
	return nil
}

// updateSnapshotStatusWithEvent saves new snapshot.Status to API server and emits
// given event on the snapshot. It saves the status and emits the event only when
// the status has actually changed from the version saved in API server.
// Parameters:
//   snapshot - snapshot to update
//   eventtype, reason, message - event to send, see EventRecorder.Event()
func (ctrl *CSISnapshotController) updateSnapshotErrorStatusWithEvent(snapshot *crdv1.VolumeSnapshot, eventtype, reason, message string) (*crdv1.VolumeSnapshot, error) {
	glog.V(4).Infof("updateSnapshotStatusWithEvent[%s]", snapshotKey(snapshot))

	if snapshot.Status.Error != nil && snapshot.Status.Ready == false {
		glog.V(4).Infof("updateClaimStatusWithEvent[%s]: error %v already set", snapshot.Name, snapshot.Status.Error)
		return snapshot, nil
	}
	snapshotClone := snapshot.DeepCopy()
	if snapshot.Status.Error == nil {
		statusError := &storage.VolumeError{
			Time: metav1.Time{
				Time: time.Now(),
			},
			Message: message,
		}
		snapshotClone.Status.Error = statusError
	}
	snapshotClone.Status.Ready = false
	newSnapshot, err := ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
	if err != nil {
		glog.V(4).Infof("updating VolumeSnapshot[%s] error status failed %v", snapshotKey(snapshot), err)
		return newSnapshot, err
	}

	_, err = ctrl.storeSnapshotUpdate(newSnapshot)
	if err != nil {
		glog.V(4).Infof("updating VolumeSnapshot[%s] error status: cannot update internal cache %v", snapshotKey(snapshot), err)
		return newSnapshot, err
	}
	// Emit the event only when the status change happens
	ctrl.eventRecorder.Event(newSnapshot, eventtype, reason, message)

	return newSnapshot, nil
}

func (ctrl *CSISnapshotController) updateSnapshotBoundWithEvent(snapshot *crdv1.VolumeSnapshot, eventtype, reason, message string) (*crdv1.VolumeSnapshot, error) {
	glog.V(4).Infof("updateSnapshotBoundWithEvent[%s]", snapshotKey(snapshot))
	if snapshot.Status.Ready && snapshot.Status.Error == nil {
		// Nothing to do.
		glog.V(4).Infof("updateSnapshotBoundWithEvent[%s]: Ready %v already set", snapshotKey(snapshot), snapshot.Status.Ready)
		return snapshot, nil
	}

	snapshotClone := snapshot.DeepCopy()
	snapshotClone.Status.Ready = true
	snapshotClone.Status.Error = nil
	newSnapshot, err := ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
	if err != nil {
		glog.V(4).Infof("updating VolumeSnapshot[%s] error status failed %v", snapshotKey(snapshot), err)
		return newSnapshot, err
	}
	// Emit the event only when the status change happens
	ctrl.eventRecorder.Event(snapshot, eventtype, reason, message)

	_, err = ctrl.storeSnapshotUpdate(newSnapshot)
	if err != nil {
		glog.V(4).Infof("updating VolumeSnapshot[%s] error status: cannot update internal cache %v", snapshotKey(snapshot), err)
		return newSnapshot, err
	}

	return newSnapshot, nil
}

// Stateless functions
func getSnapshotStatusForLogging(snapshot *crdv1.VolumeSnapshot) string {
	return fmt.Sprintf("bound to: %q, Completed: %v", snapshot.Spec.SnapshotContentName, snapshot.Status.Ready)
}

func IsSnapshotBound(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) bool {
	if content.Spec.VolumeSnapshotRef != nil && content.Spec.VolumeSnapshotRef.Name == snapshot.Name &&
		content.Spec.VolumeSnapshotRef.UID == snapshot.UID {
		return true
	}
	return false
}

func (ctrl *CSISnapshotController) bindSnapshotContent(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) error {
	if content.Spec.VolumeSnapshotRef == nil || content.Spec.VolumeSnapshotRef.Name != snapshot.Name {
		return fmt.Errorf("Could not bind snapshot %s and content %s, the VolumeSnapshotRef does not match", snapshot.Name, content.Name)
	} else if content.Spec.VolumeSnapshotRef.UID != "" && content.Spec.VolumeSnapshotRef.UID != snapshot.UID {
		return fmt.Errorf("Could not bind snapshot %s and content %s, the VolumeSnapshotRef does not match", snapshot.Name, content.Name)
	} else if content.Spec.VolumeSnapshotRef.UID == "" {
		contentClone := content.DeepCopy()
		contentClone.Spec.VolumeSnapshotRef.UID = snapshot.UID
		newContent, err := ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshotContents().Update(contentClone)
		if err != nil {
			glog.V(4).Infof("updating VolumeSnapshotContent[%s] error status failed %v", newContent.Name, err)
			return err
		}
		_, err = ctrl.storeContentUpdate(newContent)
		if err != nil {
			glog.V(4).Infof("updating VolumeSnapshotContent[%s] error status: cannot update internal cache %v", newContent.Name, err)
			return err
		}
	}
	return nil
}

func (ctrl *CSISnapshotController) checkandUpdateSnapshotStatusOperation(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshot, error) {
	status, _, err := ctrl.handler.GetSnapshotStatus(content)
	if err != nil {
		return nil, fmt.Errorf("failed to check snapshot status %s with error %v", snapshot.Name, err)
	}

	newSnapshot, err := ctrl.updateSnapshotStatus(snapshot, status, time.Now(), IsSnapshotBound(snapshot, content))
	if err != nil {
		return nil, err
	}
	return newSnapshot, nil
}

// The function goes through the whole snapshot creation process.
// 1. Trigger the snapshot through csi storage provider.
// 2. Update VolumeSnapshot status with creationtimestamp information
// 3. Create the VolumeSnapshotContent object with the snapshot id information.
// 4. Bind the VolumeSnapshot and VolumeSnapshotContent object
func (ctrl *CSISnapshotController) createSnapshotOperation(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	glog.Infof("createSnapshot: Creating snapshot %s through the plugin ...", snapshotKey(snapshot))

	class, err := ctrl.GetClassFromVolumeSnapshot(snapshot)
	if err != nil {
		glog.Errorf("createSnapshotOperation failed to getClassFromVolumeSnapshot %s", err)
		return nil, err
	}
	volume, err := ctrl.getVolumeFromVolumeSnapshot(snapshot)
	if err != nil {
		glog.Errorf("createSnapshotOperation failed to get PersistentVolume object [%s]: Error: [%#v]", snapshot.Name, err)
		return nil, err
	}

	driverName, snapshotID, timestamp, csiSnapshotStatus, err := ctrl.handler.CreateSnapshot(snapshot, volume, class.Parameters)
	if err != nil {
		return nil, fmt.Errorf("Failed to take snapshot of the volume, %s: %q", volume.Name, err)
	}
	glog.Infof("Create snapshot driver %s, snapshotId %s, timestamp %d, csiSnapshotStatus %v", driverName, snapshotID, timestamp, csiSnapshotStatus)

	// Update snapshot status with timestamp
	newSnapshot, err := ctrl.updateSnapshotStatus(snapshot, csiSnapshotStatus, time.Unix(0, timestamp), false)
	if err != nil {
		return nil, err
	}

	// Create VolumeSnapshotContent in the database
	contentName := GetSnapshotContentNameForSnapshot(snapshot)
	volumeRef, err := ref.GetReference(scheme.Scheme, volume)

	snapshotContent := &crdv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentName,
		},
		Spec: crdv1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: &v1.ObjectReference{
				Kind:       "VolumeSnapshot",
				Namespace:  snapshot.Namespace,
				Name:       snapshot.Name,
				UID:        snapshot.UID,
				APIVersion: "v1alpha1",
			},
			PersistentVolumeRef: volumeRef,
			VolumeSnapshotSource: crdv1.VolumeSnapshotSource{
				CSI: &crdv1.CSIVolumeSnapshotSource{
					Driver:         driverName,
					SnapshotHandle: snapshotID,
					CreatedAt:      timestamp,
				},
			},
		},
	}

	// Try to create the VolumeSnapshotContent object several times
	for i := 0; i < ctrl.createSnapshotContentRetryCount; i++ {
		glog.V(4).Infof("createSnapshot [%s]: trying to save volume snapshot data %s", snapshotKey(snapshot), snapshotContent.Name)
		if _, err = ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshotContents().Create(snapshotContent); err == nil || apierrs.IsAlreadyExists(err) {
			// Save succeeded.
			if err != nil {
				glog.V(3).Infof("volume snapshot data %q for snapshot %q already exists, reusing", snapshotContent.Name, snapshotKey(snapshot))
				err = nil
			} else {
				glog.V(3).Infof("volume snapshot data %q for snapshot %q saved", snapshotContent.Name, snapshotKey(snapshot))
			}
			break
		}
		// Save failed, try again after a while.
		glog.V(3).Infof("failed to save volume snapshot data %q for snapshot %q: %v", snapshotContent.Name, snapshotKey(snapshot), err)
		time.Sleep(ctrl.createSnapshotContentInterval)
	}

	if err != nil {
		// Save failed. Now we have a storage asset outside of Kubernetes,
		// but we don't have appropriate volumesnapshotdata object for it.
		// Emit some event here and try to delete the storage asset several
		// times.
		strerr := fmt.Sprintf("Error creating volume snapshot data object for snapshot %s: %v.", snapshotKey(snapshot), err)
		glog.Error(strerr)
		ctrl.eventRecorder.Event(newSnapshot, v1.EventTypeWarning, "CreateSnapshotContentFailed", strerr)
		return nil, err
	}

	// save succeeded, bind and update status for snapshot.
	result, err := ctrl.bindandUpdateVolumeSnapshot(snapshotContent, newSnapshot)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Delete a snapshot
// 1. Find the SnapshotContent corresponding to Snapshot
//   1a: Not found => finish (it's been deleted already)
// 2. Ask the backend to remove the snapshot device
// 3. Delete the SnapshotContent object
// 4. Remove the Snapshot from vsStore
// 5. Finish
func (ctrl *CSISnapshotController) DeleteSnapshotContentOperation(content *crdv1.VolumeSnapshotContent) error {
	glog.V(4).Infof("deleteSnapshotOperation [%s] started", content.Name)

	err := ctrl.handler.DeleteSnapshot(content)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot %#v, err: %v", content.Name, err)
	}

	err = ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshotContents().Delete(content.Name, &metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete VolumeSnapshotContent %s from API server: %q", content.Name, err)
	}

	return nil
}

func (ctrl *CSISnapshotController) bindandUpdateVolumeSnapshot(snapshotContent *crdv1.VolumeSnapshotContent, snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	glog.V(4).Infof("bindandUpdateVolumeSnapshot for snapshot [%s]: snapshotContent [%s]", snapshot.Name, snapshotContent.Name)
	snapshotObj, err := ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshots(snapshot.Namespace).Get(snapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot %s from api server: %v", snapshotKey(snapshot), err)
	}

	// Copy the snapshot object before updating it
	snapshotCopy := snapshotObj.DeepCopy()

	if snapshotObj.Spec.SnapshotContentName == snapshotContent.Name {
		glog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: VolumeSnapshot %s already bind to volumeSnapshotContent [%s]", snapshot.Name, snapshotContent.Name)
	} else {
		glog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: before bind VolumeSnapshot %s to volumeSnapshotContent [%s]", snapshot.Name, snapshotContent.Name)
		snapshotCopy.Spec.SnapshotContentName = snapshotContent.Name
		updateSnapshot, err := ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshots(snapshot.Namespace).Update(snapshotCopy)
		if err != nil {
			glog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: Error binding VolumeSnapshot %s to volumeSnapshotContent [%s]. Error [%#v]", snapshot.Name, snapshotContent.Name, err)
			return nil, fmt.Errorf("error updating snapshot object %s on the API server: %v", snapshotKey(updateSnapshot), err)
		}
		snapshotCopy = updateSnapshot
		_, err = ctrl.storeSnapshotUpdate(snapshotCopy)
		if err != nil {
			glog.Errorf("%v", err)
		}
	}

	glog.V(5).Infof("bindandUpdateVolumeSnapshot for snapshot completed [%#v]", snapshotCopy)
	return snapshotCopy, nil
}

// UpdateSnapshotStatus converts snapshot status to crdv1.VolumeSnapshotCondition
func (ctrl *CSISnapshotController) updateSnapshotStatus(snapshot *crdv1.VolumeSnapshot, csistatus *csi.SnapshotStatus, timestamp time.Time, bound bool) (*crdv1.VolumeSnapshot, error) {
	glog.V(4).Infof("updating VolumeSnapshot[]%s, set status %v, timestamp %v", snapshotKey(snapshot), csistatus, timestamp)
	status := snapshot.Status
	change := false
	timeAt := &metav1.Time{
		Time: timestamp,
	}

	snapshotClone := snapshot.DeepCopy()
	switch csistatus.Type {
	case csi.SnapshotStatus_READY:
		if bound {
			status.Ready = true
			change = true
		}
		if status.CreationTime == nil {
			status.CreationTime = timeAt
			change = true
		}
	case csi.SnapshotStatus_ERROR_UPLOADING:
		if status.Error == nil {
			status.Error = &storage.VolumeError{
				Time:    *timeAt,
				Message: "Failed to upload the snapshot",
			}
			change = true
		}
	case csi.SnapshotStatus_UPLOADING:
		if status.CreationTime == nil {
			status.CreationTime = timeAt
			change = true
		}
	}
	if change {
		snapshotClone.Status = status
		newSnapshotObj, err := ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
		if err != nil {
			return nil, fmt.Errorf("error update status for volume snapshot %s: %s", snapshotKey(snapshot), err)
		} else {
			return newSnapshotObj, nil
		}
	}
	return snapshot, nil
}

// getVolumeFromVolumeSnapshot is a helper function to get PV from VolumeSnapshot.
func (ctrl *CSISnapshotController) getVolumeFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolume, error) {
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	pvName := pvc.Spec.VolumeName
	pv, err := ctrl.client.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PV %s from the API server: %q", pvName, err)
	}

	glog.V(5).Infof("getVolumeFromVolumeSnapshot: snapshot [%s] PV name [%s]", snapshot.Name, pvName)

	return pv, nil
}

// GetClassFromVolumeSnapshot is a helper function to get storage class from VolumeSnapshot.
func (ctrl *CSISnapshotController) GetClassFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotClass, error) {
	className := snapshot.Spec.VolumeSnapshotClassName
	glog.V(5).Infof("getClassFromVolumeSnapshot [%s]: VolumeSnapshotClassName [%s]", snapshot.Name, className)
	if len(className) > 0 {
		class, err := ctrl.clientset.VolumesnapshotV1alpha1().VolumeSnapshotClasses().Get(className, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("failed to retrieve storage class %s from the API server: %q", className, err)
			//return nil, fmt.Errorf("failed to retrieve storage class %s from the API server: %q", className, err)
		}
		glog.V(5).Infof("getClassFromVolumeSnapshot [%s]: VolumeSnapshotClassName [%s]", snapshot.Name, className)
		return class, nil
	} else {
		// Find default snapshot class if available
		list, err := ctrl.classLister.List(labels.Everything())
		if err != nil {
			return nil, err
		}
		pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
		if err != nil {
			return nil, err
		}
		storageclass, err := ctrl.client.StorageV1().StorageClasses().Get(*pvc.Spec.StorageClassName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		defaultClasses := []*crdv1.VolumeSnapshotClass{}

		for _, class := range list {
			if IsDefaultAnnotation(class.ObjectMeta) && storageclass.Provisioner == class.Snapshotter {
				defaultClasses = append(defaultClasses, class)
				glog.V(4).Infof("getDefaultClass added: %s", class.Name)
			}
		}

		if len(defaultClasses) == 0 {
			return nil, nil
		}
		if len(defaultClasses) > 1 {
			glog.V(4).Infof("getDefaultClass %d defaults found", len(defaultClasses))
			return nil, fmt.Errorf("%d default StorageClasses were found", len(defaultClasses))
		}
		glog.V(5).Infof("getClassFromVolumeSnapshot [%s]: default VolumeSnapshotClassName [%s]", snapshot.Name, defaultClasses[0])
		return defaultClasses[0], nil

	}
}

// getClaimFromVolumeSnapshot is a helper function to get PV from VolumeSnapshot.
func (ctrl *CSISnapshotController) getClaimFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolumeClaim, error) {
	if snapshot.Spec.Source == nil || snapshot.Spec.Source.Kind != pvcKind {
		return nil, fmt.Errorf("The snapshot source is not the right type. Expected %s, Got %v", pvcKind, snapshot.Spec.Source)
	}
	pvcName := snapshot.Spec.Source.Name
	if pvcName == "" {
		return nil, fmt.Errorf("the PVC name is not specified in snapshot %s", snapshotKey(snapshot))
	}

	pvc, err := ctrl.client.CoreV1().PersistentVolumeClaims(snapshot.Namespace).Get(pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PVC %s from the API server: %q", pvcName, err)
	}
	if pvc.Status.Phase != v1.ClaimBound {
		return nil, fmt.Errorf("the PVC %s not yet bound to a PV, will not attempt to take a snapshot yet", pvcName)
	}

	return pvc, nil
}
