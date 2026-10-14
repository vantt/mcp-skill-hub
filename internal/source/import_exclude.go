package source

import (
	"path"
	"strings"
)

const (
	excludeReasonBuild  = "build output or version-control data"
	excludeReasonSecret = "looks like a secret"
	excludeReasonSystem = "system file"
)

var excludedDirectories = map[string]bool{
	".git":         true,
	"node_modules": true,
	"__pycache__":  true,
}

var excludedKeyNames = map[string]bool{
	"id_rsa":     true,
	"id_dsa":     true,
	"id_ecdsa":   true,
	"id_ed25519": true,
}

// ExcludedFromImport reports whether a slash path relative to a skill or source
// root must stay out of the hub, and why. It is the single rule shared by local
// capture, Git sources, source import and upstream updates.
func ExcludedFromImport(rel string) (reason string, excluded bool) {
	parts := strings.Split(strings.Trim(path.Clean(rel), "/"), "/")
	for _, dir := range parts[:len(parts)-1] {
		if excludedDirectories[dir] {
			return excludeReasonBuild, true
		}
	}
	base := parts[len(parts)-1]
	switch {
	case excludedDirectories[base]:
		return excludeReasonBuild, true
	case base == ".env":
		return excludeReasonSecret, true
	case strings.HasPrefix(base, ".env.") && base != ".env.example" && base != ".env.sample":
		return excludeReasonSecret, true
	case strings.HasSuffix(base, ".pem"):
		return excludeReasonSecret, true
	case excludedKeyNames[strings.TrimSuffix(base, "_sk")]:
		return excludeReasonSecret, true
	case base == ".DS_Store":
		return excludeReasonSystem, true
	}
	return "", false
}
