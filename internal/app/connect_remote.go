package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

// RemoteConnectRequest points a scope at a hub on another machine.
type RemoteConnectRequest struct {
	Project   string
	Global    bool
	Hosts     []hostintegration.Host
	Hub       string
	TokenFile string
	// DryRun reports the changes without writing them.
	DryRun bool
	// HubClient sends the reachability probe; nil uses the default client.
	HubClient *http.Client
}

// RemoteConnection describes a remote hub connection for the preview and the
// result. It never holds the token.
type RemoteConnection struct {
	Hub       string `json:"hub"`
	TokenFile string `json:"token_file,omitempty"`
	// TokenSource is "file" (--token-file) or "environment"
	// (SKILLHUB_HUB_TOKEN in the environment the agent host starts in).
	TokenSource string `json:"token_source"`
	CacheDir    string `json:"cache_dir"`
	// Check is what the hub answered right now; a hub that cannot be reached
	// does not stop the connection, because a tunnel may not be open yet.
	Check HubProbe `json:"hub_check"`
	// Replaces lists the entries of the scope that pointed at another hub.
	Replaces []ScopeEntry `json:"replaces,omitempty"`
	// NotInstalled names what a local connection writes and a remote one
	// does not, because curation runs only on the hub machine.
	NotInstalled []string `json:"not_installed"`
}

// ScopeEntry names what a host entry in a scope points at.
type ScopeEntry struct {
	Host   string `json:"host"`
	Target string `json:"target"`
}

