package app

import (
	"context"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestTwoSessionsSameTaskFormTwoChains(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	// Two resolutions in two distinct sessions with identical task description
	// and identical resolution_id (fingerprint of same content)
	evt1 := telemetry.Event{
		ID:              "evt_sess1_res",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now,
		SessionIDHash:   "sess_one_hash",
		ResolutionID:    "res_same_fingerprint",
		CatalogSnapshot: "sha256:snap1",
		PolicyRevision:  "sha256:pol1",
		Client:          telemetry.Client{Name: "claude-code"},
		Payload: map[string]any{
			"status":                "resolved",
			"top_skill_id":          "code-review",
			"recommended_skill_ids": []string{"code-review"},
		},
	}
	evt2 := telemetry.Event{
		ID:              "evt_sess2_res",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(time.Minute),
		SessionIDHash:   "sess_two_hash",
		ResolutionID:    "res_same_fingerprint",
		CatalogSnapshot: "sha256:snap1",
		PolicyRevision:  "sha256:pol1",
		Client:          telemetry.Client{Name: "cursor"},
		Payload: map[string]any{
			"status":                "resolved",
			"top_skill_id":          "code-review",
			"recommended_skill_ids": []string{"code-review"},
		},
	}

	service := UsageService{}
	chains, _, _ := service.buildChains([]telemetry.Event{evt1, evt2})

	if len(chains) != 2 {
		t.Fatalf("expected 2 distinct chains for 2 different sessions, got %d", len(chains))
	}
	if chains[0].SessionHash == chains[1].SessionHash {
		t.Fatalf("chains share session hash: %q", chains[0].SessionHash)
	}
}

func TestForgedPriorDoesNotCountInReformulationRate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	// Valid chain with verified reformulation
	validRes1 := telemetry.Event{
		ID:              "evt_v1",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now,
		SessionIDHash:   "sess_valid",
		ResolutionID:    "res_v1",
		CatalogSnapshot: "sha256:snap1",
		PolicyRevision:  "sha256:pol1",
		Client:          telemetry.Client{Name: "claude-code"},
		Payload: map[string]any{
			"status":                "resolved",
			"top_skill_id":          "code-review",
			"recommended_skill_ids": []string{"code-review"},
		},
	}
	validRes2 := telemetry.Event{
		ID:              "evt_v2",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(2 * time.Minute),
		SessionIDHash:   "sess_valid",
		ResolutionID:    "res_v2",
		CatalogSnapshot: "sha256:snap1",
		PolicyRevision:  "sha256:pol1",
		Client:          telemetry.Client{Name: "claude-code"},
		Payload: map[string]any{
			"status":                "resolved",
			"top_skill_id":          "code-review",
			"recommended_skill_ids": []string{"code-review"},
			"prior_resolution_id":   "res_v1",
			"prior_kind":            "rejected",
			"prior_verified":        true,
		},
	}

	// Forged prior: unverified (e.g. ID not issued to this session)
	forgedRes := telemetry.Event{
		ID:              "evt_f1",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(5 * time.Minute),
		SessionIDHash:   "sess_attacker",
		ResolutionID:    "res_f1",
		CatalogSnapshot: "sha256:snap1",
		PolicyRevision:  "sha256:pol1",
		Client:          telemetry.Client{Name: "other"},
		Payload: map[string]any{
			"status":                "resolved",
			"top_skill_id":          "code-review",
			"recommended_skill_ids": []string{"code-review"},
			"prior_resolution_id":   "res_stolen_id",
			"prior_kind":            "rejected",
			"prior_verified":        false, // rejected by server verification
		},
	}

	service := UsageService{}
	chains, counts, firstValid := service.buildChains([]telemetry.Event{validRes1, validRes2, forgedRes})

	// Total chains should be 2: sess_valid (linked) and sess_attacker (unverified prior starts new chain)
	if len(chains) != 2 {
		t.Fatalf("expected 2 chains, got %d", len(chains))
	}

	metrics := calculateChainMetrics(chains, counts, firstValid, now.Add(3*time.Hour), "")

	// Total chains = 2
	if metrics.TotalChains != 2 {
		t.Fatalf("TotalChains = %d, want 2", metrics.TotalChains)
	}

	// Reformulation rate should be 1/2 (only the verified one), NOT 2/2!
	if metrics.ReformulationRate.Numerator != 1 || metrics.ReformulationRate.Denominator != 2 {
		t.Fatalf("ReformulationRate = %+v, want 1/2", metrics.ReformulationRate)
	}
	if metrics.ReformulationRate.Rate == nil || *metrics.ReformulationRate.Rate != 0.5 {
		t.Fatalf("expected rate 0.5, got %v", metrics.ReformulationRate.Rate)
	}
}

