---
phase: 11
title: "Metadata lint (routing and runtime hints)"
status: pending
priority: P2
effort: 8h
dependencies: [5b, 9]
---

# Phase 11: Metadata lint (routing and runtime hints)

## Goal

`skillhub validate` and `skillhub skill review` warn about routing metadata that will route badly (collisions, generic triggers, missing or trigger-restating examples, near-duplicates), and `skillhub validate` also warns about runtime packaging problems already detected by `skillruntime.AnalyzeHints` (absolute install paths, missing runtime block, install prose without a runtime block, missing lockfiles). Warnings never fail validation.

## Context (read these first)

- `plan.md` → "Executor notes". Design rule: "Duplicate detection/evaluation creates curation warnings for frequently ambiguous pairs" (`docs/design/03-resolver-design.md` §3 "Validation rules").
- Validate path: CLI `executeValidate` (`internal/delivery/cli/validate_staged.go:41`) → `app.WorkspaceService.ValidateWorkspace` (`internal/app/operations.go:19`) → `canonical.Validate` → `formatValidationResult` (`operations.go:33`). Staged validation: `ValidateWorkspaceStaged` (`internal/app/git_index_snapshot.go:235`). Human output prints only `result.Summary` on success (`internal/delivery/cli/workspace.go:107-109`).
- Warning contract: `app.Warning{Code, Summary}` (`internal/app/result.go:64`); `schemas/result-envelope.schema.json` `$defs.warning` has `additionalProperties: false` with only `code` and `summary`. Put the fix text inside `summary` (`"<skill>: <message> Fix: <fix>"`); do not add fields.
- Review path: `app.SkillService.ReviewSkill` (`internal/app/skill_review.go:114`); `ActivationReadiness.Warnings []string` (`skill_review.go:24-29`). Review already returns `runtime_hints` (`skillruntime.Hints`, built at `skill_review.go:150-151` with `reviewRuntimeSpec` (`:409`) and `reviewRuntimeHints` (`:381`)) and the CLI prints a "Runtime" section, so review does **not** repeat runtime-hint warnings.
- Runtime hints API (reuse, do not duplicate detection): `skillruntime.AnalyzeHints(files []HintFile, hasSpec bool) Hints` (`internal/skillruntime/hints.go:64`); `Hints{Interpreters, DependencyManifests, MissingLockfiles, AbsoluteInstallPaths, MissingRuntimeBlock, InstallProseDetected, InstallCues}` (`hints.go:21-29`). Helpers for a canonical skill: `locateSkillDir` (`skill_review.go:221`), `inspectCanonicalEntrypoint` (`:257`), `inventorySkillResources` (`:308`).
- Resolver tokenizer and overlap helpers (unexported, same package as the new lint): `stopWords` (`internal/resolver/evidence.go:150`), `tokenize` (`:152`), `overlap` (`:170`), `bestOverlap` (`:188`). Active skills projection: `resolver.NewSQLiteCatalog(db, snapshot)` (`sqlite_catalog.go:82`) → `Skills(ctx)` (`:115`), routing decoded by `decodeRouting` (`:206`); phase 9 adds `Examples`, `CounterExamples`, `Topics`, `Technologies`.

## Requirements

