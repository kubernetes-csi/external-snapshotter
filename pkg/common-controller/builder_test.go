/*
Copyright 2025 The Kubernetes Authors.

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

package common_controller

import (
	crdv1beta1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// VolumeGroupSnapshotBuilder is used by the volume group snapshots
// unit tests to build VolumeGroupSnapshot objects
type VolumeGroupSnapshotBuilder struct {
	name string
	uid  types.UID

	deletionTimestamp      *metav1.Time
	groupSnapshotClassName string
	targetContentName      string
	selectors              map[string]string
	finalizers             []string

	nilStatus              bool
	statusBoundContentName string
	statusError            string
	statusReadyToUse       *bool
	statusCreationTime     *metav1.Time
}

// NewVolumeGroupSnapshotBuilder creates a new builder with the required
// information
func NewVolumeGroupSnapshotBuilder(name string, uid types.UID) *VolumeGroupSnapshotBuilder {
	return &VolumeGroupSnapshotBuilder{
		name: name,
		uid:  uid,
	}
}

// WithDeletionTimestamp sets the deletion timestamp in the builder
func (b *VolumeGroupSnapshotBuilder) WithDeletionTimestamp(t metav1.Time) *VolumeGroupSnapshotBuilder {
	b.deletionTimestamp = &t
	return b
}

// WithSelectors set the selectors inside the builder
func (b *VolumeGroupSnapshotBuilder) WithSelectors(s map[string]string) *VolumeGroupSnapshotBuilder {
	b.selectors = s
	return b
}

// WithGroupSnapshotClass set the group snapshot class inside the buileder
func (b *VolumeGroupSnapshotBuilder) WithGroupSnapshotClass(n string) *VolumeGroupSnapshotBuilder {
	b.groupSnapshotClassName = n
	return b
}

// WithTargetContentName sets the target volume group snapshot content name
func (b *VolumeGroupSnapshotBuilder) WithTargetContentName(n string) *VolumeGroupSnapshotBuilder {
	b.targetContentName = n
	return b
}

// WithStatus pre-sets the status in the built object
func (b *VolumeGroupSnapshotBuilder) WithNilStatus() *VolumeGroupSnapshotBuilder {
	b.nilStatus = true
	return b
}

// WithBoundContentName sets the bound content name in the volume group snapshot object
func (b *VolumeGroupSnapshotBuilder) WithStatusBoundContentName(n string) *VolumeGroupSnapshotBuilder {
	b.statusBoundContentName = n
	return b
}

// WithStatusError sets the status.error field in the generated object
func (b *VolumeGroupSnapshotBuilder) WithStatusError(e string) *VolumeGroupSnapshotBuilder {
	b.statusError = e
	return b
}

// WithStatusReadyToUse sets the ready to use boolean indicator to the specified value
func (b *VolumeGroupSnapshotBuilder) WithStatusReadyToUse(readyToUse bool) *VolumeGroupSnapshotBuilder {
	b.statusReadyToUse = &readyToUse
	return b
}

// WithStatusCreationTime sets the status creation time in the generated object
func (b *VolumeGroupSnapshotBuilder) WithStatusCreationTime(t metav1.Time) *VolumeGroupSnapshotBuilder {
	b.statusCreationTime = &t
	return b
}

// WithFinalizers sets all the finalizers in the generated object
func (b *VolumeGroupSnapshotBuilder) WithAllFinalizers() *VolumeGroupSnapshotBuilder {
	b.finalizers = []string{
		utils.VolumeGroupSnapshotContentFinalizer,
		utils.VolumeGroupSnapshotBoundFinalizer,
	}
	return b
}

// WithFinalizers sets the passed finalizers in the generated object
func (b *VolumeGroupSnapshotBuilder) WithFinalizers(finalizers ...string) *VolumeGroupSnapshotBuilder {
	b.finalizers = finalizers
	return b
}

// Build builds a volume group snapshot
func (b *VolumeGroupSnapshotBuilder) Build() *crdv1beta1.VolumeGroupSnapshot {
	groupSnapshot := crdv1beta1.VolumeGroupSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:              b.name,
			Namespace:         testNamespace,
			UID:               b.uid,
			ResourceVersion:   "1",
			SelfLink:          "/apis/groupsnapshot.storage.k8s.io/v1beta1/namespaces/" + testNamespace + "/volumesnapshots/" + b.name,
			DeletionTimestamp: b.deletionTimestamp,
		},
		Spec: crdv1beta1.VolumeGroupSnapshotSpec{
			VolumeGroupSnapshotClassName: nil,
		},
	}

	if len(b.selectors) > 0 {
		groupSnapshot.Spec.Source.Selector = &metav1.LabelSelector{
			MatchLabels: b.selectors,
		}
	}

	if !b.nilStatus {
		groupSnapshot.Status = &crdv1beta1.VolumeGroupSnapshotStatus{
			CreationTime: b.statusCreationTime,
			ReadyToUse:   b.statusReadyToUse,
		}

		if b.statusError != "" {
			groupSnapshot.Status.Error = newVolumeError(b.statusError)
		}

		if b.statusBoundContentName != "" {
			groupSnapshot.Status.BoundVolumeGroupSnapshotContentName = &b.statusBoundContentName
		}
	}

	if b.groupSnapshotClassName != "" {
		groupSnapshot.Spec.VolumeGroupSnapshotClassName = &b.groupSnapshotClassName
	}

	if b.targetContentName != "" {
		groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName = &b.targetContentName
	}

	if b.finalizers != nil {
		groupSnapshot.ObjectMeta.Finalizers = b.finalizers
	}

	return &groupSnapshot
}

// BuildArray builds an array of a volume group snapshot with the specified properties
func (b *VolumeGroupSnapshotBuilder) BuildArray() []*crdv1beta1.VolumeGroupSnapshot {
	return []*crdv1beta1.VolumeGroupSnapshot{
		b.Build(),
	}
}

// VolumeGroupSnapshotContentBuilder is used by the volume group snapshots
// unit tests to build VolumeGroupSnapshotContent objects
type VolumeGroupSnapshotContentBuilder struct {
	name string

	annotations map[string]string
	finalizer   bool

	deletionPolicy            crdv1.DeletionPolicy
	groupSnapshotClassName    string
	targetGroupSnapshotHandle string
	desiredVolumeHandles      []string
	boundGroupSnapshotName    string
	boundGroupSnapshotUID     types.UID

	setStatus                 bool
	statusCreationTime        *metav1.Time
	statusGroupSnapshotHandle string
}

// NewVolumeGroupSnapshotContentBuilder creates a new helper to create a volume group snapshot content
// object
func NewVolumeGroupSnapshotContentBuilder(name string) *VolumeGroupSnapshotContentBuilder {
	return &VolumeGroupSnapshotContentBuilder{
		name: name,
	}
}

// WithAnnotation attaches an annotation to the built object
func (b *VolumeGroupSnapshotContentBuilder) WithAnnotation(name, value string) *VolumeGroupSnapshotContentBuilder {
	if b.annotations == nil {
		b.annotations = make(map[string]string)
	}

	b.annotations[name] = value
	return b
}

// WithGroupSnapshotClassName sets the group snapshot class name to be used
// in the built object
func (b *VolumeGroupSnapshotContentBuilder) WithGroupSnapshotClassName(name string) *VolumeGroupSnapshotContentBuilder {
	b.groupSnapshotClassName = name
	return b
}

// WithTargetGroupSnapshotHandle sets the group snapshot handle to be used
// in the built object
func (b *VolumeGroupSnapshotContentBuilder) WithTargetGroupSnapshotHandle(handle string) *VolumeGroupSnapshotContentBuilder {
	b.targetGroupSnapshotHandle = handle
	return b
}

// WithDeletionPolicy sets the deletion policy in the generated object
func (b *VolumeGroupSnapshotContentBuilder) WithDeletionPolicy(p crdv1.DeletionPolicy) *VolumeGroupSnapshotContentBuilder {
	b.deletionPolicy = p
	return b
}

// WithBoundGroupSnapshot sets the pointer to the bound group snapshot object
func (b *VolumeGroupSnapshotContentBuilder) WithBoundGroupSnapshot(name string, uid types.UID) *VolumeGroupSnapshotContentBuilder {
	b.boundGroupSnapshotName = name
	b.boundGroupSnapshotUID = uid
	return b
}

// WithDesiredVolumeHandles sets the desired volume handles
func (b *VolumeGroupSnapshotContentBuilder) WithDesiredVolumeHandles(handles ...string) *VolumeGroupSnapshotContentBuilder {
	b.desiredVolumeHandles = handles
	return b
}

// WithStatus fills the status subresource and mark the generated object
// as ready to use
func (b *VolumeGroupSnapshotContentBuilder) WithStatus() *VolumeGroupSnapshotContentBuilder {
	b.setStatus = true
	return b
}

// WithStatusGroupSnapshotHandle sets the group snapshot handle inside the status section
func (b *VolumeGroupSnapshotContentBuilder) WithStatusGroupSnapshotHandle(handle string) *VolumeGroupSnapshotContentBuilder {
	b.statusGroupSnapshotHandle = handle
	return b
}

// WithCreationTime sets the creation time in the generated object
func (b *VolumeGroupSnapshotContentBuilder) WithCreationTime(t metav1.Time) *VolumeGroupSnapshotContentBuilder {
	b.statusCreationTime = &t
	return b
}

// WithFinalizer sets the finalizer in the created object
func (b *VolumeGroupSnapshotContentBuilder) WithFinalizer() *VolumeGroupSnapshotContentBuilder {
	return b
}

// Build builds the object
func (b *VolumeGroupSnapshotContentBuilder) Build() *crdv1beta1.VolumeGroupSnapshotContent {
	content := crdv1beta1.VolumeGroupSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:            b.name,
			ResourceVersion: "1",
			Annotations:     b.annotations,
		},
		Spec: crdv1beta1.VolumeGroupSnapshotContentSpec{
			Driver:         mockDriverName,
			DeletionPolicy: b.deletionPolicy,
		},
	}

	if b.setStatus {
		ready := true
		content.Status = &crdv1beta1.VolumeGroupSnapshotContentStatus{
			CreationTime: b.statusCreationTime,
			ReadyToUse:   &ready,
		}
	}

	if b.setStatus && b.statusGroupSnapshotHandle != "" {
		content.Status.VolumeGroupSnapshotHandle = &b.statusGroupSnapshotHandle
	}

	if b.groupSnapshotClassName != "" {
		content.Spec.VolumeGroupSnapshotClassName = &b.groupSnapshotClassName
	}

	if b.targetGroupSnapshotHandle != "" {
		content.Spec.Source.GroupSnapshotHandles = &crdv1beta1.GroupSnapshotHandles{
			VolumeGroupSnapshotHandle: b.targetGroupSnapshotHandle,
		}
	}

	if len(b.desiredVolumeHandles) != 0 {
		content.Spec.Source.VolumeHandles = b.desiredVolumeHandles
	}

	if b.boundGroupSnapshotName != "" {
		content.Spec.VolumeGroupSnapshotRef = v1.ObjectReference{
			Kind:            "VolumeGroupSnapshot",
			APIVersion:      "groupsnapshot.storage.k8s.io/v1beta1",
			UID:             b.boundGroupSnapshotUID,
			Namespace:       testNamespace,
			Name:            b.boundGroupSnapshotName,
			ResourceVersion: "1",
		}
	}

	if b.finalizer {
		content.ObjectMeta.Finalizers = append(content.ObjectMeta.Finalizers, utils.VolumeGroupSnapshotContentFinalizer)
	}

	return &content
}

// BuildArray builds an array of a volume group snapshot with the specified properties
func (b *VolumeGroupSnapshotContentBuilder) BuildArray() []*crdv1beta1.VolumeGroupSnapshotContent {
	return []*crdv1beta1.VolumeGroupSnapshotContent{
		b.Build(),
	}
}
