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

// +kubebuilder:object:generate=true
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
// +kubebuilder:subresource:status

type VolumeSnapshot struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// spec defines the desired characteristics of a snapshot requested by a user.
	// More info: https://kubernetes.io/docs/concepts/storage/volume-snapshots#volumesnapshots
	// +optional
	Spec VolumeSnapshotSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// status represents the current information of a snapshot.
	// NOTE: status can be modified by sources other than system controllers,
	// and must not be depended upon for accuracy.
	// Controllers should only use information from the VolumeSnapshotContent object
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
	// source specifies where a snapshot will be created from.
	// +optional
	Source *VolumeSnapshotSource `json:"source,omitempty" protobuf:"bytes,1,opt,name=source"`

	// volumeSnapshotClassName is the name of the VolumeSnapshotClass requested by the VolumeSnapshot.
	// If not specified, the default snapshot class will be used if one exists.
	// If not specified, and there is no default snapshot class, dynamic snapshot creation will fail.
	// More info: https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes
	// +optional
	VolumeSnapshotClassName *string `json:"volumeSnapshotClassName,omitempty" protobuf:"bytes,3,opt,name=volumeSnapshotClassName"`
}

// VolumeSnapshotSource represents the source of a snapshot
// Exactly one of its members must be set
type VolumeSnapshotSource struct {
	// persistentVolumeClaimName specifies the name of the PersistentVolumeClaim
	// object in the same namespace as the VolumeSnapshot object where the
	// snapshot should be dynamically taken from.
	// +optional
	PersistentVolumeClaimName *string `json:"persistentVolumeClaimName,omitempty" protobuf:"bytes,2,opt,name=persistentVolumeClaimName"`

	// volumeSnapshotContentName specifies the name of a pre-existing VolumeSnapshotContent
	// object a user asks to statically bind the VolumeSnapshot object to.
	// +optional
	VolumeSnapshotContentName *string `json:"volumeSnapshotContentName,omitempty" protobuf:"bytes,2,opt,name=volumeSnapshotContentName"`
}

