package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skillhubCommand finds a skillhub command in a line of the guide: a command
// line in a shell block, a "$ skillhub" line in printed output, or inline code.
var skillhubCommand = regexp.MustCompile("(?:^|\\$ |`)(skillhub [^`#]*)")

// TestRemoteHubGuideCommandsExist checks every skillhub command the remote hub
// guide shows against the CLI's own help: each command and subcommand exists,
// and each flag appears in that command's help text.
func TestRemoteHubGuideCommandsExist(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "remote-hub-guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	inBlock := false
	for number, line := range strings.Split(string(guide), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inBlock = !inBlock
			continue
		}
		var commands []string
		if inBlock {
			if match := skillhubCommand.FindStringSubmatch(trimmed); match != nil && (strings.HasPrefix(trimmed, "skillhub ") || strings.HasPrefix(trimmed, "$ skillhub ")) {
				commands = append(commands, match[1])
			}
		} else {
			for _, match := range regexp.MustCompile("`(skillhub [^`]*)`").FindAllStringSubmatch(line, -1) {
				commands = append(commands, match[1])
			}
		}
		for _, command := range commands {
			checkGuideCommand(t, number+1, command)
			checked++
		}
	}
	if checked < 30 {
		t.Fatalf("only %d skillhub commands found in the guide", checked)
	}
}

func checkGuideCommand(t *testing.T, line int, command string) {
	t.Helper()
	fields := strings.Fields(command)[1:]
	var words, flags []string
	for _, field := range fields {
		field = strings.Trim(field, "[]|")
		switch {
		case strings.HasPrefix(field, "-"):
			flags = append(flags, strings.SplitN(field, "=", 2)[0])
		case len(flags) == 0 && regexp.MustCompile(`^[a-z][a-z-]*$`).MatchString(field):
			words = append(words, field)
		}
	}
	if len(words) == 0 {
		t.Errorf("line %d: no command in %q", line, command)
		return
	}
	code, help, stderr := runCLI(t, "help", words[0])
	if code != 0 {
		t.Errorf("line %d: `skillhub help %s` exits %d: %s", line, words[0], code, stderr)
		return
	}
	for _, word := range words[1:] {
		// Only subcommand words count; arguments such as a skill id follow
		// the flags or are placeholders.
		if !strings.Contains(help, " "+word) {
			t.Errorf("line %d: %q: `skillhub help %s` does not mention %q", line, command, words[0], word)
		}
	}
	for _, flag := range flags {
		// The whole flag, so -g does not match inside another word and --hub
		// does not match --hub-something.
		whole := regexp.MustCompile(`(^|[\s,\[|(])` + regexp.QuoteMeta(flag) + `([\s=,\]|)]|$)`)
		if !whole.MatchString(help) {
			t.Errorf("line %d: %q: `skillhub help %s` does not mention the flag %s", line, command, words[0], flag)
		}
	}
}
