---
phase: 5a
title: Runtime revision (skill-level trust, writable state dir, platform-only hub checks, setup guidance)
status: pending
depends_on: [1, 2, 3, 4, 5]
---

# Phase 5a — Runtime revision

## Context

Review of phases 3–5 (user-accepted 2026-10-04) found the runtime approach did not meet its goals:

1. **Per-file script withholding is not a boundary.** SKILL.md is always served and can instruct any command; extension/shebang detection misses `package.json` lifecycle scripts, `Makefile`, `.go`, `.php`, `.lua`, `.tsx`, `pyproject` build hooks. It gives false assurance.
2. **Hub-side checks run in the wrong environment.** Live checks run in the MCP server process (env captured at host launch); the doctor runs in the user's terminal; the agent runs scripts in its own shell (nvm/pyenv/direnv/venv). Results disagree.
3. **Setup writes into a read-only, digest-keyed snapshot.** Files are `0444` so `npm install` / `uv sync` / `poetry install` fail rewriting lockfiles; any SKILL.md edit changes the digest and loses `.venv`/`node_modules`.

Already fixed (keep the tests green, then remove what this phase makes obsolete): withheld scripts no longer leak via `resources/read`; snapshot verification is memoized (`snapshotMemo`).

Accepted decisions:
- **D1 Skill-level trust.** One human approval over the whole skill content.
- **D2 Writable state directory** separate from the read-only snapshot.
- **D3 Hub checks only the platform.** Bins/env are verified by the agent's own `check` in its shell; doctor results are hints labelled `basis: terminal`.

Plus setup-guidance behaviour for skills with and without install instructions (see Part B).

Nothing from phases 1–5 is committed or released, so renames below need no migration.

## Part A — trust, state dir, checks (implementer A)

**Part A status: done** (A1-A4 and Part A tests; `make check` passes). See `reports/fullstack-developer-261004-2100-runtime-revision-part-a.md`.

### A1. Content digest and skill-level trust (`internal/skillruntime/trust.go`, callers)
- Replace `ExecutionDigest(executables, ...)` with `ContentDigest(files []ResourceDigest, spec Spec, hasSpec bool)`: sha256 over canonical JSON `{"files":[{path,digest}... sorted, excluding skill.meta.yaml], "runtime": spec-or-null}`, prefixed `sha256:`.
- Compute it from **catalog resource rows** (path + digest already stored), not by reading file bytes, so trust can be decided without loading the skill source. Cache the verdict in the snapshot memo keyed by manifest version.
- `Evaluate(p Provenance, contentDigest, reviewedDigest string) Verdict`: trusted when not third-party, or `reviewedDigest == contentDigest`. Reason codes: `content_review_required`, plus `content_review_stale` when a different digest was approved.
- Delete `IsExecutableResource`, executable extension list, executable digests, `Withheld`, the `.restricted` snapshot variant, and `LocalResource.Executable`. File modes in the snapshot: `0444` for all files, but keep the exec bit (`0555`) for files whose source mode or shebang says so if the catalog records it; otherwise `0444` and agents invoke via interpreter. (Check what the catalog stores; do not add new detection heuristics.)
- Rename manifest field `quality.scripts_reviewed_digest` → `quality.content_reviewed_digest` (schema, canonical validator, lifecycle UpdateInput, skill_review). Rename CLI flag `--approve-scripts` → `--approve-content`. Still CLI-only; never in MCP inputs.
- Third-party = origin kind `github`/`git` or `provenance.source_id` set (unchanged). Local-folder adds stay trusted (document in Part B docs note).

