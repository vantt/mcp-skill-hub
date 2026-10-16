package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/skill"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// skillAddAdapters lets tests give skill add source adapters that accept a file://
// repository in place of a GitHub one. It is nil in the shipped command.
var skillAddAdapters func(root string) map[string]sourcepkg.Adapter

// runSkillAdd adds a skill from a GitHub URL: it previews everything a person
// needs to decide, then, at a terminal, asks the add, approve and activate
// questions in turn and lands whatever was accepted as one hub commit.
func runSkillAdd(ctx context.Context, service app.SkillService, flags skillFlags, stdout, stderr io.Writer) int {
	locator := flags.locator
	if locator == "" && len(flags.positionals) > 0 {
		locator = flags.positionals[0]
	}
	if locator == "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"add requires a GitHub URL", "Run `skillhub skill add <github-url> [--skill <name> | --all] [--id <id>] [--yes]`.")
	}

	if flags.all && len(flags.skills) > 0 {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"--all and --skill cannot be combined", "Select specific skills with --skill or import all with --all.")
	}
	if flags.all && flags.id != "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"cannot specify --id when adding multiple skills", "Omit --id when using --all.")
	}
	if flags.approveContent != "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"skill add does not take --approve-content: only a person approves third-party content",
			"Add the draft, then approve its content at the third question in a terminal, or with `skillhub skill review <id>`.")
	}
	if flags.editRouting && (flags.yes || flags.jsonOutput) {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"--edit-routing needs a terminal and cannot be combined with --yes or --json",
			"Use --trigger, --not-for, --example and --min-scope instead.")
	}

	addService := app.SkillAddService{}
	if skillAddAdapters != nil {
		if root, err := workspace.Discover(flags.workspace); err == nil {
			addService.Adapters = skillAddAdapters(root)
		}
	}
	if isFolderArgument(locator, skillAddAdapters != nil) {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"skill add takes a GitHub URL.", "To add a folder from this machine: skillhub skill commit "+shellquote.Arg(locator))
	}

	// Format locator with ref and path if explicit
	resolvedLocator := locator
	if flags.ref != "" || flags.subPath != "" {
		ref := flags.ref
		if ref == "" {
			ref = "main"
		}
		cleanPath := strings.Trim(flags.subPath, "/")
		if cleanPath != "" {
			resolvedLocator = strings.TrimSuffix(locator, "/") + "/tree/" + ref + "/" + cleanPath
		} else {
			resolvedLocator = strings.TrimSuffix(locator, "/") + "/tree/" + ref
		}
	}

	selection := ""
	if len(flags.skills) > 0 {
		selection = flags.skills[0]
	}

	input := app.SkillAddInput{
		Locator:        resolvedLocator,
		Selection:      selection,
		All:            flags.all,
		TargetID:       flags.id,
		Collection:     flags.collection,
		FullDiff:       flags.diff,
		IdempotencyKey: flags.idempotencyKey,
	}
	if flags.hasRoutingFlags() {
		input.Routing = &skill.RoutingInput{Operations: flags.operations, Triggers: flags.triggers, NotFor: flags.notFor, MinScope: flags.minScope, Examples: flags.examples, CounterExamples: flags.counterExamples}
	}

	preview, err := addService.PreviewSkillAdd(ctx, flags.workspace, input)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	// An error preview has nothing to confirm; show its own cause even under --yes.
	if app.ErrorOf(preview, nil) != nil {
		return writeResult(stdout, stderr, flags.jsonOutput, preview, nil)
	}
	if flags.editRouting {
		edited, code, ok := editSuggestedRouting(ctx, flags, app.SkillCommitPreview{SkillID: preview.SkillID, RoutingSuggestions: preview.RoutingSuggestions}, stdout, stderr)
		if !ok {
			return code
		}
		input.Routing = edited
		if preview, err = addService.PreviewSkillAdd(ctx, flags.workspace, input); err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		if app.ErrorOf(preview, nil) != nil {
			return writeResult(stdout, stderr, flags.jsonOutput, preview, nil)
		}
	}

	options := app.SkillAddApplyOptions{}
	if !flags.yes {
		interactive := !preview.Replayed && !flags.jsonOutput && confirmInteractive(stdout)
		code := writeResult(stdout, stderr, flags.jsonOutput, preview, func(p *termui.Printer) {
			writeSkillAddPreview(p, preview, previewHints{rerun: flags.rerun, diffShown: flags.diff, interactive: interactive})
		})
		if !interactive || code != 0 {
			return code
		}
		var proceed bool
		if options, proceed = askAddQuestions(stdout, preview); !proceed {
			return 0
		}
	}

	result, err := addService.ConfirmSkillAddWith(commitContext(ctx, flags.noCommit, stdout), flags.workspace, preview, preview.Confirmation.Confirmation.Pins, options)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}

	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		writeSkillAddResult(p, flags.verbose, result)
	})
}

