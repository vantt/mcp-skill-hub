---
phase: 1
title: "Web adapter foundation (Go)"
status: complete
priority: P1
effort: 32h
dependencies: []
---

<!-- Updated: Validation Session 1 - rewritten as an executor handover (task-level Verify, Failure Protocol); fixes: schema read by path, state validated with app.SkillStates, help tests added, listen rule D14 -->

# Phase 1: Web adapter foundation (Go)

## Goal

`skillhub serve web` (alias `skillhub web`) starts a secure HTTP server that picks its listen address by the D14 rule, serves the embedded UI directory, and answers five read endpoints with bodies produced by existing `internal/app` code.

## Before you start

- Read [plan.md](./plan.md) "Executor hard rules". They apply to every task.
- Read [decisions.md](./decisions.md) D2, D3, D7, D11, D14.
- Read these files once (do not edit unless a task names them): `internal/delivery/mcpserver/server.go` (lines 59–92 `New` and `Serve`; 622 `appResult`), `internal/delivery/cli/mcp.go`, `internal/delivery/cli/root.go`, `internal/delivery/cli/help.go` (line 37 global list, line 54 `commandUsage`), `internal/delivery/cli/workspace_resolve.go` (`resolveWorkspace`), `internal/app/errors.go`, `internal/app/error_classify.go`, `internal/delivery/mcpserver/server_test.go` lines 685–705 (`newMCPWorkspace`: how to create a test workspace with one active skill).
- Facts verified on 2026-10-04 (do not re-derive):
  - `app.ClassifyError(err)` maps `skill.ErrNotFound` to `invalid_request`. The adapter adds HTTP 404 only.
  - `SkillService.ListSkills` returns a plain error for an unknown state; the classifier turns it into `internal_error`. The adapter must validate `state` against `app.SkillStates` first.
  - `schemas/embed.go` does **not** embed `error-envelope.schema.json`; tests read it from disk at `../../../schemas/error-envelope.schema.json` (relative to `internal/delivery/web`).
  - The error-code enum in that schema has exactly 30 codes.

## Tasks

### Task 1.1 — Preflight
- Goal: confirm the baseline is green before any change.
- Target files: none.
- Steps:
  1. Run `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check 0`.
- Success criteria: guard passes on the untouched tree.
- Verify: the last line printed is exactly `GUARD RESULT: PASS (phase 0)` and the exit code is 0.

### Task 1.2 — Package skeleton and server lifecycle
- Goal: an importable `internal/delivery/web` package with a `Server` that can start and stop.
- Target files: create `internal/delivery/web/server.go`.
- Steps:
  1. Declare `package web` with a package comment `// Package web is the local HTTP delivery adapter for the Skill Hub WebUI.`
  2. Define `type InterfaceInfo struct { Name string; Up bool; Loopback bool; IPs []net.IP }`.
  3. Define `type Options struct` with fields: `Workspace string`, `Token string`, `ListenPort int` (the port actually bound), `AllowHosts []string`, `Dev bool`, `Now func() time.Time`, `Interfaces func() ([]InterfaceInfo, error)`, `Hostname func() (string, error)`, `Assets fs.FS`.
  4. Define `type Server struct` holding: the resolved workspace path, the options, and one value of each service it uses: `app.CurationService`, `app.SkillService`, `app.SkillAddService`, `app.SourceService`, `app.DistillService`, `app.InsightService`. Same-package tests may replace these fields (pattern: `mcpserver` tests set `adapter.resolver.Telemetry`).
  5. `func New(opts Options) (*Server, error)`: resolve the workspace with `workspace.Discover(opts.Workspace)` exactly as `mcpserver.New` does; default `Now` to `time.Now`, `Interfaces` to a function built on `net.Interfaces()` (fill `Up` from `net.FlagUp`, `Loopback` from `net.FlagLoopback`, `IPs` from each `Addr` that is a `*net.IPNet`), `Hostname` to `os.Hostname`, and `Assets` to `DefaultAssets()` (task 1.6).
  6. `func (s *Server) Handler() http.Handler` builds a `http.ServeMux` and wraps it with the middleware chain from task 1.4 (leave the chain function as a stub that returns the mux until task 1.4; the stub must still compile).
  7. `func (s *Server) Serve(ctx context.Context, ln net.Listener) error`: run `http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}` on `ln`; when `ctx` is done call `Shutdown` with a 5-second timeout; return `nil` on clean shutdown.
