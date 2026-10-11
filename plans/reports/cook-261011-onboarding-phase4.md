# Onboarding UX redesign, phase 4 (status and doctor by role, maintain group): implementation report

Branch `worktree-agent-a5c101367df3589e1`, 2026-10-12. Plan:
[plan.md](../261011-2250-onboarding-ux-redesign/plan.md),
[phase-04](../261011-2250-onboarding-ux-redesign/phase-04-status-doctor-and-maintain.md). Status: completed.

## What changed

Commits on the branch (nothing pushed):

| Commit | Change |
|---|---|
| `9a65e3e` feat(mutation) | `TryAcquireSharedLock` and `TryInspectRecovery`: return `ErrWorkspaceBusy` at once instead of waiting 15 s. |
| `d954d2f` feat(cli)! | The check engine (`internal/app/health.go`), the hub provider (`health_hub.go`) and the client providers (`health_client.go`); `status` and `doctor` rewritten on it (`internal/delivery/cli/status_doctor.go`); `doctor --fix` through `ApplyFixes`; exit codes; `status --json` optional `role`/`client_sections` (schema updated); `doctor --json` combined shape; status/home wording; doctor and status help. `host-connections.go` removed (its checks became the client-local provider). |
| `1b37027` feat(cli)! | `skillhub maintain validate|rebuild|migrate|diff`; top-level names removed; hint strings renamed in Go, web UI and its tests, web e2e, curator skill (both copies), evaluation workflow, remote hub guide. |
| `303621d` feat(cli) | Optional `Finding.Detail`: doctor prints the managed-block previews a fix rewrites (as the old report did); change paths named relative to their folder; wrapped "Not fixed" reasons. |
| `42bcab7` fix(app) | Invalid-workspace, recovery and migration fix texts say `skillhub doctor` in the hub instead of `--workspace <path>` placeholders. |
| docs(plans) | Coordination row and phase-04 corrected for the distill-lab session (see below), phase status, this report. |

## Check extension point (exact exported names, for the distill-lab session)

Package `github.com/vantt/mcp-skill-hub/internal/app`, file `internal/app/health.go`:

```go
type Depth int
const (
    DepthLight Depth = iota // status: never waits for the workspace lock, at most one remote probe (1 s)
    DepthFull               // doctor: every check, waits up to the lock timeout
)

type Severity string
const (
    SeverityOK      Severity = "ok"      // passed; shown so the table is complete
    SeverityNotice  Severity = "notice"  // worth knowing; doctor still exits 0
    SeverityProblem Severity = "problem" // doctor exits 1 until fixed
    SeveritySkipped Severity = "skipped" // could not run; Message says why
)

type Changed []string // absolute paths of the files a fix wrote

type FixAction struct {
    Summary string                                      // one line, e.g. "rebuild the search index"
    Apply   func(ctx context.Context) (Changed, error) // must be safe when the problem is already gone
}

type Finding struct {
    Code     string    // stable id, e.g. "hub_node_missing"
    Label    string    // table label, e.g. "Node.js"
    Severity Severity
    Message  string    // plain language
    Fix      *FixAction // nil when doctor --fix cannot fix it; findings sharing one *FixAction run it once
    Manual   string    // the one command a person runs when Fix is nil (empty when there is none)
    Reason   string    // why doctor --fix leaves it to a person (printed under "Not fixed:")
    Detail   string    // optional longer evidence; doctor prints it, status does not
}
// JSON: {"code","label","severity","message","fix"(= Fix.Summary),"manual","reason","detail"}; empty ones omitted.

const (SectionHub = "hub"; SectionClient = "client")
type Section struct { Kind, Root, Title string; Findings []Finding } // json: kind, root, title, findings

type CheckProvider interface {
    Name() string                                                  // unique
    Applies(report RoleReport) bool                                // return false when report.HealthHub() == ""
    Check(ctx context.Context, report RoleReport, depth Depth) []Finding
}

func RegisterProvider(provider CheckProvider) // call from init(); panics on nil or a name already used
func Run(ctx context.Context, report RoleReport, depth Depth) []Section
func ApplyFixes(ctx context.Context, sections []Section) FixReport

type FixReport struct { Fixed []FixedItem; NotFixed []NotFixedItem }          // json: fixed, not_fixed
type FixedItem struct { Codes []string; Summary string; Changed Changed }     // json: codes, summary, changed
type NotFixedItem struct { Code, Message, Reason, Manual string }             // json: code, message, reason, manual

func HasProblems(sections []Section) bool
func ProblemCodes(sections []Section) []string
func StatusNext(report RoleReport, sections []Section, inventory HubInventory) NextStep
func WithProbeClient(ctx context.Context, client *http.Client) context.Context // test seam for remote probes
func (report RoleReport) HealthHub() string  // the hub folder the hub section is about
func (report RoleReport) HealthRole() string
func (report RoleReport) WithWorkspace(root string) RoleReport
```

