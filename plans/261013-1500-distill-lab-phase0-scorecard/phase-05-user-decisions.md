---
phase: 5
title: "User decisions"
label: user-decision
status: pending
priority: P2
effort: "0.5h (user)"
dependencies: [4]
---

# Phase 5: User decisions

The lead shows the user `reports/scorecard.md` and records the answers in this file under "Recorded decisions".
No agent changes distill-lab, `internal/distill`, `schemas/distill.schema.json`, the web types or fixtures before
every decision below is recorded.

## Decisions only the user can make

1. **What R/E/F means.** The repo defines R/E/F in the older `distill` skill as Reach / Evidence / Effort to port
   (`docs/references/skills/distill/references/extract-rules.md:107-122`). In distill-lab and D7 it names the
   `score` block (relevance, facts/impact, evidence, effort, why) plus `final_score`. Options:
   (a) confirm that R/E/F = the `score` block and `final_score` (recommended; this is what the table measures);
   (b) drop the name and decide per score component only; (c) restore Reach instead of Relevance (needs a new
   measurement, outside this plan).
2. **Thresholds.** Accept the defaults in Phase 4 (0.3 on the judge scale or 10 pp, 4 of 5 same sign, kappa 0.6,
   tau 0.9) or set others. Changing a threshold only re-renders the table (`report` with a new
   `thresholds.json`); no rerun is needed.
3. **Final keep / keep-optional / drop** for `notable`, `contrast` and each R/E/F component, including whether
   `final_score` and the priority order stay.
4. **If R/E/F or `impact` is dropped:** the new rule for the Home "high value" count (`internal/app/curation_home.go:375`,
   public field `pending_high_value_insights`), or keep the field with a documented constant 0.
5. **Migration style** for a drop (Phase 6): one release with a release note (recommended for a single hub), or
   two releases with tolerant readers first.
6. **Whether to keep `cmd/distill-scorecard`** after M6 for later skills, or delete it in Phase 6.

## Recorded decisions

(empty until the user answers; date each entry)

## Success criteria

Every numbered decision has a dated answer here, and the lead has updated backlog row M6.
