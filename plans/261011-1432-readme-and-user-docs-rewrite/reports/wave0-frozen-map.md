# Wave 0 frozen map

Input for every wave 1 and wave 2 writer. Everything below was run on 2026-10-11 with one binary built from
branch `docs/readme-and-guides-rewrite` at `fb9a1b4` (main plus plan files). Writers link only to the anchors
listed here and copy commands only from the transcripts here or from their own sandbox runs.

## 1. Binary, sandbox and captures

- Binary: `/tmp/skillhub-docs-rewrite/bin/skillhub` (`make web-build`, then
  `go build -o /tmp/skillhub-docs-rewrite/bin/skillhub ./cmd/skillhub`). Do not rebuild it before Phase 11.
- `skillhub version` prints `skillhub version:  dev`, `commit: none`, `built: unknown`, `dirty: unknown`. The build
  does not stamp a version. Docs never print the version string; show only the command `skillhub version`.
- Sandbox env script: `/tmp/skillhub-docs-rewrite/wave0/env.sh`. Use it as
  `SB=/tmp/skillhub-docs-rewrite/<phase-slug>; . /tmp/skillhub-docs-rewrite/wave0/env.sh`. It sets `HOME`,
  `USERPROFILE`, all `XDG_*` (including `XDG_RUNTIME_DIR`) under `$SB`, puts the binary first on `PATH`, unsets
  `SKILLHUB_WORKSPACE`, and writes a sandbox Git identity with `init.defaultBranch main`.
- Help captures: `/tmp/skillhub-docs-rewrite/help/` holds `help.txt` (top level) and `help-<cmd>.txt` for init,
  connect, doctor, status, skill, source, check, resolve, mcp, serve, validate, rebuild, migrate, diff,
  telemetry, eval, resolution, version, update. `skillhub <group> <sub> --help` and `skillhub help <group> <sub>`
  print the same text as the group help, so there is one file per group, not per subcommand.
- MCP stdio probe: `/tmp/skillhub-docs-rewrite/wave0/mcp_probe.py` (reads `.mcp.json` from the folder given,
  starts the server, runs `initialize`, `tools/list`, `skill_resolve`, `skill_get`). Run it with `python3 -I`.

## 2. Doc set and owners

Unchanged from plan.md ("Doc set" and "Parallel execution" tables). No fact forced a change to the file set or
the budgets. Content changes forced by facts are listed in section 8 (notably: `skillhub inbox` and
`skillhub insight` no longer exist).

## 3. Frozen H2 lists (new files)

Anchors are GitHub slugs. Writers keep the heading text exactly; other pages link these anchors.

### `docs/getting-started.md` (Phase 2)

| H2 | Anchor |
|---|---|
| Install Skill Hub | `#install-skill-hub` |
| Create your workspace | `#create-your-workspace` |
| Connect your agent | `#connect-your-agent` |
| Create your first skill | `#create-your-first-skill` |
| See your first win | `#see-your-first-win` |
| See it in the web UI | `#see-it-in-the-web-ui` |
| If it did not work | `#if-it-did-not-work` |
| Next steps | `#next-steps` |

### `docs/installation.md` (Phase 3)

| H2 | Anchor |
|---|---|
| Install with one command | `#install-with-one-command` |
| Choose options | `#choose-options` |
| Check signatures | `#check-signatures` |
| Upgrade | `#upgrade` |
| Uninstall | `#uninstall` |
| Build from source | `#build-from-source` |

"Supported platforms" is one line, not a heading. Go version for "Build from source": `go.mod` says `go 1.26.0`.

### `CONTRIBUTING.md` (Phase 4)

| H2 | Anchor |
|---|---|
| Build and test | `#build-and-test` |
| Keep tests isolated | `#keep-tests-isolated` |
| Release | `#release` |
| Design records | `#design-records` |

### `docs/web-ui.md` (Phase 5)

| H2 | Anchor |
|---|---|
| Start the web UI | `#start-the-web-ui` |
| Do common tasks | `#do-common-tasks` |
| Keep it safe on your network | `#keep-it-safe-on-your-network` |

