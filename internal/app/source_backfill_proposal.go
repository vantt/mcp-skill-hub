package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"time"

	"github.com/vantt/skillhub-mcp/internal/skill"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// UpstreamTrackProposal is a pinned, stored proposal that links one skill's
// untracked upstream to a source record. It is confirmed through the shared
// source proposal confirm step.
type UpstreamTrackProposal struct {
	SourceProposal
	FullDiff      string                 `json:"full_diff,omitempty"`
	Candidate     *BackfillCandidateItem `json:"candidate,omitempty"`
	skillNotFound bool
}

// SkillNotFound reports whether the proposal failed because the skill does
// not exist in the workspace.
func (p UpstreamTrackProposal) SkillNotFound() bool {
	return p.skillNotFound
}

// PreviewTrackUpstream plans the backfill of one skill and stores it as a
// source proposal, together with the full diff of every file it would write.
// Nothing is written to the workspace until the proposal is confirmed.
func (service SourceService) PreviewTrackUpstream(ctx context.Context, path, skillID string) (UpstreamTrackProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return UpstreamTrackProposal{}, err
	}
	if err := ctx.Err(); err != nil {
		return UpstreamTrackProposal{}, err
	}
	service = service.defaults(root)

	skillIDs, err := listAllSkillIDs(root)
	if err != nil {
		return UpstreamTrackProposal{}, err
	}
	if !slices.Contains(skillIDs, skillID) {
		return UpstreamTrackProposal{
			SourceProposal: SourceProposal{Result: ErrorResult(NewInvalidRequestError(fmt.Sprintf("skill %q not found", skillID), "Check skill ID."))},
			skillNotFound:  true,
		}, nil
	}

	preview, err := service.PreviewBackfill(ctx, root, BackfillInput{SkillID: skillID})
	if err != nil {
		return UpstreamTrackProposal{}, err
	}
	if preview.Error != nil {
		return UpstreamTrackProposal{SourceProposal: SourceProposal{Result: preview.Result}}, nil
	}
	if len(preview.Candidates) == 0 || len(preview.changes) == 0 {
		return UpstreamTrackProposal{
			SourceProposal: SourceProposal{
				Result: NewResult(StatusOK, "Nothing to track: the skill has no untracked upstream."),
				Diff:   SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{}},
			},
		}, nil
	}
	candidate := preview.Candidates[0]

	record := sourcepkg.Record{ID: candidate.SourceID}
	_, records, err := readSourceRecords(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return UpstreamTrackProposal{}, err
	}
	for _, existing := range records {
		if existing.ID == candidate.SourceID {
			record = existing
			break
		}
	}

	fullDiff, err := renderBackfillDiff(root, preview)
	if err != nil {
		return UpstreamTrackProposal{}, err
	}

	now := service.Clock.Now().UTC()
	pins := ConfirmationPins{
		ProposalID:     preview.planned.ID,
		ProposalDigest: preview.planned.Digest,
		BaseVersion:    preview.planned.BaseCatalogSnapshot,
	}
	proposal := UpstreamTrackProposal{
		SourceProposal: SourceProposal{
			Result: NewResult(StatusActionRequired, fmt.Sprintf("Track upstream proposal for %s on %s is ready for review.", candidate.SourceID, candidate.SkillID)),
			Source: record,
			Diff:   preview.Diff,
			Confirmation: ConfirmationPolicy{
				PolicyRevision:     "policy_v1",
				ActionClass:        "semantic",
				ApplicationCommand: "source_watch_confirm",
				Confirmation: ConfirmationRequirement{
					Required: true,
					Mode:     "preview-and-approval",
					Pins:     pins,
				},
			},
			planned:   preview.planned,
			expiresAt: now.Add(24 * time.Hour),
		},
		FullDiff:  fullDiff,
		Candidate: &candidate,
	}
	if err := storeSourceProposal(root, proposal.SourceProposal, now); err != nil {
		return UpstreamTrackProposal{}, err
	}
	return proposal, nil
}

// renderBackfillDiff renders the planned changes against the current
// contents of each path; a path that does not exist yet is shown as added.
func renderBackfillDiff(root string, preview BackfillPreview) (string, error) {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer handle.Close()
	before := make(map[string][]byte, len(preview.changes))
	for _, change := range preview.changes {
		data, readErr := handle.ReadFile(change.Path)
		if readErr != nil {
			if errors.Is(readErr, fs.ErrNotExist) {
				continue
			}
			return "", readErr
		}
		before[change.Path] = data
	}
	return skill.RenderFullDiff(preview.changes, before), nil
}
