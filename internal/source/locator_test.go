package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"gopkg.in/yaml.v3"
)

func TestAuthorizedLocalRootLifecycleAndSerializationProtection(t *testing.T) {
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "my-skill")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(sourceDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# My Skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	realSourceDir, err := filepath.EvalSymlinks(sourceDir)
	if err != nil {
		t.Fatal(err)
	}

	// Direct directory
	authRoot, err := NewAuthorizedLocalRoot(sourceDir)
	if err != nil {
		t.Fatalf("NewAuthorizedLocalRoot failed: %v", err)
	}
	if authRoot.Path() != realSourceDir {
		t.Fatalf("expected path %q, got %q", realSourceDir, authRoot.Path())
	}

	// File path points to parent folder
	authFromFile, err := NewAuthorizedLocalRoot(skillFile)
	if err != nil {
		t.Fatalf("NewAuthorizedLocalRoot from file failed: %v", err)
	}
	if authFromFile.Path() != realSourceDir {
		t.Fatalf("expected parent dir %q, got %q", realSourceDir, authFromFile.Path())
	}

	// Symlinked root is resolved once
	symlinkPath := filepath.Join(temp, "symlink-root")
	if err := os.Symlink(sourceDir, symlinkPath); err == nil {
		authSymlink, err := NewAuthorizedLocalRoot(symlinkPath)
		if err != nil {
			t.Fatalf("NewAuthorizedLocalRoot symlink failed: %v", err)
		}
		if authSymlink.Path() != realSourceDir {
			t.Fatalf("expected resolved target %q, got %q", realSourceDir, authSymlink.Path())
		}
	}

	// Serialization prevention: JSON
	if _, err := json.Marshal(authRoot); err == nil {
		t.Fatal("expected JSON serialization of AuthorizedLocalRoot to fail")
	}

	// Serialization prevention: YAML
	if _, err := yaml.Marshal(authRoot); err == nil {
		t.Fatal("expected YAML serialization of AuthorizedLocalRoot to fail")
	}

	// String() formatting masks the path
	str := authRoot.String()
	if strings.Contains(str, sourceDir) || str != "<authorized-local-root>" {
		t.Fatalf("String() leaked root path: %s", str)
	}

	// Non-existent path fails
	if _, err := NewAuthorizedLocalRoot(filepath.Join(temp, "non-existent")); err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestSnapshotReferenceAndNormalizedLocator(t *testing.T) {
	ref := SnapshotReference{
		SnapshotID:    "abc12345",
		ContentDigest: "sha256:0123456789abcdef",
		SelectedPath:  "sub/skill",
	}

	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "/") && !strings.Contains(string(data), "sub/skill") {
		t.Fatalf("unexpected content in snapshot reference json: %s", string(data))
	}

	norm := NormalizedLocator{
		Kind:       "github",
		Repository: "https://github.com/example/repo.git",
		Ref:        "main",
		Path:       "skills/pdf",
	}
	loc := norm.ToLocator()
	if loc.Repository != norm.Repository || loc.Ref != norm.Ref || loc.Path != norm.Path {
		t.Fatalf("ToLocator mismatch: %#v", loc)
	}
}

