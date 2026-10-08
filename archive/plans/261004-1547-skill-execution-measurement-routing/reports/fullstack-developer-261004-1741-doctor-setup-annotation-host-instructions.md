# Doctor CLI, setup annotation, and host instructions: implementation report

Status: done. `make check` passes: vet is clean, golangci-lint reports 0 issues, and every package passes its tests. Nothing is committed.

## Files

Created:
- `internal/app/skill_doctor.go`: `SkillDoctorService.Run`, `SkillDoctorResult`, `ErrDoctorSkillUnavailable`, `doctorFingerprint` (the one source of the doctor cache key), and `setupAnnotation`.
- `internal/app/skill_doctor_test.go`
- `internal/delivery/cli/skill_doctor.go`: human and JSON output, plus exit codes 0 (ready), 1 (setup_required or unsupported_platform), and 2 (invalid request or unknown skill).
- `internal/delivery/cli/skill_doctor_test.go`

Modified:
- `internal/delivery/cli/skill.go` dispatches `doctor`, validates its flags, and lists it in the usage strings. `internal/delivery/cli/help.go` documents it.
- `internal/app/resolver.go` adds the post-ranking `setup` to primary and supporting, and puts `setup_state` in the resolution telemetry payload. `internal/app/resolver_test.go` has the new tests.
- `internal/resolver/types.go` adds `SetupStatus` and `Setup *SetupStatus \`json:"setup,omitempty"\`` on `Recommendation` and `Supporting`.
- `schemas/skill-resolve-response-v1.schema.json` adds a shared `$defs.setup` (`additionalProperties: false`, `state` enum, `reason_codes`, `checked_at` date-time), referenced from `primary` and `supporting`. `schemas/skill-resolve-request-v1.schema.json` adds a description to `task.description`.
- `internal/hostintegration/bootstrap.go` adds the two paragraphs and keeps the v1 markers. `internal/hostintegration/integration_test.go` checks the block text and the in-place replacement.
- `internal/delivery/mcpserver/resolver_tools.go` and `skill_tools.go`: only the tool descriptions changed.
- `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`: only the text between the markers changed. `CLAUDE.md` and `GEMINI.md` are gitignored.

Two files outside the phase list, both justified:
- `internal/app/skill_snapshot.go`: `Ensure` now wraps an internal `ensure` that also returns the spec, the trust verdict, and the root. Live checks moved into a shared `runtimeProbe`. With this, the doctor reuses the exact manifest version and verdict that the preflight uses. Behavior is unchanged and all snapshot tests pass.
- `schemas/embed_test.go`: one contract test (valid and invalid cases) for the optional `setup` object.

## Carry-over requirements
- **Doctor-write to preflight-read round trip.** The doctor, `local.preflight.doctor`, and the resolver annotation all key the cache by `doctorFingerprint(DistributedSkill.Version, spec)`. That version is the same value as `LocalSkill.ManifestVersion`. `TestDoctorWritesCacheThatActivationAndResolutionRead` runs the doctor, then shows that `Ensure` and `setupAnnotation` both read the result back.
- **Trust.** `setup.check` runs only when `verdict.Trusted`. For an unreviewed third-party skill, the check shows as `SKIP ... scripts_review_required`, the setup command is never shown, and `skillhub skill review <id>` is suggested instead.
- **Env values.** Checks record presence only. Tests set a real env value and confirm it never appears in human output, JSON output, the cache file, or telemetry. Check output is limited to the last 2 KiB, shown once, and never stored. Terminal control characters are stripped from human output.
- **Host text.** The bootstrap block now tells agents to send `task.description` in English, use `local.path`, run the `local.preflight` check in `working_directory` under their own permissions, ask before running `setup`, and not run withheld scripts.

## Tests
- App: round trip; missing bin and a too-old version (detail `version 16.2.0 does not satisfy >=18`); unsupported platform (check not run); untrusted third-party skill (check skipped, telemetry payload limited to the allowlisted fields); no env values in the cache; a skill with no runtime is ready and writes nothing; unknown or invalid IDs; a real `sh` check with `exit 0` and `exit 3` (skipped on Windows).
- Resolver: `setup` is absent when there is no runtime block. The state is `unknown` (`doctor_not_run`) with no cache, `setup_required` with a missing bin, `ready` with a passing cache, and `setup_required` with a failing cache. With `setup` stripped, all of these responses are identical. Each response validates against the committed schema, and `setup_state` reaches telemetry.
- CLI: exit codes 0, 1, and 2; PASS/FAIL/SKIP lines; the setup suggestion; JSON output; env values never printed.

## Unresolved questions
- When the canonical tree is invalid, the doctor checks the catalog's last-good (fallback) generation, the same one MCP serves. This seems right, but the output does not mention it.
- `docs/user-guide.md` and `docs/curating-skills.md` do not mention `skill doctor` yet. Docs belong to the documentation phase.
