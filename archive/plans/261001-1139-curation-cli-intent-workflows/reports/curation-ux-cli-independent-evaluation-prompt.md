# Independent evaluation prompt: simplify Skill Hub curation CLI

You are an independent product-interface and CLI reviewer. Evaluate Skill Hub's curation experience and produce a decision-ready redesign recommendation. Do not implement code. Do not assume the proposed redesign is correct. Challenge it against current behavior, safety constraints, established CLI conventions, and the requirement that the same task model later power a simple web UI.

## Project context

Skill Hub is a Go CLI and MCP server that maintains a Git-backed collection of agent skills. Skills are plain canonical files. Active skills may be recommended to agents through a derived catalog. The product supports source intake and monitoring, importing skills as drafts, creating and editing skills, lifecycle transitions, source distillation, insight review, and Git-based workspace ownership.

Users should not need to understand candidate IDs, manually invented source IDs, adapters, catalog generations, proposal digests, or repository/path decomposition for ordinary curation. The target audience includes:

1. A person who copies a GitHub URL pointing directly to a skill folder.
2. A person who has a local folder containing `SKILL.md`.
3. A person creating and editing their own skill.
4. A maintainer reviewing active-skill changes in a shared Git repository.
5. An agent executing the same workflows through MCP tools.

The product owner’s priority is explicit: the CLI must be genuinely simple, memorable, convenient, and natural. The CLI contract should map directly to a future web UI; the web layer should not need to invent a second orchestration model.

## Primary decision

Should Skill Hub adopt the following intent-first command model, or is there a smaller and safer alternative?

```text
Use one skill                skillhub skill add <locator>
Create a local skill         skillhub skill create ...
Change a skill               skillhub skill edit <id> ...
Inspect or review a skill    skillhub skill show|review <id>
Change lifecycle             skillhub skill activate|deprecate|archive <id>
Learn from upstream          skillhub source watch <locator>
Check upstream               skillhub source check ...
Start from priorities        skillhub status
```

The leading proposal is that `skill add <locator>` accepts one GitHub skill-folder URL or local folder, derives repository/ref/path and IDs, previews one draft import, and does not enable monitoring unless `--watch` is explicit. `source watch <locator>` remains the monitoring/learning workflow. `skill review <id>` is proposed as a read-only composition of validity, lifecycle, routing, provenance, Git state, and exact next action.

## Required repository evidence

Begin by reading:

- `plans/reports/curation-ux-cli-simplification-report.md`
- `plans/reports/code-reviewer-260930-1045-ux-surface-audit.md`
- `docs/curating-skills.md`
- `docs/user-guide.md`
- `README.md`
- `internal/delivery/cli/source.go`
- `internal/delivery/cli/skill.go`
- `internal/delivery/cli/help.go`
- `internal/app/source.go`
- `internal/app/source_import.go`
- `internal/app/skill_lifecycle.go`
- `internal/catalog/open.go`
- `internal/catalog/build.go`
- `internal/delivery/mcpserver/source_tools.go`
- `internal/delivery/mcpserver/source_import_tools.go`
- `internal/delivery/mcpserver/skill_tools.go`
- `system-skills/curator/SKILL.md` or the current generated/bundled curator source if that path moved.

Run the current binary or `go run ./cmd/skillhub` against disposable workspaces. Do not mutate the user's real workspace. Exercise at minimum:

1. Status in a new workspace.
2. Create, inspect, edit with `--editor` if a controlled editor can be supplied, activate, and diff.
3. Source capture, triage preview, confirmation, and import using a bounded public test repository or existing fixture.
4. Direct editing of draft and active `SKILL.md`, followed by catalog-triggering reads.
5. Invalid direct edits to verify catalog validation behavior.
6. Partial Git staging to test whether any proposed hook validates the index or only the working tree.
7. Concurrent editor behavior: open editor A, mutate canonical content through writer B, close editor A, then inspect what `PreviewUpdate` proposes. Verify the editor-open-to-preview window rather than assuming proposal pins cover it.

Never execute scripts from imported sources. Respect network policy and use repository fixtures where they provide equivalent evidence.

## External comparison

Compare the proposed grammar and progressive disclosure with a small set of established CLIs whose official documentation demonstrates relevant patterns, for example:

- `gh` for URL/resource inference and human-versus-JSON output;
- `cargo add`, `npm install`, `pip`, or `brew` for one-locator/package addition;
- Git for staged-versus-working-tree semantics;
- a mature plugin or extension manager for install versus update/watch distinctions.

Prefer official documentation, source repositories, standards, and changelogs. Treat forum posts and social media as weak signals only. Do not import another product's conventions without showing why they fit Skill Hub's safety and provenance model.

## Questions to answer

### 1. User task model

- What are the minimum nouns and verbs a first-time user must learn?
- Is `skill add <locator>` the correct noun/verb for importing one skill?
- Is `source watch <locator>` the correct separation for persistent upstream learning?
- Can any command be removed or folded without hiding a material safety decision?
- Which internal identifiers should never appear in default human output?

### 2. Locator resolution

- Define exact behavior for GitHub repository, tree, blob, and `SKILL.md` URLs.
- Account for Git refs containing `/`; reject naive path splitting.
- Define behavior for local relative, home-relative, and absolute folders.
- Define symlink, path traversal, file size, repository size, and network boundaries.
- Decide how skill IDs and internal provenance/source IDs are derived and how collisions are handled.
- Decide what happens when zero, one, or many skills are discovered.

### 3. Import versus monitoring

- Should `skill add` create a provenance source record even when monitoring is disabled?
- Should `--watch` compose onboarding and import atomically or remain a second command?
- What durable provenance is needed for a local folder without persisting a personal absolute path?
- How should a non-portable watched local path be represented and warned about?

