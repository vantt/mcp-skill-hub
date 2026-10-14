---
phase: 4
title: "Web: Home command, toast, Resources, provenance, docs, counts, left-out files"
status: pending
effort: 0.5d
depends_on: []   # steps 6 and 7 need phases 1 and 2 merged
---

# Phase 4: web fixes

## Context

The web report's three P0 items and the small web defects the user listed. The web file tree, review screen and
web commit (D6, D9) are out of scope. Steps 1-5 are independent of the other phases; step 6 needs phase 2's
`file_count`, step 7 needs phase 1's `excluded`.

## Items, evidence and fixes

### 4.1 Home "Copy command" copies the MCP tool name `workspace_diff` (web P0 #1; user item 5)

**Evidence.** `internal/app/curation_home.go:522` (`review_git_changes`) and `:545` (`first_run_commit`) set
`Command: "workspace_diff"`; other actions carry `source_check`, `curation_run_start`, `doctor`, `skill_list`.
These are MCP tool names, which the curation MCP profile returns to agents by design. The CLI maps them to shell
commands (`internal/delivery/cli/next_commands.go:110-165`); the web route passes them through unchanged
(`internal/delivery/web/routes_read.go:38-45`), and `web/src/screens/home/action-cta.ts:60-68` puts
`action.command` into the copy block. The guard spec misses it because its INTERNAL_ID check skips `code`/`pre`
(`web/e2e/ux/guards.spec.ts:35-48`) and its fixture hub has no uncommitted Git changes.

**Fix.**
1. Move the tool-to-command switch out of `cliNextCommand` into a new `internal/app/next_commands.go`:
   `ShellCommandFor(command, id string) string` (the plain command, no `--json`, no `--workspace`). The CLI's
   `cliNextCommand` calls it and then adds `--workspace`/`--json` as today, so CLI output is byte-identical.
2. `handleHome` copies the home result and replaces each `Actions[i].Command` and `SuggestedActions[i].Command`
   with `app.ShellCommandFor(...)`. `/api/v1/home` serves only the bundled web app; the MCP and CLI outputs are
   unchanged. `workspace_diff` becomes `skillhub maintain diff`, which prints the real `git add ... && git commit`
   line.

**Tests first.**
- `internal/delivery/web/routes_read_test.go`: a temp hub with an uncommitted file; `GET /api/v1/home` → every
  non-empty `command` starts with `skillhub ` or `git `, and none equals `workspace_diff`.
- `internal/delivery/cli/next_commands_test.go`: existing tests stay green (byte-identical guard).
- `web/e2e/ux/guards.spec.ts`: new test "Home offers a shell command when the hub has uncommitted changes": the
  fixture writes an uncommitted change through the CLI (`hub.runCLI` with a skill edit `--yes`, no commit) before
  opening `/`; the Home command block text matches `/^(skillhub|git) /` and does not match
  `/^[a-z]+(?:_[a-z]+)+$/`. Add the seeding helper to `web/e2e/ux/seed-states.ts` if the existing helpers do not
  leave the hub dirty.

### 4.2 Toast after a lifecycle change shows "Transitioned · OP-…" (web P0 #3; user item 6)

**Evidence.** `web/src/screens/skill-detail/SkillDetailScreen.tsx:180`:
``setToastMessage(`Transitioned · ${res.operation_id}`)``.

**Fix.** Use the target already held in `transitionTarget` (`:106`, set at `:158`): "<id> is now active." /
"... is now deprecated." / "... is now archived.", with no operation ID.

**Test first.** `skill-detail-tabs.test.tsx` (or a new `skill-detail-transition.test.tsx`): confirming an
activation shows "is now active" and no `OP-`.

### 4.3 Resources table says "Active" for every file, even on a draft (web UC3 pain 4; user item 6)

**Evidence.** `web/src/screens/skill-detail/ResourcesTab.tsx:12,20,191-195`: a hard-coded `State` column with the
chip "Active" for every row.

**Fix.** Remove the State column (header, cell and the mobile `data-label`). The skill's state is shown in the
header already. The Digest column and the banner are P2 in the report and stay.

