# Validating Webhook

The snapshot validating webhook is an HTTP callback which responds to [admission requests](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/). It is part of a larger [plan](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/1900-volume-snapshot-validation-webhook#proposal) to tighten validation for volume snapshot objects. This webhook introduces the [ratcheting validation](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/1900-volume-snapshot-validation-webhook#backwards-compatibility) mechanism targeting the tighter validation. The cluster admin or Kubernetes distribution admin should install the webhook alongside the snapshot controllers and CRDs.

## How to build the webhook

Build the binary

```bash
make 
```

Build the docker image

```bash
docker build -t snapshot-validation-webhook:latest -f ./cmd/snapshot-validation-webhook/Dockerfile .
```

## How to deploy the webhook

The webhook server is provided as an image which can be built from this repository. It can be deployed anywhere, as long as the api server is able to reach it over HTTPS. It is recommended to deploy the webhook server in the cluster as snapshotting is latency sensitive. A `ValidatingWebhookConfiguration` object is needed to configure the api server to contact the webhook server. Please see the [documentation](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) for more details. The webhook server code is adapted from the [webhook server](https://github.com/kubernetes/kubernetes/tree/v1.18.6/test/images/agnhost/webhook) used in the kubernetes/kubernetes end to end testing code.

### Example in-cluster deployment using Kubernetes Secrets

Please note this is not considered to be a production ready method to deploy the certificates and is only provided for demo purposes. This is only one of many ways to deploy the certificates, it is your responsibility to ensure the security of your cluster. TLS certificates and private keys should be handled with care and you may not want to keep them in plain Kubernetes secrets.

This method was heavily adapted from [banzai cloud](https://banzaicloud.com/blog/k8s-admission-webhooks/).

#### Method

These commands should be run from the top level directory.

1. Run the `create-cert.sh` script. Note using the default namespace will allow anyone with access to that namespace to read your secret. It is recommended to change the namespace in all the files and the commands given below.


    ```bash
    # This script will create a TLS certificate signed by the [cluster](https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/). It will place the public and private key into a secret on the cluster.
    ./deploy/kubernetes/webhook-example/create-cert.sh --service snapshot-validation-service --secret snapshot-validation-secret --namespace default # Make sure to use a different namespace
    ```

2. Patch the `ValidatingWebhookConfiguration` file from the template, filling in the CA bundle field.

    ```bash
    cat ./deploy/kubernetes/webhook-example/admission-configuration-template | ./deploy/kubernetes/webhook-example/patch-ca-bundle.sh > ./deploy/kubernetes/webhook-example/admission-configuration.yaml
    ```

3. Change the namespace in the generated `admission-configuration.yaml` file. Change the namespace in the service and deployment in the `webhook.yaml` file.

4. Create the deployment, service, RBAC, and admission configuration objects on the cluster.

    ```bash
    kubectl apply -f ./deploy/kubernetes/webhook-example
    ```

Once all the pods from the deployment are up and running, you should be ready to go.

#### Verify the webhook works

Try to create an invalid snapshot object, the snapshot creation should fail.

```bash
kubectl create -f ./examples/kubernetes/invalid-snapshot-v1.yaml
```

### Other methods to deploy the webhook server

Look into [cert-manager](https://cert-manager.io/) to handle the certificates, and this kube-builder [tutorial](https://book.kubebuilder.io/cronjob-tutorial/cert-manager.html) on how to deploy a webhook.

#### Important

Please see the deployment [yaml](./webhook.yaml) for the arguments expected by the webhook server. The snapshot validation webhook is served at the path `/volumesnapshot`.
