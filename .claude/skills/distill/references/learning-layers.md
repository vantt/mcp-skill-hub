# Learning layers — distill

What a source says is only part of what it can teach. How it enforces its
rules, why it changed them, how it proves they work, and how it builds its
system are often worth more than the rules themselves. A pass reads every
source on the seven layers below, in this order, and records each lesson's
`layer`.

The layers are ranked by how much a lesson on that layer improves the host and
how long the improvement lasts, highest first. The rank breaks ties between
equal scores, and when two lessons compete for the human's attention, the one
on the higher layer goes first. **Cover every layer in every pass.** A pass
that reads only prose is incomplete: in the experiment this skill was built
from, reading the source's scripts added a new enforcement lesson and three
design details to the strongest lesson, all of which a prose-only pass had
missed. The report lists the count per layer, and every layer with 0 lessons
needs a one-line reason ("no CI in this repository", not silence).

## 1. Enforcement (`enforcement`): code, scripts, CI, helpers

**Why it ranks first.** A rule that has become an automatic check holds by
itself; it does not depend on anyone remembering it. Rules that live only in
prose erode, and sources sometimes prove it with numbers.

**Ask.** Which of the source's rules are enforced by code rather than by text?
What does the check accept, reject and grandfather? Which helper makes the
correct way the easiest way, so nobody is tempted by the wrong one?

**Look at.** Validators, linters and their baselines or allowlists, CI workflow
steps, pre-commit hooks, shared helpers and libraries, schema checks at a
boundary, and the tests of those scripts. Follow every script or command a
document names.

**Example.** openclaw banned wall-clock timeout races in its testing guide, yet
call sites grew from 123 to 478 in under a month. A shrink-only ratchet (a
checked-in baseline of existing sites, a CI check that rejects new ones, a
`--prune` step) stopped the growth, and helpers give authors a correct
replacement so the check never leaves them stuck.

## 2. Content (`content`): mechanisms, features, procedures, criteria

**Why it ranks here.** This is where most lessons are found: the mechanisms,
data models, procedures and criteria the host lacks or states more weakly.

**Ask.** Which questions from the host's goal can the source answer that the
host cannot? Which mechanism does the host lack entirely, and which does it
have in a weaker form?

**Look at.** The source's core modules and their design documents, its data
model, its main commands and their behaviour, the README sections that describe
mechanisms (not marketing).

**Example.** gengirish/skills-mcp keys its per-repository catalog cache on the
repository's HEAD commit: on a refresh an unchanged repository costs one API
call, and only a changed one is listed and fetched again. A hub that checks
every skill on every refresh has a direct content lesson there.

## 3. History (`history`): what was tried, dropped, and why

**Why it ranks here.** It is the strongest evidence a source can offer and it
saves the host from repeating someone else's mistake. It ranks below content
only because such lessons are rarer.

**Ask.** What did the source remove, revert or supersede, and what reason did
it give? Which design failed in practice, and what replaced it? Are there
numbers?

**Look at.** Commit messages in the range (`git log`, removals with
`--diff-filter=D`), reverts, ADRs and superseded design documents, changelogs,
migration guides. Cite the commit itself (`source@<sha>`) when the evidence is
its message.

**Example.** beads pivoted away from JSONL-as-truth: Dolt SQL became the
canonical store and JSONL only an export. A host that plans "files are the
truth, a database is rebuilt from them" should read that pivot's reasons
before committing to the same design.

## 4. Validation (`validation`): how they prove it works

**Why it ranks here.** The host only improves when it can tell whether a
change helped. A source's proof practices give the host a way to check its own
claims and close the loop from lesson to outcome.

**Ask.** How does the source prove that a feature, a fix or a rule works? What
proof bar does it set, and what does it refuse to accept as proof?

**Look at.** Test strategy documents, acceptance tests, eval suites, benchmark
scripts, mutation checks, controls that revert a fix, incident write-ups cited
as the reason for a rule.

**Example.** superpowers makes support for a new agent harness falsifiable: the
pull request must include a session transcript where a fixed prompt in a clean
session triggers the expected skill before any code is written.

## 5. Craft (`craft`): how the system is built

**Why it ranks here.** Structure decides whether a design stays understandable
and extensible. Its impact is broad but indirect, so it ranks below lessons
that change what the host does.

**Ask.** How is the source split into modules and layers? What does it load
lazily? Where are its boundaries and contracts? How does it keep one source of
truth?

**Look at.** Directory layout, module boundaries, interfaces between
components, configuration layering, extension points.

**Example.** skills-mcp writes a 120-byte `catalog-meta.json` next to its
6.7 MB catalog, so the MCP handshake reports totals without parsing the
catalog, and the full catalog loads only on the first tool call that needs it
(cold start about 750 ms → 525 ms).

## 6. Wording (`wording`): text that closes loopholes

**Why it ranks here.** Wording is cheap and effective, but each lesson is a
small step.

**Ask.** Which user- or agent-facing sentences leave no room for
interpretation? Which error messages tell the reader exactly what to do next?
Which rules name the exact substitutes someone might reach for?

**Look at.** Error and refusal messages, CLI help text, agent instructions,
imperative rules in docs, defined terms.

**Example.** A refusal written as ERROR (the rule) / WHY (the reason) / FIX
(the next command) lets an agent recover without guessing; a bare "invalid
input" does not.

## 7. Ecosystem (`ecosystem`): how the source fits its surroundings

**Why it ranks last.** These lessons usually help other projects or the wider
toolchain more than the host's own goal, so most of them are recorded with a
low relevance and an `also_fits` home.

**Ask.** How is the source distributed, installed and kept current? How does it
integrate with neighbouring tools? Where are its trust boundaries?

**Look at.** Install scripts, packaging, release process, plugin or extension
manifests, cross-project links, rules about what may run where.

**Example.** A source that refuses to run untrusted fork tooling locally and
routes it to secretless CI has a trust-boundary lesson for any host that runs
third-party code.

## Related material counts as part of the source

References the source's docs link to, the code those docs name, and the
repository's agent instructions are part of the source. Follow those links
from the source's scope and list what you read in its coverage.

## Stay on the host's subject

Every layer is read **within the host's subject**, as the goal defines it.
Craft and wording lessons come from how the source builds and words the parts
that bear on the host's goal, not from its general contribution guide, style
guide or "how to write docs" manual. Those apply to every project and belong to
a different pass. List them in the source's `coverage.not_read` with that
reason instead of reading them for this host.

Ask: would this lesson still make sense if the host's goal were about something
else entirely? If yes, it is general advice and probably off subject.
Example: superpowers' `writing-skills` manual is a general guide for writing any
skill; for a host whose goal is skill distribution, it goes to `not_read`
("general skill-writing manual; not this host's subject").
