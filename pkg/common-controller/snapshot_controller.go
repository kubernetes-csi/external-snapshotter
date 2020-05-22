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

package common_controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	ref "k8s.io/client-go/tools/reference"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/slice"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/v2/pkg/utils"
)

// ==================================================================
// PLEASE DO NOT ATTEMPT TO SIMPLIFY THIS CODE.
// KEEP THE SPACE SHUTTLE FLYING.
// ==================================================================

// Design:
//
// The fundamental key to this design is the bi-directional "pointer" between
// VolumeSnapshots and VolumeSnapshotContents, which is represented here
// as snapshot.Status.BoundVolumeSnapshotContentName and content.Spec.VolumeSnapshotRef.
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
// The snapshot controller is split into two controllers in its beta phase: a
// common controller that is deployed on the kubernetes master node and a sidecar
// controller that is deployed with the CSI driver.

// The dynamic snapshot creation is multi-step process: first snapshot controller
// creates snapshot content object, then the snapshot sidecar triggers snapshot
// creation though csi volume driver and updates snapshot content status with
// snapshotHandle, creationTime, restoreSize, readyToUse, and error fields. The
// snapshot controller updates snapshot status based on content status until
// bi-directional binding is complete and readyToUse becomes true. Error field
// in the snapshot status will be updated accordingly when failure occurs.

const snapshotKind = "VolumeSnapshot"
const snapshotAPIGroup = crdv1.GroupName

const controllerUpdateFailMsg = "snapshot controller failed to update"

// syncContent deals with one key off the queue
func (ctrl *csiSnapshotCommonController) syncContent(content *crdv1.VolumeSnapshotContent) error {
	snapshotName := utils.SnapshotRefKey(&content.Spec.VolumeSnapshotRef)
	klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: content is bound to snapshot %s", content.Name, snapshotName)
	// TODO(xiangqian): Putting this check in controller as webhook has not been implemented
	//                   yet. Remove the source checking once issue #187 is resolved:
	//                   https://github.com/kubernetes-csi/external-snapshotter/issues/187
	if (content.Spec.Source.VolumeHandle == nil && content.Spec.Source.SnapshotHandle == nil) ||
		(content.Spec.Source.VolumeHandle != nil && content.Spec.Source.SnapshotHandle != nil) {
		err := fmt.Errorf("Exactly one of VolumeHandle and SnapshotHandle should be specified")
		klog.Errorf("syncContent[%s]: validation error, %s", content.Name, err.Error())
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "ContentValidationError", err.Error())
		return err
	}

	// The VolumeSnapshotContent is reserved for a VolumeSnapshot;
	// that VolumeSnapshot has not yet been bound to this VolumeSnapshotContent;
	// syncSnapshot will handle it.
	if content.Spec.VolumeSnapshotRef.UID == "" {
		klog.V(4).Infof("syncContent [%s]: VolumeSnapshotContent is pre-bound to VolumeSnapshot %s", content.Name, snapshotName)
		return nil
	}

	if utils.NeedToAddContentFinalizer(content) {
		// Content is not being deleted -> it should have the finalizer.
		klog.V(5).Infof("syncContent [%s]: Add Finalizer for VolumeSnapshotContent", content.Name)
		return ctrl.addContentFinalizer(content)
	}

	// Check if snapshot exists in cache store
	// If getSnapshotFromStore returns (nil, nil), it means snapshot not found
	// and it may have already been deleted, and it will fall into the
	// snapshot == nil case below
	var snapshot *crdv1.VolumeSnapshot
	snapshot, err := ctrl.getSnapshotFromStore(snapshotName)
	if err != nil {
		return err
	}

	if snapshot != nil && snapshot.UID != content.Spec.VolumeSnapshotRef.UID {
		// The snapshot that the content was pointing to was deleted, and another
		// with the same name created.
		klog.V(4).Infof("syncContent [%s]: snapshot %s has different UID, the old one must have been deleted", content.Name, snapshotName)
		// Treat the content as bound to a missing snapshot.
		snapshot = nil
	} else {
		// Check if snapshot.Status is different from content.Status and add snapshot to queue
		// if there is a difference and it is worth triggering an snapshot status update.
		if snapshot != nil && ctrl.needsUpdateSnapshotStatus(snapshot, content) {
			klog.V(4).Infof("synchronizing VolumeSnapshotContent for snapshot [%s]: update snapshot status to true if needed.", snapshotName)
			// Manually trigger a snapshot status update to happen
			// right away so that it is in-sync with the content status
			ctrl.snapshotQueue.Add(snapshotName)
		}
	}

	// NOTE(xyang): Do not trigger content deletion if
	// snapshot is nil. This is to avoid data loss if
	// the user copied the yaml files and expect it to work
	// in a different setup. In this case snapshot is nil.
	// If we trigger content deletion, it will delete
	// physical snapshot resource on the storage system
	// and result in data loss!
	//
	// Trigger content deletion if snapshot is not nil
	// and snapshot has deletion timestamp.
	// If snapshot has deletion timestamp and finalizers, set
	// AnnVolumeSnapshotBeingDeleted annotation on the content.
	// This may trigger the deletion of the content in the
	// sidecar controller depending on the deletion policy
	// on the content.
	// Snapshot won't be deleted until content is deleted
	// due to the finalizer.
	if snapshot != nil && utils.IsSnapshotDeletionCandidate(snapshot) {
		return ctrl.setAnnVolumeSnapshotBeingDeleted(content)
	}

	return nil
}

// syncSnapshot is the main controller method to decide what to do with a snapshot.
// It's invoked by appropriate cache.Controller callbacks when a snapshot is
// created, updated or periodically synced. We do not differentiate between
// these events.
// For easier readability, it is split into syncUnreadySnapshot and syncReadySnapshot
func (ctrl *csiSnapshotCommonController) syncSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	klog.V(5).Infof("synchronizing VolumeSnapshot[%s]: %s", utils.SnapshotKey(snapshot), utils.GetSnapshotStatusForLogging(snapshot))
	klog.V(5).Infof("syncSnapshot [%s]: check if we should remove finalizer on snapshot PVC source and remove it if we can", utils.SnapshotKey(snapshot))

	// Check if we should remove finalizer on PVC and remove it if we can.
	if err := ctrl.checkandRemovePVCFinalizer(snapshot); err != nil {
		klog.Errorf("error check and remove PVC finalizer for snapshot [%s]: %v", snapshot.Name, err)
		// Log an event and keep the original error from checkandRemovePVCFinalizer
		ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "ErrorPVCFinalizer", "Error check and remove PVC Finalizer for VolumeSnapshot")
	}

	// Proceed with snapshot deletion only if snapshot is not in the middled of being
	// created from a PVC with a finalizer. This is to ensure that the PVC finalizer
	// can be removed even if a delete snapshot request is received before create
	// snapshot has completed.
	if snapshot.ObjectMeta.DeletionTimestamp != nil && !ctrl.isPVCwithFinalizerInUseByCurrentSnapshot(snapshot) {
		return ctrl.processSnapshotWithDeletionTimestamp(snapshot)
	}

	// TODO(xiangqian@): Putting this check in controller as webhook has not been implemented
	//                   yet. Remove the source checking once issue #187 is resolved:
	//                   https://github.com/kubernetes-csi/external-snapshotter/issues/187
	klog.V(5).Infof("syncSnapshot[%s]: validate snapshot to make sure source has been correctly specified", utils.SnapshotKey(snapshot))
	if (snapshot.Spec.Source.PersistentVolumeClaimName == nil && snapshot.Spec.Source.VolumeSnapshotContentName == nil) ||
		(snapshot.Spec.Source.PersistentVolumeClaimName != nil && snapshot.Spec.Source.VolumeSnapshotContentName != nil) {
		err := fmt.Errorf("Exactly one of PersistentVolumeClaimName and VolumeSnapshotContentName should be specified")
		klog.Errorf("syncSnapshot[%s]: validation error, %s", utils.SnapshotKey(snapshot), err.Error())
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotValidationError", err.Error())
		return err
	}

	klog.V(5).Infof("syncSnapshot[%s]: check if we should add finalizers on snapshot", utils.SnapshotKey(snapshot))
	if err := ctrl.checkandAddSnapshotFinalizers(snapshot); err != nil {
		klog.Errorf("error check and add Snapshot finalizers for snapshot [%s]: %v", snapshot.Name, err)
		ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "SnapshotFinalizerError", fmt.Sprintf("Failed to check and update snapshot: %s", err.Error()))
		return err
	}
	// Need to build or update snapshot.Status in following cases:
	// 1) snapshot.Status is nil
	// 2) snapshot.Status.ReadyToUse is false
	// 3) snapshot.Status.BoundVolumeSnapshotContentName is not set
	if !utils.IsSnapshotReady(snapshot) || !utils.IsBoundVolumeSnapshotContentNameSet(snapshot) {
		return ctrl.syncUnreadySnapshot(snapshot)
	}
	return ctrl.syncReadySnapshot(snapshot)
}

