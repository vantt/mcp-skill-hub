---
title: "Fresh-clone end-to-end test"
description: "One Go test that takes a committed sample hub to a new machine by running the commands the docs show (read from the docs themselves), then talks real MCP over stdio to the server the agent host would start, and fails on any step: skill_resolve and skill_get serve the good skills, and an unservable skill is not served."
status: pending
priority: P2
effort: 5h
branch: TBD (own worktree, see Execution)
tags: [testing, e2e, portability, mcp, docs]
blockedBy: []
blocks: []
created: 2026-10-14
---

# Fresh-clone end-to-end test

Adopts distill lesson `test-the-claim-not-the-doc` (`docs/distillery/lessons/evaluation.yaml`, score 13.5,
decision `candidate`): a documented promise is trusted only when a test runs that exact scenario end to end and the
test can fail. The promise here is the hub's goal "a fresh clone on a new machine cannot rebuild a working hub"
(`docs/distillery/distill.yaml:37`), worded for users as "Your skills live in Git, so moving is a fetch"
(`docs/user-guide.md`, "Move to another machine").

## Outcome, constraints, non-goals

- **Outcome.** `go test ./internal/delivery/mcpserver/ -run TestFreshClone` (and therefore `make test`, `make check`
  and CI) builds the real `skillhub` binary, builds and commits a sample hub on a simulated old machine, moves it to
  a simulated new machine **by running the command lines copied out of the docs**, connects a project, starts the
  MCP server exactly as the `.mcp.json` entry written by `connect` says, and over stdio asserts that
  `skill_resolve` picks the good skills, `skill_get` returns their committed content, and the deliberately
  unservable skill is neither resolved nor served. Any failed step fails the test (`t.Fatalf`/`t.Errorf`); nothing
  only logs.
- **Constraints.**
  - Never the live hub (`/home/vantt/skill-hub`), never the real HOME, never real user projects. Every directory is
    under `t.TempDir()`; each simulated machine has its own HOME, XDG_*, USERPROFILE and `SKILLHUB_USER_CONFIG_DIR`
    (the `machine` helper in `internal/delivery/mcpserver/remote_e2e_test.go:39-69`), on top of the package's
    `TestMain` isolation.
  - Runs in the default suite: target under 10 s per run on a warm Go build cache (estimate below). No build tag
    unless the measured time breaks that budget (then: phase 3, fallback).
  - No plan IDs, phase numbers, audit labels or lesson keys in code comments, test names or commit messages.
  - Follow `AGENTS.md` "Testing": use the `test-audit` skill when writing the test; one owner test per contract;
    no test-only exports or seams.
- **Non-goals.** No product features. No change to `init`, `connect`, `maintain rebuild` or `mcp serve` behaviour,
  except a fix for a defect this test exposes, and each such fix is a separate follow-up item with its own commit
  and regression evidence, never bundled into the test commit. No other distill lesson. No Windows run of the doc
  lines (see open question 2). No deletion of existing narrower clone tests.

## Verified context (2026-10-14, at `893ff54`)

### The lesson's `why` against history

The lesson says: "the clone+rebuild promise broke on 2026-10-08 and current tests cover only absent directories"
(host `internal/workspace/workspace_test.go`).

| Claim | Verdict | Evidence |
|---|---|---|
| The promise broke on 2026-10-08 | **True** | `archive/plans/261008-1433-simplify-hub-model/plan.md` "Verified starting facts (2026-10-08)": on a fresh clone `skillhub status` failed with `statat sources/intake: no such file or directory` (Git does not keep empty layout dirs). Fixed by `8225938` (2026-10-09) with `TestFreshCloneWithMissingLayoutDirectoriesGivesHealthyStatusAndValidate`. |
| It broke again after that | **True, not in the lesson** | The lead's manual fresh-clone check on 2026-10-10 (`bfef05e`; `docs/plans/handoff-2026-10-10-lead.md`; archived plan "Acceptance criteria") cloned the live hub `786c60e`, ran `validate` → `rebuild` → `status`, and the clone served `test-audit`. It found that `servableSkillWarnings` (`internal/catalog/servable.go`) took the skill directory from `.meta/skill.yaml`, so since `.meta/skill.yaml` landed (`63cc3de`, 2026-10-09) every rebuild warned "SKILL.md is missing" and `ValidateServableSkill` never ran. Skills were still served. Fixed by `f24f806` (2026-10-10, now `servable.go:115` trims `/.meta`). Its regression test, `TestBuildWarnsAboutActiveSkillsThatCannotBeServed`, checks the **warning** of an in-process catalog build only. |
| Current tests cover only absent directories | **True for the host file, false for the repo** | `internal/workspace/workspace_test.go:73` `TestInspectAcceptsCloneLikeAbsentEmptyCanonicalDirectories` and its siblings in `canonical` and `catalog` only delete empty dirs. But other tests do a real `git clone` (list below). None of them runs the built binary along the documented route and then talks MCP. |
| (implied) the manual check proves the promise | **Partly** | It ran on one real hub, on one day, with commands (`validate`, `rebuild`) that are **not** the route the docs show today; it was never repeated. |

