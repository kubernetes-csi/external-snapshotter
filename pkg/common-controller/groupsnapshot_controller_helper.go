/*
Copyright 2023 The Kubernetes Authors.

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
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ref "k8s.io/client-go/tools/reference"
	klog "k8s.io/klog/v2"

	crdv1beta2 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta2"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/metrics"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
)

func (ctrl *csiSnapshotCommonController) storeGroupSnapshotUpdate(groupsnapshot interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.groupSnapshotStore, groupsnapshot, "groupsnapshot")
}

func (ctrl *csiSnapshotCommonController) storeGroupSnapshotContentUpdate(groupsnapshotcontent interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.groupSnapshotContentStore, groupsnapshotcontent, "groupsnapshotcontent")
}

// getGroupSnapshotClass is a helper function to get group snapshot class from the group snapshot class name.
func (ctrl *csiSnapshotCommonController) getGroupSnapshotClass(className string) (*crdv1beta2.VolumeGroupSnapshotClass, error) {
	klog.V(5).Infof("getGroupSnapshotClass: VolumeGroupSnapshotClassName [%s]", className)

	groupSnapshotClass, err := ctrl.groupSnapshotClassLister.Get(className)
	if err != nil {
		klog.Errorf("failed to retrieve group snapshot class %s from the informer: %q", className, err)
		return nil, err
	}

	return groupSnapshotClass, nil
}

// updateGroupSnapshotErrorStatusWithEvent saves new groupsnapshot.Status to API
// server and emits given event on the group snapshot. It saves the status and
// emits the event only when the status has actually changed from the version
// saved in API server.
//
// Parameters:
//
//   - groupSnapshot - group snapshot to update
//   - setReadyToFalse bool - indicates whether to set the group snapshot's
//     ReadyToUse status to false.
//     if true, ReadyToUse will be set to false;
//     otherwise, ReadyToUse will not be changed.
//   - eventtype, reason, message - event to send, see EventRecorder.Event()
func (ctrl *csiSnapshotCommonController) updateGroupSnapshotErrorStatusWithEvent(groupSnapshot *crdv1beta2.VolumeGroupSnapshot, setReadyToFalse bool, eventtype, reason, message string) error {
	klog.V(5).Infof("updateGroupSnapshotErrorStatusWithEvent[%s]", utils.GroupSnapshotKey(groupSnapshot))

	if groupSnapshot.Status != nil && groupSnapshot.Status.Error != nil && *groupSnapshot.Status.Error.Message == message {
		klog.V(4).Infof("updateGroupSnapshotErrorStatusWithEvent[%s]: the same error %v is already set", groupSnapshot.Name, groupSnapshot.Status.Error)
		return nil
	}
	groupSnapshotClone := groupSnapshot.DeepCopy()
	if groupSnapshotClone.Status == nil {
		groupSnapshotClone.Status = &crdv1beta2.VolumeGroupSnapshotStatus{}
	}
	statusError := &crdv1.VolumeSnapshotError{
		Time: &metav1.Time{
			Time: time.Now(),
		},
		Message: &message,
	}
	groupSnapshotClone.Status.Error = statusError
	// Only update ReadyToUse in VolumeGroupSnapshot's Status to false if setReadyToFalse is true.
	if setReadyToFalse {
		ready := false
		groupSnapshotClone.Status.ReadyToUse = &ready
	}
	newSnapshot, err := ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshots(groupSnapshotClone.Namespace).UpdateStatus(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})

	// Emit the event even if the status update fails so that user can see the error
	ctrl.eventRecorder.Event(newSnapshot, eventtype, reason, message)

	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshot[%s] error status failed %v", utils.GroupSnapshotKey(groupSnapshot), err)
		return err
	}

	_, err = ctrl.storeGroupSnapshotUpdate(newSnapshot)
	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshot[%s] error status: cannot update internal cache %v", utils.GroupSnapshotKey(groupSnapshot), err)
		return err
	}

	return nil
}

// SetDefaultGroupSnapshotClass is a helper function to figure out the default
// group snapshot class.
// For pre-provisioned case, it's an no-op.
// For dynamic provisioning, it gets the default GroupSnapshotClasses in the
// system if there is any (could be multiple), and finds the one with the same
// CSI Driver as a PV from which a group snapshot will be taken.
func (ctrl *csiSnapshotCommonController) SetDefaultGroupSnapshotClass(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) (*crdv1beta2.VolumeGroupSnapshotClass, *crdv1beta2.VolumeGroupSnapshot, error) {
	klog.V(5).Infof("SetDefaultGroupSnapshotClass for group snapshot [%s]", groupSnapshot.Name)

	if groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName != nil {
		// don't return error for pre-provisioned group snapshots
		klog.V(5).Infof("Don't need to find GroupSnapshotClass for pre-provisioned group snapshot [%s]", groupSnapshot.Name)
		return nil, groupSnapshot, nil
	}

	// Find default group snapshot class if available
	list, err := ctrl.groupSnapshotClassLister.List(labels.Everything())
	if err != nil {
		return nil, groupSnapshot, err
	}

	pvDriver, err := ctrl.pvDriverFromGroupSnapshot(groupSnapshot)
	if err != nil {
		klog.Errorf("failed to get pv csi driver from group snapshot %s/%s: %q", groupSnapshot.Namespace, groupSnapshot.Name, err)
		return nil, groupSnapshot, err
	}

	defaultClasses := []*crdv1beta2.VolumeGroupSnapshotClass{}
	for _, groupSnapshotClass := range list {
		if utils.IsVolumeGroupSnapshotClassDefaultAnnotation(groupSnapshotClass.ObjectMeta) && pvDriver == groupSnapshotClass.Driver {
			defaultClasses = append(defaultClasses, groupSnapshotClass)
			klog.V(5).Infof("get defaultGroupClass added: %s, driver: %s", groupSnapshotClass.Name, pvDriver)
		}
	}
	if len(defaultClasses) == 0 {
		return nil, groupSnapshot, fmt.Errorf("cannot find default group snapshot class")
	}
	if len(defaultClasses) > 1 {
		klog.V(4).Infof("get DefaultGroupSnapshotClass %d defaults found", len(defaultClasses))
		return nil, groupSnapshot, fmt.Errorf("%d default snapshot classes were found", len(defaultClasses))
	}
	klog.V(5).Infof("setDefaultGroupSnapshotClass [%s]: default VolumeGroupSnapshotClassName [%s]", groupSnapshot.Name, defaultClasses[0].Name)
	groupSnapshotClone := groupSnapshot.DeepCopy()
	groupSnapshotClone.Spec.VolumeGroupSnapshotClassName = &(defaultClasses[0].Name)
	newGroupSnapshot, err := ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshots(groupSnapshotClone.Namespace).Update(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})
	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshot[%s] default group snapshot class failed %v", utils.GroupSnapshotKey(groupSnapshot), err)
	}
	_, updateErr := ctrl.storeGroupSnapshotUpdate(newGroupSnapshot)
	if updateErr != nil {
		// We will get a "group snapshot update" event soon, this is not a big error
		klog.V(4).Infof("setDefaultSnapshotClass [%s]: cannot update internal cache: %v", utils.GroupSnapshotKey(groupSnapshot), updateErr)
	}

	return defaultClasses[0], newGroupSnapshot, nil
}

// pvDriverFromGroupSnapshot is a helper function to get the CSI driver name from the targeted persistent volume.
// It looks up every PVC from which the group snapshot is specified to be created from, and looks for the PVC's
// corresponding PV. Bi-directional binding will be verified between PVC and PV before the PV's CSI driver is returned.
// For an non-CSI volume, it returns an error immediately as it's not supported.
func (ctrl *csiSnapshotCommonController) pvDriverFromGroupSnapshot(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) (string, error) {
	pvs, err := ctrl.getVolumesFromVolumeGroupSnapshot(groupSnapshot)
	if err != nil {
		return "", err
	}
	// Take any volume to get the driver
	if pvs[0].Spec.PersistentVolumeSource.CSI == nil {
		return "", fmt.Errorf("snapshotting non-CSI volumes is not supported, group snapshot:%s/%s", groupSnapshot.Namespace, groupSnapshot.Name)
	}
	return pvs[0].Spec.PersistentVolumeSource.CSI.Driver, nil
}

// getVolumesFromVolumeGroupSnapshot returns the list of PersistentVolume from a VolumeGroupSnapshot.
func (ctrl *csiSnapshotCommonController) getVolumesFromVolumeGroupSnapshot(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) ([]*v1.PersistentVolume, error) {
	var pvReturnList []*v1.PersistentVolume
	pvcs, err := ctrl.getClaimsFromVolumeGroupSnapshot(groupSnapshot)
	if err != nil {
		return nil, err
	}

	for _, pvc := range pvcs {
		if pvc.Status.Phase != v1.ClaimBound {
			return nil, fmt.Errorf("the PVC %s is not yet bound to a PV, will not attempt to take a group snapshot", pvc.Name)
		}
		pvName := pvc.Spec.VolumeName
		pv, err := ctrl.client.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve PV %s from the API server: %q", pvName, err)
		}

		// Verify binding between PV/PVC is still valid
		bound := ctrl.isVolumeBoundToClaim(pv, &pvc)
		if bound == false {
			klog.Warningf("binding between PV %s and PVC %s is broken", pvName, pvc.Name)
			return nil, fmt.Errorf("claim in dataSource not bound or invalid")
		}
		pvReturnList = append(pvReturnList, pv)
		klog.V(5).Infof("getVolumeFromVolumeGroupSnapshot: group snapshot [%s] PV name [%s]", groupSnapshot.Name, pvName)
	}

	return pvReturnList, nil
}

// getClaimsFromVolumeGroupSnapshot is a helper function to get a list of PVCs from VolumeGroupSnapshot.
func (ctrl *csiSnapshotCommonController) getClaimsFromVolumeGroupSnapshot(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) ([]v1.PersistentVolumeClaim, error) {
	labelSelector := groupSnapshot.Spec.Source.Selector

	// Get PVC that has group snapshot label applied.
	pvcList, err := ctrl.client.CoreV1().PersistentVolumeClaims(groupSnapshot.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labels.Set(labelSelector.MatchLabels).String()})
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs with label selector %s: %q", metav1.FormatLabelSelector(labelSelector), err)
	}
	if len(pvcList.Items) == 0 {
		return nil, fmt.Errorf("label selector %s for group snapshot not applied to any PVC", metav1.FormatLabelSelector(labelSelector))
	}
	return pvcList.Items, nil
}

// updateGroupSnapshot runs in worker thread and handles "groupsnapshot added",
// "groupsnapshot updated" and "periodic sync" events.
func (ctrl *csiSnapshotCommonController) updateGroupSnapshot(ctx context.Context, groupSnapshot *crdv1beta2.VolumeGroupSnapshot) error {
	// Store the new group snapshot version in the cache and do not process it
	// if this is an old version.
	klog.V(5).Infof("updateGroupSnapshot %q", utils.GroupSnapshotKey(groupSnapshot))
	newGroupSnapshot, err := ctrl.storeGroupSnapshotUpdate(groupSnapshot)
	if err != nil {
		klog.Errorf("%v", err)
	}
	if !newGroupSnapshot {
		return nil
	}

	err = ctrl.syncGroupSnapshot(ctx, groupSnapshot)
	if err != nil {
		if apierrs.IsConflict(err) {
			// Version conflict error happens quite often and the controller
			// recovers from it easily.
			klog.V(3).Infof("could not sync group snapshot %q: %+v", utils.GroupSnapshotKey(groupSnapshot), err)
		} else {
			klog.Errorf("could not sync group snapshot %q: %+v", utils.GroupSnapshotKey(groupSnapshot), err)
		}
		return err
	}
	return nil
}

// deleteGroupSnapshot runs in worker thread and handles "groupsnapshot deleted" event.
func (ctrl *csiSnapshotCommonController) deleteGroupSnapshot(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) {
	_ = ctrl.groupSnapshotStore.Delete(groupSnapshot)
	klog.V(4).Infof("group snapshot %q deleted", utils.GroupSnapshotKey(groupSnapshot))

	driverName, err := ctrl.getGroupSnapshotDriverName(groupSnapshot)
	if err != nil {
		klog.Errorf("failed to getGroupSnapshotDriverName while recording metrics for group snapshot %q: %v", utils.GroupSnapshotKey(groupSnapshot), err)
	} else {
		deleteOperationKey := metrics.NewOperationKey(metrics.DeleteGroupSnapshotOperationName, groupSnapshot.UID)
		ctrl.metricsManager.RecordMetrics(deleteOperationKey, metrics.NewSnapshotOperationStatus(metrics.SnapshotStatusTypeSuccess), driverName)
	}

	groupSnapshotContentName := ""
	if groupSnapshot.Status != nil && groupSnapshot.Status.BoundVolumeGroupSnapshotContentName != nil {
		groupSnapshotContentName = *groupSnapshot.Status.BoundVolumeGroupSnapshotContentName
	}
	if groupSnapshotContentName == "" {
		klog.V(5).Infof("deleteGroupSnapshot[%q]: group snapshot content not bound", utils.GroupSnapshotKey(groupSnapshot))
		return
	}

	// sync the group snapshot content when its group snapshot is deleted.  Explicitly sync'ing
	// the group snapshot content here in response to group snapshot deletion prevents the group
	// snapshot content from waiting until the next sync period for its release.
	klog.V(5).Infof("deleteGroupSnapshot[%q]: scheduling sync of group snapshot content %s", utils.GroupSnapshotKey(groupSnapshot), groupSnapshotContentName)
	ctrl.groupSnapshotContentQueue.Add(groupSnapshotContentName)
}

// syncGroupSnapshot is the main controller method to decide what to do with a
// group snapshot. It's invoked by appropriate cache.Controller callbacks when
// a group snapshot is created, updated or periodically synced. We do not
// differentiate between these events.
// For easier readability, it is split into syncUnreadyGroupSnapshot and syncReadyGroupSnapshot
func (ctrl *csiSnapshotCommonController) syncGroupSnapshot(ctx context.Context, groupSnapshot *crdv1beta2.VolumeGroupSnapshot) error {
	klog.V(5).Infof("synchronizing VolumeGroupSnapshot[%s]", utils.GroupSnapshotKey(groupSnapshot))

	klog.V(5).Infof("syncGroupSnapshot [%s]: check if we should remove finalizer on group snapshot PVC source and remove it if we can", utils.GroupSnapshotKey(groupSnapshot))

	// Proceed with group snapshot deletion and remove finalizers when needed
	if groupSnapshot.ObjectMeta.DeletionTimestamp != nil {
		return ctrl.processGroupSnapshotWithDeletionTimestamp(ctx, groupSnapshot)
	}

	klog.V(5).Infof("syncGroupSnapshot[%s]: validate group snapshot to make sure source has been correctly specified", utils.GroupSnapshotKey(groupSnapshot))
	if (groupSnapshot.Spec.Source.Selector == nil && groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName == nil) ||
		(groupSnapshot.Spec.Source.Selector != nil && groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName != nil) {
		err := fmt.Errorf("Exactly one of Selector and VolumeGroupSnapshotContentName should be specified")
		klog.Errorf("syncGroupSnapshot[%s]: validation error, %s", utils.GroupSnapshotKey(groupSnapshot), err.Error())
		ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotValidationError", err.Error())
		return err
	}

	klog.V(5).Infof("syncGroupSnapshot: check if we should add finalizers on group snapshot [%s]", utils.GroupSnapshotKey(groupSnapshot))
	if err := ctrl.checkandAddGroupSnapshotFinalizers(groupSnapshot); err != nil {
		klog.Errorf("error checkandAddGroupSnapshotFinalizers for group snapshot [%s]: %v", utils.GroupSnapshotKey(groupSnapshot), err)
		ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeWarning, "GroupSnapshotFinalizerError", fmt.Sprintf("Failed to check and update group snapshot: %s", err.Error()))
		return err
	}

	// Need to build or update groupSnapshot.Status in following cases:
	// 1) groupSnapshot.Status is nil
	// 2) groupSnapshot.Status.ReadyToUse is false
	// 3) groupSnapshot.Status.IsBoundVolumeGroupSnapshotContentNameSet is not set
	// 4) groupSnapshot.Status.IsVolumeSnapshotRefListSet is not set
	if !utils.IsGroupSnapshotReady(groupSnapshot) || !utils.IsBoundVolumeGroupSnapshotContentNameSet(groupSnapshot) {
		return ctrl.syncUnreadyGroupSnapshot(ctx, groupSnapshot)
	}
	return ctrl.syncReadyGroupSnapshot(groupSnapshot)
}

// syncReadyGroupSnapshot checks the group snapshot which has been bound to group
// snapshot content successfully before.
// If there is any problem with the binding (e.g., group snapshot points to a
// non-existent group snapshot content), update the group snapshot status and emit event.
func (ctrl *csiSnapshotCommonController) syncReadyGroupSnapshot(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) error {
	if !utils.IsBoundVolumeGroupSnapshotContentNameSet(groupSnapshot) {
		return fmt.Errorf("group snapshot %s is not bound to a group snapshot content", utils.GroupSnapshotKey(groupSnapshot))
	}
	groupSnapshotContent, err := ctrl.getGroupSnapshotContentFromStore(*groupSnapshot.Status.BoundVolumeGroupSnapshotContentName)
	if err != nil {
		return nil
	}
	if groupSnapshotContent == nil {
		// this meant there is no matching group snapshot content in cache found
		// update status of the group snapshot and return
		return ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotContentMissing", "VolumeGroupSnapshotContent is missing")
	}
	klog.V(5).Infof("syncReadyGroupSnapshot[%s]: VolumeGroupSnapshotContent %q found", utils.GroupSnapshotKey(groupSnapshot), groupSnapshotContent.Name)
	// check binding from group snapshot content side to make sure the binding is still valid
	if !utils.IsVolumeGroupSnapshotRefSet(groupSnapshot, groupSnapshotContent) {
		// group snapshot is bound but group snapshot content is not pointing to the group snapshot
		return ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotMisbound", "VolumeGroupSnapshotContent is not bound to the VolumeGroupSnapshot correctly")
	}

	// everything is verified, return
	return nil
}

// getGroupSnapshotContentFromStore tries to find a VolumeGroupSnapshotContent from group
// snapshot content cache store by name.
// Note that if no VolumeGroupSnapshotContent exists in the cache store and no error
// encountered, it returns (nil, nil)
func (ctrl *csiSnapshotCommonController) getGroupSnapshotContentFromStore(contentName string) (*crdv1beta2.VolumeGroupSnapshotContent, error) {
	obj, exist, err := ctrl.groupSnapshotContentStore.GetByKey(contentName)
	if err != nil {
		// should never reach here based on implementation at:
		// https://github.com/kubernetes/client-go/blob/master/tools/cache/store.go#L226
		return nil, err
	}
	if !exist {
		// not able to find a matching group snapshot content
		return nil, nil
	}
	groupSnapshotContent, ok := obj.(*crdv1beta2.VolumeGroupSnapshotContent)
	if !ok {
		return nil, fmt.Errorf("expected VolumeGroupSnapshotContent, got %+v", obj)
	}
	return groupSnapshotContent, nil
}

// syncUnreadyGroupSnapshot is the main controller method to decide what to do
// with a group snapshot which is not set to ready.
func (ctrl *csiSnapshotCommonController) syncUnreadyGroupSnapshot(ctx context.Context, groupSnapshot *crdv1beta2.VolumeGroupSnapshot) error {
	uniqueGroupSnapshotName := utils.GroupSnapshotKey(groupSnapshot)
	klog.V(5).Infof("syncUnreadyGroupSnapshot %s", uniqueGroupSnapshotName)
	driverName, err := ctrl.getGroupSnapshotDriverName(groupSnapshot)
	if err != nil {
		klog.Errorf("failed to getGroupSnapshotDriverName while recording metrics for groupsnapshot %q: %s", utils.GroupSnapshotKey(groupSnapshot), err)
	}

	groupSnapshotProvisionType := metrics.DynamicGroupSnapshotType
	if groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName != nil {
		groupSnapshotProvisionType = metrics.PreProvisionedGroupSnapshotType
	}

	// Start metrics operations for volumegroupsnapshot
	if !utils.IsGroupSnapshotCreated(groupSnapshot) {
		// Only start CreateGroupSnapshot operation if the groupsnapshot has not been cut
		ctrl.metricsManager.OperationStart(
			metrics.NewOperationKey(metrics.CreateGroupSnapshotOperationName, groupSnapshot.UID),
			metrics.NewOperationValue(driverName, groupSnapshotProvisionType),
		)
	}
	ctrl.metricsManager.OperationStart(
		metrics.NewOperationKey(metrics.CreateGroupSnapshotAndReadyOperationName, groupSnapshot.UID),
		metrics.NewOperationValue(driverName, groupSnapshotProvisionType),
	)

	// Pre-provisioned snapshot
	if groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName != nil {
		groupSnapshotContent, err := ctrl.getPreprovisionedGroupSnapshotContentFromStore(groupSnapshot)
		if err != nil {
			return err
		}

		// if no group snapshot content found yet, update status and return
		if groupSnapshotContent == nil {
			// can not find the desired VolumeGroupSnapshotContent from cache store
			ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotContentMissing", "VolumeGroupSnapshotContent is missing")
			klog.V(4).Infof("syncUnreadyGroupSnapshot[%s]: group snapshot content %q requested but not found, will try again", utils.GroupSnapshotKey(groupSnapshot), *groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName)

			return fmt.Errorf("group snapshot %s requests an non-existing group snapshot content %s", utils.GroupSnapshotKey(groupSnapshot), *groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName)
		}

		// Set VolumeGroupSnapshotRef UID
		newGroupSnapshotContent, err := ctrl.checkAndBindGroupSnapshotContent(groupSnapshot, groupSnapshotContent)
		if err != nil {
			// group snapshot is bound but group snapshot content is not bound to group snapshot correctly
			ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotBindFailed", fmt.Sprintf("GroupSnapshot failed to bind VolumeGroupSnapshotContent, %v", err))
			return fmt.Errorf("group snapshot %s is bound, but VolumeGroupSnapshotContent %s is not bound to the VolumeGroupSnapshot correctly, %v", uniqueGroupSnapshotName, groupSnapshotContent.Name, err)
		}

		// update group snapshot status
		klog.V(5).Infof("syncUnreadyGroupSnapshot [%s]: trying to update group snapshot status", utils.GroupSnapshotKey(groupSnapshot))
		if _, err = ctrl.updateGroupSnapshotStatus(groupSnapshot, newGroupSnapshotContent); err != nil {
			// update group snapshot status failed
			klog.V(4).Infof("failed to update group snapshot %s status: %v", utils.GroupSnapshotKey(groupSnapshot), err)
			ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, false, v1.EventTypeWarning, "GroupSnapshotStatusUpdateFailed", fmt.Sprintf("GroupSnapshot status update failed, %v", err))
			return err
		}

		return nil
	}

	// groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName == nil - dynamically created group snapshot
	klog.V(5).Infof("getDynamicallyProvisionedGroupContentFromStore for snapshot %s", uniqueGroupSnapshotName)
	contentObj, err := ctrl.getDynamicallyProvisionedGroupContentFromStore(groupSnapshot)
	if err != nil {
		klog.V(4).Infof("getDynamicallyProvisionedGroupContentFromStore[%s]: error when getting group snapshot content for group snapshot %v", uniqueGroupSnapshotName, err)
		return err
	}

	if contentObj != nil {
		klog.V(5).Infof("Found VolumeGroupSnapshotContent object %s for group snapshot %s", contentObj.Name, uniqueGroupSnapshotName)
		if contentObj.Spec.Source.GroupSnapshotHandles != nil {
			ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotHandleSet", fmt.Sprintf("GroupSnapshot handle should not be set in group snapshot content %s for dynamic provisioning", uniqueGroupSnapshotName))
			return fmt.Errorf("VolumeGroupSnapshotHandle should not be set in the group snapshot content for dynamic provisioning for group snapshot %s", uniqueGroupSnapshotName)
		}

		newGroupSnapshotContentObj, err := ctrl.createSnapshotsForGroupSnapshotContent(ctx, contentObj, groupSnapshot)
		if err != nil {
			klog.V(4).Infof("createSnapshotsForGroupSnapshotContent[%s]: failed to create snapshots and snapshotcontents for group snapshot %v: %v",
				contentObj.Name, groupSnapshot.Name, err.Error())
			return err
		}

		updatedGroupSnapshot, err := ctrl.bindandUpdateVolumeGroupSnapshot(newGroupSnapshotContentObj, groupSnapshot)
		if err != nil {
			klog.V(4).Infof("bindandUpdateVolumeGroupSnapshot[%s]: failed to bind group snapshot content [%s] to group snapshot %v", uniqueGroupSnapshotName, contentObj.Name, err)
			return err
		}
		klog.V(5).Infof("bindandUpdateVolumeGroupSnapshot %v", updatedGroupSnapshot)
		return nil
	}

	// If reach here, it is a dynamically provisioned group snapshot, and the VolumeGroupSnapshotContent object is not yet created.
	var groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent
	if groupSnapshotContent, err = ctrl.createGroupSnapshotContent(groupSnapshot); err != nil {
		ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotContentCreationFailed", fmt.Sprintf("failed to create group snapshot content with error %v", err))
		return err
	}

	// Update group snapshot status with BoundVolumeGroupSnapshotContentName
	klog.V(5).Infof("syncUnreadyGroupSnapshot [%s]: trying to update group snapshot status", utils.GroupSnapshotKey(groupSnapshot))
	if _, err = ctrl.updateGroupSnapshotStatus(groupSnapshot, groupSnapshotContent); err != nil {
		// update group snapshot status failed
		ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, false, v1.EventTypeWarning, "GroupSnapshotStatusUpdateFailed", fmt.Sprintf("GroupSnapshot status update failed, %v", err))
		return err
	}
	return nil
}

func (ctrl *csiSnapshotCommonController) createSnapshotsForGroupSnapshotContent(
	ctx context.Context,
	groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent,
	groupSnapshot *crdv1beta2.VolumeGroupSnapshot,
) (*crdv1beta2.VolumeGroupSnapshotContent, error) {
	// No status is present, or no volume snapshot was provisioned.
	// Let's wait for the snapshotter sidecar to fill it.
	if groupSnapshotContent.Status == nil || len(groupSnapshotContent.Status.VolumeSnapshotInfoList) == 0 {
		return groupSnapshotContent, nil
	}

	// No volume group snapshot handle is present.
	// Let's wait for the snapshotter sidecar to fill it.
	if groupSnapshotContent.Status.VolumeGroupSnapshotHandle == nil {
		return groupSnapshotContent, nil
	}

	// The contents of the volume group snapshot class are needed to set the
	// metadata containing the secrets to recover the snapshots
	if groupSnapshot.Spec.VolumeGroupSnapshotClassName == nil {
		return groupSnapshotContent, fmt.Errorf(
			"createSnapshotsForGroupSnapshotContent: internal error: cannot find reference to volume group snapshot class")
	}

	groupSnapshotClass, err := ctrl.groupSnapshotClassLister.Get(*groupSnapshot.Spec.VolumeGroupSnapshotClassName)
	if err != nil {
		return groupSnapshotContent, fmt.Errorf(
			"createSnapshotsForGroupSnapshotContent: failed to get volume snapshot class %s: %q",
			*groupSnapshot.Spec.VolumeGroupSnapshotClassName, err)
	}

	groupSnapshotSecret, err := utils.GetGroupSnapshotSecretReference(
		utils.GroupSnapshotterSecretParams,
		groupSnapshotClass.Parameters,
		groupSnapshotContent.GetObjectMeta().GetName(), nil)
	if err != nil {
		return groupSnapshotContent, fmt.Errorf(
			"createSnapshotsForGroupSnapshotContent: failed to get secret reference for group snapshot content %s: %v",
			groupSnapshotContent.Name, err)
	}

	// Phase 1: create the VolumeSnapshotContent and VolumeSnapshot objects
	klog.V(4).Infof(
		"createSnapshotsForGroupSnapshotContent[%s]: creating volumesnapshots and volumesnapshotcontent for group snapshot content",
		groupSnapshotContent.Name)

	for _, snapshot := range groupSnapshotContent.Status.VolumeSnapshotInfoList {
		snapshotHandle := snapshot.SnapshotHandle
		volumeHandle := snapshot.VolumeHandle

		pv, err := ctrl.findPersistentVolumeByCSIDriverHandle(groupSnapshotContent.Spec.Driver, volumeHandle)
		if err != nil {
			klog.Errorf(
				"updateGroupSnapshotContentStatus: error while finding PV for volumeHandle:[%s] and CSI driver:[%s]: %s",
				volumeHandle,
				groupSnapshotContent.Spec.Driver,
				err)
		}

		volumeSnapshotContentName := getSnapshotContentNameForVolumeGroupSnapshotContent(
			string(groupSnapshot.UID), volumeHandle)

		volumeSnapshotName := getSnapshotNameForVolumeGroupSnapshotContent(
			string(groupSnapshot.UID), volumeHandle)

		volumeSnapshotNamespace := groupSnapshotContent.Spec.VolumeGroupSnapshotRef.Namespace

		volumeSnapshotContent := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{
				Name: volumeSnapshotContentName,
				Annotations: map[string]string{
					utils.VolumeGroupSnapshotHandleAnnotation: *groupSnapshotContent.Status.VolumeGroupSnapshotHandle,
				},
			},
			Spec: crdv1.VolumeSnapshotContentSpec{
				VolumeSnapshotRef: v1.ObjectReference{
					Kind:      "VolumeSnapshot",
					Name:      volumeSnapshotName,
					Namespace: volumeSnapshotNamespace,
				},
				DeletionPolicy: groupSnapshotContent.Spec.DeletionPolicy,
				Driver:         groupSnapshotContent.Spec.Driver,
				Source: crdv1.VolumeSnapshotContentSource{
					VolumeHandle: &volumeHandle,
				},
			},
			// The status will be set in a separate patch request by
			// common snapshot controller, using the information from
			// the VolumeGroupSnapshotContent object.
		}

		if pv != nil {
			volumeSnapshotContent.Spec.SourceVolumeMode = pv.Spec.VolumeMode
		}

		if groupSnapshotSecret != nil {
			klog.V(5).Infof("createSnapshotsForGroupSnapshotContent: set annotation [%s] on volume snapshot content [%s].", utils.AnnDeletionSecretRefName, volumeSnapshotContent.Name)
			metav1.SetMetaDataAnnotation(&volumeSnapshotContent.ObjectMeta, utils.AnnDeletionSecretRefName, groupSnapshotSecret.Name)

			klog.V(5).Infof("createSnapshotsForGroupSnapshotContent: set annotation [%s] on volume snapshot content [%s].", utils.AnnDeletionSecretRefNamespace, volumeSnapshotContent.Name)
			metav1.SetMetaDataAnnotation(&volumeSnapshotContent.ObjectMeta, utils.AnnDeletionSecretRefNamespace, groupSnapshotSecret.Namespace)
		}

		volumeSnapshot := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      volumeSnapshotName,
				Namespace: volumeSnapshotNamespace,
				OwnerReferences: []metav1.OwnerReference{
					utils.BuildVolumeGroupSnapshotOwnerReference(groupSnapshot),
				},
				Finalizers: []string{utils.VolumeSnapshotInGroupFinalizer},
			},
			// The spec stanza is set immediately
			// The status will be set by VolumeSnapshot reconciler
		}

		if pv != nil {
			volumeSnapshot.Spec.Source.PersistentVolumeClaimName = &pv.Spec.ClaimRef.Name
		} else {
			// If no persistent volume was found, set the PVC name to empty
			var emptyString string
			volumeSnapshot.Spec.Source.PersistentVolumeClaimName = &emptyString
		}

		createdVolumeSnapshotContent, err := ctrl.clientset.SnapshotV1().VolumeSnapshotContents().Create(ctx, volumeSnapshotContent, metav1.CreateOptions{})
		if err != nil && !apierrs.IsAlreadyExists(err) {
			return groupSnapshotContent, fmt.Errorf(
				"createSnapshotsForGroupSnapshotContent: creating volumesnapshotcontent %w", err)
		}

		createdVolumeSnapshot, err := ctrl.clientset.SnapshotV1().VolumeSnapshots(volumeSnapshotNamespace).Create(ctx, volumeSnapshot, metav1.CreateOptions{})
		if err != nil && !apierrs.IsAlreadyExists(err) {
			return groupSnapshotContent, fmt.Errorf(
				"createSnapshotsForGroupSnapshotContent: creating volumesnapshot %w", err)
		}

		// bind the volume snapshot content to the volume snapshot
		// like a dynamically provisioned snapshot would do
		volumeSnapshotContent.Spec.VolumeSnapshotRef.UID = createdVolumeSnapshot.UID
		_, err = utils.PatchVolumeSnapshotContent(volumeSnapshotContent, []utils.PatchOp{
			{
				Op:    "replace",
				Path:  "/spec/volumeSnapshotRef/uid",
				Value: volumeSnapshotContent.Spec.VolumeSnapshotRef.UID,
			},
		}, ctrl.clientset)
		if err != nil {
			return groupSnapshotContent, fmt.Errorf(
				"createSnapshotsForGroupSnapshotContent: binding volumesnapshotcontent to volumesnapshot %w", err)
		}

		// bind the volume snapshot to the volume snapshot content
		// like a dynamically provisioned snapshot would do
		_, err = utils.PatchVolumeSnapshot(createdVolumeSnapshot, []utils.PatchOp{
			{
				Op:    "replace",
				Path:  "/status",
				Value: &crdv1.VolumeSnapshotStatus{},
			},
			{
				Op:    "replace",
				Path:  "/status/boundVolumeSnapshotContentName",
				Value: volumeSnapshotContentName,
			},
		}, ctrl.clientset, "status")
		if err != nil {
			return groupSnapshotContent, fmt.Errorf(
				"createSnapshotsForGroupSnapshotContent: binding volumesnapshot to volumesnapshotcontent %w", err)
		}

		// set the snapshot handle and the group snapshot handle
		// inside the volume snapshot content to allow
		// the CSI Snapshotter sidecar to reconcile its status
		_, err = utils.PatchVolumeSnapshotContent(createdVolumeSnapshotContent, []utils.PatchOp{
			{
				Op:    "replace",
				Path:  "/status",
				Value: &crdv1.VolumeSnapshotContentStatus{},
			},
			{
				Op:    "replace",
				Path:  "/status/snapshotHandle",
				Value: snapshotHandle,
			},
			{
				Op:    "replace",
				Path:  "/status/volumeGroupSnapshotHandle",
				Value: groupSnapshotContent.Status.VolumeGroupSnapshotHandle,
			},
			{
				Op:    "replace",
				Path:  "/status/creationTime",
				Value: snapshot.CreationTime,
			},
			{
				Op:    "replace",
				Path:  "/status/restoreSize",
				Value: snapshot.RestoreSize,
			},
			{
				Op:    "replace",
				Path:  "/status/readyToUse",
				Value: snapshot.ReadyToUse,
			},
		}, ctrl.clientset, "status")
		if err != nil {
			return groupSnapshotContent, fmt.Errorf(
				"createSnapshotsForGroupSnapshotContent: setting snapshotHandle in volumesnapshotcontent %w", err)
		}

	}

	return groupSnapshotContent, nil
}

// findPersistentVolumeByCSIDriverHandle looks at an existing PersistentVolume
// by CSI driver name and volume handle.
func (ctrl *csiSnapshotCommonController) findPersistentVolumeByCSIDriverHandle(driverName, volumeHandle string) (*v1.PersistentVolume, error) {
	pvList, err := ctrl.pvIndexer.ByIndex(
		utils.CSIDriverHandleIndexName,
		utils.PersistentVolumeKeyFuncByCSIDriverHandle(driverName, volumeHandle),
	)
	switch {
	case err != nil:
		return nil, err

	case len(pvList) == 0:
		return nil, nil

	case len(pvList) > 1:
		klog.Errorf(
			"findPersistentVolumeByCSIDriverHandle: multiple PVs found for for volumeHandle:[%s] and CSI driver:[%s]",
			volumeHandle,
			driverName)
		return nil, fmt.Errorf("multiple PVs found")

	default:
		if pvObject, ok := pvList[0].(*v1.PersistentVolume); ok {
			return pvObject, nil
		}

		klog.Errorf(
			"findPersistentVolumeByCSIDriverHandle: found erroneous content in the index")
		klog.V(5).Info("findPersistentVolumeByCSIDriverHandle: erroneous content", pvList[0])
		return nil, fmt.Errorf("found erroneous indexed content")
	}
}

// getSnapshotNameForVolumeGroupSnapshotContent returns a unique snapshot name for a VolumeGroupSnapshotContent.
func getSnapshotNameForVolumeGroupSnapshotContent(groupSnapshotUUID, volumeHandle string) string {
	return fmt.Sprintf("snapshot-%x", sha256.Sum256([]byte(groupSnapshotUUID+volumeHandle)))
}

// getSnapshotContentNameForVolumeGroupSnapshotContent returns a unique content name for the
// passed in VolumeGroupSnapshotContent.
func getSnapshotContentNameForVolumeGroupSnapshotContent(groupSnapshotUUID, volumeHandle string) string {
	return fmt.Sprintf("snapcontent-%x", sha256.Sum256([]byte(groupSnapshotUUID+volumeHandle)))
}

// getPreprovisionedGroupSnapshotContentFromStore tries to find a pre-provisioned
// volume group snapshot content object from group snapshot content cache store
// for the passed in VolumeGroupSnapshot.
// Note that this function assumes the passed in VolumeGroupSnapshot is a pre-provisioned
// one, i.e., groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName != nil.
// If no matching group snapshot content is found, it returns (nil, nil).
// If it found a group snapshot content which is not a pre-provisioned one, it
// updates the status of the group snapshot with an event and returns an error.
// If it found a group snapshot content which does not point to the passed in
// VolumeGroupSnapshot, it updates the status of the group snapshot with an event
// and returns an error.
// Otherwise, the found group snapshot content will be returned.
func (ctrl *csiSnapshotCommonController) getPreprovisionedGroupSnapshotContentFromStore(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) (*crdv1beta2.VolumeGroupSnapshotContent, error) {
	contentName := *groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName
	if contentName == "" {
		return nil, fmt.Errorf("empty VolumeGroupSnapshotContentName for group snapshot %s", utils.GroupSnapshotKey(groupSnapshot))
	}
	groupSnapshotContent, err := ctrl.getGroupSnapshotContentFromStore(contentName)
	if err != nil {
		return nil, err
	}
	if groupSnapshotContent == nil {
		// can not find the desired VolumeGroupSnapshotContent from cache store
		return nil, nil
	}
	// check whether the content is a pre-provisioned VolumeGroupSnapshotContent
	if groupSnapshotContent.Spec.Source.GroupSnapshotHandles == nil {
		// found a group snapshot content which represents a dynamically provisioned group snapshot
		// update the group snapshot and return an error
		ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotContentMismatch", "VolumeGroupSnapshotContent is dynamically provisioned while expecting a pre-provisioned one")
		klog.V(4).Infof("sync group snapshot[%s]: group snapshot content %q is dynamically provisioned while expecting a pre-provisioned one", utils.GroupSnapshotKey(groupSnapshot), contentName)
		return nil, fmt.Errorf("group snapshot %s expects a pre-provisioned VolumeGroupSnapshotContent %s but gets a dynamically provisioned one", utils.GroupSnapshotKey(groupSnapshot), contentName)
	}
	// verify the group snapshot content points back to the group snapshot
	ref := groupSnapshotContent.Spec.VolumeGroupSnapshotRef
	if ref.Name != groupSnapshot.Name || ref.Namespace != groupSnapshot.Namespace || (ref.UID != "" && ref.UID != groupSnapshot.UID) {
		klog.V(4).Infof("sync group snapshot[%s]: VolumeGroupSnapshotContent %s is bound to another group snapshot %v", utils.GroupSnapshotKey(groupSnapshot), contentName, ref)
		msg := fmt.Sprintf("VolumeGroupSnapshotContent [%s] is bound to a different group snapshot", contentName)
		ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotContentMisbound", msg)
		return nil, errors.New(msg)
	}
	return groupSnapshotContent, nil
}

// checkandBindGroupSnapshotContent checks whether the VolumeGroupSnapshotRef in
// the group snapshot content matches the given group snapshot. If match, it binds
// the group snapshot content with the group snapshot. This is for static binding where
// user has specified group snapshot name but not UID of the group snapshot in
// groupSnapshotContent.Spec.VolumeGroupSnapshotRef.
func (ctrl *csiSnapshotCommonController) checkAndBindGroupSnapshotContent(groupSnapshot *crdv1beta2.VolumeGroupSnapshot, groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent) (*crdv1beta2.VolumeGroupSnapshotContent, error) {
	if groupSnapshotContent.Spec.VolumeGroupSnapshotRef.Name != groupSnapshot.Name {
		return nil, fmt.Errorf("Could not bind group snapshot %s and group snapshot content %s, the VolumeGroupSnapshotRef does not match", groupSnapshot.Name, groupSnapshotContent.Name)
	} else if groupSnapshotContent.Spec.VolumeGroupSnapshotRef.UID != "" && groupSnapshotContent.Spec.VolumeGroupSnapshotRef.UID != groupSnapshot.UID {
		return nil, fmt.Errorf("Could not bind group snapshot %s and group snapshot content %s, the VolumeGroupSnapshotRef does not match", groupSnapshot.Name, groupSnapshotContent.Name)
	} else if groupSnapshotContent.Spec.VolumeGroupSnapshotRef.UID != "" && groupSnapshotContent.Spec.VolumeGroupSnapshotClassName != nil {
		return groupSnapshotContent, nil
	}

	patches := []utils.PatchOp{
		{
			Op:    "replace",
			Path:  "/spec/volumeGroupSnapshotRef/uid",
			Value: string(groupSnapshot.UID),
		},
	}
	if groupSnapshot.Spec.VolumeGroupSnapshotClassName != nil {
		className := *(groupSnapshot.Spec.VolumeGroupSnapshotClassName)
		patches = append(patches, utils.PatchOp{
			Op:    "replace",
			Path:  "/spec/volumeGroupSnapshotClassName",
			Value: className,
		})
	}

	newContent, err := utils.PatchVolumeGroupSnapshotContent(groupSnapshotContent, patches, ctrl.clientset)
	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshotContent[%s] error status failed %v", groupSnapshotContent.Name, err)
		return groupSnapshotContent, err
	}

	_, err = ctrl.storeGroupSnapshotContentUpdate(newContent)
	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshotContent[%s] error status: cannot update internal cache %v", newContent.Name, err)
		return newContent, err
	}
	return newContent, nil
}

// updateGroupSnapshotStatus updates group snapshot status based on group snapshot content status
func (ctrl *csiSnapshotCommonController) updateGroupSnapshotStatus(groupSnapshot *crdv1beta2.VolumeGroupSnapshot, groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent) (*crdv1beta2.VolumeGroupSnapshot, error) {
	klog.V(5).Infof("updateGroupSnapshotStatus[%s]", utils.GroupSnapshotKey(groupSnapshot))

	boundContentName := groupSnapshotContent.Name
	var createdAt *time.Time
	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.CreationTime != nil {
		createdAt = &groupSnapshotContent.Status.CreationTime.DeepCopy().Time
	}
	var readyToUse bool
	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.ReadyToUse != nil {
		readyToUse = *groupSnapshotContent.Status.ReadyToUse
	}
	var volumeSnapshotErr *crdv1.VolumeSnapshotError
	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.Error != nil {
		volumeSnapshotErr = groupSnapshotContent.Status.Error.DeepCopy()
	}

	klog.V(5).Infof("updateGroupSnapshotStatus: updating VolumeGroupSnapshot [%+v] based on VolumeGroupSnapshotContentStatus [%+v]", groupSnapshot, groupSnapshotContent.Status)

	groupSnapshotObj, err := ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshots(groupSnapshot.Namespace).Get(context.TODO(), groupSnapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get group snapshot %s from api server: %v", utils.GroupSnapshotKey(groupSnapshot), err)
	}

	var newStatus *crdv1beta2.VolumeGroupSnapshotStatus
	updated := false
	if groupSnapshotObj.Status == nil {
		newStatus = &crdv1beta2.VolumeGroupSnapshotStatus{
			BoundVolumeGroupSnapshotContentName: &boundContentName,
			ReadyToUse:                          &readyToUse,
		}
		if createdAt != nil {
			newStatus.CreationTime = &metav1.Time{Time: *createdAt}
		}
		if volumeSnapshotErr != nil {
			newStatus.Error = volumeSnapshotErr
		}

		updated = true
	} else {
		newStatus = groupSnapshotObj.Status.DeepCopy()
		if newStatus.BoundVolumeGroupSnapshotContentName == nil {
			newStatus.BoundVolumeGroupSnapshotContentName = &boundContentName
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
		if (newStatus.Error == nil && volumeSnapshotErr != nil) || (newStatus.Error != nil && volumeSnapshotErr != nil && newStatus.Error.Time != nil && volumeSnapshotErr.Time != nil && &newStatus.Error.Time != &volumeSnapshotErr.Time) || (newStatus.Error != nil && volumeSnapshotErr == nil) {
			newStatus.Error = volumeSnapshotErr
			updated = true
		}
	}

	if updated {
		groupSnapshotClone := groupSnapshotObj.DeepCopy()
		groupSnapshotClone.Status = newStatus

		// We need to record metrics before updating the status due to a bug causing cache entries after a failed UpdateStatus call.
		// Must meet the following criteria to emit a successful CreateGroupSnapshot status
		// 1. Previous status was nil OR Previous status had a nil CreationTime
		// 2. New status must be non-nil with a non-nil CreationTime
		driverName := groupSnapshotContent.Spec.Driver
		createOperationKey := metrics.NewOperationKey(metrics.CreateGroupSnapshotOperationName, groupSnapshot.UID)

		// Must meet the following criteria to emit a successful CreateGroupSnapshot status
		// 1. Previous status was nil OR Previous status had a nil CreationTime
		// 2. New status must be non-nil with a non-nil CreationTime
		if !utils.IsGroupSnapshotCreated(groupSnapshotObj) && utils.IsGroupSnapshotCreated(groupSnapshotClone) {
			msg := fmt.Sprintf("GroupSnapshot %s was successfully created by the CSI driver.", utils.GroupSnapshotKey(groupSnapshot))
			ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeNormal, "GroupSnapshotCreated", msg)
			ctrl.metricsManager.RecordVolumeGroupSnapshotMetrics(createOperationKey, metrics.NewSnapshotOperationStatus(metrics.SnapshotStatusTypeSuccess), driverName)
		}

		// Must meet the following criteria to emit a successful CreateGroupSnapshotAndReady status
		// 1. Previous status was nil OR Previous status had a nil ReadyToUse OR Previous status had a false ReadyToUse
		// 2. New status must be non-nil with a ReadyToUse as true
		if !utils.IsGroupSnapshotReady(groupSnapshotObj) && utils.IsGroupSnapshotReady(groupSnapshotClone) {
			msg := fmt.Sprintf("GroupSnapshot %s is ready to use.", utils.GroupSnapshotKey(groupSnapshot))
			ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeNormal, "GroupSnapshotReady", msg)
			createAndReadyOperation := metrics.NewOperationKey(metrics.CreateGroupSnapshotAndReadyOperationName, groupSnapshot.UID)
			ctrl.metricsManager.RecordMetrics(createAndReadyOperation, metrics.NewSnapshotOperationStatus(metrics.SnapshotStatusTypeSuccess), driverName)
		}

		newGroupSnapshotObj, err := ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshots(groupSnapshotClone.Namespace).UpdateStatus(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})
		if err != nil {
			return nil, newControllerUpdateError(utils.GroupSnapshotKey(groupSnapshot), err.Error())
		}

		return newGroupSnapshotObj, nil
	}

	return groupSnapshotObj, nil
}

// getDynamicallyProvisionedGroupContentFromStore tries to find a dynamically created
// group snapshot content object for the passed in VolumeGroupSnapshot from the
// group snapshot content store.
// Note that this function assumes the passed in VolumeGroupSnapshot is a dynamic
// one which requests creating a group snapshot from a group of PVCs.
// If no matching VolumeGroupSnapshotContent exists in the group snapshot content
// cache store, it returns (nil, nil)
// If a group snapshot content is found but it's not dynamically provisioned,
// the passed in group snapshot status will be updated with an error along with
// an event, and an error will be returned.
// If a group snapshot content is found but it does not point to the passed in VolumeGroupSnapshot,
// the passed in group snapshot will be updated with an error along with an event,
// and an error will be returned.
func (ctrl *csiSnapshotCommonController) getDynamicallyProvisionedGroupContentFromStore(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) (*crdv1beta2.VolumeGroupSnapshotContent, error) {
	contentName := utils.GetDynamicSnapshotContentNameForGroupSnapshot(groupSnapshot)
	groupSnapshotContent, err := ctrl.getGroupSnapshotContentFromStore(contentName)
	if err != nil {
		return nil, err
	}
	if groupSnapshotContent == nil {
		// no matching group snapshot content with the desired name has been found in cache
		return nil, nil
	}
	// check whether the group snapshot content represents a dynamically provisioned snapshot
	if groupSnapshotContent.Spec.Source.GroupSnapshotHandles != nil {
		ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotContentMismatch", "VolumeGroupSnapshotContent "+contentName+" is pre-provisioned while expecting a dynamically provisioned one")
		klog.V(4).Infof("sync group snapshot[%s]: group snapshot content %s is pre-provisioned while expecting a dynamically provisioned one", utils.GroupSnapshotKey(groupSnapshot), contentName)
		return nil, fmt.Errorf("group snapshot %s expects a dynamically provisioned VolumeGroupSnapshotContent %s but gets a pre-provisioned one", utils.GroupSnapshotKey(groupSnapshot), contentName)
	}
	// check whether the group snapshot content points back to the passed in VolumeGroupSnapshot
	ref := groupSnapshotContent.Spec.VolumeGroupSnapshotRef
	// Unlike a pre-provisioned group snapshot content, whose Spec.VolumeGroupSnapshotRef.UID will be
	// left to be empty to allow binding to a group snapshot, a dynamically provisioned
	// group snapshot content MUST have its Spec.VolumeGroupSnapshotRef.UID set to the group snapshot's
	// UID from which it's been created, thus ref.UID == "" is not a legit case here.
	if ref.Name != groupSnapshot.Name || ref.Namespace != groupSnapshot.Namespace || ref.UID != groupSnapshot.UID {
		klog.V(4).Infof("sync group snapshot[%s]: VolumeGroupSnapshotContent %s is bound to another group snapshot %v", utils.GroupSnapshotKey(groupSnapshot), contentName, ref)
		msg := fmt.Sprintf("VolumeGroupSnapshotContent [%s] is bound to a different group snapshot", contentName)
		ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotContentMisbound", msg)
		return nil, errors.New(msg)
	}
	return groupSnapshotContent, nil
}

// This routine sets snapshot.Spec.Source.VolumeGroupSnapshotContentName
func (ctrl *csiSnapshotCommonController) bindandUpdateVolumeGroupSnapshot(groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent, groupSnapshot *crdv1beta2.VolumeGroupSnapshot) (*crdv1beta2.VolumeGroupSnapshot, error) {
	klog.V(5).Infof("bindandUpdateVolumeGroupSnapshot for group snapshot [%s]: groupSnapshotContent [%s]", groupSnapshot.Name, groupSnapshotContent.Name)
	groupSnapshotObj, err := ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshots(groupSnapshot.Namespace).Get(context.TODO(), groupSnapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get group snapshot %s from api server: %v", utils.GroupSnapshotKey(groupSnapshot), err)
	}

	// Copy the group snapshot object before updating it
	groupSnapshotCopy := groupSnapshotObj.DeepCopy()
	// update group snapshot status
	var updateGroupSnapshot *crdv1beta2.VolumeGroupSnapshot
	klog.V(5).Infof("bindandUpdateVolumeGroupSnapshot [%s]: trying to update group snapshot status", utils.GroupSnapshotKey(groupSnapshotCopy))
	updateGroupSnapshot, err = ctrl.updateGroupSnapshotStatus(groupSnapshotCopy, groupSnapshotContent)
	if err == nil {
		groupSnapshotCopy = updateGroupSnapshot
	}
	if err != nil {
		// update group snapshot status failed
		klog.V(4).Infof("failed to update group snapshot %s status: %v", utils.GroupSnapshotKey(groupSnapshot), err)
		ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshotCopy, true, v1.EventTypeWarning, "GroupSnapshotStatusUpdateFailed", fmt.Sprintf("GroupSnapshot status update failed, %v", err))
		return nil, err
	}

	_, err = ctrl.storeGroupSnapshotUpdate(groupSnapshotCopy)
	if err != nil {
		klog.Errorf("%v", err)
	}

	klog.V(5).Infof("bindandUpdateVolumeGroupSnapshot for group snapshot completed [%#v]", groupSnapshotCopy)
	return groupSnapshotCopy, nil
}

// createGroupSnapshotContent will only be called for dynamic provisioning
func (ctrl *csiSnapshotCommonController) createGroupSnapshotContent(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) (*crdv1beta2.VolumeGroupSnapshotContent, error) {
	klog.Infof("createGroupSnapshotContent: Creating group snapshot content for group snapshot %s through the plugin ...", utils.GroupSnapshotKey(groupSnapshot))

	/*
		TODO: Add PVC finalizer
	*/

	groupSnapshotClass, volumes, contentName, snapshotterSecretRef, err := ctrl.getCreateGroupSnapshotInput(groupSnapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to get input parameters to create group snapshot %s: %q", groupSnapshot.Name, err)
	}

	snapshotRef, err := ref.GetReference(scheme.Scheme, groupSnapshot)
	if err != nil {
		return nil, err
	}
	var volumeHandles []string
	for _, pv := range volumes {
		if pv.Spec.CSI == nil {
			err := fmt.Errorf(
				"cannot snapshot a non-CSI volume for group snapshot %s: %s",
				utils.GroupSnapshotKey(groupSnapshot), pv.Name)
			klog.Error(err.Error())
			ctrl.eventRecorder.Event(
				groupSnapshot,
				v1.EventTypeWarning,
				"CreateGroupSnapshotContentFailed",
				fmt.Sprintf("Cannot snapshot a non-CSI volume: %s", pv.Name),
			)
			return nil, err
		}

		volumeCSIDriver := pv.Spec.CSI.Driver
		classCSIDriver := groupSnapshotClass.Driver
		if volumeCSIDriver != classCSIDriver {
			strErr := fmt.Sprintf(
				"Volume CSI driver (%s) mismatch with VolumeGroupSnapshotClass (%s) %s: %s",
				volumeCSIDriver, classCSIDriver, utils.GroupSnapshotKey(groupSnapshot), pv.Name)
			klog.Error(strErr)
			ctrl.eventRecorder.Event(
				groupSnapshot,
				v1.EventTypeWarning,
				"CreateGroupSnapshotContentFailed",
				strErr,
			)
			return nil, newControllerUpdateError(utils.GroupSnapshotKey(groupSnapshot), strErr)

		}
		volumeHandles = append(volumeHandles, pv.Spec.CSI.VolumeHandle)
	}

	groupSnapshotContent := &crdv1beta2.VolumeGroupSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentName,
		},
		Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
			VolumeGroupSnapshotRef: *snapshotRef,
			Source: crdv1beta2.VolumeGroupSnapshotContentSource{
				VolumeHandles: volumeHandles,
			},
			VolumeGroupSnapshotClassName: &(groupSnapshotClass.Name),
			DeletionPolicy:               groupSnapshotClass.DeletionPolicy,
			Driver:                       groupSnapshotClass.Driver,
		},
	}

	/*
		Add secret reference details
	*/
	if snapshotterSecretRef != nil {
		klog.V(5).Infof("createGroupSnapshotContent: set annotation [%s] on volume group snapshot content [%s].", utils.AnnDeletionGroupSecretRefName, groupSnapshotContent.Name)
		metav1.SetMetaDataAnnotation(&groupSnapshotContent.ObjectMeta, utils.AnnDeletionGroupSecretRefName, snapshotterSecretRef.Name)

		klog.V(5).Infof("creategroupSnapshotContent: set annotation [%s] on volume group snapshot content [%s].", utils.AnnDeletionGroupSecretRefNamespace, groupSnapshotContent.Name)
		metav1.SetMetaDataAnnotation(&groupSnapshotContent.ObjectMeta, utils.AnnDeletionGroupSecretRefNamespace, snapshotterSecretRef.Namespace)
	}

	var updateGroupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent
	klog.V(5).Infof("volume group snapshot content %#v", groupSnapshotContent)
	// Try to create the VolumeGroupSnapshotContent object
	klog.V(5).Infof("createGroupSnapshotContent [%s]: trying to save volume group snapshot content %s", utils.GroupSnapshotKey(groupSnapshot), groupSnapshotContent.Name)
	if updateGroupSnapshotContent, err = ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshotContents().Create(context.TODO(), groupSnapshotContent, metav1.CreateOptions{}); err == nil || apierrs.IsAlreadyExists(err) {
		// Save succeeded.
		if err != nil {
			klog.V(3).Infof("volume group snapshot content %q for group snapshot %q already exists, reusing", groupSnapshotContent.Name, utils.GroupSnapshotKey(groupSnapshot))
			err = nil
			updateGroupSnapshotContent = groupSnapshotContent
		} else {
			klog.V(3).Infof("volume group snapshot content %q for group snapshot %q saved, %v", groupSnapshotContent.Name, utils.GroupSnapshotKey(groupSnapshot), groupSnapshotContent)
		}
	}

	if err != nil {
		strerr := fmt.Sprintf("Error creating volume group snapshot content object for group snapshot %s: %v.", utils.GroupSnapshotKey(groupSnapshot), err)
		klog.Error(strerr)
		ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeWarning, "CreateGroupSnapshotContentFailed", strerr)
		return nil, newControllerUpdateError(utils.GroupSnapshotKey(groupSnapshot), err.Error())
	}

	msg := fmt.Sprintf("Waiting for a group snapshot %s to be created by the CSI driver.", utils.GroupSnapshotKey(groupSnapshot))
	ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeNormal, "CreatingGroupSnapshot", msg)

	// Update group snapshot content in the cache store
	_, err = ctrl.storeGroupSnapshotContentUpdate(updateGroupSnapshotContent)
	if err != nil {
		klog.Errorf("failed to update group snapshot content store %v", err)
	}

	return updateGroupSnapshotContent, nil
}

