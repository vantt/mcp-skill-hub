# UX surface audit: is Skill Hub easy to use without searching?

Date: 2026-09-30 (Asia/Saigon). Scope: the surface only. That means README, docs/user-guide.md, every `skillhub help` page, human CLI output, first-run init/connect/status/doctor, the curator skill, the bootstrap block, and MCP tool names and descriptions.

Method: I built the binary with `go build -o $S/skillhub ./cmd/skillhub` and ran it with `HOME=$S/home` (S = session scratchpad/uxaudit). I ran everything under bash and did not change any repo file. I followed the README Quickstart word for word, then each user-guide task. The real GitHub source was `https://github.com/anthropics/skills.git`. I also called the MCP tools over stdio (`skillhub mcp serve`).

## Verdict

No, not yet. The first 5 minutes are good: init, commit, connect, and restart work, and each step prints a clear "Next:" hint. After that it gets hard. Three promises in the README "What next" section break for a newcomer:

- "Ask your agent to create a skill" fails. The MCP server has no tool to create a skill.
- "Bring in skills from a GitHub repo" does not import anything.
- `skillhub status` fails when you run it inside the project you just connected.

Past the first run, the CLI speaks in internal terms: pins, digests, generations, catalog snapshots, canonical, native-skill-instruction-coordination. Many errors end with a generic FIX line instead of a command you can run.

## P0: blocks a newcomer

### P0-1. The agent cannot create, activate, or list skills, but the docs tell users to ask it
- Where:
  - README.md:52 ("Ask your agent ... Create a skill for reviewing reliability risks").
  - docs/user-guide.md:110 (create), :140 ("Ask your agent: Activate reliability-review").
  - internal/delivery/mcpserver: 27 tools. None of them creates a skill, changes its state, or lists skills.
- What the user sees: I called `skill_update_preview` with a new `skill_id` over MCP. It returned `"The request conflicts with the current object state." / "Refresh the object state, correct the request, and retry."`. The curator's intent table (system-skills/curator/SKILL.md:57-73) has no row for activate, deprecate, archive, or list. `skills/list` is a custom JSON-RPC method, not a tool the agent can call.
- Why it hurts: the headline flow is "talk to your agent", and the first thing the docs suggest cannot be done that way. At best the agent falls back to telling the user to run CLI commands. At worst it edits files by hand, which the curator forbids.
- Direction: pick one:
  - Add MCP tools for create, lifecycle change, and list/show.
  - Or change README and the guide to say these are CLI-only, and add curator rows that hand off to the exact CLI command.
  - Also make the "not found" case of `skill_update_preview` say "skill X does not exist".

### P0-2. "Bring in skills from a GitHub repo" does not bring in skills
- Where: README.md:53; docs/user-guide.md:166-205; curator intent "Start learning from a source".
- What the user sees:
  - After `source confirm`, `skillhub check --all` prints `anthropics-skills: up_to_date`. The newly watched source is used as the baseline, so there is nothing to distill.
  - `skillhub inbox` prints `Insight inbox is empty.`
  - Forcing `skillhub distill prepare anthropics-skills` prints `Prepared 0 source run(s); 1 source(s) failed independently. ... - anthropics-skills: failed: source exceeded configured limits` and exits 0.
  - Triage accepted this repo without any size warning.
- Why it hurts: the user expects to get skills from the repo. What they get is a watcher that only reports future upstream changes. And for a typical skills repo, even that fails, with no word on which limit was hit or how to narrow it (`--path` exists on triage but is never suggested).
- Direction:
  - Say plainly in README, the guide, and the triage/confirm output: "Watching learns from future changes; it does not import skills."
  - Offer an import or first-analysis path, or say it is not supported.
  - Make the limits error name the limit (files/bytes) and suggest `source triage ... --path <subdir>`. Warn at triage time.
  - Return a non-zero exit code when every source failed.

### P0-3. A typo in the workspace path leads the user to create a new, empty workspace
- Where: `skillhub status --workspace ~/skilhub` (typo), then the recommended fix. See internal/app/workspace.go:41-54 (Doctor treats a missing path as "will be created").
- What the user sees: `status` prints `Workspace needs repair ... Workspace invalid; search index stale; Git not configured ... Recommended next: Run skillhub doctor --fix.` and exits 0. `doctor --fix --yes` then creates `~/skilhub` and prints 30+ lines: a generation ID, two sha256 values, 17 lines of `<table>: N row(s)`, and 9 connection lines.
- Why it hurts: the user ends up with a second, empty hub and agent files pointed at it. The real mistake, a wrong path, is never named.
- Direction: for status, doctor, and every command except `init`, a missing path should give "No Skill Hub workspace at <path>. Did you mean ...? To create one: `skillhub init <path> --yes`". Only `init` should create a workspace.

