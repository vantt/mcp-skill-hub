package source

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseLocatorWithOptionsMatchesGitHubWrappers(t *testing.T) {
	urls := []string{
		"https://github.com/anthropics/skills",
		"https://github.com/anthropics/skills.git",
		"https://github.com/anthropics/skills/tree/main/skills/pdf",
		"https://github.com/anthropics/skills/blob/main/skills/pdf/SKILL.md",
		"https://github.com/anthropics/skills/blob/main/skills/pdf",
		"https://GitHub.com./anthropics/skills#v1",
	}
	for _, raw := range urls {
		viaRegistry, err := ParseLocatorWithOptions(raw, "", "", LocatorOptions{})
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		viaWrapper, err := ParseGitHubLocator(raw, "", "")
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if !reflect.DeepEqual(viaRegistry, viaWrapper) {
			t.Fatalf("%s: registry %+v differs from wrapper %+v", raw, viaRegistry, viaWrapper)
		}
		if viaRegistry.HostKind != "github" {
			t.Fatalf("%s: HostKind = %q, want github", raw, viaRegistry.HostKind)
		}
		if viaRegistry.OriginalURL != raw {
			t.Fatalf("%s: OriginalURL = %q", raw, viaRegistry.OriginalURL)
		}
	}
}

func TestParseLocatorWithOptionsUnsupportedHostKeepsMessage(t *testing.T) {
	_, err := ParseLocatorWithOptions("https://gitlab.com/owner/repo", "", "", LocatorOptions{})
	if !errors.Is(err, ErrInvalidLocator) {
		t.Fatalf("error = %v, want ErrInvalidLocator", err)
	}
	want := `invalid source locator: unsupported host "gitlab.com"; only public github.com is supported`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseLocatorWithOptionsFileFixtureUsesProviderKind(t *testing.T) {
	route, err := ParseLocatorWithOptions("file:///tmp/fixture/repo.git", "", "", LocatorOptions{AllowFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if route.HostKind != "github" || route.RepoName != "repo" {
		t.Fatalf("route = %+v", route)
	}
	if _, err := ParseLocatorWithOptions("file:///tmp/fixture/repo.git", "", "", LocatorOptions{}); err == nil {
		t.Fatal("file:// must stay refused without the test option")
	}
}

func TestHostRegistryHoldsOnlyGitHub(t *testing.T) {
	if got := SupportedHosts(); !reflect.DeepEqual(got, []string{"github.com"}) {
		t.Fatalf("SupportedHosts = %v", got)
	}
	provider := GitHubProvider{}
	if provider.Kind() != "github" || provider.Host() != "github.com" {
		t.Fatalf("provider = %s/%s", provider.Kind(), provider.Host())
	}
}

func TestKindForRepository(t *testing.T) {
	cases := map[string]string{
		"https://github.com/o/r.git":       "github",
		"https://GitHub.com/o/r":           "github",
		"git@github.com:o/r.git":           "github",
		"https://gitlab.com/o/r.git":       "git",
		"file:///tmp/fixture/repo.git":     "git",
		"https://example.com/github.com/r": "git",
		"":                                 "git",
	}
	for repo, want := range cases {
		if got := KindForRepository(repo); got != want {
			t.Errorf("KindForRepository(%q) = %q, want %q", repo, got, want)
		}
	}
}
