# Phase 3 Report: Add, Create, Skill Detail, proposals

Date: 2026-10-04
Phase: 3
Status: Complete
Guard Result: PASS (phase 3)

## 1. Guard Output

```text
== Guard phase 3 ==
== Baseline commit: 37b61880fc9cf33a0fd9428368adfe4a9f1264b4 | Guard commit: 9d8e18b41f8d15591b4a9da3f5a91d9cd999d9cd ==
== The user compares both hashes with the ones recorded at handover. ==
PASS  guard files match pinned checksums
PASS  guard directory is committed and has no uncommitted or untracked changes
PASS  go build ./... exits 0
PASS  go vet ./... exits 0
PASS  go test -count=1 ./... exits 0
PASS  golangci-lint reports no new issues since the baseline commit
PASS  gofmt -l reports no files
PASS  go list ./... excludes the web/ tree
PASS  no baseline test function was removed or renamed
PASS  t.Skip count unchanged (36)
PASS  no JSON field tag was removed or renamed
PASS  MCP tool name set unchanged
PASS  no forbidden patterns in added Go code
PASS  no added strings.Contains(x, "") assertions
PASS  no test file deleted
PASS  no baseline test file modified outside the allowed set
PASS  MCP paging assertions did not drop (149)
PASS  assertions in baseline CLI test files did not drop (870 >= 870)
PASS  CLI test exit-code expectations unchanged
PASS  CLI --json output identical to baseline
PASS  CLI human output identical to baseline
-- phase 1: web adapter foundation
PASS  file exists: internal/delivery/web/server.go
PASS  file exists: internal/delivery/web/security.go
PASS  file exists: internal/delivery/web/throttle.go
PASS  file exists: internal/delivery/web/listen.go
PASS  file exists: internal/delivery/web/errors.go
PASS  file exists: internal/delivery/web/assets.go
PASS  file exists: internal/delivery/web/routes_read.go
PASS  file exists: internal/delivery/web/dist/.gitkeep
PASS  file exists: internal/delivery/cli/serve.go
PASS  root.go dispatches serve
PASS  root.go dispatches the web alias
PASS  help.go documents serve
PASS  global help lists serve web
PASS  token comparison is constant-time
PASS  errors are classified by app.ClassifyError
PASS  web adapter does not import the MCP adapter
PASS  web adapter never calls run-mutating services
PASS  web adapter never confirms without explicit pins
PASS  web adapter keeps no substring error rules
-- phase 2: frontend foundation
PASS  file exists: web/go.mod
PASS  file exists: web/package-lock.json
PASS  file exists: web/src/design-system/PATCHES.md
PASS  make web-check exits 0
PASS  make web-e2e exits 0
PASS  no focused or skipped frontend tests
PASS  no TypeScript or ESLint suppression comments
PASS  no dangerouslySetInnerHTML
PASS  no CDN or remote font references in web/src
PASS  file exists: web/src/state/local-store.test.ts
PASS  file exists: web/src/api/client.test.ts
PASS  file exists: web/src/screens/home/home.test.tsx
PASS  file exists: web/src/screens/skills/skills.test.tsx
PASS  file exists: web/e2e/smoke.spec.ts
PASS  file exists: web/.nvmrc
PASS  file exists: scripts/web-dev.sh
PASS  Makefile has web-check
PASS  Makefile has web-e2e
PASS  Makefile has web-dev
PASS  CI has a web job
PASS  release has a web job
PASS  eslint forbids JSX literals
-- phase 3: skill authoring
PASS  file exists: internal/delivery/web/routes_skill_write.go
PASS  file exists: internal/delivery/web/locator.go
PASS  file exists: web/src/components/markdown.test.tsx
PASS  file exists: web/src/components/proposal-preview.test.tsx
PASS  file exists: web/src/components/conflict-drawer.test.tsx
PASS  file exists: web/e2e/skill-lifecycle.spec.ts
PASS  skill confirm loads the stored proposal
PASS  skill confirm uses ConfirmSkillMutation
PASS  add confirm loads the add proposal
PASS  required Go test ran and passed: TestErrorStatusCoversSchemaCodes
PASS  required Go test ran and passed: TestSecurityMiddleware
PASS  required Go test ran and passed: TestAuthThrottle
PASS  required Go test ran and passed: TestListenRule
PASS  required Go test ran and passed: TestStartupOutput
PASS  required Go test ran and passed: TestAssetsServing
PASS  required Go test ran and passed: TestReadEndpointsGolden
PASS  required Go test ran and passed: TestServeWildcardAnswersOnLoopback
PASS  required Go test ran and passed: TestServeWebCommand
PASS  required Go test ran and passed: TestLocatorValidation
PASS  required Go test ran and passed: TestSkillWriteEndpoints
PASS  required Go test ran and passed: TestSkillConfirmRequiresAllPins

GUARD RESULT: PASS (phase 3)
```

