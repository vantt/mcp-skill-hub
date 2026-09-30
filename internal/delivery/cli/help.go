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

Setup:
  init      Create a workspace (preview by default; --yes applies)
  connect   Connect an agent to a workspace, per project or with -g for all projects
  doctor    Check the workspace and agent connections; --fix repairs
  status    Show what needs attention next

Skills:
  skill     List, show, create, edit, and change the state of skills

Sources & learning:
  source    Register and inspect skill sources
  check     Check sources for updates
  distill   Turn changed sources into skill proposals
  inbox     List insights waiting for review
  insight   Review, apply, or dismiss an insight

Agent:
  resolve   Pick the best skill for a request
  mcp serve Run the MCP server for an agent host

Maintenance:
  validate  Validate canonical workspace files
  rebuild   Rebuild the derived search catalog
  migrate   Preview or apply a canonical schema migration
  diff      Show uncommitted canonical changes
  telemetry Inspect or export local telemetry
  eval      Run routing evaluations
  resolution  Replay a recorded resolution
  version   Print build information
  update    Update skillhub to the latest release or specified version

Workspace selection: pass --workspace <path>, else set SKILLHUB_WORKSPACE, else run
inside the workspace. Run ` + "`skillhub <command> --help`" + ` for a command's flags.
`

var commandUsage = map[string]string{
	"init": `Usage: skillhub init <path> [--yes] [--force] [--verbose] [--json]

Create a Skill Hub workspace at <path> and connect agents to it. Without --yes
the command previews what it would do (directory structure, git repository,
agent connections, search index). --force allows initializing into an existing
non-empty directory. --verbose adds generation IDs, digests, and row counts;
--json prints the full machine-readable result.
`,
	"connect": `Usage: skillhub connect [--project <dir>] [-g|--global] [--workspace <path>]
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
`,
	"doctor": `Usage: skillhub doctor [--workspace <path>] [--fix [--yes]] [--json]

Check the workspace, catalog, and agent connections for this workspace. --fix
previews repairs; add --yes to apply them.
`,
	"status": `Usage: skillhub status [--workspace <path>] [--json|--quiet]

Show workspace health and the single recommended next action.
`,
	"skill": `Usage: skillhub skill <subcommand> [flags]

Subcommands:
  list [--state draft|active|deprecated|archived]   List skills
  show <id> [--verbose]                             Show a skill in any state
  create --id <id> --collection <c> --name <n> --description <d> [--content-file <f>]
         [--operation <o>]... [--trigger <t>]... [--not-for <n>]... [--min-scope <s>] [--yes]
  edit <id> [--name <n>] [--description <d>] [--rationale <r>] [--content-file <f>|--editor]
       [--operation <o>]... [--trigger <t>]... [--not-for <n>]... [--min-scope <s>] [--yes]
  activate <id> | deprecate <id> | archive <id> [--yes]
  confirm --proposal <id> --proposal-digest <d> --base-version <v>

--content-file takes any readable markdown file (relative to the current directory
or absolute). Frontmatter is optional; if present, its "name" field must match the skill id.
Allowed --operation values: explore, design, implement, review, debug, test, refactor, migrate, document, operate, research, other.
Allowed --min-scope values: single_step, multi_step, project.
On edit, routing flags you pass replace that field; the others keep their values.
An active skill needs a trigger, a --not-for entry (or a --rationale), and --min-scope.

Examples:
  skillhub skill create --id my-skill --collection core --name "My Skill" --description "Review changes" --trigger "review code" --not-for "write prose" --min-scope single_step --yes
  skillhub skill edit my-skill --editor
  skillhub skill activate my-skill --yes
`,
	"source": `Usage: skillhub source <subcommand> [--workspace <path>] [flags]

Subcommands:
  capture <locator> --reason <text>   Record a candidate source (applies immediately; no --yes)
  list [--status <s>]                 List candidates and sources
  show <id>                           Show one candidate or source
  triage <candidate-id> --decision accept|defer|reject [--reason <t>] [--source-id <id>]
         [--adapter git] [--ref <r>] [--path <p>] [--license <l>] [--trust <t>]
         [--cadence <c>] [--skill-id <id>] [--no-monitor]
                                      accept previews a proposal; defer and reject apply
                                      --trust: community, curated, internal (default: community)
                                      --cadence: daily, weekly, manual (default: weekly)
  confirm --proposal <id> --proposal-digest <d> --base-version <v>
                                      Apply the proposal that triage accept printed
  import <source-id> [--path <p>] [--skill <name>]... [--yes]
                                      Import existing skills from a watched source as drafts