H3s under "Do common tasks" (frozen; the text of the signed-off walk-through is kept, so "Link and check sources"
stays instead of the proposed "Check sources"): See what needs your attention; Find a skill; Check one skill;
Activate, deprecate or archive a skill; Fix a skill's description and routing fields; Create a skill; Add skills
from GitHub; Act on upstream updates; Manage one skill's sources; Link and check sources; Hand a source to your
curator agent; Change the theme. Their anchors are `#see-what-needs-your-attention`, `#find-a-skill`,
`#check-one-skill`, `#activate-deprecate-or-archive-a-skill`, `#fix-a-skills-description-and-routing-fields`,
`#create-a-skill`, `#add-skills-from-github`, `#act-on-upstream-updates`, `#manage-one-skills-sources`,
`#link-and-check-sources`, `#hand-a-source-to-your-curator-agent`, `#change-the-theme`.

### `docs/troubleshooting.md` (Phase 6)

| H2 | Anchor |
|---|---|
| Start with doctor | `#start-with-doctor` |
| Setup and connection | `#setup-and-connection` |
| Skills | `#skills` |
| Workspace and index | `#workspace-and-index` |
| Windows | `#windows` |

Symptom H3s are not frozen (no other page links them); getting-started links only the H2 anchors above.

### `docs/sources-and-learning.md` (Phase 7)

| H2 | Anchor |
|---|---|
| Keep copied skills up to date | `#keep-copied-skills-up-to-date` |
| Learn from other repositories | `#learn-from-other-repositories` |
| Import many skills from a repository | `#import-many-skills-from-a-repository` |
| Triage candidate repositories | `#triage-candidate-repositories` |

Content correction: the "review lessons in the inbox" step is replaced by the verified flow (section 8, item 1).

### `docs/command-reference.md` (Phase 8)

| H2 | Anchor |
|---|---|
| Set up | `#set-up` |
| Work with skills | `#work-with-skills` |
| Sources and learning | `#sources-and-learning` |
| Workspace and Git | `#workspace-and-git` |
| Routing and usage | `#routing-and-usage` |

Row corrections: drop `inbox` and `insight` (not commands); add `skillhub check` (top-level, `source check` is
its alias); `integrate` works as an alias of `connect` but `skillhub help integrate` and `integrate --help` both
fail, so mention it only as "also `integrate`" in the connect row.

### `README.md` (Phase 9) and `docs/README.md` (Phase 15)

README H2s: Why Skill Hub (`#why-skill-hub`), How it works (`#how-it-works`), Quickstart (`#quickstart`), See it
(`#see-it`), Works with (`#works-with`), Documentation (`#documentation`), Contributing and license
(`#contributing-and-license`).
docs/README.md H2s: Start here (`#start-here`), Guides (`#guides`), Reference (`#reference`), For maintainers
(`#for-maintainers`).

## 4. Surviving anchors and stubs (old files)

Kept headings (text unchanged):

- `README.md#quickstart`
- `docs/user-guide.md`: `#concepts-in-plain-words`, `#set-up-and-connect-agents`, `#connect-one-project-recommended`,
  `#connect-every-project-at-once`, `#what-gets-written`, `#undo-a-connection`, `#how-the-agent-picks-a-skill`,
  `#run-skills-that-need-tools-or-secrets`, `#move-to-another-machine`
- `docs/curating-skills.md`: `#start-with-the-hub-not-a-command`, `#safety-model`, `#activate-deprecate-and-archive`,
  `#review-and-commit-workspace-changes`; the H2s that carry a command in backticks keep their exact text too
  (for example "Add a skill (`skillhub skill add`)" gives `#add-a-skill-skillhub-skill-add`).

Stubs (heading text unchanged, one line `Moved to [<Title>](<file>.md#<anchor>).`):