**Test first.** `resources-tab.test.tsx`: a draft skill's Resources tab has no "Active" text and no "State" header.

### 4.4 Provenance says "Created in this workspace" for a folder-added skill (web UC4 pain 6; user item 6)

**Evidence.** `web/src/screens/skill-detail/ProvenanceCard.tsx:6` shows that label whenever no provenance is
recorded. A skill added from a folder records none: its `.meta/skill.yaml` (checked on 2026-10-14) has no
`provenance` or source, the same as a created skill (`internal/app/skill_add.go:572-635` builds `src` only for
non-local origins). The web cannot tell the two apart.

**Fix.** Change the label to "Made on this machine (created here or added from a folder); no tracked source."
Recording a folder origin is D7 work (plan open question 6).

**Test first.** `sources-tab.test.tsx:30-32` asserts the old text; change it to the new label first (fails), then
change the component.

### 4.5 `docs/web-ui.md` says archive is terminal-only (web UC4 pain 7; user item 6)

**Evidence.** `docs/web-ui.md:87`: "To archive a deprecated skill, run `skillhub skill archive <skill-id> --yes` in
the terminal." The web has an Archive button for deprecated skills
(`SkillDetailScreen.tsx:308-316`, confirmation dialog at `:486-497`).

**Fix.** Step 1 at `docs/web-ui.md:79` names **Archive** for a deprecated skill; line 87 says archive works from the
web (with a confirmation) or with `skillhub skill archive <skill-id> --yes`. No test (docs).

### 4.6 File counts include hub metadata (web UC2 pain 4; user item 10) — after phase 2

**Evidence.** `OverviewTab.tsx:80,106` and `ReviewTab.tsx:54-56` show `resource_status.resource_count`, which
includes `.meta/skill.yaml` ("Files: 2 files" for a skill with only `SKILL.md`).

**Fix.** Read `file_count` and `file_bytes` (phase 2) with fallback to the old fields; add both to the
`resource_status` type in `web/src/api/types.ts:146`.

**Test first.** `overview-tab.test.tsx`: a review payload with `resource_count: 2, file_count: 1` renders
"1 file".

### 4.7 Add from GitHub does not show what was left out — after phase 1

**Evidence.** `web/src/screens/skill-add/AddSkillScreen.tsx:39,488` lists "Files that would be written" from the
proposal; phase 1 adds `excluded` to the proposal.

**Fix.** Under the file list, a "Left out (not copied into the hub)" list of `excluded[].path` with reasons, shown
only when non-empty; add `excluded?: {path: string; reason: string}[]` to `SkillAddProposal` in `types.ts:200`.

**Test first.** New `web/src/screens/skill-add/add-skill-screen.test.tsx` (the existing
`skill-create/create-add.test.tsx` belongs to phase 2): a proposal with one excluded `.env` renders ".env" under
"Left out".

## Steps

1. Write the failing Go and vitest tests for 4.1-4.4; record failures.
2. `app.ShellCommandFor`, CLI refactor, web route mapping; then the guard spec.
3. Toast, Resources column, provenance label.
4. `docs/web-ui.md` archive text.
5. `make web-check`.
6. After phase 2 merges: 4.6.
7. After phase 1 merges: 4.7.

## Validation

```bash
go test ./internal/delivery/web/ -run 'Home' -count=1
go test ./internal/delivery/cli/ -run 'Next' -count=1
cd web && npx vitest run src/screens/skill-detail src/screens/home src/screens/skill-add
make check && make web-check
make web-build && go build -o web/.e2e/skillhub ./cmd/skillhub && (cd web && npx playwright test e2e/ux/guards.spec.ts)
```

The E2E guard starts its own hub and server from `web/e2e/support/server`; it must not use the live hub or port.
Stop any server the run leaves behind.

## Risks

- `ShellCommandFor` must keep the CLI's exact output; the existing `next_commands_test.go` is the guard.
- Web screenshots in `docs/images/web/` that show the Resources State column or the old toast go stale; refresh only
  if `make web-shots` is part of the release checklist, otherwise note it.

## Rollback

Revert the phase's commits; no stored data or public contract changed (`/api/v1/home` serves only the bundled web
app).
