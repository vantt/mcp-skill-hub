package web

import (
	"strings"
	"testing"
)

func TestLocatorValidation(t *testing.T) {
	longURL := "https://github.com/acme/repo/tree/main/" + strings.Repeat("a", 3000)

	tests := []struct {
		name       string
		raw        string
		wantAccept bool
	}{
		{
			name:       "accept repo root",
			raw:        "https://github.com/acme/agent-skills",
			wantAccept: true,
		},
		{
			name:       "accept tree subfolder",
			raw:        "https://github.com/acme/agent-skills/tree/main/skills",
			wantAccept: true,
		},
		{
			name:       "accept blob file case-insensitive host",
			raw:        "https://GitHub.com/acme/repo/blob/main/SKILL.md",
			wantAccept: true,
		},
		{
			name:       "reject http scheme",
			raw:        "http://github.com/a/b",
			wantAccept: false,
		},
		{
			name:       "reject explicit port",
			raw:        "https://github.com:8443/a/b",
			wantAccept: false,
		},
		{
			name:       "reject userinfo",
			raw:        "https://user@github.com/a/b",
			wantAccept: false,
		},
		{
			name:       "reject single segment path",
			raw:        "https://github.com/a",
			wantAccept: false,
		},
		{
			name:       "reject query parameter",
			raw:        "https://github.com/a/b?x=1",
			wantAccept: false,
		},
		{
			name:       "reject non-github host",
			raw:        "https://gitlab.com/a/b",
			wantAccept: false,
		},
		{
			name:       "reject file scheme",
			raw:        "file:///etc",
			wantAccept: false,
		},
		{
			name:       "reject local path /etc/passwd",
			raw:        "/etc/passwd",
			wantAccept: false,
		},
		{
			name:       "reject relative local path",
			raw:        "./skills",
			wantAccept: false,
		},
		{
			name:       "reject git scp syntax",
			raw:        "git@github.com:a/b.git",
			wantAccept: false,
		},
		{
			name:       "reject ssh scheme",
			raw:        "ssh://github.com/a/b",
			wantAccept: false,
		},
		{
			name:       "reject 3000-character URL",
			raw:        longURL,
			wantAccept: false,
		},
		{
			name:       "reject empty string",
			raw:        "",
			wantAccept: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGitHubLocator(tc.raw)
			if tc.wantAccept && err != nil {
				t.Errorf("expected %q to be accepted, got error: %v", tc.raw, err)
			}
			if !tc.wantAccept && err == nil {
				t.Errorf("expected %q to be rejected, got nil error", tc.raw)
			}
			if !tc.wantAccept && err != nil {
				if !strings.Contains(err.Render.Why, "accepts only public GitHub URLs") {
					t.Errorf("unexpected error message: %v", err)
				}
			}
		})
	}
}
