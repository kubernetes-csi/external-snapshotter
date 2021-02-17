# Release notes for v2.1.4 (Changelog since v2.1.3)

## Changes by Kind

### Bug or Regression

- Cherry pick of #287: Allows the sidecar to delete volume snapshots if the volume snapshot class is not found. ([#466](https://github.com/kubernetes-csi/external-snapshotter/pull/466), [@ialidzhikov](https://github.com/ialidzhikov))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# Release notes for v2.1.3 (Changelog since v2.1.2)

## Changes by Kind

### Uncategorized

- Update release-tools and prow script for release-2.1 ([#448](https://github.com/kubernetes-csi/external-snapshotter/pull/448), [@mattcary](https://github.com/mattcary))

### Bug or Regression

- Backports fix #381 for crashloop when there are errors in the VolumeSnapshot, like a missing VolumeSnapshotClass. ([#446](https://github.com/kubernetes-csi/external-snapshotter/pull/446), [@mattcary](https://github.com/mattcary))

## Dependencies

### Added
_Nothing has changed._

### Changed
_Nothing has changed._

### Removed
_Nothing has changed._

# Release notes for v2.1.2 (Changelog since v2.1.1)

## Changes by Kind

### Uncategorized

- release-2.1: update release-tools ([#392](https://github.com/kubernetes-csi/external-snapshotter/pull/392), [@Jiawei0227](https://github.com/Jiawei0227))
  - Build with Go 1.15

## Dependencies

### Added
_Nothing has changed._

### Changed
- github.com/kubernetes-csi/csi-lib-utils: [v0.7.0 → v0.7.1](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.7.0...v0.7.1)

### Removed
_Nothing has changed._


# v2.1.1 (Changelog since v2.1.0)

## Bug Fixes

### Snapshot Controller

- Cherry pick PR #293: Fixes issue #290. Disallow a pre-provisioned VolumeSnapshot pointing to a dynamically created VolumeSnapshotContent. ([#303](https://github.com/kubernetes-csi/external-snapshotter/pull/303), [@yuxiangqian](https://github.com/yuxiangqian))
- Cherry pick PR #293: Fixes issue #291. Verify VolumeSnapshot and VolumeSnapshotContent are bi-directional bound before initializing a deletion on a VolumeSnapshotContent which the to-be-deleted VolumeSnapshot points to. ([#303](https://github.com/kubernetes-csi/external-snapshotter/pull/303), [@yuxiangqian](https://github.com/yuxiangqian))
- Cherry pick PR #293: Fixes issue #292. Allow deletion of a VolumeSnapshot when the VolumeSnapshotContent's DeletionPolicy has been updated from Delete to Retain. ([#303](https://github.com/kubernetes-csi/external-snapshotter/pull/303), [@yuxiangqian](https://github.com/yuxiangqian))

# v2.1.0 (Changelog since v2.0.0)

## New Features

### Snapshot Controller

- The number of worker threads in the snapshot-controller is now configurable via the `worker-threads` flag. ([#282](https://github.com/kubernetes-csi/external-snapshotter/pull/282), [@huffmanca](https://github.com/huffmanca))

### CSI External-Snapshotter Sidecar

- The number of worker threads in the csi-snapshotter is now configurable via the `worker-threads` flag. ([#282](https://github.com/kubernetes-csi/external-snapshotter/pull/282), [@huffmanca](https://github.com/huffmanca))
- Adds support for ListSnapshots secrets ([#252](https://github.com/kubernetes-csi/external-snapshotter/pull/252), [@bells17](https://github.com/bells17))

## Breaking Changes

- Update package path to v2. Vendoring with dep depends on https://github.com/golang/dep/pull/1963 or the workaround described in v2/README.md. ([#240](https://github.com/kubernetes-csi/external-snapshotter/pull/240), [@alex1989hu](https://github.com/alex1989hu))

## Bug Fixes

### Snapshot Controller

- Fixes a problem of not removing the PVC finalizer when it is no longer used by a snapshot as source. ([#283](https://github.com/kubernetes-csi/external-snapshotter/pull/283), [@xing-yang](https://github.com/xing-yang))
- Fixes a problem deleting VolumeSnapshotContent with `Retain` policy for pre-provisioned snapshots. ([#249](https://github.com/kubernetes-csi/external-snapshotter/pull/249), [@xing-yang](https://github.com/xing-yang))
- Allows the volume snapshot to be deleted if the volume snapshot class is not found. ([#275](https://github.com/kubernetes-csi/external-snapshotter/pull/275), [@huffmanca](https://github.com/huffmanca))

### CSI External-Snapshotter Sidecar

- Fixes a create snapshot timeout issue. ([#261](https://github.com/kubernetes-csi/external-snapshotter/pull/261), [@xing-yang](https://github.com/xing-yang))

## Other Notable Changes

### API Changes

- Prints additional details when using kubectl get on VolumeSnapshot, VolumeSnapshotContent, and VolumeSnapshotClass API objects. ([#260](https://github.com/kubernetes-csi/external-snapshotter/pull/260), [@huffmanca](https://github.com/huffmanca))
