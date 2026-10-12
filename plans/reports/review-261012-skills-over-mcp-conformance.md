# Review: Skills over MCP conformance, load-path speed and the MCP delivery layer (2026-10-12)

Read-only review of `main` at `494fd3c`, done 2026-10-12 around 08:00 Asia/Saigon. No tracked file was changed and nothing was committed, pushed or tagged. Nothing was written to `/home/vantt/skill-hub`. The only reads there were file counts and sizes.

All measurements ran with `HOME`, `XDG_*`, `TMPDIR` and `SKILLHUB_USER_CONFIG_DIR` pointed at temp directories. The machine is an Intel i7-13700F (24 threads, 31 GB RAM) on Linux with Go 1.26.9.

Two benchmarks I wrote for this review exist only in a scratch copy of the tree (`git archive HEAD`), not in the repo:

- `BenchmarkReviewLoadPaths` covers warm, in-process `skill_resolve`, `skill_get`, `skills/get`, `skills/list` and `resources/read`, on fixture skills shaped like test-audit and herdr-cook-plan.
- `BenchmarkReviewStartup` covers `New` and `startServing`.

## Summary

1. **The standard is final, and this server is mostly conformant.** SEP-2640 "Skills Extension" is Final: it merged on 2026-09-13. The server uses the right extension key, settings, methods, result shapes, digest and size fields, file limits, error code and caching fields. Its version-pinned URIs are allowed by the spec.
2. **There are four real deviations:**
   - `skills/bundle` sits in the extension's namespace. The spec explicitly deferred archive bundles.
   - The curation profile also advertises and serves the skills extension. A host with both servers connected sees every skill twice.
   - `skills/get` entries carry a top-level vendor field, `local`.
   - The remote proxy exposes no skills extension at all.
3. **`skill_get` is the slowest load path by far, and it is the path Claude Code uses today.** A warm `skill_get` takes 15 to 33 ms and allocates 4.8 MB. Each call spawns about 7 `git` processes, makes about 2,000 `openat` calls and opens the catalog 4 times. `skills/get` takes about 3 ms. The cause is that the runtime tool calls the curation read model `GetSkillDetail`, which runs `AssessSkillState` → `canonical.Validate`, and that spawns `git`.
4. **Every load request re-does work on immutable data.** Each one re-opens the catalog generation (a lock, a pin file write and two SQLite opens that parse the schema) and re-hashes every file of the skill several times. A single `resources/read` hashes the whole skill about 3 times. Caching per generation would make `skills/list`, `skills/get` and `resources/read` several times faster without changing any contract.
5. **Telemetry opens the catalog on the request path for some events.** Skill loads without a prior resolution, and server metrics (including `tools/list`), have no snapshot or policy attached, so recording them opens the catalog. This adds about 1 ms to every unsolicited `resources/read`. The event types and `EventVersion` can stay as they are.
6. **There is a real but small bug in catalog pointer publishing.** In `internal/catalog/build.go:505-506`, a shadowed `err` drops the `Write` error. If `Chmod` fails, the code skips the write and still publishes the pointer file.

## 1. The standard: Skills over MCP, as of 2026-10-12

Sources are primary unless noted: SEP text, PR metadata, release notes and repository files.

| Item | Status | Date | Source |
|---|---|---|---|
| SEP-2640 "Skills Extension" (Extensions Track, id `io.modelcontextprotocol/skills`) | **Final**, merged | created 2026-04-23, merged 2026-09-13 | https://github.com/modelcontextprotocol/modelcontextprotocol/pull/2640 ; `seps/2640-skills-extension.md` |
| SEP-2133 Extensions framework (`capabilities.extensions`) | Final | 2026-01-26 | PR #2133 |
| SEP-2549 TTL for list results (`ttlMs`, `cacheScope`) | Final | 2026-05-15 | PR #2549 |
| SEP-2575 Stateless MCP (`server/discover`) | Final | 2026-05-11 | PR #2575 |
| SEP-2076 "Agent Skills as a first-class primitive" (the alternative design) | Closed, not merged | n/a | PR #2076 |
| Protocol revision 2026-07-28 (`server/discover`, stateless, `CacheableResult`, required `resultType`, official `extensions`, resource-not-found becomes -32602) | Released, latest | 2026-07-28 (RC 2026-05-29) | `docs/specification/2026-07-28/changelog.mdx` |
| Skills Over MCP Working Group (Interest Group from about Feb 2026, chartered mid-April 2026) | Active | n/a | https://modelcontextprotocol.io/community/skills-over-mcp/charter ; working repo https://github.com/modelcontextprotocol/ext-skills (`specification/stable/skills.mdx`, `docs/decisions.md`) |

### What the Final spec requires

- **Capability and companion methods:**
  - The server declares `capabilities.extensions["io.modelcontextprotocol/skills"]`, not `experimental`.
  - `directoryRead` defaults to false. Setting it true means the server implements `resources/directory/read`.
  - The server must also declare the base `resources` capability and implement `skills/list` and `skills/get`.
