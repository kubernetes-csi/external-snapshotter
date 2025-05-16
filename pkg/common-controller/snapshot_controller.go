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
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	ref "k8s.io/client-go/tools/reference"
	"k8s.io/client-go/util/retry"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	klog "k8s.io/klog/v2"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/metrics"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
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

const (
	snapshotKind     = "VolumeSnapshot"
	snapshotAPIGroup = crdv1.GroupName
)

const controllerUpdateFailMsg = "snapshot controller failed to update"

// syncContent deals with one key off the queue
func (ctrl *csiSnapshotCommonController) syncContent(content *crdv1.VolumeSnapshotContent) error {
	snapshotName := utils.SnapshotRefKey(&content.Spec.VolumeSnapshotRef)
	klog.V(4).Infof("synchronizing VolumeSnapshotContent[%s]: content is bound to snapshot %s", content.Name, snapshotName)

	klog.V(5).Infof("syncContent[%s]: check if we should add invalid label on content", content.Name)

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
		// Do not need to use the returned content here, as syncContent will get
		// the correct version from the cache next time. It is also not used after this.
		_, err = ctrl.setAnnVolumeSnapshotBeingDeleted(content)
		return err
	}

	return nil
}

// syncSnapshot is the main controller method to decide what to do with a snapshot.
// It's invoked by appropriate cache.Controller callbacks when a snapshot is
// created, updated or periodically synced. We do not differentiate between
// these events.
// For easier readability, it is split into syncUnreadySnapshot and syncReadySnapshot
func (ctrl *csiSnapshotCommonController) syncSnapshot(ctx context.Context, snapshot *crdv1.VolumeSnapshot) error {
	klog.V(5).Infof("synchronizing VolumeSnapshot[%s]: %s", utils.SnapshotKey(snapshot), utils.GetSnapshotStatusForLogging(snapshot))

	klog.V(5).Infof("syncSnapshot [%s]: check if we should remove finalizer on snapshot PVC source and remove it if we can", utils.SnapshotKey(snapshot))

	// Check if we should remove finalizer on PVC and remove it if we can.
	if err := ctrl.checkandRemovePVCFinalizer(snapshot, false); err != nil {
		klog.Errorf("error check and remove PVC finalizer for snapshot [%s]: %v", snapshot.Name, err)
		// Log an event and keep the original error from checkandRemovePVCFinalizer
		ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "ErrorPVCFinalizer", "Error check and remove PVC Finalizer for VolumeSnapshot")
	}

	klog.V(5).Infof("syncSnapshot[%s]: check if we should add invalid label on snapshot", utils.SnapshotKey(snapshot))

	// Proceed with snapshot deletion and remove finalizers when needed
	if snapshot.ObjectMeta.DeletionTimestamp != nil {
		return ctrl.processSnapshotWithDeletionTimestamp(snapshot)
	}

	klog.V(5).Infof("syncSnapshot[%s]: validate snapshot to make sure source has been correctly specified", utils.SnapshotKey(snapshot))
	if (snapshot.Spec.Source.PersistentVolumeClaimName == nil && snapshot.Spec.Source.VolumeSnapshotContentName == nil) ||
		(snapshot.Spec.Source.PersistentVolumeClaimName != nil && snapshot.Spec.Source.VolumeSnapshotContentName != nil) {
		err := fmt.Errorf("Exactly one of PersistentVolumeClaimName and VolumeSnapshotContentName should be specified")
		klog.Errorf("syncSnapshot[%s]: validation error, %s", utils.SnapshotKey(snapshot), err.Error())
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotValidationError", err.Error())
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
	return ctrl.syncReadySnapshot(ctx, snapshot)
}

