# Host check: does any stock MCP host use the Skills extension?

Date: 2026-10-12 (Asia/Saigon). Extension key: `io.modelcontextprotocol/skills`.

## Result

No host could be observed. Every installed host stopped at authentication before it started the MCP server, so there is no executed evidence either way for any host. Nothing was promoted to `verified`.

## Setup

- Temp hub from `skillhub init` in an empty directory, binary installed from this worktree into a temp `GOBIN`, HOME/XDG/`SKILLHUB_USER_CONFIG_DIR` and the global git config redirected to temp paths. One active skill (`probe-skill`) plus the bundled `system-curator`.
- Logging wrapper (bash, `tee` on both directions) as the MCP server command, exec'ing `skillhub mcp serve --profile runtime --workspace <temp hub>`.
- Wrapper self-test with a hand-written client: initialize, `skills/list` were logged in both directions, so the capture path works.

## Evidence table

| Host | Version | Run | Observed |
|------|---------|-----|----------|
| Claude Code | 2.1.296 | `claude -p "say ok" --mcp-config <temp> --strict-mcp-config`, temp cwd, temp `CLAUDE_CONFIG_DIR` | "Not logged in", exit before MCP start; wrapper log empty. Blocked by authentication. |
| Codex CLI | 0.161.0 | `codex exec`, temp `CODEX_HOME` with `[mcp_servers.skillhub]` | HTTP 401 to the responses endpoint, five reconnects, no MCP start; wrapper log empty. Blocked by authentication. |
| Gemini CLI | not installed | `command -v gemini` found nothing | Skipped. |

Logging in inside the temp config dirs would need the user's credentials. Using the real config dirs would write to `~/.claude.json` / `~/.codex`, which was out of bounds. Per instruction no workaround was attempted.

To finish the check, the user can run the same wrapper with their own login: `claude -p "say ok" --mcp-config <json using wrap.sh> --strict-mcp-config` from an empty directory, then read the logged `initialize` (`capabilities.extensions`, `clientInfo`) and the method names. Note this writes to their own Claude config.

## Server side (executed)

The server always advertises `capabilities.extensions["io.modelcontextprotocol/skills"] = {"directoryRead": false}` and answers `skills/list` and `skills/get` to any client. It does not read the client's declared extensions at all. The only gate is `nativeCuratorForSession`, which looks at `clientInfo.name` and `HostSupportsSkillsExtension` (matrix entry `verified` with evidence):

| clientInfo.name | client capabilities | `skills/list` returned |
|-----------------|---------------------|------------------------|
| anon | `{}` | probe-skill, system-curator |
| claude-code | `{}` | probe-skill (curator omitted, native install assumed) |
| codex-mcp-client | `{}` | probe-skill (curator omitted) |
| unknown-host | declares the skills extension | probe-skill, system-curator |

So a non-declaring host still gets the full list; only identified Claude/Codex/Gemini hosts lose the curator, and that is based on name, not on a declaration.

## Matrix changes

Only the `skills_extension.reason` text of Claude Code and Codex CLI was extended with this attempt; statuses stay `unverified`. Gemini CLI untouched. Both matrix files are kept identical.

## What this means for the plan

Not decided by this run. Nothing shows any stock host declares or calls the extension, so the case for building skills extension support into the proxy or stateless HTTP path rests on the spec, not on host behaviour. Until a logged-in run of Claude Code and Codex CLI is captured, keep MCP tool delivery (`skill_get`) as the primary path and treat extension work as speculative. One more reason for caution: the curator is withheld from identified native hosts on name alone, so if an identified host does not call `skills/list`, the curator reaches it only through native files.
