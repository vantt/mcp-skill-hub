package resolver

import (
	"fmt"
	"testing"
)

func TestLintSkillsSingleSkillRules(t *testing.T) {
	t.Parallel()

	t.Run("missing_examples", func(t *testing.T) {
		skillFailing := Skill{
			ID:       "test-missing",
			Name:     "Test",
			Triggers: []string{"audit kubernetes security"},
			Examples: []string{"first example"},
		}
		findings := LintSkills([]Skill{skillFailing})
		found := false
		for _, f := range findings {
			if f.Code == "missing_examples" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected missing_examples finding, got: %v", findings)
		}

		skillPassing := Skill{
			ID:       "test-passing",
			Name:     "Test",
			Triggers: []string{"audit kubernetes security"},
			Examples: []string{"first natural request", "second natural request", "third natural request"},
		}
		findings = LintSkills([]Skill{skillPassing})
		for _, f := range findings {
			if f.Code == "missing_examples" {
				t.Fatalf("unexpected missing_examples finding on valid skill: %v", f)
			}
		}
	})

	t.Run("generic_trigger", func(t *testing.T) {
		skillFailing := Skill{
			ID:       "generic-skill",
			Name:     "Generic",
			Triggers: []string{"fix code issue", "short"},
			Examples: []string{"e1", "e2", "e3"},
		}
		findings := LintSkills([]Skill{skillFailing})
		foundCount := 0
		for _, f := range findings {
			if f.Code == "generic_trigger" {
				foundCount++
			}
		}
		if foundCount != 2 {
			t.Fatalf("expected 2 generic_trigger findings, got %d (all: %v)", foundCount, findings)
		}

		skillPassing := Skill{
			ID:       "domain-skill",
			Name:     "Domain",
			Triggers: []string{"audit kubernetes security cluster"},
			Examples: []string{"e1", "e2", "e3"},
		}
		findings = LintSkills([]Skill{skillPassing})
		for _, f := range findings {
			if f.Code == "generic_trigger" {
				t.Fatalf("unexpected generic_trigger finding on domain trigger: %v", f)
			}
		}
	})

	t.Run("example_restates_trigger", func(t *testing.T) {
		skillFailing := Skill{
			ID:       "restate-skill",
			Name:     "Restater",
			Triggers: []string{"deploy kubernetes container"},
			Examples: []string{"deploy kubernetes container", "ex2", "ex3"},
		}
		findings := LintSkills([]Skill{skillFailing})
		found := false
		for _, f := range findings {
			if f.Code == "example_restates_trigger" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected example_restates_trigger finding, got: %v", findings)
		}

		skillPassing := Skill{
			ID:       "natural-skill",
			Name:     "Natural",
			Triggers: []string{"deploy kubernetes container"},
			Examples: []string{"roll out our payment service pod to production cluster", "ex2", "ex3"},
		}
		findings = LintSkills([]Skill{skillPassing})
		for _, f := range findings {
			if f.Code == "example_restates_trigger" {
				t.Fatalf("unexpected example_restates_trigger finding: %v", f)
			}
		}
	})
}

func TestLintSkillsPairwiseRules(t *testing.T) {
	t.Parallel()

	t.Run("trigger_collision", func(t *testing.T) {
		a := Skill{
			ID:       "skill-a",
			Name:     "Skill A",
			Triggers: []string{"audit kubernetes security"},
			Examples: []string{"e1", "e2", "e3"},
		}
		b := Skill{
			ID:       "skill-b",
			Name:     "Skill B",
			Triggers: []string{"audit kubernetes security"},
			Examples: []string{"e1", "e2", "e3"},
		}

		// Colliding pair without relationship
		findings := LintSkills([]Skill{a, b})
		foundCollision := false
		for _, f := range findings {
			if f.Code == "trigger_collision" {
				foundCollision = true
			}
		}
		if !foundCollision {
			t.Fatalf("expected trigger_collision finding, got: %v", findings)
		}

		// Suppressed via distinguish_from
		aSuppressed := a
		aSuppressed.DistinguishFrom = []Discriminator{{SkillID: "skill-b"}}
		findings = LintSkills([]Skill{aSuppressed, b})
		for _, f := range findings {
			if f.Code == "trigger_collision" {
				t.Fatalf("unexpected trigger_collision after distinguish_from: %v", f)
			}
		}
	})

	t.Run("near_duplicate", func(t *testing.T) {
		a := Skill{
			ID:          "dup-a",
			Name:        "Docker Container Builder",
			Description: "build docker container images with multi-stage caching and push",
			Triggers:    []string{"build docker container image"},
			Examples:    []string{"e1", "e2", "e3"},
		}
		b := Skill{
			ID:          "dup-b",
			Name:        "Container Image Builder",
			Description: "build docker container images with multi-stage caching and push",
			Triggers:    []string{"build docker container image"},
			Examples:    []string{"e1", "e2", "e3"},
		}

		findings := LintSkills([]Skill{a, b})
		foundDup := false
		for _, f := range findings {
			if f.Code == "near_duplicate" {
				foundDup = true
			}
		}
		if !foundDup {
			t.Fatalf("expected near_duplicate finding, got: %v", findings)
		}

		// Suppressed via equivalent_to
		aSuppressed := a
		aSuppressed.EquivalentTo = []string{"dup-b"}
		findings = LintSkills([]Skill{aSuppressed, b})
		for _, f := range findings {
			if f.Code == "near_duplicate" {
				t.Fatalf("unexpected near_duplicate after equivalent_to: %v", f)
			}
		}
	})

	t.Run("lint_pairs_skipped_above_2000", func(t *testing.T) {
		manySkills := make([]Skill, 2001)
		for i := range manySkills {
			manySkills[i] = Skill{
				ID:       fmt.Sprintf("skill-%d", i),
				Triggers: []string{"audit kubernetes security"},
				Examples: []string{"e1", "e2", "e3"},
			}
		}
		findings := LintSkills(manySkills)
		foundSkipped := false
		for _, f := range findings {
			if f.Code == "lint_pairs_skipped" {
				foundSkipped = true
			}
			if f.Code == "trigger_collision" || f.Code == "near_duplicate" {
				t.Fatalf("pairwise rule executed despite > 2000 skills: %v", f)
			}
		}
		if !foundSkipped {
			t.Fatalf("expected lint_pairs_skipped finding")
		}
	})
}
