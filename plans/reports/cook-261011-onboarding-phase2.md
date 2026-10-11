# Onboarding UX redesign, phase 2 (init, connect, user config, role detection): implementation report

Branch `worktree-agent-a5c101367df3589e1`, 2026-10-12. Plan:
[plan.md](../261011-2250-onboarding-ux-redesign/plan.md),
[phase-02](../261011-2250-onboarding-ux-redesign/phase-02-init-connect-and-role-detection.md). Status: completed.

## What changed

Commits on the branch (nothing pushed):

| Commit | Change |
|---|---|
| `abd08a3` feat(userconfig) | `internal/userconfig`: `<UserConfigDir>/skillhub/config.json` (or `$SKILLHUB_USER_CONFIG_DIR`), `{"hub", "previous_hubs"}`, mode 0600, temp file + rename, 64 KiB bound, unknown keys kept, five previous hubs newest first. All five `TestMain` lists isolate `APPDATA`, `LOCALAPPDATA` and `SKILLHUB_USER_CONFIG_DIR`. |
| `4436502` fix(canonical) | Validation no longer turns the `machine_file_tracked` finding into an issue (see Decisions). |
| `6b35bbd` feat(shellquote) | `Chain` (`&&` in sh, `;` in PowerShell) and `MakeDir` (`mkdir -p` / `mkdir -Force`). |
| `469776d` feat(hubtoken) | `ClientPath(hubURL)` = `~/.skillhub-token-<host>-<port>`. |
| `3114b03` feat(cli)! | The phase itself: `DetectRole`, role-aware resolution, the init rewrite (local and remote rows), the remote Git runner, `connect` target resolution and token-file refusal, help for init and connect, and every test call site. |
| `809222b` test(web) | e2e: init runs with `cwd: ws` and a temporary per-user config; the server helper uses a temporary HOME/XDG_CONFIG_HOME/APPDATA/LOCALAPPDATA/`SKILLHUB_USER_CONFIG_DIR`. |
| `8cfd182` ci | evaluation.yml and both release smoke steps run `skillhub init` inside the temp folder with `SKILLHUB_USER_CONFIG_DIR` in another temp folder; installers print `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init` (PowerShell `mkdir -Force ~/skill-hub; cd ~/skill-hub; skillhub init`). |
| `0cfbe76`, `e575121` fix | Init summary lines with paths are not wrapped; refusals name the enclosing repository by the path the user typed (macOS `/var` vs `/private/var`). |
| `61273e7` test(cli) | Token-file refusal, unborn-HEAD hub, no push to a remote that has commits, `core.sshCommand` warning. |

Main files: `internal/app/{role,git_remote,workspace_init,workspace_remote}.go` (new), `internal/app/{workspace,connect,connect_remote,doctor_remote,host-connections}.go`,
`internal/workspace/workspace.go` (`InitRepository` = `git init -b main` + `.git/skillhub-created`, `IsInitOwned`,
`MissingIgnoreEntries`, `MachineFiles`, plain `safeTarget` texts), `internal/delivery/cli/{workspace,workspace_resolve,connect,connect_remote,help}.go`,
`internal/hostintegration/bootstrap.go` (`FreshInstructionFile`, used by the trust check).

Interfaces: `WorkspaceService.Init(path string, opts InitOptions)` with `InitOptions{DryRun, Source InitSource, Store userconfig.Store}`;
the result carries `*InitReport` in `Details["init"]`. `DetectRole(cwd, home string, cfg userconfig.Config) RoleReport`;
`FindRemoteEntries(RoleReport)`; `MovedHubFix(report, missing)` (shared text for the Phase 4 check); `NewHubCommand()`;
`ConnectService{Store}` and `ConnectRequest.Workspace` = optional hub folder. CLI seams: `runInitIn(ctx, dir, args, ...)`,
`runConnectIn(ctx, dir, args, ...)`; test helpers `initHub`, `runInitCLI`, `runConnectCLI` (`init_helper_test.go`).
The connection summary wording lives only in `connectionSummaryLine` (`internal/delivery/cli/workspace.go`).

## Tests