// Check if PVC has a finalizer and if it is being used by the current snapshot as source.
func (ctrl *csiSnapshotCommonController) isPVCwithFinalizerInUseByCurrentSnapshot(snapshot *crdv1.VolumeSnapshot) bool {
	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already.", snapshot.Name, err)
		return false
	}

	// Check if there is a Finalizer on PVC. If not, return false
	if !slice.ContainsString(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer, nil) {
		return false
	}

	if !utils.IsSnapshotReady(snapshot) {
		klog.V(2).Infof("PVC %s/%s is being used by snapshot %s/%s as source", pvc.Namespace, pvc.Name, snapshot.Namespace, snapshot.Name)
		return true
	}
	return false
}

// processSnapshotWithDeletionTimestamp processes finalizers and deletes the content when appropriate. It has the following steps:
// 1. Get the content which the to-be-deleted VolumeSnapshot points to and verifies bi-directional binding.
// 2. Call checkandRemoveSnapshotFinalizersAndCheckandDeleteContent() with information obtained from step 1. This function name is very long but the name suggests what it does. It determines whether to remove finalizers on snapshot and whether to delete content.
func (ctrl *csiSnapshotCommonController) processSnapshotWithDeletionTimestamp(snapshot *crdv1.VolumeSnapshot) error {
	klog.V(5).Infof("processSnapshotWithDeletionTimestamp VolumeSnapshot[%s]: %s", utils.SnapshotKey(snapshot), utils.GetSnapshotStatusForLogging(snapshot))

	var contentName string
	if snapshot.Status != nil && snapshot.Status.BoundVolumeSnapshotContentName != nil {
		contentName = *snapshot.Status.BoundVolumeSnapshotContentName
	}
	// for a dynamically created snapshot, it's possible that a content has been created
	// however the Status of the snapshot has not been updated yet, i.e., failed right
	// after content creation. In this case, use the fixed naming scheme to get the content
	// name and search
	if contentName == "" && snapshot.Spec.Source.PersistentVolumeClaimName != nil {
		contentName = utils.GetDynamicSnapshotContentNameForSnapshot(snapshot)
	}
	// find a content from cache store, note that it's complete legit that no
	// content has been found from content cache store
	content, err := ctrl.getContentFromStore(contentName)
	if err != nil {
		return err
	}
	// check whether the content points back to the passed in snapshot, note that
	// binding should always be bi-directional to trigger the deletion on content
	// or adding any annotation to the content
	var deleteContent bool
	if content != nil && utils.IsVolumeSnapshotRefSet(snapshot, content) {
		// content points back to snapshot, whether or not to delete a content now
		// depends on the deletion policy of it.
		deleteContent = (content.Spec.DeletionPolicy == crdv1.VolumeSnapshotContentDelete)
	} else {
		// the content is nil or points to a different snapshot, reset content to nil
		// such that there is no operation done on the found content in
		// checkandRemoveSnapshotFinalizersAndCheckandDeleteContent
		content = nil
	}

	klog.V(5).Infof("processSnapshotWithDeletionTimestamp[%s]: delete snapshot content and remove finalizer from snapshot if needed", utils.SnapshotKey(snapshot))
	return ctrl.checkandRemoveSnapshotFinalizersAndCheckandDeleteContent(snapshot, content, deleteContent)
}

// checkandRemoveSnapshotFinalizersAndCheckandDeleteContent deletes the content and removes snapshot finalizers (VolumeSnapshotAsSourceFinalizer and VolumeSnapshotBoundFinalizer) if needed
func (ctrl *csiSnapshotCommonController) checkandRemoveSnapshotFinalizersAndCheckandDeleteContent(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent, deleteContent bool) error {
	klog.V(5).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent VolumeSnapshot[%s]: %s", utils.SnapshotKey(snapshot), utils.GetSnapshotStatusForLogging(snapshot))

	if !utils.IsSnapshotDeletionCandidate(snapshot) {
		return nil
	}

	// check if the snapshot is being used for restore a PVC, if yes, do nothing
	// and wait until PVC restoration finishes
	if content != nil && ctrl.isVolumeBeingCreatedFromSnapshot(snapshot) {
		klog.V(4).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent[%s]: snapshot is being used to restore a PVC", utils.SnapshotKey(snapshot))
		ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "SnapshotDeletePending", "Snapshot is being used to restore a PVC")
		// TODO(@xiangqian): should requeue this?
		return nil
	}

	// regardless of the deletion policy, set the VolumeSnapshotBeingDeleted on
	// content object, this is to allow snapshotter sidecar controller to conduct
	// a delete operation whenever the content has deletion timestamp set.
	if content != nil {
		klog.V(5).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent[%s]: Set VolumeSnapshotBeingDeleted annotation on the content [%s]", utils.SnapshotKey(snapshot), content.Name)
		if err := ctrl.setAnnVolumeSnapshotBeingDeleted(content); err != nil {
			klog.V(4).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent[%s]: failed to set VolumeSnapshotBeingDeleted annotation on the content [%s]", utils.SnapshotKey(snapshot), content.Name)
			return err
		}
	}

	// VolumeSnapshot should be deleted. Check and remove finalizers
	// If content exists and has a deletion policy of Delete, set DeletionTimeStamp on the content;
	// content won't be deleted immediately due to the VolumeSnapshotContentFinalizer
	if content != nil && deleteContent {
		klog.V(5).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent: set DeletionTimeStamp on content [%s].", content.Name)
		err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Delete(context.TODO(), content.Name, metav1.DeleteOptions{})
		if err != nil {
			ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "SnapshotContentObjectDeleteError", "Failed to delete snapshot content API object")
			return fmt.Errorf("failed to delete VolumeSnapshotContent %s from API server: %q", content.Name, err)
		}
	}

	klog.V(5).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent: Remove Finalizer for VolumeSnapshot[%s]", utils.SnapshotKey(snapshot))
	// remove finalizers on the VolumeSnapshot object, there are two finalizers:
	// 1. VolumeSnapshotAsSourceFinalizer, once reached here, the snapshot is not
	//    in use to restore PVC, and the finalizer will be removed directly.
	// 2. VolumeSnapshotBoundFinalizer:
	//    a. If there is no content found, remove the finalizer.
	//    b. If the content is being deleted, i.e., with deleteContent == true,
	//       keep this finalizer until the content object is removed from API server
	//       by snapshot sidecar controller.
	//    c. If deletion will not cascade to the content, remove the finalizer on
	//       the snapshot such that it can be removed from API server.
	removeBoundFinalizer := !(content != nil && deleteContent)
	return ctrl.removeSnapshotFinalizer(snapshot, true, removeBoundFinalizer)
}

