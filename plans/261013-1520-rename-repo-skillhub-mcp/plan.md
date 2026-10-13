---
title: "Rename the repository and Go module from mcp-skill-hub to skillhub-mcp"
description: "Drop the transport-first name: the repo, module path and release URLs become skillhub-mcp, while the product name Skill Hub, the skillhub command, SKILLHUB_* variables and user config stay unchanged. Includes a bridge release so installed binaries can still verify and self-update across the rename."
status: pending
priority: P2
effort: 6h
branch: main
tags: [rename, release, selfupdate, supply-chain]
blockedBy: []
blocks: []
created: 2026-10-13
---

# Rename mcp-skill-hub to skillhub-mcp

## Outcome, constraints, non-goals

- **Outcome.** The GitHub repository is `vantt/skillhub-mcp`, the Go module is
  `github.com/vantt/skillhub-mcp`, and every URL, installer, workflow and document uses the new name. A person
  with an installed v0.6.x binary can still run `skillhub self-update` and reach the first release built under
  the new name, without a manual reinstall.
- **Decision (user, 2026-10-13).** The name is `skillhub-mcp`. It was chosen over `mcp-skillhub` because the
  distinctive part comes first and `mcp` reads as a qualifier that can be dropped later. The reasoning, and the
  naming panel it came from, are in the PRD reference survey.
- **Constraints.**
  - Only the session `mcp-skill-hub-68` pushes, tags, renames the GitHub repository and releases. This plan
    prepares branches and commits; every remote step below is marked **68**.
  - Replace the exact string `mcp-skill-hub` only. The string `skill-hub` stands alone as the default hub
    folder (`~/skill-hub`) and must not change. Never touch `/home/vantt/skill-hub` or `/home/vantt/skillhub`.
  - Do not change the product name "Skill Hub", the `skillhub` command, `SKILLHUB_*` variables,
    `~/.config/skillhub`, the telemetry `EventVersion` or any event type.
  - Never rewrite published tags or the history of `archive/` and `plans/` records. Those keep the old name as
    historical record.
- **Non-goals.** Renaming the product, the binary or the hub folder; a new logo; trademark or npm/PyPI/domain
  checks (open item for the user, not part of this plan).

## Why a bridge release is required

Release verification pins the signing identity to the repository path. In
`internal/selfupdate/selfupdate.go` the identity is
`<DefaultSigstoreWorkflow>@refs/tags/<tag>` with the workflow URL
`https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml`, and the installers
(`scripts/install.sh`, `scripts/install.ps1`) pin the same prefix. Sigstore stamps a release with the repository
name at signing time. After the rename, a release is signed as
`https://github.com/vantt/skillhub-mcp/.github/workflows/release.yml@refs/tags/vX`, so every binary that only
knows the old identity (v0.6.4 and earlier) rejects it and aborts its self-update.

GitHub keeps redirects for a renamed repository, so the download URLs in old binaries still resolve. The
identity check is the only break. The fix is a release before the rename whose binary accepts **both**
identities, then the rename, then a release signed under the new name.

## Phases

| # | Phase | Owner | Depends on |
|---|-------|-------|-----------|
| 1 | Accept both signing identities (bridge release) | me, then **68** releases | none |
| 2 | Rename the module and every reference | me | phase 1 released |
| 3 | Rename the GitHub repository and ship the first release under the new name | **68** | phase 2 merged locally |
| 4 | Verify installers, self-update and links on the new name | me and **68** | phase 3 |

### Phase 1: bridge release (accept both identities)

- **Change.** In `internal/selfupdate/selfupdate.go` keep `DefaultSigstoreWorkflow` as is and add the new
  workflow URL as a second accepted identity. Verification passes when either identity verifies; a failure of
  both reports both identities in the error. Do the same in `scripts/install.sh` and `scripts/install.ps1`,
  which already check the identity before installing.
- **Files.** `internal/selfupdate/selfupdate.go` and its tests, `scripts/install.sh`, `scripts/install.ps1`,
  `scripts/test-installer-lifecycle.sh`, `scripts/test-installer-windows.ps1`, `docs/release-runbook.md`.
- **Tests.** Table test: identity A passes, identity B passes, a third identity fails, and the error text lists
  both accepted identities. Installer lifecycle tests keep a wrong-identity failure case.
- **Release.** **68** tags this as the next patch (v0.6.5). It is built and signed under the old name. After it
  ships, anyone who runs `skillhub self-update` once gets a binary that survives the rename.
- **Gate.** Do not start phase 3 until v0.6.5 is released, so that the number of people stuck on an older binary
  stays as small as possible.

### Phase 2: rename in the repository (one mechanical commit, then fixes)

1. In a branch, replace `github.com/vantt/mcp-skill-hub` with `github.com/vantt/skillhub-mcp` across Go files,
   `go.mod`, `web/go.mod`, `.github/workflows/*.yml`, `scripts/*`, `Makefile` and the ldflags in
   `release.yml`. Then replace the remaining bare `mcp-skill-hub` strings in tracked files **outside**
   `archive/`, `plans/` and the two path-based test fixtures below.
