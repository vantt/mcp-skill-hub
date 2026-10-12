# Phases 4 and 5 implementation report: installer layout, per-item blocking, ignore entries

Branch `feat/distill-lab-bundle-and-script`, worktree `/home/vantt/projects/mcp-skill-hub-distill`.

## Commits

1. `fix(systemskills): recognize the curator versions committed with the onboarding redesign`: the digest
   collection loop from `known-digests.go` rerun after the rebase; two new curator digests (the onboarding
   curator text, which is also the current bundle).
2. `feat(hostintegration): install system skills once under .agents/skills with a Claude Code wrapper`.
3. `feat(doctor): hint when distill-lab cannot find Node 18 or newer`.
4. `feat(workspace): keep generated system skill files out of Git through the hub .gitignore`.

## Re-verify against the merged onboarding main (what changed in the plan's citations)

- `internal/app/host-connections.go` and `applyExternalConnections` no longer exist. `DoctorFix` repairs only the
  hub; project and user connections are repaired by the health engine (`ApplyFixes` runs every `FixAction`
  independently). So the "DoctorFix gate" item reduces to: `applyHostIntegration` never changes the status for
  blocked items, so `repairHub` still succeeds and the client fixes run. Tested by
  `TestBlockedHubFileDoesNotStopTheProjectRepair`.
- Conflict pre-checks now live in `internal/app/connect.go`, `connect_remote.go` (`planScope`),
  `health_hub.go` (`inspectHubWhileLocked`) and `health_client.go` (`localGroupFindings`). They keep reading
  only `FileState.Conflict`; skill files never set it.
- Extension point names match (`health.go`: `CheckProvider`, `Finding`, `SeverityNotice`, `DepthFull`,
  `RegisterProvider`). The summary host list is `connectionSummaryLine` in `internal/delivery/cli/workspace.go`.
- The wrapper tag test is `TestKnownWrappersCoverReleasedRenders` (matches the CI `-run 'CoverReleased'` step).

## Design as built

- Adapters: `SkillsDirRelativePath`, `Wrapper` (Claude), `LegacySkillsDirRelativePath` (Gemini `.gemini/skills`,
  cleanup only). New kind `ChangeSharedSkill`, host-less, in `Inspection.Shared`; `PlanResult.Blocked`
  (`FileState.Blocked` holds the reason, `FileState.Host` is set) and `PlanResult.Notices`.
- `changeRank` replaces `changeOrder` in `buildPlan` (stable sort) and `Apply`: MCP 1, permissions 2, shared
  write 3, native skill 4, shared removal 5, bootstrap 6. `validateChangePath`/`expectedChange` check skill
  changes against `plannedSystemSkills` (hub-only distill-lab only when `Root == Workspace` locally, only with
  the hub marker for remote/remove); the Gemini legacy path is accepted only as a removal of owned bytes.
- Ownership: shared files by `KnownFileDigest`; wrappers by current render or `known-wrappers.go` (empty map:
  no release shipped a wrapper yet), plus known full copies (old layout); never by the description in the file.
  `SystemSkillFileOwned(relPath, raw)` is exported (the `root` parameter from the plan was not needed).
- Blocked files (unknown content or a symlink component) block only themselves; a blocked shared file
  withdraws that id's owned wrapper. Consumers: items with impact `Left unchanged: ...` and a
  `system_skill_blocked` warning (connect, connect --hub/--remove, init, doctor --fix); doctor's hub and
  local-client sections get a `system_skill_blocked` problem finding without a fix; `MissingFinding` returns
  `host_integration_blocked` after the missing case. The remote-client check ignores blocked files (a user's
  own file in a remote-connected project was silently left before, and still is).
- Remote/remove: wrappers and legacy copies of the selected hosts are removed when owned; shared files only
  when no unselected host keeps a local registration (`canonicalKeeper` reads every adapter's config); a
  `shared_skill_kept` notice names the limit ("Codex still sees system-curator in .agents/skills while Claude
  Code in this folder uses a local hub.").
- Item IDs: skill items are `host_<host>_<kind>:<rel path>`, shared ones use the key `agents` and level
  `shared`; one CLI parser `parseHostItemID`; `formatChangeKind` prints `<id> skill`; the summary line skips
  `agents`; the shared folder prints as "Agent Skills folder (.agents/skills)".
- Doctor provider `systemSkillProvider` (`internal/app/system_skill_doctor.go`), `Applies` when
  `HealthHub() != ""`, runs only at `DepthFull`: `distill_lab_node_missing`, `system_skill_tracked`,
  `system_skill_unowned_ignored`, all notices without a fix.
- Four `requiredIgnoreEntries` appended; `machine_files_test.go` count made relative to the list.

## Deviations to note

- Phase 4 asked for three commits; the per-item blocking and the layout change share every consumer, so they
  are one commit.
- The tracked and unowned notices print `git -C '<hub>' rm --cached -- <path>` / `git -C '<hub>' add -f --
  <path>` (the pattern of `machine_file_tracked`), so the command runs from a project folder too.
- The connect line verb for the shared folder reuses the host verbs ("will connect", "removed").
- `connect --hub` for a symlinked `<root>/.claude` still fails on the Claude permissions file (owned by the
  onboarding plan); the per-item symlink refusal is tested for `.agents` in user and project scope.

## Validation

- `gofmt -l` clean; `go vet ./...` and `GOOS=windows go vet ./...` pass.
- `GIT_CONFIG_NOSYSTEM=1 SKILLHUB_REQUIRE_TAGS=1 go test -count=1 ./...` pass.
- `make lint LINT_BASE=main`: 0 issues.
- Smoke run with the built binary in a scratch HOME: `init` writes both skills (canonical + wrappers), `git
  status` clean after the first commit; `connect` in a project writes the curator only and prints the shared
  line; `doctor` healthy; `connect --remove` deletes everything it wrote.

## Open points

- Release notes (Phase 7): skill item ID change, unknown content now left unchanged, `host_integration_missing`
  and `gitignore_entry_missing` after the upgrade, Gemini CLI v0.28.0 note.
- `known-wrappers.go` gets its first digests after the first release that ships the wrappers.
