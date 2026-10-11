package skill

import (
	"fmt"
	"strings"
)

// diffContextLines is how many unchanged lines surround each change in a
// unified diff hunk.
const diffContextLines = 3

// noNewlineMarker follows a diff line whose source line has no terminating
// newline, matching the convention of diff(1) and git.
const noNewlineMarker = `\ No newline at end of file`

type lineOpKind byte

const (
	lineEqual  lineOpKind = ' '
	lineDelete lineOpKind = '-'
	lineInsert lineOpKind = '+'
)

// lineOp is one step of an edit script: an old line kept, an old line
// removed, or a new line added. oldPos and newPos count the old and new lines
// that come before this step.
type lineOp struct {
	kind           lineOpKind
	text           string
	oldPos, newPos int
}

// splitDiffLines splits data into lines that keep their "\n" terminator, so a
// last line without one compares unequal to the same text with one. A "\r"
// before the newline stays part of the line, which keeps line-ending changes
// visible in the diff.
func splitDiffLines(data []byte) []string {
	text := string(data)
	var lines []string
	for text != "" {
		end := strings.IndexByte(text, '\n')
		if end < 0 {
			lines = append(lines, text)
			break
		}
		lines = append(lines, text[:end+1])
		text = text[end+1:]
	}
	return lines
}

// diffLines returns a shortest edit script turning a into b, computed with
// Myers' O(ND) algorithm after trimming the common prefix and suffix. Among
// equally short scripts it prefers deletions before insertions, so the output
// is deterministic.
func diffLines(a, b []string) []lineOp {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	kinds := make([]lineOpKind, 0, len(a)+len(b))
	for range prefix {
		kinds = append(kinds, lineEqual)
	}
	kinds = append(kinds, myers(a[prefix:len(a)-suffix], b[prefix:len(b)-suffix])...)
	for range suffix {
		kinds = append(kinds, lineEqual)
	}

	ops := make([]lineOp, len(kinds))
	oldPos, newPos := 0, 0
	for i, kind := range kinds {
		op := lineOp{kind: kind, oldPos: oldPos, newPos: newPos}
		switch kind {
		case lineEqual:
			op.text = a[oldPos]
			oldPos++
			newPos++
		case lineDelete:
			op.text = a[oldPos]
			oldPos++
		case lineInsert:
			op.text = b[newPos]
			newPos++
		}
		ops[i] = op
	}
	return ops
}

// myers returns the kinds of a shortest edit script turning a into b.
func myers(a, b []string) []lineOpKind {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil
	}
	maxD := n + m
	offset := maxD + 1
	v := make([]int, 2*maxD+3)
	// down reports whether the furthest path on diagonal k at edit distance d
	// is reached by an insertion (a step down from diagonal k+1) rather than a
	// deletion (a step right from diagonal k-1).
	down := func(v []int, k, d int) bool {
		return k == -d || (k != d && v[offset+k-1] < v[offset+k+1])
	}
	var trace [][]int
search:
	for d := 0; d <= maxD; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if down(v, k, d) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				break search
			}
		}
	}

	// Walk the trace backwards from (n, m) to recover the script.
	var reversed []lineOpKind
	x, y := n, m
	for d := len(trace) - 1; d >= 0; d-- {
		k := x - y
		var prevX, prevY int
		if d > 0 {
			prevK := k - 1
			if down(trace[d], k, d) {
				prevK = k + 1
			}
			prevX = trace[d][offset+prevK]
			prevY = prevX - prevK
		}
		for x > prevX && y > prevY {
			x--
			y--
			reversed = append(reversed, lineEqual)
		}
		if d == 0 {
			break
		}
		if x == prevX {
			y--
			reversed = append(reversed, lineInsert)
		} else {
			x--
			reversed = append(reversed, lineDelete)
		}
	}
	kinds := make([]lineOpKind, len(reversed))
	for i, kind := range reversed {
		kinds[len(reversed)-1-i] = kind
	}
	return kinds
}

// writeUnifiedHunks writes the hunks of a unified diff between old and new,
// each with up to context unchanged lines around its changes. It writes
// nothing when the contents are equal.
func writeUnifiedHunks(output *strings.Builder, old, new []byte, context int) {
	ops := diffLines(splitDiffLines(old), splitDiffLines(new))
	for start := 0; start < len(ops); {
		first := start
		for first < len(ops) && ops[first].kind == lineEqual {
			first++
		}
		if first == len(ops) {
			return
		}
		// Extend the hunk while the gap of unchanged lines before the next
		// change is small enough that the two context windows would meet.
		last := first
		for next := first + 1; next < len(ops); next++ {
			if ops[next].kind == lineEqual {
				continue
			}
			if next-last-1 > 2*context {
				break
			}
			last = next
		}
		end := min(last+context+1, len(ops))
		writeHunk(output, ops[max(first-context, start):end])
		start = end
	}
}

func writeHunk(output *strings.Builder, ops []lineOp) {
	oldCount, newCount := 0, 0
	for _, op := range ops {
		if op.kind != lineInsert {
			oldCount++
		}
		if op.kind != lineDelete {
			newCount++
		}
	}
	fmt.Fprintf(output, "@@ -%s +%s @@\n", hunkRange(ops[0].oldPos, oldCount), hunkRange(ops[0].newPos, newCount))
	for _, op := range ops {
		output.WriteByte(byte(op.kind))
		if text, terminated := strings.CutSuffix(op.text, "\n"); terminated {
			output.WriteString(text)
			output.WriteByte('\n')
		} else {
			output.WriteString(text)
			output.WriteString("\n" + noNewlineMarker + "\n")
		}
	}
}

// hunkRange formats one side of a hunk header from the count of lines before
// the hunk and the count inside it. As in diff(1), an empty side names the
// line it follows and a one-line side omits its count.
func hunkRange(before, count int) string {
	switch count {
	case 0:
		return fmt.Sprintf("%d,0", before)
	case 1:
		return fmt.Sprintf("%d", before+1)
	default:
		return fmt.Sprintf("%d,%d", before+1, count)
	}
}
