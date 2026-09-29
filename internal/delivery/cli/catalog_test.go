package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestRebuildCancellationUsesStructuredContract(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdout.Reset()
	stderr.Reset()
	if code := RunContext(ctx, []string{"rebuild", "--workspace", root, "--json"}, &stdout, &stderr); code != 130 {
		t.Fatalf("cancelled rebuild exit = %d: %s", code, stderr.String())
	}
	var result app.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == nil || result.Error.Code != app.ErrorOperationCancelled {
		t.Fatalf("cancelled result = %#v", result)
	}
}

func TestRebuildHumanOutputIncludesProgress(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"rebuild", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild exit = %d: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "[0/5]") || !strings.Contains(stderr.String(), "[5/5]") {
		t.Fatalf("rebuild progress = %q", stderr.String())
	}
}

func TestRebuildUsesSharedResultEnvelope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr.String())
	}
	if err := os.RemoveAll(filepath.Join(root, "runtime")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"rebuild", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild exit = %d: %s", code, stderr.String())
	}
	var result app.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != app.ResultSchemaVersion || result.Status != app.StatusApplied || result.Error != nil {
		t.Fatalf("unexpected rebuild result: %#v", result)
	}
}
