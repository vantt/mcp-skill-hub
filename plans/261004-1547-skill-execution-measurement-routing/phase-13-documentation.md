---
phase: 13
title: "Documentation"
status: pending
priority: P2
effort: 8h
dependencies: [1, 2, 3, 4, 5, 5a, 5b, 6, 7, 8, 9, 10, 11, 12, 12a]
---

# Phase 13: Documentation

## Goal

Every user-visible behavior shipped by this plan is documented once, in its owning document, matching the code: skill execution and content trust, secret env and doctor, host permissions, measurement, routing quality, and the WebUI runtime view.

## Context (read these first)

- Rules: `.claude/rules/documentation-management.md` (update the smallest owning surface, link to schemas instead of copying them, read before editing, verify after editing). `plan.md` → "Executor notes", decisions D2–D4, D10, D11.
- Design docs under `docs/design/` are **Vietnamese**; keep edits in Vietnamese and in their existing style. User docs (`docs/user-guide.md`, `docs/curating-skills.md`, `docs/contracts/*.md`, `README.md`) are **English**.
- Source material already written by implementers (read it; it is the authority for wording of behavior):
  - `phase-05a-runtime-revision.md` → "Part B" behavior matrix (cases A–D), "Risks" (Node ESM cannot use `NODE_PATH`; any content edit resets approval; imported third-party skills lose `local.path` until approved).
  - `phase-05b-runtime-hardening.md` → "Docs notes", "R1 status", "R6 status".
  - `reports/*.md` for phases 6–12a (written during execution).
- Commands to document (verify each with `go run ./cmd/skillhub help <cmd>` before writing): `skill review <id> [--verbose]`, `skill edit <id> --approve-content <digest>`, `skill edit <id> --runtime-file <yaml>`, `skill edit <id> --example/--counter-example`, `skill doctor <id> [--json]` (exit 0 ready, 1 setup required or unsupported platform, 2 invalid/unknown), `skill env set|unset <id> <KEY>`, `skill env list <id>`, `doctor [--fix --yes]`, `connect [-g]`, `telemetry funnel`, `telemetry import-transcripts --project <dir>`, `eval routing`, `validate`.
- Schemas to link, never copy: `schemas/skill-metadata.schema.json`, `schemas/skill-resolve-response-v1.schema.json`, `schemas/telemetry-event-v1.schema.json`, `schemas/result-envelope.schema.json`.

## Targets and required content

Read each file in full before editing; change only claims made false by this plan and add the missing surfaces.

