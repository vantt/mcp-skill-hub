# Command reference

Find the command for a task here. For every flag, run `skillhub help` or `skillhub help <command>`; the built-in help wins over this page.

- Every command acts on the folder you run it in. Run hub commands inside the hub or a connected project: `cd <folder> && skillhub <command>`. For scripts, command help names `--workspace <path>` (a hub folder on this machine) and `SKILLHUB_WORKSPACE`; `--hub` always takes a URL.
- Most commands accept `--json` for machine-readable output.
- Setup commands (`init`, `connect`, `doctor --fix`, `token rotate`, `cache clear`) act at once and print what they changed; add `--dry-run` to see the changes first.
- Commands that change skills or sources show a preview first. Add `--yes` to write. `source capture` and `skill env set|unset` write at once.
- Only Claude Code is verified end to end; `connect` also writes files for Codex and Gemini CLI.

## Set up

| Task | Command |
|---|---|
| Make the current, empty folder your hub | `skillhub init [--dry-run]` |
| Fetch a hub from Git, or push a new hub to an empty repository | `skillhub init <git-url>` |
| Connect this project's agents to your hub | `skillhub connect [<path>] [--host claude\|codex\|gemini] [--dry-run]` |
| Connect your agents in every project | `skillhub connect -g [--host claude\|codex\|gemini] [--dry-run]` |
| Connect your agents to a hub on another machine | `skillhub connect --hub <url> --token-file <file> [-g] [--host claude\|codex\|gemini] [--dry-run]` |
| Remove a connection | `skillhub connect --remove [--cache] [-g] [--host claude\|codex\|gemini] [--dry-run]` |
| See the state of the hub and this folder, and the next command | `skillhub status [--json\|--quiet]` |
| Check the hub and the connections | `skillhub doctor [--json]` |
| Repair what doctor can | `skillhub doctor --fix [--dry-run]` |
| Open the web UI | `skillhub serve web [--addr <host:port>] [--no-open]` |
| Print the version | `skillhub --version` |
| Check for a new release | `skillhub update --check` |
| Update the `skillhub` program | `skillhub update [--version <v>]` |

`doctor` exits with `0` when everything is healthy, `1` when problems remain and `2` for an invalid request. `status` exits with `2` in a folder that is neither a hub nor a connected project, and prints the command that sets one up.

## Work with skills

| Task | Command |
|---|---|
| List skills | `skillhub skill list [--state draft\|active\|deprecated\|archived]` |
| Read a skill in any state | `skillhub skill show <skill-id> [--verbose]` |
| Add a skill from GitHub or a local folder, as a draft | `skillhub skill add <locator> [--path <p>] [--skill <name>\|--all] [--yes]` |
| Create a draft skill | `skillhub skill create` in a terminal asks the questions; scripts use `skillhub skill create <skill-id> --collection <c> --name <n> --description <d> [--content-file <f>] [routing flags] [--yes]` |
| Review a skill before you activate it | `skillhub skill review <skill-id> [--verbose]` |
| Edit instructions in your editor | `skillhub skill edit <skill-id> --editor` |
| Change the description or routing fields | `skillhub skill edit <skill-id> [--description <d>] [--trigger <t>] [--not-for <n>] [--min-scope <s>] [--yes]` |
| Add requests that should, or should not, pick the skill | `skillhub skill edit <skill-id> --example <e> --counter-example <e> [--yes]` |
| Declare the tools a skill needs | `skillhub skill edit <skill-id> --runtime-file <yaml> [--yes]` |
| Approve content from someone else's repository (terminal only) | `skillhub skill edit <skill-id> --approve-content <digest>` |
| Check this machine against a skill's needs | `skillhub skill doctor <skill-id>` |
| Store, remove or list a skill's secrets | `skillhub skill env set\|unset <skill-id> <KEY>`, `skillhub skill env list <skill-id>` |
| Apply a previewed change | `skillhub skill confirm <proposal-id>` |
| Activate, deprecate or archive a skill | `skillhub skill activate\|deprecate\|archive <skill-id> [--yes]` |

Routing flags for `create` and `edit`: `--operation`, `--trigger`, `--not-for`, `--min-scope`, `--example`, `--counter-example`. Activation needs real instructions, not the untouched starter text. It also needs a trigger, a `--not-for` entry (or `--rationale`) and `--min-scope`.

