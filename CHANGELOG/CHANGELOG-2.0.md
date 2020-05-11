# Changelog since v1.2.0

## New Features

- Split the external-snapshotter sidecar controller into two controllers, a snapshot-controller and an external-snapshotter sidecar. Only the external-snapshotter sidecar should be deployed with the CSI driver. ([#182](https://github.com/kubernetes-csi/external-snapshotter/pull/182), [@xing-yang](https://github.com/xing-yang))

### Snapshot Controller
- Add a finalizer on VolumeSnapshot object to protect it from being deleted when it is bound to
 a VolumeSnapshotContent. ([#182](https://github.com/kubernetes-csi/external-snapshotter/pull/182), [@xing-yang](https://github.com/xing-yang))
- Adds deletion secret as annotation to volume snapshot content. ([#165](https://github.com/kubernetes-csi/external-snapshotter/pull/165), [@xing-yang](https://github.com/xing-yang))

### CSI External-Snapshotter Sidecar
- Add prometheus metrics to CSI external-snapshotter under the /metrics endpoint. This can be enabled via the "--metrics-address" and "--metrics-path" options. ([#227](https://github.com/kubernetes-csi/external-snapshotter/pull/227), [@saad-ali](https://github.com/saad-ali))
- Adds deletion secret as annotation to volume snapshot content. ([#165](https://github.com/kubernetes-csi/external-snapshotter/pull/165), [@xing-yang](https://github.com/xing-yang))

### Breaking Changes

#### API Changes
- Changes VolumeSnapshot CRD version from v1alpha1 to v1beta1. v1alpha1 is no longer supported. ([#139](https://github.com/kubernetes-csi/external-snapshotter/pull/139), [@xing-yang](https://github.com/xing-yang))

#### Snapshot Controller
- Removes createSnapshotContentRetryCount and createSnapshotContentInterval
  from command line options. ([#211](https://github.com/kubernetes-csi/external-snapshotter/pull/211), [@xing-yang](https://github.com/xing-yang))

#### CSI External-Snapshotter Sidecar
- Add a prefix "external-snapshotter-leader" and the driver name to the snapshotter leader election lock name. Rolling update will not work if leader election is enabled because the lock name is changed in v2.0.0. ([#129](https://github.com/kubernetes-csi/external-snapshotter/pull/129), [@zhucan](https://github.com/zhucan))

### Bug Fixes

#### Snapshot Controller
- Added extra verification of source PersistentVolumeClaim before creating snapshot. ([#172](https://github.com/kubernetes-csi/external-snapshotter/pull/172), [@xing-yang](https://github.com/xing-yang))
- Fixes issue when SelfLink removal is turned on in Kubernetes. ([#160](https://github.com/kubernetes-csi/external-snapshotter/pull/160), [@msau42](https://github.com/msau42))

#### CSI External-Snapshotter Sidecar
- Snapshotter will no longer call ListSnapshots if the CSI Driver does not support this operation. ([#138](https://github.com/kubernetes-csi/external-snapshotter/pull/138), [@ggriffiths](https://github.com/ggriffiths))
- Fixes issue when SelfLink removal is turned on in Kubernetes. ([#160](https://github.com/kubernetes-csi/external-snapshotter/pull/160), [@msau42](https://github.com/msau42))

### Other Notable Changes

#### Snapshot Controller
- Migrated to Go modules, so the source builds also outside of GOPATH. ([#179](https://github.com/kubernetes-csi/external-snapshotter/pull/179), [@pohly](https://github.com/pohly))

#### CSI External-Snapshotter Sidecar
- Migrated to Go modules, so the source builds also outside of GOPATH. ([#179](https://github.com/kubernetes-csi/external-snapshotter/pull/179), [@pohly](https://github.com/pohly))
- We will now exit the external-snapshotter when the connection to a CSI driver is lost, allowing for another leader to takeover. ([#171](https://github.com/kubernetes-csi/external-snapshotter/pull/171), [@ggriffiths](https://github.com/ggriffiths))
