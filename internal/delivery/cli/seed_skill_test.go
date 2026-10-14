package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

// Tests that need a skill on the hub seed it here. `skill create` writes in a
// project now, so a fixture goes through the lifecycle service the managed
// MCP create tool uses, committing like any confirmed change.

// seedSkillCLI creates a hub skill from the flags `skill create` took before it
// moved to the project, and returns what a command run returns.
func seedSkillCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	return seedSkillArgs(args)
}

func seedSkillArgs(args []string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := seedSkillRun(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func seedSkillRun(args []string, stdout, stderr io.Writer) int {
	var (
		input      skill.CreateInput
		workspace  string
		confirm    bool
		jsonOutput bool
		fullDiff   bool
		noCommit   bool
		contentRel string
	)
	valueFlags := map[string]*string{"--workspace": &workspace, "--id": &input.ID, "--collection": &input.Collection, "--name": &input.Name, "--description": &input.Description, "--min-scope": &input.Routing.MinScope, "--rationale": &input.Rationale, "--idempotency-key": &input.IdempotencyKey, "--content-file": &contentRel}
	listFlags := map[string]*[]string{"--operation": &input.Routing.Operations, "--trigger": &input.Routing.Triggers, "--not-for": &input.Routing.NotFor, "--example": &input.Routing.Examples, "--counter-example": &input.Routing.CounterExamples}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case valueFlags[arg] != nil:
			index++
			*valueFlags[arg] = args[index]
		case listFlags[arg] != nil:
			index++
			*listFlags[arg] = append(*listFlags[arg], args[index])
		case arg == "--yes":
			confirm = true
		case arg == "--json":
			jsonOutput = true
		case arg == "--diff":
			fullDiff = true
		case arg == "--no-commit":
			noCommit = true
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "seedSkill: unknown flag %s\n", arg)
			return 2
		default:
			input.ID = arg
		}
	}
	resolved, resolveErr := resolveWorkspace(workspace)
	if resolveErr != nil {
		fmt.Fprintln(stderr, resolveErr)
		return 2
	}
	if contentRel != "" {
		content, readErr := readContentFile(contentRel)
		if readErr != nil {
			fmt.Fprintln(stderr, readErr)
			return 2
		}
		input.Content = content
	}
	service := app.SkillService{}
	ctx := context.Background()
	preview, err := service.PreviewCreate(ctx, resolved, input, fullDiff)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if !confirm {
		if jsonOutput {
			_ = writeJSON(stdout, preview)
		}
		return 0
	}
	result, err := service.ConfirmSkillMutation(commitContext(ctx, noCommit, stdout), resolved, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if jsonOutput {
		result.Proposal = &preview
		_ = writeJSON(stdout, result)
	} else {
		fmt.Fprintln(stdout, result.Summary)
	}
	return 0
}