### P0-4. Everyday commands fail inside a connected project, with misleading errors
- Where: README.md:54 (What next: `skillhub status`) right after "cd your-project; connect". The `SKILLHUB_WORKSPACE` tip (README.md:42) is optional.
- What the user sees, for the same underlying cause (no workspace given):
  - `skillhub status` → `ERROR: The workspace is invalid. / WHY: no workspace found; pass --workspace <path> / FIX: Run skillhub doctor --workspace <path> ...`
  - `skillhub doctor` → `WHY: --workspace is required for non-interactive use`
  - `skillhub skill list` → `WHY: no workspace found ... / FIX: Correct the skill fields or workspace state and retry.`
  - `skillhub skill lst` (a typo) → `--workspace is required for non-interactive use`. The typo is hidden.
  - `skillhub skill create --id foo` (missing flags) → the same workspace error. The missing flags are hidden.
  - Only `connect` gets it right: `No Skill Hub workspace was found ... FIX: Pass --workspace <path> or set the SKILLHUB_WORKSPACE environment variable.`
- Why it hurts:
  - The workspace is not invalid; it simply was not found.
  - "Non-interactive use" means nothing to a newcomer.
  - The workspace check runs before argument parsing, so real typos stay hidden.
  - The connected project already records the workspace path in `.mcp.json`, `.codex/config.toml`, and `.gemini/settings.json`, but the CLI does not use it.
- Direction:
  - Use one message everywhere, the `connect` wording.
  - Parse the subcommand and flags before resolving the workspace.
  - Consider falling back to the workspace recorded in the current project's connection files.
  - Make the README Quickstart set `SKILLHUB_WORKSPACE` as a real step, not a tip.

## P1: confusing or slow

### P1-1. The "pins" confirmation model leaks into every preview
- Where: internal/delivery/cli/skill.go:405,415; source.go:255; insight.go:238; docs/user-guide.md:187-191.
- What the user sees: `- Proposal: PROP-... - Digest: sha256:... - Base catalog: sha256:... No files changed. Confirm these exact pins with skillhub skill confirm; --yes confirms only the proposal shown in this invocation.`
- Why it hurts:
  - Two confirm paths (`--yes` or `confirm`) are shown with no guidance on which to use.
  - The output labels the value "Base catalog", but the flag is `--base-version`. The user has to work out the mapping.
  - For sources there is no `--yes`, so the user must copy three hashes by hand.
  - A mistyped digest gives `Proposal is stale; nothing was applied.`, which is false: it is a typo, not a stale proposal.
- Direction:
  - Print the full ready-to-run `confirm` command.
  - Name the value after its flag ("Base version").
  - Give `source triage` a `--yes`, like skill.
  - Tell "digest does not match" apart from "stale", and point to rerunning triage.

### P1-2. Connect and doctor output is noisy and full of jargon
- Where: internal/app/workspace.go:439 (`"%s (%s) %s: %s"`), :445 (warning); internal/hostintegration/types.go:26.
- What the user sees: 9 lines like `- claude-code (native-skill-instruction-coordination) mcp-registration: /abs/path/.mcp.json`, then `WARNING: Host activation coordination is instruction-only and best effort; each Agent Host remains responsible for permissions and activation.`
- Why it hurts:
  - The WARNING appears on every connect and every doctor run, even on a no-op ("already current; nothing to change").
  - Newcomers read "WARNING" as "something is wrong".
  - The level string means nothing to them.
- Direction:
  - Group by agent and show relative paths, e.g. `Claude Code: .mcp.json, CLAUDE.md, .claude/skills/system-curator`.
  - Drop the level from human output.
  - Show the caveat once, in plain words ("Your agent decides whether to follow these instructions"), or only with `--verbose`.

### P1-3. `doctor` says "healthy" while the project's connection is broken
- Where: `cd proj3; skillhub doctor` after putting a bad command path in proj3/.mcp.json. See internal/app/workspace.go:41-90: it only inspects the workspace's own host files.
- What the user sees: `Workspace is healthy.`
- Why it hurts: user-guide.md:237-275 sends users to doctor when "the agent does not see the hub". Doctor never looks at the current project or at the global (`-g`) registration.
- Direction: when run inside a connected project, and for the global config, check that the recorded binary and workspace paths exist. Report them with the exact `connect` command that repairs them.

