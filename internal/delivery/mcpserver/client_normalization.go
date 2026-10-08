package mcpserver

import (
	"regexp"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

var (
	semverPattern = regexp.MustCompile(`^v?([0-9]+)(?:\.([0-9]+))?`)
	tokenPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@+\-]{0,255}$`)
)

// NormalizeClient normalizes an MCP clientInfo name and version into the bounded
// enum claude-code | codex | cursor | gemini | other, with version at most major.minor.
// Any unknown or odd name maps to "other" and never causes an event to be rejected.
func NormalizeClient(name, version string) telemetry.Client {
	normalizedName := normalizeClientName(name)
	normalizedVersion := normalizeClientVersion(version)
	return telemetry.Client{
		Name:    normalizedName,
		Version: normalizedVersion,
	}
}

func normalizeClientName(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(clean, "claude"):
		return "claude-code"
	case strings.Contains(clean, "codex"):
		return "codex"
	case strings.Contains(clean, "cursor"):
		return "cursor"
	case strings.Contains(clean, "gemini"):
		return "gemini"
	default:
		return "other"
	}
}

func normalizeClientVersion(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return ""
	}
	match := semverPattern.FindStringSubmatch(v)
	if len(match) >= 2 && match[1] != "" {
		major := match[1]
		if len(match) >= 3 && match[2] != "" {
			result := major + "." + match[2]
			if validToken(result) {
				return result
			}
		}
		if validToken(major) {
			return major
		}
	}
	parts := strings.Split(v, ".")
	if len(parts) >= 2 {
		candidate := parts[0] + "." + parts[1]
		if validToken(candidate) {
			return candidate
		}
	}
	if len(parts) == 1 && validToken(parts[0]) {
		return parts[0]
	}
	return ""
}

func validToken(s string) bool {
	return tokenPattern.MatchString(s)
}