// checkandAddSnapshotFinalizers checks and adds snapshot finailzers when needed
func (ctrl *csiSnapshotCommonController) checkandAddSnapshotFinalizers(snapshot *crdv1.VolumeSnapshot) error {
	// get the content for this Snapshot
	var (
		content *crdv1.VolumeSnapshotContent
		err     error
	)
	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		content, err = ctrl.getPreprovisionedContentFromStore(snapshot)
	} else {
		content, err = ctrl.getDynamicallyProvisionedContentFromStore(snapshot)
	}
	if err != nil {
		return err
	}

	// NOTE: Source finalizer will be added to snapshot if DeletionTimeStamp is nil
	// and it is not set yet. This is because the logic to check whether a PVC is being
	// created from the snapshot is expensive so we only go through it when we need
	// to remove this finalizer and make sure it is removed when it is not needed any more.
	addSourceFinalizer := utils.NeedToAddSnapshotAsSourceFinalizer(snapshot)

	// note that content could be nil, in this case bound finalizer is not needed
	addBoundFinalizer := false
	if content != nil {
		// A bound finalizer is needed ONLY when all following conditions are satisfied:
		// 1. the VolumeSnapshot is bound to a content
		// 2. the VolumeSnapshot does not have deletion timestamp set
		// 3. the matching content has a deletion policy to be Delete
		// Note that if a matching content is found, it must points back to the snapshot
		addBoundFinalizer = utils.NeedToAddSnapshotBoundFinalizer(snapshot) && (content.Spec.DeletionPolicy == crdv1.VolumeSnapshotContentDelete)
	}
	if addSourceFinalizer || addBoundFinalizer {
		// Snapshot is not being deleted -> it should have the finalizer.
		klog.V(5).Infof("checkandAddSnapshotFinalizers: Add Finalizer for VolumeSnapshot[%s]", utils.SnapshotKey(snapshot))
		return ctrl.addSnapshotFinalizer(snapshot, addSourceFinalizer, addBoundFinalizer)
	}
	return nil
}

// syncReadySnapshot checks the snapshot which has been bound to snapshot content successfully before.
// If there is any problem with the binding (e.g., snapshot points to a non-existent snapshot content), update the snapshot status and emit event.
func (ctrl *csiSnapshotCommonController) syncReadySnapshot(snapshot *crdv1.VolumeSnapshot) error {
	if !utils.IsBoundVolumeSnapshotContentNameSet(snapshot) {
		return fmt.Errorf("snapshot %s is not bound to a content", utils.SnapshotKey(snapshot))
	}
	content, err := ctrl.getContentFromStore(*snapshot.Status.BoundVolumeSnapshotContentName)
	if err != nil {
		return nil
	}
	if content == nil {
		// this meant there is no matching content in cache found
		// update status of the snapshot and return
		return ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMissing", "VolumeSnapshotContent is missing")
	}
	klog.V(5).Infof("syncReadySnapshot[%s]: VolumeSnapshotContent %q found", utils.SnapshotKey(snapshot), content.Name)
	// check binding from content side to make sure the binding is still valid
	if !utils.IsVolumeSnapshotRefSet(snapshot, content) {
		// snapshot is bound but content is not pointing to the snapshot
		return ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotMisbound", "VolumeSnapshotContent is not bound to the VolumeSnapshot correctly")
	}
	// everything is verified, return
	return nil
}

// syncUnreadySnapshot is the main controller method to decide what to do with a snapshot which is not set to ready.
func (ctrl *csiSnapshotCommonController) syncUnreadySnapshot(snapshot *crdv1.VolumeSnapshot) error {
	uniqueSnapshotName := utils.SnapshotKey(snapshot)
	klog.V(5).Infof("syncUnreadySnapshot %s", uniqueSnapshotName)

	// Pre-provisioned snapshot
	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		content, err := ctrl.getPreprovisionedContentFromStore(snapshot)
		if err != nil {
			return err
		}
		// if no content found yet, update status and return
		if content == nil {
			// can not find the desired VolumeSnapshotContent from cache store
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMissing", "VolumeSnapshotContent is missing")
			klog.V(4).Infof("syncUnreadySnapshot[%s]: snapshot content %q requested but not found, will try again", utils.SnapshotKey(snapshot), *snapshot.Spec.Source.VolumeSnapshotContentName)
			return fmt.Errorf("snapshot %s requests an non-existing content %s", utils.SnapshotKey(snapshot), *snapshot.Spec.Source.VolumeSnapshotContentName)
		}
		// Set VolumeSnapshotRef UID
		newContent, err := ctrl.checkandBindSnapshotContent(snapshot, content)
		if err != nil {
			// snapshot is bound but content is not bound to snapshot correctly
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotBindFailed", fmt.Sprintf("Snapshot failed to bind VolumeSnapshotContent, %v", err))
			return fmt.Errorf("snapshot %s is bound, but VolumeSnapshotContent %s is not bound to the VolumeSnapshot correctly, %v", uniqueSnapshotName, content.Name, err)
		}

		// update snapshot status
		klog.V(5).Infof("syncUnreadySnapshot [%s]: trying to update snapshot status", utils.SnapshotKey(snapshot))
		if _, err = ctrl.updateSnapshotStatus(snapshot, newContent); err != nil {
			// update snapshot status failed
			klog.V(4).Infof("failed to update snapshot %s status: %v", utils.SnapshotKey(snapshot), err)
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotStatusUpdateFailed", fmt.Sprintf("Snapshot status update failed, %v", err))
			return err
		}
		return nil
	}
	// snapshot.Spec.Source.VolumeSnapshotContentName == nil - dynamically creating snapshot
	klog.V(5).Infof("getDynamicallyProvisionedContentFromStore for snapshot %s", uniqueSnapshotName)
	contentObj, err := ctrl.getDynamicallyProvisionedContentFromStore(snapshot)
	if err != nil {
		klog.V(4).Infof("getDynamicallyProvisionedContentFromStore[%s]: error when get content for snapshot %v", uniqueSnapshotName, err)
		return err
	}

	if contentObj != nil {
		klog.V(5).Infof("Found VolumeSnapshotContent object %s for snapshot %s", contentObj.Name, uniqueSnapshotName)
		if contentObj.Spec.Source.SnapshotHandle != nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotHandleSet", fmt.Sprintf("Snapshot handle should not be set in content %s for dynamic provisioning", uniqueSnapshotName))
			return fmt.Errorf("snapshotHandle should not be set in the content for dynamic provisioning for snapshot %s", uniqueSnapshotName)
		}
		newSnapshot, err := ctrl.bindandUpdateVolumeSnapshot(contentObj, snapshot)
		if err != nil {
			klog.V(4).Infof("bindandUpdateVolumeSnapshot[%s]: failed to bind content [%s] to snapshot %v", uniqueSnapshotName, contentObj.Name, err)
			return err
		}
		klog.V(5).Infof("bindandUpdateVolumeSnapshot %v", newSnapshot)
		return nil
	} else if snapshot.Status == nil || snapshot.Status.Error == nil || isControllerUpdateFailError(snapshot.Status.Error) {
		if snapshot.Spec.Source.PersistentVolumeClaimName == nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotPVCSourceMissing", fmt.Sprintf("PVC source for snapshot %s is missing", uniqueSnapshotName))
			return fmt.Errorf("expected PVC source for snapshot %s but got nil", uniqueSnapshotName)
		}
		var err error
		var content *crdv1.VolumeSnapshotContent
		if content, err = ctrl.createSnapshotContent(snapshot); err != nil {
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentCreationFailed", fmt.Sprintf("Failed to create snapshot content with error %v", err))
			return err
		}

		// Update snapshot status with BoundVolumeSnapshotContentName
		klog.V(5).Infof("syncUnreadySnapshot [%s]: trying to update snapshot status", utils.SnapshotKey(snapshot))
		if _, err = ctrl.updateSnapshotStatus(snapshot, content); err != nil {
			// update snapshot status failed
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotStatusUpdateFailed", fmt.Sprintf("Snapshot status update failed, %v", err))
			return err
		}
	}
	return nil
}

