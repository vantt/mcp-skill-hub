package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/hostintegration"
)

const connectFix = "Run `skillhub connect [<path>] [-g|--global] [--host claude|codex|gemini] [--dry-run]` in the project folder, or `skillhub connect --hub <url> --token-file <file>` for a hub on another machine."

type connectFlags struct {
	// hubPath is the optional folder of a hub on this machine.
	hubPath              string
	hub, tokenFile       string
	hosts                []hostintegration.Host
	global, dryRun, json bool
	remove, cache        bool
	hubSet, tokenFileSet bool
}

// connectUsageError is a flag combination connect refuses, with its own
// plain-words message.
type connectUsageError struct{ what, why, fix string }

func (e *connectUsageError) Error() string { return e.what + " " + e.why }

func runConnect(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	dir, err := os.Getwd()
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "The current folder cannot be determined: "+err.Error(), "Run `skillhub connect` in the project folder.")
	}
	return runConnectIn(ctx, dir, args, stdout, stderr)
}

// runConnectIn runs connect for the project folder dir, which stands for the
// current folder. Tests call it directly so they never change the process
// working directory.
func runConnectIn(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) int {
	flags, err := parseConnectFlags(args)
	if err != nil {
		var usage *connectUsageError
		if errors.As(err, &usage) {
			return writeWorkspaceResult(app.ErrorResult(&app.Error{Code: app.ErrorInvalidRequest, Render: app.ErrorRender{Error: usage.what, Why: usage.why, Fix: usage.fix}}), nil, stdout, stderr, hasJSONFlag(args))
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), connectFix)
	}
	switch {
	case flags.remove:
		return runConnectRemove(ctx, dir, flags, stdout, stderr)
	case flags.hubSet:
		return runConnectHub(ctx, dir, flags, stdout, stderr)
	}
	result, err := (app.ConnectService{}).Connect(ctx, app.ConnectRequest{
		Project: dir, Workspace: flags.hubPath, Global: flags.global, Hosts: flags.hosts, DryRun: flags.dryRun,
	})
	if err != nil {
		if code, handled := writeConnectSymlinkError(err, stdout, stderr, flags.json); handled {
			return code
		}
		return writeWorkspaceResult(result, err, stdout, stderr, flags.json)
	}
	if result.Error != nil {
		return writeWorkspaceResult(result, err, stdout, stderr, flags.json)
	}
	if flags.json {
		response := struct {
			app.Result
			CurationGuidance string `json:"curation_guidance,omitempty"`
		}{Result: result, CurationGuidance: connectCurationGuidance(result)}
		if err := writeJSON(stdout, response); err != nil {
			p := termui.New(stderr)
			p.Error("Unable to write the command result.", "Output destination failed.", "Check the output destination and retry.")
			return 1
		}
		return 0
	}
	renderConnectResult(stdout, result)
	return 0
}

// writeConnectSymlinkError explains a host path that contains a symbolic link,
// which connect never writes through. It reports false for any other error.
func writeConnectSymlinkError(err error, stdout, stderr io.Writer, jsonOutput bool) (int, bool) {
	if !errors.Is(err, hostintegration.ErrSymlink) {
		return 0, false
	}
	symlinkPath := ""
	parts := strings.Split(err.Error(), ": ")
	if len(parts) > 1 {
		symlinkPath = strings.TrimSpace(parts[len(parts)-1])
	}
	why := "A host integration path contains a symbolic link. Skill Hub will not write through symlinks."
	if symlinkPath != "" {
		why = fmt.Sprintf("Host integration path %s contains a symbolic link. Skill Hub will not write through symlinks.", symlinkPath)
	}
	fix := "Connect per project using `skillhub connect`, select unaffected hosts with `--host <name>`, or replace the symlink with a regular directory."
	return writeInvalidRequest(stdout, stderr, jsonOutput, why, fix), true
}