func TestParseGitHubLocatorStructuralRoutes(t *testing.T) {
	cases := []struct {
		name        string
		url         string
		ref         string
		path        string
		wantRepo    string
		wantKind    string
		wantRest    string
		wantRef     string
		wantPath    string
		wantErrPart string
	}{
		{
			name:     "repo root",
			url:      "https://github.com/anthropics/skills",
			wantRepo: "https://github.com/anthropics/skills.git",
			wantKind: "root",
		},
		{
			name:     "repo root with git suffix",
			url:      "https://github.com/anthropics/skills.git",
			wantRepo: "https://github.com/anthropics/skills.git",
			wantKind: "root",
		},
		{
			name:     "tree route with branch and subpath",
			url:      "https://github.com/anthropics/skills/tree/main/skills/pdf",
			wantRepo: "https://github.com/anthropics/skills.git",
			wantKind: "tree",
			wantRest: "main/skills/pdf",
		},
		{
			name:     "blob route with SKILL.md",
			url:      "https://github.com/anthropics/skills/blob/main/skills/pdf/SKILL.md",
			wantRepo: "https://github.com/anthropics/skills.git",
			wantKind: "blob",
			wantRest: "main/skills/pdf",
		},
		{
			name:        "blob route with other file rejected",
			url:         "https://github.com/anthropics/skills/blob/main/skills/pdf/other.md",
			wantErrPart: "link the skill folder or its SKILL.md",
		},
		{
			name:        "unsupported route kind pull",
			url:         "https://github.com/anthropics/skills/pull/123",
			wantErrPart: "unsupported GitHub route",
		},
		{
			name:     "fragment specifies ref",
			url:      "https://github.com/anthropics/skills#v1.0.0",
			wantRepo: "https://github.com/anthropics/skills.git",
			wantKind: "root",
			wantRef:  "v1.0.0",
		},
		{
			name:        "credentials in url rejected",
			url:         "https://user:token@github.com/anthropics/skills.git",
			wantErrPart: "invalid source locator",
		},
		{
			name:        "query parameters in url rejected",
			url:         "https://github.com/anthropics/skills.git?foo=bar",
			wantErrPart: "invalid source locator",
		},
		{
			name:        "unsupported host rejected",
			url:         "https://gitlab.com/anthropics/skills.git",
			wantErrPart: "only public github.com is supported",
		},
		{
			name:        "non-https scheme rejected",
			url:         "http://github.com/anthropics/skills.git",
			wantErrPart: "allowed protocol is https",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route, err := ParseGitHubLocator(tc.url, tc.ref, tc.path)
			if tc.wantErrPart != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrPart) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErrPart, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if route.Repository != tc.wantRepo {
				t.Errorf("repo = %q, want %q", route.Repository, tc.wantRepo)
			}
			if route.RouteKind != tc.wantKind {
				t.Errorf("kind = %q, want %q", route.RouteKind, tc.wantKind)
			}
			if route.Rest != tc.wantRest {
				t.Errorf("rest = %q, want %q", route.Rest, tc.wantRest)
			}
			if tc.wantRef != "" && route.Ref != tc.wantRef {
				t.Errorf("ref = %q, want %q", route.Ref, tc.wantRef)
			}
		})
	}
}

// helper to create a local git repo fixture with branches and tags
func createTestGitRepo(t *testing.T) (string, *git.Repository) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	// Initial commit on main
	fileA := filepath.Join(dir, "README.md")
	if err := os.WriteFile(fileA, []byte("# Test Repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillA := filepath.Join(dir, "skills", "pdf", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillA), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillA, []byte("---\nname: pdf\n---\nBody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatal(err)
	}
	c1, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create tag feature pointing to c1
	refFeature := plumbing.NewReferenceFromStrings("refs/tags/feature", c1.String())
	if err := repo.Storer.SetReference(refFeature); err != nil {
		t.Fatal(err)
	}
	// Create feature/x branch with a skill
	if err := wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature/x"),
		Create: true,
	}); err != nil {
		t.Fatal(err)
	}
	skillX := filepath.Join(dir, "skills", "doc", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillX), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillX, []byte("---\nname: doc\n---\nBody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("feature/x commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}

	// Create annotated tag v1.0 pointing to c1
	if _, err := repo.CreateTag("v1.0", c1, &git.CreateTagOptions{
		Tagger:  &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()},
		Message: "Release v1.0",
	}); err != nil {
		t.Fatal(err)
	}

	// Switch back to master/main
	_ = wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("master")})

	return dir, repo
}