### A2. Behaviour of an unapproved third-party skill
- `skill_get` / `skills/get`: still return SKILL.md content and routing (unchanged behaviour), with `local: {status: "review_required", reason_codes: [...], review_command: "skillhub skill review <id>"}` and **no** `path`, no `preflight` commands.
- No snapshot is exported for it.
- `resources/read` of any resource other than the entrypoint `SKILL.md` is refused with the existing error helper, code `content_review_required`.
- `skill_resolve` still recommends it; attach `setup.state = "review_required"` (add to enum in response schema).
- `skill review` shows `content_trust` (third_party, approved, content_digest, approve command). Update the existing review→approve digest equality test.
- If `skillhub status` has an attention-items mechanism, add one item counting third-party skills awaiting content review; otherwise skip and say so.

### A3. Writable state directory (`internal/app/skill_snapshot.go`, `skillruntime`)
- `runtime/envs/<id>@<deps16>/` created on demand (`0700`, symlink-safe like the snapshot root), where deps16 = first 16 hex of sha256 over canonical runtime JSON + sorted (path, digest) of dependency manifest files anywhere in the skill: `package.json, package-lock.json, npm-shrinkwrap.json, pnpm-lock.yaml, yarn.lock, requirements*.txt, pyproject.toml, uv.lock, poetry.lock, Pipfile, Pipfile.lock, go.mod, go.sum, Gemfile, Gemfile.lock, Cargo.toml, Cargo.lock`.
- On creation, copy those dependency files into the state dir (same relative paths, writable `0644`) so installers can run there and rewrite lockfiles. Never overwrite an existing state dir's copies on reuse.
- SKILL.md-only edits must keep the same state dir (test this).
- `LocalPreflight` gains `state_directory` and `env: {"SKILLHUB_SKILL_DIR": <snapshot>, "SKILLHUB_STATE_DIR": <state>}`; `working_directory` becomes the state dir. Instruction text tells the host to export those two variables when running `check`, `setup`, or the skill's scripts.
- Doctor runs `check` in the state dir with both variables set (added to the inherited env).
- GC: remove state dirs not referenced by any active trusted skill's (id, deps16) and untouched for 7 days. Touch on reuse. Same chmod-then-remove approach as snapshots.

### A4. Platform-only hub checks (`skillruntime/checks.go`, snapshot preflight, resolver setup annotation)
- MCP flow (activation preflight and resolver annotation) evaluates only the platform requirement. Remove bin/env presence from `LiveCheck` usage in the MCP flow (keep the functions only if the doctor still uses them).
- Doctor keeps bins (with version probe), env presence, platform, and `check`; its cached result and every surface that shows it carry `basis: "terminal"`.
- `SetupState` order: `review_required` (untrusted third-party) → `unsupported_platform` → cached doctor state (hint) → `unknown`. Update resolver and response schema; keep the "setup never changes ranking" test.

### A tests / validation
Update/replace tests for: content digest (order-independent, meta excluded, runtime included); approve → trusted; content change → stale; unapproved → no path, entrypoint readable, other resources refused, no snapshot dir created; state dir stable across SKILL.md edit and changed by lockfile edit; dependency copies writable and not overwritten on reuse; doctor runs check in state dir with both env vars; MCP preflight has only platform live check; GC of state dirs. `make check` must pass.

## Part B — setup guidance with and without install instructions (implementer B)

**Part B status: done** (B1-B4, B tests, and the controller addition: every trusted skill gets `local.state_directory` and `local.env`, with or without a runtime block; `make check` passes). Deviations: the host text points at `local.env` (same values as `local.preflight.env`); curator bumped to 1.4.0. See `reports/fullstack-developer-261004-2119-runtime-revision-part-b.md`.

Behaviour matrix (document in docs later; implement the hub side now):

| Case | Hub | Agent |
|---|---|---|
| A. Has `runtime` block | Preflight with check/setup | Run check in state dir; ask user before setup |
| B. Install steps only in prose | Curator proposes a `runtime` block; human confirms | Until then, treat prose install steps as setup: ask user, install into `SKILLHUB_STATE_DIR`, never globally |
| C. Scripts, no instructions | `skill review` reports runtime hints | Run; on missing-dependency failure, stop, tell user, report `setup_failed` |
| D. Instruction-only | Nothing | Normal |

