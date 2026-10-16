package cli

import (
	"regexp"
	"strings"
	"testing"
)

// TestSkillSubcommandHelpPrintsThatSubcommand keeps `skillhub help skill <sub>`
// and `skillhub skill <sub> --help` to the one subcommand, with an example,
// and every flag it names also on the `skill` page the docs are checked against.
func TestSkillSubcommandHelpPrintsThatSubcommand(t *testing.T) {
	t.Parallel()
	page := commandUsage["skill"]
	flag := regexp.MustCompile(`--[a-z][a-z-]*`)
	for _, name := range skillSubcommands {
		for _, args := range [][]string{{"help", "skill", name}, {"skill", name, "--help"}, {"skill", name, "-h"}, {"skill", name, "my-skill", "--help"}} {
			code, stdout, stderr := runCLI(t, args...)
			if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "Usage: skillhub skill "+name) || strings.Contains(stdout, "Subcommands:") || !strings.Contains(stdout, "Example") {
				t.Fatalf("%v: exit %d stderr %q\n%s", args, code, stderr, stdout)
			}
		}
		_, stdout, _ := runCLI(t, "help", "skill", name)
		for number, line := range strings.Split(stdout, "\n") {
			if example := strings.TrimSpace(line); strings.HasPrefix(line, "  skillhub ") {
				checkGuideCommand(t, number+1, example)
			}
		}
		for _, named := range flag.FindAllString(stdout, -1) {
			if named != "--help" && !regexp.MustCompile(`(^|[\s\[|(])`+regexp.QuoteMeta(named)+`([\s=,\]|)]|$)`).MatchString(page) {
				t.Errorf("help skill %s names %s, which `skillhub help skill` does not", name, named)
			}
		}
	}
	for _, args := range [][]string{{"help", "skill"}, {"skill", "--help"}} {
		if code, stdout, _ := runCLI(t, args...); code != 0 || !strings.Contains(stdout, "Subcommands:") {
			t.Fatalf("%v must print the whole skill page: exit %d\n%s", args, code, stdout)
		}
	}
	if code, _, stderr := runCLI(t, "help", "skill", "nope"); code != 2 || !strings.Contains(stderr, "skillhub help skill") {
		t.Fatalf("an unknown subcommand: exit %d stderr %q", code, stderr)
	}
}
