# Complete Skill Hub V1 phases 13–16

## Outcome

Completed the remaining V1 plan in the working tree: bundled curator and host bootstrap, disposable telemetry and reproducible evaluation, explicit schema migration and release hardening. Evaluated the post-core gate and added no optional capability because current evidence does not justify one.

## Durable decisions

- `system-curator` is an immutable reserved bundled skill; host files are projections and never canonical inputs.
- Claude Code is the only V1 stock client with executed end-to-end curation evidence. Codex and Gemini projections remain best-effort until blocked authentication/trust checks can run.
- Telemetry remains content-free by default, disposable, path-confined, and behaviorally non-authoritative. Promotion produces human-reviewed drafts only.
- Evaluation and replay pin exact retained generations and fail closed; undefined metrics remain distinct from measured zero.
- Canonical schema upgrades require explicit preview/confirm and normal WAL recovery. Derived schema changes may rebuild only after canonical and recovery checks pass.
- Release installation authenticates the tag-bound Sigstore signature on the checksum manifest before trusting archive hashes. Tag pushes are the only installable publication route; manual dispatch is draft evidence only.
- Vector retrieval, LLM fallback, additional adapters, deep-dive/consult, and remote tenancy remain outside V1 because current corpus and CLI evidence show no measured need.

## Verification

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- Go formatting and `git diff --check`
- Installer lifecycle, including streamed install, authentication, upgrade rollback, and safe uninstall
- Native bounded fuzzing for canonical, mutation, and MCP input surfaces
- Deterministic p50/p95/p99 performance gates
- Plan validation (`ak plan validate ... --json` returned `valid: true`)
- Independent final security/release review: GO, no critical/high static blockers
- Claude Code 2.1.284 isolated end-to-end read-only `Curate my Skill Hub` run

## Pending external evidence

The first real version tag must exercise hosted GitHub OIDC, Sigstore transparency, GitHub artifact attestations, and publication. Codex end-to-end evidence requires local reauthentication; Gemini requires an explicitly trusted project folder. These limitations are recorded in the compatibility matrix and are not represented as passing evidence.
