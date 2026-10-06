package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

func (adapter *Server) registerInsightTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{Name: "inbox_list", Title: "List insight inbox", Description: "Return a snapshot-bound page of ranked insight groups. Nothing is auto-adopted.", Annotations: annotations(true, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input pageInput) (*mcp.CallToolResult, toolOutcome[paging.Page[app.InsightInboxGroup]], error) {
			limit, err := paging.NormalizeLimit(input.Limit)
			if err != nil {
				return failure[paging.Page[app.InsightInboxGroup]](err)
			}
			result, err := (app.InsightService{}).GetInsightInbox(ctx, adapter.workspace)
			if err != nil {
				return failure[paging.Page[app.InsightInboxGroup]](err)
			}
			filter := "inbox"
			owner := paging.Owner(filter, result.Groups)
			lastKey, err := paging.DecodeCursor(input.Cursor, owner, filter)
			if err != nil {
				return failure[paging.Page[app.InsightInboxGroup]](fmt.Errorf("snapshot_expired: inbox cursor is invalid or expired"))
			}
			paged, err := paging.Make(result.Groups, limit, lastKey, owner, filter, func(item app.InsightInboxGroup) string { return item.SkillID + "\x00" + item.Category })
			if err != nil {
				return failure[paging.Page[app.InsightInboxGroup]](fmt.Errorf("snapshot_expired: inbox cursor is invalid or expired"))
			}
			return success(paged)
		})

	addTool(server, &mcp.Tool{Name: "insight_get", Title: "Get insight", Description: "Load one insight and its bounded supporting findings and comparisons on demand.", Annotations: annotations(true, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input insightIDInput) (*mcp.CallToolResult, toolOutcome[app.InsightDetailResult], error) {
			return appResult((app.InsightService{}).GetInsightDetail(ctx, adapter.workspace, input.InsightID))
		})
	addTool(server, &mcp.Tool{Name: "insight_decide", Title: "Decide insight", Description: "Explicitly plan, reject, obsolete, or reopen one insight with rationale.", Annotations: annotations(false, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input insightDecideInput) (*mcp.CallToolResult, toolOutcome[app.InsightDecisionResult], error) {
			return appResult((app.InsightService{}).DecideInsight(ctx, adapter.workspace, input.InsightID, app.InsightDecisionInput{Decision: input.Decision, Rationale: input.Rationale, IdempotencyKey: input.IdempotencyKey}))
		})
	addTool(server, &mcp.Tool{Name: "insight_apply_preview", Title: "Preview insight application", Description: "Persist a bounded, immutable application preview and return proposal_id, proposal_digest, and base_version. This does not change canonical skill files.", Annotations: annotations(false, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input insightApplyPreviewInput) (*mcp.CallToolResult, toolOutcome[app.InsightApplicationPreview], error) {
			return appResult((app.InsightService{}).PreviewInsightApplication(ctx, adapter.workspace, input.InsightID, input.Input))
		})
	addTool(server, &mcp.Tool{Name: "insight_apply_confirm", Title: "Confirm insight application", Description: "Apply exactly one persisted insight proposal. All proposal_id, proposal_digest, and base_version pins are required and exact replay returns the prior receipt.", Annotations: annotations(false, true, true, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.InsightApplicationResult], error) {
			if strings.TrimSpace(input.ProposalID) == "" || strings.TrimSpace(input.ProposalDigest) == "" || strings.TrimSpace(input.BaseVersion) == "" {
				return failure[app.InsightApplicationResult](app.NewInvalidRequestError("proposal_id, proposal_digest, and base_version pins are required", "Supply all confirmation pins."))
			}
			return appResult((app.InsightService{}).ConfirmInsightApplication(ctx, adapter.workspace, input.ProposalID, input.ProposalDigest, input.BaseVersion))
		})

	addTool(server, &mcp.Tool{Name: "skill_update_preview", Title: "Preview skill update", Description: "Persist a validated skill update preview and return exact confirmation pins. No canonical skill files change during preview. runtime replaces the skill's runtime block (requires.bins/env/platforms, setup.check/command; an empty object removes it); changing it requires the user to review the skill again before its files are served to agents.", Annotations: annotations(false, false, false, false)},
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
	addTool(server, &mcp.Tool{Name: "skill_update_confirm", Title: "Confirm skill update", Description: "Apply exactly one persisted skill proposal. All proposal_id, proposal_digest, and base_version pins are required and exact replay returns the prior receipt.", Annotations: annotations(false, true, true, false)},
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

	addTool(server, &mcp.Tool{Name: "routing_evaluate", Title: "Evaluate routing", Description: "Evaluate one versioned request with the same deterministic resolver, catalog snapshot, policy revision, and distribution pins used by skill_resolve. This read-only evaluation never changes policy.", Annotations: annotations(true, false, true, false)},
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

	addTool(server, &mcp.Tool{Name: "outcome_record", Title: "Record outcome", Description: "Record a deduplicated explicit incorporation outcome from supplied evidence. It never infers usefulness from activation or use.", Annotations: annotations(false, false, true, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input outcomeRecordInput) (*mcp.CallToolResult, toolOutcome[app.OutcomeResult], error) {
			if input.EventID == "" {
				return failure[app.OutcomeResult](fmt.Errorf("event_id is required for idempotency"))
			}
			return appResult((app.InsightService{}).RecordIncorporationOutcome(ctx, adapter.workspace, input.IncorporationID, app.OutcomeInput{State: input.State, Evidence: input.Evidence, Note: input.Note, Supersedes: input.Supersedes, IdempotencyKey: input.EventID}))
		})
}
