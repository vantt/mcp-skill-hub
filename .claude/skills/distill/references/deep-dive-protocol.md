# Deep-Dive Protocol — distill

Theme-centric analysis across sources ("how do the references solve X?").
This is the payoff step of the whole learning system: side-by-side
perspectives of competing approaches, ending in a SYNTHESIS — a combined
best-of design fitted to the host project, not just a comparison. Built on
already-paid layers so depth stays cheap. Triggered by the human naming a
topic; never self-initiated.

**Picking what to dive into:** run `distill.mjs rank`. Strong deep-dive
candidates are rows with high R×E where the matrix shows sources solving the
same problem DIFFERENTLY (a `hòa`/trade-off verdict, or ~ cells) — divergence
between strong sources is exactly where a combined solution beats copying.

## Cost pyramid — descend only where needed

| Layer | Material | Cost |
|---|---|---|
| L0 Assemble | grep comparison-matrix + source indexes (related-scan recipe in extract-rules.md) → list of relevant `source:slug` entries | ~zero |
| L1 Reuse | the inventory/research reports already in the host's reports dir — read only the sections covering the L0 entries | cheap, already paid for |
| L2 Targeted read | ONLY the files listed in the chosen entries' `Where:` lines; dispatch cheap subagents for mechanical quote-pulling | bounded by Where |
| L3 External | web/docs research beyond the sources (xia-style, evidence-labeled) | optional, ask first |

Never re-scan a whole source for a deep-dive; if L0 finds no entries for the
topic, that is a signal the topic needs a normal extract pass first.

## Output — `docs/distillery/deep-dives/<topic-slug>.md`

```markdown
---
topic: <slug>
date: YYYY-MM-DD
based_on: [beegog@e70602a, repository-harness@9cc306d]   # source cursors analyzed
entries: [beegog:capture-queue-settlement, repository-harness:growth-rule-friction]
---

# Deep-dive: <topic>

**Bottom Line:** 3–5 câu — kết luận + khuyến nghị trước, chi tiết sau.

## Câu hỏi
## Cách từng nguồn giải quyết   (per source: mechanism, evidence quotes, file refs,
                                 và QUAN TRỌNG: "why" — trade-off họ chấp nhận, bối cảnh khiến họ chọn vậy)
## So sánh & trade-offs         (bảng đối chiếu chiều-by-chiều, không phải liệt kê)
## Giải pháp tổng hợp cho host  (BẮT BUỘC — design đề xuất ghép cái tốt nhất của từng
                                 approach, nêu rõ lấy gì từ đâu, bỏ gì và vì sao không
                                 hợp bối cảnh host; đây là output chính của deep-dive)
## Portable ideas               (mỗi ý → candidate row trong porting-log, kèm R/E/F)
## Open questions
```

## Rules

- Every claim carries its layer of evidence (entry / report / file quote /
  external) — same evidence-labeling discipline as research briefs.
- Portable ideas found here do not bypass the porting log: add `candidate`
  rows with R/E/F scores; the human still decides.
- Staleness: `based_on` pins the source cursors analyzed. A later delta scan
  that updates any entry listed in `entries:` should append a line
  `> stale vs <source>@<new-commit> — <what changed>` to the deep-dive.
  Re-dive only when the human asks.
- Deep-dives are committed knowledge (policy side), same as the indexes.
