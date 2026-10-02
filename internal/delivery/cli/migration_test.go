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

func TestMigrateCLIJSONPreviewAndConfirmation(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr.String())
	}
	if err := os.Remove(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"migrate", "--workspace", root, "--to", "1", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("preview exit = %d: %s", code, stderr.String())
	}
	var preview app.MigrationResult
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Status != app.StatusActionRequired || preview.SourceSchemaVersion != 0 || preview.TargetSchemaVersion != 1 || len(preview.Changes) != 1 || preview.Receipt != nil {
		t.Fatalf("preview = %#v", preview)
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); !os.IsNotExist(err) {
		t.Fatalf("preview wrote marker: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"migrate", "--workspace", root, "--yes", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("confirm exit = %d: %s", code, stderr.String())
	}
	var applied app.MigrationResult
	if err := json.Unmarshal(stdout.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Status != app.StatusApplied || applied.Receipt == nil || applied.Receipt.SourceSchemaVersion != 0 || applied.Receipt.TargetSchemaVersion != 1 {
		t.Fatalf("applied = %#v", applied)
	}
}

func TestMigrateCLIHumanPreviewIncludesPinnedDiff(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr.String())
	}
	if err := os.Remove(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"migrate", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("preview exit = %d: %s", code, stderr.String())
	}
	output := stdout.String()
	for _, expected := range []string{"Source schema version:  0", "Target schema version:  1", "Proposal digest:        sha256:", "+++ b/.skillhub/schema-version", "+1"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("human preview lacks %q:\n%s", expected, output)
		}
	}
}

func TestMigrateCLIValidatesFlags(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"migrate", "--to", "nope", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("invalid flags exit = %d", code)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("invalid flags JSON = %q", stdout.String())
	}
}