// getPreprovisionedContentFromStore tries to find a pre-provisioned content object
// from content cache store for the passed in VolumeSnapshot.
// Note that this function assumes the passed in VolumeSnapshot is a pre-provisioned
// one, i.e., snapshot.Spec.Source.VolumeSnapshotContentName != nil.
// If no matching content is found, it returns (nil, nil).
// If it found a content which is not a pre-provisioned one, it updates the status
// of the snapshot with an event and returns an error.
// If it found a content which does not point to the passed in VolumeSnapshot, it
// updates the status of the snapshot with an event and returns an error.
// Otherwise, the found content will be returned.
// A content is considered to be a pre-provisioned one if its Spec.Source.SnapshotHandle
// is not nil, or a dynamically provisioned one if its Spec.Source.VolumeHandle is not nil.
func (ctrl *csiSnapshotCommonController) getPreprovisionedContentFromStore(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotContent, error) {
	contentName := *snapshot.Spec.Source.VolumeSnapshotContentName
	if contentName == "" {
		return nil, fmt.Errorf("empty VolumeSnapshotContentName for snapshot %s", utils.SnapshotKey(snapshot))
	}
	content, err := ctrl.getContentFromStore(contentName)
	if err != nil {
		return nil, err
	}
	if content == nil {
		// can not find the desired VolumeSnapshotContent from cache store
		return nil, nil
	}
	// check whether the content is a pre-provisioned VolumeSnapshotContent
	if content.Spec.Source.SnapshotHandle == nil {
		// found a content which represents a dynamically provisioned snapshot
		// update the snapshot and return an error
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMismatch", "VolumeSnapshotContent is dynamically provisioned while expecting a pre-provisioned one")
		klog.V(4).Infof("sync snapshot[%s]: snapshot content %q is dynamically provisioned while expecting a pre-provisioned one", utils.SnapshotKey(snapshot), contentName)
		return nil, fmt.Errorf("snapshot %s expects a pre-provisioned VolumeSnapshotContent %s but gets a dynamically provisioned one", utils.SnapshotKey(snapshot), contentName)
	}
	// verify the content points back to the snapshot
	ref := content.Spec.VolumeSnapshotRef
	if ref.Name != snapshot.Name || ref.Namespace != snapshot.Namespace || (ref.UID != "" && ref.UID != snapshot.UID) {
		klog.V(4).Infof("sync snapshot[%s]: VolumeSnapshotContent %s is bound to another snapshot %v", utils.SnapshotKey(snapshot), contentName, ref)
		msg := fmt.Sprintf("VolumeSnapshotContent [%s] is bound to a different snapshot", contentName)
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMisbound", msg)
		return nil, fmt.Errorf(msg)
	}
	return content, nil
}

// getDynamicallyProvisionedContentFromStore tries to find a dynamically created
// content object for the passed in VolumeSnapshot from the content store.
// Note that this function assumes the passed in VolumeSnapshot is a dynamic
// one which requests creating a snapshot from a PVC.
// i.e., with snapshot.Spec.Source.PersistentVolumeClaimName != nil
// If no matching VolumeSnapshotContent exists in the content cache store, it
// returns (nil, nil)
// If a content is found but it's not dynamically provisioned, the passed in
// snapshot status will be updated with an error along with an event, and an error
// will be returned.
// If a content is found but it does not point to the passed in VolumeSnapshot,
// the passed in snapshot will be updated with an error along with an event,
// and an error will be returned.
// A content is considered to be a pre-provisioned one if its Spec.Source.SnapshotHandle
// is not nil, or a dynamically provisioned one if its Spec.Source.VolumeHandle is not nil.
func (ctrl *csiSnapshotCommonController) getDynamicallyProvisionedContentFromStore(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotContent, error) {
	contentName := utils.GetDynamicSnapshotContentNameForSnapshot(snapshot)
	content, err := ctrl.getContentFromStore(contentName)
	if err != nil {
		return nil, err
	}
	if content == nil {
		// no matching content with the desired name has been found in cache
		return nil, nil
	}
	// check whether the content represents a dynamically provisioned snapshot
	if content.Spec.Source.VolumeHandle == nil {
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMismatch", "VolumeSnapshotContent "+contentName+" is pre-provisioned while expecting a dynamically provisioned one")
		klog.V(4).Infof("sync snapshot[%s]: snapshot content %s is pre-provisioned while expecting a dynamically provisioned one", utils.SnapshotKey(snapshot), contentName)
		return nil, fmt.Errorf("snapshot %s expects a dynamically provisioned VolumeSnapshotContent %s but gets a pre-provisioned one", utils.SnapshotKey(snapshot), contentName)
	}
	// check whether the content points back to the passed in VolumeSnapshot
	ref := content.Spec.VolumeSnapshotRef
	// Unlike a pre-provisioned content, whose Spec.VolumeSnapshotRef.UID will be
	// left to be empty to allow binding to a snapshot, a dynamically provisioned
	// content MUST have its Spec.VolumeSnapshotRef.UID set to the snapshot's UID
	// from which it's been created, thus ref.UID == "" is not a legit case here.
	if ref.Name != snapshot.Name || ref.Namespace != snapshot.Namespace || ref.UID != snapshot.UID {
		klog.V(4).Infof("sync snapshot[%s]: VolumeSnapshotContent %s is bound to another snapshot %v", utils.SnapshotKey(snapshot), contentName, ref)
		msg := fmt.Sprintf("VolumeSnapshotContent [%s] is bound to a different snapshot", contentName)
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SnapshotContentMisbound", msg)
		return nil, fmt.Errorf(msg)
	}
	return content, nil
}

