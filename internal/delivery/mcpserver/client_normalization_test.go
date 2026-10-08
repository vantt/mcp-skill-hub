package mcpserver

import (
	"testing"
)

func TestClientNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		rawName     string
		rawVersion  string
		wantName    string
		wantVersion string
	}{
		{
			name:        "Claude Code standard",
			rawName:     "claude-code",
			rawVersion:  "1.2.3",
			wantName:    "claude-code",
			wantVersion: "1.2",
		},
		{
			name:        "Claude uppercase with v prefix",
			rawName:     "Claude Code",
			rawVersion:  "v2.1.0-alpha",
			wantName:    "claude-code",
			wantVersion: "2.1",
		},
		{
			name:        "Cursor editor",
			rawName:     "Cursor",
			rawVersion:  "0.41.2",
			wantName:    "cursor",
			wantVersion: "0.41",
		},
		{
			name:        "Codex CLI",
			rawName:     "codex-cli",
			rawVersion:  "3.0",
			wantName:    "codex",
			wantVersion: "3.0",
		},
		{
			name:        "Google Gemini",
			rawName:     "Google Gemini",
			rawVersion:  "1",
			wantName:    "gemini",
			wantVersion: "1",
		},
		{
			name:        "Unknown agent client",
			rawName:     "custom-bot-9000",
			rawVersion:  "4.5.6",
			wantName:    "other",
			wantVersion: "4.5",
		},
		{
			name:        "Garbled or weird clientInfo",
			rawName:     "weird !@#$%^&*() client",
			rawVersion:  "invalid/unbounded/version/string",
			wantName:    "other",
			wantVersion: "",
		},
		{
			name:        "Empty clientInfo",
			rawName:     "",
			rawVersion:  "",
			wantName:    "other",
			wantVersion: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeClient(tc.rawName, tc.rawVersion)
			if got.Name != tc.wantName {
				t.Errorf("NormalizeClient(%q, %q).Name = %q, want %q", tc.rawName, tc.rawVersion, got.Name, tc.wantName)
			}
			if got.Version != tc.wantVersion {
				t.Errorf("NormalizeClient(%q, %q).Version = %q, want %q", tc.rawName, tc.rawVersion, got.Version, tc.wantVersion)
			}
			if !validToken(got.Name) {
				t.Errorf("NormalizeClient(%q, %q).Name %q is not a valid token", tc.rawName, tc.rawVersion, got.Name)
			}
			if got.Version != "" && !validToken(got.Version) {
				t.Errorf("NormalizeClient(%q, %q).Version %q is not a valid token", tc.rawName, tc.rawVersion, got.Version)
			}
		})
	}
}
