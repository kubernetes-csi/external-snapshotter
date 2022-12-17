# Release notes for v6.2.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v6.1.0

## Changes by Kind

### Feature

- Add --retry-crd-interval-max flag to the snapshot-controller in order to allow customization of CRD detection on startup. ([#777](https://github.com/kubernetes-csi/external-snapshotter/pull/777), [@mattcary](https://github.com/mattcary))

### Uncategorized

- Change webhook example to be compatible with TLS-type secrets. ([#793](https://github.com/kubernetes-csi/external-snapshotter/pull/793), [@haslersn](https://github.com/haslersn))
- Fixes an issue introduced by PR 793 by respecting the format of TLS-type secrets in the script. ([#796](https://github.com/kubernetes-csi/external-snapshotter/pull/796), [@haslersn](https://github.com/haslersn))
- Update go to v1.19 and kubernetes dependencies to 1.26.0. ([#797](https://github.com/kubernetes-csi/external-snapshotter/pull/797), [@sunnylovestiramisu](https://github.com/sunnylovestiramisu))

## Dependencies

### Added
- github.com/cenkalti/backoff/v4: [v4.1.3](https://github.com/cenkalti/backoff/v4/tree/v4.1.3)
- github.com/go-logr/stdr: [v1.2.2](https://github.com/go-logr/stdr/tree/v1.2.2)
- github.com/grpc-ecosystem/grpc-gateway/v2: [v2.7.0](https://github.com/grpc-ecosystem/grpc-gateway/v2/tree/v2.7.0)
- go.opentelemetry.io/otel/exporters/otlp/internal/retry: v1.10.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc: v1.10.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace: v1.10.0
- k8s.io/dynamic-resource-allocation: v0.26.0
- k8s.io/kms: v0.26.0

### Changed
- github.com/antlr/antlr4/runtime/Go/antlr: [f25a4f6 → v1.4.10](https://github.com/antlr/antlr4/runtime/Go/antlr/compare/f25a4f6...v1.4.10)
- github.com/aws/aws-sdk-go: [v1.38.49 → v1.44.116](https://github.com/aws/aws-sdk-go/compare/v1.38.49...v1.44.116)
- github.com/container-storage-interface/spec: [v1.6.0 → v1.7.0](https://github.com/container-storage-interface/spec/compare/v1.6.0...v1.7.0)
- github.com/containerd/ttrpc: [v1.0.2 → v1.1.0](https://github.com/containerd/ttrpc/compare/v1.0.2...v1.1.0)
- github.com/docker/go-units: [v0.4.0 → v0.5.0](https://github.com/docker/go-units/compare/v0.4.0...v0.5.0)
- github.com/felixge/httpsnoop: [v1.0.1 → v1.0.3](https://github.com/felixge/httpsnoop/compare/v1.0.1...v1.0.3)
- github.com/google/cadvisor: [v0.45.0 → v0.46.0](https://github.com/google/cadvisor/compare/v0.45.0...v0.46.0)
- github.com/google/martian/v3: [v3.2.1 → v3.0.0](https://github.com/google/martian/v3/compare/v3.2.1...v3.0.0)
- github.com/ianlancetaylor/demangle: [28f6c0f → 5e5cf60](https://github.com/ianlancetaylor/demangle/compare/28f6c0f...5e5cf60)
- github.com/karrick/godirwalk: [v1.16.1 → v1.17.0](https://github.com/karrick/godirwalk/compare/v1.16.1...v1.17.0)
- github.com/kubernetes-csi/csi-lib-utils: [v0.11.0 → v0.12.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.11.0...v0.12.0)
- github.com/moby/sys/mountinfo: [v0.6.0 → v0.6.2](https://github.com/moby/sys/mountinfo/compare/v0.6.0...v0.6.2)
- github.com/moby/term: [3f7ff69 → 39b0c02](https://github.com/moby/term/compare/3f7ff69...39b0c02)
- github.com/onsi/ginkgo/v2: [v2.1.6 → v2.4.0](https://github.com/onsi/ginkgo/v2/compare/v2.1.6...v2.4.0)
- github.com/onsi/ginkgo: [v1.16.4 → v1.10.3](https://github.com/onsi/ginkgo/compare/v1.16.4...v1.10.3)
- github.com/onsi/gomega: [v1.20.1 → v1.23.0](https://github.com/onsi/gomega/compare/v1.20.1...v1.23.0)
- github.com/opencontainers/runc: [v1.1.3 → v1.1.4](https://github.com/opencontainers/runc/compare/v1.1.3...v1.1.4)
- github.com/prometheus/client_golang: [v1.13.1 → v1.14.0](https://github.com/prometheus/client_golang/compare/v1.13.1...v1.14.0)
- go.etcd.io/etcd/api/v3: v3.5.4 → v3.5.5
- go.etcd.io/etcd/client/pkg/v3: v3.5.4 → v3.5.5
- go.etcd.io/etcd/client/v2: v2.305.4 → v2.305.5
- go.etcd.io/etcd/client/v3: v3.5.4 → v3.5.5
- go.etcd.io/etcd/pkg/v3: v3.5.4 → v3.5.5
- go.etcd.io/etcd/raft/v3: v3.5.4 → v3.5.5
- go.etcd.io/etcd/server/v3: v3.5.4 → v3.5.5
- go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful: v0.20.0 → v0.35.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.20.0 → v0.35.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.20.0 → v0.35.0
- go.opentelemetry.io/otel/metric: v0.20.0 → v0.31.0
- go.opentelemetry.io/otel/sdk: v0.20.0 → v1.10.0
- go.opentelemetry.io/otel/trace: v0.20.0 → v1.10.0
- go.opentelemetry.io/otel: v0.20.0 → v1.10.0
- go.opentelemetry.io/proto/otlp: v0.7.0 → v0.19.0
- go.uber.org/goleak: v1.1.10 → v1.2.0
- golang.org/x/crypto: 3147a52 → v0.1.0
- golang.org/x/exp: 85be41e → 6cc2880
- golang.org/x/lint: 6edffad → 738671d
- golang.org/x/mod: 86c51ed → v0.6.0
- golang.org/x/net: v0.1.0 → v0.4.0
- golang.org/x/sys: v0.1.0 → v0.3.0
- golang.org/x/term: v0.1.0 → v0.3.0
- golang.org/x/text: v0.4.0 → v0.5.0
- golang.org/x/tools: v0.1.12 → v0.2.0
- k8s.io/api: v0.25.2 → v0.26.0
- k8s.io/apiextensions-apiserver: v0.25.2 → v0.26.0
- k8s.io/apimachinery: v0.25.2 → v0.26.0
- k8s.io/apiserver: v0.25.2 → v0.26.0
- k8s.io/cli-runtime: v0.25.2 → v0.26.0
- k8s.io/client-go: v0.25.2 → v0.26.0
- k8s.io/cloud-provider: v0.25.2 → v0.26.0
- k8s.io/cluster-bootstrap: v0.25.2 → v0.26.0
- k8s.io/code-generator: v0.25.2 → v0.26.0
- k8s.io/component-base: v0.25.2 → v0.26.0
- k8s.io/component-helpers: v0.25.2 → v0.26.0
- k8s.io/controller-manager: v0.25.2 → v0.26.0
- k8s.io/cri-api: v0.25.2 → v0.26.0
- k8s.io/csi-translation-lib: v0.25.2 → v0.26.0
- k8s.io/kube-aggregator: v0.25.2 → v0.26.0
- k8s.io/kube-controller-manager: v0.25.2 → v0.26.0
- k8s.io/kube-proxy: v0.25.2 → v0.26.0
- k8s.io/kube-scheduler: v0.25.2 → v0.26.0
- k8s.io/kubectl: v0.25.2 → v0.26.0
- k8s.io/kubelet: v0.25.2 → v0.26.0
- k8s.io/kubernetes: v1.25.3 → v1.26.0
- k8s.io/legacy-cloud-providers: v0.25.2 → v0.26.0
- k8s.io/metrics: v0.25.2 → v0.26.0
- k8s.io/mount-utils: v0.25.2 → v0.26.0
- k8s.io/pod-security-admission: v0.25.2 → v0.26.0
- k8s.io/sample-apiserver: v0.25.2 → v0.26.0
- k8s.io/system-validators: v1.7.0 → v1.8.0
- k8s.io/utils: 61b03e2 → 1a15be2

### Removed
- github.com/auth0/go-jwt-middleware: [v1.0.1](https://github.com/auth0/go-jwt-middleware/tree/v1.0.1)
- github.com/benbjohnson/clock: [v1.1.0](https://github.com/benbjohnson/clock/tree/v1.1.0)
- github.com/boltdb/bolt: [v1.3.1](https://github.com/boltdb/bolt/tree/v1.3.1)
- github.com/creack/pty: [v1.1.11](https://github.com/creack/pty/tree/v1.1.11)
- github.com/getkin/kin-openapi: [v0.76.0](https://github.com/getkin/kin-openapi/tree/v0.76.0)
- github.com/go-ozzo/ozzo-validation: [v3.5.0+incompatible](https://github.com/go-ozzo/ozzo-validation/tree/v3.5.0)
- github.com/golang/snappy: [v0.0.3](https://github.com/golang/snappy/tree/v0.0.3)
- github.com/gophercloud/gophercloud: [v0.1.0](https://github.com/gophercloud/gophercloud/tree/v0.1.0)
- github.com/gorilla/mux: [v1.8.0](https://github.com/gorilla/mux/tree/v1.8.0)
- github.com/heketi/heketi: [v10.3.0+incompatible](https://github.com/heketi/heketi/tree/v10.3.0)
- github.com/heketi/tests: [f3775cb](https://github.com/heketi/tests/tree/f3775cb)
- github.com/lpabon/godbc: [v0.1.1](https://github.com/lpabon/godbc/tree/v0.1.1)
- github.com/mvdan/xurls: [v1.1.0](https://github.com/mvdan/xurls/tree/v1.1.0)
- github.com/nxadm/tail: [v1.4.8](https://github.com/nxadm/tail/tree/v1.4.8)
- github.com/russross/blackfriday: [v1.5.2](https://github.com/russross/blackfriday/tree/v1.5.2)
- github.com/spf13/afero: [v1.2.2](https://github.com/spf13/afero/tree/v1.2.2)
- github.com/urfave/negroni: [v1.0.0](https://github.com/urfave/negroni/tree/v1.0.0)
- go.opentelemetry.io/contrib: v0.20.0
- go.opentelemetry.io/otel/exporters/otlp: v0.20.0
- go.opentelemetry.io/otel/oteltest: v0.20.0
- go.opentelemetry.io/otel/sdk/export/metric: v0.20.0
- go.opentelemetry.io/otel/sdk/metric: v0.20.0
- gonum.org/v1/gonum: v0.6.2
- gonum.org/v1/netlib: 7672324
- google.golang.org/grpc/cmd/protoc-gen-go-grpc: v1.1.0