// getContentFromStore tries to find a VolumeSnapshotContent from content cache
// store by name.
// Note that if no VolumeSnapshotContent exists in the cache store and no error
// encountered, it returns(nil, nil)
func (ctrl *csiSnapshotCommonController) getContentFromStore(contentName string) (*crdv1.VolumeSnapshotContent, error) {
	obj, exist, err := ctrl.contentStore.GetByKey(contentName)
	if err != nil {
		// should never reach here based on implementation at:
		// https://github.com/kubernetes/client-go/blob/master/tools/cache/store.go#L226
		return nil, err
	}
	if !exist {
		// not able to find a matching content
		return nil, nil
	}
	content, ok := obj.(*crdv1.VolumeSnapshotContent)
	if !ok {
		return nil, fmt.Errorf("expected VolumeSnapshotContent, got %+v", obj)
	}
	return content, nil
}

// createSnapshotContent will only be called for dynamic provisioning
func (ctrl *csiSnapshotCommonController) createSnapshotContent(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotContent, error) {
	klog.Infof("createSnapshotContent: Creating content for snapshot %s through the plugin ...", utils.SnapshotKey(snapshot))

	// If PVC is not being deleted and finalizer is not added yet, a finalizer should be added to PVC until snapshot is created
	klog.V(5).Infof("createSnapshotContent: Check if PVC is not being deleted and add Finalizer for source of snapshot [%s] if needed", snapshot.Name)
	err := ctrl.ensurePVCFinalizer(snapshot)
	if err != nil {
		klog.Errorf("createSnapshotContent failed to add finalizer for source of snapshot %s", err)
		return nil, err
	}

	class, volume, contentName, snapshotterSecretRef, err := ctrl.getCreateSnapshotInput(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to get input parameters to create snapshot %s: %q", snapshot.Name, err)
	}

	// Create VolumeSnapshotContent in the database
	if volume.Spec.CSI == nil {
		return nil, fmt.Errorf("cannot find CSI PersistentVolumeSource for volume %s", volume.Name)
	}
	snapshotRef, err := ref.GetReference(scheme.Scheme, snapshot)
	if err != nil {
		return nil, err
	}

	snapshotContent := &crdv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentName,
		},
		Spec: crdv1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: *snapshotRef,
			Source: crdv1.VolumeSnapshotContentSource{
				VolumeHandle: &volume.Spec.CSI.VolumeHandle,
			},
			VolumeSnapshotClassName: &(class.Name),
			DeletionPolicy:          class.DeletionPolicy,
			Driver:                  class.Driver,
		},
	}

	// Set AnnDeletionSecretRefName and AnnDeletionSecretRefNamespace
	if snapshotterSecretRef != nil {
		klog.V(5).Infof("createSnapshotContent: set annotation [%s] on content [%s].", utils.AnnDeletionSecretRefName, snapshotContent.Name)
		metav1.SetMetaDataAnnotation(&snapshotContent.ObjectMeta, utils.AnnDeletionSecretRefName, snapshotterSecretRef.Name)

		klog.V(5).Infof("createSnapshotContent: set annotation [%s] on content [%s].", utils.AnnDeletionSecretRefNamespace, snapshotContent.Name)
		metav1.SetMetaDataAnnotation(&snapshotContent.ObjectMeta, utils.AnnDeletionSecretRefNamespace, snapshotterSecretRef.Namespace)
	}

	var updateContent *crdv1.VolumeSnapshotContent
	klog.V(5).Infof("volume snapshot content %#v", snapshotContent)
	// Try to create the VolumeSnapshotContent object
	klog.V(5).Infof("createSnapshotContent [%s]: trying to save volume snapshot content %s", utils.SnapshotKey(snapshot), snapshotContent.Name)
	if updateContent, err = ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Create(context.TODO(), snapshotContent, metav1.CreateOptions{}); err == nil || apierrs.IsAlreadyExists(err) {
		// Save succeeded.
		if err != nil {
			klog.V(3).Infof("volume snapshot content %q for snapshot %q already exists, reusing", snapshotContent.Name, utils.SnapshotKey(snapshot))
			err = nil
			updateContent = snapshotContent
		} else {
			klog.V(3).Infof("volume snapshot content %q for snapshot %q saved, %v", snapshotContent.Name, utils.SnapshotKey(snapshot), snapshotContent)
		}
	}

	if err != nil {
		strerr := fmt.Sprintf("Error creating volume snapshot content object for snapshot %s: %v.", utils.SnapshotKey(snapshot), err)
		klog.Error(strerr)
		ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "CreateSnapshotContentFailed", strerr)
		return nil, newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
	}

	msg := fmt.Sprintf("Waiting for a snapshot %s to be created by the CSI driver.", utils.SnapshotKey(snapshot))
	ctrl.eventRecorder.Event(snapshot, v1.EventTypeNormal, "CreatingSnapshot", msg)

	// Update content in the cache store
	_, err = ctrl.storeContentUpdate(updateContent)
	if err != nil {
		klog.Errorf("failed to update content store %v", err)
	}

	return updateContent, nil
}

func (ctrl *csiSnapshotCommonController) getCreateSnapshotInput(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotClass, *v1.PersistentVolume, string, *v1.SecretReference, error) {
	className := snapshot.Spec.VolumeSnapshotClassName
	klog.V(5).Infof("getCreateSnapshotInput [%s]", snapshot.Name)
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
	contentName := utils.GetDynamicSnapshotContentNameForSnapshot(snapshot)

	// Resolve snapshotting secret credentials.
	snapshotterSecretRef, err := utils.GetSecretReference(utils.SnapshotterSecretParams, class.Parameters, contentName, snapshot)
	if err != nil {
		return nil, nil, "", nil, err
	}

	return class, volume, contentName, snapshotterSecretRef, nil
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

	if snapshot.Status != nil && snapshot.Status.Error != nil && *snapshot.Status.Error.Message == message {
		klog.V(4).Infof("updateSnapshotStatusWithEvent[%s]: the same error %v is already set", snapshot.Name, snapshot.Status.Error)
		return nil
	}
	snapshotClone := snapshot.DeepCopy()
	if snapshotClone.Status == nil {
		snapshotClone.Status = &crdv1.VolumeSnapshotStatus{}
	}
	statusError := &crdv1.VolumeSnapshotError{
		Time: &metav1.Time{
			Time: time.Now(),
		},
		Message: &message,
	}
	snapshotClone.Status.Error = statusError
	ready := false
	snapshotClone.Status.ReadyToUse = &ready
	newSnapshot, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(context.TODO(), snapshotClone, metav1.UpdateOptions{})

	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] error status failed %v", utils.SnapshotKey(snapshot), err)
		return err
	}

	// Emit the event only when the status change happens
	ctrl.eventRecorder.Event(newSnapshot, eventtype, reason, message)

	_, err = ctrl.storeSnapshotUpdate(newSnapshot)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] error status: cannot update internal cache %v", utils.SnapshotKey(snapshot), err)
		return err
	}

	return nil
}

