# Independent evaluation: Skill Hub curation CLI

Date: 2026-10-01. Evaluation target: local working tree based on `157507de254a5005c32120dd318a4abf1c229da6`, including pre-existing uncommitted changes. This is a design recommendation, not an implementation or a release certification.

All mutations and staging experiments ran under `/tmp/skillhub-cli-eval-FJVolj`. During evaluation, no project file was edited, no user workspace was curated, and no commit or push was performed. The completed report and supporting evidence were subsequently moved into `plans/reports/` at the user's request. Imported content was read, never executed. The controlled editor is an evaluator-owned script, not an imported script.

Raw command evidence: [main transcript](curation-ux-cli-independent-evaluation-evidence/transcript.json), [gap-round transcript](curation-ux-cli-independent-evaluation-evidence/gap-transcript.json), [experiment harness](curation-ux-cli-independent-evaluation-evidence/run-evidence.cjs), [gap harness](curation-ux-cli-independent-evaluation-evidence/gap-round.cjs), [controlled editor](curation-ux-cli-independent-evaluation-evidence/editor.cjs). The archived harnesses retain their original temporary workspace paths as reproduction context. Initial `init`, `status`, `create`, and `show` commands are reproduced below; they preceded the transcript harness.

Repository citations below are immutable links to the baseline commit. Core cited curation implementation files were clean at inspection. `docs/user-guide.md`, `internal/delivery/cli/help.go`, and workspace setup code had pre-existing edits: claims about their current behavior use local file anchors and experiments, not an assertion that the baseline permalink contains those edits. Neither the two input reports nor their conclusions are treated as product authority.

## 1. Verdict

Adopt the intent-first split, with a smaller first contract than the leading proposal:

```text
skillhub status
skillhub skill add <locator>
skillhub skill create <id> ...
skillhub skill list
skillhub skill show <id>
skillhub skill review <id>
skillhub skill edit <id> --editor|--content-file <file>|<field changes>
skillhub skill activate|deprecate|archive <id>
skillhub source watch <locator>
skillhub source list|show <selector>
skillhub source check <selector>|--all-due|--all
skillhub validate
skillhub diff
```

Keep existing learning/inbox/insight operations and advanced source primitives. `source check` is a new spelling of the existing `check` application operation. `skill add`, `source watch`, and `skill review` do not currently exist.

`skill add` means **bring a copy into the collection as a draft**. It must not imply immediate agent availability. Say that in help, preview, and receipt. `source watch` means **register upstream for explicit future checks and learning**, including an initial analysis opportunity; it must not imply a background daemon or automatic content updates.

Do not offer `skill add --watch` in the initial contract. Two explicit tasks cost one additional command only for users wanting both, and avoid an unnecessary combined transaction. A later combined action is acceptable only when the application layer owns one persisted preview, write set, receipt, and recovery boundary. A CLI or web wrapper calling today's source-confirm and import-confirm sequentially is insufficient. Existing onboarding and import are separate transactions [E2, E3]. This matters because a failed import must not leave an undisclosed watcher behind.

A separate read-only `skill review` is justified if it summarizes validity, distribution readiness, origin, Git differences, and actionability—even when the current catalog cannot rebuild. It is not justified as an approval database or an alias that merely dumps `show`, `validate`, and `diff` output. Current `show` requires a fresh catalog and `diff` only groups changed files [E4, E9]. This matters because the broken skill most needing review may currently make `show` fail.

The redesign is worth doing, but its release gates are behavioral: preserve draft imports, exact confirmation, bounded source reads, provenance, local ownership, and explicit network intent. Better nouns alone cannot fix the confirmed monitoring and editor races [E2, E5].

## 2. Observed current journeys

### Measurement convention

Count one CLI invocation as one step. Workspace setup is excluded from task comparisons; inspection and activation are counted separately from importing. Copy/paste counts mean copying values returned by the previous command, not typing the original locator. A regenerated `--yes` proposal is a fresh proposal, not confirmation of an earlier preview.

For reproduction, use the actual temporary binary and workspace:

```sh
BIN=/tmp/skillhub-cli-eval-FJVolj/skillhub
WS=/tmp/skillhub-cli-eval-FJVolj/hub
```

### New workspace: one initialization, one status read

```sh
go build -o /tmp/skillhub-cli-eval-FJVolj/skillhub ./cmd/skillhub
"$BIN" init "$WS" --yes
"$BIN" status --workspace "$WS"
```

Observed status, exit 0:

```text
Workspace is new; commit it, then connect an agent.
No skills yet. Next: ask your agent 'create a skill for ...' or run `skillhub skill create ...`
Workspace valid; search index current; Git has uncommitted changes.
Recommended next: Commit the new workspace, then run `skillhub connect` in your project.
```

The old audit's empty-hub dead end is not current behavior. However, `create ...` is still an incomplete runnable example. Evidence: [current status renderer](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/curation.go#L40). This matters because improvements should build on the existing status home rather than replace it.

### Create, inspect, edit preview, activate, diff: five task invocations

```sh
"$BIN" skill create --id reliability-review --collection software \
  --name 'Reliability Review' --description 'Review reliability risks.' \
  --trigger 'review reliability' --not-for 'design service' \
  --min-scope multi_step --yes --workspace "$WS"
"$BIN" skill show reliability-review --workspace "$WS"
VISUAL='node /tmp/skillhub-cli-eval-FJVolj/editor.cjs' \
  "$BIN" skill edit reliability-review --editor --full-diff --workspace "$WS"
"$BIN" skill activate reliability-review --yes --workspace "$WS"
"$BIN" diff --workspace "$WS"
```

`create` saved a draft and gave edit/activate hints. `show` displayed state, entrypoint path, triggers, negative boundaries, minimum scope, and content. The controlled editor appended one instruction to a temporary file; without confirmation, canonical bytes remained unchanged. Its output provided a complete pin-bearing confirmation command. Activation then succeeded on the **unchanged generated scaffold**, still containing `First step` and `Second step`. This is a concrete usability/quality failure, not proof that arbitrary semantic quality can be mechanically validated [E4, E5, E8].

Activation receipt, exit 0:

```text
Skill reliability-review is now active.
Next: ask your agent to use it, or inspect with `skillhub skill show reliability-review`. Commit with `git -C <ws> commit`.
```

`diff` reported `active skills (2)` for the two files of **one skill**. Draft grouping is now present, so the old claim that every draft is always grouped as active is obsolete; the entity/file count ambiguity remains [E9]. This matters because maintainers need to know how many skills are affected, not infer it from file counts.

### Source capture → onboarding → draft import: five invocations

A real two-file local fixture lived under the disposable workspace's ignored runtime directory. Putting an intake fixture in an arbitrary workspace folder first failed canonical-layout validation; moving it under runtime corrected the fixture setup. This is an experiment setup issue, not a claim that ordinary external folders work today.

