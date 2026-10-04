package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// fakeDoctorRunner answers version probes from a fixed table and records
// every command it is asked to run.
type fakeDoctorRunner struct {
	mu       sync.Mutex
	versions map[string]string
	checkRC  int
	commands []skillruntime.Command
}

func (runner *fakeDoctorRunner) Run(_ context.Context, command skillruntime.Command) (skillruntime.CommandResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.commands = append(runner.commands, command)
	if len(command.Args) == 1 && command.Args[0] == "--version" {
		return skillruntime.CommandResult{Output: []byte(runner.versions[filepath.Base(command.Path)])}, nil
	}
	return skillruntime.CommandResult{Output: []byte("check ran\n"), ExitCode: runner.checkRC}, nil
}

func (runner *fakeDoctorRunner) ranSetupCheck() bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for _, command := range runner.commands {
		if len(command.Args) == 0 || command.Args[0] != "--version" {
			return true
		}
	}
	return false
}

func doctorRuntime(bins []any, platforms []any, check string) func(map[string]any) {
	return func(document map[string]any) {
		requires := map[string]any{"bins": bins, "env": []any{"DOCTOR_TEST_TOKEN"}}
		if platforms != nil {
			requires["platforms"] = platforms
		}
		document["runtime"] = map[string]any{
			"requires": requires,
			"setup":    map[string]any{"command": "npm install", "check": check},
		}
	}
}

func fakeDoctorService(runner skillruntime.Runner, sink TelemetrySink) SkillDoctorService {
	return SkillDoctorService{
		Telemetry: sink,
		snapshots: fakeSnapshotService(),
		options: skillruntime.DoctorOptions{
			Runner:    runner,
			LookPath:  func(name string) (string, error) { return "/usr/bin/" + name, nil },
			LookupEnv: func(string) (string, bool) { return "secret-value", true },
			GOOS:      "linux",
			Now:       func() time.Time { return time.Date(2026, 10, 4, 9, 30, 0, 0, time.UTC) },
		},
	}
}

func newDoctorSkill(t *testing.T, id string, mutate ...func(map[string]any)) string {
	t.Helper()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, id, "Doctor Skill")
	writeSkillFile(t, root, id, "scripts/check.sh", "#!/bin/sh\nexit 0\n")
	updateSkillMeta(t, root, id, func(document map[string]any) {
		for _, apply := range mutate {
			apply(document)
		}
	})
	return root
}

func TestDoctorWritesCacheThatActivationAndResolutionRead(t *testing.T) {
	t.Parallel()
	root := newDoctorSkill(t, "doctor-ready", doctorRuntime([]any{map[string]any{"name": "node", "version": ">=18"}}, nil, "sh scripts/check.sh"))
	runner := &fakeDoctorRunner{versions: map[string]string{"node": "v20.11.1\n"}}
	result, err := fakeDoctorService(runner, nil).Run(t.Context(), root, "doctor-ready")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != skillruntime.StateReady || result.Status != StatusReady || !runner.ranSetupCheck() {
		t.Fatalf("result = %#v", result)
	}
	if result.CheckedAt != "2026-10-04T09:30:00Z" || result.CheckOutput != "check ran\n" || result.SetupCommand != "" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(result.CachePath); err != nil {
		t.Fatalf("doctor cache missing: %v", err)
	}

	local := ensureSnapshot(t, fakeSnapshotService(), root, "doctor-ready")
	if local.Preflight == nil || local.Preflight.Doctor == nil || local.Preflight.Doctor.State != skillruntime.StateReady || local.Preflight.State != skillruntime.StateReady {
		t.Fatalf("activation preflight did not read the doctor result: %#v", local.Preflight)
	}
	if local.Preflight.Doctor.CheckedAt != result.CheckedAt || local.ManifestVersion != result.ManifestVersion {
		t.Fatalf("preflight doctor = %#v, result = %#v", local.Preflight.Doctor, result)
	}

	if result.Basis != skillruntime.BasisTerminal || local.Preflight.Doctor.Basis != skillruntime.BasisTerminal {
		t.Fatalf("doctor results must carry the terminal basis: %#v / %#v", result, local.Preflight.Doctor)
	}

	contentJSON := readCatalogContentJSON(t, root, "doctor-ready")
	setup, err := setupAnnotation(root, lookupDistributedEntry(t, root, "doctor-ready"), contentJSON, runtimeProbe{goos: "linux"})
	if err != nil || setup == nil || setup.State != skillruntime.StateReady || setup.CheckedAt != result.CheckedAt || setup.Basis != skillruntime.BasisTerminal {
		t.Fatalf("resolver setup annotation = %#v, %v", setup, err)
	}
}

