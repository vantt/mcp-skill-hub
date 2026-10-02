package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSkillReviewHumanAndJSON(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: rev-test\ndescription: Review test skill.\n---\n\n# Review Test\n\nInstructions.\n"), 0o600)

	code, _, stderr := runCLI(t, "skill", "create", "rev-test", "--workspace", root,
		"--collection", "core", "--name", "Review Test", "--description", "Review test skill.",
		"--trigger", "review it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile, "--yes")
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}

	// 1. Human review output
	code, stdout, stderr := runCLI(t, "skill", "review", "rev-test", "--workspace", root)
	if code != 0 {
		t.Fatalf("review failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Review: Review Test (rev-test)") {
		t.Errorf("review missing title line: %s", stdout)
	}
	if !strings.Contains(stdout, "State: draft") {
		t.Errorf("review missing draft state: %s", stdout)
	}
	if !strings.Contains(stdout, "Files:") {
		t.Errorf("review missing files line: %s", stdout)
	}
	if !strings.Contains(stdout, "Origin: local authoring") {
		t.Errorf("review missing local authoring origin: %s", stdout)
	}
	if !strings.Contains(stdout, "Next:") {
		t.Errorf("review missing Next action: %s", stdout)
	}

	// 2. Verbose review output
	code, stdout, stderr = runCLI(t, "skill", "review", "rev-test", "--workspace", root, "--verbose")
	if code != 0 {
		t.Fatalf("verbose review failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Catalog facts:") {
		t.Errorf("verbose review missing catalog facts: %s", stdout)
	}
	if !strings.Contains(stdout, "Canonical entrypoint:") {
		t.Errorf("verbose review missing canonical entrypoint: %s", stdout)
	}

	// 3. JSON review output
	var jsonBuf, jsonErrBuf bytes.Buffer
	code = Run([]string{"skill", "review", "rev-test", "--workspace", root, "--json"}, &jsonBuf, &jsonErrBuf)
	if code != 0 {
		t.Fatalf("json review failed (exit %d): %s", code, jsonErrBuf.String())
	}
	var result app.SkillReviewResult
	if err := json.Unmarshal(jsonBuf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal review JSON: %v", err)
	}
	if result.SkillID != "rev-test" || result.Collection != "core" || result.LifecycleState != "draft" {
		t.Errorf("unexpected review JSON content: %#v", result)
	}

	// 4. Missing skill review returns error exit 2
	code, _, stderr = runCLI(t, "skill", "review", "non-existent", "--workspace", root)
	if code != 2 {
		t.Fatalf("expected exit 2 on missing skill review, got %d", code)
	}
	if !strings.Contains(stderr, "skill not found") {
		t.Errorf("expected skill not found in stderr, got: %s", stderr)
	}
}