```sh
"$BIN" source capture runtime/incoming/fixture \
  --reason 'Controlled bounded fixture' --json --workspace "$WS"
"$BIN" source triage SRCQ-47F1282621BD --decision accept \
  --source-id fixture --adapter filesystem --json --workspace "$WS"
"$BIN" source confirm --proposal PROP-6d71d8c0bb43fe99947a \
  --proposal-digest sha256:6d71d8c0bb43fe99947ad73988ed70608bf6a138ae971f59cd41e60c151880f9 \
  --base-version sha256:e790a63487f4d254c8de7e658baf15ac811e99b4cdd157d83201ef54944f3365 \
  --workspace "$WS"
"$BIN" source import fixture --workspace "$WS"
"$BIN" source import fixture --yes --workspace "$WS"
```

Exact selected outputs, all exit 0:

```text
Watching fixture. First analysis is ready: ask your agent 'distill new sources' or run `skillhub distill prepare fixture`.
Watching does not auto-import skills; accepted insights can create draft skills.

Found 1 skill(s) (1 importable, 0 skipped).
- fixture-review -> fixture-review [draft] (importable)

Imported 1 draft skill (fixture-review). Next: review with `skillhub skill show fixture-review`, then `skillhub skill activate fixture-review --yes` (or ask your agent).
```

This path needs one candidate-ID transfer, one manually selected source ID, and one confirmation-command transfer (or three individual pin fields). Import `--yes` generated a new preview. Confirming the stored import proposal instead would preserve the same five-step count, with one additional confirmation-command transfer. Inspecting and activating add two steps; the imported skill also needs routing configuration before activation [E1–E3]. This matters because the proposed front door should remove derived inputs, not the review decisions.

Repeating import left the existing skill untouched:

```text
Found 1 skill(s) (0 importable, 1 skipped).
- skipped fixture-review: Skill "fixture-review" already exists in workspace
```

Import preserved provenance and the reference file, and created empty draft routing. The fixture path exercises capture, triage, snapshot, confirmation, discovery, import, and conflict handling without network dependence [E3]. GitHub URL decomposition was verified by source inspection, not by pretending this fixture proves GitHub branch-resolution behavior.

### Direct edits and two different meanings of invalid

Appending `Direct draft edit.` and `Direct active edit.` to the corresponding entrypoints, followed by `skill show`, returned those bytes without a manual rebuild. Draft stayed draft; active stayed active. These direct edits created no managed mutation receipt [E4, E6].

Setting active metadata `routing.min_scope: huge` made `show` exit 2; `validate` reported its exact file and line. The published pointer remained byte-for-byte unchanged. This confirms canonical structural validation before publication [E6].

Replacing active `SKILL.md` with the following content had a **different** result:

```markdown
---
name: [invalid
---
Bad edit
```

`show` succeeded and displayed it; the pointer changed. `validate` exited 2 and reported a frontmatter-name mismatch. Catalog tests and distribution/resolver tests confirm that an unservable active skill can be published with warnings and is excluded from distribution/routing, allowing other skills to remain usable [E7]. This matters because “invalid edits cannot publish” is true for canonical structural failures, not for every stricter CLI/distribution diagnostic.

### Concurrent editor: a confirmed lost update

```sh
EVAL_RACE=1 VISUAL='node /tmp/skillhub-cli-eval-FJVolj/editor.cjs' \
  "$BIN" skill edit reliability-review --editor --full-diff --json --workspace "$WS"
```

Inside editor A, writer B ran:

```sh
"$BIN" skill edit reliability-review --description 'Writer B description' \
  --yes --workspace "$WS"
```

B succeeded before A returned. A's preview also succeeded. Its `SKILL.md` diff contained:

```diff
-description: Writer B description
+description: Review reliability risks.
```

Confirming A's exact proposal succeeded. The entrypoint reverted B's description while metadata retained `Writer B description`. No confirmation-pin violation occurred: the pins were created after B's write. Source: editor timing and manager update construction [E5]. This matters because the simpler CLI must protect the whole editing interval, not merely preview-to-confirm.

### Partial staging: both false acceptance and false rejection

With a complete valid index baseline, stage a wrong frontmatter name, then restore only the working file. `validate --workspace "$WS"` passed. Exporting the index into `staged-complete`, adding only required empty layout directories and an empty temporary Git repository, then validating it failed **only** on the staged wrong name. Inverse experiment: stage valid bytes, make an unstaged `min_scope: huge` edit; working-tree validation failed while the exported index passed. No hook was installed or claimed to exist [E10]. This matters because a working-tree hook cannot honestly promise commit validation.

### Additional observed contract failures

```sh
"$BIN" source triage SRCQ-9494AAD4B6B4 --decision accept \
  --source-id no-monitor-fixture --no-monitor --json --workspace "$WS"
```

Returned `"monitoring":{"enabled":true,"cadence":"weekly"}`. A fallback re-enables monitoring when cadence is omitted [E2]. This is a P0 release blocker for an add-without-watch workflow that reuses onboarding unchanged.

Creating `json-draft` using `--yes --json` returned `"state":"draft"` and `"active_locally":true`. The application hard-codes that boolean for every managed skill mutation [E4]. This matters because a future UI must distinguish published local state from actual routing eligibility.

An absolute filesystem locator was rejected. Capturing a GitHub tree URL stored the entire URL without parsing repository/ref/path. `skill add`, `source watch`, and `skill review` each exited 2 as unsupported subcommands [E1, E2].

## 3. Confirmed facts, inferences, and unresolved uncertainty

### Confirmed evidence ledger

Each evidence entry states the claim, its source anchor, and why it affects the decision. Section findings refer to these entries; experiment outputs are linked at the top.

