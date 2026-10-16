---
name: release-checklist
description: Walk through the team's release checklist before tagging a version. Use when preparing a release.
---

# release-checklist

## When to use

- prepare a release

## When not to use

- writing release notes

## Examples

- get this repository ready for a release
- check that we can tag version 1.4.0
- run the release checklist

## Instructions

1. Run `scripts/check.sh` and fix anything it reports.
2. Make sure the changelog has an entry for the new version.
3. Tag the release only when the checks pass.
