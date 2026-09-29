# Consult Protocol — distill

Recall-first materials gathering for designing a NEW host feature, when you
do not yet know which keywords to search for. The inverse of a deep-dive:
deep-dive descends from a KNOWN theme into depth; consult expands from an
UNNAMED feature outward until the whole net has been walked.

Why this works without search infrastructure: completeness is already paid
for by the extract layer. Every entry sits under a `## <domain>` heading
(extract rules), every sealed source covers every taxonomy domain (backfill
invariant, enforced by `check`), and the taxonomy is a closed set kept
honest by the New-domain-discovery rule. So walking domains IS exhaustive
recall over everything the distillery has ingested — no index needed. The
walk is exhaustive at the INDEX layer (~200 tokens/entry), never at the
upstream layer; token spend scales with relevance, not with corpus size.

## The five steps

1. **Map — feature → domains, by DEFINITION not keyword.**
   Read the domain definitions in the host's learning-system doc (the
   companion of `taxonomy.txt`; in this host:
   `docs/reference-learning-system.md`). Judge each of the domains against
   the feature description semantically. Grade every domain into exactly
   one bucket: `hit` (clearly touches), `maybe` (could touch), `miss`
   (does not touch). Every domain gets a grade — this list seeds the
   coverage ledger in step 5.

2. **Domain-walk — the recall core.**
   For every `hit` and `maybe` domain, read that domain's `## <domain>`
   section in EVERY source index under `docs/distillery/sources/`. Collect
   each relevant entry as `<source>:<slug>` + one line on why it matters
   for the feature. Do not summarize away the Notable line — that is where
   the transferable idea lives. Cost-tiering applies: the mechanical
   section-gathering may go to cheap subagents; relevance judgment and the
   brief itself stay with the session model.

3. **Overlay the curated layers.**
   - comparison-matrix rows in the mapped domains (verdicts + divergences —
     a `hòa`/`~` cell signals a design trade-off worth presenting);
   - porting-log rows whose `Đích` column points at the feature's area
     (that column is the pre-computed reverse map "material → host area"),
     plus their R/E/F scores and any Outcome notes;
   - deep-dives whose `entries:` overlap the collected set (check their
     `based_on` cursors for staleness).

4. **Keyword sweep — AFTER the walk, never instead of it.**
   By now you have absorbed the sources' own vocabulary from the entries
   read in steps 2–3. Grep it across the whole learning area
   (`grep -rn <term> docs/distillery/`) to catch entries living under
   `miss` domains — themes that straddle domains (this is how
   integration-contract material was once spread across three domains).
   Promote any catch into the collected set and note its domain in the
   ledger.

5. **Coverage ledger — the no-silent-loss guarantee (MANDATORY).**
   The brief ENDS with a table listing EVERY taxonomy domain:
   `consulted (N entries)` or `ruled out — <one-line reason>`. Ruling out
   is free but must be signed; a wrong ruling is then visible in review
   instead of silent. Follow with an **Ngoài lưới** section: intake rows
   not yet triaged/scanned, sources sealed but thin in a mapped domain,
   stale deep-dives — the net's boundary stated explicitly, never implied
   to be infinite.

## Output — a report, not a distillery artifact

Write to the host's reports directory (this is feature-time material tied
to the current roadmap; contextual fit is never persisted in the learning
area — same principle as triage). Follow the host's report-naming
convention; otherwise `distill-consult-<feature-slug>-<date>.md`.

Shape:

```markdown
# Consult: <feature>
**Bottom Line:** 3–5 câu — chất liệu nào định hình thiết kế mạnh nhất, và lỗ hổng nào của lưới cần biết.
## Chất liệu theo domain     (per domain: entries + 1 dòng vì-sao-liên-quan; giữ nguyên Notable)
## Trade-offs đáng cân nhắc  (từ matrix verdicts + divergences)
## Candidate liên quan       (porting-log rows chạm feature area, kèm score)
## Coverage ledger           (MỌI domain: consulted N / ruled out — lý do)
## Ngoài lưới                (intake chưa scan, deep-dive stale, coverage mỏng)
```

## Rules

- Descend to a `Where:` file (upstream layer) only when a specific entry
  earns it for this design — same discipline as deep-dive L2. Never
  re-scan a source during a consult.
- Consult is read-only on the learning area. If the walk surfaces a gap
  worth fixing (a concept fitting no domain, a stale deep-dive, an entry
  whose Where broke), report it in Ngoài lưới — fixing it is a separate,
  human-approved action.
- Consult gathers and grades material; it does not decide the design and
  does not move porting-log rows. The human designs; candidates stay
  candidates until the human says otherwise.
- Do not add a dump/sections command to distill.mjs for the mechanical
  walk unless repeated consults prove the friction (growth rule): the
  agent's file reads are sufficient at current scale. The command design
  (dump = slice by structure; find = slice by keyword) was already worked
  out on 2026-07-13 — retrieve it from that session's notes if the
  friction materializes.
