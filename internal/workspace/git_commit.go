package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// Commit statuses reported in CommitOutcome.Status.
const (
	CommitCommitted = "committed"
	CommitSkipped   = "skipped"
	CommitFailed    = "failed"
	CommitDeferred  = "deferred"
)

// commitTimeout bounds a commit nobody can answer a prompt for: a signing
// passphrase or a slow hook without a terminal.
var commitTimeout = 60 * time.Second

// CommitPolicy says whether a confirmed change becomes a Git commit.
type CommitPolicy struct {
	// Disabled keeps today's behaviour: files are written, nothing is committed.
	Disabled bool
	// Message replaces the caller's default commit message.
	Message string
	// Interactive means a person is at a terminal, so signing may prompt.
	Interactive bool
	// Defer makes a chain collect paths and commit once at its end.
	Defer bool
}

// CommitOutcome is reported on every mutation result.
type CommitOutcome struct {
	Status string `json:"status"`
	Hash   string `json:"hash,omitempty"`
	// Message is the subject line of the commit.
	Message string `json:"message,omitempty"`
	// Reason says in plain words why the commit was skipped or failed.
	Reason string `json:"reason,omitempty"`
	// ManualCommand is the exact command that finishes the commit by hand.
	ManualCommand string `json:"manual_command,omitempty"`
	Reach         Reach  `json:"reach"`
}

// Reach says who gets a committed version and when.
type Reach struct {
	// ThisHubNow is true: agents that use this hub read the working tree.
	ThisHubNow bool `json:"this_hub_now"`
	// PushNeeded is true when other clones only get the change after a push.
	PushNeeded bool   `json:"push_needed"`
	Remote     string `json:"remote,omitempty"`
	// PushCommand is the command that publishes the commit to Remote.
	PushCommand string `json:"push_command,omitempty"`
}

type commitPolicyKey struct{}

// WithCommitPolicy returns a context that carries the commit policy of the
// confirm surface down to the mutation layer.
func WithCommitPolicy(ctx context.Context, policy CommitPolicy) context.Context {
	return context.WithValue(ctx, commitPolicyKey{}, policy)
}

// CommitPolicyFrom returns the policy a confirm surface set. The second result
// is false when none did: an internal caller that never asked for a commit
// only writes files, so a fresh context can never commit by accident.
func CommitPolicyFrom(ctx context.Context) (CommitPolicy, bool) {
	if ctx == nil {
		return CommitPolicy{}, false
	}
	policy, ok := ctx.Value(commitPolicyKey{}).(CommitPolicy)
	return policy, ok
}