## Sources and learning

| Task | Command |
|---|---|
| Check sources for updates | `skillhub source check [--all-due\|--all\|<source-id>...]` |
| See which copied skills differ from upstream | `skillhub skill outdated [--check] [--all] [--exit-code]` |
| Show one skill's upstream and drift | `skillhub skill upstream <skill-id> [--check]` |
| Pull upstream changes into a skill | `skillhub skill update <skill-id> [--target <commit>] [--yes]` |
| List candidates and sources | `skillhub source list [--status <s>]` |
| Show one candidate or source | `skillhub source show <id>` |
| Watch a repository as a learning reference | `skillhub source watch <locator> --skill-id <skill-id> [--cadence daily\|weekly\|manual] [--yes]` |
| Stop watching a source | `skillhub source unwatch <source-id> [--yes]` |
| Link or unlink a learning reference | `skillhub source attach\|detach <source-id> --skill-id <skill-id> [--yes]` |
| Fill in source records for older skills | `skillhub source backfill [--skill <skill-id>] [--yes]` |
| Import skills from a repository as drafts | `skillhub source import <locator\|source-id> [--path <p>] [--skill <name>\|--all] [--yes]` |
| Note a repository to look at later | `skillhub source capture <locator> --reason <text>` |
| Decide on a candidate | `skillhub source triage <candidate-id> --decision accept\|defer\|reject\|import` |

`source check` records whether a source changed; it never edits your skills, and nothing runs in the background. Lessons have no command: start them from **Sources** in the web UI with **Distill with Curator Agent**. Then read them in the skill's **Distill** tab ([Sources and learning](sources-and-learning.md#learn-from-other-repositories)).

## Maintain the hub

`skillhub doctor` runs these checks and `skillhub doctor --fix` the repairs most people need; the `maintain` commands are the single steps.

| Task | Command |
|---|---|
| See uncommitted hub changes | `skillhub maintain diff` |
| Check the hub files | `skillhub maintain validate` |
| Check only the files staged in Git, for a pre-commit hook | `skillhub maintain validate --staged` |
| Rebuild the search index | `skillhub maintain rebuild [--verbose]` |
| Preview or apply a file format migration | `skillhub maintain migrate [--to <version>] [--yes]` |

Skill Hub never commits or pushes your skill changes for you. Add the hub's own paths by name; never stage the whole folder. See [Curating skills](curating-skills.md#review-and-commit-workspace-changes).

## Serve other machines

| Task | Command |
|---|---|
| Let agents on other machines use this hub | `skillhub serve [--addr <host:port>[,<host:port>...]] [--allow-host <name>]` |
| Keep the server on this machine, for an SSH tunnel | `skillhub serve --addr 127.0.0.1:7420` |
| Show the token those agents need (terminal only) | `skillhub token show` |
| Replace that token (terminal only) | `skillhub token rotate [--dry-run]` |
| Remove the skills this machine fetched from another hub | `skillhub cache clear [--hub <url>] [--dry-run]` |

`serve` and `serve web` listen on every network interface by default, over plain HTTP. [Use one hub from several machines](remote-hub-guide.md) explains when to narrow that with `--addr`.

## Routing and usage

These are developer commands; `skillhub help --dev` lists them.

| Task | Command |
|---|---|
| Ask which skill fits a request | `skillhub resolve --request <request.json>` |
| Measure routing quality across skill examples | `skillhub eval routing [--no-skill <file>] [--policy <file>]` |
| Check the local usage record | `skillhub telemetry health` |
| See how often skills are recommended and used | `skillhub telemetry funnel [--since <YYYY-MM-DD\|Nd>] [--skill <skill-id>]` |
| Import tool use from Claude Code transcripts | `skillhub telemetry import-transcripts --project <dir>` |
| Preview or write a sanitized export | `skillhub telemetry export [--output <file> --yes]` |

Your agent starts Skill Hub by itself over stdio; you never start that by hand. The usage record stays on your machine unless you export it. Developer tools such as `eval run` and `resolution replay` are described in `skillhub help eval` and `skillhub help resolution`.