- Success criteria: the package compiles; no test yet.
- Verify: `go build ./internal/delivery/web/` exits 0 with no output.

### Task 1.3 — Error envelope and status table
- Goal: one place that turns any error into the classified envelope with the right HTTP status.
- Target files: create `internal/delivery/web/errors.go`, `internal/delivery/web/errors_test.go`.
- Steps:
  1. Define `var statusByCode = map[app.ErrorCode]int{...}` with **exactly** the 30 rows of the table below.
  2. `func writeJSON(w http.ResponseWriter, status int, value any)`: set `Content-Type: application/json; charset=utf-8`, write status, encode with `json.NewEncoder` and `SetEscapeHTML(false)`.
  3. `func writeError(w http.ResponseWriter, err error, notFound bool)`: `classified := app.ClassifyError(err)`; status is `404` when `notFound` is true, else `statusByCode[classified.Code]`, else `500`; body is `app.ErrorResult(classified)`.
  4. `func writeAppError(w http.ResponseWriter, e *app.Error)` for errors the adapter itself creates (for example `app.NewInvalidRequestError(...)`); same status lookup.
  5. Test `TestErrorStatusCoversSchemaCodes`: read `../../../schemas/error-envelope.schema.json`, decode `properties.code.enum` into `[]string`, assert the slice has 30 entries and every entry is a key of `statusByCode`.

| Codes | Status |
|---|---|
| `invalid_request`, `validation_failed`, `ambiguous_locator`, `ambiguous_ref`, `local_watch_unsupported`, `unsupported_schema`, `resource_limits_exceeded` | 400 |
| `skill_selection_required`, `unknown_resolution`, `clarification_budget_exhausted` | 422 |
| `permission_denied` | 403 |
| `skill_conflict`, `source_conflict`, `edit_conflict`, `stale_proposal`, `stale_context`, `source_changed`, `resolution_retry_exhausted`, `resource_content_unavailable` | 409 |
| `snapshot_expired` | 410 |
| `workspace_invalid`, `recovery_required`, `index_stale` | 503 |
| `source_unavailable`, `resource_read_failed` | 502 |
| `operation_cancelled` | 499 |
| `internal_error`, `resource_digest_mismatch`, `run_interrupted`, `partial_distill_failure` | 500 |

- Success criteria: every schema code has an explicit status.
- Verify: `go test -count=1 -v -run '^TestErrorStatusCoversSchemaCodes$' ./internal/delivery/web/` exits 0 and prints `--- PASS: TestErrorStatusCoversSchemaCodes`.

