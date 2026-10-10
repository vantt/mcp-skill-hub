# Observer Handoff after Worktree G (Profile Split & Recovery)

## 1. Profile Split Verification Steps & Sources
All hosts are currently unverified; `integrate` writes one full entry everywhere.
- **Claude Code**: Run `/mcp` to disable `skillhub-curation`, verify `claude -p "task"` omits curation tools and tools/list stays ~4.2k tokens. Source: `anthropics/claude-code#78314` (unconfirmed).
- **Cursor**: Toggle off `skillhub-curation` under Settings > Features > MCP, check tools panel in chat. Source: `forum.cursor.com/t/how-to-disable-mcp-server/86531` (unconfirmed).
- **Codex CLI**: Set `enabled = false` in `[mcp_servers.skillhub-curation]` of `.codex/config.toml`, test with project trust. Source: `internal/hostintegration/preview.go:51`.
- **Gemini CLI**: No per-server toggle supported in `.gemini/settings.json`. Source: `internal/hostintegration/preview.go:49`.
- **Claude Desktop**: Toggle off in Settings > Developer > MCP UI. Source: Anthropic Desktop docs (unconfirmed).

## 2. Observer Phase 3 Options (distill-lab `where: usage:<case_id>` blocker)
1. **Eval Case Promotion First (Recommended)**: Store candidates as Git eval cases (`evals/routing/<case_id>.json`) and cite them in `where` using standard `repo@commit:path` format without changing distill-lab.
2. **Upstream distill-lab Expansion**: Update `.claude/skills/distill-lab/scripts/distill.py` to allow `where: usage:<case_id>` for candidate lessons.
3. **Hub-side Candidate Store**: Hold candidates in local `runtime/candidates/` until human confirmation promotes them to Git.

## 3. Gotchas
- **outputSchema vs normalizeMultiTypeSchemas**: `go-sdk` validates live tool outputs against `outputSchema`. Pruning `anyOf: [null, T]` breaks on `nil` Go pointers/slices (`null` in JSON), failing validation. Strip only metadata (`description`, `title`, `additionalProperties`).
- **Paging cursor binding**: `paging.Owner(filter, ...)` must bind `filter` (`"state=" + state`) and items; mismatched filters reject valid cursors as expired.
- **CLAUDE.md is gitignored**: `.gitignore` ignores `CLAUDE.md`. The canonical host template is `internal/hostintegration/bootstrap.go` (`bootstrapBlock`), which synchronizes `AGENTS.md` and project files via `updateBootstrap`.

## 4. Noticed but Unfixed (Out of Scope / Worktree C Ownership)
- `internal/delivery/mcpserver/server.go:215-217, 260-262`: Returns code `snapshot_expired` for malformed/missing URI inputs instead of `invalid_params` (owned by Worktree C).
- `internal/delivery/mcpserver/server.go:527-533`: `committedResponse` schema injection for `skill_resolve` and `routing_evaluate` injects `$ref: "#/$defs/setup"`, panicking if root-level `$defs` are pruned.