// addContentFinalizer adds a Finalizer for VolumeSnapshotContent.
func (ctrl *csiSnapshotCommonController) addContentFinalizer(content *crdv1.VolumeSnapshotContent) error {
	contentClone := content.DeepCopy()
	contentClone.ObjectMeta.Finalizers = append(contentClone.ObjectMeta.Finalizers, utils.VolumeSnapshotContentFinalizer)

	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Update(context.TODO(), contentClone, metav1.UpdateOptions{})
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

// isVolumeBeingCreatedFromSnapshot checks if an volume is being created from the snapshot.
func (ctrl *csiSnapshotCommonController) isVolumeBeingCreatedFromSnapshot(snapshot *crdv1.VolumeSnapshot) bool {
	pvcList, err := ctrl.pvcLister.PersistentVolumeClaims(snapshot.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("Failed to retrieve PVCs from the lister to check if volume snapshot %s is being used by a volume: %q", utils.SnapshotKey(snapshot), err)
		return false
	}
	for _, pvc := range pvcList {
		if pvc.Spec.DataSource != nil && pvc.Spec.DataSource.Name == snapshot.Name {
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

// ensurePVCFinalizer checks if a Finalizer needs to be added for the snapshot source;
// if true, adds a Finalizer for VolumeSnapshot Source PVC
func (ctrl *csiSnapshotCommonController) ensurePVCFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	if snapshot.Spec.Source.PersistentVolumeClaimName == nil {
		// PVC finalizer is only needed for dynamic provisioning
		return nil
	}

	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already.", snapshot.Name, err)
		return newControllerUpdateError(snapshot.Name, "cannot get claim from snapshot")
	}

	if pvc.ObjectMeta.DeletionTimestamp != nil {
		klog.Errorf("cannot add finalizer on claim [%s] for snapshot [%s]: claim is being deleted", pvc.Name, snapshot.Name)
		return newControllerUpdateError(pvc.Name, "cannot add finalizer on claim because it is being deleted")
	}

	// If PVC is not being deleted and PVCFinalizer is not added yet, the PVCFinalizer should be added.
	if pvc.ObjectMeta.DeletionTimestamp == nil && !slice.ContainsString(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer, nil) {
		// Add the finalizer
		pvcClone := pvc.DeepCopy()
		pvcClone.ObjectMeta.Finalizers = append(pvcClone.ObjectMeta.Finalizers, utils.PVCFinalizer)
		_, err = ctrl.client.CoreV1().PersistentVolumeClaims(pvcClone.Namespace).Update(context.TODO(), pvcClone, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("cannot add finalizer on claim [%s] for snapshot [%s]: [%v]", pvc.Name, snapshot.Name, err)
			return newControllerUpdateError(pvcClone.Name, err.Error())
		}
		klog.Infof("Added protection finalizer to persistent volume claim %s", pvc.Name)
	}

	return nil
}

// removePVCFinalizer removes a Finalizer for VolumeSnapshot Source PVC.
func (ctrl *csiSnapshotCommonController) removePVCFinalizer(pvc *v1.PersistentVolumeClaim, snapshot *crdv1.VolumeSnapshot) error {
	// Get snapshot source which is a PVC
	// TODO(xyang): We get PVC from informer but it may be outdated
	// Should get it from API server directly before removing finalizer
	pvcClone := pvc.DeepCopy()
	pvcClone.ObjectMeta.Finalizers = slice.RemoveString(pvcClone.ObjectMeta.Finalizers, utils.PVCFinalizer, nil)

	_, err := ctrl.client.CoreV1().PersistentVolumeClaims(pvcClone.Namespace).Update(context.TODO(), pvcClone, metav1.UpdateOptions{})
	if err != nil {
		return newControllerUpdateError(pvcClone.Name, err.Error())
	}

	klog.V(5).Infof("Removed protection finalizer from persistent volume claim %s", pvc.Name)
	return nil
}

// isPVCBeingUsed checks if a PVC is being used as a source to create a snapshot
func (ctrl *csiSnapshotCommonController) isPVCBeingUsed(pvc *v1.PersistentVolumeClaim, snapshot *crdv1.VolumeSnapshot) bool {
	klog.V(5).Infof("Checking isPVCBeingUsed for snapshot [%s]", utils.SnapshotKey(snapshot))

	// Going through snapshots in the cache (snapshotLister). If a snapshot's PVC source
	// is the same as the input snapshot's PVC source and snapshot's ReadyToUse status
	// is false, the snapshot is still being created from the PVC and the PVC is in-use.
	snapshots, err := ctrl.snapshotLister.VolumeSnapshots(snapshot.Namespace).List(labels.Everything())
	if err != nil {
		return false
	}
	for _, snap := range snapshots {
		// Skip pre-provisioned snapshot without a PVC source
		if snap.Spec.Source.PersistentVolumeClaimName == nil && snap.Spec.Source.VolumeSnapshotContentName != nil {
			klog.V(4).Infof("Skipping static bound snapshot %s when checking PVC %s/%s", snap.Name, pvc.Namespace, pvc.Name)
			continue
		}
		if snap.Spec.Source.PersistentVolumeClaimName != nil && pvc.Name == *snap.Spec.Source.PersistentVolumeClaimName && !utils.IsSnapshotReady(snap) {
			klog.V(2).Infof("Keeping PVC %s/%s, it is used by snapshot %s/%s", pvc.Namespace, pvc.Name, snap.Namespace, snap.Name)
			return true
		}
	}

	klog.V(5).Infof("isPVCBeingUsed: no snapshot is being created from PVC %s/%s", pvc.Namespace, pvc.Name)
	return false
}

// checkandRemovePVCFinalizer checks if the snapshot source finalizer should be removed
// and removed it if needed.
func (ctrl *csiSnapshotCommonController) checkandRemovePVCFinalizer(snapshot *crdv1.VolumeSnapshot) error {
	if snapshot.Spec.Source.PersistentVolumeClaimName == nil {
		// PVC finalizer is only needed for dynamic provisioning
		return nil
	}

	// Get snapshot source which is a PVC
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		klog.Infof("cannot get claim from snapshot [%s]: [%v] Claim may be deleted already. No need to remove finalizer on the claim.", snapshot.Name, err)
		return nil
	}

	klog.V(5).Infof("checkandRemovePVCFinalizer for snapshot [%s]: snapshot status [%#v]", snapshot.Name, snapshot.Status)

	// Check if there is a Finalizer on PVC to be removed
	if slice.ContainsString(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer, nil) {
		// There is a Finalizer on PVC. Check if PVC is used
		// and remove finalizer if it's not used.
		inUse := ctrl.isPVCBeingUsed(pvc, snapshot)
		if !inUse {
			klog.Infof("checkandRemovePVCFinalizer[%s]: Remove Finalizer for PVC %s as it is not used by snapshots in creation", snapshot.Name, pvc.Name)
			err = ctrl.removePVCFinalizer(pvc, snapshot)
			if err != nil {
				klog.Errorf("checkandRemovePVCFinalizer [%s]: removePVCFinalizer failed to remove finalizer %v", snapshot.Name, err)
				return err
			}
		}
	}

	return nil
}

// The function checks whether the volumeSnapshotRef in the snapshot content matches
// the given snapshot. If match, it binds the content with the snapshot. This is for
// static binding where user has specified snapshot name but not UID of the snapshot
// in content.Spec.VolumeSnapshotRef.
func (ctrl *csiSnapshotCommonController) checkandBindSnapshotContent(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshotContent, error) {
	if content.Spec.VolumeSnapshotRef.Name != snapshot.Name {
		return nil, fmt.Errorf("Could not bind snapshot %s and content %s, the VolumeSnapshotRef does not match", snapshot.Name, content.Name)
	} else if content.Spec.VolumeSnapshotRef.UID != "" && content.Spec.VolumeSnapshotRef.UID != snapshot.UID {
		return nil, fmt.Errorf("Could not bind snapshot %s and content %s, the VolumeSnapshotRef does not match", snapshot.Name, content.Name)
	} else if content.Spec.VolumeSnapshotRef.UID != "" && content.Spec.VolumeSnapshotClassName != nil {
		return content, nil
	}
	contentClone := content.DeepCopy()
	contentClone.Spec.VolumeSnapshotRef.UID = snapshot.UID
	if snapshot.Spec.VolumeSnapshotClassName != nil {
		className := *(snapshot.Spec.VolumeSnapshotClassName)
		contentClone.Spec.VolumeSnapshotClassName = &className
	}
	newContent, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Update(context.TODO(), contentClone, metav1.UpdateOptions{})
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status failed %v", contentClone.Name, err)
		return nil, err
	}

	_, err = ctrl.storeContentUpdate(newContent)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status: cannot update internal cache %v", newContent.Name, err)
		return nil, err
	}
	return newContent, nil
}

