package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func (adapter *Server) registerInsightTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "skill_update_preview",
		Title:       "Preview skill update",
		Description: "Preview a skill update and return confirmation pins. An updated runtime block requires re-review with `skillhub skill review <id>`.",
		Annotations: annotations(false, false, false, false),
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, input skillUpdatePreviewInput) (*mcp.CallToolResult, toolOutcome[app.SkillProposal], error) {
			update := skill.UpdateInput{
				IdempotencyKey:        input.IdempotencyKey,
				Name:                  input.Name,
				Description:           input.Description,
				ExpectedContentDigest: strings.TrimSpace(input.ExpectedContentDigest),
				Routing:               input.Routing,
				Rationale:             input.Rationale,
				Runtime:               input.Runtime,
			}
			if input.Content != nil {
				update.SetContent = true
				update.Content = []byte(*input.Content)
			}
			if input.Name == nil && input.Description == nil && input.Content == nil && input.Routing == nil && input.Rationale == nil && input.Runtime == nil && input.ExpectedContentDigest == "" {
				return failure[app.SkillProposal](fmt.Errorf("at least one update field is required"))
			}
			proposal, err := (app.SkillService{}).PreviewSkillUpdate(ctx, adapter.workspace, input.SkillID, update, input.FullDiff)
			if err != nil {
				if errors.Is(err, skill.ErrNotFound) || strings.Contains(err.Error(), "does not exist; use skill_create_preview") {
					item := toolError{
						Code:            "invalid_request",
						Message:         fmt.Sprintf("Skill %s does not exist; use skill_create_preview", input.SkillID),
						Retryable:       false,
						SuggestedAction: "Use skill_create_preview to create a new draft skill.",
					}
					return &mcp.CallToolResult{IsError: true}, toolOutcome[app.SkillProposal]{SchemaVersion: SchemaVersion, Error: &item}, nil
				}
				return failure[app.SkillProposal](err)
			}
			return success(proposal)
		})

	addTool(server, &mcp.Tool{
		Name:        "skill_update_confirm",
		Title:       "Confirm skill update",
		Description: "Apply a persisted skill proposal with required confirmation pins.",
		Annotations: annotations(false, true, true, false),
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SkillMutationResult], error) {
			if strings.TrimSpace(input.ProposalID) == "" || strings.TrimSpace(input.ProposalDigest) == "" || strings.TrimSpace(input.BaseVersion) == "" {
				return failure[app.SkillMutationResult](app.NewInvalidRequestError("proposal_id, proposal_digest, and base_version pins are required", "Supply all confirmation pins."))
			}
			service := app.SkillService{}
			preview, err := service.LoadSkillProposal(ctx, adapter.workspace, input.ProposalID)
			if err != nil {
				return failure[app.SkillMutationResult](err)
			}
			return appResult(service.ConfirmSkillMutation(ctx, adapter.workspace, preview, app.ConfirmationPins{ProposalID: input.ProposalID, ProposalDigest: input.ProposalDigest, BaseVersion: input.BaseVersion}))
		})

	addTool(server, &mcp.Tool{
		Name:        "routing_evaluate",
		Title:       "Evaluate routing",
		Description: "Evaluate a routing request with the deterministic resolver against the current catalog. This read-only evaluation never changes policy.",
		Annotations: annotations(true, false, true, false),
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, input resolveInput) (*mcp.CallToolResult, toolOutcome[routingEvaluationResult], error) {
			request := resolverpkg.Request(input)
			if _, err := resolverpkg.NormalizeRequest(request); err != nil {
				return failure[routingEvaluationResult](err)
			}
			response, err := (app.ResolverService{}).Resolve(ctx, adapter.workspace, request)
			if err != nil {
				return failure[routingEvaluationResult](err)
			}
			return success(routingEvaluationResult{Resolution: response, Mutation: false})
		})
}
