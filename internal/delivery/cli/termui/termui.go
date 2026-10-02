package termui

import (
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	DefaultWidth = 100 // used when the writer is not a terminal
	MinWidth     = 60
	MaxWidth     = 120
)

// Printer writes plain, wrapped text. It records the first write error and ignores later writes.
type Printer struct {
	w            io.Writer
	width        int
	err          error
	hasWritten   bool
	pendingBlank bool
	lastWasBlank bool
}

// New detects the width: when w is an *os.File and term.IsTerminal(fd), use term.GetSize width
// clamped to [MinWidth, MaxWidth]; otherwise DefaultWidth.
func New(w io.Writer) *Printer {
	width := DefaultWidth
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if termWidth, _, err := term.GetSize(int(f.Fd())); err == nil {
			width = clampWidth(termWidth)
		}
	}
	return &Printer{
		w:     w,
		width: width,
	}
}

// NewWithWidth is for tests and callers that already know the width; width is clamped the same way.
func NewWithWidth(w io.Writer, width int) *Printer {
	return &Printer{
		w:     w,
		width: clampWidth(width),
	}
}

func clampWidth(width int) int {
	if width < MinWidth {
		return MinWidth
	}
	if width > MaxWidth {
		return MaxWidth
	}
	return width
}

// Width returns the effective clamped width used for wrapping.
func (p *Printer) Width() int {
	return p.width
}

// Err returns the first write error encountered, if any.
func (p *Printer) Err() error {
	return p.err
}

func (p *Printer) write(s string) {
	if p.err != nil || s == "" {
		return
	}
	if p.pendingBlank {
		if _, err := io.WriteString(p.w, "\n"); err != nil {
			p.err = err
			return
		}
		p.pendingBlank = false
	}
	if _, err := io.WriteString(p.w, s); err != nil {
		p.err = err
		return
	}
	p.hasWritten = true
	p.lastWasBlank = strings.HasSuffix(s, "\n\n")
}

// Line writes one wrapped paragraph. Embedded "\n" starts a new paragraph line.
func (p *Printer) Line(text string) {
	lines := Wrap(text, p.width)
	for _, l := range lines {
		p.write(l + "\n")
	}
}

// Blank writes one empty line; consecutive Blank calls and a Blank before any output write nothing.
func (p *Printer) Blank() {
	if p.hasWritten && !p.lastWasBlank {
		p.pendingBlank = true
		p.lastWasBlank = true
	}
}

// Heading writes Blank then the title on its own line.
func (p *Printer) Heading(title string) {
	p.Blank()
	p.write(title + "\n")
}

// Field is one label/value row. Rows with an empty Value are skipped.
type Field struct {
	Label string
	Value string
}

// Fields writes rows as "<Label>:" padded to the longest label in this call plus two spaces,
// then the value wrapped with a hanging indent aligned to the value column.
func (p *Printer) Fields(fields ...Field) {
	var active []Field
	maxLabelLen := 0
	for _, f := range fields {
		if f.Value == "" {
			continue
		}
		active = append(active, f)
		lLen := utf8.RuneCountInString(f.Label)
		if lLen > maxLabelLen {
			maxLabelLen = lLen
		}
	}
	if len(active) == 0 {
		return
	}

	valueCol := maxLabelLen + 3 // len(Label) + len(":") + 2 spaces
	indent := strings.Repeat(" ", valueCol)
	availWidth := p.width - valueCol
	if availWidth < 10 {
		availWidth = MinWidth
	}

	for _, f := range active {
		labelStr := f.Label + ":"
		padLen := valueCol - utf8.RuneCountInString(labelStr)
		if padLen < 0 {
			padLen = 1
		}
		prefix := labelStr + strings.Repeat(" ", padLen)

		lines := Wrap(f.Value, availWidth)
		if len(lines) == 0 {
			p.write(prefix + "\n")
			continue
		}
		p.write(prefix + lines[0] + "\n")
		for _, l := range lines[1:] {
			p.write(indent + l + "\n")
		}
	}
}

// Bullets writes "  - item", wrapped with a 4-space hanging indent.
func (p *Printer) Bullets(items ...string) {
	availWidth := p.width - 4
	if availWidth < 10 {
		availWidth = MinWidth
	}
	indent := "    "
	for _, item := range items {
		lines := Wrap(item, availWidth)
		if len(lines) == 0 {
			continue
		}
		p.write("  - " + lines[0] + "\n")
		for _, l := range lines[1:] {
			p.write(indent + l + "\n")
		}
	}
}

