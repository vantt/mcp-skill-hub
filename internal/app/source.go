package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// SourceService is the shared boundary for source intake, onboarding, and explicit checks.
type SourceService struct {
	Clock     Clock
	IDs       IDGenerator
	Adapters  map[string]sourcepkg.Adapter
	Telemetry TelemetrySink
}

type SourceCandidateInput struct{ Locator, Reason, IdempotencyKey string }
type SourceCandidateResult struct {
	Result
	Candidate   sourcepkg.Candidate `json:"candidate"`
	OperationID string              `json:"operation_id,omitempty"`
}
type SourceListResult struct {
	Result
	Candidates []sourcepkg.Candidate `json:"candidates"`
	Sources    []sourcepkg.Record    `json:"sources"`
}

type SourceTriageInput struct {
	CandidateID, Decision, DecisionReason, SourceID, Adapter, Ref, SourcePath, License, Trust, Cadence, SkillID, IdempotencyKey string
	MonitoringEnabled                                                                                                           bool
}

type SourceDiff struct {
	Added    []string `json:"added"`
	Modified []string `json:"modified"`
	Deleted  []string `json:"deleted"`
}

type SourceProposal struct {
	Result
	CandidateID  string             `json:"candidate_id"`
	Source       sourcepkg.Record   `json:"source"`
	Link         *sourcepkg.Link    `json:"link,omitempty"`
	Diff         SourceDiff         `json:"diff"`
	Confirmation ConfirmationPolicy `json:"confirmation"`
	planned      mutation.Proposal
	expiresAt    time.Time
}

type SourceMutationResult struct {
	Result
	SourceID        string   `json:"source_id,omitempty"`
	OperationID     string   `json:"operation_id"`
	ChangedPaths    []string `json:"changed_paths"`
	CatalogSnapshot string   `json:"catalog_snapshot"`
	Generation      string   `json:"generation"`
	GitDirty        bool     `json:"git_dirty"`
}

type SourceCheckItem struct {
	SourceID  string              `json:"source_id"`
	Status    string              `json:"status"`
	Revision  *sourcepkg.Revision `json:"revision,omitempty"`
	LatencyMS int64               `json:"latency_ms"`
	Error     string              `json:"error,omitempty"`
}

type SourceCheckResult struct {
	Result
	Checked     int               `json:"checked"`
	Changed     int               `json:"changed"`
	Unchanged   int               `json:"unchanged"`
	Unavailable int               `json:"unavailable"`
	Results     []SourceCheckItem `json:"results"`
}

func (service SourceService) defaults(root string) SourceService {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	if service.IDs == nil {
		service.IDs = RandomIDGenerator{}
	}
	if service.Adapters == nil {
		service.Adapters = map[string]sourcepkg.Adapter{
			"git":            sourcepkg.GitRepositoryAdapter{CacheRoot: filepath.Join(root, "runtime", "sources", "git")},
			"filesystem":     sourcepkg.FilesystemAdapter{Root: root},
			"immutable-http": sourcepkg.HTTPDocumentAdapter{Immutable: true},
			"living-http":    sourcepkg.HTTPDocumentAdapter{},
		}
	}
	return service
}

