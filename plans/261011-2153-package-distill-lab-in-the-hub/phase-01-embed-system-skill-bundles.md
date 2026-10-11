---
phase: 1
title: "Embed system skill bundles and distill-lab"
status: pending
priority: P2
effort: "4h"
wave: 1
dependencies: []
---

# Phase 1: Embed system skill bundles and distill-lab

## Context (traced)

- Today one system skill is embedded as a string: `//go:embed curator/SKILL.md` into `CuratorSkill`
  (`internal/systemskills/embed.go:23-27`); `CuratorBundle()` returns instructions plus a sha256 digest
  (`:94-102`). Consumers: `internal/hostintegration/bootstrap.go:19`, `internal/app/distribution.go:131,516-517`,
  `internal/delivery/mcpserver/server.go:329-331,365,419`, `skill_tools.go:214,229`, `internal/app/skill_bundle.go:54,130`,
  `internal/app/skill_snapshot.go:393`. They keep working unchanged.
- The editable curator source is `system-skills/curator/SKILL.md`; `TestCuratorSkillIsEmbeddedAndVersioned`
  asserts the embedded copy equals it (`internal/systemskills/embed_test.go:22-28`). `AGENTS.md:16` names that path.
- The distill-lab dev copy is `.claude/skills/distill-lab/SKILL.md` (493 lines) and `scripts/distill.py`
  (421 lines), both mode 100644 in Git. The repo `.gitignore` ignores `.agents/`, `.claude/` and `.gemini/` as
  local agent folders; the dev copy is tracked because it was added explicitly.
- Source of truth (plan decision 3): this repo's `.agents/skills/distill-lab/` becomes the single editable source,
  and `internal/systemskills/distill-lab/` is its embed mirror. There is no `system-skills/distill-lab/`.
  `.agents/skills/system-curator/` exists locally only because `connect` wrote it; it must stay ignored.
- The script is rewritten in Node by Phase 2, which owns `scripts/**` in both trees. This phase copies only
  `SKILL.md`; `distill.py` is never copied into the new layout.

## Owned files

- Create: `.agents/skills/distill-lab/SKILL.md` (verbatim copy of the dev SKILL.md),
  `internal/systemskills/distill-lab/SKILL.md` (byte-identical mirror), `internal/systemskills/bundles.go`,
  `internal/systemskills/known-digests.go`, `internal/systemskills/known_digests_test.go` (the tag-derived test).
- Modify: `internal/systemskills/embed.go`, `internal/systemskills/embed_test.go`, repo `/.gitignore`.
- Not owned: `scripts/**` under either tree and `internal/systemskills/distill_script_test.go` (Phase 2).
- Read-only: everything else, including `.claude/skills/distill-lab/**` (Phase 9 turns it into a wrapper).

## Requirements

1. Add a bundle model (names are suggestions):
   ```go
   type SkillFile struct { Path string; Content []byte } // slash-separated, relative to the skill folder
   type SystemSkill struct {
       ID, Version   string
       Files         []SkillFile // SKILL.md first, then sorted
       HubOnly       bool        // installed only in the hub folder
       ServedOverMCP bool        // listed by skills/list and skills/get
   }
   func SystemSkills() []SystemSkill            // deterministic order: system-curator, distill-lab
   func KnownFileDigest(id, path, digest string) bool // current bundle bytes or a previously shipped version
   ```
   Curator: `HubOnly=false`, `ServedOverMCP=true`. distill-lab: `HubOnly=true`, `ServedOverMCP=false`.
   Return defensive copies.
2. Embed distill-lab with `//go:embed distill-lab` into an `embed.FS`; walk it to build `Files`, so every file
   Phase 2 adds (`scripts/distill.mjs`, `scripts/vendor/js-yaml.mjs`, `scripts/vendor/js-yaml.LICENSE`) is part of
   the bundle without further code. `embed.FS` skips dot and underscore files by default.
3. `DistillSkillID = "distill-lab"`, `DistillSkillVersion = "0.1.0"`. The version is asserted against the
   SKILL.md frontmatter once Phase 3 adds the `version:` field; until then the test checks the constant only.
