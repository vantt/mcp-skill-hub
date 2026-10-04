# Runtime hardening R2-R5: report

`go vet ./...` is clean. Tests pass for app, skillruntime, delivery/*, systemskills, schemas, telemetry. hostintegration bootstrap tests pass; the rest of that package was mid-change for R1 and not mine. `make check` not run (controller). Nothing committed.

## R2 unapproved content
- `skill_get` (MCP): `content` is omitted (shadowing `omitempty` field in `skillGetResult`) when `local.status` is `review_required`; id, name, description, routing, lifecycle and `local {status, reason_codes, review_command}` stay. The web and CLI read model (`GetSkillDetail`) is unchanged.
- `resources/read` refuses every resource including SKILL.md with `content_review_required`.
- `skills/get` already returned only URI, frontmatter and resource list (no body), so it is unchanged.
- Tool descriptions (skill_get, skill_resolve), the CLI review text and the host paragraph now say "do not use the skill; tell the user to run `skillhub skill review <id>`". AGENTS.md/CLAUDE.md/GEMINI.md refreshed from the new block.
- No published JSON schema describes `skill_get` output, so none changed.

## R3 review diff
- New `internal/app/skill_review_changes.go`, using the existing `offlineGitCommand`. `content_trust.changes_since_approval` has found, commit, diff_command, added/removed/modified, runtime_changed, scripts_changed, dependencies_changed, history_truncated. It is absent when never approved, when currently approved, or when Git is unavailable.
- Baseline: the oldest commit of the unbroken run of manifest commits carrying the recorded digest (not the newest as the phase text says), so later manifest-only edits cannot hide earlier content changes. Walk is capped at 200 commits; if the cap cuts the run, nothing is claimed (`found: false`, `history_truncated: true`).
- File comparison uses Git blob ids (`ls-tree` vs `hash-object --stdin-paths`). scripts_changed: path under `scripts/` or an interpreter by extension or shebang (`skillruntime.InterpreterOf`, new export). dependencies_changed: `skillruntime.IsDependencyManifest`.
- CLI prints the summary above the approve command ("Never approved: review the full skill." when there is no approval; a distinct line when the approved content cannot be found).

## R4
`Hints.missing_lockfiles` (`"<ecosystem>: <path>"`) for npm, python (pyproject without uv/poetry lock; requirements*.txt with any line lacking `==`), go (go.mod with requires and no go.sum), cargo, ruby. Shown in CLI review. Curator SKILL.md (both copies, identical) now prefers pinned, lockfile-based installs.

## R5
Bootstrap paragraph (and `skill_get`/preflight instruction text): no concurrent `setup` (`.setup.lock`), POSIX `export` / PowerShell `$env:`, call `skill_get` again if `local.path` is gone. Docs-note bullets added to the phase file.

## Tests
mcpserver: unapproved `skill_get` and SKILL.md refusal, release after approval. app: never approved, approved, script+dependency changes with baseline pinned, runtime-only, doc-only, bounded history. cli: never-approved text, change summary order. skillruntime: lockfile cases, `InterpreterOf`. hostintegration: bootstrap text. Web golden regenerated (additive).

Status: DONE_WITH_CONCERNS
Summary: R2-R5 implemented with tests; vet and focused tests pass.
Concerns/Blockers: (1) A draft third-party skill still returns `content` from `skill_get` because trust is evaluated only for active skills; say if drafts should be gated too. (2) Baseline choice (oldest commit of the digest run) differs from the phase wording; chosen to avoid hiding changes. (3) hostintegration full-package tests were failing from R1 work in progress at my last run.
