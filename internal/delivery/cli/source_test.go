package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestSourceCaptureListAndExplicitEmptyCheck(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"source", "capture", "https://github.com/example/repo.git", "--reason", "review later", "--workspace", root}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "Candidate: SRCQ-") {
		t.Fatalf("capture code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = RunContext(context.Background(), []string{"source", "list", "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"status":"pending"`) {
		t.Fatalf("list code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = RunContext(context.Background(), []string{"check", "--all-due", "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"checked":0`) {
		t.Fatalf("check code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSourceCaptureRejectsCredentialBearingLocator(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"source", "capture", "https://user:secret@example.com/repo.git", "--reason", "unsafe", "--workspace", root}, &stdout, &stderr)
	if code != 2 || strings.Contains(stdout.String()+stderr.String(), "secret") {
		t.Fatalf("credential rejection code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
