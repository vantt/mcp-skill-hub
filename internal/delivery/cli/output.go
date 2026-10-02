package cli

import (
	"io"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

// writeResult is the single exit path for a command value that was returned without a Go error.
//   JSON mode:  writeJSON(stdout, value); returns 2 when app.ErrorOf(value, nil) != nil, else 0;
//               returns 1 when writing fails.
//   human mode: when app.ErrorOf(value, nil) != nil, prints its ERROR/WHY/FIX with termui to stderr
//               and returns 2; otherwise calls render(termui.New(stdout)) and returns 0
//               (1 if the printer recorded a write error).
func writeResult(stdout, stderr io.Writer, jsonOutput bool, value any, render func(p *termui.Printer)) int {
	appErr := app.ErrorOf(value, nil)
	if jsonOutput {
		if err := writeJSON(stdout, value); err != nil {
			return 1
		}
		if appErr != nil {
			return 2
		}
		return 0
	}

	if appErr != nil {
		p := termui.New(stderr)
		p.Error(appErr.Render.Error, appErr.Render.Why, appErr.Render.Fix)
		if p.Err() != nil {
			return 1
		}
		return 2
	}

	p := termui.New(stdout)
	if render != nil {
		render(p)
	}
	if p.Err() != nil {
		return 1
	}
	return 0
}
