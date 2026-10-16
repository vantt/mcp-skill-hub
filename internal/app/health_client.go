package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// localGroup is the hosts of one scope whose entries point at one hub folder
// on this machine.
type localGroup struct {
	global bool
	// root is the project folder, or the home folder for the user scope.
	root string
	// hub is the hub folder the entries name; empty when the entry names none.
	hub string
	// path is a config file of the group, named when hub is empty.
	path  string
	hosts []hostintegration.Host
}

func localGroups(report RoleReport) []localGroup {
	var groups []localGroup
	for _, global := range []bool{false, true} {
		for _, host := range report.Hosts {
			entry := host.Project
			if global {
				entry = host.User
			}
			if entry == nil || entry.Remote() || entry.Self {
				continue
			}
			index := slices.IndexFunc(groups, func(group localGroup) bool {
				return group.global == global && group.root == entry.Root && group.hub == entry.Workspace
			})
			if index < 0 {
				groups = append(groups, localGroup{global: global, root: entry.Root, hub: entry.Workspace, path: entry.Path})
				index = len(groups) - 1
			}
			groups[index].hosts = append(groups[index].hosts, host.Host)
		}
	}
	return groups
}

// clientLocalCheck is the built-in provider for the connections of this
// project and of the user account to hubs on this machine: the hub folder
// still exists, the entries are current, and the Claude permission rules are
// the current set. Its fixes re-run connect's safe merge.
type clientLocalCheck struct{}

func (clientLocalCheck) Name() string { return "client-local" }

func (clientLocalCheck) Applies(report RoleReport) bool { return len(localGroups(report)) > 0 }

func (check clientLocalCheck) Check(ctx context.Context, report RoleReport, depth Depth) []Finding {
	return sectionFindings(check.sections(ctx, report, depth))
}

func (clientLocalCheck) sections(ctx context.Context, report RoleReport, _ Depth) []Section {
	var sections []Section
	for _, group := range localGroups(report) {
		hubName := group.hub
		if hubName == "" {
			hubName = "the folder named in " + group.path
		}
		title := scopeTitle(report, group.global, group.root, "uses") + " the hub at " + hubName
		if group.hub != "" && sameDirectory(group.hub, report.HealthHub()) {
			// The hub section above already names this hub.
			title = scopeTitle(report, group.global, group.root, "uses") + " this hub"
		}
		section := Section{Kind: SectionClient, Root: group.root, Title: title}
		section.Findings = append(section.Findings, hostScopeFindings(report, group.global, group.hosts)...)
		section.Findings = append(section.Findings, localGroupFindings(ctx, report, group)...)
		sections = append(sections, section)
	}
	return sections
}

// scopeTitle starts a client section header: "This project", "This project
// (<root>)" when the entries are in a folder above the current one, or "Your
// user account".
func scopeTitle(report RoleReport, global bool, root, verb string) string {
	if global {
		return "Your user account " + verb
	}
	if root != "" && !sameDirectory(root, report.Cwd) {
		return "This project (" + root + ") " + verb
	}
	return "This project " + verb
}

// hostScopeFindings names, per host, which entry the host uses, so a folder
// with project and user entries says which one wins.
func hostScopeFindings(report RoleReport, global bool, hosts []hostintegration.Host) []Finding {
	var findings []Finding
	for _, host := range hosts {
		index := slices.IndexFunc(report.Hosts, func(role HostRole) bool { return role.Host == host })
		if index < 0 {
			continue
		}
		role := report.Hosts[index]
		message := "project entry in "
		entry := role.Project
		if global {
			message, entry = "user entry in ", role.User
		}
		if entry == nil {
			continue
		}
		message += displayConfigPath(*entry)
		inHub := role.Project != nil && report.Hub != nil && sameDirectory(role.Project.Root, report.Hub.Root)
		switch {
		case global && inHub:
			message += "; serves your other projects, while the hub folder uses its own connection (as expected)"
		case global && role.Project != nil:
			message += "; serves your other projects, while this project uses its own entry"
		case !global && inHub && role.User != nil:
			message += "; the hub folder's own connection, used here instead of your user account's entry (as expected)"
		case !global && role.User != nil:
			message += "; it wins over the entry of your user account"
		}
		findings = append(findings, Finding{Code: "client_host", Label: HostDisplayName(host), Severity: SeverityOK, Message: message})
	}
	return findings
}

// displayConfigPath names the config file of an entry the same way on every
// platform: relative to the project, or under ~ for the user scope.
func displayConfigPath(entry ClientEntry) string {
	relative, err := filepath.Rel(entry.Root, entry.Path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return entry.Path
	}
	if entry.Global {
		return "~/" + filepath.ToSlash(relative)
	}
	return filepath.ToSlash(relative)
}

