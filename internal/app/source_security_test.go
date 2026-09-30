package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestAllDueExcludesManualAndDisabledButExplicitIDsStillRun(t *testing.T) {
	root := newSourceWorkspace(t)
	now := time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC)
	for _, record := range []sourcepkg.Record{
		testSourceRecord("manual-source", sourcepkg.Monitoring{Enabled: true, Cadence: "manual"}, revision("manual")),
		testSourceRecord("disabled-source", sourcepkg.Monitoring{Enabled: false, Cadence: "weekly"}, revision("disabled")),
	} {
		data, err := sourcepkg.MarshalCanonical(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "sources", "catalog", record.ID+".yaml"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	adapter := &fakeSourceAdapter{revisions: map[string]sourcepkg.Revision{"manual-source": revision("manual"), "disabled-source": revision("disabled")}, errors: map[string]error{}}
	service := SourceService{Clock: sourceClock{now: now}, Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
	result, err := service.CheckSources(context.Background(), root, nil, true)
	if err != nil || result.Checked != 0 {
		t.Fatalf("all-due result = %#v, %v", result, err)
	}
	result, err = service.CheckSources(context.Background(), root, []string{"manual-source", "disabled-source"}, true)
	if err != nil || result.Checked != 2 || result.Unchanged != 2 {
		t.Fatalf("explicit result = %#v, %v", result, err)
	}
}

func TestCorruptOperationalDatabaseDoesNotBlockStatusAndSourceBecomesDue(t *testing.T) {
	root := newSourceWorkspace(t)
	record := testSourceRecord("due-after-reset", sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"}, revision("old"))
	data, err := sourcepkg.MarshalCanonical(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", record.ID+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime", "operational.db"), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	home, err := (CurationService{}).GetCurationHome(context.Background(), root)
	if err != nil || home.HomeSummary.SourcesDue != 1 {
		t.Fatalf("status after operational corruption = %#v, %v", home, err)
	}
}

func TestChangedSourceIsScheduledOnlyAfterCanonicalPublication(t *testing.T) {
	root := newSourceWorkspace(t)
	now := time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC)
	record := testSourceRecord("publish-failure", sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"}, revision("old"))
	data, err := sourcepkg.MarshalCanonical(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", record.ID+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	generations := filepath.Join(root, "runtime", "catalog", "generations")
	if err := os.RemoveAll(generations); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), generations); err != nil {
		t.Fatal(err)
	}
	adapter := &fakeSourceAdapter{revisions: map[string]sourcepkg.Revision{record.ID: revision("new")}, errors: map[string]error{}}
	service := SourceService{Clock: sourceClock{now: now}, Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
	result, err := service.CheckSources(context.Background(), root, []string{record.ID}, false)
	if err != nil || result.Unavailable != 1 || result.Changed != 0 {
		t.Fatalf("publication failure = %#v, %v", result, err)
	}
	state, found, err := (sourcepkg.OperationalStore{Root: root}).Get(context.Background(), record.ID)
	if err != nil || !found || state.NextCheckAt.After(now) || state.Availability != "unavailable" {
		t.Fatalf("publication failure schedule = %#v, %t, %v", state, found, err)
	}
}

func TestSourceProposalExpiryIsRecheckedAtConfirmation(t *testing.T) {
	root := newSourceWorkspace(t)
	now := time.Date(2026, 9, 29, 1, 0, 0, 0, time.UTC)
	adapter := &fakeSourceAdapter{revisions: map[string]sourcepkg.Revision{"expiring-source": revision("one")}, errors: map[string]error{}}
	service := SourceService{Clock: sourceClock{now: now}, IDs: fixedSourceID("0011223344556677"), Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
	captured, err := service.CaptureSourceCandidate(context.Background(), root, SourceCandidateInput{Locator: "https://github.com/example/expiry.git", Reason: "expiry test"})
	if err != nil {
		t.Fatal(err)
	}
	preview, _, err := service.TriageSourceCandidate(context.Background(), root, SourceTriageInput{CandidateID: captured.Candidate.ID, Decision: "accept", SourceID: "expiring-source", Adapter: "git", MonitoringEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	service.Clock = sourceClock{now: now.Add(25 * time.Hour)}
	result, err := service.ConfirmSourceProposal(context.Background(), root, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil || result.Status != StatusError {
		t.Fatalf("expired confirmation = %#v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "expiring-source.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expired proposal changed canonical state: %v", err)
	}
}

func testSourceRecord(id string, monitoring sourcepkg.Monitoring, current sourcepkg.Revision) sourcepkg.Record {
	return sourcepkg.Record{
		SchemaVersion: 1, ID: id, Adapter: "git", Locator: sourcepkg.Locator{Repository: "https://github.com/example/" + id + ".git", Ref: "main"},
		Status: "watching", Identity: sourcepkg.Identity{Name: id, Canonical: "https://github.com/example/" + id + ".git", DefaultBranch: "main"},
		Trust: sourcepkg.Trust{Source: "community"}, Monitoring: monitoring,
		Limits:            sourcepkg.Limits{TimeoutSeconds: 20, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize},
		CurrentRevision:   &current,
		DistilledRevision: &current,
	}
}
