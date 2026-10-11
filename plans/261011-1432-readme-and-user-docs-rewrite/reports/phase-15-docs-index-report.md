# Docs index report

Status: DONE

## Changes

- Created `docs/README.md` (40 lines): Start here, Guides, Reference, For maintainers.
- Each description is taken from the target page's intro (contracts, matrix keys, distillery `distill.yaml`,
  references notes were read for their lines).
- Design records (`PRD.md`, `design/`, `use-cases/`, `distillery/`, `brainstorm/`, `plans/`, `references/`)
  appear only under "For maintainers", with a note that most are in Vietnamese.

## Verification

- Every link target checked with `test -e` from `docs/`: all 24 resolve.
- Inbound links that earlier pages flagged as pending now resolve: `README.md` ("All docs") and
  `CONTRIBUTING.md` ("docs index").

## Notes

- `user-guide.md`, `curating-skills.md` and `release-runbook.md` are being rewritten in parallel; their
  descriptions follow those phases' stated goals ("Using Skill Hub", curating without sources, release mechanics)
  and should still match after merge.
