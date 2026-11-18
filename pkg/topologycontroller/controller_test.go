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

package topologycontroller

import (
	"context"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapfake "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	snapinformers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/topology"
)

const zoneKey = "topology.kubernetes.io/zone"

func TestMain(m *testing.M) {
	// Snapshot fake clients do not support watch-list streaming.
	os.Setenv("KUBE_FEATURE_WatchListClient", "false")
	os.Exit(m.Run())
}

func ptr[T any](v T) *T { return &v }

func zoneTerm(values ...string) v1.TopologySelectorTerm {
	return v1.TopologySelectorTerm{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: values}}}
}

func regionZoneTerm(region, zone string) v1.TopologySelectorTerm {
	return v1.TopologySelectorTerm{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
		{Key: "topology.kubernetes.io/region", Values: []string{region}},
		{Key: zoneKey, Values: []string{zone}},
	}}
}

func zoneAffinity(values ...string) *v1.NodeAffinity {
	return &v1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
		NodeSelectorTerms: []v1.NodeSelectorTerm{{MatchExpressions: []v1.NodeSelectorRequirement{
			{Key: zoneKey, Operator: v1.NodeSelectorOpIn, Values: values},
		}}},
	}}
}

func wffcClass() *storagev1.StorageClass {
	mode := storagev1.VolumeBindingWaitForFirstConsumer
	return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "wffc"}, VolumeBindingMode: &mode}
}

func snapshotPVC(name, snapName string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: ptr("wffc"),
			DataSource:       &v1.TypedLocalObjectReference{APIGroup: ptr(topology.SnapshotAPIGroup), Kind: "VolumeSnapshot", Name: snapName},
		},
	}
}

func boundSnapshot(name, contentName string) *snapv1.VolumeSnapshot {
	return &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name, UID: types.UID("uid-" + name)},
		Status:     &snapv1.VolumeSnapshotStatus{BoundVolumeSnapshotContentName: ptr(contentName)},
	}
}

func content(name, snapName string, terms ...v1.TopologySelectorTerm) *snapv1.VolumeSnapshotContent {
	return &snapv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: snapv1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: v1.ObjectReference{Namespace: "default", Name: snapName, UID: types.UID("uid-" + snapName)},
			NodeAffinity:      terms,
		},
		Status: &snapv1.VolumeSnapshotContentStatus{ReadyToUse: ptr(true)},
	}
}

type podOption func(*v1.Pod)

func gated(p *v1.Pod) {
	p.Spec.SchedulingGates = append(p.Spec.SchedulingGates, v1.PodSchedulingGate{Name: topology.SchedulingGate})
}

func newPod(name string, claims []string, opts ...podOption) *v1.Pod {
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace:         "default",
		Name:              name,
		UID:               types.UID(name + "-uid"),
		ResourceVersion:   "7",
		CreationTimestamp: metav1.Now(),
		Labels:            map[string]string{topology.ManagedLabel: "true"},
	}}
	for _, cn := range claims {
		pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
			Name:         cn,
			VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: cn}},
		})
	}
	for _, o := range opts {
		o(pod)
	}
	return pod
}

type env struct {
	t          *testing.T
	ctx        context.Context
	client     *fake.Clientset
	snapClient *snapfake.Clientset
	ctrl       *Controller
	recorder   *record.FakeRecorder
	podLists   []metav1.ListOptions
}

type objects struct {
	kube []runtime.Object
	snap []runtime.Object
}

func defaultObjects(extra ...runtime.Object) objects {
	return objects{
		kube: append([]runtime.Object{wffcClass(), snapshotPVC("c", "snap")}, extra...),
		snap: []runtime.Object{boundSnapshot("snap", "content"), content("content", "snap", zoneTerm("a"))},
	}
}

// newEnv seeds the informer caches without starting watches or handlers.
func newEnv(t *testing.T, config Config, objs objects, setup ...func(*env)) *env {
	t.Helper()
	e, informers := newTestEnv(t, config, objs, setup...)
	for _, group := range [][]runtime.Object{objs.kube, objs.snap} {
		for _, obj := range group {
			informer, ok := informers[reflect.TypeOf(obj)]
			if !ok {
				t.Fatalf("no informer for fixture %T", obj)
			}
			if err := informer.GetIndexer().Add(obj.DeepCopyObject()); err != nil {
				t.Fatal(err)
			}
		}
	}
	return e
}

