# Skill Hub curation UX and CLI simplification report

Date: 2026-10-01. Status: design discussion consolidated; proposed commands are not implemented unless explicitly marked current.

## Decision to make

Define the smallest CLI that lets a person collect, add, create, inspect, edit, review, improve, activate, and retire skills without learning Skill Hub's storage model. The CLI must remain safe enough for agent automation and Git-backed team workflows. Its task model should translate directly into a future web UI rather than requiring a second product vocabulary.

## Executive verdict

The current safety mechanisms are stronger than the current user journey. Previewed mutations, canonical validation, immutable catalog generations, provenance, lifecycle states, and Git ownership are useful. The difficulty comes from exposing their implementation vocabulary and orchestration steps to users.

The clearest example is adding one skill from a URL. A user naturally copies a GitHub folder URL or chooses a local folder. The current CLI instead requires candidate capture, candidate ID handling, source triage, a manually chosen source ID, repository/path knowledge, exact proposal pins, source confirmation, and a later import. This is source-governance machinery presented as an installation workflow.

The recommended product model is:

```text
Use one skill                skillhub skill add <locator>
Create a local skill         skillhub skill create ...
Change a skill               skillhub skill edit <id> ...
Inspect or review a skill    skillhub skill show|review <id>
Change lifecycle             skillhub skill activate|deprecate|archive <id>
Learn from upstream          skillhub source watch <locator>
Check upstream               skillhub source check ...
Start from hub priorities    skillhub status
```

`skill add`, `source watch`, and `skill review` are proposed surfaces. They do not exist today.

## Outcome

A first-time user should be able to complete common curation jobs from intent and one locator:

1. Paste a GitHub skill-folder URL and receive a reviewable draft.
2. Point to a local skill folder and receive a reviewable draft.
3. Create a skill from an idea, edit it, validate it, and activate it.
4. Edit a skill through an agent, through `$EDITOR`, or directly as a plain file.
5. Understand when a change is structurally valid, semantically reviewed, active locally, committed, watched for updates, or merely proposed.
6. Use the same task vocabulary in CLI, MCP-driven agent conversations, documentation, and a future web UI.

## Constraints

- Canonical skill files remain plain files in a Git repository owned by the user.
- Direct file editing remains supported.
- New and imported skills remain drafts until explicitly activated.
- Upstream changes never overwrite local skill content automatically.
- Structural validation remains mandatory before a new catalog generation is published.
- Mutating managed operations retain preview/confirmation, stale-state checks, bounded diffs, atomic writes, and operation receipts.
- Network access remains explicit through the command the user invoked.
- Skill Hub never commits or pushes automatically.
- Convenience must hide derived inputs, not remove provenance or safety invariants.

## Non-goals

- Designing the web UI before the CLI contract is stable.
- Replacing Git branch protection or pull-request review with a second team approval system.
- Preventing users from editing files they own.
- Auto-activating imported skills.
- Auto-watching every imported locator.
- Encoding subjective semantic quality as a structural validator rule.

## User mental model

Users work with four primary nouns:

| Noun | User meaning |
|---|---|
| Skill | A reusable workflow the agent may follow. |
| Draft | A skill being reviewed; it is not eligible for routing. |
| Source | An upstream location watched for future learning. |
| Improvement | A proposed local change derived from evidence or user intent. |

The common path should not require users to understand candidate records, catalog generations, source IDs, proposal digests, base snapshots, adapter names, or internal operation IDs. These remain useful in verbose output, JSON, debugging, receipts, and recovery.

## Current behavior: evidence

### Source onboarding and import

The current CLI accepts `capture`, `list`, `show`, `triage`, `confirm`, and `import`; there is no `source add` or `source watch` command (`internal/delivery/cli/source.go:21-30`).

Accepting a candidate requires a caller-supplied safe source ID (`internal/app/source.go:250-253`). Adapter selection can be inferred, but a GitHub folder URL is not split into repository, ref, and subpath: `sourceLocator` places the original locator into `Repository` and only uses an explicitly supplied `path` (`internal/app/source.go:726-744`).

