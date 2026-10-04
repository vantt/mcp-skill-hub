---
phase: 5b
title: Runtime hardening (host permissions, unapproved content, review diff, supply-chain hint, small fixes)
status: done
depends_on: [5a]
effort: 8h
---

# Phase 5b — Runtime hardening

## Context
Second runtime review (2026-10-04). User decisions: unapproved third-party skill content must not reach the agent (item 2); local-folder skills stay trusted (item 10). Item 4 (secret storage) is pending a user decision and is **out of scope here**.

## Requirements

### R1. Host permissions for snapshot and state paths (`internal/hostintegration/`)
`local.path` and `state_directory` live under `<workspace>/runtime/cache/skills` and `<workspace>/runtime/envs`, outside the project. Hosts sandbox or prompt for paths outside the project.
- During `skillhub connect` (project and `-g` user scope), add those two directories to each host's allowed-directory config, idempotently, without clobbering user entries, and remove them on disconnect if the codebase has a disconnect/uninstall path:
  - Claude Code: `permissions.additionalDirectories` in the settings file the integration already manages (project `.claude/settings.json` or user `~/.claude/settings.json`; check which file the code writes today — if it only writes `.mcp.json`/`.claude.json`, add settings handling minimally).
  - Codex: `[sandbox_workspace_write] writable_roots` in `config.toml`.
  - Gemini CLI: verify the current setting name from official docs (e.g. `context.includeDirectories`); if you cannot verify it, skip Gemini and report.
- Verify each key against current official docs (WebFetch) before coding; cite the URL in your report.
- `skillhub doctor` reports when a connected host lacks these entries, and `--fix` adds them, following existing doctor check patterns.

### R2. No content for unapproved third-party skills
For a third-party skill whose content digest is not approved:
- `skill_get` / `skills/get`: return id, name, description, routing, lifecycle state, and `local {status: review_required, reason_codes, review_command}`; **omit SKILL.md content** (empty/absent field — follow the response schema; update schema + tool description).
- `resources/read`: refuse every resource **including** SKILL.md, code `content_review_required`.
- `skills/list` and `skill_resolve` unchanged apart from existing `review_required` flags.
- Host instruction paragraph + `skill_get` description: replace "use only the SKILL.md guidance" with "do not use the skill; tell the user to run `skillhub skill review <id>`". Refresh AGENTS.md/CLAUDE.md/GEMINI.md via the existing bootstrap mechanism.
- CLI `skillhub skill review` and the web UI still show content to the human (that is how review happens) — do not gate those.

### R3. Review diff since last approval (`internal/app/skill_review.go`, CLI review output)
- Find the last approved content: walk Git history of the skill's `skill.meta.yaml` (use the repo's existing git helper layer; no new git library) for the most recent commit whose `quality.content_reviewed_digest` equals the currently recorded approval, then compare that commit's skill folder file list (path → blob digest or content digest) with the current catalog files.
- Report `content_trust.changes_since_approval`: `added`, `removed`, `modified` path lists, plus booleans `runtime_changed`, `scripts_changed` (any changed path under `scripts/` or with an interpreter per runtime hints), `dependencies_changed` (dependency manifest list from skillruntime).
- No approval yet → `changes_since_approval` absent and human output says "never approved — review the full skill".
- Human output prints the change summary above the approve command, highlighting scripts/runtime/dependency changes, and suggests `git diff <commit> -- <skill dir>` for full detail.
- Bounded: stop history walk after a reasonable limit (e.g. 200 commits) and report `history_truncated`.

### R4. Supply-chain hint
In runtime hints (`skillruntime.AnalyzeHints`): `missing_lockfiles` listing ecosystems with a manifest but no lockfile (npm: package.json without package-lock.json/npm-shrinkwrap.json/pnpm-lock.yaml/yarn.lock; python: pyproject.toml without uv.lock/poetry.lock, requirements*.txt is acceptable only if every line pins `==`… keep it simple: flag `requirements*.txt` only when it contains a line without `==` — report the file name, not lines; go/cargo/ruby: manifest without lock). Curator SKILL.md (both copies, kept identical): when proposing a runtime block, prefer pinned versions and lockfile-based installs.

