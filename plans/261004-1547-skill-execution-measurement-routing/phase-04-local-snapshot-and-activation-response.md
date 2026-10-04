---
phase: 4
title: "Local snapshot export and activation response"
status: done
priority: P1
effort: 10h
dependencies: [1, 3]
---

# Phase 4: Local snapshot export and activation response

## Context

- [plan.md](./plan.md) D2, D3, D4.
- `buildDistributedSkill` (`internal/app/distribution.go`) already verifies every resource digest via `readPinnedResource` and keeps the unexported `resourcePaths` map (URI → workspace-relative path); snapshot export must live in package `app` to reuse it.
- Activation surfaces: `skill_get` tool (`internal/delivery/mcpserver/skill_tools.go:135`, result type alias `skillGetResult = app.SkillDetail` at `types.go:180`), `skills/get` (`server.go:187`, `skillEntry` at `types.go:306`), `resources/read` (`server.go:198`).
- Runtime directory safety helper pattern: `catalog.ensureRuntimeDirectory` (`internal/catalog/build.go:531`, unexported; replicate the same checks in app or export a small helper from `catalog`).
- Catalog GC precedent: `catalog.CollectGarbage` (`internal/catalog/catalog.go:229`) with a 24 h grace.

## Requirements

1. `app.SnapshotService.Ensure(ctx, workspacePath, skillID) (LocalSkill, error)`:
   - Opens the current catalog generation, builds the distributed skill (active and servable only), loads `content_json` for `runtime`, `quality.scripts_reviewed_digest`, and `provenance`, and computes the trust verdict with `skillruntime.Evaluate`.
   - Target dir `runtime/cache/skills/<id>@<hex[:16]>` or `...@<hex[:16]>.restricted` when executables are withheld.
   - Reuse: marker matches manifest version and every listed file matches size and digest. Otherwise build in `runtime/cache/skills/.staging/<random>`, write files (`0444`, executables `0555`, dirs `0755`), write the marker last, then `os.Rename`. If the rename loses a race, verify the winner and delete the staging copy. A corrupt existing dir is renamed to `.staging/trash-<random>` before the new one is placed.
   - Never writes outside `runtime/cache/skills`; every path component is lstat-checked (no symlinks); relative paths come only from verified resource rows.
2. `LocalSkill` (JSON):
   ```json
   {
     "path": "/abs/workspace/runtime/cache/skills/foo@0123456789abcdef",
     "manifest_version": "sha256:…",
     "status": "ready | scripts_withheld | unavailable",
     "reason_codes": ["scripts_review_required"],
     "resources": [{"path": "scripts/run.py", "local_path": "/abs/…/scripts/run.py", "kind": "script", "executable": true}],
     "withheld": ["scripts/run.py"],
     "execution_digest": "sha256:…",
     "preflight": {
       "state": "ready | setup_required | unsupported_platform | unknown",
       "working_directory": "/abs/…",
       "requires": {"bins": [...], "env": [...], "platforms": [...]},
       "check": "python3 scripts/check_env.py",
       "setup": "pip install -r requirements.txt",
       "live_checks": [{"kind": "bin", "name": "python3", "status": "pass"}],
       "doctor": {"state": "ready", "checked_at": "…"},
       "instruction": "Run `check` in working_directory under your own permissions. If it fails, ask the user before running `setup`."
     }
   }
   ```
   `preflight` is omitted when the skill has no `runtime` block. `check` is omitted while scripts are withheld. For a non-active or unservable skill, return `status: unavailable` with `reason_codes: ["not_servable"]` (or `resource_content_unavailable`) and no path; this is not a tool error.
3. MCP surfaces:
   - `skill_get`: replace the alias with `type skillGetResult struct { app.SkillDetail; Local *app.LocalSkill \`json:"local,omitempty"\` }`. Call `Ensure` only when `LifecycleState == "active"`; failure to export degrades to `status: unavailable`, never fails the tool.
   - `skills/get`: `skillEntry` gains `Local *app.LocalSkill \`json:"local,omitempty"\``; `skills/list` leaves it nil (no export on listing).
   - `resources/read`: set `ResourceContents.Meta["io.skillhub/local_path"]` to the file's local path when the snapshot contains it, and `"io.skillhub/withheld": true` when withheld.
   - `system-curator` is bundled and instruction-only: no `local`.
4. `SnapshotService.CollectGarbage(root, minAge)`: remove `runtime/cache/skills/*` entries whose `<id>@<d16>` does not match a current active skill's manifest version and whose mtime is older than `minAge` (24 h); remove `.staging` entries older than 1 h. Reuse touches the dir mtime (`os.Chtimes`). Called best-effort from `mcpserver.Serve` startup and at most once per hour per process after an export.
5. `skill review` shows script trust so a human can approve: `SkillReviewResult` gains `ScriptTrust{ThirdParty bool; Executables []string; ExecutionDigest string; Reviewed bool; ReasonCodes []string}`; human output prints the exact command `skillhub skill edit <id> --approve-scripts <digest>` when review is required.

