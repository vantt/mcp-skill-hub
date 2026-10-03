# WebUI Design Brief for Claude Design

**Document:** `docs/use-cases/05-webui-design-brief.md`
**Version:** v1.0 (2026-10-03)
**Companion spec:** [`04-webui-user-flows-and-screen-specs.md`](04-webui-user-flows-and-screen-specs.md). That document owns behavior: routes, data contract, states, errors, responsive rules and accessibility. This brief owns visual direction, layout, English copy and sample data. When the two disagree on behavior or fields, `04` wins.

---

## 0. How to use this brief

1. Attach this brief and the relevant sections of `04` to each Claude Design prompt.
2. Design one batch at a time (section 8). Do not send all screens in one prompt.
3. **UI language is English for v1.** Vietnamese is a later locale (section 7). The Vietnamese CTA strings in `04` are the reference for that later locale. They are not design copy for v1.
4. Every screen needs **light and dark** variants.
5. Use only the sample data in section 6. Do not invent fields. Section 1.3 of `04` lists every field that may appear on screen.

### If a design system or theme is already attached in Claude Design

Section 2 defines a small set of semantic tokens because no design system exists for this product today. If you attach one in Claude Design:

- Skip section 2.2 (palette values) and 2.3 (typography). Use the theme's tokens.
- Keep section 2.1: map every **semantic role** (lifecycle states, diff added/removed, status severities) to a token in the theme. Lifecycle and diff colors carry meaning that the theme probably does not define.
- Check that the theme has a dark mode. If it does not, ask for dark values for the semantic roles before designing.
- Check that the theme's fonts render Vietnamese diacritics (needed for the later locale).
- Keep everything else in this brief: app shell, wireframes, copy, sample data, batches.

---

## 1. Product and visual direction

**Product.** Skill Hub WebUI is a local-first, single-user console for curating agent skills. You use it to import and author skills, review and activate them, watch upstream Git sources, hand distill work to a Curator Agent, and apply improvement insights. It runs on `localhost` against a trusted workspace. It has no login, no team features and no cloud.

**Audience.** Developers and AI-tooling maintainers who are comfortable with Git, diffs and CLI commands. They value accuracy and speed over decoration.

**Direction: a quiet developer tool.** Reference feel: Linear, Vercel dashboard, GitHub's diff views.

- **Dense but calm.** Compact tables and 14px base text, with generous whitespace between sections and none inside rows.
- **Content first.** Diffs, Markdown and identifiers are the product. Chrome stays neutral; color is reserved for meaning (state, severity, diff).
- **Honest states.** "Unavailable" and "Not configured" are first-class visual states, not errors and not hidden.
- **Safety is visible.** Every mutation goes Preview → Confirm. The Proposal Preview is the most important component in the product. Design it carefully.
- **No** illustrations, hero images, gradients, glassmorphism or marketing copy. Empty states use a small icon, one sentence and one CTA.

---

## 2. Theme

### 2.1 Semantic roles (required, theme or no theme)

| Role | Used for |
|---|---|
| `surface/base`, `surface/raised`, `surface/sunken` | Page, cards/modals, code and diff blocks |
| `border/default`, `border/strong` | Dividers, inputs, focus-adjacent outlines |
| `text/primary`, `text/secondary`, `text/muted` | Body, labels, helper text |
| `accent` | Primary buttons, links, focus ring |
| `state/draft` | Lifecycle draft (amber family) |
| `state/active` | Lifecycle active, success (green family) |
| `state/deprecated` | Lifecycle deprecated, warnings (orange family) |
| `state/archived` | Lifecycle archived (neutral) |
| `status/info` | Notices such as fallback or a snapshot reload |
| `status/danger` | Errors, destructive buttons, unreachable source |
| `diff/added-bg`, `diff/added-fg`, `diff/removed-bg`, `diff/removed-fg` | Diff lines (always paired with `+` / `−` prefixes) |

Color is never the only signal. Every badge has an icon and text. Every diff line has a prefix (see `04` §7.2).

### 2.2 Default palette (only when no theme is attached)

