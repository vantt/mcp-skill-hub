# Data format — distill

Two kinds of YAML file, both rewritten by `distill.mjs format` in one canonical
layout. Every field below exists to prevent a specific failure; the reason is
given next to it.

## `docs/distillery/distill.yaml`

```yaml
goal:
  status: confirmed            # draft | confirmed; scores against a draft are provisional
  purpose: >-
    The outcome the host exists to produce, in one or two sentences.
  core_domains: [skill-over-mcp]  # optional; domains that ARE the purpose (relevance 3 unless "peripheral:")
  in_scope: [the jobs the host does]
  out_of_scope: [what it explicitly does not do]
  failures_it_prevents: [the concrete failures without it; impact fact a cites these]

domains:                       # the subject areas; every lesson lives under one primary domain
  - name: routing
    definition: choosing which skill an agent loads for a task, and explaining why

sources:
  - name: skills-mcp           # kebab-case; used in every where reference
    type: git-repo             # git-repo | paper | living-doc (cursor semantics differ)
    url: https://github.com/gengirish/skills-mcp
    ref: main                  # optional branch
    local: upstreams/skills-mcp  # optional; default upstreams/<name>
    scope: [src/, scripts/]    # optional; limits delta listings
    derived_from: []           # optional; forks/ports of another source count once as evidence
    upstream_of_host: false    # optional; the host came from this source (not independent of the host)
    cursor: dc3dda4f8660       # last commit (or version) fully distilled; written by seal only
    analyzed: '2026-10-08'
    domains_covered: [routing, catalog]   # backfill: domains this source has been read for
    coverage:                  # what the last pass read and skipped, stamped with the commit read
      at: dc3dda4f8660
      read: [README.md, scripts/build-catalog.mjs, src/]
      not_read:
        - {path: web/, reason: UI, outside the goal}
    notes: free text kept from older passes (optional)

intake:                        # sources waiting for triage (distill.mjs intake <name> --url ...)
  - {name: meta-skill, url: https://github.com/Dicklesworthstone/meta_skill, added: '2026-10-08', why: ...}
```

## `docs/distillery/lessons/<domain>.yaml`

```yaml
lessons:
  - key: head-sha-catalog-cache        # stable forever; never rename
    domains: [catalog, routing]        # first = primary = this file; the others are searchable tags
    layer: content                     # enforcement | content | history | validation | craft | wording | ecosystem
    what: One or two sentences on what the source does.
    notable: >-
      The transferable idea, the trade-off it accepts, and what it means for the goal.
    where:                             # one or more; several sources = convergence
      - skills-mcp@dc3dda4f8660:scripts/build-catalog.mjs#L312-L318
      - skills-mcp@8a87dd8c         # a commit alone, when the evidence is its message
    contrast: extends                  # new | extends | contradicts | already-covered
    host: internal/catalog/refresh.go  # optional: the host place it relates to / landed in
    score:
      relevance: 2
      facts: [a, b, d]
      impact: 3                        # written by the script = number of facts
      evidence: 1
      effort: 2
      why: One sentence on the weights that drive the priority, naming the facts.
    final_score: 3.0                   # written by the script; absent when not ranked
    keywords: [sha cache, trees api] # optional: the source's own vocabulary, for find
    also_fits: [project:forgent]       # optional: project:<name> | skill:<name> | upstream
    status: removed                    # optional: removed | moved-to:<name> | superseded-by:<key>
    found_by: human                    # optional: the human found it and the pass missed it
    outcome: {result: confirmed, note: ...}   # optional, after ported/adapted
    decision: {state: candidate}       # candidate | planned | in-progress | ported | adapted | rejected
                                       #   + reason, at (cursor when decided), local (name in the host)
```

### `where`

`source@commit:path#Lx-Ly`, `source@commit:path`, or `source@commit`. For a git
source the commit must be a SHA and the path must exist at that commit; the
line range must fit the file. Pinning to the commit read, not HEAD, keeps the
evidence true after the upstream moves or deletes files. For a paper or living
doc, use the version as the commit (`paper@v2:paper.pdf#p12`); the file is
checked when a copy exists under `upstreams/<name>/`.

### Fields only migration writes

- `legacy: true` marks a lesson converted from the old markdown layout. Its
  `layer`, `contrast`, `relevance`, `facts`, `evidence` and `effort` may be
  null, and its `where` problems are warnings instead of errors. The script
  drops the flag once every field is filled in.
- `legacy_where` keeps old `Where:` prose that could not be pinned to
  `source@commit:path` (shorthand, line notes), so no evidence is lost.

## Migration from the old markdown layout

Areas made by distill 0.1 have `taxonomy.txt`, `intake.md`, `sources/*.md`,
`comparison-matrix.md` and `porting-log.md`. Every command refuses on them and
points to `migrate`.

`distill.mjs migrate --dry-run` prints what it would do; `distill.mjs migrate`
does it (it refuses when the area has uncommitted changes, because it deletes
the old files and Git history is how you get them back):

- `taxonomy.txt` → `domains` (an inline `# comment` becomes the definition).
- each `sources/<name>.md` → a source entry (cursor, dates, domains covered;
  prose outside entries is kept in `notes`) and one legacy lesson per
  `### entry`, with `Where:` paths pinned to the entry's `Seen:` commit.
- each `porting-log.md` row → the decision on the matching lesson (status,
  local name, destination and commit as `host`, notes as `reason`); a row that
  cites several entries collects their `where` (convergence). The old R/E/F
  score keeps E as evidence and F as effort; reach is dropped, and relevance
  and impact stay empty until the lesson is scored against the goal.
- `intake.md` rows → `intake`.
- `comparison-matrix.md` links are rewritten to `lessons/<domain>.yaml#<key>`;
  the matrix becomes optional (convergence now lives on the lesson).
- `deep-dives/` are kept as they are; `state/` (from a forgent-only `move`
  command) is no longer read.
