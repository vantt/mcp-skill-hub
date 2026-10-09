package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"gopkg.in/yaml.v3"
)

func createActiveDoctorSkill(t *testing.T, root, id, metaSuffix string) {
	t.Helper()
	contentFile := filepath.Join(t.TempDir(), "content.md")
	if err := os.WriteFile(contentFile, []byte("---\nname: "+id+"\ndescription: Doctor test skill.\n---\n\n# Doctor Test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "skill", "create", id, "--workspace", root,
		"--collection", "core", "--name", "Doctor Test", "--description", "Doctor test skill.",
		"--trigger", "doctor it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile, "--yes")
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	if code, _, stderr := runCLI(t, "skill", "activate", id, "--workspace", root, "--yes"); code != 0 {
		t.Fatalf("activate failed (exit %d): %s", code, stderr)
	}
	if metaSuffix == "" {
		return
	}
	setDoctorSkillMeta(t, root, id, metaSuffix)
}

// setDoctorSkillMeta replaces any runtime block with the given top-level YAML.
func setDoctorSkillMeta(t *testing.T, root, id, metaSuffix string) {
	t.Helper()
	metaPath := filepath.Join(root, "skills", "core", id, ".meta", "skill.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = filepath.Join(root, "skills", "core", id, "skill.meta.yaml")
	}
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(meta)
	if index := strings.Index(text, "\nruntime:\n"); index >= 0 {
		text = text[:index+1]
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := os.WriteFile(metaPath, []byte(text+metaSuffix), 0o644); err != nil {
		t.Fatal(err)
	}
}

func otherPlatform() string {
	if runtime.GOOS == "linux" {
		return "darwin"
	}
	return "linux"
}

func TestSkillDoctorCheckPassesAndFailsWithExitCodes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("setup check commands use sh")
	}
	t.Parallel()
	root := initTestWorkspace(t)
	createActiveDoctorSkill(t, root, "doctor-cli", "runtime:\n  requires:\n    bins: [sh]\n  setup:\n    command: echo installing\n    check: exit 0\n")

	code, stdout, stderr := runCLI(t, "skill", "doctor", "doctor-cli", "--workspace", root)
	if code != 0 {
		t.Fatalf("doctor exit = %d, stdout=%s stderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{"Skill doctor: doctor-cli", "PASS  bin sh", "PASS  setup_check check", "State: ready"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "echo installing") {
		t.Fatalf("ready output must not suggest setup:\n%s", stdout)
	}

	setDoctorSkillMeta(t, root, "doctor-cli", "runtime:\n  requires:\n    bins: [sh]\n  setup:\n    command: echo installing\n    check: echo check-failed-output; exit 3\n")
	code, stdout, _ = runCLI(t, "skill", "doctor", "doctor-cli", "--workspace", root)
	if code != 1 {
		t.Fatalf("failing check exit = %d:\n%s", code, stdout)
	}
	for _, want := range []string{"FAIL  setup_check check  exit status 3", "State: setup_required", "check-failed-output", "Suggested setup (not run", "$ echo installing"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout)
		}
	}

	code, stdout, stderr = runCLI(t, "skill", "doctor", "doctor-cli", "--workspace", root, "--json")
	if code != 1 {
		t.Fatalf("json exit = %d: %s", code, stderr)
	}
	var result app.SkillDoctorResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode doctor JSON: %v\n%s", err, stdout)
	}
	if result.SkillID != "doctor-cli" || result.State != skillruntime.StateSetupRequired || result.SetupCommand != "echo installing" || result.CachePath == "" || result.Status != app.StatusActionRequired {
		t.Fatalf("doctor JSON = %#v", result)
	}
	cached, err := os.ReadFile(result.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cached), "check-failed-output") {
		t.Fatalf("check output was stored in the doctor cache: %s", cached)
	}
}

func TestSkillDoctorReportsMissingBinAndUnsupportedPlatform(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	createActiveDoctorSkill(t, root, "doctor-missing", "runtime:\n  requires:\n    bins: [skillhub-doctor-missing-bin]\n")
	code, stdout, _ := runCLI(t, "skill", "doctor", "doctor-missing", "--workspace", root)
	if code != 1 || !strings.Contains(stdout, "FAIL  bin skillhub-doctor-missing-bin  not_found") || !strings.Contains(stdout, "State: setup_required") {
		t.Fatalf("missing bin exit = %d:\n%s", code, stdout)
	}

	createActiveDoctorSkill(t, root, "doctor-platform", "runtime:\n  requires:\n    platforms: ["+otherPlatform()+"]\n  setup:\n    check: echo check-ran\n")
	code, stdout, _ = runCLI(t, "skill", "doctor", "doctor-platform", "--workspace", root)
	if code != 1 || !strings.Contains(stdout, "FAIL  platform "+runtime.GOOS) || !strings.Contains(stdout, "State: unsupported_platform") {
		t.Fatalf("unsupported platform exit = %d:\n%s", code, stdout)
	}
	if strings.Contains(stdout, "check-ran") || strings.Contains(stdout, "setup_check") {
		t.Fatalf("setup check ran on an unsupported platform:\n%s", stdout)
	}
}

