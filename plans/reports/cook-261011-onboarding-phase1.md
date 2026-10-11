# Onboarding UX redesign, phase 1 (syntax cleanup): implementation report

Branch `worktree-agent-a5c101367df3589e1`, 2026-10-12. Plan:
[plan.md](../261011-2250-onboarding-ux-redesign/plan.md),
[phase-01](../261011-2250-onboarding-ux-redesign/phase-01-syntax-cleanup.md). Status: completed.

## What changed

Four commits on the branch (nothing pushed):

| Commit | Change |
|---|---|
| `c945e53` feat(shellquote) | New package `internal/shellquote` with `Arg` / `ArgFor`: POSIX single quotes, PowerShell single quotes on Windows (doubling `'` and the typographic single quotes), plain values unquoted. |
| `913e15c` feat(hostintegration) | Ask rules `Bash(skillhub *init*)`, `*connect*`, `*--fix*`, `*token rotate*`, `*cache clear*`, `*serve*` next to the existing three. Curator `SKILL.md` (both identical copies): `source check` instead of `check`, plus the ask-first sentence for Codex and Gemini CLI. |
| `6dd7452` refactor(cli) | One `parseSince(value, base)` with one error text, used by funnel, baseline, chains, cases and import-transcripts; help shows `--since <YYYY-MM-DD|Nd>`. One `skillSubcommands` list builds both skill hints. |
| `1e79a2d` feat(cli)! | `integrate`, `web` and top-level `check` removed; `--version` added; global help hides `resolve`, `eval`, `resolution`, `telemetry` and `mcp`, and `help --dev` lists the developer commands. `init`, `connect` (incl. `--remove`), `doctor --fix`, `cache clear` and `token rotate` act directly; `--dry-run` prints the old report under "Would change:" and writes nothing; `--yes` on them is an unknown argument. `token rotate` refuses off a terminal unless `--dry-run`. `cache clear [--hub <url>]`. All four old quoting helpers replaced by `shellquote.Arg`. The remote-doctor text says `connect --hub ... --dry-run`. The extractor `commandSegments` and the global-flag case of `checkGuideCommand` are in `remote_guide_doc_test.go`. |

