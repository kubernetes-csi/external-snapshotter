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

package sidecar_controller

import (
	"fmt"
	"strings"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/goroutinemap"
	"k8s.io/kubernetes/pkg/util/goroutinemap/exponentialbackoff"
	"k8s.io/kubernetes/pkg/util/slice"
)

// Design:
//
// This is the sidecar controller that is responsible for creating and deleting a
// snapshot on the storage system through a csi volume driver. It watches
// VolumeSnapshotContent objects which have been either created/deleted by the
// common snapshot controller in the case of dynamic provisioning or by the admin
// in the case of pre-provisioned snapshots.

// The snapshot creation through csi driver should return a snapshot after
// it is created successfully(however, the snapshot might not be ready to use yet
// if there is an uploading phase). The creationTime will be updated accordingly
// on the status of VolumeSnapshotContent.
// After that, the sidecar controller will keep checking the snapshot status
// through csi snapshot calls. When the snapshot is ready to use, the sidecar
// controller set the status "ReadyToUse" to true on the VolumeSnapshotContent object
// to indicate the snapshot is ready to be used to restore a volume.
// If the creation failed for any reason, the Error status is set accordingly.

const controllerUpdateFailMsg = "snapshot controller failed to update"

// syncContent deals with one key off the queue.  It returns false when it's time to quit.
func (ctrl *csiSnapshotSideCarController) syncContent(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("synchronizing VolumeSnapshotContent[%s]", content.Name)

	var err error
	if ctrl.shouldDelete(content) {
		klog.V(4).Infof("VolumeSnapshotContent[%s]: the policy is %s", content.Name, content.Spec.DeletionPolicy)
		if content.Spec.DeletionPolicy == crdv1.VolumeSnapshotContentDelete &&
			content.Status != nil && content.Status.SnapshotHandle != nil {
			// issue a CSI deletion call if the snapshot has not been deleted yet from
			// underlying storage system. Note that the deletion snapshot operation will
			// update content SnapshotHandle to nil upon a successful deletion. At this
			// point, the finalizer on content should NOT be removed to avoid leaking.
			return ctrl.deleteCSISnapshot(content)
		} else {
			// otherwise, either the snapshot has been deleted from the underlying
			// storage system, or the deletion policy is Retain, remove the finalizer
			// if there is one so that API server could delete the object if there is
			// no other finalizer.
			return ctrl.removeContentFinalizer(content)
		}
	} else {
		if content.Spec.Source.VolumeHandle != nil && content.Status == nil {
			klog.V(5).Infof("syncContent: Call CreateSnapshot for content %s", content.Name)
			if err = ctrl.createSnapshot(content); err != nil {
				ctrl.updateContentErrorStatusWithEvent(content, v1.EventTypeWarning, "SnapshotCreationFailed", fmt.Sprintf("Failed to create snapshot with error %v", err))
				return err
			}
		} else {
			// Skip checkandUpdateContentStatus() if ReadyToUse is
			// already true. We don't want to keep calling CreateSnapshot
			// or ListSnapshots CSI methods over and over again for
			// performance reasons.
			if content.Status != nil && content.Status.ReadyToUse != nil && *content.Status.ReadyToUse == true {
				return nil
			}
			if err = ctrl.checkandUpdateContentStatus(content); err != nil {
				ctrl.updateContentErrorStatusWithEvent(content, v1.EventTypeWarning, "SnapshotContentStatusUpdateFailed", fmt.Sprintf("Failed to update snapshot content status with error %v", err))
				return err
			}
		}
	}
	return nil
}

// deleteCSISnapshot starts delete action.
func (ctrl *csiSnapshotSideCarController) deleteCSISnapshot(content *crdv1.VolumeSnapshotContent) error {
	operationName := fmt.Sprintf("delete-%s", content.Name)
	klog.V(5).Infof("schedule to delete snapshot, operation named %s", operationName)
	ctrl.scheduleOperation(operationName, func() error {
		return ctrl.deleteCSISnapshotOperation(content)
	})
	return nil
}

// scheduleOperation starts given asynchronous operation on given snapshot. It
// makes sure there is no running operation with the same operationName
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

func (ctrl *csiSnapshotSideCarController) storeContentUpdate(content interface{}) (bool, error) {
	return utils.StoreObjectUpdate(ctrl.contentStore, content, "content")
}

