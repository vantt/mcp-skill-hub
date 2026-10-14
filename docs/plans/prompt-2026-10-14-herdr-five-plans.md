# Prompt: điều phối 5 plan ngày 2026-10-14 bằng herdr-cook-plan

Mở một chat mới **bên trong một pane Herdr** (`HERDR_ENV=1`), tại `/home/vantt/projects/mcp-skill-hub`, rồi dán khối dưới đây.

```text
You are the coordinator for five accepted plans in the Go repo mcp-skill-hub
(/home/vantt/projects/mcp-skill-hub, branch main). Timezone Asia/Saigon. Report to the
user in Vietnamese; keep commands, paths and IDs verbatim.

Use the skill herdr-cook-plan (resolve it through Skill Hub: skill_resolve, then skill_get;
follow its local.path / local.env / preflight rules from CLAUDE.md). Follow the skill exactly:
its three gates, one plan per run, one fresh worker per phase, mailbox decisions, per-phase
verification and commit, closing the panes it created, and the run-closed check.

## Run order

herdr-cook-plan runs ONE plan per invocation and refuses to start while a run is live. Run
the plans one after another, in this order, starting the next only after the previous run
is completed and closed (run-closed check passed, implementation summary written):

1. plans/261014-1509-server-instructions-protocol/plan.md
   The Skill Hub protocol moves into MCP server `instructions`; `connect` stops writing the
   instruction-file block; `doctor --fix` removes it. Phases 1 and 2 must ship together.
2. plans/261014-1600-tools-list-budget/plan.md
   Depends on run 1: phase 2 (shorter tool descriptions) needs the server-instructions code
   on main, and phase 1's budget must count the `instructions` text added by run 1.
3. plans/261014-1530-fresh-clone-e2e-test/plan.md
   Run after 1 and 2 because those change docs and the tool list the test exercises.
   README.md and docs/user-guide.md were rewritten after this plan was written (commits
   9e7cb00, d764816): the first phase must re-read the "Move to another machine" and
   "Check" sections and adapt the doc reader to their current text before anything else.
4. plans/261014-1545-findability-lint/plan.md            (status: on-hold)
5. plans/261014-1540-session-mining-routing-proposals/plan.md (status: on-hold)
   Plans 4 and 5 are ON HOLD until the user finishes a UX review of skill curation, and
   they may change after it. Before starting plan 4, STOP and ask the user: "Đánh giá UX
   phần curation đã xong chưa, plan 4 và 5 có thay đổi không?" Start plan 4 only on an
   explicit yes, using the plan files as they are on main at that moment, and set the plan
   status from on-hold to in-progress through `ak plan`. Plan 5 runs after plan 4 because it
   reuses plan 4's shared self-resolution code.

Suggested invocation per plan (adjust runtime/model to what the user wants):
  /herdr-cook-plan <plan.md> --auto
Ask the user once, before run 1, which worker runtime and model to use (or use
--select-agent).

## Decisions already made (answer workers with these; do not re-ask the user)

server-instructions-protocol:
- No fallback for clients that ignore server instructions (user decision). If phase 3 finds
  such clients, the docs name them as "tools only, no protocol"; do not add a fallback.
- Leave the HTTP listener as is (open question 2, recommended option).
- Phase 4 removes the old block from this repo's own AGENTS.md and CLAUDE.md as repo
  content. Never edit ~/.claude/CLAUDE.md or anything outside the repository.
tools-list-budget:
- Phase 3 is approved in full (option a): advertise the compact request schema and validate
  every call against the full committed schema, with the per-bound test.
- Optional request blocks (open question 2): out of scope, record only.
- Phase 1's budget includes the server `instructions` from run 1.
- No curation-profile budget (curation work is on hold). Cursor stays unverified.
fresh-clone-e2e-test:
- Do not cover `git clone` + `maintain rebuild` (undocumented route). Skip Windows.
- A single `.meta/distill.yaml` fixture is allowed if it stays one file.

## Questions that must go to the user (never decide them under --auto)

- Whether `skill_get` should keep returning the content of an active skill that cannot be
  served (fresh-clone plan). Until answered, the test must not assert that content.
- Any change to a distill lesson's decision (ported/adapted). Report which lessons are
  ready; the user records them.
- Any defect the fresh-clone test exposes: report it as its own follow-up item; never
  loosen the test to pass and never bundle the fix silently.
- Scope expansion, publication, pushing, or anything destructive.

## Hard constraints for every worker and for you

- Never write to the live hub /home/vantt/skill-hub, never read or change the user's real
  HOME config, telemetry.db or real projects. Tests use temp dirs and isolated HOME/XDG.
- Never push, tag or release. Never delete branches or worktrees you did not create.
- Another session may be editing main at the same time. Commit only each phase's owned
  paths (never `git add -A`); leave unrelated uncommitted changes untouched; if a phase's
  owned files have foreign edits, stop that phase and ask the user.
- No plan IDs or phase numbers in code comments, test names or commit messages.
  Conventional commits ending with: Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
- Fix regressions instead of weakening tests; run `make check` (or the repo's check
  target) before accepting the last phase of each plan.

## Reporting

After each accepted phase: one short Vietnamese status line (plan, phase x/y, SHA, checks,
"Tiếp theo: ..."). After each plan: its implementation summary path and a short summary.
At the end: a table of the five plans with status and commits, every question still open,
and the reminder that the user should run `skillhub doctor --fix` on their machines after
the server-instructions change ships, and that nothing has been pushed.
```
