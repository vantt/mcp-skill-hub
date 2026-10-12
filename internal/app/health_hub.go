package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	catalogpkg "github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// hubState is what one inspection of a hub found. The caller holds the
// workspace lock. Doctor's report (Doctor) and the hub checks (hubCheck) both
// derive from it, so the hub is inspected by one piece of code.
type hubState struct {
	plan       workspace.Plan
	issues     []canonical.Issue
	recoveries []mutation.Recovery
	// catalog is nil when it was not inspected: content problems, pending
	// recovery or a finding that needs a person come first.
	catalog *catalogpkg.Status
	// host is nil unless the catalog is healthy.
	host *hubHostState
}

type hubHostState struct {
	request    hostintegration.Request
	inspection hostintegration.Inspection
	conflicts  []string
	// plan is empty when the files have conflicts.
	plan hostintegration.PlanResult
}

// inspectHubWhileLocked inspects the hub; hosts limits the check of its own
// agent connection (nil: every host; empty: none).
func inspectHubWhileLocked(ctx context.Context, root string, hosts []hostintegration.Host) (hubState, error) {
	var state hubState
	var err error
	if state.plan, err = workspace.Inspect(root); err != nil {
		return hubState{}, err
	}
	if state.issues, err = canonical.Validate(root); err != nil {
		return hubState{}, err
	}
	if state.recoveries, err = mutation.InspectRecoveryWhileLocked(root); err != nil {
		return hubState{}, err
	}
	// The search index is judged only when the layout is in order. Missing
	// .gitignore entries and tracked machine files are Git notices; they do not
	// stop the index and connection checks.
	if len(contentIssues(state)) > 0 || len(state.recoveries) > 0 || slices.ContainsFunc(state.plan.Findings, func(finding workspace.Finding) bool {
		return finding.ID != "machine_file_tracked" && finding.ID != "gitignore_entry_missing"
	}) {
		return state, nil
	}
	status, err := catalogpkg.InspectWhileLocked(ctx, root)
	if err != nil {
		return hubState{}, err
	}
	state.catalog = &status
	if status.State != catalogpkg.StateHealthy || (hosts != nil && len(hosts) == 0) {
		return state, nil
	}
	request, err := hostIntegrationRequest(root)
	if err != nil {
		return hubState{}, err
	}
	request.Hosts = hosts
	inspection, err := hostintegration.Inspect(ctx, request)
	if err != nil {
		return hubState{}, err
	}
	host := &hubHostState{request: request, inspection: inspection}
	for _, inspected := range inspection.Hosts {
		for _, file := range inspected.Files {
			if file.Conflict != "" {
				host.conflicts = append(host.conflicts, file.Path+": "+file.Conflict)
			}
		}
	}
	if len(host.conflicts) == 0 {
		if host.plan, err = hostintegration.Plan(ctx, request); err != nil {
			return hubState{}, err
		}
	}
	state.host = host
	return state, nil
}

// contentIssues are the validation issues that are not the same problem as
// a workspace finding (validation repeats layout and .gitignore findings).
func contentIssues(state hubState) []canonical.Issue {
	var issues []canonical.Issue
	for _, issue := range state.issues {
		if !containsFinding(state.plan, issue) {
			issues = append(issues, issue)
		}
	}
	return issues
}

// hubCheck is the built-in hub provider: the Git repository, the layout and
// schema, the content, pending recovery, the search index, the hub's own
// agent connection and, for doctor, the pre-commit hook and the local
// listener.
type hubCheck struct{}

func (hubCheck) Name() string { return "hub" }

func (hubCheck) Applies(report RoleReport) bool { return report.HealthHub() != "" }

