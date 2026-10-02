package termui

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestWrapKeepsLongTokensIntact(t *testing.T) {
	t.Parallel()
	input := "see https://example.com/a/very/long/path/that/exceeds/the/width now"
	got := Wrap(input, 20)
	want := []string{
		"see",
		"https://example.com/a/very/long/path/that/exceeds/the/width",
		"now",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Wrap(%q, 20) = %#v; want %#v", input, got, want)
	}
}

func TestWrapKeepsBacktickSpansIntact(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := NewWithWidth(&buf, 60)
	cmd := "`skillhub skill edit long-review --trigger \"<when to use>\" --not-for \"<when not>\" --yes`"
	fixText := fmt.Sprintf("Run %s, then retry activating the skill.", cmd)
	p.Error("The request cannot be accepted.", "missing fields", fixText)

	output := buf.String()
	found := false
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, cmd) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected backtick span to stay intact on one line, got output:\n%s", output)
	}
}

func TestFieldsAlignLabels(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := NewWithWidth(&buf, 60)
	desc := "Review a multi-module change for correctness, regressions, security posture, documentation impact and rollout risk, recording evidence for every finding before recommending a fix, and never approving."
	p.Fields(
		Field{Label: "ID", Value: "long-review"},
		Field{Label: "Collection", Value: "core"},
		Field{Label: "State", Value: "draft"},
		Field{Label: "Empty", Value: ""},
		Field{Label: "Description", Value: desc},
	)

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d:\n%s", len(lines), out)
	}

	// Longest label is "Description" (11). valueCol is 11 + 3 = 14.
	wantPrefix := "ID:           long-review"
	if lines[0] != wantPrefix {
		t.Fatalf("line 0 = %q; want %q", lines[0], wantPrefix)
	}
	if strings.Contains(out, "Empty:") {
		t.Fatalf("expected empty field to be skipped, got:\n%s", out)
	}

	// Continuation lines of Description (lines after index 3) must start with 14 spaces
	descPrefix := "Description:  "
	if !strings.HasPrefix(lines[3], descPrefix) {
		t.Fatalf("line 3 = %q; want prefix %q", lines[3], descPrefix)
	}
	indent := strings.Repeat(" ", 14)
	for i := 4; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], indent) {
			t.Fatalf("continuation line %d does not have indent of 14 spaces: %q", i, lines[i])
		}
	}
}

func TestErrorBlockOrder(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := NewWithWidth(&buf, 60)
	longWhy := "This is a rather long why message that will certainly wrap across multiple lines because it exceeds the sixty rune width limit for testing."
	p.Error("Operation failed.", longWhy, "Check logs.")

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "ERROR: ") {
		t.Fatalf("expected first line to start with 'ERROR: ', got %q", lines[0])
	}

	whyIndex := -1
	fixIndex := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "WHY: ") {
			whyIndex = i
		}
		if strings.HasPrefix(l, "FIX: ") {
			fixIndex = i
		}
	}
	if whyIndex == -1 || fixIndex == -1 || whyIndex >= fixIndex {
		t.Fatalf("expected ERROR then WHY then FIX order, got:\n%s", out)
	}

	// Check continuation lines of WHY have 5 spaces indent
	for i := whyIndex + 1; i < fixIndex; i++ {
		if !strings.HasPrefix(lines[i], "     ") {
			t.Fatalf("expected WHY continuation line to have 5-space indent, got %q", lines[i])
		}
	}
}

func TestCommandLinesAreNeverWrapped(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := NewWithWidth(&buf, 60)
	longCmd := "git -C /a/very/long/workspace/path/that/keeps/going/and/going/and/going/and/exceeds/the/sixty/rune/terminal/width/by/a/large/margin/and/continues/to/run/without/breaking/any/rules/at/all add -A && git commit -m 'A very descriptive and long commit message that definitely pushes past three hundred characters total in length easily.'"
	p.Command(longCmd)
	p.Next("Review changes", longCmd)

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	expectedLine := "  $ " + longCmd
	foundCount := 0
	for _, l := range lines {
		if l == expectedLine {
			foundCount++
		}
	}
	if foundCount != 2 {
		t.Fatalf("expected exactly 2 unwrapped command lines, found %d in:\n%s", foundCount, out)
	}
}

