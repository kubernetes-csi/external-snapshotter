#!/usr/bin/env bash

# Copyright 2025 The Kubernetes Authors.
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

set -euxo pipefail

BASE_DIR="$( cd "$( dirname "$0" )" && pwd )/../.."

TEMP_DIR="$( mktemp -d )"
trap 'rm -rf ${TEMP_DIR}' EXIT

# Convert snapshot-controller as Helm Charts.
SNAPSHOT_CONTROLLER_TEMPLATES=${BASE_DIR}/charts/snapshot-controller/templates
SNAPSHOT_CONTROLLER_YAML=${SNAPSHOT_CONTROLLER_TEMPLATES}/setup-snapshot-controller.yaml

cp -rfp ${BASE_DIR}/client/config/crd/* ${SNAPSHOT_CONTROLLER_TEMPLATES}/
cp -rfp ${BASE_DIR}/deploy/kubernetes/snapshot-controller/* ${SNAPSHOT_CONTROLLER_TEMPLATES}/
rm -rf ${SNAPSHOT_CONTROLLER_TEMPLATES}/kustomization.yaml

find ${SNAPSHOT_CONTROLLER_TEMPLATES} -type f -name '*.yaml' | while read -r _YAML
do
    yq -i -P -I 2 '... comments=""' $_YAML
    yq -i e '(select(.metadata.namespace == "kube-system") | .metadata.namespace) = "{{ .Release.Namespace }}"' $_YAML
    yq -i e '(select(.subjects.[].namespace == "kube-system") | .subjects.[].namespace) = "{{ .Release.Namespace }}"' $_YAML
done

yq -i e '(select(.spec | has("replicas")) | .spec.replicas) = "{{ .Values.snapshotController.replicaCount }}"' $SNAPSHOT_CONTROLLER_YAML
yq -i e '(select(.spec | has("template")) | .spec.template.spec.containers.[] | select(.name == "snapshot-controller") | .image) = "{{ .Values.snapshotController.snapshotController.image.repository }}:{{ .Values.snapshotController.snapshotController.image.tag }}"' $SNAPSHOT_CONTROLLER_YAML
yq -i e '(select(.spec | has("template")) | .spec.template.spec.containers.[] | select(.name == "snapshot-controller") | .imagePullPolicy) = "{{ .Values.snapshotController.snapshotController.image.pullPolicy }}"' $SNAPSHOT_CONTROLLER_YAML

find ${SNAPSHOT_CONTROLLER_TEMPLATES} -type f -name '*.yaml' | xargs sed -i "s/'{{/{{/g;s/}}'/}}/g"

# Convert csi-snapshotter as Helm Charts.
CSI_SNAPSHOTTER_TEMPLATES=${BASE_DIR}/charts/csi-snapshotter/templates
CSI_SNAPSHOTTER_YAML=${CSI_SNAPSHOTTER_TEMPLATES}/setup-csi-snapshotter.yaml

cp -rfp ${BASE_DIR}/client/config/crd/* ${CSI_SNAPSHOTTER_TEMPLATES}/
cp -rfp ${BASE_DIR}/deploy/kubernetes/csi-snapshotter/* ${CSI_SNAPSHOTTER_TEMPLATES}/
rm -rf ${CSI_SNAPSHOTTER_TEMPLATES}/kustomization.yaml
rm -rf ${CSI_SNAPSHOTTER_TEMPLATES}/README.md

find ${CSI_SNAPSHOTTER_TEMPLATES} -type f -name '*.yaml' | while read -r _YAML
do
    yq -i -P -I 2 '... comments=""' $_YAML
    yq -i e '(select(.metadata | has("namespace")) | .metadata.namespace) = "{{ .Release.Namespace }}"' $_YAML
    yq -i e '(select(.subjects.[] | has("namespace")) | .subjects.[].namespace) = "{{ .Release.Namespace }}"' $_YAML
done

yq -i e '(select(.spec | has("replicas")) | .spec.replicas) = "{{ .Values.csiSnapshotter.replicaCount }}"' $CSI_SNAPSHOTTER_YAML

yq -i e '(select(.spec | has("template")) | .spec.template.spec.containers.[] | select(.name == "csi-provisioner") | .image) = "{{ .Values.csiSnapshotter.csiProvisioner.image.repository }}:{{ .Values.csiSnapshotter.csiProvisioner.image.tag }}"' $CSI_SNAPSHOTTER_YAML
yq -i e '(select(.spec | has("template")) | .spec.template.spec.containers.[] | select(.name == "csi-provisioner") | .imagePullPolicy) = "{{ .Values.csiSnapshotter.csiProvisioner.image.pullPolicy }}"' $CSI_SNAPSHOTTER_YAML

yq -i e '(select(.spec | has("template")) | .spec.template.spec.containers.[] | select(.name == "csi-snapshotter") | .image) = "{{ .Values.csiSnapshotter.csiSnapshotter.image.repository }}:{{ .Values.csiSnapshotter.csiSnapshotter.image.tag }}"' $CSI_SNAPSHOTTER_YAML
yq -i e '(select(.spec | has("template")) | .spec.template.spec.containers.[] | select(.name == "csi-snapshotter") | .imagePullPolicy) = "{{ .Values.csiSnapshotter.csiSnapshotter.image.pullPolicy }}"' $CSI_SNAPSHOTTER_YAML

yq -i e '(select(.spec | has("template")) | .spec.template.spec.containers.[] | select(.name == "hostpath") | .image) = "{{ .Values.csiSnapshotter.hostpath.image.repository }}:{{ .Values.csiSnapshotter.hostpath.image.tag }}"' $CSI_SNAPSHOTTER_YAML
yq -i e '(select(.spec | has("template")) | .spec.template.spec.containers.[] | select(.name == "hostpath") | .imagePullPolicy) = "{{ .Values.csiSnapshotter.hostpath.image.pullPolicy }}"' $CSI_SNAPSHOTTER_YAML

find ${CSI_SNAPSHOTTER_TEMPLATES} -type f -name '*.yaml' | xargs sed -i "s/'{{/{{/g;s/}}'/}}/g"

