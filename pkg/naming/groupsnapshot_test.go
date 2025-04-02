/*
Copyright 2025 The Kubernetes Authors.

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

package naming

import "testing"

func TestMemberSnapshotsNaming(t *testing.T) {
	results := []struct {
		groupSnapshotUUID string
		volumeHandleUUID  string
		expectedName      string
	}{
		{
			groupSnapshotUUID: "b0e14ea3-138d-11f0-a142-da4b6452e9d8",
			volumeHandleUUID:  "e8e932dc-4911-40c7-9da3-3f3d85c49458",
			expectedName:      "snapshot-a1cafe086c8f775046b4d4f51ae8ee57bd72c595399a04c1ec64eedbed64bc49",
		},
		{
			groupSnapshotUUID: "b0e14ea3-138d-11f0-a142-da4b6452e9d8",
			volumeHandleUUID:  "054c23b9-d11a-4383-88a1-e1ef2648fb9f",
			expectedName:      "snapshot-8707de99194420fc588501d1800f11c41046c58810ddd7ac671d0b035b0ce07f",
		},
	}

	for _, test := range results {
		result := GetSnapshotNameForVolumeGroupSnapshotContent(test.groupSnapshotUUID, test.volumeHandleUUID)
		if result != test.expectedName {
			t.Errorf("Wrong volume snapshot name:[%s] expected:[%s]", result, test.expectedName)
		}
	}
}

func TestMemberSnapshotContentsNaming(t *testing.T) {
	results := []struct {
		groupSnapshotUUID string
		volumeHandleUUID  string
		expectedName      string
	}{
		{
			groupSnapshotUUID: "b0e14ea3-138d-11f0-a142-da4b6452e9d8",
			volumeHandleUUID:  "e8e932dc-4911-40c7-9da3-3f3d85c49458",
			expectedName:      "snapcontent-a1cafe086c8f775046b4d4f51ae8ee57bd72c595399a04c1ec64eedbed64bc49",
		},
		{
			groupSnapshotUUID: "b0e14ea3-138d-11f0-a142-da4b6452e9d8",
			volumeHandleUUID:  "054c23b9-d11a-4383-88a1-e1ef2648fb9f",
			expectedName:      "snapcontent-8707de99194420fc588501d1800f11c41046c58810ddd7ac671d0b035b0ce07f",
		},
	}

	for _, test := range results {
		result := GetSnapshotContentNameForVolumeGroupSnapshotContent(test.groupSnapshotUUID, test.volumeHandleUUID)
		if result != test.expectedName {
			t.Errorf("Wrong volume snapshot name:[%s] expected:[%s]", result, test.expectedName)
		}
	}
}
