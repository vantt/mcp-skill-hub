package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
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
		case "suggest":
			return runCasesSuggest(ctx, args[1:], stdout, stderr)
		case "dismiss":
			return runCasesDismiss(ctx, args[1:], stdout, stderr)
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
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub telemetry cases [enable|disable|status|suggest|dismiss|list] [--since <YYYY-MM-DD|Nd>] [--kind override|after_no_skill|...]`.")
	}

	var since time.Time
	if flags.sinceStr != "" {
		parsedSince, err := parseSince(flags.sinceStr, time.Now().UTC())
		if err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), sinceFix)
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

const (
	casesSuggestUsage = "Run `skillhub telemetry cases suggest [--since <YYYY-MM-DD|Nd>] [--skill <skill-id>] [--min-events <1-100>] [--workspace <path>] [--json]`."
	casesDismissUsage = "Run `skillhub telemetry cases dismiss <suggestion-id> [--workspace <path>] [--json]`; `skillhub telemetry cases suggest` lists the IDs."
)

type suggestFlags struct {
	workspace, since, skill string
	minEvents               int
	jsonOutput              bool
}

func parseSuggestFlags(args []string) (suggestFlags, error) {
	flags := suggestFlags{since: "30d", minEvents: 1}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--json" {
			flags.jsonOutput = true
			continue
		}
		if arg != "--workspace" && arg != "--since" && arg != "--skill" && arg != "--min-events" {
			return flags, fmt.Errorf("unknown argument %q", arg)
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
			return flags, fmt.Errorf("%s requires a value", arg)
		}
		i++
		switch arg {
		case "--workspace":
			flags.workspace = args[i]
		case "--since":
			flags.since = args[i]
		case "--skill":
			flags.skill = args[i]
		case "--min-events":
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 || n > 100 {
				return flags, fmt.Errorf("--min-events must be an integer from 1 to 100, got %q", args[i])
			}
			flags.minEvents = n
		}
	}
	resolved, err := resolveWorkspace(flags.workspace)
	if err != nil {
		return flags, err
	}
	flags.workspace = resolved
	return flags, nil
}

// writeCasesFlagError reports a flag or workspace problem of a cases command.
func writeCasesFlagError(stdout, stderr io.Writer, args []string, err error, fix string) int {
	var resErr *WorkspaceResolutionError
	if errors.As(err, &resErr) {
		return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
	}
	return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), fix)
}

func runCasesSuggest(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags, err := parseSuggestFlags(args)
	if err != nil {
		return writeCasesFlagError(stdout, stderr, args, err, casesSuggestUsage)
	}
	since, err := parseSince(flags.since, time.Now().UTC())
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), sinceFix)
	}
	report, err := app.RoutingSuggestionService{}.Suggest(ctx, flags.workspace, app.RoutingSuggestionQuery{
		Since: since, SkillID: flags.skill, MinEvents: flags.minEvents,
	})
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "failed to mine routing suggestions: "+err.Error(), "Check arguments and workspace health with `skillhub doctor`.")
	}
	return writeResult(stdout, stderr, flags.jsonOutput, report, func(p *termui.Printer) {
		printRoutingSuggestions(p, report)
	})
}

// printRoutingSuggestions renders a mining report. Phrasings come from agents,
// so each is quoted onto one line, and the printer escapes anything left. The
// Next line holds only IDs, so it is safe to copy.
func printRoutingSuggestions(p *termui.Printer, report app.RoutingSuggestionReport) {
	p.Heading("Routing suggestions")
	if !report.CaseJournalEnabled {
		p.Line("The case journal is off, so there is no task text to learn from. Run `skillhub telemetry cases enable`; only sessions after that count.")
		return
	}
	suggestions := append([]app.RoutingSuggestion(nil), report.Suggestions...)
	sort.SliceStable(suggestions, func(i, j int) bool { return suggestions[i].SkillID < suggestions[j].SkillID })
	if len(suggestions) == 0 {
		p.Line("No routing suggestions in the selected window.")
	}
	group := ""
	for _, suggestion := range suggestions {
		if suggestion.SkillID != group {
			group = suggestion.SkillID
			p.Blank()
			p.Line("Skill " + group)
		}
		p.Blank()
		p.Line(fmt.Sprintf("%s  %s  %s", suggestion.ID, suggestion.Field, strconv.Quote(suggestion.Text)))
		p.Fields(
			termui.Field{Label: "Why", Value: suggestion.Evidence.Reason},
			termui.Field{Label: "Evidence", Value: suggestionEvidenceLine(suggestion.Evidence)},
		)
		p.Raw("Next: skillhub skill edit " + shellquote.Arg(suggestion.SkillID) + " --from-suggestion " + shellquote.Arg(suggestion.ID) + "\n")
	}
	p.Blank()
	if line := skippedLine(report.Skipped); line != "" {
		p.Line(line)
	}
	if report.RawEventsSince != "" && len(report.RawEventsSince) >= 10 && report.RawEventsSince[:10] > report.Window.From {
		p.Line("Usage events before " + report.RawEventsSince[:10] + " have expired, so evidence that needs them may be missing.")
	}
}

func suggestionEvidenceLine(evidence app.RoutingSuggestionEvidence) string {
	kinds := make([]string, 0, len(evidence.Kinds))
	for kind, count := range evidence.Kinds {
		kinds = append(kinds, fmt.Sprintf("%s %d", kind, count))
	}
	sort.Strings(kinds)
	line := fmt.Sprintf("%d event(s) in %d session(s), %s..%s", evidence.Events, evidence.Sessions, evidence.FirstSeen, evidence.LastSeen)
	if len(kinds) > 0 {
		line += "; " + strings.Join(kinds, ", ")
	}
	if evidence.OtherSkill != "" {
		line += "; other skill " + evidence.OtherSkill
	}
	return line
}

func skippedLine(skipped map[string]int) string {
	reasons := make([]string, 0, len(skipped))
	for reason, count := range skipped {
		if count > 0 {
			reasons = append(reasons, fmt.Sprintf("%s %d", reason, count))
		}
	}
	if len(reasons) == 0 {
		return ""
	}
	sort.Strings(reasons)
	return "Skipped: " + strings.Join(reasons, ", ")
}

func runCasesDismiss(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "dismiss requires a suggestion ID", casesDismissUsage)
	}
	id := args[0]
	flags, err := parseCasesFlags(args[1:])
	if err != nil {
		return writeCasesFlagError(stdout, stderr, args, err, casesDismissUsage)
	}
	if err := (app.RoutingSuggestionService{}).Dismiss(ctx, flags.workspace, id); err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), casesDismissUsage)
	}
	return writeResult(stdout, stderr, flags.jsonOutput, map[string]any{"dismissed": id}, func(p *termui.Printer) {
		p.Line(fmt.Sprintf("Suggestion %s dismissed; it will not be suggested again on this machine.", id))
	})
}
