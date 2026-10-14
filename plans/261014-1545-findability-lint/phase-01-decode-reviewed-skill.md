---
phase: 1
title: "Decode the reviewed skill like the catalog does"
status: pending
effort: 2h
depends_on: []
---

# Phase 1: Decode the reviewed skill like the catalog does

## Context

`ReviewSkill` (internal/app/skill_review.go:146) calls
`resolverpkg.DecodeSkillDocument(id, skillMetaBytes)` on raw YAML. `DecodeSkillDocument` runs
`json.Unmarshal` on its input, so this always fails for `skill.meta.yaml` and `.meta/skill.yaml`. The
error is dropped, so `LintTargetSkill` never runs during review. Verified with a probe on
4198bb7: `invalid character 'a' in literal null (expecting 'u')`.

The draft case of self-resolution needs the reviewed skill decoded exactly as activation would
decode it. The catalog does that in `catalog.parseEntity` (internal/catalog/input.go:193-231):

- it decodes the YAML into a map;
- it derives `name` and `description` from SKILL.md (`DeriveSkillNameAndDesc`);
- it copies `aliases`, `topics`, `technologies` and `domain` from `routing` to the top level;
- it encodes the result with `jsonMarshalStable`.

## Requirements

- Review decodes the canonical skill (draft or active) into a `resolverpkg.Skill` through the same
  normalization the catalog build uses. Do not write a second YAML-to-JSON path.
- When decoding fails, review reports one warning instead of skipping silently: code
  `lint_skipped`, matching the code `workspaceLintWarnings` already uses.
- The same function must also decode skill files read from Git blobs: HEAD versions, and index
  versions from `CaptureGitIndexSnapshot`. Phase 4 uses these as the before-state and the staged
  after-state. Its input is therefore bytes, not a path.

## Files

- Modify `internal/catalog/input.go`: extract the skill branch of `parseEntity` into an exported
  `NormalizeSkillDocument(metaBytes, skillMD []byte, id string) ([]byte, error)`, which returns
  stable JSON. `parseEntity` calls it, so build output stays byte-identical.
- Modify `internal/app/skill_review.go`: read SKILL.md bytes, which `inspectCanonicalEntrypoint`
  already returns as `entrypointBytes`. Normalize, then decode, then call `LintTargetSkill`. On error,
  append the `lint_skipped` warning.
- Modify `internal/app/skill_review_test.go`: add tests.
- Golden files under `internal/delivery/web/testdata/golden/` and CLI review goldens: update only if
  the fixtures now gain real lint warnings, and review each diff.

## Steps

1. Write a failing test in `skill_review_test.go`. Use a temp workspace with one active skill with
   trigger "review a pull request" and a draft with the same trigger. Assert that
   `activation_readiness.warnings` contains `trigger_collision`.
2. Extract `NormalizeSkillDocument`. Keep `parseEntity` behaviour identical. The existing catalog
   tests, and catalog digests that depend on `JSON`, must not change.
3. Use it in `ReviewSkill`. Add the `lint_skipped` warning path, with a test that feeds unparsable meta.
4. Run the web and CLI review golden tests. Inspect and accept any diff with `-update`.

## Validation

```bash
go test ./internal/app -run 'Review' -count=1
go test ./internal/catalog -count=1
go test ./internal/delivery/web ./internal/delivery/cli -run 'Review' -count=1
make check
```

## Risks

- Changing `parseEntity` could change catalog digests. Mitigation: a pure extraction, plus the
  existing catalog tests and a before/after comparison of `BuildCatalogGeneration` output in the test.
- Review output gains warnings it was always meant to show. This is user-visible, so call it out in
  phase 4.

## Rollback

Revert the commit. Review goes back to silently skipping routing lint, and nothing else depends on it.
