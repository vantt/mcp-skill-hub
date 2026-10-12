# Sources and learning

Your skills can keep improving after you add them. Skill Hub helps with two jobs: keep copied skills up to date, and learn from what others publish.

A **source** is a Git repository linked to one or more skills. It plays one of two roles:

- An **upstream** is the source a skill was copied from; you can pull its updates.
- A **learning reference** is a source your **curator agent** reads for ideas; nothing is copied. Your curator agent is your own agent, started in the hub folder, where Skill Hub installs the distill-lab skill.

Nothing runs in the background. Skill Hub checks a source only when you run a check, from the terminal or the web UI.

The commands below run inside the hub or a connected project, for example `cd <your-project> && skillhub source list`. Each one that changes files shows a preview first; add `--yes` to apply it.

> [!IMPORTANT]
> Skill Hub never commits or pushes for you, and it never approves content for you. You review each change and commit it yourself.

## Keep copied skills up to date

When you add a skill from GitHub with `skillhub skill add`, the hub remembers where it came from. That repository becomes the skill's upstream, so you can see and pull its changes later. [Add a skill](curating-skills.md#add-a-skill-skillhub-skill-add) shows how to add one.

### Check for updates

See which copied skills differ from their upstream. `--check` contacts each repository first, so the answer is current.

```bash
skillhub skill outdated --check
```

Without `--check`, the hub reports what it learned at the last check. Add `--all` to list skills that are up to date too. In scripts, `--exit-code` exits with 1 when any skill needs attention.

The **STATUS** column uses these words:

| Status | What it means |
|---|---|
| up to date | Your copy matches the latest upstream. |
| update available | Upstream changed and you did not edit your copy. |
| modified locally | You edited your copy; upstream did not change. |
| diverged | Both you and upstream changed the skill. |
| removed upstream | The skill folder no longer exists upstream. |
| pinned | The skill follows one fixed commit, not a branch. |
| unreachable | The hub could not reach the repository. |
| not checked | No check has run yet. |
| not tracked | The hub has no source record to check the skill against. |

### Inspect one skill

See where a skill comes from and how far it is behind.

```bash
skillhub skill upstream <skill-id> --check
```

It prints the source, the repository and folder, your current commit, the latest upstream commit and the status.

### Apply an update

Preview the update first. The hub combines three versions of each file: the one you started from, your copy, and the newest upstream. This is called a three-way merge.

```bash
skillhub skill update <skill-id>
```

The preview lists every file with its result, such as `merged`, `added`, `deleted` or `conflicted`. Nothing changes yet. A file is conflicted when you and upstream changed the same lines.

For each conflicted file, choose what to keep:

- **Take upstream**: `--accept <path>=upstream`
- **Keep mine**: `--accept <path>=local`
- **Edit it yourself**: `--manual <path>=<file>`, where `<file>` is your edited version without conflict markers.

To edit conflicts in your own editor, write them out with Git conflict markers first.

```bash
skillhub skill update <skill-id> --write-conflicts <dir>
```

When every file has a decision, apply the update.

```bash
skillhub skill update <skill-id> --accept <path>=upstream --yes
```

If a file still needs a decision, the hub refuses and says how many files are left. For a file that merged cleanly, `--accept <path>=merged` keeps the merged result.

### Approve the content again

An update never approves content. When the skill comes from someone else's repository, agents cannot use the updated skill until you approve it again. This **content approval** happens only in the terminal; agents cannot do it.

Read the new content and its digest, a fingerprint of the skill's exact files:

```bash
skillhub skill review <skill-id>
```

```text
Content trust
Origin:          third-party
Approval:        review required; agents get no content from this skill (content_review_required)
Content digest:  sha256:<digest>
```

When you are satisfied, approve exactly that content. Here `<digest>` is the full value from the review, including `sha256:`.

```bash
skillhub skill edit <skill-id> --approve-content <digest>
```

### Track an older skill

Skills added with an older version of Skill Hub may show **not tracked**. Preview the fix first; it lists every skill it can link to a source record.

