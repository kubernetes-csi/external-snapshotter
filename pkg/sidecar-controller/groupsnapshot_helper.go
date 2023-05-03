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

package sidecar_controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	klog "k8s.io/klog/v2"

	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/utils"
)

func (ctrl *csiSnapshotSideCarController) storeGroupSnapshotContentUpdate(groupSnapshotContent interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.groupSnapshotContentStore, groupSnapshotContent, "groupsnapshotcontent")
}

// enqueueGroupSnapshotContentWork adds group snapshot content to given work queue.
func (ctrl *csiSnapshotSideCarController) enqueueGroupSnapshotContentWork(obj interface{}) {
	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if groupSnapshotContent, ok := obj.(*crdv1alpha1.VolumeGroupSnapshotContent); ok {
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(groupSnapshotContent)
		if err != nil {
			klog.Errorf("failed to get key from object: %v, %v", err, groupSnapshotContent)
			return
		}
		klog.V(5).Infof("enqueued %q for sync", objName)
		ctrl.groupSnapshotContentQueue.Add(objName)
	}
}

// groupSnapshotContentWorker processes items from groupSnapshotContentQueue.
// It must run only once, syncGroupSnapshotContent is not assured to be reentrant.
func (ctrl *csiSnapshotSideCarController) groupSnapshotContentWorker() {
	keyObj, quit := ctrl.groupSnapshotContentQueue.Get()
	if quit {
		return
	}
	defer ctrl.groupSnapshotContentQueue.Done(keyObj)

	if err := ctrl.syncGroupSnapshotContentByKey(keyObj.(string)); err != nil {
		// Rather than wait for a full resync, re-add the key to the
		// queue to be processed.
		ctrl.groupSnapshotContentQueue.AddRateLimited(keyObj)
		klog.V(4).Infof("Failed to sync group snapshot content %q, will retry again: %v", keyObj.(string), err)
		return
	}

	// Finally, if no error occurs we forget this item so it does not
	// get queued again until another change happens.
	ctrl.groupSnapshotContentQueue.Forget(keyObj)
	return
}

func (ctrl *csiSnapshotSideCarController) syncGroupSnapshotContentByKey(key string) error {
	klog.V(5).Infof("syncGroupSnapshotContentByKey[%s]", key)

	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.V(4).Infof("error getting name of groupSnapshotContent %q from informer: %v", key, err)
		return nil
	}
	groupSnapshotContent, err := ctrl.groupSnapshotContentLister.Get(name)
	// The group snapshot content still exists in informer cache, the event must
	// have been add/update/sync
	if err == nil {
		if ctrl.isDriverMatch(groupSnapshotContent) {
			err = ctrl.updateGroupSnapshotContentInInformerCache(groupSnapshotContent)
		}
		if err != nil {
			// If error occurs we add this item back to the queue
			return err
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		klog.V(2).Infof("error getting group snapshot content %q from informer: %v", key, err)
		return nil
	}

	// The groupSnapshotContent is not in informer cache, the event must have been
	// "delete"
	groupSnapshotContentObj, found, err := ctrl.groupSnapshotContentStore.GetByKey(key)
	if err != nil {
		klog.V(2).Infof("error getting group snapshot content %q from cache: %v", key, err)
		return nil
	}
	if !found {
		// The controller has already processed the delete event and
		// deleted the group snapshot content from its cache
		klog.V(2).Infof("deletion of group snapshot content %q was already processed", key)
		return nil
	}
	groupSnapshotContent, ok := groupSnapshotContentObj.(*crdv1alpha1.VolumeGroupSnapshotContent)
	if !ok {
		klog.Errorf("expected group snapshot content, got %+v", groupSnapshotContent)
		return nil
	}
	ctrl.deleteGroupSnapshotContentInCacheStore(groupSnapshotContent)
	return nil
}

