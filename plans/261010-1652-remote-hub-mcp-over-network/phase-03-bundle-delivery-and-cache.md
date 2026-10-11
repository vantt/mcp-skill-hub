---
phase: 3
title: "Skill bundle delivery and client cache (skill runs, fetch is fast)"
status: pending
priority: P1
effort: "1.5d"
dependencies: [2]
---

# Phase 3: Skill bundle delivery and client cache

## Goal
A remote client gets a complete, runnable copy of an approved skill in one exchange, keeps it in a local cache keyed by the skill digest, and never pays for it twice. This phase carries the two headline criteria.

## The problem today
- `skill_get` and the resources carry `io.skillhub/local_path` (`server.go:459`), a path on the hub's disk. A remote client cannot use it.
- Reading a skill file by file costs one `resources/read` per file: 14 round trips for `herdr-cook-plan`. On a 100 ms link that is more than 1.4 s before anything runs.
- Skills with `scripts/`, `setup` or `check` expect a real local directory and the `SKILLHUB_*` environment variables (see `internal/skillruntime/statedir.go`, `spec.go`).

## Design

1. **One bundle per skill version.** The hub offers a bundle of the whole approved skill folder (a deterministic archive of the same files the content digest covers, `.meta/` excluded). It is addressed by the existing skill URI, which already contains the skill digest, so a bundle is immutable and cacheable forever. Compress it (zstd or gzip) and send it in one response; skip files over the existing size caps.
2. **Trust is unchanged.** The bundle is served only for approved, active skills, with the same checks as `resources/read` today. The client verifies the archive against the digest in the URI before using it; a mismatch is discarded.
3. **Client cache.** The client stores each bundle under its local state directory by digest (`<state>/remote/<hub-id>/<skill-id>/<digest>/`). A lookup is a directory check, so a repeat use has no network cost. `skill_get` returns, on the client side, a `local.path` that points at this directory, so the activation flow and the `SKILLHUB_*` variables work exactly as for a local hub.
4. **Where this runs.** The client side needs a small local component: a stdio MCP proxy (`skillhub mcp proxy --remote <url>`) that the agent host starts like today's `skillhub mcp serve`. It forwards `skill_resolve` and `skill_feedback`, and intercepts `skill_get` to fetch and materialize the bundle. This keeps every host working with no change to how hosts launch MCP servers, and keeps the skills runnable.
5. **Speed beyond the cache.** Send the bundle with `ETag` equal to the digest and honour `If-None-Match`; prefetch nothing automatically (resolve then get is the normal pair, so `skill_resolve` may include the top skill's digest so the client can start the fetch at once); keep one HTTP/2 connection open.
6. **Skills that run code.** The `check` and `setup` commands run only on the client, in the cached directory, with the same preflight and approval rules as local skills (`local.preflight`). They never run on the hub for a remote caller (decision Q3).
7. **Failure behavior.** If the hub is unreachable and the digest is cached, the client may use the cached copy and says so; if not cached, `skill_get` returns a plain error and the agent continues without the skill.

## Files to Create / Modify
- Create: `internal/app/skill_bundle.go` (deterministic archive of an approved skill folder; digest check helper)
- Modify: `internal/delivery/mcpserver/distribution.go` (bundle resource or tool method, `ETag` handling, no `local_path` over HTTP)
- Create: `internal/delivery/mcpserver/proxy.go` and `internal/delivery/cli/mcp_proxy.go` (the stdio proxy and its local cache)
- Create: `internal/skillruntime/remote_cache.go` (cache layout, digest verify, atomic extract, garbage collect by age and size)
- Create tests next to each file

## Tasks & Steps
1. Implement the archive and prove it is deterministic and covers exactly the content-digest file set.
2. Serve it over HTTP and over stdio (so local tests and the proxy share one code path).
3. Implement the proxy and cache; extract atomically (temp directory, then rename) so a crash never leaves a half skill.
4. Make `skill_get` through the proxy return a valid `local.path` and `local.env`.
5. Run a real skill's `check` through the proxy end to end on loopback.
6. Add cache garbage collection (keep the last N versions, cap total size) and a `skillhub remote cache clear` command.

## Verification
- Digest of the extracted folder equals the hub's digest for `test-audit` and `herdr-cook-plan`.
- One network exchange fetches a whole skill (counted in the test).
- Second use of the same version makes zero requests for content (test with a request counter).
- A changed skill produces a new digest and a new fetch; an unrelated hub change fetches nothing.
- An unapproved skill returns the same blocked error as local.
- A tampered archive is rejected by the digest check.
- `check` command of a test skill runs on the client from the cached directory.
