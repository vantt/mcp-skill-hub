package source

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestGitRevisionAt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	r1Dir := t.TempDir()
	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	runGit(r1Dir, "init", "-b", "main")
	runGit(r1Dir, "config", "user.name", "Test")
	runGit(r1Dir, "config", "user.email", "test@example.com")
	runGit(r1Dir, "config", "uploadpack.allowReachableSHA1InWant", "true")

	// Commit A in R1
	if err := os.WriteFile(filepath.Join(r1Dir, "fileA.txt"), []byte("content A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(r1Dir, "add", "fileA.txt")
	runGit(r1Dir, "commit", "-m", "commit A")
	shaA := runGit(r1Dir, "rev-parse", "HEAD")

	// Tag v1.0 on commit A
	runGit(r1Dir, "tag", "-a", "v1.0", "-m", "tag v1.0", shaA)

	// Commit B in R1
	if err := os.WriteFile(filepath.Join(r1Dir, "fileB.txt"), []byte("content B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(r1Dir, "add", "fileB.txt")
	runGit(r1Dir, "commit", "-m", "commit B")
	shaB := runGit(r1Dir, "rev-parse", "HEAD")

	cacheDir := t.TempDir()
	adapter := GitRepositoryAdapter{
		CacheRoot:         cacheDir,
		AllowFileProtocol: true,
	}
	source := Source{
		Locator: Locator{
			Repository: fileURLFromPath(r1Dir),
			Ref:        "main",
		},
	}

	// Mirror synced at depth 1 (B only)
	curRev, err := adapter.CurrentRevision(context.Background(), source)
	if err != nil {
		t.Fatalf("CurrentRevision failed: %v", err)
	}
	if curRev.Value != shaB {
		t.Fatalf("expected current rev to be shaB %s, got %s", shaB, curRev.Value)
	}

	// 1. RevisionAt(A) succeeds and equals the digest of A's scoped tree
	revA, err := adapter.RevisionAt(context.Background(), source, shaA)
	if err != nil {
		t.Fatalf("RevisionAt(shaA) failed: %v", err)
	}
	if revA.Value != shaA {
		t.Fatalf("expected shaA %s, got %s", shaA, revA.Value)
	}
	treeHashA := runGit(r1Dir, "rev-parse", shaA+"^{tree}")
	expectedDigestA := Digest([]byte(treeHashA))
	if revA.ContentDigest != expectedDigestA {
		t.Fatalf("expected content digest %s, got %s", expectedDigestA, revA.ContentDigest)
	}

	// 2. RevisionAt(B, Path: "missing") returns ErrPathNotFound
	sourceMissing := source
	sourceMissing.Locator.Path = "missing"
	_, err = adapter.RevisionAt(context.Background(), sourceMissing, shaB)
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("expected ErrPathNotFound, got %v", err)
	}

	// 3. Unknown 40-hex commit returns ErrHistoryUnavailable
	unknownSHA := "0123456789012345678901234567890123456789"
	_, err = adapter.RevisionAt(context.Background(), source, unknownSHA)
	if !errors.Is(err, ErrHistoryUnavailable) {
		t.Fatalf("expected ErrHistoryUnavailable for unknown commit, got %v", err)
	}

	// 4. After fetching A, resolveCommit(repo, "main") still returns B
	mirrorRepo, err := adapter.openMirror(source.Locator.Repository)
	if err != nil {
		t.Fatalf("openMirror failed: %v", err)
	}
	resolvedHash, err := resolveCommit(mirrorRepo, "main")
	if err != nil {
		t.Fatalf("resolveCommit(main) failed: %v", err)
	}
	if resolvedHash.String() != shaB {
		t.Fatalf("expected main to still resolve to shaB %s, got %s", shaB, resolvedHash.String())
	}

	// Repository R2 without the config: RevisionAt(A) returns ErrHistoryUnavailable
	r2Dir := t.TempDir()
	runGit(r2Dir, "init", "-b", "main")
	runGit(r2Dir, "config", "user.name", "Test")
	runGit(r2Dir, "config", "user.email", "test@example.com")
	// Notice: DO NOT set uploadpack.allowReachableSHA1InWant

	if err := os.WriteFile(filepath.Join(r2Dir, "fileA.txt"), []byte("content A in R2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(r2Dir, "add", "fileA.txt")
	runGit(r2Dir, "commit", "-m", "commit A")
	shaA2 := runGit(r2Dir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(r2Dir, "fileB.txt"), []byte("content B in R2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(r2Dir, "add", "fileB.txt")
	runGit(r2Dir, "commit", "-m", "commit B")

	sourceR2 := Source{
		Locator: Locator{
			Repository: fileURLFromPath(r2Dir),
			Ref:        "main",
		},
	}
	adapterR2 := GitRepositoryAdapter{
		CacheRoot:         t.TempDir(),
		AllowFileProtocol: true,
	}
	if _, err := adapterR2.CurrentRevision(context.Background(), sourceR2); err != nil {
		t.Fatalf("CurrentRevision for R2 failed: %v", err)
	}
	_, err = adapterR2.RevisionAt(context.Background(), sourceR2, shaA2)
	if !errors.Is(err, ErrHistoryUnavailable) {
		t.Fatalf("expected ErrHistoryUnavailable for R2 without config, got %v", err)
	}

	// RemoteRefCommit tests
	freshCacheDir := t.TempDir()
	adapterRemote := GitRepositoryAdapter{
		CacheRoot:         freshCacheDir,
		AllowFileProtocol: true,
	}
	r1URL := fileURLFromPath(r1Dir)

	// Branch name
	branchCommit, err := adapterRemote.RemoteRefCommit(context.Background(), r1URL, "main")
	if err != nil {
		t.Fatalf("RemoteRefCommit(main) failed: %v", err)
	}
	if branchCommit != shaB {
		t.Fatalf("expected branch commit %s, got %s", shaB, branchCommit)
	}

	// Tag name (preferring peeled tag commits)
	tagCommit, err := adapterRemote.RemoteRefCommit(context.Background(), r1URL, "v1.0")
	if err != nil {
		t.Fatalf("RemoteRefCommit(v1.0) failed: %v", err)
	}
	if tagCommit != shaA {
		t.Fatalf("expected peeled tag commit %s, got %s", shaA, tagCommit)
	}

	// Nonexistent ref returns error
	_, err = adapterRemote.RemoteRefCommit(context.Background(), r1URL, "nonexistent")
	if err == nil {
		t.Fatalf("expected error for nonexistent ref, got nil")
	}

	// Does not write to mirror
	entries, err := os.ReadDir(freshCacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no mirror files in freshCacheDir, found %d entries", len(entries))
	}

	// Mirror lock contention returns ErrMirrorBusy on timeout
	mirrorPath, err := adapter.mirrorPath(source.Locator.Repository)
	if err != nil {
		t.Fatal(err)
	}
	flockFile := flock.New(mirrorPath + ".lock")
	locked, err := flockFile.TryLock()
	if err != nil || !locked {
		t.Fatalf("failed to hold test lock: %v", err)
	}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shortCancel()
	_, err = adapter.RevisionAt(shortCtx, source, shaB)
	_ = flockFile.Unlock()
	if !errors.Is(err, ErrMirrorBusy) {
		t.Fatalf("expected ErrMirrorBusy, got %v", err)
	}
}