4. `KnownFileDigest` is true for the current bytes of any bundle file (computed from the bundle, so the vendored
   js-yaml and its license are owned like every other file) and for the digests in `known-digests.go`: a static
   map `id -> skill-relative path -> []sha256` of previously shipped bytes, generated once from Git history and
   pasted in, with a comment explaining how it was produced (no plan or phase ids). Sources:
   - every committed blob of `internal/systemskills/curator/SKILL.md` and `system-skills/curator/SKILL.md`
     (12 commits before 5.0.0: `git log --format=%H -- <path>` then `git show <sha>:<path> | sha256sum`; run it
     on the post-5.0.0 main so the curator text edits made by the onboarding plan are included);
   - every committed blob of the old dev `.claude/skills/distill-lab/SKILL.md`, keyed as `SKILL.md`, so an
     unedited hand copy in an owner's hub is recognized and replaced by the wrapper. These digests stay valid
     after Phase 9 because the key is the path inside the skill folder, not the repo path.
   - No `scripts/distill.py` digest: Skill Hub never installed it (it existed only in the dev copy), so it was
     never shipped. State this in the comment.
   Rule for later edits, stated in the comment: when a bundled file changes after a release, add the digest of
   the bytes that shipped.
   <!-- Red team: hand-maintained known digests -->A test enforces the rule instead of memory:
   `TestKnownDigestsCoverReleasedBundles` (in `known_digests_test.go`) lists release tags with `git tag -l 'v*'` and, for every tag, every file
   under `internal/systemskills/curator/` and `internal/systemskills/distill-lab/` in that tag
   (`git ls-tree -r --name-only <tag> -- <dir>` then `git show <tag>:<path>`), asserts its digest is known for that
   id and skill-relative path. So a release that changes a bundled file without adding the old digest fails this test
   on the next commit, not on a user's upgrade. The test skips when the checkout has no tags or no Git, unless
   `SKILLHUB_REQUIRE_TAGS=1`, then it fails; the CI `lint` job (full history) and release verify set it (workflow
   lines owned by Phase 2). Tags `v0.1.0` to `v0.5.0-alpha.1` exist today and each holds the curator file.
5. Do not change `CuratorSkill`, `CuratorBundle()` or `CuratorMetadata()` signatures or bytes.
6. Repo `/.gitignore`: replace the line `.agents/` with anchored patterns that keep everything under `.agents/`
   ignored except the distill-lab source:
   ```gitignore
   /.agents/*
   !/.agents/skills/
   /.agents/skills/*
   !/.agents/skills/distill-lab/
   ```

## Tests (in `embed_test.go`)

- Files under `internal/systemskills/distill-lab/` equal `../../.agents/skills/distill-lab/` byte for byte, file
  set included (walk both trees, skip names starting with `.` or `_` like `embed.FS`). The failure message names
  the fix: copy `.agents/skills/distill-lab/` over `internal/systemskills/distill-lab/`.
- `SystemSkills()` returns both ids in order, SKILL.md first, no empty file, curator bytes equal `CuratorSkill`.
- Every digest in the known list is a well-formed `sha256:` hex string; every current bundle file of both ids is
  recognized by `KnownFileDigest`; an arbitrary digest is not.
- <!-- Red team: hand-maintained known digests --> `TestKnownDigestsCoverReleasedBundles` (requirement 4) passes
  with `SKILLHUB_REQUIRE_TAGS=1` on a full clone; removing one entry from the table makes it fail with the tag and
  path named.
- Mutating a returned `SystemSkill` does not change the next call's result.

## Steps

1. Edit `/.gitignore` and commit it alone first (`chore: track the distill-lab source under .agents/skills`), so
   Phase 2 can start from that commit. <!-- Red team: Phase 2 files ignored until Phase 1 --> Then copy the dev
   SKILL.md verbatim into `.agents/skills/distill-lab/` and `internal/systemskills/distill-lab/`.
2. Add `bundles.go`, constants, `known-digests.go`; wire curator into the list from the existing string.
3. Extend tests; run `go test ./internal/systemskills/...`, then `go build ./...`.
4. Commit: `feat(systemskills): bundle distill-lab next to the curator as a multi-file system skill`.

## Validation

- `go test ./internal/systemskills/... && go build ./... && go vet ./internal/systemskills/...` pass.
- `cmp` of every file between the two trees is clean; `.claude/skills/distill-lab/` shows no diff in `git status`.
- `git check-ignore -v .agents/skills/system-curator/SKILL.md` reports it ignored;
  `git check-ignore .agents/skills/distill-lab/SKILL.md` reports nothing; `git status --porcelain` lists only the
  intended new files (no other previously ignored path appears).

## Risks

- Binary size grows by about 110 KB once Phase 2 lands (js-yaml is 104 KB): acceptable.
- A future edit to only one of the two trees: caught by the sync test.
- Until Phase 9, Claude Code in this repo still loads the old full dev copy (with `distill.py`) from
  `.claude/skills/distill-lab/` while Codex loads the new source: transient and intended.

## Rollback

Revert the commit; no consumer uses the new API until Phase 4.