### Task 1.4 — Security middleware and auth throttle
- Goal: no request reaches a handler unless Host, token (API only), Origin and Content-Type rules pass.
- Target files: create `internal/delivery/web/security.go`, `internal/delivery/web/throttle.go`, `internal/delivery/web/security_test.go`; modify `internal/delivery/web/server.go` (replace the stub chain).
- Steps:
  1. Put the chain in `func (s *Server) withMiddleware(next http.Handler) http.Handler`; `Handler()` returns `s.withMiddleware(mux)`. Middleware order, outermost first: request ID → Host allowlist → panic recovery (returns the `internal_error` envelope) → security headers → body limit (`http.MaxBytesReader`, 4 MiB) → auth + throttle (only for paths starting `/api/`) → Origin and Content-Type (only for `POST`, `PUT`, `PATCH`, `DELETE`) → mux.
  2. Security headers on every response: `Content-Security-Policy: default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store` for `/api/` paths.
  3. Host allowlist: split `r.Host` with `net.SplitHostPort`; a missing port is rejected. Lowercase the host. Allowed when the port equals `ListenPort` and the host is one of: `127.0.0.1`, `localhost`, `::1`; any IPv4 of any interface returned by `Interfaces()` (cache the set for 5 seconds using `Now`); `Hostname()` and `Hostname()+".local"`. Also allowed: any `AllowHosts` entry (an entry `name` matches `name` with `ListenPort`; an entry `name:port` matches exactly). With `Dev`, also allow `127.0.0.1:5421` and `localhost:5421`. Rejection: status 421, body `app.NewInvalidRequestError("The Host header "+host+" is not allowed.", "Open the URL printed by `skillhub serve web`, or add --allow-host "+host+".")`.
  4. Auth: header `Authorization: Bearer <token>`; compare with `subtle.ConstantTimeCompare`. Failure: status 401, body `app.NewInvalidRequestError("The session token is missing or invalid.", "Reopen the web UI from the URL printed by `skillhub serve web`.")`. Never log the token or the header.
  5. Throttle (`throttle.go`): key = the IP part of `r.RemoteAddr` (ignore `X-Forwarded-For`). Count failures in a sliding 60-second window using `Now`; at 20 failures, respond 429 with header `Retry-After: 60` until the window clears; keep at most 1,024 keys and evict the oldest. Successful requests do not reset another address's counter.
  6. Origin and Content-Type for mutating methods: `Origin` must equal `"http://" + r.Host`; else 403 with an `invalid_request` body. `Content-Type` must start with `application/json`; else 415 with an `invalid_request` body.
  7. Write `TestSecurityMiddleware` as one table test against `Server.Handler()` with fake `Interfaces` (one interface `wlan0` up with `10.0.0.5`) and fake `Hostname` (`devbox`), plus a counting test handler wrapped with `s.withMiddleware(...)` (the same function production uses, so no test-only seam) to exercise GET and POST. Rows: no token → 401; wrong token → 401; right token → 200; Host `evil.example:7421` → 421; Host `10.0.0.9:7421` → 421; Host `10.0.0.5:7421` → 200; Host `devbox:7421` → 200; Host `devbox.local:7421` → 200; Host `127.0.0.1:9999` → 421; Host without port → 421; `--allow-host` entry `skillhub.lan` with Host `skillhub.lan:7421` → 200; POST with foreign Origin → 403; POST with `text/plain` → 415; POST body over 4 MiB → 4xx and handler not reached; every response carries the CSP header. Assert the handler-call counter is unchanged for every rejected row.
  8. Write `TestAuthThrottle`: with a fake clock, 20 failures from `192.0.2.1` then a 21st request (even with a valid token) → 429 with `Retry-After: 60`; a request from `192.0.2.2` with a valid token → 200; advance the clock 61 seconds → `192.0.2.1` with a valid token → 200.
- Success criteria: both tests pass.
- Verify: `go test -count=1 -v -run '^(TestSecurityMiddleware|TestAuthThrottle)$' ./internal/delivery/web/` exits 0 and prints `--- PASS: TestSecurityMiddleware` and `--- PASS: TestAuthThrottle`.

### Task 1.5 — Listen rule and startup output
- Goal: the D14 address choice and the startup text, as pure functions.
- Target files: create `internal/delivery/web/listen.go`, `internal/delivery/web/listen_test.go`.
- Steps:
  1. `func UsableIPv4(ifaces []InterfaceInfo) []net.IP`: include an IP only if its interface is `Up` and not `Loopback`, the IP has a 4-byte form, is not loopback, not link-local (`169.254.0.0/16`), and is unicast. Each IP counts separately.
  2. `func ChooseListenAddr(explicit string, loopbackOnly bool, port int, ifaces []InterfaceInfo) (string, error)`: if `explicit` is non-empty, validate it with `net.SplitHostPort` (numeric port 0–65535) and return it unchanged; else if `loopbackOnly`, return `127.0.0.1:<port>`; else if `len(UsableIPv4(ifaces)) >= 2`, return `0.0.0.0:<port>`; else `127.0.0.1:<port>`. Precedence between `--addr` and `SKILLHUB_WEB_ADDR` is resolved by the caller (flag first).
  3. `func WriteStartup(w io.Writer, boundHost string, port int, token string, ifaces []InterfaceInfo)`: if `boundHost` is `0.0.0.0`, write first the line `WARNING: The web UI is reachable from other devices over plain HTTP. Anyone who can observe this traffic can read the session token. Use --loopback-only to keep it on this machine.`, then `Skill Hub web UI: http://127.0.0.1:<port>/#token=<token>`, then one line per usable IP: `  <iface>: http://<ip>:<port>/#token=<token>`. If `boundHost` is a loopback address, write only the loopback line. If it is a specific non-loopback IP, write the warning and `Skill Hub web UI: http://<boundHost>:<port>/#token=<token>`.
  4. `TestListenRule` table rows: zero usable; one usable; two usable on two interfaces; two IPs on one interface; a down interface with two IPs (counts 0); IPv6-only interface; link-local `169.254.1.1` plus one usable (counts 1); `loopbackOnly` with three usable; explicit `10.0.0.5:8080` with three usable; explicit `bad` → error; explicit `127.0.0.1:0` accepted.
  5. `TestStartupOutput`: wildcard case contains the warning and one line per usable IP with the interface name; loopback case does not contain `WARNING`.
