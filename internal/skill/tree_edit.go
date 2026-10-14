package skill

import (
	"context"
	"errors"
	"fmt"

	"github.com/vantt/skillhub-mcp/internal/mutation"
)

// PreviewChanges plans one skill_edit write set from a list of file changes
// the caller has already validated and resolved to workspace-relative paths:
// every added, modified and deleted file of a skill, and optionally its
// metadata. One proposal is one mutation, so confirming it is one Git commit.
func (manager Manager) PreviewChanges(ctx context.Context, root, id string, changes []mutation.Change, fullDiff bool) (Proposal, error) {
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	if len(changes) == 0 {
		return Proposal{}, errors.New("there are no changes to preview")
	}
	const command = "skill_edit"
	requestDigest := normalizedDigest(struct {
		ID      string
		Changes []mutation.Change
	}{id, changes})
	idempotencyKey := lifecycleIdempotencyKey("", command, id, requestDigest)
	if prior, found, err := mutation.LookupOperation(root, mutation.WriteSet{Command: command, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest}); err != nil {
		return Proposal{}, err
	} else if found {
		return manager.appliedProposal(root, id, command, prior, requestDigest, idempotencyKey), nil
	}
	proposal, err := manager.plan(ctx, root, id, command, changes, nil, nil, fullDiff, idempotencyKey, requestDigest)
	if err != nil {
		return Proposal{}, fmt.Errorf("plan skill changes: %w", err)
	}
	return proposal, nil
}
