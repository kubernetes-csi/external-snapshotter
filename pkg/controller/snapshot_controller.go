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
	"strings"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1beta1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	ref "k8s.io/client-go/tools/reference"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/goroutinemap"
	"k8s.io/kubernetes/pkg/util/goroutinemap/exponentialbackoff"
	"k8s.io/kubernetes/pkg/util/slice"
)

// ==================================================================
// PLEASE DO NOT ATTEMPT TO SIMPLIFY THIS CODE.
// KEEP THE SPACE SHUTTLE FLYING.
// ==================================================================

// Design:
//
// The fundamental key to this design is the bi-directional "pointer" between
// VolumeSnapshots and VolumeSnapshotContents, which is represented here
// as snapshot.Spec.Source.VolumeSnapshotContentName and content.Spec.VolumeSnapshotRef.
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
// it is created successfully (however, the snapshot might not be ready to use yet if
// there is an uploading phase). The creationTimestamp will be updated according to
// VolumeSnapshot, and then a VolumeSnapshotContent object is created to represent
// this snapshot. After that, the controller will keep checking the snapshot status
// though csi snapshot calls. When the snapshot is ready to use, the controller set
// the status "Bound" to true to indicate the snapshot is bound and ready to use.
// If the createtion failed for any reason, the Error status is set accordingly.
// In alpha version, the controller not retry to create the snapshot after it failed.
// In the future version, a retry policy will be added.

const pvcKind = "PersistentVolumeClaim"
const apiGroup = ""
const snapshotKind = "VolumeSnapshot"
const snapshotAPIGroup = crdv1.GroupName

const controllerUpdateFailMsg = "snapshot controller failed to update"

const IsDefaultSnapshotClassAnnotation = "snapshot.storage.kubernetes.io/is-default-class"

// syncContent deals with one key off the queue.  It returns false when it's time to quit.
func (ctrl *csiSnapshotController) syncContent(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("synchronizing VolumeSnapshotContent[%s]", content.Name)

	if isContentDeletionCandidate(content) {
		// Volume snapshot content should be deleted. Check if it's used
		// and remove finalizer if it's not.
		// Check if snapshot content is still bound to a snapshot.
		isUsed := ctrl.isSnapshotContentBeingUsed(content)
		if !isUsed {
			klog.V(5).Infof("syncContent: Remove Finalizer for VolumeSnapshotContent[%s]", content.Name)
			return ctrl.removeContentFinalizer(content)
		}
	}

	if needToAddContentFinalizer(content) {
		// Content is not being deleted -> it should have the finalizer.
		klog.V(5).Infof("syncContent: Add Finalizer for VolumeSnapshotContent[%s]", content.Name)
		return ctrl.addContentFinalizer(content)
	}

	// VolumeSnapshotContent is not bound to any VolumeSnapshot, in this case we just return err
	if content.Spec.VolumeSnapshotRef.Name == "" {
		// content is not bound
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: VolumeSnapshotContent is not bound to any VolumeSnapshot", content.Name)
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotContentNotBound", "VolumeSnapshotContent is not bound to any VolumeSnapshot")
		return fmt.Errorf("volumeSnapshotContent %s is not bound to any VolumeSnapshot", content.Name)
	}
	klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: content is bound to snapshot %s", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
	// The VolumeSnapshotContent is reserved for a VolumeSnapshot;
	// that VolumeSnapshot has not yet been bound to this VolumeSnapshotContent; the VolumeSnapshot sync will handle it.
	if content.Spec.VolumeSnapshotRef.UID == "" {
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: VolumeSnapshotContent is pre-bound to VolumeSnapshot %s", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
		return nil
	}
	// Get the VolumeSnapshot by _name_
	var snapshot *crdv1.VolumeSnapshot
	snapshotName := snapshotRefKey(content.Spec.VolumeSnapshotRef)
	obj, found, err := ctrl.snapshotStore.GetByKey(snapshotName)
	if err != nil {
		return err
	}
	if !found {
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: snapshot %s not found", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
		// Fall through with snapshot = nil
	} else {
		var ok bool
		snapshot, ok = obj.(*crdv1.VolumeSnapshot)
		if !ok {
			return fmt.Errorf("cannot convert object from snapshot cache to snapshot %q!?: %#v", content.Name, obj)
		}
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: snapshot %s found", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
	}
	if snapshot != nil && snapshot.UID != content.Spec.VolumeSnapshotRef.UID {
		// The snapshot that the content was pointing to was deleted, and another
		// with the same name created.
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: content %s has different UID, the old one must have been deleted", content.Name, snapshotRefKey(content.Spec.VolumeSnapshotRef))
		// Treat the content as bound to a missing snapshot.
		snapshot = nil
	}
	if snapshot == nil {
		switch content.Spec.DeletionPolicy {
		case crdv1.VolumeSnapshotContentRetain:
			klog.V(4).Infof("VolumeSnapshotContent[%s]: policy is Retain, nothing to do", content.Name)

		case crdv1.VolumeSnapshotContentDelete:
			klog.V(4).Infof("VolumeSnapshotContent[%s]: policy is Delete", content.Name)
			ctrl.deleteSnapshotContent(content)
		default:
			// Unknown VolumeSnapshotDeletionolicy
			ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotUnknownDeletionPolicy", "Volume Snapshot Content has unrecognized deletion policy")
		}
		return nil
		// By default, we use Retain policy if it is not set by users
		klog.V(4).Infof("VolumeSnapshotContent[%s]: by default the policy is Retain", content.Name)

	}
	return nil
}

