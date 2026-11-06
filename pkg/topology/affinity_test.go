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

package topology

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
)

const (
	regionKey = "topology.kubernetes.io/region"
	zoneKey   = "topology.kubernetes.io/zone"
	rackKey   = "example.com/rack"
)

func zoneTerm(values ...string) v1.TopologySelectorTerm {
	return v1.TopologySelectorTerm{
		MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: values}},
	}
}

func regionZoneTerm(region, zone string) v1.TopologySelectorTerm {
	return v1.TopologySelectorTerm{
		MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
			{Key: regionKey, Values: []string{region}},
			{Key: zoneKey, Values: []string{zone}},
		},
	}
}

func req(key string, op v1.NodeSelectorOperator, values ...string) v1.NodeSelectorRequirement {
	return v1.NodeSelectorRequirement{Key: key, Operator: op, Values: values}
}

func requiredAffinity(terms ...v1.NodeSelectorTerm) *v1.NodeAffinity {
	return &v1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: terms},
	}
}

// Ported from scheduler-plugin TestNodeMatchesTopology.
func TestNodeMatches(t *testing.T) {
	testcases := map[string]struct {
		nodeLabels map[string]string
		terms      []v1.TopologySelectorTerm
		want       bool
	}{
		"no terms matches nothing": {
			nodeLabels: map[string]string{zoneKey: "us-west-2a"},
			want:       false,
		},
		"node zone in the term values matches": {
			nodeLabels: map[string]string{zoneKey: "us-west-2a"},
			terms:      []v1.TopologySelectorTerm{zoneTerm("us-west-2a", "us-west-2b")},
			want:       true,
		},
		"node zone not in the term values does not match": {
			nodeLabels: map[string]string{zoneKey: "us-west-2c"},
			terms:      []v1.TopologySelectorTerm{zoneTerm("us-west-2a", "us-west-2b")},
			want:       false,
		},
		"missing label key on node does not match": {
			nodeLabels: map[string]string{regionKey: "us-west-2"},
			terms:      []v1.TopologySelectorTerm{zoneTerm("us-west-2a")},
			want:       false,
		},
		"multiple expressions in a term are ANDed - all satisfied": {
			nodeLabels: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2a"},
			terms:      []v1.TopologySelectorTerm{regionZoneTerm("us-west-2", "us-west-2a")},
			want:       true,
		},
		"multiple expressions in a term are ANDed - one unsatisfied": {
			nodeLabels: map[string]string{regionKey: "us-west-2", zoneKey: "us-west-2c"},
			terms:      []v1.TopologySelectorTerm{regionZoneTerm("us-west-2", "us-west-2a")},
			want:       false,
		},
		"multiple terms are ORed - second term matches": {
			nodeLabels: map[string]string{zoneKey: "us-west-2c"},
			terms:      []v1.TopologySelectorTerm{zoneTerm("us-west-2a"), zoneTerm("us-west-2c")},
			want:       true,
		},
		"multiple terms are ORed - none match": {
			nodeLabels: map[string]string{zoneKey: "us-west-2d"},
			terms:      []v1.TopologySelectorTerm{zoneTerm("us-west-2a"), zoneTerm("us-west-2c")},
			want:       false,
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			if got := NodeMatches(tc.nodeLabels, tc.terms); got != tc.want {
				t.Errorf("NodeMatches() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Ported from scheduler-plugin TestFilterMultiPVC.
func TestMultiPVCSemantics(t *testing.T) {
	testcases := map[string]struct {
		termSets  [][]v1.TopologySelectorTerm
		nodeZone  string
		wantMatch bool
	}{
		"single PVC, node in zone": {
			termSets:  [][]v1.TopologySelectorTerm{{zoneTerm("us-west-2a")}},
			nodeZone:  "us-west-2a",
			wantMatch: true,
		},
		"single PVC, node in other zone": {
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("us-west-2a")}},
			nodeZone: "us-west-2b",
		},
		"two PVCs in different zones, node matches only one": {
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("us-west-2a")}, {zoneTerm("us-west-2b")}},
			nodeZone: "us-west-2a",
		},
		"two PVCs in the same zone": {
			termSets:  [][]v1.TopologySelectorTerm{{zoneTerm("us-west-2a")}, {zoneTerm("us-west-2a")}},
			nodeZone:  "us-west-2a",
			wantMatch: true,
		},
		"one PVC allows two zones, other pins the shared one": {
			termSets:  [][]v1.TopologySelectorTerm{{zoneTerm("us-west-2a", "us-west-2b")}, {zoneTerm("us-west-2b")}},
			nodeZone:  "us-west-2b",
			wantMatch: true,
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{zoneKey: tc.nodeZone}}}
			for mergeName, merge := range map[string]func(*v1.NodeAffinity, [][]v1.TopologySelectorTerm, int) (*v1.NodeAffinity, error){
				"MergeAtCreate": MergeAtCreate,
				"MergeGated":    MergeGated,
			} {
				na, err := merge(nil, tc.termSets, 32)
				if err != nil {
					t.Fatalf("%s() error: %v", mergeName, err)
				}
				if got := matches(t, na, node); got != tc.wantMatch {
					t.Errorf("%s(): node match = %v, want %v", mergeName, got, tc.wantMatch)
				}
			}
		})
	}
}

