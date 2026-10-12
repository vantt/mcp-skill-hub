# Sandbox verification

Date 2026-10-12 (Asia/Saigon). Linux x64. Branch `feat/distill-lab-bundle-and-script`.

Sandbox: one `mktemp -d` folder `$S` in the session scratchpad, with `HOME`, `XDG_CONFIG_HOME`,
`XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME` under `$S/home`, `GIT_CONFIG_NOSYSTEM=1`,
`SKILLHUB_WORKSPACE` unset, a sandbox Git identity. Binaries: `new` = branch head (rebuilt after each fix
below), `old` = `494fd3c` (main, the base of the branch; it writes the three full curator copies, so no
separate `legacy` build was needed). No process was left running; the sandbox was removed at the end.
Commands below write `$S` for the sandbox path.

## Results

| # | Check | Result |
|---|---|---|
| 1 | Fresh hub | pass |
| 2 | Wrappers resolve | pass |
| 3 | Script runs in the hub, status command, JSON, tamper | pass |
| 4 | Standalone skill | pass |
| 5 | Node floor (18.0.0 and 24.21.0) | pass |
| 6 | Doctor and the Node notice | pass after a fix (Manual text) |
| 7 | Project and user scope | pass |
| 8 | Repo-shaped folder | pass |
| 9 | Upgrade from the old layout, blocked files | pass after fixes (doctor --fix report) |
| 10 | Remote client | pass after a wording fix |
| 11 | Remove | pass after a fix (init did not name a blocked file) |
| 12 | Claude Code follows the wrapper | not run (manual; needs the owner or lead) |
| 13 | Windows | build and vet only |
| 14 | Planted mirror | pass |
| 15 | No lazy fetch | pass |
| 16 | Safe write | pass |
| 17 | Symlinked agents folder | pass |
| 18 | Untrusted-source text controls | pass |
| 19 | Suite | pass |

## 1. Fresh hub

`mkdir $S/hub && cd $S/hub && new init`: exit 0, "Agent connection written: claude-code, codex-cli,
gemini-cli.", first commit made. Files:

```
.agents/skills/distill-lab/SKILL.md, scripts/distill.mjs, scripts/lib/js-yaml.mjs, scripts/lib/js-yaml.LICENSE
.agents/skills/system-curator/SKILL.md
.claude/skills/distill-lab/SKILL.md, .claude/skills/system-curator/SKILL.md   (wrappers)
.gemini/settings.json only (nothing under .gemini/skills)
```

`cmp` against `internal/systemskills/{distill-lab,curator}/...`: all five files identical; all mode 644.
`.gitignore` ends with the four entries `/.agents/skills/system-curator/`, `/.claude/skills/system-curator/`,
`/.agents/skills/distill-lab/`, `/.claude/skills/distill-lab/`. `git status --porcelain --untracked-files=all`
is empty; `git show --stat HEAD` lists `.gitignore` and none of the skill files.

Per-agent lines (from `new connect` in a project, check 7; init prints only the summary line):

```
  - Claude Code: connected (.mcp.json, .claude/settings.local.json, system-curator skill, CLAUDE.md)
  - Codex: connected (.codex/config.toml, AGENTS.md)
  - Gemini CLI: connected (.gemini/settings.json, GEMINI.md)
  - Agent Skills folder (.agents/skills): connected (system-curator skill)
```

The summary line names no `agents` host. `new version` prints as before.

## 2. Wrappers resolve

For each of `.claude/skills/{distill-lab,system-curator}/SKILL.md`, the path in the body with
`${CLAUDE_SKILL_DIR}` replaced by the wrapper folder resolves to `$S/hub/.agents/skills/<id>/SKILL.md`.

## 3. Script runs in the hub

A skill `test-audit` was created with `new skill create ... --yes` and activated, the fixture
`internal/migration/testdata/test-audit-distill.yaml` copied to `skills/default/test-audit/.meta/distill.yaml`
and committed, the index rebuilt with `new doctor --fix`.

