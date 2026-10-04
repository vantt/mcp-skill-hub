# Red-team review: WebUI v1 plan

Reviewer: the planner, applying the four personas of the plan skill's red-team workflow, with evidence taken from the repository (not from file names). Each finding has a disposition: **Fixed** (the plan now handles it), **Decision** (needs your approval in [decisions.md](../decisions.md)), or **Accepted** (known, reason given).

## Assumptions reviewer

| # | Finding | Evidence | Disposition |
|---|---|---|---|
| A1 | Spec 04 §6 says `not_found` arrives after `ClassifyError`. It does not: the classifier maps `skill.ErrNotFound` to `invalid_request`; `not_found` exists only inside the MCP adapter's own error type and is missing from `schemas/error-envelope.schema.json`. | `internal/app/error_classify.go` (ErrNotFound branch), `mcpserver/skill_tools.go`, `skill_review_tools.go`, the schema enum | Decision D11 (HTTP 404, no registry change); spec corrected in phase 6 |
| A2 | Spec 04 §2.8 assumes server-side inbox paging as a shared contract, but the paging lives in the MCP adapter. A web copy would duplicate cursor logic. | `mcpserver/server.go` lines 528–620 and its callers; `app.GetInsightInbox` returns all groups | Decision D13 (user chose a full move with MCP test updates); task 5.1 compares MCP behavior by page content because the cursor key is per process |
| A3 | Brief 05 only listed light, dark and system, but the approved mockup also ships six themes and per-theme accents. | Mockup Appearance menu, lines 40–76 | Decision D4 |
| A4 | Documents say "no Web UI in V1" (architecture lines 125, 482, 555; curation lifecycle header; error-code registry; PRD) while spec 04 says "WebUI v1". | grep results during scouting | Fixed: phase 6 updates each after reading it |
| A5 | The mockup's design-system theme CSS imports Google Fonts; a local-first tool should not call a CDN. | `themes/*.css` `@import` lines | Decision D5 |
| A6 | Vietnamese glyph coverage of the theme typefaces is unknown, but a Vietnamese locale is planned. | Not verifiable from the repository | Fixed: explicit coverage check and fallbacks in phase 6; flagged as open verification in D5 |
| A7 | The mockup has a `?` button with no contract. | Mockup header | Decision D12 |

## Failure-mode reviewer

| # | Failure | Plan response |
|---|---|---|
| F1 | `node_modules` contains `.go` files (some npm packages ship them) and breaks `go vet ./...`, `gofmt -l internal`, and lint. | Fixed: `web/` lives outside `internal/` with its own `go.mod` guard; verified in phase 2 step 2 and by the existing Go CI jobs |
| F2 | `go build` fails when the embedded directory is empty or missing. | Fixed: `dist/.gitkeep` is committed; the server serves a "UI not built" page and the API keeps working |
| F3 | Source builders get a binary without the UI. | Accepted and documented (D3); phase 6 adds the `make web-build` instruction |
| F4 | Releases cut before the release workflow builds the UI would ship without it. | Fixed: release workflow changes are in phase 2, validated through the pull-request cross-compile job; no tag before it passes |
| F5 | Token lost after a server restart or in a new tab. | Fixed: 401 screen explains to reopen from the terminal; documented |
| F6 | Port already in use; stale dev servers pile up across worktrees. | Fixed: deterministic ports, fail with owner information, no increment, foreground dev command with trap, per the process-management rule (D6) |
| F7 | A long clone holds the connection or finishes after the user navigates away. | Request context cancels on disconnect (app returns `operation_cancelled`, nothing applied); UI offers Cancel; no global write timeout |
| F8 | Two tabs edit the same skill. | Existing `edit_conflict` path plus the Conflict Recovery Drawer; covered by a Playwright journey in phase 3 |
| F9 | Local storage disabled, full, or from another workspace on the same port. | Fixed: single storage layer, per-workspace scope, versioning, expiry, try/catch, app works without storage (D10) |
| F10 | Preview proposals expire server-side; a refresh loses the modal. | Spec already says the modal closes and the draft survives; covered by a Playwright check in phase 3 |
| F11 | Unknown run IDs return a plain error that the classifier may map generically. | Fixed: characterization test and, only if needed, a typed sentinel in `internal/app` (phase 4) |
| F12 | The web adapter exposes a run-mutating method by accident later. | Fixed: route-table test in phase 4 fails the build |
| F13 | Unknown Dashboard action kinds appear in a later backend version. | Fixed: unknown kinds render read-only without a CTA (phase 2 test) |

