package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	RecoveryID    string               `json:"recovery_id,omitempty"`
	proposal      skill.Proposal
}

// MissingActivationRequirementsError indicates a skill cannot transition to active
// because one or more required routing fields are missing.
type MissingActivationRequirementsError struct {
	SkillID string   `json:"skill_id"`
	Missing []string `json:"missing"`
}

func (e *MissingActivationRequirementsError) Error() string {
	return fmt.Sprintf("skill %s cannot be activated: missing required fields: %s", e.SkillID, strings.Join(e.Missing, ", "))
}

// SkillMutationResult reports active local state and the Git boundary.
type SkillMutationResult struct {
	Result
	SkillID         string              `json:"skill_id"`
	State           string              `json:"state,omitempty"`
	LifecycleState  string              `json:"lifecycle_state,omitempty"`
	RoutingEligible bool                `json:"routing_eligible"`
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
		var conflictErr *skill.EditConflictError
		if errors.As(err, &conflictErr) {
			why := fmt.Sprintf("SKILL.md was modified concurrently: expected %s, found %s.", conflictErr.ExpectedDigest, conflictErr.ActualDigest)
			fix := fmt.Sprintf("Re-read current content with `skillhub skill review %s` or inspect differences, then retry your edit.", id)
			return SkillProposal{}, NewEditConflictError(why, fix)
		}
		if errors.Is(err, skill.ErrNotFound) {
			return SkillProposal{}, fmt.Errorf("%w: Skill %s does not exist; use skill_create_preview", skill.ErrNotFound, id)
		}
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
	if target == "active" {
		missing, err := service.CheckActivationRequirements(ctx, path, id)
		if err != nil {
			return SkillProposal{}, err
		}
		if len(missing) > 0 {
			return SkillProposal{}, &MissingActivationRequirementsError{
				SkillID: id,
				Missing: missing,
			}
		}
	}
	proposal, err := service.Manager.PreviewTransition(ctx, root, id, target, fullDiff, idempotencyKey)
	if err != nil {
		if errors.Is(err, skill.ErrUntouchedScaffold) {
			return SkillProposal{}, &MissingActivationRequirementsError{
				SkillID: id,
				Missing: []string{"content (untouched scaffold instructions; edit instructions before activation)"},
			}
		}
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
		if pins.ProposalDigest != preview.Confirmation.Confirmation.Pins.ProposalDigest {
			return digestMismatchSkillProposal(preview), nil
		}
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
	state := skillStateAfter(ctx, root, preview)
	activeLocally := (state == "active")
	response := SkillMutationResult{
		Result:  NewResult(StatusApplied, appliedSkillSummary(preview.SkillID, preview.Command, state)),
		SkillID: preview.SkillID, State: state, LifecycleState: state,
		RoutingEligible: activeLocally, OperationID: result.OperationID, ChangedPaths: result.ChangedPaths,
		CatalogSnapshot: result.CatalogSnapshot, Generation: result.Generation,
		ActiveLocally: activeLocally, GitDirty: result.GitDirty,
	}
	if preview.proposal.RecoveryID != "" {
		_ = skill.DeleteEditorRecovery(root, preview.proposal.RecoveryID)
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

// ReadSkillRouting returns a skill's stored routing fields in any state.
func (SkillService) ReadSkillRouting(ctx context.Context, path, id string) (skill.RoutingInput, error) {
	if err := ctx.Err(); err != nil {
		return skill.RoutingInput{}, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return skill.RoutingInput{}, err
	}
	return skill.ReadRouting(root, id)
}

// ReadSkillRationale returns a skill's stored routing review rationale in any state.
func (SkillService) ReadSkillRationale(ctx context.Context, path, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return "", err
	}
	return skill.ReadRationale(root, id)
}

// CheckActivationRequirements returns all missing requirements for activating a skill.
// Active skills require at least one trigger, not_for or rationale, min_scope,
// and must not be an untouched generated scaffold template.
func (SkillService) CheckActivationRequirements(ctx context.Context, path, id string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return nil, err
	}
	routing, err := skill.ReadRouting(root, id)
	if err != nil {
		return nil, err
	}
	rationale, err := skill.ReadRationale(root, id)
	if err != nil {
		return nil, err
	}
	content, err := skill.ReadEditableContent(root, id)
	if err != nil {
		return nil, err
	}
	var missing []string
	if skill.IsUntouchedScaffold(content) {
		missing = append(missing, "content (replace untouched scaffold instructions)")
	}
	if len(routing.Triggers) == 0 {
		missing = append(missing, "trigger")
	}
	if len(routing.NotFor) == 0 && strings.TrimSpace(rationale) == "" {
		missing = append(missing, "not_for or rationale (quality.routing_review_rationale)")
	}
	if strings.TrimSpace(routing.MinScope) == "" {
		missing = append(missing, "min_scope")
	}
	return missing, nil
}

func (SkillService) ReadSkill(ctx context.Context, path, id string) (SkillReadResult, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillReadResult{}, err
	}
	manifest, err := skill.GetManifestAnyState(ctx, root, id)
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
		return SkillReadResult{}, fmt.Errorf("%w: skill has no SKILL.md manifest entry", skill.ErrSnapshotExpired)
	}
	contents, err := skill.ReadResource(ctx, root, manifest.CatalogSnapshot, entry.Path, entry.Digest)
	if err != nil {
		if errors.Is(err, skill.ErrResourceDigestMismatch) {
			why := fmt.Sprintf("Current canonical content for %s no longer matches published manifest digest %s.", entry.Path, entry.Digest)
			fix := fmt.Sprintf("Run `skillhub rebuild` to publish current canonical files, or inspect differences with `skillhub skill review %s`.", id)
			return SkillReadResult{}, NewResourceContentUnavailableError(why, fix)
		}
		return SkillReadResult{}, err
	}
	response := SkillReadResult{Result: NewResult(StatusOK, fmt.Sprintf("Skill %s (%s) loaded.", id, manifest.Status)), Manifest: manifest, Content: string(contents)}
	response.Items = append(response.Items, Item{ID: id, Summary: manifest.Name, Impact: fmt.Sprintf("%d resource(s).", len(manifest.Resources))})
	return response, nil
}