Contract for a registered provider: its findings go into the hub section (the section of `report.HealthHub()`),
after the built-in hub checks; `Check` must only read and never send the hub token; a panic becomes a problem finding
`<name>_check_failed` instead of stopping doctor. A notice-only provider never changes doctor's exit code. Use
`depth == app.DepthLight` to skip slow work in status. Wrapper/canonical integrity of system skills is not a
provider: it stays in the hostintegration inspection items, reported by the hub's `hub_self_connection` check
(plan.md Coordination row and phase-04 corrected accordingly).

## Behavior

- **Sections.** Hub section first ("Hub at <path>": the explicit `--workspace`, else the hub around the current
  folder, else the local hub a project entry, then a user entry, points at), then one client section per scope and
  hub ("This project uses the hub at ...", "This project (<root>) uses ..." when found by walking up, "Your user
  account uses ..."), with one `client_host` row per agent naming the entry and which scope wins.
- **Hub codes:** `hub_git_missing`, `hub_busy`, `hub_unreadable`, `hub_schema`, `gitignore_entry_missing`,
  `machine_file_tracked`, other layout ids (`schema_version_missing`, `directory_collision`, ...),
  `hub_content_invalid` (light: one with a count; full: one per issue), `hub_recovery_pending`, `hub_index_stale`,
  `hub_self_connection`, `client_permissions_outdated`, `hub_connection_elsewhere` (notice), `hub_uncommitted`
  (notice with the Git line), `hub_precommit_validate` and `hub_listener` (doctor only).
- **Client codes:** `client_host`, `client_hub_missing`, `client_entry_outdated`, `client_permissions_outdated`;
  remote: `remote_used_by`, `remote_token_file`, `remote_reachable`, `remote_token_accepted`, `remote_version`,
  `remote_cache`, `remote_last_fetch` (the last three doctor only), `remote_reconnect`.
- **Token invariant.** The remote checks keep `tokenMaySend` (user scope names the hub, or loopback). No client-remote
  finding has a fix; `remote_reconnect` (connection files out of date) carries `RemoteFixRefusal(url, tokenFile)` as
  its reason for a hub only the project names, so `doctor --fix` prints the explicit connect command.
- **Locks.** status: `TryAcquireSharedLock` in the hub check and `CurationService{NoWait: true}` (one try-lock held for
  the whole curation read); busy gives the notice and `Next: skillhub status`. doctor: shared lock, 15 s.
- **Exit codes.** doctor 0 healthy, 1 problems remain, 2 invalid request (also in a folder with no role). status 0,
  2 in a folder with no role (message and the command on stderr; `--json` still prints the schema shape).

## Decisions where the plan was silent

1. **A tracked connection file does not block `doctor --fix`** (open question from phase 2, decided by the lead):
   `machine_file_tracked` is a problem with `Manual: git -C <hub> rm --cached -- <path>` and every other fix runs.
   `blocksRemediation` no longer depends on init.
2. **One inspection, two views.** `inspectHubWhileLocked` gathers layout, validation, recovery, catalog and host
   state once; `WorkspaceService.Doctor` (kept for its app-level callers) and the hub provider both derive from it.
   The index and the hub's own connection are checked only when the layout has no finding (a missing `.gitignore`
   entry already makes the index read as stale); they show as `skipped` otherwise.
