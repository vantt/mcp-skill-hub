package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// settleHub backdates every canonical file, like a hub nobody edited in the
// last minute, so scans trust file identity instead of rehashing.
func settleHub(t *testing.T, root string) {
	t.Helper()
	files, err := workspace.RelativeFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute)
	for _, relative := range files {
		if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(relative)), past, past); err != nil {
			t.Fatal(err)
		}
	}
}

// memoReads performs every request that serves a skill from one generation and
// returns the manifest the distribution service produced.
func memoReads(t *testing.T, root string, snapshots SnapshotService) DistributedSkill {
	t.Helper()
	ctx := t.Context()
	service := DistributionService{}
	entries, _, err := service.ListSkills(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	var entry DistributedSkill
	for _, candidate := range entries {
		if candidate.SkillID == "bundle-skill" {
			entry = candidate
		}
	}
	if entry.URI == "" {
		t.Fatalf("skills/list has no bundle-skill: %v", entries)
	}
	if _, err := service.GetSkill(ctx, root, entry.URI); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadResource(ctx, root, entry.URI); err != nil {
		t.Fatal(err)
	}
	if _, err := (SkillService{}).ReadSkillCore(ctx, root, "bundle-skill"); err != nil {
		t.Fatal(err)
	}
	if local, err := snapshots.Ensure(ctx, root, "bundle-skill"); err != nil || local.Status != LocalStatusReady {
		t.Fatalf("ensure = %+v, %v", local, err)
	}
	return entry
}

// The tests below count generation databases the process opens; they run one
// at a time so no other test's requests are counted.

func TestUnchangedGenerationIsServedWithoutOpeningItsDatabase(t *testing.T) {
	root := newBundleWorkspace(t)
	settleHub(t, root)
	snapshots := NewSnapshotService()
	memoReads(t, root, snapshots)

	before := catalog.PinnedOpens()
	for range 5 {
		memoReads(t, root, snapshots)
	}
	if opened := catalog.PinnedOpens() - before; opened != 0 {
		t.Fatalf("repeated reads of an unchanged generation opened %d databases, want 0", opened)
	}
}

func TestReadsOfAnUnchangedGenerationWriteNoPinFiles(t *testing.T) {
	root := newBundleWorkspace(t)
	settleHub(t, root)
	snapshots := NewSnapshotService()
	memoReads(t, root, snapshots)

	pins := filepath.Join(root, "runtime", "catalog", "pins")
	before := countPathEntries(t, pins)
	memoReads(t, root, snapshots)
	if after := countPathEntries(t, pins); after != before {
		t.Fatalf("pins directory changed from %d to %d entries", before, after)
	}
}

func countPathEntries(t *testing.T, directory string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(directory, func(string, os.DirEntry, error) error { count++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestRebuildInvalidatesTheMemoizedManifest(t *testing.T) {
	root := newBundleWorkspace(t)
	settleHub(t, root)
	snapshots := NewSnapshotService()
	first := memoReads(t, root, snapshots)

	writeSkillFile(t, root, "bundle-skill", "references/guide.md", "# Guide\n\nChanged after the first read.\n")
	rebuildBundleCatalog(t, root)

	before := catalog.PinnedOpens()
	second := memoReads(t, root, snapshots)
	if catalog.PinnedOpens() == before {
		t.Fatal("a new generation was served without opening its database")
	}
	if second.Version == first.Version || second.URI == first.URI {
		t.Fatalf("manifest still has the old version %s after a rebuild", first.Version)
	}
	content, err := (DistributionService{}).ReadResource(t.Context(), root, resourceURI(t, second, "references/guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content.Bytes), "Changed after the first read") {
		t.Fatalf("resource = %q, want the edited content", content.Bytes)
	}
}

func resourceURI(t *testing.T, entry DistributedSkill, relative string) string {
	t.Helper()
	for _, resource := range entry.Resources {
		if strings.HasSuffix(resource.URI, "/"+relative) {
			return resource.URI
		}
	}
	t.Fatalf("manifest has no %s: %v", relative, entry.Resources)
	return ""
}

func TestEditedFileIsNoticedWhileTheGenerationIsServedAsFallback(t *testing.T) {
	root := newBundleWorkspace(t)
	settleHub(t, root)
	snapshots := NewSnapshotService()
	memoReads(t, root, snapshots)

	// A second, broken skill keeps the catalog from rebuilding, so the published
	// generation stays current for the memo while a file of the first changes.
	if err := os.MkdirAll(filepath.Join(root, "skills", "core", "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "broken", "skill.meta.yaml"), []byte("id: [unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, root, "bundle-skill", "references/guide.md", "# Guide\n\nEdited without a rebuild.\n")

	_, _, skipped, err := (DistributionService{}).ListSkillsReport(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	var reason string
	for _, item := range skipped {
		if item.SkillID == "bundle-skill" {
			reason = item.Reason
		}
	}
	if !strings.Contains(reason, "digest mismatch") {
		t.Fatalf("skipped reason = %q, want a digest mismatch for the edited file", reason)
	}
	_, _, err = (DistributionService{}).LookupSkills(t.Context(), root, []string{"bundle-skill"})
	if !errors.Is(err, ErrResourceContentUnavailable) {
		t.Fatalf("lookup of the edited skill = %v, want %v", err, ErrResourceContentUnavailable)
	}
}

func TestCorruptedGenerationIsDetectedAfterTheMemoFilled(t *testing.T) {
	root := newBundleWorkspace(t)
	settleHub(t, root)
	snapshots := NewSnapshotService()
	first := memoReads(t, root, snapshots)

	pointer, err := catalog.CurrentPointer(root)
	if err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(root, "runtime", "catalog", filepath.FromSlash(pointer.Database))
	contents, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	for index := len(contents) / 2; index < len(contents)/2+512; index++ {
		contents[index] ^= 0xff
	}
	if err := os.Chmod(database, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(database, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	before := catalog.PinnedOpens()
	second := memoReads(t, root, snapshots)
	after, err := catalog.CurrentPointer(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Generation == pointer.Generation {
		t.Fatal("a corrupted generation kept serving")
	}
	if catalog.PinnedOpens() == before {
		t.Fatal("the replacement generation was served from the corrupted one's memo")
	}
	if second.Version != first.Version {
		t.Fatalf("manifest version %s changed to %s although no skill file did", first.Version, second.Version)
	}
}

func TestMemoizedResolverInputsEqualTheDatabaseOnes(t *testing.T) {
	root := newBundleWorkspace(t)
	view, err := catalog.ObserveWithFallbackLocked(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	database, err := view.DB(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	direct, err := resolverpkg.NewSQLiteCatalog(database, view.Pointer.CatalogSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	want, err := direct.Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	memoized := memoSkillsCatalog{Catalog: direct, view: view}
	for range 2 {
		got, err := memoized.Skills(t.Context())
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("memoized skills = %#v, %v; want %#v", got, err, want)
		}
	}
	wantPolicy, err := resolverpkg.LoadPolicy(t.Context(), database)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if got, err := generationPolicy(t.Context(), view); err != nil || !reflect.DeepEqual(got, wantPolicy) {
			t.Fatalf("memoized policy = %#v, %v; want %#v", got, err, wantPolicy)
		}
	}
}
