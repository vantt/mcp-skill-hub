package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

func runtimeHintFindings(skillID string, hints skillruntime.Hints) []Warning {
	var warnings []Warning

	// absolute_install_path
	if len(hints.AbsoluteInstallPaths) > 0 {
		paths := strings.Join(hints.AbsoluteInstallPaths, ", ")
		warnings = append(warnings, Warning{
			Code:    "absolute_install_path",
			Summary: fmt.Sprintf("%s: references absolute install paths (%s). Fix: Reference files relative to the skill folder ($SKILLHUB_SKILL_DIR)", skillID, paths),
		})
	}

	// missing_runtime_block
	if hints.MissingRuntimeBlock {
		warnings = append(warnings, Warning{
			Code:    "missing_runtime_block",
			Summary: fmt.Sprintf("%s: missing runtime block. Fix: Declare a runtime block (bins, env, setup) with `skillhub skill edit %s --runtime-file <yaml>`", skillID, skillID),
		})
	}

	// install_prose_without_runtime
	if hints.InstallProseDetected {
		cues := strings.Join(hints.InstallCues, ", ")
		warnings = append(warnings, Warning{
			Code:    "install_prose_without_runtime",
			Summary: fmt.Sprintf("%s: installation prose detected (%s) without a runtime block. Fix: Turn the install steps into runtime.setup that installs into $SKILLHUB_STATE_DIR", skillID, cues),
		})
	}

	// missing_lockfile
	if len(hints.MissingLockfiles) > 0 {
		entries := strings.Join(hints.MissingLockfiles, ", ")
		warnings = append(warnings, Warning{
			Code:    "missing_lockfile",
			Summary: fmt.Sprintf("%s: missing lockfiles for dependency manifests (%s). Fix: Commit a lockfile or pin versions", skillID, entries),
		})
	}

	return warnings
}

func canonicalRuntimeHints(root, id string) (skillruntime.Hints, error) {
	_, skillRelDir, skillMetaBytes, err := locateSkillDir(root, id)
	if err != nil {
		return skillruntime.Hints{}, err
	}
	entrypointRelPath, entrypointDigest, _ := inspectCanonicalEntrypoint(root, skillRelDir)
	fullSkillDir := filepath.Join(root, filepath.FromSlash(skillRelDir))
	_, resources, _ := inventorySkillResources(root, fullSkillDir, entrypointRelPath, entrypointDigest)
	_, hasRuntimeBlock := reviewRuntimeSpec(skillMetaBytes)
	return reviewRuntimeHints(root, skillRelDir, resources, hasRuntimeBlock), nil
}

func workspaceLintWarnings(ctx context.Context, root string) []Warning {
	handle, err := catalog.OpenCurrentLocked(ctx, root)
	if err != nil {
		return []Warning{{
			Code:    "lint_skipped",
			Summary: "Metadata lint skipped: catalog generation unavailable. Run `skillhub rebuild` first.",
		}}
	}
	defer handle.Close()

	sqliteCatalog, err := resolverpkg.NewSQLiteCatalog(handle.DB, handle.Pointer.CatalogSnapshot)
	if err != nil {
		return []Warning{{
			Code:    "lint_skipped",
			Summary: fmt.Sprintf("Metadata lint skipped: %v", err),
		}}
	}

	allSkills, err := sqliteCatalog.Skills(ctx)
	if err != nil {
		return []Warning{{
			Code:    "lint_skipped",
			Summary: fmt.Sprintf("Metadata lint skipped: %v", err),
		}}
	}

	var activeSkills []resolverpkg.Skill
	for _, s := range allSkills {
		if s.Status == "active" && s.ID != "system-curator" {
			activeSkills = append(activeSkills, s)
		}
	}

	var warnings []Warning

	// 1. Routing lint
	routingFindings := resolverpkg.LintSkills(activeSkills)
	for _, f := range routingFindings {
		warnings = append(warnings, Warning{
			Code:    f.Code,
			Summary: fmt.Sprintf("%s: %s Fix: %s", f.SkillID, f.Message, f.Fix),
		})
	}

	// 2. Runtime hint lint
	for _, s := range activeSkills {
		hints, hintErr := canonicalRuntimeHints(root, s.ID)
		if hintErr == nil {
			warnings = append(warnings, runtimeHintFindings(s.ID, hints)...)
		}
	}

	return warnings
}
