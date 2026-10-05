package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func createTestSkill(t *testing.T, root, id, name string, active bool) {
	t.Helper()
	service := SkillService{}
	ctx := context.Background()
	preview, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID: id, Collection: "software", Name: name, Description: "Test skill " + name,
		Content: []byte("# " + name + "\n"), Routing: skill.RoutingInput{Triggers: []string{name}, NotFor: []string{"not-" + name}, MinScope: "multi_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	if active {
		previewAct, err := service.PreviewActivate(ctx, root, id, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.ConfirmSkillMutation(ctx, root, previewAct, previewAct.Confirmation.Confirmation.Pins); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUsageServiceFunnel(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	createTestSkill(t, root, "skill-a", "Skill A", true)
	createTestSkill(t, root, "skill-b", "Skill B", true)
	createTestSkill(t, root, "skill-dead", "Skill Dead", true)
	createTestSkill(t, root, "skill-60d", "Skill Sixty", true)

	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	clock := now

	recorder, err := (TelemetryService{Config: telemetry.Config{Clock: func() time.Time { return clock }}}).Open(root)
	if err != nil {
		t.Fatal(err)
	}

	makeEvent := func(id, eventType string, t time.Time) telemetry.Event {
		return telemetry.Event{
			Version:         telemetry.EventVersion,
			ID:              id,
			Type:            eventType,
			OccurredAt:      t,
			CatalogSnapshot: "test-snapshot",
			PolicyRevision:  "test-policy",
			Client:          telemetry.Client{Name: "test", Version: "1.0"},
		}
	}

	recordEvent := func(evt telemetry.Event) {
		recorder.Record(evt)
	}

	// 1. 60 days ago: activity on skill-60d
	clock = now.AddDate(0, 0, -60)
	evt60 := makeEvent("evt_rec_60d", telemetry.EventResolutionRecommended, clock)
	evt60.Payload = map[string]any{
		"top_skill_id":          "skill-60d",
		"recommended_skill_ids": []string{"skill-60d"},
	}
	recordEvent(evt60)

	// 2. Today: activity on skill-a and skill-b
	clock = now
	resEvt := makeEvent("evt_res_completed", telemetry.EventResolutionCompleted, clock)
	resEvt.ResolutionID = "res_today"
	resEvt.Payload = map[string]any{
		"status":                "resolved",
		"top_skill_id":          "skill-a",
		"recommended_skill_ids": []string{"skill-a"},
		"setup_state":           "ready",
	}
	recordEvent(resEvt)

	recEvtA := makeEvent("evt_rec_a", telemetry.EventResolutionRecommended, clock)
	recEvtA.ResolutionID = "res_today"
	recEvtA.Payload = map[string]any{
		"top_skill_id":          "skill-a",
		"recommended_skill_ids": []string{"skill-a"},
	}
	recordEvent(recEvtA)

	loadEvtA := makeEvent("evt_load_a", telemetry.EventSkillLoaded, clock)
	loadEvtA.ResolutionID = "res_today"
	loadEvtA.Payload = map[string]any{
		"skill_id":         "skill-a",
		"basis":            telemetry.LoadBasisServerObserved,
		"resource_kind":    "entrypoint",
		"surface":          "skill_get",
		"attribution":      "recommended",
		"first_activation": true,
	}
	recordEvent(loadEvtA)

	docEvtA1 := makeEvent("evt_doc_a1", telemetry.EventSkillDoctorChecked, clock)
	docEvtA1.Payload = map[string]any{
		"skill_id": "skill-a",
		"status":   "ready",
	}
	recordEvent(docEvtA1)

	docEvtA2 := makeEvent("evt_doc_a2", telemetry.EventSkillDoctorChecked, clock)
	docEvtA2.Payload = map[string]any{
		"skill_id": "skill-a",
		"status":   "setup_required",
	}
	recordEvent(docEvtA2)

	// skill-b: recommended, blocked by review, setup review required, override activation, setup failed feedback
	recEvtB := makeEvent("evt_rec_b", telemetry.EventResolutionRecommended, clock)
	recEvtB.ResolutionID = "res_b"
	recEvtB.Payload = map[string]any{
		"top_skill_id":          "skill-b",
		"recommended_skill_ids": []string{"skill-b"},
	}
	recordEvent(recEvtB)

	resEvtB := makeEvent("evt_res_b", telemetry.EventResolutionCompleted, clock)
	resEvtB.ResolutionID = "res_b"
	resEvtB.Payload = map[string]any{
		"status":                "resolved",
		"top_skill_id":          "skill-b",
		"recommended_skill_ids": []string{"skill-b"},
		"setup_state":           "review_required",
	}
	recordEvent(resEvtB)

	blockedEvtB := makeEvent("evt_blocked_b", telemetry.EventSkillLoaded, clock)
	blockedEvtB.ResolutionID = "res_b"
	blockedEvtB.Payload = map[string]any{
		"skill_id":         "skill-b",
		"basis":            telemetry.LoadBasisServerObserved,
		"resource_kind":    "entrypoint",
		"surface":          "skill_get",
		"status":           "review_required",
		"first_activation": false,
	}
	recordEvent(blockedEvtB)

	actOverrideB := makeEvent("evt_act_override_b", telemetry.EventSkillLoaded, clock)
	actOverrideB.ResolutionID = "res_b"
	actOverrideB.Payload = map[string]any{
		"skill_id":         "skill-b",
		"basis":            telemetry.LoadBasisServerObserved,
		"resource_kind":    "entrypoint",
		"surface":          "skill_get",
		"attribution":      "override",
		"first_activation": true,
	}
	recordEvent(actOverrideB)

	if err := recorder.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Feedback on skill-b: setup_failed
	if _, err := recorder.RecordFeedback(context.Background(), telemetry.Feedback{
		EventID:      "fb_b_setup_fail",
		ResolutionID: "res_b",
		Outcome:      "failed",
		ReasonCode:   telemetry.FeedbackReasonSetupFailed,
		SkillID:      "skill-b",
	}); err != nil {
		t.Fatal(err)
	}

	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	usageService := UsageService{}

	// Test 1: Default 30-day window
	rep30, err := usageService.Funnel(context.Background(), root, FunnelQuery{
		Until: now,
		Since: now.AddDate(0, 0, -30),
	})
	if err != nil {
		t.Fatalf("Funnel(30d): %v", err)
	}

	if rep30.Overall == nil {
		t.Fatal("expected Overall != nil for workspace query")
	}

	// Acceptance rate for skill-a
	var skillAFunnel *SkillFunnel
	for i := range rep30.Skills {
		if rep30.Skills[i].SkillID == "skill-a" {
			skillAFunnel = &rep30.Skills[i]
			break
		}
	}
	if skillAFunnel == nil {
		t.Fatal("skill-a not found in report.Skills")
	}
	if skillAFunnel.AcceptanceRate == nil || *skillAFunnel.AcceptanceRate != 1.0 {
		t.Fatalf("expected skill-a AcceptanceRate=1.0, got %v", skillAFunnel.AcceptanceRate)
	}
	// Doctor failure rate: 1 failure out of 2 runs -> 0.5
	if skillAFunnel.DoctorFailureRate == nil || *skillAFunnel.DoctorFailureRate != 0.5 {
		t.Fatalf("expected skill-a DoctorFailureRate=0.5, got %v", skillAFunnel.DoctorFailureRate)
	}

	// Null rates at zero denominators for skill-dead
	var skillDeadFunnel *SkillFunnel
	for i := range rep30.Skills {
		if rep30.Skills[i].SkillID == "skill-dead" {
			skillDeadFunnel = &rep30.Skills[i]
			break
		}
	}
	if skillDeadFunnel == nil {
		t.Fatal("skill-dead not found in report.Skills")
	}
	if skillDeadFunnel.AcceptanceRate != nil {
		t.Fatalf("expected nil AcceptanceRate for zero recommendations, got %v", *skillDeadFunnel.AcceptanceRate)
	}
	if skillDeadFunnel.DoctorFailureRate != nil {
		t.Fatalf("expected nil DoctorFailureRate for zero doctor runs, got %v", *skillDeadFunnel.DoctorFailureRate)
	}
	if skillDeadFunnel.SetupFailedRate != nil {
		t.Fatalf("expected nil SetupFailedRate for zero activations, got %v", *skillDeadFunnel.SetupFailedRate)
	}

	// Dead skills in 30-day window: skill-dead AND skill-60d (since 60d is outside the 30-day window)
	hasDead := false
	has60dDead := false
	for _, id := range rep30.DeadSkills {
		if id == "skill-dead" {
			hasDead = true
		}
		if id == "skill-60d" {
			has60dDead = true
		}
	}
	if !hasDead || !has60dDead {
		t.Fatalf("expected skill-dead and skill-60d in dead_skills for 30d window, got: %v", rep30.DeadSkills)
	}

	// Now check 90-day window: skill-60d should NOT be in dead_skills
	rep90, err := usageService.Funnel(context.Background(), root, FunnelQuery{
		Until: now,
		Since: now.AddDate(0, 0, -90),
	})
	if err != nil {
		t.Fatalf("Funnel(90d): %v", err)
	}
	for _, id := range rep90.DeadSkills {
		if id == "skill-60d" {
			t.Fatalf("skill-60d should NOT be in dead_skills for 90d window: %v", rep90.DeadSkills)
		}
	}

	// Test 2: Window clamp at 180 days
	repClamp, err := usageService.Funnel(context.Background(), root, FunnelQuery{
		Until: now,
		Since: now.AddDate(0, 0, -250),
	})
	if err != nil {
		t.Fatalf("Funnel(clamp): %v", err)
	}
	if repClamp.Window.Days != 181 {
		t.Fatalf("expected 181 days window after clamp, got %d", repClamp.Window.Days)
	}
	if repClamp.Window.Since != now.AddDate(0, 0, -180).Format("2006-01-02") {
		t.Fatalf("expected clamped since %s, got %s", now.AddDate(0, 0, -180).Format("2006-01-02"), repClamp.Window.Since)
	}

	// Test 3: Blocked by review list and recommended_never_activated exclusion
	hasBBlocked := false
	for _, id := range rep30.BlockedByReview {
		if id == "skill-b" {
			hasBBlocked = true
		}
	}
	if !hasBBlocked {
		t.Fatalf("expected skill-b in blocked_by_review: %v", rep30.BlockedByReview)
	}
	for _, id := range rep30.RecommendedNeverActivated {
		if id == "skill-b" {
			t.Fatalf("skill-b must NOT be in recommended_never_activated because blocked_by_review > 0: %v", rep30.RecommendedNeverActivated)
		}
	}

	// Test 4: Setup failed rate on skill-b
	var skillBFunnel *SkillFunnel
	for i := range rep30.Skills {
		if rep30.Skills[i].SkillID == "skill-b" {
			skillBFunnel = &rep30.Skills[i]
			break
		}
	}
	if skillBFunnel == nil {
		t.Fatal("skill-b not found in skills")
	}
	if skillBFunnel.SetupFailed != 1 {
		t.Fatalf("expected skill-b SetupFailed=1, got %d", skillBFunnel.SetupFailed)
	}
	// 1 activation (override), 1 setup_failed -> rate = 1.0
	if skillBFunnel.SetupFailedRate == nil || *skillBFunnel.SetupFailedRate != 1.0 {
		t.Fatalf("expected skill-b SetupFailedRate=1.0, got %v", skillBFunnel.SetupFailedRate)
	}

	// Test 5: --skill filter
	repSkillA, err := usageService.Funnel(context.Background(), root, FunnelQuery{
		Until:   now,
		Since:   now.AddDate(0, 0, -30),
		SkillID: "skill-a",
	})
	if err != nil {
		t.Fatalf("Funnel(skill-a): %v", err)
	}
	if repSkillA.Overall != nil {
		t.Fatal("expected Overall == nil for single skill query")
	}
	if repSkillA.Skill == nil || repSkillA.Skill.SkillID != "skill-a" {
		t.Fatalf("expected Skill.SkillID=skill-a, got %#v", repSkillA.Skill)
	}
	if len(repSkillA.Skills) != 1 || repSkillA.Skills[0].SkillID != "skill-a" {
		t.Fatalf("expected single element Skills with skill-a, got %#v", repSkillA.Skills)
	}
	if len(repSkillA.DeadSkills) != 0 || len(repSkillA.BlockedByReview) != 0 {
		t.Fatalf("expected lists to be empty/omitted for single skill query: %#v", repSkillA)
	}

	// Unknown skill -> error
	_, errUnknown := usageService.Funnel(context.Background(), root, FunnelQuery{
		Until:   now,
		Since:   now.AddDate(0, 0, -30),
		SkillID: "non-existent-skill",
	})
	if errUnknown == nil {
		t.Fatal("expected error for non-existent skill query")
	}
}
