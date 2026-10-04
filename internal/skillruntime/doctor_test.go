package skillruntime

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	results map[string]CommandResult
	errs    map[string]error
	block   map[string]bool
	calls   []Command
}

func (f *fakeRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	f.calls = append(f.calls, command)
	key := command.Path
	if command.Path == "sh" || command.Path == "cmd" {
		key = "shell"
	}
	if f.block[key] {
		<-ctx.Done()
		return CommandResult{}, ctx.Err()
	}
	if err := f.errs[key]; err != nil {
		return CommandResult{}, err
	}
	return f.results[key], nil
}

func doctorOptions(runner Runner, env map[string]string, found ...string) DoctorOptions {
	return DoctorOptions{
		Runner:         runner,
		LookPath:       fakeLook(found...),
		LookupEnv:      fakeEnv(env),
		GOOS:           "linux",
		Now:            func() time.Time { return time.Date(2026, 10, 4, 9, 0, 0, 0, time.FixedZone("ICT", 7*3600)) },
		VersionTimeout: 20 * time.Millisecond,
		CheckTimeout:   20 * time.Millisecond,
	}
}

func findCheck(t *testing.T, result Result, kind, name string) Check {
	t.Helper()
	for _, check := range result.Checks {
		if check.Kind == kind && check.Name == name {
			return check
		}
	}
	t.Fatalf("no %s check %q in %+v", kind, name, result.Checks)
	return Check{}
}

func TestRunDoctorReady(t *testing.T) {
	runner := &fakeRunner{results: map[string]CommandResult{
		"/usr/bin/node": {Output: []byte("v20.11.0\n")},
		"shell":         {Output: []byte("ok")},
	}}
	spec := Spec{
		Requires: Requires{Bins: []Bin{{Name: "node", Version: ">=18"}, {Name: "git"}}, Env: []string{"API_TOKEN"}, Platforms: []string{"linux"}},
		Setup:    Setup{Check: "node -e 1"},
	}
	result := RunDoctor(context.Background(), spec, "/snapshot", nil, true, doctorOptions(runner, map[string]string{"API_TOKEN": secretSentinel}, "node", "git"))
	if result.State != StateReady {
		t.Fatalf("state = %s, checks %+v", result.State, result.Checks)
	}
	if !result.CheckedAt.Equal(time.Date(2026, 10, 4, 2, 0, 0, 0, time.UTC)) || result.CheckedAt.Location() != time.UTC {
		t.Fatalf("checked_at = %v", result.CheckedAt)
	}
	if got := findCheck(t, result, KindBin, "node"); got.Detail != "version 20.11.0 satisfies >=18" {
		t.Fatalf("node check = %+v", got)
	}
	if result.CheckOutput != "ok" {
		t.Fatalf("check output = %q", result.CheckOutput)
	}
	var probe, shell Command
	for _, call := range runner.calls {
		if call.Path == "/usr/bin/node" {
			probe = call
		} else {
			shell = call
		}
	}
	if len(probe.Args) != 1 || probe.Args[0] != "--version" {
		t.Fatalf("version probe = %+v", probe)
	}
	if shell.Dir != "/snapshot" || shell.Args[len(shell.Args)-1] != "node -e 1" {
		t.Fatalf("shell command = %+v", shell)
	}
	for _, check := range result.Checks {
		if strings.Contains(check.Detail, secretSentinel) {
			t.Fatal("env value leaked into doctor checks")
		}
	}
}

func TestRunDoctorUnsupportedPlatformShortCircuits(t *testing.T) {
	runner := &fakeRunner{}
	spec := Spec{Requires: Requires{Platforms: []string{"windows"}, Bins: []Bin{{Name: "node", Version: ">=18"}}}, Setup: Setup{Check: "true"}}
	result := RunDoctor(context.Background(), spec, "/snapshot", nil, true, doctorOptions(runner, nil, "node"))
	if result.State != StateUnsupportedPlatform || len(result.Checks) != 1 || len(runner.calls) != 0 {
		t.Fatalf("result = %+v, calls %d", result, len(runner.calls))
	}
}

func TestRunDoctorVersionFailures(t *testing.T) {
	tests := []struct {
		name   string
		runner *fakeRunner
		detail string
	}{
		{name: "too old", runner: &fakeRunner{results: map[string]CommandResult{"/usr/bin/node": {Output: []byte("v16.2.0")}}}, detail: "version 16.2.0 does not satisfy >=18"},
		{name: "unparsable", runner: &fakeRunner{results: map[string]CommandResult{"/usr/bin/node": {Output: []byte("unknown option")}}}, detail: DetailVersionUnparsable},
		{name: "timeout", runner: &fakeRunner{block: map[string]bool{"/usr/bin/node": true}}, detail: DetailTimeout},
		{name: "start failure", runner: &fakeRunner{errs: map[string]error{"/usr/bin/node": errors.New("exec format error")}}, detail: DetailVersionProbeFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := Spec{Requires: Requires{Bins: []Bin{{Name: "node", Version: ">=18"}}}}
			result := RunDoctor(context.Background(), spec, "", nil, false, doctorOptions(tt.runner, nil, "node"))
			check := findCheck(t, result, KindBin, "node")
			if result.State != StateSetupRequired || check.Status != StatusFail || check.Detail != tt.detail {
				t.Fatalf("state %s, check %+v", result.State, check)
			}
		})
	}
}

