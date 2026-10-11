package cli

import (
	"fmt"
	"io"
	"strings"
)

const globalUsage = `Skill Hub keeps one curated set of agent skills and serves it to Claude Code, Codex, and Gemini CLI.

Usage: skillhub <command> [flags]

Get started:
  1. skillhub init ~/skillhub --yes                                  Create your workspace
  2. cd <your project> && skillhub connect --workspace ~/skillhub --yes
     (or: skillhub connect -g --workspace ~/skillhub --yes          Connect every project)
  3. Open your agent there and ask: "curate my Skill Hub"
  On another machine, use that hub through an SSH tunnel or Tailscale:
     skillhub connect --hub <url> --token-file <path> --yes

Setup:
  init      Create a workspace (preview by default; --yes applies)
  connect   Connect an agent to a workspace (-g for all projects), --hub to a hub on another machine
  doctor    Check the workspace and agent connections; --fix repairs
  status    Show what needs attention next

Skills:
  skill     List, show, create, edit, review, add, update from upstream, and change the state of skills

Sources & learning:
  source    Repositories and documents your skills come from or learn from
  check     Check sources for updates

Agent:
  resolve   Pick the best skill for a request
  serve mcp Run the MCP server for an agent host (stdio), or with --addr for other machines
  mcp serve The same stdio server, the command agent hosts already launch
  mcp proxy The stdio bridge to a hub on another machine, which agent hosts launch
  serve web Run the local web UI
  token     Show or rotate the shared token of the network MCP listener

Maintenance:
  validate  Validate canonical workspace files
  rebuild   Rebuild the derived search catalog
  migrate   Preview or apply a canonical schema migration
  diff      Show uncommitted canonical changes
  telemetry Inspect or export local telemetry
  cache     Clear the copies of skills fetched from a remote hub
  eval      Run routing evaluations
  resolution  Replay a recorded resolution
  version   Print build information
  update    Update skillhub to the latest release or specified version

Workspace selection: pass --workspace <path>, else set SKILLHUB_WORKSPACE, else run
inside the workspace. Run ` + "`skillhub <command> --help`" + ` for a command's flags.
`

