# Curating skills

A good collection makes your agent reliably pick the right skill. This guide shows how to build one: add, create, review, edit, activate, commit, and check how well skills get picked.

Finish [Get started](getting-started.md) first. It creates your workspace, connects Claude Code and makes your first skill. Run the commands below inside a connected project, or anywhere with `--workspace ~/skillhub`.

## Start with the hub, not a command

Ask your connected agent:

> curate my Skill Hub

Your agent checks the hub and suggests one next step. You see the same suggestion in the terminal:

```bash
skillhub status
```

```text
Status:     Skill Hub is up to date.
Health:     Workspace valid; search index current; Git clean.
Inventory:  1 active, 0 draft; 0 watched sources.
```

Output is trimmed. Your agent curates through the same `skillhub` commands, and it asks you before any command that changes files.

Most sessions follow one path: add or create a skill, review and edit it, activate it, then commit.

## Safety model

Skill Hub changes nothing without your say-so. These rules hold for every command:

- **Drafts by default.** Every skill you add or create starts as a **draft**: a skill agents are not offered yet. It becomes **active** only when you activate it.
- **Content approval in the terminal only.** A skill copied from someone else's repository gives agents nothing until you approve its content. Only you can do that, with a terminal command; the web UI and your agent cannot.
- **Preview before confirm.** Every command that changes files shows a preview first. Add `--yes`, or run the `skillhub skill confirm` line it prints, to apply exactly that preview.
- **Review changes nothing.** `skillhub skill review` reports facts; it never approves or activates a skill.

> [!IMPORTANT]
> Skill Hub never commits or pushes for you. You check each change and commit it yourself.

<details>
<summary>More detail on approval and previews</summary>

- Any change to a copied skill's files or runtime block clears its content approval. You review and approve it again.
- Content approval is a guard against mistakes by a cooperative agent. It does not stop a program that can write to the workspace folder directly.
- The short form `skillhub skill confirm <proposal-id>` is for you in the terminal. Agents and scripts pass the full `--proposal`, `--proposal-digest` and `--base-version` values from the preview. That way they apply exactly what was previewed.
- If a skill or source with the same ID already exists, the command stops instead of overwriting it.

</details>

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

Adding from a local folder works in the terminal only; your agent asks you to run it. Skill Hub copies the other files in the folder too, hidden ones included, so keep secrets out of it. A skill from your own folder needs no content approval.

### Add from GitHub

Point at the skill's folder in the repository with `--path`. A folder URL such as `https://github.com/anthropics/skills/tree/main/skills/pdf` works too:

```bash
skillhub skill add https://github.com/anthropics/skills --path skills/pdf
```

```text
Add pdf as a draft from https://github.com/anthropics/skills.git at <commit>.
11 files, 57.3 KB. Origin retained. Agent use: off.
Upstream: tracked by new source anthropics-skills (checked weekly by `skillhub check`; nothing runs
in the background).
No collection files changed.
Next: skillhub skill confirm PROP-<id>
```

Run the `skillhub skill confirm` line, or repeat the command with `--yes`. The repository becomes the skill's **upstream**: the source it was copied from, so you can pull its updates later.

For a repository with several skills, `--skill <name>` picks one and `--all` adds every skill as its own draft. Repositories larger than 8 MiB are refused, so for a large one point at the skill's folder as above. `--ref <branch-or-tag>` picks another branch or tag.

### After you add a skill

A new draft needs four things before agents can use it:

