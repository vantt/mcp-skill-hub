# Wave 3 · Worktree C round 4: small cleanup after merge

```text
Lead: round 3 is verified and fast-forwarded to main at d6af65f. On a fresh
clone, the live hub migrates to v4 with distill.yaml byte-identical, and
distill.py check prints OK. Thanks. Four small leftovers remain. Create a new
branch from main:
`git -C /home/vantt/projects/mcp-skill-hub-simplify switch -c wave3/simplify-cleanup main`
Same rules: test-first, make check and make web-check green before each
commit, no push, no merge.

Worktree G (/home/vantt/projects/mcp-skill-hub-profile) runs in parallel and
owns server.go registerTools and distributionRPCError, internal/hostintegration,
system-skills/, CLAUDE.md and docs/mcp-compatibility-matrix.json. In server.go,
edit only the getSkill/readResource URI checks. If item 2's grep hits G's
files, list those hits as follow-ups instead of editing them.

1. You missed the last bullet of item 5. server.go getSkill (around :221) and
   readResource (around :266) still return code snapshot_expired for an empty
   or >4096-byte URI. Return invalid_params with code invalid_uri, and add a
   test.
2. internal/app/source.go:729 and internal/delivery/cli/source.go:760 tell the
   user to run `skillhub distill prepare` and mention "accepted insights". That
   command no longer exists. Rewrite the hint to the new flow (distill-lab
   writes .meta/distill.yaml; porting goes through skill_update). Grep the
   whole repo outside docs/plans and the migration package for "distill
   prepare", "insight" and "inbox" in user-facing strings, and fix every hit.
3. Dead fields: PendingInsights / PendingHighValueInsights
   (curation_home.go:57-58,294,426-428; skill_sources.go:26,34) are now always
   0, and the "pending_insights" category still shows as available. Remove
   them, or replace them with the count of decision.state=candidate lessons
   from .meta/distill.yaml. Update the web types and the golden tests to
   match.
4. internal/catalog/input.go:360-372 still classifies /runs/, /insights/,
   /proposals/, /incorporations/, /findings/ and distill/comparisons/. Remove
   the kinds that no longer exist, unless migration still needs them; if so,
   say where.

Report hashes, what changed for each item with the test that proves it, the
grep output, make check and make web-check. End with "Status: DONE |
DONE_WITH_CONCERNS | BLOCKED" and one sentence.
```
