package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/transcripts"
)

func TestTelemetryCLIImportTranscripts(t *testing.T) {
	root, _ := telemetryCLIWorkspace(t)

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "claude-config")
	projectRoot := filepath.Join(tempDir, "app-project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	canonRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		canonRoot = projectRoot
	}
	encProject := transcripts.EncodeProjectDir(canonRoot)
	projectTranscriptDir := filepath.Join(configDir, "projects", encProject)
	if err := os.MkdirAll(projectTranscriptDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLAUDE_CONFIG_DIR", configDir)

	now := time.Now().UTC()
	content := fmt.Sprintf(`{"type":"message","sessionId":"s1","cwd":%q,"timestamp":%q,"message":{"id":"m1","content":[{"type":"tool_use","id":"tu_1","name":"Skill","input":{"skill":"code-review"}}]}}
`, projectRoot, now.Add(-1*time.Hour).Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(projectTranscriptDir, "session.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	// 1. Missing --project returns exit code 2
	if code := Run([]string{"telemetry", "import-transcripts", "--workspace", root}, &stdout, &stderr); code != 2 {
		t.Fatalf("missing project code=%d, want 2", code)
	}

	// 2. Valid import with --json
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "import-transcripts", "--workspace", root, "--project", projectRoot, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("import json code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var res app.ImportResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal import result: %v", err)
	}
	if res.Observations != 1 || res.Inserted != 1 {
		t.Fatalf("unexpected import result: %+v", res)
	}

	// 3. Human output
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"telemetry", "import-transcripts", "--workspace", root, "--project", projectRoot}, &stdout, &stderr); code != 0 {
		t.Fatalf("import human code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	outStr := stdout.String()
	if !strings.Contains(outStr, "Transcript Import") {
		t.Errorf("missing heading in output: %s", outStr)
	}
	if !strings.Contains(outStr, "Privacy notice: Stored only tool name, skill ID, timestamp, and a session hash.") {
		t.Errorf("missing privacy notice in output: %s", outStr)
	}
}
