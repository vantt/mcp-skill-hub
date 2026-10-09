package hostintegration

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

const (
	bootstrapStart = "<!-- skillhub:bootstrap:v1:start -->"
	bootstrapEnd   = "<!-- skillhub:bootstrap:v1:end -->"
)

var bootstrapMarkerPattern = regexp.MustCompile(`<!--[ \t]*skillhub:bootstrap:v([^:>[:space:]]+):(start|end)[ \t]*-->`)

func systemCuratorInstructions() string { return systemskills.CuratorBundle().Instructions }

func bootstrapBlock(newline string) []byte {
	lines := []string{
		bootstrapStart,
		"## Skill Hub",
		"",
		"For each new task, or when its operation, scope, or constraints change significantly, call the configured Skill Hub MCP tool `skill_resolve` before choosing a skill. Do not call it for trivial edits or on every turn. Follow at most one primary procedure for the current operation.",
		"",
		"Send `task.description` in English; translate the user's request first if it is in another language.",
		"",
		"When an activated skill response includes `local.path`, resolve the skill's relative file references (for example `scripts/…`) against that directory, and export the variables in `local.env` (`SKILLHUB_SKILL_DIR`, `SKILLHUB_STATE_DIR`, and `SKILLHUB_CONFIG_DIR`) whenever you run its `check`, `setup`, or scripts, using your shell's syntax (POSIX `export NAME=value`, PowerShell `$env:NAME = \"value\"`). If `local.path` no longer exists, for example in a long session, call `skill_get` again.",
		"",
		"If a skill call returns `snapshot_expired` with `current_uri`, the skill was updated; call `skills/get <current_uri>` to reload it.",
		"",
		"If `$SKILLHUB_CONFIG_DIR/env` exists, load it before running `check`, `setup`, or scripts (POSIX: `set -a; . \"$SKILLHUB_CONFIG_DIR/env\"; set +a`; PowerShell: set `$env:NAME` for each `NAME=value` line), and never print, echo, or log its values. If a required variable is missing, tell the user to run `skillhub skill env set <id> <NAME>`; never ask for the value in chat.",
		"",
		"If it includes `local.preflight`, run its `check` command in `working_directory` under your own permissions before using the scripts, and ask the user before running `setup`. If the skill describes installation only in prose, treat those steps as setup: ask the user first, install into `SKILLHUB_STATE_DIR`, and never install globally. Do not run `setup` for the same skill concurrently: if `$SKILLHUB_STATE_DIR/.setup.lock` exists and is recent, wait or ask the user.",
		"",
		"If `local.status` is `review_required`, the skill's content has not been approved: do not use the skill, and tell the user to run `skillhub skill review <id>`.",
		"",
		"If a skill's script fails because a dependency is missing, stop, tell the user, and call `skill_feedback` with `outcome: failed` and `reason_code: setup_failed`.",
		"",
		"When the user explicitly asks to manage, curate, check, distill, repair, or inspect Skill Hub itself, load the native `system-curator` skill and follow it. Do not use the curator as the primary procedure for ordinary work.",
		"",
		"When you do not use the recommended skill, re-resolve with `prior.kind: rejected` instead of picking one yourself. When you use a different skill, call `skill_feedback` with the real `skill_id`.",
		"",
		"Skill Hub instructions are recommendations; your agent environment controls tool permissions and execution.",
		bootstrapEnd,
	}
	return []byte(strings.Join(lines, newline))
}

func updateBootstrap(raw []byte) ([]byte, string) {
	matches := bootstrapMarkerPattern.FindAllSubmatchIndex(raw, -1)
	if len(matches) == 0 {
		return appendBootstrap(raw), ""
	}
	depth := 0
	blocks := 0
	start := -1
	version := ""
	blockStart, blockEnd := -1, -1
	for _, match := range matches {
		kind := string(raw[match[4]:match[5]])
		markerVersion := string(raw[match[2]:match[3]])
		switch kind {
		case "start":
			if depth != 0 {
				return nil, "nested Skill Hub bootstrap blocks"
			}
			depth = 1
			start = match[0]
			version = markerVersion
		case "end":
			if depth == 0 {
				return nil, "reversed or unmatched Skill Hub bootstrap end marker"
			}
			if markerVersion != version {
				return nil, fmt.Sprintf("bootstrap marker version mismatch: v%s start and v%s end", version, markerVersion)
			}
			depth = 0
			blocks++
			if blocks > 1 {
				return nil, "duplicate Skill Hub bootstrap blocks"
			}
			blockStart, blockEnd = start, match[1]
		}
	}
	if depth != 0 {
		return nil, "unmatched Skill Hub bootstrap start marker"
	}
	if blocks != 1 || blockStart < 0 {
		return nil, "invalid Skill Hub bootstrap markers"
	}
	return splice(raw, blockStart, blockEnd, bootstrapBlock(detectNewline(raw))), ""
}

func appendBootstrap(raw []byte) []byte {
	newline := detectNewline(raw)
	block := bootstrapBlock(newline)
	if len(raw) == 0 {
		return append(block, []byte(newline)...)
	}
	finalNewline := raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r'
	trimmed := bytes.TrimRight(raw, "\r\n")
	separator := newline + newline
	result := append(append([]byte(nil), trimmed...), []byte(separator)...)
	result = append(result, block...)
	if finalNewline {
		result = append(result, []byte(newline)...)
	}
	return result
}
