# Delivery layer cleanup: cook report (2026-10-12)

Scope: `internal/delivery/mcpserver` (plus one new file in `internal/app`). Source: the review `review-261012-skills-over-mcp-conformance.md`, section 4 and the startup hotspot.

## Commits (in order)

| Commit | What |
|---|---|
| `test(mcpserver): pin the serialized tools/list per profile and benchmark startup` | Digest guard for `tools/list` of `all`, `runtime`, `curation`, proxy equals runtime profile; `BenchmarkServerStartup`, `BenchmarkProxyRuntimeToolDefinitions` |
| `refactor(mcpserver): assemble load cases in app and share session and review helpers` | `app.RecordLoadCase` / `app.LoadResolution` (new file `internal/app/skill_load_case.go`); `recordLoad` flattened; `toolSession`, `reviewBlock` replace the repeated session and review lines |
| `refactor(mcpserver): walk schemas once, take per-tool schema options ...` | one `walkSchema`; `addTool` options replace the tool-name special cases; `requirePins` and `confirmSkillMutation`; error helpers; dead code removed |
| `perf(mcpserver): register typed tools on first call ...` | startup change, numbers below |
| `refactor(mcpserver): split server.go by concern` | `server.go` 1,018 lines to 200 |
| `fix(mcpserver): answer skills/get for the curator on native sessions` | with test |

## 1. Delivery cleanup

- Telemetry case assembly moved to `app` (`RecordLoadCase`); `mcpserver` only passes the load and the remembered resolution. Order of the two events (case, then `skill.loaded`) is unchanged.
- Session extraction: `toolSession(req)` for tool calls; in `readResource` the dead nil checks are gone (`request.Session` directly, the request was already dereferenced).
- `walkSchema` replaces the five traversal copies (`closeObjectSchemas`, `normalizeMultiTypeSchemas`, `shrinkOutputSchema`, `applyCommonConstraints`, `replacePropertySchema`); the old names stay as thin wrappers because tests call them. One difference by construction: `applyCommonConstraints` now also visits `anyOf` children (it did not before); the serialized tool lists are byte-identical for all three profiles, so no derived schema is affected.
- `addTool` has no tool-name switches: `withInputSchema`, `withPropertyEnums`, `withCommittedResolverSchemas` are passed at the registration sites (`curation_session_record`, `skill_feedback`, `source_watch_preview`, `skill_resolve`, `routing_evaluate`).
- Error mapping: the two tool-error paths (`safeToolError` for returned errors, `resultToolError` for errors embedded in a result) are kept apart on purpose because their retry rules differ (the first limits retryable to stale_context, snapshot_expired, source_unavailable, index_stale for `*app.Error`; the second passes the application's flag through; internal messages and suggested actions differ). They now share one `logInternalFailure` (correlation ID and log line), the `_ = errorEnvelope{}` placeholder and `applicationError` are gone, and the code carries comments stating the rule. Observable output is unchanged; `error_characterization_test.go` passes unchanged. The distribution JSON-RPC errors share `correlatedInternalError`.
- Confirmation pins: `confirmationInput.requirePins()` and `confirmSkillMutation` replace five copies (skill create/transition/update confirm, source watch confirm, skill add confirm). One tiny widening: pins are now trimmed in the three skill confirm tools, as the source-watch and skill-add confirm tools already did (before, padded pins passed the blank check but failed later).
- Dead code removed: `registerSkillTools`, `unavailable`, `errorEnvelope`, and the unused DTOs `sourceIntakeAddInput`, `sourceIntakeListInput`, `sourceTriageInput`, `sourceLinkPreviewInput`, `sourceUnwatchPreviewInput`, `outcomeRecordInput`, `sourceTriageResult`, `sourceListItem`. `golangci-lint` full run on `mcpserver` reports no `unused` any more.
- Not done (outside my packages): dead functions in `catalog` and `app` (`openPinnedGeneration`, `isCanonicalEntityPath`, `splitSkillFrontmatter`, `safeSourcePath`, the `OpenServable*` aliases) and the "curator or buildDistributedSkill" branch repeated in `app/distribution.go`; the parallel agent owns those packages. The `catalog/build.go` shadowed `err` bug and `BenchmarkMCPToolList` (asserts a stale tool count) are untouched.

## 2. Startup

Why the SDK was the cost: `mcp.AddTool` resolves the input and output schema of every typed tool at registration (77% of `New` in the CPU profile). Caching derived schemas does not help a real process start (it starts once), so the change is lazy resolution.

- `toolRegistry` (tool_registry.go): at start each tool is listed from its derived schema with a placeholder handler; the typed `mcp.AddTool` (resolution, input validation, output validation) runs when the first `tools/call` for that tool arrives, through a receiving middleware and a per-tool `sync.Once`. Validation and result handling are the SDK's own code, unchanged. The server does not advertise list-changed, so no notification is sent when a tool is swapped in.
- Guards: digest test (listing before any call and after registering every tool in full is byte-identical), a test that registers every tool of every profile in full (a schema that cannot be resolved fails a test instead of the first user call), concurrent first calls, and validation of a malformed first call.
- The committed resolver schemas are decoded once per process and cloned per tool.
- Proxy: `runtimeToolDefinitions` no longer builds a server and an in-memory session; it derives the definitions and round-trips them through JSON so the proxy offers exactly the form a client decodes from a hub. Signature lost its unused `ctx`.
- Kept: the resolve check in `embedResolverResponse` runs at registration, so a dangling reference in the committed schema still fails at start.

Cold numbers (fresh process, first iteration, median of 21 runs, this machine, 24 threads):

| Case | Before | After |
|---|---|---|
| `New`, all profile | 24.98 ms, 16.27 MB | 8.38 ms, 8.45 MB |
| `New`, runtime profile | 5.34 ms, 3.27 MB | 3.02 ms, 2.37 MB |
| `New`, curation profile | 20.44 ms, 13.17 MB | 7.76 ms, 6.77 MB |
| Proxy `runtimeToolDefinitions` | 9.47 ms, 5.51 MB | 6.26 ms, 3.05 MB |

Warm numbers (`go test -bench`, benchtime 30x, same process, committed resolver schemas already decoded): all 24.0 ms / 16.1 MB to about 5.6-11 ms / 7.7 MB (noisy, other agents were running), runtime 5.1 ms / 3.1 MB to 1.8 ms / 1.6 MB, proxy 7.6 ms / 5.2 MB to about 4 ms / 2.3 MB.

Remaining startup cost: schema derivation by reflection (about 21%), the SDK's `AddTool` input check for the placeholder (about 22%), and the one-time decode of the committed schemas. A session pays the deferred resolution only for the tools it calls (typically `skill_resolve`, `skill_get`), on their first call.

## 3. Split and curator

- `server.go` (1,018 lines) is now: `server.go` (lifecycle, 200), `memory_releaser.go`, `session_telemetry.go` (client naming, tools/list size metric, caller context), `load_attribution.go`, `skills_extension.go`, `rpc_errors.go`, `tool_registration.go`, `tool_registry.go`, `schema_shaping.go`, `tool_errors.go`, `tool_confirmation.go`. No exported name changed.
- `skills/get` now answers for the curator on native sessions; the curator stays out of `skills/list`. Test: `TestSkillsGetServesTheCuratorToNativeSessionsThatDoNotListIt`. The older per-client test that expected `skill_not_found` was changed to the new expectation.

## Verification

gofmt clean; `go vet ./...` (linux, windows, darwin) clean; `make lint` 0 issues (against the base commit 6688bf3, funlen 120); `go test -count=1 ./...` and `go test -race` on `mcpserver` and `app` (see the status line).
