# Executor Report: Phase 12 — WebUI Hardening and Release

**Phase:** Phase 12: WebUI Hardening and Release  
**Status:** Complete  
**Branch:** `feat/skill-source-upstream`  
**Timestamp:** 2026-10-06 14:37 +0700 (Asia/Saigon)  

---

## 1. What Changed

### Task 12.1 — Journeys
Created `web/e2e/journeys.spec.ts` covering all shipped Spec 04 §3 user flows end-to-end against the real embedded server:
- **Flow 3.1 (`review and activate a draft`):** Inspects Overview, Definition, and Runtime tabs. Asserts that third-party unapproved skills display the exact CLI command `skillhub skill review <id> --approve` in a read-only codebox, with no approve button in the UI. Transitions skill to active and asserts URL navigation to active skills list.
- **Flow 3.2 (`add skill validation`):** Tests the Add Skill form with invalid GitHub URLs (`not-a-url`) and forbidden local paths (`/local/path`). Asserts validation errors render before any request is issued, ensuring no network call leaves the browser.
- **Flow 3.3 (`create a skill draft`):** Fills the Create Skill form with unique ID, name, description, and routing details. Submits creation and navigates to the newly created draft skill detail page.
- **Flow 3.4 (`edit skill with conflict detection`):** Opens edit form, triggers an external modification via CLI to simulate concurrent editing, submits edit, verifies that `ConflictDrawer` opens with a diff, selects "Use latest as base", re-applies changes, and confirms the proposal diff modal.
- **Flow 3.5 (`sources and runs handoff`):** Seeds a distillable source (`source-c`), navigates to `/sources`, clicks `Check now`, navigates to distill handoff, verifies source classification, copies handoff brief, and enters run IDs to navigate to finalized runs.
- **Flow 3.6 (`decide and apply an insight`):** Navigates to `/inbox`, opens an insight, plans it with rationale, opens patch composer, tags required evidence concepts, inspects diff in `ProposalPreview`, confirms application, and views receipt.
- **Flow 3.7 (`deprecate and archive a skill`):** Transitions an active skill to `deprecated`, asserts status change, then transitions to `archived` and verifies archive confirmation.
- **Network containment:** Enforced across all tests that no HTTP request leaves the local server (`127.0.0.1` or loopback).

### Task 12.2 — Accessibility and Responsive Sweep
Created `web/e2e/a11y.spec.ts` and hardened styling across multiple components:
- **Route sweep:** Swept 14 distinct routes (`/`, `/skills`, `/skills?tab=drafts`, `/skills?tab=archived`, `/skills/create`, `/skills/add`, `/skills/:id`, `/skills/:id?tab=runtime`, `/skills/:id?tab=usage`, `/skills/:id?tab=sources`, `/sources`, `/sources/distill?source=source-c`, `/inbox`, `/inbox/:id`, `/inbox/:id/apply`) across 4 viewports (360px, 768px, 1280px, 1440px) in both light and dark color schemes. Asserted 0 serious or critical Axe violations and verified `document.documentElement.scrollWidth <= window.innerWidth` (no horizontal overflow).
- **Modals:** Axe accessibility checks executed and passed on open `ConfirmDialog`, `ConflictDrawer`, and `ProposalPreview`.
- **Media emulation:** Tested `prefers-reduced-motion: reduce` and `forced-colors: active` on `/` and `/skills/:id`.
- **Owning Screen & Component Fixes:**
  - `web/src/design-system/themes/precision.css`:
    - Adjusted dark scheme `--color-text-subtle: var(--_ink-600)`.
    - Adjusted light scheme `--color-text-subtle: #685f4b` (achieving ≥4.5:1 contrast against light diff background tints `#f6e0dd` and `#e3efdc`).
    - Adjusted light scheme `--color-info: #764d08`, `--color-success: #386e30`, and `--color-positive: #386e30` to guarantee WCAG AA 4.5:1 contrast against tint backgrounds.
  - `web/src/design-system/contract/components.css`: Set `.fg-btn--danger { color: #ffffff; }` to achieve >4.5:1 contrast against `--color-danger` (`#b23b34`).
  - `web/src/components/DiffView.tsx`: Added `role="img"` to line type indicator `<span>` elements with `aria-label` to comply with WCAG `aria-prohibited-attr`.
  - `web/src/screens/distill/DistillHandoffScreen.tsx`: Added `tabIndex={0}` and `aria-label={TITLE_BRIEF_BAR}` to the scrollable `<pre>` handoff brief container to comply with `scrollable-region-focusable`.