- Success criteria: both tests pass.
- Verify: `go test -count=1 -v -run '^(TestListenRule|TestStartupOutput)$' ./internal/delivery/web/` exits 0 and prints `--- PASS: TestListenRule` and `--- PASS: TestStartupOutput`.

### Task 1.6 — Embedded assets and SPA fallback
- Goal: serve the built UI when present and a clear page when it is not.
- Target files: create `internal/delivery/web/assets.go`, `internal/delivery/web/dist/.gitkeep` (empty file), `internal/delivery/web/assets_test.go`; modify `.gitignore`.
- Steps:
  1. `//go:embed all:dist` into `var distFS embed.FS`; `func DefaultAssets() fs.FS` returns `sub` from `sub, err := fs.Sub(distFS, "dist")`, and returns `distFS` itself if `err != nil` (it cannot fail for the literal `"dist"`; do not panic).
  2. Handler for non-`/api/` `GET`/`HEAD`: if the cleaned path names a file in `Assets` (and is not `/`), serve it; paths under `/assets/` get `Cache-Control: public, max-age=31536000, immutable`, others `no-cache`. Otherwise, if `Accept` contains `text/html`, serve `index.html` with `Cache-Control: no-store`; if `index.html` is absent, respond 503 with `Content-Type: text/html; charset=utf-8` and a body containing `Skill Hub web UI is not built. Run make web-build, then rebuild skillhub.` Otherwise 404 with an empty body.
  3. Unknown `/api/` paths return 404 with the JSON envelope from `app.NewInvalidRequestError("Unknown API path.", "Check the API version and path.")`, never HTML.
  4. `.gitignore`: append `internal/delivery/web/dist/*`, `!internal/delivery/web/dist/.gitkeep`, `web/node_modules/`, `web/test-results/`, `web/playwright-report/`, `web/.e2e/`.
  5. `TestAssetsServing` with `fstest.MapFS`: present `index.html` + `assets/app-123.js`; deep link `/skills/x` with `Accept: text/html` → 200 index with `no-store`; `/assets/app-123.js` → immutable cache header; `/missing.png` without HTML accept → 404; `/api/v1/nope` → 404 JSON; with an empty MapFS, `/` → 503 and the "not built" text.
- Success criteria: test passes; `git status --porcelain internal/delivery/web/dist` shows only `.gitkeep`.
- Verify: `go test -count=1 -v -run '^TestAssetsServing$' ./internal/delivery/web/` exits 0 and prints `--- PASS: TestAssetsServing`; `git check-ignore internal/delivery/web/dist/index.html` prints that path.

