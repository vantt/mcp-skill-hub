package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func rollupCounts(t *testing.T, recorder *Recorder, from, to string) map[string]int64 {
	t.Helper()
	rows, err := recorder.Rollups(t.Context(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Day+"|"+row.SkillID+"|"+row.Metric] = row.Count
	}
	return counts
}

func assertCounts(t *testing.T, got map[string]int64, want map[string]int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rollups = %v, want %v", got, want)
	}
	for key, count := range want {
		if got[key] != count {
			t.Fatalf("rollup %s = %d, want %d (all: %v)", key, got[key], count, got)
		}
	}
}

func measurementEvents(occurredAt time.Time) []Event {
	completed := validEvent(EventResolutionCompleted)
	completed.ID, completed.OccurredAt, completed.ResolutionID = "evt_completed", occurredAt, "res_rollup"
	completed.Payload = map[string]any{"status": "resolved", "top_skill_id": "alpha", "recommended_skill_ids": []string{"alpha", "beta"}, "setup_state": "setup_required"}

	recommended := validEvent(EventResolutionRecommended)
	recommended.ID, recommended.OccurredAt, recommended.ResolutionID = "evt_recommended", occurredAt, "res_rollup"
	recommended.Payload = map[string]any{"status": "resolved", "top_skill_id": "alpha", "recommended_skill_ids": []string{"alpha", "beta"}}

	failed := validEvent(EventResolutionFailed)
	failed.ID, failed.OccurredAt = "evt_failed", occurredAt
	failed.Payload = map[string]any{"status": "failed", "recommended_skill_ids": []string{}}

	loaded := validEvent(EventSkillLoaded)
	loaded.ID, loaded.OccurredAt, loaded.ResolutionID = "evt_loaded", occurredAt, "res_rollup"
	loaded.Payload = map[string]any{"skill_id": "alpha", "basis": LoadBasisServerObserved, "resource_kind": "entrypoint", "surface": "skill_get", "attribution": "recommended", "first_activation": true}

	reference := validEvent(EventSkillLoaded)
	reference.ID, reference.OccurredAt = "evt_reference", occurredAt
	reference.Payload = map[string]any{"skill_id": "alpha", "basis": LoadBasisServerObserved, "resource_kind": "reference", "surface": "resources_read", "attribution": "recommended"}

	doctor := validEvent(EventSkillDoctorChecked)
	doctor.ID, doctor.OccurredAt = "evt_doctor", occurredAt
	doctor.Payload = map[string]any{"skill_id": "alpha", "status": "setup_required", "reason_codes": []string{"missing_bin"}, "duration_ms": 4}

	native := validEvent(EventTranscriptToolObserved)
	native.ID, native.OccurredAt = "evt_native", occurredAt
	native.Payload = map[string]any{"tool": "Skill", "skill_id": "alpha", "source": TranscriptSourceClaude, "basis": TranscriptBasis, "resolved_before": false}

	mcpTool := validEvent(EventTranscriptToolObserved)
	mcpTool.ID, mcpTool.OccurredAt = "evt_transcript_get", occurredAt
	mcpTool.Payload = map[string]any{"tool": "mcp__skillhub__skill_get", "skill_id": "beta", "source": TranscriptSourceClaude, "basis": TranscriptBasis, "resolved_before": true}

	resolved := validEvent(EventTranscriptToolObserved)
	resolved.ID, resolved.OccurredAt = "evt_native_resolved", occurredAt
	resolved.Payload = map[string]any{"tool": "Skill", "source": TranscriptSourceClaude, "basis": TranscriptBasis, "resolved_before": true}

	return []Event{completed, recommended, failed, loaded, reference, doctor, native, mcpTool, resolved}
}

