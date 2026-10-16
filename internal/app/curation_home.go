package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vantt/skillhub-mcp/internal/canonical"
	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/distill"

	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/mutation"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"github.com/vantt/skillhub-mcp/internal/workspace"
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
	// Command is the command or tool to run. For review_lessons it is a
	// command line to run from the hub folder, and it is empty when the
	// hub's distill-lab differs from Skill Hub's copy.
	Command string `json:"command"`
	// Script and File name the script and the file a review_lessons command
	// runs on, relative to the hub folder (HubRelative), so a reader in
	// another folder can build the command from the hub path.
	Script      string `json:"script,omitempty"`
	File        string `json:"file,omitempty"`
	HubRelative bool   `json:"hub_relative,omitempty"`
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
	// Human inventory counts; existing JSON summary meanings stay unchanged.
	DraftSkills int `json:"-"`
	TotalSkills int `json:"-"`
}

// CurationService owns the local/offline Curation Home application query.
type CurationService struct {
	// NoWait makes GetCurationHome return an error wrapping
	// mutation.ErrWorkspaceBusy at once when another process holds the
	// workspace lock, instead of waiting for it. status sets it; the web Home
	// and the MCP hub_status tool keep waiting.
	NoWait bool
	// ProjectDir, when set, lists the checkouts of this hub that the project
	// around it holds. The web Home and the MCP tool leave it empty.
	ProjectDir string
}

// GetCurationHome reads only workspace files, Git metadata, and the local derived catalog.
func (service CurationService) GetCurationHome(ctx context.Context, path string) (CurationHome, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return CurationHome{}, err
	}
	if err := ctx.Err(); err != nil {
		return CurationHome{}, err
	}
	inspectRecovery := mutation.InspectRecovery
	if service.NoWait {
		// Hold one reader lock for the whole read, taken without waiting.
		// The reads below take their own reader locks, which a held reader
		// lock never blocks, so none of them waits for a writer either.
		lock, err := mutation.TryAcquireSharedLock(root)
		if err != nil {
			return CurationHome{}, err
		}
		defer func() { _ = lock.Unlock() }()
		inspectRecovery = mutation.InspectRecoveryWhileLocked
	}
	recoveries, err := inspectRecovery(root)
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
		Submissions:     submissionEntries(root),
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
		if !state.RecoveryPending {
			req, reqErr := hostIntegrationRequest(root)
			if reqErr == nil {
				if missing, _ := hostintegration.CheckMissing(ctx, req); missing != nil {
					state.HostIntegrationFinding = missing
				}
			}
		}
	} else {
		markCatalogCategoriesUnavailable(state.Categories)
		state.CountsKnown = false
	}
	home := deriveCurationHome(state)
	if service.ProjectDir != "" {
		home.addOpenCheckouts(openCheckoutActions(ctx, root, service.ProjectDir))
	}
	return home, nil
}

// addOpenCheckouts adds the checkouts of the surrounding project to the
// actions. They are reminders: they never change the status of the hub.
func (home *CurationHome) addOpenCheckouts(actions []ActionItem) {
	if len(actions) == 0 {
		return
	}
	home.Actions = append(home.Actions, actions...)
	sort.SliceStable(home.Actions, func(i, j int) bool {
		if home.Actions[i].Priority == home.Actions[j].Priority {
			return home.Actions[i].Kind < home.Actions[j].Kind
		}
		return home.Actions[i].Priority > home.Actions[j].Priority
	})
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
	TotalSkills            int
	TotalSources           int
	DraftSkills            int
	CountsKnown            bool
	InterruptedID          string
	ChangedSources         int
	DueSources             int
	UnavailableSources     int
	Categories             []ActionCategory
	HostIntegrationFinding *hostintegration.Finding
	TopCandidateSkillID    string
	TopCandidateSkillPath  string
	// DistillLabIntact is true when the hub's distill-lab files match the
	// current bundle, so a command that runs its script may be printed.
	DistillLabIntact bool
	// Submissions are the skills other machines sent that wait for a person.
	Submissions []Submission
}