| Token | Light | Dark |
|---|---|---|
| surface/base | `#FFFFFF` | `#0B0D10` |
| surface/raised | `#F7F8FA` | `#14171C` |
| surface/sunken | `#F1F3F5` | `#0F1216` |
| border/default | `#E3E6EA` | `#262B33` |
| border/strong | `#C9CED6` | `#3A414C` |
| text/primary | `#14171C` | `#E8EAED` |
| text/secondary | `#4A515C` | `#A9B0BA` |
| text/muted | `#6B7380` | `#7D8592` |
| accent | `#3B5BDB` | `#7C93F0` |
| state/draft | `#B7791F` | `#E3B341` |
| state/active | `#2F855A` | `#4FBF7F` |
| state/deprecated | `#C05621` | `#F0883E` |
| state/archived | `#6B7380` | `#8B939F` |
| status/info | `#2B6CB0` | `#58A6FF` |
| status/danger | `#C53030` | `#F47067` |
| diff/added-bg | `#E6F4EA` | `#12261B` |
| diff/removed-bg | `#FDECEC` | `#2D1517` |

All text and icon pairs must meet WCAG 2.2 AA in both modes. Dark mode follows the OS preference by default and also has a manual toggle in the app shell (Light / Dark / System).

### 2.3 Typography and spacing (only when no theme is attached)

- UI: **Inter**, 14px base, scale 12 / 14 / 16 / 20 / 24. Weights 400 / 500 / 600.
- Code, IDs, digests, paths: **JetBrains Mono**, 13px.
- Both fonts cover Vietnamese diacritics.
- Spacing on a 4px grid. Radius 6px for inputs/buttons and 8px for cards/modals. No shadows except modal and drawer elevation.

---

## 3. App shell

```text
┌──────────────┬────────────────────────────────────────────────────────────┐
│ ◆ Skill Hub  │  Page title                              [◐ theme] [?]     │
│              ├────────────────────────────────────────────────────────────┤
│ ▸ Home       │  (blocking banner area: workspace invalid / recovery /     │
│ ▸ Skills     │   index needs rebuild — full width, above page content)    │
│ ▸ Sources    │                                                            │
│ ▸ Inbox   7  │  Page content                                              │
│              │                                                            │
│ ──────────── │                                                            │
│ Workspace    │                                                            │
│ ● Valid      │                                                            │
│ ● Index ok   │                                                            │
│ ● Git dirty  │                                                            │
└──────────────┴────────────────────────────────────────────────────────────┘
```

- A left sidebar, 232px wide, collapses to icons at tablet width and becomes a bottom tab bar on mobile.
- The Inbox count is `home_summary.pending_insights` and appears only when counts are known.
- The sidebar footer is a compact workspace-health summary from `workspace.*`. It links to Home.
- No global search, no user avatar, no notifications bell. None of these exist in v1.
- A skip link to main content is the first focusable element.

---

## 4. Screen wireframes

These are layout intent, not pixel specs. Every screen still needs the loading, empty, error and success variants from `04` §5.

### 4.1 Home `/`

```text
Home
┌ Next action ─────────────────────────────────────────────────────────────┐
│ 2 source(s) are ready to distill                    [Open sources →]     │
└──────────────────────────────────────────────────────────────────────────┘
┌ Workspace health ──────────────┐ ┌ Summary ─────────────────────────────┐
│ Health        ● Valid          │ │ Active skills          12            │
│ Index         ● Current        │ │ Watching sources        5            │
│ Git           ● Configured     │ │ Sources due             1            │
│ Uncommitted   ▲ Yes            │ │ Pending insights        7  (2 high)  │
│ Recovery      ● None pending   │ │ Failed/interrupted runs 0            │
└────────────────────────────────┘ │ Unavailable sources     0            │
                                   │ Recovery items          0            │
                                   └──────────────────────────────────────┘
┌ Action categories ──────────────────────────────────────────────────────┐
│ Changed sources 2 · Pending insights 7 · Sources due 1 · …              │
│ Blocking decisions — Not configured in v1                               │
└──────────────────────────────────────────────────────────────────────────┘
```

- Show only one Next action card. For copy-command actions, the card shows the command in mono with a `Copy command` button.
- When counts are unknown, every number reads "Unavailable" in muted text. Do not show `0`.

