---
title: "Rename the repository and Go module from mcp-skill-hub to skillhub-mcp"
description: "Drop the transport-first name: the repo, module path and release URLs become skillhub-mcp, while the product name Skill Hub, the skillhub command, SKILLHUB_* variables and user config stay unchanged. Decision: no bridge release; binaries older than the first release under the new name reinstall once."
status: completed
priority: P2
effort: 4h
branch: main
tags: [rename, release, selfupdate, supply-chain]
blockedBy: []
blocks: []
created: 2026-10-13
---

# Rename mcp-skill-hub to skillhub-mcp

## Outcome, constraints, non-goals

- **Outcome.** The GitHub repository is `vantt/skillhub-mcp`, the Go module is
  `github.com/vantt/skillhub-mcp`, and every URL, installer, workflow and document uses the new name. People on
  v0.6.4 or earlier reinstall once with the install one-liner (decision below).
- **Decision (user, 2026-10-13).** No bridge release: older binaries cannot verify a release signed under the
  new name, and that is accepted. Whoever is on v0.6.4 or earlier reinstalls once.
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

## What breaks without a bridge, and why it is accepted

Release verification pins the signing identity to the repository path: `internal/selfupdate/selfupdate.go`
builds `<DefaultSigstoreWorkflow>@refs/tags/<tag>` from
`https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml`, and `scripts/install.sh` and
`scripts/install.ps1` pin the same prefix. A release signed after the rename carries
`https://github.com/vantt/skillhub-mcp/...`, so a v0.6.4 binary rejects it and `skillhub self-update` aborts
before replacing anything (fails safe). GitHub redirects keep the old download URLs working, so the only break is
that one signature check, and the cure is one reinstall with the install one-liner, which is fetched from the
redirected URL and already carries the new identity. Nothing on disk is lost.

## Working in separate worktrees

The main checkout stays untouched while the rename is prepared: it may hold other sessions' uncommitted work, and
the module-path change would make every open build in it stale. The preparing phase runs in its own git worktree
created from `origin/main`, never from the main checkout's working tree.

| Phase | Worktree | Branch | Starts from |
|-------|----------|--------|-------------|
| 1 | `.claude/worktrees/rename-module` | `rename-module` | `origin/main` |

Rules for the worktrees:

- Create them with `git worktree add -b <branch> .claude/worktrees/<name> origin/main` and remove only the ones
  this work created. The worktrees `agent-a238343e8dbb379fc` and `agent-a3357cc17b83133d8` belong to other
  sessions and stay.
- Work and run every command inside the worktree (`cd` into it, or `git -C` and `go` with the worktree as the
  working directory). Do not run `go mod tidy` or the mechanical replace in the main checkout.
- Run `make check` and the three-platform `go vet` inside the worktree. Start no long-running process there; if a
  test starts one, stop it before the worktree is removed.
- Merge into local `main` with a normal merge commit once the checks pass (no rebase, no history rewrite), then
  remove the worktree and its branch. Only **68** pushes, and only after the repository rename in phase 2.
- Settings such as `git remote set-url origin` are shared by all worktrees of the repository, so phase 2 changes
  the URL once.
- Before starting, re-run the `git grep -n "mcp-skill-hub"` inventory in the new worktree; do not trust a list made
  earlier.

## Phases

| # | Phase | Owner | Depends on |
|---|-------|-------|-----------|
| 1 | Rename the module and every reference | me | none |
| 2 | Rename the GitHub repository and ship the first release under the new name | **68** | phase 1 merged locally |
| 3 | Verify installers, redirects and links on the new name | me and **68** | phase 2 |

### Phase 1: rename in the repository (one mechanical commit, then fixes)

