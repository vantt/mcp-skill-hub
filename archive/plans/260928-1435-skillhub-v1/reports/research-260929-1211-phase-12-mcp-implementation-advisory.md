# Phase 12 MCP implementation advisory

**Researched:** 2026-09-29 12:11 Asia/Saigon  
**Scope:** Read-only advisory against the Phase 12 plan, current design, current Go code/module, MCP 2026-07-28, SEP-2640, and released Go SDK/client tooling.

## Outcome

Implement Phase 12 on the official `github.com/modelcontextprotocol/go-sdk` pinned to **v1.8.0**, using stdio and the SDK's default multi-version negotiation. Target **MCP 2026-07-28**, but retain the SDK's legacy negotiation for `2025-11-25`, `2025-06-18`, `2025-03-26`, and `2024-11-05` so ordinary tool clients continue to work. Do not pin the server to 2026-only.

The released Go SDK has core extension capability maps and custom-method support, but **does not yet ship SEP-2640 Skills types/handlers**. Implement a small local SEP-2640 adapter with exact wire DTOs and `mcp.AddReceivingCustomMethod`; do not depend on the open, currently unmergeable Go SDK PR #1238 or copy its filesystem provider. Keep the adapter replaceable when an official released package arrives.

The current repository has no MCP dependency or delivery package yet. Its transport-neutral `internal/app` services and digest-pinned `internal/skill/resource.go` are the right architectural seam; MCP handlers must only validate/map/call those services.

## Pinned recommendations

### 1. Protocol and client compatibility

1. Set the product protocol baseline to **2026-07-28** and pin Go SDK **v1.8.0**. Go SDK v1.8.0 was released 2026-09-14, supports Go 1.25+, and negotiates all five revisions listed above; this project already declares Go 1.26.
2. Leave `ServerOptions.SupportedProtocolVersions` unset. The SDK will probe `server/discover` for modern clients and fall back to legacy `initialize` for old clients. A 2026-only restriction would needlessly break current host integrations and provides no Phase 12 benefit.
3. Maintain one behavior contract across protocol eras. Custom tools and resource reads behave identically; only lifecycle/capability envelopes differ. Do not use roots, sampling, logging capability, batching, SSE, or server-initiated requests.
4. Treat SEP-2640 as an opt-in distribution extension, not as the compatibility path for recommendation. `skill_resolve`, `skill_feedback`, and curation remain ordinary MCP tools, because Claude Code, Codex CLI, Gemini CLI, and most stock hosts are not listed as Skills clients.

### 2. Stdio lifecycle

- Start one server process per host connection. Run `server.Run(ctx, &mcp.StdioTransport{MaxLineLength: 4 << 20})`; use tighter per-tool byte/count limits. The SDK default is 16 MiB, but Phase 12 requests do not require that much inbound data.
- Write only newline-delimited JSON-RPC to stdout. Send logs/diagnostics to stderr, with no secrets, absolute paths, source bodies, or stack traces.
- Discover the configured workspace once, run `CatalogService.EnsureCatalog` at startup, then open/close immutable generation handles inside application calls. Do not retain hidden per-session recommendation or activation state.
- On stdin EOF, exit promptly. On SIGINT/SIGTERM, cancel the root context and wait for `Run` to stop. Treat expected EOF/signal cancellation as clean shutdown; report other transport errors on stderr and non-zero exit.
- Let concurrent calls run through existing application services and workspace locking. Test separate stdio processes against the same workspace; never serialize all reads behind an MCP-global mutex.
- Require explicit handles/IDs in multi-step tool arguments (`resolution_id`, `proposal_id`, digests, `base_version`, `event_id`). MCP connection/session state is not authority.

### 3. Skills extension and resources

Register server capabilities as `resources` plus:

```json
"extensions": {
  "io.modelcontextprotocol/skills": {"directoryRead": false}
}
```

Implement `skills/list` and `skills/get` with local types embedding `mcp.ParamsBase`/`mcp.ResultBase`. Register them with `mcp.AddReceivingCustomMethod`. Register a resource template/handler for `skill://...`; `resources/read` remains the core MCP method. Do **not** implement `resources/directory/read` in the first Phase 12 cut: the complete manifest already permits progressive reads, and directory reading is optional.

Use SEP-2640 shapes exactly:

- `skills/list`: optional opaque `cursor`; result has `resultType: "complete"`, atomic skill entries, optional `nextCursor`, and on 2026-07-28 `ttlMs` plus `cacheScope`.
- `skills/get`: parameter is **URI**, not internal skill ID or version; return the same complete entry shape.
- Each entry preserves **all** `SKILL.md` frontmatter as JSON and contains either a complete resource manifest or `"dynamic"`. This project should emit only static manifests.
- Each manifest file has URI, `sha256:<hex>` digest, and raw byte size. Include `SKILL.md`. Keep each skill at or below the interoperability baseline of 512 files/16 MiB.
- `resources/read` accepts only URI. Do not add nonstandard digest/snapshot parameters.

Resolve the design's snapshot-pinning requirement with immutable, per-skill-version URIs, for example:

