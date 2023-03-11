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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

// getGroupSnapshotClass is a helper function to get group snapshot class from the class name.
func (ctrl *csiSnapshotCommonController) getGroupSnapshotClass(className string) (*crdv1alpha1.VolumeGroupSnapshotClass, error) {
	klog.V(5).Infof("getGroupSnapshotClass: VolumeGroupSnapshotClassName [%s]", className)

	class, err := ctrl.groupSnapshotClassLister.Get(className)
	if err != nil {
		klog.Errorf("failed to retrieve group snapshot class %s from the informer: %q", className, err)
		return nil, err
	}

	return class, nil
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
	// Only update ReadyToUse in VolumeSnapshot's Status to false if setReadyToFalse is true.
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
	for _, class := range list {
		if utils.IsDefaultAnnotation(class.ObjectMeta) && pvDriver == class.Driver {
			defaultClasses = append(defaultClasses, class)
			klog.V(5).Infof("get defaultGroupClass added: %s, driver: %s", class.Name, pvDriver)
		}
	}
	if len(defaultClasses) == 0 {
		return nil, groupSnapshot, fmt.Errorf("cannot find default group snapshot class")
	}
	if len(defaultClasses) > 1 {
		klog.V(4).Infof("get DefaultClass %d defaults found", len(defaultClasses))
		return nil, groupSnapshot, fmt.Errorf("%d default snapshot classes were found", len(defaultClasses))
	}
	klog.V(5).Infof("setDefaultSnapshotClass [%s]: default VolumeSnapshotClassName [%s]", groupSnapshot.Name, defaultClasses[0].Name)
	groupSnapshotClone := groupSnapshot.DeepCopy()
	groupSnapshotClone.Spec.VolumeGroupSnapshotClassName = &(defaultClasses[0].Name)
	newGroupSnapshot, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshots(groupSnapshotClone.Namespace).Update(context.TODO(), groupSnapshotClone, metav1.UpdateOptions{})
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshot[%s] default class failed %v", utils.GroupSnapshotKey(groupSnapshot), err)
	}
	_, updateErr := ctrl.storeGroupSnapshotUpdate(newGroupSnapshot)
	if updateErr != nil {
		// We will get an "snapshot update" event soon, this is not a big error
		klog.V(4).Infof("setDefaultSnapshotClass [%s]: cannot update internal cache: %v", utils.GroupSnapshotKey(groupSnapshot), updateErr)
	}

	return defaultClasses[0], newGroupSnapshot, nil
}

// pvDriverFromGroupSnapshot is a helper function to get the CSI driver name from the targeted PersistentVolume.
// It looks up the PVC from which the snapshot is specified to be created from, and looks for the PVC's corresponding
// PV. Bi-directional binding will be verified between PVC and PV before the PV's CSI driver is returned.
// For an non-CSI volume, it returns an error immediately as it's not supported.
func (ctrl *csiSnapshotCommonController) pvDriverFromGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (string, error) {
	pvs, err := ctrl.getVolumesFromVolumeGroupSnapshot(groupSnapshot)
	if err != nil {
		return "", err
	}
	// Take any volume to get the driver
	if pvs[0].Spec.PersistentVolumeSource.CSI == nil {
		return "", fmt.Errorf("snapshotting non-CSI volumes is not supported, snapshot:%s/%s", groupSnapshot.Namespace, groupSnapshot.Name)
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
			klog.V(3).Infof("could not sync snapshot %q: %+v", utils.GroupSnapshotKey(groupSnapshot), err)
		} else {
			klog.Errorf("could not sync snapshot %q: %+v", utils.GroupSnapshotKey(groupSnapshot), err)
		}
		return err
	}
	return nil
}

// deleteGroupSnapshot runs in worker thread and handles "groupsnapshot deleted" event.
func (ctrl *csiSnapshotCommonController) deleteGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) {
	_ = ctrl.snapshotStore.Delete(groupSnapshot)
	klog.V(4).Infof("snapshot %q deleted", utils.GroupSnapshotKey(groupSnapshot))

	groupSnapshotContentName := ""
	if groupSnapshot.Status != nil && groupSnapshot.Status.BoundVolumeGroupSnapshotContentName != nil {
		groupSnapshotContentName = *groupSnapshot.Status.BoundVolumeGroupSnapshotContentName
	}
	if groupSnapshotContentName == "" {
		klog.V(5).Infof("deleteGroupSnapshot[%q]: group snapshot content not bound", utils.GroupSnapshotKey(groupSnapshot))
		return
	}

	// sync the content when its group snapshot is deleted.  Explicitly sync'ing
	// the content here in response to group snapshot deletion prevents the content
	// from waiting until the next sync period for its release.
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
	// 3) groupSnapshot.Status.BoundVolumeSnapshotContentName is not set
	if !utils.IsGroupSnapshotReady(groupSnapshot) || !utils.IsBoundVolumeGroupSnapshotContentNameSet(groupSnapshot) {
		//return ctrl.syncUnreadyGroupSnapshot(groupSnapshot)
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
	content, err := ctrl.getGroupSnapshotContentFromStore(*groupSnapshot.Status.BoundVolumeGroupSnapshotContentName)
	if err != nil {
		return nil
	}
	if content == nil {
		// this meant there is no matching group snapshot content in cache found
		// update status of the group snapshot and return
		return ctrl.updateGroupSnapshotErrorStatusWithEvent(groupSnapshot, true, v1.EventTypeWarning, "GroupSnapshotContentMissing", "VolumeGroupSnapshotContent is missing")
	}
	klog.V(5).Infof("syncReadyGroupSnapshot[%s]: VolumeGroupSnapshotContent %q found", utils.GroupSnapshotKey(groupSnapshot), content.Name)
	// check binding from group snapshot content side to make sure the binding is still valid
	if !utils.IsVolumeGroupSnapshotRefSet(groupSnapshot, content) {
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
		// not able to find a matching content
		return nil, nil
	}
	content, ok := obj.(*crdv1alpha1.VolumeGroupSnapshotContent)
	if !ok {
		return nil, fmt.Errorf("expected VolumeSnapshotContent, got %+v", obj)
	}
	return content, nil
}