// isFolderArgument reports whether the argument names a folder on this machine,
// which skill add no longer takes: an existing path, or a file:// URL unless the
// command was given adapters that accept one.
func isFolderArgument(arg string, allowFile bool) bool {
	value := strings.TrimSpace(arg)
	if strings.HasPrefix(strings.ToLower(value), "file://") {
		return !allowFile
	}
	if strings.Contains(value, "://") || strings.HasPrefix(value, "git@") {
		return false
	}
	if strings.HasPrefix(value, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			value = filepath.Join(home, value[2:])
		}
	}
	_, err := os.Stat(value)
	return err == nil
}

// askAddQuestions asks what the add needs, in order, and reports whether to go
// on. Each "n" stops the chain, and what was accepted so far is committed.
func askAddQuestions(stdout io.Writer, preview app.SkillAddProposal) (app.SkillAddApplyOptions, bool) {
	options := app.SkillAddApplyOptions{}
	if len(preview.SkillIDs) > 1 {
		if !askYesNo(stdout, fmt.Sprintf("Add %s as drafts?", strings.Join(preview.SkillIDs, ", "))) {
			fmt.Fprintln(stdout, "Nothing written.")
			return options, false
		}
		return options, true
	}
	if !askYesNo(stdout, fmt.Sprintf("Add %s as a draft?", preview.SkillID)) {
		fmt.Fprintln(stdout, "Nothing written.")
		return options, false
	}
	if preview.Trust != nil && preview.Trust.ThirdParty {
		if !askYesNo(stdout, "Approve this content for agents?") {
			return options, true
		}
		options.Approve = true
	}
	if !preview.ActivationReady {
		p := termui.New(stdout)
		p.Line(fmt.Sprintf("%s cannot be activated yet: %s.", preview.SkillID, strings.Join(preview.ActivationMissing, ", ")))
		return options, true
	}
	options.Activate = askYesNo(stdout, fmt.Sprintf("Activate %s now?", preview.SkillID))
	return options, true
}

func writeSkillAddPreview(p *termui.Printer, preview app.SkillAddProposal, hints previewHints) {
	skillName := preview.SkillID
	if len(preview.SkillIDs) > 1 {
		skillName = strings.Join(preview.SkillIDs, ", ")
	}
	if preview.Replayed {
		writeSkillAddReplayed(p, preview.SkillID, preview.Assessment.Canonical.Status)
		return
	}

	header := fmt.Sprintf("Add %s from %s", skillName, preview.SourceLabel())
	if preview.UpstreamSource != nil {
		header += ", tracked for upstream updates"
	}
	p.Line(header + ".")
	writeNewSkillFiles(p, app.SkillCommitPreview{Files: preview.Files, TotalBytes: totalFileBytes(preview.Files)})
	writeLeftOutLine(p, preview.Excluded)
	p.Line("License: " + licenseLine(preview.License))
	for _, warn := range preview.Warnings {
		p.Warning(warn.Summary)
	}
	if routing := preview.RoutingSuggestions; routing != nil {
		writeCommitRouting(p, app.SkillCommitPreview{Case: "new", RoutingSuggestions: routing})
	}
	writeCommitLint(p, preview.RoutingLint)
	if preview.Trust != nil && preview.Trust.ThirdParty {
		p.Line(fmt.Sprintf("Third-party: agents get %s only after you approve its content.", skillName))
	}
	if preview.UpstreamSource != nil {
		if preview.UpstreamSource.Created {
			p.Line(fmt.Sprintf("Upstream: tracked by new source %s (checked weekly by `skillhub source check`; nothing runs in the background).", preview.UpstreamSource.SourceID))
		} else {
			p.Line(fmt.Sprintf("Upstream: tracked by source %s.", preview.UpstreamSource.SourceID))
		}
	}
	if preview.FullDiff != "" {
		p.Raw(preview.FullDiff)
		if !strings.HasSuffix(preview.FullDiff, "\n") {
			p.Raw("\n")
		}
	}
	if !hints.interactive {
		p.Line("Nothing written yet.")
	}
	if !hints.diffShown {
		p.Raw("Show every file: " + hints.rerun + " --diff\n")
	}
	if !hints.interactive {
		p.Raw("Apply: " + hints.rerun + " --yes   (adds the draft only; a person approves and activates it)\n")
	}
}

