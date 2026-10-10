# Wave 3 · Worktree H round 2: drop the curator receipt files

Send to the running H agent (worktree /home/vantt/projects/mcp-skill-hub-curator).

```text
Review of c09742a..e04538c by the lead. Items 1, 3, 4 and 5 are accepted. Item 2
breaks the "no behavior change today" requirement, so fix it before merge.
Test-first, make check green before each commit. No push, no merge.

0. Rebase on main (now 92318da; C's Phase 5 merged, schema v5). No overlap with
   your files is expected; run make check after the rebase.

1. HIGH, connect now writes a new file per host into every project. Lead
   smoke test: init + connect in a temp HOME and project creates
     .claude/skills/system-curator/SKILL.md.skillhub-sha256
     .agents/skills/system-curator/SKILL.md.skillhub-sha256
     .gemini/skills/system-curator/SKILL.md.skillhub-sha256
   next to each SKILL.md. These files show up in users' repos, and they are
   written today even though no client is verified, so the path they serve
   cannot run. Remove the receipt files: curatorReceiptSuffix,
   writeCuratorReceipt, and the receipt branch in nativeCuratorOwned.
   Ownership on cutover is: the file's bytes equal the current bundled
   curator. Anything else is a conflict and is left in place, as you already
   do for legacy copies. Say in the conflict text that `skillhub doctor --fix`
   restores the bundled copy, after which connect can remove it.
   Tests: (a) connect in a temp project writes exactly the same file set as
   main does (assert the full list under .claude, .agents and .gemini); (b) the
   matrix-flip cutover still removes an unchanged copy and keeps an edited
   one; (c) no file named *.skillhub-sha256 is ever created. Delete any
   receipt-specific tests.

2. LOW (your own concern): doctor's text output reports native curator drift
   but not the fix command. Print the same fix command the JSON has. Test it.

Keep the matrix, the server filter, and the lesson as they are. Update the
observer plan row hashes if they change after the rebase.

Report: commits (hash + subject); for each item, the test that proves it; the
file list from a real connect smoke in a temp HOME and project; make check
output. End with "Status: DONE | DONE_WITH_CONCERNS | BLOCKED" and one
sentence.
```
