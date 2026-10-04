---
title: "Phase 2: Source attachment on add and import"
status: todo
---

# Phase 2: Source attachment on add and import

## Context

- Plan: [plan.md](./plan.md) (D1, D2, D11). Depends on phase 1 (`buildGitOrigin`, `RevisionAt`).
- Read first: `internal/app/upstream_origin.go` (phase 1), `internal/app/skill_add.go`, `internal/app/source_import.go`, `internal/app/source_watch.go:168-320` (record construction, `deriveSourceID` at line 460, `sanitizeSourceID` at line 490), `internal/app/source.go:625-682` (`readSourceRecords`), `internal/delivery/cli/skill_add.go:94-135`.

## Overview

A skill vendored from a repository is always attached to that repository's source. `skill add` creates or reuses the source in the same atomic write set and sets `provenance.source_id`. `source import` writes a full `provenance.origin`, reads the ref's current commit, and stops writing legacy `role: origin` links.

## Requirements

1. `ensureUpstreamSource(ctx, root, adapter, repository, ref, commit)` returns the matching or new source record, an optional `mutation.Change` (only when new), and `created bool`.
   - Match: an existing record with `adapter: git`, `sameRepository(record.Locator.Repository, repository)` and `record.Locator.Ref == ref`. `sameRepository` normalizes both URLs: lowercase scheme and host; for host `github.com` also lowercase the path (GitHub owner/repo names are case-insensitive); trim one trailing `/` and then one trailing `.git`. Other hosts keep path case.
   - New ID: `sanitizeSourceID(owner + "-" + repo)` from the repository URL path (fallback `deriveSourceID(repository, "")`), truncated to 64 characters. If that ID is already used by a source for a different repository or ref, or by any skill ID (`listWorkspaceSkillIDs`), append `"-" + sanitizeSourceID(ref)`; if still taken, append `-2`, `-3`, ... (canonical IDs share one namespace, `internal/canonical/canonical.go:141-147`).
   - The new record carries `purpose: upstream` (Requirement 5).
   - New record: `SchemaVersion 1`, `Adapter "git"`, `Locator{Repository, Ref, Path: ""}`, `Status "watching"`, `Identity` from `adapter.Identify`, `License` from identity, `Trust{Source: "community"}`, `Monitoring{Enabled: true, Cadence: "weekly"}` (default pending user decision, Q4), default `Limits` as in `buildSourceWatchRecord`, `CurrentRevision` = `RevisionAt(Locator{Repository, Ref, Path: ""}, commit)`. Validate with `sourcepkg.ParseRecord(mustYAML(record))`.
2. `skill add` from GitHub/Git: every new skill's `skill.meta.yaml` has `provenance.source_id: <id>`; the write set contains `sources/catalog/<id>.yaml` only when the source is new. `SkillAddProposal` and `SkillAddResult` gain `UpstreamSource *UpstreamSourceRef \`json:"upstream_source,omitempty"\`` with `SourceID string \`json:"source_id"\`` and `Created bool \`json:"created"\``. Local adds: field absent, no source.
3. `source import` from a source with `adapter: git`: reads the ref's current commit (`adapter.CurrentRevision` with the record's locator) instead of requiring `record.CurrentRevision`; writes `provenance: {created_by: source_import, source_id, origin: buildGitOrigin(...)}` with `origin.kind: github` when the repository host is `github.com`, else `git`, and `origin.path = repoRelativeSkillPath(record.Locator.Path, item.SkillDir)`; writes no `sources/skills/LINK-*` file and no legacy `revision`/`path` keys. The source record is not modified. Sources with other adapters (`filesystem`, `immutable-http`, `living-http`) keep today's provenance keys (`created_by`, `source_id`, `revision`, `path`) and get no origin block; they also stop writing the `role: origin` link, because `provenance.source_id` already records the edge. Such skills are linked to their source but are not upstream-tracked.
4. CLI `skill add` human output names the upstream source (samples below) and no longer prints `Watching: off`.
5. **Source purpose marker.** `sourcepkg.Record` gains `Purpose string \`yaml:"purpose,omitempty" json:"purpose,omitempty"\``; `ParseRecord` accepts only `""` or `"upstream"`; `schemas/source-record.schema.json` adds `"purpose": {"enum": ["upstream"]}`. Absent means the source was created by watch/triage (learning, legacy behavior). Only `ensureUpstreamSource` writes `upstream`. Phase 3 uses it to decide which sources skip canonical revision writes, so a watched source keeps its distill pipeline even after skills are imported from it.
6. **Confirm through the stored proposal.** `skillhub skill confirm <proposal>` reaches `ConfirmSkillAddProposal` with only the stored write set (`internal/app/skill_add.go:863-893`, artifact fields at `:985-1000`). It re-derives `UpstreamSource` from that write set: `SourceID` from `provenance.source_id` in any `skill.meta.yaml` change, `Created` = the write set contains `sources/catalog/<id>.yaml`. No artifact field is added.

## Related code files

Create:
- `internal/app/upstream_source.go` — `sameRepository`, `upstreamSourceID`, `ensureUpstreamSource`, `UpstreamSourceRef`.
- `internal/app/upstream_source_test.go` — table tests for `sameRepository` and ID derivation.

Modify:
- `internal/app/skill_add.go` — attach the source (preview builds the change; `buildSkillAddChanges` writes `source_id`); add the struct fields.
- `internal/app/source_import.go` — fresh revision, origin block, no link (lines 109-260).
- `internal/delivery/cli/skill_add.go` — preview/result lines.
- `internal/source/records.go` (`Purpose` field and validation), `schemas/source-record.schema.json`, `schemas/embed_test.go` (one `purpose` case if the schema test enumerates record fields).
- Tests: `internal/app/skill_add_test.go` (extend `TestSkillAddRemoteGitRealAdapter`), `internal/app/source_import_test.go` (update link/revision assertions at lines 168-182, 400, 434), `internal/delivery/cli/skill_add_test.go` (render assertion).

