# Load-path speed work: results

Date: 2026-10-12. Branch `worktree-agent-a2a91d42b18467219`, from main `de31b9f`. Nothing pushed or tagged.

## What changed

| Area | Change |
|---|---|
| `skill_get` runtime path | New `app.SkillCore` and `SkillService.ReadSkillCore`: one catalog open (shared lock held), one query for the skill row plus its manifest document, one resource query, `SKILL.md` read once and verified against its pinned digest. Routing and rationale come from the catalog's copy of the metadata through `skill.RoutingFromDocument` and `RationaleFromDocument`, which `ReadRouting` and `ReadRationale` now share. No `AssessSkillState`, no `canonical.Validate`, no git. |
| Curation model | `GetSkillDetail` is now `ReadSkillCore` plus `AssessSkillState`. `SkillDetail` keeps every field, so the CLI, WebUI and web golden files are unchanged. |
| Dropped from MCP `skill_get` and the hub bundle detail | `diverged`, `changed_resources`, `missing_resources` (assessment output), `state_basis` (constant `canonical`, describes how curation judged state) and `routing_eligible` (needs the served-state assessment; at runtime `status`/`lifecycle_state` plus `local.status` answer it). |
| Proxy cache | The proxy decodes the cached detail into `SkillCore`. An entry written by an older hub, with the dropped fields, still loads and the answer omits them (test below). A new hub's bundle read by an older proxy just leaves those fields zero. |
| Telemetry enrichment | `recordCurationTelemetry` no longer opens the catalog per event. `catalog.CurrentPointer` reads the published pointer; snapshot and policy revision are cached per generation and the catalog is opened only the first time a generation is seen (not cached while serving a fallback generation). `tools_list_bytes` is encoded once per tool set (keyed by the registered tool objects) instead of on every `tools/list`. Event types, fields and `EventVersion` unchanged. |
| Distribution build | `buildDistributedSkillKeeping` parses frontmatter once (`catalog.ParseServableSkill`), maps relative to internal paths with one map, and returns the verified bytes of requested files. `ReadResource` serves those bytes; `loadDistributedSkillSource` (snapshot export and bundle) reuses them instead of re-reading every file. |
| Resolve | Policy is loaded once per resolve (`service.Policy` is set after the first load). |
| Host matrix | `HostSupportsSkillsExtension` decodes the matrix once with `sync.OnceValues`. One hostintegration test that swaps `matrixData` now resets the cache. |

## Measurements

Scratch-free: `BenchmarkLoadPaths` (new, `internal/delivery/mcpserver/load_path_bench_test.go`). Herdr-shaped fixture: entrypoint plus 12 reference files, about 190 KB, telemetry recorder attached, in-memory MCP session, workspace files backdated so the integrity cache applies. Before is `de31b9f` plus the same benchmark file, run in a separate worktree, alternating with the after runs. The machine was shared with other agents, so read ranges, not decimals. `-benchtime 300x -count 5`.

| Request | Before | After |
|---|---|---|
| `skill_get` warm | 25.7-39.9 ms, 3.62 MB, 15.6k allocs | **3.5-3.9 ms**, 1.42 MB, 7.4k allocs |
| `skill_get` first call of a session (fresh server) | 16.1-18.6 ms, 4.0 MB, 16.3k allocs | **4.2-4.5 ms**, 1.91 MB, 8.1-8.3k allocs |
| `resources/read`, server handler, with telemetry | 4.6-5.1 ms, 1.05 MB, 6.45k allocs | **2.1-2.8 ms**, 0.69 MB, 4.2k allocs |
| `skills/get` | 3.8-4.7 ms, 1.45 MB, 6.8k allocs | **2.4-2.5 ms**, 1.10 MB, 4.6k allocs |
| `skill_resolve` over MCP (2 skills) | 2.1-2.6 ms, 0.66 MB, 2.9k allocs | **1.1-1.2 ms**, 0.65 MB, 2.87k allocs |

`BenchmarkSkillResolve` in process: 32 skills 2.8-3.6 ms before, 2.1-2.3 ms after; 300 skills 12.6-13.3 ms before, 12.2 ms at best after (the 300-skill runs after were disturbed by other load, 16-23 ms). Resolve time at 300 skills is dominated by decoding every skill's `content_json`, which was not in scope.

