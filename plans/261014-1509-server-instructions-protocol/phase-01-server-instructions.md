# Phase 1: Server instructions from registered tools

## Context

- `mcp.NewServer` is called at `internal/delivery/mcpserver/server.go:96` with no `Instructions`. Tools are registered
  after the server exists (`server.go:111-112`). go-sdk v1.8.0 copies `ServerOptions` at `NewServer`
  (`mcp/server.go:215-217`) and returns `s.opts.Instructions` in both `initialize` (`mcp/server.go:2105`) and the
  newer `discover` (`mcp/server.go:956`). The text therefore has to be known before `NewServer` is called.
- The profile picks the tool set in `tool_registration.go:7-38`: runtime is `skill_resolve`, `skill_feedback`
  (`resolver_tools.go:12`) and `skill_get` (`skill_tools.go:152`). Curation is the source, upstream, insight, skill
  curation, add, review and workspace tools. All is both sets.
- `toolRegistry` (`tool_registry.go:24-46`) has two modes. With a server, `add` installs a placeholder `AddTool` and
  keeps the typed registration lazy. As a collector (`newToolCollector`, used by the proxy at `proxy.go:358`), it only
  records definitions. `addTool` derives schemas before calling `add` (`tool_registration.go:40-46`), so running
  `registerTools` twice would derive every schema twice at startup.
- The proxy is its own MCP server (`proxy.go:224`). It is built from `runtimeToolDefinitions()` (`proxy.go:356-376`)
  and dials the hub lazily on the first tool call (`proxy.go:378`). It never relays the hub's `initialize`, so "pass
  through" means it generates the same runtime text from the same tool definitions. It advertises no skills extension
  (`proxy.go:225`), and remote connect installs no `system-curator` skill (`internal/delivery/cli/help.go:99-101`).

## Requirements

1. `serverInstructions(names []string, options instructionOptions) string` lives in a new
   `internal/delivery/mcpserver/instructions.go`. It is pure and deterministic.
   `instructionOptions{skillsExtension, curatorSkill bool}`.
2. The text is built from an ordered list of paragraphs `{text string; requires []string; flag func(options) bool}`.
   A paragraph is rendered only when all `requires` are in `names` and its flag (if any) holds. If no paragraph
   renders, the result is `""`.
3. `New` collects tools once, builds the text, creates the server with `Instructions`, then installs the collected
   tools. Startup cost and the lazy registration are unchanged.
4. The proxy sets `Instructions: serverInstructions(names(tools), instructionOptions{})`.
5. ProfileAll renders the runtime paragraphs and then the curation paragraph. `connect` never registers ProfileAll,
   so the 2048 cap applies only to runtime and curation.

## Exact instructions text

Runtime paragraphs, in order. Each item gives its `requires` and flag, then the text. Paragraphs are joined with
`"\n"`, and the header is followed by a blank line.

| # | requires / flag | text |
|---|-----------------|------|
| R0 | `skill_resolve` | `Skill Hub picks the best skill in the user's hub for a task.` + blank line |
| R1 | `skill_resolve` | ``- At the start of each new task, or when its operation, scope or constraints change, call `skill_resolve` before choosing a skill; skip trivial edits. Send `task.description` in English. Follow at most one primary procedure.`` |
| R2 | `skill_resolve`, `skill_feedback` | ``- If you do not use the recommended skill, call `skill_resolve` again with `prior.kind: rejected`. If you use another skill, send `skill_feedback` with its real `skill_id`.`` |
| R3 | `skill_get` | ``- When a response has `local.path`, resolve the skill's relative files (such as `scripts/`) against it, and export `local.env` (`SKILLHUB_SKILL_DIR`, `SKILLHUB_STATE_DIR`, `SKILLHUB_CONFIG_DIR`) when you run its `check`, `setup` or scripts. If `local.path` is gone, call `skill_get` again.`` |
| R4 | `skill_get` | ``- On `snapshot_expired` with `current_uri`, call `skill_get` again.`` and, only when `skillsExtension`, append `` A host with the MCP skills extension reloads with `skills/get <current_uri>`.`` |
| R5 | `skill_resolve` | ``- If `$SKILLHUB_CONFIG_DIR/env` exists, load it before `check`, `setup` or scripts and never print its values. If a variable is missing, tell the user to run `skillhub skill env set <id> <NAME>`; never ask for it in chat.`` |
| R6 | `skill_resolve` | ``- With `local.preflight`, run `check` in `working_directory` first and ask the user before `setup`. Treat install steps written in prose as `setup`: ask first, install into `SKILLHUB_STATE_DIR`, never globally. Do not run `setup` while `$SKILLHUB_STATE_DIR/.setup.lock` is recent.`` |
| R7 | `skill_resolve` | ``- If `local.status` is `review_required`, do not use the skill; tell the user to run `skillhub skill review <id>`.`` |
| R8 | `skill_feedback` | ``- If a script fails on a missing dependency, stop, tell the user, and send `skill_feedback` with `outcome: failed` and `reason_code: setup_failed`.`` |
| R9 | `skill_resolve`, flag `curatorSkill` | ``- Only when the user explicitly asks to manage, curate, check, distill, repair or inspect Skill Hub itself, load the native `system-curator` skill.`` |
| R10 | `skill_resolve` | `- These are recommendations; your environment controls permissions and execution.` |

