# Wave 3 · Worktree G: MCP profile split (§5.1) and Phase 5 spec fixes

Start the agent inside its worktree:

```bash
cd /home/vantt/projects/mcp-skill-hub-profile && omp --profile=gemini-tetcu72
```

```text
You are worktree G, working in the Go repo mcp-skill-hub.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory)
- Worktree /home/vantt/projects/mcp-skill-hub-profile, branch
  wave3/observer-profile-split, created by the lead from main at d6af65f.
  `git -C /home/vantt/projects/mcp-skill-hub-profile status -sb` must show it.
- Use ABSOLUTE paths only, under /home/vantt/projects/mcp-skill-hub-profile/.
- Never write to /home/vantt/projects/mcp-skill-hub (the lead's checkout),
  /home/vantt/projects/mcp-skill-hub-simplify (worktree C, running now),
  /home/vantt/projects/mcp-skill-hub-observer, or /home/vantt/skill-hub (the
  live hub).

Worktree C is running a small cleanup in parallel. C owns these, and you must
not edit them: server.go getSkill/readResource URI validation (around
:219-223 and :264-268), internal/app/source.go, internal/delivery/cli/source.go,
internal/app/{curation_home,skill_sources}.go, internal/catalog/input.go, and
the web types and golden tests for home/skill-sources. You own
registerTools/profile wiring in server.go, internal/hostintegration/**,
docs/mcp-compatibility-matrix.json, system-skills/system-curator, and
CLAUDE.md. If you need something from C's files, write it down as a follow-up
instead of editing.

Read first: docs/plans/2026-10-08-observer-and-enrichment.md §5 and §5.1,
docs/plans/observer-handoff-wave3.md, docs/plans/observer-handoff-F.md,
docs/plans/phase-03 contract table (plans/261008-1433-simplify-hub-model/
phase-03-minimal-distill.md), and AGENTS.md.

Measured by the lead on main d6af65f
(`go test -v ./internal/delivery/mcpserver -run TestToolsListSize`):
26 tools, 129,733 bytes (~32k tokens), outputSchema 76.2%. The runtime set
was 18,681 bytes. The split is still worth about 7x, so do it.

Do the following in order, one commit per item, test-first, make check green
before each commit:

1. Profile split (§5.1, decisions 1-5).
   - `skillhub mcp serve --profile runtime|curation|all`. Without the flag the
     behavior is unchanged (all tools).
   - runtime = skill_resolve, skill_get, skill_feedback, plus the skills
     extension skills/list and resources/read. It does NOT include skill_list.
   - curation = every other tool.
   - Server-boundary test: the curator skill's compatible-tools is a subset of
     the curation profile's tools, and runtime ∪ curation = all, with no
     overlap except the skills extension methods.
   - `skillhub integrate` writes two entries (`skillhub` with --profile
     runtime, `skillhub-curation` with --profile curation) ONLY for hosts that
     docs/mcp-compatibility-matrix.json marks as able to enable and disable
     each server. All other hosts keep one full entry. inspect, plan and remove
     handle both entries (hostintegration/integration.go:280,288,
     preview.go:71). Only mark a host as verified in the matrix if you can cite
     the source; otherwise leave it unverified.
   - The curator SKILL.md gets a first step: if the curation tools are
     missing, tell the user to enable `skillhub-curation` or use the matching
     CLI command.
   - Extend TestToolsListSize to report every profile, and report the numbers.
2. Phase 5 (a), snapshot_expired recovery. Use the snapshot_expired error in
   distributionRPCError, NOT the URI validation that C owns. Add `current_uri`
   to the error data when the skill still exists, with a clear message ("the
   skill was updated; call skills/get <current_uri>"). Add one line to
   CLAUDE.md. Do not add a `current` alias. Write a test with a skill edited
   after its URI was issued.
3. skill_list pagination (§5 table): default limit about 50, using the paging
   package the way skills/list does. Change the description to "for curation;
   to pick a skill for a task, call skill_resolve". Keep it in the curation
   profile only. Write a test.
4. Shrink outputSchema and the heavy tool descriptions (§5.1 decision 6) for
   both profiles. Do not change the shape of any tool result, and do not
   remove fields. Report the bytes before and after for each profile.

Do NOT start observer Phase 3 (usage lessons). It is blocked:
distill-lab rejects `where: usage:<case_id>`. Write one paragraph on the
options in your report.

Rules: AGENTS.md; never remove telemetry event types or change EventVersion;
additive public contracts only. Use conventional commits ending with
"Co-Authored-By: <your real model name> <noreply@...>". make check must be
green before EVERY commit. No push, no merge. If main moves, rebase on main
and run make check again.

Final report: commits (hash + subject); for each item, what changed and the
test that proves it; a tools/list table per profile, before and after item 4;
which hosts you marked verified, with sources; follow-ups for C; make check
output. End with "Status: DONE | DONE_WITH_CONCERNS | BLOCKED" and one
sentence.
```
