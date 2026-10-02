package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillReviewOfflineWithoutCatalog(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	// Create a draft skill canonical files directly without catalog rebuild
	skillDir := filepath.Join(root, "skills", "core", "diagnostic-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := "schema_version: 1\nid: diagnostic-skill\nname: diagnostic-skill\nstatus: draft\ndescription: Offline diagnostic review test.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: diagnostic-skill\ndescription: Offline diagnostic review test.\n---\n# Diagnostic Skill\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	// Corrupt or remove the catalog pointer to prove review works offline
	pointerPath := filepath.Join(root, "catalog", "generation.json")
	_ = os.Remove(pointerPath)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_review",
		Arguments: map[string]any{
			"skill_id": "diagnostic-skill",
		},
	})
	if err != nil {
		t.Fatalf("call skill_review error: %v", err)
	}
	if res.IsError {
		t.Fatalf("skill_review failed offline: %#v", res)
	}
	var outcome toolOutcome[app.SkillReviewResult]
	decodeStructuredContent(t, res, &outcome)
	if outcome.Result == nil {
		t.Fatal("expected non-nil review result")
	}
	if outcome.Result.SkillID != "diagnostic-skill" {
		t.Fatalf("skill_id = %q, want diagnostic-skill", outcome.Result.SkillID)
	}
	if outcome.Result.LifecycleState != "draft" {
		t.Fatalf("lifecycle_state = %q, want draft", outcome.Result.LifecycleState)
	}
	if outcome.Result.RoutingEligible {
		t.Fatal("draft skill should not be routing eligible")
	}
	if outcome.Result.NextAction == "" {
		t.Fatal("expected non-empty next action")
	}
}

func TestSkillReviewNonexistent(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_review",
		Arguments: map[string]any{
			"skill_id": "nonexistent-skill",
		},
	})
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected isError=true for nonexistent skill review")
	}
	var outcome toolOutcome[app.SkillReviewResult]
	decodeStructuredContent(t, res, &outcome)
	if outcome.Error == nil || (outcome.Error.Code != "not_found" && outcome.Error.Code != "invalid_request") {
		t.Fatalf("expected not_found or invalid_request, got %#v", outcome.Error)
	}
}

func TestSkillReviewWhenCanonicalCorrupt(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	// Create a skill with corrupt/invalid YAML
	skillDir := filepath.Join(root, "skills", "core", "invalid-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	invalidMeta := "schema_version: 1\nid: invalid-skill\nstatus: not-a-valid-status\n"
	if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(invalidMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_review",
		Arguments: map[string]any{
			"skill_id": "invalid-skill",
		},
	})
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if res.IsError {
		t.Fatalf("skill_review should succeed diagnositically even when canonical is invalid: %#v", res)
	}
	var outcome toolOutcome[app.SkillReviewResult]
	decodeStructuredContent(t, res, &outcome)
	if outcome.Result == nil {
		t.Fatal("expected result")
	}
	if outcome.Result.Valid {
		t.Fatal("expected valid=false for invalid skill")
	}
	if len(outcome.Result.CanonicalIssues) == 0 {
		t.Fatal("expected canonical issues reported")
	}
}
