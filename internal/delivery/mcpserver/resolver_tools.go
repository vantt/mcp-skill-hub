package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
)

func (adapter *Server) registerResolverTools(tools *toolRegistry) {
	addTool(tools, &mcp.Tool{
		Name: "skill_resolve", Title: "Resolve a skill",
		Description: "Recommend at most one primary skill from task evidence (task.description in English). If setup is review_required, do not use the skill and ask the user to run `skillhub skill review <id>`. This does not load skill content: load the recommended skill with skill_get, or, only when your host supports the MCP skills extension, with skills/get and resources/read.",
		Annotations: annotations(true, false, true, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input resolveInput) (*mcp.CallToolResult, toolOutcome[resolveResult], error) {
		request := resolverpkg.Request(input)
		if _, err := resolverpkg.NormalizeRequest(request); err != nil {
			return failure[resolveResult](err)
		}
		session := toolSession(req)
		caller := adapter.callerContext(session)
		ctx = app.WithCallerContext(ctx, caller)

		response, err := adapter.resolver.Resolve(ctx, adapter.workspace, request)
		if err != nil {
			return failure[resolveResult](err)
		}
		if adapter.tracker != nil {
			priorVer := false
			if request.Prior != nil {
				priorVer = caller.VerifyPrior(request.Prior.ResolutionID)
			}
			adapter.tracker.noteResolution(session, request, response, priorVer, caller.Client)
		}
		trueValue := true
		return success(resolveResult{
			Resolution: response, ActivationContext: &trueValue, Feedback: &trueValue,
			SchemaMajor: SchemaVersion, ResourceVersionPinning: true,
		})
	}, withCommittedResolverSchemas())

	addTool(tools, &mcp.Tool{
		Name: "skill_feedback", Title: "Record skill feedback",
		Description: "Record a bounded, deduplicated runtime outcome for a prior resolution. event_id is required. Feedback never mutates routing policy.",
		Annotations: annotations(false, false, true, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input feedbackInput) (*mcp.CallToolResult, toolOutcome[app.FeedbackResult], error) {
		if input.SchemaVersion != SchemaVersion || input.ResolutionID == "" || input.EventID == "" || input.Outcome == "" {
			return failure[app.FeedbackResult](fmt.Errorf("schema_version, resolution_id, event_id, and outcome are required"))
		}
		session := toolSession(req)
		caller := adapter.callerContext(session)
		ctx = app.WithCallerContext(ctx, caller)

		return appResult(adapter.feedback.Record(ctx, adapter.workspace, app.FeedbackInput{
			SchemaVersion: input.SchemaVersion, ResolutionID: input.ResolutionID, EventID: input.EventID,
			Outcome: input.Outcome, ReasonCode: input.ReasonCode, SelectedSkill: input.SelectedSkill,
			Utility: input.Utility, Basis: input.Basis,
		}))
	}, withPropertyEnums(map[string][]any{
		"outcome":     {"activated", "loaded", "used", "abandoned", "rejected", "completed", "failed"},
		"reason_code": {"user_rejected", "scope_mismatch", "capability_unavailable", "constraint_conflict", "workflow_completed", "workflow_failed", "abandoned", "host_report", "setup_failed"},
		"utility":     {"helpful", "harmful", "neutral"},
		"basis":       {"user", "evaluator", "controlled-benchmark"},
	}))
}
