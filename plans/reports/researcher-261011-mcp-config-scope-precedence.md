# MCP server name collision: user vs project scope (2026-10-11)

## Bottom line
Project scope beats user scope in all three tools. How they combine differs: Claude Code and Gemini CLI replace the whole entry for that name. Codex deep-merges the entry field by field.

## (a) Codex CLI
- **Precedence:** CLI flags/`-c` > project `.codex/config.toml` (closest to cwd wins) > `--profile` > `~/.codex/config.toml` > cloud-managed > `/etc/codex/config.toml` > defaults.
- **Trust:** "Codex loads project `.codex/` layers only when you trust the project." Untrusted projects skip project config, hooks, and rules. MCP docs: project scoping is for "trusted projects only".
- **Merge:** The docs do not say. Source (`codex-rs/config/src/merge.rs`, `merge_toml_values`) merges tables recursively. A higher layer's `[mcp_servers.skillhub]` therefore overrides only the keys it sets. Keys set only in the user entry survive (for example `env`, `args`, `enabled`). **Unconfirmed:** whether a mixed result (user `command` + project `url`) gets rejected when the merged config is checked. Not tested.
- Sources: https://learn.chatgpt.com/docs/config-file/config-basic , https://learn.chatgpt.com/docs/extend/mcp , https://github.com/openai/codex/blob/main/codex-rs/config/src/merge.rs

## (b) Gemini CLI
- **Precedence:** defaults < system-defaults < user `~/.gemini/settings.json` < workspace `.gemini/settings.json` < system settings < env < CLI args.
- **Trust:** Trusted Folders is "disabled by default". When it is on and the folder is untrusted, the CLI "will not load the `.gemini/settings.json` file from the project" and skips MCP servers. Source: `mergeSettings` swaps the workspace settings for `{}` when the folder is untrusted.
- **Merge:** The docs do not say. Source: `mcpServers` has `mergeStrategy: SHALLOW_MERGE` (`{...user, ...workspace}`). Same name means the workspace entry replaces the user entry whole, with no field merge. Names that do not collide are combined.
- Note: the docs banner says Gemini CLI was replaced by Antigravity CLI on 2026-06-18 for unpaid and Google One users.
- Sources: https://geminicli.com/docs/reference/configuration , https://geminicli.com/docs/cli/trusted-folders , https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/config/settingsSchema.ts , .../src/utils/deepMerge.ts

## (c) Claude Code
- **Precedence:** local > project (`.mcp.json`) > user > plugin > claude.ai connectors.
- **Merge:** "The entire server entry from that source is used; fields are not merged across scopes."
- **Approval:** Interactive sessions ask for approval before using `.mcp.json` servers. `claude -p` and SDK sessions load them without asking. In untrusted folders, a committed `enableAllProjectMcpServers` setting is ignored.
- Source: https://code.claude.com/docs/en/mcp

## Implication for "skillhub"
If a user-scope `skillhub` entry and a project-scope one both exist, Claude and Gemini use the project entry as written. Codex combines the two, so stale user-level keys can leak into the result. Keep one scope per host, or make the project entry set every field.

## Unresolved
- Codex validation of merged mixed-transport entries (not tested).
- Codex merge behavior comes from source on `main`, not from docs. It could change without notice.