```bash
skillhub source backfill
```

It prints `Nothing to backfill.` when it finds no skill to link.

Apply it to one skill with `skillhub source backfill --skill <skill-id> --yes`. Add `--path <subdir>` when the repository holds several candidate folders. Backfill changes only the skill's upstream entry; other sources stay as they are.

## Learn from other repositories

Some repositories hold good ideas but no skill you want to copy. Link them to a skill as a learning reference. Your curator agent reads their changes and proposes lessons; you decide what to keep.

### Watch a repository for a skill

Watching links a repository to a skill and records how often you want to check it. The interval is a reminder for `--all-due`; nothing runs on a schedule. The skill must already exist.

```bash
skillhub source watch https://github.com/anthropics/skills --skill-id <skill-id> \
  --id anthropic-skills --path skills --cadence weekly --yes
```

```text
Watching source anthropic-skills (https://github.com/anthropics/skills). Cadence: weekly. Changes
are not committed.
Next: skillhub source check anthropic-skills
```

`--cadence` accepts `daily`, `weekly` or `manual`. Every watched source must belong to a skill, so the hub refuses a watch without `--skill-id`.

To link a repository without watching it, use `skillhub source attach <locator> --skill-id <skill-id> --yes`. To unlink it from one skill, use `skillhub source detach <source-id> --skill-id <skill-id> --yes`. To stop watching it, use `skillhub source unwatch <source-id> --yes`; the hub removes the source when no skill uses it.

### Check sources for updates

A check contacts each source and records whether it changed. It never edits your skills. Check only the sources whose interval has passed:

```bash
skillhub source check --all-due
```

Use `--all` to check every watched source, or name sources by ID.

```text
Source checks completed; curated skills are unchanged.
  - anthropic-skills: needs_analysis
```

`needs_analysis` means the check found new commits, so the source is ready to distill. When skills track an upstream, the check also reports whether they changed upstream.

See every source, its skills and its last check:

```bash
skillhub source list
```

```text
1 source from 1 repository; 0 candidates in intake.

https://github.com/anthropics/skills.git
  SOURCE            REF   ROLE             SKILLS      CHECKED   WATCH
  anthropic-skills  main  learning-source  <skill-id>  just now  weekly
```

<img src="images/web/sources.png" alt="The sources list with one source and the result of its last check." width="800"><br><em>Sources: where skills learn from, and when each was last checked.</em>

### Hand changes to your curator agent

Your curator agent reads sources and proposes lessons. Skill Hub does not distill by itself: your agent does it with the distill-lab skill, which Skill Hub installs in the hub folder (`skillhub init` writes it, and `skillhub doctor --fix` adds it to a hub made by an older version). Projects and machines connected with `--hub` do not get it.

