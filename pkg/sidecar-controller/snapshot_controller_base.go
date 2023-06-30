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
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/group_snapshotter"
	"time"

	v1 "k8s.io/api/core/v1"
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

	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumegroupsnapshot/v1alpha1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	groupsnapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v6/informers/externalversions/volumegroupsnapshot/v1alpha1"
	snapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v6/informers/externalversions/volumesnapshot/v1"
	groupsnapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v6/listers/volumegroupsnapshot/v1alpha1"
	snapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v6/listers/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/snapshotter"
)

type csiSnapshotSideCarController struct {
	clientset           clientset.Interface
	client              kubernetes.Interface
	driverName          string
	eventRecorder       record.EventRecorder
	contentQueue        workqueue.RateLimitingInterface
	extraCreateMetadata bool

	contentLister       snapshotlisters.VolumeSnapshotContentLister
	contentListerSynced cache.InformerSynced
	classLister         snapshotlisters.VolumeSnapshotClassLister
	classListerSynced   cache.InformerSynced

	contentStore cache.Store

	handler Handler

	resyncPeriod time.Duration

	enableVolumeGroupSnapshots       bool
	groupSnapshotContentQueue        workqueue.RateLimitingInterface
	groupSnapshotContentLister       groupsnapshotlisters.VolumeGroupSnapshotContentLister
	groupSnapshotContentListerSynced cache.InformerSynced
	groupSnapshotClassLister         groupsnapshotlisters.VolumeGroupSnapshotClassLister
	groupSnapshotClassListerSynced   cache.InformerSynced
	groupSnapshotContentStore        cache.Store
}

// NewCSISnapshotSideCarController returns a new *csiSnapshotSideCarController
func NewCSISnapshotSideCarController(
	clientset clientset.Interface,
	client kubernetes.Interface,
	driverName string,
	volumeSnapshotContentInformer snapshotinformers.VolumeSnapshotContentInformer,
	volumeSnapshotClassInformer snapshotinformers.VolumeSnapshotClassInformer,
	snapshotter snapshotter.Snapshotter,
	groupSnapshotter group_snapshotter.GroupSnapshotter,
	timeout time.Duration,
	resyncPeriod time.Duration,
	snapshotNamePrefix string,
	snapshotNameUUIDLength int,
	groupSnapshotNamePrefix string,
	groupSnapshotNameUUIDLength int,
	extraCreateMetadata bool,
	contentRateLimiter workqueue.RateLimiter,
	enableVolumeGroupSnapshots bool,
	volumeGroupSnapshotContentInformer groupsnapshotinformers.VolumeGroupSnapshotContentInformer,
	volumeGroupSnapshotClassInformer groupsnapshotinformers.VolumeGroupSnapshotClassInformer,
	groupSnapshotContentRateLimiter workqueue.RateLimiter,
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
		handler:             NewCSIHandler(snapshotter, groupSnapshotter, timeout, snapshotNamePrefix, snapshotNameUUIDLength, groupSnapshotNamePrefix, groupSnapshotNameUUIDLength),
		resyncPeriod:        resyncPeriod,
		contentStore:        cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		contentQueue:        workqueue.NewNamedRateLimitingQueue(contentRateLimiter, "csi-snapshotter-content"),
		extraCreateMetadata: extraCreateMetadata,
	}

	volumeSnapshotContentInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) { ctrl.enqueueContentWork(obj) },
			UpdateFunc: func(oldObj, newObj interface{}) {
				// Considering the object is modified more than once during the workflow we are not relying on the
				// "AnnVolumeSnapshotBeingCreated" annotation. Instead we will just check if newobj status has error
				// and avoid the immediate re-queue. This allows the retry to happen with exponential backoff.
				newSnapContent := newObj.(*crdv1.VolumeSnapshotContent)
				if newSnapContent.Status != nil && newSnapContent.Status.Error != nil {
					return
				}
				ctrl.enqueueContentWork(newObj)
			},
			DeleteFunc: func(obj interface{}) { ctrl.enqueueContentWork(obj) },
		},
		ctrl.resyncPeriod,
	)
	ctrl.contentLister = volumeSnapshotContentInformer.Lister()
	ctrl.contentListerSynced = volumeSnapshotContentInformer.Informer().HasSynced

	ctrl.classLister = volumeSnapshotClassInformer.Lister()
	ctrl.classListerSynced = volumeSnapshotClassInformer.Informer().HasSynced

	ctrl.enableVolumeGroupSnapshots = enableVolumeGroupSnapshots
	if enableVolumeGroupSnapshots {
		ctrl.groupSnapshotContentStore = cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
		ctrl.groupSnapshotContentQueue = workqueue.NewNamedRateLimitingQueue(groupSnapshotContentRateLimiter, "csi-snapshotter-groupsnapshotcontent")

		volumeGroupSnapshotContentInformer.Informer().AddEventHandlerWithResyncPeriod(
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) { ctrl.enqueueGroupSnapshotContentWork(obj) },
				UpdateFunc: func(oldObj, newObj interface{}) {
					/*
						TODO: Determine if we need to skip requeueing in case of CSI driver failure.
					*/
					ctrl.enqueueGroupSnapshotContentWork(newObj)
				},
				DeleteFunc: func(obj interface{}) { ctrl.enqueueGroupSnapshotContentWork(obj) },
			},
			ctrl.resyncPeriod,
		)

		ctrl.groupSnapshotContentLister = volumeGroupSnapshotContentInformer.Lister()
		ctrl.groupSnapshotContentListerSynced = volumeGroupSnapshotContentInformer.Informer().HasSynced

		ctrl.groupSnapshotClassLister = volumeGroupSnapshotClassInformer.Lister()
		ctrl.groupSnapshotClassListerSynced = volumeGroupSnapshotClassInformer.Informer().HasSynced

	}

	return ctrl
}

