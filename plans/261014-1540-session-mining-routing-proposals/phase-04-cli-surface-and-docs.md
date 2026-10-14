# Phase 4: CLI surface and docs

## Context

Plan: [plan.md](plan.md), section "Where a curator sees it". The human surface is the CLI, which is already the
only place case text is shown. This phase extends `skillhub telemetry cases` and `skillhub skill edit`. It adds no
command group, no MCP tool and no web route.

## Requirements

### `skillhub telemetry cases suggest [--since <YYYY-MM-DD|Nd>] [--skill <skill-id>] [--min-events <n>] [--workspace <path>] [--json]`

- Default `--since 30d`. `--min-events` is an integer from 1 to 100.
- Human output groups suggestions by skill. Per suggestion it prints:
  - the line `<id>  <field>  "<text>"`, with the text printed through `strconv.Quote`-style quoting so it stays one
    line and shows any residue literally;
  - `Why: <reason>`;
  - `Evidence: <events> event(s) in <sessions> session(s), <first_seen>..<last_seen>; <kinds>; other skill <id>`;
  - `Next: skillhub skill edit <skill-id> --from-suggestion <id>`, which contains no untrusted text, so it is safe
    to copy.
- After the list it prints the skipped counts per reason (one line), and when `raw_events_since` limits the Y-side
  evidence, one line saying so.
- If the case journal is disabled, it prints: "The case journal is off, so there is no task text to learn from. Run
  `skillhub telemetry cases enable`; only sessions after that count." It exits 0.
- `--json` writes the `RoutingSuggestionReport` in the usual result envelope (`writeResult`/`writeJSON`
  conventions of `telemetry_cases.go`).
- Invalid flags follow the existing `writeInvalidRequest` pattern with a fix line.

### `skillhub telemetry cases dismiss <suggestion-id> [--workspace <path>] [--json]`

- It validates the ID, calls `Dismiss`, and prints "Suggestion <id> dismissed; it will not be suggested again on
  this machine." It exits 2 on a malformed ID.

### `skillhub skill edit <skill-id> --from-suggestion <id> [--yes] [--json] [--workspace <path>]`

- It is mutually exclusive with content, editor, runtime and routing flags (`--example`, `--counter-example`,
  `--not-for`, `--trigger`, `--operation`, `--min-scope`, `--description`, `--name`, `--rationale`,
  `--approve-content`). Combining them is an `invalid_request` with a fix line.
- The `<skill-id>` must equal the suggestion's skill. A mismatch is an `invalid_request`.
- Before the normal preview block (`writeSkillPreview`), it prints `Suggestion`, `Why`, `Evidence`, and the
  routing check (`Routing impact` summary and warnings, which the existing preview rendering already prints).
- Without `--yes`, it ends with the existing "apply it later" line, `skillhub skill confirm <proposal-id>`. With
  `--yes`, it applies through the existing confirm path, exactly like `skill edit --example ... --yes`.

### Help and docs

- `internal/delivery/cli/help.go`: the `telemetry` usage gains `cases suggest` and `cases dismiss`, and the `skill`
  usage gains `--from-suggestion`.
- `docs/command-reference.md`, under "Work with skills": add the row "Add routing entries that usage suggests |
  `skillhub telemetry cases suggest`, then `skillhub skill edit <skill-id> --from-suggestion <id>`". Under the
  telemetry rows, add the dismiss row.
- `docs/curating-skills.md`: add a short subsection "Improve routing from usage" after "Measure usage". It explains
  what is mined (the three signals in one sentence each), that the case journal must be on, that nothing changes
  until you confirm, that phrasings with redacted parts are never offered, and that the check is approximate, so
  you should run `skillhub eval routing` after confirming. Follow the explanatory style (why, what to look at, one
  example).
- `docs/design/04-telemetry-reproducibility-evaluation.md` §7.4: add one paragraph saying that the routing
  suggestion pipeline follows the same rule (no automatic promotion, a human confirms each diff) and naming its
  inputs. If the lead accepts open question 7, also correct the raw retention to the code default.

## Files

| Path | Action | Change |
|---|---|---|
| `internal/delivery/cli/telemetry_cases.go` | modify | `suggest` and `dismiss` subcommands and their flag parsing (keep functions within 120 lines; split the printer into its own function) |
| `internal/delivery/cli/skill.go` | modify | The `--from-suggestion` flag, exclusivity, and the `edit` branch calling `PreviewRoutingSuggestion` |
| `internal/delivery/cli/help.go` | modify | Usage text |
| `internal/delivery/cli/telemetry_cases_test.go` | modify | Extend `TestTelemetryCLICases` |
| `internal/delivery/cli/skill_test.go` | modify | One table row group for `--from-suggestion` |
| `docs/command-reference.md`, `docs/curating-skills.md`, `docs/design/04-telemetry-reproducibility-evaluation.md` | modify | As above |

## Steps

1. Add the subcommands and the printer.
2. Add `--from-suggestion` to the skill flags (the flag list in `skill.go` around line 331) and to the `edit` branch.
3. Update the help text, then the docs.
4. Extend the tests.

## Validation

```bash
go test ./internal/delivery/cli/ -run 'TestTelemetryCLICases|TestDocsCommandsExist' -count=1
go test ./internal/delivery/cli/ -run 'TestSkillCLI' -count=1
go test ./internal/delivery/cli/ ./internal/delivery/web/ -count=1
make check
```

Test contracts (extend the existing owners, no new near-duplicates):

- `suggest` on a seeded temp workspace prints the quoted text, the reason, the evidence and the `Next:` line. A
  phrasing holding `\x1b[31m` is never printed raw: it is skipped as `unsafe_characters`, and the skipped line
  counts it. `--json` output parses and holds `suggestions[0].evidence.events`.
- `suggest` with the case journal disabled prints the enable hint and exits 0.
- `dismiss` followed by `suggest` hides the ID. A malformed ID exits 2.
- `skill edit <id> --from-suggestion <sid>` without `--yes` prints the pins and the confirm line, and leaves the
  skill files unchanged. `skill confirm <proposal-id>` applies it. Combining it with `--example` is refused.
- `TestDocsCommandsExist` covers the new documented commands.
- `TestCasesIsolationWebAndExport` stays green.

Manual check, which the lead may run in a temp hub and never in the live hub: `skillhub init` in a temp dir with an
isolated HOME and XDG dirs, then `telemetry cases enable`, a scripted MCP session (`skill_resolve` → `no_skill` →
`skill_get X`), then `telemetry cases suggest`, `skill edit X --from-suggestion <id>`, and `skill confirm`.

## Risks

- **Copy-paste of untrusted text into a shell.** The `Next:` line holds only IDs. The text is shown quoted and is
  never part of a suggested command.
- **CLI output width tests** (`human_output_width_test.go`) may flag long evidence lines. Wrap them through
  `termui` as other commands do.

## Rollback

Revert the commit. The phase 2 and 3 app code stays unused but harmless, or revert those commits too, in reverse
order.
