# Cook report: backlog items F12, F13, F15 (2026-10-12, Asia/Saigon)

Branch: `worktree-agent-a7553522e5f7a7515`. Nothing pushed or tagged.

## F12 remote load counting

The hub dedupe key for a bundle load is now skill + digest + the resolution the load is attributed to (`markBundleFetch` in `internal/delivery/mcpserver/activation_tracker.go`, called from `recordBundleLoad` in `distribution.go`). The proxy's `counted` key and `rememberVersion` carry the resolution too (`proxy.go`), so a cache hit under a new resolution is confirmed to the hub again. Repeated fetches under one resolution still count once. The two accepted points are unchanged; no event type removed, `EventVersion` unchanged. Design doc section 4.4 updated.

Tests: `TestMarkBundleFetchCountsEachVersionOncePerSessionAndResolution`, `TestBundleLoadCountsAgainUnderANewResolution` (HTTP), `TestProxyCacheHitCountsAgainUnderANewResolution` (real proxy).

## F13 Go line and macOS note

`go` line in `go.mod` and `web/go.mod` is now 1.26.9, matching `ci.yml` and `release.yml`. `evaluation.yml` uses `1.26.x` (consistent). There is no `toolchain` line. Local Go 1.26.5 downloaded the 1.26.9 toolchain automatically (network available here) and vet/tests ran under it. Anyone building offline with an older Go will need the toolchain download or Go 1.26.9 installed. CONTRIBUTING.md and docs/installation.md say 1.26.9. `docs/release-runbook.md` now tells macOS users of v0.2.0 to v0.4.0 to update to v0.4.1 or later, and says the crash in a released binary has not been reproduced.

## F15

| Item | State |
|---|---|
| (a) keep pre-existing Claude permission rules on `connect --remove` | NOT fixed in code. Connect writes exact fixed rule strings and keeps no record of whether a rule was already there, so removal cannot tell a rule connect added from an identical hand-written one (for example `Bash(skillhub:*)`). A safe fix needs a record written at connect time (a sidecar file, or a marker in the settings file whose tolerance by Claude Code is unverified); leaving the allow rule behind would silently drop the ask rules' protection. Documented exact behavior in `docs/user-guide.md` (dry-run names every rule). Needs an owner decision. |
| (b) doctor exit code on remote failure | Already done by the redesign: a failed remote check is a problem finding, so doctor exits 1 (`TestDoctorExplainsEachBrokenRemoteConnection`, `TestDoctorWithAWorkspaceAlsoChecksRemoteEntries`). Contract line in `docs/command-reference.md` now states it. |
| (c) Windows quoting | Already done: `app.shellArgument` and `cli.shellQuote` no longer exist; everything uses `internal/shellquote`. |
| (d) eager connection | Not implemented; documented in `docs/design/remote-hub.md`. The session is opened on the first call because the hub labels it `remote:<host>` from the host name the first call carries (a newer-protocol host sends it only then), and an eager open would create a hub session and token check for every agent start, used or not. |
| (e) PowerShell timing script | Not added (the script embeds a 150-line Python MCP client; a PowerShell sibling is not small). Note added to `docs/remote-hub-guide.md`: use WSL or Git Bash with python3. |
| (f) real two-machine run | User task, not done. |

## F8

Not done: pushing local tags `pre-schema-v4` and `pre-schema-v5` waits for the release owner.

## Backlog

Rows F8, F12, F13, F15 in `docs/plans/2026-10-10-observation-backlog.md` carry a short dated note.
