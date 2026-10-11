---
title: "Package distill-lab in the hub and turn host skill copies into thin wrappers"
description: "Ship distill-lab as a bundled system skill installed only in the hub folder, keep system skill content once in .agents/skills with thin wrappers for Claude Code and Gemini CLI, and make every distill instruction runnable."
status: pending
priority: P2
effort: 25.5h
branch: main
tags: [system-skills, distill, hostintegration, init, doctor, web, docs]
blockedBy: []
blocks: []
created: 2026-10-11
---

# Package distill-lab in the hub and turn host skill copies into thin wrappers

## Why (evidence)

Full evidence with `file:line` is in [reports/scout-notes.md](reports/scout-notes.md).

- distill-lab exists only as a dev copy in this code repo (`.claude/skills/distill-lab/`, not embedded). A hub owner
  who clicks **Distill** in the web UI gets a handoff for "an agent that has the distill-lab skill", but nothing
  installs it, and `docs/sources-and-learning.md:178` says Skill Hub "does not ship the distill-lab skill".
- The Home action `review_lessons` prints `distill.py list <path>` (`internal/app/curation_home.go:484-487`),
  which cannot run: no interpreter, no script path. The skill itself falls back to a personal path
  `~/skill-hub` (`.claude/skills/distill-lab/SKILL.md:292`).
- `init`/`connect` write three identical full copies of `system-curator` (`internal/hostintegration/integration.go:34-36`).
  Codex and Gemini CLI both read `.agents/skills` natively (official docs, cited in the scout notes); Claude Code
  reads only `.claude/skills`.

## Outcome and acceptance criteria

- [ ] After `skillhub init <hub> --yes` the hub folder holds `.agents/skills/distill-lab/SKILL.md` and
      `.agents/skills/distill-lab/scripts/distill.py`, byte-equal to the embedded bundle, plus thin wrappers at
      `.claude/skills/distill-lab/SKILL.md` and `.gemini/skills/distill-lab/SKILL.md`.
- [ ] `system-curator` follows the same layout: full content once in `.agents/skills/system-curator/SKILL.md`,
      wrappers in `.claude/skills/` and `.gemini/skills/`, in project and user (`-g`) scope.
- [ ] `connect` in a project other than the hub, `connect -g`, and `connect --hub` never write any distill-lab file
      (asserted by tests for all three request shapes).
- [ ] `connect --hub` and `connect --remove` delete owned system-skill files (current or previously shipped
      bytes, owned wrappers); any other content at those paths is reported as a conflict and left in place.
- [ ] A hub created by the previous binary (three full curator copies) is migrated by `doctor --fix --yes` to
      canonical + wrappers with no conflict; an edited copy produces a conflict naming its path and stays unchanged.
- [ ] In a sandbox, `python3 <hub>/.agents/skills/distill-lab/scripts/distill.py list <fixture distill.yaml>`
      exits 0 and prints numbered lessons; the bundled SKILL.md contains no `~/skill-hub`.
- [ ] `skillhub doctor` in a hub without Python 3 or PyYAML keeps its status and adds one warning naming what is
      missing and how to install it.
- [ ] `skillhub status` prints the `review_lessons` next step as `cd <hub> && python3
      .agents/skills/distill-lab/scripts/distill.py list <path>`, and that command runs in the sandbox.
- [ ] The web handoff, Distill tab note, source confirmation text and user docs say where distill runs (an agent
      started in the hub folder) and what it needs; no doc says the skill is not shipped.
- [ ] `make check`, `make web-check` and `go test ./internal/hostintegration -run TestMatrixSynchronized` pass.

## Scope and non-goals

In scope: embed distill-lab as a multi-file system skill; generalize system-skill handling beyond the single
curator id; canonical-plus-wrapper installer with migration and removal; distill-lab content fixes; product text,
tests, docs; sandbox verification; retiring the dev copy behind an owner gate.

Non-goals: a Go port of `distill.py`; any UI or command that launches an agent; serving distill-lab over MCP or
installing it into projects, user scope or remote clients; changing the `distill.yaml` schema; changing which
hosts are verified in the compatibility matrix.

## Design decisions

1. **Canonical layout.** Every system skill's real files live in `<root>/.agents/skills/<id>/` (SKILL.md and
   `scripts/`). Codex reads that folder directly, so Codex gets no wrapper. Claude Code and Gemini CLI get a
   one-file wrapper `<host dir>/skills/<id>/SKILL.md`. The canonical files are host-independent, so they become a
   new shared change kind instead of belonging to one host (today Codex's native skill path *is* the canonical
   path, `integration.go:35`).
