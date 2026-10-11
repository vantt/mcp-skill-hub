# CI fixes after the remote hub merge (CI run 38132035511, commit e9289b4)

Every failure in the three logs was traced to a cause. One cause was a real
product bug on Windows: the connect summary named the Codex and Gemini config
files wrongly. The others were test assumptions: wall-clock timing, temporary
path length, Windows path escaping and quoting, and producer pacing.

## Failures, causes and fixes

| Test (OS) | Evidence in the log | Cause | Fix |
|---|---|---|---|
| `TestRemoteFetchFailures/a_download_that_stalls...` (Linux `-race`, Windows) | `remote_e2e_test.go:512: context canceled`, printed twice: once for the deadline call and once for the cancel call. The proxy's own assertions passed. | A test timing problem, not a proxy bug. The test's `damageBundle` helper forwarded the client's request context to the upstream hub. The test gave the client 500 ms (deadline) or 300 ms (cancel) to give up. Under `-race`, or on a slow runner, the hub needed longer than that to build the changed bundle, so the client gave up before the helper got its answer. The upstream request was then cancelled and the helper called `t.Error`. Reproduced on Linux by delaying the helper 600 ms: the output is exactly the CI output. The proxy classifies the deadline and the cancellation correctly whether they arrive before or during the body (`proxy.go` `requestBundle`). | The test no longer races the wall clock. The helper fetches the real bundle with `context.WithoutCancel`. A "stall" answer signals `front.stalled` once it has sent half the bundle. The deadline is a `deadlineOnSignal` context that expires on that signal, and the cancellation also fires on it. Checked with an artificial 1.5 s hub delay: the test still passes. `go test -race -count=3` passes. |
| `TestConnectWorkspaceOutputIsPinned` (macOS) | `temporary directory /private/var/folders/.../001 is longer than 90 characters` | `fixedLengthDir` padded `t.TempDir()`. On macOS that path is about 90 characters before the test name is added. | `fixedLengthDir` now pads from a short root of its own (`/tmp` where it exists, otherwise `os.TempDir()`), created with `os.MkdirTemp`. It is removed with Windows-tolerant retries. The pinned lengths (80, 90, 100) and the goldens are unchanged. Checked with a long `TMPDIR`. |
| `TestConnectWorkspaceOutputIsPinned/doctor_for_a_local_entry` (Windows) | `temporary directory D:\tmp\TestConnectWorkspaceOutputIsPinneddoctor_for_a_local_entry...\001 is longer than 80 characters` | Same as macOS: the subtest name made the `t.TempDir` path too long. | Same fix. |
| `TestConnectWorkspaceOutputIsPinned/project` and `/global` (Windows) | The golden diff: `Codex: will connect (.mcp.json, ...)` and `Gemini CLI: ... (.mcp.json, ...)`, where the golden has `.codex/config.toml` and `.gemini/settings.json` | **Product bug.** `formatChangeKind` (`internal/delivery/cli/connect.go`) looked for `.codex/config.toml` in an item summary that names the path with the OS separator. On Windows it never matched, so the summary showed `.mcp.json`. | `formatChangeKind` applies `filepath.ToSlash` to the summary first. This changes nothing on Linux or macOS. New test: `TestConnectNamesHostFilesTheSameWayOnEveryPlatform`. The `project-files`/`global-files` goldens would have failed next on Windows, because host files hold JSON-escaped backslash paths. The test normalizer now also replaces each path's escaped form, and on Windows only it turns the separators after a placeholder into `/`. |
| `TestConnectHubWritesEachHostsEntryAndNeverTheToken` (Windows) | `connect_remote_test.go:158: claude config: {..."command":"D:\\tmp\\...\\cli.test.exe"...}` | The test looked for the raw binary path in JSON. JSON escapes each backslash, so the raw path never appears. The cache directory checks on lines 160 and 177 had the same problem. | New `containsPath` helper accepts the raw or the escaped form. It is also used in the negative check of `TestConnectSwitchesAScopeAndRemoveRestoresIt`, which could never fail on Windows. |
| `TestConnectSwitchesAScopeAndRemoveRestoresIt` (Windows) | `connect_remote_test.go:352: remove preview: ... $ skillhub connect --remove --project "D:\tmp\...\005" --yes` | The test expected the path unquoted. On Windows, `app.shellArgument` deliberately puts a path containing backslashes in double quotes. | New `commandLineArg` helper: double quotes on Windows, the raw path elsewhere. |
| `TestServeMCPListenerLifecycle` (Windows) | `serve_mcp_test.go:281: connect line lacks the token file path: ... --token-file 'D:\tmp\...\.hub-token' --yes` | The test expected the path unquoted. The banner quotes it with `shellQuote` because of the backslashes. | The expectation uses `shellQuote(tokenFile)`. This is identical on Linux and macOS, where temp paths need no quotes. |
| `TestTokenCommands` (Windows) | `serve_mcp_test.go:365: ... --workspace 'D:\tmp\...\workspace' --yes`, but the test wanted the path unquoted | Same cause as the line above. | The expectation uses `shellQuote(ws)`. |
| `TestDoctorSendsTheTokenOnlyToHubsTheUserScopeNames` (Windows, `internal/app`) | `doctor_remote_test.go:190: ... --token-file "D:\tmp\...\hub-token"` | Same quoting cause, through `app.shellArgument`. | The expectation uses `shellArgument(tokenFile)`. |
| `TestWriterProcessesOnOneStoreLoseNoEvents` (Windows, `internal/telemetry`) | `writer 1 health = {... Accepted:321 Written:321 Dropped:79 ... Errors:0}`, then `stored events = 3678, want 4000` | No accepted event was lost, and no write failed or timed out (`Errors:0`, `Accepted == Written`). In each failing writer, Accepted plus Dropped is exactly 400: the 256-event queue overflowed and `Record` dropped events, as designed. At the Unix pace (5 ms per event), the eleven processes contending for SQLite file locks on the Windows runner sometimes held a writer for more than 1.3 s. This is not a handle leak: `Close` releases the write connection (`handleOperation` calls `writer.release()` before `opClose`). | On Windows only, the helper processes produce at a quarter of the pace (as the race build already does). What the test checks does not change: no failed write, every accepted event stored, the store intact and writable after a crash. |

