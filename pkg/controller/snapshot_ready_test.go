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
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	storagev1beta1 "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var metaTimeNow = &metav1.Time{
	Time: time.Now(),
}

var volumeErr = &storagev1beta1.VolumeError{
	Time:    *metaTimeNow,
	Message: "Failed to upload the snapshot",
}

// Test single call to syncSnapshot and syncContent methods.
// 1. Fill in the controller with initial data
// 2. Call the tested function (syncSnapshot/syncContent) via
//    controllerTest.testCall *once*.
// 3. Compare resulting contents and snapshots with expected contents and snapshots.
func TestSync(t *testing.T) {
	tests := []controllerTest{
		{
			// snapshot is bound to a non-existing content
			name:              "2-1 - snapshot is bound to a non-existing content",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap2-1", validSecretClass, "content2-1", "snapuid2-1", "claim2-1", false, nil, nil, nil),
			expectedSnapshots: newSnapshotArray("snap2-1", validSecretClass, "content2-1", "snapuid2-1", "claim2-1", false, newVolumeError("VolumeSnapshotContent is missing"), nil, nil),
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			errors:            noerrors,
			test:              testSyncSnapshotError,
		},
		{
			name:              "2-2 - could not bind snapshot and content, the VolumeSnapshotRef does not match",
			initialContents:   newContentArray("content2-2", validSecretClass, "sid2-2", "vuid2-2", "volume2-2", "snapuid2-2-x", "snap2-2", nil, nil),
			expectedContents:  newContentArray("content2-2", validSecretClass, "sid2-2", "vuid2-2", "volume2-2", "snapuid2-2-x", "snap2-2", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-2", validSecretClass, "content2-2", "snapuid2-2", "claim2-2", false, nil, nil, nil),
			expectedSnapshots: newSnapshotArray("snap2-2", validSecretClass, "content2-2", "snapuid2-2", "claim2-2", false, newVolumeError("Snapshot failed to bind VolumeSnapshotContent, Could not bind snapshot snap2-2 and content content2-2, the VolumeSnapshotRef does not match"), nil, nil),
			expectedEvents:    []string{"Warning SnapshotBindFailed"},
			errors:            noerrors,
			test:              testSyncSnapshotError,
		},
		{
			name:              "2-3 - success bind snapshot and content, no status changed",
			initialContents:   newContentArray("content2-3", validSecretClass, "sid2-3", "vuid2-3", "volume2-3", "", "snap2-3", nil, nil),
			expectedContents:  newContentArray("content2-3", validSecretClass, "sid2-3", "vuid2-3", "volume2-3", "snapuid2-3", "snap2-3", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-3", validSecretClass, "content2-3", "snapuid2-3", "claim2-3", false, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap2-3", validSecretClass, "content2-3", "snapuid2-3", "claim2-3", false, nil, metaTimeNow, nil),
			expectedListCalls: []listCall{
				{
					snapshotID: "sid2-3",
					status: &csi.SnapshotStatus{
						Type:    csi.SnapshotStatus_UPLOADING,
						Details: "uploading",
					},
				},
			},
			errors: noerrors,
			test:   testSyncSnapshot,
		},
		{
			// nothing changed
			name:              "2-4 - noop",
			initialContents:   newContentArray("content2-4", validSecretClass, "sid2-4", "vuid2-4", "volume2-4", "snapuid2-4", "snap2-4", nil, nil),
			expectedContents:  newContentArray("content2-4", validSecretClass, "sid2-4", "vuid2-4", "volume2-4", "snapuid2-4", "snap2-4", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-4", validSecretClass, "content2-4", "snapuid2-4", "claim2-4", false, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap2-4", validSecretClass, "content2-4", "snapuid2-4", "claim2-4", false, nil, metaTimeNow, nil),
			expectedListCalls: []listCall{
				{
					snapshotID: "sid2-4",
					status: &csi.SnapshotStatus{
						Type:    csi.SnapshotStatus_UPLOADING,
						Details: "uploading",
					},
				},
			},
			errors: noerrors,
			test:   testSyncSnapshot,
		},
		{
			name:              "2-5 - snapshot and content bound, status ready false -> true",
			initialContents:   newContentArray("content2-5", validSecretClass, "sid2-5", "vuid2-5", "volume2-5", "snapuid2-5", "snap2-5", nil, nil),
			expectedContents:  newContentArray("content2-5", validSecretClass, "sid2-5", "vuid2-5", "volume2-5", "snapuid2-5", "snap2-5", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-5", validSecretClass, "content2-5", "snapuid2-5", "claim2-5", false, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap2-5", validSecretClass, "content2-5", "snapuid2-5", "claim2-5", true, nil, metaTimeNow, nil),
			expectedListCalls: []listCall{
				{
					snapshotID: "sid2-5",
					status: &csi.SnapshotStatus{
						Type:    csi.SnapshotStatus_READY,
						Details: "success",
					},
				},
			},
			errors: noerrors,
			test:   testSyncSnapshot,
		},
		{
			name:              "2-6 - snapshot and content bound, status -> error uploading",
			initialContents:   newContentArray("content2-6", validSecretClass, "sid2-6", "vuid2-6", "volume2-6", "snapuid2-6", "snap2-6", nil, nil),
			expectedContents:  newContentArray("content2-6", validSecretClass, "sid2-6", "vuid2-6", "volume2-6", "snapuid2-6", "snap2-6", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-6", validSecretClass, "content2-6", "snapuid2-6", "claim2-6", false, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap2-6", validSecretClass, "content2-6", "snapuid2-6", "claim2-6", false, volumeErr, metaTimeNow, nil),
			expectedEvents:    []string{"Warning SnapshotUploadError"},
			expectedListCalls: []listCall{
				{
					snapshotID: "sid2-6",
					status: &csi.SnapshotStatus{
						Type:    csi.SnapshotStatus_ERROR_UPLOADING,
						Details: "error upload",
					},
				},
			},
			errors: noerrors,
			test:   testSyncSnapshot,
		},
		{
			name:              "2-7 - snapshot and content bound, csi driver get status error",
			initialContents:   newContentArray("content2-7", validSecretClass, "sid2-7", "vuid2-7", "volume2-7", "snapuid2-7", "snap2-7", nil, nil),
			expectedContents:  newContentArray("content2-7", validSecretClass, "sid2-7", "vuid2-7", "volume2-7", "snapuid2-7", "snap2-7", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-7", validSecretClass, "content2-7", "snapuid2-7", "claim2-7", false, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap2-7", validSecretClass, "content2-7", "snapuid2-7", "claim2-7", false, newVolumeError("Failed to check and update snapshot: failed to check snapshot status snap2-7 with error failed to list snapshot data content2-7: \"mock driver get status error\""), metaTimeNow, nil),
			expectedEvents:    []string{"Warning SnapshotCheckandUpdateFailed"},
			expectedListCalls: []listCall{
				{
					snapshotID: "sid2-7",
					err:        errors.New("mock driver get status error"),
				},
			},
			errors: noerrors,
			test:   testSyncSnapshot,
		},
		{
			name:              "2-8 - snapshot and content bound, apiserver update status error",
			initialContents:   newContentArray("content2-8", validSecretClass, "sid2-8", "vuid2-8", "volume2-8", "snapuid2-8", "snap2-8", nil, nil),
			expectedContents:  newContentArray("content2-8", validSecretClass, "sid2-8", "vuid2-8", "volume2-8", "snapuid2-8", "snap2-8", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-8", validSecretClass, "content2-8", "snapuid2-8", "claim2-8", false, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap2-8", validSecretClass, "content2-8", "snapuid2-8", "claim2-8", false, newVolumeError("Failed to check and update snapshot: snapshot controller failed to update default/snap2-8 on API server: mock update error"), metaTimeNow, nil),
			expectedEvents:    []string{"Warning SnapshotUploadError"},
			expectedListCalls: []listCall{
				{
					snapshotID: "sid2-8",
					status: &csi.SnapshotStatus{
						Type:    csi.SnapshotStatus_ERROR_UPLOADING,
						Details: "error upload",
					},
				},
			},
			errors: []reactorError{
				// Inject error to the first client.VolumesnapshotV1alpha1().VolumeSnapshots().Update call.
				// All other calls will succeed.
				{"update", "volumesnapshots", errors.New("mock update error")},
			},
			test: testSyncSnapshot,
		},
		{
			name:              "2-9 - bind when snapshot and content matches",
			initialContents:   newContentArray("content2-9", validSecretClass, "sid2-9", "vuid2-9", "volume2-9", "snapuid2-9", "snap2-9", nil, nil),
			expectedContents:  newContentArray("content2-9", validSecretClass, "sid2-9", "vuid2-9", "volume2-9", "snapuid2-9", "snap2-9", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-9", validSecretClass, "", "snapuid2-9", "claim2-9", false, nil, nil, nil),
			expectedSnapshots: newSnapshotArray("snap2-9", validSecretClass, "content2-9", "snapuid2-9", "claim2-9", false, nil, nil, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "2-10 - do not bind when snapshot and content not match",
			initialContents:   newContentArray("content2-10", validSecretClass, "sid2-10", "vuid2-10", "volume2-10", "snapuid2-10-x", "snap2-10", nil, nil),
			expectedContents:  newContentArray("content2-10", validSecretClass, "sid2-10", "vuid2-10", "volume2-10", "snapuid2-10-x", "snap2-10", nil, nil),
			initialSnapshots:  newSnapshotArray("snap2-10", validSecretClass, "", "snapuid2-10", "claim2-10", false, newVolumeError("mock driver error"), nil, nil),
			expectedSnapshots: newSnapshotArray("snap2-10", validSecretClass, "", "snapuid2-10", "claim2-10", false, newVolumeError("mock driver error"), nil, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-1 - ready snapshot lost reference to VolumeSnapshotContent",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-1", validSecretClass, "", "snapuid3-1", "claim3-1", true, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap3-1", validSecretClass, "", "snapuid3-1", "claim3-1", false, newVolumeError("Bound snapshot has lost reference to VolumeSnapshotContent"), metaTimeNow, nil),
			errors:            noerrors,
			expectedEvents:    []string{"Warning SnapshotLost"},
			test:              testSyncSnapshot,
		},
		{
			name:              "3-2 - ready snapshot bound to none-exist content",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-2", validSecretClass, "content3-2", "snapuid3-2", "claim3-2", true, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap3-2", validSecretClass, "content3-2", "snapuid3-2", "claim3-2", false, newVolumeError("VolumeSnapshotContent is missing"), metaTimeNow, nil),
			errors:            noerrors,
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			test:              testSyncSnapshot,
		},
		{
			name:              "3-3 - ready snapshot(everything is well, do nothing)",
			initialContents:   newContentArray("content3-3", validSecretClass, "sid3-3", "vuid3-3", "volume3-3", "snapuid3-3", "snap3-3", nil, nil),
			expectedContents:  newContentArray("content3-3", validSecretClass, "sid3-3", "vuid3-3", "volume3-3", "snapuid3-3", "snap3-3", nil, nil),
			initialSnapshots:  newSnapshotArray("snap3-3", validSecretClass, "content3-3", "snapuid3-3", "claim3-3", true, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap3-3", validSecretClass, "content3-3", "snapuid3-3", "claim3-3", true, nil, metaTimeNow, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-4 - ready snapshot misbound to VolumeSnapshotContent",
			initialContents:   newContentArray("content3-4", validSecretClass, "sid3-4", "vuid3-4", "volume3-4", "snapuid3-4-x", "snap3-4", nil, nil),
			expectedContents:  newContentArray("content3-4", validSecretClass, "sid3-4", "vuid3-4", "volume3-4", "snapuid3-4-x", "snap3-4", nil, nil),
			initialSnapshots:  newSnapshotArray("snap3-4", validSecretClass, "content3-4", "snapuid3-4", "claim3-4", true, nil, metaTimeNow, nil),
			expectedSnapshots: newSnapshotArray("snap3-4", validSecretClass, "content3-4", "snapuid3-4", "claim3-4", false, newVolumeError("VolumeSnapshotContent is not bound to the VolumeSnapshot correctly"), metaTimeNow, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-5 - snapshot bound to content in which the driver does not match",
			initialContents:   newContentWithUnmatchDriverArray("content3-5", validSecretClass, "sid3-5", "vuid3-5", "volume3-5", "", "snap3-5", nil, nil),
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-5", validSecretClass, "content3-5", "snapuid3-5", "claim3-5", false, nil, nil, nil),
			expectedSnapshots: newSnapshotArray("snap3-5", validSecretClass, "content3-5", "snapuid3-5", "claim3-5", false, newVolumeError("VolumeSnapshotContent is missing"), nil, nil),
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			errors:            noerrors,
			test:              testSyncSnapshotError,
		},
		{
			name:              "3-6 - snapshot bound to content in which the snapshot uid does not match",
			initialContents:   newContentArray("content3-4", validSecretClass, "sid3-4", "vuid3-4", "volume3-4", "snapuid3-4-x", "snap3-6", nil, nil),
			expectedContents:  newContentArray("content3-4", validSecretClass, "sid3-4", "vuid3-4", "volume3-4", "snapuid3-4-x", "snap3-6", nil, nil),
			initialSnapshots:  newSnapshotArray("snap3-5", validSecretClass, "content3-5", "snapuid3-5", "claim3-5", false, nil, nil, nil),
			expectedSnapshots: newSnapshotArray("snap3-5", validSecretClass, "content3-5", "snapuid3-5", "claim3-5", false, newVolumeError("VolumeSnapshotContent is missing"), nil, nil),
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			errors:            noerrors,
			test:              testSyncSnapshotError,
		},
	}

	runSyncTests(t, tests, snapshotClasses)
}
