package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/selfupdate"
)

func runUpdate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	targetVersion, checkOnly, jsonOutput, err := parseUpdateFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub update [--version <v>] [--check] [--json]`.")
	}

	opts := selfupdate.Options{
		TargetVersion: targetVersion,
		CheckOnly:     checkOnly,
		Stdout:        stdout,
		Stderr:        stderr,
	}
	if jsonOutput {
		opts.Stdout = io.Discard
	}

	res, err := selfupdate.Run(ctx, opts)
	if err != nil {
		var suErr *selfupdate.Error
		if errors.As(err, &suErr) {
			result := app.ErrorResult(&app.Error{
				Code: app.ErrorInvalidRequest,
				Render: app.ErrorRender{
					Error: suErr.Summary,
					Why:   suErr.Why,
					Fix:   suErr.Fix,
				},
			})
			if jsonOutput {
				_ = writeJSON(stdout, result)
				return 1
			}
			fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", suErr.Summary, suErr.Why, suErr.Fix)
			return 1
		}

		if jsonOutput {
			result := app.ErrorResult(app.NewInvalidRequestError(err.Error(), "Check update arguments or environment and retry."))
			_ = writeJSON(stdout, result)
			return 1
		}
		fmt.Fprintf(stderr, "ERROR: %s\nWHY: %v\nFIX: Retry the update or inspect the installation.\n", err.Error(), err)
		return 1
	}

	if jsonOutput {
		result := app.NewResult(app.StatusOK, res.Message)
		result.Details = map[string]any{
			"current_version":    res.CurrentVersion,
			"target_version":     res.TargetVersion,
			"updated":            res.Updated,
			"already_up_to_date": res.AlreadyUpToDate,
			"check_only":         res.CheckOnly,
		}
		result.Items = []app.Item{
			{ID: "current_version", Summary: res.CurrentVersion, Impact: "Currently installed version."},
			{ID: "target_version", Summary: res.TargetVersion, Impact: "Target release version."},
		}
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintf(stderr, "ERROR: Unable to write the command result.\nWHY: %v\nFIX: Check the output destination and retry.\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintln(stdout, res.Message)
	return 0
}

func parseUpdateFlags(args []string) (targetVersion string, checkOnly, jsonOutput bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--check":
			checkOnly = true
		case arg == "--json":
			jsonOutput = true
		case arg == "--version":
			if i+1 >= len(args) {
				return "", false, false, errors.New("missing value for --version")
			}
			i++
			targetVersion = args[i]
			if strings.TrimSpace(targetVersion) == "" {
				return "", false, false, errors.New("target version cannot be empty")
			}
		case strings.HasPrefix(arg, "--version="):
			targetVersion = strings.TrimPrefix(arg, "--version=")
			if strings.TrimSpace(targetVersion) == "" {
				return "", false, false, errors.New("target version cannot be empty")
			}
		default:
			return "", false, false, fmt.Errorf("the flag or argument %q is not supported for update", arg)
		}
	}
	return targetVersion, checkOnly, jsonOutput, nil
}
