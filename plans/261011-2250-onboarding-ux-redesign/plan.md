---
title: "Onboarding UX redesign: init, connect, serve, status and README"
description: "Make Skill Hub onboarding two steps (install the hub, install the client project) with the fewest commands, plain syntax, role-aware status and doctor, and a README that holds the whole onboarding."
status: pending
priority: P1
effort: 6.5d
branch: worktree-agent-a5c101367df3589e1
tags: [cli, onboarding, init, connect, serve, doctor, status, docs]
blockedBy: []
blocks: []
created: 2026-10-11
---

# Onboarding UX redesign

## Goal

A new user installs `skillhub`, runs one command in an empty folder to get a hub, runs one command in a project to
connect it, and asks the agent "curate my Skill Hub". Two roles exist: the **hub** (a folder with skills, a Git
repository) and the **client project** (a project whose agents use a hub on this machine or on another one).
Every command acts on the current directory, acts directly, prints what it changed, and ends with one runnable next
command. Backward compatibility is not required: old names and flags are removed, not aliased.

Reference: [CLI onboarding patterns research](../reports/researcher-261011-cli-onboarding-patterns.md) (in the main
checkout; patterns 1-9 adopted, pattern 10 "pairing codes" deferred per decision 10).

## Decisions (made by the user; not reopened)

1. **Docs convention.** Every hub command is written `cd <folder> && skillhub <command>`. Hub commands act on the
   current directory (discovery already walks up to the hub root, `internal/workspace/workspace.go:59`). No
   `--workspace` in any example.
2. **Two-step README.** After Install: Step 1 install the hub, Step 2 install the client project. The README holds the
   whole onboarding; `docs/getting-started.md` is merged into it and deleted. The install one-liner lives only in the
   README; `docs/installation.md` keeps details.
3. **Hub creation is `skillhub init` in the current, empty folder.** No subfolder, no implicit directory, no `--from`,
   the user never types a Git command, the first commit is automatic. `skillhub init <git-url>` inspects the remote:
   it has a hub -> clone into the current folder and build the index; it is empty -> create a hub here, set `origin`,
   commit, push the first commit and print what was pushed (the only automatic push); it has other content -> refuse
   with guidance; unreachable -> print Git's error verbatim plus one hint line. `init <git-url>` inside an existing hub
   without a remote adds `origin` (refuse on unrelated history). `init` is idempotent (empty folder, empty Git repo,
   already-cloned hub: build index, store hub path, change no content; other repo with content: refuse with a concrete
   next step). `init` keeps writing the agent connection files for the hub folder itself.
4. **`init` stores the hub path in per-user config; `skillhub connect` reads it.** `connect <path>` selects another
   hub; `connect --hub <url> --token-file <file>` connects to a remote hub. Without a stored hub and without `--hub`,
   connect fails with the exact `skillhub init` command; it never creates a hub.
5. **No preview-then-`--yes`.** `init`, `connect`, `doctor`, `cache clear`, `token rotate` and the rest of the
   onboarding surface act directly and print what they changed; `--dry-run` previews. connect keeps its safe merge
   (never overwrites user content, refuses on a conflicting managed block).
6. **doctor and status, by role.** `doctor` only reports; `doctor --fix` fixes directly and prints what changed, what
   it could not fix and why. `status` runs a lightweight subset of the same checks from the same code and always ends
   with one runnable next command. Both are context-aware by current directory for both roles:
   - hub folder: Git repo, schema, catalog/index, the hub's own agent connection files, listener state if serving;
   - client project: connection files per agent host exist and are valid, stored hub path still exists (local) or the
     remote hub is reachable, token file present and accepted, hub version compatible, skill cache writable and last
     fetch ok. `doctor --fix` in a client re-runs the safe connect merge and re-points a moved hub path; it cannot fix
     an unreachable remote and says so;
   - both roles apply: report both sections;
   - neither: say so and print the exact `cd <folder> && skillhub init` or `skillhub connect` command.
   One shared role-detection function (`DetectRole`) feeds status, doctor and every "no hub found" error. validate,
   rebuild, migrate and diff fold into doctor/status or a `maintain` group.