## Other assumptions fixed before CI could report them

- `internal/hostintegration/remote_test.go` (`TestSwitchingAScopeBetweenLocalAndRemoteKeepsOneEntry`):
  the check "the config no longer names the workspace" looked only for the raw
  path, so on Windows it could never fail. It now also looks for the escaped form.
- `internal/delivery/mcpserver/remote_e2e_test.go` (legacy hub subtest): the
  check "the answer holds no hub path" had the same gap in its JSON. It now also
  looks for the escaped form.
- The connect golden file snapshots (see the table) would have failed on Windows
  after the output goldens.

I also searched the listed packages for mode checks, `sh` scripts, signals and
`file://` URLs. Every hit is already skipped on Windows or passed there in this
CI run. Go runs every test in a package even after one fails, so the only checks
CI has not exercised are those after a `t.Fatal` in a failing test. I read them
for the tests above.

## Verification

- `make check` on Linux: passed.
- `go test -race -count=3` on `internal/delivery/mcpserver`, `internal/telemetry`
  and `internal/delivery/cli`: passed.
- `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...`: clean.
- `GOOS=windows go test -c` for `internal/delivery/cli`, `internal/delivery/mcpserver`,
  `internal/telemetry`, `internal/app` and `internal/hostintegration`: compiles.
- The Windows path normalizer was checked on sample JSON and TOML text with a
  standalone program.

## Not verified here

- No test ran on Windows or macOS. The next CI run is the check. In particular:
  the Windows form of the connect `*-files.golden` snapshots;
  `TestConnectSwitchesAScopeAndRemoveRestoresIt` after the preview step (byte
  restoration on Windows); and whether a quarter pace is enough headroom on the
  Windows runner for the multi-process telemetry test.

## Open questions

- Two quoting styles exist for printed commands on Windows. `app.shellArgument`
  uses double quotes, which work in cmd.exe and PowerShell. `cli.shellQuote`,
  used by the listener banner, `token rotate` and the `chmod` hint, uses single
  quotes, which only PowerShell accepts. I left the product as it is and made
  the tests follow each one. Should the CLI use one style?
- Under heavy contention between processes on Windows, telemetry can drop events
  by design when the 256-event queue fills. Is a larger buffer worth considering
  for Windows hubs?
