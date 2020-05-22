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
	"fmt"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	clientset "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/clientset/versioned"
	storageinformers "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/informers/externalversions/volumesnapshot/v1beta1"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/listers/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/v2/pkg/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

type csiSnapshotCommonController struct {
	clientset     clientset.Interface
	client        kubernetes.Interface
	eventRecorder record.EventRecorder
	snapshotQueue workqueue.RateLimitingInterface
	contentQueue  workqueue.RateLimitingInterface

	snapshotLister       storagelisters.VolumeSnapshotLister
	snapshotListerSynced cache.InformerSynced
	contentLister        storagelisters.VolumeSnapshotContentLister
	contentListerSynced  cache.InformerSynced
	classLister          storagelisters.VolumeSnapshotClassLister
	classListerSynced    cache.InformerSynced
	pvcLister            corelisters.PersistentVolumeClaimLister
	pvcListerSynced      cache.InformerSynced

	snapshotStore cache.Store
	contentStore  cache.Store

	resyncPeriod time.Duration
}

// NewCSISnapshotController returns a new *csiSnapshotCommonController
func NewCSISnapshotCommonController(
	clientset clientset.Interface,
	client kubernetes.Interface,
	volumeSnapshotInformer storageinformers.VolumeSnapshotInformer,
	volumeSnapshotContentInformer storageinformers.VolumeSnapshotContentInformer,
	volumeSnapshotClassInformer storageinformers.VolumeSnapshotClassInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	resyncPeriod time.Duration,
) *csiSnapshotCommonController {
	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(klog.Infof)
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: client.CoreV1().Events(v1.NamespaceAll)})
	var eventRecorder record.EventRecorder
	eventRecorder = broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: fmt.Sprintf("snapshot-controller")})

	ctrl := &csiSnapshotCommonController{
		clientset:     clientset,
		client:        client,
		eventRecorder: eventRecorder,
		resyncPeriod:  resyncPeriod,
		snapshotStore: cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		contentStore:  cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		snapshotQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "snapshot-controller-snapshot"),
		contentQueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "snapshot-controller-content"),
	}

	ctrl.pvcLister = pvcInformer.Lister()
	ctrl.pvcListerSynced = pvcInformer.Informer().HasSynced

	volumeSnapshotInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { ctrl.enqueueSnapshotWork(obj) },
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueSnapshotWork(newObj) },
			DeleteFunc: func(obj interface{}) { ctrl.enqueueSnapshotWork(obj) },
		},
		ctrl.resyncPeriod,
	)
	ctrl.snapshotLister = volumeSnapshotInformer.Lister()
	ctrl.snapshotListerSynced = volumeSnapshotInformer.Informer().HasSynced

	volumeSnapshotContentInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { ctrl.enqueueContentWork(obj) },
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.enqueueContentWork(newObj) },
			DeleteFunc: func(obj interface{}) { ctrl.enqueueContentWork(obj) },
		},
		ctrl.resyncPeriod,
	)
	ctrl.contentLister = volumeSnapshotContentInformer.Lister()
	ctrl.contentListerSynced = volumeSnapshotContentInformer.Informer().HasSynced

	ctrl.classLister = volumeSnapshotClassInformer.Lister()
	ctrl.classListerSynced = volumeSnapshotClassInformer.Informer().HasSynced

	return ctrl
}

func (ctrl *csiSnapshotCommonController) Run(workers int, stopCh <-chan struct{}) {
	defer ctrl.snapshotQueue.ShutDown()
	defer ctrl.contentQueue.ShutDown()

	klog.Infof("Starting snapshot controller")
	defer klog.Infof("Shutting snapshot controller")

	if !cache.WaitForCacheSync(stopCh, ctrl.snapshotListerSynced, ctrl.contentListerSynced, ctrl.classListerSynced, ctrl.pvcListerSynced) {
		klog.Errorf("Cannot sync caches")
		return
	}

	ctrl.initializeCaches(ctrl.snapshotLister, ctrl.contentLister)

	for i := 0; i < workers; i++ {
		go wait.Until(ctrl.snapshotWorker, 0, stopCh)
		go wait.Until(ctrl.contentWorker, 0, stopCh)
	}

	<-stopCh
}