2. **Wrapper mechanism: a generated wrapper SKILL.md** (option 1). Same `name` and `description` as the canonical
   frontmatter, a short body that tells the agent to read and follow the canonical SKILL.md and to run scripts
   from the canonical folder. Claude wrappers use `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/`, which Claude
   Code substitutes in skill content (official docs); Gemini wrappers name the path in words relative to the
   folder that holds `.gemini`. The relative path is the same in project and user scope.
   - Rejected, symlinked directories: Claude Code and Codex document symlink support, but the installer refuses
     symlink components by design (`internal/hostintegration/files.go:19-35,51`), Windows needs Developer Mode
     or admin for symlinks and Git with `core.symlinks=false` checks them out as text files, and Gemini CLI does
     not document symlink loading.
   - Rejected, hash-verified copies: that is what we have today and is what the owner's rule removes.
   - Evidence limits: only Claude Code is verified end to end in this project (`docs/mcp-compatibility-matrix.json`).
     Wrapper-following is verified for Claude Code in Phase 7; for Gemini CLI it is unverified. Current Gemini CLI
     reads `.agents/skills` itself and gives it precedence over `.gemini/skills`, so the Gemini wrapper is
     shadowed there and only serves older Gemini versions.
3. **Bundle model.** `internal/systemskills` exposes a list of system skills, each with id, version, files
   (relative path, bytes), `HubOnly` and `ServedOverMCP` flags, and a list of previously shipped file digests.
   Multi-file content is embedded with `embed.FS`. The existing `CuratorBundle()` API stays for MCP distribution.
   The editable source stays in `system-skills/<id>/`, mirrored byte for byte in `internal/systemskills/<id>/` and
   guarded by the existing sync-test pattern (`internal/systemskills/embed_test.go:22-28`).
4. **Ownership.** A canonical file is ours when its bytes equal the current bundle or any previously shipped
   version (known digests, generated once from Git history, including every committed version of the dev copy).
   A wrapper is ours when re-rendering the wrapper template with the description found in the file reproduces
   it byte for byte, so description changes between versions do not orphan wrappers. Anything else is the user's:
   it is reported as a conflict and never overwritten or deleted. This extends today's single-string check
   (`internal/hostintegration/curator-source.go:27-29`).
   - Behavior change, stated: today `doctor --fix` overwrites unknown curator content after confirmation
     (`internal/hostintegration/doctor-skew_test.go:62-75`). Under this plan unknown content is a conflict; known
     old versions are still upgraded silently. This matches the owner's rule that edited copies are reported.
5. **Executable bit.** Scripts are written `0644` and always invoked as `python3 <path>` (Windows: `py -3` or
   `python`). No step relies on the exec bit, so Windows and `embed.FS` (which drops modes) behave the same.
6. **Hub-only install.** distill-lab is planned only when the plan is local, project scope, and `Root ==
   Workspace` (the hub folder itself). `connect` from a project sets `Root` to the project
   (`internal/app/host-connections.go:23`) and `-g` sets it to home (`:26`), so neither installs it.
7. **Tracked vs ignored: ignored.** The hub `.gitignore` gets `/.agents/skills/<id>/`, `/.claude/skills/<id>/`,
   `/.gemini/skills/<id>/` for both system skills. Reason: these files are generated from the binary version, so
   committing them churns on every upgrade and a clone on another machine regenerates them on `init`/`doctor
   --fix` anyway. Effect: `doctor` on old hubs reports the missing entries as fixable (existing finding
   `gitignore_entry_missing`, `internal/workspace/workspace.go:172-176`) and reports already tracked system-skill
   files with a `git rm -r --cached` fix (new finding, same pattern as `machine_file_tracked`, `:177-179`);
   `git add -A` no longer picks them up. Other skills a user keeps under `.claude/skills/` stay tracked.
8. **Removal.** Remote (`connect --hub`) and `--remove` plans delete owned wrappers and owned old full copies for
   the selected hosts; the shared canonical files go only when no Skill Hub registration remains in that root
   after the plan. Verified MCP-skills hosts (none today) drop the curator wrapper but keep the distill-lab wrapper,
   because distill-lab is never served over MCP.
9. **Doctor hints.** In the hub, doctor adds a warning (never a status change) when `python3`/`py -3`/`python` or
   `import yaml` is missing. Wrapper and canonical integrity are reported through the existing per-file host
   inspection items.
10. **Runnable product text.** The app keeps emitting a hub-relative command
    (`python3 .agents/skills/distill-lab/scripts/distill.py list <rel path>`), so JSON and MCP output carry no
    machine path; the CLI status renderer prefixes `cd <hub> && `.

## Phases

| # | Phase | Effort | Depends on | File |
|---|---|---|---|---|
| 1 | Embed system skill bundles and distill-lab | 3h | - | [phase-01](phase-01-embed-system-skill-bundles.md) |
| 2 | distill-lab content: hub path, runnable steps, requirements | 2h | 1 | [phase-02](phase-02-distill-lab-content.md) |
| 3 | Installer: canonical folder, wrappers, migration, removal, doctor hint | 8h | 1 | [phase-03](phase-03-installer-canonical-and-wrappers.md) |
| 4 | Hub Git ignore entries and tracked system-skill finding | 2h | - | [phase-04](phase-04-hub-gitignore-and-tracked-finding.md) |
| 5 | Product text and tests (Go, web, e2e, one screenshot) | 4h | - | [phase-05](phase-05-product-text-and-tests.md) |
| 6 | User docs | 3h | 2, 3, 4, 5 | [phase-06](phase-06-docs.md) |
| 7 | Sandbox verification | 3h | 6 | [phase-07](phase-07-sandbox-verification.md) |
| 8 | Retire the dev copy (owner gate) | 0.5h | 7 + owner go-ahead | [phase-08](phase-08-retire-dev-copy.md) |

