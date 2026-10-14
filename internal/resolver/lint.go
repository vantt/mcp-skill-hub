package resolver

import (
	"fmt"
	"strings"
)

type LintFinding struct {
	Code         string `json:"code"`
	SkillID      string `json:"skill_id"`
	OtherSkillID string `json:"other_skill_id,omitempty"`
	Field        string `json:"field,omitempty"`
	Value        string `json:"value,omitempty"`
	Message      string `json:"message"`
	Fix          string `json:"fix"`
}

var genericTriggerStoplist = setOf(
	"code", "coding", "help", "fix", "task", "work", "write", "create", "update",
	"change", "build", "make", "do", "run", "use", "general", "stuff", "thing",
	"project", "app", "file", "review", "test", "debug", "issue", "problem",
)

const maxPairwiseLintSkills = 2000

// LintSkills audits skills for routing metadata quality issues.
func LintSkills(skills []Skill) []LintFinding {
	var findings []LintFinding

	// Per-skill single-entity rules
	for _, skill := range skills {
		findings = append(findings, lintSingleSkill(skill)...)
	}

	// Pairwise rules
	if len(skills) > maxPairwiseLintSkills {
		findings = append(findings, LintFinding{
			Code:    "lint_pairs_skipped",
			Message: "pairwise lint skipped because active skill count exceeds 2000",
			Fix:     "audit skill pairs in smaller scopes",
		})
		return findings
	}

	for i := 0; i < len(skills); i++ {
		for j := i + 1; j < len(skills); j++ {
			findings = append(findings, lintSkillPair(skills[i], skills[j])...)
		}
	}

	return findings
}

// LintTargetSkill audits a single skill (which may be a draft under review)
// against a set of active skills.
func LintTargetSkill(target Skill, activeSkills []Skill) []LintFinding {
	findings := lintSingleSkill(target)
	if len(activeSkills) > maxPairwiseLintSkills {
		findings = append(findings, LintFinding{
			Code:    "lint_pairs_skipped",
			SkillID: target.ID,
			Message: "pairwise lint skipped because active skill count exceeds 2000",
			Fix:     "audit skill pairs in smaller scopes",
		})
		return findings
	}
	for _, other := range activeSkills {
		if other.ID == target.ID {
			continue
		}
		findings = append(findings, lintSkillPair(target, other)...)
	}
	return findings
}

func lintSingleSkill(skill Skill) []LintFinding {
	var findings []LintFinding

	// Rule: missing_examples
	if len(skill.Examples) < 3 {
		findings = append(findings, LintFinding{
			Code:    "missing_examples",
			SkillID: skill.ID,
			Field:   "routing.examples",
			Message: fmt.Sprintf("skill %q has %d examples; active skills require at least 3 examples", skill.ID, len(skill.Examples)),
			Fix:     "Add 3–5 natural task phrasings",
		})
	}

	// Rule: generic_trigger
	for _, trigger := range skill.Triggers {
		tokens := tokenize(trigger)
		isGeneric := false
		if len(tokens) < 2 {
			isGeneric = true
		} else {
			allGeneric := true
			for _, t := range tokens {
				if !genericTriggerStoplist[t] {
					allGeneric = false
					break
				}
			}
			if allGeneric {
				isGeneric = true
			}
		}
		if isGeneric {
			findings = append(findings, LintFinding{
				Code:    "generic_trigger",
				SkillID: skill.ID,
				Field:   "routing.triggers",
				Value:   trigger,
				Message: fmt.Sprintf("trigger %q on skill %q contains only generic terms or fewer than 2 tokens", trigger, skill.ID),
				Fix:     "Make the trigger name the domain object",
			})
		}
	}

	// Rule: example_restates_trigger
	for _, example := range skill.Examples {
		exTokens := tokenize(example)
		if bestOverlap(exTokens, skill.Triggers) >= 0.9 {
			findings = append(findings, LintFinding{
				Code:    "example_restates_trigger",
				SkillID: skill.ID,
				Field:   "routing.examples",
				Value:   example,
				Message: fmt.Sprintf("example %q on skill %q overlaps >= 0.9 with a trigger", example, skill.ID),
				Fix:     "Rephrase as a real request",
			})
		}
	}

	return findings
}

