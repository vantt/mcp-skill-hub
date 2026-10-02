package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func writeDistributionFixtureSkill(t *testing.T, root, id, trigger, entrypoint string, extra map[string][]byte) {
	t.Helper()
	directory := filepath.Join(root, "skills", "core", id)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := "schema_version: 1\nid: " + id + "\nname: " + id + "\nstatus: active\ndescription: Fixture skill " + id + ".\nrouting:\n  operations: [review]\n  triggers: [" + trigger + "]\n  not_for: [write marketing prose]\n  min_scope: multi_step\nquality:\n  reviewed: true\n"
	files := map[string][]byte{"skill.meta.yaml": []byte(metadata), "SKILL.md": []byte(entrypoint)}
	for name, contents := range extra {
		files[name] = contents
	}
	for name, contents := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newDistributionFixtureWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func rebuildFixtureCatalog(t *testing.T, root string) catalog.BuildResult {
	t.Helper()
	result, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func servableEntrypoint(id string) string {
	return "---\nname: " + id + "\ndescription: Fixture skill " + id + ".\n---\n\n# " + id + "\n"
}

func TestClarificationAnswerIsAcceptedByAnIndependentResolverInstance(t *testing.T) {
	t.Parallel()
	root := newDistributionFixtureWorkspace(t)
	writeDistributionFixtureSkill(t, root, "code-review", "review code changes pull requests", servableEntrypoint("code-review"), nil)
	rebuildFixtureCatalog(t, root)

	// The first and second resolutions use separate services and caches, as two
	// CLI processes would; nothing but the request itself may carry the contract.
	request := resolverpkg.Request{
		SchemaVersion: resolverpkg.SchemaVersion, RequestID: "req-first",
		Task: resolverpkg.Task{Description: "review code changes for pull requests"}, Operation: "review",
	}
	issued, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)
	if err != nil || issued.Status != resolverpkg.StatusNeedsContext || issued.Question == nil {
		t.Fatalf("first resolution = %#v, %v", issued, err)
	}

	answered := request
	answered.RequestID = "req-second"
	answered.Prior = &resolverpkg.Prior{ResolutionID: issued.ResolutionID, ContextRevision: issued.ContextRevision, Kind: "clarification", QuestionID: issued.Question.ID, Answer: "multi_step"}
	final, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, answered)
	if err != nil {
		t.Fatalf("independent instance rejected a valid prior: %v", err)
	}
	if final.Status != resolverpkg.StatusResolved || final.Primary == nil || final.Primary.ID != "code-review" {
		t.Fatalf("answered resolution = %#v", final)
	}

	forged := answered
	forgedPrior := *answered.Prior
	forgedPrior.ResolutionID = "res_forged"
	forged.Prior = &forgedPrior
	if _, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, forged); err == nil || !strings.Contains(err.Error(), "prior clarification was not issued") {
		t.Fatalf("forged prior error = %v", err)
	}
}

func TestResolveSkipsSkillWithUnservableFiles(t *testing.T) {
	t.Parallel()
	root := newDistributionFixtureWorkspace(t)
	// The frontmatter description is blank, so it can never be served.
	writeDistributionFixtureSkill(t, root, "broken-review", "review kafka consumer retries", "---\nname: broken-review\ndescription: \"\"\n---\n\n# Broken\n", nil)
	writeDistributionFixtureSkill(t, root, "working-review", "review kafka consumer retries", servableEntrypoint("working-review"), nil)
	build := rebuildFixtureCatalog(t, root)
	warned := false
	for _, warning := range build.Warnings {
		warned = warned || strings.Contains(warning, "broken-review")
	}
	if !warned {
		t.Fatalf("build did not warn about the unservable skill: %#v", build.Warnings)
	}

	request := resolverpkg.Request{
		SchemaVersion: resolverpkg.SchemaVersion, RequestID: "req-isolation",
		Task: resolverpkg.Task{Description: "review kafka consumer retries", Scope: "multi_step"}, Operation: "review",
	}
	response, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)
	if err != nil {
		t.Fatalf("one unservable skill failed the resolution: %v", err)
	}
	if response.Primary != nil && response.Primary.ID == "broken-review" {
		t.Fatalf("resolver recommended an unservable skill: %#v", response)
	}

	entries, _, skipped, err := (DistributionService{}).ListSkillsReport(t.Context(), root)
	if err != nil {
		t.Fatalf("listing failed for the whole workspace: %v", err)
	}
	if len(skipped) != 1 || skipped[0].SkillID != "broken-review" {
		t.Fatalf("skipped = %#v", skipped)
	}
	found := false
	for _, entry := range entries {
		found = found || entry.SkillID == "working-review"
		if entry.SkillID == "broken-review" {
			t.Fatalf("unservable skill was listed: %#v", entry)
		}
	}
	if !found {
		t.Fatalf("servable skill missing from listing: %#v", entries)
	}
}

