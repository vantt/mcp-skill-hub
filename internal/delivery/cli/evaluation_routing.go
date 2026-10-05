package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

type routingEvalFlags struct {
	workspacePath    string
	noSkillPath      string
	policyPath       string
	minPrecision     *float64
	minRecall        *float64
	minNoSkillRecall *float64
	maxFPR           *float64
	jsonOutput       bool
}

const routingUsage = "Run `skillhub eval routing [--workspace <path>] [--no-skill <file>] [--policy <file>] [--min-precision F] [--min-recall F] [--min-no-skill-recall F] [--max-fpr F] [--json]`."

func parseRoutingEvalFlags(args []string) (routingEvalFlags, error) {
	var result routingEvalFlags
	if len(args) > maxDeliveryArgs {
		return result, errors.New("too many arguments")
	}
	seen := map[string]bool{}
	for index := 0; index < len(args); index++ {
		flag := args[index]
		if seen[flag] {
			return result, fmt.Errorf("%s may only be provided once", flag)
		}
		seen[flag] = true
		switch flag {
		case "--json":
			result.jsonOutput = true
		case "--workspace", "--no-skill", "--policy", "--min-precision", "--min-recall", "--min-no-skill-recall", "--max-fpr":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return result, fmt.Errorf("%s requires a value", flag)
			}
			value := args[index+1]
			if len(value) == 0 || len(value) > maxDeliveryValueSize {
				return result, fmt.Errorf("%s value must be between 1 and %d bytes", flag, maxDeliveryValueSize)
			}
			switch flag {
			case "--workspace":
				result.workspacePath = value
			case "--no-skill":
				result.noSkillPath = value
			case "--policy":
				result.policyPath = value
			case "--min-precision":
				val, err := strconv.ParseFloat(value, 64)
				if err != nil || val < 0.0 || val > 1.0 {
					return result, fmt.Errorf("--min-precision must be a float between 0.0 and 1.0")
				}
				result.minPrecision = &val
			case "--min-recall":
				val, err := strconv.ParseFloat(value, 64)
				if err != nil || val < 0.0 || val > 1.0 {
					return result, fmt.Errorf("--min-recall must be a float between 0.0 and 1.0")
				}
				result.minRecall = &val
			case "--min-no-skill-recall":
				val, err := strconv.ParseFloat(value, 64)
				if err != nil || val < 0.0 || val > 1.0 {
					return result, fmt.Errorf("--min-no-skill-recall must be a float between 0.0 and 1.0")
				}
				result.minNoSkillRecall = &val
			case "--max-fpr":
				val, err := strconv.ParseFloat(value, 64)
				if err != nil || val < 0.0 || val > 1.0 {
					return result, fmt.Errorf("--max-fpr must be a float between 0.0 and 1.0")
				}
				result.maxFPR = &val
			}
			index++
		default:
			return result, fmt.Errorf("unknown argument %q", flag)
		}
	}
	resolved, resErr := resolveWorkspace(result.workspacePath)
	if resErr != nil {
		return result, resErr
	}
	result.workspacePath = resolved
	return result, nil
}