func TestResolveGitHubRouteWithLocalRepo(t *testing.T) {
	repoDir, _ := createTestGitRepo(t)
	adapter := GitRepositoryAdapter{
		CacheRoot:         t.TempDir(),
		AllowFileProtocol: true,
	}
	fileURL := "file://" + filepath.ToSlash(repoDir)

	ctx := context.Background()

	// 1. Root route resolves default branch
	rootRoute := &ParsedGitHubRoute{
		Repository: fileURL,
		RouteKind:  "root",
	}
	resolved, err := ResolveGitHubRoute(ctx, adapter, rootRoute)
	if err != nil {
		t.Fatalf("resolve root route failed: %v", err)
	}
	if resolved.Commit == "" || resolved.Ref == "" {
		t.Fatalf("expected resolved commit and ref, got: %#v", resolved)
	}

	// 2. Explicit ref resolves directly
	explicitRoute := &ParsedGitHubRoute{
		Repository: fileURL,
		RouteKind:  "tree",
		Rest:       "feature/skills/pdf",
		Ref:        "feature",
	}
	resolvedExp, err := ResolveGitHubRoute(ctx, adapter, explicitRoute)
	if err != nil {
		t.Fatalf("resolve explicit ref route failed: %v", err)
	}
	if resolvedExp.Ref != "feature" || resolvedExp.Path != "skills/pdf" {
		t.Fatalf("unexpected resolved: %#v", resolvedExp)
	}

	// 3. Slash ref resolution: feature/x should resolve as ref "feature/x" and path "skills/doc"
	slashRoute := &ParsedGitHubRoute{
		Repository: fileURL,
		RouteKind:  "tree",
		Rest:       "feature/x/skills/doc",
	}
	resolvedSlash, err := ResolveGitHubRoute(ctx, adapter, slashRoute)
	if err != nil {
		t.Fatalf("resolve slash route failed: %v", err)
	}
	if resolvedSlash.Ref != "feature/x" || resolvedSlash.Path != "skills/doc" {
		t.Fatalf("unexpected slash resolved: %#v", resolvedSlash)
	}

	// 4. Annotated tag resolution: peels to commit hash
	tagRoute := &ParsedGitHubRoute{
		Repository: fileURL,
		RouteKind:  "tree",
		Rest:       "v1.0/skills/pdf",
	}
	resolvedTag, err := ResolveGitHubRoute(ctx, adapter, tagRoute)
	if err != nil {
		t.Fatalf("resolve tag route failed: %v", err)
	}
	if resolvedTag.Ref != "v1.0" || resolvedTag.Path != "skills/pdf" {
		t.Fatalf("unexpected tag resolved: %#v", resolvedTag)
	}

	// 5. Commit SHA directly in path
	shaRoute := &ParsedGitHubRoute{
		Repository: fileURL,
		RouteKind:  "tree",
		Rest:       resolvedTag.Commit + "/skills/pdf",
	}
	resolvedSHA, err := ResolveGitHubRoute(ctx, adapter, shaRoute)
	if err != nil {
		t.Fatalf("resolve SHA route failed: %v", err)
	}
	if resolvedSHA.Commit != resolvedTag.Commit || resolvedSHA.Path != "skills/pdf" {
		t.Fatalf("unexpected sha resolved: %#v", resolvedSHA)
	}

	// 6. Non-existent ref returns error
	badRoute := &ParsedGitHubRoute{
		Repository: fileURL,
		RouteKind:  "tree",
		Rest:       "nonexistent-branch/path",
	}
	if _, err := ResolveGitHubRoute(ctx, adapter, badRoute); err == nil {
		t.Fatal("expected error for non-existent ref")
	}
}

