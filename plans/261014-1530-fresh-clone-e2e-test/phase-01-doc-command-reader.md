# Phase 1: Doc command reader

## Context

Plan: [plan.md](plan.md). The end-to-end test must run the commands the docs show, not copies of them, so a doc
and the test cannot drift apart. `internal/delivery/cli/docs_commands_test.go` already checks that every documented
`skillhub` command exists in the CLI help (`TestDocsCommandsExist`), but its helpers (`commandSegments`, `repoRoot`)
are test code in package `cli` and cannot be imported from `mcpserver`. This phase adds a small reader in the
`mcpserver` test package. It is not shared or exported (AGENTS.md: no test-only exports).

## Requirements

- `docSectionCommands(t, docPath, section string) []string` returns the command lines of every fenced code block
  whose info string is `bash` inside one section of a repository doc:
  - `section` matches a Markdown heading line exactly (for example `## Move to another machine`); the section ends
    at the next heading of the same or a higher level, or at the end of the file.
  - `section` may instead be a `<summary>...</summary>` line; that section ends at the next `</details>`.
  - Blank lines and lines starting with `#` inside a block are skipped; every other line is one command line,
    returned verbatim (no trimming of `&&` chains).
  - The doc path is relative to the repository root (`filepath.Join("..", "..", "..")`, as `buildSkillHub` uses).
  - Fails the test (`t.Fatalf`) with the doc path and section when the file, the section or any `bash` block in
    it is missing.
- `fillDocPlaceholders(t, line string, values map[string]string) string` replaces each `<name>` from `values` with
  the shell-quoted value and fails when any `<...>` token is left, naming the line. Quoting uses single quotes with
  `'` escaped, enough for temp paths.
- The test that uses them pins the expected number of lines per section (`len(lines) == want`), so a doc that
  gains or loses a step fails until the test's section table is updated on purpose.

## Files

- Create `internal/delivery/mcpserver/doc_commands_test.go` (reader, placeholder filler, and their own small test).
- Read only: `README.md`, `docs/user-guide.md`, `docs/troubleshooting.md`,
  `internal/delivery/cli/docs_commands_test.go` (style reference).

## Steps

1. Write `docSectionCommands` and `fillDocPlaceholders` as above (about 50 lines).
2. Add `TestDocSectionCommandsReadsTheDocumentedMove`, a table over the four sections the end-to-end test uses,
   asserting the exact lines read today:
   - `docs/user-guide.md`, `## Move to another machine` → `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init <git-url>`,
     `cd <your-project> && skillhub connect`
   - `README.md`, `<summary>Use the hub on a second machine</summary>` → the same `init` line
   - `docs/troubleshooting.md`, `### Fresh clone on a new machine` → `cd ~/skill-hub && skillhub init`
   - `README.md`, `### Check` → `cd <your-project> && skillhub status`

   Decision for the implementer: if pinning exact text duplicates the end-to-end test's own checks, keep only the
   line counts here and let the end-to-end test own the behaviour. Do not keep both an exact-text table and an
   exact-text assertion in phase 2.
3. Add one case per failure branch that matters: an unknown placeholder fails, a missing heading fails, a section
   without a `bash` block fails. Keep the parsing in plain functions that return an error
   (`parseSectionCommands(content, section string) ([]string, error)`,
   `fillPlaceholders(line string, values map[string]string) (string, error)`) and let the `t`-taking wrappers call
   `t.Fatalf`, so the failure branches are tested on in-memory strings without a fake `testing.T`.

## Validation (narrowest first)

```bash
go test -count=1 -run '^TestDocSectionCommands' ./internal/delivery/mcpserver/
go vet ./internal/delivery/mcpserver/
gofmt -l internal/delivery/mcpserver/
```

## Risks

- A doc section gains a second `bash` block with a non-command (for example sample output marked `bash`): the
  pinned line count fails loudly; fix the doc's fence (`text`) rather than teaching the reader to skip it.
- `README.md` `### Check` is followed by a `text` block of sample output; only `bash` blocks are read, so it is
  ignored by design.

## Rollback

Delete `internal/delivery/mcpserver/doc_commands_test.go`. No production code changes.