// totalFileBytes sums the sizes of the listed files.
func totalFileBytes(files []app.SkillCommitFile) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

// licenseLine says which license the skill declares, and where its license file is.
func licenseLine(license app.SkillLicenseInfo) string {
	line := "not declared"
	if license.Declared != "" {
		line = license.Declared
	}
	if license.LicenseFile != "" {
		line += " (" + license.LicenseFile + ")"
	}
	return line
}

func writeSkillAddResult(p *termui.Printer, verbose bool, result app.SkillAddResult) {
	if result.Replayed {
		writeSkillAddReplayed(p, result.SkillID, result.Assessment.Canonical.Status)
		return
	}
	skillName := result.SkillID
	if len(result.SkillIDs) > 1 {
		skillName = strings.Join(result.SkillIDs, ", ")
	}
	upstream := ""
	if result.UpstreamSource != nil {
		upstream = " Upstream: " + result.UpstreamSource.SourceID + "."
	}
	switch {
	case result.Activated:
		p.Line(fmt.Sprintf("Added %s, approved its content and activated it.%s%s", skillName, upstream, notCommittedNote(result.Commit)))
	case result.Approved:
		p.Line(fmt.Sprintf("Draft %s added and its content approved. Agent use: off.%s%s", skillName, upstream, notCommittedNote(result.Commit)))
	default:
		p.Line(fmt.Sprintf("Draft %s added. Agent use: off.%s%s", skillName, upstream, notCommittedNote(result.Commit)))
	}
	writeCommitLines(p, result.Commit)
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
	}
	if verbose {
		p.Fields(
			termui.Field{Label: "Operation", Value: result.OperationID},
			termui.Field{Label: "Catalog snapshot", Value: result.CatalogSnapshot},
			termui.Field{Label: "Generation", Value: result.Generation},
		)
	}
	switch {
	case result.Activated:
	case result.Approved && result.ActivationFix != "":
		p.Line(fmt.Sprintf("%s cannot be activated yet; it needs: %s.", result.SkillID, strings.Join(result.ActivationMissing, ", ")))
		p.Next("add them (fill in the placeholders), then activate it:", result.ActivationFix)
	case result.Approved:
		p.Line(fmt.Sprintf("Next: skillhub skill activate %s", result.SkillID))
	default:
		p.Line(fmt.Sprintf("Next: skillhub skill review %s", result.SkillID))
		if result.Origin.Kind != "local" {
			p.Line("A person approves its content: run that in a terminal, or use the Waiting for review page on the web.")
		}
	}
}

// writeSkillAddReplayed explains that an identical add already happened, with
// the skill's real lifecycle state, and offers no confirm command.
func writeSkillAddReplayed(p *termui.Printer, skillID, state string) {
	line := fmt.Sprintf("%s was already added from this source; nothing changed.", skillID)
	if state != "" {
		line += fmt.Sprintf(" It is %s now.", state)
	}
	p.Line(line)
	p.Line(fmt.Sprintf("Next: skillhub skill review %s", skillID))
}

// notCommittedNote is the sentence for a change that no commit covers.
func notCommittedNote(outcome *workspace.CommitOutcome) string {
	if outcome == nil {
		return " Changes are not committed."
	}
	return ""
}
