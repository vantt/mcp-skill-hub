package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestProposalArtifactRoundTripIsRestrictiveAndExpires(t *testing.T) {
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
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
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
