---
title: "Phase 7: MCP tools and curator"
status: in-progress
---

# Phase 7: MCP tools and curator

<!-- Updated: Validation Session 1 - agents may not apply upstream updates; only skill_upstream_status reports them and returns the CLI/WebUI handoff -->

## Context

- Plan: [plan.md](./plan.md) (D10, D11). Depends on phases 3–5.
- Read first: `internal/delivery/mcpserver/server.go:305-330` (`registerTools`, `addTool`) and `:515` (`annotations`), `internal/delivery/mcpserver/source_tools.go`, `source_watch_tools.go`, `source_import_tools.go`, `skill_add_tools.go`, `types.go`, `server_test.go:48-73` (`expectedToolAnnotations`), `internal/systemskills/embed.go:10-64`, `internal/systemskills/embed_test.go:100-180`, `system-skills/curator/SKILL.md` (identical copy at `internal/systemskills/curator/SKILL.md`), `plans/261004-1547-skill-execution-measurement-routing/phase-05b-runtime-hardening.md` (R2: unapproved third-party content never reaches agents).

## Overview

Let agents **report** upstream status and manage source links through tools that carry metadata only. Agents cannot preview or apply an upstream update: taking an update is a review decision made where the diff is visible (CLI or WebUI), and applying it flips the skill to `review_required` mid-work (Validation Session 1, decision 5). The curator learns the new intents, hands updates off to `skillhub skill update <id>` or the WebUI, and keeps content approval human-only.

## Requirements

1. New tools (registered in a new `registerUpstreamTools`, called from `registerTools`):

   | Tool | Input | Output | Annotations (readOnly, destructive, idempotent, openWorld) |
   |---|---|---|---|
   | `skill_upstream_status` | `skill_id?` | list (`UpstreamListResult`) or one (`SkillUpstreamResult`); no network | true, false, false, false |
   | `source_link_preview` | `action: attach\|detach`, `skill_id`, `source_id?`, `locator?`, `ref?`, `path?`, `cadence?`, `idempotency_key?` | `SourceProposal` | false, false, false, true |
   | `source_unwatch_preview` | `source_id`, `idempotency_key?` | `SourceProposal` | false, false, false, false |

   `skill_upstream_status` returns paths, statuses, counts, commits, and the upstream commit date only; no file content, diff, or commit message. Every entry with status `update_available` or `diverged` carries `next_action: "skillhub skill update <id>"` (phase 3 read model) and a `webui_hint` added by the tool: "Open the skill in the Skill Hub WebUI → Sources tab → Review update."`; the tool description says: "Agents cannot apply upstream updates. Give the user the update command or the WebUI hint; the user reviews the diff and applies it. After applying, a third-party skill needs the user's content approval (`skillhub skill review <id>`)." There is no `skill_upstream_update_preview` or `skill_upstream_update_confirm` tool, and no MCP tool returns pins for an `upstream_update` proposal; the generic confirm tools refuse that kind (phase 4 kind guard).
2. Changed tools:
   - `source_watch_confirm`: instead of a new confirm tool, accept any stored source proposal whose `SourceProposal.WriteCommand()` (phase 5) is `source_watch`, `source_attach`, `source_detach`, or `source_unwatch` (replacing the `ApplicationCommand` check at `source_watch_tools.go:57`, which stored proposals cannot satisfy because `loadSourceProposal` hard-codes `TriageSourceCandidate`, `internal/app/source.go:944`); description updated.
   - `source_watch_preview`: new required `skill_id` input; description says the source becomes a learning reference of that skill.
   - `source_triage`: new `new_skill_id` input and decision `import` (`selection?`, `all?` inputs); `sourceTriageResult` gains `SkillAdd *app.SkillAddProposal \`json:"skill_add,omitempty"\``; description lists the outcomes (accept with `skill_id` or `new_skill_id`, import, defer, reject).
   - `source_check`: description mentions per-skill upstream results in `results[].skills`.
   - `source_import_preview`: description says it reads the ref's current commit and marks already-imported skills.
3. Curator (`system-skills/curator/SKILL.md`, copied byte-for-byte to `internal/systemskills/curator/SKILL.md`):
   - Start-order list: insert `Skills with upstream updates to review.` after "Changed sources ready to distill."
   - Intent rows (replace the "Watch a repository for updates" row; add the others):
     - Check whether my skills are outdated → call `skill_upstream_status`; call `source_check` first only when the user asks to check now (network).
     - Update a skill from its repository → call `skill_upstream_status` for that skill, summarize what changed (file counts, local edits, upstream commit date), and tell the user to run `skillhub skill update <id>` or open the WebUI Sources tab to review the diff and apply it. Never try to apply the update yourself, never write the skill's files to imitate it, and never approve content; after the user applies it, remind them that `skillhub skill review <id>` is required before agents can use the skill again.
     - Watch a repository → ask which skill it should improve and call `source_link_preview` (attach), or offer `skill_add_preview` to vendor its skills.
     - Use a repository or document to improve a skill → `source_link_preview` with `action: attach`; documents go through `source_intake_add` then `source_triage` with `skill_id`.
     - Stop watching or unlink a source → `source_unwatch_preview` or `source_link_preview` with `action: detach`; confirm with `source_watch_confirm`.
     - Track skills added before upstream tracking → tell the user to run `skillhub source backfill` (CLI only).
   - One sentence under the safety intro: `Upstream file content is never returned by MCP tools; do not ask for it or reconstruct it.`
   - Bump the minor version (frontmatter `version:` and `CuratorSkillVersion`) from its current value.