3. **One hub repair.** All fixable hub findings share one `FixAction` that runs the existing `DoctorFix`; changed files
   are read from its result items. `DoctorFix` no longer repairs project/user connections (the client provider does,
   through `ConnectService.Connect`, the safe merge).
4. **`doctor --fix` runs up to three rounds**, re-detecting the role between rounds, never the same finding twice: a
   re-pointed project reaches its moved hub, whose own connection is then repaired in the same run. Problems still
   found are listed under "Not fixed" ("tried to ..., but the check still finds the problem").
5. **Hub-folder entries that point at another existing hub or a remote hub are left alone** (`hub_connection_elsewhere`
   notice, hub repair limited to the other hosts through an unexported `WorkspaceService.hosts`), so the hub repair
   and the client fix cannot undo each other in the `both` role. An entry naming a folder that is no hub (moved hub)
   is treated as the hub's own stale connection and repaired.
6. **`--workspace` narrows the report** (`RoleReport.WithWorkspace`): local entries that point at another hub are
   dropped, remote entries kept (reported, never rewritten). This keeps `doctor --fix --workspace X` from touching
   other hubs' connections (existing safety tests) and keeps tests run from inside the repository away from the main
   checkout's entries. Printed commands repeat `--workspace <hub>` when it was given.
7. **`doctor --fix --dry-run` exits 0** (a preview, like the other onboarding `--dry-run` commands and the existing
   dry-run test); it prints the sections, "Would fix" and "Not fixed".
8. **status in a folder with no role exits 2** (existing tests require a non-zero exit, as for every command that needs
   a hub); doctor there is an invalid request (2) naming the command from the next-step table.
9. **First-skill next command** is `skillhub skill create --id my-first-skill --collection core --name "My first skill"
   --description "What this skill does"`: the plan's positional form fails the help check (the checker treats a
   positional id as a subcommand word), `--id` is documented and keeps the id.
10. **Status JSON `suggested_actions`** holds the one next command (same as the human `Next:`); the overall `status`
    becomes `action_required` when a section has a problem or the hub is busy. Hub findings are not added to
    `status --json` (only `role` and `client_sections`, as the plan says); `doctor --json` has them all.
11. **Commit command** for uncommitted hub changes names the changed top-level folders and files (exact path under
    `.skillhub`), never `-A`, in the platform chain form (`;` in PowerShell); the message is quoted with
    `shellquote.Arg` (single quotes).
12. **Pre-commit hook** path comes from `git rev-parse --git-path hooks/pre-commit` (honours `core.hooksPath` and
    worktrees); the rewrite replaces only `skillhub[.exe] validate` (word boundary), keeps every other byte and the
    file mode (temp file + rename).
13. **Listener probe** is a notice when something answers on `127.0.0.1:7420`, an ok row otherwise; tests use
    `WithProbeClient` with a dialer that refuses that address so a running `skillhub serve` cannot change output.
14. **Curation work not covered by health checks** (sources to distill, upstream updates, lessons) stays visible in
    status under "Waiting" with its command; it is not the `Next:` (the table has no row for it).
15. **Home wording:** "The hub has changes that are not committed to Git", label "Commit the changes of the hub", the
    empty-actions fallback "List the skills of the hub" (`skill_list`) instead of "Continue normal work" (web golden,
    web unit test and the curation-home fixture updated). Recovery/invalid-workspace fix texts no longer name
    `--workspace <path>`.
16. **`RemoteDoctorResult`, `runRemoteDoctor`, `renderRemoteDoctor` removed**; their golden became a section-renderer
    golden (`TestDoctorSectionOutputGolden`). FIX lines now start with the check label.

## Tests

