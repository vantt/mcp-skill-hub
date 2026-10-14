# Curating skills

A good collection makes your agent reliably pick the right skill. This guide shows how to build one: create or add a skill, edit and try it, review and approve it, activate it, and check how well skills get picked.

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

Most sessions follow one path: create or add a skill, try it in a project, commit it to the hub, review and approve it if it came from outside, then activate it.

## The four flows

| You want to | Flow | Commands |
|---|---|---|
| Change a skill you have, and try the change | [Edit and try](#edit-and-try-a-skill) | `skill checkout`, edit, try, `skill commit` |
| Write a new skill | [New skill](#create-your-first-skill) | `skill create`, try, `skill commit` |
| Use a skill from GitHub | [Add from GitHub](#add-a-skill-skillhub-skill-add) | `skill add <github-url>` |
| Check and approve what is waiting | [Review and approve](#review-and-approve) | `skill review`, or the web page **Waiting for review** |

A quick fix has a shorter path: fix a routing field with [`skill edit`](#edit-and-improve-a-skill-skillhub-skill-edit), or edit the files in the [web UI](web-ui.md#change-the-files-of-a-skill). Both end in one commit as well.

## Safety model

Skill Hub changes nothing without your say-so. These rules hold for every command:

- **Drafts by default.** Every skill you add or commit as new starts as a **draft**: a skill agents are not offered yet. It becomes **active** only when you activate it.
- **A person approves outside content.** A skill copied from someone else's repository, and anything another machine submits, gives agents nothing until a person approves its content. You approve in a terminal (a `y/N` question) or on the web page **Waiting for review**, after you have read every changed file. Your agent cannot approve; when it commits third-party content it stops and hands over to you.
- **Preview before apply.** Every command that changes the hub shows a preview first. In a terminal it asks `Apply this change? [y/N]` right under it. Without a terminal (an agent, a script) it only previews; add `--yes` to apply exactly that preview. `--diff` adds the full diff.
- **One change, one commit.** Each applied change becomes one Git commit in the hub, with only that change's files. `git revert` undoes it. Skill Hub never pushes.
- **Review changes nothing.** `skillhub skill review` of a skill reports facts; it never approves or activates one.

<details>
<summary>More detail on approval and previews</summary>

- Any change to a copied skill's files or runtime block clears its content approval. Committing a checkout of such a skill approves the new content in the same step, so the skill is never blocked; the hub serves the old version until then.
- Content approval is a guard against mistakes by a cooperative agent. It does not stop a program that can write to the hub folder directly, and the web UI's Approve button works for anyone who can open the web UI. [Keep it safe on your network](web-ui.md#keep-it-safe-on-your-network) says how to keep it on this machine.
- The short form `skillhub skill confirm <proposal-id>` is for you in the terminal. Agents and scripts pass the full `--proposal`, `--proposal-digest` and `--base-version` values from the preview. That way they apply exactly what was previewed.
- If a skill or source with the same ID already exists, the command stops instead of overwriting it.
- `--no-commit` writes the files and skips the commit. Files you edited by hand in the hub folder block a change to the same files until you commit or discard them. See [What gets committed and who gets it](#what-gets-committed-and-who-gets-it).

</details>

## Edit and try a skill

Use this flow to change instructions, scripts or references, and to try the change in a real agent before anyone else gets it. It works for an active skill too: the hub keeps serving the current version until you commit.

1. **Check the skill out.** Run this in your project:

   ```bash
   cd <your-project> && skillhub skill checkout commit-style
   ```

   ```text
   Copied commit-style (active, 3 files) to .claude/skills/commit-style/
   Added .claude/skills/commit-style/ to .gitignore.
   The hub keeps serving the current version; agents in other projects are not affected.
   Try it: ask your agent to use /commit-style.
   When ready: skillhub skill commit commit-style
   ```

   Output is trimmed. The whole folder is copied, scripts and references included. A project connected to a hub on another machine fetches the active skill from that hub.
2. **Edit the files** with your editor or your agent, in `.claude/skills/commit-style/`.
3. **Try it.** Ask your agent to use `/commit-style`. Your agent loads the project copy, so you see the real behavior; other projects still get the hub's version.
4. **Commit it.** Run `skillhub skill commit commit-style`. It lists every changed file with the lines gained and lost, runs the routing lint, and in a terminal asks `Apply this change? [y/N]`. If the hub changed the same file meanwhile, it shows a three-way merge and asks for each file: `--accept <path>=hub` or `=project`, `--manual <path>=<file>`, or `--write-conflicts <dir>` to write the merge with markers. After the commit it says who gets the new version.

```text
Committed 4be19c0 "Commit commit-style from a project (3 files)".
Agents using this hub get it now; other clones after `git -C <home>/skill-hub push`.
```

Run `skillhub skill commit commit-style --diff` first to read the full diff. `--runtime-file <yaml>` declares the tools a new skill needs. A skill from someone else's repository needs your approval of the new content: the terminal asks, and without a terminal the preview prints the `--approve-content <digest>` line for you to run after reading the diff.

## Create your first skill

A **skill** is a folder with a `SKILL.md` your agent follows and a few routing fields that say when it applies. This one teaches your team's commit style. Its last rule adds a marker line, so you can tell when the skill was used.

Start it inside your connected project. Nothing changes in the hub yet: the command writes `.claude/skills/<id>/SKILL.md` in the project. In a terminal, run `skillhub skill create` and answer the questions: what the skill does, when an agent should use it, when it should not, and an example request.

```bash
cd <your-project> && skillhub skill create
```

Scripts and agents, which cannot answer questions, pass flags instead. `--trigger` says when the skill applies, `--not-for` when it does not, and `--example` gives a request that should pick it:

```bash
cd <your-project> && skillhub skill create commit-style --name "Commit style" \
  --description "Write Git commit messages in our team style" \
  --trigger "write a commit message" --not-for "writing release notes" \
  --example "write a commit message for my staged changes"
```

Open `.claude/skills/commit-style/SKILL.md` and replace the starter text with these rules (an agent can write the folder itself, without the form):

```markdown
# Commit style

Use this skill when you write a Git commit message.

1. Start with a short summary line in the imperative mood, at most 50 characters.
2. Leave one blank line, then explain what changed and why in plain sentences.
3. End every commit message with this exact line:

   Checked-by: skillhub-demo
```

### See your first win

Try it before it reaches the hub. Your agent needs a staged change to write about: change a file in the project and stage it with `git add <file>`. Then ask your agent:

> use /commit-style to write a commit message for my staged changes

If your agent used the skill, the commit message ends with this line:

```text
Checked-by: skillhub-demo
```

### Commit the new skill

`skill commit` brings the folder to the hub as a new draft. A folder that was never checked out is a new skill.

```bash
cd <your-project> && skillhub skill commit commit-style
```

In a terminal the preview lists every file and anything left out, suggests routing (trigger, not-for, examples) from the description, shows the routing lint, and then asks `Apply this change? [y/N]`, followed by `Activate commit-style now?` when the skill is ready. Everything becomes one hub commit. Change the suggested routing with `--edit-routing` (it opens `$EDITOR`) or with `--trigger`, `--not-for`, `--example` and `--min-scope`. Without a terminal it stops at the preview; apply it with `--yes`, then activate it:

```bash
cd <your-project> && skillhub skill commit commit-style --yes
skillhub skill activate commit-style --yes
```

```text
Skill commit-style is now active.
```

[How the agent picks a skill](user-guide.md#how-the-agent-picks-a-skill) shows how to run the same lookup by hand.

## Add a skill (`skillhub skill add`)

`skill add` brings in a skill that lives in a GitHub repository. The skill arrives as a draft, and the repository becomes its **upstream**: the source it was copied from, so you can pull its updates later. For a skill in a folder on this machine, use [`skill commit <folder>`](#commit-a-folder-you-already-have) instead.

Point at the skill's folder in the repository with `--path`. A folder URL such as `https://github.com/anthropics/skills/tree/main/skills/pdf` works too. In a terminal, `skill add` previews and then asks, one question at a time:

```bash
skillhub skill add https://github.com/anthropics/skills --path skills/pdf
```

```text
Add pdf from https://github.com/anthropics/skills.git at <commit>, tracked for upstream updates.
Files (11, 57.3 KB):
  SKILL.md           9.9 KB
  LICENSE.txt        1.4 KB
  scripts/extract.py 4.2 KB
  ...one line for each of the other 8 files
License: Apache-2.0
Routing suggested from the description:
  trigger: ...
Routing lint: no findings.
Third-party: agents get pdf only after you approve its content.
Upstream: tracked by new source anthropics-skills (checked weekly by `skillhub source check`; nothing runs in the background).
Show every file: skillhub skill add https://github.com/anthropics/skills --path skills/pdf --diff
Add pdf as a draft? [y/N]
Approve this content for agents? [y/N]
Activate pdf now? [y/N]
```

Output is trimmed. The preview names every file, the files left out (`.env*`, `*.pem`, SSH private keys, `node_modules/` and similar) and the license, so you can judge the skill before you answer. All your answers become one hub commit. The approval question appears for a skill from someone else's repository; read the files first (`--diff` shows them all).

Without a terminal (an agent, a script) `skill add --yes` adds the draft and stops there: content approval is a person's step, on the review screen or in the terminal. For a repository with several skills, `--skill <name>` picks one and `--all` adds every skill as its own draft. Repositories larger than 8 MiB are refused with the same advice: point at the skill's folder as above. `--ref <branch-or-tag>` picks another branch or tag.

### Commit a folder you already have

A folder with a `SKILL.md` on this machine, such as `~/review-checklist`, is added with `skill commit`. Run it from a project that is connected to your hub:

```bash
cd <your-project> && skillhub skill commit ~/review-checklist
```

The preview lists every file with its size, scripts keep their executable bit, and the files that look like secrets or build output are named under `Left out`, so you can see what did not come along. Committing the same folder again is an update with a diff, not a conflict. A skill from your own folder needs no content approval.

### After you add a skill

A new draft needs three things before agents can use it:

1. **Review it.** Run `skillhub skill review <skill-id>` and read what it reports.
2. **Fill in its routing fields**, if the review lists any. Add a trigger, a `--not-for` entry and `--min-scope` with `skillhub skill edit`. See [Edit and improve a skill](#edit-and-improve-a-skill-skillhub-skill-edit).
3. **Activate it.** Run `skillhub skill activate <skill-id> --yes` (the terminal asks before it applies). See [Activate, deprecate, and archive](#activate-deprecate-and-archive).

## Review and approve

A person looks at what changed before agents get it. Two screens do this, with the same facts: the terminal and the web page **Waiting for review** ([guide](web-ui.md#review-what-waits-for-you)).

- **Terminal.** `skillhub skill review <skill-id>` reports one skill's facts (below). The commit and add commands ask for approval in their own flow, right after their preview.
- **Web.** **Waiting for review** lists new drafts, skills whose content changed after you approved it, and skills sent from other machines. Each opens a full review with every file, its diff and the license. **Approve** stays off until you have opened every changed file.

An agent never approves. When it commits third-party content, it stops with the digest line for a person.

## From another machine

A project connected to a hub on another machine (`skillhub connect --hub <url>`) uses the same two commands. `skillhub skill checkout <skill-id>` fetches the active skill from that hub. That sends your hub token, so for a hub that only this project's files name (not your own settings) the terminal asks first. `skillhub skill commit <skill-id|folder>` sends the folder there, and says `Sent as SUB-<id>`. Nothing on the hub changes and nothing is served until a person on the hub machine reviews it:

```bash
skillhub skill review SUB-<id>            # preview, like a commit
skillhub skill review SUB-<id> --diff     # with the full diff
skillhub skill review SUB-<id> --discard  # drop it
```

In a terminal on the hub machine, `skill review SUB-<id>` asks `Apply this change? [y/N]`, then approves the content and, for a new skill, asks whether to activate it; it is one hub commit. The same submission opens on the web page **Waiting for review**, and `skillhub status` on the hub machine counts what waits. The sender chooses neither the collection nor the routing; the reviewer sets them. [One hub from several machines](remote-hub-guide.md) covers the setup and the security notes.

## What gets committed and who gets it

Every change you apply, whether from the CLI, the web UI or an agent, becomes one Git commit in the hub:

- **Scoped.** The commit holds only the files of that change. Other files in the hub, staged or not, stay as they were.
- **Yours.** Your Git hooks run, and your signing applies. Skill Hub writes the message. If signing needs a passphrase and there is no terminal, the files are still written and the command prints the line that finishes the commit.
- **Not pushed.** Agents on this machine get the new version at once. Other clones get it after you push: the result says so and prints the command, for example `git -C ~/skill-hub push`.
- **Refused for dirty files.** If a file the change touches already differs from Git (you edited it by hand), the command refuses and names the files. Commit them, discard them, or run the command again with `--no-commit`.

`--no-commit` writes the files and leaves the commit to you, for example to gather several changes into one. Then add only the hub's own paths by name, never the whole folder: `init` writes connection files into the hub, and some hold absolute paths for this machine.

### Working inside the hub folder

You can still edit the files in the hub folder directly. It is possible but not the official path: nothing previews or tries the change, and the hub serves it at once. Commit the files yourself. `skillhub status` reports the uncommitted files and the Git line that commits them; a search index that is out of date because of hand edits shows as a notice, not as `FAILED`. Prefer `skill checkout` and `skill commit`, or the web UI.

## Review diagnostic facts (`skillhub skill review`)

Run a review before you activate a skill and after every change. It reads the hub files and changes nothing. `skill commit` and `skill add` print the same routing lint warnings in their preview, such as a trigger that collides with another skill's and missing examples:

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

Under the **Routing lint** heading, review also tries the skill's own examples. It resolves them against the active library as if this version of the skill were active, and names the skill each example picked when it was not this one. It lists the examples of other skills that the change would take over as well. It records nothing and rebuilds nothing, so you can run it on an edit you have not committed. For example, after adding the trigger "tidy up the changelog entries for the version" to `release-notes`, which `changelog-curation` lists as its own example:

```text
Routing lint
  - example_taken_by: example "tidy up the changelog entries for the version" of
    "changelog-curation" now resolves to "release-notes" Fix: Add a not_for entry on release-notes
    that covers this request, narrow release-notes's triggers, or add distinguish_from between
    release-notes and changelog-curation
```

The `skill commit` preview and the web **Review** screen show the same findings. They are warnings: they never block a commit or an approval.

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

The digest is a fingerprint of the files you reviewed. Read them, then approve in the terminal: `skill add`, `skill commit` and `skill review <SUB-id>` ask `Approve this content for agents?` right after their preview, and the command above does it by hand. After an approved skill changes, the block lists what changed and a `git diff` command to inspect it. The web UI shows the same facts in a skill's **Review** tab and approves from **Waiting for review**, once you have opened every changed file.

## Edit and improve a skill (`skillhub skill edit`)

`skill edit` is the quick path for the description and routing fields, to sharpen when a skill gets picked. To change instructions, scripts or references and try them first, [check the skill out](#edit-and-try-a-skill) instead. Each edit shows a preview first, and in a terminal asks `Apply this change? [y/N]`; the applied edit is one hub commit.

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

Edit the instructions quickly in your editor (`$VISUAL` or `$EDITOR`), without trying them first:

```bash
skillhub skill edit review-checklist --editor
```

If the skill changes while your editor is open, Skill Hub refuses the edit. Your text stays in a recovery file under `runtime/edits/` in the hub. To replace the instructions from a file instead, use `--content-file <file>`.

Declare the tools and variables a skill's scripts need with a runtime file:

```bash
skillhub skill edit review-checklist --runtime-file ~/runtime.yaml --yes
```

The file is a YAML mapping such as `requires: {bins: [git]}`; a file that holds `{}` removes the block. Changing it clears content approval on a skill from GitHub. [Run skills that need tools or secrets](user-guide.md#run-skills-that-need-tools-or-secrets) explains the rest.

Without a terminal and without `--yes`, every edit prints a preview and a line to apply it later:

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

Preview a change by leaving out `--yes` (in a terminal the command then asks `Apply this change? [y/N]`). Apply it without a terminal with `--yes`:

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

## Check the hub files

`skillhub maintain diff` shows what differs from your last commit, for example files you edited by hand. `skillhub maintain validate` checks that every hub file is valid:

```bash
cd ~/skill-hub && skillhub maintain validate
```

```text
Workspace validation passed.
```

To run the check on every commit, add it to a Git hook, such as `.git/hooks/pre-commit` in the hub:

```sh
#!/bin/sh
skillhub maintain validate --staged
```

The hook also warns when a staged skill's routing change makes one of its examples, or another skill's example, resolve to a different skill. These warnings never block the commit. Skill Hub's own commits run that hook too. Skill Hub does not install hooks for you; any hook manager works. A hook written for an earlier release runs the old top-level validate command, which no longer exists. `skillhub doctor` reports such a hook, and `skillhub doctor --fix` changes that one command and keeps the rest of the hook.

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

### Improve routing from usage

Agents often tell you what your routing is missing without meaning to: they ask for a skill and get none, they reject a recommendation and phrase the task again, or they load a skill the resolver never suggested. `skillhub telemetry cases suggest` reads those moments and proposes one routing entry at a time, so you do not have to guess which examples to write.

It looks for three signals. A task that found no skill, followed by loading skill X, suggests adding that task as an example of X. A rejected recommendation of X, followed by loading Y for the original task, suggests a counter-example on X and an example on Y. Three or more such rejections of X from at least two sessions suggest one `not_for` entry on X instead.

This only works while the case journal is on, because the task text comes from it. Run `skillhub telemetry cases enable` once; only sessions after that count. Nothing changes until you confirm: `suggest` lists each entry with the reason and the evidence (how many events and sessions, and when), and phrasings with a redacted part are never offered, so private text cannot move into Git.

```bash
skillhub telemetry cases suggest
skillhub skill edit code-review --from-suggestion rs_3f9a0c41d27b8e55a610
```

The second command shows the exact line it would add, the reason, and a routing check that replays the change against the skills agents are served now. Read the warnings: a warning names another skill whose example would start resolving to this skill. In a terminal it then asks `Apply this change? [y/N]`; without one, apply it with `skillhub skill confirm` or `--yes`. The check covers the examples of your skills, so it is approximate evidence about real requests; run `skillhub eval routing` after confirming. If a suggestion is wrong, `skillhub telemetry cases dismiss <suggestion-id>` hides it on this machine.

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

`skillhub maintain validate` checks the examples of the skills you changed since the last commit, `--full` checks every active skill, and `eval routing` gives the totals and thresholds. Output is trimmed. Each failure is a request that did not pick its skill; fix it with a better trigger or more examples. `--json` puts the numbers under `metrics`, for example `metrics.recall`.

To use it as a gate, add thresholds such as `--min-precision 0.60` or `--min-recall 0.58`. A missed threshold exits with code 1. `--no-skill <file>` adds requests that should pick no skill.

## Import repository skills as drafts (`skillhub source import`)

Moved to [Import many skills from a repository](sources-and-learning.md#import-many-skills-from-a-repository).

## Advanced intake and recovery

Moved to [Triage candidate repositories](sources-and-learning.md#triage-candidate-repositories).

## Command map

Moved to [Command reference](command-reference.md).
