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

package topology

import (
	"context"
	"reflect"
	"testing"
	"time"

	"errors"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
)

const testPodUID = "pod-uid"

func newIndexer[T runtime.Object](objs ...T) cache.Indexer {
	idx := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, o := range objs {
		_ = idx.Add(o)
	}
	return idx
}

func newTestResolver(pvcs []*v1.PersistentVolumeClaim, scs []*storagev1.StorageClass, snaps []*snapv1.VolumeSnapshot, contents []*snapv1.VolumeSnapshotContent) *Resolver {
	return &Resolver{
		PVCs:           corelisters.NewPersistentVolumeClaimLister(newIndexer(pvcs...)),
		StorageClasses: storagelisters.NewStorageClassLister(newIndexer(scs...)),
		Snapshots:      snapshotlisters.NewVolumeSnapshotLister(newIndexer(snaps...)),
		Contents:       snapshotlisters.NewVolumeSnapshotContentLister(newIndexer(contents...)),
	}
}

func storageClass(name string, mode storagev1.VolumeBindingMode) *storagev1.StorageClass {
	return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: name}, VolumeBindingMode: &mode}
}

func snapshotPVC(name, scName, snapName, apiGroup string) *v1.PersistentVolumeClaim {
	sc := scName
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
		Spec:       v1.PersistentVolumeClaimSpec{StorageClassName: &sc},
	}
	if snapName != "" {
		grp := apiGroup
		pvc.Spec.DataSource = &v1.TypedLocalObjectReference{APIGroup: &grp, Kind: "VolumeSnapshot", Name: snapName}
	}
	return pvc
}

func boundSnapshot(name, contentName string) *snapv1.VolumeSnapshot {
	return &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name, UID: types.UID("uid-" + name)},
		Status:     &snapv1.VolumeSnapshotStatus{BoundVolumeSnapshotContentName: &contentName},
	}
}

// contentWithAffinity returns a ready content bound back to snapshot snapName
// in the default namespace.
func contentWithAffinity(name, snapName string, terms ...v1.TopologySelectorTerm) *snapv1.VolumeSnapshotContent {
	ready := true
	return &snapv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: snapv1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: v1.ObjectReference{Namespace: "default", Name: snapName, UID: types.UID("uid-" + snapName)},
			NodeAffinity:      terms,
		},
		Status: &snapv1.VolumeSnapshotContentStatus{ReadyToUse: &ready},
	}
}

func notReady(c *snapv1.VolumeSnapshotContent) *snapv1.VolumeSnapshotContent {
	c.Status = nil
	return c
}

func podWithPVCs(claimNames ...string) *v1.Pod {
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "app", UID: testPodUID}}
	for i, cn := range claimNames {
		pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
			Name:         "v" + string(rune('a'+i)),
			VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: cn}},
		})
	}
	return pod
}

// The ephemeral volume's generated PVC is named app-eph.
func podWithEphemeral(scName *string, snapName string) *v1.Pod {
	grp := SnapshotAPIGroup
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "app", UID: testPodUID},
		Spec: v1.PodSpec{Volumes: []v1.Volume{{
			Name: "eph",
			VolumeSource: v1.VolumeSource{Ephemeral: &v1.EphemeralVolumeSource{
				VolumeClaimTemplate: &v1.PersistentVolumeClaimTemplate{Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: scName,
					DataSource:       &v1.TypedLocalObjectReference{APIGroup: &grp, Kind: "VolumeSnapshot", Name: snapName},
				}},
			}},
		}}},
	}
}

func uncreated(pod *v1.Pod) *v1.Pod {
	pod.UID = ""
	return pod
}

func ownedByPod(pvc *v1.PersistentVolumeClaim) *v1.PersistentVolumeClaim {
	controller := true
	pvc.OwnerReferences = []metav1.OwnerReference{{Kind: "Pod", Name: "app", UID: testPodUID, Controller: &controller}}
	return pvc
}

func withSelectedNode(pvc *v1.PersistentVolumeClaim, node string) *v1.PersistentVolumeClaim {
	pvc.Annotations = map[string]string{SelectedNodeAnnotation: node}
	return pvc
}

func ptr[T any](v T) *T { return &v }