func (hubCheck) Check(ctx context.Context, report RoleReport, depth Depth) []Finding {
	root := report.HealthHub()
	var findings []Finding
	git, gitErr := inspectGit(ctx, root)
	switch {
	case gitErr != nil:
		findings = append(findings, Finding{Code: "hub_git_missing", Label: "Git repository", Severity: SeverityProblem,
			Message: "Git cannot read the hub folder: " + plainError(gitErr) + ".", Reason: "Check that Git is installed and can read " + root + "."})
	case !git.Configured:
		findings = append(findings, Finding{Code: "hub_git_missing", Label: "Git repository", Severity: SeverityProblem,
			Message: "The hub folder is not a Git repository, so its skills have no history.",
			Manual:  shellquote.Chain("cd "+shellquote.Arg(root), "skillhub init"),
			Reason:  "skillhub init makes the hub folder a Git repository with a first commit."})
	}

	var lock *mutation.WorkspaceLock
	var err error
	if depth == DepthLight {
		lock, err = mutation.TryAcquireSharedLock(root)
	} else {
		lock, err = mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	}
	if err != nil {
		if errors.Is(err, mutation.ErrWorkspaceBusy) {
			return append(findings, busyFinding(report, depth))
		}
		return append(findings, Finding{Code: "hub_unreadable", Label: "Hub folder", Severity: SeverityProblem,
			Message: "The hub cannot be checked: " + plainError(err) + "."})
	}
	keep, elsewhere := hubFolderHosts(root)
	state, err := inspectHubWhileLocked(ctx, root, keep)
	_ = lock.Unlock()
	if err != nil {
		return append(findings, Finding{Code: "hub_unreadable", Label: "Hub folder", Severity: SeverityProblem,
			Message: "The hub cannot be checked: " + plainError(err) + "."})
	}
	findings = append(findings, hubStateFindings(root, report, state, depth, keep)...)
	findings = append(findings, elsewhere...)

	if git.Configured && gitErr == nil {
		if len(git.Files) > 0 {
			findings = append(findings, Finding{Code: "hub_uncommitted", Label: "Git", Severity: SeverityNotice,
				Message: termCount(len(git.Files), "changed file is", "changed files are") + " not committed to Git yet.",
				Manual:  commitCommand(root, git.Files)})
		} else {
			findings = append(findings, Finding{Code: "hub_uncommitted", Label: "Git", Severity: SeverityOK, Message: "clean"})
		}
	}
	if depth == DepthFull {
		if git.Configured && gitErr == nil {
			findings = append(findings, precommitFinding(ctx, root))
		}
		findings = append(findings, listenerFinding(ctx))
	}
	return findings
}

func busyFinding(report RoleReport, depth Depth) Finding {
	if depth == DepthLight {
		return Finding{Code: "hub_busy", Label: "Hub lock", Severity: SeverityNotice,
			Message: "The hub is busy with another change; run `skillhub status` again in a moment.", Manual: report.hubCommand("skillhub status")}
	}
	return Finding{Code: "hub_busy", Label: "Hub lock", Severity: SeverityProblem,
		Message: fmt.Sprintf("The hub stayed busy with another change for %s, so it was not checked.", mutation.DefaultLockTimeout),
		Manual:  report.hubCommand("skillhub doctor"), Reason: "Wait until the other change finishes, then run doctor again."}
}