// updateGroupSnapshotContentInInformerCache runs in worker thread and handles
// "group snapshot content added", "group snapshot content updated" and "periodic
// sync" events.
func (ctrl *csiSnapshotSideCarController) updateGroupSnapshotContentInInformerCache(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) error {
	// Store the new group snapshot content version in the cache and do not process
	// it if this is an old version.
	new, err := ctrl.storeGroupSnapshotContentUpdate(groupSnapshotContent)
	if err != nil {
		klog.Errorf("%v", err)
	}
	if !new {
		return nil
	}
	err = ctrl.syncGroupSnapshotContent(groupSnapshotContent)
	if err != nil {
		if errors.IsConflict(err) {
			// Version conflict error happens quite often and the controller
			// recovers from it easily.
			klog.V(3).Infof("could not sync group snapshot content %q: %+v", groupSnapshotContent.Name, err)
		} else {
			klog.Errorf("could not sync group snapshot content %q: %+v", groupSnapshotContent.Name, err)
		}
		return err
	}
	return nil
}

// deleteGroupSnapshotContentInCacheStore runs in worker thread and handles "group
// snapshot content deleted" event.
func (ctrl *csiSnapshotSideCarController) deleteGroupSnapshotContentInCacheStore(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) {
	_ = ctrl.groupSnapshotContentStore.Delete(groupSnapshotContent)
	klog.V(4).Infof("group snapshot content %q deleted", groupSnapshotContent.Name)
}

// syncGroupSnapshotContent deals with one key off the queue.  It returns false when it's time to quit.
func (ctrl *csiSnapshotSideCarController) syncGroupSnapshotContent(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) error {
	klog.V(5).Infof("synchronizing VolumeGroupSnapshotContent[%s]", groupSnapshotContent.Name)

	/*
		TODO: Check if the group snapshot content should be deleted
	*/

	if len(groupSnapshotContent.Spec.Source.PersistentVolumeNames) != 0 && groupSnapshotContent.Status == nil {
		klog.V(5).Infof("syncGroupSnapshotContent: Call CreateGroupSnapshot for group snapshot content %s", groupSnapshotContent.Name)
		return ctrl.createGroupSnapshot(groupSnapshotContent)
	}

	// Skip checkandUpdateGroupSnapshotContentStatus() if ReadyToUse is already
	// true. We don't want to keep calling CreateGroupSnapshot CSI methods over
	// and over again for performance reasons.
	var err error
	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.ReadyToUse != nil && *groupSnapshotContent.Status.ReadyToUse == true {
		// Try to remove AnnVolumeGroupSnapshotBeingCreated if it is not removed yet for some reason
		_, err = ctrl.removeAnnVolumeGroupSnapshotBeingCreated(groupSnapshotContent)
		return err
	}
	return ctrl.checkandUpdateGroupSnapshotContentStatus(groupSnapshotContent)
}

// createGroupSnapshot starts new asynchronous operation to create group snapshot
func (ctrl *csiSnapshotSideCarController) createGroupSnapshot(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) error {
	klog.V(5).Infof("createGroupSnapshot for group snapshot content [%s]: started", groupSnapshotContent.Name)
	groupSnapshotContentObj, err := ctrl.createGroupSnapshotWrapper(groupSnapshotContent)
	if err != nil {
		ctrl.updateGroupSnapshotContentErrorStatusWithEvent(groupSnapshotContentObj, v1.EventTypeWarning, "GroupSnapshotCreationFailed", fmt.Sprintf("Failed to create group snapshot: %v", err))
		klog.Errorf("createGroupSnapshot for groupSnapshotContent [%s]: error occurred in createGroupSnapshotWrapper: %v", groupSnapshotContent.Name, err)
		return err
	}

	_, updateErr := ctrl.storeGroupSnapshotContentUpdate(groupSnapshotContentObj)
	if updateErr != nil {
		// We will get a "group snapshot update" event soon, this is not a big error
		klog.V(4).Infof("createGroupSnapshot for groupSnapshotContent [%s]: cannot update internal groupSnapshotContent cache: %v", groupSnapshotContent.Name, updateErr)
	}

	return nil
}

