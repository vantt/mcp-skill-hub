# Deep-Dive Protocol — distill

Theme-centric analysis across sources ("how do the references solve X?").
This is the payoff of the learning area: side-by-side views of competing
approaches, ending in a SYNTHESIS — a combined design fitted to the host's
goal, not just a comparison. Built on already-paid layers so depth stays
cheap. Triggered by the human naming a topic; never self-initiated.

**Picking what to dive into:** `distill.mjs rank --domain <d>`. Strong
candidates are high-scoring lessons where sources solve the same problem
DIFFERENTLY: separate lessons in one domain with `contrast: contradicts`
against each other or the host, or lessons whose `notable` names a trade-off
another source made the other way. Divergence between strong sources is
exactly where a combined design beats copying one of them.

## Cost pyramid — descend only where needed

| Layer | Material | Cost |
|---|---|---|
| L0 Assemble | `rank --domain` + `find <term>` over the lesson files → list of relevant lesson keys | ~zero |
| L1 Reuse | inventory reports already in the host's reports directory — read only the sections covering the L0 lessons | cheap, already paid for |
| L2 Targeted read | ONLY the files named in the chosen lessons' `where` refs, at the pinned commit (`git -C upstreams/<s> show "${sha}:path"`); cheap subagents may pull quotes | bounded by where |
| L3 External | web/docs research beyond the sources, evidence-labeled | optional, ask first |

Never re-scan a whole source for a deep-dive; if L0 finds no lessons for the
topic, the topic needs a normal pass first.

## Output — `docs/distillery/deep-dives/<topic-slug>.md`

```markdown
---
topic: <slug>
date: YYYY-MM-DD
based_on: [skills-mcp@dc3dda4, meta-skill@1a2b3c4]   # source cursors analyzed
entries: [head-sha-catalog-cache, rrf-hybrid-search] # lesson keys
---

# Deep-dive: <topic>

**Bottom Line:** 3–5 sentences — conclusion and recommendation first.

## Question
## How each source solves it   (mechanism, pinned evidence, and the WHY: the trade-off
                                each accepted and the context that made them choose it)
## Comparison & trade-offs     (dimension-by-dimension table, not a list)
## Combined design for the host (REQUIRED — take the best of each approach, say what
                                comes from where, what is dropped and why it does not
                                fit the host's goal; this is the main output)
## Portable ideas              (each one a lesson: new key, or a where/notable update)
## Open questions
```

## Rules

- Every claim carries its layer of evidence (lesson / report / pinned file
  quote / external).
- Portable ideas become lessons in `lessons/<domain>.yaml`, scored like any
  other, `decision: candidate`; the human still decides.
- Staleness: `based_on` pins the source cursors analyzed; `check` warns when a
  source's cursor has moved past a pin. When a later pass changes a lesson in
  `entries:`, append `> stale vs <source>@<new-commit> — <what changed>` to the
  deep-dive. Re-dive only when the human asks.
- Deep-dives are committed knowledge, like the lesson files.
