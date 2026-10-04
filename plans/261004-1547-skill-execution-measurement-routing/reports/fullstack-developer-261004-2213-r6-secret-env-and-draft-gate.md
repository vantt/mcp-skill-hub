# R6 per-skill secret env and draft content gate: report

## Secret env
- Store and parsing live in `internal/app/skill_env.go` (`SkillEnvService` Set/Unset/List, `loadSkillConfigEnv`, `redactValues`). File `runtime/config/<id>/env`: 0600 file, 0700 directories, temp file plus rename, Lstat refusal of symlinked directories and files, 1 MiB cap. Values with shell-special characters are single-quoted so `. env` cannot run code; the parser reverses that. A malformed file errors with the line number only.
- CLI: `internal/delivery/cli/skill_env.go`. Value from a TTY via `term.ReadPassword` (prompt on stderr) or stdin when piped (one trailing newline dropped, 64 KiB cap). Extra positional arguments are refused, so a value cannot be passed on the command line. `x/term` was already in go.mod.
- `SKILLHUB_CONFIG_DIR` added to `local.env` and `preflight.env` for trusted skills only (`skillExportEnv`); untrusted responses carry no env or config path.
- Doctor: stored keys satisfy env presence (values never copied into checks), are added to the `check` environment, and are redacted from `check_output`. A broken env file is a structured invalid-request error. Missing env yields a suggested action `skillhub skill env set <id> <KEY>`.
- Host paragraph, `skill_get` description, preflight instruction, CLI help updated; AGENTS.md, CLAUDE.md, GEMINI.md refreshed through `updateBootstrap`.

## Draft gate
`SkillService.ContentTrustFor` evaluates trust from canonical files for any lifecycle state. `skill_get` uses it for non-active skills: an unapproved third-party skill returns no `content` and `local {status: review_required, reason_codes, review_command}` with no paths or env. Active skills keep the snapshot-based gate. `skills/get`, `skills/list` and `resources/read` already serve active skills only.

## Tests
- app: set/list/unset, permissions, quoting round trip, invalid input, symlink refusal, malformed file, config dir only for trusted skills, doctor presence/redaction/cache/telemetry sentinel walk.
- cli: set/list/unset/doctor flow with sentinel check over all outputs and every workspace file except the env file; value never an argument.
- mcpserver: draft, deprecated and archived third-party skills withhold content; approved and local drafts serve it; sentinel absent from skill_get, skills/list, resource reads, skill_resolve.
