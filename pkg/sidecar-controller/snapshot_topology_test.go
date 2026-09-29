/*
Copyright 2026 The Kubernetes Authors.

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

package sidecar_controller

import (
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	v1 "k8s.io/api/core/v1"
)

const (
	regionKey = "topology.kubernetes.io/region"
	zoneKey   = "topology.kubernetes.io/zone"
)

// TestTopologySelectorTermsToCSI covers the VolumeSnapshotClass.AllowedTopologies
// -> CSI CreateSnapshotRequest.accessibility_requirements conversion.
func TestTopologySelectorTermsToCSI(t *testing.T) {
	testcases := map[string]struct {
		terms    []v1.TopologySelectorTerm
		expected []*csi.Topology
	}{
		"nil terms yields nil": {
			terms:    nil,
			expected: nil,
		},
		"empty terms yields nil": {
			terms:    []v1.TopologySelectorTerm{},
			expected: nil,
		},
		"single key single value": {
			terms: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2a"}},
				},
			}},
			expected: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
			},
		},
		"single key multiple values fans out": {
			terms: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b", "us-west-2c"}},
				},
			}},
			expected: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
				{Segments: map[string]string{zoneKey: "us-west-2b"}},
				{Segments: map[string]string{zoneKey: "us-west-2c"}},
			},
		},
		"multiple expressions in one term fan out as cartesian product": {
			terms: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b"}},
				},
			}},
			expected: []*csi.Topology{
				{Segments: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2a"}},
				{Segments: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2b"}},
			},
		},
		"multiple terms are ORed independently": {
			terms: []v1.TopologySelectorTerm{
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2a"}},
				}},
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2b"}},
				}},
			},
			expected: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
				{Segments: map[string]string{zoneKey: "us-west-2b"}},
			},
		},
		"expression with no values makes the term unsatisfiable": {
			terms: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{}},
				},
			}},
			expected: nil,
		},
		"repeated key with disjoint values is unsatisfiable (ANDed, not ORed)": {
			terms: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2a"}},
					{Key: zoneKey, Values: []string{"us-west-2b"}},
				},
			}},
			expected: nil,
		},
		"repeated key intersects its values": {
			terms: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b"}},
					{Key: zoneKey, Values: []string{"us-west-2b", "us-west-2c"}},
				},
			}},
			expected: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2b"}},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got := topologySelectorTermsToCSI(tc.terms)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("topologySelectorTermsToCSI() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestCSITopologyToTerms covers the CSI CreateSnapshotResponse.accessible_topology
// -> VolumeSnapshotContent.Spec.NodeAffinity conversion.
func TestCSITopologyToTerms(t *testing.T) {
	testcases := map[string]struct {
		topos    []*csi.Topology
		expected []v1.TopologySelectorTerm
	}{
		"nil yields empty": {
			topos:    nil,
			expected: []v1.TopologySelectorTerm{},
		},
		"nil entry and empty segments are skipped": {
			topos: []*csi.Topology{
				nil,
				{Segments: map[string]string{}},
			},
			expected: []v1.TopologySelectorTerm{},
		},
		"single zone": {
			topos: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2b"}},
			},
			expected: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2b"}},
				},
			}},
		},
		"each single-key topology becomes its own term": {
			topos: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
				{Segments: map[string]string{zoneKey: "us-west-2b"}},
				{Segments: map[string]string{zoneKey: "us-west-2c"}},
			},
			expected: []v1.TopologySelectorTerm{
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: []string{"us-west-2a"}}}},
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: []string{"us-west-2b"}}}},
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: []string{"us-west-2c"}}}},
			},
		},
		"duplicate topologies are de-duplicated": {
			topos: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
			},
			expected: []v1.TopologySelectorTerm{
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: []string{"us-west-2a"}}}},
			},
		},
		"each multi-key topology becomes one term with a single value per key": {
			topos: []*csi.Topology{
				{Segments: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2a"}},
				{Segments: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2b"}},
			},
			// Each topology stays its own term (keys sorted within a term).
			expected: []v1.TopologySelectorTerm{
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{"us-west-2a"}},
				}},
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{"us-west-2b"}},
				}},
			},
		},
		"distinct region+zone pairs do not cross-merge (regression)": {
			// {r1,z1} and {r2,z2} must NOT merge into region{r1,r2} zone{z1,z2},
			// which would spuriously match {r2,z1}.
			topos: []*csi.Topology{
				{Segments: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2a"}},
				{Segments: map[string]string{regionKey: "us-east-1", zoneKey: "us-east-1a"}},
			},
			expected: []v1.TopologySelectorTerm{
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{"us-west-2a"}},
				}},
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-east-1"}},
					{Key: zoneKey, Values: []string{"us-east-1a"}},
				}},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got := csiTopologyToTerms(tc.topos)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("csiTopologyToTerms() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestTopologyRoundTrip verifies the class -> CSI request -> CSI response ->
// NodeAffinity round trip.
func TestTopologyRoundTrip(t *testing.T) {
	testcases := map[string]struct {
		terms    []v1.TopologySelectorTerm
		expected []v1.TopologySelectorTerm
	}{
		"single key multiple values expands to one term per value": {
			terms: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b", "us-west-2c"}},
				},
			}},
			expected: []v1.TopologySelectorTerm{
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: []string{"us-west-2a"}}}},
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: []string{"us-west-2b"}}}},
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: []string{"us-west-2c"}}}},
			},
		},
		"region and zone in one term expands to the cartesian product, AND preserved": {
			terms: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b"}},
				},
			}},
			// region is ANDed with each zone; no spurious cross-region pairing.
			expected: []v1.TopologySelectorTerm{
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{"us-west-2a"}},
				}},
				{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{"us-west-2b"}},
				}},
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			roundTripped := csiTopologyToTerms(topologySelectorTermsToCSI(tc.terms))
			if !reflect.DeepEqual(roundTripped, tc.expected) {
				t.Errorf("round trip = %v, want %v", roundTripped, tc.expected)
			}
		})
	}
}
