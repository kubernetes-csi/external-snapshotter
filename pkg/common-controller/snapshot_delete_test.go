/*
Copyright 2018 The Kubernetes Authors.

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

package common_controller

import (
	"errors"
	"testing"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var class1Parameters = map[string]string{
	"param1": "value1",
}

var class2Parameters = map[string]string{
	"param2": "value2",
}

var class3Parameters = map[string]string{
	"param3":                               "value3",
	utils.PrefixedSnapshotterSecretNameKey: "name",
}

var class4Parameters = map[string]string{
	// utils.SnapshotterSecretNameKey:      "emptysecret",
	// utils.SnapshotterSecretNamespaceKey: "default",
}

var class5Parameters = map[string]string{
	utils.PrefixedSnapshotterSecretNameKey:      "secret",
	utils.PrefixedSnapshotterSecretNamespaceKey: "default",
}

var timeNowMetav1 = metav1.Now()

var (
	content31 = "content3-1"
	claim31   = "claim3-1"
)

var snapshotClasses = []*crdv1.VolumeSnapshotClass{
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: classGold,
		},
		Driver:         mockDriverName,
		Parameters:     class1Parameters,
		DeletionPolicy: crdv1.VolumeSnapshotContentDelete,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: classSilver,
		},
		Driver:         mockDriverName,
		Parameters:     class2Parameters,
		DeletionPolicy: crdv1.VolumeSnapshotContentDelete,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: emptySecretClass,
		},
		Driver:         mockDriverName,
		Parameters:     class4Parameters,
		DeletionPolicy: crdv1.VolumeSnapshotContentDelete,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: invalidSecretClass,
		},
		Driver:         mockDriverName,
		Parameters:     class3Parameters,
		DeletionPolicy: crdv1.VolumeSnapshotContentDelete,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: validSecretClass,
		},
		Driver:         mockDriverName,
		Parameters:     class5Parameters,
		DeletionPolicy: crdv1.VolumeSnapshotContentDelete,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        defaultClass,
			Annotations: map[string]string{utils.IsDefaultSnapshotClassAnnotation: "true"},
		},
		Driver:         mockDriverName,
		DeletionPolicy: crdv1.VolumeSnapshotContentDelete,
	},
}

// Test single call to syncContent, expecting deleting to happen.
// 1. Fill in the controller with initial data
// 2. Call the syncContent *once*.
// 3. Compare resulting contents with expected contents.
func TestDeleteSync(t *testing.T) {
	tests := []controllerTest{
		{
			name:              "1-1 - noop: content will not be deleted if it is bound to a snapshot correctly, snapshot uid is not specified",
			initialContents:   newContentArray("content1-1", "", "snap1-1", "snaphandle1-1", validSecretClass, "snaphandle1-1", "", deletePolicy, nil, nil, true),
			expectedContents:  newContentArray("content1-1", "", "snap1-1", "snaphandle1-1", validSecretClass, "snaphandle1-1", "", deletePolicy, nil, nil, true),
			initialSnapshots:  newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "", validSecretClass, "content1-1", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap1-1", "snapuid1-1", "claim1-1", "", validSecretClass, "content1-1", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncContent,
		},
		{
			// delete success - content is deleted before doDelete() starts
			name:              "1-2 - content is deleted before deleting",
			initialContents:   newContentArray("content1-2", "sid1-2", "snap1-2", "sid1-2", validSecretClass, "", "", deletionPolicy, nil, nil, true),
			expectedContents:  nocontents,
			initialSnapshots:  nosnapshots,
			expectedSnapshots: nosnapshots,
			initialSecrets:    []*v1.Secret{secret()},
			expectedEvents:    noevents,
			errors:            noerrors,
			test: wrapTestWithInjectedOperation(testSyncContent, func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor) {
				// Delete the volume before delete operation starts
				reactor.lock.Lock()
				delete(reactor.contents, "content1-2")
				reactor.lock.Unlock()
			}),
		},
		{
			name:              "1-3 - will not delete content with retain policy set which is bound to a snapshot incorrectly",
			initialContents:   newContentArray("content1-3", "snapuid1-3-x", "snap1-3", "snaphandle1-3", validSecretClass, "snaphandle1-3", "", retainPolicy, nil, nil, true),
			expectedContents:  newContentArray("content1-3", "snapuid1-3-x", "snap1-3", "snaphandle1-3", validSecretClass, "snaphandle1-3", "", retainPolicy, nil, nil, true),
			initialSnapshots:  newSnapshotArray("snap1-3", "snapuid1-3", "claim1-3", "", validSecretClass, "content1-3", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap1-3", "snapuid1-3", "claim1-3", "", validSecretClass, "content1-3", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncContent,
		},
		{
			name:             "3-1 - (dynamic) content will be deleted if snapshot deletion timestamp is set",
			initialContents:  newContentArray("snapcontent-snapuid3-1", "snapuid3-1", "snap3-1", "sid3-1", validSecretClass, "", "volume3-1", deletePolicy, nil, nil, true),
			expectedContents: nocontents,
			initialSnapshots: newSnapshotArray("snap3-1", "snapuid3-1", "claim3-1", "", validSecretClass, "snapcontent-snapuid3-1", &True, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: withSnapshotFinalizers(newSnapshotArray("snap3-1", "snapuid3-1", "claim3-1", "", validSecretClass, "snapcontent-snapuid3-1", &True, nil, nil, nil, false, false, &timeNowMetav1),
				utils.VolumeSnapshotBoundFinalizer,
			),
			initialClaims:  newClaimArray("claim3-1", "pvc-uid3-1", "1Gi", "volume3-1", v1.ClaimBound, &classEmpty),
			expectedEvents: noevents,
			initialSecrets: []*v1.Secret{secret()},
			errors:         noerrors,
			test:           testSyncSnapshot,
		},
		{
			name:            "3-2 - (dynamic) content will not be deleted if deletion API call fails",
			initialContents: newContentArray("snapcontent-snapuid3-2", "snapuid3-2", "snap3-2", "sid3-2", validSecretClass, "", "volume3-2", deletePolicy, nil, nil, true),
			expectedContents: withContentAnnotations(newContentArray("snapcontent-snapuid3-2", "snapuid3-2", "snap3-2", "sid3-2", validSecretClass, "", "volume3-2", deletePolicy, nil, nil, true),
				map[string]string{
					"snapshot.storage.kubernetes.io/volumesnapshot-being-deleted": "yes",
				}),
			initialSnapshots:  newSnapshotArray("snap3-2", "snapuid3-2", "claim3-2", "", validSecretClass, "snapcontent-snapuid3-2", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-2", "snapuid3-2", "claim3-2", "", validSecretClass, "snapcontent-snapuid3-2", &False, nil, nil, nil, false, true, &timeNowMetav1),
			initialClaims:     newClaimArray("claim3-2", "pvc-uid3-2", "1Gi", "volume3-2", v1.ClaimBound, &classEmpty),
			expectedEvents:    []string{"Warning SnapshotContentObjectDeleteError"},
			initialSecrets:    []*v1.Secret{secret()},
			errors: []reactorError{
				// Inject error to the first client.VolumesnapshotV1().VolumeSnapshotContents().Delete call.
				// All other calls will succeed.
				{"delete", "volumesnapshotcontents", errors.New("mock delete error")},
			},
			expectSuccess: false,
			test:          testSyncSnapshotError,
		},
		{
			name:            "3-3 - (dynamic) content will not be deleted if retainPolicy is set, snapshot should have its finalizer removed",
			initialContents: newContentArray("snapcontent-snapuid3-3", "snapuid3-3", "snap3-3", "sid3-3", validSecretClass, "", "volume3-3", retainPolicy, nil, nil, true),
			expectedContents: withContentAnnotations(newContentArray("snapcontent-snapuid3-3", "snapuid3-3", "snap3-3", "sid3-3", validSecretClass, "", "volume3-3", retainPolicy, nil, nil, true),
				map[string]string{
					"snapshot.storage.kubernetes.io/volumesnapshot-being-deleted": "yes",
				}),
			initialSnapshots:  newSnapshotArray("snap3-3", "snapuid3-3", "claim3-3", "", validSecretClass, "snapcontent-snapuid3-3", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-3", "snapuid3-3", "claim3-3", "", validSecretClass, "snapcontent-snapuid3-3", &False, nil, nil, nil, false, false, &timeNowMetav1),
			initialClaims:     newClaimArray("claim3-3", "pvc-uid3-3", "1Gi", "volume3-3", v1.ClaimBound, &classEmpty),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-4 - (dynamic) snapshot should have its finalizer removed if no content has been found",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-4", "snapuid3-4", "claim3-4", "", validSecretClass, "", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-4", "snapuid3-4", "claim3-4", "", validSecretClass, "", &False, nil, nil, nil, false, false, &timeNowMetav1),
			initialClaims:     newClaimArray("claim3-4", "pvc-uid3-4", "1Gi", "volume3-4", v1.ClaimBound, &classEmpty),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-5 - (dynamic) snapshot should have its finalizer removed if a content is found but points to a different snapshot - uid mismatch",
			initialContents:   newContentArray("snapcontent-snapuid3-5", "snapuid3-5-x", "snap3-5", "sid3-5", validSecretClass, "", "volume3-5", deletePolicy, nil, nil, true),
			expectedContents:  newContentArray("snapcontent-snapuid3-5", "snapuid3-5-x", "snap3-5", "sid3-5", validSecretClass, "", "volume3-5", deletePolicy, nil, nil, true),
			initialSnapshots:  newSnapshotArray("snap3-5", "snapuid3-5", "claim3-5", "", validSecretClass, "snapcontent-snapuid3-5", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-5", "snapuid3-5", "claim3-5", "", validSecretClass, "snapcontent-snapuid3-5", &False, nil, nil, nil, false, false, &timeNowMetav1),
			initialClaims:     newClaimArray("claim3-5", "pvc-uid3-5", "1Gi", "volume3-5", v1.ClaimBound, &classEmpty),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-6 - (dynamic) snapshot should have its finalizer removed if a content is found but points to a different snapshot - name mismatch",
			initialContents:   newContentArray("snapcontent-snapuid3-6", "snapuid3-6", "snap3-6-x", "sid3-6", validSecretClass, "", "volume3-6", deletePolicy, nil, nil, true),
			expectedContents:  newContentArray("snapcontent-snapuid3-6", "snapuid3-6", "snap3-6-x", "sid3-6", validSecretClass, "", "volume3-6", deletePolicy, nil, nil, true),
			initialSnapshots:  newSnapshotArray("snap3-6", "snapuid3-6", "claim3-6", "", validSecretClass, "snapcontent-snapuid3-6", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-6", "snapuid3-6", "claim3-6", "", validSecretClass, "snapcontent-snapuid3-6", &False, nil, nil, nil, false, false, &timeNowMetav1),
			initialClaims:     newClaimArray("claim3-6", "pvc-uid3-6", "1Gi", "volume3-6", v1.ClaimBound, &classEmpty),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:             "3-7 - (static) content will be deleted if snapshot deletion timestamp is set, snapshot should have its finalizers removed",
			initialContents:  newContentArray("content-3-7", "snapuid3-7", "snap3-7", "sid3-7", validSecretClass, "sid3-7", "", deletePolicy, nil, nil, true),
			expectedContents: nocontents,
			initialSnapshots: newSnapshotArray("snap3-7", "snapuid3-7", "", "content-3-7", validSecretClass, "content-3-7", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: withSnapshotFinalizers(newSnapshotArray("snap3-7", "snapuid3-7", "", "content-3-7", validSecretClass, "content-3-7", &False, nil, nil, nil, false, false, &timeNowMetav1),
				utils.VolumeSnapshotBoundFinalizer,
			),
			expectedEvents: noevents,
			initialSecrets: []*v1.Secret{secret()},
			errors:         noerrors,
			test:           testSyncSnapshot,
		},
		{
			name:            "3-8 - (static) content will not be deleted if deletion API call fails, snapshot should have its finalizers remained",
			initialContents: newContentArray("content-3-8", "snapuid3-8", "snap3-8", "sid3-8", validSecretClass, "sid3-8", "", deletePolicy, nil, nil, true),
			expectedContents: withContentAnnotations(newContentArray("content-3-8", "snapuid3-8", "snap3-8", "sid3-8", validSecretClass, "sid3-8", "", deletePolicy, nil, nil, true),
				map[string]string{
					"snapshot.storage.kubernetes.io/volumesnapshot-being-deleted": "yes",
				}),
			initialSnapshots:  newSnapshotArray("snap3-8", "snapuid3-8", "", "content-3-8", validSecretClass, "content-3-8", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-8", "snapuid3-8", "", "content-3-8", validSecretClass, "content-3-8", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedEvents:    []string{"Warning SnapshotContentObjectDeleteError"},
			initialSecrets:    []*v1.Secret{secret()},
			errors: []reactorError{
				// Inject error to the first client.VolumesnapshotV1().VolumeSnapshotContents().Delete call.
				// All other calls will succeed.
				{"delete", "volumesnapshotcontents", errors.New("mock delete error")},
			},
			expectSuccess: false,
			test:          testSyncSnapshotError,
		},
		{
			name:            "3-9 - (static) content will not be deleted if retainPolicy is set, snapshot should have its finalizer removed",
			initialContents: newContentArray("content-3-9", "snapuid3-9", "snap3-9", "sid3-9", validSecretClass, "sid3-9", "", retainPolicy, nil, nil, true),
			expectedContents: withContentAnnotations(newContentArray("content-3-9", "snapuid3-9", "snap3-9", "sid3-9", validSecretClass, "sid3-9", "", retainPolicy, nil, nil, true),
				map[string]string{
					"snapshot.storage.kubernetes.io/volumesnapshot-being-deleted": "yes",
				}),
			initialSnapshots:  newSnapshotArray("snap3-9", "snapuid3-9", "", "content-3-9", validSecretClass, "content-3-9", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-9", "snapuid3-9", "", "content-3-9", validSecretClass, "content-3-9", &False, nil, nil, nil, false, false, &timeNowMetav1),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-10 - (static) snapshot should have its finalizer removed if no content has been found",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-10", "snapuid3-10", "", "content-3-10", validSecretClass, "", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-10", "snapuid3-10", "", "content-3-10", validSecretClass, "", &False, nil, nil, nil, false, false, &timeNowMetav1),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-11 - (static) snapshot should have its finalizer removed if a content is found but points to a different snapshot - uid mismatch",
			initialContents:   newContentArray("content-3-11", "snapuid3-11-x", "snap3-11", "sid3-11", validSecretClass, "sid3-11", "", deletePolicy, nil, nil, true),
			expectedContents:  newContentArray("content-3-11", "snapuid3-11-x", "snap3-11", "sid3-11", validSecretClass, "sid3-11", "", deletePolicy, nil, nil, true),
			initialSnapshots:  newSnapshotArray("snap3-11", "snapuid3-11", "", "content-3-11", validSecretClass, "content-3-11", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-11", "snapuid3-11", "", "content-3-11", validSecretClass, "content-3-11", &False, nil, nil, nil, false, false, &timeNowMetav1),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-12 - (static) snapshot should have its finalizer removed if a content is found but points to a different snapshot - name mismatch",
			initialContents:   newContentArray("content-3-12", "snapuid3-12", "snap3-12-x", "sid3-12", validSecretClass, "sid3-12", "", deletePolicy, nil, nil, true),
			expectedContents:  newContentArray("content-3-12", "snapuid3-12", "snap3-12-x", "sid3-12", validSecretClass, "sid3-12", "", deletePolicy, nil, nil, true),
			initialSnapshots:  newSnapshotArray("snap3-12", "snapuid3-12", "", "content-3-12", validSecretClass, "content-3-12", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap3-12", "snapuid3-12", "", "content-3-12", validSecretClass, "content-3-12", &False, nil, nil, nil, false, false, &timeNowMetav1),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
	}
	runSyncTests(t, tests, snapshotClasses)
}
