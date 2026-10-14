package skill

import (
	"errors"
	"os"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/mutation"
)

func TestPreviewChangesPlansOneMutationForSeveralFiles(t *testing.T) {
	t.Parallel()
	fixture := newResourceFixture(t)
	manager := Manager{}
	changes := []mutation.Change{
		{Path: fixture.path("references/guide.md"), BeforeDigest: digestOf([]byte("# Guide\n\nFirst line.\n")), Contents: []byte("# Guide\n\nSecond line.\n")},
		{Path: fixture.path("references/new.md"), Contents: []byte("new\n")},
		{Path: fixture.path("docs/nested/note.md"), BeforeDigest: digestOf([]byte("# Note\n")), Delete: true},
	}
	proposal, err := manager.PreviewChanges(t.Context(), fixture.root, "res-skill", changes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Summary.Added) != 1 || len(proposal.Summary.Modified) != 1 || len(proposal.Summary.Deleted) != 1 {
		t.Fatalf("summary = %+v", proposal.Summary)
	}
	if _, err := os.Stat(fixture.abs("references/new.md")); !os.IsNotExist(err) {
		t.Fatal("a preview must not write")
	}
	if _, err := manager.Confirm(t.Context(), fixture.root, proposal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture.abs("references/new.md")); err != nil {
		t.Fatalf("confirm did not write: %v", err)
	}
	if _, err := os.Stat(fixture.abs("docs/nested/note.md")); !os.IsNotExist(err) {
		t.Fatal("confirm did not delete")
	}
}

func TestPreviewChangesRefusesAnEmptySetAndAStaleDigest(t *testing.T) {
	t.Parallel()
	fixture := newResourceFixture(t)
	manager := Manager{}
	if _, err := manager.PreviewChanges(t.Context(), fixture.root, "res-skill", nil, false); err == nil {
		t.Fatal("an empty changeset must be refused")
	}
	_, err := manager.PreviewChanges(t.Context(), fixture.root, "res-skill", []mutation.Change{
		{Path: fixture.path("references/guide.md"), BeforeDigest: digestOf([]byte("not what is there")), Contents: []byte("x\n")},
	}, false)
	var conflict *EditConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want an edit conflict", err)
	}
}