func (ctrl *csiSnapshotCommonController) getCreateGroupSnapshotInput(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) (*crdv1beta2.VolumeGroupSnapshotClass, []*v1.PersistentVolume, string, *v1.SecretReference, error) {
	className := groupSnapshot.Spec.VolumeGroupSnapshotClassName
	klog.V(5).Infof("getCreateGroupSnapshotInput [%s]", groupSnapshot.Name)
	var groupSnapshotClass *crdv1beta2.VolumeGroupSnapshotClass
	var err error
	if className != nil {
		groupSnapshotClass, err = ctrl.getGroupSnapshotClass(*className)
		if err != nil {
			klog.Errorf("getCreateGroupSnapshotInput failed to getClassFromVolumeGroupSnapshot %s", err)
			return nil, nil, "", nil, err
		}
	} else {
		klog.Errorf("failed to getCreateGroupSnapshotInput %s without a group snapshot class", groupSnapshot.Name)
		return nil, nil, "", nil, fmt.Errorf("failed to take group snapshot %s without a group snapshot class", groupSnapshot.Name)
	}

	volumes, err := ctrl.getVolumesFromVolumeGroupSnapshot(groupSnapshot)
	if err != nil {
		klog.Errorf("getCreateGroupSnapshotInput failed to get PersistentVolume objects [%s]: Error: [%#v]", groupSnapshot.Name, err)
		return nil, nil, "", nil, err
	}

	// Create VolumeGroupSnapshotContent name
	contentName := utils.GetDynamicSnapshotContentNameForGroupSnapshot(groupSnapshot)

	// Get the secret reference
	snapshotterSecretRef, err := utils.GetGroupSnapshotSecretReference(utils.GroupSnapshotterSecretParams, groupSnapshotClass.Parameters, contentName, groupSnapshot)
	if err != nil {
		return nil, nil, "", nil, err
	}

	return groupSnapshotClass, volumes, contentName, snapshotterSecretRef, nil
}

