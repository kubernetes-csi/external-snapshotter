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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshot is a user's request for taking a point-in-time snapshot of a PersistentVolumeClaim.
// Upon successful creation of a snapshot by the underlying storage system, it is bound to a
// corresponding VolumeSnapshotContent.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type VolumeSnapshot struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// spec defines the desired characteristics of a snapshot requested by a user.
	// More info: https:s//kubernetes.io/docs/concepts/storage/volume-snapshots#volumesnapshots
	// +optional
	Spec VolumeSnapshotSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// status represents the current information of a snapshot.
	// NOTE: status can be modified by sources other than system controllers,
	// and must not be depended upon for accuracy.
	// Controllers should only be using information from the VolumeSnapshotContent object
	// after verifying that the binding is accurate and complete.
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
	// source specifies where a snapshot should be created from.
	// Currently, PersistentVolumeClaim is the only supported source type.
	// If specified, and volumeSnapshotContentName is not specified(i.e., nil),
	// snapshot will be dynamically created from the given source.
	// If both source and volumeSnapshotContentName are not specified, dynamic snapshot
	// creation will fail.
	// +optional
	Source *core_v1.TypedLocalObjectReference `json:"source,omitempty" protobuf:"bytes,1,opt,name=source"`

	// volumeSnapshotContentName is the name of the VolumeSnapshotContent object backing the VolumeSnapshot.
	// To request a pre-existing VolumeSnapshotContent object, this field MUST be specified during
	// VolumeSnapshot object creation. Otherwise, a snapshot will be dynamically created, and this field will
	// be populated afterwards.
	// +optional
	VolumeSnapshotContentName *string `json:"volumeSnapshotContentName,omitempty" protobuf:"bytes,2,opt,name=volumeSnapshotContentName"`

	// volumeSnapshotClassName is the name of the VolumeSnapshotClass requested by the VolumeSnapshot.
	// If not specified, the default snapshot class will be used if one exists.
	// If not specified, and there is no default snapshot class, dynamic snapshot creation will fail.
	// More info: https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes
	// +optional
	VolumeSnapshotClassName *string `json:"volumeSnapshotClassName,omitempty" protobuf:"bytes,3,opt,name=volumeSnapshotClassName"`
}

