---
title: Plan intent-first curation CLI workflows
date: 2026-10-01
summary: Created and adversarially validated a parallel nine-phase implementation plan for the curation CLI redesign.
---

# Plan intent-first curation CLI workflows

## What happened
Synthesized two independent curation CLI reviews into a nine-phase parallel implementation plan. Four codebase scouts mapped engine, add/watch, lifecycle/review, and delivery surfaces. Four adversarial reviewers found and evidenced staged-index, fallback-content, MCP filesystem, Windows traversal, proposal-dispatch, state-basis, and dependency defects.

## Decision
Keep Design A. Run canonical/catalog/locator foundations in parallel, then lifecycle safety, skill add, parallel CLI/MCP adapters, documentation reconciliation, and one integration gate. Validate staged bytes from raw Git objects, restrict local add to interactive CLI, and serve fallback only for resources matching the published digest.

## Evidence
The plan has 9 phases and 68 actionable tasks. ak plan validate passed. Ownership audit found 105 exclusive entries with zero duplicates or classification errors; all local Markdown links resolve.

## Next steps
Execute with /ak:cook /home/vantt/projects/mcp-skill-hub/plans/261001-1139-curation-cli-intent-workflows/plan.md --parallel.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
