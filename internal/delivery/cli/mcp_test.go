package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestMCPServeFlags(t *testing.T) {
	ws := initTestWorkspace(t)

	// Missing profile value
	var stdout, stderr bytes.Buffer
	code := Run([]string{"mcp", "serve", "--workspace", ws, "--profile"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "--profile requires a value") {
		t.Fatalf("missing profile code = %d, stderr = %q", code, stderr.String())
	}

	// Invalid profile
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"mcp", "serve", "--workspace", ws, "--profile", "bogus"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "invalid profile \"bogus\"") {
		t.Fatalf("invalid profile code = %d, stderr = %q", code, stderr.String())
	}

	// Help contains --profile
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"help", "mcp"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "--profile runtime|curation|all") {
		t.Fatalf("help mcp code = %d, stdout = %q", code, stdout.String())
	}
}
