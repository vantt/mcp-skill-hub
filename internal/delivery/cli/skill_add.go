package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
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
	if preview.Error != nil {
		if flags.jsonOutput {
			if writeErr := writeJSON(stdout, preview); writeErr != nil {
				fmt.Fprintln(stderr, writeErr)
				return 1
			}
			return 2
		}
		fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n",
			preview.Error.Render.Error, preview.Error.Render.Why, preview.Error.Render.Fix)
		return 2
	}

	if !flags.yes {
		if flags.jsonOutput {
			if writeErr := writeJSON(stdout, preview); writeErr != nil {
				fmt.Fprintln(stderr, writeErr)
				return 1
			}
			return 0
		}
		return writeSkillAddPreview(stdout, preview)
	}

	// Apply immediately with fresh preview pins
	result, err := addService.ConfirmSkillAdd(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	if result.Error != nil {
		if flags.jsonOutput {
			if writeErr := writeJSON(stdout, result); writeErr != nil {
				fmt.Fprintln(stderr, writeErr)
				return 1
			}
			return 2
		}
		fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n",
			result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
		return 2
	}

	if flags.jsonOutput {
		if writeErr := writeJSON(stdout, result); writeErr != nil {
			fmt.Fprintln(stderr, writeErr)
			return 1
		}
		return 0
	}

	return writeSkillAddResult(stdout, stderr, flags.jsonOutput, flags.verbose, result, flags.workspace)
}

func writeSkillAddPreview(stdout io.Writer, preview app.SkillAddProposal) int {
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

	fmt.Fprintf(stdout, "Add %s as a draft from %s.\n", skillName, originDesc)
	fmt.Fprintf(stdout, "%d files, %d bytes. Origin retained. Watching: off. Agent use: off.\n",
		len(preview.Resources), preview.TotalBytes)
	fmt.Fprintln(stdout, "No collection files changed.")
	pins := preview.Confirmation.Confirmation.Pins
	fmt.Fprintf(stdout, "Next: skillhub skill confirm %s\n", pins.ProposalID)
	return 0
}

func writeSkillAddResult(stdout, stderr io.Writer, jsonOutput, verbose bool, result app.SkillAddResult, workspacePath string) int {
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	skillName := result.SkillID
	if len(result.SkillIDs) > 1 {
		skillName = strings.Join(result.SkillIDs, ", ")
	}
	fmt.Fprintf(stdout, "Draft %s added. Agent use: off. Watching: off. Changes are not committed.\n", skillName)
	if verbose {
		fmt.Fprintf(stdout, "Operation: %s\nCatalog snapshot: %s\nGeneration: %s\n",
			result.OperationID, result.CatalogSnapshot, result.Generation)
	}
	fmt.Fprintf(stdout, "Next: skillhub skill review %s\n", result.SkillID)
	return 0
}
