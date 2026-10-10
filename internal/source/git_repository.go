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
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/gofrs/flock"
)

var (
	ErrMirrorBusy   = errors.New("another Skill Hub process is using the repository cache; retry shortly")
	ErrPathNotFound = errors.New("scoped path not found in commit tree")
)

const (
	mirrorLockTimeout    = 30 * time.Second
	mirrorLockRetryDelay = 25 * time.Millisecond
)

func withMirrorLock(ctx context.Context, mirror string, shared bool, fn func() error) error {
	lockPath := mirror + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	lock := flock.New(lockPath)
	waitContext, cancel := context.WithTimeout(ctx, mirrorLockTimeout)
	defer cancel()

	var ok bool
	var err error
	if shared {
		ok, err = lock.TryRLockContext(waitContext, mirrorLockRetryDelay)
	} else {
		ok, err = lock.TryLockContext(waitContext, mirrorLockRetryDelay)
	}
	if err != nil {
		if errors.Is(waitContext.Err(), context.DeadlineExceeded) {
			return ErrMirrorBusy
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("acquire mirror lock: %w", err)
	}
	if !ok {
		return ErrMirrorBusy
	}
	defer func() {
		_ = lock.Unlock()
	}()
	return fn()
}

// go-git's transport registry is process-global. Serialize temporary transport
// installation so unrelated Git operations can never inherit this source's
// resolver or transfer budget.
var gitTransportMu sync.Mutex

// GitRepositoryAdapter stores untrusted repositories in a disposable bare
// cache. Network operations use a credential-free pure-Go HTTPS transport that
// validates DNS, pins a validated address for each connection, preserves the
// original TLS server name, and refuses redirects.
type GitRepositoryAdapter struct {
	CacheRoot         string
	Timeout           time.Duration
	MirrorTimeout     time.Duration
	MaxBytes          int64
	MaxMirrorBytes    int64
	MaxFiles          int
	MaxFileSize       int64
	Now               func() time.Time
	Resolver          IPResolver
	AllowFileProtocol bool
}

func (adapter GitRepositoryAdapter) defaults() GitRepositoryAdapter {
	if adapter.Timeout <= 0 {
		adapter.Timeout = DefaultTimeout
	}
	if adapter.MirrorTimeout <= 0 {
		adapter.MirrorTimeout = DefaultMirrorTimeout
	}
	if adapter.MaxBytes <= 0 {
		adapter.MaxBytes = DefaultMaxBytes
	}
	if adapter.MaxMirrorBytes <= 0 {
		adapter.MaxMirrorBytes = DefaultMaxMirrorBytes
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
	if _, err := ValidateRemoteURLWithOptions(locator.Repository, URLValidationOptions{AllowFile: adapter.AllowFileProtocol}); err != nil {
		return Identity{}, err
	}
	repository, _, err := adapter.syncMirror(ctx, locator.Repository, locator.Ref)
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
	repository, _, err := adapter.syncMirror(ctx, source.Locator.Repository, source.Locator.Ref)
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
	mirror, err := adapter.mirrorPath(source.Locator.Repository)
	if err != nil {
		return ChangeSet{}, err
	}
	var fromFiles, toFiles map[string]gitFile
	err = withMirrorLock(ctx, mirror, true, func() error {
		repository, err := adapter.openMirrorLocked(mirror)
		if err != nil {
			return err
		}
		var fromErr, toErr error
		fromFiles, fromErr = adapter.revisionFiles(ctx, repository, source, from)
		if fromErr != nil {
			return fromErr
		}
		toFiles, toErr = adapter.revisionFiles(ctx, repository, source, to)
		return toErr
	})
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
	mirror, err := adapter.mirrorPath(source.Locator.Repository)
	if err != nil {
		return nil, err
	}
	var data []byte
	err = withMirrorLock(ctx, mirror, true, func() error {
		repository, err := adapter.openMirrorLocked(mirror)
		if err != nil {
			return err
		}
		commit, err := adapter.verifiedCommit(repository, source, revision)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		root, err := commit.Tree()
		if err != nil {
			return err
		}
		joined := resourcePath
		if source.Locator.Path != "" {
			joined = strings.TrimSuffix(source.Locator.Path, "/") + "/" + resourcePath
		}
		file, err := root.File(joined)
		if err != nil {
			return err
		}
		if file.Mode == filemode.Symlink || (file.Mode != filemode.Regular && file.Mode != filemode.Executable) {
			return ErrInvalidLocator
		}
		if file.Size > adapter.MaxFileSize {
			return &LimitExceededError{Limit: "file_size", Actual: file.Size, Max: adapter.MaxFileSize, Path: resourcePath}
		}
		reader, err := file.Reader()
		if err != nil {
			return err
		}
		defer reader.Close()
		data, err = boundedRead(reader, min64(adapter.MaxBytes, adapter.MaxFileSize))
		return err
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (adapter GitRepositoryAdapter) List(ctx context.Context, source Source, revision Revision, scope Scope) ([]Resource, error) {
	adapter = adapter.withLimits(source.Limits)
	if scope.Prefix != "" && !safeResourcePath(scope.Prefix) {
		return nil, ErrInvalidLocator
	}
	mirror, err := adapter.mirrorPath(source.Locator.Repository)
	if err != nil {
		return nil, err
	}
	var resources []Resource
	err = withMirrorLock(ctx, mirror, true, func() error {
		repository, err := adapter.openMirrorLocked(mirror)
		if err != nil {
			return err
		}
		files, err := adapter.revisionFiles(ctx, repository, source, revision)
		if err != nil {
			return err
		}
		resources = make([]Resource, 0, len(files))
		var total int64
		for name, item := range files {
			if scope.Prefix != "" && name != scope.Prefix && !strings.HasPrefix(name, strings.TrimSuffix(scope.Prefix, "/")+"/") {
				continue
			}
			total += item.size
			if total > adapter.MaxBytes {
				return &LimitExceededError{Limit: "bytes", Actual: total, Max: adapter.MaxBytes}
			}
			resources = append(resources, Resource{Path: name, Size: item.size})
		}
		return nil
	})
	if err != nil {
		return nil, err
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
		if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, object.ErrFileNotFound) {
			return plumbing.ZeroHash, ErrPathNotFound
		}
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
	if strings.HasPrefix(ref, "refs/skillhub/commits/") {
		return plumbing.ZeroHash, fmt.Errorf("git source ref %q was not found", ref)
	}
	clean := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/remotes/origin/")
	clean = strings.TrimPrefix(clean, "refs/tags/")
	for _, name := range []plumbing.ReferenceName{
		plumbing.NewBranchReferenceName(clean),
		plumbing.NewRemoteReferenceName("origin", clean),
		plumbing.NewTagReferenceName(clean),
		plumbing.ReferenceName(ref),
	} {
		resolved, err := repository.Reference(name, true)
		if err == nil {
			tagObj, tagErr := repository.TagObject(resolved.Hash())
			if tagErr == nil {
				return tagObj.Target, nil
			}
			return resolved.Hash(), nil
		}
	}
	if validGitObject(ref) {
		hash := plumbing.NewHash(ref)
		if _, err := repository.CommitObject(hash); err == nil {
			return hash, nil
		}
	}
	return plumbing.ZeroHash, fmt.Errorf("git source ref %q was not found", ref)
}

func (adapter GitRepositoryAdapter) withLimits(limits Limits) GitRepositoryAdapter {
	adapter = adapter.defaults()
	if limits.TimeoutSeconds > 0 {
		t := time.Duration(limits.TimeoutSeconds) * time.Second
		if t < adapter.Timeout {
			adapter.Timeout = t
		}
		if t > adapter.MirrorTimeout {
			adapter.MirrorTimeout = t
		}
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
	if _, err := ValidateRemoteURLWithOptions(repository, URLValidationOptions{AllowFile: adapter.AllowFileProtocol}); err != nil {
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

func (adapter GitRepositoryAdapter) openMirrorLocked(mirror string) (*git.Repository, error) {
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

func (adapter GitRepositoryAdapter) openMirror(repository string) (*git.Repository, error) {
	mirror, err := adapter.mirrorPath(repository)
	if err != nil {
		return nil, err
	}
	var repo *git.Repository
	err = withMirrorLock(context.Background(), mirror, true, func() error {
		var openErr error
		repo, openErr = adapter.openMirrorLocked(mirror)
		return openErr
	})
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (adapter GitRepositoryAdapter) openOrCloneLocked(ctx context.Context, mirror string, remoteURL string, ref string) (*git.Repository, error) {
	repo, err := adapter.openMirrorLocked(mirror)
	if err == nil {
		return repo, nil
	}
	if !errors.Is(err, ErrHistoryUnavailable) {
		return nil, err
	}
	return adapter.syncMirrorLocked(ctx, mirror, remoteURL, ref)
}

func (adapter GitRepositoryAdapter) syncMirror(ctx context.Context, remoteURL string, ref string) (*git.Repository, string, error) {
	adapter = adapter.defaults()
	mirror, err := adapter.mirrorPath(remoteURL)
	if err != nil {
		return nil, "", err
	}
	var repository *git.Repository
	err = withMirrorLock(ctx, mirror, false, func() error {
		var syncErr error
		repository, syncErr = adapter.syncMirrorLocked(ctx, mirror, remoteURL, ref)
		return syncErr
	})
	if err != nil {
		return nil, "", err
	}
	return repository, mirror, nil
}

func (adapter GitRepositoryAdapter) syncMirrorLocked(ctx context.Context, mirror string, remoteURL string, ref string) (*git.Repository, error) {
	timeout := adapter.MirrorTimeout
	if timeout <= 0 {
		timeout = DefaultMirrorTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var repository *git.Repository
	created := false
	err := adapter.withSafeTransport(func() error {
		info, statErr := os.Lstat(mirror)
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			created = true
			var refName plumbing.ReferenceName
			if ref != "" && ref != "HEAD" {
				clean := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")
				refName = plumbing.NewBranchReferenceName(clean)
			}
			cloneOpts := &git.CloneOptions{
				URL:          remoteURL,
				NoCheckout:   true,
				SingleBranch: true,
				Depth:        1,
				Tags:         git.NoTags,
			}
			if refName != "" {
				cloneOpts.ReferenceName = refName
			}
			repository, statErr = git.PlainCloneContext(ctx, mirror, true, cloneOpts)
			if statErr != nil && refName != "" {
				clean := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")
				cloneOpts.ReferenceName = plumbing.NewTagReferenceName(clean)
				repository, statErr = git.PlainCloneContext(ctx, mirror, true, cloneOpts)
				if statErr != nil {
					cloneOpts.ReferenceName = ""
					repository, statErr = git.PlainCloneContext(ctx, mirror, true, cloneOpts)
				}
			}
			return statErr
		case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
			return errors.New("unsafe Git mirror")
		default:
			repository, statErr = git.PlainOpen(mirror)
			if statErr == nil {
				if ref != "" && ref != "HEAD" {
					clean := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")
					refSpec := config.RefSpec("+refs/heads/" + clean + ":refs/heads/" + clean)
					remoteConfig := &config.RemoteConfig{Name: "origin", URLs: []string{remoteURL}, Fetch: []config.RefSpec{refSpec}}
					remote := git.NewRemote(repository.Storer, remoteConfig)
					fetchErr := remote.FetchContext(ctx, &git.FetchOptions{
						Depth:    1,
						Tags:     git.NoTags,
						Force:    true,
						RefSpecs: remoteConfig.Fetch,
					})
					if fetchErr != nil && !errors.Is(fetchErr, git.NoErrAlreadyUpToDate) {
						tagSpec := config.RefSpec("+refs/tags/" + clean + ":refs/tags/" + clean)
						remoteConfig = &config.RemoteConfig{Name: "origin", URLs: []string{remoteURL}, Fetch: []config.RefSpec{tagSpec}}
						remote = git.NewRemote(repository.Storer, remoteConfig)
						_ = remote.FetchContext(ctx, &git.FetchOptions{
							Depth:    1,
							Tags:     git.NoTags,
							Force:    true,
							RefSpecs: remoteConfig.Fetch,
						})
					}
				} else {
					remoteConfig := &config.RemoteConfig{Name: "origin", URLs: []string{remoteURL}, Fetch: []config.RefSpec{"+refs/heads/*:refs/heads/*"}}
					remote := git.NewRemote(repository.Storer, remoteConfig)
					_ = remote.FetchContext(ctx, &git.FetchOptions{
						Depth:    1,
						Tags:     git.NoTags,
						Force:    true,
						RefSpecs: remoteConfig.Fetch,
					})
				}
			}
		}
		return statErr
	})
	if err != nil {
		if created || errors.Is(err, ErrLimitExceeded) {
			_ = os.RemoveAll(mirror)
		}
		return nil, fmt.Errorf("git HTTPS source operation failed: %w", err)
	}
	if err := adapter.checkMirrorSize(mirror); err != nil {
		_ = os.RemoveAll(mirror)
		return nil, err
	}
	return repository, nil
}

func (adapter GitRepositoryAdapter) withSafeTransport(operation func() error) error {
	gitTransportMu.Lock()
	defer gitTransportMu.Unlock()
	original := transportclient.Protocols["https"]
	timeout := adapter.MirrorTimeout
	if timeout <= 0 {
		timeout = DefaultMirrorTimeout
	}
	maxBytes := adapter.MaxMirrorBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxMirrorBytes
	}
	client := newPinnedHTTPClient(timeout, adapter.Resolver, maxBytes, true)
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
		maxBytes := adapter.MaxMirrorBytes
		if maxBytes <= 0 {
			maxBytes = DefaultMaxMirrorBytes
		}
		if total > maxBytes {
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

// AdvertisedRef represents one ref advertised by a remote Git repository.
type AdvertisedRef struct {
	Name       string
	Hash       string
	PeeledHash string
}

// ListAdvertisedRefs queries the fresh advertised heads and tags from a remote repository.
func (adapter GitRepositoryAdapter) ListAdvertisedRefs(ctx context.Context, remoteURL string) ([]AdvertisedRef, error) {
	adapter = adapter.defaults()
	if _, err := ValidateRemoteURLWithOptions(remoteURL, URLValidationOptions{AllowFile: adapter.AllowFileProtocol}); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, adapter.Timeout)
	defer cancel()

	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	var refs []*plumbing.Reference
	err := adapter.withSafeTransport(func() error {
		var listErr error
		refs, listErr = remote.ListContext(ctx, &git.ListOptions{
			PeelingOption: git.AppendPeeled,
		})
		return listErr
	})
	if err != nil {
		return nil, fmt.Errorf("Git HTTPS source operation failed: %w", err)
	}

	peeled := make(map[string]string)
	for _, ref := range refs {
		name := string(ref.Name())
		if strings.HasSuffix(name, "^{}") {
			base := strings.TrimSuffix(name, "^{}")
			peeled[base] = ref.Hash().String()
		}
	}
	result := make([]AdvertisedRef, 0, len(refs))
	for _, ref := range refs {
		name := string(ref.Name())
		if strings.HasSuffix(name, "^{}") {
			continue
		}
		item := AdvertisedRef{
			Name:       name,
			Hash:       ref.Hash().String(),
			PeeledHash: peeled[name],
		}
		result = append(result, item)
	}
	return result, nil
}

// RemoteRefCommit resolves a ref name or branch/tag name to a commit hash from advertised refs without modifying the local mirror.
func (adapter GitRepositoryAdapter) RemoteRefCommit(ctx context.Context, repository string, ref string) (string, error) {
	adapter = adapter.defaults()
	refs, err := adapter.ListAdvertisedRefs(ctx, repository)
	if err != nil {
		return "", err
	}
	if ref == "" || ref == "HEAD" {
		for _, item := range refs {
			if item.Name == "HEAD" {
				return item.Hash, nil
			}
		}
		return "", fmt.Errorf("git source ref %q was not found", ref)
	}

	clean := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")
	var candidates []string
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		candidates = []string{ref}
	case strings.HasPrefix(ref, "refs/tags/"):
		candidates = []string{ref}
	default:
		candidates = []string{
			"refs/heads/" + clean,
			"refs/tags/" + clean,
			ref,
		}
	}

	refMap := make(map[string]AdvertisedRef, len(refs))
	for _, item := range refs {
		refMap[item.Name] = item
	}

	for _, cand := range candidates {
		if item, ok := refMap[cand]; ok {
			if item.PeeledHash != "" {
				return item.PeeledHash, nil
			}
			return item.Hash, nil
		}
	}
	return "", fmt.Errorf("git source ref %q was not found", ref)
}

// RevisionAt resolves the revision of a specific commit, fetching it into the mirror at depth 1 if not already present.
func (adapter GitRepositoryAdapter) RevisionAt(ctx context.Context, source Source, commit string) (Revision, error) {
	adapter = adapter.withLimits(source.Limits)
	if !validGitObject(commit) {
		return Revision{}, ErrInvalidLocator
	}
	if _, err := ValidateRemoteURLWithOptions(source.Locator.Repository, URLValidationOptions{AllowFile: adapter.AllowFileProtocol}); err != nil {
		return Revision{}, err
	}
	mirror, err := adapter.mirrorPath(source.Locator.Repository)
	if err != nil {
		return Revision{}, err
	}

	var rev Revision
	err = withMirrorLock(ctx, mirror, false, func() error {
		repository, err := adapter.openOrCloneLocked(ctx, mirror, source.Locator.Repository, source.Locator.Ref)
		if err != nil {
			return err
		}
		hash := plumbing.NewHash(commit)
		commitObj, err := repository.CommitObject(hash)
		if err != nil {
			refSpec := config.RefSpec(commit + ":refs/skillhub/commits/" + commit)
			remoteConfig := &config.RemoteConfig{
				Name:  "origin",
				URLs:  []string{source.Locator.Repository},
				Fetch: []config.RefSpec{refSpec},
			}
			remote := git.NewRemote(repository.Storer, remoteConfig)
			fetchErr := adapter.withSafeTransport(func() error {
				timeout := adapter.MirrorTimeout
				if timeout <= 0 {
					timeout = DefaultMirrorTimeout
				}
				fetchCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				return remote.FetchContext(fetchCtx, &git.FetchOptions{
					Depth:    1,
					Tags:     git.NoTags,
					Force:    true,
					RefSpecs: []config.RefSpec{refSpec},
				})
			})
			if fetchErr != nil && !errors.Is(fetchErr, git.NoErrAlreadyUpToDate) {
				if errors.Is(fetchErr, git.ErrExactSHA1NotSupported) ||
					isUnreachableCommitFetchError(fetchErr) {
					return ErrHistoryUnavailable
				}
				if errors.Is(fetchErr, ErrLimitExceeded) {
					return fetchErr
				}
				return fmt.Errorf("fetch commit: %w", fetchErr)
			}
			commitObj, err = repository.CommitObject(hash)
			if err != nil {
				return ErrHistoryUnavailable
			}
		}

		objectHash, err := scopedObjectHash(commitObj, source.Locator.Path)
		if err != nil {
			return err
		}
		rev = Revision{
			Kind:          "git-commit",
			Value:         commit,
			ContentDigest: Digest([]byte(objectHash.String())),
			ObservedAt:    adapter.Now().UTC(),
		}
		return nil
	})
	if err != nil {
		return Revision{}, err
	}
	return rev, nil
}

// CommitTime returns the committer time of the given commit from the local mirror.
func (adapter GitRepositoryAdapter) CommitTime(ctx context.Context, repository, commit string) (time.Time, error) {
	adapter = adapter.defaults()
	if !validGitObject(commit) {
		return time.Time{}, ErrInvalidLocator
	}
	mirror, err := adapter.mirrorPath(repository)
	if err != nil {
		return time.Time{}, err
	}
	var commitTime time.Time
	err = withMirrorLock(ctx, mirror, true, func() error {
		repo, openErr := adapter.openMirrorLocked(mirror)
		if openErr != nil {
			return openErr
		}
		hash := plumbing.NewHash(commit)
		commitObj, cErr := repo.CommitObject(hash)
		if cErr != nil {
			return cErr
		}
		commitTime = commitObj.Committer.When.UTC()
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return commitTime, nil
}

// ResolveRefCommit resolves a ref name or SHA to a full commit hash in the local mirror.
func (adapter GitRepositoryAdapter) ResolveRefCommit(ctx context.Context, remoteURL, ref string) (string, error) {
	adapter = adapter.defaults()
	repository, _, err := adapter.syncMirror(ctx, remoteURL, ref)
	if err != nil {
		return "", err
	}
	hash, err := resolveCommit(repository, ref)
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

// CommitHasSkill checks if the given commit hash at scopedPath contains a SKILL.md file.
func (adapter GitRepositoryAdapter) CommitHasSkill(ctx context.Context, remoteURL, ref, commitHash, scopedPath string) (bool, error) {
	adapter = adapter.defaults()
	mirror, err := adapter.mirrorPath(remoteURL)
	if err != nil {
		return false, err
	}
	var repository *git.Repository
	_ = withMirrorLock(ctx, mirror, true, func() error {
		var openErr error
		repository, openErr = adapter.openMirrorLocked(mirror)
		return openErr
	})
	if repository == nil {
		var syncErr error
		repository, _, syncErr = adapter.syncMirror(ctx, remoteURL, ref)
		if syncErr != nil {
			return false, syncErr
		}
	}
	hash := plumbing.NewHash(commitHash)
	commit, err := repository.CommitObject(hash)
	if err != nil {
		if newRepo, _, syncErr := adapter.syncMirror(ctx, remoteURL, ref); syncErr == nil {
			repository = newRepo
			commit, err = repository.CommitObject(hash)
		}
	}
	if err != nil {
		return false, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return false, err
	}
	targetPath := "SKILL.md"
	if scopedPath != "" && scopedPath != "." {
		targetPath = strings.TrimSuffix(scopedPath, "/") + "/SKILL.md"
	}
	entry, err := tree.FindEntry(targetPath)
	if err != nil {
		return false, nil
	}
	return entry.Mode == filemode.Regular || entry.Mode == filemode.Executable, nil
}

// isUnreachableCommitFetchError reports whether a single-commit fetch failed because the remote
// does not have that commit. A server that rejects an unknown SHA ("not our ref") exits while the
// client is still writing its haves, so the client may see a closed pipe before it reads the
// message; both outcomes mean the commit is unavailable.
func isUnreachableCommitFetchError(err error) bool {
	if errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"not our ref", "couldn't find remote ref", "broken pipe", "closed pipe"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