Captured filesystem locators are constrained to safe relative paths; absolute paths are rejected (`internal/app/source.go:684-703`). The filesystem adapter is rooted under the Skill Hub workspace (`internal/app/source.go:91-104`). Therefore the current source workflow does not satisfy the expected “paste any local skill folder” behavior.

Source import derives a target skill ID from `SKILL.md` front matter, then the skill folder name, then a fallback (`internal/app/source_import.go:152-187`). Existing IDs are skipped rather than overwritten (`internal/app/source_import.go:194-205`). These are suitable derivation and conflict rules for a simpler front door.

### Managed editing with `--editor`

`skillhub skill edit <id> --editor` reads current skill content, writes it to a permission-restricted temporary file, opens `$VISUAL` or `$EDITOR`, checks that the temporary file remains a regular non-symlink file, and reads the edited bytes (`internal/delivery/cli/skill.go:374-434`).

Only after the editor exits does the CLI construct an update and call `PreviewSkillUpdate` (`internal/delivery/cli/skill.go:120-145`). Without `--yes`, it returns a preview. With `--yes`, it confirms the freshly created proposal (`internal/delivery/cli/skill.go:155-170`). Managed updates therefore gain proposal validation, preview/confirmation, exact pins, atomic mutation, operation history, and a published catalog generation.

There is an important concurrency window: the original text is read before the editor opens, but `PreviewUpdate` loads current canonical state only after the editor closes. If another writer changes the skill while the first editor remains open, the later preview is fresh relative to the new base but may contain edited content derived from the old text. A subsequent confirmation can overwrite the other writer's content. Proposal pins protect the preview-to-confirm interval; they do not protect the editor-open-to-preview interval. Any redesign must either detect the original-content digest at preview time or explicitly accept and explain this behavior.

### Direct file editing

Direct editing changes canonical files immediately and creates no Skill Hub proposal, pins, managed diff, routing-impact summary, idempotency key, or operation receipt. Git still provides filesystem diff and history.

Direct editing does not bypass structural validation. When catalog inputs are stale, `EnsureFreshOrRebuild` validates canonical files before rebuilding (`internal/catalog/open.go:31-54`). The builder performs final canonical validation and input inventory immediately before swapping the verified generation (`internal/catalog/build.go:130-156`). Invalid canonical changes therefore do not publish a new generation.

What direct editing lacks is managed semantic review and transaction history, not structural validation. Valid direct edits to an active skill may become active after automatic catalog rebuild; current behavior does not require a separate semantic publish action.

### Git hooks

A local pre-commit hook can run `skillhub validate` and provide earlier feedback. It must not be treated as the primary safety boundary because hooks are local, can be bypassed, may not exist after clone, and run only at commit time.

A further caveat is partial staging: `skillhub validate --workspace ...` reads the working tree, not Git's index. It can pass while the staged commit is invalid, or fail because of unstaged edits even when the staged commit is valid. A hook may claim to validate the commit only if it materializes the staged snapshot into an isolated temporary workspace and validates that snapshot without changing the user's index or working tree.

### Existing strong entry points

`skillhub status` already provides a useful curation home with one ranked next action. Skill lifecycle vocabulary (`draft`, `active`, `deprecated`, `archived`) is understandable. Managed mutations preview by default and report that files were not changed. Source import preserves provenance and skips ID conflicts. These behaviors should be retained.

## Problems to solve

### P0 — The common add flow exposes internal source machinery

Current conceptual journey:

```text
capture locator
→ copy candidate ID
→ triage candidate
→ invent source ID
→ specify adapter/path
→ copy proposal pins
→ confirm source
→ import from source ID
→ inspect draft
→ activate
```

Expected journey:

```text
skill add <URL-or-folder>
→ inspect preview
→ confirm draft import
```

