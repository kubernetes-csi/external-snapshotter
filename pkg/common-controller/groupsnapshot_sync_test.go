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

package common_controller

import (
	"fmt"
	"strings"
	"testing"

	crdv1beta2 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta2"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSyncReadyGroupSnapshot tests the syncReadyGroupSnapshot function
func TestSyncReadyGroupSnapshot(t *testing.T) {
	tests := []controllerTest{
		{
			name: "6-1 - ready group snapshot with valid content binding",
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-6-1", "group-snapuid6-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid6-1", &True, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-6-1", "group-snapuid6-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid6-1", &True, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid6-1", "group-snapuid6-1", "group-snap-6-1", "group-snapshot-handle", classGold, []string{
					"1-pv-handle6-1",
					"2-pv-handle6-1",
				}, "", deletionPolicy, nil, false, true,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid6-1", "group-snapuid6-1", "group-snap-6-1", "group-snapshot-handle", classGold, []string{
					"1-pv-handle6-1",
					"2-pv-handle6-1",
				}, "", deletionPolicy, nil, false, true,
			),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim6-1", "pvc-uid6-1", "1Gi", "volume6-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-1", "pv-uid6-1", "pv-handle6-1", "1Gi", "pvc-uid6-1", "claim6-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  true,
		},
		{
			name: "6-2 - ready group snapshot with missing content",
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-6-2", "group-snapuid6-2", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-missing", &True, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-6-2", "group-snapuid6-2", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-missing", &False, nil,
					newVolumeError("VolumeGroupSnapshotContent is missing"),
					false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim6-2", "pvc-uid6-2", "1Gi", "volume6-2", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-2", "pv-uid6-2", "pv-handle6-2", "1Gi", "pvc-uid6-2", "claim6-2", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  false,
		},
		{
			name: "6-3 - ready group snapshot with misbound content",
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-6-3", "group-snapuid6-3", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid6-3", &True, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-6-3", "group-snapuid6-3", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid6-3", &False, nil,
					newVolumeError("VolumeGroupSnapshotContent [groupsnapcontent-group-snapuid6-3] is bound to a different group snapshot"),
					false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid6-3", "wrong-uid", "wrong-snap", "group-snapshot-handle", classGold, []string{
					"1-pv-handle6-3",
					"2-pv-handle6-3",
				}, "", deletionPolicy, nil, false, true,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid6-3", "wrong-uid", "wrong-snap", "group-snapshot-handle", classGold, []string{
					"1-pv-handle6-3",
					"2-pv-handle6-3",
				}, "", deletionPolicy, nil, false, true,
			),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim6-3", "pvc-uid6-3", "1Gi", "volume6-3", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume6-3", "pv-uid6-3", "pv-handle6-3", "1Gi", "pvc-uid6-3", "claim6-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshot,
			expectSuccess:  false,
		},
	}
	runSyncTests(t, tests, nil, groupSnapshotClasses)
}

// TestGroupSnapshotContentSync tests the syncGroupSnapshotContent function
func TestGroupSnapshotContentSync(t *testing.T) {
	tests := []controllerTest{
		{
			name: "7-1 - sync group snapshot content adds finalizer to content",
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-7-1", "group-snapuid7-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid7-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-7-1", "group-snapuid7-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid7-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-group-snapuid7-1", "group-snapuid7-1", "group-snap-7-1", "group-snapshot-handle", classGold, []string{
					"1-pv-handle7-1",
					"2-pv-handle7-1",
				}, "", deletionPolicy, nil, false, true,
			),
			expectedGroupContents: withGroupSnapshotContentFinalizers(
				newGroupSnapshotContentArray(
					"groupsnapcontent-group-snapuid7-1", "group-snapuid7-1", "group-snap-7-1", "group-snapshot-handle", classGold, []string{
						"1-pv-handle7-1",
						"2-pv-handle7-1",
					}, "", deletionPolicy, nil, false, true,
				),
				utils.VolumeGroupSnapshotContentFinalizer,
			),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim7-1", "pvc-uid7-1", "1Gi", "volume7-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume7-1", "pv-uid7-1", "pv-handle7-1", "1Gi", "pvc-uid7-1", "claim7-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test:           testSyncGroupSnapshotContent,
			expectSuccess:  true,
		},
	}
	runSyncTests(t, tests, nil, groupSnapshotClasses)
}

