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
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	ref "k8s.io/client-go/tools/reference"
	klog "k8s.io/klog/v2"

	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/utils"
)

func (ctrl *csiSnapshotCommonController) storeGroupSnapshotUpdate(groupsnapshot interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.groupSnapshotStore, groupsnapshot, "groupsnapshot")
}

func (ctrl *csiSnapshotCommonController) storeGroupSnapshotContentUpdate(groupsnapshotcontent interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.groupSnapshotContentStore, groupsnapshotcontent, "groupsnapshotcontent")
}

// getGroupSnapshotClass is a helper function to get group snapshot class from the group snapshot class name.
func (ctrl *csiSnapshotCommonController) getGroupSnapshotClass(className string) (*crdv1alpha1.VolumeGroupSnapshotClass, error) {
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
func (ctrl *csiSnapshotCommonController) updateGroupSnapshotErrorStatusWithEvent(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot, setReadyToFalse bool, eventtype, reason, message string) error {
	klog.V(5).Infof("updateGroupSnapshotErrorStatusWithEvent[%s]", utils.GroupSnapshotKey(groupSnapshot))

	if groupSnapshot.Status != nil && groupSnapshot.Status.Error != nil && *groupSnapshot.Status.Error.Message == message {
		klog.V(4).Infof("updateGroupSnapshotErrorStatusWithEvent[%s]: the same error %v is already set", groupSnapshot.Name, groupSnapshot.Status.Error)
		return nil
	}
	groupSnapshotClone := groupSnapshot.DeepCopy()
	if groupSnapshotClone.Status == nil {
		groupSnapshotClone.Status = &crdv1alpha1.VolumeGroupSnapshotStatus{}
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
	newSnapshot, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshots(groupSnapshotClone.Namespace).UpdateStatus(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})

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
func (ctrl *csiSnapshotCommonController) SetDefaultGroupSnapshotClass(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (*crdv1alpha1.VolumeGroupSnapshotClass, *crdv1alpha1.VolumeGroupSnapshot, error) {
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

	defaultClasses := []*crdv1alpha1.VolumeGroupSnapshotClass{}
	for _, groupSnapshotClass := range list {
		if utils.IsDefaultAnnotation(groupSnapshotClass.ObjectMeta) && pvDriver == groupSnapshotClass.Driver {
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
	newGroupSnapshot, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshots(groupSnapshotClone.Namespace).Update(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})
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
func (ctrl *csiSnapshotCommonController) pvDriverFromGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (string, error) {
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
func (ctrl *csiSnapshotCommonController) getVolumesFromVolumeGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) ([]*v1.PersistentVolume, error) {
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
func (ctrl *csiSnapshotCommonController) getClaimsFromVolumeGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) ([]v1.PersistentVolumeClaim, error) {
	labelSelector := groupSnapshot.Spec.Source.Selector

	// Get PVC that has group snapshot label applied.
	pvcList, err := ctrl.client.CoreV1().PersistentVolumeClaims(groupSnapshot.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labels.Set(labelSelector.MatchLabels).String()})
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs with label selector %s: %q", labelSelector.String(), err)
	}
	if len(pvcList.Items) == 0 {
		return nil, fmt.Errorf("label selector %s for group snapshot not applied to any PVC", labelSelector.String())
	}
	return pvcList.Items, nil
}

// updateGroupSnapshot runs in worker thread and handles "groupsnapshot added",
// "groupsnapshot updated" and "periodic sync" events.
func (ctrl *csiSnapshotCommonController) updateGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) error {
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

	err = ctrl.syncGroupSnapshot(groupSnapshot)
	if err != nil {
		if errors.IsConflict(err) {
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
func (ctrl *csiSnapshotCommonController) deleteGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) {
	_ = ctrl.snapshotStore.Delete(groupSnapshot)
	klog.V(4).Infof("group snapshot %q deleted", utils.GroupSnapshotKey(groupSnapshot))

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
func (ctrl *csiSnapshotCommonController) syncGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) error {
	klog.V(5).Infof("synchronizing VolumeGroupSnapshot[%s]", utils.GroupSnapshotKey(groupSnapshot))

	klog.V(5).Infof("syncGroupSnapshot [%s]: check if we should remove finalizer on group snapshot PVC source and remove it if we can", utils.GroupSnapshotKey(groupSnapshot))

	/*
		TODO:
		- Check and remove finalizer if needed.
		- Check and set invalid group snapshot label, if needed.
		- Process if deletion timestamp is set.
		- Check and add group snapshot finalizers.
	*/

	// Need to build or update groupSnapshot.Status in following cases:
	// 1) groupSnapshot.Status is nil
	// 2) groupSnapshot.Status.ReadyToUse is false
	// 3) groupSnapshot.Status.IsBoundVolumeGroupSnapshotContentNameSet is not set
	// 4) groupSnapshot.Status.IsVolumeSnapshotRefListSet is not set
	if !utils.IsGroupSnapshotReady(groupSnapshot) || !utils.IsBoundVolumeGroupSnapshotContentNameSet(groupSnapshot) || !utils.IsVolumeSnapshotRefListSet(groupSnapshot) {
		return ctrl.syncUnreadyGroupSnapshot(groupSnapshot)
	}
	return ctrl.syncReadyGroupSnapshot(groupSnapshot)
}

// syncReadyGroupSnapshot checks the group snapshot which has been bound to group
// snapshot content successfully before.
// If there is any problem with the binding (e.g., group snapshot points to a
// non-existent group snapshot content), update the group snapshot status and emit event.
func (ctrl *csiSnapshotCommonController) syncReadyGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) error {
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
func (ctrl *csiSnapshotCommonController) getGroupSnapshotContentFromStore(contentName string) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
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
	groupSnapshotContent, ok := obj.(*crdv1alpha1.VolumeGroupSnapshotContent)
	if !ok {
		return nil, fmt.Errorf("expected VolumeGroupSnapshotContent, got %+v", obj)
	}
	return groupSnapshotContent, nil
}

// syncUnreadyGroupSnapshot is the main controller method to decide what to do
// with a group snapshot which is not set to ready.
func (ctrl *csiSnapshotCommonController) syncUnreadyGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) error {
	uniqueGroupSnapshotName := utils.GroupSnapshotKey(groupSnapshot)
	klog.V(5).Infof("syncUnreadyGroupSnapshot %s", uniqueGroupSnapshotName)
	/*
		TODO: Add metrics
	*/

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
		if contentObj.Spec.Source.VolumeGroupSnapshotHandle != nil {
			ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotHandleSet", fmt.Sprintf("GroupSnapshot handle should not be set in group snapshot content %s for dynamic provisioning", uniqueGroupSnapshotName))
			return fmt.Errorf("VolumeGroupSnapshotHandle should not be set in the group snapshot content for dynamic provisioning for group snapshot %s", uniqueGroupSnapshotName)
		}

		newGroupSnapshot, err := ctrl.bindandUpdateVolumeGroupSnapshot(contentObj, groupSnapshot)
		if err != nil {
			klog.V(4).Infof("bindandUpdateVolumeGroupSnapshot[%s]: failed to bind group snapshot content [%s] to group snapshot %v", uniqueGroupSnapshotName, contentObj.Name, err)
			return err
		}
		klog.V(5).Infof("bindandUpdateVolumeGroupSnapshot %v", newGroupSnapshot)
		return nil
	}

	// If we reach here, it is a dynamically provisioned group snapshot, and the VolumeGroupSnapshotContent object is not yet created.
	var groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent
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
func (ctrl *csiSnapshotCommonController) getPreprovisionedGroupSnapshotContentFromStore(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
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
	if groupSnapshotContent.Spec.Source.VolumeGroupSnapshotHandle == nil {
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
		return nil, fmt.Errorf(msg)
	}
	return groupSnapshotContent, nil
}

// checkandBindGroupSnapshotContent checks whether the VolumeGroupSnapshotRef in
// the group snapshot content matches the given group snapshot. If match, it binds
// the group snapshot content with the group snapshot. This is for static binding where
// user has specified group snapshot name but not UID of the group snapshot in
// groupSnapshotContent.Spec.VolumeGroupSnapshotRef.
func (ctrl *csiSnapshotCommonController) checkAndBindGroupSnapshotContent(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot, groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
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
func (ctrl *csiSnapshotCommonController) updateGroupSnapshotStatus(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot, groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshot, error) {
	klog.V(5).Infof("updateGroupSnapshotStatus[%s]", utils.GroupSnapshotKey(groupSnapshot))

	boundContentName := groupSnapshotContent.Name
	var createdAt *time.Time
	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.CreationTime != nil {
		unixTime := time.Unix(0, *groupSnapshotContent.Status.CreationTime)
		createdAt = &unixTime
	}
	var readyToUse bool
	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.ReadyToUse != nil {
		readyToUse = *groupSnapshotContent.Status.ReadyToUse
	}
	var volumeSnapshotErr *crdv1.VolumeSnapshotError
	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.Error != nil {
		volumeSnapshotErr = groupSnapshotContent.Status.Error.DeepCopy()
	}

	var volumeSnapshotRefList []v1.ObjectReference
	if groupSnapshotContent.Status != nil && len(groupSnapshotContent.Status.VolumeSnapshotContentRefList) != 0 {
		for _, contentRef := range groupSnapshotContent.Status.VolumeSnapshotContentRefList {
			groupSnapshotContent, err := ctrl.contentLister.Get(contentRef.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to get group snapshot content %s from group snapshot content store: %v", contentRef.Name, err)
			}
			volumeSnapshotRefList = append(volumeSnapshotRefList, groupSnapshotContent.Spec.VolumeSnapshotRef)
		}
	}

	klog.V(5).Infof("updateGroupSnapshotStatus: updating VolumeGroupSnapshot [%+v] based on VolumeGroupSnapshotContentStatus [%+v]", groupSnapshot, groupSnapshotContent.Status)

	groupSnapshotObj, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshots(groupSnapshot.Namespace).Get(context.TODO(), groupSnapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get group snapshot %s from api server: %v", utils.GroupSnapshotKey(groupSnapshot), err)
	}

	var newStatus *crdv1alpha1.VolumeGroupSnapshotStatus
	updated := false
	if groupSnapshotObj.Status == nil {
		newStatus = &crdv1alpha1.VolumeGroupSnapshotStatus{
			BoundVolumeGroupSnapshotContentName: &boundContentName,
			ReadyToUse:                          &readyToUse,
		}
		if createdAt != nil {
			newStatus.CreationTime = &metav1.Time{Time: *createdAt}
		}
		if volumeSnapshotErr != nil {
			newStatus.Error = volumeSnapshotErr
		}
		if len(volumeSnapshotRefList) == 0 {
			newStatus.VolumeSnapshotRefList = volumeSnapshotRefList
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
		if len(newStatus.VolumeSnapshotRefList) == 0 {
			newStatus.VolumeSnapshotRefList = volumeSnapshotRefList
			updated = true
		}
	}

	if updated {
		groupSnapshotClone := groupSnapshotObj.DeepCopy()
		groupSnapshotClone.Status = newStatus

		// Must meet the following criteria to emit a successful CreateGroupSnapshot status
		// 1. Previous status was nil OR Previous status had a nil CreationTime
		// 2. New status must be non-nil with a non-nil CreationTime
		if !utils.IsGroupSnapshotCreated(groupSnapshotObj) && utils.IsGroupSnapshotCreated(groupSnapshotClone) {
			msg := fmt.Sprintf("GroupSnapshot %s was successfully created by the CSI driver.", utils.GroupSnapshotKey(groupSnapshot))
			ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeNormal, "GroupSnapshotCreated", msg)
		}

		// Must meet the following criteria to emit a successful CreateGroupSnapshotAndReady status
		// 1. Previous status was nil OR Previous status had a nil ReadyToUse OR Previous status had a false ReadyToUse
		// 2. New status must be non-nil with a ReadyToUse as true
		if !utils.IsGroupSnapshotReady(groupSnapshotObj) && utils.IsGroupSnapshotReady(groupSnapshotClone) {
			msg := fmt.Sprintf("GroupSnapshot %s is ready to use.", utils.GroupSnapshotKey(groupSnapshot))
			ctrl.eventRecorder.Event(groupSnapshot, v1.EventTypeNormal, "GroupSnapshotReady", msg)
		}

		newGroupSnapshotObj, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshots(groupSnapshotClone.Namespace).UpdateStatus(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})
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
func (ctrl *csiSnapshotCommonController) getDynamicallyProvisionedGroupContentFromStore(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
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
	if groupSnapshotContent.Spec.Source.VolumeGroupSnapshotHandle != nil {
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
		return nil, fmt.Errorf(msg)
	}
	return groupSnapshotContent, nil
}

// This routine sets snapshot.Spec.Source.VolumeGroupSnapshotContentName
func (ctrl *csiSnapshotCommonController) bindandUpdateVolumeGroupSnapshot(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent, groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (*crdv1alpha1.VolumeGroupSnapshot, error) {
	klog.V(5).Infof("bindandUpdateVolumeGroupSnapshot for group snapshot [%s]: groupSnapshotContent [%s]", groupSnapshot.Name, groupSnapshotContent.Name)
	groupSnapshotObj, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshots(groupSnapshot.Namespace).Get(context.TODO(), groupSnapshot.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get group snapshot %s from api server: %v", utils.GroupSnapshotKey(groupSnapshot), err)
	}

	// Copy the group snapshot object before updating it
	groupSnapshotCopy := groupSnapshotObj.DeepCopy()
	// update group snapshot status
	var updateGroupSnapshot *crdv1alpha1.VolumeGroupSnapshot
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
func (ctrl *csiSnapshotCommonController) createGroupSnapshotContent(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	klog.Infof("createSnapshotContent: Creating group snapshot content for groupn snapshot %s through the plugin ...", utils.GroupSnapshotKey(groupSnapshot))

	/*
		TODO: Add finalizer to group snapshot
	*/

	groupSnapshotClass, volumes, contentName, err := ctrl.getCreateGroupSnapshotInput(groupSnapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to get input parameters to create group snapshot %s: %q", groupSnapshot.Name, err)
	}

	snapshotRef, err := ref.GetReference(scheme.Scheme, groupSnapshot)
	if err != nil {
		return nil, err
	}
	var pvNames []string
	for _, pv := range volumes {
		pvNames = append(pvNames, pv.Name)
	}

	groupSnapshotContent := &crdv1alpha1.VolumeGroupSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentName,
		},
		Spec: crdv1alpha1.VolumeGroupSnapshotContentSpec{
			VolumeGroupSnapshotRef: *snapshotRef,
			Source: crdv1alpha1.VolumeGroupSnapshotContentSource{
				PersistentVolumeNames: pvNames,
			},
			VolumeGroupSnapshotClassName: &(groupSnapshotClass.Name),
			DeletionPolicy:               groupSnapshotClass.DeletionPolicy,
			Driver:                       groupSnapshotClass.Driver,
		},
	}

	/*
		TODO: Add secret reference details
	*/

	var updateGroupSnapshoyContent *crdv1alpha1.VolumeGroupSnapshotContent
	klog.V(5).Infof("volume group snapshot content %#v", groupSnapshotContent)
	// Try to create the VolumeGroupSnapshotContent object
	klog.V(5).Infof("createGroupSnapshotContent [%s]: trying to save volume group snapshot content %s", utils.GroupSnapshotKey(groupSnapshot), groupSnapshotContent.Name)
	if updateGroupSnapshoyContent, err = ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().Create(context.TODO(), groupSnapshotContent, metav1.CreateOptions{}); err == nil || apierrs.IsAlreadyExists(err) {
		// Save succeeded.
		if err != nil {
			klog.V(3).Infof("volume group snapshot content %q for group snapshot %q already exists, reusing", groupSnapshotContent.Name, utils.GroupSnapshotKey(groupSnapshot))
			err = nil
			updateGroupSnapshoyContent = groupSnapshotContent
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
	_, err = ctrl.storeGroupSnapshotContentUpdate(updateGroupSnapshoyContent)
	if err != nil {
		klog.Errorf("failed to update group snapshot content store %v", err)
	}

	return updateGroupSnapshoyContent, nil
}

func (ctrl *csiSnapshotCommonController) getCreateGroupSnapshotInput(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (*crdv1alpha1.VolumeGroupSnapshotClass, []*v1.PersistentVolume, string, error) {
	className := groupSnapshot.Spec.VolumeGroupSnapshotClassName
	klog.V(5).Infof("getCreateGroupSnapshotInput [%s]", groupSnapshot.Name)
	var groupSnapshotClass *crdv1alpha1.VolumeGroupSnapshotClass
	var err error
	if className != nil {
		groupSnapshotClass, err = ctrl.getGroupSnapshotClass(*className)
		if err != nil {
			klog.Errorf("getCreateGroupSnapshotInput failed to getClassFromVolumeGroupSnapshot %s", err)
			return nil, nil, "", err
		}
	} else {
		klog.Errorf("failed to getCreateGroupSnapshotInput %s without a group snapshot class", groupSnapshot.Name)
		return nil, nil, "", fmt.Errorf("failed to take group snapshot %s without a group snapshot class", groupSnapshot.Name)
	}

	volumes, err := ctrl.getVolumesFromVolumeGroupSnapshot(groupSnapshot)
	if err != nil {
		klog.Errorf("getCreateGroupSnapshotInput failed to get PersistentVolume objects [%s]: Error: [%#v]", groupSnapshot.Name, err)
		return nil, nil, "", err
	}

	// Create VolumeGroupSnapshotContent name
	contentName := utils.GetDynamicSnapshotContentNameForGroupSnapshot(groupSnapshot)

	return groupSnapshotClass, volumes, contentName, nil
}

// syncGroupSnapshotContent deals with one key off the queue
func (ctrl *csiSnapshotCommonController) syncGroupSnapshotContent(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) error {
	groupSnapshotName := utils.GroupSnapshotRefKey(&groupSnapshotContent.Spec.VolumeGroupSnapshotRef)
	klog.V(4).Infof("synchronizing VolumeGroupSnapshotContent[%s]: group snapshot content is bound to group snapshot %s", groupSnapshotContent.Name, groupSnapshotName)

	klog.V(5).Infof("syncGroupSnapshotContent[%s]: check if we should add invalid label on group snapshot content", groupSnapshotContent.Name)

	// Keep this check in the controller since the validation webhook may not have been deployed.
	if (groupSnapshotContent.Spec.Source.VolumeGroupSnapshotHandle == nil && len(groupSnapshotContent.Spec.Source.PersistentVolumeNames) == 0) ||
		(groupSnapshotContent.Spec.Source.VolumeGroupSnapshotHandle != nil && len(groupSnapshotContent.Spec.Source.PersistentVolumeNames) > 0) {
		err := fmt.Errorf("Exactly one of VolumeGroupSnapshotHandle and PersistentVolumeNames should be specified")
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

	/*
		TODO: Add finalizer to prevent deletion
	*/

	// Check if group snapshot exists in cache store
	// If getGroupSnapshotFromStore returns (nil, nil), it means group snapshot not found
	// and it may have already been deleted, and it will fall into the
	// group snapshot == nil case below
	var groupSnapshot *crdv1alpha1.VolumeGroupSnapshot
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
func (ctrl *csiSnapshotCommonController) getGroupSnapshotFromStore(groupSnapshotName string) (*crdv1alpha1.VolumeGroupSnapshot, error) {
	// Get the VolumeGroupSnapshot by _name_
	var groupSnapshot *crdv1alpha1.VolumeGroupSnapshot
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
	groupSnapshot, ok = obj.(*crdv1alpha1.VolumeGroupSnapshot)
	if !ok {
		return nil, fmt.Errorf("cannot convert object from group snapshot cache to group snapshot %q!?: %#v", groupSnapshotName, obj)
	}
	klog.V(4).Infof("getGroupSnapshotFromStore: group snapshot %s found", groupSnapshotName)

	return groupSnapshot, nil
}

// needsUpdateGroupSnapshotStatus compares group snapshot status with the group snapshot content
// status and decide if group snapshot status needs to be updated based on group snapshot content
// status
func (ctrl *csiSnapshotCommonController) needsUpdateGroupSnapshotStatus(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot, groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) bool {
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