## Scope reviewer

| # | Finding | Disposition |
|---|---|---|
| S1 | The user named four steps; the plan has six phases. Phase 1 was split into backend and frontend halves because it bundles a server, a build pipeline and two screens. Phase 6 exists because documentation conflicts, release integration, licensing and sign-off are required work. | Accepted; mapping is in the plan table. No user-requested feature was cut |
| S2 | The plan adds nothing the spec marks out of scope (auth, private repositories, local folder picker, companion-file apply, run listing, force overwrite). | Confirmed |
| S3 | Hidden scope that is real: security layer, self-hosted fonts, third-party notices, i18n scaffolding, Markdown sanitization. | Each justified in decisions.md; none is optional |
| S4 | Effort total (about 214 hours across phases) is an estimate made before pinning versions and measuring font size. | Accepted; phase 2 records measured numbers and the estimate is revisited after it |
| S5 | If D4 is reduced to one theme, about 8 hours and the font volume drop. | Offered as the alternative in D4 |

## Security reviewer

Threat model: the server edits canonical, user-owned files and clones repositories. It runs on the user's machine where other websites and other local users can reach loopback ports and, after the D14 listen rule (one `0.0.0.0` socket when the host has two or more non-loopback IPv4 addresses), possibly other devices on the network. It stores no credentials of its own.

| # | Threat | Control |
|---|---|---|
| X1 | Any website posts to `127.0.0.1` (CSRF) | Token required for every API call; `Origin` must match for mutating methods; JSON content type required; no CORS headers |
| X2 | DNS rebinding makes a hostile page look same-origin | `Host` must be on the allowlist (loopback names, this machine's interface IPs, its hostname, `--allow-host`), always with the listening port |
| X3 | Another local user or another device connects to the port | Per-run random token compared in constant time; wildcard bind only when D14's rule or an explicit `--addr` says so; `--loopback-only` opts out |
| X4 | Imported skill Markdown executes script or beacons out | Markdown rendered without raw HTML, link-scheme allowlist, no remote images; CSP `default-src 'self'`, `img-src 'self' data:`; hostile-input test suite |
| X5 | The browser-facing add endpoint reads local directories because the app accepts local locators | GitHub-URL-only validation at the transport boundary before any app call; tests assert the adapter's own WHY text, proving the app was not reached |
| X6 | Token leaks through logs, history or `Referer` | Token delivered in the URL fragment, moved to `sessionStorage` and removed from the address bar; never logged; `Referrer-Policy: no-referrer` |
| X7 | Supply chain through npm dependencies and vendored third-party CSS | Committed lockfile with exact versions, `npm ci`, `npm audit --omit=dev` reported, vendored files recorded in `PATCHES.md`, third-party notices shipped. `--ignore-scripts` is not used because build tooling may rely on install scripts |
| X8 | Oversized or slow requests | Body size caps, header-read and idle timeouts, client-disconnect cancellation |
| X9 | An unmapped future error code returns a misleading status | Test fails if any schema error code lacks an explicit status mapping |
| X10 | Network exposure lets other devices guess the token | Token is 256-bit; failed authentication is throttled per remote address (429 after 20 failures a minute) and logged with the source address; `X-Forwarded-*` is ignored |
| X11 | Plain HTTP on the network lets an observer read the token and skill content | **Accepted residual risk** (user chose D14). Startup prints a warning naming every reachable URL; `--loopback-only` opts out. TLS is not planned; adding it later would need a certificate story (self-signed warning or a user-supplied pair) |
| X12 | The wildcard rule is literal, so Docker bridge and Tailscale addresses count and expose the UI to the Docker networks and the tailnet (this machine has eight non-loopback addresses) | Documented consequence of D14; the same controls apply; the startup output lists every address so the exposure is visible |
| X13 | A `Host` allowlist that is too strict breaks legitimate LAN-by-name access | `--allow-host` entries; the refusal names the rejected host |

Non-issues recorded: there is no multi-user model or credential store to protect (spec 04 §0 item 1), so authorization beyond the session token is out of scope; CORS is deliberately absent because nothing legitimate calls cross-origin.

## Result

No blocking issue remains. D14 adds one accepted residual risk (X11, cleartext HTTP on the network) that is recorded as an open item. The plan holds as written, subject to your answers on the not-yet-confirmed decisions, above all D7 and D13. Findings A1, A2 and F1 were discovered by reading the code and change the work in phases 1, 4, 5 and 6.
