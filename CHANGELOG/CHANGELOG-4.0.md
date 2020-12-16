# Release notes for v4.0.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v3.0.0

## Breaking Changes

- Update to client/v4 ([#451](https://github.com/kubernetes-csi/external-snapshotter/pull/451), [@xing-yang](https://github.com/xing-yang))

- Update snapshot-controller, CSI snapshotter sidecar, and validation webhook based on V1 APIs. This requires V1 snapshot APIs to be installed. ([#425](https://github.com/kubernetes-csi/external-snapshotter/pull/425), [@xing-yang](https://github.com/xing-yang))

### Deprecations

#### CSI Snapshotter Sidecar

- `metrics-address` flag is deprecated and replaced by `http-endpoint`, which enables handlers from both metrics manager and leader election health check. ([#439](https://github.com/kubernetes-csi/external-snapshotter/pull/439), [@xing-yang](https://github.com/xing-yang))

### Feature

#### API Changes

- Move Snapshot APIs to V1. ([#419](https://github.com/kubernetes-csi/external-snapshotter/pull/419), [@xing-yang](https://github.com/xing-yang))

#### CSI Snapshotter Sidecar

- Add `process_start_time_seconds` metric. ([#387](https://github.com/kubernetes-csi/external-snapshotter/pull/387), [@Jiawei0227](https://github.com/Jiawei0227))
- The CSI snapshotter has a new `--extra-create-metadata` flag.
  When set to true, the name and namespace of the source VolumeSnapshot, and the name of the source  VolumeSnapshotContent will be passed as parameters to the CSI driver in CreateSnapshotRequest. ([#375](https://github.com/kubernetes-csi/external-snapshotter/pull/375), [@chrishenzie](https://github.com/chrishenzie))
- Added leader election health check at the metrics port + path "/healthz/leader-election".
  - `metrics-address` flag is deprecated and replaced by `http-endpoint`, which enables handlers from both metrics manager and leader election health check. ([#439](https://github.com/kubernetes-csi/external-snapshotter/pull/439), [@xing-yang](https://github.com/xing-yang))

#### Snapshot Controller

- The `http-endpoint` and `metrics-path` flags have been added to support metrics for the snapshot-controller. ([#409](https://github.com/kubernetes-csi/external-snapshotter/pull/409), [@ggriffiths](https://github.com/ggriffiths))

### Bug or Regression

#### Snapshot Controller

- Bug fix to allow creation of snapshot content if pvc finalizer exists, even if pvc is marked for deletion. ([#413](https://github.com/kubernetes-csi/external-snapshotter/pull/413), [@RaunakShah](https://github.com/RaunakShah))
- Fixed crash of snapshot-controller when source PVC of a snapshot to take does not exist. ([#381](https://github.com/kubernetes-csi/external-snapshotter/pull/381), [@jsafrane](https://github.com/jsafrane))

#### Validation Webhook

- The validation-webhook server will now watch for changes to --tls-cert-file and --tls-private-key-file and update the server's TLS config accordingly. ([#429](https://github.com/kubernetes-csi/external-snapshotter/pull/429), [@ggriffiths](https://github.com/ggriffiths))

### Other (Cleanup or Flake)

- Increased the default re-sync period to 15 minutes for the snapshotter sidecar and snapshot controller. ([#447](https://github.com/kubernetes-csi/external-snapshotter/pull/447), [@ggriffiths](https://github.com/ggriffiths))
- klog/v2 is used for logging. ([#439](https://github.com/kubernetes-csi/external-snapshotter/pull/439), [@xing-yang](https://github.com/xing-yang))

## Dependencies

### Added
- cloud.google.com/go/firestore: v1.1.0
- github.com/Azure/go-autorest: [v14.2.0+incompatible](https://github.com/Azure/go-autorest/tree/v14.2.0)
- github.com/Knetic/govaluate: [9aa4983](https://github.com/Knetic/govaluate/tree/9aa4983)
- github.com/Shopify/sarama: [v1.19.0](https://github.com/Shopify/sarama/tree/v1.19.0)
- github.com/Shopify/toxiproxy: [v2.1.4+incompatible](https://github.com/Shopify/toxiproxy/tree/v2.1.4)
- github.com/VividCortex/gohistogram: [v1.0.0](https://github.com/VividCortex/gohistogram/tree/v1.0.0)
- github.com/afex/hystrix-go: [fa1af6a](https://github.com/afex/hystrix-go/tree/fa1af6a)
- github.com/apache/thrift: [v0.13.0](https://github.com/apache/thrift/tree/v0.13.0)
- github.com/armon/go-metrics: [f0300d1](https://github.com/armon/go-metrics/tree/f0300d1)
- github.com/armon/go-radix: [7fddfc3](https://github.com/armon/go-radix/tree/7fddfc3)
- github.com/aryann/difflib: [e206f87](https://github.com/aryann/difflib/tree/e206f87)
- github.com/aws/aws-lambda-go: [v1.13.3](https://github.com/aws/aws-lambda-go/tree/v1.13.3)
- github.com/aws/aws-sdk-go-v2: [v0.18.0](https://github.com/aws/aws-sdk-go-v2/tree/v0.18.0)
- github.com/bketelsen/crypt: [5cbc8cc](https://github.com/bketelsen/crypt/tree/5cbc8cc)
- github.com/casbin/casbin/v2: [v2.1.2](https://github.com/casbin/casbin/v2/tree/v2.1.2)
- github.com/clbanning/x2j: [8252494](https://github.com/clbanning/x2j/tree/8252494)
- github.com/codahale/hdrhistogram: [3a0bb77](https://github.com/codahale/hdrhistogram/tree/3a0bb77)
- github.com/eapache/go-resiliency: [v1.1.0](https://github.com/eapache/go-resiliency/tree/v1.1.0)
- github.com/eapache/go-xerial-snappy: [776d571](https://github.com/eapache/go-xerial-snappy/tree/776d571)
- github.com/eapache/queue: [v1.1.0](https://github.com/eapache/queue/tree/v1.1.0)
- github.com/edsrzf/mmap-go: [v1.0.0](https://github.com/edsrzf/mmap-go/tree/v1.0.0)
- github.com/form3tech-oss/jwt-go: [v3.2.2+incompatible](https://github.com/form3tech-oss/jwt-go/tree/v3.2.2)
- github.com/franela/goblin: [c9ffbef](https://github.com/franela/goblin/tree/c9ffbef)
- github.com/franela/goreq: [bcd34c9](https://github.com/franela/goreq/tree/bcd34c9)
- github.com/fvbommel/sortorder: [v1.0.1](https://github.com/fvbommel/sortorder/tree/v1.0.1)
- github.com/go-gl/glfw: [e6da0ac](https://github.com/go-gl/glfw/tree/e6da0ac)
- github.com/go-sql-driver/mysql: [v1.4.0](https://github.com/go-sql-driver/mysql/tree/v1.4.0)
- github.com/gogo/googleapis: [v1.1.0](https://github.com/gogo/googleapis/tree/v1.1.0)
- github.com/golang/snappy: [2e65f85](https://github.com/golang/snappy/tree/2e65f85)
- github.com/google/martian/v3: [v3.0.0](https://github.com/google/martian/v3/tree/v3.0.0)
- github.com/hashicorp/consul/api: [v1.3.0](https://github.com/hashicorp/consul/api/tree/v1.3.0)
- github.com/hashicorp/consul/sdk: [v0.3.0](https://github.com/hashicorp/consul/sdk/tree/v0.3.0)
- github.com/hashicorp/errwrap: [v1.0.0](https://github.com/hashicorp/errwrap/tree/v1.0.0)
- github.com/hashicorp/go-cleanhttp: [v0.5.1](https://github.com/hashicorp/go-cleanhttp/tree/v0.5.1)
- github.com/hashicorp/go-immutable-radix: [v1.0.0](https://github.com/hashicorp/go-immutable-radix/tree/v1.0.0)
- github.com/hashicorp/go-msgpack: [v0.5.3](https://github.com/hashicorp/go-msgpack/tree/v0.5.3)
- github.com/hashicorp/go-multierror: [v1.0.0](https://github.com/hashicorp/go-multierror/tree/v1.0.0)
- github.com/hashicorp/go-rootcerts: [v1.0.0](https://github.com/hashicorp/go-rootcerts/tree/v1.0.0)
- github.com/hashicorp/go-sockaddr: [v1.0.0](https://github.com/hashicorp/go-sockaddr/tree/v1.0.0)
- github.com/hashicorp/go-uuid: [v1.0.1](https://github.com/hashicorp/go-uuid/tree/v1.0.1)
- github.com/hashicorp/go-version: [v1.2.0](https://github.com/hashicorp/go-version/tree/v1.2.0)
- github.com/hashicorp/go.net: [v0.0.1](https://github.com/hashicorp/go.net/tree/v0.0.1)
- github.com/hashicorp/logutils: [v1.0.0](https://github.com/hashicorp/logutils/tree/v1.0.0)
- github.com/hashicorp/mdns: [v1.0.0](https://github.com/hashicorp/mdns/tree/v1.0.0)
- github.com/hashicorp/memberlist: [v0.1.3](https://github.com/hashicorp/memberlist/tree/v0.1.3)
- github.com/hashicorp/serf: [v0.8.2](https://github.com/hashicorp/serf/tree/v0.8.2)
- github.com/hudl/fargo: [v1.3.0](https://github.com/hudl/fargo/tree/v1.3.0)
- github.com/influxdata/influxdb1-client: [8bf82d3](https://github.com/influxdata/influxdb1-client/tree/8bf82d3)
- github.com/jmespath/go-jmespath/internal/testify: [v1.5.1](https://github.com/jmespath/go-jmespath/internal/testify/tree/v1.5.1)
- github.com/jpillora/backoff: [v1.0.0](https://github.com/jpillora/backoff/tree/v1.0.0)
- github.com/kubernetes-csi/csi-test/v4: [v4.0.2](https://github.com/kubernetes-csi/csi-test/v4/tree/v4.0.2)
- github.com/lightstep/lightstep-tracer-common/golang/gogo: [bc2310a](https://github.com/lightstep/lightstep-tracer-common/golang/gogo/tree/bc2310a)
- github.com/lightstep/lightstep-tracer-go: [v0.18.1](https://github.com/lightstep/lightstep-tracer-go/tree/v0.18.1)
- github.com/lyft/protoc-gen-validate: [v0.0.13](https://github.com/lyft/protoc-gen-validate/tree/v0.0.13)
- github.com/mitchellh/cli: [v1.0.0](https://github.com/mitchellh/cli/tree/v1.0.0)
- github.com/mitchellh/go-testing-interface: [v1.0.0](https://github.com/mitchellh/go-testing-interface/tree/v1.0.0)
- github.com/mitchellh/gox: [v0.4.0](https://github.com/mitchellh/gox/tree/v0.4.0)
- github.com/mitchellh/iochan: [v1.0.0](https://github.com/mitchellh/iochan/tree/v1.0.0)
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
- github.com/opentracing/opentracing-go: [v1.1.0](https://github.com/opentracing/opentracing-go/tree/v1.1.0)
- github.com/openzipkin-contrib/zipkin-go-opentracing: [v0.4.5](https://github.com/openzipkin-contrib/zipkin-go-opentracing/tree/v0.4.5)
- github.com/openzipkin/zipkin-go: [v0.2.2](https://github.com/openzipkin/zipkin-go/tree/v0.2.2)
- github.com/pact-foundation/pact-go: [v1.0.4](https://github.com/pact-foundation/pact-go/tree/v1.0.4)
- github.com/pascaldekloe/goe: [57f6aae](https://github.com/pascaldekloe/goe/tree/57f6aae)
- github.com/performancecopilot/speed: [v3.0.0+incompatible](https://github.com/performancecopilot/speed/tree/v3.0.0)
- github.com/pierrec/lz4: [v2.0.5+incompatible](https://github.com/pierrec/lz4/tree/v2.0.5)
- github.com/pkg/profile: [v1.2.1](https://github.com/pkg/profile/tree/v1.2.1)
- github.com/posener/complete: [v1.1.1](https://github.com/posener/complete/tree/v1.1.1)
- github.com/rcrowley/go-metrics: [3113b84](https://github.com/rcrowley/go-metrics/tree/3113b84)
- github.com/robertkrimen/otto: [c382bd3](https://github.com/robertkrimen/otto/tree/c382bd3)
- github.com/ryanuber/columnize: [9b3edd6](https://github.com/ryanuber/columnize/tree/9b3edd6)
- github.com/samuel/go-zookeeper: [2cc03de](https://github.com/samuel/go-zookeeper/tree/2cc03de)
- github.com/sean-/seed: [e2103e2](https://github.com/sean-/seed/tree/e2103e2)
- github.com/sony/gobreaker: [v0.4.1](https://github.com/sony/gobreaker/tree/v0.4.1)
- github.com/stoewer/go-strcase: [v1.2.0](https://github.com/stoewer/go-strcase/tree/v1.2.0)
- github.com/streadway/amqp: [edfb901](https://github.com/streadway/amqp/tree/edfb901)
- github.com/streadway/handy: [d5acb31](https://github.com/streadway/handy/tree/d5acb31)
- github.com/subosito/gotenv: [v1.2.0](https://github.com/subosito/gotenv/tree/v1.2.0)
- github.com/willf/bitset: [d5bec33](https://github.com/willf/bitset/tree/d5bec33)
- go.uber.org/tools: 2cfd321
- golang.org/x/term: 2321bbc
- gopkg.in/ini.v1: v1.51.0
- gopkg.in/sourcemap.v1: v1.0.5
- gopkg.in/yaml.v3: eeeca48
- k8s.io/component-helpers: v0.20.0
- k8s.io/controller-manager: v0.20.0
- k8s.io/mount-utils: v0.20.0
- sourcegraph.com/sourcegraph/appdash: ebfcffb

### Changed
- cloud.google.com/go/bigquery: v1.0.1 → v1.8.0
- cloud.google.com/go/datastore: v1.0.0 → v1.1.0
- cloud.google.com/go/pubsub: v1.0.1 → v1.3.1
- cloud.google.com/go/storage: v1.0.0 → v1.10.0
- cloud.google.com/go: v0.51.0 → v0.65.0
- github.com/Azure/go-autorest/autorest/adal: [v0.8.2 → v0.9.5](https://github.com/Azure/go-autorest/autorest/adal/compare/v0.8.2...v0.9.5)
- github.com/Azure/go-autorest/autorest/date: [v0.2.0 → v0.3.0](https://github.com/Azure/go-autorest/autorest/date/compare/v0.2.0...v0.3.0)
- github.com/Azure/go-autorest/autorest/mocks: [v0.3.0 → v0.4.1](https://github.com/Azure/go-autorest/autorest/mocks/compare/v0.3.0...v0.4.1)
- github.com/Azure/go-autorest/autorest: [v0.9.6 → v0.11.1](https://github.com/Azure/go-autorest/autorest/compare/v0.9.6...v0.11.1)
- github.com/Azure/go-autorest/logger: [v0.1.0 → v0.2.0](https://github.com/Azure/go-autorest/logger/compare/v0.1.0...v0.2.0)
- github.com/Azure/go-autorest/tracing: [v0.5.0 → v0.6.0](https://github.com/Azure/go-autorest/tracing/compare/v0.5.0...v0.6.0)
- github.com/Microsoft/go-winio: [fc70bd9 → v0.4.15](https://github.com/Microsoft/go-winio/compare/fc70bd9...v0.4.15)
- github.com/alecthomas/units: [c3de453 → f65c72e](https://github.com/alecthomas/units/compare/c3de453...f65c72e)
- github.com/aws/aws-sdk-go: [v1.28.2 → v1.35.24](https://github.com/aws/aws-sdk-go/compare/v1.28.2...v1.35.24)
- github.com/blang/semver: [v3.5.0+incompatible → v3.5.1+incompatible](https://github.com/blang/semver/compare/v3.5.0...v3.5.1)
- github.com/cenkalti/backoff: [v2.1.1+incompatible → v2.2.1+incompatible](https://github.com/cenkalti/backoff/compare/v2.1.1...v2.2.1)
- github.com/checkpoint-restore/go-criu/v4: [v4.0.2 → v4.1.0](https://github.com/checkpoint-restore/go-criu/v4/compare/v4.0.2...v4.1.0)
- github.com/cncf/udpa/go: [269d4d4 → efcf912](https://github.com/cncf/udpa/go/compare/269d4d4...efcf912)
- github.com/container-storage-interface/spec: [v1.2.0 → v1.3.0](https://github.com/container-storage-interface/spec/compare/v1.2.0...v1.3.0)
- github.com/containerd/containerd: [v1.3.3 → v1.4.1](https://github.com/containerd/containerd/compare/v1.3.3...v1.4.1)
- github.com/containerd/ttrpc: [v1.0.0 → v1.0.2](https://github.com/containerd/ttrpc/compare/v1.0.0...v1.0.2)
- github.com/containerd/typeurl: [v1.0.0 → v1.0.1](https://github.com/containerd/typeurl/compare/v1.0.0...v1.0.1)
- github.com/coreos/etcd: [v3.3.10+incompatible → v3.3.13+incompatible](https://github.com/coreos/etcd/compare/v3.3.10...v3.3.13)
- github.com/docker/docker: [aa6a989 → bd33bbf](https://github.com/docker/docker/compare/aa6a989...bd33bbf)
- github.com/envoyproxy/go-control-plane: [v0.9.4 → v0.9.7](https://github.com/envoyproxy/go-control-plane/compare/v0.9.4...v0.9.7)
- github.com/go-gl/glfw/v3.3/glfw: [12ad95a → 6f7a984](https://github.com/go-gl/glfw/v3.3/glfw/compare/12ad95a...6f7a984)
- github.com/go-kit/kit: [v0.9.0 → v0.10.0](https://github.com/go-kit/kit/compare/v0.9.0...v0.10.0)
- github.com/go-logfmt/logfmt: [v0.4.0 → v0.5.0](https://github.com/go-logfmt/logfmt/compare/v0.4.0...v0.5.0)
- github.com/go-logr/logr: [v0.2.0 → v0.3.0](https://github.com/go-logr/logr/compare/v0.2.0...v0.3.0)
- github.com/golang/groupcache: [215e871 → 8c9f03a](https://github.com/golang/groupcache/compare/215e871...8c9f03a)
- github.com/golang/mock: [v1.4.3 → v1.4.4](https://github.com/golang/mock/compare/v1.4.3...v1.4.4)
- github.com/golang/protobuf: [v1.4.2 → v1.4.3](https://github.com/golang/protobuf/compare/v1.4.2...v1.4.3)
- github.com/google/cadvisor: [v0.37.0 → v0.38.5](https://github.com/google/cadvisor/compare/v0.37.0...v0.38.5)
- github.com/google/go-cmp: [v0.4.0 → v0.5.4](https://github.com/google/go-cmp/compare/v0.4.0...v0.5.4)
- github.com/google/gofuzz: [v1.1.0 → v1.2.0](https://github.com/google/gofuzz/compare/v1.1.0...v1.2.0)
- github.com/google/pprof: [d4f498a → 1a94d86](https://github.com/google/pprof/compare/d4f498a...1a94d86)
- github.com/google/uuid: [v1.1.1 → v1.1.2](https://github.com/google/uuid/compare/v1.1.1...v1.1.2)
- github.com/googleapis/gnostic: [v0.4.1 → v0.5.3](https://github.com/googleapis/gnostic/compare/v0.4.1...v0.5.3)
- github.com/gorilla/mux: [v1.7.3 → v1.8.0](https://github.com/gorilla/mux/compare/v1.7.3...v1.8.0)
- github.com/gorilla/websocket: [v1.4.0 → v1.4.2](https://github.com/gorilla/websocket/compare/v1.4.0...v1.4.2)
- github.com/hashicorp/golang-lru: [v0.5.1 → v0.5.4](https://github.com/hashicorp/golang-lru/compare/v0.5.1...v0.5.4)
- github.com/imdario/mergo: [v0.3.9 → v0.3.11](https://github.com/imdario/mergo/compare/v0.3.9...v0.3.11)
- github.com/jmespath/go-jmespath: [c2b33e8 → v0.4.0](https://github.com/jmespath/go-jmespath/compare/c2b33e8...v0.4.0)
- github.com/julienschmidt/httprouter: [v1.2.0 → v1.3.0](https://github.com/julienschmidt/httprouter/compare/v1.2.0...v1.3.0)
- github.com/karrick/godirwalk: [v1.7.5 → v1.16.1](https://github.com/karrick/godirwalk/compare/v1.7.5...v1.16.1)
- github.com/kubernetes-csi/csi-lib-utils: [v0.7.0 → v0.9.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.7.0...v0.9.0)
- github.com/mwitkow/go-conntrack: [cc309e4 → 2f06839](https://github.com/mwitkow/go-conntrack/compare/cc309e4...2f06839)
- github.com/onsi/gomega: [v1.7.0 → v1.7.1](https://github.com/onsi/gomega/compare/v1.7.0...v1.7.1)
- github.com/opencontainers/go-digest: [v1.0.0-rc1 → v1.0.0](https://github.com/opencontainers/go-digest/compare/v1.0.0-rc1...v1.0.0)
- github.com/opencontainers/runc: [819fcc6 → v1.0.0-rc92](https://github.com/opencontainers/runc/compare/819fcc6...v1.0.0-rc92)
- github.com/opencontainers/runtime-spec: [237cc4f → 4d89ac9](https://github.com/opencontainers/runtime-spec/compare/237cc4f...4d89ac9)
- github.com/opencontainers/selinux: [v1.5.2 → v1.6.0](https://github.com/opencontainers/selinux/compare/v1.5.2...v1.6.0)
- github.com/prometheus/client_golang: [v1.7.1 → v1.8.0](https://github.com/prometheus/client_golang/compare/v1.7.1...v1.8.0)
- github.com/prometheus/common: [v0.10.0 → v0.15.0](https://github.com/prometheus/common/compare/v0.10.0...v0.15.0)
- github.com/prometheus/procfs: [v0.1.3 → v0.2.0](https://github.com/prometheus/procfs/compare/v0.1.3...v0.2.0)
- github.com/quobyte/api: [v0.1.2 → v0.1.8](https://github.com/quobyte/api/compare/v0.1.2...v0.1.8)
- github.com/spf13/cobra: [v1.0.0 → v1.1.1](https://github.com/spf13/cobra/compare/v1.0.0...v1.1.1)
- github.com/spf13/viper: [v1.4.0 → v1.7.0](https://github.com/spf13/viper/compare/v1.4.0...v1.7.0)
- github.com/storageos/go-api: [343b3ef → v2.2.0+incompatible](https://github.com/storageos/go-api/compare/343b3ef...v2.2.0)
- github.com/stretchr/testify: [v1.4.0 → v1.6.1](https://github.com/stretchr/testify/compare/v1.4.0...v1.6.1)
- github.com/vishvananda/netns: [52d707b → db3c7e5](https://github.com/vishvananda/netns/compare/52d707b...db3c7e5)
- github.com/yuin/goldmark: [v1.1.27 → v1.1.32](https://github.com/yuin/goldmark/compare/v1.1.27...v1.1.32)
- go.etcd.io/etcd: 17cef6e → dd1b699
- go.opencensus.io: v0.22.2 → v0.22.4
- go.uber.org/atomic: v1.4.0 → v1.5.0
- go.uber.org/multierr: v1.1.0 → v1.3.0
- go.uber.org/zap: v1.10.0 → v1.13.0
- golang.org/x/crypto: 75b2880 → 5f87f34
- golang.org/x/exp: da58074 → 6cc2880
- golang.org/x/lint: fdd1cda → 738671d
- golang.org/x/net: ab34263 → ac852fb
- golang.org/x/oauth2: bf48bf1 → 08078c5
- golang.org/x/sync: cd5d95a → 6e8e738
- golang.org/x/sys: ed371f2 → aec9a39
- golang.org/x/text: v0.3.3 → v0.3.4
- golang.org/x/time: 89c76fb → 7e3f01d
- golang.org/x/tools: c1934b7 → b303f43
- golang.org/x/xerrors: 9bdfabe → 5ec99f8
- google.golang.org/api: v0.15.1 → v0.30.0
- google.golang.org/appengine: v1.6.5 → v1.6.7
- google.golang.org/genproto: cb27e3a → 40ec1c2
- google.golang.org/grpc: v1.28.0 → v1.34.0
- google.golang.org/protobuf: v1.24.0 → v1.25.0
- gopkg.in/gcfg.v1: v1.2.0 → v1.2.3
- gopkg.in/warnings.v0: v0.1.1 → v0.1.2
- gopkg.in/yaml.v2: v2.2.8 → v2.4.0
- honnef.co/go/tools: v0.0.1-2019.2.3 → v0.0.1-2020.1.4
- k8s.io/api: v0.19.0 → v0.20.0
- k8s.io/apiextensions-apiserver: v0.19.0 → v0.20.0
- k8s.io/apimachinery: v0.19.0 → v0.20.0
- k8s.io/apiserver: v0.19.0 → v0.20.0
- k8s.io/cli-runtime: v0.19.0 → v0.20.0
- k8s.io/client-go: v0.19.0 → v0.20.0
- k8s.io/cloud-provider: v0.19.0 → v0.20.0
- k8s.io/cluster-bootstrap: v0.19.0 → v0.20.0
- k8s.io/code-generator: v0.19.0 → v0.20.0
- k8s.io/component-base: v0.19.0 → v0.20.0
- k8s.io/cri-api: v0.19.0 → v0.20.0
- k8s.io/csi-translation-lib: v0.19.0 → v0.20.0
- k8s.io/gengo: 8167cfd → 83324d8
- k8s.io/klog/v2: v2.2.0 → v2.4.0
- k8s.io/kube-aggregator: v0.19.0 → v0.20.0
- k8s.io/kube-controller-manager: v0.19.0 → v0.20.0
- k8s.io/kube-openapi: 6aeccd4 → d219536
- k8s.io/kube-proxy: v0.19.0 → v0.20.0
- k8s.io/kube-scheduler: v0.19.0 → v0.20.0
- k8s.io/kubectl: v0.19.0 → v0.20.0
- k8s.io/kubelet: v0.19.0 → v0.20.0
- k8s.io/kubernetes: v1.19.0 → v1.20.0
- k8s.io/legacy-cloud-providers: v0.19.0 → v0.20.0
- k8s.io/metrics: v0.19.0 → v0.20.0
- k8s.io/sample-apiserver: v0.19.0 → v0.20.0
- k8s.io/system-validators: v1.1.2 → v1.2.0
- k8s.io/utils: d5654de → 67b214c
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.9 → v0.0.14
- sigs.k8s.io/structured-merge-diff/v4: v4.0.1 → v4.0.2

### Removed
- github.com/go-ini/ini: [v1.9.0](https://github.com/go-ini/ini/tree/v1.9.0)
- github.com/kubernetes-csi/csi-test: [v2.0.0+incompatible](https://github.com/kubernetes-csi/csi-test/tree/v2.0.0)
- github.com/xlab/handysort: [fb3537e](https://github.com/xlab/handysort/tree/fb3537e)
- vbom.ml/util: db5cfe1
