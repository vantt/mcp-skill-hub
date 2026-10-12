---
title: "Onboarding UX redesign: init, connect, serve, status and README"
description: "Make Skill Hub onboarding two steps (install the hub, install the client project) with the fewest commands, plain syntax, role-aware status and doctor, and a README that holds the whole onboarding."
status: completed
priority: P1
effort: 12.5d
branch: worktree-agent-a5c101367df3589e1
tags: [cli, onboarding, init, connect, serve, doctor, status, docs]
blockedBy: []
blocks: [261011-2153-package-distill-lab-in-the-hub]
created: 2026-10-11
---

# Onboarding UX redesign

## Goal

A new user installs `skillhub`, runs one command in an empty folder to get a hub, runs one command in a project to
connect it, and asks the agent "curate my Skill Hub". Two roles exist: the **hub** (a folder with skills, a Git
repository) and the **client project** (a project whose agents use a hub on this machine or on another one).
Every command acts on the current directory, acts directly, prints what it changed, and ends with one runnable next
command. Backward compatibility is not required: old names and flags are removed, not aliased. The one exception is
the stdio launch command agents already have in their registrations, which keeps its name (P1).

Reference: [CLI onboarding patterns research](../reports/researcher-261011-cli-onboarding-patterns.md) (in the main
checkout; patterns 1-9 adopted, pattern 10 "pairing codes" deferred per decision 10).

## Decisions (made by the user; not reopened)

1. **Docs convention.** Every hub command is written `cd <folder> && skillhub <command>`. Hub commands act on the
   current directory (discovery already walks up to the hub root, `internal/workspace/workspace.go:66-85`). No
   `--workspace` in any example.
2. **Two-step README.** After Install: Step 1 install the hub, Step 2 install the client project. The README holds the
   whole onboarding; `docs/getting-started.md` is merged into it and deleted. The install one-liner lives only in the
   README; `docs/installation.md` keeps details.
3. **Hub creation is `skillhub init` in the current, empty folder.** No subfolder, no implicit directory, no `--from`,
   the user never types a Git command, the first commit is automatic. `skillhub init <git-url>` inspects the remote:
   it has a hub -> fetch it into the current folder and build the index; it is empty -> create a hub here, commit,
   push the first commit, set `origin` and print what was pushed (the only automatic push); it has other content ->
   refuse with guidance; unreachable -> print Git's error verbatim plus one hint line. `init <git-url>` inside an
   existing hub without a remote binds `origin` (refuse on unrelated history). `init` is idempotent (empty folder,
   empty Git repo, already-fetched hub: build index, store hub path, change no skill content; other repo with content:
   refuse with a concrete next step). `init` keeps writing the agent connection files for the hub folder itself.
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
   - neither: say so and print the exact `cd <folder> && skillhub init` or `skillhub connect` command (refined by F5:
     `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`, which is the same command with the folder created).
   One shared role-detection function (`DetectRole`) feeds status, doctor and every "no hub found" error. validate,
   rebuild, migrate and diff fold into doctor/status or a `maintain` group.
7. **Serve.** `skillhub serve [mcp|web] [--addr host:port[,host:port...]]`; no subcommand means mcp. Default bind
   0.0.0.0 for mcp (7420) and web (7421). `--addr` takes 1-3 comma-separated addresses: one listener each, one server,
   one token; any bind failure stops the command naming the address and reason; duplicates or overlaps are an error;
   a missing port uses the mode default. `--trust-private-network` and the 0.0.0.0/[::] refusal are removed. On start,
   print every usable address with a ready `skillhub connect --hub <url> --token-file <file>` command and one warning
   line about listening on all interfaces (on both the mcp and the web banner). A token is always required. The Host
   check accepts every address the listener is bound to, or any IP literal a client uses when bound to a wildcard
   address. The web default no longer depends on the number of interfaces. Token hand-off: print a ready `scp` line.
   **Revised 2026-10-11:** the web UI also keeps the 0.0.0.0 default although it has write routes; the user accepted
   that risk (see Security trade-off). No loopback-only write gate, no `--allow-remote-edit`.
8. **Remove aliases and duplicates**: `integrate`, `web`, `check`. `mcp serve` stays as the hidden stdio command agents
   launch (P1, revised 2026-10-11) and `mcp proxy` stays as its hidden remote counterpart. Add `--version`. Hide
   developer commands (`eval`, `resolution`, `telemetry`, `resolve`); `help --dev` shows them. One spelling for scope
   and `--hub` flags (P8). Plain language in first-run output (no "canonical", "proposal", "digest", "runtime block");
   fix the preview path shown as `.` and the wrapped `git -C` lines; fix `skill.go:29` vs `skill.go:271` usage strings
   and the `--since` format mismatch.
9. **First skill.** The README stops at "restart the agent and ask it: curate my Skill Hub". A guided `skill create`
   in a terminal is a later, optional phase; flags remain for scripts and agents.
10. **No pairing codes.** Token transfer is a printed `scp` line plus `--token-file`. No automatic push beyond
    decision 3.

## Planner decisions (derived from the code; see Resolved questions)

