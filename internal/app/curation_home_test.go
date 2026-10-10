package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

type curationFixture struct {
	Scenario string `yaml:"scenario"`
	Given    struct {
		Workspace struct {
			Health   string `yaml:"health"`
			Index    string `yaml:"index"`
			GitDirty bool   `yaml:"git_dirty"`
		} `yaml:"workspace"`
		DiagnosticID     string `yaml:"diagnostic_id"`
		InterruptedRunID string `yaml:"interrupted_run_id"`
		Counts           struct {
			ActiveSkills             int `yaml:"active_skills"`
			WatchingSources          int `yaml:"watching_sources"`
			FailedRuns               int `yaml:"failed_runs"`
			InterruptedRuns          int `yaml:"interrupted_runs"`
			ChangedSources           int `yaml:"changed_sources"`
			PendingInsights          int `yaml:"pending_insights"`
			PendingHighValueInsights int `yaml:"pending_high_value_insights"`
		} `yaml:"counts"`
	} `yaml:"given"`
	When struct {
		Intent             string `yaml:"intent"`
		ApplicationCommand string `yaml:"application_command"`
	} `yaml:"when"`
	Expect struct {
		Status           Status   `yaml:"status"`
		Summary          string   `yaml:"summary"`
		SuggestedActions []string `yaml:"suggested_actions"`
		Confirmation     struct {
			PolicyRevision     string `yaml:"policy_revision"`
			ActionClass        string `yaml:"action_class"`
			ApplicationCommand string `yaml:"application_command"`
			Confirmation       struct {
				Required bool   `yaml:"required"`
				Mode     string `yaml:"mode"`
			} `yaml:"confirmation"`
		} `yaml:"confirmation"`
		ProgressiveDisclosure struct {
			L0 struct {
				Counts            map[string]int `yaml:"counts"`
				RecommendedAction string         `yaml:"recommended_action"`
			} `yaml:"L0"`
			L1 struct {
				Items  []Item `yaml:"items"`
				Impact string `yaml:"impact"`
			} `yaml:"L1"`
			L2 struct {
				Evidence   []map[string]any `yaml:"evidence"`
				Comparison any              `yaml:"comparison"`
			} `yaml:"L2"`
			L3 struct {
				CanonicalArtifacts []string `yaml:"canonical_artifacts"`
				Diagnostics        []string `yaml:"diagnostics"`
			} `yaml:"L3"`
		} `yaml:"progressive_disclosure"`
		Error *struct {
			Error string `yaml:"ERROR"`
			Why   string `yaml:"WHY"`
			Fix   string `yaml:"FIX"`
		} `yaml:"error"`
	} `yaml:"expect"`
}