// syncSnapshot is the main controller method to decide what to do with a snapshot.
// It's invoked by appropriate cache.Controller callbacks when a snapshot is
// created, updated or periodically synced. We do not differentiate between
// these events.
// For easier readability, it is split into syncUnreadySnapshot and syncReadySnapshot
func (ctrl *csiSnapshotController) syncSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	klog.V(5).Infof("synchonizing VolumeSnapshot[%s]: %s", snapshotKey(snapshot), getSnapshotStatusForLogging(snapshot))

	if isSnapshotDeletionCandidate(snapshot) {
		// Volume snapshot should be deleted. Check if it's used
		// and remove finalizer if it's not.
		// Check if a volume is being created from snapshot.
		isUsed := ctrl.isVolumeBeingCreatedFromSnapshot(snapshot)
		if !isUsed {
			klog.V(5).Infof("syncSnapshot: Remove Finalizer for VolumeSnapshot[%s]", snapshotKey(snapshot))
			return ctrl.removeSnapshotFinalizer(snapshot)
		}
	}

	if needToAddSnapshotFinalizer(snapshot) {
		// Snapshot is not being deleted -> it should have the finalizer.
		klog.V(5).Infof("syncSnapshot: Add Finalizer for VolumeSnapshot[%s]", snapshotKey(snapshot))
		return ctrl.addSnapshotFinalizer(snapshot)
	}

	klog.V(5).Infof("syncSnapshot[%s]: check if we should remove finalizer on snapshot source and remove it if we can", snapshotKey(snapshot))
	// Check if we should remove finalizer on snapshot source and remove it if we can.
	errFinalizer := ctrl.checkandRemoveSnapshotSourceFinalizer(snapshot)
	if errFinalizer != nil {
		klog.Errorf("error check and remove snapshot source finalizer for snapshot [%s]: %v", snapshot.Name, errFinalizer)
		// Log an event and keep the original error from syncUnready/ReadySnapshot
		ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "ErrorSnapshotSourceFinalizer", "Error check and remove PVC Finalizer for VolumeSnapshot")
	}

	if snapshot.Status.ReadyToUse == nil || !*snapshot.Status.ReadyToUse {
		return ctrl.syncUnreadySnapshot(snapshot)
	}
	return ctrl.syncReadySnapshot(snapshot)
}

// syncReadySnapshot checks the snapshot which has been bound to snapshot content successfully before.
// If there is any problem with the binding (e.g., snapshot points to a non-exist snapshot content), update the snapshot status and emit event.
func (ctrl *csiSnapshotController) syncReadySnapshot(snapshot *crdv1.VolumeSnapshot) error {
	if snapshot.Status.BoundVolumeSnapshotContentName == nil {
		if err := ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotLost", "Bound snapshot has lost reference to VolumeSnapshotContent"); err != nil {
			return err
		}
		return nil
	}
	obj, found, err := ctrl.contentStore.GetByKey(*snapshot.Status.BoundVolumeSnapshotContentName)
	if err != nil {
		return err
	}
	if !found {
		if err = ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMissing", "VolumeSnapshotContent is missing"); err != nil {
			return err
		}
		return nil
	} else {
		content, ok := obj.(*crdv1.VolumeSnapshotContent)
		if !ok {
			return fmt.Errorf("Cannot convert object from snapshot content store to VolumeSnapshotContent %q!?: %#v", *snapshot.Status.BoundVolumeSnapshotContentName, obj)
		}

		klog.V(5).Infof("syncReadySnapshot[%s]: VolumeSnapshotContent %q found", snapshotKey(snapshot), content.Name)
		if !IsSnapshotBound(snapshot, content) {
			// snapshot is bound but content is not bound to snapshot correctly
			if err = ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotMisbound", "VolumeSnapshotContent is not bound to the VolumeSnapshot correctly"); err != nil {
				return err
			}
			return nil
		}
		// Snapshot is correctly bound.
		return nil
	}
}

// syncUnreadySnapshot is the main controller method to decide what to do with a snapshot which is not set to ready.
func (ctrl *csiSnapshotController) syncUnreadySnapshot(snapshot *crdv1.VolumeSnapshot) error {
	uniqueSnapshotName := snapshotKey(snapshot)
	klog.V(5).Infof("syncUnreadySnapshot %s", uniqueSnapshotName)

	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		contentObj, found, err := ctrl.contentStore.GetByKey(*snapshot.Spec.Source.VolumeSnapshotContentName)
		if err != nil {
			return err
		}
		if !found {
			// snapshot is bound to a non-existing content.
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMissing", "VolumeSnapshotContent is missing")
			klog.V(4).Infof("synchronizing unready snapshot[%s]: snapshotcontent %q requested and not found, will try again next time", uniqueSnapshotName, *snapshot.Spec.Source.VolumeSnapshotContentName)
			return fmt.Errorf("snapshot %s is bound to a non-existing content %s", uniqueSnapshotName, *snapshot.Spec.Source.VolumeSnapshotContentName)
		}
		content, ok := contentObj.(*crdv1.VolumeSnapshotContent)
		if !ok {
			return fmt.Errorf("expected volume snapshot content, got %+v", contentObj)
		}
		contentBound, err := ctrl.checkandBindSnapshotContent(snapshot, content)
		if err != nil {
			// snapshot is bound but content is not bound to snapshot correctly
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotBindFailed", fmt.Sprintf("Snapshot failed to bind VolumeSnapshotContent, %v", err))
			return fmt.Errorf("snapshot %s is bound, but VolumeSnapshotContent %s is not bound to the VolumeSnapshot correctly, %v", uniqueSnapshotName, content.Name, err)
		}
		// snapshot is already bound correctly, check the status and update if it is ready.
		klog.V(5).Infof("Check and update snapshot %s status", uniqueSnapshotName)
		if err = ctrl.checkandUpdateBoundSnapshotStatus(snapshot, contentBound); err != nil {
			return err
		}
		return nil
	} else { // snapshot.Source.Spec.VolumeSnapshotContentName == nil
		if contentObj := ctrl.getMatchSnapshotContent(snapshot); contentObj != nil {
			klog.V(5).Infof("Find VolumeSnapshotContent object %s for snapshot %s", contentObj.Name, uniqueSnapshotName)
			newSnapshot, err := ctrl.bindandUpdateVolumeSnapshot(contentObj, snapshot)
			if err != nil {
				return err
			}
			klog.V(5).Infof("bindandUpdateVolumeSnapshot %v", newSnapshot)
			return nil
		} else if snapshot.Status.Error == nil || isControllerUpdateFailError(snapshot.Status.Error) { // Try to create snapshot if no error status is set
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
func (ctrl *csiSnapshotController) getMatchSnapshotContent(snapshot *crdv1.VolumeSnapshot) *crdv1.VolumeSnapshotContent {
	var snapshotContentObj *crdv1.VolumeSnapshotContent
	var found bool

	objs := ctrl.contentStore.List()
	for _, obj := range objs {
		content := obj.(*crdv1.VolumeSnapshotContent)
		if content.Spec.VolumeSnapshotRef.Name == snapshot.Name &&
			content.Spec.VolumeSnapshotRef.Namespace == snapshot.Namespace &&
			content.Spec.VolumeSnapshotRef.UID == snapshot.UID {
			found = true
			snapshotContentObj = content
			break
		}
	}

	if !found {
		klog.V(4).Infof("No VolumeSnapshotContent for VolumeSnapshot %s found", snapshotKey(snapshot))
		return nil
	}

	return snapshotContentObj
}

// deleteSnapshotContent starts delete action.
func (ctrl *csiSnapshotController) deleteSnapshotContent(content *crdv1.VolumeSnapshotContent) {
	operationName := fmt.Sprintf("delete-%s[%s]", content.Name, string(content.UID))
	klog.V(5).Infof("Snapshotter is about to delete volume snapshot content and the operation named %s", operationName)
	ctrl.scheduleOperation(operationName, func() error {
		return ctrl.deleteSnapshotContentOperation(content)
	})
}

// scheduleOperation starts given asynchronous operation on given volume. It
// makes sure the operation is already not running.
func (ctrl *csiSnapshotController) scheduleOperation(operationName string, operation func() error) {
	klog.V(5).Infof("scheduleOperation[%s]", operationName)

	err := ctrl.runningOperations.Run(operationName, operation)
	if err != nil {
		switch {
		case goroutinemap.IsAlreadyExists(err):
			klog.V(4).Infof("operation %q is already running, skipping", operationName)
		case exponentialbackoff.IsExponentialBackoff(err):
			klog.V(4).Infof("operation %q postponed due to exponential backoff", operationName)
		default:
			klog.Errorf("error scheduling operation %q: %v", operationName, err)
		}
	}
}

func (ctrl *csiSnapshotController) storeSnapshotUpdate(snapshot interface{}) (bool, error) {
	return storeObjectUpdate(ctrl.snapshotStore, snapshot, "snapshot")
}

func (ctrl *csiSnapshotController) storeContentUpdate(content interface{}) (bool, error) {
	return storeObjectUpdate(ctrl.contentStore, content, "content")
}

// createSnapshot starts new asynchronous operation to create snapshot
func (ctrl *csiSnapshotController) createSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	klog.V(5).Infof("createSnapshot[%s]: started", snapshotKey(snapshot))
	opName := fmt.Sprintf("create-%s[%s]", snapshotKey(snapshot), string(snapshot.UID))
	ctrl.scheduleOperation(opName, func() error {
		snapshotObj, err := ctrl.createSnapshotOperation(snapshot)
		if err != nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotCreationFailed", fmt.Sprintf("Failed to create snapshot: %v", err))
			klog.Errorf("createSnapshot [%s]: error occurred in createSnapshotOperation: %v", opName, err)
			return err
		}
		_, updateErr := ctrl.storeSnapshotUpdate(snapshotObj)
		if updateErr != nil {
			// We will get an "snapshot update" event soon, this is not a big error
			klog.V(4).Infof("createSnapshot [%s]: cannot update internal cache: %v", snapshotKey(snapshotObj), updateErr)
		}
		return nil
	})
	return nil
}

