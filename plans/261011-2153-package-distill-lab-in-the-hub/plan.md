---
title: "Package distill-lab in the hub and keep system skills once in .agents/skills"
description: "Ship distill-lab as a self-contained Node system skill installed only in the hub folder, keep system skill content once in .agents/skills with one thin wrapper for Claude Code, and make every distill instruction runnable."
status: pending
priority: P2
effort: 36h
branch: main
tags: [system-skills, distill, node, hostintegration, init, doctor, web, docs]
blockedBy: [261011-2250-onboarding-ux-redesign]
blocks: []
created: 2026-10-11
---

# Package distill-lab in the hub and keep system skills once in .agents/skills

## Why (evidence)

Full evidence with `file:line` is in [reports/scout-notes.md](reports/scout-notes.md).

- distill-lab exists only as a dev copy in this code repo (`.claude/skills/distill-lab/`, not embedded). A hub owner
  who clicks **Distill** in the web UI gets a handoff for "an agent that has the distill-lab skill", but nothing
  installs it, and `docs/sources-and-learning.md:178` says Skill Hub "does not ship the distill-lab skill".
- The Home action `review_lessons` prints `distill.py list <path>` (`internal/app/curation_home.go:484-487`),
  which cannot run: no interpreter, no script path. The skill itself falls back to a personal path
  `~/skill-hub` (`.claude/skills/distill-lab/SKILL.md:292`).
- The script needs Python 3 and PyYAML (`.claude/skills/distill-lab/scripts/distill.py:24`), which a hub machine
  may not have; Node is already the runtime of this repo's other script skills (a vendored js-yaml exists under
  `.claude/skills/distill/scripts/vendor/`).
- `init`/`connect` write three identical full copies of `system-curator` (`internal/hostintegration/integration.go:34-36`).
  Codex and Gemini CLI both read `.agents/skills` natively (official docs, cited in the scout notes); Claude Code
  reads only `.claude/skills`.

## Outcome and acceptance criteria

- [ ] After `skillhub init` creates a hub (exact form per the onboarding plan) the hub folder holds `.agents/skills/distill-lab/SKILL.md`,
      `scripts/distill.mjs`, `scripts/vendor/js-yaml.mjs` and `scripts/vendor/js-yaml.LICENSE`, byte-equal to the
      embedded bundle, plus one thin wrapper at `.claude/skills/distill-lab/SKILL.md`. Nothing is written under
      `.gemini/skills/`. The four generated system-skill folders are listed in the hub's `.git/info/exclude`
      (not `.gitignore`), are not in `init`'s first commit, and `git status` stays clean.
- [ ] The connection summary that today reads "connection written: claude-code, codex-cli, gemini-cli" describes
      the new layout; `skillhub version` output is unchanged.
- [ ] `system-curator` follows the same layout: full content once in `.agents/skills/system-curator/SKILL.md`,
      one Claude Code wrapper in `.claude/skills/system-curator/SKILL.md`, in project and user (`-g`) scope (user
      scope: `~/.claude/skills/system-curator/SKILL.md` pointing at `~/.agents/skills/system-curator/`).
- [ ] `connect` in a project other than the hub, `connect -g`, and `connect --hub` never write any distill-lab file
      (asserted by tests for all three request shapes).
- [ ] `connect --hub` and `connect --remove` delete owned system-skill files (current or previously shipped
      bytes, owned wrappers, owned old `.gemini/skills/system-curator/SKILL.md` copies); any other content at those
      paths is reported as a conflict and left in place. distill-lab files are only ever planned in a folder that
      carries the hub marker, so running these commands in this code repo never touches its tracked
      `.agents/skills/distill-lab/` source.
- [ ] A hub created by the previous binary (three full curator copies) is migrated by `doctor --fix` to
      canonical + Claude wrapper, and its owned `.gemini/skills/system-curator/SKILL.md` is removed, with no
      conflict; an edited copy (Claude or Gemini path) produces a conflict naming its path and stays unchanged.
- [ ] Unknown content at a system-skill path is a reported conflict and is left unchanged; known older shipped
      versions still upgrade silently (decided behaviour change, test `doctor-skew_test.go:62-75` updated, release
      note written).