func TestDoctorRunsCheckInStateDirectoryWithSkillEnvironment(t *testing.T) {
	t.Parallel()
	root := newDoctorSkill(t, "doctor-env-dirs", doctorRuntime([]any{"node"}, nil, "sh scripts/check.sh"))
	runner := &fakeDoctorRunner{}
	result, err := fakeDoctorService(runner, nil).Run(t.Context(), root, "doctor-env-dirs")
	if err != nil {
		t.Fatal(err)
	}
	local := ensureSnapshot(t, fakeSnapshotService(), root, "doctor-env-dirs")
	if local.Preflight == nil || local.Preflight.StateDirectory == "" {
		t.Fatalf("preflight = %#v", local.Preflight)
	}
	if result.WorkingDirectory != local.Preflight.StateDirectory {
		t.Fatalf("working directory = %q, want the state directory %q", result.WorkingDirectory, local.Preflight.StateDirectory)
	}
	var check *skillruntime.Command
	for index, command := range runner.commands {
		if len(command.Args) == 0 || command.Args[0] != "--version" {
			check = &runner.commands[index]
		}
	}
	if check == nil {
		t.Fatal("setup check did not run")
	}
	if check.Dir != local.Preflight.StateDirectory {
		t.Fatalf("check ran in %q, want %q", check.Dir, local.Preflight.StateDirectory)
	}
	wantEnv := []string{skillruntime.EnvSkillDir + "=" + local.Path, skillruntime.EnvStateDir + "=" + local.Preflight.StateDirectory, skillruntime.EnvConfigDir + "=" + local.Preflight.Env[skillruntime.EnvConfigDir]}
	for _, want := range wantEnv {
		if !slices.Contains(check.Env, want) {
			t.Fatalf("check env = %v, missing %q", check.Env, want)
		}
	}
}

func TestDoctorReportsMissingBinAndOldVersion(t *testing.T) {
	t.Parallel()
	root := newDoctorSkill(t, "doctor-bins", doctorRuntime([]any{"jq", map[string]any{"name": "node", "version": ">=18"}}, nil, "sh scripts/check.sh"))
	runner := &fakeDoctorRunner{versions: map[string]string{"node": "v16.2.0\n"}}
	service := fakeDoctorService(runner, nil)
	service.options.LookPath = func(name string) (string, error) {
		if name == "jq" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}
	result, err := service.Run(t.Context(), root, "doctor-bins")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != skillruntime.StateSetupRequired || result.Status != StatusActionRequired || result.SetupCommand != "npm install" {
		t.Fatalf("result = %#v", result)
	}
	details := map[string]skillruntime.Check{}
	for _, check := range result.Checks {
		details[check.Name] = check
	}
	if details["jq"].Status != skillruntime.StatusFail || details["jq"].Detail != skillruntime.DetailNotFound {
		t.Fatalf("jq check = %#v", details["jq"])
	}
	if details["node"].Status != skillruntime.StatusFail || details["node"].Detail != "version 16.2.0 does not satisfy >=18" {
		t.Fatalf("node check = %#v", details["node"])
	}
	cached, ok, err := skillruntime.ReadCache(root, "doctor-bins", cacheFingerprintFor(t, root, "doctor-bins", result.ManifestVersion))
	if err != nil || !ok || cached.State != skillruntime.StateSetupRequired {
		t.Fatalf("cached = %#v, %v, %v", cached, ok, err)
	}
}

