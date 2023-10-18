# Release notes for v6.1.1

[Documentation](https://kubernetes-csi.github.io)

## Changes by Kind

### Other (Cleanup or Flake)

- Update deployment tags to v6.1.0 (#766, @RaunakShah)

### Uncategorized

- CVE fixes: CVE-2023-44487,  CVE-2023-3978 (#929, @dannawang0221)

## Dependencies

### Added
_Nothing has changed._

### Changed
- golang.org/x/crypto: 3147a52 → v0.14.0
- golang.org/x/mod: 86c51ed → v0.8.0
- golang.org/x/net: bea034e → v0.17.0
- golang.org/x/sys: fb04ddd → v0.13.0
- golang.org/x/term: 03fcf44 → v0.13.0
- golang.org/x/text: v0.3.7 → v0.13.0
- golang.org/x/tools: v0.1.12 → v0.6.0

### Removed
_Nothing has changed._

# Release notes for v6.1.0

# Changelog since v6.0.0

## Changes by Kind

### API Change

- Add VolumeSnapshot v1beta1 manifests back. VolumeSnapshot v1beta1 APIs are no longer served.
  Action Item: Please update to VolumeSnapshot v1 APIs as soon as possible. ([#718](https://github.com/kubernetes-csi/external-snapshotter/pull/718),[ @RaunakShah](https://github.com/RaunakShah))

### Other (Cleanup or Flake)

- Remove v1beta1 from admission config template ([#734](https://github.com/kubernetes-csi/external-snapshotter/pull/734), [@RaunakShah](https://github.com/RaunakShah))
- Upgrade kube dependencies and snapshotter client kube deps to v0.25.2 ([#765](https://github.com/kubernetes-csi/external-snapshotter/pull/765), [@RaunakShah](https://github.com/RaunakShah))
- Update to go 1.18. ([#762](https://github.com/kubernetes-csi/external-snapshotter/pull/762), [@humblec](https://github.com/humblec))
- Update kubernetes module dependencies to v1.25 ([#753](https://github.com/kubernetes-csi/external-snapshotter/pull/753), [@humblec](https://github.com/humblec))
- Update the external-snapshotter client dep to v6.1.0 ([#768](https://github.com/kubernetes-csi/external-snapshotter/pull/768), [@xing-yang](https://github.com/xing-yang))

## Dependencies

### Added
- github.com/buger/jsonparser: [v1.1.1](https://github.com/buger/jsonparser/tree/v1.1.1)
- github.com/emicklei/go-restful/v3: [v3.9.0](https://github.com/emicklei/go-restful/v3/tree/v3.9.0)
- github.com/flowstack/go-jsonschema: [v0.1.1](https://github.com/flowstack/go-jsonschema/tree/v0.1.1)
- github.com/go-task/slim-sprig: [348f09d](https://github.com/go-task/slim-sprig/tree/348f09d)
- github.com/golang-jwt/jwt/v4: [v4.2.0](https://github.com/golang-jwt/jwt/v4/tree/v4.2.0)
- github.com/golang/snappy: [v0.0.3](https://github.com/golang/snappy/tree/v0.0.3)
- github.com/onsi/ginkgo/v2: [v2.1.6](https://github.com/onsi/ginkgo/v2/tree/v2.1.6)
- github.com/xeipuuv/gojsonpointer: [4e3ac27](https://github.com/xeipuuv/gojsonpointer/tree/4e3ac27)
- github.com/xeipuuv/gojsonreference: [bd5ef7b](https://github.com/xeipuuv/gojsonreference/tree/bd5ef7b)
- github.com/xeipuuv/gojsonschema: [v1.2.0](https://github.com/xeipuuv/gojsonschema/tree/v1.2.0)
- go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful: v0.20.0
- google.golang.org/grpc/cmd/protoc-gen-go-grpc: v1.1.0

### Changed
- bitbucket.org/bertimus9/systemstat: 0eeff89 → v0.5.0
- cloud.google.com/go: v0.81.0 → v0.97.0
- dmitri.shuralyov.com/gpu/mtl: 28db891 → 666a987
- github.com/Azure/go-autorest/autorest/adal: [v0.9.13 → v0.9.20](https://github.com/Azure/go-autorest/autorest/adal/compare/v0.9.13...v0.9.20)
- github.com/Azure/go-autorest/autorest/mocks: [v0.4.1 → v0.4.2](https://github.com/Azure/go-autorest/autorest/mocks/compare/v0.4.1...v0.4.2)
- github.com/Azure/go-autorest/autorest: [v0.11.18 → v0.11.27](https://github.com/Azure/go-autorest/autorest/compare/v0.11.18...v0.11.27)
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [ea6160c → f118173](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/compare/ea6160c...f118173)
- github.com/MakeNowJust/heredoc: [bb23615 → v1.0.0](https://github.com/MakeNowJust/heredoc/compare/bb23615...v1.0.0)
- github.com/antlr/antlr4/runtime/Go/antlr: [b48c857 → f25a4f6](https://github.com/antlr/antlr4/runtime/Go/antlr/compare/b48c857...f25a4f6)
- github.com/chai2010/gettext-go: [c6fed77 → v1.0.2](https://github.com/chai2010/gettext-go/compare/c6fed77...v1.0.2)
- github.com/checkpoint-restore/go-criu/v5: [v5.0.0 → v5.3.0](https://github.com/checkpoint-restore/go-criu/v5/compare/v5.0.0...v5.3.0)
- github.com/cilium/ebpf: [v0.6.2 → v0.7.0](https://github.com/cilium/ebpf/compare/v0.6.2...v0.7.0)
- github.com/cncf/udpa/go: [5459f2c → 04548b0](https://github.com/cncf/udpa/go/compare/5459f2c...04548b0)
- github.com/cncf/xds/go: [fbca930 → cb28da3](https://github.com/cncf/xds/go/compare/fbca930...cb28da3)
- github.com/container-storage-interface/spec: [v1.5.0 → v1.6.0](https://github.com/container-storage-interface/spec/compare/v1.5.0...v1.6.0)
- github.com/containerd/console: [v1.0.2 → v1.0.3](https://github.com/containerd/console/compare/v1.0.2...v1.0.3)
- github.com/coredns/corefile-migration: [v1.0.14 → v1.0.17](https://github.com/coredns/corefile-migration/compare/v1.0.14...v1.0.17)
- github.com/cyphar/filepath-securejoin: [v0.2.2 → v0.2.3](https://github.com/cyphar/filepath-securejoin/compare/v0.2.2...v0.2.3)
- github.com/daviddengcn/go-colortext: [511bcaf → v1.0.0](https://github.com/daviddengcn/go-colortext/compare/511bcaf...v1.0.0)
- github.com/envoyproxy/go-control-plane: [63b5d3c → 49ff273](https://github.com/envoyproxy/go-control-plane/compare/63b5d3c...49ff273)
- github.com/go-logr/logr: [v1.2.0 → v1.2.3](https://github.com/go-logr/logr/compare/v1.2.0...v1.2.3)
- github.com/go-logr/zapr: [v1.2.0 → v1.2.3](https://github.com/go-logr/zapr/compare/v1.2.0...v1.2.3)
- github.com/go-openapi/jsonreference: [v0.19.5 → v0.20.0](https://github.com/go-openapi/jsonreference/compare/v0.19.5...v0.20.0)
- github.com/go-openapi/swag: [v0.19.14 → v0.22.3](https://github.com/go-openapi/swag/compare/v0.19.14...v0.22.3)
- github.com/godbus/dbus/v5: [v5.0.4 → v5.0.6](https://github.com/godbus/dbus/v5/compare/v5.0.4...v5.0.6)
- github.com/golang/glog: [v1.0.0 → 23def4e](https://github.com/golang/glog/compare/v1.0.0...23def4e)
- github.com/google/cadvisor: [v0.43.0 → v0.45.0](https://github.com/google/cadvisor/compare/v0.43.0...v0.45.0)
- github.com/google/cel-go: [v0.10.1 → v0.12.5](https://github.com/google/cel-go/compare/v0.10.1...v0.12.5)
- github.com/google/gnostic: [v0.5.7-v3refs → v0.6.9](https://github.com/google/gnostic/compare/v0.5.7-v3refs...v0.6.9)
- github.com/google/go-cmp: [v0.5.6 → v0.5.8](https://github.com/google/go-cmp/compare/v0.5.6...v0.5.8)
- github.com/google/martian/v3: [v3.1.0 → v3.2.1](https://github.com/google/martian/v3/compare/v3.1.0...v3.2.1)
- github.com/google/pprof: [cbba55b → 4bb14d4](https://github.com/google/pprof/compare/cbba55b...4bb14d4)
- github.com/googleapis/gax-go/v2: [v2.0.5 → v2.1.1](https://github.com/googleapis/gax-go/v2/compare/v2.0.5...v2.1.1)
- github.com/kr/pretty: [v0.2.1 → v0.2.0](https://github.com/kr/pretty/compare/v0.2.1...v0.2.0)
- github.com/mailru/easyjson: [v0.7.6 → v0.7.7](https://github.com/mailru/easyjson/compare/v0.7.6...v0.7.7)
- github.com/moby/sys/mountinfo: [v0.4.1 → v0.6.0](https://github.com/moby/sys/mountinfo/compare/v0.4.1...v0.6.0)
- github.com/onsi/ginkgo: [v1.16.5 → v1.16.4](https://github.com/onsi/ginkgo/compare/v1.16.5...v1.16.4)
- github.com/onsi/gomega: [v1.17.0 → v1.20.1](https://github.com/onsi/gomega/compare/v1.17.0...v1.20.1)
- github.com/opencontainers/runc: [v1.0.2 → v1.1.3](https://github.com/opencontainers/runc/compare/v1.0.2...v1.1.3)
- github.com/opencontainers/selinux: [v1.8.2 → v1.10.0](https://github.com/opencontainers/selinux/compare/v1.8.2...v1.10.0)
- github.com/pquerna/cachecontrol: [0dec1b3 → v0.1.0](https://github.com/pquerna/cachecontrol/compare/0dec1b3...v0.1.0)
- github.com/seccomp/libseccomp-golang: [v0.9.1 → f33da4d](https://github.com/seccomp/libseccomp-golang/compare/v0.9.1...f33da4d)
- github.com/spf13/afero: [v1.6.0 → v1.2.2](https://github.com/spf13/afero/compare/v1.6.0...v1.2.2)
- github.com/stretchr/testify: [v1.7.0 → v1.8.0](https://github.com/stretchr/testify/compare/v1.7.0...v1.8.0)
- github.com/xlab/treeprint: [a009c39 → v1.1.0](https://github.com/xlab/treeprint/compare/a009c39...v1.1.0)
- github.com/yuin/goldmark: [v1.4.1 → v1.4.13](https://github.com/yuin/goldmark/compare/v1.4.1...v1.4.13)
- go.etcd.io/etcd/api/v3: v3.5.1 → v3.5.4
- go.etcd.io/etcd/client/pkg/v3: v3.5.1 → v3.5.4
- go.etcd.io/etcd/client/v2: v2.305.0 → v2.305.4
- go.etcd.io/etcd/client/v3: v3.5.1 → v3.5.4
- go.etcd.io/etcd/pkg/v3: v3.5.0 → v3.5.4
- go.etcd.io/etcd/raft/v3: v3.5.0 → v3.5.4
- go.etcd.io/etcd/server/v3: v3.5.0 → v3.5.4
- golang.org/x/crypto: 8634188 → 3147a52
- golang.org/x/mobile: e6ae53a → d2bd2a2
- golang.org/x/mod: 9b9b3d8 → 86c51ed
- golang.org/x/net: cd36cc0 → bea034e
- golang.org/x/sync: 036812b → 886fb93
- golang.org/x/sys: 3681064 → fb04ddd
- golang.org/x/tools: 897bd77 → v0.1.12
- google.golang.org/api: v0.46.0 → v0.60.0
- google.golang.org/genproto: 42d7afd → c8bf987
- google.golang.org/grpc: v1.40.0 → v1.47.0
- google.golang.org/protobuf: v1.27.1 → v1.28.1
- gopkg.in/check.v1: 8fa4692 → 10cb982
- gopkg.in/yaml.v3: 496545a → v3.0.1
- k8s.io/api: v0.24.0 → v0.25.2
- k8s.io/apiextensions-apiserver: v0.24.0 → v0.25.2
- k8s.io/apimachinery: v0.24.0 → v0.25.2
- k8s.io/apiserver: v0.24.0 → v0.25.2
- k8s.io/cli-runtime: v0.24.0 → v0.25.2
- k8s.io/client-go: v0.24.0 → v0.25.2
- k8s.io/cloud-provider: v0.24.0 → v0.25.2
- k8s.io/cluster-bootstrap: v0.24.0 → v0.25.2
- k8s.io/code-generator: v0.24.0 → v0.25.2
- k8s.io/component-base: v0.24.0 → v0.25.2
- k8s.io/component-helpers: v0.24.0 → v0.25.2
- k8s.io/controller-manager: v0.24.0 → v0.25.2
- k8s.io/cri-api: v0.24.0 → v0.25.2
- k8s.io/csi-translation-lib: v0.24.0 → v0.25.2
- k8s.io/gengo: c02415c → 3913671
- k8s.io/klog/v2: v2.60.1 → v2.80.1
- k8s.io/kube-aggregator: v0.24.0 → v0.25.2
- k8s.io/kube-controller-manager: v0.24.0 → v0.25.2
- k8s.io/kube-openapi: 3ee0da9 → a70c9af
- k8s.io/kube-proxy: v0.24.0 → v0.25.2
- k8s.io/kube-scheduler: v0.24.0 → v0.25.2
- k8s.io/kubectl: v0.24.0 → v0.25.2
- k8s.io/kubelet: v0.24.0 → v0.25.2
- k8s.io/kubernetes: v1.23.0 → v1.25.2
- k8s.io/legacy-cloud-providers: v0.24.0 → v0.25.2
- k8s.io/metrics: v0.24.0 → v0.25.2
- k8s.io/mount-utils: v0.24.0 → v0.25.2
- k8s.io/pod-security-admission: v0.24.0 → v0.25.2
- k8s.io/sample-apiserver: v0.24.0 → v0.25.2
- k8s.io/system-validators: v1.6.0 → v1.7.0
- k8s.io/utils: 3a6ce19 → ee6ede2
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.30 → v0.0.32
- sigs.k8s.io/json: 9f7c6b3 → f223a00
- sigs.k8s.io/kustomize/api: v0.11.4 → v0.12.1
- sigs.k8s.io/kustomize/kustomize/v4: v4.5.4 → v4.5.7
- sigs.k8s.io/kustomize/kyaml: v0.13.6 → v0.13.9
- sigs.k8s.io/structured-merge-diff/v4: v4.2.1 → v4.2.3

### Removed
- bazil.org/fuse: 371fbbd
- cloud.google.com/go/firestore: v1.1.0
- github.com/ajstarks/svgo: [644b8db](https://github.com/ajstarks/svgo/tree/644b8db)
- github.com/armon/consul-api: [eb2c6b5](https://github.com/armon/consul-api/tree/eb2c6b5)
- github.com/armon/go-metrics: [f0300d1](https://github.com/armon/go-metrics/tree/f0300d1)
- github.com/armon/go-radix: [7fddfc3](https://github.com/armon/go-radix/tree/7fddfc3)
- github.com/bgentry/speakeasy: [v0.1.0](https://github.com/bgentry/speakeasy/tree/v0.1.0)
- github.com/bits-and-blooms/bitset: [v1.2.0](https://github.com/bits-and-blooms/bitset/tree/v1.2.0)
- github.com/bketelsen/crypt: [v0.0.4](https://github.com/bketelsen/crypt/tree/v0.0.4)
- github.com/blang/semver: [v3.5.1+incompatible](https://github.com/blang/semver/tree/v3.5.1)
- github.com/certifi/gocertifi: [2c3bb06](https://github.com/certifi/gocertifi/tree/2c3bb06)
- github.com/clusterhq/flocker-go: [2b8b725](https://github.com/clusterhq/flocker-go/tree/2b8b725)
- github.com/cockroachdb/datadriven: [bf6692d](https://github.com/cockroachdb/datadriven/tree/bf6692d)
- github.com/cockroachdb/errors: [v1.2.4](https://github.com/cockroachdb/errors/tree/v1.2.4)
- github.com/cockroachdb/logtags: [eb05cc2](https://github.com/cockroachdb/logtags/tree/eb05cc2)
- github.com/containerd/containerd: [v1.4.11](https://github.com/containerd/containerd/tree/v1.4.11)
- github.com/containerd/continuity: [v0.1.0](https://github.com/containerd/continuity/tree/v0.1.0)
- github.com/containerd/fifo: [v1.0.0](https://github.com/containerd/fifo/tree/v1.0.0)
- github.com/containerd/go-runc: [v1.0.0](https://github.com/containerd/go-runc/tree/v1.0.0)
- github.com/containerd/typeurl: [v1.0.2](https://github.com/containerd/typeurl/tree/v1.0.2)
- github.com/containernetworking/cni: [v0.8.1](https://github.com/containernetworking/cni/tree/v0.8.1)
- github.com/coreos/bbolt: [v1.3.2](https://github.com/coreos/bbolt/tree/v1.3.2)
- github.com/coreos/etcd: [v3.3.13+incompatible](https://github.com/coreos/etcd/tree/v3.3.13)
- github.com/coreos/go-systemd: [95778df](https://github.com/coreos/go-systemd/tree/95778df)
- github.com/coreos/pkg: [399ea9e](https://github.com/coreos/pkg/tree/399ea9e)
- github.com/dgrijalva/jwt-go: [v3.2.0+incompatible](https://github.com/dgrijalva/jwt-go/tree/v3.2.0)
- github.com/dgryski/go-sip13: [e10d5fe](https://github.com/dgryski/go-sip13/tree/e10d5fe)
- github.com/dnaeon/go-vcr: [v1.0.1](https://github.com/dnaeon/go-vcr/tree/v1.0.1)
- github.com/docker/docker: [v20.10.7+incompatible](https://github.com/docker/docker/tree/v20.10.7)
- github.com/docker/go-connections: [v0.4.0](https://github.com/docker/go-connections/tree/v0.4.0)
- github.com/emicklei/go-restful: [v2.9.5+incompatible](https://github.com/emicklei/go-restful/tree/v2.9.5)
- github.com/fatih/color: [v1.7.0](https://github.com/fatih/color/tree/v1.7.0)
- github.com/flynn/go-shlex: [3f9db97](https://github.com/flynn/go-shlex/tree/3f9db97)
- github.com/fogleman/gg: [0403632](https://github.com/fogleman/gg/tree/0403632)
- github.com/frankban/quicktest: [v1.11.3](https://github.com/frankban/quicktest/tree/v1.11.3)
- github.com/getsentry/raven-go: [v0.2.0](https://github.com/getsentry/raven-go/tree/v0.2.0)
- github.com/golang/freetype: [e2365df](https://github.com/golang/freetype/tree/e2365df)
- github.com/golangplus/testing: [af21d9c](https://github.com/golangplus/testing/tree/af21d9c)
- github.com/google/cel-spec: [v0.6.0](https://github.com/google/cel-spec/tree/v0.6.0)
- github.com/googleapis/gnostic: [v0.5.5](https://github.com/googleapis/gnostic/tree/v0.5.5)
- github.com/gopherjs/gopherjs: [fce0ec3](https://github.com/gopherjs/gopherjs/tree/fce0ec3)
- github.com/hashicorp/consul/api: [v1.1.0](https://github.com/hashicorp/consul/api/tree/v1.1.0)
- github.com/hashicorp/consul/sdk: [v0.1.1](https://github.com/hashicorp/consul/sdk/tree/v0.1.1)
- github.com/hashicorp/errwrap: [v1.0.0](https://github.com/hashicorp/errwrap/tree/v1.0.0)
- github.com/hashicorp/go-cleanhttp: [v0.5.1](https://github.com/hashicorp/go-cleanhttp/tree/v0.5.1)
- github.com/hashicorp/go-immutable-radix: [v1.0.0](https://github.com/hashicorp/go-immutable-radix/tree/v1.0.0)
- github.com/hashicorp/go-msgpack: [v0.5.3](https://github.com/hashicorp/go-msgpack/tree/v0.5.3)
- github.com/hashicorp/go-multierror: [v1.0.0](https://github.com/hashicorp/go-multierror/tree/v1.0.0)
- github.com/hashicorp/go-rootcerts: [v1.0.0](https://github.com/hashicorp/go-rootcerts/tree/v1.0.0)
- github.com/hashicorp/go-sockaddr: [v1.0.0](https://github.com/hashicorp/go-sockaddr/tree/v1.0.0)
- github.com/hashicorp/go-syslog: [v1.0.0](https://github.com/hashicorp/go-syslog/tree/v1.0.0)
- github.com/hashicorp/go-uuid: [v1.0.1](https://github.com/hashicorp/go-uuid/tree/v1.0.1)
- github.com/hashicorp/go.net: [v0.0.1](https://github.com/hashicorp/go.net/tree/v0.0.1)
- github.com/hashicorp/hcl: [v1.0.0](https://github.com/hashicorp/hcl/tree/v1.0.0)
- github.com/hashicorp/logutils: [v1.0.0](https://github.com/hashicorp/logutils/tree/v1.0.0)
- github.com/hashicorp/mdns: [v1.0.0](https://github.com/hashicorp/mdns/tree/v1.0.0)
- github.com/hashicorp/memberlist: [v0.1.3](https://github.com/hashicorp/memberlist/tree/v0.1.3)
- github.com/hashicorp/serf: [v0.8.2](https://github.com/hashicorp/serf/tree/v0.8.2)
- github.com/jmespath/go-jmespath/internal/testify: [v1.5.1](https://github.com/jmespath/go-jmespath/internal/testify/tree/v1.5.1)
- github.com/jtolds/gls: [v4.20.0+incompatible](https://github.com/jtolds/gls/tree/v4.20.0)
- github.com/jung-kurt/gofpdf: [24315ac](https://github.com/jung-kurt/gofpdf/tree/24315ac)
- github.com/kr/fs: [v0.1.0](https://github.com/kr/fs/tree/v0.1.0)
- github.com/magiconair/properties: [v1.8.5](https://github.com/magiconair/properties/tree/v1.8.5)
- github.com/mattn/go-colorable: [v0.0.9](https://github.com/mattn/go-colorable/tree/v0.0.9)
- github.com/mattn/go-isatty: [v0.0.3](https://github.com/mattn/go-isatty/tree/v0.0.3)
- github.com/mattn/go-runewidth: [v0.0.7](https://github.com/mattn/go-runewidth/tree/v0.0.7)
- github.com/miekg/dns: [v1.0.14](https://github.com/miekg/dns/tree/v1.0.14)
- github.com/mitchellh/cli: [v1.0.0](https://github.com/mitchellh/cli/tree/v1.0.0)
- github.com/mitchellh/go-homedir: [v1.1.0](https://github.com/mitchellh/go-homedir/tree/v1.1.0)
- github.com/mitchellh/go-testing-interface: [v1.0.0](https://github.com/mitchellh/go-testing-interface/tree/v1.0.0)
- github.com/mitchellh/gox: [v0.4.0](https://github.com/mitchellh/gox/tree/v0.4.0)
- github.com/mitchellh/iochan: [v1.0.0](https://github.com/mitchellh/iochan/tree/v1.0.0)
- github.com/morikuni/aec: [v1.0.0](https://github.com/morikuni/aec/tree/v1.0.0)
- github.com/oklog/ulid: [v1.3.1](https://github.com/oklog/ulid/tree/v1.3.1)
- github.com/olekukonko/tablewriter: [v0.0.4](https://github.com/olekukonko/tablewriter/tree/v0.0.4)
- github.com/opencontainers/image-spec: [v1.0.1](https://github.com/opencontainers/image-spec/tree/v1.0.1)
- github.com/opentracing/opentracing-go: [v1.1.0](https://github.com/opentracing/opentracing-go/tree/v1.1.0)
- github.com/pascaldekloe/goe: [57f6aae](https://github.com/pascaldekloe/goe/tree/57f6aae)
- github.com/pelletier/go-toml: [v1.9.3](https://github.com/pelletier/go-toml/tree/v1.9.3)
- github.com/pkg/sftp: [v1.10.1](https://github.com/pkg/sftp/tree/v1.10.1)
- github.com/posener/complete: [v1.1.1](https://github.com/posener/complete/tree/v1.1.1)
- github.com/prometheus/tsdb: [v0.7.1](https://github.com/prometheus/tsdb/tree/v0.7.1)
- github.com/quobyte/api: [v0.1.8](https://github.com/quobyte/api/tree/v0.1.8)
- github.com/remyoudompheng/bigfft: [52369c6](https://github.com/remyoudompheng/bigfft/tree/52369c6)
- github.com/ryanuber/columnize: [9b3edd6](https://github.com/ryanuber/columnize/tree/9b3edd6)
- github.com/sean-/seed: [e2103e2](https://github.com/sean-/seed/tree/e2103e2)
- github.com/sergi/go-diff: [v1.1.0](https://github.com/sergi/go-diff/tree/v1.1.0)
- github.com/shurcooL/sanitized_anchor_name: [v1.0.0](https://github.com/shurcooL/sanitized_anchor_name/tree/v1.0.0)
- github.com/smartystreets/assertions: [v1.1.0](https://github.com/smartystreets/assertions/tree/v1.1.0)
- github.com/smartystreets/goconvey: [v1.6.4](https://github.com/smartystreets/goconvey/tree/v1.6.4)
- github.com/spf13/cast: [v1.3.1](https://github.com/spf13/cast/tree/v1.3.1)
- github.com/spf13/jwalterweatherman: [v1.1.0](https://github.com/spf13/jwalterweatherman/tree/v1.1.0)
- github.com/spf13/viper: [v1.8.1](https://github.com/spf13/viper/tree/v1.8.1)
- github.com/storageos/go-api: [v2.2.0+incompatible](https://github.com/storageos/go-api/tree/v2.2.0)
- github.com/subosito/gotenv: [v1.2.0](https://github.com/subosito/gotenv/tree/v1.2.0)
- github.com/ugorji/go: [v1.1.4](https://github.com/ugorji/go/tree/v1.1.4)
- github.com/urfave/cli: [v1.22.2](https://github.com/urfave/cli/tree/v1.22.2)
- github.com/xordataexchange/crypt: [b2862e3](https://github.com/xordataexchange/crypt/tree/b2862e3)
- gonum.org/v1/plot: e2840ee
- gopkg.in/ini.v1: v1.62.0
- gopkg.in/resty.v1: v1.12.0
- modernc.org/cc: v1.0.0
- modernc.org/golex: v1.0.0
- modernc.org/mathutil: v1.0.0
- modernc.org/strutil: v1.0.0
- modernc.org/xc: v1.0.0
- rsc.io/pdf: v0.1.1
- sigs.k8s.io/kustomize/cmd/config: v0.10.6
