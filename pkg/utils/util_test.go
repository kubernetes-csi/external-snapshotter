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

package utils

import (
	"reflect"
	"testing"

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestContainsString(t *testing.T) {
	src := []string{"aa", "bb", "cc"}
	if !ContainsString(src, "bb") {
		t.Errorf("ContainsString didn't find the string as expected")
	}
}

func TestRemoveString(t *testing.T) {
	tests := []struct {
		testName string
		input    []string
		remove   string
		want     []string
	}{
		{
			testName: "Nil input slice",
			input:    nil,
			remove:   "",
			want:     nil,
		},
		{
			testName: "Slice doesn't contain the string",
			input:    []string{"a", "ab", "cdef"},
			remove:   "NotPresentInSlice",
			want:     []string{"a", "ab", "cdef"},
		},
		{
			testName: "All strings removed, result is nil",
			input:    []string{"a"},
			remove:   "a",
			want:     nil,
		},
		{
			testName: "One string removed",
			input:    []string{"a", "ab", "cdef"},
			remove:   "ab",
			want:     []string{"a", "cdef"},
		},
		{
			testName: "All(three) strings removed",
			input:    []string{"ab", "a", "ab", "cdef", "ab"},
			remove:   "ab",
			want:     []string{"a", "cdef"},
		},
	}
	for _, tt := range tests {
		if got := RemoveString(tt.input, tt.remove); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%v: RemoveString(%v, %q) = %v WANT %v", tt.testName, tt.input, tt.remove, got, tt.want)
		}
	}
}

func TestGetSecretReference(t *testing.T) {
	testcases := map[string]struct {
		secretParams    secretParamsMap
		params          map[string]string
		snapContentName string
		snapshot        *crdv1.VolumeSnapshot
		expectRef       *v1.SecretReference
		expectErr       bool
	}{
		"no params": {
			secretParams: SnapshotterSecretParams,
			params:       nil,
			expectRef:    nil,
		},
		"namespace, no name": {
			secretParams: SnapshotterSecretParams,
			params:       map[string]string{PrefixedSnapshotterSecretNamespaceKey: "foo"},
			expectErr:    true,
		},
		"simple - valid": {
			secretParams: SnapshotterSecretParams,
			params:       map[string]string{PrefixedSnapshotterSecretNameKey: "name", PrefixedSnapshotterSecretNamespaceKey: "ns"},
			snapshot:     &crdv1.VolumeSnapshot{},
			expectRef:    &v1.SecretReference{Name: "name", Namespace: "ns"},
		},
		"simple - invalid name": {
			secretParams: SnapshotterSecretParams,
			params:       map[string]string{PrefixedSnapshotterSecretNameKey: "bad name", PrefixedSnapshotterSecretNamespaceKey: "ns"},
			snapshot:     &crdv1.VolumeSnapshot{},
			expectRef:    nil,
			expectErr:    true,
		},
		"template - invalid": {
			secretParams: SnapshotterSecretParams,
			params: map[string]string{
				PrefixedSnapshotterSecretNameKey:      "static-${volumesnapshotcontent.name}-${volumesnapshot.namespace}-${volumesnapshot.name}-${volumesnapshot.annotations['akey']}",
				PrefixedSnapshotterSecretNamespaceKey: "static-${volumesnapshotcontent.name}-${volumesnapshot.namespace}",
			},
			snapContentName: "snapcontentname",
			snapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "snapshotname",
					Namespace:   "snapshotnamespace",
					Annotations: map[string]string{"akey": "avalue"},
				},
			},
			expectRef: nil,
			expectErr: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			ref, err := GetSecretReference(tc.secretParams, tc.params, tc.snapContentName, tc.snapshot)
			if err != nil {
				if tc.expectErr {
					return
				}
				t.Fatalf("Did not expect error but got: %v", err)

			} else {
				if tc.expectErr {
					t.Fatalf("Expected error but got none")
				}
			}
			if !reflect.DeepEqual(ref, tc.expectRef) {
				t.Errorf("Expected %v, got %v", tc.expectRef, ref)
			}
		})
	}
}

func TestRemovePrefixedCSIParams(t *testing.T) {
	testcases := []struct {
		name           string
		params         map[string]string
		expectedParams map[string]string
		expectErr      bool
	}{
		{
			name:           "no prefix",
			params:         map[string]string{"csiFoo": "bar", "bim": "baz"},
			expectedParams: map[string]string{"csiFoo": "bar", "bim": "baz"},
		},
		{
			name:           "one prefixed",
			params:         map[string]string{PrefixedSnapshotterSecretNameKey: "bar", "bim": "baz"},
			expectedParams: map[string]string{"bim": "baz"},
		},
		{
			name: "all known prefixed",
			params: map[string]string{
				PrefixedSnapshotterSecretNameKey:          "csiBar",
				PrefixedSnapshotterSecretNamespaceKey:     "csiBar",
				PrefixedSnapshotterListSecretNameKey:      "csiBar",
				PrefixedSnapshotterListSecretNamespaceKey: "csiBar",
			},
			expectedParams: map[string]string{},
		},
		{
			name:      "unknown prefixed var",
			params:    map[string]string{csiParameterPrefix + "bim": "baz"},
			expectErr: true,
		},
		{
			name:           "empty",
			params:         map[string]string{},
			expectedParams: map[string]string{},
		},
	}
	for _, tc := range testcases {
		t.Logf("test: %v", tc.name)
		newParams, err := RemovePrefixedParameters(tc.params)
		if err != nil {
			if tc.expectErr {
				continue
			} else {
				t.Fatalf("Encountered unexpected error: %v", err)
			}
		} else {
			if tc.expectErr {
				t.Fatalf("Did not get error when one was expected")
			}
		}
		eq := reflect.DeepEqual(newParams, tc.expectedParams)
		if !eq {
			t.Fatalf("Stripped parameters: %v not equal to expected parameters: %v", newParams, tc.expectedParams)
		}
	}
}
