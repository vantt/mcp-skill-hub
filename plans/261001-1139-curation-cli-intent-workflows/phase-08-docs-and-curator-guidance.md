---
phase: 8
title: "Documentation and curator guidance"
status: completed
priority: P1
effort: "2-3d"
dependencies: [6, 7]
---

# Phase 8: Documentation and curator guidance

## Context Links

- [Current curation guide](../../docs/curating-skills.md)
- [Current user guide](../../docs/user-guide.md)
- [System curator](../../system-skills/curator/SKILL.md)
- [Accepted redesign synthesis](../reports/curation-ux-cli-simplification-report.md)

## Objective

Make the intent-first workflows the beginner path, preserve advanced/recovery commands, align durable architecture contracts with implemented behavior, and teach the system curator to choose one focused application workflow rather than primitive orchestration.

## Documentation Contract

Beginner journey:

```text
status → skill add OR skill create → skill review/edit → activate
status → source watch → source check → distill/inbox
```

Advanced compatibility journey remains documented but is not the default tutorial:

```text
source capture → triage → confirm → source import
pin-explicit CLI confirmation
```

Documentation must distinguish current canonical state from served published state; explain draft/published/servable/routing eligibility; and state explicitly that watch is not a daemon, add does not watch, review does not approve, and no command commits Git. Local folder add is interactive CLI-only in this release; MCP add accepts public GitHub locators.

## Exclusive File Ownership

Modify:

- `/home/vantt/projects/mcp-skill-hub/README.md`
- `/home/vantt/projects/mcp-skill-hub/docs/user-guide.md`
- `/home/vantt/projects/mcp-skill-hub/docs/curating-skills.md`
- `/home/vantt/projects/mcp-skill-hub/docs/design/05-curation-lifecycle.md`
- `/home/vantt/projects/mcp-skill-hub/docs/design/06-source-learning-and-distillation.md`
- `/home/vantt/projects/mcp-skill-hub/docs/design/07-storage-and-mutation-model.md`
- `/home/vantt/projects/mcp-skill-hub/docs/contracts/error-codes.md`
- `/home/vantt/projects/mcp-skill-hub/docs/contracts/result-envelope.md`
- `/home/vantt/projects/mcp-skill-hub/docs/contracts/ux-fixtures.md`
- `/home/vantt/projects/mcp-skill-hub/system-skills/curator/SKILL.md`

No other phase edits these files. `docs/mcp-compatibility-matrix.json` belongs exclusively to Phase 7 because it records executed evidence.

## Implementation Steps

1. Rewrite curation quick paths around `skill add`, `skill create`, `skill review`, `skill edit`, `source watch`, and `source check`. Every command example must be runnable and use actual accepted flags.
2. Move capture/triage/import into an “advanced intake and recovery” section without marking them removed. Document top-level `check` as a supported compatibility spelling.
3. Explain locator behavior, explicit multi-skill selection, fresh advertised refs, local CLI snapshot privacy, MCP local-path refusal, companion inventory, license warnings, conflict/idempotency behavior, and no local watch.
4. Document preview/confirm differences: short proposal-ID command for humans; exact three pins for MCP/automation; `--yes` confirms only a fresh preview.
5. Document digest-pinned editor conflict semantics, explicit blind replacement compatibility, 24-hour bounded recovery artifacts, and review as diagnostic facts rather than approval.
6. Document resource-verified fallback: unchanged skills may remain available with degraded diagnostics; changed/deleted resources are unavailable and historical bytes are never guessed. Explain mutation blocking, fresh-clone semantics, degraded MCP startup, and basis-aware state fields.
7. Add `validate --staged` and a hook-manager-neutral one-line example. State that it reads literal index blobs without checkout filters, does not mutate index/worktree, and no hook installer exists.
8. Update architecture/storage/error/result contracts to match actual fields and typed codes. Link to machine-owned schemas instead of copying them.
9. Update system curator routing: use focused add/watch/review workflows; use CLI/user-mediated local add rather than passing raw local paths to MCP; ask only on ambiguity; retain primitives for intake/recovery.
10. Reconcile links, examples, CLI help, MCP tool names, state basis, and schemas against landed Phases 6/7. Return code discrepancies to their owners rather than documenting incorrect behavior.

## Todo

- [x] Replace beginner multi-step source import with intent-first add/watch paths.
- [x] Preserve and clearly label advanced compatibility commands.
- [x] Document CLI-only local add, MCP refusal, state basis, resource fallback, editor recovery, staged validation, and safety.
- [x] Update durable design/error/result contracts.
- [x] Update system-curator workflow selection.
- [x] Verify all links and literal command/tool examples against landed adapters.

## Success Criteria

- A newcomer can add one GitHub or local skill through CLI, review/edit it, and activate it without candidate/source IDs.
- A connected agent can add a public GitHub skill but cannot read an arbitrary host path through MCP.
- A newcomer can watch/check a repository without assuming import or a daemon.
- Every documented command is accepted by CLI help and every named MCP tool exists.
- No document claims historical fallback bytes, hook installation, local watch, add-and-watch, auto-activation, auto-commit, or semantic approval.
- Existing advanced users can find capture/triage/import and pin-explicit confirmation behavior.

## Verification

```bash
go test ./internal/delivery/cli -run 'Test.*(Help|Usage|Onboarding)'
go test ./internal/systemskills
```

Additionally:

- search all owned docs for superseded beginner sequences and old state claims;
- run every new CLI snippet against the Phase 6 smoke binary with disposable paths;
- validate Markdown links and verify system-curator tool names against MCP `tools/list`.

## Risks and Security

- Do not include real local paths, proposal IDs, tokens, credentials, or private repository examples.
- Avoid promising legal conclusions from license detection.
- Keep architecture facts sourced to code/schema paths and observed tests.

## Next Step

Phase 9 performs the final command/tool/doc consistency sweep.