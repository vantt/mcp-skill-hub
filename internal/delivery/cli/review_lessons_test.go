package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
)

// hubWithCandidateLessons creates a hub at root with one skill whose
// distill.yaml holds candidate lessons, and returns that file's hub-relative
// path.
func hubWithCandidateLessons(t *testing.T, root string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := runInitTo(t, root, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr.String())
	}
	createActiveDoctorSkill(t, root, "test-audit", "")
	fixture, err := os.ReadFile(filepath.Join("..", "..", "migration", "testdata", "test-audit-distill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	relative := "skills/core/test-audit/.meta/distill.yaml"
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, "maintain", "rebuild", "--workspace", root); code != 0 {
		t.Fatalf("rebuild exit = %d: %s", code, stderr)
	}
	return relative
}

func statusJSONOf(t *testing.T, root string) []byte {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"status", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exit = %d: %s", code, stderr.String())
	}
	return stdout.Bytes()
}

func reviewLessons(t *testing.T, document []byte) map[string]any {
	t.Helper()
	var status struct {
		Actions []map[string]any `json:"actions"`
	}
	if err := json.Unmarshal(document, &status); err != nil {
		t.Fatal(err)
	}
	for _, action := range status.Actions {
		if action["kind"] == "review_lessons" {
			return action
		}
	}
	t.Fatalf("no review_lessons action in %s", document)
	return nil
}

func TestStatusPrintsAnAbsoluteDistillLabCommand(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "skill hub")
	file := hubWithCandidateLessons(t, root)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exit = %d: %s", code, stderr.String())
	}
	want := "$ " + reviewLessonsWant(root, file)
	if !strings.Contains(stdout.String(), want+"\n") {
		t.Fatalf("status output lacks %q:\n%s", want, stdout.String())
	}

	document := statusJSONOf(t, root)
	validateJSONDocument(t, filepath.Join("..", "..", "..", "schemas", "curation-home-v1.schema.json"), document)
	action := reviewLessons(t, document)
	if action["script"] != ".agents/skills/distill-lab/scripts/distill.mjs" || action["file"] != file || action["hub_relative"] != true ||
		action["command"] != "node .agents/skills/distill-lab/scripts/distill.mjs list "+file {
		t.Fatalf("review_lessons JSON = %v", action)
	}
}

// reviewLessonsWant is the command status prints on this platform for a hub
// at root.
func reviewLessonsWant(root, file string) string {
	command, _ := reviewLessonsCommand(app.ActionItem{Kind: "review_lessons", HubRelative: true,
		Script: ".agents/skills/distill-lab/scripts/distill.mjs", File: file}, root, runtime.GOOS)
	return command
}

func TestStatusWithholdsTheCommandOfAChangedDistillLab(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	hubWithCandidateLessons(t, root)
	script := filepath.Join(root, ".agents", "skills", "distill-lab", "scripts", "distill.mjs")
	raw, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 1
	if err := os.WriteFile(script, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	document := statusJSONOf(t, root)
	validateJSONDocument(t, filepath.Join("..", "..", "..", "schemas", "curation-home-v1.schema.json"), document)
	action := reviewLessons(t, document)
	if action["command"] != "" || action["script"] != nil || action["file"] != nil || action["hub_relative"] != nil {
		t.Fatalf("review_lessons JSON of a changed distill-lab = %v", action)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exit = %d: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "$ node") || !strings.Contains(stdout.String(), "differs from Skill Hub's copy, run `skillhub doctor`") {
		t.Fatalf("status output of a changed distill-lab:\n%s", stdout.String())
	}
}

func TestStatusSchemaRejectsAnUnknownActionProperty(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	hubWithCandidateLessons(t, root)
	var status map[string]any
	if err := json.Unmarshal(statusJSONOf(t, root), &status); err != nil {
		t.Fatal(err)
	}
	actions := status["actions"].([]any)
	actions[0].(map[string]any)["working_directory"] = "/elsewhere"
	document, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	err = validateSchemaFile(filepath.Join("..", "..", "..", "schemas", "curation-home-v1.schema.json"), document)
	if err == nil || !strings.Contains(err.Error(), "working_directory") {
		t.Fatalf("an unknown actionItem property passed the schema: %v", err)
	}
}

func TestReviewLessonsCommandPerShell(t *testing.T) {
	t.Parallel()
	action := app.ActionItem{Kind: "review_lessons", HubRelative: true, Script: ".agents/skills/distill-lab/scripts/distill.mjs",
		File: "skills/core/test-audit/.meta/distill.yaml", Command: "node .agents/skills/distill-lab/scripts/distill.mjs list skills/core/test-audit/.meta/distill.yaml"}
	cases := []struct {
		name, hub, goos, command, in string
	}{
		{"posix with a space", "/home/me/skill hub", "linux",
			"node '/home/me/skill hub/.agents/skills/distill-lab/scripts/distill.mjs' list '/home/me/skill hub/skills/core/test-audit/.meta/distill.yaml'", ""},
		{"posix with a quote", "/home/me/it's", "darwin",
			`node '/home/me/it'\''s/.agents/skills/distill-lab/scripts/distill.mjs' list '/home/me/it'\''s/skills/core/test-audit/.meta/distill.yaml'`, ""},
		{"windows", `C:\Users\me\skill hub`, "windows",
			`node "C:\Users\me\skill hub\.agents\skills\distill-lab\scripts\distill.mjs" list "C:\Users\me\skill hub\skills\core\test-audit\.meta\distill.yaml"`, ""},
		{"windows with a percent sign", `C:\Users\me\100% hub`, "windows", action.Command, `C:\Users\me\100% hub`},
		{"windows with a dollar sign", `C:\Users\$me\hub`, "windows", action.Command, `C:\Users\$me\hub`},
	}
	for _, test := range cases {
		command, in := reviewLessonsCommand(action, test.hub, test.goos)
		if command != test.command || in != test.in {
			t.Errorf("%s:\n got %q in %q\nwant %q in %q", test.name, command, in, test.command, test.in)
		}
		if test.goos == "windows" && strings.Contains(command, "&&") {
			t.Errorf("%s: %q chains with &&", test.name, command)
		}
	}
	withheld := action
	withheld.Command, withheld.Script, withheld.File, withheld.HubRelative = "", "", "", false
	if command, _ := reviewLessonsCommand(withheld, "/hub", "linux"); command != "" {
		t.Fatalf("a withheld command was rendered: %q", command)
	}
}
