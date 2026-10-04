# skillruntime package implementation report

Status: done. Phase 3 is implemented as a new stdlib-only package, `internal/skillruntime`. It does not import `app`, `catalog`, delivery code, or `internal/telemetry`.

## Files created
- `internal/skillruntime/spec.go` – `Spec`/`Requires`/`Bin`/`Setup`, `ParseSpec` (string or object bins, unknown keys rejected, the same validation rules as the canonical manifest), `CanonicalJSON` (lists sorted), `Fingerprint` (hex sha256), `HasCommands`.
- `internal/skillruntime/trust.go` – `IsExecutableResource`, `Provenance`, `IsThirdParty`, `ResourceDigest`, `ExecutionDigest` (`sha256:` prefixed), `Verdict`, `Evaluate`, reason codes `scripts_review_required` and `scripts_review_stale`.
- `internal/skillruntime/checks.go` – `Check`, `LiveCheck` (platform, PATH presence, env presence only), version constraint parsing and comparison.
- `internal/skillruntime/doctor.go` – `Runner`, `ExecRunner` (no shell for probes, nil stdin, output capped at 8 KiB, `WaitDelay`), `DoctorOptions`, `RunDoctor`, `Result`.
- `internal/skillruntime/cache.go` – `MachineID`, `RuntimeFingerprint`, `CachePath`, atomic `WriteCache`, `ReadCache`, `SetupState`.
- Tests: `spec_test.go`, `trust_test.go`, `checks_test.go`, `doctor_test.go`, `cache_test.go`.
- I updated the checklist, status, and implementation notes in `phase-03-skillruntime-package.md`.

## Validation
- `go vet ./internal/skillruntime/` is clean.
- `go test -race -count=1 ./internal/skillruntime/` passes, with 92.1% statement coverage.
- I did not run `make check`, as instructed.

## Deviations and decisions
- `Evaluate(p, executables, spec, hasSpec, reviewedDigest)` takes a `hasSpec` argument that the phase signature does not list. Without it, an absent runtime block and an empty `runtime: {}` cannot be told apart, and they produce different execution digests.
- `CachePath` returns `(string, error)` because it validates the skill ID and fingerprint before building a path.
- A bare version constraint means `>=`.
- When the setup check is skipped for review, the doctor state is `setup_required`.
- The check command's output goes only into `Result.CheckOutput`. That field has the tag `json:"-"`, so it is never cached. A test confirms that a sentinel secret never appears in check results or in the cache file.
- `RuntimeFingerprint(manifestVersion, spec)` takes the version as a string from the caller. The manifest schema has no `version` field, so Phase 5 must choose what to pass.
- I found and fixed a bug while testing. When `cappedBuffer` embedded `bytes.Buffer`, `io.Copy` used the promoted `ReadFrom` and skipped the 8 KiB cap. A smoke test now covers this.

## Unresolved questions
- Phase 5 needs to decide which value to pass as `manifestVersion` to `RuntimeFingerprint` (for example the manifest digest or the binary version).
