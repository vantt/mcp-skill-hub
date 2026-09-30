package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

const (
	maxDeliveryArgs      = 16
	maxDeliveryValueSize = 4096
)

type telemetryPreviewResult struct {
	Version string `json:"version"`
	Events  int    `json:"events"`
	Skipped int    `json:"skipped"`
	JSONL   string `json:"jsonl"`
}

type telemetryPurgeResult struct {
	Status string `json:"status"`
}

func runTelemetry(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := validateDeliveryArguments(args); err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub telemetry health --workspace <path>`.")
	}
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "telemetry requires a subcommand", "Run `skillhub telemetry health --workspace <path>`.")
	}
	subcommand := args[0]
	workspacePath, outputPath, jsonOutput, yes, err := telemetryFlags(subcommand, args[1:])
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), telemetryUsageFix(subcommand))
	}
	service := app.TelemetryService{}
	switch subcommand {
	case "health":
		health, operationErr := service.Health(ctx, workspacePath)
		if operationErr != nil {
			return writeTelemetryError(operationErr, workspacePath, stdout, stderr, jsonOutput)
		}
		if jsonOutput {
			return writeTelemetryJSON(stdout, stderr, health)
		}
		fmt.Fprintf(stdout, "Telemetry is %s: %d written, %d dropped, %d rejected, %d errors.\n", health.State, health.Written, health.Dropped, health.Rejected, health.Errors)
		return 0
	case "preview":
		preview, operationErr := service.Preview(ctx, workspacePath, 0)
		if operationErr != nil {
			return writeTelemetryError(operationErr, workspacePath, stdout, stderr, jsonOutput)
		}
		return writeTelemetryPreview(preview, stdout, stderr, jsonOutput)
	case "export":
		if !yes {
			preview, operationErr := service.Preview(ctx, workspacePath, 0)
			if operationErr != nil {
				return writeTelemetryError(operationErr, workspacePath, stdout, stderr, jsonOutput)
			}
			return writeTelemetryPreview(preview, stdout, stderr, jsonOutput)
		}
		if err := validateTelemetryOutputPath(outputPath); err != nil {
			return writeInvalidRequest(stdout, stderr, jsonOutput, err.Error(), "Choose a regular output path and retry.")
		}
		result, operationErr := service.Export(ctx, workspacePath, outputPath)
		if operationErr != nil {
			return writeTelemetryError(operationErr, workspacePath, stdout, stderr, jsonOutput)
		}
		if jsonOutput {
			return writeTelemetryJSON(stdout, stderr, result)
		}
		fmt.Fprintf(stdout, "Exported %d sanitized telemetry events (%d bytes) to %s.\n", result.Events, result.Bytes, result.Path)
		return 0
	case "purge":
		if operationErr := service.Purge(ctx, workspacePath); operationErr != nil {
			// Purge is the remedy itself, so it never suggests running purge.
			if errors.Is(operationErr, context.Canceled) {
				return writeWorkspaceResult(app.Result{}, operationErr, stdout, stderr, jsonOutput)
			}
			return writeInvalidWorkspace(stdout, stderr, jsonOutput, operationErr)
		}
		if jsonOutput {
			return writeTelemetryJSON(stdout, stderr, telemetryPurgeResult{Status: "purged"})
		}
		fmt.Fprintln(stdout, "Telemetry purged.")
		return 0
	default:
		panic("validated telemetry subcommand")
	}
}

// telemetryUsageFix names the exact command to retry; purge needs --yes.
func telemetryUsageFix(subcommand string) string {
	if subcommand == "purge" {
		return "Run `skillhub telemetry purge --workspace <path> --yes`."
	}
	return "Run `skillhub telemetry " + subcommand + " --workspace <path>`."
}

func validateDeliveryArguments(args []string) error {
	if len(args) > maxDeliveryArgs {
		return errors.New("too many arguments")
	}
	for _, arg := range args {
		if len(arg) == 0 || len(arg) > maxDeliveryValueSize {
			return fmt.Errorf("each argument must be between 1 and %d bytes", maxDeliveryValueSize)
		}
	}
	return nil
}

func telemetryFlags(subcommand string, args []string) (workspacePath, outputPath string, jsonOutput, yes bool, err error) {
	if subcommand != "health" && subcommand != "preview" && subcommand != "export" && subcommand != "purge" {
		return "", "", false, false, fmt.Errorf("unsupported telemetry subcommand %q", subcommand)
	}
	if len(args) > maxDeliveryArgs {
		return "", "", false, false, errors.New("too many arguments")
	}
	seen := map[string]bool{}
	for index := 0; index < len(args); index++ {
		flag := args[index]
		if seen[flag] {
			return "", "", jsonOutput, yes, fmt.Errorf("%s may only be provided once", flag)
		}
		seen[flag] = true
		switch flag {
		case "--json":
			jsonOutput = true
		case "--yes":
			yes = true
		case "--workspace", "--output":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", "", jsonOutput, yes, fmt.Errorf("%s requires a value", flag)
			}
			value := args[index+1]
			if len(value) == 0 || len(value) > maxDeliveryValueSize {
				return "", "", jsonOutput, yes, fmt.Errorf("%s value must be between 1 and %d bytes", flag, maxDeliveryValueSize)
			}
			if flag == "--workspace" {
				workspacePath = value
			} else {
				outputPath = value
			}
			index++
		default:
			return "", "", jsonOutput, yes, fmt.Errorf("unknown argument %q", flag)
		}
	}
	switch subcommand {
	case "health", "preview":
		if outputPath != "" || yes {
			return "", "", jsonOutput, yes, fmt.Errorf("--output and --yes are not valid for telemetry %s", subcommand)
		}
	case "export":
		if yes && outputPath == "" {
			return "", "", jsonOutput, yes, errors.New("--output is required when --yes is provided")
		}
	case "purge":
		if outputPath != "" {
			return "", "", jsonOutput, yes, errors.New("--output is not valid for telemetry purge")
		}
		if !yes {
			return "", "", jsonOutput, yes, errors.New("telemetry purge requires --yes")
		}
	}
	resolved, resErr := resolveWorkspace(workspacePath)
	if resErr != nil {
		return "", "", jsonOutput, yes, resErr
	}
	return resolved, outputPath, jsonOutput, yes, nil
}

func writeTelemetryPreview(preview telemetry.Preview, stdout, stderr io.Writer, jsonOutput bool) int {
	if jsonOutput {
		return writeTelemetryJSON(stdout, stderr, telemetryPreviewResult{Version: preview.Version, Events: preview.Events, Skipped: preview.Skipped, JSONL: string(preview.JSONL)})
	}
	if len(preview.JSONL) == 0 {
		fmt.Fprintln(stdout, "No telemetry events.")
		return 0
	}
	if _, err := stdout.Write(preview.JSONL); err != nil {
		fmt.Fprintf(stderr, "ERROR: Unable to write telemetry preview.\nWHY: %v\nFIX: Check the output destination and retry.\n", err)
		return 1
	}
	return 0
}

func writeTelemetryJSON(stdout, stderr io.Writer, value any) int {
	if err := writeJSON(stdout, value); err != nil {
		fmt.Fprintf(stderr, "ERROR: Unable to write telemetry result.\nWHY: %v\nFIX: Check the output destination and retry.\n", err)
		return 1
	}
	return 0
}

func writeTelemetryError(err error, workspacePath string, stdout, stderr io.Writer, jsonOutput bool) int {
	if errors.Is(err, context.Canceled) {
		return writeWorkspaceResult(app.Result{}, err, stdout, stderr, jsonOutput)
	}
	var storeErr *app.TelemetryStoreError
	if !errors.As(err, &storeErr) {
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	// The store is disposable and independent of workspace health, so doctor
	// cannot repair it; purging recreates an empty store.
	result := app.ErrorResult(&app.Error{Code: app.ErrorInternal, Render: app.ErrorRender{
		Error: "Telemetry storage is degraded.",
		Why:   err.Error(),
		Fix:   "Run `skillhub telemetry purge --workspace " + shellQuote(workspacePath) + " --yes` to discard and recreate the disposable telemetry store.",
	}})
	if jsonOutput {
		if writeErr := writeJSON(stdout, result); writeErr != nil {
			fmt.Fprintln(stderr, writeErr)
			return 1
		}
		return 2
	}
	fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
	return 2
}

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-:+@%", r))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func validateTelemetryOutputPath(path string) error {
	if path == "" || len(path) > maxDeliveryValueSize {
		return fmt.Errorf("output path must be between 1 and %d bytes", maxDeliveryValueSize)
	}
	info, err := os.Lstat(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect output path: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return errors.New("output path must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return errors.New("output path must be a regular file")
	}
	// --yes is the explicit authorization to replace an existing regular export.
	return nil
}