capture and confirm write source records only; your skills are never changed.
Watching a source does not auto-import skills; import creates draft skills that you review and activate.
Use --json for machine-readable output.
`,
	"check": `Usage: skillhub check [--workspace <path>] [--all-due | --all | <source-id>...] [--json]

Check sources for updates. Contacts each watched source and records whether it
changed; it never edits your skills.

  --all-due         Check only sources whose check interval has passed
  --all             Check every watched source
  <source-id>...    Check the named sources

Example: skillhub check --all-due
`,
	"distill": `Usage: skillhub distill <subcommand> [--workspace <path>] [--json]

Turn changed sources into skill proposals. Normally your agent runs this; the
CLI exposes each step.

Subcommands:
  prepare <source-id>... | --all-changed   Create analysis runs for changed sources
  start <run-id>                           Start a prepared run
  submit <run-id> --submission <file>      Submit findings and insights (JSON file)
  get <run-id>                             Show a run
  retry <run-id> | cancel <run-id>         Retry or cancel a run
  findings | comparisons | insights [--source-id <id>] [--skill-id <id>]
                                           Query the recorded results

Other flag: --idempotency-key <key>.

Example: skillhub distill prepare --all-changed
`,
	"inbox": `Usage: skillhub inbox [--workspace <path>] [--json]

List insights waiting for review, grouped by skill and ranked. Use
` + "`skillhub insight show <id>`" + ` to read one.

Example: skillhub inbox
`,
	"insight": `Usage: skillhub insight <subcommand> [--workspace <path>] [--json]

Review, apply, or dismiss an insight from the inbox.

Subcommands:
  show <id>                                   Show an insight and its findings
  decide <id> --decision plan|reject|obsolete|reopen --reason <text>
  plan|reject|obsolete|reopen <id> --reason <text>
                                              Shorthand for decide
  apply <id> --proposal-file <file>           Preview applying an insight (never applies;
                                              does not accept --yes). The proposal file is a
                                              JSON file specifying changes ([{path, contents}])
                                              and mappings ([{observation_id, artifact_path, concept}]).
  confirm --proposal <id> --proposal-digest <d> --base-version <v>
                                              Apply the previewed proposal
  outcome <incorporation-id> --state <s> --note <t> --evidence <e>...
                                              Record what happened after applying
  provenance --artifact <path>                Which insights touched a file
  impact --finding <id>                       Insights affected by a finding
  operation-diff <operation-id>               Show what an operation changed

Other flag: --idempotency-key <key>.

Example: skillhub insight decide INS-123 --decision reject --reason "already covered"
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
	"validate": `Usage: skillhub validate [--workspace <path>] [--json]

Validate the canonical workspace files (skills, sources, schema). Prints each
problem with its file. Run it after editing files by hand, before ` + "`skillhub rebuild`" + `.

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
	"telemetry": `Usage: skillhub telemetry <health|preview|export|purge> [--workspace <path>] [--json]

Inspect or export the local, disposable telemetry record. Nothing leaves your
machine unless you export it.

Subcommands:
  health                              Show counters and whether the store is healthy
  preview                             Print the sanitized events that would be exported
  export [--output <file> --yes]      Preview, or with --yes write the sanitized file
  purge --yes                         Discard and recreate the telemetry store

Example: skillhub telemetry purge --workspace ~/skillhub --yes
`,
	"eval": `Usage: skillhub eval <manifest|run|promote> [--workspace <path>] [--json]

Run routing evaluations. Developer tool; needs a pinned suite and manifest.

Subcommands:
  manifest (--suite <s> | --case <c>) --experiment-id <id> --variant <v>
           [--seed <n>] [--output <new-file>]      Write an experiment manifest
  run --suite <s> --manifest <m> [--partition development|calibration|held_out]
      [--output <new-file>]                        Run a suite
  promote --resolution-id <id> [--output <new-file>] [--yes]
                                                   Draft an evaluation case from telemetry

Example: skillhub eval run --suite suite.json --manifest manifest.json
`,
	"version": `Usage: skillhub version [--json]

Print build information: version, commit, build date, and whether the source
tree was modified.

Example: skillhub version
`,
	"mcp": `Usage: skillhub mcp serve [--workspace <path>]

Serve the workspace over MCP (stdio). Agent hosts start this for you after
` + "`skillhub connect`" + `; you rarely run it by hand.
`,
	"update": `Usage: skillhub update [--version <v>] [--check] [--json]

Update skillhub to the latest release or a specified version.

  --version <v>   Target version (e.g. 0.2.0 or v0.2.0)
  --check         Check for updates without modifying the binary
  --json          Machine-readable result
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
