# Fresh-clone end-to-end test: mutation proof, timing and follow-ups

The test fails for five of the six breaks it was meant to catch, runs in about 1.4 s warm, and needs no opt-in guard. One break, the 2026-10-10 `servable.go` defect, is **not** caught, because no user-visible output carries the signal it changes (follow-up 1 below). I replaced that row with the nearest equivalent, which the test does catch.

Test under mutation: `TestFreshCloneOnANewMachineServesTheHubsSkills` (`internal/delivery/mcpserver/fresh_clone_e2e_test.go`, commit 75b2928). Each mutation was applied, `go test -count=1 -run '^TestFreshClone' ./internal/delivery/mcpserver/` was run, and the file was restored with `git checkout --`. `git status --porcelain` was empty after every revert.

## Mutations

| # | Mutation | Result | Failure message (trimmed) |
|---|---|---|---|
| 1 | `docs/user-guide.md`: `## Move to another machine` renamed | caught; both subtests fail | `docs/user-guide.md "## Move to another machine": section not found` |
| 2 | `docs/user-guide.md`: `skillhub init <git-url>` changed to `skillhub clone <git-url>` (lines 242 and 265) | caught; `TestDocsCommandsExist` fails as well | `docs/user-guide.md "## Move to another machine" line "mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub clone '<bare>'": exit status 2 / ERROR: The request cannot be accepted. / WHY: The command "clone" is not supported.` and `docs_commands_test.go:121: line 265: skillhub help clone fails` |
| 3 | `internal/catalog/servable.go:115`: `TrimSuffix(..., "/.meta")` dropped (the 2026-10-10 defect) | **not caught**: test passes | none; see follow-up 1 |
| 3b | nearest equivalent: `servable.go` `ParseServableSkill` stops requiring `name == id` | caught | `skill_resolve recommends the skill that cannot be served: {"result":{...,"primary":{"id":"fresh-clone-broken",...}}}` |
| 4 | `internal/app/resolver.go:242`: stop excluding `ErrSkillNotServable` | caught | `skill_resolve "repair the fresh clone broken skill" = {"error":{"code":"invalid_request","message":"The request conflicts with validation rules or the current object state.",...}}` |
| 5 | `internal/hostintegration/integration.go:325` (the runtime registration that connect writes; line 347 in the plan no longer holds the profile): `"runtime"` changed to `"curation"` | caught | `connection file entry = {Command:<binary> Args:[mcp serve --profile curation --workspace <hub>]}, want <binary> mcp serve --profile runtime --workspace <hub>` |
| 6 | test env: `PATH` prefix dropped in `freshCloneEnv` | caught, with a caveat | `connection file entry = {Command:/home/vantt/.local/bin/skillhub ...}, want <test binary> mcp serve --profile runtime ...` |

Notes on the rows:

- Row 1: both subtests fail, at `fresh_clone_e2e_test.go:154` and `:169`, with the same "section not found" message.
- Row 3: the defect only changed `servableSkillWarnings`, which feeds the `runtime_maintenance_warning` list (`internal/app/catalog.go:59`). The skills were still served. The test observes served behaviour and command output, and no command prints that warning (follow-up 1), so it cannot see it. I did not loosen or invent an assertion to force a failure.
- Row 6: the run failed only because this machine has a real `skillhub` on `PATH` (`~/.local/bin/skillhub`), so the documented line ran the wrong binary and the command assertion caught it. On a machine with no installed `skillhub` the failure would be `skillhub: not found` at the first documented line. Either way the test is red without the `PATH` prefix.

## Timing

| Run | Wall time |
|---|---|
| warm 1 | 1.41 s |
| warm 2 | 1.39 s |
| warm 3 | 1.41 s |
| cold (`GOCACHE` pointed at an empty scratch directory) | 12.62 s |

The test itself takes 1.0 to 2.1 s. The cold run is dominated by compiling the module, which the whole suite pays once, not this test. I used a scratch `GOCACHE` instead of `go clean -cache` so the shared build cache of the user's machine was not wiped.

The warm time is far below the 10 s threshold, so **the fallback was not applied**: no `SKILLHUB_E2E` guard, no `make test-e2e`, no CI step. The default suite and `make check` already run the test. CI timing on macOS and Windows was not measured here; the test skips on Windows.