func TestRollupsCountEachInsertedEventOnceAndIgnoreReplays(t *testing.T) {
	now := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }})
	for _, event := range measurementEvents(now) {
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	if health := recorder.Health(); health.Rejected != 0 {
		t.Fatalf("measurement events rejected: %+v", health)
	}
	want := map[string]int64{
		"2026-10-04||resolution:resolved":                     1,
		"2026-10-04|alpha|setup:setup_required":               1,
		"2026-10-04|alpha|recommended:primary":                1,
		"2026-10-04|beta|recommended:supporting":              1,
		"2026-10-04||resolution:failed":                       1,
		"2026-10-04|alpha|load:entrypoint":                    1,
		"2026-10-04|alpha|activation:recommended":             1,
		"2026-10-04|alpha|load:reference":                     1,
		"2026-10-04|alpha|doctor:setup_required":              1,
		"2026-10-04|alpha|transcript:Skill":                   1,
		"2026-10-04|alpha|native:no_resolve":                  1,
		"2026-10-04|beta|transcript:mcp__skillhub__skill_get": 1,
		"2026-10-04||transcript:Skill":                        1,
		"2026-10-04||native:resolved_before":                  1,
	}
	assertCounts(t, rollupCounts(t, recorder, "", ""), want)

	// Replaying the same event IDs is ignored by the raw insert and must not
	// increment any rollup.
	for _, event := range measurementEvents(now) {
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	assertCounts(t, rollupCounts(t, recorder, "", ""), want)
}

func TestRollupsOutliveRawRetentionAndExpireAfterRollupRetention(t *testing.T) {
	now := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := now
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }, RollupRetention: 50 * 24 * time.Hour})

	stale := validEvent(EventResolutionFailed)
	stale.ID, stale.OccurredAt = "evt_thirty_five_days", now.Add(-35*24*time.Hour)
	stale.Payload = map[string]any{"status": "failed", "recommended_skill_ids": []string{}}
	expired := stale
	expired.ID, expired.OccurredAt = "evt_sixty_days", now.Add(-60*24*time.Hour)
	recorder.Record(stale)
	recorder.Record(expired)
	mustFlush(t, recorder)

	if count := rawEventCount(t, recorder.config, "evt_thirty_five_days"); count != 0 {
		t.Fatalf("raw event older than raw retention was kept: %d", count)
	}
	assertCounts(t, rollupCounts(t, recorder, "", ""), map[string]int64{"2026-08-30||resolution:failed": 1})

	// Advancing the clock past the rollup retention prunes the aggregate on the
	// next maintenance pass.
	mu.Lock()
	clock = now.Add(20 * 24 * time.Hour)
	mu.Unlock()
	mustFlush(t, recorder)
	assertCounts(t, rollupCounts(t, recorder, "", ""), map[string]int64{})
}

func TestDefaultRollupRetentionIsHalfAYear(t *testing.T) {
	now := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }})
	if recorder.config.RollupRetention != 180*24*time.Hour {
		t.Fatalf("default rollup retention = %s", recorder.config.RollupRetention)
	}
	kept := validEvent(EventResolutionFailed)
	kept.ID, kept.OccurredAt = "evt_kept", now.Add(-179*24*time.Hour)
	kept.Payload = map[string]any{"status": "failed", "recommended_skill_ids": []string{}}
	dropped := kept
	dropped.ID, dropped.OccurredAt = "evt_dropped", now.Add(-181*24*time.Hour)
	recorder.Record(kept)
	recorder.Record(dropped)
	mustFlush(t, recorder)
	assertCounts(t, rollupCounts(t, recorder, "", ""), map[string]int64{"2026-04-08||resolution:failed": 1})
	if _, err := Open(Config{Path: recorder.config.Path, RollupRetention: -time.Hour}); err == nil {
		t.Fatal("negative rollup retention was accepted")
	}
}

func TestPurgeClearsRollups(t *testing.T) {
	now := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }})
	for _, event := range measurementEvents(now) {
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	if len(rollupCounts(t, recorder, "", "")) == 0 {
		t.Fatal("expected rollups before purge")
	}
	if err := recorder.Purge(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rollupCounts(t, recorder, "", ""), map[string]int64{})
}