| File | Stub heading | Slug | Points to |
|---|---|---|---|
| user-guide.md | `## Curate skills` | `#curate-skills` | `curating-skills.md` |
| user-guide.md | `## Local Web UI` | `#local-web-ui` | `web-ui.md` |
| user-guide.md | `### Do common tasks in the Web UI` | `#do-common-tasks-in-the-web-ui` | `web-ui.md#do-common-tasks` |
| user-guide.md | `## Troubleshooting` | `#troubleshooting` | `troubleshooting.md` |
| user-guide.md | `## Command cheat sheet` | `#command-cheat-sheet` | `command-reference.md` |
| curating-skills.md | `## Keep vendored skills up to date` | `#keep-vendored-skills-up-to-date` | `sources-and-learning.md#keep-copied-skills-up-to-date` |
| curating-skills.md | `## Learn from references` | `#learn-from-references` | `sources-and-learning.md#learn-from-other-repositories` |
| curating-skills.md | ``## Import repository skills as drafts (`skillhub source import`)`` | `#import-repository-skills-as-drafts-skillhub-source-import` | `sources-and-learning.md#import-many-skills-from-a-repository` |
| curating-skills.md | `## Advanced intake and recovery` | `#advanced-intake-and-recovery` | `sources-and-learning.md#triage-candidate-repositories` |
| curating-skills.md | `## Resource-verified fallback and state basis` | `#resource-verified-fallback-and-state-basis` | `troubleshooting.md#workspace-and-index` (plus `contracts/`) |
| curating-skills.md | `## Command map` | `#command-map` | `command-reference.md` |
| release-runbook.md | `## Install, upgrade, and uninstall` (kept, one-line link) | `#install-upgrade-and-uninstall` | `installation.md` |

## 5. Glossary (frozen)

The plan.md glossary stands as written, with these notes from the runs:

