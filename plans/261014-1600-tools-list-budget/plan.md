---
title: "A context budget for the runtime tool list"
description: "Measure the part of the runtime MCP surface that reaches the model, assert a budget that only blocks growth, then cut what the model actually pays for without changing the public resolver contract silently."
status: completed
priority: P2
effort: 5h
branch: (implementation gets its own worktree)
tags: [mcp-server, context-cost, tests, distillery]
blockedBy: []
blocks: []
created: 2026-10-14
---

# A context budget for the runtime tool list

Adopts distill lesson `tools-list-is-a-fixed-cost` (`docs/distillery/lessons/skill-over-mcp.yaml`), read together
with `markdown-or-json-tool-output` (`docs/distillery/lessons/ux.yaml`) and `structured-output-and-tool-annotations`.

## Outcome, constraints, non-goals

- **Outcome.** The runtime profile (`skill_resolve`, `skill_get`, `skill_feedback`, the tools `skillhub connect`
  installs into every project, and the tools the remote proxy offers) costs each agent session as little model context
  as practical, and a test stops that cost from growing without a deliberate budget change.
- **Acceptance criteria.**
  1. `go test ./internal/delivery/mcpserver -run TestRuntimeToolListStaysWithinBudget` asserts two numbers for the
     runtime profile: model-visible bytes (name + description + input schema per tool, plus server `instructions`
     once they exist) and total `tools/list` wire bytes. Both budgets equal today's size rounded up to the next 100
     bytes. Growing either fails with a message that names the tool and component that grew.
  2. Each reduction phase that lands lowers the budget to its new measured size (ratchet), keeps server-side
     validation of every request bound, and keeps `make check` green.
  3. No change to the committed resolver request or response contract (`schemas/skill-resolve-*-v1.schema.json`)
     without an explicit user decision (open question 1).
- **Constraints.** Implementation in its own git worktree. Tests use temp workspaces only (`newMCPWorkspace`), never
  the live hub (`/home/vantt/skill-hub`), the real `HOME` or real projects. No plan IDs or phase numbers in code
  comments, test names or commit messages. Conventional commits.
- **Non-goals.** Removing or merging runtime tools. Resolver behaviour changes. Removing `outputSchema` (see finding 1).
  Trimming the curation profile beyond what shared code changes give for free. Any other distill lesson. Marking the
  lesson adopted in the distillery (left to the lead).

## Verified findings that set the scope

Full evidence, sources and confidence: [research-client-model-context.md](research-client-model-context.md).

1. **outputSchema, title and annotations do not reach the model** in Claude Code 2.1.299 and Codex CLI 0.161.0 (wire
   capture, high confidence) or Gemini CLI (source, medium-high). Cursor is unknown (low). The model sees only
   `name`, `description` and the input schema, plus server `instructions`. About 64% of the runtime wire bytes
   (10,146 of 16,532) is outputSchema that no verified model reads. outputSchema stays: the Go SDK validates every
   result against it (`go-sdk@v1.8.0/mcp/server.go:460`) and `skill_resolve`'s embeds the committed response
   contract. Trimming it would save wire bytes only.
2. **What the model pays today: 5,889 bytes (~1.5k tokens)**, of which `skill_resolve` is 4,240 (input schema 3,822,
   description 363), `skill_feedback` 1,120, `skill_get` 529. Claude Code sends the input schema verbatim; Codex strips
   `$id`, `$schema` and length/count keywords itself (sends 5,126).
3. **Claude Code defers MCP tools behind tool search** by default: until loaded, a tool costs only its name. The
   protocol makes the agent load `skill_resolve` on almost every task, so its full definition is the cost that
   matters most; `skill_get` and `skill_feedback` follow when a skill is used.
