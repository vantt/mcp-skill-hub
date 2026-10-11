package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
	"github.com/vantt/mcp-skill-hub/internal/userconfig"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// probeRefPrefix holds the branches init fetches to look for a hub before
// anything is checked out. They are always deleted again.
const probeRefPrefix = "refs/skillhub-probe/"

// unreachableHint is the one line printed after Git's own error when the
// remote cannot be read.
const unreachableHint = "Check the URL and that you can run Git against it, for example with your SSH key or a credential helper."

// remoteRefs is what `git ls-remote --symref` reported.
type remoteRefs struct {
	// head is the branch the remote HEAD points at, when it says.
	head string
	// branches maps branch names to commit ids.
	branches map[string]string
	// any is true when the remote has any ref at all, tags included.
	any bool
}

// candidates orders the branches to probe for a hub: the HEAD target when it
// exists, then main, master, then the others by name.
func (refs remoteRefs) candidates() []string {
	var order []string
	add := func(name string) {
		if _, ok := refs.branches[name]; ok {
			for _, existing := range order {
				if existing == name {
					return
				}
			}
			order = append(order, name)
		}
	}
	add(refs.head)
	add("main")
	add("master")
	rest := make([]string, 0, len(refs.branches))
	for name := range refs.branches {
		rest = append(rest, name)
	}
	sort.Strings(rest)
	for _, name := range rest {
		add(name)
	}
	return order
}

func parseRemoteRefs(output string) remoteRefs {
	refs := remoteRefs{branches: map[string]string{}}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		refs.any = true
		fields := strings.Split(line, "\t")
		if len(fields) != 2 {
			continue
		}
		if target, ok := strings.CutPrefix(fields[0], "ref: refs/heads/"); ok && fields[1] == "HEAD" {
			refs.head = target
			continue
		}
		if name, ok := strings.CutPrefix(fields[1], "refs/heads/"); ok && !strings.HasPrefix(fields[0], "ref:") {
			refs.branches[name] = fields[0]
		}
	}
	return refs
}

// gitFailure renders a failed remote Git command: Git's message, redacted,
// and one hint line.
func gitFailure(what string, err error, hint string) Result {
	return ErrorResult(&Error{Code: ErrorGitFailed, Render: ErrorRender{Error: what, Why: err.Error(), Fix: hint}})
}

// runRemoteGit runs a remote Git command and returns its standard output.
func runRemoteGit(ctx context.Context, root string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	command := remoteGitCommand(ctx, root, args...)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", gitError(err, stderr.String())
	}
	return stdout.String(), nil
}

