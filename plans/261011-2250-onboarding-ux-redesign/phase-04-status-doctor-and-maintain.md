---
phase: 4
title: "status and doctor by role, maintain group"
status: pending
priority: P1
effort: "1.5d"
dependencies: [2, 3]
---

# Phase 4: status and doctor by role, maintain group

## Context

Status and doctor run different code and disagree: a fresh hub gets "Git has uncommitted changes" from status
(`internal/app/curation_home.go:490-495`) and "Workspace is healthy." from doctor (`internal/app/workspace.go:63-136`),
verified A6/A7. Status ends with "Continue normal work" and no command when nothing is pending (`curation_home.go`
`SuggestedActions` fallback, verified F8). Client support exists but is partial:

| Exists today | New in this phase |
|---|---|
| Remote doctor checks: token readable, reachable, token accepted, hub version, cache writable, last fetch (`internal/app/doctor_remote.go:389-480`) | Same checks reached by walk-up detection, in status (light subset) and in `both` |
| Remote-only doctor when no hub resolves, but not with `--fix` (`internal/delivery/cli/workspace.go:126-133`, verified R3) | `doctor --fix` for clients: re-run the safe connect merge, re-point a moved hub; says an unreachable remote cannot be fixed |
| Local client drift check for cwd and user scope, suggests `connect` (`internal/app/host-connections.go:87-112`) | Walk-up project detection, missing-hub detection, old `mcp serve` entries reported as outdated |
| Doctor exits 0 even when a remote check fails (verified R2, backlog F15) | Exit 1 when any problem is reported |
| Maintenance as top-level `validate`, `rebuild`, `migrate`, `diff` | `skillhub maintain validate|rebuild|migrate|diff` (P9) |

Decision 6, planner decision P9. Depends on `DetectRole` (Phase 2) and on serve (Phase 3) for the listener probe.

## Requirements

**One check engine.** New `internal/app/health.go`: `Checks(ctx, report RoleReport, depth Depth) []Section` where a
`Section` is `hub` or `client` with ordered `Check{ID, Label, Status ok|problem|notice|skipped, Detail, Fix, Fixable}`.
`status` uses `DepthLight`, `doctor` uses `DepthFull`. The existing doctor, remote doctor and host-connection code
become check providers; nothing is computed twice.

| Check | Role | Light (status) | Full (doctor) | `--fix` |
|---|---|---|---|---|
| Git repository present | hub | yes | yes | no (re-run `init`) |
| Uncommitted changes (notice, not a problem) | hub | yes | yes | no; prints the commit command for the hub's own paths |
| Schema version | hub | yes | yes | no; points at `skillhub maintain migrate` |
| Content valid | hub | yes (count only) | yes (each issue) | no |
| Pending recovery | hub | yes | yes | yes (existing roll-forward, `internal/app/workspace.go:212-220`) |
| Search index current | hub | yes | yes | yes (rebuild) |
| Hub's own agent connection files current | hub | yes | yes | yes (self-connection merge) |
| Listener on 127.0.0.1:7420 answers (notice) | hub | no | yes, 300 ms probe with `ProbeHub` | no |
| Connection entry per agent present and current | client | yes | yes | yes (safe merge; refuses on conflicting managed block and says so) |
| Local hub path exists and is a hub | client-local | yes | yes | yes when the stored hub exists: re-point entries to it; else no, prints `init` |
| Remote hub reachable + token accepted | client-remote | yes, one probe, 1 s timeout | yes | no: "Skill Hub cannot start a hub on another machine" + the existing fix line |
| Token file present, mode 0600 | client-remote | yes | yes | no; prints the `scp` line |
| Hub version compatible | client-remote | no | yes | no |
| Skill cache writable, last fetch | client-remote | no | yes | no |

