# Phase 13: Documentation report

## Summary
Updated project documentation across design documents, user guides, contracts, and README to reflect all shipped features of the plan:
- **`docs/design/01-system-architecture.md` (Vietnamese):**
  - Updated §2 invariant row on progressive disclosure (digest-pinned read-only local snapshot and writable state directory, hub never executes skill code in MCP flow).
  - Clarified §6 progressive loading: `local` object and review-required behavior.
  - Updated §7.2 `runtime/` layout with `runtime/cache/skills/<id>@<d16>`, `runtime/envs/<id>@<deps16>`, `runtime/config/<id>/env` (0600), and `runtime/cache/doctor/<machine>/...` (cached doctor results, basis: terminal).
  - Documented §11 security: cooperative-not-adversarial content trust gate, local-folder adds trusted by design, CLI-only approval, and secret env permissions.
- **`docs/design/02-agent-hub-protocol.md`** (Vietnamese):
  - Updated §2.2 bootstrap instruction block to match `internal/hostintegration/bootstrap.go`.
  - Updated §3 protocol layers with `local` object structure, `live_checks`, and `_meta["io.skillhub/local_path"]`.
  - Added note in §4.3 specifying that task description must be in English.
  - Documented §5.2 `resolved` response `setup` object (states, `basis: terminal`, reason codes).
- **`docs/design/03-resolver-design.md`** (Vietnamese):
  - Added `topics`, `technologies`, `routing.examples`, and `routing.counter_examples` to §3 routing metadata example.
  - Documented metadata linting rules (`trigger_collision`, `generic_trigger`, `missing_examples`, `example_restates_trigger`, `near_duplicate`) and pair limit of 2,000 skills (`lint_pairs_skipped`).
  - Updated §4.2 FTS document schema with 7 columns and explicit BM25 weights `(0.0, 8.0, 5.0, 7.0, 1.0, 3.0, 2.0)`.
  - Documented §7 feature model: trigger feature with 0.9 example discount, counter-example folding into not_for, and technology matching with artifact score lift.
  - Documented §9 calibration outcome in one sentence with no plan IDs.
- **`docs/design/04-telemetry-reproducibility-evaluation.md`** (Vietnamese):
  - Added `skill.doctor_checked`, `transcript.tool_observed`, and server-observed `skill.loaded` (including blocked loads with `status: review_required`) to §3.2 core events.
  - Updated §3.4 outcome semantics with server-observed activation and attribution classes, unsolicited curator inspection, uncounted draft loads, and negative feedback deduplication.
  - Updated §4.2 retention (14-day raw, 180-day daily rollups).
  - Added `telemetry_daily_rollups` to §5 storage architecture.
  - Documented §7 routing evaluation leave-one-out case generation, no-skill suite, and gate thresholds.
  - Updated §9 funnel metric names and noted that named measurement cases are deferred to date windowing.
- **`docs/user-guide.md`** (English):
  - Updated "How the agent picks a skill" with local snapshot, state directory, env variables, preflight, and review_required.
  - Updated "What gets written" with host permission entries (`runtime/cache/skills`, `runtime/envs`, `runtime/config`) across Claude Code, Codex, and Gemini CLI.
  - Added new section "Run skills that need tools or secrets" with behavior matrix A–D, `skill doctor` (`basis: terminal`, exit codes), `skill env set|unset|list`, and Windows / Node ESM caveats.
  - Updated "Troubleshooting" for `review_required`, `setup_failed`, missing permissions, and expired `local.path`.
  - Added all new commands to "Command cheat sheet".
- **`docs/curating-skills.md`** (English):
  - Updated "Safety model" with content trust gate, CLI-only approval, and approval reset rules.
  - Updated "Review diagnostic facts" with `content_trust`, `changes_since_approval`, `git diff` commands, and `runtime_hints`.
  - Updated "Edit and improve a skill" with `--example`, `--counter-example`, and `--runtime-file`.
  - Added new section "Measure usage" (`skillhub telemetry funnel`, WebUI Usage tab, transcript import with privacy boundary).
  - Added new section "Check routing quality" (`skillhub eval routing`, metadata linting).
  - Updated "Command map".
- **`docs/contracts/error-codes.md`** (English):
  - Added `content_review_required` error code.
- **`README.md`**:
  - Mentioned `skillhub skill doctor <id>` in "Diagnose issues".

## Verification
- Scan for deprecated/stale terms (`approve-scripts`, `scripts_reviewed_digest`, etc.): 0 matches.
- Scan for internal plan/finding IDs: 0 matches.
- All commands and flags verified against CLI help.
- `make check` (PASS).
- `make web-check` (PASS).