### R5. Small fixes
- Concurrent setup: host instruction adds one line — do not run `setup` for the same skill concurrently; if `$SKILLHUB_STATE_DIR/.setup.lock` exists and is recent, wait or ask. (Instruction only; no hub-side locking.)
- Windows: preflight instruction text and host paragraph mention setting the variables with the platform's shell syntax (POSIX `export` / PowerShell `$env:`), not only `export`.
- Long sessions: the host paragraph says to call `skill_get` again if `local.path` no longer exists.
- Docs note material for later phase 13 (just add bullets to this phase file's "Docs notes" section): trust gate protects against accidental execution by cooperative agents, not adversarial ones (agents can read the workspace directly); local-folder adds are trusted by design.

### R6. Per-skill secret env (user accepted 2026-10-04; implemented after R1–R5)
- Stable per-skill config dir `<workspace>/runtime/config/<id>/` (`0700`), file `env` (`0600`, `KEY=value` lines, never in Git — `runtime/` is already ignored). Not keyed by digest or deps, so it survives skill updates.
- CLI: `skillhub skill env set <id> <KEY>` reads the value from a TTY without echo (or from stdin when not a TTY, e.g. piped); `skillhub skill env unset <id> <KEY>`; `skillhub skill env list <id>` prints key names only, never values. KEY must match the existing env-name pattern. Atomic write, symlink-safe, preserves other keys.
- `local.env` gains `SKILLHUB_CONFIG_DIR` for trusted skills (dir path; create lazily, don't create the file). Host paragraph + `skill_get` description: when running check/setup/scripts, load `$SKILLHUB_CONFIG_DIR/env` if it exists (e.g. `set -a; . "$SKILLHUB_CONFIG_DIR/env"; set +a`, PowerShell equivalent), and never print or echo its values.
- Doctor: an env requirement passes if present in the process env OR as a key in the skill's config env file (presence only; values never read into output/cache/telemetry). Doctor's `check` command runs with the config file's variables added to its environment.
- Doctor output for a missing env var suggests `skillhub skill env set <id> <KEY>`.
- Untrusted skills: no `SKILLHUB_CONFIG_DIR` in responses.
- Tests: set/unset/list (list never shows values; sentinel value never appears in any output, cache, or telemetry), permissions 0600/0700, symlink refusal, doctor presence via file, local.env includes the dir only for trusted skills.

## Tests / validation
Connect/doctor tests for each verified host (idempotent add, preserves user entries, doctor detects missing, --fix adds); unapproved skill: skill_get has no content, SKILL.md read refused, approve → content + reads work; review diff: never approved, approved then script modified (scripts_changed true), runtime change, dependency change, truncated history; lockfile hints; host instruction tests. `make check` passes.

## R1 status
Done (uncommitted). Hosts verified against official docs and implemented: Claude Code `permissions.additionalDirectories` (new change kind `host-permissions`; project scope writes `.claude/settings.local.json` and also accepts entries already in `.claude/settings.json`; user scope uses `~/.claude/settings.json`), Codex `[sandbox_workspace_write].writable_roots` and Gemini CLI `context.includeDirectories` (folded into the existing config file change). Directories: `runtime/cache/skills`, `runtime/envs`, `runtime/config` under the workspace (`hostintegration.RuntimeAccessDirs`). Entries are merged idempotently, user entries preserved. `skillhub doctor` reports missing entries (it plans from the same inspection) and `--fix --yes` adds them. No disconnect path exists, so nothing to remove. Tests: `internal/hostintegration/runtime_access_test.go`.

## Docs notes
- Trust gate scope: the gate protects against accidental execution by cooperative agents. It does not stop an adversarial agent, which can read the workspace directly. Say so plainly in the security documentation.
- Local-folder adds (`skillhub skill add <path>`) are trusted by design; only third-party origins (GitHub, git, or a source id) need content approval.
- An unapproved third-party skill exposes nothing to agents: `skill_get` omits `content`, and `resources/read` refuses every file including SKILL.md (`content_review_required`). `skillhub skill review` and the web UI still show everything to the human.
- `skill review` reports `content_trust.changes_since_approval` (added/removed/modified, `runtime_changed`, `scripts_changed`, `dependencies_changed`, `history_truncated`, a `git diff` command). The baseline is the oldest commit of the unbroken run of manifest commits that carry the recorded digest, found by walking at most 200 manifest commits. If the walk is cut off inside that run, nothing is claimed (`found: false`).
- Runtime hints gain `missing_lockfiles` (`<ecosystem>: <manifest path>`); the curator prefers pinned versions and lockfile-based installs.
- Host instruction paragraph (refreshed in AGENTS.md/CLAUDE.md/GEMINI.md): do not run `setup` concurrently (`$SKILLHUB_STATE_DIR/.setup.lock`, instruction only), set variables with the shell's syntax (POSIX `export`, PowerShell `$env:`), call `skill_get` again if `local.path` is gone.

## R2-R5 status
Implemented, with tests; R1 is handled separately. `make check` is run by the controller. See `reports/fullstack-developer-261004-2155-r2-r5-runtime-hardening.md`.

## R6 status
Implemented, with tests. `skillhub skill env set|unset|list <id> [KEY]` manages `<workspace>/runtime/config/<id>/env` (0600 in a 0700 directory, atomic write, symlinks refused, other keys preserved, `SKILLHUB_` names reserved). `set` reads the value without echo from a TTY (`golang.org/x/term`, already a dependency) or from stdin when piped; the value is never an argument. Trusted skills get `SKILLHUB_CONFIG_DIR` in `local.env` and `preflight.env` (directory created, file not); untrusted skills get no env at all. Doctor counts a stored key as present, runs `check` with the stored variables added, redacts stored values from check output, and suggests `skillhub skill env set <id> <KEY>` for a missing variable. Host paragraph (AGENTS.md/CLAUDE.md/GEMINI.md refreshed), `skill_get` description, preflight instruction and CLI help updated.
Also fixed: `skill_get` now withholds `content` and returns `local.status: review_required` for a third-party skill in any lifecycle state (draft, deprecated, archived) whose content digest is not approved, via `SkillService.ContentTrustFor`; CLI review and the web UI are unchanged. See `reports/fullstack-developer-261004-2213-r6-secret-env-and-draft-gate.md`.

## Rollback
Uncommitted; revert touched files.
