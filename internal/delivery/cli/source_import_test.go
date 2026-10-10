package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

func TestSourceImportLocatorPreviewAndBoundConfirm(t *testing.T) {
	t.Parallel()
	root, repo, service := sourceImportLocatorFixture(t)
	code, _, stderr := runCLIForTest([]string{"skill", "create", "conflict-skill", "--collection", "default", "--name", "Conflict skill", "--description", "Existing skill", "--workspace", root, "--yes"})
	if code != 0 {
		t.Fatalf("create existing skill: %d %s", code, stderr)
	}
	before := sourceImportCanonicalSnapshot(t, root)
	flags := sourceFlags{workspace: root, ref: "main", sourcePath: "skills", all: true, jsonOutput: true}
	code, stdout, stderr := runSourceImportForTest(t, service, flags, "file://"+filepath.ToSlash(repo))
	if code != 0 || stderr != "" {
		t.Fatalf("preview: %d %s %s", code, stdout, stderr)
	}
	var preview app.SourceImportProposal
	if err := json.Unmarshal([]byte(stdout), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Status != app.StatusActionRequired || len(preview.Discovered) != 2 || len(preview.Importable) != 1 || len(preview.Skipped) != 1 {
		t.Fatalf("unexpected discovery envelope: %s", stdout)
	}
	if !preview.Skipped[0].Conflict || preview.Skipped[0].TargetID != "conflict-skill" || preview.Skipped[0].SkipReason == "" {
		t.Fatalf("missing conflict details: %s", stdout)
	}
	if got := sourceImportCanonicalSnapshot(t, root); !reflect.DeepEqual(before, got) {
		t.Fatalf("preview changed canonical files: before=%v after=%v", before, got)
	}
	pins := preview.Confirmation.Confirmation.Pins
	if pins.ProposalID == "" || pins.ProposalDigest == "" || pins.BaseVersion == "" {
		t.Fatalf("missing bound confirmation pins: %#v", pins)
	}
	if preview.CLI == "" {
		t.Fatalf("import preview missing runnable confirmation: %s", stdout)
	}

	// A reviewed proposal must remain independent of later upstream changes and availability.
	writeSourceImportFixtureSkill(t, repo, "skills/reviewed-skill", "reviewed-skill", "Unreviewed replacement")
	writeSourceImportFixtureSkill(t, repo, "skills/late-skill", "late-skill", "Added after review")
	sourceImportGit(t, repo, "add", ".")
	sourceImportGit(t, repo, "commit", "-m", "upstream changed after preview")
	if err := os.Rename(repo, repo+"-unavailable"); err != nil {
		t.Fatal(err)
	}
	args := []string{"source", "import", "--proposal", pins.ProposalID, "--proposal-digest", "sha256:wrong", "--base-version", pins.BaseVersion, "--yes", "--workspace", root, "--json"}
	code, stdout, stderr = runCLIForTest(args)
	if code != 2 || !strings.Contains(stdout, "stale_proposal") {
		t.Fatalf("wrong digest accepted: %d %s %s", code, stdout, stderr)
	}
	if got := sourceImportCanonicalSnapshot(t, root); !reflect.DeepEqual(before, got) {
		t.Fatal("wrong digest changed canonical files")
	}
	args[5] = pins.ProposalDigest
	args[7] = "unreviewed-base"
	code, stdout, stderr = runCLIForTest(args)
	if code != 2 || !strings.Contains(stdout, "stale_proposal") {
		t.Fatalf("wrong base version accepted: %d %s %s", code, stdout, stderr)
	}
	if got := sourceImportCanonicalSnapshot(t, root); !reflect.DeepEqual(before, got) {
		t.Fatal("wrong base version changed canonical files")
	}
	code, stdout, stderr = runCLIForTest(strings.Fields(preview.CLI)[1:])
	if code != 0 || stderr != "" {
		t.Fatalf("bound confirm: %d %s %s", code, stdout, stderr)
	}
	var applied app.SourceImportResult
	if err := json.Unmarshal([]byte(stdout), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Status != app.StatusApplied || applied.ImportedCount != 1 || applied.SkippedCount != 1 {
		t.Fatalf("unexpected apply: %s", stdout)
	}
	for _, changed := range applied.ChangedPaths {
		if !strings.HasPrefix(changed, "skills/default/reviewed-skill/") && !sourceImportOperationReceipt(changed, applied.OperationID) {
			t.Fatalf("import wrote more than the reviewed draft: %s", changed)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "skills", "default", "reviewed-skill", "SKILL.md"))
	if err != nil || !strings.Contains(string(content), "Reviewed instructions") || strings.Contains(string(content), "Unreviewed replacement") {
		t.Fatalf("confirmation regenerated content: %s %v", content, err)
	}
	after := sourceImportCanonicalSnapshot(t, root)
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("import modified existing canonical file %s", path)
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed && !strings.HasPrefix(path, "skills/default/reviewed-skill/") && !sourceImportOperationReceipt(path, applied.OperationID) {
			t.Fatalf("import created non-draft canonical file %s", path)
		}
	}
	assertSourceImportDraft(t, root, "reviewed-skill", "skills/reviewed-skill")
}

func TestSourceImportLocatorImmediateDraftsAndSelection(t *testing.T) {
	t.Parallel()
	root, repo, service := sourceImportLocatorFixture(t)
	flags := sourceFlags{workspace: root, ref: "main", sourcePath: "skills", skills: []string{"reviewed-skill"}, yes: true, jsonOutput: true}
	code, stdout, stderr := runSourceImportForTest(t, service, flags, "file://"+filepath.ToSlash(repo))
	if code != 0 || stderr != "" {
		t.Fatalf("immediate apply: %d %s %s", code, stdout, stderr)
	}
	var result app.SourceImportResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != app.StatusApplied || !reflect.DeepEqual(result.ImportedIDs, []string{"reviewed-skill"}) {
		t.Fatalf("selection was not respected: %s", stdout)
	}
	assertSourceImportDraft(t, root, "reviewed-skill", "skills/reviewed-skill")
	if _, err := os.Stat(filepath.Join(root, "skills", "default", "conflict-skill")); !os.IsNotExist(err) {
		t.Fatalf("unselected skill was imported: %v", err)
	}
	for _, path := range result.ChangedPaths {
		if !strings.HasPrefix(path, "skills/default/reviewed-skill/") && !sourceImportOperationReceipt(path, result.OperationID) {
			t.Fatalf("unexpected canonical write: %s", path)
		}
	}
}

func TestSourceImportLocatorUsesExplicitRef(t *testing.T) {
	t.Parallel()
	root, repo, service := sourceImportLocatorFixture(t)
	sourceImportGit(t, repo, "checkout", "-b", "review-branch")
	writeSourceImportFixtureSkill(t, repo, "skills/branch-skill", "branch-skill", "Only on the selected ref")
	sourceImportGit(t, repo, "add", ".")
	sourceImportGit(t, repo, "commit", "-m", "selected ref skill")
	sourceImportGit(t, repo, "checkout", "main")
	flags := sourceFlags{workspace: root, ref: "review-branch", sourcePath: "skills", skills: []string{"branch-skill"}, jsonOutput: true}
	code, stdout, stderr := runSourceImportForTest(t, service, flags, "file://"+filepath.ToSlash(repo))
	if code != 0 || stderr != "" {
		t.Fatalf("ref preview: %d %s %s", code, stdout, stderr)
	}
	var preview app.SourceImportProposal
	if err := json.Unmarshal([]byte(stdout), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Importable) != 1 || preview.Importable[0].TargetID != "branch-skill" {
		t.Fatalf("explicit ref was ignored: %s", stdout)
	}
}

func TestSourceImportBoundConfirmationValidation(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{},
		{"one", "two"},
		{"https://github.com/example/skills", "--all", "--skill", "one"},
		{"--proposal", "PROP-test", "--proposal-digest", "digest", "--base-version", "base"},
		{"--proposal", "PROP-test", "--yes"},
		{"one", "--proposal", "PROP-test", "--proposal-digest", "digest", "--base-version", "base", "--yes"},
		{"--proposal", "PROP-test", "--proposal-digest", "digest", "--base-version", "base", "--path", "skills", "--yes"},
		{"--proposal", "PROP-test", "--proposal-digest", "digest", "--base-version", "base", "--skill", "one", "--yes"},
		{"one", "--proposal-digest", "digest"},
		{"one", "--base-version", "base"},
	} {
		code, stdout, stderr := runCLIForTest(append([]string{"source", "import", "--json"}, args...))
		if code != 2 || !strings.Contains(stdout, "invalid_request") || stderr != "" {
			t.Fatalf("args %v: %d %s %s", args, code, stdout, stderr)
		}
	}
}

