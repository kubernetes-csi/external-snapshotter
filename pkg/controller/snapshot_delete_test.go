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

package controller

import (
	"errors"
	"testing"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var class1Parameters = map[string]string{
	"param1": "value1",
}

var class2Parameters = map[string]string{
	"param2": "value2",
}

var class3Parameters = map[string]string{
	"param3":                 "value3",
	snapshotterSecretNameKey: "name",
}

var class4Parameters = map[string]string{
	snapshotterSecretNameKey:      "emptysecret",
	snapshotterSecretNamespaceKey: "default",
}

var class5Parameters = map[string]string{
	snapshotterSecretNameKey:      "secret",
	snapshotterSecretNamespaceKey: "default",
}

var snapshotClasses = []*crdv1.VolumeSnapshotClass{
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: classGold,
		},
		Snapshotter: mockDriverName,
		Parameters:  class1Parameters,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: classSilver,
		},
		Snapshotter: mockDriverName,
		Parameters:  class2Parameters,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: emptySecretClass,
		},
		Snapshotter: mockDriverName,
		Parameters:  class4Parameters,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: invalidSecretClass,
		},
		Snapshotter: mockDriverName,
		Parameters:  class3Parameters,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: validSecretClass,
		},
		Snapshotter: mockDriverName,
		Parameters:  class5Parameters,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        defaultClass,
			Annotations: map[string]string{IsDefaultSnapshotClassAnnotation: "true"},
		},
		Snapshotter: mockDriverName,
	},
}

