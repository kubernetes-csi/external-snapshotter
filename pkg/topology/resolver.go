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

// Package topology resolves snapshot restore constraints (KEP-5943) and merges
// them into pod node affinity.
package topology

import (
	"context"
	"fmt"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/component-helpers/storage/ephemeral"

	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
)

const (
	SnapshotAPIGroup = "snapshot.storage.k8s.io"

	// SchedulingGate blocks scheduling until snapshot topology is resolved.
	SchedulingGate = "snapshot.storage.k8s.io/topology"
	// ManagedLabel selects gated pods for the controller's informer.
	ManagedLabel = "snapshot.storage.k8s.io/topology-managed"
	// SelectedNodeAnnotation records the scheduler's node choice for WFFC claims.
	SelectedNodeAnnotation = "volume.kubernetes.io/selected-node"
)

// PendingReason explains why the topology of a pod is not known yet.
type PendingReason string

const (
	PendingPVC          PendingReason = "WaitingForPVC"
	PendingSnapshot     PendingReason = "WaitingForSnapshot"
	PendingStorageClass PendingReason = "WaitingForStorageClass"
)

// Resolver resolves snapshot topology from informer caches.
type Resolver struct {
	PVCs           corelisters.PersistentVolumeClaimLister
	StorageClasses storagelisters.StorageClassLister
	Snapshots      snapshotlisters.VolumeSnapshotLister
	Contents       snapshotlisters.VolumeSnapshotContentLister
	// LiveClaims optionally bypasses cache lag; without it, missing claims stay pending.
	LiveClaims func(ctx context.Context, namespace, name string) (*v1.PersistentVolumeClaim, error)
}

// PodTopology is the snapshot topology that applies to a pod.
type PodTopology struct {
	// Relevant includes unresolved claims/classes and unbound WFFC snapshot claims.
	Relevant bool
	// PendingDetail identifies the unresolved object when Pending is set.
	Pending       PendingReason
	PendingDetail string
	// TermSets are ANDed; each content's terms are ORed.
	TermSets [][]v1.TopologySelectorTerm
}

func (t *PodTopology) setPending(reason PendingReason, detail string) {
	t.Relevant = true
	if t.Pending == "" {
		t.Pending = reason
		t.PendingDetail = detail
	}
}

// ResolvePod returns restore constraints; unready contents remain pending.
// Snapshot creation may still be recording their topology.
func (r *Resolver) ResolvePod(ctx context.Context, pod *v1.Pod) PodTopology {
	var out PodTopology
	for i := range pod.Spec.Volumes {
		r.resolveVolume(ctx, pod, &pod.Spec.Volumes[i], &out)
	}
	return out
}

func (r *Resolver) claim(ctx context.Context, namespace, name string) (*v1.PersistentVolumeClaim, error) {
	pvc, err := r.PVCs.PersistentVolumeClaims(namespace).Get(name)
	if err == nil || r.LiveClaims == nil {
		return pvc, err
	}
	live, liveErr := r.LiveClaims(ctx, namespace, name)
	if liveErr != nil {
		return nil, err
	}
	return live, nil
}

func (r *Resolver) resolveVolume(ctx context.Context, pod *v1.Pod, vol *v1.Volume, out *PodTopology) {
	switch {
	case vol.PersistentVolumeClaim != nil:
		name := vol.PersistentVolumeClaim.ClaimName
		pvc, err := r.claim(ctx, pod.Namespace, name)
		if err != nil {
			// Until the claim exists, its snapshot source is unknown.
			out.setPending(PendingPVC, fmt.Sprintf("PersistentVolumeClaim %s/%s", pod.Namespace, name))
			return
		}
		r.resolveClaim(pvc, out)

	case vol.Ephemeral != nil:
		// Admission precedes ephemeral PVC creation; resolve from the template.
		if pod.UID != "" {
			pvc, err := r.claim(ctx, pod.Namespace, ephemeral.VolumeClaimName(pod, vol))
			if err == nil {
				if ephemeral.VolumeIsForPod(pod, pvc) == nil {
					r.resolveClaim(pvc, out)
				}
				return
			}
		}
		tmpl := vol.Ephemeral.VolumeClaimTemplate
		if tmpl == nil {
			return
		}
		snapshotName := SnapshotSource(pod.Namespace, tmpl.Spec.DataSource, tmpl.Spec.DataSourceRef)
		if snapshotName == "" {
			return
		}
		wffc, missing := r.waitForFirstConsumer(tmpl.Spec.StorageClassName)
		if missing != "" {
			out.setPending(PendingStorageClass, missing)
			return
		}
		if !wffc {
			return
		}
		out.Relevant = true
		r.resolveSnapshot(pod.Namespace, snapshotName, out)
	}
}

func (r *Resolver) resolveClaim(pvc *v1.PersistentVolumeClaim, out *PodTopology) {
	snapshotName, missing := r.constrainedClaim(pvc)
	if missing != "" {
		out.setPending(PendingStorageClass, missing)
		return
	}
	if snapshotName == "" {
		return
	}
	out.Relevant = true
	r.resolveSnapshot(pvc.Namespace, snapshotName, out)
}