### 4. Editing and governance

Evaluate three editing paths independently:

```text
Agent preview/confirm
skill edit --editor
Direct canonical file edit
```

For each, state:

- when canonical files change;
- which validation runs;
- whether preview and exact confirmation exist;
- stale/concurrent update behavior;
- operation receipt and provenance behavior;
- when valid content becomes active locally;
- which user or team workflow it best serves.

Verify this known caveat: current `--editor` reads original content before opening the editor and constructs `PreviewUpdate` only after editor exit. Pins protect preview-to-confirm, not editor-open-to-preview. Determine whether an original-content digest, a three-way merge, or another simpler mechanism should close that window.

Do not claim direct edits bypass structural validation. Current catalog rebuild validates canonical files before publishing a new generation. Separate structural validity from semantic approval and routing quality.

### 5. Hooks, CI, and Git

- Should Skill Hub offer an optional hook installer?
- Should a hook validate the working tree or a materialized staged snapshot?
- Demonstrate the partial-staging failure mode if working-tree validation is presented as commit validation.
- Keep hooks read-only and optional unless evidence supports a stronger policy.
- Define the minimum CI check for a shared skills repository.
- Avoid rebuilding GitHub/GitLab branch protection or pull-request approval inside Skill Hub.

### 6. Review surface

- Is a separate `skill review <id>` command justified, or can `skill show`, `validate`, and `diff` compose adequately?
- Which facts belong in default output versus `--verbose` or `--json`?
- Which semantic checks can be deterministic, and which must remain warnings or human judgment?
- How should the CLI communicate active-local effect, uncommitted state, provenance, and routing overlap without jargon?

### 7. CLI-to-web portability

For every recommended CLI operation, map it to one future web action. Reject designs where the web UI must reimplement a multi-command transaction that the application layer does not own atomically. Identify which application-service workflows should back CLI, MCP, and web equally.

### 8. Migration and compatibility

- Which current commands remain as advanced primitives?
- Which commands should be aliases, deprecated, or hidden from beginner documentation?
- How can scripts using current commands continue working?
- What telemetry or controlled usability tests would prove the redesign is better without collecting sensitive task content?

## Required option comparison

Compare no more than three coherent designs. At minimum include:

1. Intent-first split: `skill add` for import, `source watch` for monitoring.
2. A consolidated `source add` design that onboards and optionally imports.
3. A minimal-change design that keeps current primitives but adds inference and better guidance.

For each design provide:

- common-path commands;
- number of user decisions and copy/paste steps;
- safety and provenance properties;
- automation and MCP implications;
- web mapping;
- compatibility cost;
- the assumption it depends on most;
- the condition under which it fails first.

Recommend one design or reject all three and provide a better bounded alternative.

## Evaluation rubric

Score each design from 1–5 with evidence:

| Criterion | Meaning |
|---|---|
| Simplicity | Few concepts, flags, IDs, and round trips on the common path. |
| Learnability | A new user can predict the next command from intent. |
| Memorability | Commands remain recallable without reopening documentation. |
| Convenience | GitHub-folder and local-folder journeys require minimal manual decomposition. |
| Safety | No hidden activation, overwrite, monitoring, network, or destructive behavior. |
| Governance | Clear structural validation, semantic review, receipts, and Git ownership. |
| Automation | Deterministic non-interactive behavior and stable JSON/MCP contracts. |
| Web portability | One application workflow maps to one coherent web action. |
| Compatibility | Existing scripts and advanced workflows have a credible migration path. |

Do not let total scores hide a P0 failure. A design that silently overwrites, activates, watches, or weakens canonical validation is unacceptable regardless of average score.

## Required deliverable

Return one detailed Markdown report with this structure:

1. **Verdict** — direct answer and recommended command model.
2. **Observed current journeys** — exact commands, outputs, and step counts.
3. **Confirmed facts, inferences, and unresolved uncertainty** — clearly separated.
4. **Option comparison** — no more than three designs and rubric scores.
5. **Recommended CLI specification** — grammar, defaults, errors, output, JSON/MCP implications.
6. **Locator-resolution contract** — GitHub and local cases, including edge cases.
7. **Edit/review/governance contract** — agent, `--editor`, direct files, hooks, CI, concurrency.
8. **CLI-to-web mapping** — one table covering every recommended task.
9. **Compatibility and migration plan** — current primitives, aliases, deprecations.
10. **Acceptance tests** — behavior-focused scenarios a future implementation must pass.
11. **Rejected ideas** — with evidence and failure conditions.
12. **Open decisions** — only decisions that genuinely require product-owner judgment.

Every finding must include:

- the specific claim;
- a source URL, preferably an immutable GitHub permalink for repository evidence;
- a repository file and line anchor when applicable;
- one sentence explaining why the evidence matters to the decision.

When sources conflict, state the conflict and distinguish confirmed fact, inference, and uncertainty. After the first analysis, perform a gap round: revisit contradictions, single-source claims, untested edge cases, and any recommendation that depends on an assumption. Do not call the report complete until each P0/P1 recommendation has evidence, a failure mode, and an observable acceptance check.

## Avoid

- Do not implement code or edit project files.
- Do not merely restate `plans/reports/curation-ux-cli-simplification-report.md`.
- Do not assume proposed commands already exist.
- Do not recommend a web UI as a substitute for fixing the application/CLI contract.
- Do not expose internal IDs merely because the current implementation uses them.
- Do not remove preview, provenance, lifecycle, canonical validation, or Git ownership to reduce command count.
- Do not describe hooks as commit validation unless an isolated staged snapshot is actually validated.
- Do not claim proposal pins protect the editor-open-to-preview interval.
- Do not use generic “make it simpler” advice; specify commands, defaults, errors, and acceptance evidence.