- **skill**: verified shape is `skills/<collection>/<skill-id>/SKILL.md` plus `.meta/skill.yaml` (the "small file
  that says when it applies"). The definition stands.
- **agent**: CLI output itself uses "host" ("Host approval is required before activation.", "Host activation
  coordination is instruction-only ..."). Quote output verbatim in `text` blocks; never adopt the word in prose.
- **recommendation**: `skillhub resolve` prints "Resolved commit-style (medium confidence)". Do not explain
  "confidence" levels beyond that line (not verified).
- **lesson**: lessons live in the skill's `.meta/distill.yaml` and are read in the skill's **Distill** tab of the
  web UI. There is no `inbox` or `insight` command; do not use "inbox" as a product term.
- **curator agent**: the distill-lab skill is not shipped with Skill Hub (only `system-curator` is). Say "an agent
  that has the distill-lab skill"; do not say where to get it (unverified).

## 6. Brand file names (Phase 10)

`docs/images/brand/skill-hub-logo-light.svg`, `docs/images/brand/skill-hub-logo-dark.svg`,
`docs/images/brand/skill-hub-mark-light.svg`, `docs/images/brand/skill-hub-mark-dark.svg`.
"light" means "for light backgrounds"; "dark" means "for dark backgrounds".

## 7. Verified first-win transcript

All commands ran in `SB=/tmp/skillhub-docs-rewrite/wave0` (HOME `$SB/home`), so `~` below is the sandbox HOME.
Output is real and trimmed only where marked; sandbox paths are shown as `<home>`.

### Create the workspace

```bash
skillhub init ~/skillhub --yes
```

```text
Workspace created at <home>/skillhub.
Agent connection for this workspace written: claude-code, codex-cli, gemini-cli.

Next:
1. Commit the workspace: git -C <home>/skillhub add -A && git -C <home>/skillhub commit -m "Initialize Skill Hub workspace"
2. Connect another project: cd <project> && skillhub connect --workspace <home>/skillhub --yes
(add -g instead to connect every project: skillhub connect -g --workspace <home>/skillhub --yes)
3. Open your agent there and ask: "curate my Skill Hub"
```

Docs must not copy hint 1 (it says `add -A`). Files `init` wrote (`git status --porcelain -uall`, all untracked):

```text
.agents/skills/system-curator/SKILL.md
.claude/settings.local.json
.claude/skills/system-curator/SKILL.md
.codex/config.toml
.gemini/settings.json
.gemini/skills/system-curator/SKILL.md
.gitignore
.mcp.json
.skillhub/schema-version
AGENTS.md
CLAUDE.md
GEMINI.md
```

Files that contain the absolute sandbox path (`grep -rl "$SB"`): `.mcp.json`, `.claude/settings.local.json`,
`.gemini/settings.json`, `.codex/config.toml`. `.gitignore` holds only `/runtime/` and `/.skillhub/transactions/`.
`runtime/` (catalog, databases, lock) exists but is ignored. `skills/`, `sources/`, `history/`, `registry/`,
`config/`, `distill/`, `evals/` exist as empty folders.

### Safe add list (frozen)

Commit only the workspace's own files. The connection files (`.mcp.json`, `.claude/`, `.codex/`, `.gemini/`,
`.agents/`) and the instruction files `AGENTS.md`, `CLAUDE.md`, `GEMINI.md` stay uncommitted: they are written
per machine by `init`/`connect`, the system-curator copies change with each binary, and four of them hold
absolute paths. `skillhub status` and `skillhub doctor` report "Git clean" / healthy with them untracked.

First commit:

```bash
git -C ~/skillhub add .gitignore .skillhub/schema-version
git -C ~/skillhub commit -m "Initialize Skill Hub workspace"
```

```text
[main (root-commit) <sha>] Initialize Skill Hub workspace
 2 files changed, 3 insertions(+)
 create mode 100644 .gitignore
 create mode 100644 .skillhub/schema-version
```

After creating or changing skills (verified after `skill create` + `skill activate`):

```bash
git -C ~/skillhub add skills history
git -C ~/skillhub commit -m "Add the commit-style skill"
```

```text
[main <sha>] Add the commit-style skill
 4 files changed, 66 insertions(+)
 create mode 100644 history/operations/2026/10/OP-<id>.yaml
 create mode 100644 history/operations/2026/10/OP-<id>.yaml
 create mode 100644 skills/core/commit-style/.meta/skill.yaml
 create mode 100644 skills/core/commit-style/SKILL.md
```

General form for any curation change (verified on a fresh workspace: stages exactly `.gitignore` and
`.skillhub/schema-version`, empty folders do not error, nothing staged contains a machine path):

```bash
git -C ~/skillhub add .gitignore .skillhub skills sources history registry config distill evals
```

Validation: `grep -l "$SB"` over every committed file returned nothing.

### Connect a project (Claude Code only)

```bash
mkdir -p ~/myproject && cd ~/myproject && git init -q
skillhub connect --workspace ~/skillhub --host claude --yes
```

Preview (without `--yes`):

```text
Preview only: agent connection for project <home>/myproject was not
written without --yes.
  - Claude Code: will connect (.mcp.json, .claude/settings.local.json, curator skill, CLAUDE.md)

Claude Code: curation runs through the CLI. Commands with --yes or confirm require approval;
--approve-content is blocked. First opening this folder shows a trust dialog listing
Bash(skillhub:*).

To write these files, run:
  $ skillhub connect --workspace <home>/skillhub --host claude-code --yes
```

With `--yes`:

```text
Agent connection for project <home>/myproject written.
  - Claude Code: connected (.mcp.json, .claude/settings.local.json, curator skill, CLAUDE.md)
WARNING: Host activation coordination is instruction-only and best effort; each Agent Host remains
         responsible for permissions and activation.

Claude Code: curation runs through the CLI. Commands with --yes or confirm require approval;
--approve-content is blocked. First opening this folder shows a trust dialog listing
Bash(skillhub:*).

Next: restart your agent so it loads the new MCP server, then ask it: "curate my Skill Hub"
```

Files written in the project: `.claude/settings.local.json`, `.claude/skills/system-curator/SKILL.md`,
`.mcp.json`, `CLAUDE.md`. A rerun prints "Agent connection for project <home>/myproject is already current;
nothing to change." `skillhub integrate --workspace ~/skillhub --host claude` gives the same result.

`.mcp.json` registers `<binary> mcp serve --profile runtime --workspace <home>/skillhub`. The Claude Code
permission rules written to `.claude/settings.local.json` match the user-guide table exactly: allow
`Bash(skillhub:*)`; ask `Bash(skillhub * --yes*)`, `Bash(skillhub * confirm *)`; deny
`Bash(skillhub * --approve-content*)`; plus `additionalDirectories` for `runtime/cache/skills`, `runtime/envs`,
`runtime/config` of the workspace.

The README Quickstart prompt "curate my Skill Hub" is confirmed as the product's own suggestion, and `connect`
installs the `system-curator` skill into `.claude/skills/`. What the agent answers was not seen (no live
session); describe it modestly.

### Connect every project (`-g`, second sandbox HOME `/tmp/skillhub-docs-rewrite/wave0-global`)

```bash
skillhub connect -g --workspace ~/skillhub --yes
```

```text
Agent connection for your user account (all projects) written.
  - Claude Code: connected (.claude.json, .claude/settings.json, curator skill, CLAUDE.md)
  - Codex: connected (.codex/config.toml, curator skill, AGENTS.md)
  - Gemini CLI: connected (.gemini/settings.json, curator skill, GEMINI.md)
WARNING: Host activation coordination is instruction-only and best effort; each Agent Host remains
         responsible for permissions and activation.

Claude Code: curation runs through the CLI. Commands with --yes or confirm require approval;
--approve-content is blocked. First opening this folder shows a trust dialog listing
Bash(skillhub:*).

Next: restart your agent so it loads the new MCP server, then ask it: "curate my Skill Hub"
```

Files written in HOME (matches the user-guide "Global" row): `~/.claude.json`, `~/.claude/settings.json`,
`~/.claude/CLAUDE.md`, `~/.claude/skills/system-curator/SKILL.md`, `~/.codex/config.toml`, `~/.codex/AGENTS.md`,
`~/.agents/skills/system-curator/SKILL.md`, `~/.gemini/settings.json`, `~/.gemini/GEMINI.md`,
`~/.gemini/skills/system-curator/SKILL.md`. Preview shows the fix line
`skillhub connect --global --workspace <home>/skillhub --yes`.

### The workspace is not found outside a connected project

After `connect -g`, in an unrelated folder (`~/otherproject`), both `skillhub status` and `skillhub doctor`
exit 2 with:

```text
ERROR: The request cannot be accepted.
WHY: No Skill Hub workspace was found from the current directory.
FIX: Pass `--workspace <path>`, set the SKILLHUB_WORKSPACE environment variable, or run
     `skillhub init <path> --yes`.
```

`SKILLHUB_WORKSPACE=~/skillhub skillhub status` fixes it:

```text
Workspace:  <home>/skillhub
Status:     Skill Hub is up to date.
Health:     Workspace valid; search index current; Git clean.
Inventory:  No skills yet.
            Next: ask your agent 'create a skill for ...' or run:
            skillhub skill create my-skill --collection core --name "My Skill" --description "Skill
            description"

Next: Continue normal work
```

Important for Phase 2: the same error appears when you run `skillhub skill create` from your home folder. Run
the skill commands inside the connected project (`cd ~/myproject`) or pass `--workspace ~/skillhub`. The skill
file itself can live anywhere; the transcript keeps it at `~/commit-style.md`.

### Create the first skill

The skill file, saved as `~/commit-style.md` (frontmatter optional; Skill Hub adds `name` and `description`):

```markdown
# Commit style

Use this skill when you write a Git commit message.

1. Start with a short summary line in the imperative mood, at most 50 characters.
2. Leave one blank line, then explain what changed and why in plain sentences.
3. End every commit message with this exact line:

   Checked-by: skillhub-demo
```

```bash
cd ~/myproject
skillhub skill create commit-style --collection core --name "Commit style" \
  --description "Write git commit messages in our team style" --content-file ~/commit-style.md \
  --operation document --trigger "write a commit message" --not-for "writing release notes" \
  --min-scope single_step --example "write a commit message for my staged changes" --yes
```

```text
Draft skill commit-style saved.
Next: edit the instructions with `skillhub skill edit commit-style --editor`, then preview
      activation with `skillhub skill activate commit-style`.
```

Without `--yes` the command prints a preview with a proposal ID, digests and a `skill confirm` line; docs show
only the `--yes` form (or say "without `--yes` you see a preview first").

```bash
skillhub skill activate commit-style --yes
```

```text
Skill commit-style is now active.
Next: ask your agent to use it, or inspect with `skillhub skill show commit-style`. Commit with
      `git -C <home>/skillhub commit`.
```

`--content-file` is required in practice: a bare `skill create` (no content file, no routing fields) saves a
draft, and `skill activate --yes` then exits 2 with:

```text
ERROR: The request cannot be accepted.
WHY: skill bare-skill requires: content (replace untouched scaffold instructions), trigger, not_for
     or rationale (quality.routing_review_rationale), min_scope
FIX: Preview the fix with
     `skillhub skill edit bare-skill --trigger "<when to use>" --not-for "<when not to use>" --min-scope single_step`,
     review and confirm it, then preview `skillhub skill activate bare-skill`.
```

### Sanity checks

```text
$ skillhub doctor
Workspace is healthy.

$ skillhub skill list
ID            STATE   COLLECTION  NAME
commit-style  active  core        Commit style
```

`skillhub status` before the skill commit:

```text
Workspace:  <home>/skillhub
Status:     Git has uncommitted canonical changes.
Health:     Workspace valid; search index current; Git has uncommitted changes.
Inventory:  1 active, 0 draft; 0 watched sources.

Next: Review uncommitted changes
  $ git -C <home>/skillhub add -A && git -C <home>/skillhub commit -m "Update skills"
```

After the commit: `Status: Skill Hub is up to date.`, `Health: Workspace valid; search index current; Git clean.`,
`Inventory: 1 active, 0 draft; 0 watched sources.`, `Next: Continue normal work`.

`skillhub validate` after activation (one example only):

```text
Workspace validation passed.
WARN missing_examples commit-style: skill "commit-style" has 1 examples; active skills require at
least 3 examples Fix: Add 3–5 natural task phrasings
```

Docs say "at least 3 examples are recommended" (activation succeeded with one).

### Resolve by hand (working request JSON, frozen)

`~/req.json`:

```json
{"schema_version":"1","request_id":"r1","task":{"description":"write a commit message for my staged changes","scope":"single_step"},"operation":"document"}
```

```bash
skillhub resolve --request ~/req.json
```

```text
Resolved commit-style (medium confidence). Host approval is required before activation.
```

The same request without `"operation"` also resolves to commit-style. `--json` returns `"status":"resolved"`,
`"primary":{"id":"commit-style", ... "confidence":"medium"}`, `"reason_codes":["example_match","task_match"]`.

Without `task.scope` (exit 0, a question instead of a match):

```text
What is the bounded scope of this task?
Choices: single_step | multi_step | project | unknown

To answer, set "task.scope" in your request JSON, e.g.:
{"task": {"description": "...", "task.scope": "single_step"}}
or provide prior in your request:
{"prior": {"resolution_id": "res_<id>", "answers": {"task.scope":
"single_step"}}}
```

Following that hint literally fails: `"task.scope"` inside `task` or at the top level both exit 2 with
`WHY: parse request: json: unknown field "task.scope"`. The working key is `"scope"` inside `task`.

### MCP stdio proof (no live Claude Code session)

`python3 -I /tmp/skillhub-docs-rewrite/wave0/mcp_probe.py .mcp.json` in `~/myproject` starts the exact command
from `.mcp.json`:

```text
server: {"name": "skillhub", "version": "1"}
tools: ['skill_feedback', 'skill_get', 'skill_resolve']
skill_resolve primary: commit-style medium
skill_get has marker: True | error: False
```

So over MCP the agent-facing tools resolve the request to commit-style, and `skill_get` (argument `skill_id`)
returns the skill text containing `Checked-by: skillhub-demo`. That is the most the docs may promise for "what
you should see"; an agent writing the marker in a real session is unverified.

### Add a skill from GitHub (network, preview only)

Working forms (both print the same preview, exit 0, nothing written to the workspace):

```bash
skillhub skill add https://github.com/anthropics/skills --path skills/pdf
skillhub skill add https://github.com/anthropics/skills/tree/main/skills/pdf
```

```text
Add pdf as a draft from https://github.com/anthropics/skills.git at <commit>.
11 files, 57.3 KB. Origin retained. Agent use: off.
Upstream: tracked by new source anthropics-skills (checked weekly by `skillhub check`; nothing runs
in the background).
No collection files changed.
Next: skillhub skill confirm PROP-<id>
```

Failing form (the one in `skillhub help skill`), exit 2:

```bash
skillhub skill add https://github.com/anthropics/skills --skill pdf
```

```text
ERROR: The request cannot be accepted.
WHY: list repository resources: source exceeded bytes limit (8560432 > 8388608)
FIX: Correct the skill fields or workspace state and retry.
```

### Other verified commands

- `skillhub --version` exits 2: `WHY: The command "--version" is not supported.` Use `skillhub version`.
- `skillhub serve web --no-open --loopback-only --addr 127.0.0.1:47391` prints one line:
  `Skill Hub web UI: http://127.0.0.1:47391/#token=<token>`. With no flags on a machine with several network
  interfaces, it prints first
  `WARNING: The web UI is reachable from other devices over plain HTTP. Anyone who can observe this traffic can read the session token. Use --loopback-only to keep it on this machine.`
  then `Skill Hub web UI: http://127.0.0.1:7421/#token=<token>` and one `<interface>: http://<ip>:7421/#token=<token>`
  line per interface. Default port observed: 7421.
- `skillhub skill review <skill-id>` on an untracked skill shows
  `Git: uncommitted changes (0 staged, 0 unstaged, 1 untracked)`.

## 8. Surprises and contradictions with the synthesis

1. **`skillhub inbox` and `skillhub insight` do not exist.** `skillhub help inbox` and `skillhub inbox` both
   fail with `The command "inbox" is not supported.` (same for `insight`). They were removed in commit `3dcf539`
   ("remove CLI commands distill, inbox, and insight"). The current docs (`curating-skills.md:434-448`, `:634`,
   `user-guide.md:36`, `:518`) and phases 7 and 8 still list them. Verified flow instead: on **Sources** in the
   web UI, choose **Distill with Curator Agent**, **Copy handoff**, paste it to an agent that has the distill-lab
   skill; it records lessons in the skill's `.meta/distill.yaml`; read and decide them in the skill's **Distill**
   tab (texts in `web/src/screens/distill/DistillHandoffScreen.tsx:24`, `web/src/screens/skill-detail/DistillTab.tsx:11`).
2. **Skill commands need a connected folder.** Running `skill create` from `~` fails with "No Skill Hub workspace
   was found"; the synthesis listing implies `cd ~/myproject` but does not say why. Getting-started must run the
   skill steps in the connected project (frozen transcript above).
3. **Activation also needs real content.** A bare create fails activation for four reasons, including
   "content (replace untouched scaffold instructions)", not only the three routing fields.
4. **Follow-up 5 in plan.md does not reproduce.** `skill review` on an untracked skill shows
   `Git: uncommitted changes (... 1 untracked)`, not `Git: clean`. The owner may drop that follow-up.
5. **Follow-up 1 is slightly different.** The resolve hint puts `"task.scope"` inside the `task` object; that
   form is rejected too (`unknown field "task.scope"`).
6. **`init` also suggests `git add -A`** (not only `status`), and `status` suggests `add -A` after changes.
   Extends follow-up 3.
7. **`connect` prints "restart your agent"**; docs say "open a new Claude Code session in that folder" anyway.
8. **`skillhub help` itself makes the banned claim**: "serves it to Claude Code, Codex, and Gemini CLI".
   Docs must not quote that line.
9. **`skill add` preview mentions `skillhub check` "checked weekly"**: `check` is a real top-level command;
   "weekly" is the recorded interval, and the same line says "nothing runs in the background". Docs keep that
   wording.
10. **The distill-lab skill is not shipped** (only `system-curator` is installed by `connect`). Docs must not
    say where to get it.
11. **`integrate` has no help** (`help integrate` fails; `integrate --help` fails with the connect usage line).
12. **The build is `dev`.** No behavior seen was `dev`-specific; `skillhub update` was not run.

New product follow-ups for the owner (add to plan.md "Owner follow-ups" when the lead next edits it): items 1
(stale docs only; no product bug), 6, 7, 8, 11 above.

## 9. Isolation check

- `/home/vantt/skill-hub`: `git status --porcelain` and `HEAD` identical before and after.
- `~/.codex`, `~/.gemini`, `~/.local/bin/skillhub`: mtimes unchanged. `~/.claude.json` and `~/.claude` mtimes
  changed during the run because the controlling Claude Code session writes them; none of
  `~/.claude.json`, `~/.claude/settings.json`, `~/.codex/config.toml`, `~/.gemini/settings.json` contains
  `skillhub-docs-rewrite`.
- The repository worktree had no changes from `make web-build` (the embedded `dist` output is ignored).

## Unverified

- An agent in a live Claude Code session calling the hub and writing `Checked-by: skillhub-demo`.
- What the agent answers to "curate my Skill Hub".
- `/mcp` in Claude Code listing `skillhub` with 3 tools (the stdio probe shows the 3 tools only).
- Codex and Gemini CLI behavior after `connect` (files written, nothing run).
- The installer scripts and `skillhub update` (not run; Phase 3 reads the scripts).
