package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOperationalStoreResetsCorruptDatabaseAndTreatsSourcesAsDue(t *testing.T) {
	root := t.TempDir()
	runtime := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, "operational.db"), []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, found, err := (OperationalStore{Root: root}).Get(context.Background(), "source-a")
	if err != nil || found || state.SourceID != "" {
		t.Fatalf("Get after corruption = %#v, %t, %v", state, found, err)
	}
	if _, err := os.Stat(filepath.Join(runtime, "operational.db.corrupt")); err != nil {
		t.Fatalf("corrupt database was not retained safely: %v", err)
	}
	states, err := (OperationalStore{Root: root}).List(context.Background())
	if err != nil || len(states) != 0 {
		t.Fatalf("List after reset = %#v, %v", states, err)
	}
}

func TestOperationalUpstreamState(t *testing.T) {
	root := t.TempDir()
	store := OperationalStore{Root: root}
	ctx := context.Background()

	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	commitTime := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)

	// 1. Initial record with ChangedFiles == nil
	stateA := UpstreamState{
		SkillID:         "skill-a",
		SourceID:        "source-1",
		Repository:      "https://github.com/example/skills",
		Ref:             "main",
		Path:            "skills/a",
		BaseCommit:      "1111111111111111111111111111111111111111",
		CheckedCommit:   "2222222222222222222222222222222222222222",
		CheckedCommitAt: commitTime,
		Upstream:        "same",
		UpstreamDigest:  "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ChangedFiles:    nil,
		CheckedAt:       now,
		LastError:       "",
	}

	stateB := UpstreamState{
		SkillID:         "skill-b",
		SourceID:        "source-1",
		Repository:      "https://github.com/example/skills",
		Ref:             "main",
		Path:            "skills/b",
		BaseCommit:      "1111111111111111111111111111111111111111",
		CheckedCommit:   "2222222222222222222222222222222222222222",
		CheckedCommitAt: commitTime,
		Upstream:        "changed",
		UpstreamDigest:  "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ChangedFiles: []Change{
			{Path: "SKILL.md", Status: "modified"},
		},
		CheckedAt: now,
		LastError: "",
	}

	if err := store.RecordUpstream(ctx, []UpstreamState{stateA, stateB}); err != nil {
		t.Fatalf("RecordUpstream failed: %v", err)
	}

	// 2. Round-trip ListUpstream
	list, err := store.ListUpstream(ctx)
	if err != nil {
		t.Fatalf("ListUpstream failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	if list[0].SkillID != "skill-a" || list[0].ChangedFiles != nil {
		t.Fatalf("expected skill-a with nil ChangedFiles, got %#v", list[0])
	}
	if list[1].SkillID != "skill-b" || len(list[1].ChangedFiles) != 1 || list[1].ChangedFiles[0].Path != "SKILL.md" {
		t.Fatalf("expected skill-b with 1 changed file, got %#v", list[1])
	}

	// 3. Upsert replacing a row (newer checked_at)
	later := now.Add(time.Hour)
	stateAUpdated := stateA
	stateAUpdated.Upstream = "changed"
	stateAUpdated.CheckedAt = later
	stateAUpdated.ChangedFiles = []Change{{Path: "README.md", Status: "added"}}
	if err := store.RecordUpstream(ctx, []UpstreamState{stateAUpdated}); err != nil {
		t.Fatalf("RecordUpstream upsert failed: %v", err)
	}
	listAfterUpsert, err := store.ListUpstream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if listAfterUpsert[0].Upstream != "changed" || len(listAfterUpsert[0].ChangedFiles) != 1 {
		t.Fatalf("expected updated row for skill-a, got %#v", listAfterUpsert[0])
	}

	// Older checked_at does not overwrite
	older := now.Add(-time.Hour)
	stateAOlder := stateA
	stateAOlder.Upstream = "same"
	stateAOlder.CheckedAt = older
	if err := store.RecordUpstream(ctx, []UpstreamState{stateAOlder}); err != nil {
		t.Fatalf("RecordUpstream older failed: %v", err)
	}
	listAfterOlder, err := store.ListUpstream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if listAfterOlder[0].Upstream != "changed" {
		t.Fatalf("older checked_at should not overwrite newer state, got %#v", listAfterOlder[0])
	}

	// 4. DeleteUpstreamExcept
	if err := store.DeleteUpstreamExcept(ctx, []string{"skill-a"}); err != nil {
		t.Fatalf("DeleteUpstreamExcept failed: %v", err)
	}
	listAfterDelete, err := store.ListUpstream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listAfterDelete) != 1 || listAfterDelete[0].SkillID != "skill-a" {
		t.Fatalf("expected only skill-a remaining, got %#v", listAfterDelete)
	}

	// Truncate table when keep is empty
	if err := store.DeleteUpstreamExcept(ctx, nil); err != nil {
		t.Fatalf("DeleteUpstreamExcept nil failed: %v", err)
	}
	listAfterTruncate, err := store.ListUpstream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listAfterTruncate) != 0 {
		t.Fatalf("expected empty list after truncate, got %d", len(listAfterTruncate))
	}
}