- New: `internal/userconfig` (missing/corrupt/oversized file, 0600, atomic replace, unknown keys, rotation, override, fallback dir),
  `TestDetectRole` (17 fixtures incl. hub self-connection, both, mixed, remote subfolder, HOME a repo, hub outside HOME, home never scanned),
  `TestMovedHubFixNamesTheRightRepair`, `TestClassifyInitSource` (the plan's table plus `fd::`, `git://`, token-only userinfo, a folder inside a repo),
  `TestRemoteGitCommandKeepsPromptsOnlyForAPerson`, `TestParseRemoteRefsOrdersCandidateBranches`, `TestRedactUserinfo...`,
  `internal/delivery/cli/init_test.go` (20 tests: every init row with exit code, files, `git rev-list --count`, clean `git status --porcelain --untracked-files=all`,
  stored config and `checkGuideCommand` on every printed `$` line; local bare repositories as remotes, incl. HEAD=master with only main,
  untracked runtime file, foreign `CLAUDE.md` + tracked `.mcp.json`, unreachable `file://`, push failure and retry, unrelated history, origin rules;
  injected failures: `commit.gpgsign=true` with a bogus key, failing hooks in a pre-existing `.git`, interrupted runs, read-only `.git/objects`),
  connect: stored hub, no stored hub, stored hub gone, hub-root refresh, removed flags, token-file refusal; `TestValidateDoesNotFailOnATrackedConnectionFile`.
- Updated: 46 `WorkspaceService.Init` calls (signature), the CLI `init` call sites (`runInitTo`/`initHub`), connect calls (`runConnectCLI`),
  status first-run tests (init now commits), resolution messages, connect usage goldens. Connect output goldens regenerated; the
  file goldens (`project-files`, `global-files`, `doctor-output`) are unchanged.
- Results (isolated HOME/XDG_*/`SKILLHUB_USER_CONFIG_DIR`, `GIT_CONFIG_GLOBAL=/dev/null`): `go test -count=1 ./...` passes;
  `go vet ./...`, `GOOS=windows go vet ./...`, `GOOS=darwin go vet ./...` clean; `gofmt -l` empty.
- Hand check of the installed binary in a scratch sandbox: empty folder, rerun, hub subfolder, folder with files, connect with the stored hub,
  `init <empty bare>` (pushed, origin set), nested repo, `init file://<bare>` (fetched), connect and `skill list` with no stored hub.

## Decisions where the plan was silent

1. **Canonical validation ignores `machine_file_tracked`.** Validation copied every workspace finding into its issues, so a hub that
   tracks `.mcp.json` could not build its catalog at all; the plan requires init to name the file with `git rm --cached` and finish.
   The finding stays in doctor. (`internal/canonical/canonical.go`, outside the phase file list.)
2. **A folder without the hub marker uses the bootstrap path** (`workspace.Apply`), also when it exists, so the first commit holds no
   `history/operations` receipt and no receipt is written before the first commit. An interrupted run's empty `.skillhub` is removed
   before resuming (otherwise it reads as a legacy v0 layout).
3. **Fallback identity** is used when `user.name` or `user.email` is empty (not only email), for that commit only.
4. **Upstream without a second fetch:** after a push or a probe fetch, `refs/remotes/origin/<branch>` is set with `update-ref` instead of
   `git fetch origin`, so a person is asked for credentials once.
5. **`init <git-url> --dry-run` is refused**: telling whether a remote holds a hub needs a fetch, which writes objects.
6. **Trust-check warnings**: `.gemini/skills/` is exempt (init commits the generated curator skill there), and `GEMINI.md` is treated
   like `CLAUDE.md`/`AGENTS.md` (warned unless identical to init's own content).
7. **`init <git-url>` in an interrupted-init folder with a non-empty remote** is refused with the command for a new folder.
8. **Resolution adds a user-scope fallback** after the hub discovery step, so `Kind() = client-local` from a `connect -g` entry resolves
   the same hub (order: `--workspace`, `SKILLHUB_WORKSPACE`, project entry, enclosing hub, user entry).
9. **Token-file refusal ignores the entries the same connect replaces** (same scope and host), so switching one scope to another hub
   with the same file still works; a file used by another scope for another hub is refused.
10. **`connect` ignores `SKILLHUB_WORKSPACE`**: the target is `<path>`, else the hub the folder is in, else the stored hub (plan text).
11. **Connect summary**: `Connected <scope> to the hub at <hub>.` then `connectionSummaryLine`, one line per agent, and
    `Next: restart your agent and ask it: "curate my Skill Hub"`. The Claude guidance line now lists what asks first (the old text named
    `--yes` only). This is how "moving the printed connect guidance" was read, together with init's single next command
    `cd <your-project> && skillhub connect` replacing the old three-step block, and the installers' next step.
12. **`stageWorkspaceCommand` moved, not deleted**: init no longer prints it, but status still uses it for "Review uncommitted
    changes" (Phase 4 rewrites status).
13. **`init` output filters warnings** to what init found (`git_ssh_command`, `hub_tracks_agent_config`); routine notes stay in
    `--verbose`/`--json`. Init errors exit 2 for refusals and 1 for Git failures (also with `--json`) and for a failed push.
14. **`docs/remote-hub-guide.md`**: two mechanical edits (`connect --workspace ~/skillhub` -> `connect ~/skillhub`) because the guide test
    checks commands against help; the wording pass stays in Phase 5.

## Not done / for later phases

- Web typecheck and e2e not run: no `web/node_modules` in the worktree (not installed). The e2e edits are small: `['init']` with `cwd`,
  an isolated env in `server.ts`, `SKILLHUB_USER_CONFIG_DIR` in `isolatedEnv`. CI (`make web-e2e`) is the check.
- Windows and macOS not run; CI is the check. Platform-sensitive tests: `TestInitRefusesFoldersItMustNotTouch` (PowerShell chain,
  enclosing repo path), `TestInitReportsAnUnreachableRemote...` (drive-letter `file:///` URL), the classifier's `C:\x`/`C:/x` rows,
  `TestDefaultFallsBackToTheUserConfigDirectory` (`%AppData%`, macOS `Library/Application Support`). Two tests skip on Windows and
  as root (read-only directories).
- "Full disk" is represented by the read-only `.git/objects` write failure; no disk-full simulation.
- `git ls-remote --symref -- <url>` verified on Git 2.53; init also needs Git >= 2.29 (`init -b`, `--pathspec-from-file`,
  `--no-write-fetch-head`).
- Docs (README, getting-started, troubleshooting, command reference) still show `init <path>` and `connect --workspace`: Phase 5.
- Doctor/status texts that still name `--workspace <path>` (`NewWorkspaceInvalidError`, `writeInvalidWorkspace`): Phase 4.
- The global help's step 1 is the POSIX form on every platform.

## Unresolved questions

- None blocking. Phase 4 should decide whether `doctor --fix` (not only init) treats a tracked connection file as a notice rather
  than a blocking finding; today only init passes it.
