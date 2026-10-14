package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
)

// runSourceWatch executes the intent-first source watch workflow.
func runSourceWatch(ctx context.Context, service app.SourceService, flags sourceFlags, locator string, stdout, stderr io.Writer) int {
	if locator == "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"watch requires one locator",
			"Run `skillhub source watch <locator> [--id <id>] [--ref <ref>] [--path <path>] [--cadence daily|weekly|manual] [--yes]`.")
	}

	input := app.SourceWatchInput{
		Locator:        locator,
		SkillID:        flags.skillID,
		SourceID:       flags.sourceID,
		Ref:            flags.ref,
		Path:           flags.sourcePath,
		Cadence:        flags.cadence,
		IdempotencyKey: flags.idempotencyKey,
	}

	proposal, err := service.PreviewSourceWatch(ctx, flags.workspace, input)
	if err != nil {
		return writeSourceError(stdout, stderr, flags.jsonOutput, err)
	}
	if !flags.yes {
		apply, code := confirmInPlace(stdout, flags.jsonOutput, proposal.Error == nil, func(interactive bool) int {
			return writeResult(stdout, stderr, flags.jsonOutput, proposal, func(p *termui.Printer) {
				writeWatchPreview(p, proposal, flags, locator, interactive)
			})
		})
		if !apply {
			return code
		}
	}

	// Immediate confirmation with fresh preview pins
	result, err := service.ConfirmSourceWatch(commitContext(ctx, flags.noCommit, stdout), flags.workspace, proposal, proposal.Confirmation.Confirmation.Pins)
	if err != nil {
		return writeSourceError(stdout, stderr, flags.jsonOutput, err)
	}

	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		cadence := flags.cadence
		if cadence == "" {
			cadence = "weekly"
		}
		p.Line(fmt.Sprintf("Watching source %s (%s). Cadence: %s.%s",
			result.SourceID, locator, cadence, notCommittedNote(result.Commit)))
		writeCommitLines(p, result.Commit)
		p.Line(fmt.Sprintf("Next: skillhub source check %s", result.SourceID))
	})
}

// writeWatchPreview prints what a watch would add and how to apply it.
func writeWatchPreview(p *termui.Printer, proposal app.SourceProposal, flags sourceFlags, locator string, interactive bool) {
	cadence := proposal.Source.Monitoring.Cadence
	if cadence == "" {
		cadence = flags.cadence
	}
	if cadence == "" {
		cadence = "weekly"
	}
	sourceID := proposal.Source.ID
	if sourceID == "" {
		sourceID = flags.sourceID
	}
	p.Line(fmt.Sprintf("Watch source %s (%s).", sourceID, locator))
	p.Line(fmt.Sprintf("Cadence: %s. Watching: on.", cadence))
	previewTail(p, flags.rerun, true, interactive)
}