- New app tests (`internal/app/health_test.go`): fresh hub at both depths and the next step; status never waits for a
  busy hub (< 1 s, `hub_busy`); hub repair with a tracked `.mcp.json` (fixed codes, changed files, not fixed with the
  `git rm --cached` command, Git still tracks it); pre-commit rewrite byte-exact incl. a second spelling, a lookalike
  name left alone, mode kept; token never sent to a project-only hub at light, full and `ApplyFixes` (recording
  `httptest` hub reached through a documentation address) and sent when the user scope names it; unreachable remote
  from a subfolder (hub-machine command, refused fix, next step); moved hub re-pointed only when recorded, the stored
  hub gone, then the moved hub's own connection repaired; permission rules rewritten keeping a user rule;
  `WithWorkspace`; registry (nil/duplicate panics, hub-section findings, panic isolation, `Applies` respected);
  `ApplyFixes` dedupe/failure/cancel; finding JSON round trip; next-step table order; `changedPaths`.
- New CLI tests (`status_doctor_test.go`): 18 role fixtures in real temp folders (hub fresh/dirty/stale index/missing
  `.gitignore` entries/tracked `.mcp.json`/validate hook, hub folder connected to another hub (`both`), client-local
  healthy/moved recorded/moved unrecorded/stored hub moved/old permission rules, client-remote down/subfolder, mixed,
  user scope only, none with/without stored hub): status and doctor problem codes agree (minus doctor-only checks),
  every `skillhub` segment of `Next:` passes `checkGuideCommand`, `status --json` validates against the schema with
  the right role and the same next command, doctor exit code, and `doctor --fix` results per fixture. Plus S1 at the
  CLI (status, doctor, `doctor --fix` never send the token; `--fix` exits 1 with the refusal text), busy status under 1 s,
  `doctor --json` keys, explicit `--workspace` in the next command, permission rules kept, maintain group.
- Updated: doctor/status expectations (exit 1 on problems, new JSON shapes, wording), `doctor-output.golden`
  regenerated, web `home.json` golden regenerated, maintain renames in tests, `TestMutation` try-lock test.
- Results (isolated HOME/XDG_*/`SKILLHUB_USER_CONFIG_DIR`, `GIT_CONFIG_GLOBAL=/dev/null`): `go test -count=1 ./...`
  passes; `go vet ./...`, `GOOS=windows go vet ./...`, `GOOS=darwin go vet ./...` clean; `gofmt -l .` empty.
- Hand check of an installed binary in a scratch sandbox: fresh hub status/doctor, project status, moved hub
  (`doctor --fix` re-points, then healthy), folder with no role (exit 2), stale curator files with previews,
  `--fix --dry-run`, client-remote down (status next command for the hub machine, `--fix` not fixed, exit 1).

## Not done / for later phases

- Web unit tests and e2e not run: no `web/node_modules` (not installed). Web edits are string renames and one test
  expectation per renamed string; the e2e edits change `['rebuild', ...]` to `['maintain', 'rebuild', ...]`. CI is the
  check.
- Windows and macOS not run; CI is the check. Platform-sensitive: the commit line uses `shellquote.Chain`/`Arg`
  (PowerShell form on Windows), `describeChange`/`displayConfigPath` print forward-slash relative paths so goldens
  match, the hook mode assertion is skipped on Windows, `git rev-parse --git-path` output is joined with the hub
  root when relative. The nested reader lock in `GetCurationHome{NoWait}` relies on shared locks from two handles of
  one process coexisting (flock on Unix, LockFileEx on Windows).
- Docs other than `docs/remote-hub-guide.md` (one mechanical rename for the guide test) still name the top-level
  maintenance commands and the old doctor wording (`command-reference`, `curating-skills` incl. the pre-commit hook
  section, `troubleshooting`, `user-guide`, `contracts/error-codes.md`, `release-runbook`, `PRD`): Phase 5 with its
  grep gate. The release note / troubleshooting entry for rewritten pre-commit hooks is Phase 5.

## Unresolved questions

- None blocking. Phase 5 may want to document `doctor --fix --dry-run` exiting 0 and status exiting 2 in a folder
  with no role.

Status: DONE