func matches(t *testing.T, na *v1.NodeAffinity, node *v1.Node) bool {
	t.Helper()
	return matcher(t, na)(node)
}

func matcher(t *testing.T, na *v1.NodeAffinity) func(*v1.Node) bool {
	t.Helper()
	pod := &v1.Pod{Spec: v1.PodSpec{Affinity: &v1.Affinity{NodeAffinity: na}}}
	required := nodeaffinity.GetRequiredNodeAffinity(pod)
	return func(node *v1.Node) bool {
		ok, err := required.Match(node)
		if err != nil {
			t.Fatalf("matching node affinity: %v", err)
		}
		return ok
	}
}

func TestCollapse(t *testing.T) {
	testcases := map[string]struct {
		terms []v1.TopologySelectorTerm
		want  []conjunction
	}{
		"single key terms are unioned": {
			terms: []v1.TopologySelectorTerm{zoneTerm("b"), zoneTerm("a"), zoneTerm("c", "a")},
			want:  []conjunction{{zoneKey: {"a", "b", "c"}}},
		},
		"multi-key terms differing in one key are unioned": {
			terms: []v1.TopologySelectorTerm{regionZoneTerm("r1", "z1"), regionZoneTerm("r1", "z2")},
			want:  []conjunction{{regionKey: {"r1"}, zoneKey: {"z1", "z2"}}},
		},
		"multi-key terms differing in two keys are kept apart": {
			terms: []v1.TopologySelectorTerm{regionZoneTerm("r1", "z1"), regionZoneTerm("r2", "z2")},
			want: []conjunction{
				{regionKey: {"r1"}, zoneKey: {"z1"}},
				{regionKey: {"r2"}, zoneKey: {"z2"}},
			},
		},
		"terms with different keys are kept apart": {
			terms: []v1.TopologySelectorTerm{zoneTerm("z1"), regionZoneTerm("r1", "z2")},
			want: []conjunction{
				{regionKey: {"r1"}, zoneKey: {"z2"}},
				{zoneKey: {"z1"}},
			},
		},
		"identical terms are deduplicated": {
			terms: []v1.TopologySelectorTerm{regionZoneTerm("r1", "z1"), regionZoneTerm("r1", "z1")},
			want:  []conjunction{{regionKey: {"r1"}, zoneKey: {"z1"}}},
		},
		"repeated key within a term intersects values": {
			terms: []v1.TopologySelectorTerm{{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
				{Key: zoneKey, Values: []string{"a", "b"}},
				{Key: zoneKey, Values: []string{"b", "c"}},
			}}},
			want: []conjunction{{zoneKey: {"b"}}},
		},
		"term without values is dropped": {
			terms: []v1.TopologySelectorTerm{zoneTerm(), zoneTerm("a")},
			want:  []conjunction{{zoneKey: {"a"}}},
		},
		// An empty term matches no nodes, not every node.
		"term without expressions is dropped": {
			terms: []v1.TopologySelectorTerm{{}, zoneTerm("a")},
			want:  []conjunction{{zoneKey: {"a"}}},
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got, err := normalizeSet(tc.terms, &mergeBudget{remaining: maxAffinityWork})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("normalizeSet() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCollapseDoesNotWidenMultiKeyTerms(t *testing.T) {
	set := [][]v1.TopologySelectorTerm{{regionZoneTerm("r1", "z1"), regionZoneTerm("r2", "z2")}}
	na, err := MergeAtCreate(nil, set, 32)
	if err != nil {
		t.Fatalf("MergeAtCreate() error: %v", err)
	}
	crossed := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", Labels: map[string]string{regionKey: "r1", zoneKey: "z2"}}}
	if matches(t, na, crossed) {
		t.Errorf("merged affinity admits {r1, z2}: %+v", na)
	}
}

func TestMerge(t *testing.T) {
	preferred := []v1.PreferredSchedulingTerm{{Weight: 1, Preference: v1.NodeSelectorTerm{
		MatchExpressions: []v1.NodeSelectorRequirement{req("disk", v1.NodeSelectorOpIn, "ssd")},
	}}}
	testcases := map[string]struct {
		existing   *v1.NodeAffinity
		termSets   [][]v1.TopologySelectorTerm
		wantCreate *v1.NodeAffinity
		wantGated  *v1.NodeAffinity
		gatedErr   error
	}{
		"no term sets leaves affinity unchanged": {
			existing:   requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists)}}),
			wantCreate: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists)}}),
			wantGated:  requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists)}}),
		},
		"no existing affinity": {
			termSets:   [][]v1.TopologySelectorTerm{{zoneTerm("a")}},
			wantCreate: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req(zoneKey, v1.NodeSelectorOpIn, "a")}}),
			wantGated:  requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req(zoneKey, v1.NodeSelectorOpIn, "a")}}),
		},
		"preferred-only affinity is kept": {
			existing: &v1.NodeAffinity{PreferredDuringSchedulingIgnoredDuringExecution: preferred},
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("a")}},
			wantCreate: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{
					{MatchExpressions: []v1.NodeSelectorRequirement{req(zoneKey, v1.NodeSelectorOpIn, "a")}},
				}},
				PreferredDuringSchedulingIgnoredDuringExecution: preferred,
			},
			wantGated: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{
					{MatchExpressions: []v1.NodeSelectorRequirement{req(zoneKey, v1.NodeSelectorOpIn, "a")}},
				}},
				PreferredDuringSchedulingIgnoredDuringExecution: preferred,
			},
		},
		"existing terms get the topology appended": {
			existing: requiredAffinity(
				v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists)}},
				v1.NodeSelectorTerm{MatchFields: []v1.NodeSelectorRequirement{req("metadata.name", v1.NodeSelectorOpIn, "n1")}},
			),
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("a"), zoneTerm("b")}},
			wantCreate: requiredAffinity(
				v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists), req(zoneKey, v1.NodeSelectorOpIn, "a", "b")}},
				v1.NodeSelectorTerm{
					MatchExpressions: []v1.NodeSelectorRequirement{req(zoneKey, v1.NodeSelectorOpIn, "a", "b")},
					MatchFields:      []v1.NodeSelectorRequirement{req("metadata.name", v1.NodeSelectorOpIn, "n1")},
				},
			),
			wantGated: requiredAffinity(
				v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists), req(zoneKey, v1.NodeSelectorOpIn, "a", "b")}},
				v1.NodeSelectorTerm{
					MatchExpressions: []v1.NodeSelectorRequirement{req(zoneKey, v1.NodeSelectorOpIn, "a", "b")},
					MatchFields:      []v1.NodeSelectorRequirement{req("metadata.name", v1.NodeSelectorOpIn, "n1")},
				},
			),
		},
		"existing empty term is kept unchanged": {
			existing: requiredAffinity(
				v1.NodeSelectorTerm{},
				v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists)}},
			),
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("a")}},
			wantCreate: requiredAffinity(
				v1.NodeSelectorTerm{},
				v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists), req(zoneKey, v1.NodeSelectorOpIn, "a")}},
			),
			wantGated: requiredAffinity(
				v1.NodeSelectorTerm{},
				v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists), req(zoneKey, v1.NodeSelectorOpIn, "a")}},
			),
		},
		"multi-key topology with existing terms is a cross-product at creation": {
			existing: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists)}}),
			termSets: [][]v1.TopologySelectorTerm{{regionZoneTerm("r1", "z1"), regionZoneTerm("r2", "z2")}},
			wantCreate: requiredAffinity(
				v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
					req("a", v1.NodeSelectorOpExists), req(regionKey, v1.NodeSelectorOpIn, "r1"), req(zoneKey, v1.NodeSelectorOpIn, "z1"),
				}},
				v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
					req("a", v1.NodeSelectorOpExists), req(regionKey, v1.NodeSelectorOpIn, "r2"), req(zoneKey, v1.NodeSelectorOpIn, "z2"),
				}},
			),
			gatedErr: ErrUnrepresentable,
		},
		"two PVCs without existing affinity drop contradictory combinations": {
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("a"), regionZoneTerm("r", "b")}, {regionZoneTerm("r", "a")}},
			wantCreate: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
				req(zoneKey, v1.NodeSelectorOpIn, "a"), req(regionKey, v1.NodeSelectorOpIn, "r"),
			}}),
			wantGated: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
				req(zoneKey, v1.NodeSelectorOpIn, "a"), req(regionKey, v1.NodeSelectorOpIn, "r"),
			}}),
		},
		"only contradictory combinations keep one that matches no node": {
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("a")}, {zoneTerm("b")}},
			wantCreate: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
				req(zoneKey, v1.NodeSelectorOpIn, "a"), req(zoneKey, v1.NodeSelectorOpIn, "b"),
			}}),
			wantGated: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
				req(zoneKey, v1.NodeSelectorOpIn, "a"), req(zoneKey, v1.NodeSelectorOpIn, "b"),
			}}),
		},
		"term without expressions matches no node and is dropped": {
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("a"), {}}},
			wantCreate: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
				req(zoneKey, v1.NodeSelectorOpIn, "a"),
			}}),
			wantGated: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
				req(zoneKey, v1.NodeSelectorOpIn, "a"),
			}}),
		},
		"empty term set imposes no constraint": {
			termSets: [][]v1.TopologySelectorTerm{{}},
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			got, err := MergeAtCreate(tc.existing, tc.termSets, 32)
			if err != nil {
				t.Fatalf("MergeAtCreate() error: %v", err)
			}
			if !apiequality.Semantic.DeepEqual(got, tc.wantCreate) {
				t.Errorf("MergeAtCreate() =\n%+v\nwant\n%+v", got, tc.wantCreate)
			}

			got, err = MergeGated(tc.existing, tc.termSets, 32)
			if !errors.Is(err, tc.gatedErr) {
				t.Fatalf("MergeGated() error = %v, want %v", err, tc.gatedErr)
			}
			if tc.gatedErr == nil && !apiequality.Semantic.DeepEqual(got, tc.wantGated) {
				t.Errorf("MergeGated() =\n%+v\nwant\n%+v", got, tc.wantGated)
			}
		})
	}
}

