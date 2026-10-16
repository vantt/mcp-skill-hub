package app

import (
	"context"
	"errors"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// AgentsGetItNow is what a committed change of a skill means for the agents
// that use the hub when they get the skill as it is now.
const AgentsGetItNow = "Agents using this hub get it now."

// noteAgentReach records on a committed outcome what the change means for the
// agents that use this hub: they get a skill only while it is active, the hub
// can serve it and its content is approved. Several skills are named one by
// one. A change that was not committed is left alone.
func noteAgentReach(ctx context.Context, root string, outcome *workspace.CommitOutcome, ids ...string) {
	if outcome == nil || outcome.Status != workspace.CommitCommitted || len(ids) == 0 {
		return
	}
	if len(ids) == 1 {
		outcome.Reach.Agents = agentReachSentence(ctx, root, ids[0], "it")
		return
	}
	sentences := make([]string, 0, len(ids))
	for _, id := range ids {
		sentence := agentReachSentence(ctx, root, id, id)
		if sentence == AgentsGetItNow {
			sentence = "Agents using this hub get " + id + " now."
		}
		sentences = append(sentences, sentence)
	}
	outcome.Reach.Agents = strings.Join(sentences, " ")
}

// agentReachSentence says whether agents get the skill id now, and if not,
// what stops them; name is how the sentence refers to the skill.
func agentReachSentence(ctx context.Context, root, id, name string) string {
	trusted := false
	_, _, err := loadDistributedSkillManifest(ctx, root, id, func(entry DistributedSkill, contentJSON []byte) (bool, error) {
		trust, err := evaluateSkillTrust(id, contentJSON, entry.digests)
		if err != nil {
			return false, err
		}
		trusted = trust.Verdict.Trusted
		return false, nil
	})
	switch {
	case errors.Is(err, skill.ErrNotFound):
		manifest, stateErr := skill.GetManifestAnyState(ctx, root, id)
		switch {
		case stateErr != nil:
			return "Agents do not get " + name + "."
		case manifest.Status == "draft":
			return "Agents do not get " + name + " until it is activated."
		case manifest.Status == "active":
			return "Agents do not get " + name + " yet: the search index does not list it; `skillhub doctor` says why."
		}
		return "Agents no longer get " + name + ": it is " + manifest.Status + "."
	case errors.Is(err, catalog.ErrSkillNotServable):
		return "Agents cannot get " + name + ": the hub cannot serve it; `skillhub doctor` says why."
	case err != nil:
		return "Whether agents get " + name + " could not be checked: " + plainError(err) + "."
	case !trusted:
		return "Agents do not get " + name + " until its content is approved."
	}
	return AgentsGetItNow
}
