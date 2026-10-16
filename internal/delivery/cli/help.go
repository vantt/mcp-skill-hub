package cli

import (
	"fmt"
	"io"
	"strings"
)

const globalUsage = `Skill Hub keeps one curated set of agent skills and serves it to your AI agents over MCP.

Usage: skillhub <command> [flags]

Get started:
  1. mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init    Create your hub
  2. skillhub connect -g                                         Connect your agents in every project
     (or cd <your project> && skillhub connect for one project only)
  3. Open your agent there and ask: "curate my Skill Hub"
  On another machine, use that hub through an SSH tunnel or Tailscale:
     skillhub connect --hub <url> --token-file <file>

Setup:
  init      Create a hub in the current folder, or from a Git URL (--dry-run shows what it would do)
  connect   Connect your agents to the hub: -g for every project (recommended), without it this project only;
            --hub to a hub on another machine
  doctor    Check the hub and the agent connections; --fix repairs what it can
  status    Show the state of the hub and its connections, and the next command

Skills:
  skill     List, show, create, checkout, commit, review, add, update from upstream, and change the state of skills

Sources & learning:
  source    Repositories and documents your skills come from or learn from; source check finds updates

Serve:
  serve     Let agents on other machines use this hub (listens on 0.0.0.0:7420)
  serve web Run the web UI (listens on 0.0.0.0:7421)
  token     Show, rotate or invite a machine with the hub token that serve requires

Maintenance:
  maintain  validate files, rebuild the search index, migrate the schema, show uncommitted changes
  cache     Clear the copies of skills fetched from a remote hub
  update    Update skillhub to the latest release or specified version

  --version Print the skillhub version

Hub selection: run inside the hub or a connected project, else pass --workspace <path>
(a local hub folder) or set SKILLHUB_WORKSPACE; --hub always takes a URL. Run ` + "`skillhub <command> --help`" + ` for a command's flags and
` + "`skillhub help --dev`" + ` for developer commands.
`

// devUsage lists the developer commands that the global help leaves out.
const devUsage = `Developer commands:
  resolve     Pick the best skill for a request
  eval        Run routing evaluations
  resolution  Replay a recorded resolution
  telemetry   Inspect or export local telemetry

Run ` + "`skillhub <command> --help`" + ` for a command's flags.
`

