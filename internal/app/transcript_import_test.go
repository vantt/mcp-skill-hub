package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/transcripts"
)

func TestTranscriptImportService(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(workspaceRoot, true); err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "claude-config")
	projectRoot := filepath.Join(tempDir, "sample-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	// Encoded project dir name
	canonRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		canonRoot = projectRoot
	}
	encProject := transcripts.EncodeProjectDir(canonRoot)
	projectTranscriptDir := filepath.Join(configDir, "projects", encProject)
	if err := os.MkdirAll(projectTranscriptDir, 0o755); err != nil {
		t.Fatal(err)
	}

	subagentsDir := filepath.Join(projectTranscriptDir, "s1", "subagents")
	if err := os.MkdirAll(subagentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	// Write main transcript
	mainContent := fmt.Sprintf(`{"type":"message","sessionId":"s1","cwd":%q,"timestamp":%q,"message":{"id":"m1","content":[{"type":"tool_use","id":"tu_res","name":"mcp__skillhub__skill_resolve","input":{"task":{"description":"inspect"}}}]}}
{"type":"message","sessionId":"s1","cwd":%q,"timestamp":%q,"message":{"id":"m2","content":[{"type":"tool_use","id":"tu_skill","name":"Skill","input":{"skill":"my-skill","args":"SENTINEL-TRANSCRIPT-SECRET"}}]}}
{"type":"message","sessionId":"s1","cwd":%q,"timestamp":%q,"message":{"id":"m3","content":[{"type":"tool_use","id":"tu_get","name":"mcp__skillhub__skill_get","input":{"skill_id":"review-skill"}}]}}
{"type":"message","sessionId":"s1","cwd":%q,"timestamp":%q,"message":{"id":"m4","content":[{"type":"tool_use","id":"tu_res_read","name":"ReadMcpResourceTool","input":{"server":"skillhub","uri":"skill://skillhub/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef/resource-skill/references/ref.md"}}]}}
{"type":"message","sessionId":"s1","cwd":%q,"timestamp":%q,"message":{"id":"m_old","content":[{"type":"tool_use","id":"tu_old","name":"Skill","input":{"skill":"old-skill"}}]}}
`,
		projectRoot, now.Add(-1*time.Hour).Format(time.RFC3339),
		projectRoot, now.Add(-55*time.Minute).Format(time.RFC3339),
		projectRoot, now.Add(-50*time.Minute).Format(time.RFC3339),
		projectRoot, now.Add(-45*time.Minute).Format(time.RFC3339),
		// Event 35 days ago (should be excluded by raw retention clamp):
		projectRoot, now.Add(-35*24*time.Hour).Format(time.RFC3339),
	)

	if err := os.WriteFile(filepath.Join(projectTranscriptDir, "session_1.jsonl"), []byte(mainContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write subagent transcript
	subContent := fmt.Sprintf(`{"type":"message","sessionId":"s_sub","cwd":%q,"timestamp":%q,"message":{"id":"m_sub","content":[{"type":"tool_use","id":"tu_sub","name":"Skill","input":{"skill":"sub-skill"}}]}}
`,
		projectRoot, now.Add(-30*time.Minute).Format(time.RFC3339),
	)
	if err := os.WriteFile(filepath.Join(subagentsDir, "sub_1.jsonl"), []byte(subContent), 0o644); err != nil {
		t.Fatal(err)
	}

	service := TranscriptImportService{
		Now: func() time.Time { return now },
	}

	// 1. First import
	res1, err := service.Import(context.Background(), workspaceRoot, ImportInput{
		Project:   projectRoot,
		ConfigDir: configDir,
	})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	// tu_res, tu_skill, tu_get, tu_res_read, tu_sub -> 5 observations (tu_old excluded by 14d clamp)
	if res1.Observations != 5 {
		t.Fatalf("expected 5 observations, got %d", res1.Observations)
	}
	if res1.Inserted != 5 {
		t.Fatalf("expected 5 inserted, got %d", res1.Inserted)
	}
	if res1.Duplicates != 0 {
		t.Fatalf("expected 0 duplicates on first import, got %d", res1.Duplicates)
	}
	if res1.FilesScanned != 2 {
		t.Fatalf("expected 2 files scanned, got %d", res1.FilesScanned)
	}

	// Per-tool counts
	if res1.PerTool["Skill"] != 2 {
		t.Errorf("Skill count = %d, want 2", res1.PerTool["Skill"])
	}
	if res1.PerTool["skill_resolve"] != 1 {
		t.Errorf("skill_resolve count = %d, want 1", res1.PerTool["skill_resolve"])
	}
	if res1.PerTool["skill_get"] != 1 {
		t.Errorf("skill_get count = %d, want 1", res1.PerTool["skill_get"])
	}
	if res1.PerTool["resources_read"] != 1 {
		t.Errorf("resources_read count = %d, want 1", res1.PerTool["resources_read"])
	}

	// 2. Second import must insert 0 and report 5 duplicates
	res2, err := service.Import(context.Background(), workspaceRoot, ImportInput{
		Project:   projectRoot,
		ConfigDir: configDir,
	})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if res2.Inserted != 0 {
		t.Fatalf("expected 0 inserted on second import, got %d", res2.Inserted)
	}
	if res2.Duplicates != 5 {
		t.Fatalf("expected 5 duplicates on second import, got %d", res2.Duplicates)
	}

	// 3. Privacy test: ensure SENTINEL-TRANSCRIPT-SECRET is not in telemetry.db or wal
	dbPath := filepath.Join(workspaceRoot, "runtime", "telemetry.db")
	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbBytes, []byte("SENTINEL-TRANSCRIPT-SECRET")) {
		t.Fatal("telemetry.db leaked SENTINEL-TRANSCRIPT-SECRET")
	}

	walPath := filepath.Join(workspaceRoot, "runtime", "telemetry.db-wal")
	if walBytes, err := os.ReadFile(walPath); err == nil {
		if bytes.Contains(walBytes, []byte("SENTINEL-TRANSCRIPT-SECRET")) {
			t.Fatal("telemetry.db-wal leaked SENTINEL-TRANSCRIPT-SECRET")
		}
	}
}
