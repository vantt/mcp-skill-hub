---
phase: 3
title: "Source first analysis and import"
status: complete
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 3: Source first analysis and import
<!-- Updated: Validation Session 1 - add import of existing skills as drafts -->

## Goal
Onboarding a GitHub source produces useful output right away: the user can import the repo's existing skills as **draft** skills (activated only by the user), an initial analysis run lands findings in the inbox, and oversized sources get a clear, actionable size error — never a silent "up to date".

## Context
- UX audit P0-2; P2 duplicate capture / `source show` header (U26).
- Design `docs/design/06-source-learning-and-distillation.md:719,776` ("initial extraction", "Initial scan and incremental run both produce coverage"). Governance unchanged: insights are proposals; humans apply (apply may create a draft skill).
- Code: `internal/app/source.go`, `internal/app/distill.go`, `internal/source/filesystem.go:248-258` (limits), `internal/delivery/cli/source.go`, `internal/delivery/cli/distill.go`.

## Files to Create / Modify
- Create: `internal/app/source_import.go`, `internal/delivery/mcpserver/source_import_tools.go` (+ tests)
- Modify: `internal/delivery/mcpserver/server.go` (register), `internal/app/source.go`, `internal/app/distill.go`, `internal/source/filesystem.go` (error detail), `internal/delivery/cli/source.go`, `internal/delivery/cli/distill.go`, `internal/delivery/cli/help.go` (source/distill text), curator SKILL.md row "Start learning from a source" (both copies)
- Tests alongside

## Tasks & Steps
- [x] Confirming onboarding marks the source as "needs initial analysis" instead of baseline-up-to-date; `check`/`distill prepare` pick it up and prepare an initial run over the scoped path.
- [x] Triage estimates size (files/bytes) against limits; warn at triage and suggest `--path <subdir>` when over.
- [x] Limit errors name the limit and actual values and give the exact `source triage ... --path` fix; if all sources fail, exit non-zero.
- [x] Output after confirm: "Watching <id>. First analysis is ready: ask your agent 'distill new sources' or run `skillhub distill prepare <id>`." State plainly that watching does not auto-import skills; accepted insights can create draft skills.
- [x] Import: `skillhub source import <source-id> [--path <subdir>] [--skill <name>]... [--yes]` and MCP `source_import_preview` / `source_import_confirm`. Discovers `SKILL.md` folders in the watched revision (bounded by source limits), previews the list (name, target id, collection, conflicts with existing ids), and on confirm creates **draft** skills through the mutation service with provenance (source id, revision, path) so later upstream checks can diff them. Existing ids are skipped with a message, never overwritten. Never auto-activates.
- [x] Import output: "Imported 5 draft skills. Next: review with `skillhub skill show <id>`, then `skillhub skill activate <id> --yes` (or ask your agent)."
- [x] Curator skill row "Import skills from a source" → `source_import_preview/confirm` (both SKILL.md copies, bump `CuratorSkillVersion`); design doc 06 gets a short "Import existing skills as drafts" subsection.
- [x] Capturing an already-captured URL returns the existing candidate (idempotent) instead of a duplicate; `source show <id>` prints the one record without list header.

## Verification
- Import: fixture repo with 3 SKILL.md folders (one id conflict) → preview lists 3 with 1 conflict; confirm creates 2 drafts with provenance; re-run is idempotent; oversized repo → named-limit error with `--path` fix; MCP and CLI results equivalent.
- `go test ./internal/app ./internal/source ./internal/delivery/cli ./internal/delivery/mcpserver`: new source → first run prepared; oversized source → triage warning + named-limit error + exit ≠ 0; duplicate capture idempotent.
- Manual: scratch HOME, capture + triage + confirm a small public skills repo with `--path`, `distill prepare` prepares ≥1 run.

## Risks
- Imported content is untrusted data: validate frontmatter/size/paths like any canonical skill; strip nothing silently — report skipped files.
- Changing baseline semantics could make later `check` re-run full scans: first run must record coverage so subsequent runs stay incremental (design 06:719).
