# Troubleshooting

Find the symptom you see, then run the fix below it. In sample output, `<home>` stands for your home folder.

## Start with doctor

`skillhub doctor` checks your workspace, its search index and this folder's agent connections. Run it inside the connected project.

```bash
skillhub doctor
```

When something is off, doctor names it and prints a fix line. An example is "Current project connection ... is outdated or needs repair". Preview the repairs with `skillhub doctor --fix`, then apply them:

```bash
skillhub doctor --fix --yes
```

This rewrites outdated connection files, such as a moved binary path or missing permission entries. It repairs only agents already connected to this workspace. It never adds agents and never touches another workspace's connections.

## Setup and connection

### No Skill Hub workspace was found

A command run outside a connected project stops with this error:

```text
ERROR: The request cannot be accepted.
WHY: No Skill Hub workspace was found from the current directory.
FIX: Pass `--workspace <path>`, set the SKILLHUB_WORKSPACE environment variable, or run
     `skillhub init <path> --yes`.
```

Skill commands such as `skillhub skill create` need a workspace too. Run them in a connected project, or add `--workspace ~/skillhub`. To skip the flag everywhere, set `SKILLHUB_WORKSPACE=~/skillhub` in your shell profile.

### The agent does not see the hub

Open a new Claude Code session in the project folder after `connect`. If the hub is still missing, run `skillhub doctor` there and apply its fix. If you deleted `.mcp.json`, doctor no longer recognizes the project, so connect again:

```bash
skillhub connect --workspace ~/skillhub --host claude --yes
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

### Missing permission entries

Claude Code cannot read a skill's files, or doctor calls the project connection outdated. `skillhub doctor --fix --yes` restores the permission rules and folders in `.claude/settings.local.json`.

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

Then activate it with `skillhub skill activate <skill-id> --yes`. At least 3 examples are recommended; add them with `--example`. [Create your first skill](getting-started.md#create-your-first-skill) shows a full run.

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

`skillhub status` says "Search index is stale" after you edit workspace files by hand. Read commands such as `skill list` rebuild it when the files are valid. To rebuild yourself, validate first:

```bash
skillhub validate
```

`validate` names each broken file; fix them, then run `skillhub rebuild`.

The index holds only files that pass validation. If a skill's extra files change or vanish outside Skill Hub, agents stop receiving them instead of getting old copies. Restore the files or add the skill again; see the [error codes](contracts/error-codes.md#error-code-registry) and [result envelope](contracts/result-envelope.md) contracts.

### Fresh clone on a new machine

A cloned workspace has no search index yet; `status` shows "search index missing". Build it once with `skillhub rebuild --workspace ~/skillhub`.

### Telemetry is degraded

`skillhub telemetry health` reports `file is not a database`. Telemetry is a local, disposable usage record. Discard and recreate it with `skillhub telemetry purge --yes`.

## Windows

<details>
<summary>SmartScreen and firewall prompts</summary>

**SmartScreen or antivirus warning.** Windows may flag a newly downloaded `skillhub.exe`. First [check the signatures](installation.md#check-signatures). Then choose **More info** and **Run anyway**.

**Firewall prompt on `serve web`.** With several network adapters, `skillhub serve web` listens on all of them, and the firewall may ask. Allow private networks only, or pass `--loopback-only`. See [Keep it safe on your network](web-ui.md#keep-it-safe-on-your-network).

</details>
