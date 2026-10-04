---
title: "Phase 7: MCP tools and curator"
status: todo
---

# Phase 7: MCP tools and curator

## Context

- Plan: [plan.md](./plan.md) (D10, D11). Depends on phases 3–5.
- Read first: `internal/delivery/mcpserver/server.go:305-330` (`registerTools`, `addTool`) and `:515` (`annotations`), `internal/delivery/mcpserver/source_tools.go`, `source_watch_tools.go`, `source_import_tools.go`, `skill_add_tools.go`, `types.go`, `server_test.go:48-73` (`expectedToolAnnotations`), `internal/systemskills/embed.go:10-64`, `internal/systemskills/embed_test.go:100-180`, `system-skills/curator/SKILL.md` (identical copy at `internal/systemskills/curator/SKILL.md`), `plans/261004-1547-skill-execution-measurement-routing/phase-05b-runtime-hardening.md` (R2: unapproved third-party content never reaches agents).

## Overview

Let agents report and propose upstream updates and manage source links through preview/confirm tools that carry metadata only. The curator learns the new intents and the rule that content approval is human-only.

## Requirements

1. New tools (registered in a new `registerUpstreamTools`, called from `registerTools`):

   | Tool | Input | Output | Annotations (readOnly, destructive, idempotent, openWorld) |
   |---|---|---|---|
   | `skill_upstream_status` | `skill_id?` | list (`UpstreamListResult`) or one (`SkillUpstreamResult`); no network | true, false, false, false |
   | `skill_upstream_update_preview` | `skill_id`, `target_commit?`, `idempotency_key?` (no resolutions) | `PreviewUpdate(...).MetadataOnly()` | false, false, false, false |
   | `skill_upstream_update_confirm` | `proposal_id`, `proposal_digest`, `base_version` (all required) | `UpstreamUpdateResult` | false, true, true, false |
   | `source_link_preview` | `action: attach\|detach`, `skill_id`, `source_id?`, `locator?`, `ref?`, `path?`, `cadence?`, `idempotency_key?` | `SourceProposal` | false, false, false, true |
   | `source_unwatch_preview` | `source_id`, `idempotency_key?` | `SourceProposal` | false, false, false, false |

   `skill_upstream_update_preview` accepts no resolutions: agents never see upstream content, so they cannot choose between versions (D10). It returns pins only when every file has a default action and at least one skill file changes; when anything is unresolved or blocked, or when the only effect would be re-pinning the base (all files kept local), it returns no pins and a summary `Review this update in \`skillhub skill update <id>\` or the WebUI.` This keeps an agent from dropping local edits or silently suppressing an upstream fix. Descriptions state: "Returns file paths, statuses, and counts only; upstream file content and diffs are never returned." and, for confirm, "After confirming, a third-party skill needs the user's content approval (`skillhub skill review <id>`); agents cannot approve content."
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
     - Update a skill from its repository → call `skill_upstream_update_preview`; show file statuses, unresolved files, and the trust impact; require explicit approval before `skill_upstream_update_confirm`; for unresolved files hand off to `skillhub skill update <id>` or the WebUI; after confirm tell the user to run `skillhub skill review <id>` and never approve content yourself.
     - Watch a repository → ask which skill it should improve and call `source_link_preview` (attach), or offer `skill_add_preview` to vendor its skills.
     - Use a repository or document to improve a skill → `source_link_preview` with `action: attach`; documents go through `source_intake_add` then `source_triage` with `skill_id`.
     - Stop watching or unlink a source → `source_unwatch_preview` or `source_link_preview` with `action: detach`; confirm with `source_watch_confirm`.
     - Track skills added before upstream tracking → tell the user to run `skillhub source backfill` (CLI only).
   - One sentence under the safety intro: `Upstream file content is never returned by MCP tools; do not ask for it or reconstruct it.`
   - Bump the minor version (frontmatter `version:` and `CuratorSkillVersion`) from its current value.
4. `curatorCompatibleTools` adds the five new tools in a stable position (after `source_check`); `embed_test.go` `wantTools` updated; the five names are also added to the fenced list under `## Compatible tools (contract version 1)` in `SKILL.md` (`system-skills/curator/SKILL.md:237-276`; `embed_test.go:116` requires each tool to start a line). The tool set changed, so bump `CuratorContractVersion` to `"2"` (`internal/systemskills/embed.go:16`), the heading to `contract version 2`, and the `SKILL.md` frontmatter `contract-version`, and update the tests that pin it (`internal/delivery/mcpserver/subprocess_test.go:83`, `internal/delivery/mcpserver/server_test.go:234`, `internal/app/distribution_test.go:35`). `TestCuratorGuidanceRequiresPreviewBeforeConfirm` pairs gain `skill_upstream_update_preview`/`skill_upstream_update_confirm` and `source_link_preview`/`source_watch_confirm`.

