package app

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/term"

	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// InitSource is the Git repository `skillhub init <git-url>` starts from.
type InitSource struct {
	// Kind is "url" (https, ssh or file URL), "scp" ([user@]host:path) or
	// "path" (a local directory that holds a Git repository).
	Kind string `json:"kind"`
	// Location is what Git is given: the URL, the scp form or an absolute path.
	Location string `json:"location"`
	// Display is Location as it may be printed: never with a password.
	Display string `json:"display"`
}

// IsSet reports whether init was given a repository.
func (source InitSource) IsSet() bool { return source.Kind != "" }

var scpForm = regexp.MustCompile(`^(?:([^@/]+)@)?([^@/:]+):(.*)$`)

// ClassifyInitSource checks the argument of `skillhub init <git-url>`. It
// accepts https, ssh and file URLs, the scp form [user@]host:path and an
// existing local directory that holds a Git repository. Anything that Git
// could read as an option or as a transport helper (ext::, fd::) is refused,
// and a password is never accepted in the argument.
func ClassifyInitSource(arg string) (InitSource, *Error) {
	refuse := func(why string) (InitSource, *Error) {
		return InitSource{}, &Error{Code: ErrorInvalidRequest, Render: ErrorRender{
			Error: "skillhub init cannot use " + redactArgument(strings.TrimSpace(arg)) + ".",
			Why:   why,
			Fix:   "Pass the https or ssh URL of an empty repository or of an existing hub, or run `" + shellquote.Chain("cd <folder>", "skillhub init") + "` in an empty folder.",
		}}
	}
	value := strings.TrimSpace(arg)
	switch {
	case value == "":
		return refuse("The argument is empty.")
	case strings.HasPrefix(value, "-"):
		return refuse("It starts with a dash, so Git would read it as an option.")
	case strings.Contains(value, "::"):
		return refuse("Git transport helpers (such as ext:: or fd::) can run commands and are not accepted.")
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" && parsed.Scheme != "file" {
			return refuse("It is not a valid URL.")
		}
		switch parsed.Scheme {
		case "https":
			if parsed.User != nil {
				return refuse("It contains a user name or password. Let Git ask for credentials or use a credential helper instead.")
			}
		case "ssh":
			if _, hasPassword := parsed.User.Password(); hasPassword {
				return refuse("It contains a password. Use an SSH key instead.")
			}
		case "file":
		default:
			return refuse("Only https, ssh and file URLs are accepted, not " + parsed.Scheme + "://.")
		}
		return InitSource{Kind: "url", Location: value, Display: parsed.Redacted()}, nil
	}
	if !looksLikeLocalPath(value) {
		if match := scpForm.FindStringSubmatch(value); match != nil && len(match[2]) > 1 {
			if strings.Contains(match[1], ":") {
				return refuse("It contains a password. Use an SSH key instead.")
			}
			return InitSource{Kind: "scp", Location: value, Display: value}, nil
		}
	}
	if info, err := os.Stat(value); err == nil && info.IsDir() {
		absolute, err := filepath.Abs(value)
		if err == nil && isGitRepositoryPath(absolute) {
			return InitSource{Kind: "path", Location: absolute, Display: absolute}, nil
		}
		return refuse(value + " is a folder, but not a Git repository. init creates the hub in the current folder and takes no folder argument.")
	}
	return refuse("It is not an https, ssh or file URL, an scp-style address, or a local Git repository.")
}

// looksLikeLocalPath reports whether value is a path rather than an scp-style
// address: it has a slash before the first colon, or it starts with a drive
// letter (C:\x, C:/x), which Git also treats as a path.
func looksLikeLocalPath(value string) bool {
	colon := strings.Index(value, ":")
	if colon < 0 {
		return true
	}
	if slash := strings.IndexAny(value, `/\`); slash >= 0 && slash < colon {
		return true
	}
	return colon == 1 && isLetter(value[0])
}

func isLetter(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

// isGitRepositoryPath reports whether dir itself is a work tree or a bare
// repository, not a folder somewhere inside another repository.
func isGitRepositoryPath(dir string) bool {
	output, err := offlineGitCommand(context.Background(), dir, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return false
	}
	gitDir := strings.TrimSpace(string(output))
	return sameDirectory(gitDir, dir) || sameDirectory(gitDir, filepath.Join(dir, ".git"))
}

var userinfoPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/@\s]+@`)

// redactUserinfo removes the user information of every URL in text, so a
// password never reaches the output, even when Git echoes the URL back.
func redactUserinfo(text string) string {
	return userinfoPattern.ReplaceAllString(text, "${1}***@")
}

var scpPassword = regexp.MustCompile(`^([^@/:]+):[^@/]*@`)

// redactArgument removes a password from an init argument in URL or scp form.
func redactArgument(value string) string {
	return scpPassword.ReplaceAllString(redactUserinfo(value), "${1}:***@")
}

// remoteGitCommand runs Git for an operation that talks to a remote: ls-remote,
// fetch and push. Unlike offlineGitCommand it leaves credential prompts on when
// a person is at the terminal; without a terminal it turns them off, so an agent
// gets Git's own error at once instead of a hanging prompt. Transport helpers
// that run commands and file-system monitors are always disabled.
func remoteGitCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	return remoteGitCommandFor(ctx, term.IsTerminal(int(os.Stdin.Fd())), root, args...)
}

func remoteGitCommandFor(ctx context.Context, interactive bool, root string, args ...string) *exec.Cmd {
	gitArgs := []string{
		"-c", "protocol.ext.allow=never",
		"-c", "protocol.fd.allow=never",
		"-c", "core.fsmonitor=false",
	}
	if root != "" {
		gitArgs = append(gitArgs, "-C", root)
	}
	command := exec.CommandContext(ctx, "git", append(gitArgs, args...)...)
	command.Env = append(os.Environ(), "GIT_PAGER=cat")
	if interactive {
		command.Stdin = os.Stdin
	} else {
		command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	}
	return command
}
