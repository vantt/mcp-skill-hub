package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/canonical"
	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/userconfig"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// ErrorGitFailed reports a Git command that failed; the rendering carries
// Git's own message.
const ErrorGitFailed ErrorCode = "git_failed"

// InitOptions configures WorkspaceService.Init.
type InitOptions struct {
	// DryRun reports what init would do and writes nothing.
	DryRun bool
	// Source is the Git repository of `skillhub init <git-url>`; zero for a
	// hub created from nothing.
	Source InitSource
	// Store records the hub path; nil uses userconfig.Default().
	Store userconfig.Store
}

// InitReport describes what init did, for the summary the CLI prints. It is
// carried in Result.Details[InitDetailsKey].
type InitReport struct {
	Hub string `json:"hub"`
	// Created is false when the folder already was a hub with commits.
	Created bool `json:"created"`
	// Commit is the first commit init made, if it made one.
	Commit string `json:"commit,omitempty"`
	// FallbackIdentity is true when the first commit used Skill Hub's own
	// author because Git has no user name or email configured.
	FallbackIdentity bool `json:"fallback_identity,omitempty"`
	// Stored is true when the hub path was recorded in the user configuration.
	Stored bool `json:"stored"`
	// StoreError explains why the hub path could not be recorded.
	StoreError string `json:"store_error,omitempty"`
	// GitignoreAdded lists required .gitignore entries init appended to an
	// existing hub; they are not committed.
	GitignoreAdded []string `json:"gitignore_added,omitempty"`
	// TrackedMachineFiles are connection files Git tracks in this hub.
	TrackedMachineFiles []string    `json:"tracked_machine_files,omitempty"`
	Remote              *InitRemote `json:"remote,omitempty"`
	// Next is the one command to run next.
	Next string `json:"next"`
}

// InitRemote describes what `init <git-url>` did with the remote repository.
type InitRemote struct {
	URL    string `json:"url"`
	Branch string `json:"branch,omitempty"`
	// Fetched is true when the hub was taken from the remote.
	Fetched bool `json:"fetched,omitempty"`
	// Pushed is the commit pushed to the remote, if any.
	Pushed string `json:"pushed,omitempty"`
	// OriginAdded is true when init set origin to the URL.
	OriginAdded bool `json:"origin_added,omitempty"`
	// PushError is Git's message when the push failed; the local hub stays.
	PushError string `json:"push_error,omitempty"`
}

// InitDetailsKey is the Result.Details key of the InitReport.
const InitDetailsKey = "init"

// firstCommitMessage is the message of the commit init makes.
const firstCommitMessage = "Initialize Skill Hub hub"

// fallbackName and fallbackEmail are the author of the first commit when Git
// has no identity configured; they apply to that one commit only.
const (
	fallbackName  = "Skill Hub"
	fallbackEmail = "skillhub@localhost"
)

type initKind int

const (
	// initEmpty: no entries, or only a .git with an unborn HEAD.
	initEmpty initKind = iota
	// initResume: only init-owned entries and an unborn HEAD.
	initResume
	// initHubUnborn: a hub marker but no commit yet.
	initHubUnborn
	// initHubCommitted: a hub with commits.
	initHubCommitted
)

type initTarget struct {
	kind initKind
	// hasGit is true when root itself held a .git before this run.
	hasGit bool
	// missing is true when root does not exist yet.
	missing bool
}

// Init sets up the hub in the folder path. It acts on that folder only: an
// empty folder or empty Git repository becomes a committed hub, an
// interrupted first run is finished, an existing hub is refreshed, and
// anything else is refused with the command to run instead. With
// opts.Source it starts from a Git repository. The hub path is recorded in
// the user configuration so `skillhub connect` finds it.
func (service WorkspaceService) Init(path string, opts InitOptions) (Result, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return Result{}, fmt.Errorf("make the hub path absolute: %w", err)
	}
	root = filepath.Clean(root)
	if _, err := exec.LookPath("git"); err != nil {
		return ErrorResult(NewInvalidRequestError("Git is not installed or not on PATH; a hub is a Git repository.", "Install Git, then run `skillhub init` again.")), nil
	}
	store := opts.Store
	if store == nil {
		store = userconfig.Default()
	}
	target, refusal := inspectInitTarget(root)
	if refusal != nil {
		return refusal.result(), nil
	}
	if opts.Source.IsSet() {
		if opts.DryRun {
			return ErrorResult(NewInvalidRequestError(
				"--dry-run cannot be combined with a Git URL: finding out whether the repository holds a hub needs a fetch.",
				"Run `skillhub init "+shellquote.Arg(opts.Source.Display)+"` without --dry-run; it refuses a repository that is not a hub and leaves nothing behind.",
			)), nil
		}
		return service.initFromRemote(context.Background(), root, target, opts.Source, store)
	}
	if opts.DryRun {
		return service.initDryRun(root, target, store)
	}
	return service.initLocal(context.Background(), root, target, store)
}

