package resolver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type influenceCase struct {
	ownerIndex int
	kind       string // "example" or "counter_example"
	index      int
	phrase     string
}

// influenceSkills loads the golden skills with the routing examples overlay.
func influenceSkills(t *testing.T) []Skill {
	t.Helper()
	skills := loadGolden(t).Skills
	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "routing", "examples-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var overlay struct {
		Skills map[string]struct {
			Examples        []string `json:"examples"`
			CounterExamples []string `json:"counter_examples"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(contents, &overlay); err != nil {
		t.Fatal(err)
	}
	for i := range skills {
		skills[i].Status = "active"
		skills[i].Examples = overlay.Skills[skills[i].ID].Examples
		skills[i].CounterExamples = overlay.Skills[skills[i].ID].CounterExamples
		if skills[i].MinScope == "" {
			skills[i].MinScope = "single_step"
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	return skills
}

func influenceCases(skills []Skill) []influenceCase {
	var cases []influenceCase
	for i, skill := range skills {
		for index, phrase := range skill.Examples {
			cases = append(cases, influenceCase{i, "example", index, phrase})
		}
		for index, phrase := range skill.CounterExamples {
			cases = append(cases, influenceCase{i, "counter_example", index, phrase})
		}
	}
	return cases
}

func (c influenceCase) request(owner Skill) Request {
	operation := ""
	if len(owner.Operations) > 0 {
		operation = owner.Operations[0]
	}
	var facts []Fact
	for _, requirement := range owner.Requirements.FactsAll {
		facts = append(facts, Fact{Key: requirement.Key, Value: requirement.Value, Basis: "user"})
	}
	if len(owner.Requirements.FactsAny) > 0 {
		first := owner.Requirements.FactsAny[0]
		facts = append(facts, Fact{Key: first.Key, Value: first.Value, Basis: "user"})
	}
	capabilities := append([]string{}, owner.Requirements.CapabilitiesAll...)
	if len(owner.Requirements.CapabilitiesAny) > 0 {
		capabilities = append(capabilities, owner.Requirements.CapabilitiesAny[0])
	}
	return Request{
		SchemaVersion: SchemaVersion,
		RequestID:     "influence-case",
		Task:          Task{Description: c.phrase, Scope: owner.MinScope},
		Operation:     operation,
		Context:       RequestContext{Facts: facts, Execution: Execution{Capabilities: capabilities}},
	}
}

// withoutPhrase copies the library with the case's phrase removed from its owner.
func (c influenceCase) withoutPhrase(skills []Skill) []Skill {
	result := append([]Skill(nil), skills...)
	owner := result[c.ownerIndex]
	if c.kind == "example" {
		owner.Examples = append(append([]string(nil), owner.Examples[:c.index]...), owner.Examples[c.index+1:]...)
	} else {
		owner.CounterExamples = append(append([]string(nil), owner.CounterExamples[:c.index]...), owner.CounterExamples[c.index+1:]...)
	}
	result[c.ownerIndex] = owner
	return result
}

func influenceResolve(t *testing.T, skills []Skill, policy Policy, request Request) (Status, string) {
	t.Helper()
	engine, err := New(memoryCatalog{skills: skills, snapshot: "influence"}, policy, NewCache(1))
	if err != nil {
		t.Fatal(err)
	}
	response, err := engine.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	primary := ""
	if response.Primary != nil {
		primary = response.Primary.ID
	}
	return response.Status, primary
}

// linkedSkill is the linked flag of Influences: the skill has distinguish_from,
// or another skill's distinguish_from names it.
func linkedSkill(skills []Skill, id string) bool {
	for _, skill := range skills {
		if skill.ID == id && len(skill.DistinguishFrom) > 0 {
			return true
		}
		for _, discriminator := range skill.DistinguishFrom {
			if discriminator.SkillID == id {
				return true
			}
		}
	}
	return false
}

func TestInfluencePredicateCoversEveryChangedResult(t *testing.T) {
	skills := influenceSkills(t)
	policy := DefaultPolicy()
	cases := influenceCases(skills)

	stride := len(skills) / 8
	switch {
	case testing.Short():
		stride = len(skills) / 3
	case os.Getenv("SKILLHUB_PERF") == "1":
		stride = 1
	}
	if stride < 1 {
		stride = 1
	}

	type outcome struct {
		status  Status
		primary string
	}
	before := make([]outcome, len(cases))
	for i, c := range cases {
		status, primary := influenceResolve(t, c.withoutPhrase(skills), policy, c.request(skills[c.ownerIndex]))
		before[i] = outcome{status, primary}
	}

	changedResults := 0
	for target := 0; target < len(skills); target += stride {
		donor := skills[(target+1)%len(skills)]
		mutations := map[string]func(Skill) Skill{
			"borrowed trigger": func(skill Skill) Skill {
				if len(donor.Examples) == 0 {
					return skill
				}
				skill.Triggers = append(append([]string(nil), skill.Triggers...), donor.Examples[0])
				return skill
			},
			"unrelated triggers": func(skill Skill) Skill {
				skill.Triggers = []string{"quarterly zebra migration"}
				return skill
			},
		}
		for name, mutate := range mutations {
			mutated := append([]Skill(nil), skills...)
			mutated[target] = mutate(skills[target])
			id := skills[target].ID
			for i, c := range cases {
				request := c.request(skills[c.ownerIndex])
				status, primary := influenceResolve(t, c.withoutPhrase(mutated), policy, request)
				if status == before[i].status && primary == before[i].primary {
					continue
				}
				changedResults++
				predicted := Influences(skills[target], request, policy, linkedSkill(skills, id)) ||
					Influences(mutated[target], request, policy, linkedSkill(mutated, id))
				if !predicted {
					t.Errorf("%s on %s changed %q (%s #%d of %s) from %s/%s to %s/%s outside the predicted set",
						name, id, c.phrase, c.kind, c.index, skills[c.ownerIndex].ID, before[i].status, before[i].primary, status, primary)
				}
				if primary == id && before[i].primary != id && !Influences(mutated[target], request, policy, linkedSkill(mutated, id)) {
					t.Errorf("%s on %s took %q but the new version is not predicted to influence it", name, id, c.phrase)
				}
			}
		}
	}
	t.Logf("changed results: %d", changedResults)
	if changedResults == 0 {
		t.Fatal("no mutation changed any result; the test proves nothing")
	}
}

// Extracting the shared helpers must not move any score: the leave-one-out
// outcome of every case is pinned by the routing report snapshot, and the
// helpers themselves keep Resolve's arithmetic.
func TestInfluenceHelpersKeepResolveRules(t *testing.T) {
	policy := DefaultPolicy()
	floor := policy.ApplicabilityFloor
	if !runnerUpApplicable(floor, false, policy) || runnerUpApplicable(floor-.01, false, policy) {
		t.Fatal("unlinked runner-up must sit at the floor")
	}
	if !runnerUpApplicable(floor-.15, true, policy) || runnerUpApplicable(floor-.16, true, policy) {
		t.Fatal("linked runner-up gets a slack of 0.15")
	}
	candidate := scoredCandidate{Score: .1, Features: features{Constraint: 1, NotFor: .5}}
	want := .1 + policy.Weights.Constraint + policy.Weights.NotFor*.5
	if got := excludedRelevance(candidate, policy); got != want {
		t.Fatalf("excluded relevance = %v, want %v", got, want)
	}
}

func TestInfluencesRules(t *testing.T) {
	policy := DefaultPolicy()
	skill := Skill{ID: "deploy-helper", CollectionID: "core", Name: "Deploy helper", Description: "ship releases to production", Status: "active", MinScope: "multi_step", Triggers: []string{"ship release to production"}, NotFor: []string{"write poems"}}
	request := func(description, scope string) Request {
		return Request{SchemaVersion: SchemaVersion, RequestID: "r", Task: Task{Description: description, Scope: scope}}
	}
	if !Influences(skill, request("ship release to production", "multi_step"), policy, false) {
		t.Fatal("a matching request must be influenced")
	}
	if Influences(skill, request("ship release to production", "single_step"), policy, false) {
		t.Fatal("a skill above the request scope is filtered out")
	}
	if Influences(skill, request("bake sourdough bread", "multi_step"), policy, false) {
		t.Fatal("an unrelated request must not be influenced")
	}
	excluded := skill
	excluded.NotFor = []string{"ship release to production"}
	if !Influences(excluded, request("ship release to production", "multi_step"), policy, false) {
		t.Fatal("a hard-excluded skill with high relevance still shapes the outcome")
	}
}
