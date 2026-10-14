package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"gopkg.in/yaml.v3"
)

// runSkillCommit brings a project folder back to the hub as one reviewed
// change: preview first, then, at a terminal, the y/N questions, and without
// one `--yes`. A folder that was never checked out becomes a new skill.
func runSkillCommit(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	input, err := commitInputFromFlags(flags)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Review `skillhub skill commit` arguments and retry.")
	}
	service := app.SkillCommitService{}
	preview, err := service.Preview(ctx, flags.workspace, input)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	if app.ErrorOf(preview, nil) != nil {
		return writeResult(stdout, stderr, flags.jsonOutput, preview, nil)
	}
	if flags.editRouting {
		edited, code, ok := editSuggestedRouting(ctx, flags, preview, stdout, stderr)
		if !ok {
			return code
		}
		input.Routing = edited
		if preview, err = service.Preview(ctx, flags.workspace, input); err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		if app.ErrorOf(preview, nil) != nil {
			return writeResult(stdout, stderr, flags.jsonOutput, preview, nil)
		}
	}
	if flags.writeConflicts != "" {
		if writeErr := writeConflictsFiles(flags.writeConflicts, preview.MergeFiles); writeErr != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, writeErr.Error(), "Choose an empty or different directory for --write-conflicts.")
		}
	}
	options := app.SkillCommitApplyOptions{}
	canApply := !preview.NothingToCommit && len(preview.Conflicts) == 0
	if flags.yes {
		if len(preview.Conflicts) > 0 {
			return applyCommit(ctx, flags, preview, options, stdout, stderr)
		}
		if preview.Trust != nil && preview.Trust.ThirdParty && flags.approveContent != "" {
			if flags.approveContent != preview.Trust.ContentDigest {
				return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
					"--approve-content does not match the new content ("+preview.Trust.ContentDigest+")",
					"Review the new content, then pass the digest the preview prints.")
			}
			options.Approve = true
		}
	} else {
		interactive := canApply && !flags.jsonOutput && confirmInteractive(stdout)
		code := writeResult(stdout, stderr, flags.jsonOutput, preview, func(p *termui.Printer) {
			writeSkillCommitPreview(p, preview, previewHints{rerun: flags.rerun, diffShown: flags.diff, interactive: interactive})
		})
		if !interactive || code != 0 {
			return code
		}
		var proceed bool
		if options, proceed = askCommitQuestions(stdout, preview); !proceed {
			return 0
		}
	}
	if preview.NothingToCommit {
		return writeResult(stdout, stderr, flags.jsonOutput, preview, func(p *termui.Printer) { p.Line(preview.Summary) })
	}
	return applyCommit(ctx, flags, preview, options, stdout, stderr)
}

func applyCommit(ctx context.Context, flags skillFlags, preview app.SkillCommitPreview, options app.SkillCommitApplyOptions, stdout, stderr io.Writer) int {
	result, err := (app.SkillCommitService{}).Apply(commitContext(ctx, flags.noCommit, stdout), flags.workspace, preview, options)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		writeSkillCommitResult(p, result)
	})
}

// commitInputFromFlags turns the command line into the service input.
func commitInputFromFlags(flags skillFlags) (app.SkillCommitInput, error) {
	input := app.SkillCommitInput{Target: flags.positionals[0], ProjectDir: currentDirectory(), Collection: flags.collection, FullDiff: flags.diff}
	if flags.hasRoutingFlags() {
		input.Routing = &skill.RoutingInput{Operations: flags.operations, Triggers: flags.triggers, NotFor: flags.notFor, MinScope: flags.minScope, Examples: flags.examples, CounterExamples: flags.counterExamples}
	}
	for _, item := range flags.accepts {
		path, choice, _ := strings.Cut(item, "=")
		input.Resolutions = append(input.Resolutions, app.SkillCommitResolution{Path: path, Choice: choice})
	}
	for _, item := range flags.manuals {
		path, file, _ := strings.Cut(item, "=")
		content, err := os.ReadFile(strings.TrimSpace(file))
		if err != nil {
			return input, fmt.Errorf("read the manual file for %s: %w", path, err)
		}
		input.Resolutions = append(input.Resolutions, app.SkillCommitResolution{Path: path, Choice: "manual", Content: content})
	}
	return input, nil
}

