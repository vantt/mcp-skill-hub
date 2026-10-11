package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitNextStepStagesWorkspaceFoldersByName(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}
	text := strings.Join(strings.Fields(stdout.String()), " ")
	if strings.Contains(text, "add -A") || !strings.Contains(text, " add .gitignore .skillhub skills ") {
		t.Fatalf("the commit hint must name the folders it stages:\n%s", stdout.String())
	}
}

func TestStatusNextStepOfAFreshWorkspaceIsARealCommand(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}
	stdout.Reset()
	if code := Run([]string{"status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("status=%d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "workspace_diff") || strings.Contains(stdout.String(), "--json") {
		t.Fatalf("the next step must be a command a person can run:\n%s", stdout.String())
	}
}
