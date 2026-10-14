---
phase: 6
title: "Apply the decision"
label: needs-user-go
status: pending
priority: P2
effort: "4-8h depending on the decision"
dependencies: [5]
---

# Phase 6: Apply the decision to distill-lab and the lesson schema

Blueprint only. Execute after Phase 5 is recorded and the user says go. If every field is kept as it is, this
phase reduces to step 7 (docs and D7 note) and no code changes.

## Files (ownership: this phase only; one worktree, one agent)

| Change | Files |
|---|---|
| Skill text and script (editable source) | `.agents/skills/distill-lab/SKILL.md`, `.agents/skills/distill-lab/scripts/distill.mjs` |
| Embed mirror | `internal/systemskills/distill-lab/` (copy only: `rm -r internal/systemskills/distill-lab && cp -R .agents/skills/distill-lab internal/systemskills/distill-lab`, `AGENTS.md:24-25`) |
| Version | `internal/systemskills/bundles.go:18` (`DistillSkillVersion`), SKILL.md frontmatter `version`, `internal/systemskills/embed_test.go:342` literal |
| Shipped digests | `internal/systemskills/known-digests.go` (SKILL.md and `scripts/distill.mjs` bytes as shipped in the latest tag that holds them) |
| Wrapper (only if the description changes) | `internal/hostintegration/testdata/wrappers/distill-lab.golden`, `internal/hostintegration/known-wrappers.go` |
| Go model and readers | `internal/distill/distill_model.go`, `internal/app/curation_home.go` (only if the score changes), `internal/migration/v4.go:337-357` |
| Contract | `schemas/distill.schema.json`; `schemas/curation-home-v1.schema.json` only if decision 4 changes it |
| Web | `web/src/api/types.ts:741-766`, `web/src/screens/skill-detail/DistillTab.tsx`, `DistillTab.test.tsx` |
| Fixtures and goldens | `internal/migration/testdata/{test-audit,phase0}-distill.yaml`, `internal/systemskills/testdata/distill/**` |
| Docs | `docs/design/07-storage-and-mutation-model.md` (schema row), release note; `docs/sources-and-learning.md` only if review steps change |

## Steps

1. **Version.** A drop or a required→optional change alters the file format: bump distill-lab to 0.2.0 (minor). A
   wording-only change is a patch. Update the constant, the frontmatter and the pinned test literal together (the
   curator precedent is `600fb9b`).
2. **Script.** For a dropped field, `format` removes it and prints one `WARN: removed deprecated field <name>
   from N lessons` line. `check` warns about it but does not fail. Any other unknown field still fails. For a
   field made optional, remove only the required check. For score-weight changes, update `impactOf`/`finalScore`/sort and
   define where unscored lessons sort (proposal: after scored ones, then layer, then key).
3. **Skill text.** Update the schema example, the step-5 recording rules, the four-weights section, the
   description (only if it names the field) and the experiment log, which closes with the scorecard result.
4. **Go model.** Mirror the script: a dropped field leaves the struct and a deprecated key is ignored on read;
   required→optional uses `omitempty` and drops the check. Readers that skip invalid files
   (`curation_home.go:367`) then accept both old and new files.
5. **Shipped digests.** For each changed bundled file:
   `git show v0.6.4:internal/systemskills/distill-lab/<path> | sha256sum` (or the latest tag that holds it),
   added under `knownDigests[DistillSkillID]["<path>"]`; run with `SKILLHUB_REQUIRE_TAGS=1`.
6. **Fixtures.** Regenerate the goldens with the new script. Keep one fixture with the old field to prove that
   `format` strips it and that Go reads it (one owner test each, per `AGENTS.md:51-58`).
7. **Docs and decision record.** Update the design schema row and add a release note naming the format change and
   the old-binary limit. The lead updates D7's open note in the backlog.
8. **Live hub migration (user).** The binary's `doctor --fix` upgrades the hub's distill-lab copy. The user then
   runs `node .agents/skills/distill-lab/scripts/distill.mjs format skills/default/test-audit/.meta/distill.yaml`
   in their hub, reviews `git diff` and commits. Agents never run it on the live hub. An agent may rehearse it on a
   sandbox clone (Phase 3 sandbox recipe) and attach the diff for the user.

Commit messages describe the behaviour ("feat(distill-lab): drop the contrast field from lessons"), never a plan,
phase or audit label; test names likewise.

## Validation

```bash
make check
SKILLHUB_REQUIRE_TAGS=1 SKILLHUB_REQUIRE_NODE=1 go test -count=1 ./internal/systemskills/... ./internal/hostintegration/...
go test -count=1 ./internal/distill/... ./internal/migration/... ./internal/app/... ./internal/delivery/...
make web-check
```

Sandbox rehearsal: on a clone of the live hub, `format` on the old file gives the expected diff (only the dropped
lines and the header line). `skillhub maintain validate --workspace "$S/hub"` passes, and `skillhub status`
run inside `$S/hub` (confirm the flag form with `skillhub status --help` first) still counts the candidate
lessons.

## Risks

| Risk | L×I | Mitigation |
|---|---|---|
| Older binaries or older installed scripts reject new files | Med×Med | Decision 5; release note; `doctor --fix` upgrades the script |
| Missing shipped digest makes doctor treat the old script as user content | Med×High | Step 5 with `SKILLHUB_REQUIRE_TAGS=1` |
| Public contract `pending_high_value_insights` changes silently | Low×High | Decision 4 recorded first; schema test `internal/delivery/cli/review_lessons_test.go:84` |
| Golden churn hides a real behaviour change | Med×Med | Review the golden diffs line by line; only the decided field may change |

## Rollback

Revert the commits (one per concern). Users who already ran `format` can restore the field from Git history of
their hub (`git checkout <prev> -- <file>`). The old script still reads the old file because nothing else changed.
