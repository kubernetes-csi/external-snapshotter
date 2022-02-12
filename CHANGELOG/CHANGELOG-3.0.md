# Release notes for v3.0.0

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v2.1.0

## Breaking Changes

- Update volume snapshot APIs and client library to v3. Volume snapshot APIs and client library are in a separate sub-module: `github.com/kubernetes-csi/external-snapshotter/client/v3`. ([#373](https://github.com/kubernetes-csi/external-snapshotter/pull/373), [@xing-yang](https://github.com/xing-yang))

- Added Go Module for APIs and Client. Volume snapshot APIs and client library are now in a separate sub-module. ([#307](https://github.com/kubernetes-csi/external-snapshotter/pull/307), [@Kartik494](https://github.com/Kartik494))

- With kubernetes 1.18 release of client-go, signatures on methods in generated clientsets, dynamic, metadata, and scale clients have been modified to accept context.Context as a first argument. Signatures of Create, Update, and Patch methods have been updated to accept CreateOptions, UpdateOptions and PatchOptions respectively. Signatures of Delete and DeleteCollection methods now accept DeleteOptions by value instead of by reference.  These changes are now accommodated with this PR and client-go and dependencies are updated to v1.18.0 ([#286](https://github.com/kubernetes-csi/external-snapshotter/pull/286), [@humblec](https://github.com/humblec))

## New Features

- The validation for volume snapshot objects (VolumeSnapshot and VolumeSnapshotContent) is getting more strict. Due to backwards compatibility this change will occur over multiple releases. The following key changes are highlighted.

  1. As part of the first phase of the multi-phased release process, a validating
webhook server has been added. This server will perform additional validation (strict) which was not done during the beta release of volume snapshots. It will prevent the cluster from gaining (via create or update) invalid objects.
  2. The controller will label objects which fail the additional validation. The label "snapshot.storage.kubernetes.io/invalid-snapshot-content-resource" will be added to invalid VolumeSnapshotContent objects. The label "snapshot.storage.kubernetes.io/invalid-snapshot-resource" will be added to invalid VolumeSnapshot objects.

  The combination of 1 and 2 will allow cluster admins to stop the increase of invalid objects, and provide a way to easily list all objects which currently fail the strict validation. It is the kubernets distribution and cluster admin's responsibility to install the webhook and to ensure all the invalid objects in the cluster have been deleted or fixed. See the KEP at https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/177-volume-snapshot/tighten-validation-webhook-crd.md ([#353](https://github.com/kubernetes-csi/external-snapshotter/pull/353), [@AndiLi99](https://github.com/AndiLi99))

### Snapshot Controller

- With the addition of the validating webhook server, the snapshot controller is modified to label objects which fail the additional validation. The label "snapshot.storage.kubernetes.io/invalid-snapshot-content-resource" will be added to invalid VolumeSnapshotContent objects. The label "snapshot.storage.kubernetes.io/invalid-snapshot-resource" will be added to invalid VolumeSnapshot objects. ([#353](https://github.com/kubernetes-csi/external-snapshotter/pull/353), [@AndiLi99](https://github.com/AndiLi99))
- Snapshot APIs and Client are now in a separate go package. The snapshot controller is modified to use the new package. ([#307](https://github.com/kubernetes-csi/external-snapshotter/pull/307), [@Kartik494](https://github.com/Kartik494))

### CSI External-Snapshotter Sidecar

- Snapshot APIs and Client are now in a separate go package. The CSI external-snapshotter sidecar is modified to use the new package. ([#307](https://github.com/kubernetes-csi/external-snapshotter/pull/307), [@Kartik494](https://github.com/Kartik494))

### Bug Fixes

### Snapshot Controller

- Fix a problem deleting the PVC finalizer and snapshot finalizer. ([#360](https://github.com/kubernetes-csi/external-snapshotter/pull/360), [@xing-yang](https://github.com/xing-yang))
- Emit event even if status update fails. ([#347](https://github.com/kubernetes-csi/external-snapshotter/pull/347), [@xing-yang](https://github.com/xing-yang))
- Use a separate API server client for leader election. ([#344](https://github.com/kubernetes-csi/external-snapshotter/pull/344), [@RaunakShah](https://github.com/RaunakShah))
- Recover from intermittent errors in VolumeSnapshot object ([#335](https://github.com/kubernetes-csi/external-snapshotter/pull/335), [@saikat-royc](https://github.com/saikat-royc))
- Updates Error field in the snapshot status based on the status of the content. ([#284](https://github.com/kubernetes-csi/external-snapshotter/pull/284), [@xing-yang](https://github.com/xing-yang))
- Fixes the requeue logic in the snapshot controller. ([#317](https://github.com/kubernetes-csi/external-snapshotter/pull/317), [@xing-yang](https://github.com/xing-yang))
- Fixes issue #290. Disallow a pre-provisioned VolumeSnapshot pointing to a dynamically created VolumeSnapshotContent. ([#294](https://github.com/kubernetes-csi/external-snapshotter/pull/294), [@yuxiangqian](https://github.com/yuxiangqian))
- Fixes issue #291. Verify VolumeSnapshot and VolumeSnapshotContent are bi-directional bound before initializing a deletion on a VolumeSnapshotContent which the to-be-deleted VolumeSnapshot points to. ([#294](https://github.com/kubernetes-csi/external-snapshotter/pull/294), [@yuxiangqian](https://github.com/yuxiangqian)) 
- Fixes issue #292. Allow deletion of a VolumeSnapshot when the VolumeSnapshotContent's DeletionPolicy has been updated from Delete to Retain. ([#294](https://github.com/kubernetes-csi/external-snapshotter/pull/294), [@yuxiangqian](https://github.com/yuxiangqian)) 

### CSI External-Snapshotter Sidecar

- Allows the sidecar to delete volume snapshots if the volume snapshot class is not found. ([#287](https://github.com/kubernetes-csi/external-snapshotter/pull/287), [@huffmanca](https://github.com/huffmanca))
- This fixes the re-queue logic so a failed snapshot operation will be added back to a rate limited queue for retries. ([#230](https://github.com/kubernetes-csi/external-snapshotter/pull/230), [@xing-yang](https://github.com/xing-yang))

### Other Notable Changes

### Snapshot Controller

- build with Go 1.15 ([#358](https://github.com/kubernetes-csi/external-snapshotter/pull/358), [@pohly](https://github.com/pohly))
- Updates kubernetes dependencies to v1.19.0. ([#372](https://github.com/kubernetes-csi/external-snapshotter/pull/372), [@humblec](https://github.com/humblec))
- Publishing of images on k8s.gcr.io ([#319](https://github.com/kubernetes-csi/external-snapshotter/pull/319), [@pohly](https://github.com/pohly))
- Add event for snapshotting in progress ([#289](https://github.com/kubernetes-csi/external-snapshotter/pull/289), [@zhucan](https://github.com/zhucan))

### CSI External-Snapshotter Sidecar

- build with Go 1.15 ([#358](https://github.com/kubernetes-csi/external-snapshotter/pull/358), [@pohly](https://github.com/pohly))
- Updates kubernetes dependencies to v1.19.0. ([#372](https://github.com/kubernetes-csi/external-snapshotter/pull/372), [@humblec](https://github.com/humblec))
- Publishing of images on k8s.gcr.io ([#319](https://github.com/kubernetes-csi/external-snapshotter/pull/319), [@pohly](https://github.com/pohly))

### CRDs

- Update volume snapshot CRDs to apiextensions.k8s.io/v1 version. ([#367](https://github.com/kubernetes-csi/external-snapshotter/pull/367), [@prateekpandey14](https://github.com/prateekpandey14))

## Dependencies

### Added
- bitbucket.org/bertimus9/systemstat: 0eeff89
- cloud.google.com/go/bigquery: v1.0.1
- cloud.google.com/go/datastore: v1.0.0
- cloud.google.com/go/pubsub: v1.0.1
- cloud.google.com/go/storage: v1.0.0
- dmitri.shuralyov.com/gpu/mtl: 666a987
- github.com/Azure/azure-sdk-for-go: [v43.0.0+incompatible](https://github.com/Azure/azure-sdk-for-go/tree/v43.0.0)
- github.com/Azure/go-ansiterm: [d6e3b33](https://github.com/Azure/go-ansiterm/tree/d6e3b33)
- github.com/Azure/go-autorest/autorest/to: [v0.2.0](https://github.com/Azure/go-autorest/autorest/to/tree/v0.2.0)
- github.com/Azure/go-autorest/autorest/validation: [v0.1.0](https://github.com/Azure/go-autorest/autorest/validation/tree/v0.1.0)
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [7901bc8](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/tree/7901bc8)
- github.com/JeffAshton/win_pdh: [76bb4ee](https://github.com/JeffAshton/win_pdh/tree/76bb4ee)
- github.com/MakeNowJust/heredoc: [bb23615](https://github.com/MakeNowJust/heredoc/tree/bb23615)
- github.com/Microsoft/go-winio: [fc70bd9](https://github.com/Microsoft/go-winio/tree/fc70bd9)
- github.com/Microsoft/hcsshim: [5eafd15](https://github.com/Microsoft/hcsshim/tree/5eafd15)
- github.com/OneOfOne/xxhash: [v1.2.2](https://github.com/OneOfOne/xxhash/tree/v1.2.2)
- github.com/agnivade/levenshtein: [v1.0.1](https://github.com/agnivade/levenshtein/tree/v1.0.1)
- github.com/ajstarks/svgo: [644b8db](https://github.com/ajstarks/svgo/tree/644b8db)
- github.com/andreyvit/diff: [c7f18ee](https://github.com/andreyvit/diff/tree/c7f18ee)
- github.com/armon/circbuf: [bbbad09](https://github.com/armon/circbuf/tree/bbbad09)
- github.com/armon/consul-api: [eb2c6b5](https://github.com/armon/consul-api/tree/eb2c6b5)
- github.com/asaskevich/govalidator: [f61b66f](https://github.com/asaskevich/govalidator/tree/f61b66f)
- github.com/auth0/go-jwt-middleware: [5493cab](https://github.com/auth0/go-jwt-middleware/tree/5493cab)
- github.com/aws/aws-sdk-go: [v1.28.2](https://github.com/aws/aws-sdk-go/tree/v1.28.2)
- github.com/bgentry/speakeasy: [v0.1.0](https://github.com/bgentry/speakeasy/tree/v0.1.0)
- github.com/bifurcation/mint: [93c51c6](https://github.com/bifurcation/mint/tree/93c51c6)
- github.com/boltdb/bolt: [v1.3.1](https://github.com/boltdb/bolt/tree/v1.3.1)
- github.com/caddyserver/caddy: [v1.0.3](https://github.com/caddyserver/caddy/tree/v1.0.3)
- github.com/cenkalti/backoff: [v2.1.1+incompatible](https://github.com/cenkalti/backoff/tree/v2.1.1)
- github.com/cespare/xxhash/v2: [v2.1.1](https://github.com/cespare/xxhash/v2/tree/v2.1.1)
- github.com/cespare/xxhash: [v1.1.0](https://github.com/cespare/xxhash/tree/v1.1.0)
- github.com/chai2010/gettext-go: [c6fed77](https://github.com/chai2010/gettext-go/tree/c6fed77)
- github.com/checkpoint-restore/go-criu/v4: [v4.0.2](https://github.com/checkpoint-restore/go-criu/v4/tree/v4.0.2)
- github.com/cheekybits/genny: [9127e81](https://github.com/cheekybits/genny/tree/9127e81)
- github.com/chzyer/logex: [v1.1.10](https://github.com/chzyer/logex/tree/v1.1.10)
- github.com/chzyer/readline: [2972be2](https://github.com/chzyer/readline/tree/2972be2)
- github.com/chzyer/test: [a1ea475](https://github.com/chzyer/test/tree/a1ea475)
- github.com/cilium/ebpf: [1c8d4c9](https://github.com/cilium/ebpf/tree/1c8d4c9)
- github.com/clusterhq/flocker-go: [2b8b725](https://github.com/clusterhq/flocker-go/tree/2b8b725)
- github.com/cncf/udpa/go: [269d4d4](https://github.com/cncf/udpa/go/tree/269d4d4)
- github.com/cockroachdb/datadriven: [80d97fb](https://github.com/cockroachdb/datadriven/tree/80d97fb)
- github.com/codegangsta/negroni: [v1.0.0](https://github.com/codegangsta/negroni/tree/v1.0.0)
- github.com/containerd/cgroups: [0dbf7f0](https://github.com/containerd/cgroups/tree/0dbf7f0)
- github.com/containerd/console: [v1.0.0](https://github.com/containerd/console/tree/v1.0.0)
- github.com/containerd/containerd: [v1.3.3](https://github.com/containerd/containerd/tree/v1.3.3)
- github.com/containerd/continuity: [aaeac12](https://github.com/containerd/continuity/tree/aaeac12)
- github.com/containerd/fifo: [a9fb20d](https://github.com/containerd/fifo/tree/a9fb20d)
- github.com/containerd/go-runc: [5a6d9f3](https://github.com/containerd/go-runc/tree/5a6d9f3)
- github.com/containerd/ttrpc: [v1.0.0](https://github.com/containerd/ttrpc/tree/v1.0.0)
- github.com/containerd/typeurl: [v1.0.0](https://github.com/containerd/typeurl/tree/v1.0.0)
- github.com/containernetworking/cni: [v0.8.0](https://github.com/containernetworking/cni/tree/v0.8.0)
- github.com/coredns/corefile-migration: [v1.0.10](https://github.com/coredns/corefile-migration/tree/v1.0.10)
- github.com/coreos/bbolt: [v1.3.2](https://github.com/coreos/bbolt/tree/v1.3.2)
- github.com/coreos/etcd: [v3.3.10+incompatible](https://github.com/coreos/etcd/tree/v3.3.10)
- github.com/coreos/go-oidc: [v2.1.0+incompatible](https://github.com/coreos/go-oidc/tree/v2.1.0)
- github.com/coreos/go-semver: [v0.3.0](https://github.com/coreos/go-semver/tree/v0.3.0)
- github.com/coreos/go-systemd/v22: [v22.1.0](https://github.com/coreos/go-systemd/v22/tree/v22.1.0)
- github.com/coreos/go-systemd: [95778df](https://github.com/coreos/go-systemd/tree/95778df)
- github.com/coreos/pkg: [399ea9e](https://github.com/coreos/pkg/tree/399ea9e)
- github.com/cpuguy83/go-md2man/v2: [v2.0.0](https://github.com/cpuguy83/go-md2man/v2/tree/v2.0.0)
- github.com/creack/pty: [v1.1.7](https://github.com/creack/pty/tree/v1.1.7)
- github.com/cyphar/filepath-securejoin: [v0.2.2](https://github.com/cyphar/filepath-securejoin/tree/v0.2.2)
- github.com/daviddengcn/go-colortext: [511bcaf](https://github.com/daviddengcn/go-colortext/tree/511bcaf)
- github.com/dgryski/go-sip13: [e10d5fe](https://github.com/dgryski/go-sip13/tree/e10d5fe)
- github.com/dnaeon/go-vcr: [v1.0.1](https://github.com/dnaeon/go-vcr/tree/v1.0.1)
- github.com/docker/distribution: [v2.7.1+incompatible](https://github.com/docker/distribution/tree/v2.7.1)
- github.com/docker/docker: [aa6a989](https://github.com/docker/docker/tree/aa6a989)
- github.com/docker/go-connections: [v0.4.0](https://github.com/docker/go-connections/tree/v0.4.0)
- github.com/docker/go-units: [v0.4.0](https://github.com/docker/go-units/tree/v0.4.0)
- github.com/docopt/docopt-go: [ee0de3b](https://github.com/docopt/docopt-go/tree/ee0de3b)
- github.com/dustin/go-humanize: [v1.0.0](https://github.com/dustin/go-humanize/tree/v1.0.0)
- github.com/euank/go-kmsg-parser: [v2.0.0+incompatible](https://github.com/euank/go-kmsg-parser/tree/v2.0.0)
- github.com/exponent-io/jsonpath: [d6023ce](https://github.com/exponent-io/jsonpath/tree/d6023ce)
- github.com/fatih/camelcase: [v1.0.0](https://github.com/fatih/camelcase/tree/v1.0.0)
- github.com/fatih/color: [v1.7.0](https://github.com/fatih/color/tree/v1.7.0)
- github.com/flynn/go-shlex: [3f9db97](https://github.com/flynn/go-shlex/tree/3f9db97)
- github.com/fogleman/gg: [0403632](https://github.com/fogleman/gg/tree/0403632)
- github.com/globalsign/mgo: [eeefdec](https://github.com/globalsign/mgo/tree/eeefdec)
- github.com/go-acme/lego: [v2.5.0+incompatible](https://github.com/go-acme/lego/tree/v2.5.0)
- github.com/go-bindata/go-bindata: [v3.1.1+incompatible](https://github.com/go-bindata/go-bindata/tree/v3.1.1)
- github.com/go-gl/glfw/v3.3/glfw: [12ad95a](https://github.com/go-gl/glfw/v3.3/glfw/tree/12ad95a)
- github.com/go-ini/ini: [v1.9.0](https://github.com/go-ini/ini/tree/v1.9.0)
- github.com/go-openapi/analysis: [v0.19.5](https://github.com/go-openapi/analysis/tree/v0.19.5)
- github.com/go-openapi/errors: [v0.19.2](https://github.com/go-openapi/errors/tree/v0.19.2)
- github.com/go-openapi/loads: [v0.19.4](https://github.com/go-openapi/loads/tree/v0.19.4)
- github.com/go-openapi/runtime: [v0.19.4](https://github.com/go-openapi/runtime/tree/v0.19.4)
- github.com/go-openapi/strfmt: [v0.19.3](https://github.com/go-openapi/strfmt/tree/v0.19.3)
- github.com/go-openapi/validate: [v0.19.5](https://github.com/go-openapi/validate/tree/v0.19.5)
- github.com/go-ozzo/ozzo-validation: [v3.5.0+incompatible](https://github.com/go-ozzo/ozzo-validation/tree/v3.5.0)
- github.com/godbus/dbus/v5: [v5.0.3](https://github.com/godbus/dbus/v5/tree/v5.0.3)
- github.com/golang/freetype: [e2365df](https://github.com/golang/freetype/tree/e2365df)
- github.com/golangplus/bytes: [45c989f](https://github.com/golangplus/bytes/tree/45c989f)
- github.com/golangplus/fmt: [2a5d6d7](https://github.com/golangplus/fmt/tree/2a5d6d7)
- github.com/golangplus/testing: [af21d9c](https://github.com/golangplus/testing/tree/af21d9c)
- github.com/google/cadvisor: [v0.37.0](https://github.com/google/cadvisor/tree/v0.37.0)
- github.com/google/renameio: [v0.1.0](https://github.com/google/renameio/tree/v0.1.0)
- github.com/gopherjs/gopherjs: [0766667](https://github.com/gopherjs/gopherjs/tree/0766667)
- github.com/gorilla/context: [v1.1.1](https://github.com/gorilla/context/tree/v1.1.1)
- github.com/gorilla/mux: [v1.7.3](https://github.com/gorilla/mux/tree/v1.7.3)
- github.com/gorilla/websocket: [v1.4.0](https://github.com/gorilla/websocket/tree/v1.4.0)
- github.com/grpc-ecosystem/go-grpc-middleware: [f849b54](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/f849b54)
- github.com/grpc-ecosystem/go-grpc-prometheus: [v1.2.0](https://github.com/grpc-ecosystem/go-grpc-prometheus/tree/v1.2.0)
- github.com/grpc-ecosystem/grpc-gateway: [v1.9.5](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.9.5)
- github.com/hashicorp/go-syslog: [v1.0.0](https://github.com/hashicorp/go-syslog/tree/v1.0.0)
- github.com/hashicorp/hcl: [v1.0.0](https://github.com/hashicorp/hcl/tree/v1.0.0)
- github.com/heketi/heketi: [c2e2a4a](https://github.com/heketi/heketi/tree/c2e2a4a)
- github.com/heketi/tests: [f3775cb](https://github.com/heketi/tests/tree/f3775cb)
- github.com/ianlancetaylor/demangle: [5e5cf60](https://github.com/ianlancetaylor/demangle/tree/5e5cf60)
- github.com/inconshreveable/mousetrap: [v1.0.0](https://github.com/inconshreveable/mousetrap/tree/v1.0.0)
- github.com/ishidawataru/sctp: [7c296d4](https://github.com/ishidawataru/sctp/tree/7c296d4)
- github.com/jimstudt/http-authentication: [3eca13d](https://github.com/jimstudt/http-authentication/tree/3eca13d)
- github.com/jmespath/go-jmespath: [c2b33e8](https://github.com/jmespath/go-jmespath/tree/c2b33e8)
- github.com/jonboulle/clockwork: [v0.1.0](https://github.com/jonboulle/clockwork/tree/v0.1.0)
- github.com/jtolds/gls: [v4.20.0+incompatible](https://github.com/jtolds/gls/tree/v4.20.0)
- github.com/jung-kurt/gofpdf: [24315ac](https://github.com/jung-kurt/gofpdf/tree/24315ac)
- github.com/karrick/godirwalk: [v1.7.5](https://github.com/karrick/godirwalk/tree/v1.7.5)
- github.com/klauspost/cpuid: [v1.2.0](https://github.com/klauspost/cpuid/tree/v1.2.0)
- github.com/kylelemons/godebug: [d65d576](https://github.com/kylelemons/godebug/tree/d65d576)
- github.com/libopenstorage/openstorage: [v1.0.0](https://github.com/libopenstorage/openstorage/tree/v1.0.0)
- github.com/liggitt/tabwriter: [89fcab3](https://github.com/liggitt/tabwriter/tree/89fcab3)
- github.com/lithammer/dedent: [v1.1.0](https://github.com/lithammer/dedent/tree/v1.1.0)
- github.com/lpabon/godbc: [v0.1.1](https://github.com/lpabon/godbc/tree/v0.1.1)
- github.com/lucas-clemente/aes12: [cd47fb3](https://github.com/lucas-clemente/aes12/tree/cd47fb3)
- github.com/lucas-clemente/quic-clients: [v0.1.0](https://github.com/lucas-clemente/quic-clients/tree/v0.1.0)
- github.com/lucas-clemente/quic-go-certificates: [d2f8652](https://github.com/lucas-clemente/quic-go-certificates/tree/d2f8652)
- github.com/lucas-clemente/quic-go: [v0.10.2](https://github.com/lucas-clemente/quic-go/tree/v0.10.2)
- github.com/magiconair/properties: [v1.8.1](https://github.com/magiconair/properties/tree/v1.8.1)
- github.com/marten-seemann/qtls: [v0.2.3](https://github.com/marten-seemann/qtls/tree/v0.2.3)
- github.com/mattn/go-colorable: [v0.0.9](https://github.com/mattn/go-colorable/tree/v0.0.9)
- github.com/mattn/go-isatty: [v0.0.4](https://github.com/mattn/go-isatty/tree/v0.0.4)
- github.com/mattn/go-runewidth: [v0.0.2](https://github.com/mattn/go-runewidth/tree/v0.0.2)
- github.com/mholt/certmagic: [6a42ef9](https://github.com/mholt/certmagic/tree/6a42ef9)
- github.com/miekg/dns: [v1.1.4](https://github.com/miekg/dns/tree/v1.1.4)
- github.com/mindprince/gonvml: [9ebdce4](https://github.com/mindprince/gonvml/tree/9ebdce4)
- github.com/mistifyio/go-zfs: [f784269](https://github.com/mistifyio/go-zfs/tree/f784269)
- github.com/mitchellh/go-homedir: [v1.1.0](https://github.com/mitchellh/go-homedir/tree/v1.1.0)
- github.com/mitchellh/go-wordwrap: [v1.0.0](https://github.com/mitchellh/go-wordwrap/tree/v1.0.0)
- github.com/mitchellh/mapstructure: [v1.1.2](https://github.com/mitchellh/mapstructure/tree/v1.1.2)
- github.com/moby/ipvs: [v1.0.1](https://github.com/moby/ipvs/tree/v1.0.1)
- github.com/moby/sys/mountinfo: [v0.1.3](https://github.com/moby/sys/mountinfo/tree/v0.1.3)
- github.com/moby/term: [672ec06](https://github.com/moby/term/tree/672ec06)
- github.com/mohae/deepcopy: [491d360](https://github.com/mohae/deepcopy/tree/491d360)
- github.com/morikuni/aec: [v1.0.0](https://github.com/morikuni/aec/tree/v1.0.0)
- github.com/mrunalp/fileutils: [abd8a0e](https://github.com/mrunalp/fileutils/tree/abd8a0e)
- github.com/mvdan/xurls: [v1.1.0](https://github.com/mvdan/xurls/tree/v1.1.0)
- github.com/naoina/go-stringutil: [v0.1.0](https://github.com/naoina/go-stringutil/tree/v0.1.0)
- github.com/naoina/toml: [v0.1.1](https://github.com/naoina/toml/tree/v0.1.1)
- github.com/oklog/ulid: [v1.3.1](https://github.com/oklog/ulid/tree/v1.3.1)
- github.com/olekukonko/tablewriter: [a0225b3](https://github.com/olekukonko/tablewriter/tree/a0225b3)
- github.com/opencontainers/go-digest: [v1.0.0-rc1](https://github.com/opencontainers/go-digest/tree/v1.0.0-rc1)
- github.com/opencontainers/image-spec: [v1.0.1](https://github.com/opencontainers/image-spec/tree/v1.0.1)
- github.com/opencontainers/runc: [819fcc6](https://github.com/opencontainers/runc/tree/819fcc6)
- github.com/opencontainers/runtime-spec: [237cc4f](https://github.com/opencontainers/runtime-spec/tree/237cc4f)
- github.com/opencontainers/selinux: [v1.5.2](https://github.com/opencontainers/selinux/tree/v1.5.2)
- github.com/pborman/uuid: [v1.2.0](https://github.com/pborman/uuid/tree/v1.2.0)
- github.com/pelletier/go-toml: [v1.2.0](https://github.com/pelletier/go-toml/tree/v1.2.0)
- github.com/pquerna/cachecontrol: [0dec1b3](https://github.com/pquerna/cachecontrol/tree/0dec1b3)
- github.com/prometheus/tsdb: [v0.7.1](https://github.com/prometheus/tsdb/tree/v0.7.1)
- github.com/quobyte/api: [v0.1.2](https://github.com/quobyte/api/tree/v0.1.2)
- github.com/robfig/cron: [v1.1.0](https://github.com/robfig/cron/tree/v1.1.0)
- github.com/rogpeppe/fastuuid: [6724a57](https://github.com/rogpeppe/fastuuid/tree/6724a57)
- github.com/rogpeppe/go-internal: [v1.3.0](https://github.com/rogpeppe/go-internal/tree/v1.3.0)
- github.com/rubiojr/go-vhd: [02e2102](https://github.com/rubiojr/go-vhd/tree/02e2102)
- github.com/russross/blackfriday/v2: [v2.0.1](https://github.com/russross/blackfriday/v2/tree/v2.0.1)
- github.com/russross/blackfriday: [v1.5.2](https://github.com/russross/blackfriday/tree/v1.5.2)
- github.com/satori/go.uuid: [v1.2.0](https://github.com/satori/go.uuid/tree/v1.2.0)
- github.com/seccomp/libseccomp-golang: [v0.9.1](https://github.com/seccomp/libseccomp-golang/tree/v0.9.1)
- github.com/sergi/go-diff: [v1.0.0](https://github.com/sergi/go-diff/tree/v1.0.0)
- github.com/shurcooL/sanitized_anchor_name: [v1.0.0](https://github.com/shurcooL/sanitized_anchor_name/tree/v1.0.0)
- github.com/smartystreets/assertions: [b2de0cb](https://github.com/smartystreets/assertions/tree/b2de0cb)
- github.com/smartystreets/goconvey: [v1.6.4](https://github.com/smartystreets/goconvey/tree/v1.6.4)
- github.com/soheilhy/cmux: [v0.1.4](https://github.com/soheilhy/cmux/tree/v0.1.4)
- github.com/spaolacci/murmur3: [f09979e](https://github.com/spaolacci/murmur3/tree/f09979e)
- github.com/spf13/cast: [v1.3.0](https://github.com/spf13/cast/tree/v1.3.0)
- github.com/spf13/cobra: [v1.0.0](https://github.com/spf13/cobra/tree/v1.0.0)
- github.com/spf13/jwalterweatherman: [v1.1.0](https://github.com/spf13/jwalterweatherman/tree/v1.1.0)
- github.com/spf13/viper: [v1.4.0](https://github.com/spf13/viper/tree/v1.4.0)
- github.com/storageos/go-api: [343b3ef](https://github.com/storageos/go-api/tree/343b3ef)
- github.com/syndtr/gocapability: [d983527](https://github.com/syndtr/gocapability/tree/d983527)
- github.com/thecodeteam/goscaleio: [v0.1.0](https://github.com/thecodeteam/goscaleio/tree/v0.1.0)
- github.com/tidwall/pretty: [v1.0.0](https://github.com/tidwall/pretty/tree/v1.0.0)
- github.com/tmc/grpc-websocket-proxy: [0ad062e](https://github.com/tmc/grpc-websocket-proxy/tree/0ad062e)
- github.com/ugorji/go: [v1.1.4](https://github.com/ugorji/go/tree/v1.1.4)
- github.com/urfave/cli: [v1.22.2](https://github.com/urfave/cli/tree/v1.22.2)
- github.com/urfave/negroni: [v1.0.0](https://github.com/urfave/negroni/tree/v1.0.0)
- github.com/vektah/gqlparser: [v1.1.2](https://github.com/vektah/gqlparser/tree/v1.1.2)
- github.com/vishvananda/netlink: [v1.1.0](https://github.com/vishvananda/netlink/tree/v1.1.0)
- github.com/vishvananda/netns: [52d707b](https://github.com/vishvananda/netns/tree/52d707b)
- github.com/vmware/govmomi: [v0.20.3](https://github.com/vmware/govmomi/tree/v0.20.3)
- github.com/xiang90/probing: [43a291a](https://github.com/xiang90/probing/tree/43a291a)
- github.com/xlab/handysort: [fb3537e](https://github.com/xlab/handysort/tree/fb3537e)
- github.com/xordataexchange/crypt: [b2862e3](https://github.com/xordataexchange/crypt/tree/b2862e3)
- github.com/yuin/goldmark: [v1.1.27](https://github.com/yuin/goldmark/tree/v1.1.27)
- go.etcd.io/bbolt: v1.3.5
- go.etcd.io/etcd: 17cef6e
- go.mongodb.org/mongo-driver: v1.1.2
- go.uber.org/atomic: v1.4.0
- go.uber.org/multierr: v1.1.0
- go.uber.org/zap: v1.10.0
- golang.org/x/mod: v0.3.0
- gonum.org/v1/plot: e2840ee
- google.golang.org/protobuf: v1.24.0
- gopkg.in/cheggaaa/pb.v1: v1.0.25
- gopkg.in/errgo.v2: v2.1.0
- gopkg.in/gcfg.v1: v1.2.0
- gopkg.in/mcuadros/go-syslog.v2: v2.2.1
- gopkg.in/natefinch/lumberjack.v2: v2.0.0
- gopkg.in/resty.v1: v1.12.0
- gopkg.in/square/go-jose.v2: v2.2.2
- gopkg.in/warnings.v0: v0.1.1
- gotest.tools/v3: v3.0.2
- gotest.tools: v2.2.0+incompatible
- k8s.io/apiextensions-apiserver: v0.19.0
- k8s.io/apiserver: v0.19.0
- k8s.io/cli-runtime: v0.19.0
- k8s.io/cloud-provider: v0.19.0
- k8s.io/cluster-bootstrap: v0.19.0
- k8s.io/cri-api: v0.19.0
- k8s.io/csi-translation-lib: v0.19.0
- k8s.io/heapster: v1.2.0-beta.1
- k8s.io/klog/v2: v2.2.0
- k8s.io/kube-aggregator: v0.19.0
- k8s.io/kube-controller-manager: v0.19.0
- k8s.io/kube-proxy: v0.19.0
- k8s.io/kube-scheduler: v0.19.0
- k8s.io/kubectl: v0.19.0
- k8s.io/kubelet: v0.19.0
- k8s.io/legacy-cloud-providers: v0.19.0
- k8s.io/metrics: v0.19.0
- k8s.io/sample-apiserver: v0.19.0
- k8s.io/system-validators: v1.1.2
- rsc.io/binaryregexp: v0.2.0
- rsc.io/pdf: v0.1.1
- rsc.io/quote/v3: v3.1.0
- rsc.io/sampler: v1.3.0
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.9
- sigs.k8s.io/kustomize: v2.0.3+incompatible
- sigs.k8s.io/structured-merge-diff/v4: v4.0.1
- vbom.ml/util: db5cfe1

### Changed
- cloud.google.com/go: v0.38.0 → v0.51.0
- github.com/Azure/go-autorest/autorest/adal: [v0.5.0 → v0.8.2](https://github.com/Azure/go-autorest/autorest/adal/compare/v0.5.0...v0.8.2)
- github.com/Azure/go-autorest/autorest/date: [v0.1.0 → v0.2.0](https://github.com/Azure/go-autorest/autorest/date/compare/v0.1.0...v0.2.0)
- github.com/Azure/go-autorest/autorest/mocks: [v0.2.0 → v0.3.0](https://github.com/Azure/go-autorest/autorest/mocks/compare/v0.2.0...v0.3.0)
- github.com/Azure/go-autorest/autorest: [v0.9.0 → v0.9.6](https://github.com/Azure/go-autorest/autorest/compare/v0.9.0...v0.9.6)
- github.com/alecthomas/template: [a0175ee → fb15b89](https://github.com/alecthomas/template/compare/a0175ee...fb15b89)
- github.com/alecthomas/units: [2efee85 → c3de453](https://github.com/alecthomas/units/compare/2efee85...c3de453)
- github.com/beorn7/perks: [v1.0.0 → v1.0.1](https://github.com/beorn7/perks/compare/v1.0.0...v1.0.1)
- github.com/elazarl/goproxy: [c4fc265 → 947c36d](https://github.com/elazarl/goproxy/compare/c4fc265...947c36d)
- github.com/envoyproxy/go-control-plane: [5f8ba28 → v0.9.4](https://github.com/envoyproxy/go-control-plane/compare/5f8ba28...v0.9.4)
- github.com/evanphx/json-patch: [v4.5.0+incompatible → v4.9.0+incompatible](https://github.com/evanphx/json-patch/compare/v4.5.0...v4.9.0)
- github.com/fsnotify/fsnotify: [v1.4.7 → v1.4.9](https://github.com/fsnotify/fsnotify/compare/v1.4.7...v1.4.9)
- github.com/ghodss/yaml: [73d445a → v1.0.0](https://github.com/ghodss/yaml/compare/73d445a...v1.0.0)
- github.com/go-kit/kit: [v0.8.0 → v0.9.0](https://github.com/go-kit/kit/compare/v0.8.0...v0.9.0)
- github.com/go-logfmt/logfmt: [v0.3.0 → v0.4.0](https://github.com/go-logfmt/logfmt/compare/v0.3.0...v0.4.0)
- github.com/go-logr/logr: [v0.1.0 → v0.2.0](https://github.com/go-logr/logr/compare/v0.1.0...v0.2.0)
- github.com/gogo/protobuf: [65acae2 → v1.3.1](https://github.com/gogo/protobuf/compare/65acae2...v1.3.1)
- github.com/golang/groupcache: [5b532d6 → 215e871](https://github.com/golang/groupcache/compare/5b532d6...215e871)
- github.com/golang/mock: [v1.2.0 → v1.4.3](https://github.com/golang/mock/compare/v1.2.0...v1.4.3)
- github.com/golang/protobuf: [v1.3.2 → v1.4.2](https://github.com/golang/protobuf/compare/v1.3.2...v1.4.2)
- github.com/google/go-cmp: [v0.3.1 → v0.4.0](https://github.com/google/go-cmp/compare/v0.3.1...v0.4.0)
- github.com/google/gofuzz: [v1.0.0 → v1.1.0](https://github.com/google/gofuzz/compare/v1.0.0...v1.1.0)
- github.com/google/pprof: [3ea8567 → d4f498a](https://github.com/google/pprof/compare/3ea8567...d4f498a)
- github.com/googleapis/gax-go/v2: [v2.0.4 → v2.0.5](https://github.com/googleapis/gax-go/v2/compare/v2.0.4...v2.0.5)
- github.com/googleapis/gnostic: [v0.2.0 → v0.4.1](https://github.com/googleapis/gnostic/compare/v0.2.0...v0.4.1)
- github.com/imdario/mergo: [v0.3.7 → v0.3.9](https://github.com/imdario/mergo/compare/v0.3.7...v0.3.9)
- github.com/json-iterator/go: [v1.1.8 → v1.1.10](https://github.com/json-iterator/go/compare/v1.1.8...v1.1.10)
- github.com/jstemmer/go-junit-report: [af01ea7 → v0.9.1](https://github.com/jstemmer/go-junit-report/compare/af01ea7...v0.9.1)
- github.com/konsorten/go-windows-terminal-sequences: [v1.0.1 → v1.0.3](https://github.com/konsorten/go-windows-terminal-sequences/compare/v1.0.1...v1.0.3)
- github.com/kr/pretty: [v0.1.0 → v0.2.0](https://github.com/kr/pretty/compare/v0.1.0...v0.2.0)
- github.com/matttproud/golang_protobuf_extensions: [v1.0.1 → c182aff](https://github.com/matttproud/golang_protobuf_extensions/compare/v1.0.1...c182aff)
- github.com/munnerz/goautoneg: [a547fc6 → a7dc8b6](https://github.com/munnerz/goautoneg/compare/a547fc6...a7dc8b6)
- github.com/onsi/ginkgo: [v1.10.2 → v1.11.0](https://github.com/onsi/ginkgo/compare/v1.10.2...v1.11.0)
- github.com/pkg/errors: [v0.8.1 → v0.9.1](https://github.com/pkg/errors/compare/v0.8.1...v0.9.1)
- github.com/prometheus/client_golang: [v1.0.0 → v1.7.1](https://github.com/prometheus/client_golang/compare/v1.0.0...v1.7.1)
- github.com/prometheus/client_model: [14fe0d1 → v0.2.0](https://github.com/prometheus/client_model/compare/14fe0d1...v0.2.0)
- github.com/prometheus/common: [v0.4.1 → v0.10.0](https://github.com/prometheus/common/compare/v0.4.1...v0.10.0)
- github.com/prometheus/procfs: [v0.0.2 → v0.1.3](https://github.com/prometheus/procfs/compare/v0.0.2...v0.1.3)
- github.com/sirupsen/logrus: [v1.2.0 → v1.6.0](https://github.com/sirupsen/logrus/compare/v1.2.0...v1.6.0)
- go.opencensus.io: v0.21.0 → v0.22.2
- golang.org/x/crypto: 60c769a → 75b2880
- golang.org/x/exp: 4b39c73 → da58074
- golang.org/x/image: 0694c2d → cff245a
- golang.org/x/lint: d0100b6 → fdd1cda
- golang.org/x/mobile: d3739f8 → d2bd2a2
- golang.org/x/net: c0dbc17 → ab34263
- golang.org/x/oauth2: 0f29369 → bf48bf1
- golang.org/x/sync: 1122301 → cd5d95a
- golang.org/x/sys: 0732a99 → ed371f2
- golang.org/x/text: v0.3.2 → v0.3.3
- golang.org/x/time: 9d24e82 → 89c76fb
- golang.org/x/tools: 5eefd05 → c1934b7
- golang.org/x/xerrors: a985d34 → 9bdfabe
- gonum.org/v1/gonum: 3d26580 → v0.6.2
- google.golang.org/api: v0.4.0 → v0.15.1
- google.golang.org/appengine: v1.5.0 → v1.6.5
- google.golang.org/genproto: 5c49e3e → cb27e3a
- google.golang.org/grpc: v1.26.0 → v1.28.0
- gopkg.in/check.v1: 788fd78 → 41f04d3
- gopkg.in/yaml.v2: v2.2.4 → v2.2.8
- honnef.co/go/tools: ea95bdf → v0.0.1-2019.2.3
- k8s.io/api: v0.17.0 → v0.19.0
- k8s.io/apimachinery: v0.17.1-beta.0 → v0.19.0
- k8s.io/client-go: v0.17.0 → v0.19.0
- k8s.io/code-generator: v0.17.1-beta.0 → v0.19.0
- k8s.io/component-base: v0.17.0 → v0.19.0
- k8s.io/gengo: 26a6646 → 8167cfd
- k8s.io/kube-openapi: 30be4d1 → 6aeccd4
- k8s.io/kubernetes: v1.14.0 → v1.19.0
- k8s.io/utils: e782cd3 → d5654de
- sigs.k8s.io/yaml: v1.1.0 → v1.2.0

### Removed
- sigs.k8s.io/structured-merge-diff: 15d366b
