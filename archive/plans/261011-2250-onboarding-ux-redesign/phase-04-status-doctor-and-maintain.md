---
phase: 4
title: "status and doctor by role, maintain group"
status: completed
priority: P1
effort: "3d"
dependencies: [2, 3]
---

# Phase 4: status and doctor by role, maintain group

## Context

Status and doctor run different code and disagree: a fresh hub gets "Git has uncommitted changes" from status
(`internal/app/curation_home.go:489-496`) and "Workspace is healthy." from doctor (`internal/app/workspace.go:59-196`),
verified A6/A7. Status ends with "Continue normal work" and no command when nothing is pending
(`curation_home.go:528`, verified F8). Client support exists but is partial:

| Exists today | New in this phase |
|---|---|
| Remote doctor checks: token readable, reachable, token accepted, hub version, cache writable, last fetch (`internal/app/doctor_remote.go:389-480`), token sent only per `tokenMaySend` (`:429-445`) | Same checks reached by walk-up detection, in status (light subset) and in `both`; `tokenMaySend` becomes an engine invariant |
| Remote-only doctor when no hub resolves, but not with `--fix` (`internal/delivery/cli/workspace.go:126-133`, verified R3) | `doctor --fix` for clients: re-run the safe local connect merge, re-point a moved hub; refuses to reconnect a remote hub named only in project files and prints the command (S1) |
| Local client drift check for cwd and user scope, suggests `connect` (`internal/app/host-connections.go:87-112`) | Walk-up project detection, missing-hub detection, stale permission rule set (S2) |
| Doctor exits 0 even when a remote check fails (verified R2) | Exit 1 when any problem is reported |
| status takes a shared lock only inside `mutation.InspectRecovery` when a transaction directory exists, waiting up to 15 s (`internal/mutation/recovery.go:105-120`); doctor takes it always (`internal/app/workspace.go:99`, `DefaultLockTimeout` 15 s, `internal/mutation/lock.go:15`) | status never waits on the lock (F6) |
| Maintenance as top-level `validate`, `rebuild`, `migrate`, `diff` | `skillhub maintain validate|rebuild|migrate|diff` (P9), with existing pre-commit hooks detected and rewritten (F8) |

Decision 6, planner decisions P4 and P9; red-team items S1, S2, F4 (re-pointing), F6, F8, F9, F10. Depends on
`DetectRole` (Phase 2) and on serve (Phase 3) for the listener probe and the remote-doctor texts.

## Requirements

**One check engine (extension point shared with the distill-lab plan).** New `internal/app/health.go`:

```go
type Depth int // DepthLight (status) or DepthFull (doctor)
type Severity string // "ok", "notice", "problem", "skipped"
type FixAction struct {
    Summary string                                  // what --fix will do, one line
    Apply   func(ctx context.Context) (Changed, error) // Changed lists the files it wrote
}
type Finding struct {
    Code     string     // stable id, e.g. "hub_index_stale", "client_entry_outdated"
    Label    string
    Severity Severity
    Message  string     // plain language
    Fix      *FixAction // nil when doctor --fix cannot fix it
    Manual   string     // the command a person runs when Fix is nil
}
type Section struct { Kind string; Root string; Findings []Finding } // Kind "hub" or "client"
type CheckProvider interface {
    Name() string
    Applies(report RoleReport) bool
    Check(ctx context.Context, report RoleReport, depth Depth) []Finding
}
func RegisterProvider(p CheckProvider)                      // package-level registry, called from init()
func Run(ctx context.Context, report RoleReport, depth Depth) []Section
func ApplyFixes(ctx context.Context, sections []Section) FixReport // Fixed, NotFixed (with reason)
```

The existing hub doctor, host-connection check and remote doctor become providers; nothing is computed twice. The
distill-lab plan registers only notice-severity hub Findings through `RegisterProvider` (Node missing, a generated
skill file tracked by Git, an unknown file in an ignored skill folder) without touching the renderers.
Wrapper/canonical integrity of system skills does not go through `RegisterProvider`: it stays in the hostintegration
inspection items, which doctor reports in the hub section through the hub's agent connection check.

**Invariant (S1).** No provider sends the hub token to a hub unless `tokenMaySend(hubURL, entries)` is true (user scope
names that hub, or it is loopback). status, doctor and `doctor --fix` share this rule. The client-remote provider's
`FixAction` never calls `ConnectRemote` with a URL or token-file path read from project files; for such a hub `Fix` is
nil and `Manual` is `skillhub connect --hub <url> --token-file <file>` with the text from Phase 3.

**Checks.**

