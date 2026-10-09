package telemetry

import (
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
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
	pemKeyRegex       = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	urlCredsRegex     = regexp.MustCompile(`(?i)(https?|ftp)://[^:/\s]+:[^@/\s]+@`)
	assignmentRegex   = regexp.MustCompile(`(?i)\b(api_?key|password|passwd|secret|token)\s*([:=])\s*([^\s,;]+)`)
	jwtRegex          = regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]+\b`)
	awsKeyRegex       = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	githubTokenRegex  = regexp.MustCompile(`\b(?:ghp_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9_]{82}|gho_[a-zA-Z0-9]{36}|ghs_[a-zA-Z0-9]{36}|ghu_[a-zA-Z0-9]{36})\b`)
	slackTokenRegex   = regexp.MustCompile(`\bxox[abpr]-[0-9a-zA-Z-]+\b`)
	genericTokenRegex = regexp.MustCompile(`(?i)\b(?:sk-[a-zA-Z0-9]{20,}|Bearer\s+[a-zA-Z0-9\-\._~+/]+=*)\b`)
	emailRegex        = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
)

// Redact applies secret removal, path normalization, and length capping.
func (r *Redactor) Redact(text string) string {
	if text == "" {
		return ""
	}

	// 1. Multiline PEM keys
	text = pemKeyRegex.ReplaceAllString(text, "[PRIVATE KEY]")

	// 2. URL credentials (before email/token patterns)
	text = urlCredsRegex.ReplaceAllString(text, "$1://[CREDS]@")

	// 3. Key/password/secret/token assignments
	text = assignmentRegex.ReplaceAllString(text, "$1$2[REDACTED]")

	// 4. JWT tokens
	text = jwtRegex.ReplaceAllString(text, "[JWT]")

	// 5. AWS Access Keys
	text = awsKeyRegex.ReplaceAllString(text, "[AWS KEY]")

	// 6. GitHub tokens
	text = githubTokenRegex.ReplaceAllString(text, "[GITHUB TOKEN]")

	// 7. Slack tokens
	text = slackTokenRegex.ReplaceAllString(text, "[SLACK TOKEN]")

	// 8. Generic tokens (Bearer, sk-...)
	text = genericTokenRegex.ReplaceAllString(text, "[TOKEN]")

	// 9. Emails
	text = emailRegex.ReplaceAllString(text, "[EMAIL]")

	// 10. Paths
	if r.workspacePath != "" {
		text = strings.ReplaceAll(text, r.workspacePath, "<repo>")
	}
	if r.homeDir != "" {
		text = strings.ReplaceAll(text, r.homeDir, "~")
	}

	// 11. Length cap on a valid rune boundary
	if r.lengthCap > 0 && len(text) > r.lengthCap {
		target := r.lengthCap - 3
		if target < 0 {
			target = 0
		}
		// Step back to a valid UTF-8 rune start
		for target > 0 && !utf8.RuneStart(text[target]) {
			target--
		}
		text = text[:target] + "..."
	}

	return text
}
