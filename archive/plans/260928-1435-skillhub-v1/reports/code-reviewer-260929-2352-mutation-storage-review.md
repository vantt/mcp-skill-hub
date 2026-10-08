# Mutation / storage / migration review (uncommitted changes)

Scope: internal/mutation, internal/canonical, internal/workspace, internal/migration, internal/app/migration.go, internal/app/workspace.go, internal/delivery/cli/migration.go, internal/source/filesystem.go.
Baseline: `go vet` clean; `go test` on the 7 scoped packages: 295 passed.
Every finding below was reproduced with a throwaway test injected through `go test -overlay` (no repo edits). The scratch tests are in the session scratchpad (`app_review_test.go`, `app_review2_test.go`, `mut_review_test.go`).

## Confirmed findings

### 1. [High] `doctor --fix` silently upgrades a legacy v0 clone; `migrate` refuses the same workspace
- Where: internal/workspace/workspace.go:110-112 (missing marker plus missing `.skillhub/` gives `schema_version_missing`, Fixable=true) vs internal/migration/migration.go:63-66 (`DetectVersion` treats any missing marker as v0) and migration.go:128-135 (`planLegacyV0ToV1` accepts only `canonical_schema_incompatible`).
- Scenario: Git does not track empty directories, so a fresh clone of a v0 workspace has no `.skillhub/` at all. `skillhub migrate` then fails with "legacy v0 layout is not otherwise v1-compatible: .skillhub/schema-version: Workspace schema version is missing". `skillhub doctor --fix --yes` then writes `1\n` through `workspace_remediation`, which leaves no receipt with source/target schema versions. This breaks design 07 §12.3: doctor "must not rewrite" the canonical version.
- Evidence: `TestReviewDoctorFixUpgradesCloneWithoutSkillhubDir`. The migrate preview errors, doctor --fix reports `applied`, the marker becomes `"1\n"`, and no receipt contains `source_schema_version`.
- Related race: internal/app/workspace.go:203-207 checks non-fixable findings before taking the lock. The re-Inspect at :222 does not check them again, and `RemediationFiles` (:240) always writes the marker. A marker removed or changed between the two Inspects is overwritten without migration.
- Fix: in an existing workspace, classify a missing marker as `canonical_schema_incompatible` whether or not `.skillhub/` exists. Keep `schema_version_missing` fixable only for a workspace with no canonical content (for example, no files under `history/operations`, `skills` or `registry`). After taking the lock, re-check `!Fixable` findings and abort.

### 2. [High] A second `migrate --yes` after the marker is lost reports success but writes nothing
- Where: internal/migration/migration.go:111 (fixed idempotency key `canonical-migration:0:1` with a content-only request digest). This reaches internal/mutation/mutation.go:224 (`existingOperation` short-circuit) before any write, and internal/app/migration.go:90-105 still reports "applied and the catalog was rebuilt".
- Scenario: the migration is applied once, then the marker is deleted (manual `rm`, a partial `git checkout`, or a merge). `migrate --yes` finds v0 and previews it. Commit then finds the old receipt with the same key and digest and returns it. The marker stays absent, no catalog is built, and `Generation` is `""`. The same empty-`Generation` receipt comes back when two hosts migrate at once: the loser gets the winner's receipt from `existingOperation`, which never fills in `Generation`.
- Evidence: `TestReviewMigrateReappliedAfterMarkerLoss`. The second call returns status=applied with the same OperationID as the first and gen="", and the marker still does not exist.
- Fix: when `ConfirmMutationWithOptions` returns a recorded receipt, check that the current canonical snapshot equals `receipt.CatalogSnapshot` (or that `DetectVersion` equals target). If it does not, fail with a conflict or recovery error. Fill `Generation` through `FindGenerationForOperation` as `lookupAppliedMigration` already does. Another option is to include the base snapshot in the migration idempotency key.

