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
	"fmt"
	"testing"
	"time"

	"errors"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v3/apis/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/v3/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var defaultSize int64 = 1000
var emptySize int64
var deletePolicy = crdv1.VolumeSnapshotContentDelete
var retainPolicy = crdv1.VolumeSnapshotContentRetain
var timeNow = time.Now()
var timeNowMetav1 = metav1.Now()
var False = false
var True = true

var class1Parameters = map[string]string{
	"param1": "value1",
}

var class2Parameters = map[string]string{
	"param2": "value2",
}

var class3Parameters = map[string]string{
	"param3":                       "value3",
	utils.AnnDeletionSecretRefName: "name",
}

var class4Parameters = map[string]string{
	utils.AnnDeletionSecretRefName:      "emptysecret",
	utils.AnnDeletionSecretRefNamespace: "default",
}

var class5Parameters = map[string]string{
	utils.AnnDeletionSecretRefName:      "secret",
	utils.AnnDeletionSecretRefNamespace: "default",
}

var class6Parameters = map[string]string{
	utils.PrefixedSnapshotterSecretNameKey:          "secret",
	utils.PrefixedSnapshotterSecretNamespaceKey:     "default",
	utils.PrefixedSnapshotterListSecretNameKey:      "secret",
	utils.PrefixedSnapshotterListSecretNamespaceKey: "default",
}

