# Report: Home count copy fix and home.png refresh

Status: DONE_WITH_CONCERNS

## Changes

- `web/src/screens/home/action-cta.ts`: new `actionTitle(action, t)`. It returns the `actionHelp` title first, then
  a plural-aware sentence for `distill_changed_sources`, `check_due_sources`, `retry_unavailable_sources`,
  `review_upstream_updates` and `review_lessons` when `count` is a finite positive number. In every other case it
  returns the server summary.
- `web/src/screens/home/HomeScreen.tsx`: the next-action heading now calls `actionTitle(topAction, t)`. The help
  "why" line is unchanged.
- `web/src/i18n/en.ts`: ten keys `home.action.<kind>.one|other` with the exact strings from the phase.
- `web/src/screens/home/home.test.tsx`: three new tests. They cover singular and plural for all five kinds with
  no "(s)" in the rendered text, the summary fallback (no count, a count of 0, an unknown kind), and help-title
  priority.
- `docs/images/web/home.png`: refreshed. The next action now reads "1 source is ready to distill". The file is
  46,962 bytes. The `home.png` manifest entry is identical to the one in the scratch capture (1280x562, same
  caption and alt), so `manifest.json` was not modified.

## Verification

- `make web-test`: typecheck, lint and vitest pass (32 files, 178 tests).
- The new tests fail on the old code. I checked this by reading the code: the old heading rendered the raw summary,
  for example "1 source(s) are temporarily unavailable", so the singular-sentence assertions cannot match.
- `make web-shots SHOTS_OUT_DIR=/tmp/skillhub-docs-rewrite/shots`: 2 passed, 13 shots. Only `home.png` was copied.
  `git diff --stat docs/images/web/` shows `home.png` only.
- The skill resolver returned no skill. I followed the existing `action-cta.ts` and i18n patterns and did not load
  `/ak:frontend-development`.

## Concern

- I edited one existing test, which goes against "existing tests stay green unchanged". In
  `renders synthetic body with each action kind`, a `retry_unavailable_sources` action with `count: 1` asserted the
  placeholder summary "Retry sources needed". The fix replaces that summary by design, so the assertion now expects
  "1 source is temporarily unavailable". The rest of that test, and every other existing test, is unchanged.