- [ ] `distill.mjs` has the same commands, options and behaviour as `distill.py` (`check`, `format`, `list`,
      `decide`; validation, impact and final_score, sort order, fixed-layout writer, atomic write, `where` check
      against the Git mirrors). `format` on `internal/migration/testdata/*-distill.yaml` reproduces their body
      byte for byte (header lines name the new script), or, if a residual difference cannot be removed, the
      one-time reformat is documented and committed as golden output; in both cases `internal/distill` parses
      and validates the output (decision 13).
- [ ] The skill is self-contained (decision 12): the distill-lab folder copied alone to a scratch folder outside
      any hub runs `node scripts/distill.mjs --help` and `list <fixture>` with exit 0, and a hub-needing command
      (`check` without `--no-where`) exits with a clear `ERROR/WHY/FIX` message, not a stack trace. It needs only
      Node 18 or newer and `git`; it never calls the `skillhub` binary, an MCP tool, npm or the network.
- [ ] `skillhub doctor` in a hub without Node (or with Node older than 18) keeps its status and adds one warning
      naming what is missing and how to install it.
- [ ] `skillhub status` prints the `review_lessons` next step as `cd <hub> && node
      .agents/skills/distill-lab/scripts/distill.mjs list <path>`, and that command runs in the sandbox.
- [ ] The web handoff, Distill tab note, source confirmation text and user docs say where distill runs (an agent
      started in the hub folder) and what it needs (Node); no doc says the skill is not shipped or needs Python.
- [ ] js-yaml 4.3.2 is vendored byte-identical to `dist/js-yaml.mjs` of the published tarball, with its MIT
      license beside it and a section in the release notices file.
- [ ] In this code repo the dev copy uses the shipped layout: content at `.agents/skills/distill-lab/` (the single
      editable source, mirrored byte for byte into `internal/systemskills/distill-lab/`), and
      `.claude/skills/distill-lab/SKILL.md` is the same wrapper the installer renders; the old `distill.py` is gone.
- [ ] `make check` (including the Node-executing script test, which CI runs with Node required on Ubuntu, macOS
      and Windows), `make web-check` and `go test ./internal/hostintegration -run TestMatrixSynchronized` pass.

## Scope and non-goals

In scope: embed distill-lab as a multi-file system skill; rewrite its script from Python to a self-contained
Node ES module with vendored js-yaml; generalize system-skill handling beyond the single curator id;
canonical-plus-Claude-wrapper installer with migration (including removal of the old Gemini copy) and removal;
distill-lab text fixes; product text, tests, docs; sandbox verification; moving this repo's dev copy to the
shared layout.

Non-goals: porting the script into Go or the `skillhub` binary (the skill stays usable without Skill Hub,
decision 12); changing the `distill.yaml` schema or any command's behaviour; any UI or command that launches an
agent; serving distill-lab over MCP or installing it into projects, user scope or remote clients; changing which
hosts are verified in the compatibility matrix; reserving the `distill-lab` id for workspace skills; anything the
onboarding plan owns (command and option names, launch args, permission rules, MCP config writing, README and
`getting-started.md`, `machine_file_tracked`, `skillhub version`).

## Design decisions

