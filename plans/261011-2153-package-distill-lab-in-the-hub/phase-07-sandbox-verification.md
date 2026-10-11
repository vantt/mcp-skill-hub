---
phase: 7
title: "Sandbox verification"
status: pending
priority: P1
effort: "3h"
wave: 4
dependencies: [6]
---

# Phase 7: Sandbox verification

## Rules

- Never touch `/home/vantt/skill-hub`, `~/.claude`, `~/.codex`, `~/.gemini`. Run the CLI only with `HOME`,
  `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME` under one `mktemp -d` inside the session
  scratchpad; remove it at the end. Unset `SKILLHUB_WORKSPACE`.
- Build two binaries into the sandbox: `old` from the commit before Phase 1, `new` from the branch head.
- No process is left running (web e2e targets manage their own server; stop anything you start).

## Checks (each one records the command and its observed output)

1. **Fresh hub.** `new init $S/hub --yes`. Expect `.agents/skills/{system-curator,distill-lab}/...` byte-equal to
   `internal/systemskills/...` (`cmp`), wrappers in `.claude/skills/*/SKILL.md` and `.gemini/skills/*/SKILL.md`,
   no `.agents` wrapper, files mode `0644`, six new `.gitignore` lines, `git -C $S/hub status --porcelain` lists
   none of these folders.
2. **Wrappers resolve.** From each wrapper folder, the path in the wrapper body (with `${CLAUDE_SKILL_DIR}`
   replaced by that folder) resolves to an existing canonical SKILL.md.
3. **Script runs.** Copy `internal/migration/testdata/test-audit-distill.yaml` to
   `$S/hub/skills/default/test-audit/.meta/distill.yaml`; `cd $S/hub && python3 .agents/skills/distill-lab/scripts/distill.py list skills/default/test-audit/.meta/distill.yaml --top 5`
   exits 0. `new status --workspace $S/hub` prints the `cd ... && python3 ...` line when candidate lessons exist,
   and copying that line into a shell works.
4. **Doctor.** `new doctor --workspace $S/hub` is healthy. With `PATH` stripped of Python (a sandbox `PATH` with
   only `git` and the binary): status unchanged, one Python warning.
5. **Project and user scope.** `cd $S/proj && new connect --yes` and `new connect -g --yes`: curator canonical +
   wrappers present, no `distill-lab` anywhere under `$S/proj` or `$HOME`.
6. **Upgrade from the old layout.** `old init $S/hub2 --yes` (three full curator copies), then `new doctor` reports
   the native-skill and missing-ignore items; `new doctor --fix --yes` migrates to wrappers and installs
   distill-lab; a second `doctor` is healthy. Repeat with an edited `.claude/skills/system-curator/SKILL.md`:
   conflict naming the path, file unchanged.
7. **Remote client.** In `$S/client` (not a hub), `new connect --hub http://127.0.0.1:1 --token-file <file> --yes`
   on a project where `new connect` installed the curator earlier: owned curator files removed, no distill-lab
   written. Use the dry-run/preview output if the unreachable hub blocks apply, and note which.
8. **Remove.** In `$S/hub`, `new connect --remove --host claude --yes`: Claude wrappers gone, canonical kept;
   `new connect --remove --yes`: canonical folders gone, edited files left.
9. **Claude Code follows the wrapper (verified host, manual, owner or lead).** Start Claude Code in `$S/hub`
   (sandbox HOME is not possible for a logged-in session; use the owner's normal Claude Code but this sandbox hub
   path), ask "use the distill-lab skill to list the lessons of test-audit". Expect it to read
   `.agents/skills/distill-lab/SKILL.md` and run the script. Record whether a permission prompt appeared for the
   user-scope curator wrapper (`~/.agents`). Gemini CLI and Codex: not verified; record as such.
10. **Windows.** No Windows machine is assumed: rely on the Phase 3 path-shape tests and
    `GOOS=windows go build ./...` / `GOOS=windows go vet ./internal/hostintegration/...`.
11. **Suite.** `make check`, `make web-check`, `go test ./internal/hostintegration -run TestMatrixSynchronized`.

## Output

Write the observations to `reports/verification.md` in this plan folder (commands and results only). Every failed
check goes back to the owning phase; this phase edits no product files.

## Rollback

Nothing to roll back; remove the sandbox.