// hubStateFindings turns an inspection into findings. The fixable ones share
// one hub repair, the same repair `doctor --fix` has always run.
func hubStateFindings(root string, report RoleReport, state hubState, depth Depth, hosts []hostintegration.Host) []Finding {
	var findings []Finding
	repair := &FixAction{}
	var repairs []string
	addRepair := func(phrase string) {
		if !slices.Contains(repairs, phrase) {
			repairs = append(repairs, phrase)
		}
	}
	blocked := ""
	var missingIgnore []string
	layoutFixable := false
	for _, finding := range state.plan.Findings {
		switch {
		case finding.ID == "machine_file_tracked":
			findings = append(findings, Finding{Code: finding.ID, Label: "Tracked machine file", Severity: SeverityProblem,
				Message: "Git tracks " + finding.Path + ", which holds paths of this machine.",
				Manual:  "git -C " + shellquote.Arg(root) + " rm --cached -- " + shellquote.Arg(finding.Path),
				Reason:  "doctor --fix never changes what Git tracks; stop tracking the file with the command below (it stays on disk)."})
		case finding.ID == "canonical_schema_incompatible":
			blocked = "the schema migration"
			findings = append(findings, Finding{Code: "hub_schema", Label: "Schema version", Severity: SeverityProblem,
				Message: finding.Summary, Manual: report.hubCommand("skillhub maintain migrate"),
				Reason: "A schema change needs a migration that you preview and confirm."})
		case !finding.Fixable:
			if blocked == "" {
				blocked = "the problem with " + finding.Path
			}
			findings = append(findings, Finding{Code: finding.ID, Label: "Hub layout", Severity: SeverityProblem,
				Message: finding.Path + ": " + finding.Summary, Reason: "Move or remove " + finding.Path + " by hand; doctor --fix does not change it."})
		case finding.ID == "gitignore_entry_missing":
			missingIgnore = append(missingIgnore, strings.TrimPrefix(finding.Summary, "Required Git ignore entry is missing: "))
		default:
			layoutFixable = true
			findings = append(findings, Finding{Code: finding.ID, Label: "Hub layout", Severity: SeverityProblem, Message: finding.Path + ": " + finding.Summary, Fix: repair})
		}
	}
	if layoutFixable {
		addRepair("create the missing hub folders and files")
	}
	if blocked != "the schema migration" {
		findings = append(findings, Finding{Code: "hub_schema", Label: "Schema version", Severity: SeverityOK, Message: "version " + workspace.SchemaVersion})
	}
	if len(missingIgnore) > 0 {
		findings = append(findings, Finding{Code: "gitignore_entry_missing", Label: "Git ignore entries", Severity: SeverityProblem,
			Message: ".gitignore lacks " + strings.Join(missingIgnore, " ") + ", so Git could pick up files that hold paths of this machine.", Fix: repair})
		addRepair("add the missing .gitignore entries")
	} else {
		findings = append(findings, Finding{Code: "gitignore_entry_missing", Label: "Git ignore entries", Severity: SeverityOK, Message: "present"})
	}

	issues := contentIssues(state)
	contentReason := "doctor --fix does not change skill content; correct the file by hand or with `skillhub skill edit <id>`."
	switch {
	case len(issues) > 0 && depth == DepthLight:
		findings = append(findings, Finding{Code: "hub_content_invalid", Label: "Content", Severity: SeverityProblem,
			Message: termCount(len(issues), "content problem", "content problems") + "; the first: " + issues[0].Path + ": " + issues[0].Message,
			Manual:  report.hubCommand("skillhub maintain validate"), Reason: contentReason})
	case len(issues) > 0:
		for _, issue := range issues {
			findings = append(findings, Finding{Code: "hub_content_invalid", Label: "Content", Severity: SeverityProblem,
				Message: issue.Path + ": " + issue.Message, Manual: report.hubCommand("skillhub maintain validate"), Reason: contentReason})
		}
	default:
		findings = append(findings, Finding{Code: "hub_content_invalid", Label: "Content", Severity: SeverityOK, Message: "valid"})
	}

	for _, recovery := range state.recoveries {
		finding := Finding{Code: "hub_recovery_pending", Label: "Interrupted change", Severity: SeverityProblem,
			Message: "The change " + recovery.OperationID + " was interrupted (" + string(recovery.Action) + ").", Fix: repair}
		if recovery.Action == mutation.RecoveryConflict {
			finding.Fix = nil
			finding.Reason = "Automatic recovery is blocked: " + recovery.Detail
			blocked = "the blocked recovery"
		}
		findings = append(findings, finding)
	}
	if len(state.recoveries) > 0 {
		addRepair("finish the interrupted change")
	} else {
		findings = append(findings, Finding{Code: "hub_recovery_pending", Label: "Interrupted change", Severity: SeverityOK, Message: "none"})
	}

	notChecked := "not checked: the problems above come first"
	switch {
	case state.catalog == nil:
		findings = append(findings, Finding{Code: "hub_index_stale", Label: "Search index", Severity: SeveritySkipped, Message: notChecked})
	case state.catalog.State != catalogpkg.StateHealthy:
		findings = append(findings, Finding{Code: "hub_index_stale", Label: "Search index", Severity: SeverityProblem,
			Message: "The search index is " + string(state.catalog.State) + ": " + state.catalog.Detail, Fix: repair,
			Manual: report.hubCommand("skillhub maintain rebuild")})
		addRepair("rebuild the search index")
	default:
		findings = append(findings, Finding{Code: "hub_index_stale", Label: "Search index", Severity: SeverityOK, Message: "current"})
	}

	findings = append(findings, selfConnectionFindings(root, state, hosts, repair, addRepair)...)

	repair.Summary = "repair the hub: " + strings.Join(repairs, ", ")
	repair.Apply = func(context.Context) (Changed, error) {
		changed, notes, err := repairHub(root, hosts)
		repair.Notes = notes
		return changed, err
	}
	if blocked != "" {
		for index := range findings {
			if findings[index].Fix == repair {
				findings[index].Fix = nil
				findings[index].Reason = "doctor --fix repairs this after " + blocked + "."
			}
		}
	}
	return findings
}