// askCommitQuestions asks what the commit needs, in order, and reports whether
// to go on. A third-party skill's new content is approved in the same step,
// because applying it without approval would stop agents from getting the skill.
func askCommitQuestions(stdout io.Writer, preview app.SkillCommitPreview) (app.SkillCommitApplyOptions, bool) {
	options := app.SkillCommitApplyOptions{}
	nothing := func(reason string) (app.SkillCommitApplyOptions, bool) {
		p := termui.New(stdout)
		p.Line("Nothing written." + reason)
		return options, false
	}
	if preview.Case == "new" {
		if !askYesNo(stdout, fmt.Sprintf("Add %s as a draft?", preview.SkillID)) {
			return nothing("")
		}
		options.Activate = preview.ActivationReady && askYesNo(stdout, fmt.Sprintf("Activate %s now?", preview.SkillID))
		return options, true
	}
	if !askYesNo(stdout, "Apply this change?") {
		return nothing("")
	}
	if preview.Trust != nil && preview.Trust.ThirdParty {
		if !askYesNo(stdout, fmt.Sprintf("Approve this content for agents? It is third-party (%s).", preview.Trust.Origin)) {
			return nothing(fmt.Sprintf(" Applying without approval would stop agents from getting %s, so the change was not made.", preview.SkillID))
		}
		options.Approve = true
	}
	return options, true
}

// editSuggestedRouting opens the routing suggestions in the person's editor
// and returns what they saved. It needs a terminal, like every question.
func editSuggestedRouting(ctx context.Context, flags skillFlags, preview app.SkillCommitPreview, stdout, stderr io.Writer) (*skill.RoutingInput, int, bool) {
	if flags.yes || flags.jsonOutput || !confirmInteractive(stdout) {
		return nil, writeInvalidRequest(stdout, stderr, flags.jsonOutput, "--edit-routing needs a terminal", "Use --trigger, --not-for, --example and --min-scope instead."), false
	}
	if preview.RoutingSuggestions == nil {
		return nil, writeInvalidRequest(stdout, stderr, false, fmt.Sprintf("%s already has routing, so there is nothing suggested to edit", preview.SkillID), "Change it with --trigger, --not-for, --example and --min-scope."), false
	}
	edited, err := editRoutingYAML(ctx, *preview.RoutingSuggestions)
	if err != nil {
		return nil, writeInvalidRequest(stdout, stderr, false, err.Error(), "Run the command again and fix the routing file."), false
	}
	return &edited, 0, true
}

