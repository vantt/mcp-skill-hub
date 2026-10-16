# The web UI

The web UI lets you look after your skills by clicking instead of typing. It runs on your own machine, in your browser, and reads the same hub as the `skillhub` command.

> [!IMPORTANT]
> The web UI never pushes. Every change you confirm becomes one Git commit in your hub, and the page says who gets it: this machine's agents right away, other machines after you push. Third-party content is approved only by you, on a screen where you read every changed file first.

## Start the web UI

Run this in your hub folder. It opens your browser:

```bash
cd ~/skill-hub && skillhub serve web
```

If no browser opens, open the link it prints (output trimmed):

```text
Skill Hub web UI: http://127.0.0.1:7421/#token=<token>
```

The token at the end changes each time you start it. Anyone who has the link can change your skills, so treat it like a password; to keep the web UI on this machine, see [Keep it safe on your network](#keep-it-safe-on-your-network). Press `Ctrl+C` in the terminal to stop the web UI.

## Do common tasks

The pictures come from a demo hub with invented skills; your screens list your own skills. Each task starts from the menu on the left: **Home**, **Skills**, **Waiting for review** and **Sources**.

### See what needs your attention

Open **Home**. The **Next action** box at the top says what to do next and has one button for it.

Under it, **Workspace** shows the hub's health, the search index and whether Git has uncommitted changes. **Overview** counts the sources ready for your curator agent and the lessons waiting for your decision.

<img src="images/web/home.png" alt="The Skill Hub home screen showing the status of the hub and the next action." width="800"><br><em>Home: what needs attention, and the one next step to take.</em>

### Find a skill

1. Open **Skills**.
2. Type part of a name or id in **Search**, or narrow the list with **Lifecycle**, **Collection** or **Upstream**.
3. Click a row to open the skill. The **Routing** column says whether agents can be recommended the skill (**Routable**) or not yet (**Not routed**).

<img src="images/web/skills-list.png" alt="The skills list with a state filter and skills that are draft, active or deprecated." width="800"><br><em>Skills: every skill with its state, and filters to find one.</em>

<img src="images/web/mobile-skills.png" alt="The skills list on a narrow phone screen." width="260"><br><em>The same list on a 390 px wide screen.</em>

### Check one skill

Open a skill, and the **Overview** tab opens first. Its cards are **About**, **Routing** (when an agent is offered the skill), **Provenance** (where it came from) and **Sources**. **Edit** opens the **Routing** tab and **Manage sources** opens the **Sources** tab.

The **Review** tab tells you whether agents can use the skill. **Validity** says whether the skill file follows the required format.

**Activation readiness** lists what a draft still needs. **In Git vs what agents see** tells you whether the search index matches the files.

<img src="images/web/skill-review.png" alt="The Review tab of an active skill showing its content check and readiness." width="800"><br><em>Review: whether agents can use this skill, and what to check first.</em>

A skill you added from someone else's repository also shows a **Content trust** card. Agents get no content from such a skill until you read it and approve it.

This is **content approval**. The card links to **Review and approve the content**, the full review screen described under [Review what waits for you](#review-what-waits-for-you). Here `<digest>` stands for a fingerprint of the content you read; the screen sends it for you, and the terminal equivalent is `skillhub skill edit <skill-id> --approve-content <digest>`.

If a draft is not ready, the page says so next to the **Activate skill** button. Each missing item has a **Go to field** link that opens the **Routing** tab on that field, or the **Files** tab on `SKILL.md` for the instructions.

<img src="images/web/skill-review-draft.png" alt="The Review tab of a draft skill listing the missing fields with links to fix each one." width="800"><br><em>A draft that cannot be activated yet, with links to the fields to fix.</em>

### Change the files of a skill

Use this for a quick fix. To try a change with your agent before anyone gets it, click **Checkout to a project** at the bottom of the **Files** tab instead: it shows the command to run in your project, `skillhub skill checkout <skill-id>`, and later `skillhub skill commit <skill-id>` ([Curating skills](curating-skills.md#edit-a-skill-and-try-it-before-others-get-it)).

The **Files** tab shows the whole skill as a tree: `SKILL.md` first, then its folders and files. A mark next to a file says what you did to it: **A** added, **M** modified, **D** deleted, **R** renamed.

- Click a file to read it. **Edit** opens a simple editor for a text file up to 1 MiB; a Markdown file also shows how it will read.
- **+ File**, **+ Folder** and **Upload** add files. A folder exists once it holds a file. An upload may be up to 2 MiB per file; a binary file can be replaced, renamed or deleted but not edited here.
- **Rename** and **Delete** act on the open file (`SKILL.md` keeps its name and stays). **Restore** puts a deleted file back.

None of this reaches the hub yet. The bar at the bottom counts your pending changes: "2 changes not committed · agents still get the current version". The changes are kept in this browser until you commit or **Discard** them.

1. Click **Review →**. Each file shows its mark and the lines it gains and loses; open one to see its diff. The routing lint says whether the new `SKILL.md` still routes well.
2. Edit the **Commit message** if you like.
3. Click **Commit to the hub**. All your changes become one commit, and the page says who gets it: "Committed c492ab3. This machine's agents get it now." When your hub has a Git remote, it adds "others after you push" and the push command.

If the hub changed while you were editing, a drawer opens when you review. Choose **Keep my edits on the latest version** to continue on top of the new files, or **Discard my edits**.

Editing a file of a skill from another repository changes its content, so agents get no content from that skill until you approve it again under **Waiting for review**.

### Review what waits for you

**Waiting for review** lists what needs a person before agents get it: new drafts, skills from another repository whose content changed after you approved it, and skills other machines sent for review (**Submitted · needs approval**, with "Submitted from <machine>" as the origin). The number next to the menu entry is the count.

1. Click a skill to open its full review. At the top you see its license, its origin and what its files are compared with: the content you last approved, what agents get today, or nothing for a new draft.
2. Open each changed file to read it with its diff. A file you opened is marked with a check.
3. **Approve** stays off until every changed file was opened ("open every changed file first: 1 left"). Then it makes one commit and says who gets the skill.

A submission opens the same full review as any skill: every changed file with its diff, the license, "Origin: Submitted from <machine>", and the files the other machine left out. **Approve** applies the files and approves their content in one hub commit, once you have opened every changed file; a new skill lands as an approved draft, which you activate from its page. **Discard** (after a second click) drops it and changes nothing. Files that the hub and the other machine both changed since it was checked out need a decision in a terminal: `skillhub skill review <SUB-id> --accept <path>=hub`.

A draft of your own has nothing to approve: read it, then activate it from its page.

### Activate, deprecate or archive a skill

1. Open the skill and click **Activate skill** (or **Deprecate** for an active one, or **Archive** for a deprecated one). The activate button stays off until the draft is ready.
2. A confirmation lists the files that will change and says in words what changes. Open **Exact file changes** to see the patch.
3. Click **Activate skill** in the confirmation to apply it, or **Cancel** to leave everything as it was.

<img src="images/web/skill-activate-confirm.png" alt="A confirmation dialog asking to activate a skill, listing what will change." width="800"><br><em>Activate: the confirmation that shows what changes before anything is saved.</em>

Skills move in order: draft, active, deprecated, archived. A **draft** is not offered to agents yet, and an **active** skill can be.

A **deprecated** skill is being phased out, and an **archived** one is retired and kept for history. To archive a deprecated skill, click **Archive** on its page and confirm, or run `skillhub skill archive <skill-id> --yes` in the terminal. Without `--yes` the command only shows what would change.

### Fix a skill's description and routing fields

1. Open the skill and choose the **Routing** tab.
2. Change the fields. Each routing field has a short explanation and an example under it. Your draft is kept in this browser while you work.
3. Click **Preview changes**. The dialog lists the files touched and describes the change in words.
4. Click **Commit changes** to write it as one commit, or **Cancel**.

<img src="images/web/editor-preview.png" alt="The routing tab with a dialog that previews the changed description as a diff." width="800"><br><em>Routing: preview the exact change before it is committed.</em>

The skill may change somewhere else while the form is open, for example from the terminal. Then a drawer opens when you preview. It shows what you changed and what changed meanwhile, and warns when both touched the same field.

Choose **Keep my edits on the latest version** to apply them on top of the saved version. Choose **Discard my edits** to drop them. **Copy draft** and **Download draft** keep a copy of your text first.

<img src="images/web/editor-conflict.png" alt="The conflict drawer explaining that the skill changed elsewhere and offering ways to continue." width="800"><br><em>Conflict: the skill changed somewhere else while the form was open.</em>

### Create a skill

1. Open **Skills** and click **Create skill**.
2. Fill in the skill id, display name and description. You cannot change the id later.
3. Fill in the routing fields (operations, triggers, not for, minimum scope). You can leave them empty now, but activation needs them.
4. Click **Preview draft**, check the files that will be written, then confirm. The draft becomes one commit in your hub.

The new skill is a draft: agents do not use it until you activate it. To write a skill in your project and try it in your agent first, use `skillhub skill create` and `skillhub skill commit` ([Curating skills](curating-skills.md#create-your-first-skill)).

<details>
<summary>See the create skill form</summary>

<img src="images/web/create-skill.png" alt="The create skill form with hints under the fields." width="800"><br><em>Create a skill: the form tells you which fields are needed to activate it later.</em>

</details>

### Add skills from GitHub

1. Open **Skills** and click **Add from GitHub**.
2. Paste the address of a public repository, or of a folder or file inside it, and click **Discover skills**. Discover only looks: it lists the skills it finds and writes nothing.
3. Read where the skills come from, the revision, the trust note, the files that would be written and any conflicts. Choose the skills (or **Import all**), then click **Confirm import**.

Imported skills start as drafts and arrive as one hub commit. A skill from someone else's repository also needs your content approval before agents receive it (see [Check one skill](#check-one-skill)).

<img src="images/web/add-from-github-discover.png" alt="The add from GitHub form with a repository address filled in." width="800"><br><em>Add from GitHub: paste a repository address to find skills in it.</em>

### Act on upstream updates

A skill added from GitHub remembers its **upstream**: the source it was copied from, whose updates you can pull.

1. In **Skills**, set **Upstream** to **Has an update**. To refresh that first, click **Check now** in a skill's **Sources** tab, or run `skillhub skill outdated --check`.
2. Open a skill marked **Update available** and click **Review update**.
3. For each changed file choose **Take upstream**, **Keep mine** or **Edit manually**, then click **Apply choices**.

### Manage one skill's sources

A **source** is a Git repository linked to one or more skills. Your **curator agent** is your own agent, started in the hub folder, where Skill Hub installs the distill-lab skill. It reads sources and records **lessons**, proposed improvements you decide on.

Open a skill and choose the **Sources** tab. It lists the skill's upstream and every **learning reference**: a source your curator agent reads for ideas, with nothing copied. A skill with neither shows **No sources yet**.

**Add a learning reference.** Click **Add source**, then choose **Link a watched source** or **Watch a GitHub repository**. Click **Preview** to see the files that would be written, then **Add source** to confirm. **Unlink** removes a reference again.

**Track an upstream.** An older skill copied from GitHub shows **Not tracked**, so the hub cannot check it for updates. Click **Track upstream** to see the exact change, then confirm. The terminal equivalent is `skillhub source backfill --skill <skill-id>`.

### Link and check sources

Open **Sources**. It lists every source, for learning or for updates. Each row shows when it was last checked and what that check found.

**Changed** means it has commits no skill has learned from yet. Checks run only when you ask: **Check now** on one row, or **Check due sources** or **Check all** for many. The terminal equivalent is `skillhub source check`.

<img src="images/web/sources.png" alt="The sources list with one source and the result of its last check." width="800"><br><em>Sources: where skills learn from, and when each was last checked.</em>

### Hand a source to your curator agent

Your curator agent reads the sources you pick and records lessons for each skill.

1. On **Sources**, tick the sources you want learned from and click **Distill with Curator Agent**.
2. Click **Copy handoff** and paste it to an agent you start in your hub folder. The distill-lab skill there runs its script with Node 18 or newer ([Sources and learning](sources-and-learning.md#hand-changes-to-your-curator-agent)).
3. The agent records its lessons in each skill's `.meta/distill.yaml`. Read them in the skill's **Distill** tab, where you decide what to keep.

<img src="images/web/distill-handoff.png" alt="The distill hand-off screen with a brief ready to copy for the curator agent." width="800"><br><em>Distill: copy the brief and give it to your curator agent.</em>

### Change the theme

Use the appearance button at the top right of any screen to switch between light, dark and the system setting.

<img src="images/web/dark-skills-list.png" alt="The skills list in the dark theme." width="800"><br><em>The same list in the dark theme.</em>

## Keep it safe on your network

The web UI speaks plain HTTP and guards each session with the token in its address. Without `--addr`, it listens on every network interface (`0.0.0.0:7421`), so other devices on your network can reach it.

> [!WARNING]
> Traffic is not encrypted, and the web UI can change skills. Anyone who can watch your network can read the token and then add or edit the skills every connected agent uses. Use `--addr 127.0.0.1:7421` on any network you do not trust, and never expose the web UI to a public network without a TLS reverse proxy.

<details>
<summary>How the web UI chooses an address, and how to change it</summary>

Without `--addr`, the web UI listens on all interfaces. It prints a warning and one address per network interface other devices can use; virtual interfaces such as Docker bridges are left out:

```text
WARNING: Listening on all network interfaces over plain HTTP; use --addr 127.0.0.1:7421 to keep it on this machine.
Anyone with this URL can change the skills every connected agent uses.
Skill Hub web UI: http://127.0.0.1:7421/#token=<token>
  <interface>: http://<ip>:7421/#token=<token>
```

To keep it on this machine, run `skillhub serve web --addr 127.0.0.1:7421`. To choose another address or port, pass it to `--addr`, for example `skillhub serve web --addr 127.0.0.1:8080`. `--addr` takes up to three addresses separated by commas, all with the same token.

To reach the web UI through another hostname, such as a reverse proxy, add `--allow-host <host[:port]>`. You can repeat the flag. Requests with any other hostname get `421 Misdirected Request`, which blocks DNS rebinding attacks.

</details>