// runLocalGit runs an offline Git command and returns its standard output.
func runLocalGit(ctx context.Context, root string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	command := offlineGitCommand(ctx, root, args...)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", gitError(err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// initFromRemote handles `skillhub init <git-url>` for a folder that
// inspectInitTarget accepted.
func (service WorkspaceService) initFromRemote(ctx context.Context, root string, target initTarget, source InitSource, store userconfig.Store) (Result, error) {
	var warnings []Warning
	if target.hasGit {
		if command := gitConfigLocal(ctx, root, "core.sshCommand"); command != "" {
			warnings = append(warnings, Warning{Code: "git_ssh_command", Summary: "This folder's Git config sets core.sshCommand (" + command + "); Git uses it to reach " + source.Display + "."})
		}
	}
	listDir := root
	if target.missing {
		listDir = ""
	}
	output, err := runRemoteGit(ctx, listDir, "ls-remote", "--symref", "--", source.Location)
	if err != nil {
		return withWarnings(gitFailure("Git could not read "+source.Display+".", err, unreachableHint), warnings), nil
	}
	refs := parseRemoteRefs(output)
	var result Result
	switch {
	case target.kind == initHubCommitted || target.kind == initHubUnborn:
		result, err = service.initBindHub(ctx, root, target, source, refs, store)
	case !refs.any:
		result, err = service.initAndPush(ctx, root, target, source, store)
	default:
		result, err = service.initFetchHub(ctx, root, target, source, refs, store)
	}
	return withWarnings(result, warnings), err
}

func withWarnings(result Result, warnings []Warning) Result {
	result.Warnings = append(warnings, result.Warnings...)
	return result
}

// initAndPush creates the hub locally, then pushes its first commit to the
// empty remote. origin is added only after the push succeeded, so a failed
// push is retried by running the same command again.
func (service WorkspaceService) initAndPush(ctx context.Context, root string, target initTarget, source InitSource, store userconfig.Store) (Result, error) {
	if problem := refuseOtherOrigin(ctx, root, target.hasGit, source); problem != nil {
		return problem.result(), nil
	}
	result, err := service.initLocal(ctx, root, target, store)
	if err != nil || result.Error != nil {
		return result, err
	}
	report := initReportOf(result)
	if report == nil {
		return result, nil
	}
	report.Remote = &InitRemote{URL: source.Display}
	pushFirst(ctx, root, source, report.Remote)
	return result, nil
}

// pushFirst pushes the current branch to the empty remote and then sets
// origin and the upstream. A push failure is recorded in remote.PushError.
func pushFirst(ctx context.Context, root string, source InitSource, remote *InitRemote) {
	branch, err := runLocalGit(ctx, root, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		remote.PushError = err.Error()
		return
	}
	remote.Branch = branch
	hooks, err := os.MkdirTemp("", "skillhub-no-hooks-")
	if err != nil {
		remote.PushError = err.Error()
		return
	}
	defer os.RemoveAll(hooks)
	if _, err := runRemoteGit(ctx, root, "-c", "core.hooksPath="+hooks, "push", "--no-verify", "--quiet", "--", source.Location, "HEAD:refs/heads/"+branch); err != nil {
		remote.PushError = err.Error()
		return
	}
	commit, err := runLocalGit(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		remote.PushError = err.Error()
		return
	}
	remote.Pushed = commit
	if err := setOrigin(ctx, root, source, map[string]string{branch: commit}, branch, remote); err != nil {
		remote.PushError = "The push succeeded, but origin could not be set: " + err.Error()
	}
}

// setOrigin adds origin when it is missing, records the remote branches as
// origin's remote-tracking refs (no second network round trip) and sets the
// upstream of branch when the remote has it.
func setOrigin(ctx context.Context, root string, source InitSource, branches map[string]string, branch string, remote *InitRemote) error {
	if gitConfigLocal(ctx, root, "remote.origin.url") == "" {
		if _, err := runLocalGit(ctx, root, "remote", "add", "--", "origin", source.Location); err != nil {
			return err
		}
		remote.OriginAdded = true
	}
	for name, commit := range branches {
		if _, err := runLocalGit(ctx, root, "update-ref", "refs/remotes/origin/"+name, commit); err != nil {
			return err
		}
	}
	if _, ok := branches[branch]; ok && branch != "" {
		if _, err := runLocalGit(ctx, root, "branch", "--quiet", "--set-upstream-to=origin/"+branch, branch); err != nil {
			return err
		}
	}
	return nil
}

// initFetchHub takes the hub from a remote that has refs: it fetches the
// candidate branches into probe refs, picks the first that holds a hub,
// checks what it tracks and only then checks it out.
func (service WorkspaceService) initFetchHub(ctx context.Context, root string, target initTarget, source InitSource, refs remoteRefs, store userconfig.Store) (Result, error) {
	if target.kind == initResume {
		return (&initRefusal{
			what: "This folder holds the files of an unfinished `skillhub init`, and " + source.Display + " is not empty.",
			why:  "init takes a hub from a repository only into an empty folder.",
			fix:  "Take the hub into a new, empty folder:",
			next: NewHubCommand() + " " + shellquote.Arg(source.Display),
		}).result(), nil
	}
	if problem := refuseOtherOrigin(ctx, root, target.hasGit, source); problem != nil {
		return problem.result(), nil
	}
	if target.missing {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return Result{}, fmt.Errorf("create the hub folder: %w", err)
		}
	}
	if !target.hasGit {
		if err := workspace.InitRepository(root); err != nil {
			return Result{}, err
		}
	}
	// Nothing but a .git was here before; remove it again only when Skill Hub
	// created it, now or in an earlier interrupted run.
	cleanup := func() {
		deleteProbeRefs(ctx, root)
		if target.kind == initEmpty && hasCreatedMarker(root) {
			_ = os.RemoveAll(filepath.Join(root, ".git"))
			if target.missing {
				_ = os.Remove(root)
			}
		}
	}
	candidates := refs.candidates()
	if len(candidates) == 0 {
		cleanup()
		return notAHubRefusal(source).result(), nil
	}
	fetch := []string{"fetch", "--quiet", "--no-tags", "--no-write-fetch-head", "--", source.Location}
	for _, branch := range candidates {
		fetch = append(fetch, "+refs/heads/"+branch+":"+probeRefPrefix+branch)
	}
	if _, err := runRemoteGit(ctx, root, fetch...); err != nil {
		cleanup()
		return gitFailure("Git could not fetch "+source.Display+".", err, unreachableHint), nil
	}
	branch := ""
	for _, candidate := range candidates {
		if offlineGitCommand(ctx, root, "cat-file", "-e", probeRefPrefix+candidate+":.skillhub/schema-version").Run() == nil {
			branch = candidate
			break
		}
	}
	if branch == "" {
		cleanup()
		return notAHubRefusal(source).result(), nil
	}
	ref := probeRefPrefix + branch
	trust, err := checkTrackedFiles(ctx, root, ref)
	if err != nil {
		cleanup()
		return Result{}, err
	}
	if len(trust.refused) > 0 {
		cleanup()
		return (&initRefusal{
			what: source.Display + " tracks files that never belong in a hub: " + strings.Join(trust.refused, ", ") + ".",
			why:  "runtime/ and .skillhub/transactions/ hold credentials, settings that agents load into their shell, and programs of the machine that wrote them. Nothing was checked out.",
			fix:  "Ask the owner of the repository to remove them from Git (`git rm -r --cached runtime .skillhub/transactions`) and commit, then run `skillhub init " + shellquote.Arg(source.Display) + "` again.",
		}).result(), nil
	}
	hooks, err := os.MkdirTemp("", "skillhub-no-hooks-")
	if err != nil {
		cleanup()
		return Result{}, fmt.Errorf("create an empty hooks folder: %w", err)
	}
	defer os.RemoveAll(hooks)
	if _, err := runLocalGit(ctx, root, "-c", "core.hooksPath="+hooks, "checkout", "--quiet", "-B", branch, ref); err != nil {
		cleanup()
		return gitFailure("Git could not check out the hub from "+source.Display+".", err, "Fix what Git reports, then run the same command again."), nil
	}
	remote := &InitRemote{URL: source.Display, Branch: branch, Fetched: true}
	branches := map[string]string{}
	for _, candidate := range candidates {
		if commit, err := runLocalGit(ctx, root, "rev-parse", probeRefPrefix+candidate); err == nil {
			branches[candidate] = commit
		}
	}
	originErr := setOrigin(ctx, root, source, branches, branch, remote)
	deleteProbeRefs(ctx, root)
	if originErr != nil {
		return gitFailure("The hub was checked out, but origin could not be set.", originErr, "Run `git remote add origin "+shellquote.Arg(source.Display)+"` in "+root+"."), nil
	}
	result, err := service.initLocal(ctx, root, initTarget{kind: initHubCommitted, hasGit: true}, store)
	if err != nil {
		return result, err
	}
	if result.Error == nil && result.Status != StatusApplied {
		result.Summary = "The hub from " + source.Display + " was checked out at " + root + ", but it has problems; it was not stored as this machine's hub."
	}
	if report := initReportOf(result); report != nil {
		report.Remote = remote
	}
	result.Warnings = append(result.Warnings, trust.warnings...)
	return result, nil
}