func TestRollupsAreExcludedFromPreviewAndFilterByDay(t *testing.T) {
	now := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }})
	for offset, id := range []string{"evt_day_0", "evt_day_1", "evt_day_2"} {
		event := validEvent(EventResolutionFailed)
		event.ID, event.OccurredAt = id, now.Add(-time.Duration(offset)*24*time.Hour)
		event.Payload = map[string]any{"status": "failed", "recommended_skill_ids": []string{}}
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	assertCounts(t, rollupCounts(t, recorder, "2026-10-03", "2026-10-03"), map[string]int64{"2026-10-03||resolution:failed": 1})
	assertCounts(t, rollupCounts(t, recorder, "2026-10-03", ""), map[string]int64{"2026-10-03||resolution:failed": 1, "2026-10-04||resolution:failed": 1})
	assertCounts(t, rollupCounts(t, recorder, "", "2026-10-02"), map[string]int64{"2026-10-02||resolution:failed": 1})

	preview, err := recorder.Preview(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Events != 3 || strings.Contains(string(preview.JSONL), "resolution:failed") {
		t.Fatalf("preview leaked rollups or lost events: events=%d", preview.Events)
	}

	for _, bounds := range [][2]string{{"2026-13-01", ""}, {"", "yesterday"}, {"2026-10-04", "2026-10-03"}, {"2026-10-4", ""}} {
		if _, err := recorder.Rollups(t.Context(), bounds[0], bounds[1]); err == nil {
			t.Fatalf("invalid rollup range %v was accepted", bounds)
		}
	}
	if health := recorder.Health(); health.State != "healthy" {
		t.Fatalf("invalid caller range degraded health: %+v", health)
	}
}

func TestFeedbackAfterLoadFlagAndTopSkillFallback(t *testing.T) {
	now := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }})
	resolution := validEvent(EventResolutionCompleted)
	resolution.ID, resolution.OccurredAt, resolution.ResolutionID = "evt_resolution", now, "res_after_load"
	resolution.Payload = map[string]any{"status": "resolved", "top_skill_id": "alpha", "recommended_skill_ids": []string{"alpha", "beta"}}
	recorder.Record(resolution)

	// Before any server-observed load: no after_load flag, and a report without
	// a skill falls back to the resolution's top skill.
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{EventID: "evt_reject", ResolutionID: "res_after_load", Outcome: "rejected", ReasonCode: FeedbackReasonUserRejected}); err != nil {
		t.Fatal(err)
	}
	// A host-reported load is not server-observed and must not set after_load.
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{EventID: "evt_host_load", ResolutionID: "res_after_load", Outcome: "loaded", SkillID: "beta"}); err != nil {
		t.Fatal(err)
	}

	load := validEvent(EventSkillLoaded)
	load.ID, load.OccurredAt, load.ResolutionID = "evt_server_load", now, "res_after_load"
	load.Payload = map[string]any{"skill_id": "beta", "basis": LoadBasisServerObserved, "resource_kind": "entrypoint", "surface": "skill_get"}
	recorder.Record(load)

	if _, err := recorder.RecordFeedback(t.Context(), Feedback{EventID: "evt_failed", ResolutionID: "res_after_load", Outcome: "failed", SkillID: "beta", Utility: "harmful", Basis: "user"}); err != nil {
		t.Fatal(err)
	}
	// Retrying the first report after the load stays idempotent even though the
	// server-derived flag would now differ.
	retry, err := recorder.RecordFeedback(t.Context(), Feedback{EventID: "evt_reject", ResolutionID: "res_after_load", Outcome: "rejected", ReasonCode: FeedbackReasonUserRejected})
	if err != nil || !retry.Deduplicated {
		t.Fatalf("retry after load = %+v, %v", retry, err)
	}

	preview, err := recorder.Preview(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	afterLoad := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(preview.JSONL), []byte{'\n'}) {
		var event storedEnvelope
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		flag, _ := event.Payload["after_load"].(bool)
		afterLoad[event.ID] = flag
	}
	for id, want := range map[string]bool{"evt_reject": false, "evt_host_load": false, "evt_failed": true, "evt_failed:utility": true} {
		if got, ok := afterLoad[id]; !ok || got != want {
			t.Fatalf("%s after_load = %v (present %v), want %v", id, got, ok, want)
		}
	}

	assertCounts(t, rollupCounts(t, recorder, "", ""), map[string]int64{
		"2026-10-04||resolution:resolved":              1,
		"2026-10-04|alpha|feedback:rejected":           1,
		"2026-10-04|alpha|feedback:negative":           1,
		"2026-10-04|beta|feedback:loaded":              1,
		"2026-10-04|beta|load:entrypoint":              1,
		"2026-10-04|beta|feedback:failed":              1,
		"2026-10-04|beta|feedback:negative":            1,
		"2026-10-04|beta|feedback:negative_after_load": 1,
	})
}

