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

func (ctrl *csiSnapshotSideCarController) storeGroupSnapshotContentUpdate(content interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.groupSnapshotContentStore, content, "groupsnapshotcontent")
}

// enqueueGroupSnapshotContentWork adds group snapshot content to given work queue.
func (ctrl *csiSnapshotSideCarController) enqueueGroupSnapshotContentWork(obj interface{}) {
	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if content, ok := obj.(*crdv1alpha1.VolumeGroupSnapshotContent); ok {
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(content)
		if err != nil {
			klog.Errorf("failed to get key from object: %v, %v", err, content)
			return
		}
		klog.V(5).Infof("enqueued %q for sync", objName)
		ctrl.groupSnapshotContentQueue.Add(objName)
	}
}

// groupSnapshotContentWorker processes items from groupSnapshotContentQueue.
// It must run only once, syncContent is not assured to be reentrant.
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
	content, err := ctrl.groupSnapshotContentLister.Get(name)
	// The group snapshot content still exists in informer cache, the event must
	// have been add/update/sync
	if err == nil {
		if ctrl.isDriverMatch(content) {
			err = ctrl.updateGroupSnapshotContentInInformerCache(content)
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

	// The content is not in informer cache, the event must have been
	// "delete"
	contentObj, found, err := ctrl.groupSnapshotContentStore.GetByKey(key)
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
	content, ok := contentObj.(*crdv1alpha1.VolumeGroupSnapshotContent)
	if !ok {
		klog.Errorf("expected group snapshot content, got %+v", content)
		return nil
	}
	ctrl.deleteGroupSnapshotContentInCacheStore(content)
	return nil
}

// updateGroupSnapshotContentInInformerCache runs in worker thread and handles
// "group snapshot content added", "group snapshot content updated" and "periodic
// sync" events.
func (ctrl *csiSnapshotSideCarController) updateGroupSnapshotContentInInformerCache(content *crdv1alpha1.VolumeGroupSnapshotContent) error {
	// Store the new group snapshot content version in the cache and do not process
	// it if this is an old version.
	new, err := ctrl.storeGroupSnapshotContentUpdate(content)
	if err != nil {
		klog.Errorf("%v", err)
	}
	if !new {
		return nil
	}
	err = ctrl.syncGroupSnapshotContent(content)
	if err != nil {
		if errors.IsConflict(err) {
			// Version conflict error happens quite often and the controller
			// recovers from it easily.
			klog.V(3).Infof("could not sync group snapshot content %q: %+v", content.Name, err)
		} else {
			klog.Errorf("could not sync group snapshot content %q: %+v", content.Name, err)
		}
		return err
	}
	return nil
}

// deleteGroupSnapshotContentInCacheStore runs in worker thread and handles "group
// snapshot content deleted" event.
func (ctrl *csiSnapshotSideCarController) deleteGroupSnapshotContentInCacheStore(content *crdv1alpha1.VolumeGroupSnapshotContent) {
	_ = ctrl.groupSnapshotContentStore.Delete(content)
	klog.V(4).Infof("group snapshot content %q deleted", content.Name)
}

// syncGroupSnapshotContent deals with one key off the queue.  It returns false when it's time to quit.
func (ctrl *csiSnapshotSideCarController) syncGroupSnapshotContent(content *crdv1alpha1.VolumeGroupSnapshotContent) error {
	klog.V(5).Infof("synchronizing VolumeGroupSnapshotContent[%s]", content.Name)

	/*
		TODO: Check if the group snapshot content should be deleted
	*/

	if len(content.Spec.Source.PersistentVolumeNames) != 0 && content.Status == nil {
		klog.V(5).Infof("syncContent: Call CreateGroupSnapshot for group snapshot content %s", content.Name)
		return ctrl.createGroupSnapshot(content)
	}

	// Skip checkandUpdateGroupSnapshotContentStatus() if ReadyToUse is already
	// true. We don't want to keep calling CreateGroupSnapshot CSI methods over
	// and over again for performance reasons.
	var err error
	if content.Status != nil && content.Status.ReadyToUse != nil && *content.Status.ReadyToUse == true {
		// Try to remove AnnVolumeGroupSnapshotBeingCreated if it is not removed yet for some reason
		_, err = ctrl.removeAnnVolumeGroupSnapshotBeingCreated(content)
		return err
	}
	return ctrl.checkandUpdateGroupSnapshotContentStatus(content)
}

// createGroupSnapshot starts new asynchronous operation to create group snapshot
func (ctrl *csiSnapshotSideCarController) createGroupSnapshot(content *crdv1alpha1.VolumeGroupSnapshotContent) error {
	klog.V(5).Infof("createGroupSnapshot for group snapshot content [%s]: started", content.Name)
	contentObj, err := ctrl.createGroupSnapshotWrapper(content)
	if err != nil {
		ctrl.updateGroupSnapshotContentErrorStatusWithEvent(contentObj, v1.EventTypeWarning, "SnapshotCreationFailed", fmt.Sprintf("Failed to create group snapshot: %v", err))
		klog.Errorf("createSnapshot for content [%s]: error occurred in createSnapshotWrapper: %v", content.Name, err)
		return err
	}

	_, updateErr := ctrl.storeGroupSnapshotContentUpdate(contentObj)
	if updateErr != nil {
		// We will get a "group snapshot update" event soon, this is not a big error
		klog.V(4).Infof("createSnapshot for content [%s]: cannot update internal content cache: %v", content.Name, updateErr)
	}

	return nil
}

// This is a wrapper function for the group snapshot creation process.
func (ctrl *csiSnapshotSideCarController) createGroupSnapshotWrapper(content *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	klog.Infof("createGroupSnapshotWrapper: Creating group snapshot for group snapshot content %s through the plugin ...", content.Name)

	class, snapshotterCredentials, err := ctrl.getCSIGroupSnapshotInput(content)
	if err != nil {
		return content, fmt.Errorf("failed to get input parameters to create group snapshot for content %s: %q", content.Name, err)
	}

	// NOTE(xyang): handle create timeout
	// Add an annotation to indicate the group snapshot creation request has been
	// sent to the storage system and the controller is waiting for a response.
	// The annotation will be removed after the storage system has responded with
	// success or permanent failure. If the request times out, annotation will
	// remain on the content to avoid potential leaking of a group snapshot resource on
	// the storage system.
	content, err = ctrl.setAnnVolumeGroupSnapshotBeingCreated(content)
	if err != nil {
		return content, fmt.Errorf("failed to add VolumeGroupSnapshotBeingCreated annotation on the content %s: %q", content.Name, err)
	}

	parameters, err := utils.RemovePrefixedParameters(class.Parameters)
	if err != nil {
		return content, fmt.Errorf("failed to remove CSI Parameters of prefixed keys: %v", err)
	}
	if ctrl.extraCreateMetadata {
		parameters[utils.PrefixedVolumeGroupSnapshotNameKey] = content.Spec.VolumeGroupSnapshotRef.Name
		parameters[utils.PrefixedVolumeGroupSnapshotNamespaceKey] = content.Spec.VolumeGroupSnapshotRef.Namespace
		parameters[utils.PrefixedVolumeGroupSnapshotContentNameKey] = content.Name
	}

	volumeIDs, err := ctrl.getGroupSnapshotVolumeIDs(content)
	driverName, groupSnapshotID, snapshots, creationTime, readyToUse, err := ctrl.handler.CreateGroupSnapshot(content, volumeIDs, parameters, snapshotterCredentials)
	if err != nil {
		// NOTE(xyang): handle create timeout
		// If it is a final error, remove annotation to indicate
		// storage system has responded with an error
		klog.Infof("createSnapshotWrapper: CreateSnapshot for content %s returned error: %v", content.Name, err)
		if isCSIFinalError(err) {
			var removeAnnotationErr error
			if content, removeAnnotationErr = ctrl.removeAnnVolumeGroupSnapshotBeingCreated(content); removeAnnotationErr != nil {
				return content, fmt.Errorf("failed to remove VolumeGroupSnapshotBeingCreated annotation from the content %s: %s", content.Name, removeAnnotationErr)
			}
		}

		return content, fmt.Errorf("failed to take group snapshot of the volumes %s: %q", content.Spec.Source.PersistentVolumeNames, err)
	}

	klog.V(5).Infof("Created group snapshot: driver %s, groupSnapshotId %s, creationTime %v, readyToUse %t", driverName, groupSnapshotID, creationTime, readyToUse)

	if creationTime.IsZero() {
		creationTime = time.Now()
	}

	// Create individual snapshots and snapshot contents
	for _, snapshot := range snapshots {
		volumeSnapshotContentName := GetSnapshotContentNameForVolumeGroupSnapshotContent(content)
		volumeSnapshotName := GetSnapshotNameForVolumeGroupSnapshotContent(content)
		volumeSnapshotNamespace := content.Spec.VolumeGroupSnapshotRef.Namespace
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
				DeletionPolicy: content.Spec.DeletionPolicy,
				Driver:         content.Spec.Driver,
				Source: crdv1.VolumeSnapshotContentSource{
					SnapshotHandle: &snapshot.SnapshotId,
				},
				SourceVolumeMode: nil,
			},
		}

		volumeSnapshot := &crdv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      volumeSnapshotName,
				Namespace: volumeSnapshotNamespace,
			},
			Spec: crdv1.VolumeSnapshotSpec{
				Source: crdv1.VolumeSnapshotSource{
					VolumeSnapshotContentName: &volumeSnapshotContentName,
				},
			},
		}
		_, err = ctrl.clientset.SnapshotV1().VolumeSnapshotContents().Create(context.TODO(), volumeSnapshotContent, metav1.CreateOptions{})
		if err != nil {
			return content, err
		}

		_, err = ctrl.clientset.SnapshotV1().VolumeSnapshots(volumeSnapshotNamespace).Create(context.TODO(), volumeSnapshot, metav1.CreateOptions{})
		if err != nil {
			return content, err
		}

	}
	newContent, err := ctrl.updateGroupSnapshotContentStatus(content, groupSnapshotID, readyToUse, creationTime.UnixNano())
	if err != nil {
		klog.Errorf("error updating status for volume group snapshot content %s: %v.", content.Name, err)
		return content, fmt.Errorf("error updating status for volume group snapshot content %s: %v", content.Name, err)
	}
	content = newContent

	// NOTE(xyang): handle create timeout
	// Remove annotation to indicate storage system has successfully
	// cut the group snapshot
	content, err = ctrl.removeAnnVolumeGroupSnapshotBeingCreated(content)
	if err != nil {
		return content, fmt.Errorf("failed to remove VolumeGroupSnapshotBeingCreated annotation on the content %s: %q", content.Name, err)
	}
	return content, nil
}