## Related code files

Create: `internal/delivery/mcpserver/upstream_tools.go`, `internal/delivery/mcpserver/upstream_tools_test.go`.

Modify: `internal/delivery/mcpserver/server.go` (one line in `registerTools`), `internal/delivery/mcpserver/types.go`, `internal/delivery/mcpserver/source_tools.go`, `internal/delivery/mcpserver/source_watch_tools.go`, `internal/delivery/mcpserver/source_import_tools.go` (description only), `internal/delivery/mcpserver/server_test.go` (`expectedToolAnnotations`, contract version at line 234), `internal/delivery/mcpserver/subprocess_test.go` (contract version at line 83), `internal/app/distribution_test.go` (contract version at line 35), `internal/delivery/mcpserver/source_watch_tools_test.go`, `internal/systemskills/embed.go`, `internal/systemskills/embed_test.go`, `system-skills/curator/SKILL.md`, `internal/systemskills/curator/SKILL.md`.

Do not modify any other file.

## Implementation steps

### Task 7.1 — Upstream tools
- Steps: implement the three upstream tools. Test (`TestUpstreamTools`) on a workspace seeded like phase 6 Task 6.1 (state recorded directly) plus a real file:// repository for the preview: `skill_upstream_status` returns `update_available`; `skill_upstream_update_preview` structured content contains no key named `upstream_diff`, `local_diff`, `result_diff`, or `merged_with_markers` and no line of the upstream file's text (assert with `strings.Contains` against a sentinel string placed in the upstream file); a preview with a conflicting file returns no pins and the summary names `skillhub skill update`; an unknown `resolutions` argument is rejected by the input schema; confirm with the returned pins of a clean update succeeds and the result names `skillhub skill review`. Also assert no path or summary in any upstream tool response contains a raw control character.
- Verify: `go test ./internal/delivery/mcpserver/ -run TestUpstreamTools -count=1` exits 0 and prints `ok`.

### Task 7.2 — Source tools
- Steps: implement `source_link_preview`, `source_unwatch_preview`, the widened `source_watch_confirm`, and Requirement 2. Tests: link attach → confirm via `source_watch_confirm`; unwatch preview → confirm via `source_watch_confirm`; `source_watch_confirm` with an onboarding (`source_onboard`) proposal ID returns `invalid_request`; `source_watch_preview` without `skill_id` returns an error (keep the local-folder cases in `source_watch_tools_test.go`).
- Verify: `go test ./internal/delivery/mcpserver/ -run 'TestSource|TestUpstreamTools' -count=1` exits 0 and prints `ok`.

### Task 7.3 — Annotations table
- Steps: add the five new tools to `expectedToolAnnotations` with the values above.
- Verify: `go test ./internal/delivery/mcpserver/ -count=1` exits 0 and prints `ok`.

### Task 7.4 — Curator
- Steps: edit `system-skills/curator/SKILL.md` per Requirement 3, then `cp system-skills/curator/SKILL.md internal/systemskills/curator/SKILL.md`; update `embed.go` and `embed_test.go` per Requirement 4.
- Verify: `cmp system-skills/curator/SKILL.md internal/systemskills/curator/SKILL.md && go test ./internal/systemskills/ -count=1` — `cmp` prints nothing and the test prints `ok`.

### Task 7.5 — Gate
- Verify: `make check` exits 0.

## Todo

- [ ] Task 7.1 upstream tools
- [ ] Task 7.2 source tools
- [ ] Task 7.3 annotations
- [ ] Task 7.4 curator
- [ ] Task 7.5 `make check`

## Success criteria

- An agent can list outdated skills, preview and (with approval) confirm a clean update, attach/detach learning references, and unwatch sources, all through preview/confirm.
- No MCP response carries upstream file content.

## UX acceptance

Agent-facing summary for a conflicting preview: `1 file(s) need a decision before this update can be applied. Resolve the remaining files with \`skillhub skill update docx\` or in the WebUI.` Curator closing line after confirm: `docx is updated. Before agents can use it again, review and approve the new content: skillhub skill review docx`.

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Agent confirms an update the user did not see in detail | M×M | The update cannot be used by agents until the human runs `--approve-content`; curator requires explicit approval; Q5 offers a human-only alternative. |
| Content leak through warnings or summaries | L×H | `MetadataOnly()` strips text fields; sentinel test in Task 7.1. |
| Curator copies drift | L×M | `cmp` in Task 7.4 and the existing embed test. |

## Security considerations

Confirm tools require all three pins. Content approval remains CLI-only. Upstream commit messages are not exposed through MCP.

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