// Test single call to syncContent, expecting deleting to happen.
// 1. Fill in the controller with initial data
// 2. Call the syncContent *once*.
// 3. Compare resulting contents with expected contents.
func TestDeleteSync(t *testing.T) {
	tests := []controllerTest{
		{
			name:                "1-1 - content with empty snapshot class is deleted if it is bound to a non-exist snapshot and also has a snapshot uid specified",
			initialContents:     newContentArray("content1-1", classEmpty, "sid1-1", "vuid1-1", "volume1-1", "snapuid1-1", "snap1-1", nil, nil),
			expectedContents:    nocontents,
			initialSnapshots:    nosnapshots,
			expectedSnapshots:   nosnapshots,
			expectedEvents:      noevents,
			errors:              noerrors,
			expectedDeleteCalls: []deleteCall{{"sid1-1", nil, nil}},
			test:                testSyncContent,
		},
		{
			name:                "2-1 - content with empty snapshot class will not be deleted if it is bound to a non-exist snapshot but it does not have a snapshot uid specified",
			initialContents:     newContentArray("content2-1", classEmpty, "sid2-1", "vuid2-1", "volume2-1", "", "snap2-1", nil, nil),
			expectedContents:    newContentArray("content2-1", classEmpty, "sid2-1", "vuid2-1", "volume2-1", "", "snap2-1", nil, nil),
			initialSnapshots:    nosnapshots,
			expectedSnapshots:   nosnapshots,
			expectedEvents:      noevents,
			errors:              noerrors,
			expectedDeleteCalls: []deleteCall{{"sid2-1", nil, nil}},
			test:                testSyncContent,
		},
		{
			name:                "1-2 - successful delete with snapshot class that has empty secret parameter",
			initialContents:     newContentArray("content1-2", emptySecretClass, "sid1-2", "vuid1-2", "volume1-2", "snapuid1-2", "snap1-2", nil, nil),
			expectedContents:    nocontents,
			initialSnapshots:    nosnapshots,
			expectedSnapshots:   nosnapshots,
			initialSecrets:      []*v1.Secret{emptySecret()},
			expectedEvents:      noevents,
			errors:              noerrors,
			expectedDeleteCalls: []deleteCall{{"sid1-2", map[string]string{}, nil}},
			test:                testSyncContent,
		},
		{
			name:                "1-3 - successful delete with snapshot class that has valid secret parameter",
			initialContents:     newContentArray("content1-3", validSecretClass, "sid1-3", "vuid1-3", "volume1-3", "snapuid1-3", "snap1-3", nil, nil),
			expectedContents:    nocontents,
			initialSnapshots:    nosnapshots,
			expectedSnapshots:   nosnapshots,
			expectedEvents:      noevents,
			errors:              noerrors,
			initialSecrets:      []*v1.Secret{secret()},
			expectedDeleteCalls: []deleteCall{{"sid1-3", map[string]string{"foo": "bar"}, nil}},
			test:                testSyncContent,
		},
		{
			name:              "1-4 - fail delete with snapshot class that has invalid secret parameter",
			initialContents:   newContentArray("content1-4", invalidSecretClass, "sid1-4", "vuid1-4", "volume1-4", "snapuid1-4", "snap1-4", nil, nil),
			expectedContents:  newContentArray("content1-4", invalidSecretClass, "sid1-4", "vuid1-4", "volume1-4", "snapuid1-4", "snap1-4", nil, nil),
			initialSnapshots:  nosnapshots,
			expectedSnapshots: nosnapshots,
			expectedEvents:    noevents,
			errors:            noerrors,
			test:              testSyncContent,
		},
		{
			name:                "1-5 - csi driver delete snapshot returns error",
			initialContents:     newContentArray("content1-5", validSecretClass, "sid1-5", "vuid1-5", "volume1-5", "snapuid1-5", "snap1-5", nil, nil),
			expectedContents:    newContentArray("content1-5", validSecretClass, "sid1-5", "vuid1-5", "volume1-5", "snapuid1-5", "snap1-5", nil, nil),
			initialSnapshots:    nosnapshots,
			expectedSnapshots:   nosnapshots,
			initialSecrets:      []*v1.Secret{secret()},
			expectedDeleteCalls: []deleteCall{{"sid1-5", map[string]string{"foo": "bar"}, errors.New("mock csi driver delete error")}},
			expectedEvents:      []string{"Warning SnapshotDeleteError"},
			errors:              noerrors,
			test:                testSyncContent,
		},
		{
			name:                "1-6 - api server delete content returns error",
			initialContents:     newContentArray("content1-6", validSecretClass, "sid1-6", "vuid1-6", "volume1-6", "snapuid1-6", "snap1-6", nil, nil),
			expectedContents:    newContentArray("content1-6", validSecretClass, "sid1-6", "vuid1-6", "volume1-6", "snapuid1-6", "snap1-6", nil, nil),
			initialSnapshots:    nosnapshots,
			expectedSnapshots:   nosnapshots,
			initialSecrets:      []*v1.Secret{secret()},
			expectedDeleteCalls: []deleteCall{{"sid1-6", map[string]string{"foo": "bar"}, nil}},
			expectedEvents:      []string{"Warning SnapshotContentObjectDeleteError"},
			errors: []reactorError{
				// Inject error to the first client.VolumesnapshotV1alpha1().VolumeSnapshotContents().Delete call.
				// All other calls will succeed.
				{"delete", "volumesnapshotcontents", errors.New("mock delete error")},
			},
			test: testSyncContent,
		},
		{
			// delete success - snapshot that the content was pointing to was deleted, and another
			// with the same name created.
			name:                "1-7 - prebound content is deleted while the snapshot exists",
			initialContents:     newContentArray("content1-7", validSecretClass, "sid1-7", "vuid1-7", "volume1-7", "snapuid1-7", "snap1-7", nil, nil),
			expectedContents:    nocontents,
			initialSnapshots:    newSnapshotArray("snap1-7", validSecretClass, "content1-7", "snapuid1-7-x", "claim1-7", false, nil, nil, nil),
			expectedSnapshots:   newSnapshotArray("snap1-7", validSecretClass, "content1-7", "snapuid1-7-x", "claim1-7", false, nil, nil, nil),
			initialSecrets:      []*v1.Secret{secret()},
			expectedDeleteCalls: []deleteCall{{"sid1-7", map[string]string{"foo": "bar"}, nil}},
			expectedEvents:      noevents,
			errors:              noerrors,
			test:                testSyncContent,
		},
		{
			// delete success(?) - content is deleted before doDelete() starts
			name:                "1-8 - content is deleted before deleting",
			initialContents:     newContentArray("content1-8", validSecretClass, "sid1-8", "vuid1-8", "volume1-8", "snapuid1-8", "snap1-8", nil, nil),
			expectedContents:    nocontents,
			initialSnapshots:    nosnapshots,
			expectedSnapshots:   nosnapshots,
			initialSecrets:      []*v1.Secret{secret()},
			expectedDeleteCalls: []deleteCall{{"sid1-8", map[string]string{"foo": "bar"}, nil}},
			expectedEvents:      noevents,
			errors:              noerrors,
			test: wrapTestWithInjectedOperation(testSyncContent, func(ctrl *csiSnapshotController, reactor *snapshotReactor) {
				// Delete the volume before delete operation starts
				reactor.lock.Lock()
				delete(reactor.contents, "content1-8")
				reactor.lock.Unlock()
			}),
		},
		{
			name:              "1-9 - content will not be deleted if it is bound to a snapshot correctly, snapshot uid is specified",
			initialContents:   newContentArray("content1-9", validSecretClass, "sid1-9", "vuid1-9", "volume1-9", "snapuid1-9", "snap1-9", nil, nil),
			expectedContents:  newContentArray("content1-9", validSecretClass, "sid1-9", "vuid1-9", "volume1-9", "snapuid1-9", "snap1-9", nil, nil),
			initialSnapshots:  newSnapshotArray("snap1-9", validSecretClass, "content1-9", "snapuid1-9", "claim1-9", false, nil, nil, nil),
			expectedSnapshots: newSnapshotArray("snap1-9", validSecretClass, "content1-9", "snapuid1-9", "claim1-9", false, nil, nil, nil),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncContent,
		},
		{
			name:                "1-10 - should delete content which is bound to a snapshot incorrectly",
			initialContents:     newContentArray("content1-10", validSecretClass, "sid1-10", "vuid1-10", "volume1-10", "snapuid1-10-x", "snap1-10", nil, nil),
			expectedContents:    nocontents,
			initialSnapshots:    newSnapshotArray("snap1-10", validSecretClass, "content1-10", "snapuid1-10", "claim1-10", false, nil, nil, nil),
			expectedSnapshots:   newSnapshotArray("snap1-10", validSecretClass, "content1-10", "snapuid1-10", "claim1-10", false, nil, nil, nil),
			expectedEvents:      noevents,
			initialSecrets:      []*v1.Secret{secret()},
			errors:              noerrors,
			expectedDeleteCalls: []deleteCall{{"sid1-10", map[string]string{"foo": "bar"}, nil}},
			test:                testSyncContent,
		},
		{
			name:              "1-11 - content will not be deleted if it is bound to a snapshot correctly, snapsht uid is not specified",
			initialContents:   newContentArray("content1-11", validSecretClass, "sid1-11", "vuid1-11", "volume1-11", "", "snap1-11", nil, nil),
			expectedContents:  newContentArray("content1-11", validSecretClass, "sid1-11", "vuid1-11", "volume1-11", "", "snap1-11", nil, nil),
			initialSnapshots:  newSnapshotArray("snap1-11", validSecretClass, "content1-11", "snapuid1-11", "claim1-11", false, nil, nil, nil),
			expectedSnapshots: newSnapshotArray("snap1-11", validSecretClass, "content1-11", "snapuid1-11", "claim1-11", false, nil, nil, nil),
			expectedEvents:    noevents,
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncContent,
		},
	}
	runSyncTests(t, tests, snapshotClasses)
}
