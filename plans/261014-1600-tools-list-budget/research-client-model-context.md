# What part of a tool definition reaches the model

Checked 2026-10-14 (Asia/Saigon). This file records the evidence the plan's priorities rest on.

## Method

- **Empirical probe (Claude Code, Codex CLI).** A throwaway stdio MCP server served one tool whose `title`,
  `description`, `inputSchema` (property description, `$defs`, `maxLength: 777`), `outputSchema` and
  `annotations.title` each carried a unique marker, plus a marker in the initialize `instructions`. Each client ran one
  non-interactive turn against a local HTTP capture server (dummy key, isolated `CLAUDE_CONFIG_DIR` / `CODEX_HOME`,
  `--strict-mcp-config`), and the captured model request was searched for each marker. The probe was then repeated with
  the real runtime `tools/list` (dumped from `listProfileTools(ProfileRuntime)` by a temporary test that was deleted).
  No network secrets, no live hub, no real project. Probe files lived in the session scratchpad only.
- **Source reading** for Claude Code (bundled JS in the 2.1.299 binary) and Gemini CLI (GitHub `main`).
- **Public statements** for Cursor.

## Results

| Client (version) | name | description | inputSchema | outputSchema | title / annotations | server `instructions` | Confidence |
|---|---|---|---|---|---|---|---|
| Claude Code 2.1.299 | yes (`mcp__<server>__` prefix) | yes, verbatim | yes, verbatim: keeps `$id`, `$schema`, `$defs`, `maxLength` etc. | **no** | **no** (annotations drive permissions only) | yes, system prompt section "MCP Server Instructions" | High (wire capture + source) |
| Codex CLI 0.161.0 | yes, inside a `namespace` tool `mcp__<server>` | yes | yes, **sanitised**: drops `$id`, `$schema`, `maxLength`, `minLength`, `maxItems`; keeps `$defs`/`$ref`, `enum`, `required`, `additionalProperties`, property descriptions | **no** | **no** | yes, as the namespace `description` | High (wire capture) |
| Gemini CLI (`main`, 2026-10-14) | yes | yes | yes (`parametersJsonSchema: toolDef.inputSchema`, null-type arrays rewritten) | **no** (only a lenient validator in the client) | **no** (`readOnlyHint` sets `isReadOnly`) | not checked here (instructions plan, phase 3) | Medium-high (source via fetch, not run) |
| Cursor | names in a compact catalog; description + input schema loaded on demand | on demand | on demand | unknown | unknown | unknown | Low (staff forum statement, no wire check) |

### Claude Code tool search (deferral)

With tool search enabled (`ENABLE_TOOL_SEARCH=true` in the probe; the default on first-party sessions, as this very
session shows `mcp__skillhub__*` as deferred tools), MCP tools are **not** in the request's `tools` array. Only the
tool name appears in a "deferred tools" list in the system prompt. The full `{name, description, input_schema}` enters
context when the agent loads the tool (`ToolSearch select:`). Because the Skill Hub protocol tells the agent to call
`skill_resolve` on every new task, `skill_resolve` is loaded in almost every session, and `skill_get` /
`skill_feedback` whenever a skill is used. So the per-tool model-visible size still matters; `skill_resolve` matters
most. Claude Code also reads `_meta["anthropic/alwaysLoad"]` and `_meta["anthropic/searchHint"]` (not used by the hub;
out of scope).

## Measured runtime profile (from the dumped `tools/list`)

| Tool | description | inputSchema | outputSchema | title + annotations | Model-visible (Claude form) | Sent by Claude Code | Sent by Codex |
|---|---|---|---|---|---|---|---|
| skill_resolve | 363 | 3,822 | 4,941 | 106 | 4,240 | 4,255 | 3,551 |
| skill_get | 347 | 131 | 3,422 | 101 | 529 | 544 | 543 |
| skill_feedback | 133 | 931 | 1,783 | 113 | 1,120 | 1,135 | 1,032 |
| **Total** | **843** | **4,884** | **10,146** | **320** | **5,889 (~1.5k tokens)** | **5,934** | **5,126** |

Bytes are compact JSON. "Model-visible (Claude form)" is `{"name","description","input_schema"}` per tool without the
client's name prefix: the part the server controls. Wire `tools/list` is 16,532 bytes (`toolsListDigests`), so
**about 64% of the wire bytes never reach the model** in the three verified clients.

Inside `skill_resolve.inputSchema` (3,822 bytes): `$defs` activationContext 789, context 870, fact 365, prior 376;
`task` 714; `$id` + `$schema` 133; length/count/uniqueness keywords 645 more (778 with `$id`/`$schema`); property
descriptions 79.

Curation profile (23 tools): wire 101,354 bytes; outputSchema 79,637 (78%); model-visible 18,134 bytes (~4.5k
tokens).

## Other costs of outputSchema (why it stays)

- The Go SDK validates every tool result against the advertised `outputSchema` before sending it
  (`go-sdk@v1.8.0/mcp/server.go:460`). Removing or loosening it removes the server's own output self-check.
- `skill_resolve`'s output embeds the committed response contract `schemas/skill-resolve-response-v1.schema.json`,
  asserted equal by `server_test.go:196-215`.
- The only cost found is wire bytes per session (16.5 KB local stdio; the proxy builds the same definitions locally,
  `proxy.go:356`, so no network transfer). No startup or memory issue is attributed to it.

Decision: keep `outputSchema`. Trimming it saves no model context in any verified client.

## Agent use of optional request fields (telemetry, read-only)

The live hub's `runtime/telemetry.db` (read-only, `mode=ro`) has 15 `resolution.started` events (13 from claude-code).
All 15 have `constraint_count: 0` and empty `fact_keys`. Telemetry does not record whether `activation_context`,
`context.execution` or `prior` were sent. The sample is small; it hints that the optional blocks are rarely used but
does not prove it.

## Side finding for the server-instructions plan

Codex CLI 0.161.0 does read `instructions` (as the MCP namespace description). This answers part of
`plans/261014-1509-server-instructions-protocol` phase 3 for Codex.

## Sources

- Claude Code 2.1.299 binary `~/.local/share/claude/versions/2.1.299`: API tool built as
  `{name, description, input_schema, [strict], [defer_loading], [cache_control]}`; MCP tool `inputJSONSchema` from
  `inputSchema`; `anthropic/alwaysLoad` doc string ("Default: tools are deferred when tool search is enabled").
- Codex CLI 0.161.0 (`~/.codex/packages/standalone/current/bin/codex`), wire capture.
- Gemini CLI: [`packages/core/src/tools/mcp-client.ts`](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/tools/mcp-client.ts),
  [`mcp-tool.ts`](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/tools/mcp-tool.ts),
  [`tools.ts`](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/tools/tools.ts) (`getSchema()`
  returns `name`, `description`, `parametersJsonSchema`).
- Cursor: [forum thread on MCP token cost](https://forum.cursor.com/t/what-do-your-attached-mcp-servers-actually-cost-you-in-tokens-per-request-i-measured-it/166405)
  (staff: dynamic discovery, full schema loaded when needed).
- Claude Code feature request saying outputSchema does not reach the model:
  [issue 54197 mirror](https://claudeissues.com/issue/54197-feature-surface-mcp-tool-outputschema-to-the-model-context-not-just-the-renderer).