### P1-4. Generic FIX lines give no runnable command
- Where: internal/delivery/cli/skill.go:473 (`Correct the skill fields or workspace state and retry.`), source.go:277, the insight equivalent.
- What the user sees (from real runs):
  - `skill not found` → generic FIX. It should suggest `skillhub skill list`.
  - `skill already exists` → generic. It should suggest `skill edit <id>`.
  - `invalid skill lifecycle transition: draft -> deprecated` / `active -> archived` → generic. It should name the next valid step (`skill activate`, `skill deprecate`).
  - `$VISUAL or $EDITOR must be set for --editor` → generic. It should say `EDITOR=nano skillhub skill edit ... --editor`.
  - `SKILL.md frontmatter name must be "reliability-review"` → generic.
  - `accept requires a safe --source-id` → generic, and no suggested id. It could derive `owner-repo` from the URL.
  - Unknown candidate: `statat sources/intake/SRCQ-NOPE.yaml: no such file or directory`. A raw Go error leaks through.
  - `skillhub check nope` → `Source checks completed` and exit 0 for an unknown id.
  - `skill create ... --min-scope big` → FIX suggests `skill edit foo2 ...` for a skill that does not exist yet.
- Direction: map each known error to a specific FIX. Never print raw OS errors. Treat an unknown id as an error.

### P1-5. Activation reports missing fields one at a time (known; still present)
- Where: `skill activate x2 --yes`.
- What the user sees: first `requires at least one routing trigger`. After fixing that, `requires routing.min_scope`. Each takes a new round trip. user-guide.md:125 says "If one is missing, prints the exact command", which is true, but only for the first missing field.
- Direction: list every missing field, with one combined `skill edit` command.

### P1-6. The `--content-file` frontmatter rule is wrong in the docs and missing from help (known; refined)
- Where: docs/user-guide.md:123; `skillhub help skill`.
- Evidence:
  - The guide says the file "must start with SKILL.md front matter whose name matches the skill id". In practice a file with no frontmatter is accepted, and frontmatter is added for you.
  - Only a file that has frontmatter with a different `name` is refused, with a generic FIX.
  - `skillhub validate` accepts a hand-edited SKILL.md whose name does not match. The rule is enforced in one path only.
  - The help line "the skill's own SKILL.md is only written through the change itself" is hard to parse.
- Direction: document the real rule in help ("frontmatter optional; if present, `name` must equal the id"). Make validate and the CLI agree. Consider rewriting the name instead of refusing.

### P1-7. After create or activate, the user gets no "what next"
- Where: every mutation receipt.
- What the user sees: `Draft skill reliability-review saved. / Operation: OP-... / Catalog snapshot: sha256:... / Generation: gen-... / Git dirty: true`.
- Why it hurts:
  - The new SKILL.md body is just the description repeated. The user is not told to write the actual instructions (`skill edit <id> --editor`), to activate, or to commit.
  - "Git dirty: true" is developer slang.
  - An edit of a draft prints `Draft skill x2 saved.` (same text as create), while an edit of an active skill prints `Skill ... updated; it is active.`
- Direction:
  - Hide the IDs unless `--verbose`, as `init --help` already promises.
  - End with one next step, e.g. `Next: write the instructions (skillhub skill edit <id> --editor), then skillhub skill activate <id> --yes. Commit with git -C <ws> commit.`

### P1-8. `skill show` hides the routing fields and uses jargon
- What the user sees: `Skill reliability-review (active) loaded from a digest-pinned catalog snapshot. / State: active / Catalog snapshot: sha256:...`, then SKILL.md only.
- Why it hurts: triggers, not-for entries, and min-scope decide routing. The user cannot see them without opening skill.meta.yaml, and the file path is never shown.
- Direction: print the name, description, triggers, not-for, min-scope, state, and file path. Drop "digest-pinned".

### P1-9. After a hand edit, skills disappear until you rebuild
- Where: user-guide.md:34 says the catalog is "rebuilt automatically after changes".
- What the user sees after appending one line to SKILL.md:
  - `skill show` → `ERROR: The pinned catalog snapshot is no longer available. / WHY: snapshot_expired: ...`
  - `skill list` → `WHY: catalog is stale ... FIX: Correct the skill fields ...`
  - `resolve` fails the same way. The agent's `skill_resolve` depends on the same catalog.
- Why it hurts: one harmless edit silently turns off routing for every project. Only CLI changes rebuild automatically.
- Direction: rebuild automatically on read when the only problem is stale inputs that pass validation, or at least do it in `mcp serve`. At minimum, make every stale error's FIX be `skillhub rebuild`.

