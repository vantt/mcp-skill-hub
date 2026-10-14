---
phase: 4
title: "Document the new warnings"
status: pending
effort: 1h
depends_on: [3]
---

# Phase 4: Document the new warnings

## Context

Lint codes are listed in one table: `docs/design/03-resolver-design.md`, "Metadata linting rules
(warnings)", around lines 136-148. It is written in Vietnamese and states that `skillhub validate` and
`skillhub skill review` run lint without failing validation.

The curator-facing guide `docs/curating-skills.md` covers review ("Review diagnostic facts", around
line 215) and `skillhub eval routing` ("Check routing quality", around line 409). No other doc lists
lint codes. `docs/command-reference.md` needs no change because no command or flag changes.

## Requirements

- Add the three new codes to the table in `03-resolver-design.md`, in Vietnamese to match the
  table, with their trigger condition and fix. Extend the cost note under the table with the
  self-resolution bound and `self_resolution_skipped`.
- In `docs/curating-skills.md`:
  - Review section: one short paragraph with a trimmed example. Review now resolves each example
    against the active library, as if the skill were active, and names the skill it picked, with the
    fix. Also show the "Routing lint" heading.
  - "Check routing quality": one sentence saying validate and review run the same leave-one-out
    check per skill as warnings, and that `eval routing` gives the totals and thresholds.
- Follow `.claude/rules/documentation-management.md`. Read each doc before editing. Check every
  claim against the phase 3 tests and real CLI output from a temp workspace.

## Files

- Modify `docs/design/03-resolver-design.md`
- Modify `docs/curating-skills.md`

## Steps

1. Produce real output in a temp workspace with the built binary and `--workspace <tmp>`. Never
   use the live hub.
2. Edit the table and the cost note.
3. Edit the two curating-skills sections, using the real output trimmed.
4. Run the docs tests, which check documented commands.

## Validation

```bash
go test ./internal/delivery/cli -run 'Docs' -count=1
make check
```

## Risks

Doc examples can drift from the real wording. Mitigation: copy them from step 1 output.

## Rollback

Revert the commit.