// editRoutingYAML lets the person edit routing as a small YAML file.
func editRoutingYAML(ctx context.Context, routing skill.RoutingInput) (skill.RoutingInput, error) {
	body, err := yaml.Marshal(routing)
	if err != nil {
		return routing, err
	}
	file, err := os.CreateTemp("", "skillhub-routing-*.yaml")
	if err != nil {
		return routing, err
	}
	name := file.Name()
	defer os.Remove(name)
	header := "# Routing suggested from the description. Edit, save and close.\n# triggers: when an agent should use the skill; not_for: when it should not.\n"
	if _, err = file.WriteString(header + string(body)); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err != nil {
		return routing, err
	}
	if err := launchEditor(ctx, name); err != nil {
		return routing, err
	}
	edited, err := os.ReadFile(name)
	if err != nil {
		return routing, err
	}
	var result skill.RoutingInput
	decoder := yaml.NewDecoder(strings.NewReader(string(edited)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&result); err != nil && err != io.EOF {
		return routing, fmt.Errorf("the routing file is not valid: %w", err)
	}
	return result, nil
}

// writeSkillCommitPreview prints what a commit would change.
func writeSkillCommitPreview(p *termui.Printer, preview app.SkillCommitPreview, hints previewHints) {
	p.Line(preview.Summary)
	if preview.NothingToCommit {
		return
	}
	if preview.Case == "new" {
		writeNewSkillFiles(p, preview)
	} else {
		writeChangedFiles(p, preview)
	}
	writeLeftOutLine(p, preview.Excluded)
	for _, warning := range preview.Warnings {
		if warning.Code != "needs_decision" {
			p.Line(warning.Summary)
		}
	}
	writeCommitRouting(p, preview)
	writeCommitLint(p, preview.RoutingLint)
	writeCommitTrust(p, preview)
	if len(preview.Conflicts) > 0 {
		writeCommitConflicts(p, preview, hints.rerun)
		return
	}
	if preview.FullDiff != "" {
		p.Raw(preview.FullDiff)
		if !strings.HasSuffix(preview.FullDiff, "\n") {
			p.Raw("\n")
		}
	}
	writeCommitTail(p, preview, hints)
}

// writeLeftOutLine names the files a commit leaves out, on one line.
func writeLeftOutLine(p *termui.Printer, excluded []app.ExcludedFile) {
	if len(excluded) == 0 {
		return
	}
	items := make([]string, 0, len(excluded))
	for _, file := range excluded {
		items = append(items, fmt.Sprintf("%s (%s)", file.Path, file.Reason))
	}
	p.Line("Left out: " + strings.Join(items, "; "))
}

func writeNewSkillFiles(p *termui.Printer, preview app.SkillCommitPreview) {
	p.Line(fmt.Sprintf("Files (%d, %s):", len(preview.Files), termui.Bytes(preview.TotalBytes)))
	rows := make([]string, 0, len(preview.Files))
	for _, file := range preview.Files {
		rows = append(rows, file.Path+"\t"+termui.Bytes(file.Size))
	}
	p.Raw(alignedRows(rows, "  ", true))
}

func writeChangedFiles(p *termui.Printer, preview app.SkillCommitPreview) {
	rows := make([]string, 0, len(preview.Files))
	listed := map[string]bool{}
	for _, file := range preview.Files {
		listed[file.Path] = true
		row := file.Change + " " + file.Path + "\t" + lineCounts(file)
		if file.Status != "" {
			row += "\t" + file.Status
		}
		rows = append(rows, row)
	}
	for _, entry := range preview.Merge {
		if !listed[entry.Path] && entry.Action != "" {
			rows = append(rows, "  "+entry.Path+"\t(no project change)\t"+entry.Status)
		}
	}
	p.Raw(alignedRows(rows, "  ", false))
}

// lineCounts is "+3 -1", "+12" or "-20", or the size of a file that is not text.
func lineCounts(file app.SkillCommitFile) string {
	var parts []string
	if file.Added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", file.Added))
	}
	if file.Removed > 0 {
		parts = append(parts, fmt.Sprintf("-%d", file.Removed))
	}
	if len(parts) == 0 {
		return termui.Bytes(file.Size)
	}
	return strings.Join(parts, " ")
}

// alignedRows pads tab-separated columns to the widest cell of each column but
// the last. With rightAlignLast the last column lines up on its right edge.
func alignedRows(rows []string, indent string, rightAlignLast bool) string {
	var widths []int
	cells := make([][]string, len(rows))
	for index, row := range rows {
		cells[index] = strings.Split(row, "\t")
		for column, cell := range cells[index] {
			if column >= len(widths) {
				widths = append(widths, 0)
			}
			widths[column] = max(widths[column], len(cell))
		}
	}
	var out strings.Builder
	for _, row := range cells {
		out.WriteString(indent)
		for column, cell := range row {
			last := column == len(row)-1
			switch {
			case last && rightAlignLast && column > 0:
				out.WriteString(strings.Repeat(" ", widths[column]-len(cell)) + cell)
			case last:
				out.WriteString(cell)
			default:
				out.WriteString(cell + strings.Repeat(" ", widths[column]-len(cell)+2))
			}
		}
		out.WriteString("\n")
	}
	return out.String()
}