func TestDoctorUnsupportedPlatformSkipsCheck(t *testing.T) {
	t.Parallel()
	root := newDoctorSkill(t, "doctor-platform", doctorRuntime([]any{"node"}, []any{"windows"}, "sh scripts/check.sh"))
	runner := &fakeDoctorRunner{}
	result, err := fakeDoctorService(runner, nil).Run(t.Context(), root, "doctor-platform")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != skillruntime.StateUnsupportedPlatform || runner.ranSetupCheck() || result.SetupCommand != "" {
		t.Fatalf("result = %#v, commands = %#v", result, runner.commands)
	}
}

func TestDoctorSkipsCheckForUnreviewedThirdPartySkill(t *testing.T) {
	t.Parallel()
	root := newDoctorSkill(t, "doctor-third", markThirdParty, doctorRuntime([]any{"node"}, nil, "sh scripts/check.sh"))
	runner := &fakeDoctorRunner{}
	sink := &captureTelemetrySink{}
	result, err := fakeDoctorService(runner, sink).Run(t.Context(), root, "doctor-third")
	if err != nil {
		t.Fatal(err)
	}
	if runner.ranSetupCheck() {
		t.Fatal("setup check ran for an unreviewed third-party skill")
	}
	if result.State != skillruntime.StateSetupRequired || result.SetupCommand != "" || result.WorkingDirectory != "" || result.CachePath != "" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "cache", "doctor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a result for unapproved content must not be cached: %v", err)
	}
	last := result.Checks[len(result.Checks)-1]
	if last.Kind != skillruntime.KindSetupCheck || last.Status != skillruntime.StatusSkipped || last.Detail != skillruntime.ReasonContentReviewRequired {
		t.Fatalf("setup check = %#v", last)
	}
	if !slices.Contains(result.ReasonCodes, skillruntime.ReasonContentReviewRequired) {
		t.Fatalf("reason codes = %#v", result.ReasonCodes)
	}
	if len(result.SuggestedActions) != 1 || result.SuggestedActions[0].Command != "skillhub skill review doctor-third" {
		t.Fatalf("suggested actions = %#v", result.SuggestedActions)
	}

	if len(sink.events) != 1 || sink.events[0].Type != telemetry.EventSkillDoctorChecked {
		t.Fatalf("telemetry events = %#v", sink.events)
	}
	event := sink.events[0]
	if event.CatalogSnapshot == "" || event.PolicyRevision == "" {
		t.Fatalf("doctor event is not pinned to the catalog: %#v", event)
	}
	for key := range event.Payload {
		if !slices.Contains([]string{"skill_id", "status", "reason_codes", "duration_ms"}, key) {
			t.Fatalf("doctor event has unexpected payload field %q: %#v", key, event.Payload)
		}
	}
	codes, _ := event.Payload["reason_codes"].([]any)
	if event.Payload["skill_id"] != "doctor-third" || event.Payload["status"] != skillruntime.StateSetupRequired || !slices.Contains(codes, any("setup_check_skipped")) || !slices.Contains(codes, any(skillruntime.ReasonContentReviewRequired)) {
		t.Fatalf("doctor payload = %#v", event.Payload)
	}
}

func TestDoctorCacheAndTelemetryNeverContainEnvValues(t *testing.T) {
	t.Parallel()
	root := newDoctorSkill(t, "doctor-env", doctorRuntime([]any{"node"}, nil, "sh scripts/check.sh"))
	sink := &captureTelemetrySink{}
	result, err := fakeDoctorService(&fakeDoctorRunner{}, sink).Run(t.Context(), root, "doctor-env")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-value") || strings.Contains(string(data), "check ran") {
		t.Fatalf("doctor cache contains env values or check output: %s", data)
	}
	if !strings.Contains(string(data), "DOCTOR_TEST_TOKEN") {
		t.Fatalf("doctor cache lacks the env presence check: %s", data)
	}
	for _, check := range result.Checks {
		if strings.Contains(check.Detail, "secret-value") {
			t.Fatalf("check leaks env value: %#v", check)
		}
	}
	for _, event := range sink.events {
		for _, value := range event.Payload {
			if strings.Contains(strings.ToLower(toText(value)), "secret") || strings.Contains(toText(value), "DOCTOR_TEST_TOKEN") {
				t.Fatalf("doctor telemetry leaks env data: %#v", event.Payload)
			}
		}
	}
}