func (ctrl *csiSnapshotSideCarController) getCSIGroupSnapshotInput(content *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotClass, map[string]string, error) {
	className := content.Spec.VolumeGroupSnapshotClassName
	klog.V(5).Infof("getCSIGroupSnapshotInput for group snapshot content [%s]", content.Name)
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
		if len(content.Spec.Source.PersistentVolumeNames) != 0 {
			klog.Errorf("failed to getCSISnapshotInput %s without a group snapshot class", content.Name)
			return nil, nil, fmt.Errorf("failed to take group snapshot %s without a group snapshot class", content.Name)
		}
		// For pre-provisioned group snapshot, group snapshot class is not required
		klog.V(5).Infof("getCSISnapshotInput for content [%s]: no VolumeGroupSnapshotClassName provided for pre-provisioned group snapshot", content.Name)
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
func (ctrl *csiSnapshotSideCarController) setAnnVolumeGroupSnapshotBeingCreated(content *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	if metav1.HasAnnotation(content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated) {
		// the annotation already exists, return directly
		return content, nil
	}

	// Set AnnVolumeGroupSnapshotBeingCreated
	// Combine existing annotations with the new annotations.
	// If there are no existing annotations, we create a new map.
	klog.V(5).Infof("setAnnVolumeGroupSnapshotBeingCreated: set annotation [%s:yes] on content [%s].", utils.AnnVolumeGroupSnapshotBeingCreated, content.Name)
	patchedAnnotations := make(map[string]string)
	for k, v := range content.GetAnnotations() {
		patchedAnnotations[k] = v
	}
	patchedAnnotations[utils.AnnVolumeGroupSnapshotBeingCreated] = "yes"

	var patches []utils.PatchOp
	patches = append(patches, utils.PatchOp{
		Op:    "replace",
		Path:  "/metadata/annotations",
		Value: patchedAnnotations,
	})

	patchedContent, err := utils.PatchVolumeGroupSnapshotContent(content, patches, ctrl.clientset)
	if err != nil {
		return content, newControllerUpdateError(content.Name, err.Error())
	}
	// update content if update is successful
	content = patchedContent

	_, err = ctrl.storeContentUpdate(content)
	if err != nil {
		klog.V(4).Infof("setAnnVolumeGroupSnapshotBeingCreated for content [%s]: cannot update internal cache %v", content.Name, err)
	}
	klog.V(5).Infof("setAnnVolumeGroupSnapshotBeingCreated: volume group snapshot content %+v", content)

	return content, nil
}

func (ctrl *csiSnapshotSideCarController) getGroupSnapshotVolumeIDs(content *crdv1alpha1.VolumeGroupSnapshotContent) ([]string, error) {
	// TODO: Get add PV lister
	var volumeIDs []string
	for _, pvName := range content.Spec.Source.PersistentVolumeNames {
		pv, err := ctrl.client.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if pv.Spec.CSI != nil && pv.Spec.CSI.VolumeHandle != "" {
			volumeIDs = append(volumeIDs, pv.Spec.CSI.VolumeHandle)
		}
	}
	return volumeIDs, nil
}

// removeAnnVolumeGroupSnapshotBeingCreated removes the VolumeGroupSnapshotBeingCreated
// annotation from a content if there exists one.
func (ctrl csiSnapshotSideCarController) removeAnnVolumeGroupSnapshotBeingCreated(content *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	if !metav1.HasAnnotation(content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated) {
		// the annotation does not exist, return directly
		return content, nil
	}
	contentClone := content.DeepCopy()
	delete(contentClone.ObjectMeta.Annotations, utils.AnnVolumeGroupSnapshotBeingCreated)

	updatedContent, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().Update(context.TODO(), contentClone, metav1.UpdateOptions{})
	if err != nil {
		return content, newControllerUpdateError(content.Name, err.Error())
	}

	klog.V(5).Infof("Removed VolumeGroupSnapshotBeingCreated annotation from volume group snapshot content %s", content.Name)
	_, err = ctrl.storeContentUpdate(updatedContent)
	if err != nil {
		klog.Errorf("failed to update content store %v", err)
	}
	return updatedContent, nil
}

func (ctrl *csiSnapshotSideCarController) updateGroupSnapshotContentStatus(
	content *crdv1alpha1.VolumeGroupSnapshotContent,
	groupSnapshotHandle string,
	readyToUse bool,
	createdAt int64) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	klog.V(5).Infof("updateSnapshotContentStatus: updating VolumeGroupSnapshotContent [%s], groupSnapshotHandle %s, readyToUse %v, createdAt %v", content.Name, groupSnapshotHandle, readyToUse, createdAt)

	contentObj, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().Get(context.TODO(), content.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get group snapshot content %s from api server: %v", content.Name, err)
	}

	var newStatus *crdv1alpha1.VolumeGroupSnapshotContentStatus
	updated := false
	if contentObj.Status == nil {
		newStatus = &crdv1alpha1.VolumeGroupSnapshotContentStatus{
			VolumeGroupSnapshotHandle: &groupSnapshotHandle,
			ReadyToUse:                &readyToUse,
			CreationTime:              &createdAt,
		}
		updated = true
	} else {
		newStatus = contentObj.Status.DeepCopy()
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
	}

	if updated {
		contentClone := contentObj.DeepCopy()
		contentClone.Status = newStatus
		newContent, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshotContents().UpdateStatus(context.TODO(), contentClone, metav1.UpdateOptions{})
		if err != nil {
			return contentObj, newControllerUpdateError(content.Name, err.Error())
		}
		return newContent, nil
	}

	return contentObj, nil
}

