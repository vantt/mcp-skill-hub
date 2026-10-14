---
phase: 3
title: "`skill update` and `skill upstream` text output"
status: pending
effort: 0.25d
depends_on: []
---

# Phase 3: upstream update output

## Context

The update model stays as it is (preview, then confirm; nothing applies itself). D5 later decides how a preview
shows its diff and which flag shows all of it. This phase fixes only output that is wrong today, in one file:
`internal/delivery/cli/skill_upstream.go`. No app or JSON change.

## Items, evidence and fixes

### 3.1 A no-op or error update prints "from - to -" and exits 0 (report CLI update pain 1, P0 #3; user item 7)

**Evidence.** `runSkillUpdate` renders the text preview with `renderUpstreamUpdatePreview`
(`skill_upstream.go:374-381`) without checking `app.ErrorOf(preview)`, and returns 0. The app returns an error
result for "up to date" ("Upstream is unchanged.", FIX "No update needed."), "not tracked", "pinned", "removed
upstream" and "not checked" (`internal/app/upstream_update.go:418-470`). `--json` already exits 2
(`writeResult`, `internal/delivery/cli/output.go:17-46`).
The `--yes` path (`skill_upstream.go:354-366`) passes the error preview to `ConfirmUpdate`, which reports a pin
mismatch instead of the cause (same defect as `skill add --yes`, fixed for add in phase 1).

**Fix.** Text mode goes through `writeResult(stdout, stderr, false, preview, render)`, so an error preview prints
ERROR/WHY/FIX on stderr and exits 2. In the `--yes` path, return the preview's error before the unresolved check
and before `ConfirmUpdate`.

**Tests first** (`skill_upstream_test.go`, using the existing fake upstream fixtures there):
- up-to-date skill, `skill update <id>` → exit 2, stderr has "Upstream is unchanged", stdout has no `from -` and
  no `--proposal ''`;
- skill without upstream, `skill update <id> --yes` → exit 2, stderr names "not tracked", not "stale".

### 3.2 `skill upstream` suggests the no-op update (report CLI update pain 2)

**Evidence.** `skill_upstream.go:288-289` prints "Next: skillhub skill update <id>" for every status, including
"up to date (local copy is clean)".

**Fix.** The Next line depends on `upstream.Status`: `update_available`, `diverged` → `skillhub skill update <id>`;
`unknown`, `unavailable` → `skillhub source check <source-id>`; `upstream_removed` → "keep your copy, or archive
it: `skillhub skill deprecate <id>`"; `up_to_date`, `modified`, `pinned` → "Nothing to update." with no command.
These are the cases and fixes the app already uses in `upstream_update.go:448-470`; keep the words aligned.

**Tests first.** Up-to-date skill → output contains "Nothing to update" and no `skill update`; update available →
contains `skillhub skill update <id>`.

### 3.3 A resolved conflict still shows "needs decision" (report CLI update pain 4, P0 #11; user item 9)

**Evidence.** `formatActionWord` (`skill_upstream.go:556-581`) tests `f.Conflicts > 0` before `f.Action`, so a file
resolved with `--manual` or `--accept` keeps "needs decision (1 conflict)" while the apply succeeds.

**Fix.** Check an explicit resolution first: when `f.Action` is `manual`, `upstream` or `local`, print "use your
file (resolved)", "take upstream (resolved)" or "keep local (resolved)"; only an unresolved file with conflicts
prints "needs decision". Confirm with the app test fixtures that a resolved file has `Action` set and is absent from
`preview.Unresolved`; if the app does not set `Action`, use membership in `preview.Unresolved` as the test.

**Tests first.** Conflicting fixture with `--manual SKILL.md=<file>` → the row for `SKILL.md` contains "resolved" and
not "needs decision".

### 3.4 The full diff scrolls the summary and the next step away (report CLI update pain 3; user item 9)

**Evidence.** `renderUpstreamUpdatePreview` prints the title, the file table, then every `ResultDiff`
(`skill_upstream.go:434-448`, 2,864 lines in the report), then the trust warning and the confirm command. The
table, the decision the user must make, ends up thousands of lines above the prompt.

**Fix (no new flag; D5 owns `--diff`).** Print the diffs first, under a "Changes:" heading, then the title line,
the file table with the unchanged count, the trust warning and the Next command, so the summary and the next step
are the last thing on screen. Add the added/removed line counts per file to the table (`+12 -3`), computed from
`ResultDiff`, which D5 also asks for. The conflict branch is unchanged (it prints no diff).

**Tests first.** A clean multi-file update: in stdout, the index of the table header `FILE` and of the confirm
command are both after the last diff hunk (`@@`); each changed row carries `+N -M`.

## Steps

1. Write the four failing tests; record failures.
2. Route text mode through `writeResult`; add the `--yes` guard.
3. Status-dependent Next line in `runSkillUpstream`.
4. Resolution-first `formatActionWord`.
5. Reorder `renderUpstreamUpdatePreview` and add line counts.
6. Docs: `docs/sources-and-learning.md` (lines 55-85) quotes the update commands, not their output; change it only
   if a quoted line no longer matches.

## Validation

```bash
go test ./internal/delivery/cli/ -run 'Upstream|Update|Outdated' -count=1
make check
```

## Risks

- Scripts that parsed the text preview of `skill update` see a new order. The JSON output, meant for scripts, is
  unchanged.
- An error that used to exit 0 now exits 2. That is the fix; `--json` already behaved so.

## Rollback

Revert the phase's commit; only CLI rendering changed.
