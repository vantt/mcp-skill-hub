package skill

import (
	"regexp"
	"strings"
)

// DefaultSuggestedMinScope is the scope a suggestion starts with: the narrow
// end, so a skill is never offered for more than it was written for.
const DefaultSuggestedMinScope = "single_step"

var (
	triggerPrefixes = []string{"use this skill when ", "use this skill for ", "use this when ", "use this for ", "use when ", "use for "}
	notForPrefixes  = []string{
		"do not use this skill for ", "do not use this skill when ", "do not use for ", "do not use when ", "do not use ",
		"don't use this skill for ", "don't use this skill when ", "don't use for ", "don't use when ", "don't use ",
		"not for ", "not when ",
	}
	sentencePattern = regexp.MustCompile(`[^.!?\n]+`)
	listItemPattern = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(.*\S)\s*$`)
)

// SuggestRouting proposes routing from what the author already wrote: the
// "Use when ..." and "Do not use ..." sentences of the description, and the
// list items under "When to use", "When not to use" and "Examples" headings of
// SKILL.md. It is deterministic and makes no model call, so a person can edit
// the result, never argue with it.
func SuggestRouting(description, skillMD string) RoutingInput {
	var suggestion RoutingInput
	for _, sentence := range sentencePattern.FindAllString(description, -1) {
		sentence = strings.TrimSpace(sentence)
		lower := strings.ToLower(sentence)
		if phrase, ok := afterPrefix(sentence, lower, notForPrefixes); ok {
			suggestion.NotFor = append(suggestion.NotFor, splitPhrases(phrase)...)
		} else if phrase, ok := afterPrefix(sentence, lower, triggerPrefixes); ok {
			suggestion.Triggers = append(suggestion.Triggers, splitPhrases(phrase)...)
		}
	}
	section := ""
	for _, line := range strings.Split(skillMD, "\n") {
		if heading, isHeading := strings.CutPrefix(strings.TrimSpace(line), "#"); isHeading {
			section = strings.ToLower(strings.Trim(heading, "# \t"))
			continue
		}
		item := listItemPattern.FindStringSubmatch(line)
		if item == nil {
			continue
		}
		switch section {
		case "when to use":
			suggestion.Triggers = append(suggestion.Triggers, item[1])
		case "when not to use":
			suggestion.NotFor = append(suggestion.NotFor, item[1])
		case "examples", "example":
			suggestion.Examples = append(suggestion.Examples, item[1])
		}
	}
	suggestion.Triggers = uniquePhrases(suggestion.Triggers)
	suggestion.NotFor = uniquePhrases(suggestion.NotFor)
	suggestion.Examples = uniquePhrases(suggestion.Examples)
	suggestion.MinScope = DefaultSuggestedMinScope
	return suggestion
}

func afterPrefix(sentence, lower string, prefixes []string) (string, bool) {
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return sentence[len(prefix):], true
		}
	}
	return "", false
}

// splitPhrases cuts "a, b or c" into separate phrases. It never splits on
// "and": "lint and format" is one request.
func splitPhrases(text string) []string {
	var phrases []string
	for _, part := range strings.Split(text, ",") {
		for _, piece := range strings.Split(part, " or ") {
			piece = strings.Trim(strings.TrimSpace(piece), "\"'`.")
			piece = strings.TrimPrefix(piece, "or ")
			if piece != "" {
				phrases = append(phrases, piece)
			}
		}
	}
	return phrases
}

func uniquePhrases(phrases []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, phrase := range phrases {
		key := strings.ToLower(phrase)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, phrase)
		}
	}
	return unique
}
