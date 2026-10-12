package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRequiredIgnoreEntriesCoverTheGeneratedSystemSkillFolders(t *testing.T) {
	want := []string{
		"/runtime/", "/.skillhub/transactions/",
		"/.mcp.json", "/.claude/settings.local.json", "/.codex/", "/.gemini/settings.json",
		"/.agents/skills/system-curator/", "/.claude/skills/system-curator/",
		"/.agents/skills/distill-lab/", "/.claude/skills/distill-lab/",
	}
	if !slices.Equal(requiredIgnoreEntries, want) {
		t.Fatalf("requiredIgnoreEntries = %q", requiredIgnoreEntries)
	}
	for _, entry := range requiredIgnoreEntries {
		if strings.Contains(entry, `\`) || strings.Contains(entry, ".gemini/skills") {
			t.Fatalf("entry %q must use forward slashes and never name .gemini/skills", entry)
		}
	}
}

func TestApplyWritesTheSystemSkillIgnoreEntriesOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	for range 2 {
		if _, err := Apply(root); err != nil {
			t.Fatal(err)
		}
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"/.agents/skills/system-curator/", "/.claude/skills/system-curator/", "/.agents/skills/distill-lab/", "/.claude/skills/distill-lab/"} {
		if count := strings.Count("\n"+string(ignore), "\n"+entry+"\n"); count != 1 {
			t.Fatalf("%s appears %d times:\n%s", entry, count, ignore)
		}
	}
}
