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
	// Required.
	Spec VolumeSnapshotSpec `json:"spec" protobuf:"bytes,2,opt,name=spec"`

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

// VolumeSnapshotSpec describes the common attributes of a volume snapshot.
type VolumeSnapshotSpec struct {
	// source specifies where a snapshot will be created from.
	// Required.
	Source VolumeSnapshotSource `json:"source" protobuf:"bytes,1,opt,name=source"`

	// volumeSnapshotClassName is the name of the VolumeSnapshotClass requested by the VolumeSnapshot.
	// If not specified, the default snapshot class will be used if one exists.
	// If not specified, and there is no default snapshot class, dynamic snapshot creation will fail.
	// More info: https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes
	// +optional
	VolumeSnapshotClassName *string `json:"volumeSnapshotClassName,omitempty" protobuf:"bytes,2,opt,name=volumeSnapshotClassName"`
}

// VolumeSnapshotSource represents the source of a snapshot.
// Exactly one of its members must be set.
// Members in VolumeSnapshotSource are immutable.
// TODO(xiangqian): Add a webhook to ensure that VolumeSnapshotSource members
// will not be updated once specified.
type VolumeSnapshotSource struct {
	// persistentVolumeClaimName specifies the name of the PersistentVolumeClaim
	// object in the same namespace as the VolumeSnapshot object where the
	// snapshot should be dynamically taken from.
	// +optional
	PersistentVolumeClaimName *string `json:"persistentVolumeClaimName,omitempty" protobuf:"bytes,1,opt,name=persistentVolumeClaimName"`

	// volumeSnapshotContentName specifies the name of a pre-existing VolumeSnapshotContent
	// object a user asks to statically bind the VolumeSnapshot object to.
	// +optional
	VolumeSnapshotContentName *string `json:"volumeSnapshotContentName,omitempty" protobuf:"bytes,2,opt,name=volumeSnapshotContentName"`
}

// VolumeSnapshotStatus is the status of the VolumeSnapshot
type VolumeSnapshotStatus struct {
	// NOTE: All fields in VolumeSnapshotStatus are informational for user references.
	// Controllers MUST NOT rely on any fields programmatically.

	// boundVolumeSnapshotContentName represents the name of the VolumeSnapshotContent
	// object to which the VolumeSnapshot object is bound.
	// If not specified, it indicates that the VolumeSnapshot object has not been
	// successfully bound to a VolumeSnapshotContent object yet.
	// +optional
	BoundVolumeSnapshotContentName *string `json:"boundVolumeSnapshotContentName,omitempty" protobuf:"bytes,1,opt,name=boundVolumeSnapshotContentName"`

	// creationTime, if not nil, represents the timestamp when the point-in-time
	// snapshot was successfully cut on the underlying storage system.
	// In dynamic snapshot creation case, it will be filled in upon snapshot creation.
	// For a pre-existing snapshot, it will be filled in once the VolumeSnapshot object has
	// been successfully bound to a VolumeSnapshotContent object and the underlying
	// storage system has the information available.
	// If not specified, it indicates that the creation time of the snapshot is unknown.
	// +optional
	CreationTime *metav1.Time `json:"creationTime,omitempty" protobuf:"bytes,2,opt,name=creationTime"`

	// readyToUse indicates if a snapshot is ready to be used to restore a volume.
	// In dynamic snapshot creation case, readyToUse will be set to true after underlying storage
	// system has successfully finished all out-of-bound procedures to make a snapshot ready to
	// be used to restore a volume.
	// For a pre-existing snapshot, readyToUse will be set to the value returned
	// from CSI "ListSnapshots" gRPC call if the matching CSI driver exists and supports.
	// Otherwise, if there exists no CSI driver or the driver does not support "ListSnapshots",
	// this field will be set to "True".
	// If not specified, it indicates that the readiness of a snapshot is unknown.
	// +optional
	ReadyToUse *bool `json:"readyToUse,omitempty" protobuf:"varint,3,opt,name=readyToUse"`

	// restoreSize represents the complete size of the snapshot in bytes.
	// The purpose of this field is to give user guidance on how much space is
	// needed to restore a volume from this snapshot.
	// When restoring a volume from a snapshot, the size of the volume MUST NOT
	// be less than the restoreSize. Otherwise the restoration will fail.
	// If this field is not specified, it indicates that underlying storage system
	// does not have the information available.
	// +optional
	RestoreSize *resource.Quantity `json:"restoreSize,omitempty" protobuf:"bytes,4,opt,name=restoreSize"`

	// error is the last observed error during snapshot creation, if any.
	// This field could be helpful to upper level controllers(i.e., application controller)
	// to decide whether they should continue on waiting for the snapshot to be created
	// based on the type of error reported.
	// +optional
	Error *VolumeSnapshotError `json:"error,omitempty" protobuf:"bytes,5,opt,name=error,casttype=VolumeSnapshotError"`
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

	// deletionPolicy determines whether a VolumeSnapshotContent created through
	// the VolumeSnapshotClass should be deleted when its bound VolumeSnapshot is deleted.
	// Supported values are "Retain" and "Delete".
	// "Retain" means that the VolumeSnapshotContent and its physical snapshot on underlying storage system are kept.
	// "Delete" means that the VolumeSnapshotContent and its physical snapshot on underlying storage system are deleted.
	// +kubebuilder:validation:Enum=Delete;Retain
	// Required.
	DeletionPolicy DeletionPolicy `json:"deletionPolicy" protobuf:"bytes,4,opt,name=deletionPolicy"`
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
	// volumeSnapshotRef specifies the VolumeSnapshot object to which this
	// VolumeSnapshotContent object is bound.
	// VolumeSnapshot.Spec.VolumeSnapshotContentName field must reference to
	// this VolumeSnapshotContent's name for the bidirectional binding to be valid.
	// For a pre-existing VolumeSnapshotContent object, name and namespace of the
	// VolumeSnapshot object MUST be provided for binding to happen.
	// Required.
	VolumeSnapshotRef core_v1.ObjectReference `json:"volumeSnapshotRef" protobuf:"bytes,1,opt,name=volumeSnapshotRef"`

	// deletionPolicy determines whether this VolumeSnapshotContent and its physical snapshot on
	// the underlying storage system should be deleted when its bound VolumeSnapshot is deleted.
	// Supported values are "Retain" and "Delete".
	// "Retain" means that the VolumeSnapshotContent and its physical snapshot on underlying storage system are kept.
	// "Delete" means that the VolumeSnapshotContent and its physical snapshot on underlying storage system are deleted.
	// In dynamic snapshot creation case, this field will be filled in with the "DeletionPolicy" field defined in the
	// VolumeSnapshotClass the VolumeSnapshot refers to.
	// For pre-existing snapshots, users MUST specify this field when creating the VolumeSnapshotContent object.
	// +kubebuilder:validation:Enum=Delete;Retain
	// Required.
	DeletionPolicy DeletionPolicy `json:"deletionPolicy" protobuf:"bytes,2,opt,name=deletionPolicy"`

	// driver is the name of the CSI driver used to create the physical snapshot on
	// the underlying storage system.
	// This MUST be the same as the name returned by the CSI GetPluginName() call for
	// that driver.
	// Required.
	Driver string `json:"driver" protobuf:"bytes,3,opt,name=driver"`

	// name of the SnapshotClass to which this snapshot belongs.
	// +optional
	SnapshotClassName *string `json:"snapshotClassName,omitempty" protobuf:"bytes,4,opt,name=snapshotClassName"`

	// source specifies from where a snapshot will be created.
	// Required.
	Source VolumeSnapshotContentSource `json:"source" protobuf:"bytes,5,opt,name=source"`
}