// initLocal runs the init steps for a folder inspectInitTarget accepted.
func (service WorkspaceService) initLocal(ctx context.Context, root string, target initTarget, store userconfig.Store) (Result, error) {
	report := &InitReport{Hub: root, Created: target.kind != initHubCommitted, Next: shellquote.Chain("cd <your-project>", "skillhub connect")}
	// apply creates the folder and runs `git init -b main` when they are
	// missing, through the same layout step doctor --fix uses.
	if target.kind == initResume {
		// An init interrupted between the layout and the hub marker leaves an
		// empty .skillhub, which would otherwise read as a legacy layout.
		_ = os.Remove(filepath.Join(root, ".skillhub"))
	}
	if target.kind == initHubCommitted || target.kind == initHubUnborn {
		plan, err := workspace.Inspect(root)
		if err != nil {
			return Result{}, err
		}
		issues, err := canonical.Validate(root)
		if err != nil {
			return Result{}, err
		}
		// Validation repeats the setup findings; init fixes those or names them,
		// so only content errors stop it here.
		var contentErrors []canonical.Issue
		for _, issue := range issues {
			if !containsFinding(plan, issue) {
				contentErrors = append(contentErrors, issue)
			}
		}
		if len(contentErrors) > 0 {
			result, err := doctorResult(root, plan, issues, nil)
			if err == nil {
				result.Summary = "The hub at " + root + " has content errors; it was not stored as this machine's hub."
			}
			return result, err
		}
		for _, finding := range plan.Findings {
			if finding.ID == "machine_file_tracked" {
				report.TrackedMachineFiles = append(report.TrackedMachineFiles, finding.Path)
			}
		}
		if target.kind == initHubCommitted {
			missing, err := workspace.MissingIgnoreEntries(root)
			if err != nil {
				return Result{}, err
			}
			report.GitignoreAdded = missing
		}
	}
	result, err := service.apply(root, true)
	if err != nil || result.Error != nil || (result.Status != StatusApplied && result.Status != StatusReady) {
		return result, err
	}
	if target.kind != initHubCommitted {
		commit, fallback, gitErr := commitFirst(ctx, root)
		if gitErr != nil {
			return ErrorResult(&Error{Code: ErrorGitFailed, Render: ErrorRender{
				Error: "The first commit of the hub failed.",
				Why:   gitErr.Error(),
				Fix:   "Fix what Git reports, then run `skillhub init` here again. Everything written so far stays; init finishes the commit.",
			}}), nil
		}
		report.Commit, report.FallbackIdentity = commit, fallback
	}
	if err := store.SetHub(root); err != nil {
		report.StoreError = err.Error()
		report.Next = shellquote.Chain("cd <your-project>", "skillhub connect "+shellquote.Arg(root))
	} else {
		report.Stored = true
	}
	if report.Created {
		result.Summary = "Hub created at " + root + "."
	} else {
		result.Summary = "Hub already set up at " + root + "."
	}
	result.Status = StatusApplied
	if result.Details == nil {
		result.Details = map[string]any{}
	}
	result.Details[InitDetailsKey] = report
	result.SuggestedActions = []Action{{Label: "Connect a project", Command: report.Next}}
	return result, nil
}

// initDryRun reports what init would do without writing anything.
func (service WorkspaceService) initDryRun(root string, target initTarget, store userconfig.Store) (Result, error) {
	var result Result
	if target.missing {
		plan, err := workspace.Inspect(root)
		if err != nil {
			return Result{}, err
		}
		result, err = doctorResult(root, plan, nil, nil)
		if err != nil {
			return Result{}, err
		}
	} else {
		var err error
		result, err = service.doctorInternal(root, true)
		if err != nil {
			return Result{}, err
		}
	}
	result.Status = StatusActionRequired
	result.Summary = "Dry run: the hub was not set up."
	result.SuggestedActions = []Action{{Label: "Set up the hub", Command: "skillhub init", RequiresConfirmation: true}}
	var items []Item
	if target.kind != initHubCommitted {
		items = append(items,
			Item{ID: "workspace_structure", Summary: "Hub folders (skills, sources, distill, history, config, registry, evals)", Impact: "Would be created."},
			Item{ID: "git_repository", Summary: "Git repository (.git)", Impact: "Would be initialized."},
		)
	}
	items = append(items, Item{ID: "catalog_generation", Summary: "Search index", Impact: "Would be built."})
	if target.kind != initHubCommitted {
		items = append(items, Item{ID: "first_commit", Summary: "First commit of the files init writes", Impact: "Would be committed."})
	}
	if path, ok := store.(interface{ Path() string }); ok {
		items = append(items, Item{ID: "stored_hub", Summary: "Hub path recorded in " + path.Path(), Impact: "Would be written."})
	}
	result.Items = append(items, result.Items...)
	return result, nil
}

