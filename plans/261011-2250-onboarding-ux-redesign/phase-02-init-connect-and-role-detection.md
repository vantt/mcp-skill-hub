---
phase: 2
title: "init, connect, user config and role detection"
status: pending
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 2: init, connect, user config and role detection

## Context

Today `init` takes a path, never commits, refuses an empty Git repo and treats a URL as a path; `connect` needs
`--workspace`; every "no hub" error says `skillhub init <path> --yes`. The table "Verified current behavior" in [plan.md](./plan.md) is the baseline. Decisions 3 and 4, planner decisions P5, P6, P8 and P10.

Role detection gaps found while reading `internal/delivery/cli/workspace_resolve.go:42-104`:
1. A project connected with `connect --hub` has a `mcp proxy --hub <url>` entry with no `--workspace`; the local
   extractors (`workspace_resolve.go:246-330`) find nothing, so resolution says "no workspace" (verified R4-R6). Doctor
   finds remote entries through `FindRemoteEntries` (`internal/app/doctor_remote.go:342`), which reads only the cwd and
   the user scope, never parent folders (verified R5).
2. `init` writes connection files into the hub folder that point at the hub itself, so a hub looks like a client of
   itself. Defined here: a registration in the hub root that points at that hub is the hub's self-connection and
   belongs to the hub role; `both` means the hub folder (or the user scope) also has a registration pointing at a
   different local hub or at a remote hub.
3. The stored hub path (new) is an input: it tells connect which hub to use, lets doctor re-point a moved hub, and
   lets the "none" message print `skillhub connect` instead of `init`.
4. Resolution never scans the home directory (`workspace_resolve.go:66-81`), so user-scope (`-g`) connections are
   invisible to it; detection reads them explicitly from the adapters' user config paths.

## Requirements

**User config (P6).** New package `internal/userconfig`: `Load() (Config, error)`, `SetHub(path string) error`.
File `<os.UserConfigDir()>/skillhub/config.json`, mode 0600, written via temp file + rename, bounded read (64 KiB),
unknown keys preserved. A missing file is an empty config, not an error.

**Role detection.** New `internal/app/role.go`: `DetectRole(cwd, home string, cfg userconfig.Config) RoleReport` with
`Hub *HubRef` (root), `Project []ClientEntry` (nearest ancestor below home with a skillhub entry), `User
[]ClientEntry` (user scope), `StoredHub string`, and `Kind()` returning `hub`, `client-local`, `client-remote`,
`both` or `none`. Entries are read only with `hostintegration.ReadRegistration`; the three bespoke extractors in
`workspace_resolve.go` are deleted (DRY). `FindRemoteEntries` becomes a thin filter over `RoleReport` so doctor
and status see the same entries.

**Resolution.** `resolveWorkspace` keeps its order (`--workspace`, `SKILLHUB_WORKSPACE`, project connection, upward
discovery) but takes its inputs from `DetectRole`. Its error is role-aware:
- `client-remote`: "This project uses the hub at <url>. Skills are curated on the hub machine." (exit 2);
- `none` with a stored hub: "This folder is not a hub or a connected project." + `skillhub connect`;
- `none` without a stored hub: + `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`;
- local entry whose hub path no longer exists: "The hub at <path> is gone." + `skillhub doctor --fix` when a stored hub
  exists, else the `init` command.

**init** (no path argument; an argument that is not a Git URL is an error naming `cd <folder> && skillhub init`):