// updateContentStatusWithEvent saves new content.Status to API server and emits
// given event on the content. It saves the status and emits the event only when
// the status has actually changed from the version saved in API server.
// Parameters:
//
// * content - content to update
// * eventtype, reason, message - event to send, see EventRecorder.Event()
func (ctrl *csiSnapshotSideCarController) updateGroupSnapshotContentErrorStatusWithEvent(content *crdv1alpha1.VolumeGroupSnapshotContent, eventtype, reason, message string) error {
	klog.V(5).Infof("updateContentStatusWithEvent[%s]", content.Name)

	if content.Status != nil && content.Status.Error != nil && *content.Status.Error.Message == message {
		klog.V(4).Infof("updateContentStatusWithEvent[%s]: the same error %v is already set", content.Name, content.Status.Error)
		return nil
	}

	var patches []utils.PatchOp
	ready := false
	contentStatusError := &crdv1.VolumeSnapshotError{
		Time: &metav1.Time{
			Time: time.Now(),
		},
		Message: &message,
	}
	if content.Status == nil {
		// Initialize status if nil
		patches = append(patches, utils.PatchOp{
			Op:   "replace",
			Path: "/status",
			Value: &crdv1alpha1.VolumeGroupSnapshotContentStatus{
				ReadyToUse: &ready,
				Error:      contentStatusError,
			},
		})
	} else {
		// Patch status if non-nil
		patches = append(patches, utils.PatchOp{
			Op:    "replace",
			Path:  "/status/error",
			Value: contentStatusError,
		})
		patches = append(patches, utils.PatchOp{
			Op:    "replace",
			Path:  "/status/readyToUse",
			Value: &ready,
		})

	}

	newContent, err := utils.PatchVolumeGroupSnapshotContent(content, patches, ctrl.clientset, "status")

	// Emit the event even if the status update fails so that user can see the error
	ctrl.eventRecorder.Event(newContent, eventtype, reason, message)

	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshotContent[%s] error status failed %v", content.Name, err)
		return err
	}

	_, err = ctrl.storeGroupSnapshotContentUpdate(newContent)
	if err != nil {
		klog.V(4).Infof("updating VolumeGroupSnapshotContent[%s] error status: cannot update internal cache %v", content.Name, err)
		return err
	}

	return nil
}