func localGroupFindings(ctx context.Context, report RoleReport, group localGroup) []Finding {
	if group.hub == "" || !IsHubRoot(group.hub) {
		return []Finding{missingHubFinding(report, group)}
	}
	binary, err := currentBinary()
	if err != nil {
		return []Finding{{Code: "client_entry_outdated", Label: "Connection files", Severity: SeveritySkipped, Message: "not checked: " + plainError(err)}}
	}
	scope := hostintegration.ScopeProject
	if group.global {
		scope = hostintegration.ScopeUser
	}
	request := hostintegration.Request{Workspace: group.hub, Root: group.root, Scope: scope, Binary: binary, Hosts: group.hosts}
	inspection, err := hostintegration.Inspect(ctx, request)
	if err != nil {
		return []Finding{{Code: "client_entry_outdated", Label: "Connection files", Severity: SeveritySkipped, Message: "not checked: " + plainError(err)}}
	}
	command := localConnectCommand(group, group.hub)
	var conflicts []string
	for _, host := range inspection.Hosts {
		for _, file := range host.Files {
			if file.Conflict != "" {
				conflicts = append(conflicts, file.Path+": "+file.Conflict)
			}
		}
	}
	if len(conflicts) > 0 {
		return []Finding{{Code: "client_entry_outdated", Label: "Connection files", Severity: SeverityProblem,
			Message: "The connection files cannot be updated safely: " + strings.Join(conflicts, "; "), Manual: command,
			Reason: "Fix or remove the conflicting managed content in those files, then run the command below; doctor --fix never overwrites it."}}
	}
	plan, err := hostintegration.Plan(ctx, request)
	if err != nil {
		return []Finding{{Code: "client_entry_outdated", Label: "Connection files", Severity: SeveritySkipped, Message: "not checked: " + plainError(err)}}
	}
	var connection, permissions []string
	var connectionChanges, permissionChanges []hostintegration.Change
	for _, change := range plan.Changes {
		described := describeChange(group.root, change)
		if change.Kind == hostintegration.ChangeHostPermissions {
			permissions = append(permissions, described)
			permissionChanges = append(permissionChanges, change)
		} else {
			connection = append(connection, described)
			connectionChanges = append(connectionChanges, change)
		}
	}
	fix := &FixAction{Summary: "update the connection of " + scopeName(group) + " to the hub at " + group.hub + " (connect's safe merge; your own content stays)"}
	fix.Apply = func(ctx context.Context) (Changed, error) { return reconnectLocal(ctx, fix, report, group, group.hub) }
	reason := ""
	if hubNeedsMigration(group.hub) {
		fix, command = nil, "skillhub maintain migrate --workspace "+shellquote.Arg(group.hub)
		reason = "The hub at " + group.hub + " needs a schema migration first; doctor --fix updates this connection after it."
	}
	var findings []Finding
	if len(connection) > 0 {
		findings = append(findings, Finding{Code: "client_entry_outdated", Label: "Connection files", Severity: SeverityProblem,
			Message: "Out of date: " + strings.Join(connection, "; ") + ".", Fix: fix, Manual: manualIfNoFix(fix, command), Reason: reason, Detail: changePreviews(connectionChanges)})
	} else {
		findings = append(findings, Finding{Code: "client_entry_outdated", Label: "Connection files", Severity: SeverityOK, Message: "current"})
	}
	if len(permissions) > 0 {
		findings = append(findings, Finding{Code: "client_permissions_outdated", Label: "Claude permission rules", Severity: SeverityProblem,
			Message: "The Claude Code permission rules are not the current set: " + strings.Join(permissions, "; ") + ".", Fix: fix, Manual: manualIfNoFix(fix, command), Reason: reason, Detail: changePreviews(permissionChanges)})
	}
	if len(plan.Blocked) > 0 {
		findings = append(findings, blockedSkillFinding(group.root, plan.Blocked))
	}
	return findings
}

func manualIfNoFix(fix *FixAction, command string) string {
	if fix != nil {
		return ""
	}
	return command
}

