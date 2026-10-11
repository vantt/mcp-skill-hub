---
title: "Skill detail: Overview tab and a working Sources tab"
description: "Make the skill Sources tab list every source (upstream and learning references), add a learning reference with a preview, track an untracked upstream from the browser, and add an Overview tab with the skill's metadata and sources."
status: completed
priority: P1
effort: 14.75h
branch: main
tags: [web, skill-detail, sources, overview, backfill]
blockedBy: []
blocks: []
created: 2026-10-11
---

# Skill detail: Overview tab and a working Sources tab

## Why the Sources tab looks empty today (evidence)

The UI does render, but the read model drops data and the add form is framed as something else:

1. `SourceService.SkillSources` hides any upstream it cannot match to a source record:
   `internal/app/skill_sources.go:59` keeps the upstream only when `up.Status != "untracked"`.
   `GetSkillUpstream` (`internal/app/upstream.go:712-761`) returns `status: "untracked"` for a skill whose
   `.meta/skill.yaml` has an upstream `sources[]` entry with an empty `id` (or only `provenance.origin`),
   because `loadTrackedSkills` skips entries without an id (`internal/app/upstream.go:241`).
   Live proof: `/home/vantt/skill-hub/skills/default/herdr-cook-plan/.meta/skill.yaml` has
   `sources: [{id: "", kind: github, repo: https://github.com/thieung/herdr-cook-plan, roles: [upstream]}]`,
   so `GET /api/v1/skills/herdr-cook-plan/sources` returns `upstream: null`.
2. `UpstreamSection` returns `null` when `upstream` is null (`web/src/screens/skill-detail/UpstreamSection.tsx:36`);
   its untracked branch (`:118-125`) is never reached, and even there the only way forward is a CLI command.
3. Learning references come only from `sources/skills/LINK-*.yaml` (`internal/app/skill_sources.go:64`,
   `internal/app/source_links.go:33`). The live hub has none.
4. The only add control is the "Add learning reference" form in `LearningSection.tsx:98-116`: GitHub URL only,
   cannot link an already watched source, and confirms without showing the preview (`:48-55`).

No new storage is needed: Git files already hold everything (`.meta/skill.yaml` `sources[]`/`provenance`,
`sources/catalog/*.yaml`, `sources/skills/LINK-*.yaml`). The attach preview/confirm API exists
(`internal/delivery/web/routes_sources.go:22,27`), and the backfill logic exists in
`internal/app/source_backfill.go` (`PreviewBackfill` `:85`, `ApplyBackfill` `:491`); the web lacks only a
pinned preview route for it. That logic currently replaces the whole `sources:` list and deletes
`provenance` (`source_backfill.go:337-353`, `:426-442`), so it is fixed first (user decision).

## Outcome and acceptance criteria

- [x] Sources tab of a skill with an upstream origin shows it, including the "Not tracked" state (for example `herdr-cook-plan`).
- [x] For a Not tracked upstream, a **Track upstream** button shows the exact change (full diff of `.meta/skill.yaml` and any new `sources/catalog/*.yaml`) and applies it only on confirm; afterwards the upstream is tracked. The `skillhub source backfill --skill <id>` command stays as a fallback hint.
- [x] Backfill (web Track upstream and CLI `skillhub source backfill`) updates only the upstream entry: other `sources:` entries, the `provenance` block and unrecomputed upstream fields (for example `folder_digest`) are kept; an id-less upstream still gets its id; the preview diff touches only that entry. This is an intentional CLI contract change, documented in `docs/curating-skills.md`.
- [x] Sources tab lists every learning reference with unlink, and shows one clear empty state when the skill has no source.
- [x] An **Add source** button in the top `ActionBar` adds a learning reference (link a watched source or watch a GitHub repository), always showing the proposal before writing.
- [x] A new **Overview** tab, first and opened by default, shows structured metadata from existing API fields (name, description, collection, lifecycle state, routing operations, triggers, not-for and min scope, file count, provenance) and the skill's sources, with a link to the Sources tab. No raw `.meta/skill.yaml`, no new endpoint for it.
- [x] `?tab=review` and every other existing tab URL keep working.
- [x] `make check`, `make web-check` and `make web-e2e` pass; a new e2e spec tracks an upstream and links a source from the browser offline.

Non-goals: new source roles, new storage, a raw metadata view, regenerating README screenshots, and
(decided) changing telemetry: confirming a track-upstream proposal through the shared source confirm route
keeps the existing event.

## Precondition

Satisfied: the `ActionBar` and frontmatter work is committed (`63546a8`), so worktree agents see
`web/src/components/ActionBar.tsx`.

## Phases

| # | Phase | Status | Effort | Depends on |
|---|---|---|---|---|
| 1 | [Shared frontend contracts](./phase-01-shared-tab-contract-and-format-helpers.md) | Done | 1.75h | none |
| 2 | [Read model returns untracked upstream](./phase-02-read-model-returns-untracked-upstream.md) | Done | 1h | none |
| 3 | [Backfill keeps data, and the track-upstream preview route](./phase-03-backfill-and-track-upstream-route.md) | Done | 4h | none |
| 4 | [Sources tab: list, add, track](./phase-04-sources-tab-list-add-track.md) | Done | 3.5h | 1 |
| 5 | [Overview tab](./phase-05-overview-tab.md) | Done | 2h | 1 |
| 6 | [Integration: e2e, docs, build](./phase-06-integration-e2e-docs-build.md) | Done | 2.5h | 2, 3, 4, 5 |