// This is a wrapper function for the group snapshot creation process.
func (ctrl *csiSnapshotSideCarController) createGroupSnapshotWrapper(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	klog.Infof("createGroupSnapshotWrapper: Creating group snapshot for group snapshot content %s through the plugin ...", groupSnapshotContent.Name)

	class, snapshotterCredentials, err := ctrl.getCSIGroupSnapshotInput(groupSnapshotContent)
	if err != nil {
		return groupSnapshotContent, fmt.Errorf("failed to get input parameters to create group snapshot for group snapshot content %s: %q", groupSnapshotContent.Name, err)
	}

	// NOTE(xyang): handle create timeout
	// Add an annotation to indicate the group snapshot creation request has been
	// sent to the storage system and the controller is waiting for a response.
	// The annotation will be removed after the storage system has responded with
	// success or permanent failure. If the request times out, annotation will
	// remain on the groupSnapshotContent to avoid potential leaking of a group snapshot resource on
	// the storage system.
	groupSnapshotContent, err = ctrl.setAnnVolumeGroupSnapshotBeingCreated(groupSnapshotContent)
	if err != nil {
		return groupSnapshotContent, fmt.Errorf("failed to add VolumeGroupSnapshotBeingCreated annotation on the group snapshot content %s: %q", groupSnapshotContent.Name, err)
	}

	parameters, err := utils.RemovePrefixedParameters(class.Parameters)
	if err != nil {
		return groupSnapshotContent, fmt.Errorf("failed to remove CSI Parameters of prefixed keys: %v", err)
	}
	if ctrl.extraCreateMetadata {
		parameters[utils.PrefixedVolumeGroupSnapshotNameKey] = groupSnapshotContent.Spec.VolumeGroupSnapshotRef.Name
		parameters[utils.PrefixedVolumeGroupSnapshotNamespaceKey] = groupSnapshotContent.Spec.VolumeGroupSnapshotRef.Namespace
		parameters[utils.PrefixedVolumeGroupSnapshotContentNameKey] = groupSnapshotContent.Name
	}

	volumeIDs, uuidMap, err := ctrl.getGroupSnapshotVolumeIDs(groupSnapshotContent)
	driverName, groupSnapshotID, snapshots, creationTime, readyToUse, err := ctrl.handler.CreateGroupSnapshot(groupSnapshotContent, volumeIDs, parameters, snapshotterCredentials)
	if err != nil {
		// NOTE(xyang): handle create timeout
		// If it is a final error, remove annotation to indicate
		// storage system has responded with an error
		klog.Infof("createGroupSnapshotWrapper: CreateGroupSnapshot for groupSnapshotContent %s returned error: %v", groupSnapshotContent.Name, err)
		if isCSIFinalError(err) {
			var removeAnnotationErr error
			if groupSnapshotContent, removeAnnotationErr = ctrl.removeAnnVolumeGroupSnapshotBeingCreated(groupSnapshotContent); removeAnnotationErr != nil {
				return groupSnapshotContent, fmt.Errorf("failed to remove VolumeGroupSnapshotBeingCreated annotation from the group snapshot content %s: %s", groupSnapshotContent.Name, removeAnnotationErr)
			}
		}

		return groupSnapshotContent, fmt.Errorf("failed to take group snapshot of the volumes %s: %q", groupSnapshotContent.Spec.Source.PersistentVolumeNames, err)
	}

	klog.V(5).Infof("Created group snapshot: driver %s, groupSnapshotId %s, creationTime %v, readyToUse %t", driverName, groupSnapshotID, creationTime, readyToUse)

	if creationTime.IsZero() {
		creationTime = time.Now()
	}

	// Create individual snapshots and snapshot contents
	var snapshotContentNames []string
	for _, snapshot := range snapshots {
		uuid, ok := uuidMap[snapshot.SourceVolumeId]
		if !ok {
			continue
		}
		volumeSnapshotContentName := GetSnapshotContentNameForVolumeGroupSnapshotContent(string(groupSnapshotContent.UID), uuid)
		volumeSnapshotName := GetSnapshotNameForVolumeGroupSnapshotContent(string(groupSnapshotContent.UID), uuid)
		volumeSnapshotNamespace := groupSnapshotContent.Spec.VolumeGroupSnapshotRef.Namespace
		volumeSnapshotContent := &crdv1.VolumeSnapshotContent{
			ObjectMeta: metav1.ObjectMeta{
				Name: volumeSnapshotContentName,
			},
			Spec: crdv1.VolumeSnapshotContentSpec{
				VolumeSnapshotRef: v1.ObjectReference{
					Kind:      "VolumeSnapshots",
					Name:      volumeSnapshotName,
					Namespace: volumeSnapshotNamespace,
				},
				DeletionPolicy: groupSnapshotContent.Spec.DeletionPolicy,
				Driver:         groupSnapshotContent.Spec.Driver,
				Source: crdv1.VolumeSnapshotContentSource{
					SnapshotHandle: &snapshot.SnapshotId,
				},
				// TODO: Populate this field when volume mode conversion is enabled by default
				SourceVolumeMode: nil,
			},
			Status: &crdv1.VolumeSnapshotContentStatus{
				VolumeGroupSnapshotContentName: &groupSnapshotContent.Name,
			},
		}
		label := make(map[string]string)
		label["volumeGroupSnapshotName"] = groupSnapshotContent.Spec.VolumeGroupSnapshotRef.Name
		name := "f"
		volumeSnapshot := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      volumeSnapshotName,
				Namespace: volumeSnapshotNamespace,
				Labels:    label,
			},
			Spec: crdv1.VolumeSnapshotSpec{
				Source: crdv1.VolumeSnapshotSource{
					VolumeSnapshotContentName: &volumeSnapshotContentName,
				},
			},
		}
		vsc, err := ctrl.clientset.SnapshotV1().VolumeSnapshotContents().Create(context.TODO(), volumeSnapshotContent, metav1.CreateOptions{})
		if err != nil {
			return groupSnapshotContent, err
		}
		snapshotContentNames = append(snapshotContentNames, vsc.Name)

		klog.Infof("making snapshot %v %s %s", volumeSnapshot.Status, *volumeSnapshot.Status.VolumeGroupSnapshotName, name)
		_, err = ctrl.clientset.SnapshotV1().VolumeSnapshots(volumeSnapshotNamespace).Create(context.TODO(), volumeSnapshot, metav1.CreateOptions{})
		if err != nil {
			return groupSnapshotContent, err
		}
		//		klog.Infof("raunak made snapshot 1 %v", spew.Sdump(sn))
		//		sn.Status = &crdv1.VolumeSnapshotStatus{
		//			VolumeGroupSnapshotName: &name,
		//		}
		//		sn, err = ctrl.clientset.SnapshotV1().VolumeSnapshots(volumeSnapshotNamespace).UpdateStatus(context.TODO(), sn, metav1.UpdateOptions{})
		//		if err != nil {
		//			klog.Infof("failed 2")
		//			return groupSnapshotContent, err
		//		}
		//		klog.Infof("made snapshot 2 %v", spew.Sdump(sn))
	}
	klog.Infof("raunak 2")
	newGroupSnapshotContent, err := ctrl.updateGroupSnapshotContentStatus(groupSnapshotContent, groupSnapshotID, readyToUse, creationTime.UnixNano(), snapshotContentNames)
	if err != nil {
		klog.Errorf("error updating status for volume group snapshot content %s: %v.", groupSnapshotContent.Name, err)
		return groupSnapshotContent, fmt.Errorf("error updating status for volume group snapshot content %s: %v", groupSnapshotContent.Name, err)
	}
	groupSnapshotContent = newGroupSnapshotContent

	// NOTE(xyang): handle create timeout
	// Remove annotation to indicate storage system has successfully
	// cut the group snapshot
	groupSnapshotContent, err = ctrl.removeAnnVolumeGroupSnapshotBeingCreated(groupSnapshotContent)
	if err != nil {
		return groupSnapshotContent, fmt.Errorf("failed to remove VolumeGroupSnapshotBeingCreated annotation on the groupSnapshotContent %s: %q", groupSnapshotContent.Name, err)
	}
	return groupSnapshotContent, nil
}