var commandUsage = map[string]string{
	"init": `Usage: skillhub init [<git-url>] [--dry-run] [--verbose] [--json]

Make the current folder a hub: an empty folder or an empty Git repository
becomes a Git repository with the hub layout, the search index, the agent
connection of the hub folder and a first commit (Git hooks and commit signing
are skipped for it). The hub path is stored for this machine, so
` + "`skillhub connect -g`" + ` (or ` + "`skillhub connect`" + ` in one project) finds it. Run again in a hub, init refreshes it.
A folder with other files is refused; init takes no folder argument.

  <git-url>   Start from a repository (https, ssh or file URL, user@host:path,
              or a local repository): one that holds a hub is fetched into this
              folder; an empty one gets this new hub pushed and becomes origin.
              In an existing hub without origin, init connects it to the URL.
  --dry-run   Show what init would do; write nothing (not with <git-url>)
  --verbose   Add generation IDs, digests and row counts
  --json      Machine-readable result
`,
	"connect": `Usage: skillhub connect [<path>] [-g|--global] [--host claude|codex|gemini]... [--dry-run] [--json]
       skillhub connect --hub <url> --token-file <file> [-g|--global]
                        [--host claude|codex|gemini]... [--dry-run] [--json]
       skillhub connect --hub <url> --invite <code> [--token-file <file>] [--strict-pairing]
                        [-g|--global] [--host claude|codex|gemini]... [--json]
       skillhub connect --remove [--cache] [-g|--global]
                        [--host claude|codex|gemini]... [--dry-run] [--json]

Connect the agents of the current folder to the hub. Writes the MCP
registration and the system-curator skill, removes an old Skill Hub
instruction block, and prints what it changed. It never overwrites your own
content in those files.

  <path>            A hub folder on this machine (default: the hub ` + "`skillhub init`" + `
                    set up last on this machine)
  -g, --global      Register for your user account so every project sees the hub
                    (recommended; without -g only the current folder is connected)
  --host <name>     Limit to claude, codex, or gemini (repeat or comma-separate)
  --dry-run         Show what would change; write nothing
  --json            Machine-readable result

A hub on another machine (through an SSH tunnel or Tailscale):
  --hub <url>       The base URL the hub printed when ` + "`skillhub serve`" + ` started,
                    for example http://192.168.1.20:7420. The agent host then
                    starts ` + "`skillhub mcp proxy`" + `. Curation stays on the hub machine, so no
                    curation server and no system-curator skill are installed.
  --token-file <f>  This machine's copy of the hub's token file, one file per hub, for
                    example ~/.skillhub-token-<host>-<port>. Copy it from the hub
                    machine with scp and chmod 600 it; only its path is written,
                    never the token. Without it, the agent host must start the proxy
                    with SKILLHUB_HUB_TOKEN set.
  --invite <code>   Take the token from a one-time invite instead of copying the file:
                    run ` + "`skillhub token invite`" + ` on the hub machine and pass the code
                    it prints, with the URL it prints. The token is saved in this
                    machine's Skill Hub config folder (or at --token-file; the file
                    must not exist yet) and then connected as usual. The invite works
                    once and expires, so it cannot be combined with --dry-run or
                    --remove. On a plain network the code and the token travel
                    unencrypted: --strict-pairing refuses to send the invite anywhere
                    but to this machine or over Tailscale

Undo a connection:
  --remove          Remove the Skill Hub entry of the scope (local or remote hub), its
                    old instruction block, if any, and an unchanged system-curator
                    skill; unrelated host configuration stays
  --cache           With --remove, also clear the skills fetched from the remote hub

<path> is a local folder and --hub a URL; they exclude each other: one scope
points at one hub, and connecting it to another hub replaces its entry.
`,
	"doctor": `Usage: skillhub doctor [--fix [--dry-run]] [--workspace <path>] [--json]

Check the hub this folder is in or uses, and the agent connections of this
project and of your user account. doctor only reports: one table per hub or
connection, a FIX line for each problem, then "Healthy." or the number of
problems. Exit codes: 0 healthy, 1 problems remain, 2 invalid request.

  --fix        Repair what doctor can, then print what changed (with the files)
               and what it could not fix, why, and the command to run instead.
               It re-runs connect's safe merge for the connections to a hub on
               this machine and points them at a moved hub this machine recorded.
               It never reconnects to a hub on another machine: that sends the
               token, so it prints the connect command for you to run.
  --dry-run    With --fix, show what it would repair; write nothing
  --workspace  A local hub folder to check instead of the current one; only the
               connections to that hub are checked and repaired
  --json       Machine-readable result: role, sections, fixed, not_fixed

For a connection to a remote hub (connect --hub) doctor checks the token file,
that the hub is reachable, the token is accepted, the hub version, the skill
cache and the last fetch. The token is sent only to a hub your user account
names or a hub on this machine, and it is never printed.
`,
	"status": `Usage: skillhub status [--workspace <path>] [--json|--quiet]

Show the state of the hub this folder is in or uses, and of the agent
connections, from the same checks doctor runs (a quick subset: status never
waits for a busy hub and probes a remote hub once). It always ends with one
command to run next.

  --workspace  A local hub folder instead of the current one
  --json       The curation home of the hub (schemas/curation-home-v1.schema.json)
               with the role of this folder and its connection sections
  --quiet      Print only the overall status word
`,
	"skill": `Usage: skillhub skill <subcommand> [flags]

Subcommands:
  add <github-url> [--skill <name> | --all] [--id <id>] [--collection <c>] [--ref <r>] [--path <p>] [--edit-routing]
      [--diff] [--yes] [--no-commit]
                                                    Add draft skills from a GitHub repository, tracked for upstream updates
                                                    (a folder on this machine: skill commit <folder>)
  create [<id>] [--description <d>] [--name <n>] [--trigger <t>]... [--not-for <n>]... [--example <e>]...
                                                    Start a new skill in this project: write .claude/skills/<id>/SKILL.md (nothing
                                                    changes on the hub). In a terminal it asks what the skill does, when to use it,
                                                    when not to and for an example; without one the flags fill the same file
  checkout <id>                                     Copy a skill with every file into .claude/skills/<id>/ of this project to edit and
                                                    try it; the hub keeps serving the current version and the folder goes into .gitignore.
                                                    In a project connected to a hub on another machine it fetches the active skill from that hub
  commit <id|folder> [--collection <c>] [--runtime-file <yaml>] [--accept <p>=hub|project]... [--manual <p>=<f>]... [--write-conflicts <dir>]
         [--trigger <t>]... [--not-for <n>]... [--example <e>]... [--min-scope <s>] [--edit-routing]
         [--approve-content <digest>] [--diff] [--yes] [--no-commit] [--json]
                                                    Bring a project folder back to the hub as one reviewed change and one hub commit.
                                                    A folder that was never checked out becomes a new skill. In a project connected to a hub
                                                    on another machine it sends the folder there for review (collection and routing are set
                                                    by the reviewer; --yes works only for a hub your own settings name)
  show <id> [--verbose]                             Show a skill in any state
  review <id> [--verbose]                           Comprehensive diagnostic review of a skill
  review <SUB-id> [--diff] [--discard] [--accept <p>=hub|project]... [--manual <p>=<f>]... [--write-conflicts <dir>]
         [--trigger <t>]... [--not-for <n>]... [--example <e>]... [--min-scope <s>] [--edit-routing] [--json]
                                                    Review a skill another machine sent to this hub: preview it like a commit and, in a
                                                    terminal, apply it, approve its content and (for a new skill) activate it, as one hub
                                                    commit. --discard drops it. Nothing is applied without a terminal
  doctor <id> [--json]                              Check this machine against a skill's runtime block
  env set|unset <id> <KEY> | env list <id>          Store secrets for a skill's scripts (values never shown)
  edit <id> [--name <n>] [--description <d>] [--rationale <r>] [--content-file <f>|--editor]
       [--operation <o>]... [--trigger <t>]... [--not-for <n>]... [--min-scope <s>]
       [--example <e>]... [--counter-example <e>]... [--runtime-file <yaml>] [--approve-content <digest>] [--yes]
  edit <id> --from-suggestion <suggestion-id> [--diff] [--yes] [--json]
                                                    Preview a routing entry that "skillhub telemetry cases suggest" found, and apply it
                                                    like any edit. It cannot be combined with other edit flags
  outdated [--check] [--all] [--exit-code] [--json]
                                                    Show repository skills that differ from upstream
  upstream <id> [--check] [--json]                  Show upstream repository details and drift for a skill
  update <id> [--no-check] [--target <commit>] [--accept <p>=upstream|local|merged]...
         [--manual <p>=<f>]... [--write-conflicts <dir>] [--yes] [--verbose] [--json]
                                                    Apply upstream changes to a skill with 3-way merge
  activate <id> | deprecate <id> | archive <id> [--yes]
  confirm <proposal-id> | --proposal <id> --proposal-digest <d> --base-version <v>
                                                    Apply a skill add or mutation proposal
  list [--state draft|active|deprecated|archived]   List skills

Editing a skill: "skill checkout <id>" copies it into your project, you and your agent edit and try the files there, then
"skill commit <id>" shows every changed file (and a three-way merge when the hub changed meanwhile), runs the routing lint
and asks. Committing the checkout of a third-party skill also approves the new content in the same step, so the skill is
never blocked; without a terminal that needs --approve-content <digest>, which only a person gives. A new folder is added
as a draft, and in a terminal you can activate it in the same commit. Routing for a new skill is suggested from its
description (--edit-routing opens it in $EDITOR). The hub keeps serving the released version until the commit is approved.

From another machine: in a project connected to a hub with "skillhub connect --hub", "skill checkout <id>" fetches an active skill
and "skill commit <id>" sends the folder to that hub's review queue ("Sent as SUB-..."). Nothing on the hub changes and nothing is
served until a person on the hub machine runs "skill review SUB-..." (or uses "Waiting for review" on the web) and approves it.

Applying a change: in a terminal, a command that changes the hub shows its preview and asks
"Apply this change? [y/N]". Without a terminal (scripts, agents) or with --json it only previews; run the same
command with --yes to apply. --diff adds the full diff to a preview. Every applied change becomes one Git commit
of only its own files in the hub (never pushed); --no-commit writes the files without committing. A change to
files that already differ from Git is refused until you commit or discard those files.

Note: skillhub skill update <id> updates a skill from its upstream repository; skillhub update updates the skillhub binary itself.
outdated exit codes: 0 normally; with --exit-code, 1 when any skill needs attention (update available, diverged, removed upstream), 2 on invalid flags.

--content-file takes any readable markdown file (relative to the current directory
or absolute). Frontmatter is optional; if present, its "name" field must match the skill id.
Allowed --operation values: explore, design, implement, review, debug, test, refactor, migrate, document, operate, research, other.
Allowed --min-scope values: single_step, multi_step, project.
On edit, routing flags you pass replace that field; the others keep their values.
--example and --counter-example record requests that should (or should not) route to the
skill: at most 10 each, up to 300 characters.
--from-suggestion adds one suggested routing entry, as suggested, to the skill it was found for.
--runtime-file replaces the skill's runtime block with the YAML mapping in the file (on commit: for a new skill only) (requires.bins/env/platforms,
setup.check/command); a file containing {} removes the block. Changing it makes an earlier --approve-content stale.
--approve-content records your review of a skill's whole content (files and runtime block); pass the
exact content digest. Only this command can approve content; agents cannot.
An active skill needs a trigger, a --not-for entry (or a --rationale), and --min-scope.
doctor probes declared binaries and versions, checks that required environment
variables are set (values are never shown), and runs the skill's setup check in its
state directory only when the skill's content is trusted. It never runs setup.
A variable also counts as present when stored with "skill env set" (read without echo, or from stdin
when piped); the value goes to runtime/config/<id>/env (mode 0600), never to Git, and "env list" shows names only.
Results describe this terminal (basis: terminal) and are cached as a hint for agents. Exit codes: 0 ready, 1 setup required or unsupported platform,
2 invalid request or unknown skill.

Examples:
  skillhub skill add https://github.com/anthropics/skills --path skills/pdf --yes
  skillhub skill create
  skillhub skill create my-skill --description "Review changes" --trigger "review code" --not-for "write prose"
  skillhub skill checkout my-skill
  skillhub skill commit my-skill
  skillhub skill commit .claude/skills/team-release --collection tools
  skillhub skill review my-skill
  skillhub skill edit my-skill --editor
  skillhub skill edit my-skill --example "review my pull request" --counter-example "write release notes" --yes
  skillhub skill outdated --check
  skillhub skill upstream pdf
  skillhub skill update pdf --yes
  skillhub skill confirm PROP-123
  skillhub skill activate my-skill
`,
	"source": `Usage: skillhub source <subcommand> [--workspace <path>] [flags]

Subcommands:
  watch <locator> --skill-id <id> [--id <id>] [--ref <r>] [--path <p>] [--cadence daily|weekly|manual] [--yes]
                                      Watch a remote source for updates and attach as a learning reference
  attach <source-id|locator> --skill-id <id> [--ref <r>] [--path <p>] [--cadence <c>] [--yes]
                                      Attach a source to a skill as a learning reference
  detach <source-id> --skill-id <id> [--yes]
                                      Detach a learning reference from a skill
  unwatch <source-id> [--yes]         Stop watching a source and remove it if unreferenced
  backfill [--skill <id> [--path <p>]] [--yes]
                                      Backfill source records and provenance for legacy skills
  check SELECTOR... | --all-due | --all
                                      Check watched sources for updates
  capture <locator> --reason <text>   Record a candidate source (applies immediately; no --yes)
  list [--status <s>]                 List candidates and sources
  show <id>                           Show one candidate or source
  triage <candidate-id> --decision accept|defer|reject|import [--reason <t>] [--source-id <id>]
         [--adapter git] [--ref <r>] [--path <p>] [--license <l>] [--trust <t>]
         [--cadence <c>] [--skill-id <id>] [--new-skill <id>] [--no-monitor]
                                      accept requires --skill-id or --new-skill; import vendors skills
                                      --trust: community, curated, internal (default: community)
                                      --cadence: daily, weekly, manual (default: weekly)
  confirm --proposal <id> --proposal-digest <d> --base-version <v>
                                      Apply the proposal that triage accept printed
  import <locator|source-id> [--ref <r>] [--path <p>] [--skill <name> | --all] [--yes]
                                      Preview discovered skills and conflicts; --yes imports drafts only
  import --proposal <id> --proposal-digest <d> --base-version <v> --yes
                                      Apply the stored, reviewed import proposal without rediscovery

capture and confirm write source records only; your skills are never changed.
Commands that change the hub ask "Apply this change? [y/N]" in a terminal and preview only without one (apply
with --yes). Each applied change is one Git commit of only its own files, never pushed; --no-commit skips it.
Watching a source does not auto-import skills; import creates draft skills that you review and activate.
Import previews contact the source and may cache a proposal in runtime. No canonical files change.
Use --json for machine-readable output.
`,
	"resolve": `Usage: skillhub resolve --request <request.json> [--workspace <path>] [--json]

Pick the best skill for a request, the same lookup your agent performs. The
request file follows schemas/skill-resolve-request-v1.schema.json (max 64 KiB).
The result is a recommendation; nothing is activated.

Example: skillhub resolve --request request.json
`,
	"resolution": `Usage: skillhub resolution replay --case <file> --manifest <file> [--workspace <path>]
                                  [--output <new-file>] [--json]

Replay a recorded resolution against a retained catalog snapshot, using a strict
experiment manifest (see ` + "`skillhub eval manifest`" + `). Developer tool.

Example: skillhub resolution replay --case case.json --manifest manifest.json
`,
	"maintain": `Usage: skillhub maintain validate [--staged | --full] [--workspace <path>] [--json]
       skillhub maintain rebuild [--verbose] [--workspace <path>] [--json]
       skillhub maintain migrate [--to <version>] [--yes] [--workspace <path>] [--json]
       skillhub maintain diff [--workspace <path>] [--json]

Maintenance commands. skillhub doctor runs the checks and skillhub doctor --fix
the repairs most people need; these are the single steps.

  validate   Validate the hub files (skills, sources, schema) and print each
             problem with its file. --staged validates the files staged in the
             Git index without touching the working tree, for a pre-commit hook.
             It also checks that the routing examples of the changed skills
             still resolve to their skill, against HEAD; --full checks every
             active skill's own examples instead, up to a work limit.
  rebuild    Rebuild the search index from the hub files; only files that pass
             validation are published. Safe at any time. --verbose adds progress
             events and table row counts.
  migrate    Preview a schema migration; --yes applies it, or answer y at the prompt in a terminal.
             --to defaults to 1. The migration is one Git commit; --no-commit skips it.
  diff       Show the uncommitted changes of the hub's Git repository.

Examples:
  skillhub maintain validate
  skillhub maintain validate --staged
  skillhub maintain validate --full
  skillhub maintain rebuild
  skillhub maintain migrate --to 1 --yes
  skillhub maintain diff
`,
	"telemetry": `Usage: skillhub telemetry <health|preview|export|purge|funnel|cases|chains|import-transcripts|baseline> [--workspace <path>] [--json]

Inspect or export the local, disposable telemetry record. Nothing leaves your
machine unless you export it.

Subcommands:
  health                              Show counters and whether the store is healthy
  preview                             Print the sanitized events that would be exported
  export [--output <file> --yes]      Preview, or with --yes write the sanitized file
  purge --yes                         Discard and recreate the telemetry store
  funnel [--since <YYYY-MM-DD|Nd>] [--until <YYYY-MM-DD>] [--skill <id>]
         [--by client|operation|snapshot]
                                      Show recommendation and activation funnel report
  cases [list] [--since <YYYY-MM-DD|Nd>] [--kind <kind>]
                                      List case journal entries (also the bare cases command)
  cases <status|enable|disable>        Inspect or toggle the case journal
  cases suggest [--since <YYYY-MM-DD|Nd>] [--skill <id>] [--min-events <1-100>]
                                      Suggest routing entries from cases and usage (default --since 30d)
  cases dismiss <suggestion-id>        Never suggest this entry again on this machine
  chains [--since <YYYY-MM-DD|Nd>] [--kind override|after_no_skill|reformulation]
                                      Show disagreement chains
  import-transcripts --project <dir> [--since <YYYY-MM-DD|Nd>]
                                      Import tool observations from Claude Code transcripts
  baseline [--since <YYYY-MM-DD|Nd>] [--until <YYYY-MM-DD>] [--min-chains <n>]
           [--write <file>]            Measure baseline sufficiency and optionally save a snapshot

--since takes a date (YYYY-MM-DD, UTC) or a number of days back (Nd, for example 7d).

Example: skillhub telemetry import-transcripts --project ~/projects/my-app
`,
	"eval": `Usage: skillhub eval <manifest|run|promote|routing> [--workspace <path>] [--json]

Run routing evaluations. Developer tool; needs a pinned suite and manifest.

Subcommands:
  manifest (--suite <s> | --case <c>) --experiment-id <id> --variant <v>
           [--seed <n>] [--output <new-file>]      Write an experiment manifest
  run --suite <s> --manifest <m> [--partition development|calibration|held_out]
      [--output <new-file>]                        Run a suite
  promote --resolution-id <id> [--output <new-file>] [--yes]
                                                   Draft an evaluation case from telemetry
  routing [--no-skill <file>] [--policy <file>] [--min-precision F] [--min-recall F]
          [--min-no-skill-recall F] [--max-fpr F]  Evaluate routing across skills' examples

Example: skillhub eval run --suite suite.json --manifest manifest.json
`,
	"version": `Usage: skillhub version [--json]

Print build information: version, commit, build date, and whether the source
tree was modified.

Example: skillhub version
`,
	"mcp": `Usage: skillhub mcp serve [--workspace <path>] [--profile runtime|curation|all]
       skillhub mcp proxy --hub <url> [--token-file <path>]

mcp serve serves the workspace over MCP (stdio). Agent hosts start it for you
after ` + "`skillhub connect`" + `; you rarely run it by hand. Run skillhub help serve
for the network listener that agents on other machines use.

mcp proxy lets an agent host on this machine use a hub on another machine
(started there with skillhub serve). Agent hosts start it for you;
you rarely run it by hand. It forwards skill_resolve and skill_feedback, and
answers skill_get from a local cache: each skill version is fetched once, in
one request, checked against its digest, and kept by digest, so local.path and
the SKILLHUB_* variables point at this machine and a skill's check, setup and
scripts run here. If the hub cannot be reached, cached skills keep working.
Standard output carries MCP; messages go to standard error.

Options:
  --workspace <path>             Workspace to serve
  --profile runtime|curation|all Tool profile to expose (default: all)
  --hub <url>                    Base URL of the remote hub, as its listener printed it
  --token-file <path>            Copy of the hub's token file (mode 0600); without it the
                                 token is read from SKILLHUB_HUB_TOKEN. Never printed.

Fetched skills live in $XDG_STATE_HOME/skillhub/remote (default
~/.local/state/skillhub/remote; on macOS ~/Library/Application Support/skillhub,
on Windows %LocalAppData%\skillhub). Clear them with skillhub cache clear.
`,
	"cache": `Usage: skillhub cache clear [--hub <url>] [--dry-run]

Remove the copies of skills this machine fetched from remote hubs (see skillhub
help mcp) and print what was removed. Without --hub every hub's copies are
removed; with --hub, only that hub's. --hub always takes the hub's URL.
--dry-run shows what would be removed. Agents fetch a skill again the next
time they use it. Installed dependencies and stored secrets are kept, and
nothing on the hub changes. Old versions are also removed automatically: three
versions per skill are kept, versions unused for 30 days go, and each hub's
cache stays under 512 MB.

Examples:
  skillhub cache clear --dry-run
  skillhub cache clear --hub http://127.0.0.1:7420
`,
	"token": `Usage: skillhub token show [--workspace <path>] [--token-file <path>]
       skillhub token rotate [--workspace <path>] [--token-file <path>] [--dry-run]
       skillhub token invite [--workspace <path>] [--addr <host:port>[,<host:port>...]]
                             [--strict-pairing]

Manage the shared bearer token of the network MCP listener (skillhub serve).
The token is created on first use in <workspace>/runtime/config/.hub-token
(mode 0600, never committed) and is never written to logs, telemetry, or URLs.
Other machines get a copy of the file: skillhub serve prints the scp line.

  show              Print the token, only when standard output is a terminal;
                    otherwise refuse and print the token file path
  rotate            Replace the token with a new random one; only in a
                    terminal. --dry-run shows what would change. Every machine
                    that connects needs the new file (rotate prints the scp
                    line to run there); a running listener uses it from its
                    next request
  invite            Create a one-time code, valid for 10 minutes and usable
                    once, and print the skillhub connect --invite line for each
                    address of this machine. The other machine then fetches the
                    token itself; the code is not the token. Only in a terminal.
                    Run it again for a new code
  --workspace <p>   Workspace whose token to use
  --addr <list>     invite: the --addr the server uses, to list the right
                    addresses (default 0.0.0.0:7420). Pass the same --workspace
  --strict-pairing  invite: list only Tailscale and loopback addresses, the ones
                    where the code and the token are not readable on the network
                    (pass it to serve too)
                    and --addr as skillhub serve
  --token-file <p>  Use this token file instead of the workspace one

Examples:
  skillhub token show
  skillhub token rotate
  skillhub token invite
`,
	"update": `Usage: skillhub update [--version <v>] [--check] [--json]

Update skillhub to the latest release or a specified version.

  --version <v>   Target version (e.g. 0.2.0 or v0.2.0)
  --check         Check for updates without modifying the binary
  --json          Machine-readable result
`,
	"serve": `Usage: skillhub serve [mcp] [--addr <host:port>[,<host:port>...]] [--token-file <path>]
                      [--allow-host <name>]... [--workspace <path>] [--strict-pairing]
                      [--install-service]
       skillhub serve web [--addr <host:port>[,<host:port>...]] [--allow-host <host[:port]>]...
                          [--no-open] [--dev] [--workspace <path>]

skillhub serve (or serve mcp) lets agents on other machines use this hub. It
serves the runtime tools only (skill_resolve, skill_get, skill_feedback);
curation stays on this machine. Every request needs the hub token (see
skillhub help token). On start it prints, for each address another machine
can use, an scp line that copies the token file and the skillhub connect
command to run on that machine. Agents on this machine do not need it:
skillhub connect sets them up.

skillhub serve web runs the web UI; the printed URL carries a new session
token. Anyone with that URL can change the skills every connected agent uses.

Both listen on all network interfaces by default (mcp 0.0.0.0:7420, web
0.0.0.0:7421) over plain HTTP and print a warning saying so. On a network you
do not trust, keep the server on this machine with --addr 127.0.0.1:7420 (web:
--addr 127.0.0.1:7421) and reach it from the other machine through an SSH
tunnel (ssh -N -L); the banner prints the tunnel command.

  --addr <list>          1 to 3 comma-separated addresses, one listener each, one
                         token; a missing port takes the default, IPv6 goes in
                         brackets ([::1]:7420), and port 0 picks a free port
  --token-file <path>    Hub token file (mcp; default: <hub>/runtime/config/.hub-token)
  --allow-host <name>    Accept another Host name, for example a VPN DNS name
                         (repeatable; web also takes <host:port>)
  --workspace <path>     A local hub folder (default: the hub of the current folder)
  --install-service      mcp: keep the server running as a service instead of starting it
                         now: a systemd user unit on Linux, a launchd agent on macOS. It
                         starts the server with the --addr, --token-file and --allow-host
                         you pass, restarts it after a failure, and runs it again at boot
                         (login on macOS). Run it again to change those flags
  --strict-pairing       mcp: answer invites (skillhub token invite) only on loopback
                         and Tailscale connections without proxy headers. Without it
                         the invite works over any network the server listens on, in
                         plain HTTP, which is fine at home and risky on a network you
                         do not trust
  --no-open              web: do not open the browser on start
  --dev                  web: development mode behind the Vite dev server (loopback only)

Examples:
  skillhub serve
  skillhub serve --addr 127.0.0.1:7420
  skillhub serve --addr 192.168.1.20,100.101.102.103
  skillhub serve --addr 127.0.0.1:7420 --install-service
  skillhub serve web
`,
}

