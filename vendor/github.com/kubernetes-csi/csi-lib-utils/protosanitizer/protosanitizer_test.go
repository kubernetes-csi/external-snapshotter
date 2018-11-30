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

package protosanitizer

import (
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer/test/csitest"
	"github.com/stretchr/testify/assert"
)

func TestStripSecrets(t *testing.T) {
	secretName := "secret-abc"
	secretValue := "123"
	createVolume := &csi.CreateVolumeRequest{
		AccessibilityRequirements: &csi.TopologyRequirement{
			Requisite: []*csi.Topology{
				&csi.Topology{
					Segments: map[string]string{
						"foo": "bar",
						"x":   "y",
					},
				},
				&csi.Topology{
					Segments: map[string]string{
						"a": "b",
					},
				},
			},
		},
		Name: "foo",
		VolumeCapabilities: []*csi.VolumeCapability{
			&csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						FsType: "ext4",
					},
				},
			},
		},
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 1024,
		},
		Secrets: map[string]string{
			secretName:   secretValue,
			"secret-xyz": "987",
		},
	}

	cases := []struct {
		original, stripped interface{}
	}{
		{nil, "null"},
		{1, "1"},
		{"hello world", `"hello world"`},
		{true, "true"},
		{false, "false"},
		{&csi.CreateVolumeRequest{}, `{}`},
		// Test case from https://github.com/kubernetes-csi/csi-lib-utils/pull/1#pullrequestreview-180126394.
		{&csi.CreateVolumeRequest{
			Name: "test-volume",
			CapacityRange: &csi.CapacityRange{
				RequiredBytes: int64(1024),
				LimitBytes:    int64(1024),
			},
			VolumeCapabilities: []*csi.VolumeCapability{
				&csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"flag1", "flag2", "flag3"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
			},
			Secrets:                   map[string]string{"secret1": "secret1", "secret2": "secret2"},
			Parameters:                map[string]string{"param1": "param1", "param2": "param2"},
			VolumeContentSource:       &csi.VolumeContentSource{},
			AccessibilityRequirements: &csi.TopologyRequirement{},
		}, `{"accessibility_requirements":{},"capacity_range":{"limit_bytes":1024,"required_bytes":1024},"name":"test-volume","parameters":{"param1":"param1","param2":"param2"},"secrets":"***stripped***","volume_capabilities":[{"AccessType":{"Mount":{"fs_type":"ext4","mount_flags":["flag1","flag2","flag3"]}},"access_mode":{"mode":5}}],"volume_content_source":{"Type":null}}`},
		{createVolume, `{"accessibility_requirements":{"requisite":[{"segments":{"foo":"bar","x":"y"}},{"segments":{"a":"b"}}]},"capacity_range":{"required_bytes":1024},"name":"foo","secrets":"***stripped***","volume_capabilities":[{"AccessType":{"Mount":{"fs_type":"ext4"}}}]}`},
		{&csitest.CreateVolumeRequest{}, `{}`},
		{&csitest.CreateVolumeRequest{
			CapacityRange: &csitest.CapacityRange{
				RequiredBytes: 1024,
			},
			MaybeSecretMap: map[int64]*csitest.VolumeCapability{
				1: &csitest.VolumeCapability{ArraySecret: "aaa"},
				2: &csitest.VolumeCapability{ArraySecret: "bbb"},
			},
			Name:         "foo",
			NewSecretInt: 42,
			Seecreets: map[string]string{
				secretName:   secretValue,
				"secret-xyz": "987",
			},
			VolumeCapabilities: []*csitest.VolumeCapability{
				&csitest.VolumeCapability{
					AccessType: &csitest.VolumeCapability_Mount{
						Mount: &csitest.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
					ArraySecret: "knock knock",
				},
				&csitest.VolumeCapability{
					ArraySecret: "Who's there?",
				},
			},
			VolumeContentSource: &csitest.VolumeContentSource{
				Type: &csitest.VolumeContentSource_Volume{
					Volume: &csitest.VolumeContentSource_VolumeSource{
						VolumeId:         "abc",
						OneofSecretField: "hello",
					},
				},
				NestedSecretField: "world",
			},
		},
			// Secrets are *not* removed from all fields yet. This will have to be fixed one way or another
			// before the CSI spec can start using secrets there (currently it doesn't).
			// The test is still useful because it shows that also complicated fields get serialized.
			// `{"capacity_range":{"required_bytes":1024},"maybe_secret_map":{"1":{"AccessType":null,"array_secret":"***stripped***"},"2":{"AccessType":null,"array_secret":"***stripped***"}},"name":"foo","new_secret_int":"***stripped***","seecreets":"***stripped***","volume_capabilities":[{"AccessType":{"Mount":{"fs_type":"ext4"}},"array_secret":"***stripped***"},{"AccessType":null,"array_secret":"***stripped***"}],"volume_content_source":{"Type":{"Volume":{"oneof_secret_field":"***stripped***","volume_id":"abc"}},"nested_secret_field":"***stripped***"}}`,
			`{"capacity_range":{"required_bytes":1024},"maybe_secret_map":{"1":{"AccessType":null,"array_secret":"aaa"},"2":{"AccessType":null,"array_secret":"bbb"}},"name":"foo","new_secret_int":"***stripped***","seecreets":"***stripped***","volume_capabilities":[{"AccessType":{"Mount":{"fs_type":"ext4"}},"array_secret":"***stripped***"},{"AccessType":null,"array_secret":"***stripped***"}],"volume_content_source":{"Type":{"Volume":{"oneof_secret_field":"hello","volume_id":"abc"}},"nested_secret_field":"***stripped***"}}`,
		},
	}

	for _, c := range cases {
		before := fmt.Sprint(c.original)
		stripped := StripSecrets(c.original)
		if assert.Equal(t, c.stripped, fmt.Sprintf("%s", stripped), "unexpected result for fmt s of %s", c.original) {
			assert.Equal(t, c.stripped, fmt.Sprintf("%v", stripped), "unexpected result for fmt v of %s", c.original)
		}
		assert.Equal(t, before, fmt.Sprint(c.original), "original value modified")
	}

	// The secret is hidden because StripSecrets is a struct referencing it.
	dump := fmt.Sprintf("%#v", StripSecrets(createVolume))
	assert.NotContains(t, dump, secretName)
	assert.NotContains(t, dump, secretValue)
}