### 3. [Medium] `DetectVersion` follows symlinks, reads without a size bound, blocks on FIFOs, and echoes file contents in errors
- Where: internal/migration/migration.go:64 (`os.ReadFile(filepath.Join(...))`) and :74 (`%q` of the whole trimmed file). `Migrate` calls it at internal/app/migration.go:56, before `workspace.Inspect`, which is the call that rejects symlinks and non-regular files.
- Scenario: an untrusted workspace (for example a cloned repo) ships `.skillhub/schema-version` as a symlink to `~/.aws/credentials`. `skillhub migrate --json` prints the target's contents in the error. The CLI writes the error through `writeInvalidRequest`, and an agent driving the CLI would capture it. A symlink to `/dev/zero` can exhaust memory. A FIFO hangs the command indefinitely, and ctx is not honoured. This bypasses the V1 bounded-read rule that `workspace.readCanonicalFileBounded` enforces.
- Evidence: `TestReviewMarkerSymlinkLeak` produced `canonical schema marker "TOPSECRET-API-KEY" is invalid`. `TestReviewMarkerFifoHangs` blocked for more than 3s.
- Fix: read the marker with `os.OpenRoot(root)` plus a Lstat regular-file check and a bounded read (reuse the workspace bounded reader, exported). Do not echo the value; report the length or state that it is non-numeric.

### 4. [Medium] The operation receipt is not part of the pre-commit limit check, so a near-limit workspace gets stuck in unrecoverable `recovery_required`
- Where: internal/mutation/mutation.go:254 (`validateVirtualSnapshot` validates the tree without the receipt). internal/mutation/transaction.go:154 (`validateApplied` validates after the receipt has been written). internal/mutation/recovery.go:166 (roll-forward runs the same validation again).
- Scenario: every mutation adds one receipt file of up to about 512 KiB of inline content. At 8192 canonical files (or near 64 MiB), an in-place edit passes Plan and the virtual validation. All canonical paths and the receipt are applied, then `validateApplied` fails with "canonical file count exceeds V1 limit". The WAL stays pending, and `RollForward` fails the same way every time. Every later mutation returns `ErrRecoveryRequired` until an operator finds `RollBack`. Doctor points to roll-forward.
- Evidence: `TestReviewReceiptPushesWorkspaceOverFileLimit` showed confirm err "mutation produced invalid canonical state: .: canonical file count exceeds V1 limit of 8192 files". The transaction was still pending before and after `RollForward`, and `RollForward` returned the same error.
- Fix: build the receipt before virtual validation and write it into the virtual tree at `receiptPath`, so `commitPrepared` receives the receipt bytes and does not rebuild them. Alternatively, reserve one file and `len(receipt)` bytes of headroom in the pre-commit check. Separately, when post-apply validation fails only on resource limits, recovery should switch to rollback.

## Checked and not reported
- Schema-version fields in `proposalDigest`/`requestDigest`/receipts: they are consistent, validated in pairs, and a mismatch fails closed (`ErrIdempotencyConflict`).
- `canonical.readCanonicalFile` swapped to `os.Root` plus Lstat and fstat: escaping symlinks are blocked, and the per-file and aggregate bounds are enforced on both the Validate and inventory paths.
- `planLegacyV0ToV1` reading the marker unbounded at migration.go:149: only reachable after `Inspect` has rejected symlink and non-regular markers. Low risk.
- Filesystem source adapter gate channel: acquire and release are paired and the ctx-aware select is correct. No leak found.
- Receipts: `ChangedPaths` are workspace-relative, so no absolute paths leak into receipts.

## Unresolved questions
1. Design 07 §12.3 step 5 says "a stale preview ... fails closed". `migrate --yes` re-previews and applies in one call, and the CLI takes no `--proposal-digest` or `--base-snapshot`. The operator's reviewed preview is therefore never bound to what gets applied. Is `--yes` meant to confirm "whatever is fresh", or should the preview's digest or snapshot be passed back? This matters little for the single-marker 0→1 step but will matter for future migrations.
2. `PlanMutation` copies the whole workspace for virtual validation: the near-limit test took about 40s per mutation. Is that inside the phase-15 performance budget?

Status: DONE_WITH_CONCERNS
