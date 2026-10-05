package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func (adapter *Server) registerSourceTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name: "source_intake_add", Title: "Add source candidate",
		Description: "Capture a bounded source locator and reason in the local intake queue. This does not fetch the source or change curated skills.",
		Annotations: annotations(false, false, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceIntakeAddInput) (*mcp.CallToolResult, toolOutcome[app.SourceCandidateResult], error) {
		if strings.TrimSpace(input.IdempotencyKey) == "" {
			return failure[app.SourceCandidateResult](fmt.Errorf("idempotency_key is required"))
		}
		return appResult(adapter.source.CaptureSourceCandidate(ctx, adapter.workspace, app.SourceCandidateInput{Locator: input.Locator, Reason: input.Reason, IdempotencyKey: input.IdempotencyKey}))
	})

	addTool(server, &mcp.Tool{
		Name: "source_intake_list", Title: "List source intake",
		Description: "List source candidates and monitored sources as a snapshot-bound page. Pass next_cursor unchanged to continue; limit defaults to 25 and is at most 100.",
		Annotations: annotations(true, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceIntakeListInput) (*mcp.CallToolResult, toolOutcome[page[sourceListItem]], error) {
		limit, err := normalizeLimit(input.Limit)
		if err != nil {
			return failure[page[sourceListItem]](err)
		}
		result, err := adapter.source.ListSources(ctx, adapter.workspace, strings.TrimSpace(input.Status))
		if err != nil {
			return failure[page[sourceListItem]](err)
		}
		filter := "status=" + strings.TrimSpace(input.Status)
		items := make([]sourceListItem, 0, len(result.Candidates)+len(result.Sources))
		for index := range result.Candidates {
			candidate := result.Candidates[index]
			items = append(items, sourceListItem{Kind: "candidate", Candidate: &candidate})
		}
		for index := range result.Sources {
			source := result.Sources[index].Record
			items = append(items, sourceListItem{Kind: "source", Source: &source})
		}
		owner := pageOwner(filter, items)
		lastKey, err := decodeCursor(input.Cursor, owner, filter)
		if err != nil {
			return failure[page[sourceListItem]](fmt.Errorf("snapshot_expired: source list cursor is invalid or expired"))
		}
		paged, err := makePage(items, limit, lastKey, owner, filter, func(item sourceListItem) string {
			if item.Candidate != nil {
				return "candidate\x00" + item.Candidate.ID
			}
			return "source\x00" + item.Source.ID
		})
		if err != nil {
			return failure[page[sourceListItem]](fmt.Errorf("snapshot_expired: source list cursor is invalid or expired"))
		}
		return success(paged)
	})

	addTool(server, &mcp.Tool{
		Name: "source_triage", Title: "Triage source candidate",
		Description: "Triage a source candidate. Outcomes: accept with skill_id (link existing skill) or new_skill_id (scaffold draft skill), import (vendor skills), defer (keep in queue), or reject (with reason). Acceptance or import returns a preview proposal.",
		Annotations: annotations(false, false, false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceTriageInput) (*mcp.CallToolResult, toolOutcome[sourceTriageResult], error) {
		service := adapter.source
		if input.Confirmation != nil {
			preview, err := service.LoadSourceProposal(ctx, adapter.workspace, input.Confirmation.ProposalID)
			if err != nil {
				return failure[sourceTriageResult](err)
			}
			mutation, err := service.ConfirmSourceProposal(ctx, adapter.workspace, preview, *input.Confirmation)
			if err != nil {
				return failure[sourceTriageResult](err)
			}
			return appResult(sourceTriageResult{Mutation: &mutation}, nil)
		}
		if strings.TrimSpace(input.CandidateID) == "" || strings.TrimSpace(input.Decision) == "" {
			return failure[sourceTriageResult](fmt.Errorf("candidate_id and decision are required when confirmation is omitted"))
		}
		monitor := true
		if input.MonitoringEnabled != nil {
			monitor = *input.MonitoringEnabled
		}
		newSkillID := strings.TrimSpace(input.NewSkillID)
		if newSkillID == "" {
			newSkillID = strings.TrimSpace(input.NewSkill)
		}
		preview, mutation, err := service.TriageSourceCandidate(ctx, adapter.workspace, app.SourceTriageInput{
			CandidateID: input.CandidateID, Decision: input.Decision, DecisionReason: input.DecisionReason,
			SourceID: input.SourceID, Adapter: input.Adapter, Ref: input.Ref, SourcePath: input.SourcePath,
			License: input.License, Trust: input.Trust, Cadence: input.Cadence, SkillID: input.SkillID,
			NewSkillID:        newSkillID,
			MonitoringEnabled: monitor, IdempotencyKey: input.IdempotencyKey,
		})
		if err != nil {
			return failure[sourceTriageResult](err)
		}
		if input.Decision == "accept" || input.Decision == "import" {
			return success(sourceTriageResult{Preview: &preview})
		}
		return appResult(sourceTriageResult{Mutation: &mutation}, nil)
	})

	addTool(server, &mcp.Tool{
		Name: "source_check", Title: "Check sources",
		Description: "Check selected or due source revisions through configured adapters. This records revision changes and per-skill upstream results in results[].skills without editing curated skills.",
		Annotations: annotations(false, false, false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceCheckInput) (*mcp.CallToolResult, toolOutcome[app.SourceCheckResult], error) {
		if len(input.SourceIDs) == 0 && !input.AllDue {
			return failure[app.SourceCheckResult](fmt.Errorf("source_ids or all_due=true is required"))
		}
		return appResult(adapter.source.CheckSources(ctx, adapter.workspace, input.SourceIDs, input.AllDue))
	})
}
