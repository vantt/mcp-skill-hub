---
phase: 14
title: "Trim docs/release-runbook.md"
status: completed
priority: P2
effort: "0.5h"
wave: 2
dependencies: [3, 4]
---

# Phase 14: Trim docs/release-runbook.md

## Audience and goal

The maintainer cutting a release. The runbook keeps release mechanics and drops the user install text, which
now lives in `docs/installation.md`. A release is in progress on main: this branch only edits text, it does not
change any release step.

## Owned files

- `docs/release-runbook.md` (about 160 lines).

## Required changes

1. `## Install, upgrade, and uninstall` (`:111-150`): replace the body with one line: "User install, upgrade
   and uninstall steps are in [Installation](installation.md)." Keep the heading (anchor survives) and keep the
   one maintainer fact that is not in installation.md: the uninstaller removes only the managed binary and marker.
2. Keep Supported platforms (archive table), preflight, pipeline, signature policy and manual verification
   (installation.md links to it: keep the heading "Manual artifact verification" unchanged), migration, disaster
   recovery, rollback, support.
3. Link `CONTRIBUTING.md` for the local test gates if the runbook repeats them; do not change the commands.
4. Light style pass only (no "simply", no exclamation marks); this is a maintainer page, so the avoid-terms rule
   does not apply.

## Commands to verify

None run; confirm nothing in the release steps changed: `git diff docs/release-runbook.md` touches only the
install section and wording.

## Acceptance criteria

- About 160 lines; no release step altered; the `#manual-artifact-verification` and
  `#install-upgrade-and-uninstall` anchors still exist.

## Rollback

Revert the commit.
