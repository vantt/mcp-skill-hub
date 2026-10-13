---
phase: 2
title: "init, connect, user config and role detection"
status: completed
priority: P1
effort: "4d"
dependencies: [1]
---

# Phase 2: init, connect, user config and role detection

## Context

Today `init` takes a path, never commits, refuses an empty Git repo and treats a URL as a path; `connect` needs
`--workspace`; every "no hub" error says `skillhub init <path> --yes`. The table "Verified current behavior" in
[plan.md](./plan.md) is the baseline. Decisions 3 and 4, planner decisions P6, P8 and P10; red-team items S3, S4, S5
(helper and refusal), S7, F1, F2, F3, F4, F5, F12.

Facts this phase builds on (re-verified 2026-10-11):
- `internal/workspace/workspace.go:284-289` runs `git init` before the layout and schema are written; the only hub
  marker is `.skillhub/schema-version` (`Discover`, `:66-85`). An interrupted first run can therefore leave a `.git`
  with an unborn HEAD and part of the layout but no marker.
- `.gitignore` already lists the six required entries (`requiredIgnoreEntries`, `:38-41`) and `desiredGitignore`
  (`:425-443`) appends missing ones; `machine_file_tracked` (`:178`) names tracked connection files. P5 is dropped;
  this mechanism stays.
- The offline Git runner (`offlineGitCommand`, `internal/app/operations.go:164-181`) forces `GIT_TERMINAL_PROMPT=0`,
  `GCM_INTERACTIVE=never` and empty askpass. It must not be reused for remote operations.
- `resolveWorkspace` (`internal/delivery/cli/workspace_resolve.go:42-104`) walks up from cwd and stops when it reaches
  home (home is never scanned); a cwd outside home walks to the filesystem root. `workspace.Discover` has no home bound.
- A project connected with `connect --hub` has a `mcp proxy --hub <url>` entry with no `--workspace`; the local
  extractors (`workspace_resolve.go:171-330`) find nothing (verified R4-R6). `FindRemoteEntries`
  (`internal/app/doctor_remote.go:342`) reads only cwd and the user scope (verified R5).
- `init` writes connection files into the hub folder that point at the hub itself. A registration in the hub root that
  points at that hub is the hub's self-connection (hub role only).

## Requirements

- Remove `init --force` (P10): a non-empty folder is refused with the exact command to create an empty one; remove the flag from `initFlags` (`internal/delivery/cli/workspace.go:235`), help, tests and docs.

**User config (P6, F4).** New package `internal/userconfig`: `type Config struct { Hub string; PreviousHubs []string }`,
`type Store interface { Load() (Config, error); SetHub(path string) error }`, `Default() Store` (directory
`$SKILLHUB_USER_CONFIG_DIR` when set, else `<os.UserConfigDir()>/skillhub`), and `NewStore(dir string) Store` for
tests. File `config.json`, mode 0600, written via temp file + rename, bounded read (64 KiB), unknown keys preserved.
A missing file is an empty config. `SetHub(p)` moves the previous `hub` into `previous_hubs` (deduplicated, newest
first, at most 5) when it differs. `app.Init` takes the store as an option so tests inject a temp store.

**Role detection (F5).** New `internal/app/role.go`, one signature used everywhere:
`DetectRole(cwd, home string, cfg userconfig.Config) RoleReport` with `Hub *HubRef` (root), per host `Project`,
`User` and `Effective` `ClientEntry` (effective = project entry when present, else user entry; resolved question 6),
`StoredHub string`, `PreviousHubs []string` and `Kind()` returning `hub`, `client-local`, `client-remote`, `both` or
`none`, computed from the hub and the effective entries (self-connection excluded): hub plus any other effective entry
is `both`; otherwise any effective local entry gives `client-local`, else any effective remote entry gives
`client-remote`. Per-host sections still show a remote host in a mixed project. The walk matches `resolveWorkspace`:
from cwd up to the filesystem root, stopping when it reaches home, home itself not scanned, so `/srv/skill-hub`,
`D:\hubs`, `/workspace` and `t.TempDir()` hubs work. Entries are read only with `hostintegration.ReadRegistration`
(`internal/hostintegration/connection.go:31`); the bespoke extractors in `workspace_resolve.go:171-330` are deleted
(DRY). `FindRemoteEntries` becomes a thin filter over `RoleReport` so doctor and status see the same entries.