### 4.2 Skills catalog `/skills`

```text
Skills                                   [Add from GitHub] [Create skill]
[Search id or name……]  Lifecycle [All ▾]  Collection [All ▾]
┌───────────────────────────┬─────────────┬──────────────┬──────────────┬────┐
│ Skill                     │ Collection  │ Lifecycle    │ Routing      │    │
├───────────────────────────┼─────────────┼──────────────┼──────────────┼────┤
│ Code Review               │ engineering │ ● Active     │ Routable     │ ⋯  │
│ code-review               │             │              │              │    │
│ PR Description            │ engineering │ ✎ Draft      │ Not routed   │ ⋯  │
└───────────────────────────┴─────────────┴──────────────┴──────────────┴────┘
```

- The row menu contains `Review`, plus `Deprecate` (active rows) or `Archive` (deprecated rows) only.
- Fallback notice: an info banner above the table with the backend `summary` text verbatim.

### 4.3 Add skill `/skills/add` — 2-step wizard

```text
Add skill from GitHub                         Step 1 Discover · 2 Review
GitHub URL *  [https://github.com/acme/agent-skills/tree/main/skills  ]
              Public repository, subfolder or file URL.
▸ Advanced    Collection [default]   Target ID [            ]
                                                   [Cancel] [Discover skills]
```

- **Selection required state:** an info panel shows the backend WHY text verbatim. Below it are a `Skill name or path` input with a `Preview` button and an `Import all` button.
- **Step 2** is a full-page review, not a modal:
  - Identity block.
  - Origin block (repository / ref / commit / path in mono).
  - License row with a warning when unknown or proprietary.
  - Resources list with file count and total size.
  - The `Diff` component.
  - Collapsed `Technical details` (proposal pins).
  - Footer: `[Back] [Cancel] [Confirm import]`.

### 4.4 Create skill `/skills/create`

A single-column form with three section cards: **Identity**, **Routing**, **Instructions**.

- Instructions has a segmented control: `Default scaffold | Upload Markdown | Write`.
- The sticky footer has `Preview draft`.

### 4.5 Skill detail `/skills/:id`

```text
Code Review   code-review ⧉   engineering   ● Active — Routable
Next: Add triggers before activation                     [Deprecate]
[ Review ] [ Editor ] [ Resources ]
```

- **Review tab** is a stack of cards: Validity · Activation readiness (checklist; each missing item links into the Editor) · Resources status · Canonical vs served (divergence banner when they differ) · Provenance · Git.
- **Editor tab, desktop:**

```text
┌ Instructions (SKILL.md) ─────────────────────┬ Routing & metadata ─────┐
│ [Edit | Preview]  split on ≥1280             │ Name / Description      │
│ ┌ markdown ──────────┐┌ rendered ──────────┐ │ Operations  [chips]     │
│ │                    ││                    │ │ Triggers    [chips]     │
│ └────────────────────┘└────────────────────┘ │ Not for / Rationale     │
│                                              │ Min scope   [select]    │
└──────────────────────────────────────────────┴─────────────────────────┘
Draft saved in this browser · 2 min ago            [Preview changes]
```

- **Resources tab:** a read-only tree. Each row shows path, kind, size and digest, plus a `Changed` or `Missing` tag. There are no file-content viewers.

### 4.6 Sources `/sources`

```text
Sources            [Watch new source] [Check due sources] [Check all sources]
Filter [All | Ready to distill]                [Distill with Curator Agent (2)]
┌─┬──────────────────────────┬─────────┬───────────┬──────────┬─────────────┐
│☐│ Source                   │ Monitor │ Trust     │ Revision │ Status      │
│☑│ acme-agent-skills        │ Weekly  │ Community │ 3f9c2a1  │ ◆ Changed   │
│ │ github.com/acme/…@main   │         │ MIT       │ ← 81d04be│ [Check now] │
└─┴──────────────────────────┴─────────┴───────────┴──────────┴─────────────┘
Last check: 3 checked · 1 changed · 2 unchanged · 0 unreachable  (this session)

┌ Open a run ─────────────────────────────────────────────────────────────┐
│ Run ID [RUN-…………………]  [Open]                                             │
│ Recent runs on this browser  (not a full workspace history)             │
│ RUN-7K2M9QX4B1D8F3A6  acme-agent-skills  Awaiting decision   [Remove]   │
└──────────────────────────────────────────────────────────────────────────┘
```

