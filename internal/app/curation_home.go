package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const (
	AvailabilityAvailable     = "available"
	AvailabilityNotConfigured = "not_configured"
	AvailabilityUnavailable   = "unavailable"
)

// CurationWorkspace summarizes local-only workspace state.
type CurationWorkspace struct {
	Health          string `json:"health"`
	Index           string `json:"index"`
	GitDirty        bool   `json:"git_dirty"`
	GitConfigured   bool   `json:"git_configured"`
	RecoveryPending bool   `json:"recovery_pending"`
}

// ActionCategory makes zero counts and unsupported/not-configured capabilities explicit.
type ActionCategory struct {
	Kind         string `json:"kind"`
	Count        int    `json:"count"`
	Availability string `json:"availability"`
}

// ActionItem is a prioritized local action. Higher priority sorts first.
type ActionItem struct {
	Kind     string `json:"kind"`
	ID       string `json:"id,omitempty"`
	Count    int    `json:"count,omitempty"`
	Priority int    `json:"priority"`
	Summary  string `json:"summary"`
	Command  string `json:"command"`
}

// CurationSummary contains compact counts that are safe to show at L0.
type CurationSummary struct {
	ActiveSkills             int `json:"active_skills"`
	WatchingSources          int `json:"watching_sources"`
	FailedOrInterruptedRuns  int `json:"failed_or_interrupted_runs"`
	PendingInsights          int `json:"pending_insights"`
	PendingHighValueInsights int `json:"pending_high_value_insights"`
	AttentionItems           int `json:"attention_items"`
	InvalidWorkspaces        int `json:"invalid_workspaces"`
	RecoveryItems            int `json:"recovery_items"`
	OptionalItems            int `json:"optional_items"`
	SourcesDue               int `json:"sources_due"`
	UnavailableSources       int `json:"unavailable_sources"`
	UpstreamUpdates          int `json:"upstream_updates"`
}

// CurationHome is the single read model rendered by human, JSON, and quiet adapters.
type CurationHome struct {
	Result
	Workspace   CurationWorkspace `json:"workspace"`
	Actions     []ActionItem      `json:"actions"`
	Categories  []ActionCategory  `json:"categories"`
	HomeSummary CurationSummary   `json:"home_summary"`
	CountsKnown bool              `json:"-"`
}

// CurationService owns the local/offline Curation Home application query.
type CurationService struct{}

// GetCurationHome reads only workspace files, Git metadata, and the local derived catalog.
func (CurationService) GetCurationHome(ctx context.Context, path string) (CurationHome, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return CurationHome{}, err
	}
	if err := ctx.Err(); err != nil {
		return CurationHome{}, err
	}
	recoveries, err := mutation.InspectRecovery(root)
	if err != nil {
		return CurationHome{}, err
	}
	var issues []canonical.Issue
	if len(recoveries) == 0 {
		issues, err = canonical.Validate(root)
		if err != nil {
			return CurationHome{}, err
		}
	}
	git, err := inspectGit(ctx, root)
	if err != nil {
		return CurationHome{}, err
	}

	state := homeState{
		Health:          "valid",
		Index:           "current",
		GitDirty:        len(git.Files) > 0,
		GitConfigured:   git.Configured,
		RecoveryPending: len(recoveries) > 0,
		Categories:      defaultCategories(),
	}
	if len(issues) > 0 {
		state.Health = "invalid"
		state.Index = "stale"
		state.DiagnosticID = "workspace_validation"
		state.InvalidReason = fmt.Sprintf("Canonical validation found %d issue(s).", len(issues))
		state.DiagnosticSummary = issues[0].Path + ": " + issues[0].Message
		state.DiagnosticImpact = "Canonical data must be corrected before catalog-backed work can continue."
	}
	if len(recoveries) > 0 {
		state.RecoveryID = recoveries[0].OperationID
	}
	if state.Health == "valid" {
		var catalogStatus catalog.Status
		var inspectErr error
		if state.RecoveryPending {
			catalogStatus, inspectErr = catalog.InspectPublished(ctx, root)
		} else {
			catalogStatus, inspectErr = catalog.Inspect(ctx, root)
		}
		if inspectErr != nil {
			return CurationHome{}, inspectErr
		}
		state.Index = string(catalogStatus.State)
		if catalogStatus.State == catalog.StateHealthy {
			state.Index = "current"
			if err := readHomeCounts(ctx, root, &state); err != nil {
				return CurationHome{}, err
			}
		} else {
			markCatalogCategoriesUnavailable(state.Categories)
		}
	} else {
		markCatalogCategoriesUnavailable(state.Categories)
		state.CountsKnown = false
	}
	return deriveCurationHome(state), nil
}