// VolumeSnapshotContentSource represents the source of a snapshot.
// Exactly one of its members must be set.
// Members in VolumeSnapshotContentSource are immutable.
// TODO(xiangqian): Add a webhook to ensure that VolumeSnapshotContentSource members
// will not be updated once specified.
type VolumeSnapshotContentSource struct {
	// volumeHandle specifies the CSI name of the volume from which a snapshot
	// should be dynamically taken from.
	// +optional
	VolumeHandle *string `json:"volumeHandle,omitempty" protobuf:"bytes,1,opt,name=volumeHandle"`

	// snapshotHandle specifies the CSI name of a pre-existing snapshot on the
	// underlying storage system.
	// +optional
	SnapshotHandle *string `json:"snapshotHandle,omitempty" protobuf:"bytes,2,opt,name=snapshotHandle"`
}

// VolumeSnapshotContentStatus is the status of a VolumeSnapshotContent object
type VolumeSnapshotContentStatus struct {
	// snapshotHandle is the CSI name of a snapshot on the underlying storage system.
	// +optional
	SnapshotHandle *string `json:"snapshotHandle,omitempty" protobuf:"bytes,1,opt,name=snapshotHandle"`

	// creationTime is the timestamp when the point-in-time snapshot is taken
	// by the underlying storage system. This timestamp is returned by the CSI
	// driver after the snapshot is cut.
	// The format of this field is a Unix nanoseconds time encoded as an int64.
	// On Unix, the command `date +%s%N` returns the current time in nanoseconds
	// since 1970-01-01 00:00:00 UTC.
	// +optional
	CreationTime *int64 `json:"creationTime,omitempty" protobuf:"varint,2,opt,name=creationTime"`

	// restoreSize represents the complete size of the snapshot in bytes.
	// When restoring a volume from this snapshot, the size of the volume MUST NOT
	// be smaller than the restoreSize if it is specified.
	// Otherwise the restoration will fail.
	// If not specified, it indicates that the size is unknown.
	// +kubebuilder:validation:Minimum=0
	// +optional
	RestoreSize *int64 `json:"restoreSize,omitempty" protobuf:"bytes,3,opt,name=restoreSize"`

	// readyToUse indicates if a snapshot is ready to be used to restore a volume.
	// In dynamic snapshot creation case, this field will be filled in with the
	// value returned from CSI "CreateSnapshotRequest" gRPC call.
	// For pre-existing snapshot, this field will be updated with the value returned
	// from CSI "ListSnapshots" gRPC call if the corresponding driver supports.
	// If not specified, it means the readiness of a snapshot is unknown.
	// +optional.
	ReadyToUse *bool `json:"readyToUse,omitempty" protobuf:"varint,4,opt,name=readyToUse"`
	// error is the latest observed error during snapshot creation, if any.
	// +optional
	Error *VolumeSnapshotError `json:"error,omitempty" protobuf:"bytes,5,opt,name=error,casttype=VolumeSnapshotError"`
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
