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

import v1 "k8s.io/api/core/v1"

// Work is bounded independently of the output term limit. Units account for
// input bytes and requirements, including unsuccessful or duplicate merges.
const maxAffinityWork = 1_000_000

type mergeBudget struct {
	remaining int
}

func newMergeBudget(existing *v1.NodeAffinity, termSets [][]v1.TopologySelectorTerm) (*mergeBudget, error) {
	b := &mergeBudget{remaining: maxAffinityWork}
	hasTopology := false
	for _, terms := range termSets {
		if err := b.spend(1 + len(terms)); err != nil {
			return nil, err
		}
		hasTopology = hasTopology || len(terms) != 0
		for _, term := range terms {
			if err := b.spend(topologyTermWork(term)); err != nil {
				return nil, err
			}
		}
	}
	if hasTopology {
		for _, term := range requiredTerms(existing) {
			if err := b.spend(nodeTermWork(term)); err != nil {
				return nil, err
			}
		}
	}
	return b, nil
}

func (b *mergeBudget) spend(work int) error {
	if work > b.remaining {
		return ErrTooComplex
	}
	b.remaining -= work
	return nil
}

func (b *mergeBudget) append(term v1.NodeSelectorTerm, reqs []v1.NodeSelectorRequirement) error {
	if err := b.spend(nodeTermWork(term) + requirementsWork(reqs)); err != nil {
		return err
	}
	existingWork := requirementsWork(term.MatchExpressions)
	count := len(term.MatchExpressions)
	for i := range reqs {
		reqWork := requirementsWork(reqs[i : i+1])
		// Each implication check can inspect both requirements' values.
		if err := b.spend(existingWork); err != nil {
			return err
		}
		if count > 0 && reqWork > b.remaining/count {
			return ErrTooComplex
		}
		if err := b.spend(reqWork * count); err != nil {
			return err
		}
		existingWork += reqWork
		count++
	}
	return nil
}

func labelWork(key string, values []string) int {
	work := 1 + len(key) + len(values)
	for _, value := range values {
		work += len(value)
	}
	return work
}

func requirementsWork(reqs []v1.NodeSelectorRequirement) int {
	work := len(reqs)
	for _, req := range reqs {
		work += len(req.Operator) + labelWork(req.Key, req.Values)
	}
	return work
}

func nodeTermWork(term v1.NodeSelectorTerm) int {
	return 1 + requirementsWork(term.MatchExpressions) + requirementsWork(term.MatchFields)
}

func topologyTermWork(term v1.TopologySelectorTerm) int {
	work := 1
	for _, req := range term.MatchLabelExpressions {
		work += labelWork(req.Key, req.Values)
	}
	return work
}

func conjunctionWork(c conjunction) int {
	work := 1
	for key, values := range c {
		work += labelWork(key, values)
	}
	return work
}
