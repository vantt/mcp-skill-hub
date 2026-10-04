---
title: "Skill execution, measurement funnel, and routing quality"
description: "Serve digest-pinned local skill snapshots with runtime preflight and script trust, measure the recommend-to-activate funnel server-side with long-lived rollups and transcript ground truth, and raise routing quality with examples, a generated eval gate, lint, and calibration."
status: pending
priority: P1
effort: 100h
branch: main
tags: [feature, mcp, resolver, telemetry, security, web]
blockedBy: []
blocks: []
created: 2026-10-04
---

# Skill execution, measurement funnel, and routing quality

## Overview

Three user-accepted workstreams, executed as 13 sequential phases:

- **A. Skill execution.** Activation exports a read-only, digest-verified snapshot of the skill folder into `runtime/cache/skills/` and returns its absolute path, a runtime preflight plan, and a trust verdict for scripts. A new `runtime` manifest block, `skillhub skill doctor`, and a `setup` annotation on resolver recommendations complete it. This intentionally retires the "MCP-text-only delivery" principle; the hub still never executes skill code inside the MCP flow.
- **B. Measurement funnel.** Server-side load events on `skill_get`, `skills/get`, and `resources/read`, attributed to the recommending resolution through a per-MCP-session tracker, aggregated into 180-day daily rollups, reported by `skillhub telemetry funnel` and a Usage panel in the WebUI, and cross-checked by an explicit Claude Code transcript import.
- **C. Routing quality.** `routing.examples` / `routing.counter_examples`, use of `technologies` / `topics`, a generated routing eval (`skillhub eval routing`) gated in `make check`, metadata lint, evidence-based calibration, and an English-normalization instruction for multilingual requests.

## Verified starting facts (re-checked 2026-10-04)

- The activation surface agents actually use is the `skill_get` tool (`internal/delivery/mcpserver/skill_tools.go:135`), confirmed by local Claude Code transcripts (`mcp__skillhub__skill_get` with `skill_id`). `skills/get` and `resources/read` are the standards path (`server.go:144-156`). None records telemetry today.
- `skill_get` returns `app.SkillDetail` (`internal/app/skill_detail.go:36`) built from the canonical working tree in any lifecycle state; it is shared with CLI and WebUI, so local-snapshot data must be added in an MCP-only wrapper.
- Distributed URIs reject query strings (`internal/app/distribution.go` `parseDistributedURI`), so a resolution ID cannot ride in the URI without breaking the contract.
- Skill FTS ranking is `bm25(skill_fts,0.0,8.0,5.0,7.0)` over columns `(skill_id,name,aliases,description,triggers)` (`internal/resolver/sqlite_catalog.go:99`, `internal/catalog/schema.go:131`). Only four weights are supplied, so **triggers rank at the default weight 1.0, not 5**; aliases get 5. FTS only gates candidacy; scoring is token overlap in `scoreSkill` (`internal/resolver/evidence.go:256`).
- **Latent data-loss bug:** `skill.Manager.PreviewUpdate` replaces the whole `routing` map with four fields (`internal/skill/lifecycle.go:254-255`, `routingDocument` at `:630`), silently dropping `requirements`, `distinguish_from`, `supporting`, `equivalent_to`, and `boosts` on every routing edit. New routing fields would be wiped the same way, so Phase 1 fixes it.
- A synthetic 42-skill / 150-case golden corpus already exists (`testdata/resolver/golden-v1.json`, used by `internal/resolver/resolver_test.go:767` and `internal/evaluation/evaluation_test.go:172`) with a calibration grid in `testdata/resolver/evaluation-policy-v1.json`.
- `system-skills/` contains only `system-curator`, which the resolver excludes by design (`reservedSystemSkillID`, `sqlite_catalog.go:21`).
- Claude Code transcripts write one JSONL line per content block, so several lines share one `message.id` (measured: 32 of 119 tool-calling message IDs span multiple lines). Dedupe must use the `tool_use.id`. Subagent transcripts live in `<project>/<session>/subagents/*.jsonl`.

