# Curating skills

This guide covers the recurring work of building and maintaining a useful Skill Hub collection: collecting external sources, importing or creating draft skills, reviewing and editing them, controlling their lifecycle, and learning from upstream changes.

Complete the [README quickstart](../README.md#quickstart) first. For workspace setup, agent connections, moving machines, and troubleshooting, use the [general user guide](user-guide.md).

## Start with the hub, not a command

Ask your connected agent:

> Curate my Skill Hub.

The agent checks local status and recommends one next action. The CLI equivalent is:

```bash
skillhub status
```

Use the recommended action rather than running every workflow. Most sessions need only one of these paths:

| Goal | Path |
|---|---|
| Bring in complete skills from a repository | Collect source → import drafts → review → activate |
| Learn patterns from a repository without copying its skills | Collect source → check updates → distill → review inbox |
| Capture your own reusable workflow | Create draft → review and edit → activate |
| Improve an existing skill | Inspect → edit or apply an insight → review diff |
| Retire a skill | Deprecate → archive |

## Safety model

Skill Hub separates evidence, proposals, and active behavior:

- Watching a source does not change skills.
- Imported and newly created skills start as `draft`; agents cannot be routed to them.
- Mutating commands preview by default. `--yes` applies the freshly generated preview but never bypasses validation.
- MCP mutations use a pinned preview followed by explicit confirmation.
- Existing skill IDs are not overwritten during source import.
- Skill Hub writes workspace files but never runs `git commit` or `git push`.

Before confirming a change, verify its target, effect on active skills, and displayed diff. Use `skillhub diff` before committing workspace changes.

## Collect a source

A **source** is an upstream Git repository that may contain reusable skills or useful patterns. Collection has three stages so that saving a URL, deciding to trust it, and importing content remain separate decisions.

### 1. Save a candidate

Ask your agent:

> Save https://github.com/owner/repo as a source candidate because it contains useful testing and review patterns.

CLI:

```bash
skillhub source capture https://github.com/owner/repo.git \
  --reason "Useful testing and review patterns"
```

`capture` is idempotent and records the candidate locally without fetching or accepting it. Keep the returned candidate ID, such as `SRCQ-XXXXXXXXXXXX`.

### 2. Review and accept the candidate

Inspect saved candidates:

```bash
skillhub source list
skillhub source show SRCQ-XXXXXXXXXXXX
```

Then preview acceptance and choose a stable local source ID:

```bash
skillhub source triage SRCQ-XXXXXXXXXXXX \
  --decision accept \
  --source-id owner-repo \
  --adapter git
```

The source ID (`owner-repo`) identifies **which registered source** later commands use. It is not a repository path. For a large repository, limit the watched scope during onboarding:

```bash
skillhub source triage SRCQ-XXXXXXXXXXXX \
  --decision accept \
  --source-id owner-repo \
  --adapter git \
  --path skills
```

Here `--path skills` identifies **which subdirectory inside that source** is relevant.

### 3. Confirm onboarding

Acceptance prints a proposal ID, proposal digest, and base version. Confirm using those exact values:

```bash
skillhub source confirm \
  --proposal PROP-... \
  --proposal-digest sha256:... \
  --base-version sha256:...
```

The source is now watched and its first analysis can be prepared. Active skills remain unchanged.

## Import existing skills as drafts

Use this path when a watched source already contains skill folders you want to adopt.

Preview every discovered skill under a subdirectory:

```bash
skillhub source import owner-repo --path skills
```

Or narrow the preview to named skills:

```bash
skillhub source import owner-repo --path skills \
  --skill code-review \
  --skill testing
```

After reviewing importable skills and ID conflicts, apply the import:

```bash
skillhub source import owner-repo --path skills --yes
```

`owner-repo` selects the registered source; `--path skills` scopes discovery within that source. Imported skills retain source provenance, remain `draft`, and require separate review and activation.

## Create a skill from your own workflow

Use a draft to capture a recurring workflow that is not owned by an upstream source.

Ask your agent:

> Create a skill for reviewing reliability risks. Use it when I ask to review reliability, not when I ask to design a new service.

The agent previews a draft and asks for approval before creating it. CLI preview:

```bash
skillhub skill create --id reliability-review --collection software \
  --name "Reliability Review" \
  --description "Review reliability risks." \
  --trigger "review reliability" \
  --not-for "design a new service" \
  --min-scope multi_step
```

Re-run the command with `--yes` to generate, validate, and apply a fresh proposal. Add `--full-diff` to the preview when you need the complete file changes.

To start from prepared Markdown, pass `--content-file <file>`. Front matter is optional; if present, its `name` must match the skill ID. The file must be a readable regular file, not a symlink or directory.

## Review a draft before activation

List and inspect skills in any state:

```bash
skillhub skill list
skillhub skill list --state draft
skillhub skill show reliability-review --verbose
```

Review the draft as a routing contract, not only as prose:

1. **Purpose:** the name and description say what outcome the skill owns.
2. **Positive boundary:** triggers describe concrete requests that should load it.
3. **Negative boundary:** `not-for` entries prevent plausible misrouting. Use a rationale only when no honest negative boundary exists.
4. **Minimum scope:** `single_step`, `multi_step`, or `project` matches the smallest task that justifies loading the skill.
5. **Instructions:** the content is actionable, scoped, and free of source-specific assumptions that do not apply locally.
6. **Provenance:** for imported skills, the source, revision, and path are expected and trusted.
7. **Overlap:** compare active skills and remove ambiguous ownership before activation.

Activation validation requires at least one trigger, a `not-for` entry or rationale, and a minimum scope.

## Edit and improve a skill

Ask your agent for the intended outcome rather than dictating file edits:

> Tighten reliability-review so it handles service failure modes but not architecture design.

The agent previews the semantic change before applying it. CLI examples:

```bash
skillhub skill edit reliability-review \
  --description "Review failure modes and operational risks in a service."

skillhub skill edit reliability-review --editor
```

Without `--yes`, both commands preview. Add `--yes` only after reviewing the change. `--content-file <file>` replaces the instruction text. Routing flags such as `--trigger`, `--not-for`, `--operation`, and `--min-scope` replace only the field supplied; other fields remain unchanged.

For an active skill, editing changes active local content after confirmation. Review `skillhub diff` before committing.

## Activate, deprecate, and archive

A skill moves through this lifecycle:

```text
draft → active → deprecated → archived
```

- `draft`: under review; never recommended by the resolver.
- `active`: eligible for routing.
- `deprecated`: retained while being phased out.
- `archived`: retired and kept for history.

Preview lifecycle changes by omitting `--yes`; apply them after review:

```bash
skillhub skill activate reliability-review --yes
skillhub skill deprecate reliability-review --yes
skillhub skill archive reliability-review --yes
```

Transitions are ordered. An active skill must be deprecated before it can be archived.

## Learn from source updates

Use source learning when you want improvements rather than a direct copy of upstream skills.

Check sources explicitly:

```bash
skillhub check --all-due
skillhub check --all
skillhub check owner-repo
```

A check contacts watched sources and records revision changes; it never edits skills. Then ask your agent:

> Distill changed sources and show me the most valuable idea.

The agent prepares pinned source revisions, reads the source evidence, records findings, and proposes insights. Distillation does not apply proposals to active skills.

Review the ranked inbox:

```bash
skillhub inbox
skillhub insight show <insight-id>
```

For each insight, decide whether to plan, reject, mark obsolete, or reopen it. Rejection requires a reason:

```bash
skillhub insight decide <insight-id> \
  --decision reject \
  --reason "Already covered by the active reliability skill"
```

Applying an insight is a separate preview-and-confirm operation. Prefer the connected agent for this flow: it maps findings to exact artifacts, shows impact and a bounded diff, and stops for explicit approval. CLI details are available through `skillhub help insight`.

## Review and save workspace changes

Inspect canonical changes after an import, creation, edit, lifecycle transition, or accepted insight:

```bash
skillhub diff
skillhub validate
```

Commit only after the affected skills and routing metadata pass review:

```bash
git -C ~/skillhub add -A
git -C ~/skillhub commit -m "Curate Skill Hub skills"
```

Skill Hub never pushes automatically. Push through your normal Git workflow when you want the workspace backed up or shared.

## Command map

| Intent | Command |
|---|---|
| See hub state and one next action | `skillhub status` |
| Save a source candidate | `skillhub source capture <locator> --reason <text>` |
| List or inspect source records | `skillhub source list`; `skillhub source show <id>` |
| Decide whether to watch a source | `skillhub source triage <candidate-id> --decision ...` |
| Confirm source onboarding | `skillhub source confirm --proposal ... --proposal-digest ... --base-version ...` |
| Import draft skills | `skillhub source import <source-id> [--path <subdir>] [--skill <name>]... [--yes]` |
| Create or edit a draft | `skillhub skill create ...`; `skillhub skill edit <id> ...` |
| Inspect skills | `skillhub skill list`; `skillhub skill show <id> --verbose` |
| Change lifecycle state | `skillhub skill activate|deprecate|archive <id> [--yes]` |
| Check upstream revisions | `skillhub check --all-due|--all|<source-id>...` |
| Review proposed improvements | `skillhub inbox`; `skillhub insight show <id>` |
| Inspect workspace changes | `skillhub diff`; `skillhub validate` |

Run `skillhub help <command>` for the current flags and examples. Most commands also accept `--workspace <path>` and `--json`.
