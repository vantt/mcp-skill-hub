# Wave 5 · Worktree K prompt: connect for CLI curation (step 4)

Start the agent inside its worktree:

```bash
cd /home/vantt/projects/mcp-skill-hub-connect-cli && omp --profile=<profile>
```

```text
You are worktree K. Make `skillhub connect` set Claude Code up for CLI curation:
one runtime MCP server plus permission rules, no skillhub-curation server. Go
repo mcp-skill-hub, test-first.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory)
- Worktree /home/vantt/projects/mcp-skill-hub-connect-cli, branch
  wave5/connect-cli (the lead created it from current main).
  `git -C /home/vantt/projects/mcp-skill-hub-connect-cli status -sb` must show it.
- ABSOLUTE paths only, under /home/vantt/projects/mcp-skill-hub-connect-cli/.
- Never write to /home/vantt/projects/mcp-skill-hub (lead's main checkout),
  /home/vantt/skill-hub (the live hub), or the real HOME (~/.claude.json,
  ~/.claude/settings.json). Smoke only in a temp HOME. Do not push.
- Never edit .claude/skills/distill-lab. Never remove telemetry event types or
  change EventVersion. Tests must not read paths outside the repo or a temp dir.
- Run `make check` with a temp HOME and XDG_* dirs outside the repo (keep
  GOCACHE/GOMODCACHE/GOPATH). Rerun TestGitRevisionAt once on "broken pipe".

Read first: docs/plans/2026-10-10-curator-via-cli.md (approved; you do step 4 of
§7, using the rules verified in §5.2), AGENTS.md, docs/mcp-compatibility-matrix.json
(Claude Code entry), internal/hostintegration/{integration.go,runtime_access.go,
files.go,preview.go,matrix.go}, internal/delivery/cli/connect.go.

Facts already verified by the user on Claude Code 2.1.296 (do not re-test, cite
the design note):
- In project `.claude/settings.local.json`: allow `Bash(skillhub:*)`; ask
  `Bash(skillhub * --yes*)` and `Bash(skillhub * confirm *)`; deny
  `Bash(skillhub * --approve-content*)`. Ask beats allow; flag order does not
  bypass; the deny blocks.
- On first open the trust dialog says the folder "pre-approves 1 tool
  permission ... Bash(skillhub:*)".
- The curator (1.6.0, now on main) uses the CLI when a shell works, and a real
  `claude -p` read-only curation with only the runtime MCP entry used the CLI
  and changed nothing.

Do, in order, one commit per item (conventional commits), `make check` green
before each:

1. Claude Code MCP registration: write only `skillhub` with `--profile runtime`
   (project and `-g`). Remove a `skillhub-curation` entry that connect manages
   (same key it wrote in wave 4); preview must show the removal. Codex and
   Gemini: unchanged (one full entry) in this wave; say so in docs.
   Keep the matrix `server_toggle` evidence; the split is now decided by
   "curation via CLI" for Claude Code, not by the toggle. Make that explicit in
   code (a clear named decision, not a reuse of HostSupportsServerToggle that
   would surprise a reader) and in the matrix scope_notes (both copies identical).

2. Claude Code permission rules. Where connect already manages
   `permissions.additionalDirectories` (project `.claude/settings.local.json`,
   user `~/.claude/settings.json` for -g), also manage exactly the four rules
   above in `permissions.allow/ask/deny`. Add missing rules, never remove or
   reorder the user's own rules, do not duplicate on re-run, and remove only
   rules connect added when disconnecting/removing the integration (follow
   how additionalDirectories removal works today). Preview shows the rule
   changes. Tests: fresh file, file with user rules, re-run no-op, removal.

3. Connect output. Replace the wave-4 `/mcp` Disable reminder for Claude Code
   with: curation runs through the CLI; Claude Code will ask before any
   `skillhub` command with `--yes` or `confirm` and blocks
   `--approve-content`; the first open of the folder shows a trust dialog
   listing `Bash(skillhub:*)`. Keep it short. JSON: keep the optional
   `curation_guidance` field (new text), no prose in other fields; omit on
   no-op as today.

4. Doctor and migration from wave 4. A project with the wave-4 pair
   (`skillhub` runtime + `skillhub-curation`) or the older single full entry
   must not be reported as broken; doctor suggests re-running connect, and
   `doctor --fix` / connect converts it. Tests for both shapes.

5. Fix TestDoctorHostIntegrationPreviewIsReadOnlyAndDependencyOrdered
   (internal/app): it reads the real HOME global connection and fails with
   `global_connection_outdated` when the user's `connect -g` is stale. Isolate
   it (temp HOME / injected user root) so it passes regardless of the real HOME.
   Show it failing first with a stale fake global connection.

6. Small CLI wording left from wave 5 J (one commit): next-step summaries still
   suggest a bare `--yes` (e.g. skill add: "then activate with `skillhub skill
   activate demo --yes`"), which the curator forbids. Point them at the preview
   form (`skillhub skill activate demo`) instead. `skillhub status` says "No
   skills yet" while drafts exist; make the inventory line count drafts too
   (e.g. "0 active, 1 draft"). Do not change JSON field meanings.

7. Docs: docs/user-guide.md (connect, curating), docs/curating-skills guide if
   it describes the server toggle, the design note §7 table (step 4 done) only
   if you can do it without rewriting decisions.

Smoke (report, no commit unless it finds a bug), temp HOME + temp project +
temp workspace, binary built from your branch:
- `skillhub connect --workspace <ws> --yes`: `.mcp.json` has only `skillhub`
  with `--profile runtime`; `.claude/settings.local.json` has the four rules and
  the additionalDirectories; output shows the new guidance. Second run: no-op,
  no guidance. Seed a wave-4 pair first in another project and show it converts.
- `skillhub connect -g --workspace <ws> --yes` with the temp HOME: same checks
  on the user files.
- Codex/Gemini files unchanged from main's behavior.

When done, report: commits (hash + subject), files changed, tests added, `make
check` result, the smoke output, and anything you noticed but did not fix.
```
