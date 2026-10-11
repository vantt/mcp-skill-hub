package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runInitTo runs `skillhub init args...` with dir as the current folder,
// creating dir first when it is missing. Tests never change the process
// working directory, because most of them run in parallel.
func runInitTo(t *testing.T, dir string, stdout, stderr io.Writer, args ...string) int {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return runInitIn(context.Background(), dir, args, stdout, stderr)
}

// runInitCLI is runInitTo with the output returned as strings.
func runInitCLI(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runInitTo(t, dir, &stdout, &stderr, args...)
	return code, stdout.String(), stderr.String()
}

// initHub creates a hub in dir and returns dir.
func initHub(t *testing.T, dir string) string {
	t.Helper()
	if code, stdout, stderr := runInitCLI(t, dir); code != 0 {
		t.Fatalf("init in %s: exit %d\n%s%s", dir, code, stdout, stderr)
	}
	return dir
}

// runConnectCLI runs `skillhub connect args...` with dir as the project
// folder, the way a user runs it after `cd dir`.
func runConnectCLI(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runConnectIn(context.Background(), dir, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// gitOutput runs Git in dir and returns its trimmed standard output.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v in %s: %v: %s", args, dir, err, stderr.String())
	}
	return strings.TrimSpace(string(output))
}
