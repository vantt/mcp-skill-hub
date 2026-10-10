package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
)

type sourceListInput struct {
	Status string `json:"status,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (adapter *Server) registerSourceTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "source_list",
		Title:       "List sources",
		Description: "List monitored sources in the catalog as a snapshot-bound page. Pass next_cursor unchanged to continue.",
		Annotations: annotations(true, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceListInput) (*mcp.CallToolResult, toolOutcome[paging.Page[app.SourceListItem]], error) {
		limit, err := paging.NormalizeLimit(input.Limit)
		if err != nil {
			return failure[paging.Page[app.SourceListItem]](err)
		}
		result, err := adapter.source.ListSources(ctx, adapter.workspace, strings.TrimSpace(input.Status))
		if err != nil {
			return failure[paging.Page[app.SourceListItem]](err)
		}
		filter := "status=" + strings.TrimSpace(input.Status)
		items := result.Sources
		owner := paging.Owner(filter, items)
		lastKey, err := paging.DecodeCursor(input.Cursor, owner, filter)
		if err != nil {
			return failure[paging.Page[app.SourceListItem]](fmt.Errorf("snapshot_expired: source list cursor is invalid or expired"))
		}
		paged, err := paging.Make(items, limit, lastKey, owner, filter, func(item app.SourceListItem) string {
			return item.Record.ID
		})
		if err != nil {
			return failure[paging.Page[app.SourceListItem]](fmt.Errorf("snapshot_expired: source list cursor is invalid or expired"))
		}
		return success(paged)
	})

	addTool(server, &mcp.Tool{
		Name:        "source_check",
		Title:       "Check sources",
		Description: "Check selected or due source revisions through configured adapters without editing curated skills.",
		Annotations: annotations(false, false, false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceCheckInput) (*mcp.CallToolResult, toolOutcome[app.SourceCheckResult], error) {
		if len(input.SourceIDs) == 0 && !input.AllDue {
			return failure[app.SourceCheckResult](fmt.Errorf("source_ids or all_due=true is required"))
		}
		return appResult(adapter.source.CheckSources(ctx, adapter.workspace, input.SourceIDs, input.AllDue))
	})
}
