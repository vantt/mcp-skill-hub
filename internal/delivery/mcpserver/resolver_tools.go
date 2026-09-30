package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
)

func (adapter *Server) registerResolverTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name: "skill_resolve", Title: "Resolve a skill",
		Description: "Recommend at most one primary skill from bounded task evidence. This does not load or activate skill content; use skills/get and resources/read only after host approval.",
		Annotations: annotations(true, false, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input resolveInput) (*mcp.CallToolResult, toolOutcome[resolveResult], error) {
		request := resolverpkg.Request(input)
		if _, err := resolverpkg.NormalizeRequest(request); err != nil {
			return failure[resolveResult](err)
		}
		response, err := adapter.resolver.Resolve(ctx, adapter.workspace, request)
		if err != nil {
			return failure[resolveResult](err)
		}
		trueValue := true
		return success(resolveResult{
			Resolution: response, ActivationContext: &trueValue, Feedback: &trueValue,
			SchemaMajor: SchemaVersion, ResourceVersionPinning: true,
		})
	})

	addTool(server, &mcp.Tool{
		Name: "skill_feedback", Title: "Record skill feedback",
		Description: "Record a bounded, deduplicated runtime outcome for a prior resolution. event_id is required. Feedback never mutates routing policy.",
		Annotations: annotations(false, false, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input feedbackInput) (*mcp.CallToolResult, toolOutcome[app.FeedbackResult], error) {
		if input.SchemaVersion != SchemaVersion || input.ResolutionID == "" || input.EventID == "" || input.Outcome == "" {
			return failure[app.FeedbackResult](fmt.Errorf("schema_version, resolution_id, event_id, and outcome are required"))
		}
		return appResult(adapter.feedback.Record(ctx, adapter.workspace, app.FeedbackInput{
			SchemaVersion: input.SchemaVersion, ResolutionID: input.ResolutionID, EventID: input.EventID,
			Outcome: input.Outcome, ReasonCode: input.ReasonCode, SelectedSkill: input.SelectedSkill,
			Utility: input.Utility, Basis: input.Basis,
		}))
	})
}