```text
skill://skillhub/<manifest-digest>/<skill-name>/SKILL.md
skill://skillhub/<manifest-digest>/<skill-name>/references/x.md
```

The final parent segment still matches the Agent Skills name. Use a **skill manifest digest**, not the global catalog snapshot, so unrelated catalog changes do not change skill identity. `skill_resolve` returns that URI, manifest digest/version, and the catalog snapshot. `skills/get` and `resources/read` resolve the URI only if the pinned version is still available. With the current current-generation-only store, stale versions fail rather than silently serving new bytes.

This requires adapting the current `Manifest`: it presently exposes internal paths plus selected metadata, while SEP-2640 requires resource URIs and verbatim frontmatter. Keep internal paths private to the application layer.

### 4. Custom tools

Use Go typed inputs/outputs with `mcp.AddTool`; let the SDK generate and validate schemas, but add explicit JSON Schema constraints for lengths, counts, enums, patterns, and `additionalProperties: false` where silent mistakes are unsafe. Every successful structured tool result must include schema-matching `structuredContent` and text fallback; the Go SDK helper supplies both.

Use these annotation classes explicitly:

| Tool class | readOnly | destructive | idempotent | openWorld |
|---|---:|---:|---:|---:|
| `skill_resolve`, status/list/get/check/diff, pure evaluation | true | false | true | false |
| `skill_feedback`, `outcome_record` with deduplicated event ID | false | false | true | false |
| start/submit/retry/cancel and preview tools that persist runtime/proposal state | false | false | only when request/operation ID is enforced | false/true according to source access |
| confirm/apply/update/rebuild tools | false | truthfully set; apply/overwrite is destructive | only with proposal+digest+base pins and receipt replay | false |
| source/network tools | according to mutation | according to effect | according to keying | true |

Annotations are hints, never authorization. Preview tools are not read-only if they persist a proposal. Confirm tools must enforce `proposal_id + proposal_digest + base_version` and return the prior receipt on an exact replay.

Keep outputs bounded: compact summary, stable status/code, suggested actions, and paged items. For list/findings/diffs use default `limit=25`, maximum `100`, and an opaque cursor bound to catalog snapshot, normalized filters, and last sort key. Return `has_more` and `next_cursor`; return total only when already cheap. Never split a SEP-2640 skill manifest across pages.

### 5. Errors

- Tool/application failures: return `isError: true` with a safe actionable text block and stable structured fields such as `code`, `message`, `retryable`, `suggested_action`, and optional `correlation_id`. Validation/business/not-found errors are tool errors, not JSON-RPC errors.
- Valid outcomes (`no_skill`, `needs_context`, `already_covered`) are successful results, never `isError`.
- Protocol failures (unknown method/tool, malformed JSON-RPC, unsupported version) remain SDK JSON-RPC errors.
- SEP-2640 unknown skill/file or stale version must use JSON-RPC **-32602**; include `error.data.code: "snapshot_expired"` for the project's recovery rule. An actual canonical-byte/digest mismatch is an integrity/internal failure: return **-32603** with safe `error.data.code: "resource_digest_mismatch"`, serve no bytes, and log the correlation ID.
- Preserve `unsupported_schema`, `stale_context`, `index_stale`, `permission_denied`, and retry guidance from `docs/contracts/error-codes.md`; never collapse infrastructure failure into `no_skill`.

### 6. Capability negotiation

- Configure `ServerCapabilities.Extensions` with `io.modelcontextprotocol/skills`; resource registration advertises Resources. Modern 2026 requests carry capabilities per request; legacy requests carry them in `initialize`. Use the SDK-normalized request/session accessors rather than parsing `_meta` yourself.
- Clients must only call Skills methods after both sides declare the extension. The server may still return a normal error to an undeclared direct call; do not vary the skill/tool catalog by hidden session state.
- `directoryRead` is false until implemented and tested. Do not advertise roots, logging, sampling, or list-change support that the server does not provide.
- Do not invent a second MCP extension for resolver features. Keep `activation_context`, feedback support, schema major, and pinning support as optional fields in the `skill_resolve` tool contract; absent means unknown/unsupported, never false.

### 7. Verification matrix

