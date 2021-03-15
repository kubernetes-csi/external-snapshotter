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

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	crdv1beta1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
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

// ValidateV1Beta1Snapshot performs additional strict validation.
// Do NOT rely on this function to fully validate snapshot objects.
// This function will only check the additional rules provided by the webhook.
func ValidateV1Beta1Snapshot(snapshot *crdv1beta1.VolumeSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("VolumeSnapshot is nil")
	}

	source := snapshot.Spec.Source

	if source.PersistentVolumeClaimName != nil && source.VolumeSnapshotContentName != nil {
		return fmt.Errorf("only one of Spec.Source.PersistentVolumeClaimName = %s and Spec.Source.VolumeSnapshotContentName = %s should be set", *source.PersistentVolumeClaimName, *source.VolumeSnapshotContentName)
	}
	if source.PersistentVolumeClaimName == nil && source.VolumeSnapshotContentName == nil {
		return fmt.Errorf("one of Spec.Source.PersistentVolumeClaimName and Spec.Source.VolumeSnapshotContentName should be set")
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

// ValidateV1Beta1SnapshotContent performs additional strict validation.
// Do NOT rely on this function to fully validate snapshot content objects.
// This function will only check the additional rules provided by the webhook.
func ValidateV1Beta1SnapshotContent(snapcontent *crdv1beta1.VolumeSnapshotContent) error {
	if snapcontent == nil {
		return fmt.Errorf("VolumeSnapshotContent is nil")
	}

	source := snapcontent.Spec.Source

	if source.VolumeHandle != nil && source.SnapshotHandle != nil {
		return fmt.Errorf("only one of Spec.Source.VolumeHandle = %s and Spec.Source.SnapshotHandle = %s should be set", *source.VolumeHandle, *source.SnapshotHandle)
	}
	if source.VolumeHandle == nil && source.SnapshotHandle == nil {
		return fmt.Errorf("one of Spec.Source.VolumeHandle and Spec.Source.SnapshotHandle should be set")
	}

	vsref := snapcontent.Spec.VolumeSnapshotRef

	if vsref.Name == "" || vsref.Namespace == "" {
		return fmt.Errorf("both Spec.VolumeSnapshotRef.Name = %s and Spec.VolumeSnapshotRef.Namespace = %s must be set", vsref.Name, vsref.Namespace)
	}

	return nil
}