// TestSetDefaultGroupSnapshotClass tests the SetDefaultGroupSnapshotClass function
func TestSetDefaultGroupSnapshotClass(t *testing.T) {
	// Use only one default class
	singleDefaultClass := []*crdv1beta2.VolumeGroupSnapshotClass{
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
	}

	tests := []controllerTest{
		{
			name: "5-1 - SetDefaultGroupSnapshotClass returns default class for dynamic snapshot",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-5-1", "group-snapuid5-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", "", "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-5-1", "group-snapuid5-1", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", defaultClass, "", nil, nil, nil, true, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim5-1", "pvc-uid5-1", "1Gi", "volume5-1", v1.ClaimBound, &defaultClass),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume5-1", "pv-uid5-1", "pv-handle5-1", "1Gi", "pvc-uid5-1", "claim5-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, defaultClass),
			errors:         noerrors,
			test:           testSetDefaultGroupSnapshotClass,
			expectSuccess:  true,
		},
		{
			name: "5-2 - SetDefaultGroupSnapshotClass returns nil for pre-provisioned snapshot",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-5-2", "group-snapuid5-2", nil,
				"groupsnapcontent-snapuid5-2", "", "", &False, nil, nil, false, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-5-2", "group-snapuid5-2", nil,
				"groupsnapcontent-snapuid5-2", "", "", &False, nil, nil, false, false, nil,
			),
			initialGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid5-2", "group-snapuid5-2", "group-snap-5-2", "", "", nil,
				"group-snapshot-handle", deletionPolicy, nil, false, true,
			),
			expectedGroupContents: newGroupSnapshotContentArray(
				"groupsnapcontent-snapuid5-2", "group-snapuid5-2", "group-snap-5-2", "", "", nil,
				"group-snapshot-handle", deletionPolicy, nil, false, true,
			),
			initialClaims:  nil,
			initialVolumes: nil,
			errors:         noerrors,
			test:           testSetDefaultGroupSnapshotClass,
			expectSuccess:  true,
		},
	}
	runSyncTests(t, tests, nil, singleDefaultClass)
}

// TestSetDefaultGroupSnapshotClassMultipleDefaults tests error handling when multiple default classes exist
func TestSetDefaultGroupSnapshotClassMultipleDefaults(t *testing.T) {
	// Create multiple default classes with the same driver - this should cause an error
	multipleDefaultClasses := []*crdv1beta2.VolumeGroupSnapshotClass{
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

	tests := []controllerTest{
		{
			name: "5-3 - SetDefaultGroupSnapshotClass fails when multiple default classes exist",
			initialGroupSnapshots: newGroupSnapshotArray(
				"group-snap-5-3", "group-snapuid5-3", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", "", "", nil, nil, nil, true, false, nil,
			),
			expectedGroupSnapshots: newGroupSnapshotArray(
				"group-snap-5-3", "group-snapuid5-3", map[string]string{
					"app.kubernetes.io/name": "postgresql",
				},
				"", "", "", nil, nil, nil, true, false, nil,
			),
			initialGroupContents:  nogroupcontents,
			expectedGroupContents: nogroupcontents,
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim5-3", "pvc-uid5-3", "1Gi", "volume5-3", v1.ClaimBound, &defaultClass),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume5-3", "pv-uid5-3", "pv-handle5-3", "1Gi", "pvc-uid5-3", "claim5-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, defaultClass),
			errors:         noerrors,
			test:           testSetDefaultGroupSnapshotClassMultipleDefaults,
			expectSuccess:  false,
		},
	}
	runSyncTests(t, tests, nil, multipleDefaultClasses)
}

// testSetDefaultGroupSnapshotClassMultipleDefaults tests that an error is returned when multiple default classes exist
func testSetDefaultGroupSnapshotClassMultipleDefaults(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	groupSnapshot := test.initialGroupSnapshots[0]
	class, _, err := ctrl.SetDefaultGroupSnapshotClass(groupSnapshot)

	// Should return an error when multiple default classes exist
	if err == nil {
		return fmt.Errorf("expected error when multiple default classes exist, but got nil")
	}

	// Verify the error message mentions multiple defaults
	expectedErrMsg := "default snapshot classes were found"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		return fmt.Errorf("expected error message to contain '%s', got: %v", expectedErrMsg, err)
	}

	// Class should be nil when error occurs
	if class != nil {
		return fmt.Errorf("expected nil class when error occurs, got: %v", class)
	}

	return nil
}

