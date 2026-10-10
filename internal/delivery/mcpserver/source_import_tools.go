package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func (adapter *Server) registerSourceImportTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "source_import_preview",
		Title:       "Preview source skill import",
		Description: "Discover skills in a watched source repository revision and return an import proposal preview.",
		Annotations: annotations(false, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceImportPreviewInput) (*mcp.CallToolResult, toolOutcome[app.SourceImportProposal], error) {
		sourceID := strings.TrimSpace(input.SourceID)
		if sourceID == "" {
			return failure[app.SourceImportProposal](fmt.Errorf("source_id is required"))
		}
		service := app.SourceImportService{}
		return appResult(service.PreviewSourceImport(ctx, adapter.workspace, app.SourceImportPreviewInput{
			SourceID:       sourceID,
			Path:           input.Path,
			Skills:         input.Skills,
			IdempotencyKey: input.IdempotencyKey,
		}))
	})

	addTool(server, &mcp.Tool{
		Name:        "source_import_confirm",
		Title:       "Confirm source skill import",
		Description: "Apply a reviewed source skill import proposal with confirmation pins.",
		Annotations: annotations(false, true, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SourceImportResult], error) {
		service := app.SourceImportService{}
		preview, err := service.LoadSourceImportProposal(ctx, adapter.workspace, input.ProposalID)
		if err != nil {
			return failure[app.SourceImportResult](err)
		}
		return appResult(service.ConfirmSourceImport(ctx, adapter.workspace, preview, app.ConfirmationPins{
			ProposalID:     input.ProposalID,
			ProposalDigest: input.ProposalDigest,
			BaseVersion:    input.BaseVersion,
		}))
	})
}