// syncGroupSnapshotContent deals with one key off the queue
func (ctrl *csiSnapshotCommonController) syncGroupSnapshotContent(groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent) error {
	groupSnapshotName := utils.GroupSnapshotRefKey(&groupSnapshotContent.Spec.VolumeGroupSnapshotRef)
	klog.V(4).Infof("synchronizing VolumeGroupSnapshotContent[%s]: group snapshot content is bound to group snapshot %s", groupSnapshotContent.Name, groupSnapshotName)

	klog.V(5).Infof("syncGroupSnapshotContent[%s]: check if we should add invalid label on group snapshot content", groupSnapshotContent.Name)

	if (groupSnapshotContent.Spec.Source.GroupSnapshotHandles == nil && len(groupSnapshotContent.Spec.Source.VolumeHandles) == 0) ||
		(groupSnapshotContent.Spec.Source.GroupSnapshotHandles != nil && len(groupSnapshotContent.Spec.Source.VolumeHandles) > 0) {
		err := fmt.Errorf("Exactly one of GroupSnapshotHandles and VolumeHandles should be specified")
		klog.Errorf("syncGroupSnapshotContent[%s]: validation error, %s", groupSnapshotContent.Name, err.Error())
		ctrl.eventRecorder.Event(groupSnapshotContent, v1.EventTypeWarning, "GroupContentValidationError", err.Error())
		return err
	}

	// The VolumeGroupSnapshotContent is reserved for a VolumeGroupSnapshot;
	// that VolumeGroupSnapshot has not yet been bound to this VolumeGroupSnapshotContent;
	// syncGroupSnapshot will handle it.
	if groupSnapshotContent.Spec.VolumeGroupSnapshotRef.UID == "" {
		klog.V(4).Infof("syncGroupSnapshotContent [%s]: VolumeGroupSnapshotContent is pre-bound to VolumeGroupSnapshot %s", groupSnapshotContent.Name, groupSnapshotName)
		return nil
	}

	if utils.NeedToAddGroupSnapshotContentFinalizer(groupSnapshotContent) {
		// Group Snapshot Content is not being deleted -> it should have the finalizer.
		klog.V(5).Infof("syncGroupSnapshotContent [%s]: Add Finalizer for VolumeGroupSnapshotContent", groupSnapshotContent.Name)
		return ctrl.addGroupSnapshotContentFinalizer(groupSnapshotContent)
	}

	// Check if group snapshot exists in cache store
	// If getGroupSnapshotFromStore returns (nil, nil), it means group snapshot not found
	// and it may have already been deleted, and it will fall into the
	// group snapshot == nil case below
	var groupSnapshot *crdv1beta2.VolumeGroupSnapshot
	groupSnapshot, err := ctrl.getGroupSnapshotFromStore(groupSnapshotName)
	if err != nil {
		return err
	}

	if groupSnapshot != nil && groupSnapshot.UID != groupSnapshotContent.Spec.VolumeGroupSnapshotRef.UID {
		// The group snapshot that the group snapshot content was pointing to was deleted, and another
		// with the same name created.
		klog.V(4).Infof("syncGroupSnapshotContent [%s]: group snapshot %s has different UID, the old one must have been deleted", groupSnapshotContent.Name, groupSnapshotName)
		// Treat the group snapshot content as bound to a missing snapshot.
		groupSnapshot = nil
	} else {
		// Check if groupSnapshot.Status is different from groupSnapshotContent.Status
		// and add group snapshot to queue if there is a difference and it is worth
		// triggering a group snapshot status update.
		if groupSnapshot != nil && ctrl.needsUpdateGroupSnapshotStatus(groupSnapshot, groupSnapshotContent) {
			klog.V(4).Infof("synchronizing VolumeGroupSnapshotContent for group snapshot [%s]: update group snapshot status to true if needed.", groupSnapshotName)
			// Manually trigger a group snapshot status update to happen
			// right away so that it is in-sync with the group snapshot content status
			ctrl.groupSnapshotQueue.Add(groupSnapshotName)
		}
	}
	return nil
}

