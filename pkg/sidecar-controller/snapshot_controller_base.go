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
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v3/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned"
	storageinformers "github.com/kubernetes-csi/external-snapshotter/client/v3/informers/externalversions/volumesnapshot/v1"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/client/v3/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v3/pkg/snapshotter"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	klog "k8s.io/klog/v2"
)

type csiSnapshotSideCarController struct {
	clientset           clientset.Interface
	client              kubernetes.Interface
	driverName          string
	eventRecorder       record.EventRecorder
	contentQueue        workqueue.RateLimitingInterface
	extraCreateMetadata bool

	contentLister       storagelisters.VolumeSnapshotContentLister
	contentListerSynced cache.InformerSynced
	classLister         storagelisters.VolumeSnapshotClassLister
	classListerSynced   cache.InformerSynced

	contentStore cache.Store

	handler Handler

	resyncPeriod time.Duration
}

// NewCSISnapshotSideCarController returns a new *csiSnapshotSideCarController
func NewCSISnapshotSideCarController(
	clientset clientset.Interface,
	client kubernetes.Interface,
	driverName string,
	volumeSnapshotContentInformer storageinformers.VolumeSnapshotContentInformer,
	volumeSnapshotClassInformer storageinformers.VolumeSnapshotClassInformer,
	snapshotter snapshotter.Snapshotter,
	timeout time.Duration,
	resyncPeriod time.Duration,
	snapshotNamePrefix string,
	snapshotNameUUIDLength int,
	extraCreateMetadata bool,
) *csiSnapshotSideCarController {
	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(klog.Infof)
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: client.CoreV1().Events(v1.NamespaceAll)})
	var eventRecorder record.EventRecorder
	eventRecorder = broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: fmt.Sprintf("csi-snapshotter %s", driverName)})

	ctrl := &csiSnapshotSideCarController{
		clientset:           clientset,
		client:              client,
		driverName:          driverName,
		eventRecorder:       eventRecorder,
		handler:             NewCSIHandler(snapshotter, timeout, snapshotNamePrefix, snapshotNameUUIDLength),
		resyncPeriod:        resyncPeriod,
		contentStore:        cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		contentQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "csi-snapshotter-content"),
		extraCreateMetadata: extraCreateMetadata,
	}

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

func (ctrl *csiSnapshotSideCarController) Run(workers int, stopCh <-chan struct{}) {
	defer ctrl.contentQueue.ShutDown()

	klog.Infof("Starting CSI snapshotter")
	defer klog.Infof("Shutting CSI snapshotter")

	if !cache.WaitForCacheSync(stopCh, ctrl.contentListerSynced, ctrl.classListerSynced) {
		klog.Errorf("Cannot sync caches")
		return
	}

	ctrl.initializeCaches(ctrl.contentLister)

	for i := 0; i < workers; i++ {
		go wait.Until(ctrl.contentWorker, 0, stopCh)
	}

	<-stopCh
}

// enqueueContentWork adds snapshot content to given work queue.
func (ctrl *csiSnapshotSideCarController) enqueueContentWork(obj interface{}) {
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

// contentWorker processes items from contentQueue. It must run only once,
// syncContent is not assured to be reentrant.
func (ctrl *csiSnapshotSideCarController) contentWorker() {
	for ctrl.processNextItem() {
	}
}

func (ctrl *csiSnapshotSideCarController) processNextItem() bool {
	keyObj, quit := ctrl.contentQueue.Get()
	if quit {
		return false
	}
	defer ctrl.contentQueue.Done(keyObj)

	if err := ctrl.syncContentByKey(keyObj.(string)); err != nil {
		// Rather than wait for a full resync, re-add the key to the
		// queue to be processed.
		ctrl.contentQueue.AddRateLimited(keyObj)
		klog.V(4).Infof("Failed to sync content %q, will retry again: %v", keyObj.(string), err)
		return true
	}

	// Finally, if no error occurs we Forget this item so it does not
	// get queued again until another change happens.
	ctrl.contentQueue.Forget(keyObj)
	return true
}

func (ctrl *csiSnapshotSideCarController) syncContentByKey(key string) error {
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
		if ctrl.isDriverMatch(content) {
			err = ctrl.updateContentInInformerCache(content)
		}
		if err != nil {
			// If error occurs we add this item back to the queue
			return err
		}
		return nil
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
	ctrl.deleteContentInCacheStore(content)
	return nil
}

// verify whether the driver specified in VolumeSnapshotContent matches the controller's driver name
func (ctrl *csiSnapshotSideCarController) isDriverMatch(content *crdv1.VolumeSnapshotContent) bool {
	if content.Spec.Source.VolumeHandle == nil && content.Spec.Source.SnapshotHandle == nil {
		// Skip this snapshot content if it does not have a valid source
		return false
	}
	if content.Spec.Driver != ctrl.driverName {
		// Skip this snapshot content if the driver does not match
		return false
	}
	snapshotClassName := content.Spec.VolumeSnapshotClassName
	if snapshotClassName != nil {
		if snapshotClass, err := ctrl.classLister.Get(*snapshotClassName); err == nil {
			if snapshotClass.Driver != ctrl.driverName {
				return false
			}
		}
	}
	return true
}

// updateContentInInformerCache runs in worker thread and handles "content added",
// "content updated" and "periodic sync" events.
func (ctrl *csiSnapshotSideCarController) updateContentInInformerCache(content *crdv1.VolumeSnapshotContent) error {
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

// deleteContent runs in worker thread and handles "content deleted" event.
func (ctrl *csiSnapshotSideCarController) deleteContentInCacheStore(content *crdv1.VolumeSnapshotContent) {
	_ = ctrl.contentStore.Delete(content)
	klog.V(4).Infof("content %q deleted", content.Name)
}

// initializeCaches fills all controller caches with initial data from etcd in
// order to have the caches already filled when first addSnapshot/addContent to
// perform initial synchronization of the controller.
func (ctrl *csiSnapshotSideCarController) initializeCaches(contentLister storagelisters.VolumeSnapshotContentLister) {
	contentList, err := contentLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("CSISnapshotController can't initialize caches: %v", err)
		return
	}
	for _, content := range contentList {
		if ctrl.isDriverMatch(content) {
			contentClone := content.DeepCopy()
			if _, err = ctrl.storeContentUpdate(contentClone); err != nil {
				klog.Errorf("error updating volume snapshot content cache: %v", err)
			}
		}
	}

	klog.V(4).Infof("controller initialized")
}
