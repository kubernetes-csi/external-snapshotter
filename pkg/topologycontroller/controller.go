/*
Copyright 2026 The Kubernetes Authors.

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

// Package topologycontroller applies snapshot topology to gated pods (KEP-5943).
package topologycontroller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotinformers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-helpers/storage/ephemeral"
	"k8s.io/klog/v2"

	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/topology"
)

const (
	claimIndex    = "claim"
	snapshotIndex = "snapshot"

	reasonTopologyNotApplied = "SnapshotTopologyNotApplied"
)

// PodListOptions selects managed, unscheduled pods for the controller.
func PodListOptions(options *metav1.ListOptions) {
	options.LabelSelector = labels.Set{topology.ManagedLabel: "true"}.String()
	options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", "").String()
}

type Config struct {
	// Enabled reports whether the VolumeSnapshotTopology feature gate is on.
	Enabled          func() bool
	MaxAffinityTerms int
	// RetryIntervalStart and RetryIntervalMax bound retry backoff.
	RetryIntervalStart time.Duration
	RetryIntervalMax   time.Duration
	// GateTimeout bounds waiting before ungating without snapshot topology.
	GateTimeout time.Duration
	// ResolverSynced adds caches that must sync before reconciliation.
	// A partial cache could cause a pod to be ungated without topology.
	ResolverSynced []cache.InformerSynced
}

// Controller reconciles pods labeled with topology.ManagedLabel.
type Controller struct {
	config     Config
	kubeClient kubernetes.Interface
	resolver   *topology.Resolver
	pods       corelisters.PodLister
	podIndexer cache.Indexer
	pvcs       corelisters.PersistentVolumeClaimLister
	recorder   record.EventRecorder
	queue      workqueue.TypedRateLimitingInterface[string]
	synced     []cache.InformerSynced
}

// NewController requires an unstarted pod informer filtered by PodListOptions.
func NewController(
	config Config,
	kubeClient kubernetes.Interface,
	resolver *topology.Resolver,
	podInformer coreinformers.PodInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	snapshotInformer snapshotinformers.VolumeSnapshotInformer,
	contentInformer snapshotinformers.VolumeSnapshotContentInformer,
) (*Controller, error) {
	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(klog.Infof)
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events(v1.NamespaceAll)})
	recorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "snapshot-topology-controller"})
	return newController(config, kubeClient, resolver, podInformer, pvcInformer, snapshotInformer, contentInformer, recorder)
}

func newController(
	config Config,
	kubeClient kubernetes.Interface,
	resolver *topology.Resolver,
	podInformer coreinformers.PodInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	snapshotInformer snapshotinformers.VolumeSnapshotInformer,
	contentInformer snapshotinformers.VolumeSnapshotContentInformer,
	recorder record.EventRecorder,
) (*Controller, error) {
	c := &Controller{
		config:     config,
		kubeClient: kubeClient,
		resolver:   resolver,
		pods:       podInformer.Lister(),
		podIndexer: podInformer.Informer().GetIndexer(),
		pvcs:       pvcInformer.Lister(),
		recorder:   recorder,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.NewTypedItemExponentialFailureRateLimiter[string](config.RetryIntervalStart, config.RetryIntervalMax),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "snapshot-topology-pods"}),
		synced: append([]cache.InformerSynced{
			podInformer.Informer().HasSynced,
			pvcInformer.Informer().HasSynced,
			snapshotInformer.Informer().HasSynced,
			contentInformer.Informer().HasSynced,
		}, config.ResolverSynced...),
	}

	if err := podInformer.Informer().AddIndexers(cache.Indexers{
		claimIndex:    podClaimKeys,
		snapshotIndex: podSnapshotKeys,
	}); err != nil {
		return nil, fmt.Errorf("adding pod indexers: %w", err)
	}

	handlers := []struct {
		informer cache.SharedIndexInformer
		handler  cache.ResourceEventHandler
	}{
		{podInformer.Informer(), cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueuePod,
			UpdateFunc: func(_, obj interface{}) { c.enqueuePod(obj) },
		}},
		{pvcInformer.Informer(), cache.ResourceEventHandlerFuncs{
			AddFunc:    c.pvcChanged,
			UpdateFunc: func(_, obj interface{}) { c.pvcChanged(obj) },
		}},
		{snapshotInformer.Informer(), cache.ResourceEventHandlerFuncs{
			AddFunc:    c.snapshotChanged,
			UpdateFunc: func(_, obj interface{}) { c.snapshotChanged(obj) },
		}},
		{contentInformer.Informer(), cache.ResourceEventHandlerFuncs{
			AddFunc:    c.contentChanged,
			UpdateFunc: func(_, obj interface{}) { c.contentChanged(obj) },
		}},
	}
	for _, h := range handlers {
		if _, err := h.informer.AddEventHandler(h.handler); err != nil {
			return nil, fmt.Errorf("adding event handler: %w", err)
		}
	}
	return c, nil
}

func podClaimKeys(obj interface{}) ([]string, error) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		return nil, nil
	}
	return claimKeys(pod), nil
}

func claimKeys(pod *v1.Pod) []string {
	var keys []string
	for i := range pod.Spec.Volumes {
		vol := &pod.Spec.Volumes[i]
		switch {
		case vol.PersistentVolumeClaim != nil:
			keys = append(keys, pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName)
		case vol.Ephemeral != nil:
			keys = append(keys, pod.Namespace+"/"+ephemeral.VolumeClaimName(pod, vol))
		}
	}
	return keys
}

// Templates need their own index while ephemeral PVCs do not exist.
func podSnapshotKeys(obj interface{}) ([]string, error) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		return nil, nil
	}
	var keys []string
	for _, vol := range pod.Spec.Volumes {
		if vol.Ephemeral == nil || vol.Ephemeral.VolumeClaimTemplate == nil {
			continue
		}
		spec := vol.Ephemeral.VolumeClaimTemplate.Spec
		if name := topology.SnapshotSource(spec.DataSource, spec.DataSourceRef); name != "" {
			keys = append(keys, pod.Namespace+"/"+name)
		}
	}
	return keys, nil
}

func (c *Controller) enqueuePod(obj interface{}) {
	if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
		c.queue.Add(key)
	}
}

func (c *Controller) enqueueIndexed(index, key string) {
	objs, err := c.podIndexer.ByIndex(index, key)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	for _, obj := range objs {
		c.enqueuePod(obj)
	}
}

func (c *Controller) pvcChanged(obj interface{}) {
	if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
		c.enqueueIndexed(claimIndex, key)
	}
}

func (c *Controller) snapshotChanged(obj interface{}) {
	if snapshot, ok := obj.(*snapv1.VolumeSnapshot); ok {
		c.enqueueSnapshotPods(snapshot.Namespace, snapshot.Name)
	}
}

func (c *Controller) contentChanged(obj interface{}) {
	content, ok := obj.(*snapv1.VolumeSnapshotContent)
	if !ok {
		return
	}
	ref := content.Spec.VolumeSnapshotRef
	if ref.Name != "" {
		c.enqueueSnapshotPods(ref.Namespace, ref.Name)
	}
}

// enqueueSnapshotPods follows both template and PVC snapshot references.
func (c *Controller) enqueueSnapshotPods(namespace, name string) {
	c.enqueueIndexed(snapshotIndex, namespace+"/"+name)
	pvcs, err := c.pvcs.PersistentVolumeClaims(namespace).List(labels.Everything())
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	for _, pvc := range pvcs {
		if topology.SnapshotSource(pvc.Spec.DataSource, pvc.Spec.DataSourceRef) == name {
			c.enqueueIndexed(claimIndex, namespace+"/"+pvc.Name)
		}
	}
}

// Run starts the workers and blocks until ctx is done.
func (c *Controller) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()

	klog.Info("Starting snapshot topology controller")
	defer klog.Info("Shutting down snapshot topology controller")
	if !cache.WaitForNamedCacheSync("snapshot-topology", ctx.Done(), c.synced...) {
		c.queue.ShutDown()
		return
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wait.UntilWithContext(ctx, func(ctx context.Context) { processAll(ctx, c.queue, c.syncPod) }, time.Second)
		}()
	}
	<-ctx.Done()
	c.queue.ShutDown()
	wg.Wait()
}

func processAll(ctx context.Context, queue workqueue.TypedRateLimitingInterface[string], sync func(context.Context, string) error) {
	for processNext(ctx, queue, sync) {
	}
}

func processNext(ctx context.Context, queue workqueue.TypedRateLimitingInterface[string], sync func(context.Context, string) error) bool {
	key, quit := queue.Get()
	if quit {
		return false
	}
	defer queue.Done(key)
	if err := sync(ctx, key); err != nil {
		if errors.Is(err, errWaiting) {
			klog.V(4).Infof("Requeuing pod %s: %v", key, err)
		} else {
			klog.Errorf("Failed to sync pod %s, retrying: %v", key, err)
		}
		queue.AddRateLimited(key)
		return true
	}
	queue.Forget(key)
	return true
}

func (c *Controller) getPod(key string) (*v1.Pod, error) {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, err
	}
	pod, err := c.pods.Pods(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	return pod, err
}

func (c *Controller) syncPod(ctx context.Context, key string) error {
	pod, err := c.getPod(key)
	if err != nil || pod == nil || pod.Spec.NodeName != "" || pod.DeletionTimestamp != nil {
		return err
	}
	if !slices.ContainsFunc(pod.Spec.SchedulingGates, func(g v1.PodSchedulingGate) bool { return g.Name == topology.SchedulingGate }) {
		return nil
	}
	if !c.config.Enabled() {
		// Never leave pods stuck behind a gate nothing will remove.
		return c.patchPod(ctx, pod, podNodeAffinity(pod))
	}
	return c.ungate(ctx, pod)
}

func (c *Controller) ungate(ctx context.Context, pod *v1.Pod) error {
	topo := c.resolver.ResolvePod(ctx, pod)
	if topo.Pending != "" {
		deadline := pod.CreationTimestamp.Add(c.config.GateTimeout)
		// An unset timestamp is not evidence that the gate has expired.
		if pod.CreationTimestamp.IsZero() || time.Now().Before(deadline) {
			if !pod.CreationTimestamp.IsZero() {
				// The timeout must wake the pod even if retry backoff is longer.
				c.queue.AddAfter(pod.Namespace+"/"+pod.Name, time.Until(deadline))
			}
			c.recorder.Eventf(pod, v1.EventTypeNormal, string(topo.Pending), "Waiting for %s before applying snapshot topology", topo.PendingDetail)
			return fmt.Errorf("%w for %s", errWaiting, topo.PendingDetail)
		}
		c.recorder.Eventf(pod, v1.EventTypeWarning, reasonTopologyNotApplied,
			"Removed the scheduling gate after waiting %v for %s. The pod is scheduled without the snapshot topology in its node affinity.",
			c.config.GateTimeout, topo.PendingDetail)
		return c.patchPod(ctx, pod, podNodeAffinity(pod))
	}
	merged, err := topology.MergeGated(podNodeAffinity(pod), topo.TermSets, c.config.MaxAffinityTerms)
	if err != nil {
		// Fail open; PVC validation still guards the scheduler's node choice.
		c.recorder.Eventf(pod, v1.EventTypeWarning, reasonTopologyNotApplied,
			"Removed the scheduling gate without adding the snapshot topology to the pod's node affinity: %s", notAppliedReason(err, c.config.MaxAffinityTerms))
		return c.patchPod(ctx, pod, podNodeAffinity(pod))
	}
	return c.patchPod(ctx, pod, merged)
}

var errWaiting = errors.New("waiting")

func notAppliedReason(err error, maxTerms int) string {
	switch {
	case errors.Is(err, topology.ErrUnrepresentable):
		return "it cannot be combined with the pod's own required node affinity while the pod is gated. " +
			"A pod created after the snapshot is ready gets the topology merged at creation instead."
	case errors.Is(err, topology.ErrTooManyTerms):
		return fmt.Sprintf("the merged node affinity would need more than %d node selector terms.", maxTerms)
	case errors.Is(err, topology.ErrUnsatisfiable):
		return "the snapshot topology matches no node, so no node can run this pod."
	default:
		return err.Error()
	}
}

func podNodeAffinity(pod *v1.Pod) *v1.NodeAffinity {
	if pod.Spec.Affinity == nil {
		return nil
	}
	return pod.Spec.Affinity.NodeAffinity
}

// managedLabelPath is the JSON pointer to the managed label (RFC 6901).
var managedLabelPath = "/metadata/labels/" + strings.ReplaceAll(strings.ReplaceAll(topology.ManagedLabel, "~", "~0"), "/", "~1")

type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// patchPod uses an atomic patch with preconditions to protect against pod
// replacement and concurrent changes to gates, labels, or node affinity.
func (c *Controller) patchPod(ctx context.Context, pod *v1.Pod, affinity *v1.NodeAffinity) error {
	remaining := slices.DeleteFunc(slices.Clone(pod.Spec.SchedulingGates), func(g v1.PodSchedulingGate) bool {
		return g.Name == topology.SchedulingGate
	})
	ops := []patchOp{
		{Op: "test", Path: "/metadata/uid", Value: pod.UID},
		{Op: "test", Path: "/spec/schedulingGates", Value: pod.Spec.SchedulingGates},
	}
	if len(remaining) == 0 {
		ops = append(ops, patchOp{Op: "remove", Path: "/spec/schedulingGates"})
	} else {
		ops = append(ops, patchOp{Op: "replace", Path: "/spec/schedulingGates", Value: remaining})
	}

	if value, ok := pod.Labels[topology.ManagedLabel]; ok {
		ops = append(ops,
			patchOp{Op: "test", Path: managedLabelPath, Value: value},
			patchOp{Op: "remove", Path: managedLabelPath})
	}

	if current := podNodeAffinity(pod); !apiequality.Semantic.DeepEqual(current, affinity) {
		if pod.Spec.Affinity == nil {
			ops = append(ops,
				patchOp{Op: "test", Path: "/spec/affinity", Value: nil},
				patchOp{Op: "add", Path: "/spec/affinity", Value: &v1.Affinity{NodeAffinity: affinity}})
		} else {
			ops = append(ops,
				patchOp{Op: "test", Path: "/spec/affinity/nodeAffinity", Value: current},
				patchOp{Op: "add", Path: "/spec/affinity/nodeAffinity", Value: affinity})
		}
	}

	patch, err := json.Marshal(ops)
	if err != nil {
		return err
	}
	if _, err := c.kubeClient.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, types.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("removing scheduling gate: %w", err)
	}
	klog.V(2).Infof("Removed snapshot topology scheduling gate from pod %s/%s", pod.Namespace, pod.Name)
	return nil
}