**Resolution messages (reworded once, here).** `resolveWorkspace` keeps its order (`--workspace`,
`SKILLHUB_WORKSPACE`, project connection, upward discovery) but takes its inputs from `DetectRole`. Its error is
role-aware:
- `client-remote`: "This project uses the hub at <url>. Skills are curated on the hub machine." (exit 2);
- `none` with a stored hub that exists: "This folder is not a hub or a connected project." + `skillhub connect`;
- `none` without a stored hub: + `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init` (decision 6, refined);
- local entry whose hub path no longer exists: "The hub at <path> is gone." + `skillhub doctor --fix` when the
  missing path is the stored hub's previous path and the stored hub exists; when the stored hub path itself is
  missing: `cd <new-location> && skillhub init`, then `skillhub doctor --fix` in each project (never `doctor --fix`
  alone when the stored hub is gone); a missing path that was never recorded: `skillhub connect <stored hub path>`
  (same text as the Phase 4 check).
- Help text says once: `--workspace` takes a local hub path, `--hub` takes a URL (P8).

**`init` argument (S3).** `init` takes no path. Its optional argument is classified by
`classifyInitSource(arg string) (InitSource, error)`:
- refuse anything starting with `-`;
- URL with scheme `https`, `ssh` or `file`; refuse other schemes (incl. `http`, `git`, `ext`, `fd`);
- scp form `[user@]host:path`, where host is not a single letter (so `C:\x` and `C:/x` are Windows paths, not scp);
- an existing local directory that holds a Git repository (passed as a path, used like a `file` URL);
- anything else: error naming `cd <folder> && skillhub init`.
- An https URL with userinfo is refused (as `internal/app/connect_remote.go:85` does for `--hub`); for ssh and scp a
  user name is allowed, a password is refused. Every printed URL, incl. the retry hint and Git's echoed stderr, is
  redacted (`url.URL.Redacted()`, plus a stderr scrub of the userinfo string).

