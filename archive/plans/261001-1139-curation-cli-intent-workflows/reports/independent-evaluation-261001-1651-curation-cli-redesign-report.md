# Independent evaluation: Skill Hub curation CLI redesign

Date: 2026-10-01. Evaluator: independent review per `curation-ux-cli-independent-evaluation-prompt.md`. No project code was changed.

Evidence base:

- Repository at commit `157507de` (permalink base `P` = `https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6`). Every cited source file is unmodified in the working tree except `internal/delivery/cli/help.go`, which is not used for any finding.
- A binary built from the working tree (`go build ./cmd/skillhub`, version `dev`). It was run against disposable workspaces under the session scratchpad, with `HOME` redirected there. The user's real workspace was not touched.
- One bounded public repository, `anthropics/skills` at commit `8a1541c4a3ff`, scoped to `skills/pdf`. No imported script was executed.
- Every defect found, with reproduction commands, expected vs actual behavior, cause and fix, is tracked in [bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md](bug-ledger-261001-1702-curation-cli-defects-found-in-evaluation.md). A fix plan must cover BUG-01 to BUG-17.
- External conventions from official documentation and pinned source, collected in `researcher-261001-1644-cli-conventions-evidence.md`. The tables below cite those URLs directly.

Labels used throughout:

- **[C] Confirmed:** observed in a run, or read directly in code.
- **[I] Inference:** reasoned from evidence but not executed.
- **[U] Unresolved:** not verified.

---

## 1. Verdict

**Adopt the intent-first split (Design A), but not as written.** The grammar is right and the precedents support it: `skill add <locator>` to adopt one skill, and `source watch <locator>` to monitor upstream. Helm (`repo add` vs `install`), Homebrew (`tap` vs `install`), gh (`extension install` vs `upgrade`) and cargo (add pins, update is explicit) all separate "register a source" from "use one item".

The proposal assumes a sound engine under a new front door. The engine is not sound. Wrapping it today would ship confident one-command UX on top of silent data loss.

Six defects block any front-door redesign. Fix them first, whichever design is chosen:

| # | Defect [C] | Why it blocks |
|---|---|---|
| B1 | **Hidden monitoring.** `--no-monitor` is ignored unless `--cadence` is also given. The record says `monitoring.enabled: true, cadence: weekly`. | Breaks the "no hidden watch" P0 rule for the very flag that disables watching. |
| B2 | **Lost updates while the editor is open, and the same race over MCP.** A concurrent managed edit was overwritten and both commands exited 0. | Breaks the "no silent overwrite" P0 rule. |
| B3 | **Editor work is discarded on any preview failure.** The temporary file is deleted. | Destroys user content. |
| B4 | **Folder-scoped import silently drops companion files.** The canonical layout also forbids common upstream layouts. | `skill add <folder-url>` would yield broken skills. This is the exact primary journey. |
| B5 | **Two validators disagree.** `skillhub validate` rejects content that catalog publication accepts and serves. | "Structural validation before publish" is only partly true. |
| B6 | **A fresh clone fails validation.** Empty required directories are not tracked by Git. | The documented move-machine flow fails, and any CI check fails out of the box. |

**Recommended command model** (section 5 has the full specification):

```text
Start                 skillhub status
Adopt one skill       skillhub skill add <github-url|folder> [--skill <id>]... [--all] [--watch] [--yes]
Create                skillhub skill create <id> [--description ...] [--content-file f | --editor] [--yes]
Change                skillhub skill edit <id> [--editor | --content-file f | field flags] [--yes]
Inspect / review      skillhub skill show <id>          (enriched; no separate `review` verb)
Lifecycle             skillhub skill activate|deprecate|archive <id> [--yes]
Watch upstream        skillhub source watch <github-url> [--yes]
Check upstream        skillhub check [<source>...|--all-due]   (exists today; `source check` does not)
Before commit / CI    skillhub validate [--staged]; skillhub diff
```

Three corrections to the proposal follow from this:

- `skill review` should not become a new verb; enrich `skill show` instead.
- `source check` should not be invented; keep `check`.
- `skill add --watch` must be one atomic application workflow. Two chained transactions would leave the future web UI to orchestrate them.

---

## 2. Observed current journeys

All runs used a disposable workspace (`init <ws> --yes`, then commit).

### J1. New workspace, then status

```text
$ skillhub status
Workspace is new; commit it, then connect an agent.
No skills yet. Next: ask your agent 'create a skill for ...' or run `skillhub skill create ...`
```

This takes 1 command. The suggested next command is not runnable: `...` is a placeholder and `skill create` needs four flags.

### J2. Create, show, edit, activate, diff

| Step | Command | Result |
|---|---|---|
| 1 | `skill create reliability-review` | `WHY: create does not accept positional arguments` (exit 2) |
| 2 | `skill create --id … --collection … --name … --description … --trigger … --not-for … --min-scope … --yes` | `Draft skill reliability-review saved.` |
| 3 | `skill show reliability-review` | Shows routing fields and the scaffold body: "Describe when your agent should choose this skill", "1. First step." |
| 4 | `EDITOR=… skill edit reliability-review --editor` | Preview prints file counts and three pins. It also says "or re-run with --yes to apply directly", which would **reopen the editor on the original text** and lose the edits just made. |
| 5 | `skill activate placeholder-demo --yes` on an **untouched scaffold** | `Skill placeholder-demo is now active.` (exit 0) |
| 6 | `diff` | `active skills (4):` lists 4 **files** across 2 skills, then 8 operation-history files. |

Creating and activating a skill took 2 commands and 7 required flags. The scaffold placeholders became routable.

### J3. Adopt one skill from a GitHub folder URL

| Step | Command | Result |
|---|---|---|
| 1 | `source capture https://github.com/anthropics/skills/tree/main/skills/pdf --reason …` | Captured. |
| 2 | `source triage SRCQ-… --decision accept --source-id anthropics-pdf` | `Git HTTPS source operation failed: repository not found:`, followed by blank lines. |
| 3 | Re-capture the bare repository URL, then `source list` to find the new candidate ID | Manual decomposition. |
| 4 | `source triage SRCQ-… --decision accept --source-id anthropics-pdf --path skills/pdf` | Preview with three pins. `license unknown`, `cadence weekly`. |
| 5 | `source confirm --proposal … --proposal-digest … --base-version …` | `Watching anthropics-pdf.` A persistent weekly watch now exists. |
| 6 | `source import anthropics-pdf` | `1 importable`. |
| 7 | `source import anthropics-pdf --yes` | `Imported 1 draft skill (pdf).` |
| 8 | `find skills/default/pdf` | **Only `SKILL.md` and `skill.meta.yaml`.** Upstream has 12 files: `forms.md`, `reference.md`, `LICENSE.txt` and 8 `scripts/*.py`. The imported `SKILL.md` still says "see REFERENCE.md" and "read FORMS.md". |

