# Phase 1 implementation report: embed system skill bundles and distill-lab

Branch `feat/distill-lab-bundle-and-script`, worktree `/home/vantt/projects/mcp-skill-hub-distill`, from main `327f2ed`.

## Commits

1. `chore: track the distill-lab source under .agents/skills` — repo `/.gitignore` only (`.agents/` replaced by the
   four anchored patterns from the phase file). Phase 2 can start from this commit.
2. `feat(systemskills): bundle distill-lab next to the curator as a multi-file system skill` — code, tests, the two
   SKILL.md copies and this report.

## What changed

- `.agents/skills/distill-lab/SKILL.md` and `internal/systemskills/distill-lab/SKILL.md`: verbatim copies of
  `.claude/skills/distill-lab/SKILL.md` (no `distill.py` copied). `.claude/skills/distill-lab/` untouched.
- `internal/systemskills/bundles.go`: `SkillFile`, `SystemSkill{ID, Version, Files, HubOnly, ServedOverMCP}`,
  `SystemSkills()` (deep copies; order system-curator, distill-lab; SKILL.md first then sorted),
  `KnownFileDigest(id, path, digest)`, `FileDigest(content)` (same `sha256:<hex>` form as `CuratorBundle`, exported
  for Phase 4), `DistillSkillID = "distill-lab"`, `DistillSkillVersion = "0.1.0"`. distill-lab is embedded with
  `//go:embed distill-lab` into an `embed.FS` and walked, so Phase 2's `scripts/**` files join the bundle with no
  code change.
- `internal/systemskills/known-digests.go`: 12 curator digests (every committed blob of both curator paths) and 3
  digests of the old dev distill-lab SKILL.md, keyed `SKILL.md`; no `distill.py` entry; comment gives the generating
  commands, the coverage difference and the rule for later edits.
- `embed.go` needed no change: `CuratorSkill`, `CuratorBundle()` and `CuratorMetadata()` are untouched (bytes and
  signatures).

## Tests

- `embed_test.go`: `TestDistillLabMirrorEqualsSource` (both trees walked like `embed.FS`, file set and bytes, plus
  embedded files equal the mirror on disk; failure names the copy command), `TestSystemSkillsListsBundlesInOrder`,
  `TestSystemSkillsReturnsDeepCopies`, `TestKnownFileDigest` (well-formed digests, current bundle recognized, arbitrary
  and cross-id digests rejected).
- `known_digests_test.go`: `TestKnownDigestsCoverReleasedBundles` over `git tag -l 'v*'`; skips without Git or tags
  unless `SKILLHUB_REQUIRE_TAGS=1`. It fails if a tag yields no system-skill file, which guards against a vacuous
  pass (the first draft listed paths relative to the package folder and checked nothing; fixed with
  `ls-tree --full-tree`).

## Validation run

- `SKILLHUB_REQUIRE_TAGS=1 go test -count=1 ./internal/systemskills/...` pass (tags v0.1.0 to v0.5.0-alpha.1).
- Removing the v0.1.0 curator digest makes the tag test fail with
  `v0.1.0 shipped internal/systemskills/curator/SKILL.md (system-curator SKILL.md) with a digest missing ...`.
  Removing the v0.4.1 digest does not fail, correctly: those bytes equal the current bundle.
- An extra file in the mirror makes the sync test fail naming the file and the fix.
- `go build ./...`, `go vet ./...`, `go test ./internal/hostintegration/...` pass; `gofmt -l` clean.
- `cmp` of both SKILL.md copies clean; `git check-ignore -v` reports `.agents/skills/system-curator/SKILL.md` and
  `.agents/other/x` ignored, `.agents/skills/distill-lab/SKILL.md` and `.agents/skills/distill-lab/scripts/lib/*`
  not ignored; `git status` listed only the intended files.

## Concerns

- The plan's sequencing says this phase runs after the onboarding release (0.5.0) is merged; the lead started it on
  main `327f2ed` by request. Requirement 4 asks for the curator digests to be generated on the post-0.5.0 main.
  When this branch is rebased onto that main, rerun the loop in the `known-digests.go` comment and add any curator
  digests the onboarding plan's curator text edits introduced (the tag test only enforces tagged versions, so it
  will not catch an untagged intermediate on its own).
- The CI wiring of `SKILLHUB_REQUIRE_TAGS=1` (lint job, release verify) is Phase 2's; until then the tag test runs
  only where tags are present and skips otherwise.
- Node 18 not relevant to this phase (no script yet).
