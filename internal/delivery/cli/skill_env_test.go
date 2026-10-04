package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const cliEnvSentinel = "cli-sentinel-secret-5d2c80"

// withEnvInput feeds stdin to `skill env set`. Tests using it must not run in
// parallel because the input is process-wide.
func withEnvInput(t *testing.T, input string) {
	t.Helper()
	previous := skillEnvInput
	skillEnvInput = strings.NewReader(input)
	t.Cleanup(func() { skillEnvInput = previous })
}

func TestSkillEnvSetListUnsetNeverPrintsValues(t *testing.T) {
	root := initTestWorkspace(t)
	createActiveDoctorSkill(t, root, "env-cli", "runtime:\n  requires:\n    env: [CLI_TOKEN]\n  setup:\n    check: test -n \"$CLI_TOKEN\"\n")
	var outputs []string

	withEnvInput(t, cliEnvSentinel+"\n")
	code, stdout, stderr := runCLI(t, "skill", "env", "set", "env-cli", "CLI_TOKEN", "--workspace", root)
	outputs = append(outputs, stdout, stderr)
	if code != 0 || !strings.Contains(stdout, "CLI_TOKEN") {
		t.Fatalf("set exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	withEnvInput(t, "second-value\r\n")
	if code, stdout, stderr = runCLI(t, "skill", "env", "set", "env-cli", "OTHER_KEY", "--workspace", root, "--json"); code != 0 {
		t.Fatalf("set second exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	outputs = append(outputs, stdout, stderr)

	code, stdout, stderr = runCLI(t, "skill", "env", "list", "env-cli", "--workspace", root)
	outputs = append(outputs, stdout, stderr)
	if code != 0 || !strings.Contains(stdout, "CLI_TOKEN") || !strings.Contains(stdout, "OTHER_KEY") || !strings.Contains(stdout, "never shown") {
		t.Fatalf("list exit = %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr = runCLI(t, "skill", "env", "list", "env-cli", "--workspace", root, "--json")
	outputs = append(outputs, stdout, stderr)
	if code != 0 || !strings.Contains(stdout, `"keys"`) {
		t.Fatalf("list json exit = %d stdout=%s", code, stdout)
	}

	if runtime.GOOS != "windows" {
		code, stdout, stderr = runCLI(t, "skill", "doctor", "env-cli", "--workspace", root)
		outputs = append(outputs, stdout, stderr)
		if code != 0 || !strings.Contains(stdout, "PASS  env CLI_TOKEN") {
			t.Fatalf("doctor must accept the stored variable (exit %d): %s%s", code, stdout, stderr)
		}
		if code, stdout, stderr = runCLI(t, "skill", "env", "unset", "env-cli", "CLI_TOKEN", "--workspace", root); code != 0 {
			t.Fatalf("unset exit = %d stdout=%s stderr=%s", code, stdout, stderr)
		}
		t.Setenv("CLI_TOKEN", "")
		os.Unsetenv("CLI_TOKEN")
		code, stdout, stderr = runCLI(t, "skill", "doctor", "env-cli", "--workspace", root)
		outputs = append(outputs, stdout, stderr)
		if code != 1 || !strings.Contains(stdout, "skillhub skill env set env-cli CLI_TOKEN") {
			t.Fatalf("doctor must suggest storing the variable (exit %d): %s%s", code, stdout, stderr)
		}
	}

	for _, output := range outputs {
		if strings.Contains(output, cliEnvSentinel) || strings.Contains(output, "second-value") {
			t.Fatalf("a CLI output leaks a stored value: %s", output)
		}
	}
	// Telemetry, caches, and every other workspace file stay free of the value.
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || filepath.Base(path) == "env" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "config" {
			return walkErr
		}
		if contents, readErr := os.ReadFile(path); readErr == nil && strings.Contains(string(contents), cliEnvSentinel) {
			t.Errorf("%s contains a stored value", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSkillEnvValueIsNeverAnArgument(t *testing.T) {
	root := initTestWorkspace(t)
	createActiveDoctorSkill(t, root, "env-args", "")
	for _, args := range [][]string{
		{"skill", "env", "set", "env-args", "KEY_ONE", cliEnvSentinel},
		{"skill", "env", "set", "env-args"},
		{"skill", "env", "list"},
		{"skill", "env", "list", "env-args", "extra"},
		{"skill", "env", "rotate", "env-args", "KEY_ONE"},
		{"skill", "env"},
	} {
		code, stdout, stderr := runCLI(t, append(args, "--workspace", root)...)
		if code != 2 || strings.Contains(stdout+stderr, cliEnvSentinel) {
			t.Fatalf("%v exit = %d, output = %s%s", args, code, stdout, stderr)
		}
	}
	withEnvInput(t, "value")
	if code, _, stderr := runCLI(t, "skill", "env", "set", "env-args", "bad-name", "--workspace", root); code != 2 {
		t.Fatalf("invalid key exit = %d: %s", code, stderr)
	}
	withEnvInput(t, "")
	if code, _, stderr := runCLI(t, "skill", "env", "set", "env-args", "KEY_ONE", "--workspace", root); code != 2 {
		t.Fatalf("empty value exit = %d: %s", code, stderr)
	}
}
