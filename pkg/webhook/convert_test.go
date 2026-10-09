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

package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func TestFromBeta1ToBeta2(t *testing.T) {
	matches, _ := filepath.Glob("testdata/v1beta1_to_v1beta2/*v1beta1*yaml")
	for _, beta1FileName := range matches {
		beta2FileName := strings.ReplaceAll(beta1FileName, "v1beta1.yaml", "v1beta2.yaml")

		t.Run(fmt.Sprintf("%s -> %s", beta1FileName, beta2FileName), func(t *testing.T) {
			from := fromFile(t, beta1FileName)
			to := fromFile(t, beta2FileName)

			err := convertVolumeGroupSnapshotContentFromV1beta1ToV1beta2(from)
			if err != nil {
				t.Fatalf("conversion failed: %v", err.Error())
			}

			// The API version is changed by the framework, here we emulate it
			from.SetAPIVersion(to.GetAPIVersion())

			if !equality.Semantic.DeepEqual(from, to) {
				fromJSON, _ := json.MarshalIndent(from, "", "  ")
				toJSON, _ := json.MarshalIndent(to, "", "  ")
				t.Errorf("unexpected result %v vs %v", string(fromJSON), string(toJSON))
			}
		})
	}
}

func TestFromBeta2ToBeta1(t *testing.T) {
	matches, _ := filepath.Glob("testdata/v1beta2_to_v1beta1/*v1beta1*yaml")
	for _, beta1FileName := range matches {
		beta2FileName := strings.ReplaceAll(beta1FileName, "v1beta1.yaml", "v1beta2.yaml")

		t.Run(fmt.Sprintf("%s -> %s", beta2FileName, beta1FileName), func(t *testing.T) {
			from := fromFile(t, beta2FileName)
			to := fromFile(t, beta1FileName)

			err := convertVolumeGroupSnapshotContentFromV1beta2ToV1beta1(from)
			if err != nil {
				t.Fatalf("conversion failed: %v", err.Error())
			}

			// The API version is changed by the framework, here we emulate it
			from.SetAPIVersion(to.GetAPIVersion())

			if !equality.Semantic.DeepEqual(from, to) {
				fromJSON, _ := json.MarshalIndent(from, "", "  ")
				toJSON, _ := json.MarshalIndent(to, "", "  ")
				t.Errorf("unexpected result %v vs %v", string(fromJSON), string(toJSON))
			}
		})
	}
}

func TestConvertGroupSnapshotCRDToAndFromV1(t *testing.T) {
	testCases := []struct {
		name        string
		fromFile    string
		fromVersion string
		toFile      string
		toVersion   string
	}{
		{
			name:        "v1 to v1beta2 leaves the object unchanged",
			fromFile:    "testdata/v1beta2_to_v1beta1/annotation_status_v1beta2.yaml",
			fromVersion: v1Version,
			toFile:      "testdata/v1beta2_to_v1beta1/annotation_status_v1beta2.yaml",
			toVersion:   v1beta2Version,
		},
		{
			name:        "v1beta2 to v1 leaves the object unchanged",
			fromFile:    "testdata/v1beta2_to_v1beta1/annotation_status_v1beta2.yaml",
			fromVersion: v1beta2Version,
			toFile:      "testdata/v1beta2_to_v1beta1/annotation_status_v1beta2.yaml",
			toVersion:   v1Version,
		},
		{
			name:        "v1 to v1beta1 converts as v1beta2 does",
			fromFile:    "testdata/v1beta2_to_v1beta1/annotation_status_v1beta2.yaml",
			fromVersion: v1Version,
			toFile:      "testdata/v1beta2_to_v1beta1/annotation_status_v1beta1.yaml",
			toVersion:   v1beta1Version,
		},
		{
			name:        "v1beta1 to v1 converts as it does to v1beta2",
			fromFile:    "testdata/v1beta1_to_v1beta2/annotation_status_v1beta1.yaml",
			fromVersion: v1beta1Version,
			toFile:      "testdata/v1beta1_to_v1beta2/annotation_status_v1beta2.yaml",
			toVersion:   v1Version,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			from := fromFile(t, tc.fromFile)
			from.SetAPIVersion(tc.fromVersion)

			to := fromFile(t, tc.toFile)
			to.SetAPIVersion(tc.toVersion)

			converted, status := convertGroupSnapshotCRD(from, tc.toVersion)
			if status.Status != metav1.StatusSuccess {
				t.Fatalf("conversion from %q to %q failed: %v", tc.fromVersion, tc.toVersion, status.Message)
			}

			// The API version is changed by the framework, here we emulate it
			converted.SetAPIVersion(tc.toVersion)

			if !equality.Semantic.DeepEqual(converted, to) {
				convertedJSON, _ := json.MarshalIndent(converted, "", "  ")
				toJSON, _ := json.MarshalIndent(to, "", "  ")
				t.Errorf("unexpected result %v vs %v", string(convertedJSON), string(toJSON))
			}
		})
	}
}

func TestConvertGroupSnapshotCRDRejectsUnknownVersion(t *testing.T) {
	from := fromFile(t, "testdata/v1beta2_to_v1beta1/annotation_status_v1beta2.yaml")

	_, status := convertGroupSnapshotCRD(from, "groupsnapshot.storage.k8s.io/v2")
	if status.Status != metav1.StatusFailure {
		t.Fatalf("expected conversion to an unknown version to fail, got %q", status.Status)
	}
}

func fromFile(t *testing.T, fileName string) *unstructured.Unstructured {
	file, err := os.Open(fileName)
	if err != nil {
		t.Fatalf("opening file %q: %v", fileName, err)
	}

	defer func() {
		_ = file.Close()
	}()

	obj := &unstructured.Unstructured{
		Object: map[string]any{},
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("reading file %q: %v", fileName, err)
	}

	err = yaml.Unmarshal(data, &obj.Object)
	if err != nil {
		t.Fatalf("unmarshalling JSON from file %q: %v", fileName, err)
	}

	return obj
}
