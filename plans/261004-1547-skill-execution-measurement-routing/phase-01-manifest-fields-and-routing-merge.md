---
phase: 1
title: "Manifest fields and routing-merge fix"
status: completed
priority: P1
effort: 6h
dependencies: []
---

# Phase 1: Manifest fields and routing-merge fix

## Context

- [plan.md](./plan.md) D3, D7; design `docs/design/03-resolver-design.md` §3 (routing metadata), `schemas/skill-metadata.schema.json`.
- Validation lives in `internal/canonical/skill.go` (`validateSkillMetadata`, allowed keys at `:113`, routing keys at `:247`, `validateQuality` at `:566`). The JSON schema is a published contract checked by `schemas/embed_test.go:175`.
- Routing writes go through `skill.Manager.PreviewUpdate` (`internal/skill/lifecycle.go:235`) and `routingDocument` (`:630`), which currently **replaces** the routing map and drops `requirements`, `distinguish_from`, `supporting`, `equivalent_to`, `boosts`.

## Requirements

1. New optional top-level block:
   ```yaml
   runtime:
     requires:
       bins:                     # string or {name, version}
         - python3
         - {name: node, version: ">=18"}
       env: [OPENAI_API_KEY]     # names only; values are never stored
       platforms: [linux, darwin]  # GOOS values: linux, darwin, windows, freebsd
     setup:
       command: "pip install -r requirements.txt"
       check: "python3 scripts/check_env.py"
   ```
   Validation: `bins[].name` matches `^[A-Za-z0-9._+-]{1,64}$`; `version` matches `^(>=|>|<=|<|=)?\s*\d+(\.\d+){0,2}$`; env names match `^[A-Za-z_][A-Za-z0-9_]{0,127}$`; platforms from the enum; `command`/`check` non-empty single-line strings ≤ 1024 bytes; unknown keys rejected.
2. New `routing.examples` and `routing.counter_examples`: lists of unique non-empty strings, each ≤ 300 characters, at most 10 per list. Not required for activation (lint warns instead, Phase 11).
3. New `quality.scripts_reviewed_digest`: `^sha256:[0-9a-f]{64}$`.
4. Fix the routing merge: `PreviewUpdate` must start from the existing routing map and overwrite only the keys supplied. For slices, nil means "not supplied" (keep) and an explicit empty slice clears. `operations`, `triggers`, `not_for`, `min_scope` keep their current semantics when supplied.
5. `skill.RoutingInput` gains `Examples` and `CounterExamples` (`yaml:"examples,omitempty" json:"examples,omitempty"` etc.), so MCP `skill_create_preview` / `skill_update_preview`, CLI, and WebUI all carry them through the existing types.
6. `skill.UpdateInput` gains `ScriptsReviewedDigest *string`, written to `quality.scripts_reviewed_digest`. It is exposed on the CLI only (`skillhub skill edit <id> --approve-scripts <digest>`), not on MCP inputs, so agents cannot self-approve (D3). Phase 4 makes `skill review` print the digest to approve.
7. CLI flags on `skill create` and `skill edit`: repeatable `--example`, `--counter-example`; `skill edit` only: `--approve-scripts <digest>`.

## Files

Modify:
- `schemas/skill-metadata.schema.json` (add `runtime`, `routing.examples`, `routing.counter_examples`, `quality.scripts_reviewed_digest`)
- `internal/canonical/skill.go` (allowed keys, `validateRuntime`, routing list checks, quality field)
- `internal/canonical/canonical_test.go` (cases below)
- `internal/skill/lifecycle.go` (`RoutingInput`, `UpdateInput`, merge in `PreviewUpdate`, `routingDocument` replaced by `mergeRouting(existing map[string]any, input RoutingInput) map[string]any`, `normalizeRouting`, `updateRequestDigest` includes new fields)
- `internal/skill/lifecycle_test.go`
- `internal/app/skill_review.go` (`skillReviewMeta.Routing` gains `Examples`, `CounterExamples`; `Quality` gains `ScriptsReviewedDigest`)
- `internal/delivery/cli/skill.go` (flag parsing for the three flags; pass-through to `RoutingInput` / `UpdateInput`)
- `internal/delivery/cli/skill_test.go`
- `internal/delivery/cli/help.go` (`skill` usage text)

## Steps

1. Add the schema properties; keep `additionalProperties: false` everywhere.
2. In `canonical/skill.go`, add `"runtime"` to the top-level allowed set, `"examples","counter_examples"` to routing keys, `"scripts_reviewed_digest"` to quality keys; implement `validateRuntime(node *yaml.Node) error` following the existing `validateRequirements` style; reuse `stringSequence` and `isValidDigest`.
3. In `lifecycle.go`, implement `mergeRouting`: copy the existing `routing` map (or empty), then set each supplied key. Write a table test proving that an update supplying only `triggers` keeps `requirements`, `distinguish_from`, `supporting`, `equivalent_to`, `boosts`, `examples`, `counter_examples`.
4. Wire `ScriptsReviewedDigest` into `PreviewUpdate` next to the existing `Rationale` quality write (`lifecycle.go:257-261`).
5. Add the CLI flags and help text.

## Todo

- [x] Schema properties for `runtime`, `routing.examples`, `routing.counter_examples`, `quality.scripts_reviewed_digest`
- [x] Canonical validation (`validateRuntime`, example list bounds, reviewed-digest format)
- [x] `mergeRouting` replaces `routingDocument`; nil keeps, explicit empty clears optional lists
- [x] `RoutingInput.Examples` / `CounterExamples`, `UpdateInput.ScriptsReviewedDigest`, request digests distinguish keep/clear and stay stable for legacy requests
- [x] `skillReviewMeta` carries examples and reviewed digest
- [x] CLI `--example`, `--counter-example` (create, edit), `--approve-scripts` (edit only) plus help text
- [x] Focused tests in canonical, skill, app, cli, schemas
- [x] `make check` passes

## Implementation status

Completed 2026-10-04. Report: [reports/fullstack-developer-261004-1609-manifest-fields-routing-merge.md](./reports/fullstack-developer-261004-1609-manifest-fields-routing-merge.md).

## Tests and validation

- `go test ./internal/canonical/ ./internal/skill/ ./internal/app/ ./internal/delivery/cli/ ./schemas/`
- Canonical cases: valid full runtime block; bad bin name, bad version constraint, env with `=` or a value, unknown platform, multi-line command, unknown runtime key; >10 examples; duplicate examples; malformed reviewed digest.
- Lifecycle: merge preservation test above; explicit `[]` clears `examples`; idempotency digest differs when examples differ.
- Schema test: a fixture `skill.meta.yaml` using every new field validates against `skill-metadata.schema.json`.
- `make check`.

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Merge fix changes behavior that callers relied on (wipe-on-edit) | Low × Medium | Behavior was silent data loss; test documents the new contract; WebUI and MCP send routing without the new keys, which are now preserved. |
| Older binaries reject the new keys | Medium × Low | Documented minimum version (Phase 13); fields are optional. |

## Rollback

Revert the commit. Workspaces that already wrote new keys must remove them before running an older binary (`skillhub validate` names each line).