// enqueueSnapshotWork adds snapshot to given work queue.
func (ctrl *csiSnapshotCommonController) enqueueSnapshotWork(obj interface{}) {
	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if snapshot, ok := obj.(*crdv1.VolumeSnapshot); ok {
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(snapshot)
		if err != nil {
			klog.Errorf("failed to get key from object: %v, %v", err, snapshot)
			return
		}
		klog.V(5).Infof("enqueued %q for sync", objName)
		ctrl.snapshotQueue.Add(objName)
	}
}

// enqueueContentWork adds snapshot content to given work queue.
func (ctrl *csiSnapshotCommonController) enqueueContentWork(obj interface{}) {
	// Beware of "xxx deleted" events
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}
	if content, ok := obj.(*crdv1.VolumeSnapshotContent); ok {
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(content)
		if err != nil {
			klog.Errorf("failed to get key from object: %v, %v", err, content)
			return
		}
		klog.V(5).Infof("enqueued %q for sync", objName)
		ctrl.contentQueue.Add(objName)
	}
}

// snapshotWorker is the main worker for VolumeSnapshots.
func (ctrl *csiSnapshotCommonController) snapshotWorker() {
	keyObj, quit := ctrl.snapshotQueue.Get()
	if quit {
		return
	}
	defer ctrl.snapshotQueue.Done(keyObj)

	if err := ctrl.syncSnapshotByKey(keyObj.(string)); err != nil {
		// Rather than wait for a full resync, re-add the key to the
		// queue to be processed.
		ctrl.snapshotQueue.AddRateLimited(keyObj)
		klog.V(4).Infof("Failed to sync snapshot %q, will retry again: %v", keyObj.(string), err)
	} else {
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		ctrl.snapshotQueue.Forget(keyObj)
	}
}

// syncSnapshotByKey processes a VolumeSnapshot request.
func (ctrl *csiSnapshotCommonController) syncSnapshotByKey(key string) error {
	klog.V(5).Infof("syncSnapshotByKey[%s]", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	klog.V(5).Infof("snapshotWorker: snapshot namespace [%s] name [%s]", namespace, name)
	if err != nil {
		klog.Errorf("error getting namespace & name of snapshot %q to get snapshot from informer: %v", key, err)
		return nil
	}
	snapshot, err := ctrl.snapshotLister.VolumeSnapshots(namespace).Get(name)
	if err == nil {
		// The volume snapshot still exists in informer cache, the event must have
		// been add/update/sync
		newSnapshot, err := ctrl.checkAndUpdateSnapshotClass(snapshot)
		if err == nil || (newSnapshot.ObjectMeta.DeletionTimestamp != nil && errors.IsNotFound(err)) {
			// If the VolumeSnapshotClass is not found, we still need to process an update
			// so that syncSnapshot can delete the snapshot, should it still exist in the
			// cluster after it's been removed from the informer cache
			if newSnapshot.ObjectMeta.DeletionTimestamp != nil && errors.IsNotFound(err) {
				klog.V(5).Infof("Snapshot %q is being deleted. SnapshotClass has already been removed", key)
			}
			klog.V(5).Infof("Updating snapshot %q", key)
			return ctrl.updateSnapshot(newSnapshot)
		}
		return err
	}
	if err != nil && !errors.IsNotFound(err) {
		klog.V(2).Infof("error getting snapshot %q from informer: %v", key, err)
		return err
	}
	// The snapshot is not in informer cache, the event must have been "delete"
	vsObj, found, err := ctrl.snapshotStore.GetByKey(key)
	if err != nil {
		klog.V(2).Infof("error getting snapshot %q from cache: %v", key, err)
		return nil
	}
	if !found {
		// The controller has already processed the delete event and
		// deleted the snapshot from its cache
		klog.V(2).Infof("deletion of snapshot %q was already processed", key)
		return nil
	}
	snapshot, ok := vsObj.(*crdv1.VolumeSnapshot)
	if !ok {
		klog.Errorf("expected vs, got %+v", vsObj)
		return nil
	}

	klog.V(5).Infof("deleting snapshot %q", key)
	ctrl.deleteSnapshot(snapshot)

	return nil
}

// contentWorker is the main worker for VolumeSnapshotContent.
func (ctrl *csiSnapshotCommonController) contentWorker() {
	keyObj, quit := ctrl.contentQueue.Get()
	if quit {
		return
	}
	defer ctrl.contentQueue.Done(keyObj)

	if err := ctrl.syncContentByKey(keyObj.(string)); err != nil {
		// Rather than wait for a full resync, re-add the key to the
		// queue to be processed.
		ctrl.contentQueue.AddRateLimited(keyObj)
		klog.V(4).Infof("Failed to sync content %q, will retry again: %v", keyObj.(string), err)
	} else {
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		ctrl.contentQueue.Forget(keyObj)
	}
}

// syncContentByKey processes a VolumeSnapshotContent request.
func (ctrl *csiSnapshotCommonController) syncContentByKey(key string) error {
	klog.V(5).Infof("syncContentByKey[%s]", key)

	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.V(4).Infof("error getting name of snapshotContent %q to get snapshotContent from informer: %v", key, err)
		return nil
	}
	content, err := ctrl.contentLister.Get(name)
	// The content still exists in informer cache, the event must have
	// been add/update/sync
	if err == nil {
		// If error occurs we add this item back to the queue
		return ctrl.updateContent(content)
	}
	if !errors.IsNotFound(err) {
		klog.V(2).Infof("error getting content %q from informer: %v", key, err)
		return nil
	}

	// The content is not in informer cache, the event must have been
	// "delete"
	contentObj, found, err := ctrl.contentStore.GetByKey(key)
	if err != nil {
		klog.V(2).Infof("error getting content %q from cache: %v", key, err)
		return nil
	}
	if !found {
		// The controller has already processed the delete event and
		// deleted the content from its cache
		klog.V(2).Infof("deletion of content %q was already processed", key)
		return nil
	}
	content, ok := contentObj.(*crdv1.VolumeSnapshotContent)
	if !ok {
		klog.Errorf("expected content, got %+v", content)
		return nil
	}
	ctrl.deleteContent(content)
	return nil
}

