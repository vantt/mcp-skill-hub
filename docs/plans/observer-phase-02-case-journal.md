# Observer Phase 2: Case Journal

## Goal
Implement a local case journal to capture detailed resolution context (task description, operation, sanitized requests) exclusively for chains showing disagreement (overrides, reformulations, missing skills). This enables future analysis and synthesis without recording full conversations or persisting PII/secrets.

## Files
- `internal/telemetry/redactor.go`: Implementation of the redaction logic.
- `internal/telemetry/redactor_test.go`: Tests for the redactor.
- `internal/telemetry/cases.go`: Store implementation for cases.
- `internal/telemetry/cases_test.go`: Tests for case storage and limits.
- `internal/delivery/mcpserver/activation_tracker.go`: RAM storage for resolution data and trigger for case persistence.
- `internal/delivery/cli/telemetry_cases.go`: CLI command to view cases.
- `docs/design/04-telemetry-reproducibility-evaluation.md`: Documentation updates.

## Steps
1. **Redactor**: Build a standalone redactor (`redact-v1`) that strips secret patterns, normalizes paths (home to `~`, absolute repo paths), applies a length cap, and sanitizes fact values.
2. **Configuration**: Add an opt-in flag (off by default) to the telemetry configuration for the `redacted` content mode. Update `docs/design/04`.
3. **Tracker RAM**: Update `notedResolution` to hold the task description, operation, and the sanitized request.
4. **Case Store**: Implement a separate SQLite table or JSON store (e.g., `telemetry_cases`) with limits: 90 days retention, max ~500 cases total, and a daily cap. Hook this up to `skillhub telemetry purge`.
5. **Persistence Trigger**: In the activation tracker and usage chain logic, detect disagreements (override, after_no_skill, verified reformulation, needs_context -> resolved, rejected, repeated gaps) and persist the case to the separate store. Agreeing resolutions are dropped. Note: `scope_mismatch` is dropped as a distinct prior kind because it does not exist in the request model (in the agent-hub protocol it is a reason code under `kind: "rejected"`, recorded as `rejected` or `verified_reformulation`).
6. **CLI Viewing**: Implement `skillhub telemetry cases [--since] [--kind] [--json]` to view the cases.
7. **Isolation**: Verify case text does *not* leak into `skillhub telemetry export` or web JSON responses.

## Tests
- **Redactor**: Verify all secret classes are removed, paths normalized, and caps applied.
- **Persistence**: Verify no cases are written when the flag is off. Verify only disagreement chains trigger writes when on.
- **Limits**: Verify 90-day retention, 500 total cap, and daily caps are enforced.
- **Isolation**: Verify `telemetry export` and web JSON do not contain case text. Verify `purge` removes cases.

## Acceptance
- The redactor is a standalone, tested module.
- Cases are only persisted on disagreement and only when the opt-in flag is enabled.
- Case shape includes `catalog_snapshot`, `prior_verified`, top-k arrays, and `followup`.
- Limits and retention are strictly enforced.
- Case data is exclusively accessible via the CLI and never exported or sent to the web UI.