| Situation | Behavior |
|---|---|
| Empty folder, or only `.git` with no commits | Create hub layout, `git init` if needed, remediation files, index, self-connection, append the self-connection paths to `.git/info/exclude`, first commit of every non-excluded file, `SetHub(cwd)`. Print: hub path, agents connected, commit id, next `cd <your-project> && skillhub connect`. |
| Already a hub (cwd is the hub root) | Build/refresh index, refresh self-connection, ensure exclude entries, `SetHub`; no commit, no content change. Print "Hub already set up at <path>" and the same next command. |
| Inside a hub subfolder | Refuse: "This folder is inside the hub at <root>." Next: `cd <root> && skillhub status`. |
| Folder with files, Git or not, not a hub (incl. `go.mod`, `package.json`, `Cargo.toml`, `.gitmodules`) | Refuse; nothing written. Next: `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`. If the folder has a connection file: "To use a hub from this project, run `skillhub connect`." |
| Empty folder inside another Git repo | Refuse: "A hub must be its own Git repository, but this folder is inside <repo>." Next: the `mkdir` line with a folder outside. |
| `init <git-url>`, remote unreachable | Print Git's stderr verbatim, then one hint line ("Check the URL and that you can run Git against it, for example with your SSH key or a credential helper."). Exit 1. Nothing left behind. |
| `init <git-url>`, remote empty | As "empty folder", then `git remote add origin <url>` and `git push -u origin HEAD`; print remote URL, branch and commit pushed. Push failure: print Git's error; the local hub stays valid and the message gives `skillhub init <url>` to retry the binding. |
| `init <git-url>`, remote has a hub | Folder must be empty; `git clone --no-checkout <url> .`, `git ls-tree HEAD .skillhub/schema-version` succeeds, `git checkout`, then as "already a hub". |
| `init <git-url>`, remote has other content | Remove the `.git` this command created; refuse: "<url> holds a repository that is not a Skill Hub hub." Next: use an empty repository or the URL of an existing hub. |
| `init <git-url>` in an existing hub without `origin` | `ls-remote`: empty -> add origin and push (resolved question 2); refs sharing history (`git merge-base` after fetch) -> add origin, no push; unrelated history -> refuse, origin not added. |
| `init <git-url>` in a hub whose `origin` differs | Refuse naming the current origin; same URL -> no-op. |

Git commands run with `GIT_TERMINAL_PROMPT` inherited (credential prompts work in a terminal) and the user's Git
config; the first commit message is "Initialize Skill Hub hub". Missing Git identity: resolved question 1.

**connect** (decision 4, P8):
- `skillhub connect [<path>] [-g|--global] [--host claude|codex|gemini]... [--dry-run] [--json]`
- `skillhub connect --hub <url> --token-file <file> [-g] [--host ...] [--dry-run] [--json]`
- `skillhub connect --remove [--cache] [-g] [--host ...] [--dry-run] [--json]`
- Target hub: `<path>`, else stored hub, else error "No hub is set up on this machine." + the `init` line (exit 2).
  A stored hub that no longer exists: "The hub stored at <path> is gone." + the `init` line.
- Run in the hub root itself: "This is the hub; `skillhub init` already connected its agents." Refresh only.
- Keep the safe merge and the conflicting-managed-block refusal (`internal/delivery/cli/onboarding_test.go:243`).
- Output: one line per agent with the files written, then `Next: restart your agent and ask it: "curate my Skill Hub"`.

## Files

- Create: `internal/userconfig/userconfig.go`, `userconfig_test.go`
- Create: `internal/app/role.go`, `role_test.go`
- Modify: `internal/workspace/workspace.go` (`safeTarget` messages at `:310,313`; empty check that ignores a
  commit-less `.git`; exclude-file helper; first-commit helper using the offline Git runner pattern of
  `internal/app/operations.go:164`)
- Modify: `internal/app/workspace.go` (`Init` rewrite: cases above, `SetHub`, commit, remote binding), new
  `internal/app/workspace_remote.go` for `ls-remote`/clone/push
- Modify: `internal/app/connect.go`, `internal/app/connect_remote.go` (target resolution from stored hub; plain texts)
- Modify: `internal/app/doctor_remote.go` (`FindRemoteEntries` over `RoleReport`)
- Modify: `internal/delivery/cli/workspace.go` (`runInit`, `initFlags`, `renderInitSummary`, drop
  `stageWorkspaceCommand` once commits are automatic), `connect.go` (flags), `workspace_resolve.go` (use
  `DetectRole`, delete extractors), `workspace_env.go`, `help.go` (init and connect usage only)
