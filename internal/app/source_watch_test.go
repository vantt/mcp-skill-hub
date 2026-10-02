package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

const testGitHubURL = "https://github.com/example/skills.git"

func TestLocalWatchReturnsUnsupportedWithoutWrites(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	service := SourceService{
		Clock: sourceClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, loc := range []string{"./local-folder", "/abs/folder", "~/my-skills", "plain-folder"} {
		proposal, err := service.PreviewSourceWatch(context.Background(), root, SourceWatchInput{
			Locator: loc,
		})
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", loc, err)
		}
		if proposal.Error == nil {
			t.Fatalf("expected error for local watch locator %q, got nil", loc)
		}
		if proposal.Error.Code != ErrorLocalWatchUnsupported {
			t.Fatalf("expected ErrorLocalWatchUnsupported for %q, got %s", loc, proposal.Error.Code)
		}

		// Verify no files were created
		catalogEntries, err := os.ReadDir(filepath.Join(root, "sources", "catalog"))
		if err == nil && len(catalogEntries) > 0 {
			t.Fatalf("files written to sources/catalog for local watch %q", loc)
		}
	}
}

func TestSourceWatchPreviewAndConfirmPublicGitHub(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)

	adapter := &fakeSourceAdapter{
		revisions: map[string]sourcepkg.Revision{
			"custom-watch": revision("one"),
		},
		errors: map[string]error{},
	}

	service := SourceService{
		Clock:    sourceClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	ctx := context.Background()

	// 1. Preview source watch
	preview, err := service.PreviewSourceWatch(ctx, root, SourceWatchInput{
		Locator:  testGitHubURL,
		SourceID: "custom-watch",
	})
	if err != nil {
		t.Fatalf("PreviewSourceWatch failed: %v", err)
	}
	if preview.Error != nil {
		t.Fatalf("unexpected preview error: %s: %s", preview.Error.Code, preview.Error.Render.Why)
	}
	if preview.Source.ID != "custom-watch" {
		t.Fatalf("expected source ID custom-watch, got %s", preview.Source.ID)
	}
	if preview.Source.Status != "watching" {
		t.Fatalf("expected status watching, got %s", preview.Source.Status)
	}
	if !preview.Source.Monitoring.Enabled || preview.Source.Monitoring.Cadence != "weekly" {
		t.Fatalf("expected default weekly enabled monitoring, got %#v", preview.Source.Monitoring)
	}
	if len(preview.Diff.Added) != 1 || preview.Diff.Added[0] != "sources/catalog/custom-watch.yaml" {
		t.Fatalf("unexpected diff added: %v", preview.Diff.Added)
	}

	// 2. Confirm source watch
	pins := preview.Confirmation.Confirmation.Pins
	result, err := service.ConfirmSourceWatch(ctx, root, preview, pins)
	if err != nil {
		t.Fatalf("ConfirmSourceWatch failed: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("unexpected confirm error: %s", result.Error.Render.Why)
	}
	if result.SourceID != "custom-watch" {
		t.Fatalf("expected source ID custom-watch, got %s", result.SourceID)
	}

	// 3. Verify canonical source record exists
	recordPath := filepath.Join(root, "sources", "catalog", "custom-watch.yaml")
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("canonical record was not written: %v", err)
	}

	// 4. Verify watching NEVER imports skills
	skillsEntries, err := os.ReadDir(filepath.Join(root, "skills"))
	if err == nil {
		for _, entry := range skillsEntries {
			if entry.Name() != ".gitkeep" {
				t.Fatalf("source watch unexpectedly created skill file: %s", entry.Name())
			}
		}
	}
}

