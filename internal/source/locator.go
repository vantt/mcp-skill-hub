package source

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AuthorizedLocalRoot represents an in-memory, explicitly authorized filesystem root for one-shot capture.
// Accidental serialization (JSON, YAML, text) is prohibited and blocked.
type AuthorizedLocalRoot struct {
	rootPath string // unexported to prevent automatic field reflection or serialization
}

// NewAuthorizedLocalRoot constructs and validates an authorized local directory root.
// The root itself may be a symlink, which is resolved once. Any file path is adjusted to its parent directory.
func NewAuthorizedLocalRoot(rawPath string) (AuthorizedLocalRoot, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" || len(trimmed) > 4096 || strings.ContainsRune(trimmed, '\x00') {
		return AuthorizedLocalRoot{}, ErrInvalidLocator
	}

	if strings.HasPrefix(trimmed, "~/") || trimmed == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return AuthorizedLocalRoot{}, fmt.Errorf("%w: cannot resolve home directory", ErrInvalidLocator)
		}
		if trimmed == "~" {
			trimmed = home
		} else {
			trimmed = filepath.Join(home, filepath.FromSlash(trimmed[2:]))
		}
	}

	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return AuthorizedLocalRoot{}, fmt.Errorf("%w: %v", ErrInvalidLocator, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return AuthorizedLocalRoot{}, fmt.Errorf("%w: %v", ErrInvalidLocator, err)
	}

	targetDir := absPath
	if !info.IsDir() {
		targetDir = filepath.Dir(absPath)
		dirInfo, dirErr := os.Stat(targetDir)
		if dirErr != nil || !dirInfo.IsDir() {
			return AuthorizedLocalRoot{}, fmt.Errorf("%w: %s is not a directory", ErrInvalidLocator, targetDir)
		}
	}

	resolved, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		return AuthorizedLocalRoot{}, fmt.Errorf("%w: %v", ErrInvalidLocator, err)
	}

	resolvedInfo, err := os.Stat(resolved)
	if err != nil || !resolvedInfo.IsDir() {
		return AuthorizedLocalRoot{}, fmt.Errorf("%w: resolved root is not a directory", ErrInvalidLocator)
	}

	return AuthorizedLocalRoot{rootPath: resolved}, nil
}

// Path returns the authorized absolute directory path in memory.
func (a AuthorizedLocalRoot) Path() string {
	return a.rootPath
}

// MarshalJSON prohibits serialization of the transient local root.
func (AuthorizedLocalRoot) MarshalJSON() ([]byte, error) {
	return nil, errors.New("cannot serialize transient authorized local root")
}

// MarshalYAML prohibits serialization of the transient local root.
func (AuthorizedLocalRoot) MarshalYAML() (any, error) {
	return nil, errors.New("cannot serialize transient authorized local root")
}

// String masks the raw path to prevent leakage in default formatting.
func (AuthorizedLocalRoot) String() string {
	return "<authorized-local-root>"
}

// SnapshotReference is a serializable, privacy-safe reference to an immutable local snapshot.
// It contains only content-addressed handles and relative paths, never local host paths.
type SnapshotReference struct {
	SnapshotID    string `json:"snapshot_id" yaml:"snapshot_id"`
	ContentDigest string `json:"content_digest" yaml:"content_digest"`
	SelectedPath  string `json:"selected_path,omitempty" yaml:"selected_path,omitempty"`
}

// NormalizedLocator is a canonical, adapter-neutral, serializable representation of a source location.
type NormalizedLocator struct {
	Kind           string `json:"kind" yaml:"kind"` // "github", "git", "local-snapshot"
	Repository     string `json:"repository,omitempty" yaml:"repository,omitempty"`
	Ref            string `json:"ref,omitempty" yaml:"ref,omitempty"`
	Commit         string `json:"commit,omitempty" yaml:"commit,omitempty"`
	Path           string `json:"path,omitempty" yaml:"path,omitempty"`
	SnapshotID     string `json:"snapshot_id,omitempty" yaml:"snapshot_id,omitempty"`
	SnapshotDigest string `json:"snapshot_digest,omitempty" yaml:"snapshot_digest,omitempty"`
}

