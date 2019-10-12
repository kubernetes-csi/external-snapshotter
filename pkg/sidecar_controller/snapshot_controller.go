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
	"github.com/kubernetes-csi/external-snapshotter/pkg/utils"
	"k8s.io/api/core/v1"
	//storage "k8s.io/api/storage/v1beta1"
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
func (ctrl *csiSnapshotSideCarController) syncContent(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("synchronizing VolumeSnapshotContent[%s]", content.Name)

	if ann := content.Annotations[utils.AnnDynamicallyProvisioned]; ann != ctrl.snapshotterName {
		klog.Errorf("syncContent: Content [%s] annDynamicallyProvisioned [%s] is not the same as snapshotterName [%s].", content.Name, ann, ctrl.snapshotterName)
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotContentNotBound", "VolumeSnapshotContent is not bound to any VolumeSnapshot")
		return fmt.Errorf("volumeSnapshotContent %s annDynamicallyProvisioned [%s] is not the same as snapshotterName [%s].", content.Name, ann, ctrl.snapshotterName)
	}

	if metav1.HasAnnotation(content.ObjectMeta, utils.AnnShouldDelete) {
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
		// By default, we use Retain policy if it is not set by users
		klog.V(4).Infof("VolumeSnapshotContent[%s]: the policy is %s", content.Name, content.Spec.DeletionPolicy)

	}
	return nil
}

// syncSnapshot is the main controller method to decide what to do with a snapshot.
// It's invoked by appropriate cache.Controller callbacks when a snapshot is
// created, updated or periodically synced. We do not differentiate between
// these events.
func (ctrl *csiSnapshotSideCarController) syncSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	uniqueSnapshotName := utils.SnapshotKey(snapshot)
	klog.V(5).Infof("synchonizing VolumeSnapshot[%s]: %s", uniqueSnapshotName, utils.GetSnapshotStatusForLogging(snapshot))

	klog.V(5).Infof("syncSnapshot[%s]: check if we should remove finalizer on snapshot source and remove it if we can", uniqueSnapshotName)
	// Check if we should remove finalizer on snapshot source and remove it if we can.
	errFinalizer := ctrl.checkandRemoveSnapshotSourceFinalizer(snapshot)
	if errFinalizer != nil {
		klog.Errorf("error check and remove snapshot source finalizer for snapshot [%s]: %v", snapshot.Name, errFinalizer)
		// Log an event and keep the original error from syncUnready/ReadySnapshot
		ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "ErrorSnapshotSourceFinalizer", "Error check and remove PVC Finalizer for VolumeSnapshot")
	}

	if snapshot.Status.BoundVolumeSnapshotContentName != nil && *snapshot.Status.BoundVolumeSnapshotContentName != "" {
		contentObj, found, err := ctrl.contentStore.GetByKey(*snapshot.Status.BoundVolumeSnapshotContentName)
		if err != nil {
			return err
		}
		if !found {
			// snapshot is bound to a non-existing content.
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMissing", "VolumeSnapshotContent is missing")
			klog.V(4).Infof("synchronizing unready snapshot[%s]: snapshotcontent %q requested and not found, will try again next time", uniqueSnapshotName, *snapshot.Status.BoundVolumeSnapshotContentName)
			return fmt.Errorf("snapshot %s is bound to a non-existing content %s", uniqueSnapshotName, *snapshot.Status.BoundVolumeSnapshotContentName)
		}
		content, ok := contentObj.(*crdv1.VolumeSnapshotContent)
		if !ok {
			return fmt.Errorf("expected volume snapshot content, got %+v", contentObj)
		}

		// snapshot is already bound correctly, check the status and update if it is ready.
		klog.V(5).Infof("Check and update snapshot %s status", uniqueSnapshotName)
		if err = ctrl.checkandUpdateBoundSnapshotStatus(snapshot, content); err != nil {
			return err
		}
		return nil
	} else { // snapshot.Status.BoundVolumeSnapshotContentName == nil
		provision, err := ctrl.shouldProvision(snapshot)
		if !provision {
			klog.V(3).Infof("syncUnreadySnapshot: Should not provision snapshot %s", snapshot.Name)
			return nil
		}
		klog.V(5).Infof("syncUnreadySnapshot: Call CreateSnapshot %s", snapshot.Name)
		if err = ctrl.createSnapshot(snapshot); err != nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotCreationFailed", fmt.Sprintf("Failed to create snapshot with error %v", err))
			return err
		}
		return nil
	}
}