func (ctrl *csiSnapshotController) checkandUpdateBoundSnapshotStatus(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("checkandUpdateSnapshotStatus[%s] started", snapshotKey(snapshot))
	opName := fmt.Sprintf("check-%s[%s]", snapshotKey(snapshot), string(snapshot.UID))
	ctrl.scheduleOperation(opName, func() error {
		snapshotObj, err := ctrl.checkandUpdateBoundSnapshotStatusOperation(snapshot, content)
		if err != nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotCheckandUpdateFailed", fmt.Sprintf("Failed to check and update snapshot: %v", err))
			klog.Errorf("checkandUpdateSnapshotStatus [%s]: error occured %v", snapshotKey(snapshot), err)
			return err
		}
		_, updateErr := ctrl.storeSnapshotUpdate(snapshotObj)
		if updateErr != nil {
			// We will get an "snapshot update" event soon, this is not a big error
			klog.V(4).Infof("checkandUpdateSnapshotStatus [%s]: cannot update internal cache: %v", snapshotKey(snapshotObj), updateErr)
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
func (ctrl *csiSnapshotController) updateSnapshotErrorStatusWithEvent(snapshot *crdv1.VolumeSnapshot, eventtype, reason, message string) error {
	klog.V(5).Infof("updateSnapshotStatusWithEvent[%s]", snapshotKey(snapshot))

	if snapshot.Status.Error != nil && snapshot.Status.Error.Message != nil && *snapshot.Status.Error.Message == message {
		klog.V(4).Infof("updateSnapshotStatusWithEvent[%s]: the same error %v is already set", snapshot.Name, snapshot.Status.Error)
		return nil
	}
	snapshotClone := snapshot.DeepCopy()
	statusError := &crdv1.VolumeSnapshotError{
		Time: &metav1.Time{
			Time: time.Now(),
		},
		Message: &message,
	}
	snapshotClone.Status.Error = statusError
	ready := false
	snapshotClone.Status.ReadyToUse = &ready
	newSnapshot, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(snapshotClone)

	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] error status failed %v", snapshotKey(snapshot), err)
		return err
	}

	_, err = ctrl.storeSnapshotUpdate(newSnapshot)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] error status: cannot update internal cache %v", snapshotKey(snapshot), err)
		return err
	}
	// Emit the event only when the status change happens
	ctrl.eventRecorder.Event(newSnapshot, eventtype, reason, message)

	return nil
}

// Stateless functions
func getSnapshotStatusForLogging(snapshot *crdv1.VolumeSnapshot) string {
	snapshotContentName := ""
	readyToUse := false
	if snapshot.Status.BoundVolumeSnapshotContentName != nil {
		snapshotContentName = *snapshot.Status.BoundVolumeSnapshotContentName
	}
	if snapshot.Status.ReadyToUse != nil {
		readyToUse = *snapshot.Status.ReadyToUse
	}
	return fmt.Sprintf("bound to: %q, Completed: %v", snapshotContentName, readyToUse)
}

// IsSnapshotBound returns true/false if snapshot is bound
func IsSnapshotBound(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) bool {
	if content.Spec.VolumeSnapshotRef.Name == snapshot.Name &&
		content.Spec.VolumeSnapshotRef.UID == snapshot.UID {
		return true
	}
	return false
}