func TestSkillDoctorSkipsCheckForUnreviewedThirdPartyScripts(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	createActiveDoctorSkill(t, root, "doctor-third", "runtime:\n  setup:\n    command: echo installing\n    check: echo check-ran\n")
	metaPath := filepath.Join(root, "skills", "core", "doctor-third", ".meta", "skill.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = filepath.Join(root, "skills", "core", "doctor-third", "skill.meta.yaml")
	}
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(meta, &doc); err != nil {
		t.Fatal(err)
	}
	doc["sources"] = []any{
		map[string]any{
			"id":         "src-third",
			"roles":      []string{"upstream"},
			"kind":       "github",
			"repository": "https://github.com/example/skills",
			"commit":     strings.Repeat("a", 40),
		},
	}
	doc["quality"] = map[string]any{
		"reviewed": false,
	}
	encoded, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := runCLI(t, "skill", "doctor", "doctor-third", "--workspace", root)
	if code != 1 {
		t.Fatalf("third-party exit = %d:\n%s", code, stdout)
	}
	for _, want := range []string{"SKIP  setup_check check  " + skillruntime.ReasonContentReviewRequired, "State: setup_required", "skillhub skill review doctor-third"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "check-ran") || strings.Contains(stdout, "echo installing") {
		t.Fatalf("unreviewed commands were run or offered:\n%s", stdout)
	}
}

func TestSkillDoctorNeverPrintsEnvValues(t *testing.T) {
	t.Setenv("SKILLHUB_DOCTOR_TEST_SECRET", "doctor-secret-value")
	root := initTestWorkspace(t)
	createActiveDoctorSkill(t, root, "doctor-env", "runtime:\n  requires:\n    env: [SKILLHUB_DOCTOR_TEST_SECRET, SKILLHUB_DOCTOR_TEST_UNSET]\n")
	for _, args := range [][]string{{}, {"--json"}} {
		code, stdout, stderr := runCLI(t, append([]string{"skill", "doctor", "doctor-env", "--workspace", root}, args...)...)
		if code != 1 {
			t.Fatalf("env doctor exit = %d: %s", code, stderr)
		}
		if strings.Contains(stdout+stderr, "doctor-secret-value") {
			t.Fatalf("doctor printed an env value:\n%s%s", stdout, stderr)
		}
		if !strings.Contains(stdout, "SKILLHUB_DOCTOR_TEST_UNSET") {
			t.Fatalf("doctor output lacks the env check:\n%s", stdout)
		}
	}
}

func TestSkillDoctorWithoutRuntimeAndInvalidRequests(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	createActiveDoctorSkill(t, root, "doctor-plain", "")
	code, stdout, stderr := runCLI(t, "skill", "doctor", "doctor-plain", "--workspace", root)
	if code != 0 || !strings.Contains(stdout, app.DoctorNoRuntime) || !strings.Contains(stdout, "State: ready") {
		t.Fatalf("no runtime exit = %d: %s %s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "cache", "doctor")); !os.IsNotExist(err) {
		t.Fatalf("doctor wrote a cache for a skill without runtime: %v", err)
	}

	for name, args := range map[string][]string{
		"unknown skill":  {"skill", "doctor", "doctor-missing-skill", "--workspace", root},
		"missing id":     {"skill", "doctor", "--workspace", root},
		"two ids":        {"skill", "doctor", "a", "b", "--workspace", root},
		"mutation flag":  {"skill", "doctor", "doctor-plain", "--workspace", root, "--yes"},
		"routing flag":   {"skill", "doctor", "doctor-plain", "--workspace", root, "--trigger", "x"},
		"invalid id":     {"skill", "doctor", "Bad_ID", "--workspace", root},
		"unknown --json": {"skill", "doctor", "doctor-missing-skill", "--workspace", root, "--json"},
	} {
		if code, stdout, stderr := runCLI(t, args...); code != 2 {
			t.Fatalf("%s exit = %d, want 2: %s %s", name, code, stdout, stderr)
		}
	}
}

func TestStripTerminalControls(t *testing.T) {
	t.Parallel()
	if got := stripTerminalControls("ok\x1b[31mred\x07\tline\n"); got != "ok[31mred\tline\n" {
		t.Fatalf("stripTerminalControls = %q", got)
	}
}
