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
		Description: "Preview watching a public GitHub repository for skill updates. Local filesystem folders are rejected. Returns an immutable watch proposal with confirmation pins. No canonical source records change during preview.",
		Annotations: annotations(false, false, false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input sourceWatchPreviewInput) (*mcp.CallToolResult, toolOutcome[app.SourceProposal], error) {
		locator := strings.TrimSpace(input.Locator)
		if locator == "" {
			return failure[app.SourceProposal](fmt.Errorf("locator is required"))
		}
		service := app.SourceService{}
		return appResult(service.PreviewSourceWatch(ctx, adapter.workspace, app.SourceWatchInput{
			Locator:           locator,
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
		Description: "Apply an approved source watch proposal. All proposal_id, proposal_digest, and base_version pins are required.",
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
		if preview.Confirmation.ApplicationCommand != "source_watch" {
			return failure[app.SourceMutationResult](app.NewInvalidRequestError(
				fmt.Sprintf("proposal %s is not a source_watch proposal", proposalID),
				"Supply a valid source_watch proposal ID.",
			))
		}
		return appResult(service.ConfirmSourceWatch(ctx, adapter.workspace, preview, app.ConfirmationPins{
			ProposalID:     proposalID,
			ProposalDigest: proposalDigest,
			BaseVersion:    baseVersion,
		}))
	})
}