// Ported from scheduler-plugin TestPreFilter; unbound snapshots now stay pending.
func TestResolvePod(t *testing.T) {
	wffc := storageClass("wffc", storagev1.VolumeBindingWaitForFirstConsumer)
	immediate := storageClass("immediate", storagev1.VolumeBindingImmediate)
	terms := []v1.TopologySelectorTerm{zoneTerm("us-west-2a")}
	defaultWFFC := storageClass("default-wffc", storagev1.VolumeBindingWaitForFirstConsumer)
	defaultWFFC.Annotations = map[string]string{isDefaultStorageClassAnnotation: "true"}
	defaultImmediate := storageClass("default-immediate", storagev1.VolumeBindingImmediate)
	defaultImmediate.Annotations = map[string]string{isDefaultStorageClassAnnotation: "true"}

	testcases := map[string]struct {
		pvcs     []*v1.PersistentVolumeClaim
		scs      []*storagev1.StorageClass
		snaps    []*snapv1.VolumeSnapshot
		contents []*snapv1.VolumeSnapshotContent
		pod      *v1.Pod
		want     PodTopology
	}{
		"no volumes": {
			pod: &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "app"}},
		},
		"wffc pvc without datasource": {
			pvcs: []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "", "")},
			scs:  []*storagev1.StorageClass{wffc},
			pod:  podWithPVCs("c"),
		},
		"immediate binding is skipped": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "immediate", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{immediate},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
		},
		"wrong api group is skipped": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", "example.com")},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
		},
		"already-bound pvc is skipped": {
			pvcs: []*v1.PersistentVolumeClaim{func() *v1.PersistentVolumeClaim {
				p := snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)
				p.Spec.VolumeName = "pv-1"
				return p
			}()},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
		},
		"snapshot not yet bound is pending": {
			pvcs:  []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:   []*storagev1.StorageClass{wffc},
			snaps: []*snapv1.VolumeSnapshot{{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap"}}},
			pod:   podWithPVCs("c"),
			want:  PodTopology{Relevant: true, Pending: PendingSnapshot, PendingDetail: "VolumeSnapshot default/snap"},
		},
		"content with empty nodeAffinity imposes no constraint": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap")},
			pod:      podWithPVCs("c"),
			want:     PodTopology{Relevant: true},
		},
		"single snapshot pvc yields one term set": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
			want:     PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}},
		},
		"ephemeral volume owned by pod yields one term set": {
			pvcs:     []*v1.PersistentVolumeClaim{ownedByPod(snapshotPVC("app-eph", "wffc", "snap", SnapshotAPIGroup))},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithEphemeral(ptr("wffc"), "snap"),
			want:     PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}},
		},
		"ephemeral volume not owned by pod is skipped": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("app-eph", "wffc", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithEphemeral(ptr("wffc"), "snap"),
		},
		"two snapshot pvcs yield two term sets": {
			pvcs: []*v1.PersistentVolumeClaim{
				snapshotPVC("c1", "wffc", "snap1", SnapshotAPIGroup),
				snapshotPVC("c2", "wffc", "snap2", SnapshotAPIGroup),
			},
			scs:   []*storagev1.StorageClass{wffc},
			snaps: []*snapv1.VolumeSnapshot{boundSnapshot("snap1", "content1"), boundSnapshot("snap2", "content2")},
			contents: []*snapv1.VolumeSnapshotContent{
				contentWithAffinity("content1", "snap1", terms...),
				contentWithAffinity("content2", "snap2", zoneTerm("us-west-2b")),
			},
			pod: podWithPVCs("c1", "c2"),
			want: PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{
				terms, {zoneTerm("us-west-2b")},
			}},
		},

		"missing pvc is pending": {
			pod:  podWithPVCs("c"),
			want: PodTopology{Relevant: true, Pending: PendingPVC, PendingDetail: "PersistentVolumeClaim default/c"},
		},
		"missing named storage class is pending even with a default": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "missing", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{defaultWFFC},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
			want:     PodTopology{Relevant: true, Pending: PendingStorageClass, PendingDetail: "StorageClass missing"},
		},
		"explicitly empty pvc storage class is skipped even with a default": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{defaultWFFC},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
		},
		"missing snapshot is pending": {
			pvcs: []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:  []*storagev1.StorageClass{wffc},
			pod:  podWithPVCs("c"),
			want: PodTopology{Relevant: true, Pending: PendingSnapshot, PendingDetail: "VolumeSnapshot default/snap"},
		},
		"missing content is pending": {
			pvcs:  []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:   []*storagev1.StorageClass{wffc},
			snaps: []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			pod:   podWithPVCs("c"),
			want:  PodTopology{Relevant: true, Pending: PendingSnapshot, PendingDetail: "VolumeSnapshotContent content"},
		},
		"content bound to another snapshot name is pending": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "other", terms...)},
			pod:      podWithPVCs("c"),
			want: PodTopology{Relevant: true, Pending: PendingSnapshot,
				PendingDetail: "VolumeSnapshotContent content to bind to VolumeSnapshot default/snap"},
		},
		"content bound to a snapshot in another namespace is pending": {
			pvcs:  []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:   []*storagev1.StorageClass{wffc},
			snaps: []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{func() *snapv1.VolumeSnapshotContent {
				c := contentWithAffinity("content", "snap", terms...)
				c.Spec.VolumeSnapshotRef.Namespace = "other"
				return c
			}()},
			pod: podWithPVCs("c"),
			want: PodTopology{Relevant: true, Pending: PendingSnapshot,
				PendingDetail: "VolumeSnapshotContent content to bind to VolumeSnapshot default/snap"},
		},
		"content bound to a recreated snapshot is pending": {
			pvcs:  []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:   []*storagev1.StorageClass{wffc},
			snaps: []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{func() *snapv1.VolumeSnapshotContent {
				c := contentWithAffinity("content", "snap", terms...)
				c.Spec.VolumeSnapshotRef.UID = "uid-of-a-deleted-snapshot"
				return c
			}()},
			pod: podWithPVCs("c"),
			want: PodTopology{Relevant: true, Pending: PendingSnapshot,
				PendingDetail: "VolumeSnapshotContent content to bind to VolumeSnapshot default/snap"},
		},
		"pre-provisioned content not yet bound back is pending": {
			pvcs:  []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:   []*storagev1.StorageClass{wffc},
			snaps: []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{func() *snapv1.VolumeSnapshotContent {
				c := contentWithAffinity("content", "snap", terms...)
				c.Spec.VolumeSnapshotRef.UID = ""
				return c
			}()},
			pod: podWithPVCs("c"),
			want: PodTopology{Relevant: true, Pending: PendingSnapshot,
				PendingDetail: "VolumeSnapshotContent content to bind to VolumeSnapshot default/snap"},
		},
		"content not ready is pending": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{notReady(contentWithAffinity("content", "snap", terms...))},
			pod:      podWithPVCs("c"),
			want:     PodTopology{Relevant: true, Pending: PendingSnapshot, PendingDetail: "VolumeSnapshotContent content"},
		},
		"one resolved and one pending volume is pending": {
			pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c1", "wffc", "snap", SnapshotAPIGroup)},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c1", "c2"),
			want: PodTopology{
				Relevant: true, Pending: PendingPVC, PendingDetail: "PersistentVolumeClaim default/c2",
				TermSets: [][]v1.TopologySelectorTerm{terms},
			},
		},
		"claim with a selected node still resolves": {
			pvcs:     []*v1.PersistentVolumeClaim{withSelectedNode(snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup), "node-1")},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
			want:     PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}},
		},
		"pvc without storage class waits for assignment": {
			pvcs: []*v1.PersistentVolumeClaim{func() *v1.PersistentVolumeClaim {
				p := snapshotPVC("c", "", "snap", SnapshotAPIGroup)
				p.Spec.StorageClassName = nil
				return p
			}()},
			scs: []*storagev1.StorageClass{defaultWFFC},
			pod: podWithPVCs("c"),
			want: PodTopology{
				Relevant: true, Pending: PendingStorageClass,
				PendingDetail: "StorageClass assignment for PersistentVolumeClaim default/c",
			},
		},
		"pvc restored through dataSourceRef": {
			pvcs: []*v1.PersistentVolumeClaim{func() *v1.PersistentVolumeClaim {
				p := snapshotPVC("c", "wffc", "", "")
				p.Spec.DataSourceRef = &v1.TypedObjectReference{APIGroup: ptr(SnapshotAPIGroup), Kind: "VolumeSnapshot", Name: "snap"}
				return p
			}()},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
			want:     PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}},
		},
		"dataSourceRef naming the claim's own namespace": {
			pvcs: []*v1.PersistentVolumeClaim{func() *v1.PersistentVolumeClaim {
				p := snapshotPVC("c", "wffc", "", "")
				p.Spec.DataSourceRef = &v1.TypedObjectReference{APIGroup: ptr(SnapshotAPIGroup), Kind: "VolumeSnapshot", Name: "snap", Namespace: ptr("default")}
				return p
			}()},
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithPVCs("c"),
			want:     PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}},
		},
		"cross-namespace dataSourceRef is skipped": {
			pvcs: []*v1.PersistentVolumeClaim{func() *v1.PersistentVolumeClaim {
				p := snapshotPVC("c", "wffc", "", "")
				p.Spec.DataSourceRef = &v1.TypedObjectReference{APIGroup: ptr(SnapshotAPIGroup), Kind: "VolumeSnapshot", Name: "snap", Namespace: ptr("other")}
				return p
			}()},
			scs: []*storagev1.StorageClass{wffc},
			pod: podWithPVCs("c"),
		},

		"ephemeral template at admission": {
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      uncreated(podWithEphemeral(ptr("wffc"), "snap")),
			want:     PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}},
		},
		"ephemeral template before its claim is created": {
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      podWithEphemeral(ptr("wffc"), "snap"),
			want:     PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}},
		},
		"ephemeral template with pending snapshot": {
			scs:  []*storagev1.StorageClass{wffc},
			pod:  uncreated(podWithEphemeral(ptr("wffc"), "snap")),
			want: PodTopology{Relevant: true, Pending: PendingSnapshot, PendingDetail: "VolumeSnapshot default/snap"},
		},
		"ephemeral template with a missing named storage class": {
			scs:      []*storagev1.StorageClass{defaultWFFC},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      uncreated(podWithEphemeral(ptr("missing"), "snap")),
			want:     PodTopology{Relevant: true, Pending: PendingStorageClass, PendingDetail: "StorageClass missing"},
		},
		"ephemeral template uses the default storage class": {
			scs:      []*storagev1.StorageClass{defaultWFFC, immediate},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      uncreated(podWithEphemeral(nil, "snap")),
			want:     PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}},
		},
		"ephemeral template with an Immediate default storage class": {
			scs:      []*storagev1.StorageClass{defaultImmediate},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      uncreated(podWithEphemeral(nil, "snap")),
		},
		"ephemeral template without a default storage class": {
			scs:      []*storagev1.StorageClass{wffc},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      uncreated(podWithEphemeral(nil, "snap")),
			want:     PodTopology{Relevant: true, Pending: PendingStorageClass, PendingDetail: "default StorageClass"},
		},
		"ephemeral template with an explicitly empty storage class": {
			scs:      []*storagev1.StorageClass{defaultWFFC},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      uncreated(podWithEphemeral(ptr(""), "snap")),
		},
		"ephemeral template with an Immediate storage class": {
			scs:      []*storagev1.StorageClass{immediate},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			pod:      uncreated(podWithEphemeral(ptr("immediate"), "snap")),
		},
		"ephemeral volume without a template": {
			pod: uncreated(&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "app"},
				Spec: v1.PodSpec{Volumes: []v1.Volume{{
					Name: "eph", VolumeSource: v1.VolumeSource{Ephemeral: &v1.EphemeralVolumeSource{}},
				}}},
			}),
		},
		"non-pvc volume is skipped": {
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "app"},
				Spec: v1.PodSpec{Volumes: []v1.Volume{{
					Name: "scratch", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
				}}},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			r := newTestResolver(tc.pvcs, tc.scs, tc.snaps, tc.contents)
			if got := r.ResolvePod(context.Background(), tc.pod); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ResolvePod() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPVCTopology(t *testing.T) {
	wffc := storageClass("wffc", storagev1.VolumeBindingWaitForFirstConsumer)
	terms := []v1.TopologySelectorTerm{zoneTerm("us-west-2a")}
	ownNamespaceRef := func(namespace string) *v1.PersistentVolumeClaim {
		p := snapshotPVC("c", "wffc", "", "")
		p.Spec.DataSourceRef = &v1.TypedObjectReference{APIGroup: ptr(SnapshotAPIGroup), Kind: "VolumeSnapshot", Name: "snap", Namespace: ptr(namespace)}
		return p
	}

	testcases := map[string]struct {
		pvc      *v1.PersistentVolumeClaim
		scs      []*storagev1.StorageClass
		snaps    []*snapv1.VolumeSnapshot
		contents []*snapv1.VolumeSnapshotContent
		want     ClaimTopology
		wantOK   bool
	}{
		// Resolved topology.
		"constrained claim": {
			pvc:      snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup),
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			want:     ClaimTopology{Content: "content", Terms: terms},
			wantOK:   true,
		},
		"content not ready still reports its current affinity": {
			pvc:      snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup),
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{notReady(contentWithAffinity("content", "snap", terms...))},
			want:     ClaimTopology{Content: "content", Terms: terms},
			wantOK:   true,
		},
		"ready content without affinity imposes no constraint": {
			pvc:      snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup),
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap")},
			want:     ClaimTopology{Content: "content"},
			wantOK:   true,
		},
		"dataSourceRef naming the claim's own namespace": {
			pvc:      ownNamespaceRef("default"),
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
			want:     ClaimTopology{Content: "content", Terms: terms},
			wantOK:   true,
		},

		// Pending topology.
		"content not ready and without affinity yet": {
			pvc:      snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup),
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{notReady(contentWithAffinity("content", "snap"))},
			want:     ClaimTopology{Pending: "VolumeSnapshotContent content"},
			wantOK:   true,
		},
		"snapshot not bound": {
			pvc:    snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup),
			snaps:  []*snapv1.VolumeSnapshot{{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap"}}},
			want:   ClaimTopology{Pending: "VolumeSnapshot default/snap"},
			wantOK: true,
		},
		"snapshot missing": {
			pvc:    snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup),
			want:   ClaimTopology{Pending: "VolumeSnapshot default/snap"},
			wantOK: true,
		},
		"content missing": {
			pvc:    snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup),
			snaps:  []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			want:   ClaimTopology{Pending: "VolumeSnapshotContent content"},
			wantOK: true,
		},
		"content bound to another snapshot": {
			pvc:      snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup),
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "other", terms...)},
			want:     ClaimTopology{Pending: "VolumeSnapshotContent content to bind to VolumeSnapshot default/snap"},
			wantOK:   true,
		},
		"storage class not found": {
			pvc:    snapshotPVC("c", "missing", "snap", SnapshotAPIGroup),
			snaps:  []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			want:   ClaimTopology{Pending: "StorageClass missing"},
			wantOK: true,
		},

		// Not constrained.
		"cross-namespace dataSourceRef": {
			pvc:      ownNamespaceRef("other"),
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
		},
		"bound claim": {
			pvc: func() *v1.PersistentVolumeClaim {
				p := snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)
				p.Spec.VolumeName = "pv"
				return p
			}(),
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
		},
		"Immediate binding": {
			pvc:      snapshotPVC("c", "immediate", "snap", SnapshotAPIGroup),
			scs:      []*storagev1.StorageClass{storageClass("immediate", storagev1.VolumeBindingImmediate)},
			snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
			contents: []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)},
		},
		"claim not restored from a snapshot": {
			pvc: snapshotPVC("c", "wffc", "", ""),
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			r := newTestResolver(nil, append([]*storagev1.StorageClass{wffc}, tc.scs...), tc.snaps, tc.contents)
			got, gotOK := r.PVCTopology(tc.pvc)
			if gotOK != tc.wantOK {
				t.Fatalf("PVCTopology() ok = %v, want %v (%+v)", gotOK, tc.wantOK, got)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("PVCTopology() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveStorageClass(t *testing.T) {
	now := time.Now()
	class := func(name string, created time.Time, annotations map[string]string) *storagev1.StorageClass {
		return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
			Name: name, CreationTimestamp: metav1.NewTime(created), Annotations: annotations,
		}}
	}
	ga := map[string]string{isDefaultStorageClassAnnotation: "true"}
	beta := map[string]string{betaIsDefaultStorageClassAnnotation: "true"}

	testcases := map[string]struct {
		classes []*storagev1.StorageClass
		name    *string
		want    string
	}{
		"explicit name":                 {classes: []*storagev1.StorageClass{class("a", now, nil)}, name: ptr("a"), want: "a"},
		"explicit name not found":       {classes: []*storagev1.StorageClass{class("a", now, ga)}, name: ptr("b")},
		"empty name means no class":     {classes: []*storagev1.StorageClass{class("a", now, ga)}, name: ptr("")},
		"default by GA annotation":      {classes: []*storagev1.StorageClass{class("a", now, nil), class("b", now, ga)}, want: "b"},
		"default by beta annotation":    {classes: []*storagev1.StorageClass{class("a", now, nil), class("b", now, beta)}, want: "b"},
		"annotation not true":           {classes: []*storagev1.StorageClass{class("a", now, map[string]string{isDefaultStorageClassAnnotation: "false"})}},
		"no default":                    {classes: []*storagev1.StorageClass{class("a", now, nil)}},
		"newest default wins":           {classes: []*storagev1.StorageClass{class("old", now.Add(-time.Hour), ga), class("new", now, beta)}, want: "new"},
		"same age resolved by name":     {classes: []*storagev1.StorageClass{class("b", now, ga), class("a", now, ga)}, want: "a"},
		"both annotations on one class": {classes: []*storagev1.StorageClass{class("a", now, map[string]string{isDefaultStorageClassAnnotation: "true", betaIsDefaultStorageClassAnnotation: "true"})}, want: "a"},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got := resolveStorageClass(storagelisters.NewStorageClassLister(newIndexer(tc.classes...)), tc.name)
			gotName := ""
			if got != nil {
				gotName = got.Name
			}
			if gotName != tc.want {
				t.Errorf("resolveStorageClass() = %q, want %q", gotName, tc.want)
			}
		})
	}

	if isWaitForFirstConsumer(&storagev1.StorageClass{}) {
		t.Errorf("isWaitForFirstConsumer() = true for a class without a binding mode")
	}
}

