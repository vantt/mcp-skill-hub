package resolver

import (
	"fmt"
	"slices"
)

// LintFinding describes one routing quality warning.
type LintFinding struct {
	Code         string `json:"code"`
	SkillID      string `json:"skill_id,omitempty"`
	OtherSkillID string `json:"other_skill_id,omitempty"`
	Field        string `json:"field,omitempty"`
	Value        string `json:"value,omitempty"`
	Message      string `json:"message"`
	Fix          string `json:"fix"`
}

var genericStoplist = map[string]bool{
	"code": true, "coding": true, "help": true, "fix": true, "task": true,
	"work": true, "write": true, "create": true, "update": true, "change": true,
	"build": true, "make": true, "do": true, "run": true, "use": true,
	"general": true, "stuff": true, "thing": true, "project": true, "app": true,
	"file": true, "review": true, "test": true, "debug": true, "issue": true, "problem": true,
}

const maxPairwiseSkills = 2000

func namesSkill(a Skill, bID string) bool {
	for _, df := range a.DistinguishFrom {
		if df.SkillID == bID {
			return true
		}
	}
	if slices.Contains(a.EquivalentTo, bID) {
		return true
	}
	for _, eq := range a.Equivalence {
		if eq.SkillID == bID {
			return true
		}
	}
	return false
}

func crossNotForSuppressed(a, b Skill) bool {
	targetPhrases := append([]string{b.Name, b.ID}, b.Triggers...)
	candidates := append([]string{}, a.NotFor...)
	candidates = append(candidates, a.CounterExamples...)

	for _, target := range targetPhrases {
		targetTokens := tokenize(target)
		for _, cand := range candidates {
			candTokens := tokenize(cand)
			if overlap(candTokens, targetTokens) >= 0.5 {
				return true
			}
		}
	}
	return false
}

func arePairSuppressed(a, b Skill) bool {
	if namesSkill(a, b.ID) || namesSkill(b, a.ID) {
		return true
	}
	if crossNotForSuppressed(a, b) || crossNotForSuppressed(b, a) {
		return true
	}
	return false
}

func uniqueTokenSet(tokens []string) []string {
	seen := make(map[string]bool, len(tokens))
	var out []string
	for _, tok := range tokens {
		if !seen[tok] {
			seen[tok] = true
			out = append(out, tok)
		}
	}
	return out
}

func skillDescriptionAndTriggerTokens(s Skill) []string {
	var tokens []string
	tokens = append(tokens, tokenize(s.Description)...)
	for _, tr := range s.Triggers {
		tokens = append(tokens, tokenize(tr)...)
	}
	return uniqueTokenSet(tokens)
}

func lintSingleSkill(s Skill) []LintFinding {
	var findings []LintFinding
	for _, tr := range s.Triggers {
		tokens := tokenize(tr)
		if len(tokens) < 2 {
			findings = append(findings, LintFinding{
				Code:    "generic_trigger",
				SkillID: s.ID,
				Field:   "routing.triggers",
				Value:   tr,
				Message: fmt.Sprintf("Trigger %q in skill %s has fewer than 2 tokens after normalization.", tr, s.ID),
				Fix:     "Make the trigger name the domain object",
			})
			continue
		}
		isAllGeneric := true
		for _, tok := range tokens {
			if !genericStoplist[tok] {
				isAllGeneric = false
				break
			}
		}
		if isAllGeneric {
			findings = append(findings, LintFinding{
				Code:    "generic_trigger",
				SkillID: s.ID,
				Field:   "routing.triggers",
				Value:   tr,
				Message: fmt.Sprintf("Every token of trigger %q in skill %s is generic.", tr, s.ID),
				Fix:     "Make the trigger name the domain object",
			})
		}
	}

	if s.Status == "active" || s.Status == "" {
		if len(s.Examples) < 3 {
			findings = append(findings, LintFinding{
				Code:    "missing_examples",
				SkillID: s.ID,
				Field:   "routing.examples",
				Value:   fmt.Sprintf("%d", len(s.Examples)),
				Message: fmt.Sprintf("Active skill %s has %d routing examples (minimum 3).", s.ID, len(s.Examples)),
				Fix:     "Add 3–5 natural task phrasings",
			})
		}
	}

	for _, ex := range s.Examples {
		exTokens := tokenize(ex)
		for _, tr := range s.Triggers {
			trTokens := tokenize(tr)
			if overlap(exTokens, trTokens) >= 0.9 {
				findings = append(findings, LintFinding{
					Code:    "example_restates_trigger",
					SkillID: s.ID,
					Field:   "routing.examples",
					Value:   ex,
					Message: fmt.Sprintf("Example %q in skill %s restates trigger %q (overlap >= 0.9).", ex, s.ID, tr),
					Fix:     "Rephrase as a real request",
				})
				break
			}
		}
	}
	return findings
}

func checkPairCollisions(a, b Skill) []LintFinding {
	var findings []LintFinding
	for _, tA := range a.Triggers {
		tokA := tokenize(tA)
		normA := normalizeText(tA)
		for _, tB := range b.Triggers {
			tokB := tokenize(tB)
			normB := normalizeText(tB)
			if normA == normB || overlap(tokA, tokB) >= 0.8 {
				findings = append(findings, LintFinding{
					Code:         "trigger_collision",
					SkillID:      a.ID,
					OtherSkillID: b.ID,
					Field:        "routing.triggers",
					Value:        tA,
					Message:      fmt.Sprintf("Trigger %q of %s collides with trigger %q of %s.", tA, a.ID, tB, b.ID),
					Fix:          "Add `distinguish_from` or narrow one trigger",
				})
				return findings
			}
		}
	}
	return findings
}

func lintSkillPairs(skills []Skill) []LintFinding {
	if len(skills) > maxPairwiseSkills {
		return []LintFinding{{
			Code:    "lint_pairs_skipped",
			Message: "Pair-wise linting skipped because active skills count exceeds 2,000.",
			Fix:     "Inspect smaller subsets of skills",
		}}
	}

	var findings []LintFinding
	descTokens := make([][]string, len(skills))
	for i := range skills {
		descTokens[i] = skillDescriptionAndTriggerTokens(skills[i])
	}

	for i := 0; i < len(skills); i++ {
		for j := i + 1; j < len(skills); j++ {
			a, b := skills[i], skills[j]
			if arePairSuppressed(a, b) {
				continue
			}
			findings = append(findings, checkPairCollisions(a, b)...)
			if overlap(descTokens[i], descTokens[j]) >= 0.6 {
				findings = append(findings, LintFinding{
					Code:         "near_duplicate",
					SkillID:      a.ID,
					OtherSkillID: b.ID,
					Field:        "description",
					Message:      fmt.Sprintf("Skills %s and %s have description and trigger overlap >= 0.6.", a.ID, b.ID),
					Fix:          "Add `distinguish_from` or `equivalent_to`, or merge",
				})
			}
		}
	}
	return findings
}

// LintSkills audits a slice of active skills for routing quality issues.
func LintSkills(skills []Skill) []LintFinding {
	var findings []LintFinding
	for _, s := range skills {
		findings = append(findings, lintSingleSkill(s)...)
	}
	findings = append(findings, lintSkillPairs(skills)...)
	return findings
}
