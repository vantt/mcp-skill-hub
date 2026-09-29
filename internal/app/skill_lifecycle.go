package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

// SkillService is the shared application boundary for CLI and future MCP tools.
type SkillService struct {
	Manager skill.Manager
}

// ConfirmationPins are the complete optimistic-concurrency approval contract.
type ConfirmationPins struct {
	ProposalID     string `json:"proposal_id"`
	ProposalDigest string `json:"proposal_digest"`
	BaseVersion    string `json:"base_version"`
}

// ConfirmationRequirement is the explicit approval rule and immutable pins.
type ConfirmationRequirement struct {
	Required bool             `json:"required"`
	Mode     string           `json:"mode"`
	Pins     ConfirmationPins `json:"pins"`
}

// ConfirmationPolicy describes why a semantic preview cannot apply implicitly.
type ConfirmationPolicy struct {
	PolicyRevision     string                  `json:"policy_revision"`
	ActionClass        string                  `json:"action_class"`
	ApplicationCommand string                  `json:"application_command"`
	Confirmation       ConfirmationRequirement `json:"confirmation"`
}

// SkillProposal is a transport-neutral preview with optional full diff.
type SkillProposal struct {
	Result
	SkillID       string               `json:"skill_id"`
	Command       string               `json:"command"`
	Diff          skill.DiffSummary    `json:"diff"`
	FullDiff      string               `json:"full_diff,omitempty"`
	Confirmation  ConfirmationPolicy   `json:"confirmation"`
	RoutingImpact *skill.RoutingImpact `json:"routing_impact,omitempty"`
	proposal      skill.Proposal
}

// SkillMutationResult reports active local state and the Git boundary.
type SkillMutationResult struct {
	Result
	SkillID         string              `json:"skill_id"`
	OperationID     string              `json:"operation_id"`
	ChangedPaths    []string            `json:"changed_paths"`
	CatalogSnapshot string              `json:"catalog_snapshot"`
	Generation      string              `json:"generation"`
	ActiveLocally   bool                `json:"active_locally"`
	GitDirty        bool                `json:"git_dirty"`
	Confirmation    *ConfirmationPolicy `json:"confirmation,omitempty"`
	Proposal        *SkillProposal      `json:"proposal,omitempty"`
}

// SkillReadResult is the basic progressive-disclosure read surface.
type SkillReadResult struct {
	Result
	Manifest skill.Manifest `json:"manifest"`
	Content  string         `json:"content"`
}

func (service SkillService) PreviewCreate(ctx context.Context, path string, input skill.CreateInput, fullDiff bool) (SkillProposal, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillProposal{}, err
	}
	proposal, err := service.Manager.PreviewCreate(ctx, root, input, fullDiff)
	if err != nil {
		return SkillProposal{}, err
	}
	if err := skill.StoreProposal(root, proposal, time.Now()); err != nil {
		return SkillProposal{}, err
	}
	return makeSkillProposal(proposal, "Draft skill creation is ready for review."), nil
}

// PreviewSkillUpdate plans a direct edit against a pinned catalog base.
func (service SkillService) PreviewSkillUpdate(ctx context.Context, path, id string, input skill.UpdateInput, fullDiff bool) (SkillProposal, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillProposal{}, err
	}
	proposal, err := service.Manager.PreviewUpdate(ctx, root, id, input, fullDiff)
	if err != nil {
		return SkillProposal{}, err
	}
	if err := skill.StoreProposal(root, proposal, time.Now()); err != nil {
		return SkillProposal{}, err
	}
	return makeSkillProposal(proposal, "Skill edit is ready for review."), nil
}

func (service SkillService) PreviewActivate(ctx context.Context, path, id string, fullDiff bool) (SkillProposal, error) {
	return service.previewTransition(ctx, path, id, "active", fullDiff, "")
}

func (service SkillService) PreviewDeprecate(ctx context.Context, path, id string, fullDiff bool) (SkillProposal, error) {
	return service.previewTransition(ctx, path, id, "deprecated", fullDiff, "")
}

func (service SkillService) PreviewArchive(ctx context.Context, path, id string, fullDiff bool) (SkillProposal, error) {
	return service.previewTransition(ctx, path, id, "archived", fullDiff, "")
}

// PreviewTransitionWithKey exposes a stable caller key without widening the
// lifecycle manager's public transition vocabulary.
func (service SkillService) PreviewTransitionWithKey(ctx context.Context, path, id, target string, fullDiff bool, idempotencyKey string) (SkillProposal, error) {
	return service.previewTransition(ctx, path, id, target, fullDiff, idempotencyKey)
}

func (service SkillService) previewTransition(ctx context.Context, path, id, target string, fullDiff bool, idempotencyKey string) (SkillProposal, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillProposal{}, err
	}
	proposal, err := service.Manager.PreviewTransition(ctx, root, id, target, fullDiff, idempotencyKey)
	if err != nil {
		return SkillProposal{}, err
	}
	if err := skill.StoreProposal(root, proposal, time.Now()); err != nil {
		return SkillProposal{}, err
	}
	return makeSkillProposal(proposal, "Skill "+target+" transition is ready for review."), nil
}

