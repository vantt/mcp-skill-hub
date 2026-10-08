# Phase 10: Routing eval command, corpus, CI gate report

## Summary
- Extracted `resolveWithin` from `ResolverService.Resolve` in `internal/app/resolver.go`:
  - Keeps identical resolution behavior, error handling stages, and telemetry in `Resolve`.
  - Enables in-memory catalog reuse (`*catalog.Handle`) and catalog decoration (leave-one-out) without telemetry pollution.
- Extracted `ParsePolicy` in `internal/resolver/sqlite_catalog.go` for policy parsing from JSON/YAML without database mutations.
- Implemented `RoutingEvalService` in `internal/app/routing_eval.go`:
  - Generates positive test cases from active skills' `routing.examples` (leave-one-out).
  - Generates counter test cases from active skills' `routing.counter_examples` (leave-one-out).
  - Evaluates external no-skill test cases (`schema_version: 1`, `expected.status: no_skill`).
  - Computes precision@1, recall, no-skill recall, no-skill precision, and false positive rate.
  - Verifies thresholds with strict gate pass/fail semantics.
- Added CLI command `skillhub eval routing` in `internal/delivery/cli/evaluation_routing.go`:
  - Flags: `--workspace <path>`, `--no-skill <file>`, `--policy <file>`, `--min-precision <F>`, `--min-recall <F>`, `--min-no-skill-recall <F>`, `--max-fpr <F>`, `--json`.
  - Exit codes: 0 (pass / no thresholds), 1 (threshold failure), 2 (invalid request).
  - Updated CLI command dispatch and help in `internal/delivery/cli/help.go`.
- Created routing evaluation corpus:
  - `testdata/routing/examples-v1.json`: examples (3–5) and counter-examples (1–3) for all 42 golden-v1 skills.
  - `testdata/routing/no-skill-v1.yaml`: 32 diverse no-skill requests.
  - Backfilled `routing.examples` and `routing.counter_examples` into `testdata/evaluation/workspace-overlay` skills.
- Implemented CI gate test `TestRoutingEvalGate` in `internal/app/routing_eval_gate_test.go`:
  - Materializes workspace from golden-v1 skills plus examples overlay.
  - Enforces thresholds from `testdata/routing/gate-v1.json`: precision@1 >= 0.55, recall >= 0.56, no-skill recall >= 0.82, FPR <= 0.16.
  - Runtime measured at ~2.2s, satisfying the < 10s budget.
  - Recorded initial baseline run in `reports/routing-eval-baseline-report.md`.

## Verification
- Unit & integration tests:
  - `go test -count=1 -run 'Resolve|Continuity' ./internal/app/` (PASS)
  - `go test -count=1 -run RoutingEval ./internal/app/` (PASS)
  - `go test -count=1 -run Routing ./internal/delivery/cli/` (PASS)
  - `go run ./cmd/skillhub help eval` (PASS: lists `routing`)
  - `make check` (go vet, golangci-lint 0 issues, full test suite across all 24 packages) (PASS)
- Acceptance criteria:
  - Precision@1, recall, no-skill recall/precision, and false positive rate computed and verified.
  - Zero telemetry leakage: `telemetry.db` remains untouched during evaluation.
  - CI gate test runs and passes in `make check`.
