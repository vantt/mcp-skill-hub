# Issue inventory: UX surface, release CI, installation

Date: 2026-09-30. Research only; nothing below is implemented yet.
Sources (full evidence, file:line, quotes):
- UX audit: [code-reviewer-260930-1045-ux-surface-audit.md](code-reviewer-260930-1045-ux-surface-audit.md)
- Release/install research: [researcher-260930-1040-release-install-pipeline.md](researcher-260930-1040-release-install-pipeline.md)
- Earlier onboarding review (mostly fixed): [onboarding-ux-review-260930-1004-readme-first-run.md](onboarding-ux-review-260930-1004-readme-first-run.md)

Priority rule (user): easiest for the user first; then simple, understandable, few dependencies.

## A. Installation and release

| ID | P | Issue | Direction |
|---|---|---|---|
| I1 | P0 | No release/tag exists; install URL 404 | First stable tag (e.g. v0.1.0) after a manual draft run passes |
| I2 | P0 | README one-liner omits `--version`, but install.sh requires it (README.md:69, install.sh:75) | Release job stamps version into install.sh; one-liner works bare |
| I3 | P0 | Installer hard-requires cosign (install.sh:169) | SHA-256 always; cosign verify only when available (fail if it fails); `SKILLHUB_REQUIRE_SIGNATURE=1` strict mode. Needs user approval (reverses release-runbook.md:83) |
| I4 | P0 | No Windows installer; install.sh rejects Windows | `install.ps1` release asset, `irm ... \| iex`, SHA-256, user PATH via registry, re-run upgrades |
| I5 | P0 | upgrade.sh/uninstall.sh need a repo clone; re-running install refuses | Re-run = upgrade; `--uninstall` flag; delete upgrade.sh/uninstall.sh/scripts/lib |
| I6 | P1 | Only linux/amd64, darwin/arm64, windows/amd64 built; installer accepts 2 | Add linux/arm64, darwin/amd64, windows/arm64 (pure Go, one line each) |
| I7 | P1 | No PATH guidance after install | Print exact PATH line for detected shell (or edit profile — decision) |
| I8 | P1 | `latest` link ignores prereleases | First tag stable; docs note |
| I9 | P1 | No post-publish smoke of real one-liners; no Windows installer test in CI | Post-release job: Linux/macOS/Windows one-liner + upgrade-from-previous; Windows test in ci.yml |
| I10 | P1 | README vs release-runbook contradictions | Rewrite runbook to match |
| I11 | P2 | Duplicated verify steps in release.yml; Go version not pinned exactly | Refactor |
| I12 | P2 | No `skillhub self-update` | Later; re-running the one-liner is the upgrade path |
| I13 | P2 | No Homebrew/Scoop/winget; no short domain | Later |

## B. UX surface — P0 (blocks a newcomer)

| ID | Issue | Direction |
|---|---|---|
| U1 | Docs tell users to ask the agent to create/activate/list skills, but no MCP tool exists (design §17 only has `skill_update`). Agent returns "conflicts with current object state" | Decision: add MCP tools `skill_create_preview/confirm`, `skill_transition_preview/confirm`, `skill_list` (matches design 05 §1 "user need not know commands"), or make docs CLI-only |
| U2 | "Bring in skills from GitHub" imports nothing: new watched source is treated as up to date; forcing analysis on a large repo fails "source exceeded configured limits" (no limit named) yet exits 0 | Bug vs design 06:776 (initial scan). Run initial extraction on onboarding; name the limit + how to raise/scope it; non-zero exit |
| U3 | Typo in `--workspace` → "needs repair" → `doctor --fix` creates an empty hub at the typo path | Non-existent path = "not found" error with did-you-mean/`SKILLHUB_WORKSPACE`; creation only via `init` |
| U4 | Inside a connected project, `status`/`doctor`/`skill list` fail with 3 different messages; real typos hidden | Resolve workspace from the project's connection file (.mcp.json etc.) as fallback; one consistent "no workspace" message; parse errors before workspace errors |

## C. UX surface — P1 (confusing or slow)

