# Release notes for v6.3.2

[Documentation](https://kubernetes-csi.github.io)

## Changes by Kind

### Bug or Regression

- Bump google.golang.org/grpc from v1.58.0 to v1.59.0 to fix CVE-2023-44487. ([#954](https://github.com/kubernetes-csi/external-snapshotter/pull/954), [@songjiaxun](https://github.com/songjiaxun))

### Uncategorized

- Cherry-pick from PR 876: Update VolumeSnapshot and VolumeSnapshotContent using JSON patch ([#974](https://github.com/kubernetes-csi/external-snapshotter/pull/974), [@phoenix-bjoern](https://github.com/phoenix-bjoern))

## Dependencies

### Added
_Nothing has changed._

### Changed
- cloud.google.com/go/compute: v1.21.0 → v1.23.0
- github.com/golang/glog: [v1.1.0 → v1.1.2](https://github.com/golang/glog/compare/v1.1.0...v1.1.2)
- github.com/google/uuid: [v1.3.0 → v1.3.1](https://github.com/google/uuid/compare/v1.3.0...v1.3.1)
- github.com/kubernetes-csi/csi-lib-utils: [v0.14.0 → v0.14.1](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.14.0...v0.14.1)
- golang.org/x/oauth2: v0.10.0 → v0.11.0
- google.golang.org/genproto/googleapis/api: 782d3b1 → b8732ec
- google.golang.org/genproto/googleapis/rpc: 782d3b1 → b8732ec
- google.golang.org/genproto: 782d3b1 → b8732ec
- google.golang.org/grpc: v1.58.0 → v1.59.0

### Removed
_Nothing has changed._

# Release notes for v6.3.2

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v6.3.1

## Changes by Kind

### Bug or Regression

- Bump google.golang.org/grpc from v1.58.0 to v1.59.0 to fix CVE-2023-44487. ([#954](https://github.com/kubernetes-csi/external-snapshotter/pull/954), [@songjiaxun](https://github.com/songjiaxun))

## Dependencies

### Added
_Nothing has changed._

### Changed
- cloud.google.com/go/compute: v1.21.0 → v1.23.0
- github.com/golang/glog: [v1.1.0 → v1.1.2](https://github.com/golang/glog/compare/v1.1.0...v1.1.2)
- github.com/google/uuid: [v1.3.0 → v1.3.1](https://github.com/google/uuid/compare/v1.3.0...v1.3.1)
- golang.org/x/oauth2: v0.10.0 → v0.11.0
- google.golang.org/genproto/googleapis/api: 782d3b1 → b8732ec
- google.golang.org/genproto/googleapis/rpc: 782d3b1 → b8732ec
- google.golang.org/genproto: 782d3b1 → b8732ec
- google.golang.org/grpc: v1.58.0 → v1.59.0

### Removed
_Nothing has changed._

# Release notes for v6.3.1

[Documentation](https://kubernetes-csi.github.io)

# Changelog since v6.3.0

## Changes by Kind

### Uncategorized

- CVE fixes: CVE-2023-44487,  CVE-2023-3978 ([#938](https://github.com/kubernetes-csi/external-snapshotter/pull/938), [@dannawang0221](https://github.com/dannawang0221))
- Webhooks for group snapshot CRs will be disabled by default. Command line argument `enable-volume-group-snapshot-webhook` needs to be added to enable these webhooks. ([#940](https://github.com/kubernetes-csi/external-snapshotter/pull/940), [@k8s-infra-cherrypick-robot](https://github.com/k8s-infra-cherrypick-robot))

## Dependencies

### Added
_Nothing has changed._

### Changed
- golang.org/x/crypto: v0.11.0 → v0.14.0
- golang.org/x/net: v0.13.0 → v0.17.0
- golang.org/x/sys: v0.10.0 → v0.13.0
- golang.org/x/term: v0.10.0 → v0.13.0
- golang.org/x/text: v0.11.0 → v0.13.0

### Removed
_Nothing has changed._

# Release notes for v6.3.0

# Changelog since v6.2.0

## Changes by Kind

### Feature

- Add a marker to the snapshot-controller manifests to enable feature gates in CSI prow jobs. ([#790](https://github.com/kubernetes-csi/external-snapshotter/pull/790), [@RaunakShah](https://github.com/RaunakShah))

### Bug or Regression

- Fix a problem in CSI snapshotter sidecar that constantly retries CreateSnapshot call on error without exponential backoff. Comparing old and new object is not working as expected ([#871](https://github.com/kubernetes-csi/external-snapshotter/pull/871), [@sameshai](https://github.com/sameshai))
- Fix: CVE-2022-41723 ([#824](https://github.com/kubernetes-csi/external-snapshotter/pull/824), [@andyzhangx](https://github.com/andyzhangx))

### Other (Cleanup or Flake)

- Update Kubernetes deps to 0.28.0 and update generated code in the client ([#902](https://github.com/kubernetes-csi/external-snapshotter/pull/902), [@xing-yang](https://github.com/xing-yang))

### Uncategorized

- Update kubernetes dependencies to v1.28.0 ([#899](https://github.com/kubernetes-csi/external-snapshotter/pull/899), [@Sneha-at](https://github.com/Sneha-at))

## Dependencies

### Added
- cloud.google.com/go/compute/metadata: v0.2.3
- github.com/alecthomas/kingpin/v2: [v2.3.2](https://github.com/alecthomas/kingpin/v2/tree/v2.3.2)
- github.com/creack/pty: [v1.1.9](https://github.com/creack/pty/tree/v1.1.9)
- github.com/google/gnostic-models: [v0.6.8](https://github.com/google/gnostic-models/tree/v0.6.8)
- github.com/nxadm/tail: [v1.4.8](https://github.com/nxadm/tail/tree/v1.4.8)
- github.com/xhit/go-str2duration/v2: [v2.1.0](https://github.com/xhit/go-str2duration/v2/tree/v2.1.0)
- google.golang.org/genproto/googleapis/api: 782d3b1
- google.golang.org/genproto/googleapis/rpc: 782d3b1

### Changed
- cloud.google.com/go/compute: v1.7.0 → v1.21.0
- cloud.google.com/go: v0.105.0 → v0.34.0
- github.com/NYTimes/gziphandler: [v1.1.1 → 56545f4](https://github.com/NYTimes/gziphandler/compare/v1.1.1...56545f4)
- github.com/alecthomas/units: [f65c72e → b94a6e3](https://github.com/alecthomas/units/compare/f65c72e...b94a6e3)
- github.com/cenkalti/backoff/v4: [v4.1.3 → v4.2.1](https://github.com/cenkalti/backoff/v4/compare/v4.1.3...v4.2.1)
- github.com/census-instrumentation/opencensus-proto: [v0.2.1 → v0.4.1](https://github.com/census-instrumentation/opencensus-proto/compare/v0.2.1...v0.4.1)
- github.com/cespare/xxhash/v2: [v2.1.2 → v2.2.0](https://github.com/cespare/xxhash/v2/compare/v2.1.2...v2.2.0)
- github.com/cncf/udpa/go: [04548b0 → c52dc94](https://github.com/cncf/udpa/go/compare/04548b0...c52dc94)
- github.com/cncf/xds/go: [cb28da3 → e9ce688](https://github.com/cncf/xds/go/compare/cb28da3...e9ce688)
- github.com/container-storage-interface/spec: [v1.7.0 → v1.8.0](https://github.com/container-storage-interface/spec/compare/v1.7.0...v1.8.0)
- github.com/emicklei/go-restful/v3: [v3.9.0 → v3.10.1](https://github.com/emicklei/go-restful/v3/compare/v3.9.0...v3.10.1)
- github.com/envoyproxy/go-control-plane: [49ff273 → v0.11.1](https://github.com/envoyproxy/go-control-plane/compare/49ff273...v0.11.1)
- github.com/envoyproxy/protoc-gen-validate: [v0.1.0 → v1.0.2](https://github.com/envoyproxy/protoc-gen-validate/compare/v0.1.0...v1.0.2)
- github.com/evanphx/json-patch: [v4.12.0+incompatible → v5.7.0+incompatible](https://github.com/evanphx/json-patch/compare/v4.12.0...v5.7.0)
- github.com/go-kit/log: [v0.2.0 → v0.2.1](https://github.com/go-kit/log/compare/v0.2.0...v0.2.1)
- github.com/go-logr/logr: [v1.2.3 → v1.2.4](https://github.com/go-logr/logr/compare/v1.2.3...v1.2.4)
- github.com/go-openapi/jsonpointer: [v0.19.5 → v0.19.6](https://github.com/go-openapi/jsonpointer/compare/v0.19.5...v0.19.6)
- github.com/go-openapi/jsonreference: [v0.20.0 → v0.20.2](https://github.com/go-openapi/jsonreference/compare/v0.20.0...v0.20.2)
- github.com/go-task/slim-sprig: [348f09d → 52ccab3](https://github.com/go-task/slim-sprig/compare/348f09d...52ccab3)
- github.com/golang/glog: [23def4e → v1.1.0](https://github.com/golang/glog/compare/23def4e...v1.1.0)
- github.com/golang/protobuf: [v1.5.2 → v1.5.3](https://github.com/golang/protobuf/compare/v1.5.2...v1.5.3)
- github.com/google/uuid: [v1.1.2 → v1.3.0](https://github.com/google/uuid/compare/v1.1.2...v1.3.0)
- github.com/ianlancetaylor/demangle: [5e5cf60 → 28f6c0f](https://github.com/ianlancetaylor/demangle/compare/5e5cf60...28f6c0f)
- github.com/inconshreveable/mousetrap: [v1.0.1 → v1.1.0](https://github.com/inconshreveable/mousetrap/compare/v1.0.1...v1.1.0)
- github.com/kr/pretty: [v0.2.0 → v0.3.1](https://github.com/kr/pretty/compare/v0.2.0...v0.3.1)
- github.com/kubernetes-csi/csi-lib-utils: [v0.12.0 → v0.14.0](https://github.com/kubernetes-csi/csi-lib-utils/compare/v0.12.0...v0.14.0)
- github.com/kubernetes-csi/csi-test/v4: [v4.0.2 → v4.4.0](https://github.com/kubernetes-csi/csi-test/v4/compare/v4.0.2...v4.4.0)
- github.com/moby/term: [39b0c02 → 1aeaba8](https://github.com/moby/term/compare/39b0c02...1aeaba8)
- github.com/onsi/ginkgo/v2: [v2.4.0 → v2.9.4](https://github.com/onsi/ginkgo/v2/compare/v2.4.0...v2.9.4)
- github.com/onsi/ginkgo: [v1.10.3 → v1.16.5](https://github.com/onsi/ginkgo/compare/v1.10.3...v1.16.5)
- github.com/onsi/gomega: [v1.23.0 → v1.27.6](https://github.com/onsi/gomega/compare/v1.23.0...v1.27.6)
- github.com/prometheus/client_golang: [v1.14.0 → v1.16.0](https://github.com/prometheus/client_golang/compare/v1.14.0...v1.16.0)
- github.com/prometheus/client_model: [v0.3.0 → v0.4.0](https://github.com/prometheus/client_model/compare/v0.3.0...v0.4.0)
- github.com/prometheus/common: [v0.37.0 → v0.44.0](https://github.com/prometheus/common/compare/v0.37.0...v0.44.0)
- github.com/prometheus/procfs: [v0.8.0 → v0.10.1](https://github.com/prometheus/procfs/compare/v0.8.0...v0.10.1)
- github.com/rogpeppe/go-internal: [v1.3.0 → v1.10.0](https://github.com/rogpeppe/go-internal/compare/v1.3.0...v1.10.0)
- github.com/spf13/cobra: [v1.6.1 → v1.7.0](https://github.com/spf13/cobra/compare/v1.6.1...v1.7.0)
- github.com/stretchr/objx: [v0.4.0 → v0.5.0](https://github.com/stretchr/objx/compare/v0.4.0...v0.5.0)
- github.com/stretchr/testify: [v1.8.0 → v1.8.2](https://github.com/stretchr/testify/compare/v1.8.0...v1.8.2)
- github.com/yuin/goldmark: [v1.4.13 → v1.3.5](https://github.com/yuin/goldmark/compare/v1.4.13...v1.3.5)
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.35.0 → v0.35.1
- go.uber.org/atomic: v1.7.0 → v1.10.0
- go.uber.org/goleak: v1.2.0 → v1.2.1
- go.uber.org/multierr: v1.6.0 → v1.11.0
- golang.org/x/crypto: v0.1.0 → v0.11.0
- golang.org/x/exp: 6cc2880 → 509febe
- golang.org/x/lint: 738671d → d0100b6
- golang.org/x/mod: v0.6.0 → v0.8.0
- golang.org/x/net: v0.4.0 → v0.13.0
- golang.org/x/oauth2: v0.1.0 → v0.10.0
- golang.org/x/sync: 886fb93 → v0.3.0
- golang.org/x/sys: v0.3.0 → v0.10.0
- golang.org/x/term: v0.3.0 → v0.10.0
- golang.org/x/text: v0.5.0 → v0.11.0
- golang.org/x/time: v0.1.0 → v0.3.0
- golang.org/x/tools: v0.2.0 → v0.8.0
- golang.org/x/xerrors: 5ec99f8 → 04be3eb
- google.golang.org/genproto: 115e99e → 782d3b1
- google.golang.org/grpc: v1.50.1 → v1.58.0
- google.golang.org/protobuf: v1.28.1 → v1.31.0
- honnef.co/go/tools: v0.0.1-2020.1.4 → ea95bdf
- k8s.io/api: v0.26.0 → v0.28.0
- k8s.io/apimachinery: v0.26.0 → v0.28.0
- k8s.io/client-go: v0.26.0 → v0.28.0
- k8s.io/code-generator: v0.26.0 → v0.28.0
- k8s.io/component-base: v0.26.0 → v0.28.0
- k8s.io/component-helpers: v0.26.0 → v0.28.0
- k8s.io/gengo: 3913671 → fad74ee
- k8s.io/klog/v2: v2.80.1 → v2.100.1
- k8s.io/kube-openapi: 172d655 → 2695361
- k8s.io/utils: 1a15be2 → d93618c
- sigs.k8s.io/json: f223a00 → bc3834c

### Removed
- bitbucket.org/bertimus9/systemstat: v0.5.0
- cloud.google.com/go/accessapproval: v1.4.0
- cloud.google.com/go/accesscontextmanager: v1.3.0
- cloud.google.com/go/aiplatform: v1.24.0
- cloud.google.com/go/analytics: v0.12.0
- cloud.google.com/go/apigateway: v1.3.0
- cloud.google.com/go/apigeeconnect: v1.3.0
- cloud.google.com/go/appengine: v1.4.0
- cloud.google.com/go/area120: v0.6.0
- cloud.google.com/go/artifactregistry: v1.8.0
- cloud.google.com/go/asset: v1.9.0
- cloud.google.com/go/assuredworkloads: v1.8.0
- cloud.google.com/go/automl: v1.7.0
- cloud.google.com/go/baremetalsolution: v0.3.0
- cloud.google.com/go/batch: v0.3.0
- cloud.google.com/go/beyondcorp: v0.2.0
- cloud.google.com/go/bigquery: v1.42.0
- cloud.google.com/go/billing: v1.6.0
- cloud.google.com/go/binaryauthorization: v1.3.0
- cloud.google.com/go/certificatemanager: v1.3.0
- cloud.google.com/go/channel: v1.8.0
- cloud.google.com/go/cloudbuild: v1.3.0
- cloud.google.com/go/clouddms: v1.3.0
- cloud.google.com/go/cloudtasks: v1.7.0
- cloud.google.com/go/contactcenterinsights: v1.3.0
- cloud.google.com/go/container: v1.6.0
- cloud.google.com/go/containeranalysis: v0.6.0
- cloud.google.com/go/datacatalog: v1.7.0
- cloud.google.com/go/dataflow: v0.7.0
- cloud.google.com/go/dataform: v0.4.0
- cloud.google.com/go/datafusion: v1.4.0
- cloud.google.com/go/datalabeling: v0.6.0
- cloud.google.com/go/dataplex: v1.3.0
- cloud.google.com/go/dataproc: v1.7.0
- cloud.google.com/go/dataqna: v0.6.0
- cloud.google.com/go/datastore: v1.1.0
- cloud.google.com/go/datastream: v1.4.0
- cloud.google.com/go/deploy: v1.4.0
- cloud.google.com/go/dialogflow: v1.18.0
- cloud.google.com/go/dlp: v1.6.0
- cloud.google.com/go/documentai: v1.9.0
- cloud.google.com/go/domains: v0.7.0
- cloud.google.com/go/edgecontainer: v0.2.0
- cloud.google.com/go/essentialcontacts: v1.3.0
- cloud.google.com/go/eventarc: v1.7.0
- cloud.google.com/go/filestore: v1.3.0
- cloud.google.com/go/functions: v1.8.0
- cloud.google.com/go/gaming: v1.7.0
- cloud.google.com/go/gkebackup: v0.2.0
- cloud.google.com/go/gkeconnect: v0.6.0
- cloud.google.com/go/gkehub: v0.10.0
- cloud.google.com/go/gkemulticloud: v0.3.0
- cloud.google.com/go/gsuiteaddons: v1.3.0
- cloud.google.com/go/iam: v0.6.0
- cloud.google.com/go/iap: v1.4.0
- cloud.google.com/go/ids: v1.1.0
- cloud.google.com/go/iot: v1.3.0
- cloud.google.com/go/kms: v1.5.0
- cloud.google.com/go/language: v1.7.0
- cloud.google.com/go/lifesciences: v0.6.0
- cloud.google.com/go/longrunning: v0.1.1
- cloud.google.com/go/managedidentities: v1.3.0
- cloud.google.com/go/mediatranslation: v0.6.0
- cloud.google.com/go/memcache: v1.6.0
- cloud.google.com/go/metastore: v1.7.0
- cloud.google.com/go/monitoring: v1.7.0
- cloud.google.com/go/networkconnectivity: v1.6.0
- cloud.google.com/go/networkmanagement: v1.4.0
- cloud.google.com/go/networksecurity: v0.6.0
- cloud.google.com/go/notebooks: v1.4.0
- cloud.google.com/go/optimization: v1.1.0
- cloud.google.com/go/orchestration: v1.3.0
- cloud.google.com/go/orgpolicy: v1.4.0
- cloud.google.com/go/osconfig: v1.9.0
- cloud.google.com/go/oslogin: v1.6.0
- cloud.google.com/go/phishingprotection: v0.6.0
- cloud.google.com/go/policytroubleshooter: v1.3.0
- cloud.google.com/go/privatecatalog: v0.6.0
- cloud.google.com/go/pubsub: v1.3.1
- cloud.google.com/go/recaptchaenterprise/v2: v2.4.0
- cloud.google.com/go/recommendationengine: v0.6.0
- cloud.google.com/go/recommender: v1.7.0
- cloud.google.com/go/redis: v1.9.0
- cloud.google.com/go/resourcemanager: v1.3.0
- cloud.google.com/go/resourcesettings: v1.3.0
- cloud.google.com/go/retail: v1.10.0
- cloud.google.com/go/run: v0.2.0
- cloud.google.com/go/scheduler: v1.6.0
- cloud.google.com/go/secretmanager: v1.8.0
- cloud.google.com/go/security: v1.9.0
- cloud.google.com/go/securitycenter: v1.15.0
- cloud.google.com/go/servicecontrol: v1.4.0
- cloud.google.com/go/servicedirectory: v1.6.0
- cloud.google.com/go/servicemanagement: v1.4.0
- cloud.google.com/go/serviceusage: v1.3.0
- cloud.google.com/go/shell: v1.3.0
- cloud.google.com/go/speech: v1.8.0
- cloud.google.com/go/storage: v1.10.0
- cloud.google.com/go/storagetransfer: v1.5.0
- cloud.google.com/go/talent: v1.3.0
- cloud.google.com/go/texttospeech: v1.4.0
- cloud.google.com/go/tpu: v1.3.0
- cloud.google.com/go/trace: v1.3.0
- cloud.google.com/go/translate: v1.3.0
- cloud.google.com/go/video: v1.8.0
- cloud.google.com/go/videointelligence: v1.8.0
- cloud.google.com/go/vision/v2: v2.4.0
- cloud.google.com/go/vmmigration: v1.2.0
- cloud.google.com/go/vpcaccess: v1.4.0
- cloud.google.com/go/webrisk: v1.6.0
- cloud.google.com/go/websecurityscanner: v1.3.0
- cloud.google.com/go/workflows: v1.8.0
- dmitri.shuralyov.com/gpu/mtl: 666a987
- github.com/Azure/azure-sdk-for-go: [v55.0.0+incompatible](https://github.com/Azure/azure-sdk-for-go/tree/v55.0.0)
- github.com/Azure/go-autorest/autorest/adal: [v0.9.20](https://github.com/Azure/go-autorest/autorest/adal/tree/v0.9.20)
- github.com/Azure/go-autorest/autorest/date: [v0.3.0](https://github.com/Azure/go-autorest/autorest/date/tree/v0.3.0)
- github.com/Azure/go-autorest/autorest/mocks: [v0.4.2](https://github.com/Azure/go-autorest/autorest/mocks/tree/v0.4.2)
- github.com/Azure/go-autorest/autorest/to: [v0.4.0](https://github.com/Azure/go-autorest/autorest/to/tree/v0.4.0)
- github.com/Azure/go-autorest/autorest/validation: [v0.1.0](https://github.com/Azure/go-autorest/autorest/validation/tree/v0.1.0)
- github.com/Azure/go-autorest/autorest: [v0.11.27](https://github.com/Azure/go-autorest/autorest/tree/v0.11.27)
- github.com/Azure/go-autorest/logger: [v0.2.1](https://github.com/Azure/go-autorest/logger/tree/v0.2.1)
- github.com/Azure/go-autorest/tracing: [v0.6.0](https://github.com/Azure/go-autorest/tracing/tree/v0.6.0)
- github.com/Azure/go-autorest: [v14.2.0+incompatible](https://github.com/Azure/go-autorest/tree/v14.2.0)
- github.com/BurntSushi/xgb: [27f1227](https://github.com/BurntSushi/xgb/tree/27f1227)
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [f118173](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/tree/f118173)
- github.com/JeffAshton/win_pdh: [76bb4ee](https://github.com/JeffAshton/win_pdh/tree/76bb4ee)
- github.com/MakeNowJust/heredoc: [v1.0.0](https://github.com/MakeNowJust/heredoc/tree/v1.0.0)
- github.com/Microsoft/go-winio: [v0.4.17](https://github.com/Microsoft/go-winio/tree/v0.4.17)
- github.com/Microsoft/hcsshim: [v0.8.22](https://github.com/Microsoft/hcsshim/tree/v0.8.22)
- github.com/OneOfOne/xxhash: [v1.2.2](https://github.com/OneOfOne/xxhash/tree/v1.2.2)
- github.com/PuerkitoBio/purell: [v1.1.1](https://github.com/PuerkitoBio/purell/tree/v1.1.1)
- github.com/PuerkitoBio/urlesc: [de5bf2a](https://github.com/PuerkitoBio/urlesc/tree/de5bf2a)
- github.com/alecthomas/template: [fb15b89](https://github.com/alecthomas/template/tree/fb15b89)
- github.com/antlr/antlr4/runtime/Go/antlr: [v1.4.10](https://github.com/antlr/antlr4/runtime/Go/antlr/tree/v1.4.10)
- github.com/armon/circbuf: [bbbad09](https://github.com/armon/circbuf/tree/bbbad09)
- github.com/aws/aws-sdk-go: [v1.44.116](https://github.com/aws/aws-sdk-go/tree/v1.44.116)
- github.com/buger/jsonparser: [v1.1.1](https://github.com/buger/jsonparser/tree/v1.1.1)
- github.com/cespare/xxhash: [v1.1.0](https://github.com/cespare/xxhash/tree/v1.1.0)
- github.com/chai2010/gettext-go: [v1.0.2](https://github.com/chai2010/gettext-go/tree/v1.0.2)
- github.com/checkpoint-restore/go-criu/v5: [v5.3.0](https://github.com/checkpoint-restore/go-criu/v5/tree/v5.3.0)
- github.com/cilium/ebpf: [v0.7.0](https://github.com/cilium/ebpf/tree/v0.7.0)
- github.com/containerd/cgroups: [v1.0.1](https://github.com/containerd/cgroups/tree/v1.0.1)
- github.com/containerd/console: [v1.0.3](https://github.com/containerd/console/tree/v1.0.3)
- github.com/containerd/ttrpc: [v1.1.0](https://github.com/containerd/ttrpc/tree/v1.1.0)
- github.com/coredns/caddy: [v1.1.0](https://github.com/coredns/caddy/tree/v1.1.0)
- github.com/coredns/corefile-migration: [v1.0.17](https://github.com/coredns/corefile-migration/tree/v1.0.17)
- github.com/coreos/go-oidc: [v2.1.0+incompatible](https://github.com/coreos/go-oidc/tree/v2.1.0)
- github.com/coreos/go-semver: [v0.3.0](https://github.com/coreos/go-semver/tree/v0.3.0)
- github.com/coreos/go-systemd/v22: [v22.3.2](https://github.com/coreos/go-systemd/v22/tree/v22.3.2)
- github.com/cyphar/filepath-securejoin: [v0.2.3](https://github.com/cyphar/filepath-securejoin/tree/v0.2.3)
- github.com/daviddengcn/go-colortext: [v1.0.0](https://github.com/daviddengcn/go-colortext/tree/v1.0.0)
- github.com/docker/distribution: [v2.8.1+incompatible](https://github.com/docker/distribution/tree/v2.8.1)
- github.com/docker/go-units: [v0.5.0](https://github.com/docker/go-units/tree/v0.5.0)
- github.com/docopt/docopt-go: [ee0de3b](https://github.com/docopt/docopt-go/tree/ee0de3b)
- github.com/dustin/go-humanize: [v1.0.0](https://github.com/dustin/go-humanize/tree/v1.0.0)
- github.com/elazarl/goproxy: [947c36d](https://github.com/elazarl/goproxy/tree/947c36d)
- github.com/euank/go-kmsg-parser: [v2.0.0+incompatible](https://github.com/euank/go-kmsg-parser/tree/v2.0.0)
- github.com/exponent-io/jsonpath: [d6023ce](https://github.com/exponent-io/jsonpath/tree/d6023ce)
- github.com/fatih/camelcase: [v1.0.0](https://github.com/fatih/camelcase/tree/v1.0.0)
- github.com/flowstack/go-jsonschema: [v0.1.1](https://github.com/flowstack/go-jsonschema/tree/v0.1.1)
- github.com/form3tech-oss/jwt-go: [v3.2.3+incompatible](https://github.com/form3tech-oss/jwt-go/tree/v3.2.3)
- github.com/fvbommel/sortorder: [v1.0.1](https://github.com/fvbommel/sortorder/tree/v1.0.1)
- github.com/go-errors/errors: [v1.0.1](https://github.com/go-errors/errors/tree/v1.0.1)
- github.com/go-gl/glfw/v3.3/glfw: [6f7a984](https://github.com/go-gl/glfw/v3.3/glfw/tree/6f7a984)
- github.com/go-gl/glfw: [e6da0ac](https://github.com/go-gl/glfw/tree/e6da0ac)
- github.com/go-kit/kit: [v0.9.0](https://github.com/go-kit/kit/tree/v0.9.0)
- github.com/go-stack/stack: [v1.8.0](https://github.com/go-stack/stack/tree/v1.8.0)
- github.com/godbus/dbus/v5: [v5.0.6](https://github.com/godbus/dbus/v5/tree/v5.0.6)
- github.com/gofrs/uuid: [v4.0.0+incompatible](https://github.com/gofrs/uuid/tree/v4.0.0)
- github.com/golang-jwt/jwt/v4: [v4.2.0](https://github.com/golang-jwt/jwt/v4/tree/v4.2.0)
- github.com/google/cadvisor: [v0.46.0](https://github.com/google/cadvisor/tree/v0.46.0)
- github.com/google/cel-go: [v0.12.5](https://github.com/google/cel-go/tree/v0.12.5)
- github.com/google/martian/v3: [v3.0.0](https://github.com/google/martian/v3/tree/v3.0.0)
- github.com/google/martian: [v2.1.0+incompatible](https://github.com/google/martian/tree/v2.1.0)
- github.com/google/renameio: [v0.1.0](https://github.com/google/renameio/tree/v0.1.0)
- github.com/google/shlex: [e7afc7f](https://github.com/google/shlex/tree/e7afc7f)
- github.com/googleapis/gax-go/v2: [v2.1.1](https://github.com/googleapis/gax-go/v2/tree/v2.1.1)
- github.com/gorilla/websocket: [v1.4.2](https://github.com/gorilla/websocket/tree/v1.4.2)
- github.com/grpc-ecosystem/go-grpc-middleware: [v1.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/tree/v1.3.0)
- github.com/grpc-ecosystem/go-grpc-prometheus: [v1.2.0](https://github.com/grpc-ecosystem/go-grpc-prometheus/tree/v1.2.0)
- github.com/hashicorp/golang-lru: [v0.5.1](https://github.com/hashicorp/golang-lru/tree/v0.5.1)
- github.com/ishidawataru/sctp: [7c296d4](https://github.com/ishidawataru/sctp/tree/7c296d4)
- github.com/jmespath/go-jmespath: [v0.4.0](https://github.com/jmespath/go-jmespath/tree/v0.4.0)
- github.com/jonboulle/clockwork: [v0.2.2](https://github.com/jonboulle/clockwork/tree/v0.2.2)
- github.com/jstemmer/go-junit-report: [v0.9.1](https://github.com/jstemmer/go-junit-report/tree/v0.9.1)
- github.com/karrick/godirwalk: [v1.17.0](https://github.com/karrick/godirwalk/tree/v1.17.0)
- github.com/konsorten/go-windows-terminal-sequences: [v1.0.3](https://github.com/konsorten/go-windows-terminal-sequences/tree/v1.0.3)
- github.com/kr/logfmt: [b84e30a](https://github.com/kr/logfmt/tree/b84e30a)
- github.com/libopenstorage/openstorage: [v1.0.0](https://github.com/libopenstorage/openstorage/tree/v1.0.0)
- github.com/liggitt/tabwriter: [89fcab3](https://github.com/liggitt/tabwriter/tree/89fcab3)
- github.com/lithammer/dedent: [v1.1.0](https://github.com/lithammer/dedent/tree/v1.1.0)
- github.com/mindprince/gonvml: [9ebdce4](https://github.com/mindprince/gonvml/tree/9ebdce4)
- github.com/mistifyio/go-zfs: [f784269](https://github.com/mistifyio/go-zfs/tree/f784269)
- github.com/mitchellh/go-wordwrap: [v1.0.0](https://github.com/mitchellh/go-wordwrap/tree/v1.0.0)
- github.com/mitchellh/mapstructure: [v1.4.1](https://github.com/mitchellh/mapstructure/tree/v1.4.1)
- github.com/moby/ipvs: [v1.0.1](https://github.com/moby/ipvs/tree/v1.0.1)
- github.com/moby/sys/mountinfo: [v0.6.2](https://github.com/moby/sys/mountinfo/tree/v0.6.2)
- github.com/mohae/deepcopy: [491d360](https://github.com/mohae/deepcopy/tree/491d360)
- github.com/monochromegane/go-gitignore: [205db1a](https://github.com/monochromegane/go-gitignore/tree/205db1a)
- github.com/mrunalp/fileutils: [v0.5.0](https://github.com/mrunalp/fileutils/tree/v0.5.0)
- github.com/niemeyer/pretty: [a10e7ca](https://github.com/niemeyer/pretty/tree/a10e7ca)
- github.com/opencontainers/go-digest: [v1.0.0](https://github.com/opencontainers/go-digest/tree/v1.0.0)
- github.com/opencontainers/runc: [v1.1.4](https://github.com/opencontainers/runc/tree/v1.1.4)
- github.com/opencontainers/runtime-spec: [1c3f411](https://github.com/opencontainers/runtime-spec/tree/1c3f411)
- github.com/opencontainers/selinux: [v1.10.0](https://github.com/opencontainers/selinux/tree/v1.10.0)
- github.com/pquerna/cachecontrol: [v0.1.0](https://github.com/pquerna/cachecontrol/tree/v0.1.0)
- github.com/robertkrimen/otto: [c382bd3](https://github.com/robertkrimen/otto/tree/c382bd3)
- github.com/robfig/cron/v3: [v3.0.1](https://github.com/robfig/cron/v3/tree/v3.0.1)
- github.com/rubiojr/go-vhd: [02e2102](https://github.com/rubiojr/go-vhd/tree/02e2102)
- github.com/seccomp/libseccomp-golang: [f33da4d](https://github.com/seccomp/libseccomp-golang/tree/f33da4d)
- github.com/sirupsen/logrus: [v1.8.1](https://github.com/sirupsen/logrus/tree/v1.8.1)
- github.com/soheilhy/cmux: [v0.1.5](https://github.com/soheilhy/cmux/tree/v0.1.5)
- github.com/spaolacci/murmur3: [f09979e](https://github.com/spaolacci/murmur3/tree/f09979e)
- github.com/stoewer/go-strcase: [v1.2.0](https://github.com/stoewer/go-strcase/tree/v1.2.0)
- github.com/syndtr/gocapability: [42c35b4](https://github.com/syndtr/gocapability/tree/42c35b4)
- github.com/tmc/grpc-websocket-proxy: [e5319fd](https://github.com/tmc/grpc-websocket-proxy/tree/e5319fd)
- github.com/vishvananda/netlink: [v1.1.0](https://github.com/vishvananda/netlink/tree/v1.1.0)
- github.com/vishvananda/netns: [db3c7e5](https://github.com/vishvananda/netns/tree/db3c7e5)
- github.com/vmware/govmomi: [v0.20.3](https://github.com/vmware/govmomi/tree/v0.20.3)
- github.com/xeipuuv/gojsonpointer: [4e3ac27](https://github.com/xeipuuv/gojsonpointer/tree/4e3ac27)
- github.com/xeipuuv/gojsonreference: [bd5ef7b](https://github.com/xeipuuv/gojsonreference/tree/bd5ef7b)
- github.com/xeipuuv/gojsonschema: [v1.2.0](https://github.com/xeipuuv/gojsonschema/tree/v1.2.0)
- github.com/xiang90/probing: [43a291a](https://github.com/xiang90/probing/tree/43a291a)
- github.com/xlab/treeprint: [v1.1.0](https://github.com/xlab/treeprint/tree/v1.1.0)
- go.etcd.io/bbolt: v1.3.6
- go.etcd.io/etcd/api/v3: v3.5.5
- go.etcd.io/etcd/client/pkg/v3: v3.5.5
- go.etcd.io/etcd/client/v2: v2.305.5
- go.etcd.io/etcd/client/v3: v3.5.5
- go.etcd.io/etcd/pkg/v3: v3.5.5
- go.etcd.io/etcd/raft/v3: v3.5.5
- go.etcd.io/etcd/server/v3: v3.5.5
- go.opencensus.io: v0.23.0
- go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful: v0.35.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.35.0
- go.starlark.net: 8dd3e2e
- golang.org/x/image: cff245a
- golang.org/x/mobile: d2bd2a2
- google.golang.org/api: v0.60.0
- gopkg.in/alecthomas/kingpin.v2: v2.2.6
- gopkg.in/errgo.v2: v2.1.0
- gopkg.in/gcfg.v1: v1.2.0
- gopkg.in/natefinch/lumberjack.v2: v2.0.0
- gopkg.in/sourcemap.v1: v1.0.5
- gopkg.in/square/go-jose.v2: v2.2.2
- gopkg.in/warnings.v0: v0.1.1
- gotest.tools/v3: v3.0.3
- k8s.io/apiextensions-apiserver: v0.26.0
- k8s.io/apiserver: v0.26.0
- k8s.io/cli-runtime: v0.26.0
- k8s.io/cloud-provider: v0.26.0
- k8s.io/cluster-bootstrap: v0.26.0
- k8s.io/controller-manager: v0.26.0
- k8s.io/cri-api: v0.26.0
- k8s.io/csi-translation-lib: v0.26.0
- k8s.io/dynamic-resource-allocation: v0.26.0
- k8s.io/klog: v1.0.0
- k8s.io/kms: v0.26.0
- k8s.io/kube-aggregator: v0.26.0
- k8s.io/kube-controller-manager: v0.26.0
- k8s.io/kube-proxy: v0.26.0
- k8s.io/kube-scheduler: v0.26.0
- k8s.io/kubectl: v0.26.0
- k8s.io/kubelet: v0.26.0
- k8s.io/kubernetes: v1.26.0
- k8s.io/legacy-cloud-providers: v0.26.0
- k8s.io/metrics: v0.26.0
- k8s.io/mount-utils: v0.26.0
- k8s.io/pod-security-admission: v0.26.0
- k8s.io/sample-apiserver: v0.26.0
- k8s.io/system-validators: v1.8.0
- rsc.io/binaryregexp: v0.2.0
- rsc.io/quote/v3: v3.1.0
- rsc.io/sampler: v1.3.0
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.33
- sigs.k8s.io/kustomize/api: v0.12.1
- sigs.k8s.io/kustomize/kustomize/v4: v4.5.7
- sigs.k8s.io/kustomize/kyaml: v0.13.9
