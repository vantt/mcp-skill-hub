# Consult Protocol — distill

Recall-first materials gathering for designing a NEW host feature, when you
do not yet know which keywords to search for. The inverse of a deep-dive:
deep-dive descends from a KNOWN theme into depth; consult expands from an
UNNAMED feature outward until the whole net has been walked.

Why this works without search infrastructure: completeness is already paid
for by the pass. Every lesson carries its domains (the primary one decides
its file, the others are tags), every sealed source records which domains it
was read for (`domains_covered`, backfill enforced by `check`), and the domain
list is a closed set kept honest by the rule that a concept fitting no domain
goes to `unclassified` with a proposal. So walking domains IS exhaustive
recall over everything the area has ingested. The walk is exhaustive at the
lesson layer, never at the upstream layer; token spend scales with relevance,
not with corpus size.

## The five steps

1. **Map — feature → domains, by DEFINITION not keyword.**
   Read each domain's `definition` in `distill.yaml`. Judge each domain
   against the feature description semantically and grade it into exactly
   one bucket: `hit` (clearly touches), `maybe` (could touch), `miss` (does
   not touch). Every domain gets a grade — this list seeds the coverage
   ledger in step 5.

2. **Domain-walk — the recall core.**
   For every `hit` and `maybe` domain, run
   `distill.mjs rank --domain <d> --all`: it lists every lesson whose domains
   include `<d>`, from every source, ranked ones first. Read the matching
   entries in the lesson files. Collect each relevant lesson as its key + one
   line on why it matters for the feature. Do not summarize away `notable` —
   that is where the transferable idea lives. Mechanical gathering may go to
   cheap subagents; relevance judgement and the brief stay with you.

3. **Overlay the curated layers.**
   - decisions on the collected lessons: `planned` / `ported` / `adapted`
     ones and their `host` (the pre-computed map "material → host area"),
     `rejected` ones and their reasons, and any `outcome`;
   - `contrast: contradicts` lessons — a trade-off worth presenting;
   - deep-dives whose `entries:` overlap the collected set (check `based_on`
     for staleness);
   - `comparison-matrix.md`, if the area still has one from before.

4. **Keyword sweep — AFTER the walk, never instead of it.**
   By now you have absorbed the sources' own vocabulary from the lessons read
   in steps 2–3. Run `distill.mjs find <term>` for that vocabulary to catch
   lessons filed under `miss` domains — themes that straddle domains.
   Promote any catch into the collected set and note its domain in the
   ledger; if it clearly also belongs to a mapped domain, propose adding that
   domain to the lesson's `domains`.

5. **Coverage ledger — the no-silent-loss guarantee (MANDATORY).**
   The brief ENDS with a table listing EVERY domain:
   `consulted (N lessons)` or `ruled out — <one-line reason>`. Ruling out is
   free but must be signed; a wrong ruling is then visible in review instead
   of silent. Follow with an **Outside the net** section: intake items not
   yet triaged, sources not yet read for a mapped domain, sources thin in a
   mapped domain, stale deep-dives, unscored lessons in mapped domains — the
   net's boundary stated explicitly, never implied to be infinite.

## Output — a report, not a learning-area artifact

Write to the host's reports directory (this is feature-time material tied to
the current roadmap; roadmap fit is never stored in the learning area).
Follow the host's report-naming convention; otherwise
`distill-consult-<feature-slug>-<date>.md`.

```markdown
# Consult: <feature>
**Bottom Line:** 3–5 sentences — which material shapes the design most, and which gaps in the net to know about.
## Material by domain      (per domain: lessons + one line on why each matters; keep notable)
## Trade-offs to weigh     (contradicting lessons, divergent approaches)
## Related decisions       (planned/ported/rejected lessons touching the feature, with reasons)
## Coverage ledger         (EVERY domain: consulted N / ruled out — reason)
## Outside the net         (untriaged intake, unread domains, stale deep-dives, unscored lessons)
```

## Rules

- Descend to a `where` file (upstream layer) only when a specific lesson
  earns it for this design, at its pinned commit. Never re-scan a source
  during a consult.
- Consult is read-only on the learning area. If the walk surfaces a gap worth
  fixing (a concept fitting no domain, a stale deep-dive, a lesson missing a
  domain tag), report it in Outside the net — fixing it is a separate,
  human-approved action.
- Consult gathers and grades material; it does not decide the design and
  does not change decisions. The human designs; candidates stay candidates
  until the human says otherwise.
