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

import v1 "k8s.io/api/core/v1"

// GetPersistentVolumeFromHandle looks for the PV having a certain CSI driver name
// and corresponding to a volume with a given handle, in a PV List.
// If the PV is not found, returns nil
func GetPersistentVolumeFromHandle(pvList *v1.PersistentVolumeList, driverName, volumeHandle string) *v1.PersistentVolume {
	for i := range pvList.Items {
		if pvList.Items[i].Spec.CSI == nil {
			continue
		}

		if pvList.Items[i].Spec.CSI.Driver == driverName && pvList.Items[i].Spec.CSI.VolumeHandle == volumeHandle {
			return &pvList.Items[i]
		}
	}

	return nil
}
