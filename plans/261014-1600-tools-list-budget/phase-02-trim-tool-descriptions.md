# Phase 2: Trim runtime tool descriptions

## Gate

Start only after the server-instructions code (`plans/261014-1509-server-instructions-protocol`, phases 1 and 2) is
merged. Today the descriptions are one of the channels that carry `review_required`, `local.preflight` and the
skills-extension route; after that plan, the initialize `instructions` carry them (rows R3, R4, R7 of its phase 1).
Removing them from descriptions earlier would leave agents without the protocol.

## Goal

Each runtime description says only: when to call (trigger), what to send (input shape), what to call next
(follow-up). This is the tech-leads-club pattern from the lesson (`tech-leads-club@120b67676388:packages/mcp/src/tools/search-tool.ts#L7-L10`),
which cut descriptions from 719 to 385 tokens without a search-quality change.

## Proposed text

| Tool | Today (chars) | Proposed | Chars |
|------|------|----------|-------|
| `skill_resolve` | 361 | `Pick at most one skill for the current task. Send task.description in English, plus operation and a request_id. Then load the recommended skill with skill_get.` | 159 |
| `skill_get` | 345 | `Load one skill by skill_id: content, metadata and local.path (read-only skill folder; local.state_directory is writable). Use the id that skill_resolve recommended.` | 164 |
| `skill_feedback` | 131 | `Report how a resolved skill worked out: resolution_id from skill_resolve, a new event_id, and outcome.` | 102 |

Saving: about 412 model-visible bytes (~100 tokens). Dropped sentences and where they live afterwards:

- `review_required` / `skillhub skill review <id>`: instructions row R7, and the `skill_get` result itself
  (`internal/app/skill_bundle.go:156`, `ReviewCommand`).
- "Run local.preflight check before scripts; ask before setup": instructions rows R3 and the preflight row.
- Skills-extension route (`skills/get`, `resources/read`): instructions row R4, rendered only when the extension is
  advertised.
- "Feedback never mutates routing policy": server behaviour, not something the agent acts on.

Adjust the wording if the merged instructions text differs; the rule is that no sentence is dropped unless the merged
instructions or the tool result carry it. Keep the `title` fields as they are (not model-visible, used by UIs).

## Files

- Modify: `internal/delivery/mcpserver/resolver_tools.go` (`skill_resolve`, `skill_feedback` descriptions),
  `internal/delivery/mcpserver/skill_tools.go` (`skill_get` description, line ~157).
- Modify: `internal/delivery/mcpserver/tools_list_digest_test.go` (re-pin runtime, all and curation digests: the
  `all` profile includes these tools; curation is unchanged unless it lists them).
- Modify: `internal/delivery/mcpserver/tools_size_test.go` (lower the runtime model-visible and wire budgets to the
  new measured size rounded up to the next 100).

## Steps

1. Diff the merged instructions text against the dropped sentences above; keep any sentence it does not cover.
2. Replace the three descriptions.
3. Run the digest test, copy the new values, and lower the budgets.
4. Behaviour checks (no server logic changed, so these confirm agents still route the same way):
   - Routing eval on a temp copy of a hub workspace, before and after:
     `cp -r <fixture-or-hub-copy> "$TMP/ws" && go run ./cmd/skillhub eval routing --workspace "$TMP/ws" --json`.
     Metrics must be identical (the resolver does not read tool descriptions; a difference means something else
     changed). Never point it at `/home/vantt/skill-hub` directly.
   - Manual smoke, one session each in Claude Code and Codex CLI, on a temp project connected to a temp hub
     (isolated `HOME`/`CLAUDE_CONFIG_DIR`/`CODEX_HOME`): give a task that matches a fixture skill and check in the
     transcript that the agent calls `skill_resolve` with `task.description` in English, then `skill_get` with the
     recommended id. Record the result in the PR description.

## Validation

```bash
go test ./internal/delivery/mcpserver -run 'TestRuntimeToolListStaysWithinBudget|TestToolsListSerializationIsStablePerProfile|TestProxyRuntimeToolsMatchHubRuntimeProfile' -count=1
go test ./internal/delivery/mcpserver -count=1
make check
```

## Risks

- An agent stops calling `skill_resolve` on its own: mitigated by the instructions channel and the smoke test.
- A client that ignores `instructions` loses the dropped sentences: accepted by the user in the instructions plan
  (its open question 1); not re-decided here.

## Rollback

Revert the commit; digests and budgets revert with it.
