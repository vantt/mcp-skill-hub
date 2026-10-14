package app

import (
	"context"
	"sort"

	"github.com/vantt/skillhub-mcp/internal/mutation"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// commitChain lets several confirmed changes land as one Git commit: each step
// runs with its own commit deferred, and Finish commits everything the steps
// changed once. A caller whose context carries no commit policy, or asked for
// none, gets the steps run and nothing committed.
type commitChain struct {
	ctx     context.Context
	root    string
	message string
	policy  workspace.CommitPolicy
	asked   bool
	paths   map[string]bool
	ops     []string
}

// beginCommitChain starts a chain over the given hub-relative paths. Steps skip
// the check for uncommitted edits, because their own earlier steps leave those
// files dirty, so the check is made here once, before anything is written.
func beginCommitChain(ctx context.Context, root, message string, touched []string) (*commitChain, error) {
	policy, asked := workspace.CommitPolicyFrom(ctx)
	chain := &commitChain{ctx: ctx, root: root, message: message, policy: policy, asked: asked && !policy.Disabled, paths: map[string]bool{}}
	if !chain.asked {
		return chain, nil
	}
	dirty, err := workspace.DirtyPaths(ctx, root, touched)
	if err != nil {
		return nil, err
	}
	if len(dirty) > 0 {
		return nil, &mutation.UncommittedEditsError{Root: root, Paths: dirty}
	}
	deferred := policy
	deferred.Defer = true
	chain.ctx = workspace.WithCommitPolicy(ctx, deferred)
	return chain, nil
}

// SetMessage replaces the commit subject, for a chain whose last step did not
// go as planned.
func (chain *commitChain) SetMessage(message string) { chain.message = message }

// Context is the context every step of the chain runs with.
func (chain *commitChain) Context() context.Context { return chain.ctx }

// Record notes what a finished step changed.
func (chain *commitChain) Record(receipt mutation.Receipt) {
	for _, path := range receipt.ChangedPaths {
		chain.paths[path] = true
	}
	if receipt.OperationID != "" {
		chain.ops = append(chain.ops, receipt.OperationID)
	}
}

// Finish commits the recorded paths once. It returns nil when no commit was
// asked for.
func (chain *commitChain) Finish() *workspace.CommitOutcome {
	if !chain.asked {
		return nil
	}
	paths := make([]string, 0, len(chain.paths))
	for path := range chain.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	message := chain.message
	if chain.policy.Message != "" {
		message = chain.policy.Message
	}
	if len(chain.ops) > 0 {
		message += "\n\nSkillhub-Operation: " + chain.ops[0]
	}
	outcome := workspace.CommitPaths(chain.ctx, chain.root, paths, message, chain.policy.Interactive)
	return &outcome
}