### Task 1.7 — Read endpoints with golden files
- Goal: five read endpoints whose JSON equals the app result types.
- Target files: create `internal/delivery/web/routes_read.go`, `internal/delivery/web/fixtures_test.go`, `internal/delivery/web/routes_read_test.go`, `internal/delivery/web/testdata/golden/*.json`.
- Steps:
  1. Register on the mux: `GET /api/v1/session`, `GET /api/v1/home`, `GET /api/v1/skills`, `GET /api/v1/skills/{id}`, `GET /api/v1/skills/{id}/review`.
  2. `/session` returns `{"api_version":1,"skillhub_version":<version.Version>,"workspace_id":"sha256:<hex of sha256 of the absolute workspace path>","workspace_name":<base name of the workspace path>}`.
  3. `/home` → `s.curation.GetCurationHome(ctx, ws)`; `/skills` → validate `state` query (empty or one of `app.SkillStates`; else `writeAppError` with `app.NewInvalidRequestError("Unknown lifecycle state.", "Use draft, active, deprecated or archived.")`), then `s.skills.ListSkills`; `/skills/{id}` → `s.skills.GetSkillDetail`; `/skills/{id}/review` → `s.skills.ReviewSkill`. For the last two, pass `notFound = errors.Is(err, skill.ErrNotFound)` to `writeError`. Return results unchanged with `writeJSON(w, 200, result)`.
  4. `fixtures_test.go`: `newWebWorkspace(t)` copying the recipe of `newMCPWorkspace` (init, create `review-skill`, activate) and `newTestServer(t, root)` returning a `*Server` with token `test-token`, port 7421, fake interfaces with no usable IPs, and an empty `fstest.MapFS`. Helper `get(t, srv, path)` sets `Host: 127.0.0.1:7421` and the bearer token.
  5. `TestReadEndpointsGolden`: cases `session`, `home`, `skills`, `skill-detail` (`review-skill`), `skill-review`, `skill-unknown` (expects 404), `skills-bad-state` (expects 400). Normalize volatile values before comparing (regexes: RFC3339 timestamps → `<TIME>`, `sha256:[0-9a-f]{64}` → `<DIGEST>`, the temp root path → `<WORKSPACE>`, `gen-[A-Za-z0-9_-]+` → `gen-<ID>`, `(OP|RUN|PROP|PRP|INS|OBS)-[A-Za-z0-9_-]+` prefixes → `<PREFIX>-<ID>`, `"skillhub_version":"[^"]*"` → `"skillhub_version":"<VERSION>"`). Golden path `testdata/golden/<case>.json`; write files only when the test flag `-update` is set (`var update = flag.Bool("update", false, "rewrite golden files")`), and always compare status codes.
  6. Generate with `go test ./internal/delivery/web/ -run '^TestReadEndpointsGolden$' -update`, open each golden file, and confirm by reading that bodies are app results (they contain `"schema_version"` and `"status"`), and that `skill-unknown.json` contains `"code": "invalid_request"` (or the compact form). Then run without `-update`.
- Success criteria: the test passes without `-update`; seven golden files exist.
- Verify: `go test -count=1 -v -run '^TestReadEndpointsGolden$' ./internal/delivery/web/` exits 0 and prints `--- PASS: TestReadEndpointsGolden`; `ls internal/delivery/web/testdata/golden/*.json | wc -l` prints `7`.

### Task 1.8 — `skillhub serve web` command and `web` alias
- Goal: the CLI entry point with flags, the listen rule, startup output and clean shutdown.
- Target files: create `internal/delivery/cli/serve.go`, `internal/delivery/cli/serve_test.go`, `internal/delivery/web/serve_test.go`; modify `internal/delivery/cli/root.go`, `internal/delivery/cli/help.go`.
- Steps:
  1. `root.go`: add `case "serve": return runServe(ctx, args[1:], stdout, stderr)` and `case "web": return runServe(ctx, append([]string{"web"}, args[1:]...), stdout, stderr)`.
  2. `serve.go` `runServe`: if `len(args)==0 || args[0] != "web"` → `writeInvalidRequest(stdout, stderr, false, "serve requires the web subcommand", "Run `skillhub serve web`.")`. Parse the remaining flags in the style of `mcp.go`: `--workspace <path>`, `--addr <host:port>`, `--loopback-only`, `--allow-host <host[:port]>` (repeatable), `--no-open`, `--dev`. Unknown flags → `invalid_request` with FIX `Run skillhub help serve.`.
  3. Explicit address: `--addr`, else the `SKILLHUB_WEB_ADDR` environment variable, else empty. `--dev` with an explicit non-loopback host → `invalid_request` ("--dev only listens on loopback"). `--dev` implies `--loopback-only` when no explicit address is set.
  4. Resolve the workspace with `resolveWorkspace`; on error use `writeWorkspaceResolutionError`.
  5. Token: 32 bytes from `crypto/rand`, hex encoded.
  6. `addr, err := web.ChooseListenAddr(explicit, loopbackOnly, 7421, ifaces)`; `ln, err := net.Listen("tcp4", addr)`; on error print with `termui` `p.Error("The web server could not listen on "+addr+".", err.Error(), "Stop the process that owns the port (for example `ss -ltnp | grep 7421`), or pass --addr.")` and return 1. Never retry another port.
  7. Build `web.Options` with the bound port (`ln.Addr().(*net.TCPAddr).Port`), call `web.New`, write `web.WriteStartup` to `stdout` (with `--dev`, write instead the single line `Skill Hub web UI (dev): http://127.0.0.1:5421/#token=<token>`, because the browser loads the UI from the Vite dev server), and unless `--no-open` open the loopback URL (or the bound-IP URL when bound to a specific IP) with the platform opener (`xdg-open` on Linux, `open` on macOS, `rundll32 url.dll,FileProtocolHandler` on Windows) started with `exec.Command(...).Start()`; on failure print one notice line to stderr and continue.
  8. `srv.Serve(ctx, ln)`; return 0 on clean shutdown, 1 on error.
  9. `help.go`: add the line `  serve web Run the local web UI` directly under the `mcp serve` line in the global list, and a `"serve"` entry in `commandUsage` documenting every flag and the listen rule in one sentence ("Listens on 0.0.0.0 when this machine has two or more non-loopback IPv4 addresses, otherwise on 127.0.0.1; --addr or --loopback-only override."). Add a `"web"` entry that says it is an alias of `skillhub serve web`.
  10. `cli/serve_test.go` `TestServeWebCommand` (table plus one live case): `serve` → exit 2 and stderr contains `skillhub serve web`; `serve web --addr bad` → exit 2; `serve web --bogus` → exit 2; `help serve` → exit 0 and stdout contains `--loopback-only`; `web --help` → exit 0 and stdout contains `alias`; live: run `RunContext` in a goroutine with `serve web --addr 127.0.0.1:0 --no-open --workspace <initTestWorkspace(t)>` and a cancellable context, read the printed URL from a synchronized buffer until it appears (timeout 10 s), `GET /api/v1/session` with the token and `Host` from the URL → 200, without the token → 401, cancel → `RunContext` returns 0.
  11. `web/serve_test.go` `TestServeWildcardAnswersOnLoopback`: `net.Listen("tcp4", "0.0.0.0:0")`, serve a test server, request `http://127.0.0.1:<port>/api/v1/session` with the token → 200, without → 401.
