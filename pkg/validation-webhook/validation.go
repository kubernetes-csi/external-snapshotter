/*
Copyright 2021 The Kubernetes Authors.

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

package webhook

import (
	"fmt"

	groupsnapshotcrdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumegroupsnapshot/v1alpha1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
)

// ValidateV1Snapshot performs additional strict validation.
// Do NOT rely on this function to fully validate snapshot objects.
// This function will only check the additional rules provided by the webhook.
func ValidateV1Snapshot(snapshot *crdv1.VolumeSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("VolumeSnapshot is nil")
	}

	vscname := snapshot.Spec.VolumeSnapshotClassName
	if vscname != nil && *vscname == "" {
		return fmt.Errorf("Spec.VolumeSnapshotClassName must not be the empty string")
	}
	return nil
}

// ValidateV1SnapshotContent performs additional strict validation.
// Do NOT rely on this function to fully validate snapshot content objects.
// This function will only check the additional rules provided by the webhook.
func ValidateV1SnapshotContent(snapcontent *crdv1.VolumeSnapshotContent) error {
	if snapcontent == nil {
		return fmt.Errorf("VolumeSnapshotContent is nil")
	}

	vsref := snapcontent.Spec.VolumeSnapshotRef

	if vsref.Name == "" || vsref.Namespace == "" {
		return fmt.Errorf("both Spec.VolumeSnapshotRef.Name = %s and Spec.VolumeSnapshotRef.Namespace = %s must be set", vsref.Name, vsref.Namespace)
	}

	return nil
}

// ValidateV1Alpha1GroupSnapshotContent performs additional strict validation.
// Do NOT rely on this function to fully validate group snapshot content objects.
// This function will only check the additional rules provided by the webhook.
func ValidateV1Alpha1GroupSnapshotContent(groupSnapcontent *groupsnapshotcrdv1alpha1.VolumeGroupSnapshotContent) error {
	if groupSnapcontent == nil {
		return fmt.Errorf("VolumeGroupSnapshotContent is nil")
	}

	vgsref := groupSnapcontent.Spec.VolumeGroupSnapshotRef

	if vgsref.Name == "" || vgsref.Namespace == "" {
		return fmt.Errorf("both Spec.VolumeGroupSnapshotRef.Name = %s and Spec.VolumeGroupSnapshotRef.Namespace = %s must be set", vgsref.Name, vgsref.Namespace)
	}

	return nil
}

// ValidateV1Alpha1GroupSnapshot performs additional strict validation.
// Do NOT rely on this function to fully validate group snapshot objects.
// This function will only check the additional rules provided by the webhook.
func ValidateV1Alpha1GroupSnapshot(snapshot *groupsnapshotcrdv1alpha1.VolumeGroupSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("VolumeGroupSnapshot is nil")
	}

	vgscname := snapshot.Spec.VolumeGroupSnapshotClassName
	if vgscname != nil && *vgscname == "" {
		return fmt.Errorf("Spec.VolumeGroupSnapshotClassName must not be the empty string")
	}
	return nil
}
