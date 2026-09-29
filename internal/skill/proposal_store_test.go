package skill

import (
	"errors"
	"os"
	"path/filepath"
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
	if err != nil || string(loaded.planned.WriteSet.Changes[0].Contents) != "# exact contents\n" {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}
	if _, err := LoadProposal(root, proposal.ID, now.Add(proposalLifetime)); err == nil {
		t.Fatal("expired proposal was accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired artifact was not cleaned up: %v", err)
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
