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
	"encoding/json"
	"errors"
	"slices"
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// ErrUnrepresentable means topology cannot be appended to existing gated terms.
	ErrUnrepresentable = errors.New("snapshot topology cannot be merged into the pod's existing required node affinity")
	// ErrTooManyTerms means the merged affinity exceeds the term cap.
	ErrTooManyTerms = errors.New("merged node affinity exceeds the maximum number of node selector terms")
	// ErrTooComplex is returned when normalization or merging exceeds its work budget.
	ErrTooComplex = errors.New("snapshot topology exceeds the normalization and merge work limit")
	// ErrUnsatisfiable means a source topology set matches no node.
	ErrUnsatisfiable = errors.New("snapshot topology matches no node")
)

// NodeMatches ORs topology terms; requirements within a term are ANDed.
// Empty terms match no node.
func NodeMatches(nodeLabels map[string]string, terms []v1.TopologySelectorTerm) bool {
	for _, term := range terms {
		if termMatches(nodeLabels, term) {
			return true
		}
	}
	return false
}

func termMatches(nodeLabels map[string]string, term v1.TopologySelectorTerm) bool {
	if len(term.MatchLabelExpressions) == 0 {
		return false
	}
	for _, expr := range term.MatchLabelExpressions {
		nodeVal, exists := nodeLabels[expr.Key]
		if !exists || !slices.Contains(expr.Values, nodeVal) {
			return false
		}
	}
	return true
}

// conjunction maps topology keys to sorted, deduplicated allowed values.
type conjunction map[string][]string