// createSnapshot starts new asynchronous operation to create snapshot
func (ctrl *csiSnapshotSideCarController) createSnapshot(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("createSnapshot for content [%s]: started", content.Name)
	opName := fmt.Sprintf("create-%s", content.Name)
	ctrl.scheduleOperation(opName, func() error {
		contentObj, err := ctrl.createSnapshotOperation(content)
		if err != nil {
			ctrl.updateContentErrorStatusWithEvent(content, v1.EventTypeWarning, "SnapshotCreationFailed", fmt.Sprintf("Failed to create snapshot: %v", err))
			klog.Errorf("createSnapshot [%s]: error occurred in createSnapshotOperation: %v", opName, err)
			return err
		}

		_, updateErr := ctrl.storeContentUpdate(contentObj)
		if updateErr != nil {
			// We will get an "snapshot update" event soon, this is not a big error
			klog.V(4).Infof("createSnapshot [%s]: cannot update internal content cache: %v", content.Name, updateErr)
		}
		return nil
	})
	return nil
}

func (ctrl *csiSnapshotSideCarController) checkandUpdateContentStatus(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("checkandUpdateContentStatus[%s] started", content.Name)
	opName := fmt.Sprintf("check-%s", content.Name)
	ctrl.scheduleOperation(opName, func() error {
		contentObj, err := ctrl.checkandUpdateContentStatusOperation(content)
		if err != nil {
			ctrl.updateContentErrorStatusWithEvent(content, v1.EventTypeWarning, "SnapshotContentCheckandUpdateFailed", fmt.Sprintf("Failed to check and update snapshot content: %v", err))
			klog.Errorf("checkandUpdateContentStatus [%s]: error occurred %v", content.Name, err)
			return err
		}
		_, updateErr := ctrl.storeContentUpdate(contentObj)
		if updateErr != nil {
			// We will get an "snapshot update" event soon, this is not a big error
			klog.V(4).Infof("checkandUpdateContentStatus [%s]: cannot update internal cache: %v", content.Name, updateErr)
		}

		return nil
	})
	return nil
}

// updateContentStatusWithEvent saves new content.Status to API server and emits
// given event on the content. It saves the status and emits the event only when
// the status has actually changed from the version saved in API server.
// Parameters:
//   content - content to update
//   eventtype, reason, message - event to send, see EventRecorder.Event()
func (ctrl *csiSnapshotSideCarController) updateContentErrorStatusWithEvent(content *crdv1.VolumeSnapshotContent, eventtype, reason, message string) error {
	klog.V(5).Infof("updateContentStatusWithEvent[%s]", content.Name)

	if content.Status != nil && content.Status.Error != nil && *content.Status.Error.Message == message {
		klog.V(4).Infof("updateContentStatusWithEvent[%s]: the same error %v is already set", content.Name, content.Status.Error)
		return nil
	}
	contentClone := content.DeepCopy()
	if contentClone.Status == nil {
		contentClone.Status = &crdv1.VolumeSnapshotContentStatus{}
	}
	contentClone.Status.Error = &crdv1.VolumeSnapshotError{
		Time: &metav1.Time{
			Time: time.Now(),
		},
		Message: &message,
	}
	ready := false
	contentClone.Status.ReadyToUse = &ready
	newContent, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().UpdateStatus(contentClone)

	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status failed %v", content.Name, err)
		return err
	}

	// Emit the event only when the status change happens
	ctrl.eventRecorder.Event(newContent, eventtype, reason, message)

	_, err = ctrl.storeContentUpdate(newContent)
	if err != nil {
		klog.V(4).Infof("updating VolumeSnapshotContent[%s] error status: cannot update internal cache %v", content.Name, err)
		return err
	}

	return nil
}

