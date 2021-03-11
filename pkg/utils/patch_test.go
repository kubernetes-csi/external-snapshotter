package utils

import (
	"testing"
	"time"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var fixedTime = metav1.Time{Time: time.Date(2021, time.April, 1, 0, 0, 30, 0, time.UTC)}

func TestCreateSnapshotPatch(t *testing.T) {
	testcases := []struct {
		name             string
		originalSnapshot *crdv1.VolumeSnapshot
		updatedSnapshot  *crdv1.VolumeSnapshot

		expectedPatch string
	}{
		{
			name: "1-1: empty case",

			originalSnapshot: &crdv1.VolumeSnapshot{},
			updatedSnapshot:  &crdv1.VolumeSnapshot{},
			expectedPatch:    `{}`,
		},
		{
			name: "1-2 patch new status",

			originalSnapshot: &crdv1.VolumeSnapshot{},
			updatedSnapshot: &crdv1.VolumeSnapshot{
				Status: &crdv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: stringPtr("test"),
				},
			},
			expectedPatch: `{"status":{"boundVolumeSnapshotContentName":"test"}}`,
		},
		{
			name: "1-3: clear status",

			originalSnapshot: &crdv1.VolumeSnapshot{
				Status: &crdv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: stringPtr("test"),
					CreationTime:                   nil,
					ReadyToUse:                     nil,
					RestoreSize:                    nil,
					Error:                          nil,
				},
			},
			updatedSnapshot: &crdv1.VolumeSnapshot{
				Status: &crdv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: nil,
					CreationTime:                   nil,
					ReadyToUse:                     nil,
					RestoreSize:                    nil,
					Error:                          nil,
				},
			},
			// null json value unmarshals to a nil Go value.
			// This is the expected patch for clearing a boundVolumeSnapshotContentName value.
			expectedPatch: `{"status":{"boundVolumeSnapshotContentName":null}}`,
		},
		{
			name: "1-4: patch snapshotclass",

			originalSnapshot: &crdv1.VolumeSnapshot{
				Spec: crdv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: stringPtr("old"),
				},
			},
			updatedSnapshot: &crdv1.VolumeSnapshot{
				Spec: crdv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: stringPtr("test"),
				},
			},
			expectedPatch: `{"spec":{"volumeSnapshotClassName":"test"}}`,
		},
		{
			name: "1-5: patch snapshotclass no change",
			originalSnapshot: &crdv1.VolumeSnapshot{
				Spec: crdv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: stringPtr("test"),
				},
			},
			updatedSnapshot: &crdv1.VolumeSnapshot{
				Spec: crdv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: stringPtr("test"),
				},
			},
			expectedPatch: `{}`,
		},
		{
			name: "1-6: patch snapshot status with non-nil values and other fields",

			originalSnapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snap-1-6",
					Namespace: "default",
				},
				Spec: crdv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: stringPtr("snap-class-1-6"),
				},
				Status: &crdv1.VolumeSnapshotStatus{
					ReadyToUse: boolPtr(false),
				},
			},
			updatedSnapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snap-1-6",
					Namespace: "default",
				},
				Spec: crdv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: stringPtr("snap-class-1-6"),
				},
				Status: &crdv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: stringPtr("content-1-6"),
					CreationTime:                   &fixedTime,
					ReadyToUse:                     boolPtr(true),
					RestoreSize:                    resource.NewQuantity(1, resource.BinarySI),
				},
			},
			expectedPatch: `{"status":{"boundVolumeSnapshotContentName":"content-1-6","creationTime":"2021-04-01T00:00:30Z","readyToUse":true,"restoreSize":"1"}}`,
		},
		{
			name: "1-7: patch snapshot status with non-nil error and other fields",

			originalSnapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snap-1-7",
					Namespace: "default",
				},
				Spec: crdv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: stringPtr("snap-class-1-7"),
				},
				Status: &crdv1.VolumeSnapshotStatus{
					ReadyToUse: boolPtr(false),
				},
			},
			updatedSnapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snap-1-7",
					Namespace: "default",
				},
				Spec: crdv1.VolumeSnapshotSpec{
					VolumeSnapshotClassName: stringPtr("snap-class-1-7"),
				},
				Status: &crdv1.VolumeSnapshotStatus{
					ReadyToUse: boolPtr(false),
					Error: &crdv1.VolumeSnapshotError{
						Message: stringPtr("failed to patch"),
						Time:    &fixedTime,
					},
				},
			},
			expectedPatch: `{"status":{"error":{"message":"failed to patch","time":"2021-04-01T00:00:30Z"}}}`,
		},
	}
	for _, tc := range testcases {
		t.Logf("test: %v", tc.name)

		patch, err := createSnapshotPatch(tc.originalSnapshot, tc.updatedSnapshot)
		if err != nil {
			t.Fatalf("Encountered unexpected error: %v", err)
		}
		if string(patch) != tc.expectedPatch {
			t.Errorf("Patch not equal to expected patch:\n     GOT: %s\nEXPECTED: %s", patch, tc.expectedPatch)
		}
	}
}

