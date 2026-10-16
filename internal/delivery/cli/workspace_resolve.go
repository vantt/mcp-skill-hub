package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// workspaceFix is the fix line for an explicit --workspace or
// SKILLHUB_WORKSPACE value that does not name a hub.
const workspaceFix = "Pass the folder of a hub on this machine with `--workspace <path>` (a local path; a hub on another machine is used with `skillhub connect --hub <url>`)."

// WorkspaceResolutionError encapsulates a user-facing workspace resolution failure.
type WorkspaceResolutionError struct {
	Path string
	Why  string
	Fix  string
}

func (e *WorkspaceResolutionError) Error() string {
	return e.Why
}

// writeWorkspaceResolutionError renders a workspace resolution error using the standard error format.
func writeWorkspaceResolutionError(stdout, stderr io.Writer, jsonOutput bool, err *WorkspaceResolutionError) int {
	return writeInvalidRequest(stdout, stderr, jsonOutput, err.Why, err.Fix)
}

// resolveWorkspace implements the resolution order:
//  1. --workspace flag
//  2. SKILLHUB_WORKSPACE environment variable
//  3. a project connection (from the current directory upward) to a hub on this machine
//  4. the hub folder that contains the current directory
//  5. a user-scope connection to a hub on this machine
//
// Steps 3-5 read what app.DetectRole found; the home directory is never
// scanned. The error explains the role of the current directory.
func resolveWorkspace(flagValue string) (string, *WorkspaceResolutionError) {
	flagValue = strings.TrimSpace(flagValue)
	if flagValue != "" {
		return validateExplicitWorkspace(flagValue)
	}
	envValue := strings.TrimSpace(os.Getenv(workspaceEnvVar))
	if envValue != "" {
		return validateExplicitWorkspace(envValue)
	}
	report := app.DetectRoleHere()
	if report.Cwd == "" || report.Cwd == "." {
		return "", &WorkspaceResolutionError{Why: "The current directory cannot be determined.", Fix: workspaceFix}
	}
	return resolveFromRole(report)
}

// resolveFromRole picks the hub for a role report, or explains why there is none.
func resolveFromRole(report app.RoleReport) (string, *WorkspaceResolutionError) {
	localHub := func(global bool) string {
		for _, host := range report.Hosts {
			entry := host.Project
			if global {
				if host.Project != nil {
					continue
				}
				entry = host.User
			}
			if entry != nil && !entry.Remote() && app.IsHubRoot(entry.Workspace) {
				return entry.Workspace
			}
		}
		return ""
	}
	if hub := localHub(false); hub != "" {
		return hub, nil
	}
	if report.Hub != nil {
		return report.Hub.Root, nil
	}
	if hub := localHub(true); hub != "" {
		return hub, nil
	}
	return "", roleResolutionError(report)
}

// roleResolutionError explains, by role, why no hub on this machine applies to
// the current directory, and names the one command to run.
func roleResolutionError(report app.RoleReport) *WorkspaceResolutionError {
	storedExists := app.IsHubRoot(report.StoredHub)
	for _, entry := range report.EffectiveEntries() {
		if entry.Remote() || app.IsHubRoot(entry.Workspace) {
			continue
		}
		missing := entry.Workspace
		if missing == "" {
			missing = "the path in " + entry.Path
		}
		return &WorkspaceResolutionError{Path: entry.Workspace, Why: "The hub at " + missing + " is gone.", Fix: app.MovedHubFix(report, entry.Workspace)}
	}
	for _, entry := range report.EffectiveEntries() {
		if entry.Remote() {
			return &WorkspaceResolutionError{
				Why: "This project uses the hub at " + entry.HubURL + ". Skills are curated on the hub machine.",
				Fix: "Run this command on the hub machine. Here, `skillhub doctor` checks the connection to that hub.",
			}
		}
	}
	switch {
	case storedExists:
		return &WorkspaceResolutionError{
			Why: "This folder is not a hub or a connected project.",
			Fix: "To use the hub at " + report.StoredHub + " from every project, run `skillhub connect -g` (or `skillhub connect` for this project only).",
		}
	case report.StoredHub != "":
		return &WorkspaceResolutionError{
			Path: report.StoredHub,
			Why:  "The hub stored at " + report.StoredHub + " is gone.",
			Fix:  "If you moved it, run `" + shellquote.Chain("cd <new-location>", "skillhub init") + "` there.",
		}
	}
	return &WorkspaceResolutionError{
		Why: "This folder is not a hub or a connected project.",
		Fix: "To create a hub, run `" + app.NewHubCommand() + "`.",
	}
}

// validateExplicitWorkspace verifies that an explicitly provided workspace path exists and is initialized.
func validateExplicitWorkspace(rawPath string) (string, *WorkspaceResolutionError) {
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		return "", &WorkspaceResolutionError{
			Path: rawPath,
			Why:  fmt.Sprintf("The workspace path cannot be resolved: %v", err),
			Fix:  workspaceFix,
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", &WorkspaceResolutionError{
				Path: rawPath,
				Why:  fmt.Sprintf("No Skill Hub workspace at %s.", rawPath),
				Fix:  workspaceFix,
			}
		}
		return "", &WorkspaceResolutionError{
			Path: rawPath,
			Why:  fmt.Sprintf("Cannot access workspace at %s: %v", rawPath, err),
			Fix:  workspaceFix,
		}
	}
	if !info.IsDir() {
		return "", &WorkspaceResolutionError{
			Path: rawPath,
			Why:  fmt.Sprintf("No Skill Hub workspace at %s.", rawPath),
			Fix:  workspaceFix,
		}
	}
	schemaFile := filepath.Join(abs, ".skillhub", "schema-version")
	if _, err := os.Stat(schemaFile); err != nil {
		if _, skillhubErr := os.Stat(filepath.Join(abs, ".skillhub")); skillhubErr == nil {
			return abs, nil
		}
		return "", &WorkspaceResolutionError{
			Path: rawPath,
			Why:  fmt.Sprintf("No Skill Hub workspace was found at %s.", rawPath),
			Fix:  workspaceFix,
		}
	}
	return abs, nil
}