func TestMergeUnsatisfiable(t *testing.T) {
	for name, sets := range map[string][][]v1.TopologySelectorTerm{
		"term without values":      {{zoneTerm()}},
		"term without expressions": {{{}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := MergeAtCreate(nil, sets, 32); !errors.Is(err, ErrUnsatisfiable) {
				t.Errorf("MergeAtCreate() error = %v, want %v", err, ErrUnsatisfiable)
			}
			if _, err := MergeGated(nil, sets, 32); !errors.Is(err, ErrUnsatisfiable) {
				t.Errorf("MergeGated() error = %v, want %v", err, ErrUnsatisfiable)
			}
		})
	}
}

func TestMergeTermCap(t *testing.T) {
	rackRow := func(rack, row string) v1.TopologySelectorTerm {
		return v1.TopologySelectorTerm{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
			{Key: rackKey, Values: []string{rack}},
			{Key: "example.com/row", Values: []string{row}},
		}}
	}
	sets := [][]v1.TopologySelectorTerm{
		{regionZoneTerm("r1", "z1"), regionZoneTerm("r2", "z2")},
		{rackRow("k1", "w1"), rackRow("k2", "w2")},
	}
	existing := requiredAffinity(
		v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("a", v1.NodeSelectorOpExists)}},
		v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{req("b", v1.NodeSelectorOpExists)}},
	)
	testcases := map[string]struct {
		existing *v1.NodeAffinity
		max      int
		wantErr  error
	}{
		"at the cap":                      {max: 4},
		"over the cap":                    {max: 3, wantErr: ErrTooManyTerms},
		"existing terms multiply, at cap": {existing: existing, max: 8},
		"existing terms multiply, over":   {existing: existing, max: 7, wantErr: ErrTooManyTerms},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			na, err := MergeAtCreate(tc.existing, sets, tc.max)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("MergeAtCreate() error = %v, want %v", err, tc.wantErr)
			}
			if err == nil && len(requiredTerms(na)) > tc.max {
				t.Errorf("MergeAtCreate() produced %d terms, cap %d", len(requiredTerms(na)), tc.max)
			}
			if tc.existing == nil {
				if _, err := MergeGated(nil, sets, tc.max); !errors.Is(err, tc.wantErr) {
					t.Errorf("MergeGated() error = %v, want %v", err, tc.wantErr)
				}
			}
		})
	}

	if _, err := MergeGated(existing, [][]v1.TopologySelectorTerm{{zoneTerm("a")}}, 1); !errors.Is(err, ErrTooManyTerms) {
		t.Errorf("MergeGated() with 2 existing terms and cap 1: error = %v, want %v", err, ErrTooManyTerms)
	}
}

