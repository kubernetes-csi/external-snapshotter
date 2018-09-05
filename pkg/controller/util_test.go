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

package controller

import (
	"fmt"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
)

func TestGetSecretReference(t *testing.T) {
	testcases := map[string]struct {
		params          map[string]string
		snapContentName string
		snapshot        *crdv1.VolumeSnapshot
		expectRef       *v1.SecretReference
		expectErr       error
	}{
		"no params": {
			params:    nil,
			expectRef: nil,
			expectErr: nil,
		},
		"empty err": {
			params:    map[string]string{snapshotterSecretNameKey: "", snapshotterSecretNamespaceKey: ""},
			expectErr: fmt.Errorf("csiSnapshotterSecretName and csiSnapshotterSecretNamespace parameters must be specified together"),
		},
		"name, no namespace": {
			params:    map[string]string{snapshotterSecretNameKey: "foo"},
			expectErr: fmt.Errorf("csiSnapshotterSecretName and csiSnapshotterSecretNamespace parameters must be specified together"),
		},
		"namespace, no name": {
			params:    map[string]string{snapshotterSecretNamespaceKey: "foo"},
			expectErr: fmt.Errorf("csiSnapshotterSecretName and csiSnapshotterSecretNamespace parameters must be specified together"),
		},
		"simple - valid": {
			params:    map[string]string{snapshotterSecretNameKey: "name", snapshotterSecretNamespaceKey: "ns"},
			snapshot:  &crdv1.VolumeSnapshot{},
			expectRef: &v1.SecretReference{Name: "name", Namespace: "ns"},
			expectErr: nil,
		},
		"simple - valid, no pvc": {
			params:    map[string]string{snapshotterSecretNameKey: "name", snapshotterSecretNamespaceKey: "ns"},
			snapshot:  nil,
			expectRef: &v1.SecretReference{Name: "name", Namespace: "ns"},
			expectErr: nil,
		},
		"simple - invalid name": {
			params:    map[string]string{snapshotterSecretNameKey: "bad name", snapshotterSecretNamespaceKey: "ns"},
			snapshot:  &crdv1.VolumeSnapshot{},
			expectRef: nil,
			expectErr: fmt.Errorf(`csiSnapshotterSecretName parameter "bad name" is not a valid secret name`),
		},
		"simple - invalid namespace": {
			params:    map[string]string{snapshotterSecretNameKey: "name", snapshotterSecretNamespaceKey: "bad ns"},
			snapshot:  &crdv1.VolumeSnapshot{},
			expectRef: nil,
			expectErr: fmt.Errorf(`csiSnapshotterSecretNamespace parameter "bad ns" is not a valid namespace name`),
		},
		"template - valid": {
			params: map[string]string{
				snapshotterSecretNameKey:      "static-${volumesnapshotcontent.name}-${volumesnapshot.namespace}-${volumesnapshot.name}-${volumesnapshot.annotations['akey']}",
				snapshotterSecretNamespaceKey: "static-${volumesnapshotcontent.name}-${volumesnapshot.namespace}",
			},
			snapContentName: "snapcontentname",
			snapshot: &crdv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "snapshotname",
					Namespace:   "snapshotnamespace",
					Annotations: map[string]string{"akey": "avalue"},
				},
			},
			expectRef: &v1.SecretReference{Name: "static-snapcontentname-snapshotnamespace-snapshotname-avalue", Namespace: "static-snapcontentname-snapshotnamespace"},
			expectErr: nil,
		},
		"template - invalid namespace tokens": {
			params: map[string]string{
				snapshotterSecretNameKey:      "myname",
				snapshotterSecretNamespaceKey: "mynamespace${bar}",
			},
			snapshot:  &crdv1.VolumeSnapshot{},
			expectRef: nil,
			expectErr: fmt.Errorf(`error resolving csiSnapshotterSecretNamespace value "mynamespace${bar}": invalid tokens: ["bar"]`),
		},
		"template - invalid name tokens": {
			params: map[string]string{
				snapshotterSecretNameKey:      "myname${foo}",
				snapshotterSecretNamespaceKey: "mynamespace",
			},
			snapshot:  &crdv1.VolumeSnapshot{},
			expectRef: nil,
			expectErr: fmt.Errorf(`error resolving csiSnapshotterSecretName value "myname${foo}": invalid tokens: ["foo"]`),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			ref, err := GetSecretReference(tc.params, tc.snapContentName, tc.snapshot)
			if !reflect.DeepEqual(err, tc.expectErr) {
				t.Errorf("Expected %v, got %v", tc.expectErr, err)
			}
			if !reflect.DeepEqual(ref, tc.expectRef) {
				t.Errorf("Expected %v, got %v", tc.expectRef, ref)
			}
		})
	}
}
