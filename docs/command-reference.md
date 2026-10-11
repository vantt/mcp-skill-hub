# Command reference

Find the command for a task here. For every flag, run `skillhub help` or `skillhub help <command>`; the built-in help wins over this page.

- Skill and source commands need a workspace: run them in a connected project, pass `--workspace <path>`, or set `SKILLHUB_WORKSPACE`.
- Most commands accept `--json` for machine-readable output.
- Commands that change files show a preview first. Add `--yes` to write. `source capture` is the one exception: it writes at once.

## Set up

| Task | Command |
|---|---|
| Create your workspace | `skillhub init [path] [--force] --yes` |
| Connect your agent to one project (also `integrate`) | `skillhub connect [--project <dir>] [--workspace <path>] [--host claude\|codex\|gemini] --yes` |
| Connect your agent to every project | `skillhub connect -g --workspace <path> --yes` |
| See health and the next step | `skillhub status [--quiet]` |
| Check the workspace and connections, and repair them | `skillhub doctor [--fix [--yes]]` |
| Open the web UI | `skillhub serve web [--loopback-only] [--addr <host:port>] [--no-open]` |
| Print build information | `skillhub version` |
| Check for a new release | `skillhub update --check` |
| Update the `skillhub` program | `skillhub update [--version <v>]` |

## Work with skills

| Task | Command |
|---|---|
| List skills | `skillhub skill list [--state draft\|active\|deprecated\|archived]` |
| Read a skill in any state | `skillhub skill show <skill-id> [--verbose]` |
| Add a skill from GitHub or a local folder, as a draft | `skillhub skill add <locator> [--path <p>] [--skill <name>\|--all] [--yes]` |
| Create a draft skill | `skillhub skill create <skill-id> --collection <c> --name <n> --description <d> [--content-file <f>] [routing flags] [--yes]` |
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
| Check sources for updates | `skillhub check [--all-due\|--all\|<source-id>...]` (also `skillhub source check`) |
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

`check` records whether a source changed; it never edits your skills, and nothing runs in the background. Lessons have no command: start them from **Sources** in the web UI with **Distill with Curator Agent**, then read them in the skill's **Distill** tab. See [Sources and learning](sources-and-learning.md#learn-from-other-repositories).

## Workspace and Git

| Task | Command |
|---|---|
| See uncommitted workspace changes | `skillhub diff` |
| Check the workspace files | `skillhub validate` |
| Check only the files staged in Git | `skillhub validate --staged` |
| Rebuild the search index | `skillhub rebuild [--verbose]` |
| Preview or apply a file format migration | `skillhub migrate [--to <version>] [--yes]` |

Skill Hub never commits or pushes for you. Add the workspace's own folders by name, never with `git add -A`; see [Curating skills](curating-skills.md#review-and-commit-workspace-changes).

## Routing and usage

| Task | Command |
|---|---|
| Ask which skill fits a request | `skillhub resolve --request <request.json>` |
| Measure routing quality across skill examples | `skillhub eval routing [--no-skill <file>] [--policy <file>]` |
| Check the local usage record | `skillhub telemetry health` |
| See how often skills are recommended and used | `skillhub telemetry funnel [--since <date\|Nd>] [--skill <skill-id>]` |
| Import tool use from Claude Code transcripts | `skillhub telemetry import-transcripts --project <dir>` |
| Preview or write a sanitized export | `skillhub telemetry export [--output <file> --yes]` |
| Run the MCP server by hand (your agent starts it) | `skillhub mcp serve [--profile runtime\|curation\|all]` |

The usage record stays on your machine unless you export it. Developer tools such as `eval run` and `resolution replay` are described in `skillhub help eval` and `skillhub help resolution`.