1. **Canonical layout.** Every system skill's real files live in `<root>/.agents/skills/<id>/` (SKILL.md and
   `scripts/`). Codex and current Gemini CLI read that folder directly, so they get no wrapper. Claude Code gets
   a one-file wrapper `.claude/skills/<id>/SKILL.md`. Nothing is written under `.gemini/skills/`. The canonical
   files are host-independent, so they become a new shared change kind instead of belonging to one host (today
   Codex's native skill path *is* the canonical path, `integration.go:35`).
2. **Wrapper mechanism: a generated wrapper SKILL.md, Claude Code only.** Same `name` and `description` as the
   canonical frontmatter, a short body that tells the agent to read and follow the canonical SKILL.md and to run
   scripts from the canonical folder, through `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/`, which Claude
   Code substitutes in skill content (official docs). The relative path is the same in project and user scope.
   - Rejected, symlinked directories: Claude Code and Codex document symlink support, but the installer refuses
     symlink components by design (`internal/hostintegration/files.go:19-35,51`), Windows needs Developer Mode
     or admin for symlinks and Git with `core.symlinks=false` checks them out as text files.
   - Rejected, hash-verified copies: that is what we have today and is what the owner's rule removes.
   - Rejected, a Gemini wrapper (owner decision): current Gemini CLI reads `.agents/skills` and gives it
     precedence over `.gemini/skills`, so a wrapper would be shadowed or show the skill twice.
   - Evidence limits: only Claude Code is verified end to end in this project (`docs/mcp-compatibility-matrix.json`).
     Wrapper-following is verified for Claude Code in Phase 8. Gemini CLI and Codex loading from `.agents/skills`
     rests on their official docs and is unverified here; a Gemini CLI too old to read `.agents/skills` no longer
     sees the curator natively.
3. **Bundle model and single source of truth.** `internal/systemskills` exposes a list of system skills, each with
   id, version, files (relative path, bytes), `HubOnly` and `ServedOverMCP` flags, and a list of previously shipped
   file digests. Multi-file content is embedded with `embed.FS`. The existing `CuratorBundle()` API stays for MCP
   distribution. Editable sources:
   - curator: `system-skills/curator/SKILL.md` (unchanged);
   - distill-lab: this repo's `.agents/skills/distill-lab/` (the dev copy in its new home). There is no
     `system-skills/distill-lab/`, so only two copies exist: the source and the embed mirror.
   `go:embed` cannot reach outside the package folder, so `internal/systemskills/distill-lab/` is a byte-identical
   mirror. A sync test (existing pattern, `internal/systemskills/embed_test.go:22-28`) walks both trees, file set
   included, skipping names that start with `.` or `_` like `embed.FS` does; its failure message names the copy
   command, which `AGENTS.md` also documents. A second test asserts that this repo's
   `.claude/skills/distill-lab/SKILL.md` equals the wrapper the installer renders (Phase 9).
   - Rejected, `system-skills/distill-lab/` as source: the dev copy would be a third copy that drifts.
   - Rejected, a new Go package at the repo root embedding `.agents/skills/distill-lab` directly: removes the
     mirror but adds a root-level package and departs from the curator pattern for one skill.
   - Consequence: the repo `.gitignore` ignores `.agents/` today, so Phase 1 replaces that line with patterns that
     keep `.agents/` ignored except `.agents/skills/distill-lab/`.
4. **Ownership and unknown content (decided behaviour change).** A bundle file is ours when its bytes equal the
   current bundle (every file, including the vendored js-yaml and its license) or a previously shipped version
   listed in the known-digest table. The table holds every committed curator version and every committed version
   of the old dev `SKILL.md` (so an unedited hand copy in a hub is recognized and replaced by the wrapper). It
   holds no `distill.py` digest: Skill Hub never installed that file (it existed only in the dev copy), so a
   hand-copied `distill.py` is the user's and is left alone. A wrapper is ours when re-rendering the wrapper
   template with the description found in the file reproduces it byte for byte. Anything else is the user's: it
   is reported as a conflict and never overwritten or deleted. This extends today's single-string check
   (`internal/hostintegration/curator-source.go:27-29`).
   - Owner-approved change: today `doctor --fix` overwrites unknown curator content after confirmation
     (`internal/hostintegration/doctor-skew_test.go:62-75`). Under this plan unknown content is a conflict that is
     left unchanged; known old versions are still upgraded silently. Phase 4 rewrites that test; Phase 7 adds the
     troubleshooting entry and the release-note line.
5. **Executable bit.** Scripts are written `0644` and always invoked as `node <path>`, the same command on
   Windows. No step relies on the exec bit, so Windows and `embed.FS` (which drops modes) behave the same.
6. **Hub-only distill-lab.** distill-lab is planned for install only when the plan is local, project scope, and
   `Root == Workspace` (the hub folder itself). `connect` from a project sets `Root` to the project
   (`internal/app/host-connections.go:23`) and `-g` sets it to home (`:26`), so neither installs it. Removal
   plans consider distill-lab only when the root contains `.skillhub/schema-version`
   (`internal/workspace/workspace.go:75`), because remote and removal plans carry no workspace
   (`internal/app/connect_remote.go:159,441`) and this code repo holds a tracked distill-lab that matches the
   shipped bytes.
7. **Tracked vs ignored: excluded locally, not in `.gitignore`.** The hub's generated system-skill folders
   `/.agents/skills/<id>/` and `/.claude/skills/<id>/` for both system skills (four patterns) are added to
   `<git dir>/info/exclude` through the onboarding plan's helper `ExcludeLocally(root, patterns...)` (in
   `internal/workspace`, idempotent; name to re-verify), the same way that plan excludes the hub's own connection
   files; the hub `.gitignore` is not edited. No `.gemini/skills/` pattern: nothing is written there any more.
   Reason: these files are generated from the binary version, so committing them churns on every upgrade, a clone
   on another machine regenerates them on `init`/`doctor --fix`, and untracked files would make status "dirty"
   (the onboarding plan's reason for `info/exclude`). `init` calls it for the hub before its first commit, and
   `doctor --fix` calls it for older hubs; `doctor` reports a fixable finding when one of the four paths is not
   ignored (`git check-ignore`), and reports already tracked system-skill files under the four folders with a
   `git rm -r --cached` fix (new finding, same pattern as `machine_file_tracked`, which stays owned by the
   onboarding plan). Other skills a user keeps under `.claude/skills/` stay tracked. If an old hub had committed
   `.gemini/skills/system-curator/SKILL.md`, its removal by `doctor --fix` shows as a deletion the owner commits.
8. **Old Gemini copy.** An owned full copy at `.gemini/skills/system-curator/SKILL.md` (current or previously
   shipped bytes, project or user scope) is removed by `init`, `connect` and `doctor --fix`, because Gemini CLI
   reads `.agents/skills` and the stale copy would shadow or duplicate it; empty parents are pruned. An edited
   copy is a conflict and is left.
9. **Removal.** Remote (`connect --hub`) and `--remove` plans delete, for the selected hosts, owned Claude
   wrappers, owned old full copies (Claude and Gemini paths); the shared canonical files go only when no Skill Hub
   registration remains in that root after the plan. Unowned content is left and reported, never fatal for
   `--remove`. A verified MCP-skills host (none today) is not delivered the curator natively: Claude drops its
   curator wrapper; the canonical curator stays while another host in the root still needs it. distill-lab is
   never served over MCP, so its wrapper stays.
10. **Doctor hint.** In the hub, doctor adds one warning (never a status change) when `node` is not on `PATH` or
    `node --version` is older than 18: "distill-lab runs scripts/distill.mjs with Node 18 or newer; <not found |
    found vX>. Install Node from https://nodejs.org". Wrapper and canonical integrity are reported through the
    existing per-file host inspection items. All doctor additions of this plan (Node warning, exclude and tracked
    findings, wrapper and canonical integrity, the `--fix` migration from the three-full-copy layout) are cited
    here against today's `Finding` type and are re-homed at implementation start onto the onboarding plan's
    role-based check set (`DetectRole`; every finding has a code, a message and an optional fix). They go through
    one small adapter (one function that returns this plan's findings for a hub root), so re-homing touches
    only that adapter.
11. **Runnable product text.** The app keeps emitting a hub-relative command
    (`node .agents/skills/distill-lab/scripts/distill.mjs list <rel path>`), so JSON and MCP output carry no
    machine path; the CLI status renderer prefixes `cd <hub> && `.
12. **Self-contained Node skill (owner decision).** distill-lab's script is one ES module, `scripts/distill.mjs`,
    that imports js-yaml from `scripts/vendor/js-yaml.mjs` by relative path: no npm install, no bundler, no
    network. Requirements for the shipped SKILL.md and script:
    - they need only Node 18 or newer and `git`; never Python, the `skillhub` binary or an MCP tool;
    - they locate the hub only from an explicit argument (`--hub <dir>`, or a folder the user names), then
      `$SKILLHUB_WORKSPACE`, then by walking up from the current folder to the first folder that contains
      `.skillhub/schema-version`; they never hard-code a personal path;
    - commands that do not need the hub (`--help`, `list`, `decide`, `check --no-where`, `format --no-where`)
      work anywhere; commands that need it and find none exit with an `ERROR/WHY/FIX` message, not a stack trace;
    - SKILL.md calls `node <skill folder>/scripts/distill.mjs ...` from one documented section only.
    Minimum Node, with evidence: the Node ESM docs (https://nodejs.org/api/esm.html, fetched 2026-10-11) give
    unflagged ES modules from v12.17.0/v13.2.0, stable from v14.17.0/v15.3.0, and `node:` imports from
    v14.13.1. The plan documents Node 18 as the floor (a margin over those, and the oldest line Phase 8 runs the
    script on with an official Node 18 binary); doctor warns below it. GitHub's `ubuntu-latest`,
    `windows-latest` and `macos-latest` images ship Node 22 on `PATH` (runner-images readmes for Ubuntu 24.04,
    Windows 2025 and macOS 15 arm64, fetched 2026-10-11), so CI needs no setup step.
13. **YAML output parity.** PyYAML's output is what existing `distill.yaml` files and the fixtures carry, and
    js-yaml's `dump` cannot reproduce it by options alone: its `flowLevel` is one global depth, but the layout puts
    `decision` and `score.facts` in flow style while their siblings are block; it has no per-node folded style;
    it emits integral floats as `45` where PyYAML writes `45.0`; and its YAML 1.2 quoting differs from PyYAML's
    YAML 1.1 resolver. So `distill.mjs` keeps the layout in its own small writer, as `distill.py` already
    does with its custom dumper, and uses js-yaml only to load (with `CORE_SCHEMA`, so dates and hex-like
    strings stay strings). Phase 2 lists the PyYAML rules the writer reproduces. Parity is proven by a
    differential run against `distill.py` during the rewrite and kept by golden-output tests. Fallback if byte
    parity proves impossible for some input class: accept a one-time reformat on the next `format`, document it
    in the skill and the release note, commit the new golden output, and keep the Go reader tests passing.

## Sequencing with the onboarding plan

Another session implements [261011-2250-onboarding-ux-redesign](../../.claude/worktrees/agent-a5c101367df3589e1/plans/261011-2250-onboarding-ux-redesign/plan.md)
(worktree branch `worktree-agent-a5c101367df3589e1`, read-only for this plan) and merges and releases 5.0.0
first. Implementation of this plan starts in a fresh worktree from the new main only after that session announces
main is final; then the "Re-verify at start" checklist runs before Phase 1.

Ownership split (agreed):

- **Theirs:** launch args (`mcp serve` becomes hidden `mcp stdio`), permission rules (`cli-permissions.go`), MCP
  entry and config writing, `--hub`/`connect` option parsing, command renames (`check`, `integrate`, `web`,
  `mcp serve` removed; hub commands documented as `cd <hub> && skillhub <cmd>`, no `--workspace` in docs
  examples; `init` commits automatically and stores the hub path in per-user config; no preview-then-`--yes` on
  onboarding commands), `machine_file_tracked` and its tests, the role-based doctor/status check set
  (`DetectRole`), `getting-started.md` deletion and the README rewrite, and the text of the curator SKILL.md.
- **Ours:** the skill-file path: `curator-source.go`, the curator/system-skill writer in `integration.go`, its
  removal in `remote.go`, the `internal/systemskills` restructure, distill-lab, and the doctor additions listed
  in decision 10. The one place that prints "connection written: claude-code, codex-cli, gemini-cli" is kept by
  them; Phase 4 changes its wording to match the new layout (Codex and Gemini CLI read `.agents/skills`, Claude
  Code gets a wrapper). `skillhub version` stays untouched (the self-updater and installers call it).
- **Order inside shared files:** our `review_lessons` command change lands after their `curation_home.go` and
  `next_commands.go` edits; our distill sentences in docs, web (`handoff-brief.ts`, `DistillTab.tsx`,
  `DistillHandoffScreen.tsx`) and `source.go` come after their mechanical renames, which leave the distill
  sentences untouched and leave a slot in `docs/sources-and-learning.md` and `docs/web-ui.md`.

## Re-verify at start

Statements in this plan that depend on the onboarding plan's final names. Check each against the new main and
update the affected phase before starting it.

- [ ] `ExcludeLocally(root string, patterns ...string) error` name and package (`internal/workspace`), and where
      `init` calls it before its first commit (decision 7, Phase 5).
- [ ] The doctor/status check-set types and functions (`DetectRole`, the finding type with code, message and
      fix), to re-home decision 10's adapter (Phases 4, 5).
- [ ] Hub command forms used in Phase 6 and Phase 8: `init` (no `--yes`, auto-commit, per-user hub path),
      `status`, `doctor`, `doctor --fix`, `connect`, `connect -g`, `connect --hub`, `connect --remove`,
      `--host`, and that `--workspace` is gone from examples.
- [ ] `cliNextCommand`/`shellArgument` and the `review_lessons` rendering after their edits to
      `internal/delivery/cli/curation.go` and `next_commands.go` (Phase 6).
- [ ] Line numbers cited from `internal/app/workspace.go`, `internal/app/host-connections.go`,
      `internal/app/connect_remote.go`, `internal/workspace/workspace.go`, `internal/delivery/cli/*`,
      `internal/hostintegration/*` (all phases); the curator SKILL.md text they edited (Phase 1 known digests must
      include their new curator versions).
- [ ] The "connection written: ..." output location (Phase 4).
- [ ] Docs file set after 5.0.0: `getting-started.md` deleted, README rewritten, the distill slots in
      `docs/sources-and-learning.md` and `docs/web-ui.md` (Phase 7).
- [ ] Whether the self-connection paths they exclude already cover `.claude/skills/system-curator/` in the hub
      (avoid a duplicate pattern, Phase 5).

## Phases

| # | Phase | Effort | Depends on | File |
|---|---|---|---|---|
| 1 | Embed system skill bundles and distill-lab | 3h | - | [phase-01](phase-01-embed-system-skill-bundles.md) |
| 2 | Rewrite the distill script as a self-contained Node module | 10h | - | [phase-02](phase-02-rewrite-distill-script-in-node.md) |
| 3 | distill-lab skill text: hub lookup, one script section, requirements | 1.5h | 1, 2 | [phase-03](phase-03-distill-lab-skill-text.md) |
| 4 | Installer: canonical folder, Claude wrapper, migration, removal, doctor hint | 8h | 1 | [phase-04](phase-04-installer-canonical-and-wrappers.md) |
| 5 | Exclude generated system-skill folders locally and report tracked ones | 2h | - | [phase-05](phase-05-local-exclude-and-tracked-finding.md) |
| 6 | Product text and tests (Go, web, e2e, one screenshot) | 4h | - | [phase-06](phase-06-product-text-and-tests.md) |
| 7 | User docs | 3h | 2, 3, 4, 5, 6 | [phase-07](phase-07-docs.md) |
| 8 | Sandbox verification | 3.5h | 7 | [phase-08](phase-08-sandbox-verification.md) |
| 9 | Move the dev copy to the shared layout | 1h | 8 | [phase-09](phase-09-move-dev-copy-to-shared-layout.md) |

## Parallel execution and file ownership

| Wave | Phases | Owned files (no overlap inside a wave) |
|---|---|---|
| 1 | 1, 2, 5, 6 | P1: `internal/systemskills/{embed.go,embed_test.go,bundles.go,known-digests.go}`, `internal/systemskills/distill-lab/SKILL.md`, `.agents/skills/distill-lab/SKILL.md` (create), repo `/.gitignore`. P2: `.agents/skills/distill-lab/scripts/**`, `internal/systemskills/distill-lab/scripts/**`, new `internal/systemskills/distill_script_test.go`, new `internal/systemskills/testdata/distill/**`, `web/scripts/notices.mjs`, `web/THIRD_PARTY_NOTICES.md`, `.github/workflows/ci.yml`. P5: `internal/workspace/workspace.go` (new finding only), new `internal/workspace/system_skill_files_test.go`, the `ExcludeLocally` pattern list at the hub `init` and `doctor --fix` call sites (file to re-verify; not `machine_files_test.go`, which the onboarding plan keeps). P6: `internal/app/curation_home.go`, `internal/app/curation_home_test.go`, `internal/app/source.go`, `internal/delivery/cli/source.go`, `internal/delivery/cli/source_test.go`, `internal/delivery/cli/curation.go`, `internal/delivery/cli/workspace_next_steps_test.go`, `internal/distill/distill_model.go`, `web/src/domain/handoff-brief*.ts`, `web/src/screens/skill-detail/DistillTab.tsx`, `web/src/screens/distill/*`, `web/e2e/tier3.spec.ts`, `web/e2e/journeys.spec.ts`, `docs/images/web/distill-handoff.png` + its manifest entry |
| 2 | 3, 4 | P3: `.agents/skills/distill-lab/SKILL.md`, `internal/systemskills/distill-lab/SKILL.md`, `internal/systemskills/embed_test.go`. P4: `internal/hostintegration/**`, `docs/mcp-compatibility-matrix.json`, `internal/app/workspace.go`, new `internal/app/distill_requirements.go` (+ test) |
| 3 | 7 | `docs/*.md` listed in the phase, `docs/design/05-curation-lifecycle.md`, `AGENTS.md` |
| 4 | 8 | none (scratch sandbox only; fixes go back to the owning phase) |
| 5 | 9 | `.claude/skills/distill-lab/**` of this code repo, new `internal/hostintegration/repo_wrapper_test.go` |

Phase 2 is the longest and starts at once: it reads only the dev `distill.py` and writes only under `scripts/`,
so it does not wait for Phase 1, and the sync test passes as soon as both phases land (each mirrors its own
files). Phase 6 hard-codes the canonical script path `.agents/skills/distill-lab/scripts/distill.mjs` fixed by
decisions 1 and 12, so it does not wait for Phase 4. Phase 4 tests read bytes through the bundle API, never
literal content, so Phase 2 and 3 edits do not break them. Between Phase 1 and Phase 9 this repo briefly holds the
old full dev copy (with `distill.py`) in `.claude/skills/distill-lab/` next to the new source; Phase 9 ends that.
Critical path: 2 -> 3 -> 7 -> 8 -> 9 (19h).

## Risks

| Risk | L x I | Mitigation |
|---|---|---|
| The Node rewrite changes behaviour or output of a command | M x H | Differential run against `distill.py` on fixtures and edge-case inputs during Phase 2; golden-output tests kept afterwards; Go reader parses every output; fallback in decision 13. |
| Python-compatible details are missed (`round(x, 2)` ties, `45.0` floats, YAML 1.1 quoting such as `yes` or all-digit SHAs, line folding at width 88, sort by code point) | M x M | Phase 2 lists each rule with a dedicated test input; differential run covers them. |
| Node missing or too old on the hub machine | M x M | Doctor warning, docs requirement line; the binary never needs Node. |
| The vendored js-yaml differs from the published file or misses its license notice | L x M | Pinned 4.3.2, sha256 of `dist/js-yaml.mjs` from the registry tarball recorded and asserted by a test; license beside it and in the release notices. |
| Node-executing test is silently skipped in CI | L x M | It skips only when `node` is missing and `SKILLHUB_REQUIRE_NODE` is unset; CI sets it, so a missing Node fails there. |
| Claude Code does not follow the wrapper and runs with the wrapper text only | M x H | Wrapper body is a single imperative instruction with an explicit path; Phase 8 checks Claude Code by hand. Docs state Claude Code is the verified host. |
| Gemini CLI or Codex does not load `.agents/skills` (old version, undocumented edge) | L x M | Both document it; docs name the requirement; Phase 8 records them as unverified. No `.gemini` fallback by owner decision. |
| Claude Code asks for permission to read `~/.agents/...` from a user-scope wrapper | M x M | Phase 8 checks; if it prompts, the canonical folder must join Claude user `additionalDirectories` (same mechanism as `RuntimeAccessDirs`). Permission rules (`cli-permissions.go`) belong to the onboarding plan's area, so the lead raises it with the owner before Phase 4 edits them. |
| The onboarding plan (5.0.0) renames commands, options, files or the `Finding` type this plan cites | H x M | Implementation starts only after that plan is merged and main is announced final; the "Re-verify at start" checklist lists every dependent statement; doctor additions sit behind one adapter. |
| `connect --remove` in this code repo deletes the tracked distill-lab source or its wrapper (bytes match the shipped ones) | M x H | Decision 6: distill-lab is planned only in a root with the hub marker; Phase 4 test and Phase 8 check in a repo-shaped folder. |
| The repo `.gitignore` change un-ignores more than `.agents/skills/distill-lab/` | L x M | Anchored negation patterns; Phase 1 validates with `git check-ignore` and an empty `git status` diff. |
| The experimental `distill.yaml` schema is now frozen by shipped fixtures and `internal/distill` | M x M | Keep the schema unchanged in this plan; version the skill (`version:` frontmatter) so later schema changes are visible. |
| Conflicts abort a whole `doctor --fix` plan (existing `ConflictError` contract, `integration.go:91-93`), now also for unknown content that used to be overwritten | M x M | Known digests cover every shipped version, so only real user edits conflict; the message names the path and the fix; troubleshooting entry and release note. |
| Windows paths and missing exec bit | L x M | `filepath`-based paths, `0644`, `node <path>` invocation, a Windows path unit test, CI runs the script test on Windows. |
| A hub already has a workspace skill named `distill-lab` | L x L | Decided not to reserve the id. Workspace skills live under `skills/<collection>/` and system distill-lab is not served over MCP, so no file clashes; an agent might list two skills with that name. |

## Compatibility and rollback

- Existing hubs migrate on `doctor --fix` or the next `init`/`connect`; nothing is deleted unless owned.
- Existing `distill.yaml` files stay readable by both the old and the new script; their header lines name
  `distill.py` until the next `format` rewrites them to name `distill.mjs` (comment lines only).
- Downgrading the binary is safe: the old binary sees a wrapper at the old full-copy path and a missing Gemini
  copy, reports skew and, after confirmation, writes its full copies back (`prepareExactFile`,
  `integration.go:390-396`). The re-created `.gemini/skills/system-curator/SKILL.md` is not ignored, so it shows
  as untracked. Left-over distill-lab files are inert and stay excluded.
- Each phase is one or a few commits; revert them in reverse wave order. Phase 5 patterns live only in each
  hub's local `.git/info/exclude` and are harmless after a revert.

## Decisions taken by the owner (2026-10-11)

1. No Gemini wrapper: system skills go to `.agents/skills/<id>/` plus one Claude Code wrapper; owned old
   `.gemini/skills/system-curator/` copies are removed, edited ones reported (decisions 1, 2, 8).
2. `distill-lab` is not a reserved skill id.
3. The dev copy stays in this repo and moves to the shared layout; `.agents/skills/distill-lab/` is the single
   editable source (decision 3, Phase 9). The move is approved; Phase 9 stays last, after verification.
4. Unknown content at a system-skill path becomes a reported conflict (decision 4).
5. The script is rewritten in Node as `scripts/distill.mjs` with vendored js-yaml; no Python, no PyYAML. The skill
   stays self-contained so it can be used without the hub or the binary; no port into the binary is planned
   (decisions 12, 13). Doctor warns when Node is missing.

## Owner follow-ups

None from this plan. The lesson schema is still experimental (Phase 0 scorecard,
`docs/plans/2026-10-10-observation-backlog.md:19`, M6); this plan keeps the `distill.yaml` schema unchanged so
that work is not affected.

## Open questions

None blocking. Residual uncertainty is evidential: Gemini CLI and Codex loading from `.agents/skills` is
unverified in this project (Phase 8 records it), and full byte parity with PyYAML is proven only for the inputs
the differential run covers (decision 13 fallback).