What is proven today: an in-process clone plus `PrepareLayout` plus a catalog build keeps source revisions; an
in-process `init <bare-repo>` checks out a hub that has no skills; the unservable warning is computed from the skill
root; the resolver and `skill_get` skip unservable skills (unit and package tests on hand-made workspaces).
What is **not** proven: that the binary, following the commands the docs print today, turns a committed hub with
skills into a server that resolves and serves those skills and refuses a broken one; that the `.mcp.json` entry
`connect` writes on the new machine starts that server; and that the docs and the CLI still agree on that route.

### The documented route is `init`, not `maintain rebuild`

Since `3114b03` (2026-10-12, "init and connect act on the current folder") the docs move a hub with `init`:

| Doc, section | Command lines (verbatim) |
|---|---|
| `docs/user-guide.md`, "## Move to another machine" | `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init <git-url>`; `cd <your-project> && skillhub connect` |
| `README.md`, "Use the hub on a second machine" (details block) | same `init <git-url>` line |
| `docs/troubleshooting.md`, "### Fresh clone on a new machine" | `cd ~/skill-hub && skillhub init` (for a hub already fetched with `git clone`) |
| `README.md`, "### Check" | `cd <your-project> && skillhub status` |

`skillhub maintain rebuild` is documented only for broken derived state (`docs/troubleshooting.md` "maintain
validate names each broken file...", `docs/release-runbook.md:161,174`), and `mcp serve` is never typed by users:
`connect` writes it into `.mcp.json` as `mcp serve --profile runtime --workspace <hub>`
(`internal/hostintegration/integration.go:347`). So the test runs the four doc lines above and starts the server from
the written `.mcp.json` entry, the way `TestRemoteHubEndToEndThroughATunnel` already starts the proxy. A
`git clone` + `maintain rebuild` variant is open question 1.

### Existing tests to reuse, not duplicate

| Test / helper | What it covers | Reuse |
|---|---|---|
| `internal/delivery/mcpserver/main_test.go` `TestMain` | isolates HOME, XDG_*, APPDATA, `SKILLHUB_USER_CONFIG_DIR`; pins Go caches | the new test lives in this package and inherits it |
| `subprocess_test.go:712` `buildSkillHub` | builds `./cmd/skillhub` into a temp dir | reuse; note its fallback stub only knows `mcp serve`, so a failed real build makes `init` fail loudly (exit 2), never a false pass |
| `remote_e2e_test.go:39-69` `machine`, `newMachine`, `env()` | per-machine HOME/XDG/config dirs, Git identity, cleared `SKILLHUB_WORKSPACE` and token env | one machine for the old, one for the new computer |
| `remote_e2e_test.go:187-224` `toolCall`, `resolveFor`, `decodeSkillGet`; `types.go` `skillGetResult`, `resolveResult`, `toolOutcome` | MCP tool calls and decoding | assertions |
| `remote_e2e_test.go:257` `TestRemoteHubEndToEndThroughATunnel` | runs `connect`, reads `.mcp.json`, starts the written command | pattern for starting the server from the entry |
| `distribution_test.go:30` `newBundleHubFixture`, `writeFixtureFile` | hand-written active skills in `.meta/skill.yaml` + `SKILL.md` | fixture style for the broken skill (the fixture itself uses in-process `Init`/`EnsureCatalog`, so it is not reused as is) |
| `subprocess_test.go:32` `TestCLICreateActivateThenMCPListGetRead` | `skill create`/`activate` through the binary, then MCP | fixture commands for the good skill |
| `remote_bench_test.go:430` `cloneRemoteBenchHub` | clone + `maintain rebuild` with the binary | perf-only (`SKILLHUB_PERF=1`), external hub; not a correctness owner |
| `internal/delivery/cli/init_test.go:433` `TestInitFromARemoteHubChecksItOut` | in-process `init <bare>` of a hub with no skills | stays owner of init's Git behaviour |
| `internal/app/source_test.go:229` `TestCloneRebuildRetainsRevision...` | in-process `git clone` + `PrepareLayout` + catalog build keeps source revisions | stays owner of source state across a clone |
| `internal/app/workspace_test.go:355`, `catalog_test.go:459`, `canonical_test.go:266`, `workspace_test.go:73` | absent empty layout dirs | stay owners of that contract |
| `internal/delivery/cli/docs_commands_test.go` `TestDocsCommandsExist` | every `skillhub` command in 7 docs exists in the CLI help with its flags | already fails a doc whose command no longer exists; the new guard covers the other direction (phase 1) |
| `internal/delivery/cli/testdata/connect-workspace/*.golden` (`connect_regression_test.go`) | exact files and output of `connect` | not duplicated: the new test checks only the `.mcp.json` server entry it needs |
| `startup_bench_test.go` | `BenchmarkServerStartup`, proxy tool definitions | timing only, unrelated |
| `internal/app/resolver_continuity_test.go:98`, `distribution_integrity_test.go:116`, `skill_tools_test.go:621` | resolver skips unservable skills; `skills/list` omits them; `skill_get` reports `not_servable` | unit owners; the new test proves the same through the clone route |

The new test becomes the single owner of the end-to-end promise; it does not re-assert what the tests above own.

## Design in one page

```text
old machine (machine "old")                         new machine (machine "new")
---------------------------                         ---------------------------
skillhub init            (README step 1)            sh -c "<user-guide init line>"   <git-url> = bare repo
skillhub skill create/activate  -> good skills         -> ~/skill-hub is the new hub, stored for this machine
hand-written broken skill (name != id)              sh -c "<user-guide connect line>"  <your-project> = ~/project
git add + commit; git push -> bare repo                -> ~/project/.mcp.json  {skillhub: mcp serve --profile runtime ...}
                                                    sh -c "<README check line>"  -> exit 0, valid, index current
                                                    start .mcp.json command; MCP client over stdio:
                                                      skill_resolve(good trigger) -> primary = good
                                                      skill_get(good)  -> ready, content = committed SKILL.md
                                                      skill_resolve(broken trigger) -> primary != broken
                                                      skill_get(broken) -> unavailable / not_servable, no snapshot
                                                    old hub and bare repo unchanged (HEAD, status)
second subtest: git clone bare ~/skill-hub, then sh -c "<troubleshooting init line>", same assertions
```

- **Doc lines are data.** A small helper reads the first fenced `bash` block after a named heading (or `<summary>`)
  in a doc and returns its command lines. The test substitutes only known placeholders (`<git-url>`,
  `<your-project>`), fails on any other `<...>` left in a line, and runs each line with `sh -c` on the new machine,
  with `HOME` set to that machine's home (so `~` is the isolated home) and the built binary's directory first in
  `PATH`. Renaming a command, flag or heading in the docs either keeps the test passing because the new doc line
  still works, or fails it. Renaming a command in the CLI without the docs fails it. That is the doc-sync guard; no
  separate sync test is needed. The README copy of the `init` line must equal the user-guide line (one string
  comparison), so the two docs cannot drift apart.
- **Fixture shape.** Two good skills (one made with `skill create`/`activate` through the binary, so it carries what
  the CLI writes; one hand-written with an extra file under `references/`) and one active skill whose `SKILL.md`
  frontmatter `name` does not match its id (the case `ValidateServableSkill` rejects). All committed with plain
  `git` and pushed to a bare repo in the old machine's temp dir.

## Phases

| # | Phase | Depends on | Status |
|---|---|---|---|
| 1 | [Doc command reader](phase-01-doc-command-reader.md): read command lines from a doc section, placeholders, guard tests | none | pending |
| 2 | [Fresh-clone end-to-end test](phase-02-fresh-clone-e2e-test.md): fixture, two documented routes, MCP assertions | 1 | pending |
| 3 | [Prove it can fail, time it, record follow-ups](phase-03-prove-fail-time-followups.md) | 2 | pending |

Phases 1 and 2 touch one new test file each in the same package and can land as one commit; phase 3 changes no code
unless the measured time forces the fallback target.

## Execution

- Implementation runs in its own git worktree branched from current `main`, never in the main checkout and never
  against the live hub. The lead verifies, merges and decides on pushes (memory: lead orchestration workflow).
- Commits (conventional, no plan IDs): `test(mcpserver): run the documented move to a new machine end to end`;
  any fallback target `build: run the fresh-clone test in its own make target`; each defect fix found by the test is
  its own `fix(...)` commit on a separate follow-up item.
- Time estimate: measured on this machine on 2026-10-14, `TestCLICreateActivateThenMCPListGetRead` plus
  `TestRemoteHubEndToEndThroughATunnel` (each builds the binary, runs `init`, MCP) took 6.2 s wall for the package
  run. The new test does one build, about ten binary runs (`init` ×3, `skill create`/`activate`, `connect` ×2,
  `status` ×2, two `mcp serve` sessions) and local `git push`/`clone`: **estimate 4-8 s warm, 15-25 s on a cold CI
  cache** (the build dominates and is shared with the other tests' cache). This fits `make check`; CI also runs it
  under `-race`.

## Acceptance criteria

- [ ] `go test -count=1 -run '^TestFreshClone' ./internal/delivery/mcpserver/` passes on Linux and macOS; it skips
      on Windows with a reason that names the POSIX shell.
- [ ] The test runs the doc lines read from `docs/user-guide.md`, `docs/troubleshooting.md` and `README.md` at run
      time; no `skillhub` command of the documented route is hard-coded in the test.
- [ ] Each of these, applied one at a time in a scratch change, makes the test fail with a message naming the step
      (phase 3): rename the `init` heading or command in `user-guide.md`; reintroduce the `/.meta` bug in
      `servable.go:115`; stop the resolver excluding unservable skills; make `connect` write a different profile
      (full table in phase 3).
- [ ] No step only logs: every check is `t.Fatalf`/`t.Errorf`; process stderr is attached to failures.
- [ ] The old hub's HEAD and `git status --porcelain` and the bare repo's refs are unchanged after the run; no file
      outside the test's temp dirs is written (`TestMain` isolation plus per-machine env).
- [ ] Measured run time is recorded in the phase 3 report; under 10 s warm keeps it in the default suite.
- [ ] Any defect the test exposes is listed as a separate follow-up item with evidence; the test is not loosened
      to pass.
- [ ] `make check` passes.

## Risks

| Risk | Mitigation |
|---|---|
| Doc text is reformatted (heading, fence language) and the reader breaks | the reader fails with the doc path and heading it looked for; the fix is a one-line update in the test's section table, a deliberate act |
| `sh -c` on doc lines can do more than intended | lines come from this repository's own docs; only known placeholders are substituted, any other `<...>` fails; `HOME`, `PATH` and every XDG var point into the temp machine |
| `skillhub` on the developer's real `PATH` shadows the built binary | the built binary's dir is prepended and the test asserts the `.mcp.json` command resolves (EvalSymlinks) to the built binary |
| `buildSkillHub` falls back to the stub on a broken build | `init` then fails with "unsupported command in test stub", a loud failure; the test message points at the build |
| Content review blocks good skills on the clone (`review_required`) | expected not to, since fixture skills have no third-party source; if it happens it is a finding (follow-up), never approved inside the test |
| The test is slow on CI Windows/macOS | Windows skipped; if macOS or cold Linux exceeds the budget, phase 3 moves it to `make test-e2e` with a CI step |

## Open questions

1. Should the test also cover `git clone` + `skillhub maintain rebuild` (the route the remote bench and the
   2026-10-10 manual check used)? It is not the documented move, so it would be hard-coded; recommended no, unless
   the docs start showing it.
2. Windows: the docs show PowerShell variants (`mkdir -Force ~/skill-hub; cd ~/skill-hub; skillhub init`) in
   `<details>` blocks. Running them needs `pwsh` on the runner and a second reader path. Recommended: skip Windows
   now, revisit if a Windows-only break is reported.
3. Should the fixture include a `.meta/distill.yaml` with lessons? The archived plan's acceptance criterion says a
   hub with distilled lessons must rebuild ("unreachable evidence is a warning"). It adds one fixture file and one
   assertion (`init` succeeds and names no error for it). Recommended yes if it stays one file; the implementer
   asks before adding more.
4. After this lands, should the lesson's decision in `docs/distillery/lessons/evaluation.yaml` be recorded as
   adopted, citing the test commit? That is the user's decision through the distill workflow, not part of this
   plan's code change.
5. `skill_get` on an active but unservable skill still returns its `content` from the canonical `SKILL.md`
   (`internal/delivery/mcpserver/skill_tools.go:178`) while reporting `local.status = unavailable / not_servable`,
   and the resolver and `skills/list` skip it. Is returning the content intended ("not served" = no snapshot, not
   resolved, not listed) or a gap? The test asserts the first three and does not assert on `content` until the
   user decides; if it is a gap, the fix is a separate follow-up.
