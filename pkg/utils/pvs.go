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
	"fmt"

	v1 "k8s.io/api/core/v1"
)

const CSIDriverHandleIndexName = "ByVolumeHandle"

// PersistentVolumeKeyFunc maps a persistent volume to a string usable
// as KeyFunc to recover it from the CSI driver name and the volume handle.
// If the passed PV is not CSI-based, it will return the empty string
func PersistentVolumeKeyFunc(pv *v1.PersistentVolume) string {
	if pv != nil && pv.Spec.CSI != nil {
		return fmt.Sprintf("%s^%s", pv.Spec.CSI.Driver, pv.Spec.CSI.VolumeHandle)
	}
	return ""
}

// PersistentVolumeKeyFuncByCSIDriverHandle returns the key to be used form
// the individual data components
func PersistentVolumeKeyFuncByCSIDriverHandle(driverName, volumeHandle string) string {
	return fmt.Sprintf("%s^%s", driverName, volumeHandle)
}
