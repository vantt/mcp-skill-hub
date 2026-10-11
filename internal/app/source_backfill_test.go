package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	createdSourceID := ""
	if srcs, ok := verifyDocA["sources"].([]any); ok && len(srcs) > 0 {
		if sm, ok := srcs[0].(map[string]any); ok {
			createdSourceID, _ = sm["id"].(string)
		}
	} else if verifyProvA, ok := verifyDocA["provenance"].(map[string]any); ok {
		createdSourceID, _ = verifyProvA["source_id"].(string)
	}
	if createdSourceID == "" {
		t.Fatalf("skill-a source_id was not set: %#v", verifyDocA)
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", createdSourceID+".yaml")); err != nil {
		t.Fatalf("created source file %s.yaml missing: %v", createdSourceID, err)
	}

	// 7. Verify Candidate (B) persisted state: origin populated with git repository & commit
	updatedMetaB, _ := os.ReadFile(metaPathB)
	var verifyDocB map[string]any
	_ = yaml.Unmarshal(updatedMetaB, &verifyDocB)
	var repoB, commitB string
	if srcs, ok := verifyDocB["sources"].([]any); ok && len(srcs) > 0 {
		if sm, ok := srcs[0].(map[string]any); ok {
			repoB, _ = sm["repo"].(string)
			if repoB == "" {
				repoB, _ = sm["repository"].(string)
			}
			commitB, _ = sm["commit"].(string)
		}
	} else if verifyProvB, ok := verifyDocB["provenance"].(map[string]any); ok {
		if originB, ok := verifyProvB["origin"].(map[string]any); ok {
			repoB, _ = originB["repository"].(string)
			commitB, _ = originB["commit"].(string)
		}
	}
	if repoB != "https://github.com/example/upstream-b.git" {
		t.Fatalf("skill-b origin repository = %v", repoB)
	}
	if commitB != revB.Value {
		t.Fatalf("skill-b origin commit = %v, want %s", commitB, revB.Value)
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

// writeBackfillSkill creates a skill through the normal lifecycle, then
// replaces its metadata with the given YAML so a test controls the exact shape.
func writeBackfillSkill(t *testing.T, root, id, meta string) string {
	t.Helper()
	ctx := context.Background()
	service := SkillService{}
	preview, err := service.PreviewCreate(ctx, root, skill.CreateInput{ID: id, Collection: "default", Name: id, Description: "Backfill fixture"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(root, "skills", "default", id, ".meta", "skill.yaml")
	if err := os.WriteFile(metaPath, []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	return metaPath
}

// plannedMetaChange returns the planned contents of a skill's metadata file.
func plannedMetaChange(t *testing.T, preview BackfillPreview, id string) []byte {
	t.Helper()
	want := "skills/default/" + id + "/.meta/skill.yaml"
	for _, change := range preview.changes {
		if change.Path == want {
			return change.Contents
		}
	}
	t.Fatalf("no planned change for %s in %#v", want, preview.Diff)
	return nil
}

// lineDelta returns the lines removed from before and added in after,
// compared as multisets, which is what a minimal line diff would show.
func lineDelta(before, after []byte) (removed, added []string) {
	split := func(data []byte) []string { return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") }
	count := map[string]int{}
	for _, line := range split(before) {
		count[line]++
	}
	for _, line := range split(after) {
		if count[line] > 0 {
			count[line]--
			continue
		}
		added = append(added, line)
	}
	for _, line := range split(before) {
		if count[line] > 0 {
			count[line]--
			removed = append(removed, line)
		}
	}
	return removed, added
}

func assertLines(t *testing.T, label string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s lines = %q, want %q", label, got, want)
	}
}

func backfillTestService() BackfillService {
	return BackfillService{Clock: sourceClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}}
}

const backfillDigest = "sha256:0382941395c0d6eca961c5a7005d8034c3d1ce690985e362147e35603d15c71d"

func TestBackfillFillsUpstreamIDAndKeepsOtherEntriesAndProvenance(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	ctx := context.Background()
	meta := `description: Backfill fixture
id: skill-mixed
name: skill-mixed
provenance:
  created_by: skill_add
schema_version: 1
sources:
  - folder_digest: ` + backfillDigest + `
    id: ""
    kind: github
    path: skills/skill-mixed
    ref: main
    repo: https://github.com/example/mixed-upstream
    roles:
      - upstream
  - id: notes-src
    kind: github
    repo: https://github.com/example/notes
    roles:
      - learning
status: draft
`
	metaPath := writeBackfillSkill(t, root, "skill-mixed", meta)
	commitWorkspace(t, root)

	service := backfillTestService()
	preview, err := service.PreviewBackfill(ctx, root, BackfillInput{SkillID: "skill-mixed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 1 || preview.Candidates[0].SourceID != "mixed-upstream" || !preview.Candidates[0].CreateSource {
		t.Fatalf("candidates = %#v", preview.Candidates)
	}
	removed, added := lineDelta([]byte(meta), plannedMetaChange(t, preview, "skill-mixed"))
	assertLines(t, "removed", removed, []string{`    id: ""`})
	assertLines(t, "added", added, []string{`    id: mixed-upstream`})

	if _, err := service.ApplyBackfill(ctx, root, preview); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Provenance map[string]any   `yaml:"provenance"`
		Sources    []map[string]any `yaml:"sources"`
	}
	if err := yaml.Unmarshal(written, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Provenance["created_by"] != "skill_add" {
		t.Fatalf("provenance was not kept: %#v", doc.Provenance)
	}
	if len(doc.Sources) != 2 {
		t.Fatalf("sources = %#v, want both entries", doc.Sources)
	}
	if doc.Sources[0]["id"] != "mixed-upstream" || doc.Sources[0]["folder_digest"] != backfillDigest {
		t.Fatalf("upstream entry = %#v", doc.Sources[0])
	}
	if doc.Sources[1]["id"] != "notes-src" {
		t.Fatalf("learning entry = %#v", doc.Sources[1])
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "mixed-upstream.yaml")); err != nil {
		t.Fatalf("source record was not created: %v", err)
	}
	again, err := service.PreviewBackfill(ctx, root, BackfillInput{SkillID: "skill-mixed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Candidates) != 0 {
		t.Fatalf("re-run candidates = %#v, want none", again.Candidates)
	}
}

// herdrCookPlanMeta mirrors a real skill vendored with an id-less upstream
// entry, written with four-space indentation.
const herdrCookPlanMeta = `id: herdr-cook-plan
quality:
    content_reviewed_digest: sha256:79943a910ec0ec59e05e5abf367e7a38a8ffc99ad14f2dc593392220f1b1b58b
    reviewed: true
routing:
    min_scope: multi_step
    not_for:
        - Creating or writing a plan
        - Full handoff of work to another agent
        - Running outside a Herdr-managed pane
    operations: []
    triggers:
        - Execute an existing AgentKit plan phase by phase through Herdr worker panes
        - Run a plan with one fresh worker agent per phase from inside a Herdr pane
schema_version: 1
sources:
    - commit: 619b8cdea5de1f57d48814c5c87ba623575bb41d
      folder_digest: sha256:0382941395c0d6eca961c5a7005d8034c3d1ce690985e362147e35603d15c71d
      id: ""
      kind: github
      path: herdr-cook-plan
      ref: main
      repo: https://github.com/thieung/herdr-cook-plan
      roles:
        - upstream
      synced: 619b8cdea5de1f57d48814c5c87ba623575bb41d
status: active
`

func TestBackfillFillsIDOfLoneUpstreamEntryWithMinimalDiff(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	ctx := context.Background()
	writeBackfillSkill(t, root, "herdr-cook-plan", herdrCookPlanMeta)
	commitWorkspace(t, root)

	preview, err := backfillTestService().PreviewBackfill(ctx, root, BackfillInput{SkillID: "herdr-cook-plan"})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 1 || preview.Candidates[0].SourceID != "herdr-cook-plan-1" {
		t.Fatalf("candidates = %#v", preview.Candidates)
	}
	removed, added := lineDelta([]byte(herdrCookPlanMeta), plannedMetaChange(t, preview, "herdr-cook-plan"))
	assertLines(t, "removed", removed, []string{`      id: ""`})
	assertLines(t, "added", added, []string{`      id: herdr-cook-plan-1`})
}

func TestBackfillAppendsUpstreamFromProvenanceOriginAndKeepsProvenance(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	ctx := context.Background()
	meta := `description: Backfill fixture
id: skill-origin
name: skill-origin
provenance:
    created_by: skill_add
    origin:
        commit: "1111222233334444555566667777888899990000"
        kind: github
        path: skills/skill-origin
        ref: main
        repository: https://github.com/example/upstream-o.git
schema_version: 1
status: draft
`
	metaPath := writeBackfillSkill(t, root, "skill-origin", meta)
	commitWorkspace(t, root)

	service := backfillTestService()
	preview, err := service.PreviewBackfill(ctx, root, BackfillInput{SkillID: "skill-origin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 1 {
		t.Fatalf("candidates = %#v", preview.Candidates)
	}
	removed, _ := lineDelta([]byte(meta), plannedMetaChange(t, preview, "skill-origin"))
	assertLines(t, "removed", removed, nil)

	if _, err := service.ApplyBackfill(ctx, root, preview); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Provenance struct {
			CreatedBy string         `yaml:"created_by"`
			Origin    map[string]any `yaml:"origin"`
		} `yaml:"provenance"`
		Sources []map[string]any `yaml:"sources"`
	}
	if err := yaml.Unmarshal(written, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Provenance.CreatedBy != "skill_add" || doc.Provenance.Origin["repository"] != "https://github.com/example/upstream-o.git" {
		t.Fatalf("provenance was not kept: %#v", doc.Provenance)
	}
	if len(doc.Sources) != 1 {
		t.Fatalf("sources = %#v, want one appended upstream entry", doc.Sources)
	}
	entry := doc.Sources[0]
	const commit = "1111222233334444555566667777888899990000"
	if entry["id"] != preview.Candidates[0].SourceID || entry["repo"] != "https://github.com/example/upstream-o.git" ||
		entry["commit"] != commit || entry["synced"] != commit {
		t.Fatalf("appended upstream entry = %#v", entry)
	}
	if roles, _ := entry["roles"].([]any); len(roles) != 1 || roles[0] != "upstream" {
		t.Fatalf("appended upstream roles = %#v", entry["roles"])
	}
}

func TestBackfillImportedSkillKeepsProvenanceAndOtherEntries(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	ctx := context.Background()
	rev := revision("rev-imported")
	record := sourcepkg.Record{
		SchemaVersion:   1,
		ID:              "src-git",
		Adapter:         "git",
		Locator:         sourcepkg.Locator{Repository: "https://github.com/example/upstream-b.git", Ref: "main", Path: "skills/skill-b"},
		Status:          "watching",
		Identity:        sourcepkg.Identity{Name: "src-git", Canonical: "https://github.com/example/upstream-b.git", DefaultBranch: "main"},
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Limits:          sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
		CurrentRevision: &rev,
	}
	recordBytes, err := sourcepkg.MarshalCanonical(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", "src-git.yaml"), recordBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `description: Backfill fixture
id: skill-imported
name: skill-imported
provenance:
    created_by: source_import
    path: skills/skill-b
    source_id: src-git
schema_version: 1
sources:
    - id: notes-src
      kind: github
      repo: https://github.com/example/notes
      roles:
        - learning
status: draft
`
	metaPath := writeBackfillSkill(t, root, "skill-imported", meta)
	commitWorkspace(t, root)

	service := backfillTestService()
	preview, err := service.PreviewBackfill(ctx, root, BackfillInput{SkillID: "skill-imported"})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 1 || preview.Candidates[0].Kind != BackfillKindSourceWithoutOrigin {
		t.Fatalf("candidates = %#v", preview.Candidates)
	}
	removed, _ := lineDelta([]byte(meta), plannedMetaChange(t, preview, "skill-imported"))
	assertLines(t, "removed", removed, nil)
	if _, err := service.ApplyBackfill(ctx, root, preview); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Provenance map[string]any   `yaml:"provenance"`
		Sources    []map[string]any `yaml:"sources"`
	}
	if err := yaml.Unmarshal(written, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Provenance["created_by"] != "source_import" || doc.Provenance["source_id"] != "src-git" {
		t.Fatalf("provenance was not kept: %#v", doc.Provenance)
	}
	if len(doc.Sources) != 2 || doc.Sources[0]["id"] != "notes-src" {
		t.Fatalf("sources = %#v, want the learning entry plus the upstream entry", doc.Sources)
	}
	upstream := doc.Sources[1]
	if upstream["id"] != "src-git" || upstream["repo"] != "https://github.com/example/upstream-b.git" || upstream["commit"] != rev.Value {
		t.Fatalf("upstream entry = %#v", upstream)
	}
}
