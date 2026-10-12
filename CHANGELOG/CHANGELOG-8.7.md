# Release notes for v8.7.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v8.6.0

## Changes by Kind

### Bug or Regression

- Skip NotFound errors when deleting VolumeSnapshotContent objects. ([#1460](https://github.com/kubernetes-csi/external-snapshotter/pull/1460), [@rvagner78](https://github.com/rvagner78))
- Requeue VolumeGroupSnapshotContent with backoff when it is not ready to use. ([#1461](https://github.com/kubernetes-csi/external-snapshotter/pull/1461), [@xing-yang](https://github.com/xing-yang))
- Fixed a data race in the in-flight operation metric. ([#1472](https://github.com/kubernetes-csi/external-snapshotter/pull/1472), [@andyzhangx](https://github.com/andyzhangx))
- Ignore sidecar-owned VolumeGroupSnapshotContent updates so that the requeue backoff is honored. ([#1489](https://github.com/kubernetes-csi/external-snapshotter/pull/1489), [@somaz94](https://github.com/somaz94))
- Fixed a leak by shutting down the event broadcaster when the snapshot controller's Run returns. ([#1499](https://github.com/kubernetes-csi/external-snapshotter/pull/1499), [@thebhdn](https://github.com/thebhdn))
- Fixed GHSA-hrxh-6v49-42gf and Go CVEs by building with Go 1.26.9. ([#1438](https://github.com/kubernetes-csi/external-snapshotter/pull/1438), [#1465](https://github.com/kubernetes-csi/external-snapshotter/pull/1465), [#1466](https://github.com/kubernetes-csi/external-snapshotter/pull/1466), [#1471](https://github.com/kubernetes-csi/external-snapshotter/pull/1471), [#1509](https://github.com/kubernetes-csi/external-snapshotter/pull/1509), [@andyzhangx](https://github.com/andyzhangx))

### Other (Cleanup or Flake)

- Enabled the VolumeGroupSnapshot feature gate. ([#1475](https://github.com/kubernetes-csi/external-snapshotter/pull/1475), [@xing-yang](https://github.com/xing-yang))
- Updated the sidecar image versions in the deploy manifests to csi-provisioner v6.3.0, csi-snapshotter v8.6.0 and snapshot-controller v8.6.0. ([#1439](https://github.com/kubernetes-csi/external-snapshotter/pull/1439), [@humblec](https://github.com/humblec))
- Updated release-tools. ([#1449](https://github.com/kubernetes-csi/external-snapshotter/pull/1449), [@humblec](https://github.com/humblec))
- Updated the client module dependencies. ([#1429](https://github.com/kubernetes-csi/external-snapshotter/pull/1429), [@xing-yang](https://github.com/xing-yang))
- Bumped Kubernetes dependencies to v0.37.0 and other Go and GitHub Actions dependencies. ([#1507](https://github.com/kubernetes-csi/external-snapshotter/pull/1507), [@xing-yang](https://github.com/xing-yang))

## Dependencies

### Added
- github.com/go-openapi/swag/pools: v0.29.2
- go.opentelemetry.io/otel/log: v1.47.0
- go.uber.org/mock: v0.6.0

### Changed
- cel.dev/expr: v0.25.2 → v0.25.3
- github.com/container-storage-interface/spec: [v1.12.0 → v1.13.0](https://github.com/container-storage-interface/spec/compare/v1.12.0...v1.13.0)
- github.com/felixge/httpsnoop: [v1.0.4 → v1.1.0](https://github.com/felixge/httpsnoop/compare/v1.0.4...v1.1.0)
- github.com/fxamacker/cbor/v2: v2.9.2 → v2.9.4
- github.com/go-logr/logr: [v1.4.3 → v1.4.4](https://github.com/go-logr/logr/compare/v1.4.3...v1.4.4)
- github.com/go-openapi/jsonpointer: [v0.23.1 → v1.0.2](https://github.com/go-openapi/jsonpointer/compare/v0.23.1...v1.0.2)
- github.com/go-openapi/jsonreference: [v0.21.5 → v1.0.3](https://github.com/go-openapi/jsonreference/compare/v0.21.5...v1.0.3)
- github.com/go-openapi/swag: [v0.26.0 → v0.29.2](https://github.com/go-openapi/swag/compare/v0.26.0...v0.29.2)
- github.com/go-openapi/swag/cmdutils: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/conv: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/fileutils: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/jsonutils: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/loading: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/mangling: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/netutils: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/stringutils: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/typeutils: v0.26.0 → v0.29.2
- github.com/go-openapi/swag/yamlutils: v0.26.0 → v0.29.2
- github.com/google/cel-go: [v0.28.1 → v0.29.2](https://github.com/google/cel-go/compare/v0.28.1...v0.29.2)
- github.com/grpc-ecosystem/go-grpc-middleware/v2: v2.3.3 → v2.3.4
- github.com/grpc-ecosystem/grpc-gateway/v2: v2.29.0 → v2.31.0
- github.com/kubernetes-csi/csi-lib-utils: v0.24.0 → v0.25.0
- github.com/kubernetes-csi/csi-test/v5: v5.4.0 → v5.6.0
- github.com/kubernetes-csi/external-snapshotter/client/v8: v8.4.0 → v8.6.0
- github.com/prometheus/client_golang: [v1.23.2 → v1.24.1](https://github.com/prometheus/client_golang/compare/v1.23.2...v1.24.1)
- github.com/prometheus/client_model: [v0.6.2 → v0.6.3](https://github.com/prometheus/client_model/compare/v0.6.2...v0.6.3)
- github.com/prometheus/common: [v0.67.5 → v0.72.0](https://github.com/prometheus/common/compare/v0.67.5...v0.72.0)
- github.com/prometheus/procfs: [v0.20.1 → v0.22.0](https://github.com/prometheus/procfs/compare/v0.20.1...v0.22.0)
- go.etcd.io/etcd/api/v3: v3.6.11 → v3.7.2
- go.etcd.io/etcd/client/pkg/v3: v3.6.11 → v3.7.2
- go.etcd.io/etcd/client/v3: v3.6.11 → v3.7.2
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.68.0 → v0.72.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.68.0 → v0.71.0
- go.opentelemetry.io/otel: v1.43.0 → v1.47.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.43.0 → v1.47.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.43.0 → v1.47.0
- go.opentelemetry.io/otel/metric: v1.43.0 → v1.47.0
- go.opentelemetry.io/otel/sdk: v1.43.0 → v1.47.0
- go.opentelemetry.io/otel/trace: v1.43.0 → v1.47.0
- go.opentelemetry.io/proto/otlp: v1.10.0 → v1.11.0
- go.yaml.in/yaml/v3: v3.0.4 → v3.0.5
- golang.org/x/crypto: v0.52.0 → v0.57.0
- golang.org/x/exp: v0.0.0-20260112195511-716be5621a96 → v0.0.0-20260908205506-85c1c2202aba
- golang.org/x/net: v0.54.0 → v0.59.0
- golang.org/x/oauth2: v0.36.0 → v0.37.0
- golang.org/x/sync: v0.20.0 → v0.23.0
- golang.org/x/sys: v0.45.0 → v0.48.0
- golang.org/x/term: v0.43.0 → v0.46.0
- golang.org/x/text: v0.37.0 → v0.42.0
- golang.org/x/time: v0.15.0 → v0.16.0
- google.golang.org/genproto/googleapis/api: v0.0.0-20260414002931-afd174a4e478 → v0.0.0-20260928230214-8a89bd6388cc
- google.golang.org/genproto/googleapis/rpc: v0.0.0-20260414002931-afd174a4e478 → v0.0.0-20260928230214-8a89bd6388cc
- google.golang.org/grpc: v1.81.1 → v1.84.0
- google.golang.org/protobuf: v1.36.12-0.20260120151049-f2248ac996af → v1.36.12
- k8s.io/api: v0.36.1 → v0.37.0
- k8s.io/apiextensions-apiserver: v0.36.1 → v0.37.0
- k8s.io/apimachinery: v0.36.1 → v0.37.0
- k8s.io/apiserver: v0.36.1 → v0.37.0
- k8s.io/client-go: v0.36.1 → v0.37.0
- k8s.io/component-base: v0.36.1 → v0.37.0
- k8s.io/component-helpers: v0.36.1 → v0.37.0
- k8s.io/kube-openapi: v0.0.0-20260317180543-43fb72c5454a → v0.0.0-20260821135717-be32def86098
- k8s.io/streaming: v0.36.1 → v0.37.0
- k8s.io/utils: v0.0.0-20260210185600-b8788abfbbc2 → v0.0.0-20260707023825-cf1189d6abe3
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.35.0 → v0.36.0
- sigs.k8s.io/structured-merge-diff/v6: v6.4.0 → v6.4.2

### Removed
- github.com/go-openapi/swag/jsonname: v0.26.0
- github.com/gogo/protobuf: v1.3.2
- github.com/golang/mock: v1.6.0
