# Scout notes: lesson fields of distill-lab (M6)

Date 2026-10-13, HEAD `5b379e1`. Every number below comes from a read-only command run on
`/home/vantt/skill-hub/skills/default/test-audit/.meta/distill.yaml` (the "live file") with the js-yaml copy
vendored in `internal/systemskills/distill-lab/scripts/lib/js-yaml.mjs`. The ad-hoc commands lived in the session
scratchpad; Phase 1 replaces them with a tested tool and must reproduce these numbers.

## 1. What R/E/F is

The repo does define it, but in the predecessor skill, not in distill-lab:

- `docs/references/skills/distill/references/extract-rules.md:107-122` ("Impact scoring, Score column `R# E# F#`"):
  **R = Reach** (1 narrow component, 2 subsystem, 3 cross-cutting), **E = Evidence** (1 one source, 2 several
  sources or measured, 3 independent convergence or falsified by data), **F = Effort to port** (1 doc or small
  script, 2 component, 3 subsystem). Priority is `R×E/F`, derived and never stored. Also used in
  `docs/references/skills/distill/SKILL.md:89`, `references/consult-protocol.md:42`,
  `references/deep-dive-protocol.md:48,57`. Git: added in `2610bab`, reshaped in `7b2c9cb` (2026-10-08).
- distill-lab, created the same day (`5404786`), replaced R/E/F with the `score` block: `relevance` 0-3 (not reach),
  `facts` a-e giving `impact` 0-5, `evidence` 1-3, `effort` 1-3, `why`, plus the derived `final_score =
  relevance × impact × evidence / effort` (`internal/systemskills/distill-lab/SKILL.md:228-289`).
- D7 ("`contrast` and R/E/F scores stay experimental until phase 0's scorecard decides",
  `archive/plans/261008-1433-simplify-hub-model/plan.md:75`, first written in `3f8a9a8`) and the Phase 3 spec
  ("Scorecard Fields: Relevance (0-3), Facts, Impact (0-5), Evidence (1-3), Effort (1-3), Why, FinalScore",
  `archive/plans/261008-1433-simplify-hub-model/phase-03-minimal-distill.md:66`) use "R/E/F" as the name of that
  `score` block. Conclusion: in M6, R/E/F means `score.{relevance, facts, impact, evidence, effort, why}` plus
  `final_score`. The "R" changed meaning (Reach → Relevance); the user confirms this mapping (Phase 5, decision 1).

## 2. Real data (live file == both fixtures)

`sha256 048ab876…7675` for the live file, `internal/migration/testdata/test-audit-distill.yaml` and
`internal/migration/testdata/phase0-distill.yaml`: the three files are byte-identical. Header still names
`distill.py` (the file was last written by the Python script; `distill.mjs format` would rewrite line 1).

- 42 lessons, top keys `goal, cursors, coverage, lessons`. Every lesson has exactly
  `key, layer, what, notable, where, contrast, score, final_score, also_fits, decision` (no `status`, no `found_by`).
- `decision.state`: candidate 39, planned 3, ported 0, rejected 0. The planned ones are
  `ratchet-over-doctrine`, `mock-isolation-must-be-declared`, `ban-ships-with-replacement` (priority 4, 6, 9),
  each with `at: 96370cf438dc`.
- The 3 planned decisions already exist in the first commit of the file (`12c7925`, 29 lessons, older impact
  scale) and survive `bb19027` (42 lessons) and `0f88983` (impact rescored as counted facts). The decisions were
  taken with scores in the same file, so any score/decision association is partly circular.
- `contrast`: new 19, extends 12, already-covered 8, contradicts 3. Shannon entropy 1.762 bits (max 2).
- `layer`: content 23, enforcement 5, craft 4, validation 3, ecosystem 3, history 2, wording 2.
- `score.relevance`: 0→8, 1→6, 2→15, 3→13. `impact`: 0→24, 2→5, 3→4, 4→3, 5→6. `evidence`: 1→30, 2→8, 3→4.
  `effort`: 1→30, 2→10, 3→2. `final_score == 0`: 24 of 42.