// getGroupSnapshotFromStore finds group snapshot from the cache store.
// If getGroupSnapshotFromStore returns (nil, nil), it means group snapshot not
// found and it may have already been deleted.
func (ctrl *csiSnapshotCommonController) getGroupSnapshotFromStore(groupSnapshotName string) (*crdv1beta2.VolumeGroupSnapshot, error) {
	// Get the VolumeGroupSnapshot by _name_
	var groupSnapshot *crdv1beta2.VolumeGroupSnapshot
	obj, found, err := ctrl.groupSnapshotStore.GetByKey(groupSnapshotName)
	if err != nil {
		return nil, err
	}
	if !found {
		klog.V(4).Infof("getGroupSnapshotFromStore: group snapshot %s not found", groupSnapshotName)
		// Fall through with group snapshot = nil
		return nil, nil
	}
	var ok bool
	groupSnapshot, ok = obj.(*crdv1beta2.VolumeGroupSnapshot)
	if !ok {
		return nil, fmt.Errorf("cannot convert object from group snapshot cache to group snapshot %q!?: %#v", groupSnapshotName, obj)
	}
	klog.V(4).Infof("getGroupSnapshotFromStore: group snapshot %s found", groupSnapshotName)

	return groupSnapshot, nil
}

// needsUpdateGroupSnapshotStatus compares group snapshot status with the group snapshot content
// status and decide if group snapshot status needs to be updated based on group snapshot content
// status
func (ctrl *csiSnapshotCommonController) needsUpdateGroupSnapshotStatus(groupSnapshot *crdv1beta2.VolumeGroupSnapshot, groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent) bool {
	klog.V(5).Infof("needsUpdateGroupSnapshotStatus[%s]", utils.GroupSnapshotKey(groupSnapshot))

	if groupSnapshot.Status == nil && groupSnapshotContent.Status != nil {
		return true
	}
	if groupSnapshotContent.Status == nil {
		return false
	}
	if groupSnapshot.Status.BoundVolumeGroupSnapshotContentName == nil {
		return true
	}
	if groupSnapshot.Status.CreationTime == nil && groupSnapshotContent.Status.CreationTime != nil {
		return true
	}
	if groupSnapshot.Status.ReadyToUse == nil && groupSnapshotContent.Status.ReadyToUse != nil {
		return true
	}
	if groupSnapshot.Status.ReadyToUse != nil && groupSnapshotContent.Status.ReadyToUse != nil && groupSnapshot.Status.ReadyToUse != groupSnapshotContent.Status.ReadyToUse {
		return true
	}

	return false
}