func TestReadResourceDeliversNonUTF8TextAsBinaryWithMatchingDigest(t *testing.T) {
	t.Parallel()
	root := newDistributionFixtureWorkspace(t)
	invalid := []byte("id,value\n\xff\xfe,1\n")
	writeDistributionFixtureSkill(t, root, "data-review", "review csv exports", servableEntrypoint("data-review"), map[string][]byte{"assets/data.csv": invalid, "references/notes.md": []byte("# Notes\n")})
	rebuildFixtureCatalog(t, root)

	entries, _, err := (DistributionService{}).LookupSkills(t.Context(), root, []string{"data-review"})
	if err != nil {
		t.Fatal(err)
	}
	var advertised DistributedResource
	for _, resource := range entries["data-review"].Resources {
		if strings.HasSuffix(resource.URI, "/data.csv") {
			advertised = resource
		}
	}
	content, err := (DistributionService{}).ReadResource(t.Context(), root, advertised.URI)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content.Bytes)
	if "sha256:"+hex.EncodeToString(sum[:]) != advertised.Digest {
		t.Fatalf("delivered digest does not match the advertised digest %s", advertised.Digest)
	}
	if content.IsText() || content.MIMEType != "application/octet-stream" {
		t.Fatalf("invalid UTF-8 was labelled as text: %q", content.MIMEType)
	}
}

