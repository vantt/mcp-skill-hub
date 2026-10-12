# Patch group: conformance and small fixes (2026-10-12)

Branch: `worktree-agent-af2b016677ff00778` (from origin/main de31b9f). Nothing pushed or tagged; nothing written to `/home/vantt/skill-hub`.

## Done

| Item | Change |
|---|---|
| R4 | `internal/catalog/build.go`: the pointer temp file is now filled by `fillPointerFile`, which returns the first Chmod, Write, injected-fault or Sync error, so an empty pointer is never renamed into place. Test `pointer_file_test.go` (closed file, read-only file, injected `FaultPointerTempSync`). `BenchmarkMCPToolList` now compares against `len(expectedToolAnnotations())` (26) and runs. |
| D7 | `serverInfo.version` is `version.Current().Version` for the server and the proxy (`server.go`, `proxy.go`). Test `TestServerReportsTheBuildVersion`. Live check: `skillhub serve` answered `"version":"dev"`. |
| R3 | `New` registers the skills methods and declares the extension only when the profile is not curation. Test `TestSkillsExtensionIsServedOnlyByRuntimeAndAllProfiles` (runtime, all serve; curation has no capability and `skills/list` returns method not found). `curator_boundary_test.go` now expects the curation `skills/list` to fail. |
| R14 | `skills/list` drops skills whose `RemoteStatus` is `review_required` (no snapshot export, trust verdict only). `skills/get` and `resources/read` unchanged. Tests: listing omits the skill, `resources/read` still refuses, `skills/get` still returns `review_required`. Two existing tests that fetched a review-required skill through the listing now build the entry from `DistributionService.LookupSkills` (`heldSkillEntry`). Documented in `docs/design/02-agent-hub-protocol.md`. |
| R9 | The publish step in `build.go` stats the generation file before and after the final `integrity_check`; if the identity is unchanged it seeds `integrityVerified`. Existing tests pass; new `TestIntegrityIsSeededByTheBuildThatCheckedTheFile`; the "recently written file" test now touches the file first (changed identity is still checked every time). |
| R10 / G3 | Conformance run, see below; matrix `unverified_checks` updated in both copies (`docs/mcp-compatibility-matrix.json` and `internal/hostintegration/matrix.json`, a test keeps them equal). |
| G4 | Example URI in protocol doc is now `skill://skillhub/<digest>/consumer-reliability-review/SKILL.md`. |
| D8 | `skill_resolve` description: "This does not load skill content: load the recommended skill with skill_get, or, only when your host supports the MCP skills extension, with skills/get and resources/read." |
| R7 | One note in the protocol doc: `skills/bundle` is a vendor method inside the extension namespace and will be renamed if the standard defines another bundle. Wire name unchanged. |

Cost note for R14: listing now evaluates trust per active skill (one catalog open each). The parallel load-path work may want to batch this.

## R10 conformance result (honest)

Setup: `go install` of this branch into a scratch GOBIN; isolated HOME, XDG_*, TMPDIR and `SKILLHUB_USER_CONFIG_DIR`; temp hub created with `skillhub init` plus one fixture skill; `skillhub serve --addr 127.0.0.1:17420`. The runner cannot send headers and the hub needs a token, so a 15-line Node forwarding proxy added the token and rewrote Host. Processes stopped afterwards. npm and network worked.

- `@modelcontextprotocol/conformance@0.2.0-alpha.12` (has skills scenarios `sep-2640-skills-directory|enumeration|manifest`):
  - directory: passed (directory checks skipped, directoryRead false).
  - enumeration and manifest: not exercised. The runner speaks 2026-07-28 for them and the stateful HTTP listener answers `-32022 protocol version "2026-07-28" is not supported` (go-sdk v1.8.0 stateful handler only supports 2025-11-25 and older). The extension capability checks passed (declared inline, resources capability present); `skills/list` checks were failures/skips for that reason, not for a content defect. Forcing 2025-11-25 is refused by the runner (extension scenarios are not on that timeline).
  - Also at 2026-07-28: `server-stateless`, `caching`, `http-header-validation` fail for the same reason, plus the listener not being stateless.
- `0.1.16` active suite: 8 of 30 passed (server-initialize, logging-set-level, ping, tools-list, server-sse-multiple-streams, resources-list, prompts-list, dns-rebinding-protection); 22 fail because they require the runner's own fixture tools, prompts and resources, which this hub does not serve. The dns-rebinding pass is not evidence because the proxy rewrote Host.

Conclusion: no skills conformance is claimed. To get a real skills result, the HTTP listener would need a stateless mode for the modern protocol (a design decision for the user; not changed here). Matrix status is `attempted_not_passing`.

## Proposed CLAUDE.md / connect snippet wording (not changed, for approval)

Current line in the generated Skill Hub block: "If a skill call returns `snapshot_expired` with `current_uri`, the skill was updated; call `skills/get <current_uri>` to reload it."

Proposed: "If a skill call returns `snapshot_expired` with `current_uri`, the skill was updated; call `skill_get` for that skill again. Only a host that supports the MCP skills extension reloads with `skills/get <current_uri>`."

Optionally add one line before it: "Load a recommended skill with `skill_get`; use `skills/get` and `resources/read` only if your host supports the MCP skills extension."

## Verification

`gofmt -l .` clean; `go vet ./...` plus GOOS=windows and darwin clean; `make lint` 0 issues; `go test -count=1 ./...` all pass.

Status: DONE_WITH_CONCERNS
Summary: Items 1-9 implemented and committed on the branch (4 code commits, 1 docs commit, plus this report and the matrix update); conformance run recorded as attempted, not passing.
Concerns: skills conformance could not run because the stateful HTTP listener rejects protocol 2026-07-28; R14 adds a per-skill trust evaluation to `skills/list`; the snippet wording above awaits approval.