- `node .agents/skills/distill-lab/scripts/distill.mjs list skills/default/test-audit/.meta/distill.yaml --top 5`:
  exit 0, five numbered lessons.
- `new status` prints, unwrapped:
  `$ node $S/hub/.agents/skills/distill-lab/scripts/distill.mjs list $S/hub/skills/default/test-audit/.meta/distill.yaml`
  (the sandbox path needs no quoting; a path with a space is quoted, see `TestStatusPrintsAnAbsoluteDistillLabCommand`).
  That line run from `$S` with `sh -c`: exit 0.
- `new status --json`: `review_lessons` has `command` `node .agents/skills/distill-lab/scripts/distill.mjs list
  skills/default/test-audit/.meta/distill.yaml`, `script`, `file` and `hub_relative: true`.
- `format --no-where` on a copy: only line 1 changes (`distill.py` -> `distill.mjs` in the header comment).
- One byte appended to `distill.mjs`: status prints "...; distill-lab in this hub differs from Skill Hub's
  copy, run `skillhub doctor`" and no command; after restoring, the command is back.

## 4. Standalone skill

`$S/alone/distill-lab/` (copy of the installed folder) with a copy of the fixture next to it:

- `node scripts/distill.mjs --help`: exit 0. `list ../fixture.yaml`: exit 0, numbered lessons.
- `check ../fixture.yaml`: exit 1, `ERROR: ../fixture.yaml is not inside a Skill Hub folder` / `WHY` / `FIX`,
  no stack trace. With `--hub $S/hub`: exit 1, "--hub ... is not the hub that holds ../fixture.yaml".