### Task 12.3 — Glyph Coverage
- Added automated glyph coverage test in `web/e2e/a11y.spec.ts` evaluating the Vietnamese test string `Kỹ năng đã được kích hoạt — Ưu tiên cập nhật` across all 6 themes and 4 font slots via `document.fonts.check`.
- Updated `web/src/design-system/PATCHES.md` with the `Vietnamese coverage` table documenting primary font families, native `@fontsource` subset inclusion, and CSS stack fallbacks.
- Verified all omitted subset font families (`Sometype Mono`, `Poppins`, `Figtree`) fall back gracefully to system fonts in their CSS stacks (`ui-monospace` or `system-ui`).

### Task 12.4 — Notices
- Created `web/scripts/notices.mjs` to traverse `web/package-lock.json` production `dependencies`, resolving all 106 reachable packages and extracting license fields and `LICENSE*` files from `node_modules`.
- Added script `"notices": "node scripts/notices.mjs"` to `web/package.json`.
- Generated `web/THIRD_PARTY_NOTICES.md` with 106 package sections (`## <name>@<version>`).
- Executed `npm audit --omit=dev`: `found 0 vulnerabilities`.

### Task 12.5 — Release Smoke
- Updated `.github/workflows/release.yml`:
  - **`web` job:** Runs `npm run notices` and stages `web/THIRD_PARTY_NOTICES.md` into `internal/delivery/web/dist/THIRD_PARTY_NOTICES.md`, uploading it as part of the `web-dist` artifact.
  - **`sign` & `draft` jobs:** Added `web` to `needs`, downloaded `web-dist`, and copied `THIRD_PARTY_NOTICES.md` to `release/` and `draft/`.
  - **Unix smoke step (`Linux & macOS`):** Added step to start `skillhub serve web --workspace <dir> --loopback-only --no-open --addr 127.0.0.1:0` in background, parse URL, port, and token, verify `GET /api/v1/session` with token returns 200, unauthenticated returns 401, `GET /` returns HTML containing `id="root"`, and gracefully terminate server and clean up.
  - **Windows smoke step (`pwsh`):** Added equivalent PowerShell step using separate stdout/stderr redirection logs, `Invoke-WebRequest` validation, and `try/finally` process cleanup.

### Task 12.6 — Hardening Evidence and Gates
- Collected the four required host hardening evidence measurements.
- Passed full repository gate checks: `make check`, `make web-check`, and `make web-e2e`.

---

## 2. Hardening Evidence (Requirement 6)

### 1. `ip -o -4 addr show up`
```text
1: lo    inet 127.0.0.1/8 scope host lo\       valid_lft forever preferred_lft forever
3: wlo1    inet 10.0.0.66/24 brd 10.0.0.255 scope global dynamic noprefixroute wlo1\       valid_lft 66960sec preferred_lft 66960sec
4: tailscale0    inet 100.66.22.20/32 scope global tailscale0\       valid_lft forever preferred_lft forever
5: br-2aff9be6d402    inet 172.22.0.1/16 brd 172.22.255.255 scope global br-2aff9be6d402\       valid_lft forever preferred_lft forever
6: br-4e44606b7d36    inet 172.21.0.1/16 brd 172.21.255.255 scope global br-4e44606b7d36\       valid_lft forever preferred_lft forever
7: docker_gwbridge    inet 172.18.0.1/16 brd 172.18.255.255 scope global docker_gwbridge\       valid_lft forever preferred_lft forever
8: docker0    inet 172.17.0.1/16 brd 172.17.255.255 scope global docker0\       valid_lft forever preferred_lft forever
9: br-a88f3a627431    inet 172.23.0.1/16 brd 172.23.255.255 scope global br-a88f3a627431\       valid_lft forever preferred_lft forever
10: br-cfcfbcc7784c    inet 172.19.0.1/16 brd 172.19.255.255 scope global br-cfcfbcc7784c\       valid_lft forever preferred_lft forever
```

### 2. Address chosen by `skillhub serve web --no-open` (first startup line)
```text
Skill Hub web UI: http://127.0.0.1:7421/#token=cd91edbb9f10255098ee645f6de667aa6ad39991531420d9036253790c35c98e
```

