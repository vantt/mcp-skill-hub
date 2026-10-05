package app

import (
	"context"
	"fmt"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

// SkillListEntry is one skill row from the current catalog generation.
type SkillListEntry struct {
	ID              string `json:"id"`
	State           string `json:"state"`
	LifecycleState  string `json:"lifecycle_state,omitempty"`
	ActiveLocally   bool   `json:"active_locally"` // Deprecated compatibility alias for lifecycle_state == "active"
	RoutingEligible bool   `json:"routing_eligible"`
	Collection      string `json:"collection"`
	Name            string `json:"name"`
	UpstreamStatus  string `json:"upstream_status,omitempty"`
}

// SkillListResult lists skills from the current catalog generation.
type SkillListResult struct {
	Result
	Skills []SkillListEntry `json:"skills"`
}

// SkillStates are the lifecycle states accepted by ListSkills filters.
var SkillStates = []string{"draft", "active", "deprecated", "archived"}

// ListSkills reads skills from the current catalog. An empty state returns every state.
func (SkillService) ListSkills(ctx context.Context, path, state string) (SkillListResult, error) {
	if state != "" {
		known := false
		for _, candidate := range SkillStates {
			known = known || candidate == state
		}
		if !known {
			return SkillListResult{}, fmt.Errorf("unknown skill state %q; use one of draft, active, deprecated, archived", state)
		}
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillListResult{}, err
	}
	handle, err := catalog.OpenWithFallback(ctx, root)
	if err != nil {
		return SkillListResult{}, err
	}
	defer handle.Close()
	query := `SELECT id, status, collection_id, name FROM skills`
	args := []any{}
	if state != "" {
		query += ` WHERE status = ?`
		args = append(args, state)
	}
	rows, err := handle.DB.QueryContext(ctx, query+` ORDER BY collection_id, id`, args...)
	if err != nil {
		return SkillListResult{}, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	skills := []SkillListEntry{}
	for rows.Next() {
		var entry SkillListEntry
		if err := rows.Scan(&entry.ID, &entry.State, &entry.Collection, &entry.Name); err != nil {
			return SkillListResult{}, fmt.Errorf("read skill row: %w", err)
		}
		entry.LifecycleState = entry.State
		entry.ActiveLocally = (entry.State == "active")
		entry.RoutingEligible = (entry.State == "active")
		skills = append(skills, entry)
	}
	if err := rows.Err(); err != nil {
		return SkillListResult{}, fmt.Errorf("list skills: %w", err)
	}
	summary := fmt.Sprintf("%d skill(s).", len(skills))
	if handle.Status.ServingMode == catalog.ServingFallback {
		summary = fmt.Sprintf("%d skill(s) (served from fallback generation %s).", len(skills), handle.Pointer.Generation)
	}
	result := SkillListResult{Result: NewResult(StatusOK, summary), Skills: skills}
	for _, entry := range skills {
		result.Items = append(result.Items, Item{ID: entry.ID, Summary: entry.Name, Impact: "State: " + entry.State + "; collection: " + entry.Collection + "."})
	}
	return result, nil
}

// ListSkillsWithUpstream reads skills and merges their upstream drift statuses.
func (s SkillService) ListSkillsWithUpstream(ctx context.Context, path, state string) (SkillListResult, error) {
	res, err := s.ListSkills(ctx, path, state)
	if err != nil {
		return SkillListResult{}, err
	}
	upstreams, err := ListSkillUpstream(ctx, path)
	if err != nil {
		return res, nil
	}
	statusMap := make(map[string]string, len(upstreams))
	for _, u := range upstreams {
		statusMap[u.SkillID] = u.Status
	}
	for i := range res.Skills {
		res.Skills[i].UpstreamStatus = statusMap[res.Skills[i].ID]
	}
	return res, nil
}