// initRefusal is a folder init will not touch, with the command to run instead.
type initRefusal struct {
	what, why, fix, next string
}

func (refusal *initRefusal) result() Result {
	result := ErrorResult(&Error{Code: ErrorInvalidRequest, Render: ErrorRender{Error: refusal.what, Why: refusal.why, Fix: refusal.fix}})
	if refusal.next != "" {
		result.SuggestedActions = []Action{{Label: "Run instead", Command: refusal.next}}
	}
	return result
}

// inspectInitTarget decides which init case applies to root, or why init
// refuses it. It only reads.
func inspectInitTarget(root string) (initTarget, *initRefusal) {
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		if repo := enclosingRepository(nearestExisting(root)); repo != "" {
			return initTarget{}, insideRepositoryRefusal(root, repo)
		}
		return initTarget{kind: initEmpty, missing: true}, nil
	}
	if err != nil {
		return initTarget{}, &initRefusal{what: "The folder " + root + " cannot be read.", why: plainError(err) + ".", fix: "Check its permissions, then run `skillhub init` again."}
	}
	if !info.IsDir() {
		return initTarget{}, &initRefusal{what: root + " is not a folder.", why: "A hub is a folder.", fix: "Create an empty folder and run init there:", next: NewHubCommand()}
	}
	repo := enclosingRepository(root)
	hasGit := repo != "" && sameDirectory(repo, root)
	if IsHubRoot(root) {
		if repo != "" && !hasGit {
			return initTarget{}, insideRepositoryRefusal(root, repo)
		}
		if hasGit && hasCommits(root) {
			return initTarget{kind: initHubCommitted, hasGit: true}, nil
		}
		return initTarget{kind: initHubUnborn, hasGit: hasGit}, nil
	}
	home, _ := os.UserHomeDir()
	if hub := DetectRole(root, home, userconfig.Config{}).Hub; hub != nil {
		return initTarget{}, &initRefusal{
			what: "This folder is inside the hub at " + hub.Root + ".",
			why:  "init sets up a hub in the folder where you run it, and a hub cannot hold another hub.",
			fix:  "To see the state of that hub, run:",
			next: shellquote.Chain("cd "+shellquote.Arg(hub.Root), "skillhub status"),
		}
	}
	if repo != "" && !hasGit {
		return initTarget{}, insideRepositoryRefusal(root, repo)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return initTarget{}, &initRefusal{what: "The folder " + root + " cannot be read.", why: plainError(err) + ".", fix: "Check its permissions, then run `skillhub init` again."}
	}
	onlyOwned, onlyGit := true, true
	for _, entry := range entries {
		if entry.Name() != ".git" {
			onlyGit = false
		}
		if !workspace.IsInitOwned(entry.Name()) {
			onlyOwned = false
		}
	}
	switch {
	case len(entries) == 0:
		return initTarget{kind: initEmpty}, nil
	case hasGit && !hasCommits(root) && onlyGit:
		return initTarget{kind: initEmpty, hasGit: true}, nil
	case hasGit && !hasCommits(root) && onlyOwned:
		return initTarget{kind: initResume, hasGit: true}, nil
	}
	refusal := &initRefusal{
		what: "This folder already holds other files, so init will not turn it into a hub.",
		why:  "A hub is a folder of its own, kept in its own Git repository; nothing was written here.",
		fix:  "Create an empty folder for the hub and run init there:",
		next: NewHubCommand(),
	}
	if hasConnectionFile(root) {
		refusal.fix = "To use a hub from this project, run `skillhub connect`. To create a hub, use an empty folder:"
	}
	return initTarget{}, refusal
}

func insideRepositoryRefusal(root, repo string) *initRefusal {
	dir := siblingFolder(repo)
	return &initRefusal{
		what: "A hub must be its own Git repository, but this folder is inside " + repo + ".",
		why:  "Its files would end up in that repository's history; nothing was written.",
		fix:  "Create the hub in a folder outside " + repo + ":",
		next: shellquote.Chain(shellquote.MakeDir(shellquote.Arg(dir)), "cd "+shellquote.Arg(dir), "skillhub init"),
	}
}

