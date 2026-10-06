package app

import (
	"testing"

	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestUpstreamSourceMatching(t *testing.T) {
	cases := []struct {
		name     string
		urlA     string
		urlB     string
		expected bool
	}{
		{
			name:     "github case insensitive and trailing slash git",
			urlA:     "https://github.com/A/B.git",
			urlB:     "https://github.com/a/b/",
			expected: true,
		},
		{
			name:     "gitlab case sensitive path",
			urlA:     "https://gitlab.example/A/B",
			urlB:     "https://gitlab.example/a/b",
			expected: false,
		},
		{
			name:     "different owner differs",
			urlA:     "https://github.com/A/skills",
			urlB:     "https://github.com/B/skills",
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sameRepository(tc.urlA, tc.urlB)
			if got != tc.expected {
				t.Fatalf("expected sameRepository(%q, %q) = %v, got %v", tc.urlA, tc.urlB, tc.expected, got)
			}
		})
	}
}

func TestUpstreamSourceIDDerivation(t *testing.T) {
	repo := "https://github.com/anthropics/skills"

	// 1. Initial ID without collision
	id1 := upstreamSourceID(repo, "main", map[string]bool{})
	if id1 != "anthropics-skills" {
		t.Fatalf("expected 'anthropics-skills', got %q", id1)
	}

	// 2. Collision with a different ref yields anthropics-skills-v1-2 for ref v1.2
	takenRef := map[string]bool{
		"anthropics-skills": true,
	}
	id2 := upstreamSourceID(repo, "v1.2", takenRef)
	if id2 != "anthropics-skills-v1-2" {
		t.Fatalf("expected 'anthropics-skills-v1-2', got %q", id2)
	}

	// 3. A skill named anthropics-skills forces the suffix
	takenSkill := map[string]bool{
		"anthropics-skills": true,
	}
	id3 := upstreamSourceID(repo, "v1.2", takenSkill)
	if id3 != "anthropics-skills-v1-2" {
		t.Fatalf("expected 'anthropics-skills-v1-2', got %q", id3)
	}

	// 4. Second collision yields -2
	takenSecond := map[string]bool{
		"anthropics-skills":      true,
		"anthropics-skills-v1-2": true,
	}
	id4 := upstreamSourceID(repo, "v1.2", takenSecond)
	if id4 != "anthropics-skills-v1-2-2" {
		t.Fatalf("expected 'anthropics-skills-v1-2-2', got %q", id4)
	}
}

func TestUpstreamSourcePurposeValidation(t *testing.T) {
	validRecord := `schema_version: 1
id: test-source
purpose: upstream
adapter: git
locator:
  repository: https://github.com/foo/bar
status: watching
identity:
  name: bar
  canonical: https://github.com/foo/bar
trust:
  source: community
  reviewed: false
monitoring:
  enabled: true
  cadence: weekly
limits:
  timeout_seconds: 60
  max_bytes: 10485760
  max_files: 100
  max_file_bytes: 1048576
`
	if _, err := sourcepkg.ParseRecord([]byte(validRecord)); err != nil {
		t.Fatalf("expected valid purpose: upstream to pass, got: %v", err)
	}

	invalidRecord := `schema_version: 1
id: test-source
purpose: other
adapter: git
locator:
  repository: https://github.com/foo/bar
status: watching
identity:
  name: bar
  canonical: https://github.com/foo/bar
trust:
  source: community
  reviewed: false
monitoring:
  enabled: true
  cadence: weekly
limits:
  timeout_seconds: 60
  max_bytes: 10485760
  max_files: 100
  max_file_bytes: 1048576
`
	if _, err := sourcepkg.ParseRecord([]byte(invalidRecord)); err == nil {
		t.Fatalf("expected ParseRecord to reject purpose: other, got nil error")
	}
}