// skillStateAfter reports the lifecycle state a confirmed mutation left the
// skill in. Transitions name their target; create always yields a draft; other
// commands read the freshly published catalog. Empty means unknown.
func skillStateAfter(ctx context.Context, root string, preview SkillProposal) string {
	switch {
	case preview.Command == "skill_create":
		return "draft"
	case strings.HasPrefix(preview.Command, "skill_") && preview.Command != "skill_edit":
		return strings.TrimPrefix(preview.Command, "skill_")
	}
	manifest, err := skill.GetManifestAnyState(ctx, root, preview.SkillID)
	if err != nil {
		return ""
	}
	return manifest.Status
}

func appliedSkillSummary(id, command, state string) string {
	switch {
	case command == "skill_create":
		return fmt.Sprintf("Draft skill %s saved.", id)
	case command == "skill_edit" && state == "draft":
		return fmt.Sprintf("Draft skill %s saved.", id)
	case command == "skill_edit" && state != "":
		return fmt.Sprintf("Skill %s updated; it is %s.", id, state)
	case command == "skill_edit":
		return fmt.Sprintf("Skill %s updated.", id)
	case state != "":
		return fmt.Sprintf("Skill %s is now %s.", id, state)
	}
	return fmt.Sprintf("Skill %s changed.", id)
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
		RoutingImpact: proposal.RoutingImpact,
		RecoveryID:    proposal.RecoveryID,
		proposal:      proposal,
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

func digestMismatchSkillProposal(preview SkillProposal) SkillMutationResult {
	confirmation := preview.Confirmation
	result := SkillMutationResult{Result: NewResult(StatusError, "Proposal digest does not match the preview; nothing was applied."), SkillID: preview.SkillID, Confirmation: &confirmation}
	result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{
		Error: "The proposal cannot be confirmed.",
		Why:   "Proposal digest does not match the preview.",
		Fix:   "Pass the exact proposal digest printed by the preview command, or re-run with --yes.",
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

// ConfirmProposal loads a stored proposal by ID, verifies optional confirmation pins,
// and dispatches confirmation to the handler registered for the proposal's kind.
func (service SkillService) ConfirmProposal(ctx context.Context, path, proposalID string, pins *ConfirmationPins) (SkillMutationResult, error) {
	if err := ctx.Err(); err != nil {
		return SkillMutationResult{}, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillMutationResult{}, err
	}
	stored, err := skill.LoadProposal(root, proposalID, time.Now())
	if err != nil {
		return SkillMutationResult{}, err
	}
	preview := makeSkillProposal(stored, "Stored proposal")
	effectivePins := preview.Confirmation.Confirmation.Pins
	if pins != nil {
		effectivePins = *pins
	}
	return service.ConfirmSkillMutation(ctx, path, preview, effectivePins)
}

// DispatchConfirmProposal dispatches confirmation of a proposal by ID across any registered kind.
func (service SkillService) DispatchConfirmProposal(ctx context.Context, path, proposalID string, pins *ConfirmationPins) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return nil, err
	}
	stored, err := skill.LoadProposal(root, proposalID, time.Now())
	if err != nil {
		return nil, err
	}
	preview := makeSkillProposal(stored, "Stored proposal")
	effectivePins := preview.Confirmation.Confirmation.Pins
	if pins != nil {
		effectivePins = *pins
	}
	if stored.Kind == skill.ProposalKindLifecycle || stored.Kind == "" {
		return service.ConfirmSkillMutation(ctx, path, preview, effectivePins)
	}
	customProposalConfirmersMu.RLock()
	confirmer, ok := customProposalConfirmers[stored.Kind]
	customProposalConfirmersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported or unregistered proposal kind %q", stored.Kind)
	}
	return confirmer(ctx, path, stored, effectivePins)
}

