package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	transportclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// go-git's transport registry is process-global. Serialize temporary transport
// installation so unrelated Git operations can never inherit this source's
// resolver or transfer budget.
var gitTransportMu sync.Mutex

// GitRepositoryAdapter stores untrusted repositories in a disposable bare
// cache. Network operations use a credential-free pure-Go HTTPS transport that
// validates DNS, pins a validated address for each connection, preserves the
// original TLS server name, and refuses redirects.
type GitRepositoryAdapter struct {
	CacheRoot   string
	Timeout     time.Duration
	MaxBytes    int64
	MaxFiles    int
	MaxFileSize int64
	Now         func() time.Time
	Resolver    IPResolver
}

func (adapter GitRepositoryAdapter) defaults() GitRepositoryAdapter {
	if adapter.Timeout <= 0 {
		adapter.Timeout = DefaultTimeout
	}
	if adapter.MaxBytes <= 0 {
		adapter.MaxBytes = DefaultMaxBytes
	}
	if adapter.MaxFiles <= 0 {
		adapter.MaxFiles = DefaultMaxFiles
	}
	if adapter.MaxFileSize <= 0 {
		adapter.MaxFileSize = DefaultMaxFileSize
	}
	if adapter.Now == nil {
		adapter.Now = time.Now
	}
	return adapter
}

func (adapter GitRepositoryAdapter) Identify(ctx context.Context, locator Locator) (Identity, error) {
	adapter = adapter.defaults()
	if _, err := ValidateRemoteURL(locator.Repository, false); err != nil {
		return Identity{}, err
	}
	repository, _, err := adapter.syncMirror(ctx, locator.Repository)
	if err != nil {
		return Identity{}, err
	}
	head, err := repository.Head()
	if err != nil {
		return Identity{}, err
	}
	branch := head.Name().Short()
	commit, err := repository.CommitObject(head.Hash())
	if err != nil {
		return Identity{}, err
	}
	root, err := commit.Tree()
	if err != nil {
		return Identity{}, err
	}
	name := strings.TrimSuffix(filepath.Base(strings.TrimSuffix(locator.Repository, "/")), ".git")
	license, detectedPath := "", locator.Path
	var skillPaths []string
	count := 0
	err = root.Files().ForEach(func(file *object.File) error {
		if locator.Path != "" {
			prefix := strings.TrimSuffix(locator.Path, "/") + "/"
			if !strings.HasPrefix(file.Name, prefix) && file.Name != locator.Path {
				return nil
			}
		}
		count++
		if count > adapter.MaxFiles {
			return &LimitExceededError{Limit: "files", Actual: int64(count), Max: int64(adapter.MaxFiles)}
		}
		upper := strings.ToUpper(file.Name)
		if license == "" && (upper == "LICENSE" || strings.HasPrefix(upper, "LICENSE.") || upper == "COPYING") {
			license = file.Name
		}
		if filepath.Base(file.Name) == "SKILL.md" {
			skillPaths = append(skillPaths, filepath.ToSlash(filepath.Dir(file.Name)))
		}
		return nil
	})
	if err != nil {
		return Identity{}, err
	}
	if detectedPath == "" && len(skillPaths) == 1 {
		detectedPath = skillPaths[0]
	}
	if license != "" {
		file, fileErr := root.File(license)
		if fileErr == nil {
			reader, readerErr := file.Reader()
			if readerErr == nil {
				text, readErr := boundedRead(reader, min64(adapter.MaxFileSize, 128<<10))
				_ = reader.Close()
				if readErr == nil {
					license = detectLicense(text, license)
				}
			}
		}
	}
	return Identity{Name: name, Canonical: locator.Repository, DefaultBranch: branch, Path: detectedPath, License: license}, nil
}

func (adapter GitRepositoryAdapter) CurrentRevision(ctx context.Context, source Source) (Revision, error) {
	adapter = adapter.withLimits(source.Limits)
	repository, _, err := adapter.syncMirror(ctx, source.Locator.Repository)
	if err != nil {
		return Revision{}, err
	}
	commitHash, err := resolveCommit(repository, source.Locator.Ref)
	if err != nil {
		return Revision{}, err
	}
	commit, err := repository.CommitObject(commitHash)
	if err != nil {
		return Revision{}, err
	}
	objectHash, err := scopedObjectHash(commit, source.Locator.Path)
	if err != nil {
		return Revision{}, err
	}
	return Revision{Kind: "git-commit", Value: commitHash.String(), ContentDigest: Digest([]byte(objectHash.String())), ObservedAt: adapter.Now().UTC()}, nil
}

