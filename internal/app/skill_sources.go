package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

type LearningReference struct {
	SourceID        string               `json:"source_id"`
	Locator         string               `json:"locator"`
	Ref             string               `json:"ref,omitempty"`
	Path            string               `json:"path,omitempty"`
	Role            string               `json:"role"`
	Monitoring      sourcepkg.Monitoring `json:"monitoring"`
	LastCheckedAt   *time.Time           `json:"last_checked_at,omitempty"`
	Availability    string               `json:"availability,omitempty"`
	PendingInsights int                  `json:"pending_insights"`
}

type SkillSourcesResult struct {
	Result
	SkillID         string              `json:"skill_id"`
	Upstream        *SkillUpstream      `json:"upstream"`
	Learning        []LearningReference `json:"learning"`
	PendingInsights int                 `json:"pending_insights"`
}

func (service SourceService) SkillSources(ctx context.Context, path, skillID string) (SkillSourcesResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillSourcesResult{}, err
	}

	skillID = strings.TrimSpace(skillID)
	if skillID == "" {
		return SkillSourcesResult{}, skill.ErrNotFound
	}

	_, _, _, err = locateSkillDir(root, skillID)
	if err != nil {
		return SkillSourcesResult{}, skill.ErrNotFound
	}

	// 1. Get Upstream (nil if skill has no github/git origin)
	var upstream *SkillUpstream
	up, upErr := GetSkillUpstream(ctx, root, skillID)
	if upErr == nil && up.Repository != "" && up.Status != "untracked" {
		upstream = &up
	}

	// 2. Discover learning references from sources/skills/
	links, err := readSkillSourceLinks(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return SkillSourcesResult{}, err
	}

	_, records, err := readSourceRecords(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return SkillSourcesResult{}, err
	}
	recordsByID := make(map[string]sourcepkg.Record, len(records))
	for _, r := range records {
		recordsByID[r.ID] = r
	}

	store := sourcepkg.OperationalStore{Root: root}
	checkStates, _ := store.List(ctx)
	checkStatesByID := make(map[string]sourcepkg.CheckState, len(checkStates))
	for _, cs := range checkStates {
		checkStatesByID[cs.SourceID] = cs
	}

	learning := []LearningReference{}
	for _, l := range links {
		if l.SkillID != skillID {
			continue
		}
		rec, found := recordsByID[l.SourceID]
		locator := ""
		ref := ""
		path := ""
		monitoring := sourcepkg.Monitoring{Enabled: false, Cadence: "manual"}
		var lastChecked *time.Time
		availability := "available"

		if found {
			locator = rec.Locator.Repository
			if locator == "" {
				locator = rec.Locator.URL
			}
			if locator == "" {
				locator = rec.Locator.Path
			}
			ref = rec.Locator.Ref
			path = rec.Locator.Path
			monitoring = rec.Monitoring
			if rec.CurrentRevision != nil && !rec.CurrentRevision.ObservedAt.IsZero() {
				t := rec.CurrentRevision.ObservedAt
				lastChecked = &t
			}
			if rec.Status == "unavailable" {
				availability = "unavailable"
			}
		}
		if cs, ok := checkStatesByID[l.SourceID]; ok {
			if cs.Availability != "" {
				availability = cs.Availability
			}
		}

		learning = append(learning, LearningReference{
			SourceID:      l.SourceID,
			Locator:       locator,
			Ref:           ref,
			Path:          path,
			Role:          l.Role,
			Monitoring:    monitoring,
			LastCheckedAt: lastChecked,
			Availability:  availability,
		})
	}

	sort.Slice(learning, func(i, j int) bool {
		return learning[i].SourceID < learning[j].SourceID
	})

	summary := fmt.Sprintf("Sources for %s.", skillID)
	result := SkillSourcesResult{
		Result:   NewResult(StatusOK, summary),
		SkillID:  skillID,
		Upstream: upstream,
		Learning: learning,
	}
	return result, nil
}