func TestResolvePodLiveClaims(t *testing.T) {
	wffc := storageClass("wffc", storagev1.VolumeBindingWaitForFirstConsumer)
	terms := []v1.TopologySelectorTerm{zoneTerm("us-west-2a")}
	snaps := []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")}
	contents := []*snapv1.VolumeSnapshotContent{contentWithAffinity("content", "snap", terms...)}

	t.Run("claim only in the live read", func(t *testing.T) {
		r := newTestResolver(nil, []*storagev1.StorageClass{wffc}, snaps, contents)
		var calls int
		r.LiveClaims = func(_ context.Context, namespace, name string) (*v1.PersistentVolumeClaim, error) {
			calls++
			return snapshotPVC(name, "wffc", "snap", SnapshotAPIGroup), nil
		}
		want := PodTopology{Relevant: true, TermSets: [][]v1.TopologySelectorTerm{terms}}
		if got := r.ResolvePod(context.Background(), podWithPVCs("c")); !reflect.DeepEqual(got, want) {
			t.Errorf("ResolvePod() = %+v, want %+v", got, want)
		}
		if calls != 1 {
			t.Errorf("live reads = %d, want 1", calls)
		}
	})

	t.Run("cached claim needs no live read", func(t *testing.T) {
		r := newTestResolver([]*v1.PersistentVolumeClaim{snapshotPVC("c", "wffc", "snap", SnapshotAPIGroup)},
			[]*storagev1.StorageClass{wffc}, snaps, contents)
		r.LiveClaims = func(_ context.Context, _, _ string) (*v1.PersistentVolumeClaim, error) {
			t.Error("unexpected live read for a cached claim")
			return nil, errors.New("unexpected")
		}
		if got := r.ResolvePod(context.Background(), podWithPVCs("c")); len(got.TermSets) != 1 {
			t.Errorf("ResolvePod() = %+v", got)
		}
	})

	t.Run("claim that does not exist is pending", func(t *testing.T) {
		r := newTestResolver(nil, []*storagev1.StorageClass{wffc}, snaps, contents)
		r.LiveClaims = func(_ context.Context, namespace, name string) (*v1.PersistentVolumeClaim, error) {
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "persistentvolumeclaims"}, name)
		}
		want := PodTopology{Relevant: true, Pending: PendingPVC, PendingDetail: "PersistentVolumeClaim default/c"}
		if got := r.ResolvePod(context.Background(), podWithPVCs("c")); !reflect.DeepEqual(got, want) {
			t.Errorf("ResolvePod() = %+v, want %+v", got, want)
		}
	})
}