func TestMergeWorkBudget(t *testing.T) {
	oversized := strings.Repeat("a", maxAffinityWork)
	largeExisting := requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
		req(zoneKey, v1.NodeSelectorOpIn, oversized),
	}})
	var correlated []v1.TopologySelectorTerm
	var growing [][]v1.TopologySelectorTerm
	for i := 0; i < 256; i++ {
		correlated = append(correlated, regionZoneTerm(fmt.Sprintf("r%d", i), fmt.Sprintf("z%d", i)))
		growing = append(growing, []v1.TopologySelectorTerm{{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{
			Key: fmt.Sprintf("example.com/key-%d", i), Values: []string{"a"},
		}}}})
	}

	testcases := map[string]struct {
		existing *v1.NodeAffinity
		termSets [][]v1.TopologySelectorTerm
		wantErr  error
	}{
		"oversized topology input": {
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm(oversized)}},
			wantErr:  ErrTooComplex,
		},
		"oversized existing affinity with topology": {
			existing: largeExisting,
			termSets: [][]v1.TopologySelectorTerm{{zoneTerm("a")}},
			wantErr:  ErrTooComplex,
		},
		// Collapsing these costs work linear in their number, so they fit in
		// the budget; they cannot collapse, so the term cap rejects them.
		"correlated terms that cannot collapse exceed the term cap": {
			termSets: [][]v1.TopologySelectorTerm{correlated},
			wantErr:  ErrTooManyTerms,
		},
		"growing implication comparisons exceed budget": {
			existing: requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
				req("disk", v1.NodeSelectorOpExists),
			}}),
			termSets: growing,
			wantErr:  ErrTooComplex,
		},
		"no topology leaves oversized user affinity alone": {
			existing: largeExisting,
		},
		"empty term set leaves oversized user affinity alone": {
			existing: largeExisting,
			termSets: [][]v1.TopologySelectorTerm{{}},
		},
	}
	for name, merge := range map[string]func(*v1.NodeAffinity, [][]v1.TopologySelectorTerm, int) (*v1.NodeAffinity, error){
		"MergeAtCreate": MergeAtCreate,
		"MergeGated":    MergeGated,
	} {
		t.Run(name, func(t *testing.T) {
			for name, tc := range testcases {
				t.Run(name, func(t *testing.T) {
					got, err := merge(tc.existing, tc.termSets, 32)
					if !errors.Is(err, tc.wantErr) {
						t.Fatalf("error = %v, want %v", err, tc.wantErr)
					}
					if tc.wantErr != nil && got != nil {
						t.Errorf("failed merge returned partial affinity: %+v", got)
					}
					if tc.wantErr == nil && !apiequality.Semantic.DeepEqual(got, tc.existing) {
						t.Error("no-op merge changed the user's affinity")
					}
				})
			}
		})
	}
}