| Code (examples) | Check | Role | Light (status) | Full (doctor) | `--fix` |
|---|---|---|---|---|---|
| `hub_git_missing` | Git repository present | hub | yes | yes | no (re-run `init`) |
| `hub_uncommitted` | Uncommitted changes (notice, not a problem) | hub | yes | yes | no; prints the commit command for the changed hub paths, staged by name |
| `gitignore_entry_missing` | Required `.gitignore` entries (existing finding) | hub | yes | yes | yes (append) |
| `machine_file_tracked` | Git tracks a connection file (existing finding) | hub | yes | yes | no; `git rm --cached <path>` |
| `hub_schema` | Schema version | hub | yes | yes | no; points at `skillhub maintain migrate` |
| `hub_content_invalid` | Content valid | hub | yes (count only) | yes (each issue) | no |
| `hub_recovery_pending` | Pending recovery | hub | yes | yes | yes (existing roll-forward, `internal/app/workspace.go:218-226`) |
| `hub_index_stale` | Search index current | hub | yes | yes | yes (rebuild) |
| `hub_self_connection` | Hub's own agent connection files current | hub | yes | yes | yes (self-connection merge) |
| `hub_precommit_validate` | `.git/hooks/pre-commit` runs `skillhub validate` (F8) | hub | no | yes | yes (rewrite the call to `skillhub maintain validate`, keep the rest of the hook byte-identical, print the file) |
| `hub_listener` | Listener on 127.0.0.1:7420 answers (notice) | hub | no | yes, 300 ms probe with `ProbeHub` (`internal/app/doctor_remote.go:93`) | no |
| `client_entry_outdated` | Connection entry per agent present and current | client | yes | yes | yes (safe merge; refuses on conflicting managed block and says so) |
| `client_permissions_outdated` | Claude permission rules equal the current set (S2) | client, hub | yes | yes | yes (rewrite the managed rule entries in `.claude/settings.local.json`, keep user entries) |
| `client_hub_missing` | Local hub path exists and is a hub | client-local | yes | yes | yes only when the missing path is in the stored `previous_hubs` and the stored hub exists: re-point entries to it, print old and new path (F4). Stored hub itself missing: no; `cd <new-location> && skillhub init`, then `skillhub doctor --fix` in each project |
| `remote_reachable` | Remote hub reachable + token accepted | client-remote | yes, one probe, 1 s timeout | yes | no: "Skill Hub cannot start a hub on another machine" + the hub-machine command |
| `remote_token_file` | Token file present, mode 0600 | client-remote | yes | yes | no; prints the `scp` line |
| `remote_version` | Hub version compatible | client-remote | no | yes | no |
| `remote_cache` | Skill cache writable, last fetch | client-remote | no | yes | no |
| `remote_reconnect` | Client-remote entry stale and named only in project files | client-remote | yes | yes | no (S1): prints `skillhub connect --hub <url> --token-file <file>` |