func (service SourceService) CaptureSourceCandidate(ctx context.Context, path string, input SourceCandidateInput) (SourceCandidateResult, error) {
	startedAt := time.Now()
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceCandidateResult{}, err
	}
	service = service.defaults(root)
	locator, err := normalizeCapturedLocator(input.Locator)
	if err != nil {
		return SourceCandidateResult{}, err
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" || len(reason) > 2000 {
		return SourceCandidateResult{}, errors.New("source reason is required and must not exceed 2000 bytes")
	}
	candidates, _, err := readSourceRecords(root)
	if err != nil {
		return SourceCandidateResult{}, err
	}
	for _, candidate := range candidates {
		if candidate.Locator == locator && candidate.Status != "rejected" {
			result := SourceCandidateResult{Result: NewResult(StatusOK, "Source candidate already exists; returning existing candidate."), Candidate: candidate}
			result.Items = append(result.Items, Item{ID: candidate.ID, Summary: candidate.Locator, Impact: "Intake status: " + candidate.Status})
			return result, nil
		}
	}
	random, err := service.IDs.New()
	if err != nil {
		return SourceCandidateResult{}, err
	}
	if random == "" {
		return SourceCandidateResult{}, errors.New("source candidate ID generator returned an empty value")
	}
	id := "SRCQ-" + strings.ToUpper(random[:minInt(12, len(random))])
	if _, statErr := os.Lstat(filepath.Join(root, "sources", "intake", id+".yaml")); statErr == nil {
		return SourceCandidateResult{}, errors.New("source candidate ID collision; retry capture")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return SourceCandidateResult{}, statErr
	}
	candidate := sourcepkg.Candidate{SchemaVersion: 1, ID: id, Locator: locator, CapturedAt: service.Clock.Now().UTC().Format(time.RFC3339Nano), Reason: reason, Status: "pending"}
	contents, err := sourcepkg.MarshalCanonical(candidate)
	if err != nil {
		return SourceCandidateResult{}, err
	}
	set := mutation.WriteSet{Command: "source_candidate_capture", IdempotencyKey: input.IdempotencyKey, Changes: []mutation.Change{{Path: "sources/intake/" + id + ".yaml", Contents: contents}}}
	proposal, err := mutation.PlanMutation(root, set)
	if err != nil {
		return SourceCandidateResult{}, err
	}
	receipt, err := confirmAndPublish(ctx, root, proposal)
	if err != nil {
		return SourceCandidateResult{}, err
	}
	result := SourceCandidateResult{Result: NewResult(StatusApplied, "Source candidate captured; no network fetch or skill changes were made."), Candidate: candidate, OperationID: receipt.OperationID}
	result.Items = append(result.Items, Item{ID: id, Summary: locator, Impact: "Pending source triage."})
	recordCurationTelemetry(ctx, service.Telemetry, root, curationTelemetryEvent(telemetry.EventSourceCandidateCaptured, map[string]any{
		"candidate_id": candidate.ID, "status": candidate.Status, "duration_ms": time.Since(startedAt).Milliseconds(),
	}))
	return result, nil
}

func (SourceService) ListSources(ctx context.Context, path, status string) (SourceListResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceListResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return SourceListResult{}, err
	}
	candidates, sources, err := readSourceRecords(root)
	if err != nil {
		return SourceListResult{}, err
	}
	if status != "" {
		filtered := candidates[:0]
		for _, item := range candidates {
			if item.Status == status {
				filtered = append(filtered, item)
			}
		}
		candidates = filtered
	}
	result := SourceListResult{Result: NewResult(StatusOK, fmt.Sprintf("%d candidate(s) and %d monitored source(s).", len(candidates), len(sources))), Candidates: candidates, Sources: sources}
	for _, item := range candidates {
		result.Items = append(result.Items, Item{ID: item.ID, Summary: item.Locator, Impact: "Intake status: " + item.Status})
	}
	for _, item := range sources {
		result.Items = append(result.Items, Item{ID: item.ID, Summary: item.Identity.Name, Impact: "Source status: " + item.Status})
	}
	return result, nil
}

func (service SourceService) TriageSourceCandidate(ctx context.Context, path string, input SourceTriageInput) (SourceProposal, SourceMutationResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceProposal{}, SourceMutationResult{}, err
	}
	service = service.defaults(root)
	candidate, candidateBytes, err := findCandidate(root, input.CandidateID)
	if err != nil {
		return SourceProposal{}, SourceMutationResult{}, err
	}
	if candidate.Status != "pending" && candidate.Status != "deferred" {
		return SourceProposal{}, SourceMutationResult{}, errors.New("candidate is not pending or deferred")
	}
	switch input.Decision {
	case "defer", "reject":
		if input.Decision == "reject" && strings.TrimSpace(input.DecisionReason) == "" {
			return SourceProposal{}, SourceMutationResult{}, errors.New("rejection requires --reason")
		}
		candidate.Status = input.Decision + "red"
		if input.Decision == "reject" {
			candidate.Status = "rejected"
		}
		candidate.DecisionReason = strings.TrimSpace(input.DecisionReason)
		contents, marshalErr := sourcepkg.MarshalCanonical(candidate)
		if marshalErr != nil {
			return SourceProposal{}, SourceMutationResult{}, marshalErr
		}
		set := mutation.WriteSet{Command: "source_candidate_triage", IdempotencyKey: input.IdempotencyKey, Changes: []mutation.Change{{Path: "sources/intake/" + candidate.ID + ".yaml", BeforeDigest: sourcepkg.Digest(candidateBytes), Contents: contents}}}
		proposal, planErr := mutation.PlanMutation(root, set)
		if planErr != nil {
			return SourceProposal{}, SourceMutationResult{}, planErr
		}
		receipt, confirmErr := confirmAndPublish(ctx, root, proposal)
		if confirmErr != nil {
			return SourceProposal{}, SourceMutationResult{}, confirmErr
		}
		result := sourceMutationResult("Source candidate decision recorded.", "", receipt)
		recordCurationTelemetry(ctx, service.Telemetry, root, curationTelemetryEvent(telemetry.EventSourceCandidateTriaged, map[string]any{
			"candidate_id": candidate.ID, "status": candidate.Status, "triage": input.Decision,
		}))
		return SourceProposal{}, result, nil
	case "accept":
		proposal, previewErr := service.previewOnboarding(ctx, root, candidate, candidateBytes, input)
		return proposal, SourceMutationResult{}, previewErr
	default:
		return SourceProposal{}, SourceMutationResult{}, errors.New("triage decision must be accept, defer, or reject")
	}
}

