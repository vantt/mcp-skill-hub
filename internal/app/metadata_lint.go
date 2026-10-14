package app

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/skillruntime"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
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

// lintScope says which skills the example check covers and where their new
// versions are read from.
type lintScope struct {
	// Full checks the own examples of every active skill instead of the
	// skills that changed.
	Full bool
	// Snapshot is the directory of a staged index snapshot. When set, the
	// changed skills are the staged ones and a hub without a catalog
	// generation is skipped without a warning.
	Snapshot string
}

func workspaceLintWarnings(ctx context.Context, root string, scope lintScope) []Warning {
	return append(secretLikeFileWarnings(ctx, root), catalogLintWarnings(ctx, root, scope)...)
}

// selfResolutionWarnings reports the routing examples that no longer resolve as
// their author meant. The changed skills are the ones that differ from HEAD,
// or every active skill when asked for or when Git cannot tell.
func selfResolutionWarnings(ctx context.Context, root string, scope lintScope) []Warning {
	changed, mode, ok := changedForLint(ctx, root, scope)
	if !ok {
		return nil
	}
	var findings []resolverpkg.LintFinding
	if mode == selfCheckChanges && changed.policyChanged {
		findings = []resolverpkg.LintFinding{resolverpkg.SelfResolutionSkipped("", "the recommendation policy changed and the served catalog may still have the old one", "Rebuild, then run skillhub maintain validate --full")}
	} else if findings = checkedChanges(ctx, root, scope, changed, mode); len(findings) == 0 {
		return nil
	}
	warnings := make([]Warning, 0, len(findings))
	for _, f := range findings {
		summary := fmt.Sprintf("%s Fix: %s", f.Message, f.Fix)
		if f.SkillID != "" {
			summary = f.SkillID + ": " + summary
		}
		warnings = append(warnings, Warning{Code: f.Code, Summary: summary})
	}
	return warnings
}

// checkedChanges replays the examples against the current generation. A
// catalog that is not current is skipped: the pair lint has already said so.
func checkedChanges(ctx context.Context, root string, scope lintScope, changed changedSkills, mode selfCheckMode) []resolverpkg.LintFinding {
	view, err := catalog.ObserveCurrentLocked(ctx, root)
	if err != nil {
		return nil
	}
	defer view.Close()
	policy, err := generationPolicy(ctx, view)
	if err != nil {
		return nil
	}
	if mode == selfCheckFull {
		return checkSkillChanges(ctx, view, policy, nil, selfCheckFull)
	}
	newRead := directoryReader(root)
	if scope.Snapshot != "" {
		newRead = directoryReader(scope.Snapshot)
	}
	changes := changed.changes(newRead, headReader(ctx, root))
	if len(changes) == 0 {
		return nil
	}
	return checkSkillChanges(ctx, view, policy, changes, selfCheckChanges)
}

// changedForLint finds the skills to check and how. It reports false when
// nothing can be checked.
func changedForLint(ctx context.Context, root string, scope lintScope) (changedSkills, selfCheckMode, bool) {
	if scope.Full {
		return changedSkills{}, selfCheckFull, true
	}
	if scope.Snapshot != "" {
		paths, ok := stagedChangedPaths(ctx, root)
		return parseChangedPaths(paths), selfCheckChanges, ok
	}
	paths, ok := workingTreeChangedPaths(ctx, root)
	if !ok {
		return changedSkills{}, selfCheckFull, true
	}
	return parseChangedPaths(paths), selfCheckChanges, true
}

const secretLikeFileListLimit = 20

// secretLikeFileWarnings reports files under skills/ that an import would have
// left out, such as .env files a hub received before imports skipped them. It
// reads names only, never contents, and works without a catalog generation.
func secretLikeFileWarnings(ctx context.Context, root string) []Warning {
	var found []string
	skillsDir := filepath.Join(root, "skills")
	_ = filepath.WalkDir(skillsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || ctx.Err() != nil {
			return fs.SkipAll
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, skip := sourcepkg.ExcludedFromImport(rel); skip {
			found = append(found, rel)
			if entry.IsDir() {
				return fs.SkipDir
			}
		}
		return nil
	})
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)
	listed := found
	more := ""
	if len(listed) > secretLikeFileListLimit {
		more = fmt.Sprintf(" and %d more", len(listed)-secretLikeFileListLimit)
		listed = listed[:secretLikeFileListLimit]
	}
	return []Warning{{
		Code: "secret_like_file",
		Summary: fmt.Sprintf("Files that look like secrets or build output are inside skill folders: %s%s. Fix: Remove it from the hub with `git rm --cached` and delete it, or move the secret out of the skill folder.",
			strings.Join(listed, ", "), more),
	}}
}

func catalogLintWarnings(ctx context.Context, root string, scope lintScope) []Warning {
	handle, err := catalog.OpenCurrentLocked(ctx, root)
	if err != nil {
		if scope.Snapshot != "" {
			return nil
		}
		return []Warning{{
			Code:    "lint_skipped",
			Summary: "Metadata lint skipped: catalog generation unavailable. Run `skillhub maintain rebuild` first.",
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

	// 3. Routing examples of the changed skills
	warnings = append(warnings, selfResolutionWarnings(ctx, root, scope)...)

	return warnings
}
