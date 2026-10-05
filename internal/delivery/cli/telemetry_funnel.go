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

type funnelFlags struct {
	workspace  string
	sinceStr   string
	untilStr   string
	skillID    string
	jsonOutput bool
}

func parseFunnelFlags(args []string) (funnelFlags, error) {
	flags := funnelFlags{}
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
		case "--workspace", "--since", "--until", "--skill":
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
			case "--skill":
				flags.skillID = val
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

func parseFunnelDates(sinceStr, untilStr string) (time.Time, time.Time, error) {
	var until time.Time
	now := time.Now().UTC()
	if untilStr != "" {
		t, err := time.Parse("2006-01-02", untilStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --until date %q: expected YYYY-MM-DD", untilStr)
		}
		until = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	} else {
		until = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	var since time.Time
	if sinceStr != "" {
		if strings.HasSuffix(sinceStr, "d") {
			daysStr := strings.TrimSuffix(sinceStr, "d")
			n, err := strconv.Atoi(daysStr)
			if err != nil || n <= 0 {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid --since duration %q: expected format like 30d", sinceStr)
			}
			since = until.AddDate(0, 0, -n)
		} else {
			t, err := time.Parse("2006-01-02", sinceStr)
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid --since date %q: expected YYYY-MM-DD or Nd", sinceStr)
			}
			since = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		}
	} else {
		since = until.AddDate(0, 0, -30)
	}

	if since.After(until) {
		return time.Time{}, time.Time{}, errors.New("since date must not be after until date")
	}

	return since, until, nil
}

func formatRate(rate *float64) string {
	if rate == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.1f%%", *rate*100)
}

func renderFunnel(stdout io.Writer, report app.FunnelReport, singleSkill bool) {
	p := termui.New(stdout)
	p.Heading(fmt.Sprintf("Funnel Report (%s to %s, %d days)", report.Window.Since, report.Window.Until, report.Window.Days))

	if report.Overall != nil && !singleSkill {
		p.Heading("Overall Summary")
		o := report.Overall
		resStatus := fmt.Sprintf("%d resolved, %d no skill, %d needs context, %d failed",
			o.Resolutions["resolved"], o.Resolutions["no_skill"], o.Resolutions["needs_context"], o.Resolutions["failed"])
		p.Fields(
			termui.Field{Label: "Resolutions", Value: fmt.Sprintf("%d total (%s)", o.TotalResolutions, resStatus)},
			termui.Field{Label: "Recommendations", Value: fmt.Sprintf("%d primary, %d supporting", o.RecommendedPrimary, o.RecommendedSupporting)},
			termui.Field{Label: "Activations", Value: fmt.Sprintf("%d total (%d recommended, %d override, %d unsolicited)",
				o.TotalActivations, o.Activations["recommended"], o.Overrides, o.Unsolicited)},
			termui.Field{Label: "Acceptance Rate", Value: formatRate(o.AcceptanceRate)},
			termui.Field{Label: "Blocked by Review", Value: fmt.Sprintf("%d", o.BlockedByReview)},
			termui.Field{Label: "Setup Failures", Value: fmt.Sprintf("%d (%s)", o.SetupFailed, formatRate(o.SetupFailedRate))},
			termui.Field{Label: "Doctor Runs", Value: fmt.Sprintf("%d (%s failure rate)", o.TotalDoctor, formatRate(o.DoctorFailureRate))},
			termui.Field{Label: "Negative Feedback", Value: fmt.Sprintf("%d (%d after load)", o.NegativeFeedback, o.NegativeAfterLoad)},
		)
	}

	p.Heading("Skills")
	headers := []string{"SKILL", "PRIMARY", "SUPPORTING", "ACTIVATIONS", "ACCEPTANCE", "BLOCKED", "SETUP FAIL"}
	var rows [][]string

	skills := report.Skills
	if !singleSkill && len(skills) > 20 {
		skills = skills[:20]
	}

	for _, s := range skills {
		rows = append(rows, []string{
			s.SkillID,
			fmt.Sprintf("%d", s.RecommendedPrimary),
			fmt.Sprintf("%d", s.RecommendedSupporting),
			fmt.Sprintf("%d", s.TotalActivations),
			formatRate(s.AcceptanceRate),
			fmt.Sprintf("%d", s.BlockedByReview),
			fmt.Sprintf("%d", s.SetupFailed),
		})
	}
	p.Table(headers, rows)

	if !singleSkill {
		if len(report.BlockedByReview) > 0 {
			p.Heading("Blocked by Review (approve to unlock demand)")
			p.Bullets(report.BlockedByReview...)
		}
		if len(report.RecommendedNeverActivated) > 0 {
			p.Heading("Recommended Never Activated")
			p.Bullets(report.RecommendedNeverActivated...)
		}
		if len(report.DeadSkills) > 0 {
			p.Heading("Dead Skills (active with no recommendations or loads)")
			p.Bullets(report.DeadSkills...)
		}
		if len(report.SetupFailures) > 0 {
			p.Heading("Setup Failures")
			p.Bullets(report.SetupFailures...)
		}
		if len(report.NegativeAfterLoad) > 0 {
			p.Heading("Negative Feedback After Load")
			p.Bullets(report.NegativeAfterLoad...)
		}
	}
}

func runTelemetryFunnel(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags, err := parseFunnelFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub telemetry funnel [--since <Nd|YYYY-MM-DD>] [--until <YYYY-MM-DD>] [--skill <id>]`.")
	}

	since, until, err := parseFunnelDates(flags.sinceStr, flags.untilStr)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Pass `--since <Nd|YYYY-MM-DD>` and `--until <YYYY-MM-DD>`.")
	}

	usageService := app.UsageService{}
	report, err := usageService.Funnel(ctx, flags.workspace, app.FunnelQuery{
		Since:   since,
		Until:   until,
		SkillID: flags.skillID,
	})
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Check arguments and workspace health with `skillhub doctor`.")
	}

	if flags.jsonOutput {
		return writeTelemetryJSON(stdout, stderr, report)
	}

	renderFunnel(stdout, report, flags.skillID != "")
	return 0
}