func TestResolveGitHubRouteBranchTagCollision(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatal(err)
	}
	c1, err := wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "Tester", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create branch named 'release'
	if err := wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("release"),
		Create: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Create lightweight tag also named 'release'
	ref := plumbing.NewReferenceFromStrings("refs/tags/release", c1.String())
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatal(err)
	}

	adapter := GitRepositoryAdapter{
		CacheRoot:         t.TempDir(),
		AllowFileProtocol: true,
	}
	fileURL := "file://" + filepath.ToSlash(dir)

	route := &ParsedGitHubRoute{
		Repository: fileURL,
		RouteKind:  "tree",
		Rest:       "release/some/path",
	}

	_, err = ResolveGitHubRoute(context.Background(), adapter, route)
	if err == nil {
		t.Fatal("expected AmbiguousRefError on branch/tag collision")
	}
	var ambErr *AmbiguousRefError
	if !errors.As(err, &ambErr) {
		t.Fatalf("expected AmbiguousRefError, got %T: %v", err, err)
	}
	if len(ambErr.Candidates) != 2 {
		t.Fatalf("expected 2 candidates in AmbiguousRefError, got %v", ambErr.Candidates)
	}
}

func TestCaptureAuthorizedRootPrivacyAndCacheIndependence(t *testing.T) {
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "authorized-source")
	cacheDir := filepath.Join(temp, "cache")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Create sample skill files
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("# Test Skill Content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(sourceDir, "nested")
	if err := os.MkdirAll(subDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "helper.py"), []byte("print('hello')\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	authRoot, err := NewAuthorizedLocalRoot(sourceDir)
	if err != nil {
		t.Fatal(err)
	}

	adapter := FilesystemAdapter{
		CacheRoot: cacheDir,
	}

	ctx := context.Background()
	manifest, err := adapter.CaptureAuthorizedRoot(ctx, authRoot, "")
	if err != nil {
		t.Fatalf("CaptureAuthorizedRoot failed: %v", err)
	}

	if manifest.Digest == "" || len(manifest.Resources) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}

	// Byte-scan the entire cache directory to prove that the host absolute path
	// (sourceDir) is completely absent from all cached files and manifest.
	sourceDirBytes := []byte(sourceDir)
	err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(data, sourceDirBytes) {
			t.Errorf("cache file %s leaked host path %s", path, sourceDir)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify subsequent reads work from snapshot even if source directory is completely deleted!
	if err := os.RemoveAll(sourceDir); err != nil {
		t.Fatal(err)
	}

	sourceObj := Source{
		ID: "test-source",
		Locator: Locator{
			SnapshotDigest: manifest.Digest,
		},
	}
	rev := Revision{
		Kind:          "filesystem-snapshot",
		Value:         manifest.Digest,
		ContentDigest: manifest.Digest,
	}

	data, err := adapter.Read(ctx, sourceObj, rev, "SKILL.md")
	if err != nil {
		t.Fatalf("failed to read from snapshot after source deletion: %v", err)
	}
	if string(data) != "# Test Skill Content\n" {
		t.Fatalf("unexpected content read from snapshot: %s", string(data))
	}

	resources, err := adapter.List(ctx, sourceObj, rev, Scope{})
	if err != nil {
		t.Fatalf("failed to list snapshot resources: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
}

func TestCaptureAuthorizedRootRejectsSymlinksInside(t *testing.T) {
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "source-with-symlink")
	cacheDir := filepath.Join(temp, "cache")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}

	outsideFile := filepath.Join(temp, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(sourceDir, "escaped.txt")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	authRoot, err := NewAuthorizedLocalRoot(sourceDir)
	if err != nil {
		t.Fatal(err)
	}

	adapter := FilesystemAdapter{
		CacheRoot: cacheDir,
	}

	_, err = adapter.CaptureAuthorizedRoot(context.Background(), authRoot, "")
	if err == nil {
		t.Fatal("expected CaptureAuthorizedRoot to reject internal symlink")
	}
	if !errors.Is(err, ErrUnsafeFile) && !strings.Contains(err.Error(), "unsafe source symlink") {
		t.Fatalf("expected ErrUnsafeFile for internal symlink, got: %v", err)
	}
}
