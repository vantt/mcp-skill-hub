package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

type fixedSourceID string

func (id fixedSourceID) New() (string, error) { return string(id), nil }

type sourceClock struct{ now time.Time }

func (clock sourceClock) Now() time.Time { return clock.now }

type fakeSourceAdapter struct {
	mu        sync.Mutex
	revisions map[string]sourcepkg.Revision
	errors    map[string]error
	calls     int
}

func (adapter *fakeSourceAdapter) Identify(_ context.Context, locator sourcepkg.Locator) (sourcepkg.Identity, error) {
	adapter.mu.Lock()
	adapter.calls++
	adapter.mu.Unlock()
	return sourcepkg.Identity{Name: "fixture", Canonical: locator.Repository, DefaultBranch: "main", License: "MIT"}, nil
}
func (adapter *fakeSourceAdapter) CurrentRevision(_ context.Context, source sourcepkg.Source) (sourcepkg.Revision, error) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.calls++
	if err := adapter.errors[source.ID]; err != nil {
		return sourcepkg.Revision{}, err
	}
	return adapter.revisions[source.ID], nil
}
func (*fakeSourceAdapter) Diff(context.Context, sourcepkg.Source, sourcepkg.Revision, sourcepkg.Revision) (sourcepkg.ChangeSet, error) {
	return sourcepkg.ChangeSet{}, nil
}
func (*fakeSourceAdapter) Read(context.Context, sourcepkg.Source, sourcepkg.Revision, string) ([]byte, error) {
	return nil, nil
}
func (*fakeSourceAdapter) List(context.Context, sourcepkg.Source, sourcepkg.Revision, sourcepkg.Scope) ([]sourcepkg.Resource, error) {
	return []sourcepkg.Resource{}, nil
}

