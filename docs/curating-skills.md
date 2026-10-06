# Curating skills

This guide covers the recurring work of building and maintaining a useful Skill Hub collection: adding skills from remote repositories or local folders, creating draft skills, reviewing diagnostic facts, editing instructions, controlling lifecycle states, watching upstream sources, and learning from upstream changes.

Complete the [README quickstart](../README.md#quickstart) first. For workspace setup, agent connections, moving machines, and troubleshooting, use the [general user guide](user-guide.md).

## Start with the hub, not a command

Ask your connected agent:

> Curate my Skill Hub.

The agent checks local status and recommends one next action. The CLI equivalent is:

```bash
skillhub status
```

Use the recommended action rather than running every workflow. Most curation sessions follow one of two beginner journeys:

**Skill authoring and adoption journey:**
```text
status → skill add OR skill create → skill review / skill edit → activate
```

**Source monitoring and learning journey:**
```text
status → source watch → source check → distill / inbox
```

| Goal | Recommended path |
|---|---|
| Bring in a complete skill from a repository or folder | `skillhub skill add <locator> [--skill <name>] [--yes]` → review → activate |
| Capture your own reusable workflow | `skillhub skill create <id> ... [--yes]` → review and edit → activate |
| Inspect comprehensive diagnostic facts | `skillhub skill review <id>` |
| Edit instructions or routing metadata | `skillhub skill edit <id> [--editor] [--yes]` |
| Control lifecycle | `skillhub skill activate|deprecate|archive <id> [--yes]` |
| Watch an upstream repository for updates | `skillhub source watch <locator> [--yes]` → `skillhub source check --all-due` |
| Learn patterns without copying skills | `source watch` → `source check` → distill → review inbox |
| Advanced intake, staged governance, or recovery | `source capture` → `source triage` → `source confirm` → `source import` |

## Safety model

Skill Hub enforces durable boundaries between evidence, proposals, canonical files, and active behavior:

- **Drafts by default:** Newly added and newly created skills always start in `draft` state. They are never recommended by the resolver or loaded by agents until explicitly activated.
- **Content trust gate:** Skills originating from third-party sources (GitHub repositories, Git remotes, or upstream source imports) require explicit human content review before agents receive instructions or files. When unapproved, `local.status` is `review_required`, content is withheld, and file reads return `content_review_required`. Skills added directly from a local directory are trusted by design.
- **Approval is CLI-only:** Content approval is granted strictly via `skillhub skill edit <id> --approve-content <digest>`. The WebUI and MCP tools never expose an approval action, preventing cooperative agents or browser automation from self-approving untrusted code. Any change to skill instructions, scripts, assets, or runtime requirements resets approval and makes prior digests stale.
- **Cooperative security scope:** The content trust gate protects against accidental invocation by cooperative agents following host instructions. It is not an adversary boundary against rogue agents that possess direct filesystem access to the workspace directory.
- **Preview before confirm:** Mutating commands preview their proposed changes by default. Passing `--yes` confirms only the freshly generated preview; it never skips validation or blindly overrides conflicts.
- **Confirmation pins:** In the interactive CLI, users confirm proposals by short ID (`skillhub skill confirm <proposal-id>`). Automated interfaces and MCP tools require all three exact pins (`proposal_id`, `proposal_digest`, and `base_version`).
- **No silent overwrites:** If a skill or source identifier already exists in the workspace, operations halt with `skill_conflict` or `source_conflict`.
- **Local add is CLI-only:** Adding from a local filesystem path is interactive CLI-only. MCP tools reject raw local paths to maintain host security boundaries.
- **No local watch:** `source watch` supports remote Git repositories only. Local directories cannot be watched (`local_watch_unsupported`).
- **Watch is not a daemon:** Watching a source records monitoring configuration; it does not run a background daemon, poll automatically, or import skills. Upstream checks occur only when you or an authorized workflow run `skillhub source check`.
- **Review is diagnostic, not approval:** `skill review` compiles diagnostic facts (validation, readiness, resources, provenance, git status); it is not an approval gate and does not mutate lifecycle state.
- **Skill Hub never commits or pushes:** Skill Hub writes canonical workspace files but never executes `git commit` or `git push`. You inspect changes with `skillhub diff` and commit them when ready.
---

## Add a skill (`skillhub skill add`)

`skillhub skill add` is the intent-first front door for bringing in an existing skill. It inspects the locator, inventories resources, creates a proposal, and imports the skill as a `draft`.

### Add from a public GitHub repository

Add a single skill by folder URL:

```bash
skillhub skill add https://github.com/anthropics/skills/tree/main/skills/pdf --yes
```

When a repository contains multiple skills at the root or under a subpath, specify the skill name:

```bash
skillhub skill add https://github.com/anthropics/skills --skill pdf --yes
```

To import all skills discovered in the repository at once:

```bash
skillhub skill add https://github.com/anthropics/skills --all --yes
```

*Rules for multi-skill sources:*
- If a source contains multiple skills and you pass neither `--skill <name>` nor `--all`, the command halts with `skill_selection_required` and lists available skill names.
- `--all` imports every discovered skill as an independent draft. `--id` cannot be used with `--all`.
- You can specify `--ref <branch-or-tag>` and `--path <subpath>` explicitly if they are not part of the URL. Skill Hub queries fresh advertised refs from the remote repository.

### Add from a local folder (CLI only)

You can add a skill directly from a local directory:

```bash
skillhub skill add ./path/to/local-skill --yes
```

*Privacy and security boundaries for local add:*
- **CLI-only:** Local folder import is available exclusively through the CLI. MCP tools reject local filesystem locators (`./`, `../`, `~/`, absolute paths, `file://`) with `invalid_request` because the MCP server has no host-granted filesystem selection capability. Connected agents instruct the user to run `skillhub skill add` via the CLI.
- **Privacy-safe snapshots:** When reading a local directory, Skill Hub captures only the skill entrypoint (`SKILL.md`) and authorized companion resources (`references/`, `scripts/`, `assets/`). It ignores `.git`, hidden files, editor metadata, and files exceeding size limits.
- **Companion inventory:** The proposal inventories all companion files and total byte counts. If a skill exceeds configured limits, the operation halts with `resource_limits_exceeded`.
- **License warnings:** Upstream license indicators are detected and displayed as informational warnings. License detection is advisory and does not constitute formal legal clearance.

---

## Create a skill from your own workflow (`skillhub skill create`)

When you want to capture a custom workflow rather than importing from upstream, create a new draft:

```bash
skillhub skill create reliability-review \
  --collection software \
  --name "Reliability Review" \
  --description "Review service failure modes and operational risks." \
  --trigger "review reliability" \
  --not-for "design a new service" \
  --min-scope multi_step \
  --yes
```

*Creation requirements:*
- `--id`: Stable skill identifier (lowercase letters, digits, and hyphens).
- `--collection`: Collection folder name (e.g. `software`, `core`).
- `--name` and `--description`: Plain-language purpose.
- Routing metadata: `--trigger <text>`, `--not-for <text>` (or `--rationale <text>`), and `--min-scope single_step|multi_step|project`.
- Initial content: Pass `--content-file <path>` to seed instructions from an existing Markdown file. If omitted, Skill Hub seeds a draft template.
- *Scaffold guard:* Newly created skills containing untouched template placeholder text cannot be activated until genuine instructions are provided.

---

## Review diagnostic facts (`skillhub skill review`)

Before activating a draft or after modifying an existing skill, run a comprehensive diagnostic review:

```bash
skillhub skill review reliability-review
```

Add `--verbose` to view catalog snapshots, generation identifiers, and full git file lists:

```bash
skillhub skill review reliability-review --verbose
```

`skill review` is an offline, read-only diagnostic report compiled directly from canonical files without rebuilding the catalog. It reports:
1. **Lifecycle state:** `draft`, `active`, `deprecated`, or `archived`.
2. **Canonical validity:** Structural schema validation of `SKILL.md` and metadata.
3. **Routing eligibility:** Whether triggers, negative boundaries (`not-for`), and minimum scope are complete and non-conflicting.
4. **Served status:** Whether the skill is currently indexed and servable in the active SQLite catalog generation (`ServedFacts`).
5. **Resource status:** Total file count, byte size, entrypoint path, and companion resource inventory.
6. **Provenance:** Upstream repository locator, commit revision, and subpath, or local authoring designation.
7. **Git working-tree status:** Clean or dirty (staged, unstaged, untracked changes in the skill folder).
8. **Activation readiness:** Actionable warnings, such as untouched scaffold text or missing routing fields.
9. **Content trust & history changes:** For third-party skills, reports `content_trust` (`third_party`, `approved`, `content_digest`, `approve_command`) and `changes_since_approval`:
   - Lists added, removed, and modified files.
   - Flags whether `scripts_changed`, `runtime_changed`, or `dependencies_changed`.
   - Baseline commit is the oldest commit of the unbroken run of manifest commits carrying the recorded digest (walk capped at 200 commits).
   - Provides the exact `git diff <commit>..HEAD -- <skill-dir>` command to inspect changes.
   - Notes when history was truncated.
10. **Runtime hints:** Advisory static hints from `skillruntime.AnalyzeHints`: detected interpreters, dependency manifests, `missing_lockfiles`, absolute install paths, and install cues.
11. **Routing lint warnings:** Warnings for trigger collisions, generic triggers, missing examples, or near-duplicates.

The WebUI Review tab and Runtime tab show these same diagnostic facts, diff commands, and approve commands with no approve button.

`skill review` reports diagnostic facts to assist human decision-making. It does not store an approval flag and does not activate the skill.

---

## Edit and improve a skill (`skillhub skill edit`)

Update metadata, routing boundaries, or instruction text:

### Edit in your preferred external editor

Open the skill in `$VISUAL` or `$EDITOR`:

```bash
skillhub skill edit reliability-review --editor
```

Skill Hub opens a permission-restricted temporary copy (mode 0600) in your editor.

*Digest-pinned editor conflict semantics:*
- Before opening the editor, Skill Hub records the SHA-256 digest of the current canonical content.
- If another process or user modifies the canonical file while your editor session is open, Skill Hub detects the mismatch and refuses to preview or apply the edit, returning `edit_conflict`. This prevents lost updates.
- **24-hour recovery artifacts:** Whenever an editor session closes with changes, Skill Hub persists an immutable copy of your edited buffer to `runtime/edits/REC-<proposal-id>-<timestamp>.md` before previewing. If an edit conflict occurs or the proposal is cancelled, your work is preserved in this recovery file. Recovery files have a 24-hour TTL and are cleaned up automatically upon proposal confirmation or expiration.

### Edit via flags or content file

You can update fields directly from the command line:

```bash
skillhub skill edit reliability-review \
  --description "Review failure modes, resilience policies, and operational risks." \
  --yes
```

To replace the instruction text directly from a file:

```bash
skillhub skill edit reliability-review \
  --content-file ./updated-instructions.md \
  --yes
```

*Explicit blind replacement:* Passing `--content-file <file>` without an editor session performs a standard preview against the current canonical base version without expecting an editor-captured digest.

Routing flags (`--trigger`, `--not-for`, `--operation`, `--min-scope`) replace only the specified fields; unmentioned routing fields retain their existing values.

### Add routing examples and counter-examples

Provide natural task phrasings to guide resolver matching:

```bash
skillhub skill edit reliability-review \
  --example "review message consumer for idempotent message processing" \
  --counter-example "design a new event-driven service topology" \
  --yes
```

### Attach or remove runtime specifications

Declare executable dependencies, environment variables, and preflight commands:

```bash
skillhub skill edit reliability-review --runtime-file ./runtime.yaml --yes
skillhub skill edit reliability-review --runtime-file '{}' --yes   # removes runtime block
```

The System Curator skill will propose runtime blocks with pinned versions and lockfiles. Any change to instructions, files, or runtime specifications resets content approval for third-party skills.

### Confirming an edit proposal

When run without `--yes`, `skill edit` prints a preview with a diff and a short confirmation command:

```bash
skillhub skill confirm PROP-XXXXX
```

---

## Activate, deprecate, and archive

Skill lifecycle transitions follow a strict sequence:

```text
draft → active → deprecated → archived
```

- **`draft`:** Being authored, reviewed, or imported. Not eligible for agent routing.
- **`active`:** Validated, reviewed, and eligible for resolver recommendations.
- **`deprecated`:** Retained for backward compatibility while being phased out.
- **`archived`:** Retired and kept for historical audit; not servable.

Preview transitions by omitting `--yes`; apply them after reviewing routing impact:

```bash
skillhub skill activate reliability-review --yes
skillhub skill deprecate reliability-review --yes
skillhub skill archive reliability-review --yes
```

Transitions are strictly ordered. An active skill cannot jump directly to archived; it must be deprecated first. If activation validation fails (e.g. missing triggers or untouched scaffold), the transition is blocked with `validation_failed`.

---

## Keep vendored skills up to date

When you add skills from a remote Git repository (`skillhub skill add https://github.com/owner/repo --skill pdf --yes`), Skill Hub automatically records the repository as the skill's upstream source. It captures the repository URL, branch or tag ref, commit SHA, relative directory path, and initial `files_digest`.

### Check for upstream drift (`skillhub skill outdated`)

Check whether vendored skills have upstream updates available:

```bash
skillhub skill outdated --check
```

- `--check`: Probes remote repositories via `ls-remote` to check current branch commits before reporting.
- `--all`: Includes skills that are up to date in the output table.
- `--exit-code`: Exits with code `1` when any skill needs attention (`update_available`, `diverged`, `upstream_removed`), and `0` when clean.
- `--json`: Emits machine-readable JSON status details.

The output table reports:
- **SKILL**: The skill ID.
- **UPSTREAM**: The source repository and ref.
- **STATUS**: The drift classification:
  - `up_to_date`: Local files match the latest upstream commit.
  - `update_available`: Upstream has newer commits that change files in the skill directory, with no conflicting local edits.
  - `modified`: Local files have been edited since vendoring, but upstream has not changed.
  - `diverged`: Both upstream and local files have changed since vendoring; a 3-way merge is needed.
  - `upstream_removed`: The skill folder was removed upstream.
  - `pinned`: The skill tracks a fixed commit rather than a moving branch ref.
  - `unavailable`: The remote repository cannot be reached.
  - `untracked`: The skill has no upstream repository attached.
- **LOCAL**: Whether local files match `files_digest` (`clean` or `edited`).
- **BEHIND**: The count of changed files in the skill folder and the upstream commit date.

### Inspect upstream details (`skillhub skill upstream`)

Inspect full upstream repository metadata, commit SHAs, file lists, and drift for an individual skill:

```bash
skillhub skill upstream pdf --check
```

### Apply updates with 3-way merge (`skillhub skill update`)

When an update is available, merge changes from upstream into your local skill:

```bash
skillhub skill update pdf
```

Skill Hub executes a 3-way merge using the system `git merge-file` engine between the original base commit, your current local files, and the newest upstream commit.

The update displays a file status table:
- **Status flags:** `merged` (cleanly integrated changes), `modified` (applied upstream changes), `added` (new upstream files), `deleted` (files deleted upstream), or `conflicted` (overlapping changes requiring resolution).
- **Conflict resolution options:**
  - `--accept <path>=upstream|local|merged`: Choose how to resolve a file.
  - `--manual <path>=<file>`: Supply an edited resolution file for a conflicted path.
  - `--write-conflicts <dir>`: Write conflicted files with standard Git conflict markers to a directory for manual editing.
  - `--yes`: Apply a clean update immediately.

If conflicts exist, Skill Hub refuses to build confirmation pins until every conflict is explicitly resolved. Once clean or resolved, confirm the proposal:

```bash
skillhub skill confirm PROP-123
```

### Re-approval step (updates never approve content)

Applying an upstream update updates files and metadata, but **never approves content automatically**. Because updated code or instructions are untrusted third-party changes, the skill transitions to `review_required` and connected agents cannot use it until you re-review and approve:

```bash
skillhub skill review pdf
skillhub skill edit pdf --approve-content <digest>
```

### Backfill legacy skills (`skillhub source backfill`)

Skills vendored before Skill Hub's upstream tracking model lack `provenance.source_id` and `origin.files_digest`. Attach source records and compute missing digests using `source backfill`:

```bash
skillhub source backfill --yes
```

If multiple candidate paths exist within an upstream repository, specify `--skill <id> --path <subdir>` to resolve ambiguity.

*Binary compatibility note:* Workspaces containing skills with `provenance.origin.files_digest` require Skill Hub binaries that support upstream provenance. Older binaries will reject the unknown field during strict YAML validation.

---

## Learn from references

Use learning references when you want curator agents to monitor repositories for patterns, architectural ideas, or domain guidance without vendoring whole skills directly.

### Attach and detach learning references

Attach an existing source or new repository to a skill as a learning reference:

```bash
skillhub source attach https://github.com/owner/reference-repo --skill-id my-skill --yes
```

To remove a learning link:

```bash
skillhub source detach reference-repo --skill-id my-skill --yes
```

### Watch a repository for a skill (`skillhub source watch`)

Register a new repository to watch for a specific skill:

```bash
skillhub source watch https://github.com/owner/reference-repo --skill-id my-skill --cadence weekly --yes
```

- **The No-Orphan Invariant:** Every watched source must be linked to at least one skill. Watching a repository without `--skill-id` is refused with actionable guidance.
- To completely stop watching a source and delete its monitoring record if unreferenced by other skills:
  ```bash
  skillhub source unwatch reference-repo --yes
  ```

### Triage candidate sources (`skillhub source triage`)

When reviewing intake candidates recorded with `skillhub source capture`:

```bash
# Accept as a learning reference for an existing skill
skillhub source triage SRCQ-12345 --decision accept --skill-id my-skill

# Accept as a learning reference for a new skill draft
skillhub source triage SRCQ-12345 --decision accept --new-skill new-skill

# Vendor skills directly from candidate repository
skillhub source triage SRCQ-12345 --decision import --path skills

# Defer or reject candidates
skillhub source triage SRCQ-12345 --decision defer --reason "Revisit next quarter"
skillhub source triage SRCQ-12345 --decision reject --reason "Incompatible license"
```

Triage `accept` always requires a skill target (`--skill-id` or `--new-skill`), enforcing the no-orphan rule.

### Check for updates (`skillhub source check`)

Check watched sources to detect upstream revision changes:

```bash
skillhub source check --all-due
skillhub source check --all
skillhub source check owner-repo
```

*Top-level compatibility spelling:* `skillhub check` is fully supported as an exact alias of `skillhub source check`. When skills track an upstream repository, running check also refreshes per-skill upstream drift.

### Distill changes and review the inbox

When learning sources have new revisions, run `skillhub distill` or ask your connected curator agent:

> Distill changed sources and show me the most valuable idea.

The agent analyzes changes, creates findings and comparisons, and submits insight proposals to your inbox. Inspect and decide insights:

```bash
skillhub inbox
skillhub insight show INS-101
skillhub insight decide INS-101 --decision plan --reason "Incorporate security patterns"
```

Applying an insight generates a pinned preview. Approve the proposal only after inspecting the diff.

## Resource-verified fallback and state basis

Skill Hub maintains a clear distinction between two layers of state:

1. **Canonical state:** The validated YAML, Markdown, and resource files stored in your local Git repository (`skills/`, `sources/`, `distill/`).
2. **Served state:** The compiled SQLite search catalog (`runtime/catalog/generations/<gen>.db`) used for sub-millisecond agent routing.

### Resource fallback behavior

When canonical files are modified by hand or an external Git merge:
- **Unchanged skills:** If a skill's files are untouched and match recorded digests, the skill remains servable from the catalog, though diagnostics may note that the catalog generation is stale.
- **Modified or missing resources:** If companion resource files have changed or been deleted in the working tree, they become unavailable (`resource_content_unavailable`). Historical bytes are **never guessed, synthesized, or restored from stale caches**.
- **Mutation blocking:** If canonical workspace files fail validation, managed mutations (`skill create`, `skill edit`, `skill activate`, etc.) are blocked with `workspace_invalid` or `validation_failed` until canonical files are corrected.
- **Fresh-clone semantics:** When a workspace is cloned onto a new machine, no runtime database exists. Running `skillhub rebuild` or any read command triggers validation and compiles a fresh catalog generation directly from canonical files.
- **Degraded MCP startup:** If the catalog database is missing or corrupt when an agent starts `skillhub mcp serve`, the server launches in degraded fallback mode. Diagnostic tools (`hub_status`, `skill_review`, `workspace_validate`, `workspace_rebuild`) remain operational so the agent can diagnose and repair the hub, while routing tools return actionable errors without crashing the stdio transport.

---

## Review and commit workspace changes

Skill Hub never commits or pushes Git repositories automatically. Inspect canonical changes and validate files before committing:

```bash
skillhub diff
skillhub validate
```

### Staged index validation (`validate --staged`)

Before committing, validate the exact files staged in the Git index:

```bash
skillhub validate --staged
```

*Staged validation guarantees:*
- **Literal index blobs:** It reads literal stage-0 blobs directly from the Git index without applying working-tree filters or checkout modifications.
- **Zero side-effects:** It never mutates the Git index or working tree.
- **No hook installer:** Skill Hub does not include a proprietary hook installer (`skillhub hook install` does not exist).
- **Pre-commit integration:** You can add this command directly to any Git hook manager (Husky, Lefthook, pre-commit, or `.git/hooks/pre-commit`):

```bash
#!/bin/sh
skillhub validate --staged
```

### Commit your changes

Once validation passes, commit your changes using standard Git:

```bash
git -C ~/skillhub add -A
git -C ~/skillhub commit -m "Curate Skill Hub skills"
```

Push to your remote backup using standard `git push`.


## Measure usage (`skillhub telemetry funnel`)

Analyze how skills are recommended, activated, loaded, and evaluated over time:

```bash
skillhub telemetry funnel                         # last 30 days overall
skillhub telemetry funnel --since 90d --json      # last 90 days JSON output
skillhub telemetry funnel --skill reliability-review
```

The funnel report measures:
- Resolutions by status (`resolved`, `no_skill`, `needs_context`, `already_covered`, `failed`).
- Recommendations (primary vs supporting).
- Activations by attribution class (`recommended`, `supporting`, `override`, `after_no_skill`, `after_needs_context`, `unsolicited`).
- Overall and per-skill acceptance rates (`activation:recommended / recommended:primary`).
- Content loads by resource kind, and blocked loads (`blocked:review_required`).
- Diagnostic lists: dead skills (zero recommendations or loads), recommended never activated, and skills blocked by review ("approve these to unlock demand").
- Terminal doctor checks and failure rates.
- Feedback outcomes and setup failure rates (`feedback:setup_failed`). Negative feedback is deduplicated so a combined failed outcome and harmful utility report counts once.

### Import local Claude Code transcripts

Import local execution transcripts as ground-truth telemetry:

```bash
skillhub telemetry import-transcripts --project ~/my-project
```

**Privacy boundary:** Transcript import scans only tool invocation blocks (`tool_use`). It records only the tool name, skill ID, timestamp, and a 16-byte session hash. Task text, conversation messages, file paths, and code snippets are never read or stored. The import is clamped to a 14-day window.

---

## Check routing quality (`skillhub eval routing`)

Evaluate the deterministic resolver across all active skills' examples and counter-examples using leave-one-out cross-validation:

```bash
skillhub eval routing
skillhub eval routing --no-skill testdata/routing/no-skill-v1.yaml
skillhub eval routing --policy custom-policy.yaml --min-precision 0.60 --min-recall 0.58 --max-fpr 0.13
```

- **Leave-one-out:** For each test case, the tested example is removed from the skill's candidate scoring set to measure true generalization.
- **Metrics:** Reports Precision@1, Recall, No-Skill Recall, No-Skill Precision, and False Positive Rate.
- **CI Gate:** Automated as a `make check` gate test (`TestRoutingEvalGate`) executing in under 1 second.

## Advanced intake and recovery

The original multi-step source intake commands remain fully supported for advanced workflows, formal compliance review, and recovery. They are not required for everyday skill curation.

```bash
# 1. Record an intake candidate without network access
skillhub source capture https://github.com/owner/repo.git --reason "Audit candidate"

# 2. Inspect candidates
skillhub source list
skillhub source show SRCQ-XXXXXXXXXXXX

# 3. Triage candidate and preview onboarding (targeting a skill or importing)
skillhub source triage SRCQ-XXXXXXXXXXXX \
  --decision accept \
  --skill-id my-skill \
  --source-id audit-repo \
  --adapter git \
  --path skills
# 4. Confirm onboarding with exact pins
skillhub source confirm \
  --proposal PROP-YYYYY \
  --proposal-digest sha256:... \
  --base-version sha256:...

# 5. Import discovered skills as drafts
skillhub source import audit-repo --path skills --yes
```

Use this workflow when you require multi-stage governance, audit logging of candidate intake reasons, or precise scope isolation before watching.

---

## Command map

| Intent | Command |
|---|---|
| See hub status and recommended next action | `skillhub status` |
| Add a skill from GitHub or local folder | `skillhub skill add <locator> [--skill <name>\|--all] [--yes]` |
| Create a new draft skill | `skillhub skill create <id> --collection <c> --name <n> --description <d> [flags] [--yes]` |
| Run comprehensive diagnostic review | `skillhub skill review <id> [--verbose]` |
| Approve third-party skill content | `skillhub skill edit <id> --approve-content <digest>` |
| Edit skill instructions or routing | `skillhub skill edit <id> [--editor\|--content-file <f>] [flags] [--yes]` |
| Attach or remove runtime specification | `skillhub skill edit <id> --runtime-file <yaml>` |
| Add routing examples or counter-examples | `skillhub skill edit <id> --example <t> / --counter-example <t>` |
| Test skill machine requirements | `skillhub skill doctor <id> [--json]` |
| Manage secret environment variables | `skillhub skill env set\|unset\|list <id> [<KEY>]` |
| Confirm a proposed mutation | `skillhub skill confirm <proposal-id>` |
| List skills | `skillhub skill list [--state <s>]` |
| Read a skill (any state) | `skillhub skill show <id> [--verbose]` |
| Change lifecycle state | `skillhub skill activate\|deprecate\|archive <id> [--yes]` |
| Check skill drift from upstream | `skillhub skill outdated [--check] [--all] [--exit-code] [--json]` |
| Inspect upstream repository details | `skillhub skill upstream <id> [--check] [--json]` |
| Apply 3-way upstream update | `skillhub skill update <id> [--yes] [--json]` |
| List candidates, sources, groups | `skillhub source list [--status <s>]` |
| Watch repository for a skill | `skillhub source watch <locator> --skill-id <id> [--cadence <c>] [--yes]` |
| Attach learning reference | `skillhub source attach <source-id\|url> --skill-id <id> [--yes]` |
| Detach learning reference | `skillhub source detach <source-id> --skill-id <id> [--yes]` |
| Stop watching source | `skillhub source unwatch <source-id> [--yes]` |
| Backfill legacy provenance | `skillhub source backfill [--skill <id>] [--yes]` |
| Check watched sources for updates | `skillhub source check --all-due\|--all` (or `skillhub check ...`) |
| Review insight proposals in inbox | `skillhub inbox`; `skillhub insight show <id>` |
| Decide an insight proposal | `skillhub insight decide <id> --decision plan\|reject [flags]` |
| Import skills from source | `skillhub source import <source-id> [--path <subdir>] [--skill <name>] [--yes]` |
| Measure funnel usage and conversion | `skillhub telemetry funnel [--since <period>] [--skill <id>]` |
| Import local Claude Code transcripts | `skillhub telemetry import-transcripts --project <dir>` |
| Evaluate routing quality | `skillhub eval routing [--no-skill <f>] [--policy <f>]` |
| Show uncommitted canonical changes | `skillhub diff` |
| Validate working-tree files | `skillhub validate` |
| Validate staged Git index files | `skillhub validate --staged` |
| Rebuild derived search catalog | `skillhub rebuild [--verbose]` |
| Diagnose and repair connections | `skillhub doctor [--fix [--yes]]` |
Run `skillhub help <command>` for detailed flag specifications and examples. Most commands accept `--workspace <path>` and `--json`.
