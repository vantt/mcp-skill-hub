package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestVersionJSONOutputUsesResultEnvelope(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"version", "--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}

	var result app.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("version JSON is invalid: %v\noutput: %s", err, stdout.String())
	}
	if result.SchemaVersion != app.ResultSchemaVersion {
		t.Errorf("schema_version = %q, want %q", result.SchemaVersion, app.ResultSchemaVersion)
	}
	if result.Status != app.StatusOK {
		t.Errorf("status = %q, want %q", result.Status, app.StatusOK)
	}
	if result.Summary == "" {
		t.Error("summary must not be empty")
	}
	if result.Items == nil || result.SuggestedActions == nil || result.Warnings == nil {
		t.Error("required array fields must be present")
	}
	if result.Error != nil {
		t.Errorf("error = %#v, want nil", result.Error)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestInitPreviewsUntilYes(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if exitCode := Run([]string{"init", root}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("init preview exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("init preview wrote workspace: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := Run([]string{"init", root, "--yes"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("confirmed init exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatalf("confirmed init did not create workspace: %v", err)
	}
}

func TestInitDefaultsToCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	if exitCode := Run([]string{"init"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("init preview exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read current directory after preview: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("init preview wrote to the current directory: %v", entries)
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := Run([]string{"init", "--yes"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("confirmed init exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); err != nil {
		t.Fatalf("confirmed init did not initialize the current directory: %v", err)
	}
}

func TestInitRejectsMultipleWorkspacePaths(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if exitCode := Run([]string{"init", "first", "second"}, &stdout, &stderr); exitCode != 2 {
		t.Fatalf("init exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "init accepts at most one workspace path") {
		t.Fatalf("unexpected init error: %q", stderr.String())
	}
}

func TestInvalidRequestWriteFailureReturnsRuntimeExitCode(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	exitCode := Run([]string{"unknown", "--json"}, failingWriter{}, &stderr)
	if exitCode != 1 {
		t.Fatalf("Run() exit code = %d, want 1", exitCode)
	}
}

func TestUnsupportedCommandReturnsStructuredJSONError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	exitCode := Run([]string{"unknown", "--json"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}

	var result app.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("error JSON is invalid: %v", err)
	}
	if result.Status != app.StatusError || result.Error == nil || result.Error.Code != app.ErrorInvalidRequest {
		t.Errorf("unexpected error result: %#v", result)
	}
}
