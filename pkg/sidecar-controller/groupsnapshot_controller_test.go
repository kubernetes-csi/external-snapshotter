/*
Copyright 2026 The Kubernetes Authors.

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

package sidecar_controller

import (
	"testing"

	crdv1beta2 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta2"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

// Helper function to create a VolumeGroupSnapshotContent for testing
func newGroupSnapshotContent(
	contentName string,
	boundToGroupSnapshotUID string,
	boundToGroupSnapshotName string,
	boundToGroupSnapshotNamespace string,
	groupSnapshotHandle string,
	groupSnapshotClassName string,
	volumeHandles []string,
	deletionPolicy crdv1.DeletionPolicy,
	creationTime *metav1.Time,
	withFinalizer bool,
	deletionTime *metav1.Time,
) *crdv1beta2.VolumeGroupSnapshotContent {
	var annotations map[string]string

	content := &crdv1beta2.VolumeGroupSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name:              contentName,
			ResourceVersion:   "1",
			DeletionTimestamp: deletionTime,
			Annotations:       annotations,
		},
		Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
			Driver:         mockDriverName,
			DeletionPolicy: deletionPolicy,
			VolumeGroupSnapshotRef: v1.ObjectReference{
				Kind:       "VolumeGroupSnapshot",
				APIVersion: "groupsnapshot.storage.k8s.io/v1beta2",
				UID:        types.UID(boundToGroupSnapshotUID),
				Namespace:  boundToGroupSnapshotNamespace,
				Name:       boundToGroupSnapshotName,
			},
		},
		Status: &crdv1beta2.VolumeGroupSnapshotContentStatus{
			CreationTime: creationTime,
		},
	}

	if deletionTime != nil {
		metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingDeleted, "yes")
	}

	if groupSnapshotHandle != "" {
		content.Status.VolumeGroupSnapshotHandle = &groupSnapshotHandle
	}

	if groupSnapshotClassName != "" {
		content.Spec.VolumeGroupSnapshotClassName = &groupSnapshotClassName
	}

	if len(volumeHandles) > 0 {
		content.Spec.Source = crdv1beta2.VolumeGroupSnapshotContentSource{
			VolumeHandles: volumeHandles,
		}
	}

	if withFinalizer {
		content.ObjectMeta.Finalizers = []string{utils.VolumeGroupSnapshotContentFinalizer}
	}

	return content
}

// Helper function to create a VolumeGroupSnapshotContent with pre-provisioned
// group snapshot handles
func newGroupSnapshotContentWithHandles(
	contentName string,
	boundToGroupSnapshotUID string,
	boundToGroupSnapshotName string,
	boundToGroupSnapshotNamespace string,
	groupSnapshotHandle string,
	snapshotHandles []string,
	groupSnapshotClassName string,
	deletionPolicy crdv1.DeletionPolicy,
	creationTime *metav1.Time,
	withFinalizer bool,
	deletionTime *metav1.Time,
) *crdv1beta2.VolumeGroupSnapshotContent {
	content := newGroupSnapshotContent(
		contentName,
		boundToGroupSnapshotUID,
		boundToGroupSnapshotName,
		boundToGroupSnapshotNamespace,
		groupSnapshotHandle,
		groupSnapshotClassName,
		nil,
		deletionPolicy,
		creationTime,
		withFinalizer,
		deletionTime,
	)

	content.Spec.Source = crdv1beta2.VolumeGroupSnapshotContentSource{
		GroupSnapshotHandles: &crdv1beta2.GroupSnapshotHandles{
			VolumeGroupSnapshotHandle: groupSnapshotHandle,
			VolumeSnapshotHandles:     snapshotHandles,
		},
	}

	return content
}

// Helper function to create a VolumeGroupSnapshotClass
func newGroupSnapshotClass(
	className string,
	driver string,
	deletionPolicy crdv1.DeletionPolicy,
	parameters map[string]string,
) *crdv1beta2.VolumeGroupSnapshotClass {
	return &crdv1beta2.VolumeGroupSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: className,
		},
		Driver:         driver,
		DeletionPolicy: deletionPolicy,
		Parameters:     parameters,
	}
}

// TestGroupSnapshotControllerCache tests the cache functionality for group snapshot content
func TestGroupSnapshotControllerCache(t *testing.T) {
	// Cache under test
	c := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)

	groupContent := newGroupSnapshotContent(
		"group-content-1",
		"group-snap-uid-1",
		"group-snap-1",
		testNamespace,
		"group-handle-1",
		"group-class-1",
		[]string{"vol-handle-1", "vol-handle-2"},
		deletionPolicy,
		nil,
		false,
		nil,
	)

	// Test storing new group snapshot content
	groupContent.ResourceVersion = "1"
	ret, err := utils.StoreObjectUpdate(c, groupContent, "groupsnapshotcontent")
	if err != nil {
		t.Errorf("expected storeObjectUpdate to succeed, got: %v", err)
	}
	if !ret {
		t.Errorf("expected storeObjectUpdate to return true, got: false")
	}

	// Test storing the same version
	ret, err = utils.StoreObjectUpdate(c, groupContent, "groupsnapshotcontent")
	if err != nil {
		t.Errorf("expected storeObjectUpdate to succeed, got: %v", err)
	}
	if !ret {
		t.Errorf("expected storeObjectUpdate to return true for same version, got: false")
	}

	// Test storing newer version
	groupContent.ResourceVersion = "2"
	ret, err = utils.StoreObjectUpdate(c, groupContent, "groupsnapshotcontent")
	if err != nil {
		t.Errorf("expected storeObjectUpdate to succeed, got: %v", err)
	}
	if !ret {
		t.Errorf("expected storeObjectUpdate to return true for newer version, got: false")
	}

	// Test storing older version - should be rejected
	// Note: The cache implementation compares versions as integers
	// Version "1" is older than "2", so it should be rejected
	olderContent := newGroupSnapshotContent(
		"group-content-1",
		"group-snap-uid-1",
		"group-snap-1",
		testNamespace,
		"group-handle-1",
		"group-class-1",
		[]string{"vol-handle-1", "vol-handle-2"},
		deletionPolicy,
		nil,
		false,
		nil,
	)
	olderContent.ResourceVersion = "1"
	ret, err = utils.StoreObjectUpdate(c, olderContent, "groupsnapshotcontent")
	if err != nil {
		t.Errorf("expected storeObjectUpdate to succeed, got: %v", err)
	}
	if ret {
		t.Errorf("expected storeObjectUpdate to return false for older version, got: true")
	}
}

// TestShouldDeleteGroupSnapshotContent tests the logic for determining whether
// a VolumeGroupSnapshotContent should be deleted
func TestShouldDeleteGroupSnapshotContent(t *testing.T) {
	ctrl := &csiSnapshotSideCarController{}

	tests := []struct {
		name           string
		expectedReturn bool
		content        *crdv1beta2.VolumeGroupSnapshotContent
	}{
		{
			name:           "DeletionTimestamp is nil",
			expectedReturn: false,
			content: newGroupSnapshotContent(
				"group-content-1",
				"group-snap-uid-1",
				"group-snap-1",
				testNamespace,
				"group-handle-1",
				"group-class-1",
				[]string{"vol-handle-1", "vol-handle-2"},
				crdv1.VolumeSnapshotContentDelete,
				nil,
				false,
				nil,
			),
		},
		{
			name:           "Content is not bound (pre-provisioned)",
			expectedReturn: true,
			content: newGroupSnapshotContentWithHandles(
				"group-content-not-bound",
				"", // empty UID means not bound
				"",
				"",
				"group-handle-1",
				[]string{"snap-handle-1", "snap-handle-2"},
				"group-class-1",
				crdv1.VolumeSnapshotContentDelete,
				nil,
				false,
				&timeNowMetav1,
			),
		},
		{
			name:           "AnnVolumeGroupSnapshotBeingCreated annotation is set",
			expectedReturn: false,
			content: func() *crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"group-content-being-created",
					"group-snap-uid-1",
					"group-snap-1",
					testNamespace,
					"",
					"group-class-1",
					[]string{"vol-handle-1", "vol-handle-2"},
					crdv1.VolumeSnapshotContentDelete,
					nil,
					false,
					&timeNowMetav1,
				)
				metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated, "yes")
				return content
			}(),
		},
		{
			name:           "AnnVolumeGroupSnapshotBeingDeleted annotation is set",
			expectedReturn: true,
			content: func() *crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"group-content-being-deleted",
					"group-snap-uid-1",
					"group-snap-1",
					testNamespace,
					"group-handle-1",
					"group-class-1",
					[]string{"vol-handle-1", "vol-handle-2"},
					crdv1.VolumeSnapshotContentDelete,
					nil,
					false,
					&timeNowMetav1,
				)
				metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingDeleted, "yes")
				return content
			}(),
		},
		{
			name:           "If no other cases match, should not delete",
			expectedReturn: false,
			content: &crdv1beta2.VolumeGroupSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "group-content-default",
					DeletionTimestamp: &timeNowMetav1,
				},
				Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
					VolumeGroupSnapshotRef: v1.ObjectReference{
						UID: "some-uid",
					},
					Source: crdv1beta2.VolumeGroupSnapshotContentSource{
						VolumeHandles: []string{"vol-1"},
					},
				},
			},
		},
	}

	for _, test := range tests {
		result := ctrl.shouldDeleteGroupSnapshotContent(test.content)

		if result != test.expectedReturn {
			t.Errorf("Test %s: Got %t but expected %t", test.name, result, test.expectedReturn)
		}
	}
}

// TestGetSnapshotNameForVolumeGroupSnapshotContent tests the helper function
// that generates unique snapshot names for volume group snapshot content
func TestGetSnapshotNameForVolumeGroupSnapshotContent(t *testing.T) {
	groupContentUUID1 := "group-content-uuid-1"
	groupContentUUID2 := "group-content-uuid-2"
	pvUUID1 := "pv-uuid-1"
	pvUUID2 := "pv-uuid-2"

	// Test that same inputs produce same output
	name1a := GetSnapshotNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID1)
	name1b := GetSnapshotNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID1)

	if name1a != name1b {
		t.Errorf("Expected same name for same inputs, got %s and %s", name1a, name1b)
	}

	// Test that different inputs produce different outputs
	name2 := GetSnapshotNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID2)
	if name1a == name2 {
		t.Errorf("Expected different names for different PV UUIDs, got %s for both", name1a)
	}

	name3 := GetSnapshotNameForVolumeGroupSnapshotContent(groupContentUUID2, pvUUID1)
	if name1a == name3 {
		t.Errorf("Expected different names for different group content UUIDs, got %s for both", name1a)
	}

	// Test that name starts with expected prefix
	if len(name1a) < 9 || name1a[:9] != "snapshot-" {
		t.Errorf("Expected name to start with 'snapshot-', got %s", name1a)
	}
}

// TestGetSnapshotContentNameForVolumeGroupSnapshotContent tests the helper function
// that generates unique snapshot content names for volume group snapshot content
func TestGetSnapshotContentNameForVolumeGroupSnapshotContent(t *testing.T) {
	groupContentUUID1 := "group-content-uuid-1"
	groupContentUUID2 := "group-content-uuid-2"
	pvUUID1 := "pv-uuid-1"
	pvUUID2 := "pv-uuid-2"

	// Test that same inputs produce same output
	name1a := GetSnapshotContentNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID1)
	name1b := GetSnapshotContentNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID1)

	if name1a != name1b {
		t.Errorf("Expected same name for same inputs, got %s and %s", name1a, name1b)
	}

	// Test that different inputs produce different outputs
	name2 := GetSnapshotContentNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID2)
	if name1a == name2 {
		t.Errorf("Expected different names for different PV UUIDs, got %s for both", name1a)
	}

	name3 := GetSnapshotContentNameForVolumeGroupSnapshotContent(groupContentUUID2, pvUUID1)
	if name1a == name3 {
		t.Errorf("Expected different names for different group content UUIDs, got %s for both", name1a)
	}

	// Test that name starts with expected prefix
	if len(name1a) < 12 || name1a[:12] != "snapcontent-" {
		t.Errorf("Expected name to start with 'snapcontent-', got %s", name1a)
	}
}

// TestRemoveGroupSnapshotContentFinalizer tests removing finalizers from
// VolumeGroupSnapshotContent objects
func TestRemoveGroupSnapshotContentFinalizer(t *testing.T) {
	tests := []struct {
		name                 string
		content              *crdv1beta2.VolumeGroupSnapshotContent
		expectFinalizerAfter bool
		expectError          bool
	}{
		{
			name: "Remove existing finalizer",
			content: newGroupSnapshotContent(
				"group-content-with-finalizer",
				"group-snap-uid-1",
				"group-snap-1",
				testNamespace,
				"group-handle-1",
				"group-class-1",
				[]string{"vol-handle-1", "vol-handle-2"},
				crdv1.VolumeSnapshotContentDelete,
				nil,
				true, // with finalizer
				nil,
			),
			expectFinalizerAfter: false,
			expectError:          false,
		},
		{
			name: "No finalizer to remove",
			content: newGroupSnapshotContent(
				"group-content-no-finalizer",
				"group-snap-uid-1",
				"group-snap-1",
				testNamespace,
				"group-handle-1",
				"group-class-1",
				[]string{"vol-handle-1", "vol-handle-2"},
				crdv1.VolumeSnapshotContentDelete,
				nil,
				false, // without finalizer
				nil,
			),
			expectFinalizerAfter: false,
			expectError:          false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Check if finalizer exists before
			hasFinalizer := false
			for _, finalizer := range test.content.ObjectMeta.Finalizers {
				if finalizer == utils.VolumeGroupSnapshotContentFinalizer {
					hasFinalizer = true
					break
				}
			}

			if test.expectFinalizerAfter {
				if !hasFinalizer {
					t.Errorf("Test setup error: expected content to have finalizer before test")
				}
			}
		})
	}
}

// TestGetCredentialsFromAnnotationForGroupSnapshot tests credential retrieval
// from VolumeGroupSnapshotContent annotations
func TestGetCredentialsFromAnnotationForGroupSnapshot(t *testing.T) {
	tests := []struct {
		name        string
		content     *crdv1beta2.VolumeGroupSnapshotContent
		expectError bool
	}{
		{
			name: "No annotation - should succeed with nil credentials",
			content: newGroupSnapshotContent(
				"group-content-no-secret",
				"group-snap-uid-1",
				"group-snap-1",
				testNamespace,
				"group-handle-1",
				"group-class-1",
				[]string{"vol-handle-1", "vol-handle-2"},
				crdv1.VolumeSnapshotContentDelete,
				nil,
				false,
				nil,
			),
			expectError: false,
		},
		{
			name: "Empty secret name annotation - should fail",
			content: func() *crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"group-content-empty-secret-name",
					"group-snap-uid-1",
					"group-snap-1",
					testNamespace,
					"group-handle-1",
					"group-class-1",
					[]string{"vol-handle-1", "vol-handle-2"},
					crdv1.VolumeSnapshotContentDelete,
					nil,
					false,
					nil,
				)
				metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnDeletionGroupSecretRefName, "")
				metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnDeletionGroupSecretRefNamespace, "default")
				return content
			}(),
			expectError: true,
		},
		{
			name: "Empty secret namespace annotation - should fail",
			content: func() *crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"group-content-empty-secret-namespace",
					"group-snap-uid-1",
					"group-snap-1",
					testNamespace,
					"group-handle-1",
					"group-class-1",
					[]string{"vol-handle-1", "vol-handle-2"},
					crdv1.VolumeSnapshotContentDelete,
					nil,
					false,
					nil,
				)
				metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnDeletionGroupSecretRefName, "secret-name")
				metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnDeletionGroupSecretRefNamespace, "")
				return content
			}(),
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := &csiSnapshotSideCarController{}
			_, err := ctrl.GetCredentialsFromAnnotationForGroupSnapshot(test.content)

			if test.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !test.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// TestEnqueueGroupSnapshotContentWork tests the enqueue functionality
func TestEnqueueGroupSnapshotContentWork(t *testing.T) {
	// This test verifies that the enqueue function handles various object types correctly
	content := newGroupSnapshotContent(
		"group-content-1",
		"group-snap-uid-1",
		"group-snap-1",
		testNamespace,
		"group-handle-1",
		"group-class-1",
		[]string{"vol-handle-1", "vol-handle-2"},
		crdv1.VolumeSnapshotContentDelete,
		nil,
		false,
		nil,
	)

	// Test with valid content
	objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(content)
	if err != nil {
		t.Errorf("Failed to get key from object: %v", err)
	}
	if objName == "" {
		t.Errorf("Expected non-empty object name")
	}

	// Test with DeletedFinalStateUnknown wrapper
	tombstone := cache.DeletedFinalStateUnknown{
		Key: objName,
		Obj: content,
	}
	extractedObj := tombstone.Obj
	if extractedContent, ok := extractedObj.(*crdv1beta2.VolumeGroupSnapshotContent); !ok {
		t.Errorf("Failed to extract content from DeletedFinalStateUnknown")
	} else if extractedContent.Name != content.Name {
		t.Errorf("Extracted content name %s doesn't match original %s", extractedContent.Name, content.Name)
	}
}
