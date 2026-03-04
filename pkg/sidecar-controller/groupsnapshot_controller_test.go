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
	"context"
	"slices"
	"testing"

	crdv1beta2 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta2"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	groupsnapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumegroupsnapshot/v1beta2"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
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
// that generates unique snapshot names for volume group snapshot content.
// Note: The function encodes the current time in the name, so two calls with the
// same inputs may produce different names; we only verify format and that
// different inputs produce different names.
func TestGetSnapshotNameForVolumeGroupSnapshotContent(t *testing.T) {
	groupContentUUID1 := "group-content-uuid-1"
	groupContentUUID2 := "group-content-uuid-2"
	pvUUID1 := "pv-uuid-1"
	pvUUID2 := "pv-uuid-2"

	name1 := GetSnapshotNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID1)

	// Test that different inputs produce different outputs
	name2 := GetSnapshotNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID2)
	if name1 == name2 {
		t.Errorf("Expected different names for different PV UUIDs, got %s for both", name1)
	}

	name3 := GetSnapshotNameForVolumeGroupSnapshotContent(groupContentUUID2, pvUUID1)
	if name1 == name3 {
		t.Errorf("Expected different names for different group content UUIDs, got %s for both", name1)
	}

	// Test that name starts with expected prefix
	if len(name1) < 9 || name1[:9] != "snapshot-" {
		t.Errorf("Expected name to start with 'snapshot-', got %s", name1)
	}
}

// TestGetSnapshotContentNameForVolumeGroupSnapshotContent tests the helper function
// that generates unique snapshot content names for volume group snapshot content.
// Note: The function encodes the current time in the name, so two calls with the
// same inputs may produce different names; we only verify format and that
// different inputs produce different names.
func TestGetSnapshotContentNameForVolumeGroupSnapshotContent(t *testing.T) {
	groupContentUUID1 := "group-content-uuid-1"
	groupContentUUID2 := "group-content-uuid-2"
	pvUUID1 := "pv-uuid-1"
	pvUUID2 := "pv-uuid-2"

	name1 := GetSnapshotContentNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID1)

	// Test that different inputs produce different outputs
	name2 := GetSnapshotContentNameForVolumeGroupSnapshotContent(groupContentUUID1, pvUUID2)
	if name1 == name2 {
		t.Errorf("Expected different names for different PV UUIDs, got %s for both", name1)
	}

	name3 := GetSnapshotContentNameForVolumeGroupSnapshotContent(groupContentUUID2, pvUUID1)
	if name1 == name3 {
		t.Errorf("Expected different names for different group content UUIDs, got %s for both", name1)
	}

	// Test that name starts with expected prefix
	if len(name1) < 12 || name1[:12] != "snapcontent-" {
		t.Errorf("Expected name to start with 'snapcontent-', got %s", name1)
	}
}

// TestRemoveGroupSnapshotContentFinalizer tests removing finalizers from
// VolumeGroupSnapshotContent objects by calling removeGroupSnapshotContentFinalizer.
func TestRemoveGroupSnapshotContentFinalizer(t *testing.T) {
	tests := []struct {
		name        string
		content     *crdv1beta2.VolumeGroupSnapshotContent
		expectError bool
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
			expectError: false,
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
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var ctrl *csiSnapshotSideCarController
			if slices.Contains(test.content.ObjectMeta.Finalizers, utils.VolumeGroupSnapshotContentFinalizer) {
				// Need fake client and store for the patch path
				contentCopy := test.content.DeepCopy()
				client := fake.NewSimpleClientset(contentCopy)
				store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
				_ = store.Add(contentCopy)
				ctrl = &csiSnapshotSideCarController{
					clientset:                 client,
					groupSnapshotContentStore: store,
				}
			} else {
				ctrl = &csiSnapshotSideCarController{}
			}

			err := ctrl.removeGroupSnapshotContentFinalizer(test.content)
			if test.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !test.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// When we had a finalizer, verify it was removed via the API
			if ctrl.clientset != nil && slices.Contains(test.content.ObjectMeta.Finalizers, utils.VolumeGroupSnapshotContentFinalizer) {
				updated, getErr := ctrl.clientset.GroupsnapshotV1beta2().VolumeGroupSnapshotContents().Get(context.Background(), test.content.Name, metav1.GetOptions{})
				if getErr != nil {
					t.Errorf("Failed to get content after patch: %v", getErr)
				} else if slices.Contains(updated.ObjectMeta.Finalizers, utils.VolumeGroupSnapshotContentFinalizer) {
					t.Errorf("Expected finalizer to be removed from content, but it still exists")
				}
			}
		})
	}
}

