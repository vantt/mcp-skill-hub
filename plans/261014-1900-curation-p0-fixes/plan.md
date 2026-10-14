---
title: "Small curation fixes from the UX reviews"
description: "Fix the P0 curation defects the two 2026-10-14 UX reviews found that stay valid under the accepted redesign (D1-D9): secrets copied into the hub, lost executable bits, wrong or misleading CLI messages, and four small web defects."
status: completed
priority: P1
effort: 2.5d
branch: (implementation gets its own worktree)
tags: [cli, web, import, security, ux]
blockedBy: []
blocks: []
created: 2026-10-14
---

# Small curation fixes from the UX reviews

## Outcome

The P0 defects in `plans/reports/ux-review-261014-1651-curation-cli-agent.md` and
`plans/reports/ux-review-261014-1651-curation-web.md` that the redesign in
`plans/261014-1740-curation-ux-redesign/design-decisions.md` does not replace are fixed now:

- No secret or junk file reaches the hub through any path that imports a folder, and the add preview lists every
  file and every file it left out.
- An imported script keeps its executable bit.
- Every printed next step, FIX and commit hint is true and can be run as shown; an error is never printed as
  success and never exits 0.
- The web shows no MCP tool name as a shell command, no operation ID in a toast, no false "Active" or
  "Created in this workspace", and the docs match the Archive button.

Every item below was reproduced on 2026-10-14 against `main` at `4de8625` with a scratch hub (isolated `HOME`,
`XDG_*`, `GIT_CONFIG_GLOBAL`), or read in code where a run needs the network. The probe was deleted.

## Constraints

- Do not plan what D1-D9 replace: no new commands or flags, no change to the two-step `--approve-content` flow,
  no diff-by-default preview (D5 owns the `--diff` flag), no update-from-folder on re-add (D7), no change to where
  `skill create` writes (D8), no web file tree or review screen (D6, D9).
- Public JSON contracts change by addition only. MCP tool names, descriptions and schemas stay byte-identical.
- Tests are written first and must fail before the fix. Tests use temporary hubs only: never
  `/home/vantt/skill-hub`, the real `HOME` or a real project.
- No plan IDs, phase numbers or finding codes in code comments, test names or commit messages.
- Each phase runs in its own worktree from `main`; commits use conventional format.

## Non-goals (verified and left out)

| Report item | Why it is left out |
|---|---|
| Preview always shows a unified diff; document or drop `--full-diff`; "No files changed" wording | D5 decides the preview shape and the `--diff` flag. |
| `skill add <dir>` a second time becomes an update | D7 (`skill commit <folder>`). Only the false "Draft added" message is fixed here. |
| Approve in the same step as the confirm | D7, D9. |
| `status` reports `FAILED` for a stale index after a hand edit | D8 decides it, and it changes a JSON severity value; see open question 4. |
| Resolver stops returning a `review_required` skill as primary | Runtime behaviour; D9 removes the cause. Only the edit preview warning is added here. |
| `--ref <sha>` accepted, GitLab, `file://` | Port work and P2 in the report. The wrong "stale" error is fixed here. |
| Web: add from a folder, file tree, review screen, ID from name, collections from the hub | D6, D7, D9 or P1/P2 in the web report. |
| Executable bit for files that `skill update` adds from upstream | See open question 3. |

## Phases

| Phase | Area | Status | Depends on | Effort |
|---|---|---|---|---|
| [1](phase-01-import-files-and-add-messages.md) | Folder import: excluded files, executable bit, add preview and add messages | pending | none | 1d |
| [2](phase-02-cli-next-steps-and-fix-texts.md) | CLI next steps, commit hints, FIX texts, default collection, edit trust warning, file counts | pending | none (its one docs step waits for phase 1) | 0.75d |
| [3](phase-03-upstream-update-output.md) | `skill update` and `skill upstream` text output | pending | none | 0.25d |
| [4](phase-04-web-fixes.md) | Web: Home command, toast, Resources, provenance, docs, counts, left-out files | pending | steps 6-7 wait for phases 1 and 2 | 0.5d |

Phases 1, 2 and 3 run in parallel. Phase 4 steps 1-5 run in parallel with them; steps 6-7 start after phases 1
and 2 merge.

## File ownership

No file is edited by two phases. `docs/curating-skills.md` is the one shared document: phase 1 owns the
"Add a skill" section, and phase 2 edits the create examples after phase 1 merges.

