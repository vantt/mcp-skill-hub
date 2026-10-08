# Runtime revision Part A: report

`make check` passes: vet clean, golangci-lint 0 issues, all packages pass. Nothing is committed.

## What changed
- **Skill-level trust.** `skillruntime.ContentDigest` hashes catalog resource rows (path, digest) plus the runtime block. A third-party skill is trusted only when `quality.content_reviewed_digest` equals that digest. All per-file executable detection, withheld lists, `.restricted` snapshots and `LocalResource.Executable` are deleted.
- **Unapproved third-party skill.** `skill_get` returns `local: {status: "review_required", reason_codes, review_command}` with no path, resources or preflight. No snapshot or state directory is created, and the file bytes are not read. `resources/read` refuses everything except `SKILL.md` with code `content_review_required`. `skill_resolve` still recommends it and attaches `setup.state = "review_required"` (also when the skill has no runtime block).
- **Review.** `skill review` now reports `content_trust` (third_party, approved, content_digest, reason_codes, approve_command) and prints a "Content trust" block for third-party skills. The web golden `skill-review.json` was regenerated (additive).
- **State directory.** `runtime/envs/<id>@<deps16>/`, created on demand for trusted skills that have a runtime block. It is built in a staging dir and renamed into place. The dependency manifest files are copied in as `0644`. An existing dir is only touched (mtime) and never overwritten. GC removes dirs that are unreferenced by an active trusted skill with a runtime block and untouched for 7 days, plus abandoned staging after 1 hour. Removal chmods files and dirs first.
- **Platform-only hub checks.** `LiveCheck` and `presenceCheck` are deleted and replaced by `PlatformChecks`. `LocalPreflight.live_checks` holds only the platform check. `SetupState` order is review_required, unsupported_platform, cached doctor state, unknown.
- **Doctor.** It runs bins (with version probe), env presence, platform and `check`. `check` runs in the state dir with both env vars added to the inherited env. Results carry `basis: "terminal"` in the cache file, `skill doctor` JSON and human output, `local.preflight.doctor.basis`, and `setup.basis`. A doctor run on an unapproved skill skips `check` and is not cached, so a skipped check cannot outlive the approval as a stale hint.
- **Renames.** `quality.scripts_reviewed_digest` is now `quality.content_reviewed_digest` (schema, canonical validator, `UpdateInput.ContentReviewedDigest`, review). `--approve-scripts` is now `--approve-content`, still CLI-only. Doctor and resolver `setup_state` accept `review_required` in the telemetry enum and the resolve response schema.

## Final API (`internal/skillruntime`)
- Trust: `ContentDigest(files []ResourceDigest, spec Spec, hasSpec bool) string`; `Evaluate(p Provenance, contentDigest, reviewedDigest string) Verdict`; `Verdict{Trusted, RequiresReview, ContentDigest, ReasonCodes}`; `IsThirdParty(Provenance)`; `ResourceDigest{Path, Digest}`; reason codes `ReasonContentReviewRequired` ("content_review_required") and `ReasonContentReviewStale` ("content_review_stale").
- State dir: `IsDependencyManifest(relPath) bool`; `DependencyFiles([]ResourceDigest) []ResourceDigest`; `DepsKey(files, spec, hasSpec) string` (16 hex); `StateDirName(skillID, depsKey) (string, error)`; constants `StateDirRoot = "runtime/envs"`, `EnvSkillDir`, `EnvStateDir`.
- Checks: `PlatformChecks(spec, goos) []Check`; `SetupState(untrusted, hasSpec bool, platform []Check, cached *Result) string`; `StateReviewRequired`; `BasisTerminal`; `Result.Basis`; `RunDoctor(ctx, spec, workDir, extraEnv map[string]string, allowCheck bool, opts)`; `Command.Env []string`.
- The filesystem side of state dirs lives in `internal/app/skill_snapshot.go` (`ensureStateDir`, GC), reusing the snapshot path-safety helpers. `SnapshotGCMinAge` is 24h, `StateDirGCMinAge` is 7 days.

