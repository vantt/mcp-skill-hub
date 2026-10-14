package app

import (
	"fmt"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/skill"
	"gopkg.in/yaml.v3"
)

// The commit changes hub metadata in two places only: routing, which the person
// accepts or edits in the preview, and the content approval of a third-party
// skill. Every other field, lifecycle and origin included, stays as the hub has
// it, whatever a project folder claims.

func decodeMeta(meta []byte) (map[string]any, error) {
	var document map[string]any
	if err := yaml.Unmarshal(meta, &document); err != nil {
		return nil, fmt.Errorf("read skill metadata: %w", err)
	}
	if document == nil {
		document = map[string]any{}
	}
	return document, nil
}

func encodeMeta(document map[string]any) ([]byte, error) {
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode skill metadata: %w", err)
	}
	return encoded, nil
}

// metaRouting reads the routing block of hub metadata.
func metaRouting(meta []byte) skill.RoutingInput {
	var document struct {
		Routing skill.RoutingInput `yaml:"routing"`
	}
	_ = yaml.Unmarshal(meta, &document)
	return document.Routing
}

// setMetaRouting replaces the routing fields of hub metadata with routing.
func setMetaRouting(meta []byte, routing skill.RoutingInput) ([]byte, error) {
	document, err := decodeMeta(meta)
	if err != nil {
		return nil, err
	}
	block, _ := document["routing"].(map[string]any)
	if block == nil {
		block = map[string]any{}
	}
	block["operations"] = cleanPhrases(routing.Operations)
	block["triggers"] = cleanPhrases(routing.Triggers)
	block["not_for"] = cleanPhrases(routing.NotFor)
	block["min_scope"] = strings.TrimSpace(routing.MinScope)
	for key, values := range map[string][]string{"examples": routing.Examples, "counter_examples": routing.CounterExamples} {
		if cleaned := cleanPhrases(values); len(cleaned) > 0 {
			block[key] = cleaned
		} else {
			delete(block, key)
		}
	}
	document["routing"] = block
	return encodeMeta(document)
}

// setMetaApproval records a human approval of the whole content.
func setMetaApproval(meta []byte, digest string) ([]byte, error) {
	document, err := decodeMeta(meta)
	if err != nil {
		return nil, err
	}
	quality, _ := document["quality"].(map[string]any)
	if quality == nil {
		quality = map[string]any{}
	}
	quality["content_reviewed_digest"] = digest
	document["quality"] = quality
	return encodeMeta(document)
}

// overrideRouting replaces each routing field the override sets and keeps the
// rest, so explicit flags win over suggestions and over what the hub has.
func overrideRouting(base, override skill.RoutingInput) skill.RoutingInput {
	merged := base
	if len(override.Operations) > 0 {
		merged.Operations = override.Operations
	}
	if len(override.Triggers) > 0 {
		merged.Triggers = override.Triggers
	}
	if len(override.NotFor) > 0 {
		merged.NotFor = override.NotFor
	}
	if len(override.Examples) > 0 {
		merged.Examples = override.Examples
	}
	if len(override.CounterExamples) > 0 {
		merged.CounterExamples = override.CounterExamples
	}
	if strings.TrimSpace(override.MinScope) != "" {
		merged.MinScope = override.MinScope
	}
	return merged
}

func sameRouting(a, b skill.RoutingInput) bool {
	return strings.Join(cleanPhrases(a.Operations), "\x00") == strings.Join(cleanPhrases(b.Operations), "\x00") &&
		strings.Join(cleanPhrases(a.Triggers), "\x00") == strings.Join(cleanPhrases(b.Triggers), "\x00") &&
		strings.Join(cleanPhrases(a.NotFor), "\x00") == strings.Join(cleanPhrases(b.NotFor), "\x00") &&
		strings.Join(cleanPhrases(a.Examples), "\x00") == strings.Join(cleanPhrases(b.Examples), "\x00") &&
		strings.Join(cleanPhrases(a.CounterExamples), "\x00") == strings.Join(cleanPhrases(b.CounterExamples), "\x00") &&
		strings.TrimSpace(a.MinScope) == strings.TrimSpace(b.MinScope)
}

func cleanPhrases(values []string) []string {
	cleaned := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}
