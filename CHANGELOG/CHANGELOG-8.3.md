# Release notes for v8.3.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v8.2.0

## Changes by Kind

### Other (Cleanup or Flake)

- Update Kubernetes dependencies to 1.32.0 ([#1251](https://github.com/kubernetes-csi/external-snapshotter/pull/1251), [@dfajmon](https://github.com/dfajmon))

## Dependencies

### Added
- github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp: [v1.24.2](https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/tree/detectors/gcp/v1.24.2)
- go.opentelemetry.io/auto/sdk: v1.1.0
- go.opentelemetry.io/contrib/detectors/gcp: v1.31.0
- go.opentelemetry.io/otel/sdk/metric: v1.31.0

### Changed
- cel.dev/expr: v0.16.1 → v0.18.0
- cloud.google.com/go/compute/metadata: v0.5.0 → v0.5.2
- github.com/Azure/go-ansiterm: [d185dfc → 306776e](https://github.com/Azure/go-ansiterm/compare/d185dfc...306776e)
- github.com/envoyproxy/go-control-plane: [v0.13.0 → v0.13.1](https://github.com/envoyproxy/go-control-plane/compare/v0.13.0...v0.13.1)
- github.com/google/cel-go: [v0.20.1 → v0.22.0](https://github.com/google/cel-go/compare/v0.20.1...v0.22.0)
- github.com/google/pprof: [4bfdf5a → d1b30fe](https://github.com/google/pprof/compare/4bfdf5a...d1b30fe)
- github.com/gregjones/httpcache: [9cad4c3 → 901d907](https://github.com/gregjones/httpcache/compare/9cad4c3...901d907)
- github.com/jonboulle/clockwork: [v0.2.2 → v0.4.0](https://github.com/jonboulle/clockwork/compare/v0.2.2...v0.4.0)
- github.com/kubernetes-csi/csi-lib-utils: [v0.19.0 → v0.20.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.19.0...v0.20.0)
- github.com/mailru/easyjson: [v0.7.7 → v0.9.0](https://github.com/mailru/easyjson/compare/v0.7.7...v0.9.0)
- github.com/moby/spdystream: [v0.4.0 → v0.5.0](https://github.com/moby/spdystream/compare/v0.4.0...v0.5.0)
- github.com/onsi/ginkgo/v2: [v2.19.0 → v2.21.0](https://github.com/onsi/ginkgo/compare/v2.19.0...v2.21.0)
- github.com/onsi/gomega: [v1.33.1 → v1.35.1](https://github.com/onsi/gomega/compare/v1.33.1...v1.35.1)
- github.com/prometheus/common: [v0.60.1 → v0.61.0](https://github.com/prometheus/common/compare/v0.60.1...v0.61.0)
- github.com/rogpeppe/go-internal: [v1.12.0 → v1.13.1](https://github.com/rogpeppe/go-internal/compare/v1.12.0...v1.13.1)
- github.com/stoewer/go-strcase: [v1.2.0 → v1.3.0](https://github.com/stoewer/go-strcase/compare/v1.2.0...v1.3.0)
- github.com/stretchr/testify: [v1.9.0 → v1.10.0](https://github.com/stretchr/testify/compare/v1.9.0...v1.10.0)
- github.com/xiang90/probing: [43a291a → a49e3df](https://github.com/xiang90/probing/compare/43a291a...a49e3df)
- go.etcd.io/bbolt: v1.3.9 → v1.3.11
- go.etcd.io/etcd/api/v3: v3.5.14 → v3.5.16
- go.etcd.io/etcd/client/pkg/v3: v3.5.14 → v3.5.16
- go.etcd.io/etcd/client/v2: v2.305.13 → v2.305.16
- go.etcd.io/etcd/client/v3: v3.5.14 → v3.5.16
- go.etcd.io/etcd/pkg/v3: v3.5.13 → v3.5.16
- go.etcd.io/etcd/raft/v3: v3.5.13 → v3.5.16
- go.etcd.io/etcd/server/v3: v3.5.13 → v3.5.16
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.57.0 → v0.58.0
- go.opentelemetry.io/otel/metric: v1.32.0 → v1.33.0
- go.opentelemetry.io/otel/sdk: v1.28.0 → v1.31.0
- go.opentelemetry.io/otel/trace: v1.32.0 → v1.33.0
- go.opentelemetry.io/otel: v1.32.0 → v1.33.0
- go.uber.org/zap: v1.26.0 → v1.27.0
- golang.org/x/crypto: v0.29.0 → v0.30.0
- golang.org/x/exp: f3d0a9c → 8a7402a
- golang.org/x/mod: v0.17.0 → v0.20.0
- golang.org/x/net: v0.31.0 → v0.32.0
- golang.org/x/sync: v0.9.0 → v0.10.0
- golang.org/x/sys: v0.27.0 → v0.28.0
- golang.org/x/term: v0.26.0 → v0.27.0
- golang.org/x/text: v0.20.0 → v0.21.0
- golang.org/x/tools: e35e4cc → v0.26.0
- golang.org/x/xerrors: 04be3eb → 5ec99f8
- google.golang.org/genproto/googleapis/api: 8af14fe → 796eee8
- google.golang.org/genproto/googleapis/rpc: dd2ea8e → 9240e9c
- google.golang.org/genproto: b8732ec → ef43131
- google.golang.org/grpc: v1.68.0 → v1.69.0
- google.golang.org/protobuf: v1.35.2 → v1.36.0
- k8s.io/api: v0.31.0 → v0.32.0
- k8s.io/apimachinery: v0.31.0 → v0.32.0
- k8s.io/apiserver: v0.31.3 → v0.32.0
- k8s.io/client-go: v0.31.0 → v0.32.0
- k8s.io/code-generator: v0.31.0 → v0.30.0
- k8s.io/component-base: v0.31.0 → v0.32.0
- k8s.io/component-helpers: v0.31.0 → v0.32.0
- k8s.io/gengo/v2: 51d4e06 → a7b603a
- k8s.io/kms: v0.31.3 → v0.32.0
- k8s.io/kube-openapi: 70dd376 → 2c72e55
- k8s.io/utils: 18e509b → 24370be
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.30.3 → v0.31.0
- sigs.k8s.io/json: bc3834c → cfa47c3
- sigs.k8s.io/structured-merge-diff/v4: v4.4.3 → v4.5.0

### Removed
- github.com/golang/groupcache: [41bb18b](https://github.com/golang/groupcache/tree/41bb18b)
- github.com/imdario/mergo: [v0.3.13](https://github.com/imdario/mergo/tree/v0.3.13)