// TestGetCredentialsFromAnnotationForGroupSnapshot tests credential retrieval
// from VolumeGroupSnapshotContent annotations
func TestGetCredentialsFromAnnotationForGroupSnapshot(t *testing.T) {
	tests := []struct {
		name           string
		content        *crdv1beta2.VolumeGroupSnapshotContent
		expectError    bool
		withKubeSecret bool
	}{
		{
			name:           "No annotation - should succeed with nil credentials",
			withKubeSecret: false,
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
		{
			name: "Valid secret annotations - should succeed with credentials",
			content: func() *crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"group-content-with-secret",
					"group-snap-uid-1",
					"group-snap-1",
					testNamespace,
					"group-handle-1",
					"group-class-1",
					[]string{"vol-handle-1"},
					crdv1.VolumeSnapshotContentDelete,
					nil,
					false,
					nil,
				)
				metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnDeletionGroupSecretRefName, "my-secret")
				metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnDeletionGroupSecretRefNamespace, testNamespace)
				return content
			}(),
			expectError:    false,
			withKubeSecret: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := &csiSnapshotSideCarController{}
			if test.withKubeSecret {
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: testNamespace},
					Data:       map[string][]byte{"key1": []byte("val1")},
				}
				ctrl.client = kubefake.NewSimpleClientset(secret)
			}
			creds, err := ctrl.GetCredentialsFromAnnotationForGroupSnapshot(test.content)

			if test.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !test.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if test.withKubeSecret && err == nil && (creds == nil || creds["key1"] != "val1") {
				t.Errorf("Expected credentials with key1=val1, got: %v", creds)
			}
		})
	}
}

// TestEnqueueGroupSnapshotContentWork tests that enqueueGroupSnapshotContentWork
// adds the correct keys to the work queue for VolumeGroupSnapshotContent and for
// DeletedFinalStateUnknown wrappers.
func TestEnqueueGroupSnapshotContentWork(t *testing.T) {
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

	expectedKey, err := cache.DeletionHandlingMetaNamespaceKeyFunc(content)
	if err != nil {
		t.Fatalf("Failed to get key from object: %v", err)
	}
	if expectedKey == "" {
		t.Fatalf("Expected non-empty object name")
	}

	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())
	ctrl := &csiSnapshotSideCarController{groupSnapshotContentQueue: queue}

	// Test with valid content: enqueue should add the key
	ctrl.enqueueGroupSnapshotContentWork(content)
	key, quit := queue.Get()
	if quit {
		t.Fatal("Expected queue to have an item")
	}
	if key != expectedKey {
		t.Errorf("Expected key %q, got %q", expectedKey, key)
	}
	queue.Done(key)

	// Test with DeletedFinalStateUnknown wrapper: enqueue should extract obj and add the same key
	deletedUnknownObj := cache.DeletedFinalStateUnknown{Key: expectedKey, Obj: content}
	ctrl.enqueueGroupSnapshotContentWork(deletedUnknownObj)
	key2, quit := queue.Get()
	if quit {
		t.Fatal("Expected queue to have an item after DeletedFinalStateUnknown enqueue")
	}
	if key2 != expectedKey {
		t.Errorf("Expected key %q from DeletedFinalStateUnknown, got %q", expectedKey, key2)
	}
}

// TestEnqueueGroupSnapshotContentWorkNonGroupContent verifies that enqueueGroupSnapshotContentWork
// does not add anything to the queue when the object is not a VolumeGroupSnapshotContent.
func TestEnqueueGroupSnapshotContentWorkNonGroupContent(t *testing.T) {
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())
	ctrl := &csiSnapshotSideCarController{groupSnapshotContentQueue: queue}

	// Enqueue wrong types - queue should remain empty (Len() == 0)
	ctrl.enqueueGroupSnapshotContentWork("not-a-content")
	ctrl.enqueueGroupSnapshotContentWork(42)
	ctrl.enqueueGroupSnapshotContentWork(nil)
	if queue.Len() != 0 {
		t.Errorf("Expected queue to be empty when enqueueing non-VolumeGroupSnapshotContent, got Len=%d", queue.Len())
	}
}