// GetSnapshotNameForVolumeGroupSnapshotContent returns a unique snapshot name for a VolumeGroupSnapshotContent.
func GetSnapshotNameForVolumeGroupSnapshotContent(content *crdv1alpha1.VolumeGroupSnapshotContent) string {
	return fmt.Sprintf("groupsnapshot-%x-%d", sha256.Sum256([]byte(content.UID)), time.Duration(time.Now().UnixNano())/time.Millisecond)
}

// GetSnapshotContentNameForVolumeGroupSnapshotContent returns a unique content name for the
// passed in VolumeGroupSnapshotContent.
func GetSnapshotContentNameForVolumeGroupSnapshotContent(content *crdv1alpha1.VolumeGroupSnapshotContent) string {
	return fmt.Sprintf("groupsnapcontent-%x-%d", sha256.Sum256([]byte(content.UID)), time.Duration(time.Now().UnixNano())/time.Millisecond)
}

func (ctrl *csiSnapshotSideCarController) checkandUpdateGroupSnapshotContentStatus(content *crdv1alpha1.VolumeGroupSnapshotContent) error {
	klog.V(5).Infof("checkandUpdateGroupSnapshotContentStatus[%s] started", content.Name)
	contentObj, err := ctrl.checkandUpdateGroupSnapshotContentStatusOperation(content)
	if err != nil {
		ctrl.updateGroupSnapshotContentErrorStatusWithEvent(contentObj, v1.EventTypeWarning, "GroupSnapshotContentCheckandUpdateFailed", fmt.Sprintf("Failed to check and update group snapshot content: %v", err))
		klog.Errorf("checkandUpdateGroupSnapshotContentStatus [%s]: error occurred %v", content.Name, err)
		return err
	}
	_, updateErr := ctrl.storeGroupSnapshotContentUpdate(contentObj)
	if updateErr != nil {
		// We will get a "group snapshot update" event soon, this is not a big error
		klog.V(4).Infof("checkandUpdateGroupSnapshotContentStatus [%s]: cannot update internal cache: %v", content.Name, updateErr)
	}

	return nil
}

