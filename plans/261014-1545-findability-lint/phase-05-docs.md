---
phase: 5
title: "Document the new warnings and modes"
status: pending
effort: 1.5h
depends_on: [4]
---

# Phase 5: Document the new warnings and modes

## Context

- Lint codes are listed in one table: `docs/design/03-resolver-design.md`, "Metadata linting rules
  (warnings)", around lines 136-148. It is written in Vietnamese and states that validate and review
  run lint without failing.
- The curator guide `docs/curating-skills.md` covers:
  - review ("Review diagnostic facts", around line 215);
  - the pre-commit hook (`maintain validate --staged`, around lines 371-386);
  - `skillhub eval routing` ("Check routing quality", around line 409).
- `docs/command-reference.md:80` lists `maintain validate --staged`. The new `--full` flag belongs
  next to it.

## Requirements

- `03-resolver-design.md`, written in Vietnamese to match the table:
  - add rows for `example_resolves_elsewhere`, `counter_example_resolves_to_skill`,
    `example_taken_by` and `self_resolution_skipped`;
  - add a short note on the affected-examples rule: own cases, plus other skills' examples where the
    changed skill influences the decision (score at least the applicability floor);
  - describe the modes: changed by default, `--staged`, `--full`;
  - give the work bound, and the known gap above `CandidateLimit`.
- `docs/curating-skills.md`:
  - **Review section:** one paragraph with a trimmed real example. Review resolves the skill's
    examples against the active library as if the skill were active, and names the skill each one
    picked. It also lists the examples of other skills the change would take. Mention the "Routing
    lint" heading.
  - **Pre-commit section:** one sentence. The hook now also warns about routing changes in staged
    skills, and these warnings never block the commit.
  - **"Check routing quality":** one sentence. `maintain validate` checks the changed skills,
    `--full` checks all of them, and `eval routing` gives totals and thresholds.
- `docs/command-reference.md`: add one row for `skillhub maintain validate --full`.
- Follow `.claude/rules/documentation-management.md`. Read each doc before editing, and check every
  claim against the phase 3 and 4 tests and real CLI output.

## Files

- Modify `docs/design/03-resolver-design.md`
- Modify `docs/curating-skills.md`
- Modify `docs/command-reference.md`

## Steps

1. Produce real output with the built binary and `--workspace <tmp>`: review the steal case, a
   changed-mode validate, and `--full`. Never use the live hub.
2. Edit the design table and its notes.
3. Edit the three curating-skills passages and the command-reference row, using the trimmed real
   output.
4. Run the docs tests, which check that every documented command exists.

## Validation

```bash
go test ./internal/delivery/cli -run 'Docs' -count=1
make check
```

## Risks

Doc examples can drift from the real wording. Mitigation: copy them from step 1 output.

## Rollback

Revert the commit.
