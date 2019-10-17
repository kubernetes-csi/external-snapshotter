# Changelog since v1.0.1

## Notable Changes

- Cherry picks PR #172: Added extra verification of source PersistentVolumeClaim before creating snapshot.([#175](https://github.com/kubernetes-csi/external-snapshotter/pull/175), [@xing-yang](https://github.com/xing-yang))

# Changelog since v0.4.1

## Breaking Changes

- Switch to use TypedLocalObjectReference in core API.([#16](https://github.com/kubernetes-csi/external-snapshotter/pull/16), [@xing-yang](https://github.com/xing-yang))
- Update snapshotter to use CSI spec 1.0.([#58](https://github.com/kubernetes-csi/external-snapshotter/pull/58), [@msau42](https://github.com/msau42))
- Call CreateSnapshot to check if snapshot is processed instead of ListSnapshots.([#61](https://github.com/kubernetes-csi/external-snapshotter/pull/61), [@xing-yang](https://github.com/xing-yang))
- Bumping k8s version to 1.13.0-beta.1.([#64](https://github.com/kubernetes-csi/external-snapshotter/pull/64), [@verult](https://github.com/verult))
- Rename `Ready` to `ReadyToUse` in the `Status` field of `VolumeSnapshot` API object.([#74](https://github.com/kubernetes-csi/external-snapshotter/pull/74), [@xing-yang](https://github.com/xing-yang))

## Actions Required

- CSI plugin must support the 1.0 spec. CSI spec versions < 1.0 are no longer supported.
    - In CSI v1.0, `SnapshotStatus` is removed from `CreateSnapshotResponse`, and is replaced with a Boolean `ReadyToUse`.
    - The snapshot controller now calls `CreateSnapshot` instead of `ListSnapshots` to check if snapshot is processed after it is cut. Driver maintainer needs to make changes accordingly in the driver.
    - In CSI v1.0, .google.protobuf.Timestamp is used instead of int64 to represent the creation time of a snapshot. Driver maintainer needs to make changes accordingly in the driver. The creation time is visible to the admin when running `kubectl describe volumesnapshotcontent` to examine the details of a `VolumeSnapshotContent`.

- The renaming from `Ready` to `ReadyToUse` in the `Status` field of `VolumeSnapshot` API object is visible to the user when running `kubectl describe volumesnapshot` to view the details of a snapshot.

- External-provisioner sidecar container which depends on external-snapshotter APIs needs to be updated whenever there is an API change in external-snapshotter. Compatible sidecar container images for external-provisioner and external-snapshotter need to be used together, i.e., both are v1.0.0 or both are v1.0.1.

## Deprecations

- The following VolumeSnapshotClass parameters are deprecated and will be removed in a future release:

| Deprecated                                          | Replacement                                                                 |
| ------------------------------------ | --------------------------------------------------- |
| csiSnapshotterSecretName               | csi.storage.k8s.io/snapshotter-secret-name             |
| csiSnapshotterSecretNameSpace    | csi.storage.k8s.io/snapshotter-secret-namespace   | 

## Major Changes

- Add classListerSynced for WaitForCacheSync.([#74](https://github.com/kubernetes-csi/external-snapshotter/pull/74), [@wackxu](https://github.com/wackxu))
- Fix initializeCaches bug.([#74](https://github.com/kubernetes-csi/external-snapshotter/pull/74), [@wackxu](https://github.com/wackxu))
- Deploy: split out RBAC definitions.([#53](https://github.com/kubernetes-csi/external-snapshotter/pull/53), [@pohly](https://github.com/pohly))
- Update unit tests and disable broken status unit tests.([#58](https://github.com/kubernetes-csi/external-snapshotter/pull/58), [@msau42](https://github.com/msau42))
- Use existing content name if already in snapshot object for static provisioning, otherwise construct new name for content for dynamic provisioning.([#65](https://github.com/kubernetes-csi/external-snapshotter/pull/65), [@xing-yang](https://github.com/xing-yang))
- Add `VolumeSnapshotContent` deletion policy.([#74](https://github.com/kubernetes-csi/external-snapshotter/pull/74), [@jingxu97](https://github.com/jingxu97))
- Add `VolumeSnapshot` and `VolumeSnapshotContent` in Use Protection using Finalizers.([#74](https://github.com/kubernetes-csi/external-snapshotter/pull/74), [@xing-yang](https://github.com/xing-yang))
- Cherry-pick #76: Use protosanitizer library so secrets will be stripped from the logs.([#77](https://github.com/kubernetes-csi/external-snapshotter/pull/77), [@msau42](https://github.com/msau42))
- Adds new reserved prefixed parameter keys which are stripped out of parameter list, and adds deprecation notice for old keys and keep their behavior the same.([#79](https://github.com/kubernetes-csi/external-snapshotter/pull/79), [@xing-yang](https://github.com/xing-yang))
