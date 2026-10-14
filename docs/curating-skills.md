# Curating skills

A good collection makes your agent reliably pick the right skill. This guide shows how to build one: add, create, review, edit, activate, commit, and check how well skills get picked.

Finish the [Quickstart](../README.md#quickstart) first. It installs your hub and connects a project. Run the commands below inside the hub or a connected project, for example `cd <your-project> && skillhub skill list`.

## Start with the hub, not a command

Ask your connected agent:

> curate my Skill Hub

The curator skill tells your agent to check the hub and suggest one next step. You see the same suggestion in the terminal:

```bash
skillhub status
```

```text
Status:     Skill Hub is up to date.
Health:     Workspace valid; search index current; Git clean.
Inventory:  1 active, 0 draft; 0 watched sources.
```

Output is trimmed. Your agent curates through the same `skillhub` commands. Claude Code asks you before any `skillhub` command with `--yes` or `confirm`, and before commands that change the hub, its connections, the token or the cache, such as `skillhub doctor --fix`. Codex and Gemini CLI do not ask first.

Most sessions follow one path: add or create a skill, review and edit it, activate it, then commit.

## Safety model

Skill Hub changes nothing without your say-so. These rules hold for every command:

- **Drafts by default.** Every skill you add or create starts as a **draft**: a skill agents are not offered yet. It becomes **active** only when you activate it.
- **Content approval in the terminal only.** A skill copied from someone else's repository gives agents nothing until you approve its content. Only you can do that, with a terminal command; the web UI and your agent cannot.
- **Preview before confirm.** Commands that change skills show a preview first. Add `--yes`, or run the `skillhub skill confirm` line it prints, to apply exactly that preview.
- **Review changes nothing.** `skillhub skill review` reports facts; it never approves or activates a skill.

> [!IMPORTANT]
> Skill Hub never commits or pushes your skill changes for you. You check each change and commit it yourself. Only `skillhub init` makes the hub's first commit, and pushes it to an empty repository when you give it one.

<details>
<summary>More detail on approval and previews</summary>

- Any change to a copied skill's files or runtime block clears its content approval. You review and approve it again.
- Content approval is a guard against mistakes by a cooperative agent. It does not stop a program that can write to the hub folder directly.
- The short form `skillhub skill confirm <proposal-id>` is for you in the terminal. Agents and scripts pass the full `--proposal`, `--proposal-digest` and `--base-version` values from the preview. That way they apply exactly what was previewed.
- If a skill or source with the same ID already exists, the command stops instead of overwriting it.

</details>

## Create your first skill

A **skill** is a folder with a `SKILL.md` your agent follows and a small file that says when it applies. This one teaches your team's commit style. Its last rule adds a marker line, so you can tell when the skill was used.

Save this text as `~/commit-style.md`:

```markdown
# Commit style

Use this skill when you write a Git commit message.

1. Start with a short summary line in the imperative mood, at most 50 characters.
2. Leave one blank line, then explain what changed and why in plain sentences.
3. End every commit message with this exact line:

   Checked-by: skillhub-demo
```

Create the skill from inside your connected project. In a terminal, run `skillhub skill create` and answer the questions: a name, an id (suggested from the name), a one-sentence description, when an agent should use the skill and when it should not. Your answers are the confirmation, so the draft is saved when you give the last one. The skill gets the starter text, which you replace with the contents of `~/commit-style.md` in the file the command prints.

```bash
cd <your-project> && skillhub skill create
```

Scripts and agents, which cannot answer questions, pass flags instead. The flags tell the hub when the skill applies (`--trigger`, `--example`) and when it does not (`--not-for`). At least 3 examples are recommended; one is enough to start:

```bash
cd <your-project> && skillhub skill create commit-style --collection default --name "Commit style" \
  --description "Write Git commit messages in our team style" --content-file ~/commit-style.md \
  --operation document --trigger "write a commit message" --not-for "writing release notes" \
  --min-scope single_step --example "write a commit message for my staged changes" --yes
```

```text
Draft skill commit-style saved.
```

New skills start as a **draft**: a skill agents are not offered yet. Make it **active**, so agents can be offered it:

```bash
skillhub skill activate commit-style --yes
```

```text
Skill commit-style is now active.
```

### See your first win

Your agent needs a staged change to write about. In a connected project, change a file and stage it with `git add <file>`. Then ask your agent:

> write a commit message for my staged changes

Your agent is told to ask the hub which skill fits. The hub sends back a **recommendation**: the skill that fits, or "no skill fits". If your agent used the skill, the commit message ends with this line:

```text
Checked-by: skillhub-demo
```

[How the agent picks a skill](user-guide.md#how-the-agent-picks-a-skill) shows how to run the same lookup by hand.

### Commit the new skill

The new skill is a set of files in your hub, not yet committed. `skillhub status` names them and prints the exact Git line that commits them by name:

```bash
skillhub status
```

```text
Status:     The hub has changes that are not committed to Git.
Health:     Workspace valid; search index current; Git has uncommitted changes.
Inventory:  1 active, 0 draft; 0 watched sources.

Check  Status  Detail
Git    notice  4 changed files are not committed to Git yet.

NOTE:  Git: run
       `git -C <home>/skill-hub add -- history skills && git -C <home>/skill-hub commit -m 'Update skills'`.
```

Output is trimmed. Run that line, or commit the same paths with a message of your own.

## Add a skill (`skillhub skill add`)

Use `skill add` to bring in a skill that already exists, from a folder on this machine or from GitHub. The skill arrives as a draft.

### Add from a local folder

Point `skill add` at a folder that holds a `SKILL.md`. Leave out `--yes` to see a preview first:

```bash
skillhub skill add ~/review-checklist --yes
```

```text
Draft review-checklist added. Agent use: off. Changes are not committed.
Next: skillhub skill review review-checklist
```

Adding from a local folder works in the terminal only. Skill Hub copies the other files in the folder, scripts keep their executable bit, and the preview lists every file with its size. It leaves out files that look like secrets or build output: `.env` and `.env.*` (except `.env.example` and `.env.sample`), `*.pem`, SSH private keys such as `id_rsa` and `id_ed25519`, `.DS_Store`, and the folders `.git`, `node_modules` and `__pycache__`. The preview names each one under "Left out", so you can see what did not come along. A skill from your own folder needs no content approval.

```text
Add review-checklist as a draft from local.
Files (2, 98 B):
  - SKILL.md (80 B)
  - scripts/check.sh (18 B) (executable)
Left out (not copied into the hub):
  - .env (looks like a secret)
Origin retained. Agent use: off.
No collection files changed.
Next: skillhub skill confirm PROP-<id>
```

Adding the same folder again changes nothing: the command says the skill was already added and shows its current state.

### Add from GitHub

Point at the skill's folder in the repository with `--path`. A folder URL such as `https://github.com/anthropics/skills/tree/main/skills/pdf` works too:

```bash
skillhub skill add https://github.com/anthropics/skills --path skills/pdf
```

```text
Add pdf as a draft from https://github.com/anthropics/skills.git at <commit>.
Files (11, 57.3 KB):
  - SKILL.md (9.9 KB)
  - LICENSE.txt (1.4 KB)
  - scripts/extract.py (4.2 KB)
  ...one line for each of the other 8 files
Origin retained. Agent use: off.
Upstream: tracked by new source anthropics-skills (checked weekly by `skillhub source check`; nothing
runs in the background).
No collection files changed.
Next: skillhub skill confirm PROP-<id>
```

Run the `skillhub skill confirm` line, or repeat the command with `--yes`. The repository becomes the skill's **upstream**: the source it was copied from, so you can pull its updates later.

For a repository with several skills, `--skill <name>` picks one and `--all` adds every skill as its own draft. Repositories larger than 8 MiB are refused with the same advice: point at the skill's folder as above. `--ref <branch-or-tag>` picks another branch or tag.

### After you add a skill

A new draft needs four things before agents can use it:

1. **Review it.** Run `skillhub skill review <skill-id>` and read what it reports.
2. **Approve its content** (skills from GitHub only). Read its files, then run the approval command that the review prints. See [Review diagnostic facts](#review-diagnostic-facts-skillhub-skill-review).
3. **Fill in its routing fields.** Add a trigger, a `--not-for` entry and `--min-scope` with `skillhub skill edit`. See [Edit and improve a skill](#edit-and-improve-a-skill-skillhub-skill-edit).
4. **Activate it.** Run `skillhub skill activate <skill-id> --yes`. See [Activate, deprecate, and archive](#activate-deprecate-and-archive).

## Create a skill from your own workflow (`skillhub skill create`)

Capture a workflow your team repeats. In a terminal, `skillhub skill create` asks for the name, id, description, when to use the skill and when not to, then saves a draft and prints the file to edit and the command that activates it:

```bash
skillhub skill create
```

The guided form uses the collection `default` (or the only collection you have), the minimum scope `single_step` and the operation `other`, and gives the skill starter text. When stdin or stdout is not a terminal, the command never asks: it names the missing flags instead.

To script it, write the instructions in a Markdown file first, then create the skill with all its routing fields in one command:

```bash
skillhub skill create commit-style --collection default --name "Commit style" \
  --description "Write Git commit messages in our team style" --content-file ~/commit-style.md \
  --operation document --trigger "write a commit message" --not-for "writing release notes" \
  --min-scope single_step --example "write a commit message for my staged changes" --yes
```

```text
Draft skill commit-style saved.
```

[Create your first skill](#create-your-first-skill) walks through this example end to end. What each flag tells the hub:

- `--trigger`: a phrase for when the skill applies. `--not-for`: when it does not.
- `--min-scope`: the smallest task it fits: `single_step`, `multi_step` or `project`.
- `--operation`: the kind of work, such as `document`, `review` or `implement`.
- `--example`: a request that should pick this skill. At least 3 examples are recommended.

Always pass `--content-file`. Without it, the skill gets starter text, and activation refuses a skill whose instructions are still starter text.

## Review diagnostic facts (`skillhub skill review`)

Run a review before you activate a skill and after every change. It reads the hub files and changes nothing:

```bash
skillhub skill review review-checklist
```

```text
Review:       review-checklist (review-checklist)
State:        draft (valid; routing: not eligible; servable)
Files:        2 files, 378 B (entrypoint: skills/default/review-checklist/SKILL.md)
Origin:       local authoring (not watched)

Activation readiness
  - Missing required field: trigger
  - Missing required field: not_for or rationale (quality.routing_review_rationale)
  - Missing required field: min_scope
```

Output is trimmed. The full review also shows origin, Git status, the tools its scripts seem to need and weak routing fields. Add `--verbose` to list every file with its size.

For a skill from GitHub, the review adds a **Content trust** block:

```text
Content trust
Origin:          third-party
Approval:        review required; agents get no content from this skill (content_review_required)
Content digest:  sha256:<digest>
Never approved: review the full skill.
After reviewing the skill's files and runtime block, approve exactly this content with:
  $ skillhub skill edit pdf --approve-content sha256:<digest>
```

The digest is a fingerprint of the files you reviewed; read them, then run that exact command in your terminal. After an approved skill changes, the block lists what changed and a `git diff` command to inspect it. The web UI shows the same facts in a skill's **Review** tab, without an approve button.

## Edit and improve a skill (`skillhub skill edit`)

Edit a skill to sharpen when it gets picked, or to change its instructions. Each edit shows a preview first.

Change the description or routing fields. Fields you leave out keep their values:

```bash
skillhub skill edit review-checklist --trigger "review a pull request" \
  --not-for "writing new code" --min-scope single_step --yes
```

```text
Draft skill review-checklist saved.
```

Add requests that should pick the skill, and requests that should not:

```bash
skillhub skill edit review-checklist --example "review my pull request" \
  --example "check this PR against our checklist" --counter-example "write release notes" --yes
```

Each flag can repeat, up to 10 of each. At least 3 examples are recommended.

Edit the instructions in your editor (`$VISUAL` or `$EDITOR`):

```bash
skillhub skill edit review-checklist --editor
```

If the skill changes while your editor is open, Skill Hub refuses the edit. Your text stays in a recovery file under `runtime/edits/` in the hub. To replace the instructions from a file instead, use `--content-file <file>`.

Declare the tools and variables a skill's scripts need with a runtime file:

```bash
skillhub skill edit review-checklist --runtime-file ~/runtime.yaml --yes
```

The file is a YAML mapping such as `requires: {bins: [git]}`; a file that holds `{}` removes the block. Changing it clears content approval on a skill from GitHub. [Run skills that need tools or secrets](user-guide.md#run-skills-that-need-tools-or-secrets) explains the rest.

Without `--yes`, every edit prints a preview and a line to apply it later:

```bash
skillhub skill confirm <proposal-id>
```

## Activate, deprecate, and archive

A skill moves through four states, in this order:

```text
draft -> active -> deprecated -> archived
```

- **draft**: agents are not offered it yet.
- **active**: agents can be offered it.
- **deprecated**: being phased out.
- **archived**: retired and kept for history; agents are not offered it.

Preview a change by leaving out `--yes`. Apply it with `--yes`:

```bash
skillhub skill activate review-checklist --yes
```

```text
Skill review-checklist is now active.
```

Later, retire it in two steps:

```bash
skillhub skill deprecate review-checklist --yes
```

```bash
skillhub skill archive review-checklist --yes
```

An active skill cannot go straight to archived; `archive` refuses with `invalid skill lifecycle transition: active -> archived`. Activation also refuses a skill with starter instructions or missing routing fields. [Activation refuses a draft](troubleshooting.md#activation-refuses-a-draft) shows the fix.

## Keep vendored skills up to date

Moved to [Keep copied skills up to date](sources-and-learning.md#keep-copied-skills-up-to-date).

## Learn from references

Moved to [Learn from other repositories](sources-and-learning.md#learn-from-other-repositories).

## Resource-verified fallback and state basis

Moved to [Workspace and index](troubleshooting.md#workspace-and-index); the full rules are in [contracts](contracts/error-codes.md).

## Review and commit workspace changes

Your hub is a Git repository, and you decide what goes into it. See what changed since your last commit:

```bash
cd ~/skill-hub && skillhub maintain diff
```

Check that every hub file is valid:

```bash
cd ~/skill-hub && skillhub maintain validate
```

```text
Workspace validation passed.
```

Stage the hub's own paths by name, never the whole folder. `init` writes connection files into the hub, and some hold absolute paths for this machine. `skillhub status` prints a Git line that stages exactly the changed paths, for example:

```bash
cd ~/skill-hub && git add -- history skills
```

Check exactly what you staged, without touching any other file:

```bash
cd ~/skill-hub && skillhub maintain validate --staged
```

Then commit:

```bash
cd ~/skill-hub && git commit -m "Curate skills"
```

Push to your own remote with `git push` when you want a backup.

To run the check on every commit, add it to a Git hook, such as `.git/hooks/pre-commit` in the hub:

```sh
#!/bin/sh
skillhub maintain validate --staged
```

Skill Hub does not install hooks for you; any hook manager works. A hook written for an earlier release runs the old top-level validate command, which no longer exists. `skillhub doctor` reports such a hook, and `skillhub doctor --fix` changes that one command and keeps the rest of the hook.

## Measure usage (`skillhub telemetry funnel`)

See how often each skill is recommended, used and blocked. The report covers the last 30 days:

```bash
skillhub telemetry funnel
```

It also lists skills nobody uses and skills blocked because their content is not approved yet. Narrow it with `--since 90d` or `--skill <skill-id>`.

To count skill use from your Claude Code history, import its transcripts:

```bash
skillhub telemetry import-transcripts --project ~/myproject
```

It reads only tool-call entries from the transcripts. The usage record stays on your machine unless you export it.

## Check routing quality (`skillhub eval routing`)

Test whether your active skills' examples pick the right skill. Each example is matched with itself left out, so the score shows how well new requests will match:

```bash
skillhub eval routing
```

```text
Metrics:
Precision@1:          1.0000
Recall:               0.6667

Failures (1):
  - [positive] review-checklist #1: "do a code review of the open PR" (got status=no_skill,
    primary=)
```

Output is trimmed. Each failure is a request that did not pick its skill; fix it with a better trigger or more examples. `--json` puts the numbers under `metrics`, for example `metrics.recall`.

To use it as a gate, add thresholds such as `--min-precision 0.60` or `--min-recall 0.58`. A missed threshold exits with code 1. `--no-skill <file>` adds requests that should pick no skill.

## Import repository skills as drafts (`skillhub source import`)

Moved to [Import many skills from a repository](sources-and-learning.md#import-many-skills-from-a-repository).

## Advanced intake and recovery

Moved to [Triage candidate repositories](sources-and-learning.md#triage-candidate-repositories).

## Command map

Moved to [Command reference](command-reference.md).
