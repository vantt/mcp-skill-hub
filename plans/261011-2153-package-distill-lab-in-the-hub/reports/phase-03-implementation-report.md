# Phase 3 implementation report: distill-lab skill text

Branch `feat/distill-lab-bundle-and-script`, worktree `/home/vantt/projects/mcp-skill-hub-distill`, on top of Phases 1 and 2.

## Changes

- `.agents/skills/distill-lab/SKILL.md` (copied byte for byte to `internal/systemskills/distill-lab/SKILL.md`):
  - frontmatter `version: 0.1.0`; `name` and `description` unchanged;
  - hub root: `$SKILLHUB_WORKSPACE`, else the current folder when it holds `.skillhub/schema-version`, else ask;
    the script finds the hub from the `distill.yaml` path;
  - new "Requirements" section (Node 18+, Git 2.45.0+, nothing to install, which commands need a hub, no fetch,
    "blob not fetched" handling, ERROR/WHY/FIX);
  - "The writer script" replaced by "Running the script", the only place that sets `S=` and shows
    `node $S check|format|list|decide` plus the optional `--hub` assertion; it says where Skill Hub installs the
    skill and that `.claude/skills/` holds only a pointer; it lists the user-visible differences from the previous
    implementation (duplicate keys rejected, UTF-8/LF output, symlink and tracked-mirror refusal);
  - every other `distill.py` mention now refers to that section;
  - untrusted-content paragraph before the pass; the mirror step uses a bare partial clone read through
    `git --git-dir=$M show|ls-tree|log`, never a checkout.
- `internal/systemskills/embed_test.go`: `TestDistillLabFrontmatterDeclaresVersion` and
  `TestDistillLabIsSelfContained` (bans `skillhub ` commands, `skill_`, `skills/get`, `~/`, `skill-hub`,
  `python`, `pyyaml`, `distill.py`, `npm install` in every bundled file except `scripts/lib/`; asserts the
  untrusted-content paragraph, the `--bare` clone line, the requirements line and a single `S=`).

## One addition to the phase text (correctness, not a design change)

The phase says "then `git --git-dir=<mirror> fetch`". A bare clone has no fetch refspec, so a plain fetch only
writes `FETCH_HEAD` and never moves `main` (checked with Git 2.53 in a scratch folder). SKILL.md therefore sets
`remote.origin.fetch '+refs/heads/*:refs/heads/*'` once after the clone and fetches with
`git --git-dir=$M fetch --prune origin`, and records the target with `rev-parse <ref>`.

## Validation

- `go test -count=1 ./internal/systemskills/... -skip TestDistillScriptLoaderMatchesReference`: pass.
  `go vet ./...`: pass. `gofmt -l`: clean.
- `grep -n -i "skill-hub\|python\|pyyaml" .agents/skills/distill-lab/SKILL.md`: nothing. No `:[0-9]` line
  references in SKILL.md.
- Scratch hub (HOME and XDG_* in a scratch folder): the documented mirror commands against a local upstream with
  `uploadpack.allowfilter`, then every command of "Running the script" on a two-lesson fixture: `check` reported
  "blob not fetched" first; one `git show` downloaded the blob; `format`, `check`, `list --top 15`,
  `decide 1 2 --state planned`, `decide <key> --state rejected --reason ...`, `check --hub <hub>` exit 0;
  `check --hub <other>` exits 1 with ERROR/WHY/FIX. Node 24.21.0 only; this phase changes no script code, so
  Phase 2's Node 18.20.8 run still covers the script.

## Concerns

- Pre-existing failure, not caused by this phase (it fails on the branch head without these edits):
  `TestDistillScriptLoaderMatchesReference` collects `git ls-files "internal/**/testdata/*distill*.yaml"`, and
  since Phase 2's commit that pathspec also matches its own `testdata/distill/format/*.yaml` goldens (6 files),
  which have no `loaded/*.json`. The test passed before the files were committed. Owner: Phase 2
  (`distill_script_test.go`); fix the pathspec (for example `internal/**/testdata/*-distill.yaml`) or add the six
  goldens.
- The script's message "blob not fetched; run git fetch in the mirror" is misleading for a `blob:none` clone:
  `git fetch` does not download blobs of old commits. SKILL.md gives the working step (`git show` once); the
  script text belongs to Phase 2.