// setSnapshotAnnDynamicallyCreated sets annotation AnnDynamicallyCreated for snapshot
func (ctrl *csiSnapshotSideCarController) setSnapshotAnnDynamicallyCreated(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("setSnapshotAnnDynamicallyCreated for snapshot [%s]", utils.SnapshotKey(snapshot))
	snapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshot.Namespace).Get(snapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot %s from api server: %v", utils.SnapshotKey(snapshot), err)
	}

	// Copy the snapshot object before updating it
	snapshotCopy := snapshotObj.DeepCopy()

	// Set AnnBoundByCreated if it is not set yet
	if !metav1.HasAnnotation(snapshotCopy.ObjectMeta, utils.AnnDynamicallyCreated) {
		klog.V(5).Infof("setSnapshotAnnDynamicallyCreated: Set annotation [%s] to yes on snapshot %s", utils.AnnDynamicallyCreated, utils.SnapshotKey(snapshot))
		metav1.SetMetaDataAnnotation(&snapshotCopy.ObjectMeta, utils.AnnDynamicallyCreated, "yes")
	}

	updateSnapshot, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotCopy.Namespace).Update(snapshotCopy)
	if err != nil {
		return nil, newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
	}

	snapshotCopy = updateSnapshot
	_, err = ctrl.storeSnapshotUpdate(snapshotCopy)
	if err != nil {
		klog.Errorf("failed to update snapshot store %v", err)
	}
	klog.V(5).Infof("setSnapshotAnnDynamicallyCreated for snapshot completed [%#v]", snapshotCopy)
	return snapshotCopy, nil
}

// deleteSnapshotContent starts delete action.
func (ctrl *csiSnapshotSideCarController) deleteSnapshotContent(content *crdv1.VolumeSnapshotContent) {
	operationName := fmt.Sprintf("delete-%s[%s]", content.Name, string(content.UID))
	klog.V(5).Infof("Snapshotter is about to delete volume snapshot content and the operation named %s", operationName)
	ctrl.scheduleOperation(operationName, func() error {
		return ctrl.deleteSnapshotContentOperation(content)
	})
}

// scheduleOperation starts given asynchronous operation on given volume. It
// makes sure the operation is already not running.
func (ctrl *csiSnapshotSideCarController) scheduleOperation(operationName string, operation func() error) {
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

func (ctrl *csiSnapshotSideCarController) storeSnapshotUpdate(snapshot interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.snapshotStore, snapshot, "snapshot")
}

func (ctrl *csiSnapshotSideCarController) storeContentUpdate(content interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.contentStore, content, "content")
}