## Files

Create:
- `internal/app/skill_snapshot.go` (`SnapshotService`, `LocalSkill`, export, verify, GC)
- `internal/app/skill_snapshot_test.go`

Modify:
- `internal/app/distribution.go` (small internal helper returning the distributed skill plus `content_json` and executable resource digests; no public behavior change)
- `internal/app/skill_review.go` (`ScriptTrust`)
- `internal/delivery/cli/skill_review.go` (print trust block)
- `internal/delivery/mcpserver/types.go` (`skillGetResult` struct, `skillEntry.Local`)
- `internal/delivery/mcpserver/skill_tools.go` (`skill_get` handler)
- `internal/delivery/mcpserver/server.go` (`getSkill`, `readResource`, Serve startup GC)
- `internal/delivery/mcpserver/skill_tools_test.go`, `server_test.go`, `distribution_integrity_test.go` (new assertions only)

## Steps

1. Implement export and verification against a temp workspace built with existing test helpers (`skill create` + activate + rebuild).
2. Add trust evaluation and the restricted variant.
3. Add GC.
4. Wire MCP surfaces; keep `skills/list` output byte-identical (existing tests prove it).
5. Add `ScriptTrust` to review.

## Implementation status

- [x] Export and verification (`SnapshotService.Ensure`, digest-keyed dirs, marker written last, staging + atomic rename, race and corrupt-dir handling, lstat-checked components)
- [x] Trust evaluation and the `.restricted` variant (executables withheld, `scripts_review_required` / `scripts_review_stale`)
- [x] Garbage collection (`CollectGarbage`, 24 h grace for non-current dirs, 1 h for staging, reuse touches mtime; run at `Serve` startup and at most hourly after an export)
- [x] MCP surfaces (`skill_get` `local`, `skills/get` `local`, `resources/read` `_meta`; `skills/list` unchanged; curator gets no `local`)
- [x] `ScriptTrust` in `skill review` (JSON field `script_trust`; human output prints the approval command)

Implementation notes (decisions the spec left open):
- `CollectGarbage` takes a `context.Context` first, because it opens the catalog generation to compute the active manifest set.
- While scripts are withheld, `preflight` omits both `check` and `setup`, since unreviewed runtime commands are part of the execution digest. The instruction text tells the host not to run them.
- If the export itself fails (for example, because of an unsafe cache directory), MCP surfaces report `status: unavailable` with reason code `export_failed` and log a warning. The tool call does not fail.
- The doctor cache key uses `RuntimeFingerprint(LocalSkill.ManifestVersion, spec)`, so Phase 5 must pass the same manifest version when it writes the cache.
- `DistributedContent` gains `SkillID` and `Path` so `resources/read` can map a resource to its snapshot file without re-parsing the URI.
- The `skill-review` WebUI golden file (`internal/delivery/web/testdata/golden/skill-review.json`) was regenerated for the additive `script_trust` field.

## Tests and validation

- `go test ./internal/app/ -run 'Snapshot|Review'` then `go test ./internal/delivery/mcpserver/`
- Cases: first call exports and files match digests; second call reuses (no rewrite, mtime touched); a modified snapshot file triggers re-export; working-tree drift yields `unavailable/resource_content_unavailable`; third-party skill with `scripts/x.sh` and no approval → `.restricted`, no `x.sh` on disk, reason code present, `check` omitted; matching approval → full dir; stale approval → `scripts_review_stale`; local skill with scripts → full dir; two goroutines calling `Ensure` concurrently end with one valid dir; GC removes an outdated dir older than 24 h and keeps the current one; a symlink planted at `runtime/cache` is refused.
- MCP: `skill_get` on an active skill includes an absolute `local.path`; on a draft returns `local.status=unavailable`; `resources/read` `_meta` carries the local path; `skills/list` unchanged.
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Disk growth from many digests | Medium × Low | Digest-keyed dirs, 24 h GC, size bounded by `MaxDistributedSkillBytes` (16 MiB) per skill. |
| GC deletes a dir an agent is still using | Low × Medium | Only non-current digests older than 24 h; reuse touches mtime. |
| Setup artifacts written into the snapshot dir break verification | Medium × Low | Verification checks only manifest-listed files; extra files are allowed. |
| Windows read-only/permission semantics differ | Medium × Low | Mode bits are best-effort on Windows; integrity relies on digests, not modes. |
| Large skills make `skill_get` slower | Low × Medium | Reuse costs one hash pass, the same order as the existing `buildDistributedSkill` verification. |

## Rollback

Revert; delete `runtime/cache/skills`. MCP output returns to the previous shape.
