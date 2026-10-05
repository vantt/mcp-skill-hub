package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

type routingFlags struct {
	workspace        string
	noSkillPath      string
	policyPath       string
	minPrecision     *float64
	minRecall        *float64
	minNoSkillRecall *float64
	maxFPR           *float64
	jsonOutput       bool
}

func parseRoutingFlags(args []string) (routingFlags, error) {
	flags := routingFlags{}
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
		case "--workspace", "--no-skill", "--policy", "--min-precision", "--min-recall", "--min-no-skill-recall", "--max-fpr":
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
			case "--no-skill":
				if _, err := os.Stat(val); err != nil {
					return flags, fmt.Errorf("--no-skill file %q not found", val)
				}
				flags.noSkillPath = val
			case "--policy":
				if _, err := os.Stat(val); err != nil {
					return flags, fmt.Errorf("--policy file %q not found", val)
				}
				flags.policyPath = val
			case "--min-precision":
				f, err := strconv.ParseFloat(val, 64)
				if err != nil || f < 0 || f > 1 {
					return flags, fmt.Errorf("--min-precision must be a float between 0.0 and 1.0")
				}
				flags.minPrecision = &f
			case "--min-recall":
				f, err := strconv.ParseFloat(val, 64)
				if err != nil || f < 0 || f > 1 {
					return flags, fmt.Errorf("--min-recall must be a float between 0.0 and 1.0")
				}
				flags.minRecall = &f
			case "--min-no-skill-recall":
				f, err := strconv.ParseFloat(val, 64)
				if err != nil || f < 0 || f > 1 {
					return flags, fmt.Errorf("--min-no-skill-recall must be a float between 0.0 and 1.0")
				}
				flags.minNoSkillRecall = &f
			case "--max-fpr":
				f, err := strconv.ParseFloat(val, 64)
				if err != nil || f < 0 || f > 1 {
					return flags, fmt.Errorf("--max-fpr must be a float between 0.0 and 1.0")
				}
				flags.maxFPR = &f
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

func formatEvalPercent(val *float64) string {
	if val == nil {
		return "null"
	}
	return fmt.Sprintf("%.1f%%", *val*100)
}

func renderRoutingReport(stdout io.Writer, report app.RoutingEvalReport) {
	p := termui.New(stdout)
	p.Heading("Routing Evaluation Summary")
	p.Fields(
		termui.Field{Label: "Total Positives", Value: fmt.Sprintf("%d", report.TotalPositives)},
		termui.Field{Label: "Total Counters", Value: fmt.Sprintf("%d", report.TotalCounters)},
		termui.Field{Label: "Total No-Skill", Value: fmt.Sprintf("%d", report.TotalNoSkill)},
	)

	p.Heading("Metrics")
	p.Fields(
		termui.Field{Label: "Precision@1", Value: formatEvalPercent(report.Precision1)},
		termui.Field{Label: "Recall", Value: formatEvalPercent(report.Recall)},
		termui.Field{Label: "No-Skill Recall", Value: formatEvalPercent(report.NoSkillRecall)},
		termui.Field{Label: "No-Skill Precision", Value: formatEvalPercent(report.NoSkillPrecision)},
		termui.Field{Label: "False Positive Rate", Value: formatEvalPercent(report.FalsePositiveRate)},
	)

	p.Blank()
	if report.PassedGate {
		p.Line("Gate: PASS")
	} else {
		p.Line("Gate: FAIL")
		if len(report.GateFailures) > 0 {
			p.Heading("Gate Failures:")
			p.Bullets(report.GateFailures...)
		}
	}

	if len(report.FailedCases) > 0 {
		p.Heading(fmt.Sprintf("Failing Cases (%d):", len(report.FailedCases)))
		var lines []string
		for _, fc := range report.FailedCases {
			lines = append(lines, fmt.Sprintf("[%s] %s #%d %q -> status=%s, primary=%s",
				fc.Kind, fc.SkillID, fc.Index, fc.Phrase, fc.GotStatus, fc.GotPrimary))
		}
		p.Bullets(lines...)
	}
}

func runEvaluationRouting(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	const usage = "Run `skillhub eval routing [--workspace <path>] [--no-skill <file>] [--policy <file>] [--min-precision F] [--min-recall F] [--min-no-skill-recall F] [--max-fpr F] [--json]`."
	flags, err := parseRoutingFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), usage)
	}

	service := app.RoutingEvalService{}
	report, err := service.Run(ctx, app.RoutingEvalQuery{
		WorkspacePath:    flags.workspace,
		NoSkillPath:      flags.noSkillPath,
		PolicyPath:       flags.policyPath,
		MinPrecision:     flags.minPrecision,
		MinRecall:        flags.minRecall,
		MinNoSkillRecall: flags.minNoSkillRecall,
		MaxFPR:           flags.maxFPR,
	})
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), usage)
	}

	if flags.jsonOutput {
		if err := writeJSON(stdout, report); err != nil {
			p := termui.New(stderr)
			p.Error("Unable to write evaluation result.", err.Error(), "Check output destination.")
			return 1
		}
	} else {
		renderRoutingReport(stdout, report)
	}

	if !report.PassedGate {
		return 1
	}
	return 0
}
