#!/bin/bash

# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(unset CDPATH && cd $(dirname "${BASH_SOURCE[0]}")/.. && pwd)

# find or download controller-gen
CONTROLLER_GEN=$(which controller-gen)

if [ "$CONTROLLER_GEN" = "" ]
then
  TMP_DIR=$(mktemp -d);
  cd $TMP_DIR;
  go mod init tmp;
  go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.12.0;
  rm -rf $TMP_DIR;
  CONTROLLER_GEN=$(which controller-gen)
fi

if [ "$CONTROLLER_GEN" = "" ]
then
  echo "ERROR: failed to get controller-gen";
  exit 1;
fi

$CONTROLLER_GEN crd paths=${SCRIPT_ROOT}/apis/...

# To use your own boilerplate text use:
#   --go-header-file ${SCRIPT_ROOT}/hack/custom-boilerplate.go.txt