// isSnapshotConentBeingUsed checks if snapshot content is bound to snapshot.
func (ctrl *csiSnapshotController) isSnapshotContentBeingUsed(content *crdv1.VolumeSnapshotContent) bool {
	snapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(content.Spec.VolumeSnapshotRef.Namespace).Get(content.Spec.VolumeSnapshotRef.Name, metav1.GetOptions{})
	if err != nil {
		klog.Infof("isSnapshotContentBeingUsed: Cannot get snapshot %s from api server: [%v]. VolumeSnapshot object may be deleted already.", content.Spec.VolumeSnapshotRef.Name, err)
		return false
	}

	// Check if the snapshot content is bound to the snapshot
	if IsSnapshotBound(snapshotObj, content) && *snapshotObj.Status.BoundVolumeSnapshotContentName == content.Name {
		klog.Infof("isSnapshotContentBeingUsed: VolumeSnapshot %s is bound to volumeSnapshotContent [%s]", snapshotObj.Name, content.Name)
		return true
	}

	klog.V(5).Infof("isSnapshotContentBeingUsed: Snapshot content %s is not being used", content.Name)
	return false
}

// isVolumeBeingCreatedFromSnapshot checks if an volume is being created from the snapshot.
func (ctrl *csiSnapshotController) isVolumeBeingCreatedFromSnapshot(snapshot *crdv1.VolumeSnapshot) bool {
	pvcList, err := ctrl.pvcLister.PersistentVolumeClaims(snapshot.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("Failed to retrieve PVCs from the lister to check if volume snapshot %s is being used by a volume: %q", snapshotKey(snapshot), err)
		return false
	}
	for _, pvc := range pvcList {
		if pvc.Spec.DataSource != nil && len(pvc.Spec.DataSource.Name) > 0 && pvc.Spec.DataSource.Name == snapshot.Name {
			if pvc.Spec.DataSource.Kind == snapshotKind && *(pvc.Spec.DataSource.APIGroup) == snapshotAPIGroup {
				if pvc.Status.Phase == v1.ClaimPending {
					// A volume is being created from the snapshot
					klog.Infof("isVolumeBeingCreatedFromSnapshot: volume %s is being created from snapshot %s", pvc.Name, pvc.Spec.DataSource.Name)
					return true
				}
			}
		}
	}
	klog.V(5).Infof("isVolumeBeingCreatedFromSnapshot: no volume is being created from snapshot %s", snapshotKey(snapshot))
	return false
}

// The function checks whether the volumeSnapshotRef in snapshot content matches the given snapshot. If match, it binds the content with the snapshot
func (ctrl *csiSnapshotController) checkandBindSnapshotContent(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshotContent, error) {
	if content.Spec.VolumeSnapshotRef.Name != snapshot.Name {
		return nil, fmt.Errorf("Could not bind snapshot %s and content %s, the VolumeSnapshotRef does not match", snapshot.Name, content.Name)
	} else if content.Spec.VolumeSnapshotRef.UID != "" && content.Spec.VolumeSnapshotRef.UID != snapshot.UID {
		return nil, fmt.Errorf("Could not bind snapshot %s and content %s, the VolumeSnapshotRef does not match", snapshot.Name, content.Name)
	} else if content.Spec.VolumeSnapshotRef.UID != "" {
		return content, nil
	}
	contentClone := content.DeepCopy()
	contentClone.Spec.VolumeSnapshotRef.UID = snapshot.UID
	newContent, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Update(contentClone)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status failed %v", newContent.Name, err)
		return nil, err
	}
	_, err = ctrl.storeContentUpdate(newContent)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status: cannot update internal cache %v", newContent.Name, err)
		return nil, err
	}
	return newContent, nil
}

func (ctrl *csiSnapshotController) getCreateSnapshotInput(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotClass, *v1.PersistentVolume, string, *v1.SecretReference, error) {
	className := snapshot.Spec.VolumeSnapshotClassName
	klog.V(5).Infof("getCreateSnapshotInput [%s]: VolumeSnapshotClassName [%s]", snapshot.Name, *className)
	var class *crdv1.VolumeSnapshotClass
	var err error
	if className != nil {
		class, err = ctrl.GetSnapshotClass(*className)
		if err != nil {
			klog.Errorf("getCreateSnapshotInput failed to getClassFromVolumeSnapshot %s", err)
			return nil, nil, "", nil, err
		}
	} else {
		klog.Errorf("failed to getCreateSnapshotInput %s without a snapshot class", snapshot.Name)
		return nil, nil, "", nil, fmt.Errorf("failed to take snapshot %s without a snapshot class", snapshot.Name)
	}

	volume, err := ctrl.getVolumeFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Errorf("getCreateSnapshotInput failed to get PersistentVolume object [%s]: Error: [%#v]", snapshot.Name, err)
		return nil, nil, "", nil, err
	}

	// Create VolumeSnapshotContent name
	contentName := GetSnapshotContentNameForSnapshot(snapshot)

	// Resolve snapshotting secret credentials.
	snapshotterSecretRef, err := getSecretReference(class.Parameters, contentName, snapshot)
	if err != nil {
		return nil, nil, "", nil, err
	}

	return class, volume, contentName, snapshotterSecretRef, nil
}

