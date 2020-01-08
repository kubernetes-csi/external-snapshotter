module github.com/kubernetes-csi/external-snapshotter

go 1.12

require (
	github.com/container-storage-interface/spec v1.1.0
	github.com/golang/mock v1.2.0
	github.com/golang/protobuf v1.3.2
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.7.0
	github.com/kubernetes-csi/csi-test v2.0.0+incompatible
	google.golang.org/grpc v1.26.0
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.1-beta.0
	k8s.io/client-go v0.17.0
	k8s.io/code-generator v0.0.0-20191121015212-c4c8f8345c7e
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.14.0
)

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.17.0

replace k8s.io/apiserver => k8s.io/apiserver v0.17.0

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.17.0

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.17.0

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.17.0

replace k8s.io/code-generator => k8s.io/code-generator v0.17.1-beta.0

replace k8s.io/component-base => k8s.io/component-base v0.17.0

replace k8s.io/cri-api => k8s.io/cri-api v0.17.1-beta.0

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.17.0

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.17.0

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.17.0

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.17.0

replace k8s.io/kubectl => k8s.io/kubectl v0.17.0

replace k8s.io/kubelet => k8s.io/kubelet v0.17.0

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.17.0

replace k8s.io/metrics => k8s.io/metrics v0.17.0

replace k8s.io/node-api => k8s.io/node-api v0.17.0

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.17.0

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.17.0

replace k8s.io/sample-controller => k8s.io/sample-controller v0.17.0