var class7Annotations = map[string]string{
	utils.AnnDeletionSecretRefName:      "secret-x",
	utils.AnnDeletionSecretRefNamespace: "default-x",
}

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
			Name:        invalidSecretClass,
			Annotations: class7Annotations,
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
		Parameters:     class6Parameters,
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
			name:             "1-1 - content non-nil DeletionTimestamp with delete policy will delete snapshot",
			initialContents:  newContentArrayWithDeletionTimestamp("content1-1", "snapuid1-1", "snap1-1", "sid1-1", classGold, "", "snap1-1-volumehandle", deletionPolicy, nil, nil, true, &timeNowMetav1),
			expectedContents: newContentArrayWithDeletionTimestamp("content1-1", "snapuid1-1", "snap1-1", "", classGold, "", "snap1-1-volumehandle", deletionPolicy, nil, nil, false, &timeNowMetav1),
			expectedEvents:   noevents,
			errors:           noerrors,
			initialSecrets:   []*v1.Secret{secret()},
			expectedCreateCalls: []createCall{
				{
					snapshotName: "snapshot-snapuid1-1",
					volumeHandle: "snap1-1-volumehandle",
					parameters:   map[string]string{"param1": "value1"},
					driverName:   mockDriverName,
					size:         defaultSize,
					snapshotId:   "snapuid1-1-deleted",
					creationTime: timeNow,
					readyToUse:   true,
				},
			},
			expectedListCalls:   []listCall{{"sid1-1", map[string]string{}, true, time.Now(), 1, nil}},
			expectedDeleteCalls: []deleteCall{{"sid1-1", nil, nil}},
			expectSuccess:       true,
			test:                testSyncContent,
		},
		{
			name:             "1-2 - content non-nil DeletionTimestamp with retain policy will not delete snapshot",
			initialContents:  newContentArrayWithDeletionTimestamp("content1-2", "snapuid1-2", "snap1-2", "sid1-2", classGold, "", "snap1-2-volumehandle", retainPolicy, nil, nil, true, &timeNowMetav1),
			expectedContents: newContentArrayWithDeletionTimestamp("content1-2", "snapuid1-2", "snap1-2", "sid1-2", classGold, "", "snap1-2-volumehandle", retainPolicy, nil, nil, false, &timeNowMetav1),
			expectedEvents:   noevents,
			errors:           noerrors,
			expectedCreateCalls: []createCall{
				{
					snapshotName: "snapshot-snapuid1-2",
					volumeHandle: "snap1-2-volumehandle",
					parameters:   map[string]string{"param1": "value1"},
					driverName:   mockDriverName,
					size:         defaultSize,
					snapshotId:   "snapuid1-2-deleted",
					creationTime: timeNow,
					readyToUse:   true,
				},
			},
			expectedListCalls:   []listCall{{"sid1-2", map[string]string{}, true, time.Now(), 1, nil}},
			expectedDeleteCalls: []deleteCall{{"sid1-2", nil, nil}},
			expectSuccess:       true,
			test:                testSyncContent,
		},
		{
			name:             "1-3 - delete snapshot error should result in an event, bound finalizer should remain",
			initialContents:  newContentArrayWithDeletionTimestamp("content1-3", "snapuid1-3", "snap1-3", "sid1-3", validSecretClass, "", "snap1-3-volumehandle", deletePolicy, nil, nil, true, &timeNowMetav1),
			expectedContents: newContentArrayWithDeletionTimestamp("content1-3", "snapuid1-3", "snap1-3", "sid1-3", validSecretClass, "", "snap1-3-volumehandle", deletePolicy, nil, nil, true, &timeNowMetav1),
			errors:           noerrors,
			expectedCreateCalls: []createCall{
				{
					snapshotName: "snapshot-snapuid1-3",
					volumeHandle: "snap1-3-volumehandle",
					parameters:   map[string]string{"foo": "bar"},
					driverName:   mockDriverName,
					size:         defaultSize,
					snapshotId:   "snapuid1-3-deleted",
					creationTime: timeNow,
					readyToUse:   true,
				},
			},
			expectedDeleteCalls: []deleteCall{{"sid1-3", nil, fmt.Errorf("mock csi driver delete error")}},
			expectedEvents:      []string{"Warning SnapshotDeleteError"},
			expectedListCalls:   []listCall{{"sid1-3", map[string]string{}, true, time.Now(), 1, nil}},
			test:                testSyncContent,
		},
		{
			name:                "1-4 - fail to delete with a snapshot class which has invalid secret parameter, bound finalizer should remain",
			initialContents:     newContentArrayWithDeletionTimestamp("content1-1", "snapuid1-1", "snap1-1", "sid1-1", "invalid", "", "snap1-4-volumehandle", deletionPolicy, nil, nil, true, &timeNowMetav1),
			expectedContents:    newContentArrayWithDeletionTimestamp("content1-1", "snapuid1-1", "snap1-1", "sid1-1", "invalid", "", "snap1-4-volumehandle", deletionPolicy, nil, nil, true, &timeNowMetav1),
			expectedEvents:      noevents,
			expectedDeleteCalls: []deleteCall{{"sid1-1", nil, fmt.Errorf("mock csi driver delete error")}},
			errors: []reactorError{
				// Inject error to the first client.VolumesnapshotV1beta1().VolumeSnapshotContents().Delete call.
				// All other calls will succeed.
				{"get", "secrets", errors.New("mock get invalid secret error")},
			},
			test: testSyncContent,
		},
		{
			name:                "1-5 - csi driver delete snapshot returns error, bound finalizer should remain",
			initialContents:     newContentArrayWithDeletionTimestamp("content1-5", "sid1-5", "snap1-5", "sid1-5", validSecretClass, "", "snap1-5-volumehandle", deletionPolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedContents:    newContentArrayWithDeletionTimestamp("content1-5", "sid1-5", "snap1-5", "sid1-5", validSecretClass, "", "snap1-5-volumehandle", deletionPolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedListCalls:   []listCall{{"sid1-5", map[string]string{}, true, time.Now(), 1000, nil}},
			expectedDeleteCalls: []deleteCall{{"sid1-5", nil, errors.New("mock csi driver delete error")}},
			expectedEvents:      []string{"Warning SnapshotDeleteError"},
			errors:              noerrors,
			test:                testSyncContent,
		},
		{
			// delete success(?) - content is deleted before deleteSnapshot starts
			name:                "1-6 - content is deleted before deleting",
			initialContents:     newContentArray("content1-6", "sid1-6", "snap1-6", "sid1-6", classGold, "sid1-6", "", deletionPolicy, nil, nil, true),
			expectedContents:    nocontents,
			expectedListCalls:   []listCall{{"sid1-6", nil, false, time.Now(), 0, nil}},
			expectedDeleteCalls: []deleteCall{{"sid1-6", map[string]string{"foo": "bar"}, nil}},
			expectedEvents:      noevents,
			errors:              noerrors,
			test: wrapTestWithInjectedOperation(testSyncContent, func(ctrl *csiSnapshotSideCarController, reactor *snapshotReactor) {
				// Delete the content before delete operation starts
				reactor.lock.Lock()
				delete(reactor.contents, "content1-6")
				reactor.lock.Unlock()
			}),
		},
		{
			name:              "1-7 - content will not be deleted if it is bound to a snapshot correctly, snapshot uid is not specified",
			initialContents:   newContentArrayWithReadyToUse("content1-7", "", "snap1-7", "sid1-7", validSecretClass, "sid1-7", "", deletePolicy, nil, &defaultSize, &True, true),
			expectedContents:  newContentArrayWithReadyToUse("content1-7", "", "snap1-7", "sid1-7", validSecretClass, "sid1-7", "", deletePolicy, nil, &defaultSize, &True, true),
			expectedEvents:    noevents,
			expectedListCalls: []listCall{{"sid1-7", map[string]string{}, true, time.Now(), 1000, nil}},
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncContent,
		},
		{
			name:              "1-8 - content with retain policy will not be deleted if it is bound to a non-exist snapshot and also has a snapshot uid specified",
			initialContents:   newContentArrayWithReadyToUse("content1-8", "sid1-8", "none-existed-snapshot", "sid1-8", validSecretClass, "sid1-8", "", retainPolicy, nil, &defaultSize, &True, true),
			expectedContents:  newContentArrayWithReadyToUse("content1-8", "sid1-8", "none-existed-snapshot", "sid1-8", validSecretClass, "sid1-8", "", retainPolicy, nil, &defaultSize, &True, true),
			expectedEvents:    noevents,
			expectedListCalls: []listCall{{"sid1-8", map[string]string{}, true, time.Now(), 0, nil}},
			errors:            noerrors,
			test:              testSyncContent,
		},
		{
			name:                "1-9 - continue deletion with snapshot class that has nonexistent secret, bound finalizer removed",
			initialContents:     newContentArrayWithDeletionTimestamp("content1-9", "sid1-9", "snap1-9", "sid1-9", emptySecretClass, "", "snap1-9-volumehandle", deletePolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedContents:    newContentArrayWithDeletionTimestamp("content1-9", "sid1-9", "snap1-9", "", emptySecretClass, "", "snap1-9-volumehandle", deletePolicy, nil, &defaultSize, false, &timeNowMetav1),
			expectedEvents:      noevents,
			expectedListCalls:   []listCall{{"sid1-9", map[string]string{}, true, time.Now(), 0, nil}},
			errors:              noerrors,
			initialSecrets:      []*v1.Secret{}, // secret does not exist
			expectedDeleteCalls: []deleteCall{{"sid1-9", nil, nil}},
			test:                testSyncContent,
		},
		{
			name:              "1-10 - (dynamic)deletion of content with retain policy should not trigger CSI call, not update status, but remove bound finalizer",
			initialContents:   newContentArrayWithDeletionTimestamp("content1-10", "sid1-10", "snap1-10", "sid1-10", emptySecretClass, "", "snap1-10-volumehandle", retainPolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedContents:  newContentArrayWithDeletionTimestamp("content1-10", "sid1-10", "snap1-10", "sid1-10", emptySecretClass, "", "snap1-10-volumehandle", retainPolicy, nil, &defaultSize, false, &timeNowMetav1),
			expectedEvents:    noevents,
			expectedListCalls: []listCall{{"sid1-10", map[string]string{}, true, time.Now(), 0, nil}},
			errors:            noerrors,
			initialSecrets:    []*v1.Secret{},
			test:              testSyncContent,
		},
		{
			name:                "1-11 - (dynamic)deletion of content with deletion policy should trigger CSI call, update status, and remove bound finalizer removed.",
			initialContents:     newContentArrayWithDeletionTimestamp("content1-11", "sid1-11", "snap1-11", "sid1-11", emptySecretClass, "", "snap1-11-volumehandle", deletePolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedContents:    newContentArrayWithDeletionTimestamp("content1-11", "sid1-11", "snap1-11", "", emptySecretClass, "", "snap1-11-volumehandle", deletePolicy, nil, nil, false, &timeNowMetav1),
			expectedEvents:      noevents,
			errors:              noerrors,
			expectedDeleteCalls: []deleteCall{{"sid1-11", nil, nil}},
			test:                testSyncContent,
		},
		{
			name:              "1-12 - (pre-provision)deletion of content with retain policy should not trigger CSI call, not update status, but remove bound finalizer",
			initialContents:   newContentArrayWithDeletionTimestamp("content1-12", "sid1-12", "snap1-12", "sid1-12", emptySecretClass, "sid1-12", "", retainPolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedContents:  newContentArrayWithDeletionTimestamp("content1-12", "sid1-12", "snap1-12", "sid1-12", emptySecretClass, "sid1-12", "", retainPolicy, nil, &defaultSize, false, &timeNowMetav1),
			expectedEvents:    noevents,
			expectedListCalls: []listCall{{"sid1-12", map[string]string{}, true, time.Now(), 0, nil}},
			errors:            noerrors,
			initialSecrets:    []*v1.Secret{},
			test:              testSyncContent,
		},
		{
			name:                "1-13 - (pre-provision)deletion of content with deletion policy should trigger CSI call, update status, and remove bound finalizer removed.",
			initialContents:     newContentArrayWithDeletionTimestamp("content1-13", "sid1-13", "snap1-13", "sid1-13", emptySecretClass, "sid1-13", "", deletePolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedContents:    newContentArrayWithDeletionTimestamp("content1-13", "sid1-13", "snap1-13", "", emptySecretClass, "sid1-13", "", deletePolicy, nil, nil, false, &timeNowMetav1),
			expectedEvents:      noevents,
			errors:              noerrors,
			expectedDeleteCalls: []deleteCall{{"sid1-13", nil, nil}},
			test:                testSyncContent,
		},
		{
			name:                "1-14 - (pre-provision)deletion of content with deletion policy and no snapshotclass should trigger CSI call, update status, and remove bound finalizer removed.",
			initialContents:     newContentArrayWithDeletionTimestamp("content1-14", "sid1-14", "snap1-14", "sid1-14", "", "sid1-14", "", deletePolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedContents:    newContentArrayWithDeletionTimestamp("content1-14", "sid1-14", "snap1-14", "", "", "sid1-14", "", deletePolicy, nil, nil, false, &timeNowMetav1),
			expectedEvents:      noevents,
			errors:              noerrors,
			expectedDeleteCalls: []deleteCall{{"sid1-14", nil, nil}},
			test:                testSyncContent,
		},
		{
			name:                "1-15 - (dynamic)deletion of content with no snapshotclass should succeed",
			initialContents:     newContentArrayWithDeletionTimestamp("content1-15", "sid1-15", "snap1-15", "sid1-15", "", "", "snap1-15-volumehandle", deletePolicy, nil, &defaultSize, true, &timeNowMetav1),
			expectedContents:    newContentArrayWithDeletionTimestamp("content1-15", "sid1-15", "snap1-15", "", "", "", "snap1-15-volumehandle", deletePolicy, nil, &defaultSize, false, &timeNowMetav1),
			errors:              noerrors,
			expectedDeleteCalls: []deleteCall{{"sid1-15", nil, nil}},
			test:                testSyncContent,
		},
	}
	runSyncContentTests(t, tests, snapshotClasses)
}
