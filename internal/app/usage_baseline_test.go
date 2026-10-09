package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestBaselineSufficiencyTransition(t *testing.T) {
	service := UsageService{}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	minChains := 3
	snapshot := "sha256:test_snap_1"
	client := "claude-code"

	// 1. Initially: 2 resolved and 1 no_skill (< minChains 3)
	var events []telemetry.Event
	// 2 resolved
	for i := range 2 {
		events = append(events, telemetry.Event{
			ID:              fmt.Sprintf("evt_res_%d", i),
			Type:            telemetry.EventResolutionCompleted,
			OccurredAt:      now.Add(-time.Duration(10-i) * time.Hour),
			CatalogSnapshot: snapshot,
			Client:          telemetry.Client{Name: client},
			SessionIDHash:   fmt.Sprintf("sess_%d", i),
			ResolutionID:    fmt.Sprintf("res_ok_%d", i),
			Payload: map[string]any{
				"status":       "resolved",
				"top_skill_id": "skill-a",
			},
		})
	}
	// 1 no_skill
	events = append(events, telemetry.Event{
		ID:              "evt_noskill_0",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(-8 * time.Hour),
		CatalogSnapshot: snapshot,
		Client:          telemetry.Client{Name: client},
		SessionIDHash:   "sess_noskill_0",
		ResolutionID:    "res_noskill_0",
		Payload: map[string]any{
			"status": "no_skill",
		},
	})

	report := service.compileBaseline(events, now.Add(-24*time.Hour), now, minChains)
	if len(report.Buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(report.Buckets))
	}
	b := report.Buckets[0]
	if b.Sufficiency.Verdict != "insufficient" {
		t.Fatalf("expected insufficient, got %s", b.Sufficiency.Verdict)
	}
	if b.Sufficiency.ResolvedChains != 2 || b.Sufficiency.MissingResolved != 1 {
		t.Errorf("resolved: got %d, missing %d, want 2, 1", b.Sufficiency.ResolvedChains, b.Sufficiency.MissingResolved)
	}
	if b.Sufficiency.NoSkillChains != 1 || b.Sufficiency.MissingNoSkill != 2 {
		t.Errorf("no_skill: got %d, missing %d, want 1, 2", b.Sufficiency.NoSkillChains, b.Sufficiency.MissingNoSkill)
	}

	// 2. Add 1 more resolved and 2 more no_skill -> now resolved=3, no_skill=3 >= minChains
	events = append(events, telemetry.Event{
		ID:              "evt_res_2",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(-4 * time.Hour),
		CatalogSnapshot: snapshot,
		Client:          telemetry.Client{Name: client},
		SessionIDHash:   "sess_2",
		ResolutionID:    "res_ok_2",
		Payload: map[string]any{
			"status":       "resolved",
			"top_skill_id": "skill-a",
		},
	})
	for i := 1; i <= 2; i++ {
		events = append(events, telemetry.Event{
			ID:              fmt.Sprintf("evt_noskill_%d", i),
			Type:            telemetry.EventResolutionCompleted,
			OccurredAt:      now.Add(-time.Duration(4-i) * time.Hour),
			CatalogSnapshot: snapshot,
			Client:          telemetry.Client{Name: client},
			SessionIDHash:   fmt.Sprintf("sess_noskill_%d", i),
			ResolutionID:    fmt.Sprintf("res_noskill_%d", i),
			Payload: map[string]any{
				"status": "no_skill",
			},
		})
	}

	report2 := service.compileBaseline(events, now.Add(-24*time.Hour), now, minChains)
	if len(report2.Buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(report2.Buckets))
	}
	b2 := report2.Buckets[0]
	if b2.Sufficiency.Verdict != "sufficient" {
		t.Fatalf("expected verdict 'sufficient', got %q", b2.Sufficiency.Verdict)
	}
	if b2.Sufficiency.MissingResolved != 0 || b2.Sufficiency.MissingNoSkill != 0 {
		t.Errorf("expected 0 missing, got missing resolved %d, missing no_skill %d", b2.Sufficiency.MissingResolved, b2.Sufficiency.MissingNoSkill)
	}
}

func TestBaselineSnapshotChangeResetsBaseline(t *testing.T) {
	service := UsageService{}
	t0 := time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)

	events := []telemetry.Event{
		{
			ID:              "evt_old",
			Type:            telemetry.EventResolutionCompleted,
			OccurredAt:      t0,
			CatalogSnapshot: "sha256:snapshot_v1",
			Client:          telemetry.Client{Name: "claude-code"},
			ResolutionID:    "res_old",
			Payload:         map[string]any{"status": "resolved", "top_skill_id": "skill-1"},
		},
		{
			ID:              "evt_new",
			Type:            telemetry.EventResolutionCompleted,
			OccurredAt:      t1,
			CatalogSnapshot: "sha256:snapshot_v2",
			Client:          telemetry.Client{Name: "claude-code"},
			ResolutionID:    "res_new",
			Payload:         map[string]any{"status": "resolved", "top_skill_id": "skill-2"},
		},
	}

	report := service.compileBaseline(events, t0.Add(-24*time.Hour), t1.Add(time.Hour), 5)
	if report.ActiveSnapshot != "sha256:snapshot_v2" {
		t.Fatalf("expected active snapshot 'sha256:snapshot_v2', got %q", report.ActiveSnapshot)
	}
	if len(report.Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(report.Buckets))
	}

	var bucketV1, bucketV2 *BaselineBucket
	for i := range report.Buckets {
		switch report.Buckets[i].CatalogSnapshot {
		case "sha256:snapshot_v1":
			bucketV1 = &report.Buckets[i]
		case "sha256:snapshot_v2":
			bucketV2 = &report.Buckets[i]
		}
	}
	if bucketV1 == nil || bucketV2 == nil {
		t.Fatal("missing bucket for snapshot_v1 or snapshot_v2")
	}

	if !bucketV2.IsBaselineWindow || bucketV2.Status != "baseline" {
		t.Errorf("snapshot_v2 should be baseline window: isBaseline=%v, status=%s", bucketV2.IsBaselineWindow, bucketV2.Status)
	}
	if bucketV1.IsBaselineWindow || bucketV1.Status != "superseded" {
		t.Errorf("snapshot_v1 should be superseded: isBaseline=%v, status=%s", bucketV1.IsBaselineWindow, bucketV1.Status)
	}
}