1. **Review it.** Run `skillhub skill review <skill-id>` and read what it reports.
2. **Approve its content** (skills from GitHub only). Read its files, then run the approval command that the review prints. See [Review diagnostic facts](#review-diagnostic-facts-skillhub-skill-review).
3. **Fill in its routing fields.** Add a trigger, a `--not-for` entry and `--min-scope` with `skillhub skill edit`. See [Edit and improve a skill](#edit-and-improve-a-skill-skillhub-skill-edit).
4. **Activate it.** Run `skillhub skill activate <skill-id> --yes`. See [Activate, deprecate, and archive](#activate-deprecate-and-archive).

## Create a skill from your own workflow (`skillhub skill create`)

Capture a workflow your team repeats. Write the instructions in a Markdown file first, then create the skill with all its routing fields in one command:

```bash
skillhub skill create commit-style --collection core --name "Commit style" \
  --description "Write git commit messages in our team style" --content-file ~/commit-style.md \
  --operation document --trigger "write a commit message" --not-for "writing release notes" \
  --min-scope single_step --example "write a commit message for my staged changes" --yes
```

```text
Draft skill commit-style saved.
```

[Create your first skill](getting-started.md#create-your-first-skill) walks through this example end to end. What each flag tells the hub:

- `--trigger`: a phrase for when the skill applies. `--not-for`: when it does not.
- `--min-scope`: the smallest task it fits: `single_step`, `multi_step` or `project`.
- `--operation`: the kind of work, such as `document`, `review` or `implement`.
- `--example`: a request that should pick this skill. At least 3 examples are recommended.

Always pass `--content-file`. Without it, the skill gets starter text, and activation refuses a skill whose instructions are still starter text.

## Review diagnostic facts (`skillhub skill review`)

Run a review before you activate a skill and after every change. It reads the workspace files and changes nothing:

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

Output is trimmed. The review also shows where the skill came from and its Git status. It names the tools its scripts seem to need and flags weak routing fields. Add `--verbose` to list every file with its size.

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

The digest is a fingerprint of the files you reviewed. Read the files, then run that exact command in your terminal. After an approved skill changes, the block lists what changed and a `git diff` command to inspect it. The web UI shows the same facts in a skill's **Review** tab, without an approve button.

## Edit and improve a skill (`skillhub skill edit`)

Edit a skill to sharpen when it gets picked, or to change its instructions. Each edit shows a preview first.

Change the description or routing fields. Fields you leave out keep their values:

```bash
skillhub skill edit review-checklist --trigger "review a pull request" --not-for "writing new code" --min-scope single_step --yes
```

```text
Draft skill review-checklist saved.
```

Add requests that should pick the skill, and requests that should not:

```bash
skillhub skill edit review-checklist --example "review my pull request" --example "check this PR against our checklist" --counter-example "write release notes" --yes
```

Each flag can repeat, up to 10 of each. At least 3 examples are recommended.

Edit the instructions in your editor (`$VISUAL` or `$EDITOR`):

```bash
skillhub skill edit review-checklist --editor
```

If the skill changes while your editor is open, Skill Hub refuses the edit. Your text stays in a recovery file under `runtime/edits/` in the workspace. To replace the instructions from a file instead, use `--content-file <file>`.

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

An active skill cannot go straight to archived; `archive` refuses with `invalid skill lifecycle transition: active -> archived`. Activation also refuses a skill with starter instructions or missing routing fields. [Activation refuses a draft](troubleshooting.md#skills) shows the fix.

## Keep vendored skills up to date

Moved to [Keep copied skills up to date](sources-and-learning.md#keep-copied-skills-up-to-date).

## Learn from references

Moved to [Learn from other repositories](sources-and-learning.md#learn-from-other-repositories).

## Resource-verified fallback and state basis

Moved to [Workspace and index](troubleshooting.md#workspace-and-index); the full rules are in [contracts](contracts/error-codes.md).

## Review and commit workspace changes

Your workspace is a Git repository, and you decide what goes into it. See what changed since your last commit:

```bash
skillhub diff
```

Check that every workspace file is valid:

```bash
skillhub validate
```

```text
Workspace validation passed.
```

Stage the workspace's own folders by name, never the whole folder. `init` and `connect` write connection files into the workspace, and some hold absolute paths for this machine.

```bash
git -C ~/skillhub add .gitignore .skillhub skills sources history registry config distill evals
```

Check exactly what you staged, without touching any other file:

```bash
skillhub validate --staged
```

Then commit:

```bash
git -C ~/skillhub commit -m "Curate skills"
```

`skillhub diff` and `skillhub status` suggest a command that stages everything; stage by name instead. Push to your own remote with `git push` when you want a backup.

To run the check on every commit, add it to a Git hook, such as `.git/hooks/pre-commit` in the workspace:

```sh
#!/bin/sh
skillhub validate --staged
```

Skill Hub does not install hooks for you; any hook manager works.

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
