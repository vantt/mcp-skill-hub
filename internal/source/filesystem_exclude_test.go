package source

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeCaptureFile(t *testing.T, root, rel, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestFilesystemCaptureExcludesSecretsAndBuildOutput(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	writeCaptureFile(t, src, "SKILL.md", "# skill\n", 0o644)
	writeCaptureFile(t, src, "scripts/run.sh", "echo hi\n", 0o644)
	writeCaptureFile(t, src, ".env", "TOKEN=abc\n", 0o644)
	writeCaptureFile(t, src, ".env.example", "TOKEN=\n", 0o644)
	writeCaptureFile(t, src, "node_modules/pkg/index.js", "module.exports = 1\n", 0o644)
	writeCaptureFile(t, src, "scripts/__pycache__/m.pyc", "x", 0o644)

	adapter := (FilesystemAdapter{Root: root, CacheRoot: filepath.Join(t.TempDir(), "cache")}).defaults()
	manifest, err := adapter.capture(context.Background(), "source")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, item := range manifest.Resources {
		got[item.Path] = true
	}
	for _, want := range []string{"SKILL.md", "scripts/run.sh", ".env.example"} {
		if !got[want] {
			t.Errorf("resource %q missing from %v", want, manifest.Resources)
		}
	}
	for _, left := range []string{".env", "node_modules/pkg/index.js", "scripts/__pycache__/m.pyc"} {
		if got[left] {
			t.Errorf("resource %q must not be captured", left)
		}
	}
	excluded := map[string]bool{}
	for _, path := range manifest.Excluded {
		excluded[path] = true
	}
	for _, want := range []string{".env", "node_modules", "scripts/__pycache__"} {
		if !excluded[want] {
			t.Errorf("Excluded = %v, want %q listed", manifest.Excluded, want)
		}
	}
}

func TestFilesystemCaptureDigestIgnoresExcludedFiles(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "plain")
	noisy := filepath.Join(root, "noisy")
	for _, dir := range []string{plain, noisy} {
		writeCaptureFile(t, dir, "SKILL.md", "# skill\n", 0o644)
	}
	writeCaptureFile(t, noisy, ".env", "TOKEN=abc\n", 0o644)
	adapter := (FilesystemAdapter{Root: root, CacheRoot: filepath.Join(t.TempDir(), "cache")}).defaults()
	a, err := adapter.capture(context.Background(), "plain")
	if err != nil {
		t.Fatal(err)
	}
	b, err := adapter.capture(context.Background(), "noisy")
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest != b.Digest {
		t.Fatalf("digest differs: %s vs %s; excluded files must not change it", a.Digest, b.Digest)
	}
}

func TestFilesystemCaptureRecordsExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	root := t.TempDir()
	src := filepath.Join(root, "source")
	writeCaptureFile(t, src, "scripts/bump.sh", "#!/bin/sh\n", 0o755)
	writeCaptureFile(t, src, "notes.md", "n\n", 0o644)
	adapter := (FilesystemAdapter{Root: root, CacheRoot: filepath.Join(t.TempDir(), "cache")}).defaults()
	manifest, err := adapter.capture(context.Background(), "source")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range manifest.Resources {
		want := item.Path == "scripts/bump.sh"
		if item.Executable != want {
			t.Errorf("%s Executable = %v, want %v", item.Path, item.Executable, want)
		}
	}
	// The mode survives the manifest round trip used by later reads.
	rev := Revision{Kind: "filesystem-snapshot", Value: manifest.Digest, ContentDigest: manifest.Digest}
	listed, err := adapter.List(context.Background(), Source{}, rev, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listed {
		if item.Path == "scripts/bump.sh" && !item.Executable {
			t.Errorf("listed %s lost its executable flag", item.Path)
		}
	}
}

func TestFilesystemCaptureExcludesSymlinkedSecretInsteadOfFailing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	src := filepath.Join(root, "source")
	writeCaptureFile(t, src, "SKILL.md", "# skill\n", 0o644)
	outside := filepath.Join(root, "outside-secret")
	if err := os.WriteFile(outside, []byte("TOKEN=abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, ".env")); err != nil {
		t.Fatal(err)
	}
	adapter := (FilesystemAdapter{Root: root, CacheRoot: filepath.Join(t.TempDir(), "cache")}).defaults()
	manifest, err := adapter.capture(context.Background(), "source")
	if err != nil {
		t.Fatalf("a symlinked .env must be left out, not fail the capture: %v", err)
	}
	if len(manifest.Excluded) != 1 || manifest.Excluded[0] != ".env" {
		t.Fatalf("Excluded = %v, want [.env]", manifest.Excluded)
	}
}

func TestFilesystemCaptureDigestChangesWithExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	root := t.TempDir()
	src := filepath.Join(root, "source")
	writeCaptureFile(t, src, "scripts/run.sh", "#!/bin/sh\n", 0o644)
	adapter := (FilesystemAdapter{Root: root, CacheRoot: filepath.Join(t.TempDir(), "cache")}).defaults()
	before, err := adapter.capture(context.Background(), "source")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(src, "scripts", "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	after, err := adapter.capture(context.Background(), "source")
	if err != nil {
		t.Fatal(err)
	}
	if before.Digest == after.Digest {
		t.Fatal("chmod +x on an unchanged file must change the folder digest, or a cached snapshot hides the mode")
	}
	if !after.Resources[0].Executable {
		t.Fatalf("resources = %#v", after.Resources)
	}
}
