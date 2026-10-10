package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
)

const connectFix = "Run `skillhub connect [--project <dir>] [-g|--global] [--workspace <path>] [--host claude|codex|gemini] [--yes]`."

type connectFlags struct {
	project, workspace string
	hosts              []hostintegration.Host
	global, yes, json  bool
}

func runConnect(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags, err := parseConnectFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), connectFix)
	}
	ws, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, flags.json, resErr)
	}
	result, err := (app.ConnectService{}).Connect(ctx, app.ConnectRequest{
		Project: flags.project, Workspace: ws, Global: flags.global, Hosts: flags.hosts, Yes: flags.yes,
	})
	if err != nil {
		if errors.Is(err, hostintegration.ErrSymlink) {
			symlinkPath := ""
			parts := strings.Split(err.Error(), ": ")
			if len(parts) > 1 {
				symlinkPath = strings.TrimSpace(parts[len(parts)-1])
			}
			why := "A host integration path contains a symbolic link. Skill Hub will not write through symlinks."
			if symlinkPath != "" {
				why = fmt.Sprintf("Host integration path %s contains a symbolic link. Skill Hub will not write through symlinks.", symlinkPath)
			}
			fix := "Connect per project using `skillhub connect --yes`, select unaffected hosts with `--host <name>`, or replace the symlink with a regular directory."
			return writeInvalidRequest(stdout, stderr, flags.json, why, fix)
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

// Claude Code curates through the CLI. Emit guidance only when registration or
// permissions change, never for already-current connections or other hosts.
func connectCurationGuidance(result app.Result) string {
	for _, item := range result.Items {
		if item.ID == "host_"+string(hostintegration.HostClaude)+"_"+string(hostintegration.ChangeMCP) ||
			item.ID == "host_"+string(hostintegration.HostClaude)+"_"+string(hostintegration.ChangeHostPermissions) {
			return "Claude Code: curation runs through the CLI. Commands with --yes or confirm require approval; --approve-content is blocked. First opening this folder shows a trust dialog listing Bash(skillhub:*)."
		}
	}
	return ""
}

// renderConnectResult prints one line per managed file. Full managed diffs stay
// in the --json items so the preview remains readable.
func renderConnectResult(stdout io.Writer, result app.Result) {
	p := termui.New(stdout)
	p.Line(result.Summary)
	hostFiles := make(map[string][]string)
	var hostOrder []string
	for _, item := range result.Items {
		parts := strings.Split(item.ID, "_")
		if len(parts) >= 3 && parts[0] == "host" {
			hostKey := parts[1]
			if _, exists := hostFiles[hostKey]; !exists {
				hostOrder = append(hostOrder, hostKey)
			}
			fileDesc := formatChangeKind(parts[2], item.Summary)
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
		p.Next("restart your agent so it loads the new MCP server, then ask it: \"curate my Skill Hub\"", "")
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
	default:
		return hostKey
	}
}

func formatChangeKind(kind, summary string) string {
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
	case "native-skill":
		return "curator skill"
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
		case "--yes":
			flags.yes = true
		case "-g", "--global":
			flags.global = true
		case "--project":
			item, err := value(&index)
			if err != nil {
				return flags, err
			}
			flags.project = item
		case "--workspace":
			item, err := value(&index)
			if err != nil {
				return flags, err
			}
			flags.workspace = item
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
			return flags, fmt.Errorf("unknown argument %q", args[index])
		}
	}
	if flags.global && flags.project != "" {
		return flags, fmt.Errorf("--global and --project cannot be combined")
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