// processSnapshotWithDeletionTimestamp processes finalizers and deletes the content when appropriate. It has the following steps:
// 1. Get the content which the to-be-deleted VolumeSnapshot points to and verifies bi-directional binding.
// 2. Call checkandRemoveSnapshotFinalizersAndCheckandDeleteContent() with information obtained from step 1. This function name is very long but the name suggests what it does. It determines whether to remove finalizers on snapshot and whether to delete content.
func (ctrl *csiSnapshotCommonController) processSnapshotWithDeletionTimestamp(snapshot *crdv1.VolumeSnapshot) error {
	klog.V(5).Infof("processSnapshotWithDeletionTimestamp VolumeSnapshot[%s]: %s", utils.SnapshotKey(snapshot), utils.GetSnapshotStatusForLogging(snapshot))
	driverName, err := ctrl.getSnapshotDriverName(snapshot)
	if err != nil {
		klog.Errorf("failed to getSnapshotDriverName while recording metrics for snapshot %q: %v", utils.SnapshotKey(snapshot), err)
	}

	snapshotProvisionType := metrics.DynamicSnapshotType
	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		snapshotProvisionType = metrics.PreProvisionedSnapshotType
	}

	// Processing delete, start operation metric
	deleteOperationKey := metrics.NewOperationKey(metrics.DeleteSnapshotOperationName, snapshot.UID)
	deleteOperationValue := metrics.NewOperationValue(driverName, snapshotProvisionType)
	ctrl.metricsManager.OperationStart(deleteOperationKey, deleteOperationValue)

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

	removeGroupFinalizer := false
	// Block deletion if this snapshot belongs to a group snapshot.
	if snapshot.Status != nil && snapshot.Status.VolumeGroupSnapshotName != nil {
		groupSnapshot, err := ctrl.groupSnapshotLister.VolumeGroupSnapshots(snapshot.Namespace).Get(*snapshot.Status.VolumeGroupSnapshotName)
		if err == nil {
			msg := fmt.Sprintf("deletion of the individual volume snapshot %s is not allowed as it belongs to group snapshot %s. Deleting the group snapshot will trigger the deletion of all the individual volume snapshots that are part of the group.", utils.SnapshotKey(snapshot), utils.GroupSnapshotKey(groupSnapshot))
			klog.Error(msg)
			ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "SnapshotDeletePending", msg)
			return errors.New(msg)
		}
		if !apierrs.IsNotFound(err) {
			klog.Errorf("failed to delete snapshot %s: %v", utils.SnapshotKey(snapshot), err)
			return err
		}
		// group snapshot API object was deleted.
		// The VolumeSnapshotInGroupFinalizer can be removed from this snapshot
		// to trigger deletion.
		removeGroupFinalizer = true
	}

	// regardless of the deletion policy, set the VolumeSnapshotBeingDeleted on
	// content object, this is to allow snapshotter sidecar controller to conduct
	// a delete operation whenever the content has deletion timestamp set.
	if content != nil {
		klog.V(5).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent[%s]: Set VolumeSnapshotBeingDeleted annotation on the content [%s]", utils.SnapshotKey(snapshot), content.Name)
		updatedContent, err := ctrl.setAnnVolumeSnapshotBeingDeleted(content)
		if err != nil {
			klog.V(4).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent[%s]: failed to set VolumeSnapshotBeingDeleted annotation on the content [%s]", utils.SnapshotKey(snapshot), content.Name)
			return err
		}
		content = updatedContent
	}

	// VolumeSnapshot should be deleted. Check and remove finalizers
	// If content exists and has a deletion policy of Delete, set DeletionTimeStamp on the content;
	// content won't be deleted immediately due to the VolumeSnapshotContentFinalizer
	if content != nil && deleteContent {
		klog.V(5).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent: set DeletionTimeStamp on content [%s].", content.Name)
		err := ctrl.clientset.SnapshotV1().VolumeSnapshotContents().Delete(context.TODO(), content.Name, metav1.DeleteOptions{})
		if err != nil {
			ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "SnapshotContentObjectDeleteError", "Failed to delete snapshot content API object")
			return fmt.Errorf("failed to delete VolumeSnapshotContent %s from API server: %q", content.Name, err)
		}
	}

	klog.V(5).Infof("checkandRemoveSnapshotFinalizersAndCheckandDeleteContent: Remove Finalizer for VolumeSnapshot[%s]", utils.SnapshotKey(snapshot))
	// remove finalizers on the VolumeSnapshot object, there are three finalizers:
	// 1. VolumeSnapshotAsSourceFinalizer, once reached here, the snapshot is not
	//    in use to restore PVC, and the finalizer will be removed directly.
	// 2. VolumeSnapshotBoundFinalizer:
	//    a. If there is no content found, remove the finalizer.
	//    b. If the content is being deleted, i.e., with deleteContent == true,
	//       keep this finalizer until the content object is removed from API server
	//       by snapshot sidecar controller.
	//    c. If deletion will not cascade to the content, remove the finalizer on
	//       the snapshot such that it can be removed from API server.
	// 3. VolumeSnapshotInGroupFinalizer, if the snapshot was part of a group snapshot,
	//    then the group snapshot has been deleted, so remove the finalizer.
	removeBoundFinalizer := !(content != nil && deleteContent)
	return ctrl.removeSnapshotFinalizer(snapshot, true, removeBoundFinalizer, removeGroupFinalizer)
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
func (ctrl *csiSnapshotCommonController) syncReadySnapshot(ctx context.Context, snapshot *crdv1.VolumeSnapshot) error {
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
		return ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotContentMissing", "VolumeSnapshotContent is missing")
	}
	klog.V(5).Infof("syncReadySnapshot[%s]: VolumeSnapshotContent %q found", utils.SnapshotKey(snapshot), content.Name)
	// check binding from content side to make sure the binding is still valid
	if !utils.IsVolumeSnapshotRefSet(snapshot, content) {
		// snapshot is bound but content is not pointing to the snapshot
		return ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotMisbound", "VolumeSnapshotContent is not bound to the VolumeSnapshot correctly")
	}

	// If this snapshot is a member of a volume group snapshot, ensure we have
	// the correct ownership. This happens when the user
	// statically provisioned volume group snapshot members.
	if utils.NeedToAddVolumeGroupSnapshotOwnership(snapshot) {
		if _, err := ctrl.addVolumeGroupSnapshotOwnership(ctx, snapshot); err != nil {
			return err
		}
	}

	// everything is verified, return
	return nil
}

// addVolumeGroupSnapshotOwnership adds the ownership information to a statically provisioned VolumeSnapshot
// that is a member of a volume group snapshot
func (ctrl *csiSnapshotCommonController) addVolumeGroupSnapshotOwnership(ctx context.Context, snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	klog.V(4).Infof("addVolumeGroupSnapshotOwnership[%s]: adding ownership information", utils.SnapshotKey(snapshot))
	if snapshot.Status == nil || snapshot.Status.VolumeGroupSnapshotName == nil {
		klog.V(4).Infof("addVolumeGroupSnapshotOwnership[%s]: no need to add ownership information, empty volumeGroupSnapshotName", utils.SnapshotKey(snapshot))
		return nil, nil
	}
	parentObjectName := *snapshot.Status.VolumeGroupSnapshotName

	parentGroup, err := ctrl.groupSnapshotLister.VolumeGroupSnapshots(snapshot.Namespace).Get(parentObjectName)
	if err != nil {
		klog.V(4).Infof("addVolumeGroupSnapshotOwnership[%s]: error while looking for parent group %v", utils.SnapshotKey(snapshot), err)
		return nil, err
	}
	if parentGroup == nil {
		klog.V(4).Infof("addVolumeGroupSnapshotOwnership[%s]: parent group not found %v", utils.SnapshotKey(snapshot), err)
		return nil, fmt.Errorf("missing parent group for snapshot %v", utils.SnapshotKey(snapshot))
	}

	updatedSnapshot := snapshot.DeepCopy()
	updatedSnapshot.ObjectMeta.OwnerReferences = append(
		snapshot.ObjectMeta.OwnerReferences,
		utils.BuildVolumeGroupSnapshotOwnerReference(parentGroup),
	)

	newSnapshot, err := ctrl.clientset.SnapshotV1().VolumeSnapshots(snapshot.Namespace).Update(ctx, updatedSnapshot, metav1.UpdateOptions{})
	if err != nil {
		klog.V(4).Infof("addVolumeGroupSnapshotOwnership[%s]: error when updating VolumeSnapshot %v", utils.SnapshotKey(snapshot), err)
		return nil, err
	}

	klog.V(4).Infof("addVolumeGroupSnapshotOwnership[%s]: updated ownership", utils.SnapshotKey(snapshot))

	return newSnapshot, nil
}

