# Phase 11: Metadata lint (routing and runtime hints) report

## Summary
- Implemented routing lint in `internal/resolver/lint.go`:
  - `LintSkills(skills []Skill) []LintFinding` and `LintTargetSkill(target Skill, activeSkills []Skill) []LintFinding`.
  - Rules: `trigger_collision` (overlap >= 0.8 without mutual relationship suppression), `generic_trigger` (triggers composed solely of generic stoplist tokens or < 2 tokens), `missing_examples` (active skills with < 3 examples), `example_restates_trigger` (example overlap >= 0.9 with a trigger), and `near_duplicate` (description + triggers overlap >= 0.6 without mutual relationship suppression).
  - Skips pairwise checks when active skills exceed 2,000, emitting `lint_pairs_skipped`.
- Implemented runtime-hint lint in `internal/app/metadata_lint.go`:
  - `runtimeHintFindings(skillID string, hints skillruntime.Hints) []Warning` covering `absolute_install_path`, `missing_runtime_block`, `install_prose_without_runtime`, and `missing_lockfile`.
  - Ensured no file content is leaked in warning summaries (reporting only file paths, cue names, and fix instructions).
- Shared helper extraction:
  - Extracted `canonicalRuntimeHints(root, id string) (skillruntime.Hints, error)` used consistently across `ReviewSkill` and `ValidateWorkspace`.
- Integrated with `ValidateWorkspace` in `internal/app/operations.go`:
  - Appends routing and runtime hint warnings after canonical validation succeeds.
  - Returns `lint_skipped` if catalog generation is unavailable.
- Integrated with `ReviewSkill` in `internal/app/skill_review.go`:
  - Exports `resolver.DecodeSkillDocument` to project reviewed skills (including drafts).
  - Evaluates routing rules against all active skills and appends findings to `readiness.Warnings`.
- Updated CLI output in `internal/delivery/cli/workspace.go`:
  - `skillhub validate` human output prints `WARN <code> <summary>` lines after the summary without failing validation (exit 0).
  - JSON output carries the `warnings` array in `app.Result`.

## Verification
- Unit & regression tests:
  - `go test -count=1 -run Lint ./internal/resolver/` (PASS)
  - `go test -count=1 -run 'MetadataLint|RuntimeHint' ./internal/app/` (PASS)
  - `go test -count=1 -run 'Validate|Review' ./internal/app/` (PASS)
  - `go test -count=1 ./internal/delivery/web/` (PASS)
  - `go test -count=1 -run 'Validate' ./internal/delivery/cli/` (PASS)
  - `go test -v -count=1 -run RoutingEvalGate ./internal/app/` (PASS)
- Quality gate:
  - `make check` (vet, golangci-lint with 0 issues, go test across all packages) passes cleanly.