This took 7 commands, 4 copied values (candidate ID twice, one pin line), 1 invented ID, and a manually split repository and path. The user ended up with an unrequested weekly watch and a broken skill. Upstream `SKILL.md` declares `license: Proprietary`, yet the source record says `license: unknown`.

### J4. Adopt one skill from a local folder

| Locator | Result |
|---|---|
| Absolute path `/…/local-skills/code-review` | `invalid source locator` |
| `./local-skills/code-review` | Captured, then triage fails with `invalid source locator`. Capture is looser than triage. |
| `"~/x"` (quoted) | Captured literally as a relative path. |
| `vendor-skills` inside the workspace | `path is outside the canonical workspace layout`. The whole workspace becomes invalid. |
| `runtime/fixture` (a gitignored folder) | Works after 6 commands. The **committed** source record points at a gitignored path that does not exist in any clone. |

No supported path adopts a skill from a local folder outside the workspace.

### J5. Concurrent editing (B2, B3)

1. Editor A (`skill edit --editor --yes`) opened, and its script waited on a signal.
2. Writer B ran `skill edit --content-file b.md --yes` and succeeded: `Draft skill … saved`.
3. A was released and also reported `saved` (exit 0). The final file contains A's change and **not** B's. B's operation receipt still exists in `history/operations/`.
4. Repeating the test with preview, then a direct file edit by B, then `skill confirm` with A's pins gave the same loss. The pins were fresh because preview ran after B.
5. An editor that produced an invalid frontmatter `name` got `WHY: SKILL.md frontmatter name must be "reliability-review"`, and the temporary file `/tmp/skillhub-edit-*.md` no longer existed.

### J6. Direct edits of an active skill

1. **Valid edit:** appended a section. `skill show` then served it immediately because the catalog rebuilt on read. The edit became active locally with no confirmation step.
2. **Edit invalid under `validate`:** set frontmatter `name: WRONG NAME`. `skill show` printed the invalid content with exit 0, and `status` said `Workspace valid; search index current`. `validate` exited 2: `frontmatter name "WRONG NAME" does not match skill ID`.
3. **Edit invalid under the catalog:** set `status: shiny` in one skill's metadata. `skill show` of **another** skill, `skill list` and `resolve` all failed. `status` said `No skills yet` although two active skills exist.

### J7. Partial staging and CI

1. Staged an invalid `skill.meta.yaml`, then restored the working tree without staging the fix. `validate` passed (exit 0) while the index held an invalid commit.
2. `git checkout-index -a --prefix=<tmp>/` followed by `validate <tmp>` failed for environmental reasons: `.git` missing, and 10 `Required canonical directory is missing` findings. Adding `git init` and recreating the empty directories exposed the real staged error.
3. A fresh `git clone` of a valid workspace failed `validate` and `rebuild` (exit 2) with the same missing-directory findings. The documented flow is `git clone` then `skillhub rebuild`. Only `doctor --fix --yes` repaired the clone, and it also rewrote three agent-connection files.

### J8. Agents (MCP), from code reading [I]

Adopting a skill from a URL takes five tool calls: `source_intake_add`, then `source_triage` accept, then `source_triage` with confirmation, then `source_import_preview`, then `source_import_confirm`. The tree-URL failure from J3 applies. `skill_update_preview` takes full `content` without any expected-base digest, so the J5 race applies to agents as well.

---

## 3. Confirmed facts, inferences, and unresolved uncertainty

Each row gives the claim, its evidence, and why it matters.

### 3.1 Confirmed by code and runs

