package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/delivery/paging"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"github.com/vantt/skillhub-mcp/internal/systemskills"
)

func (adapter *Server) registerSkillCurationTools(tools *toolRegistry) {
	adapter.registerSkillCreateTools(tools)
	adapter.registerSkillTransitionTools(tools)
	adapter.registerSkillListTool(tools)
}

func (adapter *Server) registerSkillCreateTools(tools *toolRegistry) {
	addTool(tools, &mcp.Tool{
		Name:        "skill_create_preview",
		Title:       "Preview skill creation",
		Description: "Preview creating a draft skill and return confirmation pins.",
		Annotations: annotations(false, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillCreatePreviewInput) (*mcp.CallToolResult, toolOutcome[app.SkillProposal], error) {
		if strings.TrimSpace(input.SkillID) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Description) == "" {
			return failure[app.SkillProposal](fmt.Errorf("skill_id, name, and description are required"))
		}
		collection := strings.TrimSpace(input.Collection)
		if collection == "" {
			collection = skill.DefaultCollection
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

	addTool(tools, &mcp.Tool{
		Name:        "skill_create_confirm",
		Title:       "Confirm skill creation",
		Description: "Apply a persisted draft skill creation proposal with required confirmation pins.",
		Annotations: annotations(false, true, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SkillMutationResult], error) {
		return adapter.confirmSkillMutation(ctx, input)
	})
}

func (adapter *Server) registerSkillTransitionTools(tools *toolRegistry) {

	addTool(tools, &mcp.Tool{
		Name:        "skill_transition_preview",
		Title:       "Preview skill lifecycle transition",
		Description: "Preview a skill lifecycle transition (target: active, deprecated, or archived) and return confirmation pins.",
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

	addTool(tools, &mcp.Tool{
		Name:        "skill_transition_confirm",
		Title:       "Confirm skill lifecycle transition",
		Description: "Apply a persisted skill lifecycle transition proposal with required confirmation pins.",
		Annotations: annotations(false, true, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SkillMutationResult], error) {
		return adapter.confirmSkillMutation(ctx, input)
	})
}

func (adapter *Server) registerSkillListTool(tools *toolRegistry) {

	addTool(tools, &mcp.Tool{
		Name:        "skill_list",
		Title:       "List skills",
		Description: "for curation; to pick a skill for a task, call skill_resolve",
		Annotations: annotations(true, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillListInput) (*mcp.CallToolResult, toolOutcome[app.SkillListResult], error) {
		limit := input.Limit
		if limit == 0 {
			limit = 50
		}
		if limit < 1 || limit > paging.MaximumLimit {
			return failure[app.SkillListResult](fmt.Errorf("limit must be between 1 and %d", paging.MaximumLimit))
		}
		raw, err := (app.SkillService{}).ListSkills(ctx, adapter.workspace, strings.TrimSpace(input.State))
		if err != nil {
			return failure[app.SkillListResult](err)
		}
		filter := "state=" + strings.TrimSpace(input.State)
		owner := paging.Owner(filter, raw.Skills)
		lastKey, err := paging.DecodeCursor(input.Cursor, owner, filter)
		if err != nil {
			return failure[app.SkillListResult](fmt.Errorf("snapshot_expired: skill list cursor is invalid or expired"))
		}
		paged, err := paging.Make(raw.Skills, limit, lastKey, owner, filter, func(entry app.SkillListEntry) string {
			return entry.ID
		})
		if err != nil {
			return failure[app.SkillListResult](fmt.Errorf("snapshot_expired: skill list cursor is invalid or expired"))
		}
		result := app.SkillListResult{
			Result:     app.NewResult(app.StatusOK, fmt.Sprintf("%d skill(s).", len(paged.Items))),
			Skills:     paged.Items,
			NextCursor: paged.NextCursor,
			HasMore:    paged.HasMore,
			Total:      paged.Total,
		}
		for _, entry := range paged.Items {
			result.Items = append(result.Items, app.Item{ID: entry.ID, Summary: entry.Name, Impact: "State: " + entry.State + "; collection: " + entry.Collection + "."})
		}
		return success(result)
	})
}

func (adapter *Server) registerSkillGetTool(tools *toolRegistry) {

	addTool(tools, &mcp.Tool{
		Name:        "skill_get",
		Title:       "Get skill",
		Description: "Get an active skill by ID (routing, content, metadata), using the id skill_resolve recommended. local.path is a read-only copy of the skill folder; local.state_directory is writable.",
		Annotations: annotations(true, false, false, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input skillGetInput) (*mcp.CallToolResult, toolOutcome[skillGetResult], error) {
		id := strings.TrimSpace(input.SkillID)
		if id == "" {
			return failure[skillGetResult](fmt.Errorf("skill_id is required"))
		}
		if adapter.profile == ProfileRuntime {
			if refusal := adapter.runtimeSkillRefusal(ctx, id); refusal != nil {
				return &mcp.CallToolResult{IsError: true}, toolOutcome[skillGetResult]{SchemaVersion: SchemaVersion, Error: refusal}, nil
			}
		}
		detail, err := (app.SkillService{}).ReadSkillCore(ctx, adapter.workspace, id)
		if err != nil {
			if errors.Is(err, skill.ErrNotFound) {
				return &mcp.CallToolResult{IsError: true}, toolOutcome[skillGetResult]{SchemaVersion: SchemaVersion, Error: skillNotFoundError(id)}, nil
			}
			return failure[skillGetResult](err)
		}
		local := adapter.localSkill(ctx, detail.SkillID, detail.LifecycleState)
		result := skillGetResult{SkillCore: detail, Content: detail.Content, Local: local}
		if detail.SkillID != systemskills.CuratorSkillID && detail.LifecycleState != "active" {
			// Snapshots exist only for active skills, so a draft or archived
			// third-party skill is gated here from its canonical files.
			trust, trustErr := (app.SkillService{}).ContentTrustFor(ctx, adapter.workspace, detail.SkillID)
			if trustErr != nil {
				return failure[skillGetResult](trustErr)
			}
			if trust.RequiresReview() {
				local = &app.LocalSkill{Status: app.LocalStatusReviewRequired, ReasonCodes: trust.ReasonCodes, ReviewCommand: "skillhub skill review " + detail.SkillID}
				result.Local = local
			}
		}
		if local != nil && local.Status == app.LocalStatusReviewRequired {
			result.Content = ""
		}
		if detail.LifecycleState == "active" && detail.SkillID != systemskills.CuratorSkillID {
			blocked, reasons := reviewBlock(local)
			adapter.recordLoad(ctx, toolSession(req), app.SkillLoad{
				SkillID:      detail.SkillID,
				ResourceKind: "entrypoint",
				Surface:      "skill_get",
				Blocked:      blocked,
				ReasonCodes:  reasons,
			})
		}
		return success(result)
	})
}

func skillNotFoundError(id string) *toolError {
	return &toolError{
		Code:            "not_found",
		Message:         fmt.Sprintf("Skill %s not found", id),
		Retryable:       false,
		SuggestedAction: "Run skill_resolve to find an available skill.",
	}
}

// runtimeSkillRefusal applies the delivery rule of the runtime profile: only a
// skill that skill_resolve may return is served. It asks the distribution
// service, the source resolution uses, so a skill cannot be resolvable yet
// refused here or the reverse. A skill that is not active is reported as not
// found, like an unknown ID, so its existence is not disclosed; an active skill
// that cannot be served is reported as not_servable. Neither answer carries
// skill content. It returns nil when the skill may be delivered.
func (adapter *Server) runtimeSkillRefusal(ctx context.Context, id string) *toolError {
	_, _, err := adapter.distribution.LookupSkills(ctx, adapter.workspace, []string{id})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, catalog.ErrSkillNotServable):
		return &toolError{
			Code:            "not_servable",
			Message:         fmt.Sprintf("Skill %s cannot be served.", id),
			Retryable:       false,
			SuggestedAction: "Choose another skill from skill_resolve; the skill's author must repair it before it can be delivered.",
		}
	case errors.Is(err, skill.ErrNotFound):
		return skillNotFoundError(id)
	default:
		item := safeToolError(err)
		return &item
	}
}
