# Recording and scoring lessons — distill

## Record everything, then weigh it

The lesson files are the complete record of what the pass recognized in a
source, not a shortlist. Every mechanism you recognize becomes a lesson,
including:

- **things the host already does.** Record them with
  `contrast: already-covered` and `host: <where the host does it>`. They are
  evidence that an independent team reached the same design, which strengthens
  the host's choice, and they keep the next pass from mistaking them for new
  findings. They are recorded, not ranked.
- **things specific to the source's own environment**, such as its own
  infrastructure. Record them with a low relevance. Their idea may transfer
  later, and recording them shows they were read and judged.
- **things that fit somewhere else better than the host**: another project, a
  skill, the upstream itself. Record them here too and tag `also_fits`, for
  example `also_fits: [project:forgent, upstream]`. The human decides later
  whether to copy them.

Weights decide the order in which the human sees lessons, never whether a
lesson is kept. The only things you leave out are claims you cannot verify from
the cited lines and platitudes with no mechanism behind them ("good structure",
"well documented").

Why: a shortlist hides the agent's judgement. When the record is complete, the
human can see what was read and judged low, correct a weight, and the next pass
does not rediscover the same thing.

## The four weights

Score each lesson when you create it, against the confirmed goal. Re-score only
when new evidence arrives (a new source agrees, a removal is found) or the goal
changes. All scores are predictions until an `outcome` is recorded.

```yaml
score:
  relevance: 2          # 0-3
  facts: [a, b, d]      # impact = number of facts that hold (0-5); the script writes impact
  evidence: 1           # 1-3
  effort: 2             # 1-3
  why: >-
    In scope (catalog freshness). a: refresh re-downloads every skill; b: no
    blob cache today; d: lands as a cache check in the indexer. One source.
```

### Relevance (0-3): does the lesson serve the goal's purpose?

- **3** it is the core of the purpose; without it the host fails at its main job.
- **2** it serves the purpose directly, within `in_scope`.
- **1** adjacent: related to the host's area but not its purpose.
- **0** outside the goal; recorded for completeness, usually with an `also_fits` home.

Ask: which sentence of the goal does this lesson serve? If you cannot name one,
it is 1 or 0.

**Core domains.** The goal may name `core_domains`: the domains that *are* the
purpose (for a skill hub: serving skills over MCP and recommending the right
skill). A lesson whose primary domain is core is relevance 3 by default; if you
score it lower, start a sentence of `why` with `peripheral:` and say why (for
example "peripheral: only matters once a second retrieval channel exists").
`check` warns otherwise. Why: in the first real run the scorer gave core
recommendation lessons 2 ("in scope") out of habit, which pushed them below
lessons on side topics; the warning makes that judgement explicit instead of
silent. It never sets the score itself, because a lesson in a core domain can
still be a side detail.

### Impact (0-5): count the facts that hold

Impact is a prediction, so it is built from facts a second reader can check,
not from a feeling of "medium" or "high". Write down which facts hold; the
script sets `impact` to their number.

- **a — a concrete failure, today or by design.** You can write "the host does
  X, and the result is wrong because Y", naming a failure from
  `failures_it_prevents` or one plainly inside the goal. X is either what the
  host does today, or what its own written design says it will do (cite the
  document section in `why`, e.g. "doc 03 §13 plans a vector channel with no
  admission rule"). A design that is only an idea in someone's head does not
  count. **Required**: without fact a, impact is 0 and no other fact counts.
  Why "by design" counts: in the first real run, a lesson backed by a measured
  incident scored 0 only because the host had not built the channel yet,
  although its design document already planned the exact mechanism that failed.
  A lesson is most valuable before the design is built.
- **b — a real gap.** No host rule, code or document covers it today
  (`contrast: new`), or it is `extends` and you can quote the host line and
  show what it lacks.
- **c — it recurs.** You can point to two or more places in the sources (code,
  tests, history) where the failure occurs, or it applies to every run of the
  host's main procedure.
- **d — enforceable.** Adopted, it becomes a check, a required field, a gate
  or a script, not advice prose. (This overlaps the enforcement layer on
  purpose: a lesson that can enforce itself acts more directly on the failure.)
- **e — silent or proven.** The failure passes unnoticed while wrong, or a
  source shows a commit, incident or number fixing it.

Why five counted facts instead of a 0-3 judgement: on a 3-point impact scale,
most lessons in the experiment landed on 0 or 1 and seven tied at the same
score. Counting facts spreads the scale and makes every point auditable: a
reviewer who disagrees can name the fact they dispute.

### Evidence (1-3): how strong is the sources' support?

- **1** one source states it.
- **2** two sources agree, or one source shows it was measured or used in practice.
- **3** independent convergence (two or more independent sources reached the
  same mechanism on their own), or a removal or change backed by data.

**Independence rule.** Sources linked by `derived_from` (a fork, a port, a
distillation of another source) are one line of descent and count once. A
source with `upstream_of_host: true` is where the host came from; its agreement
with the host is not independent confirmation of the host's choices. `check`
warns when evidence 3 rests on one independent source; keep a 3 there only for
a removal or change backed by data, and say so in `why`.

### Effort (1-3): how much does adopting it change the host?

- **1** wording, one setting, or one small script or check.
- **2** a new component, step, or section.
- **3** restructures a subsystem.

## Priority

`final_score = relevance × impact × evidence / effort`, computed by the script,
stored on the lesson and used to sort. Never compute or write it by hand.

A lesson is **ranked** only when it is fully scored, has relevance ≥ 1, is not
`already-covered`, and is not retired (`status`). Everything else is recorded
but not ranked, in named groups: not yet scored, outside the goal, already
covered, retired. So a score of 0 never means two different things.

Ties break by layer rank (enforcement first), then higher evidence, then lower
effort, then more `where` entries, then key. `rank` marks tied scores with `=`;
say in the report that tied lessons' order is only the tie-break.

## Convergence

When a new source shows the same mechanism as an existing lesson, add its
`where` to that lesson instead of creating a second one, refresh `what` and
`notable` if the new source adds detail, and raise `evidence` if the
independence rule allows. In the experiment, the second source turned two weak
lessons into strong ones and produced a new top lesson. Report lessons whose
rank moved because of convergence: the human's earlier decisions may need a
second look.

## Decisions

The human owns every decision; you create `candidate` only. Record their word
with `decide`, by number (as `rank` prints it) or key. Run `rank` before
resolving numbers, because a `format` can reorder.

- `planned`, `in-progress`: they intend to apply it.
- `ported` / `adapted`: applied as is / changed; needs `--host <path>` and
  ideally `--local <name it has in the host>`, so `map` can answer "where did
  this come from?" both ways.
- `rejected`: needs `--reason`. A rejected lesson is proposed again only when a
  later pass adds a `where` newer than `decision.at`; say so explicitly.

After a ported lesson has shown its real value, record `outcome` (confirmed,
ineffective, adjusted). That is how scoring stays honest over time.
