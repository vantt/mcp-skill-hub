package cli

import (
	"os"
	"strings"
)

// workspaceEnvVar names the environment variable that supplies a default
// workspace when --workspace is omitted. The flag wins over the variable, and
// the variable wins over upward discovery from the current directory. Home
// directories are never scanned.
const workspaceEnvVar = "SKILLHUB_WORKSPACE"

// defaultWorkspace returns the resolved workspace path when resolution succeeds,
// or the explicit flag/environment value. Commands should prefer resolveWorkspace
// to preserve actionable resolution error messages.
func defaultWorkspace(flagValue string) string {
	if ws, err := resolveWorkspace(flagValue); err == nil && ws != "" {
		return ws
	}
	if flagValue != "" {
		return flagValue
	}
	return strings.TrimSpace(os.Getenv(workspaceEnvVar))
}
