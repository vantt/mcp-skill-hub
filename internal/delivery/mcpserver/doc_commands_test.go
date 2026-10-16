package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var docPlaceholder = regexp.MustCompile(`<[A-Za-z][A-Za-z0-9_-]*>`)

// docSectionCommands returns the command lines of every bash block in one
// section of a repository doc, so a test runs the commands the doc shows.
func docSectionCommands(t *testing.T, docPath, section string) []string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "..", docPath))
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	lines, err := parseSectionCommands(string(content), section)
	if err != nil {
		t.Fatalf("%s %q: %v", docPath, section, err)
	}
	return lines
}

// fillDocPlaceholders replaces each <name> in a documented command with the
// shell-quoted value and fails when a placeholder is left unfilled.
func fillDocPlaceholders(t *testing.T, line string, values map[string]string) string {
	t.Helper()
	filled, err := fillPlaceholders(line, values)
	if err != nil {
		t.Fatal(err)
	}
	return filled
}

// parseSectionCommands finds the section by its heading line (ending at the
// next heading of the same or a higher level) or by a <summary> line (ending at
// the next </details>) and returns the non-blank, non-comment lines of its
// bash blocks verbatim.
func parseSectionCommands(content, section string) ([]string, error) {
	section = strings.TrimSpace(section)
	level := headingLevel(section)
	isSummary := strings.HasPrefix(section, "<summary>")

	var (
		found, inFence, inBash bool
		commands               []string
		blocks                 int
	)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				inFence, inBash = false, false
			} else {
				inFence = true
				inBash = found && strings.TrimSpace(strings.TrimPrefix(trimmed, "```")) == "bash"
				if inBash {
					blocks++
				}
			}
			continue
		}
		if inFence {
			if inBash && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				commands = append(commands, line)
			}
			continue
		}
		if !found {
			found = trimmed == section
			continue
		}
		if isSummary && trimmed == "</details>" {
			break
		}
		if !isSummary {
			if l := headingLevel(trimmed); l > 0 && l <= level {
				break
			}
		}
	}
	switch {
	case !found:
		return nil, fmt.Errorf("section not found")
	case blocks == 0:
		return nil, fmt.Errorf("section has no bash block")
	}
	return commands, nil
}

// headingLevel returns the number of leading # of a Markdown heading line, or 0.
func headingLevel(line string) int {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level == len(line) || line[level] != ' ' {
		return 0
	}
	return level
}

func fillPlaceholders(line string, values map[string]string) (string, error) {
	for name, value := range values {
		quoted := "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
		line = strings.ReplaceAll(line, "<"+name+">", quoted)
	}
	if left := docPlaceholder.FindString(line); left != "" {
		return "", fmt.Errorf("placeholder %s left unfilled in %q", left, line)
	}
	return line, nil
}

// The line counts pin the documented move, so a doc that gains or loses a step
// fails here until this table is updated on purpose.
func TestDocSectionCommandsReadsTheDocumentedMove(t *testing.T) {
	for _, tc := range []struct {
		doc, section string
		want         int
	}{
		{"docs/user-guide.md", "## Move to another machine", 2},
		{"docs/troubleshooting.md", "### Fresh clone on a new machine", 1},
		{"README.md", "### 4. Check", 1},
	} {
		t.Run(tc.doc+" "+tc.section, func(t *testing.T) {
			lines := docSectionCommands(t, tc.doc, tc.section)
			if len(lines) != tc.want {
				t.Fatalf("%s %q has %d command lines, want %d: %q", tc.doc, tc.section, len(lines), tc.want, lines)
			}
		})
	}
}

func TestParseSectionCommands(t *testing.T) {
	doc := strings.Join([]string{
		"# Title",
		"## First",
		"```bash",
		"# a comment",
		"",
		"cd a && run <x>",
		"```",
		"```text",
		"sample output",
		"```",
		"### Nested",
		"```bash",
		"nested one",
		"```",
		"## Second",
		"```bash",
		"second one",
		"```",
		"## Empty",
		"only prose",
		"## Details",
		"<details>",
		"<summary>More</summary>",
		"```bash",
		"inside details",
		"```",
		"</details>",
		"```bash",
		"after details",
		"```",
	}, "\n")

	for _, tc := range []struct {
		name, section string
		want          []string
		wantErr       string
	}{
		{"keeps lines verbatim and includes nested headings", "## First", []string{"cd a && run <x>", "nested one"}, ""},
		{"stops at a heading of the same level", "## Second", []string{"second one"}, ""},
		{"summary section ends at details close", "<summary>More</summary>", []string{"inside details"}, ""},
		{"missing heading fails", "## Missing", nil, "section not found"},
		{"section without a bash block fails", "## Empty", nil, "no bash block"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSectionCommands(doc, tc.section)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFillPlaceholders(t *testing.T) {
	got, err := fillPlaceholders("init <git-url> && cd <dir>", map[string]string{"git-url": "/tmp/it's", "dir": "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `init '/tmp/it'\''s' && cd '/x'`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := fillPlaceholders("cd <your-project>", map[string]string{"git-url": "x"}); err == nil || !strings.Contains(err.Error(), "<your-project>") {
		t.Fatalf("unknown placeholder error = %v", err)
	}
}
