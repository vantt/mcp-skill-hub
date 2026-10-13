package skill

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/mutation"
)

func numberedLines(count int) []string {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	return lines
}

func joinLines(lines []string) []byte { return []byte(strings.Join(lines, "\n") + "\n") }

func renderOne(path string, before, after []byte, deleted bool) string {
	change := mutation.Change{Path: path, Contents: after, Delete: deleted}
	beforeMap := map[string][]byte{}
	if before != nil {
		beforeMap[path] = before
	}
	return RenderFullDiff([]mutation.Change{change}, beforeMap)
}

func assertDiff(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("diff mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderFullDiffSingleLineChangeGivesOneSmallHunk(t *testing.T) {
	t.Parallel()
	old := numberedLines(20)
	updated := append([]string(nil), old...)
	updated[9] = "line ten"
	got := renderOne("a.txt", joinLines(old), joinLines(updated), false)
	assertDiff(t, got, "--- a/a.txt\n+++ b/a.txt\n"+
		"@@ -7,7 +7,7 @@\n line 7\n line 8\n line 9\n-line 10\n+line ten\n line 11\n line 12\n line 13\n")
}

func TestRenderFullDiffDistantChangesGiveTwoHunks(t *testing.T) {
	t.Parallel()
	old := numberedLines(30)
	updated := append([]string(nil), old...)
	updated[1] = "second"
	updated[27] = "twenty-eighth"
	got := renderOne("a.txt", joinLines(old), joinLines(updated), false)
	assertDiff(t, got, "--- a/a.txt\n+++ b/a.txt\n"+
		"@@ -1,5 +1,5 @@\n line 1\n-line 2\n+second\n line 3\n line 4\n line 5\n"+
		"@@ -25,6 +25,6 @@\n line 25\n line 26\n line 27\n-line 28\n+twenty-eighth\n line 29\n line 30\n")
}

func TestRenderFullDiffNearbyChangesShareOneHunk(t *testing.T) {
	t.Parallel()
	old := numberedLines(20)
	updated := append([]string(nil), old...)
	updated[4] = "five"
	updated[10] = "eleven" // six unchanged lines between: the context windows meet
	got := renderOne("a.txt", joinLines(old), joinLines(updated), false)
	if strings.Count(got, "@@ -") != 1 {
		t.Fatalf("want one hunk:\n%s", got)
	}
	if !strings.Contains(got, "@@ -2,13 +2,13 @@\n") {
		t.Fatalf("hunk header:\n%s", got)
	}
}

func TestRenderFullDiffInsertionAndDeletionHeaders(t *testing.T) {
	t.Parallel()
	old := numberedLines(3)
	inserted := []string{"line 1", "line 2", "new", "line 3"}
	assertDiff(t, renderOne("a", joinLines(old), joinLines(inserted), false),
		"--- a/a\n+++ b/a\n@@ -1,3 +1,4 @@\n line 1\n line 2\n+new\n line 3\n")
	assertDiff(t, renderOne("a", joinLines(inserted), joinLines(old), false),
		"--- a/a\n+++ b/a\n@@ -1,4 +1,3 @@\n line 1\n line 2\n-new\n line 3\n")
}

func TestRenderFullDiffAddedAndDeletedFiles(t *testing.T) {
	t.Parallel()
	assertDiff(t, renderOne("new.txt", nil, []byte("one\ntwo\n"), false),
		"--- a/new.txt\n+++ b/new.txt\n@@ -0,0 +1,2 @@\n+one\n+two\n")
	assertDiff(t, renderOne("gone.txt", []byte("one\ntwo\n"), nil, true),
		"--- a/gone.txt\n+++ b/gone.txt\n@@ -1,2 +0,0 @@\n-one\n-two\n")
	assertDiff(t, renderOne("single.txt", nil, []byte("only\n"), false),
		"--- a/single.txt\n+++ b/single.txt\n@@ -0,0 +1 @@\n+only\n")
}

func TestRenderFullDiffSkipsUnchangedFilesAndSortsPaths(t *testing.T) {
	t.Parallel()
	before := map[string][]byte{"b.txt": []byte("same\n"), "c.txt": []byte("old\n")}
	changes := []mutation.Change{
		{Path: "c.txt", Contents: []byte("new\n")},
		{Path: "b.txt", Contents: []byte("same\n")},
		{Path: "a.txt", Contents: []byte("added\n")},
	}
	assertDiff(t, RenderFullDiff(changes, before),
		"--- a/a.txt\n+++ b/a.txt\n@@ -0,0 +1 @@\n+added\n"+
			"--- a/c.txt\n+++ b/c.txt\n@@ -1 +1 @@\n-old\n+new\n")
}

func TestRenderFullDiffMarksMissingNewlineAtEnd(t *testing.T) {
	t.Parallel()
	assertDiff(t, renderOne("a", []byte("one\ntwo"), []byte("one\ntwo\n"), false),
		"--- a/a\n+++ b/a\n@@ -1,2 +1,2 @@\n one\n-two\n\\ No newline at end of file\n+two\n")
	assertDiff(t, renderOne("a", []byte("one\ntwo\n"), []byte("one\nTWO"), false),
		"--- a/a\n+++ b/a\n@@ -1,2 +1,2 @@\n one\n-two\n+TWO\n\\ No newline at end of file\n")
	assertDiff(t, renderOne("a", nil, []byte("x"), false),
		"--- a/a\n+++ b/a\n@@ -0,0 +1 @@\n+x\n\\ No newline at end of file\n")
	// An unchanged last line without a newline stays context and keeps its marker.
	assertDiff(t, renderOne("a", []byte("one\ntwo"), []byte("ONE\ntwo"), false),
		"--- a/a\n+++ b/a\n@@ -1,2 +1,2 @@\n-one\n+ONE\n two\n\\ No newline at end of file\n")
}

func TestRenderFullDiffKeepsCarriageReturns(t *testing.T) {
	t.Parallel()
	got := renderOne("a", []byte("one\r\ntwo\r\nthree\r\n"), []byte("one\r\n2\r\nthree\r\n"), false)
	assertDiff(t, got, "--- a/a\n+++ b/a\n@@ -1,3 +1,3 @@\n one\r\n-two\r\n+2\r\n three\r\n")
	// A line-ending-only change is still visible.
	got = renderOne("a", []byte("one\r\n"), []byte("one\n"), false)
	assertDiff(t, got, "--- a/a\n+++ b/a\n@@ -1 +1 @@\n-one\r\n+one\n")
}

// lcsLength is a reference longest-common-subsequence length.
func lcsLength(a, b []string) int {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	return table[0][0]
}

func TestDiffLinesProducesValidShortestScripts(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewSource(1))
	alphabet := []string{"a\n", "b\n", "c\n", "d\n"}
	randomLines := func() []string {
		lines := make([]string, random.Intn(12))
		for i := range lines {
			lines[i] = alphabet[random.Intn(len(alphabet))]
		}
		return lines
	}
	for iteration := range 2000 {
		a, b := randomLines(), randomLines()
		ops := diffLines(a, b)
		var gotA, gotB []string
		equal := 0
		for _, op := range ops {
			switch op.kind {
			case lineEqual:
				gotA, gotB = append(gotA, op.text), append(gotB, op.text)
				equal++
			case lineDelete:
				gotA = append(gotA, op.text)
			case lineInsert:
				gotB = append(gotB, op.text)
			}
		}
		if strings.Join(gotA, "") != strings.Join(a, "") || strings.Join(gotB, "") != strings.Join(b, "") {
			t.Fatalf("iteration %d: script does not reproduce inputs a=%q b=%q ops=%v", iteration, a, b, ops)
		}
		if want := lcsLength(a, b); equal != want {
			t.Fatalf("iteration %d: script keeps %d lines, shortest keeps %d (a=%q b=%q)", iteration, equal, want, a, b)
		}
	}
}