func (c conjunction) keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (c conjunction) canonical() string {
	type kv struct {
		K string   `json:"k"`
		V []string `json:"v"`
	}
	out := make([]kv, 0, len(c))
	for _, k := range c.keys() {
		out = append(out, kv{K: k, V: c[k]})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func (c conjunction) requirements() []v1.NodeSelectorRequirement {
	reqs := make([]v1.NodeSelectorRequirement, 0, len(c))
	for _, k := range c.keys() {
		reqs = append(reqs, v1.NodeSelectorRequirement{
			Key:      k,
			Operator: v1.NodeSelectorOpIn,
			Values:   slices.Clone(c[k]),
		})
	}
	return reqs
}

// normalizeTerm intersects repeated keys; ok=false means a contradiction.
func normalizeTerm(term v1.TopologySelectorTerm) (conjunction, bool) {
	c := conjunction{}
	for _, expr := range term.MatchLabelExpressions {
		values := sets.New(expr.Values...)
		if existing, found := c[expr.Key]; found {
			values = values.Intersection(sets.New(existing...))
		}
		if values.Len() == 0 {
			return nil, false
		}
		c[expr.Key] = sets.List(values)
	}
	return c, true
}

func normalizeTerms(terms []v1.TopologySelectorTerm) []conjunction {
	var set []conjunction
	seen := sets.New[string]()
	for _, term := range terms {
		if len(term.MatchLabelExpressions) == 0 {
			continue
		}
		c, ok := normalizeTerm(term)
		if !ok {
			continue
		}
		if key := c.canonical(); !seen.Has(key) {
			seen.Insert(key)
			set = append(set, c)
		}
	}
	sort.Slice(set, func(i, j int) bool { return set[i].canonical() < set[j].canonical() })
	return set
}

func normalizeSet(terms []v1.TopologySelectorTerm, budget *mergeBudget) ([]conjunction, error) {
	for _, term := range terms {
		if err := budget.spend(topologyTermWork(term)); err != nil {
			return nil, err
		}
	}
	return collapse(normalizeTerms(terms), budget)
}

// collapse unions terms with identical keys and at most one differing value set.
// Merging {r1,z1} with {r2,z2} would wrongly admit {r1,z2}.
func collapse(set []conjunction, budget *mergeBudget) ([]conjunction, error) {
	for merged := true; merged; {
		merged = false
		for i := 0; i < len(set) && !merged; i++ {
			for j := i + 1; j < len(set); j++ {
				if err := budget.spend(conjunctionWork(set[i]) + conjunctionWork(set[j])); err != nil {
					return nil, err
				}
				if c, ok := mergeConjunctions(set[i], set[j]); ok {
					set[i] = c
					set = slices.Delete(set, j, j+1)
					merged = true
					break
				}
			}
		}
	}
	sort.Slice(set, func(i, j int) bool { return set[i].canonical() < set[j].canonical() })
	return set, nil
}

func mergeConjunctions(a, b conjunction) (conjunction, bool) {
	if len(a) != len(b) {
		return nil, false
	}
	differing := ""
	for k, av := range a {
		bv, found := b[k]
		if !found {
			return nil, false
		}
		if slices.Equal(av, bv) {
			continue
		}
		if differing != "" {
			return nil, false
		}
		differing = k
	}
	out := conjunction{}
	for k, v := range a {
		out[k] = v
	}
	if differing != "" {
		out[differing] = sets.List(sets.New(a[differing]...).Union(sets.New(b[differing]...)))
	}
	return out, true
}

func normalizeSets(termSets [][]v1.TopologySelectorTerm, budget *mergeBudget) ([][]conjunction, error) {
	var out [][]conjunction
	seen := sets.New[string]()
	for _, terms := range termSets {
		if len(terms) == 0 {
			continue
		}
		set, err := normalizeSet(terms, budget)
		if err != nil {
			return nil, err
		}
		if len(set) == 0 {
			return nil, ErrUnsatisfiable
		}
		// Repeated topology sets add no constraint.
		parts := make([]string, 0, len(set))
		for _, c := range set {
			parts = append(parts, c.canonical())
		}
		b, _ := json.Marshal(parts)
		if key := string(b); !seen.Has(key) {
			seen.Insert(key)
			out = append(out, set)
		}
	}
	return out, nil
}

// MergeAtCreate ANDs topology into node affinity, allowing a cross-product
// of existing required terms and topology terms.
func MergeAtCreate(existing *v1.NodeAffinity, termSets [][]v1.TopologySelectorTerm, maxTerms int) (*v1.NodeAffinity, error) {
	budget, err := newMergeBudget(existing, termSets)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeSets(termSets, budget)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return existing.DeepCopy(), nil
	}
	return crossProduct(existing, normalized, maxTerms, budget)
}

// MergeGated ANDs topology into node affinity using append-only changes.
// Existing required terms cannot be replaced or multiplied on gated pods.
func MergeGated(existing *v1.NodeAffinity, termSets [][]v1.TopologySelectorTerm, maxTerms int) (*v1.NodeAffinity, error) {
	budget, err := newMergeBudget(existing, termSets)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeSets(termSets, budget)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return existing.DeepCopy(), nil
	}
	existingTerms := requiredTerms(existing)
	if len(existingTerms) == 0 {
		return crossProduct(existing, normalized, maxTerms, budget)
	}
	if len(existingTerms) > maxTerms {
		return nil, ErrTooManyTerms
	}

	var reqs []v1.NodeSelectorRequirement
	for _, set := range normalized {
		if len(set) != 1 {
			return nil, ErrUnrepresentable
		}
		reqs = append(reqs, set[0].requirements()...)
	}
	out := make([]v1.NodeSelectorTerm, 0, len(existingTerms))
	for _, t := range existingTerms {
		if isEmptyTerm(t) {
			// The API server rejects appending requirements to an empty term.
			out = append(out, t)
			continue
		}
		if err := budget.append(t, reqs); err != nil {
			return nil, err
		}
		out = append(out, appendRequirements(t, reqs))
	}
	return withRequiredTerms(existing, out), nil
}