- The checkbox is enabled only for rows that are ready to distill.
- The revision cell shows `current ← distilled`, or "Never distilled".

### 4.7 Watch source `/sources/watch`

A single-column form with these fields: GitHub URL*, Source ID, Ref, Repository path, Monitoring (switch), Cadence (Daily / Weekly / Manual; locked to Manual when monitoring is off), Trust, License. The footer has `Preview`.

### 4.8 Distill handoff `/sources/distill`

```text
Distill with Curator Agent
Selected sources (2)   acme-agent-skills 3f9c2a1 ← 81d04be · …
┌ Handoff for your Curator Agent ─────────────────────────── [Copy handoff] ┐
│ (mono, scrollable) idempotency_key: hnd-5e1a…                             │
│ 1. Call curation_run_start with source_ids=[…] …                          │
└───────────────────────────────────────────────────────────────────────────┘
Paste run IDs returned by the agent
[ RUN-…  one per line                                         ] [Open runs]
ⓘ The WebUI never starts a run. Your agent does.
```

### 4.9 Run `/sources/runs/:id`

- **Header:** run ID ⧉, source, a state badge and `Attempt 1`.
- **Key-value list:** from → to revision, package digest, changed resources (collapsible list of path + status).
- **State panel** changes by state (`04` §2.6 table):
  - `awaiting_decision` lists each outstanding decision (kind, resource, question) and a required **Decision** textarea, then `Copy resume handoff`.
  - `finalized` shows coverage counts and finding/comparison/insight counts, plus `Open Inbox →`.
- **Footer:** `Refresh status` · `Copy resume handoff` · `Cancel run` (danger, secondary).
- When resume is unavailable, an info panel explains that this browser does not have the run's handoff key.

### 4.10 Inbox `/inbox`

```text
Inbox                     Status [All ▾] Category [All ▾] Priority [All ▾]
Filtering 3 loaded groups
▾ code-review · reliability
  ▲ High  Flag unbounded retry loops in reviewed code     Pending
          Score 82 · Impact high · 2 sources · 3 findings      [Open →]
  ⚠ Stale evidence
                                                          [Load more]
```

### 4.11 Insight detail `/inbox/:id`

- **Left column:** recommendation (large) · rationale · findings list · comparisons (subject, verdict, trade-offs, stale tag) · decision history.
- **Right column:** status badge and the actions valid for that status (`Plan`, `Reject`, `Obsolete`, `Reopen`, `Compose patch`). `Compose patch` is disabled when the evidence is stale, with a visible reason.
- Decisions open a small modal with a required rationale. Obsolete uses the destructive variant.

### 4.12 Patch composer `/inbox/:id/apply`

```text
Compose patch · INS-code-review--flag-unbounded-retries
┌ skills/engineering/code-review/SKILL.md ───┬ Evidence mapping  3/4 ────┐
│ (full-height markdown editor)              │ OBS-…--retry-loop-guidance│
│                                            │   concept [retry limits ] │
│                                            │   + Add concept           │
│                                            │ OBS-…--backoff  Unmapped  │
└────────────────────────────────────────────┴───────────────────────────┘
[Save local draft]  Mapped 3 of 4 required             [Cancel] [Preview apply]
```

- `Preview apply` stays disabled until coverage is complete, with the reason shown next to it.

### 4.13 Shared components

- **Proposal Preview (modal, max 960px; full-screen on mobile):**
  - Header: operation, target, `Draft → Active` style transition.
  - Summary: affected paths, routing impact, warnings.
  - The `Diff` component, with a toggle between Unified and Split (Split only on wide screens).
  - Collapsed `Technical details` (proposal ID, digest, base version; mono with copy).
  - Footer: `[Cancel] [Confirm …]`.
  - Stale variant: an inline danger notice, Confirm disabled, and `Create new preview`.