| # | Decision | Evidence and reason |
|---|---|---|
| P1 | **Revised 2026-10-11.** The stdio server that agents launch keeps the hidden name `skillhub mcp serve`. No `mcp stdio`, no registration rewrite, no migration of existing registrations. Bare `serve` and `serve mcp` mean the network listener; `mcp` is hidden from `skillhub help` and `skillhub help mcp` still prints its usage. | User decision. Hosts launch `mcp serve --workspace <hub>` today (`internal/hostintegration/integration.go:328,346,357,361,374`, `runtime_access.go:185,189,194`); `mcp serve` already rejects network flags (`internal/delivery/cli/mcp.go:24-31`, `parseMCPServeFlags` with `allowNetwork=false`), so it is a stdio-only command and does not collide with the new `serve`. |
| P2 | `skillhub version` stays (unlisted) next to the new `--version`. | The self-updater of every installed release runs `<new binary> version` (`internal/selfupdate/selfupdate.go:912`); installers do too (`scripts/install.sh:319`, `scripts/test-installer-lifecycle.sh:347`). |
| P3 | `--yes` stays on curation mutations (`skill *`, `source *`, `maintain migrate`, `telemetry purge/export`). | Decision 5 names the onboarding surface. The Claude permission rule `ask Bash(skillhub * --yes*)` (`internal/hostintegration/cli-permissions.go:14`) is the human approval gate for agent-driven curation. |
| P4 | **Revised 2026-10-11.** Commands that lose `--yes`, plus `serve`, get position-independent `ask` rules: `Bash(skillhub *init*)`, `Bash(skillhub *connect*)`, `Bash(skillhub *--fix*)`, `Bash(skillhub *token rotate*)`, `Bash(skillhub *cache clear*)`, `Bash(skillhub *serve*)`. doctor reports a stale rule set as outdated and `doctor --fix` rewrites it. Codex and Gemini CLI have no permission file: accepted risk, mitigated by curator-skill text and a terminal guard on `token rotate`. | Flags may come in any order (the per-command flag loops accept any order, for example `internal/delivery/cli/workspace.go:235` `initFlags`), so a prefix rule such as `Bash(skillhub doctor --fix*)` misses `skillhub doctor --json --fix`. `serve` binds 0.0.0.0 by default (decision 7). Rules are only written at connect time (`cli-permissions.go:9-16`), so existing projects need doctor to update them. |
| P5 | **Dropped 2026-10-11.** No `.git/info/exclude`. The hub keeps main's `.gitignore` mechanism (`requiredIgnoreEntries`, `internal/workspace/workspace.go:38-41`; findings `gitignore_entry_missing` and `machine_file_tracked`, `:172-179`; tests `internal/workspace/machine_files_test.go`). | User decision; commit b734bc9 on main. The distill-lab plan extends the same `.gitignore` list for generated skill folders (see Coordination). |
| P6 | **Revised 2026-10-11.** The per-user config is `<os.UserConfigDir()>/skillhub/config.json` holding `{"hub": "<abs path>", "previous_hubs": ["<abs path>", ...]}` (at most 5 previous paths, mode 0600, atomic write). Only `init` writes it; `connect <path>` does not change it. One override, `SKILLHUB_USER_CONFIG_DIR`, replaces the directory for Go tests, web e2e and installer smoke. | No per-user config exists today (only the remote cache under the state dir, `internal/skillruntime/remote_cache.go:385-399`). `previous_hubs` lets `doctor --fix` re-point only entries that pointed at a hub this machine recorded (F4). The override name avoids `SKILLHUB_CONFIG_DIR`, which is already the per-skill config directory exported to skill scripts (`internal/app/skill_snapshot.go:510`). |
| P7 | No interactive confirmation prompt. | Decision 5 allows one; KISS and scriptability favor none, since the merge is safe, `--dry-run` previews and `connect --remove` undoes. |
| P8 | `--workspace <path>` and `SKILLHUB_WORKSPACE` remain as advanced options in command help, absent from docs examples; `connect` drops `--workspace` (positional `<path>` instead) and `--project` (use `cd`). `--workspace` is always a local path; `--hub` is always a URL: `cache clear [--hub <url>]`. Help says so in one line on every command that takes either. | Decision 1 and decision 8 ("one spelling"). Agents' stdio registrations still pass `--workspace` to `mcp serve`. |
| P9 | Maintenance commands move under `skillhub maintain validate|rebuild|migrate|diff`; doctor already validates and `doctor --fix` already rebuilds a stale index (`internal/app/workspace.go:199-232`). Existing pre-commit hooks that run `skillhub validate --staged` break; doctor flags them and `doctor --fix` rewrites them (F8). | Decision 6. The hook pattern is documented in `docs/curating-skills.md:291-296`. |
| P10 | `init --force` is removed. | Decision 3 forbids implicit targets; a non-empty folder is refused with the exact command to create an empty one. |

## Verified current behavior (re-verified 2026-10-11 after rebasing onto main; `go install` binary from this branch, isolated HOME and XDG_*, `GIT_CONFIG_GLOBAL=/dev/null`, scratch dir)

