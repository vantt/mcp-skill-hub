---
title: "Runtime profile serves only active, servable skills"
description: "skill_get, skills/get and resources/read in the runtime profile return content only for skills that skill_resolve could return: active and servable. The curation profile keeps reading every lifecycle state."
status: completed
priority: P1
effort: 3h
branch: (implementation gets its own worktree)
tags: [mcp-server, runtime, lifecycle, contract]
blockedBy: []
blocks: [261014-1600-tools-list-budget, 261014-1530-fresh-clone-e2e-test]
created: 2026-10-14
---

# Runtime profile serves only active, servable skills

## Outcome

The user decided on 2026-10-14 (recorded in `docs/design/02-agent-hub-protocol.md`, section on distribution):
a skill that `skill_resolve` may not return is not delivered by the runtime profile either. In the `runtime`
profile, `skill_get` and the skills extension (`skills/get`, `resources/read`) return content only for a skill that
is `active` and passes `ValidateServableSkill`. Everything else (draft, deprecated, archived, or active but not
servable) returns a tool error with no content. The `curation` profile, and the combined profile that includes it,
keep reading every lifecycle state, because curating needs drafts.

## Evidence (verified 2026-10-14)

- `internal/delivery/mcpserver/skill_tools.go:157`: the `skill_get` description says "Get a skill by ID in any
  lifecycle state".
- `skill_tools.go:164-178`: the handler reads any state through `ReadSkillCore` and returns `Content`; it withholds
  content only when `local.Status` is `review_required` (unreviewed third-party). A user-written draft is returned
  in full.
- `internal/delivery/mcpserver/tool_registration.go:18-21`: `skill_get` is in the runtime profile, which `connect`
  installs into every project.
- `internal/delivery/mcpserver/skills_extension.go:143-151`: non-active skills get `local.status: unavailable`
  with `not_servable`; check whether content is still returned on this path.

## Acceptance criteria

- Runtime profile, `skill_get` on a draft, deprecated, archived, or active-but-not-servable skill: `IsError: true`,
  no `content`, no `local.path`; a stable error code (reuse an existing one if it fits, e.g. `not_found` for
  non-active and `not_servable` for broken; document the choice) and a FIX that does not leak the content.
- Runtime profile, `skills/list` / `skills/get` / `resources/read`: the same rule; a non-servable or non-active
  skill is neither listed nor readable.
- Runtime profile over the remote paths (HTTP listener and `skillhub mcp proxy`): the same results, because they
  reach the same handlers; one test through each path if they decode arguments separately.
- Curation and combined profiles: behaviour unchanged (drafts readable, review gating unchanged).
- The bundled curator skill keeps its current special case only where it applies today; do not widen it.
- The runtime `skill_get` description no longer says "any lifecycle state". Keep it short: the tool list has a
  byte budget planned in `261014-1600-tools-list-budget`; re-pin tool-list digests in the same change.
- Telemetry: a refused `skill_get` must not be recorded as a successful load; follow how blocked loads are
  recorded today and keep `EventVersion` and event types unchanged.
- Docs: `docs/design/02-agent-hub-protocol.md` already states the rule; update any user-facing doc that says
  `skill_get` reads drafts from a project (search `docs/` and `README*`), smallest owning surface.

## Phase 1: gate runtime delivery (single phase)

Files (expected; confirm by reading first):
- `internal/delivery/mcpserver/skill_tools.go` (handler and description; the adapter already knows its profile)
- `internal/delivery/mcpserver/skills_extension.go` (list/get/read paths)
- tests next to them, e.g. `skills_profile_test.go`, `skills_get_curator_test.go`, a new runtime gate test
- `internal/delivery/mcpserver/tools_list_digest_test.go` (re-pin)
- docs found by the search above

Steps:
1. Read the handlers and existing profile tests. Write failing tests first: runtime `skill_get` on a draft, on an
   archived skill, and on an active skill whose `SKILL.md` name differs from its id; runtime `skills/get` and
   `resources/read` on the same; curation `skill_get` on a draft still returns content.
2. Implement the gate in one place used by all runtime delivery paths (servability is already computed for
   resolution; reuse it rather than re-validating per call if a cached result exists).
3. Shorten the runtime description; re-pin digests.
4. Update docs.

Validation (narrowest first):
```bash
go test ./internal/delivery/mcpserver -run 'Profile|SkillGet|Skills' -count=1
go test ./internal/delivery/mcpserver ./internal/app -count=1
make check
```

Risks:
- An agent that relied on reading drafts from a project loses that path. Intended; the curation profile and the
  CLI keep it.
- The servability check must use the same source as resolution, or a skill could be resolvable but not gettable.
  Test one skill through both.

Rollback: revert the commit; no data or schema changes.

Constraints: implementation in its own git worktree; tests never touch the live hub (/home/vantt/skill-hub), real
HOME or real projects. No plan IDs or phase numbers in code comments, test names or commit messages.

## Open questions

None at planning time. The fresh-clone end-to-end test (`261014-1530-fresh-clone-e2e-test`) asserts this behaviour
once this plan lands.