func TestBaselineUnknownVsZero(t *testing.T) {
	service := UsageService{}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	// An event with no_skill status and no load
	events := []telemetry.Event{
		{
			ID:              "evt_1",
			Type:            telemetry.EventResolutionCompleted,
			OccurredAt:      now.Add(-time.Hour),
			CatalogSnapshot: "sha256:snap",
			Client:          telemetry.Client{Name: "claude-code"},
			ResolutionID:    "res_1",
			Payload:         map[string]any{"status": "no_skill"},
		},
	}

	report := service.compileBaseline(events, now.Add(-24*time.Hour), now, 5)
	b := report.Buckets[0]

	// acceptance_rate denominator is 0 (since 0 resolved chains)
	if b.Metrics.AcceptanceRate.Status != "unknown" {
		t.Errorf("expected acceptance_rate status 'unknown', got %q", b.Metrics.AcceptanceRate.Status)
	}
	if b.Metrics.AcceptanceRate.Rate != nil {
		t.Errorf("expected acceptance_rate.Rate to be nil, got %v", *b.Metrics.AcceptanceRate.Rate)
	}
}

func TestBaselineUnsolicitedShare(t *testing.T) {
	service := UsageService{}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	snap := "sha256:snap_load"
	client := "claude-code"

	events := []telemetry.Event{
		// 1 resolution
		{
			ID:              "evt_res",
			Type:            telemetry.EventResolutionCompleted,
			OccurredAt:      now.Add(-2 * time.Hour),
			CatalogSnapshot: snap,
			Client:          telemetry.Client{Name: client},
			ResolutionID:    "res_load_1",
			SessionIDHash:   "sess_load",
			Payload:         map[string]any{"status": "resolved", "top_skill_id": "skill-1"},
		},
		// 1 recommended load
		{
			ID:              "evt_load_rec",
			Type:            telemetry.EventSkillLoaded,
			OccurredAt:      now.Add(-time.Hour),
			CatalogSnapshot: snap,
			Client:          telemetry.Client{Name: client},
			ResolutionID:    "res_load_1",
			SessionIDHash:   "sess_load",
			Payload: map[string]any{
				"basis":       telemetry.LoadBasisServerObserved,
				"skill_id":    "skill-1",
				"attribution": "recommended",
			},
		},
		// 1 unsolicited load (e.g. after server restart)
		{
			ID:              "evt_load_unsol",
			Type:            telemetry.EventSkillLoaded,
			OccurredAt:      now.Add(-30 * time.Minute),
			CatalogSnapshot: snap,
			Client:          telemetry.Client{Name: client},
			SessionIDHash:   "sess_unsol",
			Payload: map[string]any{
				"basis":       telemetry.LoadBasisServerObserved,
				"skill_id":    "skill-2",
				"attribution": "unsolicited",
			},
		},
	}

	report := service.compileBaseline(events, now.Add(-24*time.Hour), now, 5)
	b := report.Buckets[0]

	if b.TotalLoads != 2 {
		t.Fatalf("expected 2 total loads, got %d", b.TotalLoads)
	}
	if b.UnsolicitedLoads != 1 {
		t.Fatalf("expected 1 unsolicited load, got %d", b.UnsolicitedLoads)
	}
	if b.UnsolicitedShare.Rate == nil || *b.UnsolicitedShare.Rate != 0.5 {
		t.Fatalf("expected unsolicited share 0.5, got %v", b.UnsolicitedShare.Rate)
	}
}

func TestBaselineRawRetentionPruned(t *testing.T) {
	service := UsageService{}
	now := time.Now().UTC()

	// Window of 7 days -> not pruned
	rep1 := service.compileBaseline(nil, now.Add(-7*24*time.Hour), now, 30)
	if rep1.RawRetentionPruned {
		t.Fatal("expected RawRetentionPruned to be false for 7d window")
	}

	// Window of 45 days -> pruned (retention is 30d)
	rep2 := service.compileBaseline(nil, now.Add(-45*24*time.Hour), now, 30)
	if !rep2.RawRetentionPruned {
		t.Fatal("expected RawRetentionPruned to be true for 45d window")
	}
}