- Every lesson with impact > 0 (18) has fact `b`; all 8 `already-covered` and 14 of 19 `new` lessons have impact 0.
- `where` entries per lesson: 1→22, 2→12, 3→5, 4→2, 8→1. `also_fits` non-empty: 17 (hub 11).
- Words: `what` min/median/max 10/30/59; `notable` 5/28/126; `score.why` 12/17/44.

## 3. Preliminary information measures (to be reproduced by Phase 1)

- `notable` vs `what`: word-set Jaccard median 0.09 (max 0.16); share of `notable` content words absent from
  `what` median 0.91 (min 0.75); gzip of all `what` 3566 B, all `notable` 3758 B, both 6857 B, so `notable`
  adds 3291 B. Most frequent `notable` trigrams occur in at most 3 lessons ("for the goal", "already in the").
- Spearman over 42 lessons: impact~final_score 0.98, relevance~final_score 0.49, evidence~final_score 0.28,
  effort~final_score −0.12, relevance~impact 0.47.
- Planned lessons by `final_score` rank 4, 6, 9. The chance that 3 lessons drawn at random from 42 all land in the
  top 9 is C(9,3)/C(42,3) = 0.0073; this is not evidence for the score (see circularity above).

## 4. Where each field is written, read and enforced

| Surface | File:line | notable | contrast | score / final_score |
|---|---|---|---|---|
| Skill text (authority) | `internal/systemskills/distill-lab/SKILL.md` | 333, 473-475 (rule), 9 (description) | 185-188, 252-254 (fact b defined by it), 337 | 228-289, 338, 542 |
| Script validation | `internal/systemskills/distill-lab/scripts/distill.mjs` | 765-767 required | 37, 775-777 required | 39-46, 778-795 required; unknown fields rejected 761-762 |
| Script derived values | same | — | — | `impactOf` 665-668, `finalScore` 670-676, sort 689-700, writer 1103-1113, `dropDerived` 1962-1971, stale warning 1988-1991 |
| Script `list` output | same 2018-2020 | — | printed column | priority number and order |
| Go model | `internal/distill/distill_model.go` | 73, 195-197 required | 31, 75, 224-226 required | 76-77, 85-92, 102-117, 228-253 required |
| Go readers | `internal/app/curation_home.go:366-377`, `internal/app/skill_sources.go:162`, `internal/delivery/web/routes_distill.go:40-50` | via `LoadDocument` (a file that fails validation is skipped or errors) | same | `curation_home.go:375` counts `Relevance >= 3 && Impact >= 4` as "high value" → public JSON `pending_high_value_insights` (`schemas/curation-home-v1.schema.json:100,104`), Home text `curation_home.go:639-640` |
| Migration | `internal/migration/v4.go:337-357` | written from legacy insight | hard-coded `extends` | hard-coded score |
| JSON schema | `schemas/distill.schema.json` (lesson `required` at line 94) | required | required | required (`score` sub-fields required at 125) |
| Web UI | `web/src/api/types.ts:741-766`, `web/src/screens/skill-detail/DistillTab.tsx:212-235`, `DistillTab.test.tsx:63-67` | shown if present | shown if present | relevance/impact/evidence shown |
| Tests on fixtures | `internal/distill/distill_model_test.go:13`, `internal/migration/v4_test.go:35`, `internal/delivery/cli/review_lessons_test.go:25`, `internal/delivery/web/routes_distill_test.go:34`, `internal/delivery/mcpserver/hub_status_lessons_test.go:16`, `internal/systemskills/distill_script_test.go:36` | fixture content | fixture content, `list` goldens `internal/systemskills/testdata/distill/list/*.txt` | goldens under `internal/systemskills/testdata/distill/` |
| Docs | `docs/design/07-storage-and-mutation-model.md:1034` (schema row only); `docs/sources-and-learning.md:194-217` (review flow, no field names) | — | — | — |

Contradiction to resolve: D7 and the Phase 3 acceptance (`phase-03-minimal-distill.md:195`,
`prompt-wave2-C.md:68-69`) say `contrast` and R/E/F are optional, but the script, the Go model and the JSON schema
all require them. Today they are schema in practice.

