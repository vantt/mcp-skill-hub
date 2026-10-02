package cli

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHumanOutputFitsWidth(t *testing.T) {
	root := initTestWorkspace(t)
	t.Setenv("SKILLHUB_WORKSPACE", root)

	longDescription := "Review a multi-module change for correctness, regressions, security posture, documentation impact and rollout risk, recording evidence for every finding before recommending a fix, and never approving a change whose cross-module contracts were not traced to their callers."
	resCode, _, stderr := runCLI(t, "skill", "create", "wide-skill", "--collection", "core", "--name", "Wide Skill", "--description", longDescription, "--yes")
	if resCode != 0 {
		t.Fatalf("create failed (exit %d): %s", resCode, stderr)
	}

	commands := [][]string{
		{"status"},
		{"skill", "list"},
		{"skill", "review", "wide-skill"},
		{"skill", "activate", "wide-skill", "--yes"},
		{"source", "list"},
		{"validate"},
	}

	assertWidth := func(output, cmdName string) {
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimRight(line, "\r")
			if strings.HasPrefix(trimmed, "  $ ") {
				continue
			}
			trimmedSpace := strings.TrimSpace(trimmed)
			// Unbreakable tokens: single word or backtick-enclosed span
			unquoted := strings.TrimRight(trimmedSpace, ",.:;")
			if strings.HasPrefix(unquoted, "`") && strings.HasSuffix(unquoted, "`") {
				continue
			}
			if !strings.Contains(trimmedSpace, " ") {
				continue
			}
			runeCount := utf8.RuneCountInString(trimmed)
			if runeCount > 100 {
				t.Errorf("%s: line exceeds 100 runes (%d): %q", cmdName, runeCount, trimmed)
			}
		}
	}

	for _, cmd := range commands {
		cmdName := strings.Join(cmd, " ")
		_, stdout, stderr := runCLI(t, cmd...)
		assertWidth(stdout, cmdName+" (stdout)")
		assertWidth(stderr, cmdName+" (stderr)")
	}
}