func TestSourceWatchIdempotencyAndConflict(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)

	adapter := &fakeSourceAdapter{
		revisions: map[string]sourcepkg.Revision{
			"shared-source": revision("one"),
			"different-id":  revision("two"),
		},
		errors: map[string]error{},
	}

	service := SourceService{
		Clock:    sourceClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	ctx := context.Background()

	// 1. Initial watch
	preview, err := service.PreviewSourceWatch(ctx, root, SourceWatchInput{
		Locator:  testGitHubURL,
		SourceID: "shared-source",
		Cadence:  "weekly",
	})
	if err != nil || preview.Error != nil {
		t.Fatalf("initial preview failed: %v, %v", err, preview.Error)
	}
	_, err = service.ConfirmSourceWatch(ctx, root, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatalf("initial confirm failed: %v", err)
	}

	// 2. Exact same configuration -> idempotent success
	idempotentPreview, err := service.PreviewSourceWatch(ctx, root, SourceWatchInput{
		Locator:  testGitHubURL,
		SourceID: "shared-source",
		Cadence:  "weekly",
	})
	if err != nil {
		t.Fatalf("idempotent preview call failed: %v", err)
	}
	if idempotentPreview.Error != nil {
		t.Fatalf("expected idempotent success, got error: %s", idempotentPreview.Error.Code)
	}
	if idempotentPreview.Status != StatusOK {
		t.Fatalf("expected StatusOK for idempotent preview, got %s", idempotentPreview.Status)
	}

	// 3. Same ID with different policy -> conflict
	conflictPreview, err := service.PreviewSourceWatch(ctx, root, SourceWatchInput{
		Locator:  testGitHubURL,
		SourceID: "shared-source",
		Cadence:  "daily", // different cadence!
	})
	if err != nil {
		t.Fatalf("conflict preview call error: %v", err)
	}
	if conflictPreview.Error == nil || conflictPreview.Error.Code != ErrorSourceConflict {
		t.Fatalf("expected ErrorSourceConflict for changed cadence, got %#v", conflictPreview.Error)
	}

	// 4. Same locator under different ID with different policy -> conflict
	diffIDPreview, err := service.PreviewSourceWatch(ctx, root, SourceWatchInput{
		Locator:  testGitHubURL,
		SourceID: "different-id",
		Cadence:  "daily",
	})
	if err != nil {
		t.Fatalf("diff ID preview call error: %v", err)
	}
	if diffIDPreview.Error == nil || diffIDPreview.Error.Code != ErrorSourceConflict {
		t.Fatalf("expected ErrorSourceConflict for same locator under different ID, got %#v", diffIDPreview.Error)
	}
}

func TestMonitoringDisabledRemainsFalseBUG01(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)

	adapter := &fakeSourceAdapter{
		revisions: map[string]sourcepkg.Revision{
			"unmonitored": revision("one"),
		},
		errors: map[string]error{},
	}

	service := SourceService{
		Clock:    sourceClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	ctx := context.Background()

	// Explicitly disable monitoring without cadence
	noMonitor := false
	preview, err := service.PreviewSourceWatch(ctx, root, SourceWatchInput{
		Locator:           testGitHubURL,
		SourceID:          "unmonitored",
		MonitoringEnabled: &noMonitor,
	})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %v", err, preview.Error)
	}

	// Must remain false and default cadence to manual!
	if preview.Source.Monitoring.Enabled {
		t.Fatalf("BUG-01 regression: Monitoring.Enabled unexpectedly became true!")
	}
	if preview.Source.Monitoring.Cadence != "manual" {
		t.Fatalf("expected cadence to default to manual when monitoring is disabled, got %s", preview.Source.Monitoring.Cadence)
	}

	// Confirm and check persisted record
	_, err = service.ConfirmSourceWatch(ctx, root, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}

	recordData, err := os.ReadFile(filepath.Join(root, "sources", "catalog", "unmonitored.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := sourcepkg.ParseRecord(recordData)
	if err != nil {
		t.Fatal(err)
	}
	if record.Monitoring.Enabled {
		t.Fatalf("BUG-01 regression in persisted record: Monitoring.Enabled is true!")
	}
	if record.Monitoring.Cadence != "manual" {
		t.Fatalf("persisted record cadence = %s, want manual", record.Monitoring.Cadence)
	}
}

func TestSourceWatchStaleProposalFails(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)

	adapter := &fakeSourceAdapter{
		revisions: map[string]sourcepkg.Revision{
			"stale-watch": revision("one"),
		},
		errors: map[string]error{},
	}

	service := SourceService{
		Clock:    sourceClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	ctx := context.Background()

	preview, err := service.PreviewSourceWatch(ctx, root, SourceWatchInput{
		Locator:  testGitHubURL,
		SourceID: "stale-watch",
	})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %v", err, preview.Error)
	}

	// 1. Wrong proposal digest
	wrongPins := preview.Confirmation.Confirmation.Pins
	wrongPins.ProposalDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	result, err := service.ConfirmSourceWatch(ctx, root, preview, wrongPins)
	if err != nil {
		t.Fatalf("unexpected confirm error: %v", err)
	}
	if result.Status != StatusError || result.Error == nil || result.Error.Code != ErrorStaleProposal {
		t.Fatalf("expected ErrorStaleProposal for wrong digest, got %#v", result)
	}

	// 2. Expired proposal
	expiredPreview := preview
	expiredPreview.expiresAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	resultExp, err := service.ConfirmSourceWatch(ctx, root, expiredPreview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatalf("unexpected confirm error: %v", err)
	}
	if resultExp.Status != StatusError || resultExp.Error == nil || resultExp.Error.Code != ErrorStaleProposal {
		t.Fatalf("expected ErrorStaleProposal for expired preview, got %#v", resultExp)
	}
}
