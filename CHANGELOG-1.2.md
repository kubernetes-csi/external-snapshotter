# Changelog since v1.2.1

## Notable Changes

- Cherry picks PR #172: Added extra verification of source PersistentVolumeClaim before creating snapshot.([#173](https://github.com/kubernetes-csi/external-snapshotter/pull/173), [@xing-yang](https://github.com/xing-yang))

# Changelog since v1.2.0

## Notable Changes

- Cherry picks PR #138: Prebound snapshots will work correctly with CSI drivers that does not support ListSnasphots.([#156](https://github.com/kubernetes-csi/external-snapshotter/pull/156), [@hakanmemisoglu](https://github.com/hakanmemisoglu))

# Changelog since v1.1.0

## Breaking Changes

- Changes the API group name for the fake VolumeSnapshot object to "snapshot.storage.k8s.io" to be in-sync with the group name of the real VolumeSnapshot object. As a result, the generated interfaces for clientset and informers of VolumeSnapshot are also changed from "VolumeSnapshot" to "Snapshot". ([#123](https://github.com/kubernetes-csi/external-snapshotter/pull/123), [@xing-yang](https://github.com/xing-yang))

## New Features

- Adds Finalizer on the snapshot source PVC to prevent it from being deleted when a snapshot is being created from it. ([#47](https://github.com/kubernetes-csi/external-snapshotter/pull/47), [@xing-yang](https://github.com/xing-yang))

## Other Notable Changes

- Add Status subresource for VolumeSnapshot. ([#121](https://github.com/kubernetes-csi/external-snapshotter/pull/121), [@zhucan](https://github.com/zhucan))