### P1-10. `validate` does not say what is wrong, and the doctor loop is circular
- Where: `skillhub validate` after I set `min_scope: huge` by hand. See internal/app/workspace.go:467.
- What the user sees:
  - `ERROR: The workspace is invalid. / WHY: Canonical validation found 1 issue(s). / FIX: Run skillhub doctor ...`. The issue itself is not printed, although `help validate` says "Prints each problem with its file".
  - `status` shows `0 active skills; 0 watched sources` (wrong; they exist) and recommends `doctor --fix`.
  - `doctor --fix --yes` fails with a FIX that points back to doctor.
- Direction:
  - `validate` should print each issue with its file and the exact edit or command.
  - `status` should say the counts are unavailable instead of showing 0.
  - Don't recommend `doctor --fix` for content errors it cannot fix.

### P1-11. The empty-hub status gives no first step
- Where: internal/app/curation_home.go:423; internal/delivery/cli/curation.go:42,54,63.
- What the user sees on a fresh, committed hub: `Skill Hub is up to date. 0 active skills; 0 watched sources. ... blocking decisions: 0 (not configured); routing evaluations: 0 (not configured). Recommended next: Continue normal work.`
- Why it hurts:
  - With zero skills, "continue normal work" is a dead end.
  - The "not configured" line is noise for users.
  - When there are uncommitted changes, the output says `Recommended next: Review uncommitted changes.` without a command.
  - `1 active skills` has a plural bug.
- Direction:
  - For 0 skills, recommend the create command (or the agent phrase).
  - Hide the not-configured categories.
  - Always print a runnable command next to "Recommended next".

### P1-12. The agent receives internal operation names, not tools
- Where: internal/app/curation_home.go:327-444, `hub_status` result `actions[].command`.
- What the agent sees: `GetCurationDiff`, `PrepareDistillRuns`, `BuildCatalogGeneration`, `DoctorFix`, `ContinueNormalWork`. None of these is an MCP tool name (`workspace_diff`, `curation_run_start`, `workspace_rebuild`) or a CLI command.
- Direction: return the matching tool name, and the CLI equivalent where one exists.

### P1-13. `init` in a project folder quietly turns the project into a workspace
- What the user sees: `skillhub init <existing project dir> --yes` → `Workspace ready at .../proj1. Agent connection for this workspace written...` with no warning. It adds skills/, sources/, history/, and other folders to the project.
- Why it hurts: newcomers mix up `init` and `connect`.
- Direction: when the target is a non-empty folder that is not a workspace, stop and ask "Did you mean `skillhub connect`?".

### P1-14. The symlink refusal has the wrong headline and FIX
- Where: `connect -g` when `~/.claude` is a symlink. This is common with dotfile managers.
- What the user sees: `ERROR: The workspace is invalid. / WHY: host integration path contains a symbolic link: ... / FIX: Run skillhub doctor ...`
- Why it hurts: the workspace is fine, and doctor cannot help. The right fix is only in the guide (user-guide.md:271).
- Direction: use the guide's wording as the FIX: "use per-project `skillhub connect --yes`, or replace the symlink".

### P1-15. The `resolve` example in the guide returns a question, and nothing explains how to answer it
- Where: docs/user-guide.md:215-218.
- What the user sees: `What is the bounded scope of this task?`. With `--json`: `status: needs_context`, `choices: single_step|multi_step|project|unknown`.
- Direction: add `"scope"` to the example request so it returns a recommendation, or explain how to answer.

### P1-16. Connection files store absolute machine paths (known; still present), and nobody says whether to commit them
- Evidence:
  - The project's `.mcp.json` and `.codex/config.toml` contain `/abs/path/skillhub` and `--workspace /abs/home/skillhub`.
  - The workspace's own `.mcp.json` is committed by the README's `git add -A`. So a clone on another machine has wrong paths, and the remote exposes the home path.
  - The guide's "Move to another machine" section (user-guide.md:222-233) does not mention `doctor --fix` for the workspace's own files.
- Direction:
  - Say in connect output and the guide whether to commit project connection files. Teammates' paths will differ.
  - Add `doctor --fix --yes` to the move steps.
  - Consider bare `skillhub` from PATH when it resolves to the same binary.

### P1-17. `skillhub diff` mislabels files and is not a diff
- Where: internal/delivery/cli/curation.go:83-85.
- What the user sees: `active skills (4):` lists the files of the draft skill x2, followed by quoted `?? "path"` lines.
- Direction: label it "skills". Show the git status letters in words. End with the commit command.

### P1-18. Exit codes are unreliable for scripts
- `distill prepare` with every source failed → exit 0.
- `check <unknown-id>` → exit 0.
- `status` on an invalid or missing workspace → exit 0.

## P2: polish