7. **Serve.** `skillhub serve [mcp|web] [--addr host:port[,host:port...]]`; no subcommand means mcp. Default bind
   0.0.0.0 for mcp (7420) and web (7421). `--addr` takes 1-3 comma-separated addresses: one listener each, one server,
   one token; any bind failure stops the command naming the address and reason; duplicates or overlaps are an error;
   a missing port uses the mode default. `--trust-private-network` and the 0.0.0.0/[::] refusal are removed. On start,
   print every usable address with a ready `skillhub connect --hub http://<addr>:7420 --token-file <file>` command
   and one warning line about listening on all interfaces and `--addr 127.0.0.1:7420`. A token is always required.
   The Host check accepts every address the listener is bound to, or any address a client uses when bound to
   0.0.0.0. The web default no longer depends on the number of interfaces. Token hand-off: print a ready `scp` line.
8. **Remove aliases and duplicates**: `integrate`, `web`, `mcp serve`, `check`. `mcp proxy` stays as an internal
   command hidden from help. Add `--version`. Hide developer commands (`eval`, `resolution`, `telemetry`, `resolve`);
   `help --dev` shows them. One spelling for scope and `--hub` flags. Plain language in first-run output (no
   "canonical", "proposal", "digest", "runtime block"); fix the preview path shown as `.` and the wrapped `git -C`
   lines; fix `skill.go:29` vs `skill.go:271` usage strings and the `--since` format mismatch.
9. **First skill.** The README stops at "restart the agent and ask it: curate my Skill Hub". A guided `skill create`
   in a terminal is a later, optional phase; flags remain for scripts and agents.
10. **No pairing codes.** Token transfer is a printed `scp` line plus `--token-file`. No automatic push beyond
    decision 3.

## Planner decisions (derived from the code; see Resolved questions)

| # | Decision | Evidence and reason |
|---|---|---|
| P1 | The stdio server that agents launch becomes the hidden `skillhub mcp stdio`; `serve mcp` is the network listener only. | Hosts launch `mcp serve --workspace <hub>` today (`internal/hostintegration/integration.go:328,346,357,361,374`, `runtime_access.go:185,189,194`). Decision 7 makes bare `serve` a 0.0.0.0 listener, so stdio needs its own name; `mcp stdio`/`mcp proxy` is the hidden pair agents call. Old registrations are reported as outdated by doctor and rewritten by `doctor --fix`. |
| P2 | `skillhub version` stays (unlisted) next to the new `--version`. | The self-updater of every installed release runs `<new binary> version` (`internal/selfupdate/selfupdate.go:912`); installers do too (`scripts/install.sh:319`, `scripts/test-installer-lifecycle.sh:347`). Removing it breaks `skillhub update` from older releases. |
| P3 | `--yes` stays on curation mutations (`skill *`, `source *`, `maintain migrate`, `telemetry purge/export`). | Decision 5 names the onboarding surface. The Claude permission rule `ask Bash(skillhub * --yes*)` (`internal/hostintegration/cli-permissions.go:14`) is the human approval gate for agent-driven curation. |
| P4 | Commands that lose `--yes` get their own `ask` permission rules: `skillhub init*`, `skillhub connect*`, `skillhub doctor --fix*`, `skillhub token rotate*`, `skillhub cache clear*`. | Without them an agent allowed `Bash(skillhub:*)` (`cli-permissions.go:13`) could rewrite agent configs or rotate the token with no prompt. |
| P5 | The hub's own connection files are ignored through `.git/info/exclude`, not `.gitignore`. | `git status --untracked-files=all` makes every untracked file "dirty" (`internal/app/operations.go:152`, `curation_home.go:115`), so a fresh hub reports uncommitted changes forever (verified, A6 below). `.git/info/exclude` is local, so cloned hubs need no content change (decision 3 "change no content"). |
| P6 | The per-user config is `<os.UserConfigDir()>/skillhub/config.json` (`{"hub": "<abs path>"}`, mode 0600, atomic write). Only `init` writes it; `connect <path>` does not change it. | No per-user config exists today (only `$XDG_STATE_HOME` for the remote cache, `internal/skillruntime/remote_cache.go:385-399`). One key keeps it minimal. |
| P7 | No interactive confirmation prompt. | Decision 5 allows one; KISS and scriptability favor none, since the merge is safe, `--dry-run` previews and `connect --remove` undoes. |
| P8 | `--workspace <path>` and `SKILLHUB_WORKSPACE` remain as advanced options in command help, absent from docs examples; `connect` drops `--workspace` (positional `<path>` instead) and `--project` (use `cd`). `--hub` always means a URL: `cache clear [--hub <url>]`. | Decision 1 and 8. Agents' stdio registrations still pass `--workspace` to `mcp stdio`. |
| P9 | Maintenance commands move under `skillhub maintain validate|rebuild|migrate|diff`; doctor already validates and `doctor --fix` already rebuilds a stale index (`internal/app/workspace.go:197-230`). | Decision 6; `validate --staged` keeps working for pre-commit hooks (`docs/curating-skills.md:291`). |
| P10 | `init --force` is removed. | Decision 3 forbids implicit targets; a non-empty folder is refused with the exact command to create an empty one. |

