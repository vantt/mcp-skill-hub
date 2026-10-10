package cli

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
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
		result, err := service.ConfirmSourceImport(ctx, flags.workspace, preview, app.ConfirmationPins{
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
	if !flags.yes || preview.Error != nil || len(preview.Importable) == 0 {
		return writeSourceImportProposal(stdout, stderr, flags.jsonOutput, preview)
	}
	result, err := service.ConfirmSourceImport(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
	return writeSourceImportResult(stdout, stderr, flags.jsonOutput, result, err)
}