- **`skills/list` request and result:**
  - Request: optional `cursor`.
  - Result: `resultType`, `skills[]` and `nextCursor`. On 2026-07-28 or later, `ttlMs` and `cacheScope` are required as well.
- **`skills/list` entry rules:**
  - Each entry has `uri` (the URI of its `SKILL.md`), `frontmatter` (the verbatim YAML rendered as JSON) and `resources`.
  - `resources` is required. It is either a complete array of `{uri, digest:"sha256:<hex>", size}` or the string `"dynamic"`.
  - An entry is never split across pages.
  - A partial listing is allowed.
- **`skills/get`:**
  - Takes `{uri}` and returns `{resultType, skill}`. An unknown URI returns -32602.
  - It must answer for skills that are missing from the listing.
  - `ttlMs` and `cacheScope` are required here too. The stable page settled this on 2026-09-08 (ext-skills PR #139); the SEP text itself left it open.
- **URIs:**
  - The form is `skill://<skill-path>/<file-path>`. The scheme is a SHOULD and grants no privilege.
  - The last segment of `<skill-path>` must equal `frontmatter.name`. Earlier segments are a free prefix and may encode a version.
  - Supporting files are read with plain `resources/read`.
  - `skill://index.json` and index resource templates were removed during the July 2026 rework. Discovery is `skills/list` only.
- **Integrity and limits:**
  - Hosts verify every file against its digest and size.
  - Hosts compare the fetched `SKILL.md` frontmatter field by field with the listed `frontmatter`.
  - Limits are 512 files and 16 MiB per skill.
  - Digests are not a security boundary.
  - Hosts must not prefetch.
- **Hosts that have native skills:**
  - A skill's identity is the pair (server label, uri).
  - Name collisions must be disambiguated, not dropped, and must not silently shadow another skill.
  - No precedence order is defined.
  - A plain `resources/read` of a `SKILL.md` does not activate the skill.
- **Security (MUST):**
  - Skill content is untrusted model input.
  - The skill's origin is shown.
  - No host-side code execution without per-skill approval.
  - `allowed-tools` is ignored for skills from MCP unless the user approves it.
  - Approvals are bound to the set of `{uri, digest}` pairs.
  - A disk cache is either writable only by the host, or re-hashed on each access.
- **Not in the spec:**
  - No `skills/bundle` and no archive distribution; this is deferred in the SEP appendix.
  - No skills list-changed notification.
  - `_meta` keys with the prefix `io.modelcontextprotocol.skills/` are reserved.

### Implementation status

- **Go SDK:**
  - go-sdk v1.8.0 (2026-09-14) ships no skills package.
  - It does have `AddExtension`, `AddReceivingCustomMethod` and `SetCacheable`, which this repo uses.
  - go-sdk PR #1238, "skills: add SEP-2640 protocol support", is **open**.
- **Other SDKs:**
  - python-sdk PR #3485 merged on 2026-10-11. I did not check whether it is in a release yet.
  - The TypeScript and C# PRs are open.
- **Conformance and Inspector:**
  - conformance PR #330 merged on 2026-09-11.
  - The Inspector CLI supports `skills/list --verify`.
- **Clients:**
  - The official client matrix lists Skills support as Partial for ChatGPT, fast-agent and Inspector, and full for mcpc.
  - **I found no public evidence that Claude Code supports the skills extension.** The SEP says Anthropic prototyped it internally. This matches the repo's own matrix, where `skills_extension` is "unverified".

I could not verify three things: Claude Code's current behaviour, whether a future revision will add a skills list-changed notification, and whether the Python SDK support is in a tagged release.

## 2. Conformance of this repo

### Conformant points (verified in code)

| Point | Evidence |
|---|---|
| Extension declared under `extensions`, with `directoryRead:false`; base `resources` capability declared | `internal/delivery/mcpserver/server.go:93-97`. `ServerCapabilities.AddExtension` writes the `extensions` map (go-sdk v1.8.0 `protocol.go:2297`). |
| `skills/list` with cursor paging, `resultType`, `ttlMs`, `cacheScope`, `nextCursor`; entries are never split | `server.go:316-355`, `types.go:296-329` |
| `skills/get` takes `{uri}`, only for a `SKILL.md` URI, and returns `{resultType, skill, ttlMs, cacheScope}` | `server.go:357-383`, `internal/app/distribution.go:186-211` |
| Entry `resources` is a complete `{uri, digest, size}` array with `sha256:` digests | `distribution.go:31-35, 380-395` |
| The last segment of the skill path equals `frontmatter.name`, and `description` is required | `internal/catalog/servable.go:66-83` (`name != id` rejected) |
| Limits of 512 files and 16 MiB match the spec | `servable.go:22-27` |
| Version pinning in the path prefix (`skill://skillhub/<manifest-digest>/<id>/…`), which the spec allows. Every content change changes the URI, so host approvals bound to `{uri, digest}` are revoked as the spec intends. | `distribution.go:380`, `server.go:309` |
| Supporting files are read through plain `resources/read`; each file is digest- and size-checked before it is served; text versus blob is decided by UTF-8 validity; `SKILL.md` is served as `text/markdown` | `distribution.go:213-274, 645-658` |
| An unknown or stale URI returns -32602 (`invalidParams`). `snapshot_expired` carries `current_uri` in the error data, an additive vendor field. | `server.go:597-639` |
| `ttlMs` and `cacheScope` set on every cacheable result, `resources/read` included | `server.go:102-105, 480` |
| Frontmatter served from the same bytes the client fetches. The bundled curator's compatibility contract lives in its own `SKILL.md`, so the listed frontmatter equals the fetched frontmatter. | `distribution.go:516-556` |
| Vendor `_meta` key `io.skillhub/local_path` stays outside the reserved `io.modelcontextprotocol.skills/` prefix | `server.go:587` |
| Modern stateless negotiation: go-sdk fills `InitializeParams` from per-request `_meta`, so client identification (curator hiding, telemetry) still works on 2026-07-28 | go-sdk v1.8.0 `server.go:1978-1995` |
| Recommendation stays separate from distribution: `skill_resolve` returns identity, URI and version only, never content (progressive disclosure) | `resolver_tools.go`, `docs/design/02-agent-hub-protocol.md` §8 |
| The content-review gate refuses `resources/read` for unreviewed third-party skills. This goes beyond the spec and fits its "untrusted content" MUST. | `server.go:445-460` |

### Deviations

| # | Deviation | Intentional? | Evidence | Recommendation |
|---|---|---|---|---|
| D1 | `skills/bundle` is a custom method inside the extension's `skills/` namespace. The spec explicitly deferred bundles. Only tests call it; the proxy uses HTTP `GET /mcp/bundle`. | Intentional feature, accidental naming | `server.go:305`; `distribution_test.go` is the only caller; `proxy.go` uses `bundleURL` | **Align.** Rename it to a vendor method such as `io.skillhub/bundle`, or drop the stdio method and keep the HTTP endpoint. Mention it upstream as evidence for the deferred bundle feature. |
| D2 | The curation profile also registers the skills extension, because `registerSkills` runs for every profile. `skillhub connect` registers both `skillhub` (runtime) and `skillhub-curation`, so a host that supports the extension lists every skill twice. The spec says hosts must disambiguate, not drop, so the user sees duplicates. | Accidental | `server.go:107-111` (registration does not depend on profile); `internal/hostintegration/runtime_access.go:185-189`, `integration.go:328` | **Align.** Register skills, and declare the extension, only for the `runtime` and `all` profiles. |
| D3 | `skills/get` returns a top-level vendor field `local` inside the spec-defined entry object | Intentional | `types.go:307-312`, `server.go:369` | **Keep for now.** Plan a move to `_meta["io.skillhub/local"]`, writing both places for one minor release, because a strict validator could reject the extra field. Propose upstream a reserved place for host-local materialization hints. |
| D4 | The curator is hidden from `skills/get` (not only from `skills/list`) for native Claude, Codex and Gemini sessions. The spec says `skills/get` must answer for skills missing from the listing. | Intentional | `server.go:365-367` | **Align at low cost.** Keep filtering the list, but serve `skills/get`; a host never asks for a URI it was not given. |
| D5 | Skills that await content review are listed, but every `resources/read` for them fails (`content_review_required`). A host that conforms to the spec will try to load them and fail. | Intentional (the review gate) | `server.go:316-355` (no trust filter); `server.go:445-460` | **Design decision.** Either omit `review_required` skills from `skills/list` (allowed, because a listing may be partial), or keep them and document the error. |
| D6 | Frontmatter goes through yaml.v3 → `map[string]any` → JSON. An unquoted YAML date becomes an RFC 3339 timestamp string, and large integers become float64. A host that compares field by field using a different YAML parser can see a mismatch and refuse the skill. | Accidental edge case | `servable.go:34-62` | **Keep and document.** Optionally add a lint warning for timestamp or large-number frontmatter values. |
| D7 | `serverInfo.version` is hard-coded to `"1"` for both the server and the proxy | Accidental | `server.go:98`, `proxy.go:221` | **Align.** Report `internal/version`. |
| D8 | The `skill_resolve` description says "use skills/get and resources/read after approval". The only verified host, Claude Code, cannot call those because it has no extension support; it loads through `skill_get`. The CLAUDE.md snippet's "snapshot_expired … call skills/get" also applies only to hosts with the extension. | Accidental wording | `resolver_tools.go:15`; `CLAUDE.md` Skill Hub block | **Align.** Wording: "load with skill_get, or skills/get plus resources/read when your host supports the MCP skills extension". |

### Gaps against the standard

| # | Gap | Evidence | Recommendation |
|---|---|---|---|
| G1 | The remote proxy (`skillhub` against a remote hub) exposes only tools: no `resources` capability, no skills extension, no `skills/list`, `skills/get` or `resources/read` | `proxy.go:221-237` | **Design decision.** Either forward the extension to the hub, or serve it from the proxy's verified cache. Needed before any remote user with an extension-capable host can use the standard path. |
| G2 | No `resources/directory/read` (`directoryRead:false`) | `server.go:97` | Keep. It is optional and `skills/list` already lists every file. |
| G3 | Stock-client conformance is verified only with the go-sdk client and Inspector. The conformance runner (PR #330) now covers skills, but the repo lists `@modelcontextprotocol/conformance` 0.1.16 as blocked (it requires HTTP). `skillhub serve` now offers Streamable HTTP, so that blocker may be gone. | `docs/mcp-compatibility-matrix.json` `unverified_checks` | Re-run the conformance suite against the HTTP listener and record the result in the matrix. |
| G4 | Protocol doc drift: the example URI `skill://software/consumer-reliability-review` does not match the implemented form `skill://skillhub/<digest>/<id>/SKILL.md` | `docs/design/02-agent-hub-protocol.md:423` | Fix the doc. |
| G5 | Wire types are local copies until the Go SDK ships skills support | `types.go:296` | Keep. When go-sdk PR #1238 merges, switch to its types and verification and delete the local copies. |

Not a gap: there is no skills list-changed notification because the spec defines none. The 300 s TTL with private cache scope is the spec's freshness mechanism.

## 3. Performance of suggest (`skill_resolve`) and load

### Measured on this machine

| Path | Measurement | Source |
|---|---|---|
| `skill_resolve`, in process, 32 / 300 skills | 2.7-3.0 ms, 407 KB, 5,104 allocs / 11.4-12.2 ms, 2.74 MB, 31,580 allocs | `BenchmarkSkillResolve` (matches `cook-261011-catalog-freshness`: 2.72-3.04 / 12.2-12.5 ms) |
| Resolver engine only (256 skills) | 0.89-1.24 ms, 676 KB, 3,798 allocs per operation | `BenchmarkResolverPerOperation` |
| Catalog freshness check, 32 / 300 skills | 0.77-0.81 ms / 5.8-6.2 ms. With 64 KiB bodies: 0.82-0.89 ms (was 12.7 ms before the quick_check cache) | `BenchmarkInspectWhileLocked` |
| `catalog.OpenCurrent` and `Close` | 0.98-1.66 ms, 73-175 KB | `BenchmarkCatalogStartupAndOpen/open_current` |
| Cold catalog build (8 skills) | 62-72 ms | same benchmark, `cold_startup` |
| `skill_resolve` miss over MCP, warm (2 fixture skills) | 1.8-2.8 ms, 0.98-1.01 MB | `BenchmarkReviewLoadPaths` (scratch) |
| **`skill_get` tool over MCP, warm (herdr-shaped, 13 files, 186 KB)** | **15-33 ms, 4.8 MB, 15,750-16,170 allocs** | same |
| `skills/get` over MCP, warm | 2.9-4.6 ms, 1.48-1.50 MB | same |
| `skills/list` over MCP, warm (3 skills) | 2.0-2.5 ms, 1.03 MB | same |
| `resources/read` of one file (server handler) | 2.4-2.9 ms without telemetry; 3.5-3.9 ms with telemetry (+1.1 ms, +105 KB) | same |
| Process start and initialize over stdio | p50 47.1 ms, p95 68.2 ms | `TestRemoteFetchBench` (`make remote-baseline`, 20 runs) |
| First request of a fresh session over stdio (cold process caches) | `skill_resolve` 7.0-8.3 ms p50; **`skill_get` 49-64 ms p50 (95-109 ms p95)**; `skills/get` 6.6-10.3 ms; `resources/read` about 7-12 ms per file; whole herdr folder (16 calls) 229 ms p50 | `TestRemoteFetchBench` |
| `New()` (tool schema derivation) | runtime profile 4.8-5.0 ms, 3.2 MB, 65k allocs; **all profile 23.9-24.0 ms, 16.2 MB, 358k allocs**; `startServing` and stop (all) 24-27 ms | `BenchmarkReviewStartup` (scratch). 54% of the time is in `jsonschema.(*Schema).Resolve`. |
| Long-running memory, 32 skills | RSS / PSS / private: start 28.0 / 26.2 / 26.2 MB; after 1,001 resolves 39.1 / 37.4 / 37.4 MB; after 60 s idle 35.5 / 33.7 / 33.7 MB. Go heap 10.1 MB, then 7.4 MB idle. The 18.6 MB binary mapping is the largest share. Resolve p50 6.4 ms, p95 15.6 ms over stdio. Telemetry 2002/2002 events written. | `TestMCPMemoryBudgetPerformance` |
| Heap after 1,001 resolves | in use 5.7 MB (3.5 MB before), 317 GCs | `TestMCPHeapProfilePerformance` |
| Binary size | 32.6 MB (`go install`, unstripped) | scratch `GOBIN` |

`strace` per request, from 100 warm iterations each, setup subtracted:

| Request | Catalog opens (pin files created) | `openat` calls | `git` processes spawned |
|---|---|---|---|
| `skill_get` | about 4 | about 2,000 | **about 7** |
| `skills/get` | 3 | about 1,200 | 0 |
| `resources/read` | 3 | about 1,200 | 0 |
| `skill_resolve` | 1 | about 480 | 0 |
| `skills/list` | 1 | about 720 | 0 |

### Hotspots, ranked by cost to the user

**H1. The `skill_get` tool runs the curation read model.**
- `skill_get` (`skill_tools.go:187-249`) calls `SkillService.GetSkillDetail` (`internal/app/skill_detail.go:58-117`). That calls:
  - `ReadSkill`, which opens the catalog through `skill.GetManifestAnyState`, then opens it again through `skill.ReadResource`;
  - `catalog.AssessSkillState` (`servable.go:177-300`).
- `AssessSkillState` costs the most:
  - It runs `canonical.Validate(root)` over the whole workspace (`servable.go:217`).
  - That calls `workspace.InspectWithOptions`, which spawns `git rev-parse --show-toplevel`, `git ls-files … machine files` and `git ls-files --unmerged` (`internal/workspace/workspace.go:190, 419-438`).
  - It opens SQLite directly.
  - It reads and SHA-256 hashes every file of the skill (`servable.go:280`).
- After that, `localSkill` → `SnapshotService.ensure` opens the catalog again and hashes every file twice more.
- In the CPU profile, `GetSkillDetail` is 35% and `AssessSkillState` is 24% (`canonical.Validate` alone 13%). `Ensure` is 17%.
- This is the load path for Claude Code, the only verified host. Warm it costs 15-33 ms; first per session it costs 49-64 ms, against about 3 ms for `skills/get`.

**H2. Each request opens the catalog generation and verifies it again.**
- `catalog.openWithFallback` (`open.go:304-403`) runs per request:
  - a flock;
  - `readPointer`;
  - `verifyPointerDatabase`, which is SQLite open #1 plus a metadata query (`open.go:503-546`);
  - `canonical.ScanCached`, which lstats the whole tree; in the profile, `stampTree` → `workspace.RelativeFiles` is 8%;
  - a pin file `mkdir`, `create`, `write` and `close`, then SQLite open #2 and a ping (`open.go:234-276`);
  - closing, which removes the pin file.
- Every SQLite open parses the schema again (`_sqlite3Init` is 11% of `skill_get` CPU).
- `skills/get` and `resources/read` each do this 3 times: `distribution.GetSkill` or `ReadResource`, `Ensure`, and the telemetry path.
- These are read-only requests, yet each one also writes to disk: pin files, `os.Chtimes` on the snapshot and the state directory (`skill_snapshot.go:751-756, 463`), and creating the config directory (`skill_snapshot.go:467`).

**H3. Immutable work is repeated within one request.**
- `buildDistributedSkill` (`distribution.go:294-396`):
  - reads and hashes `SKILL.md` (348), then re-reads and re-hashes **every other file** (360-373);
  - parses the frontmatter twice, once in `ValidateServableSkill` (352) and again at 355;
  - maps paths with nested O(n²) loops (361-373, 388-393).
- `ReadResource` builds the whole manifest (hashing every file) and then reads the selected file again (260).
- `loadDistributedSkillSource` builds the manifest and then reads and hashes every file again (504).
- Net effect: one `resources/read` hashes the whole skill about 3 times.
- `skills/list` builds and verifies every active skill on every call (`distribution.go:105-152`), and `paging.Owner` then JSON-encodes all entries to fingerprint the cursor (`server.go:338-341`).

**H4. Telemetry opens the catalog on the request goroutine.**
- `recordCurationTelemetry` (`internal/app/curation_telemetry.go:17-67`) calls `catalog.OpenCurrent` plus `LoadPolicy` for any event that lacks a snapshot or policy.
- That covers unsolicited `skill.loaded` events (any `resources/read` or `skills/get` without a prior resolution) and every `server.metric`.
- One such metric is `tools_list_bytes`, recorded on every `tools/list` after a full `json.Marshal` of the result (`server.go:286-296`).
- Measured cost: +1.1 ms and +105 KB per `resources/read`.
- `Recorder.Record` itself is asynchronous and drops events rather than block (`telemetry/recorder.go:213-242`). Only the enrichment step is synchronous.

**H5. Integrity check window after a rebuild.**
- The quick_check result is cached only for files older than 5 s (`canonical/file_identity.go:38`, `catalog/integrity_cache.go`).
- For 5 s after every rebuild (startup `EnsureCatalog`, every curation mutation), each request reruns `PRAGMA quick_check`, including FTS5 integrity. In the first profile this was 24% of CPU.
- The builder already ran `PRAGMA integrity_check` on that exact file (`build.go:313`), so the building process could seed the cache.

**H6. Resolve-specific work.**
- `LoadPolicy` runs twice per resolve (`app/resolver.go:173-179` and `204-210`).
- `SQLiteCatalog.Skills` JSON-decodes every active skill's `content_json` on each cache miss (`resolver/sqlite_catalog.go:122-147`). This scales linearly: 12 ms at 300 skills.
- `buildDistributedSkill` (which hashes every file) runs for the primary and each supporting skill (`app/resolver.go:235, 248`).
- The resolver cache key includes the task text (`resolver/resolver.go:418-428`), so it rarely hits in real sessions.

**H7. Startup.**
- `New` derives and resolves JSON schemas for every tool: 24 ms and 16 MB with the all profile, 5 ms and 3.2 MB with the runtime profile.
- The proxy pays the runtime cost again at every start (`runtimeToolDefinitions`, `proxy.go:355-379`, an in-memory server and client round trip).
- `HostSupportsSkillsExtension` unmarshals the embedded matrix JSON on every `skills/list` and `skills/get` (`hostintegration/matrix.go:63-79`).

**Not hotspots (checked):**
- The resolver's own scoring is about 1 ms.
- `Record` is non-blocking.
- The idle memory releaser works: 37.4 MB falls to 33.7 MB private after 60 s.
- Proxy cache hits take 1.4-7 ms with the hub confirmation sent in the background (`cook-261011-remote-phase5-loads`).
- `tools/list` is cached on the client by `ttlMs`, so repeat calls cost nothing.

### Cheap wins (ordered by payoff per effort)

1. Give `skill_get` a lean runtime path for active skills: one pinned generation, one manifest build, then `Ensure`. Drop `AssessSkillState`/`canonical.Validate` from runtime calls. Expected: 15-33 ms to about 3-4 ms, and no `git` spawns. See R1 for the contract question.
2. Pass the request's catalog snapshot and policy into load and metric events, or cache the last known values in the adapter, so `recordCurationTelemetry` never opens the catalog on the hot path. About -1 ms per unsolicited load; event types and `EventVersion` unchanged.
3. In `buildDistributedSkill`: parse the frontmatter once, build a `relative → internal` map once instead of the O(n²) loops, and let `ReadResource` serve the bytes it already verified instead of reading the file again.
4. Load the policy once per resolve, and wrap `HostSupportsSkillsExtension` in `sync.OnceValue`.
5. Seed the integrity cache from the build that just ran `integrity_check`, which removes the 5 s window.

**Bigger, contract-neutral win:** a per-generation memo keyed on the generation ID plus the file identity, the same pattern as `integrityCache` and `snapshotMemo`. It would cache the parsed policy, the decoded resolver `Skills()`, and the `DistributedSkill` manifests, with per-file digests verified by file identity. Optionally it would keep one read-only `*sql.DB` per pinned generation for the life of the process. Expected effect: `skills/list` and `skills/get` well under 1 ms, `resources/read` limited to reading and hashing one file, and resolve time no longer growing with the number of skills.

## 4. Code cleanliness (hot path and MCP delivery only)

Concrete issues, in rough order of payoff:

**Boundary leak.** The runtime tool depends on a curation diagnostic.
- `skill_get` returns `SkillDetail` (diverged, changed_resources, routing, rationale), described as "the shared read model for MCP skill_get, CLI, and WebUI" (`skill_detail.go:14, 56-57`).
- Runtime delivery and curation inspection are different jobs. Give runtime its own small read model; keep `SkillDetail` for CLI, WebUI and curation.

**Business logic in the delivery layer.**
- `recordLoad` (`server.go:498-547`) assembles telemetry `CaseRecord`s, with resolver top-k and attribution, inside `mcpserver`.
- That belongs in `app`, next to `RecordSkillLoad`, which `ResolverService.Resolve` already uses for its own cases.

**Duplicated logic.**
- The same three-line session extraction appears 6 times (`var session *mcp.ServerSession; if req != nil { session = req.Session }`).
- In `readResource` it appears twice (`server.go:448-451, 461-464`), after `request.Params` was already dereferenced at 438, so the nil checks are dead.
- The "curator, otherwise `buildDistributedSkill`" branch is repeated in `GetSkill`, `ReadResource` and `LookupSkills` (`distribution.go:169-181, 196-206, 224-234`).
- The confirmation-pin validation block is copied across the `*_confirm` handlers (`skill_tools.go:66-81, 123-138`).
- Five hand-written recursive schema walkers repeat the same traversal: `closeObjectSchemas`, `normalizeMultiTypeSchemas`, `shrinkOutputSchema`, `applyCommonConstraints`, `replacePropertySchema` (`server.go:766-888`). One `walkSchema(schema, fn)` would remove about 60 lines.

**Special cases by tool name inside the generic `addTool`.**
- `curation_session_record`, `skill_resolve` and `routing_evaluate` are special-cased there (`server.go:681, 691-704`), along with a `switch toolName` in `applyToolInputEnums` (`server.go:711-736`).
- Pass these as per-tool options at the registration site.

**Two error-mapping paths with different retry rules.**
- `safeToolError` (`server.go:944-978`) and `appResult` (`server.go:1001-1018`) each build `toolError` and handle internal errors separately.
- `applicationError` keeps a type alive with `_ = errorEnvelope{}` (`server.go:996-999`).

**Dead code** (golangci-lint `unused`, existing debt not gated by `--new-from-rev`):

| Kind | Items |
|---|---|
| Functions | `unavailable` (`server.go:939`), `registerSkillTools` (`skill_tools.go:16`), `openPinnedGeneration` (`catalog/open.go:224`), `isCanonicalEntityPath` (`catalog/input.go:369`), `splitSkillFrontmatter` (`app/skill_discovery.go:131`), `safeSourcePath` (`app/source.go:1188`) |
| Unused DTOs in `mcpserver/types.go` | `sourceIntakeAddInput`, `sourceIntakeListInput`, `sourceTriageInput`, `sourceLinkPreviewInput`, `sourceUnwatchPreviewInput`, `outcomeRecordInput`, `sourceTriageResult`, `sourceListItem` |
| Alias functions | `OpenServable` and `OpenServableLocked` exist only as aliases (`open.go:294-302`) |

**Bug found by staticcheck SA4006** (`catalog/build.go:505-506`):

```go
if err := temporary.Chmod(0o600); err == nil {
    _, err = temporary.Write(contents)   // assigns the shadowed err; the Write error is lost
}
```

If `Chmod` fails, the write is skipped and an empty pointer file is still synced and renamed into place. Readers detect a corrupt pointer and trigger a rebuild, so the effect is self-healing, but it is still wrong. Fix: drop the `:=`.

**Broken benchmark.** `BenchmarkMCPToolList` fails because it asserts 33 tools and the server registers 26 (`hardening_test.go:265`). CI does not run benchmarks, so nobody noticed.

**Oversized functions** (only where splitting pays):

| Function | Length | Note |
|---|---|---|
| `Resolver.Resolve` | 270 lines (`resolver/resolver.go`) | Splits into scoring and decision. |
| `ResolverService.Resolve` | 156 lines (`app/resolver.go:36-192`) | 115 of them are a deferred telemetry closure. Moving it into `recordResolutionTelemetry(...)` would make the request flow readable. |
| `AssessSkillStateWhileLocked` | 149 lines | |
| `buildDistributedSkill` | 102 lines | |
| `openWithFallback` | 99 lines, with 3 near-identical "open pinned, keep or release lock" tails (`open.go:357-393`) | |

**File layout.** `server.go` (1,018 lines) mixes six concerns: lifecycle, idle releaser, skills-extension handlers, load attribution, schema shaping, and error mapping. Splitting it into `skills_extension.go`, `schema_shaping.go`, `tool_errors.go` and `memory_releaser.go` is a pure move with zero risk. This is P3: do it only alongside other work in the file.

Things I would **not** change, because they are deliberate and verified:
- the per-request freshness guarantee (only make it cheaper);
- digest verification before serving;
- the asynchronous telemetry recorder;
- the idle memory releaser;
- the content-review gate.

## 5. Recommendations

Effort is in focused engineer-hours. Each item says what it buys against the four goals: (a) conformance, (b) suggest and load speed, (c) light and fast code, (d) clean code. Constraints kept as given: Git is the database, minimal model, no telemetry event type removed, `EventVersion` unchanged, the resolver stays recommendation-only.

### P1

| # | Item | Effort | Risk | Buys | Patch release? |
|---|---|---|---|---|---|
| R1 | Give `skill_get` a lean runtime path. For active skills: one pinned generation, then `buildDistributedSkill`, then `Ensure`, then content. No `AssessSkillState`, no `canonical.Validate`, no `git`. | 4-6 h | Medium. `skill_get` output fields `diverged`, `changed_resources` and `missing_resources` currently come from the assessment. | (b) 15-33 ms to about 3-4 ms warm and 50-64 ms to about 10 ms first call, for Claude Code loads; (c) -4.8 MB allocated per call; (d) separates runtime from curation. | **Patch** if those fields are kept, computed from the same digest pass (the snapshot build already detects mismatches). **Design decision** if you prefer to drop them from runtime output, which is a public output-schema change. |
| R2 | Telemetry enrichment without opening the catalog: pass snapshot and policy from the request, or the adapter's last known values, into `skill.loaded` and `server.metric`. Stop the extra `json.Marshal` on every `tools/list` by measuring once per tool set. | 2-3 h | Low | (b) about -1 ms per unsolicited load and per `tools/list`; (c) | Patch. Event types, fields and `EventVersion` unchanged. |
| R3 | Stop the curation profile from advertising or serving the skills extension (D2) | 0.5-1 h | Low | (a) no duplicate skills on extension-capable hosts | Patch |
| R4 | Fix the shadowed `err` in `catalog/build.go:505-506`, and fix or delete `BenchmarkMCPToolList` | 0.5 h | Very low | (d) correctness | Patch |
| R5 | Micro wins in the load path: frontmatter parsed once, O(n²) mapping replaced by one map, `ReadResource` serves the bytes it already verified, policy loaded once per resolve, `sync.OnceValue` for the matrix | 2-3 h | Low | (b) roughly -20-30% of `resources/read` and `skills/get` server CPU; (c) | Patch |

### P2

| # | Item | Effort | Risk | Buys | Patch release? |
|---|---|---|---|---|---|
| R6 | Per-generation memo for immutable data: policy, decoded resolver skills, `DistributedSkill` manifests, file digests verified by file identity. Optionally one long-lived read-only `*sql.DB` per pinned generation, with the pin held while it is cached and released on generation change. | 1.5-3 days | Medium. Pin and GC interaction; must keep the "no mutation between open and read" invariant (`OpenWithFallbackLocked`). | (b) `skills/list` and `skills/get` well under 1 ms, `resources/read` costs one file, resolve time stops growing with skill count; (c) far fewer syscalls (about 1,200 `openat` per load today) and no disk writes on reads | Mostly patch-safe (internal). Holding long-lived pins needs a short design note. |
| R7 | Rename `skills/bundle` to a vendor method, or drop the stdio method (D1) | 1 h plus doc | Low; only tests call it | (a) | Needs your decision (wire name). Minor release. |
| R8 | Serve `skills/get` for the curator on native sessions (D4); clarify the `skill_resolve` and CLAUDE.md wording for hosts without the extension (D8); set `serverInfo.version` (D7) | 1-2 h | Low | (a) | Patch, except that the CLAUDE.md snippet is generated by `doctor`/`connect` and should be reviewed with you. |
| R9 | Seed the integrity cache from the build's `integrity_check` (H5) | 1-2 h | Low | (b) removes 5 s of per-request quick_check after every rebuild | Patch |
| R10 | Run `@modelcontextprotocol/conformance`, now with skills scenarios, against `skillhub serve` HTTP. Update the matrix (G3) and fix the doc URI drift (G4). | 2-4 h | None | (a) evidence | Patch (docs and evidence) |
| R11 | Delivery-layer cleanup: move case assembly from `recordLoad` into `app`; one `walkSchema`; one error-mapping path; delete dead code and unused DTOs; a shared confirmation-pin helper | 4-6 h | Low (pure refactor, covered by existing tests) | (d), slightly (c) | Patch |

### P3

| # | Item | Effort | Risk | Buys | Patch release? |
|---|---|---|---|---|---|
| R12 | Skills extension in the proxy (G1): forward to the hub, or serve from the verified cache | 2-4 days | Medium | (a) for remote users with extension-capable hosts | Design decision |
| R13 | Move `local` from the entry's top level to `_meta["io.skillhub/local"]`, writing both places for one minor release (D3). Propose upstream a reserved key for host-local materialization hints. | 0.5-1 day plus upstream discussion | Low-medium | (a) | Design decision (wire shape) |
| R14 | Decide whether skills awaiting review are listed (D5) | 1-2 h after the decision | Low | (a) | Design decision |
| R15 | Switch to go-sdk skills types and verification once go-sdk PR #1238 merges (G5); delete the local DTOs | 0.5-1 day | Low | (a), (d) | Minor release |
| R16 | Startup: cache derived tool schemas (they are static per binary), or resolve them lazily, to cut `New` from 24 ms / 16 MB to a few ms with the all profile and for each proxy start | 0.5-1 day | Low-medium (SDK schema resolution) | (b) startup, (c) | Patch |
| R17 | Split `server.go` by concern; decompose `ResolverService.Resolve`'s deferred telemetry closure and `openWithFallback`'s repeated tails | 3-4 h | Very low | (d) | Patch |

## Open questions for you

1. R1: keep `diverged`, `changed_resources` and `missing_resources` in runtime `skill_get` output (computed cheaply), or reserve them for curation `skill_get`?
2. R7: rename `skills/bundle` to `io.skillhub/bundle`, or remove the stdio method and keep only `GET /mcp/bundle`?
3. R14: should skills awaiting content review appear in `skills/list`?
4. R12: is the skills extension in the remote proxy in scope before Claude Code ships public extension support?