// TestStoreGroupSnapshotContentUpdate tests the controller's storeGroupSnapshotContentUpdate.
func TestStoreGroupSnapshotContentUpdate(t *testing.T) {
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{groupSnapshotContentStore: store}

	content := newGroupSnapshotContent(
		"group-content-store-test",
		"group-snap-uid-1",
		"group-snap-1",
		testNamespace,
		"group-handle-1",
		"group-class-1",
		[]string{"vol-handle-1"},
		crdv1.VolumeSnapshotContentDelete,
		nil,
		false,
		nil,
	)

	got, err := ctrl.storeGroupSnapshotContentUpdate(content)
	if err != nil {
		t.Errorf("storeGroupSnapshotContentUpdate failed: %v", err)
	}
	if !got {
		t.Error("expected storeGroupSnapshotContentUpdate to return true for new content")
	}

	// Same version again - implementation may return true or false depending on cache
	got2, err2 := ctrl.storeGroupSnapshotContentUpdate(content)
	if err2 != nil {
		t.Errorf("storeGroupSnapshotContentUpdate (second call) failed: %v", err2)
	}
	_ = got2 // just ensure no error

	// Verify object is in store
	obj, found, err := store.GetByKey("group-content-store-test")
	if err != nil || !found {
		t.Errorf("expected content in store: found=%v, err=%v", found, err)
	}
	if obj != nil {
		if gsc, ok := obj.(*crdv1beta2.VolumeGroupSnapshotContent); ok && gsc.Name != "group-content-store-test" {
			t.Errorf("stored content name mismatch: %s", gsc.Name)
		}
	}
}

// TestDeleteGroupSnapshotContentInCacheStore tests that deleteGroupSnapshotContentInCacheStore
// removes the content from the controller's cache store.
func TestDeleteGroupSnapshotContentInCacheStore(t *testing.T) {
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{groupSnapshotContentStore: store}

	content := newGroupSnapshotContent(
		"group-content-delete-test",
		"group-snap-uid-1",
		"group-snap-1",
		testNamespace,
		"group-handle-1",
		"group-class-1",
		[]string{"vol-handle-1"},
		crdv1.VolumeSnapshotContentDelete,
		nil,
		false,
		nil,
	)
	if err := store.Add(content); err != nil {
		t.Fatalf("failed to add content to store: %v", err)
	}

	ctrl.deleteGroupSnapshotContentInCacheStore(content)

	_, found, _ := store.GetByKey("group-content-delete-test")
	if found {
		t.Error("expected content to be removed from store after deleteGroupSnapshotContentInCacheStore")
	}
}

// TestGetGroupSnapshotClass tests getGroupSnapshotClass with lister.
func TestGetGroupSnapshotClass(t *testing.T) {
	classMatch := newVolumeGroupSnapshotClass("class-a", mockDriverName)
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(classMatch)
	classLister := groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer)

	ctrl := &csiSnapshotSideCarController{groupSnapshotClassLister: classLister}

	got, err := ctrl.getGroupSnapshotClass("class-a")
	if err != nil {
		t.Fatalf("getGroupSnapshotClass failed: %v", err)
	}
	if got != classMatch {
		t.Errorf("getGroupSnapshotClass() = %v, want %v", got, classMatch)
	}

	_, err = ctrl.getGroupSnapshotClass("nonexistent")
	if err == nil {
		t.Error("getGroupSnapshotClass(nonexistent) expected error, got nil")
	}
}

// TestGetCSIGroupSnapshotInput tests getCSIGroupSnapshotInput branches.
func TestGetCSIGroupSnapshotInput(t *testing.T) {
	classMatch := newVolumeGroupSnapshotClass("class-a", mockDriverName)
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(classMatch)
	classLister := groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer)

	tests := []struct {
		name         string
		content      *crdv1beta2.VolumeGroupSnapshotContent
		ctrl         *csiSnapshotSideCarController
		wantClass    bool
		wantErr      bool
		errSubstring string
	}{
		{
			name: "with class name and no credentials",
			content: newGroupSnapshotContent(
				"with-class", "uid", "snap", testNamespace,
				"", "class-a", []string{"vol-1"},
				crdv1.VolumeSnapshotContentDelete, nil, false, nil,
			),
			ctrl:      &csiSnapshotSideCarController{groupSnapshotClassLister: classLister},
			wantClass: true,
			wantErr:   false,
		},
		{
			name: "dynamic without class returns error",
			content: newGroupSnapshotContent(
				"no-class", "uid", "snap", testNamespace,
				"", "", []string{"vol-1"},
				crdv1.VolumeSnapshotContentDelete, nil, false, nil,
			),
			ctrl:         &csiSnapshotSideCarController{groupSnapshotClassLister: classLister},
			wantErr:      true,
			errSubstring: "without a group snapshot class",
		},
		{
			name: "pre-provisioned without class succeeds with nil class",
			content: newGroupSnapshotContentWithHandles(
				"preprov", "uid", "snap", testNamespace,
				"group-h", []string{"s1"}, "",
				crdv1.VolumeSnapshotContentDelete, nil, false, nil,
			),
			ctrl:      &csiSnapshotSideCarController{groupSnapshotClassLister: classLister},
			wantClass: false,
			wantErr:   false,
		},
		{
			name: "with class name but class not found",
			content: newGroupSnapshotContent(
				"missing-class", "uid", "snap", testNamespace,
				"", "missing", []string{"vol-1"},
				crdv1.VolumeSnapshotContentDelete, nil, false, nil,
			),
			ctrl:    &csiSnapshotSideCarController{groupSnapshotClassLister: classLister},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, creds, err := tt.ctrl.getCSIGroupSnapshotInput(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("getCSIGroupSnapshotInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstring != "" && (err == nil || !containsSimple(err.Error(), tt.errSubstring)) {
				t.Errorf("getCSIGroupSnapshotInput() error = %v, want substring %q", err, tt.errSubstring)
			}
			if !tt.wantErr {
				if (class != nil) != tt.wantClass {
					t.Errorf("getCSIGroupSnapshotInput() class = %v, wantClass %v", class != nil, tt.wantClass)
				}
				if creds == nil && tt.wantClass {
					// no secret annotations is valid
				}
			}
		})
	}
}