// DirtyPaths returns every path under the given workspace-relative paths that
// has a staged, unstaged or untracked change. It returns nil when root is not
// the top level of a Git repository.
func DirtyPaths(ctx context.Context, root string, paths []string) ([]string, error) {
	if len(paths) == 0 || !ownGitRepository(ctx, root) {
		return nil, nil
	}
	args := append([]string{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--"}, paths...)
	output, err := gitCommand(ctx, root, nil, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("inspect Git status: %w", err)
	}
	var dirty []string
	records := bytes.Split(output, []byte{0})
	for index := 0; index < len(records); index++ {
		record := string(records[index])
		if len(record) < 4 {
			continue
		}
		dirty = append(dirty, record[3:])
		if record[0] == 'R' || record[0] == 'C' {
			index++ // the source path of a rename follows as its own record
		}
	}
	sort.Strings(dirty)
	return dirty, nil
}

// CommitPaths commits exactly the given paths with message. Everything else in
// the index and the working tree is left as it was. modes says, for the paths
// whose mode the change decided, whether each is executable; Git records that
// mode (100755 or 100644) whatever the file system can store. A path absent
// from modes keeps the mode Git already had for it. Hooks and signing run as
// the person configured them. It never fails the caller: the outcome says what
// happened and how to finish by hand.
func CommitPaths(ctx context.Context, root string, paths []string, modes map[string]bool, message string, interactive bool) CommitOutcome {
	outcome := CommitOutcome{Message: firstLine(message)}
	if len(paths) == 0 {
		outcome.Status, outcome.Reason = CommitSkipped, "There was nothing to commit."
		return outcome
	}
	if _, err := exec.LookPath("git"); err != nil {
		outcome.Status, outcome.Reason = CommitSkipped, "Git is not installed, so nothing was committed."
		return outcome
	}
	if !ownGitRepository(ctx, root) {
		outcome.Status, outcome.Reason = CommitSkipped, "The hub is not a Git repository, so nothing was committed."
		return outcome
	}
	outcome.Reach = reachOf(ctx, root)
	outcome.ManualCommand = manualCommand(root, paths)
	paths = existingOrTracked(ctx, root, paths)
	if len(paths) == 0 {
		outcome.Status, outcome.Reason = CommitSkipped, "There was nothing to commit."
		return outcome
	}
	if !interactive {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, commitTimeout)
		defer cancel()
	}
	fail := func(reason string) CommitOutcome {
		outcome.Status, outcome.Reason = CommitFailed, reason
		return outcome
	}
	const staysStaged = " The files are staged: run the manual command to finish, or `git reset` to unstage them."
	if unmerged, stderr, err := runGit(ctx, root, false, "ls-files", "-u", "-z"); err != nil {
		return fail(gitReason(ctx, stderr, err))
	} else if unmerged != "" {
		return fail("The hub's Git index has unresolved merge conflicts, so nothing was committed. Resolve them, then run the manual command.")
	}
	before, stderr, err := indexModes(ctx, root, paths)
	if err != nil {
		return fail(gitReason(ctx, stderr, err))
	}
	if _, stderr, err := runGit(ctx, root, interactive, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return fail(gitReason(ctx, stderr, err))
	}
	adjusted, reason, ok := recordModes(ctx, root, paths, modes, before)
	if !ok {
		return fail(reason + staysStaged)
	}
	// Only the paths that differ from HEAD go into the commit, so a path that
	// was added and removed again since HEAD never reaches the scratch index.
	diff, stderr, err := runGit(ctx, root, false, append([]string{"diff", "--cached", "--name-only", "--no-renames", "-z", "--"}, paths...)...)
	if err != nil {
		return fail(gitReason(ctx, stderr, err) + staysStaged)
	}
	changed := strings.FieldsFunc(diff, func(r rune) bool { return r == 0 })
	if len(changed) == 0 {
		outcome.Status, outcome.Reason = CommitSkipped, "The files already match the last commit."
		return outcome
	}
	// The commit is built in an index of its own that holds HEAD plus the
	// staged entries of these paths, which is what `git commit -- <paths>`
	// does itself. Git's own version re-reads the paths from the file system,
	// and with core.filemode=false it would record a new script as 100644.
	index, err := scratchIndex(ctx, root)
	if err != nil {
		return fail("A scratch index for the commit could not be made: " + err.Error() + staysStaged)
	}
	keepIndex := false
	defer func() {
		if !keepIndex {
			_ = os.Remove(index)
		}
	}()
	env := []string{"GIT_INDEX_FILE=" + index}
	if reason, ok := fillScratchIndex(ctx, root, env, changed); !ok {
		return fail(reason + staysStaged)
	}
	if _, stderr, err := runGitEnv(ctx, root, interactive, env, nil, "commit", "-q", "-m", message); err != nil {
		if adjusted {
			// Only the scratch index holds the commit with the modes the
			// change decided, so the manual command commits from it.
			keepIndex = true
			outcome.ManualCommand = manualCommandFor(runtime.GOOS, root, paths, index)
		}
		return fail(gitReason(ctx, stderr, err) + staysStaged)
	}
	head, _, err := runGit(ctx, root, false, "rev-parse", "HEAD")
	if err != nil {
		return fail("The commit was made but its hash could not be read.")
	}
	outcome.Status, outcome.Hash, outcome.ManualCommand = CommitCommitted, strings.TrimSpace(head), ""
	return outcome
}

// HeadCommit returns the commit the hub's Git repository is at, or "" when root
// is not the top level of its own repository or has no commit yet.
func HeadCommit(ctx context.Context, root string) string {
	if !ownGitRepository(ctx, root) {
		return ""
	}
	output, err := gitCommand(ctx, root, nil, "rev-parse", "--verify", "-q", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// ShortHash is the abbreviated commit hash used in one-line reports.
func ShortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func manualCommand(root string, paths []string) string {
	return manualCommandFor(runtime.GOOS, root, paths, "")
}

// manualCommandFor is the command that finishes a failed commit by hand in the
// shell of goos. With index, the commit is made from that scratch index, which
// already holds exactly the paths with their modes: `git commit -- <paths>`
// would read the files again and, where Git ignores file modes, record a new
// script without its executable bit. The scratch index is removed once the
// commit is made.
func manualCommandFor(goos, root string, paths []string, index string) string {
	repo := shellquote.ArgFor(goos, root)
	if index == "" {
		quoted := make([]string, len(paths))
		for position, path := range paths {
			quoted[position] = shellquote.ArgFor(goos, path)
		}
		list := strings.Join(quoted, " ")
		return fmt.Sprintf("git -C %s add -- %s && git -C %s commit -- %s", repo, list, repo, list)
	}
	file := shellquote.ArgFor(goos, index)
	if goos == "windows" {
		value := file
		if !strings.HasPrefix(value, "'") {
			value = "'" + value + "'" // a plain path has no quote to escape
		}
		return fmt.Sprintf("$env:GIT_INDEX_FILE = %s; git -C %s commit; $committed = $?; Remove-Item Env:GIT_INDEX_FILE; if ($committed) { Remove-Item -LiteralPath %s }", value, repo, file)
	}
	return fmt.Sprintf("GIT_INDEX_FILE=%s git -C %s commit && rm -f %s", file, repo, file)
}

func firstLine(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	return strings.TrimSpace(line)
}

// ownGitRepository reports whether root is itself the top level of a work tree,
// so a hub nested in another project never commits into that project.
func ownGitRepository(ctx context.Context, root string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	output, err := gitCommand(ctx, root, nil, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return false
	}
	top, topErr := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	here, hereErr := filepath.EvalSymlinks(root)
	return topErr == nil && hereErr == nil && top == here
}

func reachOf(ctx context.Context, root string) Reach {
	reach := Reach{ThisHubNow: true}
	output, err := gitCommand(ctx, root, nil, "remote").Output()
	if err != nil {
		return reach
	}
	if remotes := strings.Fields(string(output)); len(remotes) > 0 {
		reach.PushNeeded, reach.Remote = true, remotes[0]
		reach.PushCommand = "git -C " + shellquote.Arg(root) + " push"
	}
	return reach
}

// existingOrTracked drops the paths Git could not match: a deleted path only
// stays when Git tracks it.
func existingOrTracked(ctx context.Context, root string, paths []string) []string {
	tracked := map[string]bool{}
	if output, err := gitCommand(ctx, root, nil, "ls-files", "-z").Output(); err == nil {
		for _, name := range strings.Split(string(output), "\x00") {
			tracked[name] = true
		}
	}
	kept := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path))); err == nil || tracked[path] {
			kept = append(kept, path)
		}
	}
	return kept
}

