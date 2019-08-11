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
	"fmt"
	"strings"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	"github.com/kubernetes-csi/external-snapshotter/pkg/utils"
	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storage "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
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
func (ctrl *csiSnapshotCommonController) syncContent(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("synchronizing VolumeSnapshotContent[%s]", content.Name)

	klog.V(5).Infof("syncContent: check if we should remove Finalizer for VolumeSnapshotContent[%s]", content.Name)
	// It is a deletion candidate if DeletionTimestamp is not nil and
	// VolumeSnapshotContentFinalizer is set.
	if utils.IsContentDeletionCandidate(content) {
		// Volume snapshot content is a deletion candidate. Check if it's
		// used and remove finalizer if it's not.
		// Check if snapshot content is still bound to a snapshot.
		klog.V(5).Infof("syncContent: Content [%s] is a deletion candidate. Check if it is bound to a snapshot.", content.Name)
		isUsed := ctrl.isSnapshotContentBeingUsed(content)
		if !isUsed {
			klog.V(5).Infof("syncContent: Remove Finalizer for VolumeSnapshotContent[%s]", content.Name)
			return ctrl.removeContentFinalizer(content)
		}
	}

	if utils.NeedToAddContentFinalizer(content) {
		// Content is not being deleted -> it should have the finalizer.
		klog.V(5).Infof("syncContent: Add Finalizer for VolumeSnapshotContent[%s]", content.Name)
		return ctrl.addContentFinalizer(content)
	}

	// VolumeSnapshotContent is not bound to any VolumeSnapshot, in this case we just return err
	if content.Spec.VolumeSnapshotRef == nil {
		// content is not bound
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: VolumeSnapshotContent is not bound to any VolumeSnapshot", content.Name)
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotContentNotBound", "VolumeSnapshotContent is not bound to any VolumeSnapshot")
		return fmt.Errorf("volumeSnapshotContent %s is not bound to any VolumeSnapshot", content.Name)
	}
	klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: content is bound to snapshot %s", content.Name, utils.SnapshotRefKey(content.Spec.VolumeSnapshotRef))
	// The VolumeSnapshotContent is reserved for a VolumeSnapshot;
	// that VolumeSnapshot has not yet been bound to this VolumeSnapshotContent; the VolumeSnapshot sync will handle it.
	if content.Spec.VolumeSnapshotRef.UID == "" {
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: VolumeSnapshotContent is pre-bound to VolumeSnapshot %s", content.Name, utils.SnapshotRefKey(content.Spec.VolumeSnapshotRef))
		return nil
	}
	// Get the VolumeSnapshot by _name_
	var snapshot *crdv1.VolumeSnapshot
	snapshotName := utils.SnapshotRefKey(content.Spec.VolumeSnapshotRef)
	obj, found, err := ctrl.snapshotStore.GetByKey(snapshotName)
	if err != nil {
		return err
	}
	if !found {
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: snapshot %s not found", content.Name, utils.SnapshotRefKey(content.Spec.VolumeSnapshotRef))
		// Fall through with snapshot = nil
	} else {
		var ok bool
		snapshot, ok = obj.(*crdv1.VolumeSnapshot)
		if !ok {
			return fmt.Errorf("cannot convert object from snapshot cache to snapshot %q!?: %#v", content.Name, obj)
		}
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: snapshot %s found", content.Name, utils.SnapshotRefKey(content.Spec.VolumeSnapshotRef))
	}
	if snapshot != nil && snapshot.UID != content.Spec.VolumeSnapshotRef.UID {
		// The snapshot that the content was pointing to was deleted, and another
		// with the same name created.
		klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: content %s has different UID, the old one must have been deleted", content.Name, utils.SnapshotRefKey(content.Spec.VolumeSnapshotRef))
		// Treat the content as bound to a missing snapshot.
		snapshot = nil
	}

	if snapshot == nil {
		if content.Spec.DeletionPolicy != nil && *content.Spec.DeletionPolicy != crdv1.VolumeSnapshotContentDelete {
			klog.V(5).Infof("syncContent: Content [%s] deletion policy [%s] is not delete.", content.Name, *content.Spec.DeletionPolicy)
			return nil
		} else if content.Spec.DeletionPolicy == nil {
			klog.V(5).Infof("syncContent: Content [%s] deletion policy [%s] is not set. Treat it as retain.", content.Name, *content.Spec.DeletionPolicy)
			return nil
		}

		if !metav1.HasAnnotation(content.ObjectMeta, utils.AnnDynamicallyProvisioned) {
			klog.V(5).Infof("syncContent: Content [%s] does not have annotation [%s].", content.Name, utils.AnnDynamicallyProvisioned)
			return nil
		}

		// Set AnnShouldDelete if it is not set yet
		if !metav1.HasAnnotation(content.ObjectMeta, utils.AnnShouldDelete) {
			klog.V(5).Infof("syncContent: set annotation [%s] on content [%s].", utils.AnnShouldDelete, content.Name)
			metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnShouldDelete, "yes")

			updateContent, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshotContents().Update(content)
			if err != nil {
				return newControllerUpdateError(content.Name, err.Error())
			}

			_, err = ctrl.storeContentUpdate(updateContent)
			if err != nil {
				klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status: cannot update internal cache %v", content.Name, err)
				return err
			}
			klog.V(5).Infof("syncContent: Xing: volume snapshot content %#v", content)
		}
	}

	return nil
}