// checkAndUpdateSnapshotClass gets the VolumeSnapshotClass from VolumeSnapshot. If it is not set,
// gets it from default VolumeSnapshotClass and sets it.
func (ctrl *csiSnapshotCommonController) checkAndUpdateSnapshotClass(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	className := snapshot.Spec.VolumeSnapshotClassName
	var class *crdv1.VolumeSnapshotClass
	var err error
	newSnapshot := snapshot
	if className != nil {
		klog.V(5).Infof("checkAndUpdateSnapshotClass [%s]: VolumeSnapshotClassName [%s]", snapshot.Name, *className)
		class, err = ctrl.getSnapshotClass(*className)
		if err != nil {
			klog.Errorf("checkAndUpdateSnapshotClass failed to getSnapshotClass %v", err)
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "GetSnapshotClassFailed", fmt.Sprintf("Failed to get snapshot class with error %v", err))
			// we need to return the original snapshot even if the class isn't found, as it may need to be deleted
			return newSnapshot, err
		}
	} else {
		klog.V(5).Infof("checkAndUpdateSnapshotClass [%s]: SetDefaultSnapshotClass", snapshot.Name)
		class, newSnapshot, err = ctrl.SetDefaultSnapshotClass(snapshot)
		if err != nil {
			klog.Errorf("checkAndUpdateSnapshotClass failed to setDefaultClass %v", err)
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "SetDefaultSnapshotClassFailed", fmt.Sprintf("Failed to set default snapshot class with error %v", err))
			return nil, err
		}
	}

	// For pre-provisioned snapshots, we may not have snapshot class
	if class != nil {
		klog.V(5).Infof("VolumeSnapshotClass [%s] Driver [%s]", class.Name, class.Driver)
	}
	return newSnapshot, nil
}

// updateSnapshot runs in worker thread and handles "snapshot added",
// "snapshot updated" and "periodic sync" events.
func (ctrl *csiSnapshotCommonController) updateSnapshot(snapshot *crdv1.VolumeSnapshot) error {
	// Store the new snapshot version in the cache and do not process it if this is
	// an old version.
	klog.V(5).Infof("updateSnapshot %q", utils.SnapshotKey(snapshot))
	newSnapshot, err := ctrl.storeSnapshotUpdate(snapshot)
	if err != nil {
		klog.Errorf("%v", err)
	}
	if !newSnapshot {
		return nil
	}
	err = ctrl.syncSnapshot(snapshot)
	if err != nil {
		if errors.IsConflict(err) {
			// Version conflict error happens quite often and the controller
			// recovers from it easily.
			klog.V(3).Infof("could not sync snapshot %q: %+v", utils.SnapshotKey(snapshot), err)
		} else {
			klog.Errorf("could not sync snapshot %q: %+v", utils.SnapshotKey(snapshot), err)
		}
		return err
	}
	return nil
}