1. **`docs/design/01-system-architecture.md`** (Vietnamese): §2 invariants (row "Progressive disclosure": skills are delivered as MCP content and, for trusted skills, as a digest-pinned read-only local snapshot plus a writable state directory; the hub never executes skill code in the MCP flow), §6 progressive loading (activation via `skill_get`/`skills/get` with `local`, review-required behavior), §7.2 `runtime/` layout (`runtime/cache/skills/<id>@<16hex>`, `runtime/envs/<id>@<deps16>`, `runtime/config/<id>/env` 0600, `runtime/cache/doctor/<machine>/…`; disposable vs user data), §11 security (content trust gate is cooperative-not-adversarial: it prevents accidental use by cooperative agents, not an adversarial agent that can read the workspace directly; local-folder adds are trusted by design; approval is CLI-only, never MCP or WebUI; secret values never enter output, cache, or telemetry), §12 non-goals (replace "Tự execute skill scripts" wording with: the hub does not run skill scripts in the MCP flow; `skill doctor` runs the declared `check` only on explicit user command and only for trusted content).
2. **`docs/design/02-agent-hub-protocol.md`** (Vietnamese): §2 bootstrap text (current host paragraph from `internal/hostintegration/bootstrap.go`: export `local.env`, load `$SKILLHUB_CONFIG_DIR/env` without printing values, run `check` in `working_directory`, ask before `setup`, prose install = setup into `SKILLHUB_STATE_DIR` never globally, no concurrent setup (`.setup.lock`), POSIX `export` vs PowerShell `$env:`, call `skill_get` again if `local.path` is gone, `review_required` = do not use the skill, report `setup_failed`), §3 protocol layers (`local` object fields, `preflight` with platform-only `live_checks`, `_meta["io.skillhub/local_path"]` on `resources/read`), §4.3 task description in English, §5.2 `resolved` (`setup` object: states, `basis: terminal`, `checked_at`; link the response schema), §7.1 normal path, §9 feedback (`setup_failed` reason), §10 error model (`content_review_required`).
3. **`docs/design/03-resolver-design.md`** (Vietnamese): §3 routing metadata example (`examples`, `counter_examples`, `technologies`, `topics`), "Validation rules" (lint warnings and their codes, warnings never fail validation), §4.2 FTS document (columns and explicit weights; triggers at 1.0 unless phase 12 changed it — read `reports/routing-calibration-report.md`), §7 feature model (example and counter-example folding, technology match), §9 calibration (link the calibration report outcome in one sentence, no plan IDs).
4. **`docs/design/04-telemetry-reproducibility-evaluation.md`** (Vietnamese): §3.2 core events (`skill.doctor_checked`, `transcript.tool_observed`, server-observed `skill.loaded` incl. blocked loads with `status: review_required`), §3.4 outcome semantics (server-observed activation and attribution classes vs host claims; curator `skill_get` inspections land in `unsolicited`; draft `skill_get` is not counted), §4.2 retention (14-day raw, 180-day daily rollups), §5 storage (`telemetry_daily_rollups`), §7 golden corpus (generated routing cases, leave-one-out, no-skill set, gate thresholds), §9 metrics (funnel metric names including `blocked:review_required`, `feedback:setup_failed`, `doctor:*`, `feedback:negative` counted once per report), and a note that named measurement cases are deferred.
5. **`docs/user-guide.md`** (English):
   - "How the agent picks a skill": local path, state directory, env variables, preflight, `setup_required`, `review_required` (agents get nothing until you approve).
   - "Set up and connect agents" → "What gets written": host permission entries added by `connect`/`doctor --fix --yes` for `runtime/cache/skills`, `runtime/envs`, `runtime/config` — Claude Code `permissions.additionalDirectories` (project: `.claude/settings.local.json`, also honors `.claude/settings.json`; user: `~/.claude/settings.json`), Codex `[sandbox_workspace_write] writable_roots`, Gemini CLI `context.includeDirectories`; `skillhub doctor` reports missing entries; there is no automatic removal.
   - New section "Run skills that need tools or secrets": behavior matrix A–D from phase 5a; `skill doctor` (what it checks, `basis: terminal`, exit codes, result is a cached hint for agents, not cached for unapproved skills); `skill env set|unset|list` (value read without echo or from stdin, stored 0600 under `runtime/config/<id>/env`, never in Git, names only in `list`, `SKILLHUB_` names reserved); **Windows caveat**: the env file is POSIX `KEY=value` with single-quoted values for shell-special characters; PowerShell hosts must parse it (or set variables with `$env:NAME`) rather than dot-source it [UNVERIFIED: per-host Windows behavior of Gemini CLI/Codex shells was not tested; state the format, not host guarantees]; Node ESM cannot use `NODE_PATH`, so ESM skills must run from the state directory or bundle dependencies.
   - "Troubleshooting": `review_required`, `setup_failed`, missing host permission entries, `local.path` gone in a long session, replace any mention of restricted snapshots.
   - "Command cheat sheet": add the commands listed in Context.
6. **`docs/curating-skills.md`** (English):
   - "Safety model": content trust (third-party = GitHub/git origin or a source id; local-folder adds trusted by design), approval is CLI-only and why (agents and browser sessions must not self-approve), approval resets on any content or runtime-block change (including typo fixes), cooperative-not-adversarial scope.
   - "Review diagnostic facts": `content_trust` and `changes_since_approval` (added/removed/modified, `scripts_changed`, `runtime_changed`, `dependencies_changed`, `history_truncated`; baseline is the oldest commit of the unbroken run of manifest commits carrying the recorded digest, walk capped at 200 commits; `found: false` means nothing is claimed), the printed `git diff` command, `runtime_hints` incl. `missing_lockfiles`; the WebUI Review tab and Runtime tab show the same facts and the approve command, with no approve button.
   - "Edit and improve a skill": `--example`, `--counter-example`, `--runtime-file` (`{}` removes; makes approval stale), curator proposes runtime blocks with pinned, lockfile-based installs.
   - New short section "Measure usage": `telemetry funnel` (metrics, window, basis meanings), Usage tab, `import-transcripts` privacy boundary (only tool, skill ID, timestamp, session hash; explicit `--project`; window clamp to 14 days).
   - New short section "Check routing quality": `eval routing`, lint warnings from `validate` and `skill review`.
   - "Command map": add the new commands.
