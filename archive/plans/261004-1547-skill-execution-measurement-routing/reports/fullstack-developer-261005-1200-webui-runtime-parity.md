# Phase 12a: WebUI runtime parity report

## Summary
- Extracted shared helper `cachedDoctorResult` in `internal/app/skill_doctor.go` used by both `setupAnnotation` and `RuntimeStatus`.
- Implemented `(SkillService).RuntimeStatus` in `internal/app/skill_runtime_status.go`:
  - Returns `SkillRuntimeStatus` carrying canonical runtime spec, `served` status, `setup` hint, cached `doctor` result, `doctor_command`, `env_keys` (names only, never values), and `env_set_command`.
  - Reuses `ResolverService{}.setupStatus` for `setup` and `cachedDoctorResult` for terminal doctor checks.
  - Returns `env_file_unusable` warning on malformed or symlinked env files without failing the request.
  - Verified read-only guarantee: does not create snapshots, state dirs, or config files.
- Implemented Web read endpoint `GET /api/v1/skills/{id}/runtime` in `internal/delivery/web/routes_runtime.go`.
- Added tests verifying web interface has no approval path:
  - `POST /api/v1/skills/vendor-skill/approve` returns 404 or 405.
  - `POST /api/v1/skills/vendor-skill/update/preview` with `content_reviewed_digest` is rejected with 400 (`DisallowUnknownFields`).
- Generated and verified goldens in `internal/delivery/web/testdata/golden/`:
  - `skill-review-third-party.json`
  - `skill-runtime-third-party.json`
  - `skill-runtime-approved.json`
  - `skill-runtime.json`
  - `skill-runtime-unknown.json`
  - Verified 0 occurrences of sentinel env values across all goldens.
- Implemented frontend components:
  - `ContentTrustCard.tsx`: displays content digest, approval badge, changes since approval (scripts/runtime/dependencies changed flags, modified paths, diff command), approve command with notice that approval is CLI-only, and agent impact text. Contains no approve button or form.
  - `RuntimeTab.tsx`: displays header verdict from `setup.state` with terminal basis caption, readiness checklist rows (platforms, binaries with doctor evidence, environment variables with inline `skillhub skill env set` command and "Values are never shown" notice, setup check and setup command with state dir notice), re-check terminal command, and authoring warnings from `runtime_hints`.
  - Wired into `ReviewTab.tsx` and `SkillDetailScreen.tsx`.
- Validated via Vitest and Playwright:
  - `ContentTrustCard.test.tsx` (4 tests pass)
  - `RuntimeTab.test.tsx` (3 tests pass)
  - `web/e2e/skill-runtime.spec.ts` (Playwright E2E passes in 17.7s)

## Verification
- `go test -count=1 -run 'Doctor|Resolve|Setup' ./internal/app/` (PASS)
- `go test -count=1 -run RuntimeStatus ./internal/app/` (PASS)
- `go test -count=1 ./internal/delivery/web/` (PASS)
- `make web-test` (typecheck, eslint, vitest 12 files / 46 tests) (PASS)
- `make web-build` (Vite build) (PASS)
- `make web-e2e` (Playwright 5 tests) (PASS)
- `make check` (PASS)
- `make web-check` (PASS)
