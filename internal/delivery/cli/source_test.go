package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestSourceCaptureListAndExplicitEmptyCheck(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"source", "capture", "https://github.com/example/repo.git", "--reason", "review later", "--workspace", root}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "Candidate:") || !strings.Contains(stdout.String(), "SRCQ-") {
		t.Fatalf("capture code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = RunContext(context.Background(), []string{"source", "list", "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"status":"pending"`) {
		t.Fatalf("list code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = RunContext(context.Background(), []string{"check", "--all-due", "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"checked":0`) {
		t.Fatalf("check code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSourceCaptureRejectsCredentialBearingLocator(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"source", "capture", "https://user:secret@example.com/repo.git", "--reason", "unsafe", "--workspace", root}, &stdout, &stderr)
	if code != 2 || strings.Contains(stdout.String()+stderr.String(), "secret") {
		t.Fatalf("credential rejection code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestSourceCLIBlockedTelemetryPreservesSuccessAndErrorContracts(t *testing.T) {
	t.Parallel()
	normal := newSourceCLIWorkspace(t)
	blocked := newSourceCLIWorkspace(t)
	if err := os.WriteFile(filepath.Join(blocked, "runtime", "telemetry.db"), []byte("blocked telemetry"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		args func(string) []string
	}{
		{name: "success", args: func(root string) []string { return []string{"check", "--all-due", "--workspace", root, "--json"} }},
		{name: "error", args: func(root string) []string {
			return []string{"source", "capture", "https://user:private@example.com/repo.git", "--reason", "private", "--workspace", root, "--json"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			normalCode, normalOut, normalErr := runCLIForTest(test.args(normal))
			blockedCode, blockedOut, blockedErr := runCLIForTest(test.args(blocked))
			if normalCode != blockedCode || normalOut != blockedOut || normalErr != blockedErr {
				t.Fatalf("telemetry changed command contract\nnormal=(%d,%q,%q)\nblocked=(%d,%q,%q)", normalCode, normalOut, normalErr, blockedCode, blockedOut, blockedErr)
			}
		})
	}
}

func TestSourceCLIRecordsSanitizedCaptureAndCheckEvents(t *testing.T) {
	t.Parallel()
	root := onboardCLIFilesystemSource(t)
	code, stdout, stderr := runCLIForTest([]string{"check", "source-a", "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("check=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	preview, err := (app.TelemetryService{}).Preview(t.Context(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	jsonl := string(preview.JSONL)
	for _, eventType := range []string{telemetry.EventSourceCandidateCaptured, telemetry.EventSourceChecked} {
		if !strings.Contains(jsonl, `"event_type":"`+eventType+`"`) {
			t.Fatalf("missing %s event: %s", eventType, jsonl)
		}
	}
	for _, sensitive := range []string{"private source rationale", "sources/upstream", "upstream content"} {
		if strings.Contains(jsonl, sensitive) {
			t.Fatalf("telemetry leaked %q: %s", sensitive, jsonl)
		}
	}
}

func newSourceCLIWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	return root
}

func onboardCLIFilesystemSource(t *testing.T) string {
	t.Helper()
	root := newSourceCLIWorkspace(t)
	fixture := filepath.Join(root, "sources", "upstream")
	if err := os.MkdirAll(fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "notes.md"), []byte("upstream content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, _, se := runCLIForTest([]string{"skill", "create", "--workspace", root, "--id", "cli-skill", "--collection", "default", "--name", "CLI Skill", "--description", "CLI test skill", "--yes"}); c != 0 {
		t.Fatalf("skill create failed: %s", se)
	}
	code, stdout, stderr := runCLIForTest([]string{"source", "capture", "sources/upstream", "--reason", "private source rationale", "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("capture=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var captured app.SourceCandidateResult
	if err := json.Unmarshal([]byte(stdout), &captured); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runCLIForTest([]string{"source", "triage", captured.Candidate.ID, "--decision", "accept", "--source-id", "source-a", "--skill-id", "cli-skill", "--adapter", "filesystem", "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("triage=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var proposal app.SourceProposal
	if err := json.Unmarshal([]byte(stdout), &proposal); err != nil {
		t.Fatal(err)
	}
	pins := proposal.Confirmation.Confirmation.Pins
	code, stdout, stderr = runCLIForTest([]string{"source", "confirm", "--proposal", pins.ProposalID, "--proposal-digest", pins.ProposalDigest, "--base-version", pins.BaseVersion, "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("confirm=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	return root
}

func TestSourceShowPrintsWithoutListHeader(t *testing.T) {
	t.Parallel()
	root := onboardCLIFilesystemSource(t)
	code, stdout, stderr := runCLIForTest([]string{"source", "show", "source-a", "--workspace", root})
	if code != 0 {
		t.Fatalf("show=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "candidate(s) and") || strings.Contains(stdout, "monitored source(s)") {
		t.Fatalf("show output should not contain list header: %s", stdout)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "- source-a") {
		t.Fatalf("expected '- source-a...', got: %s", stdout)
	}
}

func TestSourceImportCLI(t *testing.T) {
	t.Parallel()
	root := newSourceCLIWorkspace(t)
	fixture := filepath.Join(root, "sources", "upstream")
	skillDir := filepath.Join(fixture, "calc-tool")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: calc-tool\ndescription: A calculation tool\n---\n# Calc Tool\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, _, se := runCLIForTest([]string{"skill", "create", "--workspace", root, "--id", "calc-skill", "--collection", "default", "--name", "Calc Skill", "--description", "Calc test skill", "--yes"}); c != 0 {
		t.Fatalf("skill create failed: %s", se)
	}

	// Capture, triage, confirm
	code, stdout, stderr := runCLIForTest([]string{"source", "capture", "sources/upstream", "--reason", "has skills", "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("capture failed: %d, %s %s", code, stdout, stderr)
	}
	var captured app.SourceCandidateResult
	_ = json.Unmarshal([]byte(stdout), &captured)

	code, stdout, stderr = runCLIForTest([]string{"source", "triage", captured.Candidate.ID, "--decision", "accept", "--source-id", "calc-source", "--skill-id", "calc-skill", "--adapter", "filesystem", "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("triage failed: %d, %s %s", code, stdout, stderr)
	}
	var proposal app.SourceProposal
	_ = json.Unmarshal([]byte(stdout), &proposal)
	pins := proposal.Confirmation.Confirmation.Pins

	code, stdout, stderr = runCLIForTest([]string{"source", "confirm", "--proposal", pins.ProposalID, "--proposal-digest", pins.ProposalDigest, "--base-version", pins.BaseVersion, "--workspace", root})
	if code != 0 {
		t.Fatalf("confirm failed: %d, %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Watching calc-source.") || !strings.Contains(stdout, "distill-lab") {
		t.Fatalf("confirm output missing distill-lab guidance: %s", stdout)
	}
	if !strings.Contains(stdout, "Watching does not auto-import skills") {
		t.Fatalf("confirm output missing auto-import disclaimer: %s", stdout)
	}

	// Run import preview
	code, stdout, stderr = runCLIForTest([]string{"source", "import", "calc-source", "--workspace", root})
	if code != 0 {
		t.Fatalf("import preview failed: %d, %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "calc-tool") || !strings.Contains(stdout, "importable") {
		t.Fatalf("import preview output missing calc-tool: %s", stdout)
	}

	// Run import with --yes
	code, stdout, stderr = runCLIForTest([]string{"source", "import", "calc-source", "--workspace", root, "--yes"})
	if code != 0 {
		t.Fatalf("import confirm failed: %d, %s %s", code, stdout, stderr)
	}

	// Verify draft skill exists
	code, stdout, stderr = runCLIForTest([]string{"skill", "show", "calc-tool", "--workspace", root, "--json"})
	if code != 0 {
		t.Fatalf("skill show failed: %d, %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"status":"draft"`) {
		t.Fatalf("imported skill is not in draft status: %s", stdout)
	}
}

func TestSourceCommandsPhase6(t *testing.T) {
	t.Parallel()

	t.Run("source list grouped and json", func(t *testing.T) {
		root := onboardCLIFilesystemSource(t)

		// 1. source list --json contains "groups"
		code, stdout, stderr := runCLIForTest([]string{"source", "list", "--workspace", root, "--json"})
		if code != 0 {
			t.Fatalf("list --json failed: %d, %s %s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, `"groups"`) {
			t.Fatalf("expected list --json to contain '\"groups\"', got:\n%s", stdout)
		}

		// 2. human list prints the repository heading and role
		code, stdout, stderr = runCLIForTest([]string{"source", "list", "--workspace", root})
		if code != 0 {
			t.Fatalf("list human failed: %d, %s %s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "sources/upstream") {
			t.Fatalf("expected repository heading in human output, got:\n%s", stdout)
		}
		if !strings.Contains(stdout, "learning-source") && !strings.Contains(stdout, "upstream") {
			t.Fatalf("expected role in human output, got:\n%s", stdout)
		}
	})

	t.Run("source watch without skill-id exits 2", func(t *testing.T) {
		root := initTestWorkspace(t)
		code, stdout, stderr := runCLIForTest([]string{"source", "watch", "https://github.com/example/skills.git", "--workspace", root})
		if code != 2 {
			t.Fatalf("expected exit 2, got %d, out=%s, err=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "A watched source must belong to a skill.") {
			t.Fatalf("expected 'A watched source must belong to a skill.', got:\n%s", stderr)
		}
	})

	t.Run("source backfill without candidates prints Nothing to backfill", func(t *testing.T) {
		root := initTestWorkspace(t)
		code, stdout, stderr := runCLIForTest([]string{"source", "backfill", "--workspace", root})
		if code != 0 {
			t.Fatalf("expected exit 0, got %d, out=%s, err=%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "Nothing to backfill.") {
			t.Fatalf("expected 'Nothing to backfill.', got:\n%s", stdout)
		}
	})

	t.Run("source unwatch missing-id exits 2", func(t *testing.T) {
		root := initTestWorkspace(t)
		code, stdout, stderr := runCLIForTest([]string{"source", "unwatch", "missing-id", "--workspace", root})
		if code != 2 {
			t.Fatalf("expected exit 2, got %d, out=%s, err=%s", code, stdout, stderr)
		}
	})
}

func TestSourceBackfillFillsUpstreamIDAndKeepsOtherMetadata(t *testing.T) {
	t.Parallel()
	root := newSourceCLIWorkspace(t)
	writeMeta := func(id, meta string) string {
		t.Helper()
		if c, _, se := runCLIForTest([]string{"skill", "create", "--workspace", root, "--id", id, "--collection", "default", "--name", id, "--description", "Backfill fixture", "--yes"}); c != 0 {
			t.Fatalf("skill create %s failed: %s", id, se)
		}
		path := filepath.Join(root, "skills", "default", id, ".meta", "skill.yaml")
		if err := os.WriteFile(path, []byte(meta), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	mixedPath := writeMeta("mixed-skill", `description: Backfill fixture
id: mixed-skill
name: mixed-skill
provenance:
    created_by: skill_add
schema_version: 1
sources:
    - folder_digest: sha256:0382941395c0d6eca961c5a7005d8034c3d1ce690985e362147e35603d15c71d
      id: ""
      kind: github
      ref: main
      repo: https://github.com/example/mixed-upstream
      roles:
        - upstream
    - id: notes-src
      kind: github
      repo: https://github.com/example/notes
      roles:
        - learning
status: draft
`)
	lonePath := writeMeta("lone-skill", `description: Backfill fixture
id: lone-skill
name: lone-skill
schema_version: 1
sources:
    - id: ""
      kind: github
      ref: main
      repo: https://github.com/example/lone-upstream
      roles:
        - upstream
status: draft
`)
	commitCLIWorkspace(t, root)

	for _, id := range []string{"mixed-skill", "lone-skill"} {
		code, stdout, stderr := runCLIForTest([]string{"source", "backfill", "--skill", id, "--yes", "--workspace", root})
		if code != 0 {
			t.Fatalf("backfill %s=%d stdout=%s stderr=%s", id, code, stdout, stderr)
		}
	}

	type meta struct {
		Provenance map[string]any   `yaml:"provenance"`
		Sources    []map[string]any `yaml:"sources"`
	}
	read := func(path string) meta {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc meta
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	mixed := read(mixedPath)
	if mixed.Provenance["created_by"] != "skill_add" {
		t.Fatalf("provenance was not kept: %#v", mixed.Provenance)
	}
	if len(mixed.Sources) != 2 || mixed.Sources[0]["id"] != "mixed-upstream" || mixed.Sources[0]["folder_digest"] == nil || mixed.Sources[1]["id"] != "notes-src" {
		t.Fatalf("mixed-skill sources = %#v", mixed.Sources)
	}
	lone := read(lonePath)
	if len(lone.Sources) != 1 || lone.Sources[0]["id"] != "lone-upstream" {
		t.Fatalf("lone-skill sources = %#v", lone.Sources)
	}
}

func runCLIForTest(args []string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}
