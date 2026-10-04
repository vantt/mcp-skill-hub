---
phase: 13
title: "Documentation"
status: pending
priority: P2
effort: 6h
dependencies: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]
---

# Phase 13: Documentation

## Context

- Rule: `.claude/rules/documentation-management.md`: update the smallest owning surface, link to schemas instead of copying them, read before editing, verify claims after editing.
- Design docs are Vietnamese; keep edits in Vietnamese and in the existing style. User docs are English.
- Targets (read each in full before editing):
  - `docs/design/01-system-architecture.md`: §2 invariants table (row "Progressive disclosure"), §6 progressive loading sequence and boundary bullets, §7.2 `runtime/cache/` layout, §11 security constraints, §12 non-goals ("Tự execute skill scripts").
  - `docs/design/02-agent-hub-protocol.md`: §2 bootstrap text, §3 protocol layers (activation via `skill_get` / `skills/get` with `local`, preflight, trust), §4.3 task description (English normalization), §5.2 `resolved` (`setup`), §7.1 normal path.
  - `docs/design/03-resolver-design.md`: §3 routing metadata example (`examples`, `counter_examples`, `technologies`, `topics`), validation rules (lint warnings), §4.2 FTS document (columns and weights, note that triggers rank at 1.0 unless Phase 12 changed it), §7 feature model (example and counter-example folding, technology match), §9 calibration (link to the Phase 12 report outcome).
  - `docs/design/04-telemetry-reproducibility-evaluation.md`: §3.2 core events (`skill.doctor_checked`, `transcript.tool_observed`, server-observed `skill.loaded`), §3.4 outcome semantics (server-observed activation and attribution classes vs host claims), §4.2 retention (14-day raw, 180-day daily rollups), §5 storage, §7 golden corpus (generated routing cases, leave-one-out, no-skill set, gate), and a short note that named measurement cases are deferred.
  - `docs/user-guide.md`: "How the agent picks a skill" (local path, preflight, setup_required), "Troubleshooting" (`skill doctor`, restricted snapshots), "Command cheat sheet".
  - `docs/curating-skills.md`: "Review diagnostic facts" (script trust, `--approve-scripts`), "Edit and improve a skill" (`--example`, `--counter-example`, runtime block), "Command map"; new short section "Measure usage" (`telemetry funnel`, Usage tab, `import-transcripts`, what each basis means).
  - `README.md` only if its feature list or command list names behavior that changed.

## Requirements

1. State the principle change plainly in 01 §2/§6/§12: skills are delivered as MCP content **and** as a digest-pinned local snapshot; the hub still never executes skill code in the MCP flow; `skill doctor` runs checks only on explicit user command; third-party scripts require a human approval bound to the execution digest.
2. Document the minimum binary version for workspaces that use the new manifest fields, and that the catalog rebuilds automatically after upgrade.
3. Document the privacy boundary of the transcript import (fields stored, explicit `--project`, window clamp).
4. Do not copy schemas; link `schemas/skill-metadata.schema.json`, `schemas/skill-resolve-response-v1.schema.json`, `schemas/telemetry-event-v1.schema.json`.
5. No plan IDs, phase numbers, or decision labels in the docs.

## Files

Modify: the files listed in Context, nothing else.

## Steps

1. Read each target; edit only claims made false by Phases 1–12 and add the missing surfaces.
2. Verify every command and flag against `go run ./cmd/skillhub help <cmd>` output, and every field name against the committed schemas.
3. Check internal links resolve (`grep -o '](\S*\.md[^)]*)'` per edited file and confirm each target exists).

## Tests and validation

- `go run ./cmd/skillhub help skill`, `help telemetry`, `help eval` match the documented commands.
- `make check` (docs are not tested, but the help tests guard the command surface).

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Docs overstate guarantees (for example "scripts can never run") | Medium × Medium | Phrase as implemented: withheld scripts are absent from the snapshot; hosts control execution. |
| Vietnamese/English drift between design and user docs | Low × Low | Each fact is documented once in its owner doc and linked from the other. |

## Rollback

Revert the docs commit.
