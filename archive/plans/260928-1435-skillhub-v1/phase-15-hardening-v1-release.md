---
title: "Phase 15 — Hardening, migrations and V1 release"
status: done
---

# Phase 15 — Hardening, migrations and V1 release

## Objective

Turn the integrated prototype into a supportable local product.

### Deliverables

- canonical migration framework;
- derived schema rebuild policy;
- backup/recovery documentation;
- install/upgrade/uninstall scripts;
- signed release artifacts/checksums;
- security review and resource limits;
- performance budgets and compatibility matrix;
- V1 operational runbook.

### Tasks

1. Implement explicit canonical migration preview/confirm/receipt flow.
2. Rebuild automatically for derived-only schema changes when safe.
3. Test binary upgrade never silently mutates canonical files.
4. Add source/network/file size/time limits.
5. Fuzz parsers, path handling and MCP inputs.
6. Run race/deadlock tests with multiple stdio processes.
7. Benchmark startup, rebuild, resolve p95/p99 and large workspace limits.
8. Test disk-full/read-only/permission/clock-skew scenarios.
9. Verify install paths, primary workspace discovery and host remediation.
10. Document Git clone → rebuild → serve disaster recovery.
11. Run complete fault-injection suite on every release platform.
12. Freeze V1 canonical/tool schemas only after compatibility/eval gates.

### Exit gate

- `curl | sh` installs one verified binary.
- Clone + `skillhub rebuild` restores equivalent behavior offline.
- Every managed mutation has tested crash recovery.
- `doctor` explains all invalid/stale/recovery states.
- No critical/high security finding remains.
- Release candidate passes stock-client and cross-platform matrix.

## Dependencies

- Phase 14 must be complete unless this phase is explicitly split into a smaller accepted cook scope.
- Preserve authority from `docs/PRD.md`, `docs/design/*`, `final.md`, and the V1 product boundary in `plan.md`.

## Related files

Expected file ownership will be refined before cooking this phase. The likely affected areas are:

- `/home/vantt/projects/mcp-skill-hub/cmd/skillhub/`
- `/home/vantt/projects/mcp-skill-hub/internal/`
- `/home/vantt/projects/mcp-skill-hub/schemas/`
- `/home/vantt/projects/mcp-skill-hub/system-skills/`
- `/home/vantt/projects/mcp-skill-hub/testdata/`
- `/home/vantt/projects/mcp-skill-hub/docs/`

## Validation gate

Before this phase is considered done, run the narrow tests added for the phase plus the current shared gates:

```bash
go test ./...
go vet ./...
```

Add phase-specific commands from the roadmap section above when implementation reaches this phase. If shared public contracts change, also run relevant CLI/MCP contract tests and delete-runtime/rebuild tests.

## Risks

- Implementing this phase before its prerequisites can weaken Git-first and derived-state invariants.
- Adding shortcuts to satisfy a command surface can create a second source of truth outside canonical files.
- User-facing behavior can drift from Phase 01 UX fixtures if adapters format results independently.

## Execution evidence

- Canonical migration is explicit, registry-driven, previewed, pinned, WAL-protected, recoverable, idempotent, and receipted with source/target versions. Doctor and rebuild never silently upgrade canonical files; derived schema incompatibility rebuilds only after canonical/recovery checks pass.
- Canonical ingestion is bounded to 8,192 files, 4 MiB per file, and 64 MiB aggregate. Host files, source walks, network operations, MCP frames, telemetry, evaluation inputs, and subprocess operations have explicit bounds.
- Native fuzz targets, every mutation/catalog fault hook, disk-full/read-only/permission/clock-skew tests, and deterministic multi-process stdio race tests pass. Release verification runs unit/race/vet/fuzz/fault gates on Linux, macOS, and Windows.
- Deterministic performance tests enforce 256-skill warm-open p95 below 100 ms and uncached resolver p95 below 50 ms while reporting p50/p95/p99 rebuild evidence.
- The standalone streamed installer authenticates the signed checksum manifest before archive verification, supports stable/prerelease SemVer, installs atomically, and has hermetic install/upgrade/rollback/uninstall coverage. It requires `cosign` and supports the published Linux/amd64 and Darwin/arm64 shell targets.
- The tag-only publication path builds CGO-free archives, generates SPDX SBOMs and deterministic checksums, keylessly signs all assets, attests SBOM/provenance, verifies evidence, and uses SHA-pinned actions with least privileges. Manual dispatch produces non-installable draft artifacts only.
- `docs/release-runbook.md` covers verification, migration, backup, clone/rebuild/serve disaster recovery, rollback, support, and performance budgets. `docs/mcp-compatibility-matrix.json` records executed and blocked client evidence.
- Final independent security/release review reported GO with no critical/high static blockers. Hosted OIDC/signing/publication remains evidence to collect on the first real tag, not a static implementation blocker.

## Rollback

- Revert files touched by this phase using Git.
- If canonical workspace mutations were introduced, verify recovery can roll forward or restore the previous valid state before deleting runtime artifacts.
- Runtime databases and caches remain disposable and may be removed with `rm -rf runtime/` in test workspaces only.