func (ctrl *csiSnapshotSideCarController) getCSIGroupSnapshotInput(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotClass, map[string]string, error) {
	className := groupSnapshotContent.Spec.VolumeGroupSnapshotClassName
	klog.V(5).Infof("getCSIGroupSnapshotInput for group snapshot content [%s]", groupSnapshotContent.Name)
	var class *crdv1alpha1.VolumeGroupSnapshotClass
	var err error
	if className != nil {
		class, err = ctrl.getGroupSnapshotClass(*className)
		if err != nil {
			klog.Errorf("getCSISnapshotInput failed to getClassFromVolumeGroupSnapshot %s", err)
			return nil, nil, err
		}
	} else {
		// If dynamic provisioning, return failure if no group snapshot class
		if len(groupSnapshotContent.Spec.Source.PersistentVolumeNames) != 0 {
			klog.Errorf("failed to getCSISnapshotInput %s without a group snapshot class", groupSnapshotContent.Name)
			return nil, nil, fmt.Errorf("failed to take group snapshot %s without a group snapshot class", groupSnapshotContent.Name)
		}
		// For pre-provisioned group snapshot, group snapshot class is not required
		klog.V(5).Infof("getCSISnapshotInput for groupSnapshotContent [%s]: no VolumeGroupSnapshotClassName provided for pre-provisioned group snapshot", groupSnapshotContent.Name)
	}

	// TODO: Resolve snapshotting secret credentials.

	return class, nil, nil
}