func (adapter GitRepositoryAdapter) Diff(ctx context.Context, source Source, from, to Revision) (ChangeSet, error) {
	adapter = adapter.withLimits(source.Limits)
	if !validGitObject(from.Value) || !validGitObject(to.Value) {
		return ChangeSet{}, ErrInvalidLocator
	}
	repository, err := adapter.openMirror(source.Locator.Repository)
	if err != nil {
		return ChangeSet{}, err
	}
	fromFiles, err := adapter.revisionFiles(ctx, repository, source, from)
	if err != nil {
		return ChangeSet{}, err
	}
	toFiles, err := adapter.revisionFiles(ctx, repository, source, to)
	if err != nil {
		return ChangeSet{}, err
	}
	paths := make(map[string]struct{}, len(fromFiles)+len(toFiles))
	for path := range fromFiles {
		paths[path] = struct{}{}
	}
	for path := range toFiles {
		paths[path] = struct{}{}
	}
	names := make([]string, 0, len(paths))
	for path := range paths {
		names = append(names, path)
	}
	sort.Strings(names)
	changes := make([]Change, 0)
	for _, name := range names {
		before, beforeOK := fromFiles[name]
		after, afterOK := toFiles[name]
		status := ""
		switch {
		case !beforeOK:
			status = "added"
		case !afterOK:
			status = "deleted"
		case before.hash != after.hash || before.size != after.size:
			status = "modified"
		}
		if status != "" {
			changes = append(changes, Change{Path: name, Status: status})
		}
	}
	return ChangeSet{From: from, To: to, Changes: changes}, nil
}

func (adapter GitRepositoryAdapter) Read(ctx context.Context, source Source, revision Revision, resourcePath string) ([]byte, error) {
	adapter = adapter.withLimits(source.Limits)
	if !validGitObject(revision.Value) || !safeResourcePath(resourcePath) {
		return nil, ErrInvalidLocator
	}
	repository, err := adapter.openMirror(source.Locator.Repository)
	if err != nil {
		return nil, err
	}
	commit, err := adapter.verifiedCommit(repository, source, revision)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	joined := resourcePath
	if source.Locator.Path != "" {
		joined = strings.TrimSuffix(source.Locator.Path, "/") + "/" + resourcePath
	}
	file, err := root.File(joined)
	if err != nil {
		return nil, err
	}
	if file.Mode == filemode.Symlink || (file.Mode != filemode.Regular && file.Mode != filemode.Executable) {
		return nil, ErrInvalidLocator
	}
	if file.Size > adapter.MaxFileSize {
		return nil, &LimitExceededError{Limit: "file_size", Actual: file.Size, Max: adapter.MaxFileSize, Path: resourcePath}
	}
	reader, err := file.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return boundedRead(reader, min64(adapter.MaxBytes, adapter.MaxFileSize))
}

func (adapter GitRepositoryAdapter) List(ctx context.Context, source Source, revision Revision, scope Scope) ([]Resource, error) {
	adapter = adapter.withLimits(source.Limits)
	if scope.Prefix != "" && !safeResourcePath(scope.Prefix) {
		return nil, ErrInvalidLocator
	}
	repository, err := adapter.openMirror(source.Locator.Repository)
	if err != nil {
		return nil, err
	}
	files, err := adapter.revisionFiles(ctx, repository, source, revision)
	if err != nil {
		return nil, err
	}
	resources := make([]Resource, 0, len(files))
	var total int64
	for name, item := range files {
		if scope.Prefix != "" && name != scope.Prefix && !strings.HasPrefix(name, strings.TrimSuffix(scope.Prefix, "/")+"/") {
			continue
		}
		total += item.size
		if total > adapter.MaxBytes {
			return nil, &LimitExceededError{Limit: "bytes", Actual: total, Max: adapter.MaxBytes}
		}
		resources = append(resources, Resource{Path: name, Size: item.size})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Path < resources[j].Path })
	return resources, nil
}

type gitFile struct {
	hash plumbing.Hash
	size int64
}