func (ctrl *csiSnapshotController) checkandUpdateBoundSnapshotStatusOperation(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshot, error) {
	var err error
	var creationTime time.Time
	var size int64
	var readyToUse = false
	var driverName string
	var snapshotID string

	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		klog.V(5).Infof("checkandUpdateBoundSnapshotStatusOperation: checking whether snapshot [%s] is pre-bound to content [%s]", snapshot.Name, content.Name)
		readyToUse, creationTime, size, err = ctrl.handler.GetSnapshotStatus(content)
		if err != nil {
			klog.Errorf("checkandUpdateBoundSnapshotStatusOperation: failed to call get snapshot status to check whether snapshot is ready to use %q", err)
			return nil, err
		}
		driverName, snapshotID = content.Spec.Driver, *content.Status.SnapshotHandle
	} else {
		class, volume, _, snapshotterSecretRef, err := ctrl.getCreateSnapshotInput(snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to get input parameters to create snapshot %s: %q", snapshot.Name, err)
		}
		snapshotterCredentials, err := getCredentials(ctrl.client, snapshotterSecretRef)
		if err != nil {
			return nil, err
		}
		driverName, snapshotID, creationTime, size, readyToUse, err = ctrl.handler.CreateSnapshot(snapshot, volume, class.Parameters, snapshotterCredentials)
		if err != nil {
			klog.Errorf("checkandUpdateBoundSnapshotStatusOperation: failed to call create snapshot to check whether the snapshot is ready to use %q", err)
			return nil, err
		}
	}
	klog.V(5).Infof("checkandUpdateBoundSnapshotStatusOperation: driver %s, snapshotId %s, creationTime %v, size %d, readyToUse %t", driverName, snapshotID, creationTime, size, readyToUse)

	if creationTime.IsZero() {
		creationTime = time.Now()
	}
	newSnapshot, err := ctrl.updateSnapshotStatus(snapshot, content.Name, readyToUse, creationTime, size)
	if err != nil {
		return nil, err
	}
	err = ctrl.updateSnapshotContentStatus(content, snapshotID, readyToUse, creationTime.UnixNano(), size)
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
func (ctrl *csiSnapshotController) createSnapshotOperation(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	klog.Infof("createSnapshot: Creating snapshot %s through the plugin ...", snapshotKey(snapshot))

	if snapshot.Status.Error != nil && snapshot.Status.Error.Message != nil && !isControllerUpdateFailError(snapshot.Status.Error) {
		klog.V(4).Infof("error is already set in snapshot, do not retry to create: %s", *snapshot.Status.Error.Message)
		return snapshot, nil
	}

	// If PVC is not being deleted and finalizer is not added yet, a finalizer should be added.
	klog.V(5).Infof("createSnapshotOperation: Check if PVC is not being deleted and add Finalizer for source of snapshot [%s] if needed", snapshot.Name)
	err := ctrl.ensureSnapshotSourceFinalizer(snapshot)
	if err != nil {
		klog.Errorf("createSnapshotOperation failed to add finalizer for source of snapshot %s", err)
		return nil, err
	}

	class, volume, contentName, snapshotterSecretRef, err := ctrl.getCreateSnapshotInput(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to get input parameters to create snapshot %s: %q", snapshot.Name, err)
	}

	snapshotterCredentials, err := getCredentials(ctrl.client, snapshotterSecretRef)
	if err != nil {
		return nil, err
	}

	driverName, snapshotID, creationTime, size, readyToUse, err := ctrl.handler.CreateSnapshot(snapshot, volume, class.Parameters, snapshotterCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to take snapshot of the volume, %s: %q", volume.Name, err)
	}

	klog.V(5).Infof("Created snapshot: driver %s, snapshotId %s, creationTime %v, size %d, readyToUse %t", driverName, snapshotID, creationTime, size, readyToUse)

	var newSnapshot *crdv1.VolumeSnapshot
	// Update snapshot status with creationTime
	for i := 0; i < ctrl.createSnapshotContentRetryCount; i++ {
		klog.V(5).Infof("createSnapshot [%s]: trying to update snapshot creation timestamp", snapshotKey(snapshot))
		newSnapshot, err = ctrl.updateSnapshotStatus(snapshot, contentName, readyToUse, creationTime, size)
		if err == nil {
			break
		}
		klog.V(4).Infof("failed to update snapshot %s creation timestamp: %v", snapshotKey(snapshot), err)
	}

	if err != nil {
		return nil, err
	}
	// Create VolumeSnapshotContent in the database
	snapshotRef, err := ref.GetReference(scheme.Scheme, snapshot)
	if err != nil {
		return nil, err
	}

	timestamp := creationTime.UnixNano()
	snapshotContent := &crdv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentName,
		},
		Spec: crdv1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: *snapshotRef,
			DeletionPolicy:    class.DeletionPolicy,
			Driver:            driverName,
			SnapshotClassName: &class.Name,
			Source: crdv1.VolumeSnapshotContentSource{
				VolumeHandle: &volume.Spec.CSI.VolumeHandle,
			},
		},
		Status: crdv1.VolumeSnapshotContentStatus{
			SnapshotHandle: &snapshotID,
			CreationTime:   &timestamp,
			RestoreSize:    &size,
			ReadyToUse:     &readyToUse,
		},
	}

	// Set AnnDeletionSecretRefName and AnnDeletionSecretRefNamespace
	if snapshotterSecretRef != nil {
		klog.V(5).Infof("createSnapshotOperation: set annotation [%s] on content [%s].", AnnDeletionSecretRefName, snapshotContent.Name)
		metav1.SetMetaDataAnnotation(&snapshotContent.ObjectMeta, AnnDeletionSecretRefName, snapshotterSecretRef.Name)

		klog.V(5).Infof("syncContent: set annotation [%s] on content [%s].", AnnDeletionSecretRefNamespace, snapshotContent.Name)
		metav1.SetMetaDataAnnotation(&snapshotContent.ObjectMeta, AnnDeletionSecretRefNamespace, snapshotterSecretRef.Namespace)
	}

	klog.V(3).Infof("volume snapshot content %v", snapshotContent)
	// Try to create the VolumeSnapshotContent object several times
	for i := 0; i < ctrl.createSnapshotContentRetryCount; i++ {
		klog.V(5).Infof("createSnapshot [%s]: trying to save volume snapshot content %s", snapshotKey(snapshot), snapshotContent.Name)
		if _, err = ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Create(snapshotContent); err == nil || apierrs.IsAlreadyExists(err) {
			// Save succeeded.
			if err != nil {
				klog.V(3).Infof("volume snapshot content %q for snapshot %q already exists, reusing", snapshotContent.Name, snapshotKey(snapshot))
				err = nil
			} else {
				klog.V(3).Infof("volume snapshot content %q for snapshot %q saved, %v", snapshotContent.Name, snapshotKey(snapshot), snapshotContent)
			}
			break
		}
		// Save failed, try again after a while.
		klog.V(3).Infof("failed to save volume snapshot content %q for snapshot %q: %v", snapshotContent.Name, snapshotKey(snapshot), err)
		time.Sleep(ctrl.createSnapshotContentInterval)
	}

	if err != nil {
		// Save failed. Now we have a snapshot asset outside of Kubernetes,
		// but we don't have appropriate volumesnapshot content object for it.
		// Emit some event here and controller should try to create the content in next sync period.
		strerr := fmt.Sprintf("Error creating volume snapshot content object for snapshot %s: %v.", snapshotKey(snapshot), err)
		klog.Error(strerr)
		ctrl.eventRecorder.Event(newSnapshot, v1.EventTypeWarning, "CreateSnapshotContentFailed", strerr)
		return nil, newControllerUpdateError(snapshotKey(snapshot), err.Error())
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
// 4. Remove the Snapshot from store
// 5. Finish
func (ctrl *csiSnapshotController) deleteSnapshotContentOperation(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("deleteSnapshotOperation [%s] started", content.Name)

	// get secrets if VolumeSnapshotClass specifies it
	var snapshotterCredentials map[string]string
	/* TODO(@Xing-yang): secrete ref retrivial needs to be implemented here
	  	snapshotClassName := content.Spec.VolumeSnapshotClassName
		if snapshotClassName != nil {
			if snapshotClass, err := ctrl.classLister.Get(*snapshotClassName); err == nil {
				// Resolve snapshotting secret credentials.
				// No VolumeSnapshot is provided when resolving delete secret names, since the VolumeSnapshot may or may not exist at delete time.
				snapshotterSecretRef, err := getSecretReference(snapshotClass.Parameters, content.Name, nil)
				if err != nil {
					return err
				}
				snapshotterCredentials, err = getCredentials(ctrl.client, snapshotterSecretRef)
				if err != nil {
					return err
				}
			}
		}*/

	err := ctrl.handler.DeleteSnapshot(content, snapshotterCredentials)
	if err != nil {
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotDeleteError", "Failed to delete snapshot")
		return fmt.Errorf("failed to delete snapshot %#v, err: %v", content.Name, err)
	}

	err = ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Delete(content.Name, &metav1.DeleteOptions{})
	if err != nil {
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotContentObjectDeleteError", "Failed to delete snapshot content API object")
		return fmt.Errorf("failed to delete VolumeSnapshotContent %s from API server: %q", content.Name, err)
	}

	return nil
}

func (ctrl *csiSnapshotController) bindandUpdateVolumeSnapshot(snapshotContent *crdv1.VolumeSnapshotContent, snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("bindandUpdateVolumeSnapshot for snapshot [%s]: snapshotContent [%s]", snapshot.Name, snapshotContent.Name)
	snapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshot.Namespace).Get(snapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot %s from api server: %v", snapshotKey(snapshot), err)
	}

	// Copy the snapshot object before updating it
	snapshotCopy := snapshotObj.DeepCopy()

	if snapshotObj.Status.BoundVolumeSnapshotContentName != nil && *snapshotObj.Status.BoundVolumeSnapshotContentName == snapshotContent.Name {
		klog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: VolumeSnapshot %s already bind to volumeSnapshotContent [%s]", snapshot.Name, snapshotContent.Name)
	} else {
		klog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: before bind VolumeSnapshot %s to volumeSnapshotContent [%s]", snapshot.Name, snapshotContent.Name)
		snapshotCopy.Status.BoundVolumeSnapshotContentName = &snapshotContent.Name
		updateSnapshot, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshot.Namespace).Update(snapshotCopy)
		if err != nil {
			klog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: Error binding VolumeSnapshot %s to volumeSnapshotContent [%s]. Error [%#v]", snapshot.Name, snapshotContent.Name, err)
			return nil, newControllerUpdateError(snapshotKey(snapshot), err.Error())
		}
		snapshotCopy = updateSnapshot
		_, err = ctrl.storeSnapshotUpdate(snapshotCopy)
		if err != nil {
			klog.Errorf("%v", err)
		}
	}

	klog.V(5).Infof("bindandUpdateVolumeSnapshot for snapshot completed [%#v]", snapshotCopy)
	return snapshotCopy, nil
}

