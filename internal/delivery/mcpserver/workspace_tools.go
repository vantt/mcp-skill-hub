package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
)

func (adapter *Server) registerWorkspaceTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name: "curation_session_record", Title: "Record completed curation session",
		Description: "Record explicitly observed, content-free UX measurements after a curation session has ended. This telemetry operation never mutates canonical files.",
		Annotations: annotations(false, false, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input curationSessionRecordInput) (*mcp.CallToolResult, toolOutcome[app.CurationSessionResult], error) {
		if input.SchemaVersion != SchemaVersion || input.EventID == "" || input.Status == "" || input.Basis == "" {
			return failure[app.CurationSessionResult](fmt.Errorf("schema_version, event_id, status, and basis are required"))
		}
		return appResult(adapter.curationUX.RecordSession(ctx, adapter.workspace, app.CurationSessionInput{
			SchemaVersion: input.SchemaVersion, EventID: input.EventID, Status: input.Status, Basis: input.Basis,
			TurnsToNextAction: input.TurnsToNextAction, UnnecessaryConfirmations: input.UnnecessaryConfirmations,
			PromptsPerBatch: input.PromptsPerBatch, BatchSize: input.BatchSize,
			AutoFinalized: input.AutoFinalized, RecoveryCompleted: input.RecoveryCompleted,
			RoutineGitNoise: input.RoutineGitNoise, DurationMS: input.DurationMS, ErrorCode: input.ErrorCode,
		}))
	})
	addTool(server, &mcp.Tool{Name: "hub_status", Title: "Skill Hub status", Description: "Return local curation home, workspace health, action counts, and ranked next actions.", Annotations: annotations(true, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, toolOutcome[app.CurationHome], error) {
			return appResult((app.CurationService{}).GetCurationHome(ctx, adapter.workspace))
		})
	addTool(server, &mcp.Tool{Name: "workspace_validate", Title: "Validate workspace", Description: "Validate canonical workspace files without rebuilding or changing them.", Annotations: annotations(true, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, toolOutcome[app.Result], error) {
			return appResult((app.WorkspaceService{}).ValidateWorkspace(ctx, adapter.workspace))
		})
	addTool(server, &mcp.Tool{Name: "workspace_rebuild", Title: "Rebuild catalog", Description: "Rebuild and atomically publish disposable derived catalog state from canonical files. Canonical files are never changed.", Annotations: annotations(false, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, toolOutcome[app.Result], error) {
			return appResult((app.CatalogService{}).BuildCatalogGeneration(ctx, adapter.workspace))
		})
	addTool(server, &mcp.Tool{Name: "workspace_diff", Title: "Read workspace diff", Description: "Return canonical Git diff paths or changes for an operation_id.", Annotations: annotations(true, false, false, false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, input workspaceDiffInput) (*mcp.CallToolResult, toolOutcome[workspaceDiffResult], error) {
			limit, err := paging.NormalizeLimit(input.Limit)
			if err != nil {
				return failure[workspaceDiffResult](err)
			}
			filter := "operation_id=" + strings.TrimSpace(input.OperationID)
			if input.OperationID != "" {
				value, callErr := (app.WorkspaceService{}).GetOperationDiff(ctx, adapter.workspace, input.OperationID)
				if callErr != nil {
					return failure[workspaceDiffResult](callErr)
				}
				owner := paging.Owner(filter, value.Changes)
				lastKey, cursorErr := paging.DecodeCursor(input.Cursor, owner, filter)
				if cursorErr != nil {
					return failure[workspaceDiffResult](fmt.Errorf("snapshot_expired: diff cursor is invalid or expired"))
				}
				paged, pageErr := paging.Make(value.Changes, limit, lastKey, owner, filter, func(item app.OperationChange) string { return item.Path })
				if pageErr != nil {
					return failure[workspaceDiffResult](fmt.Errorf("snapshot_expired: diff cursor is invalid or expired"))
				}
				return success(workspaceDiffResult{Kind: "operation", Operation: &paged})
			}
			value, callErr := (app.WorkspaceService{}).GetCurationDiff(ctx, adapter.workspace)
			if callErr != nil {
				return failure[workspaceDiffResult](callErr)
			}
			files := []app.DiffFile{}
			for _, group := range value.Groups {
				files = append(files, group.Files...)
			}
			owner := paging.Owner(filter, struct {
				Configured bool           `json:"configured"`
				Files      []app.DiffFile `json:"files"`
			}{Configured: value.GitConfigured, Files: files})
			lastKey, cursorErr := paging.DecodeCursor(input.Cursor, owner, filter)
			if cursorErr != nil {
				return failure[workspaceDiffResult](fmt.Errorf("snapshot_expired: diff cursor is invalid or expired"))
			}
			paged, pageErr := paging.Make(files, limit, lastKey, owner, filter, func(item app.DiffFile) string { return item.Path + "\x00" + item.Status })
			if pageErr != nil {
				return failure[workspaceDiffResult](fmt.Errorf("snapshot_expired: diff cursor is invalid or expired"))
			}
			return success(workspaceDiffResult{Kind: "workspace", GitDirty: value.Dirty, Files: &paged})
		})
}
