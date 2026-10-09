package telemetry

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRedactorClasses(t *testing.T) {
	home, _ := os.UserHomeDir()
	repo := "/home/vantt/projects/mcp-skill-hub"
	r := NewRedactor(repo)

	tests := []struct {
		name     string
		input    string
		contains string
		notCont  string
	}{
		{
			name:     "aws_access_key",
			input:    "Connect with AKIAIOSFODNN7EXAMPLE to S3",
			contains: "[AWS KEY]",
			notCont:  "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:     "github_tokens",
			input:    "tokens: ghp_123456789012345678901234567890123456 gho_abcdefghijklmnopqrstuvwxyz0123456789 github_pat_11AAAAAA01234567890123456789012345678901234567890123456789012345678901234567890123",
			contains: "[GITHUB TOKEN]",
			notCont:  "ghp_",
		},
		{
			name:     "slack_token",
			input:    "post message using xoxb-123456789012-3456789012345-abcdef123456",
			contains: "[SLACK TOKEN]",
			notCont:  "xoxb-123456789012",
		},
		{
			name:     "jwt",
			input:    "Authorization header eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c in request",
			contains: "[JWT]",
			notCont:  "eyJhbGci",
		},
		{
			name: "pem_private_key",
			input: `config:
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA0Y1+example+key+material+here
-----END RSA PRIVATE KEY-----
done`,
			contains: "[PRIVATE KEY]",
			notCont:  "MIIEowIBAAKCAQEA0Y1",
		},
		{
			name:     "key_value_assignment",
			input:    "connect with api_key=superSecretKey123 and password: myHiddenPassword!",
			contains: "api_key=[REDACTED]",
			notCont:  "superSecretKey123",
		},
		{
			name:     "email",
			input:    "send notification to developer.support+alert@company.co.uk",
			contains: "[EMAIL]",
			notCont:  "developer.support",
		},
		{
			name:     "url_credentials",
			input:    "git clone https://bot_user:p4ssw0rd@gitlab.internal.com/group/repo.git",
			contains: "https://[CREDS]@gitlab.internal.com",
			notCont:  "p4ssw0rd",
		},
		{
			name:     "generic_token",
			input:    "Authorization: Bearer mySecretToken123456789 and sk-1234567890abcdef1234567890abcdef",
			contains: "[TOKEN]",
			notCont:  "sk-1234567890abcdef",
		},
		{
			name:     "path_normalization",
			input:    "files in " + home + "/.config and " + repo + "/internal/telemetry",
			contains: "~/.config and <repo>/internal/telemetry",
			notCont:  home,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.Redact(tt.input)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("expected output to contain %q, got %q", tt.contains, got)
			}
			if strings.Contains(got, tt.notCont) {
				t.Errorf("expected output to NOT contain %q, got %q", tt.notCont, got)
			}
		})
	}
}

func TestRedactorRuneBoundaryTruncation(t *testing.T) {
	r := &Redactor{lengthCap: 30} // small cap
	vietnamese := "Xin chào thế giới! Đây là một câu tiếng Việt rất dài có nhiều ký tự đa byte."
	got := r.Redact(vietnamese)

	if !utf8.ValidString(got) {
		t.Fatalf("redacted string is not valid UTF-8: %q", got)
	}
	if len(got) > 30 {
		t.Fatalf("redacted string length %d exceeds cap 30: %q", len(got), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated string to end with '...': %q", got)
	}
}