## Parallel execution and file ownership

| Wave | Phases | Owned files (no overlap inside a wave) |
|---|---|---|
| 1 | 1, 4, 5 | P1: `internal/systemskills/**`, `system-skills/distill-lab/**`. P4: `internal/workspace/workspace.go`, `internal/workspace/machine_files_test.go`. P5: `internal/app/curation_home.go`, `internal/app/curation_home_test.go`, `internal/app/source.go`, `internal/delivery/cli/source.go`, `internal/delivery/cli/source_test.go`, `internal/delivery/cli/curation.go`, `internal/delivery/cli/workspace_next_steps_test.go`, `internal/distill/distill_model.go`, `web/src/domain/handoff-brief*.ts`, `web/src/screens/skill-detail/DistillTab.tsx`, `web/src/screens/distill/*`, `web/e2e/tier3.spec.ts`, `web/e2e/journeys.spec.ts`, `docs/images/web/distill-handoff.png` + its manifest entry |
| 2 | 2, 3 | P2: `system-skills/distill-lab/**`, `internal/systemskills/distill-lab/**`. P3: `internal/hostintegration/**`, `docs/mcp-compatibility-matrix.json`, `internal/app/workspace.go`, new `internal/app/distill_requirements.go` (+ test) |
| 3 | 6 | `docs/*.md` listed in the phase, `docs/design/05-curation-lifecycle.md`, `AGENTS.md` |
| 4 | 7 | none (scratch sandbox only; fixes go back to the owning phase) |
| 5 | 8 | `.claude/skills/distill-lab/**` of this code repo |

Phase 5 hard-codes the canonical script path `.agents/skills/distill-lab/scripts/distill.py` fixed by decision 1,
so it does not wait for Phase 3. Phase 3 tests read bytes through the bundle API, never literal content, so
Phase 2 edits do not break them.

## Risks

| Risk | L x I | Mitigation |
|---|---|---|
| A host does not follow the wrapper and runs with the wrapper text only | M x H | Wrapper body is a single imperative instruction with an explicit path; Phase 7 checks Claude Code by hand; Gemini shadows the wrapper with `.agents/skills`; Codex needs none. Docs state Claude Code is the verified host. |
| Claude Code asks for permission to read `~/.agents/...` from a user-scope wrapper | M x M | Phase 7 checks; if it prompts, add the canonical folder to Claude user `additionalDirectories` in Phase 3 (same mechanism as `RuntimeAccessDirs`). |
| Python 3 or PyYAML missing on the hub machine | M x M | Doctor warning, a clear `ImportError` message in `distill.py`, a docs requirement line; the binary never needs Python. |
| The experimental `distill.yaml` schema is now frozen by shipped fixtures and `internal/distill` | M x M | Keep the script header and schema unchanged in this plan; version the skill (`version:` frontmatter) so later schema changes are visible. |
| Conflicts abort a whole `doctor --fix` plan (existing `ConflictError` contract, `integration.go:91-93`) | L x M | Known digests cover every shipped version, so only real user edits conflict; the message names the path and the fix. |
| Windows paths and missing exec bit | L x M | `filepath`-based paths, `0644`, interpreter-invoked scripts, a Windows path unit test, docs mention `py -3`. |
| A hub already has an unrelated workspace skill named `distill-lab` | L x M | Not served natively, so no file clash; an owner question below decides whether to reserve the id. |

## Compatibility and rollback

- Existing hubs migrate on `doctor --fix --yes` or the next `init`/`connect`; nothing is deleted unless owned.
- Downgrading the binary is safe: the old binary sees a wrapper at the old full-copy path, reports skew and, after
  confirmation, writes its full copy back (`prepareExactFile`, `integration.go:390-396`). Left-over distill-lab
  files are inert and stay ignored.
- Each phase is one or a few commits; revert them in reverse wave order. Phase 4 ignore lines are append-only
  and harmless after a revert.

## Owner follow-ups

- Phase 8 needs an explicit go-ahead before the dev copy in `.claude/skills/distill-lab/` changes.
- Later: port the deterministic part of `distill.py` to Go (out of scope here).

## Open questions for the owner

1. Keep the Gemini wrapper even though current Gemini CLI reads `.agents/skills` first (the plan keeps it, per
   your rule), or drop it for Gemini?
2. Should `distill-lab` become a reserved skill id in `skill create`/`skill add`, like a system skill?
3. Phase 8: replace the dev copy with a wrapper pointing at `system-skills/distill-lab/`, or delete it?