// addGroupSnapshotContentFinalizer adds a Finalizer for VolumeGroupSnapshotContent.
func (ctrl *csiSnapshotCommonController) addGroupSnapshotContentFinalizer(groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent) error {
	var patches []utils.PatchOp
	if len(groupSnapshotContent.Finalizers) > 0 {
		// Add to the end of the finalizers if we have any other finalizers
		patches = append(patches, utils.PatchOp{
			Op:    "add",
			Path:  "/metadata/finalizers/-",
			Value: utils.VolumeGroupSnapshotContentFinalizer,
		})
	} else {
		// Replace finalizers with new array if there are no other finalizers
		patches = append(patches, utils.PatchOp{
			Op:    "add",
			Path:  "/metadata/finalizers",
			Value: []string{utils.VolumeGroupSnapshotContentFinalizer},
		})
	}
	newGroupSnapshotContent, err := utils.PatchVolumeGroupSnapshotContent(groupSnapshotContent, patches, ctrl.clientset)
	if err != nil {
		return newControllerUpdateError(groupSnapshotContent.Name, err.Error())
	}

	_, err = ctrl.storeGroupSnapshotContentUpdate(newGroupSnapshotContent)
	if err != nil {
		klog.Errorf("failed to update group snapshot content store %v", err)
	}

	klog.V(5).Infof("Added protection finalizer to volume group snapshot content %s", newGroupSnapshotContent.Name)
	return nil
}

