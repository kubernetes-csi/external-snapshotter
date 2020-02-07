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
	//"errors"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
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
	size := int64(1)
	tests := []controllerTest{
		{
			// snapshot is bound to a non-existing content
			name:              "2-1 - snapshot is bound to a non-existing content",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap2-1", "snapuid2-1", "claim2-1", "", validSecretClass, "content2-1", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-1", "snapuid2-1", "claim2-1", "", validSecretClass, "content2-1", &False, nil, nil, newVolumeError("VolumeSnapshotContent is missing"), false, true, nil),
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		/*{
			name:              "2-2 - snapshot points to a content but content does not point to snapshot(VolumeSnapshotRef does not match)",
			initialContents:   newContentArray("content2-2", "snapuid2-2-x", "snap2-2", "sid2-2", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content2-2", "snapuid2-2-x", "snap2-2", "sid2-2", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-2", "snapuid2-2", "claim2-2", "", validSecretClass, "content2-2", &False, nil, nil, nil),
			expectedSnapshots: newSnapshotArray("snap2-2", "snapuid2-2", "claim2-2", "", validSecretClass, "content2-2", &False, nil, nil, newVolumeError("Snapshot is bound to a VolumeSnapshotContent which is bound to other Snapshot")),
			expectedEvents:    []string{"Warning InvalidSnapshotBinding"},
			errors:            noerrors,
			test:              testSyncSnapshotError,
		},*/
		{
			name:              "2-3 - success bind snapshot and content but not ready, no status changed",
			initialContents:   newContentArray("content2-3", "snapuid2-3", "snap2-3", "sid2-3", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-3", "snapuid2-3", "snap2-3", "sid2-3", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &True, false),
			initialSnapshots:  newSnapshotArray("snap2-3", "snapuid2-3", "claim2-3", "", validSecretClass, "content2-3", &False, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-3", "snapuid2-3", "claim2-3", "", validSecretClass, "content2-3", &True, metaTimeNow, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim2-3", "pvc-uid2-3", "1Gi", "volume2-3", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-3", "pv-uid2-3", "pv-handle2-3", "1Gi", "pvc-uid2-3", "claim2-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			/*expectedCreateCalls: []createCall{
				{
					snapshotName: "snapshot-snapuid2-3",
					volume:       newVolume("volume2-3", "pv-uid2-3", "pv-handle2-3", "1Gi", "pvc-uid2-3", "claim2-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
					parameters:   class5Parameters,
					secrets:      map[string]string{"foo": "bar"},
					// information to return
					driverName:   mockDriverName,
					size:         defaultSize,
					snapshotId:   "sid2-3",
					creationTime: timeNow,
					readyToUse:   False,
				},
			},*/
			errors: noerrors,
			test:   testSyncSnapshot,
		},
		{
			// nothing changed
			name:              "2-4 - noop",
			initialContents:   newContentArray("content2-4", "snapuid2-4", "snap2-4", "sid2-4", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content2-4", "snapuid2-4", "snap2-4", "sid2-4", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-4", "snapuid2-4", "claim2-4", "", validSecretClass, "content2-4", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-4", "snapuid2-4", "claim2-4", "", validSecretClass, "content2-4", &True, metaTimeNow, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "2-5 - snapshot and content bound, status ready false -> true",
			initialContents:   newContentArrayWithReadyToUse("content2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			initialSnapshots:  newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "content2-5", &False, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "content2-5", &False, metaTimeNow, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim2-5", "pvc-uid2-5", "1Gi", "volume2-5", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-5", "pv-uid2-5", "pv-handle2-5", "1Gi", "pvc-uid2-5", "claim2-5", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			/*expectedCreateCalls: []createCall{
				{
					snapshotName: "snapshot-snapuid2-5",
					volume:       newVolume("volume2-5", "pv-uid2-5", "pv-handle2-5", "1Gi", "pvc-uid2-5", "claim2-5", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
					parameters:   class5Parameters,
					secrets:      map[string]string{"foo": "bar"},
					// information to return
					driverName:   mockDriverName,
					size:         defaultSize,
					snapshotId:   "sid2-5",
					creationTime: timeNow,
					readyToUse:   True,
				},
			},*/
			errors: noerrors,
			test:   testSyncSnapshot,
		},
		{
			name:              "2-6 - snapshot bound to prebound content correctly, status ready false -> true, ref.UID '' -> 'snapuid2-6'",
			initialContents:   newContentArrayWithReadyToUse("content2-6", "snapuid2-6", "snap2-6", "sid2-6", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-6", "snapuid2-6", "snap2-6", "sid2-6", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			initialSnapshots:  newSnapshotArray("snap2-6", "snapuid2-6", "", "content2-6", validSecretClass, "content2-6", &False, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-6", "snapuid2-6", "", "content2-6", validSecretClass, "content2-6", &False, metaTimeNow, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		/*{
			name:              "2-7 - snapshot and content bound, csi driver get status error",
			initialContents:   newContentArrayWithReadyToUse("content2-7", "snapuid2-7", "snap2-7", "sid2-7", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-7", "snapuid2-7", "snap2-7", "sid2-7", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			initialSnapshots:  newSnapshotArray("snap2-7", "snapuid2-7", "claim2-7", "", validSecretClass, "content2-7", &False, metaTimeNow, nil, nil),
			expectedSnapshots: newSnapshotArray("snap2-7", "snapuid2-7", "claim2-7", "", validSecretClass, "content2-7", &False, metaTimeNow, nil, newVolumeError("Failed to check and update snapshot: mock create snapshot error")),
			expectedEvents:    []string{"Warning SnapshotCheckandUpdateFailed"},
			initialClaims:     newClaimArray("claim2-7", "pvc-uid2-7", "1Gi", "volume2-7", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-7", "pv-uid2-7", "pv-handle2-7", "1Gi", "pvc-uid2-7", "claim2-7", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			expectedCreateCalls: []createCall{
				{
					snapshotName: "snapshot-snapuid2-7",
					volume:       newVolume("volume2-7", "pv-uid2-7", "pv-handle2-7", "1Gi", "pvc-uid2-7", "claim2-7", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
					parameters:   class5Parameters,
					secrets:      map[string]string{"foo": "bar"},
					// information to return
					err: errors.New("mock create snapshot error"),
				},
			},
			errors: noerrors,
			test:   testSyncSnapshot,
		},*/
		/*{
		name:              "2-8 - snapshot and content bound, apiserver update status error",
		initialContents:   newContentArrayWithReadyToUse("content2-8", "snapuid2-8", "snap2-8", "sid2-8", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
		expectedContents:  newContentArrayWithReadyToUse("content2-8", "snapuid2-8", "snap2-8", "sid2-8", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
		initialSnapshots:  newSnapshotArray("snap2-8", "snapuid2-8", "claim2-8", "", validSecretClass, "content2-8", &False, metaTimeNow, nil, nil),
		expectedSnapshots: newSnapshotArray("snap2-8", "snapuid2-8", "claim2-8", "", validSecretClass, "content2-8", &False, metaTimeNow, nil, newVolumeError("Failed to check and update snapshot: snapshot controller failed to update default/snap2-8 on API server: mock update error")),
		expectedEvents:    []string{"Warning SnapshotCheckandUpdateFailed"},
		initialClaims:     newClaimArray("claim2-8", "pvc-uid2-8", "1Gi", "volume2-8", v1.ClaimBound, &classEmpty),
		initialVolumes:    newVolumeArray("volume2-8", "pv-uid2-8", "pv-handle2-8", "1Gi", "pvc-uid2-8", "claim2-8", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
		initialSecrets:    []*v1.Secret{secret()},
		/*expectedCreateCalls: []createCall{
			{
				snapshotName: "snapshot-snapuid2-8",
				volume:       newVolume("volume2-8", "pv-uid2-8", "pv-handle2-8", "1Gi", "pvc-uid2-8", "claim2-8", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
				parameters:   class5Parameters,
				secrets:      map[string]string{"foo": "bar"},
				// information to return
				driverName:   mockDriverName,
				size:         defaultSize,
				snapshotId:   "sid2-8",
				creationTime: timeNow,
				readyToUse:   true,
			},
		},*/ /*
			errors: []reactorError{
				// Inject error to the first client.VolumesnapshotV1beta1().VolumeSnapshots().Update call.
				// All other calls will succeed.
				{"update", "volumesnapshots", errors.New("mock update error")},
			},
			test: testSyncSnapshot,
		},*/
		{
			name:              "2-9 - fail on status update as there is not pvc provided",
			initialContents:   newContentArray("content2-9", "snapuid2-9", "snap2-9", "sid2-9", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content2-9", "snapuid2-9", "snap2-9", "sid2-9", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-9", "snapuid2-9", "claim2-9", "", validSecretClass, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-9", "snapuid2-9", "claim2-9", "", validSecretClass, "content2-9", &True, nil, nil, nil, false, true, nil),
			//expectedSnapshots: newSnapshotArray("snap2-9", "snapuid2-9", "claim2-9", "", validSecretClass, "content2-9", &False, nil, nil, newVolumeError("Failed to check and update snapshot: failed to get input parameters to create snapshot snap2-9: \"failed to retrieve PVC claim2-9 from the lister: \\\"persistentvolumeclaim \\\\\\\"claim2-9\\\\\\\" not found\\\"\"")),
			errors: noerrors,
			test:   testSyncSnapshot,
		},
		{
			name:              "2-10 - do not bind when snapshot and content not match",
			initialContents:   newContentArray("content2-10", "snapuid2-10-x", "snap2-10", "sid2-10", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content2-10", "snapuid2-10-x", "snap2-10", "sid2-10", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-10", "snapuid2-10", "claim2-10", "", validSecretClass, "", &False, nil, nil, newVolumeError("mock driver error"), false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-10", "snapuid2-10", "claim2-10", "", validSecretClass, "", &False, nil, nil, newVolumeError("mock driver error"), false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-1 - ready snapshot lost reference to VolumeSnapshotContent",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-1", "snapuid3-1", "claim3-1", "", validSecretClass, "content3-1", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-1", "snapuid3-1", "claim3-1", "", validSecretClass, "content3-1", &False, metaTimeNow, nil, newVolumeError("VolumeSnapshotContent is missing"), false, true, nil),
			errors:            noerrors,
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			test:              testSyncSnapshot,
		},
		{
			name:              "3-2 - ready snapshot bound to none-exist content",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-2", "snapuid3-2", "claim3-2", "", validSecretClass, "content3-2", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-2", "snapuid3-2", "claim3-2", "", validSecretClass, "content3-2", &False, metaTimeNow, nil, newVolumeError("VolumeSnapshotContent is missing"), false, true, nil),
			errors:            noerrors,
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			test:              testSyncSnapshot,
		},
		{
			name:              "3-3 - ready snapshot(everything is well, do nothing)",
			initialContents:   newContentArray("content3-3", "snapuid3-3", "snap3-3", "sid3-3", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content3-3", "snapuid3-3", "snap3-3", "sid3-3", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap3-3", "snapuid3-3", "claim3-3", "", validSecretClass, "content3-3", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-3", "snapuid3-3", "claim3-3", "", validSecretClass, "content3-3", &True, metaTimeNow, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-4 - ready snapshot misbound to VolumeSnapshotContent",
			initialContents:   newContentArray("content3-4", "snapuid3-4-x", "snap3-4", "sid3-4", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content3-4", "snapuid3-4-x", "snap3-4", "sid3-4", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap3-4", "snapuid3-4", "claim3-4", "", validSecretClass, "content3-4", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-4", "snapuid3-4", "claim3-4", "", validSecretClass, "content3-4", &False, metaTimeNow, nil, newVolumeError("VolumeSnapshotContent is not bound to the VolumeSnapshot correctly"), false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "4-1 - content bound to snapshot, snapshot status missing and rebuilt",
			initialContents:   newContentArrayWithReadyToUse("content2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "", deletionPolicy, nil, &size, &True, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "", deletionPolicy, nil, &size, &True, false),
			initialSnapshots:  newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "", &False, nil, nil, nil, true, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "content2-5", &True, nil, getSize(1), nil, false, true, nil),
			initialClaims:     newClaimArray("claim2-5", "pvc-uid2-5", "1Gi", "volume2-5", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-5", "pv-uid2-5", "pv-handle2-5", "1Gi", "pvc-uid2-5", "claim2-5", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "4-2 - snapshot and content bound, ReadyToUse in snapshot status missing and rebuilt",
			initialContents:   newContentArrayWithReadyToUse("content2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "", deletionPolicy, nil, nil, &True, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "", deletionPolicy, nil, nil, &True, false),
			initialSnapshots:  newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "content2-5", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "content2-5", &True, nil, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim2-5", "pvc-uid2-5", "1Gi", "volume2-5", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-5", "pv-uid2-5", "pv-handle2-5", "1Gi", "pvc-uid2-5", "claim2-5", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "4-3 - content bound to snapshot, fields in snapshot status missing and rebuilt",
			initialContents:   newContentArrayWithReadyToUse("content2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "", deletionPolicy, nil, &size, &True, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "", deletionPolicy, nil, &size, &True, false),
			initialSnapshots:  newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "content2-5", &True, nil, getSize(1), nil, false, true, nil),
			initialClaims:     newClaimArray("claim2-5", "pvc-uid2-5", "1Gi", "volume2-5", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-5", "pv-uid2-5", "pv-handle2-5", "1Gi", "pvc-uid2-5", "claim2-5", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
	}

	runSyncTests(t, tests, snapshotClasses)
}
