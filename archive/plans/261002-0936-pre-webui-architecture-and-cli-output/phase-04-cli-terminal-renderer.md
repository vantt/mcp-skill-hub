---
phase: 4
title: "CLI terminal renderer package"
status: completed
priority: P1
effort: "5h"
dependencies: [0]
---

# Phase 4: CLI terminal renderer package

## Goal

Create `internal/delivery/cli/termui`, a small plain-text renderer that every CLI command will use in phase 5. It handles word wrap to terminal width, aligned labels, bullet and table layout, copy-safe command lines, the ERROR/WHY/FIX block, and human-readable sizes. This phase changes **no** existing command.

## Context

Problems seen in real output (`guard/baseline/cli-before/*.human.txt`):

- `skill-review-draft.human.txt`: the description prints as one ~280-character line; `Files: 2 file(s), 1395 bytes` prints raw bytes; label alignment differs between `skill review` and `status`.
- `status-dirty.human.txt`: `Recommended next:` embeds a long git command inside a sentence, so the command cannot be copied cleanly.

Every human-output path today calls `fmt.Fprintf` directly: 13 files, 33 hand-written ERROR/WHY/FIX blocks.

Design constraints:

- `termui` is presentation only. It must not import any `github.com/vantt/mcp-skill-hub/internal/...` package. Callers pass plain strings.
- The ERROR/WHY/FIX contract in `docs/contracts/error-codes.md` requires three labeled lines in that order. Keep the exact prefixes `ERROR: `, `WHY: `, `FIX: `.
- Commands must stay copy-pasteable: a command line is never wrapped.

## Files to Create / Modify

- Create: `internal/delivery/cli/termui/termui.go`, `internal/delivery/cli/termui/wrap.go`, `internal/delivery/cli/termui/termui_test.go`
- Create: `plans/261002-0936-pre-webui-architecture-and-cli-output/reports/termui-samples.txt`
- Modify: `go.mod`, `go.sum` (add `golang.org/x/term v0.44.0` as a direct requirement in Task 4.2)

Do not modify any file under `internal/delivery/cli/` outside `termui/`.

## Required API (implement exactly; doc comment on every exported identifier)

```go
package termui

const (
    DefaultWidth = 100 // used when the writer is not a terminal
    MinWidth     = 60
    MaxWidth     = 120
)

// Printer writes plain, wrapped text. It records the first write error and ignores later writes.
type Printer struct { /* unexported fields */ }

// New detects the width: when w is an *os.File and term.IsTerminal(fd), use term.GetSize width
// clamped to [MinWidth, MaxWidth]; otherwise DefaultWidth.
func New(w io.Writer) *Printer
// NewWithWidth is for tests and callers that already know the width; width is clamped the same way.
func NewWithWidth(w io.Writer, width int) *Printer
func (p *Printer) Width() int
func (p *Printer) Err() error

// Line writes one wrapped paragraph. Embedded "\n" starts a new paragraph line.
func (p *Printer) Line(text string)
// Blank writes one empty line; consecutive Blank calls and a Blank before any output write nothing.
func (p *Printer) Blank()
// Heading writes Blank then the title on its own line.
func (p *Printer) Heading(title string)

// Field is one label/value row. Rows with an empty Value are skipped.
type Field struct{ Label, Value string }
// Fields writes rows as "<Label>:" padded to the longest label in this call plus two spaces,
// then the value wrapped with a hanging indent aligned to the value column.
func (p *Printer) Fields(fields ...Field)
// Bullets writes "  - item", wrapped with a 4-space hanging indent.
func (p *Printer) Bullets(items ...string)
// Table pads columns with two spaces between them; only the last column wraps,
// with a hanging indent at its start column. Empty rows: nothing is written.
func (p *Printer) Table(headers []string, rows [][]string)
// Command writes "  $ " + command and never wraps it.
func (p *Printer) Command(command string)
// Next writes "Next: " + label (wrapped), then Command(command) when command != "".
func (p *Printer) Next(label, command string)
// Warning writes "WARNING: " + text wrapped with a hanging indent of len("WARNING: ").
func (p *Printer) Warning(text string)
// Error writes exactly three labeled lines, in order: "ERROR: what", "WHY: why", "FIX: fix",
// each wrapped with a hanging indent equal to its own label width. Empty why/fix still print their label.
func (p *Printer) Error(what, why, fix string)
// Raw writes text verbatim (diffs, file contents); it is never wrapped.
func (p *Printer) Raw(text string)

// Wrap splits text into lines of at most width runes at spaces. Unbreakable tokens:
//   - a single word longer than width (URL, path, digest) stays whole on its own line;
//   - a span enclosed in backticks (`skillhub skill edit x --trigger "y"`) is never split,
//     so commands quoted inside prose (for example in FIX text) stay copy-pasteable.
// Leading/trailing spaces are trimmed per line.
func Wrap(text string, width int) []string
// Bytes renders a size with base 1024 and one decimal: 0 B, 512 B, 1.0 KB, 1.4 KB, 183.2 KB, 4.0 MB, 1.0 GB.
func Bytes(n int64) string
// Plural renders "1 skill" / "3 skills" style counts.
func Plural(n int, singular, plural string) string
```

Width is measured in runes (`utf8.RuneCountInString`).

## Tasks

### Task 4.1 — Implement the package
- Target: `termui.go` (Printer and its methods), `wrap.go` (`Wrap`, `Bytes`, `Plural`).
- Steps: implement the Required API exactly. No color codes, no ANSI escapes, no global state.
- Verify: `grep -n 'mcp-skill-hub/internal' internal/delivery/cli/termui/*.go` prints nothing.