// VolumeSnapshotStatus is the status of the VolumeSnapshot
// +kubebuilder:subresource:status
type VolumeSnapshotStatus struct {
	// NOTE: All fields in VolumeSnapshotStatus are informational for user references.
	// Controllers MUST NOT rely on any fields programmatically.

	// creationTime, if not nil, represents the timestamp when the snapshot was successfully
	// cut by the underlying storage system.
	// When dynamically creating a snapshot, this will be filled in upon creation.
	// For pre-existing snapshot, it will be filled in once the VolumeSnapshot object has
	// been successfully bound to a VolumeSnapshotContent object and the underlying
	// storage system has the information available.
	// NOTE: Controllers MUST NOT rely on this field programmatically.
	// +optional
	CreationTime *metav1.Time `json:"creationTime,omitempty" protobuf:"bytes,1,opt,name=creationTime"`

	// readyToUse is an informational flag which provides transparency to users.
	// In dynamic snapshot creation case, readyToUse will be set to true when underlying storage
	// system has successfully finished all out-of-bound procedures to make a snapshot ready to consume,
	// i.e., restoring a PersistentVolumeClaim from the snapshot.
	// When manually binding to a pre-existing VolumeSnapshotContent object, readyToUse will be set to
	// the value returned by CSI "ListSnapshot" gRPC call if the corresponding driver supports,
	// otherwise, readyToUse field will remain its original value.
	// If not specified(i.e., nil), it means the readiness of the snapshot is unknown to system controllers.
	// NOTE: Controllers MUST NOT rely on this field programmatically.
	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty" protobuf:"varint,3,opt,name=readyToUse"`

	// restoreSize specifies the number of bytes that the snapshot's data would consume when
	// restored to a volume.
	// When restoring a volume from a snapshot, the volume size needs to be equal to or
	// larger than the restoreSize if it is specified. Otherwise the restoration will fail.
	// The field could be nil if the underlying storage system does not have the information available.
	// NOTE: Controllers MUST NOT rely on this field programmatically.
	// +optional
	RestoreSize *resource.Quantity `json:"restoreSize,omitempty" protobuf:"bytes,2,opt,name=restoreSize"`

	// error is the latest observed error during snapshot creation, if any.
	// This field could be helpful to upper level controllers(i.e., application controller)
	// to decide whether they should continue on waiting for the snapshot to be created
	// based on the type of error reported.
	// +optional
	Error *VolumeSnapshotError `json:"error,omitempty" protobuf:"bytes,4,opt,name=error,casttype=VolumeSnapshotError"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotClass specifies parameters that a underlying storage system uses when
// creating a volume snapshot. A specific VolumeSnapshotClass is used by specifying its
// name in a VolumeSnapshot object.
// VolumeSnapshotClasses are non-namespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type VolumeSnapshotClass struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// driver is the name of the storage driver that handles this VolumeSnapshotClass.
	Driver string `json:"driver" protobuf:"bytes,2,opt,name=driver"`

	// parameters is a key-value map with storage driver specific parameters for creating snapshots.
	// These values are opaque to Kubernetes.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty" protobuf:"bytes,3,rep,name=parameters"`

	// deletionPolicy determines whether a VolumeSnapshotContent created through the VolumeSnapshotClass should
	// be deleted when its bound VolumeSnapshot is deleted.
	// Supported values are "Retain" and "Delete".
	// "Retain" means that the VolumeSnapshotContent and its physical snapshot on underlying storage system are kept.
	// "Delete" means that the VolumeSnapshotContent and its physical snapshot on underlying storage system are deleted.
	// If not specified, the default value is "Delete"
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

	// items is the list of VolumeSnapshotClasses
	Items []VolumeSnapshotClass `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotContent represents the actual "on-disk" snapshot object in the
// underlying storage system
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type VolumeSnapshotContent struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// spec defines properties of a VolumeSnapshotContent created by the underlying storage system.
	Spec VolumeSnapshotContentSpec `json:"spec" protobuf:"bytes,2,opt,name=spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotContentList is a list of VolumeSnapshotContent objects
type VolumeSnapshotContentList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// items is the list of VolumeSnapshotContents
	Items []VolumeSnapshotContent `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// VolumeSnapshotContentSpec is the specification of a VolumeSnapshotContent
type VolumeSnapshotContentSpec struct {

	// volumeSnapshotRef specifies the VolumeSnapshot object that this VolumeSnapshotContent is bound with.
	// The VolumeSnapshot.Spec.VolumeSnapshotContentName field must reference this VolumeSnapshotContent name
	// for the bidirectional binding to be considered valid.
	// If the referenced VolumeSnapshot object does not exist(i.e., deleted by user) AND
	// the UID of the referent is not empty(i.e., volumeSnapshotRef.UID != ""),
	// then the VolumeSnapshotContent.Spec.DeletionPolicy is triggered.
	// To manually bind a pre-existing VolumeSnapshotContent object to a VolumeSnapshot object,
	// the volumeSnapshotRef.UID should be left empty to avoid triggering deletion policy.
	// The UID field will be populated once binding is considered valid.
	// +optional
	VolumeSnapshotRef *core_v1.ObjectReference `json:"volumeSnapshotRef,omitempty" protobuf:"bytes,2,opt,name=volumeSnapshotRef"`

	// deletionPolicy determines whether this VolumeSnapshotContent and its physical snapshot on
	// the underlying storage system should be deleted when its VolumeSnapshot is deleted.
	// Supported values are "Retain" and "Delete".
	// "Retain" means that the VolumeSnapshotContent and its physical snapshot on underlying storage system are kept.
	// "Delete" means that the VolumeSnapshotContent and its physical snapshot on underlying storage system are deleted.
	// If not specified, the default value is "Retain"
	// +optional
	DeletionPolicy *DeletionPolicy `json:"deletionPolicy,omitempty" protobuf:"bytes,5,opt,name=deletionPolicy"`

	// driver is the name of the CSI driver used to create the physical snapshot on
	// the underlying storage system.
	// This MUST be the same as the name returned by the CSI GetPluginName() call for
	// that driver.
	// Required.
	Driver string `json:"driver" protobuf:"bytes,1,opt,name=driver"`

	// snapshotHandle is the snapshot id returned by the CSI driver in the CreateSnapshotResponse
	// and is used as the snapshot identifier for all subsequent CSI calls.
	// Required.
	SnapshotHandle string `json:"snapshotHandle" protobuf:"bytes,2,opt,name=snapshotHandle"`

	// creationTime is the timestamp when the point-in-time snapshot is taken by the underlying storage
	// system. This timestamp is returned by the CSI driver after the snapshot is cut.
	// The format of this field is a Unix nanoseconds time encoded as an int64.
	// On Unix, the command `date +%s%N` returns
	// the current time in nanoseconds (aka, epoch time) since 1970-01-01 00:00:00 UTC.
	// +optional
	CreationTime *int64 `json:"creationTime,omitempty" protobuf:"varint,3,opt,name=creationTime"`

	// restoreSize specifies the number of bytes that the snapshot's data would consume when
	// restored to a volume.
	// When restoring a volume from a snapshot, the volume size needs to be equal to or
	// larger than the restoreSize if it is specified. Otherwise the restoration will fail.
	// +optional
	RestoreSize *int64 `json:"restoreSize,omitempty" protobuf:"bytes,4,opt,name=restoreSize"`
}

// DeletionPolicy describes a policy for end-of-life maintenance of volume snapshot contents
// +kubebuilder:validation:Enum=Delete;Retain
type DeletionPolicy string

const (
	// volumeSnapshotContentDelete means the snapshot content will be deleted from Kubernetes on release from its volume snapshot.
	VolumeSnapshotContentDelete DeletionPolicy = "Delete"

	// volumeSnapshotContentRetain means the snapshot will be left in its current state on release from its volume snapshot.
	// The default policy is Retain if not specified.
	VolumeSnapshotContentRetain DeletionPolicy = "Retain"
)

// VolumeSnapshotError describes an error encountered during snapshot creation.
type VolumeSnapshotError struct {
	// time is the timestamp when the error was encountered.
	// +optional
	Time *metav1.Time `json:"time,omitempty" protobuf:"bytes,1,opt,name=time"`

	// message is a string detailing the encountered error during snapshot
	// creation if specified.
	// NOTE: message maybe logged, thus it should not contain sensitive
	// information.
	// +optional
	Message *string `json:"message,omitempty" protobuf:"bytes,2,opt,name=message"`
}
