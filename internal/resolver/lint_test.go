package resolver

import (
	"fmt"
	"testing"
)

func findFindingByCode(findings []LintFinding, code string) *LintFinding {
	for i := range findings {
		if findings[i].Code == code {
			return &findings[i]
		}
	}
	return nil
}

func TestLintSkillsRules(t *testing.T) {
	t.Parallel()

	// 1. trigger_collision: positive vs suppressed
	t.Run("trigger_collision", func(t *testing.T) {
		skillA := Skill{
			ID:          "skill-a",
			Status:      "active",
			Description: "First skill",
			Triggers:    []string{"deploy container image to cloud"},
			Examples:    []string{"e1", "e2", "e3"},
		}
		skillB := Skill{
			ID:          "skill-b",
			Status:      "active",
			Description: "Second skill",
			Triggers:    []string{"deploy container image to cloud"},
			Examples:    []string{"e1", "e2", "e3"},
		}

		// Positive: collision without suppression
		findings := LintSkills([]Skill{skillA, skillB})
		f := findFindingByCode(findings, "trigger_collision")
		if f == nil {
			t.Fatalf("expected trigger_collision, got findings: %+v", findings)
		}

		// Suppressed via distinguish_from
		skillASuppressed := skillA
		skillASuppressed.DistinguishFrom = []Discriminator{{SkillID: "skill-b"}}
		findingsSuppressed := LintSkills([]Skill{skillASuppressed, skillB})
		if f := findFindingByCode(findingsSuppressed, "trigger_collision"); f != nil {
			t.Fatalf("trigger_collision should be suppressed via distinguish_from: %+v", f)
		}

		// Suppressed via cross not_for
		skillANotFor := skillA
		skillANotFor.NotFor = []string{"deploy container image to cloud"}
		findingsNotFor := LintSkills([]Skill{skillANotFor, skillB})
		if f := findFindingByCode(findingsNotFor, "trigger_collision"); f != nil {
			t.Fatalf("trigger_collision should be suppressed via not_for: %+v", f)
		}
	})

	// 2. generic_trigger: positive vs valid
	t.Run("generic_trigger", func(t *testing.T) {
		// Fewer than 2 tokens
		skillShort := Skill{
			ID:          "short-skill",
			Triggers:    []string{"code"},
			Examples:    []string{"e1", "e2", "e3"},
			Description: "Short trigger",
		}
		findingsShort := LintSkills([]Skill{skillShort})
		if f := findFindingByCode(findingsShort, "generic_trigger"); f == nil {
			t.Fatalf("expected generic_trigger for <2 tokens")
		}

		// All tokens in generic stoplist
		skillGeneric := Skill{
			ID:          "generic-skill",
			Triggers:    []string{"code help fix task"},
			Examples:    []string{"e1", "e2", "e3"},
			Description: "Generic trigger",
		}
		findingsGeneric := LintSkills([]Skill{skillGeneric})
		if f := findFindingByCode(findingsGeneric, "generic_trigger"); f == nil {
			t.Fatalf("expected generic_trigger for all stoplist words")
		}

		// Valid trigger
		skillValid := Skill{
			ID:          "valid-skill",
			Triggers:    []string{"optimize postgres query index"},
			Examples:    []string{"e1", "e2", "e3"},
			Description: "Valid trigger",
		}
		findingsValid := LintSkills([]Skill{skillValid})
		if f := findFindingByCode(findingsValid, "generic_trigger"); f != nil {
			t.Fatalf("unexpected generic_trigger for domain trigger: %+v", f)
		}
	})

	// 3. missing_examples: positive vs valid
	t.Run("missing_examples", func(t *testing.T) {
		skillMissing := Skill{
			ID:          "missing-ex",
			Status:      "active",
			Triggers:    []string{"valid trigger words"},
			Examples:    []string{"only one example"},
			Description: "Missing examples",
		}
		findings := LintSkills([]Skill{skillMissing})
		if f := findFindingByCode(findings, "missing_examples"); f == nil {
			t.Fatalf("expected missing_examples for <3 examples")
		}

		skillComplete := Skill{
			ID:          "complete-ex",
			Status:      "active",
			Triggers:    []string{"valid trigger words"},
			Examples:    []string{"ex 1", "ex 2", "ex 3"},
			Description: "Complete examples",
		}
		findingsComplete := LintSkills([]Skill{skillComplete})
		if f := findFindingByCode(findingsComplete, "missing_examples"); f != nil {
			t.Fatalf("unexpected missing_examples: %+v", f)
		}
	})

	// 4. example_restates_trigger: positive vs valid
	t.Run("example_restates_trigger", func(t *testing.T) {
		skillRestating := Skill{
			ID:          "restate-skill",
			Status:      "active",
			Triggers:    []string{"optimize postgres database query indexes"},
			Examples:    []string{"optimize postgres database query indexes", "ex 2", "ex 3"},
			Description: "Restating example",
		}
		findings := LintSkills([]Skill{skillRestating})
		if f := findFindingByCode(findings, "example_restates_trigger"); f == nil {
			t.Fatalf("expected example_restates_trigger")
		}

		skillGood := Skill{
			ID:          "good-skill",
			Status:      "active",
			Triggers:    []string{"optimize postgres database query indexes"},
			Examples:    []string{"tune slow running order query execution plan", "ex 2", "ex 3"},
			Description: "Good example",
		}
		findingsGood := LintSkills([]Skill{skillGood})
		if f := findFindingByCode(findingsGood, "example_restates_trigger"); f != nil {
			t.Fatalf("unexpected example_restates_trigger: %+v", f)
		}
	})

	// 5. near_duplicate: positive vs suppressed
	t.Run("near_duplicate", func(t *testing.T) {
		skill1 := Skill{
			ID:          "service-deploy",
			Status:      "active",
			Description: "deploy container application service to production kubernetes cluster",
			Triggers:    []string{"deploy container image to kubernetes production"},
			Examples:    []string{"e1", "e2", "e3"},
		}
		skill2 := Skill{
			ID:          "service-publish",
			Status:      "active",
			Description: "deploy container application service to staging kubernetes cluster",
			Triggers:    []string{"deploy container image to kubernetes staging"},
			Examples:    []string{"e1", "e2", "e3"},
		}

		// Positive: near duplicate without relationship
		findings := LintSkills([]Skill{skill1, skill2})
		if f := findFindingByCode(findings, "near_duplicate"); f == nil {
			t.Fatalf("expected near_duplicate for overlapping description and triggers")
		}

		// Suppressed via equivalent_to
		skill1Eq := skill1
		skill1Eq.EquivalentTo = []string{"service-publish"}
		findingsEq := LintSkills([]Skill{skill1Eq, skill2})
		if f := findFindingByCode(findingsEq, "near_duplicate"); f != nil {
			t.Fatalf("near_duplicate should be suppressed via equivalent_to: %+v", f)
		}
	})

	// 6. pair-skip above 2000
	t.Run("lint_pairs_skipped", func(t *testing.T) {
		largeSkills := make([]Skill, 2001)
		for i := 0; i < 2001; i++ {
			largeSkills[i] = Skill{
				ID:          fmt.Sprintf("skill-%d", i),
				Status:      "active",
				Triggers:    []string{fmt.Sprintf("domain trigger %d", i)},
				Examples:    []string{"e1", "e2", "e3"},
				Description: fmt.Sprintf("Description %d", i),
			}
		}
		findings := LintSkills(largeSkills)
		if f := findFindingByCode(findings, "lint_pairs_skipped"); f == nil {
			t.Fatalf("expected lint_pairs_skipped for >2000 skills")
		}
		if f := findFindingByCode(findings, "trigger_collision"); f != nil {
			t.Fatalf("pairwise checks should be skipped when >2000 skills: %+v", f)
		}
	})
}
