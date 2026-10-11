package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

func runDiff(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path, jsonOutput, err := workspaceFlag(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub diff --workspace <path>`.")
	}
	result, err := (app.WorkspaceService{}).GetCurationDiff(ctx, path)
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			p := termui.New(stderr)
			p.Line(err.Error())
			return 1
		}
		return 0
	}
	p := termui.New(stdout)
	p.Line(result.Summary)
	for _, group := range result.Groups {
		label := strings.ReplaceAll(group.Kind, "_", " ")
		p.Line(fmt.Sprintf("%s (%d):", label, group.Count))
		for _, file := range group.Files {
			statusWord := diffStatusWord(file.Status)
			p.Line(fmt.Sprintf("  %s  %s", statusWord, file.Path))
		}
	}
	p.Blank()
	p.Line("To commit these changes, run:")
	p.Command(fmt.Sprintf("%s && git -C %s commit -m \"...\"", stageWorkspaceCommand(path), path))
	return 0
}

func diffStatusWord(status string) string {
	switch status {
	case "A":
		return "added"
	case "M":
		return "modified"
	case "D":
		return "deleted"
	case "??":
		return "untracked"
	case "R":
		return "renamed"
	case "U":
		return "conflicted"
	default:
		return status
	}
}

// stageWorkspaceCommand stages the folders Skill Hub owns by name. The connection files that init
// and connect write hold paths of this machine, so the whole folder is never staged at once.
func stageWorkspaceCommand(path string) string {
	return fmt.Sprintf("git -C %s add .gitignore .skillhub skills sources history registry config distill evals", path)
}