| Case | Current behavior | Target |
|---|---|---|
| A1 `init` in empty folder | Preview only; lists `.git`, schema and six `.gitignore: Required Git ignore entry is missing` findings; exit 0 | Acts directly |
| A2 `init --yes` in empty folder | Creates hub, `git init` (branch from the user's Git config; `master` with an empty config), writes `.gitignore` with the six required entries so the four connection files are ignored, writes Claude/Codex/Gemini files, **no commit**; says "Workspace ready at" (not "created"); long paths and `git -C` commands wrap. Untracked afterwards: `.gitignore`, `.skillhub/schema-version`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.agents/skills/system-curator/SKILL.md`, `.claude/skills/system-curator/SKILL.md`, `.gemini/skills/system-curator/SKILL.md`, one `history/operations/...yaml`. The printed stage command (`stageWorkspaceCommand`, `internal/delivery/cli/workspace.go:71`) names only `.gitignore .skillhub skills sources history registry config distill evals`, so it would leave the instruction files and skill folders untracked | Creates, commits every file it wrote that `.gitignore` does not exclude, stores path, unwrapped output |
| A3 re-run in hub | No change; "Agent connection for this workspace is already in place." | Same, plus store path |
| A4 `init` in a hub subfolder | "refusing nested workspace inside Git repository"; fix points at `doctor --workspace`; exit 2 | "This folder is inside the hub at X" + `cd X && skillhub status` |
| A6/A7 status vs doctor in fresh hub | status: "Workspace is new; commit it, then connect an agent." / "Git has uncommitted changes", Next `skillhub diff --workspace <hub>`; doctor: "Workspace is healthy.", exit 0 (contradiction) | One check set; both agree |
| B1 empty Git repo | Refused: "Directory is not empty" (`.git` counts as an entry, `internal/app/workspace.go:22-37`); fix suggests `--force` and `connect --yes`; exit 2 | Accepted |
| C1/C3 folder with files (Git or not) | Refused, suggests `--force`; exit 2 | Refused with `mkdir ... && cd ... && skillhub init` |
| C2 repo with go.mod | Refused by the non-empty check before `looksLikeSourceCheckout` (`internal/workspace/workspace.go:342,405`) | Refused, same message as C1 |
| C4 empty folder inside another repo | "refusing nested workspace inside Git repository" (`internal/workspace/workspace.go:340`), fix points at doctor; exit 2 | Plain message naming the parent repo and a folder outside it |
| D1 cloned hub (`git clone` + `init --yes`) | Works; connection files are ignored by the cloned `.gitignore`; `git status` is clean when the clone's history already holds the instruction files | Same through `init <git-url>`; reports missing `.gitignore` entries and tracked machine files instead of failing |
| E1 `init https://...` | Treated as a relative path: preview says "`.`: Workspace directory will be created"; exit 0 | Remote inspection per decision 3 |
| F1 `token show` without TTY (in a hub) | Refuses, prints token file path; exit 1 | Unchanged; banner prints `scp` line |
| F2 `--version` | "The command "--version" is not supported"; exit 2 | Prints version |
| F3-F5 connect/doctor/status in a folder that is neither | "No Skill Hub workspace was found"; fix says `skillhub init <path> --yes` (`internal/delivery/cli/workspace_resolve.go:15`); exit 2 | Role-aware message with exact command |
| F8 status in connected project | Works through the connection file; ends "Next: Continue normal work" (no command) | Client section + runnable next command |
| F10 `serve mcp --addr 0.0.0.0:0` | Refused (plain HTTP warning, `internal/delivery/cli/mcp.go:146`, `mcpserver.CheckListenAddress` at `internal/delivery/mcpserver/http.go:84`) | Default bind |
| R2 doctor in remote-connected project | Remote report works (reachable FAILED, fix line names `skillhub serve mcp --addr 127.0.0.1:9` and an SSH tunnel) but **exit 0** | Exit non-zero on problems |
| R3 `doctor --fix --yes` in remote project | "No Skill Hub workspace was found" (remote branch requires `!fix`, `internal/delivery/cli/workspace.go:130`) | Refreshes local connection files; refuses to re-run the remote connect and prints the command (S1) |
| R4 status in remote project | "No Skill Hub workspace was found" | Client-remote section |
| R5 doctor in a subfolder of remote project | "No Skill Hub workspace was found" (`FindRemoteEntries` reads only cwd and user scope, `internal/app/doctor_remote.go:342`) | Walks up like local detection |
| R6 `skill list` in remote project | "No Skill Hub workspace was found" (extractors read only local `--workspace`, `internal/delivery/cli/workspace_resolve.go:171-330`) | "This project uses the hub at <url>; curation runs on the hub machine" |
| Web Host check on 0.0.0.0 (code read, not re-run) | Accepts loopback, `[::1]`, any local IPv4 (refreshed every 5 s), hostname; rejects other names, foreign IPs and other ports (HTTP 421) (`internal/delivery/web/security.go:26-90`) | One shared Host policy for both servers |
| Web banner on 0.0.0.0 (code read, not re-run) | Lists every up interface incl. `docker0` and `br-*` bridges; warning names `--loopback-only` | Usable addresses only; new warning line |

Existing remote doctor (keep and reuse): when this project (cwd only) or the user scope has a `mcp proxy` entry,
doctor checks hub token readable, hub reachable, token accepted, hub version, skill cache writable and last fetch
(`internal/app/doctor_remote.go:389-480`). It sends the token only to a hub named in the user scope or on loopback
(`tokenMaySend`, `:429-445`); a hub named only in project files gets no token (S1 keeps this). On a machine without a
hub it checks only the remote hub (`internal/delivery/cli/workspace.go:126-133`). Existing local client checks: doctor
compares project (cwd only) and user-scope local entries with the expected plan and suggests `connect`
(`internal/app/host-connections.go:87-112`). New: walk-up detection, the `both` case, moved-hub detection and
re-pointing, client checks in status, `doctor --fix` for clients, non-zero exit on problems, role-aware "no hub" errors.

## Data flows

- **`init` (no URL):** cwd -> `DetectRole` (refuse if inside another repo or a hub subfolder) -> emptiness check
  (`.git` with an unborn HEAD and init-owned paths count as empty; see the init table in Phase 2) -> `git init -b main`
  when no `.git` exists, writing the marker `.git/skillhub-created` -> layout -> remediation files incl. `.gitignore`
  -> catalog build -> hub self-connection (`applyHostIntegration`) -> first commit of exactly the paths this run wrote
  that `.gitignore` does not exclude, with hooks and signing disabled for that commit -> `SetHub(cwd)` -> print created
  paths, commit id, next command `cd <your-project> && skillhub connect`.
- **`init <git-url>`:** classify the argument (URL scheme, scp form, local repo path; refuse a leading `-`) ->
  `git ls-remote --symref` with the interactive runner -> error: print Git's stderr verbatim (userinfo redacted) + one
  hint; no refs: same as `init`, then `git push --no-verify <url> HEAD:refs/heads/<branch>`, and only after success
  `git remote add origin` + upstream; refs: `git init -b <branch>` (marker) or reuse the existing `.git` -> `git fetch
  <url>` into probe refs -> pick the branch holding `.skillhub/schema-version` (HEAD target first) -> trust check
  (`git ls-tree -r --name-only <ref>`) -> `git checkout -B <branch> <ref>` -> origin + upstream -> as "already a hub".
  Not a hub or untrusted content: delete probe refs; remove `.git` only when the marker shows this run created it.
- **`connect`:** flags -> target hub (positional path, else user config `hub`, else `--hub` URL) -> missing: error with
  the exact `init` command -> remote: refuse a token file already registered for a different hub -> `hostintegration.Plan`
  -> `Apply` (or print the plan with `--dry-run`) -> print changed files per agent and the next step.
- **`DetectRole(cwd, home string, cfg userconfig.Config) RoleReport`:** walk up from cwd to the filesystem root, stopping
  when the walk reaches home (home itself is not scanned), exactly like `resolveWorkspace`
  (`internal/delivery/cli/workspace_resolve.go:70-98`); hub root = nearest ancestor with `.skillhub/schema-version`;
  project registrations = nearest ancestor with a skillhub entry, read by `hostintegration.ReadRegistration`
  (`internal/hostintegration/connection.go:31`); user-scope registrations from the adapters' user config paths. A
  registration inside the hub root that points at that hub is the hub's self-connection (hub role only). Per host the
  effective entry is the project entry when one exists, else the user entry (resolved question 6). Result: `hub`,
  `client-local`, `client-remote`, `both`, or `none`, with per-host project, user and effective entries and the
  stored hub.
- **Health checks:** `RoleReport` -> `app.Run(ctx, report, depth)` (`internal/app/health.go`) -> registered `CheckProvider`s -> `[]Section`
  of `Finding{Code, Message, Fix}` -> status (light, unlocked) or doctor (full, shared lock) renderer -> `--fix` runs
  each finding's `Fix.Apply` and prints what changed. The token is sent only per `tokenMaySend`.
- **`serve`:** parse mode and `--addr` list -> validate (port default, duplicates, wildcard overlap, port 0 exempt from
  duplicate detection) -> open every listener (close all and stop on the first failure) -> one server, one token, one
  Host policy -> banner per usable address, with ports taken from each listener's actual `Addr()`.

## Phases

| # | Phase | Effort | Depends on | Status |
|---|---|---|---|---|
| 1 | [Syntax cleanup](./phase-01-syntax-cleanup.md) | 2d | none | completed |
| 2 | [init, connect, user config and role detection](./phase-02-init-connect-and-role-detection.md) | 4d | 1 | completed |
| 3 | [serve and token hand-off](./phase-03-serve-and-token-handoff.md) | 1.5d | 1, 2 | completed |
| 4 | [status and doctor by role, maintain group](./phase-04-status-doctor-and-maintain.md) | 3d | 2, 3 | completed |
| 5 | [Docs: README onboarding and guides](./phase-05-docs-readme-and-guides.md) | 1.5d | 2, 3, 4 | completed |
| 6 | [Guided skill create (optional, later)](./phase-06-guided-skill-create.md) | 0.5d | 1 (5 for its doc section) | pending |

Run the phases in order 1 -> 2 -> 3 -> 4 -> 5. Phase 3 uses the per-hub client token path helper from Phase 2, and
both edit `internal/delivery/cli/help.go`, so they are not run in parallel. Phase 6 can start any time after 1.

Sizing basis (counted 2026-10-11): 51 CLI `init` invocations in 23 test files and 46 direct `WorkspaceService.Init`
calls in 17 files change with the new `init` signature; 33 test files under `internal/{delivery/cli,app,hostintegration}`
pass `--yes` (195 occurrences, about 70 on onboarding commands); 73 cli/app test files use `t.Parallel`, so tests must not change
the process working directory (Phase 2 keeps a path parameter in `app.Init` and adds a test seam).

Each user-facing surface is reworded once, in the phase that owns it: Phase 1 only removes commands and drops `--yes`
tokens mechanically; resolution and connect messages in Phase 2; serve, token and remote-doctor fix texts in Phase 3;
doctor and status texts in Phase 4; docs in Phase 5.

## Coordination with the distill-lab packaging plan

Session mcp-skill-hub-68 owns [plans/261011-2153-package-distill-lab-in-the-hub/](../261011-2153-package-distill-lab-in-the-hub/plan.md),
which declares `blockedBy: [261011-2250-onboarding-ux-redesign]`. Agreed order: this plan lands and releases 5.0.0
first; the other session implements afterwards from the new main.

| Area | Owner |
|---|---|
| Skill-file writer path: `internal/hostintegration/curator-source.go`, the `SkillFiles`/curator part of `internal/hostintegration/integration.go` and its removal in `remote.go` | distill-lab plan |
| `internal/systemskills` restructuring | distill-lab plan |
| Distill sentences in `web/src/domain/handoff-brief.ts`, `web/src/screens/skill-detail/DistillTab.tsx`, `web/src/screens/distill/DistillHandoffScreen.tsx`, `internal/delivery/cli/source.go:717`, `docs/sources-and-learning.md`, `docs/web-ui.md` | distill-lab plan (this plan makes only mechanical command renames in those files) |
| `.gitignore` entries for generated `.agents`/`.claude` skill folders (extends `requiredIgnoreEntries`) | distill-lab plan |
| Launch args, permission rules, config entry writing, connect option parsing | this plan |
| `init`, `connect`, `doctor`, `status`, `serve`; command renames everywhere; README | this plan |
| Curator `SKILL.md` text (both identical copies, `system-skills/curator/SKILL.md` and `internal/systemskills/curator/SKILL.md`), text edits only | this plan |
| Health check extension point (`internal/app/health.go`: `CheckProvider`, `Finding`, `FixAction`, `RegisterProvider`) | this plan defines it (Phase 4); the distill-lab plan registers only notice-severity hub Findings through `RegisterProvider`: Node missing, a generated skill file tracked by Git, an unknown file in an ignored skill folder. Wrapper/canonical integrity of system skills does not go through `RegisterProvider`: it stays in the hostintegration inspection items, which doctor reports in the hub section (the hub's agent connection check) |

The connection summary wording `connection written: claude-code, codex-cli, gemini-cli` lives in one function
(Phase 2 names it); both plans reuse it. Because `init` commits "every file it wrote that `.gitignore` does not
exclude", the generated skill folders drop out of the first commit automatically once the other plan ignores them.

## Acceptance criteria

- [x] From an empty folder, `skillhub init` creates a committed hub with `git status --porcelain --untracked-files=all`
      empty, stores its path, and prints one next command; `cd <project> && skillhub connect` connects all agents with
      no other argument.
- [x] Every `init` case in the Phase 2 table behaves as specified (incl. unborn-HEAD resume, push retry, HEAD/branch
      mismatch, untrusted clone, pre-existing `.git`); each has a test, incl. injected-failure tests for a failing
      pre-commit hook, `commit.gpgsign=true`, an interrupted run and a full disk.
- [x] `skillhub --version`, `skillhub help --dev`; no `integrate`, `web`, `check`, `--yes` (on onboarding commands),
      `--trust-private-network`, `--loopback-only`, `init --force` anywhere in code, help, web UI, e2e or user docs
      (repo-wide grep gate). `mcp serve` is absent from help and docs examples but still launches stdio for existing
      registrations, which are not rewritten.
- [x] `skillhub serve` listens on `0.0.0.0:7420`, prints one `connect` line and one `scp` line per usable address and a
      warning line; `serve web` prints the warning line too; a client using any of those IP addresses passes the Host
      check, while an `Origin` from a foreign IP gets 403; `--addr a,b` opens two listeners with one token; port 0 works.
- [x] status, doctor and `doctor --fix` never send the hub token to a hub named only in project files (test).
- [x] The `ask` permission rules match permuted flag orders; doctor reports an outdated rule set and `doctor --fix`
      updates it.
- [x] `status` and `doctor` agree in every role fixture (hub, client-local, client-remote, both, mixed, none); status
      always ends with a command whose `skillhub` segments pass the help check; doctor exits non-zero when it reports a
      problem; `status --json` still validates against `schemas/curation-home-v1.schema.json`.
- [x] README holds Install + Step 1 + Step 2 + Check under the existing `#quickstart` anchor; `docs/getting-started.md`
      is gone (or a "Moved to" stub) and no link points to it; one docs-vs-help test checks every `skillhub` command in
      README and the touched guides plus the few non-command bans.
- [x] `make check` green locally; CI green on Linux, macOS and Windows; web e2e green.

## Risks

| Risk | L x I | Mitigation |
|---|---|---|
| Agents run `connect`, `init`, `doctor --fix`, `token rotate`, `serve` without a human prompt once `--yes` is gone | Medium x High | Position-independent `ask` rules (P4) in Phase 1, tested with permuted flags; doctor flags and fixes stale rule sets (Phase 4). Codex and Gemini: accepted risk (no permission file), curator-skill text asks first, `token rotate` refuses without a terminal. Open item: whether Claude Code splits `cd ... && skillhub ...` before matching (Phase 1 verification step). |
| Hub token sent to a hub named in a cloned project's files | Low x Critical | `tokenMaySend` is a required invariant of the check engine; client-remote `--fix` never re-runs `ConnectRemote` from project-file values (S1); test. |
| `init <git-url>` argument injection or credential leak (`-` options, `ext::`/`fd::` transports, userinfo in printed URLs, hooks in a pre-existing `.git`) | Low x High | Strict argument classification, `--`, `protocol.ext/fd.allow=never`, userinfo refused or redacted, hooks and signing disabled for init's commit and push, interactive runner separate from the offline runner (S3, S7). |
| A cloned "hub" carries tracked runtime files or agent configs that agents execute | Low x High | Trust check before checkout refuses tracked `runtime/` or `.skillhub/transactions/` and warns by name on agent config files (S4). |
| 0.0.0.0 default exposes the token and the web write routes over plain HTTP on every interface | High x High | Accepted by the user (Security trade-off below); warning line on both banners; token always required; docs route untrusted networks to `--addr 127.0.0.1:<port>` plus SSH tunnel; auth-failure throttling stays (`internal/delivery/mcpserver/http.go:425`). |
| Auto push in `init <empty-remote>` pushes to the wrong remote | Low x Medium | Push only when `ls-remote` returned no refs; print remote URL (redacted), branch and commit; never force-push. |
| Tests, e2e, CI or installer smoke write the real user config (`init` always stores the hub) | Medium x High | `SKILLHUB_USER_CONFIG_DIR` set by every `TestMain` isolation list, `web/e2e/support/server.ts`, `.github/workflows/evaluation.yml`, release smoke; `APPDATA`/`LOCALAPPDATA` added too; new `userconfig` tests use `t.Setenv` (F4). |
| Existing pre-commit hooks running `skillhub validate --staged` fail every commit after the `maintain` move | High x Medium | doctor finding + `doctor --fix` rewrite of `.git/hooks/pre-commit`; release note and troubleshooting entry (F8). |
| `cd <folder> && ...` fails in Windows PowerShell 5.1 | High x Low | Windows blocks in docs use `cd <folder>; skillhub init`; printed next commands use the platform form. |
| Four quoting helpers in printed commands break copy-paste on Windows | Medium x Medium | Phase 1 makes one platform-aware quoting helper (replaces `internal/app/connect_remote.go:325`, `internal/app/operations.go:711`, `internal/delivery/cli/next_commands.go:178`, `internal/delivery/cli/telemetry.go:261`). |
| macOS temp paths are long and symlinked (`/var` -> `/private/var`) | Medium x Medium | Tests compare paths with `EvalSymlinks` (as `samePath` does, `internal/workspace/workspace.go:348`); commands print via the non-wrapping `Printer.Command` (`internal/delivery/cli/termui/termui.go:264`). |
| Git identity missing or `commit.gpgsign` set on a fresh machine makes the first commit fail | Medium x Medium | Resolved question 1; `-c commit.gpgsign=false`; tests set identity explicitly and cover the failure paths (F1). |
| Windows and macOS cannot be tested locally | High x Medium | CI matrix is the gate; each phase lists the platform-sensitive tests; wait for green before merging. |
| Sizing: the `init` signature change touches ~97 test call sites | High x Medium | Path parameter kept in `app.Init`, CLI test helper avoids `chdir`; Phase 2 estimate based on counted call sites. |

### Security trade-off (decision 7, revised 2026-10-11)

With the default bind, both servers listen on every network interface over plain HTTP:

- **MCP (`serve`, 7420):** the hub token and all MCP traffic (task text, skill content) cross the network unencrypted.
  Anyone who can sniff the LAN can take the token and call the runtime tools.
- **Web (`serve web`, 7421):** the per-run session token sits in the URL fragment and the UI is reachable from the LAN.
  The web server has write routes, not only reads: skill add and confirm (`POST /api/v1/skills/add/confirm`,
  `internal/delivery/web/routes_skill_write.go:19`), skill proposal confirm (`:23`), upstream proposal confirm
  (`internal/delivery/web/routes_sources.go:21`), source proposal confirm (`:27`) and source import confirm (`:29`).
  Whoever obtains the session token can add or change skills in the hub. Because every connected client (local agents
  and remote proxies) loads skills from this hub, a write through the web UI is a supply-chain change that reaches
  every agent on every connected machine.

The user heard this and accepted it on 2026-10-11 for LAN and Tailscale convenience. No loopback-only write gate and
no `--allow-remote-edit` flag are added. Mitigations: token always required on both servers, failed-auth throttling
on MCP, Host check against DNS rebinding, a strict Origin check (S6; web mutations require `Origin` equal to
`http://` + Host, `internal/delivery/web/security.go:195`), a warning line on every start of both servers, and docs
telling users on untrusted networks to run `skillhub serve --addr 127.0.0.1:7420` (web: `--addr 127.0.0.1:7421`) and
reach it through `ssh -N -L`.

## Old-name inventory (regenerated 2026-10-11 with grep over `internal/**`, `web/src`, `web/e2e`, `scripts`, `.github`, `system-skills`, `docs`, `README.md`, `Makefile`; excludes `archive/`, `plans/`, `.claude/`)

| Old name | Phase | Files |
|---|---|---|
| `integrate` | 1 | `internal/delivery/cli/root.go:38`, `help.go:454`, `internal/app/curation_home.go`, `internal/hostintegration/types.go`, tests `internal/app/workspace_test.go`, `internal/delivery/cli/workspace_next_steps_test.go`, `internal/delivery/cli/curation_test.go:182`; matrix note `internal/hostintegration/matrix.json:155` |
| `skillhub web` | 1 | `internal/delivery/cli/root.go:74-75`, `help.go:429-432` |
| `skillhub check` | 1 | `internal/delivery/cli/root.go:60`, `help.go:215-226`, `next_commands.go`, `skill_add.go`, `internal/app/source_import.go`; tests `skill_add_test.go`, `source_test.go`, `source_watch_test.go`, `telemetry_lifecycle_test.go:69` (top-level `check`); curator skill `system-skills/curator/SKILL.md:90,154,209,334` and its identical copy `internal/systemskills/curator/SKILL.md` (`internal/systemskills/embed_test.go:22`); docs `command-reference.md`, `curating-skills.md`, `sources-and-learning.md`, `web-ui.md`, `getting-started.md` |
| `--yes` on onboarding commands | 1 | `internal/delivery/cli/{workspace,connect,connect_remote,cache,token}.go`, `help.go:14-16,59,67,104,349,363`, `internal/app/{workspace,connect,connect_remote,doctor_remote,host-connections,errors}.go`; `internal/delivery/mcpserver/proxy.go:944,949` (`cache clear --hub --yes`, also invalid under P8); `scripts/remote-timing.sh:110`; `scripts/install.sh:751,811`, `scripts/install.ps1:670,811`; 33 test files (195 occurrences, about 70 onboarding lines); docs |
| `mcp serve` in help/docs (command itself stays, hidden) | 1, 5 | `internal/delivery/cli/help.go:321,390`, `docs/command-reference.md:89`, `docs/remote-hub-guide.md:26`, `docs/release-runbook.md:164`; registration writers and their tests are **not** changed |
| `--trust-private-network` | 3 | `internal/delivery/cli/mcp.go`, `help.go`, `serve_mcp_test.go`, `internal/delivery/mcpserver/http.go` (`TrustPrivateNetwork`, `CheckListenAddress` at `:84`), `http_test.go`, `internal/app/doctor_remote.go`, docs `command-reference.md`, `remote-hub-guide.md`, `release-runbook.md`, `design/remote-hub.md` |
| `--loopback-only`, `SKILLHUB_WEB_ADDR` | 3 | `internal/delivery/cli/serve.go:44,78`, `help.go`, `serve_test.go`, `internal/delivery/web/listen.go:30`, `README.md`, docs `command-reference`, `getting-started`, `release-runbook`, `sources-and-learning`, `troubleshooting`, `web-ui`, `contracts/web-api.md`; `.github/workflows/release.yml:732,840`; `web/e2e/smoke.spec.ts` (test name only) |
| `init <path> --yes`, `init ~/...` | 2 | `internal/delivery/cli/help.go:14-16,59-66`, `workspace.go:20,63-66`, `workspace_resolve.go:15`, `internal/app/workspace.go:22-57`, `internal/app/connect.go`; CLI tests (51 call sites, 23 files); `internal/delivery/mcpserver/{memory_budget_test.go:521,remote_bench_test.go:481,skill_tools_test.go:367,subprocess_test.go:44}`; `scripts/install.sh:751,811`, `scripts/install.ps1:670,811`; `.github/workflows/release.yml:730,837`, `.github/workflows/evaluation.yml:43`; web e2e `web/e2e/support/server.ts:33`, `web/e2e/support/seed-workspace.ts:81`, `web/e2e/skill-resources.spec.ts:27`, `web/e2e/skill-sources.spec.ts:20`; `README.md`, docs `getting-started.md`, `troubleshooting.md`, `design/01-system-architecture.md` |
| `connect --workspace`, `--project` | 2 | `internal/delivery/cli/connect.go`, `help.go:67`, `workspace.go:63-66`, `internal/app/connect.go`, `internal/app/host-connections.go:97-99`; tests `connect_*_test.go`, `onboarding_test.go`, `curation_test.go:182`, `internal/delivery/mcpserver/remote_e2e_test.go`; docs `README.md`, `getting-started`, `remote-hub-guide`, `troubleshooting`, `user-guide`, `design/remote-hub.md` |
| `validate`, `rebuild`, `migrate`, `diff` (top level) | 4 | `internal/delivery/cli/root.go`, `help.go:243-271`, `catalog.go:21`, `migration.go:22`, `curation.go:113`, `workspace.go:85,365`, `validate_staged.go`, `next_commands.go:127-131`, `resolver.go:112`; `internal/app/workspace.go:128,188,190`, `migration.go:86`, `operations.go:53,69`, `metadata_lint.go:72`, `skill_resource.go:124`, `skill_lifecycle.go:348`, `skill_review.go:613`, `curation_home.go:572`; `internal/canonical/skill.go:898`; `internal/catalog/open.go:51`; web UI `web/src/screens/home/action-cta.ts:31`, `web/src/screens/skill-detail/ResourcesTab.tsx:19`, `ResourceFileView.tsx:33,35`, `web/src/components/AppShell.tsx:66`, `web/src/i18n/en.ts:89`, tests `web/src/screens/home/home.test.tsx:53`, `web/src/screens/skill-detail/resource-file-view.test.tsx:141`; web e2e `web/e2e/skill-resources.spec.ts:57`, `skill-runtime.spec.ts:35-36`, `journeys.spec.ts:48-49`, `a11y.spec.ts:42` (CI runs them through `.github/workflows/ci.yml:155`, `make web-e2e`); curator skill `SKILL.md:132,162,235-236` and copy; `.github/workflows/evaluation.yml:46`; docs `command-reference.md`, `curating-skills.md:291-296` |
| Old remote-doctor remote fixtures (`"mcp","serve"` args used as data) | none | `internal/app/doctor_remote_test.go:331` stays: `mcp serve` registrations remain valid (P1) |
| `token show` advice, fixed client token path | 3 | `internal/delivery/cli/{token,mcp,connect_remote}.go`, `internal/app/doctor_remote.go`, `internal/hubtoken/hubtoken.go`, docs `remote-hub-guide.md`, `command-reference.md` |
| `getting-started.md` links | 5 | `README.md:98,122`, `docs/README.md:7`, `docs/curating-skills.md:5,115`, `docs/installation.md:25`, `docs/troubleshooting.md:83`, `docs/user-guide.md:3,25,120` |
| `git add` staging hint | 2 | `stageWorkspaceCommand`, `internal/delivery/cli/workspace.go:71` (removed: init commits) |

Phases 1, 4 and 5 each end with a repo-wide grep gate over the same paths for the names they remove (Phase 1:
`integrate`, `skillhub web`, `skillhub check`, onboarding `--yes`; Phase 4: top-level maintenance names; Phase 5: all
of them in docs). Design records (`docs/design/*`, `docs/use-cases/*`, `docs/distillery/*`) are historical and in
Vietnamese; Phase 5 adds a one-line note at the top of `docs/design/remote-hub.md` only.

## Backwards compatibility

Not required (user decision). Consequences handled: agent registrations are unchanged (`mcp serve` keeps its name,
P1); existing projects get the new permission rules through `doctor --fix`; existing pre-commit hooks are rewritten by
`doctor --fix` (F8); `skillhub version` stays for the self-updater (P2); `status --json` and MCP `hub_status` keep their
shape; the MCP tool names and the result envelope do not change.

## Rollback

Each phase is one or more commits on a feature branch; `git revert` of a phase restores its behavior. No phase rewrites
agent registrations. Permission rules written by the new `connect` or `doctor --fix` are extra `ask` entries that an
older binary ignores. Phases 1-4 ship in one release (5.0.0), never separately, because the command syntax changes
together; Phase 5 ships in the same release.

## Resolved questions (user confirmed 2026-10-11)

1. Missing Git identity on the first commit: `init` commits as `Skill Hub <skillhub@localhost>` for that commit only and
   says so in the output.
2. `init <git-url>` inside an existing hub with no remote, remote empty: push the existing history. Never push when the
   remote already has refs.
3. **Revised 2026-10-11:** the stdio entry agents launch keeps the hidden name `skillhub mcp serve` (P1). Registrations
   are not rewritten.
4. `connect <path>` does not change the stored hub (P6).
5. `--yes` stays on curation commands and `maintain migrate` (P3).
6. A project entry wins over a user-scope entry with the same server name in Claude Code, Codex and Gemini CLI
   (see [scope precedence report](../reports/researcher-261011-mcp-config-scope-precedence.md)). Claude Code and
   Gemini CLI replace the whole entry; Codex merges the two entries key by key (from source, not documented), so
   stale keys from a user-scope entry can leak into a project entry. Consequences for this plan: the project entry
   that `connect` writes must set every field it needs, and `status` explains a folder that is both a hub and a
   client by naming the winning scope. Untested: whether Codex rejects a merged entry that mixes transports.
7. **2026-10-11:** web UI default bind stays 0.0.0.0 despite its write routes; warning line on both banners; token
   always required; no `--allow-remote-edit`.
8. **2026-10-11:** `.git/info/exclude` is dropped (P5); main's `.gitignore` model stays. `init` in a cloned hub whose
   `.gitignore` lacks required entries appends them and reports it (no commit in an existing hub; status then offers
   the one-line commit for `.gitignore`); a clone that tracks a machine file gets the `git rm --cached <path>` notice
   and `init` finishes.

## Red Team Review

Four lenses reviewed the plan on 2026-10-11: **Security Adversary (SA)**, **Failure Mode Analyst (FM)**, **Assumption
Destroyer (AD)** and **Scope & Complexity Critic (SC)**. Raw outputs were not saved; the adjudicated findings below are
the authority. All were accepted.

| ID | Finding | Severity | Lens | Disposition | Rationale and where applied |
|---|---|---|---|---|---|
| S1 | Shared check engine and client `--fix` could send the hub token to a hub named only in project files; fix text calls connect "a preview that writes nothing" | Critical | SA | Accept | `tokenMaySend` (`internal/app/doctor_remote.go:429-445`) becomes an engine invariant; client-remote `--fix` refuses to re-run `ConnectRemote` and prints the connect command; fix text corrected. Phase 1 (text), Phase 4 (engine, test) |
| S2 | Prefix `ask` rules miss permuted flags; `serve` unguarded; rules only written at connect; Codex/Gemini have no permission file | High | SA | Accept | P4 revised: infix patterns incl. `serve`, permuted-order tests, doctor stale-rule finding + fix, curator-skill text, `token rotate` terminal guard, open item on `&&` splitting. Phase 1, Phase 4 |
| S3 | `init <git-url>` argument injection, dangerous transports, userinfo leaks, prompts disabled by the offline runner, hooks/signing on first commit | High | SA | Accept | Argument classifier with table tests, leading `-` refused, `--`, `protocol.ext/fd.allow=never`, userinfo refused/redacted, separate interactive runner, hardened commit. Phase 2 |
| S4 | Cloned hub may track runtime files, tokens, binaries or agent configs that agents execute | High | SA | Accept | Trust check with `git ls-tree -r --name-only` before checkout; refuse `runtime/`, `.skillhub/transactions/`; warn by name on agent config files (exempting files byte-identical to what `init` writes); validation before declaring a hub. Phase 2 |
| S5 | One fixed client token path mixes tokens of different hubs; scp quoting differs between legacy scp and SFTP | Medium | SA | Accept | Per-hub path `~/.skillhub-token-<host>-<port>`; connect refuses a token file registered for another hub; banner tells users to `cd` and uses a quoting-free remote path. Phase 2 (helper, refusal), Phase 3 (banner) |
| S6 | Relaxing the Host policy to any IP literal would also relax the MCP Origin check, which reuses the host set | High | SA | Accept | Origin stays strict (bound addresses, request Host, localhost, `--allow-host`); test foreign-IP Origin = 403 while Host passes. Phase 3 |
| S7 | A pre-existing `.git` (archive, template) can run hooks, `core.sshCommand`, `core.fsmonitor`; refusal cleanup could delete a user's `.git` | High | SA | Accept | Hardened commit/push flags; never clone into or remove a `.git` this run did not create (marker); remote add + fetch + checkout in an existing `.git`; warn on local `core.sshCommand`. Phase 2 |
| F1 | init not idempotent after partial failure (unborn HEAD, interrupted run); commit failures unclear | Critical | FM | Accept | Resume rows for unborn-HEAD hub and init-owned-only folders; Git stderr verbatim + `skillhub init` retry; injected-failure tests (gpgsign, failing hook, interrupt, full disk). Phase 2 |
| F2 | After `remote add` + failed push, re-running `init <url>` is a no-op | High | FM | Accept | `origin` is added only after a successful push; retry sees no origin and pushes again. Phase 2 |
| F3 | Bare remote HEAD can point at a missing branch (HEAD=master, branch=main) | Medium | FM | Accept | `ls-remote --symref`; probe each branch with `ls-tree`; fixture with mismatch; tests pin `git init -b main` (`internal/source/git_revision_at_test.go:32` precedent). Phase 2 |
| F4 | `init` always stores the hub path, so tests/e2e/CI/smoke write the real user config; moved hub/project re-pointing underspecified | High | FM | Accept | `SKILLHUB_USER_CONFIG_DIR` + APPDATA/LOCALAPPDATA isolation everywhere; `previous_hubs` bounds re-pointing; stale stored hub prints `cd <new-location> && skillhub init` then `skillhub doctor --fix`. Phase 2, Phase 4 |
| F5 | "Below home" detection breaks hubs outside home and `t.TempDir()`; nested refusal could suggest the refused folder; signature drift; P8 vs decision 8 | High | AD | Accept | Walk matches `resolveWorkspace`; outside-path suggestion; HOME-is-a-repo test; one signature; effective-scope `Kind()` and mixed case; P8 wording in help. Phase 2 |
| F6 | status vs doctor locking undefined | Medium | FM | Accept | status best effort without waiting ("hub busy, retry"); doctor shared lock (`internal/app/workspace.go:99`, 15 s); web Home and MCP `hub_status` never probe remotes; test with exclusive lock held. Phase 4 |
| F7 | `serve --addr` duplicate rules break port 0; banner template `http://<addr>:7420` regresses actual-port printing | Medium | AD | Accept | Port 0 exempt; URLs from each listener's `Addr()` (`hubURLs`, `internal/delivery/cli/mcp.go:291-307`, already does); 0.0.0.0 and [::] same port overlap. Phase 3 |
| F8 | `validate --staged` pre-commit hooks break after the `maintain` move | High | AD | Accept | Hub check reads `.git/hooks/pre-commit`; `doctor --fix` rewrites; release note + troubleshooting. Phase 4, Phase 5 |
| F9 | Shared JSON shape would break `status --json` consumers (schema, curator skill, MCP `hub_status`) | High | SC | Accept | `status --json` keeps `schemas/curation-home-v1.schema.json` with optional `role` and `client_sections`; new shape for `doctor --json` only; no version bump. Phase 4 |
| F10 | `checkGuideCommand` cannot check `cd ... && skillhub ...` or `--version`; empty-hub next command had `...` placeholders | Medium | AD | Accept | Segment extractor moved into Phase 1; contract "every `skillhub` segment passes"; runnable empty-hub command. Phase 1, Phase 4 |
| F11 | Old-name inventory incomplete (web UI, e2e, Go hints, proxy, tests, scripts, fourth quoting helper, curator skill) | High | AD | Accept | Inventory regenerated above; repo-wide grep gates in Phases 1, 4 and 5. Also found: `web/e2e/support/seed-workspace.ts:81`, `resource-file-view.test.tsx:141`. |
| F12 | Phases 1-2 undersized; `init` signature change hits ~97 test call sites; rewording done three times; Phase 1 could print invalid connect commands | High | SC | Accept | Path parameter kept in `app.Init`, CLI test seam, injectable user config store; estimates 2d/4d; one rewording per surface; connect flag removal entirely in Phase 2. |
| F13 | Docs scope too large (concepts page, troubleshooting rewrite, separate banned-strings and link tests) | Medium | SC | Accept | Drop `docs/concepts.md` (token types in README `<details>` or the remote guide), replace only outdated troubleshooting entries, one generalized docs-vs-help test incl. non-command bans, no link checker; follow the archived docs style guide; keep the three anchors. Phase 5 |
| Coord | Overlap with the distill-lab packaging plan | Medium | SC | Accept | Coordination section above; this plan lands first. |

### Whole-Plan Consistency Sweep

After the edits, every file in this plan directory (plan.md and phase-01..06) was searched for: `mcp stdio`, `ExcludeLocally`, `info/exclude`,
`P5`, `concepts.md`, `link check`, `banned-strings`, `--allow-remote-edit`, `skillhub-hub-token`, `old mcp serve`,
`TrustPrivateNetwork`. Remaining hits are intentional: this section, P5 marked "Dropped", the Security trade-off
sentence that says `--allow-remote-edit` is not added, and Phase 5's note that `docs/concepts.md` is not created.
Dependencies (Phase 3 now depends on 2), success criteria and the risk table were reconciled with the phase files.

Reconciled, not open:
- F2 suggested pushing when "the local branch is not on the remote"; resolved question 2 forbids pushing when the
  remote has refs. The plan keeps the user decision: push only to a remote with no refs; a remote that has refs without
  the local branch is refused with the `git push -u origin <branch>` command for the user to run.
- S4 asks to warn on tracked `CLAUDE.md`/`AGENTS.md`, while `init` itself commits these instruction files. The warning
  skips a file whose content is byte-identical to what `init` writes for this hub, so a hub created by `init` and
  cloned elsewhere does not warn on every clone.

Unresolved (needs a decision before or during implementation):
- Whether Claude Code splits compound shell commands (`cd x && skillhub init`) before matching `Bash(...)` rules is
  unverified. If it splits, the infix rules match the `skillhub` segment. If it does not, a line that starts with `cd`
  matches no rule that starts with `skillhub`, so the prompt is missed. Phase 1 step 8 verifies this by hand; the
  fallback is an extra `ask` rule `Bash(*&& skillhub *)`, decided after verification.
- `git ls-remote` acceptance of `--` before the repository argument must be confirmed on the oldest Git in CI; if it is
  not accepted, the leading-`-` refusal alone protects `ls-remote`.
