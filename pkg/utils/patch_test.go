package utils

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPatchOpsBytesToRemoveFinalizers(t *testing.T) {
	type args struct {
		object             *corev1.PersistentVolumeClaim
		finalizersToRemove []string
	}
	tests := []struct {
		name                 string
		args                 args
		finalizersAfterPatch []string
		wantErr              bool
	}{
		{
			name: "remove all finalizers",
			args: args{
				object: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "pvc1",
						Namespace:  "default",
						Finalizers: []string{"finalizer1", "finalizer2"},
					},
				},
				finalizersToRemove: []string{"finalizer1", "finalizer2"},
			},
			finalizersAfterPatch: []string{},
			wantErr:              false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(tt.args.object)
			gotObj, err := client.CoreV1().PersistentVolumeClaims(tt.args.object.GetNamespace()).Get(context.Background(), tt.args.object.GetName(), metav1.GetOptions{})
			if err != nil {
				t.Errorf("PatchOpsBytesToRemoveFinalizers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for i, finalizer := range gotObj.GetFinalizers() {
				if finalizer != tt.args.object.Finalizers[i] {
					t.Errorf("PatchOpsBytesToRemoveFinalizers() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
			}
			got, err := PatchOpsBytesToRemoveFinalizers(tt.args.object, tt.args.finalizersToRemove...)
			if (err != nil) != tt.wantErr {
				t.Errorf("PatchOpsBytesToRemoveFinalizers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// try to apply the patch
			_, err = client.CoreV1().PersistentVolumeClaims(tt.args.object.GetNamespace()).Patch(context.Background(), tt.args.object.GetName(), types.JSONPatchType, got, metav1.PatchOptions{})
			if err != nil {
				t.Errorf("PatchOpsBytesToRemoveFinalizers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// check if the finalizers were removed
			gotObj, err = client.CoreV1().PersistentVolumeClaims(tt.args.object.GetNamespace()).Get(context.Background(), tt.args.object.GetName(), metav1.GetOptions{})
			if err != nil {
				t.Errorf("PatchOpsBytesToRemoveFinalizers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(gotObj.GetFinalizers()) != len(tt.finalizersAfterPatch) {
				t.Errorf("PatchOpsBytesToRemoveFinalizers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for i, finalizer := range gotObj.GetFinalizers() {
				if finalizer != tt.finalizersAfterPatch[i] {
					t.Errorf("PatchOpsBytesToRemoveFinalizers() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
			}
		})
	}
}
