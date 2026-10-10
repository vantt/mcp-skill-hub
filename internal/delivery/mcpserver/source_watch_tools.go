package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func (adapter *Server) registerSourceWatchTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "source_watch_preview",
		Title:       "Preview source watch",
		Description: "Preview watching a GitHub repository and attaching it as a learning reference. Returns confirmation pins.",
		Annotations: annotations(false, false, false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceWatchPreviewInput) (*mcp.CallToolResult, toolOutcome[app.SourceProposal], error) {
		locator := strings.TrimSpace(input.Locator)
		if locator == "" {
			return failure[app.SourceProposal](fmt.Errorf("locator is required"))
		}
		service := app.SourceService{}
		return appResult(service.PreviewSourceWatch(ctx, adapter.workspace, app.SourceWatchInput{
			Locator:           locator,
			SkillID:           strings.TrimSpace(input.SkillID),
			SourceID:          strings.TrimSpace(input.SourceID),
			Ref:               strings.TrimSpace(input.Ref),
			Path:              strings.TrimSpace(input.Path),
			Cadence:           strings.TrimSpace(input.Cadence),
			MonitoringEnabled: input.MonitoringEnabled,
			Trust:             strings.TrimSpace(input.Trust),
			License:           strings.TrimSpace(input.License),
			IdempotencyKey:    strings.TrimSpace(input.IdempotencyKey),
		}))
	})

	addTool(server, &mcp.Tool{
		Name:        "source_watch_confirm",
		Title:       "Confirm source watch",
		Description: "Apply an approved source watch, attach, detach, or unwatch proposal with confirmation pins.",
		Annotations: annotations(false, true, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SourceMutationResult], error) {
		proposalID := strings.TrimSpace(input.ProposalID)
		proposalDigest := strings.TrimSpace(input.ProposalDigest)
		baseVersion := strings.TrimSpace(input.BaseVersion)
		if proposalID == "" || proposalDigest == "" || baseVersion == "" {
			return failure[app.SourceMutationResult](app.NewInvalidRequestError(
				"proposal_id, proposal_digest, and base_version pins are required",
				"Supply all confirmation pins.",
			))
		}
		service := app.SourceService{}
		preview, err := service.LoadSourceProposal(ctx, adapter.workspace, proposalID)
		if err != nil {
			return failure[app.SourceMutationResult](err)
		}
		cmd := preview.WriteCommand()
		switch cmd {
		case "source_watch", "source_attach", "source_detach", "source_unwatch":
		default:
			return failure[app.SourceMutationResult](app.NewInvalidRequestError(
				fmt.Sprintf("proposal %s has command %q, expected source_watch, source_attach, source_detach, or source_unwatch", proposalID, cmd),
				"Supply a valid source proposal ID.",
			))
		}
		return appResult(service.ConfirmSourceProposal(ctx, adapter.workspace, preview, app.ConfirmationPins{
			ProposalID:     proposalID,
			ProposalDigest: proposalDigest,
			BaseVersion:    baseVersion,
		}))
	})
}
