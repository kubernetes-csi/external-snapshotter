module github.com/kubernetes-csi/external-snapshotter

go 1.12

require (
	github.com/container-storage-interface/spec v1.1.0
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/mock v1.2.0
	github.com/golang/protobuf v1.3.2
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.6.1
	github.com/kubernetes-csi/csi-test v2.0.0+incompatible
	google.golang.org/grpc v1.23.0
	k8s.io/api v0.0.0-20191122220107-b5267f2975e0
	k8s.io/apimachinery v0.0.0-20191121175448-79c2a76c473a
	k8s.io/client-go v0.0.0-20191122220542-ed16ecbdf3a0
	k8s.io/code-generator v0.0.0-20191121015212-c4c8f8345c7e
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.14.0
)
