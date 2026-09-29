package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
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