func (service SourceService) previewOnboarding(ctx context.Context, root string, candidate sourcepkg.Candidate, candidateBytes []byte, input SourceTriageInput) (SourceProposal, error) {
	if input.SourceID == "" || !safeSourceID(input.SourceID) {
		return SourceProposal{}, errors.New("accept requires a safe --source-id")
	}
	adapterName := input.Adapter
	if adapterName == "" {
		adapterName = inferAdapter(candidate.Locator)
	}
	adapter, ok := service.Adapters[adapterName]
	if !ok {
		return SourceProposal{}, fmt.Errorf("unsupported source adapter %q", adapterName)
	}
	locator := sourceLocator(candidate.Locator, adapterName, input.Ref, input.SourcePath)
	if err := sourcepkg.ValidateLocator(adapterName, locator); err != nil {
		return SourceProposal{}, err
	}
	inspectionContext, cancelInspection := context.WithTimeout(ctx, sourcepkg.DefaultTimeout)
	defer cancelInspection()
	identity, err := adapter.Identify(inspectionContext, locator)
	if err != nil {
		return SourceProposal{}, err
	}
	if locator.Ref == "" {
		locator.Ref = identity.DefaultBranch
	}
	if locator.Path == "" {
		locator.Path = identity.Path
	}
	revision, err := adapter.CurrentRevision(inspectionContext, sourcepkg.Source{ID: input.SourceID, Locator: locator})
	if err != nil {
		return SourceProposal{}, err
	}
	cadence := input.Cadence
	if cadence == "" {
		cadence = "weekly"
	}
	trust := input.Trust
	if trust == "" {
		trust = "community"
	}
	license := input.License
	if license == "" {
		license = identity.License
	}
	record := sourcepkg.Record{SchemaVersion: 1, ID: input.SourceID, Adapter: adapterName, Locator: locator, Status: "watching", Identity: identity, License: license, Trust: sourcepkg.Trust{Source: trust}, Monitoring: sourcepkg.Monitoring{Enabled: input.MonitoringEnabled, Cadence: cadence}, Limits: sourcepkg.Limits{TimeoutSeconds: int(sourcepkg.DefaultTimeout / time.Second), MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize}, CurrentRevision: &revision}
	if !input.MonitoringEnabled && input.Cadence == "" {
		record.Monitoring.Enabled = true
	}
	if _, err := sourcepkg.ParseRecord(mustYAML(record)); err != nil {
		return SourceProposal{}, err
	}
	candidate.Status = "accepted"
	candidate.DecisionReason = strings.TrimSpace(input.DecisionReason)
	candidateAfter, _ := sourcepkg.MarshalCanonical(candidate)
	sourceBytes, _ := sourcepkg.MarshalCanonical(record)
	changes := []mutation.Change{{Path: "sources/intake/" + candidate.ID + ".yaml", BeforeDigest: sourcepkg.Digest(candidateBytes), Contents: candidateAfter}, {Path: "sources/catalog/" + record.ID + ".yaml", Contents: sourceBytes}}
	diff := SourceDiff{Added: []string{"sources/catalog/" + record.ID + ".yaml"}, Modified: []string{"sources/intake/" + candidate.ID + ".yaml"}, Deleted: []string{}}
	var link *sourcepkg.Link
	if input.SkillID != "" {
		linkID := "LINK-" + input.SkillID + "--" + record.ID
		link = &sourcepkg.Link{SchemaVersion: 1, ID: linkID, SkillID: input.SkillID, SourceID: record.ID, Role: "learning-source"}
		linkBytes, marshalErr := sourcepkg.MarshalCanonical(link)
		if marshalErr != nil {
			return SourceProposal{}, marshalErr
		}
		linkPath := "sources/skills/" + linkID + ".yaml"
		changes = append(changes, mutation.Change{Path: linkPath, Contents: linkBytes})
		diff.Added = append(diff.Added, linkPath)
	}
	set := mutation.WriteSet{Command: "source_onboard", IdempotencyKey: input.IdempotencyKey, Changes: changes}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		return SourceProposal{}, err
	}
	pins := ConfirmationPins{ProposalID: planned.ID, ProposalDigest: planned.Digest, BaseVersion: planned.BaseCatalogSnapshot}
	expiresAt := service.Clock.Now().UTC().Add(24 * time.Hour)
	proposal := SourceProposal{Result: NewResult(StatusActionRequired, "Source onboarding is ready for review."), CandidateID: candidate.ID, Source: record, Link: link, Diff: diff, Confirmation: ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: "TriageSourceCandidate", Confirmation: ConfirmationRequirement{Required: true, Mode: "preview-and-approval", Pins: pins}}, planned: planned, expiresAt: expiresAt}
	resources, listErr := adapter.List(inspectionContext, sourcepkg.Source{ID: input.SourceID, Locator: locator, Limits: record.Limits}, revision, sourcepkg.Scope{})
	var limitErr *sourcepkg.LimitExceededError
	if errors.As(listErr, &limitErr) {
		proposal.Warnings = append(proposal.Warnings, Warning{
			Code:    "source_size_warning",
			Summary: fmt.Sprintf("Source size exceeds %s limit (%d > %d); distillation may fail. Consider scoping with `skillhub source triage %s --decision accept --path <subdir>`.", limitErr.Limit, limitErr.Actual, limitErr.Max, candidate.ID),
		})
	} else if listErr == nil {
		var totalBytes int64
		for _, r := range resources {
			totalBytes += r.Size
		}
		if len(resources) > record.Limits.MaxFiles {
			proposal.Warnings = append(proposal.Warnings, Warning{
				Code:    "source_size_warning",
				Summary: fmt.Sprintf("Source size exceeds files limit (%d > %d); distillation may fail. Consider scoping with `skillhub source triage %s --decision accept --path <subdir>`.", len(resources), record.Limits.MaxFiles, candidate.ID),
			})
		} else if totalBytes > record.Limits.MaxBytes {
			proposal.Warnings = append(proposal.Warnings, Warning{
				Code:    "source_size_warning",
				Summary: fmt.Sprintf("Source size exceeds bytes limit (%d > %d); distillation may fail. Consider scoping with `skillhub source triage %s --decision accept --path <subdir>`.", totalBytes, record.Limits.MaxBytes, candidate.ID),
			})
		}
	}
	proposal.Items = append(proposal.Items, Item{ID: record.ID, Summary: fmt.Sprintf("%s via %s at %s", identity.Name, adapterName, revision.Value), Impact: "Watch " + cadence + "; trust " + trust + "; license " + firstNonEmpty(license, "unknown") + "."})
	proposal.SuggestedActions = append(proposal.SuggestedActions, Action{Label: "Confirm the reviewed onboarding proposal", Command: "source confirm", RequiresConfirmation: true})
	if err := storeSourceProposal(root, proposal, service.Clock.Now()); err != nil {
		return SourceProposal{}, err
	}
	return proposal, nil
}