// syncUnreadySnapshot is the main controller method to decide what to do with a snapshot which is not set to ready.
func (ctrl *csiSnapshotCommonController) syncUnreadySnapshot(snapshot *crdv1.VolumeSnapshot) error {
	uniqueSnapshotName := utils.SnapshotKey(snapshot)
	klog.V(5).Infof("syncUnreadySnapshot %s", uniqueSnapshotName)
	driverName, err := ctrl.getSnapshotDriverName(snapshot)
	if err != nil {
		klog.Errorf("failed to getSnapshotDriverName while recording metrics for snapshot %q: %s", utils.SnapshotKey(snapshot), err)
	}

	snapshotProvisionType := metrics.DynamicSnapshotType
	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		snapshotProvisionType = metrics.PreProvisionedSnapshotType
	}

	// Start metrics operations
	if !utils.IsSnapshotCreated(snapshot) {
		// Only start CreateSnapshot operation if the snapshot has not been cut
		ctrl.metricsManager.OperationStart(
			metrics.NewOperationKey(metrics.CreateSnapshotOperationName, snapshot.UID),
			metrics.NewOperationValue(driverName, snapshotProvisionType),
		)
	}
	ctrl.metricsManager.OperationStart(
		metrics.NewOperationKey(metrics.CreateSnapshotAndReadyOperationName, snapshot.UID),
		metrics.NewOperationValue(driverName, snapshotProvisionType),
	)

	// Pre-provisioned snapshot
	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		content, err := ctrl.getPreprovisionedContentFromStore(snapshot)
		if err != nil {
			return err
		}

		// if no content found yet, update status and return
		if content == nil {
			// can not find the desired VolumeSnapshotContent from cache store
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotContentMissing", "VolumeSnapshotContent is missing")
			klog.V(4).Infof("syncUnreadySnapshot[%s]: snapshot content %q requested but not found, will try again", utils.SnapshotKey(snapshot), *snapshot.Spec.Source.VolumeSnapshotContentName)

			return fmt.Errorf("snapshot %s requests an non-existing content %s", utils.SnapshotKey(snapshot), *snapshot.Spec.Source.VolumeSnapshotContentName)
		}

		// Set VolumeSnapshotRef UID
		newContent, err := ctrl.checkandBindSnapshotContent(snapshot, content)
		if err != nil {
			// snapshot is bound but content is not bound to snapshot correctly
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotBindFailed", fmt.Sprintf("Snapshot failed to bind VolumeSnapshotContent, %v", err))
			return fmt.Errorf("snapshot %s is bound, but VolumeSnapshotContent %s is not bound to the VolumeSnapshot correctly, %v", uniqueSnapshotName, content.Name, err)
		}

		// update snapshot status
		klog.V(5).Infof("syncUnreadySnapshot [%s]: trying to update snapshot status", utils.SnapshotKey(snapshot))
		if _, err = ctrl.updateSnapshotStatus(snapshot, newContent); err != nil {
			// update snapshot status failed
			klog.V(4).Infof("failed to update snapshot %s status: %v", utils.SnapshotKey(snapshot), err)
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, false, v1.EventTypeWarning, "SnapshotStatusUpdateFailed", fmt.Sprintf("Snapshot status update failed, %v", err))
			return err
		}

		return nil
	}

	// member of a dynamically provisioned volume group snapshot
	if utils.IsVolumeGroupSnapshotMember(snapshot) {
		if snapshot.Status == nil || snapshot.Status.BoundVolumeSnapshotContentName == nil {
			klog.V(5).Infof(
				"syncUnreadySnapshot [%s]: detected group snapshot member with no content, retrying",
				utils.SnapshotKey(snapshot))
			return fmt.Errorf("detected group snapshot member %s with no content, retrying",
				utils.SnapshotKey(snapshot))
		}

		volumeSnapshotContentName := *snapshot.Status.BoundVolumeSnapshotContentName

		content, err := ctrl.getContentFromStore(volumeSnapshotContentName)
		if err != nil {
			return err
		}
		if content == nil {
			// can not find the desired VolumeSnapshotContent from cache store
			// we'll retry
			return fmt.Errorf("group snapshot member %s requests an non-existing content %s", utils.SnapshotKey(snapshot), volumeSnapshotContentName)
		}

		// update snapshot status
		klog.V(5).Infof("syncUnreadySnapshot [%s]: trying to update group snapshot member status", utils.SnapshotKey(snapshot))
		if _, err = ctrl.updateSnapshotStatus(snapshot, content); err != nil {
			// update snapshot status failed
			klog.V(4).Infof("failed to update group snapshot member %s status: %v", utils.SnapshotKey(snapshot), err)
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, false, v1.EventTypeWarning, "SnapshotStatusUpdateFailed", fmt.Sprintf("Snapshot status update failed, %v", err))
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
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotHandleSet", fmt.Sprintf("Snapshot handle should not be set in content %s for dynamic provisioning", uniqueSnapshotName))
			return fmt.Errorf("snapshotHandle should not be set in the content for dynamic provisioning for snapshot %s", uniqueSnapshotName)
		}
		newSnapshot, err := ctrl.bindandUpdateVolumeSnapshot(contentObj, snapshot)
		if err != nil {
			klog.V(4).Infof("bindandUpdateVolumeSnapshot[%s]: failed to bind content [%s] to snapshot %v", uniqueSnapshotName, contentObj.Name, err)
			return err
		}
		klog.V(5).Infof("bindandUpdateVolumeSnapshot %v", newSnapshot)
		return nil
	}

	// If we reach here, it is a dynamically provisioned snapshot, and the volumeSnapshotContent object is not yet created.
	if snapshot.Spec.Source.PersistentVolumeClaimName == nil {
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotPVCSourceMissing", fmt.Sprintf("PVC source for snapshot %s is missing", uniqueSnapshotName))
		return fmt.Errorf("expected PVC source for snapshot %s but got nil", uniqueSnapshotName)
	}
	var content *crdv1.VolumeSnapshotContent
	if content, err = ctrl.createSnapshotContent(snapshot); err != nil {
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotContentCreationFailed", fmt.Sprintf("Failed to create snapshot content with error %v", err))
		return err
	}

	// Update snapshot status with BoundVolumeSnapshotContentName
	klog.V(5).Infof("syncUnreadySnapshot [%s]: trying to update snapshot status", utils.SnapshotKey(snapshot))
	if _, err = ctrl.updateSnapshotStatus(snapshot, content); err != nil {
		// update snapshot status failed
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, false, v1.EventTypeWarning, "SnapshotStatusUpdateFailed", fmt.Sprintf("Snapshot status update failed, %v", err))
		return err
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
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotContentMismatch", "VolumeSnapshotContent is dynamically provisioned while expecting a pre-provisioned one")
		klog.V(4).Infof("sync snapshot[%s]: snapshot content %q is dynamically provisioned while expecting a pre-provisioned one", utils.SnapshotKey(snapshot), contentName)
		return nil, fmt.Errorf("snapshot %s expects a pre-provisioned VolumeSnapshotContent %s but gets a dynamically provisioned one", utils.SnapshotKey(snapshot), contentName)
	}
	// verify the content points back to the snapshot
	ref := content.Spec.VolumeSnapshotRef
	if ref.Name != snapshot.Name || ref.Namespace != snapshot.Namespace || (ref.UID != "" && ref.UID != snapshot.UID) {
		klog.V(4).Infof("sync snapshot[%s]: VolumeSnapshotContent %s is bound to another snapshot %v", utils.SnapshotKey(snapshot), contentName, ref)
		msg := fmt.Sprintf("VolumeSnapshotContent [%s] is bound to a different snapshot", contentName)
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotContentMisbound", msg)
		return nil, errors.New(msg)
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
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotContentMismatch", "VolumeSnapshotContent "+contentName+" is pre-provisioned while expecting a dynamically provisioned one")
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
		ctrl.updateSnapshotErrorStatusWithEvent(snapshot, true, v1.EventTypeWarning, "SnapshotContentMisbound", msg)
		return nil, errors.New(msg)
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

	if ctrl.enableDistributedSnapshotting {
		nodeName, err := ctrl.getManagedByNode(volume)
		if err != nil {
			return nil, err
		}
		if nodeName != "" {
			snapshotContent.Labels = map[string]string{
				utils.VolumeSnapshotContentManagedByLabel: nodeName,
			}
		}
	}

	if ctrl.preventVolumeModeConversion {
		if volume.Spec.VolumeMode != nil {
			snapshotContent.Spec.SourceVolumeMode = volume.Spec.VolumeMode
			klog.V(5).Infof("snapcontent %s has volume mode %s", snapshotContent.Name, *snapshotContent.Spec.SourceVolumeMode)
		}
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
	if updateContent, err = ctrl.clientset.SnapshotV1().VolumeSnapshotContents().Create(context.TODO(), snapshotContent, metav1.CreateOptions{}); err == nil || apierrs.IsAlreadyExists(err) {
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

// updateSnapshotErrorStatusWithEvent saves new snapshot.Status to API server and emits
// given event on the snapshot. It saves the status and emits the event only when
// the status has actually changed from the version saved in API server.
// Parameters:
//
//   - snapshot - snapshot to update
//   - setReadyToFalse bool - indicates whether to set the snapshot's ReadyToUse status to false.
//     if true, ReadyToUse will be set to false;
//     otherwise, ReadyToUse will not be changed.
//   - eventtype, reason, message - event to send, see EventRecorder.Event()
func (ctrl *csiSnapshotCommonController) updateSnapshotErrorStatusWithEvent(snapshot *crdv1.VolumeSnapshot, setReadyToFalse bool, eventtype, reason, message string) error {
	klog.V(5).Infof("updateSnapshotErrorStatusWithEvent[%s]", utils.SnapshotKey(snapshot))

	if snapshot.Status != nil && snapshot.Status.Error != nil && *snapshot.Status.Error.Message == message {
		klog.V(4).Infof("updateSnapshotErrorStatusWithEvent[%s]: the same error %v is already set", snapshot.Name, snapshot.Status.Error)
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
	// Only update ReadyToUse in VolumeSnapshot's Status to false if setReadyToFalse is true.
	if setReadyToFalse {
		ready := false
		snapshotClone.Status.ReadyToUse = &ready
	}
	newSnapshot, err := ctrl.clientset.SnapshotV1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(context.TODO(), snapshotClone, metav1.UpdateOptions{})

	// Emit the event even if the status update fails so that user can see the error
	ctrl.eventRecorder.Event(newSnapshot, eventtype, reason, message)

	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] error status failed %v", utils.SnapshotKey(snapshot), err)
		return err
	}

	_, err = ctrl.storeSnapshotUpdate(newSnapshot)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] error status: cannot update internal cache %v", utils.SnapshotKey(snapshot), err)
		return err
	}

	return nil
}