func TestMergeAtCreateBoundsAttemptedWork(t *testing.T) {
	terms := make([]v1.NodeSelectorTerm, 8192)
	for i := range terms {
		terms[i] = v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
			req(zoneKey, v1.NodeSelectorOpIn, "a"),
		}}
	}
	existing := requiredAffinity(terms...)
	// Output fits the cap, but duplicate and contradictory attempts still cost work.
	for name, zone := range map[string]string{"duplicate results": "a", "contradictory results": "b"} {
		t.Run(name, func(t *testing.T) {
			got, err := MergeAtCreate(existing, [][]v1.TopologySelectorTerm{{zoneTerm(zone)}}, 1)
			if !errors.Is(err, ErrTooComplex) {
				t.Fatalf("error = %v, want %v", err, ErrTooComplex)
			}
			if got != nil {
				t.Error("failed merge returned partial affinity")
			}
		})
	}
}

func TestMergeDeduplicates(t *testing.T) {
	sets := [][]v1.TopologySelectorTerm{{zoneTerm("a", "b")}, {regionZoneTerm("r", "a")}}
	for name, merge := range map[string]func(*v1.NodeAffinity, [][]v1.TopologySelectorTerm, int) (*v1.NodeAffinity, error){
		"MergeAtCreate": MergeAtCreate,
		"MergeGated":    MergeGated,
	} {
		t.Run(name, func(t *testing.T) {
			once, err := merge(nil, sets, 32)
			if err != nil {
				t.Fatalf("first merge: %v", err)
			}
			twice, err := merge(once, sets, 32)
			if err != nil {
				t.Fatalf("second merge: %v", err)
			}
			if !apiequality.Semantic.DeepEqual(once, twice) {
				t.Errorf("merging twice changed the affinity:\n%+v\n%+v", once, twice)
			}
		})
	}
}

var (
	regions = []string{"r1", "r2"}
	zones   = []string{"z1", "z2", "z3"}
	racks   = []string{"k1", "k2"}
)

func randomValues(r *rand.Rand, domain []string) []string {
	var out []string
	for _, v := range domain {
		if r.Intn(2) == 0 {
			out = append(out, v)
		}
	}
	if len(out) == 0 && r.Intn(4) != 0 {
		out = []string{domain[r.Intn(len(domain))]}
	}
	return out
}

func randomTopologyTerm(r *rand.Rand) v1.TopologySelectorTerm {
	var term v1.TopologySelectorTerm
	switch r.Intn(3) {
	case 0:
		term.MatchLabelExpressions = append(term.MatchLabelExpressions, v1.TopologySelectorLabelRequirement{Key: zoneKey, Values: randomValues(r, zones)})
	case 1:
		term.MatchLabelExpressions = append(term.MatchLabelExpressions,
			v1.TopologySelectorLabelRequirement{Key: regionKey, Values: []string{regions[r.Intn(len(regions))]}},
			v1.TopologySelectorLabelRequirement{Key: zoneKey, Values: []string{zones[r.Intn(len(zones))]}})
	default:
		term.MatchLabelExpressions = append(term.MatchLabelExpressions,
			v1.TopologySelectorLabelRequirement{Key: regionKey, Values: randomValues(r, regions)},
			v1.TopologySelectorLabelRequirement{Key: zoneKey, Values: randomValues(r, zones)})
	}
	return term
}

func randomTermSets(r *rand.Rand) [][]v1.TopologySelectorTerm {
	sets := make([][]v1.TopologySelectorTerm, 1+r.Intn(3))
	for i := range sets {
		n := 1 + r.Intn(3)
		for j := 0; j < n; j++ {
			sets[i] = append(sets[i], randomTopologyTerm(r))
		}
	}
	return sets
}

