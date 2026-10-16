package app

import (
	"fmt"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// commitView is the additive `commit` object of a mutation result. It is nil
// when the confirming surface did not ask for a commit.
func commitView(outcome workspace.CommitOutcome) *workspace.CommitOutcome {
	if outcome.Status == "" {
		return nil
	}
	return &outcome
}

// commitItem tells a person what happened to the Git commit, and who gets the
// new version and when. Without a commit it keeps the old reminder that the
// changed files still have to be committed.
func commitItem(outcome workspace.CommitOutcome, gitDirty bool) Item {
	switch outcome.Status {
	case workspace.CommitCommitted:
		return Item{
			ID:      "commit",
			Summary: fmt.Sprintf("Committed %s %q.", workspace.ShortHash(outcome.Hash), outcome.Message),
			Impact:  reachText(outcome.Reach),
		}
	case workspace.CommitFailed:
		return Item{
			ID:      "commit",
			Summary: "Not committed: " + outcome.Reason,
			Impact:  "The files are written. Finish it with: " + outcome.ManualCommand,
		}
	case workspace.CommitSkipped:
		return Item{ID: "commit", Summary: "Not committed: " + outcome.Reason, Impact: "The files are written."}
	case workspace.CommitDeferred:
		return Item{ID: "commit", Summary: "The commit comes with the rest of this change.", Impact: "The files are written."}
	}
	return Item{ID: "git_state", Summary: gitMutationSummary(gitDirty), Impact: "No commit was made; commit the changed files when you are ready."}
}

// reachText says who gets a committed version and when: the agents of this
// hub, for a change to skills, and other clones after a push.
func reachText(reach workspace.Reach) string {
	var parts []string
	if reach.Agents != "" {
		parts = append(parts, reach.Agents)
	}
	if reach.PushNeeded {
		parts = append(parts, "Other clones get it after `"+reach.PushCommand+"`.")
	}
	if len(parts) == 0 {
		return "Committed to this hub."
	}
	return strings.Join(parts, " ")
}
