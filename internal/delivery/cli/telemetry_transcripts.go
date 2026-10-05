package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

type importTranscriptsFlags struct {
	project    string
	sinceStr   string
	workspace  string
	jsonOutput bool
}

func parseImportTranscriptsFlags(args []string) (importTranscriptsFlags, error) {
	flags := importTranscriptsFlags{}
	seen := map[string]bool{}

	for i := 0; i < len(args); i++ {
		flag := args[i]
		if seen[flag] {
			return flags, fmt.Errorf("%s may only be provided once", flag)
		}
		seen[flag] = true

		switch flag {
		case "--json":
			flags.jsonOutput = true
		case "--project", "--since", "--workspace":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return flags, fmt.Errorf("%s requires a value", flag)
			}
			val := args[i+1]
			if len(val) == 0 || len(val) > maxDeliveryValueSize {
				return flags, fmt.Errorf("value for %s must be between 1 and %d bytes", flag, maxDeliveryValueSize)
			}
			switch flag {
			case "--project":
				flags.project = val
			case "--since":
				flags.sinceStr = val
			case "--workspace":
				flags.workspace = val
			}
			i++
		default:
			return flags, fmt.Errorf("unrecognized flag %q", flag)
		}
	}

	if flags.project == "" {
		return flags, errors.New("--project is required")
	}

	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return flags, resErr
	}
	flags.workspace = resolved
	return flags, nil
}

func parseSinceDate(sinceStr string, now time.Time) (time.Time, error) {
	if sinceStr == "" {
		return time.Time{}, nil
	}
	if strings.HasSuffix(sinceStr, "d") {
		daysStr := strings.TrimSuffix(sinceStr, "d")
		n, err := strconv.Atoi(daysStr)
		if err != nil || n <= 0 {
			return time.Time{}, fmt.Errorf("invalid --since duration %q: expected format like 14d", sinceStr)
		}
		return now.AddDate(0, 0, -n), nil
	}
	t, err := time.Parse("2006-01-02", sinceStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since date %q: expected YYYY-MM-DD or Nd", sinceStr)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
}

func runTelemetryImportTranscripts(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags, err := parseImportTranscriptsFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub telemetry import-transcripts --project <dir> [--since <Nd|YYYY-MM-DD>]`.")
	}

	now := time.Now().UTC()
	since, err := parseSinceDate(flags.sinceStr, now)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Pass `--since <Nd|YYYY-MM-DD>` with a valid duration or date.")
	}

	service := app.TranscriptImportService{}
	result, err := service.Import(ctx, flags.workspace, app.ImportInput{
		Project: flags.project,
		Since:   since,
	})
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Check arguments and project directory path.")
	}

	if flags.jsonOutput {
		return writeTelemetryJSON(stdout, stderr, result)
	}

	p := termui.New(stdout)
	p.Heading("Transcript Import")
	p.Line(result.Summary)
	if len(result.PerTool) > 0 {
		for tool, count := range result.PerTool {
			p.Line(fmt.Sprintf("  %s: %d", tool, count))
		}
	}
	p.Blank()
	p.Line("Privacy notice: Stored only tool name, skill ID, timestamp, and a session hash. Tasks and arguments were not read or stored.")
	return 0
}
