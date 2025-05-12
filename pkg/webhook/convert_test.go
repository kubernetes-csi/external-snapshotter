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

			err := convertVolumeGroupSnapshotFromV1beta1ToV1beta2(from)
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

			err := convertVolumeGroupSnapshotFromV1beta2ToV1beta1(from)
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
