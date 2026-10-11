# Troubleshooting page report

Wrote `docs/troubleshooting.md` (147 lines, budget 150). H2s match the frozen list: Start with doctor, Setup and
connection, Skills, Workspace and index, Windows. Every relative link and anchor resolves (checked with a script).

## Verified in sandbox `/tmp/skillhub-docs-rewrite/troubleshooting`

- No workspace found: `skillhub status` in an empty folder, exit 2, error text copied verbatim.
- Activation refusal: bare `skill create` then `skill activate --yes`, exit 2, WHY text copied. The one-edit fix
  (`skill edit <id> --content-file ... --trigger ... --not-for ... --min-scope single_step --yes`) then
  `skill activate --yes` succeeds.
- Resolve without `task.scope`: prints the scope question (exit 0); `"scope"` inside `task` resolves.
- `doctor` / `doctor --fix` / `doctor --fix --yes`: with permission entries stripped from
  `.claude/settings.local.json`, and separately with a wrong binary path in `.mcp.json`, doctor reports "Current
  project connection ... is outdated or needs repair" and `--fix --yes` rewrites the file. Repair is limited to
  hosts already connected to this workspace (code: `internal/app/host-connections.go:59-63`).
- Deleting `.mcp.json` makes the project unrecognized (all commands give "No Skill Hub workspace was found");
  doctor with `--workspace` then reports healthy and does not recreate it. Fix documented: `connect` again.
- `connect -g` with `~/.claude` as a symlink (sandbox HOME): refusal text copied; `--host codex,gemini` previews fine.
- Stale index: hand edit gives "Search index is stale"; `skill list` rebuilds it automatically. Broken
  `.meta/skill.yaml`: `validate` names the file, `rebuild` refuses (exit 2).
- Fresh clone: `status` shows "search index missing"; `rebuild --workspace` fixes it.
- Telemetry: corrupted `runtime/telemetry.db` gives `file is not a database (26)`; `telemetry purge --yes` fixes;
  `purge` without `--yes` exits 2.
- `skill doctor <id>`: runs, "State: ready" for a skill with no runtime block.
- `review_required` / `setup_failed`: wording matches the instruction block `connect` writes into `CLAUDE.md`.
  The **Content trust** block and its approve command come from code (`internal/delivery/cli/skill_review.go:130-148`).
- Large repository `--skill` failure and the `--path skills/pdf` form: from the wave 0 transcript (no network here).

## Dropped or changed from the old text

- Degraded MCP startup tool list: dropped. The runtime profile lists only 3 tools, and I could not corrupt the
  catalog (generation files are read-only) to test the claim.
- `local.path` expiry: dropped (the agent instruction block already covers it; no user action).
- `validate --staged`: left for the curating page as planned.
- "Resource-verified fallback and state basis": one plain paragraph under "The search index is stale", linking
  `contracts/error-codes.md#error-code-registry` and `contracts/result-envelope.md`.

## Observations for the owner

- A local-path `skill add` shows `Git: clean` in `skill review` for an untracked skill folder in `skills/default/`,
  while wave 0 saw "1 untracked" for a created skill. Follow-up 5 may still apply to local adds.
- `doctor` does not notice a deleted `.mcp.json`; the project simply stops being recognized.

## Unverified

- Windows SmartScreen and firewall wording (kept from the old guide, not runnable here).
- Codex and Gemini CLI trust prompts (stated as not verified end to end).
- Claude Code asking to approve the MCP server separately (kept from the old guide).
- The `review_required` path end to end (needs a GitHub source; network not allowed in this phase).

Status: DONE_WITH_CONCERNS
Summary: Troubleshooting page written within budget; all offline symptoms reproduced and error texts copied from
real runs. Windows items, third-party content approval end to end and the degraded MCP claim are unverified.
