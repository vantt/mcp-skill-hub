# Architecture decisions

Each decision records the recommendation, the alternatives that were weighed, and what is given up. The approval status below is authoritative; the executor implements the decisions as recorded here.

## Approval status

- **Approved by the user (2026-10-04), as recommended:** D1 (React, TypeScript, Vite), D3 (build output not committed, CI and release build it), D4 (six themes, accents, light, dark, system), D5 (self-hosted fonts, loaded per theme).
- **Approved by the user (2026-10-04):** D14, the `skillhub serve web` command and the listen rule, using the "two or more non-loopback IPs" trigger.
- **Accepted during validation session 1 (2026-10-04):** D2, D6–D12 as recommended (no objection raised); D13 changed by the user to a full move of the paging helpers (see D13).

## Summary

| ID | Topic | Recommendation |
|---|---|---|
| D1 | Frontend stack | React + TypeScript + Vite, `react-router`, TanStack Query, npm, Vitest, Playwright |
| D2 | API shape | JSON over HTTP under `/api/v1`; response bodies are the existing `app` result types; errors are the existing result envelope |
| D3 | Assets: build, embed, serve | Source in `web/`; build output embedded with `go:embed`; build output is not committed; CI and release build it |
| D4 | Appearance scope | Deliver what the mockup shows: scheme (light, dark, system), six themes, per-theme accent |
| D5 | Fonts | Self-host, loaded per theme on demand; no CDN requests |
| D6 | Dev workflow and ports | Port 7421 for the server, Vite on `127.0.0.1:5421`, one `make web-dev` command (forces loopback), no port auto-increment |
| D7 | Server security | Per-run session token, Host and Origin allowlists, auth throttling, strict CSP, sanitized Markdown; the listen address follows D14 |
| D8 | Porting the mockup | Port to TSX by hand using the same `.fg-*` classes; do not depend on the mockup runtime or the compiled design-system bundle |
| D9 | i18n readiness | English-only now through a typed message catalog; Vietnamese is a later drop-in |
| D10 | Browser-local state | One namespaced, versioned local-storage layer scoped per workspace |
| D11 | `not_found` handling | Web adapter returns HTTP 404 for missing objects; no change to the shared error registry |
| D12 | Help button in the mockup | Remove it; it has no contract or destination |
| D13 | Inbox pagination ownership | Extract the cursor and paging helpers out of the MCP adapter into a shared package; both adapters use it |
| D14 | Command and listen rule | `skillhub serve web` (alias `skillhub web`); one wildcard socket on `0.0.0.0` when the host has two or more non-loopback IPv4 addresses, otherwise loopback |

---

## D1. Frontend stack

**Recommendation.** React, TypeScript (strict), Vite, `react-router`, TanStack Query for server state, plain CSS from the fgDesign System. Tests: Vitest and Testing Library for components, Playwright for end-to-end runs against the real `skillhub serve web` binary. Package manager: npm with a committed lockfile and `npm ci` in CI. Exact versions are pinned at the start of phase 2 after checking current releases.

**Why.** The mockup is already React-shaped (class component, state, handlers). The Editor, Patch Composer and drafts need real client state, which rules out server-rendered pages. The design system already ships a React companion and plain CSS roles.

**Alternatives.**
- *Go templates plus HTMX.* No Node toolchain and a simpler release, but the Editor split view, composer coverage counter, drafts and conflict drawer would be hand-rolled DOM code. Rejected for the interactive screens.
- *Preact.* Smaller bundle, but saves little in a Go binary and adds ecosystem friction for Testing Library and Playwright helpers.

**You give up.** A Node toolchain is required to build the UI (not to build or test the Go code).

## D2. API shape

**Recommendation.** Resource-oriented JSON endpoints under `/api/v1`. Each handler maps to exactly one `internal/app` method. Success bodies are the existing `app` result types serialized unchanged, so the CLI `--json`, MCP and web contracts stay identical. Error bodies are the existing result envelope (`app.ErrorResult(app.ClassifyError(err))`), which already matches `schemas/error-envelope.schema.json`. HTTP status is derived from the error code; the frontend branches on `error.code`, never on status alone (the only exception is D11).

**Contract bridge.** Go handler tests write golden JSON files under `internal/delivery/web/testdata/golden/`. The frontend test suite reads those same files as its fixtures, so a backend change that breaks the frontend fails a frontend test. TypeScript types live in `web/src/api/types.ts`, written from the data-contract matrix in spec 04 §1.3.