// This routine sets snapshot.Spec.Source.VolumeSnapshotContentName
func (ctrl *csiSnapshotCommonController) bindandUpdateVolumeSnapshot(snapshotContent *crdv1.VolumeSnapshotContent, snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("bindandUpdateVolumeSnapshot for snapshot [%s]: snapshotContent [%s]", snapshot.Name, snapshotContent.Name)
	snapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshot.Namespace).Get(context.TODO(), snapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot %s from api server: %v", utils.SnapshotKey(snapshot), err)
	}

	// Copy the snapshot object before updating it
	snapshotCopy := snapshotObj.DeepCopy()
	// update snapshot status
	var updateSnapshot *crdv1.VolumeSnapshot
	klog.V(5).Infof("bindandUpdateVolumeSnapshot [%s]: trying to update snapshot status", utils.SnapshotKey(snapshotCopy))
	updateSnapshot, err = ctrl.updateSnapshotStatus(snapshotCopy, snapshotContent)
	if err == nil {
		snapshotCopy = updateSnapshot
	}
	if err != nil {
		// update snapshot status failed
		klog.V(4).Infof("failed to update snapshot %s status: %v", utils.SnapshotKey(snapshot), err)
		ctrl.updateSnapshotErrorStatusWithEvent(snapshotCopy, v1.EventTypeWarning, "SnapshotStatusUpdateFailed", fmt.Sprintf("Snapshot status update failed, %v", err))
		return nil, err
	}

	_, err = ctrl.storeSnapshotUpdate(snapshotCopy)
	if err != nil {
		klog.Errorf("%v", err)
	}

	klog.V(5).Infof("bindandUpdateVolumeSnapshot for snapshot completed [%#v]", snapshotCopy)
	return snapshotCopy, nil
}

// needsUpdateSnapshotStatus compares snapshot status with the content status and decide
// if snapshot status needs to be updated based on content status
func (ctrl *csiSnapshotCommonController) needsUpdateSnapshotStatus(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) bool {
	klog.V(5).Infof("needsUpdateSnapshotStatus[%s]", utils.SnapshotKey(snapshot))

	if snapshot.Status == nil && content.Status != nil {
		return true
	}
	if content.Status == nil {
		return false
	}
	if snapshot.Status.BoundVolumeSnapshotContentName == nil {
		return true
	}
	if snapshot.Status.CreationTime == nil && content.Status.CreationTime != nil {
		return true
	}
	if snapshot.Status.ReadyToUse == nil && content.Status.ReadyToUse != nil {
		return true
	}
	if snapshot.Status.ReadyToUse != nil && content.Status.ReadyToUse != nil && snapshot.Status.ReadyToUse != content.Status.ReadyToUse {
		return true
	}
	if snapshot.Status.RestoreSize == nil && content.Status.RestoreSize != nil {
		return true
	}
	if snapshot.Status.RestoreSize != nil && snapshot.Status.RestoreSize.IsZero() && content.Status.RestoreSize != nil && *content.Status.RestoreSize > 0 {
		return true
	}

	return false
}

// UpdateSnapshotStatus updates snapshot status based on content status
func (ctrl *csiSnapshotCommonController) updateSnapshotStatus(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("updateSnapshotStatus[%s]", utils.SnapshotKey(snapshot))

	boundContentName := content.Name
	var createdAt *time.Time
	if content.Status != nil && content.Status.CreationTime != nil {
		unixTime := time.Unix(0, *content.Status.CreationTime)
		createdAt = &unixTime
	}
	var size *int64
	if content.Status != nil && content.Status.RestoreSize != nil {
		size = content.Status.RestoreSize
	}
	var readyToUse bool
	if content.Status != nil && content.Status.ReadyToUse != nil {
		readyToUse = *content.Status.ReadyToUse
	}

	klog.V(5).Infof("updateSnapshotStatus: updating VolumeSnapshot [%+v] based on VolumeSnapshotContentStatus [%+v]", snapshot, content.Status)

	snapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshot.Namespace).Get(context.TODO(), snapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot %s from api server: %v", utils.SnapshotKey(snapshot), err)
	}

	var newStatus *crdv1.VolumeSnapshotStatus
	updated := false
	if snapshotObj.Status == nil {
		newStatus = &crdv1.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &boundContentName,
			ReadyToUse:                     &readyToUse,
		}
		if createdAt != nil {
			newStatus.CreationTime = &metav1.Time{Time: *createdAt}
		}
		if size != nil {
			newStatus.RestoreSize = resource.NewQuantity(*size, resource.BinarySI)
		}
		updated = true
	} else {
		newStatus = snapshotObj.Status.DeepCopy()
		if newStatus.BoundVolumeSnapshotContentName == nil {
			newStatus.BoundVolumeSnapshotContentName = &boundContentName
			updated = true
		}
		if newStatus.CreationTime == nil && createdAt != nil {
			newStatus.CreationTime = &metav1.Time{Time: *createdAt}
			updated = true
		}
		if newStatus.ReadyToUse == nil || *newStatus.ReadyToUse != readyToUse {
			newStatus.ReadyToUse = &readyToUse
			updated = true
			if readyToUse && newStatus.Error != nil {
				newStatus.Error = nil
			}
		}
		if (newStatus.RestoreSize == nil && size != nil) || (newStatus.RestoreSize != nil && newStatus.RestoreSize.IsZero() && size != nil && *size > 0) {
			newStatus.RestoreSize = resource.NewQuantity(*size, resource.BinarySI)
			updated = true
		}
	}

	if updated {
		snapshotClone := snapshotObj.DeepCopy()
		snapshotClone.Status = newStatus
		newSnapshotObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(context.TODO(), snapshotClone, metav1.UpdateOptions{})
		if err != nil {
			return nil, newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
		}
		return newSnapshotObj, nil
	}

	return snapshotObj, nil
}