**Locking (F6).** status (light) does not wait for the workspace lock: it reads what it can and, when a shared lock
cannot be taken immediately (try-lock with zero timeout in place of `InspectRecovery`'s 15 s wait), shows the notice
"The hub is busy with another change; run `skillhub status` again in a moment." doctor (full) keeps the shared lock
with the 15 s timeout. The web Home (`internal/delivery/web/routes_read.go:39`) and MCP `hub_status`
(`internal/delivery/mcpserver/workspace_tools.go:30-32`) keep calling `GetCurationHome` and never run remote probes.

**Output.** One header per section ("Hub at <path>", "This project uses the hub at <path|url>", "Your user account
uses ..."; mixed hosts get one line per host naming the winning scope), a table of findings, then:
- status: one `Next:` with exactly one runnable command chosen by the table below;
- doctor: "Healthy." or the list of problems with their fix; with `--fix`, "Fixed:" (what changed, with file paths),
  "Not fixed:" (what and why). Exit 0 healthy, 1 problems remain, 2 invalid request.
- `none`: "This folder is not a Skill Hub hub or a connected project." + the command from `DetectRole` (Phase 2).

| State (first match wins) | Next command |
|---|---|
| none, stored hub exists | `skillhub connect` |
| none, stored hub path missing | `cd <new-location> && skillhub init` |
| none, no stored hub | `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init` |
| hub busy | `skillhub status` |
| any fixable problem | `skillhub doctor --fix` |
| hub schema too old | `skillhub maintain migrate` |
| hub content invalid | `skillhub doctor` (lists each issue) |
| client-remote unreachable | the platform tunnel or `skillhub serve` hint as a command for the hub machine, labelled "on the hub machine" (exempt from the help check) |
| client-remote needs reconnect (S1) | `skillhub connect --hub <url> --token-file <file>` |
| hub uncommitted changes | `git -C <hub> add -- <changed paths> && git -C <hub> commit -m "Update skills"` on one unwrapped line, paths named (never `-A`), labelled as a Git line (exempt) |
| hub healthy, no skills | `skillhub skill create my-first-skill --collection core --name "My first skill" --description "What this skill does"` (runnable as is; it previews because `--yes` is absent; Phase 6 replaces it with `skillhub skill create`) |
| hub healthy with skills, or client-local healthy | `skillhub skill list` |
| client-remote healthy | `skillhub doctor` |

Wording of doctor and status messages is changed once, here (F12).

**JSON (F9).** `status --json` keeps `schemas/curation-home-v1.schema.json` (required keys incl. `workspace`,
`actions`, `suggested_actions`, `categories`, `home_summary`; guarded by `internal/delivery/cli/status_schema_test.go:24-27`;
consumed by `system-skills/curator/SKILL.md:112,169` and MCP `hub_status`). Role and client data are optional added
properties `role` (string) and `client_sections` (array) declared in the schema (it has `additionalProperties: false`);
no schema version bump. In a role without a local hub the required keys are present with empty values (`workspace: ""`,
empty arrays, zero `home_summary`). Only `doctor --json` uses the new combined shape `{role, sections:[{kind, root,
findings:[...]}], fixed, not_fixed}`.

**maintain group.** `skillhub maintain validate [--staged] | rebuild | migrate [--to <v>] [--yes] | diff`, same
behavior as today's commands (`internal/delivery/cli/{validate_staged,catalog,migration,curation}.go`). Top-level names
are removed. Every hint string that names them is updated (inventory row "validate, rebuild, migrate, diff" in
plan.md): Go hints in `internal/app`, `internal/canonical/skill.go:898`, `internal/catalog/open.go:51`,
`internal/delivery/cli/{catalog.go:21,migration.go:22,curation.go:113,workspace.go:85,365,next_commands.go:127-131,resolver.go:112}`;
web UI strings and their tests (`web/src/screens/home/action-cta.ts:31`, `ResourcesTab.tsx:19`,
`ResourceFileView.tsx:33,35`, `AppShell.tsx:66`, `i18n/en.ts:89`, `home.test.tsx:53`, `resource-file-view.test.tsx:141`);
web e2e calls (`web/e2e/skill-resources.spec.ts:57`, `skill-runtime.spec.ts:36`, `journeys.spec.ts:49`,
`a11y.spec.ts:42`); curator skill `SKILL.md:132,162,235-236` (both copies); `.github/workflows/evaluation.yml:46`. MCP
tool names (`workspace_validate`, `workspace_rebuild`, `workspace_diff`) do not change.

## Files

- Create: `internal/app/health.go`, `health_test.go`, providers next to the code they wrap
  (`internal/app/health_hub.go`, `health_client.go`)
- Modify: `internal/app/workspace.go` (doctor becomes a provider; `DoctorFix` dispatches through `ApplyFixes`),
  `internal/app/doctor_remote.go` (provider; invariant), `internal/app/host-connections.go` (provider, permission
  check), `internal/hostintegration/cli-permissions.go` (expose the current rule set for comparison),
  `internal/app/curation_home.go` (git notice wording, next-command fallback at `:528`, optional `role`),
  `internal/mutation/recovery.go` (try-lock variant for status), plus the hint files listed above
- Modify: `internal/delivery/cli/workspace.go` (`runDoctor`), `curation.go` (`runStatus`, `renderCurationHome`),
  `root.go` and `help.go` (`maintain`, doctor and status usage), `catalog.go`, `migration.go`, `validate_staged.go`
- Modify: `schemas/curation-home-v1.schema.json` (two optional properties)
- Modify: web files listed above; `system-skills/curator/SKILL.md` and `internal/systemskills/curator/SKILL.md`
- Tests to update: `internal/delivery/cli/{onboarding,ux_behavior,status_schema,curation,catalog,migration,validate_staged,doctor-safety,workspace_resolve,workspace_next_steps,remote_commands,human_output_width}_test.go`,
  `internal/app/{workspace,doctor_remote,curation_home,curation_first_run,migration}_test.go`,
  `internal/delivery/web` golden for Home if the summary text changes, web unit tests above

## Steps

1. Define the types and registry; wrap the existing hub doctor, host-connection and remote doctor logic as providers;
   keep `tokenMaySend` in the remote provider and assert it in a test before anything else changes.
2. Add client-local checks (missing hub path with `previous_hubs`, outdated entry, permission rule set), walk-up
   detection and the pre-commit hook check.
3. Implement `--fix` per role through `ApplyFixes` on top of `connect`'s safe merge and the existing hub repair.
4. Status renderer and next-command table; doctor renderer and exit codes; `status --json` optional fields;
   `doctor --json` shape; status try-lock.
5. `maintain` group and hint strings (Go, web, e2e, curator skill, workflow).
6. Repo-wide grep gate for `skillhub validate|rebuild|migrate|diff` outside `maintain` (same paths as Phase 1).

## Todo

- [x] Engine `internal/app/health.go`: `Depth`, `Severity`, `Changed`, `FixAction`, `Finding` (plus optional `Reason`
      and `Detail`), `Section`, `CheckProvider`, `RegisterProvider`, `Run`, `ApplyFixes`/`FixReport`, `StatusNext`,
      `WithProbeClient`; registered providers add to the hub section; a panicking provider becomes a problem finding
- [x] Providers: hub (`health_hub.go`, one shared inspection with `WorkspaceService.Doctor`), client-local and
      client-remote (`health_client.go`); `tokenMaySend` kept in the remote checks and asserted by tests (app and CLI)
- [x] Client-local: missing hub re-pointed only to the stored hub when the missing path is in `previous_hubs`; outdated
      entries and permission rules fixed with connect's safe merge; `--workspace` keeps only connections to that hub
- [x] Pre-commit hook check (doctor only) and `--fix` rewrite of `skillhub validate` to `skillhub maintain validate`
- [x] `doctor --fix` through `ApplyFixes` over up to three rounds, re-detecting the role; "Fixed:" with files and
      "Not fixed:" with reason and command; client-remote never fixed (`RemoteFixRefusal`); a tracked connection file
      no longer blocks the hub repair
- [x] status: light depth, try-lock (`mutation.TryAcquireSharedLock`, `CurationService{NoWait: true}`), one `Next:` per
      the table; exit 2 with the role message in a folder without a role
- [x] doctor exit codes 0/1/2; `doctor --json` combined shape; `status --json` on curation-home-v1 with optional
      `role` and `client_sections` declared in the schema
- [x] `maintain validate|rebuild|migrate|diff`; top-level names removed; Go, web UI, web e2e, curator skill (both
      copies), evaluation workflow and remote guide hints renamed; grep gate clean (only the hook check names the old
      call on purpose)
- [x] `go test ./...`, `go vet` (linux, windows, darwin) and `gofmt -l` clean; web unit tests and e2e not run here (no
      `web/node_modules`)

Report: [cook-261011-onboarding-phase4](../../../plans/reports/cook-261011-onboarding-phase4.md)

## Validation

- Role fixtures (real temp folders, isolated HOME and `SKILLHUB_USER_CONFIG_DIR`): hub fresh, hub dirty, hub stale
  index, hub missing `.gitignore` entries, hub tracking `.mcp.json`, hub with a `skillhub validate --staged` pre-commit
  hook (fix rewrites it, other lines untouched), hub + other hub entry (`both`), client-local healthy, client-local with
  moved hub recorded in `previous_hubs` (fix re-points), client-local whose missing path is not recorded (fix refuses,
  prints `skillhub connect <stored>`), moved hub with stale stored path (prints `cd <new-location> && skillhub init`),
  client with an old permission rule set (fix updates, user rules kept), client-remote down (cannot fix, exit 1),
  client-remote in a subfolder, mixed project, user scope only, none with/without stored hub.
- S1: a project-only remote entry pointing at an `httptest` hub that records the `Authorization` header: status,
  doctor and `doctor --fix` never send the token; `doctor --fix` exits 1 and prints the connect command.
- For each fixture: status and doctor report the same problems (assert by finding codes); every `skillhub` segment of
  status `Next:` passes `checkGuideCommand` through the Phase 1 extractor; doctor exit code matches.
- F6: status while another goroutine holds the exclusive workspace lock returns within 1 s with the busy notice.
- `status --json` validates against the schema in every fixture; `doctor --json` has the new shape.
- Remote fixtures use an `httptest` hub; no real network.
- `go test ./internal/app/... ./internal/delivery/...`; `make check`; `make web-e2e`; `npm test` in `web/`.
- Windows: the `git -C ... add ... && ... commit` next command uses the PowerShell form (`;`) on Windows; CI checks.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Token sent to a hub named in a cloned project | Low x Critical | Engine invariant + test (S1) |
| Status becomes slow with remote probes or a held lock | Medium x Medium | Light depth: one probe, 1 s timeout, skipped when no remote entry; try-lock |
| `doctor --fix` re-points entries to the wrong hub | Low x High | Re-point only paths recorded in `previous_hubs` when the stored hub exists; print old and new path |
| Hook rewrite damages a user's hook | Low x Medium | Replace only the `skillhub validate` token sequence; print the file; doctor without `--fix` changes nothing |
| `status --json` or web Home contract changes by accident | Low x High | Schema test, golden `home.json`, `GetCurationHome` keeps its shape |

## Rollback

Revert the phase commits; `maintain` reverts to top-level commands together with the hint strings in the same commits.
A rewritten pre-commit hook keeps working with the new binary only; the release note tells users who downgrade to run
`skillhub validate` in their hook again.
