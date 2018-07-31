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

.PHONY: all csi-snapshotter clean test

REGISTRY_NAME=quay.io/k8scsi
IMAGE_NAME=csi-snapshotter
IMAGE_VERSION=canary
IMAGE_TAG=$(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)

ifdef V
TESTARGS = -v -args -alsologtostderr -v 5
else
TESTARGS =
endif


all: csi-snapshotter

csi-snapshotter:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o ./bin/csi-snapshotter ./cmd/csi-snapshotter

clean:
	-rm -rf bin

container: csi-snapshotter
	docker build -t $(IMAGE_TAG) .

push: container
	docker push $(IMAGE_TAG)

test:
	go test `go list ./... | grep -v 'vendor'` $(TESTARGS)
	go vet `go list ./... | grep -v vendor`
