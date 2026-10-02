package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	distillpkg "github.com/vantt/mcp-skill-hub/internal/distill"
	insightpkg "github.com/vantt/mcp-skill-hub/internal/insight"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// DistillService owns the exact application contracts intended for CLI and future MCP adapters.
type DistillService struct {
	Clock           Clock
	IDs             IDGenerator
	Adapters        map[string]sourcepkg.Adapter
	MutationOptions mutation.Options
	Telemetry       TelemetrySink
}

type DistillPrepareInput struct {
	SourceIDs      []string `json:"source_ids"`
	AllChanged     bool     `json:"all_changed"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
}
type DistillPrepareItem struct {
	SourceID string                      `json:"source_id"`
	Run      *distillpkg.Run             `json:"run,omitempty"`
	Package  *distillpkg.RevisionPackage `json:"revision_package,omitempty"`
	Error    string                      `json:"error,omitempty"`
}
type DistillBatchResult struct {
	Result
	Prepared int                  `json:"prepared"`
	Failed   int                  `json:"failed"`
	Results  []DistillPrepareItem `json:"results"`
}
type FindingSubmission struct {
	StableKey      string                `json:"stable_key"`
	Status         string                `json:"status"`
	What           string                `json:"what"`
	Vocabulary     []string              `json:"vocabulary"`
	Evidence       []distillpkg.Evidence `json:"evidence"`
	SupersedesIDs  []string              `json:"supersedes_ids,omitempty"`
	SupersededByID string                `json:"superseded_by_id,omitempty"`
}
type ComparisonSubmission struct {
	ID             string                                 `json:"id"`
	Subject        string                                 `json:"subject"`
	ObservationIDs []string                               `json:"observation_ids"`
	Verdict        string                                 `json:"verdict"`
	Tradeoffs      string                                 `json:"tradeoffs"`
	BasedOn        map[string]distillpkg.RevisionIdentity `json:"based_on"`
}
type InsightSubmission struct {
	StableKey      string   `json:"stable_key"`
	SkillID        string   `json:"skill_id"`
	Recommendation string   `json:"recommendation"`
	ObservationIDs []string `json:"observation_ids"`
	ComparisonIDs  []string `json:"comparison_ids,omitempty"`
	Category       string   `json:"category"`
	Priority       string   `json:"priority"`
	Rationale      string   `json:"rationale"`
}
type DistillSubmission struct {
	Coverage             []distillpkg.CoverageEntry    `json:"coverage"`
	Findings             []FindingSubmission           `json:"findings"`
	Comparisons          []ComparisonSubmission        `json:"comparisons,omitempty"`
	Insights             []InsightSubmission           `json:"insights,omitempty"`
	OutstandingDecisions []distillpkg.OutstandingIssue `json:"outstanding_decisions,omitempty"`
	Resolution           string                        `json:"resolution,omitempty"`
	IdempotencyKey       string                        `json:"idempotency_key,omitempty"`
}
type DistillRetryInput struct {
	Decision string `json:"decision"`
}
type DistillBatchSubmission struct {
	RunID      string            `json:"run_id"`
	Submission DistillSubmission `json:"submission"`
}
type DistillFinalizeItem struct {
	RunID  string            `json:"run_id"`
	Result *DistillRunResult `json:"result,omitempty"`
	Error  string            `json:"error,omitempty"`
}
type DistillFinalizeBatchResult struct {
	Result
	Finalized int                   `json:"finalized"`
	Failed    int                   `json:"failed"`
	Results   []DistillFinalizeItem `json:"results"`
}
type DistillRunResult struct {
	Result
	Run             distillpkg.Run `json:"run"`
	OperationID     string         `json:"operation_id,omitempty"`
	ChangedPaths    []string       `json:"changed_paths"`
	CatalogSnapshot string         `json:"catalog_snapshot,omitempty"`
	Generation      string         `json:"generation,omitempty"`
}
type DistillQueryResult struct {
	Result
	Runs        []distillpkg.Run         `json:"runs,omitempty"`
	Findings    []distillpkg.Observation `json:"findings,omitempty"`
	Comparisons []distillpkg.Comparison  `json:"comparisons,omitempty"`
	Insights    []distillpkg.Insight     `json:"insights,omitempty"`
}

func (service DistillService) defaults(root string) DistillService {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	if service.IDs == nil {
		service.IDs = RandomIDGenerator{}
	}
	if service.Adapters == nil {
		service.Adapters = (SourceService{}).defaults(root).Adapters
	}
	return service
}

// PrepareDistillRuns isolates each source so one unavailable package does not abort the batch.
func (service DistillService) PrepareDistillRuns(ctx context.Context, path string, input DistillPrepareInput) (DistillBatchResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return DistillBatchResult{}, err
	}
	service = service.defaults(root)
	_, records, err := readSourceRecords(root)
	if err != nil {
		return DistillBatchResult{}, err
	}
	requested := make(map[string]bool, len(input.SourceIDs))
	for _, id := range input.SourceIDs {
		requested[id] = true
	}
	result := DistillBatchResult{Result: NewResult(StatusOK, "Distill preparation completed; curated skills are unchanged."), Results: []DistillPrepareItem{}}
	seen := map[string]bool{}
	for _, record := range records {
		if len(requested) > 0 && !requested[record.ID] {
			continue
		}
		if len(requested) == 0 && input.AllChanged && record.Status != "changed" && record.Status != "distill_pending" && record.DistilledRevision != nil {
			continue
		}
		seen[record.ID] = true
		run, pkg, prepareErr := service.prepareOne(ctx, root, record, input.IdempotencyKey)
		item := DistillPrepareItem{SourceID: record.ID}
		if prepareErr != nil {
			item.Error = sanitizeDistillErrorForSource(prepareErr, record.ID)
			result.Failed++
		} else {
			item.Run, item.Package = &run, &pkg
			result.Prepared++
		}
		result.Results = append(result.Results, item)
	}
	for id := range requested {
		if !seen[id] {
			result.Results = append(result.Results, DistillPrepareItem{SourceID: id, Error: "source not found"})
			result.Failed++
		}
	}
	sort.Slice(result.Results, func(i, j int) bool { return result.Results[i].SourceID < result.Results[j].SourceID })
	if result.Failed > 0 {
		result.Status = StatusPartialFailure
	}
	result.Summary = fmt.Sprintf("Prepared %d source run(s); %d source(s) failed independently. Curated skills are unchanged.", result.Prepared, result.Failed)
	events := make([]telemetry.Event, 0, result.Prepared)
	for _, item := range result.Results {
		summary := "prepared"
		if item.Error != "" {
			summary = item.Error
		}
		result.Items = append(result.Items, Item{ID: item.SourceID, Summary: summary, Impact: "Active skill content was not modified."})
		if item.Run != nil {
			events = append(events, curationTelemetryEvent(telemetry.EventDistillRunPrepared, map[string]any{
				"run_id": item.Run.ID, "status": item.Run.State, "resource_count": len(item.Run.ChangedResources), "retry_count": item.Run.Attempt,
			}))
		}
	}
	recordCurationTelemetry(ctx, service.Telemetry, root, events...)
	return result, nil
}

func (service DistillService) prepareOne(ctx context.Context, root string, record sourcepkg.Record, key string) (distillpkg.Run, distillpkg.RevisionPackage, error) {
	if record.CurrentRevision == nil {
		return distillpkg.Run{}, distillpkg.RevisionPackage{}, errors.New("source has no current revision")
	}
	idempotencyKey := "distill:prepare:" + record.ID + ":" + revisionIntent(*record.CurrentRevision)
	if key != "" {
		idempotencyKey = key + ":" + record.ID
	}
	intentDigest, err := normalizedIntentDigest("distill_prepare", struct {
		SourceID string                       `json:"source_id"`
		From     *distillpkg.RevisionIdentity `json:"from_revision,omitempty"`
		To       distillpkg.RevisionIdentity  `json:"to_revision"`
	}{SourceID: record.ID, From: optionalRevisionIdentity(record.DistilledRevision), To: distillpkg.IdentityOf(*record.CurrentRevision)})
	if err != nil {
		return distillpkg.Run{}, distillpkg.RevisionPackage{}, err
	}
	lookup := mutation.WriteSet{Command: "distill_prepare", IdempotencyKey: idempotencyKey, RequestDigest: intentDigest}
	if receipt, found, lookupErr := mutation.LookupOperation(root, lookup); lookupErr != nil {
		return distillpkg.Run{}, distillpkg.RevisionPackage{}, lookupErr
	} else if found {
		run, _, loadErr := readRunFromReceipt(root, receipt)
		if loadErr != nil {
			return distillpkg.Run{}, distillpkg.RevisionPackage{}, loadErr
		}
		pkg, loadErr := distillpkg.LoadRevisionPackage(root, run.ID)
		return run, pkg, loadErr
	}
	adapter, ok := service.Adapters[record.Adapter]
	if !ok {
		return distillpkg.Run{}, distillpkg.RevisionPackage{}, errors.New("source adapter is not configured")
	}
	src := sourcepkg.Source{ID: record.ID, Locator: record.Locator, Limits: record.Limits}
	operationCtx, cancel := context.WithTimeout(ctx, time.Duration(record.Limits.TimeoutSeconds)*time.Second)
	defer cancel()
	changes := []distillpkg.ChangedResource{}
	if record.DistilledRevision == nil {
		resources, listErr := adapter.List(operationCtx, src, *record.CurrentRevision, sourcepkg.Scope{})
		if listErr != nil {
			return distillpkg.Run{}, distillpkg.RevisionPackage{}, listErr
		}
		for _, item := range resources {
			changes = append(changes, distillpkg.ChangedResource{Path: item.Path, Status: "added"})
		}
	} else {
		delta, diffErr := adapter.Diff(operationCtx, src, *record.DistilledRevision, *record.CurrentRevision)
		if diffErr != nil {
			return distillpkg.Run{}, distillpkg.RevisionPackage{}, diffErr
		}
		if !distillpkg.SameRevision(delta.From, *record.DistilledRevision) || !distillpkg.SameRevision(delta.To, *record.CurrentRevision) {
			return distillpkg.Run{}, distillpkg.RevisionPackage{}, errors.New("adapter diff revision identity does not match requested endpoints")
		}
		for _, item := range delta.Changes {
			changes = append(changes, distillpkg.ChangedResource{Path: item.Path, Status: item.Status})
		}
	}
	random, err := service.IDs.New()
	if err != nil || random == "" {
		return distillpkg.Run{}, distillpkg.RevisionPackage{}, errors.New("run ID generation failed")
	}
	runID := "RUN-" + strings.ToUpper(random[:minInt(16, len(random))])
	now := service.Clock.Now().UTC()
	pkg, err := distillpkg.CreateRevisionPackage(operationCtx, root, adapter, src, runID, record.DistilledRevision, *record.CurrentRevision, changes, now)
	if err != nil {
		return distillpkg.Run{}, distillpkg.RevisionPackage{}, err
	}
	run := distillpkg.Run{SchemaVersion: 1, ID: runID, SourceID: record.ID, State: "prepared", FromRevision: record.DistilledRevision, ToRevision: *record.CurrentRevision, ChangedResources: changes, PackageDigest: pkg.Digest, PreparedAt: now.Format(time.RFC3339Nano), Attempt: 0}
	distillpkg.SortRunCollections(&run)
	contents, _ := distillpkg.Marshal(run)
	set := mutation.WriteSet{Command: "distill_prepare", IdempotencyKey: idempotencyKey, RequestDigest: intentDigest, Changes: []mutation.Change{{Path: runPath(record.ID, run.ID), Contents: contents}}}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		return distillpkg.Run{}, distillpkg.RevisionPackage{}, err
	}
	if _, err = confirmAndPublish(ctx, root, planned); err != nil {
		return distillpkg.Run{}, distillpkg.RevisionPackage{}, err
	}
	return run, pkg, nil
}

func (service DistillService) StartDistillRun(ctx context.Context, path, runID string) (DistillRunResult, error) {
	return service.transitionRun(ctx, path, runID, []string{"prepared"}, "in_progress", "Distill run started; submit structured findings when analysis is complete.")
}
func (service DistillService) RetryDistillRun(ctx context.Context, path, runID string, decisions ...DistillRetryInput) (DistillRunResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return DistillRunResult{}, err
	}
	service = service.defaults(root)
	run, original, err := readRun(root, runID)
	if err != nil {
		return DistillRunResult{}, err
	}
	if run.State != "failed" && run.State != "awaiting_decision" {
		return DistillRunResult{}, fmt.Errorf("run in state %s cannot transition to in_progress", run.State)
	}
	if run.State == "awaiting_decision" {
		if len(decisions) != 1 || strings.TrimSpace(decisions[0].Decision) == "" {
			return DistillRunResult{}, errors.New("awaiting-decision retry requires an explicit decision or correction")
		}
		run.DecisionHistory = append(run.DecisionHistory, distillpkg.DecisionRecord{Decision: strings.TrimSpace(decisions[0].Decision), At: service.Clock.Now().UTC().Format(time.RFC3339Nano)})
	}
	pkg, err := distillpkg.LoadRevisionPackage(root, run.ID)
	if err != nil || pkg.Digest != run.PackageDigest {
		return DistillRunResult{}, errors.New("immutable revision package is unavailable or invalid")
	}
	run.State = "in_progress"
	run.Failure = ""
	run.Attempt++
	run.StartedAt = service.Clock.Now().UTC().Format(time.RFC3339Nano)
	// Blockers and the immutable proposal remain as audit history until a
	// corrected submission explicitly resolves them.
	return service.writeRun(ctx, root, run, original, "distill_in_progress", "Distill run is ready for a corrected submission attempt.")
}
func (service DistillService) CancelDistillRun(ctx context.Context, path, runID string) (DistillRunResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return DistillRunResult{}, err
	}
	service = service.defaults(root)
	run, original, err := readRun(root, runID)
	if err != nil {
		return DistillRunResult{}, err
	}
	if run.State == "finalized" || run.State == "cancelled" {
		return DistillRunResult{}, errors.New("finalized or cancelled run cannot be cancelled")
	}
	run.State = "cancelled"
	run.CancelledAt = service.Clock.Now().UTC().Format(time.RFC3339Nano)
	return service.writeRun(ctx, root, run, original, "distill_cancel", "Distill run cancelled; its source cursor was not advanced.")
}
func (service DistillService) transitionRun(ctx context.Context, path, runID string, allowed []string, target, summary string) (DistillRunResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return DistillRunResult{}, err
	}
	service = service.defaults(root)
	run, original, err := readRun(root, runID)
	if err != nil {
		return DistillRunResult{}, err
	}
	if !containsStringValue(allowed, run.State) {
		return DistillRunResult{}, fmt.Errorf("run in state %s cannot transition to %s", run.State, target)
	}
	pkg, err := distillpkg.LoadRevisionPackage(root, run.ID)
	if err != nil || pkg.Digest != run.PackageDigest {
		return DistillRunResult{}, errors.New("immutable revision package is unavailable or invalid")
	}
	run.State = target
	run.Failure = ""
	run.Attempt++
	run.StartedAt = service.Clock.Now().UTC().Format(time.RFC3339Nano)
	run.OutstandingDecisions = nil
	return service.writeRun(ctx, root, run, original, "distill_"+target, summary)
}
func (service DistillService) writeRun(ctx context.Context, root string, run distillpkg.Run, original []byte, command, summary string) (DistillRunResult, error) {
	return service.writeRunWithIntent(ctx, root, run, original, command, command+":"+run.ID+fmt.Sprintf(":%d", run.Attempt), "", summary)
}

func (service DistillService) writeRunWithIntent(ctx context.Context, root string, run distillpkg.Run, original []byte, command, idempotencyKey, requestDigest, summary string) (DistillRunResult, error) {
	contents, err := distillpkg.Marshal(run)
	if err != nil {
		return DistillRunResult{}, err
	}
	planned, err := mutation.PlanMutation(root, mutation.WriteSet{Command: command, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest, Changes: []mutation.Change{{Path: runPath(run.SourceID, run.ID), BeforeDigest: sourcepkg.Digest(original), Contents: contents}}})
	if err != nil {
		return DistillRunResult{}, err
	}
	receipt, err := confirmAndPublish(ctx, root, planned)
	if err != nil {
		return DistillRunResult{}, err
	}
	return distillRunResult(summary, run, receipt), nil
}

// SubmitDistillRun validates real packaged bytes and auto-finalizes one canonical WriteSet when unblocked.
func (service DistillService) SubmitDistillRun(ctx context.Context, path, runID string, input DistillSubmission) (result DistillRunResult, resultErr error) {
	startedAt, telemetryEligible, artifactEvents := time.Now(), false, []telemetry.Event{}
	root, err := workspace.Discover(path)
	if err != nil {
		return DistillRunResult{}, err
	}
	service = service.defaults(root)
	intentInput := input
	intentInput.IdempotencyKey = ""
	intentDigest, err := normalizedIntentDigest("distill_submit", intentInput)
	if err != nil {
		return DistillRunResult{}, err
	}
	idempotencyKey := firstNonEmpty(input.IdempotencyKey, "distill:submit:"+runID)

	if replayed, found, repErr := checkReplayedDistillRun(root, runID, idempotencyKey, intentDigest); repErr != nil {
		return DistillRunResult{}, repErr
	} else if found {
		return *replayed, nil
	}

	run, runBytes, err := readRun(root, runID)
	if err != nil {
		return DistillRunResult{}, err
	}
	if err := gateDistillRunState(run, input); err != nil {
		return DistillRunResult{}, err
	}

	telemetryEligible = true
	defer func() {
		if telemetryEligible {
			ti := distillTelemetryInput{runID: runID, input: input, initialAttempt: run.Attempt, startedAt: startedAt}
			service.recordDistillSubmissionTelemetry(ctx, root, ti, artifactEvents, result, resultErr)
		}
	}()

	pkg, err := distillpkg.LoadRevisionPackage(root, run.ID)
	if err != nil || pkg.Digest != run.PackageDigest || !distillpkg.SameRevision(pkg.ToRevision, run.ToRevision) {
		return service.failSubmission(ctx, root, run, runBytes, errors.New("immutable target revision package is unavailable or invalid"))
	}

	sc := &distillSubmissionContext{
		root: root, runID: runID, run: run, runBytes: runBytes,
		pkg: pkg, input: input, intentInput: intentInput,
		idempotencyKey: idempotencyKey, intentDigest: intentDigest,
	}
	if awaitingResult, handled, err := service.handleDistillBlockingDecisions(ctx, sc); err != nil {
		return DistillRunResult{}, err
	} else if handled {
		return *awaitingResult, nil
	}

	obs, err := service.validateAndBuildObservations(ctx, sc)
	if err != nil {
		return DistillRunResult{}, err
	}
	cmps, err := service.validateAndBuildComparisons(ctx, sc, obs.all)
	if err != nil {
		return DistillRunResult{}, err
	}
	ins, err := service.validateAndBuildInsights(ctx, sc, obs.all, cmps.all)
	if err != nil {
		return DistillRunResult{}, err
	}
	res, events, err := service.finalizeDistillRun(ctx, sc, obs, cmps, ins)
	if err != nil {
		return DistillRunResult{}, err
	}
	artifactEvents = events
	return res, nil
}

type distillSubmissionContext struct {
	root           string
	runID          string
	run            distillpkg.Run
	runBytes       []byte
	pkg            distillpkg.RevisionPackage
	input          DistillSubmission
	intentInput    DistillSubmission
	idempotencyKey string
	intentDigest   string
}

type distillObservationsResult struct {
	existingBytes map[string][]byte
	existingByID  map[string]distillpkg.Observation
	newByItem     map[string]distillpkg.Observation
	all           map[string]distillpkg.Observation
}

type distillComparisonsResult struct {
	existingBytes map[string][]byte
	newByItem     map[string]distillpkg.Comparison
	all           map[string]distillpkg.Comparison
}

type distillInsightsResult struct {
	existingBytes map[string][]byte
	newByItem     map[string]distillpkg.Insight
}

type distillTelemetryInput struct {
	runID          string
	input          DistillSubmission
	initialAttempt int
	startedAt      time.Time
}

func checkReplayedDistillRun(root, runID, idempotencyKey, intentDigest string) (*DistillRunResult, bool, error) {
	for _, command := range []string{"distill_finalize", "distill_awaiting_decision"} {
		lookup := mutation.WriteSet{Command: command, IdempotencyKey: idempotencyKey, RequestDigest: intentDigest}
		if receipt, found, lookupErr := mutation.LookupOperation(root, lookup); lookupErr != nil {
			return nil, false, lookupErr
		} else if found {
			run, _, loadErr := readRun(root, runID)
			if loadErr != nil {
				return nil, false, loadErr
			}
			var pointer catalog.Pointer
			if pointerBytes, readErr := os.ReadFile(filepath.Join(root, "runtime", "catalog", "current.json")); readErr == nil && json.Unmarshal(pointerBytes, &pointer) == nil && pointer.CatalogSnapshot == receipt.CatalogSnapshot {
				receipt.Generation = pointer.Generation
			}
			res := distillRunResult("Original distill submission result recovered idempotently.", run, receipt)
			return &res, true, nil
		}
	}
	return nil, false, nil
}

func gateDistillRunState(run distillpkg.Run, input DistillSubmission) error {
	if run.State != "in_progress" && run.State != "awaiting_decision" {
		return fmt.Errorf("run in state %s cannot accept a submission", run.State)
	}
	if run.State == "awaiting_decision" && strings.TrimSpace(input.Resolution) == "" {
		return errors.New("correcting an awaiting-decision run requires an explicit resolution")
	}
	return nil
}

func (service DistillService) recordDistillSubmissionTelemetry(ctx context.Context, root string, ti distillTelemetryInput, artifactEvents []telemetry.Event, result DistillRunResult, resultErr error) {
	persisted := result.Run
	if resultErr != nil {
		if loaded, _, loadErr := readRun(root, ti.runID); loadErr == nil {
			persisted = loaded
		}
	}
	events := []telemetry.Event{curationTelemetryEvent(telemetry.EventDistillRunSubmitted, map[string]any{
		"run_id": ti.runID, "status": "submitted", "observation_count": len(ti.input.Findings),
		"coverage_gap_count": countCoverageGaps(ti.input.Coverage), "resource_count": len(ti.input.Coverage),
		"retry_count": ti.initialAttempt, "duration_ms": time.Since(ti.startedAt).Milliseconds(),
	})}
	events = append(events, coverageGapTelemetry(ti.input.Coverage, ti.runID)...)
	if resultErr != nil {
		if persisted.State == "failed" {
			events = append(events, curationTelemetryEvent(telemetry.EventDistillRunFailed, map[string]any{
				"run_id": ti.runID, "status": "failed", "cursor_advanced": false, "retry_count": persisted.Attempt,
				"duration_ms": time.Since(ti.startedAt).Milliseconds(), "error_code": "submission_failed",
			}))
		}
	} else if result.Run.State == "finalized" {
		events = append(events, artifactEvents...)
		events = append(events, curationTelemetryEvent(telemetry.EventDistillRunFinalized, map[string]any{
			"run_id": ti.runID, "status": "finalized", "observation_count": len(result.Run.FindingIDs),
			"coverage_gap_count": countCoverageGaps(ti.input.Coverage), "resource_count": len(result.Run.ChangedResources),
			"cursor_advanced": true, "auto_finalized": true, "retry_count": result.Run.Attempt,
			"duration_ms": time.Since(ti.startedAt).Milliseconds(),
		}))
	}
	recordCurationTelemetry(ctx, service.Telemetry, root, events...)
}

func (service DistillService) handleDistillBlockingDecisions(ctx context.Context, sc *distillSubmissionContext) (*DistillRunResult, bool, error) {
	if err := distillpkg.ValidateCoverage(sc.run.ChangedResources, sc.input.Coverage); err != nil {
		_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, err)
		return nil, false, failErr
	}
	issues := append([]distillpkg.OutstandingIssue(nil), sc.input.OutstandingDecisions...)
	for _, coverage := range sc.input.Coverage {
		if coverage.Blocking {
			issues = append(issues, distillpkg.OutstandingIssue{Kind: "coverage", Resource: coverage.Resource, Question: coverage.Reason})
		}
	}
	for _, issue := range issues {
		if issue.Kind != "ambiguity" && issue.Kind != "coverage" || strings.TrimSpace(issue.Question) == "" {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, NewInvalidRequestError("only explicit blocking ambiguity or coverage decisions may pause a run", "Only submit outstanding decisions of kind ambiguity or coverage with a question."))
			return nil, false, failErr
		}
	}
	if len(issues) > 0 {
		proposal, proposalErr := proposedArtifacts(sc.run, sc.intentInput)
		if proposalErr != nil {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, proposalErr)
			return nil, false, failErr
		}
		sc.run.State = "awaiting_decision"
		sc.run.Coverage = sc.input.Coverage
		sc.run.OutstandingDecisions = issues
		sc.run.ProposedArtifacts = &proposal
		distillpkg.SortRunCollections(&sc.run)
		res, writeErr := service.writeRunWithIntent(ctx, sc.root, sc.run, sc.runBytes, "distill_awaiting_decision", sc.idempotencyKey, sc.intentDigest, "Distill run is awaiting a blocking ambiguity or coverage decision; its cursor was not advanced.")
		if writeErr != nil {
			return nil, false, writeErr
		}
		return &res, true, nil
	}
	return nil, false, nil
}

func (service DistillService) validateAndBuildObservations(ctx context.Context, sc *distillSubmissionContext) (distillObservationsResult, error) {
	observations, existingObservationBytes, err := readObservations(sc.root)
	if err != nil {
		return distillObservationsResult{}, err
	}
	observationByID := map[string]distillpkg.Observation{}
	for _, item := range observations {
		observationByID[item.ID] = item
	}
	newObservations := map[string]distillpkg.Observation{}
	for _, submitted := range sc.input.Findings {
		observation := distillpkg.Observation{SchemaVersion: 1, ID: distillpkg.ObservationID(sc.run.SourceID, submitted.StableKey), SourceID: sc.run.SourceID, RunID: sc.run.ID, StableKey: submitted.StableKey, Status: submitted.Status, FirstSeen: distillpkg.IdentityOf(sc.run.ToRevision), LastSeen: distillpkg.IdentityOf(sc.run.ToRevision), What: submitted.What, Vocabulary: uniqueStrings(submitted.Vocabulary), Evidence: submitted.Evidence, SupersedesIDs: uniqueStrings(submitted.SupersedesIDs), SupersededByID: submitted.SupersededByID}
		if previous, ok := observationByID[observation.ID]; ok {
			if previous.SourceID != sc.run.SourceID || previous.StableKey != observation.StableKey {
				_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, errors.New("observation identity cannot be reused for another concept"))
				return distillObservationsResult{}, failErr
			}
			observation.FirstSeen = previous.FirstSeen
			observation.Evidence = mergeEvidence(previous.Evidence, submitted.Evidence)
		}
		if err := distillpkg.ValidateObservation(observation); err != nil {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, err)
			return distillObservationsResult{}, failErr
		}
		if err := validateEvidence(sc.root, sc.pkg, sc.run, submitted.Status, submitted.Evidence); err != nil {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, err)
			return distillObservationsResult{}, failErr
		}
		if _, duplicate := newObservations[observation.ID]; duplicate {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, NewInvalidRequestError("duplicate submitted observation", "Submit each finding stable key once."))
			return distillObservationsResult{}, failErr
		}
		newObservations[observation.ID] = observation
	}

	changed := map[string]string{}
	for _, item := range sc.run.ChangedResources {
		changed[item.Path] = item.Status
	}
	for _, previous := range observations {
		if previous.SourceID != sc.run.SourceID || previous.Status != "active" {
			continue
		}
		if _, supplied := newObservations[previous.ID]; supplied {
			continue
		}
		for _, priorEvidence := range previous.Evidence {
			status, affected := changed[priorEvidence.Path]
			if !affected {
				continue
			}
			side := "to"
			if status == "deleted" {
				side = "from"
			}
			for _, packaged := range sc.pkg.Resources {
				if packaged.Side == side && packaged.Path == priorEvidence.Path {
					previous.Status = "removed"
					previous.RunID = sc.run.ID
					previous.LastSeen = distillpkg.IdentityOf(sc.run.ToRevision)
					removal := distillpkg.Evidence{Revision: packaged.Revision, RunID: sc.run.ID, PackageDigest: sc.pkg.Digest, Path: priorEvidence.Path, Locator: priorEvidence.Path, Digest: packaged.Digest}
					previous.Evidence = mergeEvidence(previous.Evidence, []distillpkg.Evidence{removal})
					newObservations[previous.ID] = previous
					break
				}
			}
			break
		}
	}
	allObservations := map[string]distillpkg.Observation{}
	for id, item := range observationByID {
		allObservations[id] = item
	}
	for id, item := range newObservations {
		allObservations[id] = item
	}
	for _, item := range newObservations {
		for _, prior := range item.SupersedesIDs {
			previous, ok := allObservations[prior]
			if !ok {
				_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, fmt.Errorf("superseded observation %s does not exist", prior))
				return distillObservationsResult{}, failErr
			}
			if prior == item.ID {
				_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, errors.New("observation cannot supersede itself"))
				return distillObservationsResult{}, failErr
			}
			previous.Status = "superseded"
			previous.SupersededByID = item.ID
			previous.RunID = sc.run.ID
			previous.LastSeen = distillpkg.IdentityOf(sc.run.ToRevision)
			newObservations[prior] = previous
			allObservations[prior] = previous
		}
	}
	return distillObservationsResult{
		existingBytes: existingObservationBytes,
		existingByID:  observationByID,
		newByItem:     newObservations,
		all:           allObservations,
	}, nil
}

func (service DistillService) validateAndBuildComparisons(ctx context.Context, sc *distillSubmissionContext, allObservations map[string]distillpkg.Observation) (distillComparisonsResult, error) {
	comparisons, comparisonBytes, err := readComparisons(sc.root)
	if err != nil {
		return distillComparisonsResult{}, err
	}
	comparisonByID := map[string]distillpkg.Comparison{}
	for _, item := range comparisons {
		comparisonByID[item.ID] = item
	}
	newComparisons := map[string]distillpkg.Comparison{}
	for _, submitted := range sc.input.Comparisons {
		if len(uniqueStrings(submitted.ObservationIDs)) != len(submitted.ObservationIDs) {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, errors.New("comparison observation identities must not contain duplicates"))
			return distillComparisonsResult{}, failErr
		}
		comparison := distillpkg.Comparison{SchemaVersion: 1, ID: submitted.ID, RunID: sc.run.ID, Subject: submitted.Subject, ObservationIDs: append([]string(nil), submitted.ObservationIDs...), Verdict: submitted.Verdict, Tradeoffs: submitted.Tradeoffs, BasedOn: submitted.BasedOn}
		if previous, exists := comparisonByID[comparison.ID]; exists && previous.Subject != comparison.Subject {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, errors.New("comparison stable identity cannot be reused for another subject"))
			return distillComparisonsResult{}, failErr
		}
		if _, duplicate := newComparisons[comparison.ID]; duplicate {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, NewInvalidRequestError("duplicate comparison stable identity", "Submit each comparison once."))
			return distillComparisonsResult{}, failErr
		}
		if err := validateComparison(comparison, allObservations); err != nil {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, err)
			return distillComparisonsResult{}, failErr
		}
		newComparisons[comparison.ID] = comparison
	}
	for _, previous := range comparisons {
		if _, replaced := newComparisons[previous.ID]; replaced {
			continue
		}
		stale := false
		for observationID, revision := range previous.BasedOn {
			if current, ok := allObservations[observationID]; !ok || current.LastSeen != revision || current.Status != "active" {
				stale = true
				break
			}
		}
		if stale != previous.Stale {
			previous.Stale = stale
			previous.RunID = sc.run.ID
			newComparisons[previous.ID] = previous
		}
	}
	allComparisons := map[string]distillpkg.Comparison{}
	for id, item := range comparisonByID {
		allComparisons[id] = item
	}
	for id, item := range newComparisons {
		allComparisons[id] = item
	}
	return distillComparisonsResult{
		existingBytes: comparisonBytes,
		newByItem:     newComparisons,
		all:           allComparisons,
	}, nil
}

func (service DistillService) validateAndBuildInsights(ctx context.Context, sc *distillSubmissionContext, allObservations map[string]distillpkg.Observation, allComparisons map[string]distillpkg.Comparison) (distillInsightsResult, error) {
	existingInsights, existingInsightBytes, err := readInsightsWithBytes(sc.root)
	if err != nil {
		return distillInsightsResult{}, err
	}
	existingInsightByID := make(map[string]distillpkg.Insight, len(existingInsights))
	for _, existing := range existingInsights {
		existingInsightByID[existing.ID] = existing
	}
	newInsights := map[string]distillpkg.Insight{}
	for _, existing := range existingInsights {
		if existing.Status != "pending" {
			continue
		}
		activeSupport := false
		for _, id := range existing.ObservationIDs {
			activeSupport = activeSupport || allObservations[id].Status == "active"
		}
		validComparisons := true
		for _, id := range existing.ComparisonIDs {
			comparison, ok := allComparisons[id]
			validComparisons = validComparisons && ok && !comparison.Stale
		}
		if !activeSupport || !validComparisons {
			existing.Status = "withdrawn"
			existing.RunID = sc.run.ID
			newInsights[existing.ID] = existing
		}
	}
	for _, submitted := range sc.input.Insights {
		insight := distillpkg.Insight{SchemaVersion: 1, ID: distillpkg.InsightID(submitted.SkillID, submitted.StableKey), RunID: sc.run.ID, StableKey: submitted.StableKey, SkillID: submitted.SkillID, Status: "pending", Recommendation: submitted.Recommendation, ObservationIDs: uniqueStrings(submitted.ObservationIDs), ComparisonIDs: uniqueStrings(submitted.ComparisonIDs), Category: submitted.Category, Priority: submitted.Priority, Rationale: submitted.Rationale}
		activeSupport := false
		for _, id := range insight.ObservationIDs {
			observation, ok := allObservations[id]
			if !ok {
				_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, fmt.Errorf("insight observation %s does not exist", id))
				return distillInsightsResult{}, failErr
			}
			if observation.Status == "active" {
				activeSupport = true
			}
		}
		for _, id := range insight.ComparisonIDs {
			comparison, ok := allComparisons[id]
			if !ok {
				_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, fmt.Errorf("insight comparison %s does not exist", id))
				return distillInsightsResult{}, failErr
			}
			if comparison.Stale {
				_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, fmt.Errorf("insight comparison %s is stale", id))
				return distillInsightsResult{}, failErr
			}
		}
		if !activeSupport {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, errors.New("insight requires at least one active observation and cannot rely solely on removed or superseded knowledge"))
			return distillInsightsResult{}, failErr
		}
		insight.EvidenceDigest = distillpkg.InsightEvidenceDigest(insight.ObservationIDs, insight.ComparisonIDs, allObservations, allComparisons)
		if previous, exists := existingInsightByID[insight.ID]; exists {
			if previous.SkillID != insight.SkillID || previous.StableKey != insight.StableKey {
				_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, errors.New("insight stable identity cannot be reused"))
				return distillInsightsResult{}, failErr
			}
			if previous.Status == "rejected" {
				if previous.RejectedEvidenceDigest == insight.EvidenceDigest {
					continue
				}
				insight.Status = "rejected"
				insight.RejectedEvidenceDigest = previous.RejectedEvidenceDigest
				insight.DecisionRationale = previous.DecisionRationale
				insight.DecisionHistory = append([]insightpkg.Decision(nil), previous.DecisionHistory...)
			} else if previous.Status != "pending" && previous.Status != "withdrawn" {
				insight.Status = previous.Status
				insight.DecisionRationale = previous.DecisionRationale
				insight.RejectedEvidenceDigest = previous.RejectedEvidenceDigest
				insight.DecisionHistory = append([]insightpkg.Decision(nil), previous.DecisionHistory...)
			}
		}
		if err := distillpkg.ValidateInsight(insight); err != nil {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, err)
			return distillInsightsResult{}, failErr
		}
		if _, duplicate := newInsights[insight.ID]; duplicate {
			_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, NewInvalidRequestError("duplicate insight stable identity", "Submit each insight stable key once."))
			return distillInsightsResult{}, failErr
		}
		newInsights[insight.ID] = insight
	}
	return distillInsightsResult{
		existingBytes: existingInsightBytes,
		newByItem:     newInsights,
	}, nil
}

func (service DistillService) finalizeDistillRun(ctx context.Context, sc *distillSubmissionContext, obs distillObservationsResult, cmps distillComparisonsResult, ins distillInsightsResult) (DistillRunResult, []telemetry.Event, error) {
	_, records, err := readSourceRecords(sc.root)
	if err != nil {
		return DistillRunResult{}, nil, err
	}
	var record sourcepkg.Record
	found := false
	for _, item := range records {
		if item.ID == sc.run.SourceID {
			record = item
			found = true
			break
		}
	}
	if !found || record.CurrentRevision == nil || !distillpkg.SameRevision(*record.CurrentRevision, sc.run.ToRevision) {
		_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, errors.New("source current revision no longer matches the pinned target"))
		return DistillRunResult{}, nil, failErr
	}
	recordBytes, err := readWorkspaceFile(sc.root, "sources/catalog/"+record.ID+".yaml")
	if err != nil {
		return DistillRunResult{}, nil, err
	}
	record.DistilledRevision = &sc.run.ToRevision
	record.Status = "watching"
	recordAfter, _ := sourcepkg.MarshalCanonical(record)
	sc.run.State = "finalized"
	sc.run.FinalizedAt = service.Clock.Now().UTC().Format(time.RFC3339Nano)
	sc.run.Coverage = sc.input.Coverage
	sc.run.OutstandingDecisions = nil
	sc.run.ProposedArtifacts = nil
	changes := []mutation.Change{{Path: runPath(sc.run.SourceID, sc.run.ID), BeforeDigest: sourcepkg.Digest(sc.runBytes)}, {Path: "sources/catalog/" + record.ID + ".yaml", BeforeDigest: sourcepkg.Digest(recordBytes), Contents: recordAfter}}
	for id, item := range obs.newByItem {
		data, _ := distillpkg.Marshal(item)
		change := mutation.Change{Path: observationPath(item.SourceID, id), Contents: data}
		if before, ok := obs.existingBytes[id]; ok {
			change.BeforeDigest = sourcepkg.Digest(before)
		}
		changes = append(changes, change)
		sc.run.FindingIDs = append(sc.run.FindingIDs, id)
	}
	for id, item := range cmps.newByItem {
		data, _ := distillpkg.Marshal(item)
		change := mutation.Change{Path: comparisonPath(id), Contents: data}
		if before, ok := cmps.existingBytes[id]; ok {
			change.BeforeDigest = sourcepkg.Digest(before)
		}
		changes = append(changes, change)
		sc.run.ComparisonIDs = append(sc.run.ComparisonIDs, id)
	}
	for id, item := range ins.newByItem {
		data, _ := distillpkg.Marshal(item)
		change := mutation.Change{Path: insightPath(item.SkillID, id), Contents: data}
		if before, ok := ins.existingBytes[id]; ok {
			change.BeforeDigest = sourcepkg.Digest(before)
		}
		changes = append(changes, change)
		sc.run.InsightIDs = append(sc.run.InsightIDs, id)
	}
	artifactEvents := distillArtifactTelemetry(sc.run.ID, sc.input, obs.existingByID, obs.newByItem, cmps.newByItem, ins.newByItem)
	distillpkg.SortRunCollections(&sc.run)
	runAfter, _ := distillpkg.Marshal(sc.run)
	changes[0].Contents = runAfter
	set := mutation.WriteSet{Command: "distill_finalize", IdempotencyKey: sc.idempotencyKey, RequestDigest: sc.intentDigest, Changes: changes}
	planned, err := mutation.PlanMutation(sc.root, set)
	if err != nil {
		_, failErr := service.failSubmission(ctx, sc.root, sc.run, sc.runBytes, err)
		return DistillRunResult{}, nil, failErr
	}
	receipt, err := confirmAndPublishWithOptions(ctx, sc.root, planned, service.MutationOptions)
	if err != nil {
		return DistillRunResult{}, nil, err
	}
	return distillRunResult("Distill run finalized atomically; findings and insight proposals were recorded and the source cursor advanced. Active skills are unchanged.", sc.run, receipt), artifactEvents, nil
}

func failureText(cause error) string {
	var appErr *Error
	if errors.As(cause, &appErr) && appErr != nil && appErr.Render.Why != "" {
		return appErr.Render.Why
	}
	if cause == nil {
		return ""
	}
	return cause.Error()
}

func (service DistillService) failSubmission(ctx context.Context, root string, run distillpkg.Run, original []byte, cause error) (DistillRunResult, error) {
	run.State = "failed"
	run.Failure = boundedDistillMessage(failureText(cause), 2000)
	_, writeErr := service.writeRun(ctx, root, run, original, "distill_failed", "Distill submission failed; the source cursor was not advanced.")
	if writeErr != nil {
		return DistillRunResult{}, fmt.Errorf("%v; additionally failed to persist failed run: %w", cause, writeErr)
	}
	return DistillRunResult{}, cause
}

// SubmitDistillRuns finalizes independent runs one at a time. A failed source
// never prevents validation/publication of another source, and an interrupted
// per-source mutation is recovered before the batch proceeds.
func (service DistillService) SubmitDistillRuns(ctx context.Context, path string, submissions []DistillBatchSubmission) (DistillFinalizeBatchResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return DistillFinalizeBatchResult{}, err
	}
	result := DistillFinalizeBatchResult{Result: NewResult(StatusOK, "Distill batch finalized."), Results: []DistillFinalizeItem{}}
	seen := map[string]bool{}
	for _, item := range submissions {
		entry := DistillFinalizeItem{RunID: item.RunID}
		if item.RunID == "" || seen[item.RunID] {
			entry.Error = "run IDs must be non-empty and unique within a batch"
			result.Failed++
			result.Results = append(result.Results, entry)
			continue
		}
		seen[item.RunID] = true
		one, submitErr := service.SubmitDistillRun(ctx, root, item.RunID, item.Submission)
		if submitErr != nil {
			if recoveries, inspectErr := mutation.InspectRecovery(root); inspectErr == nil && len(recoveries) > 0 {
				recoverErr := mutation.RollForwardWithOptions(root, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
					built, buildErr := catalog.BuildCatalogGenerationWhileLocked(ctx, root, expected, catalog.BuildOptions{})
					if buildErr != nil {
						return mutation.Publication{}, buildErr
					}
					return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
				}})
				if recoverErr == nil {
					if recovered, retryErr := service.SubmitDistillRun(ctx, root, item.RunID, item.Submission); retryErr == nil && recovered.Run.State == "finalized" {
						one = recovered
						submitErr = nil
					}
				}
			}
		}
		if submitErr != nil {
			entry.Error = sanitizeDistillError(submitErr)
			result.Failed++
		} else {
			entry.Result = &one
			result.Finalized++
		}
		result.Results = append(result.Results, entry)
	}
	if result.Failed > 0 {
		result.Status = StatusPartialFailure
	}
	result.Summary = fmt.Sprintf("Finalized %d run(s); %d run(s) failed independently.", result.Finalized, result.Failed)
	return result, nil
}

func (DistillService) GetDistillRun(ctx context.Context, path, runID string) (DistillRunResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return DistillRunResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return DistillRunResult{}, err
	}
	run, _, err := readRun(root, runID)
	if err != nil {
		return DistillRunResult{}, err
	}
	return DistillRunResult{Result: NewResult(StatusOK, "Distill run loaded."), Run: run, ChangedPaths: []string{}}, nil
}
func (DistillService) QueryDistill(ctx context.Context, path, kind, sourceID, skillID string) (DistillQueryResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return DistillQueryResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return DistillQueryResult{}, err
	}
	result := DistillQueryResult{Result: NewResult(StatusOK, "Distillation details loaded on demand.")}
	switch kind {
	case "findings":
		items, _, e := readObservations(root)
		if e != nil {
			return result, e
		}
		for _, i := range items {
			if sourceID == "" || i.SourceID == sourceID {
				result.Findings = append(result.Findings, i)
			}
		}
	case "comparisons":
		items, _, e := readComparisons(root)
		if e != nil {
			return result, e
		}
		result.Comparisons = items
	case "insights":
		items, e := readInsights(root)
		if e != nil {
			return result, e
		}
		for _, i := range items {
			if skillID == "" || i.SkillID == skillID {
				result.Insights = append(result.Insights, i)
			}
		}
	default:
		return result, errors.New("query kind must be findings, comparisons, or insights")
	}
	count := len(result.Findings) + len(result.Comparisons) + len(result.Insights)
	result.Summary = fmt.Sprintf("Loaded %d distillation artifact(s) on demand.", count)
	return result, nil
}

func validateEvidence(root string, pkg distillpkg.RevisionPackage, run distillpkg.Run, status string, evidence []distillpkg.Evidence) error {
	for _, e := range evidence {
		if e.Path == "" || e.Locator == "" || e.Digest == "" || e.RunID != run.ID || e.PackageDigest != pkg.Digest || !(e.Locator == e.Path || strings.HasPrefix(e.Locator, e.Path+"#")) {
			return errors.New("evidence requires the current immutable package, a contained locator, and digest")
		}
		expected := distillpkg.IdentityOf(run.ToRevision)
		if status == "removed" {
			if run.FromRevision == nil {
				return errors.New("removed observation evidence requires a from revision")
			}
			expected = distillpkg.IdentityOf(*run.FromRevision)
		}
		if e.Revision != expected {
			return errors.New("evidence revision identity does not match the required pinned revision")
		}
		contents, err := distillpkg.ReadEvidence(root, pkg, e.Revision, e.Path)
		if err != nil {
			return fmt.Errorf("evidence %s does not resolve in the pinned revision package: %w", e.Path, err)
		}
		if sourcepkg.Digest(contents) != e.Digest {
			return fmt.Errorf("evidence digest mismatch for %s", e.Path)
		}
		if err := validateEvidenceLocator(e.Path, e.Locator, contents); err != nil {
			return err
		}
	}
	return nil
}
func validateComparison(item distillpkg.Comparison, observations map[string]distillpkg.Observation) error {
	if err := distillpkg.ValidateComparison(item); err != nil {
		return err
	}
	for _, id := range item.ObservationIDs {
		obs, ok := observations[id]
		if !ok {
			return fmt.Errorf("comparison observation %s does not exist", id)
		}
		if item.BasedOn[id] != obs.LastSeen {
			return fmt.Errorf("comparison %s must exactly pin observation %s revision identity", item.ID, id)
		}
	}
	if item.Stale {
		return errors.New("submitted comparison against exact current observations cannot be marked stale")
	}
	return nil
}

func readRun(root, id string) (distillpkg.Run, []byte, error) {
	if !safeOpaqueRecordID(id) {
		return distillpkg.Run{}, nil, NewInvalidRequestError("invalid run ID", "Pass a run ID returned by curation_run_start.")
	}
	matches, err := filepath.Glob(filepath.Join(root, "distill", "sources", "*", "runs", id+".yaml"))
	if err != nil || len(matches) != 1 {
		return distillpkg.Run{}, nil, errors.New("distill run not found")
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		return distillpkg.Run{}, nil, err
	}
	run, err := distillpkg.ParseRun(data)
	return run, data, err
}
func readObservations(root string) ([]distillpkg.Observation, map[string][]byte, error) {
	var items []distillpkg.Observation
	bytesByID := map[string][]byte{}
	err := walkYAML(root, "distill/sources", func(path string, data []byte) error {
		if !strings.Contains(path, "/observations/") {
			return nil
		}
		item, err := distillpkg.ParseObservation(data)
		if err != nil {
			return err
		}
		items = append(items, item)
		bytesByID[item.ID] = data
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, bytesByID, err
}
func readComparisons(root string) ([]distillpkg.Comparison, map[string][]byte, error) {
	var items []distillpkg.Comparison
	bytesByID := map[string][]byte{}
	err := walkYAML(root, "distill/comparisons", func(_ string, data []byte) error {
		item, err := distillpkg.ParseComparison(data)
		if err != nil {
			return err
		}
		items = append(items, item)
		bytesByID[item.ID] = data
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, bytesByID, err
}
func readInsights(root string) ([]distillpkg.Insight, error) {
	items, _, err := readInsightsWithBytes(root)
	return items, err
}
func readInsightsWithBytes(root string) ([]distillpkg.Insight, map[string][]byte, error) {
	var items []distillpkg.Insight
	bytesByID := map[string][]byte{}
	err := walkYAML(root, "distill/skills", func(path string, data []byte) error {
		if !strings.Contains(path, "/insights/") {
			return nil
		}
		item, err := distillpkg.ParseInsight(data)
		if err != nil {
			return err
		}
		items = append(items, item)
		bytesByID[item.ID] = data
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, bytesByID, err
}
func walkYAML(root, relative string, visit func(string, []byte) error) error {
	base := filepath.Join(root, filepath.FromSlash(relative))
	return filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return visit(filepath.ToSlash(rel), data)
	})
}
func runPath(sourceID, runID string) string {
	return "distill/sources/" + sourceID + "/runs/" + runID + ".yaml"
}
func observationPath(sourceID, id string) string {
	return "distill/sources/" + sourceID + "/observations/" + id + ".yaml"
}
func comparisonPath(id string) string { return "distill/comparisons/" + id + ".yaml" }
func insightPath(skillID, id string) string {
	return "distill/skills/" + skillID + "/insights/" + id + ".yaml"
}

func validateEvidenceLocator(path, locator string, contents []byte) error {
	return distillpkg.ValidateLocator(path, locator, contents)
}

func mergeEvidence(previous, current []distillpkg.Evidence) []distillpkg.Evidence {
	result := append([]distillpkg.Evidence(nil), previous...)
	seen := map[string]bool{}
	for _, item := range result {
		encoded, _ := json.Marshal(item)
		seen[string(encoded)] = true
	}
	for _, item := range current {
		encoded, _ := json.Marshal(item)
		if !seen[string(encoded)] {
			seen[string(encoded)] = true
			result = append(result, item)
		}
	}
	return result
}

func normalizedIntentDigest(command string, value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(command + "\x00"))
	_, _ = hash.Write(encoded)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func optionalRevisionIdentity(value *sourcepkg.Revision) *distillpkg.RevisionIdentity {
	if value == nil {
		return nil
	}
	identity := distillpkg.IdentityOf(*value)
	return &identity
}

func revisionIntent(value sourcepkg.Revision) string {
	identity := distillpkg.IdentityOf(value)
	digest, _ := normalizedIntentDigest("revision", identity)
	return strings.TrimPrefix(digest, "sha256:")
}

func proposedArtifacts(run distillpkg.Run, input DistillSubmission) (distillpkg.ProposedArtifacts, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return distillpkg.ProposedArtifacts{}, err
	}
	proposal := distillpkg.ProposedArtifacts{Digest: sourcepkg.Digest(encoded), Payload: string(encoded)}
	for _, finding := range input.Findings {
		proposal.FindingIDs = append(proposal.FindingIDs, distillpkg.ObservationID(run.SourceID, finding.StableKey))
	}
	for _, comparison := range input.Comparisons {
		proposal.ComparisonIDs = append(proposal.ComparisonIDs, comparison.ID)
	}
	for _, insight := range input.Insights {
		proposal.InsightIDs = append(proposal.InsightIDs, distillpkg.InsightID(insight.SkillID, insight.StableKey))
	}
	sort.Strings(proposal.FindingIDs)
	sort.Strings(proposal.ComparisonIDs)
	sort.Strings(proposal.InsightIDs)
	return proposal, nil
}

func readRunFromReceipt(root string, receipt mutation.Receipt) (distillpkg.Run, []byte, error) {
	for _, path := range receipt.ChangedPaths {
		if strings.Contains(path, "/runs/") && strings.HasSuffix(path, ".yaml") {
			id := strings.TrimSuffix(filepath.Base(path), ".yaml")
			return readRun(root, id)
		}
	}
	return distillpkg.Run{}, nil, errors.New("idempotent distill receipt does not identify a run")
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func containsStringValue(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func boundedDistillMessage(value string, maximum int) string {
	characters := []rune(strings.TrimSpace(value))
	if len(characters) > maximum {
		characters = characters[:maximum]
	}
	return string(characters)
}

func countCoverageGaps(coverage []distillpkg.CoverageEntry) int {
	count := 0
	for _, item := range coverage {
		if coverageGapClassification(item.Status) != "" {
			count++
		}
	}
	return count
}

func coverageGapTelemetry(coverage []distillpkg.CoverageEntry, runID string) []telemetry.Event {
	events := []telemetry.Event{}
	for _, item := range coverage {
		classification := coverageGapClassification(item.Status)
		if classification == "" {
			continue
		}
		events = append(events, curationTelemetryEvent(telemetry.EventCoverageGapRecorded, map[string]any{
			"run_id": runID, "resource_id": telemetryResourceID(item.Resource), "classification": classification,
			"reason_code": classification,
		}))
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Payload["resource_id"].(string) < events[j].Payload["resource_id"].(string)
	})
	return events
}

func coverageGapClassification(status string) string {
	switch status {
	case "deferred", "unreadable", "out_of_scope":
		return status
	case "ruled_out_with_reason":
		return "ruled_out"
	default:
		return ""
	}
}

func telemetryResourceID(resource string) string {
	digest := sha256.Sum256([]byte(resource))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func distillArtifactTelemetry(runID string, input DistillSubmission, existing map[string]distillpkg.Observation, observations map[string]distillpkg.Observation, comparisons map[string]distillpkg.Comparison, insights map[string]distillpkg.Insight) []telemetry.Event {
	events := []telemetry.Event{}
	observationIDs := make([]string, 0, len(observations))
	for id := range observations {
		observationIDs = append(observationIDs, id)
	}
	sort.Strings(observationIDs)
	for _, id := range observationIDs {
		item := observations[id]
		eventType := telemetry.EventObservationCreated
		if item.Status == "removed" || item.Status == "superseded" {
			eventType = telemetry.EventObservationTombstoned
		} else if _, found := existing[id]; found {
			eventType = telemetry.EventObservationUpdated
		}
		events = append(events, curationTelemetryEvent(eventType, map[string]any{
			"entity_id": id, "run_id": runID, "status": item.Status,
		}))
	}
	comparisonIDs := make([]string, 0, len(comparisons))
	for id := range comparisons {
		comparisonIDs = append(comparisonIDs, id)
	}
	sort.Strings(comparisonIDs)
	for _, id := range comparisonIDs {
		status := "current"
		if comparisons[id].Stale {
			status = "stale"
		}
		events = append(events, curationTelemetryEvent(telemetry.EventComparisonUpdated, map[string]any{
			"entity_id": id, "run_id": runID, "status": status,
		}))
	}
	insightIDs := make([]string, 0, len(input.Insights))
	for _, submitted := range input.Insights {
		id := distillpkg.InsightID(submitted.SkillID, submitted.StableKey)
		if _, found := insights[id]; found {
			insightIDs = append(insightIDs, id)
		}
	}
	sort.Strings(insightIDs)
	for _, id := range insightIDs {
		events = append(events, curationTelemetryEvent(telemetry.EventInsightProposed, map[string]any{
			"entity_id": id, "run_id": runID, "status": insights[id].Status,
		}))
	}
	return events
}

func sanitizeDistillError(err error) string {
	return sanitizeDistillErrorForSource(err, "")
}

func sanitizeDistillErrorForSource(err error, sourceID string) string {
	var limitErr *sourcepkg.LimitExceededError
	if errors.As(err, &limitErr) {
		if sourceID != "" {
			return fmt.Sprintf("source exceeded %s limit (%d > %d); narrow scope with `skillhub source triage %s --path <subdir>`", limitErr.Limit, limitErr.Actual, limitErr.Max, sourceID)
		}
		return fmt.Sprintf("source exceeded %s limit (%d > %d); narrow scope with `skillhub source triage <source-id> --path <subdir>`", limitErr.Limit, limitErr.Actual, limitErr.Max)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "source preparation timed out"
	case errors.Is(err, sourcepkg.ErrHistoryUnavailable):
		return "pinned source history is unavailable"
	case errors.Is(err, sourcepkg.ErrLimitExceeded):
		if sourceID != "" {
			return fmt.Sprintf("source exceeded configured limits; narrow scope with `skillhub source triage %s --path <subdir>`", sourceID)
		}
		return "source exceeded configured limits; narrow scope with `skillhub source triage <source-id> --path <subdir>`"
	default:
		return failureText(err)
	}
}
func distillRunResult(summary string, run distillpkg.Run, receipt mutation.Receipt) DistillRunResult {
	return DistillRunResult{Result: NewResult(StatusApplied, summary), Run: run, OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths, CatalogSnapshot: receipt.CatalogSnapshot, Generation: receipt.Generation}
}
func confirmAndPublishWithOptions(ctx context.Context, root string, proposal mutation.Proposal, options mutation.Options) (mutation.Receipt, error) {
	options.PostCanonical = func(expected string) (mutation.Publication, error) {
		built, err := catalog.BuildCatalogGenerationWhileLocked(ctx, root, expected, catalog.BuildOptions{})
		if err != nil {
			return mutation.Publication{}, err
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}
	return mutation.ConfirmMutationWithOptions(root, proposal, mutation.Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseCatalogSnapshot}, options)
}
