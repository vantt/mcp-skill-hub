package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
	"github.com/vantt/mcp-skill-hub/internal/userconfig"
)

// noRoleMessage is what status and doctor say in a folder that is neither a
// hub nor a connected project.
const noRoleMessage = "This folder is not a Skill Hub hub or a connected project."

// healthReport finds the role of dir, the folder status or doctor runs in,
// and narrows it to the hub --workspace or SKILLHUB_WORKSPACE names.
func healthReport(dir, workspaceFlag string) (app.RoleReport, *WorkspaceResolutionError) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	config, err := userconfig.Default().Load()
	if err != nil {
		config = userconfig.Config{}
	}
	report := app.DetectRole(dir, home, config)
	explicit := strings.TrimSpace(workspaceFlag)
	if explicit == "" {
		explicit = strings.TrimSpace(os.Getenv(workspaceEnvVar))
	}
	if explicit != "" {
		root, resErr := validateExplicitWorkspace(explicit)
		if resErr != nil {
			return report, resErr
		}
		report = report.WithWorkspace(root)
	}
	return report, nil
}

func currentDir(stdout, stderr io.Writer, jsonOutput bool, command string) (string, int, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", writeInvalidRequest(stdout, stderr, jsonOutput, "The current folder cannot be determined: "+err.Error(), "Run `skillhub "+command+"` in the hub or in a connected project."), false
	}
	return dir, 0, true
}

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	dir, code, ok := currentDir(stdout, stderr, hasJSONFlag(args), "status")
	if !ok {
		return code
	}
	return runStatusIn(ctx, dir, args, stdout, stderr)
}

// statusJSON is `status --json`: the curation home of the hub
// (schemas/curation-home-v1.schema.json) with the role of the folder and the
// sections of its connections added.
type statusJSON struct {
	app.CurationHome
	Role           string        `json:"role"`
	ClientSections []app.Section `json:"client_sections,omitempty"`
}