// NormalizeConnectHubURL checks a hub URL given to connect: http or https, a
// host, and no path other than "/". It returns the URL the proxy is started
// with, without a trailing slash.
func NormalizeConnectHubURL(raw string) (string, *Error) {
	fail := func(why string) (string, *Error) {
		return "", &Error{Code: ErrorInvalidRequest, Render: ErrorRender{
			Error: "The hub URL cannot be used.",
			Why:   why,
			Fix:   "Pass the base URL the hub printed when it started, for example --hub http://127.0.0.1:7420.",
		}}
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil && !strings.Contains(raw, "://") {
		// A bare host:port does not even parse as a URL.
		return fail(fmt.Sprintf("%q must start with http:// or https://.", raw))
	}
	if err != nil {
		return fail(fmt.Sprintf("%q is not a URL.", raw))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fail(fmt.Sprintf("%q must start with http:// or https://.", raw))
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return fail(fmt.Sprintf("%q has no host.", raw))
	}
	if parsed.User != nil {
		return fail(fmt.Sprintf("%q contains a user name or password; the token is passed with --token-file, never in the URL.", raw))
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return fail(fmt.Sprintf("%q has a query or fragment; the hub URL is only scheme, host and port.", raw))
	}
	if parsed.Path != "" && parsed.Path != "/" {
		base := parsed.Scheme + "://" + parsed.Host
		return "", &Error{Code: ErrorInvalidRequest, Render: ErrorRender{
			Error: "The hub URL cannot be used.",
			Why:   fmt.Sprintf("%q has the path %s; the hub URL has no path (the proxy adds /mcp itself).", raw, parsed.Path),
			Fix:   "Pass --hub " + base + ".",
		}}
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// checkTokenFile makes the token file path absolute and refuses a file the
// proxy would refuse: missing, not a token, or readable by others (Unix only;
// Windows files have no such permission bits).
func checkTokenFile(path string) (string, *Error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", NewInvalidRequestError("The token file path cannot be resolved: "+err.Error(), "Pass an absolute --token-file path.")
	}
	absolute = filepath.Clean(absolute)
	if _, err := hubtoken.Read(absolute); err != nil {
		render := ErrorRender{Error: "The token file " + absolute + " cannot be used."}
		switch {
		case errors.Is(err, fs.ErrNotExist):
			render.Why = "The file does not exist on this machine."
			render.Fix = tokenCopyFix(absolute) + " Or leave out --token-file and set " + HubTokenEnv + " in the environment the agent host starts in."
		case errors.Is(err, hubtoken.ErrInsecureMode):
			render.Why = "Other users can read it; the proxy only uses a token file readable by you alone."
			render.Fix = "Run `chmod 600 " + absolute + "`, then run this command again."
		case errors.Is(err, hubtoken.ErrInvalidToken):
			render.Why = "It does not hold a hub token."
			render.Fix = tokenCopyFix(absolute)
		default:
			render.Why = "It cannot be read: " + plainError(err) + "."
			render.Fix = "Check that you can read the file, or copy the hub's token file again."
		}
		return "", &Error{Code: ErrorInvalidRequest, Render: render}
	}
	return absolute, nil
}

// ConnectRemote writes, or with DryRun only reports, host entries that start the
// stdio proxy for a hub on another machine. It writes no curation server and
// no system-curator skill, because curation runs only on the hub machine, and
// never writes the token, only the token file path.
func (service ConnectService) ConnectRemote(ctx context.Context, request RemoteConnectRequest) (Result, *RemoteConnection, error) {
	hubURL, problem := NormalizeConnectHubURL(request.Hub)
	if problem != nil {
		return ErrorResult(problem), nil, nil
	}
	tokenFile := ""
	if request.TokenFile != "" {
		tokenFile, problem = checkTokenFile(request.TokenFile)
		if problem != nil {
			return ErrorResult(problem), nil, nil
		}
	}
	root, scope, label, problem := service.target(ConnectRequest{Project: request.Project, Global: request.Global})
	if problem != nil {
		return ErrorResult(problem), nil, nil
	}
	binary, err := currentBinary()
	if err != nil {
		return Result{}, nil, err
	}
	cacheDir, err := hostintegration.RemoteAccessDir(hubURL)
	if err != nil {
		return ErrorResult(NewInvalidRequestError("This machine has no directory for skills fetched from a hub: "+err.Error()+".", "Set XDG_STATE_HOME to a writable directory, then run this command again.")), nil, nil
	}
	hostRequest := hostintegration.Request{Root: root, Scope: scope, Binary: binary, Hosts: request.Hosts, Hub: &hostintegration.RemoteHub{URL: hubURL, TokenFile: tokenFile}}
	connection := &RemoteConnection{Hub: hubURL, TokenFile: tokenFile, TokenSource: "file", CacheDir: cacheDir, NotInstalled: []string{"curation server", "system-curator skill"}}
	if tokenFile == "" {
		connection.TokenSource = "environment"
	}
	for _, entry := range scopeEntries(root, scope, request.Hosts) {
		if entry.Target != "" && entry.Target != remoteTarget(hubURL, tokenFile) {
			connection.Replaces = append(connection.Replaces, entry)
		}
	}
	result, err := service.planScope(ctx, hostRequest, request.DryRun)
	if err != nil || result.Error != nil {
		return result, nil, err
	}
	token := ReadHubToken(hubURL, tokenFile)
	connection.Check = ProbeHub(ctx, request.HubClient, hubURL, token.Value)
	switch result.Status {
	case StatusReady:
		result.Summary = "Agent connection for " + label + " to the hub at " + hubURL + " is already current; nothing to change."
	case StatusActionRequired:
		result.Summary = "Dry run: agent connection for " + label + " to the hub at " + hubURL + " was not written."
		result.SuggestedActions = []Action{{Label: "Write the agent connection", Command: remoteConnectCommand(request, hubURL, tokenFile), RequiresConfirmation: true}}
	case StatusApplied:
		result.Summary = "Agent connection for " + label + " to the hub at " + hubURL + " written."
		result.Warnings = append(result.Warnings, hostBestEffortWarning())
	}
	result.Warnings = append(result.Warnings, remoteConnectWarnings(hubURL, connection.Check, token)...)
	return result, connection, nil
}

// planScope runs the inspect, conflict, plan and (unless dryRun) apply
// sequence of connect for a remote or removal request. The caller sets the
// summary.
func (service ConnectService) planScope(ctx context.Context, hostRequest hostintegration.Request, dryRun bool) (Result, error) {
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
		return NewResult(StatusReady, ""), nil
	}
	if dryRun {
		result := NewResult(StatusActionRequired, "")
		for _, change := range plan.Changes {
			impact := "Would be written."
			if change.Remove {
				impact = "Would be deleted."
			}
			item := hostChangeItem(change.Host, levels[change.Host], change.Kind, change.Path, impact)
			if change.Preview != "" {
				item.Summary += "\nManaged diff preview:\n" + change.Preview
			}
			result.Items = append(result.Items, item)
		}
		return result, nil
	}
	applied, err := hostintegration.Apply(ctx, plan, hostintegration.ApplyOptions{Confirmed: true})
	if err != nil {
		return Result{}, err
	}
	result := NewResult(StatusApplied, "")
	for _, change := range applied.Changed {
		impact := "Written."
		if change.Remove {
			impact = "Deleted."
		}
		result.Items = append(result.Items, hostChangeItem(change.Host, levels[change.Host], change.Kind, change.Path, impact))
	}
	return result, nil
}

func remoteTarget(hubURL, tokenFile string) string {
	target := "the remote hub at " + hubURL
	if tokenFile != "" {
		target += " (token file " + tokenFile + ")"
	}
	return target
}

// scopeEntries describes what each selected host's entry in the scope points
// at now; Target is empty for a host without an entry.
func scopeEntries(root string, scope hostintegration.Scope, hosts []hostintegration.Host) []ScopeEntry {
	var entries []ScopeEntry
	for _, adapter := range hostintegration.SupportedAdapters() {
		if len(hosts) > 0 && !slices.Contains(hosts, adapter.Host) {
			continue
		}
		relative := adapter.ConfigRelativePath
		if scope == hostintegration.ScopeUser {
			relative = adapter.UserConfigRelativePath
		}
		entry := ScopeEntry{Host: HostDisplayName(adapter.Host)}
		raw, err := readSmallFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err == nil {
			registration := hostintegration.ReadRegistration(adapter.Host, raw)
			switch {
			case registration.Remote():
				entry.Target = remoteTarget(registration.HubURL, registration.TokenFile)
			case registration.Present && registration.Workspace != "":
				entry.Target = "the local hub at " + registration.Workspace
			case registration.Present:
				entry.Target = "a local hub"
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func remoteConnectWarnings(hubURL string, probe HubProbe, token HubToken) []Warning {
	var warnings []Warning
	if token.Source == "environment" {
		warnings = append(warnings, Warning{Code: "hub_token_from_environment", Summary: "No --token-file: the agent host must start the proxy with " + HubTokenEnv + " set to the hub's token, or the proxy cannot authenticate."})
	}
	switch probe.Outcome {
	case ProbeUnreachable:
		warnings = append(warnings, Warning{Code: "hub_unreachable", Summary: "The hub does not answer at " + hubURL + " right now (" + probe.Reason + "). The connection can be written anyway; start `skillhub serve mcp --addr ...` on machine B and open the SSH tunnel or Tailscale connection before you start the agent."})
	case ProbeNotAHub, ProbeHostRefused, ProbeHubError:
		warnings = append(warnings, Warning{Code: "hub_check_failed", Summary: reachableCheck(hubURL, probe).Detail})
	case ProbeRejected:
		warnings = append(warnings, Warning{Code: "hub_token_rejected", Summary: "The hub at " + hubURL + " refuses this token; copy the current token file from machine B before you start the agent."})
	case ProbeAccepted:
		if check := versionCheck(hubURL, probe); check.Status == CheckFailed {
			warnings = append(warnings, Warning{Code: "hub_version_skew", Summary: check.Detail})
		}
	}
	return warnings
}

func remoteConnectCommand(request RemoteConnectRequest, hubURL, tokenFile string) string {
	parts := []string{"skillhub connect"}
	if request.Global {
		parts = append(parts, "--global")
	} else if request.Project != "" {
		parts = append(parts, "--project", shellquote.Arg(request.Project))
	}
	parts = append(parts, "--hub", hubURL)
	if tokenFile != "" {
		parts = append(parts, "--token-file", shellquote.Arg(tokenFile))
	}
	for _, host := range request.Hosts {
		parts = append(parts, "--host", string(host))
	}
	return strings.Join(parts, " ")
}

// DisconnectRequest removes the Skill Hub connection of a scope.
type DisconnectRequest struct {
	Project string
	Global  bool
	Hosts   []hostintegration.Host
	// ClearCache also removes the skills this machine fetched from the remote
	// hubs the removed entries pointed at.
	ClearCache bool
	// DryRun reports the removal without changing anything.
	DryRun bool
}

// Disconnection describes what a removal takes out.
type Disconnection struct {
	Entries []ScopeEntry `json:"entries"`
	Cache   []CacheUsage `json:"cache,omitempty"`
}

// CacheUsage is the cached skill content of one remote hub.
type CacheUsage struct {
	Hub      string `json:"hub"`
	CacheDir string `json:"cache_dir"`
	Skills   int    `json:"skills"`
	Versions int    `json:"versions"`
	Bytes    int64  `json:"bytes"`
}

// Disconnect applies, or with DryRun only reports, the removal of every Skill Hub
// surface connect wrote in a scope, for a local or a remote hub: the MCP
// entries, their allowed directories and permission rules, the instruction
// block and an unchanged system-curator skill. Unrelated host configuration
// stays as it is.
func (service ConnectService) Disconnect(ctx context.Context, request DisconnectRequest) (Result, *Disconnection, error) {
	root, scope, label, problem := service.target(ConnectRequest{Project: request.Project, Global: request.Global})
	if problem != nil {
		return ErrorResult(problem), nil, nil
	}
	binary, err := currentBinary()
	if err != nil {
		return Result{}, nil, err
	}
	disconnection := &Disconnection{}
	var hubs []string
	for _, entry := range scopeEntries(root, scope, request.Hosts) {
		if entry.Target == "" {
			continue
		}
		disconnection.Entries = append(disconnection.Entries, entry)
	}
	for _, adapter := range hostintegration.SupportedAdapters() {
		if len(request.Hosts) > 0 && !slices.Contains(request.Hosts, adapter.Host) {
			continue
		}
		relative := adapter.ConfigRelativePath
		if scope == hostintegration.ScopeUser {
			relative = adapter.UserConfigRelativePath
		}
		if raw, err := readSmallFile(filepath.Join(root, filepath.FromSlash(relative))); err == nil {
			if registration := hostintegration.ReadRegistration(adapter.Host, raw); registration.Remote() && !slices.Contains(hubs, registration.HubURL) {
				hubs = append(hubs, registration.HubURL)
			}
		}
	}
	var warnings []Warning
	var cache skillruntime.RemoteCache
	cacheToClear := false
	if request.ClearCache {
		if len(hubs) == 0 {
			warnings = append(warnings, Warning{Code: "no_remote_hub", Summary: "--cache clears the skills fetched from a remote hub, and this scope has no remote hub entry; there is no cache to clear."})
		} else {
			state, err := skillruntime.ClientStateDir()
			if err != nil {
				return ErrorResult(NewInvalidRequestError("This machine has no directory for cached skills: "+err.Error()+".", "Set XDG_STATE_HOME to the directory the proxy uses, or leave out --cache.")), nil, nil
			}
			cache = skillruntime.RemoteCache{Root: skillruntime.RemoteCacheDir(state)}
			for _, hub := range hubs {
				hubID, err := skillruntime.HubID(hub)
				if err != nil {
					continue
				}
				usage, err := cache.Usage(hubID)
				if err != nil {
					return Result{}, nil, fmt.Errorf("read the skill cache of %s: %w", hub, err)
				}
				item := CacheUsage{Hub: hub, CacheDir: cache.HubDir(hubID)}
				for _, entry := range usage {
					item.Skills, item.Versions, item.Bytes = entry.Skills, entry.Versions, entry.Bytes
				}
				cacheToClear = cacheToClear || item.Versions > 0
				disconnection.Cache = append(disconnection.Cache, item)
			}
			warnings = append(warnings, sharedHubWarnings(hubs, request.Global, root)...)
		}
	}
	if !request.DryRun && request.ClearCache {
		// The cache goes first: if clearing fails, the connection is still
		// in place and the same command can be run again.
		for _, hub := range hubs {
			hubID, err := skillruntime.HubID(hub)
			if err != nil {
				continue
			}
			if _, err := cache.Clear(hubID); err != nil {
				return Result{}, nil, fmt.Errorf("clear the skill cache of %s: %w", hub, err)
			}
		}
	}
	hostRequest := hostintegration.Request{Root: root, Scope: scope, Binary: binary, Hosts: request.Hosts, Remove: true}
	result, err := service.planScope(ctx, hostRequest, request.DryRun)
	if err != nil || result.Error != nil {
		return result, nil, err
	}
	if request.Global && removesInstructions(result) {
		warnings = append(warnings, Warning{Code: "global_instructions_removed", Summary: "Projects that were connected while your user account had the Skill Hub instructions did not get their own copy; run `skillhub connect` in those projects again if they keep using Skill Hub."})
	}
	result.Warnings = append(result.Warnings, warnings...)
	if result.Status == StatusReady && !cacheToClear {
		result.Summary = "No Skill Hub connection in " + label + "; nothing to remove."
		return result, disconnection, nil
	}
	if request.DryRun {
		result.Status = StatusActionRequired
		result.Summary = "Dry run: the Skill Hub connection for " + label + " was not removed."
		result.SuggestedActions = []Action{{Label: "Remove the agent connection", Command: disconnectCommand(request), RequiresConfirmation: true}}
		return result, disconnection, nil
	}
	result.Status = StatusApplied
	result.Summary = "Skill Hub connection for " + label + " removed."
	return result, disconnection, nil
}

// removesInstructions reports whether a removal takes out an instruction block.
func removesInstructions(result Result) bool {
	for _, item := range result.Items {
		if strings.HasSuffix(item.ID, "_"+string(hostintegration.ChangeBootstrap)) {
			return true
		}
	}
	return false
}

// sharedHubWarnings names the other scope that still uses a hub whose cache
// is about to be cleared: the user account when a project is disconnected,
// the current project when the user account is.
func sharedHubWarnings(hubs []string, global bool, root string) []Warning {
	project, home := "", ""
	if global {
		if current, err := os.Getwd(); err == nil && filepath.Clean(current) != filepath.Clean(root) {
			project = current
		}
	} else if userHome, err := os.UserHomeDir(); err == nil && filepath.Clean(userHome) != filepath.Clean(root) {
		home = userHome
	}
	var warnings []Warning
	for _, entry := range FindRemoteEntries(project, home) {
		if slices.Contains(hubs, entry.HubURL) {
			warnings = append(warnings, Warning{Code: "hub_cache_shared", Summary: HostDisplayName(entry.Host) + " in " + entry.ScopeLabel() + " still uses " + entry.HubURL + "; its agents fetch the skills again after the cache is cleared."})
		}
	}
	return warnings
}

func disconnectCommand(request DisconnectRequest) string {
	parts := []string{"skillhub connect --remove"}
	if request.Global {
		parts = append(parts, "--global")
	} else if request.Project != "" {
		parts = append(parts, "--project", shellquote.Arg(request.Project))
	}
	for _, host := range request.Hosts {
		parts = append(parts, "--host", string(host))
	}
	if request.ClearCache {
		parts = append(parts, "--cache")
	}
	return strings.Join(parts, " ")
}

// readSmallFile reads a host config file for display, refusing anything
// larger than a host file can reasonably be.
func readSmallFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxHostFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxHostFileBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, maxHostFileBytes)
	}
	return data, nil
}

const maxHostFileBytes = 4 << 20