// getGroupSnapshotClass is a helper function to get group snapshot class from the class name.
func (ctrl *csiSnapshotSideCarController) getGroupSnapshotClass(className string) (*crdv1alpha1.VolumeGroupSnapshotClass, error) {
	klog.V(5).Infof("getGroupSnapshotClass: VolumeGroupSnapshotClassName [%s]", className)

	class, err := ctrl.groupSnapshotClassLister.Get(className)
	if err != nil {
		klog.Errorf("failed to retrieve group snapshot class %s from the informer: %q", className, err)
		return nil, err
	}

	return class, nil
}

// setAnnVolumeGroupSnapshotBeingCreated sets VolumeGroupSnapshotBeingCreated annotation
// on VolumeGroupSnapshotContent
// If set, it indicates group snapshot is being created
func (ctrl *csiSnapshotSideCarController) setAnnVolumeGroupSnapshotBeingCreated(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	if metav1.HasAnnotation(groupSnapshotContent.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated) {
		// the annotation already exists, return directly
		return groupSnapshotContent, nil
	}

	// Set AnnVolumeGroupSnapshotBeingCreated
	// Combine existing annotations with the new annotations.
	// If there are no existing annotations, we create a new map.
	klog.V(5).Infof("setAnnVolumeGroupSnapshotBeingCreated: set annotation [%s:yes] on groupSnapshotContent [%s].", utils.AnnVolumeGroupSnapshotBeingCreated, groupSnapshotContent.Name)
	patchedAnnotations := make(map[string]string)
	for k, v := range groupSnapshotContent.GetAnnotations() {
		patchedAnnotations[k] = v
	}
	patchedAnnotations[utils.AnnVolumeGroupSnapshotBeingCreated] = "yes"

	var patches []utils.PatchOp
	patches = append(patches, utils.PatchOp{
		Op:    "replace",
		Path:  "/metadata/annotations",
		Value: patchedAnnotations,
	})

	patchedGroupSnapshotContent, err := utils.PatchVolumeGroupSnapshotContent(groupSnapshotContent, patches, ctrl.clientset)
	if err != nil {
		return groupSnapshotContent, newControllerUpdateError(groupSnapshotContent.Name, err.Error())
	}
	// update groupSnapshotContent if update is successful
	groupSnapshotContent = patchedGroupSnapshotContent

	_, err = ctrl.storeContentUpdate(groupSnapshotContent)
	if err != nil {
		klog.V(4).Infof("setAnnVolumeGroupSnapshotBeingCreated for groupSnapshotContent [%s]: cannot update internal cache %v", groupSnapshotContent.Name, err)
	}
	klog.V(5).Infof("setAnnVolumeGroupSnapshotBeingCreated: volume group snapshot content %+v", groupSnapshotContent)

	return groupSnapshotContent, nil
}

func (ctrl *csiSnapshotSideCarController) getGroupSnapshotVolumeIDs(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) ([]string, map[string]string, error) {
	// TODO: Get add PV lister
	var volumeIDs []string
	uuidMap := make(map[string]string)
	for _, pvName := range groupSnapshotContent.Spec.Source.PersistentVolumeNames {
		pv, err := ctrl.client.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
		if err != nil {
			return nil, nil, err
		}
		if pv.Spec.CSI != nil && pv.Spec.CSI.VolumeHandle != "" {
			volumeIDs = append(volumeIDs, pv.Spec.CSI.VolumeHandle)
			if pv.Spec.ClaimRef != nil {
				uuidMap[pv.Spec.CSI.VolumeHandle] = string(pv.Spec.ClaimRef.UID)
			}
		}
	}
	return volumeIDs, uuidMap, nil
}