// siblingFolder suggests a new folder next to repo, never inside it.
func siblingFolder(repo string) string {
	parent := filepath.Dir(repo)
	candidate := filepath.Join(parent, "skill-hub")
	for index := 2; index < 100; index++ {
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			break
		}
		candidate = filepath.Join(parent, fmt.Sprintf("skill-hub-%d", index))
	}
	return candidate
}

// enclosingRepository returns the top level of the Git work tree that holds
// dir, or "" when there is none. It is written as the ancestor of dir that the
// user knows (Git resolves links, for example macOS /var to /private/var).
func enclosingRepository(dir string) string {
	output, err := offlineGitCommand(context.Background(), dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	top := filepath.Clean(strings.TrimSpace(string(output)))
	for current := filepath.Clean(dir); ; current = filepath.Dir(current) {
		if sameDirectory(current, top) {
			return current
		}
		if filepath.Dir(current) == current {
			return top
		}
	}
}

// hasCommits reports whether HEAD names a commit.
func hasCommits(root string) bool {
	return offlineGitCommand(context.Background(), root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}").Run() == nil
}

// hasConnectionFile reports whether root holds a skillhub entry of any host.
func hasConnectionFile(root string) bool {
	for _, adapter := range hostintegration.SupportedAdapters() {
		if _, ok := readClientEntry(adapter.Host, root, adapter.ConfigRelativePath, false); ok {
			return true
		}
	}
	return false
}

// commitCandidates lists the files of a hub's first commit: every untracked
// or staged file under an init-owned top-level name that .gitignore does not
// exclude.
func commitCandidates(ctx context.Context, root string) ([]string, error) {
	seen := map[string]bool{}
	for _, args := range [][]string{
		{"ls-files", "-z", "--others", "--exclude-standard"},
		{"ls-files", "-z", "--cached"},
	} {
		var stderr bytes.Buffer
		command := offlineGitCommand(ctx, root, args...)
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			return nil, gitError(err, stderr.String())
		}
		for _, path := range strings.Split(string(output), "\x00") {
			if path == "" {
				continue
			}
			top := strings.SplitN(path, "/", 2)[0]
			if top != ".git" && workspace.IsInitOwned(top) {
				seen[path] = true
			}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

// commitFirst makes the hub's first commit of commitCandidates. Hooks and
// signing are disabled for this commit, so a hook or signing setup in a
// pre-existing .git or the user's Git config cannot stop or run code during
// init. Without a configured identity it commits as Skill Hub.
func commitFirst(ctx context.Context, root string) (commit string, fallback bool, err error) {
	paths, err := commitCandidates(ctx, root)
	if err != nil {
		return "", false, err
	}
	if len(paths) == 0 {
		return "", false, errors.New("init found no files to commit")
	}
	hooks, err := os.MkdirTemp("", "skillhub-no-hooks-")
	if err != nil {
		return "", false, fmt.Errorf("create an empty hooks folder: %w", err)
	}
	defer os.RemoveAll(hooks)
	pathspec := strings.Join(paths, "\x00") + "\x00"
	if err := runGitWithInput(ctx, root, pathspec, "add", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
		return "", false, err
	}
	args := []string{"-c", "core.hooksPath=" + hooks, "-c", "commit.gpgsign=false"}
	if gitConfigValue(ctx, root, "user.name") == "" || gitConfigValue(ctx, root, "user.email") == "" {
		args = append(args, "-c", "user.name="+fallbackName, "-c", "user.email="+fallbackEmail)
		fallback = true
	}
	args = append(args, "commit", "--no-verify", "--quiet", "-m", firstCommitMessage, "--pathspec-from-file=-", "--pathspec-file-nul")
	if err := runGitWithInput(ctx, root, pathspec, args...); err != nil {
		return "", false, err
	}
	output, err := offlineGitCommand(ctx, root, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false, fmt.Errorf("read the new commit: %w", err)
	}
	return strings.TrimSpace(string(output)), fallback, nil
}

func runGitWithInput(ctx context.Context, root, input string, args ...string) error {
	var stderr bytes.Buffer
	command := offlineGitCommand(ctx, root, args...)
	command.Stdin = strings.NewReader(input)
	command.Stderr = &stderr
	command.Stdout = &stderr
	if err := command.Run(); err != nil {
		return gitError(err, stderr.String())
	}
	return nil
}

// gitConfigValue returns the effective value of a Git config key, or "".
func gitConfigValue(ctx context.Context, root, key string) string {
	output, err := offlineGitCommand(ctx, root, "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// gitError keeps Git's own message: it is what the user needs to fix the cause.
func gitError(err error, stderr string) error {
	message := strings.TrimSpace(redactUserinfo(stderr))
	if message == "" {
		return fmt.Errorf("git: %w", err)
	}
	return errors.New(message)
}
