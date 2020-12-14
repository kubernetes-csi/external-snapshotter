# Scripts User Guide

This README documents:
* What update-crd.sh and update-generated-code.sh do
* When and how to use them

## update-generated-code.sh

This is the script to update clientset/informers/listers and API deepcopy code using [code-generator](https://github.com/kubernetes/code-generator).

Make sure to run this script after making changes to /client/apis/volumesnapshot/v1/types.go.

Pre-requisites for running update-generated-code.sh:

* GOPATH=~/go

* Ensure external-snapshotter repository is at ~/go/src/github.com/kubernetes-csi/external-snapshotter

* git clone https://github.com/kubernetes/code-generator.git under ~/go/src/k8s.io

* git checkout to version v0.19.0
```bash
git checkout v0.19.0
```

* Ensure the path exist ${GOPATH}/src/k8s.io/code-generator/generate-groups.sh

Run: ./hack/update-generated-code.sh from the client directory.

Once you run the script, you will get an output as follows:
```bash
Generating deepcopy funcs
Generating clientset for volumesnapshot:v1 at github.com/kubernetes-csi/external-snapshotter/client/v4/clientset
Generating listers for volumesnapshot:v1 at github.com/kubernetes-csi/external-snapshotter/client/v4/listers
Generating informers for volumesnapshot:v1 at github.com/kubernetes-csi/external-snapshotter/client/v4/informers

```

NOTE: We need to keep both v1beta1 and v1 snapshot clients at the current phase.

## update-crd.sh

NOTE: We need to serve both v1beta1 and v1 snapshot APIs and keep storage version at v1beta1 at the current phase.

This is the script to update CRD yaml files under /client/config/crd/ based on types.go file.

Make sure to run this script after making changes to /client/apis/volumesnapshot/v1/types.go.

Follow these steps to update the CRD:

* Run ./hack/update-crd.sh from client directory, new yaml files should have been created under ./config/crd/

* Add api-approved.kubernetes.io annotation value in all yaml files in the metadata section with the PR where the API is approved by the API reviewers. The current approved PR for snapshot v1 API is https://github.com/kubernetes-csi/external-snapshotter/pull/419. Refer to https://github.com/kubernetes/enhancements/pull/1111 for details about this annotation.

* Remove any metadata sections from the yaml file which does not belong to the generated type.
For example, the following command will add a metadata section for a nested object, remove any newly added metadata sections. TODO(xiangqian): this is to make sure the generated CRD is compatible with apiextensions.k8s.io/v1. Once controller-gen supports generating CRD with apiextensions.k8s.io/v1, switch to use the correct version of controller-gen and remove the last step from this README.

```bash
./hack/update-crd.sh; git diff
+        metadata:
+          description: 'Standard object''s metadata. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata'
           type: object
```

* Update the restoreSize property to string in snapshot.storage.k8s.io_volumesnapshots.yaml

The generated yaml file contains restoreSize property anyOf as described below: 
 
```bash
            restoreSize:
              anyOf:
              - type: integer
              - type: string
              description: restoreSize represents the complete size of the snapshot
                in bytes. In dynamic snapshot creation case, this field will be filled
                in with the "size_bytes" value returned from CSI "CreateSnapshotRequest"
                gRPC call. For a pre-existing snapshot, this field will be filled
                with the "size_bytes" value returned from the CSI "ListSnapshots"
                gRPC call if the driver supports it. When restoring a volume from
                this snapshot, the size of the volume MUST NOT be smaller than the
                restoreSize if it is specified, otherwise the restoration will fail.
                If not specified, it indicates that the size is unknown.
```

Update the restoreSize property to use type string only:

```bash
   
            restoreSize:
              type: string
              description: restoreSize represents the complete size of the snapshot
                in bytes. In dynamic snapshot creation case, this field will be filled
                in with the "size_bytes" value returned from CSI "CreateSnapshotRequest"
                gRPC call. For a pre-existing snapshot, this field will be filled
                with the "size_bytes" value returned from the CSI "ListSnapshots"
                gRPC call if the driver supports it. When restoring a volume from
                this snapshot, the size of the volume MUST NOT be smaller than the
                restoreSize if it is specified, otherwise the restoration will fail.
                If not specified, it indicates that the size is unknown.

```

* In `client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml`, we need to add the `oneOf` constraint to make sure only one of `persistentVolumeClaimName` and `volumeSnapshotContentName` is specified in the `source` field of the `spec` of `VolumeSnapshot`.

```
              source:
                description: source specifies where a snapshot will be created from. This field is immutable after creation. Required.
                properties:
                  persistentVolumeClaimName:
                    description: persistentVolumeClaimName specifies the name of the PersistentVolumeClaim object representing the volume from which a snapshot should be created. This PVC is assumed to be in the same namespace as the VolumeSnapshot object. This field should be set if the snapshot does not exists, and should be created. This field is immutable.
                    type: string
                  volumeSnapshotContentName:
                    description: volumeSnapshotContentName specifies the name of a pre-existing VolumeSnapshotContent object representing an existing volume snapshot. This field should be set if the snapshot already exists. This field is immutable.
                    type: string
                type: object
                oneOf:
                - required: ["persistentVolumeClaimName"]
                - required: ["volumeSnapshotContentName"]
              volumeSnapshotClassName:
```

* In `client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml `, we need to add the `oneOf` constraint to make sure only one of `snapshotHandle` and `volumeHandle` is specified in the `source` field of the `spec` of `VolumeSnapshotContent`.

```
              source:
                description: source specifies from where a snapshot will be created. This field is immutable after creation. Required.
                properties:
                  snapshotHandle:
                    description: snapshotHandle specifies the CSI "snapshot_id" of a pre-existing snapshot on the underlying storage system. This field is immutable.
                    type: string
                  volumeHandle:
                    description: volumeHandle specifies the CSI "volume_id" of the volume from which a snapshot should be dynamically taken from. This field is immutable.
                    type: string
                type: object
                oneOf:
                - required: ["snapshotHandle"]
                - required: ["volumeHandle"]
              volumeSnapshotClassName:
```

* Because we need to serve both v1 and v1beta1 snapshot APIs, we need to make sure that both v1 and v1beta1 APIs are in the manifest yaml file. Because `update-crd.sh` only generates v1 manifest, make sure to copy the v1beta1 manifest below the v1 manifest after running `update-crd.sh` in the manifest yaml files. See `snapshot.storage.k8s.io_volumesnapshots.yaml` as an example. `served` is true for both v1beta1 and v1. `storage` is true for v1beta and false for v1.

```
spec:
  group: snapshot.storage.k8s.io
  names:
    kind: VolumeSnapshot
    listKind: VolumeSnapshotList
    plural: volumesnapshots
    singular: volumesnapshot
  scope: Namespaced
  versions:
  - additionalPrinterColumns:
    - description: Indicates if a snapshot is ready to be used to restore a volume.
      jsonPath: .status.readyToUse
      name: ReadyToUse
      type: boolean
......
    - description: Timestamp when the point-in-time snapshot is taken by the underlying storage system.
      jsonPath: .status.creationTime
      name: CreationTime
      type: date
    - jsonPath: .metadata.creationTimestamp
      name: Age
      type: date
    name: v1
    schema:
      openAPIV3Schema:
        description: VolumeSnapshot is a user's request for either creating a point-in-time snapshot of a persistent volume, or binding to a pre-existing snapshot.
        properties:
......
    served: true
    storage: false
    subresources:
      status: {}
  - additionalPrinterColumns:
    - description: Indicates if a snapshot is ready to be used to restore a volume.
      jsonPath: .status.readyToUse
      name: ReadyToUse
      type: boolean
......
    - description: Timestamp when the point-in-time snapshot is taken by the underlying storage system.
      jsonPath: .status.creationTime
      name: CreationTime
      type: date
    - jsonPath: .metadata.creationTimestamp
      name: Age
      type: date
    name: v1beta1
    schema:
      openAPIV3Schema:
        description: VolumeSnapshot is a user's request for either creating a point-in-time snapshot of a persistent volume, or binding to a pre-existing snapshot.
        properties:
......
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
``````