// LoadSkillProposal retrieves a previously displayed immutable proposal. It
// does not regenerate input or reopen an external editor.
func (service SkillService) LoadSkillProposal(ctx context.Context, path, proposalID string) (SkillProposal, error) {
	if err := ctx.Err(); err != nil {
		return SkillProposal{}, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillProposal{}, err
	}
	proposal, err := skill.LoadProposal(root, proposalID, time.Now())
	if err != nil {
		return SkillProposal{}, err
	}
	return makeSkillProposal(proposal, "Stored skill proposal is ready for confirmation."), nil
}

// ConfirmSkillMutation applies exactly the preview and pins supplied by the caller.
func (service SkillService) ConfirmSkillMutation(ctx context.Context, path string, preview SkillProposal, pins ConfirmationPins) (SkillMutationResult, error) {
	if pins != preview.Confirmation.Confirmation.Pins {
		return staleSkillProposal(preview), nil
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillMutationResult{}, err
	}
	result, err := service.Manager.Confirm(ctx, root, preview.proposal)
	if errors.Is(err, mutation.ErrConflict) || errors.Is(err, skill.ErrSnapshotExpired) {
		return staleSkillProposal(preview), nil
	}
	if err != nil {
		return SkillMutationResult{}, err
	}
	response := SkillMutationResult{
		Result:  NewResult(StatusApplied, "Skill mutation was published and is active locally."),
		SkillID: preview.SkillID, OperationID: result.OperationID, ChangedPaths: result.ChangedPaths,
		CatalogSnapshot: result.CatalogSnapshot, Generation: result.Generation,
		ActiveLocally: true, GitDirty: result.GitDirty,
	}
	response.Items = append(response.Items,
		Item{ID: "operation", Summary: result.OperationID, Impact: "Immutable managed-mutation receipt."},
		Item{ID: "catalog_snapshot", Summary: result.CatalogSnapshot, Impact: "Active local catalog version."},
		Item{ID: "generation", Summary: result.Generation, Impact: "Published immutable catalog generation."},
		Item{ID: "git_state", Summary: gitMutationSummary(result.GitDirty), Impact: "Git commit remains user-controlled."},
	)
	return response, nil
}

// ReadSkillContentForEdit returns canonical bytes for a caller-owned temporary
// editor file; it never lets the editor write the workspace directly.
func (SkillService) ReadSkillContentForEdit(ctx context.Context, path, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return nil, err
	}
	return skill.ReadEditableContent(root, id)
}

func (SkillService) ReadSkill(ctx context.Context, path, id string) (SkillReadResult, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillReadResult{}, err
	}
	manifest, err := skill.GetManifest(ctx, root, id)
	if err != nil {
		return SkillReadResult{}, err
	}
	var entry *skill.Resource
	for index := range manifest.Resources {
		if strings.HasSuffix(manifest.Resources[index].Path, "/SKILL.md") {
			entry = &manifest.Resources[index]
			break
		}
	}
	if entry == nil {
		return SkillReadResult{}, fmt.Errorf("%w: active skill has no SKILL.md manifest entry", skill.ErrSnapshotExpired)
	}
	contents, err := skill.ReadResource(ctx, root, manifest.CatalogSnapshot, entry.Path, entry.Digest)
	if err != nil {
		return SkillReadResult{}, err
	}
	response := SkillReadResult{Result: NewResult(StatusOK, "Active skill loaded from a digest-pinned catalog snapshot."), Manifest: manifest, Content: string(contents)}
	response.Items = append(response.Items, Item{ID: id, Summary: manifest.Name, Impact: fmt.Sprintf("%d digest-pinned resource(s).", len(manifest.Resources))})
	return response, nil
}

func makeSkillProposal(proposal skill.Proposal, summary string) SkillProposal {
	pins := ConfirmationPins{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseVersion: proposal.BaseSnapshot}
	response := SkillProposal{
		Result:  NewResult(StatusActionRequired, summary),
		SkillID: proposal.SkillID, Command: proposal.Command, Diff: proposal.Summary, FullDiff: proposal.FullDiff,
		Confirmation: ConfirmationPolicy{
			PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: proposal.Command,
			Confirmation: ConfirmationRequirement{Required: true, Mode: "preview-and-approval", Pins: pins},
		},
		RoutingImpact: proposal.RoutingImpact, proposal: proposal,
	}
	response.Items = append(response.Items, Item{ID: proposal.ID, Summary: diffSummary(proposal.Summary), Impact: "No canonical files changed during preview."})
	response.SuggestedActions = append(response.SuggestedActions, Action{Label: "Confirm the reviewed proposal", Command: proposal.Command, RequiresConfirmation: true})
	return response
}

func staleSkillProposal(preview SkillProposal) SkillMutationResult {
	confirmation := preview.Confirmation
	result := SkillMutationResult{Result: NewResult(StatusError, "Proposal is stale; nothing was applied."), SkillID: preview.SkillID, Confirmation: &confirmation}
	result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{
		Error: "The proposal can no longer be confirmed.",
		Why:   "The target or confirmation pins changed after the preview was created.",
		Fix:   "Regenerate the proposal and review the updated diff.",
	}}
	result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Regenerate the proposal", Command: "PreviewSkillUpdate"})
	return result
}

func diffSummary(diff skill.DiffSummary) string {
	return fmt.Sprintf("%d added, %d modified, %d deleted canonical file(s).", len(diff.Added), len(diff.Modified), len(diff.Deleted))
}

func gitMutationSummary(dirty bool) string {
	if dirty {
		return "Canonical changes are not committed to Git."
	}
	return "Git working tree is clean."
}
