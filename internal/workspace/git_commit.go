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
// the index and the working tree is left as it was. Hooks and signing run as
// the person configured them. It never fails the caller: the outcome says what
// happened and how to finish by hand.
func CommitPaths(ctx context.Context, root string, paths []string, message string, interactive bool) CommitOutcome {
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
	if _, stderr, err := runGit(ctx, root, interactive, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return fail(gitReason(ctx, stderr, err))
	}
	if _, _, err := runGit(ctx, root, false, append([]string{"diff", "--cached", "--quiet", "--"}, paths...)...); err == nil {
		outcome.Status, outcome.Reason = CommitSkipped, "The files already match the last commit."
		return outcome
	}
	if _, stderr, err := runGit(ctx, root, interactive, append([]string{"commit", "-q", "-m", message, "--"}, paths...)...); err != nil {
		return fail(gitReason(ctx, stderr, err))
	}
	head, _, err := runGit(ctx, root, false, "rev-parse", "HEAD")
	if err != nil {
		return fail("The commit was made but its hash could not be read.")
	}
	outcome.Status, outcome.Hash, outcome.ManualCommand = CommitCommitted, strings.TrimSpace(head), ""
	return outcome
}

// ShortHash is the abbreviated commit hash used in one-line reports.
func ShortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func manualCommand(root string, paths []string) string {
	quoted := make([]string, len(paths))
	for index, path := range paths {
		quoted[index] = shellquote.Arg(path)
	}
	list := strings.Join(quoted, " ")
	repo := shellquote.Arg(root)
	return fmt.Sprintf("git -C %s add -- %s && git -C %s commit -- %s", repo, list, repo, list)
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

func gitCommand(ctx context.Context, root string, stdin io.Reader, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat")
	command.Stdin = stdin
	command.WaitDelay = 2 * time.Second
	return command
}

// runGit runs one Git step. Without a terminal it has no stdin and cannot ask
// for a password or a passphrase.
func runGit(ctx context.Context, root string, interactive bool, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	var stdin io.Reader
	if interactive {
		stdin = os.Stdin
	}
	command := gitCommand(ctx, root, stdin, args...)
	if !interactive {
		command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_ASKPASS=", "SSH_ASKPASS=")
	}
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func gitReason(ctx context.Context, stderr string, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("The commit did not finish within %d seconds. A hook or your signing key may be waiting for input, and there is no terminal.", int(commitTimeout.Seconds()))
	}
	if strings.Contains(stderr, "identity unknown") || strings.Contains(stderr, "auto-detect email") || strings.Contains(stderr, "empty ident") {
		return "Git has no author identity. Set user.name and user.email, for example: git config --global user.email you@example.com"
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