// syncSnapshot is the main controller method to decide what to do with a snapshot.
// It's invoked by appropriate cache.Controller callbacks when a snapshot is
// created, updated or periodically synced. We do not differentiate between
// these events.
// For easier readability, it is split into syncUnreadySnapshot and syncReadySnapshot
func (ctrl *csiSnapshotCommonController) syncSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	klog.V(5).Infof("synchonizing VolumeSnapshot[%s]: %s", utils.SnapshotKey(snapshot), utils.GetSnapshotStatusForLogging(snapshot))

	if utils.IsSnapshotDeletionCandidate(snapshot) {
		// Volume snapshot should be deleted. Check if it's used
		// and remove finalizer if it's not.
		// Check if a volume is being created from snapshot.
		isUsed := ctrl.isVolumeBeingCreatedFromSnapshot(snapshot)
		if !isUsed {
			klog.V(5).Infof("syncSnapshot: Remove Finalizer for VolumeSnapshot[%s]", utils.SnapshotKey(snapshot))
			return ctrl.removeSnapshotFinalizer(snapshot)
		}
	}

	if utils.NeedToAddSnapshotFinalizer(snapshot) {
		// Snapshot is not being deleted -> it should have the finalizer.
		klog.V(5).Infof("syncSnapshot: Add Finalizer for VolumeSnapshot[%s]", utils.SnapshotKey(snapshot))
		return ctrl.addSnapshotFinalizer(snapshot)
	}

	klog.V(5).Infof("syncSnapshot[%s]: check if we should remove finalizer on snapshot source and remove it if we can", utils.SnapshotKey(snapshot))

	// TODO(xyang): Controller should not rely on ReadyToUse field
	// Also external-provisioner should not rely on RestoreSize in VolumeSnapshot
	// at restore time
	if !snapshot.Status.ReadyToUse {
		return ctrl.syncUnreadySnapshot(snapshot)
	}
	return ctrl.syncReadySnapshot(snapshot)
}