Process and syscall counts for 100 warm `skill_get` calls, `strace -f -e trace=execve,openat`, setup subtracted:

| | Before | After |
|---|---|---|
| git processes per call | 7 (700 per 100) | **0** (25 total, all in test setup) |
| `openat` per call | about 1,580 | about 520 |

Target met: warm `skill_get` went from 25-40 ms (15-33 ms in the review) to under 4 ms with no git spawns.

What is left in the warm 3.5 ms: two catalog opens (the core and `SnapshotService.Ensure`, which exports the snapshot and opens its own generation), each with a SQLite open, schema parse and a tree lstat. Merging them into one open needs `Ensure` to run with the shared lock held while it writes the snapshot, which `loadDistributedSkillSource` deliberately avoids. Removing the opens altogether is the per-generation memo, left out of scope.

## Equivalence

- Empirical: a scratch test (not committed) dumped the raw MCP `skill_get` JSON, including `isError`, for 11 scenarios on the old and the new code: active, active with rationale, draft, deprecated and archived each as local and as unapproved third-party (review_required gating, content withheld), the curator id (not a workspace skill, so not found), an unknown id and an invalid id. After removing the five dropped fields from the old output, all 11 were byte-equal in JSON value, including `local`, `content_digest`, `catalog_snapshot`, resources and routing.
- Committed: `TestReadSkillCoreMatchesTheSeparateReads` compares the core against the old separate reads (`ReadSkill`, `ReadSkillRouting`, `ReadSkillRationale`) for active, plain, draft and archived skills, with routing examples and a rationale, and checks `GetSkillDetail` agrees. `TestReadSkillCoreReportsMissingAndInvalidSkills` covers missing, curator and invalid ids against the old reads. The existing content-gate and distribution tests pass with only a field rename (`SkillDetail` to `SkillCore`) and the removal of one `state_basis` assertion.
- `TestSkillGetFollowsAnEditAfterRebuild`: edit `SKILL.md`, rebuild, next `skill_get` returns the new content, `content_digest`, `catalog_snapshot`, a new `local.path` whose folder name carries the new manifest digest, and the file on disk matches. It also asserts the five dropped fields are absent from the raw answer.
- `TestProxyServesCachedDetailFromAnOlderHub`: a cached detail with the dropped fields loads; the answer omits them.
- `TestTelemetryPinsFollowThePublishedGeneration`: an unpinned event carries the new snapshot after a rebuild. `TestToolsListBytesFollowsTheToolSet` covers the size cache.

## Checks

`gofmt -l .` clean; `go vet ./...` clean for linux, `GOOS=windows` and `GOOS=darwin`; `make lint` 0 issues; `go test -count=1 ./...` all packages pass; `go test -race` passes for app, mcpserver, hostintegration, catalog, skill.

## Notes for the caller

- Public output change: MCP `skill_get` and the bundle detail lose five fields (listed above). No JSON schema file under `schemas/` describes `skill_get`; the tool's output schema is derived from the Go type and follows automatically. Docs updated: `docs/use-cases/01-agent-runtime-protocol.md`, `docs/design/01-system-architecture.md`. `docs/use-cases/04-webui-user-flows-and-screen-specs.md` already lists only kept fields.
- `routing` and `rationale` in `skill_get` now come from the catalog generation, not from a fresh read of the canonical YAML. They are the same values whenever the catalog is current, which `OpenWithFallbackLocked` guarantees; the content is digest-verified against the file either way.
- Files owned by the parallel work were not edited: `catalog/build.go`, `integrity_cache.go`, `open.go`, `mcpserver` `registerSkills` and `skills/list`, `resolver_tools.go`. The edits to `server.go` are limited to `telemetryMiddleware`, the new `toolsListBytes` method and three struct fields; `catalog/catalog.go` gained `CurrentPointer`; `catalog/servable.go` gained `ParseServableSkill`.
- The review's 5-second integrity window (`quick_check` rerun after every rebuild) still applies and is the other agent's item. My benchmark backdates file times so it measures steady state; inside the window `skill_get` costs about 3 ms more per call.
- `BenchmarkMCPToolList` is still broken (asserts 33 tools); left for the cleanup item.