// updateContent runs in worker thread and handles "content added",
// "content updated" and "periodic sync" events.
func (ctrl *csiSnapshotCommonController) updateContent(content *crdv1.VolumeSnapshotContent) error {
	// Store the new content version in the cache and do not process it if this is
	// an old version.
	new, err := ctrl.storeContentUpdate(content)
	if err != nil {
		klog.Errorf("%v", err)
	}
	if !new {
		return nil
	}
	err = ctrl.syncContent(content)
	if err != nil {
		if errors.IsConflict(err) {
			// Version conflict error happens quite often and the controller
			// recovers from it easily.
			klog.V(3).Infof("could not sync content %q: %+v", content.Name, err)
		} else {
			klog.Errorf("could not sync content %q: %+v", content.Name, err)
		}
		return err
	}
	return nil
}

// deleteSnapshot runs in worker thread and handles "snapshot deleted" event.
func (ctrl *csiSnapshotCommonController) deleteSnapshot(snapshot *crdv1.VolumeSnapshot) {
	_ = ctrl.snapshotStore.Delete(snapshot)
	klog.V(4).Infof("snapshot %q deleted", utils.SnapshotKey(snapshot))

	snapshotContentName := ""
	if snapshot.Status != nil && snapshot.Status.BoundVolumeSnapshotContentName != nil {
		snapshotContentName = *snapshot.Status.BoundVolumeSnapshotContentName
	}
	if snapshotContentName == "" {
		klog.V(5).Infof("deleteSnapshot[%q]: content not bound", utils.SnapshotKey(snapshot))
		return
	}
	// sync the content when its snapshot is deleted.  Explicitly sync'ing the
	// content here in response to snapshot deletion prevents the content from
	// waiting until the next sync period for its Release.
	klog.V(5).Infof("deleteSnapshot[%q]: scheduling sync of content %s", utils.SnapshotKey(snapshot), snapshotContentName)
	ctrl.contentQueue.Add(snapshotContentName)
}

// deleteContent runs in worker thread and handles "content deleted" event.
func (ctrl *csiSnapshotCommonController) deleteContent(content *crdv1.VolumeSnapshotContent) {
	_ = ctrl.contentStore.Delete(content)
	klog.V(4).Infof("content %q deleted", content.Name)

	snapshotName := utils.SnapshotRefKey(&content.Spec.VolumeSnapshotRef)
	if snapshotName == "" {
		klog.V(5).Infof("deleteContent[%q]: content not bound", content.Name)
		return
	}
	// sync the snapshot when its content is deleted.  Explicitly sync'ing the
	// snapshot here in response to content deletion prevents the snapshot from
	// waiting until the next sync period for its Release.
	klog.V(5).Infof("deleteContent[%q]: scheduling sync of snapshot %s", content.Name, snapshotName)
	ctrl.snapshotQueue.Add(snapshotName)
}

// initializeCaches fills all controller caches with initial data from etcd in
// order to have the caches already filled when first addSnapshot/addContent to
// perform initial synchronization of the controller.
func (ctrl *csiSnapshotCommonController) initializeCaches(snapshotLister storagelisters.VolumeSnapshotLister, contentLister storagelisters.VolumeSnapshotContentLister) {
	snapshotList, err := snapshotLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("CSISnapshotController can't initialize caches: %v", err)
		return
	}
	for _, snapshot := range snapshotList {
		snapshotClone := snapshot.DeepCopy()
		if _, err = ctrl.storeSnapshotUpdate(snapshotClone); err != nil {
			klog.Errorf("error updating volume snapshot cache: %v", err)
		}
	}

	contentList, err := contentLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("CSISnapshotController can't initialize caches: %v", err)
		return
	}
	for _, content := range contentList {
		contentClone := content.DeepCopy()
		if _, err = ctrl.storeContentUpdate(contentClone); err != nil {
			klog.Errorf("error updating volume snapshot content cache: %v", err)
		}
	}

	klog.V(4).Infof("controller initialized")
}
