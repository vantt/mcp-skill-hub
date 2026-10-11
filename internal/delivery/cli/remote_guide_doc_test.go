package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// inlineCode finds the inline code spans of a Markdown line.
var inlineCode = regexp.MustCompile("`([^`]+)`")

// commandSegments splits a documented or printed command line into the
// skillhub commands it runs. The line is split on "&&" and ";"; parts that run
// another program (cd, mkdir, git, ssh, scp and so on) are dropped, a leading
// "$ " prompt is removed and a trailing "# comment" is cut off.
func commandSegments(line string) []string {
	var segments []string
	for _, chain := range strings.Split(line, "&&") {
		for _, part := range strings.Split(chain, ";") {
			part = strings.TrimSpace(part)
			part = strings.TrimSpace(strings.TrimPrefix(part, "$ "))
			if index := strings.Index(part, " #"); index >= 0 {
				part = strings.TrimSpace(part[:index])
			}
			if strings.HasPrefix(part, "skillhub ") {
				segments = append(segments, part)
			}
		}
	}
	return segments
}

func TestCommandSegments(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"cd ~/skill-hub && skillhub init":                   {"skillhub init"},
		"mkdir -p a && cd a && skillhub init":               {"skillhub init"},
		"cd a; skillhub init":                               {"skillhub init"},
		"skillhub --version":                                {"skillhub --version"},
		"git -C h commit -m x":                              nil,
		"$ skillhub doctor --fix   # repair":                {"skillhub doctor --fix"},
		"scp b:/x/.hub-token ~/t && skillhub connect --hub": {"skillhub connect --hub"},
		"skillhub status && skillhub doctor":                {"skillhub status", "skillhub doctor"},
	}
	for line, want := range cases {
		if got := commandSegments(line); !reflect.DeepEqual(got, want) {
			t.Errorf("commandSegments(%q) = %q, want %q", line, got, want)
		}
	}
}

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
			commands = commandSegments(trimmed)
		} else {
			for _, match := range inlineCode.FindAllStringSubmatch(line, -1) {
				commands = append(commands, commandSegments(match[1])...)
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

// TestCheckGuideCommandAcceptsGlobalFlags keeps `skillhub --version` valid in
// docs and printed next commands, and rejects a global flag that does not exist.
func TestCheckGuideCommandAcceptsGlobalFlags(t *testing.T) {
	checkGuideCommand(t, 1, "skillhub --version")
	if guideCommandProblems(t, "skillhub --no-such-flag") == nil {
		t.Error("an unknown global flag passed the guide check")
	}
}

func checkGuideCommand(t *testing.T, line int, command string) {
	t.Helper()
	for _, problem := range guideCommandProblems(t, command) {
		t.Errorf("line %d: %s", line, problem)
	}
}

// guideCommandProblems lists why a skillhub command does not match the CLI
// help: an unknown command or subcommand, or a flag the help does not name.
// A command with no command word is checked against the global help, so
// `skillhub --version` passes.
func guideCommandProblems(t *testing.T, command string) []string {
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
	if len(words) == 0 && len(flags) == 0 {
		return []string{"no command in " + strconv.Quote(command)}
	}
	var problems []string
	var help string
	if len(words) == 0 {
		code, out, stderr := runCLI(t, "help")
		if code != 0 {
			return []string{"`skillhub help` fails: " + stderr}
		}
		help = out
	} else {
		code, out, stderr := runCLI(t, "help", words[0])
		if code != 0 {
			return []string{"`skillhub help " + words[0] + "` fails: " + stderr}
		}
		help = out
		for _, word := range words[1:] {
			// Only subcommand words count; arguments such as a skill id follow
			// the flags or are placeholders.
			if !strings.Contains(help, " "+word) {
				problems = append(problems, strconv.Quote(command)+": `skillhub help "+words[0]+"` does not mention "+strconv.Quote(word))
			}
		}
	}
	for _, flag := range flags {
		// The whole flag, so -g does not match inside another word and --hub
		// does not match --hub-something.
		whole := regexp.MustCompile(`(^|[\s,\[|(])` + regexp.QuoteMeta(flag) + `([\s=,\]|)]|$)`)
		if !whole.MatchString(help) {
			problems = append(problems, strconv.Quote(command)+": the help does not mention the flag "+flag)
		}
	}
	return problems
}
