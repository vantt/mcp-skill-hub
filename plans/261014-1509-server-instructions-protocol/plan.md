---
title: "Server instructions carry the Skill Hub protocol"
description: "The skillhub MCP server sends the usage protocol in its initialize instructions, generated from the tools it registers; connect stops writing the CLAUDE.md/AGENTS.md/GEMINI.md block and doctor --fix removes the old one."
status: completed
priority: P2
effort: 7h
branch: worktree-agent-a169e2483df1cb908
tags: [mcp-server, host-integration, doctor, docs]
blockedBy: []
blocks: []
created: 2026-10-14
---

# Server instructions carry the Skill Hub protocol

## Outcome, constraints, non-goals

- **Outcome.** The protocol an agent needs (when to call `skill_resolve`, how to use `local.path`, `local.env`,
  preflight, `snapshot_expired`, `review_required`, feedback) reaches the agent only through the MCP `instructions`
  field of the initialize (and `discover`) result. The text is built at startup from the tools the server actually
  registers for its profile, so it never names a tool the server does not serve. No Skill Hub instruction text is kept
  in `CLAUDE.md`, `AGENTS.md` or `GEMINI.md`.
- **Decisions (user, 2026-10-14).**
  - Server `instructions` are the only channel for the protocol. There is no file block and no short pointer, not even
    for clients that ignore server instructions. That gap is accepted and has no fallback (see Risks).
  - `skillhub connect` stops writing the bootstrap block. `doctor` reports an existing block as obsolete, and
    `doctor --fix` removes only the marked block. The rest of the file stays byte-identical. The file is deleted only
    when nothing but whitespace is left, which is the code's existing ownership rule (see phase 2).
  - Modern agents are smart enough, so the text is kept short (it must fit Claude Code's 2048-character cap, see
    Risks).
- **Constraints.**
  - Implementation runs in its own git worktree. Do not edit the main checkout.
  - Tests must never read the live hub (`/home/vantt/skill-hub`), the real `HOME`, or real user projects, and never
    write to them. They use `t.TempDir()`, an isolated `HOME`, isolated XDG dirs and `SKILLHUB_USER_CONFIG_DIR`. The
    `hostintegration` package already isolates `HOME` in `TestMain` (`internal/hostintegration/main_test.go:13`).
  - Do not edit `~/.claude/CLAUDE.md` or the live hub. The user runs `skillhub doctor --fix` after release (see
    phase 4, manual steps).
  - No plan IDs or phase numbers in code comments, test names or commit messages. Use conventional commits.
  - Public contract kept: the change kind `bootstrap-instructions` and the item ID `host_<host>_bootstrap-instructions`
    (`internal/hostintegration/types.go:49`) stay, so JSON consumers and existing tests keep their IDs.
- **Non-goals.** Per-project tuning of instructions. New commands. Changes to `skill_resolve` behaviour. Telemetry
  additions. Any other distill lesson.

## Design in one page

```text
mcp serve --profile P                    skillhub mcp proxy --hub U
  registry := collector                    tools := runtimeToolDefinitions()   (same collector)
  adapter.registerTools(registry)          instr := serverInstructions(names(tools), {extension:false, curator:false})
  instr := serverInstructions(             mcp.NewServer(..., Instructions: instr)
     registry.names(),                     (the proxy never relays the hub's initialize: it is its own
     {extension: P != curation,             MCP server and dials the hub lazily on the first tool call,
      curator: true})                       proxy.go:224 and proxy.go:367)
  mcp.NewServer(..., Instructions: instr)
  registry.install(server)   <- same lazy AddTool placeholders as today, no second schema derivation

connect / doctor --fix (local)            instruction file planning (both connect and --remove)
  prepare() -> prepareInstructionFile     desired := removeBootstrap(raw)
  expectedChange(ChangeBootstrap)         no block: desired == raw  -> Current, nothing planned
                                          block:    splice it out   -> change "obsolete block"
                                          only whitespace left      -> delete the file
```

`serverInstructions` is a list of paragraphs. Each paragraph declares the tools it names (`requires`) and an optional
capability flag. A paragraph is rendered only when every tool it requires is registered. Phase 1 has the exact text.

## Phases

| # | Phase | Depends on | Effort | Files owned |
|---|-------|-----------|--------|-------------|
| 1 | [Server instructions from registered tools](phase-01-server-instructions.md) | none | 2.5h | `internal/delivery/mcpserver/{instructions.go,instructions_test.go,tool_registry.go,server.go,proxy.go}` |
| 2 | [Retire the instruction-file block](phase-02-retire-instruction-block.md) | 1 (ships together) | 3h | `internal/hostintegration/*`, `internal/app/{health.go,connect_remote.go,workspace_remote.go}`, `internal/delivery/cli/connect.go`, affected tests and goldens |
| 3 | [Client support research](phase-03-client-support-research.md) | none (runs in parallel with 1) | 0.5h | `plans/261014-1509-server-instructions-protocol/reports/` only |
| 4 | [Docs, repo AGENTS.md, release steps](phase-04-docs-and-release.md) | 1, 2, 3 | 1h | `README.md`, `README.vi.md`, `docs/user-guide.md`, `docs/remote-hub-guide.md`, `docs/design/02-agent-hub-protocol.md`, `docs/use-cases/01-agent-runtime-protocol.md`, `internal/delivery/cli/help.go`, `AGENTS.md` |