func isHelpFlag(arg string) bool { return arg == "--help" || arg == "-h" }

// wantsHelp reports whether the leading arguments of a command ask for help.
func wantsHelp(args []string) bool {
	for index := 0; index < len(args) && index < 2; index++ {
		if isHelpFlag(args[index]) {
			return true
		}
	}
	return false
}

func writeGlobalHelp(stdout io.Writer) int {
	fmt.Fprint(stdout, globalUsage)
	return 0
}

func writeDevHelp(stdout io.Writer) int {
	fmt.Fprint(stdout, devUsage)
	return 0
}

// writeCommandHelp prints usage for a known command; ok is false for unknown names.
func writeCommandHelp(stdout io.Writer, command string) (code int, ok bool) {
	if usage, found := commandUsage[command]; found {
		fmt.Fprint(stdout, usage)
		return 0, true
	}
	return 0, false
}

func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeGlobalHelp(stdout)
	}
	if args[0] == "--dev" {
		return writeDevHelp(stdout)
	}
	if code, ok := writeCommandHelp(stdout, strings.TrimSpace(args[0])); ok {
		return code
	}
	return writeInvalidRequest(stdout, stderr, false, fmt.Sprintf("The command %q is not supported.", args[0]), "Run `skillhub help` to list commands.")
}