## Verified current behavior (2026-10-11, binary from this branch, isolated HOME and XDG_* in a scratch dir)

| Case | Current behavior | Target |
|---|---|---|
| A1 `init` in empty folder | Preview only; lists `.git`, schema, `.gitignore` findings; exit 0 | Acts directly |
| A2 `init --yes` in empty folder | Creates hub, `git init`, writes Claude/Codex/Gemini files, **no commit**; says "Workspace ready" (not "created"); long paths and `git -C` commands wrap onto separate lines | Creates, commits, stores path, unwrapped output |
| A3 re-run in hub | No change, same wrapped next steps | Same, plus store path |
| A4 `init` in a hub subfolder | "refusing nested workspace inside Git repository"; fix points at `doctor --workspace` | "This folder is inside the hub at X" + `cd X && skillhub status` |
| A6/A7 status vs doctor in fresh hub | status: "Git has uncommitted changes"; doctor: "Workspace is healthy." (contradiction) | One check set; both agree |
| B1 empty Git repo | Refused: "Directory is not empty" (`.git` counts as an entry, `internal/app/workspace.go:26-37`) | Accepted |
| C1/C3 folder with files (Git or not) | Refused, suggests `--force` | Refused with `mkdir ... && cd ... && skillhub init` |
| C2 repo with go.mod | Refused by the non-empty check before `looksLikeSourceCheckout` (`internal/workspace/workspace.go:375`) | Refused, same message as C1 |
| C4 empty folder inside another repo | "refusing nested workspace inside Git repository" (`workspace.go:310`), fix points at doctor | Plain message naming the parent repo and a folder outside it |
| D1 cloned hub | Works; its connection files then show as untracked | Works; files excluded locally; no content change |
| E1 `init https://...` | Treated as a path: preview says "Workspace directory will be created" | Remote inspection per decision 3 |
| F1 `token show` without TTY | Refuses, prints token file path; exit 1 | Unchanged; banner prints `scp` line |
| F2 `--version` | "The command "--version" is not supported"; exit 2 | Prints version |
| F3-F5 connect/doctor/status in a folder that is neither | "No Skill Hub workspace was found"; fix says `skillhub init <path> --yes` | Role-aware message with exact command |
| F8 status in connected project | Works through the connection file; ends "Next: Continue normal work" (no command) | Client section + runnable next command |
| F10 `serve mcp --addr 0.0.0.0:0` | Refused (plain HTTP warning, `internal/delivery/cli/mcp.go:146-165`) | Default bind |
| R2 doctor in remote-connected project | Remote report works (reachable FAILED, fix line) but **exit 0** | Exit non-zero on problems |
| R3 `doctor --fix` in remote project | "No Skill Hub workspace was found" (remote branch requires `!fix`, `internal/delivery/cli/workspace.go:130`) | Re-runs safe merge; says unreachable cannot be fixed |
| R4 status in remote project | "No Skill Hub workspace was found" | Client-remote section |
| R5 doctor in a subfolder of remote project | "No Skill Hub workspace was found" (`FindRemoteEntries` reads only cwd, `internal/app/doctor_remote.go:342`) | Walks up like local detection |
| R6 `skill list` in remote project | "No Skill Hub workspace was found" (extractors read only `--workspace`, `internal/delivery/cli/workspace_resolve.go:246-271`) | "This project uses the hub at <url>; curation runs on the hub machine" |
| Web Host check on 0.0.0.0 | Accepts loopback, `[::1]`, any local IPv4 (refreshed every 5 s), hostname; rejects other names, foreign IPs and other ports (HTTP 421) (`internal/delivery/web/security.go:26`) | One shared host policy for both servers |
| Web banner on 0.0.0.0 | Lists every up interface incl. `docker0` and `br-*` bridges; warning names `--loopback-only` | Usable addresses only; new warning line |