// createSnapshot starts new asynchronous operation to create snapshot
func (ctrl *csiSnapshotSideCarController) createSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	klog.V(5).Infof("createSnapshot[%s]: started", utils.SnapshotKey(snapshot))
	opName := fmt.Sprintf("create-%s[%s]", utils.SnapshotKey(snapshot), string(snapshot.UID))
	ctrl.scheduleOperation(opName, func() error {
		snapshotObj, err := ctrl.createSnapshotOperation(snapshot)
		if err != nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotCreationFailed", fmt.Sprintf("Failed to create snapshot: %v", err))
			klog.Errorf("createSnapshot [%s]: error occurred in createSnapshotOperation: %v", opName, err)
			return err
		}

		// Set AnnDynamicallyCreated
		updateSnapshot, err := ctrl.setSnapshotAnnDynamicallyCreated(snapshotObj)
		if err != nil {
			klog.Errorf("createSnapshot [%s]: cannot update annotation annDynamicallyCreated: %v", utils.SnapshotKey(snapshotObj), err)
			return err
		}

		snapshotObj = updateSnapshot
		_, updateErr := ctrl.storeSnapshotUpdate(snapshotObj)
		if updateErr != nil {
			// We will get an "snapshot update" event soon, this is not a big error
			klog.V(4).Infof("createSnapshot [%s]: cannot update internal cache: %v", utils.SnapshotKey(snapshotObj), updateErr)
		}
		return nil
	})
	return nil
}