// syncReadySnapshot checks the snapshot which has been bound to snapshot content successfully before.
// If there is any problem with the binding (e.g., snapshot points to a non-exist snapshot content), update the snapshot status and emit event.
func (ctrl *csiSnapshotCommonController) syncReadySnapshot(snapshot *crdv1.VolumeSnapshot) error {
	if snapshot.Spec.SnapshotContentName == "" {
		if err := ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotLost", "Bound snapshot has lost reference to VolumeSnapshotContent"); err != nil {
			return err
		}
		return nil
	}
	obj, found, err := ctrl.contentStore.GetByKey(snapshot.Spec.SnapshotContentName)
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
			return fmt.Errorf("Cannot convert object from snapshot content store to VolumeSnapshotContent %q!?: %#v", snapshot.Spec.SnapshotContentName, obj)
		}

		klog.V(5).Infof("syncReadySnapshot[%s]: VolumeSnapshotContent %q found", utils.SnapshotKey(snapshot), content.Name)
		if !utils.IsSnapshotBound(snapshot, content) {
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
func (ctrl *csiSnapshotCommonController) syncUnreadySnapshot(snapshot *crdv1.VolumeSnapshot) error {
	uniqueSnapshotName := utils.SnapshotKey(snapshot)
	klog.V(5).Infof("syncUnreadySnapshot %s", uniqueSnapshotName)

	if snapshot.Spec.SnapshotContentName != "" {
		contentObj, found, err := ctrl.contentStore.GetByKey(snapshot.Spec.SnapshotContentName)
		if err != nil {
			return err
		}
		if !found {
			// snapshot is bound to a non-existing content.
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMissing", "VolumeSnapshotContent is missing")
			klog.V(4).Infof("synchronizing unready snapshot[%s]: snapshotcontent %q requested and not found, will try again next time", uniqueSnapshotName, snapshot.Spec.SnapshotContentName)
			return fmt.Errorf("snapshot %s is bound to a non-existing content %s", uniqueSnapshotName, snapshot.Spec.SnapshotContentName)
		}
		content, ok := contentObj.(*crdv1.VolumeSnapshotContent)
		if !ok {
			return fmt.Errorf("expected volume snapshot content, got %+v", contentObj)
		}
		_, err = ctrl.checkandBindSnapshotContent(snapshot, content)
		if err != nil {
			// snapshot is bound but content is not bound to snapshot correctly
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotBindFailed", fmt.Sprintf("Snapshot failed to bind VolumeSnapshotContent, %v", err))
			return fmt.Errorf("snapshot %s is bound, but VolumeSnapshotContent %s is not bound to the VolumeSnapshot correctly, %v", uniqueSnapshotName, content.Name, err)
		}
		return nil
	} else { // snapshot.Spec.SnapshotContentName == nil
		if contentObj := ctrl.getMatchSnapshotContent(snapshot); contentObj != nil {
			klog.V(5).Infof("Find VolumeSnapshotContent object %s for snapshot %s", contentObj.Name, uniqueSnapshotName)
			newSnapshot, err := ctrl.bindandUpdateVolumeSnapshot(contentObj, snapshot)
			if err != nil {
				return err
			}
			klog.V(5).Infof("bindandUpdateVolumeSnapshot %v", newSnapshot)
			return nil
		}
		return nil
	}
}

// getMatchSnapshotContent looks up VolumeSnapshotContent for a VolumeSnapshot named snapshotName
func (ctrl *csiSnapshotCommonController) getMatchSnapshotContent(snapshot *crdv1.VolumeSnapshot) *crdv1.VolumeSnapshotContent {
	var snapshotContentObj *crdv1.VolumeSnapshotContent
	var found bool

	objs := ctrl.contentStore.List()
	for _, obj := range objs {
		content := obj.(*crdv1.VolumeSnapshotContent)
		if content.Spec.VolumeSnapshotRef != nil &&
			content.Spec.VolumeSnapshotRef.Name == snapshot.Name &&
			content.Spec.VolumeSnapshotRef.Namespace == snapshot.Namespace &&
			content.Spec.VolumeSnapshotRef.UID == snapshot.UID &&
			content.Spec.VolumeSnapshotClassName != nil && snapshot.Spec.VolumeSnapshotClassName != nil &&
			*(content.Spec.VolumeSnapshotClassName) == *(snapshot.Spec.VolumeSnapshotClassName) {
			found = true
			snapshotContentObj = content
			break
		}
	}

	if !found {
		klog.V(4).Infof("No VolumeSnapshotContent for VolumeSnapshot %s found", utils.SnapshotKey(snapshot))
		return nil
	}

	return snapshotContentObj
}

func (ctrl *csiSnapshotCommonController) storeSnapshotUpdate(snapshot interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.snapshotStore, snapshot, "snapshot")
}

func (ctrl *csiSnapshotCommonController) storeContentUpdate(content interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.contentStore, content, "content")
}