func writeCommitRouting(p *termui.Printer, preview app.SkillCommitPreview) {
	routing := preview.RoutingSuggestions
	if routing == nil {
		return
	}
	heading := "Routing, suggested from the description (edit with --edit-routing):"
	if preview.Case != "new" {
		heading = "This skill has no routing yet; suggested from its description (edit with --edit-routing):"
	}
	p.Line(heading)
	var rows []string
	for _, group := range []struct {
		label  string
		values []string
	}{{"trigger", routing.Triggers}, {"not for", routing.NotFor}, {"examples", routing.Examples}} {
		if len(group.values) > 0 {
			rows = append(rows, group.label+"\t"+quoteAll(group.values))
		}
	}
	rows = append(rows, "min scope\t"+routing.MinScope)
	p.Raw(alignedRows(rows, "  ", false))
}

func quoteAll(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quoted, ", ")
}

func writeCommitLint(p *termui.Printer, findings []app.SkillCommitLint) {
	if len(findings) == 0 {
		p.Line("Routing lint: no findings.")
		return
	}
	p.Line(fmt.Sprintf("Routing lint: %s:", termui.Plural(len(findings), "warning", "warnings")))
	items := make([]string, 0, len(findings))
	for _, finding := range findings {
		item := finding.Message
		if finding.Fix != "" {
			item += " Fix: " + finding.Fix
		}
		items = append(items, item)
	}
	p.Bullets(items...)
}

func writeCommitTrust(p *termui.Printer, preview app.SkillCommitPreview) {
	if preview.Trust == nil || !preview.Trust.ThirdParty {
		return
	}
	p.Line(fmt.Sprintf("%s is third-party (%s). The new content needs your approval; without it agents could not use %s.", preview.SkillID, preview.Trust.Origin, preview.SkillID))
}

func writeCommitConflicts(p *termui.Printer, preview app.SkillCommitPreview, rerun string) {
	p.Line(fmt.Sprintf("%s a decision before this can be applied:", termui.Plural(len(preview.Conflicts), "file needs", "files need")))
	p.Bullets(preview.Conflicts...)
	first := preview.Conflicts[0]
	p.Raw("Decide each: " + rerun + " --accept " + shellquote.Arg(first) + "=hub   (or =project)\n")
	p.Raw("Or give your own: --manual " + shellquote.Arg(first) + "=<file>   Or write the merge with markers to a folder: --write-conflicts <dir>\n")
}

// writeCommitTail ends a preview with the hints that apply: the diff, and how
// to apply it without a terminal.
func writeCommitTail(p *termui.Printer, preview app.SkillCommitPreview, hints previewHints) {
	if !hints.interactive {
		p.Line("Nothing written yet.")
	}
	if !hints.diffShown {
		p.Raw("Show the diff: " + hints.rerun + " --diff\n")
	}
	if hints.interactive {
		return
	}
	apply := hints.rerun + " --yes"
	if preview.Trust != nil && preview.Trust.ThirdParty {
		apply += " --approve-content " + preview.Trust.ContentDigest
		p.Line("Only a person approves third-party content: run this in a terminal and answer the question, or pass the digest after reading the diff.")
	}
	p.Raw("Apply: " + apply + "\n")
}

// writeSkillCommitResult says what a commit did and who gets the new version.
func writeSkillCommitResult(p *termui.Printer, result app.SkillCommitResult) {
	if result.Commit == nil || result.Commit.Status != "committed" {
		p.Line(result.Summary + notCommittedNote(result.Commit))
	}
	writeCommitLines(p, result.Commit)
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
	}
	if result.Case == "new" && !result.Activated {
		p.Next("it is a draft and agents do not get it yet; review it, then activate it:", "skillhub skill review "+shellquote.Arg(result.SkillID))
	}
}
