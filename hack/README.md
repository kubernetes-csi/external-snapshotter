# Scripts User Guide

This README documents:
* What update-crd.sh and update-generated-code.sh do
* When and how to use them

## update-generated-code.sh

This is the script to update clientset/informers/listers and API deepcopy code using [code-generator](https://github.com/kubernetes/code-generator).

Make sure to run this script after making changes to /pkg/apis/volumesnapshot/v1beta1/types.go.

To run this script, you have to patch it:
```patch
diff --git a/hack/update-generated-code.sh b/hack/update-generated-code.sh
--- a/hack/update-generated-code.sh
+++ b/hack/update-generated-code.sh
@@ -27,7 +27,7 @@ CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-ge
 #                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
 #                  instead of the $GOPATH directly. For normal projects this can be dropped.
 bash ${CODEGEN_PKG}/generate-groups.sh "deepcopy,client,informer,lister" \
-  github.com/kubernetes-csi/external-snapshotter/pkg/client github.com/kubernetes-csi/external-snapshotter/pkg/apis \
+  github.com/kubernetes-csi/external-snapshotter/v2/pkg/client github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis \
   volumesnapshot:v1beta1 \
   --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt
```

Once you are done with patching, continue:
```bash
rm -fr v2/
ln -sfvn $(pwd) v2
rm -fr pkg/client
```

Run: ./hack/update-generated-code.sh from the project root directory.

Do not forget to revert previously applied workaround:
```bash
git checkout -- v2/
git checkout -- hack/update-generated-code.sh
```

## update-crd.sh

This is the script to update CRD yaml files under ./config/crd/ based on types.go file.

Make sure to run this script after making changes to /pkg/apis/volumesnapshot/v1beta1/types.go.

Follow these steps to update the CRD:

* Run ./hack/update-crd.sh from root directory, new yaml files should have been created under ./config/crd/

* Replace `api-approved.kubernetes.io` annotation value in all yaml files in the metadata section with your PR.
For example, `api-approved.kubernetes.io: "https://github.com/kubernetes-csi/external-snapshotter/pull/YOUR-PULL-REQUEST-#"`

* Remove any metadata sections from the yaml file which does not belong to the generated type.
For example, the following command will add a metadata section for a nested object, remove any newly added metadata sections. TODO(xiangqian): this is to make sure the generated CRD is compatible with apiextensions.k8s.io/v1. Once controller-gen supports generating CRD with apiextensions.k8s.io/v1, switch to use the correct version of controller-gen and remove the last step from this README.
```bash
./hack/update-crd.sh; git diff
+        metadata:
+          description: 'Standard object''s metadata. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata'
```
