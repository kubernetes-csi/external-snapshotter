# Scripts User Guide

This README documents:
* What update-crd.sh and update-generated-code.sh do
* When and how to use them

## update-generated-code.sh

This is the script to update clientset/informers/listers and API deepcopy code using [code-generator](https://github.com/kubernetes/code-generator).

Make sure to run this script after making changes to /client/apis/volumesnapshot/v1/types.go.

### Pre-requisites for running update-generated-code.sh:

* Set `GOPATH`
    ```bash
    export GOPATH=~/go
    ```

* Ensure external-snapshotter repository is at `~/go/src/github.com/kubernetes-csi/external-snapshotter`

* Clone code-generator 
    ```bash
    cd ~/go/src/k8s.io
    git clone https://github.com/kubernetes/code-generator.git 
    ```
* Checkout latest release version
    ```bash
    git checkout v0.29.1
    ```

* Ensure the file `generate-groups.sh` exists

    ```bash
    ls ${GOPATH}/src/k8s.io/code-generator/generate-groups.sh
    ```
  
Update generated client code in external-snapshotter
    
```bash
    cd ~/go/src/github.com/kubernetes-csi/external-snapshotter/client
    ./hack/update-generated-code.sh
``` 

Once you run the script, you will get an output as follows:
    
```bash
    Generating deepcopy funcs
    Generating clientset for volumesnapshot:v1 at github.com/kubernetes-csi/external-snapshotter/client/v7/clientset
    Generating listers for volumesnapshot:v1 at github.com/kubernetes-csi/external-snapshotter/client/v7/listers
    Generating informers for volumesnapshot:v1 at github.com/kubernetes-csi/external-snapshotter/client/v7/informers
    
```

## update-crd.sh

NOTE: We need to keep both v1beta1 and v1 snapshot APIs but set served and storage version of v1beta1 to false. Please copy back the v1beta1 manifest back to the files as this script will remove it.

This is the script to update CRD yaml files under /client/config/crd/ based on types.go file.

Make sure to run this script after making changes to /client/apis.

Follow these steps to update the CRD:

* Run ./hack/update-crd.sh from client directory, new yaml files should have been created under ./config/crd/

* Add api-approved.kubernetes.io annotation value in all yaml files in the metadata section with the PR where the API is approved by the API reviewers. Refer to https://github.com/kubernetes/enhancements/pull/1111 for details about this annotation.

* Update the restoreSize property to string in snapshot.storage.k8s.io_volumesnapshots.yaml

The generated yaml file contains restoreSize property anyOf as described below: 
 
```bash
            restoreSize:
              anyOf:
              - type: integer
              - type: string
              description: restoreSize represents the complete size of the snapshot
                in bytes. In dynamic snapshot creation case, this field will be filled
                in with the "size_bytes" value returned from CSI "CreateSnapshotRequest"
                gRPC call. For a pre-existing snapshot, this field will be filled
                with the "size_bytes" value returned from the CSI "ListSnapshots"
                gRPC call if the driver supports it. When restoring a volume from
                this snapshot, the size of the volume MUST NOT be smaller than the
                restoreSize if it is specified, otherwise the restoration will fail.
                If not specified, it indicates that the size is unknown.
```

Update the restoreSize property to use type string only:

```bash
   
            restoreSize:
              type: string
              description: restoreSize represents the complete size of the snapshot
                in bytes. In dynamic snapshot creation case, this field will be filled
                in with the "size_bytes" value returned from CSI "CreateSnapshotRequest"
                gRPC call. For a pre-existing snapshot, this field will be filled
                with the "size_bytes" value returned from the CSI "ListSnapshots"
                gRPC call if the driver supports it. When restoring a volume from
                this snapshot, the size of the volume MUST NOT be smaller than the
                restoreSize if it is specified, otherwise the restoration will fail.
                If not specified, it indicates that the size is unknown.

```

* Add the VolumeSnapshot namespace to the `additionalPrinterColumns` section in `client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml`. Refer https://github.com/kubernetes-csi/external-snapshotter/pull/535 for more details.
