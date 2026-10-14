# Phase 1: Runtime budget test

## Goal

A test that measures what the model pays for the runtime profile separately from what travels over the wire, and
fails when either grows past a budget set at today's size. It blocks growth only; it does not force any reduction.

## Context

- Model-visible part per tool (verified, see [research](research-client-model-context.md)): `name`, `description`,
  input schema. Server `instructions` are also model-visible (Claude Code system prompt, Codex namespace description).
- Not model-visible: `outputSchema`, `title`, `annotations`. They still count in wire bytes.
- `tools_size_test.go` already builds in-memory sessions per profile and logs sizes. `serializeToolsList`
  (`tools_list_digest_test.go:25`) is the canonical wire measurement (runtime: 16,532 bytes).
- Proxy equivalence is already enforced by `TestProxyRuntimeToolsMatchHubRuntimeProfile`, so a budget on the runtime
  profile also covers `skillhub mcp proxy`.

## Files

- Modify: `internal/delivery/mcpserver/tools_size_test.go` (only this file).

## Steps

1. Add a helper `modelVisibleBytes(tools []*mcp.Tool) (total int, perTool map[string]int)` that encodes, per tool,
   `struct{ Name string `json:"name"`; Description string `json:"description"`; InputSchema any `json:"input_schema"` }`
   with a `json.Encoder` and `SetEscapeHTML(false)` (clients send `<id>` as-is, Go's default would count `<`),
   trimming the trailing newline. This is the Claude API shape, the largest of the verified clients.
2. Add `TestRuntimeToolListStaysWithinBudget`:
   - Build the runtime profile with `New(newMCPWorkspace(t), nil, ProfileRuntime)` and connect an in-memory client
     (reuse the pattern of `listProfileTools`, but keep the client session so `session.InitializeResult()` is
     available).
   - `instructionsBytes := len(session.InitializeResult().Instructions)` (0 today).
   - `toolsModel, perTool := modelVisibleBytes(listed.Tools)`; `wire, _ := serializeToolsList(t, listed.Tools)`.
   - Constants with a comment stating the rule: "Budgets are the measured size rounded up to the next 100 bytes. Raise
     one only on purpose and say why in the commit body; lower it when a change makes the list smaller."
     - `runtimeModelVisibleBudget = 5900` (measured 5,889 with Python's encoder; confirm the Go figure first and round
       up to the next 100 if it differs).
     - `runtimeWireBudget = 16600` (measured 16,532).
   - Assert `toolsModel + instructionsBytes <= runtimeModelVisibleBudget + instructionsAllowance`, where
     `instructionsAllowance` is 0 while the server sends no instructions. If the server-instructions code has landed
     before this phase runs, use its exported cap constant instead of a literal, so the 2048 cap stays owned by
     `instructions_test.go` (see open question 3). Also assert `toolsModel <= runtimeModelVisibleBudget` on its own,
     so instruction text cannot hide tool growth.
   - Assert `wire <= runtimeWireBudget`.
   - On failure, print a table: tool, description bytes, input schema bytes, output schema bytes, model-visible bytes,
     and the approximate tokens (bytes / 4), so the reviewer sees which part grew.
3. Optional (open question 4): the same assertions for `ProfileCuration` with `curationModelVisibleBudget = 18200` and
   `curationWireBudget = 101400`, in a table-driven loop over profiles.
4. Leave `TestToolsListSize` as the logging report, but make it log the model-visible bytes too (reuse the helper), so
   one run shows both views.

## Validation

```bash
go test ./internal/delivery/mcpserver -run 'TestRuntimeToolListStaysWithinBudget|TestToolsListSize' -count=1 -v
go test ./internal/delivery/mcpserver -count=1
make check
```

Negative check before committing: temporarily lengthen the `skill_get` description by 200 characters and confirm the
test fails and names `skill_get`; revert.

## Risks

- Budgets re-pinned without thought: mitigated by the failure message and the commit-body rule above.
- Encoding differences make the Go figure differ from the research figure: the step says to confirm and round from
  the Go measurement.

## Rollback

Revert the commit; no production code changes.