// selfConnectionFindings checks the hub's own agent connection files and the
// Claude Code permission rules, registering what doctor --fix would repair.
func selfConnectionFindings(root string, state hubState, hosts []hostintegration.Host, repair *FixAction, addRepair func(string)) []Finding {
	var out []Finding
	switch host := state.host; {
	case host == nil && hosts != nil && len(hosts) == 0 && state.catalog != nil:
		out = append(out, Finding{Code: "hub_self_connection", Label: "Agent connection", Severity: SeverityOK, Message: "every agent of the hub folder uses another hub"})
	case host == nil:
		out = append(out, Finding{Code: "hub_self_connection", Label: "Agent connection", Severity: SeveritySkipped, Message: "not checked: the search index must be current first"})
	case len(host.conflicts) > 0:
		out = append(out, Finding{Code: "hub_self_connection", Label: "Agent connection", Severity: SeverityProblem,
			Message: "The hub's agent connection files cannot be updated safely: " + strings.Join(host.conflicts, "; "),
			Reason:  "Fix or remove the conflicting managed content in those files; doctor --fix never overwrites it."})
	default:
		var connection, permissions []string
		var connectionChanges, permissionChanges []hostintegration.Change
		for _, change := range host.plan.Changes {
			described := describeChange(root, change)
			if change.Kind == hostintegration.ChangeHostPermissions {
				permissions = append(permissions, described)
				permissionChanges = append(permissionChanges, change)
			} else {
				connection = append(connection, described)
				connectionChanges = append(connectionChanges, change)
			}
		}
		if len(connection) > 0 {
			message := "The hub's agent connection files are out of date: " + strings.Join(connection, "; ") + "."
			if missing := hostintegration.MissingFinding(host.plan); missing != nil {
				message += " Some are missing (" + missing.Note + ")."
			}
			out = append(out, Finding{Code: "hub_self_connection", Label: "Agent connection", Severity: SeverityProblem, Message: message, Fix: repair, Detail: changePreviews(connectionChanges)})
			addRepair("refresh the hub's agent connection files")
		} else {
			out = append(out, Finding{Code: "hub_self_connection", Label: "Agent connection", Severity: SeverityOK, Message: "current"})
		}
		if len(permissions) > 0 {
			out = append(out, Finding{Code: "client_permissions_outdated", Label: "Claude permission rules", Severity: SeverityProblem,
				Message: "The Claude Code permission rules are not the current set: " + strings.Join(permissions, "; ") + ".", Fix: repair, Detail: changePreviews(permissionChanges)})
			addRepair("update the Claude Code permission rules (your own rules stay)")
		}
		if len(host.plan.Blocked) > 0 {
			out = append(out, blockedSkillFinding(root, host.plan.Blocked))
		}
	}
	return out
}

