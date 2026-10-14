package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
)

func validateSourceImportRequest(flags sourceFlags, positionals []string) error {
	if flags.proposalID != "" {
		if flags.proposalDigest == "" || flags.baseVersion == "" || !flags.yes {
			return errors.New("confirming import requires --proposal, --proposal-digest, --base-version, and --yes")
		}
		if len(positionals) != 0 || flags.ref != "" || flags.sourcePath != "" || len(flags.skills) != 0 || flags.all || flags.sourceID != "" || flags.skillID != "" || flags.idempotencyKey != "" {
			return errors.New("confirming import uses the reviewed proposal; do not pass a locator, source ID, ref, path, or skill selection")
		}
		return nil
	}
	if flags.proposalDigest != "" || flags.baseVersion != "" {
		return errors.New("--proposal-digest and --base-version require --proposal and --yes")
	}
	if len(positionals) != 1 || strings.TrimSpace(positionals[0]) == "" {
		return errors.New("import requires one locator or source ID")
	}
	if flags.all && len(flags.skills) > 0 {
		return errors.New("--all and --skill cannot be combined")
	}
	return nil
}

func runSourceImport(ctx context.Context, service app.SourceImportService, flags sourceFlags, positionals []string, stdout, stderr io.Writer) int {
	if flags.proposalID != "" {
		preview, err := service.LoadSourceImportProposal(ctx, flags.workspace, flags.proposalID)
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		result, err := service.ConfirmSourceImport(commitContext(ctx, flags.noCommit, stdout), flags.workspace, preview, app.ConfirmationPins{
			ProposalID:     flags.proposalID,
			ProposalDigest: flags.proposalDigest,
			BaseVersion:    flags.baseVersion,
		})
		return writeSourceImportResult(stdout, stderr, flags.jsonOutput, result, err)
	}

	input := app.SourceImportPreviewInput{
		Path:           flags.sourcePath,
		Ref:            flags.ref,
		Skills:         flags.skills,
		IdempotencyKey: flags.idempotencyKey,
	}
	target := positionals[0]
	if strings.Contains(target, "://") || strings.HasPrefix(target, "git@") || strings.ContainsAny(target, "/\\") || strings.HasPrefix(target, ".") || strings.HasPrefix(target, "~") {
		input.Locator = target
	} else {
		input.SourceID = target
	}
	preview, err := service.PreviewSourceImport(ctx, flags.workspace, input)
	if err != nil {
		return writeSourceError(stdout, stderr, flags.jsonOutput, err)
	}
	if preview.Error != nil || len(preview.Importable) == 0 {
		return writeSourceImportProposal(stdout, stderr, flags.jsonOutput, preview, flags.rerun, false)
	}
	if !flags.yes {
		apply, code := confirmInPlace(stdout, flags.jsonOutput, true, func(interactive bool) int {
			return writeSourceImportProposal(stdout, stderr, flags.jsonOutput, preview, flags.rerun, interactive)
		})
		if !apply {
			return code
		}
	}
	result, err := service.ConfirmSourceImport(commitContext(ctx, flags.noCommit, stdout), flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
	return writeSourceImportResult(stdout, stderr, flags.jsonOutput, result, err)
}

// writeImportLeftOut prints one line per importable skill that has files the
// import will not copy.
func writeImportLeftOut(p *termui.Printer, items []app.DiscoveredSkill) {
	for _, item := range items {
		if len(item.Excluded) == 0 {
			continue
		}
		left := make([]string, 0, len(item.Excluded))
		for _, file := range item.Excluded {
			left = append(left, fmt.Sprintf("%s (%s)", file.Path, file.Reason))
		}
		p.Line(fmt.Sprintf("Left out of %s: %s", item.TargetID, strings.Join(left, ", ")))
	}
}
