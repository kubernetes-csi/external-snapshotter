# Release notes for v7.0.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v6.3.0

## Urgent Upgrade Notes 

### (No, really, you MUST read this before you upgrade)

- Enable prevent-volume-mode-conversion feature flag by default.
  
  Volume mode change will be rejected when creating a PVC from a VolumeSnapshot unless the AllowVolumeModeChange annotation has been set to true. Applications relying on volume mode change when creating a PVC from VolumeSnapshot need to be updated accordingly. ([#916](https://github.com/kubernetes-csi/external-snapshotter/pull/916), [@akalenyu](https://github.com/akalenyu))
 
## Changes by Kind

### API Change

- Add VolumeGroupSnapshot API definitions. ([#814](https://github.com/kubernetes-csi/external-snapshotter/pull/814), [@RaunakShah](https://github.com/RaunakShah))
- The VolumeGroupSnapshotSource.Selector is now an optional attribute, so that a pre-provisioned VolumeGroupSnapshotContent can be specified which does not require a matching label-selector. ([#995](https://github.com/kubernetes-csi/external-snapshotter/pull/995), [@nixpanic](https://github.com/nixpanic))
- Update API for pre provisioned group snapshots ([#971](https://github.com/kubernetes-csi/external-snapshotter/pull/971), [@RaunakShah](https://github.com/RaunakShah))

### Feature

- Create Volume functionality for volume group snapshots (Note: this feature is partially implemented and therefore it is not ready for use) ([#826](https://github.com/kubernetes-csi/external-snapshotter/pull/826), [@RaunakShah](https://github.com/RaunakShah))
- More detail printed columns output when get vgs/vgsc/vgsclass with kubectl ([#865](https://github.com/kubernetes-csi/external-snapshotter/pull/865), [@winrouter](https://github.com/winrouter))
- Webhooks for VolumeGroupSnapshot, VolumeGroupSnapshotContent and VolumeGroupSnapshotClass. ([#825](https://github.com/kubernetes-csi/external-snapshotter/pull/825), [@Rakshith-R](https://github.com/Rakshith-R))
- Add finalizer to prevent deletion of individual volume snapshots that are part of a group ([#972](https://github.com/kubernetes-csi/external-snapshotter/pull/972), [@RaunakShah](https://github.com/RaunakShah))
- Delete individual snapshots as part of volume group snapshots delete API ([#952](https://github.com/kubernetes-csi/external-snapshotter/pull/952), [@RaunakShah](https://github.com/RaunakShah))
- Implement GetGroupSnapshotStatus so that pre-provisioned VolumeGroupSnapshots can be imported. ([#837](https://github.com/kubernetes-csi/external-snapshotter/pull/837), [@nixpanic](https://github.com/nixpanic))
- Introduce logic to delete volume group snapshots ([#882](https://github.com/kubernetes-csi/external-snapshotter/pull/882), [@RaunakShah](https://github.com/RaunakShah))

### Bug or Regression

- Fixed the max duration to wait for CRDs to appear especially in case of the apiserver being unreachable ([#987](https://github.com/kubernetes-csi/external-snapshotter/pull/987), [@Fricounet](https://github.com/Fricounet))
- Fixed waiting for a snapshot to become ready with exponential backoff in CSI Snapshotter sidecar. ([#958](https://github.com/kubernetes-csi/external-snapshotter/pull/958), [@jsafrane](https://github.com/jsafrane))
- Webhooks for group snapshot CRs will be disabled by default. Command line argument `enable-volume-group-snapshot-webhook` needs to be added to enable these webhooks. ([#922](https://github.com/kubernetes-csi/external-snapshotter/pull/922), [@Rakshith-R](https://github.com/Rakshith-R))

### Other (Cleanup or Flake)

- Adopt Kubernetes recommended label "app.kubernetes.io/name" when deploying csi-snapshotter, snapshot-controller, and snapshot-validation-webhook. ([#844](https://github.com/kubernetes-csi/external-snapshotter/pull/844), [@mowangdk](https://github.com/mowangdk))
- Store VolumeGroupSnapshotHandle in SnapshotContent.Status instead of VolumeGroupSnapshotContentName ([#955](https://github.com/kubernetes-csi/external-snapshotter/pull/955), [@RaunakShah](https://github.com/RaunakShah))

### Uncategorized

- Update VolumeSnapshot and VolumeSnapshotContent using JSON patch ([#876](https://github.com/kubernetes-csi/external-snapshotter/pull/876), [@shubham-pampattiwar](https://github.com/shubham-pampattiwar))
- Update kubernetes dependencies to v1.29.0 ([#978](https://github.com/kubernetes-csi/external-snapshotter/pull/978), [@RaunakShah](https://github.com/RaunakShah))
- Update to use volume snapshot client v7. ([#998](https://github.com/kubernetes-csi/external-snapshotter/pull/998), [@xing-yang](https://github.com/xing-yang))

## Dependencies

### Added
- github.com/gorilla/websocket: [v1.5.0](https://github.com/gorilla/websocket/tree/v1.5.0)
- github.com/kubernetes-csi/csi-test/v5: [v5.2.0](https://github.com/kubernetes-csi/csi-test/v5/tree/v5.2.0)
- github.com/matttproud/golang_protobuf_extensions/v2: [v2.0.0](https://github.com/matttproud/golang_protobuf_extensions/v2/tree/v2.0.0)
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.46.0

### Changed
- cloud.google.com/go/compute: v1.21.0 → v1.23.3
- github.com/alecthomas/kingpin/v2: [v2.3.2 → v2.4.0](https://github.com/alecthomas/kingpin/v2/compare/v2.3.2...v2.4.0)
- github.com/cncf/xds/go: [e9ce688 → 523115e](https://github.com/cncf/xds/go/compare/e9ce688...523115e)
- github.com/container-storage-interface/spec: [v1.8.0 → v1.9.0](https://github.com/container-storage-interface/spec/compare/v1.8.0...v1.9.0)
- github.com/cpuguy83/go-md2man/v2: [v2.0.2 → v2.0.3](https://github.com/cpuguy83/go-md2man/v2/compare/v2.0.2...v2.0.3)
- github.com/emicklei/go-restful/v3: [v3.10.1 → v3.11.0](https://github.com/emicklei/go-restful/v3/compare/v3.10.1...v3.11.0)
- github.com/evanphx/json-patch: [v5.7.0+incompatible → v5.9.0+incompatible](https://github.com/evanphx/json-patch/compare/v5.7.0...v5.9.0)
- github.com/fsnotify/fsnotify: [v1.6.0 → v1.7.0](https://github.com/fsnotify/fsnotify/compare/v1.6.0...v1.7.0)
- github.com/go-logr/logr: [v1.2.4 → v1.4.1](https://github.com/go-logr/logr/compare/v1.2.4...v1.4.1)
- github.com/golang/glog: [v1.1.0 → v1.1.2](https://github.com/golang/glog/compare/v1.1.0...v1.1.2)
- github.com/google/go-cmp: [v0.5.9 → v0.6.0](https://github.com/google/go-cmp/compare/v0.5.9...v0.6.0)
- github.com/google/uuid: [v1.3.0 → v1.4.0](https://github.com/google/uuid/compare/v1.3.0...v1.4.0)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.7.0 → v2.16.0](https://github.com/grpc-ecosystem/grpc-gateway/v2/compare/v2.7.0...v2.16.0)
- github.com/kubernetes-csi/csi-lib-utils: [v0.14.0 → v0.17.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.14.0...v0.17.0)
- github.com/onsi/ginkgo/v2: [v2.9.4 → v2.13.1](https://github.com/onsi/ginkgo/v2/compare/v2.9.4...v2.13.1)
- github.com/onsi/gomega: [v1.27.6 → v1.30.0](https://github.com/onsi/gomega/compare/v1.27.6...v1.30.0)
- github.com/prometheus/client_golang: [v1.16.0 → v1.18.0](https://github.com/prometheus/client_golang/compare/v1.16.0...v1.18.0)
- github.com/prometheus/client_model: [v0.4.0 → v0.5.0](https://github.com/prometheus/client_model/compare/v0.4.0...v0.5.0)
- github.com/prometheus/common: [v0.44.0 → v0.46.0](https://github.com/prometheus/common/compare/v0.44.0...v0.46.0)
- github.com/prometheus/procfs: [v0.10.1 → v0.12.0](https://github.com/prometheus/procfs/compare/v0.10.1...v0.12.0)
- github.com/spf13/cobra: [v1.7.0 → v1.8.0](https://github.com/spf13/cobra/compare/v1.7.0...v1.8.0)
- github.com/stretchr/testify: [v1.8.2 → v1.8.4](https://github.com/stretchr/testify/compare/v1.8.2...v1.8.4)
- github.com/yuin/goldmark: [v1.3.5 → v1.4.13](https://github.com/yuin/goldmark/compare/v1.3.5...v1.4.13)
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.35.1 → v0.44.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.10.0 → v1.19.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.10.0 → v1.19.0
- go.opentelemetry.io/otel/metric: v0.31.0 → v1.20.0
- go.opentelemetry.io/otel/sdk: v1.10.0 → v1.19.0
- go.opentelemetry.io/otel/trace: v1.10.0 → v1.20.0
- go.opentelemetry.io/otel: v1.10.0 → v1.20.0
- go.opentelemetry.io/proto/otlp: v0.19.0 → v1.0.0
- golang.org/x/crypto: v0.11.0 → v0.18.0
- golang.org/x/mod: v0.8.0 → v0.12.0
- golang.org/x/net: v0.13.0 → v0.20.0
- golang.org/x/oauth2: v0.10.0 → v0.16.0
- golang.org/x/sync: v0.3.0 → v0.5.0
- golang.org/x/sys: v0.10.0 → v0.16.0
- golang.org/x/term: v0.10.0 → v0.16.0
- golang.org/x/text: v0.11.0 → v0.14.0
- golang.org/x/tools: v0.8.0 → v0.14.0
- google.golang.org/appengine: v1.6.7 → v1.6.8
- google.golang.org/genproto/googleapis/api: 782d3b1 → bbf56f3
- google.golang.org/genproto/googleapis/rpc: 782d3b1 → bbf56f3
- google.golang.org/genproto: 782d3b1 → bbf56f3
- google.golang.org/grpc: v1.58.0 → v1.61.0
- google.golang.org/protobuf: v1.31.0 → v1.32.0
- k8s.io/api: v0.28.0 → v0.29.0
- k8s.io/apimachinery: v0.28.0 → v0.29.0
- k8s.io/client-go: v0.28.0 → v0.29.0
- k8s.io/code-generator: v0.28.0 → v0.29.0
- k8s.io/component-base: v0.28.0 → v0.29.0
- k8s.io/component-helpers: v0.28.0 → v0.29.0
- k8s.io/gengo: fad74ee → 9cce18d
- k8s.io/klog/v2: v2.100.1 → v2.120.1
- k8s.io/kube-openapi: 2695361 → 2dd684a
- k8s.io/utils: d93618c → 3b25d92
- sigs.k8s.io/structured-merge-diff/v4: v4.2.3 → v4.4.1

### Removed
- cloud.google.com/go: v0.34.0
- github.com/BurntSushi/toml: [v0.3.1](https://github.com/BurntSushi/toml/tree/v0.3.1)
- github.com/antihax/optional: [v1.0.0](https://github.com/antihax/optional/tree/v1.0.0)
- github.com/chzyer/logex: [v1.1.10](https://github.com/chzyer/logex/tree/v1.1.10)
- github.com/chzyer/readline: [2972be2](https://github.com/chzyer/readline/tree/2972be2)
- github.com/chzyer/test: [a1ea475](https://github.com/chzyer/test/tree/a1ea475)
- github.com/client9/misspell: [v0.3.4](https://github.com/client9/misspell/tree/v0.3.4)
- github.com/ghodss/yaml: [v1.0.0](https://github.com/ghodss/yaml/tree/v1.0.0)
- github.com/google/gnostic: [v0.6.9](https://github.com/google/gnostic/tree/v0.6.9)
- github.com/grpc-ecosystem/grpc-gateway: [v1.16.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.16.0)
- github.com/hpcloud/tail: [v1.0.0](https://github.com/hpcloud/tail/tree/v1.0.0)
- github.com/ianlancetaylor/demangle: [28f6c0f](https://github.com/ianlancetaylor/demangle/tree/28f6c0f)
- github.com/kubernetes-csi/csi-test/v4: [v4.4.0](https://github.com/kubernetes-csi/csi-test/v4/tree/v4.4.0)
- github.com/nxadm/tail: [v1.4.8](https://github.com/nxadm/tail/tree/v1.4.8)
- github.com/onsi/ginkgo: [v1.16.5](https://github.com/onsi/ginkgo/tree/v1.16.5)
- github.com/rogpeppe/fastuuid: [v1.2.0](https://github.com/rogpeppe/fastuuid/tree/v1.2.0)
- go.opentelemetry.io/otel/exporters/otlp/internal/retry: v1.10.0
- go.uber.org/goleak: v1.2.1
- golang.org/x/exp: 509febe
- golang.org/x/lint: d0100b6
- gopkg.in/fsnotify.v1: v1.4.7
- gopkg.in/tomb.v1: dd63297
- honnef.co/go/tools: ea95bdf
