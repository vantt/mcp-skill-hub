# Wave 3 · Worktree C round 5: Phase 5 review fixes

Send to the running C agent (worktree /home/vantt/projects/mcp-skill-hub-simplify).

```text
Review of df37f57 + db11620 by the lead. make check is green after a trial
merge with main, and the v5 migration shrinks history/operations on a live-hub
clone from 272,761 to 13,328 bytes with distill.yaml byte-identical. But the
lead reproduced a regression, so do not stop yet. Fix test-first, one commit
per item, make check (and make web-check for item 5) green before each.

0. Rebase on main first (main is at 6ce87d0, 6 commits ahead of your base).
   main already rewrote internal/app/migration_v3_fixture_test.go to read
   internal/app/testdata/live-hub-v1 (c54fe12). Keep main's version and apply
   only your v5 changes on top; drop your `git show 0f88983` fallback and the
   live-hub path, they are dead code now.

1. HIGH, GetOperationDiff loses every diff after the v5 migration
   (internal/app/operations.go, gitCommitForFile). It uses the LAST commit
   that touched the receipt file. The v5 migration rewrites every receipt, so
   after the migration is committed, every operation resolves to the
   migration commit, where the skill files did not change. Reproduced on a
   live-hub clone, migrated to v5 and committed:
   - OP-02e11071… (had stored bodies before v5): now diff_available=false,
     digest_only=true. The diff was lost.
   - OP-c011d730…: diff_available=true but Before == After for every file.
     The output is wrong.
   Fix: find the commit by content, not by receipt history. Walk
   `git log --format=%H -- <changed path>` (bounded), and pick the commit
   where sha256(path at commit) == stored after digest (empty when deleted)
   and sha256(path at commit^) == stored before digest (empty when created;
   handle a root commit with no parent). If no commit matches, return
   digest_only=true. Never return a diff whose digests do not match the
   receipt.
   Tests: (a) commit an operation, run planV4ToV5 + commit, and the diff is
   still correct; (b) the same path is edited again in a later commit, and the
   diff still shows the operation's change, not the later one; (c) the
   operation and an unrelated edit to the same path land in one commit, so
   the digests do not match and the result is digest_only.

2. HIGH, the uncommitted branch shows a diff that may not be this
   operation. It checks HEAD == before but not that the working file ==
   after. Require both; otherwise digest_only. Test: the file is edited again
   after the operation and before any commit, so the result is digest_only.

3. HIGH, internal/migration/v5_test.go reads
   /home/vantt/skill-hub/history/operations/…/OP-c011d730….yaml. That is the
   same mistake main just fixed in c54fe12. Once the live hub is migrated to
   v5, the file is under 100 KB and the test FAILS (t.Fatalf on size). On
   any other machine it silently skips. Build the large legacy receipt in
   the test instead: generate before_content/after_content bodies over 100 KB.
   No test may read a path outside the repo or a temp dir.

4. MEDIUM, the v5 rewrite reorders every receipt. Decoding into
   map[string]any and re-encoding sorts the keys alphabetically and changes
   the indent from 4 to 2 (seen on OP-02e11071…: schema_version moved from
   line 1 to the end, and proposal.digest now comes before proposal.id).
   Edit a yaml.Node instead: remove only before_content, after_content and
   content_available from each change, and keep key order and indent as
   mutation writes them. Test: the stripped receipt equals the original with
   those three keys removed (compare the decoded structs and the key order).

5. MEDIUM, the review_lessons command does not run in a hub. The action
   emits `python3 .claude/skills/distill-lab/scripts/distill.py list <path>`.
   distill-lab lives only in the mcp-skill-hub repo: /home/vantt/skill-hub
   has .claude/skills/system-curator and nothing else, so the relative path
   fails there. (Your report also said `format --check`, but the code emits
   `list`.) Do not emit a path that is not there. Set Command to
   `distill.py list <hub-relative distill.yaml path>`, and say in Summary
   that it is the distill-lab skill's script. Keep the hub-relative path and
   the ID; the web CTA routing is fine. Update the test.

Do not change the receipt fields you kept, the v4 migration, or distill-lab.
No push, no merge.

Report: commits (hash + subject); for each item, the test that proves it;
history/operations size and GetOperationDiff output for OP-02e11071… and
OP-c011d730… on a fresh clone of /home/vantt/skill-hub migrated to v5 and
committed, with isolated XDG_* dirs, and the clone only, never the live hub;
make check and make web-check output.
End with "Status: DONE | DONE_WITH_CONCERNS | BLOCKED" and one sentence.
```
