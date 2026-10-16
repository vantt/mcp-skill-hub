# Curating skills

This guide shows how to grow the skills your agent uses: write a new one, improve one you have, bring one in from GitHub, and approve what others send. Each task below is a short path you can copy and run; flags and tuning come after them, under [More](#more).

Finish the [Quickstart](../README.md#quickstart) first. Then run the commands below in any project folder you work in (`<your-project>`), not in `~/skill-hub`: with `connect -g` every project uses your hub. (If you connected only one project with `skillhub connect`, use that project.)

You can also ask your agent: **curate my Skill Hub**. It checks the hub and suggests one next step, using the same commands as this guide.

## Create your first skill

Write a skill in your project, try it with your agent, then publish it so every project gets it. A **skill** is a folder with a `SKILL.md` file of instructions your agent follows. This one teaches your team's commit style.

**1. Start the skill.** In a terminal, the command asks four questions: what the skill does, when an agent should use it, when it should not, and an example request. Press Enter on an empty line to finish a list.

```bash
cd <your-project> && skillhub skill create commit-style
```

```text
What does this skill do? Write Git commit messages in our team style
When should an agent use it? (empty line to finish) write a commit message
Another one?
When should it not? (empty line to finish) writing release notes
Another one?
One or two example requests: write a commit message for my staged changes
Another one?
Created .claude/skills/commit-style/SKILL.md (nothing changed on the hub).
Write the instructions there and try them: ask your agent to use /commit-style.
When ready: skillhub skill commit commit-style
```

Nothing reaches the hub yet: the skill lives only in this project.

<details>
<summary>Without a terminal (an agent or a script)</summary>

Pass the answers as flags. Each flag can repeat:

```bash
cd <your-project> && skillhub skill create commit-style \
  --description "Write Git commit messages in our team style" \
  --trigger "write a commit message" --not-for "writing release notes" \
  --example "write a commit message for my staged changes"
```

</details>

**2. Write the instructions.** Open `.claude/skills/commit-style/SKILL.md`. Keep the top of the file: the sections **When to use**, **When not to use** and **Examples** hold your answers, and they tell the hub when to recommend this skill. Replace only the placeholder under **Instructions** (the `<!-- skillhub:scaffold -->` comment and the paragraph under it) with:

```markdown
1. Start with a short summary line in the imperative mood, at most 50 characters.
2. Leave one blank line, then explain what changed and why in plain sentences.
3. End every commit message with this exact line:

   Checked-by: skillhub-demo
```

Add two more requests under **Examples**, such as `- write the commit message for this fix` and `- describe my staged changes as a commit`. Skill Hub warns until a skill has at least three examples; it does not block you.

**3. Try it.** Stay in your project; do not run these in `~/skill-hub`. Change a file and stage it (`skill create` keeps its folder out of Git with `.gitignore`):

```bash
cd <your-project> && git add <your-file>
```

Then ask Claude Code:

> use /commit-style to write a commit message for my staged changes

If your agent used the skill, the message ends with `Checked-by: skillhub-demo`. Not right yet? Edit `SKILL.md` and ask again.

**4. Publish it.**

```bash
cd <your-project> && skillhub skill commit commit-style
```

The command shows what it will add and asks two questions. Answer `y` to both (output trimmed):

```text
commit-style is not on the hub: this adds a new skill (collection: default).
Files (1, 623 B):
  SKILL.md  623 B
Add commit-style as a draft? [y/N] y
Activate commit-style now? [y/N] y
Committed da67f76 "Add commit-style from a project, activate".
Agents using this hub get it now.
```

A new skill starts as a **draft**, which agents are not offered; **activating** it lets the hub recommend it. Both answers become one Git commit in your hub, which Skill Hub makes for you.

**What happens next.** Every agent that uses this hub gets the skill now, in every project. If your hub has a Git remote, other clones get it after you push; the output prints the push command. The folder stays in your project and now counts as a checkout (next task), so you can keep improving it and run `skill commit` again. Delete the folder when you are done, so this project uses the hub's version like the others.

<details>
<summary>Without a terminal (an agent or a script)</summary>

Without a terminal, `skill commit` only shows the preview. Apply it with `--yes`, then activate:

```bash
cd <your-project> && skillhub skill commit commit-style --yes
skillhub skill activate commit-style --yes
```

```text
Skill commit-style is now active.
```

Claude Code asks you before it runs a `skillhub` command with `--yes`.

</details>

## Edit a skill and try it before others get it

Change any file of a skill, scripts included, and try the change with your agent while every other project keeps the version it has.

**1. Copy the skill into your project.** Just created the skill? The folder is already a checkout: go to step 2. Otherwise:

```bash
cd <your-project> && skillhub skill checkout commit-style
```

```text
Copied commit-style (active, 1 file) to .claude/skills/commit-style/
Added .claude/skills/commit-style/ to .gitignore.
The hub keeps serving the current version; agents in other projects are not affected.
Try it: ask your agent to use /commit-style.
When ready: skillhub skill commit commit-style
```

**2. Edit the files** in `.claude/skills/commit-style/`, yourself or with your agent. For example, change "at most 50 characters" to "at most 72 characters" in `SKILL.md`, and add a script `scripts/trailer.sh`:

```sh
#!/bin/sh
echo "Checked-by: skillhub-demo"
```

A file that starts with `#!` becomes executable in the hub, so you need no `chmod`.

**3. Try it.** Ask your agent to use `/commit-style`. It reads the copy in this project, so you see the new behaviour before anyone else does.

**4. Publish the change.**

```bash
cd <your-project> && skillhub skill commit commit-style
```

```text
commit-style: 2 files changed since checkout (the hub did not change)
  M SKILL.md            +1 -1
  A scripts/trailer.sh  +2
Routing lint: no findings.
Show the diff: skillhub skill commit commit-style --diff
Apply this change? [y/N] y
Committed a7ba5e5 "Commit commit-style from a project (2 files)".
Agents using this hub get it now.
```

Add `--diff` to read every changed line before you answer.

**What happens next.** Agents on this hub get the new version now; other clones after you push. Run `skill commit` again after more edits; with nothing changed it says `Nothing to commit`. `skillhub status` lists the skills checked out in this project and how many files changed in each.

- **Routing** is what the hub uses to decide when to recommend a skill: its triggers, "not for" entries and examples. The **When to use**, **When not to use** and **Examples** sections of `SKILL.md` were read as routing suggestions on the skill's first commit only. To change routing later, use `skill edit`: see [Tune routing](#tune-routing-skillhub-skill-edit).
- **A skill from GitHub** asks one more question, `Approve this content for agents?`, because agents get nothing from changed third-party content until a person approves it. See [Approve and publish](#approve-and-publish).
- **The hub changed meanwhile** (someone else committed the same skill): `skill commit` merges both versions and asks you to decide each file that both sides changed. `skillhub help skill commit` lists the choices.

## Add a skill from GitHub

Bring in a skill someone published on GitHub. A skill written by someone else is **third-party**: agents get none of its content until you have read it and approved it.

**1. Read it.** Point at the skill's folder in the repository and page through every file. Piped into a pager, the command only shows; it writes nothing:

```bash
cd <your-project> && skillhub skill add https://github.com/anthropics/skills --path skills/webapp-testing --diff | less
```

A folder address such as `https://github.com/anthropics/skills/tree/main/skills/webapp-testing` works too. The preview lists every file, the license, and the files left out because they look like secrets or build output (`.env*`, `*.pem`, SSH keys, `node_modules/`).

**2. Add it and approve what you read.** Run the command again without `--diff`, and answer `y` to both questions (output trimmed):

```bash
cd <your-project> && skillhub skill add https://github.com/anthropics/skills --path skills/webapp-testing
```

```text
Add webapp-testing from github.com/anthropics/skills@main, tracked for upstream updates.
Files (6, 21.9 KB):
  LICENSE.txt                         11.1 KB
  SKILL.md                             3.8 KB
  ...
License: Complete terms in LICENSE.txt (LICENSE.txt)
Third-party: agents get webapp-testing only after you approve its content.
Add webapp-testing as a draft? [y/N] y
Approve this content for agents? [y/N] y
webapp-testing cannot be activated yet: trigger, not_for or rationale
(quality.routing_review_rationale).
Draft webapp-testing added and its content approved. Agent use: off. Upstream: anthropics-skills.
Committed d6ea5c0 "Add webapp-testing from github.com/anthropics/skills, approve content".
```

**3. Say when to use it, then activate it.** A skill from GitHub rarely says when to use it in a form the hub can read, so it cannot be activated yet. Add a trigger, a "not for" entry and three example requests, then activate:

```bash
cd <your-project> && skillhub skill edit webapp-testing --trigger "test a local web app in a browser" \
  --not-for "unit tests without a browser" --example "test my web app in a browser" \
  --example "check the login page with Playwright" --example "take a screenshot of my local site"
skillhub skill activate webapp-testing
```

Each command shows its change and asks `Apply this change? [y/N]`; each applied change is one hub commit.

**What happens next.** Agents get the skill once it is active. The repository becomes the skill's **upstream**, so you can pull its later changes: see [Keep copied skills up to date](sources-and-learning.md#keep-copied-skills-up-to-date). For a repository with several skills, `--skill <name>` picks one and `--all` adds each as its own draft; `skillhub help skill add` lists the rest.

**A skill in a folder on this machine**, such as `~/review-checklist`, is added with `skill commit` instead. It needs no approval, because it is yours:

```bash
cd <your-project> && skillhub skill commit ~/review-checklist
```

## Approve and publish

Some skills wait for a person before agents get them: a third-party skill an agent added, and skills [sent from another machine](#from-another-machine). Agents cannot approve; you do it in a terminal or in the browser.

**1. See what waits.** Open **Waiting for review** in the [web UI](web-ui.md#review-what-waits-for-you), or list the drafts:

```bash
cd <your-project> && skillhub skill list --state draft
```

**2. Read the skill and approve it.** Say your agent added a skill from GitHub with `skillhub skill add https://github.com/anthropics/skills --path skills/frontend-design --yes`. Without a terminal it stopped at the draft, unapproved. `skill review` says what the skill still needs and how to approve it (output trimmed):

```bash
cd <your-project> && skillhub skill review frontend-design
```

```text
Activation readiness
  - Missing required field: trigger
  - Missing required field: not_for or rationale (quality.routing_review_rationale)
Content trust
Origin:          third-party
Approval:        review required; agents get no content from this skill (content_review_required)
Content digest:  sha256:<digest>
After reviewing the skill's files and runtime block, approve exactly this content with:
  $ skillhub skill edit frontend-design --approve-content sha256:<digest>
```

The digest is a fingerprint of the files you read. Read them in the hub folder (`skills/<collection>/<skill-id>/`) or in the web UI, then copy the approval command from your own output; it asks `Apply this change? [y/N]`. On the web page **Waiting for review**, **Approve** turns on once you have opened every changed file.

**3. Say when to use it, then activate it.** Fill in what **Activation readiness** lists: a trigger, a "not for" entry and three example requests:

```bash
cd <your-project> && skillhub skill edit frontend-design --trigger "design a web page or user interface" \
  --not-for "backend code without a user interface" --example "design a landing page for my app" \
  --example "make this dashboard look less generic" --example "choose fonts and colours for my site"
skillhub skill activate frontend-design
```

Each command asks `Apply this change? [y/N]`. Once the skill is active, agents on this hub get it.

## Edit in the browser

For a quick fix without trying it in an agent first, edit the files in the web UI. Start it from your hub folder:

```bash
cd ~/skill-hub && skillhub serve web
```

It opens your browser. If no browser opens, open the link it prints. Anyone who has that link can change your skills; to keep the web UI on this machine, see [Keep it safe on your network](web-ui.md#keep-it-safe-on-your-network).

1. Open **Skills**, click the skill, then the **Files** tab.
2. Click a file and **Edit** it, or add one with **+ File**. A bar at the bottom counts your changes: "2 changes not committed · agents still get the current version".
3. Click **Review →**, read each file's diff, and click **Commit to the hub**. The page answers "Committed c492ab3. This machine's agents get it now."

[Change the files of a skill](web-ui.md#change-the-files-of-a-skill) and [Review what waits for you](web-ui.md#review-what-waits-for-you) show each screen. Press `Ctrl+C` in the terminal to stop the web UI.

## More

The tasks above cover daily curation. The sections below explain what happens underneath and how to tune routing. For every flag of a skill command, run `skillhub help skill <command>`, for example `skillhub help skill commit`, or see the [command reference](command-reference.md#work-with-skills).

- Pull updates from a skill's upstream: [Keep copied skills up to date](sources-and-learning.md#keep-copied-skills-up-to-date).
- Learn from other repositories without copying them: [Learn from other repositories](sources-and-learning.md#learn-from-other-repositories).
- Import many skills at once: [Import many skills from a repository](sources-and-learning.md#import-many-skills-from-a-repository).
- Fix an error message: [Troubleshooting](troubleshooting.md).

## From another machine

A project connected to a hub on another machine (`skillhub connect --hub <url>`) uses the same two commands. `skill checkout` fetches the active skill from that hub, and `skill commit` sends your folder there for review:

```text
Send commit-style to the hub at <hub-url> for review? [y/N] y
Sent as SUB-01M51D3QYM6Z5EBPVEKC644XH6. A person on the hub machine reviews it; agents get it only
after approval.
```

The hub shows the submission as sent from this machine's host name; add `--label <name>` (at most 64 printable characters, for example `--label laptop`) to choose the name it shows instead.

Nothing on the hub changes until a person on the hub machine reviews it. `skillhub status` there lists what waits. Review it in a terminal on the hub machine; it asks `Apply this change? [y/N]` and `Approve this content for agents? [y/N]`, and both answers make one hub commit:

```bash
skillhub skill review SUB-<id>            # preview, then the two questions
skillhub skill review SUB-<id> --diff     # with the full diff
skillhub skill review SUB-<id> --discard  # drop it
```

The same submission opens on the web page **Waiting for review**. The sender chooses neither the collection nor the routing; the reviewer sets them. [One hub from several machines](remote-hub-guide.md) covers the setup and the security notes.

## Retire a skill

A skill moves through four states in this order: **draft**, **active**, **deprecated** (being phased out) and **archived** (kept for history, never offered). Retire one in two steps:

```bash
cd <your-project> && skillhub skill deprecate <skill-id>
skillhub skill archive <skill-id>
```

An active skill cannot go straight to archived.

## What gets committed and who gets it

Every change you apply, from the CLI, the web UI or an agent, becomes one Git commit in the hub:

- **Scoped.** The commit holds only the files of that change. Other files in the hub, staged or not, stay as they were. `git revert` undoes it.
- **Yours.** Your Git hooks run, and your signing applies. Skill Hub writes the message. If signing needs a passphrase and there is no terminal, the files are still written and the command prints the line that finishes the commit.
- **Not pushed.** Agents on this machine get the new version at once. Other clones get it after you push: the result says so and prints the command, for example `git -C ~/skill-hub push`.
- **Refused for dirty files.** If a file the change touches already differs from Git (you edited it by hand), the command refuses and names the files. Commit them, discard them, or run the command again with `--no-commit`.

`--no-commit` writes the files and leaves the commit to you, for example to gather several changes into one. Then add only the hub's own paths by name, never the whole folder: `init` writes connection files into the hub, and some hold absolute paths for this machine.

### Working inside the hub folder

You can still edit the files in the hub folder directly. It is possible but not the official path: nothing previews or tries the change, and the hub serves it at once. Commit the files yourself. `skillhub status` reports the uncommitted files and the Git line that commits them; a search index that is out of date because of hand edits shows as a notice, not as `FAILED`. Prefer `skill checkout` and `skill commit`, or the web UI.

## Previews, confirmation and agents

Every command that changes the hub shows a preview first. In a terminal it asks right under the preview. Without a terminal (an agent, a script) it only previews and prints the command that applies it; add `--yes` to apply exactly that preview. `--diff` adds the full diff to any preview.

- Claude Code asks you before any `skillhub` command with `--yes` or `confirm`, and before commands that change the hub, its connections, the token or the cache. Codex and Gemini CLI do not ask first.
- An agent never approves third-party content: Claude Code blocks `--approve-content`, and without a terminal `skill add --yes` stops at the draft. When an agent commits changed third-party content, it stops with the digest line for you.
- Some previews print a short form, `skillhub skill confirm <proposal-id>`, for you in the terminal. Agents and scripts pass the full `--proposal`, `--proposal-digest` and `--base-version` values from the preview, so they apply exactly what was previewed.
- If a skill or source with the same ID already exists, the command stops instead of overwriting it.
- Any change to a third-party skill's files or runtime block clears its content approval. Committing a checkout of such a skill approves the new content in the same step, so the hub keeps serving the old version until then and the skill is never blocked.
- Content approval guards against mistakes by a cooperative agent. It does not stop a program that can write to the hub folder directly, and the web UI's **Approve** button works for anyone who can open the web UI. [Keep it safe on your network](web-ui.md#keep-it-safe-on-your-network) says how to keep it on this machine.

## Tune routing (`skillhub skill edit`)

`skill edit` changes a skill's description and routing fields, to sharpen when the hub recommends it. Fields you leave out keep their values, but each flag you pass replaces its whole field. To add one example, pass every example you want to keep; the current ones are in `.meta/skill.yaml` in the skill's hub folder.

```bash
cd <your-project> && skillhub skill edit commit-style --trigger "write a commit message" \
  --not-for "writing release notes" --min-scope single_step
```

Examples are requests that should pick the skill; counter-examples are requests that should not. Each flag can repeat, up to 10 of each:

```bash
cd <your-project> && skillhub skill edit commit-style \
  --example "write a commit message for my staged changes" \
  --example "write the commit message for this fix" \
  --example "describe my staged changes as a commit" \
  --counter-example "write release notes"
```

An active skill needs a trigger, a "not for" entry (or a `--rationale`) and a minimum scope. Skill Hub warns until a skill has at least three examples; it does not block you. Other forms:

- `skillhub skill edit <skill-id> --editor` edits the instructions in `$VISUAL` or `$EDITOR`, without trying them first. If the skill changes while the editor is open, the edit is refused and your text stays in a recovery file under `runtime/edits/` in the hub. `--content-file <file>` replaces the instructions from a file.
- `skillhub skill edit <skill-id> --runtime-file <yaml>` declares the tools and variables the skill's scripts need, as a YAML mapping such as `requires: {bins: [git]}`; a file that holds `{}` removes the block. [Run skills that need tools or secrets](user-guide.md#run-skills-that-need-tools-or-secrets) explains the rest.
- `skill commit` of a new skill takes the same routing flags (`--trigger`, `--not-for`, `--example`, `--min-scope`), and `--edit-routing` opens the suggested routing in `$EDITOR`.

[How the agent picks a skill](user-guide.md#how-the-agent-picks-a-skill) shows how to run the hub's lookup by hand.

### Routing lint

`skill commit`, `skill add`, `skill review` and the web **Review** screen run the same routing lint: warnings such as too few examples, a trigger made only of generic words, or a trigger that collides with another skill's. Warnings never block a commit or an approval.

The lint also tries the skill's own examples against the active library, as if this version were active, and names the skill each example picked when it was not this one. It lists the examples of other skills the change would take over too. It records nothing, so you can run `skill review` on an edit you have not committed. For example, after adding the trigger "tidy up the changelog entries for the version" to `release-notes`, which `changelog-curation` lists as its own example:

```text
Routing lint
  - example_taken_by: example "tidy up the changelog entries for the version" of
    "changelog-curation" now resolves to "release-notes" Fix: Add a not_for entry on release-notes
    that covers this request, narrow release-notes's triggers, or add distinguish_from between
    release-notes and changelog-curation
```

`skillhub skill review <skill-id> --verbose` also lists every file with its size.

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

Test whether your active skills' examples pick the right skill. Each example is matched with itself left out, both from its skill and from the search index, so it cannot find its skill through its own words and the score shows how well new requests will match:

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

## Measure usage (`skillhub telemetry funnel`)

See how often each skill is recommended, used and blocked over the last 30 days. It also lists skills nobody uses and skills blocked because their content is not approved yet. Narrow it with `--since 90d` or `--skill <skill-id>`:

```bash
skillhub telemetry funnel
```

To count skill use from your Claude Code history, import its transcripts. It reads only tool-call entries, and the usage record stays on your machine unless you export it:

```bash
skillhub telemetry import-transcripts --project ~/myproject
```

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