4. **`skill_resolve`'s input schema is the committed public request contract**, advertised verbatim
   (`schema_shaping.go:193`) for both `skill_resolve` and the curation tool `routing_evaluate` (`insight_tools.go:81`),
   and asserted equal by `server_test.go:195-203`. The Go SDK validates arguments against the advertised schema
   (`go-sdk@v1.8.0/mcp/server.go:400-409`); `resolver.NormalizeRequest` (`internal/resolver/normalize.go:36`) re-checks
   many bounds in Go, but nothing shows it covers every keyword of the committed schema (16 `maxLength`, 14
   `minLength`, 7 `maxItems`, 5 `uniqueItems`, 1 `format`). Any slimmer advertised schema therefore needs explicit
   server-side validation against the full committed schema.
5. **The protocol text in tool descriptions duplicates the server-instructions plan.** `plans/261014-1509-server-instructions-protocol`
   is merged as a plan, but its code is not (no `instructions.go`, no `Instructions:` in `server.go` on this branch).
   Its runtime text covers `review_required`, `local.path`, `local.env` and preflight (rows R3, R4, R7) and is capped
   at 2048 characters by its own test. The `skill_get` result already names the review command
   (`internal/app/skill_bundle.go:156`).
6. **Telemetry hint, small sample.** 15 of 15 recorded resolves on the live hub sent no constraints and no facts
   (read-only check). Usage of `activation_context`, `context.execution` and `prior` is not recorded.
7. **Existing guards.** `toolsListDigests` (`tools_list_digest_test.go:16`) pins exact bytes and SHA-256 per profile,
   so any change already fails, but the fix is a mechanical re-pin. `TestToolsListSize` (`tools_size_test.go`) only
   logs sizes and asserts tool counts. Neither separates what the model sees from wire bytes, and neither turns
   growth into a decision.

## Phases

Ordered by model-context saving per effort. Phase 1 is unconditional; phases 2-4 are reductions.

| # | Phase | Saves (model-visible, every session that loads the tools) | Effort | Gate | Files owned |
|---|-------|------|--------|------|-------------|
| 1 | [Runtime budget test](phase-01-runtime-budget-test.md) | 0 (guard) | 1h | none | `internal/delivery/mcpserver/tools_size_test.go` |
| 2 | [Trim runtime tool descriptions](phase-02-trim-tool-descriptions.md) | ~410 bytes (~100 tokens) | 1h + smoke | server-instructions code merged | `resolver_tools.go`, `skill_tools.go`, `tools_list_digest_test.go`, `tools_size_test.go` |
| 3 | [Compact advertised resolver request schema](phase-03-compact-resolver-request-schema.md) | ~780 bytes (~195 tokens) in Claude Code; 0 in Codex | 2.5h | user decision, open question 1 | `schema_shaping.go`, `resolver_tools.go`, `insight_tools.go`, `server_test.go`, new `resolver_request_validation_test.go`, digest/size tests |
| 4 | [Drop redundant length bounds on enum fields](phase-04-drop-redundant-enum-bounds.md) | 68 bytes runtime, 85 curation | 0.5h | optional | `schema_shaping.go`, digest/size tests |

Phases 2-4 each touch `tools_list_digest_test.go` and `tools_size_test.go` (re-pin and ratchet), so they run one
after another, never in parallel. Total potential saving: ~1,260 of 5,889 model-visible bytes (~21%). The larger
lever, the optional request blocks (~2,400 bytes), is a contract change and stays an open question.

## Rejected reductions (with evidence)

| Idea | Why not |
|------|---------|
| Drop or shrink `outputSchema` (10,146 bytes) | Not sent to the model by any verified client; SDK output validation and the committed response contract depend on it. Wire-only gain. |
| Untyped results like skillport / pinkpixel (`markdown-or-json-tool-output`) | Same as above, and loses `structuredContent` validation; contradicts `structured-output-and-tool-annotations`, which the host already follows. |
| Drop annotations / title | Not sent to the model; Claude Code and Gemini CLI use annotations for permission and read-only handling. |

## Test matrix