- Success criteria: both tests pass; `skillhub help` lists `serve web`.
- Verify: `go test -count=1 -v -run '^(TestServeWebCommand|TestServeWildcardAnswersOnLoopback)$' ./internal/delivery/cli/ ./internal/delivery/web/` exits 0 and prints both `--- PASS:` lines; `go run ./cmd/skillhub help | grep -c 'serve web'` prints `1`.

### Task 1.9 — Phase close
- Goal: prove the phase is complete and record it.
- Target files: create `plans/261003-1645-webui-v1-implementation/reports/phase-01-report.md`.
- Steps:
  1. Run `make check`.
  2. Run `bash plans/261003-1645-webui-v1-implementation/guard/guard.sh check 1`.
  3. Write the report: full guard output (including the two hashes it prints first), `git diff --stat $(cat plans/261003-1645-webui-v1-implementation/guard/baseline/base_commit.txt)`, every deviation from this file with its reason, every Failure Protocol event, open questions.
  4. Commit with a conventional message (for example `feat(web): add local HTTP adapter and serve web command`); no AI attribution.
- Success criteria: guard passes and the report exists.
- Verify: `make check` exits 0; the guard's last line is exactly `GUARD RESULT: PASS (phase 1)`.

## Progress

- [x] Task 1.1 — Preflight
- [x] Task 1.2 — Package skeleton and server lifecycle
- [x] Task 1.3 — Error envelope and status table
- [x] Task 1.4 — Security middleware and auth throttle
- [x] Task 1.5 — Listen rule and startup output
- [x] Task 1.6 — Embedded assets and SPA fallback
- [x] Task 1.7 — Read endpoints with golden files
- [x] Task 1.8 — `skillhub serve web` command and `web` alias
- [x] Task 1.9 — Phase close

## Failure Protocol
If any Verify step does not meet its stated pass condition:
1. You may make **one** fix attempt for that task. Change only the task's target files. Never edit a test's assertions to make it pass, never edit golden files by hand, never touch `plans/261003-1645-webui-v1-implementation/guard/`, `.golangci.yml`, or CI files unless the task lists them.
2. Re-run exactly the same Verify command.
3. If it still fails, STOP this phase. Do not try a second fix and do not reason around the failure.
4. If a `kongming` subagent can be spawned, give it: the phase and task id, the steps you ran, both Verify commands with their full output, and the pass condition. Apply its guidance, then re-run Verify once.
5. Otherwise, or if Verify still fails, report the same evidence to the user and wait.
Record every failure, the fix attempt and the outcome in the phase report.

## Rollback

Revert the phase's commits. Nothing outside `internal/delivery/web`, `internal/delivery/cli/{serve.go,serve_test.go,root.go,help.go}` and `.gitignore` changes in this phase.