| ID | Claim | Evidence | Why it matters |
|---|---|---|---|
| F1 | `--no-monitor` is overridden to enabled when no cadence is passed. | [`internal/app/source.go#L294-L297`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L294-L297); CLI default `monitoring: true` at [`internal/delivery/cli/source.go#L208`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/source.go#L208); MCP default at [`internal/delivery/mcpserver/source_tools.go#L84-L87`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/mcpserver/source_tools.go#L84-L87); run J3 | A safety opt-out silently fails, so any design that composes watching inherits hidden monitoring. |
| F2 | Accepting a source requires a caller-invented ID. | [`internal/app/source.go#L250-L253`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L250-L253) | This is the core abstraction leak the redesign must remove. |
| F3 | GitHub tree URLs are passed whole as the repository. | [`internal/app/source.go#L726-L745`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L726-L745); run J3 step 2 | The pasted-URL journey fails today with an unhelpful error. |
| F4 | Captured local paths must be relative, and the filesystem adapter is rooted at the workspace. | [`internal/app/source.go#L684-L700`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L684-L700), [`#L98-L104`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L98-L104); run J4 | "Point at my folder" is unsupported, and capture accepts locators that triage rejects. |
| F5 | Import ID comes from frontmatter `name`, then folder name, then `imported-skill`. Existing IDs are skipped, never overwritten. | [`internal/app/source_import.go#L167-L205`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L167-L205) | Reusable derivation and conflict rules. |
| F6 | Companion files are imported only when the skill folder is below the scope root, and only from `references/`, `scripts/` or `assets/`. Anything else is dropped without a warning. | [`internal/app/source_import.go#L207-L226`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L207-L226); run J3 step 8 | Scoping to one skill folder, which is what `skill add` does, drops everything except `SKILL.md`. |
| F7 | The canonical layout rejects any skill file other than `SKILL.md` or files under `references/`, `scripts/` or `assets/`. | [`internal/canonical/skill.go#L248`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/canonical/skill.go#L248); validate run | Real upstream skills such as `anthropics/skills/skills/pdf` use top-level `reference.md` and `forms.md`, so a faithful import is impossible today. |
| F8 | Imports always target collection `default` and draft state, and record provenance as `source_id`, `revision` and `path`. | [`internal/app/source_import.go#L260-L293`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L260-L293) | Provenance requires a source record, which couples adding a skill to watching a source. |
| F9 | The `--editor` flow reads the original, opens the editor, and builds the update afterwards. `UpdateInput` has no base digest. | [`internal/delivery/cli/skill.go#L374-L434`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L374-L434), [`#L120-L146`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L120-L146); [`internal/skill/lifecycle.go#L59-L67`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/skill/lifecycle.go#L59-L67), [`#L185-L196`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/skill/lifecycle.go#L185-L196); run J5 | Confirmed root cause of B2. |
| F10 | `PlanMutation` already rejects a caller-supplied `BeforeDigest` that differs from disk, returning `ErrConflict`. | [`internal/mutation/mutation.go#L150-L157`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/mutation/mutation.go#L150-L157) | The fix for B2 is plumbing an existing mechanism, not new infrastructure. |
| F11 | The editor temporary file is removed when `editContent` returns, before the preview can fail. | [`internal/delivery/cli/skill.go#L386-L391`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L386-L391); run J5 step 5 | Confirmed root cause of B3. |
| F12 | The preview hint "re-run with --yes" applies to `--editor` too. | [`internal/delivery/cli/skill.go#L458`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L458) | Re-running reopens the original text, so following the hint discards the reviewed edit. |
| F13 | The catalog rebuilds on read when inputs are stale and `canonical.Validate` passes. Otherwise every read fails. | [`internal/catalog/open.go#L34-L55`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/open.go#L34-L55); callers include [`internal/app/resolver.go#L59`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/resolver.go#L59); run J6 step 3 | Direct edits are structurally gated, but one bad file disables routing for every skill and project. |
| F14 | `skillhub validate` adds a frontmatter-name check that the catalog builder does not run. | [`internal/app/operations.go#L27-L49`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/operations.go#L27-L49) vs [`internal/catalog/build.go#L141-L148`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/build.go#L141-L148); run J6 step 2 | Root cause of B5: the publish gate is weaker than the documented check. |
| F15 | `status` prints "No skills yet" whenever counts are zero, including when the workspace is invalid. | [`internal/delivery/cli/curation.go#L43-L46`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/curation.go#L43-L46); run J6 step 3 | Misleading at the moment the user most needs accuracy. |
| F16 | Activation checks routing fields only, never content. | [`internal/app/skill_lifecycle.go#L257-L284`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L257-L284); run J2 step 5 | Placeholder scaffolds become routable. |
| F17 | `ActiveLocally` is hard-coded `true`, even for drafts. | [`internal/app/skill_lifecycle.go#L203-L208`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L203-L208) | JSON clients and the web UI cannot trust "active locally". |
| F18 | Next-step hints print the literal `git -C <ws> commit`. | [`internal/delivery/cli/skill.go#L485-L497`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L485-L497) | The hint is not runnable. |
| F19 | The documented machine move is `git clone` then `rebuild`. A fresh clone fails both commands. | `docs/user-guide.md` at HEAD, lines 119–127 ([`docs/user-guide.md#L119-L127`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/docs/user-guide.md#L119-L127)); run J7 step 3 | B6 also makes CI on a shared skills repository fail by default. |
| F20 | `validate` reads the working tree. | Run J7 step 1 | A hook built on today's `validate` is not commit validation. |
| F21 | `skillhub source check` and `skillhub skill review` do not exist. The upstream check is top-level `check`. | [`internal/delivery/cli/source.go#L21-L30`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/source.go#L21-L30); CLI runs | The proposal's command table contains a command that does not exist. |
| F22 | The curator skill forbids agents from editing canonical Hub files directly. | [`system-skills/curator/SKILL.md#L21`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/system-skills/curator/SKILL.md#L21) | For agents, direct editing is not a valid path; only preview/confirm is. |
| F23 | Source triage now warns about size limits. | [`internal/app/source.go#L327-L350`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L327-L350) | Supersedes part of the earlier audit's P0-2 claim. |

### 3.2 Inferences

| ID | Inference | Basis |
|---|---|---|
| I1 | MCP `skill_update_preview` has the same lost-update window between `skill_get` and the preview. | Its input carries no digest ([`internal/delivery/mcpserver/types.go#L115-L124`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/mcpserver/types.go#L115-L124)). It calls the same `PreviewUpdate`. Not executed over MCP. |
| I2 | Proposal IDs commit to roughly 80 bits of the digest, for example `PROP-e7994a1b8262f0a5911f` with digest `sha256:e7994a1b8262f0a5911f…`. | Observed in previews. This makes a human-facing short confirm (`skill confirm <proposal-id>`) safe, provided the stored digest is re-checked. |
| I3 | A `skill add` built directly on `PreviewSourceImport` would inherit F6, F7 and F8 unchanged. | Code path analysis. |

### 3.3 Unresolved

| ID | Uncertainty | How to close it |
|---|---|---|
| U1 | Live behavior with refs containing `/` (no fixture repository was available). | Add the acceptance test for slashed refs (section 10). |
| U2 | Windows path forms (`C:\…`, UNC paths). | Add platform acceptance tests. |
| U3 | Whether vercel `skills` uses the folder tree SHA for updates. The researcher read only lock-file comments. | This does not affect the recommendation, which records both the commit and the folder digest. |
| U4 | Whether real users want an add without a watch more often than a watch. | Run the usability test in section 9. |

---

## 4. Option comparison

### Design A: intent-first split, with corrections

Common path:

1. `skill add <url-or-folder>` (preview)
2. `--yes` or the printed confirm command
3. `skill edit <id> --trigger … --not-for … --min-scope … --yes`
4. `skill activate <id> --yes`

This is 2 commands to get a draft, 4 to get an active skill, and 0 copied IDs. Monitoring is opt-in with `--watch` or `source watch`.

| Property | Assessment |
|---|---|
| User decisions | 3: which skill, routing boundaries, when to activate. |
| Safety and provenance | Draft-only import, explicit watch, origin pinned to commit plus folder digest. |
| Automation and MCP | Adds `skill_add_preview` and `skill_add_confirm`. Primitives remain. |
| Web mapping | One "Add skill" action maps to one application workflow. |
| Compatibility cost | Low. All current commands remain. |
| Assumption it depends on most | The engine prerequisites B1–B6 are fixed first. |
| Fails first when | Shipped on today's import engine: the URL works but the skill is broken (F6, F7). |

### Design B: consolidated `source add <locator> [--import]`

Common path: `source add <url> --import` (preview), then `--yes`. That is 2 commands.

| Property | Assessment |
|---|---|
| User decisions | The same three, plus an implicit choice of whether the source persists. |
| Safety and provenance | A source record is always created, with monitoring enabled by default today (F1). |
| Automation and MCP | One tool, `source_add`, carrying an `import` flag. |
| Web mapping | "Add source" with an import checkbox. Skill-centric tasks start in the wrong place. |
| Compatibility cost | Low. |
| Assumption it depends on most | Users think "source first". Precedent says otherwise: brew documents fully qualified `brew install user/repo/formula` that trusts only one item without a separate tap step ([docs.brew.sh/Taps](https://docs.brew.sh/Taps)). |
| Fails first when | A user who wanted one skill gets a persistent network watch. That is a hidden watch, a **P0 failure by construction**, unless the default flips. |

### Design C: minimal change

Keep the primitives, infer repository, ref and path in `triage`, auto-derive the source ID, and print better hints.

Common path: capture, triage, confirm, import, import `--yes`. That is 5 commands, with 1 copied candidate ID and 1 copied pin line.

| Property | Assessment |
|---|---|
| Safety and provenance | Unchanged and strong once B1–B6 are fixed. |
| Automation and MCP | Unchanged. |
| Web mapping | The web layer must chain four transactions itself. **Rejected** by the portability rule. |
| Compatibility cost | None. |
| Assumption it depends on most | Better hints compensate for 5 steps. The earlier audit's P1-1, "pins leak into every preview", says they do not. |
| Fails first when | First-time users. The candidate/source/import vocabulary is still required. |

### Rubric (1–5)

| Criterion | A | B | C | Evidence |
|---|---|---|---|---|
| Simplicity | 4 | 4 | 2 | 2 vs 2 vs 5 commands to reach a draft (J3 vs design). |
| Learnability | 5 | 2 | 2 | The noun matches intent. Helm and brew separate the source verb from the item verb (researcher Q5). |
| Memorability | 5 | 3 | 2 | One verb per intent. clig.dev favors consistent noun-verb ([clig.dev#subcommands](https://clig.dev/#subcommands)). |
| Convenience | 5 | 4 | 3 | One positional locator, like `helm install <ref\|dir\|url>` ([helm docs](https://helm.sh/docs/helm/helm_install/)). |
| Safety | 4\* | 2\* | 4\* | B creates a watch by default. \*All three score 1 until B1–B3 are fixed. |
| Governance | 4 | 4 | 4 | Same mutation engine for all three. |
| Automation | 4 | 4 | 5 | C changes no contracts. |
| Web portability | 5 | 3 | 1 | Whether the application layer owns the workflow atomically. |
| Compatibility | 4 | 4 | 5 | Primitives are kept as advanced commands in all three. |

**Recommendation: Design A.** No P0 failure exists by construction. Design B carries one (hidden watch), and Design C pushes orchestration into the web layer.

---

## 5. Recommended CLI specification

### 5.0 Prerequisites, before any new command

Ordered by severity. Each is independent of the chosen grammar.

| P | Fix | Acceptance |
|---|---|---|
| P0 | Honor `--no-monitor` and `monitoring_enabled:false` regardless of cadence (F1). | Section 10, test T1 |
| P0 | Add an expected-content digest from read to preview for `--editor`, `--content-file` (optional) and MCP `skill_update_preview`. Return `stale_base` with no overwrite (F9, F10, I1). | Section 10, tests T13 and T14 |
| P0 | Never discard editor output. On any post-editor failure, keep the file and print its path along with how to resume (F11, F12). | Section 10, test T15 |
| P0 | Use one validator. Move the frontmatter-name rule (and any other `validate`-only rule) into `canonical.Validate`, so publication, `validate`, `status` and CI agree (F14). | Section 10, test T17 |
| P0 | A fresh clone validates. Either treat a missing *empty* required directory as empty, or create tracked placeholders that the layout accepts (F19). | Section 10, test T20 |
| P1 | When canonical state is invalid, reads keep serving the last published generation with a warning, and mutations stay blocked. See open decision D3 (F13, F15). | Section 10, test T18 |
| P1 | Reject activation while generated scaffold placeholder lines remain. This is deterministic because Skill Hub authored the text (F16). | Section 10, test T12 |

### 5.1 `skillhub skill add <locator>`

```text
skillhub skill add <locator> [--skill <id>]... [--all] [--ref <ref>] [--id <new-id>]
                             [--collection <c>] [--watch] [--yes] [--json] [--workspace <ws>]
```

**Default behavior:**

1. Resolve the locator (section 6).
2. Fetch one pinned snapshot. This is the only network effect, and it happens because the user ran this command.
3. Discover skills.
4. Build **one** write set:
   - every bounded regular file in the skill folder, using the layout from open decision D1;
   - `skill.meta.yaml` with `status: draft` and empty routing;
   - an `origin` provenance block.
5. Print the preview.

No source record is created, and nothing is watched.

**Preview output (human):**

```text
Add skill "pdf" as a draft
  From:     github.com/anthropics/skills · main @ 8a1541c · skills/pdf
  Files:    SKILL.md, reference.md, forms.md, LICENSE.txt, scripts/ (8 files, not run by Skill Hub)
  License:  Proprietary (declared in SKILL.md)          ← warning, not a block
  Effect:   new draft; active skills unchanged; not watched for updates
Apply:  skillhub skill add https://github.com/anthropics/skills/tree/main/skills/pdf --yes
   or:  skillhub skill confirm PROP-23f2343ef55ee3864e20
```

**After the add is applied:**

```text
Added draft "pdf". Next: set when to use it, then activate:
  skillhub skill edit pdf --trigger "<when to use>" --not-for "<when not to use>" --min-scope single_step --yes
```

- **`--watch`:** the same single proposal additionally writes the source record and link. The preview gets a separate line: `Watch: weekly upstream check of skills/pdf (network, until unwatched)`. Local locators refuse `--watch` (see 6.3).
- **Errors and exit codes:**
  - Exit 2 with a stable code for: `locator_unsupported`, `ref_not_found`, `ref_ambiguous`, `no_skill_found`, `multiple_skills` (lists IDs; fix: `--skill <id>` or `--all`), `id_conflict` (fix: `--id <new-id>`), `limit_exceeded` (names the limit and the actual value), `unsafe_file` (symlink or path escape).
  - `already_added` (same origin and same digest) exits **0** with no change. This follows gh's precedent for install-when-installed ([cli/cli command.go#L376-L386](https://github.com/cli/cli/blob/fc4b137cdef0a6bd28fd461b7cf9c84a5812a8cd/pkg/cmd/extension/command.go#L376-L386)).
- **Non-interactive:** never prompts. Ambiguity fails with the exact flag to pass ([clig.dev#interactivity](https://clig.dev/#interactivity)).

### 5.2 `skillhub source watch <locator>`

```text
skillhub source watch <github-url> [--ref <ref>] [--cadence daily|weekly|manual] [--id <source-id>] [--yes] [--json]
```

This replaces capture, triage-accept and confirm with one preview/confirm workflow.

- The source ID is derived as `owner-repo`, plus `-<last path segment>` when scoped. A collision with a different locator appends `-2`. Watching the same locator again exits 0 with "already watching".
- It never imports. The output names the follow-up: `skillhub skill add <url>`.
- `source capture` (save for later) and `triage --decision defer|reject` remain for the intake queue.

### 5.3 `skillhub skill create <id>`

- The ID becomes positional; `--id` stays accepted.
- `--collection` defaults to `default`, matching import ([F8](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L260)).
- `--name` defaults to the title-cased ID.
- `--description` is required. Without a TTY, a missing description fails with the flag to pass. With a TTY, the user is prompted.
- Body content comes from `--content-file`, from `--editor` (opened before the preview), or from the generated scaffold. The scaffold is blocked at activation (5.0).

### 5.4 `skillhub skill edit <id>`

The current surface is kept, with three changes:

- **Base check.** `--editor` and `--content-file` carry the digest of the text they started from. On mismatch: `stale_base — reliability-review changed while you were editing; your text is saved at <path>`, followed by `skillhub skill edit reliability-review --editor --from <path>` to reopen on current content with the user's draft beside it.
- **Preview content.** For `--editor`, the preview prints a unified diff of `SKILL.md` by default (it is the user's own change) and only the confirm-by-ID command. It no longer says "re-run with --yes".
- **MCP.** `skill_get` returns `content_digest`, and `skill_update_preview` accepts `expected_content_digest`. It is optional for backward compatibility, and the curator skill passes it. This mirrors HTTP `If-Match` (RFC 9110 §13.1.1), the same contract the web editor needs.

### 5.5 `skillhub skill show <id>` (the review surface)

The default output adds a short status block before the content:

```text
pdf — draft (not used by agents)
  Valid:      yes                          | no: 1 issue → skillhub validate
  Routing:    triggers 0 · not-for 0 · min-scope none → required before activation
  Overlaps:   none detected                | 1 possible overlap with code-review (warning)
  Origin:     github.com/anthropics/skills @ 8a1541c · skills/pdf · not watched
  Git:        uncommitted (new)
  Next:       skillhub skill edit pdf --trigger "…" --not-for "…" --min-scope … --yes
```

- `--verbose` adds the catalog snapshot, generation, operation IDs and the full digests.
- `--json` returns all fields with stable names.
- `--content=false` is an option for long skills.

**Why not a separate `skill review` verb.** "Review" implies an approval record that does not exist and that the non-goals forbid ("no second approval system"). It would also duplicate `show` ([clig.dev: avoid near-synonym subcommands](https://clig.dev/#subcommands)). A web "review panel" maps to `show --json`.

### 5.6 Lifecycle, `check`, `validate`, `diff`, and confirmation

- **Lifecycle:** unchanged grammar. The success hint prints a runnable `git -C '<actual path>' commit …` (F18). `ActiveLocally` reports the truth (F17).
- **`check`:** unchanged. Do not add a `source check` alias.
- **`validate --staged`:** validates `git checkout-index -a --prefix=<tmpdir>/` in a temporary directory ([git-checkout-index](https://git-scm.com/docs/git-checkout-index#_examples)). It is read-only, never touches the working tree or the index, and tolerates the absence of `.git` and of empty directories in the snapshot.
- **`diff`:** group by skill (`pdf: 2 files added`) and keep the file list under `--verbose`.
- **Short confirm:** `skillhub skill confirm <proposal-id>` reloads the stored proposal, re-verifies its digest, and relies on the existing base-snapshot conflict check (I2). MCP keeps the full three-pin contract.

### 5.7 JSON and MCP implications

New MCP tools:

- `skill_add_preview {locator, skills?, all?, ref?, id?, collection?, watch?}`, returning `{origin, skills[{id,files,license,conflict}], effects{draft:true, active_changed:false, watch:bool}, confirmation}`.
- `skill_add_confirm {pins}`.
- `source_watch_preview` and `source_watch_confirm`.

The five-call primitive chain stays for recovery and for bulk import from a watched source. Every result keeps `Result`, `Items` and `SuggestedActions`. Warnings go to stderr in human mode and never corrupt `--json` stdout ([clig.dev#the-basics](https://clig.dev/#the-basics)).

---

## 6. Locator-resolution contract

### 6.1 GitHub

| Input | Behavior |
|---|---|
| `https://github.com/o/r` or `…/r.git` | Use the default branch and discover all `SKILL.md` within limits. With 0 skills: `no_skill_found`. With 1: that skill. With more than 1: `multiple_skills` unless `--skill` or `--all` is given. |
| `…/tree/<rest>` | Resolve the ref (below). The remainder is the folder. |
| `…/blob/<rest>/SKILL.md` | Resolve the ref. The folder is the file's parent. |
| `…/blob/<rest>/<other file>` | `locator_unsupported`, with the fix "link the skill folder or its SKILL.md". |
| `#<ref>` fragment, or `--ref` | Explicit ref. It overrides URL parsing ([degit](https://github.com/Rich-Harris/degit/blob/master/docs/USAGE.md), [giget](https://github.com/unjs/giget/blob/f1dad453055b97a81d5227edce7262c8dcaf53d2/src/_utils.ts#L42-L56), [vercel skills](https://github.com/vercel-labs/skills/blob/3694740352eeef5cdd689af694c485f1ff62eec3/src/source-parser.ts#L284-L314)). |
| SSH, `git@…`, other hosts | `locator_unsupported` in v1. Generic HTTPS Git uses `--ref` and `--path`. |

**Ref resolution.** Refs may contain `/` ([git-check-ref-format](https://git-scm.com/docs/git-check-ref-format)).

1. Run one `git ls-remote --heads --tags <repo>`. The add needs network anyway, so this costs nothing extra.
2. Collect every ref name `R` such that `<rest>` equals `R`, or starts with `R/`, comparing whole segments.
3. With no match, accept a 7–40 hex commit SHA if the first segment is one. Otherwise return `ref_not_found`.
4. With one match, use it.
5. With more than one match (for example `feature` and `feature/x`), take the longest only if exactly one interpretation contains a `SKILL.md`. Otherwise return `ref_ambiguous`, listing the interpretations, with the fix `--ref`.
6. A branch and a tag with the same name produce `ref_ambiguous`, with the fix `--ref refs/heads/<n>` or `refs/tags/<n>`.

This goes beyond prior art: vercel skills, degit and giget take only the first segment (researcher Q2). The reason is that Skill Hub's preview claims to show *the* resolved origin, so silent first-segment misparsing would be a false preview. The preview always shows the resolved ref and commit.

**Pinning.** Record `commit` (full SHA) and `folder_digest` (the content digest of the imported files). This follows `Cargo.lock` and npm's `resolved` field ([cargo docs](https://doc.rust-lang.org/cargo/reference/specifying-dependencies.html#choice-of-commit), [npm package-lock](https://docs.npmjs.com/cli/v10/configuring-npm/package-lock-json#packages)), plus vercel's folder hash.

### 6.2 Local folders

| Input | Behavior |
|---|---|
| `./x`, `x` | Resolve against the **current directory**. Today the path is workspace-relative (F4); this is a contract change, and the MCP form must be absolute. |
| `~/x` | Expand in the CLI if the shell did not. MCP rejects `~`. |
| `/abs/x` | Accepted. |
| A folder or a `SKILL.md` path | A file path means its parent folder. |
| A path inside the workspace's own `skills/` | `locator_unsupported` with "already in this workspace". |

**Boundaries:**

- The root may be a symlink; it is resolved once and the resolved path is displayed.
- Any symlink, device or FIFO *inside* the folder fails with `unsafe_file` and is listed.
- `..` cannot escape the root after resolution.
- Reuse the source limits: 2,048 files, 8 MiB total, 2 MiB per file ([`internal/app/source.go#L294`](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L294)). The limit error names the limit and the actual value.
- Nothing is executed. Executables are listed as "not run".

### 6.3 Identity, provenance, and collisions

- **Skill ID:** sanitized frontmatter `name`, then folder name (existing F5 behavior). If sanitizing changes the value, the preview shows it.
- **Collisions:** a collision is never an overwrite.
  - Same origin and same digest: `already_added`, exit 0.
  - Same ID with a different origin or content: `id_conflict`, exit 2, with the fix `--id <new-id>`. `--id` rewrites the frontmatter `name` through the existing normalizer.
- **`origin` block** in `skill.meta.yaml`:

  ```yaml
  origin:
    kind: github | git | local
    repository: …          # git kinds only
    ref: …
    commit: …
    path: …
    name: <basename>       # local only
    folder_digest: …
    added_at: …
  ```

  - The block never contains an absolute local path; local provenance is basename plus digest.
  - Existing `provenance.source_id` stays for watched sources.
- **Local watching:** refused in v1, with `local_watch_unsupported: local folders are machine-specific; re-run skill add after updating`. The alternative, a gitignored machine-local watch, is open decision D2.

### 6.4 Multiple skills

`--all` previews every importable skill and lists conflicts as skipped. `--skill` is repeatable, and `#ref@skill` is accepted as shorthand, following vercel. The preview never imports silently beyond what is listed.

---

## 7. Edit, review, and governance contract

### 7.1 The three editing paths

| | Agent (MCP preview/confirm) | `skill edit --editor` | Direct file edit |
|---|---|---|---|
| When canonical files change | At `*_confirm` | At `--yes` or `confirm` | At save |
| Validation | Plan-time virtual validation plus final publish validation | Same as agent | Publish validation on next read (F13). After the P0 fix, the same single validator. |
| Preview and exact confirm | Yes, with three pins | Yes; short ID confirm proposed | No; `diff` and `validate` instead |
| Stale or concurrent writes | **Today:** pins cover preview to confirm only (I1). **Required:** `expected_content_digest` from `skill_get`. | **Today:** lost update (J5). **Required:** original digest passed to `PlanMutation` `BeforeDigest` (F10). | Last writer wins at the filesystem level. Git shows it. |
| Receipt and provenance | Operation record in `history/operations` | Same | None. Git is the record. |
| Becomes active locally | On confirm, if the skill is `active` | Same | On the next catalog read, if valid. **Document this.** |
| Best for | Agent-driven improvement | Personal, careful edits with stale protection | Bulk refactors, IDE workflows, teams with PR review |

**Concurrency mechanism.** Three options were considered:

- **Digest check** (chosen): the smallest correct fix. It uses existing mutation support and needs no new storage.
- **Three-way merge** (rejected for v1): Markdown merges produce conflict markers inside instructions that agents would follow. It also needs a base-content store. Revisit only if usage data shows frequent `stale_base` errors.
- **Lock while editing** (rejected): an editor can stay open for hours, and the lock would also block agents.

### 7.2 Review: what is deterministic and what is not

| Check | Kind | Surface |
|---|---|---|
| Schema, layout, frontmatter and ID match, lifecycle order, size | Deterministic error | One validator: publish, `validate`, CI |
| Routing fields present for `active` | Deterministic error at activation | `skill activate` |
| Untouched generated scaffold | Deterministic error at activation | `skill activate` |
| `SKILL.md` references a file that is not in the skill folder | Deterministic **warning** (links may be prose) | `skill add` preview, `show` |
| Routing overlap with other active skills | Heuristic warning | `show`, previews |
| Instruction quality, trust, licensing fit | Human judgment | `show` surfaces the facts; Git PR records the decision |

### 7.3 Hooks, CI, and Git

- **Hook installer:** do not ship `hooks install` in v1. Ship `validate --staged`, plus a documented one-line hook (`skillhub validate --staged`) that works under any hook manager.
  - Why: an installer would mutate `.git/hooks` or `core.hooksPath` and collide with pre-commit and husky setups, while the value lies entirely in staged validation.
  - Precedent: pre-commit validates only staged content, but does so by stashing worktree changes ([pre-commit.com](https://pre-commit.com/#pre-commit)). Exporting the index is cleaner and never mutates the working tree ([git-checkout-index](https://git-scm.com/docs/git-checkout-index#_examples)).
- **Partial-staging demonstration:** J7 step 1. Working-tree `validate` passed while the index was invalid.
- **Minimum CI for a shared skills repository:** on a fresh checkout, run `skillhub validate` (exit 0 required) and `skillhub eval` if suites exist. This requires the P0 fresh-clone fix (J7 step 3).
- **Team approval:** keep it in GitHub and GitLab branch protection and PR review. Skill Hub supplies `show --json` facts for PR comments and nothing more.

---

## 8. CLI-to-web mapping

Each web action calls exactly one application workflow, and the same workflow backs the CLI and MCP.

| Task | CLI | Web action | Application workflow (atomic) | MCP |
|---|---|---|---|---|
| See priorities | `status` | Curation home | `CurationHome` (exists) | `hub_status` |
| Adopt a skill | `skill add <loc> [--watch]` | Paste a URL or choose a folder, then preview, then Add | **`SkillAdd.Preview` / `Confirm`** (new; one write set including an optional watch) | `skill_add_preview` / `_confirm` |
| Create | `skill create <id>` | Create form, then preview | `SkillService.PreviewCreate` / `Confirm` | `skill_create_*` |
| Edit | `skill edit <id> --editor` | Editor with an If-Match digest, then diff, then Save | `PreviewSkillUpdate(expected_digest)` / `Confirm` | `skill_update_*` |
| Review | `skill show <id>` | Skill page status block | **`SkillOverview`** (new read model: validity, routing, overlap, origin, Git, next) | `skill_get` (extended) |
| Lifecycle | `skill activate\|deprecate\|archive` | Lifecycle button, then impact confirmation | `PreviewTransition` / `Confirm` | `skill_transition_*` |
| Watch | `source watch <url>` | Watch form | **`SourceWatch.Preview` / `Confirm`** (new; capture plus accept in one write set) | `source_watch_*` |
| Check upstream | `check` | "Check now" | `CheckSources` | `source_check` |
| Improvements | `inbox`, `insight …` | Inbox | existing | existing |
| Validate before commit | `validate [--staged]`, `diff` | "Changes" view | `ValidateWorkspace` (single validator), `Diff` | `workspace_validate`, `workspace_diff` |

Rejected for the web: chaining `source_intake_add`, then `source_triage`, then `source_import_*` client-side. No application owner would hold atomicity across those calls (Design C).

---

## 9. Compatibility and migration

| Current | Status after the redesign |
|---|---|
| `source capture\|list\|show\|triage\|confirm\|import` | **Kept** as advanced primitives. They move to an "Advanced: intake queue and bulk import" section in docs and help. `source import <source-id>` remains the bulk path from watched sources. |
| `source triage --decision accept` | In release N+1, prints one **stderr** hint: "`skillhub source watch <url>` does this in one step." Never removed while MCP primitives exist. |
| `skill create --id …` | Kept as an alias of the positional form. |
| `check` | Unchanged. |
| `skill confirm --proposal … --proposal-digest … --base-version …` | Kept. The short `skill confirm <proposal-id>` is added. |
| Relative `source capture` locators | Unchanged, since they are workspace-relative. Only the new `skill add` uses current-directory-relative paths, so no script changes meaning. |

**Deprecation policy:**

- Follow the pattern: the old form keeps working, is hidden from beginner help, and emits one stderr warning naming the replacement ([cobra `Deprecated`](https://pkg.go.dev/github.com/spf13/cobra#Command); [kubectl rule 5a/6](https://kubernetes.io/docs/reference/deprecation-policy/#deprecating-a-flag-or-cli)).
- The minimum window is 2 releases.
- Skill Hub does not use cobra, so implement the pattern by hand and keep warnings off stdout.

**Measuring whether the redesign is better** (no task content collected):

- **Telemetry:** extend the existing curation events with:
  - `skill_add_previewed{locator_kind: github|local, discovered: n, outcome_code}`;
  - `skill_add_confirmed{watch: bool}`;
  - journey counters (commands per completed add, error codes, time from first preview to draft).
  - Never record locator strings, skill text or paths.
- **Usability test:** 6–8 participants, each with no prior Skill Hub exposure, given 4 tasks: add from a GitHub folder URL, add from a local folder, edit with the editor, activate.
  - Measure task success, commands issued, documentation lookups and wrong-command errors.
  - Run the current CLI and the new CLI as a between-subjects comparison.
  - Success means at least 90% of participants complete both adds without documentation, in no more than 3 commands.

---

## 10. Acceptance tests

Each test is written as Given / When / Then.

**Locator and add**

- **T1** — Triage `--no-monitor` without `--cadence` produces `monitoring.enabled: false`. The same holds for MCP `monitoring_enabled:false`.
- **T2** — `skill add https://github.com/<fixture>/tree/main/skills/x` previews exactly one draft with zero flags. Active skills are unchanged. There is no `sources/catalog` record and no watch.
- **T3** — A fixture ref `feature/a` with path `skills/x` and URL `…/tree/feature/a/skills/x` resolves `ref=feature/a` and `path=skills/x`. With refs `feature` and `feature/a` both present and both interpretations containing `SKILL.md`, the result is `ref_ambiguous` and nothing is written.
- **T4** — A blob URL to `SKILL.md` yields the same result as the folder URL. A blob URL to another file yields `locator_unsupported`.
- **T5** — An upstream folder holding `SKILL.md`, `reference.md`, `scripts/a.py` and `LICENSE.txt` is imported with all files (or per open decision D1). No file is dropped without being listed. No script executes; verified by a fixture script that would create a sentinel file.
- **T6** — A folder holding a symlink fails with `unsafe_file`, nothing is written, and the symlink path is listed.
- **T7** — Exceeding the file-count or byte limits fails with `limit_exceeded` naming the limit and the actual value.
- **T8** — `skill add ./x`, `~/x` and `/abs/x` from any current directory resolve to the same folder. The canonical files contain no absolute path, verified by grepping the workspace for the fixture's absolute path.
- **T9** — A repository with three skills returns `multiple_skills`, listing all three, and exits 2. `--skill a --skill b` imports exactly two. `--all` imports all three.
- **T10** — Re-adding the same origin and digest exits 0 with no write. Adding a different origin under the same ID returns `id_conflict`. `--id y` succeeds, and the frontmatter `name` is `y`.
- **T11** — `skill add <url> --watch` writes the skill, the source record and the link in **one** operation receipt. With an injected failure after planning, none of them is written.

**Create and activate**

- **T12** — `skill create demo --description d --yes`, then `activate` with full routing, fails with `scaffold_placeholder` until the body is edited.

**Editing and concurrency**

- **T13** — Editor A opens. B confirms an edit, either managed or by writing the file directly. A saves. Result: `stale_base`, B's content is intact, A's text is saved at the printed path, and the exit code is non-zero.
- **T14** — MCP `skill_get` then `skill_update_preview{expected_content_digest}` after a concurrent change returns `stale_base`. Omitting the digest keeps today's behavior, which the test documents.
- **T15** — An editor output with an invalid frontmatter `name` is rejected, and the edited text survives at the printed path.
- **T16** — `skill edit --editor` without `--yes` prints a `SKILL.md` diff and a confirm-by-ID command, and does not print "re-run with --yes".

**Validation, reads, and status**

- **T17** — For every rule `skillhub validate` enforces, a direct edit violating it causes publication to fail. No read serves the invalid content.
- **T18** — With one invalid file present, `skill show <other>` and `resolve` succeed from the last published generation and print a warning. `status` names the invalid file and does not say "No skills yet".
- **T19** — Staging an invalid file and restoring a valid working tree makes `validate --staged` exit 2 while `validate` exits 0. The working tree and the index are byte-identical before and after.
- **T20** — `git clone` of a valid workspace, then `skillhub validate`, exits 0 with no other command run first.

**Output**

- **T21** — No default human output from `skill add`, `skill show`, `source watch` or lifecycle commands contains `SRCQ-`, `sha256:`, `OP-`, `generation`, `adapter` or `<ws>`. These appear only with `--verbose` or `--json`.
- **T22** — `diff` default output counts skills, for example `pdf: 2 files added`, rather than labeling files as "active skills (4)".

---

## 11. Rejected ideas

| Idea | Why rejected | Evidence | When it would fail |
|---|---|---|---|
| `source add --import` (Design B) as the primary path | A source noun for a skill intent, and a persistent watch by default | F1; brew trusts one item without a tap ([docs.brew.sh/Taps](https://docs.brew.sh/Taps)) | The first user who wanted one skill gets weekly network checks. |
| Implement `skill add` as a thin wrapper over capture, triage, confirm and import | Inherits F6 and F7 (broken skills), F8 (always creates a source) and F1 | J3 step 8 | On the first real upstream skill with top-level references. |
| A separate `skill review` verb | Duplicates `show`, implies an approval record | Non-goals; clig.dev subcommand guidance | Users assume "reviewed" means something persisted, and it does not. |
| A new `source check` | `check` already exists and is documented | F21 | Two names for one action. |
| Three-way merge on editor conflict | Merge markers inside agent instructions, plus a base-content store | §7.1 | The first conflicting edit yields a routable skill containing `<<<<<<<`. |
| `hooks install` in v1 | Mutates Git configuration and collides with hook managers. The value is `--staged`, not installation. | pre-commit and husky ecosystems | Users already on pre-commit get a clobbered hook. |
| Hooks that run plain `validate` and are described as commit validation | They validate the working tree | J7 step 1 | Partial staging commits invalid content while the hook passes. |
| First-segment-only parsing of tree refs | A silent wrong origin in the preview | git-check-ref-format; T3 | A repository using `release/1.x` branches. |
| Persisting absolute local paths to watch local folders | Machine-specific, leaks the home directory into Git, breaks on clone | F4; J4 (`runtime/fixture` record) | On the second machine or in CI. |
| Removing pins from MCP to simplify | Agents need exact binding; humans get the short form instead | I2 | A stale agent proposal applies after a concurrent human change. |
| Auto-activating imports or inferring routing | Semantic decisions the non-goals forbid | Non-goals | A misrouted, unreviewed upstream skill. |

---

## 12. Open decisions for the product owner

1. **D1: canonical layout for upstream companion files.** Real skills place `reference.md` and `forms.md` beside `SKILL.md` (anthropics/skills `skills/pdf`), and today's layout forbids that (F7). Options:
   - (a) Allow any bounded regular file in the skill folder (recommended; keeps upstream links working).
   - (b) Relocate the files into `references/` and rewrite links (mutates upstream text).
   - (c) Refuse such imports, listing the files.

   **Recommended: (a).** The trade-off is a looser schema against faithful imports.
2. **D2: watching local folders.** Options:
   - Refuse (recommended for v1).
   - Allow a machine-local watch stored under gitignored `runtime/`, with a portability warning.

   The question is whether the convenience is worth state that silently differs across machines.
3. **D3: read behavior when the workspace is invalid.** Options:
   - Keep today's fail-closed behavior: every read errors, and agents get no skills.
   - Serve the last published generation with a warning, while mutations stay blocked (recommended).

   The trade-off is availability against agents possibly using content that is about to change.
4. **D4: licensing.** Should an import whose declared license is proprietary or unknown be warned about (recommended) or blocked until `--accept-license` is passed? This is a compliance choice.
5. **D5: updating an added skill from its origin.** Should Skill Hub offer `skill update <id>` (re-fetch the pinned origin, preview a three-way diff against local edits), or keep upstream learning only through `source watch` plus insights? This is out of scope for this redesign, but the `origin` block is designed so either works.

### Gap round

The analysis was revisited for contradictions, single-source claims and untested assumptions.

**Contradictions resolved:**

- The proposal says "Direct editing does not bypass structural validation." That holds only for `canonical.Validate` rules. The frontmatter-name rule is bypassed (F14, J6 step 2).
- The earlier audit's P1-9 (skills disappear after a hand edit) is now fixed for *valid* edits by auto-rebuild, but persists as a hub-wide outage for *invalid* ones (F13).
- The audit's claim that triage gives no size warning is superseded (F23).

**Single-source claims:**

- The vercel folder-hash update logic is backed by one source (U3). The recommendation does not depend on it.
- The MCP race (I1) is inferred from code, not executed. T14 makes it observable.

**Untested edge cases** (each mapped to a test):

- refs containing `/` (U1, T3);
- Windows paths (U2);
- the `--watch` atomicity failure path (T11).

**Recommendations that rest on assumptions:**

- That users mostly want add without watch (U4). Telemetry and the usability test in §9 decide this.

Every P0 and P1 recommendation above has evidence (§3), a failure mode (§1 and §11) and an observable acceptance check (§10).