type homeState struct {
	Health            string
	Index             string
	GitDirty          bool
	GitConfigured     bool
	RecoveryPending   bool
	RecoveryID        string
	DiagnosticID      string
	DiagnosticSummary string
	DiagnosticImpact  string
	InvalidReason     string
	Summary           CurationSummary
	// TotalSkills and TotalSources count every state; CountsKnown is false when
	// the catalog could not be read, so a missing count is never taken for zero.
	TotalSkills        int
	TotalSources       int
	CountsKnown        bool
	InterruptedID      string
	ChangedSources     int
	DueSources         int
	UnavailableSources int
	Categories         []ActionCategory
}

func defaultCategories() []ActionCategory {
	return []ActionCategory{
		{Kind: "interrupted_runs", Availability: AvailabilityAvailable},
		{Kind: "changed_sources", Availability: AvailabilityAvailable},
		{Kind: "pending_insights", Availability: AvailabilityAvailable},
		{Kind: "source_unavailable", Availability: AvailabilityAvailable},
		{Kind: "sources_due", Availability: AvailabilityAvailable},
		{Kind: "blocking_decisions", Availability: AvailabilityNotConfigured},
		{Kind: "routing_evaluations", Availability: AvailabilityNotConfigured},
	}
}

func markCatalogCategoriesUnavailable(categories []ActionCategory) {
	for index := range categories {
		if categories[index].Availability == AvailabilityAvailable {
			categories[index].Availability = AvailabilityUnavailable
		}
	}
}

