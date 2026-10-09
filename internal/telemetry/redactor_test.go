package telemetry

import (
	"os"
	"strings"
	"testing"
)

func TestRedactor(t *testing.T) {
	home, _ := os.UserHomeDir()
	r := NewRedactor("/home/vantt/projects/mcp-skill-hub")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no secrets",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "email",
			input:    "contact me at test.user123@example.com please",
			expected: "contact me at [EMAIL] please",
		},
		{
			name:     "url credentials",
			input:    "fetch from https://user:secretpass@github.com/repo.git",
			expected: "fetch from https://[CREDS]@github.com/repo.git",
		},
		{
			name:     "tokens",
			input:    "my token is ghp_123456789012345678901234567890123456 and api sk-abcdef1234567890abcdef1234567890 Bearer abc.def.ghi",
			expected: "my token is [TOKEN] and api [TOKEN] [TOKEN]",
		},
		{
			name:     "paths",
			input:    "edited " + home + "/.bashrc and /home/vantt/projects/mcp-skill-hub/internal/app",
			expected: "edited ~/.bashrc and <repo>/internal/app",
		},
		{
			name:     "length cap",
			input:    strings.Repeat("a", 3000),
			expected: strings.Repeat("a", 2045) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact(tt.input)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