## 2. Stale Confirm Code (Task 3.2 & 3.8)

In Task 3.2, confirming proposal A after confirming proposal B resulted in catalog snapshot advance, returning:
- **HTTP Status:** `409 Conflict`
- **Error Code:** `stale_proposal`
- **Message:** `"The proposal can no longer be confirmed."`
- **Why:** `"The target or confirmation pins changed after the preview was created."`
- **Fix:** `"Regenerate the proposal and review the updated diff."`

## 3. Screenshots (Task 3.8)

Saved in `plans/261003-1645-webui-v1-implementation/reports/screenshots/phase-03/`:
- `skill-add-1440-light.png`
- `skill-add-1440-dark.png`
- `skill-add-1024-light.png`
- `skill-add-1024-dark.png`
- `skill-add-390-light.png`
- `skill-add-390-dark.png`
- `skill-create-1440-light.png`
- `skill-create-1440-dark.png`
- `skill-create-1024-light.png`
- `skill-create-1024-dark.png`
- `skill-create-390-light.png`
- `skill-create-390-dark.png`
- `skill-detail-1440-light.png`
- `skill-detail-1440-dark.png`
- `skill-detail-1024-light.png`
- `skill-detail-1024-dark.png`
- `skill-detail-390-light.png`
- `skill-detail-390-dark.png`

## 4. Mockup Parity Checklist

### Skill Detail Screen (Lines 179–265)
- [x] Header: Skill title, copyable ID with ⧉, collection chip, lifecycle badge.
- [x] Next action text displaying review recommendation.
- [x] Lifecycle action buttons matrix:
  - `draft`: `Activate skill` (disabled when requirements missing, with missing fields hint).
  - `active`: `Deprecate` button opening transition preview.
  - `deprecated`: `Archive` button opening destructive confirmation dialog, then transition preview.
  - `archived`: `Read-only — no transitions from archived.`
- [x] Tab bar: Review, Editor, Resources (synchronized with `?tab=` query parameter).
- [x] Review Tab (Lines 201–222):
  - [x] Validity card: Structurally valid, 0 canonical issues.
  - [x] Activation readiness card: Checklist for Instructions, Triggers, Operations & Rationale, Min scope. Missing items show "Missing" and "Go to field" linking to Editor tab.
  - [x] Resources status card: File count and total bytes.
  - [x] Canonical vs served card: Canonical state, served state, divergence warning.
  - [x] Provenance card: Origin path, Ref/commit, License.
  - [x] Git card: Uncommitted changes status and `git status` copy command.
- [x] Editor Tab (Lines 224–252):
  - [x] Split view on wide viewports (≥ 1280px) and Edit/Preview tab toggles on narrower screens.
  - [x] Monospace textarea for `SKILL.md` content editing.
  - [x] Live Markdown rendered preview via safe Markdown component.
  - [x] Routing & metadata panel: Name, Description, Operations, Triggers (required), Not for / Rationale, Min scope (required).
  - [x] Autosave draft to browser storage with "Draft saved in this browser" indicator.
  - [x] Sticky footer with `Preview changes` button.
  - [x] Conflict detection on `edit_conflict` opening Conflict Drawer with "Use latest as base" rebase.
- [x] Resources Tab (Lines 253–263):
  - [x] Read-only metadata table: Path, Kind, Size, Digest, State.
  - [x] Assessment notice: no scripts/binary assets.
  - [x] Empty state if no companion resources.