1. Start the web UI with `skillhub serve web --addr 127.0.0.1:7421` ([Start the web UI](web-ui.md#start-the-web-ui)).
2. Open **Sources** and choose **Distill with Curator Agent**.
3. Pick the sources to hand over, then choose **Copy handoff**.
4. Start your agent in the hub folder (`cd <hub>`, then your agent) and paste the handoff to it.

distill-lab runs its script with [Node](https://nodejs.org) 18 or newer and needs Git 2.45.0 or newer; there is nothing to install with npm. The skill is self-contained: its folder `.agents/skills/distill-lab/` also works when copied on its own, because the script finds the hub from the `distill.yaml` path you give it. Outside a hub only the evidence check, which reads the source mirrors, is unavailable (`check` and `format` without `--no-where`).

The script itself makes no network call. It never fetches a missing file into a mirror; it reports it as not fetched. The agent's steps that clone and fetch a source mirror do use the network.

> [!WARNING]
> A source is untrusted text, and distilling means your agent reads it. Mirrors are bare clones, so no agent file of the source lands in the hub, and the skill tells the agent never to read `runtime/config/` or `runtime/envs/` while distilling. An agent started in the hub folder can still read those secret files, because skills run there need them, so distill only sources you are willing to show to that agent.

[Hand a source to your curator agent](web-ui.md#hand-a-source-to-your-curator-agent) shows this screen.

### Review the lessons

A **lesson** is a proposed improvement your curator agent records for a skill. It writes lessons to the skill's `.meta/distill.yaml` file.

Read them on the skill's **Distill** tab in the web UI. The tab only shows lessons. To decide, tell your curator agent, for example "1 planned, 3 rejected: already covered".

In the terminal, `skillhub status` in the hub counts the candidate lessons and prints the command that lists them, with the hub's full path so it runs from any folder:

```bash
node '<hub>/.agents/skills/distill-lab/scripts/distill.mjs' list '<hub>/skills/<collection>/<skill>/.meta/distill.yaml'
```

`status` prints that command only while the hub's distill-lab files are the ones Skill Hub installed; otherwise it asks you to run `skillhub doctor`.

Nothing in your skill changes until you act on a lesson. Stage the new lesson file with your other changes. [Review and commit workspace changes](curating-skills.md#review-and-commit-workspace-changes) explains why you add only these paths.

```bash
git -C ~/skillhub add .gitignore .skillhub skills sources history registry config distill evals
```

Then commit:

```bash
git -C ~/skillhub commit -m "Record lessons"
```

## Import many skills from a repository

Copy several skills from one repository at once. Every imported skill arrives as a draft, a skill agents are not offered yet.

Preview what the repository holds:

```bash
skillhub source import https://github.com/anthropics/skills/tree/main/skills/pdf
```

```text
Found 1 skill(s) (1 importable, 0 skipped).
  - pdf -> pdf [draft] (importable)
WARNING: Skill pdf: Declared license "Proprietary. LICENSE.txt has complete terms" may have
         proprietary terms.
No canonical files changed during preview.
```

The preview also prints a `skillhub source import --proposal ... --yes` line. Run it to import exactly what you previewed. Or run the import again with `--yes` to fetch and import in one step.

Choose skills with `--skill <name>` (repeatable) or `--all`, but not both. `--ref <r>` picks a branch or tag; `--path <p>` picks a folder.

Skills you already have are skipped, never overwritten. Import never approves content, activates a skill or watches the source, so `skillhub skill outdated` shows them as **not tracked**. Review and activate each draft as in [Curating skills](curating-skills.md#review-diagnostic-facts-skillhub-skill-review).

Sources larger than 8 MiB are refused. For a large repository, use a URL that points at the skill's folder (`/tree/<branch>/<folder>`), as above.

## Triage candidate repositories

Most people never need this. It suits teams that keep a record of which repositories they considered, and why.

<details>
<summary>Capture, triage and confirm candidates</summary>

Record a candidate without contacting it. This applies at once; it never changes your skills.

```bash
skillhub source capture https://github.com/anthropics/skills --reason "Ideas for our review skills"
```

```text
Source candidate captured; no network fetch or skill changes were made.
Candidate:  SRCQ-<id>
```

List candidates and sources, or show one:

```bash
skillhub source list
```

```text
Intake: SRCQ-<id> https://github.com/anthropics/skills (pending)
```

Use `skillhub source show <id>` for one candidate or source.

Decide each candidate with `skillhub source triage <candidate-id> --decision <decision>`:

| Decision | What it does |
|---|---|
| `accept` | Link it as a learning reference. Needs `--skill-id <skill-id>` or `--new-skill <skill-id>`. |
| `import` | Copy its skills as drafts. Use `--path <p>` to pick a folder. |
| `defer` | Keep it for later. Add `--reason <text>`. |
| `reject` | Turn it down. Add `--reason <text>`. |

```bash
skillhub source triage <candidate-id> --decision defer --reason "Revisit next quarter"
```

```text
Source candidate decision recorded.
```

`accept` prints a proposal instead of applying it. Apply it with the `skillhub source confirm --proposal <id> --proposal-digest <d> --base-version <v>` line it prints. Then import skills from the new source with `skillhub source import <source-id> --path <p> --yes` if you want them.

</details>