// ToLocator converts the normalized locator to a generic source Locator.
func (n NormalizedLocator) ToLocator() Locator {
	switch n.Kind {
	case "github", "git":
		return Locator{
			Repository:     n.Repository,
			Ref:            n.Ref,
			Path:           n.Path,
			SnapshotID:     n.SnapshotID,
			SnapshotDigest: n.SnapshotDigest,
		}
	case "local-snapshot":
		return Locator{
			Path:           n.Path,
			SnapshotID:     n.SnapshotID,
			SnapshotDigest: n.SnapshotDigest,
		}
	default:
		return Locator{
			Repository:     n.Repository,
			Ref:            n.Ref,
			Path:           n.Path,
			SnapshotID:     n.SnapshotID,
			SnapshotDigest: n.SnapshotDigest,
		}
	}
}

// ParsedGitHubRoute holds the structurally parsed components of a GitHub URL.
type ParsedGitHubRoute struct {
	OriginalURL string
	Repository  string // canonical repository URL: "https://github.com/owner/repo.git"
	Owner       string
	RepoName    string
	RouteKind   string // "root", "tree", "blob"
	Rest        string // remainder after /tree/ or /blob/
	Ref         string // explicit ref from fragment or flag
	Path        string // explicit path from flag
}

// ResolvedGitHubRoute holds the fully resolved and pinned GitHub reference and path.
type ResolvedGitHubRoute struct {
	Repository    string
	Ref           string
	Commit        string
	Path          string
	ContentDigest string
}

// GitHubLocatorOptions configures parsing options for GitHub locators.
type GitHubLocatorOptions struct {
	AllowFile bool
}

// ParseGitHubLocator parses a GitHub URL structurally into its repository, route kind, and path components.
func ParseGitHubLocator(rawURL, explicitRef, explicitPath string) (*ParsedGitHubRoute, error) {
	return ParseGitHubLocatorWithOptions(rawURL, explicitRef, explicitPath, GitHubLocatorOptions{})
}

// ParseGitHubLocatorWithOptions parses a GitHub URL structurally into its repository, route kind, and path components.
func ParseGitHubLocatorWithOptions(rawURL, explicitRef, explicitPath string, opts GitHubLocatorOptions) (*ParsedGitHubRoute, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return nil, ErrInvalidLocator
	}

	// Extract fragment if present
	if strings.Contains(value, "#") {
		parts := strings.SplitN(value, "#", 2)
		value = parts[0]
		fragment := strings.TrimSpace(parts[1])
		if explicitRef == "" && fragment != "" {
			explicitRef = fragment
		}
	}

	u, err := url.Parse(value)
	if err != nil || u.User != nil || u.RawQuery != "" {
		return nil, ErrInvalidLocator
	}

	if opts.AllowFile && u.Scheme == "file" {
		repoName := strings.TrimSuffix(filepath.Base(u.Path), ".git")
		if repoName == "" {
			repoName = "repo"
		}
		return &ParsedGitHubRoute{
			OriginalURL: rawURL,
			Repository:  value,
			Owner:       "local",
			RepoName:    repoName,
			RouteKind:   "root",
			Rest:        "",
			Ref:         explicitRef,
			Path:        explicitPath,
		}, nil
	}

	if u.Scheme != "https" {
		return nil, fmt.Errorf("%w: allowed protocol is https", ErrInvalidLocator)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host != "github.com" {
		return nil, fmt.Errorf("%w: unsupported host %q; only public github.com is supported", ErrInvalidLocator, host)
	}

	// Address policy check
	if _, err := ValidateRemoteURL(value, false); err != nil {
		return nil, err
	}

	cleanPath := strings.Trim(u.Path, "/")
	if cleanPath == "" {
		return nil, fmt.Errorf("%w: GitHub URL requires owner and repository name", ErrInvalidLocator)
	}

	segments := strings.Split(cleanPath, "/")
	if len(segments) < 2 {
		return nil, fmt.Errorf("%w: GitHub URL requires owner and repository name", ErrInvalidLocator)
	}

	owner := segments[0]
	repoName := strings.TrimSuffix(segments[1], ".git")
	if !validGitHubName(owner) || !validGitHubName(repoName) {
		return nil, fmt.Errorf("%w: invalid GitHub repository name", ErrInvalidLocator)
	}

	canonicalRepo := fmt.Sprintf("https://github.com/%s/%s.git", owner, repoName)
	routeKind := "root"
	rest := ""

	if len(segments) == 2 {
		routeKind = "root"
	} else {
		kind := segments[2]
		switch kind {
		case "tree":
			if len(segments) < 4 {
				return nil, fmt.Errorf("%w: tree route requires ref or branch", ErrInvalidLocator)
			}
			routeKind = "tree"
			rest = strings.Join(segments[3:], "/")
		case "blob":
			if len(segments) < 4 {
				return nil, fmt.Errorf("%w: blob route requires ref and file path", ErrInvalidLocator)
			}
			if segments[len(segments)-1] != "SKILL.md" {
				return nil, fmt.Errorf("%w: link the skill folder or its SKILL.md", ErrInvalidLocator)
			}
			routeKind = "blob"
			// The folder is the parent of SKILL.md
			rest = strings.Join(segments[3:len(segments)-1], "/")
		default:
			return nil, fmt.Errorf("%w: unsupported GitHub route %q", ErrInvalidLocator, kind)
		}
	}

	return &ParsedGitHubRoute{
		OriginalURL: rawURL,
		Repository:  canonicalRepo,
		Owner:       owner,
		RepoName:    repoName,
		RouteKind:   routeKind,
		Rest:        rest,
		Ref:         explicitRef,
		Path:        explicitPath,
	}, nil
}

