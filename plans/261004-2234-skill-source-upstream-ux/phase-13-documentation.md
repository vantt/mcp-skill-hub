---
title: "Phase 13: Documentation and plan close"
status: in-progress
---

# Phase 13: Documentation and plan close

<!-- Updated: Validation Session 1 - document agent-free updates, no daemon, behind semantics, git merge engine -->
<!-- Updated: 2026-10-05 - renumbered from phase 10; absorbs the documentation task of the closed WebUI plan (plans/261003-1645-webui-v1-implementation phase 6, task 6.5) and closes this plan -->

## Context

- Plan: [plan.md](./plan.md). Depends on phases 1–12 (document shipped behavior only; read the code, help text, and routes, not this plan, as the source of truth).
- Read first: `.claude/rules/documentation-management.md`, `README.md`, `docs/user-guide.md`, `docs/curating-skills.md`, `docs/design/06-source-learning-and-distillation.md`, `docs/design/07-storage-and-mutation-model.md` (runtime tree at line ~109 and operational state at lines ~514 and ~721), `docs/use-cases/03-cli-and-curator-mcp-mapping.md`, `docs/use-cases/04-webui-user-flows-and-screen-specs.md` (§2.5 Skill Detail, §2.6 Sources, §2.7 Watch), `docs/use-cases/05-webui-design-brief.md` (§4.6–4.7), `internal/delivery/cli/help.go`, `docs/design/01-system-architecture.md`, `docs/design/05-curation-lifecycle.md` (line 5), `docs/contracts/error-codes.md`, `docs/PRD.md`, `docs/release-runbook.md`, `internal/delivery/web/*.go` (every `mux.HandleFunc` pattern), `internal/delivery/web/errors.go`.
- Facts verified on 2026-10-05: `01-system-architecture.md` still says the Web UI is not in V1 (line 125 sentence `Web UI không thuộc V1.`, Defaults item `- Không Web UI trong V1.` near line 486, out-of-scope item `- Web UI trong V1.` near line 561); `error-codes.md` says `adds no web UI`; `README.md` and `docs/user-guide.md` never mention `skillhub serve web`; `docs/contracts/web-api.md` does not exist. Match these by text, not by line number.
- Language: keep each document's current language (`docs/design/*` and `docs/use-cases/*` are Vietnamese; `README.md`, `docs/user-guide.md`, `docs/curating-skills.md` are English).

## Overview

Update the smallest owning surface of each document so it matches the shipped commands, tools, screens, storage, and safety rules, including the WebUI as a whole (it has never been documented as shipped), then close this plan (the absorbed WebUI plan was already closed on 2026-10-05).

## Requirements

1. `docs/user-guide.md`: "Concepts in plain words" explains a source as an attribute of a skill with two roles (upstream, learning reference) in 3–5 sentences; the command cheat sheet adds `skill outdated`, `skill upstream`, `skill update`, `source list`, `source attach|detach|unwatch|backfill`.
2. `docs/curating-skills.md`: replace "Watch and learn from upstream sources" with two sections:
   - "Keep vendored skills up to date": automatic source attachment on `skill add`; `skill outdated` (columns, statuses, `--check`, `--exit-code`); `skill upstream`; `skill update` (file table, `--accept`, `--manual`, `--write-conflicts`, confirm); the re-approval step (`skill review`, `skill edit --approve-content`), stated as: updates never approve content; `source backfill` for skills added before this release, including `--skill/--path` for ambiguous paths; the binary-compatibility note (workspaces with `origin.files_digest` need this binary or newer).
   - "Learn from references": `source attach/detach`, `source watch --skill-id`, triage outcomes (`accept --skill-id|--new-skill`, `import`, `defer`, `reject`), `source unwatch`, and the no-orphan rule.
   - Update the "Advanced intake and recovery" example if it shows `triage --decision accept` without a skill target.
