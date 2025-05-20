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

	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func TestIsVolumeSnapshotClassDefaultAnnotation(t *testing.T) {
	testcases := []struct {
		name       string
		objectMeta metav1.ObjectMeta
		isDefault  bool
	}{
		{
			name: "no default annotation in snapshot class",
			objectMeta: metav1.ObjectMeta{
				Annotations: nil,
			},
			isDefault: false,
		},
		{
			name: "with default annotation in snapshot class",
			objectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					IsDefaultSnapshotClassAnnotation: "true",
				},
			},
			isDefault: true,
		},
		{
			name: "with default=false annotation in snapshot class",
			objectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					IsDefaultSnapshotClassAnnotation: "false",
				},
			},
			isDefault: false,
		},
	}
	for _, tc := range testcases {
		t.Logf("test: %s", tc.name)
		isDefault := IsVolumeSnapshotClassDefaultAnnotation(tc.objectMeta)
		if tc.isDefault != isDefault {
			t.Fatalf("default annotation on class incorrectly detected: %v != %v", isDefault, tc.isDefault)
		}
	}
}

func TestIsVolumeGroupSnapshotClassDefaultAnnotation(t *testing.T) {
	testcases := []struct {
		name       string
		objectMeta metav1.ObjectMeta
		isDefault  bool
	}{
		{
			name: "no default annotation in group snapshot class",
			objectMeta: metav1.ObjectMeta{
				Annotations: nil,
			},
			isDefault: false,
		},
		{
			name: "with default annotation in group snapshot class",
			objectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					IsDefaultGroupSnapshotClassAnnotation: "true",
				},
			},
			isDefault: true,
		},
		{
			name: "with default=false annotation in group snapshot class",
			objectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					IsDefaultGroupSnapshotClassAnnotation: "false",
				},
			},
			isDefault: false,
		},
	}
	for _, tc := range testcases {
		t.Logf("test: %s", tc.name)
		isDefault := IsVolumeGroupSnapshotClassDefaultAnnotation(tc.objectMeta)
		if tc.isDefault != isDefault {
			t.Fatalf("default annotation on class incorrectly detected: %v != %v", isDefault, tc.isDefault)
		}
	}
}

func TestShouldEnqueueContentChange(t *testing.T) {
	oldValue := "old"
	newValue := "new"

	testcases := []struct {
		name           string
		old            *crdv1.VolumeSnapshotContent
		new            *crdv1.VolumeSnapshotContent
		expectedResult bool
	}{
		{
			name:           "basic no change",
			old:            &crdv1.VolumeSnapshotContent{},
			new:            &crdv1.VolumeSnapshotContent{},
			expectedResult: false,
		},
		{
			name: "basic change",
			old: &crdv1.VolumeSnapshotContent{
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: &oldValue,
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				Spec: crdv1.VolumeSnapshotContentSpec{
					VolumeSnapshotClassName: &newValue,
				},
			},
			expectedResult: true,
		},
		{
			name: "status change",
			old: &crdv1.VolumeSnapshotContent{
				Status: &crdv1.VolumeSnapshotContentStatus{
					Error: &crdv1.VolumeSnapshotError{
						Message: &oldValue,
					},
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				Status: &crdv1.VolumeSnapshotContentStatus{
					Error: &crdv1.VolumeSnapshotError{
						Message: &newValue,
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "finalizers change",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{
						oldValue,
					},
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{
						newValue,
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "managed fields change",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager: oldValue,
						},
					},
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager: newValue,
						},
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "sidecar-managed annotation change",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnVolumeSnapshotBeingCreated: oldValue,
					},
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnVolumeSnapshotBeingCreated: newValue,
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "sidecar-unmanaged annotation change",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"test-annotation": oldValue,
					},
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"test-annotation": newValue,
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "sidecar-managed annotation created",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnVolumeSnapshotBeingCreated: newValue,
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "sidecar-unmanaged annotation created",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"test-annotation": newValue,
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "sidecar-managed annotation deleted",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnVolumeSnapshotBeingCreated: oldValue,
					},
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			expectedResult: false,
		},
		{
			name: "sidecar-unmanaged annotation deleted",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"test-annotation": oldValue,
					},
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			expectedResult: true,
		},
		{
			name: "sidecar-unmanaged annotation change (AnnVolumeSnapshotBeingDeleted)",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnVolumeSnapshotBeingDeleted: oldValue,
					},
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnVolumeSnapshotBeingDeleted: newValue,
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "resync from informer (old matches new including resource version)",
			old: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: oldValue,
				},
			},
			new: &crdv1.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: oldValue,
				},
			},
			expectedResult: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			// Inject resource version unless it is already set in test object
			if tc.old.ResourceVersion == "" {
				tc.old.ResourceVersion = oldValue
			}
			if tc.new.ResourceVersion == "" {
				tc.old.ResourceVersion = newValue
			}
			result := ShouldEnqueueContentChange(tc.old, tc.new)
			if result != tc.expectedResult {
				t.Fatalf("Incorrect result: Expected %v received %v", tc.expectedResult, result)
			}
		})
	}
}