func (ctrl *csiSnapshotSideCarController) Run(workers int, stopCh <-chan struct{}) {
	defer ctrl.contentQueue.ShutDown()

	klog.Infof("Starting CSI snapshotter")
	defer klog.Infof("Shutting CSI snapshotter")

	informersSynced := []cache.InformerSynced{ctrl.contentListerSynced, ctrl.classListerSynced}
	if ctrl.enableVolumeGroupSnapshots {
		informersSynced = append(informersSynced, []cache.InformerSynced{ctrl.groupSnapshotContentListerSynced, ctrl.groupSnapshotClassListerSynced}...)
	}

	if !cache.WaitForCacheSync(stopCh, informersSynced...) {
		klog.Errorf("Cannot sync caches")
		return
	}

	ctrl.initializeCaches()

	for i := 0; i < workers; i++ {
		go wait.Until(ctrl.contentWorker, 0, stopCh)
		if ctrl.enableVolumeGroupSnapshots {
			go wait.Until(ctrl.groupSnapshotContentWorker, 0, stopCh)
		}
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

// isDriverMatch verifies whether the driver specified in VolumeSnapshotContent
// or VolumeGroupSnapshotContent matches the controller's driver name
func (ctrl *csiSnapshotSideCarController) isDriverMatch(object interface{}) bool {
	if content, ok := object.(*crdv1.VolumeSnapshotContent); ok {
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
	if content, ok := object.(*crdv1alpha1.VolumeGroupSnapshotContent); ok {
		if content.Spec.Source.VolumeGroupSnapshotHandle == nil && len(content.Spec.Source.PersistentVolumeNames) == 0 {
			// Skip this group snapshot content if it does not have a valid source
			return false
		}
		if content.Spec.Driver != ctrl.driverName {
			// Skip this group snapshot content if the driver does not match
			return false
		}
		groupSnapshotClassName := content.Spec.VolumeGroupSnapshotClassName
		if groupSnapshotClassName != nil {
			if groupSnapshotClass, err := ctrl.groupSnapshotClassLister.Get(*groupSnapshotClassName); err == nil {
				if groupSnapshotClass.Driver != ctrl.driverName {
					return false
				}
			}
		}
		return true
	}
	return false
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
func (ctrl *csiSnapshotSideCarController) initializeCaches() {
	contentList, err := ctrl.contentLister.List(labels.Everything())
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

	if ctrl.enableVolumeGroupSnapshots {
		groupSnapshotContentList, err := ctrl.groupSnapshotContentLister.List(labels.Everything())
		if err != nil {
			klog.Errorf("CSISnapshotController can't initialize caches: %v", err)
			return
		}
		for _, groupSnapshotContent := range groupSnapshotContentList {
			groupSnapshotContentClone := groupSnapshotContent.DeepCopy()
			if _, err = ctrl.storeGroupSnapshotContentUpdate(groupSnapshotContentClone); err != nil {
				klog.Errorf("error updating volume group snapshot content cache: %v", err)
			}
		}
	}

	klog.V(4).Infof("controller initialized")
}
