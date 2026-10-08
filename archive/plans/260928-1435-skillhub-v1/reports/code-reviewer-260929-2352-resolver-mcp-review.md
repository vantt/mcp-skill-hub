# Resolver / Catalog / MCP distribution review

Date: 2026-09-30. Scope: uncommitted `internal/resolver/`, `internal/catalog/`, `internal/app/{resolver,distribution}.go`,
`internal/delivery/mcpserver/`, `internal/delivery/cli/{mcp,resolver}.go`. Review-only; no repo code edited.
Throwaway tests ran in a scratch copy of the repo (session scratchpad), not in the working tree.

## Confirmed findings

### 1. HIGH: Pin-directory race makes concurrent catalog opens fail with ENOENT
- `internal/catalog/catalog.go:146` (`Handle.Close` removes the pin dir), `internal/catalog/open.go:171-174` (`removePin` does the same),
  racing with `internal/catalog/open.go:125-137` (`openPinnedGeneration`: `ensureRuntimeDirectory` then `OpenFile(O_CREATE|O_EXCL)`).
- Failure: `Close` runs without any lock. Opens hold only a *shared* lock, so they run at the same time as other readers. Reader A's `Close` removes the
  now-empty `pins/<gen>` directory after reader B created it and before B creates its pin file. B's `OpenCurrent`
  then fails with `open .../pins/gen-.../<rand>.pin: no such file or directory`. Any two readers can hit this: MCP plus CLI, two MCP stdio
  processes (plan phase 12, task 9), or a resolve running next to a skills/get.
- Evidence: a scratch test with 8 goroutines x 150 `OpenCurrent`+`Close` failed 1, 1 and 2 of 1200 opens in 3 of 3 runs, always with that ENOENT.
- Fix: stop removing the pin directory in `Close`/`removePin`. Let GC prune empty pin dirs while it holds the exclusive lock. Otherwise, retry
  `ensureRuntimeDirectory`+`OpenFile` on `ENOENT`.

### 2. HIGH: The CLI can never finish a clarification round-trip and reports the failure as `workspace_invalid`
- `internal/app/resolver.go:28` (`sharedResolverCache` exists only in process memory), `internal/resolver/resolver.go:45-49` (the prior must be in the
  in-memory contract map), `internal/delivery/cli/resolver.go:40-42` (every service error becomes `writeInvalidWorkspace`).
- Failure: `skillhub resolve` returns `needs_context` with a question. The follow-up `skillhub resolve` sends `prior` in a new process, so it
  always fails. The CLI prints `workspace_invalid` and "Run skillhub doctor ... --fix --yes", which sends the user to repair a healthy workspace.
  MCP maps the same error to `unknown_resolution`. That breaks the phase 12 exit gate "CLI and MCP produce semantically equivalent results".
  The same loss hits MCP after a server restart, or after 1024 other resolutions evict the contract (the cache is FIFO).
  The same CLI mapping also shows stale-prior and stale-catalog errors as `workspace_invalid`.
- Evidence: two real CLI invocations against a scratch workspace. The first returned `needs_context` (`resolution_id res_f111bd1a...`,
  question `scope-minimum-multi_step`). The second, with a matching prior, returned `{"code":"workspace_invalid","WHY":"prior clarification was not issued by this resolver"}`, exit 2.
- Fix: make the clarification contract stateless. Resolution is deterministic, so `applyPrior` can re-run the prior-free request against the
  same snapshot and policy and check that it produces the same `ResolutionID` and `Question.ID`. That drops the in-memory contract map. Separately,
  route CLI resolve errors through the same classification the MCP layer uses in `safeToolError` (`unknown_resolution`, `stale_context`, `index_stale`).

### 3. HIGH: Non-UTF-8 bytes in text-typed resources are silently corrupted over MCP, so the delivered digest does not match the pinned one
- `internal/delivery/mcpserver/server.go:197-198` (any `text/*`, json or yaml MIME type is sent as `Text: string(bytes)`) and
  `internal/app/distribution.go:483-491` (the MIME type comes from `mime.TypeByExtension`, which depends on the host OS).
- Failure: canonical validation checks UTF-8 only for `.md/.txt/.yaml/.yml/.json` (`internal/canonical/skill.go:255-261`). An asset such as
  `assets/data.csv` (or `.html`, `.xml`, `.css`, or `.py` on hosts whose mime.types lists `text/x-python`) with non-UTF-8 bytes passes validation
  and the digest check. JSON encoding then turns the invalid bytes into U+FFFD. The client receives different bytes under the advertised digest,
  labelled `charset=utf-8`. This is the "silently serving changed resource" case that phase 12 forbids. Because the MIME type depends on the host,
  the same file can also go out as `text` on one OS and `blob` on another.
- Evidence: a scratch test served `assets/data.csv` with bytes `\xff\xfe`. It went out as `text/csv; charset=utf-8`. Advertised digest
  `sha256:90f844fc...`, digest of the delivered bytes `sha256:8267a882...`, `equal=false`.
- Fix: send `Text` only when `utf8.Valid(content.Bytes)`, otherwise send `Blob`. Replace `mime.TypeByExtension` with a fixed built-in extension table.

### 4. HIGH: One non-distributable active skill breaks `skills/list` for every skill and fails any resolve that picks it
- `internal/app/distribution.go:95-98` (the first per-skill error aborts `ListSkills`), `:306-314` (checks that the frontmatter name equals the ID and the description is non-empty),
  `internal/app/resolver.go:98-101` (the primary manifest error fails the whole resolve).