func randomExistingAffinity(r *rand.Rand) *v1.NodeAffinity {
	if r.Intn(3) == 0 {
		return nil
	}
	ops := []v1.NodeSelectorOperator{v1.NodeSelectorOpIn, v1.NodeSelectorOpNotIn, v1.NodeSelectorOpExists}
	keys := map[string][]string{zoneKey: zones, regionKey: regions, rackKey: racks}
	keyNames := []string{zoneKey, regionKey, rackKey}
	terms := make([]v1.NodeSelectorTerm, 1+r.Intn(3))
	for i := range terms {
		for n := r.Intn(3); n > 0; n-- {
			key := keyNames[r.Intn(len(keyNames))]
			op := ops[r.Intn(len(ops))]
			e := v1.NodeSelectorRequirement{Key: key, Operator: op}
			if op != v1.NodeSelectorOpExists {
				e.Values = []string{keys[key][r.Intn(len(keys[key]))]}
			}
			terms[i].MatchExpressions = append(terms[i].MatchExpressions, e)
		}
		if r.Intn(4) == 0 {
			terms[i].MatchFields = []v1.NodeSelectorRequirement{req("metadata.name", v1.NodeSelectorOpIn, "n1")}
		}
	}
	return requiredAffinity(terms...)
}

// Enumerate the generator's full label domain, including missing labels.
func allNodes() []*v1.Node {
	var nodes []*v1.Node
	for _, z := range append([]string{""}, zones...) {
		for _, rg := range append([]string{""}, regions...) {
			for _, k := range append([]string{""}, racks...) {
				for _, name := range []string{"n1", "n2"} {
					labels := map[string]string{}
					for key, v := range map[string]string{zoneKey: z, regionKey: rg, rackKey: k} {
						if v != "" {
							labels[key] = v
						}
					}
					nodes = append(nodes, &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}})
				}
			}
		}
	}
	return nodes
}

// validGatedMutation mirrors the API server's validateNodeAffinityMutation.
func validGatedMutation(oldNA, newNA *v1.NodeAffinity) bool {
	oldTerms := requiredTerms(oldNA)
	if len(oldTerms) == 0 {
		return true
	}
	newTerms := requiredTerms(newNA)
	if len(oldTerms) != len(newTerms) {
		return false
	}
	for i := range oldTerms {
		o, n := oldTerms[i], newTerms[i]
		if isEmptyTerm(o) && !isEmptyTerm(n) {
			return false
		}
		if len(n.MatchExpressions) < len(o.MatchExpressions) ||
			!apiequality.Semantic.DeepEqual(n.MatchExpressions[:len(o.MatchExpressions)], o.MatchExpressions) {
			return false
		}
		if len(n.MatchFields) < len(o.MatchFields) ||
			!apiequality.Semantic.DeepEqual(n.MatchFields[:len(o.MatchFields)], o.MatchFields) {
			return false
		}
	}
	return true
}

func validRequirements(na *v1.NodeAffinity) bool {
	for _, t := range requiredTerms(na) {
		for _, e := range t.MatchExpressions {
			if e.Operator == v1.NodeSelectorOpIn && len(e.Values) == 0 {
				return false
			}
		}
	}
	return true
}

func satisfiesAll(n *v1.Node, sets [][]v1.TopologySelectorTerm) bool {
	for _, set := range sets {
		if !NodeMatches(n.Labels, set) {
			return false
		}
	}
	return true
}

// Merged affinity must match existing affinity AND every topology set.
func TestMergeParity(t *testing.T) {
	r := rand.New(rand.NewSource(5943))
	nodes := allNodes()
	merges := map[string]func(*v1.NodeAffinity, [][]v1.TopologySelectorTerm, int) (*v1.NodeAffinity, error){
		"MergeAtCreate": MergeAtCreate,
		"MergeGated":    MergeGated,
	}
	counts := map[string]int{}
	for i := 0; i < 2000; i++ {
		existing := randomExistingAffinity(r)
		sets := randomTermSets(r)
		want := make([]bool, len(nodes))
		existingMatches := matcher(t, existing)
		for j, n := range nodes {
			want[j] = existingMatches(n) && satisfiesAll(n, sets)
		}
		for name, merge := range merges {
			merged, err := merge(existing, sets, 1000)
			switch {
			case errors.Is(err, ErrUnrepresentable):
				if name != "MergeGated" || len(requiredTerms(existing)) == 0 {
					t.Fatalf("%s returned ErrUnrepresentable without existing gated terms", name)
				}
				counts[name+" unrepresentable"]++
				continue
			case errors.Is(err, ErrUnsatisfiable):
				counts[name+" unsatisfiable"]++
				for _, n := range nodes {
					if satisfiesAll(n, sets) {
						t.Fatalf("%s: ErrUnsatisfiable but node %v satisfies %v", name, n.Labels, sets)
					}
				}
				continue
			case err != nil:
				t.Fatalf("%s: unexpected error: %v", name, err)
			}
			counts[name+" merged"]++
			if !validRequirements(merged) {
				t.Fatalf("%s produced an In requirement without values: %+v", name, merged)
			}
			if name == "MergeGated" && !validGatedMutation(existing, merged) {
				t.Fatalf("MergeGated produced a mutation the API server rejects:\nold %+v\nnew %+v", existing, merged)
			}
			mergedMatches := matcher(t, merged)
			for j, n := range nodes {
				if got := mergedMatches(n); got != want[j] {
					t.Fatalf("%s: node %s %v: merged match = %v, want %v\nexisting %+v\nsets %+v\nmerged %+v",
						name, n.Name, n.Labels, got, want[j], existing, sets, merged)
				}
			}
		}
	}
	for _, key := range []string{"MergeAtCreate merged", "MergeGated merged", "MergeGated unrepresentable"} {
		if counts[key] == 0 {
			t.Errorf("generator never produced case %q; counts %v", key, counts)
		}
	}
}