// addContentFinalizer adds a Finalizer for VolumeSnapshotContent.
func (ctrl *csiSnapshotCommonController) addContentFinalizer(content *crdv1.VolumeSnapshotContent) error {
	var patches []utils.PatchOp
	if len(content.Finalizers) > 0 {
		// Add to the end of the finalizers if we have any other finalizers
		patches = append(patches, utils.PatchOp{
			Op:    "add",
			Path:  "/metadata/finalizers/-",
			Value: utils.VolumeSnapshotContentFinalizer,
		})
	} else {
		// Replace finalizers with new array if there are no other finalizers
		patches = append(patches, utils.PatchOp{
			Op:    "add",
			Path:  "/metadata/finalizers",
			Value: []string{utils.VolumeSnapshotContentFinalizer},
		})
	}
	newContent, err := utils.PatchVolumeSnapshotContent(content, patches, ctrl.clientset)
	if err != nil {
		return newControllerUpdateError(content.Name, err.Error())
	}

	_, err = ctrl.storeContentUpdate(newContent)
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

	if slices.Contains(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer) {
		klog.Infof("Protection finalizer already exists for persistent volume claim %s/%s", pvc.Namespace, pvc.Name)
		return nil
	}

	if pvc.ObjectMeta.DeletionTimestamp != nil {
		klog.Errorf("cannot add finalizer on claim [%s/%s] for snapshot [%s/%s]: claim is being deleted", pvc.Namespace, pvc.Name, snapshot.Namespace, snapshot.Name)
		return newControllerUpdateError(pvc.Name, "cannot add finalizer on claim because it is being deleted")
	} else {
		// If PVC is not being deleted and PVCFinalizer is not added yet, add the PVCFinalizer.
		pvcClone := pvc.DeepCopy()
		pvcClone.ObjectMeta.Finalizers = append(pvcClone.ObjectMeta.Finalizers, utils.PVCFinalizer)
		_, err = ctrl.client.CoreV1().PersistentVolumeClaims(pvcClone.Namespace).Update(context.TODO(), pvcClone, metav1.UpdateOptions{})
		if err != nil {
			klog.Errorf("cannot add finalizer on claim [%s/%s] for snapshot [%s/%s]: [%v]", pvc.Namespace, pvc.Name, snapshot.Namespace, snapshot.Name, err)
			return newControllerUpdateError(pvcClone.Name, err.Error())
		}
		klog.Infof("Added protection finalizer to persistent volume claim %s/%s", pvc.Namespace, pvc.Name)
	}

	return nil
}

