# Long-lived per-generation *sql.DB: measurement

## Finding 1: warm skill_get / skills/get / skills/list / resources/read already do not open SQLite
`catalog.View` (internal/catalog/view.go) opens and pins the generation only on the first `View.DB()` call.
Warm calls are served from the per-generation memo (`MemoGet`), so they do 0 opens.
Measured with `catalog.PinnedOpens()` per op (32 skills, warm): get 0, list 0, read 0, resolve 1.
Only resolve opens per call: the search query goes to the database every time
(pin-file create+write+remove, `sql.Open`, `Ping`, `Close`).
The 6-9% estimate therefore cannot apply to skill_get; the handle is not on that path.

## Prototype
Memo-owned shared `*Handle` (pin + DB), refcounted per View; retired when `memoFor` replaces the memo
for the root, closed when the last View releases. Diff: this branch's commit
(internal/catalog/view.go, internal/catalog/generation_memo.go).

## Numbers (go test -bench, -count 10, median; linux i7-13700F; noisy, ranges overlap)
| benchmark | before | after | delta |
|---|---|---|---|
| Resolve 32 skills | 1018us | 657us | -35% (-0.36 ms) |
| Resolve 300 skills | 3641us | 3077us | -15% (-0.56 ms) |
| Distribution get 32 | 831us | 550us | noise (0 opens both) |
| Distribution list 32 | 882us | 663us | noise |
| Distribution read 32 | 579us | 604us | noise |
| Distribution get 300 | 4012us | 4517us | noise |
| Distribution list 300 | 4772us | 6059us | noise |
| Distribution read 300 | 4728us | 5006us | noise |

Allocs/bytes unchanged for distribution; resolve about -1% bytes. Distribution rows move +/-30% with no
code-path difference (0 opens in both), so the noise floor is larger than any effect there.

RSS/heap: not measured separately. Expected cost is one retained pooled sqlite connection plus its page
cache (up to roughly 2 MB, an estimate) per root; per-op heap allocations did not change.

## Risks
- Breaks two existing tests (verified by running ./internal/...):
  `TestObservingAGenerationWritesNoPinUntilTheDatabaseIsNeeded` (closing a view must leave no pin) and
  `TestSkillRuntimeStatusApp/read_only_integrity` (a read-only status call must not create runtime paths).
  Both encode deliberate contracts.
- Permanent pin file + open handle on the current generation: generation cleanup and other processes see it
  as in use; on Windows an open handle blocks deleting/replacing the generation file and temp-dir cleanup in
  tests (pin_windows.go / atomic_replace_windows.go need review; not testable on this Linux host).
- Lifetime: needs a shutdown hook; prototype leaks when `clear(byRoot)` drops memos without retiring them
  and never closes at process exit.
- SetMaxOpenConns(1) shared across concurrent requests serializes resolves; a larger pool would be needed.
- Pin metadata (pid, created_at) goes stale after a crash.

## Recommendation
Skip. skill_get / skills/get gain nothing (already DB-free when warm). Only resolve gains, about 0.4-0.6 ms
(15-35%), at the cost of contract breaks and Windows file-lock hazards. Cheaper options if resolve latency
matters: skip the pin file for the read-only search under the shared lock the View already holds, or memoize
search results per (generation, query).