### B1. Runtime hints in `skill review` (`internal/app/skill_review.go`, CLI review output)
Static, content-free hints computed from catalog files (no execution):
- `interpreters`: from shebang line or extension (`.py`→python3, `.js/.mjs/.cjs`→node, `.ts`→node, `.sh`→sh, `.bash`→bash, `.rb`→ruby, `.ps1`→pwsh).
- `dependency_manifests`: dependency files found (same list as A3).
- `absolute_install_paths`: text files referencing `~/.claude/skills/`, `$HOME/.claude/skills`, `.agents/skills/`, `.codex/skills/` (these break under `local.path`).
- `missing_runtime_block`: true when the skill has interpreters or dependency manifests but no `runtime` block.
- `install_prose_detected`: true when SKILL.md/README contains install cues (`pip install`, `npm install`, `npm i `, `brew install`, `apt install`, `apt-get install`, `uv sync`, `poetry install`, `go install`, `cargo install`, headings named Prerequisites/Installation/Setup/Requirements). Report the cue names only, never surrounding text.
Human output prints a short "Runtime" section with these and a suggested next step. Expose the same struct for later reuse by metadata lint (phase 11 will call it).

### B2. Runtime block through the update flow
- Add optional `Runtime` to `skill.UpdateInput` and to `skill_update_preview` (MCP) and `skillhub skill edit --runtime-file <yaml>` (CLI); explicit empty object removes the block. Validate with `skillruntime.ParseSpec` rules. Preview/confirm stays human-confirmed. Changing runtime changes the content digest, so a prior approval becomes stale automatically (test this).
- Update `system-skills/curator/SKILL.md`: when reviewing a skill with `install_prose_detected` or `missing_runtime_block`, read its install prose and propose a `runtime` block via `skill_update_preview` (bins with versions, env names, platforms, `check`, `setup` that installs into `$SKILLHUB_STATE_DIR`), and explain it to the user before confirming. Keep the curator's existing structure/tone; keep the embedded systemskills test passing (update golden/digest if the test pins it).

### B3. setup_failed signal
- Add feedback reason code `setup_failed` (telemetry vocabulary, schema enum, MCP `skill_feedback` description).
- Rollup mapping: feedback with reason `setup_failed` also increments `feedback:setup_failed` for the skill.

### B4. Host instructions (`internal/hostintegration/bootstrap.go` + refreshed AGENTS.md/CLAUDE.md/GEMINI.md block)
Replace the current runtime paragraph with, concisely:
- Use `local.path` for relative file references; export `SKILLHUB_SKILL_DIR`/`SKILLHUB_STATE_DIR` from `local.preflight.env` when running check/setup/scripts.
- Run `check` in `working_directory`; ask the user before `setup`.
- If a skill describes installation only in prose, treat those steps as setup: ask first, install into `SKILLHUB_STATE_DIR`, never globally.
- If `local.status` is `review_required`, use only SKILL.md guidance, run nothing from the skill, and tell the user to run `skillhub skill review <id>`.
- If a script fails for a missing dependency, stop, tell the user, and call `skill_feedback` with `outcome: failed`, `reason_code: setup_failed`.
Keep the marker version unless the existing tests require bumping.

### B tests / validation
Hints unit tests (each detector, cue names only, no text leakage); update-flow runtime set/remove + stale approval; reason code accepted + rollup metric; host instruction tests. `make check` must pass.

## Risks
- Existing imported third-party skills lose `local.path` until approved — intended; surfaced in `skill review` (and `status` if available).
- Node ESM cannot use `NODE_PATH`; skills using ESM must run from the state dir or bundle deps. Document in phase 13.
- Content-review approval resets on any content edit, including harmless SKILL.md typo fixes for third-party skills. Accepted trade-off of D1.

## Rollback
All changes are uncommitted on top of phases 1–5; revert the touched files.
