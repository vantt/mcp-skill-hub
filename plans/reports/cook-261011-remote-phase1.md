# Remote hub Phase 1: baseline, budgets and decisions

Date: 2026-10-11. Plan: `plans/261010-1652-remote-hub-mcp-over-network/`. Status: completed.

## Outcome

The stdio baseline for fetching a whole skill folder exists, is repeatable (`make remote-baseline`)
and was run twice on a clone of the live hub with matching numbers. The design note holds the
threat model, the decisions Q1 to Q5, the command syntax, the fan-out numbers and the baseline
table. The three numeric budgets in plan.md part 2 are confirmed with today's numbers next to them;
the yes/no budget is unchanged. No decision, phase or syntax line was changed.

## Files

- `internal/delivery/mcpserver/remote_bench_test.go` (new, test only): `TestRemoteFetchBench`,
  skipped unless `SKILLHUB_PERF=1`. Clones `SKILLHUB_REMOTE_HUB` into a temp dir (or builds fixture
  skills of the same shape), approves content in the clone only, then per run starts a fresh
  `skillhub mcp serve --profile runtime` and walks resolve, `skill_get`, `skills/list`, `skills/get`
  and every `resources/read`, digest-checking each file and counting response bytes through
  `mcp.LoggingTransport`.
- `Makefile`: `remote-baseline` target (`REMOTE_HUB` optional).
- `docs/design/remote-hub.md` (new).
- `plans/261010-1652-remote-hub-mcp-over-network/plan.md`: only the part 2 budget lines.
- `plans/261010-1652-remote-hub-mcp-over-network/phase-01-baseline-and-decisions.md`: ticked, status completed.

## Key numbers (clone of hub commit 9c704c7, two runs)

| Skill | Served files | Folder round trips | Folder p50 | Folder p95 | At 100 ms RTT |
|---|---|---|---|---|---|
| test-audit | 2 of 4 | 5 | 95 to 99 ms | 162 to 171 ms | about 0.6 s |
| herdr-cook-plan | 13 of 14 | 16 | 248 to 266 ms | 336 to 392 ms | about 1.85 s |

Findings that matter for Phase 3: each `resources/read` costs about 15 ms on the hub (it reopens the
catalog generation and rebuilds the manifest per file), `SKILL.md` travels twice, and a repeat fetch
in the same session still costs 61 to 78 ms and up to 68 KB.

## Verification

- `make remote-baseline REMOTE_HUB=/home/vantt/skill-hub` twice: pass, numbers within noise.
- Fixture mode (`make remote-baseline`): pass.
- `make fmt-check vet test`: pass. `make lint LINT_BASE=main`: 0 issues (local `main`, since
  `origin/main` may lag the merged commits).
- No process left behind; the live hub was only read by `git clone`.

## Unresolved questions

1. Success criterion 1 says `herdr-cook-plan` has 14 files and `test-audit` 4, and asks for the
   "whole skill folder". `skills/get` serves only files outside `.meta/` (13 and 2). Should the
   remote bundle include `.meta/` (hub metadata such as `distill.yaml`, 46 KB for test-audit), or
   is "whole folder" the served files? I recommend the served files; `.meta/` is curation data.
2. Both skills await content review in a fresh clone of the live hub's committed state (the
   `herdr-cook-plan` approval exists only as an uncommitted change to its `.meta/skill.yaml` on the
   live hub). Not a problem for this plan, since remote clients read B's working tree, but the user
   may want to commit that approval.