func crossProduct(existing *v1.NodeAffinity, normalized [][]conjunction, maxTerms int, budget *mergeBudget) (*v1.NodeAffinity, error) {
	// Preserve empty existing terms: they match no nodes.
	var emptyTerm *v1.NodeSelectorTerm
	var terms []v1.NodeSelectorTerm
	for _, t := range requiredTerms(existing) {
		if isEmptyTerm(t) {
			emptyTerm = &t
		} else {
			terms = append(terms, t)
		}
	}
	if len(requiredTerms(existing)) == 0 {
		terms = []v1.NodeSelectorTerm{{}}
	}

	// Later stages may prune terms. Bound attempted work here; apply the term
	// cap only to the final result.
	var contradiction *v1.NodeSelectorTerm
	for _, set := range normalized {
		var next []v1.NodeSelectorTerm
		seen := sets.New[string]()
		for _, t := range terms {
			for _, c := range set {
				if err := budget.spend(conjunctionWork(c)); err != nil {
					return nil, err
				}
				reqs := c.requirements()
				if err := budget.append(t, reqs); err != nil {
					return nil, err
				}
				merged, ok := simplify(appendRequirements(t, reqs))
				if !ok {
					if contradiction == nil {
						contradiction = &merged
					}
					continue
				}
				if key := canonicalTerm(merged); !seen.Has(key) {
					seen.Insert(key)
					next = append(next, merged)
				}
			}
		}
		terms = next
	}

	out := terms
	if emptyTerm != nil {
		out = append([]v1.NodeSelectorTerm{*emptyTerm}, terms...)
	}
	// Retain one contradictory term to express an unschedulable result.
	if len(out) == 0 && contradiction != nil {
		out = []v1.NodeSelectorTerm{*contradiction}
	}
	if len(out) > maxTerms {
		return nil, ErrTooManyTerms
	}
	return withRequiredTerms(existing, out), nil
}

// simplify intersects repeated In requirements; ok=false means a contradiction.
func simplify(term v1.NodeSelectorTerm) (v1.NodeSelectorTerm, bool) {
	first := map[string]int{}
	var exprs []v1.NodeSelectorRequirement
	for _, e := range term.MatchExpressions {
		if e.Operator != v1.NodeSelectorOpIn {
			exprs = append(exprs, e)
			continue
		}
		i, found := first[e.Key]
		if !found {
			first[e.Key] = len(exprs)
			exprs = append(exprs, e)
			continue
		}
		values := sets.New(exprs[i].Values...).Intersection(sets.New(e.Values...))
		if values.Len() == 0 {
			return term, false
		}
		exprs[i].Values = sets.List(values)
	}
	term.MatchExpressions = exprs
	return term, true
}

func canonicalTerm(t v1.NodeSelectorTerm) string {
	canonical := func(reqs []v1.NodeSelectorRequirement) []string {
		out := make([]string, 0, len(reqs))
		for _, r := range reqs {
			b, _ := json.Marshal(v1.NodeSelectorRequirement{Key: r.Key, Operator: r.Operator, Values: sets.List(sets.New(r.Values...))})
			out = append(out, string(b))
		}
		sort.Strings(out)
		return out
	}
	b, _ := json.Marshal([][]string{canonical(t.MatchExpressions), canonical(t.MatchFields)})
	return string(b)
}

func requiredTerms(na *v1.NodeAffinity) []v1.NodeSelectorTerm {
	if na == nil || na.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return nil
	}
	return na.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
}

func withRequiredTerms(existing *v1.NodeAffinity, terms []v1.NodeSelectorTerm) *v1.NodeAffinity {
	out := existing.DeepCopy()
	if out == nil {
		out = &v1.NodeAffinity{}
	}
	out.RequiredDuringSchedulingIgnoredDuringExecution = &v1.NodeSelector{NodeSelectorTerms: terms}
	return out
}

func isEmptyTerm(t v1.NodeSelectorTerm) bool {
	return len(t.MatchExpressions) == 0 && len(t.MatchFields) == 0
}

func appendRequirements(term v1.NodeSelectorTerm, reqs []v1.NodeSelectorRequirement) v1.NodeSelectorTerm {
	out := *term.DeepCopy()
	for _, r := range reqs {
		if !slices.ContainsFunc(out.MatchExpressions, func(e v1.NodeSelectorRequirement) bool { return implies(e, r) }) {
			out.MatchExpressions = append(out.MatchExpressions, r)
		}
	}
	return out
}

// implies tests whether satisfying a guarantees b.
func implies(a, b v1.NodeSelectorRequirement) bool {
	if a.Key != b.Key || a.Operator != b.Operator {
		return false
	}
	if a.Operator == v1.NodeSelectorOpIn {
		return sets.New(b.Values...).IsSuperset(sets.New(a.Values...))
	}
	return sets.New(a.Values...).Equal(sets.New(b.Values...))
}
