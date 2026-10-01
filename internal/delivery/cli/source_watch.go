package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/vantt/mcp-skill-hub/internal/app"
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
	if proposal.Error != nil {
		if flags.jsonOutput {
			_ = writeSourceJSON(stdout, stderr, proposal)
			return 2
		}
		fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n",
			proposal.Error.Render.Error, proposal.Error.Render.Why, proposal.Error.Render.Fix)
		return 2
	}

	if !flags.yes {
		if flags.jsonOutput {
			return writeSourceJSON(stdout, stderr, proposal)
		}
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
		fmt.Fprintf(stdout, "Watch source %s (%s).\n", sourceID, locator)
		fmt.Fprintf(stdout, "Cadence: %s. Watching: on.\n", cadence)
		fmt.Fprintln(stdout, "No collection files changed.")
		fmt.Fprintf(stdout, "Next: skillhub source confirm --proposal %s --proposal-digest %s --base-version %s\n",
			pins.ProposalID, pins.ProposalDigest, pins.BaseVersion)
		return 0
	}

	// Immediate confirmation with fresh preview pins
	result, err := service.ConfirmSourceWatch(ctx, flags.workspace, proposal, proposal.Confirmation.Confirmation.Pins)
	if err != nil {
		return writeSourceError(stdout, stderr, flags.jsonOutput, err)
	}
	if result.Error != nil {
		if flags.jsonOutput {
			_ = writeSourceJSON(stdout, stderr, result)
			return 2
		}
		fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n",
			result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
		return 2
	}

	if flags.jsonOutput {
		return writeSourceJSON(stdout, stderr, result)
	}

	cadence := flags.cadence
	if cadence == "" {
		cadence = "weekly"
	}
	fmt.Fprintf(stdout, "Watching source %s (%s). Cadence: %s. Changes are not committed.\n",
		result.SourceID, locator, cadence)
	fmt.Fprintf(stdout, "Next: skillhub source check %s\n", result.SourceID)
	return 0
}