// updateSnapshotStatusWithEvent saves new snapshot.Status to API server and emits
// given event on the snapshot. It saves the status and emits the event only when
// the status has actually changed from the version saved in API server.
// Parameters:
//   snapshot - snapshot to update
//   eventtype, reason, message - event to send, see EventRecorder.Event()
func (ctrl *csiSnapshotCommonController) updateSnapshotErrorStatusWithEvent(snapshot *crdv1.VolumeSnapshot, eventtype, reason, message string) error {
	klog.V(5).Infof("updateSnapshotStatusWithEvent[%s]", utils.SnapshotKey(snapshot))

	if snapshot.Status.Error != nil && snapshot.Status.Error.Message == message {
		klog.V(4).Infof("updateSnapshotStatusWithEvent[%s]: the same error %v is already set", snapshot.Name, snapshot.Status.Error)
		return nil
	}
	snapshotClone := snapshot.DeepCopy()
	statusError := &storage.VolumeError{
		Time: metav1.Time{
			Time: time.Now(),
		},
		Message: message,
	}
	snapshotClone.Status.Error = statusError
	snapshotClone.Status.ReadyToUse = false
	newSnapshot, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(snapshotClone)

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

// isSnapshotConentBeingUsed checks if snapshot content is bound to snapshot.
func (ctrl *csiSnapshotCommonController) isSnapshotContentBeingUsed(content *crdv1.VolumeSnapshotContent) bool {
	if content.Spec.VolumeSnapshotRef != nil {
		snapshotObj, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshots(content.Spec.VolumeSnapshotRef.Namespace).Get(content.Spec.VolumeSnapshotRef.Name, metav1.GetOptions{})
		if err != nil {
			klog.Infof("isSnapshotContentBeingUsed: Cannot get snapshot %s from api server: [%v]. VolumeSnapshot object may be deleted already.", content.Spec.VolumeSnapshotRef.Name, err)
			return false
		}

		// Check if the snapshot content is bound to the snapshot
		if utils.IsSnapshotBound(snapshotObj, content) && snapshotObj.Spec.SnapshotContentName == content.Name {
			klog.Infof("isSnapshotContentBeingUsed: VolumeSnapshot %s is bound to volumeSnapshotContent [%s]", snapshotObj.Name, content.Name)
			return true
		}
	}

	klog.V(5).Infof("isSnapshotContentBeingUsed: Snapshot content %s is not being used", content.Name)
	return false
}

// addContentFinalizer adds a Finalizer for VolumeSnapshotContent.
func (ctrl *csiSnapshotCommonController) addContentFinalizer(content *crdv1.VolumeSnapshotContent) error {
	contentClone := content.DeepCopy()
	contentClone.ObjectMeta.Finalizers = append(contentClone.ObjectMeta.Finalizers, utils.VolumeSnapshotContentFinalizer)

	_, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshotContents().Update(contentClone)
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
func (ctrl *csiSnapshotCommonController) removeContentFinalizer(content *crdv1.VolumeSnapshotContent) error {
	contentClone := content.DeepCopy()
	contentClone.ObjectMeta.Finalizers = slice.RemoveString(contentClone.ObjectMeta.Finalizers, utils.VolumeSnapshotContentFinalizer, nil)

	_, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshotContents().Update(contentClone)
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

// isVolumeBeingCreatedFromSnapshot checks if an volume is being created from the snapshot.
func (ctrl *csiSnapshotCommonController) isVolumeBeingCreatedFromSnapshot(snapshot *crdv1.VolumeSnapshot) bool {
	pvcList, err := ctrl.pvcLister.PersistentVolumeClaims(snapshot.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("Failed to retrieve PVCs from the lister to check if volume snapshot %s is being used by a volume: %q", utils.SnapshotKey(snapshot), err)
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
	klog.V(5).Infof("isVolumeBeingCreatedFromSnapshot: no volume is being created from snapshot %s", utils.SnapshotKey(snapshot))
	return false
}

// The function checks whether the volumeSnapshotRef in snapshot content matches the given snapshot. If match, it binds the content with the snapshot. This is for static binding where user has specified snapshot name but not UID of the snapshot in content.Spec.VolumeSnapshotRef.
func (ctrl *csiSnapshotCommonController) checkandBindSnapshotContent(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshotContent, error) {
	if content.Spec.VolumeSnapshotRef == nil || content.Spec.VolumeSnapshotRef.Name != snapshot.Name {
		return nil, fmt.Errorf("Could not bind snapshot %s and content %s, the VolumeSnapshotRef does not match", snapshot.Name, content.Name)
	} else if content.Spec.VolumeSnapshotRef.UID != "" && content.Spec.VolumeSnapshotRef.UID != snapshot.UID {
		return nil, fmt.Errorf("Could not bind snapshot %s and content %s, the VolumeSnapshotRef does not match", snapshot.Name, content.Name)
	} else if content.Spec.VolumeSnapshotRef.UID != "" && content.Spec.VolumeSnapshotClassName != nil {
		return content, nil
	}
	contentClone := content.DeepCopy()
	contentClone.Spec.VolumeSnapshotRef.UID = snapshot.UID
	className := *(snapshot.Spec.VolumeSnapshotClassName)
	contentClone.Spec.VolumeSnapshotClassName = &className
	newContent, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshotContents().Update(contentClone)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status failed %v", newContent.Name, err)
		return nil, err
	}

	// Set AnnBoundByController if it is not set yet
	if !metav1.HasAnnotation(contentClone.ObjectMeta, utils.AnnBoundByController) {
		metav1.SetMetaDataAnnotation(&contentClone.ObjectMeta, utils.AnnBoundByController, "yes")
	}

	_, err = ctrl.storeContentUpdate(newContent)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status: cannot update internal cache %v", newContent.Name, err)
		return nil, err
	}
	return newContent, nil
}

// This routine sets snapshot.Spec.SnapshotContentName
func (ctrl *csiSnapshotCommonController) bindandUpdateVolumeSnapshot(snapshotContent *crdv1.VolumeSnapshotContent, snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("bindandUpdateVolumeSnapshot for snapshot [%s]: snapshotContent [%s]", snapshot.Name, snapshotContent.Name)
	snapshotObj, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshots(snapshot.Namespace).Get(snapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot %s from api server: %v", utils.SnapshotKey(snapshot), err)
	}

	// Copy the snapshot object before updating it
	snapshotCopy := snapshotObj.DeepCopy()

	if snapshotObj.Spec.SnapshotContentName == snapshotContent.Name {
		klog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: VolumeSnapshot %s already bind to volumeSnapshotContent [%s]", snapshot.Name, snapshotContent.Name)
	} else {
		klog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: before bind VolumeSnapshot %s to volumeSnapshotContent [%s]", snapshot.Name, snapshotContent.Name)
		snapshotCopy.Spec.SnapshotContentName = snapshotContent.Name

		// Set AnnBoundByController if it is not set yet
		if !metav1.HasAnnotation(snapshotCopy.ObjectMeta, utils.AnnBoundByController) {
			metav1.SetMetaDataAnnotation(&snapshotCopy.ObjectMeta, utils.AnnBoundByController, "yes")
		}

		// Set AnnBindCompleted if it is not set yet
		if !metav1.HasAnnotation(snapshotCopy.ObjectMeta, utils.AnnBindCompleted) {
			metav1.SetMetaDataAnnotation(&snapshotCopy.ObjectMeta, utils.AnnBindCompleted, "yes")
			//dirty = true
		}

		updateSnapshot, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshots(snapshot.Namespace).Update(snapshotCopy)
		if err != nil {
			klog.Infof("bindVolumeSnapshotContentToVolumeSnapshot: Error binding VolumeSnapshot %s to volumeSnapshotContent [%s]. Error [%#v]", snapshot.Name, snapshotContent.Name, err)
			return nil, newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
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

// UpdateSnapshotStatus converts snapshot status to crdv1.VolumeSnapshotCondition
/*func (ctrl *csiSnapshotCommonController) updateSnapshotStatus(snapshot *crdv1.VolumeSnapshot, readyToUse bool, createdAt time.Time, size int64, bound bool) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("updating VolumeSnapshot[]%s, readyToUse %v, timestamp %v", utils.SnapshotKey(snapshot), readyToUse, createdAt)
	status := snapshot.Status
	change := false
	timeAt := &metav1.Time{
		Time: createdAt,
	}

	snapshotClone := snapshot.DeepCopy()
	if readyToUse {
		if bound {
			status.ReadyToUse = true
			// Remove the error if checking snapshot is already bound and ready
			status.Error = nil
			change = true
		}
	}
	if status.CreationTime == nil {
		status.CreationTime = timeAt
		change = true
	}

	if change {
		if size > 0 {
			status.RestoreSize = resource.NewQuantity(size, resource.BinarySI)
		}
		snapshotClone.Status = status
		newSnapshotObj, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(snapshotClone)
		if err != nil {
			return nil, newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
		}
		return newSnapshotObj, nil

	}
	return snapshot, nil
}
*/

// getVolumeFromVolumeSnapshot is a helper function to get PV from VolumeSnapshot.
func (ctrl *csiSnapshotCommonController) getVolumeFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolume, error) {
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

func (ctrl *csiSnapshotCommonController) getStorageClassFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*storagev1.StorageClass, error) {
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

// getSnapshotClass is a helper function to get snapshot class from the class name.
func (ctrl *csiSnapshotCommonController) getSnapshotClass(className string) (*crdv1.VolumeSnapshotClass, error) {
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
func (ctrl *csiSnapshotCommonController) SetDefaultSnapshotClass(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotClass, *crdv1.VolumeSnapshot, error) {
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
		if utils.IsDefaultAnnotation(class.ObjectMeta) && storageclass.Provisioner == class.Snapshotter { //&& ctrl.snapshotterName == class.Snapshotter {
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
	newSnapshot, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] default class failed %v", utils.SnapshotKey(snapshot), err)
	}
	_, updateErr := ctrl.storeSnapshotUpdate(newSnapshot)
	if updateErr != nil {
		// We will get an "snapshot update" event soon, this is not a big error
		klog.V(4).Infof("setDefaultSnapshotClass [%s]: cannot update internal cache: %v", utils.SnapshotKey(snapshot), updateErr)
	}

	return defaultClasses[0], newSnapshot, nil
}

// getClaimFromVolumeSnapshot is a helper function to get PVC from VolumeSnapshot.
func (ctrl *csiSnapshotCommonController) getClaimFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolumeClaim, error) {
	if snapshot.Spec.Source == nil {
		return nil, fmt.Errorf("the snapshot source is not specified")
	}
	if snapshot.Spec.Source.Kind != pvcKind {
		return nil, fmt.Errorf("the snapshot source is not the right type. Expected %s, Got %v", pvcKind, snapshot.Spec.Source.Kind)
	}
	pvcName := snapshot.Spec.Source.Name
	if pvcName == "" {
		return nil, fmt.Errorf("the PVC name is not specified in snapshot %s", utils.SnapshotKey(snapshot))
	}
	if snapshot.Spec.Source.APIGroup != nil && *(snapshot.Spec.Source.APIGroup) != apiGroup {
		return nil, fmt.Errorf("the snapshot source does not have the right APIGroup. Expected empty string, Got %s", *(snapshot.Spec.Source.APIGroup))
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

func isControllerUpdateFailError(err *storage.VolumeError) bool {
	if err != nil {
		if strings.Contains(err.Message, controllerUpdateFailMsg) {
			return true
		}
	}
	return false
}

// addSnapshotFinalizer adds a Finalizer for VolumeSnapshot.
func (ctrl *csiSnapshotCommonController) addSnapshotFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	snapshotClone := snapshot.DeepCopy()
	snapshotClone.ObjectMeta.Finalizers = append(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotFinalizer)
	_, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
	if err != nil {
		return newControllerUpdateError(snapshot.Name, err.Error())
	}

	_, err = ctrl.storeSnapshotUpdate(snapshotClone)
	if err != nil {
		klog.Errorf("failed to update snapshot store %v", err)
	}

	klog.V(5).Infof("Added protection finalizer to volume snapshot %s", utils.SnapshotKey(snapshot))
	return nil
}

// removeSnapshotFinalizer removes a Finalizer for VolumeSnapshot.
func (ctrl *csiSnapshotCommonController) removeSnapshotFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	snapshotClone := snapshot.DeepCopy()
	snapshotClone.ObjectMeta.Finalizers = slice.RemoveString(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotFinalizer, nil)

	_, err := ctrl.clientset.SnapshotV1alpha1().VolumeSnapshots(snapshotClone.Namespace).Update(snapshotClone)
	if err != nil {
		return newControllerUpdateError(snapshot.Name, err.Error())
	}

	_, err = ctrl.storeSnapshotUpdate(snapshotClone)
	if err != nil {
		klog.Errorf("failed to update snapshot store %v", err)
	}

	klog.V(5).Infof("Removed protection finalizer from volume snapshot %s", utils.SnapshotKey(snapshot))
	return nil
}