7. **`docs/contracts/error-codes.md`** (English): add `content_review_required` (what it means, what the agent should do: do not use the skill, tell the user to run `skillhub skill review <id>`).
8. **`README.md`**: only if its feature or command list names behavior that changed (check the "Diagnose issues" line at `README.md:69` and the feature list).

General requirements:
- Minimum binary: workspaces that use `routing.examples`, `routing.counter_examples`, `runtime`, or `quality.content_reviewed_digest` need a binary newer than `v0.2.0` (latest tag at plan time; re-check with `git tag --sort=-v:refname | head -1` and use the release version if one was cut). The catalog rebuilds automatically after upgrade.
- No plan IDs, phase numbers, decision labels, or finding codes in docs.
- Do not copy schema contents; link them.

## Files

Modify only: `docs/design/01-system-architecture.md`, `docs/design/02-agent-hub-protocol.md`, `docs/design/03-resolver-design.md`, `docs/design/04-telemetry-reproducibility-evaluation.md`, `docs/user-guide.md`, `docs/curating-skills.md`, `docs/contracts/error-codes.md`, and `README.md` (only if needed).

## Steps

- [ ] **1. Read** every target in full and the source material listed in Context.
  Pass: none (reading step).
- [ ] **2. Edit** design docs (Vietnamese), then user docs and contracts (English).
  Pass: `grep -rnE 'approve-scripts|scripts_reviewed_digest|\.restricted|scripts_review_required|withheld' docs README.md` prints nothing.
- [ ] **3. Verify commands and flags** against help output.
  Pass: for each of `skill`, `telemetry`, `eval`, `doctor`, `connect`, `validate`, every subcommand and flag named in the docs appears in `go run ./cmd/skillhub help <cmd>`.
- [ ] **4. Verify field names** against schemas and code.
  Pass: every JSON field named in the docs (`local.*`, `setup.*`, `content_trust.*`, `changes_since_approval.*`, `runtime_hints.*`, funnel metric names) is found by `grep -rn '"<field>"' schemas internal --include='*.go' --include='*.json'`.
- [ ] **5. Verify links.**
  Pass: for each edited file, every relative link target from `grep -oE '\]\([^)#]+' <file>` exists on disk.
- [ ] **6. Plan-ID scan.**
  Pass: `grep -rnE '\b(D[0-9]+|R[0-9]+|phase[ -]?[0-9]+[ab]?)\b' docs/user-guide.md docs/curating-skills.md docs/contracts/error-codes.md` prints no plan references (review each hit manually; ordinary words are fine).
- [ ] **7. Gate.** Pass: `make check` exits 0 (help tests guard the command surface).

## Risks

| Risk | L × I | Mitigation |
|---|---|---|
| Docs overstate guarantees ("scripts can never run", "untrusted skills are sandboxed") | Medium × Medium | Phrase as implemented: the gate withholds content from cooperative agents; hosts control execution; the hub never runs skill code in the MCP flow. |
| Vietnamese/English drift between design and user docs | Low × Low | Each fact documented once in its owner doc and linked from the other. |
| Host config keys change upstream | Low × Medium | Cite the key names as verified in `reports/fullstack-developer-261004-2155-host-runtime-access-report.md`; do not promise behavior of the host beyond the key. |

## Rollback

Revert the docs commit.

## Failure protocol

Follow `plan.md` → "Executor notes" → "Failure protocol". If a documented behavior does not match the code, document the code's behavior and list the mismatch in `reports/<agent>-<YYMMDD-HHMM>-documentation.md`; do not change code in this phase.