func newLiveEnv(t *testing.T, config Config, objs objects) *env {
	t.Helper()
	e, informers := newTestEnv(t, config, objs)
	ctx, cancel := context.WithCancel(e.ctx)
	var wg sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
	var synced []cache.InformerSynced
	for _, informer := range informers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			informer.RunWithContext(ctx)
		}()
		synced = append(synced, informer.HasSynced)
	}
	// Wait only for informers; tests may hold ResolverSynced back.
	if !cache.WaitForCacheSync(ctx.Done(), synced...) {
		t.Fatal("informer caches did not sync")
	}
	return e
}

func newTestEnv(t *testing.T, config Config, objs objects, setup ...func(*env)) (*env, map[reflect.Type]cache.SharedIndexInformer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	e := &env{t: t, ctx: ctx, client: fake.NewClientset(objs.kube...), snapClient: snapfake.NewSimpleClientset(objs.snap...)}
	e.client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		restrictions := action.(k8stesting.ListAction).GetListRestrictions()
		e.podLists = append(e.podLists, metav1.ListOptions{LabelSelector: restrictions.Labels.String(), FieldSelector: restrictions.Fields.String()})
		return false, nil, nil
	})
	for _, f := range setup {
		f(e)
	}

	if config.Enabled == nil {
		config.Enabled = func() bool { return true }
	}
	if config.MaxAffinityTerms == 0 {
		config.MaxAffinityTerms = 32
	}
	if config.GateTimeout == 0 {
		config.GateTimeout = time.Hour
	}
	if config.RetryIntervalStart == 0 {
		config.RetryIntervalStart = 100 * time.Millisecond
	}
	if config.RetryIntervalMax == 0 {
		config.RetryIntervalMax = time.Second
	}

	coreFactory := informers.NewSharedInformerFactory(e.client, 0)
	podFactory := informers.NewSharedInformerFactoryWithOptions(e.client, 0, informers.WithTweakListOptions(PodListOptions))
	snapFactory := snapinformers.NewSharedInformerFactory(e.snapClient, 0)
	resolver := &topology.Resolver{
		PVCs:           coreFactory.Core().V1().PersistentVolumeClaims().Lister(),
		StorageClasses: coreFactory.Storage().V1().StorageClasses().Lister(),
		Snapshots:      snapFactory.Snapshot().V1().VolumeSnapshots().Lister(),
		Contents:       snapFactory.Snapshot().V1().VolumeSnapshotContents().Lister(),
	}
	e.recorder = record.NewFakeRecorder(100)
	if err := AddPVCIndexers(coreFactory.Core().V1().PersistentVolumeClaims()); err != nil {
		t.Fatal(err)
	}
	ctrl, err := newController(config, e.client, resolver,
		podFactory.Core().V1().Pods(),
		coreFactory.Core().V1().PersistentVolumeClaims(),
		snapFactory.Snapshot().V1().VolumeSnapshots(),
		snapFactory.Snapshot().V1().VolumeSnapshotContents(),
		e.recorder)
	if err != nil {
		t.Fatal(err)
	}
	e.ctrl = ctrl
	t.Cleanup(ctrl.queue.ShutDown)
	return e, map[reflect.Type]cache.SharedIndexInformer{
		reflect.TypeOf(&v1.Pod{}):                       podFactory.Core().V1().Pods().Informer(),
		reflect.TypeOf(&v1.PersistentVolumeClaim{}):     coreFactory.Core().V1().PersistentVolumeClaims().Informer(),
		reflect.TypeOf(&storagev1.StorageClass{}):       coreFactory.Storage().V1().StorageClasses().Informer(),
		reflect.TypeOf(&snapv1.VolumeSnapshot{}):        snapFactory.Snapshot().V1().VolumeSnapshots().Informer(),
		reflect.TypeOf(&snapv1.VolumeSnapshotContent{}): snapFactory.Snapshot().V1().VolumeSnapshotContents().Informer(),
	}
}

