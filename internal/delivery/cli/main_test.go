package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"testing"
)

// cliHelperEnv makes the test binary act as the skillhub command, so a test
// can run a command as a real subprocess (for example a stdio MCP server).
const cliHelperEnv = "SKILLHUB_CLI_TEST_HELPER"

// Isolate host configuration before parallel tests or subprocesses run.
func TestMain(m *testing.M) {
	if os.Getenv(cliHelperEnv) == "1" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		code := RunContext(ctx, os.Args[1:], os.Stdout, os.Stderr)
		stop()
		os.Exit(code)
	}
	cleanupCosign := shadowHostCosign()
	// Preserve HOME/XDG-derived Go paths so subprocess builds do not populate
	// the isolated home with read-only module cache files.
	args := []string{"env", "-json"}
	for _, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		if os.Getenv(key) == "" {
			args = append(args, key)
		}
	}
	if len(args) > 2 {
		output, err := exec.Command("go", args...).Output()
		var paths map[string]string
		if err == nil {
			err = json.Unmarshal(output, &paths)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve Go cache paths before HOME isolation: %v\n", err)
			os.Exit(1)
		}
		for _, key := range args[2:] {
			if err := os.Setenv(key, paths[key]); err != nil {
				fmt.Fprintf(os.Stderr, "pin %s before HOME isolation: %v\n", key, err)
				os.Exit(1)
			}
		}
	}
	root, err := os.MkdirTemp("", "skillhub-cli-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, key := range []string{
		"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		"XDG_STATE_HOME", "XDG_RUNTIME_DIR", "XDG_CONFIG_DIRS", "XDG_DATA_DIRS",
		"APPDATA", "LOCALAPPDATA", "SKILLHUB_USER_CONFIG_DIR",
	} {
		directory := filepath.Join(root, key)
		err = os.Mkdir(directory, 0o700)
		if err == nil {
			err = os.Setenv(key, directory)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "isolate %s: %v\n", key, err)
			if cleanupErr := os.RemoveAll(root); cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "clean up isolated test files in %s before running tests: %v\n", root, cleanupErr)
			}
			os.Exit(1)
		}
	}
	code := m.Run()
	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintf(os.Stderr, "clean up isolated test files in %s (test exit code %d): %v\n", root, code, err)
		if code == 0 {
			code = 1
		}
	}
	cleanupCosign()
	os.Exit(code)
}

// shadowHostCosign puts a stub cosign that always succeeds ahead of any real one on PATH, so
// a host with cosign installed behaves like one without: these tests use unsigned fixture bundles.
func shadowHostCosign() (cleanup func()) {
	if _, err := exec.LookPath("cosign"); err != nil || runtime.GOOS == "windows" {
		return func() {}
	}
	dir, err := os.MkdirTemp("", "skillhub-cosign-stub-")
	if err != nil {
		return func() {}
	}
	stub := filepath.Join(dir, "cosign")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		return func() { os.RemoveAll(dir) }
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() { os.RemoveAll(dir) }
}
