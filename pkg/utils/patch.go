/*
Copyright 2021 The Kubernetes Authors.

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
package utils

import (
	"context"
	"encoding/json"
	"fmt"

	patch "github.com/evanphx/json-patch"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// PatchVolumeSnapshot patches a volume snapshot object
func PatchVolumeSnapshot(
	existingSnapshot *crdv1.VolumeSnapshot,
	updatedSnapshot *crdv1.VolumeSnapshot,
	client clientset.Interface,
	subresources ...string,
) (*crdv1.VolumeSnapshot, error) {
	patch, err := createSnapshotPatch(existingSnapshot, updatedSnapshot)
	if err != nil {
		return updatedSnapshot, err
	}

	newSnapshot, err := client.SnapshotV1().VolumeSnapshots(updatedSnapshot.Namespace).Patch(context.TODO(), existingSnapshot.Name, types.MergePatchType, patch, metav1.PatchOptions{}, subresources...)
	if err != nil {
		return updatedSnapshot, err
	}

	return newSnapshot, nil
}

// PatchVolumeSnapshotContent patches a volume snapshot content object
func PatchVolumeSnapshotContent(
	existingSnapshotContent *crdv1.VolumeSnapshotContent,
	updatedSnapshotContent *crdv1.VolumeSnapshotContent,
	client clientset.Interface,
	subresources ...string,
) (*crdv1.VolumeSnapshotContent, error) {
	patch, err := createContentPatch(existingSnapshotContent, updatedSnapshotContent)
	if err != nil {
		return updatedSnapshotContent, err
	}

	newSnapshotContent, err := client.SnapshotV1().VolumeSnapshotContents().Patch(context.TODO(), existingSnapshotContent.Name, types.MergePatchType, patch, metav1.PatchOptions{}, subresources...)
	if err != nil {
		return updatedSnapshotContent, err
	}

	return newSnapshotContent, nil
}

func addResourceVersion(patchBytes []byte, resourceVersion string) ([]byte, error) {
	var patchMap map[string]interface{}
	err := json.Unmarshal(patchBytes, &patchMap)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling patch: %v", err)
	}
	u := unstructured.Unstructured{Object: patchMap}
	a, err := meta.Accessor(&u)
	if err != nil {
		return nil, fmt.Errorf("error creating accessor: %v", err)
	}
	a.SetResourceVersion(resourceVersion)
	versionBytes, err := json.Marshal(patchMap)
	if err != nil {
		return nil, fmt.Errorf("error marshalling json patch: %v", err)
	}
	return versionBytes, nil
}

func createSnapshotPatch(snapshot *crdv1.VolumeSnapshot, updatedSnapshot *crdv1.VolumeSnapshot) ([]byte, error) {
	oldData, err := runtime.Encode(unstructured.UnstructuredJSONScheme, snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal old data: %v", err)
	}
	newData, err := runtime.Encode(unstructured.UnstructuredJSONScheme, updatedSnapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal new data: %v", err)
	}

	patchBytes, err := patch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, fmt.Errorf("failed to create merge patch: %v", err)
	}

	patchBytes, err = addResourceVersion(patchBytes, snapshot.ResourceVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to add resource version: %v", err)
	}

	return patchBytes, nil
}

func createContentPatch(content *crdv1.VolumeSnapshotContent, updatedContent *crdv1.VolumeSnapshotContent) ([]byte, error) {
	oldData, err := runtime.Encode(unstructured.UnstructuredJSONScheme, content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal old data: %v", err)
	}

	newData, err := runtime.Encode(unstructured.UnstructuredJSONScheme, updatedContent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal new data: %v", err)
	}

	patchBytes, err := patch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, fmt.Errorf("failed to create merge patch: %v", err)
	}

	patchBytes, err = addResourceVersion(patchBytes, content.ResourceVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to add resource version: %v", err)
	}

	return patchBytes, nil
}