| Phase | Files it may modify |
|---|---|
| 1 | `internal/source/import_exclude.go` (new), `internal/source/types.go`, `internal/source/filesystem.go`, `internal/source/git_repository.go`, `internal/mutation/mutation.go`, `internal/mutation/transaction.go`, `internal/mutation/filesystem.go`, `internal/app/skill_discovery.go`, `internal/app/skill_add.go`, `internal/app/source_import.go`, `internal/app/upstream_update.go`, `internal/app/metadata_lint.go`, `internal/delivery/cli/skill_add.go`, `internal/delivery/cli/source_import.go`, their `_test.go` files, `docs/curating-skills.md` (add section) |
| 2 | `internal/delivery/cli/skill.go`, `internal/delivery/cli/skill_review.go`, `internal/delivery/cli/skill_create_prompt.go`, `internal/app/skill_review.go`, `internal/app/health_hub.go`, `internal/app/skill_lifecycle.go`, `internal/skill/` (one constant), `internal/delivery/web/routes_skill_write.go`, `internal/delivery/mcpserver/skill_tools.go`, `web/src/screens/skill-create/CreateSkillScreen.tsx` and `create-add.test.tsx`, their tests, `docs/curating-skills.md` (create examples, after phase 1) |
| 3 | `internal/delivery/cli/skill_upstream.go`, `internal/delivery/cli/skill_upstream_test.go`, `docs/sources-and-learning.md` (only if a quoted output changes) |
| 4 | `internal/app/next_commands.go` (new), `internal/delivery/cli/next_commands.go`, `internal/delivery/web/routes_read.go`, `web/src/screens/skill-detail/SkillDetailScreen.tsx`, `ResourcesTab.tsx`, `ProvenanceCard.tsx`, `OverviewTab.tsx`, `ReviewTab.tsx`, `web/src/screens/skill-add/AddSkillScreen.tsx` and a new `add-skill-screen.test.tsx`, `web/src/api/types.ts`, their tests, `web/e2e/ux/guards.spec.ts`, `web/e2e/ux/seed-states.ts`, `docs/web-ui.md` |

## Acceptance criteria

1. `skill add <dir>` on a folder holding `.env`, `scripts/run.sh` (mode 0755) and `SKILL.md` writes `SKILL.md`
   and `scripts/run.sh` with mode 0755, never `.env`; the text preview lists both files with sizes and a
   "Left out" line naming `.env`; `--json` carries `excluded` and `resources[].executable`.
2. The same exclusion applies to `skill add <url>`, `source import` and `skill update`, through one helper.
3. Re-adding an unchanged folder prints that nothing changed and the skill's real state, and no confirm line with
   empty pins.
4. Any `skill add ... --yes` or `skill update ... --yes` whose preview is an error prints that error (exit 2),
   never "proposal is stale".
5. Commit hints after edit, activate, deprecate and archive, and the review "Next" line, are the same command
   `skillhub status` prints, with the real hub path, quoted.
6. Web Home never offers `workspace_diff` (or any tool name) as a command; a guard test with uncommitted Git
   changes proves it.
7. `make check` and `make web-check` pass; the web guard spec passes.

## Validation

Narrowest test first in each phase, then `make check` (Go) and `make web-check` (web), then
`npx playwright test e2e/ux/guards.spec.ts` for phase 4.

## Open questions

**Answered 2026-10-14 by the lead, following the user's accepted design (D7, D8 and the redesign's answer to its
question 15); the user may still overrule:**
1. No keep flag in this plan; the redesign's `commit` preview lists what was left out, and an `--include` can be
   added there later if needed.
2. Confirmed, with one change: keep `.env.example` and `.env.sample` (templates without secrets); exclude `.env` and
   every other `.env.*`.
3. Not now: carrying modes through the three-way merge goes with the upstream update UX work; tracked in the
   backlog.
4. Leave it to the redesign (D8 owns the `status` change and its JSON value).
5. Confirmed: `default` is the one default collection.
6. Confirmed: neutral label now; recording the origin is D7 work.


1. **Keeping an excluded file.** The plan adds no flag to keep `.env` and similar files (D7 lists exclusion, not an
   override). Is "rename the file or add it by hand" acceptable until the redesign, or should D7's `commit` get an
   `--include`?
2. **Patterns.** The plan narrows `id_*` to SSH private key names (`id_rsa`, `id_dsa`, `id_ecdsa`, `id_ed25519`,
   with an optional `_sk` suffix) because `id_*` would also drop files such as `id_mapping.py`. It adds `.git/`,
   which a folder that is a repository root otherwise copies whole. `.env*` also drops `.env.example`. Confirm.
3. **Executable bit on upstream updates.** `skill update` builds files from a path-to-bytes map without modes, so a
   script that upstream adds keeps mode 0644. Fixing it means carrying modes through the three-way merge. Do it now
   or with the update UX work (report P1 #13)?
4. **Stale index after a hand edit.** D8 wants `status` to stop reporting `FAILED`; doing it changes the JSON
   severity value from `failed`. Fold it into this plan as an allowed contract change, or leave it to the redesign?
5. **Default collection.** The plan makes `default` the one default (what `add` and the docs' review output already
   use) and changes `create` (guided form, web, MCP) from `core`. Confirm `default` over `core`.
6. **Provenance of a folder-added skill.** A skill added from a folder records nothing that tells it apart from a
   created one, so the web can only show a neutral label. Recording the origin is D7 work; confirm.
