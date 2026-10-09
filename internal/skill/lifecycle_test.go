package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestReadEditableSkillReturnsContentAndDigest(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	content := []byte("# Editable Skill\n\nCanonical instructions.\n")
	created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "editable-read", Collection: "software", Name: "Editable Read",
		Description: "Testing editable content read.", Content: content,
		Routing: RoutingInput{Triggers: []string{"read"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, created); err != nil {
		t.Fatal(err)
	}

	editable, err := ReadEditableSkill(root, "editable-read")
	if err != nil {
		t.Fatalf("ReadEditableSkill failed: %v", err)
	}
	if !strings.HasSuffix(editable.Path, "skills/software/editable-read/SKILL.md") {
		t.Fatalf("unexpected entrypoint path: %s", editable.Path)
	}
	sum := sha256.Sum256(editable.Content)
	expectedDigest := "sha256:" + hex.EncodeToString(sum[:])
	if editable.Digest != expectedDigest {
		t.Fatalf("digest mismatch: got %s, want %s", editable.Digest, expectedDigest)
	}
}

func TestUpdateWithExpectedContentDigestDetectsConflict(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "conflict-test", Collection: "software", Name: "Conflict Test",
		Description: "Testing edit conflicts.", Content: []byte("# Original Content\n"),
		Routing: RoutingInput{Triggers: []string{"conflict"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, created); err != nil {
		t.Fatal(err)
	}

	// 1. Writer A reads the current content and digest
	readA, err := ReadEditableSkill(root, "conflict-test")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Writer B updates the skill first (either via managed update or external change)
	entrypointPath := filepath.Join(root, filepath.FromSlash(readA.Path))
	writerBContent := "---\nname: conflict-test\ndescription: Testing edit conflicts.\n---\n\n# Changed by Writer B\n"
	if err := os.WriteFile(entrypointPath, []byte(writerBContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// 3. Writer A attempts to preview an update using their stale expected digest
	updatedContentA := []byte("# Desired by Writer A\n")
	_, err = manager.PreviewUpdate(t.Context(), root, "conflict-test", UpdateInput{
		SetContent:            true,
		Content:               updatedContentA,
		ExpectedContentDigest: readA.Digest,
	}, false)
	if err == nil {
		t.Fatal("expected PreviewUpdate to fail with edit conflict")
	}

	var conflictErr *EditConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected *EditConflictError, got: %v", err)
	}
	if !errors.Is(err, ErrEditConflict) {
		t.Fatalf("expected errors.Is(err, ErrEditConflict) to be true")
	}
	if conflictErr.ExpectedDigest != readA.Digest {
		t.Fatalf("conflict expected digest = %s, want %s", conflictErr.ExpectedDigest, readA.Digest)
	}
	sumB := sha256.Sum256([]byte(writerBContent))
	actualDigestB := "sha256:" + hex.EncodeToString(sumB[:])
	if conflictErr.ActualDigest != actualDigestB {
		t.Fatalf("conflict actual digest = %s, want %s", conflictErr.ActualDigest, actualDigestB)
	}

	// Verify Writer B's bytes remain intact
	surviving, err := os.ReadFile(entrypointPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(surviving) != writerBContent {
		t.Fatalf("Writer B's content was overwritten: %q", string(surviving))
	}
}

func TestUpdateBlindReplacementRemainsCompatible(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "blind-test", Collection: "software", Name: "Blind Test",
		Description: "Testing blind replacement.", Content: []byte("# First\n"),
		Routing: RoutingInput{Triggers: []string{"blind"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, created); err != nil {
		t.Fatal(err)
	}

	// Omitted ExpectedContentDigest performs blind replacement
	preview, err := manager.PreviewUpdate(t.Context(), root, "blind-test", UpdateInput{
		SetContent: true,
		Content:    []byte("# Second\n"),
	}, false)
	if err != nil {
		t.Fatalf("blind PreviewUpdate failed: %v", err)
	}
	if _, err := manager.Confirm(t.Context(), root, preview); err != nil {
		t.Fatalf("blind Confirm failed: %v", err)
	}

	read, err := ReadEditableSkill(root, "blind-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(read.Content), "# Second") {
		t.Fatalf("blind replacement did not update content: %q", string(read.Content))
	}
}

func TestUntouchedScaffoldRejectsActivationUntilReplaced(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}

	// 1. Create a draft with empty content (generates scaffold)
	created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "scaffold-skill", Collection: "software", Name: "Scaffold Skill",
		Description: "Scaffold description.",
		Routing:     RoutingInput{Triggers: []string{"scaffold"}, NotFor: []string{"none"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, created); err != nil {
		t.Fatal(err)
	}

	// 2. Attempt to activate untouched scaffold
	_, err = manager.PreviewTransition(t.Context(), root, "scaffold-skill", "active", false)
	if err == nil {
		t.Fatal("expected untouched scaffold activation to fail")
	}
	if !errors.Is(err, ErrUntouchedScaffold) {
		t.Fatalf("expected ErrUntouchedScaffold, got: %v", err)
	}

	// 3. User edits content to replace the scaffold
	editable, err := ReadEditableSkill(root, "scaffold-skill")
	if err != nil {
		t.Fatal(err)
	}
	meaningfulContent := "# Scaffold Skill\n\nThese are actual, meaningful instructions for the agent.\n\n## Steps\n\n1. Real step one.\n"
	editProposal, err := manager.PreviewUpdate(t.Context(), root, "scaffold-skill", UpdateInput{
		SetContent:            true,
		Content:               []byte(meaningfulContent),
		ExpectedContentDigest: editable.Digest,
	}, false)
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}
	if _, err := manager.Confirm(t.Context(), root, editProposal); err != nil {
		t.Fatal(err)
	}

	// 4. Now activation succeeds
	activateProposal, err := manager.PreviewTransition(t.Context(), root, "scaffold-skill", "active", false)
	if err != nil {
		t.Fatalf("expected activation to succeed after scaffold replaced, got: %v", err)
	}
	if _, err := manager.Confirm(t.Context(), root, activateProposal); err != nil {
		t.Fatalf("confirmation failed: %v", err)
	}
}

func TestGenuineSkillsWithoutMarkerCanActivate(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}

	// Content has "1. First step." but is not the generated template (no other template lines, no marker)
	genuineContent := "# Genuine Skill\n\nReal procedure.\n\n1. First step.\n2. Verify result.\n"
	created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "genuine-skill", Collection: "software", Name: "Genuine Skill",
		Description: "Genuine instructions.", Content: []byte(genuineContent),
		Routing: RoutingInput{Triggers: []string{"genuine"}, NotFor: []string{"none"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, created); err != nil {
		t.Fatal(err)
	}

	// Should activate without being falsely flagged as scaffold
	activateProposal, err := manager.PreviewTransition(t.Context(), root, "genuine-skill", "active", false)
	if err != nil {
		t.Fatalf("genuine skill activation failed: %v", err)
	}
	if _, err := manager.Confirm(t.Context(), root, activateProposal); err != nil {
		t.Fatal(err)
	}
}

func TestMergeRoutingPreservesKeysTheInputDoesNotName(t *testing.T) {
	t.Parallel()
	existing := func() map[string]any {
		return map[string]any{
			"operations":       []any{"review"},
			"triggers":         []any{"old trigger"},
			"not_for":          []any{"old not for"},
			"min_scope":        "multi_step",
			"requirements":     map[string]any{"capabilities": map[string]any{"all": []any{"git"}}},
			"distinguish_from": []any{map[string]any{"skill": "other"}},
			"supporting":       []any{map[string]any{"skill": "helper"}},
			"equivalent_to":    []any{map[string]any{"skill": "twin"}},
			"boosts":           map[string]any{"go": 0.2},
			"examples":         []any{"stored example"},
			"counter_examples": []any{"stored counter example"},
		}
	}
	preserved := []string{"requirements", "distinguish_from", "supporting", "equivalent_to", "boosts"}
	cases := []struct {
		name                string
		input               RoutingInput
		wantExamples        any
		wantCounterExamples any
	}{
		{
			name:                "triggers only keeps optional lists",
			input:               RoutingInput{Triggers: []string{"new trigger"}},
			wantExamples:        []any{"stored example"},
			wantCounterExamples: []any{"stored counter example"},
		},
		{
			name:                "explicit empty examples clears only examples",
			input:               RoutingInput{Triggers: []string{"new trigger"}, Examples: []string{}},
			wantExamples:        nil,
			wantCounterExamples: []any{"stored counter example"},
		},
		{
			name:                "supplied lists replace and are cleaned",
			input:               RoutingInput{Triggers: []string{"new trigger"}, Examples: []string{" a ", "a", "b"}, CounterExamples: []string{"c"}},
			wantExamples:        []string{"a", "b"},
			wantCounterExamples: []string{"c"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := existing()
			merged := mergeRouting(before, tc.input)
			for _, key := range preserved {
				if !reflect.DeepEqual(merged[key], before[key]) {
					t.Fatalf("%s = %#v, want preserved %#v", key, merged[key], before[key])
				}
			}
			if !reflect.DeepEqual(merged["triggers"], []string{"new trigger"}) {
				t.Fatalf("triggers = %#v", merged["triggers"])
			}
			if !reflect.DeepEqual(merged["operations"], []string{}) || merged["min_scope"] != "" || !reflect.DeepEqual(merged["not_for"], []string{}) {
				t.Fatalf("core routing fields must be written from the input: %#v", merged)
			}
			if !reflect.DeepEqual(merged["examples"], tc.wantExamples) {
				t.Fatalf("examples = %#v, want %#v", merged["examples"], tc.wantExamples)
			}
			if !reflect.DeepEqual(merged["counter_examples"], tc.wantCounterExamples) {
				t.Fatalf("counter_examples = %#v, want %#v", merged["counter_examples"], tc.wantCounterExamples)
			}
			if !reflect.DeepEqual(before, existing()) {
				t.Fatal("mergeRouting mutated the stored routing map")
			}
		})
	}
}

func TestUpdateRoutingPreservesStoredFieldsAndRecordsScriptsReview(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "merge-test", Collection: "software", Name: "Merge Test",
		Description: "Testing routing merge.", Content: []byte("# Merge\n"),
		Routing: RoutingInput{
			Triggers: []string{"merge"}, MinScope: "single_step",
			Examples: []string{"merge these branches"}, CounterExamples: []string{"write a poem"},
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, created); err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(root, "skills", "software", "merge-test", ".meta", "skill.yaml")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		metadataPath = filepath.Join(root, "skills", "software", "merge-test", "skill.meta.yaml")
	}
	contents, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	withExtras := strings.Replace(string(contents), "routing:\n", "routing:\n    requirements:\n        capabilities:\n            all: [git]\n    boosts:\n        go: 0.2\n", 1)
	if withExtras == string(contents) {
		t.Fatalf("fixture routing block not found:\n%s", contents)
	}
	if err := os.WriteFile(metadataPath, []byte(withExtras), 0o600); err != nil {
		t.Fatal(err)
	}

	digest := "sha256:" + strings.Repeat("ab", 32)
	preview, err := manager.PreviewUpdate(t.Context(), root, "merge-test", UpdateInput{
		Routing:               &RoutingInput{Triggers: []string{"merge branches"}, MinScope: "single_step"},
		ContentReviewedDigest: &digest,
	}, false)
	if err != nil {
		t.Fatalf("PreviewUpdate failed: %v", err)
	}
	if _, err := manager.Confirm(t.Context(), root, preview); err != nil {
		t.Fatalf("Confirm failed: %v", err)
	}
	_, _, document, err := loadSkill(root, "merge-test")
	if err != nil {
		t.Fatal(err)
	}
	routing := mapValue(document, "routing")
	if !reflect.DeepEqual(routing["triggers"], []any{"merge branches"}) {
		t.Fatalf("triggers = %#v", routing["triggers"])
	}
	for key, want := range map[string]any{
		"requirements":     map[string]any{"capabilities": map[string]any{"all": []any{"git"}}},
		"boosts":           map[string]any{"go": 0.2},
		"examples":         []any{"merge these branches"},
		"counter_examples": []any{"write a poem"},
	} {
		if !reflect.DeepEqual(routing[key], want) {
			t.Fatalf("routing.%s = %#v, want %#v", key, routing[key], want)
		}
	}
	if got := mapValue(document, "quality")["content_reviewed_digest"]; got != digest {
		t.Fatalf("quality.content_reviewed_digest = %#v, want %q", got, digest)
	}
	stored, err := ReadRouting(root, "merge-test")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Examples, []string{"merge these branches"}) || !reflect.DeepEqual(stored.CounterExamples, []string{"write a poem"}) {
		t.Fatalf("ReadRouting examples = %#v / %#v", stored.Examples, stored.CounterExamples)
	}

	cleared, err := manager.PreviewUpdate(t.Context(), root, "merge-test", UpdateInput{
		Routing: &RoutingInput{Triggers: []string{"merge branches"}, MinScope: "single_step", Examples: []string{}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, cleared); err != nil {
		t.Fatal(err)
	}
	stored, err = ReadRouting(root, "merge-test")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Examples != nil || !reflect.DeepEqual(stored.CounterExamples, []string{"write a poem"}) {
		t.Fatalf("explicit empty examples must clear only examples: %#v / %#v", stored.Examples, stored.CounterExamples)
	}
}

func TestUpdateRequestDigestDistinguishesOptionalRoutingLists(t *testing.T) {
	t.Parallel()
	digest := func(routing RoutingInput) string {
		return updateRequestDigest("skill", UpdateInput{Routing: &routing})
	}
	keep := digest(RoutingInput{Triggers: []string{"t"}})
	clear := digest(RoutingInput{Triggers: []string{"t"}, Examples: []string{}})
	set := digest(RoutingInput{Triggers: []string{"t"}, Examples: []string{"e"}})
	other := digest(RoutingInput{Triggers: []string{"t"}, Examples: []string{"f"}})
	counter := digest(RoutingInput{Triggers: []string{"t"}, CounterExamples: []string{"e"}})
	seen := map[string]string{}
	for name, value := range map[string]string{"keep": keep, "clear": clear, "set": set, "other": other, "counter": counter} {
		if prior, ok := seen[value]; ok {
			t.Fatalf("%s and %s share request digest %s", prior, name, value)
		}
		seen[value] = name
	}

	// Requests that do not use the new fields keep their historical digest so
	// recorded idempotency keys still match after an upgrade.
	legacy := normalizedDigest(struct {
		ID                    string
		Name                  *string
		Description           *string
		Content               string
		SetContent            bool
		ExpectedContentDigest string
		Routing               *struct {
			Operations []string `json:"operations,omitempty"`
			Triggers   []string `json:"triggers,omitempty"`
			NotFor     []string `json:"not_for,omitempty"`
			MinScope   string   `json:"min_scope,omitempty"`
		}
		Rationale *string
	}{ID: "skill", Routing: &struct {
		Operations []string `json:"operations,omitempty"`
		Triggers   []string `json:"triggers,omitempty"`
		NotFor     []string `json:"not_for,omitempty"`
		MinScope   string   `json:"min_scope,omitempty"`
	}{Operations: []string{}, Triggers: []string{"t"}, NotFor: []string{}}})
	if keep != legacy {
		t.Fatalf("request digest without new fields changed: got %s, want %s", keep, legacy)
	}
	approved := "sha256:" + strings.Repeat("0", 64)
	if updateRequestDigest("skill", UpdateInput{ContentReviewedDigest: &approved}) == updateRequestDigest("skill", UpdateInput{}) {
		t.Fatal("scripts review approval must change the request digest")
	}
}
