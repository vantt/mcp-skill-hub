---
phase: 6
title: "Guided skill create (optional, later)"
status: pending
priority: P3
effort: "0.5d"
dependencies: [1]
---

# Phase 6: Guided skill create (optional, later)

## Context

`skillhub skill create <id> --collection <c> --name <n> --description <d> [...] --yes` needs many flags
(`internal/delivery/cli/help.go:123-126`); status suggests a long template line for an empty hub
(`internal/delivery/cli/curation.go:75`). Decision 9: the README stops at "curate my Skill Hub"; a guided create in a
terminal is a later, optional improvement. Flags remain for scripts and agents. This phase is not required for the
onboarding acceptance criteria.

## Requirements

- `skillhub skill create` with stdin and stdout both terminals and no `--name`/`--description` asks, one question at a
  time: skill id (suggested from the name, kebab-case, validated against existing ids), name, description, when to
  use it (becomes `--trigger`, one or more lines), when not to use it (`--not-for`).
- Defaults for everything else: collection `core` (or the only existing collection), `--min-scope single_step`,
  operation `other`, the existing starter content template (`TestSkillCreateStarterTemplateAndNextStep`,
  `internal/delivery/cli/ux_behavior_test.go:257`).
- Shows the summary and creates the draft directly (the interactive answers are the confirmation; `--yes` stays for the
  flag form, P3 in plan.md), then prints the file to edit and `skillhub skill activate <id> --yes` as the next command.
- Not a terminal and missing required flags: today's error naming the missing flags (no prompt, ever).
- Ctrl-C or empty input on a required question: nothing written, exit 130 or 2.
- Status's empty-hub next command becomes `skillhub skill create`.

## Files

- Modify: `internal/delivery/cli/skill.go` (create path), new `internal/delivery/cli/skill_create_prompt.go` (prompt
  loop with injectable reader/writer and terminal detector, like `stdoutIsTerminal` in `token.go:18`)
- Modify: `internal/delivery/cli/help.go` (skill usage), `internal/delivery/cli/curation.go` and
  `internal/app/curation_home.go` (next command)
- Modify: `docs/curating-skills.md` ("Create your first skill" shows the guided form first)
- Tests: new `skill_create_prompt_test.go`; update `ux_behavior_test.go`, `onboarding_test.go` status next-step tests

## Steps

1. Prompt loop with validation, using `app` validation for the id and routing fields so rules live in one place.
2. Map answers onto the existing create request; reuse the flag path for writing.
3. Next-command and docs updates.

## Validation

- Scripted terminal tests (injected reader and terminal flag) cover happy path, invalid id retry, duplicate id, empty
  required answer, Ctrl-C; the written skill equals the one the flag form writes for the same values.
- Non-terminal run without flags prints the missing-flag error and writes nothing.
- `go test ./internal/delivery/cli/...`.

## Risk

| Risk | L x I | Mitigation |
|---|---|---|
| Agents trigger the prompt and hang | Low x High | Prompt only when stdin and stdout are terminals; agents' shells are not |
| Two validation paths drift | Medium x Medium | Prompt calls the same `app` validators as the flag path |

## Rollback

Revert the phase commit; the flag form is unchanged.