## Key decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | **Correlation = per-MCP-session in-memory tracker** keyed by `*mcp.ServerSession` (available on every tool, resource, and custom-method request in go-sdk v1.8.0). It remembers recent resolutions (2 h TTL, 32 per session, 256 sessions LRU). Each load is attributed at write time as `recommended`, `supporting`, `override`, `after_no_skill`, `after_needs_context`, or `unsolicited`. | URIs stay standard and cacheable; hosts cannot add params to `ReadMcpResource`; write-time classification lets daily rollups report acceptance far beyond the 14-day raw window. |
| D2 | **Cache layout:** `<workspace>/runtime/cache/skills/<skill-id>@<first 16 hex of manifest digest>/` (full) or `...@<d16>.restricted/` (scripts withheld). Marker `.skillhub-snapshot.json` written last; built in `runtime/cache/skills/.staging/<random>/`, then renamed. Skill files are `0444` (executables `0555`); directories stay writable so setup artifacts (`node_modules`, `.venv`) can live beside them. Reuse re-verifies every file's size and digest. GC removes snapshots that do not match a current active manifest and are older than 24 h. | Atomic and idempotent across concurrent MCP processes; never touches the project or the live Git tree; digest-keyed so stale snapshots cannot be served. |
| D3 | **Trust default:** a skill is *third-party* when `provenance.origin.kind` is `github`/`git` or `provenance.source_id` is set. Everything else (`skill create`, `skill add` from a local folder) is trusted. A third-party skill that has executable resources or `runtime` commands needs `quality.scripts_reviewed_digest` equal to its **execution digest** (sha256 over sorted executable resources `{path,digest}` plus the canonical JSON of the `runtime` block). Approval is human-only (`skillhub skill edit <id> --approve-scripts <digest>`), never through MCP. | Agents cannot self-approve; the review is bound to the exact script and command content; local authorship already implies the user can run it. |
| D4 | **No execution in the MCP flow.** Activation and resolve may run only non-executing checks (`runtime.GOOS`, `exec.LookPath`, env-var presence). Version probes and the `check` command run only in `skillhub skill doctor`; results are cached at `runtime/cache/doctor/<machine-id>/<skill-id>@<runtime-fingerprint16>.json`. | Keeps the V1 safety boundary while giving hosts a precise preflight plan. |
| D5 | **`setup` annotation is post-ranking**, applied in `app.ResolverService` after `engine.Resolve`, as an optional `setup` object on `primary`/`supporting`. Ranking never sees machine state. | Resolver stays deterministic and replayable; unknown setup never hides a skill. |
| D6 | **Rollups** live in `telemetry.db` table `telemetry_daily_rollups(day, skill_id, metric, count)`, updated in the same transaction as the raw insert through one shared helper used by all three insert sites, only when the raw row was actually inserted. UTC days, 180-day retention, cleared by purge. | No background job; exactly-once per event ID; dead-skill detection over 180 days. |
| D7 | **Examples fold into existing features**: trigger feature = max(trigger overlap, 0.9 × example overlap); not-for feature = max(not_for overlap, counter-example overlap); new FTS columns `examples` and `keywords` (topics + technologies). No new policy weight. | No `config/recommendation.yaml` schema change, so existing custom policies keep working. |
| D8 | **Routing eval is leave-one-out**: when a case comes from example *i* of skill *S*, that example is removed from *S* for scoring. | Without it the gate measures memorization, not routing. |
| D9 | **Forgentx "case" windows are deferred.** Funnel supports `--since/--until`; named cases are not built. | Rollups are day-granular; sub-day before/after windows would need raw events (14-day limit). Not cheap to do correctly. |

## Phases

| # | Phase | Depends on | Effort | Status |
|---|---|---|---|---|
| 1 | [Manifest fields and routing-merge fix](./phase-01-manifest-fields-and-routing-merge.md) | — | 6h | Done |
| 2 | [Telemetry events and daily rollups](./phase-02-telemetry-events-and-daily-rollups.md) | — | 8h | Done |
| 3 | [skillruntime package: runtime, trust, doctor checks](./phase-03-skillruntime-package.md) | 1 | 8h | Done |
| 4 | [Local snapshot export and activation response](./phase-04-local-snapshot-and-activation-response.md) | 1, 3 | 10h | Done |
| 5 | [Doctor CLI, setup annotation, host instructions](./phase-05-doctor-setup-annotation-host-instructions.md) | 2, 3, 4 | 8h | Done |
| 5a | [Runtime revision: skill-level trust, state dir, platform-only checks, setup guidance](./phase-05a-runtime-revision.md) | 1–5 | 10h | Done |
| 5b | [Runtime hardening: host permissions, unapproved content, review diff](./phase-05b-runtime-hardening.md) | 5a | 8h | Done |
| 6 | [Server-side activation tracking](./phase-06-server-side-activation-tracking.md) | 2, 4, 5 | 8h | Pending |
| 7 | [Funnel aggregation, CLI, WebUI Usage panel](./phase-07-funnel-cli-and-web-usage.md) | 2, 6 | 10h | Pending |
| 8 | [Claude Code transcript import](./phase-08-transcript-import.md) | 2, 7 | 8h | Pending |
| 9 | [Resolver routing features](./phase-09-resolver-routing-features.md) | 1 | 8h | Pending |
| 10 | [Routing eval command, corpus, CI gate](./phase-10-routing-eval-and-gate.md) | 9 | 10h | Pending |
| 11 | [Metadata lint](./phase-11-metadata-lint.md) | 1, 9 | 6h | Pending |
| 12 | [Calibration with recorded evidence](./phase-12-calibration.md) | 10 | 4h | Pending |
| 13 | [Documentation](./phase-13-documentation.md) | 1–12 | 6h | Pending |