func (service SourceService) LoadSourceProposal(ctx context.Context, path, id string) (SourceProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceProposal{}, err
	}
	if err := ctx.Err(); err != nil {
		return SourceProposal{}, err
	}
	service = service.defaults(root)
	return loadSourceProposal(root, id, service.Clock.Now())
}

func (service SourceService) ConfirmSourceProposal(ctx context.Context, path string, preview SourceProposal, pins ConfirmationPins) (SourceMutationResult, error) {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	if preview.expiresAt.IsZero() || !service.Clock.Now().UTC().Before(preview.expiresAt) {
		result := SourceMutationResult{Result: NewResult(StatusError, "Proposal expired; nothing was applied.")}
		result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The source proposal can no longer be confirmed.", Why: "The reviewed proposal expired.", Fix: "Regenerate and review the proposal."}}
		return result, nil
	}
	if pins != preview.Confirmation.Confirmation.Pins {
		if pins.ProposalDigest != preview.Confirmation.Confirmation.Pins.ProposalDigest {
			result := SourceMutationResult{Result: NewResult(StatusError, "Proposal digest does not match the preview; nothing was applied.")}
			result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The source proposal cannot be confirmed.", Why: "Proposal digest does not match the preview.", Fix: "Pass the exact proposal digest printed by triage, or re-run triage."}}
			return result, nil
		}
		result := SourceMutationResult{Result: NewResult(StatusError, "Proposal is stale; nothing was applied.")}
		result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The source proposal can no longer be confirmed.", Why: "The confirmation pins do not match the reviewed proposal.", Fix: "Load or regenerate the proposal and confirm its exact ID, digest, and base version."}}
		return result, nil
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceMutationResult{}, err
	}
	receipt, err := confirmAndPublish(ctx, root, preview.planned)
	if errors.Is(err, mutation.ErrConflict) {
		result := SourceMutationResult{Result: NewResult(StatusError, "Proposal is stale; nothing was applied.")}
		result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The source proposal can no longer be confirmed.", Why: "Canonical source state changed after preview.", Fix: "Regenerate and review the onboarding proposal."}}
		return result, nil
	}
	if err != nil {
		return SourceMutationResult{}, err
	}
	summary := fmt.Sprintf("Watching %s. First analysis is ready: ask your agent 'distill new sources' or run `skillhub distill prepare %s`.\nWatching does not auto-import skills; accepted insights can create draft skills.", preview.Source.ID, preview.Source.ID)
	result := sourceMutationResult(summary, preview.Source.ID, receipt)
	recordCurationTelemetry(ctx, service.Telemetry, root, curationTelemetryEvent(telemetry.EventSourceCandidateTriaged, map[string]any{
		"candidate_id": preview.CandidateID, "source_id": preview.Source.ID, "status": "accepted", "triage": "accept",
	}))
	return result, nil
}