### Task 4.2 — Add the dependency (after the code that imports it exists)
- Steps:
  1. Run `go get golang.org/x/term@v0.44.0` (already in the local module cache).
  2. Run `go mod tidy`.
- Verify: `go build ./internal/delivery/cli/termui/` exits 0, and `grep -E '^\s*golang\.org/x/term v0\.44\.0$' go.mod` prints one line (a direct requirement, without `// indirect`).

### Task 4.3 — Tests (names checked by the guard; one owner test per contract)
- Target: `internal/delivery/cli/termui/termui_test.go`. Start every test with `t.Parallel()` (except `TestWriteSamples`, which is environment-driven). Use `bytes.Buffer` and `NewWithWidth`. Compare full expected strings, not substrings, except where noted.
- Tests:
  1. `TestWrapKeepsLongTokensIntact`: `Wrap("see https://example.com/a/very/long/path/that/exceeds/the/width now", 20)` keeps the URL whole on its own line.
  2. `TestWrapKeepsBacktickSpansIntact`: a 120-character FIX text containing a backtick-quoted 70-character command, rendered by `Error` at width 60, keeps the whole backtick span on one line.
  3. `TestFieldsAlignLabels`: labels `ID`, `Collection`, `State`, `Description` produce values starting at the same column; a 200-character `Description` wraps at width 60 with every continuation line indented to that column; an empty-value field is skipped.
  4. `TestErrorBlockOrder`: output starts with `ERROR: `, then `WHY: `, then `FIX: `, in order; a long WHY wraps with a continuation indent of 5 spaces.
  5. `TestCommandLinesAreNeverWrapped`: a 300-character command through `Command` and through `Next` stays on one line prefixed `  $ `.
  6. `TestBytesHumanized`: table for 0, 512, 1024, 1395, 187620, 4*1024*1024, 1<<30 → `0 B`, `512 B`, `1.0 KB`, `1.4 KB`, `183.2 KB`, `4.0 MB`, `1.0 GB`.
  7. `TestTableAlignsColumns`: a 3-column table aligns all columns; a long last-column value wraps under its own column start.
  8. `TestWidthClamping`: `New(&bytes.Buffer{}).Width() == DefaultWidth`; `NewWithWidth(w, 10).Width() == MinWidth`; `NewWithWidth(w, 500).Width() == MaxWidth`.
  Line-width safety across real commands is owned by `TestHumanOutputFitsWidth` in phase 5; do not add a separate width test here.
- Verify: `go test -count=1 -v ./internal/delivery/cli/termui/` exits 0 and prints `--- PASS:` for all eight names.

### Task 4.4 — Samples for human approval
- Goal: show the user what the CLI will look like before migrating every command.
- Steps:
  1. Add `TestWriteSamples` to `termui_test.go`. It does nothing unless the environment variable `TERMUI_SAMPLES_OUT` is set. When set, it writes to that path these documents, rendered at width 100 and again at width 60 and separated by `===== width N =====` headers:
     - (a) a `skill review` page re-creating `guard/baseline/cli-before/skill-review-draft.human.txt` with the same data: `Fields`, `Heading("Activation readiness")`, `Bullets`, `Next`, and the size via `Bytes(1395)`;
     - (b) a `status` page re-creating `status-dirty.human.txt`, where the recommended git command goes through `Next(label, command)`;
     - (c) the activation error from `skill-activate-missing-fields.human.txt` through `Error`, with the command in FIX written inside backticks;
     - (d) a `skill list` table with three rows, one of which has a 90-character name.

     The output must be deterministic. Environment checks are allowed only in this test file.
  2. Run `TERMUI_SAMPLES_OUT=$PWD/plans/261002-0936-pre-webui-architecture-and-cli-output/reports/termui-samples.txt go test -count=1 -run '^TestWriteSamples$' ./internal/delivery/cli/termui/`.
- Verify: `test -s plans/261002-0936-pre-webui-architecture-and-cli-output/reports/termui-samples.txt` exits 0. The guard re-renders the samples and requires byte equality, so never hand-edit this file.

### Task 4.5 — Guard, report, and HUMAN GATE
- Steps:
  1. Run `bash plans/261002-0936-pre-webui-architecture-and-cli-output/guard/guard.sh check 4`.
  2. Write `reports/phase-04-report.md` with the guard output and the full contents of `reports/termui-samples.txt` in a fenced block. Commit both.
  3. **STOP and ask the user to approve the samples.** The user records approval with `sha256sum <plan>/reports/termui-samples.txt | cut -d' ' -f1 > <plan>/reports/approvals/phase-04.approved`. Never create or edit files in `reports/approvals/` yourself. If the user requests layout changes, apply them in `termui` only, re-run Tasks 4.3–4.5, and ask again.
- Verify: exit code 0 and the last line is exactly `GUARD RESULT: PASS (phase 4)`.

## Failure Protocol
If any Verify step does not meet its stated pass condition, STOP this phase.
Do not improvise a fix, retry blindly, or reason around the failure.
Spawn the `kongming` subagent for next-step counsel and pass:
- the phase and task id,
- what you attempted (the steps you ran),
- the exact command and its full output,
- the pass condition it failed to meet.
Apply kongming's guidance, then re-run the Verify step.
If `kongming` cannot be spawned in this environment, STOP and report the same
failure evidence to the user. Never continue by self-reasoning.