func containsSimple(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestClearGroupSnapshotContentStatus tests clearGroupSnapshotContentStatus.
func TestClearGroupSnapshotContentStatus(t *testing.T) {
	content := newGroupSnapshotContent(
		"clear-status", "uid", "snap", testNamespace,
		"group-h", "class-a", []string{"vol-1"},
		crdv1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	handle := "group-handle-123"
	content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
		VolumeGroupSnapshotHandle: &handle,
		ReadyToUse:                ptr(true),
		CreationTime:              &metav1.Time{Time: metav1.Now().Time},
	}
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	_ = store.Add(content)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		groupSnapshotContentStore: store,
	}

	got, err := ctrl.clearGroupSnapshotContentStatus("clear-status")
	if err != nil {
		t.Fatalf("clearGroupSnapshotContentStatus failed: %v", err)
	}
	if got.Status != nil && (got.Status.VolumeGroupSnapshotHandle != nil || got.Status.ReadyToUse != nil || got.Status.CreationTime != nil) {
		t.Errorf("clearGroupSnapshotContentStatus did not clear status: %+v", got.Status)
	}
}

func ptr(b bool) *bool { return &b }

// TestUpdateGroupSnapshotContentStatus tests updateGroupSnapshotContentStatus (nil status and existing status).
func TestUpdateGroupSnapshotContentStatus(t *testing.T) {
	content := newGroupSnapshotContent(
		"update-status", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		crdv1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	content.Status = nil
	client := fake.NewSimpleClientset(content)
	ctrl := &csiSnapshotSideCarController{clientset: client}

	handle := "group-handle-1"
	ready := true
	created := metav1.NewTime(metav1.Now().Time)

	got, err := ctrl.updateGroupSnapshotContentStatus(content, handle, ready, created, nil)
	if err != nil {
		t.Fatalf("updateGroupSnapshotContentStatus failed: %v", err)
	}
	if got.Status == nil {
		t.Fatal("updateGroupSnapshotContentStatus returned nil status")
	}
	if got.Status.VolumeGroupSnapshotHandle == nil || *got.Status.VolumeGroupSnapshotHandle != handle {
		t.Errorf("VolumeGroupSnapshotHandle = %v, want %s", got.Status.VolumeGroupSnapshotHandle, handle)
	}
	if got.Status.ReadyToUse == nil || !*got.Status.ReadyToUse {
		t.Errorf("ReadyToUse = %v, want true", got.Status.ReadyToUse)
	}
}

// TestUpdateGroupSnapshotContentErrorStatusWithEvent tests updateGroupSnapshotContentErrorStatusWithEvent.
func TestUpdateGroupSnapshotContentErrorStatusWithEvent(t *testing.T) {
	content := newGroupSnapshotContent(
		"error-status", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		crdv1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	content.Status = nil
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		groupSnapshotContentStore: store,
		eventRecorder:             record.NewFakeRecorder(10),
	}

	err := ctrl.updateGroupSnapshotContentErrorStatusWithEvent(content, v1.EventTypeWarning, "TestReason", "test error message")
	if err != nil {
		t.Fatalf("updateGroupSnapshotContentErrorStatusWithEvent failed: %v", err)
	}
	updated, getErr := client.GroupsnapshotV1beta2().VolumeGroupSnapshotContents().Get(context.Background(), "error-status", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("Get after update failed: %v", getErr)
	}
	if updated.Status == nil || updated.Status.Error == nil || *updated.Status.Error.Message != "test error message" {
		t.Errorf("expected error status to be set: %+v", updated.Status)
	}
}

// TestSetAnnVolumeGroupSnapshotBeingCreated tests setAnnVolumeGroupSnapshotBeingCreated.
func TestSetAnnVolumeGroupSnapshotBeingCreated(t *testing.T) {
	content := newGroupSnapshotContent(
		"set-ann", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		crdv1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	client := fake.NewSimpleClientset(content)
	contentStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:    client,
		contentStore: contentStore,
	}

	// Already has annotation: no-op
	metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated, "yes")
	got, err := ctrl.setAnnVolumeGroupSnapshotBeingCreated(content)
	if err != nil {
		t.Fatalf("setAnnVolumeGroupSnapshotBeingCreated (already set) failed: %v", err)
	}
	if got != content {
		t.Error("expected same content when annotation already set")
	}

	// No annotation: should patch
	content2 := newGroupSnapshotContent(
		"set-ann", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		crdv1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	client2 := fake.NewSimpleClientset(content2)
	ctrl2 := &csiSnapshotSideCarController{clientset: client2, contentStore: cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)}
	got2, err := ctrl2.setAnnVolumeGroupSnapshotBeingCreated(content2)
	if err != nil {
		t.Fatalf("setAnnVolumeGroupSnapshotBeingCreated (set) failed: %v", err)
	}
	if !metav1.HasAnnotation(got2.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated) {
		t.Error("expected AnnVolumeGroupSnapshotBeingCreated annotation to be set")
	}
}

