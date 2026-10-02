---
phase: 1
title: "Skill detail read model in app"
status: pending
priority: P1
effort: "3h"
dependencies: [0]
---

# Phase 1: Skill detail read model in app

## Goal

Move every business rule of the MCP `skill_get` tool into one application query, `SkillService.GetSkillDetail`, so MCP, CLI, and the future WebUI share it. The MCP JSON output must not change.

## Context

Today `internal/delivery/mcpserver/skill_tools.go` (handler of tool `skill_get`, lines ~138-200) does all of this inside the adapter:

- calls `service.ReadSkill`, `service.ReadSkillRouting`, and `service.ReadSkillRationale` (it ignores the rationale error with `_`);
- finds the entrypoint by scanning `skillResult.Manifest.Resources` for a path ending in `/SKILL.md`;
- computes `content_digest = "sha256:" + hex(sha256(content))`, which must equal the digest that `skill/lifecycle.go:278-290` compares as `ExpectedContentDigest`;
- calls `catalog.AssessSkillState` (it ignores the error with `_`);
- derives `lifecycle_state` (manifest status, or the assessment's canonical status when empty) and `routing_eligible = lifecycle_state == "active" && (!assessment.Served.Known || assessment.Served.Servable)`;
- maps `skill.ErrNotFound` to tool error code `not_found`.

The output type is `skillGetResult` in `internal/delivery/mcpserver/types.go:180-198`, with JSON keys, in order:
`skill_id, name, description, status, path, catalog_snapshot, content, content_digest, state_basis, lifecycle_state, routing_eligible, diverged, changed_resources (omitempty), missing_resources (omitempty), routing, rationale (omitempty), resources`.
`state_basis` is always `string(catalog.BasisCanonical)`.

## Files to Create / Modify

- Create: `internal/app/skill_detail.go`
- Modify: `internal/delivery/mcpserver/skill_tools.go` (only the `skill_get` handler and imports)
- Modify: `internal/delivery/mcpserver/types.go` (replace the `skillGetResult` struct with a type alias)

Do not modify any other file.

## Tasks

### Task 1.1 — Create `app.SkillDetail` and `GetSkillDetail`
- Goal: one application query returns everything `skill_get` returns today.
- Target: `internal/app/skill_detail.go`, type `SkillDetail`, method `func (service SkillService) GetSkillDetail(ctx context.Context, path, id string) (SkillDetail, error)`. Use the receiver name `service`, or none.
- Steps:
  1. Declare `SkillDetail` with exactly the 17 fields, Go types, and JSON tags of `skillGetResult` (copy them from `types.go`, same order, same `omitempty` flags).
  2. Implement `GetSkillDetail` by moving the handler logic described in Context, unchanged in behavior:
     - Call `service.ReadSkill(ctx, path, id)`. Return its error unchanged; it must still satisfy `errors.Is(err, skill.ErrNotFound)` for unknown IDs.
     - Call `service.ReadSkillRouting`. Return its error unchanged.
     - Call `service.ReadSkillRationale`. On error, keep `rationale` empty, exactly like today. Write it as `rationale, rationaleErr := ...` with an `if rationaleErr != nil { rationale = "" }` block and a one-line comment saying an unreadable rationale is treated as absent. Do not use `_`.
     - Find the entrypoint path and compute `ContentDigest` exactly as described.
     - Call `catalog.AssessSkillState(ctx, path, id)`. On error, use the zero `catalog.SkillStateAssessment{}`, exactly like today. Write it as `assessment, assessErr := ...` with an explicit `if assessErr != nil` block and a comment. Do not use `_`.
     - Derive `LifecycleState`, `RoutingEligible`, and `StateBasis` exactly as described.
  3. Add a doc comment on `GetSkillDetail` saying it is the shared read model for MCP `skill_get`, CLI, and WebUI.
- Success criteria: the file compiles; `GetSkillDetail` has no `, _ :=` or `, _ =`.
- Verify: `go build ./internal/app/` exits 0.

### Task 1.2 — Reduce the MCP handler to mapping
- Goal: the adapter only validates input, calls `GetSkillDetail`, maps errors, and returns.
- Target: `internal/delivery/mcpserver/skill_tools.go` (the `skill_get` handler); `internal/delivery/mcpserver/types.go` (`skillGetResult`).
- Steps:
  1. In `types.go`, replace the `skillGetResult` struct declaration with `type skillGetResult = app.SkillDetail`.
  2. In the handler, keep the `skill_id is required` check and the `not_found` mapping for `errors.Is(err, skill.ErrNotFound)`. Replace everything else with one call to `app.SkillService{}.GetSkillDetail(ctx, adapter.workspace, id)` followed by `return success(detail)` (or `failure[skillGetResult](err)` on other errors).
  3. Remove the imports that are now unused (`crypto/sha256`, `encoding/hex`, possibly `catalog`) from `skill_tools.go`, but only if nothing else in the file uses them.
- Success criteria: the handler is at most 25 lines; `skill_tools.go` no longer mentions `sha256`, `hex.Encode`, or `SKILL.md`; and **no** production file in `internal/delivery/mcpserver/` calls `AssessSkillState`, `ReadSkillRouting`, or `ReadSkillRationale` (moving that logic to another adapter file is not allowed).
- Verify: `grep -nE 'sha256|hex\.Encode|AssessSkillState|SKILL\.md|ReadSkillRouting|ReadSkillRationale' internal/delivery/mcpserver/skill_tools.go` prints nothing, and `go test -count=1 ./internal/delivery/mcpserver/` exits 0.

### Task 1.3 — Do not add tests; confirm the existing owners
- Goal: this phase is a pure refactor, so it adds **no** new tests (see the Testing section of `AGENTS.md` and the `test-audit` skill's authoring gate). The contracts are already owned at the MCP boundary:
  - `TestSkillUpdatePreviewWithExpectedContentDigest` (`internal/delivery/mcpserver/skill_tools_test.go`): the `skill_get` digest is accepted by `skill_update_preview`, and a stale digest returns `edit_conflict`;
  - `TestSkillToolsLifecycleInMemory`: `skill_get` fields such as `content_digest`, `lifecycle_state`, and routing;
  - `TestSkillUpdatePreviewUnknownID`: the unknown-skill mapping.
- Steps:
  1. Do not create `internal/app/skill_detail_test.go`.
  2. Run the three owner tests.
- Verify: `go test -count=1 -v -run '^(TestSkillUpdatePreviewWithExpectedContentDigest|TestSkillToolsLifecycleInMemory|TestSkillUpdatePreviewUnknownID)$' ./internal/delivery/mcpserver/` exits 0 and prints `--- PASS:` for all three names.

### Task 1.4 — Guard and report
- Steps:
  1. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh check 1`.
  2. Write `reports/phase-01-report.md` (see plan.md rule 8).
- Verify: exit code 0 and the last line is exactly `GUARD RESULT: PASS (phase 1)`.

## Failure Protocol
If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP and report the same
failure evidence to the user. Never continue by self-reasoning.