func (ctrl *csiSnapshotSideCarController) checkandUpdateBoundSnapshotStatus(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("checkandUpdateSnapshotStatus[%s] started", utils.SnapshotKey(snapshot))
	opName := fmt.Sprintf("check-%s[%s]", utils.SnapshotKey(snapshot), string(snapshot.UID))
	ctrl.scheduleOperation(opName, func() error {
		snapshotObj, err := ctrl.checkandUpdateBoundSnapshotStatusOperation(snapshot, content)
		if err != nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotCheckandUpdateFailed", fmt.Sprintf("Failed to check and update snapshot: %v", err))
			klog.Errorf("checkandUpdateSnapshotStatus [%s]: error occured %v", utils.SnapshotKey(snapshot), err)
			return err
		}
		_, updateErr := ctrl.storeSnapshotUpdate(snapshotObj)
		if updateErr != nil {
			// We will get an "snapshot update" event soon, this is not a big error
			klog.V(4).Infof("checkandUpdateSnapshotStatus [%s]: cannot update internal cache: %v", utils.SnapshotKey(snapshotObj), updateErr)
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
func (ctrl *csiSnapshotSideCarController) updateSnapshotErrorStatusWithEvent(snapshot *crdv1.VolumeSnapshot, eventtype, reason, message string) error {
	klog.V(5).Infof("updateSnapshotStatusWithEvent[%s]", utils.SnapshotKey(snapshot))

	if snapshot.Status.Error != nil && *snapshot.Status.Error.Message == message {
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
		klog.V(4).Infof("updating VolumeSnapshot[%s] error status failed %v", utils.SnapshotKey(snapshot), err)
		return err
	}

	_, err = ctrl.storeSnapshotUpdate(newSnapshot)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] error status: cannot update internal cache %v", utils.SnapshotKey(snapshot), err)
		return err
	}
	// Emit the event only when the status change happens
	ctrl.eventRecorder.Event(newSnapshot, eventtype, reason, message)

	return nil
}

func (ctrl *csiSnapshotSideCarController) getCreateSnapshotInput(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotClass, *v1.PersistentVolume, string, *v1.SecretReference, error) {
	className := snapshot.Spec.VolumeSnapshotClassName
	klog.V(5).Infof("getCreateSnapshotInput [%s]: VolumeSnapshotClassName [%s]", snapshot.Name, *className)
	var class *crdv1.VolumeSnapshotClass
	var err error
	if className != nil {
		class, err = ctrl.getSnapshotClass(*className)
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
	contentName := utils.GetSnapshotContentNameForSnapshot(snapshot)

	// Resolve snapshotting secret credentials.
	snapshotterSecretRef, err := utils.GetSecretReference(class.Parameters, contentName, snapshot)
	if err != nil {
		return nil, nil, "", nil, err
	}

	return class, volume, contentName, snapshotterSecretRef, nil
}

func (ctrl *csiSnapshotSideCarController) checkandUpdateBoundSnapshotStatusOperation(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshot, error) {
	var err error
	var creationTime time.Time
	var size int64
	var readyToUse = false
	var driverName string
	var snapshotID string

	if snapshot.Spec.Source.PersistentVolumeClaimName == nil && snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		klog.V(5).Infof("checkandUpdateBoundSnapshotStatusOperation: checking whether snapshot [%s] is pre-bound to content [%s]", snapshot.Name, content.Name)
		readyToUse, creationTime, size, err = ctrl.handler.GetSnapshotStatus(content)
		if err != nil {
			klog.Errorf("checkandUpdateBoundSnapshotStatusOperation: failed to call get snapshot status to check whether snapshot is ready to use %q", err)
			return nil, err
		}
		driverName = content.Spec.Driver
		if content.Spec.Source.SnapshotHandle != nil {
			snapshotID = *content.Spec.Source.SnapshotHandle
		}
	} else if snapshot.Spec.Source.PersistentVolumeClaimName != nil {
		class, volume, _, snapshotterSecretRef, err := ctrl.getCreateSnapshotInput(snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to get input parameters to create snapshot %s: %q", snapshot.Name, err)
		}

		snapshotterCredentials, err := utils.GetCredentials(ctrl.client, snapshotterSecretRef)
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
	newSnapshot, err := ctrl.updateSnapshotStatus(snapshot, content.Name, readyToUse, creationTime, size, utils.IsSnapshotBound(snapshot, content))
	if err != nil {
		return nil, err
	}
	//err = ctrl.updateSnapshotContentSize(content, size)
	_, err = ctrl.updateSnapshotContentStatus(content, snapshotID, readyToUse, creationTime.UnixNano(), size)
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
func (ctrl *csiSnapshotSideCarController) createSnapshotOperation(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	klog.Infof("createSnapshot: Creating snapshot %s through the plugin ...", utils.SnapshotKey(snapshot))

	if snapshot.Status.Error != nil && !isControllerUpdateFailError(snapshot.Status.Error) {
		klog.V(4).Infof("error is already set in snapshot, do not retry to create: %s", snapshot.Status.Error.Message)
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

	snapshotterCredentials, err := utils.GetCredentials(ctrl.client, snapshotterSecretRef)
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
		klog.V(5).Infof("createSnapshot [%s]: trying to update snapshot creation timestamp", utils.SnapshotKey(snapshot))
		newSnapshot, err = ctrl.updateSnapshotStatus(snapshot, contentName, readyToUse, creationTime, size, false)
		if err == nil {
			break
		}
		klog.V(4).Infof("failed to update snapshot %s creation timestamp: %v", utils.SnapshotKey(snapshot), err)
	}

	if err != nil {
		return nil, err
	}

	// Create VolumeSnapshotContent in the database
	if volume.Spec.CSI == nil {
		return nil, fmt.Errorf("cannot find CSI PersistentVolumeSource for volume %s", volume.Name)
	}
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
			Source: crdv1.VolumeSnapshotContentSource{
				VolumeHandle: &volume.Spec.CSI.VolumeHandle,
				// TODO(xyang): Handle SnapshotHandle for pre-provisioned snapshot
				//SnapshotHandle: snapshotID,
			},
			SnapshotClassName: &(class.Name),
			DeletionPolicy:    class.DeletionPolicy,
			Driver:            driverName,
		},
	}

	// Set AnnDeletionSecretRefName and AnnDeletionSecretRefNamespace
	if snapshotterSecretRef != nil {
		klog.V(5).Infof("createSnapshotOperation: set annotation [%s] on content [%s].", utils.AnnDeletionSecretRefName, snapshotContent.Name)
		metav1.SetMetaDataAnnotation(&snapshotContent.ObjectMeta, utils.AnnDeletionSecretRefName, snapshotterSecretRef.Name)

		klog.V(5).Infof("syncContent: set annotation [%s] on content [%s].", utils.AnnDeletionSecretRefNamespace, snapshotContent.Name)
		metav1.SetMetaDataAnnotation(&snapshotContent.ObjectMeta, utils.AnnDeletionSecretRefNamespace, snapshotterSecretRef.Namespace)
	}

	// Set AnnDynamicallyProvisioned if it is not set yet
	if !metav1.HasAnnotation(snapshotContent.ObjectMeta, utils.AnnDynamicallyProvisioned) {
		klog.V(5).Infof("createSnapshotOperation: Set annotation [%s:%s] on snapshot content %s", utils.AnnDynamicallyProvisioned, ctrl.snapshotterName, snapshotContent.Name)
		metav1.SetMetaDataAnnotation(&snapshotContent.ObjectMeta, utils.AnnDynamicallyProvisioned, ctrl.snapshotterName)
	}

	var updateContent *crdv1.VolumeSnapshotContent
	klog.V(3).Infof("volume snapshot content %#v", snapshotContent)
	// Try to create the VolumeSnapshotContent object several times
	for i := 0; i < ctrl.createSnapshotContentRetryCount; i++ {
		klog.V(5).Infof("createSnapshot [%s]: trying to save volume snapshot content %s", utils.SnapshotKey(snapshot), snapshotContent.Name)
		if updateContent, err = ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Create(snapshotContent); err == nil || apierrs.IsAlreadyExists(err) {
			// Save succeeded.
			if err != nil {
				klog.V(3).Infof("volume snapshot content %q for snapshot %q already exists, reusing", snapshotContent.Name, utils.SnapshotKey(snapshot))
				err = nil
				updateContent = snapshotContent
			} else {
				klog.V(3).Infof("volume snapshot content %q for snapshot %q saved, %v", snapshotContent.Name, utils.SnapshotKey(snapshot), snapshotContent)
			}
			break
		}
		// Save failed, try again after a while.
		klog.V(3).Infof("failed to save volume snapshot content %q for snapshot %q: %v", snapshotContent.Name, utils.SnapshotKey(snapshot), err)
		time.Sleep(ctrl.createSnapshotContentInterval)
	}

	if err != nil {
		// Save failed. Now we have a snapshot asset outside of Kubernetes,
		// but we don't have appropriate volumesnapshot content object for it.
		// Emit some event here and controller should try to create the content in next sync period.
		strerr := fmt.Sprintf("Error creating volume snapshot content object for snapshot %s: %v.", utils.SnapshotKey(snapshot), err)
		klog.Error(strerr)
		ctrl.eventRecorder.Event(newSnapshot, v1.EventTypeWarning, "CreateSnapshotContentFailed", strerr)
		return nil, newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
	}

	newContent, err := ctrl.updateSnapshotContentStatus(updateContent, snapshotID, readyToUse, timestamp, size)
	if err != nil {
		strerr := fmt.Sprintf("error updating volume snapshot content status for snapshot %s: %v.", utils.SnapshotKey(snapshot), err)
		klog.Error(strerr)
	} else {
		updateContent = newContent

		// Update snapshot status with ReadyToUse
		for i := 0; i < ctrl.createSnapshotContentRetryCount; i++ {
			klog.V(5).Infof("createSnapshot [%s]: trying to update snapshot status readyToUse", utils.SnapshotKey(snapshot))
			if updateContent.Status.ReadyToUse == nil || (updateContent.Status.ReadyToUse != nil && *updateContent.Status.ReadyToUse == false) {
				break
			}
			newSnapshot, err = ctrl.updateSnapshotStatus(snapshot, updateContent.Name, *updateContent.Status.ReadyToUse, creationTime, size, utils.IsSnapshotBound(newSnapshot, updateContent))
			if err == nil {
				break
			}
			klog.V(4).Infof("failed to update snapshot %s creation timestamp: %v", utils.SnapshotKey(snapshot), err)
		}
	}

	// Add new content to the cache store
	_, err = ctrl.storeContentUpdate(updateContent)
	if err != nil {
		klog.Errorf("failed to update content store %v", err)
	}

	return newSnapshot, nil
}

// Delete a snapshot
// 1. Find the SnapshotContent corresponding to Snapshot
//   1a: Not found => finish (it's been deleted already)
// 2. Ask the backend to remove the snapshot device
// 3. Delete the SnapshotContent object
// 4. Remove the Snapshot from store
// 5. Finish
func (ctrl *csiSnapshotSideCarController) deleteSnapshotContentOperation(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("deleteSnapshotOperation [%s] started", content.Name)

	// get secrets if VolumeSnapshotClass specifies it
	var snapshotterCredentials map[string]string
	var err error

	// Check if annotation exists
	if metav1.HasAnnotation(content.ObjectMeta, utils.AnnDeletionSecretRefName) && metav1.HasAnnotation(content.ObjectMeta, utils.AnnDeletionSecretRefNamespace) {
		annDeletionSecretName := content.Annotations[utils.AnnDeletionSecretRefName]
		annDeletionSecretNamespace := content.Annotations[utils.AnnDeletionSecretRefNamespace]

		snapshotterSecretRef := &v1.SecretReference{}

		if annDeletionSecretName == "" || annDeletionSecretNamespace == "" {
			return fmt.Errorf("cannot delete snapshot %#v, err: secret name or namespace not specified", content.Name)
		}

		snapshotterSecretRef.Name = annDeletionSecretName
		snapshotterSecretRef.Namespace = annDeletionSecretNamespace

		snapshotterCredentials, err = utils.GetCredentials(ctrl.client, snapshotterSecretRef)
		if err != nil {
			// Continue with deletion, as the secret may have already been deleted.
			klog.Errorf("Failed to get credentials for snapshot %s: %s", content.Name, err.Error())
		}
	}

	err = ctrl.handler.DeleteSnapshot(content, snapshotterCredentials)
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

// updateSnapshotContentSize update the restore size for snapshot content
func (ctrl *csiSnapshotSideCarController) updateSnapshotContentSize(content *crdv1.VolumeSnapshotContent, size int64) error {
	if size <= 0 {
		return nil
	}
	if content.Status.RestoreSize != nil && *content.Status.RestoreSize == size {
		return nil
	}
	contentClone := content.DeepCopy()
	contentClone.Status.RestoreSize = &size
	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Update(contentClone)
	if err != nil {
		return newControllerUpdateError(content.Name, err.Error())
	}

	_, err = ctrl.storeContentUpdate(contentClone)
	if err != nil {
		klog.Errorf("failed to update content store %v", err)
	}

	return nil
}

// UpdateSnapshotStatus converts snapshot status to crdv1.VolumeSnapshotCondition
func (ctrl *csiSnapshotSideCarController) updateSnapshotStatus(snapshot *crdv1.VolumeSnapshot, boundContentName string, readyToUse bool, createdAt time.Time, size int64, bound bool) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("updating VolumeSnapshot [%s], readyToUse %v, timestamp %v, bound %v, boundContentName %v", utils.SnapshotKey(snapshot), readyToUse, createdAt, bound, boundContentName)

	snapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshot.Namespace).Get(snapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot %s from api server: %v", utils.SnapshotKey(snapshot), err)
	}

	status := snapshotObj.Status
	change := false
	timeAt := &metav1.Time{
		Time: createdAt,
	}

	snapshotClone := snapshotObj.DeepCopy()
	if readyToUse {
		if bound {
			status.ReadyToUse = &readyToUse
			// Remove the error if checking snapshot is already bound and ready
			status.Error = nil
			change = true
		}
	}
	if status.CreationTime == nil {
		status.CreationTime = timeAt
		change = true
	}

	if status.BoundVolumeSnapshotContentName == nil || (status.BoundVolumeSnapshotContentName != nil && *status.BoundVolumeSnapshotContentName == "") {
		status.BoundVolumeSnapshotContentName = &boundContentName
		change = true
	}

	if change {
		if size > 0 {
			status.RestoreSize = resource.NewQuantity(size, resource.BinarySI)
		}
		snapshotClone.Status = status
		newSnapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(snapshotClone)
		if err != nil {
			return nil, newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
		}
		return newSnapshotObj, nil

	}
	return snapshotObj, nil
}