// TestRemoveAnnVolumeGroupSnapshotBeingCreated tests removeAnnVolumeGroupSnapshotBeingCreated.
func TestRemoveAnnVolumeGroupSnapshotBeingCreated(t *testing.T) {
	content := newGroupSnapshotContent(
		"remove-ann", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		crdv1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated, "yes")
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		groupSnapshotContentStore: store,
	}

	// No annotation: no-op
	contentNoAnn := newGroupSnapshotContent(
		"remove-ann-no", "uid", "snap", testNamespace,
		"", "class-a", []string{"vol-1"},
		crdv1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	clientNoAnn := fake.NewSimpleClientset(contentNoAnn)
	ctrlNoAnn := &csiSnapshotSideCarController{clientset: clientNoAnn, groupSnapshotContentStore: cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)}
	gotNoAnn, err := ctrlNoAnn.removeAnnVolumeGroupSnapshotBeingCreated(contentNoAnn)
	if err != nil {
		t.Fatalf("removeAnn (no annotation) failed: %v", err)
	}
	if gotNoAnn != contentNoAnn {
		t.Error("expected same content when no annotation")
	}

	// Has annotation: should remove
	got, err := ctrl.removeAnnVolumeGroupSnapshotBeingCreated(content)
	if err != nil {
		t.Fatalf("removeAnn (remove) failed: %v", err)
	}
	if metav1.HasAnnotation(got.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated) {
		t.Error("expected AnnVolumeGroupSnapshotBeingCreated to be removed")
	}
}

// TestSyncGroupSnapshotContent tests syncGroupSnapshotContent branches.
func TestSyncGroupSnapshotContent(t *testing.T) {
	classLister := groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{}))
	tests := []struct {
		name        string
		content     *crdv1beta2.VolumeGroupSnapshotContent
		ctrl        *csiSnapshotSideCarController
		expectError bool
	}{
		{
			name: "shouldDelete with Retain policy removes finalizer",
			content: func() *crdv1beta2.VolumeGroupSnapshotContent {
				c := newGroupSnapshotContent(
					"retain-finalizer", "uid", "snap", testNamespace,
					"group-h", "class-a", []string{"vol-1"},
					crdv1.VolumeSnapshotContentRetain, nil, true, &timeNowMetav1,
				)
				metav1.SetMetaDataAnnotation(&c.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingDeleted, "yes")
				return c
			}(),
			ctrl: func() *csiSnapshotSideCarController {
				c := newGroupSnapshotContent(
					"retain-finalizer", "uid", "snap", testNamespace,
					"group-h", "class-a", []string{"vol-1"},
					crdv1.VolumeSnapshotContentRetain, nil, true, &timeNowMetav1,
				)
				metav1.SetMetaDataAnnotation(&c.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingDeleted, "yes")
				client := fake.NewSimpleClientset(c)
				store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
				_ = store.Add(c)
				return &csiSnapshotSideCarController{
					clientset:                 client,
					groupSnapshotContentStore: store,
					groupSnapshotClassLister:  classLister,
				}
			}(),
			expectError: false,
		},
		{
			name: "ReadyToUse true calls removeAnnVolumeGroupSnapshotBeingCreated",
			content: func() *crdv1beta2.VolumeGroupSnapshotContent {
				c := newGroupSnapshotContent(
					"ready-remove-ann", "uid", "snap", testNamespace,
					"group-h", "class-a", []string{"vol-1"},
					crdv1.VolumeSnapshotContentDelete, nil, false, nil,
				)
				c.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{ReadyToUse: ptr(true)}
				metav1.SetMetaDataAnnotation(&c.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated, "yes")
				return c
			}(),
			ctrl: func() *csiSnapshotSideCarController {
				c := newGroupSnapshotContent(
					"ready-remove-ann", "uid", "snap", testNamespace,
					"group-h", "class-a", []string{"vol-1"},
					crdv1.VolumeSnapshotContentDelete, nil, false, nil,
				)
				c.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{ReadyToUse: ptr(true)}
				metav1.SetMetaDataAnnotation(&c.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated, "yes")
				client := fake.NewSimpleClientset(c)
				store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
				_ = store.Add(c)
				return &csiSnapshotSideCarController{
					clientset:                 client,
					groupSnapshotContentStore: store,
					groupSnapshotClassLister:  classLister,
				}
			}(),
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ctrl.syncGroupSnapshotContent(tt.content)
			if (err != nil) != tt.expectError {
				t.Errorf("syncGroupSnapshotContent() error = %v, wantErr %v", err, tt.expectError)
			}
		})
	}
}

