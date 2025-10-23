# Release notes for v8.4.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v8.3.0

## Changes by Kind

### API Change

- Introduce the `v1beta2` VolumeGroupSnapshot API as described by [KEP 5013](https://github.com/kubernetes/enhancements/pull/5013) ([#1312](https://github.com/kubernetes-csi/external-snapshotter/pull/1312), [@leonardoce](https://github.com/leonardoce))

### Feature

- The number of worker threads in the snapshot-controller and csi-snapshotter is now configurable via the `worker-threads` flag. ([#282](https://github.com/kubernetes-csi/external-snapshotter/pull/282), [@huffmanca](https://github.com/huffmanca))

### Other (Cleanup or Flake)

- Several VolumeGroupSnapshot, VolumeGroupSnapshotClass and VolumeGroupSnapshotContent fields are now immutable. The `v1beta1` VolumeGroupSnapshot API is now marked as deprecated. ([#1337](https://github.com/kubernetes-csi/external-snapshotter/pull/1337), [@leonardoce](https://github.com/leonardoce))
- Update kubernetes dependencies to v1.34.0 ([#1330](https://github.com/kubernetes-csi/external-snapshotter/pull/1330), [@dobsonj](https://github.com/dobsonj))

### Uncategorized

- Update CSI spec to v1.12. ([#1341](https://github.com/kubernetes-csi/external-snapshotter/pull/1341), [@xing-yang](https://github.com/xing-yang))

## Dependencies

### Added
- github.com/antihax/optional: [v1.0.0](https://github.com/antihax/optional/tree/v1.0.0)
- github.com/envoyproxy/go-control-plane/envoy: [v1.32.4](https://github.com/envoyproxy/go-control-plane/tree/envoy/v1.32.4)
- github.com/envoyproxy/go-control-plane/ratelimit: [v0.1.0](https://github.com/envoyproxy/go-control-plane/tree/ratelimit/v0.1.0)
- github.com/go-jose/go-jose/v4: [v4.0.4](https://github.com/go-jose/go-jose/tree/v4.0.4)
- github.com/godbus/dbus/v5: [v5.0.4](https://github.com/godbus/dbus/tree/v5.0.4)
- github.com/golang-jwt/jwt/v5: [v5.2.2](https://github.com/golang-jwt/jwt/tree/v5.2.2)
- github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus: [v1.0.1](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/providers/prometheus/v1.0.1)
- github.com/grpc-ecosystem/go-grpc-middleware/v2: [v2.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/v2.3.0)
- github.com/matttproud/golang_protobuf_extensions: [v1.0.1](https://github.com/matttproud/golang_protobuf_extensions/tree/v1.0.1)
- github.com/rogpeppe/fastuuid: [v1.2.0](https://github.com/rogpeppe/fastuuid/tree/v1.2.0)
- github.com/spiffe/go-spiffe/v2: [v2.5.0](https://github.com/spiffe/go-spiffe/tree/v2.5.0)
- github.com/zeebo/errs: [v1.4.0](https://github.com/zeebo/errs/tree/v1.4.0)
- go.etcd.io/raft/v3: v3.6.0
- go.yaml.in/yaml/v2: v2.4.2
- go.yaml.in/yaml/v3: v3.0.4
- k8s.io/apiextensions-apiserver: v0.34.0
- sigs.k8s.io/structured-merge-diff/v6: v6.3.0

### Changed
- cel.dev/expr: v0.19.1 → v0.24.0
- cloud.google.com/go/compute/metadata: v0.5.2 → v0.6.0
- github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp: [v1.24.2 → v1.26.0](https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/compare/detectors/gcp/v1.24.2...detectors/gcp/v1.26.0)
- github.com/cncf/xds/go: [b4127c9 → 2f00578](https://github.com/cncf/xds/compare/b4127c9...2f00578)
- github.com/container-storage-interface/spec: [v1.11.0 → v1.12.0](https://github.com/container-storage-interface/spec/compare/v1.11.0...v1.12.0)
- github.com/cpuguy83/go-md2man/v2: [v2.0.4 → v2.0.6](https://github.com/cpuguy83/go-md2man/compare/v2.0.4...v2.0.6)
- github.com/emicklei/go-restful/v3: [v3.12.1 → v3.12.2](https://github.com/emicklei/go-restful/compare/v3.12.1...v3.12.2)
- github.com/envoyproxy/go-control-plane: [v0.13.1 → v0.13.4](https://github.com/envoyproxy/go-control-plane/compare/v0.13.1...v0.13.4)
- github.com/envoyproxy/protoc-gen-validate: [v1.1.0 → v1.2.1](https://github.com/envoyproxy/protoc-gen-validate/compare/v1.1.0...v1.2.1)
- github.com/fsnotify/fsnotify: [v1.7.0 → v1.9.0](https://github.com/fsnotify/fsnotify/compare/v1.7.0...v1.9.0)
- github.com/fxamacker/cbor/v2: [v2.7.0 → v2.9.0](https://github.com/fxamacker/cbor/compare/v2.7.0...v2.9.0)
- github.com/golang/glog: [v1.2.2 → v1.2.4](https://github.com/golang/glog/compare/v1.2.2...v1.2.4)
- github.com/google/cel-go: [v0.23.2 → v0.26.0](https://github.com/google/cel-go/compare/v0.23.2...v0.26.0)
- github.com/google/gnostic-models: [v0.6.9 → v0.7.0](https://github.com/google/gnostic-models/compare/v0.6.9...v0.7.0)
- github.com/google/pprof: [d1b30fe → 40e02aa](https://github.com/google/pprof/compare/d1b30fe...40e02aa)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.24.0 → v2.26.3](https://github.com/grpc-ecosystem/grpc-gateway/compare/v2.24.0...v2.26.3)
- github.com/jonboulle/clockwork: [v0.4.0 → v0.5.0](https://github.com/jonboulle/clockwork/compare/v0.4.0...v0.5.0)
- github.com/kubernetes-csi/csi-test/v5: [v5.3.1 → v5.4.0](https://github.com/kubernetes-csi/csi-test/compare/v5.3.1...v5.4.0)
- github.com/modern-go/reflect2: [v1.0.2 → 35a7c28](https://github.com/modern-go/reflect2/compare/v1.0.2...35a7c28)
- github.com/onsi/ginkgo/v2: [v2.21.0 → v2.22.0](https://github.com/onsi/ginkgo/compare/v2.21.0...v2.22.0)
- github.com/onsi/gomega: [v1.35.1 → v1.36.1](https://github.com/onsi/gomega/compare/v1.35.1...v1.36.1)
- github.com/spf13/cobra: [v1.8.1 → v1.9.1](https://github.com/spf13/cobra/compare/v1.8.1...v1.9.1)
- github.com/spf13/pflag: [v1.0.5 → v1.0.6](https://github.com/spf13/pflag/compare/v1.0.5...v1.0.6)
- go.etcd.io/bbolt: v1.3.11 → v1.4.2
- go.etcd.io/etcd/api/v3: v3.5.21 → v3.6.4
- go.etcd.io/etcd/client/pkg/v3: v3.5.21 → v3.6.4
- go.etcd.io/etcd/client/v3: v3.5.21 → v3.6.4
- go.etcd.io/etcd/pkg/v3: v3.5.21 → v3.6.4
- go.etcd.io/etcd/server/v3: v3.5.21 → v3.6.4
- go.opentelemetry.io/contrib/detectors/gcp: v1.31.0 → v1.34.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.58.0 → v0.60.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.33.0 → v1.34.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.33.0 → v1.34.0
- go.opentelemetry.io/otel/metric: v1.33.0 → v1.35.0
- go.opentelemetry.io/otel/sdk/metric: v1.31.0 → v1.34.0
- go.opentelemetry.io/otel/sdk: v1.33.0 → v1.34.0
- go.opentelemetry.io/otel/trace: v1.33.0 → v1.35.0
- go.opentelemetry.io/otel: v1.33.0 → v1.35.0
- go.opentelemetry.io/proto/otlp: v1.4.0 → v1.5.0
- golang.org/x/mod: v0.20.0 → v0.21.0
- golang.org/x/tools: v0.26.0 → v0.28.0
- google.golang.org/genproto/googleapis/api: e6fa225 → a0af3ef
- google.golang.org/genproto/googleapis/rpc: 9240e9c → a0af3ef
- google.golang.org/grpc: v1.69.0 → v1.72.1
- k8s.io/api: v0.33.0 → v0.34.0
- k8s.io/apimachinery: v0.33.0 → v0.34.0
- k8s.io/apiserver: v0.33.0 → v0.34.0
- k8s.io/client-go: v0.33.0 → v0.34.0
- k8s.io/code-generator: v0.33.0 → v0.34.0
- k8s.io/component-base: v0.33.0 → v0.34.0
- k8s.io/component-helpers: v0.33.0 → v0.34.0
- k8s.io/gengo/v2: a7b603a → 85fd79d
- k8s.io/kms: v0.33.0 → v0.34.0
- k8s.io/kube-openapi: c8a335a → f3f2b99
- k8s.io/utils: 24370be → 4c0f3b2
- sigs.k8s.io/yaml: v1.4.0 → v1.6.0

### Removed
- github.com/census-instrumentation/opencensus-proto: [v0.4.1](https://github.com/census-instrumentation/opencensus-proto/tree/v0.4.1)
- github.com/go-task/slim-sprig: [52ccab3](https://github.com/go-task/slim-sprig/tree/52ccab3)
- github.com/golang-jwt/jwt/v4: [v4.5.2](https://github.com/golang-jwt/jwt/tree/v4.5.2)
- github.com/grpc-ecosystem/go-grpc-middleware: [v1.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/v1.3.0)
- github.com/grpc-ecosystem/grpc-gateway: [v1.16.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.16.0)
- go.etcd.io/etcd/client/v2: v2.305.21
- go.etcd.io/etcd/raft/v3: v3.5.21
- google.golang.org/genproto: ef43131
