package skillruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"time"
)

// Doctor states.
const (
	StateReady               = "ready"
	StateSetupRequired       = "setup_required"
	StateUnsupportedPlatform = "unsupported_platform"
	StateUnknown             = "unknown"
)

// Defaults for command execution in the doctor.
const (
	DefaultVersionTimeout = 5 * time.Second
	DefaultCheckTimeout   = 60 * time.Second
	MaxCommandOutput      = 8 << 10
)

// Check details produced by the doctor.
const (
	DetailTimeout            = "timeout"
	DetailVersionProbeFailed = "version_probe_failed"
	DetailWorkDirRequired    = "work_dir_required"
)

// Command is one process invocation. It never goes through a shell unless the
// caller names the shell explicitly as Path.
type Command struct {
	Path string
	Args []string
	Dir  string
	// Env holds KEY=VALUE entries added to the inherited environment.
	Env []string
}

// CommandResult is the outcome of a finished process. Output is stdout and
// stderr combined, capped at MaxCommandOutput bytes.
type CommandResult struct {
	Output   []byte
	ExitCode int
}

// Runner executes commands. A non-nil error means the process could not be
// started or did not finish (including context cancellation); a non-zero exit
// is reported through ExitCode with a nil error.
type Runner interface {
	Run(ctx context.Context, command Command) (CommandResult, error)
}

// ExecRunner runs commands with os/exec: closed stdin, inherited environment,
// and output capped at MaxCommandOutput.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	if len(command.Env) > 0 {
		cmd.Env = append(os.Environ(), command.Env...)
	}
	cmd.Stdin = nil
	output := &cappedBuffer{limit: MaxCommandOutput}
	cmd.Stdout = output
	cmd.Stderr = output
	// A child that keeps the output pipes open must not hang the doctor
	// after the context has killed the direct process.
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	result := CommandResult{Output: output.Bytes()}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

// cappedBuffer must not embed bytes.Buffer: a promoted ReadFrom would let
// io.Copy bypass the cap.
type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

// Write keeps at most limit bytes and reports the full length as written so
// the process is never blocked or failed by the cap.
func (b *cappedBuffer) Write(p []byte) (int, error) {
	written := len(p)
	if remaining := b.limit - b.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.buf.Write(p)
	}
	return written, nil
}

func (b *cappedBuffer) Bytes() []byte { return b.buf.Bytes() }

// DoctorOptions injects the environment for RunDoctor. Zero values use the
// real process environment and defaults.
type DoctorOptions struct {
	Runner         Runner
	LookPath       LookPath
	LookupEnv      LookupEnv
	GOOS           string
	Now            func() time.Time
	VersionTimeout time.Duration
	CheckTimeout   time.Duration
}