// Claude Code curates through the CLI. Emit guidance only when registration or
// permissions change, never for already-current connections or other hosts.
func connectCurationGuidance(result app.Result) string {
	for _, item := range result.Items {
		if item.ID == "host_"+string(hostintegration.HostClaude)+"_"+string(hostintegration.ChangeMCP) ||
			item.ID == "host_"+string(hostintegration.HostClaude)+"_"+string(hostintegration.ChangeHostPermissions) {
			return "Claude Code: curation runs through the CLI. It asks before skillhub commands that change the hub, its connections, the token or the cache (init, connect, doctor --fix, serve, token rotate, cache clear, and commands with --yes or confirm); --approve-content is blocked. First opening this folder shows a trust dialog listing Bash(skillhub:*)."
		}
	}
	return ""
}

// renderConnectResult prints one line per managed file. Full managed diffs stay
// in the --json items so a dry run remains readable.
func renderConnectResult(stdout io.Writer, result app.Result) {
	p := termui.New(stdout)
	p.Line(result.Summary)
	if result.Status == app.StatusApplied {
		p.Line(connectionSummaryLine(result))
	}
	if result.Status == app.StatusActionRequired {
		p.Line("Would change:")
	}
	hostFiles := make(map[string][]string)
	var hostOrder []string
	for _, item := range result.Items {
		hostKey, kind, relPath, ok := parseHostItemID(item.ID)
		if !ok || !hostItemWritten(item) {
			continue
		}
		if _, exists := hostFiles[hostKey]; !exists {
			hostOrder = append(hostOrder, hostKey)
		}
		if fileDesc := formatChangeKind(kind, item.Summary, relPath); !slices.Contains(hostFiles[hostKey], fileDesc) {
			hostFiles[hostKey] = append(hostFiles[hostKey], fileDesc)
		}
	}
	verb := "connected"
	if result.Status == app.StatusActionRequired {
		verb = "will connect"
	} else if result.Status == app.StatusReady {
		verb = "already connected"
	}
	for _, hostKey := range hostOrder {
		displayName := hostDisplayName(hostKey)
		files := hostFiles[hostKey]
		p.Bullets(fmt.Sprintf("%s: %s (%s)", displayName, verb, strings.Join(files, ", ")))
	}
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
	}
	if guidance := connectCurationGuidance(result); guidance != "" {
		p.Blank()
		p.Line(guidance)
	}
	switch result.Status {
	case app.StatusActionRequired:
		for _, action := range result.SuggestedActions {
			p.Blank()
			p.Line("To write these files, run:")
			p.Command(action.Command)
		}
	case app.StatusApplied:
		p.Blank()
		p.Next("restart your agent and ask it: \"curate my Skill Hub\"", "")
	}
}

func hostDisplayName(hostKey string) string {
	switch hostKey {
	case "claude", "claude-code":
		return "Claude Code"
	case "codex", "codex-cli":
		return "Codex"
	case "gemini", "gemini-cli":
		return "Gemini CLI"
	case app.SharedSkillsItemKey:
		return "Agent Skills folder (.agents/skills)"
	default:
		return hostKey
	}
}

// formatChangeKind names the file of a host item. relPath is the path from
// the item ID, set for skill files.
func formatChangeKind(kind, summary, relPath string) string {
	// The summary names the file with the platform's separators.
	summary = filepath.ToSlash(summary)
	switch kind {
	case "mcp-registration", "mcp":
		if strings.Contains(summary, ".claude.json") {
			return ".claude.json"
		}
		if strings.Contains(summary, ".codex/config.toml") {
			return ".codex/config.toml"
		}
		if strings.Contains(summary, ".gemini/settings.json") {
			return ".gemini/settings.json"
		}
		return ".mcp.json"
	case "host-permissions":
		if strings.Contains(summary, "settings.local.json") {
			return ".claude/settings.local.json"
		}
		return ".claude/settings.json"
	case "native-skill", "shared-skill":
		// .claude/skills/<id>/SKILL.md or .agents/skills/<id>/<file>
		if _, rest, found := strings.Cut(relPath, "skills/"); found {
			if id, _, found := strings.Cut(rest, "/"); found {
				return id + " skill"
			}
		}
		return "skill file"
	case "bootstrap-instructions", "bootstrap":
		if strings.Contains(summary, "AGENTS.md") {
			return "AGENTS.md"
		}
		if strings.Contains(summary, "GEMINI.md") {
			return "GEMINI.md"
		}
		return "CLAUDE.md"
	default:
		return kind
	}
}