func (ctrl *csiSnapshotSideCarController) getCSISnapshotInput(content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshotClass, map[string]string, error) {
	className := content.Spec.VolumeSnapshotClassName
	klog.V(5).Infof("getCSISnapshotInput for content [%s]", content.Name)
	var class *crdv1.VolumeSnapshotClass
	var err error
	if className != nil {
		class, err = ctrl.getSnapshotClass(*className)
		if err != nil {
			klog.Errorf("getCSISnapshotInput failed to getClassFromVolumeSnapshot %s", err)
			return nil, nil, err
		}
	} else {
		// If dynamic provisioning, return failure if no snapshot class
		if content.Spec.Source.VolumeHandle != nil {
			klog.Errorf("failed to getCSISnapshotInput %s without a snapshot class", content.Name)
			return nil, nil, fmt.Errorf("failed to take snapshot %s without a snapshot class", content.Name)
		}
		// For pre-provisioned snapshot, snapshot class is not required
		klog.V(5).Infof("getCSISnapshotInput for content [%s]: no VolumeSnapshotClassName provided for pre-provisioned snapshot", content.Name)
	}

	// Resolve snapshotting secret credentials.
	snapshotterCredentials, err := ctrl.GetCredentialsFromAnnotation(content)
	if err != nil {
		return nil, nil, err
	}

	return class, snapshotterCredentials, nil
}

func (ctrl *csiSnapshotSideCarController) checkandUpdateContentStatusOperation(content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshotContent, error) {
	var err error
	var creationTime time.Time
	var size int64
	var readyToUse = false
	var driverName string
	var snapshotID string

	if content.Spec.Source.SnapshotHandle != nil {
		klog.V(5).Infof("checkandUpdateContentStatusOperation: call GetSnapshotStatus for snapshot which is pre-bound to content [%s]", content.Name)
		readyToUse, creationTime, size, err = ctrl.handler.GetSnapshotStatus(content)
		if err != nil {
			klog.Errorf("checkandUpdateContentStatusOperation: failed to call get snapshot status to check whether snapshot is ready to use %q", err)
			return nil, err
		}
		driverName = content.Spec.Driver
		snapshotID = *content.Spec.Source.SnapshotHandle
	} else {
		class, snapshotterCredentials, err := ctrl.getCSISnapshotInput(content)
		if err != nil {
			return nil, fmt.Errorf("failed to get input parameters to create snapshot %s: %q", content.Name, err)
		}

		driverName, snapshotID, creationTime, size, readyToUse, err = ctrl.handler.CreateSnapshot(content, class.Parameters, snapshotterCredentials)
		if err != nil {
			klog.Errorf("checkandUpdateContentStatusOperation: failed to call create snapshot to check whether the snapshot is ready to use %q", err)
			return nil, err
		}
	}
	klog.V(5).Infof("checkandUpdateContentStatusOperation: driver %s, snapshotId %s, creationTime %v, size %d, readyToUse %t", driverName, snapshotID, creationTime, size, readyToUse)

	if creationTime.IsZero() {
		creationTime = time.Now()
	}

	updateContent, err := ctrl.updateSnapshotContentStatus(content, snapshotID, readyToUse, creationTime.UnixNano(), size)
	if err != nil {
		return nil, err
	}
	return updateContent, nil
}

// The function goes through the whole snapshot creation process.
// 1. Trigger the snapshot through csi storage provider.
// 2. Update VolumeSnapshot status with creationtimestamp information
// 3. Create the VolumeSnapshotContent object with the snapshot id information.
// 4. Bind the VolumeSnapshot and VolumeSnapshotContent object
func (ctrl *csiSnapshotSideCarController) createSnapshotOperation(content *crdv1.VolumeSnapshotContent) (*crdv1.VolumeSnapshotContent, error) {
	klog.Infof("createSnapshotOperation: Creating snapshot for content %s through the plugin ...", content.Name)

	// content.Status will be created for the first time after a snapshot
	// is created by the CSI driver. If content.Status is not nil,
	// we should update content status without creating snapshot again.
	if content.Status != nil && content.Status.Error != nil && content.Status.Error.Message != nil && !isControllerUpdateFailError(content.Status.Error) {
		klog.V(4).Infof("error is already set in snapshot, do not retry to create: %s", *content.Status.Error.Message)
		return content, nil
	}

	class, snapshotterCredentials, err := ctrl.getCSISnapshotInput(content)
	if err != nil {
		return nil, fmt.Errorf("failed to get input parameters to create snapshot for content %s: %q", content.Name, err)
	}

	driverName, snapshotID, creationTime, size, readyToUse, err := ctrl.handler.CreateSnapshot(content, class.Parameters, snapshotterCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to take snapshot of the volume, %s: %q", *content.Spec.Source.VolumeHandle, err)
	}
	if driverName != class.Driver {
		return nil, fmt.Errorf("failed to take snapshot of the volume, %s: driver name %s returned from the driver is different from driver %s in snapshot class", *content.Spec.Source.VolumeHandle, driverName, class.Driver)
	}

	klog.V(5).Infof("Created snapshot: driver %s, snapshotId %s, creationTime %v, size %d, readyToUse %t", driverName, snapshotID, creationTime, size, readyToUse)

	timestamp := creationTime.UnixNano()
	newContent, err := ctrl.updateSnapshotContentStatus(content, snapshotID, readyToUse, timestamp, size)
	if err != nil {
		strerr := fmt.Sprintf("error updating volume snapshot content status for snapshot %s: %v.", content.Name, err)
		klog.Error(strerr)
	} else {
		content = newContent
	}

	// Update content in the cache store
	_, err = ctrl.storeContentUpdate(content)
	if err != nil {
		klog.Errorf("failed to update content store %v", err)
	}

	return content, nil
}

