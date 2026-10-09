package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

type casesFlags struct {
	workspace  string
	sinceStr   string
	kind       string
	jsonOutput bool
}

func parseCasesFlags(args []string) (casesFlags, error) {
	flags := casesFlags{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			flags.jsonOutput = true
		case "--workspace", "--since", "--kind":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return flags, fmt.Errorf("%s requires a value", arg)
			}
			val := args[i+1]
			switch arg {
			case "--workspace":
				flags.workspace = val
			case "--since":
				flags.sinceStr = val
			case "--kind":
				flags.kind = val
			}
			i++
		default:
			return flags, fmt.Errorf("unknown flag %q", arg)
		}
	}
	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return flags, resErr
	}
	flags.workspace = resolved
	return flags, nil
}

func runTelemetryCases(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		subcommand := args[0]
		switch subcommand {
		case "enable":
			flags, err := parseCasesFlags(args[1:])
			if err != nil {
				return writeInvalidRequest(stdout, stderr, false, err.Error(), "")
			}
			if err := telemetry.SetCaseJournalEnabled(flags.workspace, true); err != nil {
				return writeInvalidRequest(stdout, stderr, false, "failed to enable case journal: "+err.Error(), "")
			}
			p := termui.New(stdout)
			p.Line("Case journal enabled.")
			return 0
		case "disable":
			flags, err := parseCasesFlags(args[1:])
			if err != nil {
				return writeInvalidRequest(stdout, stderr, false, err.Error(), "")
			}
			if err := telemetry.SetCaseJournalEnabled(flags.workspace, false); err != nil {
				return writeInvalidRequest(stdout, stderr, false, "failed to disable case journal: "+err.Error(), "")
			}
			p := termui.New(stdout)
			p.Line("Case journal disabled.")
			return 0
		case "status":
			flags, err := parseCasesFlags(args[1:])
			if err != nil {
				return writeInvalidRequest(stdout, stderr, false, err.Error(), "")
			}
			p := termui.New(stdout)
			if telemetry.IsCaseJournalEnabled(flags.workspace) {
				p.Line("Case journal: enabled")
			} else {
				p.Line("Case journal: disabled")
			}
			return 0
		case "list":
			args = args[1:]
		}
	}

	flags, err := parseCasesFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub telemetry cases [enable|disable|status|list] [--since <Nd|YYYY-MM-DD>] [--kind override|after_no_skill|...]`.")
	}

	var since time.Time
	if flags.sinceStr != "" {
		parsedSince, err := parseChainsSince(flags.sinceStr)
		if err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Pass `--since <Nd|YYYY-MM-DD>`.")
		}
		since = parsedSince
	}

	telemetryService := app.TelemetryService{}
	recorder, err := telemetryService.Open(flags.workspace)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Check arguments and workspace health with `skillhub doctor`.")
	}
	defer recorder.Close(ctx)

	cases, err := recorder.Cases(ctx, since, flags.kind)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "failed to query cases: "+err.Error(), "")
	}

	if flags.jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cases); err != nil {
			fmt.Fprintf(stderr, "error formatting JSON: %v\n", err)
			return 1
		}
		return 0
	}

	p := termui.New(stdout)
	title := "Disagreement Cases"
	if flags.kind != "" {
		title = fmt.Sprintf("Disagreement Cases (%s)", flags.kind)
	}
	p.Heading(title)

	if len(cases) == 0 {
		p.Line("No cases found in the selected window.")
		return 0
	}

	for _, c := range cases {
		p.Line(fmt.Sprintf("\n--- %s | %s | %s (%s)", c.OccurredAt.Format("2006-01-02 15:04:05"), c.Kind, c.Client.Name, c.CaseID))
		if c.Chosen != "" {
			p.Line(fmt.Sprintf("Chosen: %s", c.Chosen))
		}
		task, _ := c.Task["description"].(string)
		if task != "" {
			p.Line(fmt.Sprintf("Task: %s", task))
		}
	}
	return 0
}
