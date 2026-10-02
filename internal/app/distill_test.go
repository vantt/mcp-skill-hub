package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	distillpkg "github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

type distillClock struct{ value time.Time }

func (clock distillClock) Now() time.Time { return clock.value }

type sequenceDistillID struct{ next int }

func (ids *sequenceDistillID) New() (string, error) {
	ids.next++
	return fmt.Sprintf("distillrun%06d", ids.next), nil
}

type revisionAdapter struct {
	files map[string]map[string][]byte
	fail  map[string]error
}

func (a revisionAdapter) Identify(context.Context, sourcepkg.Locator) (sourcepkg.Identity, error) {
	return sourcepkg.Identity{}, nil
}
func (a revisionAdapter) CurrentRevision(context.Context, sourcepkg.Source) (sourcepkg.Revision, error) {
	return sourcepkg.Revision{}, errors.New("unused")
}
func (a revisionAdapter) Diff(_ context.Context, src sourcepkg.Source, from, to sourcepkg.Revision) (sourcepkg.ChangeSet, error) {
	if err := a.fail[src.ID]; err != nil {
		return sourcepkg.ChangeSet{}, err
	}
	before, after := a.files[from.Value], a.files[to.Value]
	paths := map[string]bool{}
	for p := range before {
		paths[p] = true
	}
	for p := range after {
		paths[p] = true
	}
	names := make([]string, 0, len(paths))
	for p := range paths {
		names = append(names, p)
	}
	sort.Strings(names)
	var changes []sourcepkg.Change
	for _, p := range names {
		status := ""
		old, oldOK := before[p]
		current, newOK := after[p]
		switch {
		case !oldOK:
			status = "added"
		case !newOK:
			status = "deleted"
		case sourcepkg.Digest(old) != sourcepkg.Digest(current):
			status = "modified"
		}
		if status != "" {
			changes = append(changes, sourcepkg.Change{Path: p, Status: status})
		}
	}
	return sourcepkg.ChangeSet{From: from, To: to, Changes: changes}, nil
}
func (a revisionAdapter) Read(_ context.Context, src sourcepkg.Source, rev sourcepkg.Revision, path string) ([]byte, error) {
	if err := a.fail[src.ID]; err != nil {
		return nil, err
	}
	value, ok := a.files[rev.Value][path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), value...), nil
}
func (a revisionAdapter) List(_ context.Context, src sourcepkg.Source, rev sourcepkg.Revision, _ sourcepkg.Scope) ([]sourcepkg.Resource, error) {
	if err := a.fail[src.ID]; err != nil {
		return nil, err
	}
	var result []sourcepkg.Resource
	for p, b := range a.files[rev.Value] {
		result = append(result, sourcepkg.Resource{Path: p, Size: int64(len(b))})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func TestDistillOperationsEmitSanitizedOrderedTelemetry(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	sink := &captureTelemetrySink{}
	service.Telemetry = sink
	run := prepareAndStart(t, root, service, "source-a")
	target := adapter.files["r2"]["SKILL.md"]
	const rawRecommendation = "Private recommendation text must not be recorded."
	result, err := service.SubmitDistillRun(t.Context(), root, run.ID, DistillSubmission{
		Coverage: completeCoverage(),
		Findings: []FindingSubmission{{StableKey: "retry-review", Status: "active", What: "Private observation text.", Vocabulary: []string{"private vocabulary"}, Evidence: []distillpkg.Evidence{evidenceFor(run, "SKILL.md", "SKILL.md#new", target)}}},
		Insights: []InsightSubmission{{StableKey: "retry-review", SkillID: "consumer-review", Recommendation: rawRecommendation, ObservationIDs: []string{"OBS-source-a--retry-review"}, Category: "reliability", Priority: "high", Rationale: "Private rationale."}},
	})
	if err != nil || result.Run.State != "finalized" {
		t.Fatalf("submit = %#v, %v", result, err)
	}
	wantTypes := []string{
		telemetry.EventDistillRunPrepared, telemetry.EventDistillRunSubmitted, telemetry.EventObservationCreated,
		telemetry.EventInsightProposed, telemetry.EventDistillRunFinalized,
	}
	if len(sink.events) != len(wantTypes) {
		t.Fatalf("telemetry events = %#v", sink.events)
	}
	current, err := catalog.OpenCurrent(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	for index, event := range sink.events {
		if event.Version != telemetry.EventVersion || event.Type != wantTypes[index] || event.CatalogSnapshot != current.Pointer.CatalogSnapshot || event.PolicyRevision == "" {
			t.Fatalf("event %d = %#v", index, event)
		}
	}
	encoded, err := json.Marshal(sink.events)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"SKILL.md", "Private observation text.", "private vocabulary", rawRecommendation, "Private rationale."} {
		if bytes.Contains(encoded, []byte(raw)) {
			t.Fatalf("telemetry leaked raw evidence %q: %s", raw, encoded)
		}
	}
}

func TestDistillTelemetryPanicDoesNotChangeFinalization(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	service.Telemetry = panickingTelemetrySink{}
	run := prepareAndStart(t, root, service, "source-a")
	result, err := service.SubmitDistillRun(t.Context(), root, run.ID, validSubmission(run, adapter))
	if err != nil || result.Run.State != "finalized" || result.CatalogSnapshot == "" {
		t.Fatalf("finalize with panicking telemetry = %#v, %v", result, err)
	}
}

func TestDistillFinalizesArtifactsAndCursorAtomicallyWithoutEditingSkill(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	beforeSkill, err := os.ReadFile(filepath.Join(root, "skills/software/consumer-review/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	run := prepareAndStart(t, root, service, "source-a")
	target := adapter.files["r2"]["SKILL.md"]
	result, err := service.SubmitDistillRun(context.Background(), root, run.ID, DistillSubmission{
		Coverage: []distillpkg.CoverageEntry{{Resource: "SKILL.md", Status: "analyzed", Reason: "Read the complete target resource."}, {Resource: "removed.md", Status: "analyzed", Reason: "Confirmed removal against the base package."}},
		Findings: []FindingSubmission{{StableKey: "retry-review", Status: "active", What: "The source reviews retries.", Vocabulary: []string{"retry storm"}, Evidence: []distillpkg.Evidence{evidenceFor(run, "SKILL.md", "SKILL.md#new", target)}}},
		Insights: []InsightSubmission{{StableKey: "retry-review", SkillID: "consumer-review", Recommendation: "Add retry review.", ObservationIDs: []string{"OBS-source-a--retry-review"}, Category: "reliability", Priority: "high", Rationale: "Pinned source evidence adds a missing check."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.State != "finalized" || result.OperationID == "" {
		t.Fatalf("result = %#v", result)
	}
	afterSkill, _ := os.ReadFile(filepath.Join(root, "skills/software/consumer-review/SKILL.md"))
	if !reflect.DeepEqual(beforeSkill, afterSkill) {
		t.Fatal("distillation modified active skill content")
	}
	_, records, err := readSourceRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	if records[0].DistilledRevision == nil || records[0].DistilledRevision.Value != "r2" {
		t.Fatalf("cursor = %#v", records[0].DistilledRevision)
	}
	query, err := (DistillService{}).QueryDistill(context.Background(), root, "findings", "source-a", "")
	if err != nil || len(query.Findings) != 1 {
		t.Fatalf("findings = %#v, %v", query, err)
	}
	insights, err := (DistillService{}).QueryDistill(context.Background(), root, "insights", "", "consumer-review")
	if err != nil || len(insights.Insights) != 1 {
		t.Fatalf("insights = %#v, %v", insights, err)
	}
}

func TestDistillRejectsEvidenceNotPinnedToTargetAndDoesNotAdvanceCursor(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	sink := &captureTelemetrySink{}
	service.Telemetry = sink
	run := prepareAndStart(t, root, service, "source-a")
	_, err := service.SubmitDistillRun(context.Background(), root, run.ID, DistillSubmission{Coverage: completeCoverage(), Findings: []FindingSubmission{{StableKey: "wrong-pin", Status: "active", What: "Claim.", Vocabulary: []string{"claim"}, Evidence: []distillpkg.Evidence{{Revision: distillpkg.IdentityOf(*run.FromRevision), RunID: run.ID, PackageDigest: run.PackageDigest, Path: "SKILL.md", Locator: "SKILL.md", Digest: sourcepkg.Digest(adapter.files["r1"]["SKILL.md"])}}}}})
	if err == nil {
		t.Fatal("wrong revision evidence was accepted")
	}
	loaded, err := (DistillService{}).GetDistillRun(context.Background(), root, run.ID)
	if err != nil || loaded.Run.State != "failed" {
		t.Fatalf("run = %#v, %v", loaded, err)
	}
	_, records, _ := readSourceRecords(root)
	if records[0].DistilledRevision == nil || records[0].DistilledRevision.Value != "r1" {
		t.Fatal("failed run advanced cursor")
	}
	wantTypes := []string{telemetry.EventDistillRunPrepared, telemetry.EventDistillRunSubmitted, telemetry.EventDistillRunFailed}
	if len(sink.events) != len(wantTypes) {
		t.Fatalf("failed telemetry events = %#v", sink.events)
	}
	for index, event := range sink.events {
		if event.Type != wantTypes[index] {
			t.Fatalf("failed telemetry event %d = %#v", index, event)
		}
	}
	if advanced, _ := sink.events[len(sink.events)-1].Payload["cursor_advanced"].(bool); advanced {
		t.Fatal("failed telemetry claimed cursor advancement")
	}
}

func TestDistillAwaitingDecisionOnlyForBlockingCoverageAndRetryCancel(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	sink := &captureTelemetrySink{}
	service.Telemetry = sink
	run := prepareAndStart(t, root, service, "source-a")
	proposal := validSubmission(run, adapter)
	proposal.Coverage = []distillpkg.CoverageEntry{{Resource: "SKILL.md", Status: "deferred", Reason: "Needs policy decision.", Blocking: true}, {Resource: "removed.md", Status: "analyzed", Reason: "Removal checked."}}
	result, err := service.SubmitDistillRun(context.Background(), root, run.ID, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.State != "awaiting_decision" || result.Run.ProposedArtifacts == nil || len(result.Run.OutstandingDecisions) == 0 {
		t.Fatalf("awaiting proposal was not durably preserved: %#v", result.Run)
	}
	wantTelemetry := []string{telemetry.EventDistillRunPrepared, telemetry.EventDistillRunSubmitted, telemetry.EventCoverageGapRecorded}
	if len(sink.events) != len(wantTelemetry) {
		t.Fatalf("awaiting telemetry = %#v", sink.events)
	}
	for index, event := range sink.events {
		if event.Type != wantTelemetry[index] {
			t.Fatalf("awaiting telemetry event %d = %#v", index, event)
		}
	}
	if _, leakedPath := sink.events[2].Payload["resource"]; leakedPath {
		t.Fatalf("coverage telemetry persisted a resource path: %#v", sink.events[2])
	}
	if _, err := service.RetryDistillRun(context.Background(), root, run.ID); err == nil {
		t.Fatal("awaiting-decision retry without an explicit decision was accepted")
	}
	retried, err := service.RetryDistillRun(context.Background(), root, run.ID, DistillRetryInput{Decision: "Defer this resource after policy review."})
	if err != nil || retried.Run.State != "in_progress" || retried.Run.ProposedArtifacts == nil || len(retried.Run.OutstandingDecisions) == 0 || len(retried.Run.DecisionHistory) != 1 {
		t.Fatalf("retry did not preserve proposal/blocker audit: %#v %v", retried, err)
	}
	cancelled, err := service.CancelDistillRun(context.Background(), root, run.ID)
	if err != nil || cancelled.Run.State != "cancelled" {
		t.Fatalf("cancel = %#v %v", cancelled, err)
	}
	_, records, _ := readSourceRecords(root)
	if records[0].DistilledRevision.Value != "r1" {
		t.Fatal("awaiting/cancelled run advanced cursor")
	}
}

func TestDistillGeneratesRemovalTombstoneAndMarksComparisonStale(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	sink := &captureTelemetrySink{}
	service.Telemetry = sink
	run := prepareAndStart(t, root, service, "source-a")
	fromIdentity := distillpkg.IdentityOf(*run.FromRevision)
	observation := distillpkg.Observation{SchemaVersion: 1, ID: "OBS-source-a--removed-knowledge", SourceID: "source-a", RunID: run.ID, StableKey: "removed-knowledge", Status: "active", FirstSeen: fromIdentity, LastSeen: fromIdentity, What: "The source contained old knowledge.", Vocabulary: []string{"old knowledge"}, Evidence: []distillpkg.Evidence{{Revision: fromIdentity, RunID: run.ID, PackageDigest: run.PackageDigest, Path: "removed.md", Locator: "removed.md", Digest: sourcepkg.Digest(adapter.files["r1"]["removed.md"])}}}
	comparison := distillpkg.Comparison{SchemaVersion: 1, ID: "CMP-removed-knowledge", RunID: run.ID, Subject: "old knowledge", ObservationIDs: []string{observation.ID}, Verdict: "convergent", Tradeoffs: "none", BasedOn: map[string]distillpkg.RevisionIdentity{observation.ID: fromIdentity}}
	insight := distillpkg.Insight{SchemaVersion: 1, ID: "INS-consumer-review--old-knowledge", RunID: run.ID, StableKey: "old-knowledge", SkillID: "consumer-review", Status: "pending", Recommendation: "Adopt old knowledge.", ObservationIDs: []string{observation.ID}, ComparisonIDs: []string{comparison.ID}, Category: "quality", Priority: "medium", Rationale: "Previously supported."}
	observationBytes, _ := distillpkg.Marshal(observation)
	comparisonBytes, _ := distillpkg.Marshal(comparison)
	insightBytes, _ := distillpkg.Marshal(insight)
	if err := os.MkdirAll(filepath.Join(root, "distill/sources/source-a/observations"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "distill/sources/source-a/observations", observation.ID+".yaml"), observationBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "distill/comparisons", comparison.ID+".yaml"), comparisonBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "distill/skills/consumer-review/insights"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "distill/skills/consumer-review/insights", insight.ID+".yaml"), insightBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitDistillRun(context.Background(), root, run.ID, DistillSubmission{Coverage: completeCoverage(), Findings: []FindingSubmission{}}); err != nil {
		t.Fatal(err)
	}
	findings, err := service.QueryDistill(context.Background(), root, "findings", "source-a", "")
	if err != nil || len(findings.Findings) != 1 || findings.Findings[0].Status != "removed" || findings.Findings[0].Evidence[0].Revision.Value != "r1" {
		t.Fatalf("tombstone = %#v, %v", findings, err)
	}
	comparisons, err := service.QueryDistill(context.Background(), root, "comparisons", "", "")
	if err != nil || len(comparisons.Comparisons) != 1 || !comparisons.Comparisons[0].Stale {
		t.Fatalf("comparison = %#v, %v", comparisons, err)
	}
	insights, err := service.QueryDistill(context.Background(), root, "insights", "", "consumer-review")
	if err != nil || len(insights.Insights) != 1 || insights.Insights[0].Status != "withdrawn" {
		t.Fatalf("unsupported insight was not withdrawn = %#v, %v", insights, err)
	}
	wantTelemetry := []string{
		telemetry.EventDistillRunPrepared, telemetry.EventDistillRunSubmitted, telemetry.EventObservationTombstoned,
		telemetry.EventComparisonUpdated, telemetry.EventDistillRunFinalized,
	}
	if len(sink.events) != len(wantTelemetry) {
		t.Fatalf("tombstone telemetry = %#v", sink.events)
	}
	for index, event := range sink.events {
		if event.Type != wantTelemetry[index] {
			t.Fatalf("tombstone telemetry event %d = %#v", index, event)
		}
	}
}

func TestDistillPrepareAndSubmitRecoverOriginalResultsAfterLostResponses(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	prepared, err := service.PrepareDistillRuns(context.Background(), root, DistillPrepareInput{SourceIDs: []string{"source-a"}, IdempotencyKey: "prepare-lost-response"})
	if err != nil || prepared.Prepared != 1 {
		t.Fatalf("prepare: %#v %v", prepared, err)
	}
	originalRun := *prepared.Results[0].Run
	adapter.fail["source-a"] = errors.New("adapter must not be called during idempotent lookup")
	recoveredPrepare, err := service.PrepareDistillRuns(context.Background(), root, DistillPrepareInput{SourceIDs: []string{"source-a"}, IdempotencyKey: "prepare-lost-response"})
	if err != nil || recoveredPrepare.Prepared != 1 || recoveredPrepare.Results[0].Run.ID != originalRun.ID || recoveredPrepare.Results[0].Package.Digest != prepared.Results[0].Package.Digest {
		t.Fatalf("prepare recovery = %#v %v", recoveredPrepare, err)
	}
	delete(adapter.fail, "source-a")
	started, err := service.StartDistillRun(context.Background(), root, originalRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	submission := validSubmission(started.Run, adapter)
	submission.IdempotencyKey = "submit-lost-response"
	original, err := service.SubmitDistillRun(context.Background(), root, originalRun.ID, submission)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := service.SubmitDistillRun(context.Background(), root, originalRun.ID, submission)
	if err != nil || recovered.OperationID != original.OperationID || recovered.Run.ID != original.Run.ID || recovered.Run.State != "finalized" || recovered.Generation != original.Generation || recovered.CatalogSnapshot != original.CatalogSnapshot || !reflect.DeepEqual(recovered.ChangedPaths, original.ChangedPaths) {
		t.Fatalf("submit recovery = %#v, original %#v, err %v", recovered, original, err)
	}
}

func TestDistillBatchIsolatesSourcePreparationFailure(t *testing.T) {
	t.Parallel()
	root, service, _ := newDistillWorkspace(t, "source-a", true)
	result, err := service.PrepareDistillRuns(context.Background(), root, DistillPrepareInput{SourceIDs: []string{"source-a", "source-b"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusPartialFailure || result.Prepared != 1 || result.Failed != 1 {
		t.Fatalf("batch = %#v", result)
	}
}

func TestCatalogRebuildRejectsExternallyEditedInvalidFinalizedRunState(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	run := prepareAndStart(t, root, service, "source-a")
	if _, err := service.SubmitDistillRun(context.Background(), root, run.ID, validSubmission(run, adapter)); err != nil {
		t.Fatal(err)
	}
	pointerPath := filepath.Join(root, "runtime/catalog/current.json")
	before, err := os.ReadFile(pointerPath)
	if err != nil {
		t.Fatal(err)
	}
	_, records, err := readSourceRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	records[0].DistilledRevision.Kind = "content-digest"
	data, err := sourcepkg.MarshalCanonical(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources/catalog/source-a.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err == nil {
		t.Fatal("invalid cross-entity state was published")
	}
	after, err := os.ReadFile(pointerPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("catalog pointer changed after invalid rebuild")
	}
}

func TestDistillBatchFinalizeIsolatesAndRecoversEachSource(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", true)
	delete(adapter.fail, "source-b")
	service.IDs = &sequenceDistillID{}
	prepared, err := service.PrepareDistillRuns(context.Background(), root, DistillPrepareInput{SourceIDs: []string{"source-a", "source-b"}})
	if err != nil || prepared.Prepared != 2 {
		t.Fatalf("prepare batch = %#v %v", prepared, err)
	}
	runs := map[string]distillpkg.Run{}
	for _, item := range prepared.Results {
		started, startErr := service.StartDistillRun(context.Background(), root, item.Run.ID)
		if startErr != nil {
			t.Fatal(startErr)
		}
		runs[item.SourceID] = started.Run
	}
	injected := false
	service.MutationOptions.Fault = func(point mutation.FaultPoint) error {
		if point == mutation.FaultFirstCanonicalReplace && !injected {
			injected = true
			return errors.New("one source interrupted")
		}
		return nil
	}
	batch, err := service.SubmitDistillRuns(context.Background(), root, []DistillBatchSubmission{
		{RunID: runs["source-a"].ID, Submission: validSubmission(runs["source-a"], adapter)},
		{RunID: runs["source-b"].ID, Submission: validSubmission(runs["source-b"], adapter)},
	})
	if err != nil || batch.Finalized != 2 || batch.Failed != 0 {
		t.Fatalf("finalize batch = %#v %v", batch, err)
	}
	for _, run := range runs {
		loaded, getErr := service.GetDistillRun(context.Background(), root, run.ID)
		if getErr != nil || loaded.Run.State != "finalized" {
			t.Fatalf("run %s = %#v %v", run.ID, loaded, getErr)
		}
	}
}

func TestDistillCrashRecoveryKeepsCursorAndArtifactsTogether(t *testing.T) {
	t.Parallel()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	run := prepareAndStart(t, root, service, "source-a")
	injected := errors.New("crash after first canonical replacement")
	service.MutationOptions.Fault = func(point mutation.FaultPoint) error {
		if point == mutation.FaultFirstCanonicalReplace {
			return injected
		}
		return nil
	}
	_, err := service.SubmitDistillRun(context.Background(), root, run.ID, validSubmission(run, adapter))
	if !errors.Is(err, injected) {
		t.Fatalf("submit error = %v", err)
	}
	recoveries, err := mutation.InspectRecovery(root)
	if err != nil || len(recoveries) != 1 {
		t.Fatalf("recoveries = %#v %v", recoveries, err)
	}
	err = mutation.RollForwardWithOptions(root, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
		built, buildErr := catalog.BuildCatalogGenerationWhileLocked(context.Background(), root, expected, catalog.BuildOptions{})
		if buildErr != nil {
			return mutation.Publication{}, buildErr
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := (DistillService{}).GetDistillRun(context.Background(), root, run.ID)
	if err != nil || loaded.Run.State != "finalized" {
		t.Fatalf("recovered run = %#v %v", loaded, err)
	}
	_, records, _ := readSourceRecords(root)
	if records[0].DistilledRevision.Value != "r2" {
		t.Fatal("recovered finalize did not advance cursor")
	}
}

func newDistillWorkspace(t *testing.T, sourceID string, second bool) (string, DistillService, revisionAdapter) {
	t.Helper()
	root := newSkillWorkspace(t)
	createAndActivateSkill(t, SkillService{}, root)
	adapter := revisionAdapter{files: map[string]map[string][]byte{"r1": {"SKILL.md": []byte("# Old\n"), "removed.md": []byte("old knowledge\n")}, "r2": {"SKILL.md": []byte("# New\nretry review\n")}}, fail: map[string]error{}}
	writeDistillSource(t, root, sourceID, revisionForDistill("r1", adapter.files["r1"]), revisionForDistill("r2", adapter.files["r2"]))
	if second {
		writeDistillSource(t, root, "source-b", revisionForDistill("r1", adapter.files["r1"]), revisionForDistill("r2", adapter.files["r2"]))
		adapter.fail["source-b"] = sourcepkg.ErrHistoryUnavailable
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	service := DistillService{Clock: distillClock{value: time.Date(2026, 9, 29, 4, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("distillrun000001"), Adapters: map[string]sourcepkg.Adapter{"filesystem": adapter}}
	return root, service, adapter
}
func writeDistillSource(t *testing.T, root, id string, from, to sourcepkg.Revision) {
	t.Helper()
	record := sourcepkg.Record{SchemaVersion: 1, ID: id, Adapter: "filesystem", Locator: sourcepkg.Locator{Path: "testdata"}, Status: "changed", Identity: sourcepkg.Identity{Name: id, Canonical: "testdata"}, Trust: sourcepkg.Trust{Source: "test", Reviewed: true}, Monitoring: sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"}, Limits: sourcepkg.Limits{TimeoutSeconds: 20, MaxBytes: 8 << 20, MaxFiles: 100, MaxFileBytes: 2 << 20}, CurrentRevision: &to, DistilledRevision: &from}
	data, err := sourcepkg.MarshalCanonical(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources/catalog", id+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
func revisionForDistill(value string, files map[string][]byte) sourcepkg.Revision {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	all := []byte{}
	for _, name := range names {
		all = append(all, []byte(name)...)
		all = append(all, files[name]...)
	}
	return sourcepkg.Revision{Kind: "declared-version", Value: value, ContentDigest: sourcepkg.Digest(all), ObservedAt: time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC)}
}
func prepareAndStart(t *testing.T, root string, service DistillService, id string) distillpkg.Run {
	t.Helper()
	prepared, err := service.PrepareDistillRuns(context.Background(), root, DistillPrepareInput{SourceIDs: []string{id}})
	if err != nil || prepared.Prepared != 1 {
		t.Fatalf("prepare = %#v %v", prepared, err)
	}
	run := *prepared.Results[0].Run
	started, err := service.StartDistillRun(context.Background(), root, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return started.Run
}
func completeCoverage() []distillpkg.CoverageEntry {
	return []distillpkg.CoverageEntry{{Resource: "SKILL.md", Status: "analyzed", Reason: "Read target."}, {Resource: "removed.md", Status: "analyzed", Reason: "Checked deletion."}}
}
func validSubmission(run distillpkg.Run, adapter revisionAdapter) DistillSubmission {
	target := adapter.files["r2"]["SKILL.md"]
	return DistillSubmission{Coverage: completeCoverage(), Findings: []FindingSubmission{{StableKey: "retry-review", Status: "active", What: "The source reviews retries.", Vocabulary: []string{"retry"}, Evidence: []distillpkg.Evidence{evidenceFor(run, "SKILL.md", "SKILL.md", target)}}}}
}

func evidenceFor(run distillpkg.Run, path, locator string, contents []byte) distillpkg.Evidence {
	return distillpkg.Evidence{Revision: distillpkg.IdentityOf(run.ToRevision), RunID: run.ID, PackageDigest: run.PackageDigest, Path: path, Locator: locator, Digest: sourcepkg.Digest(contents)}
}
