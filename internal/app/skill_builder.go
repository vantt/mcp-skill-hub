package app

import (
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"gopkg.in/yaml.v3"
)

// buildInitialSkillMetaYAML constructs canonical .meta/skill.yaml for newly added or imported skills.
// It implements D5 and D6:
// - sources[] with id, roles, kind, repo (D2), ref, commit/synced, path, digests, transformations
// - no provenance, history, created_at, updated_at
// - no name or description unless they differ from the SKILL.md frontmatter
func buildInitialSkillMetaYAML(targetID, displayName, displayDescription string, skillMDBytes []byte, src map[string]any) ([]byte, error) {
	metaDoc := map[string]any{
		"schema_version": 1,
		"id":             targetID,
		"status":         "draft",
		"routing": map[string]any{
			"triggers":  []string{},
			"not_for":   []string{},
			"min_scope": "",
		},
		"quality": map[string]any{
			"reviewed": false,
		},
	}

	fm, _ := catalog.ParseSkillMD(skillMDBytes)
	fmName := ""
	fmDesc := ""
	if fm != nil {
		if n, ok := fm["name"].(string); ok {
			fmName = strings.TrimSpace(n)
		}
		if d, ok := fm["description"].(string); ok {
			fmDesc = strings.TrimSpace(d)
		}
	}
	trimmedName := strings.TrimSpace(displayName)
	if trimmedName != "" && (fmName == "" || trimmedName != fmName) {
		metaDoc["name"] = trimmedName
	}
	trimmedDesc := strings.TrimSpace(displayDescription)
	if trimmedDesc != "" && (fmDesc == "" || trimmedDesc != fmDesc) {
		metaDoc["description"] = trimmedDesc
	}

	if src != nil {
		metaDoc["sources"] = []any{src}
	}

	return yaml.Marshal(metaDoc)
}
