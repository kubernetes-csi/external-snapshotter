# Release notes for v8.7.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v8.6.0

## Changes by Kind

### Bug or Regression

- Skip NotFound errors when deleting VolumeSnapshotContent objects. ([#1460](https://github.com/kubernetes-csi/external-snapshotter/pull/1460), [@rvagner78](https://github.com/rvagner78))
- Requeue VolumeGroupSnapshotContent with backoff when it is not ready to use. ([#1461](https://github.com/kubernetes-csi/external-snapshotter/pull/1461), [@xing-yang](https://github.com/xing-yang))
- Fixed a data race in the in-flight operation metric. ([#1472](https://github.com/kubernetes-csi/external-snapshotter/pull/1472), [@andyzhangx](https://github.com/andyzhangx))
- Fixed GHSA-hrxh-6v49-42gf and Go CVEs by building with Go 1.26.6. ([#1438](https://github.com/kubernetes-csi/external-snapshotter/pull/1438), [#1465](https://github.com/kubernetes-csi/external-snapshotter/pull/1465), [#1466](https://github.com/kubernetes-csi/external-snapshotter/pull/1466), [#1471](https://github.com/kubernetes-csi/external-snapshotter/pull/1471), [@andyzhangx](https://github.com/andyzhangx))

### Other (Cleanup or Flake)

- Enabled the VolumeGroupSnapshot feature gate. ([#1475](https://github.com/kubernetes-csi/external-snapshotter/pull/1475), [@xing-yang](https://github.com/xing-yang))
- Updated the sidecar image versions in the deploy manifests to csi-provisioner v6.3.0, csi-snapshotter v8.6.0 and snapshot-controller v8.6.0. ([#1439](https://github.com/kubernetes-csi/external-snapshotter/pull/1439), [@humblec](https://github.com/humblec))
- Updated release-tools. ([#1449](https://github.com/kubernetes-csi/external-snapshotter/pull/1449), [@humblec](https://github.com/humblec))
- Updated the client module dependencies. ([#1429](https://github.com/kubernetes-csi/external-snapshotter/pull/1429), [@xing-yang](https://github.com/xing-yang))
- Bumped Kubernetes dependencies to v0.36.2 and other Go and GitHub Actions dependencies.

## Dependencies

### Changed
- github.com/felixge/httpsnoop: [v1.0.4 → v1.1.0](https://github.com/felixge/httpsnoop/compare/v1.0.4...v1.1.0)
- github.com/go-openapi/jsonpointer: [v0.23.1 → v1.0.0](https://github.com/go-openapi/jsonpointer/compare/v0.23.1...v1.0.0)
- github.com/go-openapi/jsonreference: [v0.21.5 → v1.0.0](https://github.com/go-openapi/jsonreference/compare/v0.21.5...v1.0.0)
- github.com/go-openapi/swag: [v0.26.0 → v0.27.0](https://github.com/go-openapi/swag/compare/v0.26.0...v0.27.0)
- github.com/go-openapi/swag/cmdutils: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/conv: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/fileutils: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/jsonutils: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/loading: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/mangling: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/netutils: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/stringutils: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/typeutils: v0.26.0 → v0.27.0
- github.com/go-openapi/swag/yamlutils: v0.26.0 → v0.27.0
- github.com/google/cel-go: [v0.28.1 → v0.29.2](https://github.com/google/cel-go/compare/v0.28.1...v0.29.2)
- github.com/kubernetes-csi/external-snapshotter/client/v8: v8.4.0 → v8.6.0
- github.com/prometheus/common: [v0.67.5 → v0.70.0](https://github.com/prometheus/common/compare/v0.67.5...v0.70.0)
- github.com/prometheus/procfs: [v0.20.1 → v0.21.1](https://github.com/prometheus/procfs/compare/v0.20.1...v0.21.1)
- go.etcd.io/etcd/api/v3: v3.6.11 → v3.7.0
- go.etcd.io/etcd/client/pkg/v3: v3.6.11 → v3.7.0
- go.etcd.io/etcd/client/v3: v3.6.11 → v3.7.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.68.0 → v0.69.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.68.0 → v0.69.0
- go.opentelemetry.io/otel: v1.43.0 → v1.44.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.43.0 → v1.44.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.43.0 → v1.44.0
- go.opentelemetry.io/otel/metric: v1.43.0 → v1.44.0
- go.opentelemetry.io/otel/sdk: v1.43.0 → v1.44.0
- go.opentelemetry.io/otel/trace: v1.43.0 → v1.44.0
- golang.org/x/crypto: v0.52.0 → v0.54.0
- golang.org/x/net: v0.54.0 → v0.57.0
- golang.org/x/sync: v0.20.0 → v0.22.0
- golang.org/x/sys: v0.45.0 → v0.47.0
- golang.org/x/term: v0.43.0 → v0.45.0
- golang.org/x/text: v0.37.0 → v0.40.0
- google.golang.org/genproto/googleapis/api: v0.0.0-20260414002931-afd174a4e478 → v0.0.0-20260526163538-3dc84a4a5aaa
- google.golang.org/genproto/googleapis/rpc: v0.0.0-20260414002931-afd174a4e478 → v0.0.0-20260810153831-ec0a7760b754
- google.golang.org/grpc: v1.81.1 → v1.83.0
- google.golang.org/protobuf: v1.36.12-0.20260120151049-f2248ac996af → v1.36.12
- k8s.io/api: v0.36.1 → v0.36.2
- k8s.io/apimachinery: v0.36.1 → v0.36.2
- k8s.io/apiserver: v0.36.1 → v0.36.2
- k8s.io/client-go: v0.36.1 → v0.36.2
- k8s.io/component-base: v0.36.1 → v0.36.2
- k8s.io/component-helpers: v0.36.1 → v0.36.2
- k8s.io/streaming: v0.36.1 → v0.36.2
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.35.0 → v0.36.0

### Removed
- github.com/go-openapi/swag/jsonname: v0.26.0
- github.com/gogo/protobuf: v1.3.2
