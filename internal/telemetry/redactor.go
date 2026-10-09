package telemetry

import (
	"os"
	"regexp"
	"strings"
)

// Redactor sanitizes strings by stripping PII and normalizing paths.
type Redactor struct {
	homeDir       string
	workspacePath string
	lengthCap     int
}

// NewRedactor initializes a redactor with the given workspace path.
func NewRedactor(workspacePath string) *Redactor {
	home, _ := os.UserHomeDir()
	return &Redactor{
		homeDir:       home,
		workspacePath: workspacePath,
		lengthCap:     2048,
	}
}

var (
	emailRegex    = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	urlCredsRegex = regexp.MustCompile(`(?i)(https?|ftp)://[^:/\s]+:[^@/\s]+@`)
	tokenRegex    = regexp.MustCompile(`(?i)(?:sk-[a-zA-Z0-9]{20,}|ghp_[a-zA-Z0-9]{36}|Bearer\s+[a-zA-Z0-9\-\._~+/]+=*)`)
)

// Redact applies secret removal, path normalization, and length capping.
func (r *Redactor) Redact(text string) string {
	if text == "" {
		return ""
	}

	// Secrets
	text = urlCredsRegex.ReplaceAllString(text, "$1://[CREDS]@")
	text = emailRegex.ReplaceAllString(text, "[EMAIL]")
	text = tokenRegex.ReplaceAllString(text, "[TOKEN]")

	// Paths
	if r.workspacePath != "" {
		text = strings.ReplaceAll(text, r.workspacePath, "<repo>")
	}
	if r.homeDir != "" {
		text = strings.ReplaceAll(text, r.homeDir, "~")
	}

	// Length Cap
	if len(text) > r.lengthCap {
		text = text[:r.lengthCap-3] + "..."
	}

	return text
}
