# Cook report: remote hub Phase 4 (client setup: connect --hub, connect --remove, doctor)

Date: 2026-10-11 (Asia/Saigon). Plan: `plans/261010-1652-remote-hub-mcp-over-network/`. Status: completed.

## Outcome

One command on machine A points an agent host at the hub on machine B: `skillhub connect --hub <url> [--token-file <path>] --yes`. The host entry starts `skillhub mcp proxy --hub <url> [--token-file <path>]`; only the token file path is written, never the token. `skillhub connect --remove [--cache] --yes` takes the connection out of a scope again (local or remote hub) and can clear the hub's cached skills. `skillhub doctor` checks a remote entry and explains a failure in one cause and one fix line. The one-machine setup is unchanged: goldens generated from main before any change pin what `connect --workspace` prints and writes, and what a local `doctor` prints.

## What connect writes for a remote hub, per item

| Item | Local hub (unchanged) | Remote hub (`--hub`) | Why |
|---|---|---|---|
| MCP entry `skillhub` | `mcp serve --profile runtime --workspace W` (Claude), `mcp serve --workspace W` (Codex, Gemini, with `cwd`) | `mcp proxy --hub URL [--token-file P]`, same entry name, no `cwd` | One scope points at one hub |
| `skillhub-curation` entry | Claude: removed only when unchanged; Codex/Gemini: removed | removed | Curation stays on machine B |
| Allowed directories (Claude `additionalDirectories`, Codex `writable_roots`, Gemini `includeDirectories`) | the workspace's three runtime directories | the client cache directory of that hub, `<state>/remote/<hub-id>` | The proxy returns `local.path` and the state/config directories there |
| Claude CLI permission rules (allow, 2 ask, deny) | written | written, unchanged | Phase rule: keep the four rules |
| Instruction block | written | written, identical | Task: as for a runtime-only connection |
| `system-curator` skill | installed | not installed; an unchanged bundled copy from an earlier local connection is removed | It drives curation, which is unavailable remotely; the preview says so |

Switching a scope from `--workspace` to `--hub` (or back, or to another hub) replaces the entry and swaps the allowed directories of the old target for the new one; the preview lists what is replaced ("This replaces: Claude Code: the local hub at ..."). A local connection over a local entry keeps today's behavior exactly (the directories of an older workspace are not touched, as before).

## What was built

| Area | Where | Notes |
|---|---|---|
| Plan modes | `internal/hostintegration/remote.go` (new), `types.go`, `integration.go`, `files.go`, `connection.go`, `toml_update.go` | `Request.Hub` and `Request.Remove` select a remote or removal plan; `Change.Remove` deletes a file. `Apply` still recomputes every change from the file's current content (`expectedChange`), now for all three modes, and refuses a plan whose content differs. `ReadRegistration` tells local and remote entries apart (`RegisteredWorkspace` is a wrapper with the same results). `upsertCodexTOMLTable` was split so the remote entry can set its lines without `cwd`; the local output is identical. |
| Removal | same | Removes the `skillhub` and `skillhub-curation` entries, the directories added for the registered target, the CLI rules, the instruction block (and the blank line connect put before it), and the curator skill only when it is the unchanged bundled copy. Containers left empty are pruned in the reverse order connect added them, a file left with nothing else is deleted, and empty directories above a deleted file are removed, so connect then remove restores the scope (tested byte for byte, empty and seeded projects, local and remote, user scope `.claude.json`). |
| App services | `internal/app/connect_remote.go`, `doctor_remote.go` (new); `host-connections.go` | `ConnectRemote`, `Disconnect`, `NormalizeConnectHubURL` (http/https, a host, no path but `/`, no user info, query or fragment), token file check through `hubtoken.Read` (missing, not a token, group/other readable on Unix), `ProbeHub`, `RemoteDoctorService`, `FindRemoteEntries`. The local doctor's connection repair now skips remote entries, so it never reports a remote entry as an outdated local one or rewrites it. |
| Status endpoint | `internal/delivery/mcpserver/status.go` (new), 2 lines in `http.go` | `GET /mcp/status` behind the same gate (Host, token, rate limit) answers `{"service":"skillhub","version":"...","remote_api":1}`: no path, workspace or secret. `app.RemoteAPILevel` is the remote protocol level; doctor compares it to tell an older from a newer hub. A hub from before this endpoint answers 404 there before checking the token; doctor then asks `/mcp` (401 means a wrong token, anything else means the token passed) and reports "older hub". |
| CLI | `internal/delivery/cli/connect.go`, `connect_remote.go` (new), `workspace.go` (doctor), `help.go`, `mcp_proxy.go` | Flags `--hub`, `--token-file`, `--remove`, `--cache`; refused combinations get their own termui error (`--workspace` with `--hub`, `--remove` with `--hub`/`--workspace`/`--token-file`, `--token-file` without `--hub`, `--cache` without `--remove`). Preview first, `--yes` applies, `--json` adds a `remote` or `removal` object. `skillhub help`, `help connect` and `help doctor` describe the new forms and the token hand-off. `hubTokenEnv` now refers to `app.HubTokenEnv`. |