1. **Routing lint** `resolver.LintSkills(skills []Skill) []LintFinding` in `internal/resolver/lint.go`; `LintFinding{Code, SkillID, OtherSkillID, Field, Value, Message, Fix string}`. Rules (warnings):

   | Code | Condition | Fix text |
   |---|---|---|
   | `trigger_collision` | A trigger of A and a trigger of B overlap ≥ 0.8 (or are equal after normalization), and neither names the other in `distinguish_from`, `equivalent_to`, or `not_for`/`counter_examples` (overlap ≥ 0.5 with the other's name or triggers) | Add `distinguish_from` or narrow one trigger |
   | `generic_trigger` | Every token of a trigger is in the generic stoplist (`code, coding, help, fix, task, work, write, create, update, change, build, make, do, run, use, general, stuff, thing, project, app, file, review, test, debug, issue, problem`) or the trigger has fewer than 2 tokens after `tokenize` | Make the trigger name the domain object |
   | `missing_examples` | Active skill with fewer than 3 `routing.examples` | Add 3–5 natural task phrasings |
   | `example_restates_trigger` | An example overlaps ≥ 0.9 with one of the same skill's triggers | Rephrase as a real request |
   | `near_duplicate` | Two active skills whose description + triggers token sets overlap ≥ 0.6 with no `distinguish_from`, `equivalent_to`, or cross `not_for` between them | Add `distinguish_from` or `equivalent_to`, or merge |

   Pair rules are O(n²); skip them above 2,000 active skills with one warning `lint_pairs_skipped`.
2. **Runtime-hint lint** `runtimeHintFindings(skillID string, hints skillruntime.Hints) []app.Warning` in `internal/app/metadata_lint.go`, converting existing hints only:

   | Code | Condition (from `Hints`) | Fix text |
   |---|---|---|
   | `absolute_install_path` | `len(AbsoluteInstallPaths) > 0` (one warning listing the file paths) | Reference files relative to the skill folder (`$SKILLHUB_SKILL_DIR`) |
   | `missing_runtime_block` | `MissingRuntimeBlock` | Declare a `runtime` block (bins, env, setup) with `skillhub skill edit <id> --runtime-file <yaml>` |
   | `install_prose_without_runtime` | `InstallProseDetected` and the skill has no runtime block | Turn the install steps into `runtime.setup` that installs into `$SKILLHUB_STATE_DIR` |
   | `missing_lockfile` | `len(MissingLockfiles) > 0` (one warning listing entries) | Commit a lockfile or pin versions |

   Messages list cue names and paths only, never file content.
3. **Validate integration** (`ValidateWorkspace`): after canonical validation passes, open the current catalog generation, run `LintSkills` over active skills, compute runtime hints per active skill from canonical files (extract a shared helper `canonicalRuntimeHints(root, id string) (skillruntime.Hints, error)` from the `ReviewSkill` code path so both use one implementation), and append all findings to `Result.Warnings`. Canonical validation results are unchanged. If the catalog is unavailable, append one `lint_skipped` warning. `ValidateWorkspaceStaged` skips lint.
4. **Review integration** (`ReviewSkill`): run the routing rules for the reviewed skill against all active skills (the reviewed skill may be a draft; project it from canonical metadata through an exported `resolver.DecodeSkillDocument(id string, contentJSON []byte) (Skill, error)` wrapping `decodeRouting`) and append findings as strings `"<code>: <message> Fix: <fix>"` to `ActivationReadiness.Warnings`.
5. **Human output** of `skillhub validate` prints warnings after the summary as `WARN <code> <summary>`; exit code stays 0 when only warnings exist. JSON output carries `warnings`.

## Files

Create:
- `internal/resolver/lint.go`, `internal/resolver/lint_test.go`
- `internal/app/metadata_lint.go`, `internal/app/metadata_lint_test.go`

Modify:
- `internal/resolver/sqlite_catalog.go` (export `DecodeSkillDocument`)
- `internal/app/operations.go` (`ValidateWorkspace` appends lint warnings)
- `internal/app/skill_review.go` (extract `canonicalRuntimeHints`; review appends routing lint)
- `internal/app/skill_review_test.go`, `internal/app/workspace_test.go` (or a new `internal/app/validate_lint_test.go`)
- `internal/delivery/cli/workspace.go` (print warnings), `internal/delivery/cli/ux_behavior_test.go` or a new focused CLI test
- `internal/delivery/web/testdata/golden/skill-review.json` only if review output changes for the fixture (regenerate with `-update`; the fixture skill has 0 examples, so expect a `missing_examples` warning)

## Steps

- [ ] **1. Routing rules** with table tests: one positive and one suppressed case per rule; pair-skip above 2,000.
  Pass: `go test -count=1 -run Lint ./internal/resolver/` → `ok`.
- [ ] **2. Runtime-hint findings** with table tests per code, using `skillruntime.AnalyzeHints` on synthetic `HintFile`s; assert no file content appears in any summary (sentinel string in a file body).
  Pass: `go test -count=1 -run 'MetadataLint|RuntimeHint' ./internal/app/` → `ok`.
- [ ] **3. Wire validate and review;** extract `canonicalRuntimeHints` (review output for existing tests stays identical apart from new warnings).
  Pass: `go test -count=1 -run 'Validate|Review' ./internal/app/` → `ok`; `go test -count=1 ./internal/delivery/web/` → `ok` (after regenerating `skill-review.json` if needed).
- [ ] **4. CLI output.** A workspace with a seeded trigger collision and a skill referencing `~/.claude/skills/` in SKILL.md: `skillhub validate --json` shows `trigger_collision` and `absolute_install_path` in `warnings` and exits 0; human output prints `WARN` lines.
  Pass: `go test -count=1 -run 'Validate' ./internal/delivery/cli/` → `ok`.
- [ ] **5. Corpus check.** Run lint over the phase 10 materialized golden-v1 workspace and the overlay fixture; fix fixture metadata only for genuine defects; note accepted findings in `reports/routing-eval-baseline-report.md`.
  Pass: `make check` still passes the routing gate.
- [ ] **6. Gate.** Pass: `make check` exits 0.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Noisy warnings erode trust | Medium × Low | Conservative thresholds; suppression through relationship fields; every warning carries a fix. |
| Validate slows on large workspaces (file reads for hints) | Low × Low | Hints read at most 256 KiB per file (`hintReadLimit`); only active skills; pair cap at 2,000. |
| Runtime and routing warnings duplicate review's Runtime section | Low × Low | Review appends routing lint only; validate carries both. |

## Rollback

Revert; warnings disappear, no data changes.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol": write `reports/<agent>-<YYMMDD-HHMM>-metadata-lint.md`, set `status: blocked`, report the blocker.
