package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/systemskills"
	"github.com/vantt/skillhub-mcp/internal/userconfig"
)

func writeFileForTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func blockedItems(result Result) []Item {
	var items []Item
	for _, item := range result.Items {
		if strings.HasPrefix(item.Impact, BlockedImpactPrefix) {
			items = append(items, item)
		}
	}
	return items
}

func TestConnectListsAChangedSkillFileAndWritesTheRest(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	project := t.TempDir()
	edited := filepath.Join(project, ".claude", "skills", "system-curator", "SKILL.md")
	writeFileForTest(t, edited, "my own curator\n")
	result, err := (ConnectService{Store: userconfig.NewStore(t.TempDir())}).Connect(t.Context(), ConnectRequest{Project: project, Workspace: hub})
	if err != nil || result.Error != nil {
		t.Fatalf("connect: %+v %v", result.Error, err)
	}
	blocked := blockedItems(result)
	if len(blocked) != 1 || blocked[0].ID != "host_claude-code_native-skill:.claude/skills/system-curator/SKILL.md" || !strings.Contains(blocked[0].Summary, edited) {
		t.Fatalf("blocked items = %+v", blocked)
	}
	if !slices.ContainsFunc(result.Warnings, func(warning Warning) bool {
		return warning.Code == "system_skill_blocked" && strings.Contains(warning.Summary, edited)
	}) {
		t.Fatalf("warnings = %+v", result.Warnings)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "settings.local.json")); err != nil {
		t.Fatalf("MCP entry not written: %v", err)
	}
	if got, _ := os.ReadFile(edited); string(got) != "my own curator\n" {
		t.Fatalf("edited wrapper changed: %q", got)
	}
}

func TestInitOverAChangedCuratorWrapperCompletes(t *testing.T) {
	t.Parallel()
	root := healthHub(t)
	if err := os.RemoveAll(filepath.Join(root, ".agents")); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(root, ".claude", "skills", "system-curator", "SKILL.md")
	writeFileForTest(t, edited, "my own curator\n")
	result, err := (WorkspaceService{}).Init(root, InitOptions{Store: userconfig.NewStore(t.TempDir())})
	if err != nil || result.Error != nil {
		t.Fatalf("init: %+v %v", result.Error, err)
	}
	if blocked := blockedItems(result); len(blocked) != 1 || !strings.Contains(blocked[0].Summary, edited) {
		t.Fatalf("blocked items = %+v", result.Items)
	}
	for _, path := range []string{".mcp.json", ".agents/skills/system-curator/SKILL.md", ".agents/skills/distill-lab/SKILL.md", ".claude/skills/distill-lab/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("init did not write %s: %v", path, err)
		}
	}
}

func TestBlockedHubFileDoesNotStopTheProjectRepair(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	project := connectProject(t, hub, hostintegration.HostClaude)
	canonical := filepath.Join(hub, ".agents", "skills", "system-curator", "SKILL.md")
	writeFileForTest(t, canonical, "not the curator\n")
	if err := os.Remove(filepath.Join(project, ".claude", "settings.local.json")); err != nil {
		t.Fatal(err)
	}
	report := DetectRole(project, t.TempDir(), userconfig.Config{})
	sections := Run(t.Context(), report, DepthFull)
	blocked := findingByCode(t, sections, "system_skill_blocked")
	if blocked.Severity != SeverityProblem || blocked.Fix != nil || !strings.Contains(blocked.Message, ".agents/skills/system-curator/SKILL.md") {
		t.Fatalf("blocked finding = %+v", blocked)
	}
	fixes := ApplyFixes(t.Context(), sections)
	if !slices.ContainsFunc(fixes.NotFixed, func(item NotFixedItem) bool { return item.Code == "system_skill_blocked" }) {
		t.Fatalf("not fixed = %+v", fixes.NotFixed)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "settings.local.json")); err != nil {
		t.Fatalf("project repair skipped: %v (fixes %+v)", err, fixes)
	}
	if got, _ := os.ReadFile(canonical); string(got) != "not the curator\n" {
		t.Fatalf("hub file changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(hub, ".claude", "skills", "system-curator", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("hub wrapper still points at a foreign file: %v", err)
	}
}

func TestStatusNamesABlockedSkillFile(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	canonical := filepath.Join(hub, ".agents", "skills", "distill-lab", "SKILL.md")
	writeFileForTest(t, canonical, "not distill-lab\n")
	request, err := hostIntegrationRequest(hub)
	if err != nil {
		t.Fatal(err)
	}
	finding, err := hostintegration.CheckMissing(t.Context(), request)
	if err != nil || finding == nil || finding.ID != hostintegration.FindingHostIntegrationBlocked || !strings.Contains(finding.Summary, canonical) {
		t.Fatalf("finding = %+v, %v", finding, err)
	}
	if err := os.Remove(filepath.Join(hub, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
	if finding, _ := hostintegration.CheckMissing(t.Context(), request); finding == nil || finding.ID != hostintegration.FindingHostIntegrationMissing {
		t.Fatalf("a missing file must win over a blocked one: %+v", finding)
	}
}

func TestStatusReportsAnOldLayoutHubUntilDoctorFix(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	for _, dir := range []string{".claude", ".agents", ".gemini"} {
		if err := os.RemoveAll(filepath.Join(hub, dir, "skills")); err != nil {
			t.Fatal(err)
		}
	}
	bundled := []byte(systemskills.CuratorSkill)
	for _, dir := range []string{".claude", ".agents", ".gemini"} {
		writeFileForTest(t, filepath.Join(hub, dir, "skills", "system-curator", "SKILL.md"), string(bundled))
	}
	request, err := hostIntegrationRequest(hub)
	if err != nil {
		t.Fatal(err)
	}
	if finding, _ := hostintegration.CheckMissing(t.Context(), request); finding == nil || finding.ID != hostintegration.FindingHostIntegrationMissing {
		t.Fatalf("old layout finding = %+v", finding)
	}
	if _, err := (WorkspaceService{}).DoctorFix(hub, false); err != nil {
		t.Fatal(err)
	}
	if finding, _ := hostintegration.CheckMissing(t.Context(), request); finding != nil {
		t.Fatalf("after doctor --fix: %+v", finding)
	}
	if _, err := os.Stat(filepath.Join(hub, ".gemini", "skills")); !os.IsNotExist(err) {
		t.Fatalf("old Gemini copy kept: %v", err)
	}
}

func TestDoctorFixReportsTheRemovedGeminiCopy(t *testing.T) {
	t.Parallel()
	hub := healthHub(t)
	for _, dir := range []string{".claude", ".agents", ".gemini"} {
		if err := os.RemoveAll(filepath.Join(hub, dir, "skills")); err != nil {
			t.Fatal(err)
		}
		writeFileForTest(t, filepath.Join(hub, dir, "skills", "system-curator", "SKILL.md"), systemskills.CuratorSkill)
	}
	report := DetectRole(hub, t.TempDir(), userconfig.Config{})
	fixes := ApplyFixes(t.Context(), Run(t.Context(), report, DepthFull))
	var notes []string
	var changed Changed
	for _, item := range fixes.Fixed {
		notes = append(notes, item.Notes...)
		changed = append(changed, item.Changed...)
	}
	if !slices.ContainsFunc(notes, func(note string) bool { return strings.Contains(note, "v0.28.0") }) {
		t.Fatalf("no Gemini CLI note in %+v", fixes)
	}
	if !slices.Contains(changed, filepath.Join(hub, ".gemini", "skills", "system-curator", "SKILL.md")) {
		t.Fatalf("removed copy not listed: %v", changed)
	}
}
