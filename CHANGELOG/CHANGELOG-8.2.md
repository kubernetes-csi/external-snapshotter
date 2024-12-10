# Release notes for v8.2.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v8.1.0

## Changes by Kind

### API Change

- Add a field called `volumegroupsnapshotcontent.status.volumeSnapshotHandlePairList` that allows the consumer to quickly map volume handles with snapshot handles. ([#1169](https://github.com/kubernetes-csi/external-snapshotter/pull/1169), [@leonardoce](https://github.com/leonardoce))
- The `volumegroupsnapshot.status.pvcVolumeSnapshotRefList` field has been removed. VolumeShapshots members of a dynamically provisioned VolumeGroupSnapshot will have their `persistentVolumeClaimName` set, allowing the consumer to map the PVC being snapshotted with the corresponding snapshot. ([#1200](https://github.com/kubernetes-csi/external-snapshotter/pull/1200), [@leonardoce](https://github.com/leonardoce))
- The `volumegroupsnapshotcontent.status.pvVolumeSnapshotContentList` field has been removed. The same information can be found in `volumegroupsnapshotcontent.status.volumeSnapshotHandlePairList` ([#1199](https://github.com/kubernetes-csi/external-snapshotter/pull/1199), [@leonardoce](https://github.com/leonardoce))
- `VolumeGroupSnapshotContent.status.creationTime` is now a metav1.Time instead of an unix epoch time ([#1235](https://github.com/kubernetes-csi/external-snapshotter/pull/1235), [@leonardoce](https://github.com/leonardoce))
- `VolumeGroupSnapshot`, `VolumeGroupSnapshotContent`, and `VolumeGroupSnapshotClass`
  are now available in `v1beta1` version. The support for the `v1alpha1` version have been removed. ([#1150](https://github.com/kubernetes-csi/external-snapshotter/pull/1150), [@leonardoce](https://github.com/leonardoce))
- The `api-approved.kubernetes.io` annotation on the VolumeGroupSnapshot, VolumeGroupSnapshotContent, and VolumeGroupSnapshotClass CRD now point to the PR introducing v1beta1 ([#1237](https://github.com/kubernetes-csi/external-snapshotter/pull/1237), [@leonardoce](https://github.com/leonardoce))

### Bug or Regression

- During VolumeGroupDeletion dont call the DeleteSnapshot RPC call when a snapshot belongs to a VolumeGroupSnapshot. ([#1231](https://github.com/kubernetes-csi/external-snapshotter/pull/1231), [@Madhu-1](https://github.com/Madhu-1))
- Fixed a race condition happening when deleting dynamically provisioned a VolumeGroupSnapshot whose DeletionPolicy is set to "Retain" ([#1216](https://github.com/kubernetes-csi/external-snapshotter/pull/1216), [@leonardoce](https://github.com/leonardoce))
- Fixes a create snapshot timeout issue. ([#261](https://github.com/kubernetes-csi/external-snapshotter/pull/261), [@xing-yang](https://github.com/xing-yang))
- Only dynamic provisioning for an independent snapshot needs a snapshot class. A member snapshot in a dynamically provisioned volume group snapshot does not need a snapshot class. ([#1204](https://github.com/kubernetes-csi/external-snapshotter/pull/1204), [@xing-yang](https://github.com/xing-yang))
- The  `groupsnapshot.storage.k8s.io/volumeGroupSnapshotName` label is not used anymore for dynamically provisioned volume group snapshot members. The members of a volume group snapshot can be identified as being owned by the VolumeGroupSnapshot. ([#1225](https://github.com/kubernetes-csi/external-snapshotter/pull/1225), [@leonardoce](https://github.com/leonardoce))
- The `groupsnapshot.storage.kubernetes.io/volumegroupsnapshot-bound-protection` finalizer is set also on dynamically provisioned VolumeGroupSnapshots whose `DeletionPolicy` is `Retain` ([#1224](https://github.com/kubernetes-csi/external-snapshotter/pull/1224), [@leonardoce](https://github.com/leonardoce))

### Other (Cleanup or Flake)

- Move the logic of creating individual VolumeSnapshot and VolumeSnapshotContent resources for dynamically created VolumeGroupSnapshot from csi-snapshotter sidecar to snapshot-controller. ([#1171](https://github.com/kubernetes-csi/external-snapshotter/pull/1171), [@leonardoce](https://github.com/leonardoce))
- The "spec.persistentVolumeClaimName" field is now set on VolumeSnapshots that are members of a dynamically provisioned VolumeGroupSnapshot. This is consistent with dynamically provisioned VolumeSnapshots. ([#1177](https://github.com/kubernetes-csi/external-snapshotter/pull/1177), [@leonardoce](https://github.com/leonardoce))
- The "spec.source.volumeHandle" field is now set on VolumeSnapshotContents that are members of a dynamically provisioned VolumeGroupSnapshot. This is consistent with dynamically provisioned VolumeSnapshotContents. ([#1198](https://github.com/kubernetes-csi/external-snapshotter/pull/1198), [@leonardoce](https://github.com/leonardoce))

### Uncategorized

- Add VolumeGroupSnapshotClass secrets for GetVolumeGroupSnapshot. ([#1173](https://github.com/kubernetes-csi/external-snapshotter/pull/1173), [@yati1998](https://github.com/yati1998))
- Fix unbounded volumesnapshots list call on Snapshot Controller startup ([#1238](https://github.com/kubernetes-csi/external-snapshotter/pull/1238), [@AndrewSirenko](https://github.com/AndrewSirenko))
- Remove the rule that ensures volumeGroupSnapshotRef is immutable. UID needs to be set by the snapshot controller after volumeGroupSnapshotRef is populated with name and namespace for pre-provisioned VolumeGroupSnapshotContent. ([#1184](https://github.com/kubernetes-csi/external-snapshotter/pull/1184), [@xing-yang](https://github.com/xing-yang))
- Store the volumegroupsnapshot handle as a annotation in the volumesnapshotcontent instead of labels as label values are restricted to 63 chars ([#1219](https://github.com/kubernetes-csi/external-snapshotter/pull/1219), [@Madhu-1](https://github.com/Madhu-1))
- The enable-volume-group-snapshots flag has been replaced by feature-gates flag.
  Enable feature gate to enable volumegroupsnapshot, i.e., --feature-gates=CSIVolumeGroupSnapshot=true. 
  By default the feature gate is disabled ([#1194](https://github.com/kubernetes-csi/external-snapshotter/pull/1194), [@yati1998](https://github.com/yati1998))
- The validation webhook was deprecated in v8.0.0 and it is now removed.
  The validation webhook would prevent creating multiple default volume snapshot classes and multiple default volume group snapshot classes for the same CSI driver. With the removal of the validation webhook, an error will still be raised when dynamically provisioning a VolumeSnapshot or VolumeGroupSnapshot when multiple default volume snapshot classes or multiple default volume group snapshot classes for the same CSI driver exist. ([#1186](https://github.com/kubernetes-csi/external-snapshotter/pull/1186), [@yati1998](https://github.com/yati1998))
- Update CSI spec to a commit after v1.10.0 where VolumeGroupSnapshot moved to GA ([#1174](https://github.com/kubernetes-csi/external-snapshotter/pull/1174), [@yati1998](https://github.com/yati1998))
- Use v1.11.0 version of CSI spec ([#1209](https://github.com/kubernetes-csi/external-snapshotter/pull/1209), [@yati1998](https://github.com/yati1998))

## Dependencies

### Added
- github.com/antlr4-go/antlr/v4: [v4.13.0](https://github.com/antlr4-go/antlr/v4/tree/v4.13.0)
- github.com/coreos/go-oidc: [v2.2.1+incompatible](https://github.com/coreos/go-oidc/tree/v2.2.1)
- github.com/coreos/go-semver: [v0.3.1](https://github.com/coreos/go-semver/tree/v0.3.1)
- github.com/coreos/go-systemd/v22: [v22.5.0](https://github.com/coreos/go-systemd/v22/tree/v22.5.0)
- github.com/dustin/go-humanize: [v1.0.1](https://github.com/dustin/go-humanize/tree/v1.0.1)
- github.com/golang-jwt/jwt/v4: [v4.5.0](https://github.com/golang-jwt/jwt/v4/tree/v4.5.0)
- github.com/google/cel-go: [v0.20.1](https://github.com/google/cel-go/tree/v0.20.1)
- github.com/grpc-ecosystem/go-grpc-middleware: [v1.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/v1.3.0)
- github.com/grpc-ecosystem/go-grpc-prometheus: [v1.2.0](https://github.com/grpc-ecosystem/go-grpc-prometheus/tree/v1.2.0)
- github.com/grpc-ecosystem/grpc-gateway: [v1.16.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.16.0)
- github.com/jonboulle/clockwork: [v0.2.2](https://github.com/jonboulle/clockwork/tree/v0.2.2)
- github.com/planetscale/vtprotobuf: [0393e58](https://github.com/planetscale/vtprotobuf/tree/0393e58)
- github.com/pquerna/cachecontrol: [v0.1.0](https://github.com/pquerna/cachecontrol/tree/v0.1.0)
- github.com/sirupsen/logrus: [v1.9.3](https://github.com/sirupsen/logrus/tree/v1.9.3)
- github.com/soheilhy/cmux: [v0.1.5](https://github.com/soheilhy/cmux/tree/v0.1.5)
- github.com/stoewer/go-strcase: [v1.2.0](https://github.com/stoewer/go-strcase/tree/v1.2.0)
- github.com/tmc/grpc-websocket-proxy: [673ab2c](https://github.com/tmc/grpc-websocket-proxy/tree/673ab2c)
- github.com/xiang90/probing: [43a291a](https://github.com/xiang90/probing/tree/43a291a)
- go.etcd.io/bbolt: v1.3.9
- go.etcd.io/etcd/api/v3: v3.5.14
- go.etcd.io/etcd/client/pkg/v3: v3.5.14
- go.etcd.io/etcd/client/v2: v2.305.13
- go.etcd.io/etcd/client/v3: v3.5.14
- go.etcd.io/etcd/pkg/v3: v3.5.13
- go.etcd.io/etcd/raft/v3: v3.5.13
- go.etcd.io/etcd/server/v3: v3.5.13
- golang.org/x/exp: f3d0a9c
- google.golang.org/genproto: b8732ec
- gopkg.in/natefinch/lumberjack.v2: v2.2.1
- gopkg.in/square/go-jose.v2: v2.6.0
- k8s.io/apiserver: v0.31.3
- k8s.io/kms: v0.31.3
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.30.3

### Changed
- cel.dev/expr: v0.15.0 → v0.16.1
- cloud.google.com/go/compute/metadata: v0.3.0 → v0.5.0
- github.com/NYTimes/gziphandler: [56545f4 → v1.1.1](https://github.com/NYTimes/gziphandler/compare/56545f4...v1.1.1)
- github.com/cncf/xds/go: [555b57e → b4127c9](https://github.com/cncf/xds/go/compare/555b57e...b4127c9)
- github.com/container-storage-interface/spec: [v1.9.0 → v1.11.0](https://github.com/container-storage-interface/spec/compare/v1.9.0...v1.11.0)
- github.com/envoyproxy/go-control-plane: [v0.12.0 → v0.13.0](https://github.com/envoyproxy/go-control-plane/compare/v0.12.0...v0.13.0)
- github.com/envoyproxy/protoc-gen-validate: [v1.0.4 → v1.1.0](https://github.com/envoyproxy/protoc-gen-validate/compare/v1.0.4...v1.1.0)
- github.com/golang/glog: [v1.2.1 → v1.2.2](https://github.com/golang/glog/compare/v1.2.1...v1.2.2)
- github.com/google/gnostic-models: [v0.6.8 → v0.6.9](https://github.com/google/gnostic-models/compare/v0.6.8...v0.6.9)
- github.com/klauspost/compress: [v1.17.9 → v1.17.11](https://github.com/klauspost/compress/compare/v1.17.9...v1.17.11)
- github.com/kubernetes-csi/csi-test/v5: [v5.2.0 → v5.3.1](https://github.com/kubernetes-csi/csi-test/v5/compare/v5.2.0...v5.3.1)
- github.com/prometheus/client_golang: [v1.20.2 → v1.20.5](https://github.com/prometheus/client_golang/compare/v1.20.2...v1.20.5)
- github.com/prometheus/common: [v0.55.0 → v0.60.1](https://github.com/prometheus/common/compare/v0.55.0...v0.60.1)
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.54.0 → v0.57.0
- go.opentelemetry.io/otel/metric: v1.29.0 → v1.32.0
- go.opentelemetry.io/otel/trace: v1.29.0 → v1.32.0
- go.opentelemetry.io/otel: v1.29.0 → v1.32.0
- golang.org/x/crypto: v0.26.0 → v0.29.0
- golang.org/x/net: v0.28.0 → v0.31.0
- golang.org/x/oauth2: v0.22.0 → v0.24.0
- golang.org/x/sync: v0.8.0 → v0.9.0
- golang.org/x/sys: v0.24.0 → v0.27.0
- golang.org/x/term: v0.23.0 → v0.26.0
- golang.org/x/text: v0.17.0 → v0.20.0
- golang.org/x/time: v0.6.0 → v0.8.0
- google.golang.org/genproto/googleapis/api: 5315273 → 8af14fe
- google.golang.org/genproto/googleapis/rpc: fc7c04a → dd2ea8e
- google.golang.org/grpc: v1.65.0 → v1.68.0
- google.golang.org/protobuf: v1.34.2 → v1.35.2
- sigs.k8s.io/structured-merge-diff/v4: v4.4.1 → v4.4.3

### Removed
_Nothing has changed._
