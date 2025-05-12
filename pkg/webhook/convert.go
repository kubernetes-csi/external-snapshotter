/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
)

const (
	// volumeSnapshotInfoAnnotationName is the name of the annotation
	// that is used when converting data from the v1beta2 to the v1beta1
	// API to make the conversion reversible.
	volumeSnapshotInfoAnnotationName = "groupsnapshot.storage.kubernetes.io/volume-snapshot-info-list"
)

func convertGroupSnapshotCRD(obj *unstructured.Unstructured, toVersion string) (*unstructured.Unstructured, metav1.Status) {
	klog.V(2).Info("converting crd")

	convertedObject := obj.DeepCopy()
	fromVersion := obj.GetAPIVersion()
	kind := obj.GetKind()

	if toVersion == fromVersion {
		return nil, statusErrorWithMessage("conversion from a version to itself should not call the webhook: %s", toVersion)
	}

	switch obj.GetAPIVersion() {
	case "groupsnapshot.storage.k8s.io/v1beta1":
		switch toVersion {
		case "groupsnapshot.storage.k8s.io/v1beta2":
			switch kind {
			case "VolumeGroupSnapshot":
			case "VolumeGroupSnapshotClass":
			case "VolumeGroupSnapshotContent":
				if err := convertVolumeGroupSnapshotFromV1beta1ToV1beta2(convertedObject); err != nil {
					return nil, statusErrorWithMessage("%s", err.Error())
				}
			default:
				return nil, statusErrorWithMessage("unexpected conversion kind %q", kind)
			}
		default:
			return nil, statusErrorWithMessage("unexpected conversion version %q", toVersion)
		}
	case "groupsnapshot.storage.k8s.io/v1beta2":
		switch toVersion {
		case "groupsnapshot.storage.k8s.io/v1beta1":
			switch kind {
			case "VolumeGroupSnapshot":
			case "VolumeGroupSnapshotClass":
			case "VolumeGroupSnapshotContent":
				if err := convertVolumeGroupSnapshotFromV1beta2ToV1beta1(convertedObject); err != nil {
					return nil, statusErrorWithMessage("%s", err.Error())
				}
			default:
				return nil, statusErrorWithMessage("unexpected conversion kind %q", kind)
			}
		default:
			return nil, statusErrorWithMessage("unexpected conversion version %q", toVersion)
		}
	default:
		return nil, statusErrorWithMessage("unexpected conversion version %q", fromVersion)
	}

	return convertedObject, statusSucceed()
}

func convertVolumeGroupSnapshotFromV1beta1ToV1beta2(obj *unstructured.Unstructured) error {
	annotations := obj.GetAnnotations()
	if value, ok := annotations[volumeSnapshotInfoAnnotationName]; ok {
		// We use the annotation to fill the missing fields into the status
		var slice any

		if err := json.Unmarshal([]byte(value), &slice); err != nil {
			return fmt.Errorf("unable to deserialize annotation %q: %w", volumeSnapshotInfoAnnotationName, err)
		}

		if err := unstructured.SetNestedSlice(
			obj.Object,
			slice.([]any),
			"status",
			"volumeSnapshotInfoList",
		); err != nil {
			return fmt.Errorf("error while setting .status.volumeSnapshotInfoList: %w", err)
		}

		unstructured.RemoveNestedField(obj.Object, "status", "volumeSnapshotHandlePairList")

		// Remove the tracking annotation
		delete(annotations, volumeSnapshotInfoAnnotationName)
		obj.SetAnnotations(annotations)
	} else {
		// Rename the old field with the new name
		volumeSnapshotHandlePairList, found, err := unstructured.NestedSlice(obj.Object, "status", "volumeSnapshotHandlePairList")
		if err != nil {
			return fmt.Errorf("unable to traverse for .status.volumeSnapshotHandlePairList: %w", err)
		}
		if found {
			if err := unstructured.SetNestedSlice(
				obj.Object,
				volumeSnapshotHandlePairList,
				"status",
				"volumeSnapshotInfoList",
			); err != nil {
				return fmt.Errorf("error while setting .status.volumeSnapshotInfoList: %w", err)
			}

			unstructured.RemoveNestedField(obj.Object, "status", "volumeSnapshotHandlePairList")
		}
	}
	return nil
}

func convertVolumeGroupSnapshotFromV1beta2ToV1beta1(obj *unstructured.Unstructured) error {
	volumeSnapshotInfoList, found, err := unstructured.NestedSlice(obj.Object, "status", "volumeSnapshotInfoList")
	if err != nil {
		return fmt.Errorf("unable to traverse for .status.volumeSnapshotInfoList: %w", err)
	}
	if found {
		// Step 1: attach the existing volumeSnapshotInfoList as an annotation
		serializedField, err := json.Marshal(volumeSnapshotInfoList)
		if err != nil {
			return fmt.Errorf("error while serializing the volumeSnapshotInfoList field: %w", err)
		}

		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[volumeSnapshotInfoAnnotationName] = string(serializedField)
		obj.SetAnnotations(annotations)

		// Step 2: convert volumeSnapshotInfoList to volumeSnapshotHandlePairList
		for i, entry := range volumeSnapshotInfoList {
			if mapEntry, ok := entry.(map[string]interface{}); ok {
				delete(mapEntry, "creationTime")
				delete(mapEntry, "readyToUse")
				delete(mapEntry, "restoreSize")
			} else {
				return fmt.Errorf("unexpected content in .status.volumeSnapshotInfoList[%q]: expected map", i)
			}
		}

		if err := unstructured.SetNestedSlice(
			obj.Object,
			volumeSnapshotInfoList,
			"status",
			"volumeSnapshotHandlePairList",
		); err != nil {
			return fmt.Errorf("error while setting .status.volumeSnapshotHandlePairList: %w", err)
		}

		// Step 3: remove volumeSnapshotInfoList
		unstructured.RemoveNestedField(obj.Object, "status", "volumeSnapshotInfoList")
	}

	return nil
}