func TestMergeAgainKeepsTermCount(t *testing.T) {
	var set []v1.TopologySelectorTerm
	for i := 1; i <= 6; i++ {
		set = append(set, regionZoneTerm(fmt.Sprintf("r%d", i), fmt.Sprintf("z%d", i)))
	}
	sets := [][]v1.TopologySelectorTerm{set}
	once, err := MergeAtCreate(nil, sets, 32)
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	twice, err := MergeAtCreate(once, sets, 32)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if n := len(requiredTerms(twice)); n != 6 {
		t.Errorf("second merge produced %d terms, want 6", n)
	}
}

func TestMergeIdenticalSets(t *testing.T) {
	set := []v1.TopologySelectorTerm{regionZoneTerm("r1", "z1"), regionZoneTerm("r2", "z2")}
	var sets [][]v1.TopologySelectorTerm
	for i := 0; i < 11; i++ {
		sets = append(sets, set)
	}
	for name, merge := range map[string]func(*v1.NodeAffinity, [][]v1.TopologySelectorTerm, int) (*v1.NodeAffinity, error){
		"MergeAtCreate": MergeAtCreate,
		"MergeGated":    MergeGated,
	} {
		t.Run(name, func(t *testing.T) {
			na, err := merge(nil, sets, 32)
			if err != nil {
				t.Fatalf("%s() error: %v", name, err)
			}
			if n := len(requiredTerms(na)); n != 2 {
				t.Errorf("%s() produced %d terms, want 2", name, n)
			}
		})
	}
}

func TestMergePrunesEachStage(t *testing.T) {
	// Only the shared region survives; the unpruned product has 2^8 terms.
	zones := []string{"z1", "z2", "z3"}
	var sets [][]v1.TopologySelectorTerm
	for i := 0; i < 8; i++ {
		var set []v1.TopologySelectorTerm
		for j := range zones {
			set = append(set, regionZoneTerm("r", zones[(i+j)%len(zones)]))
		}
		// Distinct alternatives prevent deduplication before the cross-product.
		set = append(set, regionZoneTerm(fmt.Sprintf("other%d", i), "z9"))
		sets = append(sets, set)
	}
	na, err := MergeAtCreate(nil, sets, 4)
	if err != nil {
		t.Fatalf("MergeAtCreate() error: %v", err)
	}
	if n := len(requiredTerms(na)); n > 4 {
		t.Errorf("MergeAtCreate() produced %d terms, cap 4", n)
	}
}

func gridTerms(zones, racks int, skip func(z, r int) bool) []v1.TopologySelectorTerm {
	var terms []v1.TopologySelectorTerm
	for z := 0; z < zones; z++ {
		for r := 0; r < racks; r++ {
			if skip != nil && skip(z, r) {
				continue
			}
			terms = append(terms, v1.TopologySelectorTerm{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
				{Key: zoneKey, Values: []string{fmt.Sprintf("zone-%d", z)}},
				{Key: rackKey, Values: []string{fmt.Sprintf("rack-%d", r)}},
			}})
		}
	}
	return terms
}

// TestMergeGrid covers a content whose topology is every zone x rack pair, as
// a driver listing each accessible segment would produce. It must collapse
// into a single term rather than exhaust the work budget.
func TestMergeGrid(t *testing.T) {
	for _, size := range [][2]int{{20, 10}, {10, 20}, {20, 20}} {
		zones, racks := size[0], size[1]
		var wantZones, wantRacks []string
		for z := 0; z < zones; z++ {
			wantZones = append(wantZones, fmt.Sprintf("zone-%d", z))
		}
		for r := 0; r < racks; r++ {
			wantRacks = append(wantRacks, fmt.Sprintf("rack-%d", r))
		}
		// Values are sorted as strings, so zone-10 comes before zone-2.
		slices.Sort(wantZones)
		slices.Sort(wantRacks)
		want := requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
			req(rackKey, v1.NodeSelectorOpIn, wantRacks...),
			req(zoneKey, v1.NodeSelectorOpIn, wantZones...),
		}})
		termSets := [][]v1.TopologySelectorTerm{gridTerms(zones, racks, nil)}
		for name, merge := range map[string]func(*v1.NodeAffinity, [][]v1.TopologySelectorTerm, int) (*v1.NodeAffinity, error){
			"MergeAtCreate": MergeAtCreate,
			"MergeGated":    MergeGated,
		} {
			t.Run(fmt.Sprintf("%s/%dx%d", name, zones, racks), func(t *testing.T) {
				got, err := merge(nil, termSets, 32)
				if err != nil {
					t.Fatalf("%s() error: %v", name, err)
				}
				if !apiequality.Semantic.DeepEqual(got, want) {
					t.Errorf("%s() =\n%+v\nwant\n%+v", name, got, want)
				}
			})
		}
	}
}

