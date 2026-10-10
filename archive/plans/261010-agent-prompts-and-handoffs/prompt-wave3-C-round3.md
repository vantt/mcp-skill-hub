# Wave 3 · Worktree C round 3: lead review of 965b074..0d422e8

```text
Lead review of 965b074..0d422e8. The Phase 4 fixes are fine: third_party for
local skills, the approval walk over both metadata paths, no updated_at or
history writes, untracked upstream, and read errors. make check and
make web-check are green, and the branch merges cleanly with main and with
worktree F. It is still not mergeable. Fix the items below in your worktree
only, test-first, one commit per item, make check and make web-check green
before each commit, no push, no merge.

FIRST: main moved to bd7cda1 (worktree F's observer fixes, no conflict with
your branch). Rebase wave2/simplify-p4-p3 on main before you start, and run
make check.

1. CRITICAL, migrate v4 destroys the live hub's distill.yaml. I migrated a
   fresh clone of /home/vantt/skill-hub to 4. The hub has no legacy sources/ or
   distill/ data, so the only thing v4 did was rewrite
   skills/default/test-audit/.meta/distill.yaml, which distill-lab wrote
   (commits 12c7925, bb19027, 0f88983). The rewrite cut 1184 lines to 470. It
   dropped goal.status/in_scope/out_of_scope/failures_it_prevents, all of
   coverage.*.not_read, and each lesson's layer, score, final_score and
   also_fits. It also renamed decision.state to decision.status, turned cursors
   into a list, and reordered the lessons. distill-lab can no longer read the
   file.

   DECISION (from the user): the hub's .meta/distill.yaml format IS the
   distill-lab format. The authority is the in-repo skill
   .claude/skills/distill-lab/scripts/distill.py: validate() (lines 62-165),
   TOP_ORDER/GOAL_ORDER/LESSON_ORDER/SCORE_ORDER/DECISION_ORDER, and
   COMMIT_WHERE_RE/PATH_WHERE_RE (a 7-40 hex SHA; commit-level refs allowed).
   Read it; do NOT edit anything under .claude/skills/distill-lab.
   - Migration v3->v4 never rewrites an existing .meta/distill.yaml. It must
     stay byte-identical. Legacy runs, insights and comparisons, if present,
     are converted into the distill-lab format only when the skill has no
     distill.yaml yet. If one exists, merge only by appending lessons with
     fields distill-lab accepts, or leave it and report the legacy data in the
     migration output. Pick one and say which.
   - distill.schema.json and the Go model accept exactly what distill.py
     accepts: goal object, cursors map, per-source coverage read/not_read,
     layer, score{relevance,facts,impact,evidence,effort,why}, final_score,
     also_fits, status, found_by, decision{state,reason,at}, and short or full
     SHAs. Do not reject distill-lab output, and do not write fields that
     distill.py rejects as unknown. That means no seen_where and no
     `usage:<case_id>` where entries for now. Drop the seen_where reopen rule
     and record it in phase-03-minimal-distill.md as deferred until distill-lab
     supports it. Update the phase-03 data-model section to the real format.
   - Any hub write (web POST /skills/{id}/distill) must round-trip a distill-lab
     file without losing fields or comments, or be removed. Prefer read-only
     plus "edit with distill-lab" unless a write path is really needed.
   - Tests: put a copy of the live test-audit distill.yaml in testdata.
     (a) v3->v4 migration leaves it byte-identical.
     (b) The Go validator accepts it.
     (c) The web GET returns every lesson with layer, score and decision.state.
     In your report, paste the output of
     `python3 /home/vantt/projects/mcp-skill-hub-simplify/.claude/skills/distill-lab/scripts/distill.py check <file> --no-where`
     on the testdata file, if check takes that flag. Otherwise use the closest
     offline form and say which one you used.

2. HIGH, the web UI still calls removed APIs. web/src/routes.tsx:48-56 (/inbox,
   /inbox/:id, /inbox/:id/apply), screens/inbox, screens/insight,
   screens/run/RunScreen.tsx with useDistillRun/cancelDistillRun
   (queries.ts:321-340, calling /runs/*), queries.ts:345-390 (/inbox, /insights/*),
   AppShell.tsx:59-70, home/action-cta.ts:62-65 (review_insights -> /inbox),
   skill-detail/LearningSection.tsx:158-163 (pending_insights link to /inbox).
   Remove them along with their tests and i18n keys. A grep of web/src for
   "/inbox", "/insights", "/runs/", "pending_insights" and "review_insights"
   must return nothing.

3. HIGH, the backend still has leftovers of insights. internal/catalog/schema.go:92
   still creates the insights table, and project.go:190 still projects it.
   internal/app/skill_sources.go:27-35,83-161 (countPendingInsights,
   PendingInsights) and internal/app/curation_home.go:57,189 (pending_insights)
   still use it. Remove all of it, or replace it with the number of candidate
   lessons from distill.yaml if the UI needs a count. Then change
   catalog.SchemaVersion as the catalog rules require.

4. MEDIUM, your field table does not match the files. After migration,
   .meta/skill.yaml contains `schema_version: 1` (your table says "2"), and
   still has `quality.reviewed` (your table says it was dropped).
   markdown-to-epub keeps `name:` (your table says it is derived). Decide what
   is intended. Then make the code, docs/design and the phase-04 doc agree, and
   add a test that asserts the fields of the migrated file.

5. MEDIUM, server.go follow-ups from worktree F (observer review). F is not
   allowed to edit server.go.
   - unsupported_method_calls (server.go:157-160) is always 0, because go-sdk
     rejects unknown methods before middleware runs. Count it at the
     transport/JSON-RPC level, or remove the counter but keep the metric name
     valid.
   - unlisted_resource_reads (server.go:298-313) is already blocked by
     ReadResource, and only differs after a catalog change. Compare against the
     session's pinned manifest, or remove it.
   - snapshot_expired_requests (around server.go:450) also counts
     skill.ErrNotFound. Split them: not-found returns skill_not_found and does
     not count as snapshot_expired.
   - server.go:221 (getSkill) and :266 (readResource) return code
     snapshot_expired for an empty or >4096-byte URI. Return invalid_params
     with a code like invalid_uri instead, and do not count it as
     snapshot_expired. Add a test.
   Full context: docs/plans/followup-worktree-c.md and
   docs/plans/observer-handoff-F.md (on main after you rebase).
   Never remove telemetry event types or change EventVersion.

Then rebuild, migrate a fresh clone of /home/vantt/skill-hub to 4 in a scratch
dir (XDG_* pointing at scratch; never touch /home/vantt/skill-hub), and report
`git status` plus `git diff --stat` of the clone after migration. The
distill.yaml must not appear in the diff.

Final report: hashes, what changed and the test that proves it for each item,
the clone diff --stat, the distill.py check output, the web grep output,
make check, and make web-check. End with "Status: DONE | DONE_WITH_CONCERNS |
BLOCKED" and one sentence.
```
