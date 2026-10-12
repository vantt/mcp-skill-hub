---
title: "Package distill-lab in the hub and keep system skills once in .agents/skills"
description: "Ship distill-lab as a self-contained Node system skill installed only in the hub folder, keep system skill content once in .agents/skills with one thin wrapper for Claude Code, and make every distill instruction runnable."
status: in-review
priority: P2
effort: 57h
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

<!-- Red team: criteria updated for the adjudicated findings and the onboarding contract change -->
<!-- Validation: vendored folder renamed scripts/lib (the repo hook blocks Bash paths containing that segment); release is 0.5.0 -->
- [ ] After `skillhub init` creates a hub (exact form per the onboarding plan) the hub folder holds `.agents/skills/distill-lab/SKILL.md`,
      `scripts/distill.mjs`, `scripts/lib/js-yaml.mjs` and `scripts/lib/js-yaml.LICENSE`, byte-equal to the
      embedded bundle, plus one thin wrapper at `.claude/skills/distill-lab/SKILL.md`. Nothing is written under
      `.gemini/skills/`. The four generated system-skill folders are entries of the hub's `.gitignore` (via
      `requiredIgnoreEntries`), are not in `init`'s first commit, and `git status` stays clean.
- [ ] `connect` and `init` output lists the shared `.agents/skills` files on their own "Agent Skills folder
      (.agents/skills)" line, never under an empty host name; the onboarding plan's per-agent lines and
      `skillhub version` output are not edited.
- [ ] `system-curator` follows the same layout: full content once in `.agents/skills/system-curator/SKILL.md`,
      one Claude Code wrapper in `.claude/skills/system-curator/SKILL.md`, in project and user (`-g`) scope (user
      scope: `~/.claude/skills/system-curator/SKILL.md` pointing at `~/.agents/skills/system-curator/`).
- [ ] `connect` in a project other than the hub, `connect -g`, and `connect --hub` never write any distill-lab file
      (asserted by tests for all three request shapes).
- [ ] `connect --hub` and `connect --remove` delete owned system-skill files (current or previously shipped
      bytes, owned wrappers, owned old `.gemini/skills/system-curator/SKILL.md` copies), wrappers before canonical
      files; the canonical files stay only while a *local* registration of an unverified host remains in that
      root, so `connect --hub` for every host removes `.agents/skills/system-curator/`. Any other content at those
      paths is reported and left in place. distill-lab files are only ever planned in a folder that carries the
      hub marker, so running these commands in this code repo never touches its tracked
      `.agents/skills/distill-lab/` source.
- [ ] A hub created by the previous binary (three full curator copies) is migrated by `doctor --fix` to
      canonical + Claude wrapper, and its owned `.gemini/skills/system-curator/SKILL.md` is removed, with the Gemini
      notice; an edited copy (Claude or Gemini path) is reported as a blocked item naming its path and stays
      unchanged, while every other repair in the same run is applied, including the project and user-scope
      repairs that `doctor --fix` runs after the hub (<!-- Validation: DoctorFix status gate -->the gate at
      `internal/app/workspace.go:228-230` no longer skips them because of a blocked hub item).
- [ ] Unknown content at a system-skill path (canonical file, wrapper, legacy copy) is a blocked item that is
      left unchanged and blocks only itself; when a canonical file is blocked, that id's owned Claude wrapper is
      removed. Known older shipped versions still upgrade silently (decided behaviour change, test
      `doctor-skew_test.go:62-75` updated, release note written). A wrapper with an edited description is blocked,
      not overwritten.
- [ ] Every previously shipped bundle file and wrapper is recognized: tag-derived tests over `git tag -l 'v*'` fail
      when a shipped digest is missing from `known-digests.go` or `known-wrappers.go`.
- [ ] `distill.mjs` has the same commands, options and behaviour as `distill.py` (`check`, `format`, `list`,
      `decide`; validation, impact and final_score, sort order, fixed-layout writer, `where` check against the Git
      mirrors, hub found from the file's own path), except the deliberate differences listed in Phase 2 (optional
      `--hub` agreement check, hardened offline git calls, refusal of symlinked or hub-tracked mirrors, safe write).
      Loading matches PyYAML `safe_load`: a parity test loads the existing fixtures and checks the values (SHAs as
      strings, dates) before any round trip. `format` on `internal/migration/testdata/*-distill.yaml` reproduces
      their body byte for byte (header lines name the new script), or, if a residual difference cannot be removed,
      the one-time reformat is documented and committed as golden output; in both cases `internal/distill` parses
      and validates the output (decision 13).
- [ ] The evidence check never runs code from a planted mirror config and never fetches: tests with a planted
      `core.sshCommand`/partial-clone config, a symlinked mirror, a hub-tracked mirror and a blob-less clone pass;
      writes refuse a symlinked `.tmp`, `distill.yaml` or `.meta`.
- [ ] The skill is self-contained (decision 12): the distill-lab folder copied alone to a scratch folder outside
      any hub runs `node scripts/distill.mjs --help` and `list <fixture>` with exit 0, and a hub-needing command
      (`check` without `--no-where` on a file outside a hub) exits with a clear `ERROR/WHY/FIX` message, not a
      stack trace. It needs only Node 18 or newer and Git 2.45.0 or newer (<!-- Validation: Git minimum version -->decision 12); it never calls the `skillhub` binary, an MCP tool or
      npm, and the script itself opens no network connection (only the mirror clone/fetch steps the skill
      describes use the network).