// initBindHub connects an existing hub to the remote: an empty remote gets
// the hub's history pushed; a remote that shares the history becomes origin;
// anything else is refused. It then refreshes the hub like plain init.
func (service WorkspaceService) initBindHub(ctx context.Context, root string, target initTarget, source InitSource, refs remoteRefs, store userconfig.Store) (Result, error) {
	origin := gitConfigLocal(ctx, root, "remote.origin.url")
	if origin != "" && !sameRemote(origin, source.Location) {
		return (&initRefusal{
			what: "This hub already uses " + redactUserinfo(origin) + " as origin.",
			why:  "init sets origin only when there is none or it already is " + source.Display + ".",
			fix:  "To change it, run `git remote set-url origin <url>` yourself, then `skillhub init` again.",
		}).result(), nil
	}
	if target.kind == initHubUnborn || !target.hasGit {
		// The hub needs its first commit before it can share history.
		result, err := service.initLocal(ctx, root, target, store)
		if err != nil || result.Error != nil || result.Status != StatusApplied {
			return result, err
		}
		target = initTarget{kind: initHubCommitted, hasGit: true}
		if !refs.any {
			report := initReportOf(result)
			if report != nil {
				report.Remote = &InitRemote{URL: source.Display}
				pushFirst(ctx, root, source, report.Remote)
			}
			return result, nil
		}
	}
	remote := &InitRemote{URL: source.Display}
	if !refs.any {
		pushFirst(ctx, root, source, remote)
		return service.finishBoundHub(ctx, root, remote, store)
	}
	branch, _ := runLocalGit(ctx, root, "symbolic-ref", "--short", "HEAD")
	candidates := refs.candidates()
	fetch := []string{"fetch", "--quiet", "--no-tags", "--no-write-fetch-head", "--", source.Location}
	for _, name := range candidates {
		fetch = append(fetch, "+refs/heads/"+name+":"+probeRefPrefix+name)
	}
	if len(candidates) > 0 {
		if _, err := runRemoteGit(ctx, root, fetch...); err != nil {
			deleteProbeRefs(ctx, root)
			return gitFailure("Git could not fetch "+source.Display+".", err, unreachableHint), nil
		}
	}
	defer deleteProbeRefs(ctx, root)
	branches := map[string]string{}
	shared, containsHead := false, false
	for _, name := range candidates {
		commit, err := runLocalGit(ctx, root, "rev-parse", probeRefPrefix+name)
		if err != nil {
			continue
		}
		branches[name] = commit
		if offlineGitCommand(ctx, root, "merge-base", "HEAD", commit).Run() == nil {
			shared = true
		}
		if offlineGitCommand(ctx, root, "merge-base", "--is-ancestor", "HEAD", commit).Run() == nil {
			containsHead = true
		}
	}
	remote.Branch = branch
	switch {
	case origin == "" && !shared:
		return (&initRefusal{
			what: source.Display + " holds a history that this hub does not share.",
			why:  "init connects a hub only to an empty repository or to one made from this hub; origin was not added.",
			fix:  "Use an empty repository, or the URL this hub was taken from.",
		}).result(), nil
	case origin != "" && !containsHead:
		return (&initRefusal{
			what: source.Display + " does not have this hub's latest commit on " + branch + ".",
			why:  "init pushes only to an empty repository; pushing to one that already has commits is your decision.",
			fix:  "To publish this hub's commits, run:",
			next: "git push -u origin " + shellquote.Arg(branch),
		}).result(), nil
	}
	if err := setOrigin(ctx, root, source, branches, branch, remote); err != nil {
		return gitFailure("origin could not be set.", err, "Run `git remote add origin "+shellquote.Arg(source.Display)+"` in "+root+"."), nil
	}
	return service.finishBoundHub(ctx, root, remote, store)
}