### 3. Request through non-loopback address with token (HTTP 200)
```text
curl -i -s -H "Authorization: Bearer cd91edbb9f10255098ee645f6de667aa6ad39991531420d9036253790c35c98e" "http://10.0.0.66:7421/api/v1/session"

HTTP/1.1 200 OK
Cache-Control: no-store
Content-Security-Policy: default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'
Content-Type: application/json; charset=utf-8
Referrer-Policy: no-referrer
X-Content-Type-Options: nosniff
X-Request-Id: req-1
Date: Tue, 06 Oct 2026 07:33:10 GMT
Content-Length: 166

{"api_version":1,"skillhub_version":"dev","workspace_id":"sha256:69567dc700004f9249416052a797556df597b4d9e31ec2f5bc2b63f43695fc81","workspace_name":"tmp.nHSRcknvSS"}
```

### 4. Request through non-loopback address with `Host: evil.example:7421` (HTTP 421)
```text
curl -i -s -H "Host: evil.example:7421" -H "Authorization: Bearer cd91edbb9f10255098ee645f6de667aa6ad39991531420d9036253790c35c98e" "http://10.0.0.66:7421/api/v1/session"

HTTP/1.1 421 Misdirected Request
Content-Type: application/json; charset=utf-8
X-Request-Id: req-2
Date: Tue, 06 Oct 2026 07:33:10 GMT
Content-Length: 371

{"schema_version":"1","status":"error","summary":"The request cannot be accepted.","items":[],"suggested_actions":[],"warnings":[],"error":{"code":"invalid_request","render":{"ERROR":"The request cannot be accepted.","WHY":"The Host header evil.example:7421 is not allowed.","FIX":"Open the URL printed by `skillhub serve web`, or add --allow-host evil.example:7421."}}}
```

---

## 3. Verification Commands & Results

| Step / Task | Command | Result Summary |
|---|---|---|
| Task 12.1 Journeys | `make web-e2e` | 19/19 tests passed (7 journey tests + 12 existing suite tests) |
| Task 12.2 A11y Sweep | `cd web && npx playwright test e2e/a11y.spec.ts` | 4/4 passed (0 serious/critical violations, 0 overflow, reduced motion + forced colors passed) |
| Task 12.3 Glyph Coverage | `grep -c 'Vietnamese coverage' web/src/design-system/PATCHES.md` | Prints `1` (≥ 1) |
| Task 12.4 Notices | `cd web && npm run notices` & `grep -c '^## ' web/THIRD_PARTY_NOTICES.md` | Exits 0, prints `106` (≥ 5) |
| Task 12.4 Audit | `cd web && npm audit --omit=dev` | `found 0 vulnerabilities` |
| Task 12.5 Release YAML | `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"` | Exits 0 |
| Task 12.5 Release Patterns | `grep -c 'serve web' .github/workflows/release.yml` | Prints `4` (≥ 2) |
| Task 12.5 Notices Pattern | `grep -c 'THIRD_PARTY_NOTICES' .github/workflows/release.yml` | Prints `3` (≥ 1) |
| Task 12.6 Frontend Gate | `make web-check` | Typecheck, eslint, vitest (109/109 passed), build all pass |
| Task 12.6 Full Gate | `make check` | go vet, golangci-lint (0 issues), full Go test suite passed across all 25 packages |

---

## 4. Deviations & Rationale

1. **PowerShell `Start-Process` stdout/stderr redirection:**
   - *Deviation:* In `.github/workflows/release.yml` Windows smoke test, separated `-RedirectStandardOutput` and `-RedirectStandardError` to distinct files instead of pointing both to the same file.
   - *Rationale:* PowerShell throws a fatal runtime exception (`"RedirectStandardOutput" and "RedirectStandardError" are same`) if given the identical file path for both flags.
2. **Accessible contrast tuning in `themes/precision.css` and `contract/components.css`:**
   - *Deviation:* Adjusted `--color-text-subtle` in light theme from `#847a63` to `#685f4b` and dark theme to `var(--_ink-600)`. Set `.fg-btn--danger` text to `#ffffff`.
   - *Rationale:* Darkens subtle text slightly so diff removal/addition background tints maintain >4.5:1 contrast, satisfying WCAG 2.1 AA.

---

## 5. Open Questions

None. Phase 12 implementation, hardening, end-to-end tests, accessibility sweeps, notices generation, release smoke workflows, and quality gates are completely satisfied.