**Alternatives.** GraphQL or RPC-style endpoints named after MCP tools. Rejected: more machinery for a single client, and resource URLs give clearer deep-link and cache behavior.

**You give up.** A generated client. Types are maintained by hand and checked against the golden files.

## D3. Assets: build, embed, serve

**Recommendation.**
- Frontend source lives in `web/` at the repository root, outside `internal/`, with its own `go.mod` file so the Go toolchain ignores the whole subtree (some npm packages ship `.go` files, which would break `go vet ./...`).
- Vite writes to `internal/delivery/web/dist/`; the Go package embeds it with `//go:embed all:dist`.
- Build output is **not committed**. Only `dist/.gitkeep` is, so `go build ./...` and `go test ./...` always compile. Without a built UI the server answers `/` with a short "UI not built" page while the API keeps working.
- `make web-build` builds locally. CI and the release workflow build the UI once and reuse it for every platform target.

**Alternatives.**
- *Commit the build output.* Source builds get the UI for free, but every UI change adds a large generated diff and merge conflicts.
- *Ship the UI as a separate download.* Breaks the single-binary install story.

**You give up.** `go build ./cmd/skillhub` from a fresh checkout produces a binary without the UI until you run `make web-build`. README already tells source builders to build the binary; one more command is documented in phase 6.

## D4. Appearance scope

**Recommendation.** Deliver the Appearance menu exactly as the mockup shows it: scheme (light, dark, system), six themes (precision, beRich, ClickUp-style, terminal, atelier, moday-style), and a per-theme accent set. `precision` is the default.

**Why.** The mockup is the approved visual authority, and the design system's six themes are plain CSS plus fonts. Brief 05 only specified light, dark and system, so this is a scope decision for you.

**Alternative.** Ship `precision` with light, dark and system only and keep the theme architecture ready. This cuts roughly 8 hours of work and removes about 14 font families from the build. It is a clean later addition because themes are only attributes on `<html>`.

**You give up (if you keep six themes).** More font files in the binary (mitigated by D5) and six visual variants to include in the accessibility and responsive checks.

## D5. Fonts

**Recommendation.** Self-host fonts through npm font packages, restricted to the Latin, Latin-extended and Vietnamese subsets. Each theme's fonts load only when that theme is selected; the default theme's fonts load eagerly. The design system's theme CSS contains `@import` lines for Google Fonts; those lines are removed from the vendored copy and the change is recorded in `web/src/design-system/PATCHES.md`.

**Why.** Skill Hub is local-first and works offline; a localhost tool that silently calls a third-party CDN is surprising and breaks offline use. The mockup itself uses the CDN, so this is a deliberate departure.

**Open verification.** Vietnamese glyph coverage differs per typeface (the later Vietnamese locale depends on it). Phase 6 checks every theme's faces against a Vietnamese sample string and sets a system-font fallback for any face that lacks the glyphs. A binary-size budget is measured in phase 2 and recorded in the phase report.

**Alternative.** Keep the CDN as in the mockup. Smaller binary, but online-only fonts and an external request on every load.

## D6. Dev workflow and ports

**Recommendation.**
- `skillhub serve web` uses port 7421 and picks its address by the D14 rule; `SKILLHUB_WEB_ADDR` or `--addr` overrides it.
- `make web-dev` starts the API (with `--dev --loopback-only`) and the Vite dev server (`127.0.0.1:5421`, proxying `/api`) in one foreground terminal, with a trap that stops both on exit.
- Per `.claude/rules/process-management.md`: before starting, the target checks whether either port is already bound and, if so, **fails with the owner's PID and a stop hint instead of picking another port**. Nothing is daemonized, and no PID is left behind.
- Agents running the dev server must start it through the harness background facility, record command, PID and port, and stop it when the task ends.

**You give up.** Two simultaneous worktrees need different ports; set `SKILLHUB_WEB_ADDR` and the Vite port variable for the second one.

## D7. Server security

A server that can edit canonical skills and clone repositories must not be usable by anyone except the person who started it. Binding to loopback alone would not be enough (any website can send requests to `127.0.0.1`, other local users can connect to the port, and DNS rebinding can make a hostile page look same-origin), and D14 additionally exposes the server to the network on multi-IP hosts, where the session token becomes the only barrier.