// runStatusIn runs status for dir, which stands for the current folder.
// Tests call it directly so they never change the process working directory.
func runStatusIn(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) int {
	workspaceFlag, jsonOutput, quiet, err := statusFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub status [--workspace <path>] [--json|--quiet]`.")
	}
	report, resErr := healthReport(dir, workspaceFlag)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
	}
	sections := app.Run(ctx, report, app.DepthLight)
	hub := report.HealthHub()
	home := emptyHome("This folder is not a hub, and no hub on this machine applies to it.")
	if hub != "" {
		home, err = (app.CurationService{NoWait: true}).GetCurationHome(ctx, hub)
		if errors.Is(err, mutation.ErrWorkspaceBusy) {
			home, err = emptyHome("The hub is busy with another change."), nil
			home.Status = app.StatusActionRequired
			sections = markBusy(sections)
		}
		if err != nil {
			return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
		}
	} else if remote := app.FindRemoteEntries(report); len(remote) > 0 {
		home.Summary = "This project uses the hub at " + remote[0].HubURL + "; skills are curated on the hub machine."
	}
	next := app.StatusNext(report, sections, app.HubInventory{Known: hub != "" && home.CountsKnown, Skills: home.TotalSkills})
	home.SuggestedActions = []app.Action{{Label: next.Label, Command: next.Command}}
	if app.HasProblems(sections) || len(sections) == 0 {
		if home.Status == app.StatusOK || home.Status == app.StatusReady {
			home.Status = app.StatusActionRequired
		}
		if home.Summary == "Skill Hub is up to date." {
			home.Summary = "Skill Hub needs attention: " + termui.Plural(len(problemFindings(sections)), "problem", "problems") + " found."
		}
	}
	exit := 0
	if len(sections) == 0 {
		// Nothing to report on here; like every command that needs a hub.
		exit = 2
		home.Summary = noRoleMessage
	}
	switch {
	case jsonOutput:
		var clients []app.Section
		for _, section := range sections {
			if section.Kind == app.SectionClient {
				clients = append(clients, section)
			}
		}
		if err := writeJSON(stdout, statusJSON{CurationHome: home, Role: report.HealthRole(), ClientSections: clients}); err != nil {
			p := termui.New(stderr)
			p.Error("Unable to write the command result.", err.Error(), "Check the output destination and retry.")
			return 1
		}
		return exit
	case quiet:
		p := termui.New(stdout)
		p.Line(string(home.Status))
		return exit
	case len(sections) == 0:
		p := termui.New(stderr)
		p.Line(noRoleMessage)
		p.Next(next.Label+":", next.Command)
		return exit
	}
	p := termui.New(stdout)
	for index, section := range sections {
		if index > 0 {
			p.Blank()
		}
		if section.Kind == app.SectionHub {
			renderHubStatus(p, section, home, hub)
		} else {
			p.Raw(section.Title + "\n")
		}
		renderFindings(p, section, true, doctorFixCommand(report))
	}
	p.Blank()
	p.Next(nextLabel(next), next.Command)
	return printerCode(p)
}

func nextLabel(next app.NextStep) string {
	if next.Elsewhere && strings.HasPrefix(next.Command, "git ") {
		return next.Label + " (a Git command):"
	}
	return next.Label + ":"
}

// emptyHome is the curation home of a folder without a local hub: every
// key of the schema with an empty value.
func emptyHome(summary string) app.CurationHome {
	return app.CurationHome{
		Result:     app.NewResult(app.StatusOK, summary),
		Actions:    []app.ActionItem{},
		Categories: []app.ActionCategory{},
	}
}

// markBusy makes sure the hub section says the hub is busy when the
// curation home could not take the lock either.
func markBusy(sections []app.Section) []app.Section {
	for index := range sections {
		if sections[index].Kind != app.SectionHub {
			continue
		}
		for _, finding := range sections[index].Findings {
			if finding.Code == "hub_busy" {
				return sections
			}
		}
		sections[index].Findings = append(sections[index].Findings, app.Finding{Code: "hub_busy", Label: "Hub lock", Severity: app.SeverityNotice,
			Message: "The hub is busy with another change; run `skillhub status` again in a moment.", Manual: "skillhub status"})
	}
	return sections
}

// renderHubStatus prints the hub header and the curation summary of status
// for the hub folder hub.
func renderHubStatus(p *termui.Printer, section app.Section, home app.CurationHome, hub string) {
	p.Raw(section.Title + "\n")
	fields := []termui.Field{{Label: "Status", Value: home.Summary}}
	if home.Workspace.Health != "" {
		gitState := "clean"
		if !home.Workspace.GitConfigured {
			gitState = "not configured"
		} else if home.Workspace.GitDirty {
			gitState = "has uncommitted changes"
		}
		fields = append(fields, termui.Field{Label: "Health", Value: fmt.Sprintf("Workspace %s; search index %s; Git %s.", home.Workspace.Health, home.Workspace.Index, gitState)})
	}
	switch {
	case home.Workspace.Health == "":
	case home.Workspace.Health != "valid":
	case home.Workspace.Index != "current":
		fields = append(fields, termui.Field{Label: "Inventory", Value: "Skill and source counts are unavailable until the search index is rebuilt."})
	case home.CountsKnown && home.TotalSkills == 0 && home.HomeSummary.WatchingSources == 0:
		fields = append(fields, termui.Field{Label: "Inventory", Value: "No skills yet."})
	case home.CountsKnown:
		fields = append(fields, termui.Field{Label: "Inventory", Value: fmt.Sprintf("%d active, %d draft; %s.", home.HomeSummary.ActiveSkills, home.DraftSkills, termui.Plural(home.HomeSummary.WatchingSources, "watched source", "watched sources"))})
	}
	p.Fields(fields...)
	// Curation work the health checks do not cover (sources to distill,
	// upstream updates, lessons to review) stays visible with its command.
	type waitingItem struct{ line, command string }
	var waiting []waitingItem
	for _, action := range home.Actions {
		switch action.Kind {
		case "repair_workspace", "recover_workspace", "rebuild_index", "host_integration_missing", "review_git_changes", "first_run_commit":
			continue
		case "review_lessons":
			// A script command is printed on its own line, unwrapped, so it
			// can be copied and run from any folder.
			item := waitingItem{line: action.Summary}
			if command, in := reviewLessonsCommand(action, hub, runtime.GOOS); command != "" {
				item.line += "; run:"
				if in != "" {
					item.line = action.Summary + "; in " + in + ", run:"
				}
				item.command = command
			}
			waiting = append(waiting, item)
			continue
		}
		line := action.Summary
		if command := strings.TrimSuffix(cliNextCommand(action.Command, action.ID, ""), " --json"); command != "" {
			line += " (" + command + ")"
		}
		waiting = append(waiting, waitingItem{line: line})
	}
	if len(waiting) > 0 {
		p.Heading("Waiting")
		for _, item := range waiting {
			p.Bullets(item.line)
			if item.command != "" {
				p.Command(item.command)
			}
		}
	}
}

func problemFindings(sections []app.Section) []app.Finding {
	var problems []app.Finding
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity == app.SeverityProblem {
				problems = append(problems, finding)
			}
		}
	}
	return problems
}

func severityWord(severity app.Severity) string {
	switch severity {
	case app.SeverityProblem:
		return "FAILED"
	case app.SeverityNotice:
		return "notice"
	case app.SeveritySkipped:
		return "skipped"
	}
	return "ok"
}

// renderFindings prints the findings of a section as a table, then one FIX
// line per problem and one NOTE line per notice that has a command. With
// attentionOnly (status) passing checks are left out, except the lines that
// say which entry each agent uses.
func renderFindings(p *termui.Printer, section app.Section, attentionOnly bool, fixCommand string) {
	var rows [][]string
	for _, finding := range section.Findings {
		if attentionOnly && (finding.Severity == app.SeverityOK || finding.Severity == app.SeveritySkipped) && finding.Code != "client_host" && finding.Code != "remote_used_by" {
			continue
		}
		rows = append(rows, []string{finding.Label, severityWord(finding.Severity), finding.Message})
	}
	if len(rows) == 0 {
		return
	}
	p.Blank()
	p.Table([]string{"Check", "Status", "Detail"}, rows)
	var lines []termui.Field
	for _, finding := range section.Findings {
		switch finding.Severity {
		case app.SeverityProblem:
			lines = append(lines, termui.Field{Label: "FIX", Value: finding.Label + ": " + fixText(finding, fixCommand)})
		case app.SeverityNotice:
			if finding.Manual != "" && finding.Code != "hub_busy" {
				lines = append(lines, termui.Field{Label: "NOTE", Value: finding.Label + ": run `" + finding.Manual + "`."})
			}
		}
	}
	if len(lines) > 0 {
		p.Blank()
		p.Fields(lines...)
	}
	if attentionOnly {
		return
	}
	for _, finding := range section.Findings {
		if finding.Detail == "" || (finding.Severity != app.SeverityProblem && finding.Severity != app.SeverityNotice) {
			continue
		}
		p.Blank()
		p.Line(finding.Label + ", what the fix would change:")
		for _, line := range strings.Split(finding.Detail, "\n") {
			p.Raw("  " + line + "\n")
		}
	}
}

// fixText is the one fix line of a problem: what doctor --fix will do, or why
// it leaves the problem to a person and the command to run.
func fixText(finding app.Finding, fixCommand string) string {
	if finding.Fix != nil {
		return "`" + fixCommand + "` will " + finding.Fix.Summary + "."
	}
	text := finding.Reason
	if finding.Manual != "" && !strings.Contains(text, finding.Manual) {
		text = strings.TrimSpace(text + " Run: `" + finding.Manual + "`")
	}
	if text == "" {
		text = "Fix it by hand; doctor --fix cannot change it."
	}
	return text
}

// doctorFixCommand is `skillhub doctor --fix`, naming the hub when the
// person chose it explicitly.
func doctorFixCommand(report app.RoleReport) string {
	command := "skillhub doctor --fix"
	if report.Workspace != "" {
		command += " --workspace " + shellquote.Arg(report.Workspace)
	}
	return command
}

func statusFlags(args []string) (workspace string, jsonOutput, quiet bool, err error) {
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--quiet":
			quiet = true
		case "--workspace":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", jsonOutput, quiet, fmt.Errorf("--workspace requires a path")
			}
			workspace = args[index+1]
			index++
		default:
			return "", jsonOutput, quiet, fmt.Errorf("unknown argument %q", args[index])
		}
	}
	if jsonOutput && quiet {
		return "", jsonOutput, quiet, fmt.Errorf("--json and --quiet cannot be used together")
	}
	return workspace, jsonOutput, quiet, nil
}

func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	dir, code, ok := currentDir(stdout, stderr, hasJSONFlag(args), "doctor")
	if !ok {
		return code
	}
	return runDoctorIn(ctx, dir, args, stdout, stderr)
}

// doctorJSON is `doctor --json`.
type doctorJSON struct {
	Role     string             `json:"role"`
	Sections []app.Section      `json:"sections"`
	Fixed    []app.FixedItem    `json:"fixed"`
	NotFixed []app.NotFixedItem `json:"not_fixed"`
}

// runDoctorIn runs doctor for dir, which stands for the current folder.
// Exit codes: 0 healthy, 1 problems remain, 2 invalid request.
func runDoctorIn(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) int {
	workspaceFlag, jsonOutput, fix, dryRun, err := doctorFlags(args)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub doctor [--workspace <path>] [--fix [--dry-run]] [--json]`.")
	}
	report, resErr := healthReport(dir, workspaceFlag)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
	}
	sections := app.Run(ctx, report, app.DepthFull)
	if len(sections) == 0 {
		next := app.StatusNext(report, nil, app.HubInventory{})
		return writeInvalidRequest(stdout, stderr, jsonOutput, noRoleMessage, "To "+next.Label+", run `"+next.Command+"`.")
	}
	fixes := app.FixReport{Fixed: []app.FixedItem{}, NotFixed: []app.NotFixedItem{}}
	if fix && !dryRun {
		// A fix can change the role report itself (a re-pointed entry), so
		// each round detects the role again.
		detect := func() app.RoleReport {
			if again, resErr := healthReport(dir, workspaceFlag); resErr == nil {
				report = again
			}
			return report
		}
		fixes, sections = fixUntilDone(ctx, detect, sections)
	} else if fix {
		fixes = previewFixes(sections)
	}
	exit := 0
	if app.HasProblems(sections) && !dryRun {
		exit = 1
	}
	if jsonOutput {
		if err := writeJSON(stdout, doctorJSON{Role: report.HealthRole(), Sections: sections, Fixed: fixes.Fixed, NotFixed: fixes.NotFixed}); err != nil {
			p := termui.New(stderr)
			p.Error("Unable to write the command result.", err.Error(), "Check the output destination and retry.")
			return 1
		}
		return exit
	}
	p := termui.New(stdout)
	fixCommand := doctorFixCommand(report)
	if fix && !dryRun {
		renderFixReport(p, fixes, sections)
		return printerCodeOr(p, exit)
	}
	for index, section := range sections {
		if index > 0 {
			p.Blank()
		}
		p.Raw(section.Title + "\n")
		renderFindings(p, section, false, fixCommand)
	}
	p.Blank()
	if dryRun {
		renderWouldFix(p, fixes)
		return printerCodeOr(p, exit)
	}
	problems := problemFindings(sections)
	if len(problems) == 0 {
		p.Line("Healthy.")
		return printerCodeOr(p, exit)
	}
	fixable := 0
	for _, problem := range problems {
		if problem.Fix != nil {
			fixable++
		}
	}
	summary := termui.Plural(len(problems), "problem", "problems") + " found."
	if fixable > 0 {
		summary += fmt.Sprintf(" `%s` fixes %d of them.", fixCommand, fixable)
	}
	p.Line(summary)
	return printerCodeOr(p, exit)
}