// TODO(xiangqian) This is a temp implementation to make sure the test cases pass
func (ctrl *csiSnapshotController) updateSnapshotContentStatus(
	content *crdv1.VolumeSnapshotContent,
	snapshotHandle string,
	readyToUse bool,
	createdAt int64,
	size int64) error {
	currentStatus := content.Status
	updated := false
	if currentStatus.SnapshotHandle == nil {
		currentStatus.SnapshotHandle = &snapshotHandle
		updated = true
	}
	if currentStatus.ReadyToUse == nil || *currentStatus.ReadyToUse != readyToUse {
		currentStatus.ReadyToUse = &readyToUse
		updated = true
	}
	if currentStatus.CreationTime == nil {
		currentStatus.CreationTime = &createdAt
		updated = true
	}
	if currentStatus.RestoreSize == nil {
		currentStatus.RestoreSize = &size
		updated = true
	}

	if updated {
		contentClone := content.DeepCopy()
		contentClone.Status = currentStatus
		_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().UpdateStatus(contentClone)
		if err != nil {
			return newControllerUpdateError(content.Name, err.Error())
		}
		return nil
	}
	return nil
}

// UpdateSnapshotStatus converts snapshot status to crdv1.VolumeSnapshotCondition
func (ctrl *csiSnapshotController) updateSnapshotStatus(
	snapshot *crdv1.VolumeSnapshot,
	boundContentName string,
	readyToUse bool,
	createdAt time.Time,
	size int64) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("updating VolumeSnapshot[]%s, readyToUse %v, timestamp %v", snapshotKey(snapshot), readyToUse, createdAt)
	status := snapshot.Status
	updated := false

	if status.BoundVolumeSnapshotContentName == nil {
		status.BoundVolumeSnapshotContentName = &boundContentName
		updated = true
	}

	if status.CreationTime == nil {
		status.CreationTime = &metav1.Time{Time: createdAt}
		updated = true
	}

	if status.ReadyToUse == nil || *status.ReadyToUse != readyToUse {
		status.ReadyToUse = &readyToUse
		updated = true
	}

	if status.RestoreSize == nil {
		status.RestoreSize = resource.NewQuantity(size, resource.BinarySI)
		updated = true
	}

	if updated {
		snapshotClone := snapshot.DeepCopy()
		snapshotClone.Status = status
		newSnapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(snapshotClone)
		if err != nil {
			return nil, newControllerUpdateError(snapshotKey(snapshot), err.Error())
		}
		return newSnapshotObj, nil
	}
	return snapshot, nil
}

// getVolumeFromVolumeSnapshot is a helper function to get PV from VolumeSnapshot.
func (ctrl *csiSnapshotController) getVolumeFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolume, error) {
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	if pvc.Status.Phase != v1.ClaimBound {
		return nil, fmt.Errorf("the PVC %s is not yet bound to a PV, will not attempt to take a snapshot", pvc.Name)
	}

	pvName := pvc.Spec.VolumeName
	pv, err := ctrl.client.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PV %s from the API server: %q", pvName, err)
	}

	klog.V(5).Infof("getVolumeFromVolumeSnapshot: snapshot [%s] PV name [%s]", snapshot.Name, pvName)

	return pv, nil
}

