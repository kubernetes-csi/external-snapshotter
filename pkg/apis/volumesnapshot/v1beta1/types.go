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

package v1beta1

import (
	core_v1 "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// VolumeSnapshotContentResourcePlural is "volumesnapshotcontents"
	VolumeSnapshotContentResourcePlural = "volumesnapshotcontents"
	// VolumeSnapshotResourcePlural is "volumesnapshots"
	VolumeSnapshotResourcePlural = "volumesnapshots"
	// VolumeSnapshotClassResourcePlural is "volumesnapshotclasses"
	VolumeSnapshotClassResourcePlural = "volumesnapshotclasses"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshot is a user's request for taking a point in time snapshot of a volume.
// Upon successful creation of the snapshot by the volume provider, it is bound to a
// corresponding VolumeSnapshotContent.
// VolumeSnapshot objects are namespaced
type VolumeSnapshot struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the desired characteristics of a snapshot requested by a user.
	// More info: https:s//kubernetes.io/docs/concepts/storage/volume-snapshots#volumesnapshots
	// +optional
	Spec VolumeSnapshotSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Status represents the current status/information of a volume snapshot.
	// NOTE: Status is subjected to change by sources other than snapshot controller,
	//       for example, undesired corruption from human operation errors. It is only informational to
	//       provide necessary transparency of a snapshot's status to users. It is highly recommended
	//       not to rely on this piece of information programmatically.
	// +optional
	Status VolumeSnapshotStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotList is a list of VolumeSnapshot objects
type VolumeSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of VolumeSnapshots
	Items []VolumeSnapshot `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// VolumeSnapshotSpec describes the common attributes of a volume snapshot
type VolumeSnapshotSpec struct {
	// Source has the information about where a snapshot should be created from.
	// Currently PersistentVolumeClaim is the only supported source type.
	// If specified, and VolumeSnapshotContentName is not specified (i.e., nil),
	// snapshot will be dynamically created from the given source.
	// If not specified, user can statically bind a VolumeSnapshot to a
	// VolumeSnapshotContent by specifying VolumeSnapshotContentName.
	// +optional
	Source *core_v1.TypedLocalObjectReference `json:"source,omitempty" protobuf:"bytes,1,opt,name=source"`

	// VolumeSnapshotContentName is the binding reference to the VolumeSnapshotContent backing the VolumeSnapshot.
	// In dynamic snapshot creation case, VolumeSnapshotContentName must NOT be specified.
	// +optional
	VolumeSnapshotContentName *string `json:"volumeSnapshotContentName,omitempty" protobuf:"bytes,2,opt,name=volumeSnapshotContentName"`

	// Name of the VolumeSnapshotClass requested by the VolumeSnapshot.
	// If not specified, the default snapshot class will be used if there exists one.
	// More info: https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes
	// +optional
	VolumeSnapshotClassName *string `json:"volumeSnapshotClassName,omitempty" protobuf:"bytes,3,opt,name=volumeSnapshotClassName"`
}

// VolumeSnapshotStatus is the status of the VolumeSnapshot
type VolumeSnapshotStatus struct {
	// NOTE: All fields in VolumeSnapshotStatus are informational for user references.
	//       Controllers MUST NOT rely on any fields programmatically.

	// CreationTime, if not nil, represents the timestamp when a snapshot was successfully
	// cut by the underlying storage system. In static binding, CreationTime might not be available.
	// +optional
	CreationTime *metav1.Time `json:"creationTime,omitempty" protobuf:"bytes,1,opt,name=creationTime"`

	// ReadyToUse is a status/informational flag which provides transparency to users.
	// In the dynamic snapshot creation case, ReadyToUse will be set to true when underlying storage
	// system has successfully finished all procedures out-of-bound to make a snapshot available AND
	// snapshot controller has bound the VolumeSnapshot to a VolumeSnapshotContent successfully.
	ReadyToUse bool `json:"readyToUse" protobuf:"varint,3,opt,name=readyToUse"`

	// RestoreSize, if not nil, represents the minimum volume size to restore from a VolumeSnapshot
	// It is a storage system level property of a snapshot when the underlying storage system supports.
	// The field could be nil if the underlying storage system does not have the information available,
	// , or in cases like manual/static binding.
	// +optional
	RestoreSize *resource.Quantity `json:"restoreSize,omitempty" protobuf:"bytes,2,opt,name=restoreSize"`

	// The lastest observed error during snapshot creation operation, if any.
	// +optional
	Error *storage.VolumeError `json:"error,omitempty" protobuf:"bytes,4,opt,name=error,casttype=VolumeError"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotClass describes the parameters for a class of storage snapshotter
// for which VolumeSnapshot can be dynamically taken for a given PersistentVolumeClaim
// VolumeSnapshotClasses are non-namespaced.
// The name of a VolumeSnapshotClass object is significant, it serves as the unique identifier
// for a user to request a snapshot to be created using the specific VolumeSnapshotClass
type VolumeSnapshotClass struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Snapshotter is the name of the driver expected to handle VolumeSnapshot requests of this VolumeSnapshotClass.
	Snapshotter string `json:"snapshotter" protobuf:"bytes,2,opt,name=snapshotter"`

	// Parameters holds parameters for underlying storage system.
	// These values are opaque to Kubernetes.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty" protobuf:"bytes,3,rep,name=parameters"`

	// DeletionPolicy defines whether a VolumeSnapshotContent and its
	// associated physical snapshot on underlying storage system
	// should be deleted or not when released from its corresponding VolumeSnapshot.
	// If not specified, the default will be VolumeSnapshotContentRetain for static binding,
	// and VolumeSnapshotContentDelete for dynamic snapshot creation.
	// +optional
	DeletionPolicy *DeletionPolicy `json:"deletionPolicy,omitempty" protobuf:"bytes,4,opt,name=deletionPolicy"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotClassList is a collection of VolumeSnapshotClasses.
type VolumeSnapshotClassList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is the list of VolumeSnapshotClasses
	Items []VolumeSnapshotClass `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotContent represents the actual "on-disk" snapshot object in the
// underlying storage system
type VolumeSnapshotContent struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec represents the desired state of the snapshot content
	Spec VolumeSnapshotContentSpec `json:"spec" protobuf:"bytes,2,opt,name=spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotContentList is a list of VolumeSnapshotContent objects
type VolumeSnapshotContentList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is the list of VolumeSnapshotContents
	Items []VolumeSnapshotContent `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// VolumeSnapshotContentSpec is the specification of a VolumeSnapshotContent
type VolumeSnapshotContentSpec struct {
	// VolumeSnapshotSource holds critical metadata where the snapshot on underlying storage
	// system has been created from.
	VolumeSnapshotSource `json:",inline" protobuf:"bytes,1,opt,name=volumeSnapshotSource"`

	// VolumeSnapshotRef is part of bi-directional binding between VolumeSnapshot
	// and VolumeSnapshotContent.
	// Expect to be non-nil when bound.
	// VolumeSnapshot.VolumeSnapshotContentName is the authoritative bind between VolumeSnapshot and VolumeSnapshotContent
	// +optional
	VolumeSnapshotRef *core_v1.ObjectReference `json:"volumeSnapshotRef,omitempty" protobuf:"bytes,2,opt,name=volumeSnapshotRef"`

	// PersistentVolumeRef represents the PersistentVolume that the snapshot has been taken from.
	// In dynamic snapshot creation case, the field will be specified when VolumeSnapshot and VolumeSnapshotContent are bound.
	// +optional
	PersistentVolumeRef *core_v1.ObjectReference `json:"persistentVolumeRef,omitempty" protobuf:"bytes,3,opt,name=persistentVolumeRef"`

	// DeletionPolicy defines whether a VolumeSnapshotContent and its
	// associated physical snapshot on underlying storage system
	// should be deleted or not when released from its corresponding VolumeSnapshot.
	// If not specified, the default will be VolumeSnapshotContentRetain for static binding,
	// and VolumeSnapshotContentDelete for dynamic snapshot creation.
	// +optional
	DeletionPolicy *DeletionPolicy `json:"deletionPolicy,omitempty" protobuf:"bytes,5,opt,name=deletionPolicy"`
}

// VolumeSnapshotSource represents the actual location and type of the snapshot. Only one of its members may be specified.
type VolumeSnapshotSource struct {
	// CSI (Container Storage Interface) represents storage that handled by an external CSI Volume Driver (Alpha feature).
	// +optional
	CSI *CSIVolumeSnapshotSource `json:"csiVolumeSnapshotSource,omitempty"`
}

// CSIVolumeSnapshotSource represents the source from CSI volume snapshot
type CSIVolumeSnapshotSource struct {
	// Driver is the name of the driver used to create a physical snapshot on
	// underlying storage system.
	// This MUST be the same name returned by the CSI GetPluginName() call for
	// that driver.
	// Required.
	Driver string `json:"driver" protobuf:"bytes,1,opt,name=driver"`

	// SnapshotHandle is the unique id returned from the underlying storage system
	// by the CSI driver's CreationSnapshot gRPC call. It serves as the only and sufficient
	// handle when communicating with underlying storage systems via CSI driver for
	// all subsequent calls on the specific VolumeSnapshot
	// Required.
	SnapshotHandle string `json:"snapshotHandle" protobuf:"bytes,2,opt,name=snapshotHandle"`

	// Timestamp when the point-in-time snapshot is taken by the underlying storage
	// system. This timestamp will be generated and returned by the CSI volume driver after
	// the snapshot is cut. The format of this field is a Unix nanoseconds
	// time encoded as an int64. On Unix, the command `date +%s%N` returns
	// the current time in nanoseconds (aka, epoch time) since 1970-01-01 00:00:00 UTC.
	// This field is required in the CSI spec however made optional here to support static binding.
	// +optional
	CreationTime *int64 `json:"creationTime,omitempty" protobuf:"varint,3,opt,name=creationTime"`

	// When restoring a volume from a snapshot, the volume size needs to be equal to or
	// larger than the RestoreSize if it is specified.
	// If RestoreSize is set to nil, in the dynamic snapshot creation case, it means
	// that the underlying storage system does not have this information available;
	// in the static binding case, this piece of information is not available.
	// +optional
	RestoreSize *int64 `json:"restoreSize,omitempty" protobuf:"bytes,4,opt,name=restoreSize"`
}

// DeletionPolicy describes a policy for end-of-life maintenance of volume snapshot contents
type DeletionPolicy string

const (
	// VolumeSnapshotContentDelete means the snapshot content will be deleted from Kubernetes on release from its volume snapshot.
	VolumeSnapshotContentDelete DeletionPolicy = "Delete"

	// VolumeSnapshotContentRetain means the snapshot will be left in its current state on release from its volume snapshot.
	// The default policy is Retain if not specified.
	VolumeSnapshotContentRetain DeletionPolicy = "Retain"
)