// VolumeSnapshotStatus is the status of the VolumeSnapshot
type VolumeSnapshotStatus struct {
	// NOTE: All fields in VolumeSnapshotStatus are informational for user references.
	// Controllers MUST NOT rely on any fields programmatically.

	// boundVolumeSnapshotContentName represents the name of the VolumeSnapshotContent object
	// which the VolumeSnapshot object is bound to.
	// If not specified(i.e., nil), it means the VolumeSnapshot object has not been
	// successfully bound to a VolumeSnapshotContent object yet.
	// +optional
	BoundVolumeSnapshotContentName *string `json:"boundVolumeSnapshotContentName,omitempty" protobuf:"bytes,2,opt,name=boundVolumeSnapshotContentName"`

	// creationTime, if not nil, represents the timestamp when the point-in-time
	// snapshot was successfully cut on the underlying storage system.
	// In dynamic snapshot creation case, it will be filled in upon snapshot creation.
	// For a pre-existing snapshot, it will be filled in once the VolumeSnapshot object has
	// been successfully bound to a VolumeSnapshotContent object and the underlying
	// storage system has the information available.
	// +optional
	CreationTime *metav1.Time `json:"creationTime,omitempty" protobuf:"bytes,1,opt,name=creationTime"`

	// readyToUse is an informational flag which provides transparency to users.
	// Default to "False".
	// In dynamic snapshot creation case, readyToUse will be set to true when underlying storage
	// system has successfully finished all out-of-bound procedures to make a snapshot ready to
	// be used to restore a volume.
	// In manually binding to a pre-existing snapshot case, readyToUse will be set to
	// the value returned by CSI "ListSnapshot" gRPC call if the corresponding driver exists and supports,
	// otherwise, this field will be set to "True".
	// Required.
	ReadyToUse bool `json:"readyToUse" protobuf:"varint,3,opt,name=readyToUse"`

	// restoreSize represents the complete size of the snapshot in bytes.
	// The purpose of this field is to give user guidance on how much space is
	// needed to create a volume from this snapshot.
	// When restoring a volume from a snapshot, the size of the volume MUST NOT
	// be less than the restoreSize. Otherwise the restoration will fail.
	// If this field is not specified, it indicates that underlying storage system
	// does not have the information available.
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
	// +kubebuilder:validation:Enum=Delete;Retain
	// +optional
	DeletionPolicy *DeletionPolicy `json:"deletionPolicy,omitempty" protobuf:"bytes,4,opt,name=deletionPolicy"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotClassList is a collection of VolumeSnapshotClasses.
// +kubebuilder:object:root=true
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
// +kubebuilder:subresource:status
type VolumeSnapshotContent struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// spec defines properties of a VolumeSnapshotContent created by the underlying storage system.
	Spec VolumeSnapshotContentSpec `json:"spec" protobuf:"bytes,2,opt,name=spec"`

	// status represents the current information of a snapshot.
	Status VolumeSnapshotContentStatus `json:"status" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeSnapshotContentList is a list of VolumeSnapshotContent objects
// +kubebuilder:object:root=true
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
	// +kubebuilder:validation:Enum=Delete;Retain
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
	// +optional
	SnapshotHandle *string `json:"snapshotHandle,omitempty" protobuf:"bytes,2,opt,name=snapshotHandle"`

	// creationTime is the timestamp when the point-in-time snapshot is taken
	// by the underlying storage system. This timestamp is returned by the CSI
	// driver after the snapshot is cut.
	// The format of this field is a Unix nanoseconds time encoded as an int64.
	// On Unix, the command `date +%s%N` returns the current time in nanoseconds
	// since 1970-01-01 00:00:00 UTC.
	// +optional
	CreationTime *int64 `json:"creationTime,omitempty" protobuf:"varint,3,opt,name=creationTime"`

	// restoreSize specifies the complete size of the snapshot in bytes.
	// When restoring a volume from this snapshot, the size of the volume MUST NOT
	// be smaller than the restoreSize if it is specified.
	// Otherwise the restoration will fail.
	// If this field is not set, it indicates that the size is unknown.
	// +kubebuilder:validation:Minimum=0
	// +optional
	RestoreSize *int64 `json:"restoreSize,omitempty" protobuf:"bytes,4,opt,name=restoreSize"`
}

// VolumeSnapshotContentStatus is the status of a VolumeSnapshotContent object
type VolumeSnapshotContentStatus struct {
	// readyToUse indicates if a snapshot is ready to be used as a source to restore a volume.
	// In dynamic snapshot creation case, this field will be filled in with the value returned
	// from CSI CreateSnapshotRequest call.
	// For pre-existing snapshot, this field will be updated with the value returned
	// from CSI ListSnapshotRequest if the functionality is supported by the corresponding
	// driver. Otherwise, this field will be set to "True".
	// Required.
	ReadyToUse bool `json:"readyToUse" protobuf:"varint,3,opt,name=readyToUse"`
	// error is the latest observed error during snapshot creation, if any.
	// +optional
	Error *VolumeSnapshotError `json:"error,omitempty" protobuf:"bytes,4,opt,name=error,casttype=VolumeSnapshotError"`
}

// DeletionPolicy describes a policy for end-of-life maintenance of volume snapshot contents
// +kubebuilder:validation:Enum=Delete;Retain
type DeletionPolicy string

const (
	// volumeSnapshotContentDelete means the snapshot will be deleted from Kubernetes on release from its volume snapshot.
	VolumeSnapshotContentDelete DeletionPolicy = "Delete"

	// volumeSnapshotContentRetain means the snapshot will be left in its current state on release from its volume snapshot.
	VolumeSnapshotContentRetain DeletionPolicy = "Retain"
)

// VolumeSnapshotError describes an error encountered during snapshot creation.
type VolumeSnapshotError struct {
	// time is the timestamp when the error was encountered.
	// +optional
	Time *metav1.Time `json:"time,omitempty" protobuf:"bytes,1,opt,name=time"`

	// message is a string detailing the encountered error during snapshot
	// creation if specified.
	// NOTE: message may be logged, and it should not contain sensitive
	// information.
	// +optional
	Message *string `json:"message,omitempty" protobuf:"bytes,2,opt,name=message"`
}