// removeAnnVolumeGroupSnapshotBeingCreated removes the VolumeGroupSnapshotBeingCreated
// annotation from a groupSnapshotContent if there exists one.
func (ctrl csiSnapshotSideCarController) removeAnnVolumeGroupSnapshotBeingCreated(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	if !metav1.HasAnnotation(groupSnapshotContent.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated) {
		// the annotation does not exist, return directly
		return groupSnapshotContent, nil
	}
	groupSnapshotContentClone := groupSnapshotContent.DeepCopy()
	delete(groupSnapshotContentClone.ObjectMeta.Annotations, utils.AnnVolumeGroupSnapshotBeingCreated)

	updatedContent, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().Update(context.TODO(), groupSnapshotContentClone, metav1.UpdateOptions{})
	if err != nil {
		return groupSnapshotContent, newControllerUpdateError(groupSnapshotContent.Name, err.Error())
	}

	klog.V(5).Infof("Removed VolumeGroupSnapshotBeingCreated annotation from volume group snapshot content %s", groupSnapshotContent.Name)
	_, err = ctrl.storeContentUpdate(updatedContent)
	if err != nil {
		klog.Errorf("failed to update groupSnapshotContent store %v", err)
	}
	return updatedContent, nil
}

func (ctrl *csiSnapshotSideCarController) updateGroupSnapshotContentStatus(
	groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent,
	groupSnapshotHandle string,
	readyToUse bool,
	createdAt int64,
	snapshotContentNames []string) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	klog.V(5).Infof("updateSnapshotContentStatus: updating VolumeGroupSnapshotContent [%s], groupSnapshotHandle %s, readyToUse %v, createdAt %v", groupSnapshotContent.Name, groupSnapshotHandle, readyToUse, createdAt)

	groupSnapshotContentObj, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().Get(context.TODO(), groupSnapshotContent.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get group snapshot content %s from api server: %v", groupSnapshotContent.Name, err)
	}

	var newStatus *crdv1alpha1.VolumeGroupSnapshotContentStatus
	updated := false
	if groupSnapshotContentObj.Status == nil {
		newStatus = &crdv1alpha1.VolumeGroupSnapshotContentStatus{
			VolumeGroupSnapshotHandle: &groupSnapshotHandle,
			ReadyToUse:                &readyToUse,
			CreationTime:              &createdAt,
		}
		for _, name := range snapshotContentNames {
			newStatus.VolumeSnapshotContentRefList = append(newStatus.VolumeSnapshotContentRefList, v1.ObjectReference{
				Kind: "VolumeSnapshotContent",
				Name: name,
			})
		}
		updated = true
	} else {
		newStatus = groupSnapshotContentObj.Status.DeepCopy()
		if newStatus.VolumeGroupSnapshotHandle == nil {
			newStatus.VolumeGroupSnapshotHandle = &groupSnapshotHandle
			updated = true
		}
		if newStatus.ReadyToUse == nil || *newStatus.ReadyToUse != readyToUse {
			newStatus.ReadyToUse = &readyToUse
			updated = true
			if readyToUse && newStatus.Error != nil {
				newStatus.Error = nil
			}
		}
		if newStatus.CreationTime == nil {
			newStatus.CreationTime = &createdAt
			updated = true
		}
		if len(newStatus.VolumeSnapshotContentRefList) == 0 {
			for _, name := range snapshotContentNames {
				newStatus.VolumeSnapshotContentRefList = append(newStatus.VolumeSnapshotContentRefList, v1.ObjectReference{
					Kind: "VolumeSnapshotContent",
					Name: name,
				})
			}
			updated = true
		}
	}

	if updated {
		groupSnapshotContentClone := groupSnapshotContentObj.DeepCopy()
		groupSnapshotContentClone.Status = newStatus
		newContent, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().UpdateStatus(context.TODO(), groupSnapshotContentClone, metav1.UpdateOptions{})
		if err != nil {
			return groupSnapshotContentObj, newControllerUpdateError(groupSnapshotContent.Name, err.Error())
		}
		return newContent, nil
	}

	return groupSnapshotContentObj, nil
}