// scratchIndex makes a copy of the repository's index under its Git dir, so
// the commit can be built without touching the real index and the stat data
// of every tracked file is kept rather than hashed again.
func scratchIndex(ctx context.Context, root string) (string, error) {
	gitPath := func(name string) (string, error) {
		output, stderr, err := runGit(ctx, root, false, "rev-parse", "--git-path", name)
		if err != nil {
			return "", errors.New(gitReason(ctx, stderr, err))
		}
		path := filepath.FromSlash(strings.TrimSpace(output))
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		return path, nil
	}
	current, err := gitPath("index")
	if err != nil {
		return "", err
	}
	scratch, err := gitPath("skillhub-commit-index")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(current)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(scratch), filepath.Base(scratch)+"-*")
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

// fillScratchIndex turns the scratch index env names into HEAD plus the real
// index's entries for changed, so the commit holds exactly what is staged.
func fillScratchIndex(ctx context.Context, root string, env, changed []string) (string, bool) {
	base := []string{"read-tree", "--empty"}
	if _, _, err := runGit(ctx, root, false, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		// --reset keeps the stat data of entries that match HEAD, as -m does,
		// but does not refuse an entry whose file was edited after staging.
		base = []string{"read-tree", "--reset", "HEAD"}
	}
	if _, stderr, err := runGitEnv(ctx, root, false, env, nil, base...); err != nil {
		return gitReason(ctx, stderr, err), false
	}
	entries, stderr, err := runGit(ctx, root, false, append([]string{"ls-files", "-s", "-z", "--"}, changed...)...)
	if err != nil {
		return gitReason(ctx, stderr, err), false
	}
	present := map[string]bool{}
	for _, record := range strings.Split(entries, "\x00") {
		if _, path, ok := strings.Cut(record, "\t"); ok {
			present[path] = true
		}
	}
	if entries != "" {
		if _, stderr, err := runGitEnv(ctx, root, false, env, strings.NewReader(entries), "update-index", "-z", "--index-info"); err != nil {
			return gitReason(ctx, stderr, err), false
		}
	}
	var removed []string
	for _, path := range changed {
		if !present[path] {
			removed = append(removed, path)
		}
	}
	if len(removed) > 0 {
		if _, stderr, err := runGitEnv(ctx, root, false, env, nil, append([]string{"update-index", "--force-remove", "--"}, removed...)...); err != nil {
			return gitReason(ctx, stderr, err), false
		}
	}
	return "", true
}