Existing remote doctor (keep and reuse): when this project (cwd only) or the user scope has a `mcp proxy` entry,
doctor checks hub token readable, hub reachable, token accepted, hub version, skill cache writable and last fetch
(`internal/app/doctor_remote.go:389-480`); on a machine without a hub it checks only the remote hub
(`internal/delivery/cli/workspace.go:126-133`). Existing local client checks: doctor compares project (cwd only) and
user-scope local entries with the expected plan and suggests `connect` (`internal/app/host-connections.go:87-112`).
New: walk-up detection, the `both` case, moved-hub detection and re-pointing, client checks in status, `doctor --fix`
for clients, non-zero exit on problems, role-aware "no hub" errors.

## Data flows

- **`init` (no URL):** cwd -> `DetectRole` (refuse if inside another repo or a hub subfolder) -> emptiness check
  (`.git` alone counts as empty) -> layout + `git init` -> remediation files -> catalog build -> hub self-connection
  (`applyHostIntegration`) -> append connection paths to `.git/info/exclude` -> first commit of hub-owned paths ->
  write user config `hub` -> print created paths, commit id, next command `cd <project> && skillhub connect`.
- **`init <git-url>`:** `git ls-remote <url>` -> error: print Git's stderr verbatim + one hint; no refs: same as
  `init` then `git remote add origin` + `git push -u origin HEAD`, print pushed branch and commit; refs:
  `git clone --no-checkout <url> .` -> `git ls-tree HEAD .skillhub/schema-version` -> hub: checkout, build index,
  self-connect, exclude, store path; not a hub: remove the `.git` it created (folder was empty), refuse.
- **`connect`:** flags -> target hub (positional path, else user config `hub`, else `--hub` URL) -> missing: error with
  the exact `init` command -> `hostintegration.Plan` -> `Apply` (or print the plan with `--dry-run`) -> print changed
  files per agent and the next step.
- **`DetectRole(cwd, home, storedHub)`:** hub root = nearest ancestor below home with `.skillhub/schema-version`;
  project registrations = nearest ancestor below home with a skillhub entry, read by
  `hostintegration.ReadRegistration` (`internal/hostintegration/connection.go:31`); user-scope registrations from the
  adapters' user config paths. A registration inside the hub root that points at that hub is the hub's
  self-connection (hub role only). Result: `hub`, `client-local`, `client-remote`, `both`, or `none`, with the
  per-host entries and the stored hub.
- **`serve`:** parse mode and `--addr` list -> validate (port default, duplicates, wildcard overlap) -> open every
  listener (close all and stop on the first failure) -> one server, one token, one host policy -> banner per usable
  address.

## Phases

| # | Phase | Effort | Depends on |
|---|---|---|---|
| 1 | [Syntax cleanup](./phase-01-syntax-cleanup.md) | 1d | none |
| 2 | [init, connect, user config and role detection](./phase-02-init-connect-and-role-detection.md) | 1.5d | 1 |
| 3 | [serve and token hand-off](./phase-03-serve-and-token-handoff.md) | 1d | 1 |
| 4 | [status and doctor by role, maintain group](./phase-04-status-doctor-and-maintain.md) | 1.5d | 2, 3 |
| 5 | [Docs: README onboarding and guides](./phase-05-docs-readme-and-guides.md) | 1d | 2, 3, 4 |
| 6 | [Guided skill create (optional, later)](./phase-06-guided-skill-create.md) | 0.5d | 1 (5 for its doc section) |

