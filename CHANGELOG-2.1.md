# Changelog since v2.0.0

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
