# Phase 4 (optional): Drop redundant length bounds on enum fields

## Goal

`applyCommonConstraints` (`schema_shaping.go:86`) adds `maxLength: 2048` to every string property, and
`withPropertyEnums` (`schema_shaping.go:154`) later adds an `enum`. A string limited to a fixed list cannot exceed the
longest listed value, so the `maxLength` adds nothing. Runtime: 4 fields of `skill_feedback` (`outcome`,
`reason_code`, `utility`, `basis`), 68 bytes. Curation: 5 fields, 85 bytes. No contract change: the derived schemas
of these tools are not a committed schema, and validation is at least as strict as before.

Smallest phase by saving; do it only if phases 1-3 leave time, or fold it into phase 2's commit when that one re-pins
digests anyway.

## Files

- Modify: `internal/delivery/mcpserver/schema_shaping.go` — after `options.shapeInput` runs in
  `deriveToolSchemas`, walk the input schema and clear `MaxLength` on any node with a non-empty `Enum`. Do not touch
  the committed resolver schemas.
- Modify: `internal/delivery/mcpserver/tools_list_digest_test.go`, `tools_size_test.go` (re-pin, lower budgets).
- Check: `server_test.go` curation-telemetry assertions (`"maximum":10000`, enum values) still hold.

## Steps

1. Add the pass with a one-line comment stating the invariant ("an enum already bounds the length").
2. Re-pin digests and lower budgets.
3. Confirm an out-of-enum `outcome` is still rejected (existing feedback tests, or one added case).

## Validation

```bash
go test ./internal/delivery/mcpserver -run 'Feedback|TestToolsListSerializationIsStablePerProfile|TestRuntimeToolListStaysWithinBudget' -count=1
go test ./internal/delivery/mcpserver -count=1
make check
```

## Risks

Negligible. A client that relied on `maxLength` for UI hints loses it on enum fields only.

## Rollback

Revert the commit.
