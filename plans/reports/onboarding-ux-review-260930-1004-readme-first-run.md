# Onboarding UX review: README and first run

Date: 2026-09-30. Method: read README/docs, then built the binary and walked the new-user flow in an empty HOME (scratchpad).

## Verdict

Not ready for new users. The engine works (init → doctor → status succeed), but a newcomer cannot discover how to use it and the core promise (one hub shared by all projects) is neither delivered by the bootstrap nor documented.

## Findings (most severe first)

1. **Install path is dead.** `releases/latest/download/install.sh` returns 404; no tags exist. README's first command fails. README also pins `--version 1.0.0`, which doesn't exist.
2. **Host connection only works inside the workspace folder.** `init --yes` writes `.mcp.json`, `.codex/config.toml`, `.gemini/settings.json`, `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` into the workspace directory itself. An agent opened in the user's real project sees nothing. No user-scope registration and no `connect <project>` command; README does not explain this. Contradicts PRD problem statement ("share one curated collection across many projects").
3. **No help at all.** `skillhub`, `skillhub --help`, `skillhub help` all return "not supported" and point to `skillhub version`. No per-command help, no command list.
4. **README has no Quickstart.** `init` is not mentioned anywhere in README. The "Use" section is a flat dump of ~20 developer commands (`go run ./cmd/skillhub ...`) including internal pins (`--proposal-digest sha256:DIGEST --base-version sha256:BASE`). The intro still says "planned". Install section opens with cosign/SemVer/Sigstore detail before the command.
5. **No user guide.** `docs/` holds PRD, design docs (Vietnamese, architecture-level), release runbook. Nothing explains daily use: add a skill, bring in a skill from GitHub, how the agent picks skills, how to ask the curator ("Curate my Skill Hub").
6. **`init --yes` output is noisy and confusing.** Prints generation ID, two sha256 digests, 17 DB table row counts, and still says "Workspace directory does not exist." after success. No "what next" line.
7. **First `status` points to a non-task.** Right after init: "Git has uncommitted changes. Recommended next: Review uncommitted changes." A newcomer needs "commit, then open your agent and ask X" or "add your first skill".
8. **`--workspace` required everywhere outside the workspace.** No env var / remembered default; every README command repeats `--workspace /path/to/workspace`. (CWD-upward discovery works inside the workspace.)
9. **Guessable commands missing.** `skill list` unsupported; errors like "Review `skillhub skill list` arguments" suggest it exists.
10. **Skill creation from CLI is heavy.** 7 flags, run twice (preview then `--yes`). Fine for agents via MCP, poor for humans without docs pointing to the agent path.

## Recommended fixes (priority)

1. Publish a real tagged release (or README: build-from-source path until then).
2. Rewrite README top: 1-paragraph what/why, 5-step Quickstart (install → `init ~/skillhub --yes` → commit → connect agent → ask "curate my Skill Hub"), then links. Move install security detail, full command reference and dev commands below / to docs.
3. Add `docs/user-guide.md`: concepts in plain words, daily tasks, using from other projects, troubleshooting (`doctor`).
4. Decide and implement cross-project host connection (user-scope registration or `skillhub connect <project-dir>`), and document it.
5. Add `help` / `--help` (global + per command).
6. Trim `init` output to a summary + next step; move digests/row counts behind `--verbose`/`--json`.
7. First-run `status` next action: guide to connecting an agent / first skill.
8. `SKILLHUB_WORKSPACE` env or remembered default workspace; `skill list`.

## Unresolved questions

- Cross-project connection model: user-scope registration (every project sees hub) vs explicit per-project `connect`? Product decision.
- README language: English only, or add Vietnamese user guide?
