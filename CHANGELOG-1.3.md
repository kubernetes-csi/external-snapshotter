# Changelog since v1.2.0

## Breaking Changes

- Add a prefix "external-snapshotter-leader" and the driver name to the snapshotter leader election lock name. Rolling update will not work if leader election is enabled because the lock name is changed in v1.3.0. ([#129](https://github.com/kubernetes-csi/external-snapshotter/pull/129), [@zhucan](https://github.com/zhucan))

## Other Notable Changes

- Snapshotter will no longer call ListSnapshots if the CSI Driver does not support this operation. ([#138](https://github.com/kubernetes-csi/external-snapshotter/pull/138), [@ggriffiths](https://github.com/ggriffiths))