| ID | Issue | Direction |
|---|---|---|
| U5 | Previews show digests + "Confirm these exact pins" without the command; "Base catalog" vs `--base-version`; mistyped digest reported as "stale" | Print the exact confirm command; consistent names; "digest mismatch" wording |
| U6 | connect/doctor output noisy, internal label "native-skill-instruction-coordination", WARNING even when nothing changed | Plain "Claude Code: connected (.mcp.json, CLAUDE.md, curator skill)"; warning only when writing |
| U7 | `doctor` says healthy while current project/global connection is broken | doctor also checks current project + global connection |
| U8 | Generic FIX lines; raw Go error "statat" leak; `check <unknown-id>` succeeds | Exact fix commands; wrap errors; unknown id = error |
| U9 | Activation reports missing requirements one at a time | Report all missing in one message + one combined command |
| U10 | `--content-file` rule wrong in guide (no-frontmatter accepted; only name mismatch refused); `validate` accepts mismatch; help silent | Align rule across help/guide/validate |
| U11 | After create/activate: no next step, only internal IDs; new skill body = description repeated | "Next: activate / edit content / ask your agent"; hide IDs behind --verbose; better starter template |
| U12 | `skill show` hides triggers/not-for/min-scope and file path | Show routing + path |
| U13 | Valid hand edit breaks `skill show`/`list`/resolve until rebuild | Auto-rebuild on stale when safe, or clear one-line prompt everywhere |
| U14 | `validate` doesn't print the problem; `doctor --fix` can't fix content errors, FIX loops to doctor | Print findings with file:line; point to edit/validate |
| U15 | Empty hub status says "Continue normal work"; "Review uncommitted changes" gives no command | Suggest first skill / exact git command |
| U16 | `hub_status` returns internal names like `GetCurationDiff` to the agent | Return MCP tool names / CLI commands |
| U17 | `init` on an existing project folder silently turns it into a workspace | Warn/refuse on non-empty non-workspace dir |
| U18 | Symlinked `~/.claude` error says "workspace is invalid" → doctor | Specific message + workaround |
| U19 | Guide's `resolve` example returns a clarification question with no explanation how to answer | Document/answer flow or better example |
| U20 | Absolute paths in connection files; workspace `.mcp.json` gets committed; no guidance whether to commit project connection files | Decision: gitignore guidance; document re-run `connect` after moving |
| U21 | `skillhub diff` lists draft files under "active skills" | Fix grouping |
| U22 | Some failures exit 0 | Non-zero on failure everywhere |

## D. UX surface — P2 (polish)

- U23 Jargon: canonical, generation, pins, Agent Host, substantive task.
- U24 Inconsistent terms: watched/watching/monitored; catalog/search index/Base catalog; `claude` vs `claude-code`.
- U25 Help gaps: allowed values for `--operation`, `--trust`, `--cadence`; no example for `skill`; `insight apply --proposal-file` format; thin `init` preview.
- U26 Duplicate source capture of same URL; `source show <id>` prints list header; mistyped collection silently accepted.
- U27 Connecting global + project writes the instruction block twice.
- U28 `rebuild` prints 12 progress lines + 17 row counts by default.
- U29 Host approval/trust prompts (Claude/Codex/Gemini) after connect are undocumented (unverified).

## Already good (keep)

First-run init "Next:" steps; preview-then-`--yes` everywhere; idempotent `connect` that preserves user text; ERROR/WHY/FIX layout; task-based user guide; curator safety rules; consistent MCP tool naming; strong signing/attestation in release.yml.

## Decisions needed

1. Cosign optional (I3)?
2. Agent-side skill create/activate/list MCP tools (U1)?
3. PATH on Linux/macOS: print line vs edit shell profile (I7)?
4. First-release extras: extra platforms, self-update, package managers, short domain (I6, I12, I13)?
5. Project connection files: commit or gitignore (U20)?
6. Windows install dir: `%LOCALAPPDATA%\skillhub\bin` vs `$HOME\.local\bin`.

## Decisions (user, 2026-09-30)

1. Cosign: optional. SHA-256 always; verify signature when cosign present (fail if it fails); `SKILLHUB_REQUIRE_SIGNATURE=1` strict. Update release-runbook "fails closed" wording.
2. Add agent-side MCP tools for skill create, lifecycle transitions (preview/confirm), and list.
3. PATH on Linux/macOS: installer edits shell profile automatically, supporting common shells (bash, zsh, fish, plus POSIX ~/.profile fallback); opt-out env var; idempotent.
4. First release: add linux/arm64, darwin/amd64, windows/arm64; add self-update command named `skillhub update`. Package managers and short domain: later.
5. Defaults taken (not objected): Windows dir `%LOCALAPPDATA%\skillhub\bin`; docs advise not committing project connection files (machine-specific absolute paths).
