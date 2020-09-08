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
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v3/apis/volumesnapshot/v1beta1"
	"github.com/kubernetes-csi/external-snapshotter/v3/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var metaTimeNow = &metav1.Time{
	Time: time.Now(),
}

var emptyString = ""

// Test single call to syncSnapshot and syncContent methods.
// 1. Fill in the controller with initial data
// 2. Call the tested function (syncSnapshot/syncContent) via
//    controllerTest.testCall *once*.
// 3. Compare resulting contents and snapshots with expected contents and snapshots.
func TestSync(t *testing.T) {
	size := int64(1)
	snapshotErr := newVolumeError("Mock content error")
	tests := []controllerTest{
		{
			// snapshot is bound to a non-existing content
			name:              "2-1 - (dynamic) snapshot is bound to a non-existing content",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap2-1", "snapuid2-1", "claim2-1", "", validSecretClass, "content2-1", &True, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-1", "snapuid2-1", "claim2-1", "", validSecretClass, "content2-1", &False, nil, nil, newVolumeError("VolumeSnapshotContent is missing"), false, true, nil),
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "2-2 - (static) snapshot points to a content but content does not point to snapshot(VolumeSnapshotRef does not match)",
			initialContents:   newContentArray("content2-2", "snapuid2-2-x", "snap2-2", "sid2-2", validSecretClass, "sid2-2", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content2-2", "snapuid2-2-x", "snap2-2", "sid2-2", validSecretClass, "sid2-2", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-2", "snapuid2-2", "", "content2-2", validSecretClass, "content2-2", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-2", "snapuid2-2", "", "content2-2", validSecretClass, "content2-2", &False, nil, nil, newVolumeError("VolumeSnapshotContent [content2-2] is bound to a different snapshot"), false, true, nil),
			expectedEvents:    []string{"Warning SnapshotContentMisbound"},
			errors:            noerrors,
			test:              testSyncSnapshotError,
		},
		{
			name:              "2-3 - (dynamic) success bind snapshot and content but not ready, no status changed",
			initialContents:   newContentArray("snapcontent-snapuid2-3", "snapuid2-3", "snap2-3", "sid2-3", validSecretClass, "", "pv-handle2-3", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArrayWithReadyToUse("snapcontent-snapuid2-3", "snapuid2-3", "snap2-3", "sid2-3", validSecretClass, "", "pv-handle2-3", deletionPolicy, &timeNowStamp, nil, &True, false),
			initialSnapshots:  newSnapshotArray("snap2-3", "snapuid2-3", "claim2-3", "", validSecretClass, "", &False, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-3", "snapuid2-3", "claim2-3", "", validSecretClass, "snapcontent-snapuid2-3", &True, metaTimeNow, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim2-3", "pvc-uid2-3", "1Gi", "volume2-3", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-3", "pv-uid2-3", "pv-handle2-3", "1Gi", "pvc-uid2-3", "claim2-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			// nothing changed
			name:              "2-4 - (static) noop",
			initialContents:   newContentArray("content2-4", "snapuid2-4", "snap2-4", "sid2-4", validSecretClass, "sid2-4", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content2-4", "snapuid2-4", "snap2-4", "sid2-4", validSecretClass, "sid2-4", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-4", "snapuid2-4", "", "content2-4", validSecretClass, "content2-4", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-4", "snapuid2-4", "", "content2-4", validSecretClass, "content2-4", &True, metaTimeNow, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "2-5 - (dynamic) snapshot and content bound, status ready false -> true",
			initialContents:   newContentArrayWithReadyToUse("snapcontent-snapuid2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "pv-handle2-5", deletionPolicy, &timeNowStamp, nil, &False, false),
			expectedContents:  newContentArrayWithReadyToUse("snapcontent-snapuid2-5", "snapuid2-5", "snap2-5", "sid2-5", validSecretClass, "", "pv-handle2-5", deletionPolicy, &timeNowStamp, nil, &False, false),
			initialSnapshots:  newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "snapcontent-snapuid2-5", &False, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-5", "snapuid2-5", "claim2-5", "", validSecretClass, "snapcontent-snapuid2-5", &False, metaTimeNow, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim2-5", "pvc-uid2-5", "1Gi", "volume2-5", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-5", "pv-uid2-5", "pv-handle2-5", "1Gi", "pvc-uid2-5", "claim2-5", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "2-6 - (static) snapshot bound to content correctly, status ready false -> true, ref.UID '' -> 'snapuid2-6'",
			initialContents:   newContentArrayWithReadyToUse("content2-6", "", "snap2-6", "sid2-6", validSecretClass, "sid2-6", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-6", "snapuid2-6", "snap2-6", "sid2-6", validSecretClass, "sid2-6", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			initialSnapshots:  newSnapshotArray("snap2-6", "snapuid2-6", "", "content2-6", validSecretClass, "content2-6", &False, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-6", "snapuid2-6", "", "content2-6", validSecretClass, "content2-6", &False, metaTimeNow, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "2-8 - snapshot and content bound, apiserver update status error",
			initialContents:   newContentArrayWithReadyToUse("content2-8", "snapuid2-8", "snap2-8", "sid2-8", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			expectedContents:  newContentArrayWithReadyToUse("content2-8", "snapuid2-8", "snap2-8", "sid2-8", validSecretClass, "", "", deletionPolicy, &timeNowStamp, nil, &False, false),
			initialSnapshots:  newSnapshotArray("snap2-8", "snapuid2-8", "claim2-8", "", validSecretClass, "content2-8", &False, metaTimeNow, nil, nil, false, false, nil),
			expectedSnapshots: newSnapshotArray("snap2-8", "snapuid2-8", "claim2-8", "", validSecretClass, "content2-8", &False, metaTimeNow, nil, nil, false, false, nil),
			expectedEvents:    []string{"Warning SnapshotFinalizerError"},
			initialClaims:     newClaimArray("claim2-8", "pvc-uid2-8", "1Gi", "volume2-8", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-8", "pv-uid2-8", "pv-handle2-8", "1Gi", "pvc-uid2-8", "claim2-8", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors: []reactorError{
				// Inject error to the first client.VolumesnapshotV1beta1().VolumeSnapshots().Update call.
				// All other calls will succeed.
				{"update", "volumesnapshots", errors.New("mock update error")},
			},
			test: testSyncSnapshotError,
		},
		{
			name:              "2-9 - fail on status update as there is not pvc provided",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap2-9", "snapuid2-9", "claim2-9", "", validSecretClass, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-9", "snapuid2-9", "claim2-9", "", validSecretClass, "", &False, nil, nil, newVolumeError("Failed to create snapshot content with error snapshot controller failed to update snap2-9 on API server: cannot get claim from snapshot"), false, true, nil),
			errors: []reactorError{
				{"get", "persistentvolumeclaims", errors.New("mock update error")},
				{"get", "persistentvolumeclaims", errors.New("mock update error")},
				{"get", "persistentvolumeclaims", errors.New("mock update error")},
			}, test: testSyncSnapshot,
		},
		{
			name:              "2-10 - (static) do not bind content does not point to the snapshot",
			initialContents:   newContentArray("content2-10", "snapuid2-10-x", "snap2-10", "sid2-10", validSecretClass, "sid2-10", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content2-10", "snapuid2-10-x", "snap2-10", "sid2-10", validSecretClass, "sid2-10", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-10", "snapuid2-10", "", "content2-10", validSecretClass, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-10", "snapuid2-10", "", "content2-10", validSecretClass, "", &False, nil, nil, newVolumeError("VolumeSnapshotContent [content2-10] is bound to a different snapshot"), false, true, nil),
			expectedEvents:    []string{"Warning SnapshotContentMisbound"},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "2-11 - (static) successful bind snapshot content with content classname updated",
			initialContents:   withContentSpecSnapshotClassName(newContentArray("content2-11", "snapuid2-11", "snap2-11", "sid2-11", validSecretClass, "sid2-11", "", deletionPolicy, nil, nil, false), nil),
			expectedContents:  newContentArray("content2-11", "snapuid2-11", "snap2-11", "sid2-11", validSecretClass, "sid2-11", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-11", "snapuid2-11", "", "content2-11", validSecretClass, "content2-11", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-11", "snapuid2-11", "", "content2-11", validSecretClass, "content2-11", &True, nil, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "2-12 - (static) fail bind snapshot content with volume snapshot classname due to API call failed",
			initialContents:   withContentSpecSnapshotClassName(newContentArray("content2-12", "snapuid2-12", "snap2-12", "sid2-12", validSecretClass, "sid2-12", "", deletionPolicy, nil, nil, false), nil),
			expectedContents:  withContentSpecSnapshotClassName(newContentArray("content2-12", "snapuid2-12", "snap2-12", "sid2-12", validSecretClass, "sid2-12", "", deletionPolicy, nil, nil, false), nil),
			initialSnapshots:  newSnapshotArray("snap2-12", "snapuid2-12", "", "content2-12", validSecretClass, "content2-12", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-12", "snapuid2-12", "", "content2-12", validSecretClass, "content2-12", &False, nil, nil, newVolumeError("Snapshot failed to bind VolumeSnapshotContent, mock update error"), false, true, nil),
			errors: []reactorError{
				// Inject error to the forth client.VolumesnapshotV1beta1().VolumeSnapshots().Update call.
				{"update", "volumesnapshotcontents", errors.New("mock update error")},
			},
			test: testSyncSnapshot,
		},
		{
			name:              "2-13 - (dynamic) snapshot expects a dynamically provisioned content but found one which is pre-proviioned, bind should fail",
			initialContents:   newContentArray("snapcontent-snapuid2-13", "snapuid2-13", "snap2-13", "sid2-13", validSecretClass, "sid2-13", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArrayWithReadyToUse("snapcontent-snapuid2-13", "snapuid2-13", "snap2-13", "sid2-13", validSecretClass, "sid2-13", "", deletionPolicy, &timeNowStamp, nil, &True, false),
			initialSnapshots:  newSnapshotArray("snap2-13", "snapuid2-13", "claim2-13", "", validSecretClass, "", &False, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-13", "snapuid2-13", "claim2-13", "", validSecretClass, "", &False, metaTimeNow, nil, newVolumeError("VolumeSnapshotContent snapcontent-snapuid2-13 is pre-provisioned while expecting a dynamically provisioned one"), false, true, nil),
			initialClaims:     newClaimArray("claim2-13", "pvc-uid2-13", "1Gi", "volume2-13", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume2-13", "pv-uid2-13", "pv-handle2-13", "1Gi", "pvc-uid2-13", "claim2-13", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			expectedEvents:    []string{"Warning SnapshotContentMismatch"},
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			// nothing changed
			name:              "2-14 - (dynamic) noop",
			initialContents:   newContentArray("snapcontent-snapuid2-14", "snapuid2-14", "snap2-14", "sid2-14", validSecretClass, "", "pv-handle-2-14", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("snapcontent-snapuid2-14", "snapuid2-14", "snap2-14", "sid2-14", validSecretClass, "", "pv-handle-2-14", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap2-14", "snapuid2-14", "claim2-14", "", validSecretClass, "snapcontent-snapuid2-14", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap2-14", "snapuid2-14", "claim2-14", "", validSecretClass, "snapcontent-snapuid2-14", &True, metaTimeNow, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-1 - (dynamic) ready snapshot lost reference to VolumeSnapshotContent",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-1", "snapuid3-1", "claim3-1", "", validSecretClass, "snapcontent-snapuid3-1", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-1", "snapuid3-1", "claim3-1", "", validSecretClass, "snapcontent-snapuid3-1", &False, metaTimeNow, nil, newVolumeError("VolumeSnapshotContent is missing"), false, true, nil),
			errors:            noerrors,
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			test:              testSyncSnapshot,
		},
		{
			name:              "3-2 - (static) ready snapshot bound to none-exist content",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap3-2", "snapuid3-2", "", "content3-2", validSecretClass, "content3-2", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-2", "snapuid3-2", "", "content3-2", validSecretClass, "content3-2", &False, metaTimeNow, nil, newVolumeError("VolumeSnapshotContent is missing"), false, true, nil),
			errors:            noerrors,
			expectedEvents:    []string{"Warning SnapshotContentMissing"},
			test:              testSyncSnapshot,
		},
		{
			name:              "3-3 - (static) ready snapshot(everything is well, do nothing)",
			initialContents:   newContentArray("content3-3", "snapuid3-3", "snap3-3", "sid3-3", validSecretClass, "sid3-3", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content3-3", "snapuid3-3", "snap3-3", "sid3-3", validSecretClass, "sid3-3", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap3-3", "snapuid3-3", "", "content3-3", validSecretClass, "content3-3", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-3", "snapuid3-3", "", "content3-3", validSecretClass, "content3-3", &True, metaTimeNow, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-4 - (static) ready snapshot misbound to VolumeSnapshotContent",
			initialContents:   newContentArray("content3-4", "snapuid3-4-x", "snap3-4", "sid3-4", validSecretClass, "sid3-4", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content3-4", "snapuid3-4-x", "snap3-4", "sid3-4", validSecretClass, "sid3-4", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap3-4", "snapuid3-4", "", "content3-4", validSecretClass, "content3-4", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-4", "snapuid3-4", "", "content3-4", validSecretClass, "content3-4", &False, metaTimeNow, nil, newVolumeError("VolumeSnapshotContent [content3-4] is bound to a different snapshot"), false, true, nil),
			expectedEvents:    []string{"Warning SnapshotContentMisbound"},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-5 - (dynamic) ready snapshot(everything is well, do nothing)",
			initialContents:   newContentArray("snapcontent-snapuid3-5", "snapuid3-5", "snap3-5", "sid3-5", validSecretClass, "", "volume-handle-3-5", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("snapcontent-snapuid3-5", "snapuid3-5", "snap3-5", "sid3-5", validSecretClass, "", "volume-handle-3-5", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap3-5", "snapuid3-5", "claim3-5", "", validSecretClass, "snapcontent-snapuid3-5", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-5", "snapuid3-5", "claim3-5", "", validSecretClass, "snapcontent-snapuid3-5", &True, metaTimeNow, nil, nil, false, true, nil),
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "3-6 - (dynamic) ready snapshot misbound to VolumeSnapshotContent",
			initialContents:   newContentArray("snapcontent-snapuid3-6", "snapuid3-6-x", "snap3-6", "sid3-6", validSecretClass, "", "volume-handle-3-6", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("snapcontent-snapuid3-6", "snapuid3-6-x", "snap3-6", "sid3-6", validSecretClass, "", "volume-handle-3-6", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap3-6", "snapuid3-6", "claim3-6", "", validSecretClass, "snapcontent-snapuid3-6", &True, metaTimeNow, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap3-6", "snapuid3-6", "claim3-6", "", validSecretClass, "snapcontent-snapuid3-6", &False, metaTimeNow, nil, newVolumeError("VolumeSnapshotContent [snapcontent-snapuid3-6] is bound to a different snapshot"), false, true, nil),
			expectedEvents:    []string{"Warning SnapshotContentMisbound"},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "4-1 - (dynamic) content bound to snapshot, snapshot status missing and rebuilt",
			initialContents:   newContentArrayWithReadyToUse("snapcontent-snapuid4-1", "snapuid4-1", "snap4-1", "sid4-1", validSecretClass, "", "pv-handle4-1", deletionPolicy, nil, &size, &True, false),
			expectedContents:  newContentArrayWithReadyToUse("snapcontent-snapuid4-1", "snapuid4-1", "snap4-1", "sid4-1", validSecretClass, "", "pv-handle4-1", deletionPolicy, nil, &size, &True, false),
			initialSnapshots:  newSnapshotArray("snap4-1", "snapuid4-1", "claim4-1", "", validSecretClass, "", &False, nil, nil, nil, true, true, nil),
			expectedSnapshots: newSnapshotArray("snap4-1", "snapuid4-1", "claim4-1", "", validSecretClass, "snapcontent-snapuid4-1", &True, nil, getSize(1), nil, false, true, nil),
			initialClaims:     newClaimArray("claim4-1", "pvc-uid4-1", "1Gi", "volume4-1", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume4-1", "pv-uid4-1", "pv-handle4-1", "1Gi", "pvc-uid4-1", "claim4-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "4-2 - (dynamic) snapshot and content bound, ReadyToUse in snapshot status missing and rebuilt",
			initialContents:   newContentArrayWithReadyToUse("snapcontent-snapuid4-2", "snapuid4-2", "snap4-2", "sid4-2", validSecretClass, "", "pv-handle4-2", deletionPolicy, nil, nil, &True, false),
			expectedContents:  newContentArrayWithReadyToUse("snapcontent-snapuid4-2", "snapuid4-2", "snap4-2", "sid4-2", validSecretClass, "", "pv-handle4-2", deletionPolicy, nil, nil, &True, false),
			initialSnapshots:  newSnapshotArray("snap4-2", "snapuid4-2", "claim4-2", "", validSecretClass, "snapcontent-snapuid4-2", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap4-2", "snapuid4-2", "claim4-2", "", validSecretClass, "snapcontent-snapuid4-2", &True, nil, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim4-2", "pvc-uid4-2", "1Gi", "volume4-2", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume4-2", "pv-uid4-2", "pv-handle4-2", "1Gi", "pvc-uid4-2", "claim4-2", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "4-3 - (dynamic) content bound to snapshot, fields in snapshot status missing and rebuilt",
			initialContents:   newContentArrayWithReadyToUse("snapcontent-snapuid4-3", "snapuid4-3", "snap4-3", "sid4-3", validSecretClass, "", "pv-handle4-3", deletionPolicy, nil, &size, &True, false),
			expectedContents:  newContentArrayWithReadyToUse("snapcontent-snapuid4-3", "snapuid4-3", "snap4-3", "sid4-3", validSecretClass, "", "pv-handle4-3", deletionPolicy, nil, &size, &True, false),
			initialSnapshots:  newSnapshotArray("snap4-3", "snapuid4-3", "claim4-3", "", validSecretClass, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap4-3", "snapuid4-3", "claim4-3", "", validSecretClass, "snapcontent-snapuid4-3", &True, nil, getSize(1), nil, false, true, nil),
			initialClaims:     newClaimArray("claim4-3", "pvc-uid4-3", "1Gi", "volume4-3", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume4-3", "pv-uid4-3", "pv-handle4-3", "1Gi", "pvc-uid4-3", "claim4-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:              "4-4 - (dynamic) content bound to snapshot, fields in snapshot status missing and rebuilt",
			initialContents:   newContentArrayWithReadyToUse("content4-4", "snapuid4-4", "snap4-4", "sid4-4", validSecretClass, "sid4-4", "", deletionPolicy, nil, &size, &True, false),
			expectedContents:  newContentArrayWithReadyToUse("content4-4", "snapuid4-4", "snap4-4", "sid4-4", validSecretClass, "sid4-4", "", deletionPolicy, nil, &size, &True, false),
			initialSnapshots:  newSnapshotArray("snap4-4", "snapuid4-4", "", "content4-4", validSecretClass, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap4-4", "snapuid4-4", "", "content4-4", validSecretClass, "content4-4", &True, nil, getSize(1), nil, false, true, nil),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			test:              testSyncSnapshot,
		},
		{
			name:             "5-1 - content missing finalizer is updated to have finalizer",
			initialContents:  newContentArray("content5-1", "snapuid5-1", "snap5-1", "sid5-1", validSecretClass, "", "pv-handle5-1", deletionPolicy, nil, nil, false),
			expectedContents: newContentArray("content5-1", "snapuid5-1", "snap5-1", "sid5-1", validSecretClass, "", "pv-handle5-1", deletionPolicy, nil, nil, true),
			initialClaims:    newClaimArray("claim5-1", "pvc-uid5-1", "1Gi", "volume5-1", v1.ClaimBound, &classEmpty),
			initialVolumes:   newVolumeArray("volume5-1", "pv-uid5-1", "pv-handle5-1", "1Gi", "pvc-uid5-1", "claim5-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:   []*v1.Secret{secret()},
			errors:           noerrors,
			test:             testSyncContent,
		},
		{
			name:             "5-2 - content missing finalizer update attempt fails because of failed API call",
			initialContents:  newContentArray("content5-2", "snapuid5-2", "snap5-2", "sid5-2", validSecretClass, "", "pv-handle5-2", deletionPolicy, nil, nil, false),
			expectedContents: newContentArray("content5-2", "snapuid5-2", "snap5-2", "sid5-2", validSecretClass, "", "pv-handle5-2", deletionPolicy, nil, nil, false),
			initialClaims:    newClaimArray("claim5-2", "pvc-uid5-2", "1Gi", "volume5-2", v1.ClaimBound, &classEmpty),
			initialVolumes:   newVolumeArray("volume5-2", "pv-uid5-2", "pv-handle5-2", "1Gi", "pvc-uid5-2", "claim5-2", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:   []*v1.Secret{secret()},
			errors: []reactorError{
				// Inject error to the forth client.VolumesnapshotV1beta1().VolumeSnapshots().Update call.
				{"update", "volumesnapshotcontents", errors.New("mock update error")},
			},
			expectSuccess: false,
			test:          testSyncContentError,
		},
		{
			name:              "5-3 - (dynamic) snapshot deletion candidate marked for deletion",
			initialSnapshots:  newSnapshotArray("snap5-3", "snapuid5-3", "claim5-3", "", validSecretClass, "snapcontent-snapuid5-3", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap5-3", "snapuid5-3", "claim5-3", "", validSecretClass, "snapcontent-snapuid5-3", &False, nil, nil, nil, false, true, &timeNowMetav1),
			initialContents:   newContentArray("snapcontent-snapuid5-3", "snapuid5-3", "snap5-3", "sid5-3", validSecretClass, "", "pv-handle5-3", deletionPolicy, nil, nil, true),
			expectedContents:  withContentAnnotations(newContentArray("snapcontent-snapuid5-3", "snapuid5-3", "snap5-3", "sid5-3", validSecretClass, "", "pv-handle5-3", deletionPolicy, nil, nil, true), map[string]string{utils.AnnVolumeSnapshotBeingDeleted: "yes"}),
			initialClaims:     newClaimArray("claim5-3", "pvc-uid5-3", "1Gi", "volume5-3", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume5-3", "pv-uid5-3", "pv-handle5-3", "1Gi", "pvc-uid5-3", "claim5-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			expectSuccess:     true,
			test:              testSyncContent,
		},
		{
			name:              "5-4 - (dynamic) snapshot deletion candidate fail to mark for deletion due to failed API call",
			initialSnapshots:  newSnapshotArray("snap5-4", "snapuid5-4", "claim5-4", "", validSecretClass, "snapcontent-snapuid5-4", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap5-4", "snapuid5-4", "claim5-4", "", validSecretClass, "snapcontent-snapuid5-4", &False, nil, nil, nil, false, true, &timeNowMetav1),
			initialContents:   newContentArray("snapcontent-snapuid5-4", "snapuid5-4", "snap5-4", "sid5-4", validSecretClass, "", "pv-handle5-4", deletionPolicy, nil, nil, true),
			// result of the test framework - annotation is still set in memory, but update call fails.
			expectedContents: withContentAnnotations(newContentArray("snapcontent-snapuid5-4", "snapuid5-4", "snap5-4", "sid5-4", validSecretClass, "", "pv-handle5-4", deletionPolicy, nil, nil, true), map[string]string{utils.AnnVolumeSnapshotBeingDeleted: "yes"}),
			initialClaims:    newClaimArray("claim5-4", "pvc-uid5-4", "1Gi", "volume5-4", v1.ClaimBound, &classEmpty),
			initialVolumes:   newVolumeArray("volume5-4", "pv-uid5-4", "pv-handle5-4", "1Gi", "pvc-uid5-4", "claim5-4", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:   []*v1.Secret{secret()},
			errors: []reactorError{
				// Inject error to the forth client.VolumesnapshotV1beta1().VolumeSnapshots().Update call.
				{"update", "volumesnapshotcontents", errors.New("mock update error")},
			},
			expectSuccess: false,
			test:          testSyncContentError,
		},
		{
			name:              "5-5 - (dynamic) snapshot deletion candidate marked for deletion by syncSnapshot",
			initialSnapshots:  newSnapshotArray("snap5-5", "snapuid5-5", "claim5-5", "", validSecretClass, "snapcontent-snapuid5-5", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap5-5", "snapuid5-5", "claim5-5", "", validSecretClass, "snapcontent-snapuid5-5", &False, nil, nil, nil, false, false, &timeNowMetav1),
			initialContents:   newContentArray("snapcontent-snapuid5-5", "snapuid5-5", "snap5-5", "sid5-5", validSecretClass, "", "pv-handle5-5", crdv1.VolumeSnapshotContentRetain, nil, nil, true),
			expectedContents:  withContentAnnotations(newContentArray("snapcontent-snapuid5-5", "snapuid5-5", "snap5-5", "sid5-5", validSecretClass, "", "pv-handle5-5", crdv1.VolumeSnapshotContentRetain, nil, nil, true), map[string]string{utils.AnnVolumeSnapshotBeingDeleted: "yes"}),
			initialClaims:     newClaimArray("claim5-5", "pvc-uid5-5", "1Gi", "volume5-5", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume5-5", "pv-uid5-5", "pv-handle5-5", "1Gi", "pvc-uid5-5", "claim5-5", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			expectSuccess:     true,
			test:              testSyncSnapshot,
		},
		{
			name:              "5-6 - (static) snapshot deletion candidate marked for deletion",
			initialSnapshots:  newSnapshotArray("snap5-6", "snapuid5-6", "", "content5-6", validSecretClass, "content5-6", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap5-6", "snapuid5-6", "", "content5-6", validSecretClass, "content5-6", &False, nil, nil, nil, false, true, &timeNowMetav1),
			initialContents:   newContentArray("content5-6", "snapuid5-6", "snap5-6", "sid5-6", validSecretClass, "sid5-6", "", deletionPolicy, nil, nil, true),
			expectedContents:  withContentAnnotations(newContentArray("content5-6", "snapuid5-6", "snap5-6", "sid5-6", validSecretClass, "sid5-6", "", deletionPolicy, nil, nil, true), map[string]string{utils.AnnVolumeSnapshotBeingDeleted: "yes"}),
			initialSecrets:    []*v1.Secret{secret()},
			expectSuccess:     true,
			test:              testSyncContent,
		},
		{
			name:              "5-7 - (static) snapshot deletion candidate fail to mark for deletion due to failed API call",
			initialSnapshots:  newSnapshotArray("snap5-7", "snapuid5-7", "", "content5-7", validSecretClass, "content5-7", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap5-7", "snapuid5-7", "", "content5-7", validSecretClass, "content5-7", &False, nil, nil, nil, false, true, &timeNowMetav1),
			initialContents:   newContentArray("content5-7", "snapuid5-7", "snap5-7", "sid5-7", validSecretClass, "sid5-7", "", deletionPolicy, nil, nil, true),
			// result of the test framework - annotation is still set in memory, but update call fails.
			expectedContents: withContentAnnotations(newContentArray("content5-7", "snapuid5-7", "snap5-7", "sid5-7", validSecretClass, "sid5-7", "", deletionPolicy, nil, nil, true), map[string]string{utils.AnnVolumeSnapshotBeingDeleted: "yes"}),
			initialSecrets:   []*v1.Secret{secret()},
			errors: []reactorError{
				// Inject error to the forth client.VolumesnapshotV1beta1().VolumeSnapshots().Update call.
				{"update", "volumesnapshotcontents", errors.New("mock update error")},
			},
			expectSuccess: false,
			test:          testSyncContentError,
		},
		{
			name:              "5-8 - (dynamic) snapshot deletion candidate marked for deletion by syncSnapshot",
			initialSnapshots:  newSnapshotArray("snap5-8", "snapuid5-8", "", "content5-8", validSecretClass, "content5-8", &False, nil, nil, nil, false, true, &timeNowMetav1),
			expectedSnapshots: newSnapshotArray("snap5-8", "snapuid5-8", "", "content5-8", validSecretClass, "content5-8", &False, nil, nil, nil, false, false, &timeNowMetav1),
			initialContents:   newContentArray("content5-8", "snapuid5-8", "snap5-8", "sid5-8", validSecretClass, "sid5-8", "", crdv1.VolumeSnapshotContentRetain, nil, nil, true),
			expectedContents:  withContentAnnotations(newContentArray("content5-8", "snapuid5-8", "snap5-8", "sid5-8", validSecretClass, "sid5-8", "", crdv1.VolumeSnapshotContentRetain, nil, nil, true), map[string]string{utils.AnnVolumeSnapshotBeingDeleted: "yes"}),
			initialSecrets:    []*v1.Secret{secret()},
			expectSuccess:     true,
			test:              testSyncSnapshot,
		},
		{
			name:              "7-1 - fail to create snapshot with non-existing snapshot class",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap7-1", "snapuid7-1", "claim7-1", "", classNonExisting, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap7-1", "snapuid7-1", "claim7-1", "", classNonExisting, "", &False, nil, nil, newVolumeError("Failed to create snapshot content with error failed to get input parameters to create snapshot snap7-1: \"volumesnapshotclass.snapshot.storage.k8s.io \\\"non-existing\\\" not found\""), false, true, nil),
			initialClaims:     newClaimArray("claim7-1", "pvc-uid7-1", "1Gi", "volume7-1", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume7-1", "pv-uid7-1", "pv-handle7-1", "1Gi", "pvc-uid7-1", "claim7-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			expectedEvents:    []string{"Warning SnapshotContentCreationFailed"},
			errors:            noerrors,
			expectSuccess:     false,
			test:              testSyncSnapshot,
		},
		// TODO(xiangqian@): remove test cases 7-2/7-3 when webhooks are in place
		//                   https://github.com/kubernetes-csi/external-snapshotter/issues/187
		{
			name:              "7-2 - validation fail if neither VolumeSnapshotContentName nor PersistentVolumeClaimName has been specified in Snapshot.Spec.Source",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap7-2", "snapuid7-2", "", "", validSecretClass, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: withSnapshotInvalidLabel(newSnapshotArray("snap7-2", "snapuid7-2", "", "", validSecretClass, "", &False, nil, nil, newVolumeError("Exactly one of PersistentVolumeClaimName and VolumeSnapshotContentName should be specified"), false, true, nil)),
			expectedEvents:    []string{"Warning SnapshotValidationError"},
			errors:            noerrors,
			expectSuccess:     false,
			test:              testSyncSnapshot,
		},
		{
			name:              "7-3 - validation fail if both VolumeSnapshotContentName and PersistentVolumeClaimName have been specified in Snapshot.Spec.Source",
			initialContents:   nocontents,
			expectedContents:  nocontents,
			initialSnapshots:  newSnapshotArray("snap7-3", "snapuid7-3", "claim7-3", "snaphandle7-3", validSecretClass, "", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: withSnapshotInvalidLabel(newSnapshotArray("snap7-3", "snapuid7-3", "claim7-3", "snaphandle7-3", validSecretClass, "", &False, nil, nil, newVolumeError("Exactly one of PersistentVolumeClaimName and VolumeSnapshotContentName should be specified"), false, true, nil)),
			initialClaims:     newClaimArray("claim7-3", "pvc-uid7-3", "1Gi", "volume7-3", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume7-3", "pv-uid7-3", "pv-handle7-3", "1Gi", "pvc-uid7-3", "claim7-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			expectedEvents:    []string{"Warning SnapshotValidationError"},
			errors:            noerrors,
			expectSuccess:     false,
			test:              testSyncSnapshot,
		},
		{
			name:              "7-4 - validation fail if both SnapshotHandle and VolumeHandle have been specified in Content.Spec.Source",
			initialSnapshots:  nosnapshots,
			expectedSnapshots: nosnapshots,
			initialContents:   newContentArray("content7-4", "snapuid7-4", "snap7-4", "sid7-4", validSecretClass, "sid7-4", "pv-handle7-4", deletionPolicy, nil, nil, true),
			expectedContents:  withSnapshotContentInvalidLabel(newContentArray("content7-4", "snapuid7-4", "snap7-4", "sid7-4", validSecretClass, "sid7-4", "pv-handle7-4", deletionPolicy, nil, nil, true)),
			expectedEvents:    []string{"Warning ContentValidationError"},
			errors:            noerrors,
			expectSuccess:     false,
			test:              testSyncContentError,
		},
		{
			name:              "7-5 - validation fail if neither SnapshotHandle or VolumeHandle has been specified in Content.Spec.Source",
			initialSnapshots:  nosnapshots,
			expectedSnapshots: nosnapshots,
			initialContents:   newContentArray("content7-4", "snapuid7-4", "snap7-4", "sid7-4", validSecretClass, "", "", deletionPolicy, nil, nil, true),
			expectedContents:  withSnapshotContentInvalidLabel(newContentArray("content7-4", "snapuid7-4", "snap7-4", "sid7-4", validSecretClass, "", "", deletionPolicy, nil, nil, true)),
			expectedEvents:    []string{"Warning ContentValidationError"},
			errors:            noerrors,
			expectSuccess:     false,
			test:              testSyncContentError,
		},
		{
			// Update Error in snapshot status based on content status
			name:              "6-1 - update snapshot error status",
			initialContents:   newContentArrayWithError("content6-1", "snapuid6-1", "snap6-1", "sid6-1", validSecretClass, "", "", deletionPolicy, nil, nil, false, snapshotErr),
			expectedContents:  newContentArrayWithError("content6-1", "snapuid6-1", "snap6-1", "sid6-1", validSecretClass, "", "", deletionPolicy, nil, nil, false, snapshotErr),
			initialSnapshots:  newSnapshotArray("snap6-1", "snapuid6-1", "claim6-1", "", validSecretClass, "content6-1", &False, nil, nil, nil, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap6-1", "snapuid6-1", "claim6-1", "", validSecretClass, "content6-1", &False, nil, nil, snapshotErr, false, true, nil),
			initialClaims:     newClaimArray("claim6-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim6-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			expectSuccess:     true,
			test:              testUpdateSnapshotErrorStatus,
		},
		{
			// Clear out Error in snapshot status if no Error in content status
			name:              "6-2 - clear out snapshot error status",
			initialContents:   newContentArray("content6-2", "snapuid6-2", "snap6-2", "sid6-2", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			expectedContents:  newContentArray("content6-2", "snapuid6-2", "snap6-2", "sid6-2", validSecretClass, "", "", deletionPolicy, nil, nil, false),
			initialSnapshots:  newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", validSecretClass, "content6-2", &False, metaTimeNow, nil, snapshotErr, false, true, nil),
			expectedSnapshots: newSnapshotArray("snap6-2", "snapuid6-2", "claim6-2", "", validSecretClass, "content6-2", &True, metaTimeNow, nil, nil, false, true, nil),
			initialClaims:     newClaimArray("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume6-2", "pv-uid6-2", "pv-handle6-2", "1Gi", "pvc-uid6-2", "claim6-2", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			expectSuccess:     true,
			test:              testUpdateSnapshotErrorStatus,
		},
		{
			// Snapshot status is nil, but gets updated to Error status based on content status
			name:              "6-3 - nil snapshot status updated with error status from content",
			initialContents:   newContentArrayWithError("content6-3", "snapuid6-3", "snap6-3", "sid6-3", validSecretClass, "", "", deletionPolicy, nil, nil, false, snapshotErr),
			expectedContents:  newContentArrayWithError("content6-3", "snapuid6-3", "snap6-3", "sid6-3", validSecretClass, "", "", deletionPolicy, nil, nil, false, snapshotErr),
			initialSnapshots:  newSnapshotArray("snap6-3", "snapuid6-3", "claim6-3", "", validSecretClass, "", nil, nil, nil, nil, true, true, nil),
			expectedSnapshots: newSnapshotArray("snap6-3", "snapuid6-3", "claim6-3", "", validSecretClass, "content6-3", &False, nil, nil, snapshotErr, false, true, nil),
			initialClaims:     newClaimArray("claim6-3", "pvc-uid6-3", "1Gi", "volume6-3", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume6-3", "pv-uid6-3", "pv-handle6-3", "1Gi", "pvc-uid6-3", "claim6-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			expectSuccess:     true,
			test:              testUpdateSnapshotErrorStatus,
		},
		{
			// Snapshot status and content status are both nil, create snapshot status with boundContentName and readyToUse set to false
			name:              "6-4 - both snapshot status and content status are nil",
			initialContents:   newContentArrayNoStatus("content6-4", "snapuid6-4", "snap6-4", "sid6-4", validSecretClass, "", "", deletionPolicy, nil, nil, false, false),
			expectedContents:  newContentArrayNoStatus("content6-4", "snapuid6-4", "snap6-4", "sid6-4", validSecretClass, "", "", deletionPolicy, nil, nil, false, false),
			initialSnapshots:  newSnapshotArray("snap6-4", "snapuid6-4", "claim6-4", "", validSecretClass, "", nil, nil, nil, nil, true, false, nil),
			expectedSnapshots: newSnapshotArray("snap6-4", "snapuid6-4", "claim6-4", "", validSecretClass, "content6-4", &False, nil, nil, nil, false, false, nil),
			initialClaims:     newClaimArray("claim6-4", "pvc-uid6-4", "1Gi", "volume6-3", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume6-4", "pv-uid6-4", "pv-handle6-4", "1Gi", "pvc-uid6-4", "claim6-4", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			expectSuccess:     true,
			test:              testUpdateSnapshotErrorStatus,
		},
		{
			// Snapshot status nil, no initial content, new content should be created.
			name:              "8-1 - Snapshot status nil, no initial snapshot content, new content should be created",
			initialContents:   nocontents,
			expectedContents:  withContentAnnotations(newContentArrayNoStatus("snapcontent-snapuid8-1", "snapuid8-1", "snap8-1", "sid8-1", validSecretClass, "", "pv-handle8-1", deletionPolicy, nil, nil, false, false), map[string]string{utils.AnnDeletionSecretRefName: "secret", utils.AnnDeletionSecretRefNamespace: "default"}),
			initialSnapshots:  newSnapshotArray("snap8-1", "snapuid8-1", "claim8-1", "", validSecretClass, "", nil, nil, nil, nil, true, false, nil),
			expectedSnapshots: newSnapshotArray("snap8-1", "snapuid8-1", "claim8-1", "", validSecretClass, "snapcontent-snapuid8-1", &False, nil, nil, nil, false, false, nil),
			initialClaims:     newClaimArray("claim8-1", "pvc-uid8-1", "1Gi", "volume8-1", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume8-1", "pv-uid8-1", "pv-handle8-1", "1Gi", "pvc-uid8-1", "claim8-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			expectSuccess:     true,
			test:              testNewSnapshotContentCreation,
		},
		{
			// Snapshot status with nil error, no initial content, new content should be created.
			name:              "8-2 - Snapshot status with nil error, no initial snapshot content, new content should be created",
			initialContents:   nocontents,
			expectedContents:  withContentAnnotations(newContentArrayNoStatus("snapcontent-snapuid8-2", "snapuid8-2", "snap8-2", "sid8-2", validSecretClass, "", "pv-handle8-2", deletionPolicy, nil, nil, false, false), map[string]string{utils.AnnDeletionSecretRefName: "secret", utils.AnnDeletionSecretRefNamespace: "default"}),
			initialSnapshots:  newSnapshotArray("snap8-2", "snapuid8-2", "claim8-2", "", validSecretClass, "", nil, nil, nil, nil, false, false, nil),
			expectedSnapshots: newSnapshotArray("snap8-2", "snapuid8-2", "claim8-2", "", validSecretClass, "snapcontent-snapuid8-2", &False, nil, nil, nil, false, false, nil),
			initialClaims:     newClaimArray("claim8-2", "pvc-uid8-2", "1Gi", "volume8-2", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume8-2", "pv-uid8-2", "pv-handle8-2", "1Gi", "pvc-uid8-2", "claim8-2", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			expectSuccess:     true,
			test:              testNewSnapshotContentCreation,
		},
		{
			// Snapshot status with error, no initial content, new content should be created, snapshot error should be cleared.
			name:              "8-3 - Snapshot status with error, no initial content, new content should be created, snapshot error should be cleared",
			initialContents:   nocontents,
			expectedContents:  withContentAnnotations(newContentArrayNoStatus("snapcontent-snapuid8-3", "snapuid8-3", "snap8-3", "sid8-3", validSecretClass, "", "pv-handle8-3", deletionPolicy, nil, nil, false, false), map[string]string{utils.AnnDeletionSecretRefName: "secret", utils.AnnDeletionSecretRefNamespace: "default"}),
			initialSnapshots:  newSnapshotArray("snap8-3", "snapuid8-3", "claim8-3", "", validSecretClass, "", nil, nil, nil, snapshotErr, false, false, nil),
			expectedSnapshots: newSnapshotArray("snap8-3", "snapuid8-3", "claim8-3", "", validSecretClass, "snapcontent-snapuid8-3", &False, nil, nil, nil, false, false, nil),
			initialClaims:     newClaimArray("claim8-3", "pvc-uid8-3", "1Gi", "volume8-3", v1.ClaimBound, &classEmpty),
			initialVolumes:    newVolumeArray("volume8-3", "pv-uid8-3", "pv-handle8-3", "1Gi", "pvc-uid8-3", "claim8-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classEmpty),
			initialSecrets:    []*v1.Secret{secret()},
			errors:            noerrors,
			expectSuccess:     true,
			test:              testNewSnapshotContentCreation,
		},
	}

	runSyncTests(t, tests, snapshotClasses)
}
