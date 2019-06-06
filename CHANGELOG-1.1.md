# Changelog since v1.0.1

## Deprecations
* Command line flag `--connection-timeout` is deprecated and has no effect.
* Command line flag `--snapshotter` is deprecated and has no effect ([#103](https://github.com/kubernetes-csi/external-snapshotter/pull/103))

## Notable Features
* Add Lease based Leader Election Support ([#107](https://github.com/kubernetes-csi/external-snapshotter/pull/107))
* The external snapshotter now tries to connect to the CSI driver indefinitely ([#92](https://github.com/kubernetes-csi/external-snapshotter/pull/92))
* A new --timeout parameter has been added for CSI operations ([#93](https://github.com/kubernetes-csi/external-snapshotter/pull/93))
* Prow testing ([#111](https://github.com/kubernetes-csi/external-snapshotter/pull/111))

## Other Notable Changes
* Add PR template ([#113](https://github.com/kubernetes-csi/external-snapshotter/pull/113))
* Un-prune code-generator scripts ([#110](https://github.com/kubernetes-csi/external-snapshotter/pull/110))
* Refactor external snapshotter to use csi-lib-utils/rpc ([#97](https://github.com/kubernetes-csi/external-snapshotter/pull/97))
* Fix for pre-bound snapshot empty source error ([#98](https://github.com/kubernetes-csi/external-snapshotter/pull/98))
* Update vendor to k8s 1.14.0 ([#105](https://github.com/kubernetes-csi/external-snapshotter/pull/105))
* Migrate to k8s.io/klog from glog. ([#88](https://github.com/kubernetes-csi/external-snapshotter/pull/88))
* Use distroless as base image ([#101](https://github.com/kubernetes-csi/external-snapshotter/pull/101))
* Remove constraints and update all vendor pkgs ([#100](https://github.com/kubernetes-csi/external-snapshotter/pull/100))
* Add dep prune options and remove unused packages ([#99](https://github.com/kubernetes-csi/external-snapshotter/pull/99))
* Release tools ([#86](https://github.com/kubernetes-csi/external-snapshotter/pull/86))