1. In the `rename-module` worktree, replace `github.com/vantt/mcp-skill-hub` with `github.com/vantt/skillhub-mcp` across Go files,
   `go.mod`, `web/go.mod`, `.github/workflows/*.yml`, `scripts/*`, `Makefile` and the ldflags in
   `release.yml`. Then replace the remaining bare `mcp-skill-hub` strings in tracked files **outside**
   `archive/`, `plans/` and the two path-based test fixtures below.
2. Leave alone: `archive/**` and `plans/**` (historical records), and
   `internal/telemetry/redactor_test.go` and `internal/transcripts/claude_code_test.go`, whose strings are the
   local checkout path `/home/vantt/projects/mcp-skill-hub` and the matching Claude Code project-directory
   slug, not the product name. If the checkout directory is later renamed, update those two fixtures with it.
3. `DefaultSigstoreWorkflow`, `DefaultReleaseBaseURL`, the installers and the installer test scripts move to
   the new name together with everything else; there is no second accepted identity.
4. Update the user-facing text that names the repo: README badge and install URLs (both languages),
   `docs/installation.md` (clone and `cd` lines), `docs/release-runbook.md` (`REPO=`), `docs/troubleshooting.md`,
   `docs/remote-hub-guide.md` and the design docs. Add one line to the README explaining the names: the repo is
   `skillhub-mcp`, the product is Skill Hub, the command is `skillhub`.
5. Check that `web/go.mod` (its own module, `github.com/vantt/mcp-skill-hub/web`) and any `replace` directive
   still point at the root module, then run `go mod tidy` in the root and in `web/` and confirm both compile.
6. Verification: `gofmt -l .`, `go vet ./...` for linux, `GOOS=windows` and `GOOS=darwin`, `make check`,
   and `git grep -n "mcp-skill-hub"` showing only the intended leftovers (the two fixtures, `archive/`, `plans/`
   and the release-note history).
7. Commit trailer as usual. No plan ids or phase numbers in code or commit messages.

### Phase 2: rename and release (**68**)

1. **68** renames the repository in GitHub settings to `skillhub-mcp`. GitHub redirects the old web, clone, API
   and release-download URLs. Update the local `origin` URL (`git remote set-url origin
   git@github.com:vantt/skillhub-mcp.git` or the HTTPS equivalent) in every checkout and worktree.
2. **68** pushes the phase 1 commits, waits for CI, tags the next version (v0.7.0 suggested, because the module
   path changes) and lets the release workflow sign and publish it.
3. **68** updates the repository description and topics: "Skill Hub: your skills in one place, pulled by any
   agent, on demand", topics `agent-skills`, `mcp`, `git-native`, `skills`.
4. Do not create a new repository at the old name: that would break the redirect.

### Phase 3: verify

- A fresh install from `https://github.com/vantt/skillhub-mcp/releases/latest/download/install.sh` verifies the
  signature with `cosign` and with `SKILLHUB_REQUIRE_SIGNATURE=1`.
- The old URL `https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh` still resolves through
  the redirect and installs the same release.
- An installed v0.6.4 binary fails safe: `skillhub self-update` aborts with the identity message and changes
  nothing, and the reinstall one-liner replaces it. Run this once on a throwaway HOME and record the exact
  behaviour in the release notes so people on 0.6.4 or earlier know to reinstall.
- The three calls a binary makes through the redirect work (release listing used to find the latest version,
  asset download, signature bundle download), checked from the new binary on a machine that never saw the new name.
- `go install github.com/vantt/skillhub-mcp/cmd/skillhub@latest` works. The old module path
  `github.com/vantt/mcp-skill-hub` stops being fetchable as a Go module even though the repo redirects, because
  the module's own `go.mod` declares the new path. Say so in the release notes.
- Badge, `go.mod`, CI and release workflow all show the new name; `git grep` finds no stray old URL.

## Release-process review notes (by the releasing session, 2026-10-13)

- The identity pins are only in `internal/selfupdate/selfupdate.go` (lines 30, 36, 389, 390, 418, 419),
  `scripts/install.sh` (7, 28), `scripts/install.ps1` (26, 30) and the installer test scripts; `.github/` has
  none, so the release workflow needs only the ldflags and module paths.