- Modify: every `TestMain` isolation list to add `APPDATA` and `LOCALAPPDATA`:
  `internal/{app,hostintegration}/main_test.go`, `internal/delivery/{cli,mcpserver,web}/main_test.go`
- Modify: `scripts/install.sh:751,811`, `scripts/install.ps1:670,811` (next step:
  `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`; PowerShell form with `;`)
- Modify: `.github/workflows/release.yml:730,837`, `.github/workflows/evaluation.yml:43`, `web/e2e/support/server.ts:33`
  (run `skillhub init` with the temp folder as working directory)
- Tests to update: `internal/delivery/cli/{onboarding,ux_behavior,workspace_next_steps,workspace_resolve,connect_regression,connect_profiles,connect_remote,remote_commands,doctor-safety}_test.go`,
  `internal/app/{workspace,curation_first_run,doctor_remote}_test.go`, `internal/workspace/workspace_test.go`

## Steps

1. `userconfig` package with tests (missing file, corrupt file -> error naming the file, mode 0600 on POSIX, atomic
   replace, unknown keys preserved).
2. `DetectRole` with table tests built from real temp folders: hub root, hub subfolder, hub self-connection only (=
   `hub`), hub + project entry to another hub (= `both`), hub + user-scope remote (= `both`), project local, project
   remote, subfolder of a remote project, user scope only, none with and without stored hub, stale `mcp serve`
   entry, entry pointing at a missing hub, home directory never scanned.
3. Rewire `resolveWorkspace` and `FindRemoteEntries`; delete the extractors; update resolution tests.
4. `init` local cases, then `init <git-url>` cases using local bare repositories as remotes (`git init --bare` in
   `t.TempDir()`; an unreachable `file://` path for the error case); no network in tests.
5. `connect` target resolution and messages.
6. Installer, CI and e2e call sites.

## Validation

- Each `init` row has a CLI test asserting exit code, files, `git log` (one commit or none), `git status --porcelain`
  empty after init, stored config, and the printed next command (checked against help with the docs test helper
  `checkGuideCommand`, `internal/delivery/cli/remote_guide_doc_test.go:52`).
- `init <bare-empty-remote>` leaves the bare repo with exactly one commit on the pushed branch.
- `connect` with no stored hub prints the `init` line and writes nothing; with a stored hub writes the same files as
  today's `connect --workspace <hub>` (goldens `testdata/connect-workspace/*.golden`).
- `go test ./internal/userconfig/... ./internal/app/... ./internal/workspace/... ./internal/delivery/cli/...`.
- Manual sandbox (isolated HOME and XDG_*, never `/home/vantt/skill-hub`): rerun the case script from plan.md and
  compare with the Target column.
- macOS: temp dirs live under `/var/folders/...` (symlinked to `/private/var`); assert paths with
  `filepath.EvalSymlinks`. Windows: `%AppData%` isolation; `.git/info/exclude` uses forward slashes. Both via CI.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| A cloned non-hub repo is left behind after refusal | Low x Medium | Only remove `.git` when the folder was empty before clone (checked first); test asserts the folder is empty again |
| Credential prompts hang in CI or agent shells | Medium x Medium | Inherit `GIT_TERMINAL_PROMPT`; when stdin is not a terminal set it to 0 so Git fails fast with its own message |
| Two hubs on one machine confuse the stored path | Medium x Low | `init` always prints which hub is now stored; `connect <path>` selects another explicitly |
| Removing extractors changes resolution for odd JSON (`cwd`-only entries, `workspace_resolve.go:266-268`) | Low x Medium | `ReadRegistration` gains the `cwd` fallback for Gemini if a test shows it is needed |

## Rollback

Revert the phase commits. The user config file is harmless to older binaries (they never read it); hubs created by
this phase are ordinary hubs with one commit and local exclude entries.
