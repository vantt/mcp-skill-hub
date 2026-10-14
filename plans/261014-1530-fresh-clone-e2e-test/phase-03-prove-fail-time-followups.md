# Phase 3: Prove it can fail, time it, record follow-ups

## Context

Plan: [plan.md](plan.md). Depends on phase 2. The lesson's point is that a check that cannot fail is a claim, not a
test (skillport's CI "verify" job printed failures and exited 0). This phase shows, once, that the new test fails
for each break it is meant to catch, measures its cost, and lists every defect it exposed as a separate item.

## Requirements

- Each mutation below is applied in a scratch change in the implementation worktree, the test is run, the failure
  message is copied into the report, and the change is reverted (`git checkout -- <file>`). None is committed.

  | Mutation | Expected failing step |
  |---|---|
  | `docs/user-guide.md`: rename `## Move to another machine` | doc reader: section not found |
  | `docs/user-guide.md`: `skillhub init <git-url>` → `skillhub clone <git-url>` | doc line exits non-zero (and `TestDocsCommandsExist` fails too) |
  | `internal/catalog/servable.go:115`: drop the `TrimSuffix(..., "/.meta")` (the 2026-10-10 defect) | `init` output names A and B as unservable, or A/B not `ready` |
  | `internal/app/resolver.go:242`: stop excluding `ErrSkillNotServable` | resolve or `skill_get` of the broken skill |
  | `internal/hostintegration/integration.go:347`: write `--profile curation` | `.mcp.json` args assertion |
  | test env: drop the `PATH` prefix | `.mcp.json` command is not the built binary, or `skillhub` not found |

- Time the test: `go test -count=1 -run '^TestFreshClone' ./internal/delivery/mcpserver/` three times warm, once
  after `go clean -cache` (cold). Record wall time in the report.
- If the warm time is over 10 s, or the CI job time grows by more than 60 s on any OS: guard the test with
  `SKILLHUB_E2E=1` (same pattern as `requireMemoryPerf` / `SKILLHUB_PERF`), add `make test-e2e`
  (`SKILLHUB_E2E=1 go test -count=1 -run '^TestFreshClone' ./internal/delivery/mcpserver/`), add it to `check`, and
  add one CI step on ubuntu and macOS. Otherwise change nothing: the default suite already runs it in `make check`
  and CI (`.github/workflows/ci.yml` runs `go test ./...` and `-race` on all three OSes).
- Every defect the test exposed (for example an init-written file that `git status` shows, a `status` exit code
  that is not 0 on a fresh machine, review gating of a hand-made skill) becomes one follow-up item in the report
  with: the failing assertion, the source line that causes it, a proposed fix, and the regression test that owns
  it. The test is not changed to accept the defect; if the defect blocks the merge, the lead decides whether the
  fix lands first or the test lands with `t.Skip("<follow-up item>")` on that one assertion (user decision).

## Files

- Report: `plans/reports/` in the implementation worktree, named per the repository's report convention.
- Only if the fallback is needed: `Makefile` (`test-e2e` target, `check` prerequisites),
  `.github/workflows/ci.yml` (one step), `CONTRIBUTING.md` "Build and test" (one line naming the target).
- Docs impact otherwise: none. The test adds no user-visible behaviour. `CONTRIBUTING.md` "Keep tests isolated"
  already describes the isolation it relies on.

## Steps

1. Apply each mutation, run the narrow test, copy the failure, revert. Confirm `git status` is clean after each.
2. Time it as above.
3. Apply the fallback only if the numbers require it.
4. Write the report: mutation table with actual messages, timings, follow-up items, and whether open questions
   1-5 of the plan were answered.
5. Hand the report to the lead; suggest (not record) the lesson decision for the user (plan open question 4).

## Validation (narrowest first)

```bash
go test -count=1 -run '^TestFreshClone' ./internal/delivery/mcpserver/     # after each revert: passes
git status --porcelain                                                     # empty after each revert
make check                                                                 # final
```

## Risks

- A mutation does not make the test fail: that is the most important finding of this phase. Fix the test's
  assertion until it does, and say so in the report; never weaken the mutation.
- Forgetting to revert a mutation: run `git diff --stat` before committing; the commit must contain only the test
  files (and the fallback files, if used).

## Rollback

Revert the fallback commit if one was made. The mutations are never committed.