// Table pads columns with two spaces between them; only the last column wraps,
// with a hanging indent at its start column. Empty rows: nothing is written.
func (p *Printer) Table(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	numCols := len(headers)
	if numCols == 0 {
		return
	}

	colWidths := make([]int, numCols)
	for i, h := range headers {
		colWidths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i := 0; i < len(row) && i < numCols; i++ {
			rLen := utf8.RuneCountInString(row[i])
			if rLen > colWidths[i] {
				colWidths[i] = rLen
			}
		}
	}

	lastColStart := 0
	for i := range numCols - 1 {
		lastColStart += colWidths[i] + 2
	}

	var headerLine strings.Builder
	for i, h := range headers {
		if i == numCols-1 {
			headerLine.WriteString(h)
		} else {
			headerLine.WriteString(h)
			pad := colWidths[i] + 2 - utf8.RuneCountInString(h)
			if pad > 0 {
				headerLine.WriteString(strings.Repeat(" ", pad))
			}
		}
	}
	p.write(headerLine.String() + "\n")

	lastAvail := p.width - lastColStart
	if lastAvail < 10 {
		lastAvail = MinWidth
	}
	rowIndent := strings.Repeat(" ", lastColStart)

	for _, row := range rows {
		var rowPrefix strings.Builder
		for i := range numCols - 1 {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			rowPrefix.WriteString(val)
			pad := colWidths[i] + 2 - utf8.RuneCountInString(val)
			if pad > 0 {
				rowPrefix.WriteString(strings.Repeat(" ", pad))
			}
		}

		lastVal := ""
		if numCols-1 < len(row) {
			lastVal = row[numCols-1]
		}

		lines := Wrap(lastVal, lastAvail)
		if len(lines) == 0 {
			p.write(strings.TrimRight(rowPrefix.String(), " ") + "\n")
			continue
		}
		p.write(rowPrefix.String() + lines[0] + "\n")
		for _, l := range lines[1:] {
			p.write(rowIndent + l + "\n")
		}
	}
}

// Command writes "  $ " + command and never wraps it.
func (p *Printer) Command(command string) {
	p.write("  $ " + command + "\n")
}

// Next writes "Next: " + label (wrapped), then Command(command) when command != "".
func (p *Printer) Next(label, command string) {
	availWidth := p.width - 6
	if availWidth < 10 {
		availWidth = MinWidth
	}
	indent := "      " // 6 spaces
	lines := Wrap(label, availWidth)
	if len(lines) > 0 {
		p.write("Next: " + lines[0] + "\n")
		for _, l := range lines[1:] {
			p.write(indent + l + "\n")
		}
	} else {
		p.write("Next:\n")
	}
	if command != "" {
		p.Command(command)
	}
}

// Warning writes "WARNING: " + text wrapped with a hanging indent of len("WARNING: ").
func (p *Printer) Warning(text string) {
	availWidth := p.width - 9
	if availWidth < 10 {
		availWidth = MinWidth
	}
	indent := "         " // 9 spaces
	lines := Wrap(text, availWidth)
	if len(lines) > 0 {
		p.write("WARNING: " + lines[0] + "\n")
		for _, l := range lines[1:] {
			p.write(indent + l + "\n")
		}
	} else {
		p.write("WARNING:\n")
	}
}

// Error writes exactly three labeled lines, in order: "ERROR: what", "WHY: why", "FIX: fix",
// each wrapped with a hanging indent equal to its own label width. Empty why/fix still print their label.
func (p *Printer) Error(what, why, fix string) {
	errAvail := p.width - 7
	if errAvail < 10 {
		errAvail = MinWidth
	}
	errLines := Wrap(what, errAvail)
	if len(errLines) > 0 {
		p.write("ERROR: " + errLines[0] + "\n")
		for _, l := range errLines[1:] {
			p.write("       " + l + "\n")
		}
	} else {
		p.write("ERROR:\n")
	}

	whyAvail := p.width - 5
	if whyAvail < 10 {
		whyAvail = MinWidth
	}
	whyLines := Wrap(why, whyAvail)
	if len(whyLines) > 0 {
		p.write("WHY: " + whyLines[0] + "\n")
		for _, l := range whyLines[1:] {
			p.write("     " + l + "\n")
		}
	} else {
		p.write("WHY:\n")
	}

	fixAvail := p.width - 5
	if fixAvail < 10 {
		fixAvail = MinWidth
	}
	fixLines := Wrap(fix, fixAvail)
	if len(fixLines) > 0 {
		p.write("FIX: " + fixLines[0] + "\n")
		for _, l := range fixLines[1:] {
			p.write("     " + l + "\n")
		}
	} else {
		p.write("FIX:\n")
	}
}

// Raw writes text verbatim (diffs, file contents); it is never wrapped.
func (p *Printer) Raw(text string) {
	p.write(text)
}