// fixUntilDone applies the fixes, checks again, and applies the fixes that
// only the new checks reach (a re-pointed project reaches its hub's checks),
// at most three rounds and never the same finding twice. The not-fixed list
// holds the failed fixes and every problem still found at the end.
func fixUntilDone(ctx context.Context, detect func() app.RoleReport, sections []app.Section) (app.FixReport, []app.Section) {
	fixes := app.FixReport{Fixed: []app.FixedItem{}, NotFixed: []app.NotFixedItem{}}
	attempted := map[string]bool{}
	key := func(section app.Section, finding app.Finding) string {
		return section.Root + "\x00" + finding.Code + "\x00" + finding.Message
	}
	var failed []app.NotFixedItem
	for round := 0; round < 3; round++ {
		var pending []app.Section
		for _, section := range sections {
			kept := section
			kept.Findings = nil
			for _, finding := range section.Findings {
				if finding.Fix != nil && (finding.Severity == app.SeverityProblem || finding.Severity == app.SeverityNotice) && !attempted[key(section, finding)] {
					attempted[key(section, finding)] = true
					kept.Findings = append(kept.Findings, finding)
				}
			}
			pending = append(pending, kept)
		}
		applied := app.ApplyFixes(ctx, pending)
		if len(applied.Fixed) == 0 && len(applied.NotFixed) == 0 {
			break
		}
		fixes.Fixed = append(fixes.Fixed, applied.Fixed...)
		failed = append(failed, applied.NotFixed...)
		sections = app.Run(ctx, detect(), app.DepthFull)
	}
	seen := map[string]bool{}
	add := func(item app.NotFixedItem) {
		if id := item.Code + "\x00" + item.Message; !seen[id] {
			seen[id] = true
			fixes.NotFixed = append(fixes.NotFixed, item)
		}
	}
	for _, item := range failed {
		add(item)
	}
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity != app.SeverityProblem {
				continue
			}
			item := app.NotFixedItem{Code: finding.Code, Message: finding.Message, Reason: fixText(finding, ""), Manual: finding.Manual}
			if finding.Fix != nil {
				item.Reason = "doctor --fix tried to " + finding.Fix.Summary + ", but the check still finds the problem."
			}
			add(item)
		}
	}
	return fixes, sections
}

