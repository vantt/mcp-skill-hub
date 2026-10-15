package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
)

// writeActiveSkill adds an active skill to the hub with the given SKILL.md
// frontmatter.
func writeActiveSkill(t *testing.T, hub, id, frontmatter string) {
	t.Helper()
	directory := filepath.Join(hub, "skills", "default", id)
	if err := os.MkdirAll(filepath.Join(directory, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := "schema_version: 1\nid: " + id + "\nstatus: active\nquality:\n  reviewed: true\nrouting:\n  triggers: [check the " + id + " skill]\n" +
		"  not_for: [write marketing prose]\n  min_scope: multi_step\n"
	if err := os.WriteFile(filepath.Join(directory, ".meta", "skill.yaml"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\n"+frontmatter+"---\n\n# "+id+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// warningLines returns the WARNING paragraphs of text output with wrapped
// lines joined, so a long path or message matches whatever the width.
func warningLines(output string) []string {
	var warnings []string
	open := false
	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "WARNING: "):
			warnings = append(warnings, strings.TrimPrefix(line, "WARNING: "))
			open = true
		case open && strings.HasPrefix(line, "         ") && strings.TrimSpace(line) != "":
			warnings[len(warnings)-1] += " " + strings.TrimSpace(line)
		default:
			open = false
		}
	}
	return warnings
}

func namesSkill(warnings []string, id string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, "skills/default/"+id+":") {
			return true
		}
	}
	return false
}

func TestCommandsNameAnActiveSkillThatCannotBeServed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hub := initTestWorkspace(t)
	writeActiveSkill(t, hub, "served", "name: served\ndescription: A skill agents can get.\n")
	if code, stdout, stderr := runCLI(t, "maintain", "rebuild", "--workspace", hub); code != 0 || len(warningLines(stdout)) != 0 {
		t.Fatalf("rebuild of a hub whose skills can all be served: exit %d, warnings %q\n%s", code, warningLines(stdout), stderr)
	}
	writeActiveSkill(t, hub, "nameless", "description: A skill without a name.\n")

	t.Run("rebuild", func(t *testing.T) {
		code, stdout, stderr := runCLI(t, "maintain", "rebuild", "--workspace", hub)
		warnings := warningLines(stdout)
		if code != 0 || !namesSkill(warnings, "nameless") || namesSkill(warnings, "served") {
			t.Fatalf("rebuild: exit %d, warnings %q, want one naming nameless\n%s%s", code, warnings, stdout, stderr)
		}
	})

	t.Run("init", func(t *testing.T) {
		code, stdout, stderr := runInitCLI(t, hub)
		warnings := warningLines(stdout)
		if code != 0 || !namesSkill(warnings, "nameless") || namesSkill(warnings, "served") {
			t.Fatalf("init: exit %d, warnings %q, want one naming nameless\n%s%s", code, warnings, stdout, stderr)
		}
		code, stdout, stderr = runInitCLI(t, hub, "--json")
		var result app.Result
		if code != 0 || json.Unmarshal([]byte(stdout), &result) != nil {
			t.Fatalf("init --json: exit %d\n%s%s", code, stdout, stderr)
		}
		var named []string
		for _, warning := range result.Warnings {
			if warning.Code == app.WarningSkillNotServable {
				named = append(named, warning.Summary)
			}
		}
		if !namesSkill(named, "nameless") || namesSkill(named, "served") || len(named) != 1 {
			t.Fatalf("init --json %s warnings = %q", app.WarningSkillNotServable, named)
		}
	})

	t.Run("doctor", func(t *testing.T) {
		before, _, _ := runCLI(t, "doctor", "--workspace", hub)
		_, stdout, stderr := runCLI(t, "doctor", "--workspace", hub, "--json")
		var report struct {
			Sections []app.Section `json:"sections"`
		}
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("doctor --json: %v\n%s%s", err, stdout, stderr)
		}
		var found []app.Finding
		for _, section := range report.Sections {
			for _, finding := range section.Findings {
				if finding.Code == "hub_skill_not_servable" {
					found = append(found, finding)
				}
			}
		}
		if len(found) != 1 || found[0].Label != "Content" || found[0].Severity != app.SeverityNotice || !strings.HasPrefix(found[0].Message, "skills/default/nameless:") {
			t.Fatalf("doctor findings for skills that cannot be served = %+v", found)
		}
		_, text, _ := runCLI(t, "doctor", "--workspace", hub)
		if !strings.Contains(strings.Join(strings.Fields(text), " "), "skills/default/nameless:") {
			t.Fatalf("doctor text does not name the skill:\n%s", text)
		}
		// A notice: the skill that cannot be served does not fail doctor.
		writeActiveSkill(t, hub, "nameless", "name: nameless\ndescription: Repaired.\n")
		if after, _, _ := runCLI(t, "doctor", "--workspace", hub); after != before {
			t.Fatalf("doctor exit %d with the skill broken, %d after the repair; a notice must not change it", before, after)
		}
	})
}
