package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

// requirePins returns the confirmation pins of a confirm call, trimmed, or an
// invalid-request error when any of the three is blank.
func (input confirmationInput) requirePins() (app.ConfirmationPins, error) {
	pins := app.ConfirmationPins{
		ProposalID:     strings.TrimSpace(input.ProposalID),
		ProposalDigest: strings.TrimSpace(input.ProposalDigest),
		BaseVersion:    strings.TrimSpace(input.BaseVersion),
	}
	if pins.ProposalID == "" || pins.ProposalDigest == "" || pins.BaseVersion == "" {
		return app.ConfirmationPins{}, app.NewInvalidRequestError(
			"proposal_id, proposal_digest, and base_version pins are required",
			"Supply all confirmation pins.",
		)
	}
	return pins, nil
}

// confirmSkillMutation applies a persisted skill proposal, the shared body of
// the skill creation, transition, and update confirm tools.
func (adapter *Server) confirmSkillMutation(ctx context.Context, input confirmationInput) (*mcp.CallToolResult, toolOutcome[app.SkillMutationResult], error) {
	pins, err := input.requirePins()
	if err != nil {
		return failure[app.SkillMutationResult](err)
	}
	service := app.SkillService{}
	preview, err := service.LoadSkillProposal(ctx, adapter.workspace, pins.ProposalID)
	if err != nil {
		return failure[app.SkillMutationResult](err)
	}
	return appResult(service.ConfirmSkillMutation(ctx, adapter.workspace, preview, pins))
}
