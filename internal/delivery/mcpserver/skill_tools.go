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

func (adapter *Server) registerSkillTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "skill_create_preview",
		Title:       "Preview skill creation",
		Description: "Persist a validated draft skill creation preview and return exact confirmation pins. No canonical skill files change during preview.",
		Annotations: annotations(false, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillCreatePreviewInput) (*mcp.CallToolResult, toolOutcome[app.SkillProposal], error) {
		if strings.TrimSpace(input.SkillID) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Description) == "" {
			return failure[app.SkillProposal](fmt.Errorf("skill_id, name, and description are required"))
		}
		collection := strings.TrimSpace(input.Collection)
		if collection == "" {
			collection = "core"
		}
		createInput := skill.CreateInput{
			ID:             strings.TrimSpace(input.SkillID),
			IdempotencyKey: input.IdempotencyKey,
			Collection:     collection,
			Name:           strings.TrimSpace(input.Name),
			Description:    strings.TrimSpace(input.Description),
		}
		if input.Content != nil {
			createInput.Content = []byte(*input.Content)
		}
		if input.Routing != nil {
			createInput.Routing = *input.Routing
		}
		if input.Rationale != nil {
			createInput.Rationale = strings.TrimSpace(*input.Rationale)
		}
		return appResult((app.SkillService{}).PreviewCreate(ctx, adapter.workspace, createInput, input.FullDiff))
	})

	addTool(server, &mcp.Tool{
		Name:        "skill_create_confirm",
		Title:       "Confirm skill creation",
		Description: "Apply exactly one persisted draft skill creation proposal. All proposal_id, proposal_digest, and base_version pins are required and exact replay returns the prior receipt.",
		Annotations: annotations(false, true, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SkillMutationResult], error) {
		if strings.TrimSpace(input.ProposalID) == "" || strings.TrimSpace(input.ProposalDigest) == "" || strings.TrimSpace(input.BaseVersion) == "" {
			return failure[app.SkillMutationResult](app.NewInvalidRequestError(
				"proposal_id, proposal_digest, and base_version pins are required",
				"Supply all confirmation pins.",
			))
		}
		service := app.SkillService{}
		preview, err := service.LoadSkillProposal(ctx, adapter.workspace, input.ProposalID)
		if err != nil {
			return failure[app.SkillMutationResult](err)
		}
		return appResult(service.ConfirmSkillMutation(ctx, adapter.workspace, preview, app.ConfirmationPins{
			ProposalID:     input.ProposalID,
			ProposalDigest: input.ProposalDigest,
			BaseVersion:    input.BaseVersion,
		}))
	})

	addTool(server, &mcp.Tool{
		Name:        "skill_transition_preview",
		Title:       "Preview skill lifecycle transition",
		Description: "Persist a validated skill lifecycle transition preview (target: active, deprecated, or archived) and return exact confirmation pins. No canonical skill files change during preview.",
		Annotations: annotations(false, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillTransitionPreviewInput) (*mcp.CallToolResult, toolOutcome[app.SkillProposal], error) {
		if strings.TrimSpace(input.SkillID) == "" {
			return failure[app.SkillProposal](fmt.Errorf("skill_id is required"))
		}
		target := strings.TrimSpace(input.Target)
		if target != "active" && target != "deprecated" && target != "archived" {
			return failure[app.SkillProposal](fmt.Errorf("target must be active, deprecated, or archived"))
		}
		proposal, err := (app.SkillService{}).PreviewTransitionWithKey(ctx, adapter.workspace, input.SkillID, target, input.FullDiff, input.IdempotencyKey)
		if err != nil {
			var missingErr *app.MissingActivationRequirementsError
			if errors.As(err, &missingErr) {
				item := toolError{
					Code:            "invalid_request",
					Message:         missingErr.Error(),
					Retryable:       false,
					SuggestedAction: fmt.Sprintf("Run skill_update_preview to configure the missing fields (%s) before activating.", strings.Join(missingErr.Missing, ", ")),
				}
				return &mcp.CallToolResult{IsError: true}, toolOutcome[app.SkillProposal]{SchemaVersion: SchemaVersion, Error: &item}, nil
			}
			return failure[app.SkillProposal](err)
		}
		return success(proposal)
	})

	addTool(server, &mcp.Tool{
		Name:        "skill_transition_confirm",
		Title:       "Confirm skill lifecycle transition",
		Description: "Apply exactly one persisted skill lifecycle transition proposal. All proposal_id, proposal_digest, and base_version pins are required and exact replay returns the prior receipt.",
		Annotations: annotations(false, true, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SkillMutationResult], error) {
		if strings.TrimSpace(input.ProposalID) == "" || strings.TrimSpace(input.ProposalDigest) == "" || strings.TrimSpace(input.BaseVersion) == "" {
			return failure[app.SkillMutationResult](app.NewInvalidRequestError(
				"proposal_id, proposal_digest, and base_version pins are required",
				"Supply all confirmation pins.",
			))
		}
		service := app.SkillService{}
		preview, err := service.LoadSkillProposal(ctx, adapter.workspace, input.ProposalID)
		if err != nil {
			return failure[app.SkillMutationResult](err)
		}
		return appResult(service.ConfirmSkillMutation(ctx, adapter.workspace, preview, app.ConfirmationPins{
			ProposalID:     input.ProposalID,
			ProposalDigest: input.ProposalDigest,
			BaseVersion:    input.BaseVersion,
		}))
	})

	addTool(server, &mcp.Tool{
		Name:        "skill_list",
		Title:       "List skills",
		Description: "List skills from the current catalog generation, optionally filtered by state (draft, active, deprecated, archived).",
		Annotations: annotations(true, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillListInput) (*mcp.CallToolResult, toolOutcome[app.SkillListResult], error) {
		return appResult((app.SkillService{}).ListSkills(ctx, adapter.workspace, strings.TrimSpace(input.State)))
	})

	addTool(server, &mcp.Tool{
		Name:        "skill_get",
		Title:       "Get skill",
		Description: "Get a single skill by ID in any lifecycle state, returning its routing fields, entrypoint path, content, and metadata.",
		Annotations: annotations(true, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillGetInput) (*mcp.CallToolResult, toolOutcome[skillGetResult], error) {
		id := strings.TrimSpace(input.SkillID)
		if id == "" {
			return failure[skillGetResult](fmt.Errorf("skill_id is required"))
		}
		detail, err := (app.SkillService{}).GetSkillDetail(ctx, adapter.workspace, id)
		if err != nil {
			if errors.Is(err, skill.ErrNotFound) {
				item := toolError{
					Code:            "not_found",
					Message:         fmt.Sprintf("Skill %s not found", id),
					Retryable:       false,
					SuggestedAction: "Run skill_list to view available skills.",
				}
				return &mcp.CallToolResult{IsError: true}, toolOutcome[skillGetResult]{SchemaVersion: SchemaVersion, Error: &item}, nil
			}
			return failure[skillGetResult](err)
		}
		return success(detail)
	})
}