// checkandAddGroupSnapshotFinalizers checks and adds group snapshot finailzers when needed
func (ctrl *csiSnapshotCommonController) checkandAddGroupSnapshotFinalizers(groupSnapshot *crdv1beta2.VolumeGroupSnapshot) error {
	// get the group snapshot content for this group snapshot
	var (
		groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent
		err                  error
	)
	if groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName != nil {
		groupSnapshotContent, err = ctrl.getPreprovisionedGroupSnapshotContentFromStore(groupSnapshot)
	} else {
		groupSnapshotContent, err = ctrl.getDynamicallyProvisionedGroupContentFromStore(groupSnapshot)
	}
	if err != nil {
		return err
	}

	// A bound finalizer is needed ONLY when all following conditions are satisfied:
	// 1. the VolumeGroupSnapshot is bound to a VolumeGroupSnapshotContent
	// 2. the VolumeGroupSnapshot does not have deletion timestamp set
	// Note that if a matching VolumeGroupSnapshotContent is found, it must point back to the VolumeGroupSnapshot
	if groupSnapshotContent != nil && utils.NeedToAddGroupSnapshotBoundFinalizer(groupSnapshot) {
		// Snapshot is not being deleted -> it should have the finalizer.
		klog.V(5).Infof("checkandAddGroupSnapshotFinalizers: Add Finalizer for VolumeGroupSnapshot[%s]", utils.GroupSnapshotKey(groupSnapshot))
		return ctrl.addGroupSnapshotFinalizer(groupSnapshot, true)

	}
	return nil
}