func drain(q workqueue.TypedRateLimitingInterface[string]) []string {
	var keys []string
	for q.Len() > 0 {
		k, _ := q.Get()
		q.Done(k)
		q.Forget(k)
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (e *env) pod(name string) *v1.Pod {
	e.t.Helper()
	pod, err := e.client.CoreV1().Pods("default").Get(e.ctx, name, metav1.GetOptions{})
	if err != nil {
		e.t.Fatal(err)
	}
	return pod
}

func (e *env) events() []string {
	var out []string
	for {
		select {
		case ev := <-e.recorder.Events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func (e *env) actions(verb, resource, subresource string) []k8stesting.Action {
	var out []k8stesting.Action
	for _, a := range e.client.Actions() {
		if a.GetVerb() == verb && a.GetResource().Resource == resource && a.GetSubresource() == subresource {
			out = append(out, a)
		}
	}
	return out
}

func hasEvent(events []string, substr string) bool {
	return slices.ContainsFunc(events, func(e string) bool { return strings.Contains(e, substr) })
}

func TestUngate(t *testing.T) {
	userAffinity := &v1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
		NodeSelectorTerms: []v1.NodeSelectorTerm{{MatchExpressions: []v1.NodeSelectorRequirement{
			{Key: "disk", Operator: v1.NodeSelectorOpIn, Values: []string{"ssd"}},
		}}},
	}}

	testcases := map[string]struct {
		config       Config
		objs         objects
		pod          *v1.Pod
		wantErr      bool
		wantGated    bool
		wantGates    []string
		wantAffinity *v1.NodeAffinity
		wantEvent    string
	}{
		"resolved pod gets affinity and loses the gate": {
			objs: defaultObjects(),
			pod: newPod("p", []string{"c"}, gated, func(p *v1.Pod) {
				p.Annotations = map[string]string{"note": "x"}
				p.Labels["unrelated"] = "x"
			}),
			wantAffinity: zoneAffinity("a"),
		},
		"existing required affinity gets expressions appended": {
			objs: defaultObjects(),
			pod: newPod("p", []string{"c"}, gated, func(p *v1.Pod) {
				p.Spec.Affinity = &v1.Affinity{NodeAffinity: userAffinity.DeepCopy()}
			}),
			wantAffinity: &v1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{{MatchExpressions: []v1.NodeSelectorRequirement{
					{Key: "disk", Operator: v1.NodeSelectorOpIn, Values: []string{"ssd"}},
					{Key: zoneKey, Operator: v1.NodeSelectorOpIn, Values: []string{"a"}},
				}}},
			}},
		},
		"other scheduling gates are kept": {
			objs: defaultObjects(),
			pod: newPod("p", []string{"c"}, func(p *v1.Pod) {
				p.Spec.SchedulingGates = []v1.PodSchedulingGate{{Name: "first"}, {Name: topology.SchedulingGate}, {Name: "last"}}
			}),
			wantGates:    []string{"first", "last"},
			wantAffinity: zoneAffinity("a"),
		},
		"content without affinity removes the gate only": {
			objs: objects{
				kube: []runtime.Object{wffcClass(), snapshotPVC("c", "snap")},
				snap: []runtime.Object{boundSnapshot("snap", "content"), content("content", "snap")},
			},
			pod: newPod("p", []string{"c"}, gated),
		},
		"pending snapshot stays gated": {
			objs: objects{
				kube: []runtime.Object{wffcClass(), snapshotPVC("c", "snap")},
				snap: []runtime.Object{&snapv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap"}}},
			},
			pod:       newPod("p", []string{"c"}, gated),
			wantErr:   true,
			wantGated: true,
			wantEvent: "WaitingForSnapshot Waiting for VolumeSnapshot default/snap",
		},
		"missing claim stays gated": {
			objs:      objects{kube: []runtime.Object{wffcClass()}},
			pod:       newPod("p", []string{"c"}, gated),
			wantErr:   true,
			wantGated: true,
			wantEvent: "WaitingForPVC Waiting for PersistentVolumeClaim default/c",
		},
		"missing named storage class stays gated": {
			objs: objects{
				kube: []runtime.Object{snapshotPVC("c", "snap")},
				snap: []runtime.Object{boundSnapshot("snap", "content"), content("content", "snap", zoneTerm("a"))},
			},
			pod:       newPod("p", []string{"c"}, gated),
			wantErr:   true,
			wantGated: true,
			wantEvent: "WaitingForStorageClass Waiting for StorageClass wffc",
		},
		"expired timeout removes the gate without topology": {
			config: Config{GateTimeout: 5 * time.Minute},
			objs:   objects{kube: []runtime.Object{wffcClass()}},
			pod: newPod("p", []string{"c"}, gated, func(p *v1.Pod) {
				p.CreationTimestamp = metav1.NewTime(time.Now().Add(-10 * time.Minute))
			}),
			wantEvent: "Removed the scheduling gate after waiting",
		},
		"unset creation timestamp keeps waiting": {
			config: Config{GateTimeout: time.Nanosecond},
			objs:   objects{kube: []runtime.Object{wffcClass()}},
			pod: newPod("p", []string{"c"}, gated, func(p *v1.Pod) {
				p.CreationTimestamp = metav1.Time{}
			}),
			wantErr:   true,
			wantGated: true,
			wantEvent: "WaitingForPVC Waiting for PersistentVolumeClaim default/c",
		},
		"unrepresentable topology removes the gate without it": {
			objs: objects{
				kube: []runtime.Object{wffcClass(), snapshotPVC("c", "snap")},
				snap: []runtime.Object{boundSnapshot("snap", "content"), content("content", "snap", regionZoneTerm("r1", "z1"), regionZoneTerm("r2", "z2"))},
			},
			pod: newPod("p", []string{"c"}, gated, func(p *v1.Pod) {
				p.Spec.Affinity = &v1.Affinity{NodeAffinity: userAffinity.DeepCopy()}
			}),
			wantAffinity: userAffinity,
			wantEvent:    "cannot be combined with the pod's own required node affinity",
		},
		"too many terms removes the gate without the topology": {
			config: Config{MaxAffinityTerms: 1},
			objs: objects{
				kube: []runtime.Object{wffcClass(), snapshotPVC("c", "snap")},
				snap: []runtime.Object{boundSnapshot("snap", "content"), content("content", "snap", regionZoneTerm("r1", "z1"), regionZoneTerm("r2", "z2"))},
			},
			pod:       newPod("p", []string{"c"}, gated),
			wantEvent: "more than 1 node selector terms",
		},
		"unsatisfiable topology removes the gate without it": {
			objs: objects{
				kube: []runtime.Object{wffcClass(), snapshotPVC("c", "snap")},
				snap: []runtime.Object{boundSnapshot("snap", "content"), content("content", "snap", zoneTerm())},
			},
			pod:       newPod("p", []string{"c"}, gated),
			wantEvent: "matches no node",
		},
		"feature gate off removes only the gate": {
			config: Config{Enabled: func() bool { return false }},
			objs:   defaultObjects(),
			pod:    newPod("p", []string{"c"}, gated),
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			tc.objs.kube = append(tc.objs.kube, tc.pod)
			e := newEnv(t, tc.config, tc.objs)

			err := e.ctrl.syncPod(e.ctx, "default/p")
			if (err != nil) != tc.wantErr {
				t.Fatalf("syncPod() error = %v, wantErr %v", err, tc.wantErr)
			}
			pod := e.pod("p")
			var gates []string
			gatedNow := false
			for _, g := range pod.Spec.SchedulingGates {
				if g.Name == topology.SchedulingGate {
					gatedNow = true
				} else {
					gates = append(gates, g.Name)
				}
			}
			if gatedNow != tc.wantGated {
				t.Errorf("gated = %v, want %v", gatedNow, tc.wantGated)
			}
			if _, labeled := pod.Labels[topology.ManagedLabel]; labeled != tc.wantGated {
				t.Errorf("managed label present = %v, want %v", labeled, tc.wantGated)
			}
			wantLabels := tc.pod.DeepCopy().Labels
			if !tc.wantGated {
				delete(wantLabels, topology.ManagedLabel)
			}
			if !apiequality.Semantic.DeepEqual(pod.Labels, wantLabels) {
				t.Errorf("labels = %v, want %v", pod.Labels, wantLabels)
			}
			if !apiequality.Semantic.DeepEqual(pod.Annotations, tc.pod.Annotations) {
				t.Errorf("annotations = %v, want %v", pod.Annotations, tc.pod.Annotations)
			}
			if !slices.Equal(gates, tc.wantGates) {
				t.Errorf("other gates = %v, want %v", gates, tc.wantGates)
			}
			if got := podNodeAffinity(pod); !apiequality.Semantic.DeepEqual(got, tc.wantAffinity) {
				t.Errorf("node affinity = %+v, want %+v", got, tc.wantAffinity)
			}
			events := e.events()
			if tc.wantEvent != "" && !hasEvent(events, tc.wantEvent) {
				t.Errorf("events %v do not contain %q", events, tc.wantEvent)
			}
			if tc.wantEvent == "" && len(events) > 0 {
				t.Errorf("unexpected events %v", events)
			}
			if patches := e.actions("patch", "pods", ""); !tc.wantGated && len(patches) != 1 {
				t.Errorf("got %d pod patches, want 1", len(patches))
			} else if tc.wantGated && len(patches) != 0 {
				t.Errorf("got %d pod patches, want 0", len(patches))
			}
		})
	}
}

func TestUngateConcurrentChange(t *testing.T) {
	testcases := map[string]struct {
		change  func(*v1.Pod)
		wantErr bool
	}{
		"pod replaced with the same name and spec": {
			change:  func(p *v1.Pod) { p.UID = "replacement-uid" },
			wantErr: true,
		},
		"gates changed": {
			change: func(p *v1.Pod) {
				p.Spec.SchedulingGates = append(p.Spec.SchedulingGates, v1.PodSchedulingGate{Name: "other"})
			},
			wantErr: true,
		},
		"node affinity added": {
			change: func(p *v1.Pod) {
				p.Spec.Affinity = &v1.Affinity{NodeAffinity: zoneAffinity("b")}
			},
			wantErr: true,
		},
		"annotations added": {
			change: func(p *v1.Pod) { p.Annotations = map[string]string{"example.com/queue": "x"} },
		},
		"managed label removed": {
			change:  func(p *v1.Pod) { delete(p.Labels, topology.ManagedLabel) },
			wantErr: true,
		},
		"label changed": {
			change: func(p *v1.Pod) { p.Labels["unrelated"] = "x" },
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			// Simulate an API update not yet seen by the cache.
			e := newEnv(t, Config{}, defaultObjects(newPod("p", []string{"c"}, gated)), func(e *env) {
				e.client.PrependReactor("patch", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
					stored, err := e.client.Tracker().Get(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, "default", "p")
					if err != nil {
						return true, nil, err
					}
					changed := stored.(*v1.Pod).DeepCopy()
					tc.change(changed)
					if err := e.client.Tracker().Update(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, changed, "default"); err != nil {
						return true, nil, err
					}
					return false, nil, nil
				})
			})
			before := e.pod("p")

			err := e.ctrl.syncPod(e.ctx, "default/p")
			if (err != nil) != tc.wantErr {
				t.Fatalf("syncPod() error = %v, wantErr %v", err, tc.wantErr)
			}
			after := e.pod("p")
			if tc.wantErr {
				expected := before.DeepCopy()
				tc.change(expected)
				if !apiequality.Semantic.DeepEqual(after, expected) {
					t.Errorf("pod was partially patched: %+v", after)
				}
				return
			}
			expected := before.DeepCopy()
			tc.change(expected)
			delete(expected.Labels, topology.ManagedLabel)
			if !apiequality.Semantic.DeepEqual(after.Labels, expected.Labels) || !apiequality.Semantic.DeepEqual(after.Annotations, expected.Annotations) ||
				len(after.Spec.SchedulingGates) != 0 || podNodeAffinity(after) == nil {
				t.Errorf("pod not patched as expected: %+v", after)
			}
		})
	}
}