func TestCurationHomePhaseZeroFixtures(t *testing.T) {
	t.Parallel()
	fixtures, err := filepath.Glob(filepath.Join("..", "..", "testdata", "ux", "curation-home", "*.yaml"))
	if err != nil || len(fixtures) != 3 {
		t.Fatalf("curation fixtures = %v, %v", fixtures, err)
	}
	for _, path := range fixtures {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var fixture curationFixture
			if err := yaml.Unmarshal(contents, &fixture); err != nil {
				t.Fatal(err)
			}
			state := homeState{
				Health: fixture.Given.Workspace.Health, Index: fixture.Given.Workspace.Index,
				GitDirty: fixture.Given.Workspace.GitDirty, GitConfigured: true,
				DiagnosticID: fixture.Given.DiagnosticID, InterruptedID: fixture.Given.InterruptedRunID,
				ChangedSources: fixture.Given.Counts.ChangedSources,
				Summary: CurationSummary{
					ActiveSkills: fixture.Given.Counts.ActiveSkills, WatchingSources: fixture.Given.Counts.WatchingSources,
					FailedOrInterruptedRuns:  fixture.Given.Counts.FailedRuns + fixture.Given.Counts.InterruptedRuns,
					PendingInsights:          fixture.Given.Counts.PendingInsights,
					PendingHighValueInsights: fixture.Given.Counts.PendingHighValueInsights,
				},
			}
			if fixture.Given.InterruptedRunID != "" {
				state.RecoveryPending = false // A distill run is recoverable work, not a mutation journal.
			}
			if fixture.Expect.Error != nil {
				state.InvalidReason = fixture.Expect.Error.Why
			}
			home := deriveCurationHome(state)
			if home.Status != fixture.Expect.Status || home.Summary != fixture.Expect.Summary {
				t.Fatalf("home status/summary = %q/%q, want %q/%q", home.Status, home.Summary, fixture.Expect.Status, fixture.Expect.Summary)
			}
			if len(home.SuggestedActions) != 1 || len(fixture.Expect.SuggestedActions) != 1 || home.SuggestedActions[0].Label != fixture.Expect.SuggestedActions[0] || home.SuggestedActions[0].Label != fixture.Expect.ProgressiveDisclosure.L0.RecommendedAction {
				t.Fatalf("suggested actions = %#v, want %#v", home.SuggestedActions, fixture.Expect.SuggestedActions)
			}
			if fixture.When.Intent == "" || fixture.When.ApplicationCommand != "GetCurationHome" || fixture.Expect.Confirmation.PolicyRevision != "policy_v1" || fixture.Expect.Confirmation.ActionClass != "read-only" || fixture.Expect.Confirmation.ApplicationCommand != fixture.When.ApplicationCommand || fixture.Expect.Confirmation.Confirmation.Required || fixture.Expect.Confirmation.Confirmation.Mode != "none" {
				t.Fatalf("fixture has incompatible GetCurationHome revision/confirmation contract: when=%#v confirmation=%#v", fixture.When, fixture.Expect.Confirmation)
			}
			actualCounts := map[string]int{
				"attention_items":    home.HomeSummary.AttentionItems,
				"invalid_workspaces": home.HomeSummary.InvalidWorkspaces,
				"recovery_items":     home.HomeSummary.RecoveryItems,
				"optional_items":     home.HomeSummary.OptionalItems,
			}
			for key, expected := range fixture.Expect.ProgressiveDisclosure.L0.Counts {
				if actualCounts[key] != expected {
					t.Fatalf("L0 count %s = %d, want %d", key, actualCounts[key], expected)
				}
			}
			if !reflect.DeepEqual(home.Items, fixture.Expect.ProgressiveDisclosure.L1.Items) {
				t.Fatalf("L1 items = %#v, want %#v", home.Items, fixture.Expect.ProgressiveDisclosure.L1.Items)
			}
			if (len(fixture.Expect.ProgressiveDisclosure.L1.Items) == 0 && fixture.Expect.ProgressiveDisclosure.L1.Impact == "") || fixture.Expect.ProgressiveDisclosure.L2.Evidence == nil || fixture.Expect.ProgressiveDisclosure.L3.CanonicalArtifacts == nil || fixture.Expect.ProgressiveDisclosure.L3.Diagnostics == nil {
				t.Fatalf("fixture omits a progressive disclosure level: %#v", fixture.Expect.ProgressiveDisclosure)
			}
			for _, item := range fixture.Expect.ProgressiveDisclosure.L1.Items {
				if item.Impact == "" {
					t.Fatalf("L1 item %q omits impact", item.ID)
				}
			}
			assertFixtureEvidenceReferences(t, fixture)
			if fixture.Expect.Error != nil {
				if home.Error == nil || home.Error.Render.Error != fixture.Expect.Error.Error || home.Error.Render.Why != fixture.Expect.Error.Why || home.Error.Render.Fix != fixture.Expect.Error.Fix {
					t.Fatalf("error = %#v, want %#v", home.Error, fixture.Expect.Error)
				}
			}
		})
	}
}

func assertFixtureEvidenceReferences(t *testing.T, fixture curationFixture) {
	t.Helper()
	itemIDs := make(map[string]bool)
	for _, item := range fixture.Expect.ProgressiveDisclosure.L1.Items {
		itemIDs[item.ID] = true
	}
	for _, evidence := range fixture.Expect.ProgressiveDisclosure.L2.Evidence {
		for _, key := range []string{"diagnostic_id", "run_id"} {
			if id, ok := evidence[key].(string); ok && !itemIDs[id] {
				t.Fatalf("L2 %s %q does not reference an L1 item", key, id)
			}
		}
	}
	for _, id := range fixture.Expect.ProgressiveDisclosure.L3.CanonicalArtifacts {
		if !itemIDs[id] {
			t.Fatalf("L3 canonical artifact %q does not reference an L1 item", id)
		}
	}
}

