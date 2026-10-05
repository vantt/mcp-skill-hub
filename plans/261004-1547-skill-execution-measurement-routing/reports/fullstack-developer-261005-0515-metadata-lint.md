# Phase 11: Metadata lint (routing and runtime hints) report

## Summary
- Implemented `resolver.LintSkills` in `internal/resolver/lint.go`:
  - `trigger_collision`: checks trigger overlap >= 0.8 or normalized equality, suppressed when either skill names the other in `distinguish_from`, `equivalent_to`, or cross `not_for`/`counter_examples`.
  - `generic_trigger`: flags triggers with < 2 tokens or composed entirely of stoplist terms.
  - `missing_examples`: flags active skills with fewer than 3 routing examples.
  - `example_restates_trigger`: flags examples overlapping >= 0.9 with triggers of the same skill.
  - `near_duplicate`: checks description + triggers overlap >= 0.6 without relationship suppression.
  - Pairwise lint cap: skips O(n²) comparisons when active skills count exceeds 2,000, emitting `lint_pairs_skipped`.
- Exported `resolver.DecodeSkillDocument` in `internal/resolver/sqlite_catalog.go` for projecting canonical metadata into skills without disk catalog writes.
- Implemented `runtimeHintFindings` in `internal/app/metadata_lint.go`:
  - Maps `skillruntime.Hints` to `app.Warning`s for `absolute_install_path`, `missing_runtime_block`, `install_prose_without_runtime`, and `missing_lockfile` with structured summary and fix instructions without leaking file bodies.
- Integrated linting into application workflows:
  - `ValidateWorkspace` in `internal/app/operations.go`: runs `LintSkills` and `runtimeHintFindings` across all active skills after canonical validation passes, appending findings to `Result.Warnings` (or `lint_skipped` if catalog unavailable).
  - `ReviewSkill` in `internal/app/skill_review.go`: extracts `canonicalRuntimeHints`, projects the reviewed skill, runs `LintSkills` against the active catalog, and appends routing warnings to `ActivationReadiness.Warnings`.
- Updated CLI output in `internal/delivery/cli/workspace.go`:
  - `skillhub validate` prints `WARN <code> <summary>` lines following summary; exits 0 when only warnings exist.
  - Verified JSON output carries warnings.
- Updated golden files:
  - Regenerated `internal/delivery/web/testdata/golden/skill-review.json` capturing the expected `missing_examples` warning on the fixture.
- Documented accepted corpus findings in `reports/routing-eval-baseline-report.md`:
  - 3 known intentional ambiguous pairs in golden-v1 (`api-design`/`graphql-api`, `code-review`/`pull-request-review`, `test-design`/`unit-testing`).

## Verification
- Unit & integration tests:
  - `go test -count=1 -run Lint ./internal/resolver/` (PASS)
  - `go test -count=1 -run 'MetadataLint|RuntimeHint' ./internal/app/` (PASS)
  - `go test -count=1 -run 'Validate|Review' ./internal/app/` (PASS)
  - `go test -count=1 -run 'Validate' ./internal/delivery/cli/` (PASS)
  - `go test -count=1 ./internal/delivery/web/` (PASS)
  - `make check` (go vet, golangci-lint 0 issues, full test suite across all 24 packages) (PASS)
