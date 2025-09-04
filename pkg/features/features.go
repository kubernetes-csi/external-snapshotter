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

package features

import (
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
)

const (
	// Enable usage of volume group snapshot
	VolumeGroupSnapshot featuregate.Feature = "CSIVolumeGroupSnapshot"

	// owner: @rhrmo
	// alpha: v1.34
	//
	// Releases leader election lease on sigterm / sigint.
	ReleaseLeaderElectionOnExit featuregate.Feature = "ReleaseLeaderElectionOnExit"
)

func init() {
	feature.DefaultMutableFeatureGate.Add(defaultKubernetesFeatureGates)
}

// defaultKubernetesFeatureGates consists of all known feature keys specific to external-snapshotter.
// To add a new feature, define a key for it above and add it here.
var defaultKubernetesFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	VolumeGroupSnapshot:         {Default: false, PreRelease: featuregate.Beta},
	ReleaseLeaderElectionOnExit: {Default: false, PreRelease: featuregate.Alpha},
}