2. Leave alone: `archive/**` and `plans/**` (historical records), and
   `internal/telemetry/redactor_test.go` and `internal/transcripts/claude_code_test.go`, whose strings are the
   local checkout path `/home/vantt/projects/mcp-skill-hub` and the matching Claude Code project-directory
   slug, not the product name. If the checkout directory is later renamed, update those two fixtures with it.
3. Keep the old workflow URL as the second accepted identity from phase 1; do not remove it in this commit.
4. Update the user-facing text that names the repo: README badge and install URLs (both languages),
   `docs/installation.md` (clone and `cd` lines), `docs/release-runbook.md` (`REPO=`), `docs/troubleshooting.md`,
   `docs/remote-hub-guide.md` and the design docs. Add one line to the README explaining the names: the repo is
   `skillhub-mcp`, the product is Skill Hub, the command is `skillhub`.
5. Check that `web/go.mod` (its own module, `github.com/vantt/mcp-skill-hub/web`) and any `replace` directive
   still point at the root module, then run `go mod tidy` in the root and in `web/` and confirm both compile.
6. Verification: `gofmt -l .`, `go vet ./...` for linux, `GOOS=windows` and `GOOS=darwin`, `make check`,
   and `git grep -n "mcp-skill-hub"` showing only the intended leftovers (the two fixtures, the second accepted
   identity, `archive/`, `plans/`, and the release-note history).
7. Commit trailer as usual. No plan ids or phase numbers in code or commit messages.

### Phase 3: rename and release (**68**)

1. **68** renames the repository in GitHub settings to `skillhub-mcp`. GitHub redirects the old web, clone, API
   and release-download URLs. Update the local `origin` URL (`git remote set-url origin
   git@github.com:vantt/skillhub-mcp.git` or the HTTPS equivalent) in every checkout and worktree.
2. **68** pushes the phase 2 commits, waits for CI, tags the next version (v0.7.0 suggested, because the module
   path changes) and lets the release workflow sign and publish it.
3. **68** updates the repository description and topics: "Skill Hub: your skills in one place, pulled by any
   agent, on demand", topics `agent-skills`, `mcp`, `git-native`, `skills`.
4. Do not create a new repository at the old name: that would break the redirect.

### Phase 4: verify

- A fresh install from `https://github.com/vantt/skillhub-mcp/releases/latest/download/install.sh` verifies the
  signature with `cosign` and with `SKILLHUB_REQUIRE_SIGNATURE=1`.
- The old URL `https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh` still resolves through
  the redirect and installs the same release.
- An installed v0.6.5 binary runs `skillhub self-update` to the new release and the signature check passes.
- An installed v0.6.4 binary (old identity only) fails safe: it aborts with the identity message and prints the
  reinstall one-liner. Record that behaviour in the release notes so people on 0.6.4 or earlier know to reinstall.
- `go install github.com/vantt/skillhub-mcp/cmd/skillhub@latest` works. The old module path
  `github.com/vantt/mcp-skill-hub` stops being fetchable as a Go module even though the repo redirects, because
  the module's own `go.mod` declares the new path. Say so in the release notes.
- Badge, `go.mod`, CI and release workflow all show the new name; `git grep` finds no stray old URL.

## Risks and rollback

| Risk | Effect | Mitigation |
|------|--------|-----------|
| Binary older than the bridge cannot verify the new release | That user must reinstall once | Ship the bridge first; document the reinstall one-liner |
| Sigstore identity mismatch because the rename happens mid-release | Release fails verification | Rename only when no release workflow is running; tag after the rename |
| A tag for the next version is created before the rename | Signed under the old name | **68** renames first, then tags |
| `sed` touches `skill-hub` hub folder paths | Wrong default hub folder | Replace only the exact `mcp-skill-hub` string; review `git grep` output |
| Local checkout directory name | Two test fixtures keep the old directory string | They describe the local path, not the product; update them only if the directory is renamed |
| Someone clones the old URL | Works through the redirect | Do not reuse the old name for another repository |

Rollback before phase 3: drop the branch. After the repository is renamed: GitHub allows renaming back, which
restores the old name, and the bridge release already accepts both identities, so a rollback does not strand
anyone who installed v0.6.5.

## Acceptance criteria

- [ ] Bridge release v0.6.5 ships with both identities accepted in `selfupdate` and both installers, with tests.
- [ ] The module, workflows, installers and docs use `skillhub-mcp`; `git grep mcp-skill-hub` shows only the
      intended leftovers listed in phase 2.
- [ ] `gofmt`, `go vet` (three platforms) and `make check` pass; CI is green on Linux, Windows and macOS.
- [ ] The GitHub repository is renamed and the first release under the new name is signed and published.
- [ ] A fresh install, a v0.6.5 self-update and the old URL redirect are verified as described in phase 4.
- [ ] Release notes explain the rename, the reinstall step for v0.6.4 and earlier, and the Go module path change.
- [ ] The README names the three names in one line (repo, product, command).

## Open questions for the user

1. Is the next version after the rename v0.7.0 (a module-path change is breaking for Go importers, though there
   are few) or v0.6.6?
2. Should the local checkout directory `/home/vantt/projects/mcp-skill-hub` also be renamed to
   `skillhub-mcp`? Doing so changes the Claude Code project-directory slug and so detaches this project's
   session history; the plan assumes it stays.
3. Do you want the trademark, npm, PyPI and domain checks for `skillhub` done before the rename, or only before
   a broad launch?