// updateContentStatusWithEvent saves new groupSnapshotContent.Status to API server
// and emits given event on the groupSnapshotContent. It saves the status and emits
// the event only when the status has actually changed from the version saved in API server.
// Parameters:
//
// * groupSnapshotContent - group snapshot content to update
// * eventtype, reason, message - event to send, see EventRecorder.Event()
func (ctrl *csiSnapshotSideCarController) updateGroupSnapshotContentErrorStatusWithEvent(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent, eventtype, reason, message string) error {
	klog.V(5).Infof("updateGroupSnapshotContentStatusWithEvent[%s]", groupSnapshotContent.Name)

	if groupSnapshotContent.Status != nil && groupSnapshotContent.Status.Error != nil && *groupSnapshotContent.Status.Error.Message == message {
		klog.V(4).Infof("updateGroupSnapshotContentStatusWithEvent[%s]: the same error %v is already set", groupSnapshotContent.Name, groupSnapshotContent.Status.Error)
		return nil
	}

	var patches []utils.PatchOp
	ready := false
	groupSnapshotContentStatusError := &crdv1.VolumeSnapshotError{
		Time: &metav1.Time{
			Time: time.Now(),
		},
		Message: &message,
	}
	if groupSnapshotContent.Status == nil {
		// Initialize status if nil
		patches = append(patches, utils.PatchOp{
			Op:   "replace",
			Path: "/status",
			Value: &crdv1alpha1.VolumeGroupSnapshotContentStatus{
				ReadyToUse: &ready,
				Error:      groupSnapshotContentStatusError,
			},
		})
	} else {
		// Patch status if non-nil
		patches = append(patches, utils.PatchOp{
			Op:    "replace",
			Path:  "/status/error",
			Value: groupSnapshotContentStatusError,
		})
		patches = append(patches, utils.PatchOp{
			Op:    "replace",
			Path:  "/status/readyToUse",
			Value: &ready,
		})

	}

	newContent, err := utils.PatchVolumeGroupSnapshotContent(groupSnapshotContent, patches, ctrl.clientset, "status")

	// Emit the event even if the status update fails so that user can see the error
	ctrl.eventRecorder.Event(newContent, eventtype, reason, message)

	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshotContent[%s] error status failed %v", groupSnapshotContent.Name, err)
		return err
	}

	_, err = ctrl.storeGroupSnapshotContentUpdate(newContent)
	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshotContent[%s] error status: cannot update internal cache %v", groupSnapshotContent.Name, err)
		return err
	}

	return nil
}

// GetSnapshotNameForVolumeGroupSnapshotContent returns a unique snapshot name for a VolumeGroupSnapshotContent.
func GetSnapshotNameForVolumeGroupSnapshotContent(groupSnapshotContentUUID, pvUUID string) string {
	return fmt.Sprintf("snapshot-%x-%s", sha256.Sum256([]byte(groupSnapshotContentUUID+pvUUID)), time.Now().Format("2006-01-02-3.4.5"))
}

// GetSnapshotContentNameForVolumeGroupSnapshotContent returns a unique content name for the
// passed in VolumeGroupSnapshotContent.
func GetSnapshotContentNameForVolumeGroupSnapshotContent(groupSnapshotContentUUID, pvUUID string) string {
	return fmt.Sprintf("snapcontent-%x-%s", sha256.Sum256([]byte(groupSnapshotContentUUID+pvUUID)), time.Now().Format("2006-01-02-3.4.5"))
}

