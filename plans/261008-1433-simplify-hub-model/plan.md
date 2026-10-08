---
title: "Simplify the hub data model"
description: "Make Git the database, history, audit log and integrity mechanism of the hub; keep in the binary only what Git cannot give (locks, preview/confirm pins, human content approval). Move each skill's hub-side data into skills/<collection>/<id>/.meta/, reduce distillation to lessons + repo@commit pointers + cursor + decisions, and make a fresh clone of the hub work."
status: proposed
priority: P1
effort: TBD
branch: TBD
tags: [architecture, storage, distill, portability]
blockedBy: []
blocks: []
created: 2026-10-08
---

# Simplify the hub data model

## Overview

Three independent reviews of the hub data model (skills and mutation; sources,
distill and insights; runtime and distribution) found the same pattern: the
binary re-implements what Git and the upstream repositories already guarantee.
The visible cost is portability: a fresh `git clone` of a hub breaks on a new
machine. The hidden cost is about 21 files per learned idea and a large share of
`internal/distill`, `internal/insight`, `internal/mutation` and the runtime
catalog code.

Guiding principle (to be recorded in `docs/design/01-system-architecture.md`):
Git is the database, history, audit log and integrity mechanism. For a git
source, `repo@commit:path` is complete evidence. The binary adds only what Git
lacks: a workspace lock, preview/confirm pins against concurrent edits, and the
human content approval of third-party content.

## Verified starting facts (2026-10-08)

- A fresh clone lacks the empty layout directories that `init` creates; `skillhub
  status` failed with `statat sources/intake: no such file or directory` until
  they were created by hand. `internal/workspace/workspace.go:28-32` lists 12
  required directories.
- Workspace validation fails when any distill run exists but the gitignored
  package store is missing (`internal/distill/workspace_validation.go:44-49`);
  no code rebuilds packages from source plus revision.
- `ContentDigest` covers every file in a skill folder except `skill.meta.yaml`
  (`internal/skillruntime/trust.go:39-46`). Writing hub-side files into a skill
  folder today would invalidate its content approval.
- The live hub (`vantt/skill-hub`) commits host-integration files with paths
  from another machine (`/home/vantt/projects/skillhub`) and curator copies at
  v1.3.0 while the binary ships v1.5.0.
- Operation receipts embed full before/after file bodies (`internal/mutation/receipt.go:68-95`);
  one live receipt is 212 KB, contrary to `docs/design/07` §7.
- `skill.meta.yaml` duplicates `SKILL.md` frontmatter (`name`, `description`),
  the directory name (`collection`) and `git log` (`history[]`, timestamps).
  `provenance.origin` is a source with a cursor, overlapping learning sources.
- The curator's compatible-tools list omits registered tools it references
  (`skill_add_preview`, `skill_add_confirm`, `skill_review`,
  `source_watch_preview`, `source_watch_confirm`).

## Agreed decisions

| ID | Decision |
|---|---|
| D1 | Distillation is anchored to a skill: "what is worth learning" is judged against the target skill's current content. |
| D2 | Evidence for a git source is `repo@commit:path[#Lx-Ly]`. Only non-git documents need a digest and, when upstream keeps no history, a snapshot. |
| D3 | Every source belongs to at least one skill. A source saved for later is attached to an existing skill or to a new draft skill. `sources/intake/` and top-level `sources/` go away. |
| D4 | Hub-side data of a skill lives in `skills/<collection>/<id>/.meta/`. The binary applies one rule to `.meta/`: excluded from content digest, distribution (`resources/read`, native install copies) and upstream comparison. |
| D5 | `skill.meta.yaml` moves to `.meta/skill.yaml` and keeps only `id`, `status`, `routing`, `review` (approved content digest) and `sources`. `name`/`description` come from `SKILL.md`; collection from the directory; history from Git. |
| D6 | Upstream origin and learning references are one `sources` list in `.meta/skill.yaml`, each with roles `upstream` and/or `learning`. The upstream cursor (`synced`) lives with the source in `skill.yaml`; distill cursors live with the lessons in `distill.yaml`, so lessons and their cursor move in one write. |
| D7 | Distillation is one file, `.meta/distill.yaml`: `cursors` plus `lessons`. A lesson has `key`, `what`, `notable`, `where` (one entry per source; several entries = convergence) and an inline `decision` (`candidate`, `planned`, `ported`, `rejected` with `reason` and `at`). No separate decisions, comparisons, reports or snapshots; snapshots are added only when a non-git document source exists. `contrast` and R/E/F scores stay experimental until phase 0 shows they help. |

Target layout:

```text
skills/<collection>/<id>/
├─ SKILL.md  references/  scripts/ …       content: distributed, approved, compared with upstream
└─ .meta/                                   hub-side: never distributed, never in the content digest
   ├─ skill.yaml                            id, status, routing, review, sources (roles, synced)
   └─ distill.yaml                          cursors + lessons with inline decisions
runtime/                                    gitignored cache: catalog.db, shared git mirrors by URL, envs, secrets, locks
```

## Open decisions

1. One operation = one Git commit (removes the transaction WAL and the receipt
   journal) versus the current "approved locally before commit" model
   (`docs/design/07` §3, §17.3). Needs an explicit decision.
2. Cross-skill comparisons: keep per skill only, or add a hub-level location.
3. Runtime trims (single `catalog.db` without generations and pins; serve skills
   from the checkout instead of `runtime/cache/skills` copies; cheaper staleness
   check; plain telemetry store). Each is independent and can be scheduled later.

## Phases

| # | Phase | Depends on | Status |
|---|---|---|---|
| 0 | Discovery experiment: project-local `.claude/skills/distill-lab`, target `test-audit`, sources `openclaw/openclaw` and `obra/superpowers`; writes directly to `skills/default/test-audit/.meta/{skill.yaml,distill.yaml}` in the hub. `test-audit` content stays unapproved until phase 2 lands (it has never been approved; `skillhub skill review` reports "Never approved") | none | in progress |
| 1 | Hub portability: stop writing host-integration files into the hub repo and gitignore them; create layout directories lazily; fix the curator compatible-tools drift with a server-boundary test | none | pending |
| 2 | `.meta/` exclusion rule in content digest, distribution, native install and upstream diff | none | pending |
| 3 | Minimal distill model (D1-D3, D6): lessons, decisions, sources, cursor advanced in the same write as lessons; remove revision packages, run state machine, proposal/incorporation/outcome entities and LINK files | 2, findings of 0 | pending |
| 4 | `skill.meta.yaml` → `.meta/skill.yaml` (D5) with a workspace migration | 2 | pending |
| 5 | Receipts store digests only, or disappear if open decision 1 chooses one-commit-per-operation | open decision 1 | pending |

Phase files are written before each phase starts.

## Acceptance criteria

- `git clone` of a hub plus `skillhub rebuild` gives a valid, healthy workspace
  on a new machine, including a hub with distilled lessons.
- Writing or updating `.meta/` never changes a skill's content digest or review
  state, and `.meta/` is never served to agents.
- One learned idea touches one hub file (`.meta/distill.yaml`) plus the skill
  edit when applied; the Git commit is the receipt.
- Human approval, reopen-only-on-new-evidence, tombstones and coverage gaps are
  preserved.
- Phase 0 produces a scorecard that decides which lesson fields (`notable`,
  `contrast`, R/E/F) become schema.
