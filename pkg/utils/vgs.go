/*
Copyright 2024 The Kubernetes Authors.

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

package utils

import (
	"fmt"

	groupsnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// VolumeSnapshotParentGroupIndex is the name of the cache index hosting the relationship
// between volume snapshots and their volume group snapshot owner
const VolumeSnapshotParentGroupIndex = "ByVolumeGroupSnapshotMembership"

// volumeGroupSnapshotKind is the kind of the VolumeGroupSnapshot objects.
const volumeGroupSnapshotKind = "VolumeGroupSnapshot"

// GetVolumeGroupSnapshotParentOwnerReference returns the owner reference pointing to
// the parent group snapshot, or nil when the passed snapshot is not a member of a
// volume group snapshot.
// Only the API group is matched, not the version, so that members owned by a
// previous version of the group snapshot API are still recognized.
func GetVolumeGroupSnapshotParentOwnerReference(snapshot *crdv1.VolumeSnapshot) *metav1.OwnerReference {
	if snapshot == nil {
		return nil
	}

	for i := range snapshot.ObjectMeta.OwnerReferences {
		owner := &snapshot.ObjectMeta.OwnerReferences[i]
		if owner.Kind != volumeGroupSnapshotKind {
			continue
		}
		if schema.FromAPIVersionAndKind(owner.APIVersion, owner.Kind).Group == groupsnapshotv1.SchemeGroupVersion.Group {
			return owner
		}
	}

	return nil
}

// getVolumeGroupSnapshotParentObjectName returns the name of the parent group snapshot,
// or an empty string when the passed snapshot is not a member of a volume group snapshot.
func getVolumeGroupSnapshotParentObjectName(snapshot *crdv1.VolumeSnapshot) string {
	if owner := GetVolumeGroupSnapshotParentOwnerReference(snapshot); owner != nil {
		return owner.Name
	}

	return ""
}

// IsVolumeGroupSnapshotMember returns true if the passed VolumeSnapshot object
// is a member of a VolumeGroupSnapshot.
func IsVolumeGroupSnapshotMember(snapshot *crdv1.VolumeSnapshot) bool {
	parentName := getVolumeGroupSnapshotParentObjectName(snapshot)
	return len(parentName) > 0
}

// VolumeSnapshotParentGroupKeyFunc maps a member snapshot to the name
// of the parent VolumeGroupSnapshot
func VolumeSnapshotParentGroupKeyFunc(snapshot *crdv1.VolumeSnapshot) string {
	parentName := getVolumeGroupSnapshotParentObjectName(snapshot)
	if len(parentName) == 0 {
		return ""
	}

	return VolumeSnapshotParentGroupKeyFuncByComponents(types.NamespacedName{
		Namespace: snapshot.Namespace,
		Name:      parentName,
	})
}

// VolumeSnapshotParentGroupKeyFuncByComponents computes the index key for a certain
// name and namespace pair
func VolumeSnapshotParentGroupKeyFuncByComponents(objectKey types.NamespacedName) string {
	return fmt.Sprintf("%s^%s", objectKey.Namespace, objectKey.Name)
}

// NeedToAddVolumeGroupSnapshotOwnership checks if the passed snapshot is a member
// of a volume group snapshot but the ownership is missing
func NeedToAddVolumeGroupSnapshotOwnership(snapshot *crdv1.VolumeSnapshot) bool {
	parentObjectName := getVolumeGroupSnapshotParentObjectName(snapshot)
	if len(parentObjectName) > 0 {
		return false
	}

	// Group snapshot ownership should be added only when
	// the snapshot is marked with its group snapshot name
	if snapshot == nil ||
		snapshot.Status == nil ||
		snapshot.Status.VolumeGroupSnapshotName == nil ||
		len(*snapshot.Status.VolumeGroupSnapshotName) == 0 {
		return false
	}

	return true
}

// BuildVolumeGroupSnapshotOwnerReference creates a OwnerReference record declaring an
// object as a child of passed VolumeGroupSnapshot
func BuildVolumeGroupSnapshotOwnerReference(parentGroup *groupsnapshotv1.VolumeGroupSnapshot) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: fmt.Sprintf(
			"%s/%s",
			groupsnapshotv1.SchemeGroupVersion.Group,
			groupsnapshotv1.SchemeGroupVersion.Version,
		),
		Kind: volumeGroupSnapshotKind,
		Name: parentGroup.Name,
		UID:  parentGroup.UID,
	}
}