Curation paragraph (C0 requires nothing, but renders only when any curation tool is registered, that is any name other
than the three runtime tools):

```text
Skill Hub curation tools change the user's hub. Use them only when the user explicitly asks to manage, curate, check,
distill, repair or inspect Skill Hub, and follow the native `system-curator` skill.
```

Then C1 (`hub_status`): `` Start with `hub_status`.`` Then C2 (any registered name ending in `_preview`):
`` A `_preview` tool shows a change; apply it with the matching `_confirm` tool only after the user agrees.``

Measured size of the local runtime text (R0-R10, both flags on): about 1,890 bytes, all ASCII. Use `-` and `...`, not
`…`, because Claude Code counts UTF-16 units.

Option sets:

- `New`: `skillsExtension = profile != ProfileCuration` (the same condition as `servesSkills`, `server.go:91`), and
  `curatorSkill = true`.
- Proxy: both false.

## Files

- Create `internal/delivery/mcpserver/instructions.go` (generator) and `internal/delivery/mcpserver/instructions_test.go`.
- Modify `internal/delivery/mcpserver/tool_registry.go`. The collector keeps `{tool, register}` pairs. Add
  `names() []string` and `install(server *mcp.Server)`. `install` does what `newToolRegistry` and `add` do today:
  register the `registerOnFirstCall` middleware, set the pending map, and add the placeholder `AddTool`. Remove
  `newToolRegistry`, which has one caller at `server.go:111`. `definitions()` keeps working for the proxy.
- Modify `internal/delivery/mcpserver/server.go:83-113`. Build `registry := newToolCollector()`, call
  `adapter.registerTools(registry)`, compute the text, then `mcp.NewServer(..., Instructions: text)`. Keep the
  existing order: telemetry middleware, `registerSkills`, then `registry.install(server)`, then
  `adapter.tools = registry`. Middleware order matters for telemetry, so keep it.
- Modify `internal/delivery/mcpserver/proxy.go:220-227` to add `Instructions`.

## Steps

1. Write the tests first (below). They fail now because `Instructions` is empty.
2. Add the two-phase registry. Run `go test ./internal/delivery/mcpserver -run 'ToolRegistry|ToolsList|FirstCall' -count=1`.
3. Add the generator and wire `New`.
4. Wire the proxy.
5. Run the startup and memory benchmarks for regressions:
   `go test ./internal/delivery/mcpserver -run 'Startup|MemoryBudget' -count=1`, plus the perf-gated run
   `SKILLHUB_PERF=1 go test -p 1 -count=1 -run 'Performance' ./internal/delivery/mcpserver`.

## Tests

- `TestInstructionsNameOnlyRegisteredTools`. Collect `allNames` from a ProfileAll collector. For each of runtime,
  curation and all, build the profile's names and the text. For each name in `allNames`, assert that
  `strings.Contains(text, "`"+name+"`")` equals "registered".
- `TestInstructionParagraphsDeclareTheToolsTheyName`. For each paragraph, every backticked token that is in `allNames`
  must be in `requires`.
- `TestInstructionsFitClientLimit`. Runtime (both option sets) and curation are ASCII and `len <= 2048`.
- `TestInitializeCarriesInstructions`. In a temp workspace (the existing server test helpers make one), call
  `New(ws, io.Discard, ProfileRuntime)`, connect a client over `mcp.NewInMemoryTransports()`, and assert that
  `session.InitializeResult().Instructions` equals `serverInstructions(...)`. Do the same for ProfileCuration and
  assert there is no `skill_resolve`.
- `TestProxyInitializeCarriesRuntimeInstructions`. Build the proxy against an `httptest` hub as the existing
  `proxy_test.go` does. Assert that the instructions contain `skill_resolve` and contain neither `skills/get` nor
  `system-curator`, and that `initialize` itself sends no request to the hub.

Validation: `go test ./internal/delivery/mcpserver -count=1`, then `make check`.

## Risks

- **Changing the registry breaks the lazy-registration tests** (`tool_registry_test.go:31,52`). The behaviour is
  unchanged, only the build order differs. Run those tests first.
- **Text drifts from the tools.** The paragraph-declaration test catches a newly named tool.

## Rollback

Revert this phase's commit. Instructions go back to empty. This is safe only if phase 2 is reverted too.
