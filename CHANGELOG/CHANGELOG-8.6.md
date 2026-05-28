# Release notes for v8.6.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v8.5.0

## Changes by Kind

### Feature

- Promoted VolumeGroupSnapshot API to GA (v1). Updated controllers to use v1 VolumeGroupSnapshot APIs and set v1beta2 as the stored version during migration. ([#1368](https://github.com/kubernetes-csi/external-snapshotter/pull/1368), [@xing-yang](https://github.com/xing-yang))

### Bug or Regression

- Fixed VolumeSnapshot deletion not being retried when the snapshot is used as a data source for a PVC restore in progress. The controller now returns an error to ensure the workqueue requeues the snapshot until the PVC is no longer pending. The same fix is applied to VolumeGroupSnapshot deletion. ([#1394](https://github.com/kubernetes-csi/external-snapshotter/pull/1394), [@Paramesh324](https://github.com/Paramesh324))
- Fixed deletion of snapshots that were marked for deletion while the CSI driver was still taking the snapshot. ([#1392](https://github.com/kubernetes-csi/external-snapshotter/pull/1392), [@jsafrane](https://github.com/jsafrane))
- Fix PVC finalizer update retrying on conflict ([#1339](https://github.com/kubernetes-csi/external-snapshotter/pull/1339), [@akalenyu](https://github.com/akalenyu))
- Add HTTP server timeouts to the webhook ([#1423](https://github.com/kubernetes-csi/external-snapshotter/pull/1423), [@mattcary](https://github.com/mattcary))

### Other (Cleanup or Flake)

- Bump k8s dependencies to v1.36.1 and Go to 1.26.0. ([#1422](https://github.com/kubernetes-csi/external-snapshotter/pull/1422), [@dfajmon](https://github.com/dfajmon))

## Dependencies

### Added
- github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus: [v1.1.0](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/providers/prometheus/v1.1.0)
- github.com/grpc-ecosystem/go-grpc-middleware/v2: [v2.3.3](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/v2.3.3)
- k8s.io/streaming: [v0.36.1](https://github.com/kubernetes/streaming/tree/v0.36.1)

### Changed
- cel.dev/expr: v0.25.1 → v0.25.2
- github.com/fsnotify/fsnotify: [v1.9.0 → v1.10.1](https://github.com/fsnotify/fsnotify/compare/v1.9.0...v1.10.1)
- github.com/fxamacker/cbor/v2: [v2.9.0 → v2.9.2](https://github.com/fxamacker/cbor/compare/v2.9.0...v2.9.2)
- github.com/go-openapi/jsonpointer: [v0.22.4 → v0.23.1](https://github.com/go-openapi/jsonpointer/compare/v0.22.4...v0.23.1)
- github.com/go-openapi/jsonreference: [v0.21.4 → v0.21.5](https://github.com/go-openapi/jsonreference/compare/v0.21.4...v0.21.5)
- github.com/go-openapi/swag: [v0.25.4 → v0.26.0](https://github.com/go-openapi/swag/compare/v0.25.4...v0.26.0)
- github.com/go-openapi/swag/cmdutils: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/conv: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/fileutils: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/jsonname: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/jsonutils: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/loading: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/mangling: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/netutils: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/stringutils: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/typeutils: v0.25.4 → v0.26.0
- github.com/go-openapi/swag/yamlutils: v0.25.4 → v0.26.0
- github.com/google/cel-go: [v0.27.0 → v0.28.1](https://github.com/google/cel-go/compare/v0.27.0...v0.28.1)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.27.7 → v2.29.0](https://github.com/grpc-ecosystem/grpc-gateway/compare/v2.27.7...v2.29.0)
- github.com/kubernetes-csi/csi-lib-utils: [v0.23.2 → v0.24.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.23.2...v0.24.0)
- github.com/prometheus/procfs: [v0.19.2 → v0.20.1](https://github.com/prometheus/procfs/compare/v0.19.2...v0.20.1)
- go.etcd.io/etcd/api/v3: v3.6.7 → v3.6.11
- go.etcd.io/etcd/client/pkg/v3: v3.6.7 → v3.6.11
- go.etcd.io/etcd/client/v3: v3.6.7 → v3.6.11
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.65.0 → v0.68.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.65.0 → v0.68.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.40.0 → v1.43.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.40.0 → v1.43.0
- go.opentelemetry.io/otel/metric: v1.40.0 → v1.43.0
- go.opentelemetry.io/otel/sdk: v1.40.0 → v1.43.0
- go.opentelemetry.io/otel/trace: v1.40.0 → v1.43.0
- go.opentelemetry.io/otel: v1.40.0 → v1.43.0
- go.opentelemetry.io/proto/otlp: v1.9.0 → v1.10.0
- go.uber.org/zap: [v1.27.1 → v1.28.0](https://github.com/uber-go/zap/compare/v1.27.1...v1.28.0)
- go.yaml.in/yaml/v2: v2.4.3 → v2.4.4
- golang.org/x/crypto: v0.47.0 → v0.52.0
- golang.org/x/net: v0.49.0 → v0.54.0
- golang.org/x/oauth2: v0.35.0 → v0.36.0
- golang.org/x/sync: v0.19.0 → v0.20.0
- golang.org/x/sys: v0.41.0 → v0.45.0
- golang.org/x/term: v0.39.0 → v0.43.0
- golang.org/x/text: v0.33.0 → v0.37.0
- golang.org/x/time: v0.14.0 → v0.15.0
- google.golang.org/genproto/googleapis/api: v0.0.0-20260128011058-8636f8732409 → v0.0.0-20260414002931-afd174a4e478
- google.golang.org/genproto/googleapis/rpc: v0.0.0-20260128011058-8636f8732409 → v0.0.0-20260414002931-afd174a4e478
- google.golang.org/grpc: [v1.78.0 → v1.81.1](https://github.com/grpc/grpc-go/compare/v1.78.0...v1.81.1)
- google.golang.org/protobuf: v1.36.11 → v1.36.12-0.20260120151049-f2248ac996af
- k8s.io/api: v0.35.0 → v0.36.1
- k8s.io/apiextensions-apiserver: v0.35.0 → v0.36.1
- k8s.io/apimachinery: v0.35.0 → v0.36.1
- k8s.io/apiserver: v0.35.0 → v0.36.1
- k8s.io/client-go: v0.35.0 → v0.36.1
- k8s.io/component-base: v0.35.0 → v0.36.1
- k8s.io/component-helpers: v0.35.0 → v0.36.1
- k8s.io/klog/v2: [v2.130.1 → v2.140.0](https://github.com/kubernetes/klog/compare/v2.130.1...v2.140.0)
- k8s.io/kube-openapi: v0.0.0-20251125145642-4e65d59e963e → v0.0.0-20260317180543-43fb72c5454a
- k8s.io/utils: v0.0.0-20260108192941-914a6e750570 → v0.0.0-20260210185600-b8788abfbbc2
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: [v0.34.0 → v0.35.0](https://github.com/kubernetes-sigs/apiserver-network-proxy/compare/konnectivity-client/v0.34.0...konnectivity-client/v0.35.0)
- sigs.k8s.io/structured-merge-diff/v6: [v6.3.2 → v6.4.0](https://github.com/kubernetes-sigs/structured-merge-diff/compare/v6.3.2...v6.4.0)

### Removed
- github.com/grpc-ecosystem/go-grpc-prometheus: [v1.2.0](https://github.com/grpc-ecosystem/go-grpc-prometheus/tree/v1.2.0)
