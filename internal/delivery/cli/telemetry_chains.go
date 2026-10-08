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

type chainsFlags struct {
	workspace  string
	sinceStr   string
	kind       string
	jsonOutput bool
}

func parseChainsFlags(args []string) (chainsFlags, error) {
	flags := chainsFlags{sinceStr: "7d"}
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
		case "--workspace", "--since", "--kind":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return flags, fmt.Errorf("%s requires a value", flag)
			}
			val := args[i+1]
			if len(val) == 0 || len(val) > maxDeliveryValueSize {
				return flags, fmt.Errorf("value for %s must be between 1 and %d bytes", flag, maxDeliveryValueSize)
			}
			switch flag {
			case "--workspace":
				flags.workspace = val
			case "--since":
				flags.sinceStr = val
			case "--kind":
				if val != "override" && val != "after_no_skill" && val != "reformulation" {
					return flags, fmt.Errorf("--kind must be override, after_no_skill, or reformulation")
				}
				flags.kind = val
			}
			i++
		default:
			return flags, fmt.Errorf("unrecognized flag %q", flag)
		}
	}
	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return flags, resErr
	}
	flags.workspace = resolved
	return flags, nil
}

func parseChainsSince(sinceStr string) (time.Time, error) {
	now := time.Now().UTC()
	if strings.HasSuffix(sinceStr, "d") {
		daysStr := strings.TrimSuffix(sinceStr, "d")
		n, err := strconv.Atoi(daysStr)
		if err != nil || n <= 0 {
			return time.Time{}, fmt.Errorf("invalid --since duration %q: expected format like 7d", sinceStr)
		}
		return now.AddDate(0, 0, -n), nil
	}
	t, err := time.Parse("2006-01-02", sinceStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since date %q: expected YYYY-MM-DD or Nd", sinceStr)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
}

func runTelemetryChains(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags, err := parseChainsFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub telemetry chains [--since <Nd|YYYY-MM-DD>] [--kind override|after_no_skill|reformulation]`.")
	}

	since, err := parseChainsSince(flags.sinceStr)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Pass `--since <Nd|YYYY-MM-DD>`.")
	}

	usageService := app.UsageService{}
	chains, err := usageService.Chains(ctx, flags.workspace, since, flags.kind)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Check arguments and workspace health with `skillhub doctor`.")
	}

	if flags.jsonOutput {
		return writeTelemetryJSON(stdout, stderr, chains)
	}

	p := termui.New(stdout)
	title := "Disagreement Chains"
	if flags.kind != "" {
		title = fmt.Sprintf("Disagreement Chains (%s)", flags.kind)
	}
	p.Heading(title)

	if len(chains) == 0 {
		p.Line("No disagreement chains found in the selected window.")
		return 0
	}

	headers := []string{"CHAIN ID", "KIND", "CLIENT", "RECOMMENDED", "LOADED", "AT"}
	var rows [][]string
	for _, ch := range chains {
		chainID := ch.ChainID
		if len(chainID) > 24 {
			chainID = chainID[:24] + "…"
		}
		rows = append(rows, []string{
			chainID,
			ch.Kind,
			ch.Client,
			ch.RecommendedSkill,
			ch.LoadedSkill,
			ch.OccurredAt,
		})
	}
	p.Table(headers, rows)
	return 0
}