## Doctor

Doctor finds entries for remote hubs in the current project and in the user scope. Without any, it behaves exactly as before. On a machine without a workspace (machine A) it checks only the remote hubs; with a workspace it prints the usual report and then the remote section; `--json` adds `remote_hubs`. Checks per hub (hosts grouped per scope in "Used by"):

| Check | Failure cause printed (one line, wrapped) | Fix printed |
|---|---|---|
| Hub token | token file missing / readable by others / not a token; or no `--token-file` and `SKILLHUB_HUB_TOKEN` unset | copy with scp and chmod 600 / `chmod 600 <path>` / connect with `--token-file` or set the variable |
| Hub reachable | "Nothing answers at <url> (connection refused): the hub is stopped, or the SSH tunnel or the Tailscale connection is down." Also: host name refused, not a Skill Hub listener, hub answers an error | loopback URL: `skillhub serve mcp --addr 127.0.0.1:<port>` on machine B and `ssh -N -L <port>:127.0.0.1:<port> <machine-b>`; other URL: `--addr <host:port> --trust-private-network` and `tailscale status` |
| Token accepted | "The hub refused the token: it is not the hub's current token (rotated, or from another hub)." / refusing for a minute after too many wrong tokens. Skipped, with the way to check it, for a non-loopback hub that only a project's files name: the token is not sent there | copy the current token file again (`skillhub token show` on B shows its path) |
| Hub version | "The hub on machine B runs an older Skill Hub (vX) than this machine (vY)" / "This machine runs an older Skill Hub ..." | update skillhub on B and restart the listener / `skillhub update` here |
| Skill cache | "The skill cache <dir> is not writable (permission denied) ..." | `chmod u+w <nearest existing dir>` or set `XDG_STATE_HOME` |
| Last fetch | information: "never: ..." or the time of the most recent fetch or cache use and the number of skills cached | none |

A check that cannot run is "skipped" with the reason. Doctor exits 0, as it does today for findings. No check, line or JSON field contains the token.

## Acceptance

