#!/bin/bash

ROOT=$(cd $(dirname $0)/../../; pwd)

set -o errexit
set -o nounset
set -o pipefail

CA_BUNDLE=$( kubectl get secret snapshot-conversion-webhook-secret -o json | jq -r '.data."tls.crt"' )
JSON_PATCH="{\"spec\":{\"conversion\": {\"webhook\": {\"clientConfig\": {\"caBundle\": \"${CA_BUNDLE}\"}}}}}"

kubectl patch crd volumegroupsnapshotcontents.groupsnapshot.storage.k8s.io -p "${JSON_PATCH}"