// Index modes of a regular file.
const (
	modeExecutable = "100755"
	modePlain      = "100644"
)

// indexModes returns the mode the index holds for each regular file under
// paths.
func indexModes(ctx context.Context, root string, paths []string) (map[string]string, string, error) {
	output, stderr, err := runGit(ctx, root, false, append([]string{"ls-files", "-s", "-z", "--"}, paths...)...)
	if err != nil {
		return nil, stderr, err
	}
	modes := map[string]string{}
	for _, record := range strings.Split(output, "\x00") {
		info, path, found := strings.Cut(record, "\t")
		mode, _, _ := strings.Cut(info, " ")
		if found && (mode == modeExecutable || mode == modePlain) {
			modes[path] = mode
		}
	}
	return modes, "", nil
}

// recordModes sets the index mode of every staged regular file under paths:
// the mode the change decided when modes names the file, otherwise the mode
// the index had before staging. `git add` takes the mode from the file system
// where Git trusts it and a Windows file system has no executable bit, so the
// intended mode is set explicitly. It reports whether any mode was changed.
func recordModes(ctx context.Context, root string, paths []string, modes map[string]bool, before map[string]string) (bool, string, bool) {
	after, stderr, err := indexModes(ctx, root, paths)
	if err != nil {
		return false, gitReason(ctx, stderr, err), false
	}
	var plus, minus []string
	for path, mode := range after {
		executable, decided := modes[path]
		if !decided {
			previous, known := before[path]
			if !known {
				continue
			}
			executable = previous == modeExecutable
		}
		switch {
		case executable && mode != modeExecutable:
			plus = append(plus, path)
		case !executable && mode == modeExecutable:
			minus = append(minus, path)
		}
	}
	sort.Strings(plus)
	sort.Strings(minus)
	for _, step := range []struct {
		flag  string
		paths []string
	}{{"--chmod=+x", plus}, {"--chmod=-x", minus}} {
		if len(step.paths) == 0 {
			continue
		}
		if _, stderr, err := runGit(ctx, root, false, append([]string{"update-index", step.flag, "--"}, step.paths...)...); err != nil {
			return false, gitReason(ctx, stderr, err), false
		}
	}
	return len(plus)+len(minus) > 0, "", true
}

func gitCommand(ctx context.Context, root string, stdin io.Reader, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat", "GIT_LITERAL_PATHSPECS=1")
	command.Stdin = stdin
	command.WaitDelay = 2 * time.Second
	return command
}

// runGit runs one Git step. Without a terminal it has no stdin and cannot ask
// for a password or a passphrase.
func runGit(ctx context.Context, root string, interactive bool, args ...string) (string, string, error) {
	return runGitEnv(ctx, root, interactive, nil, nil, args...)
}

// runGitEnv is runGit with extra environment, such as GIT_INDEX_FILE, and
// input for a step that reads it; an interactive step reads the terminal.
func runGitEnv(ctx context.Context, root string, interactive bool, env []string, input io.Reader, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	stdin := input
	if interactive && stdin == nil {
		stdin = os.Stdin
	}
	command := gitCommand(ctx, root, stdin, args...)
	command.Env = append(command.Env, env...)
	if !interactive {
		command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_ASKPASS=", "SSH_ASKPASS=")
	}
	command.Stdout, command.Stderr = &stdout, &stderr
	if interactive {
		// A hook or a signing tool that asks a question must be heard.
		command.Stdout, command.Stderr = io.MultiWriter(&stdout, os.Stderr), io.MultiWriter(&stderr, os.Stderr)
	}
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func gitReason(ctx context.Context, stderr string, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("The commit did not finish within %d seconds. A hook or your signing key may be waiting for input, and there is no terminal.", int(commitTimeout.Seconds()))
	}
	if strings.Contains(stderr, "identity unknown") || strings.Contains(stderr, "auto-detect email") || strings.Contains(stderr, "empty ident") {
		return "Git has no user name or email set. Set them with `" + SetGitNameCommand + "` and `" + SetGitEmailCommand + "`."
	}
	lines := make([]string, 0, 3)
	for _, line := range strings.Split(stderr, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
		if len(lines) == 3 {
			break
		}
	}
	if len(lines) == 0 {
		return "Git failed: " + err.Error()
	}
	return strings.Join(lines, " ")
}
