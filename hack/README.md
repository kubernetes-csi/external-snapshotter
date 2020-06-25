# Scripts User Guide

This README documents:
* What update-crd.sh and update-generated-code.sh do
* When and how to use them

## update-generated-code.sh

This is the script to update clientset/informers/listers and API deepcopy code using [code-generator](https://github.com/kubernetes/code-generator).

Make sure to run this script after making changes to /client/apis/volumesnapshot/v1beta1/types.go.

To run this script, Run: ./hack/update-generated-code.sh from the project root directory.

Once you run the script, you got the output as:
```bash
Generating deepcopy funcs
Generating clientset for volumesnapshot:v1beta1 at github.com/kubernetes-csi/external-snapshotter/client/v2/clientset
Generating listers for volumesnapshot:v1beta1 at github.com/kubernetes-csi/external-snapshotter/client/v2/listers
Generating informers for volumesnapshot:v1beta1 at github.com/kubernetes-csi/external-snapshotter/client/v2/informers

```


## update-crd.sh

This is the script to update CRD yaml files under /client/config/crd/ based on types.go file.

Make sure to run this script after making changes to /client/apis/volumesnapshot/v1beta1/types.go.

Follow these steps to update the CRD:

* Run ./hack/update-crd.sh from client directory, new yaml files should have been created under ./config/crd/

* Add api-approved.kubernetes.io annotation value in all yaml files in the metadata section with the PR where the API is approved by the API reviewers. The current approved PR for snapshot beta API is https://github.com/kubernetes-csi/external-snapshotter/pull/139. Refer to https://github.com/kubernetes/enhancements/pull/1111 for details about this annotation.

* Remove any metadata sections from the yaml file which does not belong to the generated type.
For example, the following command will add a metadata section for a nested object, remove any newly added metadata sections. TODO(xiangqian): this is to make sure the generated CRD is compatible with apiextensions.k8s.io/v1. Once controller-gen supports generating CRD with apiextensions.k8s.io/v1, switch to use the correct version of controller-gen and remove the last step from this README.
```bash
./hack/update-crd.sh; git diff
+        metadata:
+          description: 'Standard object''s metadata. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata'
```
* Update the restoreSize property to string in snapshot.storage.k8s.io_volumesnapshots.yaml  

```bash
    
@@ -160,7 +157,7 @@ spec:
                 of a snapshot is unknown.
               type: boolean
             restoreSize:
-              - type: string
+              type: string
               description: restoreSize represents the complete size of the snapshot
                 in bytes. In dynamic snapshot creation case, this field will be filled
                 in with the "size_bytes" value returned from CSI "CreateSnapshotRequest"

```