| Criterion | Evidence |
|---|---|
| `connect --hub` writes exactly the expected files per host and no token | `TestConnectHubWritesEachHostsEntryAndNeverTheToken` (real listener on 127.0.0.1:0, temp HOME and state dir; per host the exact file set, the entry, no curation, no `cwd`, the cache directory allowed; global scope file set; the token string found in no file and in no output, also `--json`); `TestRemotePlanWritesTheProxyEntryForEveryHost` |
| `connect --workspace` then `connect --hub` leaves one entry | `TestConnectSwitchesAScopeAndRemoveRestoresIt`, `TestSwitchingAScopeBetweenLocalAndRemoteKeepsOneEntry` (and back to local) |
| `connect --remove` restores the host config | same CLI test (seeded project, byte for byte), `TestRemoveReturnsTheScopeToItsStateBeforeConnecting` (4 cases, also no empty directory left), `TestRemoveInTheUserScopeKeepsUnrelatedClaudeState`, `TestRemoveLeavesAChangedCuratorSkillAndUnrelatedPermissions`, `TestConnectRemoveCacheClearsTheHubsFetchedSkills` (only that hub's cache) |
| `connect --workspace` and local doctor byte-for-byte unchanged | `TestConnectWorkspaceOutputIsPinned`: goldens committed in `b8977fd` from unchanged main (preview, apply, already-current output; every written file for a seeded project and for the user scope; `doctor` with and without `--workspace`); still green after the change. Existing `mcp serve` subprocess tests unchanged and green |
| Doctor: stopped hub, wrong token, old hub, unwritable cache each print a distinct message | `TestDoctorExplainsEachBrokenRemoteConnection` (real listener, then stopped; a legacy-hub stub on loopback port 0; cache directory chmod 0500, skipped on Windows and as root; asserts exactly one failed check per case and that the four messages differ, and that the token is never printed); `TestProbeHubTellsTheFailuresApart`, `TestProbeHubRecognizesOtherServersAndGateAnswers`, `TestProbeHubFollowsNoRedirect` |
| End to end on loopback | `TestConnectHubEntryStartsAProxyThatResolvesAndFetchesASkill`: listener in the test, `connect --hub` in a temp HOME, the command and args read from the written `.mcp.json` started as a process, `skill_resolve` and `skill_get` through it, the fetched file read from the client cache, proxy exit code 0, then `doctor` reports the fetch |
| termui goldens | `TestRemoteConnectAndRemoveOutputGolden`, `TestRemoteDoctorOutputGolden`, `TestConnectUsageErrorsGolden`, `TestHelpDescribesRemoteConnect` |
| Status endpoint reveals only the version, needs the token | `TestStatusEndpointNeedsTheTokenAndRevealsOnlyTheVersion` |

Commands run:
- `go test -race -count=1 ./internal/hostintegration/ ./internal/app/ ./internal/delivery/cli/ ./internal/delivery/mcpserver/`: pass (new CLI tests also 3 times under `-race`).
- `make check LINT_BASE=main` (fmt-check, vet, golangci-lint new issues, full `go test ./...`): pass, 0 issues.
- `GOOS=windows go vet` and `GOOS=darwin go vet` on the four packages: pass.
- Manual run of the built binary in an isolated HOME under the session scratch directory (one listener started and stopped by its own PID): preview, switch, doctor healthy / wrong token / stopped, connect with the hub down, remove with and without `--cache`; the project directory was empty again afterwards.

## Review

An independent code review (one-machine regression, token handling, Apply integrity, removal) confirmed with its own live round trips that the one-machine path is unchanged, that the token is in no host file, state file or output, that Apply re-derives remote and removal changes, and that connect then remove restores a project byte for byte. Fixed after the review, with tests:

- High: doctor read the token and sent it to whatever hub URL a project's `.mcp.json` named, so a cloned repository could collect it. Doctor now sends the token only to a loopback hub (an SSH tunnel; a local listener could read the token file anyway) or to a hub the user scope also names. For a hub only a project names it checks reachability without the token and says how to check the token with the user's own `connect --hub` preview, which writes nothing. Hub URLs read from host files are validated before any request (`TestDoctorSendsTheTokenOnlyToHubsTheUserScopeNames`, `TestDoctorRefusesAHubURLItCannotUse`).
- Medium: a removal stripped only the directories of the entry it found, so directories stayed when the entry had been deleted by hand or `XDG_STATE_HOME` had changed. It now also strips directories that are recognizably Skill Hub's: `.../skillhub/remote/<hub-id>`, and the runtime directories of a directory that is a Skill Hub workspace; a look-alike user directory stays (`TestRemoveCleansUpDirectoriesOfAnEntryThatIsGone`).
- Medium: the legacy-hub probe took any server that answered something other than 404 on `/mcp` for a hub that accepts the token. It now first requires the 401 with the `skillhub` realm without a token (`TestProbeHubDoesNotTakeACatchAllServerForALegacyHub`).
- Medium: removing the user-scope connection also removes the instruction block projects relied on when they were connected without their own copy; the preview now warns.
- Low: `--cache` clears the cache before removing the files, so a failed clear leaves the connection in place for a retry, and warns when the other scope still uses that hub. A 5xx answer body is never shown (a proxy in between could echo the token); only the hub's own two known messages or the HTTP status text are (`TestProbeHubNeverEchoesAnErrorBody`).

Kept, with the reason: a file that held only `{}` before connect is deleted by a removal, and an empty `mcpServers` or `context` object that existed before is pruned (no user data is lost); `skillhub-curation` is removed unconditionally in remote and removal plans (see below); doctor's write test creates and deletes one temporary file in the nearest existing directory above the cache, which may be the home directory.

## Deviations and decisions to confirm

- Files outside the listed ownership: `internal/app/connect_remote.go` and `internal/app/host-connections.go` (the connect service lives in `internal/app`, and the local doctor's repair had to skip remote entries), `internal/delivery/cli/workspace.go` (where `doctor` is parsed and printed) and `mcp_proxy.go` (one line, the shared env variable name). None is owned by the parallel telemetry phase. The new status handler is a separate file; `http.go` got two additive lines.
- `--remove` removes more than the MCP entry: also the allowed directories, CLI rules, instruction block and an unchanged curator skill, because the acceptance asks for the host config "as it was before connecting". A pre-existing permission array is re-encoded compactly by connect itself, so a round trip is byte-exact only when connect did not have to rewrite such an array with other values in it; the values are restored either way. A changed curator skill and unrelated permissions stay.
- Switching to a remote hub removes an unchanged bundled curator skill (curation is unavailable remotely); switching back reinstalls it.
- The `skillhub-curation` entry is removed for every host in remote and removal plans, also on Claude Code where the local plan removes it only when unchanged: a curation server for a local workspace contradicts one hub per scope.
- Allowed directory for a remote hub: the hub's whole cache directory `<state>/remote/<hub-id>` (one entry instead of three), computed with the state directory of the shell that runs connect. If the agent host starts with another `XDG_STATE_HOME`, the proxy caches elsewhere; doctor shows the directory it expects.
- Without `--token-file` the entry relies on `SKILLHUB_HUB_TOKEN` in the agent host's environment; connect warns, and doctor checks the variable in its own shell only (it cannot see the host's environment) and says so.
- The probe in the preview has a 3 second timeout, follows no redirect and ignores proxy settings. An unreachable hub is a warning, never an error, so the entry can be written before the tunnel is up.
- `last fetch` is the most recent fetch or cache use (the cache records use on every hit), not strictly the last download.
- Commit trailers use `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` as the lead asked.
- `ak plan check` ticked the phase file; it reported that the global plan store was not synchronized (this plan is not tracked there). `plan.md` status cells were not edited.

## Not verified

- Windows and macOS: `go vet` only. On Windows the token file mode check does not apply (no group/other bits; `hubtoken.Read` skips it), the unwritable-cache test is skipped, and a refused connection is recognized by its message text because Windows uses its own error number. Host config paths are the same relative paths as today.
- Tailscale and a real SSH tunnel: the doctor's fix lines were checked on loopback only.

## Unresolved questions

1. Should `--remove` keep the CLI permission rules when the user had them before connecting? Today it removes the exact values connect adds.
2. Should doctor exit non-zero when a remote check fails (useful for scripts)? It keeps today's convention of exit 0 with findings.
3. Phase 6 guide: recommend a fixed place for the token copy on machine A (the preview suggests `~/.config/skillhub/hub-token`)?
4. Doctor does not send the token to a Tailscale hub that only a project names (security fix above), so for a project-scoped Tailscale connection the token and version checks are skipped with instructions. Is a `doctor --trust-project` flag wanted, or is the `connect --hub` preview enough?