func TestEveryRateStaysInZeroToOne(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	// Construct chains testing all statuses and outcomes
	events := []telemetry.Event{
		// 1. Resolved and accepted
		{
			ID: "e1", Type: telemetry.EventResolutionCompleted, OccurredAt: now.Add(-3 * time.Hour),
			SessionIDHash: "s1", ResolutionID: "r1", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "claude-code"},
			Payload: map[string]any{"status": "resolved", "top_skill_id": "skill-a"},
		},
		{
			ID: "l1", Type: telemetry.EventSkillLoaded, OccurredAt: now.Add(-3*time.Hour + time.Minute),
			SessionIDHash: "s1", ResolutionID: "r1", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "claude-code"},
			Payload: map[string]any{"skill_id": "skill-a", "basis": telemetry.LoadBasisServerObserved},
		},
		// 2. Resolved and override
		{
			ID: "e2", Type: telemetry.EventResolutionCompleted, OccurredAt: now.Add(-3 * time.Hour),
			SessionIDHash: "s2", ResolutionID: "r2", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "codex"},
			Payload: map[string]any{"status": "resolved", "top_skill_id": "skill-a"},
		},
		{
			ID: "l2", Type: telemetry.EventSkillLoaded, OccurredAt: now.Add(-3*time.Hour + time.Minute),
			SessionIDHash: "s2", ResolutionID: "r2", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "codex"},
			Payload: map[string]any{"skill_id": "skill-b", "basis": telemetry.LoadBasisServerObserved},
		},
		// 3. Resolved and ignored (older than 2h TTL)
		{
			ID: "e3", Type: telemetry.EventResolutionCompleted, OccurredAt: now.Add(-3 * time.Hour),
			SessionIDHash: "s3", ResolutionID: "r3", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "cursor"},
			Payload: map[string]any{"status": "resolved", "top_skill_id": "skill-a"},
		},
		// 4. No skill and false no skill (load occurred)
		{
			ID: "e4", Type: telemetry.EventResolutionCompleted, OccurredAt: now.Add(-3 * time.Hour),
			SessionIDHash: "s4", ResolutionID: "r4", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "gemini"},
			Payload: map[string]any{"status": "no_skill"},
		},
		{
			ID: "l4", Type: telemetry.EventSkillLoaded, OccurredAt: now.Add(-3*time.Hour + time.Minute),
			SessionIDHash: "s4", ResolutionID: "r4", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "gemini"},
			Payload: map[string]any{"skill_id": "skill-c", "basis": telemetry.LoadBasisServerObserved},
		},
		// 5. No skill and true no skill (older than 2h TTL, no load)
		{
			ID: "e5", Type: telemetry.EventResolutionCompleted, OccurredAt: now.Add(-3 * time.Hour),
			SessionIDHash: "s5", ResolutionID: "r5", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "gemini"},
			Payload: map[string]any{"status": "no_skill"},
		},
		// 6. Already covered bucket
		{
			ID: "e6", Type: telemetry.EventResolutionCompleted, OccurredAt: now.Add(-3 * time.Hour),
			SessionIDHash: "s6", ResolutionID: "r6", CatalogSnapshot: "sha256:1", PolicyRevision: "sha256:1",
			Client:  telemetry.Client{Name: "other"},
			Payload: map[string]any{"status": "already_covered"},
		},
		// Clarification events
		{ID: "c1", Type: telemetry.EventClarificationRequested, OccurredAt: now.Add(-time.Hour)},
		{ID: "c2", Type: telemetry.EventClarificationAnswered, OccurredAt: now.Add(-50 * time.Minute)},
	}

	service := UsageService{}
	chains, counts, firstValid := service.buildChains(events)
	metrics := calculateChainMetrics(chains, counts, firstValid, now, "")

	if metrics.ChainsAlreadyCovered != 1 {
		t.Fatalf("ChainsAlreadyCovered = %d, want 1", metrics.ChainsAlreadyCovered)
	}

	allRates := []*RateMetric{
		&metrics.AcceptanceRate,
		&metrics.OverrideRate,
		&metrics.FalseNoSkillRate,
		&metrics.TrueNoSkill,
		&metrics.ReformulationRate,
		&metrics.IgnoreRate,
		&metrics.NeedsContextAnswerRate,
		&metrics.BypassRate,
		&metrics.NegativeAfterLoad,
	}

	for _, m := range allRates {
		if m.Rate != nil {
			if *m.Rate < 0.0 || *m.Rate > 1.0 {
				t.Fatalf("rate out of [0, 1]: %f", *m.Rate)
			}
		}
	}

	// Verify cuts by client
	cuts := calculateCuts(chains, counts, firstValid, now, "client")
	if len(cuts) == 0 {
		t.Fatal("expected non-empty cuts by client")
	}
	for _, cut := range cuts {
		if cut.Key == "" {
			t.Error("cut key is empty")
		}
	}
}

