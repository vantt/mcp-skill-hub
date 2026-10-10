# Wave 5 · Worktree J prompt: curator via CLI (steps 1–2)

Start the agent inside its worktree:

```bash
cd /home/vantt/projects/mcp-skill-hub-curator-cli && omp --profile=<profile>
```

```text
You are worktree J. Make the CLI a complete, agent-friendly curation interface
and teach the system-curator skill to use it on hosts with a shell. Go repo
mcp-skill-hub, test-first.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory)
- Worktree /home/vantt/projects/mcp-skill-hub-curator-cli, branch
  wave5/curator-cli (the lead created it from current main).
  `git -C /home/vantt/projects/mcp-skill-hub-curator-cli status -sb` must show it.
- ABSOLUTE paths only, under /home/vantt/projects/mcp-skill-hub-curator-cli/.
- Never write to /home/vantt/projects/mcp-skill-hub (lead's main checkout) or
  /home/vantt/skill-hub (the live hub). Do not push. Never run CLI commands
  against /home/vantt/skill-hub; use a temp workspace (`skillhub init <tmp> --yes`).
- Never edit .claude/skills/distill-lab. Never remove telemetry event types or
  change EventVersion. Tests must not read paths outside the repo or a temp dir.
- Run `make check` with a temp HOME and XDG_* dirs outside the repo (keep
  GOCACHE/GOMODCACHE/GOPATH): TestDoctorHostIntegrationPreviewIsReadOnlyAndDependencyOrdered
  reads the real HOME today (known, not yours to fix). Rerun TestGitRevisionAt
  once if it fails with "broken pipe".

Read first: docs/plans/2026-10-10-curator-via-cli.md (approved design; you do
§4 and §6, which are steps 1 and 2 of §7), AGENTS.md,
internal/systemskills/curator/SKILL.md, docs/user-guide.md.

Out of scope (later steps, do NOT do): connect/integrate changes, removing the
skillhub-curation entry, writing permission rules (§5.2), the MCP profiles, the
matrix. Do not remove or rename any MCP tool. Do not change MCP tool behavior.

Do, in order, one commit per item (conventional commits), `make check` green
before each:

1. CLI next-step commands (§4.1). JSON `suggested_actions[].command` today
   carries MCP-style names (`skill_active`, `skill_edit`, `skill_create`,
   `workspace_diff`). Add a `cli` field with the exact runnable CLI command
   to every suggested action the CLI emits, and to every preview result a
   field with the full confirm command bound to the reviewed proposal:
   `skillhub skill confirm --proposal <id> --proposal-digest <d> --base-version <v> --yes`
   (or the equivalent for source flows). Keep `command` unchanged for MCP
   clients. Check every preview path: skill add (remote and local), create,
   edit, activate/deprecate/archive, source watch, source import (item 3).
   Test each one: the emitted CLI command parses and, run against a temp
   workspace, applies exactly the previewed proposal.

2. `skillhub eval routing --json` (§4.2) prints a bare JSON object. Wrap it in
   the common result envelope (schema_version/status/summary/items/
   suggested_actions/warnings/error) with the metrics under a named field.
   Check nothing in the repo parses the old shape (web, tests, scripts); update
   them if so.

3. Source import via CLI (§4.3). MCP has source_import_preview/confirm; the CLI
   has capture + triage --decision import. Find out whether the CLI path gives
   a side-effect-free preview with the discovered skills and conflicts in
   JSON. If not, add `skillhub source import <locator> [--ref r] [--path p]
   [--skill s | --all] [--yes] [--json]`: preview by default, `--yes` applies,
   calling the same app service the MCP tools use. Help text, tests (preview
   writes nothing; apply creates drafts only).

4. Curator skill (§6). One SKILL.md for both interfaces, both copies
   (internal/systemskills/curator/SKILL.md and system-skills/curator/SKILL.md)
   identical, version bumped the way earlier curator edits did (embed.go
   CuratorSkillVersion, doctor/native receipts if they key on it).
   - A short section at the top, "Choose the interface": if you can run shell
     commands and `skillhub version` works, use the CLI with `--json` and
     `--workspace` when needed; otherwise use the MCP tools. Say why (one
     server fewer, CLI accepts local folders, same service underneath).
   - The intent table gets a CLI column next to the MCP column, every row,
     with real commands you have run against a temp workspace.
   - Safety rules for the CLI path, written as explanations, not bare rules:
     always run the preview form first and show the user what will change;
     confirm only after explicit approval, using the bound confirm command
     from item 1 (never a bare `--yes` on the first call, never
     `skill confirm <id>` without digest); never use `--approve-content`,
     `--force`, `telemetry purge`, `migrate --yes`; `skillhub check` uses
     the network, so run it only when the user asks.
   - `curation_session_record` is MCP-only; on the CLI path skip it (§4.5).
   - Style: for each intent say why, what to ask the user, what to look at in
     the JSON, and one example command; not a terse table only. Keep the file
     readable; move long per-intent detail into a references/ file only if
     SKILL.md becomes hard to scan, and make sure it is installed with the
     skill.
   - Keep the server-boundary test (compatible-tools ⊆ curation profile tools)
     passing; update compatible-tools only if a tool really changed.
   - Update docs/user-guide.md where it describes curating.

5. End-to-end check with a real host (report only, no commit unless it finds a
   bug): in a temp HOME and temp workspace with one draft skill, run
   `claude -p "Curate my Skill Hub: show status and review the draft skill. Inspect only, change nothing." --permission-mode default`
   with ONLY the runtime MCP entry configured (no skillhub-curation), the
   native curator installed by `skillhub connect`, and Bash allowed for
   `skillhub` (e.g. `--allowedTools "Bash(skillhub:*)"`). Confirm it uses the
   CLI, reports status and the review, and the workspace is unchanged
   (`git status`, `skillhub diff`). If `claude` is unavailable or needs login,
   say so and skip.

When done, report: commits (hash + subject), files changed, tests added, `make
check` result, the CLI commands you verified per intent, the item 5 transcript
summary, and anything you noticed but did not fix.
```