func (service WorkspaceService) finishBoundHub(ctx context.Context, root string, remote *InitRemote, store userconfig.Store) (Result, error) {
	result, err := service.initLocal(ctx, root, initTarget{kind: initHubCommitted, hasGit: true}, store)
	if report := initReportOf(result); report != nil {
		report.Remote = remote
	}
	return result, err
}

func notAHubRefusal(source InitSource) *initRefusal {
	return &initRefusal{
		what: source.Display + " holds a repository that is not a Skill Hub hub.",
		why:  "No branch of it has .skillhub/schema-version; nothing was left behind here.",
		fix:  "Use the URL of an empty repository to create a hub there, or the URL of an existing hub.",
	}
}

// refuseOtherOrigin refuses a pre-existing .git whose origin names another
// repository.
func refuseOtherOrigin(ctx context.Context, root string, hasGit bool, source InitSource) *initRefusal {
	if !hasGit {
		return nil
	}
	origin := gitConfigLocal(ctx, root, "remote.origin.url")
	if origin == "" || sameRemote(origin, source.Location) {
		return nil
	}
	return &initRefusal{
		what: "This folder's Git repository already uses " + redactUserinfo(origin) + " as origin.",
		why:  "init sets origin only when there is none or it already is " + source.Display + ".",
		fix:  "Use an empty folder for the hub:",
		next: NewHubCommand() + " " + shellquote.Arg(source.Display),
	}
}