var commandUsage = map[string]string{
	"init": `Usage: skillhub init [path] [--yes] [--force] [--verbose] [--json]

Create a Skill Hub workspace at [path], or in the current directory when path
is omitted. Without --yes the command previews what it would do (directory
structure, git repository, agent connections, search index). --force allows
initializing into an existing non-empty directory. --verbose adds generation
IDs, digests, and row counts; --json prints the full machine-readable result.
`,
	"connect": `Usage: skillhub connect [--project <dir>] [-g|--global] [--workspace <path>]
                        [--host claude|codex|gemini]... [--yes] [--json]
       skillhub connect --hub <url> [--token-file <path>] [--project <dir>] [-g|--global]
                        [--host claude|codex|gemini]... [--yes] [--json]
       skillhub connect --remove [--cache] [--project <dir>] [-g|--global]
                        [--host claude|codex|gemini]... [--yes] [--json]

Connect agents to a workspace. Writes the MCP registration, the Skill Hub
instruction block, and the system-curator skill. Preview by default; --yes applies.

  --project <dir>   Project to connect (default: current directory)
  -g, --global      Register for your user account so every project sees the hub
  --workspace <p>   Workspace to serve (default: SKILLHUB_WORKSPACE, then discovery
                    from the current directory)
  --host <name>     Limit to claude, codex, or gemini (repeat or comma-separate)
  --yes             Write the files
  --json            Machine-readable result

A hub on another machine (through an SSH tunnel or Tailscale):
  --hub <url>       The base URL the hub printed when ` + "`skillhub serve mcp --addr`" + `
                    started, for example http://127.0.0.1:7420. The agent host then
                    starts ` + "`skillhub mcp proxy`" + `. Curation stays on the hub machine, so no
                    curation server and no system-curator skill are installed.
  --token-file <p>  This machine's copy of the hub's token file. Copy it from the hub
                    machine, for example with scp, and chmod 600 it; only its path is
                    written, never the token. Without it, the agent host must start the
                    proxy with SKILLHUB_HUB_TOKEN set.

Undo a connection:
  --remove          Remove the Skill Hub entry of the scope (local or remote hub), its
                    instruction block and an unchanged system-curator skill; unrelated
                    host configuration stays
  --cache           With --remove, also clear the skills fetched from the remote hub

--workspace and --hub exclude each other: one scope points at one hub, and
connecting it to another hub replaces its entry.
`,
	"doctor": `Usage: skillhub doctor [--workspace <path>] [--fix [--yes]] [--json]

Check the workspace, catalog, and agent connections for this workspace. --fix
previews repairs; add --yes to apply them.

When this project or your user account is connected to a remote hub (connect
--hub), doctor also checks that connection: hub reachable, token accepted,
hub version, skill cache writable, and the last fetch. On a machine without a
workspace it checks only the remote hub. The token is never printed.
`,
	"status": `Usage: skillhub status [--workspace <path>] [--json|--quiet]

Show workspace health and the single recommended next action.
`,
	"skill": `Usage: skillhub skill <subcommand> [flags]

Subcommands:
  add <locator> [--skill <name> | --all] [--id <id>] [--collection <c>] [--ref <r>] [--path <p>] [--yes]
                                                    Add draft skills from local or remote locator
  create <id> --collection <c> --name <n> --description <d> [--content-file <f>]
         [--operation <o>]... [--trigger <t>]... [--not-for <n>]... [--min-scope <s>]
         [--example <e>]... [--counter-example <e>]... [--yes]
                                                    Create a new draft skill (--id <id> remains valid)
  show <id> [--verbose]                             Show a skill in any state
  review <id> [--verbose]                           Comprehensive diagnostic review of a skill
  doctor <id> [--json]                              Check this machine against a skill's runtime block
  env set|unset <id> <KEY> | env list <id>          Store secrets for a skill's scripts (values never shown)
  edit <id> [--name <n>] [--description <d>] [--rationale <r>] [--content-file <f>|--editor]
       [--operation <o>]... [--trigger <t>]... [--not-for <n>]... [--min-scope <s>]
       [--example <e>]... [--counter-example <e>]... [--runtime-file <yaml>] [--approve-content <digest>] [--yes]
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

Note: skillhub skill update <id> updates a skill from its upstream repository; skillhub update updates the skillhub binary itself.
outdated exit codes: 0 normally; with --exit-code, 1 when any skill needs attention (update available, diverged, removed upstream), 2 on invalid flags.

--content-file takes any readable markdown file (relative to the current directory
or absolute). Frontmatter is optional; if present, its "name" field must match the skill id.
Allowed --operation values: explore, design, implement, review, debug, test, refactor, migrate, document, operate, research, other.
Allowed --min-scope values: single_step, multi_step, project.
On edit, routing flags you pass replace that field; the others keep their values.
--example and --counter-example record requests that should (or should not) route to the
skill: at most 10 each, up to 300 characters.
--runtime-file replaces the skill's runtime block with the YAML mapping in the file (requires.bins/env/platforms,
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
  skillhub skill add https://github.com/anthropics/skills --skill pdf --yes
  skillhub skill create my-skill --collection core --name "My Skill" --description "Review changes" --trigger "review code" --not-for "write prose" --min-scope single_step --yes
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
                                      Check watched sources for updates (alias of skillhub check)
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
Watching a source does not auto-import skills; import creates draft skills that you review and activate.
Import previews contact the source and may cache a proposal in runtime. No canonical files change.
Use --json for machine-readable output.
`,
	"check": `Usage: skillhub check [--workspace <path>] [--all-due | --all | <source-id>...] [--json]

Check sources for updates. Contacts each watched source and records whether it
changed; it never edits your skills. When skills track an upstream repository,
check also reports per-skill drift.

  --all-due         Check only sources whose check interval has passed
  --all             Check every watched source
  <source-id>...    Check the named sources

Example: skillhub check --all-due
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
	"validate": `Usage: skillhub validate [--workspace <path>] [--staged] [--json]

Validate the canonical workspace files (skills, sources, schema). Use --staged
to validate files currently staged in the Git index without touching the working tree.
Prints each problem with its file. Run it after editing files by hand, before ` + "`skillhub rebuild`" + `.

Example: skillhub validate
`,
	"rebuild": `Usage: skillhub rebuild [--workspace <path>] [--verbose] [--json]

Rebuild the search index from the workspace files. Only files that pass validation
are published. Safe to run at any time; use it when status says the search index
is stale. Prints a single summary line by default; add --verbose for progress events
and detailed table row counts.

Example: skillhub rebuild
`,
	"migrate": `Usage: skillhub migrate [--to <version>] [--yes] [--workspace <path>] [--json]

Preview or apply a canonical schema migration. Without --yes it only previews.
--to defaults to 1.

Example: skillhub migrate --to 1 --yes
`,
	"diff": `Usage: skillhub diff [--workspace <path>] [--json]

Show uncommitted canonical changes in the workspace's Git repository.

Example: skillhub diff
`,
	"telemetry": `Usage: skillhub telemetry <health|preview|export|purge|funnel|cases|chains|import-transcripts|baseline> [--workspace <path>] [--json]

Inspect or export the local, disposable telemetry record. Nothing leaves your
machine unless you export it.

Subcommands:
  health                              Show counters and whether the store is healthy
  preview                             Print the sanitized events that would be exported
  export [--output <file> --yes]      Preview, or with --yes write the sanitized file
  purge --yes                         Discard and recreate the telemetry store
  funnel [--since <date|Nd>] [--until <date>] [--skill <id>]
         [--by client|operation|snapshot]
                                      Show recommendation and activation funnel report
  cases [list] [--since <date|Nd>] [--kind <kind>]
                                      List case journal entries (also the bare cases command)
  cases <status|enable|disable>        Inspect or toggle the case journal
  chains [--since <date|Nd>] [--kind override|after_no_skill|reformulation]
                                      Show disagreement chains
  import-transcripts --project <dir> [--since <date|Nd>]
                                      Import tool observations from Claude Code transcripts
  baseline [--since <date|Nd>] [--until <date>] [--min-chains <n>]
           [--write <file>]            Measure baseline sufficiency and optionally save a snapshot

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

mcp serve is an alias of skillhub serve mcp, kept for the agent host
registrations that already run it. Serve the workspace over MCP (stdio). Agent
hosts start this for you after ` + "`skillhub connect`" + `; you rarely run it by hand.
Run skillhub help serve for the network listener.

mcp proxy lets an agent host on this machine use a hub on another machine
(started there with skillhub serve mcp --addr). Agent hosts start it for you;
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
on Windows %LocalAppData%\skillhub). Clear them with skillhub cache clear --hub.
`,
	"cache": `Usage: skillhub cache clear --hub [<url>] [--yes]

