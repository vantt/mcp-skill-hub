package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

const testSentinelString = "SENTINEL_UPSTREAM_CONTENT_SECRET_12345"

func seedTrackedSkillForMCP(t *testing.T, root, skillID, sourceID string, origin app.SkillOrigin) {
	t.Helper()
	service := app.SkillService{}
	prev, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID:          skillID,
		Collection:  "default",
		Name:        skillID,
		Description: "Tracked skill " + skillID,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(context.Background(), root, prev, prev.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	skillMDBytes, _ := os.ReadFile(filepath.Join(root, "skills", "default", skillID, "SKILL.md"))
	skillMDDigest := sourcepkg.Digest(skillMDBytes)
	cleanFilesDigest := skillruntime.ContentDigest([]skillruntime.ResourceDigest{{Path: "SKILL.md", Digest: skillMDDigest}}, skillruntime.Spec{}, false)

	filesDigest := origin.FilesDigest
	if filesDigest == "" || strings.HasPrefix(filesDigest, "sha256:00000000") {
		filesDigest = cleanFilesDigest
	}

	metaPath := filepath.Join(root, "skills", "default", skillID, "skill.meta.yaml")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(metaBytes, &doc); err != nil {
		t.Fatal(err)
	}

	doc["provenance"] = map[string]any{
		"source_id": sourceID,
		"origin": map[string]any{
			"kind":         origin.Kind,
			"repository":   origin.Repository,
			"ref":          origin.Ref,
			"path":         origin.Path,
			"commit":       origin.Commit,
			"files_digest": filesDigest,
		},
	}
	newMetaBytes, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, newMetaBytes, 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedSourceRecordForMCP(t *testing.T, root, sourceID, repo, ref string) {
	t.Helper()
	rev := sourcepkg.Revision{
		Kind:          "git-commit",
		Value:         "1111222233334444555566667777888899990000",
		ContentDigest: sourcepkg.Digest([]byte("1111222233334444555566667777888899990000")),
		ObservedAt:    time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
	}
	rec := sourcepkg.Record{
		SchemaVersion:   1,
		ID:              sourceID,
		Adapter:         "git",
		Locator:         sourcepkg.Locator{Repository: repo, Ref: ref},
		Status:          "watching",
		Identity:        sourcepkg.Identity{Name: sourceID, Canonical: repo, DefaultBranch: ref},
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Limits:          sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
		CurrentRevision: &rev,
	}
	data, err := sourcepkg.MarshalCanonical(rec)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, "sources", "catalog"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", sourceID+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func recordUpstreamStateForMCP(t *testing.T, root, skillID, sourceID, repo, ref, path, baseCommit, latestCommit string, status string) {
	t.Helper()
	store := sourcepkg.OperationalStore{Root: root}
	changes := []sourcepkg.Change{
		{
			Path:   "SKILL.md",
			Status: "modified",
		},
	}
	now := time.Now().UTC()
	state := sourcepkg.UpstreamState{
		SkillID:         skillID,
		SourceID:        sourceID,
		Repository:      repo,
		Ref:             ref,
		Path:            path,
		BaseCommit:      baseCommit,
		CheckedCommit:   latestCommit,
		CheckedCommitAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
		Upstream:        status,
		ChangedFiles:    changes,
		CheckedAt:       now,
	}
	if err := store.RecordUpstream(context.Background(), []sourcepkg.UpstreamState{state}); err != nil {
		t.Fatal(err)
	}
}

func TestUpstreamTools(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)

	sourceID := "test-upstream-source"
	repo := "https://github.com/example/skills"
	ref := "main"
	skillID := "test-skill"
	baseCommit := "3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a"
	latestCommit := "81d04be7c2aa1111222233334444555566667777"

	seedSourceRecordForMCP(t, root, sourceID, repo, ref)
	seedTrackedSkillForMCP(t, root, skillID, sourceID, app.SkillOrigin{
		Kind:       "github",
		Repository: repo,
		Ref:        ref,
		Path:       "skills/test-skill",
		Commit:     baseCommit,
	})
	recordUpstreamStateForMCP(t, root, skillID, sourceID, repo, ref, "skills/test-skill", baseCommit, latestCommit, "changed")

	session := connectInMemoryServer(t, root)
	ctx := context.Background()

	// 1. Call skill_upstream_status without skill_id -> list returns update_available
	listRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "skill_upstream_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool skill_upstream_status error: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("unexpected tool error: %#v", listRes)
	}

	var listOutcome toolOutcome[skillUpstreamStatusResult]
	decodeStructuredContent(t, listRes, &listOutcome)
	if len(listOutcome.Result.Skills) != 1 {
		t.Fatalf("expected 1 skill in status result, got %d", len(listOutcome.Result.Skills))
	}
	item := listOutcome.Result.Skills[0]
	if item.Status != "update_available" {
		t.Fatalf("expected status 'update_available', got %q", item.Status)
	}
	if item.NextAction != "skillhub skill update "+skillID {
		t.Fatalf("expected next_action 'skillhub skill update %s', got %q", skillID, item.NextAction)
	}
	if item.WebUIHint != "Open the skill in the Skill Hub WebUI → Sources tab → Review update." {
		t.Fatalf("expected WebUIHint, got %q", item.WebUIHint)
	}

	// 2. Structured content leak tests:
	// Verify raw JSON does NOT contain sentinel string or forbidden keys
	rawJSONBytes, err := json.Marshal(listRes.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	rawJSON := string(rawJSONBytes)

	if strings.Contains(rawJSON, testSentinelString) {
		t.Fatalf("structured content leaked sentinel string!")
	}
	forbiddenKeys := []string{
		`"upstream_diff"`,
		`"local_diff"`,
		`"result_diff"`,
		`"merged_with_markers"`,
		`"confirmation"`,
	}
	for _, k := range forbiddenKeys {
		if strings.Contains(rawJSON, k) {
			t.Fatalf("structured content contains forbidden key %s:\n%s", k, rawJSON)
		}
	}

	// 3. Verify no raw control characters in any path or summary
	for _, b := range rawJSONBytes {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			t.Fatalf("structured content contains raw control byte: 0x%02x", b)
		}
	}

	// 4. Verify tools/list contains NO tool whose name starts with skill_upstream_update
	toolsRes, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range toolsRes.Tools {
		if strings.HasPrefix(tool.Name, "skill_upstream_update") {
			t.Fatalf("tools/list unexpectedly exposes upstream update tool: %s", tool.Name)
		}
	}

	// 5. Calling skill_transition_confirm with the ID of an upstream_update proposal returns error
	// Store a dummy proposal with command "upstream_update"
	fakeUpstreamProposalID := "PROP-test-upstream-forbidden"
	setProposalInWorkspace(t, root, fakeUpstreamProposalID, "upstream_update")

	transConfRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "skill_transition_confirm",
		Arguments: map[string]any{
			"proposal_id":     fakeUpstreamProposalID,
			"proposal_digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			"base_version":    "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !transConfRes.IsError {
		t.Fatalf("expected skill_transition_confirm to reject upstream_update proposal, got success")
	}
}

func setProposalInWorkspace(t *testing.T, root, proposalID, command string) {
	t.Helper()
	dir := filepath.Join(root, "runtime", "proposals")
	_ = os.MkdirAll(dir, 0o755)
	doc := map[string]any{
		"proposal_id": proposalID,
		"write_set": map[string]any{
			"command": command,
		},
		"confirmation": map[string]any{
			"application_command": command,
		},
	}
	data, _ := json.Marshal(doc)
	_ = os.WriteFile(filepath.Join(dir, proposalID+".json"), data, 0o644)
}