// addGroupSnapshotFinalizer adds a Finalizer to a VolumeGroupSnapshot.
func (ctrl *csiSnapshotCommonController) addGroupSnapshotFinalizer(groupSnapshot *crdv1beta2.VolumeGroupSnapshot, addBoundFinalizer bool) error {
	var updatedGroupSnapshot *crdv1beta2.VolumeGroupSnapshot
	var err error

	// NOTE(ggriffiths): Must perform an update if no finalizers exist.
	// Unable to find a patch that correctly updated the finalizers if none currently exist.
	if len(groupSnapshot.ObjectMeta.Finalizers) == 0 {
		groupSnapshotClone := groupSnapshot.DeepCopy()
		if addBoundFinalizer {
			groupSnapshotClone.ObjectMeta.Finalizers = append(groupSnapshotClone.ObjectMeta.Finalizers, utils.VolumeGroupSnapshotBoundFinalizer)
		}
		updatedGroupSnapshot, err = ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshots(groupSnapshotClone.Namespace).Update(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})
		if err != nil {
			return newControllerUpdateError(utils.GroupSnapshotKey(groupSnapshot), err.Error())
		}
	} else {
		// Otherwise, perform a patch
		var patches []utils.PatchOp

		if addBoundFinalizer {
			patches = append(patches, utils.PatchOp{
				Op:    "add",
				Path:  "/metadata/finalizers/-",
				Value: utils.VolumeGroupSnapshotBoundFinalizer,
			})
		}

		updatedGroupSnapshot, err = utils.PatchVolumeGroupSnapshot(groupSnapshot, patches, ctrl.clientset)
		if err != nil {
			return newControllerUpdateError(utils.GroupSnapshotKey(groupSnapshot), err.Error())
		}
	}

	_, err = ctrl.storeGroupSnapshotUpdate(updatedGroupSnapshot)
	if err != nil {
		klog.Errorf("failed to update group snapshot store %v", err)
	}

	klog.V(5).Infof("Added protection finalizer to volume group snapshot %s", utils.GroupSnapshotKey(updatedGroupSnapshot))
	return nil
}

