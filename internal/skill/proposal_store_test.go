package skill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestProposalArtifactRoundTripIsRestrictiveAndExpires(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	proposal, err := (Manager{}).PreviewCreate(t.Context(), root, CreateInput{
		ID: "roundtrip", Collection: "software", Name: "Roundtrip", Description: "Roundtrip proposal.", Content: []byte("# exact contents\n"),
		Routing: RoutingInput{Triggers: []string{"roundtrip"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC)
	if err := StoreProposal(root, proposal, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "runtime", "proposals", proposal.ID+".json")
	if info, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("artifact mode = %v, %v", info, err)
	}
	loaded, err := LoadProposal(root, proposal.ID, now.Add(time.Hour))
	wantContent := "---\nname: roundtrip\ndescription: Roundtrip proposal.\n---\n\n# exact contents\n"
	if err != nil || string(loaded.planned.WriteSet.Changes[0].Contents) != wantContent {
		t.Fatalf("loaded content = %q, want %q; err=%v", loaded.planned.WriteSet.Changes[0].Contents, wantContent, err)
	}
	if _, err := LoadProposal(root, proposal.ID, now.Add(proposalLifetime)); err == nil {
		t.Fatal("expired proposal was accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired artifact was not cleaned up: %v", err)
	}
}

func TestCreateGeneratesAndEditPreservesSkillFrontmatter(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "frontmatter", Collection: "software", Name: "Frontmatter", Description: "Valid distributed instructions.",
		Content: []byte("# Instructions\n\nOriginal body.\n"),
		Routing: RoutingInput{Triggers: []string{"use frontmatter"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, created); err != nil {
		t.Fatal(err)
	}
	updated, err := manager.PreviewUpdate(t.Context(), root, "frontmatter", UpdateInput{SetContent: true, Content: []byte("# Instructions\n\nUpdated body.\n")}, false)
	if err != nil {
		t.Fatal(err)
	}
	var entrypoint []byte
	for _, change := range updated.planned.WriteSet.Changes {
		if filepath.Base(change.Path) == "SKILL.md" {
			entrypoint = change.Contents
		}
	}
	wantPrefix := "---\nname: frontmatter\ndescription: Valid distributed instructions.\n---\n\n"
	if !strings.HasPrefix(string(entrypoint), wantPrefix) || !strings.Contains(string(entrypoint), "Updated body.") {
		t.Fatalf("edited content did not preserve frontmatter: %q", entrypoint)
	}
}

func TestManagerRechecksProposalExpiryAtConfirm(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC)
	manager := Manager{Clock: func() time.Time { return now }}
	proposal, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "expires", Collection: "software", Name: "Expires", Description: "Expiry test.",
		Routing: RoutingInput{Triggers: []string{"expiry"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(proposalLifetime)
	if _, err := manager.Confirm(t.Context(), root, proposal); !errors.Is(err, ErrSnapshotExpired) {
		t.Fatalf("expired confirmation error = %v", err)
	}
}

func TestEditableContentRejectsSymlink(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "skills", "software", "unsafe-read")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := "schema_version: 1\nid: unsafe-read\nname: Unsafe Read\nstatus: draft\ndescription: Reject symlinks\nrouting:\n  triggers: [read]\n  not_for: [other]\n  min_scope: single_step\nquality:\n  reviewed: false\nprovenance:\n  created_by: test\nhistory: []\ncreated_at: '2026-09-29T00:00:00Z'\nupdated_at: '2026-09-29T00:00:00Z'\n"
	if err := os.WriteFile(filepath.Join(directory, "skill.meta.yaml"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEditableContent(root, "unsafe-read"); err == nil {
		t.Fatal("editable read followed a symlink")
	}
}

func TestEditKeepsSkillFrontmatterDescriptionInSyncWithMetadata(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "in-sync", Collection: "software", Name: "In Sync", Description: "Original description.",
		Content: []byte("# Instructions\n\nBody.\n"),
		Routing: RoutingInput{Triggers: []string{"use in sync"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, created); err != nil {
		t.Fatal(err)
	}
	entrypoint := func(proposal Proposal) string {
		for _, change := range proposal.planned.WriteSet.Changes {
			if filepath.Base(change.Path) == "SKILL.md" {
				return string(change.Contents)
			}
		}
		return ""
	}

	description := "Updated description."
	body, err := manager.PreviewUpdate(t.Context(), root, "in-sync", UpdateInput{Description: &description, SetContent: true, Content: []byte("# Instructions\n\nNew body.\n")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := entrypoint(body); !strings.Contains(got, "description: Updated description.") || !strings.Contains(got, "name: in-sync") || !strings.Contains(got, "New body.") {
		t.Fatalf("reused header kept a stale description: %q", got)
	}

	only, err := manager.PreviewUpdate(t.Context(), root, "in-sync", UpdateInput{Description: &description}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := entrypoint(only); !strings.Contains(got, "description: Updated description.") || !strings.Contains(got, "Body.") {
		t.Fatalf("description-only edit did not update SKILL.md: %q", got)
	}

	name := "Renamed"
	nameOnly, err := manager.PreviewUpdate(t.Context(), root, "in-sync", UpdateInput{Name: &name}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := entrypoint(nameOnly); got != "" {
		t.Fatalf("name-only edit rewrote SKILL.md: %q", got)
	}
}
func TestProposalKindEnvelopeAndLegacyCompatibility(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	proposal, err := manager.PreviewCreate(t.Context(), root, CreateInput{
		ID: "kind-test", Collection: "software", Name: "Kind Test", Description: "Testing proposal kinds.",
		Content: []byte("# Kind Test\n"), Routing: RoutingInput{Triggers: []string{"test"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	proposal.RecoveryID = "REC-kind-test-12345"
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	if err := StoreProposal(root, proposal, now); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadProposal(root, proposal.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("LoadProposal failed: %v", err)
	}
	if loaded.Kind != ProposalKindLifecycle {
		t.Fatalf("loaded.Kind = %q, want %q", loaded.Kind, ProposalKindLifecycle)
	}
	if loaded.RecoveryID != "REC-kind-test-12345" {
		t.Fatalf("loaded.RecoveryID = %q, want %q", loaded.RecoveryID, "REC-kind-test-12345")
	}

	// Legacy proposal artifact without 'kind' or 'recovery_id'
	legacyJSON := `{
		"version": 1,
		"created_at": "2026-09-29T12:00:00Z",
		"expires_at": "2026-09-30T12:00:00Z",
		"id": "PROP-legacy-sample",
		"digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		"base_snapshot": "snap-1",
		"skill_id": "kind-test",
		"command": "skill_edit",
		"summary": {"added": [], "modified": [], "deleted": []},
		"write_set": {"OperationID": "OP-1", "Command": "skill_edit", "Changes": []}
	}`
	legacyPath := filepath.Join(root, "runtime", "proposals", "PROP-legacy-sample.json")
	if err := os.WriteFile(legacyPath, []byte(legacyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyLoaded, err := LoadProposal(root, "PROP-legacy-sample", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("LoadProposal on legacy artifact failed: %v", err)
	}
	if legacyLoaded.Kind != ProposalKindLifecycle {
		t.Fatalf("legacyLoaded.Kind = %q, want %q", legacyLoaded.Kind, ProposalKindLifecycle)
	}

	// Unknown kind rejection
	unknownKindJSON := `{
		"version": 1,
		"kind": "unsupported_kind",
		"created_at": "2026-09-29T12:00:00Z",
		"expires_at": "2026-09-30T12:00:00Z",
		"id": "PROP-unknown-kind",
		"digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		"base_snapshot": "snap-1",
		"skill_id": "kind-test",
		"command": "skill_edit",
		"summary": {"added": [], "modified": [], "deleted": []},
		"write_set": {"OperationID": "OP-2", "Command": "skill_edit", "Changes": []}
	}`
	unknownKindPath := filepath.Join(root, "runtime", "proposals", "PROP-unknown-kind.json")
	if err := os.WriteFile(unknownKindPath, []byte(unknownKindJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProposal(root, "PROP-unknown-kind", now.Add(time.Minute)); err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("expected unknown kind error, got: %v", err)
	}

	// Unknown field rejection
	unknownFieldJSON := `{
		"version": 1,
		"created_at": "2026-09-29T12:00:00Z",
		"expires_at": "2026-09-30T12:00:00Z",
		"id": "PROP-unknown-field",
		"digest": "sha256:3333333333333333333333333333333333333333333333333333333333333333",
		"base_snapshot": "snap-1",
		"skill_id": "kind-test",
		"command": "skill_edit",
		"summary": {"added": [], "modified": [], "deleted": []},
		"write_set": {"OperationID": "OP-3", "Command": "skill_edit", "Changes": []},
		"extra_bogus_field": true
	}`
	unknownFieldPath := filepath.Join(root, "runtime", "proposals", "PROP-unknown-field.json")
	if err := os.WriteFile(unknownFieldPath, []byte(unknownFieldJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProposal(root, "PROP-unknown-field", now.Add(time.Minute)); err == nil {
		t.Fatal("expected unknown field to be rejected by DisallowUnknownFields")
	}
}

func TestEditorRecoveryLifecycleAndCleanup(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Valuable unsaved editor content\n")
	now := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	recID, err := SaveEditorRecovery(root, "PROP-test-recovery", content, now)
	if err != nil {
		t.Fatalf("SaveEditorRecovery failed: %v", err)
	}
	if !strings.HasPrefix(recID, "REC-") {
		t.Fatalf("unexpected recovery ID format: %s", recID)
	}

	// Verify file mode
	recPath := filepath.Join(root, "runtime", "edits", recID+".md")
	info, err := os.Stat(recPath)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("recovery artifact mode: %v, err: %v", info, err)
	}

	// Read content back
	readBack, err := ReadEditorRecovery(root, recID)
	if err != nil {
		t.Fatalf("ReadEditorRecovery failed: %v", err)
	}
	if string(readBack) != string(content) {
		t.Fatalf("readBack = %q, want %q", readBack, content)
	}

	// Delete recovery
	if err := DeleteEditorRecovery(root, recID); err != nil {
		t.Fatalf("DeleteEditorRecovery failed: %v", err)
	}
	if _, err := ReadEditorRecovery(root, recID); err == nil {
		t.Fatal("expected ReadEditorRecovery to fail after deletion")
	}

	// Expiry cleanup
	oldRecID, err := SaveEditorRecovery(root, "PROP-old-recovery", content, now.Add(-25*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(root, "runtime", "edits", oldRecID+".md")
	// Set mod time to 25 hours ago
	oldTime := now.Add(-25 * time.Hour)
	_ = os.Chtimes(oldPath, oldTime, oldTime)

	if err := CleanupExpiredRecoveries(root, now); err != nil {
		t.Fatalf("CleanupExpiredRecoveries failed: %v", err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected expired recovery to be removed: %v", err)
	}
}

func TestProposalDispatcherDispatchByKind(t *testing.T) {
	t.Parallel()
	dispatcher := NewProposalDispatcher()
	calledAdd := false
	dispatcher.Register(ProposalKindAdd, func(ctx context.Context, root string, p Proposal, pins mutation.Confirmation) (MutationResult, error) {
		calledAdd = true
		return MutationResult{OperationID: "OP-ADD-CONFIRMED"}, nil
	})

	addProposal := Proposal{
		ID: "PROP-add-1", Kind: ProposalKindAdd,
	}
	res, err := dispatcher.Dispatch(context.Background(), "", addProposal, mutation.Confirmation{})
	if err != nil || !calledAdd || res.OperationID != "OP-ADD-CONFIRMED" {
		t.Fatalf("add dispatch failed: %#v, err=%v", res, err)
	}

	unregisteredProposal := Proposal{
		ID: "PROP-custom-1", Kind: "unregistered_kind",
	}
	if _, err := dispatcher.Dispatch(context.Background(), "", unregisteredProposal, mutation.Confirmation{}); err == nil {
		t.Fatal("expected error on unregistered proposal kind")
	}
}