func TestSourceOperationsEmitSanitizedPostOperationTelemetry(t *testing.T) {
	root := newSourceWorkspace(t)
	sink := &captureTelemetrySink{}
	adapter := &fakeSourceAdapter{revisions: map[string]sourcepkg.Revision{"source-a": revision("one")}, errors: map[string]error{}}
	service := SourceService{
		Clock: sourceClock{now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("0011223344556677"),
		Adapters: map[string]sourcepkg.Adapter{"git": adapter}, Telemetry: sink,
	}
	const locator = "https://github.com/private/example.git"
	const reason = "private evidence reason that must not be recorded"
	captured, err := service.CaptureSourceCandidate(t.Context(), root, SourceCandidateInput{Locator: locator, Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	preview, _, err := service.TriageSourceCandidate(t.Context(), root, SourceTriageInput{CandidateID: captured.Candidate.ID, Decision: "accept", SourceID: "source-a", Adapter: "git", MonitoringEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSourceProposal(t.Context(), root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CheckSources(t.Context(), root, []string{"source-a"}, false); err != nil {
		t.Fatal(err)
	}
	wantTypes := []string{telemetry.EventSourceCandidateCaptured, telemetry.EventSourceCandidateTriaged, telemetry.EventSourceChecked}
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
	for _, raw := range []string{locator, reason} {
		if bytes.Contains(encoded, []byte(raw)) {
			t.Fatalf("telemetry leaked raw source data %q: %s", raw, encoded)
		}
	}
}

func TestSourceTelemetryPanicDoesNotChangeCaptureResult(t *testing.T) {
	root := newSourceWorkspace(t)
	service := SourceService{Clock: sourceClock{now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("0011223344556677"), Telemetry: panickingTelemetrySink{}}
	result, err := service.CaptureSourceCandidate(t.Context(), root, SourceCandidateInput{Locator: "sources/private", Reason: "private reason"})
	if err != nil || result.Status != StatusApplied || result.Candidate.Status != "pending" {
		t.Fatalf("capture with panicking telemetry = %#v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "intake", result.Candidate.ID+".yaml")); err != nil {
		t.Fatalf("canonical result was not published: %v", err)
	}
}

func TestSourceChecksPreserveUnchangedCanonicalStatePersistChangesAndIsolateFailures(t *testing.T) {
	root := newSourceWorkspace(t)
	adapter := &fakeSourceAdapter{revisions: map[string]sourcepkg.Revision{}, errors: map[string]error{}}
	clock := sourceClock{now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}
	service := SourceService{Clock: clock, IDs: fixedSourceID("0011223344556677"), Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
	for index, id := range []string{"source-a", "source-b"} {
		adapter.revisions[id] = revision(string(rune('a' + index)))
		beforeCaptureCalls := adapter.calls
		captured, err := service.CaptureSourceCandidate(context.Background(), root, SourceCandidateInput{Locator: "https://github.com/example/repo" + string(rune('a'+index)) + ".git", Reason: "test source"})
		if err != nil {
			t.Fatal(err)
		}
		if adapter.calls != beforeCaptureCalls {
			t.Fatal("candidate capture performed network adapter work")
		}
		preview, _, err := service.TriageSourceCandidate(context.Background(), root, SourceTriageInput{CandidateID: captured.Candidate.ID, Decision: "accept", SourceID: id, Adapter: "git", MonitoringEnabled: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.ConfirmSourceProposal(context.Background(), root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
			t.Fatal(err)
		}
		service.IDs = fixedSourceID("8899aabbccddeeff")
	}
	commitWorkspace(t, root)
	before := gitStatus(t, root)
	result, err := service.CheckSources(context.Background(), root, []string{"source-a"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || len(result.Results) != 1 || result.Results[0].Status != "needs_analysis" {
		t.Fatalf("freshly onboarded source did not report needs_analysis: %#v", result)
	}
	if after := gitStatus(t, root); after != before {
		t.Fatalf("needs_analysis check dirtied Git: before %q after %q", before, after)
	}
	_, freshRecords, _ := readSourceRecords(root)
	for _, rec := range freshRecords {
		if rec.ID == "source-a" {
			rec.DistilledRevision = rec.CurrentRevision
			rec.Status = "watching"
			data, _ := sourcepkg.MarshalCanonical(rec)
			_ = os.WriteFile(filepath.Join(root, "sources/catalog/source-a.yaml"), data, 0o644)
		}
	}
	commitWorkspace(t, root)
	before = gitStatus(t, root)
	outsideOnly := adapter.revisions["source-a"]
	outsideOnly.Value = revision("different-commit-same-scoped-tree").Value
	adapter.revisions["source-a"] = outsideOnly
	result, err = service.CheckSources(context.Background(), root, []string{"source-a"}, false)
	if err != nil || result.Unchanged != 1 || result.Changed != 0 {
		t.Fatalf("scoped-tree unchanged result = %#v, %v", result, err)
	}
	_, scopedRecords, err := readSourceRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range scopedRecords {
		if record.ID == "source-a" && record.CurrentRevision.Value == outsideOnly.Value {
			t.Fatal("outside-scope commit created canonical work")
		}
	}

	adapter.revisions["source-a"] = revision("changed")
	adapter.errors["source-b"] = errors.New("temporary upstream failure")
	result, err = service.CheckSources(context.Background(), root, []string{"source-a", "source-b"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || result.Unavailable != 1 || result.Status != StatusPartialFailure {
		t.Fatalf("isolated result = %#v", result)
	}
	_, records, err := readSourceRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.ID == "source-a" && (record.CurrentRevision == nil || record.CurrentRevision.Value != revision("changed").Value) {
			t.Fatalf("changed revision not durable: %#v", record)
		}
	}
	home, err := (CurationService{}).GetCurationHome(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if home.HomeSummary.UnavailableSources != 1 || home.HomeSummary.SourcesDue != 0 {
		catalogState, _ := catalog.Inspect(context.Background(), root)
		t.Fatalf("source home = %#v catalog=%#v", home, catalogState)
	}
	var changedAction, unavailableAction bool
	for _, action := range home.Actions {
		changedAction = changedAction || action.Kind == "distill_changed_sources"
		unavailableAction = unavailableAction || action.Kind == "retry_unavailable_sources"
	}
	if !changedAction || !unavailableAction {
		t.Fatalf("source actions = %#v", home.Actions)
	}
}

func TestCloneRebuildRetainsRevisionLosesOperationalTimesAndStatusDoesNotFetch(t *testing.T) {
	root := newSourceWorkspace(t)
	adapter := &fakeSourceAdapter{revisions: map[string]sourcepkg.Revision{"source-a": revision("one")}, errors: map[string]error{}}
	service := SourceService{Clock: sourceClock{now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("0011223344556677"), Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
	skillPreview, err := (SkillService{}).PreviewCreate(context.Background(), root, skill.CreateInput{ID: "consumer-review", Collection: "software", Name: "Consumer Review", Description: "Review consumers.", Routing: skill.RoutingInput{Triggers: []string{"review consumers"}, NotFor: []string{"design systems"}, MinScope: "multi_step"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (SkillService{}).ConfirmSkillMutation(context.Background(), root, skillPreview, skillPreview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	captured, err := service.CaptureSourceCandidate(context.Background(), root, SourceCandidateInput{Locator: "https://github.com/example/repo.git", Reason: "test source"})
	if err != nil {
		t.Fatal(err)
	}
	preview, _, err := service.TriageSourceCandidate(context.Background(), root, SourceTriageInput{CandidateID: captured.Candidate.ID, Decision: "accept", SourceID: "source-a", Adapter: "git", MonitoringEnabled: true, SkillID: "consumer-review"})
	if err != nil {
		t.Fatal(err)
	}
	wrongPins := preview.Confirmation.Confirmation.Pins
	wrongPins.ProposalDigest = sourcepkg.Digest([]byte("wrong"))
	stale, err := service.ConfirmSourceProposal(context.Background(), root, preview, wrongPins)
	if err != nil || stale.Status != StatusError {
		t.Fatalf("wrong confirmation = %#v, %v", stale, err)
	}
	if _, err := service.ConfirmSourceProposal(context.Background(), root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "skills", "LINK-consumer-review--source-a.yaml")); err != nil {
		t.Fatalf("source link was not created: %v", err)
	}
	if _, err := service.CheckSources(context.Background(), root, []string{"source-a"}, false); err != nil {
		t.Fatal(err)
	}
	calls := adapter.calls
	if _, err := (CurationService{}).GetCurationHome(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if adapter.calls != calls {
		t.Fatal("status performed a source fetch")
	}
	commitWorkspace(t, root)
	clone := filepath.Join(t.TempDir(), "clone")
	command := exec.Command("git", "clone", "--quiet", root, clone)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v: %s", err, output)
	}
	if _, err := workspace.PrepareLayout(clone); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), clone, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	listed, err := (SourceService{}).ListSources(context.Background(), clone, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Sources) != 1 || listed.Sources[0].CurrentRevision == nil || listed.Sources[0].CurrentRevision.Value != revision("one").Value {
		t.Fatalf("cloned source = %#v", listed.Sources)
	}
	cloneHome, err := (CurationService{}).GetCurationHome(context.Background(), clone)
	if err != nil {
		t.Fatal(err)
	}
	if cloneHome.HomeSummary.SourcesDue != 1 {
		t.Fatalf("rebuilt source was not due after timestamp loss: %#v", cloneHome.HomeSummary)
	}
	if _, found, err := (sourcepkg.OperationalStore{Root: clone}).Get(context.Background(), "source-a"); err != nil || found {
		t.Fatalf("operational check timestamp unexpectedly restored: found=%t err=%v", found, err)
	}
}

func TestSourceTriageMonitoringOptOutBUG01(t *testing.T) {
	root := newSourceWorkspace(t)
	adapter := &fakeSourceAdapter{revisions: map[string]sourcepkg.Revision{"source-no-monitor": revision("one")}, errors: map[string]error{}}
	service := SourceService{
		Clock:    sourceClock{now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)},
		IDs:      fixedSourceID("1122334455667788"),
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	captured, err := service.CaptureSourceCandidate(t.Context(), root, SourceCandidateInput{
		Locator: "https://github.com/example/skills.git",
		Reason:  "test no monitor",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Triage with MonitoringEnabled: false and empty Cadence
	preview, _, err := service.TriageSourceCandidate(t.Context(), root, SourceTriageInput{
		CandidateID:       captured.Candidate.ID,
		Decision:          "accept",
		SourceID:          "source-no-monitor",
		Adapter:           "git",
		MonitoringEnabled: false,
		Cadence:           "",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify BUG-01 is fixed: monitoring remains disabled and cadence defaults to manual
	if preview.Source.Monitoring.Enabled {
		t.Fatalf("BUG-01 regression: Monitoring.Enabled became true when MonitoringEnabled: false")
	}
	if preview.Source.Monitoring.Cadence != "manual" {
		t.Fatalf("expected cadence manual, got %s", preview.Source.Monitoring.Cadence)
	}

	// Confirm into catalog and check persisted record
	if _, err := service.ConfirmSourceProposal(t.Context(), root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	recordData, err := os.ReadFile(filepath.Join(root, "sources", "catalog", "source-no-monitor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := sourcepkg.ParseRecord(recordData)
	if err != nil {
		t.Fatal(err)
	}
	if record.Monitoring.Enabled {
		t.Fatalf("BUG-01 regression in persisted record: monitoring.enabled is true")
	}
	if record.Monitoring.Cadence != "manual" {
		t.Fatalf("persisted record cadence = %s, want manual", record.Monitoring.Cadence)
	}
}

func TestSourceTriageGitHubTreeURLBUG10(t *testing.T) {
	root := newSourceWorkspace(t)
	adapter := &fakeSourceAdapter{revisions: map[string]sourcepkg.Revision{"source-tree": revision("one")}, errors: map[string]error{}}
	service := SourceService{
		Clock:    sourceClock{now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)},
		IDs:      fixedSourceID("2233445566778899"),
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	treeURL := "https://github.com/anthropics/skills/tree/main/skills/pdf"
	captured, err := service.CaptureSourceCandidate(t.Context(), root, SourceCandidateInput{
		Locator: treeURL,
		Reason:  "test tree url",
	})
	if err != nil {
		t.Fatal(err)
	}

	preview, _, err := service.TriageSourceCandidate(t.Context(), root, SourceTriageInput{
		CandidateID:       captured.Candidate.ID,
		Decision:          "accept",
		SourceID:          "source-tree",
		Adapter:           "git",
		MonitoringEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify BUG-10 is fixed: locator repository is canonical repo URL and ref/path resolved
	if preview.Source.Locator.Repository != "https://github.com/anthropics/skills.git" {
		t.Fatalf("expected repo https://github.com/anthropics/skills.git, got %s", preview.Source.Locator.Repository)
	}
	if preview.Source.Locator.Ref != "main" {
		t.Fatalf("expected ref main, got %s", preview.Source.Locator.Ref)
	}
	if preview.Source.Locator.Path != "skills/pdf" {
		t.Fatalf("expected path skills/pdf, got %s", preview.Source.Locator.Path)
	}
}

func TestSourceCaptureAndTriageLocalFolderBUG11(t *testing.T) {
	root := newSourceWorkspace(t)
	extFolder := filepath.Join(t.TempDir(), "external-folder")
	if err := os.MkdirAll(extFolder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extFolder, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(t.TempDir(), "cache")
	adapter := sourcepkg.FilesystemAdapter{
		Root:      extFolder,
		CacheRoot: cacheDir,
	}

	service := SourceService{
		Clock:    sourceClock{now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)},
		IDs:      fixedSourceID("3344556677889900"),
		Adapters: map[string]sourcepkg.Adapter{"filesystem": adapter},
	}

	// Capture absolute folder outside workspace - must be accepted!
	captured, err := service.CaptureSourceCandidate(t.Context(), root, SourceCandidateInput{
		Locator: extFolder,
		Reason:  "test abs folder",
	})
	if err != nil {
		t.Fatalf("capture of external folder failed: %v", err)
	}
	if captured.Candidate.Status != "pending" {
		t.Fatalf("expected pending status, got %s", captured.Candidate.Status)
	}
}

func newSourceWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	return root
}
func revision(value string) sourcepkg.Revision {
	digest := sourcepkg.Digest([]byte(value))
	return sourcepkg.Revision{Kind: "git-commit", Value: digest[len("sha256:"):][:40], ContentDigest: digest, ObservedAt: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}
}
func gitStatus(t *testing.T, root string) string {
	t.Helper()
	output, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}