// Delete a snapshot: Ask the backend to remove the snapshot device
func (ctrl *csiSnapshotSideCarController) deleteCSISnapshotOperation(content *crdv1.VolumeSnapshotContent) error {
	klog.V(5).Infof("deleteCSISnapshotOperation [%s] started", content.Name)

	_, snapshotterCredentials, err := ctrl.getCSISnapshotInput(content)
	if err != nil {
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotDeleteError", "Failed to get snapshot class or credentials")
		return fmt.Errorf("failed to get input parameters to delete snapshot for content %s: %q", content.Name, err)
	}

	err = ctrl.handler.DeleteSnapshot(content, snapshotterCredentials)
	if err != nil {
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotDeleteError", "Failed to delete snapshot")
		return fmt.Errorf("failed to delete snapshot %#v, err: %v", content.Name, err)
	}
	// the snapshot has been deleted from the underlying storage system, update
	// content status to remove snapshot handle etc.
	newContent, err := ctrl.clearVolumeContentStatus(content.Name)
	if err != nil {
		ctrl.eventRecorder.Event(content, v1.EventTypeWarning, "SnapshotDeleteError", "Failed to clear content status")
		return err
	}
	// update local cache
	ctrl.updateContentInCacheStore(newContent)
	return nil
}

// clearVolumeContentStatus resets all fields to nil related to a snapshot in
// content.Status. On success, the latest version of the content object will be
// returned.
func (ctrl *csiSnapshotSideCarController) clearVolumeContentStatus(
	contentName string) (*crdv1.VolumeSnapshotContent, error) {
	klog.V(5).Infof("cleanVolumeSnapshotStatus content [%s]", contentName)
	// get the latest version from API server
	content, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Get(contentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot content %s from api server: %v", contentName, err)
	}
	if content.Status != nil {
		content.Status.SnapshotHandle = nil
		content.Status.ReadyToUse = nil
		content.Status.CreationTime = nil
		content.Status.RestoreSize = nil
	}
	newContent, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().UpdateStatus(content)
	if err != nil {
		return nil, newControllerUpdateError(contentName, err.Error())
	}
	return newContent, nil
}

