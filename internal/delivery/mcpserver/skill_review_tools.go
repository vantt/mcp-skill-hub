package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func (adapter *Server) registerSkillReviewTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "skill_review",
		Title:       "Review skill diagnostics",
		Description: "Perform an offline diagnostic review for one skill, returning readiness, validation issues, resource divergence, and git status.",
		Annotations: annotations(true, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillReviewInput) (*mcp.CallToolResult, toolOutcome[app.SkillReviewResult], error) {
		id := strings.TrimSpace(input.SkillID)
		if id == "" {
			return failure[app.SkillReviewResult](fmt.Errorf("skill_id is required"))
		}
		res, err := (app.SkillService{}).ReviewSkill(ctx, adapter.workspace, id)
		if err != nil {
			if errors.Is(err, skill.ErrNotFound) {
				item := toolError{
					Code:            "not_found",
					Message:         fmt.Sprintf("Skill %s not found", id),
					Retryable:       false,
					SuggestedAction: "Run skill_list to view available skills.",
				}
				return &mcp.CallToolResult{IsError: true}, toolOutcome[app.SkillReviewResult]{SchemaVersion: SchemaVersion, Error: &item}, nil
			}
			return failure[app.SkillReviewResult](err)
		}
		return appResult(res, nil)
	})
}
