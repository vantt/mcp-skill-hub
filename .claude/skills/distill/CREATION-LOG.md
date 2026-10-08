# CREATION-LOG — distill

## Source material

Distilled 2026-07-13 from two full operational runs of the manual process in
the forgent repo (full scan of beegog + repository-harness, then a same-day
94-commit delta scan), plus conventions ported from studied references:

- beegog `trigger-only-descriptions`, `skill-budgets-conventions` (body <200
  lines, one references/ level, headless section, red flags, handoff),
  `managed-block-markers`, `error-why-fix-refusals`, `zero-dep-vendored-helpers`,
  `unified-dispatcher-command-registry` (simplified).
- repository-harness/Beads: markdown-as-truth, derived state rebuilt from it.

## Validation performed

- `scripts/distill.mjs` end-to-end sandbox run (init idempotency, add ×3
  types, delta never/current/behind, seal all types, check exit codes,
  managed gitignore block).
- `check` against the real forgent learning area — caught 3 genuine
  shorthand-path defects in existing entries and the moved-entry case,
  which drove the `Status:`-aware skip rule.

## Known debt (before 1.0)

1. **Iron Law debt:** no RED pressure scenarios were run before writing this
   SKILL.md (bee's discipline requires failing tests first). The rules most
   at risk under pressure — "seal is mandatory", "never rename slug",
   "re-read file not diff" — should get pressure tests when
   a skill-testing harness exists in forgent.
2. paper/living-doc flows are sandbox-tested only, not yet dogfooded on a
   real source (candidates in intake: learn-harness-engineering course).
3. No automation for matrix drift beyond anchor checking.
4. Windows: script uses only portable Node APIs but has not been executed on
   Windows.

## 2026-07-13 — New-domain-discovery rule (extract-rules.md)

Added after the routing backfill exposed the gap: no explicit rule for a
concept fitting no taxonomy domain. Behavior derived from the skill's
existing pattern (surface as proposal, never silently skip, never edit
taxonomy unprompted); first exercised for real during the symphony scan
(integration-contract domain, human-approved). Same Iron Law debt as above —
no RED scenario was run first.

## 2026-07-13 — Consult mode (consult-protocol.md + SKILL.md routing/section)

Recall-first materials brief for designing a new host feature without known
keywords. Design rationale: completeness rides the backfill invariant
(taxonomy closed-set × every source covers every domain × entries never
escape domain headings), so exhaustive recall needs a walking PROTOCOL with
a coverage ledger, not search infrastructure — honors the no-search-infra
decision (2026-07-13). A `dump <domain|source>` helper command was designed
and deliberately NOT built (YAGNI; agent file reads suffice at current
scale) — revisit only on proven friction per the growth rule. Validation:
reasoning-tested against the session's own retrieval failures (vocabulary
mismatch); NOT yet dogfooded on a real feature design — first real consult
is the acceptance test. Same Iron Law debt: no RED pressure scenario.

## 2026-10-08 — 0.2: YAML lessons weighed against a goal (from distill-lab)

Brought back what the experimental `distill-lab` skill proved, translated from
"one hub skill" to "the host project", and kept standalone (node only;
js-yaml 4.3.2 vendored under `scripts/vendor/`, MIT).

- Layout: `distill.yaml` (goal, domains with definitions, sources with cursor,
  derived_from and coverage, intake) + `lessons/<primary-domain>.yaml`. One
  lesson per mechanism; several sources = several `where` (convergence), so
  the comparison matrix became optional. Decisions live on the lesson; the
  porting log and its R/E/F were retired (reach dropped; E/F map to evidence
  and effort). `move` and the forgent-only state migration were removed
  (they imported `forgent/state/porting-store`).
- Scoring: relevance 0-3, impact 0-5 = number of facts a-e that hold (a
  required), evidence 1-3 with an independence rule, effort 1-3. Lessons
  outside the goal, already covered, unscored or retired are recorded but not
  ranked. Decided with the human; impact widened on advice after test-audit
  showed 16/42 lessons at impact 0 and 16 at 1.
- Integrity: `format` validates everything (schema, every `where` resolved at
  its pinned commit with line ranges), sorts and rewrites one layout, or
  writes nothing; `seal` additionally requires coverage stamped with the
  commit read before it moves the cursor.
- Migration: `migrate` converts a 0.1 markdown area. Tested on a copy of four
  forgent sources (53 entries, 125 porting rows → 161 lessons, 11 decisions
  kept; a second `format` changes nothing).

Validation run (this repository, draft goal): skills-mcp@dc3dda4f8660 and
meta-skill@e999668e8d8c, 32 lessons. Observed while running:
- The validator caught the author's own wrong line numbers (taken from a
  concatenated `cat -n`), and the zsh `$sha:a.md` modifier trap produced an
  absolute path that `format` refused. Both stay as rules in pass-protocol.md.
- It verifies that a line range exists, not that it shows the claim; a wrong
  range inside the file passes. Spot-check top lessons by hand.
- Fact a ("a concrete failure today") is strict: 11 of 19 ranked lessons got
  impact 0, including a lesson backed by a measured incident whose host
  channel is designed but disabled. Revisit with the human's scores.

Known debt: same Iron Law debt as 0.1 (no RED pressure scenarios); living-doc
and paper flows are sandbox-only; consult and deep-dive not yet dogfooded on
the YAML layout.