// processGroupSnapshotWithDeletionTimestamp processes finalizers and deletes the
// group snapshot content when appropriate. It has the following steps:
// 1. Get the VolumeGroupSnapshotContent which the to-be-deleted VolumeGroupSnapshot
// points to and verifies bi-directional binding.
// 2. Call checkandRemoveGroupSnapshotFinalizersAndCheckandDeleteGroupSnapshotContent()
// with information obtained from step 1. This function name is very long but the
// name suggests what it does. It determines whether to remove finalizers on group
// snapshot and whether to delete group snapshot content.
func (ctrl *csiSnapshotCommonController) processGroupSnapshotWithDeletionTimestamp(ctx context.Context, groupSnapshot *crdv1beta2.VolumeGroupSnapshot) error {
	klog.V(5).Infof("processGroupSnapshotWithDeletionTimestamp VolumeGroupSnapshot[%s]: %s", utils.GroupSnapshotKey(groupSnapshot), utils.GetGroupSnapshotStatusForLogging(groupSnapshot))

	driverName, err := ctrl.getGroupSnapshotDriverName(groupSnapshot)
	if err != nil {
		klog.Errorf("failed to getGroupSnapshotDriverName while recording metrics for group snapshot %q: %v", utils.GroupSnapshotKey(groupSnapshot), err)
	}

	groupSnapshotProvisionType := metrics.DynamicGroupSnapshotType
	if groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName != nil {
		groupSnapshotProvisionType = metrics.PreProvisionedGroupSnapshotType
	}

	// Processing delete, start operation metric
	deleteOperationKey := metrics.NewOperationKey(metrics.DeleteGroupSnapshotOperationName, groupSnapshot.UID)
	deleteOperationValue := metrics.NewOperationValue(driverName, groupSnapshotProvisionType)
	ctrl.metricsManager.OperationStart(deleteOperationKey, deleteOperationValue)

	var groupSnapshotContentName string
	if groupSnapshot.Status != nil && groupSnapshot.Status.BoundVolumeGroupSnapshotContentName != nil {
		groupSnapshotContentName = *groupSnapshot.Status.BoundVolumeGroupSnapshotContentName
	}
	// for a dynamically created group snapshot, it's possible that a group snapshot
	// content has been created however the Status of the group snapshot has not
	// been updated yet, i.e., failed right after group snapshot content creation.
	// In this case, use the fixed naming scheme to get the group snapshot content
	// name and search
	if groupSnapshotContentName == "" && &groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName == nil {
		groupSnapshotContentName = utils.GetDynamicSnapshotContentNameForGroupSnapshot(groupSnapshot)
	}
	// find a group snapshot content from cache store, note that it's completely legit
	// that no group snapshot content has been found from group snapshot content
	// cache store
	groupSnapshotContent, err := ctrl.getGroupSnapshotContentFromStore(groupSnapshotContentName)
	if err != nil {
		return err
	}
	// check whether the group snapshot content points back to the passed in group
	// snapshot, note that binding should always be bi-directional to trigger the
	// deletion on group snapshot content or adding any annotation to the group
	// snapshot content
	var deleteGroupSnapshotContent bool
	if groupSnapshotContent != nil && utils.IsVolumeGroupSnapshotRefSet(groupSnapshot, groupSnapshotContent) {
		// group snapshot content points back to group snapshot, whether or not
		// to delete a group snapshot content now depends on the deletion policy
		// of it.
		deleteGroupSnapshotContent = (groupSnapshotContent.Spec.DeletionPolicy == crdv1.VolumeSnapshotContentDelete)
	} else {
		// the group snapshot content is nil or points to a different group snapshot, reset group snapshot content to nil
		// such that there is no operation done on the found group snapshot content in
		// checkandRemoveSnapshotFinalizersAndCheckandDeleteContent
		groupSnapshotContent = nil
	}

	klog.V(5).Infof("processGroupSnapshotWithDeletionTimestamp[%s]: check if group snapshot is a candidate for deletion", utils.GroupSnapshotKey(groupSnapshot))
	if !utils.IsGroupSnapshotDeletionCandidate(groupSnapshot) {
		return nil
	}

	// Look up for members of this volume group snapshot
	snapshotMembers, err := ctrl.findGroupSnapshotMembers(
		types.NamespacedName{
			Name:      groupSnapshot.Name,
			Namespace: groupSnapshot.Namespace,
		},
	)
	if err != nil {
		klog.Errorf(
			"processGroupSnapshotWithDeletionTimestamp[%s]: Failed to look for snapshot members: %v",
			utils.GroupSnapshotKey(groupSnapshot),
			err.Error())
		return err
	}

	// check if an individual snapshot belonging to the group snapshot is being
	// used for restore a PVC
	// If yes, do nothing and wait until PVC restoration finishes
	for _, snapshot := range snapshotMembers {
		snapshot, err := ctrl.snapshotLister.VolumeSnapshots(groupSnapshot.Namespace).Get(snapshot.Name)
		if err != nil {
			if apierrs.IsNotFound(err) {
				continue
			}
			return err
		}
		if ctrl.isVolumeBeingCreatedFromSnapshot(snapshot) {
			msg := fmt.Sprintf("Snapshot %s belonging to VolumeGroupSnapshot %s is being used to restore a PVC", utils.SnapshotKey(snapshot), utils.GroupSnapshotKey(groupSnapshot))
			klog.V(4).Info(msg)
			ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeWarning, "SnapshotDeletePending", msg)
			// TODO(@xiangqian): should requeue this?
			return nil
		}

	}

	// regardless of the deletion policy, set VolumeGroupSnapshotBeingDeleted on
	// group snapshot content object, this is to allow snapshotter sidecar controller
	// to conduct a delete operation whenever the group snapshot content has deletion
	// timestamp set.
	if groupSnapshotContent != nil {
		klog.V(5).Infof("processGroupSnapshotWithDeletionTimestamp[%s]: Set VolumeGroupSnapshotBeingDeleted annotation on the group snapshot content [%s]", utils.GroupSnapshotKey(groupSnapshot), groupSnapshotContent.Name)
		updatedGroupSnapshotContent, err := ctrl.setAnnVolumeGroupSnapshotBeingDeleted(groupSnapshotContent)
		if err != nil {
			klog.V(4).Infof("processGroupSnapshotWithDeletionTimestamp[%s]: failed to set VolumeGroupSnapshotBeingDeleted annotation on the group snapshot content [%s]: %v", utils.GroupSnapshotKey(groupSnapshot), groupSnapshotContent.Name, err)
			return err
		}
		groupSnapshotContent = updatedGroupSnapshotContent
	}

	// VolumeGroupSnapshot should be deleted. Check and remove finalizers
	// If group snapshot content exists and has a deletion policy of Delete, set
	// DeletionTimeStamp on the group snapshot content;
	// VolumeGroupSnapshotContent won't be deleted immediately due to the VolumeGroupSnapshotContentFinalizer
	if groupSnapshotContent != nil && deleteGroupSnapshotContent {
		klog.V(5).Infof("processGroupSnapshotWithDeletionTimestamp[%s]: set DeletionTimeStamp on group snapshot content [%s].", utils.GroupSnapshotKey(groupSnapshot), groupSnapshotContent.Name)
		err := ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshotContents().Delete(ctx, groupSnapshotContent.Name, metav1.DeleteOptions{})
		if err != nil {
			ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeWarning, "GroupSnapshotContentObjectDeleteError", "Failed to delete group snapshot content API object")
			return fmt.Errorf("failed to delete VolumeGroupSnapshotContent %s from API server: %q", groupSnapshotContent.Name, err)
		}
	}

	klog.V(5).Infof("processGroupSnapshotWithDeletionTimestamp[%s]: Delete individual snapshots that are part of the group snapshot", utils.GroupSnapshotKey(groupSnapshot))

	// Delete the individual snapshots part of the group snapshot
	for _, snapshot := range snapshotMembers {
		err := ctrl.clientset.SnapshotV1().
			VolumeSnapshots(groupSnapshot.Namespace).
			Delete(context.TODO(), snapshot.Name, metav1.DeleteOptions{})
		if err != nil && !apierrs.IsNotFound(err) {
			msg := fmt.Sprintf(
				"failed to delete snapshot API object %s/%s part of group snapshot %s: %v",
				groupSnapshot.Namespace,
				snapshot.Name,
				utils.GroupSnapshotKey(groupSnapshot), err)
			klog.Error(msg)
			ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeWarning, "SnapshotDeleteError", msg)
			return errors.New(msg)
		}
	}

	klog.V(5).Infof("processGroupSnapshotWithDeletionTimestamp[%s] : Remove Finalizer for VolumeGroupSnapshot", utils.GroupSnapshotKey(groupSnapshot))
	// remove VolumeSnapshotBoundFinalizer on the VolumeGroupSnapshot object:
	//    a. If there is no group snapshot content found, remove the finalizer.
	//    b. If the group snapshot content is being deleted, i.e., with deleteGroupSnapshotContent == true,
	//       keep this finalizer until the group snapshot content object is removed
	//       from API server by group snapshot sidecar controller.
	//    c. If deletion will not cascade to the group snapshot content, remove
	//       the finalizer on the group snapshot such that it can be removed from
	//       the API server.
	removeBoundFinalizer := !(groupSnapshotContent != nil && deleteGroupSnapshotContent)
	return ctrl.removeGroupSnapshotFinalizer(groupSnapshot, removeBoundFinalizer)
}

func (ctrl *csiSnapshotCommonController) setAnnVolumeGroupSnapshotBeingDeleted(groupSnapshotContent *crdv1beta2.VolumeGroupSnapshotContent) (*crdv1beta2.VolumeGroupSnapshotContent, error) {
	if groupSnapshotContent == nil {
		return groupSnapshotContent, nil
	}
	// Set AnnVolumeGroupSnapshotBeingDeleted if it is not set yet
	if !metav1.HasAnnotation(groupSnapshotContent.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingDeleted) {
		klog.V(5).Infof("setAnnVolumeGroupSnapshotBeingDeleted: set annotation [%s] on group snapshot content [%s].", utils.AnnVolumeGroupSnapshotBeingDeleted, groupSnapshotContent.Name)
		var patches []utils.PatchOp
		metav1.SetMetaDataAnnotation(&groupSnapshotContent.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingDeleted, "yes")
		patches = append(patches, utils.PatchOp{
			Op:    "replace",
			Path:  "/metadata/annotations",
			Value: groupSnapshotContent.ObjectMeta.GetAnnotations(),
		})

		patchedGroupSnapshotContent, err := utils.PatchVolumeGroupSnapshotContent(groupSnapshotContent, patches, ctrl.clientset)
		if err != nil {
			return groupSnapshotContent, newControllerUpdateError(groupSnapshotContent.Name, err.Error())
		}

		// update group snapshot content if update is successful
		groupSnapshotContent = patchedGroupSnapshotContent

		_, err = ctrl.storeGroupSnapshotContentUpdate(groupSnapshotContent)
		if err != nil {
			klog.V(4).Infof("setAnnVolumeGroupSnapshotBeingDeleted for group snapshot content [%s]: cannot update internal cache %v", groupSnapshotContent.Name, err)
			return groupSnapshotContent, err
		}
		klog.V(5).Infof("setAnnVolumeGroupSnapshotBeingDeleted: volume group snapshot content %+v", groupSnapshotContent)
	}
	return groupSnapshotContent, nil
}

// findGroupSnapshotMembers get the list of members of a group snapshot
// using the local cache and indexer
func (ctrl *csiSnapshotCommonController) findGroupSnapshotMembers(groupSnapshotName types.NamespacedName) ([]*crdv1.VolumeSnapshot, error) {
	// Look up for the members of this volume group snapshot
	snapshotMembers, err := ctrl.snapshotIndexer.ByIndex(
		utils.VolumeSnapshotParentGroupIndex,
		utils.VolumeSnapshotParentGroupKeyFuncByComponents(
			groupSnapshotName,
		),
	)
	if err != nil {
		return nil, err
	}

	result := make([]*crdv1.VolumeSnapshot, len(snapshotMembers))
	for i := range snapshotMembers {
		var ok bool
		result[i], ok = snapshotMembers[i].(*crdv1.VolumeSnapshot)
		if !ok {
			return nil, fmt.Errorf("unexpected content found in snapshot index: %v", snapshotMembers[i])
		}
	}

	return result, nil
}

// removeGroupSnapshotFinalizer removes a Finalizer for VolumeGroupSnapshot.
func (ctrl *csiSnapshotCommonController) removeGroupSnapshotFinalizer(groupSnapshot *crdv1beta2.VolumeGroupSnapshot, removeBoundFinalizer bool) error {
	if !removeBoundFinalizer {
		return nil
	}

	// TODO: Remove PVC Finalizer

	groupSnapshotClone := groupSnapshot.DeepCopy()
	groupSnapshotClone.ObjectMeta.Finalizers = utils.RemoveString(groupSnapshotClone.ObjectMeta.Finalizers, utils.VolumeGroupSnapshotBoundFinalizer)
	newGroupSnapshot, err := ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshots(groupSnapshotClone.Namespace).Update(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})
	if err != nil {
		return newControllerUpdateError(groupSnapshot.Name, err.Error())
	}

	_, err = ctrl.storeGroupSnapshotUpdate(newGroupSnapshot)
	if err != nil {
		klog.Errorf("failed to update group snapshot store %v", err)
	}

	klog.V(5).Infof("Removed protection finalizer from volume group snapshot %s", utils.GroupSnapshotKey(groupSnapshot))
	return nil
}

// getGroupSnapshotDriverName is a helper function to get driver from the VolumeGroupSnapshot.
// We try to get the driverName in multiple ways, as snapshot controller metrics depend on the correct driverName.
func (ctrl *csiSnapshotCommonController) getGroupSnapshotDriverName(vgs *crdv1beta2.VolumeGroupSnapshot) (string, error) {
	klog.V(5).Infof("getGroupSnapshotDriverName: VolumeGroupSnapshot[%s]", vgs.Name)
	var driverName string

	// Pre-Provisioned groupsnapshots have contentName as source
	var contentName string
	if vgs.Spec.Source.VolumeGroupSnapshotContentName != nil {
		contentName = *vgs.Spec.Source.VolumeGroupSnapshotContentName
	}

	// Get Driver name from GroupSnapshotContent if we found a contentName
	if contentName != "" {
		content, err := ctrl.groupSnapshotContentLister.Get(contentName)
		if err != nil {
			klog.Errorf("getGroupSnapshotDriverName: failed to get groupSnapshotContent: %v", contentName)
		} else {
			driverName = content.Spec.Driver
		}

		if driverName != "" {
			return driverName, nil
		}
	}

	// Dynamic groupsnapshots will have a groupsnapshotclass with a driver
	if vgs.Spec.VolumeGroupSnapshotClassName != nil {
		class, err := ctrl.getGroupSnapshotClass(*vgs.Spec.VolumeGroupSnapshotClassName)
		if err != nil {
			klog.Errorf("getGroupSnapshotDriverName: failed to get groupSnapshotClass: %v", *vgs.Spec.VolumeGroupSnapshotClassName)
		} else {
			driverName = class.Driver
		}
	}

	return driverName, nil
}