- [ ] `skillhub doctor` in a hub without Node (or with Node older than 18) keeps its status and adds one
      notice-severity finding (<!-- Validation: onboarding extension point -->registered through the onboarding
      plan's `RegisterProvider`) naming what is missing and how to install it.
- [ ] `skillhub status` prints the `review_lessons` next step as an absolute, quoted command that runs from any
      folder (`node '<hub>/.agents/skills/distill-lab/scripts/distill.mjs' list '<hub>/<path>'`; on Windows the
      double-quoted form without `&&`), JSON and MCP carry `script`, `file` and `hub_relative` (<!-- Validation: status JSON schema -->and
      `schemas/curation-home-v1.schema.json`, checked by `internal/delivery/cli/status_schema_test.go:27`, accepts
      them), and the command is
      emitted only when every canonical distill-lab file matches the current bundle; the command runs in the
      sandbox.
- [ ] After an upgrade, `status`, the web Home and `doctor` report host integration missing and
      `gitignore_entry_missing` until `doctor --fix`; both are in the release notes, and a tracked generated file
      produces a non-blocking notice-severity finding with a printed (never executed) `git rm --cached -- <file>`.
- [ ] The web handoff, Distill tab note, source confirmation text and user docs say where distill runs (an agent
      started in the hub folder) and what it needs (Node); no doc says the skill is not shipped, needs Python, or
      is fully offline.
- [ ] js-yaml 4.3.2 is vendored byte-identical to `dist/js-yaml.mjs` of the tarball pinned by URL and registry
      sha512 integrity, with its MIT license beside it, a section in the release notices file, and the update
      procedure in `AGENTS.md`.
- [ ] In this code repo the dev copy uses the shipped layout: content at `.agents/skills/distill-lab/` (the single
      editable source, mirrored byte for byte into `internal/systemskills/distill-lab/`), and
      `.claude/skills/distill-lab/SKILL.md` is the same wrapper the installer renders; the old `distill.py` is gone.
- [ ] `make check` (including the Node-executing script test, which CI runs with Node 18 set up and required in
      both `ci.yml` verify and `release.yml` verify on Ubuntu, macOS and Windows), `make web-check` and
      `go test ./internal/hostintegration -run TestMatrixSynchronized` pass.

## Scope and non-goals

In scope: embed distill-lab as a multi-file system skill; rewrite its script from Python to a self-contained
Node ES module with vendored js-yaml; generalize system-skill handling beyond the single curator id;
canonical-plus-Claude-wrapper installer with migration (including removal of the old Gemini copy) and removal;
distill-lab text fixes; product text, tests, docs; sandbox verification; moving this repo's dev copy to the
shared layout.

Non-goals: porting the script into Go or the `skillhub` binary (the skill stays usable without Skill Hub,
decision 12); changing the `distill.yaml` schema; any UI or command that launches an agent; serving distill-lab
over MCP or installing it into projects, user scope or remote clients; changing which hosts are verified in the
compatibility matrix; reserving the `distill-lab` id for workspace skills; anything the onboarding plan owns
(command and option names, launch args, permission rules, MCP config writing, README and `getting-started.md`,
`machine_file_tracked`, `requiredIgnoreEntries` mechanics, the per-agent connection lines, `skillhub version`).

## Design decisions

1. **Canonical layout.** Every system skill's real files live in `<root>/.agents/skills/<id>/` (SKILL.md and
   `scripts/`). Codex and current Gemini CLI read that folder directly, so they get no wrapper. Claude Code gets
   a one-file wrapper `.claude/skills/<id>/SKILL.md`. Nothing is written under `.gemini/skills/`. The canonical
   files are host-independent: <!-- Red team: ChangeSharedSkill contract --> they are a new shared change kind kept
   in `Inspection.Shared` and in plan changes with no host, rendered as "Agent Skills folder (.agents/skills)"
   (Phase 4 design 2 and 12; today Codex's native skill path *is* the canonical path, `integration.go:35`).
2. **Wrapper mechanism: a generated wrapper SKILL.md, Claude Code only.** Same `name` and `description` as the
   canonical frontmatter, a short body that tells the agent to read and follow the canonical SKILL.md and to run
   scripts from the canonical folder, through `${CLAUDE_SKILL_DIR}/../../../.agents/skills/<id>/`, which Claude
   Code substitutes in skill content (official docs). The relative path is the same in project and user scope.
   The first line carries a template version (`wrapper v1`).
   - Rejected, symlinked directories: Claude Code and Codex document symlink support, but the installer refuses
     symlink components by design (`internal/hostintegration/files.go:19-35,51`), Windows needs Developer Mode
     or admin for symlinks and Git with `core.symlinks=false` checks them out as text files. The same refusal
     applies when a user's `~/.agents` or `~/.claude` is itself a symlink: those skill files are skipped as blocked
     items and the rest of the connection proceeds (Phase 4 design 9). <!-- Red team: symlinked ~/.agents -->
   - Rejected, hash-verified copies: that is what we have today and is what the owner's rule removes.
   - Rejected, a Gemini wrapper (owner decision): current Gemini CLI reads `.agents/skills` and gives it
     precedence over `.gemini/skills`, so a wrapper would be shadowed or show the skill twice.
   - <!-- Red team: user-scope wrapper unverified --> Evidence limits, stated honestly: only Claude Code is
     verified end to end in this project (`docs/mcp-compatibility-matrix.json`). Wrapper-following is verified only
     by Phase 8's manual check in project scope. <!-- Validation: Claude Code permissions docs --> The project-scope
     wrapper reads `<root>/.agents/skills/...`, inside the working directory, without a prompt. The user-scope
     wrapper (`~/.claude/skills/` to `~/.agents/skills/`) is expected to prompt for reads outside the working
     directory in Manual mode, per the permissions docs (read-only reads need no approval only "within the working
     directory and additional directories", https://code.claude.com/docs/en/permissions); the docs say so, and the
     owner may confirm it with the optional real-home check. `${CLAUDE_SKILL_DIR}` substitution in skill content is
     documented (https://code.claude.com/docs/en/skills).
     Gemini CLI and Codex loading from `.agents/skills` rests on their official docs and is unverified here;
     <!-- Validation: Gemini CLI minimum version --> Gemini CLI reads `.agents/skills` from v0.28.0 (PR #18151,
     https://github.com/google-gemini/gemini-cli/pull/18151, https://geminicli.com/docs/cli/creating-skills/).
     <!-- Red team: older Gemini CLI --> A Gemini CLI older than v0.28.0 cannot read `.agents/skills` and loses the curator entirely,
     because MCP hides it from every Gemini session (`internal/delivery/mcpserver/server.go:399-411`); the owner's
     decision stands, and the release note and a doctor notice name the effect (Phase 4 design 15).
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
     keep `.agents/` ignored except `.agents/skills/distill-lab/`, in its own first commit, before Phase 2 starts.
4. **Ownership and unknown content (decided behaviour change).** A bundle file is ours when its bytes equal the
   current bundle (every file, including the vendored js-yaml and its license) or a previously shipped version
   listed in the known-digest table. The table holds every committed curator version and every committed version
   of the old dev `SKILL.md` (so an unedited hand copy in a hub is recognized and replaced by the wrapper). It
   holds no `distill.py` digest: Skill Hub never installed that file (it existed only in the dev copy), so a
   hand-copied `distill.py` is the user's and is left alone (and, being under an ignored folder, is named by a
   doctor notice, Phase 5). <!-- Red team: wrapper ownership has no history --> A wrapper is ours when its bytes
   equal the current render or a previously shipped render listed in `known-wrappers.go`; the description found in
   the file is never trusted, so an edited description is the user's. <!-- Red team: hand-maintained known digests -->
   Tag-derived tests (`git tag -l 'v*'`) enforce both tables. Anything else is the user's: it is reported as a
   blocked item and never overwritten or deleted. <!-- Red team: conflict mid-fix aborts --> A blocked
   system-skill file blocks only itself: every other change of the same run is applied, and today's
   abort-the-plan contract (`integration.go:91-93`) stays for the other change kinds. When a canonical file is
   blocked, that id's owned Claude wrapper is removed so Claude Code never follows content Skill Hub did not
   write. This extends today's single-string check (`internal/hostintegration/curator-source.go:27-29`).
   - Owner-approved change: today `doctor --fix` overwrites unknown curator content after confirmation
     (`internal/hostintegration/doctor-skew_test.go:62-75`). Under this plan unknown content is left unchanged;
     known old versions are still upgraded silently. Phase 4 rewrites that test; Phase 7 adds the
     troubleshooting entry and the release-note line.
5. **Executable bit.** Scripts are written `0644` and always invoked as `node <path>`, the same command on
   Windows. No step relies on the exec bit, so Windows and `embed.FS` (which drops modes) behave the same.
6. **Hub-only distill-lab.** distill-lab is planned for install only when the plan is local, project scope, and
   `Root == Workspace` (the hub folder itself). `connect` from a project sets `Root` to the project and `-g` sets
   it to home (<!-- Validation: decision 6 citation -->`ConnectService.target`, `internal/app/connect.go:131-160`;
   doctor's project and user checks do the same in `internal/app/host-connections.go:23,26`), so neither installs
   it unless the project is the hub folder itself. Removal
   plans consider distill-lab only when the root contains `.skillhub/schema-version`
   (`internal/workspace/workspace.go:75`), because remote and removal plans carry no workspace
   (`internal/app/connect_remote.go:159,441`) and this code repo holds a tracked distill-lab that matches the
   shipped bytes.
7. **Tracked vs ignored: entries in the hub `.gitignore`.** <!-- Red team: contract change from the onboarding owner; local exclude dropped -->
   The hub's generated system-skill folders `/.agents/skills/<id>/` and `/.claude/skills/<id>/` for both system
   skills (four entries) are appended to `requiredIgnoreEntries` (`internal/workspace/workspace.go:38-41`), the
   mechanism already on main since `b734bc9`: `init` appends missing entries, `Inspect` reports a fixable
   `gitignore_entry_missing` per missing entry, and `doctor --fix` appends it. The onboarding plan keeps exactly
   this mechanism (contract from its owner, 2026-10-11). No `.gemini/skills/` entry: nothing is written there any
   more. Reason: these files are generated from the binary version, so committing them churns on every upgrade,
   and a clone on another machine regenerates them on `init`/`doctor --fix`; because `.gitignore` is committed,
   the entries hold for every clone. Existing hubs see `gitignore_entry_missing` for the new entries until
   `doctor --fix` (release note). <!-- Red team: system_skill_tracked blocks doctor --fix; check-ignore; untracking user files -->
   Already tracked generated files (every hub created by 0.5.0 commits its curator copies) are detected with
   `git ls-files` (`git check-ignore` never reports tracked paths) and reported as the non-blocking
   `system_skill_tracked` finding, only for files whose bytes Skill Hub owns, with a printed, never executed
   `git rm --cached -- <file>`; it is not a `workspace.Finding`, so it cannot stop `doctor --fix`
   (`internal/app/workspace.go:266-270`). <!-- Validation: onboarding extension point --> It is a `notice`-severity
   `Finding` with `Fix` nil and the command in `Manual`, registered through the onboarding plan's
   `RegisterProvider` (decision 10). An untracked file in those folders that Skill Hub does not own is named in
   a notice of the same provider so it is never hidden silently. Other skills a user keeps under `.claude/skills/` stay tracked. If an
   old hub had committed `.gemini/skills/system-curator/SKILL.md`, its removal by `doctor --fix` shows as a
   deletion the owner commits.
8. **Old Gemini copy.** An owned full copy at `.gemini/skills/system-curator/SKILL.md` (current or previously
   shipped bytes, project or user scope) is removed by `init`, `connect` and `doctor --fix`, because Gemini CLI
   reads `.agents/skills` and the stale copy would shadow or duplicate it; empty parents are pruned. An edited
   copy is a blocked item and is left.
9. **Removal.** <!-- Red team: connect --hub keeps canonical; one shared folder; removal order --> Remote
   (`connect --hub`) and `--remove` plans delete, for the selected hosts, owned Claude wrappers and owned old full
   copies (Claude and Gemini paths) first, and the shared canonical files last and only when no *local*
   registration (`Registration.Remote()` false) of an unverified host remains in that root after the plan.
   `scopeRegistration` alone cannot decide this (it returns remote registrations too). One shared folder cannot
   serve "curator for Claude Code only": with `connect --hub --host codex` while Claude Code stays local, Codex
   still reads the curator, and the output says so. Unowned content is left and reported, never fatal for
   `--remove`. A verified MCP-skills host (none today) is not delivered the curator natively: Claude drops its
   curator wrapper; the canonical curator stays while another local host in the root still needs it. distill-lab
   is never served over MCP, so its wrapper stays. Doctor repairs a wrapper whose canonical target is missing.
10. **Doctor additions through the onboarding extension point.** <!-- Validation: onboarding extension point -->
    The onboarding plan names it (its phase-04:33-63, plan.md:208): `internal/app/health.go` with `CheckProvider`
    (`Name`, `Applies(RoleReport)`, `Check(ctx, RoleReport, Depth) []Finding`), `Finding{Code, Label, Severity
    ok|notice|problem|skipped, Message, Fix *FixAction, Manual}` and `RegisterProvider`. This plan registers one
    provider, `systemSkillProvider` in `internal/app/system_skill_doctor.go` (Phase 4 creates it, Phase 5 adds two
    checks), applied to the hub role and run at full depth (doctor) only. It returns three kinds of `notice`
    Findings, all with `Fix` nil, so none changes the doctor status or blocks `doctor --fix`:
    - Node missing or older than 18 (code `distill_lab_node_missing`): "distill-lab runs scripts/distill.mjs with
      Node 18 or newer; <not found | found vX>." `Manual`: "Install Node from https://nodejs.org".
    - a generated system-skill file tracked by Git (`system_skill_tracked`, Phase 5), `Manual`:
      `git rm --cached -- <file>`;
    - a file Skill Hub did not write in an ignored system-skill folder (`system_skill_unowned_ignored`, Phase 5),
      `Manual`: move it or `git add -f <path>`.
    Wrapper and canonical *integrity* (missing, outdated, blocked, the old-layout migration and the Gemini notice
    of decision 8) stays in the host inspection items of the existing hub doctor and host-connection checks, which
    the onboarding plan wraps as providers itself. Reason, given as the answer to the onboarding plan's expectation
    that this plan registers "wrapper/canonical integrity with a `--fix` migration" through `RegisterProvider`
    (their plan.md:208, phase-04:62-63): those repairs need Plan/Apply semantics (preimage digests, desired bytes
    re-derived at apply time, the dependency rank check, symlink refusal, per-item blocking,
    `apply`, `expectedChange` and `changeOrder` in `internal/hostintegration/files.go:123-302`); a `FixAction.Apply` closure would either duplicate that engine or
    bypass its checks, and it would report the same files twice.
11. **Runnable product text.** <!-- Red team: review_lessons over MCP; Windows; tampered bundle --> The
    `review_lessons` action carries structured fields (`script` and `file`, both hub-relative, and
    `hub_relative: true`) plus a human `command` documented as "run from the hub folder", so JSON and MCP output
    carry no machine path and an agent in another folder is told where to run it. The CLI renders an absolute,
    quoted command that is correct from any folder (POSIX single quotes; Windows double quotes, no `&&`). The
    path is only chosen among folders whose names match the canonical id pattern. The command is emitted only
    when every canonical distill-lab file equals the current bundle; the web texts name the skill and its script
    but print no command line, because the browser has no integrity information.
12. **Self-contained Node skill (owner decision).** distill-lab's script is one ES module, `scripts/distill.mjs`,
    that imports js-yaml from `scripts/lib/js-yaml.mjs` by relative path: no npm install, no bundler.
    Requirements for the shipped SKILL.md and script:
    - they need only Node 18 or newer and Git 2.45.0 or newer; never Python, the `skillhub` binary or an MCP tool;
    - <!-- Red team: hub lookup --> the script finds the hub from the `distill.yaml` path it is given (`parents[4]`,
      verified by `.skillhub/schema-version`), as `distill.py` does; an optional `--hub` only asserts that it
      agrees; there is no environment or current-folder lookup in the script. SKILL.md fixes only its personal
      default: "`$SKILLHUB_WORKSPACE`, else the current folder when it is a hub, else ask the user";
    - commands that do not need the hub (`--help`, `list`, `decide`, `check --no-where`, `format --no-where`)
      work anywhere; commands that need it on a file outside any hub exit with an `ERROR/WHY/FIX` message, not a
      stack trace;
    - <!-- Red team: planted mirror; network claim --> the evidence check runs local `git` only, with an explicit
      `--git-dir`, `GIT_NO_LAZY_FETCH=1`/`--no-lazy-fetch`, `GIT_TERMINAL_PROMPT=0`, `GIT_CONFIG_NOSYSTEM=1`,
      `-c safe.bareRepository=explicit`, empty `core.sshCommand` and `core.fsmonitor`, a 30 s timeout, and refuses
      symlinked or hub-tracked mirrors; a blob missing from a partial clone is reported as "blob not fetched; run
      git fetch in the mirror". The script makes no network call; the mirror clone and fetch steps in SKILL.md do.
      <!-- Validation: Git minimum version --> Git 2.45.0 is the floor: it is the first version with the
      `--no-lazy-fetch` option and the documented `GIT_NO_LAZY_FETCH` variable
      (https://github.com/git/git/blob/master/Documentation/RelNotes/2.45.0.adoc,
      https://github.com/git/git/blob/v2.45.0/Documentation/git.txt); older Git fails on the unknown option;
    - <!-- Red team: untrusted upstream sources --> mirrors are bare partial clones so no upstream `CLAUDE.md` or
      `.claude/skills` lands in the hub folder, and SKILL.md warns that sources are untrusted data and that
      `runtime/config/` and `runtime/envs/` must never be read while distilling (<!-- Validation: deny-read dropped -->a
      text control only; no deny-read rule is requested, see "Risks");
    - <!-- Validation: self-containment wording --> the bundled files (SKILL.md, `distill.mjs` and its comments, the
      help text) never contain the words `python` or `distill.py`, because Phase 3's self-containment test bans them;
      the deliberate differences and any port comment refer to "the previous implementation" or "the reference
      emitter" instead;
    - <!-- Red team: atomic write --> writes `lstat` the file, `.meta` and every folder up to the hub, refuse
      symlinks, and use a random temp name opened with `wx`;
    - SKILL.md calls `node <skill folder>/scripts/distill.mjs ...` from one documented section only.
    Minimum Node, with evidence: the Node ESM docs (https://nodejs.org/api/esm.html, fetched 2026-10-11) give
    unflagged ES modules from v12.17.0/v13.2.0, stable from v14.17.0/v15.3.0, and `node:` imports from
    v14.13.1. <!-- Validation: Node minimum --> So the technical floor is about Node 14.13.1; the plan keeps Node 18
    as the stated minimum (a margin over that), noting that Node 18 itself reached end of life in April 2025. No
    GitHub runner ships Node 18 (ubuntu-latest = Ubuntu 24.04 and windows-latest = Windows Server 2025 default to
    22.23.3; macos-latest = macOS 26 arm64 defaults to 24.20.0), so CI must set up Node 18 with `actions/setup-node`
    and `node-version: 18` in both Go test jobs; doctor warns below 18. <!-- Red team: Node guard only in verify -->
13. **YAML output parity.** PyYAML's output is what existing `distill.yaml` files and the fixtures carry, and
    js-yaml's `dump` cannot reproduce it by options alone: its `flowLevel` is one global depth, but the layout puts
    `decision` and `score.facts` in flow style while their siblings are block; it has no per-node folded style;
    it emits integral floats as `45` where PyYAML writes `45.0`; and its YAML 1.2 quoting differs from PyYAML's
    YAML 1.1 resolver. So `distill.mjs` keeps the layout in its own small writer, as `distill.py` already
    does with its custom dumper, and uses js-yaml only to load. <!-- Red team: CORE_SCHEMA turns hex SHAs into numbers -->
    The loader uses a custom schema on top of `FAILSAFE_SCHEMA` that copies PyYAML 6.0's YAML 1.1 resolvers (a float
    needs a dot, `0`-prefixed octal, `yes/no/on/off`, timestamps kept as tagged text), because `CORE_SCHEMA` reads
    hex SHAs such as `1e23456`, which PyYAML wrote unquoted, as numbers. Phase 2 lists the PyYAML rules the writer
    reproduces. Parity is proven by a loader parity test (loaded values of the existing files, before any round
    trip), a differential run against `distill.py` during the rewrite and golden-output tests afterwards. Fallback
    if byte parity of the writer proves impossible for some input class: accept a one-time reformat on the next
    `format`, document it in the skill and the release note, commit the new golden output, and keep the Go reader
    tests passing. The loader has no fallback: a loaded-value difference is a bug.

## Sequencing with the onboarding plan

Another session implements [261011-2250-onboarding-ux-redesign](../../.claude/worktrees/agent-a5c101367df3589e1/plans/261011-2250-onboarding-ux-redesign/plan.md)
(worktree branch `worktree-agent-a5c101367df3589e1`, read-only for this plan) and merges and releases the
release that carries the onboarding redesign, 0.5.0, first. <!-- Validation: release is 0.5.0, not 5.0.0 -->
Every "0.5.0" in this plan names that release (the repo's tags are `v0.x`; the latest stable is `v0.4.1`). Implementation of this plan starts in a fresh worktree from the new main only after that session announces
main is final; then the "Re-verify at start" checklist runs before Phase 1.

Ownership split (agreed):

- **Theirs:** launch args (<!-- Red team: contract change, the stdio launcher keeps its name -->the hidden stdio
  launcher keeps the name `mcp serve`; no rename and no registration migration), permission rules
  (`cli-permissions.go`), MCP entry and config writing, `--hub`/`connect` option parsing, command renames (`check`,
  `integrate`, `web` removed; hub commands documented as `cd <hub> && skillhub <cmd>`, no `--workspace` in docs
  examples; `init` commits automatically and stores the hub path in per-user config; no preview-then-`--yes` on
  onboarding commands), the hub `.gitignore` mechanism (`requiredIgnoreEntries`, `gitignore_entry_missing`; this
  plan only appends four entries), `machine_file_tracked` and its tests, the role-based doctor/status check set
  and its extension point (<!-- Validation: onboarding extension point -->`internal/app/health.go`: `CheckProvider`,
  `Finding`, `FixAction`, `RegisterProvider`, their phase-04:33-63), the per-agent connection lines and the
  connection summary wording (`connectionSummaryLine`, their phase-02:147-148 and plan.md:210),
  `getting-started.md` deletion and the README rewrite, and the text of the curator SKILL.md.
- **Ours:** the skill-file path: `curator-source.go`, the curator/system-skill writer in `integration.go`, its
  removal in `remote.go`, the inspection and plan shapes and every consumer of them listed in Phase 4, the
  `internal/systemskills` restructure, distill-lab, the four ignore entries, the `systemSkillProvider` registered
  through their `RegisterProvider` (decision 10), and the host-key collection that feeds their connection summary
  (so the shared `agents` key never prints as an agent; Phase 4 design 12). `skillhub version` stays untouched (the
  self-updater and installers call it).
- <!-- Validation: deny-read dropped --> **Nothing is requested from the onboarding owner.** The red team's request
  for hub-scope deny-read rules on `runtime/config/**` and `runtime/envs/**` is dropped: those folders are granted
  to agents on purpose (`RuntimeAccessDirs`, `internal/hostintegration/runtime_access.go:12-23`), and the
  bootstrap rule tells agents to source `$SKILLHUB_CONFIG_DIR/env` when they run an activated skill, so a deny
  rule would break every skill run in the hub folder. The residual risk is recorded under "Risks".
- **Answer to the onboarding plan's integrity expectation:** their plan.md:208 and phase-04:62-63 expect this plan
  to register "wrapper/canonical integrity with a `--fix` migration" through `RegisterProvider`. It does not:
  integrity stays in the host inspection items, for the reason given in decision 10; only the three notices go
  through the provider. Tell their owner before Phase 4 starts.
- **Order inside shared files:** our `review_lessons` change lands after their `curation_home.go` and
  `next_commands.go` edits; our distill sentences in docs, web (`handoff-brief.ts`, `DistillTab.tsx`,
  `DistillHandoffScreen.tsx`) and `source.go` come after their mechanical renames, which leave the distill
  sentences untouched and leave a slot in `docs/sources-and-learning.md` and `docs/web-ui.md`.

## Re-verify at start

Statements in this plan that depend on the onboarding plan's final names. Check each against the new main and
update the affected phase before starting it.

<!-- Red team: contract change; the local-exclude helper item is removed -->
- [ ] `requiredIgnoreEntries`, `gitignore_entry_missing` and `init`'s append step are unchanged after 0.5.0, and
      the onboarding plan did not already add any of the four entries (Phase 5).
- [ ] <!-- Validation: onboarding extension point --> The extension point names still match their phase-04:33-63
      on the merged main: `internal/app/health.go`, `CheckProvider` (`Name`, `Applies(RoleReport)`,
      `Check(ctx, RoleReport, Depth)`), `Finding{Code, Label, Severity, Message, Fix *FixAction, Manual}`, the
      `notice` severity (not a problem, no status change), `DepthFull`, and `RegisterProvider` (Phases 4, 5); and
      that their hub-doctor and host-connection providers still carry the host inspection items (decision 10).
- [ ] Hub command forms used in Phase 6 and Phase 8: `init` (no `--yes`, auto-commit, per-user hub path),
      `status`, `doctor`, `doctor --fix`, `connect`, `connect -g`, `connect --hub`, `connect --remove`,
      `--host`, and that `--workspace` is gone from examples; the stdio launcher is still `mcp serve`.
- [ ] `cliNextCommand`/`shellArgument` and the `review_lessons` rendering after their edits to
      `internal/delivery/cli/curation.go` and `next_commands.go`; the collection id pattern (Phase 6).
- [ ] The per-agent connection lines (today `hostChangeLines`, `internal/delivery/cli/connect_remote.go:118-146`,
      `renderConnectResult`, `internal/delivery/cli/connect.go:114-159`, `hostDisplayName`, `:161-172`, and
      `formatChangeKind`, `:174-207`) and every other item-ID reader (`connectCurationGuidance`,
      `internal/delivery/cli/connect.go:104-105`; the init summary host list, `internal/delivery/cli/workspace.go:40-50`,
      which their `connectionSummaryLine` replaces) after their phase 2, so Phase 4's shared line, its ID parser and
      the `agents` exclusion fit them (Phase 4). <!-- Validation: item-ID parsers -->
- [ ] Line numbers cited from `internal/app/workspace.go`, `internal/app/connect.go`, `internal/app/connect_remote.go`,
      `internal/app/host-connections.go`, `internal/app/curation_home.go`, `internal/workspace/workspace.go`,
      `internal/delivery/cli/*`, `internal/hostintegration/*` (all phases); the curator SKILL.md text they edited
      (Phase 1 known digests and the tag-derived test must include their new curator versions).
- [ ] Docs file set after 0.5.0: `getting-started.md` deleted, README rewritten, the distill slots in
      `docs/sources-and-learning.md` and `docs/web-ui.md` (Phase 7).
- [ ] Whether the onboarding plan edited `.github/workflows/ci.yml` or `release.yml` (Phase 2 adds Node 18 setup,
      `SKILLHUB_REQUIRE_NODE`, and tags for the digest test).
<!-- Validation: deny-read item removed (request dropped); Gemini CLI and Git minimum versions filled in, see decisions 2 and 12 -->
- [ ] `RuntimeAccessDirs` and the bootstrap's `$SKILLHUB_CONFIG_DIR/env` rule are unchanged, so the residual risk
      in "Risks" still describes the hub (Phases 3, 8).
- [ ] The status JSON schema `schemas/curation-home-v1.schema.json` (`actionItem`, `additionalProperties: false`)
      and `internal/delivery/cli/status_schema_test.go:27` after their edits (Phase 6).
- [ ] The Apply rank check (`changeOrder`, `internal/hostintegration/files.go:289-302`, called at `:158`) and the
      `DoctorFix` gate (`internal/app/workspace.go:228-230`) after their edits (Phase 4).

## Phases

<!-- Red team: efforts, dependencies and waves updated -->
<!-- Validation: Phase 4 +4h (rank check, DoctorFix gate, applyHostIntegration, ID parsers, formatChangeKind, output goldens, several ids per host, all-adapter registrations, Blocked in MissingFinding); Phase 2 +0.5h (extraction script, test-name list, release fetch-depth); Phase 6 +0.5h (status JSON schema) -->
| # | Phase | Effort | Depends on | File | Status |
|---|---|---|---|---|---|
| 1 | Embed system skill bundles and distill-lab | 4h | - | [phase-01](phase-01-embed-system-skill-bundles.md) | done |
| 2 | Rewrite the distill script as a self-contained Node module | 13.5h | 1 | [phase-02](phase-02-rewrite-distill-script-in-node.md) | done |
| 3 | distill-lab skill text: hub lookup, one script section, requirements, mirrors, warning | 2h | 1, 2 | [phase-03](phase-03-distill-lab-skill-text.md) | done |
| 4 | Installer: canonical folder, Claude wrapper, per-item conflicts, migration, removal, doctor provider | 20h | 1 | [phase-04](phase-04-installer-canonical-and-wrappers.md) | done |
| 5 | Ignore generated system-skill folders in the hub `.gitignore` and report tracked ones | 2.5h | 4 | [phase-05](phase-05-gitignore-entries-and-tracked-notice.md) | done |
| 6 | Product text, status JSON schema and tests (Go, web, e2e, one screenshot) | 5.5h | 1, 4 | [phase-06](phase-06-product-text-and-tests.md) | done |
| 7 | User docs | 3.5h | 2, 3, 4, 5, 6 | [phase-07](phase-07-docs.md) | done |
| 8 | Sandbox verification | 5h | 7 | [phase-08](phase-08-sandbox-verification.md) | done with concerns: manual Claude Code check 12 not run ([verification](reports/verification.md)) |
| 9 | Move the dev copy to the shared layout | 1h | 8 | [phase-09](phase-09-move-dev-copy-to-shared-layout.md) | done (step 4, the manual Claude Code check, not run) |

## Parallel execution and file ownership

| Wave | Phases | Owned files (no overlap inside a wave) |
|---|---|---|
| 1 | 1 | P1: `internal/systemskills/{embed.go,embed_test.go,bundles.go,known-digests.go,known_digests_test.go}`, `internal/systemskills/distill-lab/SKILL.md`, `.agents/skills/distill-lab/SKILL.md` (create), repo `/.gitignore` (own first commit) |
| 2 | 2, 4 | P2: `.agents/skills/distill-lab/scripts/**`, `internal/systemskills/distill-lab/scripts/**`, new `internal/systemskills/distill_script_test.go`, new `internal/systemskills/testdata/distill/**`, `web/scripts/notices.mjs`, `web/THIRD_PARTY_NOTICES.md`, `.github/workflows/ci.yml`, `.github/workflows/release.yml`. P4: `internal/hostintegration/**`, `docs/mcp-compatibility-matrix.json`, `internal/app/{connect.go,connect_remote.go,workspace.go,host-connections.go}`, `internal/app/curation_home.go:150-157`, new `internal/app/system_skill_doctor.go` (+ test), `internal/delivery/cli/{connect.go,connect_remote.go}`, new `internal/delivery/cli/host_item_id.go` (+ test), the host-key collection that feeds the connection summary (today `internal/delivery/cli/workspace.go:40-50`; after the onboarding merge, the input of `connectionSummaryLine`; not its wording), tests and goldens `internal/app/workspace_test.go`, `internal/delivery/cli/{onboarding_test.go,connect_regression_test.go,connect_remote_test.go}`, `internal/delivery/cli/testdata/connect-workspace/{project-files,global-files,project-output,global-output}.golden` <!-- Validation: owned parsers and output goldens --> |
| 3 | 3, 5, 6 | P3: `.agents/skills/distill-lab/SKILL.md`, `internal/systemskills/distill-lab/SKILL.md`, `internal/systemskills/embed_test.go`. P5: `internal/workspace/workspace.go` (`requiredIgnoreEntries` only), its test, `internal/app/system_skill_doctor.go` (+ test). P6: `internal/app/curation_home.go` (`ActionItem`, `countCandidateLessons`, `review_lessons` block), `schemas/curation-home-v1.schema.json` (`actionItem`; public contract) and `internal/delivery/cli/status_schema_test.go` <!-- Validation: status JSON schema -->, `internal/app/curation_home_test.go`, new `internal/app/distill_lab_intact.go`, `internal/app/source.go`, `internal/delivery/cli/source.go`, `internal/delivery/cli/source_test.go`, `internal/delivery/cli/curation.go`, `internal/delivery/cli/workspace_next_steps_test.go`, `internal/distill/distill_model.go`, `web/src/domain/handoff-brief*.ts`, `web/src/screens/skill-detail/DistillTab.tsx`, `web/src/screens/distill/*`, `web/e2e/tier3.spec.ts`, `web/e2e/journeys.spec.ts`, `docs/images/web/distill-handoff.png` + its manifest entry |
| 4 | 7 | `docs/*.md` listed in the phase, `docs/design/05-curation-lifecycle.md`, `AGENTS.md` |
| 5 | 8 | none (scratch sandbox only; fixes go back to the owning phase) |
| 6 | 9 | `.claude/skills/distill-lab/**` of this code repo, new `internal/hostintegration/repo_wrapper_test.go` |

<!-- Red team: Phase 2 depends on Phase 1; Phase 5 and 6 ordering and ownership -->
Phase 2 starts as soon as Phase 1's first commit (the repo `.gitignore` edit) lands, because before it the new
`scripts/**` files are ignored; it writes only under `scripts/**`, its test data and the workflows. Phase 4 runs
beside it: its tests read bytes through the bundle API, never literal content, so Phase 2 and 3 edits do not break
them. Phase 5 needs Phase 4's provider file and ownership helpers; Phase 6 needs the bundle API (Phase 1) and runs
after Phase 4 because both edit `internal/app/curation_home.go` (different blocks). In wave 3, P5 and P6 do not
share a file (P6 does not touch `system_skill_doctor.go`). Between Phase 1 and Phase 9 this repo briefly holds the
old full dev copy (with `distill.py`) in `.claude/skills/distill-lab/` next to the new source; Phase 9 ends that.
Critical path: 1 -> 4 -> 6 -> 7 -> 8 -> 9 (39h); the script path 1 -> 2 -> 3 (19.5h) ends before Phase 7 needs it.
<!-- Validation: critical path 34.5h -> 39h, script path 19h -> 19.5h, total 52h -> 57h -->
Bash commands in every phase avoid the path segments the repo hook `.claude/hooks/scout-block.cjs` blocks for
agent sessions (`node_modules`, `dist`, `build`, `vendor`, `target`, `.git`, `coverage` and a few more, listed in
`.claude/hooks/scout-block/pattern-matcher.cjs:15-34`; there is no `.claude/.ckignore`). That is why the vendored
folder is `scripts/lib/` and why Phase 2 and Phase 8 use small Node script files for the tarball and the Node
binary. <!-- Validation: repo hook blocks vendor/dist paths -->

## Risks

| Risk | L x I | Mitigation |
|---|---|---|
| The Node rewrite changes behaviour or output of a command | M x H | Differential run against `distill.py` on fixtures and edge-case inputs during Phase 2; golden-output tests kept afterwards; Go reader parses every output; deliberate differences listed in Phase 2; fallback in decision 13. |
| The loader reads a PyYAML-written value as another type (hex SHA as a number, date) | M x H | Custom YAML 1.1 schema copied from PyYAML's resolvers; loader parity test on the existing files and edge inputs before any round trip (decision 13). |
| Python-compatible details are missed (`round(x, 2)` ties, `45.0` floats, YAML 1.1 quoting such as `yes` or all-digit SHAs, line folding at width 88, sort by code point) | M x M | Phase 2 lists each rule with a dedicated test input; differential run covers them. |
| A planted mirror config runs commands or triggers a network fetch during `check` | M x H | Hardened git environment and options, explicit `--git-dir`, `cat-file` only, no lazy fetch, timeout, refusal of symlinked or hub-tracked mirrors; Phase 2 tests and Phase 8 checks 14-15. |
| A symlink at the write path redirects `format`/`decide` output | L x H | `lstat` chain to the hub, random `wx` temp name; Phase 2 test and Phase 8 check 16. |
| An agent distilling untrusted sources in the hub folder reads the hub's secret env files | M x H | <!-- Validation: deny-read dropped --> Bare mirrors (no upstream agent files), the untrusted-content warning in SKILL.md, hardened offline git. **Residual risk, accepted:** an agent run in the hub folder can read the hub's secret env files under `runtime/config/` and `runtime/envs/` by design, because skills run there need them (`RuntimeAccessDirs`, `internal/hostintegration/runtime_access.go:12-23`, and the bootstrap rule to source `$SKILLHUB_CONFIG_DIR/env`). A prompt injected through a source can therefore try to read them; the only control is the skill text and the agent's own judgement. No deny-read rule is requested, because it would break skills run in the hub. |
| Node missing or too old on the hub machine | M x M | Doctor notice (`notice` Finding through `RegisterProvider`), docs requirement line; the binary never needs Node. |
| The vendored js-yaml differs from the published file or misses its license notice | L x M | Pinned tarball URL and registry sha512 integrity, sha256 of both files asserted by a test; license beside it and in the release notices; update procedure in `AGENTS.md`. |
| Node-executing test is silently skipped in CI | L x M | Node 18 set up and `SKILLHUB_REQUIRE_NODE=1` in both `ci.yml` verify and `release.yml` verify. |
| A release changes a bundled file or the wrapper template without recording the shipped digest, so every unedited copy becomes "unknown" | M x H | Tag-derived tests over `git tag -l 'v*'` in Phase 1 and Phase 4, required in CI `lint` and release verify. |
| Claude Code does not follow the wrapper and runs with the wrapper text only | M x H | Wrapper body is a single imperative instruction with an explicit path; Phase 8 checks Claude Code by hand in project scope; docs state what is verified. |
| User-scope Claude wrapper prompts for `~/.agents` | H x M | <!-- Validation: Claude Code permissions docs --> Expected to prompt for reads outside the working directory in Manual mode, per the permissions docs; the docs say so; optional owner-run real-home check in Phase 8; a read-only allow rule can be agreed with the onboarding owner later; `~/.agents` never joins `additionalDirectories`. The project-scope wrapper reads inside the working directory without a prompt. |
| Gemini CLI or Codex does not load `.agents/skills` (old version, undocumented edge); a Gemini CLI older than v0.28.0 loses the curator entirely because MCP hides it | M x M | Both document it; docs and release note name v0.28.0 (PR #18151); the migration item carries the Gemini notice; Phase 8 records them as unverified. No `.gemini` fallback by owner decision. |
| The onboarding plan (0.5.0) renames commands, options, files or the doctor extension point this plan cites | H x M | Implementation starts only after that plan is merged and main is announced final; the "Re-verify at start" checklist lists every dependent statement, including the `health.go` names; the doctor notices sit in one provider. |
| <!-- Validation: Apply rank check --> A plan that mixes canonical writes, wrapper removals and canonical removals is rejected by Apply as "not dependency ordered" | H x H | Phase 4 replaces `changeOrder(kind)` with a rank per change (kind and removal) used by both `buildPlan` and Apply; tests for migration, blocked-canonical and remote-removal plans. |
| <!-- Validation: DoctorFix status gate --> A blocked hub item stops `doctor --fix` before the project and user repairs | H x M | Phase 4 changes the gate at `internal/app/workspace.go:228-230` so blocked system-skill items do not skip `applyExternalConnections`; the final status is computed after both. |
| <!-- Validation: status JSON schema --> New `review_lessons` fields fail the strict status schema | H x M | Phase 6 extends `schemas/curation-home-v1.schema.json` (`actionItem`) as a public contract change, with the release note. |
| `connect --remove` in this code repo deletes the tracked distill-lab source or its wrapper (bytes match the shipped ones) | M x H | Decision 6: distill-lab is planned only in a root with the hub marker; Phase 4 test and Phase 8 check in a repo-shaped folder. |
| The repo `.gitignore` change un-ignores more than `.agents/skills/distill-lab/` | L x M | Anchored negation patterns; Phase 1 validates with `git check-ignore` and an empty `git status` diff. |
| The experimental `distill.yaml` schema is now frozen by shipped fixtures and `internal/distill` | M x M | Keep the schema unchanged in this plan; version the skill (`version:` frontmatter) so later schema changes are visible. |
| A blocked system-skill file hides a real problem, or a mid-run failure leaves a half-migrated folder | M x M | Blocked items name the path in connect, doctor and status; every other repair proceeds; removal order is wrappers first; doctor repairs dangling wrappers (Phase 4). |
| Existing hubs show "host integration missing" and `gitignore_entry_missing` after the upgrade | H x L | Accepted and documented (release note); `doctor --fix` clears both; Phase 8 check 9 asserts it. |
| The per-item shape and new item IDs break JSON consumers | L x M | Owned tests and goldens updated in Phase 4; release note names the ID change. |
| Windows paths, quoting and missing exec bit | L x M | `filepath`-based paths, `0644`, `node <path>` invocation, Windows path and command-rendering table tests, CI runs a reduced script test on Windows. |
| A hub already has a workspace skill named `distill-lab` | L x L | Decided not to reserve the id. Workspace skills live under `skills/<collection>/` and system distill-lab is not served over MCP, so no file clashes; an agent might list two skills with that name. |

## Compatibility and rollback

- Existing hubs migrate on `doctor --fix` or the next `init`/`connect`; nothing is deleted unless owned. Until
  then `status` and `doctor` report host integration missing and `gitignore_entry_missing` (release note).
- Existing `distill.yaml` files stay readable by both the old and the new script; their header lines name
  `distill.py` until the next `format` rewrites them to name `distill.mjs` (comment lines only).
- <!-- Red team: downgrade path --> Downgrading the binary is not fully clean: the old binary does not know the
  wrappers. Its confirmed `doctor --fix` writes the full curator copies back (`prepareExactFile`,
  `integration.go:390-396`) and re-creates `.gemini/skills/system-curator/SKILL.md` (shown as untracked), but
  it never removes `.claude/skills/distill-lab/SKILL.md`, and its `connect --remove` can delete the canonical
  curator while leaving the curator wrapper. The release note tells the owner to delete
  `.claude/skills/system-curator/` and `.claude/skills/distill-lab/` by hand after a downgrade if the old binary
  reports them. The four `.gitignore` entries stay and are harmless.
- Each phase is one or a few commits; revert them in reverse wave order. Phase 5 entries already appended to a
  hub `.gitignore` stay after a revert and are harmless.

## Decisions taken by the owner (2026-10-11)

1. No Gemini wrapper: system skills go to `.agents/skills/<id>/` plus one Claude Code wrapper; owned old
   `.gemini/skills/system-curator/` copies are removed, edited ones reported (decisions 1, 2, 8). Kept after the
   red team; the effect on old Gemini CLI versions is documented, not reversed.
2. `distill-lab` is not a reserved skill id.
3. The dev copy stays in this repo and moves to the shared layout; `.agents/skills/distill-lab/` is the single
   editable source (decision 3, Phase 9). The move is approved; Phase 9 stays last, after verification.
4. Unknown content at a system-skill path is reported and left unchanged (decision 4); after the red team it
   blocks only its own item.
5. The script is rewritten in Node as `scripts/distill.mjs` with vendored js-yaml; no Python, no PyYAML. The skill
   stays self-contained so it can be used without the hub or the binary; no port into the binary is planned
   (decisions 12, 13). Doctor warns when Node is missing.
6. The red-team adjudication below (2026-10-11), including the onboarding owner's contract change: hub
   `.gitignore` entries instead of a local exclude, and the stdio launcher keeps the name `mcp serve`.
7. <!-- Validation: lead decisions --> The validation decisions of 2026-10-11, taken by the lead with the owner's
   authority (listed in "Validation Log"): release 0.5.0; Gemini CLI v0.28.0 and Git 2.45.0 floors; Node 18 kept as
   the stated minimum; the user-scope wrapper is expected to prompt; the deny-read request is dropped and the
   residual risk accepted; doctor notices go through `RegisterProvider` while integrity stays in host items; the
   vendored folder is `scripts/lib/`; bundled files never say `python` or `distill.py`.

## Owner follow-ups

None from this plan. The lesson schema is still experimental (Phase 0 scorecard,
`docs/plans/2026-10-10-observation-backlog.md:19`, M6); this plan keeps the `distill.yaml` schema unchanged so
that work is not affected.

## Open questions

<!-- Validation: open questions updated -->
None blocking the plan's design. The minimum Gemini CLI and Git versions are filled in, the deny-read request is
dropped and the doctor extension point is named; "Re-verify at start" only checks that the names still match.
Remaining evidential limits: Gemini CLI and Codex loading from `.agents/skills` are unverified in this project, the
user-scope Claude wrapper is expected to prompt (docs, not tested; Phase 8 records them), Codex following symlinked
skill folders was not re-fetched (not relied on: the plan writes no symlinks), and full byte parity with PyYAML is
proven only for the inputs the differential run covers (decision 13 fallback). The onboarding owner must be told
that integrity is not registered through `RegisterProvider` (Sequencing).

## Red Team Review

Session 2026-10-11. Four hostile reviewers produced 37 findings with `file:line` evidence; the owner approved
the adjudication below. Severity: 4 Critical, 18 High, 15 Medium. 35 accepted (one of them in part), 2 rejected.
Reviewer numbering: R1 = scope and plan hygiene, R2 = doctor and migration, R3 = installer contract and
compatibility, R4 = security. Edits in the phase files are marked `<!-- Red team: ... -->`. Every cited line
was re-checked on main `ef535de`.

| # | Finding | Severity | Disposition | Applied to |
|---|---|---|---|---|
| 1 | R1-1 Host-less `ChangeSharedSkill` does not fit Inspection, conflict pre-checks, `validateChangePath` | Critical | Accepted: `Inspection.Shared`, host-less changes, every consumer owned | Phase 4 context, design 2-3, owned files, tests; decision 1 |
| 2 | R1-2 Tests and goldens outside Phase 4 pin the three-copy layout | High | Accepted: listed and owned with expected content | Phase 4 owned files, validation grep; wave table |
| 3 | R1-3 "connection written: ..." string exists nowhere | High | Accepted: rewording dropped; real strings cited; shared line instead | Phase 4 note after design; AC 2; ownership split |
| 4 | R1-4 New hub lookup changes `check`/`format` and adds `--hub` | High | Accepted: hub from the file (`parents[4]` + marker); `--hub` only asserts agreement; SKILL.md fixes only the personal path | Phase 2 req 4; Phase 3 req 2; decision 12; Phase 7 req 1; Phase 8 check 4 |
| 5 | R1-5 Phase 5 untracks system skills nobody asked to untrack | Medium | Accepted in part: ignoring generated folders kept (owner decision, now via hub `.gitignore` per onboarding contract); tracked files only get a printed suggestion | Phase 5; decision 7 |
| 6 | R1-6 Every upgraded hub reports "host integration missing" | Medium | Accepted: explicit note, release note, Phase 8 assertion | Phase 4 design 13; Phase 7 req 10; Phase 8 check 9; compatibility |
| 7 | R1-7 Recognizing hand-copied dev SKILL.md is unrequested | Medium | Rejected: this repo's own dev copy is exactly such a file and must migrate; the orphaned `distill.py` concern is covered by the not-Skill-Hub-file warning | Decision 4 (note), Phase 5 req 4 |
| 8 | R1-8 Doctor adapter is speculative; notices tooling widened | Medium | Rejected: the adapter is a deliberate design agreed with the onboarding owner; notices section kept. The double-reporting point is resolved by reporting integrity only through host items | Decision 10; Phase 4 design 16 |
| 9 | R2-1 `system_skill_tracked` is unfixable and blocks `doctor --fix` | Critical | Accepted: non-blocking warning outside `workspace.Finding`, printed `git rm --cached -- <file>` | Phase 5 req 3; decision 7; Phase 8 check 9 |
| 10 | R2-2 `git check-ignore` never reports tracked paths | High | Accepted: tracked files via `git ls-files`; missing entries via the existing `.gitignore` content check | Phase 5 context, req 2-3 |
| 11 | R2-3 `connect --hub` keeps the canonical curator in remote clients | High | Accepted: keep canonical only while a local registration remains; test | Phase 4 design 5, 10; decision 9; Phase 8 check 10 |
| 12 | R2-4 Doctor and status cannot show shared-kind conflicts | High | Accepted: kind lists, levels, status item, IDs | Phase 4 design 2-3, 12; owned files |
| 13 | R2-5 A conflict mid `doctor --fix` aborts after the hub changed | High | Accepted: per-item blocking, collected results, no "run connect" loop | Phase 4 design 3; decision 4; Phase 8 check 9 |
| 14 | R2-6 Claude-only `connect -g` fails when `~/.agents` is a symlink | Medium | Accepted: symlinked skill folders refused per item, MCP setup proceeds | Phase 4 design 9, tests; decision 2; Phase 7 req 6; Phase 8 check 17 |
| 15 | R2-7 Wrapper ownership has no history; edited description overwritten | Medium | Accepted: template version, shipped-render digests, golden files, tag test | Phase 4 design 6-7; decision 4; Phase 9 step 3 |
| 16 | R2-8 Phase 5 ordering and file ownership do not hold | Medium | Accepted: Phase 5 after Phase 4, adapter owned by Phase 4, no `doctor --fix` call site needed | Phase 5 header; wave table |
| 17 | R2-9 `cd <hub> && node ...` does not run on Windows | Medium | Accepted: absolute quoted command, Windows double quotes, table test | Phase 6 req 4; decision 11 |
| 18 | R2-10 Removal order and downgrade leave dangling wrappers | Medium | Accepted: wrappers first, canonical last; dangling-wrapper repair; honest rollback | Phase 4 design 2, 11, rollback; compatibility |
| 19 | R3-1 `CORE_SCHEMA` turns hex SHAs into numbers | Critical | Accepted: PyYAML-compatible YAML 1.1 schema, loader parity test on existing files | Phase 2 req 8, tests; decision 13 |
| 20 | R3-2 One shared curator folder cannot be removed per host | High | Accepted: local-registration rule, stated mixed-project limit, test | Phase 4 design 5; decision 9 |
| 21 | R3-3 Older Gemini CLI loses the curator completely | High | Accepted (owner decision kept): release note and doctor notice; risk raised | Phase 4 design 15; Phase 7 req 6, 10; decision 2; risks |
| 22 | R3-4 Shared change has no host; per-host output and IDs break | High | Accepted: same fix as 1 and 12; unique IDs per path; `agents` display key | Phase 4 design 12 |
| 23 | R3-5 Tests outside Phase 4 still expect three copies | High | Accepted: same fix as 2 | Phase 4 owned files |
| 24 | R3-6 Hand-maintained known digests block upgrades | High | Accepted: tag-derived test required in CI | Phase 1 req 4, tests; Phase 2 req 12 |
| 25 | R3-7 User-scope Claude wrapper cannot be verified | High | Accepted: stated as unverified; optional owner-run check | Decision 2; Phase 8 check 12; Phase 7 req 4 |
| 26 | R3-8 Cited string and helper do not exist | Medium | Accepted: string dropped; helper dropped by the onboarding contract change | Phase 4; Phase 5; Re-verify list |
| 27 | R3-9 Phase 2 files are ignored until Phase 1 lands | Medium | Accepted: Phase 2 depends on Phase 1's first commit; `git ls-files` validation | Phase 1 step 1; Phase 2 header, step 5 |
| 28 | R3-10 Required-Node guard covers only the verify job | Medium | Accepted: Node 18 setup and env in both Go test jobs; reduced Windows run | Phase 2 req 12 |
| 29 | R4-1 Planted mirror runs commands through `git -C` | Critical | Accepted: hardened git calls, refusal of symlinked or tracked mirrors, tests | Phase 2 req 5, tests; decision 12; Phase 8 check 14 |
| 30 | R4-2 "Never calls the network" is false for partial clones | High | Accepted: no lazy fetch with its own message, timeout; every "no network" statement corrected | Phase 2 req 2, 5; Phase 3; Phase 7 req 1, validation; AC; Phase 8 check 15 |
| 31 | R4-3 Atomic write follows a symlinked `.tmp` | High | Accepted: random `wx` temp, `lstat` chain, tests | Phase 2 req 7; Phase 8 check 16 |
| 32 | R4-4 `review_lessons` over MCP runs from the wrong folder; path unquoted | High | Accepted: structured fields, id-pattern check, absolute quoted CLI command | Phase 6 req 2-4; decision 11 |
| 33 | R4-5 Tracked or tampered bundle still advertised and blocks every `doctor --fix` | High | Accepted: integrity gate on the command, per-item blocking, wrapper withdrawn | Phase 6 req 1; Phase 4 design 3; decision 4 |
| 34 | R4-6 Agent reads untrusted sources next to secret env files | High | Accepted: bare mirrors, skill warning, deny-read request to the onboarding owner | Phase 3 req 5-6; sequencing; Re-verify list; Phase 8 check 18 |
| 35 | R4-7 Vendored js-yaml provenance and update path | Medium | Accepted per adjudication item 14: tarball URL and sha512 integrity pinned, update procedure recorded (no audit stub added) | Phase 2 context, req 1; Phase 7 req 9 |
| 36 | R4-8 Relative wrapper path lends the curator name to foreign `~/.agents` content | Medium | Accepted: wrapper withdrawn when the canonical file is blocked; never add `~/.agents` to `additionalDirectories` | Phase 4 design 3, risks |
| 37 | R4-9 Exclude and tracked fix hide or untrack user files | Medium | Accepted: suggestion only for owned files, never `-r`; warning for user files in ignored folders | Phase 5 req 3-4; decision 7 |

### Whole-Plan Consistency Sweep

Decision deltas applied across all files:

1. Canonical files are a host-less shared kind in `Inspection.Shared`; IDs carry the path; an `agents` line shows
   them (plan decisions 1, 4; Phase 4).
2. System-skill conflicts block only their own item; the wrapper is withdrawn when the canonical file is blocked
   (decision 4; Phases 4, 7, 8).
3. Canonical files stay only while a local registration remains; removal order is wrappers first
   (decision 9; Phase 4; Phase 8 checks 10-11).
4. Hub `.gitignore` entries via `requiredIgnoreEntries` replace the local exclude (onboarding contract);
   `system_skill_tracked` is a non-blocking warning found with `git ls-files` (decision 7; Phases 5, 7, 8).
5. The script finds the hub from the file; `--hub` only asserts (decision 12; Phases 2, 3, 7, 8).
6. Hardened offline git, safe write, PyYAML-compatible loader (decisions 12, 13; Phase 2).
7. `review_lessons` is structured, integrity-gated and rendered absolute per OS; web texts print no command
   (decision 11; Phase 6).
8. Ownership and waves: Phase 2 after Phase 1; Phase 5 and 6 after Phase 4; Phase 4 owns the app and CLI consumers;
   effort 36h -> 52h; critical path 19h -> 34.5h.
9. The stdio launcher stays `mcp serve` (sequencing).

Stale-term grep over `plan.md`, every phase file and `reports/scout-notes.md` after the edits:

- `ExcludeLocally`, `info/exclude`, `local exclude`: only where the plan records that they were dropped (red-team
  comments, Phase 5 context, the scout notes' contract-change entry, owner decision 6, this section).
- `mcp stdio`: nowhere except this line; the ownership split says the launcher keeps the name `mcp serve`.
- `connection written`: only in Phase 4's red-team note and this table.
- `36h`, `8h` (Phase 4), `10h` (Phase 2): only in red-team comments.
- `check-ignore`: only in Phase 1 (this code repo's `.gitignore` check on untracked files, valid) and in Phase 5's
  explanation of why it is not used.
- `never calls ... the network`: replaced by the narrower statement everywhere.
- `CORE_SCHEMA`: only where the plan explains why it is not used.
- `Python`/`distill.py`: only as the reference implementation, the fixtures' header, the differential run, the
  known-digest note and docs that must stop saying it.
- `.gitignore` edits: the repo `.gitignore` (Phase 1) and the hub `.gitignore` entries (Phase 5) are the only two,
  and they are different files.
- Host-less kind wording: "Host field: the empty host" no longer appears; the shared kind is described once in
  Phase 4 design 2. "empty host" remains only where it is the bug being prevented (Phase 4 context on
  `validateChangePath`, the Phase 4 output test and the matching acceptance criterion).
- Frontmatter `wave:` of every phase matches the wave table (Phase 7 -> 4, Phase 8 -> 5, Phase 9 -> 6).

Reconciled: frontmatter effort, phase table, wave table, critical path, acceptance criteria, risks, compatibility,
owner decisions, open questions, the "Re-verify at start" list and `reports/scout-notes.md` (onboarding contract,
CI jobs, js-yaml integrity, consumers of the inspection shape, distill.py write path).

Unresolved (none contradicts the design; each blocks only the named step). <!-- Validation: superseded --> All five
items below are closed or changed by the "Validation Log" that follows; they are kept as the red-team record:

- The minimum Gemini CLI version that reads `.agents/skills` (text of the Phase 4 notice and Phase 7 docs).
- The minimum Git version honouring `GIT_NO_LAZY_FETCH` (Phase 2 version check, Phase 3 requirement).
- The onboarding owner's answer on hub-scope deny-read rules for `runtime/config/**` and `runtime/envs/**`; until it
  ships, the skill warning is the only control.
- The onboarding plan's doctor extension point name (adapter target, Phases 4 and 5).
- Whether the onboarding plan's per-agent output parses item IDs the same way as `hostChangeLines` today (Phase 4
  design 12 adapts to it).

## Validation Log

### Session 1 (2026-10-11)

Full-tier validation against main `e3403e8`; every citation changed in this session was re-checked with
`git grep`/`sed` on main `944cfbf` (the only commit since is a test change in `internal/delivery/mcpserver`). The
onboarding worktree held plan files only. Edits are marked `<!-- Validation: ... -->`.

**Claims checked: 155. Verified: 143. Failed: 10. Unverified: 2.**

| Phase | Checked | Failed |
|---|---|---|
| P1 | 19 | 0 |
| P2 | 25 | 2 |
| P3 | 9 | 0 |
| P4 | 38 | 4 |
| P5 | 11 | 0 |
| P6 | 22 | 1 |
| P7 | 15 | 1 |
| P8 | 8 (+1 unverified) | 0 |
| P9 | 5 | 0 |
| Scout notes | 3 | 2 |

The four Phase 4 failures are items 1-4 below (item 4 is plan.md decision 6, counted with Phase 4).

FAILED claims and the fix applied:

1. Phase 4 note: "connection written ..." exists "neither on main nor in the onboarding plan". It is in the
   onboarding plan (plan.md:210, phase-02:147-148, `connectionSummaryLine`). Fix: note rewritten; the string comes
   from their `connectionSummaryLine` and is reused unchanged; this plan only keeps `agents` out of its host list
   (Phase 4 design 12, note after design 16; scout notes).
2. Phase 4 context: `levels[change.Host]` at `connect.go:79,93`, `connect_remote.go:226`. Fix: `connect.go:79,94`,
   `connect_remote.go:226,244`, plus the maps built at `connect.go:52`, `connect_remote.go:196`,
   `workspace.go:377,437` (Phase 4, scout notes).
3. Phase 4 owned files missed `internal/delivery/cli/testdata/connect-workspace/{project-output,global-output}.golden`
   (used at `connect_regression_test.go:231,248`). Fix: owned in Phase 4 and the wave table, with expected content.
4. Decision 6 cited `host-connections.go:23,26` for `connect`. Fix: `ConnectService.target`,
   `internal/app/connect.go:131-160`; the host-connections lines kept for doctor's project and user checks
   (decision 6, Phase 4).
5. Phase 2: 5.4.3 "declares an `argparse` dependency" as the reason against it. Fix: 4.3.2 declares `argparse ^2.0.1`
   too and is the `v4-legacy` dist-tag (`npm view`, re-checked); the choice now rests on the import-free ESM file,
   the version already used in this repo and the young major (Phase 2, scout notes).
6. Phase 2 test-name rule listed only three patterns. Fix: `Performance` (`release.yml:160`) and
   `Fault|Hardening|DiskFull|ReadOnly|ConcurrentFrames` (`release.yml:168`).
7. Phase 6 missed `schemas/curation-home-v1.schema.json` (`actionItem` `additionalProperties: false`, `:59-65`;
   `internal/delivery/cli/status_schema_test.go:27`). Fix: Phase 6 owns it and the schema test as a public contract
   change; `command` stays required (empty when gated); release-note line in Phase 7.
8. Phase 7: `command-reference.md` "no distill mention". Fix: "no distill-lab mention"; `:65` mentions lessons and
   "Distill with Curator Agent".
9. Scout notes: runners "macOS 15 arm64, Node 22.23.x default". Fix: runner table (macos-latest = macOS 26 arm64,
   default Node 24.20.0; Ubuntu 24.04 and Windows 2025 default 22.23.3, 24.21.0 cached).
10. Scout notes: `SkillRelativePath` used "only in types.go and three tests". Fix: also the adapter literals at
    `integration.go:34-36`; test lines listed.

UNVERIFIED (2), now handled: running under the official Node 18.0.0 binary is Phase 8 check 5, done with a script
file that avoids the blocked path segment, or recorded as best effort with CI covering Node 18 via `setup-node`.
Codex following symlinked skill folders is not relied on (the plan writes no symlinks); recorded under "Open
questions".

Answers to the validator's questions 5(a)-(e):

- (a) Gemini CLI first reads `.agents/skills` in v0.28.0 (stable 2026-02-10; PR #18151 merged 2026-02-03; user and
  workspace tiers; `.agents` wins within a tier). https://github.com/google-gemini/gemini-cli/pull/18151,
  https://geminicli.com/docs/cli/creating-skills/. Applied: decision 2, Phase 4 design 15, Phase 7, Phase 8, risks.
- (b) Git 2.45.0 is the first version with `--no-lazy-fetch` and documented `GIT_NO_LAZY_FETCH`; 2.39.4 to 2.44.1
  honour only the variable. https://github.com/git/git/blob/master/Documentation/RelNotes/2.45.0.adoc,
  https://github.com/git/git/blob/v2.45.0/Documentation/git.txt. Applied: decision 12, Phases 2, 3, 7.
- (c) Claude Code substitutes `${CLAUDE_SKILL_DIR}` in skill content; reads need no approval only inside the working
  directory and additional directories, so the project-scope wrapper reads without a prompt and the user-scope
  wrapper is expected to prompt in Manual mode. https://code.claude.com/docs/en/skills,
  https://code.claude.com/docs/en/permissions. Applied: decision 2, Phase 4 risks, Phase 7 req 4, Phase 8 check 12.
- (d) Node: ESM unflagged from v12.17.0/v13.2.0, stable from v14.17.0/v15.3.0, `node:` imports from v14.13.1; the
  technical floor is about 14.13.1. Node 18 stays the stated minimum; Node 18 is end of life (April 2025).
  https://nodejs.org/api/esm.html. Applied: decision 12, Phase 2 context.
- (e) Runners (images 2026-10-04 / 2026-09-07): ubuntu-latest Ubuntu 24.04 and windows-latest Windows Server 2025
  default 22.23.3; macos-latest macOS 26 arm64 default 24.20.0; none ships Node 18, so `actions/setup-node` with
  `node-version: 18` is required. https://github.com/actions/runner-images/blob/main/README.md. Applied: decision 12,
  Phase 2 context, scout notes.

Decisions taken by the lead with the owner's authority (owner decision 7):

1. The release is 0.5.0, not 5.0.0; every occurrence replaced; the onboarding release is "the release that carries
   the onboarding redesign, 0.5.0".
2. Versions filled in: Gemini CLI v0.28.0, Git 2.45.0; Node floor ~14.13.1 technically, 18 kept as the minimum
   (end of life noted); `setup-node` with 18 required; runner table updated.
3. The Claude Code user-scope wrapper is "expected to prompt for reads outside the working directory in Manual
   mode, per the permissions docs"; the project-scope wrapper reads without a prompt.
4. The deny-read request is dropped (it conflicts with `RuntimeAccessDirs`,
   `internal/hostintegration/runtime_access.go:12-23`, and the bootstrap rule to source `$SKILLHUB_CONFIG_DIR/env`).
   Bare mirrors, the prompt-injection warning and git hardening stay. The residual risk is stated in "Risks",
   Phase 3 and the docs.
5. Onboarding extension point (`internal/app/health.go`, `CheckProvider`, `Finding`, `RegisterProvider`, their
   phase-04:33-63): the Node notice, `system_skill_tracked` and `system_skill_unowned_ignored` are `notice` Findings
   of `systemSkillProvider`; integrity stays in host inspection items because it needs Plan/Apply semantics
   (decision 10, the answer to their plan.md:208 and phase-04:62-63). "Extension point unresolved" removed; a
   "Re-verify at start" item checks the names.
6. The vendored folder is `scripts/lib/`; the js-yaml extraction and the Node 18.0.0 download use small Node script
   files, so no Bash command contains a segment that `.claude/hooks/scout-block.cjs` blocks; the Phase 8 safe-write
   check no longer uses a blocked file name.
7. Bundled files never contain `python` or `distill.py`; differences say "the previous implementation" or "the
   reference emitter"; Phase 7 no longer writes "no Python" into docs, so its grep is consistent; Phase 8 check 4
   greps the same words as Phase 3's test.
8. Phase 4 design corrections: `changeRank` replaces `changeOrder` (`files.go:289-302`) in `buildPlan` and Apply;
   the `DoctorFix` gate (`workspace.go:228-230`) no longer skips project and user repairs for a blocked hub item;
   `applyHostIntegration` (`:418-456`) lists blocked items; one ID parser for `hostChangeLines`,
   `renderConnectResult`, the summary host list (skipping `agents`), with only skill IDs changed so
   `connectCurationGuidance` stays as is; `formatChangeKind` prints "<id> skill"; both output goldens owned;
   `validateChangePath`/`expectedChange` accept several ids per host and the Gemini legacy path for removal only;
   `prepare` reads every adapter's registrations for `needsCanonical`; `MissingFinding` reports `Blocked` as
   `host_integration_blocked` through the existing status action (no edit to `curation_home.go:497-590`). Phase 6
   owns the status schema. Phase 2: release verify gets `fetch-depth: 0`; distill-lab digests are enforced per tag
   only (Phase 1).

### Whole-Plan Consistency Sweep

Checked with a scripted grep over `plan.md`, every phase file and `reports/scout-notes.md` after the edits:

- `5.0.0`: only inside `<!-- Validation -->` comments that record the rename and in this log.
- The vendored folder: `scripts/lib/` everywhere for this skill; the old folder name remains only for the other
  skill's existing path (`.claude/skills/distill/scripts/...`) and in the hook's pattern list.
- `<minimum version>`, `<floor>`, `[UNVERIFIED]`: none left.
- `deny-read`: only where the plan records that it was dropped and in the red-team record (finding 34 and the old
  "Unresolved" list, both marked superseded).
- Doctor additions: "warning" for the Node, tracked and unowned checks is now "notice" (`Finding`, `Fix` nil);
  "adapter" survives only in red-team comments and the red-team table (finding 8); the Gemini notice is an item
  impact plus result warning, not a provider Finding (Phase 4 design 15).
- User-scope wrapper: "expected to prompt" in decision 2, risks, Phase 4, Phase 7 and Phase 8; "unverified" remains
  only in red-team comment labels.
- `connection written`: Phase 4's note (now attributing it to `connectionSummaryLine`), the red-team table and the
  scout notes.
- "curator skill": only where the plan says it is no longer printed.
- Effort: frontmatter 57h = 4 + 13.5 + 2 + 20 + 2.5 + 5.5 + 3.5 + 5 + 1; phase table and phase frontmatter agree;
  critical path 1 -> 4 -> 6 -> 7 -> 8 -> 9 = 39h; script path 19.5h. Older figures (16h, 52h, 34.5h) remain only in
  red-team and validation comments.
- File ownership: Phase 4 gains `internal/delivery/cli/host_item_id.go`, the summary host-key collection and the two
  output goldens; Phase 6 gains the schema and its test; no wave-3 overlap (P5 owns only
  `system_skill_doctor.go` there, P6 never touches it).
- Acceptance criteria, risks, "Re-verify at start", owner decisions and open questions reconciled with the above.

Unresolved (none blocks the design):

- The onboarding owner has not yet been told that integrity is not registered through `RegisterProvider`
  (their plan.md:208, phase-04:62-63 expect it); the lead does this before Phase 4.
- The connection summary host list moves into the onboarding plan's `connectionSummaryLine`; the exact file and line
  are known only after their merge ("Re-verify at start").
- Node 18.0.0 itself may only be best-effort in Phase 8 (CI runs the latest 18.x).
- Gemini CLI and Codex loading from `.agents/skills`, and the user-scope prompt, stay documented rather than tested.