// previewFixes lists what doctor --fix would do, without running anything.
func previewFixes(sections []app.Section) app.FixReport {
	preview := app.FixReport{Fixed: []app.FixedItem{}, NotFixed: []app.NotFixedItem{}}
	var seen []*app.FixAction
	for _, section := range sections {
		for _, finding := range section.Findings {
			switch {
			case finding.Severity != app.SeverityProblem && finding.Severity != app.SeverityNotice:
			case finding.Fix != nil && !slices.Contains(seen, finding.Fix):
				seen = append(seen, finding.Fix)
				preview.Fixed = append(preview.Fixed, app.FixedItem{Codes: []string{finding.Code}, Summary: finding.Fix.Summary, Changed: app.Changed{}})
			case finding.Fix == nil && finding.Severity == app.SeverityProblem:
				preview.NotFixed = append(preview.NotFixed, app.NotFixedItem{Code: finding.Code, Message: finding.Message, Reason: fixText(finding, ""), Manual: finding.Manual})
			}
		}
	}
	return preview
}

func renderWouldFix(p *termui.Printer, preview app.FixReport) {
	if len(preview.Fixed) == 0 {
		p.Line("Dry run: doctor --fix has nothing to repair.")
	} else {
		p.Line("Dry run: nothing was changed. doctor --fix would:")
		for _, item := range preview.Fixed {
			p.Bullets(item.Summary)
		}
	}
	renderNotFixed(p, preview.NotFixed)
}

