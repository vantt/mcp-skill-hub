# Phase 10: Routing eval command, corpus, CI gate report

## Summary
- Extracted unexported `resolveWithin(ctx, root, handle, request, decorate)` in `internal/app/resolver.go`, allowing callers with nil telemetry and custom catalog decorators to execute resolutions without altering existing telemetry or public contracts.
- Extracted `ParsePolicy(digest string, contentJSON []byte) (Policy, error)` from `LoadPolicy` in `internal/resolver/sqlite_catalog.go` to support loading custom policy files without modifying workspace files.
- Implemented `app.EvaluateRouting` in `internal/app/routing_eval.go`:
  - Iterates over all active skills (excluding `system-curator`).
  - Evaluates positive examples using leave-one-out catalog decoration (`leaveOneOutCatalog`), verifying status is `resolved` with primary matching the target skill.
  - Evaluates counter-examples using leave-one-out catalog decoration, verifying primary does not match the target skill.
  - Loads and evaluates no-skill test cases from `--no-skill` suite file, verifying status is `no_skill`.
  - Calculates Precision@1, Recall, No-Skill Recall, No-Skill Precision, and False Positive Rate.
  - Telemetry isolation: runs with `Telemetry: nil` and emits no resolution events to `telemetry.db`.
- Implemented `skillhub eval routing` CLI command in `internal/delivery/cli/evaluation_routing.go`:
  - Supports `--workspace`, `--no-skill`, `--policy`, `--min-precision`, `--min-recall`, `--min-no-skill-recall`, `--max-fpr`, `--json`.
  - Exits 0 on success/pass, 1 on any failed threshold, 2 on invalid arguments.
  - Updated `help eval` in `internal/delivery/cli/help.go`.
- Generated evaluation corpus in `testdata/routing/`:
  - `examples-v1.json`: 3–5 examples and 1–3 counter-examples across all 42 golden-v1 skills.
  - `no-skill-v1.yaml`: 34 no-skill cases covering chit-chat, trivial edits, and out-of-catalog topics.
  - `gate-v1.json`: gating thresholds based on the baseline run.
  - Backfilled examples and counter-examples into the three `workspace-overlay` test skills.
- Implemented CI gate test `TestRoutingEvalGate` in `internal/app/routing_eval_gate_test.go`:
  - Materializes workspace from golden skills and examples overlay.
  - Executes evaluation in 733 ms (well below 10 s CI target).
  - Asserts thresholds from `gate-v1.json`.
- Documented baseline measurements in `reports/routing-eval-baseline-report.md`.

## Verification
- Unit & regression tests:
  - `go test -count=1 -run 'Resolve|Continuity' ./internal/app/` (PASS)
  - `go test -count=1 -run RoutingEval ./internal/app/` (PASS)
  - `go test -count=1 -run Routing ./internal/delivery/cli/` (PASS)
  - `go run ./cmd/skillhub help eval` (PASS)
  - `go test -v -count=1 -run RoutingEvalGate ./internal/app/` (PASS, duration 733 ms)