1. **Go in-process contract tests:** official SDK client/server over `mcp.NewInMemoryTransports`; force 2026-07-28 and 2025-11-25. Cover discovery/initialize, capabilities, tools/list/call, skills list/get, resources/read, schema failures, pagination, cache fields, and all error mappings.
2. **Real stdio subprocess tests:** SDK `mcp.CommandTransport` against the built `skillhub` binary. Verify stdout purity, stderr isolation, EOF shutdown, cancellation, malformed frames, bounded input, and semantic parity with CLI services.
3. **Race/concurrency tests:** two or more subprocesses resolving/reading while one rebuilds or applies through the shared lock/generation model. Assert no mixed snapshot and exact `snapshot_expired` behavior.
4. **Official conformance:** pin `@modelcontextprotocol/conformance@0.1.16`; run the 2026-07-28 requirement set plus merged SEP-2640 enumeration, manifest, and directory-negative scenarios. Extensions are reported but not Tier-1-scored, so make their failures CI-fatal separately.
5. **Inspector:** pin `@modelcontextprotocol/inspector@2.8.0`; run CLI `skills/list --verify`, `skills/get --verify`, `resources/read`, `tools/list --strict`, and representative tool calls over stdio.
6. **Stock-client smoke tests:** test current stable Claude Code, Codex CLI, and Gemini CLI for stdio registration, tool discovery/calls, approvals, restart, and output limits. Do not claim native Skills support for them. For native Skills interoperability, test Inspector 2.8.0 and `mcpc`; optionally test fast-agent. The official matrix currently lists mcpc as full, Inspector/fast-agent/ChatGPT as partial, and does not list Claude Code, Codex, or Gemini.

## Ranked options and trade-offs

| Rank | Option | Compatibility | Complexity | Maintenance | Adoption risk | Architectural fit |
|---:|---|---|---|---|---|---|
| 1 | Go SDK v1.8.0 + small local SEP-2640 adapter | Best: modern and legacy core; exact released Skills wire | Moderate | Adapter removable after official release | Medium only for Skills adapter; low for core | Best match for Go, stdio, app-service boundaries, and pinned resources |
| 2 | Go SDK v1.8.0 + wait for official Skills package | Core works, Phase 12 distribution incomplete | Low now | Lowest later | High schedule/acceptance risk | Fails explicit Phase 12 deliverables |
| 3 | Pin open SDK PR/pseudo-version | Good test coverage but unreleased | Low initial | High; rebases/API drift | High: PR is open and currently unmergeable | Poor reproducibility |
| 4 | Hand-roll JSON-RPC/MCP or use a third-party SDK | Uncertain 2026 compatibility | High | Highest | High breaking/conformance risk | Duplicates lifecycle, schemas, transport, and errors |

## Source credibility

- **Highest:** final MCP 2026-07-28 specification and stdio lifecycle; final SEP-2640 and stable ext-skills specification. These define wire requirements.
- **High:** official Go SDK v1.8.0 source, release notes, and README; official conformance framework and Inspector 2.8.0. These define released implementation behavior and executable checks.
- **Medium-high:** official client extension matrix and ext-skills implementation list. They are community-maintained and can lag releases, so use them as compatibility evidence, not a guarantee.
- **Project authority:** Phase 12, `docs/design/02-agent-hub-protocol.md`, current app/resource code, and error contracts define local invariants, but they do not override official wire shapes.

## Known uncertainties

1. Go SDK Skills PR #1238 is open, unmergeable as of this review, and has no release target. The local adapter API should stay internal so migration is cheap.
2. The official matrix does not establish native Skills support for Claude Code, Codex CLI, or Gemini CLI. Their actual stable-version behavior must be recorded by smoke tests, not assumed.
3. The repository currently retains only the current catalog generation. Exact stale-version reads are therefore impossible; the advised behavior is deterministic `snapshot_expired`. Retaining historical immutable generations would be a separate storage decision.
4. SEP-2640 makes skill identity `(server identity, URI)`. Versioned URIs intentionally change identity when that skill's manifest changes, which strengthens project pinning but causes fresh host approval. This is the required trade-off unless the storage layer retains old bytes or the project relaxes server-side stale detection to host-side digest verification.
5. No external performance/load benchmark was run; payload limits and page sizes are conservative starting pins and should be adjusted only from measurements.

## Sources

- Local: `plans/260928-1435-skillhub-v1/phase-12-mcp-protocol-skill-distribution.md`
- Local: `docs/design/02-agent-hub-protocol.md`; `docs/contracts/error-codes.md`
- Local: `.pi/skills/ak-mcp-builder/SKILL.md` and its protocol/tool/evaluation references
- MCP 2026-07-28: https://modelcontextprotocol.io/specification/2026-07-28
- Stdio: https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/stdio
- Skills stable spec: https://github.com/modelcontextprotocol/ext-skills/blob/main/specification/stable/skills.mdx
- Skills overview/matrix: https://modelcontextprotocol.io/extensions/skills/overview and https://modelcontextprotocol.io/extensions/client-matrix
- Go SDK v1.8.0: https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0
- Go SDK Skills status: https://github.com/modelcontextprotocol/go-sdk/pull/1238
- Conformance: https://github.com/modelcontextprotocol/conformance
- Inspector 2.8.0: https://github.com/modelcontextprotocol/inspector/releases/tag/2.8.0

**Status:** DONE_WITH_CONCERNS  
**Summary:** Pin official Go SDK v1.8.0, target MCP 2026-07-28 while retaining legacy negotiation, and implement SEP-2640 through a narrow local custom-method adapter until official Go support is released. Use versioned skill-manifest URIs to reconcile stateless `resources/read` with the project's no-silent-upgrade invariant.  
**Concerns:** Native Skills client coverage is still sparse, the official Go Skills package is unreleased, and current storage can reject but cannot serve expired snapshots.