func (ctrl *csiSnapshotSideCarController) updateSnapshotContentStatus(
	content *crdv1.VolumeSnapshotContent,
	snapshotHandle string,
	readyToUse bool,
	createdAt int64,
	size int64) (*crdv1.VolumeSnapshotContent, error) {

	klog.V(5).Infof("updating VolumeSnapshotContent [%s], snapshotHandle %s, readyToUse %v, createdAt %v, size %d", content.Name, snapshotHandle, readyToUse, createdAt, size)

	contentObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Get(content.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot content %s from api server: %v", content.Name, err)
	}

	currentStatus := contentObj.Status
	updated := false
	if currentStatus.SnapshotHandle == nil {
		currentStatus.SnapshotHandle = &snapshotHandle
		updated = true
	}
	if currentStatus.ReadyToUse == nil || (currentStatus.ReadyToUse != nil && *currentStatus.ReadyToUse != readyToUse) {
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
		contentClone := contentObj.DeepCopy()
		contentClone.Status = currentStatus
		newContent, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().UpdateStatus(contentClone)
		if err != nil {
			return nil, newControllerUpdateError(content.Name, err.Error())
		}
		return newContent, nil
	}
	return contentObj, nil
}

// getVolumeFromVolumeSnapshot is a helper function to get PV from VolumeSnapshot.
func (ctrl *csiSnapshotSideCarController) getVolumeFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolume, error) {
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

// getSnapshotClass is a helper function to get snapshot class from the class name.
func (ctrl *csiSnapshotSideCarController) getSnapshotClass(className string) (*crdv1.VolumeSnapshotClass, error) {
	klog.V(5).Infof("getSnapshotClass: VolumeSnapshotClassName [%s]", className)

	class, err := ctrl.classLister.Get(className)
	if err != nil {
		klog.Errorf("failed to retrieve snapshot class %s from the informer: %q", className, err)
		return nil, fmt.Errorf("failed to retrieve snapshot class %s from the informer: %q", className, err)
	}

	return class, nil
}

// getClaimFromVolumeSnapshot is a helper function to get PVC from VolumeSnapshot.
func (ctrl *csiSnapshotSideCarController) getClaimFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolumeClaim, error) {
	//if snapshot.Spec.Source == nil {
	//	return nil, fmt.Errorf("the snapshot source is not specified")
	//}
	//if snapshot.Spec.Source.Kind != pvcKind {
	//	return nil, fmt.Errorf("the snapshot source is not the right type. Expected %s, Got %v", pvcKind, snapshot.Spec.Source.Kind)
	//}
	pvcName := *snapshot.Spec.Source.PersistentVolumeClaimName
	if pvcName == "" {
		return nil, fmt.Errorf("the PVC name is not specified in snapshot %s", utils.SnapshotKey(snapshot))
	}
	//if snapshot.Spec.Source.APIGroup != nil && *(snapshot.Spec.Source.APIGroup) != apiGroup {
	//	return nil, fmt.Errorf("the snapshot source does not have the right APIGroup. Expected empty string, Got %s", *(snapshot.Spec.Source.APIGroup))
	//}

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
	if err != nil {
		if strings.Contains(*err.Message, controllerUpdateFailMsg) {
			return true
		}
	}
	return false
}