**Recommendation (all required).**
1. Bind as D14 specifies: loopback by default, one wildcard socket on multi-IP hosts, and an explicit `--addr` always wins.
2. Generate a random 256-bit session token per run. `skillhub serve web` opens the loopback URL `http://127.0.0.1:7421/#token=<token>` in the browser (or prints it); when listening beyond loopback it also prints one such URL per reachable address. The fragment never reaches server logs or `Referer`. The page stores the token in `sessionStorage`, removes it from the address bar, and sends `Authorization: Bearer <token>` on every API call. The server compares in constant time and returns 401 otherwise.
3. Reject any request whose `Host` is not on the allowlist: loopback names, this machine's own interface IPv4 addresses (re-read every few seconds so DHCP and VPN changes work), its hostname, and any `--allow-host host[:port]` entries, always paired with the listening port. This blocks DNS rebinding while still allowing LAN access by IP or machine name.
4. For `POST`, `PUT`, `PATCH` and `DELETE`, require `Content-Type: application/json` and a matching `Origin`. Send no CORS headers.
5. Send a strict CSP (`default-src 'self'`, `img-src 'self' data:`, `frame-ancestors 'none'`, `connect-src 'self'`), `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`. Inline styles are allowed because the design system and the mockup use token-based inline styles; inline scripts are not.
6. Render Markdown from skills and insights only through a renderer that does not interpret raw HTML, allows only `http`, `https` and `mailto` links, and never loads remote images. Imported skills are untrusted third-party text.
7. Cap request bodies, set header-read and idle timeouts, and cancel work when the client disconnects (the app already returns `operation_cancelled` and applies nothing).
8. Throttle failed authentication per remote address (after 20 failures in a minute the address gets 429 with `Retry-After` for a minute), log the source address of failures, ignore `X-Forwarded-*` headers, and never log the token.
9. When the server listens beyond loopback, print a stderr warning at startup that the UI is reachable from other devices over plain HTTP, followed by every reachable URL.

**You give up.** Opening a bookmarked URL without the token shows a "session expired, reopen from the terminal" screen. This is intentional.

**Residual risk you accept with D14.** Traffic is plain HTTP, so anyone who can observe the network path can read the token and skill content. TLS is not part of this plan; it is recorded as an open item in the red-team report. The token is 256-bit, so guessing it is not feasible; sniffing is the realistic attack.

**Alternative.** Loopback plus Host check only. Simpler, but any local user or any browser tab could mutate the workspace. Rejected per the threat-model rule: the server stores and mutates user-owned canonical files.

## D8. Porting the mockup

**Recommendation.** Rewrite each mockup route as TSX components using the same `.fg-*` classes and token-based inline styles, so the result stays visually faithful. Convert `<sc-if>` and `<sc-for>` to JSX, `{{ x }}` bindings to props and state, and the mockup's built-in sample data to real API data. Repeated inline layout styles (three or more uses) move into a small shared stylesheet; one-offs stay inline to preserve fidelity. Do not depend on the mockup runtime (`support.js`) or on `_ds_bundle.js`; the raw `.fg-*` markup is the canonical authoring form of the design system.

**Parity check.** Each screen phase lists the mockup line ranges it covers and ends with a parity checklist plus Playwright screenshots at desktop, tablet and mobile in both schemes. Fields that the mockup shows but spec 04 §1.3 does not allow are dropped, not invented, and listed in the phase report.

## D9. i18n readiness

**Recommendation.** English only in v1, but every visible string goes through a typed catalog (`web/src/i18n/en.ts` plus a `useT()` hook); an ESLint rule forbids literal text in JSX. Plurals and dates use `Intl`. Vietnamese later means adding `vi.ts` and a locale switch. Server-rendered `ERROR/WHY/FIX` text stays English because spec 04 says the UI must not rewrite it; this limit is documented.

## D10. Browser-local state

**Recommendation.** One storage layer (`web/src/state/local-store.ts`) with: key prefix `skillhub.web.v1`, a per-workspace scope taken from an opaque workspace ID the server returns in `/api/v1/session` (a hash of the resolved path, so drafts never leak between workspaces served on the same port), a schema version, try/catch around every read and write, a size cap, and a 30-day expiry for drafts and recent runs. It stores editor drafts, composer drafts, form drafts, recent runs (with the `idempotency_key` needed to resume), the handoff request ID, and the appearance choice (the only global key). The app renders correctly with storage unavailable.

## D11. `not_found` handling

**Finding.** Spec 04 §6 lists `not_found` among the codes that arrive "after `app.ClassifyError`". It does not: `ClassifyError` maps `skill.ErrNotFound` to `invalid_request`, and `not_found` exists only inside the MCP adapter's own error type (`mcpserver/skill_tools.go`, `skill_review_tools.go`). It is also absent from `schemas/error-envelope.schema.json`.