Run the phases in order 1 -> 2 -> 3 -> 4 -> 5. Phase 3 has no logic dependency on 2 and may be swapped with it, but
both edit `internal/delivery/cli/help.go`, so they are not run in parallel. Phase 6 can start any time after 1.

## Acceptance criteria

- [ ] From an empty folder, `skillhub init` creates a committed hub with no untracked files, stores its path, and
      prints one next command; `cd <project> && skillhub connect` connects all agents with no other argument.
- [ ] Every `init` case in the table above behaves as its "Target" column; each has a test.
- [ ] `skillhub --version`, `skillhub help --dev`, and no `integrate`, `web`, `mcp serve`, `check`, `--yes` (on
      onboarding commands), `--trust-private-network`, `--loopback-only`, `init --force` anywhere in code, help or
      user docs.
- [ ] `skillhub serve` listens on `0.0.0.0:7420`, prints one `connect` line per usable address plus a `scp` line, and
      a client using any of those addresses passes the Host check; `--addr a,b` opens two listeners with one token.
- [ ] `status` and `doctor` agree in every role fixture (hub, client-local, client-remote, both, none); status always
      ends with a command accepted by `skillhub help`; doctor exits non-zero when it reports a problem.
- [ ] README holds Install + Step 1 + Step 2 + Check; `docs/getting-started.md` is gone and no link points to it; the
      docs-vs-help test checks every `skillhub` command in README and the touched guides.
- [ ] `make check` green locally; CI green on Linux, macOS and Windows.

## Risks

| Risk | L x I | Mitigation |
|---|---|---|
| Existing users' agent registrations call `mcp serve`, which disappears | High x High | doctor flags registrations whose args differ from the expected plan as outdated; `doctor --fix` and `connect` rewrite them; README "Check" section names the fix. Accepted by "no backward compatibility". |
| Agents run `connect`, `doctor --fix`, `token rotate` without a human prompt once `--yes` is gone | Medium x High | New `ask` permission rules (P4) in the same phase that removes `--yes`; test asserts the rule set. |
| 0.0.0.0 default exposes the token over plain HTTP on every interface | High x Medium | Decision 7 trade-off, recorded below; banner warning line; docs say `--addr 127.0.0.1:7420` plus SSH tunnel on untrusted networks; token always required; auth-failure throttling stays (`internal/delivery/mcpserver/http.go:425`). |
| Auto push in `init <empty-remote>` pushes to the wrong remote | Low x Medium | Push only when `ls-remote` returned no refs; print remote URL, branch and commit; never force-push. |
| Tests write to the real user config on Windows (`os.UserConfigDir` reads `%AppData%`) | Medium x High | Add `APPDATA` and `LOCALAPPDATA` to every `TestMain` isolation list (`internal/delivery/cli/main_test.go:56-58` and the app, hostintegration, mcpserver and web equivalents). |
| `cd <folder> && ...` fails in Windows PowerShell 5.1 | High x Low | Windows blocks in docs use `cd <folder>; skillhub init`; printed next commands use the platform form. |
| Two quoting styles in printed commands (`app.shellArgument` vs `cli.shellQuote`, backlog F15) break copy-paste on Windows | Medium x Medium | Phase 1 makes one platform-aware quoting helper used by every printed command. |
| macOS temp paths are long and symlinked (`/var` -> `/private/var`) | Medium x Medium | Tests compare paths with `EvalSymlinks` (as `samePath` does, `internal/workspace/workspace.go:318`); commands print via the non-wrapping `termui.Command` (`termui.go:264`). |
| Git identity missing on a fresh machine makes the first commit fail | Medium x Medium | Resolved question 1; tests set an identity explicitly. |
| Windows and macOS cannot be tested locally | High x Medium | CI matrix is the gate; each phase lists the platform-sensitive tests; wait for green before merging. |

