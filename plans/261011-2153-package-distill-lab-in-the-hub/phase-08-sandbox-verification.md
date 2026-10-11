---
phase: 8
title: "Sandbox verification"
status: pending
priority: P1
effort: "3.5h"
wave: 4
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
   four patterns in `$S/hub/.git/info/exclude` and none in `.gitignore`, `git -C $S/hub status --porcelain
   --untracked-files=all` lists none of these folders, and `git -C $S/hub show --stat HEAD` (init's commit) holds
   none of their files. The connection summary shows the new wording; `new version` prints as before.
2. **Wrappers resolve.** From each Claude wrapper folder, the path in the wrapper body (with `${CLAUDE_SKILL_DIR}`
   replaced by that folder) resolves to an existing canonical SKILL.md.
3. **Script runs in the hub.** Copy `internal/migration/testdata/test-audit-distill.yaml` to
   `$S/hub/skills/default/test-audit/.meta/distill.yaml`; `cd $S/hub && node .agents/skills/distill-lab/scripts/distill.mjs list skills/default/test-audit/.meta/distill.yaml --top 5`
   exits 0. `cd $S/hub && new status` prints the `cd ... && node ...` line when candidate lessons exist,
   and copying that line into a shell works. `format --no-where` on a copy changes only the first header line.
4. **Standalone skill.** Copy only `$S/hub/.agents/skills/distill-lab/` to `$S/alone/distill-lab/` (outside any
   hub, `SKILLHUB_WORKSPACE` unset) and from `$S/alone/distill-lab/`:
   `node scripts/distill.mjs --help` exits 0; `node scripts/distill.mjs list <copy of the fixture in $S/alone>`
   exits 0 with numbered lessons; `node scripts/distill.mjs check <that file>` exits 1 with the
   `ERROR: no Skill Hub folder found` / `WHY` / `FIX` lines and no stack trace; with `--hub $S/hub` it runs the
   evidence check. `grep -rn "skillhub \|python" $S/alone/distill-lab --exclude-dir=vendor` finds nothing.
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
9. **Upgrade from the old layout.** `old init` (or `legacy init`) in `$S/hub2` (three full curator copies), then
   `cd $S/hub2 && new doctor` reports the native-skill and missing-exclude items; `new doctor --fix` replaces `.claude` with the
   wrapper, removes `.gemini/skills/system-curator/` (and empty `.gemini/skills`), installs distill-lab; a second
   `doctor` is healthy. Repeat with an edited `.claude/skills/system-curator/SKILL.md`, then with an edited
   `.gemini/skills/system-curator/SKILL.md`: conflict naming the path, file unchanged. Repeat with an edited
   canonical file: conflict, not overwritten (the decided behaviour change).
10. **Remote client.** In `$S/client` (not a hub), `new connect --hub http://127.0.0.1:1` with a token as 5.0.0 expects it
    on a project where `new connect` installed the curator earlier: owned curator files removed, no distill-lab
    written. Use the dry-run/preview output if the unreachable hub blocks apply, and note which.
11. **Remove.** In `$S/hub`, `new connect --remove --host claude`: Claude wrappers gone, canonical kept;
    `new connect --remove`: canonical folders gone, edited files left.
12. **Claude Code follows the wrapper (verified host, manual, owner or lead).** Start Claude Code in `$S/hub`
    (sandbox HOME is not possible for a logged-in session; use the owner's normal Claude Code but this sandbox hub
    path), ask "use the distill-lab skill to list the lessons of test-audit". Expect it to read
    `.agents/skills/distill-lab/SKILL.md` and run `node .../distill.mjs`. Record whether a permission prompt
    appeared for the user-scope curator wrapper (`~/.agents`). Gemini CLI and Codex: not verified; record as such.
13. **Windows.** No Windows machine is assumed: rely on the Phase 4 path-shape tests, the CI Windows run of the
    Node script test, and `GOOS=windows go build ./...` / `GOOS=windows go vet ./internal/hostintegration/...`.
14. **Suite.** `make check`, `make web-check`, `go test ./internal/hostintegration -run TestMatrixSynchronized`,
    and `SKILLHUB_REQUIRE_NODE=1 go test ./internal/systemskills/...` (must not skip).

## Output

Write the observations to `reports/verification.md` in this plan folder (commands and results only). Every failed
check goes back to the owning phase; this phase edits no product files.

## Rollback

Nothing to roll back; remove the sandbox.
