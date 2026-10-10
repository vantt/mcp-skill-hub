# Wave 3 · Worktree H: one source for system-curator (observer Phase 5 (c))

Start the agent inside its worktree:

```bash
cd /home/vantt/projects/mcp-skill-hub-curator && omp --profile=openai-tetcu72
```

```text
You are worktree H, working in the Go repo mcp-skill-hub.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory)
- Worktree /home/vantt/projects/mcp-skill-hub-curator, branch
  wave3/curator-single-source, created by the lead from main at c54fe12.
  `git -C /home/vantt/projects/mcp-skill-hub-curator status -sb` must show it.
- Use ABSOLUTE paths only, under /home/vantt/projects/mcp-skill-hub-curator/.
- Never write to /home/vantt/projects/mcp-skill-hub (the lead's checkout),
  /home/vantt/projects/mcp-skill-hub-simplify (worktree C, running now), or
  /home/vantt/skill-hub (the live hub). Never run `skillhub integrate` against
  real host config. Every integrate run uses a temp HOME and a temp project.

Worktree C is running simplify Phase 5 in parallel. C owns internal/mutation,
internal/migration, internal/workspace, internal/app/operations.go,
internal/app/curation_home.go and web/src/screens/home. Do not edit them, and
do not bump the workspace schema version. You own internal/hostintegration/**,
internal/systemskills/**, system-skills/curator, the curator branches in
internal/app/distribution.go (lines ~137-260, `systemskills.CuratorSkillID`),
skills/list in internal/delivery/mcpserver, and docs/mcp-compatibility-matrix.json.

The problem (lesson `native-and-mcp-same-name`,
docs/distillery/lessons/host-integration.yaml:43-71; observer plan
docs/plans/2026-10-08-observer-and-enrichment.md §5, row "system-curator"):
`integrate` installs system-curator as a native skill
(internal/hostintegration/integration.go:33-35, for Claude, Codex and
Gemini), and the MCP server also serves system-curator over the skills
extension (skills/list, skills/get). A host that reads both sees the same name
from two origins, possibly at different versions. The MCP skills spec says a
host must not let one silently shadow the other.

DECISION (c), already made: each client gets exactly one source. A client
that supports the MCP skills extension uses MCP; every other client uses the
native copy. Per-client support comes from docs/mcp-compatibility-matrix.json.
Note what the matrix says today: no stock client is verified for skills/list
or skills/get (scope_notes), and Claude Code is verified only for MCP
registration and curator load.

Read first: the lesson above, observer plan §5 and §5.1,
docs/plans/observer-handoff-G.md, the matrix, and AGENTS.md.

Do the following in order, one commit per item, test-first, make check green
before each commit:

1. Matrix field. Add a per-client `skills_extension` entry to the matrix
   (status verified | unverified | blocked_unverified, plus reason/evidence),
   in the same style as G's `server_toggle`. Keep the embedded copy in sync
   (TestMatrixSynchronized). Mark a client verified only with evidence you
   ran or can cite; otherwise unverified.
2. Native install follows the matrix. For a host whose client is verified for
   skills_extension, integrate does NOT install the native curator. inspect,
   plan and remove also clean up a native copy that integrate wrote earlier,
   but never a user-edited copy. If the native file differs from what
   integrate wrote, report it and leave it. All other hosts keep the native
   install as today. With today's matrix the result must be: no behavior
   change for any host. Say so in the report.
3. Server side. When a session's client is one that integrate gave a native
   curator (normalized client name; see
   internal/delivery/mcpserver/client_normalization.go), skills/list and
   skills/get must not offer system-curator to it. Unknown or other clients
   keep getting it over MCP. If the client cannot be identified, keep
   today's behavior and explain why.
4. Version skew. `skillhub doctor` (hostintegration side) reports a native
   curator whose content differs from the embedded version, with the fix
   command. Check first whether this already exists; if it does, add only
   the missing test.
5. Update the lesson's decision in host-integration.yaml from candidate to
   ported, with a reason and the date. Update the observer plan §5 row with
   the commit hashes.

Tests: integrate in a temp HOME for each host, before and after flipping the
matrix entry in the test; skills/list per client name; doctor version skew;
and the existing curator_boundary_test must stay green.

Rules: AGENTS.md; never remove telemetry event types or change EventVersion;
additive public contracts only. Use conventional commits ending with
"Co-Authored-By: <your real model name> <noreply@...>". make check must be
green before EVERY commit. No push, no merge. If main moves, rebase on main
and run make check again.

Final report: commits (hash + subject); for each item, what changed and the
test that proves it; the matrix diff; a table of host × (native installed?,
served over MCP?) for today's matrix; follow-ups for C; make check output.
End with "Status: DONE | DONE_WITH_CONCERNS | BLOCKED" and one sentence.
```