func (service SourceService) CheckSources(ctx context.Context, path string, ids []string, allDue bool) (SourceCheckResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceCheckResult{}, err
	}
	service = service.defaults(root)
	_, records, err := readSourceRecords(root)
	if err != nil {
		return SourceCheckResult{}, err
	}
	requested := make(map[string]bool, len(ids))
	for _, id := range ids {
		requested[id] = true
	}
	store := sourcepkg.OperationalStore{Root: root}
	now := service.Clock.Now().UTC()
	result := SourceCheckResult{Result: NewResult(StatusOK, "Source checks completed; curated skills are unchanged."), Results: []SourceCheckItem{}}
	operationalWarning := false
	warnOperational := func() {
		if operationalWarning {
			return
		}
		operationalWarning = true
		result.Warnings = append(result.Warnings, Warning{Code: "operational_state_unavailable", Summary: "Volatile source scheduling state could not be read or written; canonical results remain authoritative."})
	}
	for _, record := range records {
		explicit := requested[record.ID]
		if len(requested) > 0 && !explicit {
			continue
		}
		if allDue && len(requested) == 0 {
			if !record.Monitoring.Enabled || record.Monitoring.Cadence == "manual" {
				continue
			}
			state, found, stateErr := store.Get(ctx, record.ID)
			if stateErr != nil {
				warnOperational()
			} else if found && state.NextCheckAt.After(now) {
				continue
			}
		}
		result.Checked++
		started := time.Now()
		adapter, adapterFound := service.Adapters[record.Adapter]
		item := SourceCheckItem{SourceID: record.ID}
		if !adapterFound {
			item.Status, item.Error = "unavailable", "source adapter is not configured"
			result.Unavailable++
			result.Results = append(result.Results, item)
			continue
		}
		checkContext, cancel := context.WithTimeout(ctx, time.Duration(record.Limits.TimeoutSeconds)*time.Second)
		revision, checkErr := adapter.CurrentRevision(checkContext, sourcepkg.Source{ID: record.ID, Locator: record.Locator, Limits: record.Limits})
		cancel()
		item.LatencyMS = time.Since(started).Milliseconds()
		previousState, found, stateErr := store.Get(ctx, record.ID)
		if stateErr != nil {
			warnOperational()
			found = false
		}
		if checkErr != nil {
			retries := 1
			if found {
				retries = previousState.RetryCount + 1
			}
			operational := sourcepkg.CheckState{SourceID: record.ID, LastCheckedAt: now, Latency: time.Duration(item.LatencyMS) * time.Millisecond, RetryCount: retries, Availability: "unavailable", NextCheckAt: now.Add(retryDelay(retries)), LastError: sanitizeOperationalError(checkErr)}
			if err := store.Record(ctx, operational); err != nil {
				warnOperational()
			}
			item.Status, item.Error = "unavailable", operational.LastError
			result.Unavailable++
			result.Results = append(result.Results, item)
			continue
		}
		item.Revision = &revision
		// ContentDigest is the meaningful scoped identity. A Git commit that
		// changes only outside Locator.Path must not create canonical work.
		if record.CurrentRevision != nil && record.CurrentRevision.ContentDigest == revision.ContentDigest {
			if record.DistilledRevision != nil && record.Status != "distill_pending" {
				if err := store.Record(ctx, sourcepkg.CheckState{SourceID: record.ID, LastCheckedAt: now, Latency: time.Duration(item.LatencyMS) * time.Millisecond, Availability: "available", NextCheckAt: nextCheck(now, record.Monitoring.Cadence)}); err != nil {
					warnOperational()
				}
				item.Status = "up_to_date"
				result.Unchanged++
				result.Results = append(result.Results, item)
				continue
			}
			if err := store.Record(ctx, sourcepkg.CheckState{SourceID: record.ID, LastCheckedAt: now, Latency: time.Duration(item.LatencyMS) * time.Millisecond, Availability: "available", NextCheckAt: nextCheck(now, record.Monitoring.Cadence)}); err != nil {
				warnOperational()
			}
			item.Status = "needs_analysis"
			result.Changed++
			result.Results = append(result.Results, item)
			continue
		}
		record.CurrentRevision = &revision
		record.Status = "changed"
		contents, mutationErr := sourcepkg.MarshalCanonical(record)
		if mutationErr == nil {
			var original []byte
			original, mutationErr = readWorkspaceFile(root, "sources/catalog/"+record.ID+".yaml")
			if mutationErr == nil {
				var planned mutation.Proposal
				planned, mutationErr = mutation.PlanMutation(root, mutation.WriteSet{Command: "source_revision_update", IdempotencyKey: "source-check:" + record.ID + ":" + revision.ContentDigest, Changes: []mutation.Change{{Path: "sources/catalog/" + record.ID + ".yaml", BeforeDigest: sourcepkg.Digest(original), Contents: contents}}})
				if mutationErr == nil {
					_, mutationErr = confirmAndPublish(ctx, root, planned)
				}
			}
		}
		if mutationErr != nil {
			// A fetched revision is not operationally successful until both the
			// canonical record and matching catalog generation are published.
			failure := sanitizeOperationalError(mutationErr)
			if err := store.Record(ctx, sourcepkg.CheckState{SourceID: record.ID, LastCheckedAt: now, Latency: time.Duration(item.LatencyMS) * time.Millisecond, RetryCount: 1, Availability: "unavailable", NextCheckAt: now, LastError: failure}); err != nil {
				warnOperational()
			}
			item.Status, item.Error = "unavailable", failure
			result.Unavailable++
			result.Results = append(result.Results, item)
			continue
		}
		if err := store.Record(ctx, sourcepkg.CheckState{SourceID: record.ID, LastCheckedAt: now, Latency: time.Duration(item.LatencyMS) * time.Millisecond, Availability: "available", NextCheckAt: nextCheck(now, record.Monitoring.Cadence)}); err != nil {
			warnOperational()
		}
		item.Status = "changed"
		if record.DistilledRevision == nil {
			item.Status = "needs_analysis"
		}
		result.Changed++
		result.Results = append(result.Results, item)
	}
	if len(requested) > 0 && result.Checked != len(requested) {
		var missingIDs []string
		for _, id := range ids {
			found := false
			for _, record := range records {
				if record.ID == id {
					found = true
					break
				}
			}
			if !found {
				missingIDs = append(missingIDs, id)
			}
		}
		result.Warnings = append(result.Warnings, Warning{Code: "source_not_found", Summary: "One or more requested source IDs were not found."})
		if result.Checked == 0 && len(missingIDs) > 0 {
			result.Status = StatusError
			why := fmt.Sprintf("Source %q was not found.", missingIDs[0])
			if len(missingIDs) > 1 {
				why = fmt.Sprintf("Requested source IDs were not found: %s.", strings.Join(missingIDs, ", "))
			}
			result.Summary = "Source check failed."
			result.Error = NewInvalidRequestError(why, "Run `skillhub source list` to view configured sources.")
			return result, nil
		}
	}
	if result.Unavailable > 0 {
		result.Status = StatusPartialFailure
		result.Summary = fmt.Sprintf("Checked %d source(s); %d changed, %d unchanged, %d unavailable. Curated skills are unchanged.", result.Checked, result.Changed, result.Unchanged, result.Unavailable)
	}
	events := make([]telemetry.Event, 0, len(result.Results))
	for _, item := range result.Results {
		result.Items = append(result.Items, Item{ID: item.SourceID, Summary: "Source check: " + item.Status, Impact: "Curated skills remain unchanged."})
		events = append(events, curationTelemetryEvent(telemetry.EventSourceChecked, map[string]any{
			"source_id": item.SourceID, "status": item.Status, "duration_ms": item.LatencyMS,
		}))
	}
	recordCurationTelemetry(ctx, service.Telemetry, root, events...)
	return result, nil
}