## Env var names
`SKILLHUB_SKILL_DIR` (read-only snapshot) and `SKILLHUB_STATE_DIR` (writable state dir). They are exposed as `local.preflight.env`, and `local.preflight.working_directory` equals the state dir.

## Decisions and deviations
- **File modes.** The catalog stores only `path`, `kind`, `digest`, `size_bytes` for resources, with no mode or shebang. Following the spec, every snapshot file is `0444` and agents invoke scripts through an interpreter. No heuristics were added.
- **Verdict caching skipped.** The spec said to cache the verdict in the snapshot memo keyed by manifest version. I did not. The verdict depends on the runtime block and `content_reviewed_digest`, which live in `skill.meta.yaml` and are not part of the manifest version, so that key would serve a stale verdict after an approval. The computation is one sha256 over rows already in hand. The existing memo of verified snapshot directories is kept.
- **Status attention items.** The mechanism exists (`CurationHome`), but its action kinds are wired into the web UI (i18n and CTA mapping), so a new kind needs UI work. Skipped, as the spec allows when it cannot be added cleanly.
- **Doctor on an unapproved skill.** It still reports bins, env and platform, skips `check` (detail `content_review_required`), and does not write the cache.
- **State dir scope.** Only skills with a runtime block get a state dir and preflight, as specced. Skills without one have no `SKILLHUB_STATE_DIR`.
- The telemetry enum, the response schema (`setup.basis`) and the MCP tool descriptions were updated to match.
- Local-folder adds stay trusted, so the Part B docs note should say so.
- **Host instruction text.** I made a minimal edit, because the old `scripts_withheld` text was wrong. It now says to export `local.preflight.env`, and that `review_required` means use only SKILL.md and run nothing. This touched `bootstrap.go`, its test, and the `AGENTS.md`/`CLAUDE.md`/`GEMINI.md` blocks (the last two are gitignored). Part B should rewrite it as specced and may drop my edit.
- **Truncation slip.** While editing `internal/app/skill_review.go` I truncated its tail by mistake and restored `assessServedSkillFacts` through `computeNextAction` verbatim from `HEAD`. Phase 4 had not touched those functions, and the review tests pass. Worth a glance in the diff.

## For Part B
- Third-party skills with no runtime block never get a state dir or preflight. Case B (prose-only install) tells the agent to install into `SKILLHUB_STATE_DIR`, which will not exist for such skills. Either give trusted skills without a runtime block a minimal `local.state_directory`, or word the instruction to apply only when `local.preflight` is present.
- `reviewContentTrust` (`internal/app/skill_review.go`) already receives the full resource list, which is what B1 hints need. The review JSON field is `content_trust`.
- For the runtime-update flow (B2), a runtime change alters `ContentDigest`, so a prior approval becomes stale. This is covered at the app level by `TestSnapshotRequiresContentReviewForThirdPartySkill`.
- `skillruntime.IsDependencyManifest` is the shared list for the B1 `dependency_manifests` hint.
- The `skill_feedback` and `setup_failed` work is untouched.

## Tests
- Updated or added: content digest (order-independent, meta excluded, runtime included); `Evaluate`; `PlatformChecks` (no bins or env); `SetupState`; dependency manifest list, `DepsKey` and `StateDirName`; doctor env and basis (real shell); snapshot approval and stale (file edit and runtime edit); unapproved gives no path or dirs; state dir stable across SKILL.md edit and changed by lock edit; copies writable and not overwritten; state dir GC; preflight is platform-only; doctor runs `check` in the state dir with both vars; resolver `review_required` and platform or doctor hint with unchanged ranking; MCP resource-read refusal and release after approval; review CLI; schema cases.

Status: DONE_WITH_CONCERNS
Summary: Part A is implemented (skill-level trust, state dir, platform-only hub checks, `basis: terminal`) and `make check` passes.
Concerns/Blockers: Verdict caching and the `status` attention item were skipped for the reasons above. Skills without a runtime block get no state dir, which Part B's prose-install case needs to account for. `skill_review.go` was restored from `HEAD` after an accidental truncation, so check that file in the diff.