No two phases touch the same file. Phases 1 and 2 must be merged in the same release: phase 2 alone would remove the
protocol from agents before the server carries it.

## Test matrix

| Level | What | Where |
|-------|------|-------|
| Unit | For each profile (runtime, curation, all), and for every tool name registered by any profile, the name appears in the instructions if and only if the profile registers it | `mcpserver/instructions_test.go` |
| Unit | Every backticked tool name inside a paragraph is listed in that paragraph's `requires` (guards future edits) | `mcpserver/instructions_test.go` |
| Unit | The runtime and curation texts are ASCII and at most 2048 characters each | `mcpserver/instructions_test.go` |
| Integration (in-memory transport) | `initialize` from `New(..., ProfileRuntime)` and `New(..., ProfileCuration)` returns the generated text. The proxy's `initialize` returns the runtime text without `skills/get` and without `system-curator` | `mcpserver/instructions_test.go` |
| Unit | Planning an instruction file with no block gives no change. A v1 block (and any `vN` block) is planned for removal, with user text byte-identical. A file holding only the block is deleted. Malformed markers give a conflict and no write | `hostintegration/integration_test.go`, `remote_test.go` |
| Integration | `connect` on a fresh temp project writes no `CLAUDE.md`/`AGENTS.md`/`GEMINI.md`. `doctor` on a temp project with an old block reports it obsolete, and `doctor --fix` removes it | `app`, `delivery/cli` tests and regenerated goldens |
| E2E (manual, after release) | Claude Code session: `/mcp` shows skillhub connected, and the agent calls `skill_resolve` on a new task with no CLAUDE.md block | phase 4 manual steps |

Gate: narrowest package tests first, then `make check` (fmt-check, vet, lint, `go test -count=1 ./...`, `Makefile:60`).

## Risks (likelihood x impact)

| Risk | L x I | Mitigation |
|------|-------|------------|
| A client ignores server `instructions`, so the agent never learns to call `skill_resolve` (no fallback, by user decision) | M x H | Phase 3 records support per client. Docs say plainly that such a client gets tools without the protocol. This is listed as an open question, not solved. |
| Claude Code truncates instructions above 2048 characters (community reports, v2.1.84+) and truncates the combined text of many servers | H x M | The runtime text is about 1.9 KB ASCII and a test caps it at 2048. The curation text is kept short so that runtime and curation together stay small. |
| A double schema derivation at startup slows `mcp serve` (startup and memory benchmarks) | M x M | The registry collects once and installs later. No second `registerTools` pass. Run `startup_bench_test.go` and `memory_budget_test.go`. |
| `doctor --fix` in a hub folder deletes or edits a tracked `CLAUDE.md`, which shows up as uncommitted | M x L | Expected and visible: `hub_uncommitted` (`internal/app/health_hub.go:163`) prints the commit command. Documented in phase 4. |
| Remote (proxy) connections: `doctor --fix` does not rewrite them (`health_client.go:436`), so their old blocks stay until the user re-runs `connect --hub` | H x L | doctor still reports the block with the reconnect command. Documented in phase 4. |
| A user's own file that holds only the block plus whitespace is deleted | L x L | This is the existing ownership rule (`remote.go:505`). It matches what `connect --remove` already does. |

## Backwards compatibility and rollback

- Old blocks of any version (`skillhub:bootstrap:vN`) are recognised by the existing marker pattern
  (`internal/hostintegration/bootstrap.go:17`) and removed. Change kind and item IDs are unchanged.
- A hub committed with an init-written `CLAUDE.md` is still accepted by `init` on another machine, because a
  block-only file is recognised by its markers instead of by an exact text match (phase 2).
- Rollback: revert the phase 1 and phase 2 commits together. Users who already ran `doctor --fix` get the block back on
  the next `connect` or `doctor --fix` of the reverted binary, so no data is lost.

## Success criteria

- `initialize` of `skillhub mcp serve --profile runtime` returns the runtime text, and the curation profile returns
  only the curation text. Neither names an unregistered tool. Tests prove it.
- On a temp project, `skillhub connect` creates none of the three instruction files. `skillhub doctor` on an old
  install shows one problem naming the obsolete block, and `doctor --fix` leaves the user's own text byte-identical.
- `rg -n "skillhub:bootstrap" docs README*.md internal/delivery/cli/help.go AGENTS.md` returns no protocol text. Only
  the removal behaviour is described.
- `make check` passes.

## Open questions

1. **Clients without instructions support.** Phase 3 may find that Codex, Cursor or Gemini CLI ignore `instructions`.
   The user chose to have no fallback. Should the docs then name those clients as "tools only, no protocol", or should
   the lead revisit that decision with the evidence?
2. **The HTTP listener (`http.go:93`)** serves the runtime profile with the curator sentence, but a direct HTTP client
   on another machine has no `system-curator` skill. Supported remote connections go through the proxy, which leaves
   that sentence out. Is it acceptable to leave the listener as is (recommended), or should the remote flag be threaded
   into `startServing`?
3. **The repo's own `AGENTS.md`** (lines 62-86) holds a v1 block. Phase 4 removes it as repo content. Confirm, or let
   the user remove it with `doctor --fix` and commit.
4. **Distill lesson status.** Should `docs/distillery/lessons/skill-over-mcp.yaml` key
   `server-instructions-carry-the-protocol` be marked adopted? This plan leaves it alone (non-goal: other distill
   lessons).