func renderFixReport(p *termui.Printer, fixes app.FixReport, after []app.Section) {
	if len(fixes.Fixed) == 0 {
		p.Line("Fixed: nothing; there was nothing doctor --fix could repair.")
	} else {
		p.Line("Fixed:")
		for _, item := range fixes.Fixed {
			p.Bullets(item.Summary)
			if len(item.Changed) == 0 {
				p.Raw("    no file needed a change\n")
			}
			for _, path := range item.Changed {
				// A repair also removes files, such as an old system skill copy.
				verb := "wrote "
				if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
					verb = "removed "
				}
				p.Raw("    " + verb + path + "\n")
			}
			for _, note := range item.Notes {
				p.Raw("    note: " + note + "\n")
			}
		}
	}
	renderNotFixed(p, fixes.NotFixed)
	p.Blank()
	if remaining := len(problemFindings(after)); remaining > 0 {
		p.Line(termui.Plural(remaining, "problem remains", "problems remain") + "; `skillhub doctor` shows them in full.")
	} else {
		p.Line("Healthy.")
	}
}

func renderNotFixed(p *termui.Printer, items []app.NotFixedItem) {
	if len(items) == 0 {
		return
	}
	p.Line("Not fixed:")
	for _, item := range items {
		p.Bullets(item.Message)
		for _, line := range termui.Wrap(item.Reason, p.Width()-4) {
			p.Raw("    " + line + "\n")
		}
		if item.Manual != "" && !strings.Contains(item.Reason, item.Manual) {
			p.Raw("    $ " + item.Manual + "\n")
		}
	}
}

func printerCodeOr(p *termui.Printer, code int) int {
	if p.Err() != nil {
		return 1
	}
	return code
}

func doctorFlags(args []string) (workspace string, jsonOutput, fix, dryRun bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--fix":
			fix = true
		case "--dry-run":
			dryRun = true
		case "--workspace":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", jsonOutput, fix, dryRun, fmt.Errorf("--workspace requires a path")
			}
			workspace = args[i+1]
			i++
		default:
			return "", jsonOutput, fix, dryRun, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if dryRun && !fix {
		return "", jsonOutput, fix, dryRun, fmt.Errorf("--dry-run requires --fix; doctor without --fix changes nothing")
	}
	return workspace, jsonOutput, fix, dryRun, nil
}