func TestRunDoctorNonZeroExitStillParsesVersion(t *testing.T) {
	runner := &fakeRunner{results: map[string]CommandResult{"/usr/bin/tool": {Output: []byte("tool 2.1"), ExitCode: 1}}}
	spec := Spec{Requires: Requires{Bins: []Bin{{Name: "tool", Version: "2"}}}}
	if result := RunDoctor(context.Background(), spec, "", nil, false, doctorOptions(runner, nil, "tool")); result.State != StateReady {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunDoctorMissingBinSkipsProbe(t *testing.T) {
	runner := &fakeRunner{}
	spec := Spec{Requires: Requires{Bins: []Bin{{Name: "node", Version: ">=18"}}}}
	result := RunDoctor(context.Background(), spec, "", nil, false, doctorOptions(runner, nil))
	if check := findCheck(t, result, KindBin, "node"); check.Detail != DetailNotFound || len(runner.calls) != 0 {
		t.Fatalf("check %+v, calls %d", check, len(runner.calls))
	}
}

func TestRunDoctorSetupCheck(t *testing.T) {
	tests := []struct {
		name       string
		runner     *fakeRunner
		allowCheck bool
		workDir    string
		status     string
		detail     string
		state      string
		ran        bool
	}{
		{name: "untrusted is skipped", runner: &fakeRunner{}, workDir: "/snapshot", status: StatusSkipped, detail: ReasonContentReviewRequired, state: StateSetupRequired},
		{name: "non-zero exit", runner: &fakeRunner{results: map[string]CommandResult{"shell": {ExitCode: 3}}}, allowCheck: true, workDir: "/snapshot", status: StatusFail, detail: "exit status 3", state: StateSetupRequired, ran: true},
		{name: "timeout", runner: &fakeRunner{block: map[string]bool{"shell": true}}, allowCheck: true, workDir: "/snapshot", status: StatusFail, detail: DetailTimeout, state: StateSetupRequired, ran: true},
		{name: "missing work dir", runner: &fakeRunner{}, allowCheck: true, status: StatusFail, detail: DetailWorkDirRequired, state: StateSetupRequired},
		{name: "pass", runner: &fakeRunner{}, allowCheck: true, workDir: "/snapshot", status: StatusPass, state: StateReady, ran: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := Spec{Setup: Setup{Command: "npm ci", Check: "test -d node_modules"}}
			result := RunDoctor(context.Background(), spec, tt.workDir, nil, tt.allowCheck, doctorOptions(tt.runner, nil))
			check := findCheck(t, result, KindSetupCheck, "check")
			if check.Status != tt.status || check.Detail != tt.detail || result.State != tt.state {
				t.Fatalf("state %s, check %+v", result.State, check)
			}
			if ran := len(tt.runner.calls) > 0; ran != tt.ran {
				t.Fatalf("shell ran = %v, want %v", ran, tt.ran)
			}
		})
	}
}

func TestExecRunnerShellSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	dir := t.TempDir()
	spec := Spec{Setup: Setup{Check: "exit 0"}}
	result := RunDoctor(context.Background(), spec, dir, nil, true, DoctorOptions{})
	if result.State != StateReady {
		t.Fatalf("result = %+v", result)
	}
	out, err := ExecRunner{}.Run(context.Background(), shellCommand("printf %0100000d 0; exit 4", dir, nil))
	if err != nil || out.ExitCode != 4 || len(out.Output) != MaxCommandOutput {
		t.Fatalf("exit %d, output %d bytes, err %v", out.ExitCode, len(out.Output), err)
	}
	var capped cappedBuffer
	capped.limit = 4
	if n, err := capped.Write([]byte("abcdef")); n != 6 || err != nil || string(capped.Bytes()) != "abcd" {
		t.Fatalf("capped write = %d, %v, %q", n, err, capped.Bytes())
	}
}

func TestRunDoctorCheckSeesSkillEnvironmentAndBasis(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	dir := t.TempDir()
	extra := map[string]string{EnvSkillDir: "/skill/dir", EnvStateDir: dir}
	spec := Spec{}
	spec.Setup.Check = `test "$SKILLHUB_SKILL_DIR" = /skill/dir && test -d "$SKILLHUB_STATE_DIR" && test "$(cd "$SKILLHUB_STATE_DIR" && pwd -P)" = "$(pwd -P)"`
	result := RunDoctor(context.Background(), spec, dir, extra, true, DoctorOptions{})
	if result.State != StateReady || result.Basis != BasisTerminal {
		t.Fatalf("result = %+v", result)
	}
	// Without the variables the same check fails, proving they came from extraEnv.
	t.Setenv(EnvSkillDir, "")
	failing := RunDoctor(context.Background(), spec, dir, nil, true, DoctorOptions{})
	if failing.State != StateSetupRequired {
		t.Fatalf("check passed without the skill variables: %+v", failing)
	}
}