func defaultCategories() []ActionCategory {
	return []ActionCategory{
		{Kind: "sources_ready_to_distill", Availability: AvailabilityAvailable},
		{Kind: "candidate_lessons", Availability: AvailabilityAvailable},
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
		{`SELECT count(*) FROM skills WHERE status='draft'`, &state.DraftSkills},
		{`SELECT count(*) FROM skills`, &state.TotalSkills},
		{`SELECT count(*) FROM canonical_entities WHERE kind='source'`, &state.TotalSources},
		{`SELECT count(*) FROM canonical_entities WHERE kind='source' AND COALESCE(json_extract(content_json,'$.status'),'watching') IN ('watching','changed','distill_pending')`, &state.Summary.WatchingSources},
		{`SELECT count(*) FROM provenance WHERE kind='run' AND state IN ('failed','interrupted')`, &state.Summary.FailedOrInterruptedRuns},
		{`SELECT count(*) FROM canonical_entities s WHERE s.kind='source' AND (json_extract(s.content_json,'$.status') IN ('changed', 'distill_pending') OR json_extract(s.content_json,'$.distilled_revision') IS NULL) AND NOT (COALESCE(json_extract(s.content_json,'$.purpose'), '') = 'upstream' AND NOT EXISTS (SELECT 1 FROM canonical_entities l WHERE l.kind='skill_source_link' AND json_extract(l.content_json,'$.source_id') = s.id AND json_extract(l.content_json,'$.role') IN ('learning-source','inspiration')))`, &state.ChangedSources},
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
	candLessons, candHighValue, topSkillID, topSkillPath := countCandidateLessons(root)
	state.Summary.PendingInsights = candLessons
	state.Summary.PendingHighValueInsights = candHighValue
	state.TopCandidateSkillID = topSkillID
	state.TopCandidateSkillPath = topSkillPath
	if candLessons > 0 {
		state.DistillLabIntact = distillLabIntact(root)
	}
	setCategoryCount(state.Categories, "source_unavailable", state.UnavailableSources)
	setCategoryCount(state.Categories, "sources_ready_to_distill", state.ChangedSources)
	setCategoryCount(state.Categories, "sources_due", state.DueSources)
	setCategoryCount(state.Categories, "candidate_lessons", state.Summary.PendingInsights)
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

func countCandidateLessons(root string) (int, int, string, string) {
	totalCandidate := 0
	totalHighValue := 0
	topSkillID := ""
	topSkillPath := ""
	maxCandidates := 0

	matches, err := filepath.Glob(filepath.Join(root, "skills", "*", "*", ".meta", "distill.yaml"))
	if err != nil {
		return 0, 0, "", ""
	}
	for _, match := range matches {
		doc, err := distill.LoadDocument(match)
		if err != nil {
			continue
		}
		skillCandidates := 0
		for _, l := range doc.Lessons {
			if l.Decision.State == "candidate" {
				totalCandidate++
				skillCandidates++
				if l.Score.Relevance >= 3 && l.Score.Impact >= 4 {
					totalHighValue++
				}
			}
		}
		// Only a file under folders named like canonical ids can appear in
		// a command, so the path never needs quoting; others still count.
		skillDir := filepath.Dir(filepath.Dir(match))
		skillID, collectionID := filepath.Base(skillDir), filepath.Base(filepath.Dir(skillDir))
		if skillCandidates > maxCandidates && canonical.ValidID(skillID) && canonical.ValidID(collectionID) {
			maxCandidates = skillCandidates
			topSkillID = skillID
			topSkillPath = "skills/" + collectionID + "/" + skillID + "/.meta/distill.yaml"
		}
	}
	return totalCandidate, totalHighValue, topSkillID, topSkillPath
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
		DraftSkills: state.DraftSkills, TotalSkills: state.TotalSkills,
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
		home.Actions = append(home.Actions, ActionItem{Kind: "retry_unavailable_sources", Count: state.UnavailableSources, Priority: 80, Summary: fmt.Sprintf("%s %s temporarily unavailable", termCount(state.UnavailableSources, "source", "sources"), pluralWord(state.UnavailableSources, "is", "are")), Command: "source_check"})
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
		home.Actions = append(home.Actions, ActionItem{Kind: "distill_changed_sources", Count: state.ChangedSources, Priority: 70, Summary: fmt.Sprintf("%s %s ready to distill", termCount(state.ChangedSources, "source", "sources"), pluralWord(state.ChangedSources, "is", "are")), Command: "curation_run_start"})
	}
	if len(state.Submissions) > 0 {
		home.HomeSummary.AttentionItems += len(state.Submissions)
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, submissionActions(state.Submissions)...)
	}
	if state.DueSources > 0 {
		home.HomeSummary.OptionalItems += state.DueSources
		home.HomeSummary.AttentionItems += state.DueSources
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "check_due_sources", Count: state.DueSources, Priority: 60, Summary: fmt.Sprintf("%s %s due for an update check", termCount(state.DueSources, "source", "sources"), pluralWord(state.DueSources, "is", "are")), Command: "source_check"})
	}
	if state.Summary.PendingInsights > 0 {
		home.HomeSummary.OptionalItems += state.Summary.PendingInsights
		// Candidate lessons are optional work: do not let them alone turn home.Status into action_required
		home.Actions = append(home.Actions, reviewLessonsAction(state))
	}
	if state.GitDirty {
		home.HomeSummary.AttentionItems++
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
		}
		home.Actions = append(home.Actions, ActionItem{Kind: "review_git_changes", Count: 1, Priority: 10, Summary: "The hub has changes that are not committed to Git", Command: "workspace_diff"})
	}
	if state.HostIntegrationFinding != nil {
		home.HomeSummary.AttentionItems++
		if home.Status == StatusOK {
			home.Status = StatusActionRequired
			home.Summary = "Workspace and search index are healthy, but host integration needs attention."
		}
		home.Actions = append(home.Actions, ActionItem{
			Kind:     "host_integration_missing",
			ID:       state.HostIntegrationFinding.ID,
			Count:    1,
			Priority: 85,
			Summary:  state.HostIntegrationFinding.Summary,
			Command:  "doctor",
		})
	}
	if state.Health == "valid" && state.CountsKnown && state.TotalSkills == 0 && state.TotalSources == 0 && state.GitDirty {
		// A fresh workspace has no skills in any state and no sources. Replace
		// the bare "review uncommitted changes" suggestion with the onboarding
		// sequence. Once the files are committed, or anything has been added,
		// the workspace is ordinary again, because host connections cannot be
		// observed from the workspace itself.
		home.Actions = append(home.Actions, ActionItem{Kind: "first_run_commit", Count: 1, Priority: 20, Summary: "The new hub has no skills and is not committed to Git yet", Command: "workspace_diff"})
	}
	sort.SliceStable(home.Actions, func(i, j int) bool {
		if home.Actions[i].Priority == home.Actions[j].Priority {
			return home.Actions[i].Kind < home.Actions[j].Kind
		}
		return home.Actions[i].Priority > home.Actions[j].Priority
	})

	if len(home.Actions) == 0 {
		home.SuggestedActions = []Action{{Label: "List the skills of the hub", Command: "skill_list"}}
		return home
	}
	recommended := home.Actions[0]
	if home.Summary == "Skill Hub is up to date." {
		switch recommended.Kind {
		case "rebuild_index":
			home.Summary = "Search index is stale; skill and source counts are unavailable until it is rebuilt."
		case "host_integration_missing":
			home.Summary = "Host integration is missing; run skillhub doctor --fix after a pull that untracked those files."
		case "review_upstream_updates":
			home.Summary = fmt.Sprintf("%d skill(s) have upstream changes to review.", recommended.Count)
		case "distill_changed_sources":
			home.Summary = fmt.Sprintf("%s %s ready to distill.", termCount(recommended.Count, "source", "sources"), pluralWord(recommended.Count, "is", "are"))
		case "submission_waiting":
			home.Summary = fmt.Sprintf("%s waiting for review.", termCount(len(state.Submissions), "submitted skill", "submitted skills"))
		case "review_lessons":
			home.Summary = fmt.Sprintf("%s %s waiting for review.", termCount(recommended.Count, "candidate lesson", "candidate lessons"), pluralWord(recommended.Count, "is", "are"))
		case "review_git_changes":
			home.Summary = "The hub has changes that are not committed to Git."
		case "first_run_commit":
			home.Summary = "The new hub is not committed to Git yet."
		default:
			home.Summary = "Skill Hub needs attention."
		}
	}
	label := recommendationLabel(recommended.Kind)
	home.SuggestedActions = []Action{{Label: label, Command: recommended.Command}}
	if recommended.Kind != "repair_workspace" {
		home.Items = append(home.Items, Item{ID: firstNonEmpty(recommended.ID, recommended.Kind), Summary: recommended.Summary, Impact: actionImpact(recommended.Kind)})
	}
	if state.HostIntegrationFinding != nil && recommended.Kind != "host_integration_missing" {
		home.Items = append(home.Items, Item{
			ID:      state.HostIntegrationFinding.ID,
			Summary: state.HostIntegrationFinding.Summary,
			Impact:  actionImpact("host_integration_missing"),
		})
	}
	return home
}