The internal records may still be created atomically. The user should not supply values Skill Hub can derive.

### P0 — Command noun does not match intent

A person adding a skill should use `skill`, not `source`. `source` should mean monitoring and learning from upstream. Reusing `source add` for direct skill installation would preserve the abstraction leak even if flags were removed.

### P0 — A generated scaffold can be activated while still containing placeholder instructions

Observed CLI smoke testing showed generated sections such as “Describe when your agent should choose this skill” and “First step,” after which activation succeeded. Routing metadata validation alone does not prove useful skill content. Creation should either generate no misleading placeholder content, require the user to supply real content, or reject unchanged scaffold content at activation.

### P1 — Terminology differs across surfaces

The same intent is described as capture/intake/save/collect, triage/accept/onboard/watch, and edit/apply/improve. User-facing language should converge on:

```text
Add a skill
Create a skill
Edit a skill
Review a skill
Activate or retire a skill
Watch a source
Review improvements
```

Internal MCP names may remain precise, but agent responses and human CLI output should translate them consistently.

### P1 — Managed and direct editing need a clear assurance model

Both paths receive structural validation before catalog publication. Managed editing additionally provides preview, pins, atomic mutation, and receipts. Documentation and UI should describe this difference accurately instead of implying direct editing is unsafe or unvalidated.

### P1 — Current `--editor` pins do not close the full editing race

The stale-proposal contract begins after editor exit. A useful fix is to carry the digest of the content initially copied into the editor and reject preview creation if that canonical content changed while the editor was open. The error should offer to reopen with current content or show a three-way diff; it must not silently overwrite.

### P1 — Human output still leaks placeholders and file counts

Observed output after activation suggested `git -C <ws> commit` rather than a runnable command with the actual workspace. `skillhub diff` labeled two changed files as “active skills (2),” which reads as two skills rather than two files. Human output should report task entities first and exact commands second; technical IDs belong in verbose or JSON output.

### P2 — MCP tools are safe but granular

The curator skill hides much of the MCP orchestration, but common flows still require several primitive tools. Keep primitives for recovery and exact control, while considering curated workflow tools for one-locator import and source watching. Tool consolidation must preserve explicit mutation boundaries.

## Recommended CLI contract

### Add a skill from one locator

```bash
skillhub skill add <locator>
skillhub skill add <locator> --yes
```

Accepted locators:

```text
https://github.com/owner/repo/tree/main/skills/code-review
https://github.com/owner/repo/blob/main/skills/code-review/SKILL.md
./skills/code-review
~/my-skills/code-review
/absolute/path/to/code-review
```

Expected behavior:

1. Detect URL versus filesystem path.
2. For GitHub URLs, derive canonical repository, ref, and skill folder.
3. Resolve refs against remote refs rather than naively splitting path segments, because ref names can contain `/`.
4. For local paths, resolve and inspect the directory without following unsafe symlinks.
5. Find `SKILL.md` and bounded companion files.
6. Derive skill ID from front matter, then folder basename.
7. Derive any internal source/provenance ID from repository owner/name or local content identity; do not require it as normal input.
8. Show skill identity, origin, files, conflicts, lifecycle result, and active-skill effect.
9. Import as `draft`; never activate automatically.
10. Do not enable monitoring unless explicitly requested.

If a locator contains no skill, return a concrete error. If it contains multiple skills, list them and require `--skill <id>` or `--all`; never import all silently. Existing IDs remain unchanged unless a separate explicit replacement/update workflow is designed.

### Watch an upstream source

```bash
skillhub source watch <locator>
skillhub source watch <locator> --yes
```

This command means “monitor and learn,” not “install a skill.” It may auto-derive a source ID and scope from a GitHub folder URL. Advanced flags may override derivation, but should not appear in the common example.

`skill add <locator> --watch` may compose import and monitoring in one preview, provided the preview clearly separates the draft import from the persistent network-monitoring effect. Default `skill add` does not watch.