// missingHubFinding reports an entry whose hub folder is gone. doctor --fix
// re-points the entries only to the hub this machine stores, and only when
// the missing folder is one this machine recorded as a previous hub, so a
// cloned project can never be pointed at an unrelated folder.
func missingHubFinding(report RoleReport, group localGroup) Finding {
	finding := Finding{Code: "client_hub_missing", Label: "Hub folder", Severity: SeverityProblem}
	if group.hub == "" {
		finding.Message = "The entry in " + group.path + " names no hub folder."
	} else {
		finding.Message = "The hub folder " + group.hub + " is gone."
	}
	finding.Reason = MovedHubFix(report, group.hub)
	recorded := group.hub != "" && slices.ContainsFunc(report.PreviousHubs, func(previous string) bool {
		return sameDirectory(previous, group.hub) || filepath.Clean(previous) == group.hub
	})
	storedExists := IsHubRoot(report.StoredHub)
	switch {
	case recorded && storedExists && !sameDirectory(report.StoredHub, group.hub):
		stored := filepath.Clean(report.StoredHub)
		fix := &FixAction{Summary: "point " + scopeName(group) + " at the hub at " + stored + " (was " + group.hub + ")"}
		fix.Apply = func(ctx context.Context) (Changed, error) { return reconnectLocal(ctx, fix, report, group, stored) }
		finding.Fix = fix
	case report.StoredHub != "" && (!storedExists || sameDirectory(report.StoredHub, group.hub)):
		finding.Manual = shellquote.Chain("cd <new-location>", "skillhub init")
	case storedExists:
		finding.Manual = localConnectCommand(group, report.StoredHub)
	default:
		finding.Manual = NewHubCommand()
	}
	return finding
}

func scopeName(group localGroup) string {
	if group.global {
		return "your user account"
	}
	return "the project " + group.root
}

// localConnectCommand is the connect command that writes the group's entries
// for hub, run from the project folder.
func localConnectCommand(group localGroup, hub string) string {
	command := "skillhub connect " + shellquote.Arg(hub)
	if group.global {
		command += " -g"
	}
	for _, host := range group.hosts {
		command += " --host " + hostFlag(host)
	}
	return command
}

// hostFlag is the --host value of a host.
func hostFlag(host hostintegration.Host) string {
	switch host {
	case hostintegration.HostClaude:
		return "claude"
	case hostintegration.HostCodex:
		return "codex"
	case hostintegration.HostGemini:
		return "gemini"
	}
	return string(host)
}

// reconnectLocal re-runs connect's safe merge for the group's hosts: it never
// overwrites a person's own content and refuses on a conflicting managed
// block.
func reconnectLocal(ctx context.Context, fix *FixAction, report RoleReport, group localGroup, hub string) (Changed, error) {
	service := ConnectService{HomeDir: func() (string, error) {
		if report.Home == "" {
			return "", errors.New("the home folder is unknown")
		}
		return report.Home, nil
	}}
	result, err := service.Connect(ctx, ConnectRequest{Project: group.root, Workspace: hub, Global: group.global, Hosts: group.hosts})
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, errors.New(strings.TrimSpace(result.Error.Render.Why + " " + result.Error.Render.Fix))
	}
	fix.Notes = systemSkillNotes(result)
	return changedPaths(group.root, result), nil
}

// hubNeedsMigration reports whether the hub's schema marker names another
// schema version than this build uses.
func hubNeedsMigration(hub string) bool {
	content, err := os.ReadFile(filepath.Join(hub, ".skillhub", "schema-version"))
	return err == nil && strings.TrimSpace(string(content)) != workspace.SchemaVersion
}

// clientRemoteCheck is the built-in provider for the connections to hubs on
// other machines: token file, reachable, token accepted and, for doctor,
// version, skill cache and last fetch. It never fixes anything: reconnecting
// sends the token, so only the person runs connect. The token goes only to a
// hub the user scope names or a hub on this machine (tokenMaySend).
type clientRemoteCheck struct{}

func (clientRemoteCheck) Name() string { return "client-remote" }

func (clientRemoteCheck) Applies(report RoleReport) bool { return len(FindRemoteEntries(report)) > 0 }

func (check clientRemoteCheck) Check(ctx context.Context, report RoleReport, depth Depth) []Finding {
	return sectionFindings(check.sections(ctx, report, depth))
}

var remoteCodes = map[string]string{
	"token":          "remote_token_file",
	"hub_reachable":  "remote_reachable",
	"token_accepted": "remote_token_accepted",
	"hub_version":    "remote_version",
	"cache_writable": "remote_cache",
	"last_fetch":     "remote_last_fetch",
}

