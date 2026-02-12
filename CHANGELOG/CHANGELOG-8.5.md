# Release notes for v8.5.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v8.4.0

## Changes by Kind

### Bug or Regression

- Updated go version to fix CVE-2025-68121. ([#1376](https://github.com/kubernetes-csi/external-snapshotter/pull/1376), [@jsafrane](https://github.com/jsafrane))

### Other (Cleanup or Flake)

- Bump k8s dependencies to v1.35.0 ([#1363](https://github.com/kubernetes-csi/external-snapshotter/pull/1363), [@dfajmon](https://github.com/dfajmon))

## Dependencies

### Added
- github.com/Masterminds/semver/v3: [v3.4.0](https://github.com/Masterminds/semver/tree/v3.4.0)
- github.com/cenkalti/backoff/v5: [v5.0.3](https://github.com/cenkalti/backoff/tree/v5.0.3)
- github.com/go-openapi/swag/cmdutils: [v0.25.4](https://github.com/go-openapi/swag/tree/cmdutils/v0.25.4)
- github.com/go-openapi/swag/conv: [v0.25.4](https://github.com/go-openapi/swag/tree/conv/v0.25.4)
- github.com/go-openapi/swag/fileutils: [v0.25.4](https://github.com/go-openapi/swag/tree/fileutils/v0.25.4)
- github.com/go-openapi/swag/jsonname: [v0.25.4](https://github.com/go-openapi/swag/tree/jsonname/v0.25.4)
- github.com/go-openapi/swag/jsonutils/fixtures_test: [v0.25.4](https://github.com/go-openapi/swag/tree/jsonutils/fixtures_test/v0.25.4)
- github.com/go-openapi/swag/jsonutils: [v0.25.4](https://github.com/go-openapi/swag/tree/jsonutils/v0.25.4)
- github.com/go-openapi/swag/loading: [v0.25.4](https://github.com/go-openapi/swag/tree/loading/v0.25.4)
- github.com/go-openapi/swag/mangling: [v0.25.4](https://github.com/go-openapi/swag/tree/mangling/v0.25.4)
- github.com/go-openapi/swag/netutils: [v0.25.4](https://github.com/go-openapi/swag/tree/netutils/v0.25.4)
- github.com/go-openapi/swag/stringutils: [v0.25.4](https://github.com/go-openapi/swag/tree/stringutils/v0.25.4)
- github.com/go-openapi/swag/typeutils: [v0.25.4](https://github.com/go-openapi/swag/tree/typeutils/v0.25.4)
- github.com/go-openapi/swag/yamlutils: [v0.25.4](https://github.com/go-openapi/swag/tree/yamlutils/v0.25.4)
- github.com/go-openapi/testify/enable/yaml/v2: [v2.0.2](https://github.com/go-openapi/testify/tree/enable/yaml/v2/v2.0.2)
- github.com/go-openapi/testify/v2: [v2.0.2](https://github.com/go-openapi/testify/tree/v2.0.2)
- golang.org/x/tools/go/expect: v0.1.1-deprecated
- golang.org/x/tools/go/packages/packagestest: v0.1.1-deprecated
- gonum.org/v1/gonum: v0.16.0

### Changed
- cel.dev/expr: v0.24.0 → v0.25.1
- cloud.google.com/go/compute/metadata: v0.6.0 → v0.9.0
- github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp: [v1.26.0 → v1.30.0](https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/compare/detectors/gcp/v1.26.0...detectors/gcp/v1.30.0)
- github.com/alecthomas/units: [b94a6e3 → 0f3dac3](https://github.com/alecthomas/units/compare/b94a6e3...0f3dac3)
- github.com/antlr4-go/antlr/v4: [v4.13.0 → v4.13.1](https://github.com/antlr4-go/antlr/compare/v4.13.0...v4.13.1)
- github.com/cncf/xds/go: [2f00578 → 0feb691](https://github.com/cncf/xds/compare/2f00578...0feb691)
- github.com/coreos/go-systemd/v22: [v22.5.0 → v22.7.0](https://github.com/coreos/go-systemd/compare/v22.5.0...v22.7.0)
- github.com/emicklei/go-restful/v3: [v3.12.2 → v3.13.0](https://github.com/emicklei/go-restful/compare/v3.12.2...v3.13.0)
- github.com/envoyproxy/go-control-plane/envoy: [v1.32.4 → v1.35.0](https://github.com/envoyproxy/go-control-plane/compare/envoy/v1.32.4...envoy/v1.35.0)
- github.com/envoyproxy/go-control-plane: [v0.13.4 → 75eaa19](https://github.com/envoyproxy/go-control-plane/compare/v0.13.4...75eaa19)
- github.com/evanphx/json-patch: [v5.9.0+incompatible → v5.9.11+incompatible](https://github.com/evanphx/json-patch/compare/v5.9.0...v5.9.11)
- github.com/go-jose/go-jose/v4: [v4.0.4 → v4.1.3](https://github.com/go-jose/go-jose/compare/v4.0.4...v4.1.3)
- github.com/go-logr/logr: [v1.4.2 → v1.4.3](https://github.com/go-logr/logr/compare/v1.4.2...v1.4.3)
- github.com/go-openapi/jsonpointer: [v0.21.0 → v0.22.4](https://github.com/go-openapi/jsonpointer/compare/v0.21.0...v0.22.4)
- github.com/go-openapi/jsonreference: [v0.21.0 → v0.21.4](https://github.com/go-openapi/jsonreference/compare/v0.21.0...v0.21.4)
- github.com/go-openapi/swag: [v0.23.0 → v0.25.4](https://github.com/go-openapi/swag/compare/v0.23.0...v0.25.4)
- github.com/godbus/dbus/v5: [v5.0.4 → v5.1.0](https://github.com/godbus/dbus/compare/v5.0.4...v5.1.0)
- github.com/golang-jwt/jwt/v5: [v5.2.2 → v5.3.0](https://github.com/golang-jwt/jwt/compare/v5.2.2...v5.3.0)
- github.com/golang/glog: [v1.2.4 → v1.2.5](https://github.com/golang/glog/compare/v1.2.4...v1.2.5)
- github.com/google/cel-go: [v0.26.0 → v0.27.0](https://github.com/google/cel-go/compare/v0.26.0...v0.27.0)
- github.com/google/gnostic-models: [v0.7.0 → v0.7.1](https://github.com/google/gnostic-models/compare/v0.7.0...v0.7.1)
- github.com/google/pprof: [40e02aa → 27863c8](https://github.com/google/pprof/compare/40e02aa...27863c8)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.26.3 → v2.27.7](https://github.com/grpc-ecosystem/grpc-gateway/compare/v2.26.3...v2.27.7)
- github.com/kubernetes-csi/csi-lib-utils: [v0.22.0 → v0.23.1](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.22.0...v0.23.1)
- github.com/onsi/ginkgo/v2: [v2.22.0 → v2.27.2](https://github.com/onsi/ginkgo/compare/v2.22.0...v2.27.2)
- github.com/onsi/gomega: [v1.36.1 → v1.38.2](https://github.com/onsi/gomega/compare/v1.36.1...v1.38.2)
- github.com/prometheus/client_golang: [v1.22.0 → v1.23.2](https://github.com/prometheus/client_golang/compare/v1.22.0...v1.23.2)
- github.com/prometheus/client_model: [v0.6.1 → v0.6.2](https://github.com/prometheus/client_model/compare/v0.6.1...v0.6.2)
- github.com/prometheus/common: [v0.62.0 → v0.67.5](https://github.com/prometheus/common/compare/v0.62.0...v0.67.5)
- github.com/prometheus/procfs: [v0.15.1 → v0.19.2](https://github.com/prometheus/procfs/compare/v0.15.1...v0.19.2)
- github.com/rogpeppe/go-internal: [v1.13.1 → v1.14.1](https://github.com/rogpeppe/go-internal/compare/v1.13.1...v1.14.1)
- github.com/spf13/cobra: [v1.9.1 → v1.10.2](https://github.com/spf13/cobra/compare/v1.9.1...v1.10.2)
- github.com/spf13/pflag: [v1.0.6 → v1.0.10](https://github.com/spf13/pflag/compare/v1.0.6...v1.0.10)
- github.com/spiffe/go-spiffe/v2: [v2.5.0 → v2.6.0](https://github.com/spiffe/go-spiffe/compare/v2.5.0...v2.6.0)
- github.com/stretchr/testify: [v1.10.0 → v1.11.1](https://github.com/stretchr/testify/compare/v1.10.0...v1.11.1)
- go.etcd.io/bbolt: v1.4.2 → v1.4.3
- go.etcd.io/etcd/api/v3: v3.6.4 → v3.6.7
- go.etcd.io/etcd/client/pkg/v3: v3.6.4 → v3.6.7
- go.etcd.io/etcd/client/v3: v3.6.4 → v3.6.7
- go.etcd.io/etcd/pkg/v3: v3.6.4 → v3.6.5
- go.etcd.io/etcd/server/v3: v3.6.4 → v3.6.5
- go.opentelemetry.io/auto/sdk: v1.1.0 → v1.2.1
- go.opentelemetry.io/contrib/detectors/gcp: v1.34.0 → v1.38.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.60.0 → v0.65.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.58.0 → v0.65.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.34.0 → v1.40.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.34.0 → v1.40.0
- go.opentelemetry.io/otel/metric: v1.35.0 → v1.40.0
- go.opentelemetry.io/otel/sdk/metric: v1.34.0 → v1.40.0
- go.opentelemetry.io/otel/sdk: v1.34.0 → v1.40.0
- go.opentelemetry.io/otel/trace: v1.35.0 → v1.40.0
- go.opentelemetry.io/otel: v1.35.0 → v1.40.0
- go.opentelemetry.io/proto/otlp: v1.5.0 → v1.9.0
- go.uber.org/zap: v1.27.0 → v1.27.1
- go.yaml.in/yaml/v2: v2.4.2 → v2.4.3
- golang.org/x/crypto: v0.37.0 → v0.47.0
- golang.org/x/exp: 8a7402a → 716be56
- golang.org/x/mod: v0.21.0 → v0.32.0
- golang.org/x/net: v0.39.0 → v0.49.0
- golang.org/x/oauth2: v0.27.0 → v0.35.0
- golang.org/x/sync: v0.13.0 → v0.19.0
- golang.org/x/sys: v0.32.0 → v0.41.0
- golang.org/x/term: v0.31.0 → v0.39.0
- golang.org/x/text: v0.24.0 → v0.33.0
- golang.org/x/time: v0.9.0 → v0.14.0
- golang.org/x/tools: v0.28.0 → v0.41.0
- google.golang.org/genproto/googleapis/api: a0af3ef → 8636f87
- google.golang.org/genproto/googleapis/rpc: a0af3ef → 8636f87
- google.golang.org/grpc: v1.72.1 → v1.78.0
- google.golang.org/protobuf: v1.36.5 → v1.36.11
- gopkg.in/evanphx/json-patch.v4: v4.12.0 → v4.13.0
- k8s.io/api: v0.34.0 → v0.35.0
- k8s.io/apiextensions-apiserver: v0.34.0 → v0.35.0
- k8s.io/apimachinery: v0.34.0 → v0.35.0
- k8s.io/apiserver: v0.34.0 → v0.35.0
- k8s.io/client-go: v0.34.0 → v0.35.0
- k8s.io/code-generator: v0.34.0 → v0.35.0
- k8s.io/component-base: v0.34.0 → v0.35.0
- k8s.io/component-helpers: v0.34.0 → v0.35.0
- k8s.io/gengo/v2: 85fd79d → ec3ebc5
- k8s.io/kms: v0.34.0 → v0.35.0
- k8s.io/kube-openapi: f3f2b99 → 4e65d59
- k8s.io/utils: 4c0f3b2 → 914a6e7
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.31.2 → v0.34.0
- sigs.k8s.io/json: cfa47c3 → 2d32026
- sigs.k8s.io/structured-merge-diff/v4: v4.6.0 → v4.4.1
- sigs.k8s.io/structured-merge-diff/v6: v6.3.0 → v6.3.1

### Removed
- github.com/zeebo/errs: [v1.4.0](https://github.com/zeebo/errs/tree/v1.4.0)