func TestCurationHomePrioritizesInvalidRecoveryStaleAndGit(t *testing.T) {
	t.Parallel()
	home := deriveCurationHome(homeState{
		Health: "invalid", Index: "stale", GitDirty: true, GitConfigured: true,
		RecoveryPending: true, RecoveryID: "op-recovery",
		Summary:       CurationSummary{FailedOrInterruptedRuns: 1, PendingHighValueInsights: 2},
		InterruptedID: "run-interrupted", ChangedSources: 3,
	})
	if len(home.SuggestedActions) != 1 || home.Actions[0].Kind != "repair_workspace" || home.Actions[0].Count != 2 || home.Summary != "Workspace needs repair before other curation work." {
		t.Fatalf("priority result = %#v", home)
	}
	for _, action := range home.Actions {
		if action.Kind == "recover_workspace" {
			t.Fatalf("repair and recovery actions were not coalesced: %#v", home.Actions)
		}
	}
	if len(home.Items) < 2 || home.Items[0].ID == home.Items[1].ID {
		t.Fatalf("coalescing lost diagnostics: %#v", home.Items)
	}
	for index := 1; index < len(home.Actions); index++ {
		if home.Actions[index-1].Priority < home.Actions[index].Priority {
			t.Fatalf("actions are not priority ordered: %#v", home.Actions)
		}
	}
}