func (ctrl *csiSnapshotSideCarController) checkandUpdateGroupSnapshotContentStatus(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) error {
	klog.V(5).Infof("checkandUpdateGroupSnapshotContentStatus[%s] started", groupSnapshotContent.Name)
	groupSnapshotContentObj, err := ctrl.checkandUpdateGroupSnapshotContentStatusOperation(groupSnapshotContent)
	if err != nil {
		ctrl.updateGroupSnapshotContentErrorStatusWithEvent(groupSnapshotContentObj, v1.EventTypeWarning, "GroupSnapshotContentCheckandUpdateFailed", fmt.Sprintf("Failed to check and update group snapshot content: %v", err))
		klog.Errorf("checkandUpdateGroupSnapshotContentStatus [%s]: error occurred %v", groupSnapshotContent.Name, err)
		return err
	}
	_, updateErr := ctrl.storeGroupSnapshotContentUpdate(groupSnapshotContentObj)
	if updateErr != nil {
		// We will get a "group snapshot update" event soon, this is not a big error
		klog.V(4).Infof("checkandUpdateGroupSnapshotContentStatus [%s]: cannot update internal cache: %v", groupSnapshotContent.Name, updateErr)
	}

	return nil
}

func (ctrl *csiSnapshotSideCarController) checkandUpdateGroupSnapshotContentStatusOperation(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	var err error
	var creationTime time.Time
	readyToUse := false
	var driverName string
	var groupSnapshotID string
	var snapshotterListCredentials map[string]string

	if groupSnapshotContent.Spec.Source.VolumeGroupSnapshotHandle != nil {
		klog.V(5).Infof("checkandUpdateGroupSnapshotContentStatusOperation: call GetGroupSnapshotStatus for group snapshot which is pre-bound to group snapshot content [%s]", groupSnapshotContent.Name)

		if groupSnapshotContent.Spec.VolumeGroupSnapshotClassName != nil {
			class, err := ctrl.getGroupSnapshotClass(*groupSnapshotContent.Spec.VolumeGroupSnapshotClassName)
			if err != nil {
				klog.Errorf("Failed to get group snapshot class %s for group snapshot content %s: %v", *groupSnapshotContent.Spec.VolumeGroupSnapshotClassName, groupSnapshotContent.Name, err)
				return groupSnapshotContent, fmt.Errorf("failed to get group snapshot class %s for group snapshot content %s: %v", *groupSnapshotContent.Spec.VolumeGroupSnapshotClassName, groupSnapshotContent.Name, err)
			}

			snapshotterListSecretRef, err := utils.GetSecretReference(utils.SnapshotterListSecretParams, class.Parameters, groupSnapshotContent.GetObjectMeta().GetName(), nil)
			if err != nil {
				klog.Errorf("Failed to get secret reference for group snapshot content %s: %v", groupSnapshotContent.Name, err)
				return groupSnapshotContent, fmt.Errorf("failed to get secret reference for group snapshot content %s: %v", groupSnapshotContent.Name, err)
			}

			snapshotterListCredentials, err = utils.GetCredentials(ctrl.client, snapshotterListSecretRef)
			if err != nil {
				// Continue with deletion, as the secret may have already been deleted.
				klog.Errorf("Failed to get credentials for group snapshot content %s: %v", groupSnapshotContent.Name, err)
				return groupSnapshotContent, fmt.Errorf("failed to get credentials for group snapshot content %s: %v", groupSnapshotContent.Name, err)
			}
		}

		readyToUse, creationTime, err = ctrl.handler.GetGroupSnapshotStatus(groupSnapshotContent, snapshotterListCredentials)
		if err != nil {
			klog.Errorf("checkandUpdateGroupSnapshotContentStatusOperation: failed to call get group snapshot status to check whether group snapshot is ready to use %q", err)
			return groupSnapshotContent, err
		}
		driverName = groupSnapshotContent.Spec.Driver
		groupSnapshotID = *groupSnapshotContent.Spec.Source.VolumeGroupSnapshotHandle

		klog.V(5).Infof("checkandUpdateGroupSnapshotContentStatusOperation: driver %s, groupSnapshotId %s, creationTime %v, size %d, readyToUse %t", driverName, groupSnapshotID, creationTime, readyToUse)

		if creationTime.IsZero() {
			creationTime = time.Now()
		}

		// TODO: Get a reference to snapshot contents for this volume group snapshot
		updatedContent, err := ctrl.updateGroupSnapshotContentStatus(groupSnapshotContent, groupSnapshotID, readyToUse, creationTime.UnixNano(), []string{})
		if err != nil {
			return groupSnapshotContent, err
		}
		return updatedContent, nil
	}
	return ctrl.createGroupSnapshotWrapper(groupSnapshotContent)
}
