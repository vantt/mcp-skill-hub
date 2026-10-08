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
		Description: "Recommend at most one primary skill from bounded task evidence. Send task.description in English; translate a non-English request first. primary.setup (and supporting[].setup), when present, is a hint: review_required means a human has not approved the skill's content yet, so do not use the skill and tell the user to run `skillhub skill review <id>`; unsupported_platform means this operating system is not supported; any other state is a hint from the user's terminal (basis), so confirm with the skill's own check in your shell via local.preflight. This does not load or activate skill content; use skills/get and resources/read only after host approval.",
		Annotations: annotations(true, false, true, false),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input resolveInput) (*mcp.CallToolResult, toolOutcome[resolveResult], error) {
		request := resolverpkg.Request(input)
		if _, err := resolverpkg.NormalizeRequest(request); err != nil {
			return failure[resolveResult](err)
		}
		var session *mcp.ServerSession
		if req != nil {
			session = req.Session
		}
		caller := adapter.callerContext(session)
		ctx = app.WithCallerContext(ctx, caller)

		response, err := adapter.resolver.Resolve(ctx, adapter.workspace, request)
		if err != nil {
			return failure[resolveResult](err)
		}
		if adapter.tracker != nil {
			adapter.tracker.noteResolution(session, response, caller.Client)
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
	}, func(ctx context.Context, req *mcp.CallToolRequest, input feedbackInput) (*mcp.CallToolResult, toolOutcome[app.FeedbackResult], error) {
		if input.SchemaVersion != SchemaVersion || input.ResolutionID == "" || input.EventID == "" || input.Outcome == "" {
			return failure[app.FeedbackResult](fmt.Errorf("schema_version, resolution_id, event_id, and outcome are required"))
		}
		var session *mcp.ServerSession
		if req != nil {
			session = req.Session
		}
		caller := adapter.callerContext(session)
		ctx = app.WithCallerContext(ctx, caller)

		return appResult(adapter.feedback.Record(ctx, adapter.workspace, app.FeedbackInput{
			SchemaVersion: input.SchemaVersion, ResolutionID: input.ResolutionID, EventID: input.EventID,
			Outcome: input.Outcome, ReasonCode: input.ReasonCode, SelectedSkill: input.SelectedSkill,
			Utility: input.Utility, Basis: input.Basis,
		}))
	})
}
