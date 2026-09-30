# Telemetry / Evaluation Review (uncommitted working tree)

Scope: internal/telemetry, internal/evaluation, internal/app/{telemetry,evaluation,feedback,curation_telemetry,distill,source}.go,
internal/delivery/cli/{telemetry*,evaluation,distill,source}.go, internal/skill/lifecycle.go, .github/workflows/evaluation.yml.
Method: code read + throwaway tests/CLI runs in a scratch copy of the repo (no repo files edited).
Gates: `go vet` clean; `go test` for telemetry, evaluation, app, delivery/cli, skill pass.

## Confirmed findings

### H1. No-skill metrics and top-1 misclassify cases with multiple acceptable statuses
- internal/evaluation/runner.go:291-311 (`calculatePointMetrics`)
- `expectsNo` is true whenever `no_skill` is *one of* the acceptable statuses (normalizeCase also sets `Expected.NoSkill=true`, parse.go:295).
  A case accepting `[resolved, no_skill]` that resolves to an acceptable primary is counted as a **no-skill false positive** and a **recall miss**,
  while `outcome.correct == true`. The same case returning `no_skill` (also acceptable) is counted as a **top-1 miss**.
- Evidence (scratch test): case `[resolved,no_skill]`, primary `beta`, outcome resolved/beta/correct → `FP=1/1 recall=0/1 top1=1/1`;
  outcome no_skill/correct → `top1=0/1`.
- Impact: the "multiple acceptable outcomes" exit-gate claim is false; CI held-out gate (`no_skill_recall >= min`) can fail or pass for the wrong reason once such a case enters the corpus. validateCase explicitly allows these cases.
- Fix: define no-skill ground truth as "no_skill is the only acceptable status" (or add an explicit `no_skill_required`), count FP only when `resolved` is *not* acceptable, and exclude from the top-1 denominator (or count as correct) cases whose outcome is another acceptable status. Add a regression test.

### M1. `eval run` / `resolution replay` human output diverges from `--json`
- internal/delivery/cli/evaluation.go:374 prints `report.Metrics.AcceptableTop1.Numerator` as "acceptable".
- Evidence: committed CLI suite → human `Evaluation complete: 4 samples, 3 acceptable.`; JSON has 4/4 `correct == true` (the no_skill case is correct but not a top-1 numerator).
- Fix: print `count(outcomes where correct)` (and optionally top-1 separately with its denominator).

### M2. `skillhub telemetry health` counters are always zero; drop/error counters are never persisted
- internal/app/telemetry.go:35-45 opens a fresh recorder and reports its in-memory counters; internal/delivery/cli/telemetry_lifecycle.go:30-38 discards every per-command recorder's counters on exit.
- Evidence: 3 x `resolve` then `telemetry health` → `0 written, 0 dropped, 0 rejected, 0 errors` while `preview` shows 10 stored events.
- Impact: the "drop counters" deliverable is unobservable for CLI commands; health output is actively misleading.
- Fix: persist cumulative counters in `telemetry_meta` on Close (best effort) and report them, or relabel output as "this invocation" and drop the misleading `written` count.

### M3. Corrupt telemetry DB is reported as "workspace invalid" with a doctor remediation that finds nothing
- internal/delivery/cli/telemetry.go:52-55 → `writeTelemetryError` → `writeInvalidWorkspace`.
- Evidence: random bytes in runtime/telemetry.db → `telemetry health --json` exit 2, code `workspace_invalid`, WHY `file is not a database (26)`, FIX "Run skillhub doctor ... --fix --yes"; `doctor` reports "Workspace is healthy."; `validate` ok. Only `telemetry purge --yes` recovers (verified).
  (Primary commands are unaffected: resolve output/exit identical with corrupt DB — verified.)
- Fix: map telemetry store failures to a telemetry-specific error (degraded) whose FIX is `skillhub telemetry purge --workspace <path> --yes`; optionally auto-recreate on `file is not a database`/quick_check failure since the store is disposable.

### M4. Long-lived MCP recorder is permanently broken after runtime/ is deleted/recreated
- internal/telemetry/store.go:310-326 (`validateIdentity`) + anchor opened once in internal/delivery/mcpserver/server.go:102.
- Evidence (scratch test): Open → Flush ok → `rm -rf runtime` → mkdir runtime → Flush: `telemetry directory identity changed`; later Record dropped, state `degraded` forever.
- Impact: the plan documents `rm -rf runtime/` as safe; afterwards every MCP `skill_feedback` / `curation_session_record` call fails (surfaced as retryable internal error) and all resolution/curation telemetry is dropped until the MCP server restarts.
- Fix: on identity mismatch, re-run `openStoreAnchor` (with the same confinement checks) once and retry, instead of failing permanently.

### M5. `resolution replay` cannot be driven end-to-end from the CLI
- internal/evaluation/runner.go:40-42 requires `manifest.CaseDigest == DigestCase(case)`; `eval manifest` (cli/evaluation.go:67-114, app/evaluation.go:50-107) only emits `suite_digest`.
- Evidence: the only producers of `case_digest` are tests calling the internal `evaluation.DigestCase` (cli/evaluation_test.go:385, app/evaluation_test.go:112). A CLI-generated manifest always fails replay with `pinned replay artifact unavailable: case digest`.
- Fix: add `eval manifest --case <path>` (sets CaseDigest) or have promotion/replay tooling emit it.

### L1. Retention cutoff and trim order use lexical comparison of variable-precision RFC3339Nano
- internal/telemetry/store.go:436,494,543; feedback.go:128; curation_session.go:153; events.go:258.
- Evidence: `"2026-09-15T10:00:00Z" < "2026-09-15T10:00:00.5Z"` is false in SQLite/Go string order.
- Impact: sub-second only (events at an exact whole second survive up to <1s past retention; trim/preview order inside one second can invert). Low.
- Fix: format with a fixed-width layout (`2006-01-02T15:04:05.000000000Z07:00`) for stored `occurred_at` and cutoffs.

### L2. SKILL.md frontmatter description drifts from skill metadata on edit
- internal/skill/lifecycle.go:220-230, 637-643.
- When `SetContent` body has no frontmatter and the current SKILL.md has one, the *existing* header is reused verbatim, ignoring a description changed in the same edit; description-only edits never touch SKILL.md. MCP distribution serves the frontmatter description (app/distribution.go:311), catalog/resolver use metadata → two descriptions for one skill.
- Fix: when reusing the existing header, rewrite its `description` from the updated metadata (or reject divergence).

## Checked, no defect found
- Path/secret leakage: allowlisted token regex excludes `/` and `\`; MCP errors are sanitized (`safeToolError`); coverage resource IDs are hashed. Health `last_error` can contain absolute paths but is local-CLI only.
- Primary-result isolation: resolve/source/distill outputs and exit codes unchanged with corrupt DB, missing DB, panicking sink (startCommandTelemetry/safeRecordTelemetry).
- Distill/source atomicity: telemetry is recorded after the domain mutation via a read-only shared-lock catalog open; no telemetry path writes canonical files.
- Evaluation determinism: seeded bootstrap, sorted branches/exclusions, deterministic clock; repeated CLI runs byte-identical apart from manifest-pinned fields.
- Recorder batching loop / Close retry logic: correct.

## Unresolved questions
- Is `[resolved, no_skill]` intended to count as "no-skill expected" for recall? H1's fix depends on this product definition.
- Should curation-session `client.version` be the binary version rather than `telemetry.EventVersion` (app/curation_telemetry.go:118)?
