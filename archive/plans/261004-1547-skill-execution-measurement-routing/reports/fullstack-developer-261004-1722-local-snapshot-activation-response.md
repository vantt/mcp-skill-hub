# Local snapshot export and activation response: implementation report

Status: done. `make check` passes: vet is clean, lint reports 0 issues, and every package passes its tests.

## Files
- Created `internal/app/skill_snapshot.go`. It holds `SnapshotService` (`Ensure`, `CollectGarbage`), `LocalSkill`, `LocalResource`, `LocalPreflight`, and the export, verify, and GC code.
- Created `internal/app/skill_snapshot_test.go` with 13 tests.
- Modified `internal/app/distribution.go`. It now has `loadDistributedSkillSource`, which returns the skill, `content_json`, and the verified bytes and kind of each file. I extracted `hashDistributedManifest` and `distributedRelativePath` so GC can compute active manifest keys from catalog rows. `DistributedContent` gains `SkillID` and `Path`.
- Modified `internal/app/skill_review.go`. It adds `ScriptTrust` (JSON `script_trust`), computed from canonical files with the same executable and trust rules the snapshot uses.
- Modified `internal/delivery/cli/skill_review.go`. Review output now has a "Script trust" block and prints `skillhub skill edit <id> --approve-scripts <digest>` when review is required.
- Modified `internal/delivery/mcpserver/{types.go,skill_tools.go,server.go}`. `skill_get` and `skills/get` now return `local`, `resources/read` returns `_meta`, and `Serve` runs GC at startup.
- New test assertions in `skill_tools_test.go`, `server_test.go`, `distribution_integrity_test.go`, and `internal/delivery/cli/skill_review_test.go`.
- Outside the phase file list, but required: I regenerated `internal/delivery/web/testdata/golden/skill-review.json` because the WebUI review endpoint serializes `SkillReviewResult`. The change is additive: 5 lines for `script_trust`.

## Security properties (tested)
- Snapshots are copies under `runtime/cache/skills/<id>@<d16>[.restricted]`. They are never the live tree. Every file is written from bytes verified against catalog digests.
- Each cache path component is lstat-checked. A symlinked `runtime/cache` is refused, and nothing is written to its target. Symlinked files inside a snapshot cause a rebuild. Relative paths are validated as clean and contained.
- Reuse requires an identical marker, matching size and sha256 for every listed file, and no withheld file on disk.
- An unreviewed third-party skill gets a `.restricted` directory without executables. `preflight` then omits both `check` and `setup`. A matching approval serves the full directory. A changed script adds `scripts_review_stale`.

## Deviations
- `CollectGarbage(ctx, workspacePath, minAge)` takes a context because it opens the catalog.
- `setup` is omitted along with `check` while scripts are withheld, so unreviewed runtime commands are never offered to a host.
- When the export itself fails, the response is `status: unavailable` with the extra reason code `export_failed`, and a warning is logged. The tool call never fails.

## For Phase 5
- The doctor cache key is `skillruntime.RuntimeFingerprint(LocalSkill.ManifestVersion, spec)`. `SkillDoctorService` must pass the same manifest version, or `local.preflight.doctor` will never find results.

## Unresolved questions
- `web/src/api/types.ts` does not declare `script_trust` yet. The field is additive and no UI uses it.
