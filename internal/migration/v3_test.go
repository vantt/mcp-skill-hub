package migration

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestMigratedSkillYAMLFields(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("2\n"), 0o644)

	// 1. Skill with custom name differing from SKILL.md frontmatter
	skill1Dir := filepath.Join(root, "skills", "default", "custom-name-skill")
	_ = os.MkdirAll(skill1Dir, 0o755)
	_ = os.WriteFile(filepath.Join(skill1Dir, "SKILL.md"), []byte("---\nname: custom-name-skill\ndescription: Frontmatter desc\n---\n# Title\n"), 0o644)
	meta1 := `schema_version: 1
id: custom-name-skill
name: Custom Name Display
description: Old metadata desc
collection: default
collection_id: default
status: active
history:
  - occurred_at: "2026-10-04T08:08:00Z"
    state: active
created_at: "2026-10-04T08:08:00Z"
updated_at: "2026-10-04T08:08:00Z"
quality:
  reviewed: true
  curated: false
  content_reviewed_digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  routing_review_rationale: "Approved"
routing:
  triggers: [custom]
  operations: [review]
  not_for: [other]
  min_scope: single_step
`
	_ = os.WriteFile(filepath.Join(skill1Dir, "skill.meta.yaml"), []byte(meta1), 0o644)

	skill2Dir := filepath.Join(root, "skills", "default", "matching-name-skill")
	_ = os.MkdirAll(skill2Dir, 0o755)
	_ = os.WriteFile(filepath.Join(skill2Dir, "SKILL.md"), []byte("---\nname: matching-name-skill\ndescription: Matching desc\n---\n# Matching Name\n"), 0o644)
	meta2 := `schema_version: 1
id: matching-name-skill
name: matching-name-skill
description: Matching desc
status: draft
quality:
  reviewed: false
  curated: true
routing:
  triggers: []
  operations: []
  not_for: []
  min_scope: ""
`
	_ = os.WriteFile(filepath.Join(skill2Dir, "skill.meta.yaml"), []byte(meta2), 0o644)

	reg := DefaultRegistry()
	proposal, err := reg.Preview(root, CurrentVersion)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	_, err = mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}

	// Assert fields of skill 1 (custom name differing from frontmatter)
	meta1Path := filepath.Join(skill1Dir, ".meta", "skill.yaml")
	data1, err := os.ReadFile(meta1Path)
	if err != nil {
		t.Fatalf("read migrated skill 1: %v", err)
	}
	var raw1 map[string]any
	if err := yaml.Unmarshal(data1, &raw1); err != nil {
		t.Fatalf("unmarshal migrated skill 1: %v", err)
	}

	// schema_version is 1
	if raw1["schema_version"] != 1 && raw1["schema_version"] != "1" {
		t.Fatalf("expected schema_version 1, got %v", raw1["schema_version"])
	}
	if raw1["id"] != "custom-name-skill" {
		t.Fatalf("expected id custom-name-skill, got %v", raw1["id"])
	}
	if raw1["status"] != "active" {
		t.Fatalf("expected status active, got %v", raw1["status"])
	}
	// name is preserved because it differs from SKILL.md frontmatter name
	if raw1["name"] != "Custom Name Display" {
		t.Fatalf("expected name preserved as Custom Name Display, got %v", raw1["name"])
	}
	// Derived/deleted fields are omitted
	if _, ok := raw1["description"]; ok {
		t.Fatal("expected description to be omitted (derived from SKILL.md)")
	}
	if _, ok := raw1["collection"]; ok {
		t.Fatal("expected collection to be omitted (derived from path)")
	}
	if _, ok := raw1["collection_id"]; ok {
		t.Fatal("expected collection_id to be omitted (derived from path)")
	}
	if _, ok := raw1["history"]; ok {
		t.Fatal("expected history to be omitted (deleted, Git is history)")
	}
	if _, ok := raw1["created_at"]; ok {
		t.Fatal("expected created_at to be omitted (deleted)")
	}
	if _, ok := raw1["updated_at"]; ok {
		t.Fatal("expected updated_at to be omitted (deleted)")
	}
	// Quality checks
	q1, ok := raw1["quality"].(map[string]any)
	if !ok {
		t.Fatalf("expected quality map, got %v", raw1["quality"])
	}
	if q1["reviewed"] != true {
		t.Fatalf("expected quality.reviewed true, got %v", q1["reviewed"])
	}
	if _, ok := q1["curated"]; ok {
		t.Fatal("expected quality.curated to be omitted (deleted)")
	}
	if q1["content_reviewed_digest"] != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected content_reviewed_digest kept, got %v", q1["content_reviewed_digest"])
	}

	// Assert fields of skill 2 (matching name)
	meta2Path := filepath.Join(skill2Dir, ".meta", "skill.yaml")
	data2, err := os.ReadFile(meta2Path)
	if err != nil {
		t.Fatalf("read migrated skill 2: %v", err)
	}
	var raw2 map[string]any
	if err := yaml.Unmarshal(data2, &raw2); err != nil {
		t.Fatalf("unmarshal migrated skill 2: %v", err)
	}

	if raw2["schema_version"] != 1 && raw2["schema_version"] != "1" {
		t.Fatalf("expected schema_version 1, got %v", raw2["schema_version"])
	}
	// name is omitted because it matches SKILL.md frontmatter name
	if _, ok := raw2["name"]; ok {
		t.Fatalf("expected name to be omitted when matching frontmatter name, got %v", raw2["name"])
	}
	q2, ok := raw2["quality"].(map[string]any)
	if !ok {
		t.Fatalf("expected quality map for skill 2, got %v", raw2["quality"])
	}
	if q2["reviewed"] != false {
		t.Fatalf("expected quality.reviewed false, got %v", q2["reviewed"])
	}
	if _, ok := q2["curated"]; ok {
		t.Fatal("expected quality.curated to be omitted in skill 2")
	}
}