### Create a local skill

Retain `skill create`, but avoid activatable placeholder content. A short form may infer name and default collection:

```bash
skillhub skill create reliability-review
```

The CLI should prompt only in an interactive terminal or return a concrete non-interactive command. Explicit automation flags remain available.

### Edit a skill

Retain these paths:

```bash
skillhub skill edit <id> --editor
skillhub skill edit <id> --content-file <file>
skillhub skill edit <id> --description "..."
```

For `--editor`, carry the original content digest through preview creation so a change made while the editor was open is detected. Without `--yes`, show the preview and one ready-to-run confirm command. With `--yes`, apply only the proposal created from that editor session.

Direct editing remains valid:

```bash
$EDITOR skills/<collection>/<id>/SKILL.md
skillhub validate
skillhub diff
```

Documentation must state that valid canonical edits can become active after catalog rebuild. Managed edit is recommended when users need preview, stale-state protection, routing impact, or an operation receipt.

### Review a skill

Proposed read-only surface:

```bash
skillhub skill review <id>
```

It should combine existing facts rather than introduce a new approval database:

- lifecycle state;
- content and metadata validity;
- uncommitted paths;
- routing field changes where a comparison base exists;
- overlaps detected by routing evaluation;
- provenance and watched-source state;
- whether the current version is active locally;
- exact next action.

Semantic warnings remain warnings unless a deterministic invariant exists.

### Validate and inspect changes

```bash
skillhub validate
skillhub diff
```

`validate` answers structural validity. `diff` answers what canonical files changed. Neither should claim a human approved semantic quality.

An optional hook installer may provide local feedback:

```bash
skillhub hooks install
```

It must be opt-in, preview its Git configuration change, remain read-only during commit, and describe whether it validates the working tree or a materialized staged snapshot. Team enforcement belongs in CI.

## Governance model

### Personal workspace

```text
add/create/edit
→ preview when managed
→ canonical validation
→ inspect diff
→ activate when ready
→ commit
```

Direct editing is acceptable. Git history is often sufficient governance.

### Shared workspace

```text
branch
→ add/create/edit
→ structural validation
→ routing/semantic review
→ pull request
→ CI validates staged repository state
→ human review
→ merge
```

Skill Hub should provide evidence and deterministic checks. Git hosting owns team approval and branch protection.

### Upstream improvements

```text
source check
→ distill findings
→ insight proposal
→ application preview
→ explicit approval
→ local canonical mutation
```

Upstream never overwrites local skill content. Local divergence is expected and must be preserved.

## Why CLI-first matters for the web UI

A future web UI should be a projection of the same task contract:

| CLI task | Web action |
|---|---|
| `skill add <locator>` | Paste URL or choose folder → preview draft |
| `skill create` | Create form/editor → preview draft |
| `skill edit --editor` | Skill editor → review changes |
| `skill review` | Review panel with validity, routing, provenance, and diff |
| `skill activate` | Lifecycle action with impact confirmation |
| `source watch` | Watch-source form with inferred repository and scope |
| `status` | Curation home with one recommended next action |

If the CLI requires candidate IDs, source IDs, adapter names, path extraction, and proposal hashes for ordinary tasks, the web layer must hide or re-orchestrate them independently. A simple semantic CLI lets web, MCP, and agent interfaces call the same application workflows.

## Recommended priorities

### P0

1. Implement `skill add <locator>` for one GitHub skill-folder URL and one local skill folder.
2. Auto-derive repository/ref/path, skill ID, and internal provenance identity.
3. Import only as draft; monitoring remains opt-in.
4. Prevent unchanged placeholder scaffolds from activation.

### P1

1. Add `source watch <locator>` as the monitoring-oriented front door.
2. Close the editor-open-to-preview lost-update window using an original-content digest.
3. Normalize user-facing vocabulary and next-action output.
4. Add a read-only `skill review <id>` composition surface.
5. Make diff counts represent skills and files unambiguously.