func (ctrl *csiSnapshotSideCarController) updateSnapshotContentStatus(
	content *crdv1.VolumeSnapshotContent,
	snapshotHandle string,
	readyToUse bool,
	createdAt int64,
	size int64) (*crdv1.VolumeSnapshotContent, error) {
	klog.V(5).Infof("updateSnapshotContentStatus: updating VolumeSnapshotContent [%s], snapshotHandle %s, readyToUse %v, createdAt %v, size %d", content.Name, snapshotHandle, readyToUse, createdAt, size)

	contentObj, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Get(content.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error get snapshot content %s from api server: %v", content.Name, err)
	}

	var newStatus *crdv1.VolumeSnapshotContentStatus
	updated := false
	if contentObj.Status == nil {
		newStatus = &crdv1.VolumeSnapshotContentStatus{
			SnapshotHandle: &snapshotHandle,
			ReadyToUse:     &readyToUse,
			CreationTime:   &createdAt,
			RestoreSize:    &size,
		}
		updated = true
	} else {
		newStatus = contentObj.Status.DeepCopy()
		if newStatus.SnapshotHandle == nil {
			newStatus.SnapshotHandle = &snapshotHandle
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
		if newStatus.RestoreSize == nil {
			newStatus.RestoreSize = &size
			updated = true
		}
	}

	if updated {
		contentClone := contentObj.DeepCopy()
		contentClone.Status = newStatus
		newContent, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().UpdateStatus(contentClone)
		if err != nil {
			return nil, newControllerUpdateError(content.Name, err.Error())
		}
		return newContent, nil
	}

	return contentObj, nil
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

func (ctrl *csiSnapshotSideCarController) GetCredentialsFromAnnotation(content *crdv1.VolumeSnapshotContent) (map[string]string, error) {
	// get secrets if VolumeSnapshotClass specifies it
	var snapshotterCredentials map[string]string
	var err error

	// Check if annotation exists
	if metav1.HasAnnotation(content.ObjectMeta, utils.AnnDeletionSecretRefName) && metav1.HasAnnotation(content.ObjectMeta, utils.AnnDeletionSecretRefNamespace) {
		annDeletionSecretName := content.Annotations[utils.AnnDeletionSecretRefName]
		annDeletionSecretNamespace := content.Annotations[utils.AnnDeletionSecretRefNamespace]

		snapshotterSecretRef := &v1.SecretReference{}

		if annDeletionSecretName == "" || annDeletionSecretNamespace == "" {
			return nil, fmt.Errorf("cannot retrieve secrets for snapshot content %#v, err: secret name or namespace not specified", content.Name)
		}

		snapshotterSecretRef.Name = annDeletionSecretName
		snapshotterSecretRef.Namespace = annDeletionSecretNamespace

		snapshotterCredentials, err = utils.GetCredentials(ctrl.client, snapshotterSecretRef)
		if err != nil {
			// Continue with deletion, as the secret may have already been deleted.
			klog.Errorf("Failed to get credentials for snapshot %s: %s", content.Name, err.Error())
			return nil, fmt.Errorf("cannot get credentials for snapshot content %#v", content.Name)
		}
	}

	return snapshotterCredentials, nil
}

// removeContentFinalizer removes the VolumeSnapshotContentFinalizer from a
// content if there exists one.
func (ctrl csiSnapshotSideCarController) removeContentFinalizer(content *crdv1.VolumeSnapshotContent) error {
	if !slice.ContainsString(content.ObjectMeta.Finalizers, utils.VolumeSnapshotContentFinalizer, nil) {
		// the finalizer does not exit, return directly
		return nil
	}
	contentClone := content.DeepCopy()
	contentClone.ObjectMeta.Finalizers = slice.RemoveString(contentClone.ObjectMeta.Finalizers, utils.VolumeSnapshotContentFinalizer, nil)

	_, err := ctrl.clientset.SnapshotV1beta1().VolumeSnapshotContents().Update(contentClone)
	if err != nil {
		return newControllerUpdateError(content.Name, err.Error())
	}

	klog.V(5).Infof("Removed protection finalizer from volume snapshot content %s", content.Name)
	_, err = ctrl.storeContentUpdate(contentClone)
	if err != nil {
		klog.Errorf("failed to update content store %v", err)
	}
	return nil
}

// shouldDelete checks if content object should be deleted
// if DeletionTimestamp is set on the content
func (ctrl *csiSnapshotSideCarController) shouldDelete(content *crdv1.VolumeSnapshotContent) bool {
	klog.V(5).Infof("Check if VolumeSnapshotContent[%s] should be deleted.", content.Name)

	if content.ObjectMeta.DeletionTimestamp == nil {
		return false
	}
	// 1) shouldDelete returns true if a content is not bound
	// (VolumeSnapshotRef.UID == "") for pre-provisioned snapshot
	if content.Spec.Source.SnapshotHandle != nil && content.Spec.VolumeSnapshotRef.UID == "" {
		return true
	}
	// 2) shouldDelete returns true if AnnVolumeSnapshotBeingDeleted annotation is set
	if metav1.HasAnnotation(content.ObjectMeta, utils.AnnVolumeSnapshotBeingDeleted) {
		return true
	}
	return false
}
