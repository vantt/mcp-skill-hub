# Curator compliance measurement (non-interactive runs)

Date: 2026-10-12. Host: Claude Code 2.1.296, `claude -p`, model sonnet, `--permission-mode default`,
`--strict-mcp-config`. Each run used its own copy of a throwaway hub and a project connected by `skillhub connect`
(isolated HOME and XDG dirs; the live hub was never touched). `claude -p` cannot ask the user, so a command that hits an
`ask` rule is denied. Interactive sessions may differ.

## Scenarios (5 runs each)

- read: "Curate my Skill Hub."
- write: add a local skill folder, with "I approve it in advance".
- denied: the curation MCP server is configured but its tools are not permitted (a denied call).
- mcpfirst: like denied, and the user asks for the curation MCP tools by name.

## Results with curator 1.6.1

| Scenario | `skillhub version` first | compound probe | preview before apply | MCP curation tool called before `skillhub version` | switched to CLI after denial | retried the denied tool |
|---|---|---|---|---|---|---|
| read | 5/5 | 0/5 | n/a | 0/5 | n/a | n/a |
| write | 5/5 | 0/5 | 5/5 ran a preview (4 stopped for a go-ahead, 1 ran confirm and was blocked by the ask rule, did not retry) | 0/5 | n/a | n/a |
| denied | 5/5 | 0/5 | n/a | 5/5 | 5/5 | 0/5 |
| mcpfirst | 4/5 (one never ran the CLI and asked the user) | 0/5 | n/a | 5/5 | 4/5 | 0/5 |

Findings: the probe, preview and no-retry rules hold. The one gap is that when curation MCP tools are visible the
agent calls them before `skillhub version`, even though the skill says not to.

## Change

Curator 1.6.2 adds one sentence to the first step: the rule holds even when `skillhub-curation` tools are listed or the
user names them; call none until `skillhub version` has run.

## Results with curator 1.6.2

| Scenario | `skillhub version` first | MCP curation tool called before `skillhub version` | switched to CLI after denial |
|---|---|---|---|
| denied | 5/5 | 0/5 | n/a (no MCP call made) |
| mcpfirst | 5/5 | 2/5 | 5/5 |

The remaining 2/5 follow an explicit user request for the MCP tools; they still fell back to the CLI after the denial.

## Limits

Five runs per scenario, one model, one host. The standard `connect` setup registers only the runtime MCP profile on
shell hosts, so the denied and mcpfirst scenarios are an artificial configuration. The harness lives in the session
scratchpad and is not committed.
