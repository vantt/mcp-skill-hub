---
title: "Phase 10: Documentation"
status: todo
---

# Phase 10: Documentation

<!-- Updated: Validation Session 1 - document agent-free updates, no daemon, behind semantics, git merge engine -->

## Context

- Plan: [plan.md](./plan.md). Depends on phases 1–9 (document shipped behavior only; read the code and help text, not this plan, as the source of truth).
- Read first: `.claude/rules/documentation-management.md`, `README.md`, `docs/user-guide.md`, `docs/curating-skills.md`, `docs/design/06-source-learning-and-distillation.md`, `docs/design/07-storage-and-mutation-model.md` (runtime tree at line ~109 and operational state at lines ~514 and ~721), `docs/use-cases/03-cli-and-curator-mcp-mapping.md`, `docs/use-cases/04-webui-user-flows-and-screen-specs.md` (§2.5 Skill Detail, §2.6 Sources, §2.7 Watch), `docs/use-cases/05-webui-design-brief.md` (§4.6–4.7), `internal/delivery/cli/help.go`.
- Language: keep each document's current language (`docs/design/*` and `docs/use-cases/*` are Vietnamese; `README.md`, `docs/user-guide.md`, `docs/curating-skills.md` are English).

## Overview

Update the smallest owning surface of each document so it matches the shipped commands, tools, screens, storage, and safety rules.

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
7. `docs/use-cases/04-webui-user-flows-and-screen-specs.md`: §2.5 adds the Sources tab (Upstream section, Upstream review, Learning section) and the real provenance card; §2.6 becomes the grouped Sources view as shipped (actions: Check now, Import more, Unwatch; orphan chip); §2.7 notes that watching requires a skill and that the dedicated page is not shipped; the Skills list section mentions the upstream chip and filter.
8. `docs/use-cases/05-webui-design-brief.md` §4.6: replace the ASCII sketch with the grouped layout (repository heading, role chips, linked skills, actions).

## Related code files

Modify only: `README.md`, `docs/user-guide.md`, `docs/curating-skills.md`, `docs/design/06-source-learning-and-distillation.md`, `docs/design/07-storage-and-mutation-model.md`, `docs/use-cases/03-cli-and-curator-mcp-mapping.md`, `docs/use-cases/04-webui-user-flows-and-screen-specs.md`, `docs/use-cases/05-webui-design-brief.md`.

## Implementation steps

### Task 10.1 — Read shipped behavior
- Steps: run `go build -o /tmp/skillhub ./cmd/skillhub && /tmp/skillhub skill --help && /tmp/skillhub source --help && /tmp/skillhub check --help` and keep the output open; every command and flag you document must appear there.
- Verify: the build exits 0 and the `skill --help` output contains `outdated` and `update <id>`.

### Task 10.2 — User docs
- Steps: implement Requirements 1–3.
- Verify: `grep -c "skill outdated" docs/user-guide.md docs/curating-skills.md README.md` prints a count of at least 1 for each file, and `grep -n "source watch" README.md` prints nothing.

### Task 10.3 — Design and use-case docs
- Steps: implement Requirements 4–8.
- Verify: `grep -l "skill_upstream_state" docs/design/07-storage-and-mutation-model.md && grep -l "files_digest" docs/design/06-source-learning-and-distillation.md docs/design/07-storage-and-mutation-model.md && grep -l "skill_upstream_status" docs/use-cases/03-cli-and-curator-mcp-mapping.md` prints all four paths.

### Task 10.4 — Link and claim check
- Steps: for every edited file, check relative links resolve (`grep -o "](\.\{0,2\}/[^)]*)" <file>` and test each path exists) and that every command quoted appears in the help output from Task 10.1.
- Verify: write the checked-link list and any mismatch to the phase report; zero mismatches remain. Then `make check` exits 0.

## Todo

- [ ] Task 10.1 shipped behavior
- [ ] Task 10.2 user docs
- [ ] Task 10.3 design + use-case docs
- [ ] Task 10.4 links and claims

## Success criteria

Docs describe shipped behavior only; no doc tells users to `source watch` without a skill or implies updates approve content.

## UX acceptance

A reader of `docs/curating-skills.md` can go from "is pdf outdated?" to an applied and re-approved update using only the commands shown, in order: `skillhub skill outdated --check`, `skillhub skill update pdf`, `skillhub skill confirm <proposal>`, `skillhub skill review pdf`, `skillhub skill edit pdf --approve-content <digest>`.

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| Docs drift from code | M×M | Task 10.1 help output is the reference; Task 10.4 claim check. |
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