## Final check

`make check`: pass. `git status --porcelain`: empty (nothing staged or committed by this phase except this report file).

## Follow-up items (not fixed here, not bundled)

Each one is a defect or gap the test or its mutations exposed. The test is not changed to accept any of them.

1. **No command surfaces servability warnings, so the 2026-10-10 defect is invisible to any end-to-end check.**
   - Failing assertion: none can be written; mutation 3 passes.
   - Source: `internal/catalog/servable.go:105` (`servableSkillWarnings`) feeds `input.Warnings` (`internal/catalog/input.go:156`), which reaches only `runtime_maintenance_warning` (`internal/app/catalog.go:59`). `init`, `maintain rebuild`, `maintain validate` and `doctor` print nothing for an active skill with no `name`.
   - Proposed fix: print the maintenance warnings in `maintain rebuild` and `init`, and report them in `doctor` under "Content".
   - Owning regression test: extend `TestFreshCloneOnANewMachineServesTheHubsSkills` to assert the broken fixture skill is named in the `init` output; mutation 3 then fails it. Today's in-process `TestBuildWarnsAboutActiveSkillsThatCannotBeServed` remains the unit owner.
2. **An active skill whose frontmatter `name` differs from its id makes `init <url>` refuse the whole hub.**
   - Evidence: exit 2, `The hub from <bare> was checked out at <home>/skill-hub, but it has problems; it was not stored as this machine's hub.` followed by `skills/default/fresh-clone-broken/SKILL.md: SKILL.md frontmatter name "some-other-name" does not match skill ID "fresh-clone-broken"` (found in the previous phase).
   - Source: `internal/app/workspace_remote.go:326`; the `servable.go` comment at line 106 says such skills "stay published", and the plan said the same.
   - Proposed fix: decide the intent. Either refuse (then update the `servable.go` comment and docs) or warn and store the hub (then change the init check to treat a servability error as a warning).
   - Owning regression test: a second fixture skill with a wrong name in the fresh-clone test, asserting whichever behaviour is chosen. This needs the user's decision first.
3. **README "### 3. Check" has no sample output.**
   - Evidence: `README.md:84` ends the block with prose only. The only documented all-clear (`Healthy.`) is `docs/remote-hub-guide.md:412`; `docs/troubleshooting.md` does not show it.
   - Proposed fix: add the `doctor` sample to the README check section or to the troubleshooting guide.
   - Owning regression test: the test's `Healthy.` assertion names its source doc in the failure message; move the reference when the sample moves.
4. **`init <url>` on an empty folder prints "Hub already set up at <folder>." before "Fetched the hub...".**
   - Source: `internal/app/workspace_init.go:218` (the summary is chosen from `report.Created`, which is false on the fetch path).
   - Proposed fix: set the created flag, or the summary, for the fetch path.
   - Owning regression test: add an output assertion to the same fresh-clone test for the first line of the `init` output.
5. **With a real network server on port 7420, `doctor` adds a "Network server notice".**
   - Evidence: exit stays 0, so the test is unaffected; it would only matter if the notice is ever promoted to a warning.
   - Proposed fix: none required; keep as an observation.
   - Owning regression test: none.

Resolved from the previous phase: its finding that `skill create` plus `skill activate` leave an untracked `history/` directory is not a defect. `history/operations` is canonical hub content (`internal/workspace/workspace.go:30`) and `docs/curating-skills.md:365` already says `git add -- history skills`. The fixture's `git add -A` is consistent with that.

## Plan open questions

Questions 1, 2, 3 and 5 were answered by the user and implemented (no clone plus rebuild test, no Windows, one `distill.yaml` fixture, `not_servable` asserted). Question 4, the lesson decision, is the user's.

## Lesson decision (suggested, not recorded)

The test now demonstrably fails for a wrong documented command, a wrong section name, a wrong connect profile, a broken `PATH` and a servable-skill regression, so it satisfies "a check must be able to fail". I suggest recording the lesson as **adopted**, with the caveat that follow-up 1 should land before the claim "catches the 2026-10-10 defect" is made.

## Unresolved questions

- Follow-up 2: should a skill with a wrong `name` refuse the hub at `init` or stay published with a warning?
- Follow-up 1: is surfacing servability warnings in `init` and `rebuild` wanted, given that no doc promises it?