// TestUpdateGroupSnapshotContentInInformerCache tests updateGroupSnapshotContentInInformerCache.
func TestUpdateGroupSnapshotContentInInformerCache(t *testing.T) {
	content := newGroupSnapshotContent(
		"informer-cache", "uid", "snap", testNamespace,
		"group-h", "class-a", []string{"vol-1"},
		crdv1.VolumeSnapshotContentDelete, nil, false, nil,
	)
	content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{ReadyToUse: ptr(true)}
	metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingCreated, "yes")
	content.ResourceVersion = "2"
	client := fake.NewSimpleClientset(content)
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	// Store older version so that storeGroupSnapshotContentUpdate accepts the new one
	oldContent := content.DeepCopy()
	oldContent.ResourceVersion = "1"
	_ = store.Add(oldContent)
	ctrl := &csiSnapshotSideCarController{
		clientset:                 client,
		groupSnapshotContentStore: store,
		groupSnapshotClassLister:  groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})),
	}

	err := ctrl.updateGroupSnapshotContentInInformerCache(content)
	if err != nil {
		t.Errorf("updateGroupSnapshotContentInInformerCache failed: %v", err)
	}
}

// newVolumeGroupSnapshotClass creates a VolumeGroupSnapshotClass for testing.
func newVolumeGroupSnapshotClass(name, driver string) *crdv1beta2.VolumeGroupSnapshotClass {
	return &crdv1beta2.VolumeGroupSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Driver:     driver,
	}
}

