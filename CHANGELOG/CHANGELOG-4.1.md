# Release notes for v4.1.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v4.0.0

## Changes by Kind

### Deprecations

- VolumeSnapshot v1beta1 is deprecated and will be removed in a future release. It is recommended for users to upgrade to VolumeSnapshot CRD version v1 as soon as possible. Any previously created invalid v1beta1 objects have to be deleted before upgrading to version 4.1.0. ([#493](https://github.com/kubernetes-csi/external-snapshotter/pull/493), [@xing-yang](https://github.com/xing-yang))

### API Change

- Changes VolumeSnapshot API storage version from v1beta1 to v1; VolumeSnapshot v1beta1 is deprecated and will be removed in a future release. ([#493](https://github.com/kubernetes-csi/external-snapshotter/pull/493), [@xing-yang](https://github.com/xing-yang))

### Bug or Regression

- --http-endpoint will now correctly be used for the metrics server address when --metrics-address is not provided. ([#496](https://github.com/kubernetes-csi/external-snapshotter/pull/496), [@ggriffiths](https://github.com/ggriffiths))
- Add check for v1 CRDs to allow for rolling update of the snapshot-controller ([#504](https://github.com/kubernetes-csi/external-snapshotter/pull/504), [@mauriciopoppe](https://github.com/mauriciopoppe))
- VolumeSnapshotContent creation errors can now propagate to the appropriate VolumeSnapshotContent resource. ([#502](https://github.com/kubernetes-csi/external-snapshotter/pull/502), [@huffmanca](https://github.com/huffmanca))
- Retain error from CreateSnapshot call ([#470](https://github.com/kubernetes-csi/external-snapshotter/pull/470), [@timoreimann](https://github.com/timoreimann))

### Uncategorized

- External-snapshotter manifests adjusted to reflect more common example. Snapshot-controller is deployed as a Deployment rather than a Statefulset in the example deployment file. ([#459](https://github.com/kubernetes-csi/external-snapshotter/pull/459), [@kvaps](https://github.com/kvaps))
- Updated runtime (Go 1.16) and dependencies ([#483](https://github.com/kubernetes-csi/external-snapshotter/pull/483), [@pohly](https://github.com/pohly))

## Dependencies

### Added
- github.com/go-errors/errors: [v1.0.1](https://github.com/go-errors/errors/tree/v1.0.1)
- github.com/gobuffalo/here: [v0.6.0](https://github.com/gobuffalo/here/tree/v0.6.0)
- github.com/google/shlex: [e7afc7f](https://github.com/google/shlex/tree/e7afc7f)
- github.com/markbates/pkger: [v0.17.1](https://github.com/markbates/pkger/tree/v0.17.1)
- github.com/moby/spdystream: [v0.2.0](https://github.com/moby/spdystream/tree/v0.2.0)
- github.com/monochromegane/go-gitignore: [205db1a](https://github.com/monochromegane/go-gitignore/tree/205db1a)
- github.com/niemeyer/pretty: [a10e7ca](https://github.com/niemeyer/pretty/tree/a10e7ca)
- github.com/xlab/treeprint: [a009c39](https://github.com/xlab/treeprint/tree/a009c39)
- go.starlark.net: 8dd3e2e
- sigs.k8s.io/kustomize/api: v0.8.5
- sigs.k8s.io/kustomize/cmd/config: v0.9.7
- sigs.k8s.io/kustomize/kustomize/v4: v4.0.5
- sigs.k8s.io/kustomize/kyaml: v0.10.15

### Changed
- dmitri.shuralyov.com/gpu/mtl: 666a987 → 28db891
- github.com/Azure/go-autorest/autorest: [v0.11.1 → v0.11.12](https://github.com/Azure/go-autorest/autorest/compare/v0.11.1...v0.11.12)
- github.com/NYTimes/gziphandler: [56545f4 → v1.1.1](https://github.com/NYTimes/gziphandler/compare/56545f4...v1.1.1)
- github.com/cilium/ebpf: [1c8d4c9 → v0.2.0](https://github.com/cilium/ebpf/compare/1c8d4c9...v0.2.0)
- github.com/container-storage-interface/spec: [v1.3.0 → v1.4.0](https://github.com/container-storage-interface/spec/compare/v1.3.0...v1.4.0)
- github.com/containerd/console: [v1.0.0 → v1.0.1](https://github.com/containerd/console/compare/v1.0.0...v1.0.1)
- github.com/containerd/containerd: [v1.4.1 → v1.4.4](https://github.com/containerd/containerd/compare/v1.4.1...v1.4.4)
- github.com/coredns/corefile-migration: [v1.0.10 → v1.0.11](https://github.com/coredns/corefile-migration/compare/v1.0.10...v1.0.11)
- github.com/creack/pty: [v1.1.7 → v1.1.11](https://github.com/creack/pty/compare/v1.1.7...v1.1.11)
- github.com/docker/docker: [bd33bbf → v20.10.2+incompatible](https://github.com/docker/docker/compare/bd33bbf...v20.10.2)
- github.com/go-logr/logr: [v0.3.0 → v0.4.0](https://github.com/go-logr/logr/compare/v0.3.0...v0.4.0)
- github.com/go-openapi/spec: [v0.19.3 → v0.19.5](https://github.com/go-openapi/spec/compare/v0.19.3...v0.19.5)
- github.com/go-openapi/strfmt: [v0.19.3 → v0.19.5](https://github.com/go-openapi/strfmt/compare/v0.19.3...v0.19.5)
- github.com/go-openapi/validate: [v0.19.5 → v0.19.8](https://github.com/go-openapi/validate/compare/v0.19.5...v0.19.8)
- github.com/gogo/protobuf: [v1.3.1 → v1.3.2](https://github.com/gogo/protobuf/compare/v1.3.1...v1.3.2)
- github.com/google/cadvisor: [v0.38.5 → v0.39.0](https://github.com/google/cadvisor/compare/v0.38.5...v0.39.0)
- github.com/heketi/heketi: [c2e2a4a → v10.2.0+incompatible](https://github.com/heketi/heketi/compare/c2e2a4a...v10.2.0)
- github.com/kisielk/errcheck: [v1.2.0 → v1.5.0](https://github.com/kisielk/errcheck/compare/v1.2.0...v1.5.0)
- github.com/kr/text: [v0.1.0 → v0.2.0](https://github.com/kr/text/compare/v0.1.0...v0.2.0)
- github.com/mattn/go-runewidth: [v0.0.2 → v0.0.7](https://github.com/mattn/go-runewidth/compare/v0.0.2...v0.0.7)
- github.com/miekg/dns: [v1.1.4 → v1.1.35](https://github.com/miekg/dns/compare/v1.1.4...v1.1.35)
- github.com/moby/sys/mountinfo: [v0.1.3 → v0.4.0](https://github.com/moby/sys/mountinfo/compare/v0.1.3...v0.4.0)
- github.com/moby/term: [672ec06 → df9cb8a](https://github.com/moby/term/compare/672ec06...df9cb8a)
- github.com/mrunalp/fileutils: [abd8a0e → v0.5.0](https://github.com/mrunalp/fileutils/compare/abd8a0e...v0.5.0)
- github.com/olekukonko/tablewriter: [a0225b3 → v0.0.4](https://github.com/olekukonko/tablewriter/compare/a0225b3...v0.0.4)
- github.com/opencontainers/runc: [v1.0.0-rc92 → v1.0.0-rc93](https://github.com/opencontainers/runc/compare/v1.0.0-rc92...v1.0.0-rc93)
- github.com/opencontainers/runtime-spec: [4d89ac9 → e6143ca](https://github.com/opencontainers/runtime-spec/compare/4d89ac9...e6143ca)
- github.com/opencontainers/selinux: [v1.6.0 → v1.8.0](https://github.com/opencontainers/selinux/compare/v1.6.0...v1.8.0)
- github.com/sergi/go-diff: [v1.0.0 → v1.1.0](https://github.com/sergi/go-diff/compare/v1.0.0...v1.1.0)
- github.com/sirupsen/logrus: [v1.6.0 → v1.7.0](https://github.com/sirupsen/logrus/compare/v1.6.0...v1.7.0)
- github.com/syndtr/gocapability: [d983527 → 42c35b4](https://github.com/syndtr/gocapability/compare/d983527...42c35b4)
- github.com/willf/bitset: [d5bec33 → v1.1.11](https://github.com/willf/bitset/compare/d5bec33...v1.1.11)
- github.com/yuin/goldmark: [v1.1.32 → v1.2.1](https://github.com/yuin/goldmark/compare/v1.1.32...v1.2.1)
- golang.org/x/crypto: 5f87f34 → 5ea612d
- golang.org/x/exp: 6cc2880 → 85be41e
- golang.org/x/mobile: d2bd2a2 → e6ae53a
- golang.org/x/mod: v0.3.0 → ce943fd
- golang.org/x/net: ac852fb → 3d97a24
- golang.org/x/sync: 6e8e738 → 67f06af
- golang.org/x/sys: aec9a39 → a50acf3
- golang.org/x/term: 2321bbc → 6a3ed07
- golang.org/x/time: 7e3f01d → f8bda1e
- golang.org/x/tools: b303f43 → v0.1.0
- gopkg.in/check.v1: 41f04d3 → 8fa4692
- gotest.tools/v3: v3.0.2 → v3.0.3
- k8s.io/api: v0.20.0 → v0.21.0
- k8s.io/apiextensions-apiserver: v0.20.0 → v0.21.0
- k8s.io/apimachinery: v0.20.0 → v0.21.0
- k8s.io/apiserver: v0.20.0 → v0.21.0
- k8s.io/cli-runtime: v0.20.0 → v0.21.0
- k8s.io/client-go: v0.20.0 → v0.21.0
- k8s.io/cloud-provider: v0.20.0 → v0.21.0
- k8s.io/cluster-bootstrap: v0.20.0 → v0.21.0
- k8s.io/code-generator: v0.20.0 → v0.21.0
- k8s.io/component-base: v0.20.0 → v0.21.0
- k8s.io/component-helpers: v0.20.0 → v0.21.0
- k8s.io/controller-manager: v0.20.0 → v0.21.0
- k8s.io/cri-api: v0.20.0 → v0.21.0
- k8s.io/csi-translation-lib: v0.20.0 → v0.21.0
- k8s.io/gengo: 83324d8 → b6c5ce2
- k8s.io/klog/v2: v2.4.0 → v2.8.0
- k8s.io/kube-aggregator: v0.20.0 → v0.21.0
- k8s.io/kube-controller-manager: v0.20.0 → v0.21.0
- k8s.io/kube-openapi: d219536 → 591a79e
- k8s.io/kube-proxy: v0.20.0 → v0.21.0
- k8s.io/kube-scheduler: v0.20.0 → v0.21.0
- k8s.io/kubectl: v0.20.0 → v0.21.0
- k8s.io/kubelet: v0.20.0 → v0.21.0
- k8s.io/kubernetes: v1.20.0 → v1.21.0
- k8s.io/legacy-cloud-providers: v0.20.0 → v0.21.0
- k8s.io/metrics: v0.20.0 → v0.21.0
- k8s.io/mount-utils: v0.20.0 → v0.21.0
- k8s.io/sample-apiserver: v0.20.0 → v0.21.0
- k8s.io/system-validators: v1.2.0 → v1.4.0
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.14 → v0.0.15
- sigs.k8s.io/structured-merge-diff/v4: v4.0.2 → v4.1.0

### Removed
- github.com/codegangsta/negroni: [v1.0.0](https://github.com/codegangsta/negroni/tree/v1.0.0)
- github.com/docker/spdystream: [449fdfc](https://github.com/docker/spdystream/tree/449fdfc)
- github.com/golangplus/bytes: [45c989f](https://github.com/golangplus/bytes/tree/45c989f)
- github.com/golangplus/fmt: [2a5d6d7](https://github.com/golangplus/fmt/tree/2a5d6d7)
- sigs.k8s.io/kustomize: v2.0.3+incompatible
