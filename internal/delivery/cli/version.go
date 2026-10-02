package cli

import (
	"fmt"
	"io"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
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
	result := app.NewResult(app.StatusOK, "Skill Hub build metadata retrieved.")
	result.Items = []app.Item{
		{ID: "version", Summary: info.Version, Impact: "Build version."},
		{ID: "commit", Summary: info.Commit, Impact: "Source revision."},
		{ID: "date", Summary: info.Date, Impact: "Build date."},
		{ID: "dirty", Summary: info.Dirty, Impact: "Source tree state at build time."},
	}
	return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
		p.Fields(
			termui.Field{Label: "skillhub version", Value: info.Version},
			termui.Field{Label: "commit", Value: info.Commit},
			termui.Field{Label: "built", Value: info.Date},
			termui.Field{Label: "dirty", Value: info.Dirty},
		)
	})
}
