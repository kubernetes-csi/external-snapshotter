/*
Copyright 2018 The Kubernetes Authors.

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

package controller

import (
	"fmt"
	"sync"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1beta1"
	clientset "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	storageinformers "github.com/kubernetes-csi/external-snapshotter/pkg/client/informers/externalversions/volumesnapshot/v1beta1"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/pkg/client/listers/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/pkg/snapshotter"

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
	"k8s.io/kubernetes/pkg/util/goroutinemap"
)

type csiSnapshotController struct {
	clientset     clientset.Interface
	client        kubernetes.Interface
	driverName    string
	eventRecorder record.EventRecorder
	snapshotQueue workqueue.RateLimitingInterface
	contentQueue  workqueue.RateLimitingInterface

	// Map UID -> *Snapshot with all snapshots in progress in the background.
	snapshotsInProgress sync.Map

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

	handler Handler
	// Map of scheduled/running operations.
	runningOperations goroutinemap.GoRoutineMap

	createSnapshotContentRetryCount int
	createSnapshotContentInterval   time.Duration
	retryIntervalStart              time.Duration
	retryIntervalMax                time.Duration
	resyncPeriod                    time.Duration
}

// NewCSISnapshotController returns a new *csiSnapshotController
func NewCSISnapshotController(
	clientset clientset.Interface,
	client kubernetes.Interface,
	driverName string,
	volumeSnapshotInformer storageinformers.VolumeSnapshotInformer,
	volumeSnapshotContentInformer storageinformers.VolumeSnapshotContentInformer,
	volumeSnapshotClassInformer storageinformers.VolumeSnapshotClassInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	createSnapshotContentRetryCount int,
	createSnapshotContentInterval time.Duration,
	retryIntervalStart time.Duration,
	retryIntervalMax time.Duration,
	snapshotter snapshotter.Snapshotter,
	timeout time.Duration,
	resyncPeriod time.Duration,
	snapshotNamePrefix string,
	snapshotNameUUIDLength int,
) *csiSnapshotController {
	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(klog.Infof)
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: client.CoreV1().Events(v1.NamespaceAll)})
	var eventRecorder record.EventRecorder
	eventRecorder = broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: fmt.Sprintf("csi-snapshotter %s", driverName)})

	ctrl := &csiSnapshotController{
		clientset:                       clientset,
		client:                          client,
		driverName:                      driverName,
		eventRecorder:                   eventRecorder,
		handler:                         NewCSIHandler(snapshotter, timeout, snapshotNamePrefix, snapshotNameUUIDLength),
		runningOperations:               goroutinemap.NewGoRoutineMap(true),
		createSnapshotContentRetryCount: createSnapshotContentRetryCount,
		createSnapshotContentInterval:   createSnapshotContentInterval,
		retryIntervalStart:              retryIntervalStart,
		retryIntervalMax:                retryIntervalMax,
		resyncPeriod:                    resyncPeriod,
		snapshotStore:                   cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		contentStore:                    cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
		snapshotQueue:                   workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(retryIntervalStart, retryIntervalMax), "csi-snapshotter-snapshot"),
		contentQueue:                    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "csi-snapshotter-content"),
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

func (ctrl *csiSnapshotController) Run(workers int, stopCh <-chan struct{}) {
	defer ctrl.snapshotQueue.ShutDown()
	defer ctrl.contentQueue.ShutDown()

	klog.Infof("Starting CSI snapshotter")
	defer klog.Infof("Shutting CSI snapshotter")

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
func (ctrl *csiSnapshotController) enqueueSnapshotWork(obj interface{}) {
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
func (ctrl *csiSnapshotController) enqueueContentWork(obj interface{}) {
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

// snapshotWorker processes items from snapshotQueue. It must run only once,
// syncSnapshot is not assured to be reentrant.
func (ctrl *csiSnapshotController) snapshotWorker() {
	workFunc := func() bool {
		keyObj, quit := ctrl.snapshotQueue.Get()
		if quit {
			return true
		}
		defer ctrl.snapshotQueue.Done(keyObj)
		key := keyObj.(string)
		klog.V(5).Infof("snapshotWorker[%s]", key)

		namespace, name, err := cache.SplitMetaNamespaceKey(key)
		klog.V(5).Infof("snapshotWorker: snapshot namespace [%s] name [%s]", namespace, name)
		if err != nil {
			klog.Errorf("error getting namespace & name of snapshot %q to get snapshot from informer: %v", key, err)
			return false
		}

		// Attempt to get snapshot from the informer
		var snapshot *crdv1.VolumeSnapshot
		snapshot, err = ctrl.snapshotLister.VolumeSnapshots(namespace).Get(name)
		if err != nil && !errors.IsNotFound(err) {
			klog.V(2).Infof("error getting snapshot %q from informer: %v", key, err)
			return false
		} else if errors.IsNotFound(err) {
			// Check snapshotsInProgress for the snapshot if not found from the informer
			inProgressObj, ok := ctrl.snapshotsInProgress.Load(key)
			if ok {
				snapshot, ok = inProgressObj.(*crdv1.VolumeSnapshot)
				if !ok {
					klog.Errorf("expected vs, got %+v", inProgressObj)
					return false
				}
			}

		}

		if snapshot != nil {
			// If the volume snapshot still exists in informer cache, the event must have
			// been add/update/sync. Otherwise, the volume snapshot was still in progress.
			newSnapshot, err := ctrl.checkAndUpdateSnapshotClass(snapshot)
			if err == nil {
				klog.V(5).Infof("passed checkAndUpdateSnapshotClass for snapshot %q", key)
				ctrl.updateSnapshot(newSnapshot)
			}
			return false
		}

		// The snapshot is not in informer cache or in progress, the event must have been "delete"
		vsObj, found, err := ctrl.snapshotStore.GetByKey(key)
		if err != nil {
			klog.V(2).Infof("error getting snapshot %q from cache: %v", key, err)
			return false
		}
		if !found {
			// The controller has already processed the delete event and
			// deleted the snapshot from its cache
			klog.V(2).Infof("deletion of snapshot %q was already processed", key)
			return false
		}
		snapshot, ok := vsObj.(*crdv1.VolumeSnapshot)
		if !ok {
			klog.Errorf("expected vs, got %+v", vsObj)
			return false
		}
		newSnapshot, err := ctrl.checkAndUpdateSnapshotClass(snapshot)
		if err == nil {
			ctrl.deleteSnapshot(newSnapshot)
		}

		ctrl.snapshotQueue.Forget(keyObj)
		ctrl.snapshotsInProgress.Delete(key)

		return false
	}

	for {
		if quit := workFunc(); quit {
			klog.Infof("snapshot worker queue shutting down")
			return
		}
	}
}

// contentWorker processes items from contentQueue. It must run only once,
// syncContent is not assured to be reentrant.
func (ctrl *csiSnapshotController) contentWorker() {
	workFunc := func() bool {
		keyObj, quit := ctrl.contentQueue.Get()
		if quit {
			return true
		}
		defer ctrl.contentQueue.Done(keyObj)
		key := keyObj.(string)
		klog.V(5).Infof("contentWorker[%s]", key)

		_, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			klog.V(4).Infof("error getting name of snapshotContent %q to get snapshotContent from informer: %v", key, err)
			return false
		}
		content, err := ctrl.contentLister.Get(name)
		// The content still exists in informer cache, the event must have
		// been add/update/sync
		if err == nil {
			if ctrl.isDriverMatch(content) {
				ctrl.updateContent(content)
			}
			return false
		}
		if !errors.IsNotFound(err) {
			klog.V(2).Infof("error getting content %q from informer: %v", key, err)
			return false
		}

		// The content is not in informer cache, the event must have been
		// "delete"
		contentObj, found, err := ctrl.contentStore.GetByKey(key)
		if err != nil {
			klog.V(2).Infof("error getting content %q from cache: %v", key, err)
			return false
		}
		if !found {
			// The controller has already processed the delete event and
			// deleted the content from its cache
			klog.V(2).Infof("deletion of content %q was already processed", key)
			return false
		}
		content, ok := contentObj.(*crdv1.VolumeSnapshotContent)
		if !ok {
			klog.Errorf("expected content, got %+v", content)
			return false
		}
		ctrl.deleteContent(content)
		return false
	}

	for {
		if quit := workFunc(); quit {
			klog.Infof("content worker queue shutting down")
			return
		}
	}
}

// verify whether the driver specified in VolumeSnapshotContent matches the controller's driver name
func (ctrl *csiSnapshotController) isDriverMatch(content *crdv1.VolumeSnapshotContent) bool {
	return content.Spec.Driver == ctrl.driverName
}

// checkAndUpdateSnapshotClass gets the VolumeSnapshotClass from VolumeSnapshot. If it is not set,
// gets it from default VolumeSnapshotClass and sets it. It also detects if snapshotter in the
// VolumeSnapshotClass is the same as the snapshotter in external controller.
func (ctrl *csiSnapshotController) checkAndUpdateSnapshotClass(snapshot *crdv1.VolumeSnapshot) (*crdv1.VolumeSnapshot, error) {
	className := snapshot.Spec.VolumeSnapshotClassName
	var class *crdv1.VolumeSnapshotClass
	var err error
	newSnapshot := snapshot
	if className != nil {
		klog.V(5).Infof("checkAndUpdateSnapshotClass [%s]: VolumeSnapshotClassName [%s]", snapshot.Name, *className)
		class, err = ctrl.GetSnapshotClass(*className)
		if err != nil {
			klog.Errorf("checkAndUpdateSnapshotClass failed to getSnapshotClass %v", err)
			ctrl.updateSnapshotErrorStatusWithEvent(snapshot, v1.EventTypeWarning, "GetSnapshotClassFailed", fmt.Sprintf("Failed to get snapshot class with error %v", err))
			return nil, err
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

	klog.V(5).Infof("VolumeSnapshotClass Driver [%s] Snapshot Controller driverName [%s]", class.Driver, ctrl.driverName)
	if class.Driver != ctrl.driverName {
		klog.V(4).Infof("Skipping VolumeSnapshot %s for snapshotter [%s] in VolumeSnapshotClass because it does not match with the snapshotter for controller [%s]", snapshotKey(snapshot), class.Driver, ctrl.driverName)
		return nil, fmt.Errorf("volumeSnapshotClass does not match with the snapshotter for controller")
	}
	return newSnapshot, nil
}

// updateSnapshot runs in worker thread and handles "snapshot added",
// "snapshot updated" and "periodic sync" events.
func (ctrl *csiSnapshotController) updateSnapshot(snapshot *crdv1.VolumeSnapshot) {
	// Store the new snapshot version in the cache and do not process it if this is
	// an old version.
	klog.V(5).Infof("updateSnapshot %q", snapshotKey(snapshot))
	newSnapshot, err := ctrl.storeSnapshotUpdate(snapshot)
	if err != nil {
		klog.Errorf("%v", err)
	}
	if !newSnapshot {
		return
	}
	err = ctrl.syncSnapshot(snapshot)
	if err != nil {
		sKey := snapshotKey(snapshot)

		// if the snapshot has been deleted, remove from snapshots in progress
		if _, exists, _ := ctrl.snapshotStore.Get(sKey); !exists {
			ctrl.snapshotsInProgress.Delete(sKey)
		} else {
			// otherwise, add back to the snapshot queue to retry.
			ctrl.snapshotQueue.AddRateLimited(sKey)
		}

		if errors.IsConflict(err) {
			// Version conflict error happens quite often and the controller
			// recovers from it easily.
			klog.V(3).Infof("could not sync claim %q: %+v", sKey, err)
		} else {
			klog.Errorf("could not sync volume %q: %+v", sKey, err)
		}
	}
}

// updateContent runs in worker thread and handles "content added",
// "content updated" and "periodic sync" events.
func (ctrl *csiSnapshotController) updateContent(content *crdv1.VolumeSnapshotContent) {
	// Store the new content version in the cache and do not process it if this is
	// an old version.
	new, err := ctrl.storeContentUpdate(content)
	if err != nil {
		klog.Errorf("%v", err)
	}
	if !new {
		return
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
	}
}

// deleteSnapshot runs in worker thread and handles "snapshot deleted" event.
func (ctrl *csiSnapshotController) deleteSnapshot(snapshot *crdv1.VolumeSnapshot) {
	_ = ctrl.snapshotStore.Delete(snapshot)
	klog.V(4).Infof("snapshot %q deleted", snapshotKey(snapshot))

	if snapshot.Status.BoundVolumeSnapshotContentName == nil {
		klog.V(5).Infof("deleteSnapshot[%q]: content not bound", snapshotKey(snapshot))
		return
	}
	snapshotContentName := *snapshot.Status.BoundVolumeSnapshotContentName
	// sync the content when its snapshot is deleted.  Explicitly sync'ing the
	// content here in response to snapshot deletion prevents the content from
	// waiting until the next sync period for its Release.
	klog.V(5).Infof("deleteSnapshot[%q]: scheduling sync of content %s", snapshotKey(snapshot), snapshotContentName)
	ctrl.contentQueue.Add(snapshotContentName)
}

// deleteContent runs in worker thread and handles "content deleted" event.
func (ctrl *csiSnapshotController) deleteContent(content *crdv1.VolumeSnapshotContent) {
	_ = ctrl.contentStore.Delete(content)
	klog.V(4).Infof("content %q deleted", content.Name)

	snapshotName := snapshotRefKey(content.Spec.VolumeSnapshotRef)
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
func (ctrl *csiSnapshotController) initializeCaches(snapshotLister storagelisters.VolumeSnapshotLister, contentLister storagelisters.VolumeSnapshotContentLister) {
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