// hubFolderHosts returns the hosts whose own connection the hub checks and
// repairs, and a notice for each host whose entry in the hub folder points at
// another existing hub or a remote hub: a person connected it there on purpose (`skillhub connect
// <other>` in the hub folder), so doctor --fix leaves it alone. keep is nil
// when every host belongs to the hub.
func hubFolderHosts(root string) (keep []hostintegration.Host, elsewhere []Finding) {
	var all []hostintegration.Host
	for _, adapter := range hostintegration.SupportedAdapters() {
		all = append(all, adapter.Host)
		entry, ok := readClientEntry(adapter.Host, root, adapter.ConfigRelativePath, false)
		// An entry that names a folder which is no hub (the hub moved, or
		// the entry is damaged) is this hub's own connection gone stale.
		if !ok || (!entry.Remote() && (sameDirectory(entry.Workspace, root) || !IsHubRoot(entry.Workspace))) {
			keep = append(keep, adapter.Host)
			continue
		}
		target := entry.HubURL
		if target == "" {
			target = entry.Workspace
		}
		elsewhere = append(elsewhere, Finding{Code: "hub_connection_elsewhere", Label: HostDisplayName(adapter.Host), Severity: SeverityNotice,
			Message: HostDisplayName(adapter.Host) + " in the hub folder uses the hub at " + target + ", so doctor leaves its connection alone.",
			Manual:  "skillhub connect --host " + hostFlag(adapter.Host)})
	}
	if len(keep) == len(all) {
		return nil, nil
	}
	if keep == nil {
		keep = []hostintegration.Host{}
	}
	return keep, elsewhere
}

// repairHub runs the hub repair doctor --fix has always run and lists the
// files it wrote and the notes about system skill files.
func repairHub(root string, hosts []hostintegration.Host) (Changed, []string, error) {
	result, err := (WorkspaceService{hosts: hosts}).DoctorFix(root, false)
	if err != nil {
		return nil, nil, err
	}
	if result.Error != nil {
		return nil, nil, errors.New(strings.TrimSpace(result.Error.Render.Error + " " + result.Error.Render.Why))
	}
	if result.Status != StatusApplied && result.Status != StatusReady && result.Status != StatusOK {
		return nil, nil, errors.New(result.Summary)
	}
	return changedPaths(root, result), systemSkillNotes(result), nil
}

// systemSkillNotes returns the warnings of a repair result about system skill
// files: an old copy removed, a shared folder kept. A file left in place is
// not repeated here; its own finding lists it as not fixed.
func systemSkillNotes(result Result) []string {
	var notes []string
	for _, warning := range result.Warnings {
		switch warning.Code {
		case hostintegration.NoticeGeminiLegacy, hostintegration.NoticeSharedSkillKept:
			if !slices.Contains(notes, warning.Summary) {
				notes = append(notes, warning.Summary)
			}
		}
	}
	return notes
}