func TestDoctorWithoutRuntimeIsReadyAndWritesNothing(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "doctor-plain", "Doctor Plain")
	sink := &captureTelemetrySink{}
	result, err := fakeDoctorService(&fakeDoctorRunner{}, sink).Run(t.Context(), root, "doctor-plain")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != skillruntime.StateReady || result.Note != DoctorNoRuntime || result.CachePath != "" || len(result.Checks) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "cache", "doctor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("doctor cache directory exists: %v", err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("telemetry events = %#v", sink.events)
	}
}

func TestDoctorRejectsUnknownAndInvalidSkills(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	service := fakeDoctorService(&fakeDoctorRunner{}, nil)
	if _, err := service.Run(t.Context(), root, "missing-skill"); !errors.Is(err, ErrDoctorSkillUnavailable) {
		t.Fatalf("unknown skill error = %v", err)
	}
	var structured *Error
	if _, err := service.Run(t.Context(), root, "../escape"); !errors.As(err, &structured) || structured.Code != ErrorInvalidRequest {
		t.Fatalf("invalid id error = %v", err)
	}
}

func TestDoctorRunsRealSetupCheckCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("setup check commands use sh")
	}
	t.Parallel()
	for _, test := range []struct {
		check  string
		state  string
		detail string
	}{
		{check: "exit 0", state: skillruntime.StateReady},
		{check: "echo failing >&2; exit 3", state: skillruntime.StateSetupRequired, detail: "exit status 3"},
	} {
		root := newDoctorSkill(t, "doctor-shell", doctorRuntime([]any{"sh"}, nil, test.check))
		service := SkillDoctorService{snapshots: fakeSnapshotService(), options: skillruntime.DoctorOptions{LookupEnv: func(string) (string, bool) { return "x", true }}}
		result, err := service.Run(t.Context(), root, "doctor-shell")
		if err != nil {
			t.Fatal(err)
		}
		last := result.Checks[len(result.Checks)-1]
		if result.State != test.state || last.Kind != skillruntime.KindSetupCheck || last.Detail != test.detail {
			t.Fatalf("check %q: result = %#v", test.check, result)
		}
		if test.detail != "" && !strings.Contains(result.CheckOutput, "failing") {
			t.Fatalf("check output = %q", result.CheckOutput)
		}
	}
}

func TestTailTextKeepsUTF8Boundary(t *testing.T) {
	t.Parallel()
	if got := tailText("short", 10); got != "short" {
		t.Fatalf("tailText = %q", got)
	}
	if got := tailText("aé", 1); got != "" {
		t.Fatalf("tailText split a rune: %q", got)
	}
	if got := tailText(strings.Repeat("x", 3000)+"END", doctorCheckOutputTail); len(got) != doctorCheckOutputTail || !strings.HasSuffix(got, "END") {
		t.Fatalf("tailText length = %d", len(got))
	}
}

func readCatalogContentJSON(t *testing.T, root, id string) []byte {
	t.Helper()
	_, source, err := loadDistributedSkillSource(t.Context(), root, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	return source.ContentJSON
}

func cacheFingerprintFor(t *testing.T, root, id, manifestVersion string) string {
	t.Helper()
	spec, _, err := skillruntime.ParseSpec(readCatalogContentJSON(t, root, id))
	if err != nil {
		t.Fatal(err)
	}
	return doctorFingerprint(manifestVersion, spec)
}

func toText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, toText(item))
		}
		return strings.Join(parts, ",")
	}
	return ""
}

func lookupDistributedEntry(t *testing.T, root, id string) DistributedSkill {
	t.Helper()
	entries, _, err := (DistributionService{}).LookupSkills(t.Context(), root, []string{id})
	if err != nil {
		t.Fatal(err)
	}
	return entries[id]
}
