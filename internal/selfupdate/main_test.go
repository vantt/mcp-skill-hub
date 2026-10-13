package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	cleanup := shadowHostCosign()
	code := m.Run()
	cleanup()
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
