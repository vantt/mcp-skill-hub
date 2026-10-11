package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const freshnessSkillMeta = "schema_version: 1\nid: freshness-skill\nname: Freshness Skill\nstatus: active\ndescription: Review freshness fixture.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review freshness fixture]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n"

func newFreshnessWorkspace(t *testing.T) string {
	t.Helper()
	root := newWorkspace(t)
	writeCanonical(t, root, "skills/freshness/freshness-skill/skill.meta.yaml", freshnessSkillMeta)
	writeCanonical(t, root, "skills/freshness/freshness-skill/SKILL.md", "---\nname: freshness-skill\ndescription: Review freshness fixture.\n---\n\n# Freshness\n\nVersion A.\n")
	writeCanonical(t, root, "skills/freshness/freshness-skill/notes.md", "Notes.\n")
	ageCanonicalFiles(t, root, time.Minute)
	build(t, root, BuildOptions{})
	return root
}

func requireCatalogState(t *testing.T, root string, want State) {
	t.Helper()
	status, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != want {
		t.Fatalf("catalog state = %s, want %s (%s)", status.State, want, status.Detail)
	}
}

// TestInspectObservesEveryCanonicalChangeOnTheNextCheck checks that the cheap
// freshness check never reports a changed workspace as healthy, and reports the
// original contents as healthy again once a change is undone.
func TestInspectObservesEveryCanonicalChangeOnTheNextCheck(t *testing.T) {
	const body = "skills/freshness/freshness-skill/SKILL.md"
	const notes = "skills/freshness/freshness-skill/notes.md"
	changes := []struct {
		name   string
		mutate func(t *testing.T, root string)
		undo   func(t *testing.T, root string)
	}{
		{
			name: "edit keeping size and modification time",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, filepath.FromSlash(body))
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				edited := []byte(string(original[:len(original)-3]) + "B.\n")
				if len(edited) != len(original) {
					t.Fatal("fixture edit must keep the size")
				}
				if err := os.WriteFile(path, edited, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			},
			undo: func(t *testing.T, root string) {
				writeCanonical(t, root, body, "---\nname: freshness-skill\ndescription: Review freshness fixture.\n---\n\n# Freshness\n\nVersion A.\n")
			},
		},
		{
			name: "very recent edit keeping size",
			mutate: func(t *testing.T, root string) {
				writeCanonical(t, root, notes, "Notez.\n")
			},
			undo: func(t *testing.T, root string) {
				writeCanonical(t, root, notes, "Notes.\n")
			},
		},
		{
			name: "add a file",
			mutate: func(t *testing.T, root string) {
				writeCanonical(t, root, "skills/freshness/freshness-skill/extra.md", "Extra.\n")
			},
			undo: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "skills/freshness/freshness-skill/extra.md")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "delete a file",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(notes))); err != nil {
					t.Fatal(err)
				}
			},
			undo: func(t *testing.T, root string) {
				writeCanonical(t, root, notes, "Notes.\n")
			},
		},
		{
			name: "rename a file",
			mutate: func(t *testing.T, root string) {
				if err := os.Rename(filepath.Join(root, filepath.FromSlash(notes)), filepath.Join(root, "skills/freshness/freshness-skill/renamed.md")); err != nil {
					t.Fatal(err)
				}
			},
			undo: func(t *testing.T, root string) {
				if err := os.Rename(filepath.Join(root, "skills/freshness/freshness-skill/renamed.md"), filepath.Join(root, filepath.FromSlash(notes))); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			root := newFreshnessWorkspace(t)
			requireCatalogState(t, root, StateHealthy)
			requireCatalogState(t, root, StateHealthy)
			change.mutate(t, root)
			requireCatalogState(t, root, StateStale)
			requireCatalogState(t, root, StateStale)
			change.undo(t, root)
			requireCatalogState(t, root, StateHealthy)
		})
	}
}

func TestOpenCurrentRefusesCatalogAfterSameSizeEdit(t *testing.T) {
	root := newFreshnessWorkspace(t)
	handle, err := OpenCurrent(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	writeCanonical(t, root, "skills/freshness/freshness-skill/notes.md", "Notez.\n")
	if _, err := OpenCurrent(context.Background(), root); !errors.Is(err, ErrCatalogUnavailable) {
		t.Fatalf("OpenCurrent after an edit = %v, want %v", err, ErrCatalogUnavailable)
	}
}

func TestOpenWithFallbackRebuildsAfterSameSizeEdit(t *testing.T) {
	root := newFreshnessWorkspace(t)
	first, err := OpenWithFallback(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	before := first.Pointer.ProjectionInputDigest
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	writeCanonical(t, root, "skills/freshness/freshness-skill/notes.md", "Notez.\n")
	second, err := OpenWithFallback(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.Pointer.ProjectionInputDigest == before || second.Status.State != StateHealthy {
		t.Fatalf("served generation %s (state %s) after an edit, want a rebuilt healthy generation", second.Pointer.Generation, second.Status.State)
	}
}