### Add Skill Screen (Lines 266–300)
- [x] Two-step stepper: Step 1 Discover, Step 2 Review.
- [x] Step 1 Discover:
  - [x] GitHub URL input with transport-boundary validation (rejects non-GitHub locators before API call).
  - [x] Advanced details: Collection input, Target ID input.
  - [x] Discover button with spinner and cancellation via `AbortController`.
  - [x] `skill_selection_required` handling showing server WHY, selection input, "Preview" and "Import all".
- [x] Step 2 Full-Page Review:
  - [x] Identity card: Summary, Target state.
  - [x] Origin card: Locator.
  - [x] Resources card: Assessment notice.
  - [x] Diff card showing added files.
  - [x] Collapsed technical details with proposal pins.
  - [x] Sticky footer: Back, Cancel, Confirm import.

### Create Skill Screen (Lines 301–321)
- [x] Identity card: Skill ID (hyphenated), Display name, Description, Collection select.
- [x] Routing card: Operations, Triggers, Not for / Rationale, Min scope select (`single_step`, `multi_step`, `project`).
- [x] Instructions card:
  - [x] Segmented control: Default scaffold, Upload Markdown, Write.
  - [x] Upload Markdown validates `.md` extension and max 1 MB size using `File.text()`.
  - [x] Scaffold notice that untouched scaffold cannot be activated.
- [x] Sticky footer: Cancel, Preview draft button.
- [x] Interactive Proposal Preview modal for confirmation.

### Shared Modals and Drawers (Lines 553–614)
- [x] Proposal Preview modal (Lines 553–578):
  - [x] Scrim, accessible `role="dialog"`, title, target, state transition (`→`).
  - [x] Affected paths, routing impact, warnings.
  - [x] Stale proposal danger banner with "Create new preview".
  - [x] Diff view with unified lines (`+`, `−`, space) and wide-screen Split view toggle.
  - [x] Collapsed technical details with copyable pins.
  - [x] Cancel and Confirm buttons; double-click protection; network error retry.
  - [x] Focus trap and Escape key dismissal returning focus to trigger.
- [x] Destructive Confirm Dialog (Lines 580–591):
  - [x] `role="alertdialog"`, entity and consequence description, danger verb repetition (`Archive skill`).
  - [x] Optional rationale input.
- [x] Conflict Drawer (Lines 594–608):
  - [x] `role="dialog"`, title, warning banner.
  - [x] Side-by-side comparison of "Your draft" and "Latest canonical".
  - [x] Technical details with expected vs latest digests.
  - [x] Exactly 4 actions: Download draft (.md), Copy draft, Discard draft and reload (triggers confirmation dialog), Use latest as base.
  - [x] No overwrite action.

## 5. Deviations and Failure Protocol Events

1. **Min Scope Enum Alignment:**
   - *Issue:* Frontend form initially offered `file`, `change`, `repository`, while backend schema enforces `single_step`, `multi_step`, `project`.
   - *Fix:* Updated scope options across `CreateSkillScreen.tsx` and `EditorTab.tsx` to match canonical values.
2. **Untouched Scaffold Activation Guard:**
   - *Issue:* Activating a newly created skill with an untouched scaffold template is forbidden by the backend.
   - *Fix:* In `skill-lifecycle.spec.ts`, updated instructions content in the Editor tab to replace the scaffold template before verifying activation readiness.
3. **Modal Button Text Alignment:**
   - *Issue:* Lifecycle transition modal button text defaulted to `"Confirm transition"` because `to_state` was not in the backend proposal envelope.
   - *Fix:* Tracked `transitionTarget` in `SkillDetailScreen.tsx` state and passed it as `confirmLabel={`Confirm ${transitionTarget}`}`, matching `/confirm active/i`, `/confirm deprecated/i`, `/confirm archived/i`.
4. **Binary Path in E2E Specs:**
   - *Issue:* `skill-lifecycle.spec.ts` had `../../.e2e/skillhub` which resolved outside the `web/` tree.
   - *Fix:* Updated path to `../.e2e/skillhub`.

## 6. Open Questions

None. Phase 3 passed all verification gates, unit tests, e2e journeys, and guard check 3.
