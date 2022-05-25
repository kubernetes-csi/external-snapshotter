# Release notes for v6.0.0 

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v5.0.1

## Breaking Changes

### API Removal

- Cherry-pick 704: Remove VolumeSnapshot v1beta1 APIs and generated functions. Please update to VolumeSnapshot v1 APIs as soon as possible. ([#709](https://github.com/kubernetes-csi/external-snapshotter/pull/709), [@RaunakShah](https://github.com/RaunakShah))
- Cherry-pick ([#718](https://github.com/kubernetes-csi/external-snapshotter/pull/718), [@RaunakShah](https://github.com/RaunakShah)): Add VolumeSnapshot v1beta1 manifests back. VolumeSnapshot v1beta1 APIs are no longer served. Please update to VolumeSnapshot v1 APIs as soon as possible. ([#719](https://github.com/kubernetes-csi/external-snapshotter/pull/719), [@xing-yang](https://github.com/xing-yang))

## Changes by Kind

### API Change

- Add SourceVolumeMode field to VolumeSnapshotContents. ([#665](https://github.com/kubernetes-csi/external-snapshotter/pull/665), [@RaunakShah](https://github.com/RaunakShah))
- Cherry-pick #683: Change SourceVolumeMode type to v1.PersistentVolumeMode. ([#686](https://github.com/kubernetes-csi/external-snapshotter/pull/686), [@RaunakShah](https://github.com/RaunakShah))
- Update snapshotter module to v6 and client module to v5. ([#670](https://github.com/kubernetes-csi/external-snapshotter/pull/670), [@RaunakShah](https://github.com/RaunakShah))
- Cherry-pick #673: Upgrade Volume Snapshot client to v6. ([#673](https://github.com/kubernetes-csi/external-snapshotter/pull/673), [@RaunakShah](https://github.com/RaunakShah))

### Feature

#### Snapshot Controller

- Cherry-pick #679: Changes to snapshot controller to add SourceVolumeMode to VolumeSnapshotContents. ([#694](https://github.com/kubernetes-csi/external-snapshotter/pull/694), [@RaunakShah](https://github.com/RaunakShah))

#### Snapshot Validation Webhook

- Cherry-pick #680: Add webhook to make SourceVolumeMode immutable. ([#701](https://github.com/kubernetes-csi/external-snapshotter/pull/701), [@RaunakShah](https://github.com/RaunakShah))
- Cherry-pick 704: Remove validation for VolumeSnapshot v1beta1 API objects from the snapshot validation webhook. ([#709](https://github.com/kubernetes-csi/external-snapshotter/pull/709), [@RaunakShah](https://github.com/RaunakShah))
- Cherry-pick #688: Added admission webhook to ensure that only one VolumeSnapshotClass can be default for each CSI driver. To benefit from this validation, please update your webhook configuration as shown in deploy/kubernetes/webhook-example/admission-configuration-template. ([#700](https://github.com/kubernetes-csi/external-snapshotter/pull/700), [@shawn-hurley](https://github.com/shawn-hurley))
- Cherry-pick #674: Adding validation for VolumeSnapshotClass to only have a single default for a particular driver. ([#693](https://github.com/kubernetes-csi/external-snapshotter/pull/693), [@shawn-hurley](https://github.com/shawn-hurley))
- Cherry-pick #706: Adding RBAC file to webhook example for updated validating webhook. ([#710](https://github.com/kubernetes-csi/external-snapshotter/pull/710), [@shawn-hurley](https://github.com/shawn-hurley))

### Bug or Regression

#### CSI Snapshotter Sidecar

- Fix a problem in CSI snapshotter sidecar that constantly retries CreateSnapshot call on error without exponential backoff. ([#651](https://github.com/kubernetes-csi/external-snapshotter/pull/651), [@zhucan](https://github.com/zhucan))
- Ensure that the CSI snapshotter sidecar allows for CreateSnapshot retries while waiting for a snapshot to be ready. ([#666](https://github.com/kubernetes-csi/external-snapshotter/pull/666), [@pwschuurman](https://github.com/pwschuurman))
- Fixes a nil pointer dereference regression introduced in #666. ([#669](https://github.com/kubernetes-csi/external-snapshotter/pull/669), [@pwschuurman](https://github.com/pwschuurman))

### Other (Cleanup or Flake)

#### CSI Snapshotter Sidecar

- Cherry-pick #689: Remove create and delete access of volumesnapshotcontents resource from csi-snapshotter RBAC as it's no longer required. ([#696](https://github.com/kubernetes-csi/external-snapshotter/pull/696), [@Madhu-1](https://github.com/Madhu-1))
- Cherry-pick ([#703](https://github.com/kubernetes-csi/external-snapshotter/pull/703), [@humblec](https://github.com/humblec)): Kube client dependencies are updated to v1.24.0 ([#712](https://github.com/kubernetes-csi/external-snapshotter/pull/712), [@humblec](https://github.com/humblec))

#### Snapshot Controller

- Remove unnecessary rbac read permissions of storage class resources from snapshot controller deployment. ([#645](https://github.com/kubernetes-csi/external-snapshotter/pull/645), [@iyashu](https://github.com/iyashu))
- Cherry-pick ([#703](https://github.com/kubernetes-csi/external-snapshotter/pull/703), [@humblec](https://github.com/humblec)): Kube client dependencies are updated to v1.24.0 ([#712](https://github.com/kubernetes-csi/external-snapshotter/pull/712), [@humblec](https://github.com/humblec))

#### Snapshot Validation Webhook

- Cherry-pick ([#703](https://github.com/kubernetes-csi/external-snapshotter/pull/703), [@humblec](https://github.com/humblec)): Kube client dependencies are updated to v1.24.0 ([#712](https://github.com/kubernetes-csi/external-snapshotter/pull/712), [@humblec](https://github.com/humblec))

## Dependencies

### Added
_Nothing has changed._

### Changed
- github.com/google/go-cmp: [v0.5.5 → v0.5.6](https://github.com/google/go-cmp/compare/v0.5.5...v0.5.6)
- github.com/nxadm/tail: [v1.4.4 → v1.4.8](https://github.com/nxadm/tail/compare/v1.4.4...v1.4.8)
- github.com/onsi/ginkgo: [v1.14.0 → v1.16.5](https://github.com/onsi/ginkgo/compare/v1.14.0...v1.16.5)
- github.com/onsi/gomega: [v1.10.1 → v1.17.0](https://github.com/onsi/gomega/compare/v1.10.1...v1.17.0)
- github.com/prometheus/client_golang: [v1.11.0 → v1.11.1](https://github.com/prometheus/client_golang/compare/v1.11.0...v1.11.1)
- sigs.k8s.io/yaml: v1.2.0 → v1.3.0

### Removed
_Nothing has changed._
