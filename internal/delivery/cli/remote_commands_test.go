package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/vantt/mcp-skill-hub/internal/app"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestRemoteAddAndWatchCLIConfirmStoredPreviews(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	repoDir := t.TempDir()
	repository, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(repoDir, "remote-tool")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: remote-tool\ndescription: Remote review instructions\n---\n\n# Reviewed remote instructions\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Add("."); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Commit("Reviewed source", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.invalid", When: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}); err != nil {
		t.Fatal(err)
	}
	adapter := sourcepkg.GitRepositoryAdapter{CacheRoot: filepath.Join(root, "runtime", "sources", "git"), AllowFileProtocol: true}
	adapters := map[string]sourcepkg.Adapter{"git": adapter}
	locator := localFileURL(repoDir)
	// The local Git transport makes remote-service discovery deterministic. The
	// delivered commands go through the real CLI stored-proposal confirmation.
	preview, err := (app.SkillAddService{Adapters: adapters}).PreviewSkillAdd(context.Background(), root, app.SkillAddInput{Locator: locator, Selection: "remote-tool"})
	if err != nil || preview.Error != nil {
		t.Fatalf("add preview: %v %+v", err, preview.Error)
	}
	runDeliveredConfirm(t, root, preview)
	actual, err := os.ReadFile(filepath.Join(root, "skills", "default", "remote-tool", "SKILL.md"))
	if err != nil || !strings.Contains(string(actual), "# Reviewed remote instructions") {
		t.Fatalf("remote preview not applied: %q %v", actual, err)
	}
	// Watch uses a different proposal family and must dispatch source confirm,
	// not regenerate a watch or accidentally confirm through the skill service.
	root = initTestWorkspace(t)
	code, output, stderr := runCLI(t, "skill", "create", "remote-tool", "--collection", "default", "--name", "Remote Tool", "--description", "Review remote tools", "--workspace", root, "--yes")
	if code != 0 {
		t.Fatalf("watch skill: %d %s %s", code, output, stderr)
	}
	adapter.CacheRoot = filepath.Join(root, "runtime", "sources", "git")
	adapters = map[string]sourcepkg.Adapter{"git": adapter}
	watched, err := (app.SourceService{Adapters: adapters}).PreviewSourceWatch(context.Background(), root, app.SourceWatchInput{Locator: locator, SkillID: "remote-tool", Cadence: "daily"})
	if err != nil || watched.Error != nil {
		t.Fatalf("watch preview: %v %+v", err, watched.Error)
	}
	if watched.Confirmation.Confirmation.Required {
		runDeliveredConfirm(t, root, watched)
		code, output, stderr := runCLI(t, "source", "list", "--workspace", root, "--json")
		if code != 0 || !strings.Contains(output, watched.Source.ID) {
			t.Fatalf("watched source absent: %d %s %s", code, output, stderr)
		}
	} else {
		t.Fatal("watch fixture did not produce a reviewed mutation")
	}
}

func runDeliveredConfirm(t *testing.T, root string, preview any) {
	t.Helper()
	var output bytes.Buffer
	if err := writeJSON(commandOutput{Writer: &output, args: []string{"--workspace", root}}, preview); err != nil {
		t.Fatal(err)
	}
	var delivered struct {
		CLI string `json:"cli"`
	}
	if err := json.Unmarshal(output.Bytes(), &delivered); err != nil {
		t.Fatal(err)
	}
	if delivered.CLI == "" {
		t.Fatalf("missing bound command: %s", output.String())
	}
	code, stdout, stderr := runCLI(t, splitCLI(delivered.CLI)[1:]...)
	if code != 0 {
		t.Fatalf("delivered confirm %s: %d %s %s", delivered.CLI, code, stdout, stderr)
	}
}
