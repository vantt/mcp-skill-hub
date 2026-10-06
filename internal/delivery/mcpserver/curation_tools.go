package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
	distillpkg "github.com/vantt/mcp-skill-hub/internal/distill"
)

func (adapter *Server) registerCurationRunTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name: "curation_run_start", Title: "Start curation runs",
		Description: "Prepare immutable revision packages for selected changed sources and transition each successfully prepared run to in_progress. Active skills remain unchanged.",
		Annotations: annotations(false, false, true, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input curationRunStartInput) (*mcp.CallToolResult, toolOutcome[runStartResult], error) {
		if len(input.SourceIDs) == 0 && !input.AllChanged {
			return failure[runStartResult](fmt.Errorf("source_ids or all_changed=true is required"))
		}
		if strings.TrimSpace(input.IdempotencyKey) == "" {
			return failure[runStartResult](fmt.Errorf("idempotency_key is required"))
		}
		service := adapter.distill
		prepared, err := service.PrepareDistillRuns(ctx, adapter.workspace, app.DistillPrepareInput{SourceIDs: input.SourceIDs, AllChanged: input.AllChanged, IdempotencyKey: input.IdempotencyKey})
		if err != nil {
			return failure[runStartResult](err)
		}
		result := runStartResult{Prepared: prepared.Prepared, Failed: prepared.Failed, Items: []runStartItem{}}
		for index := range prepared.Results {
			preparedItem := prepared.Results[index]
			outcome := runStartItem{SourceID: preparedItem.SourceID, Prepared: &preparedItem}
			if preparedItem.Run == nil {
				itemError := toolError{Code: "source_unavailable", Message: preparedItem.Error, Retryable: true, SuggestedAction: "Correct or retry this source independently."}
				outcome.Error = &itemError
				result.Items = append(result.Items, outcome)
				continue
			}
			started, startErr := service.StartDistillRun(ctx, adapter.workspace, preparedItem.Run.ID)
			if startErr != nil {
				itemError := safeToolError(startErr)
				outcome.Error = &itemError
				result.Failed++
				result.Items = append(result.Items, outcome)
				continue
			}
			outcome.Started = &started
			result.Started++
			result.Items = append(result.Items, outcome)
		}
		return success(result)
	})

	addTool(server, &mcp.Tool{Name: "curation_run_submit", Title: "Submit curation run", Description: "Validate a structured finding/coverage submission against the run's immutable package and atomically finalize it when unblocked.", Annotations: annotations(false, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input curationRunSubmitInput) (*mcp.CallToolResult, toolOutcome[app.DistillRunResult], error) {
			return appResult(adapter.distill.SubmitDistillRun(ctx, adapter.workspace, input.RunID, input.Submission))
		})
	addTool(server, &mcp.Tool{Name: "curation_run_get", Title: "Get curation run", Description: "Read one curation run and its pinned state without loading source content.", Annotations: annotations(true, false, true, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input runIDInput) (*mcp.CallToolResult, toolOutcome[app.DistillRunResult], error) {
			return appResult(adapter.distill.GetDistillRun(ctx, adapter.workspace, input.RunID))
		})
	addTool(server, &mcp.Tool{Name: "curation_run_retry", Title: "Retry curation run", Description: "Retry a failed or awaiting-decision run; awaiting-decision runs require an explicit decision string.", Annotations: annotations(false, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input curationRunRetryInput) (*mcp.CallToolResult, toolOutcome[app.DistillRunResult], error) {
			decisions := []app.DistillRetryInput{}
			if strings.TrimSpace(input.Decision) != "" {
				decisions = append(decisions, app.DistillRetryInput{Decision: input.Decision})
			}
			return appResult(adapter.distill.RetryDistillRun(ctx, adapter.workspace, input.RunID, decisions...))
		})
	addTool(server, &mcp.Tool{Name: "curation_run_cancel", Title: "Cancel curation run", Description: "Cancel a non-finalized run without advancing its source cursor.", Annotations: annotations(false, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input runIDInput) (*mcp.CallToolResult, toolOutcome[app.DistillRunResult], error) {
			return appResult(adapter.distill.CancelDistillRun(ctx, adapter.workspace, input.RunID))
		})

	addTool(server, &mcp.Tool{Name: "observation_list", Title: "List observations", Description: "Return a snapshot-bound page of distilled observations, optionally filtered by source_id. Limit defaults to 25 and is at most 100.", Annotations: annotations(true, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input observationListInput) (*mcp.CallToolResult, toolOutcome[paging.Page[distillpkg.Observation]], error) {
			limit, err := paging.NormalizeLimit(input.Limit)
			if err != nil {
				return failure[paging.Page[distillpkg.Observation]](err)
			}
			query, err := adapter.distill.QueryDistill(ctx, adapter.workspace, "findings", input.SourceID, "")
			if err != nil {
				return failure[paging.Page[distillpkg.Observation]](err)
			}
			filter := "source_id=" + strings.TrimSpace(input.SourceID)
			owner := paging.Owner(filter, query.Findings)
			lastKey, err := paging.DecodeCursor(input.Cursor, owner, filter)
			if err != nil {
				return failure[paging.Page[distillpkg.Observation]](fmt.Errorf("snapshot_expired: observation cursor is invalid or expired"))
			}
			paged, err := paging.Make(query.Findings, limit, lastKey, owner, filter, func(item distillpkg.Observation) string { return item.ID })
			if err != nil {
				return failure[paging.Page[distillpkg.Observation]](fmt.Errorf("snapshot_expired: observation cursor is invalid or expired"))
			}
			return success(paged)
		})

	addTool(server, &mcp.Tool{Name: "comparison_get", Title: "Get comparison", Description: "Read one distilled comparison by stable comparison_id.", Annotations: annotations(true, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input comparisonGetInput) (*mcp.CallToolResult, toolOutcome[distillpkg.Comparison], error) {
			query, err := adapter.distill.QueryDistill(ctx, adapter.workspace, "comparisons", "", "")
			if err != nil {
				return failure[distillpkg.Comparison](err)
			}
			for _, item := range query.Comparisons {
				if item.ID == input.ComparisonID {
					return success(item)
				}
			}
			return failure[distillpkg.Comparison](fmt.Errorf("comparison not found"))
		})
}