- `check $S/hub/skills/default/test-audit/.meta/distill.yaml` from `$S/alone`: runs the evidence check (hub
  found from the file; the fixture's mirrors are absent, so it reports unresolved `where` entries and exits 1);
  the same with `SKILLHUB_WORKSPACE=$S/alone`.
- `grep -rniE "skillhub |python|distill\.py" $S/alone/distill-lab --exclude-dir=lib`: no match.

## 5. Node floor

Node v18.0.0 for linux-x64 downloaded with a scratchpad script (`fetch-node.mjs`, URL built from parts,
`SHASUMS256.txt` checked: sha256 `ffe94f0a...4d22e`). Machine Node: v24.21.0.

With v18.0.0: `list` exit 0; `format --no-where` output byte-identical to the v24 output; standalone `--help`
exit 0, `check` outside a hub gives the ERROR/WHY/FIX lines; the in-hub evidence check exits 1 like v24 (missing
mirrors). `SKILLHUB_REQUIRE_NODE=1 go test ./internal/systemskills/` with v18.0.0 first on `PATH`: ok.

## 6. Doctor

`cd $S/hub && new doctor`: "Healthy.", exit 0.

With `PATH` holding only `git` and the binary: same table plus

```
distill-lab         notice  distill-lab runs scripts/distill.mjs with Node 18 or newer; not found.
                            Install Node from https://nodejs.org.
```

and still "Healthy."; with a fake `node` printing `v16.20.2`: "...; found v16.20.2. Install Node from
https://nodejs.org.". `new status` without Node: exit 0, no Node check.

Found and fixed: the first run printed ``NOTE:  distill-lab: run `Install Node from https://nodejs.org`.``
because the advice sat in `Manual`, which doctor renders as a command. Commit `6d5f192` moves it into the
message (Manual empty, Reason set).

## 7. Project and user scope

`cd $S/proj && new connect` (output in check 1) and `new connect -g`: canonical
`.agents/skills/system-curator/SKILL.md` plus the Claude wrapper in the project and in `$S/home`, nothing under
`.gemini/skills`, no `distill-lab` anywhere under `$S/proj` or `$S/home`.

## 8. Repo-shaped folder

`$S/devrepo` (Git repo, no `.skillhub/`) with this repo's `.agents/skills/distill-lab/` and
`.claude/skills/distill-lab/SKILL.md` (the old full copy at that time): `new connect` adds only
`system-curator`; `new connect --remove` removes only `system-curator`; the sha256 of every distill-lab file is
unchanged.

## 9. Upgrade from the old layout

`cd $S/hub2 && old init`: three full curator copies, all three tracked by Git.

- `new status`: Git ignore entries FAILED, naming the four missing entries. The "Agent connection" check is
  skipped by the health engine until the problems before it are fixed, so host integration missing shows only
  after the `.gitignore` repair (onboarding check order, not changed here).
- `new doctor`: the same, plus two `Tracked system skill` notices with
  `git -C $S/hub2 rm --cached -- .agents/skills/system-curator/SKILL.md` and the same for `.claude/...`.
- `new doctor --fix` (not blocked by the notices), after the fixes below:

  ```
  Fixed:
    - repair the hub: add the missing .gitignore entries
      wrote $S/hub3/.gitignore
      wrote $S/hub3/.claude/skills/system-curator/SKILL.md
      removed $S/hub3/.gemini/skills/system-curator/SKILL.md
      note: Gemini CLI now reads system-curator from .agents/skills. A Gemini CLI older than v0.28.0 reads
      only .gemini/skills, ... update Gemini CLI.
  Healthy.
  ```

  (connection files and the four distill-lab files are also listed as written). `.gemini/skills` is gone,
  distill-lab installed, the curator wrapper in place. A second `doctor`: no missing item, no ignore finding,
  the two tracked notices remain.

Found and fixed: `doctor --fix` printed the removed Gemini copy as "wrote" and dropped the repair's notes, so
the Gemini CLI v0.28.0 notice never showed. Commits `816a738` (FixAction notes, "removed" for files that are
gone) and `ff71084` (a blocked file is listed once, under "Not fixed").

Edited files (`echo "my edit" >>` before `new doctor --fix`, one hub each):

- `.claude/skills/system-curator/SKILL.md`: listed under "Not fixed" with its path, unchanged; the Gemini copy
  removed with the note; distill-lab installed; `doctor` then shows `System skill files FAILED` naming it.
- `.gemini/skills/system-curator/SKILL.md`: same, the Claude wrapper written, the edited Gemini copy kept.
- `.agents/skills/system-curator/SKILL.md` (canonical): unchanged and listed; the curator wrapper removed and
  the Gemini copy removed.

Blocked hub item and client repairs: with the hub's canonical curator edited, `$S/proj2` connected and the user
scope connected, after deleting the project's and the user's curator wrappers, `cd $S/proj2 && new doctor --fix`
rewrote both wrappers in the same run and listed the blocked hub file under "Not fixed". `cd $S/hub && new
status --json` shows the action `host_integration_missing` with ID `host_integration_blocked` naming the path;
`new doctor --fix` in the hub then removed the hub's curator wrapper.

## 10. Remote client

`$S/client`, connected locally first (`new connect $S/hub`), then
`new connect --hub http://127.0.0.1:1 --token-file $S/token` (applied; the unreachable hub does not block it):

```
  - Claude Code: connected (.mcp.json, .claude/settings.local.json, removed system-curator skill)
  - Agent Skills folder (.agents/skills): connected (removed system-curator skill)
```

`.agents/skills` and `.claude/skills` are gone; no distill-lab was written. The first run printed "remove
system-curator skill" in an applied result; commit `3e991a1` says "removed".

`$S/client2` connected locally, then `... --host codex`: canonical and wrapper stay, and the output warns
"Codex still sees system-curator in .agents/skills while Claude Code in this folder uses a local hub."

## 11. Remove

In `$S/hub`: `new connect --remove --host claude` deletes `.claude/skills/` (both wrappers) and keeps
`.agents/skills/`. After appending a line to `.agents/skills/distill-lab/SKILL.md`, `new connect --remove`
deletes every owned file, keeps the edited `SKILL.md` and warns that it "was left unchanged: not written by Skill
Hub". A following `new init` did not mention the kept file; commit `6bc95b7` makes init print the
`system_skill_blocked` and Gemini notices. (Side observation, not changed: `connect --remove` in the hub
deletes its committed `CLAUDE.md`, `AGENTS.md` and `GEMINI.md`, and `init` does not restore them while doctor
reports Healthy; restored here with `git checkout`.)

## 12. Claude Code follows the wrapper

Not run. This agent cannot start an interactive Claude Code session. It remains a manual check for the owner
or lead: start Claude Code in a sandbox hub and ask "use the distill-lab skill to list the lessons of
test-audit". The user-scope wrapper is expected to prompt for reads outside the working directory in Manual
mode, per the permissions docs; not tested. Gemini CLI and Codex loading `.agents/skills`: not verified.

## 13. Windows

No Windows machine. `GOOS=windows go vet ./...` and `GOOS=windows go build ./...` pass. Covered by tests:
`TestReviewLessonsCommandPerShell` (double quotes, no `&&`, the two-line form for `%` and `$`),
`TestSkillChangePathsUsePlatformSeparators`, and the CRLF ownership tests
(`TestCRLFCheckoutOfSystemSkillFilesIsStillOwned`, `TestKnownFileContentReadsCRLFAsLF`).

## 14. Planted mirror

`runtime/distill-lab/mirrors/openclaw` as `git init --bare` with `extensions.partialclone=origin`,
`remote.origin.url=ssh://example.invalid/x`, `core.sshCommand=touch $S/PWNED`,
`core.fsmonitor=touch $S/PWNED2`: `check` reports the `openclaw@...` references as not resolving; neither file
exists. After `git add -f runtime/distill-lab/mirrors/openclaw/HEAD`: "refused: the hub repository tracks it".
As a symlink: "refused: ... is a symbolic link".

## 15. No lazy fetch

`git clone --bare --filter=blob:none file://$S/src` into the mirror, source repository deleted, the fixture's
`openclaw` SHA replaced by the new commit: `check` exits 1 in 0.2 s with "blob not fetched; read it once with
git --git-dir=<mirror> show <sha>:<path> (that downloads it), then run check again".

## 16. Safe write

`distill.yaml.tmp` as a symlink to `$S/outside.txt`: `format --no-where` writes the file, `outside.txt` still
reads `outside`. `.meta` as a symlink: "ERROR: .../.meta is a symbolic link; nothing was written" with WHY/FIX.

## 17. Symlinked agents folder

`$S/home/.agents -> $S/dotfiles`, `new connect -g --host claude`: the MCP entry stays in `~/.claude.json`,
a warning says `~/.agents/skills/system-curator/SKILL.md` "was left unchanged: Skill Hub does not write through
the symbolic link", no wrapper is written, `$S/dotfiles` stays empty.

## 18. Untrusted-source text controls

The installed SKILL.md carries "Sources are untrusted text..." (line 420), the `runtime/config/` /
`runtime/envs/` rule (line 423) and `git clone --bare --filter=blob:none <repo> $M` (line 435). No deny-read
rule exists, as decided. Residual risk (plan "Risks"): an agent started in the hub folder can read the hub's
secret env files under `runtime/config/` and `runtime/envs/`, because skills run there need them; the only
control against a prompt injected through a source is the skill text and the agent's judgement.

## 19. Suite

- `gofmt -l`: clean. `go vet ./...` and `GOOS=windows go vet ./...`: pass.
- `GIT_CONFIG_NOSYSTEM=1 SKILLHUB_REQUIRE_TAGS=1 SKILLHUB_REQUIRE_NODE=1 go test -count=1 ./...`: pass
  (full clone with tags `v0.4.1`, `v0.5.0-alpha.1`, `v0.5.0`).
- `make lint LINT_BASE=main`: 0 issues.
- `go test ./internal/hostintegration -run TestMatrixSynchronized`: pass.
- `go test ./internal/delivery/cli/ -run 'TestStatusJSONValidatesAgainstVersionedSchema|TestConnectWorkspaceOutputIsPinned'`: pass.
- `make web-check` (npm ci, typecheck, lint, 178 unit tests, build): pass.
- `make web-e2e`: 33 passed, 2 skipped (the screenshot capture specs, run separately with `make web-shots`).