func TestCreateSnapshotContentPatch(t *testing.T) {
	testcases := []struct {
		name                    string
		originalSnapshotContent *crdv1.VolumeSnapshotContent
		updatedSnapshotContent  *crdv1.VolumeSnapshotContent

		expectedPatch string
	}{
		{
			name: "1-1: empty case",

			originalSnapshotContent: &crdv1.VolumeSnapshotContent{},
			updatedSnapshotContent:  &crdv1.VolumeSnapshotContent{},
			expectedPatch:           `{}`,
		},
		{
			name: "1-2: patch new status",

			originalSnapshotContent: &crdv1.VolumeSnapshotContent{},
			updatedSnapshotContent: &crdv1.VolumeSnapshotContent{
				Status: &crdv1.VolumeSnapshotContentStatus{
					SnapshotHandle: stringPtr("test"),
				},
			},
			expectedPatch: `{"status":{"snapshotHandle":"test"}}`,
		},
		{
			name: "1-3: clear status",

			originalSnapshotContent: &crdv1.VolumeSnapshotContent{
				Status: &crdv1.VolumeSnapshotContentStatus{
					SnapshotHandle: stringPtr("test"),
					CreationTime:   nil,
					ReadyToUse:     nil,
					RestoreSize:    nil,
					Error:          nil,
				},
			},
			updatedSnapshotContent: &crdv1.VolumeSnapshotContent{
				Status: &crdv1.VolumeSnapshotContentStatus{
					SnapshotHandle: nil,
					CreationTime:   nil,
					ReadyToUse:     nil,
					RestoreSize:    nil,
					Error:          nil,
				},
			},
			// null json value unmarshals to a nil Go value.
			// This is the expected patch for clearing a snapshotHandle value.
			expectedPatch: `{"status":{"snapshotHandle":null}}`,
		},
		{
			name: "1-4: patch snapshotclass",

			originalSnapshotContent: &crdv1.VolumeSnapshotContent{
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: stringPtr("old"),
				},
			},
			updatedSnapshotContent: &crdv1.VolumeSnapshotContent{
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: stringPtr("test"),
				},
			},
			expectedPatch: `{"spec":{"volumeSnapshotClassName":"test"}}`,
		},
		{
			name: "1-5: patch snapshotclass no change",

			originalSnapshotContent: &crdv1.VolumeSnapshotContent{
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: stringPtr("test"),
				},
			},
			updatedSnapshotContent: &crdv1.VolumeSnapshotContent{
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: stringPtr("test"),
				},
			},
			expectedPatch: `{}`,
		},
		{
			name: "1-6: patch snapshot content status with non-nil values and other fields",

			originalSnapshotContent: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "content-1-6",
					Namespace: "default",
				},
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: stringPtr("snap-class-1-6"),
				},
				Status: &crdv1.VolumeSnapshotContentStatus{
					ReadyToUse: boolPtr(false),
				},
			},
			updatedSnapshotContent: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "content-1-6",
					Namespace: "default",
				},
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: stringPtr("snap-class-1-6"),
				},
				Status: &crdv1.VolumeSnapshotContentStatus{
					SnapshotHandle: stringPtr("snap-1-6"),
					CreationTime:   int64Ptr(100005000),
					ReadyToUse:     boolPtr(true),
					RestoreSize:    int64Ptr(500),
				},
			},
			expectedPatch: `{"status":{"creationTime":100005000,"readyToUse":true,"restoreSize":500,"snapshotHandle":"snap-1-6"}}`,
		},
		{
			name: "1-7: patch snapshot content status with non-nil error and other fields",

			originalSnapshotContent: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "content-1-7",
					Namespace: "default",
				},
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: stringPtr("snap-class-1-7"),
				},
				Status: &crdv1.VolumeSnapshotContentStatus{
					ReadyToUse: boolPtr(false),
				},
			},
			updatedSnapshotContent: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "content-1-7",
					Namespace: "default",
				},
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: stringPtr("snap-class-1-7"),
				},
				Status: &crdv1.VolumeSnapshotContentStatus{
					ReadyToUse: boolPtr(false),
					Error: &crdv1.VolumeSnapshotError{
						Message: stringPtr("failed to patch"),
						Time:    &fixedTime,
					},
				},
			},
			expectedPatch: `{"status":{"error":{"message":"failed to patch","time":"2021-04-01T00:00:30Z"}}}`,
		},
	}
	for _, tc := range testcases {
		t.Logf("test: %v", tc.name)

		patch, err := createContentPatch(tc.originalSnapshotContent, tc.updatedSnapshotContent)
		if err != nil {
			t.Fatalf("Encountered unexpected error: %v", err)
		}
		if string(patch) != tc.expectedPatch {
			t.Errorf("Patch not equal to expected patch:\n     GOT: %s\nEXPECTED: %s", patch, tc.expectedPatch)
		}
	}
}

func stringPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }

func int64Ptr(i int64) *int64 { return &i }
