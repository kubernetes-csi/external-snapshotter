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
		"same key across entries collapses and sorts values": {
			topos: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2c"}},
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
				{Segments: map[string]string{zoneKey: "us-west-2b"}},
			},
			expected: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b", "us-west-2c"}},
				},
			}},
		},
		"duplicate values are de-duplicated": {
			topos: []*csi.Topology{
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
				{Segments: map[string]string{zoneKey: "us-west-2a"}},
			},
			expected: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: zoneKey, Values: []string{"us-west-2a"}},
				},
			}},
		},
		"multi-key segments bucket separately from single-key segments": {
			topos: []*csi.Topology{
				{Segments: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2a"}},
				{Segments: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2b"}},
			},
			// One bucket (keys region|zone). Keys are ordered by first
			// appearance after sorting each segment's keys: region, then zone.
			expected: []v1.TopologySelectorTerm{{
				MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
					{Key: regionKey, Values: []string{"us-west-2"}},
					{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b"}},
				},
			}},
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

// TestTopologyRoundTrip verifies that AllowedTopologies terms survive the
// class -> CSI request -> CSI response -> NodeAffinity round trip unchanged,
// including a multi-expression term whose AND-semantics must
// be preserved through the CSI encoding.
func TestTopologyRoundTrip(t *testing.T) {
	testcases := map[string][]v1.TopologySelectorTerm{
		"single key multiple values": {{
			MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
				{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b", "us-west-2c"}},
			},
		}},
		"region and zone in one term": {{
			MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
				{Key: regionKey, Values: []string{"us-west-2"}},
				{Key: zoneKey, Values: []string{"us-west-2a", "us-west-2b"}},
			},
		}},
	}

	for name, terms := range testcases {
		t.Run(name, func(t *testing.T) {
			roundTripped := csiTopologyToTerms(topologySelectorTermsToCSI(terms))
			if !reflect.DeepEqual(roundTripped, terms) {
				t.Errorf("round trip changed terms: got %v, want %v", roundTripped, terms)
			}
		})
	}
}