func confirmAndPublish(ctx context.Context, root string, proposal mutation.Proposal) (mutation.Receipt, error) {
	return mutation.ConfirmMutationWithOptions(root, proposal, mutation.Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseCatalogSnapshot}, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
		built, err := catalog.BuildCatalogGenerationWhileLocked(ctx, root, expected, catalog.BuildOptions{})
		if err != nil {
			return mutation.Publication{}, err
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}})
}

func readSourceRecords(root string) ([]sourcepkg.Candidate, []sourcepkg.Record, error) {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, err
	}
	defer handle.Close()
	readDir := func(relative string) ([]fs.DirEntry, error) {
		info, statErr := handle.Lstat(relative)
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, errors.New("unsafe source record directory")
		}
		return fs.ReadDir(handle.FS(), relative)
	}
	candidateEntries, err := readDir("sources/intake")
	if err != nil {
		return nil, nil, err
	}
	sourceEntries, err := readDir("sources/catalog")
	if err != nil {
		return nil, nil, err
	}
	candidates := []sourcepkg.Candidate{}
	records := []sourcepkg.Record{}
	for _, entry := range candidateEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, readErr := readWorkspaceFile(root, "sources/intake/"+entry.Name())
		if readErr != nil {
			return nil, nil, readErr
		}
		item, parseErr := sourcepkg.ParseCandidate(data)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		candidates = append(candidates, item)
	}
	for _, entry := range sourceEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, readErr := readWorkspaceFile(root, "sources/catalog/"+entry.Name())
		if readErr != nil {
			return nil, nil, readErr
		}
		item, parseErr := sourcepkg.ParseRecord(data)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		records = append(records, item)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return candidates, records, nil
}

