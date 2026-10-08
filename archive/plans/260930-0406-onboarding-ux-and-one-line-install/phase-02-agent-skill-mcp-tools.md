---
phase: 2
title: "Agent-side skill tools"
status: complete
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Agent-side skill tools

## Goal
A connected agent can create, activate/deprecate/archive, list and show skills through MCP with the same preview→confirm safety as the CLI, and the curator skill routes those intents.

## Context
- UX audit P0-1, P1 "`hub_status` for the agent" (U16).
- Design: `docs/design/05-curation-lifecycle.md` §17 tool mapping (only `skill_update_preview/confirm` today); user decision 2 extends it.
- Existing app services: `internal/app/skill_lifecycle.go`, `internal/app/skill_list.go`; MCP tools: `internal/delivery/mcpserver/curation_tools.go`, `types.go`, `server.go`; curator: `system-skills/curator/SKILL.md` (intent table ~57-73) mirrored at `internal/systemskills/curator/SKILL.md` (byte-identical test).

## Files to Create / Modify
- Create: `internal/delivery/mcpserver/skill_tools.go` (+ tests)
- Modify: `internal/delivery/mcpserver/server.go` (register), `types.go`
- Modify: `internal/app/curation_home.go` (hub_status next actions use MCP tool names / CLI commands, not `GetCurationDiff`-style internal names)
- Modify: `system-skills/curator/SKILL.md` and `internal/systemskills/curator/SKILL.md` (identical), bump `CuratorSkillVersion` in `internal/systemskills/embed.go`
- Modify: `docs/design/05-curation-lifecycle.md` §17 table (add rows)

## Tasks & Steps
- [x] Tools: `skill_create_preview` / `skill_create_confirm`, `skill_transition_preview` / `skill_transition_confirm` (target state active|deprecated|archived), `skill_list` (optional state filter), `skill_get` (any state, incl. routing fields and path). Same application commands as CLI; confirm requires proposal id + digest + base version from preview.
- [x] `skill_update_preview` on an unknown id: "Skill <id> does not exist; use skill_create_preview".
- [x] Activation preview returns ALL missing requirements (trigger, not_for or rationale, min_scope) in one structured list (shared with Phase 4 CLI fix — implement in the app layer here).
- [x] Curator skill: add intent rows (create, activate, deprecate, archive, list, show) with exact tool names and the approval rule (preview shown to user before confirm).
- [x] hub_status next actions reference MCP tool names (and CLI equivalent), never Go method names.
- [x] Update MCP compatibility notes only if tool list is asserted anywhere (`docs/mcp-compatibility-matrix.json`).

## Verification
- `go test ./internal/delivery/mcpserver ./internal/app ./internal/systemskills` incl. stdio subprocess test: create preview → confirm → transition to active → list shows it → get shows routing.
- CLI and MCP produce equivalent results for the same request (existing parity test pattern).
- Curator copies byte-identical test passes.

## Risks
- Scope creep into a full CRUD API: keep to create/transition/list/get; edit stays `skill_update_*`.
