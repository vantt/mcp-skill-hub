# Wave 5 · Worktree K round 2 prompt

Paste into the same agent/worktree (`/home/vantt/projects/mcp-skill-hub-connect-cli`).

```text
Worktree K, round 2. The lead reviewed your 8 commits (ad21087..0f3fc3d). The
runtime-only registration, the four rules, the guidance text, the wave-4/legacy
migration, the status inventory and the preview wording are accepted. Same
STEP 0 rules as round 1 (own worktree only, no live hub, no real-HOME writes,
no push).

The user decided: **remove `skillhub disconnect`**. It was not in the prompt,
main never had it, and it adds two ownership receipt files
(`.claude/skillhub-permissions.local.json`, `~/.claude/skillhub-permissions.json`)
right after wave 3 removed receipt files on purpose (06f8aab). It is also wrong:
the lead reproduced it with your binary — project connected to hub A, then
`skillhub disconnect --workspace <B> --yes` kept A's MCP entry but removed the
four rules, A's additionalDirectories, and the CLAUDE.md bootstrap block (your
advisor flagged the same).

Do, one commit per item, `make check` green before each:

1. Remove the disconnect command, its help, docs, tests, ChangePermissionReceipt,
   claudePermissionReceipt and every read/write of the two receipt files.
   Connect manages the four rules like additionalDirectories: add the ones
   missing, never duplicate, never remove or reorder user rules. No ownership
   tracking. If some code from b37a31b or 2057c0e only exists to serve
   disconnect or receipts, remove it; keep what protects connect/doctor
   (wave-4 pair conversion, not touching an edited skillhub-curation entry).
   If removing it would break a real connect/doctor guarantee, stop and
   explain instead of keeping receipts.

2. Tests must not depend on the real HOME. With the user's real HOME (which
   now has a global `connect -g`), these fail:
   - TestConnectDefaultsToCurrentDirectoryAndHonorsHostFilter
     (internal/delivery/cli/onboarding_test.go:210/244, "codex files missing:
     .../AGENTS.md") — also fails on main;
   - TestDoctorFixAppliesAllHostArtifactsAndIsIdempotent
     (internal/app/workspace_test.go:90, "host artifact CLAUDE.md: no such
     file") — fails only on your branch.
   Give every test package that can reach host integration a temp HOME and
   XDG_* (e.g. TestMain in internal/app, internal/delivery/cli,
   internal/hostintegration, internal/delivery/mcpserver; or a shared helper),
   so nothing reads ~/.claude*, ~/.codex, ~/.gemini. Also decide whether the
   second failure is a test-only issue or a real behavior change (doctor --fix
   skipping project CLAUDE.md when a global connection exists); if it is a
   behavior change, explain it and keep it only if it is correct.
   Run the real-HOME check only after the isolation is in place, and record the
   hashes first. Done means: `make check` passes BOTH with a temp HOME and with the real
   HOME, and the real HOME is unchanged afterwards: compare hashes of
   ~/.claude/settings.json, ~/.claude/skills/system-curator/SKILL.md,
   ~/.codex/config.toml, ~/.gemini/settings.json before/after, and for
   ~/.claude.json (Claude Code rewrites it constantly) compare only its
   `mcpServers` object. Read-only.

3. Bug found by the lead in a fresh-clone check:
   internal/catalog/servable.go:95 `servableSkillWarnings` sets
   `directory := item.Path[:strings.LastIndex(item.Path, "/")]`, but since
   Phase 4 item.Path is `skills/<c>/<id>/.meta/skill.yaml`, so directory is the
   `.meta` folder. Every rebuild warns "<id> will not be served:
   .../.meta/SKILL.md is missing" (false, the skill is served) and
   ValidateServableSkill never runs. Fix the directory derivation (reuse an
   existing helper if one maps a skill entity to its folder), test that a valid
   skill gives no warning and a skill with broken SKILL.md frontmatter does.

Smoke again (temp HOME): project connect, rerun no-op, wave-4 pair conversion,
`-g`; confirm no `skillhub-permissions*.json` is written anywhere and
`skillhub disconnect` is gone from help.

Report: commits, files, tests, `make check` (temp HOME and real HOME), the
before/after HOME hashes, smoke output, and what you removed.
```
