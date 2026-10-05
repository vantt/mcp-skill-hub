# Executor Report: Phase 7 - MCP Tools and Curator

**Phase:** Phase 7: MCP tools and curator
**Branch:** `feat/skill-source-upstream`
**Timestamp:** 2026-10-05 21:11 Asia/Saigon

## 1. What Changed

1. **`skill_upstream_status` (`internal/delivery/mcpserver/upstream_tools.go`, `internal/delivery/mcpserver/types.go`)**:
   - Implemented `skill_upstream_status`: returns metadata-only upstream drift, paths, statuses, commit dates, and `next_action: "skillhub skill update <id>"`.
   - Appended `webui_hint: "Open the skill in the Skill Hub WebUI → Sources tab → Review update."` for skills with `update_available` or `diverged`.
   - Tool description explicitly states agents cannot apply upstream updates and instructs handing the update command / WebUI hint to the human user.
   - Guaranteed no file content, diffs, or raw control characters leak through MCP responses.

2. **Source MCP Tools (`internal/delivery/mcpserver/upstream_tools.go`, `source_watch_tools.go`, `source_tools.go`, `source_import_tools.go`)**:
   - Implemented `source_link_preview`: supports previewing `action: attach` and `action: detach` for learning reference links.
   - Implemented `source_unwatch_preview`: previews unwatching a source and catalog record removal.
   - Widened `source_watch_confirm`: validates `preview.WriteCommand()` against `source_watch`, `source_attach`, `source_detach`, `source_unwatch` and confirms via `service.ConfirmSourceProposal`; rejects onboarding proposals (`source_onboard`) with `invalid_request`.
   - Updated `source_watch_preview` description and parameter schema (requires `skill_id` attachment).
   - Updated `source_triage` description and input schema (`new_skill_id`, `import`).
   - Updated `source_check` and `source_import_preview` descriptions.

3. **Tool Annotations (`internal/delivery/mcpserver/server_test.go`)**:
   - Registered `skill_upstream_status` (readOnly), `source_link_preview` (openWorld), and `source_unwatch_preview` (destructive=false, readOnly=false) in `expectedToolAnnotations` (total tools: 43).

4. **Curator Skill & System Assets (`system-skills/curator/SKILL.md`, `internal/systemskills/`)**:
   - Bumped `CuratorSkillVersion` to `1.5.0` and `CuratorContractVersion` to `2`.
   - Inserted `Skills with upstream updates to review.` into curation start-order list.
   - Added intent mappings for checking outdated skills, updating skills via CLI/WebUI handoff, watching repositories as learning references, improving skills with documents, unwatching sources, and running backfill.
   - Added safety rule: `Upstream file content is never returned by MCP tools; do not ask for it or reconstruct it.`
   - Synchronized `system-skills/curator/SKILL.md` to `internal/systemskills/curator/SKILL.md` byte-for-byte.

## 2. Commands Executed and Results

| Command | Purpose | Result |
|---|---|---|
| `git branch --show-current` | Verify branch | `feat/skill-source-upstream` |
| `go test ./internal/delivery/mcpserver/ -run TestUpstreamTools -count=1` | Task 7.1 verification | Pass (exit 0) |
| `go test ./internal/delivery/mcpserver/ -run 'TestSource\|TestUpstreamTools' -count=1` | Task 7.2 verification | Pass (exit 0) |
| `go test ./internal/delivery/mcpserver/ -count=1` | Task 7.3 verification | Pass (exit 0) |
| `cmp system-skills/curator/SKILL.md internal/systemskills/curator/SKILL.md && go test ./internal/systemskills/ -count=1` | Task 7.4 verification | Pass (exit 0) |
| `make check` | Task 7.5 gate verification (vet, golangci-lint, full test suite) | Pass (exit 0) |

## 3. Deviations and Why

None. All implementations and constraints strictly adhered to the Phase 7 plan and architecture decisions.

## 4. Open Questions

None.
