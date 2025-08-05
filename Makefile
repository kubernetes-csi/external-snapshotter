# Copyright 2018 The Kubernetes Authors.
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

.PHONY: all snapshot-controller csi-snapshotter snapshot-conversion-webhook clean test

CMDS=snapshot-controller csi-snapshotter snapshot-conversion-webhook
all: build
include release-tools/build.make

# The test-vendor-client target performs vendor checks in both
# the external-snapshotter module and the client module.
# This target has been added for the following reasons:
# 1. The test-vendor target does not perform vendor checks for the client module.
# 2. The test-vendor target cannot detect if vendor updates have been made in
# the external-snapshotter module when changes are made in the client module.
.PHONY: test-vendor-client
test: test-vendor-client
test-vendor-client:
	@ echo; echo "### $@:"
	@ cd client && ../release-tools/verify-vendor.sh
	@ hack/verify-vendor.sh