**Remote Git runner (S3).** New `internal/app/git_remote.go`: `remoteGitCommand(ctx, root string, args ...string)`
that keeps credential prompts on (inherits `GIT_TERMINAL_PROMPT`; sets it to 0 only when stdin is not a terminal, so
agents fail fast with Git's own message), and always passes `-c protocol.ext.allow=never -c protocol.fd.allow=never
-c core.fsmonitor=false`. It runs `ls-remote --symref`, `fetch`, `push`; the URL comes after `--` for every command
that accepts it (verify `ls-remote` on the oldest CI Git; the leading-`-` refusal covers it otherwise). Local
operations keep `offlineGitCommand`.

**First commit (S3, S7, F1).** `git -c core.hooksPath=<new empty temp dir> -c commit.gpgsign=false commit --no-verify
-m "Initialize Skill Hub hub" -- <paths>`; output says "Git hooks and commit signing were skipped for this first
commit." Identity: resolved question 1 (`-c user.name=... -c user.email=...` only when `git config user.email` is
empty). The paths are exactly the files this run wrote that `git check-ignore` does not exclude (layout, `.gitignore`,
schema, history record, instruction files, generated skill folders until the distill-lab plan ignores them). On
failure: print Git's stderr verbatim, keep everything written, and print `skillhub init` as the retry (the resume rows
below make it work). Push uses `git -c core.hooksPath=<empty> push --no-verify`.

**Pre-existing `.git` (S7).** `init` writes `.git/skillhub-created` right after a `git init` it ran. Any cleanup removes
`.git` only when that marker exists and the folder held nothing else before the run. When a `.git` existed before the
run and its local config sets `core.sshCommand`, the output names it as a warning before any remote operation. `init`
never runs `git clone`; it uses `git init -b <branch>` (or the existing `.git`) + fetch + checkout in both cases.

**Clone trust check (S4).** Before checkout, `git ls-tree -r --name-only <fetched ref>`:
- refuse when anything is tracked under `runtime/` or `.skillhub/transactions/` (tracked `runtime/config/<skill>/env`
  is shell-sourced by agents, `runtime/config/.hub-token` is a credential, `runtime/envs` holds binaries); nothing is
  checked out, probe refs are deleted;
- warn by name when `.mcp.json`, `.claude/settings*.json`, `.codex/`, `.gemini/`, `CLAUDE.md` or `AGENTS.md` are
  tracked, except an instruction file byte-identical to what `init` writes for this hub; tracked machine files also
  get the existing `machine_file_tracked` notice with `git rm --cached <path>`;
- after checkout run the existing validation (`workspace.Inspect` + `canonical.Validate`) before declaring the folder
  a hub; on validation errors keep the checkout, report the issues and do not store the hub path.

**`init` cases** (first matching row wins):

| Situation | Behavior |
|---|---|
| Empty folder, or `.git` with an unborn HEAD and nothing else | `git init -b main` if needed (marker), layout, remediation files, index, self-connection, first commit, `SetHub(cwd)`. Print: hub path, the connection summary line, commit id, next `cd <your-project> && skillhub connect`. |
| Folder whose entries are only init-owned paths (`.git`, `.gitignore`, `.skillhub`, layout dirs, `runtime`, instruction files, connection files, generated skill folders) and HEAD is unborn (interrupted first run, F1) | Resume: finish the missing steps, then the first commit as above. |
| Hub root with an unborn HEAD (first commit failed earlier, F1) | Refresh index and self-connection, then the first commit of the hub-owned paths. |
| Hub root with commits (cwd is the hub root) | Build/refresh index, refresh self-connection, append missing `.gitignore` entries and report them (no commit in an existing hub; status then offers the `.gitignore` commit line), report tracked machine files with `git rm --cached <path>` and finish, `SetHub`. Print "Hub already set up at <path>" and the same next command. |
| Inside a hub subfolder | Refuse: "This folder is inside the hub at <root>." Next: `cd <root> && skillhub status`. |
| Folder with other files, Git or not, not a hub (incl. `go.mod`, `package.json`, `Cargo.toml`, `.gitmodules`) | Refuse; nothing written. Next: `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`. If the folder has a connection file: "To use a hub from this project, run `skillhub connect`." |
| Empty folder inside another Git repo (incl. HOME itself being a dotfiles repo) | Refuse: "A hub must be its own Git repository, but this folder is inside <repo>." Next: a `mkdir -p <dir> && cd <dir> && skillhub init` line whose `<dir>` is outside `<repo>` (a sibling of the repo root, never the folder just refused). |
| `init <git-url>`, remote unreachable | Print Git's stderr verbatim (redacted), then one hint line ("Check the URL and that you can run Git against it, for example with your SSH key or a credential helper."). Exit 1. Nothing left behind (marker rule). |
| `init <git-url>`, remote empty (no refs) | As "empty folder", then `git push --no-verify -- <url> HEAD:refs/heads/main`; only after success `git remote add origin <url>`, `git fetch origin` and `git branch --set-upstream-to=origin/main`; print remote URL, branch and commit pushed. Push failure: print Git's error; the local hub stays valid without `origin`; re-running `skillhub init <url>` pushes again (F2). |
| `init <git-url>`, remote has refs | `git ls-remote --symref`; candidate branches: HEAD's target when it exists, then `main`, `master`, then the rest (F3). Fetch them into `refs/skillhub-probe/*`; the first whose tree has `.skillhub/schema-version` wins. Trust check, `git checkout -B <branch> <ref>`, `git remote add origin <url>`, upstream, delete probe refs, then as "hub root with commits". |
| `init <git-url>`, remote has refs but no branch holds a hub | Delete probe refs; remove `.git` only under the marker rule; refuse: "<url> holds a repository that is not a Skill Hub hub." Next: use an empty repository or the URL of an existing hub. |
| `init <git-url>` in an existing hub without `origin` | `ls-remote`: no refs -> push, then add origin (resolved question 2); refs sharing history (`git merge-base` after fetch into probe refs) -> add origin, no push; unrelated history -> refuse, origin not added. |
| `init <git-url>` in a hub whose `origin` equals the URL | `ls-remote`: no refs -> push (retry case); refs containing local HEAD -> no-op; refs without the local branch -> refuse and print `git push -u origin <branch>` for the user to run (resolved question 2 forbids an automatic push to a remote with refs). |
| `init <git-url>` in a hub whose `origin` differs | Refuse naming the current origin (redacted). |

**connect** (decision 4, P8, S5):
- `skillhub connect [<path>] [-g|--global] [--host claude|codex|gemini]... [--dry-run] [--json]`
- `skillhub connect --hub <url> --token-file <file> [-g] [--host ...] [--dry-run] [--json]`
- `skillhub connect --remove [--cache] [-g] [--host ...] [--dry-run] [--json]`
- `--workspace` and `--project` are removed from connect here (F12), together with every printed suggestion that used
  them (`internal/app/host-connections.go:97-99`, `internal/delivery/cli/workspace.go:63-66`).
- Target hub: `<path>`, else stored hub, else error "No hub is set up on this machine." + the `init` line (exit 2).
  A stored hub that no longer exists: "The hub stored at <path> is gone." + `cd <new-location> && skillhub init`.
- Run in the hub root itself: "This is the hub; `skillhub init` already connected its agents." Refresh only.
- Keep the safe merge and the conflicting-managed-block refusal (`internal/delivery/cli/onboarding_test.go:243`).
- Remote (S5): the client token path helper `hubtoken.ClientPath(hubURL string) string` returns
  `~/.skillhub-token-<host>-<port>` (host sanitized to `[A-Za-z0-9.-]`, IPv6 brackets dropped). connect refuses a
  `--token-file` that another registration on this machine (project or user scope, read through `DetectRole`) already
  uses for a different hub URL, naming that hub and the helper's path for this one.
- Output: one line per agent with the files written, then `Next: restart your agent and ask it: "curate my Skill Hub"`.
- One function renders the connection summary `connection written: claude-code, codex-cli, gemini-cli`
  (`connectionSummaryLine`); init and connect both call it (shared with the distill-lab plan).

**Test seam (F12).** `app.WorkspaceService.Init(path string, opts InitOptions)` keeps the path parameter
(`InitOptions{DryRun bool; Source InitSource; Store userconfig.Store}`). The CLI computes the path from `os.Getwd()`
in `runInit`, which delegates to an unexported `runInitIn(ctx, dir string, args []string, stdout, stderr)`. CLI tests
call `runInitIn` through a helper (`initHub(t, dir)`), never `os.Chdir`/`t.Chdir` (73 cli/app test files use
`t.Parallel`). Subprocess tests set `cmd.Dir`.

## Files

- Create: `internal/userconfig/userconfig.go`, `userconfig_test.go`
- Create: `internal/app/role.go`, `role_test.go`; `internal/app/git_remote.go`, `git_remote_test.go`;
  `internal/app/workspace_remote.go` (remote rows of the init table), `workspace_remote_test.go`
- Modify: `internal/workspace/workspace.go` (`safeTarget` messages at `:312-346`; emptiness check that accepts an
  unborn `.git` and init-owned paths; `git init -b main` + marker at `:284-289`; first-commit helper)
- Modify: `internal/app/workspace.go` (`Init` rewrite: cases above, `SetHub`, commit), `internal/app/connect.go`,
  `internal/app/connect_remote.go` (target resolution, token-file refusal, plain texts),
  `internal/app/doctor_remote.go` (`FindRemoteEntries` over `RoleReport`), `internal/app/host-connections.go`
  (suggested connect commands without `--workspace`)
- Modify: `internal/hubtoken/hubtoken.go` (`ClientPath`)
- Modify: `internal/delivery/cli/workspace.go` (`runInit`, `runInitIn`, `initFlags` at `:235`, `renderInitSummary`;
  delete `stageWorkspaceCommand` at `:71`), `connect.go` (flags), `workspace_resolve.go` (use `DetectRole`, delete
  extractors, messages), `workspace_env.go`, `help.go` (init and connect usage only)
- Modify: every `TestMain` isolation list to add `SKILLHUB_USER_CONFIG_DIR`, `APPDATA` and `LOCALAPPDATA`:
  `internal/delivery/cli/main_test.go:56-58`, `internal/{app,hostintegration}/main_test.go`,
  `internal/delivery/{mcpserver,web}/main_test.go`
- Modify: `scripts/install.sh:751,811`, `scripts/install.ps1:670,811` (next step:
  `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`; PowerShell form with `;`)
- Modify: `.github/workflows/release.yml:730,837` and `.github/workflows/evaluation.yml:43` (run `skillhub init` in the
  temp folder with `SKILLHUB_USER_CONFIG_DIR` pointing into a temp dir); `web/e2e/support/server.ts:28-33,71` (pass an
  env with `SKILLHUB_USER_CONFIG_DIR` and temp `HOME`/`XDG_CONFIG_HOME` instead of `process.env`; run init with
  `cwd: ws`), `web/e2e/support/seed-workspace.ts:81`, `web/e2e/skill-resources.spec.ts:27`,
  `web/e2e/skill-sources.spec.ts:20`
- Modify: `internal/delivery/mcpserver/{memory_budget_test.go:521,remote_bench_test.go:481,skill_tools_test.go:367,subprocess_test.go:44}`
  (`init` with `cmd.Dir = root`)
- Tests to update: 51 CLI `init` call sites in 23 files (largest: `internal/delivery/cli/{skill,onboarding,version,curation,resolver,catalog}_test.go`),
  46 direct `WorkspaceService.Init` calls in 17 files (signature only),
  `internal/delivery/cli/{ux_behavior,workspace_next_steps,workspace_resolve,connect_regression,connect_profiles,connect_remote,remote_commands,doctor-safety}_test.go`,
  `internal/app/{workspace,curation_first_run,doctor_remote}_test.go`, `internal/workspace/workspace_test.go`

## Steps

1. `userconfig` package with tests using `t.Setenv` (missing file, corrupt file -> error naming the file, mode 0600 on
   POSIX, atomic replace, unknown keys preserved, previous-hubs rotation, `SKILLHUB_USER_CONFIG_DIR`).
2. Isolation: add the three variables to every `TestMain` list, the e2e support env, CI and release smoke, before any
   code calls `SetHub`.
3. `DetectRole` with table tests built from real temp folders: hub root, hub subfolder, hub self-connection only
   (= `hub`), hub + project entry to another hub (= `both`), hub + user-scope remote (= `both`), project local, project
   remote, subfolder of a remote project, user scope only, mixed (project-local entry for Claude plus user-scope remote
   for Codex: Kind `client-local`, Codex section remote), none with and without stored hub, entry pointing at a missing
   hub, stale stored hub, HOME itself a Git repo, hub under a path outside HOME, home directory never scanned.
4. Rewire `resolveWorkspace` and `FindRemoteEntries`; delete the extractors; resolution messages and tests.
5. `classifyInitSource` table test (https, ssh, file, scp with and without user, `C:\x`, `C:/x`, `-oProxyCommand=x`,
   `--upload-pack=x`, `ext::sh -c x`, `http://`, userinfo with password, existing repo dir, plain dir, nonexistent
   path) and the remote runner (env assertions: prompts not disabled on a terminal, protocol config present).
6. `init` local rows with the test seam, then remote rows using local bare repositories as remotes (`git init --bare
   -b main` in `t.TempDir()`; a bare repo whose HEAD names `master` while only `main` exists; an unreachable `file://`
   path; a bare repo whose history tracks `runtime/config/x/env`; one tracking `CLAUDE.md` with foreign content; one
   tracking `.mcp.json`); no network in tests. Tests pin `git init -b main` as in
   `internal/source/git_revision_at_test.go:32`.
7. Injected-failure tests (F1): `commit.gpgsign=true` with no key (init still commits because signing is disabled),
   a pre-commit hook that exits 1 in a pre-existing `.git` (skipped), an interrupted run simulated by stopping after
   `git init` and after the layout (re-run resumes), a commit failure injected through a fake `git` on `PATH` or a
   read-only `.git/objects` (stderr printed, retry works), and the push failure + retry (F2).
8. `connect` target resolution, flag removal, token-file refusal, messages.
9. Installer and CLI call sites.

## Todo

- [x] `internal/userconfig` (`Config`, `Store`, `Default`, `NewStore`, `SetHub` with five previous hubs, 0600, atomic write, unknown keys kept, 64 KiB bound) and its tests
- [x] `SKILLHUB_USER_CONFIG_DIR`, `APPDATA`, `LOCALAPPDATA` in every `TestMain` isolation list, the web e2e support, the evaluation workflow and the release smoke
- [x] `DetectRole`/`RoleReport`/`Kind()` with table tests; `FindRemoteEntries` filters the report; the connection-file extractors are deleted
- [x] Role-aware resolution errors (remote project, none with and without a stored hub, stored hub gone, moved hub via `MovedHubFix`)
- [x] `ClassifyInitSource` and the remote Git runner (prompts only on a terminal, `protocol.ext/fd.allow=never`, `core.fsmonitor=false`, `--` before the URL) with tests
- [x] Every `init` row: empty folder, empty Git repo, interrupted runs, hub with unborn HEAD, hub with commits (`.gitignore` entries appended and reported, tracked machine files named), hub subfolder, other files, nested repo and HOME-as-repo refusals, `init <git-url>` unreachable / empty (push, then origin) / hub (probe, trust check, checkout) / not a hub / existing hub binding / origin rules
- [x] First commit with hooks and signing disabled, fallback identity, retry after a failed commit; marker-based `.git` cleanup; `core.sshCommand` warning
- [x] `init --force` removed; `init` takes no folder; `runInitIn` test seam and `initHub` helper (no `chdir` in parallel tests)
- [x] `connect [<path>]`, stored hub, hub-root refresh, `--workspace`/`--project` removed, plain missing-hub errors, token-file refusal, `hubtoken.ClientPath`, one `connectionSummaryLine`
- [x] Installer next step, CI and release smoke, web e2e call sites, mcpserver subprocess tests
- [x] `go test ./...`, `go vet` (linux, windows, darwin) and `gofmt -l` clean; web e2e not run here (no `web/node_modules`)

Report: [cook-261011-onboarding-phase2](../../../plans/reports/cook-261011-onboarding-phase2.md)

## Validation

- Each `init` row has a test asserting exit code, files, `git log` (one commit or none), `git status --porcelain
  --untracked-files=all` empty after a successful local init, stored config, and the printed next command (every
  `skillhub` segment passes `checkGuideCommand` via the Phase 1 extractor).
- `init <bare-empty-remote>` leaves the bare repo with exactly one commit on `main` and the local repo with `origin`
  and upstream set; after an injected push failure there is no `origin` and a second `init <url>` pushes.
- A pre-existing `.git` is never removed (assert inode/dir still present with its config) on any refusal path.
- No printed line contains a URL password (assert on output for a userinfo URL in ssh form).
- `connect` with no stored hub prints the `init` line and writes nothing; with a stored hub writes the same files as
  today's `connect --workspace <hub>` (goldens `internal/delivery/cli/testdata/connect-workspace/*.golden`, unchanged
  because registrations keep `mcp serve`).
- `go test ./internal/userconfig/... ./internal/app/... ./internal/workspace/... ./internal/delivery/...`;
  `make web-e2e`.
- Manual sandbox (isolated HOME, XDG_* and `SKILLHUB_USER_CONFIG_DIR`, never `/home/vantt/skill-hub`): rerun the case
  script from plan.md and compare with the Target column.
- macOS: temp dirs live under `/var/folders/...` (symlinked to `/private/var`); assert paths with
  `filepath.EvalSymlinks`. Windows: `%AppData%` isolation, scp-vs-drive-letter classification, PowerShell next command.
  Both via CI.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| A user's `.git` is deleted on refusal | Low x High | Marker rule; never `git clone`; test on every refusal path |
| Argument injection through `init <arg>` | Low x High | Classifier, `--`, protocol config, table tests |
| Cloned hub with tracked runtime files or hostile agent configs | Low x High | Trust check before checkout; refuse/warn; validation before storing the hub |
| Credential prompts hang in CI or agent shells | Medium x Medium | Prompts only when stdin is a terminal; otherwise `GIT_TERMINAL_PROMPT=0` and Git fails fast |
| Tests write the real user config | Medium x High | Isolation first (step 2); `Store` injected; `t.Setenv` in `userconfig` tests |
| Two hubs on one machine confuse the stored path | Medium x Low | `init` always prints which hub is now stored; `connect <path>` selects another explicitly |
| Removing extractors changes resolution for odd JSON (`cwd`-only entries, `workspace_resolve.go:247-271`) | Low x Medium | `ReadRegistration` gains the `cwd` fallback for Gemini if a test shows it is needed |
| Test migration larger than estimated | Medium x Medium | Path parameter kept in `app.Init`; helper `initHub(t, dir)` makes CLI call-site edits mechanical |

## Rollback

Revert the phase commits. The user config file is harmless to older binaries (they never read it); hubs created by this
phase are ordinary hubs with one commit and the standard `.gitignore`; registrations are unchanged.