func (clientRemoteCheck) sections(ctx context.Context, report RoleReport, depth Depth) []Section {
	entries := FindRemoteEntries(report)
	service := RemoteDoctorService{Client: probeClientFrom(ctx)}
	var sections []Section
	for _, hubReport := range service.checkAt(ctx, entries, depth) {
		var project, user []RemoteEntry
		for _, entry := range entries {
			if entry.HubURL != hubReport.HubURL || entry.TokenFile != hubReport.TokenFile {
				continue
			}
			if entry.Global {
				user = append(user, entry)
			} else {
				project = append(project, entry)
			}
		}
		section := Section{Kind: SectionClient}
		switch {
		case len(project) > 0 && len(user) > 0:
			section.Root, section.Title = project[0].Root, "This project and your user account use the hub at "+hubReport.HubURL
		case len(project) > 0:
			section.Root, section.Title = project[0].Root, scopeTitle(report, false, project[0].Root, "uses")+" the hub at "+hubReport.HubURL
		default:
			section.Root, section.Title = report.Home, "Your user account uses the hub at "+hubReport.HubURL
		}
		section.Findings = append(section.Findings, Finding{Code: "remote_used_by", Label: "Used by", Severity: SeverityOK, Message: strings.Join(hubReport.UsedBy, ", ")})
		for _, check := range hubReport.Checks {
			section.Findings = append(section.Findings, remoteFinding(hubReport.HubURL, check))
		}
		maySend := tokenMaySend(hubReport.HubURL, entries)
		for _, scoped := range [][]RemoteEntry{project, user} {
			if len(scoped) > 0 {
				section.Findings = append(section.Findings, remoteFilesFinding(ctx, scoped, maySend))
			}
		}
		sections = append(sections, section)
	}
	return sections
}

func remoteFinding(hubURL string, check RemoteCheck) Finding {
	finding := Finding{Code: remoteCodes[check.ID], Label: check.Label, Message: check.Detail, Manual: check.manual}
	if finding.Code == "" {
		finding.Code = "remote_" + check.ID
	}
	switch check.Status {
	case CheckFailed:
		finding.Severity = SeverityProblem
		finding.Reason = check.Fix
		if finding.Code == "remote_reachable" && check.manual != "" && !strings.HasPrefix(check.manual, "skillhub connect") {
			finding.Reason = "Skill Hub cannot start a hub on another machine. " + check.Fix
		}
	case CheckSkipped:
		finding.Severity = SeveritySkipped
	default:
		finding.Severity = SeverityOK
	}
	return finding
}

// remoteFilesFinding checks that the connection files of one scope still
// match what connect writes for the hub. It reads only local files; nothing
// is sent to the hub.
func remoteFilesFinding(ctx context.Context, entries []RemoteEntry, maySend bool) Finding {
	finding := Finding{Code: "remote_reconnect", Label: "Connection files"}
	first := entries[0]
	binary, err := currentBinary()
	if err != nil {
		finding.Severity, finding.Message = SeveritySkipped, "not checked: "+plainError(err)
		return finding
	}
	scope := hostintegration.ScopeProject
	if first.Global {
		scope = hostintegration.ScopeUser
	}
	var hosts []hostintegration.Host
	for _, entry := range entries {
		hosts = append(hosts, entry.Host)
	}
	plan, err := hostintegration.Plan(ctx, hostintegration.Request{Root: first.Root, Scope: scope, Binary: binary, Hosts: hosts, Hub: &hostintegration.RemoteHub{URL: first.HubURL, TokenFile: first.TokenFile}})
	if err != nil {
		finding.Severity, finding.Message = SeveritySkipped, "not checked: "+plainError(err)
		return finding
	}
	if len(plan.Changes) == 0 {
		finding.Severity, finding.Message = SeverityOK, "current"
		return finding
	}
	var changed []string
	for _, change := range plan.Changes {
		changed = append(changed, describeChange(first.Root, change))
	}
	finding.Severity = SeverityProblem
	finding.Message = "Out of date: " + strings.Join(changed, "; ") + "."
	finding.Detail = changePreviews(plan.Changes)
	finding.Manual = reconnectRemoteCommand(first.HubURL, first.TokenFile, first.Global)
	if first.Global || maySend {
		finding.Reason = "doctor --fix does not rewrite a connection to a hub on another machine; run: `" + finding.Manual + "`"
	} else {
		finding.Reason = RemoteFixRefusal(first.HubURL, first.TokenFile)
	}
	return finding
}

// reconnectRemoteCommand is the connect command for a remote hub, with the
// entry's token file or the per-hub default.
func reconnectRemoteCommand(hubURL, tokenFile string, global bool) string {
	file := hubtoken.ClientPath(hubURL)
	if tokenFile != "" {
		file = shellquote.Arg(tokenFile)
	}
	command := "skillhub connect"
	if global {
		command += " -g"
	}
	return command + " --hub " + hubURL + " --token-file " + file
}
