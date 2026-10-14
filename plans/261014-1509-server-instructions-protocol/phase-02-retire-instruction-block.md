# Phase 2: Retire the instruction-file block

## Context (traced)

- The block text and its writer live in `internal/hostintegration/bootstrap.go`: the constants (`:12-15`),
  `bootstrapBlock` (`:21-50`), `updateBootstrap` (`:52-95`, which validates markers, then splices in a fresh block or
  appends one), `appendBootstrap` (`:97-112`) and `FreshInstructionFile` (`:117-120`).
- `connect`, `doctor --fix` (local) and `init` all plan through `hostintegration.prepare`
  (`integration.go:132-203`). The connect path calls `prepareInstructions` (`integration.go:409-419`), after a
  `globalHasBootstrap` probe of the real `$HOME` (`integration.go:185-195`). The remove path calls
  `prepareInstructionsRemoval` (`remote.go:495-509`), unless `KeepInstructions` is set (a hub root,
  `connect_remote.go:478`).
- `Apply` re-derives each change in `expectedChange` (`files.go:244-260`). For `ChangeBootstrap` it calls
  `updateBootstrap`, or `removeBootstrap` in remove mode, and deletes the file only in remove mode when the result is
  blank.
- The removal behaviour already exists and is correct: `removeBootstrap` (`remote.go:513-537`) validates the markers,
  splices out the single block and the blank lines connect added, and leaves every other byte alone.
  `prepareInstructionsRemoval` marks the file for deletion only when it existed, changed, and holds nothing but
  whitespace afterwards (`remote.go:505-507`). That is the ownership rule to reuse: a file that held only the block is
  one Skill Hub created.
- doctor (local client) turns every planned change into "Out of date: <host> <kind> <path>" through `describeChange`
  (`internal/app/health.go:367-373`, used by `health_client.go:201-203`, by the remote check at
  `health_client.go:425-428`, and by the hub check at `health_hub.go:407`). The fix runs the same plan
  (`reconnectLocal`).
- `init` uses `FreshInstructionFile` to accept a tracked instruction file that init wrote
  (`internal/app/workspace_remote.go:503,518-521`).
- `connect --remove -g` warns `global_instructions_removed` (`internal/app/connect_remote.go:483-485`, helper
  `:502-510`). The warning only made sense while projects relied on the global copy of the block.
- The CLI labels a `bootstrap-instructions` item by file name (`internal/delivery/cli/connect.go:226-233`).

## Requirements

1. Connect writes no instruction text. For every host and scope, the instruction file is planned with
   `removeBootstrap`, in connect mode and in remove mode alike.
   - No block, or no file: desired equals raw, so the file is current and no change is planned. A missing file stays
     missing.
   - A block of any marker version: the block is spliced out (`removeBootstrap`) and user text stays byte-identical.
   - Only whitespace is left: the file is deleted (the existing rule).
   - Bad markers (nested, duplicate, reversed, unmatched, version mismatch): a conflict. Nothing is written, and doctor
     says so with the existing conflict finding (`health_client.go:166-178`).
2. In remove mode, `KeepInstructions` (a hub root) keeps its current meaning.
3. doctor names the change as obsolete. `describeChange` returns
   `<host> obsolete Skill Hub instruction block <path>` for `ChangeBootstrap`, so the message reads
   `Out of date: claude-code obsolete Skill Hub instruction block CLAUDE.md.` The preview header in
   `preview.go:179` becomes `obsolete Skill Hub block (removed):` followed by the before and after views.
4. The CLI connect list shows `CLAUDE.md (old Skill Hub block removed)` (and the same for AGENTS.md and GEMINI.md) in
   `formatChangeKind`.
5. Delete the block text and its writer: the `bootstrapStart`/`bootstrapEnd` constants, `bootstrapBlock`,
   `appendBootstrap`, `FreshInstructionFile`, `prepareInstructions`, and the `globalHasBootstrap` probe. That probe
   also removes a real-`$HOME` read from planning. Keep `bootstrapMarkerPattern` and the marker validation. Rename
   `updateBootstrap` to `bootstrapSpan(raw) (start, end int, found bool, conflict string)`, used by `removeBootstrap`
   and `managedBootstrapBlock`.
6. Add `hostintegration.IsBootstrapOnlyFile(raw []byte) bool`. It is true when the markers are valid, one block
   exists, and `removeBootstrap` leaves only whitespace. `checkTrackedFiles` uses it instead of the exact-text
   comparison, so a hub committed by any older version is still accepted, and no instruction text is kept in code.
7. Remove the `global_instructions_removed` warning and `removesInstructions`.
8. Keep `ChangeBootstrap = "bootstrap-instructions"`, the item IDs and `changeRank`, all unchanged (public JSON).

## Files