| Evidence | Confirmed claim and repository source | Why the evidence matters |
|---|---|---|
| E1 | Source CLI exposes capture/list/show/triage/confirm/import, not add/watch; triage rejects `--yes`. [source.go:21](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/source.go#L21). | A new command must be a real workflow, not a renamed documentation example. |
| E2 | Onboarding requires source ID, uses unparsed repository URLs, roots filesystem sources in the hub, and can reverse `--no-monitor`. [source.go:91](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L91), [250](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L250), [294](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L294), [684](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L684), [736](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source.go#L736). | Locator inference and explicit watch state must be fixed below the CLI. |
| E3 | Import derives IDs, skips conflicts, copies allowed companion directories, writes drafts and source links, and confirms its own transaction. [source_import.go:151](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L151), [194](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L194), [260](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L260), [408](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/source_import.go#L408). | Reuse proven import invariants, but do not present two existing transactions as atomic. |
| E4 | Managed mutation uses persisted previews and exact confirmation; ReadSkill auto-rebuilds; receipt `ActiveLocally` is always true. [skill_lifecycle.go:94](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L94), [181](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L181), [207](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L207), [286](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L286). | Keep immutable approval semantics and correct the public state vocabulary. |
| E5 | CLI reads canonical content before launching editor, creates update preview after exit; manager reads current metadata/content when constructing that preview. [skill.go:120](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L120), [374](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L374), [lifecycle.go:185](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/skill/lifecycle.go#L185). | Proposal freshness cannot prevent overwriting work changed before proposal creation. |
| E6 | Freshness reads and final generation publication call canonical validation and recheck captured inputs. [open.go:31](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/open.go#L31), [build.go:130](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/build.go#L130), [input.go:64](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/input.go#L64). | Direct file ownership does not bypass the canonical publication boundary. |
| E7 | Canonical resources enforce size/UTF-8/layout; CLI validation adds name checks; distribution readiness uses a stronger YAML parser, and build warnings isolate unservable active skills. [canonical/skill.go:242](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/canonical/skill.go#L242), [operations.go:19](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/operations.go#L19), [servable.go:23](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/servable.go#L23), [76](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/catalog/servable.go#L76), [resolver_continuity_test.go:97](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/resolver_continuity_test.go#L97). | Review must distinguish structural failure, unavailable distribution, and semantic judgment. |
| E8 | Creation generates placeholder instructions; activation checks routing and marks reviewed. [lifecycle.go:139](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/skill/lifecycle.go#L139), [skill_lifecycle.go:253](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/skill_lifecycle.go#L253), [lifecycle.go:274](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/skill/lifecycle.go#L274). | Routing completeness and a boolean do not establish useful instructions. |
| E9 | Show includes routing; diff groups file counts; preview confirmation output omits workspace. [skill.go:46](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L46), [455](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/skill.go#L455), [operations.go:289](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/operations.go#L289), [curation.go:89](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/cli/curation.go#L89). | Preserve improved show output while making task counts and next actions accurate. |
| E10 | Workspace validation reads working files; Git index export is a separate operation; pre-commit can be bypassed. [operations.go:19](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/app/operations.go#L19), [Git checkout-index](https://git-scm.com/docs/git-checkout-index), [Git hooks](https://git-scm.com/docs/githooks). | Commit validation must use staged bytes and CI remains the shared enforcement surface. |
| E11 | Create/lifecycle/list/get MCP tools exist; intake/onboarding/import remain separate tool families. [skill_tools.go:15](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/mcpserver/skill_tools.go#L15), [source_tools.go:62](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/mcpserver/source_tools.go#L62), [source_import_tools.go:14](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/delivery/mcpserver/source_import_tools.go#L14), [curator:55](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/system-skills/curator/SKILL.md#L55). | The old missing-MCP-capability finding is obsolete, but new workflows still need shared services. |
| E12 | Sources enforce 20s/8MiB/2048 files/2MiB per-file defaults, safe HTTPS, public DNS, no redirects for Git, bounded mirrors, and filesystem containment. [types.go:12](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/source/types.go#L12), [policy.go:26](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/source/policy.go#L26), [git_repository.go:450](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/source/git_repository.go#L450), [filesystem.go:274](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/source/filesystem.go#L274). | A locator front door must preserve these bounds and distinguish transfer from selected-tree limits. |

### Inferences, explicitly not empirical usability findings

The skill/source split should reduce concept switching because the primary object matches the intended result. This follows from the observed five-command source journey [E1–E3], but no first-time-user study was run. `review` should reduce three or more diagnostic calls to one shared report [E4, E7, E9]; that is a design hypothesis, not a measured retention gain.

### Unresolved uncertainty

Live GitHub branch/tag ambiguity, default-branch races, authenticated/private repositories, enterprise hosts, Windows junctions, browser upload limits, and full hook installation were not exercised. Their proposed contracts and future tests are below. The local filesystem fixture is equivalent for import transaction behavior, not remote transport behavior. Existing source-security tests support transport policy, not new URL parsing.

### Gap round and verification

The second round revisited four assumptions rather than repeating the first report:

- “All invalid edits cannot publish”: disproved in the broad form; confirmed for invalid routing metadata, with frontmatter/distribution isolation documented [E6, E7].
- “No-monitor works”: disproved when cadence is omitted; reproduced with preview JSON [E2].
- “An export failing validation proves an invalid staged skill”: first export lacked workspace scaffolding; repeated with a complete staged baseline, empty directories, and isolated Git metadata. Only the staged name error remained, and the inverse snapshot passed [E10].
- “Active-local receipt means routing eligible”: draft receipt showed otherwise; traced to the hard-coded value [E4].

Focused existing tests passed for CLI lifecycle/editor/exact-confirmation, source import/conflicts/limits/capture idempotency, MCP skill lifecycle, catalog publication races and servability warnings, filesystem containment/symlink races, and DNS policy. `TestResolveSkipsSkillWithUnservableFiles` passed separately. Commands are listed in section 10. One targeted invocation included `internal/canonical` but matched no tests there; it is not counted as canonical test coverage.

## 4. Option comparison

Only three coherent designs are compared. All share the same safety repairs, preview default, draft lifecycle, bounded resolver, and Git ownership. Scores assume those requirements are implemented; shipping today's no-monitor fallback or editor race would fail the release gate regardless of score.

| Property | A: skill add / source watch | B: consolidated source add | C: inferred current primitives |
|---|---|---|---|
| Import common path | `skill add L` → exact confirm | `source add L --import` → exact confirm | capture → inferred triage → confirm → import preview → confirm |
| Watch common path | `source watch L` → exact confirm | `source add L --watch` → exact confirm | capture → triage → confirm |
| Post-locator decisions, one skill | Approve draft; optionally resolve conflict | Choose import/watch mode, approve | Invent/accept source identity, approve source policy, approve import |
| CLI steps to draft | 2; 1 with fresh `--yes` | 2; 1 with fresh `--yes` | 5; 4 using fresh import `--yes` without separate preview |
| Returned-value copying | 1 complete confirm command; 0 IDs manually composed | Same as A | Candidate transfer + two confirm commands; source handle reused |
| Safety/provenance | Draft plus origin in one transaction; watch separate | Safe only with explicit mode and one application transaction | Preserves existing primitives, but registration remains prerequisite |
| Automation/MCP | New import and watch preview/confirm workflows | One service with mode branching and composed write sets | Agents orchestrate more calls and retain intermediate handles |
| Web mapping | Add skill; Watch source | Add source wizard with mode selection | Web needs equivalent orchestration or a new shared service |
| Compatibility cost | Additive commands/services; old scripts retained | Additive command, more combined semantics | Lowest surface cost; partial benefit only |
| Main assumption | Most copy-one-skill users need no watcher | Users think of source onboarding before copying | Users accept source governance as part of importing |
| Fails first when | Add is understood as immediate activation; prevent with explicit draft output | Defaults hide whether import, watch, or both occur | Folder/local inference still leaves a five-step task and watcher coupling |

Counts are modeled for proposed commands, not measured execution of nonexistent commands. Current five-step count is observed in section 2 [E1–E3]. This matters because apparent convenience gains must not be presented as completed implementation evidence.

| Rubric, 1–5 | A | B | C | Evidence and rationale |
|---|---:|---:|---:|---|
| Simplicity | 5 | 4 | 2 | A removes source intake from import; B adds a mode decision; C retains it [E1–E3]. |
| Learnability | 5 | 3 | 2 | Result noun predicts task; B's source noun covers two effects; C retains triage terminology [E1, E11]. |
| Memorability | 4 | 3 | 2 | A has two explicit verbs, B requires mode recall, C requires sequence recall; hypothesis from observed grammar [E1]. |
| Convenience | 5 | 5 | 3 | A/B own one-locator resolution; C only removes decomposition, not orchestration [E2, E3]. |
| Safety | 5 | 4 | 4 | A separates effects; B has combined-effect risk; C still needs no-monitor repair [E2, E5]. |
| Governance | 4 | 4 | 4 | All retain the same validation/receipts/Git model; none proves semantic quality [E4, E6–E10]. |
| Automation | 5 | 4 | 3 | A has focused workflows; B branches by mode; C exposes more intermediate state [E11]. |
| Web portability | 5 | 4 | 2 | A maps one task to one service; B maps to a mode form; C needs orchestrator parity [E2, E3, E11]. |
| Compatibility | 4 | 4 | 5 | A/B are additive but need shared workflow/schema work; C touches fewer surfaces [E1, E11]. |
| Total, descriptive only | 42/45 | 35/45 | 27/45 | Totals cannot compensate for hidden activation/watch/overwrite or weaker validation. |

### Official CLI comparisons

`gh repo clone` accepts a repository URL and infers identity; `gh` offers readable output plus explicit JSON selection. Apply inference and progressive disclosure, without copying its default mutation semantics or claiming it parses Skill Hub folder URLs. Sources: [clone](https://cli.github.com/manual/gh_repo_clone), [formatting](https://cli.github.com/manual/gh_help_formatting). This supports hiding derived identity while preserving a richer machine response.

Cargo's `add` works on the thing being added; Git and filesystem sources use explicit source options, and `--dry-run` suppresses writes. It also updates existing entries, which Skill Hub should not copy. Source: [cargo add](https://doc.rust-lang.org/cargo/commands/cargo-add.html). This supports the task verb, not a universal claim that every mature CLI uses one positional locator or safe overwrite defaults.

`gh extension install` and `upgrade --dry-run` separate acquisition from later updates; local installation uses executable symlinks. Skill Hub should adopt the explicit acquisition/update distinction and reject that executable-link behavior for curated canonical copies. Sources: [install](https://cli.github.com/manual/gh_extension_install), [upgrade](https://cli.github.com/manual/gh_extension_upgrade). This supports separate upstream intent without importing execution assumptions.

VS Code accepts an extension ID or VSIX path in one install argument and has a separate update operation; installation may itself update an extension. Source: [VS Code CLI](https://code.visualstudio.com/docs/configure/command-line). This supports locator convenience, but not silent replacement of a locally curated skill.

Git explicitly permits slashes in references, exports index bytes separately from working files, and permits bypassing pre-commit. Sources: [ref format](https://git-scm.com/docs/git-check-ref-format), [index export](https://git-scm.com/docs/git-checkout-index), [hooks](https://git-scm.com/docs/githooks). These are direct technical constraints, not merely stylistic preferences.

## 5. Recommended CLI specification

### Grammar and task defaults

```text
skill add LOCATOR [--skill SELECTOR ... | --all] [--id ID]
                 [--collection NAME] [--ref REF --path PATH]
                 [--yes] [--json] [--verbose]
skill create ID [--name NAME] [--description TEXT] [--content-file FILE]
                [--editor] [routing options] [--yes]
skill edit ID [--editor | --content-file FILE] [field changes] [--yes]
skill review ID [--verbose] [--json]
source watch LOCATOR [--ref REF --path PATH] [--cadence daily|weekly|manual]
                    [--yes] [--json] [--verbose]
source check SELECTOR... | --all-due | --all
skill add --proposal ID --proposal-digest DIGEST --base-version VERSION
source watch --proposal ID --proposal-digest DIGEST --base-version VERSION
```

These are proposed signatures. Existing `create --id` remains valid. Positional ID supplies name by a simple display fallback and collection defaults to `core`; descriptions/content come from explicit inputs or a TTY editor/form. Non-interactive missing inputs return all required fields at once. Do not infer prose from the ID or add an unrequested LLM call. The application service must own defaults equally for CLI/MCP/web, instead of relying on adapter-specific defaults [E11].

Preview is the default everywhere these operations mutate curated state. It may fetch a requested remote and write disposable caches/proposals; say “No collection files changed,” not literally “No files changed.” Outside a TTY, return the preview and a shell-quoted exact confirm command containing the workspace and existing pins. In a TTY, a single approval can confirm that same persisted proposal. `--yes` explicitly authorizes the newly generated validated proposal; it cannot select among ambiguous refs/skills, overwrite conflicts, activate drafts, or expand scope. `--json` is non-interactive.

The two proposed confirmation forms take no locator or mutation options: they load the stored add/watch proposal and apply exactly its pinned write set. Reject combinations of pins with a locator, selection flags, or `--yes`. Existing primitive confirmation commands remain unchanged. Do not force humans to compose pin fields. An opaque token is unnecessary initially: a complete copyable command already exists in current preview output [E9]. This matters because simplifying display should not weaken the approval contract.

### Human output

Example proposed add preview:

```text
Add code-review as a draft from owner/repo, skills/code-review at <short revision>.
3 files, 18 KiB. Origin retained. Watching: off. Agent use: off.
No collection files changed.
Next: <complete exact confirmation command>
```

After confirmation: `Draft code-review added. Agent use: off. Watching: off. Changes are not committed. Next: skillhub skill review code-review ...`. Short revision is useful provenance, not a mandatory user-supplied ID. Detailed file inventory, policy limits, full digests, generation, proposal and operation IDs belong in verbose/JSON output. Skill IDs remain visible because users select skills by them; internal source IDs only appear when explicitly needed to disambiguate an advanced action.

Preview and receipt must report actual watch policy and routing eligibility, including when importing from an already watched source. Default add cannot turn an existing watcher off; it should say “Existing source watch retained.” “Off by default” means add never creates or changes watch policy implicitly [E2, E4].

Error codes should distinguish `invalid_locator`, `ambiguous_ref`, `selection_required`, `no_skills`, `id_conflict`, `limit_exceeded`, `source_changed`, `edit_conflict`, `stale_proposal`, and `validation_failed`. Return non-zero for incomplete/failed requested work, including all-failed network batches. A conflict on the single selected skill applies nothing and offers `--id` or an explicit edit workflow; it must not silently report success. Existing advanced import skip behavior remains compatible [E3].

### JSON/MCP contract

Add transport-neutral `PreviewSkillAdd/ConfirmSkillAdd`, `PreviewSourceWatch/ConfirmSourceWatch`, and `ReviewSkill` services. Names here are architectural proposals, not available tools. Publish matching preview/confirm MCP workflows while retaining current tools. JSON includes resolved origin, pinned bytes/revision, selection and conflicts, watch effect, lifecycle result, files/bytes, deterministic diagnostics, semantic warnings, next actions, confirmation, and receipt [E4, E11].

Separate `published_locally`, `lifecycle_state`, `servable`, and `routing_eligible`. Preserve legacy `active_locally` during migration with documented meaning; do not silently change a public field's meaning. New fields are authoritative for the redesigned UI. Receipt truth is a P1 contract repair, not a reason to activate drafts [E4, E7].

## 6. Locator-resolution contract

This section specifies future behavior. Current sourceLocator does not implement GitHub decomposition or general external-folder intake [E2].

### GitHub cases

| Input | Required resolution |
|---|---|
| `https://github.com/O/R` or `.git` | Repository, default branch resolved once, root discovery at pinned commit. |
| `.../tree/<ref>/<folder>` | Resolve actual remote ref boundary; select that folder only. |
| `.../blob/<ref>/<folder>/SKILL.md` | Resolve commit, verify regular entrypoint blob, select its parent folder. |
| `.../blob/<ref>/SKILL.md` | Root skill plus bounded permitted companions; never exclude companions merely because the entrypoint is at root. |
| `.../blob/<ref>/something-else.md` | Reject with a folder/SKILL.md example; do not interpret an arbitrary document as a whole skill. |
| `.../tree/<full-commit-sha>/<folder>` | Resolve and verify commit object; pin it exactly. Watching an immutable commit cannot discover branch movement. |
| Raw-content URL or non-GitHub host | Not inferred by the first contract; give an advanced source route, not HTML scraping or an implicit redirect. |

Parse the URL structurally; normalize trailing slash and `.git`, validate host exactly, reject userinfo/query/fragment and unsupported ports, decode path safely once, reject traversal/NUL/backslash/encoded separators that change segment boundaries. Owner/repository are the first two path components; this rule does **not** determine the ref boundary.

After `tree`/`blob`, enumerate bounded advertised branch and tag names and consider candidate ref/path splits. Accept a unique candidate that resolves to the requested object type and scope. Multiple viable interpretations require `--ref` and `--path`; branch/tag names resolving differently also require disambiguation. No “always take the first segment,” “always longest prefix,” or default-branch fallback. Full commit SHA is an explicit candidate, not an arbitrary revision expression. Git permits slashes in ref names [official ref format](https://git-scm.com/docs/git-check-ref-format); this matters because naive parsing can import the wrong directory while appearing successful.

Pin resolved commit and content inventory during preview. If the branch moves before confirmation, confirm the previously reviewed cached bytes, clearly showing the pinned revision; never refetch and substitute bytes under the same approval. If those bytes are unavailable or changed, fail `source_changed` and regenerate. A watcher keeps a mutable branch selector separately from the immutable imported revision [E3, E12].

The current Git adapter uses `NoTags` clone/fetch [E12]. Tag URLs therefore need an explicit retrieval/resolution capability before they can be promised. Scoping `--path` does not shrink the current full repository transfer or mirror budget. Use bounded commit/tree/blob acquisition or bounded Git transfer; if unavailable within policy, fail with a named transfer-limit error rather than promising that narrowing a directory always fixes it [E12].

### Local folders and uploaded folders

Relative paths resolve from caller CWD, not the hub. Accept `.`, `./x`, ordinary relative paths and normalized `../x` if they identify an explicitly supplied external folder. `~/x` expands only the current user's home prefix when the shell did not expand it; reject `~other/x`. Accept absolute directories. Source-relative resource traversal remains forbidden even though the user may intentionally choose a folder above CWD. Canonical paths remain workspace-relative [E2, E12].

Use a no-follow directory handle, validate ancestor components, and reject symlink/junction/reparse roots or descendants in the selected copy scope. Never follow Git symlink blobs or submodules. Refuse devices, sockets, FIFOs, unexpected file types, and paths escaping the selected root. Detect replacement races using opened-file identity and bounded inventories. Omit `.git` and machine/runtime metadata by an explicit documented inventory rule; report omitted files, do not run them. Existing filesystem containment and symlink-race tests are evidence for the invariant, not proof that arbitrary external-root support already exists [E12].

Create an immutable content snapshot before preview. If the local folder changes later, use reviewed cached bytes or fail; do not recopy current files at confirm. A web upload goes through the same validated snapshot model, not a server interpretation of the browser's filesystem path.

Initial ceilings: 20 seconds for acquisition, 2048 discovered resources, 8 MiB selected source bytes, 2 MiB per source file, plus independent remote transfer/cache/ref-enumeration budgets. Retain the source adapter's current defaults unless measured data justifies changing them. Canonical files permit 16 MiB each, while distribution permits at most 512 resources and 16 MiB per skill; these are distinct boundaries [E7, E12]. Error output names the boundary and actual/allowed value. No unlimited fallback, recursive full-disk scanning, automatic submodule fetch, or script execution.

### IDs, discovery, and durable origin

Use sanitized frontmatter name, then folder basename, as the proposed skill ID. Display the chosen ID and original name. Unlike current fallback `imported-skill`, an unusable name/basename should require explicit `--id` to avoid accidental collisions. IDs are unique across collections; check reserved `system-curator` as well. Duplicate inferred IDs in one batch are ambiguous selection/conflict errors, not a filesystem-order winner [E3, E7].

Zero skills: non-zero `no_skills`, no source/watch/skill canonical mutation. One skill: infer selection, preview it. Many: return a bounded list of relative folder selectors and require `--skill` or `--all`, even with `--yes`. Folder selectors distinguish duplicate frontmatter names. For an explicit batch, preflight all selections and fail the new operation without partial canonical writes when any selected skill conflicts; keep the advanced existing skip-import route for users who explicitly need it [E3].

Remote provenance records repository, selected path, requested ref, resolved commit, snapshot digest, imported-file digests, observed license, time, and normalization/omission notes. Derive the internal origin key from canonical repository plus scope; derive a separate watch identity from repository/scope/ref. Reuse only an exact matching record; check full identity on truncated-digest collisions and extend the key or fail safely. Never ask the user to invent source IDs [E2, E3].

Create a durable **origin source record** even when not monitored, with disabled monitoring and manual cadence. It must be excluded from due checks, learning queue, and watched-source counts. Existing `Record` validates adapter locators, so this is not just an output change [source records](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/internal/source/records.go#L94). This matters because local origin snapshots cannot be represented by pretending a personal absolute path is an existing portable filesystem locator.

For local copies, retain display basename, content/inventory digests, import time, transformations and optional explicitly supplied origin label. A local snapshot origin needs a versioned locator representation distinct from the existing live filesystem adapter; it cannot be encoded as a nonexistent relative directory. Store the validated snapshot under disposable cache for preview; canonical provenance may retain safe hashes but must not require that cache for ordinary skill use. Do not persist the original absolute path in origin metadata, receipts, persisted proposals, telemetry, or canonical Git files. A path-bearing watch binding is local runtime configuration, explicitly requested by `source watch`, with a portability warning and rebind-required status on another machine. The portable source record records identity/policy without exporting the binding. “Local folder” is a valid origin even when no external publisher is known [E2, E3, E12].

## 7. Edit/review/governance contract

### Three editing paths, evaluated independently

| Property | Agent preview/confirm | CLI `--editor` | Direct canonical edit |
|---|---|---|---|
| Canonical change time | After exact confirm | After exact confirm or authorized fresh `--yes`; editor writes temp only | As soon as external editor saves |
| Validation | Managed proposal/virtual tree, canonical publication, distribution eligibility separately | Same managed path; input temporary-file checks | Canonical structure on catalog rebuild; distribution readiness can warn/isolate |
| Preview/approval | Persisted diff and exact pins | Same, but original-read protection currently absent | No Hub preview/approval; Git diff/review available |
| Concurrent edits | Preview-to-confirm stale base rejected; pre-preview agent reads need expected base too | Current open-to-preview race reproduced; must add expected original digest | User/editor/Git owns concurrency; Hub detects unstable captured input where possible |
| Receipt/provenance | Managed operation receipt; original source lineage retained | Same | No managed receipt; Git history and retained origin do not prove that current bytes were reviewed |
| Active effect | Active edit becomes locally usable after successful publication if servable | Same | Valid active edit may become locally usable on next auto-rebuild; no commit required |
| Best fit | Agents and auditable managed updates | Human authored edits with exact preview | Power users and branch/PR workflows |

Sources: managed services [E4], editor/manager [E5], catalog validation [E6], distribution isolation [E7]. This matters because “direct edit lacks managed receipts” must not become “direct edit lacks structural validation.”

### Close the editor race with a precondition

Return canonical bytes plus original entrypoint digest/path from the edit-read application service. `PreviewUpdate` accepts `expected_content_digest` and checks it against the content used to construct the proposal under the appropriate lock/snapshot boundary. Carry metadata base digest too when the client submits a whole routing/metadata object derived from an earlier read. Do not hold an exclusive lock while a human editor is open.

On mismatch, create no overwrite proposal; retain the edited temporary content as a recoverable evaluator/user artifact or provide an explicit save path, and report: `Skill changed while your editor was open. Reopen current content or compare your edits.` If content changes again during proposal planning, normal proposal/base checks still apply. A check in the CLI followed by an unguarded service call would leave another race [E5, E6].

An original-content digest is the smallest adequate first fix. Automatic three-way merge adds conflict handling, frontmatter/metadata reconciliation, and semantic merge questions not required for the common task. A later manual three-way comparison can help recovery, but must never silently merge and confirm active instructions. Apply expected-read-base inputs to agent and web editors too [E4, E5].

### Review output and deterministic limits

Default `review` reports: lifecycle; structurally valid/invalid/unknown; agent availability (servable and eligible); current origin and local modifications; watched/not watched; staged and unstaged change counts; missing activation fields; actionable routing warnings; one exact next action. Include a concise actual change excerpt or a link/command for Git content diff. `show` remains the full instruction reader.

Verbose adds resource inventory/digests, receipt history, comparison base, evaluation-suite details, and all diagnostics. JSON retains stable structured facts, not prose to be reparsed. Review must inspect canonical state and collect issues even if catalog freshness fails, without fetching upstream or mutating canonical records. It may read/cache derived state but never claims semantic approval [E4, E7, E9].

Deterministic checks: schemas, UTF-8, conflict markers, references, metadata ID, routing completeness, lifecycle rules, frontmatter/distribution requirements, missing local resources, duplicate exact triggers and configured relationship contradictions. Reuse a proper shared frontmatter parser for diagnostics; current CLI name checking is a line scanner, not full YAML validation [E7].

Semantic warnings: plausible overlap beyond exact fields, clarity of instructions, malicious-looking source prose, usefulness, intended scope, and quality of examples. Report checks actually performed; unknown origin trust or absent routing suites stays unknown. An evaluation case can prove performance on that case, not universal semantic correctness [E8, E11].

Do not reverse the verified policy that an unservable active skill can be isolated while unrelated skills remain usable. Review/status should expose `active but unavailable to agents`, and activation should require distribution readiness. Structural failures still block canonical publication. Strict CI validation can fail an active unservable skill even when rebuild intentionally emits a warning; share diagnostics and explain their differing operational purposes [E6, E7].

### Scaffolds and review history

Stop generating example steps that look like usable instructions. Empty/minimal authoring drafts are acceptable, but unchanged generated scaffolds must not activate merely because routing fields are present. Identify the tool's own untouched template or a known incomplete authoring state deterministically; do not ban the phrase `First step` from arbitrary genuine skills. Explain that content review is still human/agent judgment. Current activation marks `quality.reviewed: true`, which should be described as a requested activation acknowledgment, not a permanent review certificate for future direct edits [E8].

### Hooks and CI

An optional hook installer is useful only after staged-snapshot validation exists. Keep it outside the first beginner path. Preview installation, preserve existing hooks and `core.hooksPath`, require explicit opt-in, and support reversible uninstall. The hook must not stage, rewrite, rebuild the user's live catalog, fetch, commit, or repair content [E10].

A genuine commit check exports **the index selected for that commit**, including an alternate `GIT_INDEX_FILE` when Git supplies one. Handle unmerged entries, symlinks and submodules safely before materialization, freeze the index snapshot for the check, validate in an isolated temporary workspace, and clean up. Unset inherited Git worktree/index variables for the isolated validator so it cannot accidentally inspect the original repository. Create only required empty directories and isolated Git metadata; never copy unstaged canonical files to make a staged snapshot pass. Linked worktrees and partial commits need dedicated tests. A simple working-tree check may remain available but must be labeled as such [E10].

Minimum shared CI: check out the exact PR/merge tree with a pinned Skill Hub version, materialize required empty layout directories without modifying canonical file bytes, and run strict offline workspace validation including active distribution readiness. Do not fetch sources or mutate/repair checked-in skills. Configure an optional deterministic routing suite if the team maintains one. Git hosting owns required checks, review approvals, and branch protection; no new Skill Hub approval database is needed [E6, E7, E10].

## 8. CLI-to-web mapping

Each row represents one user action backed by an application workflow. Preview and confirmation are two states of one action, not separate web orchestration recipes. Git commit/PR rows are external user workflow links, not permission for Skill Hub to perform them.

| Recommended task | Future web action | Shared application workflow / boundary |
|---|---|---|
| `status` | Curation home | Existing GetCurationHome, factual next actions |
| `skill add L` | Paste URL / upload folder → review draft addition | New PreviewSkillAdd/ConfirmSkillAdd; snapshot + origin + files in one mutation |
| `skill create ID` | New skill form/editor | Existing create preview/confirm with shared defaults |
| `skill list` | Collection list with state filters | Existing ListSkills |
| `skill show ID` | Instruction detail | Existing ReadSkill plus routing |
| `skill review ID` | Review panel | New ReviewSkill diagnostic composition, tolerant of broken catalog |
| `skill edit ID` | Edit and review changes | Existing update preview/confirm plus expected-read-base precondition |
| `skill activate ID` | Activate button with impact preview | Existing transition service plus readiness/template diagnostics |
| `skill deprecate ID` | Deprecate action | Existing ordered transition preview/confirm |
| `skill archive ID` | Archive action | Existing ordered transition preview/confirm |
| `source watch L` | Watch-source form | New watch preview/confirm, no canonical candidate prerequisite |
| `source list/show` | Sources and source detail | Existing source read service; human selectors resolve internally |
| `source check` | Check selected/due sources | Existing CheckSources; explicit network action and per-item failures |
| Save source for later (`capture`) | Save to intake | Existing capture, offline; advanced but retained distinct intent |
| Distill source changes | Analyze source / run progress | Existing run package/start/submit/finalize workflow; host does semantic analysis |
| `inbox` / insight show | Improvement inbox/detail | Existing inbox/evidence reads |
| Insight decide | Plan/reject/obsolete/reopen action | Existing decision service; reason where required |
| Insight apply/confirm | Review proposed improvement → apply | Existing application proposal and exact confirmation; never raw web file writes |
| Outcome record | Record result | Existing outcome service, explicit observations |
| Retry/cancel run | Run recovery controls | Existing run recovery service with explicit action |
| `validate` | Check collection | Shared validation diagnostics with strict active readiness |
| `diff` | Workspace change panel | Existing Git summary enhanced with staged/unstaged facts; Git owns content diff |
| Advanced rebuild/doctor | Maintenance panel | Existing service, explicit recovery; no implicit source fetch |
| Optional staged hook | Repository settings / local setup instructions | New staged validation/installation service, optional; server cannot install a browser user's local Git hook |
| Git commit / PR / merge | Open repository/review instructions | External Git/hosting workflow; no automatic commit, push, or merge |

Current CLI/MCP already share services [E4, E11]. This matters because new add/watch/review should extend that boundary instead of introducing a second orchestration model in a web backend. A hosted web UI cannot directly read a user's local folder: upload or an explicitly authorized local bridge supplies the same snapshot contract. Watching a client-local folder requires that local binding/bridge and cannot be faked by storing a browser path on the server [E2, E12].

## 9. Compatibility and migration plan

Preserve existing `source capture/list/show/triage/confirm/import`, `skill create --id`, all skill lifecycle/confirmation commands, top-level `check`, run/insight primitives, JSON fields and existing MCP tools. Advanced primitive documentation remains reachable. Add `source check` as an alias to the same check service; do not give it different defaults. Add positional create ID without removing `--id` [E1, E11].

Beginner docs lead with add/create/edit/review/watch/status. Hide **conceptual machinery from the beginner journey**, not commands from scripts. Do not alias `source import <source-id>` directly to `skill add <locator>`: argument meanings and watch/provenance preconditions differ. Do not alias `source add` ambiguously to both import and watch [E1–E3].

First land application-level monitoring preconditions, edit concurrency, state facts, and diagnostics. Then add locator resolution and atomic origin/draft import. Then add watch/review adapters and docs. Hook installation follows staged-snapshot capability. Each stage preserves current behavior unless its explicit contract is intentionally corrected. New source-origin and local-binding representations need versioned schema compatibility and migration tests; the current filesystem locator validator only accepts portable relative paths [E2, E12].

Do not remove existing JSON fields to simplify human output. Add precise state fields, publish their semantics, and retain legacy state fields until a declared version transition. Deprecation warnings must stay out of JSON payloads and human-output parsing scripts should migrate to JSON. No command removal is needed for this redesign [E4, E11].

Controlled usability study: counterbalance A/B/C task cards across first-time users; test GitHub-folder import, external local-folder import, create/edit/activate, and shared-repo review with partial staging. Record completion, invocations, copied handles, help opens, errors/retries, and unaided command recall after a delay. Require zero hidden activation/watch/overwrite incidents. Pre-register the comparison and improvement threshold; do not invent a measured win.

Telemetry may record allowlisted command family, locator kind, mode, success/error code, duration, help/retry count and confirmation count with explicit consent/normal product telemetry policy. Do not record task text, locators, absolute paths, skill content, diffs, credentials, or free-form errors. Use controlled local transcripts for richer study observations; do not send evaluator transcripts through curation telemetry. The curator already prohibits unobserved/backfilled session measurements [curator:185](https://github.com/vantt/mcp-skill-hub/blob/157507de254a5005c32120dd318a4abf1c229da6/system-skills/curator/SKILL.md#L185). This matters because step reduction should be demonstrated without collecting sensitive work.

## 10. Acceptance tests

### Release-gating recommendations: evidence, failure, observable check

| Priority / recommendation | Evidence | Failure mode | Acceptance check |
|---|---|---|---|
| P0: add imports drafts plus origin atomically | E2, E3, E11 | Watch/source record persists after failed import, or draft becomes active | Inject failures before/after each write/publication; recovery yields one full operation or none; no routing recommendation for drafts |
| P0: no hidden monitoring | E2 + no-monitor experiment | Default add or explicit disabled monitoring schedules checks | Preview/confirm/replay preserve disabled state; all-due performs no source access; existing watch is explicitly disclosed |
| P0: no overwrite or ambiguous selection | E3 + repeated import | Existing content replaced or batch chooses first duplicate ID | Existing bytes/digests unchanged; multiple selections require explicit choice, including with `--yes`; no partial new-operation writes |
| P0: preserve canonical and source policy | E6, E12 | Relaxed resolver follows symlink/private address or publishes structural failure | Unsafe inputs fail before canonical writes; invalid metadata leaves pointer unchanged; approved snapshot bytes survive upstream movement |
| P1: full edit-read concurrency protection | E5 + race experiment | Fresh preview reverts writer B's change | A opens, B changes content, A exits: edit_conflict and no overwrite proposal; edited buffer recoverable; B remains intact |
| P1: coherent review/state facts | E4, E7, E9 | Draft shown usable; active-unservable hidden; invalid workspace cannot be reviewed | Draft eligibility false; unavailable reason visible; canonical diagnostics accessible without successful rebuild |
| P1: genuine locator inference | E2, E12 + Git ref rules | Wrong ref/path or private path leaked | Repository/tree/blob/root cases, slash refs and ambiguity fixture tests; canonical artifacts and persisted preview contain no personal absolute path |
| P1: scaffold activation boundary | E8 + activation experiment | Generated placeholder becomes an allegedly reviewed active skill | Untouched generated template cannot activate; genuine supplied content plus valid routing can; arbitrary legitimate phrases are not banned |
| P1: exact useful output | E9 | Replayed command hits wrong workspace; file count looks like skill count | Shell-quoted confirm includes actual workspace; same pins apply exact bytes; changed skill and file counts separate |
| P1: web/MCP parity | E4, E11 | Adapter defaults diverge; web implements source→import transaction itself | Same fixture inputs yield equivalent proposals/effects/receipts through all adapters; failure injection at service boundary |

Hook installation is P2; its staged-validation prerequisite is non-negotiable whenever a hook claims commit validation [E10]. This is a gate based on a demonstrated counterexample, not a request to enforce hooks globally.

### Behavior-focused future scenarios

1. Fresh workspace: status gives one executable create/add example; no technical IDs required.
2. GitHub repo contains zero/one/many skills: zero fails without canonical records; one previews; many require explicit bounded selection.
3. Branch `feature/review` and path `skills/check`: resolver selects actual ref/path; competing viable split returns ambiguity without mutation.
4. Same branch/tag name with different commits; tag-only URL; pinned full SHA; deleted branch; changing default branch: explicit resolution or clear error, no fallback substitution.
5. Blob SKILL.md and tree folder produce the same selected skill, including root references/scripts/assets; non-entrypoint blob rejects.
6. Relative, `../`, quoted `~/`, absolute and space-containing local folders: selected root correct, copy bounded, original path absent from durable artifacts.
7. Symlink root/ancestor/descendant, junction, FIFO, Git symlink/submodule, encoded traversal and concurrent replacement: fail safely without escaped reads/writes.
8. Per-file, aggregate, resource count, transfer/mirror and timeout ceilings: exact named limit; reducing path does not falsely promise reduced full-clone transfer.
9. Origin name sanitization, reserved ID, duplicate batch IDs and existing IDs: no overwrite; explicit rename available for one skill; deterministic ordering.
10. Companion read failure and invalid bytes: incomplete package is not silently called complete; either a failed preview or explicitly acknowledged omission bound into the proposal.
11. Local/Git bytes change after preview: exact cached bytes are applied, or stale/source-changed fails; same approval never imports different content.
12. Add from already watched origin: no new watch policy, clear retained-watch disclosure; add from unwatched origin: monitoring remains disabled on replay and all-due checks.
13. Watch repeated locator: idempotent; changed scope/ref/cadence requires visible preview; local binding unavailable after clone asks to rebind without exposing another user's path.
14. Agent, editor and web edit stale read: expected base rejects; unrelated concurrent change follows documented conservative conflict policy.
15. Valid direct draft/active edit: automatic rebuild, correct lifecycle and routing availability; structural invalid edit: generation unchanged; unservable entrypoint: warning/isolated skill with explicit UI reason.
16. Review active-unservable and invalid metadata: returns useful diagnostics and next action offline; “reviewed” state never inferred from an old boolean alone.
17. Lifecycle remains draft→active→deprecated→archived; all activation requirements reported together; template/readiness errors separated from subjective warnings.
18. Working-tree-valid/index-invalid and inverse partial staging: staged checker evaluates only staged tree; original index, canonical files and published runtime unchanged.
19. Existing hook/core.hooksPath, linked worktree, alternate index, partial commit, merge conflict, staged deletion and malicious staged symlink: optional installer/checker preserves ownership and fails clearly.
20. CI clone/merge tree: empty-directory scaffold reconstructed, no repaired canonical file content, no network; active distribution faults fail strict validation.
21. Preview in non-TTY/JSON never prompts; `--yes` cannot resolve ambiguity or lifecycle effects; confirm typo/staleness errors distinguish causes and return non-zero.
22. Old CLI/MCP scripts and JSON consumers continue functioning; new human summaries omit internal pins except the complete advanced confirm command.

### Verification actually run

```sh
go test ./internal/delivery/cli ./internal/app ./internal/catalog ./internal/delivery/mcpserver \
  -run 'Test(SkillCLI|SourceImport|SourceCandidateCaptureIdempotent|.*Servable|.*AutoRebuild)' -count=1

go test ./internal/catalog ./internal/source ./internal/canonical \
  -run 'Test(BuildWarnsAboutActiveSkillsThatCannotBeServed|ValidateServableSkillRequiresMatchingFrontmatter|CanonicalChangeDuringBuildRejectsPublish|PostPublishCanonicalChangeReturnsExplicitStaleResult|RemotePolicyRejectsCredentialsProtocolsAndLocalAddresses|FilesystemAdapterContainsPathsRejectsSymlinksAndDetectsChange|FilesystemSnapshotNeverCapturesConcurrentSymlinkSwap|DNSAnswersArePublicAndRevalidatedForEveryConnection)' \
  -count=1 -v

go test ./internal/app -run TestResolveSkipsSkillWithUnservableFiles -count=1 -v
```

All matched tests passed. `internal/canonical` in the second command had no matching tests. The proposed commands and the 22 future scenarios are acceptance specifications, not passing implementation tests. No full release suite, usability study, Windows run, or live GitHub inference test is claimed.

## 11. Rejected ideas

| Rejected idea | Evidence and failure condition |
|---|---|
| A thin add wrapper that confirms onboarding then imports | Existing transactions are separate [E2, E3]; fails when import fails after watch registration. |
| Default import also watches | Reproduced no-monitor reversal [E2]; unacceptable hidden persistent policy effect. |
| Add means install/activate immediately | Import drafts have empty routing [E3]; fails on unreviewed source instructions becoming eligible. |
| Source add without explicit mode | Current watch/import distinction [E2, E3]; fails when users cannot predict durable effect. |
| Always split GitHub ref at first slash or choose longest prefix | Git permits slash refs [official Git rules](https://git-scm.com/docs/git-check-ref-format); fails on valid competing ref/path combinations. |
| Follow symlinks like gh's local executable extensions | Source containment [E12] and [gh install](https://cli.github.com/manual/gh_extension_install); fails when mutable external targets escape copied provenance. |
| Pins already protect editor-open-to-preview | Reproduced fresh overwrite [E5]; fails before pin creation. |
| Mandatory automatic merge on editor conflict | E5 shows need for a precondition, not merge semantics; fails on coherent-looking but unintended active instructions. |
| Direct edits bypass structural validation | Canonical checks and metadata experiment [E6]; false premise would unnecessarily restrict the plain-file product contract. |
| Every CLI-invalid entrypoint prevents generation publication | Deliberate servability isolation and passing tests [E7]; fails by contradicting verified current behavior. |
| Worktree validate in pre-commit is commit validation | Both staged counterexamples [E10]; fails under partial staging. |
| Hooks are shared enforcement | Git permits bypass [official hooks](https://git-scm.com/docs/githooks); fails when another clone or user lacks/runs no hook. |
| Old reviewed boolean proves current content approval | Activation writes it and future direct edits create no receipts [E4, E8]; fails after content changes. |
| Block arbitrary semantic phrases to prevent placeholders | E8 establishes only known template behavior; fails by rejecting legitimate prose without measuring usefulness. |
| Web UI hides primitives and fixes UX independently | Existing service boundary [E11]; fails when CLI/MCP/web implement different transaction/default logic. |
| Persist absolute local provenance in shared Git | Current filesystem-root mismatch [E2]; fails portability and exposes personal machine paths. |
| Rename/remove advanced commands immediately | Existing CLI/MCP contracts [E1, E11]; fails scripts without a safety or UX benefit on the common path. |

These rejections have specific failure conditions; none assumes that fewer commands automatically means safer behavior.

## 12. Open decisions

Only product choices remain; the technical invariants above are not approval questions.

1. **Import-and-watch shortcut:** keep the recommended two tasks, or fund one atomic combined service now. Two tasks are the smaller contract. A combined preview must disclose both effects and pass the same failure-injection gates [E2, E3].
2. **Watch cadence:** retain weekly as the compatibility-friendly default, or prefer manual for the new front door. In either case, watch records policy; checks require an explicit invocation or separately authorized scheduler. Existing weekly defaults are evidence, not a user-study result [E2].
3. **Creation authoring UX:** use a TTY editor/form when description/content are missing, or require explicit content options universally. Both must avoid activatable empty/template drafts, and non-TTY input must be deterministic [E8].
4. **Private/enterprise upstream support:** first release limited to credential-free public GitHub, or add explicit host/credential bindings. Existing safe transport is credential-free; broadening that contract needs separate scope and security evidence [E12].
5. **Usability success threshold:** select the minimum completion/recall improvement worth the additive workflows. Do not choose the winner solely by this report's modeled scores.

Recommended default for all other issues is specified above: one-locator draft import, no hidden watcher, explicit source watch, shared workflow services, digest-based edit conflict rejection, useful read-only review, optional staged hooks, and Git-owned team governance.
