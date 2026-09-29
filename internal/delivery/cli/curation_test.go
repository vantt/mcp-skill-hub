package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestStatusHumanJSONAndQuietUseOneModel(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr.String())
	}
	commitCLIWorkspace(t, root)

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("JSON status exit = %d: %s", code, stderr.String())
	}
	var home app.CurationHome
	if err := json.Unmarshal(stdout.Bytes(), &home); err != nil {
		t.Fatal(err)
	}
	if len(home.SuggestedActions) != 1 {
		t.Fatalf("JSON status actions = %#v", home.SuggestedActions)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("human status exit = %d: %s", code, stderr.String())
	}
	human := stdout.String()
	if !strings.Contains(human, home.Summary) || !strings.Contains(human, "Recommended next: "+home.SuggestedActions[0].Label) {
		t.Fatalf("human status did not render JSON model:\n%s\n%#v", human, home)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", "--workspace", root, "--quiet"}, &stdout, &stderr); code != 0 {
		t.Fatalf("quiet status exit = %d: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != string(home.Status) {
		t.Fatalf("quiet status = %q, want %q", got, home.Status)
	}
}

func TestStatusRejectsConflictingOutputModes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"status", "--json", "--quiet"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout.String(), `"code":"invalid_request"`) {
		t.Fatalf("JSON error = %s / %s", stdout.String(), stderr.String())
	}
}

func TestValidatePropagatesApplicationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := RunContext(ctx, []string{"validate", "--workspace", t.TempDir(), "--json"}, &stdout, &stderr); code != 130 {
		t.Fatalf("validate exit = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result app.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == nil || result.Error.Code != app.ErrorOperationCancelled {
		t.Fatalf("validate cancellation = %#v", result)
	}
}

func TestDiffOutputUsesRelativePaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	commitCLIWorkspace(t, root)
	path := filepath.Join(root, "skills", "demo", "sample", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"diff", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("diff exit = %d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), root) || !strings.Contains(stdout.String(), "skills/demo/sample/SKILL.md") {
		t.Fatalf("unsafe diff output: %s", stdout.String())
	}
}

func commitCLIWorkspace(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"-c", "user.name=Skill Hub Test", "-c", "user.email=test@skillhub.invalid", "commit", "-m", "initial"}} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
}