**Recommendation.** Keep the shared registry and schema unchanged (they are public contracts). On the read endpoints that can miss (`GET /skills/{id}`, `/review`, run and insight reads), the web adapter returns **HTTP 404** with the normal classified body. Runs and insights get exported sentinels (`app.ErrDistillRunNotFound`, `app.ErrInsightNotFound`) with the existing messages, so classification is unchanged and the adapter needs no string matching (validation session 1). The frontend shows the Not-found page on 404. Phase 6 corrects spec 04 §6 accordingly.

**Alternative.** Add `not_found` to the registry, schema and `ClassifyError`. This changes CLI `--json` output for missing skills (`invalid_request` becomes `not_found`) and needs your explicit acceptance of a public contract change.

## D12. Help button

The mockup header shows a `?` button with no behavior and no destination in the spec. **Recommendation:** omit it in v1. If you want it, say where it should link (for example the user guide).

## D13. Inbox pagination ownership

**Finding.** Spec 04 §2.8 describes the inbox as a snapshot-bound page (`limit` up to 100, `cursor`, `next_cursor`, `snapshot_expired`). That behavior is implemented in the MCP adapter (`mcpserver/server.go` lines 528–620: `pageOwner`, `encodeCursor`, `decodeCursor`, `normalizeLimit`, `makePage`; used by five tool files, `server.go` itself, the CI fuzz target and `server_test.go`), not in `internal/app`. `InsightService.GetInsightInbox` returns every group.

**Decision (user, validation session 1, 2026-10-04).** At the start of phase 5, move the paging helpers fully into `internal/delivery/paging`, update every MCP caller, and update the MCP tests in place (only `server_test.go` line 355, `hardening_test.go` lines 86–87, and moving `TestOpaqueCursorMultiPageAndIntegrity` with unchanged assertions). The cursor HMAC key is random per process, so behavior is compared by page content, not cursor bytes. The planner had recommended keeping unexported wrappers in `mcpserver` to leave tests untouched; the user chose the full move.

**Alternatives.**
- *Copy the helpers into the web adapter.* Fastest, but two copies of cursor logic drift apart.
- *Return the whole inbox and page on the client.* Changes the behavior spec 04 describes (load more, `snapshot_expired`) and gives up snapshot binding. A local single-user inbox is small, so this is viable, but it needs your approval as a spec change.

**You give up.** Small edits to two MCP test files, permitted by the guard only in phase 5.

## D14. Command and listen rule

**Request.** Add the command `skillhub serve web`. If the machine has several IPs, also listen on `0.0.0.0`.

**Design.**
- `skillhub serve web [--workspace PATH] [--addr HOST:PORT] [--loopback-only] [--allow-host HOST[:PORT]]... [--no-open] [--dev]` is the canonical command, in the same family as the existing `skillhub mcp serve`. `skillhub web` stays as an alias that calls the same handler; removing it later is a one-line change.
- **Listen rule (approved).** Count the machine's usable addresses: IPv4, unicast, on an interface that is up, not loopback, not link-local (`169.254.0.0/16`). Each address counts, so two addresses on one interface count as two. If the count is two or more, listen on `0.0.0.0:7421`; otherwise on `127.0.0.1:7421`. Precedence: `--addr` and `SKILLHUB_WEB_ADDR` (explicit address, used as given) beat `--loopback-only` (forces `127.0.0.1`), which beats the automatic rule.
- **One socket, not two.** A wildcard socket already serves loopback. Opening a second socket on `127.0.0.1` with the same port would be redundant and behaves differently across operating systems, so the plan uses a single listener. IPv4 only, as requested (`0.0.0.0`, not `[::]`).
- **Consequence on this machine.** It has eight non-loopback addresses (wifi, Tailscale, six Docker bridges), so the automatic rule would listen on all of them, including the Tailscale network and the Docker networks. Docker and VPN addresses count toward the trigger because the rule is literal; use `--loopback-only` to opt out.
- **Startup output.** When listening beyond loopback: the warning from D7 item 9, then the loopback URL and one URL per usable address with the interface name, each carrying the token fragment. The browser opener always uses the loopback URL.
- **Security impact.** D7 was widened to match (Host allowlist, auth throttling, cleartext warning). The token remains mandatory for every API call regardless of the address.

**Alternatives that were offered.** Trigger from one non-loopback IP (exposes nearly every networked machine) or never automatic with an explicit `--listen-all` flag (safest). You chose the two-IP trigger.
