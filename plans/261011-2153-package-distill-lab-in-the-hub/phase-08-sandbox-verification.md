---
phase: 8
title: "Sandbox verification"
status: pending
priority: P1
effort: "5h"
wave: 5
dependencies: [7]
---

# Phase 8: Sandbox verification

## Rules

- Command forms below follow the post-5.0.0 CLI as announced by the onboarding plan (`cd <folder> && skillhub
  <cmd>`, `init` acts directly and commits, no `--workspace`, no preview-then-`--yes` on onboarding commands).
  Re-check each form against `new --help` before running and record the exact command used.

- Never touch `/home/vantt/skill-hub`, `~/.claude`, `~/.codex`, `~/.gemini`. Run the CLI only with `HOME`,
  `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME` under one `mktemp -d` inside the session
  scratchpad; remove it at the end. Unset `SKILLHUB_WORKSPACE`.
- Build two binaries into the sandbox: `old` from the commit before Phase 1 (the 5.0.0 main), `new` from the
  branch head. For the three-full-copy layout also build `legacy` from the last commit before the onboarding
  merge if 5.0.0 already changed the skill files (otherwise `old` is enough).
- No process is left running (web e2e targets manage their own server; stop anything you start).

## Checks (each one records the command and its observed output)

1. **Fresh hub.** `mkdir $S/hub && cd $S/hub && new init`. Expect `.agents/skills/{system-curator,distill-lab}/...` byte-equal to
   `internal/systemskills/...` (`cmp`, including `scripts/distill.mjs` and `scripts/vendor/js-yaml.*`), wrappers
   in `.claude/skills/*/SKILL.md`, nothing under `.gemini/`, no wrapper under `.agents`, files mode `0644`, the
   four entries in `$S/hub/.gitignore` <!-- Red team: contract change, .gitignore instead of info/exclude -->, `git
   -C $S/hub status --porcelain --untracked-files=all` lists none of these folders, and `git -C $S/hub show --stat
   HEAD` (init's commit) holds `.gitignore` but none of their files. The connection output shows an "Agent Skills
   folder (.agents/skills)" line next to the per-agent lines; `new version` prints as before.
2. **Wrappers resolve.** From each Claude wrapper folder, the path in the wrapper body (with `${CLAUDE_SKILL_DIR}`
   replaced by that folder) resolves to an existing canonical SKILL.md.
3. **Script runs in the hub.** Copy `internal/migration/testdata/test-audit-distill.yaml` to
   `$S/hub/skills/default/test-audit/.meta/distill.yaml`; `cd $S/hub && node .agents/skills/distill-lab/scripts/distill.mjs list skills/default/test-audit/.meta/distill.yaml --top 5`
   exits 0. `cd $S/hub && new status` prints `node '<abs script>' list '<abs file>'` when candidate lessons exist,
   and copying that line into a shell run from `$S` (not the hub) works. `format --no-where` on a copy changes only
   the first header line. <!-- Red team: review_lessons command --> `new status --json` shows `script`, `file` and
   `hub_relative: true`. Change one byte of `$S/hub/.agents/skills/distill-lab/scripts/distill.mjs`: status prints
   no command and names `skillhub doctor`; restore the file.
4. **Standalone skill.** Copy only `$S/hub/.agents/skills/distill-lab/` to `$S/alone/distill-lab/` (outside any
   hub, `SKILLHUB_WORKSPACE` unset) and from `$S/alone/distill-lab/`:
   `node scripts/distill.mjs --help` exits 0; `node scripts/distill.mjs list <copy of the fixture in $S/alone>`
   exits 0 with numbered lessons; `node scripts/distill.mjs check <that file>` exits 1 with the
   `ERROR: <file> is not inside a Skill Hub folder` / `WHY` / `FIX` lines and no stack trace; with `--hub $S/hub` it
   exits 1 with the "not the hub that holds" error. <!-- Red team: hub lookup --> The standalone copy run on
   `$S/hub/skills/default/test-audit/.meta/distill.yaml` from `$S/alone` runs the evidence check (hub found from
   the file), also with `SKILLHUB_WORKSPACE` pointing at another folder.
   `grep -rn "skillhub \|python" $S/alone/distill-lab --exclude-dir=vendor` finds nothing.
5. **Node floor.** Download the official Node 18.0.0 binary for this OS into the sandbox (nodejs.org dist,
   checksum from its `SHASUMS256.txt`) and repeat checks 3 and 4 with it; also run them with the machine's Node
   (v24 here). Record both versions.
6. **Doctor.** `cd $S/hub && new doctor` is healthy. With `PATH` stripped of Node (a sandbox `PATH` with
   only `git` and the binary): status unchanged, one Node warning. With a fake `node` script printing `v16.20.2`
   first on `PATH`: one warning naming the version.
7. **Project and user scope.** `cd $S/proj && new connect` and `new connect -g`: curator canonical +
   Claude wrapper present, nothing under `.gemini/`, no `distill-lab` anywhere under `$S/proj` or `$HOME`.
8. **Repo-shaped folder.** In `$S/devrepo` (a Git repo without `.skillhub/`), place a copy of this repo's
   `.agents/skills/distill-lab/` and `.claude/skills/distill-lab/SKILL.md`; `new connect`, then
   `new connect --remove`: the distill-lab files are byte-unchanged; only curator files come and go.
9. **Upgrade from the old layout.** `old init` (or `legacy init`) in `$S/hub2` (three full curator copies, committed
   by the 5.0.0 init), then `cd $S/hub2 && new status` and `new doctor` report host integration missing
   <!-- Red team: status after upgrade --> and `gitignore_entry_missing` for the four new entries, plus one
   `system_skill_tracked` warning per committed curator copy with its `git rm --cached -- <path>` command
   <!-- Red team: system_skill_tracked blocks doctor --fix -->; `new doctor --fix` (not blocked by those warnings)
   appends the entries, replaces `.claude` with the wrapper, removes `.gemini/skills/system-curator/` (and empty
   `.gemini/skills`) with the Gemini notice, installs distill-lab; a second `doctor` shows no missing item and no
   ignore finding (tracked warnings remain until the owner runs the printed commands). Repeat with an edited
   `.claude/skills/system-curator/SKILL.md`, then with an edited `.gemini/skills/system-curator/SKILL.md`, then with
   an edited canonical file: a blocked item naming the path, file unchanged, every other repair applied, and for the
   canonical case the curator wrapper removed (the decided behaviour change). <!-- Red team: conflict mid-fix -->
10. **Remote client.** In `$S/client` (not a hub), `new connect --hub http://127.0.0.1:1` with a token as 5.0.0 expects it
    on a project where `new connect` installed the curator earlier: the Claude wrapper and the canonical
    `.agents/skills/system-curator/` are removed <!-- Red team: connect --hub keeps canonical -->, no distill-lab
    written. Then a fresh locally connected project with `new connect --hub ... --host codex`: canonical and wrapper
    stay, and the output names the shared-folder limit. Use the dry-run/preview output if the unreachable hub blocks
    apply, and note which.
11. **Remove.** In `$S/hub`, `new connect --remove --host claude`: Claude wrappers gone, canonical kept;
    `new connect --remove`: canonical folders gone, edited files left.
12. **Claude Code follows the wrapper (project scope only, manual, owner or lead).** Start Claude Code in `$S/hub`
    (sandbox HOME is not possible for a logged-in session; use the owner's normal Claude Code but this sandbox hub
    path), ask "use the distill-lab skill to list the lessons of test-audit". Expect it to read
    `.agents/skills/distill-lab/SKILL.md` and run `node .../distill.mjs`. <!-- Red team: user-scope wrapper cannot be verified -->
    This is the only verification of wrapper-following. The user-scope wrapper (`~/.claude/skills/` to
    `~/.agents/skills/`) cannot be checked under these rules: record it as unverified, unless the owner explicitly
    agrees to run `new connect -g` against the real home, checks for a permission prompt, then runs
    `new connect -g --remove`; record which happened. Gemini CLI and Codex: not verified; record as such.
13. **Windows.** No Windows machine is assumed: rely on the Phase 4 path-shape tests, the CI Windows run of the
    Node script test, and `GOOS=windows go build ./...` / `GOOS=windows go vet ./internal/hostintegration/...`.
14. **Planted mirror.** <!-- Red team: planted mirror --> In `$S/hub/runtime/distill-lab/mirrors/evil/` create a
    bare-repo layout whose `config` sets `extensions.partialclone=origin`, `remote.origin.url=ssh://example.invalid/x`
    and `core.sshCommand=touch $S/PWNED`; add a lesson citing `evil@<sha>`; `node ... check` reports the ref and
    `$S/PWNED` does not exist. Commit a mirror file into the hub repo with `git add -f`: the mirror is refused.
15. **No lazy fetch.** <!-- Red team: network claim --> Make a `--bare --filter=blob:none` clone of a local repo
    served over `file://`, delete the source repo, cite one of its files: `check` reports "blob not fetched" and exits
    1 without hanging.
16. **Safe write.** <!-- Red team: atomic write --> Create `skills/default/test-audit/.meta/distill.yaml.tmp` as a
    symlink to `$S/target` and run `format`: `$S/target` is unchanged. Make `.meta` a symlink: `format` refuses.
17. **Symlinked agents folder.** <!-- Red team: symlinked ~/.agents --> In the sandbox HOME, make `~/.agents` a
    symlink to `$S/dotfiles` and run `new connect -g --host claude`: the MCP config is written, the skill files are
    reported as skipped because of the symlink, and nothing appears in `$S/dotfiles`.
18. **Deny-read rule (only if the onboarding plan shipped it).** <!-- Red team: untrusted upstream sources --> In
    the hub's Claude settings written by `init`, `runtime/config/**` and `runtime/envs/**` are denied for reads;
    otherwise record "not shipped".
19. **Suite.** `make check`, `make web-check`, `go test ./internal/hostintegration -run TestMatrixSynchronized`,
    `SKILLHUB_REQUIRE_NODE=1 go test ./internal/systemskills/...` (must not skip), and
    `SKILLHUB_REQUIRE_TAGS=1 go test ./internal/systemskills/ ./internal/hostintegration/ -run Known` on a full
    clone. <!-- Red team: known digests -->

## Output

Write the observations to `reports/verification.md` in this plan folder (commands and results only). Every failed
check goes back to the owning phase; this phase edits no product files.

## Rollback

Nothing to roll back; remove the sandbox.