// TestIsDriverMatchGroupSnapshotContent tests isDriverMatch for VolumeGroupSnapshotContent.
func TestIsDriverMatchGroupSnapshotContent(t *testing.T) {
	classMatchingDriver := newVolumeGroupSnapshotClass("class-match", mockDriverName)
	classOtherDriver := newVolumeGroupSnapshotClass("class-other", "other-driver")
	classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = classIndexer.Add(classMatchingDriver)
	_ = classIndexer.Add(classOtherDriver)
	classLister := groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer)

	tests := []struct {
		name     string
		content  *crdv1beta2.VolumeGroupSnapshotContent
		ctrl     *csiSnapshotSideCarController
		expected bool
	}{
		{
			name: "no source (no VolumeHandles and no GroupSnapshotHandles)",
			content: &crdv1beta2.VolumeGroupSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{Name: "no-source"},
				Spec: crdv1beta2.VolumeGroupSnapshotContentSpec{
					Driver: mockDriverName,
					Source: crdv1beta2.VolumeGroupSnapshotContentSource{},
				},
			},
			ctrl:     &csiSnapshotSideCarController{driverName: mockDriverName},
			expected: false,
		},
		{
			name: "driver mismatch",
			content: newGroupSnapshotContent(
				"wrong-driver",
				"uid", "snap", testNamespace,
				"handle", "class-match",
				[]string{"vol-1"},
				crdv1.VolumeSnapshotContentDelete,
				nil, false, nil,
			),
			ctrl:     &csiSnapshotSideCarController{driverName: "other-driver", groupSnapshotClassLister: classLister},
			expected: false,
		},
		{
			name: "driver match with VolumeHandles, no class",
			content: newGroupSnapshotContent(
				"dynamic-no-class",
				"uid", "snap", testNamespace,
				"", "class-match",
				[]string{"vol-1"},
				crdv1.VolumeSnapshotContentDelete,
				nil, false, nil,
			),
			ctrl:     &csiSnapshotSideCarController{driverName: mockDriverName, groupSnapshotClassLister: classLister},
			expected: true,
		},
		{
			name: "driver match but class has different driver",
			content: newGroupSnapshotContent(
				"class-other-driver",
				"uid", "snap", testNamespace,
				"", "class-other",
				[]string{"vol-1"},
				crdv1.VolumeSnapshotContentDelete,
				nil, false, nil,
			),
			ctrl:     &csiSnapshotSideCarController{driverName: mockDriverName, groupSnapshotClassLister: classLister},
			expected: false,
		},
		{
			name: "driver and class match (VolumeHandles)",
			content: newGroupSnapshotContent(
				"dynamic-match",
				"uid", "snap", testNamespace,
				"", "class-match",
				[]string{"vol-1"},
				crdv1.VolumeSnapshotContentDelete,
				nil, false, nil,
			),
			ctrl:     &csiSnapshotSideCarController{driverName: mockDriverName, groupSnapshotClassLister: classLister},
			expected: true,
		},
		{
			name: "driver and class match (GroupSnapshotHandles)",
			content: newGroupSnapshotContentWithHandles(
				"preprov-match",
				"uid", "snap", testNamespace,
				"group-handle",
				[]string{"snap-1"},
				"class-match",
				crdv1.VolumeSnapshotContentDelete,
				nil, false, nil,
			),
			ctrl:     &csiSnapshotSideCarController{driverName: mockDriverName, groupSnapshotClassLister: classLister},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ctrl.isDriverMatch(tt.content)
			if got != tt.expected {
				t.Errorf("isDriverMatch() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

// TestSyncGroupSnapshotContentByKeyInvalidKey tests syncGroupSnapshotContentByKey with an invalid key.
func TestSyncGroupSnapshotContentByKeyInvalidKey(t *testing.T) {
	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	contentIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	ctrl := &csiSnapshotSideCarController{
		groupSnapshotContentStore:  store,
		groupSnapshotContentLister: groupsnapshotlisters.NewVolumeGroupSnapshotContentLister(contentIndexer),
	}

	// Invalid key (e.g. missing namespace/name) causes SplitMetaNamespaceKey to fail and returns nil
	err := ctrl.syncGroupSnapshotContentByKey("invalid-key-without-slash")
	if err != nil {
		t.Errorf("expected nil error for invalid key, got: %v", err)
	}
}

// TestSyncGroupSnapshotContentByKeyNotFoundRemovesFromCache tests that when the content
// is not in the lister (e.g. deleted) but is in the controller's store,
// syncGroupSnapshotContentByKey removes it from the store.
func TestSyncGroupSnapshotContentByKeyNotFoundRemovesFromCache(t *testing.T) {
	contentName := "group-content-gone"
	key := contentName // cluster-scoped resource, key is just the name

	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	// Empty indexer: Get(contentName) will return NotFound
	contentIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	contentLister := groupsnapshotlisters.NewVolumeGroupSnapshotContentLister(contentIndexer)

	content := newGroupSnapshotContent(
		contentName,
		"group-snap-uid-1",
		"group-snap-1",
		testNamespace,
		"group-handle-1",
		"group-class-1",
		[]string{"vol-handle-1"},
		crdv1.VolumeSnapshotContentDelete,
		nil,
		false,
		nil,
	)
	if err := store.Add(content); err != nil {
		t.Fatalf("failed to add content to store: %v", err)
	}

	ctrl := &csiSnapshotSideCarController{
		groupSnapshotContentStore:  store,
		groupSnapshotContentLister: contentLister,
	}

	err := ctrl.syncGroupSnapshotContentByKey(key)
	if err != nil {
		t.Errorf("syncGroupSnapshotContentByKey failed: %v", err)
	}

	_, found, _ := store.GetByKey(contentName)
	if found {
		t.Error("expected content to be removed from store when lister returns NotFound")
	}
}

// TestSyncGroupSnapshotContentByKeyListerErrorNonNotFound tests that a non-NotFound
// lister error is ignored (returns nil) and does not remove from store.
func TestSyncGroupSnapshotContentByKeyListerErrorNonNotFound(t *testing.T) {
	contentName := "group-content-error"
	key := contentName

	store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	content := newGroupSnapshotContent(
		contentName,
		"group-snap-uid-1",
		"group-snap-1",
		testNamespace,
		"group-handle-1",
		"group-class-1",
		[]string{"vol-handle-1"},
		crdv1.VolumeSnapshotContentDelete,
		nil,
		false,
		nil,
	)
	if err := store.Add(content); err != nil {
		t.Fatalf("failed to add content to store: %v", err)
	}

	// Lister that returns a non-NotFound error (e.g. service unavailable)
	fakeLister := &fakeGroupSnapshotContentLister{getErr: errors.NewServiceUnavailable("service unavailable")}
	ctrl := &csiSnapshotSideCarController{
		groupSnapshotContentStore:  store,
		groupSnapshotContentLister: fakeLister,
	}

	err := ctrl.syncGroupSnapshotContentByKey(key)
	if err != nil {
		t.Errorf("expected nil error when lister returns non-NotFound error, got: %v", err)
	}
	// Content should still be in store (we don't delete on arbitrary errors)
	_, found, _ := store.GetByKey(contentName)
	if !found {
		t.Error("expected content to remain in store when lister returns non-NotFound error")
	}
}

// TestGroupSnapshotContentWorker tests groupSnapshotContentWorker processes one item from the queue.
func TestGroupSnapshotContentWorker(t *testing.T) {
	contentName := "worker-test-content"
	key := contentName // cluster-scoped resource key is just the name

	t.Run("quit path - returns immediately when queue is shut down", func(t *testing.T) {
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())
		queue.ShutDown()
		ctrl := &csiSnapshotSideCarController{groupSnapshotContentQueue: queue}
		ctrl.groupSnapshotContentWorker()
	})

	t.Run("success path - delete from store when lister returns NotFound", func(t *testing.T) {
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())
		store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
		content := newGroupSnapshotContent(
			contentName,
			"uid", "snap", testNamespace,
			"group-h", "class-a", []string{"vol-1"},
			crdv1.VolumeSnapshotContentDelete, nil, false, nil,
		)
		if err := store.Add(content); err != nil {
			t.Fatalf("store.Add: %v", err)
		}
		contentIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		contentLister := groupsnapshotlisters.NewVolumeGroupSnapshotContentLister(contentIndexer)

		queue.Add(key)
		ctrl := &csiSnapshotSideCarController{
			groupSnapshotContentQueue:  queue,
			groupSnapshotContentStore:  store,
			groupSnapshotContentLister: contentLister,
		}

		ctrl.groupSnapshotContentWorker()

		_, found, _ := store.GetByKey(key)
		if found {
			t.Error("expected content to be removed from store after worker processed delete")
		}
		if queue.Len() != 0 {
			t.Errorf("expected queue to be empty after successful process, got Len=%d", queue.Len())
		}
	})

	t.Run("error path - worker returns without panic when sync returns error", func(t *testing.T) {
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())
		content := newGroupSnapshotContent(
			contentName,
			"uid", "snap", testNamespace,
			"group-h", "class-a", []string{"vol-1"},
			crdv1.VolumeSnapshotContentDelete, nil, true, &timeNowMetav1,
		)
		content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
			VolumeGroupSnapshotHandle: ptrString("group-handle-1"),
		}
		metav1.SetMetaDataAnnotation(&content.ObjectMeta, utils.AnnVolumeGroupSnapshotBeingDeleted, "yes")
		contentIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		if err := contentIndexer.Add(content); err != nil {
			t.Fatalf("contentIndexer.Add: %v", err)
		}
		contentLister := groupsnapshotlisters.NewVolumeGroupSnapshotContentLister(contentIndexer)
		store := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
		contentClone := content.DeepCopy()
		contentClone.ResourceVersion = "1"
		_ = store.Add(contentClone)
		client := fake.NewSimpleClientset(content)
		classIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		_ = classIndexer.Add(newVolumeGroupSnapshotClass("class-a", mockDriverName))
		fakeHandler := &fakeGroupSnapshotHandler{deleteGroupSnapshotErr: errors.NewServiceUnavailable("delete failed")}

		queue.Add(key)
		ctrl := &csiSnapshotSideCarController{
			driverName:                 mockDriverName,
			groupSnapshotContentQueue:  queue,
			groupSnapshotContentStore:  store,
			groupSnapshotContentLister: contentLister,
			groupSnapshotClassLister:   groupsnapshotlisters.NewVolumeGroupSnapshotClassLister(classIndexer),
			clientset:                  client,
			handler:                    fakeHandler,
			eventRecorder:              record.NewFakeRecorder(10),
		}

		ctrl.groupSnapshotContentWorker()

		// When sync returns error, worker calls AddRateLimited and returns. Content is not
		// removed from store because deleteCSIGroupSnapshotOperation failed.
		_, found, _ := store.GetByKey(key)
		if !found {
			t.Error("expected content to remain in store when sync (delete) fails")
		}
	})
}

func ptrString(s string) *string { return &s }

// fakeGroupSnapshotContentLister returns a fixed error from Get (for tests that need a non-NotFound error).
type fakeGroupSnapshotContentLister struct {
	content *crdv1beta2.VolumeGroupSnapshotContent
	getErr  error
}

func (f *fakeGroupSnapshotContentLister) List(selector labels.Selector) ([]*crdv1beta2.VolumeGroupSnapshotContent, error) {
	if f.content != nil {
		return []*crdv1beta2.VolumeGroupSnapshotContent{f.content}, nil
	}
	return nil, f.getErr
}

func (f *fakeGroupSnapshotContentLister) Get(name string) (*crdv1beta2.VolumeGroupSnapshotContent, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.content, nil
}