| Level | What | Where |
|-------|------|-------|
| Unit (in-memory transport) | Runtime model-visible bytes and wire bytes stay within budget; failure names the component | `tools_size_test.go` |
| Unit | Digests re-pinned on purpose after each reduction | `tools_list_digest_test.go` |
| Unit | Proxy offers byte-identical runtime tools (existing) | `TestProxyRuntimeToolsMatchHubRuntimeProfile` |
| Unit (phase 3) | Every committed-schema bound is still rejected server-side after the advertised schema is compacted | new `resolver_request_validation_test.go` |
| Eval | `skillhub eval routing` on a temp copy of a workspace gives identical metrics before and after (server behaviour unchanged) | phases 2-3 validation |
| Manual smoke | One Claude Code and one Codex session still call `skill_resolve` with `task.description` in English, then `skill_get` | phases 2-3 validation |

Gate per phase: the narrowest test first, then `go test ./internal/delivery/mcpserver -count=1`, then `make check`
(`Makefile:60`: fmt-check, vet, lint, test).

## Risks

| Risk | L x I | Mitigation |
|------|-------|------------|
| A budget test that is re-pinned as casually as the digest | M x M | Separate test with a failure message that says how to raise the budget on purpose and asks for the reason in the commit body. |
| Shorter descriptions make agents skip `skill_resolve` or misuse inputs | L x H | Phase 2 keeps trigger, input shape and follow-up call; manual smoke in two clients; gated on instructions carrying the protocol. |
| Clients that ignore server `instructions` lose the protocol sentences removed from descriptions | M x M | Phase 2 runs only after the instructions code lands; open question 2 in the instructions plan already covers such clients. |
| Compact schema lets an over-long value past SDK validation | L x M | Phase 3 validates raw arguments against the full committed schema in the handler, with a test per bound. |
| Error text for invalid arguments changes | M x L | Phase 3 keeps the SDK error wording (`validating "arguments": ...`) and checks `error_characterization_test.go`. |

## Rollback

Each phase is one commit (phase 3 may be two: validation first, then the compact schema). Reverting a phase commit
restores the previous descriptions or schema and its pinned digests and budget together.

## Open questions

1. **Decided 2026-10-14: option (a), phase 3 in full.** The "advertised schema equals the contract" rule is
   dropped: its only real job was to make the SDK enforce every bound, which phase 3 keeps through explicit
   validation against the committed schema and a per-bound test. The bounds are generous (task description 4096
   characters, list items 512, at most 32 facts), so a model rarely hits one, and the saving recurs every session.
   Original question kept below for the record.

   **Advertised request schema vs committed contract (decides phase 3).** Today the advertised `skill_resolve`
   input schema equals `schemas/skill-resolve-request-v1.schema.json` byte for byte, and a test enforces it. Phase 3
   would advertise a projection (no `$id`, `$schema`, length/count/uniqueness keywords) while the server validates the
   full committed schema. Trade-off: ~780 bytes (~195 tokens) per session in Claude Code, zero in Codex (which already
   strips these), against "the discovery schema is the contract" being no longer literally true. Options: (a) approve
   phase 3; (b) skip it; (c) approve only dropping `$id` and `$schema` (133 bytes).
2. **Optional request blocks.** `activation_context`, `context` and `prior` cost ~2,400 model-visible bytes (~600
   tokens), about 40% of the runtime total. Telemetry shows no facts or constraints in 15 resolves, but does not
   record the other blocks. Removing them from the advertised schema is a public-contract change (and a v2 schema
   question). Should the hub first record which optional blocks agents send (telemetry change, separate plan), then
   decide?
3. **Instructions in the budget.** The instructions code has not landed. If it lands after phase 1, who adds the
   instructions length to the model-visible measurement: the instructions implementation or a follow-up here? Phase 1
   measures `InitializeResult().Instructions` already (empty today), so the follow-up is a budget bump only. The 2048
   cap stays owned by the instructions test; this plan does not repeat it.
4. **Curation budget.** Phase 1 offers an optional curation budget (model-visible 18,134 → 18,200; wire 101,354 →
   101,400). Include it, or keep curation unguarded beyond its digest?
5. **Cursor.** No wire-level evidence. Worth a manual check by someone with Cursor installed?