var (
	customProposalConfirmers   = make(map[skill.ProposalKind]func(ctx context.Context, path string, p skill.Proposal, pins ConfirmationPins) (any, error))
	customProposalConfirmersMu sync.RWMutex
)

// RegisterProposalConfirmer registers a custom handler for non-lifecycle proposal kinds (e.g. ProposalKindAdd).
func RegisterProposalConfirmer(kind skill.ProposalKind, fn func(ctx context.Context, path string, p skill.Proposal, pins ConfirmationPins) (any, error)) {
	customProposalConfirmersMu.Lock()
	defer customProposalConfirmersMu.Unlock()
	customProposalConfirmers[kind] = fn
}

// ReadEditableSkill returns the canonical entrypoint and its digest for a skill.
func (SkillService) ReadEditableSkill(ctx context.Context, path, id string) (skill.EditableContent, error) {
	if err := ctx.Err(); err != nil {
		return skill.EditableContent{}, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return skill.EditableContent{}, err
	}
	return skill.ReadEditableSkill(root, id)
}

// SaveEditorRecovery persists edited bytes to private recovery storage and returns an opaque recovery ID.
func (SkillService) SaveEditorRecovery(ctx context.Context, path, proposalID string, content []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return "", err
	}
	return skill.SaveEditorRecovery(root, proposalID, content, time.Now())
}

// ReadEditorRecovery reads recovery content by opaque recovery ID.
func (SkillService) ReadEditorRecovery(ctx context.Context, path, recoveryID string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return nil, err
	}
	return skill.ReadEditorRecovery(root, recoveryID)
}

// DeleteEditorRecovery deletes a recovery artifact by opaque recovery ID.
func (SkillService) DeleteEditorRecovery(ctx context.Context, path, recoveryID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return err
	}
	return skill.DeleteEditorRecovery(root, recoveryID)
}