// constrainedClaim returns a snapshot name or a missing-StorageClass detail.
func (r *Resolver) constrainedClaim(pvc *v1.PersistentVolumeClaim) (string, string) {
	if pvc.Spec.VolumeName != "" {
		return "", ""
	}
	snapshotName := SnapshotSource(pvc.Namespace, pvc.Spec.DataSource, pvc.Spec.DataSourceRef)
	if snapshotName == "" {
		return "", ""
	}
	if pvc.Spec.StorageClassName == nil {
		return "", fmt.Sprintf("StorageClass assignment for PersistentVolumeClaim %s/%s", pvc.Namespace, pvc.Name)
	}
	wffc, missing := r.waitForFirstConsumer(pvc.Spec.StorageClassName)
	if !wffc {
		return "", missing
	}
	return snapshotName, ""
}

func (r *Resolver) waitForFirstConsumer(name *string) (bool, string) {
	if name != nil && *name == "" {
		return false, ""
	}
	sc := resolveStorageClass(r.StorageClasses, name)
	if sc == nil {
		if name == nil {
			return false, "default StorageClass"
		}
		return false, fmt.Sprintf("StorageClass %s", *name)
	}
	return isWaitForFirstConsumer(sc), ""
}

func (r *Resolver) resolveSnapshot(namespace, name string, out *PodTopology) {
	content, missing := r.boundContent(namespace, name)
	if content == nil {
		out.setPending(PendingSnapshot, missing)
		return
	}
	if !contentReady(content) {
		out.setPending(PendingSnapshot, fmt.Sprintf("VolumeSnapshotContent %s", content.Name))
		return
	}
	if len(content.Spec.NodeAffinity) > 0 {
		out.TermSets = append(out.TermSets, content.Spec.NodeAffinity)
	}
}

// boundContent returns the content or a missing-object detail.
func (r *Resolver) boundContent(namespace, name string) (*snapv1.VolumeSnapshotContent, string) {
	snapshot, err := r.Snapshots.VolumeSnapshots(namespace).Get(name)
	if err != nil || !utils.IsBoundVolumeSnapshotContentNameSet(snapshot) {
		return nil, fmt.Sprintf("VolumeSnapshot %s/%s", namespace, name)
	}
	contentName := *snapshot.Status.BoundVolumeSnapshotContentName
	content, err := r.Contents.Get(contentName)
	if err != nil {
		return nil, fmt.Sprintf("VolumeSnapshotContent %s", contentName)
	}
	if !utils.IsVolumeSnapshotRefSet(snapshot, content) {
		return nil, fmt.Sprintf("VolumeSnapshotContent %s to bind to VolumeSnapshot %s/%s", contentName, namespace, name)
	}
	return content, ""
}

// ClaimTopology is the restore topology of an unbound WaitForFirstConsumer
// claim restored from a snapshot.
type ClaimTopology struct {
	// Pending names the object the topology is waiting for. Content and Terms
	// are only set once it is empty.
	Pending string
	// Content is the bound VolumeSnapshotContent, and Terms its current
	// NodeAffinity. Empty Terms mean no constraint.
	Content string
	Terms   []v1.TopologySelectorTerm
}

// PVCTopology returns the restore topology of a claim.
func (r *Resolver) PVCTopology(pvc *v1.PersistentVolumeClaim) (topo ClaimTopology, ok bool) {
	snapshotName, missing := r.constrainedClaim(pvc)
	if snapshotName == "" {
		if missing == "" {
			return ClaimTopology{}, false
		}
		return ClaimTopology{Pending: missing}, true
	}
	c, missing := r.boundContent(pvc.Namespace, snapshotName)
	if c == nil {
		return ClaimTopology{Pending: missing}, true
	}
	// The sidecar records the topology while the snapshot is created, so empty
	// NodeAffinity only means no constraint once the content is ready.
	if len(c.Spec.NodeAffinity) == 0 && !contentReady(c) {
		return ClaimTopology{Pending: fmt.Sprintf("VolumeSnapshotContent %s", c.Name)}, true
	}
	return ClaimTopology{Content: c.Name, Terms: c.Spec.NodeAffinity}, true
}

func contentReady(c *snapv1.VolumeSnapshotContent) bool {
	return c.Status != nil && c.Status.ReadyToUse != nil && *c.Status.ReadyToUse
}

// SnapshotSource returns the name of the VolumeSnapshot in namespace that a
// claim in namespace restores from, or "" if there is none. A dataSourceRef
// may name its own namespace explicitly; one naming another namespace (the
// alpha CrossNamespaceVolumeDataSource feature) is not supported.
func SnapshotSource(namespace string, ds *v1.TypedLocalObjectReference, ref *v1.TypedObjectReference) string {
	if ds != nil {
		if ds.Kind == "VolumeSnapshot" && ds.APIGroup != nil && *ds.APIGroup == SnapshotAPIGroup {
			return ds.Name
		}
		return ""
	}
	if ref != nil && ref.Kind == "VolumeSnapshot" && ref.APIGroup != nil && *ref.APIGroup == SnapshotAPIGroup &&
		(ref.Namespace == nil || *ref.Namespace == "" || *ref.Namespace == namespace) {
		return ref.Name
	}
	return ""
}
