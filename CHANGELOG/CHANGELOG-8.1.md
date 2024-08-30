# Release notes for v8.1.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v8.0.0

## Changes by Kind

### Documentation

- Document Volume Group Snapshot feature in the README file ([#1120](https://github.com/kubernetes-csi/external-snapshotter/pull/1120), [@leonardoce](https://github.com/leonardoce))

### Bug or Regression

- Look for a VolumeGroupSnapshotClass instead of a VolumeSnapshotClass when getting metrics data for VolumeGroupSnapshot metrics. ([#1115](https://github.com/kubernetes-csi/external-snapshotter/pull/1115), [@leonardoce](https://github.com/leonardoce))
- Prevents a VolumeGroupSnapshot to be provisioned when the volumes' CSI driver is different from the one referenced in the VolumeGroupSnapshotClass resource ([#1098](https://github.com/kubernetes-csi/external-snapshotter/pull/1098), [@leonardoce](https://github.com/leonardoce))
- The controller will now detect new default VolumeSnapshotClasses and VolumeGroupSnapshotClasses.
  If multiple classes exist for the same CSI driver, VolumeSnapshot and VolumeGroupSnapshots
  will be marked as failed when provisioned dynamically. ([#1100](https://github.com/kubernetes-csi/external-snapshotter/pull/1100), [@leonardoce](https://github.com/leonardoce))

### Other (Cleanup or Flake)

- Updates Kubernetes dependencies to v1.31.0 ([#1139](https://github.com/kubernetes-csi/external-snapshotter/pull/1139), [@dfajmon](https://github.com/dfajmon))

### Uncategorized

- Cleanup: Update csi release tools ([#1137](https://github.com/kubernetes-csi/external-snapshotter/pull/1137), [@andyzhangx](https://github.com/andyzhangx))
- Fixes a race condition where the PVC finalizer could end up not being removed by the snapshot-controller if the update had a conflict. ([#1133](https://github.com/kubernetes-csi/external-snapshotter/pull/1133), [@Fricounet](https://github.com/Fricounet))
- It exposes the metrics for volumegroupsnapshot operations on passing flags like --http-endpoint and --metrics-path to the snapshot controller. These are the command line arguments for the users to enable metrics and start the sever.
  The metric `snapshot_controller_operation_total_seconds` has been added for the volumegroupsnapshot operations:
  1. CreateGroupSnapshot
  2. DeleteGroupSnapshot
  3. CreateSnapshotAndReady ([#1107](https://github.com/kubernetes-csi/external-snapshotter/pull/1107), [@yati1998](https://github.com/yati1998))

## Dependencies

### Added
- cel.dev/expr: v0.15.0
- github.com/go-task/slim-sprig/v3: [v3.0.0](https://github.com/go-task/slim-sprig/tree/v3.0.0)
- github.com/klauspost/compress: [v1.17.9](https://github.com/klauspost/compress/tree/v1.17.9)
- github.com/kylelemons/godebug: [v1.1.0](https://github.com/kylelemons/godebug/tree/v1.1.0)
- gopkg.in/evanphx/json-patch.v4: v4.12.0

### Changed
- github.com/cenkalti/backoff/v4: [v4.2.1 → v4.3.0](https://github.com/cenkalti/backoff/compare/v4.2.1...v4.3.0)
- github.com/cncf/xds/go: [8a4994d → 555b57e](https://github.com/cncf/xds/compare/8a4994d...555b57e)
- github.com/cpuguy83/go-md2man/v2: [v2.0.3 → v2.0.4](https://github.com/cpuguy83/go-md2man/compare/v2.0.3...v2.0.4)
- github.com/davecgh/go-spew: [v1.1.1 → d8f796a](https://github.com/davecgh/go-spew/compare/v1.1.1...d8f796a)
- github.com/emicklei/go-restful/v3: [v3.12.0 → v3.12.1](https://github.com/emicklei/go-restful/compare/v3.12.0...v3.12.1)
- github.com/felixge/httpsnoop: [v1.0.3 → v1.0.4](https://github.com/felixge/httpsnoop/compare/v1.0.3...v1.0.4)
- github.com/fxamacker/cbor/v2: [v2.6.0 → v2.7.0](https://github.com/fxamacker/cbor/compare/v2.6.0...v2.7.0)
- github.com/go-logr/logr: [v1.4.1 → v1.4.2](https://github.com/go-logr/logr/compare/v1.4.1...v1.4.2)
- github.com/golang/glog: [v1.2.0 → v1.2.1](https://github.com/golang/glog/compare/v1.2.0...v1.2.1)
- github.com/google/pprof: [4bb14d4 → 4bfdf5a](https://github.com/google/pprof/compare/4bb14d4...4bfdf5a)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.16.0 → v2.20.0](https://github.com/grpc-ecosystem/grpc-gateway/compare/v2.16.0...v2.20.0)
- github.com/kubernetes-csi/csi-lib-utils: [v0.18.0 → v0.19.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.18.0...v0.19.0)
- github.com/moby/spdystream: [v0.2.0 → v0.4.0](https://github.com/moby/spdystream/compare/v0.2.0...v0.4.0)
- github.com/moby/term: [1aeaba8 → v0.5.0](https://github.com/moby/term/compare/1aeaba8...v0.5.0)
- github.com/onsi/ginkgo/v2: [v2.15.0 → v2.19.0](https://github.com/onsi/ginkgo/compare/v2.15.0...v2.19.0)
- github.com/onsi/gomega: [v1.31.0 → v1.33.1](https://github.com/onsi/gomega/compare/v1.31.0...v1.33.1)
- github.com/pmezard/go-difflib: [v1.0.0 → 5d4384e](https://github.com/pmezard/go-difflib/compare/v1.0.0...5d4384e)
- github.com/prometheus/client_golang: [v1.19.1 → v1.20.2](https://github.com/prometheus/client_golang/compare/v1.19.1...v1.20.2)
- github.com/prometheus/common: [v0.53.0 → v0.55.0](https://github.com/prometheus/common/compare/v0.53.0...v0.55.0)
- github.com/prometheus/procfs: [v0.15.0 → v0.15.1](https://github.com/prometheus/procfs/compare/v0.15.0...v0.15.1)
- github.com/rogpeppe/go-internal: [v1.11.0 → v1.12.0](https://github.com/rogpeppe/go-internal/compare/v1.11.0...v1.12.0)
- github.com/spf13/cobra: [v1.8.0 → v1.8.1](https://github.com/spf13/cobra/compare/v1.8.0...v1.8.1)
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.51.0 → v0.54.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.44.0 → v0.53.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.19.0 → v1.27.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.19.0 → v1.28.0
- go.opentelemetry.io/otel/metric: v1.26.0 → v1.29.0
- go.opentelemetry.io/otel/sdk: v1.19.0 → v1.28.0
- go.opentelemetry.io/otel/trace: v1.26.0 → v1.29.0
- go.opentelemetry.io/otel: v1.26.0 → v1.29.0
- go.opentelemetry.io/proto/otlp: v1.0.0 → v1.3.1
- golang.org/x/crypto: v0.23.0 → v0.26.0
- golang.org/x/mod: v0.15.0 → v0.17.0
- golang.org/x/net: v0.25.0 → v0.28.0
- golang.org/x/oauth2: v0.20.0 → v0.22.0
- golang.org/x/sync: v0.7.0 → v0.8.0
- golang.org/x/sys: v0.20.0 → v0.24.0
- golang.org/x/term: v0.20.0 → v0.23.0
- golang.org/x/text: v0.15.0 → v0.17.0
- golang.org/x/time: v0.5.0 → v0.6.0
- golang.org/x/tools: v0.18.0 → e35e4cc
- google.golang.org/appengine: v1.6.8 → v1.6.7
- google.golang.org/genproto/googleapis/api: 94a12d6 → 5315273
- google.golang.org/genproto/googleapis/rpc: 94a12d6 → fc7c04a
- google.golang.org/grpc: v1.64.0 → v1.65.0
- google.golang.org/protobuf: v1.34.1 → v1.34.2
- k8s.io/api: v0.30.0 → v0.31.0
- k8s.io/apimachinery: v0.30.0 → v0.31.0
- k8s.io/client-go: v0.30.0 → v0.31.0
- k8s.io/code-generator: v0.30.0 → v0.31.0
- k8s.io/component-base: v0.30.0 → v0.31.0
- k8s.io/component-helpers: v0.30.0 → v0.31.0
- k8s.io/klog/v2: v2.120.1 → v2.130.1
- k8s.io/utils: 3b25d92 → 18e509b

### Removed
- cloud.google.com/go/compute: v1.25.1
- github.com/matttproud/golang_protobuf_extensions: [v1.0.4](https://github.com/matttproud/golang_protobuf_extensions/tree/v1.0.4)