### Security trade-off (decision 7)

With the default bind, the hub token and all MCP traffic (task text, skill content) cross every network interface in
plain HTTP; anyone on the same LAN can sniff the token and call the runtime tools. The web session token is in the URL
fragment and exposed the same way. This is accepted for LAN and Tailscale convenience. Mitigations: token always
required, failed-auth throttling, Host check against DNS rebinding, a warning line on every start, and docs telling
users on untrusted networks to run `skillhub serve --addr 127.0.0.1:7420` and reach it through `ssh -N -L`.

## Old-name inventory (every place to update; excludes `archive/`, `plans/`, `.claude/`)

| Old name | Files |
|---|---|
| `integrate` | `internal/delivery/cli/root.go:37`, `help.go:454`, `internal/app/curation_home.go`, `internal/hostintegration/types.go`, `internal/app/workspace_test.go`, `internal/delivery/cli/workspace_next_steps_test.go:23` |
| `skillhub web` | `internal/delivery/cli/root.go:73-74`, `help.go:429-432` |
| `mcp serve` / `"mcp","serve"` | `internal/delivery/cli/mcp.go`, `help.go`, `root.go`; `internal/hostintegration/integration.go`, `runtime_access.go`, `toml_update.go`, `connection.go`, `matrix.json`; tests: `internal/delivery/cli/{onboarding,serve_mcp,connect_profiles,doctor-safety,mcp}_test.go`, `testdata/connect-workspace/{global,project}-files.golden`, `internal/delivery/mcpserver/{hardening,memory_budget,remote_bench,remote_budget_bench,remote_e2e,skill_tools,subprocess}_test.go`, `internal/hostintegration/integration_test.go`; `Makefile:19`, `scripts/mcp-fanout-bench.sh`; docs: `docs/command-reference.md`, `docs/remote-hub-guide.md`, `docs/release-runbook.md`, `docs/mcp-compatibility-matrix.json`, `docs/design/remote-hub.md`, `docs/design/07-storage-and-mutation-model.md`, `docs/distillery/lessons/skill-over-mcp.yaml` |
| `skillhub check` | `internal/delivery/cli/root.go:59`, `help.go:215-226`, `next_commands.go`, `skill_add.go`, `skill_add_test.go`, `internal/app/source_import.go`, `system-skills/curator/SKILL.md` and `internal/systemskills/curator/SKILL.md` (must stay identical, `internal/systemskills/embed_test.go:22`), `docs/command-reference.md`, `docs/curating-skills.md`, `docs/sources-and-learning.md`, `docs/web-ui.md`, `docs/getting-started.md`, `docs/design/05-*.md`, `docs/design/06-*.md`, `docs/use-cases/03-*.md` |
| `--trust-private-network` | `internal/delivery/cli/mcp.go`, `help.go`, `serve_mcp_test.go`, `internal/delivery/mcpserver/http.go` (`TrustPrivateNetwork`, `CheckListenAddress`), `http_test.go`, `internal/app/doctor_remote.go`, `docs/command-reference.md`, `docs/remote-hub-guide.md`, `docs/release-runbook.md`, `docs/design/remote-hub.md` |
| `--loopback-only`, `SKILLHUB_WEB_ADDR` | `internal/delivery/cli/serve.go:78`, `help.go`, `serve_test.go`, `internal/delivery/web/listen.go`, `README.md`, `docs/{command-reference,getting-started,release-runbook,sources-and-learning,troubleshooting,web-ui}.md`, `docs/contracts/web-api.md`, `.github/workflows/release.yml:732,840`, `web/e2e/smoke.spec.ts` (test name only) |
| `init <path> --yes`, `init ~/...` | `internal/delivery/cli/help.go:14-16,59-66`, `workspace.go:20,63-66`, `workspace_resolve.go:15`, `internal/app/workspace.go:33,52`, `internal/app/connect.go`, `scripts/install.sh:751,811`, `scripts/install.ps1:670,811`, `.github/workflows/release.yml:730,837`, `.github/workflows/evaluation.yml:43`, `web/e2e/support/server.ts:33`, `README.md`, `docs/getting-started.md`, `docs/troubleshooting.md`, `docs/design/01-system-architecture.md` |
| `connect --workspace`, `--project` | `internal/delivery/cli/connect.go`, `help.go`, `workspace.go:64-65`, `internal/app/connect.go`, `internal/app/host-connections.go:97-99`, tests `connect_*_test.go`, `onboarding_test.go`, `internal/delivery/mcpserver/remote_e2e_test.go`, docs `README.md`, `docs/{getting-started,remote-hub-guide,troubleshooting,user-guide}.md`, `docs/design/remote-hub.md` |
| `--yes` on onboarding commands | `internal/delivery/cli/{workspace,connect,cache,token}.go`, `help.go`, `internal/app/{workspace,connect,connect_remote,doctor_remote,host-connections,errors}.go`, 32 test files under `internal/{delivery/cli,app,hostintegration}` (grep `--yes`), `README.md`, `docs/*.md`, `system-skills/curator/SKILL.md:134` |
| `validate`, `rebuild`, `migrate`, `diff` (top level) | `internal/delivery/cli/root.go`, `help.go`, `catalog.go`, `migration.go`, `curation.go`, `validate_staged.go`, `next_commands.go`, `internal/app/{workspace,migration,operations,metadata_lint,skill_resource,skill_lifecycle,skill_review,curation_home}.go`, `system-skills/curator/SKILL.md:132,162,235`, `.github/workflows/evaluation.yml:46`, `docs/{command-reference,curating-skills}.md` |
| `token show` advice | `internal/delivery/cli/{token,mcp,connect_remote}.go`, `internal/app/doctor_remote.go`, `internal/hubtoken/hubtoken.go`, `docs/remote-hub-guide.md`, `docs/command-reference.md` |
| `getting-started.md` links | `README.md:98,122`, `docs/README.md:7`, `docs/curating-skills.md:5,115`, `docs/installation.md:25`, `docs/troubleshooting.md:83`, `docs/user-guide.md:3,25,120` |

