# Manifest fields and routing-merge fix: implementation report

Status: completed (2026-10-04). Not committed.

## Outcome

- `skill.meta.yaml` accepts an optional `runtime` block (`requires.bins|env|platforms`, `setup.command|check`), `routing.examples` / `routing.counter_examples` (unique, non-empty, at most 10, at most 300 characters), and `quality.scripts_reviewed_digest` (`sha256:<64 hex>`). Both the canonical validator and the committed JSON schema enforce these rules, and unknown keys are rejected at every level.
- Routing edits no longer wipe data. `mergeRouting(existing, input)` starts from the stored routing map. `requirements`, `distinguish_from`, `supporting`, `equivalent_to`, and `boosts` are always preserved. The four core fields are written exactly as before. For examples and counter-examples, nil keeps the stored list and an explicit empty list removes the key.
- Request digests tell keep (nil), clear (`[]`), and set apart for the optional lists. Requests that do not use the new fields keep their historical digest, and a test locks that in.
- `UpdateInput.ScriptsReviewedDigest` is written to `quality.scripts_reviewed_digest`. Only the CLI exposes it (`skill edit --approve-scripts`); MCP inputs do not include it.
- `ReadRouting` now returns `Examples` / `CounterExamples`, and returns nil when they are absent. The CLI merge therefore passes stored lists through, and `SkillDetail.routing` gains them as an additive change.
- CLI: `--example` and `--counter-example` (repeatable) work on `create` and `edit`. `--approve-scripts` works on `edit` only and is format-checked at parse time. All of them are rejected on `list`, `show`, `activate`, `deprecate`, `archive`, and `confirm`. Help text is updated.

## Files

The schema and code changes stayed inside the phase's file list: `schemas/skill-metadata.schema.json`, `internal/canonical/skill.go`, `internal/skill/lifecycle.go`, `internal/app/skill_review.go`, `internal/delivery/cli/skill.go`, and `internal/delivery/cli/help.go`.

New tests went into five test files: `internal/canonical/canonical_test.go`, `internal/skill/lifecycle_test.go`, `internal/delivery/cli/skill_test.go`, `internal/app/skill_review_test.go`, and `schemas/embed_test.go`. Two of these are not in the phase's list:
- `internal/app/skill_review_test.go` holds a small parse test for the new review-meta fields.
- `schemas/embed_test.go` holds the YAML fixture validated against the schema, which the phase's own validation step requires.

## Verification

- `go test ./internal/canonical/ ./internal/skill/ ./internal/app/ ./internal/delivery/cli/ ./schemas/` passes.
- `make check` (vet, golangci-lint with 0 issues, and the full `go test ./...`) passes. There were no failures before or after the change.

## Notes

- The JSON schema's `maxLength` counts characters, while the canonical validator limits `setup.command` / `setup.check` to 1024 bytes. Canonical validation is the stricter, authoritative check.
- A `bins[].version` must be a quoted YAML string, matching the schema's `type: string`. If it is left unquoted (`version: 18`), the error message tells the user to quote it.
- Phase 4 will add printing of the digest to approve in `skill review`. Until then, `--approve-scripts` only records whatever digest the user supplies.
- `ak plan phase close` failed with `planstore: plan not found`, for both the bare and the project-prefixed plan ID. The phase file is marked completed, its checklist is checked, and `ak plan status` reports 1/13 done. The Status cell in plan.md still reads "Pending" because the rules say not to edit it directly.
