# CSI Snapshotter

The CSI external-snapshotter is part of Kubernetes implementation of [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec)

## Overview

CSI Snapshotter is an external controller that watches Kubernetes Snapshot CRD objects and triggers CreateSnapshot/DeleteSnapshot against a CSI endpoint. Full design can be found at Kubernetes proposal at [here](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-snapshot.md)

## Design

External snapshotter follows [controller](https://github.com/kubernetes/community/blob/master/contributors/devel/controllers.md) pattern and uses informers to watch for `VolumeSnapshot` and `VolumeSnapshotContent` create/update/delete events. It filters out these objects with `Snapshotter==<CSI driver name>` specified in the associated VolumeSnapshotClass object and then processes these events in workqueues with exponential backoff.

### Snapshotter

Snapshotter talks to CSI over socket (/run/csi/socket by default, configurable by -csi-address). The snapshotter then:

* Discovers the supported snapshotter name by `GetDriverName` call. 

* Uses ControllerGetCapabilities for find out if CSI driver supports `ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT` and `ControllerServiceCapability_RPC_LIST_SNAPSHOTS` calls. Otherwise, the controller will not start.

* Processes new/updated/deleted `VolumeSnapshots`: The snapshotter only processes `VolumeSnapshot` that has `snapshotter` specified in its `VolumeSnapshotClass` matches its driver name. The process workflow is as follows
  * If the snapshot status is `Ready`, the controller checks whether the snapshot and its content still binds correctly. If there is any problem with the binding (e.g., snapshot points to a non-exist snapshot content), update the snapshot status and emit event.
  * If the snapshot status is not ready, there are two cases.
    * `SnapshotContentName` is not empty: the controller verifies whether the snapshot content exists and also binds to the snapshot. If verification passes, the controller binds the snapshot and its content objects and marks it is ready. Otherwise, it updates the error status of the snapshot.
    * `SnapshotContentName` is set empty: the controller will first check whether there is already a content object which binds the snapshot correctly with snapshot uid (`VolumeSnapshotRef.UID`) specified. If so, the controller binds these two objects. Otherwise, the controller issues a create snapshot operation. Please note that if the error status shows that snapshot creation already failed before, it will not try to create snapshot again.


* Processes new/updated/deleted `VolumeSnapshotContents`: The snapshotter only processes `VolumeSnapshotContent` in which the CSI driver specified in the spec matches the controller's driver name.
  * If the `VolumeSnapshotRef` is set to nil, skip this content since it is not bound to any snapshot object.
  * Otherwise, the controller verifies whether the content object is correctly bound to a snapshot object. In case the `VolumeSnapshotRef.UID` is set but it does not match its snapshot object or snapshot no long exists, the content object and its associated snapshot will be deleted.

## Usage

### Running on command line

For debugging, it is possible to run snapshotter on command line. For example,

```
$ csi-snapshotter -kubeconfig ~/.kube/config -v 5 -csi-address /run/csi/socket
```

### Running in a statefulset

It is necessary to create a new service account and give it enough privileges to run the snapshotter. We provide one omnipotent yaml file that creates everything that's necessary, however it should be split into multiple files in production.

```
$ kubectl create deploy/kubernetes/statefulset.yaml
```

## Testing

Running Unit Tests:
```
$ go test -timeout 30s  github.com/kubernetes-csi/external-snapshotter/pkg/controller
```

## Dependency Management

```
$ dep ensure
```

To modify dependencies or versions change `./Gopkg.toml`

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-storage)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
