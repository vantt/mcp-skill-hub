package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
)

// stdoutIsTerminal reports whether w is an interactive terminal. Tests
// replace it; the token is printed only when it returns true.
var stdoutIsTerminal = func(w io.Writer) bool {
	if out, ok := w.(commandOutput); ok {
		w = out.Writer
	}
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

type tokenConfig struct {
	workspacePath string
	tokenFile     string
	yes           bool
}

func runToken(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "show" && args[0] != "rotate") {
		return writeInvalidRequest(stdout, stderr, false, "token requires the show or rotate subcommand", "Run `skillhub token show` or `skillhub token rotate`.")
	}
	cfg, reason := parseTokenFlags(args[1:], args[0] == "rotate")
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, "Run `skillhub help token`.")
	}
	path := cfg.tokenFile
	if path != "" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return writeInvalidRequest(stdout, stderr, false, "the token file path is not usable", "Pass an absolute --token-file path.")
		}
		path = absolute
	} else {
		resolved, resErr := resolveWorkspace(cfg.workspacePath)
		if resErr != nil {
			return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
		}
		path = hubtoken.DefaultPath(resolved)
	}
	if args[0] == "show" {
		return runTokenShow(path, stdout, stderr)
	}
	return runTokenRotate(path, cfg, stdout, stderr)
}

func parseTokenFlags(args []string, allowYes bool) (tokenConfig, string) {
	var cfg tokenConfig
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--workspace", "--token-file":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return cfg, args[index] + " requires a path"
			}
			if args[index] == "--workspace" {
				cfg.workspacePath = args[index+1]
			} else {
				cfg.tokenFile = args[index+1]
			}
			index++
		case "--yes":
			if !allowYes {
				return cfg, "unknown argument \"--yes\""
			}
			cfg.yes = true
		default:
			return cfg, fmt.Sprintf("unknown argument %q", args[index])
		}
	}
	return cfg, ""
}

// runTokenShow prints the token only to a terminal, so it never lands in a
// file, a pipe, a log, or an agent transcript.
func runTokenShow(path string, stdout, stderr io.Writer) int {
	if !stdoutIsTerminal(stdout) {
		p := termui.New(stderr)
		p.Error(
			"The hub token is printed only on a terminal.",
			"Standard output is redirected or captured, and the token must not end up in a file, a log, or an agent transcript.",
			"Copy the token file below instead, or run `skillhub token show` directly in a terminal.",
		)
		p.Fields(termui.Field{Label: "Token file", Value: path})
		return 1
	}
	created, err := hubtoken.Ensure(path)
	if err != nil {
		writeHubTokenError(stderr, path, err)
		return 1
	}
	token, err := hubtoken.Read(path)
	if err != nil {
		writeHubTokenError(stderr, path, err)
		return 1
	}
	p := termui.New(stdout)
	if created {
		p.Line("No hub token existed, so a new one was created.")
		p.Blank()
	}
	p.Fields(
		termui.Field{Label: "Token file", Value: path},
		termui.Field{Label: "Token", Value: token},
	)
	p.Blank()
	p.Line("Anyone with this token can call the hub's runtime tools. Keep it private and prefer copying the token file.")
	if p.Err() != nil {
		return 1
	}
	return 0
}

func runTokenRotate(path string, cfg tokenConfig, stdout, stderr io.Writer) int {
	p := termui.New(stdout)
	if !cfg.yes {
		p.Line("This replaces the hub token with a new random one.")
		p.Fields(termui.Field{Label: "Token file", Value: path})
		p.Bullets(
			"Every machine that connects to this hub stops working until it gets the new token file.",
			"A running listener uses the new token from its next request; it does not need a restart.",
		)
		p.Blank()
		p.Next("Apply the rotation:", tokenRotateCommand(cfg))
		if p.Err() != nil {
			return 1
		}
		return 0
	}
	if err := hubtoken.Rotate(path); err != nil {
		writeHubTokenError(stderr, path, err)
		return 1
	}
	p.Line("Hub token rotated.")
	p.Fields(termui.Field{Label: "Token file", Value: path})
	p.Blank()
	p.Next("Copy the new token file to each machine that connects to this hub, replacing the old one.", "")
	if p.Err() != nil {
		return 1
	}
	return 0
}

func tokenRotateCommand(cfg tokenConfig) string {
	command := "skillhub token rotate"
	if cfg.tokenFile != "" {
		command += " --token-file " + shellQuote(cfg.tokenFile)
	} else if cfg.workspacePath != "" {
		command += " --workspace " + shellQuote(cfg.workspacePath)
	}
	return command + " --yes"
}