func parseConnectFlags(args []string) (connectFlags, error) {
	var flags connectFlags
	value := func(index *int) (string, error) {
		if *index+1 >= len(args) || strings.HasPrefix(args[*index+1], "-") {
			return "", fmt.Errorf("%s requires a value", args[*index])
		}
		*index++
		return args[*index], nil
	}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			flags.json = true
		case "--dry-run":
			flags.dryRun = true
		case "-g", "--global":
			flags.global = true
		case "--hub":
			item, err := value(&index)
			if err != nil {
				return flags, err
			}
			flags.hub, flags.hubSet = item, true
		case "--token-file":
			item, err := value(&index)
			if err != nil {
				return flags, err
			}
			flags.tokenFile, flags.tokenFileSet = item, true
		case "--remove":
			flags.remove = true
		case "--cache":
			flags.cache = true
		case "--host":
			item, err := value(&index)
			if err != nil {
				return flags, err
			}
			for _, name := range strings.Split(item, ",") {
				host, err := parseHost(strings.TrimSpace(name))
				if err != nil {
					return flags, err
				}
				flags.hosts = append(flags.hosts, host)
			}
		default:
			if strings.HasPrefix(args[index], "-") {
				return flags, fmt.Errorf("unknown argument %q", args[index])
			}
			if flags.hubPath != "" {
				return flags, fmt.Errorf("connect accepts at most one hub folder")
			}
			flags.hubPath = args[index]
		}
	}
	switch {
	case flags.remove && (flags.hubPath != "" || flags.hubSet || flags.tokenFileSet):
		return flags, &connectUsageError{
			what: "--remove cannot be combined with a hub folder, --hub or --token-file.",
			why:  "--remove takes out the Skill Hub entry of the scope, whichever hub it points at.",
			fix:  "Run `skillhub connect --remove [-g|--global] [--host <name>]` in the project folder.",
		}
	case flags.hubPath != "" && flags.hubSet:
		return flags, &connectUsageError{
			what: "A hub folder and --hub cannot be used together.",
			why:  "A folder is a hub on this machine and --hub a hub on another machine; one scope points at one hub.",
			fix:  "Run `skillhub connect <path>` for the hub on this machine, or `skillhub connect --hub <url> --token-file <file>` for a hub on another machine.",
		}
	case flags.tokenFileSet && !flags.hubSet:
		return flags, &connectUsageError{
			what: "--token-file needs --hub.",
			why:  "The token file is for a hub on another machine; a hub on this machine needs no token.",
			fix:  "Add --hub <url>, or leave out --token-file.",
		}
	case flags.cache && !flags.remove:
		return flags, &connectUsageError{
			what: "--cache needs --remove.",
			why:  "--cache clears the skills fetched from a remote hub when its entry is removed.",
			fix:  "Add --remove, or clear the cache alone with `skillhub cache clear --hub <url>`.",
		}
	}
	return flags, nil
}

func parseHost(name string) (hostintegration.Host, error) {
	switch name {
	case "claude", string(hostintegration.HostClaude):
		return hostintegration.HostClaude, nil
	case "codex", string(hostintegration.HostCodex):
		return hostintegration.HostCodex, nil
	case "gemini", string(hostintegration.HostGemini):
		return hostintegration.HostGemini, nil
	}
	return "", fmt.Errorf("unsupported host %q; use claude, codex, or gemini", name)
}
