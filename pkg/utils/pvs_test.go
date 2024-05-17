/*
Copyright 2024 The Kubernetes Authors.

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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPersistentVolumeFromHandle(t *testing.T) {
	testDriverName := "hostpath.csi.k8s.io"
	testVolumeHandle := "df39ea9e-1296-11ef-adde-baf37ed30dae"
	testPvName := "pv-name"

	pvListTest := v1.PersistentVolumeList{
		Items: []v1.PersistentVolume{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: testPvName,
				},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeSource: v1.PersistentVolumeSource{
						CSI: &v1.CSIPersistentVolumeSource{
							Driver:       testDriverName,
							VolumeHandle: testVolumeHandle,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pv-no-csi",
				},
				Spec: v1.PersistentVolumeSpec{
					PersistentVolumeSource: v1.PersistentVolumeSource{
						HostPath: &v1.HostPathVolumeSource{},
					},
				},
			},
		},
	}

	tests := []struct {
		testName     string
		driverName   string
		volumeHandle string
		pvList       v1.PersistentVolumeList
		pvName       string
	}{
		{
			testName:     "empty-pv-list",
			driverName:   testDriverName,
			volumeHandle: testVolumeHandle,
			pvName:       "",
		},
		{
			testName:     "pv-in-list",
			driverName:   testDriverName,
			volumeHandle: testVolumeHandle,
			pvList:       pvListTest,
			pvName:       testPvName,
		},
		{
			testName:     "not-existing-volume-handle",
			driverName:   testDriverName,
			volumeHandle: "not-existing-volume-handle",
			pvList:       pvListTest,
			pvName:       "",
		},
		{
			testName:     "invalid-driver-name",
			driverName:   "invalid-driver-name",
			volumeHandle: testVolumeHandle,
			pvList:       pvListTest,
			pvName:       "",
		},
	}
	for _, tt := range tests {
		got := GetPersistentVolumeFromHandle(&tt.pvList, tt.driverName, tt.volumeHandle)
		if got == nil {
			if len(tt.pvName) != 0 {
				t.Errorf("%v: GetPersistentVolumeFromHandle = %v WANT %v", tt.testName, got, tt.pvName)
			}
		} else {
			if tt.pvName != got.Name {
				t.Errorf("%v: GetPersistentVolumeFromHandle = %v WANT %v", tt.testName, got.Name, tt.pvName)
			}
		}
	}
}
