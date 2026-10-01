package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestReadEditableSkillReturnsContentAndDigest(t *testing.T) {
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