func (o DoctorOptions) withDefaults() DoctorOptions {
	if o.Runner == nil {
		o.Runner = ExecRunner{}
	}
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	if o.LookupEnv == nil {
		o.LookupEnv = os.LookupEnv
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.VersionTimeout <= 0 {
		o.VersionTimeout = DefaultVersionTimeout
	}
	if o.CheckTimeout <= 0 {
		o.CheckTimeout = DefaultCheckTimeout
	}
	return o
}

// BasisTerminal labels doctor results: they describe the terminal that ran the
// doctor, which may differ from the shell an agent uses.
const BasisTerminal = "terminal"

// Result is a doctor run. CheckOutput holds the capped output of the setup
// check command for the caller only; it is never written to the cache because
// it may echo secrets.
type Result struct {
	State       string    `json:"state"`
	Basis       string    `json:"basis"`
	Checks      []Check   `json:"checks"`
	CheckedAt   time.Time `json:"checked_at"`
	CheckOutput string    `json:"-"`
}

// RunDoctor performs the full, executing machine check for a skill in the
// caller's terminal environment. It is for the explicit CLI doctor only, never
// the MCP flow. Version constraints are probed with "<bin> --version" (no
// shell). The setup check command runs in workDir through the platform shell
// with extraEnv added to the inherited environment, and only when allowCheck
// is true, which the caller sets only after the trust verdict passed.
func RunDoctor(ctx context.Context, spec Spec, workDir string, extraEnv map[string]string, allowCheck bool, opts DoctorOptions) Result {
	opts = opts.withDefaults()
	result := Result{CheckedAt: opts.Now().UTC(), Basis: BasisTerminal}
	if platform, ok := platformCheck(spec, opts.GOOS); ok {
		result.Checks = append(result.Checks, platform)
		if platform.Status == StatusFail {
			result.State = StateUnsupportedPlatform
			return result
		}
	}
	for _, bin := range spec.Requires.Bins {
		result.Checks = append(result.Checks, binCheck(ctx, bin, opts))
	}
	result.Checks = append(result.Checks, envChecks(spec, opts.LookupEnv)...)
	if spec.Setup.Check != "" {
		check, output := setupCheck(ctx, spec.Setup.Check, workDir, extraEnv, allowCheck, opts)
		result.Checks = append(result.Checks, check)
		result.CheckOutput = output
	}
	result.State = StateReady
	for _, check := range result.Checks {
		// A skipped check means the setup could not be verified until the
		// scripts are reviewed, which is still an action the user must take.
		if check.Status != StatusPass {
			result.State = StateSetupRequired
			break
		}
	}
	return result
}

func binCheck(ctx context.Context, bin Bin, opts DoctorOptions) Check {
	check := Check{Kind: KindBin, Name: bin.Name, Status: StatusFail}
	resolved, err := opts.LookPath(bin.Name)
	if err != nil {
		check.Detail = DetailNotFound
		return check
	}
	if bin.Version == "" {
		check.Status = StatusPass
		return check
	}
	want, err := parseConstraint(bin.Version)
	if err != nil {
		check.Detail = "invalid_constraint"
		return check
	}
	probeCtx, cancel := context.WithTimeout(ctx, opts.VersionTimeout)
	defer cancel()
	// Many tools exit non-zero for --version yet still print it, so the exit
	// code is ignored and only the output is parsed.
	probe, err := opts.Runner.Run(probeCtx, Command{Path: resolved, Args: []string{"--version"}})
	if err != nil {
		check.Detail = DetailVersionProbeFailed
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			check.Detail = DetailTimeout
		}
		return check
	}
	token, have, ok := extractVersion(string(probe.Output))
	if !ok {
		check.Detail = DetailVersionUnparsable
		return check
	}
	if !want.satisfiedBy(have) {
		check.Detail = fmt.Sprintf("version %s does not satisfy %s", token, want.text)
		return check
	}
	check.Status = StatusPass
	check.Detail = fmt.Sprintf("version %s satisfies %s", token, want.text)
	return check
}

func setupCheck(ctx context.Context, command, workDir string, extraEnv map[string]string, allowCheck bool, opts DoctorOptions) (Check, string) {
	check := Check{Kind: KindSetupCheck, Name: "check", Status: StatusFail}
	if !allowCheck {
		check.Status = StatusSkipped
		check.Detail = ReasonContentReviewRequired
		return check, ""
	}
	if workDir == "" {
		check.Detail = DetailWorkDirRequired
		return check, ""
	}
	runCtx, cancel := context.WithTimeout(ctx, opts.CheckTimeout)
	defer cancel()
	result, err := opts.Runner.Run(runCtx, shellCommand(command, workDir, extraEnv))
	output := string(result.Output)
	switch {
	case err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded)):
		check.Detail = DetailTimeout
	case err != nil:
		check.Detail = "start_failed"
	case result.ExitCode != 0:
		check.Detail = fmt.Sprintf("exit status %d", result.ExitCode)
	default:
		check.Status = StatusPass
	}
	return check, output
}

// shellCommand selects the shell of the machine actually running the doctor.
func shellCommand(command, workDir string, extraEnv map[string]string) Command {
	env := make([]string, 0, len(extraEnv))
	for key, value := range extraEnv {
		env = append(env, key+"="+value)
	}
	slices.Sort(env)
	if runtime.GOOS == "windows" {
		return Command{Path: "cmd", Args: []string{"/C", command}, Dir: workDir, Env: env}
	}
	return Command{Path: "sh", Args: []string{"-c", command}, Dir: workDir, Env: env}
}