func (ctrl *csiSnapshotSideCarController) checkandUpdateGroupSnapshotContentStatusOperation(content *crdv1alpha1.VolumeGroupSnapshotContent) (*crdv1alpha1.VolumeGroupSnapshotContent, error) {
	var err error
	var creationTime time.Time
	readyToUse := false
	var driverName string
	var groupSnapshotID string
	var snapshotterListCredentials map[string]string

	if content.Spec.Source.VolumeGroupSnapshotHandle != nil {
		klog.V(5).Infof("checkandUpdateGroupSnapshotContentStatusOperation: call GetSnapshotStatus for group snapshot which is pre-bound to content [%s]", content.Name)

		if content.Spec.VolumeGroupSnapshotClassName != nil {
			class, err := ctrl.getGroupSnapshotClass(*content.Spec.VolumeGroupSnapshotClassName)
			if err != nil {
				klog.Errorf("Failed to get group snapshot class %s for group snapshot content %s: %v", *content.Spec.VolumeGroupSnapshotClassName, content.Name, err)
				return content, fmt.Errorf("failed to get group snapshot class %s for group snapshot content %s: %v", *content.Spec.VolumeGroupSnapshotClassName, content.Name, err)
			}

			snapshotterListSecretRef, err := utils.GetSecretReference(utils.SnapshotterListSecretParams, class.Parameters, content.GetObjectMeta().GetName(), nil)
			if err != nil {
				klog.Errorf("Failed to get secret reference for group snapshot content %s: %v", content.Name, err)
				return content, fmt.Errorf("failed to get secret reference for group snapshot content %s: %v", content.Name, err)
			}

			snapshotterListCredentials, err = utils.GetCredentials(ctrl.client, snapshotterListSecretRef)
			if err != nil {
				// Continue with deletion, as the secret may have already been deleted.
				klog.Errorf("Failed to get credentials for group snapshot content %s: %v", content.Name, err)
				return content, fmt.Errorf("failed to get credentials for group snapshot content %s: %v", content.Name, err)
			}
		}

		readyToUse, creationTime, err = ctrl.handler.GetGroupSnapshotStatus(content, snapshotterListCredentials)
		if err != nil {
			klog.Errorf("checkandUpdateGroupSnapshotContentStatusOperation: failed to call get group snapshot status to check whether group snapshot is ready to use %q", err)
			return content, err
		}
		driverName = content.Spec.Driver
		groupSnapshotID = *content.Spec.Source.VolumeGroupSnapshotHandle

		klog.V(5).Infof("checkandUpdateGroupSnapshotContentStatusOperation: driver %s, groupSnapshotId %s, creationTime %v, size %d, readyToUse %t", driverName, groupSnapshotID, creationTime, readyToUse)

		if creationTime.IsZero() {
			creationTime = time.Now()
		}

		updatedContent, err := ctrl.updateGroupSnapshotContentStatus(content, groupSnapshotID, readyToUse, creationTime.UnixNano())
		if err != nil {
			return content, err
		}
		return updatedContent, nil
	}
	return ctrl.createGroupSnapshotWrapper(content)
}