## 5. Cost of keeping versus dropping each field

| Field | Keep (required, as today) | Keep optional (what D7 promised) | Drop |
|---|---|---|---|
| `notable` | No code change. The agent writes about 28 words per lesson; the user reads them on the Distill tab. | Remove the required checks (`distill.mjs:765-767`, `distill_model.go:195-197`, schema line 94). Readers already tolerate absence (`DistillTab.tsx:212`). | Also remove it from `LESSON_ORDER` (`distill.mjs:44`), the writer (`distill.mjs:1105`), the Go struct, `types.ts:760` and the tab; rewrite SKILL.md step 5 (473-475), the schema example (333) and the description (9). Existing files: `format` must strip the field (it would otherwise fail on "unknown field", `distill.mjs:761-762`). Loses the only "why it matters" prose. |
| `contrast` | No code change. | As for notable, plus fact `b` must be redefined without it (`SKILL.md:252-254`). | As for notable, plus the `list` column (`distill.mjs:2020`) and its goldens, the v4 migration constant (`v4.go:343`), the tab. Loses the only marker that separates "already covered" (8 lessons) from "new but outside the goal" (14 lessons); both have impact 0. |
| R/E/F (`score`, `final_score`) | No code change. The agent fills 5 values and a `why` per lesson. | Sorting and `final_score` must handle missing scores (rule needed: unscored last?). `curation_home.go:375` must handle zero values. | Remove `impactOf`, `finalScore`, sort-by-score (`distill.mjs:665-700`), `dropDerived`, the stale warning, the Go `Score` type and `FinalScore`, the schema block, the tab row, about 60 lines of SKILL.md. `list` order becomes layer then key. `pending_high_value_insights` is a public contract field (`schemas/curation-home-v1.schema.json:100`) and needs a new rule or a documented constant 0. Existing files: `format` strips `score` and `final_score`. |
| One score component (for example `effort`) | — | — | Drop it from `SCORE_RANGES`/`SCORE_ORDER`, the formula, the Go struct and schema; `final_score` values change for every lesson and the file is reordered once. |

Data-migration consequence of any drop: only one real file is known (the live test-audit file, plus its two
identical fixtures). New readers must tolerate the old field until `format` rewrites the file. Older binaries
(v0.6.x) and older installed scripts still require the field, so a file written by the new script fails validation
there. `curation_home.go:367` skips such a file silently, and the web route returns an error. The follow-up must
decide between one release with a release note and two steps (tolerant readers first); see Phase 6.

## 6. Release and bundle rules that a follow-up change must respect

- Editable source `.agents/skills/distill-lab/`, mirrored byte for byte to `internal/systemskills/distill-lab/`
  (`AGENTS.md:19-25`, `TestDistillLabMirrorEqualsSource` `internal/systemskills/embed_test.go:200`).
- Version: `DistillSkillVersion = "0.1.0"` (`internal/systemskills/bundles.go:18`) must equal the frontmatter
  `version` (`TestDistillLabFrontmatterDeclaresVersion`, `embed_test.go:250-270`); `embed_test.go:342` also pins
  the literal `"0.1.0"`. Curator precedent: commit `600fb9b` bumped `internal/systemskills/embed.go:14` together
  with both SKILL.md copies.
- distill-lab files shipped in tags v0.6.0..v0.6.4 (`git ls-tree v0.6.4 -- internal/systemskills/distill-lab`), so
  any changed file needs the shipped digest added to `knownDigests[DistillSkillID][<path>]`
  (`internal/systemskills/known-digests.go:21-26`, `TestKnownDigestsCoverReleasedBundles`,
  `known_digests_test.go:27`, enforced with `SKILLHUB_REQUIRE_TAGS=1`).
- A changed `description` changes the Claude wrapper render: update
  `internal/hostintegration/testdata/wrappers/distill-lab.golden` and add the shipped golden digest to
  `internal/hostintegration/known-wrappers.go` (rule at lines 8-16).