// testSetDefaultGroupSnapshotClass is a test function that calls SetDefaultGroupSnapshotClass directly
func testSetDefaultGroupSnapshotClass(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
	groupSnapshot := test.initialGroupSnapshots[0]
	class, newGroupSnapshot, err := ctrl.SetDefaultGroupSnapshotClass(groupSnapshot)

	// For pre-provisioned snapshots, class should be nil and no error
	if groupSnapshot.Spec.Source.VolumeGroupSnapshotContentName != nil {
		if err != nil {
			return fmt.Errorf("expected no error for pre-provisioned snapshot, got: %v", err)
		}
		if class != nil {
			return fmt.Errorf("expected nil class for pre-provisioned snapshot, got: %s", class.Name)
		}
		return nil
	}

	// For dynamic snapshots, we should get a class
	if err != nil {
		return err
	}
	if class == nil {
		return fmt.Errorf("expected a default class for dynamic snapshot, got nil")
	}

	// Verify it's the default class
	if class.Name != defaultClass {
		return fmt.Errorf("expected default class %s, got %s", defaultClass, class.Name)
	}

	// Update the reactor's group snapshot with the new one that has the class name set
	if newGroupSnapshot != nil {
		reactor.lock.Lock()
		reactor.groupSnapshots[newGroupSnapshot.Name] = newGroupSnapshot
		reactor.lock.Unlock()
	}

	return nil
}

