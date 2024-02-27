/*
Copyright 2023 The Kubernetes Authors.

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
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumegroupsnapshot/v1alpha1"
	"github.com/kubernetes-csi/external-snapshotter/client/v7/clientset/versioned/fake"
	"github.com/kubernetes-csi/external-snapshotter/v7/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func Test_csiSnapshotCommonController_removeGroupSnapshotFinalizer(t *testing.T) {
	type args struct {
		groupSnapshot        *crdv1alpha1.VolumeGroupSnapshot
		removeBoundFinalizer bool
	}
	tests := []struct {
		name               string
		args               args
		wantErr            bool
		expectedFinalizers []string
	}{
		{
			name: "Test removeGroupSnapshotFinalizer",
			args: args{
				removeBoundFinalizer: true,
				groupSnapshot: &crdv1alpha1.VolumeGroupSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-group-snapshot",
						Finalizers: []string{utils.VolumeGroupSnapshotBoundFinalizer},
					},
				},
			},
		},
		{
			name: "Test removeGroupSnapshotFinalizer and not something else",
			args: args{
				removeBoundFinalizer: true,
				groupSnapshot: &crdv1alpha1.VolumeGroupSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-group-snapshot",
						Finalizers: []string{"somethingElse", utils.VolumeGroupSnapshotBoundFinalizer},
					},
				},
			},
			expectedFinalizers: []string{"somethingElse"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := &csiSnapshotCommonController{
				clientset:          fake.NewSimpleClientset(tt.args.groupSnapshot),
				groupSnapshotStore: cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc),
			}
			if err := ctrl.removeGroupSnapshotFinalizer(tt.args.groupSnapshot, tt.args.removeBoundFinalizer); (err != nil) != tt.wantErr {
				t.Errorf("csiSnapshotCommonController.removeGroupSnapshotFinalizer() error = %v, wantErr %v", err, tt.wantErr)
			}
			vgs, err := ctrl.clientset.GroupsnapshotV1alpha1().VolumeGroupSnapshots(tt.args.groupSnapshot.Namespace).Get(context.TODO(), tt.args.groupSnapshot.Name, metav1.GetOptions{})
			if err != nil {
				t.Errorf("Error getting volume group snapshot: %v", err)
			}
			if tt.expectedFinalizers == nil && vgs.Finalizers != nil {
				tt.expectedFinalizers = []string{} // if expectedFinalizers is nil, then it should be an empty slice so that cmp.Diff does not panic
			}
			if vgs.Finalizers != nil && cmp.Diff(vgs.Finalizers, tt.expectedFinalizers) != "" {
				t.Errorf("Finalizers not expected: %v", cmp.Diff(vgs.Finalizers, tt.expectedFinalizers))
			}

		})
	}
}
