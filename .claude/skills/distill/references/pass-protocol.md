# The pass — distill

One pass reads one source from its cursor to its current HEAD (or in full the
first time) and records what the host can learn from it. Run every step in
order. Steps 2 and 7 are where discovery comes from; never skip them.

## 1. Mirror

**Why.** Every `where` is verified against a local copy at the commit it cites,
and deltas are computed from it.

Keep one clone per source at `upstreams/<name>` (gitignored). Clone once with
`git clone --filter=blob:none <url> upstreams/<name>`: a partial clone keeps
full history without downloading every file, and `git show` fetches a blob
only when you read it. Then `distill.mjs delta <name>` pulls and prints what to
read.

Traps found while running:
- In a partial clone, listings that need file sizes (`git diff --stat`,
  `ls-tree -l`) download every blob in the range. Use `--name-only` or
  `--name-status`.
- In zsh, `"$sha:path"` is read as a history modifier (`$var:h`, `$var:s`...).
  Write `"${sha}:path"`.
- `delta` prints the commit to stamp as `coverage.at`; use that value, not a
  hand-typed one.

## 2. Read the host and its goal first

**Why.** You can tell "new" from "already covered" only if you know what the
host does, and you read a source well only with questions in mind.

Read the goal in `distill.yaml`, the host's README, PRD or design documents for
the domains the source touches, and the host code those documents name. Read
the existing lessons for this source (`rank --source <name> --all`) and for the
domains it touches. Then write down, for yourself, 3-5 questions the host
cannot answer today or answers weakly, chosen from `failures_it_prevents`.
Read the source looking for answers to them, and for anything that
contradicts the host. The report lists the questions and which were answered.

Example questions for a skill hub: "How does a catalog stay fresh without
re-downloading every skill?" "How does an agent learn that the skill it loaded
was the wrong one?"

## 3. Delta

No cursor: scan the source's scope at HEAD in full, following links from its
documents to related docs, scripts and code. With a cursor: read the commit
range `delta` prints. Group commits into themes; never judge commit by commit.
A diff shows where something changed, not what it is now: read every touched
file at the target commit before writing a lesson. If the cursor equals HEAD,
report "no change" and stop.

When the scope widens (new domains, a missed layer, a changed goal), backfill:
read only the new part at the current cursor and `seal --backfill`, which does
not move the cursor.

## 4. Inventory (large scopes only)

For more than about 15 files or 2000 lines, delegate the mechanical inventory
to subagents by area: list mechanisms, quote verbatim with line numbers, no
judgement, and state what they could not read. Classification, notability,
scoring and every write to the learning area stay with you.

## 5. Record each mechanism

For each mechanism you recognize, on every layer
(`references/learning-layers.md`):
- Same mechanism as an existing lesson: update it in place (add the `where`,
  refresh `what` / `notable`), keep the key.
- Already done by the host: a lesson with `contrast: already-covered` and `host`.
- Otherwise a new lesson with `key`, `domains` (primary first), `layer`,
  `what`, `notable`, `where`, `contrast`, `decision: {state: candidate}`.

`notable` must name the mechanism and its trade-off and say what it means for
the goal. "Good practice", "clear structure", or restating the README is not
notable. A lesson that fits several domains lists them all; the first is its
home file. A concept that fits no domain goes under `unclassified` and the
report proposes a new domain; never edit the domain list unprompted.

## 6. Convergence

For each new or updated lesson, search lessons from other sources
(`find <term>`, `rank --domain <d> --all`). The same mechanism found
independently is one lesson with several `where` entries, and its evidence
rises (see the independence rule in `references/scoring.md`).

## 7. Negative space

From the commit range and the current tree, find what the source removed,
reverted, or explicitly refuses to do, and why (commit messages, ADRs,
"non-goals" sections). A removal with a stated reason is often the strongest
lesson; record it on the history layer with `notable` stating the reason, and
cite the commit (`source@<sha>`) when the message is the evidence.

## 8. Upstream removals

If a mechanism an existing lesson describes is gone, set `status: removed` (or
`moved-to:<name>`, `superseded-by:<key>`) and keep the lesson. Its old `where`
stays valid because it is pinned to the old commit.

## 9. Skeptic pass

Re-read every new lesson. Remove it only if it cannot be verified from the
cited lines or is a platitude with no mechanism. For every other doubt
(environment-specific, outside the goal, already in the host) keep it and lower
its weights. Check that no lesson repeats a rejected one without new evidence.

## 10. Score

Score every new or changed lesson against the goal (`references/scoring.md`)
and tag `also_fits` where another home fits better.

## 11. Coverage

Record in the source's `coverage` what the pass read and what it did not read
in the scope's neighbourhood, each skipped area with a reason, stamped with the
commit read:

```yaml
coverage:
  at: 96370cf438dc
  read: [README.md, docs/architecture.md, src/catalog/]
  not_read:
    - {path: src/web/, reason: UI, outside the goal}
    - {path: CONTRIBUTING.md, reason: general contribution guide; not this host's subject}
```

Why: a pass that does not say what it skipped looks complete when it is not,
and the next pass cannot tell where to look.

## 12. Write and seal

Run `distill.mjs format` after editing, fix every reported error, then
`distill.mjs seal <name>` (with `--version <v>` for papers and living docs).
`seal` validates the whole area, checks every `where` against the clone and
that coverage is stamped with the commit you read, and only then moves the
cursor. When it refuses, nothing is written and the cursor stays where it was:
fix and rerun. Do not commit; tell the user what changed and let them commit.

Then report in the format in `SKILL.md`.
