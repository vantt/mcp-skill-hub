# Phase 1: Hub Portability

## Goal
Ensure a fresh `git clone` of a hub workspace works out of the box without requiring manual directory creation or failing on missing host integration files. Read paths (`status`, `validate`, `doctor`) must tolerate missing empty layout directories. Missing host files must be reported as a fixable finding with clear remediation guidance. Curator skill metadata must declare all tools its instructions invoke.

## Context & Constraints
- Plan decisions: D4, D10; Red Team finding S12.
- A fresh git clone lacks empty layout directories created by `init` (Git cannot track empty directories).
- Untracking host integration files (commit `a45b274`) deletes them on existing clones after a `git pull`.
- Do NOT add lazy creation for directories later phases delete (`sources/intake`, `sources/catalog`, `sources/skills`, `distill/sources`, `distill/comparisons`, `distill/skills`, `history/operations`).
- Server-boundary tool tests belong to Phase 3; do not write them now.

## Files to Modify
- `internal/workspace/workspace.go`: ensure inspect and layout helpers tolerate missing directories.
- `internal/app/source.go`: update `readSourceRecords` so missing `sources/intake` or `sources/catalog` directories do not cause `statat` errors on read paths.
- `internal/hostintegration/types.go` & `internal/hostintegration/integration.go`: export `Finding` and helper `MissingFinding(plan PlanResult) *Finding` for missing host integration.
- `internal/app/workspace.go`: report `host_integration_missing` as a fixable finding in `doctor`, with a note to re-run `skillhub integrate` after a pull that untracked those files.
- `internal/app/curation_home.go`: check host integration in `status` and report fixable `host_integration_missing` item when host files are missing.
- `internal/delivery/cli/root.go`: support `integrate` as an alias for `connect`.
- `internal/systemskills/embed.go`: bump `CuratorSkillVersion` to `1.5.2`, add missing tools (`skill_add_preview`, `skill_add_confirm`, `skill_review`, `source_watch_confirm`) to `curatorCompatibleTools`.
- `internal/systemskills/curator/SKILL.md` & `system-skills/curator/SKILL.md`: update version to `1.5.2` and add the 4 tools to `compatible-tools` (keep identical).

## Implementation Steps
1. Update `internal/systemskills/embed.go`, `internal/systemskills/curator/SKILL.md`, and `system-skills/curator/SKILL.md` to include `skill_add_preview`, `skill_add_confirm`, `skill_review`, and `source_watch_confirm`. Bump version to 1.5.2.
2. Modify `readSourceRecords` in `internal/app/source.go` so `errors.Is(statErr, os.ErrNotExist)` returns empty lists instead of failing.
3. In `internal/hostintegration`, add finding definition and detection for missing host integration files (`FindingHostIntegrationMissing`, note to re-run `skillhub integrate`).
4. In `internal/app/workspace.go` (`Doctor`), include `host_integration_missing` finding item when host files are missing.
5. In `internal/app/curation_home.go` (`GetCurationHome`), check host integration and report `host_integration_missing` action item and finding when host files are missing.
6. In `internal/delivery/cli/root.go`, add `integrate` command alias to `connect`.

## Verification & Tests
- `TestFreshCloneWithMissingLayoutDirectoriesGivesHealthyStatusAndValidate`: create workspace, delete all empty layout directories, assert `status` and `validate` exit 0 and report healthy workspace.
- `TestDoctorAndStatusReportHostIntegrationMissing`: remove host integration files, assert both `doctor` and `status` report `host_integration_missing` as a fixable finding with note to re-run `skillhub integrate`.
- `TestCuratorToolsMetadataMatchesSkillMD`: assert embedded metadata and both curator `SKILL.md` files are identical and include the 4 newly added tools.

## Acceptance Criteria
- Fresh clone without layout directories succeeds on `status`, `validate`, `doctor`.
- Missing host integration files produce a fixable finding with the required note.
- Curator metadata and files are synchronized at version 1.5.2 with all 4 tools.