// ensureSnapshotSourceFinalizer checks if a Finalizer needs to be added for the snapshot source;
// if true, adds a Finalizer for VolumeSnapshot Source PVC
func (ctrl *csiSnapshotSideCarController) ensureSnapshotSourceFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already.", snapshot.Name, err)
		return nil
	}

	// If PVC is not being deleted and PVCFinalizer is not added yet, the PVCFinalizer should be added.
	if pvc.ObjectMeta.DeletionTimestamp == nil && !slice.ContainsString(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer, nil) {
		// Add the finalizer
		pvcClone := pvc.DeepCopy()
		pvcClone.ObjectMeta.Finalizers = append(pvcClone.ObjectMeta.Finalizers, utils.PVCFinalizer)
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
func (ctrl *csiSnapshotSideCarController) removeSnapshotSourceFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already. No need to remove finalizer on the claim.", snapshot.Name, err)
		return nil
	}

	pvcClone := pvc.DeepCopy()
	pvcClone.ObjectMeta.Finalizers = slice.RemoveString(pvcClone.ObjectMeta.Finalizers, utils.PVCFinalizer, nil)

	_, err = ctrl.client.CoreV1().PersistentVolumeClaims(pvcClone.Namespace).Update(pvcClone)
	if err != nil {
		return newControllerUpdateError(pvcClone.Name, err.Error())
	}

	klog.V(5).Infof("Removed protection finalizer from persistent volume claim %s", pvc.Name)
	return nil
}