### P2

1. Add an optional validation-hook installer with explicit working-tree versus staged-snapshot semantics.
2. Consolidate high-value MCP workflows without removing recovery primitives.
3. Measure task completion steps and failures for the four primary journeys.

## Acceptance criteria for a redesigned CLI

### One-locator add

- A clean workspace can preview a draft from a GitHub skill-folder URL using one positional argument and no derived flags.
- The same command accepts a local relative, home-relative, or absolute skill folder.
- GitHub repository, ref, and path are correctly resolved, including refs containing `/`.
- Exactly one discovered skill needs no selection flag.
- Multiple discovered skills require an explicit selection or `--all`.
- Existing skill IDs are never overwritten silently.
- Confirmation creates a draft with provenance; active skills remain unchanged.

### Editing

- `--editor` does not mutate canonical files before confirmation.
- A concurrent canonical change made while the editor is open is detected before an overwrite proposal is accepted.
- Direct valid edits can rebuild into a new catalog generation; invalid edits cannot.
- Human output explains the difference between managed edit assurance and direct file ownership.

### Review and governance

- Structural validation, semantic warnings, lifecycle state, Git state, provenance, and active-local effect are named separately.
- Optional hooks never claim to validate a staged commit unless they validate an isolated staged snapshot.
- Shared-workspace guidance uses Git PR/CI rather than a duplicate approval store.

### Simplicity

- Common examples do not contain source IDs, candidate IDs, adapter names, proposal digests, base snapshots, or manually separated repository paths.
- Every preview ends with one recommended next action.
- Human output is concise by default; exact technical pins remain available in the runnable confirm command, verbose output, or JSON.
- CLI, agent instructions, MCP workflow descriptions, docs, and future web labels use the same primary verbs.

## Open decisions and recommended defaults

| Decision | Recommended default | Reason |
|---|---|---|
| Does `skill add` watch upstream? | No; require `--watch`. | Avoid hidden persistent network behavior. |
| What state is imported? | `draft`. | Review before routing. |
| How is skill ID chosen? | Front matter, then folder basename. | Matches current import behavior. |
| How are conflicts handled? | Preview and skip; never overwrite. | Preserves local ownership. |
| What if many skills are found? | Require `--skill` or `--all`. | Prevent surprising bulk mutation. |
| Is an absolute local path persisted? | Prefer content digest and safe display name; persist the path only for explicit watching with a portability warning. | Avoid leaking machine-specific personal paths into canonical Git history. |
| Is direct edit allowed? | Yes. | Plain files are a core product contract. |
| Does direct edit require semantic publish? | No under current behavior. | Structural validation is enforced; semantic approval is a separate proposed policy. |
| Should hooks install automatically? | No. | Git behavior changes require explicit opt-in. |

## Evidence index

- `internal/delivery/cli/source.go`: current source command grammar and orchestration.
- `internal/app/source.go`: candidate capture, source-ID requirement, adapter inference, locator construction, monitoring defaults, proposal confirmation.
- `internal/app/source_import.go`: skill discovery, ID derivation, conflict handling, provenance-aware import.
- `internal/delivery/cli/skill.go`: managed editor, proposal creation, `--yes`, confirmation.
- `internal/app/skill_lifecycle.go`: preview/update lifecycle boundary.
- `internal/catalog/open.go`: stale catalog detection and validation before rebuild.
- `internal/catalog/build.go`: final canonical validation before generation swap.
- `docs/curating-skills.md`: current user-facing curation workflow.
- `plans/reports/code-reviewer-260930-1045-ux-surface-audit.md`: prior empirical CLI and MCP UX findings.

## Handoff

Use the [companion independent-evaluation prompt](curation-ux-cli-independent-evaluation-prompt.md) for an independent review. The reviewer should challenge this recommendation rather than merely restating it. No implementation should begin until the command contract, safety boundaries, migration path, and acceptance criteria survive that review.