func TestResourceMIMETypesDoNotDependOnHostDatabase(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]string{
		"SKILL.md": "text/markdown", "references/a.md": "text/markdown", "assets/data.csv": "text/csv; charset=utf-8",
		"scripts/run.py": "text/x-python; charset=utf-8", "assets/cfg.yaml": "application/yaml", "assets/blob.bin": "application/octet-stream",
	} {
		if got := resourceMIMEType(path); got != want {
			t.Errorf("resourceMIMEType(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestDistributionReportsCorruptCatalogWithSentinelError(t *testing.T) {
	t.Parallel()
	root := newDistributionFixtureWorkspace(t)
	writeDistributionFixtureSkill(t, root, "code-review", "review code", servableEntrypoint("code-review"), nil)
	result := rebuildFixtureCatalog(t, root)

	// Make canonical invalid so auto-rebuild cannot run
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "code-review", "skill.meta.yaml"), []byte("schema_version: 1\nid: code-review\nstatus: shiny\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Corrupt SQLite database
	dbPath := filepath.Join(root, "runtime", "catalog", filepath.FromSlash(result.Pointer.Database))
	if err := os.Chmod(dbPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("corrupt SQLite content"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := (DistributionService{}).ListSkills(t.Context(), root); !errors.Is(err, catalog.ErrCatalogUnavailable) {
		t.Fatalf("ListSkills error = %v, want ErrCatalogUnavailable", err)
	}
	if _, err := (DistributionService{}).ReadResource(t.Context(), root, "skill://skillhub/"+strings.Repeat("a", 64)+"/code-review/SKILL.md"); !errors.Is(err, catalog.ErrCatalogUnavailable) {
		t.Fatalf("ReadResource error = %v, want ErrCatalogUnavailable", err)
	}
}

func TestResolverContinuityDuringInvalidCanonicalEdit(t *testing.T) {
	t.Parallel()
	root := newDistributionFixtureWorkspace(t)
	writeDistributionFixtureSkill(t, root, "skill-a", "review kafka consumer retries", servableEntrypoint("skill-a"), nil)
	writeDistributionFixtureSkill(t, root, "skill-b", "review database migrations", servableEntrypoint("skill-b"), nil)
	buildResult := rebuildFixtureCatalog(t, root)

	// 1. Invalidate skill-b with an invalid metadata status (BUG-07)
	badMeta := "schema_version: 1\nid: skill-b\nname: skill-b\nstatus: shiny\ndescription: Broken skill.\n"
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "skill-b", "skill.meta.yaml"), []byte(badMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Resolve for skill-a's task. It should succeed via fallback with a warning!
	request := resolverpkg.Request{
		SchemaVersion: resolverpkg.SchemaVersion, RequestID: "req-fallback",
		Task: resolverpkg.Task{Description: "review kafka consumer retries", Scope: "multi_step"}, Operation: "review",
	}
	response, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)
	if err != nil {
		t.Fatalf("resolve failed during unrelated canonical invalidation: %v", err)
	}
	if response.Primary == nil || response.Primary.ID != "skill-a" {
		t.Fatalf("expected skill-a to be recommended, got: %#v", response.Primary)
	}
	if response.CatalogSnapshot != buildResult.Pointer.CatalogSnapshot {
		t.Fatalf("expected catalog snapshot %q, got %q", buildResult.Pointer.CatalogSnapshot, response.CatalogSnapshot)
	}
	hasFallbackWarning := false
	for _, w := range response.Warnings {
		if strings.Contains(w, "fallback") || strings.Contains(w, "canonical validation failed") {
			hasFallbackWarning = true
			break
		}
	}
	if !hasFallbackWarning {
		t.Fatalf("expected fallback warning in response.Warnings: %#v", response.Warnings)
	}

	// 3. Show / GetSkill for skill-a succeeds
	distSkill, err := (DistributionService{}).GetSkill(t.Context(), root, response.Primary.URI)
	if err != nil {
		t.Fatalf("GetSkill failed for unchanged skill during fallback: %v", err)
	}
	if distSkill.SkillID != "skill-a" {
		t.Fatalf("got skill %q, want skill-a", distSkill.SkillID)
	}

	// 4. Now tamper with skill-a's entrypoint (break its live resource)
	tampered := "---\nname: skill-a\ndescription: Fixture skill skill-a.\n---\n\n# Tampered bytes\n"
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "skill-a", "SKILL.md"), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	// 5. Resolve again: skill-a MUST now be excluded!
	response2, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if response2.Primary != nil && response2.Primary.ID == "skill-a" {
		t.Fatalf("skill-a should be excluded when its live resources differ from generation digest: %#v", response2)
	}

	// 6. GetSkill for skill-a must now return ErrResourceContentUnavailable
	_, getErr := (DistributionService{}).GetSkill(t.Context(), root, response.Primary.URI)
	if !errors.Is(getErr, ErrResourceContentUnavailable) {
		t.Fatalf("GetSkill error = %v, want ErrResourceContentUnavailable", getErr)
	}

	// 7. ListSkillsReport must report skill-a in skipped
	entries, _, skipped, err := (DistributionService{}).ListSkillsReport(t.Context(), root)
	if err != nil {
		t.Fatalf("ListSkillsReport failed: %v", err)
	}
	foundSkippedA := false
	for _, sk := range skipped {
		if sk.SkillID == "skill-a" {
			foundSkippedA = true
			break
		}
	}
	if !foundSkippedA {
		t.Fatalf("expected skill-a to be skipped in ListSkillsReport: %#v", skipped)
	}
	for _, entry := range entries {
		if entry.SkillID == "skill-a" {
			t.Fatalf("skill-a should not be served: %#v", entry)
		}
	}
}