func (adapter GitRepositoryAdapter) revisionFiles(ctx context.Context, repository *git.Repository, source Source, revision Revision) (map[string]gitFile, error) {
	commit, err := adapter.verifiedCommit(repository, source, revision)
	if err != nil {
		return nil, err
	}
	root, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	files := make(map[string]gitFile)
	var total int64
	err = root.Files().ForEach(func(file *object.File) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := filepath.ToSlash(file.Name)
		if source.Locator.Path != "" {
			prefix := strings.TrimSuffix(source.Locator.Path, "/") + "/"
			if !strings.HasPrefix(name, prefix) {
				return nil
			}
			name = strings.TrimPrefix(name, prefix)
		}
		if !safeResourcePath(name) {
			return ErrInvalidLocator
		}
		if file.Mode == filemode.Symlink || (file.Mode != filemode.Regular && file.Mode != filemode.Executable) {
			return ErrInvalidLocator
		}
		if len(files) >= adapter.MaxFiles {
			return &LimitExceededError{Limit: "files", Actual: int64(len(files) + 1), Max: int64(adapter.MaxFiles)}
		}
		if file.Size > adapter.MaxFileSize {
			return &LimitExceededError{Limit: "file_size", Actual: file.Size, Max: adapter.MaxFileSize, Path: name}
		}
		total += file.Size
		if total > adapter.MaxBytes {
			return &LimitExceededError{Limit: "bytes", Actual: total, Max: adapter.MaxBytes}
		}
		files[name] = gitFile{hash: file.Hash, size: file.Size}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (adapter GitRepositoryAdapter) verifiedCommit(repository *git.Repository, source Source, revision Revision) (*object.Commit, error) {
	commit, err := repository.CommitObject(plumbing.NewHash(revision.Value))
	if err != nil {
		return nil, ErrHistoryUnavailable
	}
	hash, err := scopedObjectHash(commit, source.Locator.Path)
	if err != nil {
		return nil, ErrHistoryUnavailable
	}
	if Digest([]byte(hash.String())) != revision.ContentDigest {
		return nil, ErrRevisionMismatch
	}
	return commit, nil
}

func scopedObjectHash(commit *object.Commit, scopedPath string) (plumbing.Hash, error) {
	tree, err := commit.Tree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if scopedPath == "" {
		return tree.Hash, nil
	}
	if !safeResourcePath(scopedPath) {
		return plumbing.ZeroHash, ErrInvalidLocator
	}
	entry, err := tree.FindEntry(scopedPath)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return entry.Hash, nil
}

func resolveCommit(repository *git.Repository, ref string) (plumbing.Hash, error) {
	if ref == "" || ref == "HEAD" {
		head, err := repository.Head()
		if err != nil {
			return plumbing.ZeroHash, err
		}
		return head.Hash(), nil
	}
	clean := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/remotes/origin/")
	for _, name := range []plumbing.ReferenceName{plumbing.NewBranchReferenceName(clean), plumbing.NewRemoteReferenceName("origin", clean), plumbing.ReferenceName(ref)} {
		resolved, err := repository.Reference(name, true)
		if err == nil {
			return resolved.Hash(), nil
		}
	}
	if validGitObject(ref) {
		hash := plumbing.NewHash(ref)
		if _, err := repository.CommitObject(hash); err == nil {
			return hash, nil
		}
	}
	return plumbing.ZeroHash, fmt.Errorf("Git source ref %q was not found", ref)
}

func (adapter GitRepositoryAdapter) withLimits(limits Limits) GitRepositoryAdapter {
	adapter = adapter.defaults()
	if limits.TimeoutSeconds > 0 && time.Duration(limits.TimeoutSeconds)*time.Second < adapter.Timeout {
		adapter.Timeout = time.Duration(limits.TimeoutSeconds) * time.Second
	}
	if limits.MaxBytes > 0 && limits.MaxBytes < adapter.MaxBytes {
		adapter.MaxBytes = limits.MaxBytes
	}
	if limits.MaxFiles > 0 && limits.MaxFiles < adapter.MaxFiles {
		adapter.MaxFiles = limits.MaxFiles
	}
	if limits.MaxFileBytes > 0 && limits.MaxFileBytes < adapter.MaxFileSize {
		adapter.MaxFileSize = limits.MaxFileBytes
	}
	return adapter
}

func (adapter GitRepositoryAdapter) mirrorPath(repository string) (string, error) {
	if adapter.CacheRoot == "" {
		return "", errors.New("Git source cache root is required")
	}
	if _, err := ValidateRemoteURL(repository, false); err != nil {
		return "", err
	}
	if err := os.MkdirAll(adapter.CacheRoot, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(adapter.CacheRoot)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if err != nil {
			return "", err
		}
		return "", errors.New("unsafe Git source cache")
	}
	sum := sha256.Sum256([]byte(repository))
	return filepath.Join(adapter.CacheRoot, hex.EncodeToString(sum[:16])+".git"), nil
}

func (adapter GitRepositoryAdapter) openMirror(repository string) (*git.Repository, error) {
	mirror, err := adapter.mirrorPath(repository)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(mirror)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrHistoryUnavailable
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("unsafe Git mirror")
	}
	if err := adapter.checkMirrorSize(mirror); err != nil {
		return nil, err
	}
	return git.PlainOpen(mirror)
}

func (adapter GitRepositoryAdapter) syncMirror(ctx context.Context, remoteURL string) (*git.Repository, string, error) {
	adapter = adapter.defaults()
	mirror, err := adapter.mirrorPath(remoteURL)
	if err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, adapter.Timeout)
	defer cancel()
	var repository *git.Repository
	created := false
	err = adapter.withSafeTransport(func() error {
		info, statErr := os.Lstat(mirror)
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			created = true
			repository, statErr = git.PlainCloneContext(ctx, mirror, true, &git.CloneOptions{URL: remoteURL, NoCheckout: true, Tags: git.NoTags, SingleBranch: false})
		case statErr != nil:
			return statErr
		case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
			return errors.New("unsafe Git mirror")
		default:
			repository, statErr = git.PlainOpen(mirror)
			if statErr == nil {
				remoteConfig := &config.RemoteConfig{Name: "origin", URLs: []string{remoteURL}, Fetch: []config.RefSpec{"+refs/heads/*:refs/heads/*"}}
				remote := git.NewRemote(repository.Storer, remoteConfig)
				statErr = remote.FetchContext(ctx, &git.FetchOptions{Tags: git.NoTags, Force: true, RefSpecs: remoteConfig.Fetch})
				if errors.Is(statErr, git.NoErrAlreadyUpToDate) {
					statErr = nil
				}
			}
		}
		return statErr
	})
	if err != nil {
		if created || errors.Is(err, ErrLimitExceeded) {
			_ = os.RemoveAll(mirror)
		}
		return nil, "", fmt.Errorf("Git HTTPS source operation failed: %w", err)
	}
	if err := adapter.checkMirrorSize(mirror); err != nil {
		_ = os.RemoveAll(mirror)
		return nil, "", err
	}
	return repository, mirror, nil
}