- Rename only while no release workflow runs, and tag only after `gh repo view` shows the new name. After the
  rename check that Actions and the release workflow are enabled and that secrets and OIDC permissions survived.
- Fix `REPO=` in `docs/release-runbook.md` and any release-note template that links the old compare URL.

## Risks and rollback

| Risk | Effect | Mitigation |
|------|--------|-----------|
| Binary v0.6.4 or older cannot verify the new release | That user reinstalls once; self-update fails safe | Accepted; document the one-liner in the release notes |
| Sigstore identity mismatch because the rename happens mid-release | Release fails verification | Rename only when no release workflow is running; tag after the rename |
| A tag for the next version is created before the rename | Signed under the old name | **68** renames first, then tags |
| `sed` touches `skill-hub` hub folder paths | Wrong default hub folder | Replace only the exact `mcp-skill-hub` string; review `git grep` output |
| Local checkout directory name | Two test fixtures keep the old directory string | They describe the local path, not the product; update them only if the directory is renamed |
| Someone clones the old URL | Works through the redirect | Do not reuse the old name for another repository |

Rollback before phase 2: drop the branch. After the repository is renamed: GitHub allows renaming back, which
restores the old name; releases signed under the new name would then fail the old identity, so roll back only
before the first new release.

## Acceptance criteria

- [ ] The module, workflows, installers and docs use `skillhub-mcp`; `git grep mcp-skill-hub` shows only the
      intended leftovers listed in phase 1.
- [ ] `gofmt`, `go vet` (three platforms) and `make check` pass; CI is green on Linux, Windows and macOS.
- [ ] The GitHub repository is renamed and the first release under the new name is signed and published.
- [ ] A fresh install, the old URL redirect and the v0.6.4 fail-safe are verified as described in phase 3.
- [ ] Release notes explain the rename, the reinstall step for v0.6.4 and earlier, and the Go module path change.
- [ ] The README names the three names in one line (repo, product, command).

## Decisions and open questions

1. **Version (user, 2026-10-13):** the first release under the new name is v0.6.6.
2. Local checkout directory `/home/vantt/projects/mcp-skill-hub`: kept as is (renaming it detaches this project's
   Claude Code session history). The two path fixtures keep the old directory string.
3. Trademark, npm, PyPI and domain checks for `skillhub`: not needed now (user, 2026-10-13); revisit before any
   broad launch.

## Progress

- 2026-10-13: the GitHub repository was renamed to `vantt/skillhub-mcp`; local `origin` follows it.
- 2026-10-13: phase 1 merged into local `main` (module path, workflows, installers, selfupdate constants, docs,
  README names line). `gofmt`, `go vet` on three platforms, `bash -n` and `make check` pass. Not pushed.
- 2026-10-13: v0.6.6 tagged and released by 68 (Release workflow green, signed, smoke installs passed on three
  OSes). Checked from this side: the new and the old `install.sh` URLs resolve to the same asset, and
  `go list -m github.com/vantt/skillhub-mcp@v0.6.6` resolves through the Go proxy.
- 2026-10-14: the v0.6.4 `update` fail-safe was run on a copy of the v0.6.4 binary in a throwaway HOME (the copy
  needs a `.skillhub-managed` marker; the real install was untouched). Both paths abort and leave the binary
  byte-identical: plain `skillhub update` stops with "Failed to parse release tag from redirect" (it reads the
  repository redirect, `/vantt/skillhub-mcp/releases/latest`, as the final URL), and `skillhub update --version
  0.6.6` stops with "Signature verification failed" because the certificate subject is the new workflow. Neither
  message says the repository was renamed, and v0.6.4 cannot be changed, so the release notes must tell people on
  v0.6.4 or earlier to reinstall with the install one-liner.
