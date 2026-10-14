Summary: Claude Code, Codex CLI and Gemini CLI read the MCP `initialize` `instructions` field for stdio servers; Cursor is unknown (no doc, source or probe). Claude Code caps it per server at 2048 chars with a "… [truncated]" marker, Codex has no practical cap, Gemini CLI only loads it in trusted folders.
Open question 1: Cursor ("unknown") is the only gap. Per the user decision there is no fallback; docs say "tools only, no protocol" for clients that ignore it.

# Client support for MCP server `instructions`

Researched 2026-10-14. No live client probe: no Gemini/Cursor installed, and a Claude/Codex probe would need real auth config in HOME, which is off-limits for this phase. Findings come from official docs, a maintainer statement and a shallow read of the open-source client code.

| Client | Version checked | Reads `instructions` | Where it goes | Cap | Source |
|---|---|---|---|---|---|
| Claude Code | 2.1.299 installed; behaviour stated for >= 2.1.84, maintainer reproduced on 2.1.233 | yes (stdio confirmed) | injected into the model context | 2048 chars per server, per-server not shared; same 2 KB limit on each tool description | [issue 43474](https://claudeissues.com/issue/43474-mcp-server-instructions-are-silently-truncated-when-multiple-servers-are-configu) (maintainer comment), [issue 39106](https://claudeissues.com/issue/39106-docs-mcp-docs-do-not-mention-the-2kb-cap-on-tool-descriptions-and-server-instruc) |
| Codex CLI | source `openai/codex` main @ `400f235` (2026-10-14, 0.163.0-alpha line) | yes | becomes the description of the server's tool namespace (`namespace_description`), shown to the model with that server's tools | 512 KiB (`MAX_MCP_NAMESPACE_DESCRIPTION_BYTES`), effectively none; docs advise a self-contained first 512 chars. Agent-plugin servers: 1000 bytes | [docs](https://learn.chatgpt.com/docs/extend/mcp?surface=cli), `codex-rs/codex-mcp/src/rmcp_client.rs:1073`, `codex-rs/core/src/tools/handlers/mcp.rs:48-49,536` |
| Gemini CLI | source `google-gemini/gemini-cli` main @ `42c5182` (0.65.0-nightly; stable v0.64.0) | yes | all servers' instructions are wrapped in `---[start of server instructions]---` markers, concatenated, and appended to project memory (system instructions) | none found | `packages/core/src/tools/mcp-client-manager.ts:696`, `packages/core/src/context/memoryContextManager.ts:124`; feature request [#8485](https://github.com/google-gemini/gemini-cli/issues/8485) |
| Cursor | not installed; docs checked 2026-10-14 | unknown | unknown | unknown | [docs](https://cursor.com/docs/mcp) never mention the field; one third-party issue asserts Cursor shows it, unverified |

## Answers per question

1. **Read at all, and where.** Claude Code: yes, into model context. Codex: yes, as the namespace description of that server's tools. Gemini CLI: yes, appended to the system-level project memory. Cursor: unknown.
2. **Size cap, silent truncation.** Claude Code truncates at 2048 chars; the text gets a `… [truncated]` marker, but the only log is `claude --debug` (no `/mcp` or startup warning). Codex: 512 KiB limit, no concern for our size. Gemini: no cap found in the code read. Cursor: unknown.
3. **Several servers (`skillhub` + `skillhub-curation`).** Claude Code: the cap is per server, not shared (maintainer: "a per-server cap, not a shared budget"), so two servers do not cut each other. Reports of a later server's block missing entirely exist (Cowork; remote OAuth HTTP servers), the maintainer tracks the remote case separately. Codex: each server is its own namespace; a namespace-name collision between two servers is an error, but different server names are fine. Gemini: servers are concatenated with no cap.
4. **stdio.** Claude Code: yes, the maintainer reproduced truncation with three stdio servers (one commenter saw stdio injection working). Codex: the field is taken from the initialize result for every transport, nothing stdio-specific. Gemini: same, `getInstructions()` is transport-agnostic. Cursor: unknown.

## Consequences for the plan

- The 2048 cap in the server-instructions phase is confirmed: per server, applied on the client, silent. Keep the text under 2048 UTF-16 units (the issue counts characters), not just bytes.
- Gemini CLI only connects MCP servers (and so loads instructions) in a trusted folder (`mcp-client-manager.ts:461,576`; `memoryContextManager.ts` clears project memory when untrusted). Worth one clause in the docs.
- Docs sentence for the user guide: "Claude Code, Codex CLI and Gemini CLI read the server instructions; Cursor's behaviour is not documented." Any client that ignores them is "tools only, no protocol"; no fallback is added.
- Not verified here: the actual rendering inside each client's prompt (no live probe). A marker-string probe with a temp HOME is still possible later if wanted.
