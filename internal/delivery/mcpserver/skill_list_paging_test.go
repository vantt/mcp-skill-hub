package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func TestSkillListPaginationAndCurationProfileOnly(t *testing.T) {
	root := newMCPWorkspace(t)

	// 1. Verify ProfileRuntime does NOT contain skill_list
	_, runtimeServer, err := New(root, nil, ProfileRuntime)
	if err != nil {
		t.Fatalf("New ProfileRuntime: %v", err)
	}
	runtimeSession := newClientSession(t, runtimeServer)
	runtimeTools, err := runtimeSession.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("runtime ListTools: %v", err)
	}
	for _, tool := range runtimeTools.Tools {
		if tool.Name == "skill_list" {
			t.Fatal("skill_list must not be present in ProfileRuntime")
		}
	}

	// 2. Verify ProfileCuration contains skill_list with updated description
	_, curationServer, err := New(root, nil, ProfileCuration)
	if err != nil {
		t.Fatalf("New ProfileCuration: %v", err)
	}
	session := newClientSession(t, curationServer)
	curationTools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("curation ListTools: %v", err)
	}
	var skillListTool *mcp.Tool
	for _, tool := range curationTools.Tools {
		if tool.Name == "skill_list" {
			skillListTool = tool
			break
		}
	}
	if skillListTool == nil {
		t.Fatal("skill_list must be present in ProfileCuration")
	}
	wantDesc := "for curation; to pick a skill for a task, call skill_resolve"
	if skillListTool.Description != wantDesc {
		t.Fatalf("expected description %q, got %q", wantDesc, skillListTool.Description)
	}

	// 3. Create 4 additional skills so we have 5 skills total (review-skill already in workspace)
	service := app.SkillService{}
	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("extra-skill-%d", i)
		content := []byte(fmt.Sprintf("---\nname: %s\ndescription: Test skill %d.\n---\n\n# Skill\n", id, i))
		created, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
			ID:          id,
			Collection:  "core",
			Name:        fmt.Sprintf("Extra Skill %d", i),
			Description: fmt.Sprintf("Test skill %d.", i),
			Content:     content,
		}, false)
		if err != nil {
			t.Fatalf("PreviewCreate %s: %v", id, err)
		}
		if result, err := service.ConfirmSkillMutation(context.Background(), root, created, created.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
			t.Fatalf("ConfirmSkillMutation %s: %#v, %v", id, result, err)
		}
	}
	if _, err := (app.CatalogService{}).EnsureCatalog(t.Context(), root); err != nil {
		t.Fatalf("EnsureCatalog: %v", err)
	}

	// 4. Default limit ~50 returns all skills with has_more=false
	defaultRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_list",
		Arguments: map[string]any{},
	})
	if err != nil || defaultRes.IsError {
		t.Fatalf("default skill_list failed: %#v, %v", defaultRes, err)
	}
	var defaultOutcome toolOutcome[app.SkillListResult]
	decodeStructuredContent(t, defaultRes, &defaultOutcome)
	if defaultOutcome.Result == nil || len(defaultOutcome.Result.Skills) < 5 || defaultOutcome.Result.HasMore {
		t.Fatalf("default listing mismatch: %#v", defaultOutcome)
	}

	// 5. Test pagination with limit=2
	page1Res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_list",
		Arguments: map[string]any{
			"limit": 2,
		},
	})
	if err != nil || page1Res.IsError {
		t.Fatalf("page 1 failed: %#v, %v", page1Res, err)
	}
	var page1Outcome toolOutcome[app.SkillListResult]
	decodeStructuredContent(t, page1Res, &page1Outcome)
	if page1Outcome.Result == nil || len(page1Outcome.Result.Skills) != 2 || !page1Outcome.Result.HasMore || page1Outcome.Result.NextCursor == "" {
		t.Fatalf("page 1 mismatch: %#v", page1Outcome)
	}

	// Fetch page 2
	page2Res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_list",
		Arguments: map[string]any{
			"limit":  2,
			"cursor": page1Outcome.Result.NextCursor,
		},
	})
	if err != nil || page2Res.IsError {
		t.Fatalf("page 2 failed: %#v, %v", page2Res, err)
	}
	var page2Outcome toolOutcome[app.SkillListResult]
	decodeStructuredContent(t, page2Res, &page2Outcome)
	if page2Outcome.Result == nil || len(page2Outcome.Result.Skills) != 2 || !page2Outcome.Result.HasMore || page2Outcome.Result.NextCursor == "" {
		t.Fatalf("page 2 mismatch: %#v", page2Outcome)
	}

	// Fetch page 3 (remaining 1 item)
	page3Res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_list",
		Arguments: map[string]any{
			"limit":  2,
			"cursor": page2Outcome.Result.NextCursor,
		},
	})
	if err != nil || page3Res.IsError {
		t.Fatalf("page 3 failed: %#v, %v", page3Res, err)
	}
	var page3Outcome toolOutcome[app.SkillListResult]
	decodeStructuredContent(t, page3Res, &page3Outcome)
	if page3Outcome.Result == nil || len(page3Outcome.Result.Skills) != 1 || page3Outcome.Result.HasMore || page3Outcome.Result.NextCursor != "" {
		t.Fatalf("page 3 mismatch: %#v", page3Outcome)
	}

	// Ensure no duplicate IDs across pages
	seenIDs := make(map[string]bool)
	for _, p := range []toolOutcome[app.SkillListResult]{page1Outcome, page2Outcome, page3Outcome} {
		for _, s := range p.Result.Skills {
			if seenIDs[s.ID] {
				t.Fatalf("duplicate skill ID across pages: %s", s.ID)
			}
			seenIDs[s.ID] = true
		}
	}
	if len(seenIDs) != 5 {
		t.Fatalf("expected 5 distinct skills across pages, got %d", len(seenIDs))
	}

	// 6. Test invalid limit (> 100)
	invalidLimitRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_list",
		Arguments: map[string]any{
			"limit": 150,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !invalidLimitRes.IsError {
		t.Fatal("expected limit 150 to return error")
	}
	// Schema validator rejects limit > 100
	errorText := ""
	if len(invalidLimitRes.Content) > 0 {
		if tc, ok := invalidLimitRes.Content[0].(*mcp.TextContent); ok {
			errorText = tc.Text
		}
	}
	if !strings.Contains(errorText, "limit") {
		t.Fatalf("expected schema error mentioning limit, got text=%q", errorText)
	}
	// 7. Test invalid/tampered cursor
	badCursorRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_list",
		Arguments: map[string]any{
			"cursor": "invalid-cursor",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !badCursorRes.IsError {
		t.Fatal("expected invalid cursor to return error")
	}
	var badCursorOutcome toolOutcome[app.SkillListResult]
	decodeStructuredContent(t, badCursorRes, &badCursorOutcome)
	if badCursorOutcome.Error == nil || badCursorOutcome.Error.Code != "snapshot_expired" {
		t.Fatalf("expected snapshot_expired error for bad cursor, got: %#v", badCursorOutcome)
	}
}