App API change: `WorkspaceService.Init(path, dryRun, force...)` and `DoctorFix(path, dryRun)` replace the `yes`
parameter (call sites flipped). `ConnectRequest`, `RemoteConnectRequest` and `DisconnectRequest` have `DryRun` in place
of `Yes`. `hostintegration.NoteReRunIntegrate` became `NoteRunDoctorFix` ("run skillhub doctor --fix after a pull that
untracked those files").

## Tests

- New: `TestCommandSegments`, `TestCheckGuideCommandAcceptsGlobalFlags`, `TestHelpDevListsTheDeveloperCommands`,
  `TestVersionFlagPrintsTheVersionLine`, `TestRemovedCommandsAreNotSupported`, `TestOnboardingCommandsRejectYes`,
  `TestDryRunWritesNothing` (init, connect, connect -g, connect --remove, doctor --fix, cache clear, token rotate; it
  compares the content of workspace, project, home and skill cache before and after), `TestClaudeAskRulesMatchCommandsInAnyFlagOrder`
  (the plan's table plus `skill ... --yes` and `skill confirm`; `status`, `skill list`, `doctor` and `token show` hit only `allow`),
  `TestParseSinceAcceptsADateOrDays`, `TestSinceErrorsAreTheSameForEveryTelemetryCommand`,
  `TestSkillSubcommandHintMatchesHelp`, `TestSourceCheckIsTheOnlyCheckCommand`, the shellquote table and
  round-trip tests, and the token test now covers the terminal refusal.
- Updated: every onboarding `--yes` call in tests dropped (about 75 lines); preview calls got `--dry-run`; connect
  goldens regenerated (output and the new ask rules).
- Results (isolated HOME and XDG_*, `GIT_CONFIG_GLOBAL=/dev/null`): `go test -count=1 ./...` passes; `go vet ./...`,
  `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` clean; `gofmt -l cmd internal schemas` empty. The
  intermediate commit `6dd7452` was checked from a `git archive` copy (vet plus the affected packages).
- Grep gate over `internal`, `web/src`, `web/e2e`, `scripts`, `.github`, `system-skills`: no `skillhub integrate`,
  `skillhub web`, `skillhub check`, and no `--yes` after an onboarding command, outside the one test that asserts their
  absence.
- Hand check of an installed binary in a scratch folder: `--version`, `help --dev`, `init --dry-run`, `init`,
  `connect --dry-run`, `check` (exit 2), `connect --yes` (exit 2) and `token rotate` off a terminal (refused).

## Step 8 (compound commands)

Not run by hand in an interactive Claude Code session. The Claude Code permissions documentation
(code.claude.com/docs/en/permissions, "Compound commands") states: "Deny and ask rules apply when any subcommand
matches them ... An ask rule like `Bash(git clean *)` still prompts you for `cd /tmp && git clean -f`". So `cd <x> &&
skillhub doctor --fix` hits `Bash(skillhub *--fix*)` and the fallback rule `Bash(*&& skillhub *)` is not added. The
plan's unresolved item can be closed on this evidence; a hand check remains optional.

## Deviations and why

- Files outside the phase list, edited only to drop `--yes` or the removed aliases so nothing prints or runs an invalid
  command (the phase's grep gate covers them): `internal/delivery/cli/mcp.go` (banner connect line and two token
  fixes), `internal/delivery/cli/workspace_resolve.go`, `skill_add.go`, `internal/app/source_import.go`,
  `.github/workflows/{evaluation,release}.yml`, `scripts/install.{sh,ps1}`, and the web e2e helpers
  (`support/server.ts`, `support/seed-workspace.ts`, `skill-resources.spec.ts`, `skill-sources.spec.ts`,
  `ux/demo-hub.ts`). Paths stay as they are; Phase 2 changes the `init` syntax there.
- `docs/remote-hub-guide.md`: mechanical edits (drop `--yes`, previews become `--dry-run`, sample dry-run output,
  `cache clear [--hub <url>]`), because the guide test checks every flag against help. The wording pass stays in Phase 5.
- The `integrate` hint for missing host files now says `skillhub doctor --fix` (the alias is gone).
- `Printer.Command` routing done in the files this phase owns (`workspace.go` init summary and doctor FIX lines,
  `skill.go`). `skill_add.go`, `skill_upstream.go`, `source.go`, `source_watch.go` and `mcp.go` still print some
  `Next: skillhub ...` lines with `Printer.Line`; they are outside this phase's file list.
- `init --force` stays (Phase 2 removes it); `connect --workspace`/`--project` stay (Phase 2).
- Two wording changes were needed for correctness, not style: dry-run summaries read "Dry run: ... was not written."
  and item impacts "Would be written." / "Would be deleted." (the old text named `--yes` and a confirmation step that
  no longer exists).

## Not done

- Web typecheck and e2e were not run: the worktree has no `web/node_modules` and installing was out of scope. The e2e
  edits only remove the `'--yes'` element from four `init` calls; CI (`make web-e2e`) is the check.
- Windows and macOS were not run; CI covers the PowerShell round-trip test (skips when `powershell` is missing) and the
  quoting expectations, which now go through `shellquote.Arg` on every platform.

## Open concerns

- On Windows a plain path such as `C:\Users\you\hub` is now printed unquoted (backslash is ordinary in PowerShell);
  the old app helper wrapped it in double quotes. Tests were updated to use the helper itself.
- The Claude curation guidance line still says "Commands with --yes or confirm require approval"; connect wording is
  Phase 2's surface.
- The init dry-run still lists `.: Workspace directory will be created.` (the `.` path from decision 8); not in this
  phase's requirements.

Status: DONE
