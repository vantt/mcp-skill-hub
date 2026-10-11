# Command reference report

Wrote `docs/command-reference.md` (88 lines, budget 90). Five H2s exactly as frozen (Set up, Work with skills,
Sources and learning, Workspace and Git, Routing and usage), one Task | Command table each.

## Verification

- Every row checked against `/tmp/skillhub-docs-rewrite/help/*.txt`.
- Smoke-ran in `SB=/tmp/skillhub-docs-rewrite/command-reference` (workspace + one draft skill): `skill list --state`,
  `skill show`, `skill env list`, `skill doctor` (rejects drafts, as expected), `skill outdated`, `source list`,
  `check --all-due`, `diff`, `validate`, `validate --staged`, `rebuild`, `migrate`, `eval routing`,
  `telemetry health`, `telemetry funnel`, `status`, `integrate` (alias works). `update --check` was not run
  (network/binary); `update` rejects `--workspace`, so the intro says "skill and source commands need a workspace"
  rather than "all commands".

## Disagreements between the old tables and how they were resolved

| Old table entry | Resolution (`skillhub help` wins) |
|---|---|
| `skillhub inbox`, `skillhub insight ...` (both tables) | Dropped; the commands do not exist. Lesson flow described via web UI **Sources** > **Distill with Curator Agent** > skill **Distill** tab. |
| `skillhub update [--yes]` (user guide) | `update` has no `--yes`; shown as `update --check` and `update [--version <v>]`. |
| `skill show <id>` (user guide) vs `skill show <id> [--verbose]` (curating) | `--verbose` exists; kept. |
| `source check --all-due` with alias `skillhub check` | Reversed: `skillhub check` is the top-level command; `source check` is its alias (per help). |
| Connect rows without `--host`/`--project` | Added both from `help connect`; `integrate` mentioned only as "also `integrate`" (it has no help of its own). |
| `skill edit <id> --approve-content <digest>` | Kept, marked "terminal only" (agents are denied the flag by the connect permission rules). |
| Missing in both: `skillhub check`, `serve web`, `mcp serve`, `source show`, `telemetry export` | Added (all listed in help). |

Left out on purpose: `connect --hub` and any remote hub command; `eval manifest|run|promote` and
`resolution replay` (help calls them developer tools; one pointer line instead); `source confirm` and
`source import --proposal ...` (internal follow-ups printed by previews).

## Observation for the lead

- In this sandbox, `skill review demo` on a freshly created, untracked skill printed `Git: clean`, which
  contradicts the frozen map item that follow-up 5 does not reproduce. Not used in the doc.
- `validate --staged` on a workspace with nothing staged fails on missing `.gitignore` entries (the file is not
  staged yet). Not used in the doc.

## Unverified

- `skillhub update --check` and `update --version` (not run).
- `skill edit --approve-content` (no third-party skill in the sandbox).

Status: DONE
Summary: Merged command reference written and checked against help captures and sandbox runs.
