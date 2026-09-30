package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// MaxDistributedSkillResources bounds the files one skill may expose over distribution.
	MaxDistributedSkillResources = 512
	// MaxDistributedSkillBytes bounds the total bytes one skill may expose over distribution.
	MaxDistributedSkillBytes = 16 << 20
)

// ErrSkillNotServable marks an active skill whose files cannot be distributed.
var ErrSkillNotServable = errors.New("skill cannot be served")

// ParseSkillFrontmatter decodes SKILL.md frontmatter into the JSON data model
// carried by skill distribution.
func ParseSkillFrontmatter(contents []byte) (map[string]any, error) {
	if !bytes.HasPrefix(contents, []byte("---\n")) {
		return nil, errors.New("SKILL.md must begin with YAML frontmatter")
	}
	end := bytes.Index(contents[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, errors.New("SKILL.md frontmatter is not terminated")
	}
	var value map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(contents[4 : 4+end]))
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	if value == nil {
		return nil, errors.New("SKILL.md frontmatter must be an object")
	}
	// Round-trip through JSON to reject YAML-only map keys and normalize values
	// to the exact JSON data model carried by SEP-2640.
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("frontmatter is not JSON-compatible: %w", err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// ValidateServableSkill applies the invariants skill distribution enforces so an
// active skill that could never be served is rejected when the catalog is built.
func ValidateServableSkill(id string, entrypoint []byte, resourceCount int, totalBytes int64) error {
	if resourceCount < 1 || resourceCount > MaxDistributedSkillResources {
		return fmt.Errorf("%w: skill %s has %d distributed resources; supported range is 1..%d", ErrSkillNotServable, id, resourceCount, MaxDistributedSkillResources)
	}
	if totalBytes > MaxDistributedSkillBytes {
		return fmt.Errorf("%w: skill %s exceeds the %d-byte distribution limit", ErrSkillNotServable, id, MaxDistributedSkillBytes)
	}
	frontmatter, err := ParseSkillFrontmatter(entrypoint)
	if err != nil {
		return fmt.Errorf("%w: skill %s is not distributable: %v", ErrSkillNotServable, id, err)
	}
	name, _ := frontmatter["name"].(string)
	description, _ := frontmatter["description"].(string)
	if name != id || strings.TrimSpace(description) == "" {
		return fmt.Errorf("%w: skill %s frontmatter must contain name %q and a non-empty description", ErrSkillNotServable, id, id)
	}
	return nil
}

// servableSkillWarnings reports every active skill that distribution cannot
// serve. Such skills stay published (a hand edit must not block unrelated
// work) but are listed so the author can repair them; distribution and
// resolution skip them instead of failing for every other skill.
func servableSkillWarnings(input buildInput) []string {
	var warnings []string
	for _, item := range input.Entities {
		if item.Kind != "skill" || stringField(item.Document, "status") != "active" {
			continue
		}
		directory := item.Path[:strings.LastIndex(item.Path, "/")]
		var entrypoint []byte
		count, total := 0, int64(0)
		for _, file := range input.Files {
			if !strings.HasPrefix(file.Path, directory+"/") || file.Path == directory+"/skill.meta.yaml" {
				continue
			}
			count++
			total += int64(len(file.Bytes))
			if file.Path == directory+"/SKILL.md" {
				entrypoint = file.Bytes
			}
		}
		if entrypoint == nil {
			warnings = append(warnings, fmt.Sprintf("%s: skill %s will not be served: %s/SKILL.md is missing", directory, item.ID, directory))
			continue
		}
		if err := ValidateServableSkill(item.ID, entrypoint, count, total); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v; fix SKILL.md frontmatter and rebuild, or the skill is omitted from skills/list", directory, err))
		}
	}
	return warnings
}