func lintSkillPair(a, b Skill) []LintFinding {
	var findings []LintFinding

	hasRelationship := pairHasRelationship(a, b)

	// Rule: trigger_collision
	for _, tA := range a.Triggers {
		tokA := tokenize(tA)
		for _, tB := range b.Triggers {
			tokB := tokenize(tB)
			if equalEvidence(tA, tB) || overlap(tokA, tokB) >= 0.8 {
				if !hasRelationship {
					findings = append(findings, LintFinding{
						Code:         "trigger_collision",
						SkillID:      a.ID,
						OtherSkillID: b.ID,
						Field:        "routing.triggers",
						Value:        fmt.Sprintf("%s <=> %s", tA, tB),
						Message:      fmt.Sprintf("trigger %q of %q collides with trigger %q of %q", tA, a.ID, tB, b.ID),
						Fix:          "Add distinguish_from or narrow one trigger",
					})
				}
			}
		}
	}

	// Rule: near_duplicate
	tokensA := tokenize(a.Description + " " + strings.Join(a.Triggers, " "))
	tokensB := tokenize(b.Description + " " + strings.Join(b.Triggers, " "))
	if overlap(tokensA, tokensB) >= 0.6 {
		if !hasRelationship {
			findings = append(findings, LintFinding{
				Code:         "near_duplicate",
				SkillID:      a.ID,
				OtherSkillID: b.ID,
				Message:      fmt.Sprintf("skills %q and %q have >= 0.6 description and trigger overlap without distinguishing relationships", a.ID, b.ID),
				Fix:          "Add distinguish_from or equivalent_to, or merge",
			})
		}
	}

	return findings
}

func pairHasRelationship(a, b Skill) bool {
	return namesSkill(a, b) || namesSkill(b, a)
}

func namesSkill(source, target Skill) bool {
	for _, d := range source.DistinguishFrom {
		if d.SkillID == target.ID {
			return true
		}
	}
	for _, eq := range source.EquivalentTo {
		if eq == target.ID {
			return true
		}
	}
	for _, eq := range source.Equivalence {
		if eq.SkillID == target.ID {
			return true
		}
	}
	targetNameTokens := tokenize(target.Name)
	for _, notFor := range append(source.NotFor, source.CounterExamples...) {
		notForTokens := tokenize(notFor)
		if len(notForTokens) > 0 {
			if len(targetNameTokens) > 0 && overlap(notForTokens, targetNameTokens) >= 0.5 {
				return true
			}
			if bestOverlap(notForTokens, target.Triggers) >= 0.5 {
				return true
			}
		}
	}
	return false
}

// ExampleResolvesElsewhere reports an example that does not resolve to its own
// skill. The fix depends on what the resolver did instead: a winning skill, no
// skill at all, or a question to the user.
func ExampleResolvesElsewhere(skillID, phrase string, status Status, winner string) LintFinding {
	finding := LintFinding{
		Code:    "example_resolves_elsewhere",
		SkillID: skillID,
		Field:   "routing.examples",
		Value:   phrase,
	}
	switch {
	case status == StatusResolved && winner != "":
		finding.OtherSkillID = winner
		finding.Message = fmt.Sprintf("example %q of %q resolves to %q", phrase, skillID, winner)
		finding.Fix = fmt.Sprintf("Add a not_for entry on %s that covers this request, add distinguish_from, or sharpen this skill's triggers", winner)
	case status == StatusNeedsContext:
		finding.Message = fmt.Sprintf("example %q of %q needs a choice between skills", phrase, skillID)
		finding.Fix = "Narrow the triggers so one skill wins, or add distinguish_from with a discriminator question"
	default:
		finding.Message = fmt.Sprintf("example %q of %q resolves to no skill", phrase, skillID)
		finding.Fix = "Add a trigger that names this request's domain object, or rephrase the example"
	}
	return finding
}

// CounterExampleResolvesToSkill reports a counter-example that still resolves to
// the skill it says the skill is not for.
func CounterExampleResolvesToSkill(skillID, phrase string) LintFinding {
	return LintFinding{
		Code:    "counter_example_resolves_to_skill",
		SkillID: skillID,
		Field:   "routing.counter_examples",
		Value:   phrase,
		Message: fmt.Sprintf("counter-example %q of %q resolves to %q", phrase, skillID, skillID),
		Fix:     "Add a not_for entry that covers this request, or narrow the trigger it matches",
	}
}

// ExampleTakenBy reports an example of owner that the changed skill taker now
// wins, or shares with owner in a choice. already is true when the same held
// before the change.
func ExampleTakenBy(taker, owner, phrase string, wins, already bool) LintFinding {
	when := "now"
	if already {
		when = "already"
	}
	message := fmt.Sprintf("example %q of %q %s resolves to %q", phrase, owner, when, taker)
	if !wins {
		need := "now needs a choice"
		if already {
			need = "already needs one"
		}
		message = fmt.Sprintf("example %q of %q %s between %q and %q", phrase, owner, need, taker, owner)
	}
	return LintFinding{
		Code:         "example_taken_by",
		SkillID:      taker,
		OtherSkillID: owner,
		Field:        "routing.examples",
		Value:        phrase,
		Message:      message,
		Fix:          fmt.Sprintf("Add a not_for entry on %s that covers this request, narrow %s's triggers, or add distinguish_from between %s and %s", taker, taker, taker, owner),
	}
}

// SelfResolutionSkipped reports that the examples were not resolved, with the
// reason. skillID is the single skill under check, or empty.
func SelfResolutionSkipped(skillID, reason, fix string) LintFinding {
	return LintFinding{Code: "self_resolution_skipped", SkillID: skillID, Message: "example check skipped: " + reason, Fix: fix}
}
