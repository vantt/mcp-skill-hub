package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
)

type externalConnection struct {
	request hostintegration.Request
	global  bool
}

// Only repair hosts already registered in the current project or user scope;
// doctor must not install extra hosts merely because another host is connected.
func externalConnections(workspace, binary string) []externalConnection {
	var targets []externalConnection
	if cwd, err := os.Getwd(); err == nil && cwd != workspace {
		targets = append(targets, externalConnection{request: hostintegration.Request{Workspace: workspace, Root: cwd, Scope: hostintegration.ScopeProject, Binary: binary}})
	}
	if home, err := os.UserHomeDir(); err == nil {
		targets = append(targets, externalConnection{request: hostintegration.Request{Workspace: workspace, Root: home, Scope: hostintegration.ScopeUser, Binary: binary}, global: true})
	}
	var connected []externalConnection
	for _, target := range targets {
		for _, adapter := range hostintegration.SupportedAdapters() {
			relative := adapter.ConfigRelativePath
			if target.global {
				relative = adapter.UserConfigRelativePath
			}
			raw, err := os.ReadFile(filepath.Join(target.request.Root, filepath.FromSlash(relative)))
			if err != nil {
				continue
			}
			registered := false
			if adapter.Host == hostintegration.HostCodex {
				registered = bytes.Contains(raw, []byte("mcp_servers.skillhub"))
			} else {
				var config struct {
					Servers map[string]json.RawMessage `json:"mcpServers"`
				}
				if json.Unmarshal(raw, &config) == nil {
					_, registered = config.Servers["skillhub"]
				}
			}
			if registered {
				target.request.Hosts = append(target.request.Hosts, adapter.Host)
			}
		}
		if len(target.request.Hosts) > 0 {
			connected = append(connected, target)
		}
	}
	return connected
}

func applyExternalConnections(ctx context.Context, workspace string, result Result) (Result, error) {
	binary, err := currentBinary()
	if err != nil {
		return Result{}, err
	}
	for _, target := range externalConnections(workspace, binary) {
		plan, err := hostintegration.Plan(ctx, target.request)
		if err != nil {
			return Result{}, err
		}
		applied, err := hostintegration.Apply(ctx, plan, hostintegration.ApplyOptions{Confirmed: true})
		if err != nil {
			return Result{}, err
		}
		for _, change := range applied.Changed {
			result.Items = append(result.Items, hostChangeItem(change.Host, hostintegration.LevelNativeSkillBestEffort, change.Kind, change.Path, "Remediated."))
		}
		if len(applied.Changed) > 0 {
			result.Status = StatusApplied
			result.Summary = "Workspace, catalog, and connected project/user integrations are current."
		}
	}
	return result, nil
}

func checkProjectAndGlobalConnections(ctx context.Context, root string, result Result) Result {
	binary, err := currentBinary()
	if err != nil {
		return result
	}
	for _, target := range externalConnections(root, binary) {
		plan, err := hostintegration.Plan(ctx, target.request)
		if err == nil && len(plan.Changes) == 0 {
			continue
		}
		id, summary, command := "project_connection_outdated", fmt.Sprintf("Current project connection at %s is outdated or needs repair.", target.request.Root), fmt.Sprintf("skillhub connect --project %s --workspace %s --yes", target.request.Root, root)
		if target.global {
			id, summary, command = "global_connection_outdated", "Global agent connection is outdated or needs repair.", "skillhub connect -g --workspace "+root+" --yes"
		}
		for _, host := range target.request.Hosts {
			command += " --host " + string(host)
		}
		result.Status = StatusActionRequired
		result.Items = append(result.Items, Item{ID: id, Summary: summary, Impact: "Run `" + command + "` to update."})
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Update agent connection", Command: command, RequiresConfirmation: true})
	}
	return result
}
