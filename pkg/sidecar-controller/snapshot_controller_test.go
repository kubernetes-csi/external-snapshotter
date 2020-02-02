/*
Copyright 2019 The Kubernetes Authors.
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
	"reflect"
	"testing"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/clientset/versioned/fake"
	storagelisters "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/listers/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/v2/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

var deletionPolicy = crdv1.VolumeSnapshotContentDelete

func storeVersion(t *testing.T, prefix string, c cache.Store, version string, expectedReturn bool) {
	content := newContent("contentName", "snapuid1-1", "snap1-1", "sid1-1", classGold, "", "pv-handle-1-1", deletionPolicy, nil, nil, false, nil)
	content.ResourceVersion = version
	ret, err := utils.StoreObjectUpdate(c, content, "content")
	if err != nil {
		t.Errorf("%s: expected storeObjectUpdate to succeed, got: %v", prefix, err)
	}
	if expectedReturn != ret {
		t.Errorf("%s: expected storeObjectUpdate to return %v, got: %v", prefix, expectedReturn, ret)
	}

	// find the stored version

	contentObj, found, err := c.GetByKey("contentName")
	if err != nil {
		t.Errorf("expected content 'contentName' in the cache, got error instead: %v", err)
	}
	if !found {
		t.Errorf("expected content 'contentName' in the cache but it was not found")
	}
	content, ok := contentObj.(*crdv1.VolumeSnapshotContent)
	if !ok {
		t.Errorf("expected content in the cache, got different object instead: %#v", contentObj)
	}

	if ret {
		if content.ResourceVersion != version {
			t.Errorf("expected content with version %s in the cache, got %s instead", version, content.ResourceVersion)
		}
	} else {
		if content.ResourceVersion == version {
			t.Errorf("expected content with version other than %s in the cache, got %s instead", version, content.ResourceVersion)
		}
	}
}

// TestControllerCache tests func storeObjectUpdate()
func TestControllerCache(t *testing.T) {
	// Cache under test
	c := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)

	// Store new PV
	storeVersion(t, "Step1", c, "1", true)
	// Store the same PV
	storeVersion(t, "Step2", c, "1", true)
	// Store newer PV
	storeVersion(t, "Step3", c, "2", true)
	// Store older PV - simulating old "PV updated" event or periodic sync with
	// old data
	storeVersion(t, "Step4", c, "1", false)
	// Store newer PV - test integer parsing ("2" > "10" as string,
	// while 2 < 10 as integers)
	storeVersion(t, "Step5", c, "10", true)
}

func TestControllerCacheParsingError(t *testing.T) {
	c := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)
	// There must be something in the cache to compare with
	storeVersion(t, "Step1", c, "1", true)
	content := newContent("contentName", "snapuid1-1", "snap1-1", "sid1-1", classGold, "", "pv-handle-1-1", deletionPolicy, nil, nil, false, nil)
	content.ResourceVersion = "xxx"
	_, err := utils.StoreObjectUpdate(c, content, "content")
	if err == nil {
		t.Errorf("Expected parsing error, got nil instead")
	}
}

// TestShouldDelete tests logic for deleting VolumeSnapshotContent objects.
func TestShouldDelete(t *testing.T) {
	// Use an empty controller, since there's no struct
	// state we need to use in this test.
	ctrl := &csiSnapshotSideCarController{}

	tests := []struct {
		name           string
		expectedReturn bool
		content        *crdv1.VolumeSnapshotContent
	}{
		{
			name:           "DeletionTimeStamp is nil",
			expectedReturn: false,
			content:        newContent("test-content", "snap-uuid", "snapName", "desiredHandle", "default", "desiredHandle", "volHandle", crdv1.VolumeSnapshotContentDelete, nil, &defaultSize, false, nil),
		},
		{
			name:           "Content is not bound",
			expectedReturn: true,
			content:        newContent("test-content-not-bound", "", "", "snapshotHandle", "", "", "", crdv1.VolumeSnapshotContentDelete, nil, &defaultSize, false, &timeNowMetav1),
		},
		{
			name:           "AnnVolumeSnapshotBeingDeleted annotation is set. ",
			expectedReturn: true,
			// DeletionTime means that annotation is set, and being bound means the other cases are skipped.
			content: newContent("test-content", "snap-uuid", "snapName", "desiredHandle", "default", "desiredHandle", "volHandle", crdv1.VolumeSnapshotContentDelete, nil, &defaultSize, false, &timeNowMetav1),
		},
		{
			name:           "If no other cases match, then should not delete",
			expectedReturn: false,
			// Use an object that does not conform to newContent's logic in order to skip the conditionals inside shouldDelete
			content: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-content",
					DeletionTimestamp: &timeNowMetav1,
				},
			},
		},
	}

	for _, test := range tests {
		result := ctrl.shouldDelete(test.content)

		if result != test.expectedReturn {
			t.Errorf("Got %t but expected %t for test: %s", result, test.expectedReturn, test.name)
		}

	}
}

func TestCheckandUpdateContentStatus(t *testing.T) {
	tests := []struct {
		name                string
		initialContent      *crdv1.VolumeSnapshotContent
		initialSecret       *v1.Secret
		snapshotHandle      string
		expectedContent     *crdv1.VolumeSnapshotContent
		expectedReadyToUse  bool
		expectedError       error
		expectedCreateCalls []createCall
		expectedListCalls   []listCall
	}{
		{
			name:           "Update volumesnapshotcontent status",
			initialContent: newContent("test-content", "snap-uuid", "snapName", "desiredHandle", defaultClass, "desiredHandle", "volHandle", crdv1.VolumeSnapshotContentDelete, nil, &defaultSize, false, nil),
			initialSecret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"foo": []byte("bar"),
				},
			},
			snapshotHandle:     "snapName",
			expectedContent:    newContent("test-content", "snap-uuid", "snapName", "desiredHandle", defaultClass, "desiredHandle", "volHandle", crdv1.VolumeSnapshotContentDelete, nil, &defaultSize, false, nil),
			expectedReadyToUse: true,
			expectedError:      nil,
			expectedListCalls:  []listCall{{"desiredHandle", true, time.Now(), 1, map[string]string{"foo": "bar"}, nil}},
		},
		{
			name: "Create and update volumesnapshotcontent",
			initialContent: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "snapuid1-1",
				},
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: &defaultClass,
					VolumeSnapshotRef: v1.ObjectReference{
						Kind:       "VolumeSnapshot",
						APIVersion: "snapshot.storage.k8s.io/v1beta1",
						UID:        types.UID("snap-uuid"),
						Namespace:  testNamespace,
						Name:       "snapName",
					},
					Source: crdv1.VolumeSnapshotContentSource{
						VolumeHandle: (func(s string) *string {
							return &s
						})("volHandle"),
					},
				},
			},
			expectedContent:    newContent("snapuid1-1", "snap-uuid", "snapName", "desiredHandle", defaultClass, "desiredHandle", "volHandle", crdv1.VolumeSnapshotContentDelete, nil, &defaultSize, false, nil),
			expectedReadyToUse: true,
			expectedError:      nil,
			expectedCreateCalls: []createCall{{
				volumeHandle: "volHandle",
				snapshotName: "snapshot-snap-uuid",
				driverName:   mockDriverName,
				snapshotId:   "snapuid1-1",
				creationTime: timeNow,
				readyToUse:   true,
			}},
			expectedListCalls: []listCall{{"desiredHandle", true, time.Now(), 1, map[string]string{"foo": "bar"}, nil}},
		},
	}

	for _, test := range tests {
		// Initialize the controller
		kubeClient := &kubefake.Clientset{}
		client := &fake.Clientset{}

		ctrl, err := newTestController(kubeClient, client, nil, t, controllerTest{
			expectedCreateCalls: test.expectedCreateCalls,
			expectedListCalls:   test.expectedListCalls,
		})

		if err != nil {
			t.Fatalf("Test %q construct persistent content failed: %v", test.name, err)
		}

		if test.snapshotHandle != "" {
			test.initialContent.Spec.Source.SnapshotHandle = &test.snapshotHandle
			test.expectedContent.Spec.Source.SnapshotHandle = &test.snapshotHandle
		}

		if test.initialSecret != nil {
			test.initialContent.ObjectMeta.Annotations = secretAnnotations()
			test.expectedContent.ObjectMeta.Annotations = secretAnnotations()
		}

		// Setup reactor
		reactor := newSnapshotReactor(kubeClient, client, ctrl, nil, nil, nil)
		if ctrl.isDriverMatch(test.initialContent) {
			ctrl.contentStore.Add(test.initialContent)
			reactor.contents[test.initialContent.Name] = test.initialContent
			reactor.secrets[test.initialSecret.ObjectMeta.Name] = test.initialSecret
		}

		if test.initialContent.Spec.Source.SnapshotHandle == nil {
			reactor.contents[test.expectedContent.Name] = test.expectedContent
		}

		// Inject classes into controller via a custom lister.
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		for _, class := range snapshotClasses {
			indexer.Add(class)
		}
		ctrl.classLister = storagelisters.NewVolumeSnapshotClassLister(indexer)

		// Run test
		content, err := ctrl.checkandUpdateContentStatusOperation(test.initialContent)
		if !reflect.DeepEqual(err, test.expectedError) {
			t.Errorf("error check failed for test %s: %s", test.name, diff.ObjectDiff(test.expectedError, err))
		}

		test.expectedContent.Status.CreationTime = content.Status.CreationTime
		test.expectedContent.ObjectMeta.ResourceVersion = content.ObjectMeta.ResourceVersion
		test.expectedContent.Status.ReadyToUse = &test.expectedReadyToUse

		if !reflect.DeepEqual(content, test.expectedContent) {
			t.Errorf("content check failed for test %s: %s", test.name, diff.ObjectDiff(test.expectedContent, content))
		}
	}
}