// changedPaths lists the files a hub repair result reports as written. The
// item formats are the ones remediationResult and hostChangeItem produce.
func changedPaths(root string, result Result) Changed {
	changed := Changed{}
	add := func(path string) {
		if path == "" || path == "." {
			return
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		if !slices.Contains(changed, path) {
			changed = append(changed, path)
		}
	}
	for _, item := range result.Items {
		switch {
		case item.Impact == "Published immutable catalog generation.":
			add(filepath.Join(root, "runtime", "catalog"))
		case item.Impact != "Remediated." && item.Impact != "Written.":
		case strings.HasPrefix(item.ID, "host_"):
			if index := strings.LastIndex(item.Summary, ": "); index >= 0 {
				add(item.Summary[index+2:])
			}
		default:
			if path, _, found := strings.Cut(item.Summary, ": "); found {
				add(path)
			}
		}
	}
	return changed
}

// commitCommand is the Git line that commits the changed hub files: the
// changed top-level folders and files by name, never everything.
func commitCommand(root string, files []DiffFile) string {
	var paths []string
	for _, file := range files {
		path := file.Path
		if top, _, nested := strings.Cut(path, "/"); nested && top != ".skillhub" {
			path = top
		}
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	quoted := make([]string, 0, len(paths))
	for _, path := range paths {
		quoted = append(quoted, shellquote.Arg(path))
	}
	git := "git -C " + shellquote.Arg(root)
	return shellquote.Chain(git+" add -- "+strings.Join(quoted, " "), git+" commit -m "+shellquote.Arg("Update skills"))
}

// oldValidateCall matches the top-level validate command that pre-commit
// hooks ran before it moved under maintain.
var oldValidateCall = regexp.MustCompile(`(\bskillhub(?:\.exe)?)(\s+)validate\b`)

func precommitFinding(ctx context.Context, root string) Finding {
	finding := Finding{Code: "hub_precommit_validate", Label: "Pre-commit hook"}
	path, err := precommitHookPath(ctx, root)
	if err != nil {
		finding.Severity, finding.Message = SeveritySkipped, "not checked: "+plainError(err)
		return finding
	}
	content, err := readSmallFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		finding.Severity, finding.Message = SeverityOK, "no pre-commit hook"
		return finding
	case err != nil:
		finding.Severity, finding.Message = SeveritySkipped, "not checked: "+plainError(err)
		return finding
	case !oldValidateCall.Match(content):
		finding.Severity, finding.Message = SeverityOK, "does not run the removed `skillhub validate`"
		return finding
	}
	finding.Severity = SeverityProblem
	finding.Message = path + " runs `skillhub validate`, which is now `skillhub maintain validate`; the hook fails every commit."
	finding.Fix = &FixAction{
		Summary: "change `skillhub validate` to `skillhub maintain validate` in " + path + " (the rest of the hook stays as it is)",
		Apply:   func(context.Context) (Changed, error) { return rewritePrecommitHook(path) },
	}
	return finding
}

func precommitHookPath(ctx context.Context, root string) (string, error) {
	output, err := offlineGitCommand(ctx, root, "rev-parse", "--git-path", "hooks/pre-commit").Output()
	if err != nil {
		return "", fmt.Errorf("git cannot name the hooks folder: %w", err)
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", errors.New("git named no hooks folder")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	return filepath.Clean(path), nil
}

// rewritePrecommitHook replaces only the old validate call and keeps every
// other byte and the file mode.
func rewritePrecommitHook(path string) (Changed, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	content, err := readSmallFile(path)
	if err != nil {
		return nil, err
	}
	updated := oldValidateCall.ReplaceAll(content, []byte("${1}${2}maintain validate"))
	if string(updated) == string(content) {
		return Changed{}, nil
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pre-commit-*")
	if err != nil {
		return nil, err
	}
	name := temporary.Name()
	_, writeErr := temporary.Write(updated)
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, closeErr, os.Chmod(name, info.Mode().Perm())); err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	return Changed{path}, nil
}

// listenerAddress is where `skillhub serve` listens on this machine by default.
const listenerAddress = "127.0.0.1:7420"

func listenerFinding(ctx context.Context) Finding {
	probeContext, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	probe := ProbeHub(probeContext, probeClientFrom(ctx), "http://"+listenerAddress, "")
	if probe.Reachable() {
		return Finding{Code: "hub_listener", Label: "Network server", Severity: SeverityNotice,
			Message: "A Skill Hub server (skillhub serve) answers on " + listenerAddress + "; agents on other machines can use a hub through it."}
	}
	return Finding{Code: "hub_listener", Label: "Network server", Severity: SeverityOK,
		Message: "nothing listens on " + listenerAddress + "; run `skillhub serve` when agents on another machine need this hub"}
}

// termCount writes a count with the singular or plural phrase.
func termCount(count int, one, many string) string {
	return fmt.Sprintf("%d %s", count, pluralWord(count, one, many))
}