func TestEventFanIn(t *testing.T) {
	grp := topology.SnapshotAPIGroup
	ephemeralPod := newPod("eph", nil, gated, func(p *v1.Pod) {
		p.Spec.Volumes = []v1.Volume{{Name: "data", VolumeSource: v1.VolumeSource{Ephemeral: &v1.EphemeralVolumeSource{
			VolumeClaimTemplate: &v1.PersistentVolumeClaimTemplate{Spec: v1.PersistentVolumeClaimSpec{
				StorageClassName: ptr("wffc"),
				DataSource:       &v1.TypedLocalObjectReference{APIGroup: &grp, Kind: "VolumeSnapshot", Name: "snap"},
			}},
		}}}}
	})
	objs := defaultObjects(
		newPod("p1", []string{"c"}, gated),
		newPod("p2", []string{"c"}, gated),
		newPod("p3", []string{"c"}, gated),
		newPod("unrelated", []string{"other"}, gated),
		newPod("waiting", []string{"late"}, gated),
		ephemeralPod,
		snapshotPVC("other", "other-snap"),
	)
	e := newEnv(t, Config{}, objs)

	snap := boundSnapshot("snap", "content")
	testcases := map[string]struct {
		trigger func()
		want    []string
	}{
		"content update enqueues every pod restoring from it": {
			trigger: func() { e.ctrl.contentChanged(content("content", "snap", zoneTerm("b"))) },
			want:    []string{"default/eph", "default/p1", "default/p2", "default/p3"},
		},
		"content without a snapshot reference enqueues nothing": {
			trigger: func() { e.ctrl.contentChanged(&snapv1.VolumeSnapshotContent{ObjectMeta: metav1.ObjectMeta{Name: "x"}}) },
		},
		"snapshot update enqueues every pod restoring from it": {
			trigger: func() { e.ctrl.snapshotChanged(snap) },
			want:    []string{"default/eph", "default/p1", "default/p2", "default/p3"},
		},
		"claim created for a waiting pod": {
			trigger: func() { e.ctrl.pvcChanged(snapshotPVC("late", "snap")) },
			want:    []string{"default/waiting"},
		},
		"claim update enqueues the pods using it": {
			trigger: func() { e.ctrl.pvcChanged(snapshotPVC("other", "other-snap")) },
			want:    []string{"default/unrelated"},
		},
		"ephemeral claim created": {
			trigger: func() { e.ctrl.pvcChanged(snapshotPVC("eph-data", "snap")) },
			want:    []string{"default/eph"},
		},
		"unexpected objects are ignored": {
			trigger: func() {
				e.ctrl.contentChanged("not a content")
				e.ctrl.snapshotChanged("not a snapshot")
			},
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			drain(e.ctrl.queue)
			tc.trigger()
			if got := drain(e.ctrl.queue); !slices.Equal(got, tc.want) {
				t.Errorf("queued %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPodInformerSelectors(t *testing.T) {
	e := newLiveEnv(t, Config{}, defaultObjects())
	want := metav1.ListOptions{LabelSelector: topology.ManagedLabel + "=true", FieldSelector: "spec.nodeName="}
	if len(e.podLists) == 0 || e.podLists[0] != want {
		t.Errorf("pod list options = %+v, want %+v", e.podLists, want)
	}
}

func TestRun(t *testing.T) {
	e := newLiveEnv(t, Config{}, defaultObjects(newPod("p", []string{"c"}, gated)))
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		e.ctrl.Run(ctx, 1)
		close(done)
	}()

	waitFor(t, func() bool { return len(e.pod("p").Spec.SchedulingGates) == 0 }, "the gate to be removed")
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}

func TestRunWaitsForSnapshot(t *testing.T) {
	unbound := &snapv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap"}}
	e := newLiveEnv(t, Config{}, objects{
		kube: []runtime.Object{wffcClass(), snapshotPVC("c", "snap"), newPod("p", []string{"c"}, gated)},
		snap: []runtime.Object{unbound, content("content", "snap", zoneTerm("a"))},
	})
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	go e.ctrl.Run(ctx, 1)

	waitFor(t, func() bool { return hasEvent(e.events(), "WaitingForSnapshot") }, "the pod to wait for its snapshot")
	if len(e.pod("p").Spec.SchedulingGates) != 1 {
		t.Fatalf("pod was ungated before its snapshot was bound")
	}

	if _, err := e.snapClient.SnapshotV1().VolumeSnapshots("default").Update(e.ctx, boundSnapshot("snap", "content"), metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return len(e.pod("p").Spec.SchedulingGates) == 0 }, "the gate to be removed")
	if got := podNodeAffinity(e.pod("p")); !apiequality.Semantic.DeepEqual(got, zoneAffinity("a")) {
		t.Errorf("node affinity = %+v, want zone a", got)
	}
}

func TestGateTimeoutWakesBeforeRetryBackoff(t *testing.T) {
	e := newEnv(t, Config{
		GateTimeout:        500 * time.Millisecond,
		RetryIntervalStart: time.Minute,
		RetryIntervalMax:   time.Minute,
	}, objects{kube: []runtime.Object{newPod("p", []string{"missing"}, gated)}})

	e.ctrl.queue.Add("default/p")
	if !processNext(e.ctx, e.ctrl.queue, e.ctrl.syncPod) {
		t.Fatal("worker stopped before the initial reconciliation")
	}
	if len(e.pod("p").Spec.SchedulingGates) != 1 {
		t.Fatal("pod was ungated before the timeout")
	}
	if retries := e.ctrl.queue.NumRequeues("default/p"); retries != 1 {
		t.Fatalf("retry count = %d, want 1", retries)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		processNext(e.ctx, e.ctrl.queue, e.ctrl.syncPod)
	}()
	t.Cleanup(func() {
		e.ctrl.queue.ShutDown()
		<-done
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("gate deadline did not wake the worker before its one-minute retry backoff")
	}
	pod := e.pod("p")
	if len(pod.Spec.SchedulingGates) != 0 || podNodeAffinity(pod) != nil {
		t.Errorf("pod was not ungated without topology: %+v", pod)
	}
	if _, labeled := pod.Labels[topology.ManagedLabel]; labeled {
		t.Error("managed label remains after the gate was removed")
	}
	if retries := e.ctrl.queue.NumRequeues("default/p"); retries != 0 {
		t.Errorf("retry count after success = %d, want 0", retries)
	}
	if events := e.events(); !hasEvent(events, "Removed the scheduling gate after waiting") {
		t.Errorf("events = %v, want a timeout warning", events)
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNewController(t *testing.T) {
	client := fake.NewClientset()
	snapClient := snapfake.NewSimpleClientset()
	coreFactory := informers.NewSharedInformerFactory(client, 0)
	podFactory := informers.NewSharedInformerFactoryWithOptions(client, 0, informers.WithTweakListOptions(PodListOptions))
	snapFactory := snapinformers.NewSharedInformerFactory(snapClient, 0)
	newCtrl := func() error {
		_, err := NewController(Config{
			RetryIntervalStart: time.Second,
			RetryIntervalMax:   time.Minute,
		}, client, &topology.Resolver{},
			podFactory.Core().V1().Pods(),
			coreFactory.Core().V1().PersistentVolumeClaims(),
			snapFactory.Snapshot().V1().VolumeSnapshots(),
			snapFactory.Snapshot().V1().VolumeSnapshotContents())
		return err
	}
	if err := newCtrl(); err == nil || !strings.Contains(err.Error(), "AddPVCIndexers") {
		t.Errorf("NewController() without the PVC index: error = %v, want one naming AddPVCIndexers", err)
	}
	if err := AddPVCIndexers(coreFactory.Core().V1().PersistentVolumeClaims()); err != nil {
		t.Fatal(err)
	}
	if err := newCtrl(); err != nil {
		t.Fatalf("NewController() error: %v", err)
	}
}

func TestPVCSnapshotKeys(t *testing.T) {
	ref := snapshotPVC("ref", "")
	ref.Spec.DataSource = nil
	ref.Spec.DataSourceRef = &v1.TypedObjectReference{APIGroup: ptr(topology.SnapshotAPIGroup), Kind: "VolumeSnapshot", Name: "snap-ref"}
	sameNamespace := ref.DeepCopy()
	sameNamespace.Spec.DataSourceRef.Namespace = ptr("default")
	otherNamespace := ref.DeepCopy()
	otherNamespace.Spec.DataSourceRef.Namespace = ptr("other")
	plain := snapshotPVC("plain", "")
	plain.Spec.DataSource = nil
	testcases := map[string]struct {
		obj  interface{}
		want []string
	}{
		"dataSource":                             {obj: snapshotPVC("c", "snap"), want: []string{"default/snap"}},
		"dataSourceRef":                          {obj: ref, want: []string{"default/snap-ref"}},
		"dataSourceRef naming its own namespace": {obj: sameNamespace, want: []string{"default/snap-ref"}},
		"cross-namespace dataSourceRef":          {obj: otherNamespace},
		"no snapshot source":                     {obj: plain},
		"not a claim":                            {obj: "not a claim"},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got, err := pvcSnapshotKeys(tc.obj)
			if err != nil || !slices.Equal(got, tc.want) {
				t.Errorf("pvcSnapshotKeys() = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestRunWaitsForResolverCaches(t *testing.T) {
	var synced atomic.Bool
	e := newLiveEnv(t, Config{ResolverSynced: []cache.InformerSynced{synced.Load}}, defaultObjects(newPod("p", []string{"c"}, gated)))
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	go e.ctrl.Run(ctx, 1)

	time.Sleep(300 * time.Millisecond)
	if len(e.pod("p").Spec.SchedulingGates) != 1 {
		t.Fatalf("pod was ungated before the resolver caches synced")
	}
	synced.Store(true)
	waitFor(t, func() bool { return len(e.pod("p").Spec.SchedulingGates) == 0 }, "the gate to be removed")
}