Remove the copies of skills this machine fetched from remote hubs (see skillhub
help mcp). Without a URL every hub's copies are removed; with one, only that
hub's. Previews by default; --yes applies. Agents fetch a skill again the next
time they use it. Installed dependencies and stored secrets are kept, and
nothing on the hub changes. Old versions are also removed automatically: three
versions per skill are kept, versions unused for 30 days go, and each hub's
cache stays under 512 MB.

Examples:
  skillhub cache clear --hub
  skillhub cache clear --hub http://127.0.0.1:7420 --yes
`,
	"token": `Usage: skillhub token show [--workspace <path>] [--token-file <path>]
       skillhub token rotate [--workspace <path>] [--token-file <path>] [--yes]

Manage the shared bearer token of the network MCP listener (skillhub serve mcp
--addr). The token is created on first use in <workspace>/runtime/config/.hub-token
(mode 0600, never committed) and is never written to logs, telemetry, or URLs.

  show              Print the token, only when standard output is a terminal;
                    otherwise refuse and print the token file path
  rotate            Preview replacing the token; --yes applies. Every machine
                    that connects needs the new file; a running listener uses
                    it from its next request
  --workspace <p>   Workspace whose token to use
  --token-file <p>  Use this token file instead of the workspace one

Examples:
  skillhub token show
  skillhub token rotate --yes
`,
	"update": `Usage: skillhub update [--version <v>] [--check] [--json]

Update skillhub to the latest release or a specified version.

  --version <v>   Target version (e.g. 0.2.0 or v0.2.0)
  --check         Check for updates without modifying the binary
  --json          Machine-readable result
`,
	"serve": `Usage: skillhub serve mcp [--workspace <path>] [--profile runtime|curation|all]
       skillhub serve mcp --addr <host:port> [--trust-private-network]
                          [--token-file <path>] [--allow-host <host>]... [--workspace <path>]
       skillhub serve web [--workspace <path>] [--addr <host:port>]
                          [--loopback-only] [--allow-host <host[:port]>]...
                          [--no-open] [--dev]

serve mcp without --addr serves the workspace over MCP (stdio), the way agent
hosts launch it after ` + "`skillhub connect`" + ` (skillhub mcp serve is an alias).

serve mcp --addr runs a network listener so agents on other machines can use
this hub. It serves the runtime tools only (skill_resolve, skill_get,
skill_feedback); curation stays on this machine. Every request needs the shared
token (see skillhub help token) in the Authorization header. The listener
speaks plain HTTP: it starts on a loopback address (127.0.0.1, ::1), which you
reach through an SSH tunnel, and refuses any other address unless you pass
--trust-private-network for a private network such as Tailscale. On start it
prints the skillhub connect command to run on the other machine.

  --profile runtime|curation|all  Tool profile for stdio (default: all); with --addr only runtime
  --addr <host:port>              Listen on the network instead of stdio
  --trust-private-network         Allow a non-loopback address on a network you trust
  --token-file <path>             Token file (default: <workspace>/runtime/config/.hub-token)
  --allow-host <host>             Accept another Host name, for example a VPN DNS name (repeatable)

serve web runs the local web UI. Listens on 0.0.0.0 when this machine has two or more non-loopback IPv4 addresses, otherwise on 127.0.0.1; --addr or --loopback-only override.

  --workspace <path>           Workspace to serve
  --addr <host:port>           Explicit listen address and port
  --loopback-only              Listen only on 127.0.0.1
  --allow-host <host[:port]>   Allow an additional Host header (repeatable)
  --no-open                    Do not open the browser on startup
  --dev                        Development mode for Vite proxy

Examples:
  skillhub serve mcp --addr 127.0.0.1:7420
  skillhub serve mcp --addr 100.101.102.103:7420 --trust-private-network
  skillhub serve web
`,
	"web": `Usage: skillhub web [options]

An alias of skillhub serve web. Run skillhub help serve for options.
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
	if code, ok := writeCommandHelp(stdout, strings.TrimSpace(args[0])); ok {
		return code
	}
	return writeInvalidRequest(stdout, stderr, false, fmt.Sprintf("The command %q is not supported.", args[0]), "Run `skillhub help` to list commands.")
}
