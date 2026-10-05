package transcripts

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncodeProjectDir(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/home/vantt/projects/mcp-skill-hub", "-home-vantt-projects-mcp-skill-hub"},
		{"/var/data.test/repo", "-var-data-test-repo"},
		{".", "-"},
	}
	for _, tc := range cases {
		got := encodeProjectDir(tc.input)
		if got != tc.want {
			t.Errorf("encodeProjectDir(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestProjectDirs(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "claude-config")
	projectsDir := filepath.Join(configDir, "projects")

	projectRoot := filepath.Join(tempDir, "my.repo")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	canonRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		canonRoot = projectRoot
	}
	enc := encodeProjectDir(canonRoot)

	// Create matching dirs in projectsDir
	matchMain := filepath.Join(projectsDir, enc)
	matchWorktree := filepath.Join(projectsDir, enc+"--claude-worktrees-feature")
	unrelated := filepath.Join(projectsDir, "-other-project")

	if err := os.MkdirAll(matchMain, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(matchWorktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(unrelated, 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, err := ProjectDirs(configDir, projectRoot)
	if err != nil {
		t.Fatalf("ProjectDirs: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("expected 2 matching dirs, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != matchMain || dirs[1] != matchWorktree {
		t.Fatalf("unexpected matching dirs: %v", dirs)
	}

	// Test CLAUDE_CONFIG_DIR env var
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	dirsEnv, err := ProjectDirs("", projectRoot)
	if err != nil {
		t.Fatalf("ProjectDirs with CLAUDE_CONFIG_DIR: %v", err)
	}
	if len(dirsEnv) != 2 {
		t.Fatalf("expected 2 matching dirs from env, got %d", len(dirsEnv))
	}
}

func TestScan(t *testing.T) {
	testdataDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}

	validCwds := []string{"/test/project"}
	result, err := Scan([]string{testdataDir}, time.Time{}, time.Time{}, validCwds)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// FilesScanned: session_main.jsonl and subagents/agent_1.jsonl
	if result.FilesScanned != 2 {
		t.Errorf("FilesScanned = %d, want 2", result.FilesScanned)
	}

	// Malformed count: broken json line with tool_use
	if result.Malformed != 1 {
		t.Errorf("Malformed = %d, want 1", result.Malformed)
	}

	// Verify observations
	obsMap := make(map[string]Observation)
	for _, obs := range result.Observations {
		obsMap[obs.ToolUseID] = obs
	}

	// 1. Both blocks sharing message.id msg_shared survive
	if _, ok := obsMap["toolu_shared_1"]; !ok {
		t.Errorf("missing toolu_shared_1")
	}
	if _, ok := obsMap["toolu_shared_2"]; !ok {
		t.Errorf("missing toolu_shared_2")
	}

	// 2. Repeated tool_use.id collapses: only 1 entry for toolu_shared_1
	countShared1 := 0
	for _, obs := range result.Observations {
		if obs.ToolUseID == "toolu_shared_1" {
			countShared1++
		}
	}
	if countShared1 != 1 {
		t.Errorf("expected toolu_shared_1 count = 1 (deduped), got %d", countShared1)
	}

	// 3. Foreign cwd was dropped
	if _, ok := obsMap["toolu_foreign"]; ok {
		t.Errorf("foreign cwd toolu_foreign should have been dropped")
	}

	// 4. Subagent file was read
	if sub, ok := obsMap["toolu_subagent_1"]; !ok {
		t.Errorf("missing subagent observation toolu_subagent_1")
	} else {
		if sub.Tool != "Skill" || sub.SkillID != "sub-skill" {
			t.Errorf("unexpected subagent observation: %+v", sub)
		}
		if sub.ResolvedBefore {
			t.Errorf("subagent should NOT have ResolvedBefore=true")
		}
	}

	// 5. Skill observation with resolve within 30m has ResolvedBefore=true
	skillMain, ok := obsMap["toolu_skill_main"]
	if !ok {
		t.Fatalf("missing toolu_skill_main")
	}
	if skillMain.Tool != "Skill" || skillMain.SkillID != "my-skill" {
		t.Errorf("unexpected toolu_skill_main: %+v", skillMain)
	}
	if !skillMain.ResolvedBefore {
		t.Errorf("toolu_skill_main should have ResolvedBefore=true")
	}

	// 6. Resources_read from URI
	resObs, ok := obsMap["toolu_shared_2"]
	if !ok {
		t.Fatalf("missing toolu_shared_2")
	}
	if resObs.Tool != "resources_read" || resObs.SkillID != "resource-skill" {
		t.Errorf("unexpected resources_read obs: %+v", resObs)
	}

	// 7. Privacy check: ensure secret does NOT appear anywhere in observations
	for _, obs := range result.Observations {
		if obs.SkillID == "SENTINEL-TRANSCRIPT-SECRET" || obs.Tool == "SENTINEL-TRANSCRIPT-SECRET" {
			t.Fatalf("observation leaked sentinel secret: %+v", obs)
		}
	}
}
