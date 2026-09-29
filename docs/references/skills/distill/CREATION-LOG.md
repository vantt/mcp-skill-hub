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
