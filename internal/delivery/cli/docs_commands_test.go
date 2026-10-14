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

// repoRoot is the repository root seen from this package's directory.
var repoRoot = filepath.Join("..", "..", "..")

// docsWithCommands are the user docs whose skillhub commands must match the
// CLI help, with the fewest commands each must show, so a doc whose commands
// stop being found (for example after a formatting change) fails too.
var docsWithCommands = []struct {
	path string
	min  int
}{
	{"README.md", 8},
	{"docs/remote-hub-guide.md", 45},
	{"docs/troubleshooting.md", 30},
	{"docs/installation.md", 3},
	{"docs/command-reference.md", 55},
	{"docs/user-guide.md", 20},
	{"docs/curating-skills.md", 40},
}

// docBans are texts the user docs must not contain anywhere: links to the
// removed getting-started page, removed flags, a hub created in a folder
// argument, and staging the whole hub.
var docBans = []string{
	"getting-started.md",
	"--trust-private-network",
	"--loopback-only",
	"skillhub init ~",
	"git add -A",
}

// TestDocsCommandsExist checks every skillhub command the onboarding docs
// show, in code blocks and in inline code, against the CLI's own help: each
// command and subcommand exists, and each flag appears in that command's help
// text. Commands inside chains such as `cd <x> && skillhub ...` count too.
// Examples in code blocks never pass --workspace: hub commands are written
// to run in the current folder.
func TestDocsCommandsExist(t *testing.T) {
	for _, doc := range docsWithCommands {
		t.Run(doc.path, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(doc.path)))
			if err != nil {
				t.Fatal(err)
			}
			checked := 0
			inBlock := false
			for number, line := range strings.Split(string(content), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "```") {
					inBlock = !inBlock
					continue
				}
				if inBlock && strings.Contains(line, "--workspace") {
					t.Errorf("line %d: a code block passes --workspace; write `cd <folder> && skillhub <command>` instead", number+1)
				}
				var commands []string
				if inBlock && !strings.Contains(line, "`") {
					// A command line; a line of sample output that quotes a
					// command in backticks is read like prose below.
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
			t.Logf("%d skillhub commands checked", checked)
			if checked < doc.min {
				t.Fatalf("only %d skillhub commands found, want at least %d", checked, doc.min)
			}
		})
	}
}

// TestDocsAvoidRemovedNames keeps the README and every top-level user doc free
// of the removed page, flags and setup forms in docBans.
func TestDocsAvoidRemovedNames(t *testing.T) {
	docs, err := filepath.Glob(filepath.Join(repoRoot, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	docs = append(docs, filepath.Join(repoRoot, "README.md"))
	if len(docs) < 10 {
		t.Fatalf("found only %d docs", len(docs))
	}
	for _, path := range docs {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for number, line := range strings.Split(string(content), "\n") {
			for _, ban := range docBans {
				if strings.Contains(line, ban) {
					t.Errorf("%s:%d: mentions %q", filepath.Base(path), number+1, ban)
				}
			}
		}
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

// TestCheckGuideCommandReadsArgumentsAndChoices keeps the forms the docs use
// valid, and still rejects an unknown subcommand or flag.
func TestCheckGuideCommandReadsArgumentsAndChoices(t *testing.T) {
	for _, command := range []string{
		"skillhub skill activate commit-style --yes",
		"skillhub maintain validate --staged",
		"skillhub status [--json\\|--quiet]",
		"skillhub help --dev",
		"skillhub help serve",
		"skillhub <command>",
	} {
		checkGuideCommand(t, 1, command)
	}
	for _, command := range []string{
		"skillhub skill activat commit-style",
		"skillhub maintain valdate",
		"skillhub status [--json\\|--loud]",
		"skillhub help no-such-command",
		"skillhub validate",
	} {
		if guideCommandProblems(t, command) == nil {
			t.Errorf("%q passed the guide check", command)
		}
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
	placeholder := false
	for _, field := range fields {
		field = strings.Trim(field, "[]|")
		switch {
		case strings.HasPrefix(field, "-"):
			// A choice such as --json|--quiet (or --json\|--quiet inside a
			// Markdown table) names two flags.
			for _, choice := range strings.Split(strings.ReplaceAll(field, `\|`, "|"), "|") {
				if strings.HasPrefix(choice, "-") {
					flags = append(flags, strings.SplitN(strings.TrimRight(choice, "]."), "=", 2)[0])
				}
			}
		case len(flags) == 0 && regexp.MustCompile(`^[a-z][a-z-]*$`).MatchString(field):
			words = append(words, field)
		case strings.HasPrefix(field, "<"):
			placeholder = true
		}
	}
	if len(words) == 0 && len(flags) == 0 {
		if placeholder {
			// The general form `skillhub <command>` names no command.
			return nil
		}
		return []string{"no command in " + strconv.Quote(command)}
	}
	var problems []string
	var help string
	switch {
	case len(words) > 0 && words[0] == "help":
		// `skillhub help`, `skillhub help <command>` and `skillhub help --dev`
		// are valid when they run.
		args := append(append([]string{}, words...), flags...)
		if code, _, stderr := runCLI(t, args...); code != 0 {
			return []string{"`skillhub " + strings.Join(args, " ") + "` fails: " + stderr}
		}
		return nil
	case len(words) == 0:
		code, out, stderr := runCLI(t, "help")
		if code != 0 {
			return []string{"`skillhub help` fails: " + stderr}
		}
		help = out
	default:
		code, out, stderr := runCLI(t, "help", words[0])
		if code != 0 {
			return []string{"`skillhub help " + words[0] + "` fails: " + stderr}
		}
		help = out
		for index, word := range words[1:] {
			// The first word after the command must be a subcommand its help
			// names. Later words are subcommands while the help names them; the
			// first one it does not name starts the arguments, such as a skill id.
			named := regexp.MustCompile(`(^|[\s|])` + regexp.QuoteMeta(word) + `([\s|,\]]|$)`)
			if !named.MatchString(help) {
				if index > 0 {
					break
				}
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