// TestIndividualSnapshotCreation tests the isGroupSnapshotContentReadyForSnapshotCreation function
func TestIndividualSnapshotCreation(t *testing.T) {
	sizeBytes := int64(1073741824) // 1Gi in bytes

	tests := []controllerTest{
		{
			name: "8-1 - content is ready when status and VolumeSnapshotInfoList are populated",
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-8-1", "group-snapuid8-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid8-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-8-1", "group-snapuid8-1", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid8-1", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: func() []*crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"groupsnapcontent-group-snapuid8-1", "group-snapuid8-1", "group-snap-8-1",
					"", classGold, []string{"pv-handle8-1", "pv-handle8-2"}, "", deletionPolicy, nil, false, false,
				)
				// Add status with VolumeSnapshotInfoList and VolumeGroupSnapshotHandle
				ready := true
				content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
					ReadyToUse:                &ready,
					VolumeGroupSnapshotHandle: stringPtr("group-snapshot-handle-8-1"),
					VolumeSnapshotInfoList: []crdv1beta2.VolumeSnapshotInfo{
						{
							VolumeHandle:   "pv-handle8-1",
							SnapshotHandle: "snapshot-handle-8-1",
							ReadyToUse:     &ready,
							RestoreSize:    &sizeBytes,
						},
						{
							VolumeHandle:   "pv-handle8-2",
							SnapshotHandle: "snapshot-handle-8-2",
							ReadyToUse:     &ready,
							RestoreSize:    &sizeBytes,
						},
					},
				}
				return []*crdv1beta2.VolumeGroupSnapshotContent{content}
			}(),
			expectedGroupContents: func() []*crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"groupsnapcontent-group-snapuid8-1", "group-snapuid8-1", "group-snap-8-1",
					"", classGold, []string{"pv-handle8-1", "pv-handle8-2"}, "", deletionPolicy, nil, false, false,
				)
				ready := true
				content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
					ReadyToUse:                &ready,
					VolumeGroupSnapshotHandle: stringPtr("group-snapshot-handle-8-1"),
					VolumeSnapshotInfoList: []crdv1beta2.VolumeSnapshotInfo{
						{
							VolumeHandle:   "pv-handle8-1",
							SnapshotHandle: "snapshot-handle-8-1",
							ReadyToUse:     &ready,
							RestoreSize:    &sizeBytes,
						},
						{
							VolumeHandle:   "pv-handle8-2",
							SnapshotHandle: "snapshot-handle-8-2",
							ReadyToUse:     &ready,
							RestoreSize:    &sizeBytes,
						},
					},
				}
				return []*crdv1beta2.VolumeGroupSnapshotContent{content}
			}(),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim8-1", "pvc-uid8-1", "1Gi", "volume8-1", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume8-1", "pv-uid8-1", "pv-handle8-1", "1Gi", "pvc-uid8-1", "claim8-1", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test: func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
				return testIndividualSnapshotCreationReadiness(ctrl, reactor, test, true)
			},
			expectSuccess: true,
		},
		{
			name: "8-2 - content is not ready when status is nil",
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-8-2", "group-snapuid8-2", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid8-2", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-8-2", "group-snapuid8-2", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid8-2", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: []*crdv1beta2.VolumeGroupSnapshotContent{
				newGroupSnapshotContent(
					"groupsnapcontent-group-snapuid8-2", "group-snapuid8-2", "group-snap-8-2",
					"", classGold, []string{"pv-handle8-2"}, "", deletionPolicy, nil, false, false,
				),
				// Status is nil
			},
			expectedGroupContents: []*crdv1beta2.VolumeGroupSnapshotContent{
				newGroupSnapshotContent(
					"groupsnapcontent-group-snapuid8-2", "group-snapuid8-2", "group-snap-8-2",
					"", classGold, []string{"pv-handle8-2"}, "", deletionPolicy, nil, false, false,
				),
			},
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim8-2", "pvc-uid8-2", "1Gi", "volume8-2", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume8-2", "pv-uid8-2", "pv-handle8-2", "1Gi", "pvc-uid8-2", "claim8-2", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test: func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
				return testIndividualSnapshotCreationReadiness(ctrl, reactor, test, false)
			},
			expectSuccess: true,
		},
		{
			name: "8-3 - content is not ready when VolumeSnapshotInfoList is empty",
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-8-3", "group-snapuid8-3", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid8-3", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-8-3", "group-snapuid8-3", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid8-3", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: func() []*crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"groupsnapcontent-group-snapuid8-3", "group-snapuid8-3", "group-snap-8-3",
					"", classGold, []string{"pv-handle8-3"}, "", deletionPolicy, nil, false, false,
				)
				// Add status but with empty VolumeSnapshotInfoList
				ready := true
				content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
					ReadyToUse:                &ready,
					VolumeGroupSnapshotHandle: stringPtr("group-snapshot-handle-8-3"),
					VolumeSnapshotInfoList:    []crdv1beta2.VolumeSnapshotInfo{}, // Empty list
				}
				return []*crdv1beta2.VolumeGroupSnapshotContent{content}
			}(),
			expectedGroupContents: func() []*crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"groupsnapcontent-group-snapuid8-3", "group-snapuid8-3", "group-snap-8-3",
					"", classGold, []string{"pv-handle8-3"}, "", deletionPolicy, nil, false, false,
				)
				ready := true
				content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
					ReadyToUse:                &ready,
					VolumeGroupSnapshotHandle: stringPtr("group-snapshot-handle-8-3"),
					VolumeSnapshotInfoList:    []crdv1beta2.VolumeSnapshotInfo{},
				}
				return []*crdv1beta2.VolumeGroupSnapshotContent{content}
			}(),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim8-3", "pvc-uid8-3", "1Gi", "volume8-3", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume8-3", "pv-uid8-3", "pv-handle8-3", "1Gi", "pvc-uid8-3", "claim8-3", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test: func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
				return testIndividualSnapshotCreationReadiness(ctrl, reactor, test, false)
			},
			expectSuccess: true,
		},
		{
			name: "8-4 - content is not ready when VolumeGroupSnapshotHandle is nil",
			initialGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-8-4", "group-snapuid8-4", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid8-4", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			expectedGroupSnapshots: withGroupSnapshotFinalizers(
				newGroupSnapshotArray(
					"group-snap-8-4", "group-snapuid8-4", map[string]string{
						"app.kubernetes.io/name": "postgresql",
					},
					"", classGold, "groupsnapcontent-group-snapuid8-4", &False, nil, nil, false, false, nil,
				),
				utils.VolumeGroupSnapshotBoundFinalizer,
			),
			initialGroupContents: func() []*crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"groupsnapcontent-group-snapuid8-4", "group-snapuid8-4", "group-snap-8-4",
					"", classGold, []string{"pv-handle8-4"}, "", deletionPolicy, nil, false, false,
				)
				// Add status with VolumeSnapshotInfoList but no VolumeGroupSnapshotHandle
				ready := true
				content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
					ReadyToUse:                &ready,
					VolumeGroupSnapshotHandle: nil, // Missing handle
					VolumeSnapshotInfoList: []crdv1beta2.VolumeSnapshotInfo{
						{
							VolumeHandle:   "pv-handle8-4",
							SnapshotHandle: "snapshot-handle-8-4",
							ReadyToUse:     &ready,
							RestoreSize:    &sizeBytes,
						},
					},
				}
				return []*crdv1beta2.VolumeGroupSnapshotContent{content}
			}(),
			expectedGroupContents: func() []*crdv1beta2.VolumeGroupSnapshotContent {
				content := newGroupSnapshotContent(
					"groupsnapcontent-group-snapuid8-4", "group-snapuid8-4", "group-snap-8-4",
					"", classGold, []string{"pv-handle8-4"}, "", deletionPolicy, nil, false, false,
				)
				ready := true
				content.Status = &crdv1beta2.VolumeGroupSnapshotContentStatus{
					ReadyToUse:                &ready,
					VolumeGroupSnapshotHandle: nil,
					VolumeSnapshotInfoList: []crdv1beta2.VolumeSnapshotInfo{
						{
							VolumeHandle:   "pv-handle8-4",
							SnapshotHandle: "snapshot-handle-8-4",
							ReadyToUse:     &ready,
							RestoreSize:    &sizeBytes,
						},
					},
				}
				return []*crdv1beta2.VolumeGroupSnapshotContent{content}
			}(),
			initialClaims: withClaimLabels(
				newClaimCoupleArray("claim8-4", "pvc-uid8-4", "1Gi", "volume8-4", v1.ClaimBound, &classGold),
				map[string]string{
					"app.kubernetes.io/name": "postgresql",
				}),
			initialVolumes: newVolumeCoupleArray("volume8-4", "pv-uid8-4", "pv-handle8-4", "1Gi", "pvc-uid8-4", "claim8-4", v1.VolumeBound, v1.PersistentVolumeReclaimDelete, classGold),
			errors:         noerrors,
			test: func(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest) error {
				return testIndividualSnapshotCreationReadiness(ctrl, reactor, test, false)
			},
			expectSuccess: true,
		},
	}
	runSyncTests(t, tests, nil, groupSnapshotClasses)
}

// testIndividualSnapshotCreationReadiness tests that isGroupSnapshotContentReadyForSnapshotCreation works correctly
// expectedReady indicates whether the content should be ready for snapshot creation
func testIndividualSnapshotCreationReadiness(ctrl *csiSnapshotCommonController, reactor *snapshotReactor, test controllerTest, expectedReady bool) error {
	groupSnapshotContent := test.initialGroupContents[0]

	// Call the function under test
	isReady := ctrl.isGroupSnapshotContentReadyForSnapshotCreation(groupSnapshotContent)

	// Verify the result matches expectations
	if isReady != expectedReady {
		return fmt.Errorf("expected isGroupSnapshotContentReadyForSnapshotCreation to return %v, got %v", expectedReady, isReady)
	}

	return nil
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
