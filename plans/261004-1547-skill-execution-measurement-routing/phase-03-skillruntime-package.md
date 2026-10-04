---
phase: 3
title: "skillruntime package: runtime, trust, doctor checks"
status: done
priority: P1
effort: 8h
dependencies: [1]
---

# Phase 3: skillruntime package

## Context

- [plan.md](./plan.md) D3, D4. Manifest fields from Phase 1.
- Provenance values already written by imports: `skill add` writes `provenance.origin.kind` `local`/`github` (`internal/app/skill_add.go:306`, `:422`); source import writes `provenance.source_id` (`internal/app/source_import.go:223`).
- Resource kinds come from path segments (`internal/catalog/project.go:402` `resourceKind`: `instructions`, `reference`, `script`, `asset`, `resource`).
- The full parsed `skill.meta.yaml` is available as JSON in the catalog (`canonical_entities.content_json`, read by `internal/resolver/sqlite_catalog.go:116`).

## Requirements

A new transport-neutral package `internal/skillruntime` with no dependency on `app`, `catalog`, or delivery code:

1. **Types and parsing.** `Spec{Requires{Bins []Bin; Env []string; Platforms []string}; Setup{Command, Check string}}`, `Bin{Name, Version string}`; `ParseSpec(contentJSON []byte) (Spec, bool, error)` accepting both string and object `bins` entries. `Spec.Fingerprint()` = sha256 of canonical JSON.
2. **Executable detection.** `IsExecutableResource(relPath string, head []byte) bool`: true for paths with a `scripts/` segment, for extensions `.sh .bash .zsh .py .js .mjs .cjs .ts .rb .pl .ps1 .bat .cmd .exe`, or content starting with `#!`.
3. **Trust.** `Provenance{OriginKind, SourceID string}`; `IsThirdParty(p)` is true when `OriginKind ∈ {github, git}` or `SourceID != ""`. `ExecutionDigest(executables []ResourceDigest, spec Spec, hasSpec bool) string` = `sha256:` over canonical JSON `{"executables":[{path,digest}...sorted], "runtime": spec-or-null}`. `Evaluate(p, executables, spec, reviewedDigest) Verdict{Trusted bool; RequiresReview bool; ExecutionDigest string; ReasonCodes []string}`. Reason code when withheld: `scripts_review_required`; when a previous approval no longer matches: also `scripts_review_stale`. Skills with no executables and no setup commands always pass.
4. **Live checks (non-executing).** `LiveCheck(spec, env LookupEnv, look LookPath, goos string) []Check` covering platform, bin presence (`exec.LookPath`), env presence (`os.LookupEnv`, value discarded immediately). `Check{Kind, Name, Status (pass|fail|skipped), Detail}`; `Detail` never contains env values.
5. **Doctor (executing, CLI only).** `RunDoctor(ctx, spec, workDir string, allowCheck bool, opts DoctorOptions) Result`:
   - Version probe: run `<bin> --version` (timeout 5 s, no shell), parse the first `\d+(\.\d+){0,2}` from stdout+stderr, compare against the constraint with a small comparator (`>=,>,<=,<,=`, missing parts = 0). No new module dependency.
   - `check` command only when `allowCheck` (trust passed): `sh -c` on Unix, `cmd /C` on Windows, `cmd.Dir = workDir`, inherited environment, timeout 60 s, output capped at 8 KiB and returned only to the caller.
   - `Result{State: ready|setup_required|unsupported_platform, Checks []Check, CheckedAt}`; platform mismatch short-circuits to `unsupported_platform`.
   - Command execution goes through an injectable `Runner` interface so tests do not spawn real tools except one Unix-only smoke test using `sh -c 'exit 0'`.
6. **Doctor cache.** `MachineID()` = first 12 hex of sha256(hostname + GOOS + GOARCH). `CachePath(root, skillID, fingerprint)` = `runtime/cache/doctor/<machine-id>/<skill-id>@<fingerprint[:16]>.json`, where the runtime fingerprint = sha256(manifest version + spec fingerprint). `WriteCache` writes atomically (temp + rename, `0600`, parent dirs `0700`, refuse symlinked components like `catalog.ensureRuntimeDirectory`); `ReadCache` returns `(Result, bool, error)`.
7. `SetupState(spec, hasSpec, live []Check, cached *Result) string`: `""` when no spec; `unsupported_platform` if live platform fails; `setup_required` if any live check fails or cached state is `setup_required`; `ready` when a cached result is `ready` and live checks pass; otherwise `unknown`.

## Files

Create:
- `internal/skillruntime/spec.go`
- `internal/skillruntime/trust.go`
- `internal/skillruntime/checks.go` (live checks, version parse and compare)
- `internal/skillruntime/doctor.go` (Runner, RunDoctor)
- `internal/skillruntime/cache.go`
- `internal/skillruntime/spec_test.go`, `trust_test.go`, `checks_test.go`, `doctor_test.go`, `cache_test.go`

## Steps

1. Spec parsing and fingerprint with table tests (string and object bins, missing block).
2. Executable detection and trust verdicts (third-party vs local, reviewed digest match, stale digest, no executables).
3. Live checks with fake `LookPath`/`LookupEnv`.
4. Version comparator table (`3.10.2 >= 3.9`, `18 > 18.0.0` false, unparsable version means fail with detail `version_unparsable`).
5. Doctor with fake Runner (timeouts, non-zero exit, `allowCheck=false` yields check status `skipped` with reason `scripts_review_required`).
6. Cache round trip and symlink refusal.

## Implementation status

- [x] Spec parsing and fingerprint (string and object bins, missing block, strict validation)
- [x] Executable detection and trust verdicts (third-party vs local, matching, stale, no executables)
- [x] Live checks with fake `LookPath`/`LookupEnv`; env values never recorded
- [x] Version comparator table and `version_unparsable`
- [x] Doctor with fake Runner (timeouts, non-zero exit, untrusted check skipped) plus one Unix `sh -c` smoke test
- [x] Doctor cache round trip, `0600`/`0700` modes, symlink refusal, `SetupState`

Implementation notes (decisions the spec left open):
- A bare version constraint (`"3.11"`) means a minimum (`>=`).
- `Evaluate` takes an explicit `hasSpec` argument so an absent runtime block and an empty one hash differently, consistent with `ExecutionDigest`.
- A setup check skipped for review makes the doctor state `setup_required`, because the user still has to approve the scripts.
- Check command output is returned in `Result.CheckOutput` (`json:"-"`), so it is never written to the cache.
- Spec canonical JSON sorts bins, env, and platforms, so reordering a list does not change fingerprints or invalidate an approval.

## Tests and validation

- `go test ./internal/skillruntime/`
- `go test -race ./internal/skillruntime/`
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| `check` command is arbitrary shell from metadata | Medium × High | Only in explicit CLI doctor, only when trust passes; runtime block is part of the execution digest, so a changed command invalidates approval. |
| Env values leak into output or cache | Low × High | Only presence is recorded; test asserts a sentinel value never appears in results or the cache file. |
| `--version` hangs or prompts | Low × Medium | No shell, closed stdin, 5 s timeout. |

## Rollback

Revert; the package has no callers until Phase 4.
