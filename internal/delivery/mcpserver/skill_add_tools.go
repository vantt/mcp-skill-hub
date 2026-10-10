package mcpserver

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func validateSkillAddLocator(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("locator is required")
	}
	if strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") ||
		strings.HasPrefix(trimmed, "~/") || filepath.IsAbs(trimmed) ||
		strings.HasPrefix(trimmed, "file://") || !strings.Contains(trimmed, "://") {
		return app.NewInvalidRequestError(
			"local filesystem locators are rejected: MCP has no host-granted file-selection capability; provide a public GitHub URL (e.g. https://github.com/owner/repo)",
			"Provide a public GitHub repository URL.",
		)
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return app.NewInvalidRequestError(
			"only public HTTP/HTTPS GitHub locators are accepted over MCP",
			"Provide a public GitHub repository URL (https://github.com/owner/repo).",
		)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host != "github.com" {
		return app.NewInvalidRequestError(
			"only public GitHub repositories are supported over MCP; got host "+host,
			"Provide a public GitHub repository URL (https://github.com/owner/repo).",
		)
	}
	return nil
}

func (adapter *Server) registerSkillAddTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "skill_add_preview",
		Title:       "Preview skill addition",
		Description: "Preview adding a skill from a public GitHub repository. Returns an add proposal with confirmation pins.",
		Annotations: annotations(false, false, false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillAddPreviewInput) (*mcp.CallToolResult, toolOutcome[app.SkillAddProposal], error) {
		if err := validateSkillAddLocator(input.Locator); err != nil {
			return failure[app.SkillAddProposal](err)
		}
		service := app.SkillAddService{}
		return appResult(service.PreviewSkillAdd(ctx, adapter.workspace, app.SkillAddInput{
			Locator:        strings.TrimSpace(input.Locator),
			Selection:      strings.TrimSpace(input.Selection),
			All:            input.All,
			TargetID:       strings.TrimSpace(input.TargetID),
			Collection:     strings.TrimSpace(input.Collection),
			IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
			FullDiff:       input.FullDiff,
		}))
	})

	addTool(server, &mcp.Tool{
		Name:        "skill_add_confirm",
		Title:       "Confirm skill addition",
		Description: "Apply an approved draft skill addition proposal with confirmation pins.",
		Annotations: annotations(false, true, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SkillAddResult], error) {
		proposalID := strings.TrimSpace(input.ProposalID)
		proposalDigest := strings.TrimSpace(input.ProposalDigest)
		baseVersion := strings.TrimSpace(input.BaseVersion)
		if proposalID == "" || proposalDigest == "" || baseVersion == "" {
			return failure[app.SkillAddResult](app.NewInvalidRequestError(
				"proposal_id, proposal_digest, and base_version pins are required",
				"Supply all confirmation pins.",
			))
		}
		service := app.SkillAddService{}
		preview, err := service.LoadSkillAddProposal(ctx, adapter.workspace, proposalID)
		if err != nil {
			return failure[app.SkillAddResult](err)
		}
		return appResult(service.ConfirmSkillAdd(ctx, adapter.workspace, preview, app.ConfirmationPins{
			ProposalID:     proposalID,
			ProposalDigest: proposalDigest,
			BaseVersion:    baseVersion,
		}))
	})
}