func TestSetupFailedFeedbackCountsOncePerSkill(t *testing.T) {
	now := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }})
	resolution := validEvent(EventResolutionCompleted)
	resolution.ID, resolution.OccurredAt, resolution.ResolutionID = "evt_resolution", now, "res_setup"
	resolution.Payload = map[string]any{"status": "resolved", "top_skill_id": "alpha", "recommended_skill_ids": []string{"alpha"}}
	recorder.Record(resolution)

	if !IsFeedbackReasonCode(FeedbackReasonSetupFailed) {
		t.Fatal("setup_failed must be an accepted feedback reason")
	}
	// The utility event carries the same reason code but must not count again.
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{EventID: "evt_setup", ResolutionID: "res_setup", Outcome: "failed", ReasonCode: FeedbackReasonSetupFailed, SkillID: "alpha", Utility: "harmful", Basis: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{EventID: "evt_other", ResolutionID: "res_setup", Outcome: "failed", ReasonCode: FeedbackReasonWorkflowFailed, SkillID: "alpha"}); err != nil {
		t.Fatal(err)
	}
	assertCounts(t, rollupCounts(t, recorder, "", ""), map[string]int64{
		"2026-10-04||resolution:resolved":        1,
		"2026-10-04|alpha|feedback:failed":       2,
		"2026-10-04|alpha|feedback:negative":     2,
		"2026-10-04|alpha|feedback:setup_failed": 1,
	})
}

func TestMeasurementEventPrivacyAllowlist(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	valid := measurementEvents(time.Now().UTC())
	var rejectedCases []Event
	for _, base := range valid[3:] { // loads, doctor, and transcript events
		unknown := base
		unknown.ID = ""
		unknown.Payload = clonePayload(base.Payload)
		unknown.Payload["prompt"] = "raw-user-text"
		rejectedCases = append(rejectedCases, unknown)

		nonToken := base
		nonToken.ID = ""
		nonToken.Payload = clonePayload(base.Payload)
		nonToken.Payload["skill_id"] = "contains a/path"
		rejectedCases = append(rejectedCases, nonToken)
	}
	enumCases := []struct {
		base  Event
		field string
		value any
	}{
		{valid[3], "resource_kind", "binary"},
		{valid[3], "surface", "shell"},
		{valid[3], "attribution", "guessed"},
		{valid[3], "first_activation", "yes"},
		{valid[5], "status", "maybe"},
		{valid[6], "source", "cursor"},
		{valid[6], "basis", "user"},
		{valid[6], "resolved_before", "no"},
		{valid[0], "setup_state", "broken"},
	}
	for _, item := range enumCases {
		event := item.base
		event.ID = ""
		event.Payload = clonePayload(item.base.Payload)
		event.Payload[item.field] = item.value
		rejectedCases = append(rejectedCases, event)
	}
	missingKind := valid[3]
	missingKind.ID = ""
	missingKind.Payload = clonePayload(valid[3].Payload)
	delete(missingKind.Payload, "resource_kind")
	missingAttribution := valid[3]
	missingAttribution.ID = ""
	missingAttribution.Payload = clonePayload(valid[3].Payload)
	delete(missingAttribution.Payload, "attribution")
	rejectedCases = append(rejectedCases, missingKind, missingAttribution)

	for _, event := range rejectedCases {
		recorder.Record(event)
	}
	if health := recorder.Health(); health.Rejected != uint64(len(rejectedCases)) {
		t.Fatalf("Rejected = %d, want %d (%s)", health.Rejected, len(rejectedCases), health.LastError)
	}
}