// TestMergeGridMissingCell checks that a grid with one pair missing is not
// collapsed into the full grid, which would admit the missing pair.
func TestMergeGridMissingCell(t *testing.T) {
	terms := gridTerms(2, 2, func(z, r int) bool { return z == 1 && r == 1 })
	na, err := MergeAtCreate(nil, [][]v1.TopologySelectorTerm{terms}, 32)
	if err != nil {
		t.Fatalf("MergeAtCreate() error: %v", err)
	}
	missing := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", Labels: map[string]string{zoneKey: "zone-1", rackKey: "rack-1"}}}
	if matches(t, na, missing) {
		t.Errorf("merged affinity admits the missing pair: %+v", na)
	}
	for _, labels := range []map[string]string{
		{zoneKey: "zone-0", rackKey: "rack-0"},
		{zoneKey: "zone-0", rackKey: "rack-1"},
		{zoneKey: "zone-1", rackKey: "rack-0"},
	} {
		if !matches(t, na, &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", Labels: labels}}) {
			t.Errorf("merged affinity rejects %v: %+v", labels, na)
		}
	}
	if n := len(requiredTerms(na)); n != 2 {
		t.Errorf("merged affinity has %d terms, want 2", n)
	}
}

// TestCollapseBudgetGrowth checks that normalizing a grid uses work that grows
// no faster than quadratically in the number of terms.
func TestCollapseBudgetGrowth(t *testing.T) {
	used := func(zones, racks int) int {
		termSets := [][]v1.TopologySelectorTerm{gridTerms(zones, racks, nil)}
		budget, err := newMergeBudget(nil, termSets)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := normalizeSet(termSets[0], budget); err != nil {
			t.Fatalf("normalizing a %dx%d grid: %v", zones, racks, err)
		}
		return maxAffinityWork - budget.remaining
	}
	small, large := used(5, 10), used(20, 10)
	// 4x the terms may cost at most 16x the work.
	if large > 16*small {
		t.Errorf("work for 200 terms = %d, for 50 terms = %d: grew %.1fx, want at most 16x", large, small, float64(large)/float64(small))
	}
}

// TestCollapseManyValues guards against rebuilding the accumulated value set
// on every merge, which made thousands of single-key terms cost quadratic
// allocations while the budget only charged for the input.
func TestCollapseManyValues(t *testing.T) {
	var terms []v1.TopologySelectorTerm
	for i := 0; i < 4000; i++ {
		terms = append(terms, zoneTerm(fmt.Sprintf("zone-%d", i)))
	}
	termSets := [][]v1.TopologySelectorTerm{terms}
	budget, err := newMergeBudget(nil, termSets)
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	set, err := normalizeSet(terms, budget)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("normalizeSet() error: %v", err)
	}
	if len(set) != 1 || len(set[0][zoneKey]) != 4000 {
		t.Fatalf("normalizeSet() = %d terms, want one with 4000 zones", len(set))
	}
	// The quadratic version allocated about 500 MiB here; the fix about 26 MiB.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 128<<20 {
		t.Errorf("normalizing 4000 single-key terms allocated %d MiB, want at most 128 MiB", allocated>>20)
	}
}

// TestMergeGridWithExistingAffinity covers the append-only gated path: a grid
// topology merged into a pod's own required affinity must leave the pod's
// requirements unchanged, as a prefix, and append the collapsed topology.
func TestMergeGridWithExistingAffinity(t *testing.T) {
	userReq := req("disk", v1.NodeSelectorOpIn, "ssd")
	existing := requiredAffinity(v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{userReq}})
	termSets := [][]v1.TopologySelectorTerm{gridTerms(20, 10, nil)}
	for name, merge := range map[string]func(*v1.NodeAffinity, [][]v1.TopologySelectorTerm, int) (*v1.NodeAffinity, error){
		"MergeAtCreate": MergeAtCreate,
		"MergeGated":    MergeGated,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := merge(existing.DeepCopy(), termSets, 32)
			if err != nil {
				t.Fatalf("%s() error: %v", name, err)
			}
			terms := requiredTerms(got)
			if len(terms) != 1 {
				t.Fatalf("%s() produced %d terms, want 1: %+v", name, len(terms), got)
			}
			exprs := terms[0].MatchExpressions
			if len(exprs) != 3 || !apiequality.Semantic.DeepEqual(exprs[0], userReq) {
				t.Fatalf("%s() expressions = %+v, want the pod's requirement first, then rack and zone", name, exprs)
			}
			if exprs[1].Key != rackKey || len(exprs[1].Values) != 10 || exprs[2].Key != zoneKey || len(exprs[2].Values) != 20 {
				t.Errorf("%s() appended %+v, want rack In [10 racks] and zone In [20 zones]", name, exprs[1:])
			}
			if name == "MergeGated" && !validGatedMutation(existing, got) {
				t.Errorf("MergeGated() produced a mutation the API server rejects for a gated pod: %+v", got)
			}
		})
	}
}