4. `curatorCompatibleTools` adds the three new tools (`skill_upstream_status`, `source_link_preview`, `source_unwatch_preview`) in a stable position (after `source_check`); `embed_test.go` `wantTools` updated; the three names are also added to the fenced list under `## Compatible tools (contract version 1)` in `SKILL.md` (`system-skills/curator/SKILL.md:237-276`; `embed_test.go:116` requires each tool to start a line). The tool set changed, so bump `CuratorContractVersion` to `"2"` (`internal/systemskills/embed.go:16`), the heading to `contract version 2`, and the `SKILL.md` frontmatter `contract-version`, and update the tests that pin it (`internal/delivery/mcpserver/subprocess_test.go:83`, `internal/delivery/mcpserver/server_test.go:234`, `internal/app/distribution_test.go:35`). `TestCuratorGuidanceRequiresPreviewBeforeConfirm` pairs gain `source_link_preview`/`source_watch_confirm`. Add one durable-contract phrase to `TestCuratorGuidanceDurableBehaviorContract`: `"no upstream apply": "Never try to apply the update yourself"`.

## Related code files

Create: `internal/delivery/mcpserver/upstream_tools.go`, `internal/delivery/mcpserver/upstream_tools_test.go`.

Modify: `internal/delivery/mcpserver/server.go` (one line in `registerTools`), `internal/delivery/mcpserver/types.go`, `internal/delivery/mcpserver/source_tools.go`, `internal/delivery/mcpserver/source_watch_tools.go`, `internal/delivery/mcpserver/source_import_tools.go` (description only), `internal/delivery/mcpserver/server_test.go` (`expectedToolAnnotations`, contract version at line 234), `internal/delivery/mcpserver/subprocess_test.go` (contract version at line 83), `internal/app/distribution_test.go` (contract version at line 35), `internal/delivery/mcpserver/source_watch_tools_test.go`, `internal/systemskills/embed.go`, `internal/systemskills/embed_test.go`, `system-skills/curator/SKILL.md`, `internal/systemskills/curator/SKILL.md`.

Do not modify any other file.

## Implementation steps

### Task 7.1 — Upstream status tool
- Steps: implement `skill_upstream_status`. Test (`TestUpstreamTools`) on a workspace seeded like phase 6 Task 6.1 (state recorded directly; the upstream file contains a sentinel string): the list returns `update_available` with `next_action == "skillhub skill update <id>"` and the WebUI hint; the structured content contains neither the sentinel string nor keys named `upstream_diff`, `local_diff`, `result_diff`, `merged_with_markers`, or `confirmation`; no path or summary contains a raw control character; `tools/list` contains no tool whose name starts with `skill_upstream_update`; calling `skill_transition_confirm` with the ID of an `upstream_update` proposal created through `UpstreamService.PreviewUpdate` returns an error and leaves the files unchanged.
- Verify: `go test ./internal/delivery/mcpserver/ -run TestUpstreamTools -count=1` exits 0 and prints `ok`.

### Task 7.2 — Source tools
- Steps: implement `source_link_preview`, `source_unwatch_preview`, the widened `source_watch_confirm`, and Requirement 2. Tests: link attach → confirm via `source_watch_confirm`; unwatch preview → confirm via `source_watch_confirm`; `source_watch_confirm` with an onboarding (`source_onboard`) proposal ID returns `invalid_request`; `source_watch_preview` without `skill_id` returns an error (keep the local-folder cases in `source_watch_tools_test.go`).
- Verify: `go test ./internal/delivery/mcpserver/ -run 'TestSource|TestUpstreamTools' -count=1` exits 0 and prints `ok`.

### Task 7.3 — Annotations table
- Steps: add the three new tools to `expectedToolAnnotations` with the values above.
- Verify: `go test ./internal/delivery/mcpserver/ -count=1` exits 0 and prints `ok`.

### Task 7.4 — Curator
- Steps: edit `system-skills/curator/SKILL.md` per Requirement 3, then `cp system-skills/curator/SKILL.md internal/systemskills/curator/SKILL.md`; update `embed.go` and `embed_test.go` per Requirement 4.
- Verify: `cmp system-skills/curator/SKILL.md internal/systemskills/curator/SKILL.md && go test ./internal/systemskills/ -count=1` — `cmp` prints nothing and the test prints `ok`.

### Task 7.5 — Gate
- Verify: `make check` exits 0.

## Todo

- [x] Task 7.1 upstream tools
- [x] Task 7.2 source tools
- [x] Task 7.3 annotations
- [x] Task 7.4 curator
- [ ] Task 7.5 `make check`

## Success criteria

- An agent can list outdated skills and hand the user the exact update command, attach/detach learning references, and unwatch sources (preview/confirm); it has no way to apply an upstream update.
- No MCP response carries upstream file content.

## UX acceptance

Curator reply when asked to update `docx`: `docx has an upstream update (2 files changed upstream, newest commit 2026-10-03; you also edited it locally). I can't apply updates. Review and apply it with: skillhub skill update docx — or open docx in the WebUI, Sources tab, Review update. After applying, approve the new content with skillhub skill review docx before agents use it again.`

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Agent applies an update the user did not review | L×H | No MCP tool previews or applies updates; generic confirm tools refuse `upstream_update` proposals (phase 4 guard, tested in Task 7.1). |
| Content leak through summaries | L×H | `skill_upstream_status` returns only metadata; sentinel test in Task 7.1. |
| Curator copies drift | L×M | `cmp` in Task 7.4 and the existing embed test. |

## Security considerations

Source confirm tools require all three pins; no tool confirms upstream updates. Content approval remains CLI-only. Upstream commit messages are not exposed through MCP.

## Rollback

Revert the phase commit; CLI and WebUI paths are unaffected.

## Failure Protocol

If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, weaken or delete a test, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP, write the same evidence to `reports/executor-<YYMMDD-HHMM>-blocker-<phase-slug>.md`, and report the blocker to the user. Never continue by self-reasoning.
