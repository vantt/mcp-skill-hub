# Phase 3: Client support research (time-boxed to 30 minutes)

## Goal

Record how Claude Code, Codex CLI, Cursor and Gemini CLI surface the MCP `instructions` field of the initialize result.
This phase only records findings. It never changes scope: a gap goes to the plan's open questions, not into a
fallback.

## Questions per client

1. Is `instructions` read at all, and where does it go (system prompt, tool context, ignored)?
2. Is there a size cap, and is truncation silent?
3. With several servers (`skillhub` and `skillhub-curation`), is the combined text capped?
4. Does it apply to stdio servers (all Skill Hub registrations are stdio)?

## Preliminary evidence (from planning, unverified)

- **Claude Code.** Community reports say v2.1.84 and later read `instructions` and cap it, together with each tool
  description, at 2048 characters (UTF-16 units). Truncation is client-side and silent, and with many servers the
  combined text can be cut. Sources:
  [docs gap report](https://claudeissues.com/issue/39106-docs-mcp-docs-do-not-mention-the-2kb-cap-on-tool-descriptions-and-server-instruc),
  [2048-char truncation report](https://claudeissues.com/issue/81268-mcp-truncation-at-2048-chars-is-invisible-tool-description-leaves-no-log-mcp-sho),
  [multi-server truncation report](https://claudeissues.com/issue/43474-mcp-server-instructions-are-silently-truncated-when-multiple-servers-are-configu).
  Phase 1's 2048 cap test comes from this.
- **Codex CLI, Cursor, Gemini CLI.** A first search found no statement either way.

## Steps

1. For each client, check its official docs and changelog. For the open-source clients (Codex CLI, Gemini CLI), also
   grep the source for `instructions` in the MCP client code. Record the version checked.
2. Optional, if a client is installed: register a throwaway stdio server that only returns a unique marker in
   `instructions`, then ask the agent whether it sees the marker. Use a temp project and a temp `HOME` only.
3. Write the findings to `plans/261014-1509-server-instructions-protocol/reports/client-instructions-support.md`, as a
   table of client, version, reads instructions (yes/no/unknown), cap, and source URL.
4. Send the lead a two-line summary. Any "no" goes to open question 1.

## Output used by

- Phase 4 docs: one sentence in `docs/user-guide.md` naming the clients known to read server instructions.
- Phase 1: confirms or tightens the 2048 cap.

## Risk and rollback

There is no code risk. If the time box runs out, record "unknown" and move on.