## Dependency graph

```text
t=0:   Phase 1 (web contracts)    Phase 2 (read model)    Phase 3 (backfill fix, then track route)
          |          |                   |                       |
          v          v                   |                       |
       Phase 4    Phase 5                |                       |
          |          |                   |                       |
          +----------+---------+---------+-----------------------+
                               v
                            Phase 6
```

Wave 1: Phases 1, 2, 3 in parallel. Wave 2: Phases 4 and 5 in parallel (2 and 3 may still be running).
Wave 3: Phase 6. The backfill fix stays inside Phase 3 because the route's minimal-diff test depends on it;
at 4h, Phase 3 is still shorter than the 1 -> 4 frontend path (5.25h), so it does not lengthen the critical path.

## Parallel execution

| Phase | Owner files (exclusive) | Depends on | Can run with |
|---|---|---|---|
| 1 | `web/src/api/types.ts`, `web/src/api/queries.ts`, `web/src/screens/skill-detail/SkillDetailScreen.tsx`, `ReviewTab.tsx`, new `detail-format.ts`, `detail-format.test.ts`, `skill-detail-tabs.test.tsx`, new `OverviewTab.tsx` (shell only) | none | 2, 3 |
| 2 | `internal/app/skill_sources.go`, `internal/app/skill_sources_test.go` | none | 1, 3, 4, 5 |
| 3 | `internal/app/source_backfill.go`, `internal/app/source_backfill_test.go`, `internal/delivery/cli/source_test.go`, `docs/curating-skills.md` (backfill section), new `internal/app/source_backfill_proposal.go`, new `internal/app/source_backfill_proposal_test.go`, `internal/app/source.go` (one summary case), `internal/skill/lifecycle.go` (export the diff renderer), new `internal/delivery/web/routes_upstream_track.go`, new `internal/delivery/web/routes_upstream_track_test.go`, `docs/contracts/web-api.md` | none | 1, 2, 4, 5 |
| 4 | `web/src/screens/skill-detail/SourcesTab.tsx`, `UpstreamSection.tsx`, `LearningSection.tsx`, `sources-tab.test.tsx`, new `AddSourceForm.tsx`, new `add-source-form.test.tsx` | 1 | 2, 3, 5 |
| 5 | `web/src/screens/skill-detail/OverviewTab.tsx` (takes over from 1), new `overview-tab.test.tsx` | 1 | 2, 3, 4 |
| 6 | new `web/e2e/skill-sources.spec.ts`, `web/e2e/smoke.spec.ts`, `web/e2e/tier2.spec.ts`, `web/e2e/a11y.spec.ts`, `web/e2e/ux/flows.ts`, `web/e2e/ux/shots.spec.ts`, `docs/user-guide.md` | 2, 3, 4, 5 | none |

Phase 3 does not touch `routes_sources.go` or `skill_sources.go`, so it never overlaps Phase 2; it only
calls the `yaml.Node` helpers in `upstream_update.go`, which no phase edits. The new route
registers itself through `registerRoutes` in its own file (same pattern as `routes_usage.go:13`), and the
confirm step reuses the existing `POST /api/v1/sources/proposals/{proposal_id}/confirm`.

Read-only for UI phases (import, never edit): `web/src/api/*` after Phase 1, `web/src/components/*`,
`ProvenanceCard.tsx`, `UpstreamReview.tsx`, `ContentTrustCard.tsx`.

## Rules for every phase

- UI phases (1, 4, 5) use `/ak:frontend-design` for layout and visual decisions and `/ak:frontend-development` for the React implementation; follow the existing `fg-*` classes and the `ActionBar` at the top of a page (no sticky bottom bar).
- No plan IDs, phase numbers or audit labels in code comments, test names or commit messages.
- Do not build binaries while another agent rebuilds. Phases 1 to 5 verify with Go tests, typecheck, lint and vitest only. Phase 6 owns `make web-build`, `go build ./cmd/skillhub` and `web/.e2e/skillhub`.
- Never write to `/home/vantt/skill-hub`; tests use temp workspaces.

## Risks

| Risk | L x I | Mitigation |
|---|---|---|
| Overview as default tab breaks specs that assume Review opens first | High x Medium | Phase 6 pins those specs to `?tab=review` (listed with line numbers) |
| Backfill fix changes CLI behavior and `yaml.v3` re-encoding could still reformat lines | Medium x Medium | Phase 3 edits with `yaml.Node` via the existing helpers in `upstream_update.go`, keeps the file's indentation, tests the real `herdr-cook-plan` shape and asserts a minimal diff; contract change documented |
| Untracked upstream now visible: "Check now" would hit a route that rejects it (`routes_sources.go:49`) | High x Low | Phase 4 disables Check and Review for untracked and offers Track upstream instead |
| Front end codes against the track response before Phase 3 lands | Medium x Low | Phase 1 fixes the JSON shape in `types.ts`; Phase 3 tests assert the same field names |
| Attach by URL needs network | Medium x Low | Clear checking state and error banner; e2e uses an offline filesystem source; Track upstream needs no network |

## Rollback

Each phase is one commit. Phase 4 or 5 reverts alone (Phase 1's shell keeps Overview compiling). Phase 1
reverts only together with 4 and 5. Phases 2 and 3 revert alone: without 2 the UI shows no untracked upstream;
without 3 the Track upstream button gets a 404, the command hint still works, and the CLI returns to the old
replace behavior. Revert 3 together with the Track button part of 4 if needed.