func sameRemote(a, b string) bool {
	normalize := func(value string) string {
		return strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(value), "/"), ".git")
	}
	return normalize(a) == normalize(b)
}

// gitConfigLocal reads a key from the repository's own config only.
func gitConfigLocal(ctx context.Context, root, key string) string {
	value, err := runLocalGit(ctx, root, "config", "--local", "--get", key)
	if err != nil {
		return ""
	}
	return value
}

func hasCreatedMarker(root string) bool {
	info, err := os.Lstat(filepath.Join(root, ".git", workspace.CreatedMarker))
	return err == nil && info.Mode().IsRegular()
}

func deleteProbeRefs(ctx context.Context, root string) {
	output, err := runLocalGit(ctx, root, "for-each-ref", "--format=%(refname)", probeRefPrefix)
	if err != nil {
		return
	}
	for _, ref := range strings.Split(output, "\n") {
		if ref = strings.TrimSpace(ref); ref != "" {
			_, _ = runLocalGit(ctx, root, "update-ref", "-d", ref)
		}
	}
}

// trackedFileReport is the result of the trust check on a fetched hub.
type trackedFileReport struct {
	// refused are tracked paths that stop init.
	refused []string
	// warnings name tracked agent configuration files.
	warnings []Warning
}

// checkTrackedFiles inspects what the fetched ref tracks before anything is
// checked out. runtime/ and .skillhub/transactions/ refuse the hub; agent
// configuration that agents load is named in a warning, except an instruction
// file whose content is exactly what init writes.
func checkTrackedFiles(ctx context.Context, root, ref string) (trackedFileReport, error) {
	var report trackedFileReport
	output, err := runLocalGit(ctx, root, "ls-tree", "-r", "--name-only", "-z", ref)
	if err != nil {
		return report, fmt.Errorf("list the files of the fetched hub: %w", err)
	}
	fresh := hostintegration.FreshInstructionFile()
	refusedDirs := map[string]bool{}
	var configs []string
	for _, path := range strings.Split(output, "\x00") {
		switch {
		case path == "":
			continue
		case strings.HasPrefix(path, "runtime/"):
			refusedDirs["runtime/"] = true
		case strings.HasPrefix(path, ".skillhub/transactions/"):
			refusedDirs[".skillhub/transactions/"] = true
		case path == ".mcp.json", strings.HasPrefix(path, ".claude/settings") && strings.HasSuffix(path, ".json"),
			strings.HasPrefix(path, ".codex/"),
			strings.HasPrefix(path, ".gemini/") && !strings.HasPrefix(path, ".gemini/skills/"):
			configs = append(configs, path)
		case path == "CLAUDE.md", path == "AGENTS.md", path == "GEMINI.md":
			content, err := runLocalGit(ctx, root, "cat-file", "blob", ref+":"+path)
			if err != nil || strings.TrimSpace(content) != strings.TrimSpace(string(fresh)) {
				configs = append(configs, path)
			}
		}
	}
	for _, dir := range []string{"runtime/", ".skillhub/transactions/"} {
		if refusedDirs[dir] {
			report.refused = append(report.refused, dir)
		}
	}
	if len(configs) > 0 {
		report.warnings = append(report.warnings, Warning{
			Code:    "hub_tracks_agent_config",
			Summary: "The hub tracks agent configuration that your agents load: " + strings.Join(configs, ", ") + ". Read these files before you start an agent in this folder.",
		})
	}
	return report, nil
}

// initReportOf returns the InitReport of an init result, if it has one.
func initReportOf(result Result) *InitReport {
	if result.Details == nil {
		return nil
	}
	report, _ := result.Details[InitDetailsKey].(*InitReport)
	return report
}