func TestBytesHumanized(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1395, "1.4 KB"},
		{187620, "183.2 KB"},
		{4 * 1024 * 1024, "4.0 MB"},
		{1 << 30, "1.0 GB"},
	}
	for _, tc := range cases {
		got := Bytes(tc.input)
		if got != tc.want {
			t.Errorf("Bytes(%d) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestTableAlignsColumns(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := NewWithWidth(&buf, 60)
	headers := []string{"ID", "STATE", "DESCRIPTION"}
	longDesc := "A very detailed description of this skill that definitely exceeds sixty characters and needs to wrap onto multiple lines neatly under its column."
	rows := [][]string{
		{"s1", "active", "Short desc"},
		{"s2", "draft", longDesc},
	}
	p.Table(headers, rows)

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected table lines to wrap, got %d lines:\n%s", len(lines), out)
	}

	// ID col: len 2 + 2 = 4. STATE col: len 6 + 2 = 8.
	// Last col starts at 4 + 8 = 12.
	lastColStart := 12
	indent := strings.Repeat(" ", lastColStart)
	for i := 3; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], indent) {
			t.Fatalf("table continuation line %d not indented to column %d: %q", i, lastColStart, lines[i])
		}
	}
}

func TestWidthClamping(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if got := New(&buf).Width(); got != DefaultWidth {
		t.Fatalf("New(&buf).Width() = %d; want %d", got, DefaultWidth)
	}
	if got := NewWithWidth(&buf, 10).Width(); got != MinWidth {
		t.Fatalf("NewWithWidth(&buf, 10).Width() = %d; want %d", got, MinWidth)
	}
	if got := NewWithWidth(&buf, 500).Width(); got != MaxWidth {
		t.Fatalf("NewWithWidth(&buf, 500).Width() = %d; want %d", got, MaxWidth)
	}
}

func renderSamplePage(width int) string {
	var buf bytes.Buffer
	p := NewWithWidth(&buf, width)

	// (a) skill review page re-creating skill-review-draft.human.txt
	p.Fields(
		Field{Label: "Review", Value: "Long Review (long-review)"},
		Field{Label: "Collection", Value: "core"},
		Field{Label: "Description", Value: "Review a multi-module change for correctness, regressions, security posture, documentation impact and rollout risk, recording evidence for every finding before recommending a fix, and never approving a change whose cross-module contracts were not traced to their callers."},
		Field{Label: "State", Value: "draft (valid; routing: not eligible; servable)"},
		Field{Label: "Files", Value: fmt.Sprintf("2 file(s), %s (entrypoint: skills/core/long-review/SKILL.md)", Bytes(1395))},
		Field{Label: "Origin", Value: "local authoring (not watched)"},
		Field{Label: "Git", Value: "clean"},
	)
	p.Heading("Activation readiness")
	p.Bullets(
		"Missing required field: trigger",
		"Missing required field: not_for or rationale (quality.routing_review_rationale)",
		"Missing required field: min_scope",
	)
	p.Blank()
	p.Next("Complete required activation fields (trigger, not_for or rationale (quality.routing_review_rationale), min_scope) with `skillhub skill edit long-review`.", "")

	p.Blank()

	// (b) status page re-creating status-dirty.human.txt
	p.Fields(
		Field{Label: "Workspace", Value: "<TMP>/workspace"},
		Field{Label: "Status", Value: "Git has uncommitted canonical changes."},
		Field{Label: "Health", Value: "Workspace valid; search index current; Git has uncommitted changes."},
		Field{Label: "Inventory", Value: "No skills yet."},
	)
	p.Blank()
	p.Next("Review uncommitted changes", "git -C <TMP>/workspace add -A && git -C <TMP>/workspace commit -m \"Update skills\"")

	p.Blank()

	// (c) activation error from skill-activate-missing-fields.human.txt
	p.Error(
		"The request cannot be accepted.",
		"skill long-review requires: trigger, not_for or rationale (quality.routing_review_rationale), min_scope",
		"Run `skillhub skill edit long-review --trigger \"<when to use>\" --not-for \"<when not to use>\" --min-scope single_step --yes`, then retry `skillhub skill activate long-review --yes`.",
	)

	p.Blank()

	// (d) skill list table with three rows, one of which has a 90-character name
	headers := []string{"ID", "STATE", "COLLECTION", "NAME"}
	rows := [][]string{
		{"long-review", "draft", "core", "Review a multi-module change for correctness, regressions, security posture, and docs"},
		{"git-expert", "active", "core", "Git expert operations and repository analysis"},
		{"super-comprehensive-auditor", "active", "community", "A very comprehensive skill with a super long description name that easily exceeds ninety characters in length"},
	}
	p.Table(headers, rows)

	return buf.String()
}

func TestWriteSamples(t *testing.T) {
	out := os.Getenv("TERMUI_SAMPLES_OUT")
	if out == "" {
		return
	}

	var b strings.Builder
	b.WriteString("===== width 100 =====\n")
	b.WriteString(renderSamplePage(100))
	b.WriteString("\n===== width 60 =====\n")
	b.WriteString(renderSamplePage(60))

	if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
		t.Fatalf("failed to write samples: %v", err)
	}
}
