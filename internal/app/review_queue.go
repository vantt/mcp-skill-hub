package app

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/skill"
)

// Kinds of a skill waiting for a person.
const (
	ReviewKindDraft          = "draft"
	ReviewKindContentChanged = "content_changed"
)

// ReviewQueueItem is one skill that needs a human before agents get it.
type ReviewQueueItem struct {
	Kind    string `json:"kind"`
	SkillID string `json:"skill_id"`
	Title   string `json:"title"`
	// Reason says in words why the skill is waiting.
	Reason string `json:"reason"`
	// Since is the date of the skill's last commit, or empty without Git.
	Since  string `json:"since,omitempty"`
	Origin string `json:"origin,omitempty"`
}

// ReviewQueueResult lists what waits for review: new drafts, and active skills
// whose third-party content changed after its last approval.
type ReviewQueueResult struct {
	Result
	Items []ReviewQueueItem `json:"items"`
}

// ReviewQueue collects the skills a person has to look at.
func (service SkillService) ReviewQueue(ctx context.Context, path string) (ReviewQueueResult, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return ReviewQueueResult{}, err
	}
	listed, err := service.ListSkills(ctx, path, "")
	if err != nil {
		return ReviewQueueResult{}, err
	}
	items := []ReviewQueueItem{}
	for _, entry := range listed.Skills {
		item := ReviewQueueItem{SkillID: entry.ID, Title: entry.Name}
		switch entry.State {
		case "draft":
			item.Kind, item.Reason = ReviewKindDraft, "new draft"
		case "active":
			trust, trustErr := service.contentTrustWithChanges(ctx, path, entry.ID)
			if trustErr != nil || !trust.RequiresReview() {
				continue
			}
			item.Kind, item.Reason = ReviewKindContentChanged, "content needs approval"
			if trust.ChangesSinceApproval != nil {
				item.Reason = "content changed since it was approved"
			}
		default:
			continue
		}
		item.Since, item.Origin = queueFacts(ctx, root, entry.ID)
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Since != items[j].Since {
			return items[i].Since > items[j].Since
		}
		return items[i].SkillID < items[j].SkillID
	})
	result := ReviewQueueResult{Result: NewResult(StatusOK, pluralize(len(items), "skill")+" waiting for review."), Items: items}
	return result, ctx.Err()
}

// queueFacts reads when a skill last changed and where it came from.
func queueFacts(ctx context.Context, root, id string) (since, origin string) {
	_, relDir, meta, err := locateSkillDir(root, id)
	if err != nil {
		return "", ""
	}
	if out, gitErr := gitOutput(ctx, root, "log", "-1", "--format=%cs", "--", filepath.ToSlash(relDir)); gitErr == nil {
		since = strings.TrimSpace(string(out))
	}
	if provenance := extractSkillProvenance(parseSkillReviewMeta(meta)); provenance != nil {
		origin = strings.TrimSpace(provenance.SourceLocator)
	}
	if origin == "" {
		origin = thirdPartyOrigin(meta)
	}
	return since, origin
}
