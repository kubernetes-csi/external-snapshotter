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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	klog "k8s.io/klog/v2"

	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"
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

	/*
		TODO: Check if a new group snapshot should be created by calling CreateGroupSnapshot
	*/

	/*
		TODO: Check and update group snapshot content status
	*/
	return nil
}
