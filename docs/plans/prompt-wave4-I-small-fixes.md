# Wave 4 · Worktree I prompt: two small fixes

Start the agent inside its worktree:

```bash
cd /home/vantt/projects/mcp-skill-hub-small-fixes && omp --profile=<profile>
```

```text
You are worktree I. Fix two small defects and switch Claude Code to split MCP
profiles in the Go repo mcp-skill-hub, test-first. The lead verified items 1-2 on main 9dbc041.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory)
- Worktree /home/vantt/projects/mcp-skill-hub-small-fixes, branch
  wave4/small-fixes (the lead created it from main 9dbc041).
  `git -C /home/vantt/projects/mcp-skill-hub-small-fixes status -sb` must show it.
- ABSOLUTE paths only, under /home/vantt/projects/mcp-skill-hub-small-fixes/.
- Never write to /home/vantt/projects/mcp-skill-hub (lead's main checkout) or
  /home/vantt/skill-hub (the live hub). Do not push.
- Never edit .claude/skills/distill-lab. Never remove telemetry event types or
  change EventVersion. Tests must not read paths outside the repo or a temp dir.

Read first: AGENTS.md, docs/plans/handoff-2026-10-10-lead.md ("Small open items").

Fix, in this order, one commit per item (conventional commits), `make check`
green before each (if TestGitRevisionAt fails with "broken pipe", rerun it once):

1. `skillhub telemetry --help` does not list `cases`.
   internal/delivery/cli/help.go:236 lists health|preview|export|purge|funnel|
   import-transcripts, but the dispatcher also accepts `cases`
   (status|enable|disable, plus the bare `cases` listing; see
   internal/delivery/cli/telemetry_cases_test.go) and possibly `baseline`.
   - Find every subcommand the telemetry dispatcher really accepts.
   - Make the usage line and the Subcommands block list all of them, with
     their flags, in the same style as the existing lines.
   - Add a test that fails if a dispatcher subcommand is missing from the help
     text (derive the list from the dispatcher or a shared table, not a second
     hand-written list, if that stays simple).

2. Fragile `$ref` in the resolver output schema.
   internal/delivery/mcpserver/server.go:563-575 replaces the `resolution`
   property of the skill_resolve and routing_evaluate output schemas with the
   committed schema schemas/skill-resolve-response-v1.schema.json. That schema
   uses `"$ref": "#/$defs/setup"` (lines 25-26). The ref resolves today only
   because the embedded copy keeps its `$id` and `$defs`. If any later step
   (shrinkOutputSchema, a profile that strips metadata, a future pruning pass)
   removes that `$id` or the `$defs`, mcp.AddTool panics at startup.
   - First write a test that builds the server with each profile (all, runtime,
     curation) and asserts every registered tool's input and output schema
     resolves (e.g. jsonschema Resolve) and that tools/list succeeds.
   - Then write a test that reproduces the failure: strip `$id` from the
     embedded resolution schema (or run shrinkOutputSchema over the final
     output schema) and show it panics or fails to resolve.
   - Fix it the simplest robust way you can justify, for example hoisting the
     committed schema's `$defs` into the output root and rewriting its refs, or
     making the shrink/profile passes explicitly preserve embedded resources.
     Do not change the committed schema files under schemas/ or the wire
     shape a client sees beyond what the fix needs; say in the report if
     tools/list bytes change and by how much.

3. Mark Claude Code as verified for per-server toggle, so connect/integrate
   write two entries (`skillhub` --profile runtime, `skillhub-curation`
   --profile curation) for Claude Code. The dual-entry code already exists
   (internal/hostintegration/integration.go:228, files.go:247, preview.go);
   only the matrix gate blocks it.

   Evidence (the user ran it on 2026-10-10, Claude Code 2.1.296; record it):
   in an isolated workspace with a project .mcp.json holding both entries,
   both servers connected at start; `/mcp` -> Disable skillhub-curation left
   only the 3 mcp__skillhub__* tools (skill_feedback, skill_get,
   skill_resolve); Enable restored 23 mcp__skillhub-curation__* tools; after
   /exit and reopening, skillhub-curation stayed disabled until re-enabled
   (Claude Code MCP logs show it was not started). In `/mcp` an enabled
   server shows a tick, a disabled one an empty circle. Claude Code defers
   MCP tool schemas (agents see names until ToolSearch), so on this host the
   saving is mostly the tool-name list, not ~50k tokens.

   - Update the Claude Code entry in BOTH docs/mcp-compatibility-matrix.json
     and internal/hostintegration/matrix.json (they must stay identical):
     `server_toggle.status: "verified"`, a short `evidence` string with the
     facts above, client_version 2.1.296 for this check, and drop the old
     "issue #78314" reason. Update the scope_notes line that says no client
     is verified. Do not change other clients.
   - Update TestHostSupportsServerToggle (matrix_test.go:57): Claude Code is
     now true from the embedded matrix, Codex and Gemini stay false.
   - Newly written skillhub-curation entries start enabled in Claude Code.
     Make the connect/integrate text output (preview and apply) say once,
     for Claude Code only when it writes the curation entry: open `/mcp`,
     select skillhub-curation, Disable it for daily work; Enable it when you
     want to curate. Keep JSON output machine-readable (add a field, do not
     put prose in existing fields).
   - Align internal/systemskills/curator/SKILL.md:60-62 with that wording
     ("open `/mcp`, select skillhub-curation, Enable"), keep it host-neutral
     enough for other hosts. If the native curator copy has a version/digest
     that doctor checks, bump it the way earlier curator edits did.
   - Check what `skillhub doctor` says about an existing project that still
     has one full `skillhub` entry for Claude Code. It must not call that
     broken; at most suggest re-running connect. Add a test for whatever it
     does.
   - Smoke in a temp HOME and temp project: `skillhub connect --workspace
     <temp workspace> --yes` writes exactly two Claude Code entries with the
     right --profile args, the reminder is printed, a second connect is a
     no-op, and Codex/Gemini projections still write one full entry.

Out of scope: the live hub, test-audit examples, distill-lab, web/.

When done, report: commits (hash + subject), files changed, tests added,
`make check` result, the temp-HOME smoke output for item 3, the tools/list byte size for each profile before and after,
and anything you noticed but did not fix.
```