Design records (`docs/design/*`, `docs/use-cases/*`, `docs/distillery/*`) are historical and in Vietnamese; Phase 5
adds a one-line note at the top of `docs/design/remote-hub.md` only, and leaves the others as records.

## Backwards compatibility

Not required (user decision). Consequences handled: old agent registrations are detected and rewritten by
`doctor --fix`; `skillhub version` stays for the self-updater (P2); the MCP tool names and the result envelope do not
change.

## Rollback

Each phase is one or more commits on a feature branch; `git revert` of a phase restores its behavior. Phase 1 and
Phase 2 change agent registrations: rolling back after users re-connected requires `connect` again with the old
binary, so phases 1-4 ship in one release, never separately.

## Resolved questions (user confirmed 2026-10-11)

1. Missing Git identity on the first commit: `init` commits as `Skill Hub <skillhub@localhost>` for that commit only and
   says so in the output.
2. `init <git-url>` inside an existing hub with no remote, remote empty: push the existing history. Never push when the
   remote already has refs.
3. The stdio entry agents launch is the hidden `skillhub mcp stdio` (P1). Old registrations are rewritten by
   `doctor --fix`.
4. `connect <path>` does not change the stored hub (P6).
5. `--yes` stays on curation commands and `maintain migrate` (P3).
6. A project entry wins over a user-scope entry with the same server name in Claude Code, Codex and Gemini CLI
   (see [scope precedence report](../reports/researcher-261011-mcp-config-scope-precedence.md)). Claude Code and
   Gemini CLI replace the whole entry; Codex merges the two entries key by key (from source, not documented), so
   stale keys from a user-scope entry can leak into a project entry. Consequences for this plan: the project entry
   that `connect` writes must set every field it needs, and `status` explains a folder that is both a hub and a
   client by naming the winning scope. Untested: whether Codex rejects a merged entry that mixes transports.
