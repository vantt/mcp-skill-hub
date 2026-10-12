# Cook report: backlog F15(a), connect --remove keeps pre-existing permission rules (2026-10-12, Asia/Saigon)

Branch: `worktree-agent-a9f5e7bd410a56754`. Nothing pushed or tagged.

## Behaviour

- Applying a Claude settings change (connect, remote connect, `doctor --fix`; all go through `apply` in `internal/hostintegration/files.go`) records which managed rules it added (desired minus what the file held before) in `permission-records.json` in the user config dir (`SKILLHUB_USER_CONFIG_DIR` or `<UserConfigDir>/skillhub/`), mode 0600, keyed `<scope>:<settings path>`. Never written into Claude's settings file.
- `connect --remove` strips only the recorded rules (ask rules connect added are still removed). A removal that leaves the file keeps an empty record so a second removal is a no-op; a removal that deletes the file forgets the record.
- No record (older connection): every managed rule is removed, as before. Documented in `docs/user-guide.md`.
- Legacy guard: with no record yet and an ask or deny managed rule already in the file, no record is made (it is an earlier connection, not hand-written), so removal still takes out everything. Only a pre-existing `Bash(skillhub:*)` allow counts as the user's.
- A record for a file that changed is intersected with what the file held, so stale entries do not claim user rules.

## Code

`internal/hostintegration/permission-record.go` (new), `files.go` (`updatePermissionRecord`), `remote.go` (removal uses recorded rules; `settingsFile` struct keeps the argument count within lint), `internal/userconfig/userconfig.go` (`Dir()`, `Default()` uses it).

## Tests

`permission-record_test.go`: pre-existing allow rule kept while ask/deny removed; full removal and record forgotten; no record removes everything; record 0600 and not in settings; re-applying after a missing rule records and removes it. Temp dirs only.

## Checks

gofmt clean; go vet ./... (linux, windows, darwin) clean; make lint 0 issues; go test -count=1 ./... all pass.

## Docs

One paragraph in `docs/user-guide.md` (Claude Code permission rules); F15 row in `docs/plans/2026-10-10-observation-backlog.md` marks (a) done.

Status: DONE
