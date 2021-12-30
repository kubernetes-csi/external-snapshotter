# Release notes for v5.0.0 

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v4.2.0

## Changes by Kind

### Feature

- Added short names for Volume Snapshot CRDs:
  - VolumeSnapshot - vs
  - VolumeSnapshotContent - vsc, vscs
  - VolumeSnapshotClass` - vsclass, vsclasses ([#604](https://github.com/kubernetes-csi/external-snapshotter/pull/604), [@robbie-demuth](https://github.com/robbie-demuth))
- Adds support for distributed snapshotting. This affects both snapshot controller and CSI snapshotter sidecar. ([#585](https://github.com/kubernetes-csi/external-snapshotter/pull/585), [@nearora-msft](https://github.com/nearora-msft))
- Make the QPS and Burst of kube client config to be configurable in both snapshot-controller and CSI snapshotter sidecar ([#621](https://github.com/kubernetes-csi/external-snapshotter/pull/621), [@lintongj](https://github.com/lintongj))

### Design

- Added kustomization manifests to CRDs, snapshot controller, and CSI snapshotter sidecar components ([#606](https://github.com/kubernetes-csi/external-snapshotter/pull/606), [@itspngu](https://github.com/itspngu))

### Bug or Regression

- Fixed a bug introduced by [#621](https://github.com/kubernetes-csi/external-snapshotter/pull/621) which makes the QPS and Burst of kube client config configurable in both snapshot-controller and CSI snapshotter sidecar. This fix exposed the kube-api-qps cmd option properly ([#626](https://github.com/kubernetes-csi/external-snapshotter/pull/626), [@lintongj](https://github.com/lintongj))
- Fixed deadlock in reporting metrics in snapshot controller. ([#581](https://github.com/kubernetes-csi/external-snapshotter/pull/581), [@jsafrane](https://github.com/jsafrane))

### Other (Cleanup or Flake)

- Rename KUBE_NODE_NAME to NODE_NAME for CSI snapshotter sidecar deployment. ([#616](https://github.com/kubernetes-csi/external-snapshotter/pull/616), [@zhucan](https://github.com/zhucan))
- Replaces many VolumeSnapshot/VolumeSnapshotContent Update/UpdateStatus operations with Patch. This lowers the probability of the "object has been modified" update API errors occurring. This change introduces a dependency on two new RBAC rules for the CSI snapshotter sidecar: volumesnapshotcontents:patch, volumesnapshotcontents/status:patch and four new RBAC rules for the snapshot-controller: volumesnapshotcontents:patch, volumesnapshotcontents/status:patch, volumesnapshots:patch, and volumesnapshots/status: patch. ([#526](https://github.com/kubernetes-csi/external-snapshotter/pull/526), [@ggriffiths](https://github.com/ggriffiths))

### Uncategorized

- Updated `CertificateSigningRequest apiversion` to `V1` for Snapshot validation webhook deployment. ([#588](https://github.com/kubernetes-csi/external-snapshotter/pull/588), [@Kartik494](https://github.com/Kartik494))

## Dependencies

### Added
- bazil.org/fuse: 371fbbd
- github.com/antlr/antlr4/runtime/Go/antlr: [b48c857](https://github.com/antlr/antlr4/runtime/Go/antlr/tree/b48c857)
- github.com/bits-and-blooms/bitset: [v1.2.0](https://github.com/bits-and-blooms/bitset/tree/v1.2.0)
- github.com/checkpoint-restore/go-criu/v5: [v5.0.0](https://github.com/checkpoint-restore/go-criu/v5/tree/v5.0.0)
- github.com/cncf/xds/go: [fbca930](https://github.com/cncf/xds/go/tree/fbca930)
- github.com/coredns/caddy: [v1.1.0](https://github.com/coredns/caddy/tree/v1.1.0)
- github.com/frankban/quicktest: [v1.11.3](https://github.com/frankban/quicktest/tree/v1.11.3)
- github.com/getkin/kin-openapi: [v0.76.0](https://github.com/getkin/kin-openapi/tree/v0.76.0)
- github.com/go-logr/zapr: [v1.2.0](https://github.com/go-logr/zapr/tree/v1.2.0)
- github.com/google/cel-go: [v0.9.0](https://github.com/google/cel-go/tree/v0.9.0)
- github.com/google/cel-spec: [v0.6.0](https://github.com/google/cel-spec/tree/v0.6.0)
- github.com/kr/fs: [v0.1.0](https://github.com/kr/fs/tree/v0.1.0)
- github.com/pkg/sftp: [v1.10.1](https://github.com/pkg/sftp/tree/v1.10.1)
- github.com/robfig/cron/v3: [v3.0.1](https://github.com/robfig/cron/v3/tree/v3.0.1)
- k8s.io/pod-security-admission: v0.23.0
- sigs.k8s.io/json: c049b76

### Changed
- cloud.google.com/go: v0.65.0 → v0.81.0
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [7901bc8 → ea6160c](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/compare/7901bc8...ea6160c)
- github.com/Microsoft/go-winio: [v0.4.15 → v0.4.17](https://github.com/Microsoft/go-winio/compare/v0.4.15...v0.4.17)
- github.com/Microsoft/hcsshim: [5eafd15 → v0.8.22](https://github.com/Microsoft/hcsshim/compare/5eafd15...v0.8.22)
- github.com/auth0/go-jwt-middleware: [5493cab → v1.0.1](https://github.com/auth0/go-jwt-middleware/compare/5493cab...v1.0.1)
- github.com/benbjohnson/clock: [v1.0.3 → v1.1.0](https://github.com/benbjohnson/clock/compare/v1.0.3...v1.1.0)
- github.com/bketelsen/crypt: [5cbc8cc → v0.0.4](https://github.com/bketelsen/crypt/compare/5cbc8cc...v0.0.4)
- github.com/cilium/ebpf: [v0.2.0 → v0.6.2](https://github.com/cilium/ebpf/compare/v0.2.0...v0.6.2)
- github.com/containerd/cgroups: [0dbf7f0 → v1.0.1](https://github.com/containerd/cgroups/compare/0dbf7f0...v1.0.1)
- github.com/containerd/console: [v1.0.1 → v1.0.2](https://github.com/containerd/console/compare/v1.0.1...v1.0.2)
- github.com/containerd/containerd: [v1.4.4 → v1.4.11](https://github.com/containerd/containerd/compare/v1.4.4...v1.4.11)
- github.com/containerd/continuity: [aaeac12 → v0.1.0](https://github.com/containerd/continuity/compare/aaeac12...v0.1.0)
- github.com/containerd/fifo: [a9fb20d → v1.0.0](https://github.com/containerd/fifo/compare/a9fb20d...v1.0.0)
- github.com/containerd/go-runc: [5a6d9f3 → v1.0.0](https://github.com/containerd/go-runc/compare/5a6d9f3...v1.0.0)
- github.com/containerd/typeurl: [v1.0.1 → v1.0.2](https://github.com/containerd/typeurl/compare/v1.0.1...v1.0.2)
- github.com/containernetworking/cni: [v0.8.0 → v0.8.1](https://github.com/containernetworking/cni/compare/v0.8.0...v0.8.1)
- github.com/coredns/corefile-migration: [v1.0.11 → v1.0.14](https://github.com/coredns/corefile-migration/compare/v1.0.11...v1.0.14)
- github.com/docker/docker: [v20.10.2+incompatible → v20.10.7+incompatible](https://github.com/docker/docker/compare/v20.10.2...v20.10.7)
- github.com/envoyproxy/go-control-plane: [668b12f → 63b5d3c](https://github.com/envoyproxy/go-control-plane/compare/668b12f...63b5d3c)
- github.com/evanphx/json-patch: [v4.11.0+incompatible → v4.12.0+incompatible](https://github.com/evanphx/json-patch/compare/v4.11.0...v4.12.0)
- github.com/go-logr/logr: [v0.4.0 → v1.2.0](https://github.com/go-logr/logr/compare/v0.4.0...v1.2.0)
- github.com/golang/glog: [23def4e → v1.0.0](https://github.com/golang/glog/compare/23def4e...v1.0.0)
- github.com/golang/mock: [v1.4.4 → v1.5.0](https://github.com/golang/mock/compare/v1.4.4...v1.5.0)
- github.com/google/cadvisor: [v0.39.0 → v0.43.0](https://github.com/google/cadvisor/compare/v0.39.0...v0.43.0)
- github.com/google/martian/v3: [v3.0.0 → v3.1.0](https://github.com/google/martian/v3/compare/v3.0.0...v3.1.0)
- github.com/google/pprof: [1a94d86 → cbba55b](https://github.com/google/pprof/compare/1a94d86...cbba55b)
- github.com/gopherjs/gopherjs: [0766667 → fce0ec3](https://github.com/gopherjs/gopherjs/compare/0766667...fce0ec3)
- github.com/heketi/heketi: [v10.2.0+incompatible → v10.3.0+incompatible](https://github.com/heketi/heketi/compare/v10.2.0...v10.3.0)
- github.com/ianlancetaylor/demangle: [5e5cf60 → 28f6c0f](https://github.com/ianlancetaylor/demangle/compare/5e5cf60...28f6c0f)
- github.com/json-iterator/go: [v1.1.11 → v1.1.12](https://github.com/json-iterator/go/compare/v1.1.11...v1.1.12)
- github.com/kr/pretty: [v0.2.0 → v0.2.1](https://github.com/kr/pretty/compare/v0.2.0...v0.2.1)
- github.com/kr/pty: [v1.1.5 → v1.1.1](https://github.com/kr/pty/compare/v1.1.5...v1.1.1)
- github.com/magiconair/properties: [v1.8.1 → v1.8.5](https://github.com/magiconair/properties/compare/v1.8.1...v1.8.5)
- github.com/mattn/go-isatty: [v0.0.4 → v0.0.3](https://github.com/mattn/go-isatty/compare/v0.0.4...v0.0.3)
- github.com/miekg/dns: [v1.1.35 → v1.0.14](https://github.com/miekg/dns/compare/v1.1.35...v1.0.14)
- github.com/mitchellh/mapstructure: [v1.1.2 → v1.4.1](https://github.com/mitchellh/mapstructure/compare/v1.1.2...v1.4.1)
- github.com/moby/sys/mountinfo: [v0.4.0 → v0.4.1](https://github.com/moby/sys/mountinfo/compare/v0.4.0...v0.4.1)
- github.com/modern-go/reflect2: [v1.0.1 → v1.0.2](https://github.com/modern-go/reflect2/compare/v1.0.1...v1.0.2)
- github.com/opencontainers/runc: [v1.0.0-rc93 → v1.0.2](https://github.com/opencontainers/runc/compare/v1.0.0-rc93...v1.0.2)
- github.com/opencontainers/runtime-spec: [e6143ca → 1c3f411](https://github.com/opencontainers/runtime-spec/compare/e6143ca...1c3f411)
- github.com/opencontainers/selinux: [v1.8.0 → v1.8.2](https://github.com/opencontainers/selinux/compare/v1.8.0...v1.8.2)
- github.com/pelletier/go-toml: [v1.2.0 → v1.9.3](https://github.com/pelletier/go-toml/compare/v1.2.0...v1.9.3)
- github.com/prometheus/common: [v0.26.0 → v0.28.0](https://github.com/prometheus/common/compare/v0.26.0...v0.28.0)
- github.com/smartystreets/assertions: [b2de0cb → v1.1.0](https://github.com/smartystreets/assertions/compare/b2de0cb...v1.1.0)
- github.com/spf13/afero: [v1.2.2 → v1.6.0](https://github.com/spf13/afero/compare/v1.2.2...v1.6.0)
- github.com/spf13/cast: [v1.3.0 → v1.3.1](https://github.com/spf13/cast/compare/v1.3.0...v1.3.1)
- github.com/spf13/cobra: [v1.1.3 → v1.2.1](https://github.com/spf13/cobra/compare/v1.1.3...v1.2.1)
- github.com/spf13/viper: [v1.7.0 → v1.8.1](https://github.com/spf13/viper/compare/v1.7.0...v1.8.1)
- github.com/yuin/goldmark: [v1.3.5 → v1.4.0](https://github.com/yuin/goldmark/compare/v1.3.5...v1.4.0)
- go.opencensus.io: v0.22.4 → v0.23.0
- go.uber.org/zap: v1.17.0 → v1.19.0
- golang.org/x/crypto: 5ea612d → 32db794
- golang.org/x/net: 37e1c6a → e898025
- golang.org/x/oauth2: 08078c5 → 2bc19b1
- golang.org/x/sys: 59db8d7 → f4d4317
- golang.org/x/term: 6a3ed07 → 6886f2d
- golang.org/x/text: v0.3.6 → v0.3.7
- golang.org/x/tools: v0.1.2 → d4cc65f
- google.golang.org/api: v0.30.0 → v0.46.0
- google.golang.org/genproto: f16073e → fe13028
- google.golang.org/grpc: v1.38.0 → v1.40.0
- google.golang.org/protobuf: v1.26.0 → v1.27.1
- gopkg.in/ini.v1: v1.51.0 → v1.62.0
- k8s.io/api: v0.22.0 → v0.23.0
- k8s.io/apiextensions-apiserver: v0.22.0 → v0.23.0
- k8s.io/apimachinery: v0.22.0 → v0.23.0
- k8s.io/apiserver: v0.22.0 → v0.23.0
- k8s.io/cli-runtime: v0.22.0 → v0.23.0
- k8s.io/client-go: v0.22.0 → v0.23.0
- k8s.io/cloud-provider: v0.22.0 → v0.23.0
- k8s.io/cluster-bootstrap: v0.22.0 → v0.23.0
- k8s.io/code-generator: v0.22.0 → v0.23.0
- k8s.io/component-base: v0.22.0 → v0.23.0
- k8s.io/component-helpers: v0.22.0 → v0.23.0
- k8s.io/controller-manager: v0.22.0 → v0.23.0
- k8s.io/cri-api: v0.22.0 → v0.23.0
- k8s.io/csi-translation-lib: v0.22.0 → v0.23.0
- k8s.io/gengo: b6c5ce2 → 485abfe
- k8s.io/klog/v2: v2.9.0 → v2.30.0
- k8s.io/kube-aggregator: v0.22.0 → v0.23.0
- k8s.io/kube-controller-manager: v0.22.0 → v0.23.0
- k8s.io/kube-openapi: 9528897 → e816edb
- k8s.io/kube-proxy: v0.22.0 → v0.23.0
- k8s.io/kube-scheduler: v0.22.0 → v0.23.0
- k8s.io/kubectl: v0.22.0 → v0.23.0
- k8s.io/kubelet: v0.22.0 → v0.23.0
- k8s.io/kubernetes: v1.21.0 → v1.23.0
- k8s.io/legacy-cloud-providers: v0.22.0 → v0.23.0
- k8s.io/metrics: v0.22.0 → v0.23.0
- k8s.io/mount-utils: v0.22.0 → v0.23.0
- k8s.io/sample-apiserver: v0.22.0 → v0.23.0
- k8s.io/system-validators: v1.4.0 → v1.6.0
- k8s.io/utils: 4b05e18 → cb0fa31
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.22 → v0.0.25
- sigs.k8s.io/kustomize/api: v0.8.11 → v0.10.1
- sigs.k8s.io/kustomize/cmd/config: v0.9.13 → v0.10.2
- sigs.k8s.io/kustomize/kustomize/v4: v4.2.0 → v4.4.1
- sigs.k8s.io/kustomize/kyaml: v0.11.0 → v0.13.0

### Removed
- github.com/bifurcation/mint: [93c51c6](https://github.com/bifurcation/mint/tree/93c51c6)
- github.com/caddyserver/caddy: [v1.0.3](https://github.com/caddyserver/caddy/tree/v1.0.3)
- github.com/cenkalti/backoff: [v2.1.1+incompatible](https://github.com/cenkalti/backoff/tree/v2.1.1)
- github.com/checkpoint-restore/go-criu/v4: [v4.1.0](https://github.com/checkpoint-restore/go-criu/v4/tree/v4.1.0)
- github.com/cheekybits/genny: [9127e81](https://github.com/cheekybits/genny/tree/9127e81)
- github.com/go-acme/lego: [v2.5.0+incompatible](https://github.com/go-acme/lego/tree/v2.5.0)
- github.com/go-bindata/go-bindata: [v3.1.1+incompatible](https://github.com/go-bindata/go-bindata/tree/v3.1.1)
- github.com/go-openapi/spec: [v0.19.5](https://github.com/go-openapi/spec/tree/v0.19.5)
- github.com/jimstudt/http-authentication: [3eca13d](https://github.com/jimstudt/http-authentication/tree/3eca13d)
- github.com/klauspost/cpuid: [v1.2.0](https://github.com/klauspost/cpuid/tree/v1.2.0)
- github.com/kylelemons/godebug: [d65d576](https://github.com/kylelemons/godebug/tree/d65d576)
- github.com/lucas-clemente/aes12: [cd47fb3](https://github.com/lucas-clemente/aes12/tree/cd47fb3)
- github.com/lucas-clemente/quic-clients: [v0.1.0](https://github.com/lucas-clemente/quic-clients/tree/v0.1.0)
- github.com/lucas-clemente/quic-go-certificates: [d2f8652](https://github.com/lucas-clemente/quic-go-certificates/tree/d2f8652)
- github.com/lucas-clemente/quic-go: [v0.10.2](https://github.com/lucas-clemente/quic-go/tree/v0.10.2)
- github.com/marten-seemann/qtls: [v0.2.3](https://github.com/marten-seemann/qtls/tree/v0.2.3)
- github.com/mholt/certmagic: [6a42ef9](https://github.com/mholt/certmagic/tree/6a42ef9)
- github.com/naoina/go-stringutil: [v0.1.0](https://github.com/naoina/go-stringutil/tree/v0.1.0)
- github.com/naoina/toml: [v0.1.1](https://github.com/naoina/toml/tree/v0.1.1)
- github.com/robfig/cron: [v1.1.0](https://github.com/robfig/cron/tree/v1.1.0)
- github.com/thecodeteam/goscaleio: [v0.1.0](https://github.com/thecodeteam/goscaleio/tree/v0.1.0)
- github.com/willf/bitset: [v1.1.11](https://github.com/willf/bitset/tree/v1.1.11)
- go.etcd.io/etcd: dd1b699
- gopkg.in/cheggaaa/pb.v1: v1.0.25
- gopkg.in/mcuadros/go-syslog.v2: v2.2.1
- gotest.tools: v2.2.0+incompatible
- k8s.io/heapster: v1.2.0-beta.1
