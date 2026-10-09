package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

type baselineFlags struct {
	workspace  string
	sinceStr   string
	untilStr   string
	minChains  int
	writePath  string
	jsonOutput bool
}

func parseBaselineFlags(args []string) (baselineFlags, error) {
	flags := baselineFlags{
		sinceStr:  "30d",
		minChains: 30,
	}
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
		case "--workspace", "--since", "--until", "--min-chains", "--write":
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
			case "--until":
				flags.untilStr = val
			case "--write":
				flags.writePath = val
			case "--min-chains":
				n, err := strconv.Atoi(val)
				if err != nil || n <= 0 {
					return flags, fmt.Errorf("invalid --min-chains %q: must be a positive integer", val)
				}
				flags.minChains = n
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

func runTelemetryBaseline(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags, err := parseBaselineFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub telemetry baseline [--since 30d] [--min-chains N] [--json] [--write <path>]`.")
	}

	since, until, err := parseFunnelDates(flags.sinceStr, flags.untilStr)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Use dates formatted as YYYY-MM-DD or durations like 30d.")
	}

	service := app.UsageService{}
	report, err := service.Baseline(ctx, flags.workspace, app.BaselineQuery{
		Since:     since,
		Until:     until,
		MinChains: flags.minChains,
	})
	if err != nil {
		return writeTelemetryError(err, flags.workspace, stdout, stderr, flags.jsonOutput)
	}

	if flags.writePath != "" {
		if writeErr := writeBaselineJSONFile(flags.writePath, report); writeErr != nil {
			p := termui.New(stderr)
			p.Error("Unable to write baseline report to file.", writeErr.Error(), "Check directory permissions and target path.")
			return 1
		}
	}

	if flags.jsonOutput {
		return writeTelemetryJSON(stdout, stderr, report)
	}

	renderBaselineTerminal(stdout, report, flags.writePath)
	return 0
}

func writeBaselineJSONFile(path string, report app.BaselineReport) error {
	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(cleanPath, encoded, 0o644)
}

func formatBaselineRate(m app.RateMetric) string {
	if m.Status == "unknown" || m.Rate == nil {
		return fmt.Sprintf("unknown (%d/%d)", m.Numerator, m.Denominator)
	}
	return fmt.Sprintf("%.1f%% (%d/%d)", *m.Rate*100.0, m.Numerator, m.Denominator)
}

func renderBaselineTerminal(stdout io.Writer, report app.BaselineReport, writtenPath string) {
	p := termui.New(stdout)
	p.Line(fmt.Sprintf("=== Telemetry Baseline (Since: %s, Until: %s) ===", report.Since[:10], report.Until[:10]))
	if report.ActiveSnapshot != "" {
		p.Line(fmt.Sprintf("Active Baseline Snapshot: %s", report.ActiveSnapshot))
	} else {
		p.Line("Active Baseline Snapshot: none")
	}
	if report.RawRetentionPruned {
		p.Line("Warning: baseline window extends beyond 30-day raw retention; older raw events may have been pruned.")
	}
	p.Line("")

	if len(report.Buckets) == 0 {
		p.Line("No telemetry events found in the requested window.")
		if writtenPath != "" {
			p.Line(fmt.Sprintf("Baseline report written to %s", writtenPath))
		}
		return
	}

	for _, b := range report.Buckets {
		renderBaselineBucket(p, b)
	}

	if writtenPath != "" {
		p.Line(fmt.Sprintf("Baseline report written to %s", writtenPath))
	}
}

func renderBaselineBucket(p *termui.Printer, b app.BaselineBucket) {
	p.Line(fmt.Sprintf("[%s x %s] Status: %s", b.CatalogSnapshot, b.Client, b.Status))
	if b.Sufficiency.Verdict == "sufficient" {
		p.Line(fmt.Sprintf("  Sufficiency:   sufficient (resolved: %d/%d, no_skill: %d/%d)",
			b.Sufficiency.ResolvedChains, b.Sufficiency.MinChains,
			b.Sufficiency.NoSkillChains, b.Sufficiency.MinChains))
	} else {
		p.Line(fmt.Sprintf("  Sufficiency:   insufficient (resolved: %d/%d [missing %d], no_skill: %d/%d [missing %d])",
			b.Sufficiency.ResolvedChains, b.Sufficiency.MinChains, b.Sufficiency.MissingResolved,
			b.Sufficiency.NoSkillChains, b.Sufficiency.MinChains, b.Sufficiency.MissingNoSkill))
	}
	p.Line(fmt.Sprintf("  Chains:        total: %d, resolved: %d, no_skill: %d, needs_context: %d, already_covered: %d, failed: %d",
		b.Metrics.TotalChains, b.Metrics.ChainsResolved, b.Metrics.ChainsNoSkill,
		b.Metrics.ChainsNeedsContext, b.Metrics.ChainsAlreadyCovered, b.Metrics.ChainsFailed))
	p.Line("  O5 Rates:")
	p.Line(fmt.Sprintf("    acceptance_rate:          %s", formatBaselineRate(b.Metrics.AcceptanceRate)))
	p.Line(fmt.Sprintf("    override_rate:            %s", formatBaselineRate(b.Metrics.OverrideRate)))
	p.Line(fmt.Sprintf("    false_no_skill_rate:       %s", formatBaselineRate(b.Metrics.FalseNoSkillRate)))
	p.Line(fmt.Sprintf("    true_no_skill:            %s", formatBaselineRate(b.Metrics.TrueNoSkill)))
	p.Line(fmt.Sprintf("    reformulation_rate:       %s", formatBaselineRate(b.Metrics.ReformulationRate)))
	p.Line(fmt.Sprintf("    ignore_rate:              %s", formatBaselineRate(b.Metrics.IgnoreRate)))
	p.Line(fmt.Sprintf("    needs_context_answer_rate:%s", formatBaselineRate(b.Metrics.NeedsContextAnswerRate)))
	p.Line(fmt.Sprintf("    bypass_rate:              %s", formatBaselineRate(b.Metrics.BypassRate)))
	p.Line(fmt.Sprintf("    negative_after_load:      %s", formatBaselineRate(b.Metrics.NegativeAfterLoad)))
	p.Line("  Attribution & Loads:")
	p.Line(fmt.Sprintf("    total_loads:              %d", b.TotalLoads))
	p.Line(fmt.Sprintf("    unsolicited_loads:        %d", b.UnsolicitedLoads))
	p.Line(fmt.Sprintf("    unsolicited_share:        %s", formatBaselineRate(b.UnsolicitedShare)))
	if b.FirstValidDay != "" {
		p.Line(fmt.Sprintf("  First Valid Day:            %s", b.FirstValidDay))
	}
	p.Line("")
}
