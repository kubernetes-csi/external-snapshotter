# Release notes for v4.2.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v4.1.0

## Changes by Kind

### Feature

#### Snapshot APIs 

- The namespace of the referenced VolumeSnapshot is printed when printing a VolumeSnapshotContent. ([#535](https://github.com/kubernetes-csi/external-snapshotter/pull/535), [@tsmetana](https://github.com/tsmetana))

#### Snapshot Controller

- `retry-interval-start` and `retry-interval-max` arguments are added to common-controller which controls retry interval of failed volume snapshot creation and deletion. These values set the ratelimiter for snapshot and content queues. ([#530](https://github.com/kubernetes-csi/external-snapshotter/pull/530), [@humblec](https://github.com/humblec))
- Add command line arguments `leader-election-lease-duration`, `leader-election-renew-deadline`, and `leader-election-retry-period` to configure leader election options for the snapshot controller. ([#575](https://github.com/kubernetes-csi/external-snapshotter/pull/575), [@bertinatto](https://github.com/bertinatto))
- Adds an operations_in_flight metric for determining the number of snapshot operations in progress. ([#519](https://github.com/kubernetes-csi/external-snapshotter/pull/519), [@ggriffiths](https://github.com/ggriffiths))
- Introduced "SnapshotCreated" and "SnapshotReady" events. ([#540](https://github.com/kubernetes-csi/external-snapshotter/pull/540), [@rexagod](https://github.com/rexagod))

#### CSI Snapshotter Sidecar

- `retry-interval-start` and `retry-interval-max` arguments are added to csi-snapshotter sidecar which controls retry interval of failed volume snapshot creation and deletion. These values set the ratelimiter for volumesnapshotcontent queue. ([#308](https://github.com/kubernetes-csi/external-snapshotter/pull/308), [@humblec](https://github.com/humblec))
- Add command line arguments `leader-election-lease-duration`, `leader-election-renew-deadline`, and `leader-election-retry-period` to configure leader election options for CSI snapshotter sidecar. ([#538](https://github.com/kubernetes-csi/external-snapshotter/pull/538), [@RaunakShah](https://github.com/RaunakShah))

### Bug or Regression

#### Snapshot Controller

- Add process_start_time_seconds metric ([#569](https://github.com/kubernetes-csi/external-snapshotter/pull/569), [@saikat-royc](https://github.com/saikat-royc))
- Adds the leader election health check for the snapshot controller at `/healthz/leader-election` ([#573](https://github.com/kubernetes-csi/external-snapshotter/pull/573), [@ggriffiths](https://github.com/ggriffiths))
- Remove kube-system namespace verification during startup and instead list volumes across all namespaces ([#515](https://github.com/kubernetes-csi/external-snapshotter/pull/515), [@mauriciopoppe](https://github.com/mauriciopoppe))

### Other (Cleanup or Flake)

- Updates Kubernetes dependencies to v1.22.0 ([#570](https://github.com/kubernetes-csi/external-snapshotter/pull/570), [@chrishenzie](https://github.com/chrishenzie)) [SIG Storage]
- Updates csi-lib-utils dependency to v0.10.0 ([#574](https://github.com/kubernetes-csi/external-snapshotter/pull/574), [@chrishenzie](https://github.com/chrishenzie))
- Updates container-storage-interface dependency to v1.5.0 ([#532](https://github.com/kubernetes-csi/external-snapshotter/pull/532), [@chrishenzie](https://github.com/chrishenzie))

#### Snapshot Validation Webhook

- Changed the webhook image from distroless/base to distroless/static. ([#550](https://github.com/kubernetes-csi/external-snapshotter/pull/550), [@WanzenBug](https://github.com/WanzenBug))

## Dependencies

### Added
- github.com/antihax/optional: [v1.0.0](https://github.com/antihax/optional/tree/v1.0.0)
- github.com/benbjohnson/clock: [v1.0.3](https://github.com/benbjohnson/clock/tree/v1.0.3)
- github.com/certifi/gocertifi: [2c3bb06](https://github.com/certifi/gocertifi/tree/2c3bb06)
- github.com/cockroachdb/errors: [v1.2.4](https://github.com/cockroachdb/errors/tree/v1.2.4)
- github.com/cockroachdb/logtags: [eb05cc2](https://github.com/cockroachdb/logtags/tree/eb05cc2)
- github.com/felixge/httpsnoop: [v1.0.1](https://github.com/felixge/httpsnoop/tree/v1.0.1)
- github.com/getsentry/raven-go: [v0.2.0](https://github.com/getsentry/raven-go/tree/v0.2.0)
- github.com/go-kit/log: [v0.1.0](https://github.com/go-kit/log/tree/v0.1.0)
- github.com/gofrs/uuid: [v4.0.0+incompatible](https://github.com/gofrs/uuid/tree/v4.0.0)
- github.com/josharian/intern: [v1.0.0](https://github.com/josharian/intern/tree/v1.0.0)
- github.com/nxadm/tail: [v1.4.4](https://github.com/nxadm/tail/tree/v1.4.4)
- go.etcd.io/etcd/api/v3: v3.5.0
- go.etcd.io/etcd/client/pkg/v3: v3.5.0
- go.etcd.io/etcd/client/v2: v2.305.0
- go.etcd.io/etcd/client/v3: v3.5.0
- go.etcd.io/etcd/pkg/v3: v3.5.0
- go.etcd.io/etcd/raft/v3: v3.5.0
- go.etcd.io/etcd/server/v3: v3.5.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.20.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.20.0
- go.opentelemetry.io/contrib: v0.20.0
- go.opentelemetry.io/otel/exporters/otlp: v0.20.0
- go.opentelemetry.io/otel/metric: v0.20.0
- go.opentelemetry.io/otel/oteltest: v0.20.0
- go.opentelemetry.io/otel/sdk/export/metric: v0.20.0
- go.opentelemetry.io/otel/sdk/metric: v0.20.0
- go.opentelemetry.io/otel/sdk: v0.20.0
- go.opentelemetry.io/otel/trace: v0.20.0
- go.opentelemetry.io/otel: v0.20.0
- go.opentelemetry.io/proto/otlp: v0.7.0
- go.uber.org/goleak: v1.1.10

### Changed
- github.com/Azure/azure-sdk-for-go: [v43.0.0+incompatible → v55.0.0+incompatible](https://github.com/Azure/azure-sdk-for-go/compare/v43.0.0...v55.0.0)
- github.com/Azure/go-ansiterm: [d6e3b33 → d185dfc](https://github.com/Azure/go-ansiterm/compare/d6e3b33...d185dfc)
- github.com/Azure/go-autorest/autorest/adal: [v0.9.5 → v0.9.13](https://github.com/Azure/go-autorest/autorest/adal/compare/v0.9.5...v0.9.13)
- github.com/Azure/go-autorest/autorest/to: [v0.2.0 → v0.4.0](https://github.com/Azure/go-autorest/autorest/to/compare/v0.2.0...v0.4.0)
- github.com/Azure/go-autorest/autorest: [v0.11.12 → v0.11.18](https://github.com/Azure/go-autorest/autorest/compare/v0.11.12...v0.11.18)
- github.com/Azure/go-autorest/logger: [v0.2.0 → v0.2.1](https://github.com/Azure/go-autorest/logger/compare/v0.2.0...v0.2.1)
- github.com/aws/aws-sdk-go: [v1.35.24 → v1.38.49](https://github.com/aws/aws-sdk-go/compare/v1.35.24...v1.38.49)
- github.com/cenkalti/backoff: [v2.2.1+incompatible → v2.1.1+incompatible](https://github.com/cenkalti/backoff/compare/v2.2.1...v2.1.1)
- github.com/cncf/udpa/go: [efcf912 → 5459f2c](https://github.com/cncf/udpa/go/compare/efcf912...5459f2c)
- github.com/cockroachdb/datadriven: [80d97fb → bf6692d](https://github.com/cockroachdb/datadriven/compare/80d97fb...bf6692d)
- github.com/container-storage-interface/spec: [v1.4.0 → v1.5.0](https://github.com/container-storage-interface/spec/compare/v1.4.0...v1.5.0)
- github.com/coreos/go-systemd/v22: [v22.1.0 → v22.3.2](https://github.com/coreos/go-systemd/v22/compare/v22.1.0...v22.3.2)
- github.com/envoyproxy/go-control-plane: [v0.9.7 → 668b12f](https://github.com/envoyproxy/go-control-plane/compare/v0.9.7...668b12f)
- github.com/evanphx/json-patch: [v4.9.0+incompatible → v4.11.0+incompatible](https://github.com/evanphx/json-patch/compare/v4.9.0...v4.11.0)
- github.com/form3tech-oss/jwt-go: [v3.2.2+incompatible → v3.2.3+incompatible](https://github.com/form3tech-oss/jwt-go/compare/v3.2.2...v3.2.3)
- github.com/go-kit/kit: [v0.10.0 → v0.9.0](https://github.com/go-kit/kit/compare/v0.10.0...v0.9.0)
- github.com/go-openapi/jsonpointer: [v0.19.3 → v0.19.5](https://github.com/go-openapi/jsonpointer/compare/v0.19.3...v0.19.5)
- github.com/go-openapi/jsonreference: [v0.19.3 → v0.19.5](https://github.com/go-openapi/jsonreference/compare/v0.19.3...v0.19.5)
- github.com/go-openapi/swag: [v0.19.5 → v0.19.14](https://github.com/go-openapi/swag/compare/v0.19.5...v0.19.14)
- github.com/godbus/dbus/v5: [v5.0.3 → v5.0.4](https://github.com/godbus/dbus/v5/compare/v5.0.3...v5.0.4)
- github.com/golang/groupcache: [8c9f03a → 41bb18b](https://github.com/golang/groupcache/compare/8c9f03a...41bb18b)
- github.com/golang/protobuf: [v1.4.3 → v1.5.2](https://github.com/golang/protobuf/compare/v1.4.3...v1.5.2)
- github.com/google/btree: [v1.0.0 → v1.0.1](https://github.com/google/btree/compare/v1.0.0...v1.0.1)
- github.com/google/go-cmp: [v0.5.4 → v0.5.5](https://github.com/google/go-cmp/compare/v0.5.4...v0.5.5)
- github.com/googleapis/gnostic: [v0.5.3 → v0.5.5](https://github.com/googleapis/gnostic/compare/v0.5.3...v0.5.5)
- github.com/grpc-ecosystem/go-grpc-middleware: [f849b54 → v1.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/compare/f849b54...v1.3.0)
- github.com/grpc-ecosystem/grpc-gateway: [v1.9.5 → v1.16.0](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.9.5...v1.16.0)
- github.com/hashicorp/consul/api: [v1.3.0 → v1.1.0](https://github.com/hashicorp/consul/api/compare/v1.3.0...v1.1.0)
- github.com/hashicorp/consul/sdk: [v0.3.0 → v0.1.1](https://github.com/hashicorp/consul/sdk/compare/v0.3.0...v0.1.1)
- github.com/hashicorp/golang-lru: [v0.5.4 → v0.5.1](https://github.com/hashicorp/golang-lru/compare/v0.5.4...v0.5.1)
- github.com/jonboulle/clockwork: [v0.1.0 → v0.2.2](https://github.com/jonboulle/clockwork/compare/v0.1.0...v0.2.2)
- github.com/json-iterator/go: [v1.1.10 → v1.1.11](https://github.com/json-iterator/go/compare/v1.1.10...v1.1.11)
- github.com/kubernetes-csi/csi-lib-utils: [v0.9.0 → v0.10.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.9.0...v0.10.0)
- github.com/mailru/easyjson: [v0.7.0 → v0.7.6](https://github.com/mailru/easyjson/compare/v0.7.0...v0.7.6)
- github.com/moby/term: [df9cb8a → 9d4ed18](https://github.com/moby/term/compare/df9cb8a...9d4ed18)
- github.com/onsi/ginkgo: [v1.11.0 → v1.14.0](https://github.com/onsi/ginkgo/compare/v1.11.0...v1.14.0)
- github.com/onsi/gomega: [v1.7.1 → v1.10.1](https://github.com/onsi/gomega/compare/v1.7.1...v1.10.1)
- github.com/prometheus/client_golang: [v1.8.0 → v1.11.0](https://github.com/prometheus/client_golang/compare/v1.8.0...v1.11.0)
- github.com/prometheus/common: [v0.15.0 → v0.26.0](https://github.com/prometheus/common/compare/v0.15.0...v0.26.0)
- github.com/prometheus/procfs: [v0.2.0 → v0.6.0](https://github.com/prometheus/procfs/compare/v0.2.0...v0.6.0)
- github.com/rogpeppe/fastuuid: [6724a57 → v1.2.0](https://github.com/rogpeppe/fastuuid/compare/6724a57...v1.2.0)
- github.com/sirupsen/logrus: [v1.7.0 → v1.8.1](https://github.com/sirupsen/logrus/compare/v1.7.0...v1.8.1)
- github.com/soheilhy/cmux: [v0.1.4 → v0.1.5](https://github.com/soheilhy/cmux/compare/v0.1.4...v0.1.5)
- github.com/spf13/cobra: [v1.1.1 → v1.1.3](https://github.com/spf13/cobra/compare/v1.1.1...v1.1.3)
- github.com/stretchr/testify: [v1.6.1 → v1.7.0](https://github.com/stretchr/testify/compare/v1.6.1...v1.7.0)
- github.com/tmc/grpc-websocket-proxy: [0ad062e → e5319fd](https://github.com/tmc/grpc-websocket-proxy/compare/0ad062e...e5319fd)
- github.com/yuin/goldmark: [v1.2.1 → v1.3.5](https://github.com/yuin/goldmark/compare/v1.2.1...v1.3.5)
- go.etcd.io/bbolt: v1.3.5 → v1.3.6
- go.uber.org/atomic: v1.5.0 → v1.7.0
- go.uber.org/multierr: v1.3.0 → v1.6.0
- go.uber.org/zap: v1.13.0 → v1.17.0
- golang.org/x/lint: 738671d → 6edffad
- golang.org/x/mod: ce943fd → v0.4.2
- golang.org/x/net: 3d97a24 → 37e1c6a
- golang.org/x/sync: 67f06af → 036812b
- golang.org/x/sys: a50acf3 → 59db8d7
- golang.org/x/text: v0.3.4 → v0.3.6
- golang.org/x/time: f8bda1e → 1f47c86
- golang.org/x/tools: v0.1.0 → v0.1.2
- google.golang.org/genproto: 40ec1c2 → f16073e
- google.golang.org/grpc: v1.34.0 → v1.38.0
- google.golang.org/protobuf: v1.25.0 → v1.26.0
- gopkg.in/gcfg.v1: v1.2.3 → v1.2.0
- gopkg.in/warnings.v0: v0.1.2 → v0.1.1
- gopkg.in/yaml.v3: eeeca48 → 496545a
- k8s.io/api: v0.21.0 → v0.22.0
- k8s.io/apiextensions-apiserver: v0.21.0 → v0.22.0
- k8s.io/apimachinery: v0.21.0 → v0.22.0
- k8s.io/apiserver: v0.21.0 → v0.22.0
- k8s.io/cli-runtime: v0.21.0 → v0.22.0
- k8s.io/client-go: v0.21.0 → v0.22.0
- k8s.io/cloud-provider: v0.21.0 → v0.22.0
- k8s.io/cluster-bootstrap: v0.21.0 → v0.22.0
- k8s.io/code-generator: v0.21.0 → v0.22.0
- k8s.io/component-base: v0.21.0 → v0.22.0
- k8s.io/component-helpers: v0.21.0 → v0.22.0
- k8s.io/controller-manager: v0.21.0 → v0.22.0
- k8s.io/cri-api: v0.21.0 → v0.22.0
- k8s.io/csi-translation-lib: v0.21.0 → v0.22.0
- k8s.io/klog/v2: v2.8.0 → v2.9.0
- k8s.io/kube-aggregator: v0.21.0 → v0.22.0
- k8s.io/kube-controller-manager: v0.21.0 → v0.22.0
- k8s.io/kube-openapi: 591a79e → 9528897
- k8s.io/kube-proxy: v0.21.0 → v0.22.0
- k8s.io/kube-scheduler: v0.21.0 → v0.22.0
- k8s.io/kubectl: v0.21.0 → v0.22.0
- k8s.io/kubelet: v0.21.0 → v0.22.0
- k8s.io/legacy-cloud-providers: v0.21.0 → v0.22.0
- k8s.io/metrics: v0.21.0 → v0.22.0
- k8s.io/mount-utils: v0.21.0 → v0.22.0
- k8s.io/sample-apiserver: v0.21.0 → v0.22.0
- k8s.io/utils: 67b214c → 4b05e18
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.15 → v0.0.22
- sigs.k8s.io/kustomize/api: v0.8.5 → v0.8.11
- sigs.k8s.io/kustomize/cmd/config: v0.9.7 → v0.9.13
- sigs.k8s.io/kustomize/kustomize/v4: v4.0.5 → v4.2.0
- sigs.k8s.io/kustomize/kyaml: v0.10.15 → v0.11.0
- sigs.k8s.io/structured-merge-diff/v4: v4.1.0 → v4.1.2

### Removed
- github.com/Knetic/govaluate: [9aa4983](https://github.com/Knetic/govaluate/tree/9aa4983)
- github.com/Shopify/sarama: [v1.19.0](https://github.com/Shopify/sarama/tree/v1.19.0)
- github.com/Shopify/toxiproxy: [v2.1.4+incompatible](https://github.com/Shopify/toxiproxy/tree/v2.1.4)
- github.com/VividCortex/gohistogram: [v1.0.0](https://github.com/VividCortex/gohistogram/tree/v1.0.0)
- github.com/afex/hystrix-go: [fa1af6a](https://github.com/afex/hystrix-go/tree/fa1af6a)
- github.com/agnivade/levenshtein: [v1.0.1](https://github.com/agnivade/levenshtein/tree/v1.0.1)
- github.com/andreyvit/diff: [c7f18ee](https://github.com/andreyvit/diff/tree/c7f18ee)
- github.com/apache/thrift: [v0.13.0](https://github.com/apache/thrift/tree/v0.13.0)
- github.com/aryann/difflib: [e206f87](https://github.com/aryann/difflib/tree/e206f87)
- github.com/aws/aws-lambda-go: [v1.13.3](https://github.com/aws/aws-lambda-go/tree/v1.13.3)
- github.com/aws/aws-sdk-go-v2: [v0.18.0](https://github.com/aws/aws-sdk-go-v2/tree/v0.18.0)
- github.com/casbin/casbin/v2: [v2.1.2](https://github.com/casbin/casbin/v2/tree/v2.1.2)
- github.com/clbanning/x2j: [8252494](https://github.com/clbanning/x2j/tree/8252494)
- github.com/codahale/hdrhistogram: [3a0bb77](https://github.com/codahale/hdrhistogram/tree/3a0bb77)
- github.com/eapache/go-resiliency: [v1.1.0](https://github.com/eapache/go-resiliency/tree/v1.1.0)
- github.com/eapache/go-xerial-snappy: [776d571](https://github.com/eapache/go-xerial-snappy/tree/776d571)
- github.com/eapache/queue: [v1.1.0](https://github.com/eapache/queue/tree/v1.1.0)
- github.com/edsrzf/mmap-go: [v1.0.0](https://github.com/edsrzf/mmap-go/tree/v1.0.0)
- github.com/franela/goblin: [c9ffbef](https://github.com/franela/goblin/tree/c9ffbef)
- github.com/franela/goreq: [bcd34c9](https://github.com/franela/goreq/tree/bcd34c9)
- github.com/globalsign/mgo: [eeefdec](https://github.com/globalsign/mgo/tree/eeefdec)
- github.com/go-openapi/analysis: [v0.19.5](https://github.com/go-openapi/analysis/tree/v0.19.5)
- github.com/go-openapi/errors: [v0.19.2](https://github.com/go-openapi/errors/tree/v0.19.2)
- github.com/go-openapi/loads: [v0.19.4](https://github.com/go-openapi/loads/tree/v0.19.4)
- github.com/go-openapi/runtime: [v0.19.4](https://github.com/go-openapi/runtime/tree/v0.19.4)
- github.com/go-openapi/strfmt: [v0.19.5](https://github.com/go-openapi/strfmt/tree/v0.19.5)
- github.com/go-openapi/validate: [v0.19.8](https://github.com/go-openapi/validate/tree/v0.19.8)
- github.com/go-sql-driver/mysql: [v1.4.0](https://github.com/go-sql-driver/mysql/tree/v1.4.0)
- github.com/gobuffalo/here: [v0.6.0](https://github.com/gobuffalo/here/tree/v0.6.0)
- github.com/gogo/googleapis: [v1.1.0](https://github.com/gogo/googleapis/tree/v1.1.0)
- github.com/golang/snappy: [2e65f85](https://github.com/golang/snappy/tree/2e65f85)
- github.com/gorilla/context: [v1.1.1](https://github.com/gorilla/context/tree/v1.1.1)
- github.com/hashicorp/go-version: [v1.2.0](https://github.com/hashicorp/go-version/tree/v1.2.0)
- github.com/hudl/fargo: [v1.3.0](https://github.com/hudl/fargo/tree/v1.3.0)
- github.com/influxdata/influxdb1-client: [8bf82d3](https://github.com/influxdata/influxdb1-client/tree/8bf82d3)
- github.com/lightstep/lightstep-tracer-common/golang/gogo: [bc2310a](https://github.com/lightstep/lightstep-tracer-common/golang/gogo/tree/bc2310a)
- github.com/lightstep/lightstep-tracer-go: [v0.18.1](https://github.com/lightstep/lightstep-tracer-go/tree/v0.18.1)
- github.com/lyft/protoc-gen-validate: [v0.0.13](https://github.com/lyft/protoc-gen-validate/tree/v0.0.13)
- github.com/markbates/pkger: [v0.17.1](https://github.com/markbates/pkger/tree/v0.17.1)
- github.com/nats-io/jwt: [v0.3.2](https://github.com/nats-io/jwt/tree/v0.3.2)
- github.com/nats-io/nats-server/v2: [v2.1.2](https://github.com/nats-io/nats-server/v2/tree/v2.1.2)
- github.com/nats-io/nats.go: [v1.9.1](https://github.com/nats-io/nats.go/tree/v1.9.1)
- github.com/nats-io/nkeys: [v0.1.3](https://github.com/nats-io/nkeys/tree/v0.1.3)
- github.com/nats-io/nuid: [v1.0.1](https://github.com/nats-io/nuid/tree/v1.0.1)
- github.com/oklog/oklog: [v0.3.2](https://github.com/oklog/oklog/tree/v0.3.2)
- github.com/oklog/run: [v1.0.0](https://github.com/oklog/run/tree/v1.0.0)
- github.com/op/go-logging: [970db52](https://github.com/op/go-logging/tree/970db52)
- github.com/opentracing-contrib/go-observer: [a52f234](https://github.com/opentracing-contrib/go-observer/tree/a52f234)
- github.com/opentracing/basictracer-go: [v1.0.0](https://github.com/opentracing/basictracer-go/tree/v1.0.0)
- github.com/openzipkin-contrib/zipkin-go-opentracing: [v0.4.5](https://github.com/openzipkin-contrib/zipkin-go-opentracing/tree/v0.4.5)
- github.com/openzipkin/zipkin-go: [v0.2.2](https://github.com/openzipkin/zipkin-go/tree/v0.2.2)
- github.com/pact-foundation/pact-go: [v1.0.4](https://github.com/pact-foundation/pact-go/tree/v1.0.4)
- github.com/pborman/uuid: [v1.2.0](https://github.com/pborman/uuid/tree/v1.2.0)
- github.com/performancecopilot/speed: [v3.0.0+incompatible](https://github.com/performancecopilot/speed/tree/v3.0.0)
- github.com/pierrec/lz4: [v2.0.5+incompatible](https://github.com/pierrec/lz4/tree/v2.0.5)
- github.com/pkg/profile: [v1.2.1](https://github.com/pkg/profile/tree/v1.2.1)
- github.com/rcrowley/go-metrics: [3113b84](https://github.com/rcrowley/go-metrics/tree/3113b84)
- github.com/samuel/go-zookeeper: [2cc03de](https://github.com/samuel/go-zookeeper/tree/2cc03de)
- github.com/satori/go.uuid: [v1.2.0](https://github.com/satori/go.uuid/tree/v1.2.0)
- github.com/sony/gobreaker: [v0.4.1](https://github.com/sony/gobreaker/tree/v0.4.1)
- github.com/streadway/amqp: [edfb901](https://github.com/streadway/amqp/tree/edfb901)
- github.com/streadway/handy: [d5acb31](https://github.com/streadway/handy/tree/d5acb31)
- github.com/tidwall/pretty: [v1.0.0](https://github.com/tidwall/pretty/tree/v1.0.0)
- github.com/vektah/gqlparser: [v1.1.2](https://github.com/vektah/gqlparser/tree/v1.1.2)
- go.mongodb.org/mongo-driver: v1.1.2
- go.uber.org/tools: 2cfd321
- sourcegraph.com/sourcegraph/appdash: ebfcffb
