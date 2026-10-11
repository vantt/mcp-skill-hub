---
phase: 1
title: "Embed system skill bundles and distill-lab"
status: pending
priority: P2
effort: "3h"
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
- The editable source is `system-skills/curator/SKILL.md`; `TestCuratorSkillIsEmbeddedAndVersioned` asserts the
  embedded copy equals it (`internal/systemskills/embed_test.go:22-28`). `AGENTS.md:16` names that path.
- distill-lab is two files in this code repo: `.claude/skills/distill-lab/SKILL.md` (493 lines) and
  `scripts/distill.py` (421 lines), both mode 100644 in Git. The dev copy is not edited in this phase.

## Owned files

- Create: `system-skills/distill-lab/SKILL.md`, `system-skills/distill-lab/scripts/distill.py` (verbatim copies
  of the dev copy), `internal/systemskills/distill-lab/` (byte-identical mirror), `internal/systemskills/bundles.go`,
  `internal/systemskills/known-digests.go`.
- Modify: `internal/systemskills/embed.go`, `internal/systemskills/embed_test.go`.
- Read-only: everything else, including `.claude/skills/distill-lab/**`.

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
   func KnownFileDigest(id, path, digest string) bool // current or previously shipped bytes
   ```
   Curator: `HubOnly=false`, `ServedOverMCP=true`. distill-lab: `HubOnly=true`, `ServedOverMCP=false`.
   Return defensive copies.
2. Embed distill-lab with `//go:embed distill-lab` into an `embed.FS`; walk it to build `Files`. `embed.FS` skips
   dot and underscore files by default, which is what we want (no `__pycache__`).
3. `DistillSkillID = "distill-lab"`, `DistillSkillVersion = "0.1.0"`. The version is asserted against the
   SKILL.md frontmatter once Phase 2 adds the `version:` field; until then the test checks the constant only.
4. `known-digests.go`: a static map `id -> path -> []sha256` generated once from Git history and pasted in,
   with a comment explaining how it was produced (no plan or phase ids). Sources:
   - every committed blob of `internal/systemskills/curator/SKILL.md` and `system-skills/curator/SKILL.md`
     (12 commits today: `git log --format=%H -- <path>` then `git show <sha>:<path> | sha256sum`);
   - every committed blob of `.claude/skills/distill-lab/SKILL.md` and `.claude/skills/distill-lab/scripts/distill.py`
     (so a hand-copied, unedited dev version in an owner's hub is recognized as ours).
   Keep a small `scripts/` helper only if it is reused; otherwise document the two commands in the comment.
5. Do not change `CuratorSkill`, `CuratorBundle()` or `CuratorMetadata()` signatures or bytes.

## Tests (in `embed_test.go`)

- distill-lab files under `internal/systemskills/distill-lab/` equal `system-skills/distill-lab/` byte for byte,
  file set included (walk both trees).
- `SystemSkills()` returns both ids in order, SKILL.md first, no empty file, curator bytes equal `CuratorSkill`.
- Every digest in the known list is a well-formed `sha256:` hex string; the current curator digest is
  recognized by `KnownFileDigest`.
- Mutating a returned `SystemSkill` does not change the next call's result.

## Steps

1. Copy the dev copy verbatim into `system-skills/distill-lab/` and `internal/systemskills/distill-lab/`.
2. Add `bundles.go`, constants, `known-digests.go`; wire curator into the list from the existing string.
3. Extend tests; run `go test ./internal/systemskills/...`, then `go build ./...`.
4. Commit: `feat(systemskills): bundle distill-lab next to the curator as a multi-file system skill`.

## Validation

- `go test ./internal/systemskills/... && go build ./... && go vet ./internal/systemskills/...` pass.
- `cmp` of every file between the two trees is clean; the dev copy shows no diff in `git status`.

## Risks

- Binary size grows by about 50 KB: acceptable.
- A future edit to only one of the two trees: caught by the sync test.

## Rollback

Revert the commit; no consumer uses the new API until Phase 3.
