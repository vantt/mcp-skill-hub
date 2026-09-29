# Extract Rules — distill

Operating manual for the Extract/Compare steps. The skill body tells you WHEN;
this file tells you HOW.

## Entry schema (per feature, grouped under `## <domain>`)

```markdown
### <feature-slug>
- **What:** what it does, 1–2 sentences
- **Where:** main path(s) in the source, backticked, concrete (no `a|b` or `X-Y.md` shorthand — the checker verifies each token)
- **Notable:** the idea worth learning / what makes it clever
- **Keywords:** (optional) source's own vocabulary when it differs from taxonomy terms — improves grep recall
- **Seen:** <short commit / version> of last observation
- **Status:** (only when it applies) moved-to-<dest> / removed / superseded-by-<slug>, with evidence
```

Slug: kebab-case, stable forever. Cross-reference format: `<source>:<slug>`.

## Update vs new entry

- Same mechanism evolving (new flags, extended behavior, bugfix) → UPDATE the
  entry in place, append a "Từ <commit>: ..." sentence to What/Notable, bump
  `Seen`.
- New mechanism, even if related → NEW entry; link the sibling in Notable.
- Feature deleted or moved upstream → add `Status:` line + keep the entry
  (its learning value survives; the destination may become a new source —
  drop it into intake.md).
- NEVER rename a slug. To restructure, add the new entry and mark the old one
  `Status: superseded-by-<new-slug>`.

## New-domain discovery (concept fits no existing taxonomy domain)

- Never silently skip the finding, and never edit `taxonomy.txt` unprompted —
  both are structural decisions that belong to the human, same tier as
  porting adoption and slug renames.
- Extract the entry anyway, right now, under the closest-fit existing domain
  (or a temporary `## unclassified` heading if nothing is close) — the
  observation is never lost while a taxonomy decision is pending.
- Surface it in the session report as a proposal, same shape as a porting
  candidate: "found `<concept>` — no taxonomy domain fits, recommend adding
  `<candidate-domain-name>`."
- If the human approves: add the domain to `taxonomy.txt` yourself (this
  marks every already-sealed source as needing backfill for it — `check`
  will list them), then move the entry from its temporary home to the new
  domain heading.
- Still `seal` the source normally regardless — the open proposal does not
  block sealing (same headless-mode principle: cursor moves, decisions
  queue, nothing is lost).

## Delta discipline (incremental scans)

- The commit log groups into themes first; extract per theme, not per commit.
- A diff hunk is evidence that something changed, not of what it now is:
  RE-READ the touched file at HEAD before updating an entry.
- Verify every `Where:` token you write exists at HEAD (`check` enforces
  this after the fact — don't wait for it to catch you).
- Full history replay is never required; the current snapshot is the
  accumulated truth. Archaeology ("they tried X and removed it — why?") is
  opt-in, only on a concrete suspicion.

## Cost-tiering protocol

- Mechanical inventory (read files/diffs, quote verbatim, list facts, NO
  judgment) → dispatch cheap subagents (extraction tier), chunked by area,
  reports written to the host's reports directory for durability.
- Classification against taxonomy, notability judgment, matrix comparisons,
  candidate proposals, and all writes to the learning area → the session
  model does these itself.
- Instruct inventory agents: cover EVERY item in scope, compress summaries
  rather than skip coverage, state explicitly what they could not read.

## Related-artifact lookup (no search infra)

1. Grep the comparison matrix first — each canonical row IS the related-set.
2. `grep -rn <term> docs/distillery/` for slugs/keywords.
3. Read only the matched entries (~200 tokens each). Never re-read whole
   indexes; never build a vector/SQL index for this (decision 2026-07-13 —
   escalation thresholds live in the host's design doc if present).

## Matrix rules

- Curated, not exhaustive: a row exists only when a cross-source comparison
  is worth recording (both have it, or a notable absence).
- Cells with ✓ link to the entry anchor: `[→](sources/<name>.md#<slug>)`.
- Record a Best-in-class verdict + one-line why; update the verdict when a
  delta changes the balance (note "@<commit>" when you flip one).
- Symbols: ✓ có | ~ một phần/dạng khác | ✗ không | ? chưa khảo sát.

## Porting-log rules

- Only place porting status exists. Statuses:
  `candidate → planned → in-progress → ported / adapted / rejected`.
- You may ADD `candidate` rows (a shortlist proposal) and fill evidence;
  moving anything past `candidate`, and all rejections, are HUMAN decisions.
- `rejected` requires a written reason — it exists to prevent re-evaluation.
- **Local column** — when a row reaches `ported`/`adapted`, fill `Local` with
  the concept/artifact name it became in the host (keep the source slug when
  unchanged) alongside the destination path. This is the two-way
  source↔local bridge: `map [term]` answers both "what did X become here?"
  and "where did this local thing come from?".
- Delta discipline hook: when a delta scan updates an entry whose feature
  appears in a `ported`/`adapted` row (`map <slug>` to check), note it in
  that row's Ghi chú — the upstream evolved something we carry; the human
  decides whether the local copy should follow.

## Impact scoring (per candidate row, Score column `R# E# F#`)

Score ONCE at candidate creation, while the feature is fresh in context —
never batch re-evaluate the whole log (scores must stay reproducible;
re-reading everything is exactly the cost this system avoids). Re-score a
single row ONLY when a delta scan delivers new evidence for it. The total is
derived, never stored: `distill.mjs rank` computes R×E/F and sorts.

- **R — Reach** (host impact breadth): 1 một component hẹp · 2 một
  subsystem · 3 cross-cutting toàn platform (convention, layout, safety…)
- **E — Evidence** (độ tin của bài học): 1 một nguồn, chưa có outcome ·
  2 nhiều nguồn cùng có, HOẶC một nguồn đã dogfood/đo được ·
  3 hội tụ độc lập (≥2 nguồn tự đến cùng cơ chế) hoặc bài học
  falsified-by-data — tín hiệu mạnh nhất
- **F — Effort to port**: 1 doc/convention/script nhỏ · 2 component ·
  3 subsystem

Two uses of the ranking: (a) porting priority; (b) **deep-dive selection** —
high R×E rows where sources diverge in approach are where the human should
spend deep attention (see deep-dive-protocol.md). Contextual fit with the
host's current roadmap is judged by the human AT TRIAGE TIME and never
persisted — it changes as the roadmap moves.

After a candidate ships (`ported`/`adapted`), when its real value becomes
visible, append an `Outcome:` note to the row (confirmed / ineffective /
adjusted) — the predicted→actual loop that keeps scoring honest.

## Scaling guards

- A source index approaching the host's doc size limit (default ~800 lines)
  splits by domain: `sources/<name>/<domain>.md` + a thin `<name>.md` keeping
  the frontmatter cursor. Do this only when actually hit.
- Report files from inventory agents follow the host's report-naming
  convention when one exists; otherwise `distill-<source>-<scope>-<date>.md`.