**Output.** One header per section ("Hub at <path>", "This project uses the hub at <path|url>", "Your user account
uses ..."), a table of checks, then:
- status: one `Next:` with exactly one runnable command chosen by the table below;
- doctor: "Healthy." or the list of problems with their fix; with `--fix`, "Fixed:" (what changed, with file paths),
  "Not fixed:" (what and why). Exit 0 healthy, 1 problems remain, 2 invalid request.
- `none`: "This folder is not a Skill Hub hub or a connected project." + the command from `DetectRole` (Phase 2).

| State (first match wins) | Next command |
|---|---|
| none, stored hub exists | `skillhub connect` |
| none, no stored hub | `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init` |
| any fixable problem | `skillhub doctor --fix` |
| hub schema too old | `skillhub maintain migrate` |
| hub content invalid | `skillhub doctor` (lists each issue) |
| client-remote unreachable | the platform tunnel or `skillhub serve` hint as a command for the hub machine, labelled so |
| hub uncommitted changes | `git -C <hub> add <hub-owned paths> && git -C <hub> commit -m "Update skills"` on one unwrapped line |
| hub healthy, no skills | `skillhub skill create <id> ...` with today's working flag set (Phase 6 replaces it with the guided form) |
| hub healthy with skills, or client-local healthy | `skillhub skill list` |
| client-remote healthy | `skillhub doctor` |

**JSON.** `status --json` and `doctor --json` share one shape: `{role, sections:[{kind, root|hub, checks:[...]}],
next}`; `status_schema_test.go` is updated, and the web Home (`internal/delivery/web/testdata/golden/home.json`)
keeps reading `GetCurationHome`, which stays as the hub-section provider.

**maintain group.** `skillhub maintain validate [--staged] | rebuild | migrate [--to <v>] [--yes] | diff`, same
behavior as today's commands (`internal/delivery/cli/{validate_staged,catalog,migration,curation}.go`). Top-level
names are removed. Every hint string that names them is updated: `internal/app/workspace.go:128,186,188`,
`internal/app/migration.go:86`, `internal/app/operations.go:53,69`, `internal/app/metadata_lint.go:72`,
`internal/app/skill_resource.go:124`, `internal/app/skill_lifecycle.go:348`, `internal/app/skill_review.go:613`,
`internal/app/curation_home.go:572`, `internal/delivery/cli/next_commands.go`, `system-skills/curator/SKILL.md:132,162,235`
(and its embedded copy), `.github/workflows/evaluation.yml:46`. MCP tool names (`workspace_validate`,
`workspace_rebuild`, `workspace_diff`, `internal/delivery/mcpserver/workspace_tools.go`) do not change.

## Files

- Create: `internal/app/health.go`, `health_test.go`
- Modify: `internal/app/workspace.go` (doctor becomes a provider; `DoctorFix` dispatches by role),
  `internal/app/doctor_remote.go`, `internal/app/host-connections.go`, `internal/app/curation_home.go` (git notice
  wording, next-command fallback), plus the hint files listed above
- Modify: `internal/delivery/cli/workspace.go` (`runDoctor`), `curation.go` (`runStatus`, `renderCurationHome`),
  `root.go` and `help.go` (`maintain`, doctor and status usage), `catalog.go`, `migration.go`, `validate_staged.go`
  (move under `maintain`)
- Modify: `system-skills/curator/SKILL.md` and `internal/systemskills/curator/SKILL.md`
- Tests to update: `internal/delivery/cli/{onboarding,ux_behavior,status_schema,curation,catalog,migration,validate_staged,doctor-safety,workspace_resolve,workspace_next_steps,remote_commands,human_output_width}_test.go`,
  `internal/app/{workspace,doctor_remote,curation_home,curation_first_run,migration}_test.go`,
  `internal/delivery/web` golden for Home if the summary text changes

## Steps

1. Define `Section`/`Check` and wrap the existing hub doctor, host-connection and remote doctor logic as providers.
2. Add client-local checks (missing hub path, outdated entry incl. old `mcp serve` args) and walk-up detection.
3. Implement `--fix` per role on top of `connect`'s safe merge and the existing hub repair.
4. Status renderer and next-command table; doctor renderer and exit codes; shared JSON.
5. `maintain` group and hint strings; curator skill.

## Validation

- Role fixtures (real temp folders, isolated HOME): hub fresh, hub dirty, hub stale index, hub + other hub entry
  (`both`), client-local healthy, client-local with moved hub and stored hub (fix re-points), client-local with old
  `mcp serve` entry (fix rewrites to `mcp stdio`), client-remote down (fix says cannot fix, exit 1), client-remote in a
  subfolder, user scope only, none with/without stored hub.
- For each fixture: status and doctor report the same problems (assert by check IDs); status `Next:` passes
  `checkGuideCommand` (`internal/delivery/cli/remote_guide_doc_test.go:52`); doctor exit code matches.
- Remote fixtures use an `httptest` hub; no real network.
- `go test ./internal/app/... ./internal/delivery/...`; `make check`.
- Windows: the `git -C ... add ... && ... commit` next command uses the PowerShell form (`;`) on Windows; CI checks.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Status becomes slow with remote probes | Medium x Medium | Light depth: one probe, 1 s timeout, skipped when no remote entry |
| `doctor --fix` re-points entries to the wrong hub | Low x High | Re-point only when the old path is missing and a stored hub exists; print old and new path |
| Web Home or MCP status contract changes by accident | Low x High | `GetCurationHome` keeps its shape; golden `home.json` and `status_schema_test.go` guard it |

## Rollback

Revert the phase commits; `maintain` reverts to top-level commands together with the hint strings in the same commits.