- **Conflict Recovery Drawer (right side, 640px):** your draft vs the latest content, the digests in technical details, and the actions `Download draft (.md)`, `Copy draft`, `Use latest as base`, `Discard draft and reload`.
- **Destructive confirmation:** names the entity and the consequence. The danger button repeats the verb (`Archive skill`).
- **Command block:** mono command plus `Copy command`. Used everywhere the WebUI defers to the CLI.
- **Receipt toast:** "Applied · OP-…" with copy. Long errors never go in a toast.

---

## 5. English copy

### 5.1 Navigation and status labels

| Context | Label |
|---|---|
| Nav | Home · Skills · Sources · Inbox |
| Lifecycle | Draft — Not yet usable by agents · Active — Routable · Deprecated — Not preferred for new work · Archived — Removed from routing |
| Routing column | Routable · Not routed |
| Unknown counts | Unavailable |
| Category availability | Available · Not configured in v1 · Unavailable until the workspace and index are ready |
| Source status | Unreachable (last check) · Changed · Distill pending · Never distilled · Up to date |
| Source filter | All · Ready to distill |
| Check item | Up to date · Needs analysis · Changed · Unreachable |
| Run state | Prepared · In progress · Awaiting decision · Failed · Finalized · Cancelled |
| Insight status | Pending · Planned · Rejected · Obsolete · Incorporated |
| Insight priority | Low · Medium · High · Critical |
| Evidence mapping | Unmapped · Mapped |

### 5.2 Calls to action

| Where | CTA |
|---|---|
| Home | Copy command · Open sources · Open run · Open Inbox · Check all sources · Check due sources |
| Skills | Add from GitHub · Create skill · Review · Deprecate · Archive |
| Add skill | Discover skills · Preview · Import all · Back · Cancel · Confirm import |
| Create skill | Preview draft · Confirm create |
| Skill detail | Activate skill · Preview changes · Reload · Go to field |
| Sources | Watch new source · Check now · Distill with Curator Agent · Open · Remove from this browser |
| Handoff / Run | Copy handoff · Open runs · Refresh status · Copy resume handoff · Cancel run |
| Inbox / Insight | Load more · Plan · Reject · Obsolete · Reopen · Compose patch |
| Composer | Save local draft · Preview apply · Add concept · Cancel |
| Conflict | Download draft (.md) · Copy draft · Use latest as base · Discard draft and reload |

Avoid `OK` and `Submit`. Destructive buttons repeat the verb and the entity.

### 5.3 Empty-state copy

| Surface | Copy | CTA |
|---|---|---|
| Home, no action | Nothing needs attention. | — |
| Skills, none | No skills yet. | Add from GitHub |
| Skills, filtered | No skills match these filters. | Clear filters |
| Sources, none | No sources are being watched. | Watch new source |
| Recent runs | No runs opened on this browser yet. | Distill with Curator Agent |
| Inbox, none | No pending or planned insights. | — |
| Inbox, filtered | No matches in the loaded groups. | Load more / Clear filters |

Error copy is rendered from the backend's ERROR / WHY / FIX text. Do not write custom error sentences in mockups. Use the samples in §6.

---

## 6. Sample data

These are illustrative values shaped exactly like the contract. Use them consistently across screens.

**Workspace / Home**
- `health: valid`, `index: current`, `git_configured: true`, `git_dirty: true`, `recovery_pending: false`.
- Top action: `distill_changed_sources`, count 2, summary "2 source(s) are ready to distill".
- `home_summary`: active_skills 12, watching_sources 5, sources_due 1, pending_insights 7, pending_high_value_insights 2, failed_or_interrupted_runs 0, unavailable_sources 0, recovery_items 0.
- Categories: interrupted_runs 0, changed_sources 2, pending_insights 7, source_unavailable 0, sources_due 1 (all `available`); blocking_decisions and routing_evaluations are `not_configured`.

**Skills**

| id | name | collection | lifecycle | routing_eligible |
|---|---|---|---|---|
| `code-review` | Code Review | engineering | active | true |
| `incident-postmortem` | Incident Postmortem | operations | active | true |
| `pr-description` | PR Description | engineering | draft | false |
| `release-notes` | Release Notes | docs | deprecated | false |
| `legacy-changelog` | Legacy Changelog | docs | archived | false |