Do not modify any other file.

## Implementation steps

### Task 2.1 — Source matching helper
- Steps: implement `internal/app/upstream_source.go` per Requirement 1 and the `Purpose` field per Requirement 5. Table test cases: `https://github.com/A/B.git` equals `https://github.com/a/b/`; `https://gitlab.example/A/B` differs from `https://gitlab.example/a/b`; different owner differs; ID `anthropics-skills` for `https://github.com/anthropics/skills`; collision with a different ref yields `anthropics-skills-v1-2` for ref `v1.2`; a skill named `anthropics-skills` forces the suffix; second collision yields `-2`. `ParseRecord` rejects `purpose: other`.
- Verify: `go test ./internal/app/ -run TestUpstreamSource -count=1` exits 0 and prints `ok`.

### Task 2.2 — `skill add` attaches the source
- Steps:
  1. In `PreviewSkillAdd`, after capture and before `buildSkillAddChanges`, when `captured.origin.Kind != "local"`, call `ensureUpstreamSource` with the captured repository, ref (`origin.Ref`, or the commit when the ref is empty), and commit.
  2. Pass the source ID into `buildSkillAddChanges`; add `"source_id": id` to the `provenance` map (line 575). Append the source change (when new) to `changes` and its path to `diffAdded`.
  3. Fill `UpstreamSource` on the proposal; copy it to the result in `ConfirmSkillAdd`.
  4. Implement Requirement 6 in `ConfirmSkillAddProposal`.
  5. Extend `TestSkillAddRemoteGitRealAdapter`: add skill `a` and confirm it through `SkillService.DispatchConfirmProposal` (the stored-proposal path), then add skill `b` from the same file:// repository (confirm in memory). Assert exactly one file in `sources/catalog/` with `purpose: upstream`, both metas contain `source_id: <that id>`, the first result has `UpstreamSource.Created == true`, the second `false`, and `canonical.Validate(root)` returns no issues.
- Verify: `go test ./internal/app/ -run 'TestSkillAdd' -count=1` exits 0 and prints `ok`.

### Task 2.3 — `source import` writes origin, no link
- Steps: implement Requirement 3 in `PreviewSourceImport`; keep conflict skipping and license warnings unchanged. Update the assertions in `internal/app/source_import_test.go`: no `sources/skills/LINK-*` path in `preview.Diff.Added` or on disk; meta contains `source_id: <id>`, `origin:` with `commit: <stub current revision>` and `path:` equal to the repository-relative folder. Keep every other assertion.
- Verify: `go test ./internal/app/ -run 'TestSourceImport' -count=1` exits 0 and prints `ok`.

### Task 2.4 — CLI output
- Steps: in `writeSkillAddPreview` replace the line ``"%s, %s. Origin retained. Watching: off. Agent use: off."`` with ``"%s, %s. Origin retained. Agent use: off."`` and, when `preview.UpstreamSource != nil`, add one line per sample below. In `writeSkillAddResult` replace `Watching: off.` with `Upstream: <id>.` when attached and drop it otherwise. Add a render test that builds a `SkillAddProposal` with `UpstreamSource{SourceID:"anthropics-skills", Created:true}` and asserts the exact upstream line.
- Verify: `go test ./internal/delivery/cli/ -run 'TestSkillAdd' -count=1` exits 0 and prints `ok`.

### Task 2.5 — Gate
- Verify: `make check` exits 0.

## Todo

- [ ] Task 2.1 matching helper
- [ ] Task 2.2 `skill add` attachment
- [ ] Task 2.3 `source import` origin
- [ ] Task 2.4 CLI output
- [ ] Task 2.5 `make check`

## Success criteria

- One source per repository and ref; every vendored skill names it in `provenance.source_id`.
- No new write path produces a source without a linked skill or a `role: origin` link.

## UX acceptance

`skillhub skill add https://github.com/anthropics/skills --skill pdf` (new source):

```text
Add pdf as a draft from https://github.com/anthropics/skills, skills/pdf at 3f9c2a1b4d5e.
7 files, 48 KB. Origin retained. Agent use: off.
Upstream: tracked by new source anthropics-skills (checked weekly by `skillhub check`; nothing runs in the background).
No collection files changed.
Next: skillhub skill confirm PROP-…
```

Same command when the source exists: the upstream line reads `Upstream: tracked by source anthropics-skills.`

Result line: `Draft pdf added. Agent use: off. Upstream: anthropics-skills. Changes are not committed.` followed by `Next: skillhub skill review pdf`.

## Risk assessment

| Risk | L×I | Mitigation |
|---|---|---|
| A source created by a concurrent add between preview and confirm | L×L | New file has an empty `BeforeDigest`; confirm fails as stale; re-preview reuses the source. |
| Weekly default adds a "sources due" item to `skillhub status` | M×L | Intended: it prompts the explicit check; user decision Q4 may switch the default to manual. |
| `source import` now needs network | M×L | Same as `skill add`; errors surface as `source_unavailable` with the existing message. |

## Security considerations

Attaching a source never changes trust class: GitHub/Git origins are already third-party (`internal/skillruntime/trust.go:28-30`). Repository URLs pass the existing `ValidateRemoteURLWithOptions` checks.

## Rollback

Revert the phase commit. Created source records stay valid for older binaries; `provenance.source_id` is an existing allowed key.

## Failure Protocol

If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, weaken or delete a test, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP, write the same evidence to `reports/executor-<YYMMDD-HHMM>-blocker-<phase-slug>.md`, and report the blocker to the user. Never continue by self-reasoning.
