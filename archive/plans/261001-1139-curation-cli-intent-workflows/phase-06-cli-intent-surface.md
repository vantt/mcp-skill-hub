---
phase: 6
title: "CLI intent surface"
status: complete
priority: P1
effort: "4-5d"
dependencies: [1, 2, 3, 4, 5]
---

# Phase 6: CLI intent surface

## Context Links

- [Accepted command grammar](./reports/curation-ux-cli-independent-evaluation.md#5-input-resolution-and-normalization)
- [CLI conventions evidence](./reports/researcher-261001-1644-cli-conventions-evidence.md)
- [BUG-03, BUG-08, BUG-12, BUG-14, BUG-15, BUG-17](./reports/bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md)

## Objective

Expose the frozen application workflows through a discoverable, script-compatible CLI; repair editor recovery and misleading output; and prove each new intent through the actual binary.

## Command Contract

```text
skillhub status
skillhub skill add LOCATOR [--skill S ... | --all] [--id ID] [--collection C] [--ref R] [--path P] [--yes]
skillhub skill create ID [existing options]                 # --id remains valid
skillhub skill show ID
skillhub skill review ID [--verbose]
skillhub skill edit ID [--editor | --content-file FILE | field flags]
skillhub skill confirm PROPOSAL_ID                          # legacy three-pin form remains valid
skillhub source watch LOCATOR [--id ID] [--ref R] [--path P] [--cadence daily|weekly|manual] [--yes]
skillhub source check SELECTOR... | --all-due | --all       # alias of top-level check
skillhub validate [--staged]
skillhub diff
```

Human mutations preview by default. `--yes` confirms only the fresh preview from that invocation. JSON stdout remains one valid result envelope; diagnostics/warnings use the established error/result model and never corrupt stdout.

## Exclusive File Ownership

Modify:

- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/root.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/help.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/skill.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/skill_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/source.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/source_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/workspace.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/curation.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/curation_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/onboarding_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/ux_behavior_test.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/status_schema_test.go`

May create command-focused files/tests under `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/` with prefixes `skill_add_`, `skill_review_`, `skill_editor_`, `source_watch_`, or `validate_staged_`.

Own new/updated fixtures under `/home/vantt/projects/mcp-skill-hub/testdata/ux/`.

No other phase edits these files.

## Implementation Steps

1. Add command parsers/help/examples without removing old spellings. Positional create ID and `--id` are mutually consistent; duplicate/conflicting values fail before workspace/network access.
2. Map `skill add`, `skill review`, and `source watch` directly to application services. Do not orchestrate capture/triage/import in the CLI.
3. Route `source check` and top-level `check` through one parser/runner so arguments, network behavior, telemetry, JSON schema, and exit codes cannot drift.
4. Implement short confirmation through Phase 3's proposal-kind dispatcher. Load the named exact proposal, supply stored digest/base pins, then call the owning lifecycle/add confirmer. Keep the legacy flag form; never select “latest” or regenerate.
5. Repair editor lifecycle. Capture original digest; place edited bytes in a private recovery artifact before preview; bind it to the persisted proposal ID; pass expected digest; and show entrypoint diff by default.
6. Recovery artifacts use mode-0700 directory/mode-0600 regular files, proposal-equivalent per-file/count/aggregate bounds, a 24-hour TTL, and cleanup on editor start/confirm/status. Human output may print the local path; JSON returns only an opaque recovery ID. Successful application deletes atomically; failure/crash/stale confirmation retains until recovery or expiry.
7. Remove editor advice to rerun with `--yes`; print the exact short confirm command. Preserve non-editor fresh-preview `--yes` guidance where accurate.
8. Render explicit draft/watch effects and basis-labeled current-versus-served review/fallback facts. Map only Phase 1's typed error vocabulary; no adapter-local codes or string classification.
9. Fix remaining output defects: unknown counts never become “No skills yet”; commit hint uses actual workspace; diff groups by skill rather than labeling files active; status prints a runnable create command.
10. Wire `validate --staged` to Phase 1. Reject incompatible flags before Git access and preserve default working-tree validation.
11. Add human/JSON behavior tests, including cross-process preview→stale-confirm recovery, then smoke the compiled CLI.

## Todo

- [x] Add intent-first command grammar and help.
- [x] Add source-check alias with exact parity.
- [x] Add proposal-kind-dispatched human confirmation.
- [x] Preserve editor content with bounded recovery lifecycle and reject stale edits.
- [x] Render basis-aware review/fallback/state/resource facts.
- [x] Map shared typed errors identically in human/JSON modes.
- [x] Fix BUG-08/12/14/15/17 output paths.
- [x] Add staged validation adapter and CLI regression tests.

## Success Criteria

- All command examples above execute as printed.
- BUG-03: invalid/stale editor output survives under a bounded recovery ID/path; expired/abandoned artifacts are cleaned without following symlinks.
- BUG-12: editor preview shows actual content diff and only an exact confirm action.
- BUG-14/15/17: workspace hints, diff grouping, and first-run next action are truthful/runnable.
- Lifecycle and add proposals confirm across processes through one kind dispatcher; old explicit-pin CLI forms remain valid.
- Human failures are actionable/non-zero; JSON failures are valid envelopes with no prose or absolute recovery/source paths on stdout.

## Verification

```bash
go test ./internal/delivery/cli -run 'Test.*(Skill|Source|Review|Editor|Validate|Check|Status|Diff|Help|JSON)'
go test -race ./internal/delivery/cli

go build -o /tmp/skillhub-plan-smoke ./cmd/skillhub
/tmp/skillhub-plan-smoke help skill
/tmp/skillhub-plan-smoke help source
```

Run scripted disposable-workspace scenarios for local add, lifecycle/add proposal short confirmation, source watch/check alias, review of a broken skill, editor conflict/recovery/expiry, and staged validation.

## Risks and Security

- Short IDs resolve only inside the selected workspace and still verify proposal kind, digest, base snapshot, expiry, and exact write set.
- Recovery cleanup must use no-follow/private-root operations and never delete a path supplied by the user or proposal content.
- Do not print imported content, credentials, absolute origin paths, or unbounded diffs by default.
- One CLI owner handles shared parser files; do not split this phase among concurrent editors.

## Next Step

Run in parallel with Phase 7 after application contracts pass; Phase 8 follows both delivery surfaces.