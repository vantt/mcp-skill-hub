package source

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// GitHubProvider is the HostProvider for public github.com. It is the only registered provider.
// See HostProvider for what a self-hosted provider would need beyond this port.
type GitHubProvider struct{}

// Kind is the origin kind stored for skills added from GitHub.
func (GitHubProvider) Kind() string { return "github" }

// Host is the host name shown in user-facing text.
func (GitHubProvider) Host() string { return "github.com" }

// Matches reports whether the URL points at github.com.
func (GitHubProvider) Matches(u *url.URL) bool {
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")) == "github.com"
}

// ParseRoute parses a github.com URL into owner, repository and route kind.
func (GitHubProvider) ParseRoute(u *url.URL, explicitRef, explicitPath string) (*ParsedRoute, error) {
	// Address policy check
	if _, err := ValidateRemoteURL(u.String(), false); err != nil {
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

	if len(segments) > 2 {
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
			last := segments[len(segments)-1]
			if last == "SKILL.md" {
				routeKind = "blob"
				// The folder is the parent of SKILL.md
				rest = strings.Join(segments[3:len(segments)-1], "/")
			} else if filepath.Ext(last) != "" {
				return nil, fmt.Errorf("%w: link the skill folder or its SKILL.md", ErrInvalidLocator)
			} else {
				// The user linked a directory using /blob/ instead of /tree/
				routeKind = "tree"
				rest = strings.Join(segments[3:], "/")
			}
		default:
			return nil, fmt.Errorf("%w: unsupported GitHub route %q", ErrInvalidLocator, kind)
		}
	}

	return &ParsedRoute{
		Repository: canonicalRepo,
		Owner:      owner,
		RepoName:   repoName,
		RouteKind:  routeKind,
		Rest:       rest,
		Ref:        explicitRef,
		Path:       explicitPath,
	}, nil
}

func validGitHubName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	for _, r := range name {
		if !isGitHubNameRune(r) {
			return false
		}
	}
	return true
}

func isGitHubNameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return r == '-' || r == '_' || r == '.'
}
