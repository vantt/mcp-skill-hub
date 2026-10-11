package shellquote

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

var roundTripValues = []string{
	"plain",
	"/home/you/skill-hub",
	"/home/you/my skills",
	"it's here",
	"$HOME and `cmd`",
	"a;b && c | d > e",
	"~/not-expanded",
	"",
	`C:\Users\you\skill hub`,
	"*?[x]",
}

func TestArgForPOSIX(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"plain":               "plain",
		"/home/you/skill-hub": "/home/you/skill-hub",
		"http://h:7420":       "http://h:7420",
		"/home/you/my skills": "'/home/you/my skills'",
		"it's":                `'it'\''s'`,
		"$HOME":               "'$HOME'",
		"":                    "''",
		`C:\x`:                `'C:\x'`,
		"~/x":                 "'~/x'",
	}
	for value, want := range cases {
		if got := ArgFor("linux", value); got != want {
			t.Errorf("ArgFor(linux, %q) = %q, want %q", value, got, want)
		}
	}
}

func TestArgForPowerShell(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"plain":                     "plain",
		`C:\Users\you\skill-hub`:    `C:\Users\you\skill-hub`,
		`C:\Users\you\my skills`:    `'C:\Users\you\my skills'`,
		"it's":                      "'it''s'",
		"it\u2019s":                 "'it\u2019\u2019s'",
		"$env:HOME":                 "'$env:HOME'",
		"":                          "''",
		"a;b":                       "'a;b'",
		"`n":                        "'`n'",
		"http://127.0.0.1:7420":     "http://127.0.0.1:7420",
		"value with \"double\" one": "'value with \"double\" one'",
	}
	for value, want := range cases {
		if got := ArgFor("windows", value); got != want {
			t.Errorf("ArgFor(windows, %q) = %q, want %q", value, got, want)
		}
	}
}

// TestArgRoundTripsThroughTheShell pastes each quoted value into the shell of
// this platform and checks that the shell hands the program the value as it was.
func TestArgRoundTripsThroughTheShell(t *testing.T) {
	t.Parallel()
	for _, value := range roundTripValues {
		var command *exec.Cmd
		if runtime.GOOS == "windows" {
			shell, err := exec.LookPath("powershell")
			if err != nil {
				t.Skip("powershell is not available")
			}
			command = exec.Command(shell, "-NoProfile", "-NonInteractive", "-Command", "Write-Output "+Arg(value))
		} else {
			command = exec.Command("sh", "-c", "printf '%s' "+Arg(value))
		}
		out, err := command.Output()
		if err != nil {
			t.Fatalf("shell for %q: %v", value, err)
		}
		if got := strings.TrimRight(string(out), "\r\n"); got != value {
			t.Errorf("round trip of %q gave %q", value, got)
		}
	}
}

func TestChainUsesTheSeparatorOfEachShell(t *testing.T) {
	t.Parallel()
	if got := ChainFor("linux", MakeDirFor("linux", "~/hub"), "cd ~/hub", "skillhub init"); got != "mkdir -p ~/hub && cd ~/hub && skillhub init" {
		t.Fatalf("POSIX chain = %q", got)
	}
	if got := ChainFor("windows", MakeDirFor("windows", "~/hub"), "cd ~/hub", "skillhub init"); got != "mkdir -Force ~/hub; cd ~/hub; skillhub init" {
		t.Fatalf("PowerShell chain = %q", got)
	}
}