func recommendationLabel(kind string) string {
	return map[string]string{
		"repair_workspace":          "Run skillhub doctor --fix",
		"recover_workspace":         "Run skillhub doctor --fix",
		"resume_run":                "Resume the interrupted run",
		"rebuild_index":             "Run `skillhub maintain rebuild` to refresh the search index",
		"host_integration_missing":  "Run skillhub doctor --fix",
		"retry_unavailable_sources": "Retry unavailable source checks",
		"review_upstream_updates":   "Review upstream updates with skillhub skill outdated",
		"distill_changed_sources":   "Distill changed sources",
		"review_lessons":            "Review candidate lessons with distill-lab",
		"submission_waiting":        "Review the submitted skill with skillhub skill review",
		"check_due_sources":         "Check all due sources",
		"review_git_changes":        "Commit the changes of the hub",
		"first_run_commit":          "Commit the new hub, then run `skillhub connect -g`",
	}[kind]
}

func actionImpact(kind string) string {
	if kind == "host_integration_missing" {
		return "Fixable finding: run skillhub doctor --fix after a pull that untracked those files."
	}
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

// maxSubmissionActions is how many waiting submissions status names one by
// one; the rest are counted in a single line.
const maxSubmissionActions = 3

// submissionActions names the skills other machines sent for review: each one
// with the command that reviews it, and the rest as one count.
func submissionActions(submissions []Submission) []ActionItem {
	describe := func(submission Submission) string {
		return fmt.Sprintf("%s from %s, %s", submission.SkillID, submission.ClientLabel, submission.SubmittedAt[:min(10, len(submission.SubmittedAt))])
	}
	command := func(submission Submission) string { return "skillhub skill review " + submission.ID }
	if len(submissions) <= maxSubmissionActions {
		actions := make([]ActionItem, 0, len(submissions))
		for _, submission := range submissions {
			actions = append(actions, ActionItem{Kind: "submission_waiting", ID: submission.ID, Count: 1, Priority: 65,
				Summary: "1 submitted skill waits for review: " + describe(submission), Command: command(submission)})
		}
		return actions
	}
	oldest := submissions[0]
	return []ActionItem{{Kind: "submission_waiting", ID: oldest.ID, Count: len(submissions), Priority: 65,
		Summary: fmt.Sprintf("%d submitted skills wait for review, the oldest is %s", len(submissions), describe(oldest)), Command: command(oldest)}}
}

// reviewLessonsAction points at the skill with the most candidate lessons.
// Its command runs the hub's distill-lab script from the hub folder, and it
// is given only when that script is the one Skill Hub installed.
func reviewLessonsAction(state homeState) ActionItem {
	action := ActionItem{Kind: "review_lessons", ID: state.TopCandidateSkillID, Count: state.Summary.PendingInsights, Priority: 40,
		Summary: termCount(state.Summary.PendingInsights, "candidate lesson", "candidate lessons") + " ready for review with the distill-lab skill in the hub folder"}
	if state.Summary.PendingHighValueInsights > 0 {
		action.Summary += fmt.Sprintf("; %d %s high-value", state.Summary.PendingHighValueInsights, pluralWord(state.Summary.PendingHighValueInsights, "is", "are"))
	}
	if !state.DistillLabIntact {
		action.Summary += "; distill-lab in this hub differs from Skill Hub's copy, run `skillhub doctor`"
		return action
	}
	if state.TopCandidateSkillPath != "" {
		action.Script, action.File, action.HubRelative = distillLabScript, state.TopCandidateSkillPath, true
		action.Command = "node " + distillLabScript + " list " + state.TopCandidateSkillPath
	}
	return action
}