3. `README.md` line 67: replace "watch repositories (`skillhub source watch`)" with "keep vendored skills current (`skillhub skill outdated`, `skillhub skill update`)".
4. `docs/design/06-source-learning-and-distillation.md`: conceptual model gains the two roles and their canonical encoding (D1), one source per repository and ref (D2), origin fields including `files_digest` (D3), the no-orphan invariant (D11), upstream checks without canonical writes for upstream-only sources (D5), and the MCP metadata-only rule (D10). Revision-check section notes that `CheckSources` also refreshes per-skill upstream state.
5. `docs/design/07-storage-and-mutation-model.md`: `runtime/operational.db` lists table `skill_upstream_state` (volatile, rebuilt by the next check); the canonical skill metadata section lists `provenance.origin.files_digest`; mutation commands list `skill_upstream_update`, `source_backfill`, `source_attach`, `source_detach`, `source_unwatch`.
6. `docs/use-cases/03-cli-and-curator-mcp-mapping.md`: one use-case block (CLI commands + MCP tools) for "Kiểm tra và cập nhật skill từ upstream" (MCP side: only `skill_upstream_status`; applying is CLI/WebUI only) and one for "Gắn nguồn học cho skill", following the existing block format; update the watch/triage entries.
7. `docs/use-cases/04-webui-user-flows-and-screen-specs.md`: §0 item 8 states the adapter exists (`internal/delivery/web`, `skillhub serve web`); §2.5 adds the Usage, Runtime, and Sources tabs (Upstream section, Upstream review, Learning section) and the real provenance card; §2.6 becomes the grouped Sources view as shipped (actions: Check now, Import more, Unwatch, distill selection with `Distill with Curator Agent`; orphan chip), with the Distill Handoff and Run Return sections matching phase 10; §1 (route table and diagram: the `/sources/watch` row and node) and the §3.5 flow title drop the Watch page; §2.7 notes that watching requires a skill and that there is no dedicated Watch page; the Skills list section mentions the upstream chip and filter; §6 states that the UI shows Not-found when a read endpoint returns HTTP 404 while the body code stays `invalid_request`; tick §9 items with links to evidence in this plan's phase reports.
8. `docs/use-cases/05-webui-design-brief.md` §4.6: replace the ASCII sketch with the grouped layout (repository heading, role chips, linked skills, distill checkbox, actions).
9. **WebUI shipped** (Vietnamese text in `docs/design/*`): `01-system-architecture.md` — replace the sentence `Web UI không thuộc V1.` with one saying the WebUI ships as the delivery adapter `internal/delivery/web` (command `skillhub serve web`) and calls the same application services; replace the Defaults item `- Không Web UI trong V1.` with `- Web UI local qua \`skillhub serve web\`, chỉ chạy khi người dùng khởi động.`; delete the out-of-scope item `- Web UI trong V1.`; name `internal/delivery/web` wherever the module table lists delivery adapters. `05-curation-lifecycle.md` line 5: remove Web UI from the "not included" list. `docs/PRD.md`: mark the Simple Web UI as delivered by `skillhub serve web` without rewriting unrelated text.
10. `docs/contracts/error-codes.md`: replace `adds no web UI` with a sentence linking `docs/contracts/web-api.md` and saying the web adapter uses the same envelope plus HTTP status.
11. New `docs/contracts/web-api.md` (short, English): base path `/api/v1`; authentication (token in the URL fragment, Bearer header); Host and Origin rules; the listen rule (loopback by default, `--loopback-only`, `--allow-host`); the endpoint list written from the `mux.HandleFunc` patterns in code with the app method each calls; the status table (link to `internal/delivery/web/errors.go`); links to `schemas/` and `internal/delivery/web/testdata/golden/`; a note that no route starts, retries, or submits a distill run and none approves skill content.
12. `README.md`: a "Web UI" section with `skillhub serve web`, the token URL, the listen rule and its effect on machines with Docker or VPN addresses, `--loopback-only`, `--allow-host`, the plain-HTTP warning, and that source builds need `make web-build` before `go build`. `docs/user-guide.md`: the same in task-oriented form, plus the Windows firewall prompt on a wildcard bind. `docs/release-runbook.md`: the `web` job, the artifact, the notices file, and the `serve web` smoke steps.

## Related code files

Modify only: `README.md`, `docs/user-guide.md`, `docs/curating-skills.md`, `docs/design/01-system-architecture.md`, `docs/design/05-curation-lifecycle.md`, `docs/design/06-source-learning-and-distillation.md`, `docs/design/07-storage-and-mutation-model.md`, `docs/use-cases/03-cli-and-curator-mcp-mapping.md`, `docs/use-cases/04-webui-user-flows-and-screen-specs.md`, `docs/use-cases/05-webui-design-brief.md`, `docs/contracts/error-codes.md`, `docs/PRD.md`, `docs/release-runbook.md`, this plan's `plan.md` and phase frontmatter (Task 13.5 only).

Create: `docs/contracts/web-api.md`.

