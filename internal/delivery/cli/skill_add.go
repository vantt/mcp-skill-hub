package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

// runSkillAdd executes the intent-first skill add workflow.
func runSkillAdd(ctx context.Context, service app.SkillService, flags skillFlags, stdout, stderr io.Writer) int {
	locator := flags.locator
	if locator == "" && len(flags.positionals) > 0 {
		locator = flags.positionals[0]
	}
	if locator == "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"add requires a locator", "Run `skillhub skill add <locator> [--skill <name> | --all] [--id <id>] [--yes]`.")
	}

	if flags.all && len(flags.skills) > 0 {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"--all and --skill cannot be combined", "Select specific skills with --skill or import all with --all.")
	}
	if flags.all && flags.id != "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"cannot specify --id when adding multiple skills", "Omit --id when using --all.")
	}

	// Format locator with ref and path if explicit
	resolvedLocator := locator
	if strings.Contains(locator, "://") || strings.HasPrefix(locator, "git@") {
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
	} else if flags.subPath != "" {
		resolvedLocator = filepath.Join(locator, flags.subPath)
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
		FullDiff:       flags.fullDiff,
		IdempotencyKey: flags.idempotencyKey,
	}

	addService := app.SkillAddService{}
	preview, err := addService.PreviewSkillAdd(ctx, flags.workspace, input)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	if !flags.yes {
		return writeResult(stdout, stderr, flags.jsonOutput, preview, func(p *termui.Printer) {
			writeSkillAddPreview(p, preview)
		})
	}

	// Apply immediately with fresh preview pins
	result, err := addService.ConfirmSkillAdd(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}

	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		writeSkillAddResult(p, flags.verbose, result)
	})
}

func writeSkillAddPreview(p *termui.Printer, preview app.SkillAddProposal) {
	skillName := preview.SkillID
	if len(preview.SkillIDs) > 1 {
		skillName = strings.Join(preview.SkillIDs, ", ")
	}

	originDesc := preview.Origin.Kind
	if preview.Origin.Repository != "" {
		originDesc = preview.Origin.Repository
		if preview.Origin.Path != "" {
			originDesc += ", " + preview.Origin.Path
		}
		if preview.Origin.Commit != "" {
			c := preview.Origin.Commit
			if len(c) > 12 {
				c = c[:12]
			}
			originDesc += " at " + c
		}
	} else if preview.Origin.Path != "" {
		originDesc = preview.Origin.Path
	}

	p.Line(fmt.Sprintf("Add %s as a draft from %s.", skillName, originDesc))
	p.Line(fmt.Sprintf("%s, %s. Origin retained. Agent use: off.",
		termui.Plural(len(preview.Resources), "file", "files"), termui.Bytes(preview.TotalBytes)))
	if preview.UpstreamSource != nil {
		if preview.UpstreamSource.Created {
			p.Line(fmt.Sprintf("Upstream: tracked by new source %s (checked weekly by `skillhub check`; nothing runs in the background).", preview.UpstreamSource.SourceID))
		} else {
			p.Line(fmt.Sprintf("Upstream: tracked by source %s.", preview.UpstreamSource.SourceID))
		}
	}
	p.Line("No collection files changed.")
	pins := preview.Confirmation.Confirmation.Pins
	p.Line(fmt.Sprintf("Next: skillhub skill confirm %s", pins.ProposalID))
}

func writeSkillAddResult(p *termui.Printer, verbose bool, result app.SkillAddResult) {
	skillName := result.SkillID
	if len(result.SkillIDs) > 1 {
		skillName = strings.Join(result.SkillIDs, ", ")
	}
	if result.UpstreamSource != nil {
		p.Line(fmt.Sprintf("Draft %s added. Agent use: off. Upstream: %s. Changes are not committed.", skillName, result.UpstreamSource.SourceID))
	} else {
		p.Line(fmt.Sprintf("Draft %s added. Agent use: off. Changes are not committed.", skillName))
	}
	if verbose {
		p.Fields(
			termui.Field{Label: "Operation", Value: result.OperationID},
			termui.Field{Label: "Catalog snapshot", Value: result.CatalogSnapshot},
			termui.Field{Label: "Generation", Value: result.Generation},
		)
	}
	p.Line(fmt.Sprintf("Next: skillhub skill review %s", result.SkillID))
}