func clonePayload(payload map[string]any) map[string]any {
	clone := make(map[string]any, len(payload))
	for key, value := range payload {
		clone[key] = value
	}
	return clone
}
func TestBlockedLoadRollupAndFeedbackExclusion(t *testing.T) {
	now := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }})

	resolution := validEvent(EventResolutionCompleted)
	resolution.ID, resolution.OccurredAt, resolution.ResolutionID = "evt_res_blocked", now, "res_blocked"
	resolution.Payload = map[string]any{"status": "resolved", "top_skill_id": "alpha", "recommended_skill_ids": []string{"alpha"}}
	recorder.Record(resolution)

	// Blocked load
	blockedLoad := validEvent(EventSkillLoaded)
	blockedLoad.ID, blockedLoad.OccurredAt, blockedLoad.ResolutionID = "evt_blocked_load", now, "res_blocked"
	blockedLoad.Payload = map[string]any{
		"skill_id":      "alpha",
		"basis":         LoadBasisServerObserved,
		"status":        "review_required",
		"resource_kind": "entrypoint",
		"surface":       "skill_get",
		"attribution":   "recommended",
		"reason_codes":  []string{"content_review_required"},
	}
	recorder.Record(blockedLoad)

	// Feedback after blocked load only: must NOT have after_load
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID:      "evt_feedback_blocked",
		ResolutionID: "res_blocked",
		Outcome:      "rejected",
		ReasonCode:   FeedbackReasonUserRejected,
		SkillID:      "alpha",
	}); err != nil {
		t.Fatal(err)
	}

	counts := rollupCounts(t, recorder, "", "")
	assertCounts(t, counts, map[string]int64{
		"2026-10-04||resolution:resolved":          1,
		"2026-10-04|alpha|blocked:review_required": 1,
		"2026-10-04|alpha|feedback:rejected":       1,
		"2026-10-04|alpha|feedback:negative":       1,
	})

	// Check that feedback does NOT have after_load flag
	preview, err := recorder.Preview(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(preview.JSONL), []byte{'\n'}) {
		var event storedEnvelope
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		if event.ID == "evt_feedback_blocked" {
			flag, _ := event.Payload["after_load"].(bool)
			if flag {
				t.Fatalf("expected after_load=false after blocked load, got true")
			}
		}
	}

	// Semantic validation: blocked load with first_activation: true rejected
	invalidFirst := validEvent(EventSkillLoaded)
	invalidFirst.Payload = map[string]any{
		"skill_id":         "alpha",
		"basis":            LoadBasisServerObserved,
		"status":           "review_required",
		"resource_kind":    "entrypoint",
		"surface":          "skill_get",
		"attribution":      "recommended",
		"first_activation": true,
	}
	recorder.Record(invalidFirst)

	// Semantic validation: server-observed load with status != review_required rejected
	invalidStatus := validEvent(EventSkillLoaded)
	invalidStatus.Payload = map[string]any{
		"skill_id":      "alpha",
		"basis":         LoadBasisServerObserved,
		"status":        "ready",
		"resource_kind": "entrypoint",
		"surface":       "skill_get",
		"attribution":   "recommended",
	}
	recorder.Record(invalidStatus)

	if health := recorder.Health(); health.Rejected != 2 {
		t.Fatalf("expected 2 rejected events, got %d (%s)", health.Rejected, health.LastError)
	}
}

