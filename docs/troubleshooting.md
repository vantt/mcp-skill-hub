# Troubleshooting

Find the symptom you see, then run the fix below it. In sample output, `<home>` stands for your home folder.

## Start with status

`skillhub status` checks the hub and this folder's agent connections, then prints the one command to run next. Run it in the hub or in the connected project.

```bash
cd <your-project> && skillhub status
```

When something is off, it names the problem and prints a fix line. `skillhub doctor` runs every check and lists each problem with its fix. `skillhub doctor --fix` repairs what it can, then prints what it changed, what it could not fix and why:

```bash
cd <your-project> && skillhub doctor --fix
```

This rewrites outdated connection files and permission rules through the same safe merge as `connect`, and points a project at a moved hub that this machine recorded. It never adds agents, never touches another hub's connections and never reconnects to a hub on another machine. Add `--dry-run` to see the repairs first.

## Installing the hub

### This folder already holds other files

`init` makes a hub only in an empty folder, or in an empty Git repository:

```text
ERROR: This folder already holds other files, so init will not turn it into a hub.
WHY: A hub is a folder of its own, kept in its own Git repository; nothing was written here.
```

Create an empty folder for the hub and run `init` there:

```bash
mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init
```

### A hub must be its own Git repository

`init` refuses a folder inside another Git repository, because the hub's files would end up in that repository's history. The error names that repository and prints an `init` command for a folder outside it. Run that command.

### This folder is inside the hub

You ran `init` in a subfolder of an existing hub. A hub cannot hold another hub. To see the state of that hub, run the command the error prints, `cd <hub> && skillhub status`.

### Git could not read the URL

`init <git-url>` prints Git's own error, then one hint:

```text
ERROR: Git could not read <git-url>.
  fatal: unable to access '<git-url>': Failed to connect to <host> port <port>
Check the URL and that you can run Git against it, for example with your SSH key or a credential
helper.
```

Check the URL, your network and your Git credentials, then run the same command again.

### The repository is not a Skill Hub hub

`init <git-url>` found a repository with content but no hub in it, so it left nothing behind. Use the URL of an empty repository to create a hub there, or the URL of an existing hub.

### The repository holds a history that this hub does not share

In an existing hub, `init <git-url>` connects only to an empty repository or to one made from this hub. It did not add `origin`. Use an empty repository, or the URL this hub was taken from.

### The push failed

`init <git-url>` with an empty repository created the hub here, but Git could not push it. The output shows Git's message. Fix what Git reports, for example your credentials or the repository permissions, then run the same `skillhub init <git-url>` command again to push.

### Git has no user name or email set

`init` still makes the first commit, as `Skill Hub <skillhub@localhost>`, and says so. Set your own name and email for later commits:

```bash
git config --global user.name "<your name>"
git config --global user.email "<your email>"
```

### A cloned hub tracks runtime files

`init <git-url>` refuses a hub whose repository tracks `runtime/` or `.skillhub/transactions/`. Those folders hold credentials and programs of the machine that wrote them, so `init` checks out nothing. Ask the owner of the repository to stop tracking them, then run the same `init` command again. The error prints the Git command for the owner.

## Installing the client project

### No hub is set up on this machine

`connect` has no hub to use, because `init` never ran on this machine:

```text
ERROR: The request cannot be accepted.
WHY: No hub is set up on this machine.
FIX: Create one with `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`, then run
     `skillhub connect` here again. For a hub on another machine, run
     `skillhub connect --hub <url> --token-file <file>`.
```

Run the command the FIX line names. Skill and source commands give a similar message in a folder that is neither a hub nor a connected project; run them inside one, for example `cd <your-project> && skillhub skill list`.

### The hub folder is gone

The project points at a hub folder that no longer exists, usually because you moved the hub:

```text
Hub folder   FAILED  The hub folder <home>/skill-hub is gone.

FIX:  Hub folder: If you moved the hub, run `cd <new-location> && skillhub init` there, then
      `skillhub doctor --fix` in each project that uses it.
```

Run `init` in the hub's new folder. It makes that folder the hub of this machine again:

```bash
cd <new-location> && skillhub init
```

Then repair each project that uses the hub:

```bash
cd <your-project> && skillhub doctor --fix
```

### Connection files or permission rules are out of date

`status` reports "Out of date" for the connection files, or "The Claude Code permission rules are not the current set". This happens after an upgrade, after you move the `skillhub` program, or when a file was edited by hand. Repair them:

```bash
cd <your-project> && skillhub doctor --fix
```

Run it once in your hub and in each project after an upgrade. Your agents' registrations stay as they are; only Skill Hub's own entries and rules change.

## Using a hub on another machine

### Nothing answers at the hub's address

`status` or `doctor` on the client machine reports "Nothing answers at `<url>`". The server on the hub machine is stopped, or the SSH tunnel or the Tailscale connection is down. `doctor --fix` cannot fix this from the client machine; it prints the command to run on the hub machine instead. Start the server there:

```bash
cd ~/skill-hub && skillhub serve
```