func TestGetCurationHomePrioritizesStaleIndexBeforeGitChanges(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	commitWorkspace(t, root)
	path := filepath.Join(root, "skills", "testing", "sample", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(filepath.Dir(path), "skill.meta.yaml")
	if err := os.WriteFile(metadata, []byte("schema_version: 1\nid: sample\nname: Sample\nstatus: draft\ndescription: Sample draft.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home, err := (CurationService{}).GetCurationHome(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !home.Workspace.GitDirty || home.Workspace.Index != "stale" || home.Actions[0].Kind != "rebuild_index" || len(home.SuggestedActions) != 1 {
		t.Fatalf("stale home = %#v", home)
	}
}

func TestGetCurationHomeReportsUnknownIndexDuringPendingRecovery(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("leave recovery pending")
	_, err := mutation.CommitWithOptions(root, mutation.WriteSet{
		OperationID: "OP-HOME-PENDING",
		Command:     "create_source",
		Changes:     []mutation.Change{{Path: "sources/catalog/SRC-HOME-PENDING.yaml", Contents: []byte("schema_version: 1\nid: SRC-HOME-PENDING\nadapter: git\nlocator: https://example.invalid/repo\n")}},
	}, mutation.Options{Fault: func(point mutation.FaultPoint) error {
		if point == mutation.FaultManifestPhaseUpdate {
			return injected
		}
		return nil
	}})
	if !errors.Is(err, injected) {
		t.Fatalf("commit error = %v", err)
	}
	// Canonical files can be transiently inconsistent while a journal is pending.
	// The home query must not inspect those bytes until recovery resolves them.
	if err := os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("transient\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home, err := (CurationService{}).GetCurationHome(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !home.Workspace.RecoveryPending || home.Workspace.Health != "valid" || home.Workspace.Index != "unknown" || home.Actions[0].Kind != "recover_workspace" {
		t.Fatalf("pending recovery home = %#v", home)
	}
}

func TestGetCurationHomeReadsHealthyWorkspaceOffline(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	commitWorkspace(t, root)
	home, err := (CurationService{}).GetCurationHome(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if home.Status != StatusOK || home.Workspace.Index != "current" || home.Workspace.GitDirty || len(home.SuggestedActions) != 1 {
		t.Fatalf("healthy home = %#v", home)
	}
	for _, category := range home.Categories {
		if category.Availability == AvailabilityNotConfigured && category.Count != 0 {
			t.Fatalf("unsupported category is misleading: %#v", category)
		}
	}
}

func commitWorkspace(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"-c", "user.name=Skill Hub Test", "-c", "user.email=test@skillhub.invalid", "commit", "-m", "initial"}} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
}

func TestGetCurationHomeNeverReportsCountsKnownWhenInvalid(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	commitWorkspace(t, root)

	// Create an active skill
	path := filepath.Join(root, "skills", "core", "test-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: test-skill\ndescription: Test skill.\n---\n\n# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(filepath.Dir(path), "skill.meta.yaml")
	if err := os.WriteFile(metadata, []byte("schema_version: 1\nid: test-skill\nname: test-skill\nstatus: active\ndescription: Test skill.\nrouting:\n  triggers: [test]\n  not_for: [none]\n  min_scope: single_step\nquality:\n  reviewed: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}

	// Verify home when healthy has CountsKnown == true
	healthyHome, err := (CurationService{}).GetCurationHome(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !healthyHome.CountsKnown {
		t.Fatal("healthy home must have CountsKnown == true")
	}

	// Now introduce an invalid canonical file (BUG-08 scenario)
	badMeta := filepath.Join(filepath.Dir(path), "skill.meta.yaml")
	if err := os.WriteFile(badMeta, []byte("schema_version: 1\nid: test-skill\nstatus: shiny\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	invalidHome, err := (CurationService{}).GetCurationHome(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if invalidHome.Workspace.Health != "invalid" {
		t.Fatalf("workspace health = %q, want invalid", invalidHome.Workspace.Health)
	}
	// Crucial assertion for BUG-08:
	if invalidHome.CountsKnown {
		t.Fatal("invalid workspace must NEVER have CountsKnown == true; unknown counts must not become zero / 'No skills yet'")
	}
	for _, cat := range invalidHome.Categories {
		if cat.Availability == AvailabilityAvailable {
			t.Errorf("category %q should be unavailable when workspace is invalid", cat.Kind)
		}
	}
	if invalidHome.Status != StatusRecoveryRequired {
		t.Fatalf("status = %v, want StatusRecoveryRequired", invalidHome.Status)
	}
	if !strings.Contains(invalidHome.Summary, "Workspace needs repair") {
		t.Fatalf("summary = %q, want repair guidance", invalidHome.Summary)
	}
}

func TestCurationHomeUpstreamUpdatesAndExcludesUpstreamOnlySources(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	// 1. Create an upstream-only source (purpose: upstream, without distilled_revision)
	sourceData := `schema_version: 1
id: anthropics-skills
purpose: upstream
adapter: git
locator:
  repository: https://github.com/anthropics/skills
  ref: main
status: watching
identity:
  name: skills
  canonical: https://github.com/anthropics/skills
trust:
  source: community
  reviewed: false
monitoring:
  enabled: true
  cadence: weekly
limits:
  timeout_seconds: 60
  max_bytes: 10485760
  max_files: 100
  max_file_bytes: 1048576
`
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", "anthropics-skills.yaml"), []byte(sourceData), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a tracked skill for this source
	skillDir := filepath.Join(root, "skills", "default", "pdf")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: pdf\ndescription: PDF skill\n---\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillMeta := `schema_version: 1
id: pdf
name: pdf
status: draft
description: PDF skill
routing:
  triggers: []
  not_for: []
  min_scope: ""
quality:
  reviewed: false
provenance:
  created_by: skill_add
  source_id: anthropics-skills
  origin:
    kind: github
    repository: https://github.com/anthropics/skills
    ref: main
    commit: aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111
    path: skills/pdf
    files_digest: sha256:1111111111111111111111111111111111111111111111111111111111111111
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(skillMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	commitWorkspace(t, root)
	if _, err := (CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}

	// 2. Check curation home before recording upstream state:
	// - ChangedSources must be 0 (upstream-only source excluded from distill_changed_sources)
	// - UpstreamUpdates must be 0
	home, err := (CurationService{}).GetCurationHome(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, cat := range home.Categories {
		if cat.Kind == "changed_sources" && cat.Count != 0 {
			t.Fatalf("expected changed_sources count 0, got %d", cat.Count)
		}
	}
	if home.HomeSummary.UpstreamUpdates != 0 {
		t.Fatalf("expected UpstreamUpdates 0, got %d", home.HomeSummary.UpstreamUpdates)
	}

	// 3. Record an upstream state with upstream: changed
	store := sourcepkg.OperationalStore{Root: root}
	now := time.Now().UTC()
	err = store.RecordUpstream(t.Context(), []sourcepkg.UpstreamState{
		{
			SkillID:         "pdf",
			SourceID:        "anthropics-skills",
			Repository:      "https://github.com/anthropics/skills",
			Ref:             "main",
			Path:            "skills/pdf",
			BaseCommit:      "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
			CheckedCommit:   "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222",
			CheckedCommitAt: now,
			Upstream:        "changed",
			UpstreamDigest:  "sha256:2222222222222222222222222222222222222222222222222222222222222222",
			ChangedFiles:    []sourcepkg.Change{{Path: "SKILL.md", Status: "modified"}},
			CheckedAt:       now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 4. Check curation home again:
	// - UpstreamUpdates must be 1
	// - Actions contains review_upstream_updates with Count: 1, Priority: 75
	// - Recommendation is "Review upstream updates with skillhub skill outdated"
	homeAfter, err := (CurationService{}).GetCurationHome(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if homeAfter.HomeSummary.UpstreamUpdates != 1 {
		t.Fatalf("expected UpstreamUpdates 1, got %d", homeAfter.HomeSummary.UpstreamUpdates)
	}
	var foundAction *ActionItem
	for _, a := range homeAfter.Actions {
		if a.Kind == "review_upstream_updates" {
			foundAction = &a
			break
		}
	}
	if foundAction == nil {
		t.Fatalf("expected action review_upstream_updates, got actions: %#v", homeAfter.Actions)
	}
	if foundAction.Count != 1 || foundAction.Priority != 75 {
		t.Fatalf("expected count 1 priority 75, got %#v", foundAction)
	}
	if len(homeAfter.SuggestedActions) == 0 || homeAfter.SuggestedActions[0].Label != "Review upstream updates with skillhub skill outdated" {
		t.Fatalf("expected recommended label 'Review upstream updates with skillhub skill outdated', got %#v", homeAfter.SuggestedActions)
	}
}

func TestCandidateLessonsCountedInHomeAndSkillSources(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Create a skill
	skillService := SkillService{}
	created, err := skillService.PreviewCreate(ctx, root, skill.CreateInput{
		ID: "candidate-skill", Collection: "default", Name: "Candidate Skill", Description: "Testing candidate counts",
		Routing: skill.RoutingInput{Operations: []string{"review"}, Triggers: []string{"test"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillService.ConfirmSkillMutation(ctx, root, created, created.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	// Add a learning source and link
	_ = os.MkdirAll(filepath.Join(root, "sources", "skills"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "sources", "skills", "LINK-candidate-skill--src-test.yaml"), []byte("schema_version: 1\nid: LINK-candidate-skill--src-test\nskill_id: candidate-skill\nsource_id: src-test\nrole: learning-source\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "sources", "catalog"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "src-test.yaml"), []byte("schema_version: 1\nid: src-test\nadapter: git\nstatus: watching\nidentity:\n  name: src-test\nlocator:\n  repository: https://example.com/src-test.git\ncurrent_revision:\n  kind: git-commit\n  value: 3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a\n  observed_at: \"2026-10-09T08:00:00Z\"\n  content_digest: sha256:3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a3f9c2a1b4d5e6f7a8b9c0d1e\ndistilled_revision:\n  kind: git-commit\n  value: 3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a\n  observed_at: \"2026-10-09T08:00:00Z\"\n  content_digest: sha256:3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a3f9c2a1b4d5e6f7a8b9c0d1e\nmonitoring:\n  enabled: true\n  cadence: manual\nlimits:\n  max_bytes: 10485760\n  max_files: 1000\n  max_file_bytes: 1048576\n  timeout_seconds: 30\n"), 0o644)

	srcService := SourceService{}

	// Initially 0 candidate lessons
	sourcesRes, err := srcService.SkillSources(ctx, root, "candidate-skill")
	if err != nil {
		t.Fatal(err)
	}
	if sourcesRes.PendingInsights != 0 {
		t.Fatalf("expected 0 pending insights initially, got %d", sourcesRes.PendingInsights)
	}

	// Write a distill.yaml with 2 candidate lessons and 1 planned lesson
	metaDir := filepath.Join(root, "skills", "default", "candidate-skill", ".meta")
	distillDoc := `goal:
  status: draft
  purpose: Test counting
  in_scope: [test]
  out_of_scope: [other]
  failures_it_prevents: [regression]
cursors:
  src-test: 3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a
lessons:
  - key: lesson-one
    layer: content
    what: First lesson
    notable: Notable one
    where: [src-test@3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a:file.go]
    contrast: new
    score: {relevance: 3, facts: [a, b], impact: 2, evidence: 2, effort: 1, why: test}
    decision: {state: candidate}
  - key: lesson-two
    layer: validation
    what: Second lesson
    notable: Notable two
    where: [src-test@3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a:other.go]
    contrast: extends
    score: {relevance: 3, facts: [a, b, c, d], impact: 4, evidence: 3, effort: 1, why: test}
    decision: {state: candidate}
  - key: lesson-three
    layer: craft
    what: Third lesson
    notable: Notable three
    where: [src-test@3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a:third.go]
    contrast: new
    score: {relevance: 1, facts: [a], impact: 1, evidence: 1, effort: 2, why: test}
    decision: {state: planned}
`
	if err := os.WriteFile(filepath.Join(metaDir, "distill.yaml"), []byte(distillDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify SkillSources now reports 2 pending insights (candidates)
	sourcesRes2, err := srcService.SkillSources(ctx, root, "candidate-skill")
	if err != nil {
		t.Fatal(err)
	}
	if sourcesRes2.PendingInsights != 2 {
		t.Fatalf("expected 2 pending insights from candidate lessons, got %d", sourcesRes2.PendingInsights)
	}
	if len(sourcesRes2.Learning) != 1 || sourcesRes2.Learning[0].PendingInsights != 2 {
		t.Fatalf("expected learning ref to have 2 pending insights, got %#v", sourcesRes2.Learning)
	}
	commitWorkspace(t, root)
	if _, buildErr := catalog.BuildCatalogGeneration(ctx, root, catalog.BuildOptions{}); buildErr != nil {
		t.Fatalf("build catalog error: %v", buildErr)
	}

	// Verify CurationHome now counts 2 candidate lessons and 1 high-value lesson
	homeSvc := CurationService{}
	homeRes, err := homeSvc.GetCurationHome(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if homeRes.HomeSummary.PendingInsights != 2 {
		t.Fatalf("expected home pending_insights 2, got %d", homeRes.HomeSummary.PendingInsights)
	}
	if homeRes.HomeSummary.PendingHighValueInsights != 1 {
		t.Fatalf("expected home pending_high_value_insights 1, got %d", homeRes.HomeSummary.PendingHighValueInsights)
	}

	// Verify that candidate lessons alone do NOT turn home.Status into action_required
	if homeRes.Status != StatusOK {
		t.Fatalf("expected StatusOK for optional candidate lessons, got %s", homeRes.Status)
	}
	// Verify review_lessons action item points to distill-lab and the top skill
	var reviewAction *ActionItem
	for _, a := range homeRes.Actions {
		if a.Kind == "review_lessons" {
			reviewAction = &a
			break
		}
	}
	if reviewAction == nil {
		t.Fatal("expected review_lessons action item in home.Actions")
	}
	if reviewAction.ID != "candidate-skill" {
		t.Fatalf("expected review_lessons ID 'candidate-skill', got %q", reviewAction.ID)
	}
	if !strings.Contains(reviewAction.Command, "distill.py list") {
		t.Fatalf("expected command to reference distill.py list, got %q", reviewAction.Command)
	}
	if !strings.Contains(reviewAction.Summary, "ready for review with distill-lab") {
		t.Fatalf("expected summary to reference distill-lab, got %q", reviewAction.Summary)
	}
}
