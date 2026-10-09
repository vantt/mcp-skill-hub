package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

func TestBackfill(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	skillService := SkillService{}
	ctx := context.Background()

	// 1. Setup Candidate (A): skill vendored with github origin, no source_id, no source in catalog
	prevA, err := skillService.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "skill-a",
		Collection:  "default",
		Name:        "Skill A",
		Description: "Vendored without source",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillService.ConfirmSkillMutation(ctx, root, prevA, prevA.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	metaPathA := filepath.Join(root, "skills", "default", "skill-a", ".meta", "skill.yaml")
	if _, err := os.Stat(metaPathA); os.IsNotExist(err) {
		metaPathA = filepath.Join(root, "skills", "default", "skill-a", "skill.meta.yaml")
	}
	metaBytesA, _ := os.ReadFile(metaPathA)
	var docA map[string]any
	_ = yaml.Unmarshal(metaBytesA, &docA)
	if docA == nil {
		docA = make(map[string]any)
	}
	docA["provenance"] = map[string]any{
		"created_by": "skill_add",
		"origin": map[string]any{
			"kind":       "github",
			"repository": "https://github.com/example/upstream-a.git",
			"ref":        "main",
			"path":       "skills/skill-a",
			"commit":     "1111222233334444555566667777888899990000",
		},
	}
	newMetaA, _ := yaml.Marshal(docA)
	_ = os.WriteFile(metaPathA, newMetaA, 0o644)

	// 2. Setup Candidate (B): skill imported from git source, has source_id, created_by: source_import, no origin
	// Create git source record in catalog
	revB := revision("rev-b")
	sourceGitRec := sourcepkg.Record{
		SchemaVersion:   1,
		ID:              "src-git",
		Adapter:         "git",
		Locator:         sourcepkg.Locator{Repository: "https://github.com/example/upstream-b.git", Ref: "main", Path: "skills/skill-b"},
		Status:          "watching",
		Identity:        sourcepkg.Identity{Name: "src-git", Canonical: "https://github.com/example/upstream-b.git", DefaultBranch: "main"},
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Limits:          sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
		CurrentRevision: &revB,
	}
	sourceGitBytes, _ := sourcepkg.MarshalCanonical(sourceGitRec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "src-git.yaml"), sourceGitBytes, 0o644)

	prevB, err := skillService.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "skill-b",
		Collection:  "default",
		Name:        "Skill B",
		Description: "Imported from git without origin",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillService.ConfirmSkillMutation(ctx, root, prevB, prevB.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	metaPathB := filepath.Join(root, "skills", "default", "skill-b", ".meta", "skill.yaml")
	if _, err := os.Stat(metaPathB); os.IsNotExist(err) {
		metaPathB = filepath.Join(root, "skills", "default", "skill-b", "skill.meta.yaml")
	}
	metaBytesB, _ := os.ReadFile(metaPathB)
	var docB map[string]any
	_ = yaml.Unmarshal(metaBytesB, &docB)
	if docB == nil {
		docB = make(map[string]any)
	}
	docB["provenance"] = map[string]any{
		"created_by": "source_import",
		"source_id":  "src-git",
		"path":       "skills/skill-b",
	}
	newMetaB, _ := yaml.Marshal(docB)
	_ = os.WriteFile(metaPathB, newMetaB, 0o644)

	// 3. Setup Non-Candidate: skill imported from filesystem source, created_by: source_import, no origin
	sourceFsRec := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            "src-fs",
		Adapter:       "filesystem",
		Locator:       sourcepkg.Locator{Path: "sources/local"},
		Status:        "watching",
		Identity:      sourcepkg.Identity{Name: "src-fs", Canonical: "sources/local"},
		Monitoring:    sourcepkg.Monitoring{Enabled: false, Cadence: "manual"},
		Limits:        sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
	}
	sourceFsBytes, _ := sourcepkg.MarshalCanonical(sourceFsRec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "src-fs.yaml"), sourceFsBytes, 0o644)

	prevFS, err := skillService.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "skill-fs",
		Collection:  "default",
		Name:        "Skill FS",
		Description: "Imported from filesystem without origin",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillService.ConfirmSkillMutation(ctx, root, prevFS, prevFS.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	metaPathFS := filepath.Join(root, "skills", "default", "skill-fs", ".meta", "skill.yaml")
	if _, err := os.Stat(metaPathFS); os.IsNotExist(err) {
		metaPathFS = filepath.Join(root, "skills", "default", "skill-fs", "skill.meta.yaml")
	}
	metaBytesFS, _ := os.ReadFile(metaPathFS)
	var docFS map[string]any
	_ = yaml.Unmarshal(metaBytesFS, &docFS)
	if docFS == nil {
		docFS = make(map[string]any)
	}
	docFS["provenance"] = map[string]any{
		"created_by": "source_import",
		"source_id":  "src-fs",
		"path":       "skills/skill-fs",
	}
	newMetaFS, _ := yaml.Marshal(docFS)
	_ = os.WriteFile(metaPathFS, newMetaFS, 0o644)

	commitWorkspace(t, root)

	service := BackfillService{
		Clock: sourceClock{now: now},
	}

	// 4. Preview backfill
	preview, err := service.PreviewBackfill(ctx, root, BackfillInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 2 {
		t.Fatalf("expected exactly 2 candidates (skill-a, skill-b), got %d: %#v", len(preview.Candidates), preview.Candidates)
	}

	var foundA, foundB bool
	for _, c := range preview.Candidates {
		if c.SkillID == "skill-a" {
			foundA = true
			if c.Kind != BackfillKindOriginWithoutSource {
				t.Fatalf("skill-a kind = %v, want origin_without_source", c.Kind)
			}
			if !c.CreateSource {
				t.Fatalf("skill-a expected CreateSource: true")
			}
		}
		if c.SkillID == "skill-b" {
			foundB = true
			if c.Kind != BackfillKindSourceWithoutOrigin {
				t.Fatalf("skill-b kind = %v, want source_without_origin", c.Kind)
			}
			if c.CreateSource {
				t.Fatalf("skill-b expected CreateSource: false")
			}
		}
		if c.SkillID == "skill-fs" {
			t.Fatalf("skill-fs should NOT be a backfill candidate")
		}
	}
	if !foundA || !foundB {
		t.Fatalf("missing candidate A or B in preview: %#v", preview.Candidates)
	}

	// 5. Apply backfill
	result, err := service.ApplyBackfill(ctx, root, preview)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.UpdatedSkills) != 2 {
		t.Fatalf("updated skills = %v, want 2", result.UpdatedSkills)
	}
	if len(result.CreatedSources) != 1 {
		t.Fatalf("created sources = %v, want 1", result.CreatedSources)
	}

	// 6. Verify Candidate (A) persisted state: source created and skill.meta.yaml has provenance.source_id
	updatedMetaA, _ := os.ReadFile(metaPathA)
	var verifyDocA map[string]any
	_ = yaml.Unmarshal(updatedMetaA, &verifyDocA)
	verifyProvA := verifyDocA["provenance"].(map[string]any)
	createdSourceID := verifyProvA["source_id"].(string)
	if createdSourceID == "" {
		t.Fatalf("skill-a source_id was not set")
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", createdSourceID+".yaml")); err != nil {
		t.Fatalf("created source file %s.yaml missing: %v", createdSourceID, err)
	}

	// 7. Verify Candidate (B) persisted state: origin populated with git repository & commit
	updatedMetaB, _ := os.ReadFile(metaPathB)
	var verifyDocB map[string]any
	_ = yaml.Unmarshal(updatedMetaB, &verifyDocB)
	verifyProvB := verifyDocB["provenance"].(map[string]any)
	originB, ok := verifyProvB["origin"].(map[string]any)
	if !ok || originB == nil {
		t.Fatalf("skill-b origin was not populated: %#v", verifyProvB)
	}
	if originB["repository"] != "https://github.com/example/upstream-b.git" {
		t.Fatalf("skill-b origin repository = %v", originB["repository"])
	}
	if originB["commit"] != revB.Value {
		t.Fatalf("skill-b origin commit = %v, want %s", originB["commit"], revB.Value)
	}

	// 8. Verify Filesystem skill remains untouched
	updatedMetaFS, _ := os.ReadFile(metaPathFS)
	var verifyDocFS map[string]any
	_ = yaml.Unmarshal(updatedMetaFS, &verifyDocFS)
	verifyProvFS := verifyDocFS["provenance"].(map[string]any)
	if verifyProvFS["origin"] != nil {
		t.Fatalf("skill-fs should not have origin populated: %#v", verifyProvFS)
	}

	// 9. Verify Idempotence: re-running backfill finds 0 candidates
	preview2, err := service.PreviewBackfill(ctx, root, BackfillInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview2.Candidates) != 0 {
		t.Fatalf("expected 0 candidates on re-run, got %d: %#v", len(preview2.Candidates), preview2.Candidates)
	}
	res2, err := service.ApplyBackfill(ctx, root, preview2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.UpdatedSkills) != 0 || len(res2.CreatedSources) != 0 {
		t.Fatalf("expected 0 updated/created on re-run, got %#v", res2)
	}
}
