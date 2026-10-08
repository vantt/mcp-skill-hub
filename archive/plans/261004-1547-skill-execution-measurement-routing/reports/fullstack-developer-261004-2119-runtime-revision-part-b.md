# Runtime revision Part B: report

`make check` passes (vet, lint, all packages). Nothing is committed.

## What changed
- **State dir for every trusted skill (controller addition).** `LocalSkill` gains `state_directory` and `env` (`SKILLHUB_SKILL_DIR`, `SKILLHUB_STATE_DIR`). They are set for every trusted exported skill, with or without a runtime block, so prose-only install (case B) has a place to install. `local.preflight` stays only for skills with a runtime block and keeps its own `env`/`state_directory` (same values). The deps key for a skill without a runtime block covers dependency manifests only (runtime is null in the hash). GC now keeps state dirs of all active trusted skills. Untrusted skills are unchanged: no path, no state dir.
- **B1 hints.** New `skillruntime.AnalyzeHints` (pure, reusable by metadata lint) returns `interpreters`, `dependency_manifests`, `absolute_install_paths` (file paths only), `missing_runtime_block`, `install_prose_detected`, and `install_cues` (cue names like `pip install` or `heading: setup`, never surrounding text). Reads at most 256 KiB per file. `skill review` exposes it as `runtime_hints` and prints a short "Runtime" section with a next step when anything is detected. Web golden `skill-review.json` regenerated (additive).
- **B2 runtime through the update flow.** `skill.UpdateInput.Runtime` (nil keeps, empty map removes, otherwise validated by `skillruntime.ParseSpec`), MCP `skill_update_preview.runtime`, CLI `skill edit --runtime-file <yaml>` (`{}` removes; edit-only). Invalid blocks return `invalid_request` with the reason in `suggested_action`. The request digest includes runtime. A runtime change alters the content digest, so a prior approval goes stale (tested through the update flow). Curator `SKILL.md` (both copies, version 1.4.0) gains a "Propose a runtime block" section.
- **B3.** Feedback reason `setup_failed` added to the telemetry vocabulary, event schema enum, MCP `skill_feedback` enum, and app error text. Rollup counts `feedback:setup_failed` once, on the primary event only (the utility event carries the same reason and is not double counted).
- **B4.** Host paragraph rewritten (four short paragraphs). It tells hosts to export `local.env`, run `check` in `working_directory`, treat prose install as setup (ask, install into `SKILLHUB_STATE_DIR`, never globally), use only SKILL.md for `review_required`, and report `setup_failed`. Marker version kept at v1. AGENTS.md, CLAUDE.md, GEMINI.md refreshed with `updateBootstrap`. The `skill_get` tool description mentions `local.env`/`local.state_directory`.

## Tests added
Hint detectors (each detector, name-only cues, binary skip); review hints at app and CLI level; no-runtime skill gets state dir and env, and a dependency file changes it; update-flow set, stale approval, remove, and invalid block; MCP runtime-only preview and invalid block; CLI `--runtime-file` set, invalid, remove, empty file, edit-only; `setup_failed` rollup and schema; host instruction expectations.

## Notes
- A repeat of an identical approval request replays the earlier proposal (existing idempotency), so re-approving the same digest after content returns to a previously approved state needs a fresh idempotency key. The test does this.
- The web UI type for the review payload was not touched (additive field).

Status: DONE
Summary: Part B and the state-directory addition are implemented with tests; `make check` passes.
Concerns/Blockers: none.