func findCandidate(root, id string) (sourcepkg.Candidate, []byte, error) {
	if !safeOpaqueRecordID(id) {
		return sourcepkg.Candidate{}, nil, errors.New("invalid source candidate ID")
	}
	data, err := readWorkspaceFile(root, "sources/intake/"+id+".yaml")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sourcepkg.Candidate{}, nil, fmt.Errorf("source candidate %q was not found", id)
		}
		return sourcepkg.Candidate{}, nil, err
	}
	item, err := sourcepkg.ParseCandidate(data)
	return item, data, err
}
func readWorkspaceFile(root, path string) ([]byte, error) {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: file not found: %w", path, os.ErrNotExist)
		}
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("unsafe workspace file")
	}
	return handle.ReadFile(path)
}
func normalizeCapturedLocator(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return "", sourcepkg.ErrInvalidLocator
	}
	if strings.Contains(value, "://") {
		u, err := sourcepkg.ValidateRemoteURL(value, false)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
	if filepath.IsAbs(value) || !safeSourcePath(value) {
		return "", sourcepkg.ErrInvalidLocator
	}
	return filepath.ToSlash(value), nil
}
func safeSourcePath(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.HasPrefix(value, "../") && !strings.Contains(value, `\`)
}
func safeOpaqueRecordID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func safeSourceID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func inferAdapter(locator string) string {
	if !strings.Contains(locator, "://") {
		return "filesystem"
	}
	lower := strings.ToLower(locator)
	if strings.Contains(lower, "github.com/") || strings.HasSuffix(lower, ".git") {
		return "git"
	}
	return "living-http"
}
func sourceLocator(value, adapter, ref, path string) sourcepkg.Locator {
	switch adapter {
	case "git":
		return sourcepkg.Locator{Repository: value, Ref: ref, Path: path}
	case "filesystem":
		return sourcepkg.Locator{Path: value}
	default:
		return sourcepkg.Locator{URL: value, Mode: strings.TrimSuffix(adapter, "-http")}
	}
}
func mustYAML(value any) []byte { data, _ := sourcepkg.MarshalCanonical(value); return data }
func sourceMutationResult(summary, id string, receipt mutation.Receipt) SourceMutationResult {
	return SourceMutationResult{Result: NewResult(StatusApplied, summary), SourceID: id, OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths, CatalogSnapshot: receipt.CatalogSnapshot, Generation: receipt.Generation, GitDirty: receipt.GitDirty}
}
func nextCheck(now time.Time, cadence string) time.Time {
	switch cadence {
	case "daily":
		return now.Add(24 * time.Hour)
	case "weekly":
		return now.Add(7 * 24 * time.Hour)
	default:
		return now.Add(100 * 365 * 24 * time.Hour)
	}
}
func retryDelay(count int) time.Duration {
	if count > 6 {
		count = 6
	}
	return time.Duration(1<<uint(count-1)) * time.Hour
}
func sanitizeOperationalError(err error) string {
	var limitErr *sourcepkg.LimitExceededError
	if errors.As(err, &limitErr) {
		return fmt.Sprintf("source exceeded %s limit (%d > %d)", limitErr.Limit, limitErr.Actual, limitErr.Max)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "source check timed out"
	case errors.Is(err, context.Canceled):
		return "source check was cancelled"
	case errors.Is(err, sourcepkg.ErrUnsafeAddress):
		return "source address is blocked by network policy"
	case errors.Is(err, sourcepkg.ErrLimitExceeded):
		return "source exceeded configured resource limits"
	case errors.Is(err, sourcepkg.ErrInvalidLocator):
		return "source locator violates policy"
	default:
		return "source check failed"
	}
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Runtime proposal persistence preserves the exact reviewed write set, not credentials.
type sourceProposalArtifact struct {
	Version              int `json:"version"`
	CreatedAt, ExpiresAt time.Time
	CandidateID          string
	Source               sourcepkg.Record
	Link                 *sourcepkg.Link
	Diff                 SourceDiff
	Planned              mutation.Proposal
}

func storeSourceProposal(root string, proposal SourceProposal, now time.Time) error {
	if !validSourceProposalID(proposal.planned.ID) {
		return errors.New("invalid proposal ID")
	}
	expiresAt := proposal.expiresAt
	if expiresAt.IsZero() {
		expiresAt = now.UTC().Add(24 * time.Hour)
	}
	artifact := sourceProposalArtifact{1, now.UTC(), expiresAt, proposal.CandidateID, proposal.Source, proposal.Link, proposal.Diff, proposal.planned}
	data, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer handle.Close()
	if err := handle.MkdirAll("runtime/source-proposals", 0o700); err != nil {
		return err
	}
	for _, directory := range []string{"runtime", "runtime/source-proposals"} {
		info, statErr := handle.Lstat(directory)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("unsafe source proposal directory")
		}
	}
	path := "runtime/source-proposals/" + proposal.planned.ID + ".json"
	temporary := path + ".tmp"
	_ = handle.Remove(temporary)
	file, err := handle.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = handle.Remove(temporary)
		return err
	}
	if err := handle.Rename(temporary, path); err != nil {
		_ = handle.Remove(temporary)
		return err
	}
	return nil
}
func loadSourceProposal(root, id string, now time.Time) (SourceProposal, error) {
	if !validSourceProposalID(id) {
		return SourceProposal{}, errors.New("invalid proposal ID")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return SourceProposal{}, err
	}
	defer handle.Close()
	path := "runtime/source-proposals/" + id + ".json"
	info, err := handle.Lstat(path)
	if err != nil {
		return SourceProposal{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return SourceProposal{}, errors.New("unsafe source proposal artifact")
	}
	data, err := handle.ReadFile(path)
	if err != nil {
		return SourceProposal{}, err
	}
	var artifact sourceProposalArtifact
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return SourceProposal{}, err
	}
	if artifact.Version != 1 || artifact.Planned.ID != id || !now.UTC().Before(artifact.ExpiresAt) {
		return SourceProposal{}, errors.New("source proposal is invalid or expired")
	}
	pins := ConfirmationPins{ProposalID: artifact.Planned.ID, ProposalDigest: artifact.Planned.Digest, BaseVersion: artifact.Planned.BaseCatalogSnapshot}
	return SourceProposal{Result: NewResult(StatusActionRequired, "Stored source onboarding proposal is ready for confirmation."), CandidateID: artifact.CandidateID, Source: artifact.Source, Link: artifact.Link, Diff: artifact.Diff, Confirmation: ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: "TriageSourceCandidate", Confirmation: ConfirmationRequirement{Required: true, Mode: "preview-and-approval", Pins: pins}}, planned: artifact.Planned, expiresAt: artifact.ExpiresAt}, nil
}

func validSourceProposalID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
