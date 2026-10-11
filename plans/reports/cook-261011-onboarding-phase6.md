# Guided skill create report

Status: DONE

- `skillhub skill create` with no `--name`/`--description`, stdin and stdout both terminals, asks: name, id (suggested from the name, validated through the same create preview as the flag form, duplicate check), description, when to use (one or more lines), when not to use (one or more lines). The name is asked before the id so the id can be suggested from it.
- Defaults: collection core (or the only existing collection), min-scope single_step, operation other, starter template. The answers confirm; a summary prints, the draft is saved, then the file to edit and `skillhub skill activate <id> --yes` print.
- Not a terminal: the previous missing-flags error, nothing prompted or written. Empty required answer exits 2, Ctrl-C or end of input exits 130, nothing written. Flag form and `--json` unchanged.
- Status next command for an empty hub is `skillhub skill create` (change is in `internal/app/health.go`, where the next step lives).
- Updated help, README status sample, docs/curating-skills.md (guided form first), docs/command-reference.md.
- Tests: `skill_create_prompt_test.go` (parity with flag form, retries, empty answers, EOF/interrupt, non-terminal), status tests updated. Lint, vet (linux/windows/darwin) and full test suite pass.
