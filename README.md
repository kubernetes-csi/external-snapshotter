[![Build Status](https://travis-ci.org/kubernetes-csi/external-snapshotter.svg?branch=master)](https://travis-ci.org/kubernetes-csi/external-snapshotter)

# CSI Snapshotter

The CSI snapshotter is part of Kubernetes implementation of [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec).

The volume snapshot feature supports CSI v1.0 and higher. It was introduced as an Alpha feature in Kubernetes v1.12 and has been promoted to an Beta feature in Kubernetes 1.17.


## Overview

With the promotion of Volume Snapshot to beta, the feature is now enabled by default on standard Kubernetes deployments instead of being opt-in.

The move of the Kubernetes Volume Snapshot feature to beta also means:
* A revamp of volume snapshot APIs.
* The CSI external-snapshotter sidecar is split into two controllers, a snapshot controller and a CSI external-snapshotter sidecar.

The snapshot controller is deployed by the Kubernetes distributions and is responsible for watching the VolumeSnapshot CRD objects and manges the creation and deletion lifecycle of snapshots.

The CSI external-snapshotter sidecar watches Kubernetes VolumeSnapshotContent CRD objects and triggers CreateSnapshot/DeleteSnapshot against a CSI endpoint.

Blog post for the beta feature can be found [here](https://kubernetes.io/blog/2019/12/09/kubernetes-1-17-feature-cis-volume-snapshot-beta)


## Compatibility

This information reflects the head of this branch.

| Compatible with CSI Version                                                                | Container Image             | Min K8s Version | Snapshot CRD version |
| ------------------------------------------------------------------------------------------ | ----------------------------| --------------- | -------------------- |
| [CSI Spec v1.0.0](https://github.com/container-storage-interface/spec/releases/tag/v1.0.0) | quay.io/k8scsi/csi-snapshotter | 1.17         | v1beta1              |
| [CSI Spec v1.0.0](https://github.com/container-storage-interface/spec/releases/tag/v1.0.0) | quay.io/k8scsi/snapshot-controller | 1.17     | v1beta1              |


## Feature Status

The `VolumeSnapshotDataSource` feature gate was introduced in Kubernetes 1.12 and it is enabled by default in Kubernetes 1.17 when the volume snapshot feature is promoted to beta.


## Design

Both the snapshot controller and CSI external-snapshotter sidecar follow [controller](https://github.com/kubernetes/community/blob/master/contributors/devel/controllers.md) pattern and uses informers to watch for events. The snapshot controller watches for `VolumeSnapshot` and `VolumeSnapshotContent` create/update/delete events.

The CSI external-snapshotter sidecar only watches for `VolumeSnapshotContent` create/update/delete events. It filters out these objects with `Driver==<CSI driver name>` specified in the associated VolumeSnapshotClass object and then processes these events in workqueues with exponential backoff.

The CSI external-snapshotter sidecar talks to CSI over socket (/run/csi/socket by default, configurable by -csi-address).

### Hightlights in the snapshot v1beta1 APIs

* DeletionPolicy is a required field in both VolumeSnapshotClass and VolumeSnapshotContent. This way the user has to explicitly specify it, leaving no room for confusion.
* VolumeSnapshotSpec has a required Source field. Source may be either a PersistentVolumeClaimName (if dynamically provisioning a snapshot) or VolumeSnapshotContentName (if pre-provisioning a snapshot).
* VolumeSnapshotContentSpec has a required Source field. This Source may be either a VolumeHandle (if dynamically provisioning a snapshot) or a SnapshotHandle (if pre-provisioning volume snapshots).
* VolumeSnapshot contains a Status to indicate the current state of the volume snapshot. It has a field BoundVolumeSnapshotContentName to indicate the VolumeSnapshot object is bound to a VolumeSnapshotContent.
* VolumeSnapshotContent contains a Status to indicate the current state of the volume snapshot content. It has a field SnapshotHandle to indicate that the VolumeSnapshotContent represents a snapshot on the storage system.


## Usage

The Volume Snapshot feature now depends on a new, volume snapshot controller in addition to the volume snapshot CRDs. Both the volume snapshot controller and the CRDs are independent of any CSI driver. Regardless of the number CSI drivers deployed on the cluster, there must be only one instance of the volume snapshot controller running and one set of volume snapshot CRDs installed per cluster.

Therefore, it is strongly recommended that Kubernetes distributors bundle and deploy the controller and CRDs as part of their Kubernetes cluster management process (independent of any CSI Driver).

If your cluster does not come pre-installed with the correct components, you may manually install these components by executing the following steps.

Install Snapshot Beta CRDs:
* kubectl create -f config/crd
* https://github.com/kubernetes-csi/external-snapshotter/tree/master/config/crd
* Do this once per cluster

Install Common Snapshot Controller:
* kubectl create -f deploy/kubernetes/snapshot-controller
* https://github.com/kubernetes-csi/external-snapshotter/tree/master/deploy/kubernetes/snapshot-controller
* Do this once per cluster

Install CSI Driver:
* Follow instructions provided by your CSI Driver vendor.
* Here is an example to install the sample hostpath CSI driver
  * kubectl create -f deploy/kubernetes/csi-snapshotter
  * https://github.com/kubernetes-csi/external-snapshotter/tree/master/deploy/kubernetes/csi-snapshotter

### Snapshot controller command line options

#### Important optional arguments that are highly recommended to be used
* `--leader-election`: Enables leader election. This is useful when there are multiple replicas of the same snapshot controller running for the same Kubernetes deployment. Only one of them may be active (=leader). A new leader will be re-elected when current leader dies or becomes unresponsive for ~15 seconds.

* `--leader-election-namespace <namespace>`: The namespace where the leader election resource exists. Defaults to the pod namespace if not set.

* `--metrics-address`: The TCP network address where the prometheus metrics endpoint will run (example: `:8080` which corresponds to port 8080 on local host). The default is empty string, which means metrics endpoint is disabled.

* `--metrics-path`: The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.

#### Other recognized arguments
* `--kubeconfig <path>`: Path to Kubernetes client configuration that the snapshot controller uses to connect to Kubernetes API server. When omitted, default token provided by Kubernetes will be used. This option is useful only when the snapshot controller does not run as a Kubernetes pod, e.g. for debugging.

* `--resync-period <duration>`: Internal resync interval when the snapshot controller re-evaluates all existing `VolumeSnapshot` instances and tries to fulfill them, i.e. create / delete corresponding snapshots. It does not affect re-tries of failed calls! It should be used only when there is a bug in Kubernetes watch logic. Default is 60 seconds.

* `--version`: Prints current snapshot controller version and quits.

* All glog / klog arguments are supported, such as `-v <log level>` or `-alsologtostderr`.

### CSI external snapshotter sidecar command line options

#### Important optional arguments that are highly recommended to be used
* `--csi-address <path to CSI socket>`: This is the path to the CSI driver socket inside the pod that the external-snapshotter container will use to issue CSI operations (`/run/csi/socket` is used by default).

* `--leader-election`: Enables leader election. This is useful when there are multiple replicas of the same external-snapshotter running for one CSI driver. Only one of them may be active (=leader). A new leader will be re-elected when current leader dies or becomes unresponsive for ~15 seconds.

* `--leader-election-namespace <namespace>`: The namespace where the leader election resource exists. Defaults to the pod namespace if not set.

* `--timeout <duration>`: Timeout of all calls to CSI driver. It should be set to value that accommodates majority of `CreateSnapshot`, `DeleteSnapshot`, and `ListSnapshots` calls. 1 minute is used by default.

* `snapshot-name-prefix`: Prefix to apply to the name of a created snapshot. Default is `snapshot`.

* `snapshot-name-uuid-length`: Length in characters for the generated uuid of a created snapshot. Defaults behavior is to NOT truncate.

#### Other recognized arguments
* `--kubeconfig <path>`: Path to Kubernetes client configuration that the CSI external-snapshotter uses to connect to Kubernetes API server. When omitted, default token provided by Kubernetes will be used. This option is useful only when the external-snapshotter does not run as a Kubernetes pod, e.g. for debugging.

* `--resync-period <duration>`: Internal resync interval when the CSI external-snapshotter re-evaluates all existing `VolumeSnapshotContent` instances and tries to fulfill them, i.e. update / delete corresponding snapshots. It does not affect re-tries of failed CSI calls! It should be used only when there is a bug in Kubernetes watch logic. Default is 60 seconds.

* `--version`: Prints current CSI external-snapshotter version and quits.

* All glog / klog arguments are supported, such as `-v <log level>` or `-alsologtostderr`.


## Upgrade from v1alpha1 to v1beta1

The change from v1alpha1 to v1beta1 snapshot APIs is not backward compatible.

If you have already deployed v1alpha1 snapshot APIs and external-snapshotter sidecar controller and want to upgrade to v1beta1, you need to do the following:
* <strong>Note: The underlying snapshots on the storage system will be deleted in the upgrade process!!!</strong>
1. Delete volume snapshots created using v1alpha1 snapshot CRDs and external-snapshotter sidecar controller.
2. Uninstall v1alpha1 snapshot CRDs, external-snapshotter sidecar controller, and CSI driver.
3. Install v1beta1 snapshot CRDs, snapshot controller, CSI external-snapshotter sidecar and CSI driver.


## Testing

Running Unit Tests:

```bash
go test -timeout 30s  github.com/kubernetes-csi/external-snapshotter/pkg/common-controller

go test -timeout 30s  github.com/kubernetes-csi/external-snapshotter/pkg/sidecar-controller
```


## Dependency Management

external-snapshotter uses [go modules](https://blog.golang.org/using-go-modules).


## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

* [Slack channel](https://kubernetes.slack.com/messages/sig-storage)
* [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-storage)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
