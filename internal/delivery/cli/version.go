package cli

import (
	"fmt"
	"io"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/version"
)

func runVersion(args []string, stdout, stderr io.Writer) int {
	jsonOutput := hasJSONFlag(args)
	jsonFlagCount := 0
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonFlagCount++
			if jsonFlagCount > 1 {
				return writeInvalidRequest(stdout, stderr, true, "The --json flag was provided more than once.", "Run `skillhub version --json` with the flag once.")
			}
		default:
			return writeInvalidRequest(stdout, stderr, jsonOutput, fmt.Sprintf("The flag or argument %q is not supported for version.", arg), "Run `skillhub version` or `skillhub version --json`.")
		}
	}

	info := version.Current()
	if jsonOutput {
		result := app.NewResult(app.StatusOK, "Skill Hub build metadata retrieved.")
		result.Items = []app.Item{
			{ID: "version", Summary: info.Version, Impact: "Build version."},
			{ID: "commit", Summary: info.Commit, Impact: "Source revision."},
			{ID: "date", Summary: info.Date, Impact: "Build date."},
			{ID: "dirty", Summary: info.Dirty, Impact: "Source tree state at build time."},
		}
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintf(stderr, "ERROR: Unable to write the command result.\nWHY: %v\nFIX: Check the output destination and retry.\n", err)
			return 1
		}
		return 0
	}

	if _, err := fmt.Fprintf(stdout, "skillhub version: %s\ncommit: %s\nbuilt: %s\ndirty: %s\n", info.Version, info.Commit, info.Date, info.Dirty); err != nil {
		fmt.Fprintf(stderr, "ERROR: Unable to write the command result.\nWHY: %v\nFIX: Check the output destination and retry.\n", err)
		return 1
	}
	return 0
}
