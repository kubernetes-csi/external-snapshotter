# CSI Snapshotter

The CSI snapshotter is part of Kubernetes implementation of [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec) and implements both the volume snapshot and the volume group snapshot feature.

The volume snapshot feature supports CSI v1.0 and higher. It was introduced as an Alpha feature in Kubernetes v1.12 and has been promoted to a Beta feature in Kubernetes 1.17. In Kubernetes 1.20, the volume snapshot feature moves to GA.

The volume group snapshot feature supports CSI v1.10.0 and higher, and have been introduced in [Kubernetes 1.27 as an alpha feature](https://kubernetes.io/blog/2023/05/08/kubernetes-1-27-volume-group-snapshot-alpha/).

> :warning: **WARNING**: The validation webhook was deprecated in v8.0.0 and it is now removed. The validation webhook would prevent creating multiple default volume snapshot classes and multiple default volume group snapshot classes for the same CSI driver. With the removal of the validation webhook, an error will still be raised when dynamically provisioning a VolumeSnapshot or VolumeGroupSnapshot when multiple default volume snapshot classes or multiple default volume group snapshot classes for the same CSI driver exist.

## Overview

With the promotion of Volume Snapshot to GA, the feature is enabled by default on standard Kubernetes deployments and cannot be turned off.

The Volume Group Snapshot feature is disabled by default, and can be enabled using a command line option. For more information on how to enable it, see the [usage section](https://github.com/kubernetes-csi/external-snapshotter#usage) of this guide.

Blog post for the Snapshot GA feature can be found [here](https://kubernetes.io/blog/2020/12/10/kubernetes-1.20-volume-snapshot-moves-to-ga/).

Blog post for the Volume Group Snapshot Alpha feature can be found [here](https://kubernetes.io/blog/2023/05/08/kubernetes-1-27-volume-group-snapshot-alpha/).

## Compatibility

This information reflects the head of this branch.

| Minimum CSI Version                                                                | Recommended CSI Version                                                                | Container Image             | [Min K8s Version](https://kubernetes-csi.github.io/docs/kubernetes-compatibility.html#minimum-version) | [Recommended K8s Version](https://kubernetes-csi.github.io/docs/project-policies.html#recommended-version) |
| ------------------------------------------------------------------------------------------ | ----------------------------| --------------- | --------------- |  --------------- |
| [CSI Spec v1.0.0](https://github.com/container-storage-interface/spec/releases/tag/v1.0.0) | [CSI Spec v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0) | k8s.gcr.io/sig-storage/csi-snapshotter | 1.20         | 1.20         |
| [CSI Spec v1.0.0](https://github.com/container-storage-interface/spec/releases/tag/v1.0.0) | [CSI Spec v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0) | k8s.gcr.io/sig-storage/snapshot-controller  | 1.20     | 1.20         |
| [CSI Spec v1.0.0](https://github.com/container-storage-interface/spec/releases/tag/v1.0.0) | [CSI Spec v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0) | k8s.gcr.io/sig-storage/snapshot-validation-webhook  | 1.20     | 1.20         |

Note: snapshot-controller, csi-snapshotter v4.1 requires v1 snapshot CRDs to be installed, but it serves both v1 and v1beta1 snapshot objects. Storage version is changed from v1beta1 to v1 in 4.1.0 so v1beta1 is deprecated and will be removed in a future release.

Note: when the volume group snapshot feature is enabled, snapshot-controller, csi-snapshotter require the v1alpha1 volumegroupsnapshot CRDs to be installed.

## Feature Status

The `VolumeSnapshotDataSource` feature gate was introduced in Kubernetes 1.12 and it is enabled by default in Kubernetes 1.17 when the volume snapshot feature is promoted to beta. In Kubernetes 1.20, the feature gate is enabled by default on standard Kubernetes deployments and cannot be turned off.

## Design

Both the snapshot controller and CSI external-snapshotter sidecar follow [controller](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-api-machinery/controllers.md) pattern and uses informers to watch for events. The snapshot controller watches for `VolumeSnapshot` and `VolumeSnapshotContent` create/update/delete events. Similarly, when the volume group snapshot feature is enabled, the snapshot controller watches for `VolumeGroupSnapshot` and `VolumeGroupSnapshotContent` create/update/delete events.

The CSI external-snapshotter sidecar only watches for `VolumeSnapshotContent` create/update/delete events. It filters out these objects with `Driver==<CSI driver name>` specified in the associated VolumeSnapshotClass object and then processes these events in workqueues with exponential backoff. Similarly, when the volume group snapshot feature is enabled, the CSI external-controller sidecar only watches for `VolumeGroupSnapshotContent` create/update/delete events.

The CSI external-snapshotter sidecar talks to CSI over socket (/run/csi/socket by default, configurable by -csi-address).

### Snapshot v1 APIs

In the current release, both v1 and v1beta1 APIs are served while the stored API version is changed from v1beta1 to v1. v1beta1 APIs is deprecated and will be removed in a future release. It is recommended for users to switch to v1 APIs as soon as possible. Any previously created invalid v1beta1 objects have to be deleted before upgrading to version 4.1.

### Volume Group Snapshot v1alpha1 APIs

When enabled, the VolumeGroupSnapshot v1alpha1 APIs are being served.

## Usage

Volume Snapshot feature contains the following components:

* [Kubernetes Volume Snapshot and Volume Group Snapshot CRDs](https://github.com/kubernetes-csi/external-snapshotter/tree/master/client/config/crd)
* [Volume snapshot and volume group snapshot controller](https://github.com/kubernetes-csi/external-snapshotter/tree/master/pkg/common-controller)
* CSI Driver along with [CSI Snapshotter sidecar](https://github.com/kubernetes-csi/external-snapshotter/tree/master/pkg/sidecar-controller)

The Volume Snapshot feature depends on a volume snapshot controller and the volume snapshot CRDs. Both the controller and the CRDs are independent of any CSI driver. The CSI Snapshotter sidecar must run once per CSI driver. The single snapshot controller deployment works for all CSI drivers in a cluster. With leader election configured, the CSI sidecars and snapshot controller elect one leader per deployment. If deployed with two or more pods and leader election is enabled, the non-leader containers will attempt to get the lease. If the leader container dies, a non-leader will take over.

Therefore, it is strongly recommended that Kubernetes distributors bundle and deploy the controller and CRDs as part of their Kubernetes cluster management process (independent of any CSI Driver).

If your Kubernetes distribution does not bundle the snapshot controller, you may manually install these components by executing the following steps. Note that the snapshot controller YAML files in the git repository deploy into the default namespace for system testing purposes. For general use, update the snapshot controller YAMLs with an appropriate namespace prior to installing. For example, on a Vanilla Kubernetes cluster update the namespace from 'default' to 'kube-system' prior to issuing the kubectl create command.

Install Snapshot and Volume Group Snapshot CRDs:
* With the repo cloned locally: `kubectl kustomize client/config/crd | kubectl create -f -`
* From the repo remotely: `kubectl kustomize https://github.com/kubernetes-csi/external-snapshotter/client/config/crd | kubectl create -f -`
* Do this once per cluster

Install Common Snapshot Controller:
* Update the namespace to an appropriate value for your environment (e.g. kube-system)
* `kubectl -n kube-system kustomize deploy/kubernetes/snapshot-controller | kubectl create -f -`
* Do this once per cluster

Install CSI Driver:
* Follow instructions provided by your CSI Driver vendor.
* Here is an example to install the sample hostpath CSI driver
  * With the repo cloned locally: `kubectl kustomize deploy/kubernetes/csi-snapshotter | kubectl create -f -`
  * From the repo remotely: `kubectl kustomize https://github.com/kubernetes-csi/external-snapshotter/deploy/kubernetes/csi-snapshotter | kubectl create -f -`

##### Volume Snapshot

* Spec.VolumeSnapshotClassName must not be an empty string or nil on creation
* Spec.Source.PersistentVolumeClaimName must not be changed on update requests
* Spec.Source.VolumeSnapshotContentName must not be changed on update requests

##### Volume Snapshot Content

* Spec.VolumeSnapshotRef.Name must not be an empty string on creation
* Spec.VolumeSnapshotRef.Namespace must not be an empty string on creation
* Spec.Source.VolumeHandle must not be changed on update requests
* Spec.Source.SnapshotHandle must not be changed on update requests
* Spec.SourceVolumeMode must not be changes on update requests

##### Volume Snapshot Classes

* There can only be a single default volume snapshot class for a particular driver.

### Distributed Snapshotting

The distributed snapshotting feature is provided to handle snapshot operations for local volumes. To use this functionality, the snapshotter sidecar should be deployed along with the csi driver on each node so that every node manages the snapshot operations only for the volumes local to that node. This feature can be enabled by setting the following command line options to true:

#### Snapshot controller option

* `--enable-distributed-snapshotting`: This option lets the snapshot controller know that distributed snapshotting is enabled and the snapshotter sidecar will be running on each node. Off by default.

#### CSI external snapshotter sidecar option

* `--node-deployment`: Enables the snapshotter sidecar to handle snapshot operations for the volumes local to the node on which it is deployed. Off by default.

Other than this, the NODE_NAME environment variable must be set where the CSI snapshotter sidecar is deployed. The value of NODE_NAME should be the name of the node where the sidecar is running.

### Volume Group Snapshot Support

The following requisites must be met to enable the volume group snapshot feature:

* the Volume Group Snapshot CRDs are installed in the cluster
* the `--feature-gates=CSIVolumeGroupSnapshot=true` option is being passed to the snapshot controller
* the `--feature-gates=CSIVolumeGroupSnapshot=true` option is being passed to the CSI snapshotter sidecar

Specifically, `deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml` needs to be updated with `--feature-gates=CSIVolumeGroupSnapshot=true` in order to enable this feature in the snapshot controller.

### Snapshot controller command line options

#### Important optional arguments that are highly recommended to be used
* `--leader-election`: Enables leader election. This is useful when there are multiple replicas of the same snapshot controller running for the same Kubernetes deployment. Only one of them may be active (=leader). A new leader will be re-elected when current leader dies or becomes unresponsive for ~15 seconds.

* `--leader-election-namespace <namespace>`: The namespace where the leader election resource exists. Defaults to the pod namespace if not set.

* `--leader-election-lease-duration <duration>`: Duration, in seconds, that non-leader candidates will wait to force acquire leadership. Defaults to 15 seconds.

* `--leader-election-renew-deadline <duration>`: Duration, in seconds, that the acting leader will retry refreshing leadership before giving up. Defaults to 10 seconds.

* `--leader-election-retry-period <duration>`: Duration, in seconds, the LeaderElector clients should wait between tries of actions. Defaults to 5 seconds.

* `--kube-api-qps <num>`: QPS for clients that communicate with the kubernetes apiserver. Defaults to `5.0`.

* `--kube-api-burst <num>`: Burst for clients that communicate with the kubernetes apiserver. Defaults to `10`.

* `--http-endpoint`: The TCP network address where the HTTP server for diagnostics, including metrics and leader election health check, will listen (example: `:8080` which corresponds to port 8080 on local host). The default is empty string, which means the server is disabled.

* `--metrics-path`: The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.

* `--worker-threads`: Number of worker threads. Default value is 10.

* `--retry-interval-start`: Initial retry interval of failed volume snapshot creation or deletion. It doubles with each failure, up to retry-interval-max. Default value is 1 second.

* `--retry-interval-max`: Maximum retry interval of failed volume snapshot creation or deletion. Default value is 5 minutes.

* `--retry-crd-interval-max`: Maximum retry duration for detecting the snapshot CRDs on controller startup. Default is 30 seconds.

* `--enable-distributed-snapshotting` : Enables each node to handle snapshots for the volumes local to that node. Off by default. It should be set to true only if `--node-deployment` parameter for the csi external snapshotter sidecar is set to true. See https://github.com/kubernetes-csi/external-snapshotter/blob/master/README.md#distributed-snapshotting for details.

* `--prevent-volume-mode-conversion`: Boolean that prevents an unauthorised user from modifying the volume mode when creating a PVC from an existing VolumeSnapshot. Was present as an alpha feature in `v6.0.0`; Having graduated to beta, defaults to true.

#### Volume Group Snapshot support

* `--feature-gates=CSIVolumeGroupSnapshot=true`: Enables support for Volume Group Snapshots. If this option is enabled, the VolumeGroupSnapshots CRD should be available on the cluster.

#### Other recognized arguments
* `--kubeconfig <path>`: Path to Kubernetes client configuration that the snapshot controller uses to connect to Kubernetes API server. When omitted, default token provided by Kubernetes will be used. This option is useful only when the snapshot controller does not run as a Kubernetes pod, e.g. for debugging.

* `--resync-period <duration>`: Internal resync interval when the snapshot controller re-evaluates all existing `VolumeSnapshot` instances and tries to fulfill them, i.e. create / delete corresponding snapshots. It does not affect re-tries of failed calls! It should be used only when there is a bug in Kubernetes watch logic. Default is 15 minutes.

* `--version`: Prints current snapshot controller version and quits.

* All glog / klog arguments are supported, such as `-v <log level>` or `-alsologtostderr`.

### CSI external snapshotter sidecar command line options

#### Important optional arguments that are highly recommended to be used
* `--csi-address <path to CSI socket>`: This is the path to the CSI driver socket inside the pod that the external-snapshotter container will use to issue CSI operations (`/run/csi/socket` is used by default).

* `--leader-election`: Enables leader election. This is useful when there are multiple replicas of the same external-snapshotter running for one CSI driver. Only one of them may be active (=leader). A new leader will be re-elected when current leader dies or becomes unresponsive for ~15 seconds.

* `--leader-election-namespace <namespace>`: The namespace where the leader election resource exists. Defaults to the pod namespace if not set.

* `--leader-election-lease-duration <duration>`: Duration, in seconds, that non-leader candidates will wait to force acquire leadership. Defaults to 15 seconds.

* `--leader-election-renew-deadline <duration>`: Duration, in seconds, that the acting leader will retry refreshing leadership before giving up. Defaults to 10 seconds.

* `--leader-election-retry-period <duration>`: Duration, in seconds, the LeaderElector clients should wait between tries of actions. Defaults to 5 seconds.

* `--kube-api-qps <num>`: QPS for clients that communicate with the kubernetes apiserver. Defaults to `5.0`.

* `--kube-api-burst <num>`: Burst for clients that communicate with the kubernetes apiserver. Defaults to `10`.

* `--timeout <duration>`: Timeout of all calls to CSI driver. It should be set to value that accommodates majority of `CreateSnapshot`, `DeleteSnapshot`, and `ListSnapshots` calls. 1 minute is used by default.

* `snapshot-name-prefix`: Prefix to apply to the name of a created snapshot. Default is `snapshot`.

* `snapshot-name-uuid-length`: Length in characters for the generated uuid of a created snapshot. Defaults behavior is to NOT truncate.

* `--worker-threads`: Number of worker threads for running create snapshot and delete snapshot operations. Default value is 10.

* `--node-deployment`: Enables deploying the sidecar controller together with a CSI driver on nodes to manage node-local volumes. Off by default. This should be set to true along with the `--enable-distributed-snapshotting` in the snapshot controller parameters to make use of distributed snapshotting. See https://github.com/kubernetes-csi/external-snapshotter/blob/master/README.md#distributed-snapshotting for details.

* `--retry-interval-start`: Initial retry interval of failed volume snapshot creation or deletion. It doubles with each failure, up to retry-interval-max. Default value is 1 second.

* `--retry-interval-max`: Maximum retry interval of failed volume snapshot creation or deletion. Default value is 5 minutes.

#### Volume Group Snapshot support

* `--feature-gates=CSIVolumeGroupSnapshot=true`: Enables support for Volume Group Snapshots. If this option is enabled, the VolumeGroupSnapshots CRD should be available on the cluster.

#### Other recognized arguments
* `--kubeconfig <path>`: Path to Kubernetes client configuration that the CSI external-snapshotter uses to connect to Kubernetes API server. When omitted, default token provided by Kubernetes will be used. This option is useful only when the external-snapshotter does not run as a Kubernetes pod, e.g. for debugging.

* `--resync-period <duration>`: Internal resync interval when the CSI external-snapshotter re-evaluates all existing `VolumeSnapshotContent` instances and tries to fulfill them, i.e. update / delete corresponding snapshots. It does not affect re-tries of failed CSI calls! It should be used only when there is a bug in Kubernetes watch logic. Default is 15 minutes.

* `--version`: Prints current CSI external-snapshotter version and quits.

* All glog / klog arguments are supported, such as `-v <log level>` or `-alsologtostderr`.

#### HTTP endpoint

The external-snapshotter optionally exposes an HTTP endpoint at address:port specified by `--http-endpoint` argument. When set, these two paths are exposed:

* Metrics path, as set by `--metrics-path` argument (default is `/metrics`).

* Leader election health check at `/healthz/leader-election`. It is recommended to run a liveness probe against this endpoint when leader election is used to kill external-provisioner leader that fails to connect to the API server to renew its leadership. See https://github.com/kubernetes-csi/csi-lib-utils/issues/66 for details.

## Upgrade

### Upgrade from v1alpha1 to v1beta1

The change from v1alpha1 to v1beta1 snapshot APIs is not backward compatible.

If you have already deployed v1alpha1 snapshot APIs and external-snapshotter sidecar controller and want to upgrade to v1beta1, you need to do the following:
* <strong>Note: The underlying snapshots on the storage system will be deleted in the upgrade process!!!</strong>
1. Delete volume snapshots created using v1alpha1 snapshot CRDs and external-snapshotter sidecar controller.
2. Uninstall v1alpha1 snapshot CRDs, external-snapshotter sidecar controller, and CSI driver.
3. Install v1beta1 snapshot CRDs, snapshot controller, CSI external-snapshotter sidecar and CSI driver.

### Upgrade from v1beta1 to v1

* When upgrading to 4.0, change from v1beta1 to v1 is backward compatible because both v1 and v1beta1 are served while the stored API version is still v1beta1. Future releases will switch the stored version to v1 and gradually remove v1beta1 support.
* When upgrading from 3.x to 4.1, change from v1beta1 to v1 is no longer backward compatible because stored API version is changed to v1 although both v1 and v1beta1 are still served. v1beta1 is deprecated in 4.1.
* v1beta1 support will be removed in a future release. It is recommended for users to switch to v1 as soon as possible. Any previously created invalid v1beta1 objects have to be deleted before upgrading to version 4.1.

## Testing

Running Unit Tests:

```bash
go test -timeout 30s  github.com/kubernetes-csi/external-snapshotter/pkg/common-controller

go test -timeout 30s  github.com/kubernetes-csi/external-snapshotter/pkg/sidecar-controller
```

## CRDs and Client Library

Volume snapshot APIs and client library are now in a separate sub-module: `github.com/kubernetes-csi/external-snapshotter/client/v4`.

Use the command `go get -u github.com/kubernetes-csi/external-snapshotter/client/v4@v4.1.0` to get the client library.

### Setting Quota limits with Snapshot custom resources
[`ResourceQuotas`](https://kubernetes.io/docs/concepts/policy/resource-quotas/) are namespaced objects that can be used to set limits on objects of a particular [`Group.Version.Kind`](https://book.kubebuilder.io/cronjob-tutorial/gvks.html). Before we set resource quota, make sure that snapshot CRDs are installed in the cluster. If not please follow [this guide](https://github.com/kubernetes-csi/external-snapshotter#usage).
```
kubectl get crds | grep snapshot
```

Now create a `ResourceQuota` object which sets the limits on number of volumesnapshots that can be created:
```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: snapshot-quota
spec:
  hard:
    count/volumesnapshots.snapshot.storage.k8s.io: "10"
```

If you try to create more snapshots than what is allowed, you will see error like the following:
```
Error from server (Forbidden): error when creating "csi-snapshot.yaml": volumesnapshots.snapshot.storage.k8s.io "new-snapshot-demo" is forbidden: exceeded quota: snapshot-quota, requested: count/volumesnapshots.snapshot.storage.k8s.io=1, used: count/volumesnapshots.snapshot.storage.k8s.io=10, limited: count/volumesnapshots.snapshot.storage.k8s.io=10
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
