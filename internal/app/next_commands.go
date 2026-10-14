package app

import (
	"strings"

	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// ShellCommandFor turns a suggested-action command into the plain shell
// command a person can run. Commands that are MCP tool names map to their
// CLI counterpart; a command that is already a shell command is returned as
// it is. The result carries no --json or --workspace flags.
func ShellCommandFor(command, id string) string {
	if strings.HasPrefix(command, "skillhub ") {
		return command
	}
	target := ""
	if id != "" {
		target = " " + shellquote.Arg(id)
	}
	var cli string
	switch command {
	case "workspace_validate", "ValidateWorkspace":
		cli = "skillhub maintain validate"
	case "workspace_rebuild":
		cli = "skillhub maintain rebuild"
	case "workspace_diff", "GetCurationDiff":
		cli = "skillhub maintain diff"
	case "source_check":
		cli = "skillhub source check"
	case "skill_upstream_status":
		cli = "skillhub skill outdated"
	case "skill_review":
		cli = "skillhub skill review" + target
	case "skill_list":
		cli = "skillhub skill list"
	case "skill_active":
		cli = "skillhub skill activate" + target
	case "skill_deprecated":
		cli = "skillhub skill deprecate" + target
	case "skill_archived":
		cli = "skillhub skill archive" + target
	case "skill_edit", "PreviewSkillUpdate":
		// Original edit inputs are unavailable after a stale confirmation.
		// Inspect the affected skill rather than advertise an invalid bare edit.
		cli = "skillhub skill review" + target
	case "skill_create":
		// The command line no longer creates a hub draft: a draft made through
		// the managed tools is inspected like any other skill.
		cli = "skillhub skill review" + target
	case "doctor":
		cli = "skillhub doctor"
	case "curation_run_start", "curation_run_retry":
		cli = "skillhub source list"
	default:
		if command != "" {
			return command
		} // Declared runtime setup is already a shell command.
		cli = "skillhub status"
	}
	if target == "" && (strings.HasPrefix(cli, "skillhub skill review") || strings.HasPrefix(cli, "skillhub skill edit")) {
		cli = "skillhub skill list"
	}
	return cli
}