- Jargon in help and output. A newcomer won't know:
  - "canonical" (help for validate, migrate, diff; `Git has uncommitted canonical changes`), "derived search catalog", "catalog snapshot", "generation", "pins".
  - "Source onboarding is ready for review", "failed independently".
  - "Direct routing fields for x were evaluated against sha256:...", "semantic resolver impact is not claimed".
  - "Skill active transition is ready for review", "non-interactive use".
  - "Agent Host" (capitalized); "substantive task", "primary procedure", "pinned resources" (bootstrap block, internal/hostintegration/bootstrap.go:24-30).
  - The curator description "application-service-backed curation tools" (SKILL.md:5).
- Inconsistent terms:
  - watched / watching / monitored (`1 monitored source(s)` vs `[watching]` vs "watched" in the guide).
  - catalog / search index / catalog version / Base catalog / base-version.
  - `--host claude` vs the output's `claude-code`.
  - insight / idea / proposal.
  - The curator's user-term table (SKILL.md:104-113) lacks proposal, digest, base version, and generation. Those are exactly the terms the CLI prints.
- Top-level help (`skillhub help`): "Get started" leaves out the commit step that init itself prints. The footer says `skillhub <command> --help`, while the README says `skillhub help <command>`. Both work, but pick one.
- Help gaps:
  - `--operation` values are not listed.
  - `--trust` and `--cadence` values are not listed.
  - `skill` help has no example.
  - `skill confirm` appears in help but not in the guide.
  - The `insight apply --proposal-file` format is not documented anywhere a user would look.
  - `migrate` says "--to defaults to 1" with no context on when you would need it.
- The `init` preview is thin: `- .: Workspace directory will be created.` It does not say that it also creates a Git repo and agent files, and it does not print the `--yes` command (connect does).
- `source capture` of the same URL twice silently creates a second pending candidate. Capture output does not suggest the triage command.
- `source show anthropics-skills` prints the list header `2 candidate(s) and 1 monitored source(s).`
- `skill create --collection nosuch` is accepted silently (a typo creates a new collection), and there is no way to list collections.
- `connect -g` pointed at a different workspace does not show old → new before overwriting.
- Connecting both globally and per project writes the bootstrap block twice into the agent context (~/.claude/CLAUDE.md and the project CLAUDE.md).
- `rebuild` prints 12 progress lines plus 17 table row counts by default. `--verbose` should gate these.
- `init --help` says `--verbose` adds generation IDs and digests, but rebuild, doctor --fix, and every mutation receipt print them anyway.
- README.md:61: the design docs are in Vietnamese. That is fine, but a user clicking there to understand concepts hits a wall. The guide's concepts section covers it.
- Install wording (release is covered separately): README step 1 says "Release downloads are not published yet", and the lower section says the URL "currently returns 404". This is honest. Consider linking the cosign install, since the installer requires it.
- Not verified, may surprise users: Claude Code usually asks the user to approve a project `.mcp.json` server on first start. Codex may load project `.codex/config.toml` only for trusted projects. Gemini gets `"trust": false`. "Restart your agent" in the README and connect output doesn't mention any of these prompts.

## What already works well

- Init, commit, connect, restart takes 4 commands. `init` prints a numbered "Next:" list with exact commands.
- Preview-by-default and `--yes` is the same everywhere, and preview previews write nothing.
- `connect` is idempotent (`already current; nothing to change`). It keeps user text in CLAUDE.md and AGENTS.md, and refuses to write through symlinks.
- `connect` errors are good models: an unknown `--host claud` lists valid values, and a missing workspace gives an exact `init` command.
- The ERROR/WHY/FIX structure is consistent, and several FIX lines are exact (activation `skill edit ... --trigger`, `--min-scope` choices, the rebuild hint on stale status).
- The user guide is short, task-based, and pairs each task with an "ask your agent" phrase and a CLI form. The cheat sheet is useful.
- `skill list` and `source list` are compact tables.
- The curator skill has strong safety framing: one question per turn, progressive disclosure, a user-term mapping, and "Active skills were not changed" guarantees.
- The MCP tool names are consistent (`noun_verb`), and the descriptions state side effects ("does not fetch", "never changes canonical files").

## Unresolved questions

1. Is "watch a source" meant to import existing skills at some point? If not, README.md:53 should stop saying "bring in skills".
2. Are skill create and lifecycle changes deliberately CLI-only for the agent? If so, the README and guide agent phrases should change.
3. Should project connection files be committed to shared project repos? Absolute paths make them per-machine.
4. Host approval prompts (Claude project MCP approval, Codex trusted projects) were not verified here.