- Draft `pr-description` readiness is `missing_fields: [trigger, min_scope]`.
- Entry path: `skills/engineering/code-review/SKILL.md`.

**Source**
- `id: acme-agent-skills`; locator `github.com/acme/agent-skills`, ref `main`, path `skills/`.
- `status: changed`; monitoring enabled, `weekly`; trust `community`, not reviewed; license `MIT`.
- current revision `3f9c2a1e7b…`, distilled `81d04be2c4…`.
- Second source: `contoso-prompts`, `watching`, `manual`, license unknown, never distilled.

**Run**
- `RUN-7K2M9QX4B1D8F3A6`, source `acme-agent-skills`, `awaiting_decision`, attempt 1.
- changed_resources: `skills/code-review/SKILL.md` (modified), `skills/code-review/references/checklist.md` (added).
- package digest `sha256:4b1e…9a0c`.
- outstanding_decisions: `{kind: ambiguity, resource: skills/code-review/SKILL.md, question: "Upstream removed the 'nitpicks' section. Treat as intentional removal or as a pending rewrite?"}`.
- Finalized variant coverage: 2 resources `analyzed`; 3 findings, 1 comparison, 2 insights.

**Insight**
- `INS-code-review--flag-unbounded-retries`, skill `code-review`, category `reliability`, priority `high`, status `pending`.
- recommendation "Flag unbounded retry loops in reviewed code".
- rank: score 82, impact high, 2 sources, 3 findings, not stale.
- observation_ids: `OBS-acme-agent-skills--retry-loop-guidance`, `OBS-acme-agent-skills--backoff-jitter`.
- One comparison: subject "Retry guidance across sources", verdict "Adopt with explicit limit", trade-offs "Adds one checklist item; low noise risk".

**Errors (ERROR / WHY / FIX)**
- `edit_conflict`: "SKILL.md changed since you opened it" / "Another edit was applied (digest differs)" / "Merge your draft onto the latest content and preview again".
- `index_stale`: "Search index needs rebuild" / "Catalog is stale" / "Run `skillhub rebuild`, then reload".

**Identifier display rule:** show digests and SHAs truncated as `sha256:4b1e…9a0c` or `3f9c2a1`. The full value is available via copy and tooltip.

---

## 7. Vietnamese locale (later)

Design for English now, but leave room for Vietnamese:

- Vietnamese strings run about 20–35% longer. Buttons, badges and table headers must tolerate that without truncating CTAs (wrap or grow, never clip).
- No text baked into images or icons.
- Fonts must cover Vietnamese diacritics. Inter and JetBrains Mono do.
- Status and lifecycle labels come from one key set (section 5). The Vietnamese terms in `04` become the `vi` values later.
- Dates and numbers follow the locale. Technical identifiers are never translated.

---

## 8. Claude Design batches

| Batch | Screens | Attach from `04` |
|---|---|---|
| 1 | App shell, Home, Skills catalog, theme light/dark | §0, §1, §2.1, §2.2, §4.2, §5, §7 |
| 2 | Proposal Preview, Diff, Destructive confirm, Command block, Conflict drawer | §4, §6 |
| 3 | Add skill, Create skill, Skill detail (Review / Editor / Resources) | §2.3–§2.5, §3.1–§3.4 |
| 4 | Sources, Watch source, Distill handoff, Run | §2.6–§2.7, §3.5 |
| 5 | Inbox, Insight detail, Patch composer | §2.8–§2.9, §3.6 |

Prompt template for each batch:

```text
Design the following screens for Skill Hub WebUI: <batch screens>.
Follow the attached design brief (visual direction, theme, app shell, wireframes, English copy, sample data) and the attached spec sections (behavior, fields, states).
Deliver desktop (1440), tablet (1024) and mobile (390) frames, each in light and dark mode.
For every screen include loading, empty, error/degraded and success variants listed in the spec's screen-state matrix.
Use only fields from the spec's data-contract matrix and values from the brief's sample data. Do not add features listed as out of scope.
```

Sign-off uses the checklist in `04` §9 plus: both color modes are present, and no CTA is clipped at 135% string length.
