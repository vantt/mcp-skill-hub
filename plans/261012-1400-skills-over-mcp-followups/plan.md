---
title: "Skills over MCP follow-ups"
description: "Work deliberately left open after the skills-over-MCP conformance review: items that wait for a host to use the skills extension, and small cleanups."
status: pending
priority: P3
created: 2026-10-12
---

# Skills over MCP follow-ups

Source: [conformance review](../reports/review-261012-skills-over-mcp-conformance.md) and
[host check](../reports/review-261012-host-skills-extension-check.md). Shipped in v0.6.2.

## Waiting for a host that calls `skills/list`

No tested host (Claude Code 2.1.296, Codex 0.161.0) calls `skills/list` yet; Gemini CLI was not installed. Re-run the
host check when a host release changes that; the matrix status stays `unverified` until then.

- Proxy the skills extension through the remote-hub proxy.
- Move the `local` block to `_meta` in skill responses.
- Use the go-sdk types for the skills extension instead of local ones.
- Stateless HTTP listener mode.

## Waiting for the user

- Curator-compliance script.
- Evaluate delivering skills through `resources/list` (Claude Code calls it; the runtime list is empty).

## Cleanup

- Dead code: `openPinnedGeneration`, `isCanonicalEntityPath`, `splitSkillFrontmatter`, `safeSourcePath`,
  `OpenServable*` in catalog/app.
- The curator branch repeated in `internal/app/distribution.go`.
- Remote-hub proxy decodes JSON into `map[string]any`, so integers above 2^53 lose precision.

## Measured and skipped

- Merging the two observes in `skill_get` saves about 0.25 ms.
- A long-lived `*sql.DB` per generation was estimated at 6-9% and not measured.
- Freshness scan costs 4-5 ms at 300 skills; removing it needs a file watcher.

## Not verified locally

Web bundle and e2e run in CI only; F4 (`cd x && skillhub ... --yes` with Claude ask rules) is verified from docs only.