func readHomeCounts(ctx context.Context, root string, state *homeState) (resultErr error) {
	handle, err := catalog.OpenCurrent(ctx, root)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := handle.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
	}()
	queries := []struct {
		query  string
		target *int
	}{
		{`SELECT count(*) FROM skills WHERE status='active'`, &state.Summary.ActiveSkills},
		{`SELECT count(*) FROM skills`, &state.TotalSkills},
		{`SELECT count(*) FROM canonical_entities WHERE kind='source'`, &state.TotalSources},
		{`SELECT count(*) FROM canonical_entities WHERE kind='source' AND COALESCE(json_extract(content_json,'$.status'),'watching') IN ('watching','changed','distill_pending')`, &state.Summary.WatchingSources},
		{`SELECT count(*) FROM provenance WHERE kind='run' AND state IN ('failed','interrupted')`, &state.Summary.FailedOrInterruptedRuns},
		{`SELECT count(*) FROM insights WHERE status='pending'`, &state.Summary.PendingInsights},
		{`SELECT count(*) FROM insights WHERE status='pending' AND (json_extract(content_json,'$.high_value')=1 OR json_extract(content_json,'$.priority') IN ('high','critical'))`, &state.Summary.PendingHighValueInsights},
		{`SELECT count(*) FROM canonical_entities s WHERE s.kind='source' AND (json_extract(s.content_json,'$.status') IN ('changed', 'distill_pending') OR json_extract(s.content_json,'$.distilled_revision') IS NULL) AND NOT (json_extract(s.content_json,'$.purpose') = 'upstream' AND NOT EXISTS (SELECT 1 FROM canonical_entities l WHERE l.kind='skill_source_link' AND json_extract(l.content_json,'$.source_id') = s.id AND json_extract(l.content_json,'$.role') IN ('learning-source','inspiration')))`, &state.ChangedSources},
	}
	for _, item := range queries {
		if err := handle.DB.QueryRowContext(ctx, item.query).Scan(item.target); err != nil {
			return fmt.Errorf("read curation count: %w", err)
		}
	}
	if state.Summary.FailedOrInterruptedRuns > 0 {
		if err := handle.DB.QueryRowContext(ctx, `SELECT id FROM provenance WHERE kind='run' AND state IN ('failed','interrupted') ORDER BY CASE state WHEN 'interrupted' THEN 0 ELSE 1 END,id LIMIT 1`).Scan(&state.InterruptedID); err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read interrupted run: %w", err)
		}
	}
	candidates, _, err := readSourceRecords(root)
	if err != nil {
		return fmt.Errorf("read source candidates: %w", err)
	}
	state.TotalSources += len(candidates)
	state.CountsKnown = true
	operational, err := (sourcepkg.OperationalStore{Root: root}).List(ctx)
	if err != nil {
		return fmt.Errorf("read source operational state: %w", err)
	}
	states := make(map[string]sourcepkg.CheckState, len(operational))
	for _, item := range operational {
		states[item.SourceID] = item
		if item.Availability == "unavailable" {
			state.UnavailableSources++
		}
	}
	rows, err := handle.DB.QueryContext(ctx, `SELECT id,content_json FROM canonical_entities WHERE kind='source' ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read source monitoring policy: %w", err)
	}
	defer rows.Close()
	now := time.Now().UTC()
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			return err
		}
		var document struct {
			Status     string `json:"status"`
			Monitoring struct {
				Enabled bool   `json:"enabled"`
				Cadence string `json:"cadence"`
			} `json:"monitoring"`
		}
		if err := json.Unmarshal([]byte(content), &document); err != nil {
			return err
		}
		if !document.Monitoring.Enabled || document.Monitoring.Cadence == "manual" || document.Status == "changed" || document.Status == "distill_pending" {
			continue
		}
		check, found := states[id]
		if found && check.Availability == "unavailable" {
			continue // already represented by the higher-priority retry action
		}
		if !found || !check.NextCheckAt.After(now) {
			state.DueSources++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	state.Summary.SourcesDue = state.DueSources
	state.Summary.UnavailableSources = state.UnavailableSources
	setCategoryCount(state.Categories, "interrupted_runs", state.Summary.FailedOrInterruptedRuns)
	setCategoryCount(state.Categories, "source_unavailable", state.UnavailableSources)
	setCategoryCount(state.Categories, "changed_sources", state.ChangedSources)
	setCategoryCount(state.Categories, "sources_due", state.DueSources)
	setCategoryCount(state.Categories, "pending_insights", state.Summary.PendingInsights)
	upstreamSkills, upstreamErr := ListSkillUpstream(ctx, root)
	if upstreamErr == nil {
		count := 0
		for _, sk := range upstreamSkills {
			if sk.Status == "update_available" || sk.Status == "diverged" {
				count++
			}
		}
		state.Summary.UpstreamUpdates = count
	}
	return nil
}

func setCategoryCount(categories []ActionCategory, kind string, count int) {
	for index := range categories {
		if categories[index].Kind == kind {
			categories[index].Count = count
			return
		}
	}
}

func deriveCurationHome(state homeState) CurationHome {
	if state.Categories == nil {
		state.Categories = defaultCategories()
	}
	home := CurationHome{
		Result: NewResult(StatusOK, "Skill Hub is up to date."),
		Workspace: CurationWorkspace{
			Health: state.Health, Index: state.Index, GitDirty: state.GitDirty,
			GitConfigured: state.GitConfigured, RecoveryPending: state.RecoveryPending,
		},
		Actions: []ActionItem{}, Categories: state.Categories, HomeSummary: state.Summary,
		CountsKnown: state.CountsKnown,
	}
	if state.Health == "invalid" {
		home.HomeSummary.InvalidWorkspaces = 1
		home.HomeSummary.AttentionItems++
		reason := state.InvalidReason
		if reason == "" {
			reason = "Canonical data is intact but derived search data is stale."
		}
		home.Status = StatusRecoveryRequired
		home.Summary = "Workspace needs repair before other curation work."
		home.Error = &Error{Code: ErrorWorkspaceInvalid, Render: ErrorRender{
			Error: "Workspace checks found state that needs repair.", Why: reason,
			Fix: "Run skillhub doctor --fix and review its repair plan before confirming changes.",
		}}
		diagnosticSummary := state.DiagnosticSummary
		if diagnosticSummary == "" {
			diagnosticSummary = "Search data does not match canonical files"
		}
		diagnosticImpact := state.DiagnosticImpact
		if diagnosticImpact == "" {
			diagnosticImpact = "Search results may be incomplete."
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "repair_workspace", ID: state.DiagnosticID, Count: 1, Priority: 110, Summary: diagnosticSummary, Command: "workspace_validate"})
		home.Items = append(home.Items, Item{ID: firstNonEmpty(state.DiagnosticID, "workspace_validation"), Summary: diagnosticSummary, Impact: diagnosticImpact})
	}
	if state.RecoveryPending {
		home.HomeSummary.RecoveryItems++
		home.HomeSummary.AttentionItems++
		home.Status = StatusRecoveryRequired
		if state.Health == "valid" {
			home.Summary = "One workspace operation needs recovery before optional work."
			home.Actions = append(home.Actions, ActionItem{Kind: "recover_workspace", ID: state.RecoveryID, Count: 1, Priority: 105, Summary: "An interrupted canonical operation needs recovery", Command: "workspace_validate"})
		} else {
			// Validation repair and journal recovery share one mechanical DoctorFix
			// action, but each condition remains visible as a separate diagnostic.
			for index := range home.Actions {
				if home.Actions[index].Command == "workspace_validate" {
					home.Actions[index].Count++
					break
				}
			}
			home.Items = append(home.Items, Item{ID: firstNonEmpty(state.RecoveryID, "workspace_recovery"), Summary: "An interrupted canonical operation needs recovery", Impact: actionImpact("recover_workspace")})
		}
	}
	if state.Summary.FailedOrInterruptedRuns > 0 {
		home.HomeSummary.RecoveryItems += state.Summary.FailedOrInterruptedRuns
		home.HomeSummary.AttentionItems += state.Summary.FailedOrInterruptedRuns
		home.Status = StatusRecoveryRequired
		if state.Health == "valid" && !state.RecoveryPending {
			home.Summary = "One analysis run needs recovery before optional work."
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "resume_run", ID: state.InterruptedID, Count: state.Summary.FailedOrInterruptedRuns, Priority: 100, Summary: "Analysis stopped before completion", Command: "curation_run_retry"})
	}
	if state.Health == "valid" && state.Index != "current" && state.Index != string(catalog.StateHealthy) {
		home.HomeSummary.AttentionItems++
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "rebuild_index", Count: 1, Priority: 90, Summary: "Search index needs repair", Command: "workspace_rebuild"})
	}
	if state.UnavailableSources > 0 {
		home.HomeSummary.AttentionItems += state.UnavailableSources
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "retry_unavailable_sources", Count: state.UnavailableSources, Priority: 80, Summary: fmt.Sprintf("%d source(s) are temporarily unavailable", state.UnavailableSources), Command: "source_check"})
	}
	if state.Summary.UpstreamUpdates > 0 {
		home.HomeSummary.OptionalItems += state.Summary.UpstreamUpdates
		home.HomeSummary.AttentionItems += state.Summary.UpstreamUpdates
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, ActionItem{
			Kind:     "review_upstream_updates",
			Count:    state.Summary.UpstreamUpdates,
			Priority: 75,
			Summary:  fmt.Sprintf("%d skill(s) have upstream changes to review", state.Summary.UpstreamUpdates),
			Command:  "skill_upstream_status",
		})
	}
	if state.ChangedSources > 0 {
		home.HomeSummary.OptionalItems += state.ChangedSources
		home.HomeSummary.AttentionItems += state.ChangedSources
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "distill_changed_sources", Count: state.ChangedSources, Priority: 70, Summary: fmt.Sprintf("%d source(s) are ready to distill", state.ChangedSources), Command: "curation_run_start"})
	}
	if state.DueSources > 0 {
		home.HomeSummary.OptionalItems += state.DueSources
		home.HomeSummary.AttentionItems += state.DueSources
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "check_due_sources", Count: state.DueSources, Priority: 60, Summary: fmt.Sprintf("%d source(s) are due for an update check", state.DueSources), Command: "source_check"})
	}
	if state.Summary.PendingInsights > 0 {
		home.HomeSummary.OptionalItems += state.Summary.PendingInsights
		home.HomeSummary.AttentionItems += state.Summary.PendingInsights
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		summary := fmt.Sprintf("%d insight(s) need review", state.Summary.PendingInsights)
		if state.Summary.PendingHighValueInsights > 0 {
			summary = fmt.Sprintf("%d insight(s) need review; %d are high-value", state.Summary.PendingInsights, state.Summary.PendingHighValueInsights)
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "review_insights", Count: state.Summary.PendingInsights, Priority: 40, Summary: summary, Command: "inbox_list"})
	}
	if state.GitDirty {
		home.HomeSummary.AttentionItems++
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "review_git_changes", Count: 1, Priority: 10, Summary: "Git has uncommitted canonical changes", Command: "workspace_diff"})
	}
	if state.Health == "valid" && state.CountsKnown && state.TotalSkills == 0 && state.TotalSources == 0 && state.GitDirty {
		// A fresh workspace has no skills in any state and no sources. Replace
		// the bare "review uncommitted changes" suggestion with the onboarding
		// sequence. Once the files are committed, or anything has been added,
		// the workspace is ordinary again, because host connections cannot be
		// observed from the workspace itself.
		home.Actions = append(home.Actions, ActionItem{Kind: "first_run_commit", Count: 1, Priority: 20, Summary: "The new workspace has no skills and is not committed yet", Command: "workspace_diff"})
	}
	sort.SliceStable(home.Actions, func(i, j int) bool {
		if home.Actions[i].Priority == home.Actions[j].Priority {
			return home.Actions[i].Kind < home.Actions[j].Kind
		}
		return home.Actions[i].Priority > home.Actions[j].Priority
	})

	if len(home.Actions) == 0 {
		home.SuggestedActions = []Action{{Label: "Continue normal work", Command: ""}}
		return home
	}
	recommended := home.Actions[0]
	if home.Summary == "Skill Hub is up to date." {
		switch recommended.Kind {
		case "rebuild_index":
			home.Summary = "Search index is stale; skill and source counts are unavailable until it is rebuilt."
		case "review_upstream_updates":
			home.Summary = fmt.Sprintf("%d skill(s) have upstream changes to review.", recommended.Count)
		case "distill_changed_sources":
			home.Summary = fmt.Sprintf("%d source(s) are ready to distill.", recommended.Count)
		case "review_insights":
			home.Summary = fmt.Sprintf("%d insight(s) are waiting for review.", recommended.Count)
		case "review_git_changes":
			home.Summary = "Git has uncommitted canonical changes."
		case "first_run_commit":
			home.Summary = "Workspace is new; commit it, then connect an agent."
		default:
			home.Summary = "Skill Hub needs attention."
		}
	}
	label := recommendationLabel(recommended.Kind)
	home.SuggestedActions = []Action{{Label: label, Command: recommended.Command}}
	if recommended.Kind != "repair_workspace" {
		home.Items = append(home.Items, Item{ID: firstNonEmpty(recommended.ID, recommended.Kind), Summary: recommended.Summary, Impact: actionImpact(recommended.Kind)})
	}
	return home
}

func recommendationLabel(kind string) string {
	return map[string]string{
		"repair_workspace":          "Run skillhub doctor --fix",
		"recover_workspace":         "Run skillhub doctor --fix",
		"resume_run":                "Resume the interrupted run",
		"rebuild_index":             "Run `skillhub rebuild` to refresh the search index",
		"retry_unavailable_sources": "Retry unavailable source checks",
		"review_upstream_updates":   "Review upstream updates with skillhub skill outdated",
		"distill_changed_sources":   "Distill changed sources",
		"check_due_sources":         "Check all due sources",
		"review_insights":           "Review pending insights",
		"review_git_changes":        "Review uncommitted changes",
		"first_run_commit":          "Commit the new workspace, then run `skillhub connect` in your project",
	}[kind]
}

func actionImpact(kind string) string {
	if strings.Contains(kind, "workspace") || kind == "rebuild_index" {
		return "Other curation work should wait until local workspace state is healthy."
	}
	if kind == "resume_run" {
		return "No source progress was saved."
	}
	return "This local work remains pending."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "item"
}