func (ctrl *csiSnapshotController) getStorageClassFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*storagev1.StorageClass, error) {
	// Get storage class from PVC or PV
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	storageclassName := *pvc.Spec.StorageClassName
	if len(storageclassName) == 0 {
		volume, err := ctrl.getVolumeFromVolumeSnapshot(snapshot)
		if err != nil {
			return nil, err
		}
		storageclassName = volume.Spec.StorageClassName
	}
	if len(storageclassName) == 0 {
		return nil, fmt.Errorf("cannot figure out the snapshot class automatically, please specify one in snapshot spec")
	}
	storageclass, err := ctrl.client.StorageV1().StorageClasses().Get(storageclassName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return storageclass, nil
}

// GetSnapshotClass is a helper function to get snapshot class from the class name.
func (ctrl *csiSnapshotController) GetSnapshotClass(className string) (*crdv1.VolumeSnapshotClass, error) {
	klog.V(5).Infof("getSnapshotClass: VolumeSnapshotClassName [%s]", className)

	class, err := ctrl.classLister.Get(className)
	if err != nil {
		klog.Errorf("failed to retrieve snapshot class %s from the informer: %q", className, err)
		return nil, fmt.Errorf("failed to retrieve snapshot class %s from the informer: %q", className, err)
	}

	return class, nil
}

// SetDefaultSnapshotClass is a helper function to figure out the default snapshot class from
// PVC/PV StorageClass and update VolumeSnapshot with this snapshot class name.
func (ctrl *csiSnapshotController) SetDefaultSnapshotClass(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotClass, *crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("SetDefaultSnapshotClass for snapshot [%s]", snapshot.Name)

	storageclass, err := ctrl.getStorageClassFromVolumeSnapshot(snapshot)
	if err != nil {
		return nil, nil, err
	}
	// Find default snapshot class if available
	list, err := ctrl.classLister.List(labels.Everything())
	if err != nil {
		return nil, nil, err
	}
	defaultClasses := []*crdv1.VolumeSnapshotClass{}

	for _, class := range list {
		if IsDefaultAnnotation(class.ObjectMeta) && storageclass.Provisioner == class.Driver && ctrl.driverName == class.Driver {
			defaultClasses = append(defaultClasses, class)
			klog.V(5).Infof("get defaultClass added: %s", class.Name)
		}
	}
	if len(defaultClasses) == 0 {
		return nil, nil, fmt.Errorf("cannot find default snapshot class")
	}
	if len(defaultClasses) > 1 {
		klog.V(4).Infof("get DefaultClass %d defaults found", len(defaultClasses))
		return nil, nil, fmt.Errorf("%d default snapshot classes were found", len(defaultClasses))
	}
	klog.V(5).Infof("setDefaultSnapshotClass [%s]: default VolumeSnapshotClassName [%s]", snapshot.Name, defaultClasses[0].Name)
	snapshotClone := snapshot.DeepCopy()
	snapshotClone.Spec.VolumeSnapshotClassName = &(defaultClasses[0].Name)
	newSnapshot, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] default class failed %v", snapshotKey(snapshot), err)
	}
	_, updateErr := ctrl.storeSnapshotUpdate(newSnapshot)
	if updateErr != nil {
		// We will get an "snapshot update" event soon, this is not a big error
		klog.V(4).Infof("setDefaultSnapshotClass [%s]: cannot update internal cache: %v", snapshotKey(snapshot), updateErr)
	}

	return defaultClasses[0], newSnapshot, nil
}

// getClaimFromVolumeSnapshot is a helper function to get PVC from VolumeSnapshot.
func (ctrl *csiSnapshotController) getClaimFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolumeClaim, error) {
	if snapshot.Spec.Source.PersistentVolumeClaimName == nil {
		return nil, fmt.Errorf("the snapshot source is not the right type. Expected PersistentVolumeClaimName to be valid")
	}
	pvcName := *snapshot.Spec.Source.PersistentVolumeClaimName
	if pvcName == "" {
		return nil, fmt.Errorf("the PVC name is not specified in snapshot %s", snapshotKey(snapshot))
	}

	pvc, err := ctrl.pvcLister.PersistentVolumeClaims(snapshot.Namespace).Get(pvcName)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PVC %s from the lister: %q", pvcName, err)
	}

	return pvc, nil
}

var _ error = controllerUpdateError{}

type controllerUpdateError struct {
	message string
}

func newControllerUpdateError(name, message string) error {
	return controllerUpdateError{
		message: fmt.Sprintf("%s %s on API server: %s", controllerUpdateFailMsg, name, message),
	}
}

func (e controllerUpdateError) Error() string {
	return e.message
}

func isControllerUpdateFailError(err *crdv1.VolumeSnapshotError) bool {
	if err != nil && err.Message != nil {
		if strings.Contains(*err.Message, controllerUpdateFailMsg) {
			return true
		}
	}
	return false
}

// addContentFinalizer adds a Finalizer for VolumeSnapshotContent.
func (ctrl *csiSnapshotController) addContentFinalizer(content *crdv1.VolumeSnapshotContent) error {
	contentClone := content.DeepCopy()
	contentClone.ObjectMeta.Finalizers = append(contentClone.ObjectMeta.Finalizers, VolumeSnapshotContentFinalizer)

	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Update(contentClone)
	if err != nil {
		return newControllerUpdateError(content.Name, err.Error())
	}

	_, err = ctrl.storeContentUpdate(contentClone)
	if err != nil {
		klog.Errorf("failed to update content store %v", err)
	}

	klog.V(5).Infof("Added protection finalizer to volume snapshot content %s", content.Name)
	return nil
}

// removeContentFinalizer removes a Finalizer for VolumeSnapshotContent.
func (ctrl *csiSnapshotController) removeContentFinalizer(content *crdv1.VolumeSnapshotContent) error {
	contentClone := content.DeepCopy()
	contentClone.ObjectMeta.Finalizers = slice.RemoveString(contentClone.ObjectMeta.Finalizers, VolumeSnapshotContentFinalizer, nil)

	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Update(contentClone)
	if err != nil {
		return newControllerUpdateError(content.Name, err.Error())
	}

	_, err = ctrl.storeContentUpdate(contentClone)
	if err != nil {
		klog.Errorf("failed to update content store %v", err)
	}

	klog.V(5).Infof("Removed protection finalizer from volume snapshot content %s", content.Name)
	return nil
}

