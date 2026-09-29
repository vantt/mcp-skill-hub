// Package cli is the command-line delivery adapter for Skill Hub.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

// Run executes a Skill Hub command and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunContext(context.Background(), args, stdout, stderr)
}

// RunContext executes a command with cancellation propagated to application services.
func RunContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, false, "No command was provided.", "Run `skillhub version` to inspect this build.")
	}

	jsonOutput := hasJSONFlag(args)
	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "status":
		return runStatus(ctx, args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "validate":
		return runValidate(ctx, args[1:], stdout, stderr)
	case "rebuild":
		return runRebuild(ctx, args[1:], stdout, stderr)
	case "diff":
		return runDiff(ctx, args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "skill":
		return runSkill(ctx, args[1:], stdout, stderr)
	case "source":
		return runSource(ctx, args[1:], stdout, stderr)
	case "distill":
		return runDistill(ctx, args[1:], stdout, stderr)
	case "inbox":
		return runInbox(ctx, args[1:], stdout, stderr)
	case "insight":
		return runInsight(ctx, args[1:], stdout, stderr)
	case "check":
		return runCheck(ctx, args[1:], stdout, stderr)
	default:
		return writeInvalidRequest(stdout, stderr, jsonOutput, fmt.Sprintf("The command %q is not supported.", args[0]), "Run `skillhub version` to inspect this build.")
	}
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

func writeInvalidRequest(stdout, stderr io.Writer, jsonOutput bool, reason, fix string) int {
	result := app.ErrorResult(app.NewInvalidRequestError(reason, fix))
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintf(stderr, "ERROR: Unable to write the command result.\nWHY: %v\nFIX: Check the output destination and retry.\n", err)
			return 1
		}
		return 2
	}

	fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
	return 2
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
