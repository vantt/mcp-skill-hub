package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// ConnectService writes the agent-host connection for a workspace into a
// project directory or the user's home configuration.
type ConnectService struct {
	// HomeDir resolves the user home directory for global connections.
	// It defaults to os.UserHomeDir.
	HomeDir func() (string, error)
}

// ConnectRequest describes one connection. Project is ignored when Global is set.
type ConnectRequest struct {
	Project   string
	Workspace string
	Global    bool
	Hosts     []hostintegration.Host
	Yes       bool
	// Remove disconnects Claude Code, preserving user-owned configuration.
	Remove bool
}

// Connect previews the connection unless Yes is set. It reuses the same
// preimage-pinned host integration machinery as workspace initialization.
func (service ConnectService) Connect(ctx context.Context, request ConnectRequest) (Result, error) {
	workspacePath, problem := service.resolveWorkspace(request.Workspace)
	if problem != nil {
		return ErrorResult(problem), nil
	}
	root, scope, label, problem := service.target(request)
	if problem != nil {
		return ErrorResult(problem), nil
	}
	binary, err := currentBinary()
	if err != nil {
		return Result{}, err
	}
	if request.Remove && len(request.Hosts) == 0 {
		request.Hosts = []hostintegration.Host{hostintegration.HostClaude}
	}
	hostRequest := hostintegration.Request{Workspace: workspacePath, Root: root, Scope: scope, Binary: binary, Hosts: request.Hosts, Remove: request.Remove}
	inspection, err := hostintegration.Inspect(ctx, hostRequest)
	if err != nil {
		return Result{}, err
	}
	levels := make(map[hostintegration.Host]hostintegration.IntegrationLevel, len(inspection.Hosts))
	var conflicts []string
	for _, host := range inspection.Hosts {
		levels[host.Host] = host.Level
		for _, file := range host.Files {
			if file.Conflict != "" {
				conflicts = append(conflicts, fmt.Sprintf("%s: %s", file.Path, file.Conflict))
			}
		}
	}
	if len(conflicts) > 0 {
		return ErrorResult(NewInvalidRequestError(
			"Existing host files cannot be updated safely: "+strings.Join(conflicts, "; "),
			"Fix or remove the conflicting managed content in those files, then run `skillhub connect` again. Nothing was written.",
		)), nil
	}
	plan, err := hostintegration.Plan(ctx, hostRequest)
	if err != nil {
		return Result{}, err
	}
	if len(plan.Changes) == 0 {
		result := NewResult(StatusReady, "Agent connection for "+label+" is already current; nothing to change.")
		if request.Remove {
			result.Summary = "Agent connection for " + label + " has no removable managed content; nothing to change."
		}
		return result, nil
	}
	if !request.Yes {
		result := NewResult(StatusActionRequired, "Preview only: agent connection for "+label+" was not written without --yes.")
		if request.Remove {
			result.Summary = "Preview only: agent connection for " + label + " was not removed without --yes."
		}
		for _, change := range plan.Changes {
			item := hostChangeItem(change.Host, levels[change.Host], change.Kind, change.Path, "Will be written after confirmation.")
			if change.Preview != "" {
				item.Summary += "\nManaged diff preview:\n" + change.Preview
			}
			result.Items = append(result.Items, item)
		}
		result.SuggestedActions = []Action{{Label: "Write the agent connection", Command: connectCommand(request, workspacePath) + " --yes", RequiresConfirmation: true}}
		return result, nil
	}
	applied, err := hostintegration.Apply(ctx, plan, hostintegration.ApplyOptions{Confirmed: true})
	if err != nil {
		return Result{}, err
	}
	result := NewResult(StatusApplied, "Agent connection for "+label+" written.")
	if request.Remove {
		result.Summary = "Agent connection for " + label + " removed; user-owned content preserved."
	}
	for _, change := range applied.Changed {
		result.Items = append(result.Items, hostChangeItem(change.Host, levels[change.Host], change.Kind, change.Path, "Written."))
	}
	if len(applied.Changed) > 0 {
		result.Warnings = append(result.Warnings, hostBestEffortWarning())
	}
	return result, nil
}

func (service ConnectService) resolveWorkspace(path string) (string, *Error) {
	const workspaceFix = "Pass `--workspace <path>`, set the SKILLHUB_WORKSPACE environment variable, or run `skillhub init <path> --yes`."
	if path == "" {
		discovered, err := workspace.Discover("")
		if err != nil {
			return "", NewInvalidRequestError("No Skill Hub workspace was found from the current directory.", workspaceFix)
		}
		return discovered, nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", NewInvalidRequestError("The workspace path cannot be resolved: "+err.Error(), "Pass an existing workspace directory with `--workspace <path>`.")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", NewInvalidRequestError(fmt.Sprintf("No Skill Hub workspace at %s.", path), workspaceFix)
		}
		return "", NewInvalidRequestError("Cannot access workspace at "+path+": "+err.Error(), workspaceFix)
	}
	if !info.IsDir() {
		return "", NewInvalidRequestError(fmt.Sprintf("No Skill Hub workspace at %s.", path), workspaceFix)
	}
	if _, err := os.Stat(filepath.Join(absolute, ".skillhub", "schema-version")); err != nil {
		return "", NewInvalidRequestError(fmt.Sprintf("No Skill Hub workspace was found at %s.", path), workspaceFix)
	}
	return absolute, nil
}

func (service ConnectService) target(request ConnectRequest) (root string, scope hostintegration.Scope, label string, problem *Error) {
	if request.Global {
		homeDir := service.HomeDir
		if homeDir == nil {
			homeDir = os.UserHomeDir
		}
		home, err := homeDir()
		if err != nil || !filepath.IsAbs(home) {
			return "", "", "", NewInvalidRequestError("The user home directory cannot be determined.", "Set HOME, or connect a single project without `--global`.")
		}
		return filepath.Clean(home), hostintegration.ScopeUser, "your user account (all projects)", nil
	}
	project := request.Project
	if project == "" {
		current, err := os.Getwd()
		if err != nil {
			return "", "", "", NewInvalidRequestError("The current directory cannot be determined: "+err.Error(), "Pass `--project <dir>`.")
		}
		project = current
	}
	absolute, err := filepath.Abs(project)
	if err != nil {
		return "", "", "", NewInvalidRequestError("The project path cannot be resolved: "+err.Error(), "Pass an existing directory with `--project <dir>`.")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", "", "", NewInvalidRequestError("The project path is not an existing directory: "+absolute, "Create the directory or pass another one with `--project <dir>`.")
	}
	return absolute, hostintegration.ScopeProject, "project " + absolute, nil
}

func connectCommand(request ConnectRequest, workspacePath string) string {
	parts := []string{"skillhub connect"}
	if request.Remove {
		parts[0] = "skillhub disconnect"
	}
	if request.Global {
		parts = append(parts, "--global")
	} else if request.Project != "" {
		parts = append(parts, "--project", request.Project)
	}
	parts = append(parts, "--workspace", workspacePath)
	for _, host := range request.Hosts {
		parts = append(parts, "--host", string(host))
	}
	return strings.Join(parts, " ")
}