func (adapter GitRepositoryAdapter) withSafeTransport(operation func() error) error {
	gitTransportMu.Lock()
	defer gitTransportMu.Unlock()
	original := transportclient.Protocols["https"]
	client := newPinnedHTTPClient(adapter.Timeout, adapter.Resolver, adapter.MaxBytes, true)
	transportclient.InstallProtocol("https", githttp.NewClient(client))
	defer transportclient.InstallProtocol("https", original)
	return operation()
}

func (adapter GitRepositoryAdapter) checkMirrorSize(mirror string) error {
	var total int64
	return filepath.WalkDir(mirror, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("unsafe symlink in Git mirror")
		}
		relative, err := filepath.Rel(mirror, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(relative) == "objects/info/alternates" {
			return errors.New("Git object alternates are not allowed")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("unsafe object in Git mirror")
		}
		total += info.Size()
		if total > adapter.MaxBytes {
			return ErrLimitExceeded
		}
		return nil
	})
}

type budgetRoundTripper struct {
	base      http.RoundTripper
	mu        sync.Mutex
	remaining int64
}

func (transport *budgetRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	transport.mu.Lock()
	remaining := transport.remaining
	transport.mu.Unlock()
	if response.ContentLength > remaining && response.ContentLength >= 0 {
		_ = response.Body.Close()
		return nil, ErrLimitExceeded
	}
	response.Body = &budgetReadCloser{reader: response.Body, closer: response.Body, budget: transport}
	return response, nil
}

type budgetReadCloser struct {
	reader io.Reader
	closer io.Closer
	budget *budgetRoundTripper
}

func (reader *budgetReadCloser) Read(target []byte) (int, error) {
	reader.budget.mu.Lock()
	defer reader.budget.mu.Unlock()
	remaining := reader.budget.remaining
	if remaining <= 0 {
		return 0, ErrLimitExceeded
	}
	if int64(len(target)) > remaining+1 {
		target = target[:remaining+1]
	}
	n, err := reader.reader.Read(target)
	reader.budget.remaining -= int64(n)
	if reader.budget.remaining < 0 {
		return n, ErrLimitExceeded
	}
	return n, err
}
func (reader *budgetReadCloser) Close() error { return reader.closer.Close() }

func validGitObject(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
func detectLicense(contents []byte, fallback string) string {
	upper := strings.ToUpper(string(contents))
	switch {
	case strings.Contains(upper, "APACHE LICENSE") && strings.Contains(upper, "VERSION 2.0"):
		return "Apache-2.0"
	case strings.Contains(upper, "MIT LICENSE") || (strings.Contains(upper, "PERMISSION IS HEREBY GRANTED") && strings.Contains(upper, "WITHOUT WARRANTY")):
		return "MIT"
	default:
		return fallback
	}
}
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
