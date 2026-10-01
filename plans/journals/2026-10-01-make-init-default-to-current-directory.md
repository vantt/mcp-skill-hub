---
title: Make init default to current directory
date: 2026-10-01
summary: Added optional init path semantics and removed lock-file mutation from uninitialized previews.
---

# Make init default to current directory

## What happened
`skillhub init` required an explicit path. Defaulting a missing path to `.` exposed an existing preview mutation: inspecting an empty existing directory acquired a shared workspace lock and created `runtime/locks/workspace.lock`, causing the subsequent confirmed init to reject the now non-empty directory.

## Decision
`skillhub init [path]` now defaults to the current directory. Init preview inspects uninitialized targets without acquiring a workspace lock, preserving the documented read-only preview contract. Initialized workspaces retain normal locking.

## Verification
Focused CLI tests passed. A built-binary smoke run confirmed that `skillhub init` leaves an empty current directory untouched and `skillhub init --yes` then creates the workspace successfully.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
