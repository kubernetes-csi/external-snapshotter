#!/bin/bash
# File originally from https://github.com/banzaicloud/admission-webhook-example/blob/blog/deployment/webhook-patch-ca-bundle.sh

ROOT=$(cd $(dirname $0)/../../; pwd)

set -o errexit
set -o nounset
set -o pipefail

CA_BUNDLE=$(kubectl config view --raw -o json | jq -r '.clusters[0].cluster."certificate-authority-data"' | tr -d '"')
JSON_PATCH="{\"spec\":{\"conversion\": {\"webhook\": {\"clientConfig\": {\"caBundle\": \"${CA_BUNDLE}\"}}}}}"

kubectl patch crd volumegroupsnapshotclasses.groupsnapshot.storage.k8s.io -p "${JSON_PATCH}"
kubectl patch crd volumegroupsnapshotcontents.groupsnapshot.storage.k8s.io -p "${JSON_PATCH}"
kubectl patch crd volumegroupsnapshots.groupsnapshot.storage.k8s.io -p "${JSON_PATCH}"
