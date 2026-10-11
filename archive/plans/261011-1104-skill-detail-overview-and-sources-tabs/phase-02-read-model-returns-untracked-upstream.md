---
phase: 2
title: "Read model returns untracked upstream"
status: completed
priority: P1
effort: "1h"
dependencies: []
---

# Phase 2: Read model returns untracked upstream

## Context

`SourceService.SkillSources` (`internal/app/skill_sources.go:40`) builds the response of
`GET /api/v1/skills/{id}/sources` (`internal/delivery/web/routes_sources.go:18,32`; the only caller). At
`skill_sources.go:57-61` it drops the upstream unless `up.Status != "untracked"`. `GetSkillUpstream`
(`internal/app/upstream.go:696`) deliberately returns an untracked model (repository, ref, path, base commit,
local status, next action) for skills whose upstream entry has no source id. Data flow after the change:
`.meta/skill.yaml` `sources[]` or `provenance.origin` -> `GetSkillUpstream` -> `SkillSourcesResult.upstream`
(non-null, `status: "untracked"`) -> JSON -> Sources and Overview tabs, where Phase 4 offers Track upstream.

No new storage, field or route. The web golden `skill-sources.json` uses `review-skill` (a local skill), so it
does not change. The `web-api.md` row for this route is updated by Phase 3, which owns that doc.

## Requirements

- Return the upstream whenever `GetSkillUpstream` succeeds with a non-empty `Repository`, including `status: "untracked"`.
- Keep `upstream: null` for local skills and on lookup error.
- Leave `handleSkillUpstreamCheck` unchanged: it already rejects untracked (`routes_sources.go:49-55`).

## Files

- Modify: `internal/app/skill_sources.go`, `internal/app/skill_sources_test.go`

## Steps

1. Change the condition at `skill_sources.go:59` to `upErr == nil && up.Repository != ""`.
2. In `TestSkillSources` add a case: a skill whose `.meta/skill.yaml` has `sources: [{id: "", kind: github, repo: https://github.com/owner/repo, ref: main, path: x, roles: [upstream]}]`; assert `Upstream != nil`, `Status == "untracked"`, `Repository` set, `SourceID == ""`. Keep the existing local and learner cases (`skill_sources_test.go:41,93`) passing.

## Validation

- `go test ./internal/app -run 'TestSkillSources' -count=1`
- `go test ./internal/delivery/web -count=1` (golden unchanged)
- `make check` before merge (no binary build).

## Risks

- Another consumer could assume a non-null upstream is tracked. Only the web route consumes this result (verified by grep), and the web treats `untracked` explicitly. Low.

## Rollback

Revert the commit; the UI falls back to hiding untracked upstreams.