func runEvaluationRouting(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags, err := parseRoutingEvalFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), routingUsage)
	}

	report, err := app.EvaluateRouting(ctx, app.RoutingEvalOptions{
		WorkspacePath: flags.workspacePath,
		NoSkillFile:   flags.noSkillPath,
		PolicyFile:    flags.policyPath,
	})
	if err != nil {
		return writeWorkspaceResult(app.Result{}, err, stdout, stderr, flags.jsonOutput)
	}

	thresholdFailed := false
	var failedReasons []string

	formatRate := func(r *float64) string {
		if r == nil {
			return "null"
		}
		return fmt.Sprintf("%.4f", *r)
	}

	if flags.minPrecision != nil {
		if report.Precision == nil || *report.Precision < *flags.minPrecision {
			thresholdFailed = true
			failedReasons = append(failedReasons, fmt.Sprintf("precision %s < min %f", formatRate(report.Precision), *flags.minPrecision))
		}
	}
	if flags.minRecall != nil {
		if report.Recall == nil || *report.Recall < *flags.minRecall {
			thresholdFailed = true
			failedReasons = append(failedReasons, fmt.Sprintf("recall %s < min %f", formatRate(report.Recall), *flags.minRecall))
		}
	}
	if flags.minNoSkillRecall != nil {
		if report.NoSkillRecall == nil || *report.NoSkillRecall < *flags.minNoSkillRecall {
			thresholdFailed = true
			failedReasons = append(failedReasons, fmt.Sprintf("no-skill recall %s < min %f", formatRate(report.NoSkillRecall), *flags.minNoSkillRecall))
		}
	}
	if flags.maxFPR != nil {
		if report.FalsePositiveRate == nil || *report.FalsePositiveRate > *flags.maxFPR {
			thresholdFailed = true
			failedReasons = append(failedReasons, fmt.Sprintf("false positive rate %s > max %f", formatRate(report.FalsePositiveRate), *flags.maxFPR))
		}
	}

	if flags.jsonOutput {
		encoded, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			return writeWorkspaceResult(app.Result{}, marshalErr, stdout, stderr, true)
		}
		_, _ = fmt.Fprintln(stdout, string(encoded))
		if thresholdFailed {
			return 1
		}
		return 0
	}

	// Human output
	fmt.Fprintf(stdout, "Routing Evaluation Summary:\n")
	fmt.Fprintf(stdout, "  Total cases:        %d\n", report.TotalCases)
	fmt.Fprintf(stdout, "  Positive cases (P): %d\n", report.PositiveCases)
	fmt.Fprintf(stdout, "  Counter cases (N):  %d\n", report.CounterCases)
	fmt.Fprintf(stdout, "  No-skill cases (Z): %d\n", report.NoSkillCases)
	fmt.Fprintf(stdout, "\nMetrics:\n")
	fmt.Fprintf(stdout, "  Precision@1:        %s\n", formatRate(report.Precision))
	fmt.Fprintf(stdout, "  Recall:             %s\n", formatRate(report.Recall))
	fmt.Fprintf(stdout, "  No-Skill Recall:    %s\n", formatRate(report.NoSkillRecall))
	fmt.Fprintf(stdout, "  No-Skill Precision: %s\n", formatRate(report.NoSkillPrecision))
	fmt.Fprintf(stdout, "  False Positive Rate:%s\n", formatRate(report.FalsePositiveRate))

	if len(report.SkillMetrics) > 0 {
		fmt.Fprintf(stdout, "\nPer-Skill Recall:\n")
		for _, sm := range report.SkillMetrics {
			fmt.Fprintf(stdout, "  %-30s %d/%d (%s)\n", sm.SkillID, sm.Correct, sm.Total, formatRate(sm.Recall))
		}
	}

	if len(report.Failures) > 0 {
		fmt.Fprintf(stdout, "\nFailures (%d):\n", len(report.Failures))
		for _, f := range report.Failures {
			if f.Kind == "no_skill" {
				fmt.Fprintf(stdout, "  [no_skill] #%d: %q (got status=%s, primary=%s)\n", f.Index, f.Phrase, f.GotStatus, f.GotPrimary)
			} else {
				fmt.Fprintf(stdout, "  [%s] %s #%d: %q (got status=%s, primary=%s)\n", f.Kind, f.SkillID, f.Index, f.Phrase, f.GotStatus, f.GotPrimary)
			}
		}
	}

	if thresholdFailed {
		fmt.Fprintf(stderr, "\nEvaluation failed quality thresholds:\n")
		for _, r := range failedReasons {
			fmt.Fprintf(stderr, "  - %s\n", r)
		}
		return 1
	}

	return 0
}