## Implementation steps

### Task 13.1 — Read shipped behavior
- Steps: run `go build -o /tmp/skillhub ./cmd/skillhub && /tmp/skillhub skill --help && /tmp/skillhub source --help && /tmp/skillhub check --help && /tmp/skillhub serve --help && /tmp/skillhub distill --help` and keep the output open; every command and flag you document must appear there.
- Verify: the build exits 0 and the `skill --help` output contains `outdated` and `update <id>`.

### Task 13.2 — User docs
- Steps: implement Requirements 1–3.
- Verify: `grep -c "skill outdated" docs/user-guide.md docs/curating-skills.md README.md` prints a count of at least 1 for each file, and `grep -n "source watch" README.md` prints nothing.

### Task 13.3 — Design and use-case docs
- Steps: implement Requirements 4–8.
- Verify: `grep -l "skill_upstream_state" docs/design/07-storage-and-mutation-model.md && grep -l "files_digest" docs/design/06-source-learning-and-distillation.md docs/design/07-storage-and-mutation-model.md && grep -l "skill_upstream_status" docs/use-cases/03-cli-and-curator-mcp-mapping.md` prints all four paths.

### Task 13.4 — WebUI shipped docs
- Steps: implement Requirements 9–12. Write the endpoint list in `web-api.md` from `grep -hn 'mux.HandleFunc(' internal/delivery/web/*.go`, never from a plan.
- Verify: `grep -cE 'Web UI không thuộc V1|Không Web UI trong V1|^- Web UI trong V1\.$' docs/design/01-system-architecture.md` prints `0`; `grep -c 'adds no web UI' docs/contracts/error-codes.md` prints `0`; `grep -c 'skillhub serve web' README.md docs/user-guide.md` prints at least 1 for each file; `grep -c -- '--loopback-only' docs/user-guide.md` prints at least 1; every pattern printed by the grep above appears in `docs/contracts/web-api.md`.

### Task 13.5 — Link and claim check
- Steps: for every edited file, check relative links resolve (`grep -o "](\.\{0,2\}/[^)]*)" <file>` and test each path exists) and that every command quoted appears in the help output from Task 13.1.
- Verify: write the checked-link list and any mismatch to the phase report; zero mismatches remain. Then `make check` exits 0 and `make web-check` exits 0.

### Task 13.6 — Plan close
- Steps: set every phase `status: done` and every row in `plan.md` to `Done` (or `ak plan` status commands when available), set `plan.md` frontmatter `status: completed`, and write the final report naming the commits per phase.
- Verify: `grep -L 'status: done' phase-*.md` (run in this plan directory) prints nothing, and `grep -c '^status: completed' plan.md` prints `1`.

## Todo

- [x] Task 13.1 shipped behavior
- [x] Task 13.2 user docs
- [ ] Task 13.3 design + use-case docs
- [ ] Task 13.4 WebUI shipped docs
- [ ] Task 13.5 links and claims
- [ ] Task 13.6 plan close

## Success criteria

Docs describe shipped behavior only; no doc tells users to `source watch` without a skill, implies updates approve content, or says the WebUI is not part of the product.

## UX acceptance

A reader of `docs/curating-skills.md` can go from "is pdf outdated?" to an applied and re-approved update using only the commands shown, in order: `skillhub skill outdated --check`, `skillhub skill update pdf`, `skillhub skill confirm <proposal>`, `skillhub skill review pdf`, `skillhub skill edit pdf --approve-content <digest>`.

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Docs drift from code | M×M | Task 13.1 help output and the route grep are the reference; Task 13.5 claim check. |
| Translation inconsistency in Vietnamese docs | L×L | Reuse existing terms in each doc (e.g. "nguồn", "skill", "upstream"). |

## Security considerations

Document plainly that upstream updates are untrusted content, never auto-applied, never auto-approved, never shown to agents before approval, and applied only by a person through `skillhub skill update` or the WebUI (agents can only report them); that checks never run in the background (the weekly schedule only marks sources due in `skillhub status`); that "behind" means files changed in the skill folder plus the upstream commit date; and that the 3-way merge uses the system `git` (`git merge-file`).

## Rollback

Revert the docs commit.

## Failure Protocol

If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, weaken or delete a test, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP, write the same evidence to `reports/executor-<YYMMDD-HHMM>-blocker-<phase-slug>.md`, and report the blocker to the user. Never continue by self-reasoning.