// isSnapshotSourceBeingUsed checks if a PVC is being used as a source to create a snapshot
func (ctrl *csiSnapshotSideCarController) isSnapshotSourceBeingUsed(snapshot *crdv1.VolumeSnapshot) bool {
	klog.V(5).Infof("isSnapshotSourceBeingUsed[%s]: started", utils.SnapshotKey(snapshot))
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
		if snap.Spec.Source.PersistentVolumeClaimName == nil && snap.Spec.Source.VolumeSnapshotContentName == nil {
			klog.V(4).Infof("Skipping static bound snapshot %s when checking PVC %s/%s", snap.Name, pvc.Namespace, pvc.Name)
			continue
		}
		if snap.Spec.Source.PersistentVolumeClaimName != nil && pvc.Name == *snap.Spec.Source.PersistentVolumeClaimName && (snap.Status.ReadyToUse == nil || (snap.Status.ReadyToUse != nil && *snap.Status.ReadyToUse == false)) {
			klog.V(2).Infof("Keeping PVC %s/%s, it is used by snapshot %s/%s", pvc.Namespace, pvc.Name, snap.Namespace, snap.Name)
			return true
		}
	}

	klog.V(5).Infof("isSnapshotSourceBeingUsed: no snapshot is being created from PVC %s/%s", pvc.Namespace, pvc.Name)
	return false
}

