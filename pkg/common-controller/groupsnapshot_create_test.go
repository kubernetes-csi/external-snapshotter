/*
Copyright 2024 The Kubernetes Authors.

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
	"testing"

	crdv1beta1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var groupSnapshotClasses = []*crdv1beta1.VolumeGroupSnapshotClass{
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeGroupSnapshotClass",
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
			Kind: "VolumeGroupSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        defaultClass,
			Annotations: map[string]string{utils.IsDefaultGroupSnapshotClassAnnotation: "true"},
		},
		Driver:         mockDriverName,
		Parameters:     class1Parameters,
		DeletionPolicy: crdv1.VolumeSnapshotContentDelete,
	},
	{
		TypeMeta: metav1.TypeMeta{
			Kind: "VolumeGroupSnapshotClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        classSilver,
			Annotations: map[string]string{utils.IsDefaultGroupSnapshotClassAnnotation: "true"},
		},
		Driver:         mockDriverName,
		Parameters:     class1Parameters,
		DeletionPolicy: crdv1.VolumeSnapshotContentRetain,
	},
}

func TestCreateGroupSnapshotSync(t *testing.T) {
	tests := []controllerTest{
		{
			name: "1-1 - successful dynamically-provisioned create group snapshot with group snapshot class gold",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "groupsnapcontent-group-snapuid1-1", &False, nil, nil, false, false, nil,
			),
			initialGroupContents: nogroupcontents,
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "group-snapshot-handle", classGold, []string{
					"1-pv-handle6-1",
					"2-pv-handle6-1",
				}, "", deletionPolicy, nil, false, false,
			),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  true,
		},
		{
			name: "1-2 - unsuccessful create group snapshot with no existent group snapshot class",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classNonExisting, "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classNonExisting, "", &False, nil,
				newVolumeError(`failed to create group snapshot content with error failed to get input parameters to create group snapshot group-snap-1-1: "volumegroupsnapshotclass.groupsnapshot.storage.k8s.io \"non-existing\" not found"`),
				false, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  false,
		},
		{
			name: "1-3 - fail to create group snapshot without group snapshot class",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", "", "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", "", "", &False, nil,
				newVolumeError(`failed to create group snapshot content with error failed to get input parameters to create group snapshot group-snap-1-1: "failed to take group snapshot group-snap-1-1 without a group snapshot class"`),
				false, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  false,
		},
		{
			name: "1-4 - fail to create group snapshot with no existing volumes",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", &False, nil,
				newVolumeError(`failed to create group snapshot content with error failed to get input parameters to create group snapshot group-snap-1-1: "label selector app.kubernetes.io/name=postgresql for group snapshot not applied to any PVC"`),
				false, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims:         nil,
			initialVolumes:        nil,
			errors:                noerrors,
			test:                  testSyncGroupSnapshot,
			expectSuccess:         false,
		},
		{
			name: "1-5 - fail to create group snapshot with volumes that are not bound",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", &False, nil,
				newVolumeError(`failed to create group snapshot content with error failed to get input parameters to create group snapshot group-snap-1-1: "the PVC claim1-1 is not yet bound to a PV, will not attempt to take a group snapshot"`),
				false, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimArray("claim1-1", "pvc-uid6-1", "1Gi", "", v1.ClaimPending, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: nil,
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  false,
		},
		{
			name: "1-6 - fail to create group snapshot with volumes that are not created by a CSI driver",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", &False, nil,
				newVolumeError("failed to create group snapshot content with error cannot snapshot a non-CSI volume for group snapshot default/group-snap-1-1: volume6-1"),
				false, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: withVolumesLocalPath(
				newVolumeArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
				"/test",
			),
			errors:        noerrors,
			test:          testSyncGroupSnapshot,
			expectSuccess: false,
		},
		{
			name: "1-7 - fail to create group snapshot with volumes that are created by a different CSI driver",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", classGold, "", &False, nil,
				newVolumeError("failed to create group snapshot content with error snapshot controller failed to update default/group-snap-1-1 on API server: Volume CSI driver (test.csi.driver.name) mismatch with VolumeGroupSnapshotClass (csi-mock-plugin) default/group-snap-1-1: volume6-1"),
				false, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: withVolumesCSIDriverName(
				newVolumeArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
				"test.csi.driver.name",
			),
			errors:        noerrors,
			test:          testSyncGroupSnapshot,
			expectSuccess: false,
		},
		{
			name: "1-8 - successful pre-provisioned group snapshot",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", nil,
				"groupsnapcontent-snapuid1-1", classGold, "", &False, nil, nil, false, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", nil,
				"groupsnapcontent-snapuid1-1", classGold, "groupsnapcontent-snapuid1-1", &True, nil, nil, false, false, nil,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "", classGold, nil,
				"group-snapshot-handle", deletionPolicy, nil, false, true,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "", classGold, nil,
				"group-snapshot-handle", deletionPolicy, nil, false, true,
			),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  true,
		},
		{
			name: "1-9 - unsuccessful pre-provisioned group snapshot (no content)",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", nil,
				"groupsnapcontent-snapuid1-1", classGold, "", &False, nil, nil, false, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", nil,
				"groupsnapcontent-snapuid1-1", classGold, "", &False, nil,
				newVolumeError(`VolumeGroupSnapshotContent is missing`),
				false, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: nil,
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  false,
		},
		{
			name: "1-10 - unsuccessful pre-provisioned group snapshot (wrong back-ref)",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", nil,
				"groupsnapcontent-snapuid1-1", classGold, "", &False, nil, nil, false, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", nil,
				"groupsnapcontent-snapuid1-1", classGold, "", &False, nil, newVolumeError(`VolumeGroupSnapshotContent [groupsnapcontent-snapuid1-1] is bound to a different group snapshot`), false, false, nil,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid1-1", "group-snapuid1-1", "group-wrong-snap-1-1", "", classGold, nil,
				"group-snapshot-handle", deletionPolicy, nil, false, true,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid1-1", "group-snapuid1-1", "group-wrong-snap-1-1", "", classGold, nil,
				"group-snapshot-handle", deletionPolicy, nil, false, true,
			),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  false,
		},
		{
			name: "1-11 - mismatch between pre-provisioned group snapshot and dynamically provisioned group snapshot content",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", nil,
				"groupsnapcontent-snapuid1-1", classGold, "", &False, nil, nil, false, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-1-1", "group-snapuid1-1", nil,
				"groupsnapcontent-snapuid1-1", classGold, "", &False, nil, newVolumeError(`VolumeGroupSnapshotContent is dynamically provisioned while expecting a pre-provisioned one`), false, false, nil,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "", classGold, nil,
				"", deletionPolicy, nil, false, false,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid1-1", "group-snapuid1-1", "group-snap-1-1", "", classGold, nil,
				"", deletionPolicy, nil, false, false,
			),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim1-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim1-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  false,
		},
	}
	runSyncTests(t, tests, nil, groupSnapshotClasses)
}