// ResolveGitHubRoute resolves the commit hash and folder path for a parsed GitHub route.
func ResolveGitHubRoute(ctx context.Context, adapter GitRepositoryAdapter, route *ParsedGitHubRoute) (*ResolvedGitHubRoute, error) {
	if route == nil {
		return nil, ErrInvalidLocator
	}

	repoURL := route.Repository
	resolvedRef := route.Ref
	resolvedPath := route.Path

	if resolvedRef != "" {
		// Explicit ref provided
		if resolvedPath == "" && route.Rest != "" {
			if route.Rest == resolvedRef {
				resolvedPath = ""
			} else if strings.HasPrefix(route.Rest, resolvedRef+"/") {
				resolvedPath = route.Rest[len(resolvedRef)+1:]
			} else {
				shortRef := strings.TrimPrefix(strings.TrimPrefix(resolvedRef, "refs/heads/"), "refs/tags/")
				if route.Rest == shortRef {
					resolvedPath = ""
				} else if strings.HasPrefix(route.Rest, shortRef+"/") {
					resolvedPath = route.Rest[len(shortRef)+1:]
				} else {
					resolvedPath = route.Rest
				}
			}
		}

		commitSHA, err := adapter.ResolveRefCommit(ctx, repoURL, resolvedRef)
		if err != nil {
			return nil, err
		}
		return &ResolvedGitHubRoute{
			Repository: repoURL,
			Ref:        resolvedRef,
			Commit:     commitSHA,
			Path:       filepath.ToSlash(filepath.Clean(resolvedPath)),
		}, nil
	}

	// No explicit ref provided
	if route.RouteKind == "root" {
		// Resolve default branch
		advertised, err := adapter.ListAdvertisedRefs(ctx, repoURL)
		if err != nil {
			return nil, err
		}
		defaultBranch := "main"
		commitSHA := ""
		for _, ref := range advertised {
			if ref.Name == "refs/heads/main" {
				defaultBranch = "main"
				commitSHA = ref.Hash
				break
			}
			if ref.Name == "refs/heads/master" && commitSHA == "" {
				defaultBranch = "master"
				commitSHA = ref.Hash
			}
		}
		if commitSHA == "" && len(advertised) > 0 {
			for _, ref := range advertised {
				if strings.HasPrefix(ref.Name, "refs/heads/") {
					defaultBranch = strings.TrimPrefix(ref.Name, "refs/heads/")
					commitSHA = ref.Hash
					break
				}
			}
		}
		if commitSHA == "" {
			var resolveErr error
			commitSHA, resolveErr = adapter.ResolveRefCommit(ctx, repoURL, "HEAD")
			if resolveErr != nil {
				return nil, resolveErr
			}
		}
		return &ResolvedGitHubRoute{
			Repository: repoURL,
			Ref:        defaultBranch,
			Commit:     commitSHA,
			Path:       "",
		}, nil
	}

	// route.Rest is non-empty for tree or blob
	advertised, err := adapter.ListAdvertisedRefs(ctx, repoURL)
	if err != nil {
		return nil, err
	}

	type matchCandidate struct {
		name       string
		fullRef    string
		kind       string
		hash       string
		peeledHash string
	}

	var candidates []matchCandidate
	branchNames := make(map[string]bool)
	tagNames := make(map[string]bool)

	for _, ref := range advertised {
		if strings.HasPrefix(ref.Name, "refs/heads/") {
			name := strings.TrimPrefix(ref.Name, "refs/heads/")
			if route.Rest == name || strings.HasPrefix(route.Rest, name+"/") {
				branchNames[name] = true
				candidates = append(candidates, matchCandidate{
					name:    name,
					fullRef: ref.Name,
					kind:    "branch",
					hash:    ref.Hash,
				})
			}
		} else if strings.HasPrefix(ref.Name, "refs/tags/") {
			name := strings.TrimPrefix(ref.Name, "refs/tags/")
			if route.Rest == name || strings.HasPrefix(route.Rest, name+"/") {
				tagNames[name] = true
				targetHash := ref.Hash
				if ref.PeeledHash != "" {
					targetHash = ref.PeeledHash
				}
				candidates = append(candidates, matchCandidate{
					name:       name,
					fullRef:    ref.Name,
					kind:       "tag",
					hash:       targetHash,
					peeledHash: ref.PeeledHash,
				})
			}
		}
	}

	// Check for same-name branch and tag collision
	for name := range branchNames {
		if tagNames[name] {
			return nil, &AmbiguousRefError{
				Ref:        name,
				Candidates: []string{"refs/heads/" + name, "refs/tags/" + name},
			}
		}
	}

	// If no advertised ref matched, check for commit SHA
	if len(candidates) == 0 {
		firstSeg := strings.Split(route.Rest, "/")[0]
		if validGitObject(firstSeg) {
			path := strings.TrimPrefix(route.Rest, firstSeg)
			path = strings.TrimPrefix(path, "/")
			commitSHA, resolveErr := adapter.ResolveRefCommit(ctx, repoURL, firstSeg)
			if resolveErr == nil {
				return &ResolvedGitHubRoute{
					Repository: repoURL,
					Ref:        firstSeg,
					Commit:     commitSHA,
					Path:       filepath.ToSlash(filepath.Clean(path)),
				}, nil
			}
		}
		return nil, fmt.Errorf("%w: reference not found for %q", ErrInvalidLocator, route.Rest)
	}

	// If exactly one match
	if len(candidates) == 1 {
		c := candidates[0]
		path := strings.TrimPrefix(route.Rest, c.name)
		path = strings.TrimPrefix(path, "/")
		return &ResolvedGitHubRoute{
			Repository: repoURL,
			Ref:        c.name,
			Commit:     c.hash,
			Path:       filepath.ToSlash(filepath.Clean(path)),
		}, nil
	}

	// Multiple matches: sort by longest name first
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].name) > len(candidates[j].name)
	})

	// Check if exactly one interpretation contains SKILL.md
	type viableCandidate struct {
		cand matchCandidate
		path string
	}
	var withSkill []viableCandidate
	var candidateNames []string
	for _, c := range candidates {
		candPath := strings.TrimPrefix(route.Rest, c.name)
		candPath = strings.TrimPrefix(candPath, "/")
		candPath = filepath.ToSlash(filepath.Clean(candPath))
		candidateNames = append(candidateNames, c.fullRef)
		hasSkill, err := adapter.CommitHasSkill(ctx, repoURL, c.hash, candPath)
		if err == nil && hasSkill {
			withSkill = append(withSkill, viableCandidate{cand: c, path: candPath})
		}
	}

	if len(withSkill) == 1 {
		selected := withSkill[0]
		return &ResolvedGitHubRoute{
			Repository: repoURL,
			Ref:        selected.cand.name,
			Commit:     selected.cand.hash,
			Path:       selected.path,
		}, nil
	}

	return nil, &AmbiguousRefError{
		Ref:        route.Rest,
		Candidates: candidateNames,
	}
}

// ResolveGitHubLocator combines parsing and resolving a GitHub URL into a single step.
func ResolveGitHubLocator(ctx context.Context, adapter GitRepositoryAdapter, rawURL, explicitRef, explicitPath string) (*ResolvedGitHubRoute, error) {
	route, err := ParseGitHubLocator(rawURL, explicitRef, explicitPath)
	if err != nil {
		return nil, err
	}
	return ResolveGitHubRoute(ctx, adapter, route)
}

func validGitHubName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