// removePVCFinalizer removes a Finalizer for VolumeSnapshot Source PVC.
func (ctrl *csiSnapshotCommonController) removePVCFinalizer(pvc *v1.PersistentVolumeClaim) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get snapshot source which is a PVC
		newPvc, err := ctrl.client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.TODO(), pvc.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		newPvc = newPvc.DeepCopy()
		newPvc.ObjectMeta.Finalizers = utils.RemoveString(newPvc.ObjectMeta.Finalizers, utils.PVCFinalizer)
		_, err = ctrl.client.CoreV1().PersistentVolumeClaims(newPvc.Namespace).Update(context.TODO(), newPvc, metav1.UpdateOptions{})
		if err != nil {
			return newControllerUpdateError(newPvc.Name, err.Error())
		}

		klog.V(5).Infof("Removed protection finalizer from persistent volume claim %s", pvc.Name)
		return nil
	})
}

// isPVCBeingUsed checks if a PVC is being used as a source to create a snapshot.
// If skipCurrentSnapshot is true, skip checking if the current snapshot is using the PVC as source.
func (ctrl *csiSnapshotCommonController) isPVCBeingUsed(pvc *v1.PersistentVolumeClaim, snapshot *crdv1.VolumeSnapshot, skipCurrentSnapshot bool) bool {
	klog.V(5).Infof("Checking isPVCBeingUsed for snapshot [%s]", utils.SnapshotKey(snapshot))

	// Going through snapshots in the cache (snapshotLister). If a snapshot's PVC source
	// is the same as the input snapshot's PVC source and snapshot's ReadyToUse status
	// is false, the snapshot is still being created from the PVC and the PVC is in-use.
	snapshots, err := ctrl.snapshotLister.VolumeSnapshots(snapshot.Namespace).List(labels.Everything())
	if err != nil {
		return false
	}
	for _, snap := range snapshots {
		// Skip the current snapshot
		if skipCurrentSnapshot && snap.Name == snapshot.Name {
			continue
		}
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
// and removed it if needed. If skipCurrentSnapshot is true, skip checking if the current
// snapshot is using the PVC as source.
func (ctrl *csiSnapshotCommonController) checkandRemovePVCFinalizer(snapshot *crdv1.VolumeSnapshot, skipCurrentSnapshot bool) error {
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
	if slices.Contains(pvc.ObjectMeta.Finalizers, utils.PVCFinalizer) {
		// There is a Finalizer on PVC. Check if PVC is used
		// and remove finalizer if it's not used.
		inUse := ctrl.isPVCBeingUsed(pvc, snapshot, skipCurrentSnapshot)
		if !inUse {
			klog.Infof("checkandRemovePVCFinalizer[%s]: Remove Finalizer for PVC %s as it is not used by snapshots in creation", snapshot.Name, pvc.Name)
			err = ctrl.removePVCFinalizer(pvc)
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

	patches := []utils.PatchOp{
		{
			Op:    "replace",
			Path:  "/spec/volumeSnapshotRef/uid",
			Value: string(snapshot.UID),
		},
	}
	if snapshot.Spec.VolumeSnapshotClassName != nil {
		className := *(snapshot.Spec.VolumeSnapshotClassName)
		patches = append(patches, utils.PatchOp{
			Op:    "replace",
			Path:  "/spec/volumeSnapshotClassName",
			Value: className,
		})
	}

	newContent, err := utils.PatchVolumeSnapshotContent(content, patches, ctrl.clientset)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status failed %v", content.Name, err)
		return content, err
	}

	_, err = ctrl.storeContentUpdate(newContent)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status: cannot update internal cache %v", newContent.Name, err)
		return newContent, err
	}
	return newContent, nil
}

// This routine sets snapshot.Spec.Source.VolumeSnapshotContentName
func (ctrl *csiSnapshotCommonController) bindandUpdateVolumeSnapshot(snapshotContent *crdv1.VolumeSnapshotContent, snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("bindandUpdateVolumeSnapshot for snapshot [%s]: snapshotContent [%s]", snapshot.Name, snapshotContent.Name)
	snapshotObj, err := ctrl.clientset.SnapshotV1().VolumeSnapshots(snapshot.Namespace).Get(context.TODO(), snapshot.Name, metav1.GetOptions{})
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
		ctrl.updateSnapshotErrorStatusWithEvent(snapshotCopy, true, v1.EventTypeWarning, "SnapshotStatusUpdateFailed", fmt.Sprintf("Snapshot status update failed, %v", err))
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
	var volumeSnapshotErr *crdv1.VolumeSnapshotError
	if content.Status != nil && content.Status.Error != nil {
		volumeSnapshotErr = content.Status.Error.DeepCopy()
	}

	var groupSnapshotName string
	if content.Status != nil && content.Status.VolumeGroupSnapshotHandle != nil {
		// If this snapshot belongs to a group snapshot, find the group snapshot
		// name from the group snapshot content
		groupSnapshotContentList, err := ctrl.groupSnapshotContentLister.List(labels.Everything())
		if err != nil {
			return nil, err
		}
		found := false
		for _, groupSnapshotContent := range groupSnapshotContentList {
			if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.VolumeGroupSnapshotHandle != nil && *groupSnapshotContent.Status.VolumeGroupSnapshotHandle == *content.Status.VolumeGroupSnapshotHandle {
				groupSnapshotName = groupSnapshotContent.Spec.VolumeGroupSnapshotRef.Name
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("updateSnapshotStatus: cannot find the group snapshot for VolumeSnapshot [%s], will not update snapshot status", utils.SnapshotKey(snapshot))
		}
	}

	klog.V(5).Infof("updateSnapshotStatus: updating VolumeSnapshot [%+v] based on VolumeSnapshotContentStatus [%+v]", snapshot, content.Status)

	snapshotObj, err := ctrl.clientset.SnapshotV1().VolumeSnapshots(snapshot.Namespace).Get(context.TODO(), snapshot.Name, metav1.GetOptions{})
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
		if volumeSnapshotErr != nil {
			newStatus.Error = volumeSnapshotErr
		}
		if groupSnapshotName != "" {
			newStatus.VolumeGroupSnapshotName = &groupSnapshotName
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
		if (newStatus.Error == nil && volumeSnapshotErr != nil) || (newStatus.Error != nil && volumeSnapshotErr != nil && newStatus.Error.Time != nil && volumeSnapshotErr.Time != nil && &newStatus.Error.Time != &volumeSnapshotErr.Time) || (newStatus.Error != nil && volumeSnapshotErr == nil) {
			newStatus.Error = volumeSnapshotErr
			updated = true
		}
		if newStatus.VolumeGroupSnapshotName == nil && groupSnapshotName != "" {
			newStatus.VolumeGroupSnapshotName = &groupSnapshotName
			updated = true
		}
	}

	if updated {
		snapshotClone := snapshotObj.DeepCopy()
		snapshotClone.Status = newStatus

		// We need to record metrics before updating the status due to a bug causing cache entries after a failed UpdateStatus call.
		// Must meet the following criteria to emit a successful CreateSnapshot status
		// 1. Previous status was nil OR Previous status had a nil CreationTime
		// 2. New status must be non-nil with a non-nil CreationTime
		driverName := content.Spec.Driver
		createOperationKey := metrics.NewOperationKey(metrics.CreateSnapshotOperationName, snapshot.UID)
		if !utils.IsSnapshotCreated(snapshotObj) && utils.IsSnapshotCreated(snapshotClone) {
			ctrl.metricsManager.RecordMetrics(createOperationKey, metrics.NewSnapshotOperationStatus(metrics.SnapshotStatusTypeSuccess), driverName)
			msg := fmt.Sprintf("Snapshot %s was successfully created by the CSI driver.", utils.SnapshotKey(snapshot))
			ctrl.eventRecorder.Event(snapshot, v1.EventTypeNormal, "SnapshotCreated", msg)
		}

		// Must meet the following criteria to emit a successful CreateSnapshotAndReady status
		// 1. Previous status was nil OR Previous status had a nil ReadyToUse OR Previous status had a false ReadyToUse
		// 2. New status must be non-nil with a ReadyToUse as true
		if !utils.IsSnapshotReady(snapshotObj) && utils.IsSnapshotReady(snapshotClone) {
			createAndReadyOperation := metrics.NewOperationKey(metrics.CreateSnapshotAndReadyOperationName, snapshot.UID)
			ctrl.metricsManager.RecordMetrics(createAndReadyOperation, metrics.NewSnapshotOperationStatus(metrics.SnapshotStatusTypeSuccess), driverName)
			msg := fmt.Sprintf("Snapshot %s is ready to use.", utils.SnapshotKey(snapshot))
			ctrl.eventRecorder.Event(snapshot, v1.EventTypeNormal, "SnapshotReady", msg)
		}

		newSnapshotObj, err := ctrl.clientset.SnapshotV1().VolumeSnapshots(snapshotClone.Namespace).UpdateStatus(context.TODO(), snapshotClone, metav1.UpdateOptions{})
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

// pvDriverFromSnapshot is a helper function to get the CSI driver name from the targeted PersistentVolume.
// It looks up the PVC from which the snapshot is specified to be created from, and looks for the PVC's corresponding
// PV. Bi-directional binding will be verified between PVC and PV before the PV's CSI driver is returned.
// For an non-CSI volume, it returns an error immediately as it's not supported.
func (ctrl *csiSnapshotCommonController) pvDriverFromSnapshot(snapshot *crdv1.VolumeSnapshot) (string, error) {
	pv, err := ctrl.getVolumeFromVolumeSnapshot(snapshot)
	if err != nil {
		return "", err
	}
	// supports ONLY CSI volumes
	if pv.Spec.PersistentVolumeSource.CSI == nil {
		return "", fmt.Errorf("snapshotting non-CSI volumes is not supported, snapshot:%s/%s", snapshot.Namespace, snapshot.Name)
	}
	return pv.Spec.PersistentVolumeSource.CSI.Driver, nil
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

// getSnapshotDriverName is a helper function to get snapshot driver from the VolumeSnapshot.
// We try to get the driverName in multiple ways, as snapshot controller metrics depend on the correct driverName.
func (ctrl *csiSnapshotCommonController) getSnapshotDriverName(vs *crdv1.VolumeSnapshot) (string, error) {
	klog.V(5).Infof("getSnapshotDriverName: VolumeSnapshot[%s]", vs.Name)
	var driverName string

	// Pre-Provisioned snapshots have contentName as source
	var contentName string
	if vs.Spec.Source.VolumeSnapshotContentName != nil {
		contentName = *vs.Spec.Source.VolumeSnapshotContentName
	}

	// Get Driver name from SnapshotContent if we found a contentName
	if contentName != "" {
		content, err := ctrl.contentLister.Get(contentName)
		if err != nil {
			klog.Errorf("getSnapshotDriverName: failed to get snapshotContent: %v", contentName)
		} else {
			driverName = content.Spec.Driver
		}

		if driverName != "" {
			return driverName, nil
		}
	}

	// Dynamic snapshots will have a snapshotclass with a driver
	if vs.Spec.VolumeSnapshotClassName != nil {
		class, err := ctrl.getSnapshotClass(*vs.Spec.VolumeSnapshotClassName)
		if err != nil {
			klog.Errorf("getSnapshotDriverName: failed to get snapshotClass: %v", *vs.Spec.VolumeSnapshotClassName)
		} else {
			driverName = class.Driver
		}
	}

	return driverName, nil
}

// SetDefaultSnapshotClass is a helper function to figure out the default snapshot class.
// For pre-provisioned case, it's an no-op.
// For dynamic provisioning, it gets the default SnapshotClasses in the system if there is any(could be multiple),
// and finds the one with the same CSI Driver as the PV from which a snapshot will be taken.
func (ctrl *csiSnapshotCommonController) SetDefaultSnapshotClass(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshotClass, *crdv1.VolumeSnapshot, error) {
	klog.V(5).Infof("SetDefaultSnapshotClass for snapshot [%s]", snapshot.Name)

	if snapshot.Spec.Source.VolumeSnapshotContentName != nil {
		// don't return error for pre-provisioned snapshots
		klog.V(5).Infof("Don't need to find SnapshotClass for pre-provisioned snapshot [%s]", snapshot.Name)
		return nil, snapshot, nil
	}

	if utils.IsVolumeGroupSnapshotMember(snapshot) {
		// don't return error for volume group snapshot members
		klog.V(5).Infof("Don't need to find SnapshotClass for volume group snapshot member [%s]", snapshot.Name)
		return nil, snapshot, nil
	}

	// Find default snapshot class if available
	list, err := ctrl.classLister.List(labels.Everything())
	if err != nil {
		return nil, snapshot, err
	}

	pvDriver, err := ctrl.pvDriverFromSnapshot(snapshot)
	if err != nil {
		klog.Errorf("failed to get pv csi driver from snapshot %s/%s: %q", snapshot.Namespace, snapshot.Name, err)
		return nil, snapshot, err
	}

	defaultClasses := []*crdv1.VolumeSnapshotClass{}
	for _, class := range list {
		if utils.IsVolumeSnapshotClassDefaultAnnotation(class.ObjectMeta) && pvDriver == class.Driver {
			defaultClasses = append(defaultClasses, class)
			klog.V(5).Infof("get defaultClass added: %s, driver: %s", class.Name, pvDriver)
		}
	}
	if len(defaultClasses) == 0 {
		return nil, snapshot, fmt.Errorf("cannot find default snapshot class")
	}
	if len(defaultClasses) > 1 {
		klog.V(4).Infof("get DefaultClass %d defaults found", len(defaultClasses))
		return nil, snapshot, fmt.Errorf("%d default snapshot classes were found", len(defaultClasses))
	}
	klog.V(5).Infof("setDefaultSnapshotClass [%s]: default VolumeSnapshotClassName [%s]", snapshot.Name, defaultClasses[0].Name)
	snapshotClone := snapshot.DeepCopy()
	patches := []utils.PatchOp{
		{
			Op:    "replace",
			Path:  "/spec/volumeSnapshotClassName",
			Value: &(defaultClasses[0].Name),
		},
	}

	newSnapshot, err := utils.PatchVolumeSnapshot(snapshotClone, patches, ctrl.clientset)
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
	var updatedSnapshot *crdv1.VolumeSnapshot
	var err error

	// NOTE(ggriffiths): Must perform an update if no finalizers exist.
	// Unable to find a patch that correctly updated the finalizers if none currently exist.
	if len(snapshot.ObjectMeta.Finalizers) == 0 {
		snapshotClone := snapshot.DeepCopy()
		if addSourceFinalizer {
			snapshotClone.ObjectMeta.Finalizers = append(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotAsSourceFinalizer)
		}
		if addBoundFinalizer {
			snapshotClone.ObjectMeta.Finalizers = append(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotBoundFinalizer)
		}
		updatedSnapshot, err = ctrl.clientset.SnapshotV1().VolumeSnapshots(snapshotClone.Namespace).Update(context.TODO(), snapshotClone, metav1.UpdateOptions{})
		if err != nil {
			return newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
		}
	} else {
		// Otherwise, perform a patch
		var patches []utils.PatchOp

		// If finalizers exist already, add new ones to the end of the array
		if addSourceFinalizer {
			patches = append(patches, utils.PatchOp{
				Op:    "add",
				Path:  "/metadata/finalizers/-",
				Value: utils.VolumeSnapshotAsSourceFinalizer,
			})
		}
		if addBoundFinalizer {
			patches = append(patches, utils.PatchOp{
				Op:    "add",
				Path:  "/metadata/finalizers/-",
				Value: utils.VolumeSnapshotBoundFinalizer,
			})
		}

		updatedSnapshot, err = utils.PatchVolumeSnapshot(snapshot, patches, ctrl.clientset)
		if err != nil {
			return newControllerUpdateError(utils.SnapshotKey(snapshot), err.Error())
		}
	}

	_, err = ctrl.storeSnapshotUpdate(updatedSnapshot)
	if err != nil {
		klog.Errorf("failed to update snapshot store %v", err)
	}

	klog.V(5).Infof("Added protection finalizer to volume snapshot %s", utils.SnapshotKey(updatedSnapshot))
	return nil
}

// removeSnapshotFinalizer removes a Finalizer for VolumeSnapshot.
func (ctrl *csiSnapshotCommonController) removeSnapshotFinalizer(snapshot *crdv1.VolumeSnapshot, removeSourceFinalizer bool, removeBoundFinalizer bool, removeGroupFinalizer bool) error {
	if !removeSourceFinalizer && !removeBoundFinalizer && !removeGroupFinalizer {
		return nil
	}

	// NOTE(xyang): We have to make sure PVC finalizer is deleted before
	// the VolumeSnapshot API object is deleted. Once the VolumeSnapshot
	// API object is deleted, there won't be any VolumeSnapshot update
	// event that can trigger the PVC finalizer removal any more.
	// We also can't remove PVC finalizer too early. PVC finalizer should
	// not be removed if a VolumeSnapshot API object is still using it.
	// If we are here, it means we are going to remove finalizers from the
	// VolumeSnapshot API object so that the VolumeSnapshot API object can
	// be deleted. This means we no longer need to keep the PVC finalizer
	// for this particular snapshot.
	if err := ctrl.checkandRemovePVCFinalizer(snapshot, true); err != nil {
		klog.Errorf("removeSnapshotFinalizer: error check and remove PVC finalizer for snapshot [%s]: %v", snapshot.Name, err)
		// Log an event and keep the original error from checkandRemovePVCFinalizer
		ctrl.eventRecorder.Event(snapshot, v1.EventTypeWarning, "ErrorPVCFinalizer", "Error check and remove PVC Finalizer for VolumeSnapshot")
		return newControllerUpdateError(snapshot.Name, err.Error())
	}

	snapshotClone := snapshot.DeepCopy()
	if removeSourceFinalizer {
		snapshotClone.ObjectMeta.Finalizers = utils.RemoveString(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotAsSourceFinalizer)
	}
	if removeBoundFinalizer {
		snapshotClone.ObjectMeta.Finalizers = utils.RemoveString(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotBoundFinalizer)
	}
	if removeGroupFinalizer {
		snapshotClone.ObjectMeta.Finalizers = utils.RemoveString(snapshotClone.ObjectMeta.Finalizers, utils.VolumeSnapshotInGroupFinalizer)
	}
	newSnapshot, err := ctrl.clientset.SnapshotV1().VolumeSnapshots(snapshotClone.Namespace).Update(context.TODO(), snapshotClone, metav1.UpdateOptions{})
	if err != nil {
		return newControllerUpdateError(snapshot.Name, err.Error())
	}

	_, err = ctrl.storeSnapshotUpdate(newSnapshot)
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

func (ctrl *csiSnapshotCommonController) setAnnVolumeSnapshotBeingDeleted(content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshotContent, error) {
	if content == nil {
		return content, nil
	}
	// Set AnnVolumeSnapshotBeingDeleted if it is not set yet
	if !metav1.HasAnnotation(content.ObjectMeta, utils.AnnVolumeSnapshotBeingDeleted) {
		klog.V(5).Infof("setAnnVolumeSnapshotBeingDeleted: set annotation [%s] on content [%s].", utils.AnnVolumeSnapshotBeingDeleted, content.Name)
		var patches []utils.PatchOp
		metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeSnapshotBeingDeleted, "yes")
		patches = append(patches, utils.PatchOp{
			Op:    "replace",
			Path:  "/metadata/annotations",
			Value: content.ObjectMeta.GetAnnotations(),
		})

		patchedContent, err := utils.PatchVolumeSnapshotContent(content, patches, ctrl.clientset)
		if err != nil {
			return content, newControllerUpdateError(content.Name, err.Error())
		}

		// update content if update is successful
		content = patchedContent

		_, err = ctrl.storeContentUpdate(content)
		if err != nil {
			klog.V(4).Infof("setAnnVolumeSnapshotBeingDeleted for content [%s]: cannot update internal cache %v", content.Name, err)
			return content, err
		}
		klog.V(5).Infof("setAnnVolumeSnapshotBeingDeleted: volume snapshot content %+v", content)
	}
	return content, nil
}

func (ctrl *csiSnapshotCommonController) getManagedByNode(pv *v1.PersistentVolume) (string, error) {
	if pv.Spec.NodeAffinity == nil {
		klog.V(5).Infof("NodeAffinity not set for pv %s", pv.Name)
		return "", nil
	}
	nodeSelectorTerms := pv.Spec.NodeAffinity.Required

	nodes, err := ctrl.nodeLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("failed to get the list of nodes: %q", err)
		return "", err
	}

	for _, node := range nodes {
		match, _ := corev1helpers.MatchNodeSelectorTerms(node, nodeSelectorTerms)
		if match {
			return node.Name, nil
		}
	}

	klog.Errorf("failed to find nodes that match the node affinity requirements for pv[%s]", pv.Name)
	return "", nil
}