func (ctrl *csiSnapshotCommonController) getVolumeFromVolumeSnapshot(snapshot *crdv1.VolumeSnapshot) (*v1.PersistentVolume, error) {
	pvc, err := ctrl.getClaimFromVolumeSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	if pvc.Status.Phase != v1.ClaimBound {
		return nil, fmt.Errorf("the PVC %s is not yet bound to a PV, will not attempt to take a snapshot", pvc.Name)
	}

	pvName := pvc.Spec.VolumeName
	pv, err := ctrl.client.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve PV %s from the API server: %q", pvName, err)
	}

	// Verify binding between PV/PVC is still valid
	bound := ctrl.isVolumeBoundToClaim(pv, pvc)
	if bound == false {
		klog.Warningf("binding between PV %s and PVC %s is broken", pvName, pvc.Name)
		return nil, fmt.Errorf("claim in dataSource not bound or invalid")
	}

	klog.V(5).Infof("getVolumeFromVolumeSnapshot: snapshot [%s] PV name [%s]", snapshot.Name, pvName)

	return pv, nil
}

// isVolumeBoundToClaim returns true, if given volume is pre-bound or bound
// to specific claim. Both claim.Name and claim.Namespace must be equal.
// If claim.UID is present in volume.Spec.ClaimRef, it must be equal too.
func (ctrl *csiSnapshotCommonController) isVolumeBoundToClaim(volume *v1.PersistentVolume, claim *v1.PersistentVolumeClaim) bool {
	if volume.Spec.ClaimRef == nil {
		return false
	}
	if claim.Name != volume.Spec.ClaimRef.Name || claim.Namespace != volume.Spec.ClaimRef.Namespace {
		return false
	}
	if volume.Spec.ClaimRef.UID != "" && claim.UID != volume.Spec.ClaimRef.UID {
		return false
	}
	return true
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
	storageclass, err := ctrl.client.StorageV1().StorageClasses().Get(context.TODO(), storageclassName, metav1.GetOptions{})
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
		return nil, err
	}

	return class, nil
}

// SetDefaultSnapshotClass is a helper function to figure out the default snapshot class from
// PVC/PV StorageClass and update VolumeSnapshot with this snapshot class name.
func (ctrl *csiSnapshotCommonController) SetDefaultSnapshotClass(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotClass, *crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("SetDefaultSnapshotClass for snapshot [%s]", snapshot.Name)

	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		// don't return error for pre-provisioned snapshots
		klog.V(5).Infof("Don't need to find SnapshotClass for pre-provisioned snapshot [%s]", snapshot.Name)
		return nil, snapshot, nil
	}

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
		if utils.IsDefaultAnnotation(class.ObjectMeta) && storageclass.Provisioner == class.Driver { //&& ctrl.snapshotterName == class.Snapshotter {
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
	newSnapshot, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).Update(context.TODO(), snapshotClone, metav1.UpdateOptions{})
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
	if snapshot.Spec.Source.PersistentVolumeClaimName == nil {
		return nil, fmt.Errorf("the snapshot source PVC name is not specified")
	}
	pvcName := *snapshot.Spec.Source.PersistentVolumeClaimName
	if pvcName == "" {
		return nil, fmt.Errorf("the PVC name is not specified in snapshot %s", utils.SnapshotKey(snapshot))
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
	if err != nil {
		if strings.Contains(*err.Message, controllerUpdateFailMsg) {
			return true
		}
	}
	return false
}

// addSnapshotFinalizer adds a Finalizer for VolumeSnapshot.
func (ctrl *csiSnapshotCommonController) addSnapshotFinalizer(snapshot *crdv1.VolumeSnapshot, addSourceFinalizer bool, addBoundFinalizer bool) error {
	snapshotClone := snapshot.DeepCopy()
	if addSourceFinalizer {
		snapshotClone.ObjectMeta.Finalizers = append(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotAsSourceFinalizer)
	}
	if addBoundFinalizer {
		snapshotClone.ObjectMeta.Finalizers = append(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotBoundFinalizer)
	}
	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).Update(context.TODO(), snapshotClone, metav1.UpdateOptions{})
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
func (ctrl *csiSnapshotCommonController) removeSnapshotFinalizer(snapshot *crdv1.VolumeSnapshot, removeSourceFinalizer bool, removeBoundFinalizer bool) error {
	if !removeSourceFinalizer && !removeBoundFinalizer {
		return nil
	}

	snapshotClone := snapshot.DeepCopy()
	if removeSourceFinalizer {
		snapshotClone.ObjectMeta.Finalizers = slice.RemoveString(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotAsSourceFinalizer, nil)
	}
	if removeBoundFinalizer {
		snapshotClone.ObjectMeta.Finalizers = slice.RemoveString(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotBoundFinalizer, nil)
	}
	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshots(snapshotClone.Namespace).Update(context.TODO(), snapshotClone, metav1.UpdateOptions{})
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

// getSnapshotFromStore finds snapshot from the cache store.
// If getSnapshotFromStore returns (nil, nil), it means snapshot not found
// and it may have already been deleted.
func (ctrl *csiSnapshotCommonController) getSnapshotFromStore(snapshotName string) (*crdv1.VolumeSnapshot, error) {
	// Get the VolumeSnapshot by _name_
	var snapshot *crdv1.VolumeSnapshot
	obj, found, err := ctrl.snapshotStore.GetByKey(snapshotName)
	if err != nil {
		return nil, err
	}
	if !found {
		klog.V(4).Infof("getSnapshotFromStore: snapshot %s not found", snapshotName)
		// Fall through with snapshot = nil
		return nil, nil
	}
	var ok bool
	snapshot, ok = obj.(*crdv1.VolumeSnapshot)
	if !ok {
		return nil, fmt.Errorf("cannot convert object from snapshot cache to snapshot %q!?: %#v", snapshotName, obj)
	}
	klog.V(4).Infof("getSnapshotFromStore: snapshot %s found", snapshotName)

	return snapshot, nil
}

func (ctrl *csiSnapshotCommonController) setAnnVolumeSnapshotBeingDeleted(content *crdv1.VolumeSnapshotContent) error {
	if content == nil {
		return nil
	}
	// Set AnnVolumeSnapshotBeingDeleted if it is not set yet
	if !metav1.HasAnnotation(content.ObjectMeta, utils.AnnVolumeSnapshotBeingDeleted) {
		klog.V(5).Infof("setAnnVolumeSnapshotBeingDeleted: set annotation [%s] on content [%s].", utils.AnnVolumeSnapshotBeingDeleted, content.Name)
		metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeSnapshotBeingDeleted, "yes")

		updateContent, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Update(context.TODO(), content, metav1.UpdateOptions{})
		if err != nil {
			return newControllerUpdateError(content.Name, err.Error())
		}
		// update content if update is successful
		content = updateContent

		_, err = ctrl.storeContentUpdate(content)
		if err != nil {
			klog.V(4).Infof("setAnnVolumeSnapshotBeingDeleted for content [%s]: cannot update internal cache %v", content.Name, err)
			return err
		}
		klog.V(5).Infof("setAnnVolumeSnapshotBeingDeleted: volume snapshot content %+v", content)
	}
	return nil
}