// addSnapshotFinalizer adds a Finalizer for VolumeSnapshot.
func (ctrl *csiSnapshotController) addSnapshotFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	snapshotClone := snapshot.DeepCopy()
	snapshotClone.ObjectMeta.Finalizers = append(snapshotClone.ObjectMeta.Finalizers, VolumeSnapshotFinalizer)
	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
	if err != nil {
		return newControllerUpdateError(snapshot.Name, err.Error())
	}

	_, err = ctrl.storeSnapshotUpdate(snapshotClone)
	if err != nil {
		klog.Errorf("failed to update snapshot store %v", err)
	}

	klog.V(5).Infof("Added protection finalizer to volume snapshot %s", snapshotKey(snapshot))
	return nil
}

// removeContentFinalizer removes a Finalizer for VolumeSnapshot.
func (ctrl *csiSnapshotController) removeSnapshotFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	snapshotClone := snapshot.DeepCopy()
	snapshotClone.ObjectMeta.Finalizers = slice.RemoveString(snapshotClone.ObjectMeta.Finalizers, VolumeSnapshotFinalizer, nil)

	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
	if err != nil {
		return newControllerUpdateError(snapshot.Name, err.Error())
	}

	_, err = ctrl.storeSnapshotUpdate(snapshotClone)
	if err != nil {
		klog.Errorf("failed to update snapshot store %v", err)
	}

	klog.V(5).Infof("Removed protection finalizer from volume snapshot %s", snapshotKey(snapshot))
	return nil
}

// ensureSnapshotSourceFinalizer checks if a Finalizer needs to be added for the snapshot source;
// if true, adds a Finalizer for VolumeSnapshot Source PVC
func (ctrl *csiSnapshotController) ensureSnapshotSourceFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already.", snapshot.Name, err)
		return nil
	}

	// If PVC is not being deleted and PVCFinalizer is not added yet, the PVCFinalizer should be added.
	if pvc.ObjectMeta.DeletionTimestamp == nil && !slice.ContainsString(pvc.ObjectMeta.Finalizers, PVCFinalizer, nil) {
		// Add the finalizer
		pvcClone := pvc.DeepCopy()
		pvcClone.ObjectMeta.Finalizers = append(pvcClone.ObjectMeta.Finalizers, PVCFinalizer)
		_, err = ctrl.client.CoreV1().PersistentVolumeClaims(pvcClone.Namespace).Update(pvcClone)
		if err != nil {
			klog.Errorf("cannot add finalizer on claim [%s] for snapshot [%s]: [%v]", pvc.Name, snapshot.Name, err)
			return newControllerUpdateError(pvcClone.Name, err.Error())
		}
		klog.Infof("Added protection finalizer to persistent volume claim %s", pvc.Name)
	}

	return nil
}

// removeSnapshotSourceFinalizer removes a Finalizer for VolumeSnapshot Source PVC.
func (ctrl *csiSnapshotController) removeSnapshotSourceFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already. No need to remove finalizer on the claim.", snapshot.Name, err)
		return nil
	}

	pvcClone := pvc.DeepCopy()
	pvcClone.ObjectMeta.Finalizers = slice.RemoveString(pvcClone.ObjectMeta.Finalizers, PVCFinalizer, nil)

	_, err = ctrl.client.CoreV1().PersistentVolumeClaims(pvcClone.Namespace).Update(pvcClone)
	if err != nil {
		return newControllerUpdateError(pvcClone.Name, err.Error())
	}

	klog.V(5).Infof("Removed protection finalizer from persistent volume claim %s", pvc.Name)
	return nil
}

// isSnapshotSourceBeingUsed checks if a PVC is being used as a source to create a snapshot
func (ctrl *csiSnapshotController) isSnapshotSourceBeingUsed(snapshot *crdv1.VolumeSnapshot) bool {
	klog.V(5).Infof("isSnapshotSourceBeingUsed[%s]: started", snapshotKey(snapshot))
	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("isSnapshotSourceBeingUsed: cannot to get claim from snapshot: %v", err)
		return false
	}

	// Going through snapshots in the cache (snapshotLister). If a snapshot's PVC source
	// is the same as the input snapshot's PVC source and snapshot's ReadyToUse status
	// is false, the snapshot is still being created from the PVC and the PVC is in-use.
	snapshots, err := ctrl.snapshotLister.VolumeSnapshots(snapshot.Namespace).List(labels.Everything())
	if err != nil {
		return false
	}
	for _, snap := range snapshots {
		// Skip static bound snapshot without a PVC source
		if snap.Spec.Source.PersistentVolumeClaimName == nil {
			klog.V(4).Infof("Skipping static bound snapshot %s when checking PVC %s/%s", snap.Name, pvc.Namespace, pvc.Name)
			continue
		}
		if pvc.Name == *snap.Spec.Source.PersistentVolumeClaimName && (snap.Status.ReadyToUse == nil || *snap.Status.ReadyToUse == false) {
			klog.V(2).Infof("Keeping PVC %s/%s, it is used by snapshot %s/%s", pvc.Namespace, pvc.Name, snap.Namespace, snap.Name)
			return true
		}
	}

	klog.V(5).Infof("isSnapshotSourceBeingUsed: no snapshot is being created from PVC %s/%s", pvc.Namespace, pvc.Name)
	return false
}

// checkandRemoveSnapshotSourceFinalizer checks if the snapshot source finalizer should be removed
// and removed it if needed.
func (ctrl *csiSnapshotController) checkandRemoveSnapshotSourceFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already. No need to remove finalizer on the claim.", snapshot.Name, err)
		return nil
	}

	klog.V(5).Infof("checkandRemoveSnapshotSourceFinalizer for snapshot [%s]: snapshot status [%#v]", snapshot.Name, snapshot.Status)

	// Check if there is a Finalizer on PVC to be removed
	if slice.ContainsString(pvc.ObjectMeta.Finalizers, PVCFinalizer, nil) {
		// There is a Finalizer on PVC. Check if PVC is used
		// and remove finalizer if it's not used.
		isUsed := ctrl.isSnapshotSourceBeingUsed(snapshot)
		if !isUsed {
			klog.Infof("checkandRemoveSnapshotSourceFinalizer[%s]: Remove Finalizer for PVC %s as it is not used by snapshots in creation", snapshot.Name, pvc.Name)
			err = ctrl.removeSnapshotSourceFinalizer(snapshot)
			if err != nil {
				klog.Errorf("checkandRemoveSnapshotSourceFinalizer [%s]: removeSnapshotSourceFinalizer failed to remove finalizer %v", snapshot.Name, err)
				return err
			}
		}
	}

	return nil
}
