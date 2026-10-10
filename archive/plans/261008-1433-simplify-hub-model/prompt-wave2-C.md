# Wave 2 · Worktree C prompt: simplify Phase 4 → Phase 3

```text
You are implementing Phase 4 and then Phase 3 of
plans/261008-1433-simplify-hub-model/plan.md in the Go repo mcp-skill-hub.

STEP 0: WORK ONLY IN YOUR OWN WORKTREE (mandatory)
You started in the shared main checkout /home/vantt/projects/mcp-skill-hub.
The lead already moved your uncommitted work (workspace.go, migration.go,
migration/v3.go, phase-03/phase-04 files) into your own worktree:

  /home/vantt/projects/mcp-skill-hub-simplify   branch wave2/simplify-p4-p3

Your shell's working directory may still be the main checkout: `cd` into the
worktree first and use absolute paths under it. Never run a writing command
(edit, git add/commit/switch/branch/stash/reset) in /home/vantt/projects/mcp-skill-hub.
Verify with `git -C /home/vantt/projects/mcp-skill-hub-simplify status -sb`.

Another agent (worktree D) works in parallel on the observer plan
(docs/plans/2026-10-08-observer-and-enrichment.md: O3/O4 remainder, case journal).

Read these before coding:
- the simplify plan in full (Agreed decisions D2-D10, Phases, Parallel execution, Acceptance criteria,
  Red Team Review)
- observer plan §0, §3.1, §7 (D7) and §9
- the wave 1 results already on main:
  - phase-01-portability.md and phase-02-meta-rule.md, plus commits 8225938 and d115fc7:
    IsHubMeta, schema v2, migration planV1ToV2
  - docs/plans/observer-phase-01a-telemetry.md

Order: finish and commit Phase 4 completely, with `make check` green. Then start Phase 3.

PHASE 4: skill.meta.yaml → .meta/skill.yaml (D5, D6)
- Fix your phase-04-skill-yaml.md first:
  - Verify every row of the field destination table against the real Go structs and the
    schemas/skill-metadata.schema.json. Do not guess. For example, check what `quality`
    and `content` really hold today and who reads them before marking them deleted.
  - A deletion needs evidence that nothing reads the field. For a resolver field, it also needs
    routing-eval evidence.
  - Keep `runtime` inside the content digest.
  - Derive third-party status from "has a source with role `upstream`". The `learning`
    role never makes a skill third-party.
  - An `upstream` source keeps `kind`, `files_digest`, `transformations`, and the `synced` cursor.
- Migration:
  - One registered step planV2ToV3 (schema "3") through the migration WAL.
  - It is idempotent and refuses newer schemas.
  - It must handle a skill that already has a `.meta/skill.yaml` written by Phase 0 next to
    `skill.meta.yaml` (the live hub's test-audit does). Do not overwrite silently: merge or fail
    with a clear finding.
- Code changes:
  - internal/canonical reads `.meta/skill.yaml`.
  - The writers in app/ write it.
  - The resolver keeps receiving aliases/topics/technologies/domain unchanged. You may change only
    the metadata field accessors in internal/resolver/evidence.go. Note it in your report:
    worktree D owns the rest of internal/resolver.
- Approval-history walk (internal/app/skill_review_changes.go): follow both the old and the new
  path, and leave `.meta/` out of the blob sets.
- Required tests:
  - The trust verdict and upstream status are unchanged for every skill of a fixture copied
    from a realistic hub (third-party, self-authored, test-audit-like).
  - Migration v2→v3 runs once and twice; a v4 workspace is refused.
  - An upstream that ships `.meta/skill.yaml` still cannot approve itself.
  - Writing `.meta/skill.yaml` changes CatalogSnapshot. Writing `.meta/distill.yaml` does not.

PHASE 3: minimal distill model (D1-D3, D7)
- Fix your phase-03-minimal-distill.md first:
  - `notable` is a short text explaining why the lesson matters. It is not a boolean or score.
  - The phase 0 scorecard does not exist yet. Keep `contrast` and the R/E/F scores as OPTIONAL,
    experimental fields. Do not make them required schema.
  - List the exact Go types, packages and files that are kept and removed.
- Implement one file `.meta/distill.yaml` per skill: `goal`, `cursors`, `coverage`, `lessons`.
  - A lesson has `key`, `what`, `notable` and `where`. `where` has one entry per source, and
    several entries mean convergence. Git evidence is repo@<40-hex sha>:path[#Lx-Ly].
    `usage:<case_id>` is allowed for the observer.
  - Each lesson has an inline `decision`: candidate|planned|ported|rejected, with `reason`, `at`,
    and the `where` entries seen. A new `where` entry reopens a decided lesson.
  - The cursor advances in the same write as the lessons.
- Remove the following:
  - revision packages (internal/distill/revision_package.go)
  - the run state machine
  - insight/incorporation entities (internal/insight)
  - LINK files
  - the sources/intake and top-level sources/ layout (D3: every source belongs to a skill;
    a source saved for later goes to an existing skill or to a new draft skill, and draft
    skills never route and never enter routing evals)
- KEEP the following:
  - the preview/confirm pin (mutation.Proposal, runtime pin store) and the workspace lock
  - the human content approval
  - a stable catalog_snapshot identity
  - the ability to build a temporary catalog without publishing it (D10, observer replay)
- Migrate the existing distill data (runs, insights, decisions, rejections, tombstones,
  coverage gaps) into distill.yaml in a registered step planV3ToV4 (schema "4"). Build a
  pre-Phase-3 fixture and test that human decisions, reopen-only-on-new-evidence, tombstones
  and coverage gaps survive.
- Contract table in phase-03 for every MCP tool, web route, JSON schema (schemas/*.json) and
  telemetry event: kept / renamed / removed / deprecated. Telemetry event types and EventVersion
  are ALL kept (D10). Removing a public contract needs its line in the table AND in
  docs/design/07, in the same commit as the code.
- Ported text from a learning source into skill content goes through a normal skill_update
  preview that shows the source excerpt (D9).
- Write the server-boundary test deferred from Phase 1: every tool the curator SKILL.md body uses
  is registered and listed in its compatible-tools. Keep the two curator copies identical.

Owned paths in this wave
- internal/{canonical,workspace,migration,skill,skillruntime,distill,insight,source,mutation}
  - mutation: only to remove dead lifecycle code; the pin store and lock stay
- internal/catalog: snapshot input only
- app/: distill*, insight*, operations, skill_* (except skill_load_telemetry.go), upstream_*,
  source_*, distribution.go
- internal/delivery/mcpserver/server.go registerTools and insight_tools.go (you are their single
  owner in wave 2)
- distill/insight/source views and routes in internal/delivery/web and internal/delivery/cli
- internal/systemskills, system-skills/curator
- schemas/*.json except telemetry-event-v1.schema.json
- docs/design/01 and docs/design/07
- this plan's phase files

Do NOT edit (worktree D's or shared):
- internal/telemetry
- schemas/telemetry-event-v1.schema.json
- docs/design/04
- internal/app/{resolver,usage,curation_telemetry,skill_load_telemetry,telemetry,feedback,routing_eval,transcript_import}.go
- internal/delivery/mcpserver/{activation_tracker,resolver_tools}.go
- internal/resolver, except the evidence.go field accessors above
- CLI/web telemetry views
If you need a change there, write it as a follow-up in your final report.

Rules
- Read AGENTS.md and follow it.
- Never remove telemetry event types or change EventVersion.
- Preserve public contracts unless the contract table says otherwise. A schema change lands in
  the same commit as its code.
- Use conventional commits, small and focused, each ending with
  "Co-Authored-By: <your model name> <noreply@...>" (state the model you really are).
- Run `make check` (fmt-check, vet, lint, test) green before EVERY commit.
- Never touch /home/vantt/skill-hub (the live hub). Copy what you need into test fixtures.
  Do not run migrations against it.
- Do not push and do not merge into main.
- If Phase 4 is done and Phase 3 is too big for one session, stop after Phase 4, report it,
  and describe exactly where Phase 3 stands.
- When done, rebase on the latest main, resolve conflicts only inside your owned paths, run
  `make check` again, and stop.

Final report
- worktree path and branch
- confirmation that the main checkout is back on `main` with none of your changes
- commits (hash + subject)
- the final field destination table
- schema versions and migration steps added
- the contract table summary (removed / renamed / deprecated)
- tests added
- `make check` result
- deviations from the plan
- follow-ups for worktree D and wave 3 (observer §5.1, O7 baseline, observer Phase 3, simplify Phase 5)
- end with "Status: DONE | DONE_WITH_CONCERNS | BLOCKED" and a one-sentence summary
```