func TestChainsDisagreementListing(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	createAndActivateSkill(t, SkillService{}, root)

	telService := TelemetryService{}
	recorder, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	now := time.Now().UTC()
	// Record an override chain: resolved to consumer-review, loaded other-skill
	recorder.Record(telemetry.Event{
		ID:              "evt_override_res",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(-10 * time.Minute),
		SessionIDHash:   "sess_disagree",
		ResolutionID:    "res_disagree_1",
		CatalogSnapshot: "sha256:snap",
		PolicyRevision:  "sha256:pol",
		Client:          telemetry.Client{Name: "claude-code"},
		Payload: map[string]any{
			"status":                "resolved",
			"top_skill_id":          "consumer-review",
			"recommended_skill_ids": []string{"consumer-review"},
			"operation":             "review",
		},
	})
	recorder.Record(telemetry.Event{
		ID:              "evt_override_load",
		Type:            telemetry.EventSkillLoaded,
		OccurredAt:      now.Add(-5 * time.Minute),
		SessionIDHash:   "sess_disagree",
		ResolutionID:    "res_disagree_1",
		CatalogSnapshot: "sha256:snap",
		PolicyRevision:  "sha256:pol",
		Client:          telemetry.Client{Name: "claude-code"},
		Payload: map[string]any{
			"skill_id": "other-skill", "surface": "skill_get", "resource_kind": "entrypoint",
			"basis": telemetry.LoadBasisServerObserved, "attribution": "override",
		},
	})
	recorder.Flush(context.Background())

	usage := UsageService{Telemetry: telService}
	chains, err := usage.Chains(context.Background(), root, now.Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("Chains failed: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("expected at least 1 disagreement chain")
	}
	found := false
	for _, c := range chains {
		if c.Kind == "override" && c.LoadedSkill == "other-skill" {
			found = true
			if c.SessionHash != "sess_disagree" {
				t.Fatalf("session_hash = %q, want sess_disagree", c.SessionHash)
			}
		}
	}
	if !found {
		t.Fatal("expected override disagreement chain to be found")
	}
}

func TestFunnelDenominatorsWithTopKFields(t *testing.T) {
	events := []telemetry.Event{
		{
			ID: "res-1", Type: telemetry.EventResolutionCompleted, OccurredAt: time.Now(), SessionIDHash: "session-1",
			Payload: map[string]any{
				"status": "resolved", "operation": "test", "candidate_count": 1.0,
				"topk_skill_ids": []any{"skill-1"},
				"topk_matched":   []any{"1:operation"},
				"topk_channels":  []any{"fts"},
			},
		},
	}
	chains, _, _ := UsageService{}.buildChains(events)
	if len(chains) != 1 {
		t.Errorf("Expected 1 chain, got %d", len(chains))
	}
}

func TestOverrideTopKFromLoadEventShowsInChains(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	createAndActivateSkill(t, SkillService{}, root)

	telService := TelemetryService{}
	recorder, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	now := time.Now().UTC()
	recorder.Record(telemetry.Event{
		ID:              "evt_override_res",
		Type:            telemetry.EventResolutionCompleted,
		OccurredAt:      now.Add(-10 * time.Minute),
		SessionIDHash:   "sess_override_topk",
		ResolutionID:    "res_override_topk",
		CatalogSnapshot: "sha256:snap",
		PolicyRevision:  "sha256:pol",
		Client:          telemetry.Client{Name: "claude-code"},
		Payload: map[string]any{
			"status":                "resolved",
			"top_skill_id":          "consumer-review",
			"recommended_skill_ids": []string{"consumer-review"},
			"operation":             "review",
		},
	})
	recorder.Record(telemetry.Event{
		ID:              "evt_override_load",
		Type:            telemetry.EventSkillLoaded,
		OccurredAt:      now.Add(-5 * time.Minute),
		SessionIDHash:   "sess_override_topk",
		ResolutionID:    "res_override_topk",
		CatalogSnapshot: "sha256:snap",
		PolicyRevision:  "sha256:pol",
		Client:          telemetry.Client{Name: "claude-code"},
		Payload: map[string]any{
			"skill_id":       "other-skill",
			"surface":        "skill_get",
			"resource_kind":  "entrypoint",
			"basis":          telemetry.LoadBasisServerObserved,
			"attribution":    "override",
			"topk_skill_ids": []string{"consumer-review", "other-skill"},
			"topk_matched":   []string{"1:operation", "2:trigger"},
			"topk_channels":  []string{"fts", "rules"},
		},
	})
	recorder.Flush(context.Background())

	usage := UsageService{Telemetry: telService}
	chains, err := usage.Chains(context.Background(), root, now.Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("Chains failed: %v", err)
	}
	found := false
	for _, c := range chains {
		if c.Kind == "override" && c.LoadedSkill == "other-skill" {
			found = true
			if c.TopKRank != 2 {
				t.Fatalf("expected TopKRank 2, got %d", c.TopKRank)
			}
			if c.TopKMatched != "trigger" {
				t.Fatalf("expected TopKMatched 'trigger', got %q", c.TopKMatched)
			}
		}
	}
	if !found {
		t.Fatal("expected override disagreement chain with top-k to be found")
	}
}