Phases run sequentially in the listed order. Files touched by more than one phase (`internal/delivery/cli/help.go`, `internal/delivery/mcpserver/{server.go,skill_tools.go,types.go}`, `internal/telemetry/events.go`) are owned by one phase at a time; each phase file lists its exact files.

## Data flow (end state)

```text
skill_resolve ──► ResolverService.Resolve ──► engine (ranking; examples/tech features)
      │                     └─► setup annotation (manifest runtime + live checks + doctor cache)
      ├─► telemetry: resolution.* (+ setup_state)  ──► raw (14d) + rollups (180d)
      └─► session tracker: note resolution
skill_get / skills/get / resources/read
      ├─► SnapshotService.Ensure ──► runtime/cache/skills/<id>@<d16>[.restricted]/
      ├─► response: local{path,status,reason_codes,resources,preflight}
      └─► tracker.attribute ──► telemetry: skill.loaded(basis=server-observed, attribution)
skillhub skill doctor ──► checks + check cmd in snapshot dir ──► doctor cache + skill.doctor_checked
skillhub telemetry import-transcripts ──► transcript.tool_observed (dedupe by tool_use.id)
skillhub telemetry funnel / GET /api/v1/skills/{id}/usage ──► rollups ⨝ active catalog
skillhub eval routing ──► cases from examples/counter_examples (+ no-skill file) ──► metrics, gate
```

## Backwards compatibility

- New manifest fields are optional. Older binaries reject unknown keys in `skill.meta.yaml`, so a workspace that adopts them needs this binary or newer (documented in Phase 13).
- The catalog `DerivedSchemaVersion` goes from 2 to 3 (new FTS columns). Old generations report `StateIncompatible` (`internal/catalog/open.go:79`) and are rebuilt by `EnsureCatalog` at MCP startup or by `skillhub rebuild`.
- `telemetry.db` gets an additive table (`CREATE TABLE IF NOT EXISTS`); older binaries ignore it.
- MCP output changes are additive: a `local` object on `skill_get` and `skills/get`, `_meta` on `resources/read`, and an optional `setup` on resolver recommendations (committed response schema updated, `schema_version` stays `"1"`).
- The routing-merge fix changes behavior: routing edits now preserve fields they do not name. Nil slices mean "keep"; explicit empty lists clear.

## Rollback

Each phase is one or more focused commits and can be reverted with `git revert`. Disposable state needs no migration: `rm -rf runtime/cache/skills runtime/cache/doctor` clears snapshots and doctor results; the rollup table is harmless to older binaries; reverting Phase 9 brings back `DerivedSchemaVersion` 2 and the catalog rebuilds automatically.

## Acceptance criteria (whole plan)

- [ ] `skill_get` and `skills/get` for an active, servable skill return an absolute `local.path` inside `runtime/cache/skills/` whose files match the manifest digests; repeat calls reuse the snapshot; tampered snapshots are rebuilt.
- [ ] An unreviewed third-party skill with scripts gets a `.restricted` snapshot without executable files and reason code `scripts_review_required`; after `skill edit --approve-scripts <digest>` it gets the full snapshot.
- [ ] `skillhub skill doctor <id>` reports platform, bins (with version constraints), env presence, and `check` results, exits 0 or 1, and caches per (skill, runtime fingerprint, machine); resolver recommendations carry `setup.state`.
- [ ] Every entrypoint load emits one `skill.loaded` event with `basis=server-observed` and a correct attribution; `skillhub telemetry funnel --json` reports every metric listed in Phase 7 for any window up to 180 days.
- [ ] `skillhub telemetry import-transcripts --project <dir>` records only tool, skill ID, timestamp, and session hash; a re-import adds nothing.
- [ ] `skillhub eval routing` prints precision@1, recall, no-skill precision/recall, and false-positive rate, and exits non-zero below thresholds; the gate test runs in `make check`.
- [ ] `skillhub validate` and `skillhub skill review` report collision, generic-trigger, missing-example, and near-duplicate warnings.
- [ ] Any policy change is backed by a report in `reports/`; golden-v1 held-out gates still pass.
- [ ] `make check` and `make web-check` pass; docs match shipped behavior.

## Deferred

- Named measurement cases (forgentX `case` open/close windows): see D9.

## Open questions

1. `system-skills/` holds only `system-curator`, which the resolver excludes by design, so it gets no routing examples. The plan backfills examples into the skills that are actually routed in this repository: the golden-v1 corpus (through a new overlay file) and the three `testdata/evaluation/workspace-overlay` skills. If you meant your own workspace's skills, that is content work outside this repository.
2. D3 treats `skill add` from a local folder as trusted, even when the folder is a clone of someone else's repository. If you want local-folder imports gated as well, it is a one-line change to the third-party predicate in Phase 3.
