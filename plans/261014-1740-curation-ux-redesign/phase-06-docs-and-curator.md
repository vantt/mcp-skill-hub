# Phase 6: Docs, help and curator skill

## Goal

Docs, help text, the curator skill and the release note tell one story: changes are confirmed (y/N in a terminal,
`--yes` for agents) and committed to the hub automatically; skills are edited and tried through checkout and commit;
quick edits happen on the web; a person approves third-party and submitted content. Editing hub files directly is no
longer the official path.

## Decisions implemented

D8 (one consistent story across docs, the form and the curator skill), and the user-facing description of D1-D7 and
D9.

## Prerequisites

Phases 1-5 merged. Contract docs (`docs/contracts/*`, `schemas/*`) and the remote threat model were already updated
in their phases. This phase does not repeat them.

## Statements to change (from the scout, with line numbers at 4de8625; re-check them before editing)

| Topic | Where |
|-------|-------|
| Commit is the user's job | `docs/curating-skills.md:112, 344, 349, 377`; `docs/web-ui.md:6, 75`; `docs/design/07-storage-and-mutation-model.md:374, 432, 443, 784-809` (§17.2 "does not run automatic … commit", §17.3); `docs/design/05-curation-lifecycle.md:147, 211, 276, 557`; `docs/PRD.md:1541`; `docs/design/01-system-architecture.md:424`; `docs/command-reference.md:78` |
| `--yes` as the only confirm | `docs/command-reference.md:8, 37-66`; `docs/user-guide.md:19, 98, 108`; `docs/curating-skills.md:25, 35, 91, 140-143, 170, 181, 199, 286-291, 310-327`; `docs/sources-and-learning.md:12, 80, 238, 293`; `docs/web-ui.md:87`; `docs/design/02-agent-hub-protocol.md:77`; `docs/design/05-curation-lifecycle.md:628` |
| Direct edits allowed or required | `docs/design/07-storage-and-mutation-model.md:769` (§17.1); `docs/curating-skills.md:185-188`; `docs/web-ui.md:68-75` |
| `skill create` writes the hub | `docs/curating-skills.md:69, 72, 78-81, 183-203`; `docs/command-reference.md:38`; `docs/troubleshooting.md:219`; `docs/contracts/ux-fixtures.md:105` |
| `skill add <folder>` | `docs/curating-skills.md:136-153`; `docs/command-reference.md:37` |
| Approval only in the terminal | `docs/web-ui.md:6, 56-61` |
| Curator skill | `internal/systemskills/curator/SKILL.md:80, 90, 103-104, 158, 167, 174, 185-186, 205, 224, 248-249, 335, 377` |

## Content

- `docs/command-reference.md`: add rows for `skill checkout` and `skill commit`. Change the rows for `skill create`
  and `skill add`. Explain `--diff`, `--no-commit` and the TTY prompt once at the top. Remove every `--full-diff`
  mention (none exist today).
- `docs/curating-skills.md`: rewrite around four flows:
  1. edit and try (checkout → edit → try → commit);
  2. new skill (create in the project → try → commit);
  3. add from GitHub;
  4. review and approve (terminal chain, or the web Waiting for review page).
  Then add "From another machine" (remote commit), and "What gets committed and who gets it" (the scoped commit,
  `--no-commit`, push for other clones). Direct edits move to a short "Working inside the hub folder" note: possible,
  not official, commit them yourself; `status` shows a notice, not `FAILED`.
- `docs/web-ui.md`: the Files tab, Review and Commit, the Routing tab, Waiting for review, full review with Approve,
  and Checkout. The exposure note says Approve is reachable wherever the web UI listens.
- `docs/design/07-storage-and-mutation-model.md`: §17.1 (direct edits are not the managed path), §17.2-17.3 (every
  confirmed change is one scoped commit with hooks and signing, plus the refusal rule). `docs/design/05` and `01`:
  the matching approval-matrix rows. `docs/design/02-agent-hub-protocol.md:77`: the TTY prompt versus `--yes`.
  These files are partly Vietnamese, so keep the language of each section.
- `docs/user-guide.md` and `docs/troubleshooting.md`: the permission rules text (unchanged rules, new commands
  covered), "refused because files have uncommitted edits", and "commit failed: signing / hook".
- `internal/systemskills/curator/SKILL.md` (agent rules):
  - preview first, then `--yes` only after the user saw the preview (unchanged), and use `--diff` to show the diff;
  - edit skills with `skill checkout`, file tools in the project, then `skill commit`;
  - never edit hub files directly (now consistent with the docs);
  - new skills: write `.claude/skills/<id>/` then `skill commit`;
  - never approve; for third-party commits, stop and hand over to a person;
  - commits happen on apply, so the "Git commit: require an explicit request" row becomes "push: require an
    explicit request".
  Keep the explanatory style (why, what to ask, what to look at, an example per item), per the user's preference.
- `README.md` quick start, if it shows `skill create` or `skill add <dir>`.
- Release note entry (where the repo keeps them; check `docs/release-runbook.md`). It names the breaking changes:
  - `skill add <folder>`;
  - `skill create` target;
  - `--full-diff`;
  - auto-commit and the refusal for uncommitted edits;
  - the new web approval.

## Files

Only the documents listed above, `internal/delivery/cli/help.go` (if phases left wording gaps), and
`internal/systemskills/curator/SKILL.md`. No behaviour changes.

## Steps

1. Grep again for `skill create`, `skill add`, `--yes`, `git commit`, `git -C`, `full-diff`, `approve-content`,
   `Never edit` and `direct edit` across `docs/`, `README.md` and `internal/systemskills/`. Reconcile with the table.
2. Edit each document. Verify every command line by running it against a temp hub and project with isolated HOME
   and XDG.
3. Run the doc-command tests and the system skill tests.
4. Read the curator skill end to end as an agent would, and check that it never asks for something a hook or rule
   blocks.

## Validation

```bash
go test ./internal/delivery/mcpserver -run 'DocCommands|FreshClone' -count=1
go test ./internal/systemskills/... ./internal/app -run 'SystemSkill' -count=1
go test ./internal/delivery/cli -run 'Help|Docs' -count=1
make check
```

## Risks

- **Doc and code drift.** Every command line in the rewritten guides is run once against a temp hub.
- **Curator digest changes** may trigger a system-skill doctor notice in existing installs. That is expected after an
  update. Check `system_skill_doctor` tests.

## Rollback

Revert the docs commit.

## Effort

1 day.