func sourceImportLocatorFixture(t *testing.T) (string, string, app.SourceImportService) {
	t.Helper()
	root := newSourceCLIWorkspace(t)
	repo := t.TempDir()
	sourceImportGit(t, repo, "init", "-b", "main")
	sourceImportGit(t, repo, "config", "user.name", "Test")
	sourceImportGit(t, repo, "config", "user.email", "test@example.com")
	sourceImportGit(t, repo, "config", "uploadpack.allowReachableSHA1InWant", "true")
	writeSourceImportFixtureSkill(t, repo, "skills/reviewed-skill", "reviewed-skill", "Reviewed instructions")
	writeSourceImportFixtureSkill(t, repo, "skills/conflict-skill", "conflict-skill", "Conflicting instructions")
	writeSourceImportFixtureSkill(t, repo, "outside/other-skill", "other-skill", "Outside requested path")
	sourceImportGit(t, repo, "add", ".")
	sourceImportGit(t, repo, "commit", "-m", "reviewed source")
	adapter := sourcepkg.GitRepositoryAdapter{CacheRoot: filepath.Join(root, "runtime", "sources", "git"), AllowFileProtocol: true}
	return root, repo, app.SourceImportService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
}

func runSourceImportForTest(t *testing.T, service app.SourceImportService, flags sourceFlags, locator string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runSourceImport(context.Background(), service, flags, []string{locator}, commandOutput{Writer: &stdout, args: []string{"--workspace", flags.workspace}}, &stderr)
	return code, stdout.String(), stderr.String()
}