| File | Change |
|------|--------|
| `internal/hostintegration/bootstrap.go` | Delete the writer and the text. Keep the pattern, `bootstrapSpan` and `IsBootstrapOnlyFile`. Rewrite the doc comments to say the block is obsolete and is only removed. |
| `internal/hostintegration/remote.go:493-537` | Rename `prepareInstructionsRemoval` to `prepareInstructionFile`. `removeBootstrap` uses `bootstrapSpan`. |
| `internal/hostintegration/integration.go:171-201,409-419` | Use one call, `prepareInstructionFile`, for both modes. Drop `globalHasBootstrap` and `prepareInstructions`, and any `bytes`/`os` imports left unused. |
| `internal/hostintegration/files.go:249-258` | `ChangeBootstrap`: always `removeBootstrap`. Delete the file when raw changed and the result is blank, in either mode. |
| `internal/hostintegration/preview.go:176-195` | Header text. `managedBootstrapBlock` uses the shared span helper. |
| `internal/hostintegration/types.go:126-143` | Doc comments only: "the instruction block" is now "an obsolete instruction block". |
| `internal/app/health.go:367-373` | Special-case `ChangeBootstrap` in `describeChange`. |
| `internal/app/connect_remote.go:483-485,502-510` | Remove the warning and its helper. |
| `internal/app/workspace_remote.go:494-521` | Use `IsBootstrapOnlyFile`, and update the comment at `:494-497`. |
| `internal/delivery/cli/connect.go:226-233` | Add the `(old Skill Hub block removed)` suffix. |
| Tests (all listed): `internal/hostintegration/{integration_test.go,scope_test.go,remote_test.go,resource_limits_test.go}`, `internal/app/{workspace_test.go,system_skill_blocked_test.go,onboarding_repair_test.go}`, `internal/delivery/cli/{connect_remote_test.go,onboarding_test.go,init_test.go,connect_regression_test.go,host_item_id_test.go}` | Rewrite the "block is written/refreshed" assertions as "no file is written" and "old block removed". Tests build their legacy fixture inline as a short synthetic block (`<!-- skillhub:bootstrap:v1:start -->\n## Skill Hub\n\nold text\n<!-- skillhub:bootstrap:v1:end -->`), as `integration_test.go:504` already does, so the long text appears in no file. |
| Goldens: `internal/delivery/cli/testdata/connect-workspace/{project-files,global-files}.golden` | Regenerate with `SKILLHUB_UPDATE_GOLDEN=1` (`connect_regression_test.go:19`). Review the diff: only the instruction files and blocks disappear. |

## Steps

1. Change `bootstrap.go`, `remote.go`, `integration.go`, `files.go` and `preview.go`. Run
   `go test ./internal/hostintegration -count=1` and fix the expectations.
2. Add the unit tests below to `integration_test.go` and `remote_test.go`.
3. Change `internal/app` and `internal/delivery/cli`. Run `go test ./internal/app ./internal/delivery/cli -count=1`.
   Regenerate the goldens and review the diff by eye.
4. Run `make check`.

## Tests (new or rewritten)

- `TestConnectWritesNoInstructionFile`: on a temp project, plan and apply for all hosts. No `CLAUDE.md`, `AGENTS.md`
  or `GEMINI.md` is created, and none is planned.
- `TestConnectRemovesObsoleteBlockKeepingUserText`: user prose before and after a v1 block, with CRLF and LF
  variants. After apply the bytes equal `before + "\n\n" + after` (the existing `removeBootstrap` contract).
- `TestConnectDeletesFileHoldingOnlyTheBlock`, and a variant with only a block plus trailing newlines.
- `TestConnectRemovesAnyBlockVersion`: a `v2` marker pair is removed too.
- `TestMalformedBlockIsAConflictAndNotWritten`: reuse the cases at `integration_test.go:111-114`.
- `TestDoctorReportsObsoleteBlock` (`internal/app`): in a temp hub plus temp project with an old block (isolated
  `HOME` and `SKILLHUB_USER_CONFIG_DIR`), the doctor finding message contains `obsolete Skill Hub instruction block`.
  After `--fix`, the user text is byte-identical and a second doctor run reports `current`.
- `TestInitAcceptsTrackedBlockOnlyInstructionFile` (`workspace_remote` test): a tracked `CLAUDE.md` holding only a
  synthetic v1 block is not listed as foreign config.

## Risks

- **Real `HOME` leakage.** Removing the `globalHasBootstrap` probe lowers it. App and CLI tests must keep using the
  package's isolated `HOME` helpers. Grep every new test for `os.UserHomeDir` before committing.
- **Golden churn hides an unintended change.** Review the regenerated goldens line by line.
- **Hub root**: the next `doctor --fix` of the hub's own connection deletes a committed block-only `CLAUDE.md`. That is
  intended, and `hub_uncommitted` reports it.

## Rollback

Revert this phase's commit, and revert phase 1 with it. Files that were already cleaned get the block back from the
old binary's next `connect` or `doctor --fix`.