If you use an SSH tunnel, open it again on the client machine. [Check the connection](remote-hub-guide.md#check-the-connection) lists every message.

### The hub refused the token

The token was replaced on the hub machine, or the file comes from another hub. Run the `scp` line that `skillhub serve` prints on the hub machine again, then restart your agent. `doctor --fix` never copies a token for you.

## Setup and connection

### The agent does not see the hub

Open a new Claude Code session in the project folder after `connect`. If the hub is still missing, run `skillhub status` there and run the command it prints. If you deleted `.mcp.json`, the project no longer counts as connected, so connect again:

```bash
cd <your-project> && skillhub connect
```

[What gets written](user-guide.md#what-gets-written) lists the files `connect` manages.

### Trust prompts after connect

The first time you open the folder, Claude Code shows a trust dialog listing `Bash(skillhub:*)`. Review it before you trust the folder. Codex and Gemini CLI may show their own prompts; they are not yet verified end to end.

### `connect -g` refuses because of a symbolic link

Skill Hub never writes through a symbolic link. If `~/.claude` is a link, `connect -g` stops:

```text
WHY: Host integration path <home>/.claude contains a
     symbolic link. Skill Hub will not write through symlinks.
```

Connect each project instead, or limit `-g` to agents with real folders (`--host codex,gemini`). Codex and Gemini CLI are not yet verified end to end. You can also replace the link with a regular folder.

## Skills

### Activation refuses a draft

A draft is a skill agents are not offered yet. Activation needs real instructions, not the untouched starter text, plus three routing fields:

```text
WHY: skill bare-skill requires: content (replace untouched scaffold instructions), trigger, not_for
     or rationale (quality.routing_review_rationale), min_scope
```

Add your instructions and the missing fields in one edit:

```bash
skillhub skill edit <skill-id> --content-file <file> --trigger "<when to use>" \
  --not-for "<when not to use>" --min-scope single_step --yes
```

Then activate it with `skillhub skill activate <skill-id> --yes`. At least 3 examples are recommended; add them with `--example`. [Create your first skill](curating-skills.md#create-your-first-skill) shows a full run.

### Resolve asks a question instead of naming a skill

`skillhub resolve` asks "What is the bounded scope of this task?" when the request has no scope. Put `scope` inside `task`; the `"task.scope"` key in the hint is rejected.

```json
{"schema_version": "1", "request_id": "r1",
 "task": {"description": "review my pull request", "scope": "single_step"}}
```

### The agent reports `review_required`

Agents get nothing from a skill copied from someone else's repository until you approve its content. Only you can do that, in the terminal. Start the review:

```bash
skillhub skill review <skill-id>
```

After you read its files, run the exact `skillhub skill edit <skill-id> --approve-content <digest>` command from the **Content trust** block. The digest is a fingerprint of the content you reviewed. See [Safety model](curating-skills.md#safety-model).

### The agent reports `setup_failed`

A skill's script failed because a tool or variable is missing here. `skillhub skill doctor <skill-id>` shows which binary, environment variable or setup check failed.

### Adding a large GitHub repository fails

Adding a skill with `--skill` from a big repository stops with `source exceeded bytes limit`. Point at the skill's folder instead:

```bash
skillhub skill add https://github.com/anthropics/skills --path skills/pdf
```

For bulk imports, see [Import many skills from a repository](sources-and-learning.md#import-many-skills-from-a-repository).

## Workspace and index

### The search index is stale

`skillhub status` says the search index is stale after you edit hub files by hand. Read commands such as `skill list` rebuild it when the files are valid. To rebuild yourself, validate first:

```bash
cd ~/skill-hub && skillhub maintain validate
```

`maintain validate` names each broken file; fix them, then run `skillhub doctor --fix` or `skillhub maintain rebuild`.

The index holds only files that pass validation. If a skill's extra files change or vanish outside Skill Hub, agents stop receiving them instead of getting old copies. Restore the files or add the skill again; see the [error codes](contracts/error-codes.md#error-code-registry) and [result envelope](contracts/result-envelope.md) contracts.

### Fresh clone on a new machine

A hub you cloned with Git has no search index yet. Run `init` in it once; it builds the index and makes it the hub of this machine:

```bash
cd ~/skill-hub && skillhub init
```

Next time, fetch the hub with `skillhub init <git-url>` in an empty folder instead of `git clone`.

### A pre-commit hook fails with an unknown command validate

A hook written for an earlier release runs the old top-level validate command. That command is now `skillhub maintain validate --staged`, so the hook fails every commit with `The command "validate" is not supported.` `skillhub doctor` in the hub reports the hook, and `doctor --fix` changes that one command and keeps the rest of the hook:

```bash
cd ~/skill-hub && skillhub doctor --fix
```

### Telemetry is degraded

`skillhub telemetry health` reports `file is not a database`. Telemetry is a local, disposable usage record. Discard and recreate it with `skillhub telemetry purge --yes`.

## Windows

<details>
<summary>SmartScreen and firewall prompts</summary>

**SmartScreen or antivirus warning.** Windows may flag a newly downloaded `skillhub.exe`. First [check the signatures](installation.md#check-signatures). Then choose **More info** and **Run anyway**.

**Firewall prompt on `serve` or `serve web`.** Both listen on every network interface by default, so the firewall may ask. Allow private networks only. To keep the web UI on this machine, run `skillhub serve web --addr 127.0.0.1:7421`. See [Keep it safe on your network](web-ui.md#keep-it-safe-on-your-network).

**`&&` is not a valid statement separator.** Windows PowerShell 5.1 has no `&&`. Write `cd <folder>; skillhub <command>`, or run the two commands on separate lines.

</details>