// checkandRemoveSnapshotSourceFinalizer checks if the snapshot source finalizer should be removed
// and removed it if needed.
func (ctrl *csiSnapshotSideCarController) checkandRemoveSnapshotSourceFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already. No need to remove finalizer on the claim.", snapshot.Name, err)
		return nil
	}

	klog.V(5).Infof("checkandRemoveSnapshotSourceFinalizer for snapshot [%s]: snapshot status [%#v]", snapshot.Name, snapshot.Status)

	// Check if there is a Finalizer on PVC to be removed
	if slice.ContainsString(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer, nil) {
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

// shouldProvision returns whether a VolumeSnapshot should have a
// VolumeSnapshotContent provisioned for it, i.e. whether a Provision is
// "desired"
func (ctrl *csiSnapshotSideCarController) shouldProvision(snapshot *crdv1.VolumeSnapshot) (bool, error) {
	// VolumeSnapshot is already bound with VolumeSnapshotContent
	if snapshot.Status.BoundVolumeSnapshotContentName != nil && *snapshot.Status.BoundVolumeSnapshotContentName != "" {
		return false, nil
	}

	_, err := ctrl.verifySnapshotClass(snapshot)
	if err != nil {
		// updateSnapshotErrorStatusWithEvent is already called in verifySnapshotClass
		return false, err
	}

	return true, nil
}