func assertSourceImportDraft(t *testing.T, root, id, upstreamPath string) {
	t.Helper()
	result, err := (app.SkillService{}).ReadSkill(context.Background(), root, id)
	if err != nil || result.Manifest.Status != "draft" {
		t.Fatalf("imported skill must remain draft: %#v %v", result.Manifest, err)
	}
	trust, err := (app.SkillService{}).ContentTrustFor(context.Background(), root, id)
	if err != nil || !trust.RequiresReview() || trust.Approved {
		t.Fatalf("import bypassed content review: %#v %v", trust, err)
	}
	meta, err := os.ReadFile(filepath.Join(root, "skills", "default", id, ".meta", "skill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var provenance struct {
		Sources []struct {
			Path string `yaml:"path"`
		} `yaml:"sources"`
	}
	if err := yaml.Unmarshal(meta, &provenance); err != nil {
		t.Fatal(err)
	}
	if len(provenance.Sources) != 1 || provenance.Sources[0].Path != upstreamPath {
		t.Fatalf("unexpected scoped upstream provenance, want %q: %#v", upstreamPath, provenance.Sources)
	}
}

func sourceImportOperationReceipt(path, operationID string) bool {
	return strings.HasPrefix(path, "history/operations/") && strings.HasSuffix(path, "/"+operationID+".yaml")
}

func sourceImportCanonicalSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if rel == "runtime" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil {
			snapshot[filepath.ToSlash(rel)] = string(data)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func writeSourceImportFixtureSkill(t *testing.T, repo, path, name, body string) {
	t.Helper()
	dir := filepath.Join(repo, filepath.FromSlash(path))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: Source import fixture\n---\n# "+name+"\n\n"+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sourceImportGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s %v", args, output, err)
	}
}

func TestSourceImportExplicitRefDoesNotUseStoredRevision(t *testing.T) {
	t.Parallel()
	root, repo, service := sourceImportLocatorFixture(t)
	locator := sourcepkg.Locator{Repository: "file://" + filepath.ToSlash(repo), Ref: "main", Path: "skills"}
	limits := sourcepkg.Limits{TimeoutSeconds: 20, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize}
	revision, err := service.Adapters["git"].CurrentRevision(context.Background(), sourcepkg.Source{ID: "existing-source", Locator: locator, Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	record := sourcepkg.Record{
		SchemaVersion: 1, ID: "existing-source", Adapter: "git", Locator: locator,
		Status: "watching", Identity: sourcepkg.Identity{Name: "existing-source", Canonical: locator.Repository},
		Limits: limits, Monitoring: sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"}, CurrentRevision: &revision,
	}
	data, err := sourcepkg.MarshalCanonical(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", record.ID+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	before := sourceImportCanonicalSnapshot(t, root)
	flags := sourceFlags{workspace: root, ref: "missing-ref", skills: []string{"reviewed-skill"}, yes: true, jsonOutput: true}
	code, stdout, stderr := runSourceImportForTest(t, service, flags, record.ID)
	if code != 2 || !strings.Contains(stdout, "invalid_request") || stderr != "" {
		t.Fatalf("missing ref silently used stored bytes: %d %s %s", code, stdout, stderr)
	}
	if got := sourceImportCanonicalSnapshot(t, root); !reflect.DeepEqual(before, got) {
		t.Fatal("failed ref override changed canonical files")
	}
}