- Failure: the catalog build does not check distributability. A hand-edited or git-merged `SKILL.md` whose frontmatter `name` differs from the ID
  is still published as active. After that, `skills/list` returns -32603 for the whole workspace, including the bundled curator. The resolver keeps
  recommending the skill and then fails at `primary_manifest` with `internal_error`.
- Evidence: scratch test. The rebuild succeeded (`err=<nil>`). `ListSkills` then returned `n=0 err=skill consumer-review frontmatter must contain name ...`.
  Resolve returned `err=build resolved skill manifest: ...`.
- Fix: enforce the distribution invariants (SKILL.md present, frontmatter name equals ID, description present, resource limits) in canonical or catalog
  validation, so an invalid skill is either rejected or never projected as an `active` candidate. `ListSkills` should also isolate per-skill failures.

### 5. MEDIUM: `OpenCurrent` re-reads and re-hashes the whole canonical workspace twice on every call; the perf gate is flaky and the resolver budget skips this cost
- `internal/catalog/open.go:113-128` calls `InspectWhileLocked` (`:63-77`), which runs `canonical.Scan` (`internal/canonical/canonical.go:319-341`:
  two full `inventory` passes, each reading and hashing every canonical file through `os.Root`) and `verifyPointerDatabase` (`PRAGMA quick_check`,
  whose cost grows with DB size, and `resource_fts` stores full resource content).
- Evidence: pprof of `TestCatalogPerformanceBudgets`. `OpenCurrent` took 0.74s cumulative, of which `canonical.Scan` was 0.66s (89%) and
  `openPinnedGeneration` 0.02s. The cost is almost all syscalls, so it slows down under the parallel package load of `go test ./...`.
  At 256 skills, standalone: OpenCurrent p50 20ms / p95 27ms; full `ResolverService.Resolve` p50 24ms / p95 36ms. OpenCurrent is about 80% of the real
  resolve path, yet `TestResolverPerformanceBudget` (50ms) opens the handle once, outside the timed loop.
- Redundancy: `skills/get`, `resources/read` and manifest building already check each file's SHA-256 against the catalog (`readPinnedResource`),
  so the full-workspace scan adds no integrity for those paths. The generations are immutable (`immutable=1`), so running `quick_check` on every open repeats work already done at publish.
- Test gate: p95 over n=20 is the 19th of 20 samples, measured on wall-clock time inside `go test ./...`. That runs in `ci.yml:26` and the release
  gate `release.yml:150` on shared runners. The 160ms vs 100ms flake is contention, not a regression, and it can block a release.
- Fix: (a) cache freshness with a cheap stat fingerprint (path, size, mtime, inode) and do a full hash scan only when it changes, or give read paths an
  `OpenPublished` that relies on per-resource digest checks; (b) cache the `quick_check` result per generation ID for the life of the process;
  (c) move the budget tests to a serial, env-gated job (`-p 1 -count=1`) with more samples, and include OpenCurrent and manifest building in the resolver budget.

### 6. MEDIUM: Distribution misreports stale or concurrently changed state as an internal error or an integrity failure
- `internal/app/distribution.go:222-231` together with `internal/catalog/open.go:113-117`: the shared lock is released when `OpenCurrent` returns,
  and the working-tree files are read afterwards (`distribution.go:211`, `:302`). `server.go:209-223` maps the result.
- Failure, case (a): after an external canonical edit that has not been rebuilt, `skills/list`, `skills/get` and `resources/read` return
  JSON-RPC -32603 "could not be served" and log an ERROR, while `skill_resolve` returns `index_stale` with a rebuild hint.
- Failure, case (b): a mutation that commits between `OpenCurrent` and `readPinnedResource` produces `resource_digest_mismatch`, which is
  non-retryable and logged as an integrity failure. Phase 12 task 6 asks for a retryable `snapshot_expired` here.
- Evidence: established by reading the code path. Not reproduced, because the timing window is narrow.
- Fix: keep the shared lock (or a generation-scoped read) for the whole manifest build and read. Map "catalog is stale/missing" to `snapshot_expired`
  or `index_stale` in `distributionRPCError`.

## Checked and rejected (with basis)
- FTS5 query injection: tokens are limited to letters, digits and `-+#`, and each one is quoted. A scratch test with `-- ## ++`, `NEAR AND OR NOT`,
  `c#`, `c++` and `a-b-c` produced no errors and no operator interpretation. All SQL uses bound parameters.
- Resource path traversal: the URI's relative path is used only as a lookup key into `resourcePaths`, which is built from catalog rows. Reads go
  through `os.Root` plus an `Lstat` symlink check. Skill IDs containing `/` or `\` are rejected.
- Rows and handles: every `rows` is closed on all paths, and handles are closed through defers that propagate the error.
- Cache aliasing: `Get` and `Put` deep-clone slices and pointers. Candidate ordering uses deterministic tie-breaks.
- JSON-RPC ids and framing are handled by the go-sdk, and the custom methods return `jsonrpc.Error` with standard codes.

## Unresolved questions
- `OpenSnapshot` (`open.go:224-231`) picks the first generation, by random ID, whose `catalog_snapshot` matches. It ignores `builder_version` and
  `projection_input_digest`. If two retained generations from different builder versions share a snapshot, which one evaluation replays is arbitrary.
  Is builder identity meant to be part of the replay pin?
- Is losing clarification contracts across MCP server restarts accepted product behavior? It determines whether the stateless fix in finding 2 is required or optional.

Status: DONE