func TestNegativeFeedbackRollupDeduplication(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }})

	// Seed resolution so feedback can resolve top skill if needed, though we provide skill_id explicitly
	res := validEvent(EventResolutionCompleted)
	res.ID, res.OccurredAt, res.ResolutionID = "res_1", now, "res_1"
	res.Payload = map[string]any{"status": "resolved", "top_skill_id": "alpha", "recommended_skill_ids": []string{"alpha"}}
	recorder.Record(res)
	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	// 1. One report outcome: failed + utility: harmful -> feedback:negative = 1
	res1, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID:      "fb_failed_harmful",
		ResolutionID: "res_1",
		Outcome:      "failed",
		Utility:      "harmful",
		Basis:        "user",
		SkillID:      "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res1.Deduplicated {
		t.Fatal("expected Deduplicated=false")
	}

	counts := rollupCounts(t, recorder, "", "")
	if got := counts["2026-10-04|alpha|feedback:negative"]; got != 1 {
		t.Fatalf("expected feedback:negative=1 for failed+harmful, got %d (all: %v)", got, counts)
	}
	if got := counts["2026-10-04|alpha|feedback:failed"]; got != 1 {
		t.Fatalf("expected feedback:failed=1, got %d", got)
	}

	// Retry of the same report -> still 1
	resRetry, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID:      "fb_failed_harmful",
		ResolutionID: "res_1",
		Outcome:      "failed",
		Utility:      "harmful",
		Basis:        "user",
		SkillID:      "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resRetry.Deduplicated {
		t.Fatal("expected Deduplicated=true on retry")
	}
	counts = rollupCounts(t, recorder, "", "")
	if got := counts["2026-10-04|alpha|feedback:negative"]; got != 1 {
		t.Fatalf("expected feedback:negative=1 after retry, got %d", got)
	}

	// 2. outcome: completed + utility: harmful -> adds 1 feedback:negative (total 2)
	res2, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID:      "fb_completed_harmful",
		ResolutionID: "res_1",
		Outcome:      "completed",
		Utility:      "harmful",
		Basis:        "user",
		SkillID:      "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Deduplicated {
		t.Fatal("expected Deduplicated=false")
	}
	counts = rollupCounts(t, recorder, "", "")
	if got := counts["2026-10-04|alpha|feedback:negative"]; got != 2 {
		t.Fatalf("expected total feedback:negative=2 after completed+harmful, got %d (all: %v)", got, counts)
	}
	if got := counts["2026-10-04|alpha|feedback:completed"]; got != 1 {
		t.Fatalf("expected feedback:completed=1, got %d", got)
	}

	// 3. outcome: failed alone -> adds 1 feedback:negative (total 3)
	res3, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID:      "fb_failed_alone",
		ResolutionID: "res_1",
		Outcome:      "failed",
		SkillID:      "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res3.Deduplicated {
		t.Fatal("expected Deduplicated=false")
	}
	counts = rollupCounts(t, recorder, "", "")
	if got := counts["2026-10-04|alpha|feedback:negative"]; got != 3 {
		t.Fatalf("expected total feedback:negative=3 after failed alone, got %d (all: %v)", got, counts)
	}
	if got := counts["2026-10-04|alpha|feedback:failed"]; got != 2 {
		t.Fatalf("expected total feedback:failed=2, got %d", got)
	}
}

func TestToolsListBytesRollupMaxPerClient(t *testing.T) {
	temp := t.TempDir()
	recorder, err := Open(Config{
		Path:          temp + "/telemetry.db",
		WorkspaceRoot: temp,
		BufferSize:    256,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	// Event 1: client-a reports 1000 bytes
	e1 := validEvent(EventServerMetric)
	e1.ID, e1.OccurredAt, e1.Client = "evt_t1", now, Client{Name: "client-a"}
	e1.Payload = map[string]any{"metric_name": "tools_list_bytes", "metric_value": float64(1000)}
	recorder.Record(e1)
	// Event 2: client-a reports 2500 bytes (larger)
	e2 := validEvent(EventServerMetric)
	e2.ID, e2.OccurredAt, e2.Client = "evt_t2", now.Add(time.Minute), Client{Name: "client-a"}
	e2.Payload = map[string]any{"metric_name": "tools_list_bytes", "metric_value": float64(2500)}
	recorder.Record(e2)
	// Event 3: client-a reports 1500 bytes (smaller - should not reduce max)
	e3 := validEvent(EventServerMetric)
	e3.ID, e3.OccurredAt, e3.Client = "evt_t3", now.Add(2*time.Minute), Client{Name: "client-a"}
	e3.Payload = map[string]any{"metric_name": "tools_list_bytes", "metric_value": float64(1500)}
	recorder.Record(e3)
	// Event 4: client-b reports 3000 bytes
	e4 := validEvent(EventServerMetric)
	e4.ID, e4.OccurredAt, e4.Client = "evt_t4", now.Add(3*time.Minute), Client{Name: "client-b"}
	e4.Payload = map[string]any{"metric_name": "tools_list_bytes", "metric_value": float64(3000)}
	recorder.Record(e4)

	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	counts := rollupCounts(t, recorder, "", "")
	day := "2026-10-09"
	// client-a should be max (2500), not sum (5000), under empty skill_id
	if got := counts[day+"||tools_list_bytes:client-a"]; got != 2500 {
		t.Fatalf("expected client-a tools_list_bytes = 2500, got %d (all: %v)", got, counts)
	}
	// client-b should be 3000, under empty skill_id
	if got := counts[day+"||tools_list_bytes:client-b"]; got != 3000 {
		t.Fatalf("expected client-b tools_list_bytes = 3000, got %d (all: %v)", got, counts)
	}
}
