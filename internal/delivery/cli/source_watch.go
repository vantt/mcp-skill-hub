package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
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
		return writeResult(stdout, stderr, flags.jsonOutput, proposal, func(p *termui.Printer) {
			pins := proposal.Confirmation.Confirmation.Pins
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
			p.Line("No collection files changed.")
			p.Line(fmt.Sprintf("Next: skillhub source confirm --proposal %s --proposal-digest %s --base-version %s",
				pins.ProposalID, pins.ProposalDigest, pins.BaseVersion))
		})
	}

	// Immediate confirmation with fresh preview pins
	result, err := service.ConfirmSourceWatch(ctx, flags.workspace, proposal, proposal.Confirmation.Confirmation.Pins)
	if err != nil {
		return writeSourceError(stdout, stderr, flags.jsonOutput, err)
	}

	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		cadence := flags.cadence
		if cadence == "" {
			cadence = "weekly"
		}
		p.Line(fmt.Sprintf("Watching source %s (%s). Cadence: %s. Changes are not committed.",
			result.SourceID, locator, cadence))
		p.Line(fmt.Sprintf("Next: skillhub source check %s", result.SourceID))
	})
}
