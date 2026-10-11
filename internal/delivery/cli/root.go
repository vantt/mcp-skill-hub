// Package cli is the command-line delivery adapter for Skill Hub.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

// Run executes a Skill Hub command and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunContext(context.Background(), args, stdout, stderr)
}

// RunContext executes a command with cancellation propagated to application services.
func RunContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeGlobalHelp(stdout)
	}

	stdout = commandOutput{Writer: stdout, args: args}
	jsonOutput := hasJSONFlag(args)
	if isHelpFlag(args[0]) {
		return writeGlobalHelp(stdout)
	}
	if wantsHelp(args[1:]) {
		if code, ok := writeCommandHelp(stdout, args[0]); ok {
			return code
		}
	}
	switch args[0] {
	case "help":
		return runHelp(args[1:], stdout, stderr)
	case "connect":
		return runConnect(ctx, args[1:], stdout, stderr)
	case "version", "--version":
		return runVersion(args[1:], stdout, stderr)
	case "status":
		return runStatus(ctx, args[1:], stdout, stderr)
	case "init":
		return runInit(ctx, args[1:], stdout, stderr)
	case "maintain":
		return runMaintain(ctx, args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(ctx, args[1:], stdout, stderr)
	case "skill":
		return runSkill(ctx, args[1:], stdout, stderr)
	case "source":
		return runSource(ctx, args[1:], stdout, stderr)
	case "resolve":
		return runResolve(ctx, args[1:], stdout, stderr)
	case "resolution":
		return runResolution(ctx, args[1:], stdout, stderr)
	case "eval":
		return runEvaluation(ctx, args[1:], stdout, stderr)
	case "telemetry":
		return runTelemetry(ctx, args[1:], stdout, stderr)
	case "mcp":
		return runMCP(ctx, args[1:], stdout, stderr)
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr)
	case "token":
		return runToken(args[1:], stdout, stderr)
	case "cache":
		return runCache(args[1:], stdout, stderr)
	case "update":
		return runUpdate(ctx, args[1:], stdout, stderr)
	default:
		return writeInvalidRequest(stdout, stderr, jsonOutput, fmt.Sprintf("The command %q is not supported.", args[0]), "Run `skillhub help` to list commands.")
	}
}

// runMaintain runs the maintenance commands: validate, rebuild, migrate and
// diff. doctor runs the checks and doctor --fix the repairs most people need.
func runMaintain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, false, "maintain needs a subcommand: validate, rebuild, migrate or diff.", "Run `skillhub help maintain`.")
	}
	switch args[0] {
	case "validate":
		return runValidate(ctx, args[1:], stdout, stderr)
	case "rebuild":
		return runRebuild(ctx, args[1:], stdout, stderr)
	case "migrate":
		return runMigrate(ctx, args[1:], stdout, stderr)
	case "diff":
		return runDiff(ctx, args[1:], stdout, stderr)
	}
	return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), fmt.Sprintf("unsupported maintain subcommand %q.", args[0]), "Run `skillhub help maintain` for validate, rebuild, migrate and diff.")
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
			p := termui.New(stderr)
			p.Error("Unable to write the command result.", err.Error(), "Check the output destination and retry.")
			return 1
		}
		return 2
	}

	p := termui.New(stderr)
	p.Error(result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
	return 2
}

func writeJSON(writer io.Writer, value any) error {
	if out, ok := writer.(commandOutput); ok {
		value = cliJSONValue(value, out.workspace())
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
