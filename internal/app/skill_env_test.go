package app

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

const envSentinel = "sentinel-secret-7f3a91"

func configEnvPath(root, id string) string {
	return filepath.Join(root, "runtime", "config", id, "env")
}

func TestSkillEnvSetListUnsetKeepsOtherKeysAndHidesValues(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "env-skill", "Env Skill")
	service := SkillEnvService{}

	if _, err := service.Set(t.Context(), root, "env-skill", "API_TOKEN", envSentinel); err != nil {
		t.Fatal(err)
	}
	awkward := "it's a \"value\" with $vars and spaces"
	result, err := service.Set(t.Context(), root, "env-skill", "OTHER_KEY", awkward)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Keys, []string{"API_TOKEN", "OTHER_KEY"}) {
		t.Fatalf("keys = %v", result.Keys)
	}
	if encoded, _ := json.Marshal(result); strings.Contains(string(encoded), envSentinel) || strings.Contains(string(encoded), "spaces") {
		t.Fatalf("result leaks a value: %s", encoded)
	}

	entries, err := loadSkillConfigEnv(root, "env-skill")
	if err != nil || entries["API_TOKEN"] != envSentinel || entries["OTHER_KEY"] != awkward {
		t.Fatalf("round trip = %v, %v", entries, err)
	}
	if runtime.GOOS != "windows" {
		file, err := os.Stat(configEnvPath(root, "env-skill"))
		if err != nil || file.Mode().Perm() != 0o600 {
			t.Fatalf("env file mode = %v, %v", file, err)
		}
		dir, err := os.Stat(filepath.Dir(configEnvPath(root, "env-skill")))
		if err != nil || dir.Mode().Perm() != 0o700 {
			t.Fatalf("config dir mode = %v, %v", dir, err)
		}
	}

	listed, err := service.List(t.Context(), root, "env-skill")
	if err != nil || !slices.Equal(listed.Keys, []string{"API_TOKEN", "OTHER_KEY"}) {
		t.Fatalf("list = %#v, %v", listed, err)
	}
	if _, err := service.Unset(t.Context(), root, "env-skill", "API_TOKEN"); err != nil {
		t.Fatal(err)
	}
	entries, err = loadSkillConfigEnv(root, "env-skill")
	if err != nil || len(entries) != 1 || entries["OTHER_KEY"] != awkward {
		t.Fatalf("after unset = %v, %v", entries, err)
	}
	if _, err := service.Unset(t.Context(), root, "env-skill", "NEVER_SET"); err != nil {
		t.Fatalf("unsetting an absent key must succeed: %v", err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(configEnvPath(root, "env-skill")), ".env-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

func TestSkillEnvRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "env-bad", "Env Bad")
	service := SkillEnvService{}
	for _, test := range []struct{ id, key, value string }{
		{"env-bad", "1BAD", "v"},
		{"env-bad", "has-dash", "v"},
		{"env-bad", "SKILLHUB_STATE_DIR", "v"},
		{"env-bad", "OK_KEY", ""},
		{"env-bad", "OK_KEY", "line1\nline2"},
		{"../escape", "OK_KEY", "v"},
		{"missing-skill", "OK_KEY", "v"},
	} {
		if _, err := service.Set(t.Context(), root, test.id, test.key, test.value); err == nil {
			t.Fatalf("Set(%q, %q, %q) succeeded", test.id, test.key, test.value)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "config", "missing-skill")); !os.IsNotExist(err) {
		t.Fatalf("a config directory was created for an unknown skill: %v", err)
	}
}

func TestSkillEnvRefusesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "env-link", "Env Link")
	service := SkillEnvService{}
	if _, err := service.Set(t.Context(), root, "env-link", "KEY_ONE", "one"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(outside, []byte("KEY_TWO=two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := configEnvPath(root, "env-link")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, file); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Set(t.Context(), root, "env-link", "KEY_ONE", "one"); err == nil {
		t.Fatal("Set followed a symlinked env file")
	}
	if _, err := service.List(t.Context(), root, "env-link"); err == nil {
		t.Fatal("List followed a symlinked env file")
	}
	if _, err := loadSkillConfigEnv(root, "env-link"); err == nil {
		t.Fatal("the doctor loader followed a symlinked env file")
	}
	if contents, _ := os.ReadFile(outside); string(contents) != "KEY_TWO=two\n" {
		t.Fatalf("symlink target was modified: %q", contents)
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(file)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Set(t.Context(), root, "env-link", "KEY_ONE", "one"); err == nil {
		t.Fatal("Set followed a symlinked config directory")
	}
}

func TestSkillEnvRejectsMalformedFileWithoutEchoingContent(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "env-malformed", "Env Malformed")
	service := SkillEnvService{}
	if _, err := service.Set(t.Context(), root, "env-malformed", "KEY_ONE", "one"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configEnvPath(root, "env-malformed"), []byte("KEY_ONE=one\nnot a valid line "+envSentinel+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := service.Set(t.Context(), root, "env-malformed", "KEY_TWO", "two")
	if err == nil || strings.Contains(err.Error(), envSentinel) || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error = %v", err)
	}
}

func TestSnapshotExportsConfigDirectoryOnlyForTrustedSkills(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "env-trusted", "Env Trusted")
	local := ensureSnapshot(t, fakeSnapshotService(), root, "env-trusted")
	want := filepath.Join(root, "runtime", "config", "env-trusted")
	if local.Env[skillruntime.EnvConfigDir] != want {
		t.Fatalf("env = %#v", local.Env)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("config directory = %v, %v", info, err)
	}
	if _, err := os.Stat(configEnvPath(root, "env-trusted")); !os.IsNotExist(err) {
		t.Fatalf("the env file must not be created by activation: %v", err)
	}

	createActiveDistributionSkill(t, root, "env-untrusted", "Env Untrusted")
	updateSkillMeta(t, root, "env-untrusted", markThirdParty)
	untrusted := ensureSnapshot(t, fakeSnapshotService(), root, "env-untrusted")
	if untrusted.Status != LocalStatusReviewRequired || untrusted.Env != nil || untrusted.Preflight != nil {
		t.Fatalf("untrusted local = %#v", untrusted)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "config", "env-untrusted")); !os.IsNotExist(err) {
		t.Fatalf("a config directory was created for an untrusted skill: %v", err)
	}
	if encoded, _ := json.Marshal(untrusted); strings.Contains(string(encoded), "config") {
		t.Fatalf("untrusted response mentions config: %s", encoded)
	}
}

func envDoctorRuntime(check string) func(map[string]any) {
	return func(document map[string]any) {
		document["runtime"] = map[string]any{
			"requires": map[string]any{"env": []any{"STORED_TOKEN"}},
			"setup":    map[string]any{"command": "npm install", "check": check},
		}
	}
}

func TestDoctorTreatsStoredVariableAsPresentAndNeverLeaksIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("setup check commands use sh")
	}
	t.Parallel()
	root := newDoctorSkill(t, "doctor-stored", envDoctorRuntime(`test "${#STORED_TOKEN}" -eq `+strconv.Itoa(len(envSentinel))+` && echo "token is $STORED_TOKEN"`))
	sink := &captureTelemetrySink{}
	service := SkillDoctorService{Telemetry: sink, snapshots: fakeSnapshotService(), options: skillruntime.DoctorOptions{
		LookupEnv: func(string) (string, bool) { return "", false },
	}}

	missing, err := service.Run(t.Context(), root, "doctor-stored")
	if err != nil {
		t.Fatal(err)
	}
	if missing.State != skillruntime.StateSetupRequired {
		t.Fatalf("an unset variable must fail the doctor: %#v", missing)
	}
	wantCommand := "skillhub skill env set doctor-stored STORED_TOKEN"
	if !slices.ContainsFunc(missing.SuggestedActions, func(action Action) bool { return action.Command == wantCommand }) {
		t.Fatalf("missing variable must suggest %q: %#v", wantCommand, missing.SuggestedActions)
	}

	if _, err := (SkillEnvService{}).Set(t.Context(), root, "doctor-stored", "STORED_TOKEN", envSentinel); err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(t.Context(), root, "doctor-stored")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != skillruntime.StateReady {
		t.Fatalf("a stored variable must satisfy the requirement and reach the check: %#v", result)
	}
	if !strings.Contains(result.CheckOutput, "token is ***") {
		t.Fatalf("check output must be redacted: %q", result.CheckOutput)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded)+result.CheckOutput, envSentinel) {
		t.Fatalf("doctor result leaks the value: %s", encoded)
	}
	cache, err := os.ReadFile(result.CachePath)
	if err != nil || strings.Contains(string(cache), envSentinel) {
		t.Fatalf("doctor cache = %s, %v", cache, err)
	}
	for _, event := range sink.events {
		if encoded, _ := json.Marshal(event); strings.Contains(string(encoded), envSentinel) {
			t.Fatalf("telemetry leaks the value: %s", encoded)
		}
	}

	// The value may appear only in the env file itself.
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || path == configEnvPath(root, "doctor-stored") {
			return walkErr
		}
		if contents, readErr := os.ReadFile(path); readErr == nil && strings.Contains(string(contents), envSentinel) {
			t.Errorf("%s contains the stored value", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
