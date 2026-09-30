package telemetry

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrivacyAllowlistRejectsContentAndUnknownEvents(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	for _, field := range []string{"task", "conversation", "source", "path", "secret"} {
		event := validEvent(EventResolutionCompleted)
		event.Payload[field] = "raw-value"
		recorder.Record(event)
	}
	event := validEvent("routing.unlisted")
	recorder.Record(event)
	event = validEvent(EventResolutionCompleted)
	event.Payload["reason_codes"] = []string{"contains/a/path"}
	recorder.Record(event)
	if health := recorder.Health(); health.Rejected != 7 {
		t.Fatalf("Rejected = %d, want 7", health.Rejected)
	}
}

func TestDefaultEnvelopeAndEventFamilies(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	types := []string{
		EventResolutionRecommended, EventSkillActivated, EventSkillLoaded, EventSkillUsed,
		EventTaskCompleted, EventSkillAbandoned, EventSkillUtilityReported,
		EventCurationSessionCompleted, EventDistillRunFinalized, EventEvaluationRunCompleted,
	}
	for _, eventType := range types {
		event := validEvent(eventType)
		event.Payload = validPayload(eventType)
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	preview, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Events != len(types) {
		t.Fatalf("events = %d, want %d (skipped %d)", preview.Events, len(types), preview.Skipped)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(preview.JSONL)), "\n") {
		var envelope map[string]any
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatal(err)
		}
		privacy := envelope["privacy"].(map[string]any)
		if privacy["content_mode"] != ContentModeNone {
			t.Fatalf("content_mode = %v", privacy["content_mode"])
		}
		for _, prohibited := range []string{"task", "conversation", "source", "path", "secret"} {
			if strings.Contains(line, `"`+prohibited+`"`) {
				t.Fatalf("export contains prohibited field %q: %s", prohibited, line)
			}
		}
	}
}

func TestBufferExhaustionIsNonBlocking(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	originalHook := initializeBeforeOpen
	initializeBeforeOpen = func() error {
		close(started)
		<-release
		return nil
	}
	defer func() { initializeBeforeOpen = originalHook }()

	recorder, err := Open(Config{Path: filepath.Join(t.TempDir(), "telemetry.db"), BufferSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	start := time.Now()
	for i := 0; i < 10_000; i++ {
		recorder.Record(validEvent(EventResolutionStarted))
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Record blocked for %v", elapsed)
	}
	close(release)
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.healthSnapshot().Dropped == 0 {
		t.Fatal("expected buffer drops")
	}
}

func TestDatabaseCorruptionDegradesWithoutRecordError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder, err := Open(Config{Path: path, BufferSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(validEvent(EventResolutionCompleted))
	if err := recorder.Flush(context.Background()); err == nil {
		t.Fatal("Flush succeeded for corrupt database")
	}
	health := recorder.Health()
	if health.State != "degraded" || health.Errors == 0 || health.Dropped == 0 {
		t.Fatalf("unexpected health: %+v", health)
	}
	if err := recorder.Purge(context.Background()); err != nil {
		t.Fatalf("Purge did not recover corrupt store: %v", err)
	}
	recorder.Record(validEvent(EventResolutionCompleted))
	mustFlush(t, recorder)
	mustClose(t, recorder)
}

func TestRejectsEscapingAndSymlinkedDatabasePaths(t *testing.T) {
	root := t.TempDir()
	if _, err := Open(Config{WorkspaceRoot: root, Path: filepath.Join(t.TempDir(), "outside.db")}); err == nil {
		t.Fatal("accepted database path outside workspace root")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "runtime")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Open(Config{WorkspaceRoot: root, Path: filepath.Join(root, "runtime", "telemetry.db")}); err == nil {
		t.Fatal("accepted symlinked runtime directory")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside directory changed: entries=%v err=%v", entries, err)
	}
}

func TestRetentionByAgeAndSize(t *testing.T) {
	now := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }, Retention: time.Hour, MaxSizeBytes: 1_500})
	old := validEvent(EventResolutionCompleted)
	old.ID, old.OccurredAt = "evt_old", now.Add(-2*time.Hour)
	recorder.Record(old)
	for i := 0; i < 20; i++ {
		event := validEvent(EventResolutionCompleted)
		event.ID = fmt.Sprintf("evt_%03d", i)
		event.OccurredAt = now.Add(time.Duration(i) * time.Nanosecond)
		event.Payload["reason_codes"] = []string{strings.Repeat("a", 200)}
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	preview, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(preview.JSONL), "evt_old") {
		t.Fatal("age retention kept expired event")
	}
	if preview.Events >= 20 {
		t.Fatalf("size retention did not trim events: %d", preview.Events)
	}
}

func TestDormantExpiredRecordsAreMaintainedBeforeReads(t *testing.T) {
	now := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	operations := map[string]func(*testing.T, *Recorder){
		"health": func(t *testing.T, recorder *Recorder) { _ = recorder.Health() },
		"preview": func(t *testing.T, recorder *Recorder) {
			preview, err := recorder.Preview(t.Context(), 0)
			if err != nil || preview.Events != 0 {
				t.Fatalf("preview = %+v, %v", preview, err)
			}
		},
		"export": func(t *testing.T, recorder *Recorder) {
			result, err := recorder.Export(t.Context(), filepath.Join(t.TempDir(), "events.jsonl"))
			if err != nil || result.Events != 0 {
				t.Fatalf("export = %+v, %v", result, err)
			}
		},
		"promotion": func(t *testing.T, recorder *Recorder) {
			if _, err := recorder.PromotionDraft(t.Context(), "res_dormant"); !errors.Is(err, ErrPromotionNotFound) {
				t.Fatalf("promotion error = %v", err)
			}
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }, Retention: time.Hour})
			mustFlush(t, recorder)
			insertRawEvent(t, recorder.config, storedEnvelope{
				Version: EventVersion, ID: "evt_dormant", Type: EventResolutionCompleted,
				OccurredAt: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), ResolutionID: "res_dormant",
				CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: Client{Name: "test"},
				Privacy: privacyEnvelope{ContentMode: ContentModeNone, RedactionVersion: RedactionVersion},
				Payload: map[string]any{"status": "resolved", "top_skill_id": "code-review"},
			})
			operation(t, recorder)
			if count := rawEventCount(t, recorder.config, "evt_dormant"); count != 0 {
				t.Fatalf("expired row remains physically stored: %d", count)
			}
		})
	}
}

func TestInitializationAppliesRetentionAndBoundsPhysicalMaintenance(t *testing.T) {
	oldNow := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	root := t.TempDir()
	path := filepath.Join(root, "runtime", "telemetry.db")
	first := newTestRecorder(t, Config{WorkspaceRoot: root, Path: path, Clock: func() time.Time { return oldNow }, Retention: time.Hour})
	event := validEvent(EventResolutionCompleted)
	event.ID, event.OccurredAt = "evt_dormant_init", oldNow
	first.Record(event)
	mustClose(t, first)

	now := oldNow.Add(2 * time.Hour)
	second, err := Open(Config{WorkspaceRoot: root, Path: path, Clock: func() time.Time { return now }, Retention: time.Hour, MaxSizeBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for second.healthSnapshot().State == "starting" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state := second.healthSnapshot().State; state != "healthy" {
		t.Fatalf("initialization state = %s", state)
	}
	if count := rawEventCount(t, second.config, "evt_dormant_init"); count != 0 {
		t.Fatalf("initialization retained expired row: %d", count)
	}
	before, err := databaseFilesSize(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 300; index++ {
		insertRawEvent(t, second.config, storedEnvelope{
			Version: EventVersion, ID: fmt.Sprintf("evt_size_%03d", index), Type: EventResolutionCompleted,
			OccurredAt: now.Format(time.RFC3339Nano), CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy",
			Client: Client{Name: "test"}, Privacy: privacyEnvelope{ContentMode: ContentModeNone, RedactionVersion: RedactionVersion},
			Payload: map[string]any{"status": "resolved", "reason_codes": []string{strings.Repeat("a", 200)}},
		})
	}
	inflated, err := databaseFilesSize(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Health()
	after, err := databaseFilesSize(path)
	if err != nil {
		t.Fatal(err)
	}
	if inflated <= before || after >= inflated || after > before+(64<<10) {
		t.Fatalf("physical maintenance was not bounded: baseline=%d inflated=%d after=%d", before, inflated, after)
	}
	if logical := rawLogicalSize(t, second.config); logical > second.config.MaxSizeBytes {
		t.Fatalf("logical size = %d, maximum = %d", logical, second.config.MaxSizeBytes)
	}
	mustClose(t, second)
}

func TestPurgeIsAnchoredAgainstParentSwapAndFaults(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	recorder := newTestRecorder(t, Config{WorkspaceRoot: root, Path: filepath.Join(root, "runtime", "telemetry.db")})
	event := validEvent(EventResolutionCompleted)
	event.ID = "evt_preserved"
	recorder.Record(event)
	mustFlush(t, recorder)

	originalHook := purgeBeforeRemove
	defer func() { purgeBeforeRemove = originalHook }()
	injected := errors.New("injected purge failure")
	purgeBeforeRemove = func() error { return injected }
	if err := recorder.Purge(t.Context()); !errors.Is(err, injected) {
		t.Fatalf("purge fault error = %v", err)
	}
	purgeBeforeRemove = originalHook
	if count := rawEventCount(t, recorder.config, "evt_preserved"); count != 1 {
		t.Fatalf("faulted purge changed store: %d", count)
	}

	outsideDatabase := filepath.Join(outside, "telemetry.db")
	if err := os.WriteFile(outsideDatabase, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(root, "runtime")
	originalRuntime := filepath.Join(root, "runtime-original")
	swapped := false
	purgeBeforeRemove = func() error {
		if err := os.Rename(runtimePath, originalRuntime); err != nil {
			return err
		}
		if err := os.Symlink(outside, runtimePath); err != nil {
			_ = os.Rename(originalRuntime, runtimePath)
			return err
		}
		swapped = true
		return nil
	}
	purgeErr := recorder.Purge(t.Context())
	purgeBeforeRemove = originalHook
	if !swapped {
		t.Skipf("runtime swap unavailable: %v", purgeErr)
	}
	if purgeErr == nil {
		t.Fatal("purge succeeded through swapped runtime symlink")
	}
	contents, readErr := os.ReadFile(outsideDatabase)
	if readErr != nil || string(contents) != "outside" {
		t.Fatalf("outside database changed: %q, %v", contents, readErr)
	}
	if removeErr := os.Remove(runtimePath); removeErr != nil {
		t.Fatal(removeErr)
	}
	if renameErr := os.Rename(originalRuntime, runtimePath); renameErr != nil {
		t.Fatal(renameErr)
	}
}

func TestCloseCanRetryAfterCanceledFullQueue(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	originalHook := initializeBeforeOpen
	initializeBeforeOpen = func() error {
		close(started)
		<-release
		return nil
	}
	defer func() { initializeBeforeOpen = originalHook }()
	recorder, err := Open(Config{Path: filepath.Join(t.TempDir(), "telemetry.db"), BufferSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	recorder.Record(validEvent(EventResolutionStarted))
	blocked := make(chan struct{})
	originalCloseHook := closeQueueBlocked
	var blockedOnce sync.Once
	closeQueueBlocked = func() { blockedOnce.Do(func() { close(blocked) }) }
	defer func() { closeQueueBlocked = originalCloseHook }()
	ctx, cancel := context.WithCancel(context.Background())
	closeResult := make(chan error, 1)
	go func() { closeResult <- recorder.Close(ctx) }()
	<-blocked
	start := time.Now()
	recorder.Record(validEvent(EventResolutionStarted))
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Record blocked behind Close for %v", elapsed)
	}
	cancel()
	if err := <-closeResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("first close error = %v", err)
	}
	if recorder.closed {
		t.Fatal("canceled close consumed shutdown state before it was queued")
	}
	close(release)
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer closeCancel()
	if err := recorder.Close(closeCtx); err != nil {
		t.Fatalf("retry close: %v", err)
	}
	select {
	case <-recorder.done:
	case <-time.After(time.Second):
		t.Fatal("writer goroutine remained live after successful close")
	}
}

func TestPurgePreviewAndAtomicExport(t *testing.T) {
	now := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	var sequence atomic.Uint64
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }, ID: func() (string, error) { return fmt.Sprintf("evt_%03d", sequence.Add(1)), nil }})
	for _, eventType := range []string{EventSkillUsed, EventResolutionRecommended} {
		event := validEvent(eventType)
		event.Payload = validPayload(eventType)
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	first, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.JSONL) != string(second.JSONL) {
		t.Fatal("preview is not deterministic")
	}
	output := filepath.Join(t.TempDir(), "bundle.jsonl")
	if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := recorder.Export(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(first.JSONL) || result.Events != 2 || result.Version != ExportVersion {
		t.Fatalf("unexpected export: %+v", result)
	}
	if _, err := recorder.Export(context.Background(), recorder.config.Path); err == nil {
		t.Fatal("allowed export over database")
	}
	if err := recorder.Purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	empty, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Events != 0 || len(empty.JSONL) != 0 {
		t.Fatalf("purge left events: %+v", empty)
	}
}

func TestPromotionDraftIsSanitizedDeterministicAndIncomplete(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	completed := validEvent(EventResolutionCompleted)
	completed.ID = "evt_completed"
	completed.ResolutionID = "res_exact"
	completed.SessionIDHash = "hmac:private-session"
	completed.RequestID = "req_private"
	completed.Payload = map[string]any{
		"status": "resolved", "top_skill_id": "code-review",
		"reason_codes": []string{"trigger_match", "fact_match"},
		"operation":    "review", "candidate_count": 4,
	}
	recorder.Record(completed)
	used := validEvent(EventSkillUsed)
	used.ID = "evt_used"
	used.ResolutionID = "res_exact"
	used.Payload = map[string]any{"skill_id": "code-review", "status": "completed"}
	recorder.Record(used)
	mustFlush(t, recorder)

	before, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	first, err := recorder.PromotionDraft(context.Background(), "res_exact")
	if err != nil {
		t.Fatal(err)
	}
	second, err := recorder.PromotionDraft(context.Background(), "res_exact")
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := first.JSON()
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := second.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("promotion output is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
	if first.CatalogSnapshot != "sha256:catalog" || first.PolicyRevision != "sha256:policy" || !first.ReviewRequired {
		t.Fatalf("missing pinned identity or review gate: %+v", first)
	}
	if first.Observed.Status != "resolved" || first.Observed.TopSkillID != "code-review" || strings.Join(first.Observed.ReasonCodes, ",") != "fact_match,trigger_match" {
		t.Fatalf("unexpected observation: %+v", first.Observed)
	}
	if strings.Join(first.SourceEventIDs, ",") != "evt_completed,evt_used" {
		t.Fatalf("unexpected sources: %v", first.SourceEventIDs)
	}
	if !first.CaseTemplate.Incomplete || len(first.CaseTemplate.RequiredHumanFields) == 0 || first.CaseTemplate.Request.Task.Description != "" || len(first.CaseTemplate.Expected.AcceptablePrimary) != 0 || first.CaseTemplate.Expected.Rationale != "" {
		t.Fatalf("case template was populated without human review: %+v", first.CaseTemplate)
	}
	serialized := string(firstJSON)
	for _, secret := range []string{"private-session", "req_private", `"operation":"review"`, `"candidate_count":4`, `"skill_id":"code-review"`} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("promotion leaked omitted telemetry %q: %s", secret, serialized)
		}
	}
	for _, report := range []string{"client", "privacy", "request_id", "session_id_hash", "payload.operation", "payload.candidate_count", "payload.skill_id"} {
		if !contains(first.Sanitization.RemovedFields, report) {
			t.Fatalf("removed field report omits %q: %v", report, first.Sanitization.RemovedFields)
		}
	}
	after, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(before.JSONL) != string(after.JSONL) {
		t.Fatal("promotion mutated telemetry events")
	}
}

func TestPromotionDraftMissingAmbiguousAndInvalid(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	if _, err := recorder.PromotionDraft(context.Background(), "res_missing"); !errors.Is(err, ErrPromotionNotFound) {
		t.Fatalf("missing resolution error = %v", err)
	}
	if _, err := recorder.PromotionDraft(context.Background(), "not a token"); err == nil {
		t.Fatal("accepted invalid resolution_id")
	}
	for index, skill := range []string{"code-review", "architecture-review"} {
		event := validEvent(EventResolutionCompleted)
		event.ID = fmt.Sprintf("evt_conflict_%d", index)
		event.ResolutionID = "res_conflict"
		event.Payload["top_skill_id"] = skill
		event.Payload["recommended_skill_ids"] = []string{skill}
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	if _, err := recorder.PromotionDraft(context.Background(), "res_conflict"); !errors.Is(err, ErrPromotionAmbiguous) {
		t.Fatalf("ambiguous resolution error = %v", err)
	}
}

func TestPromotionDraftSkipsMalformedMatchingRows(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	event := validEvent(EventResolutionCompleted)
	event.ID = "evt_valid"
	event.ResolutionID = "res_malformed"
	event.Payload["top_skill_id"] = "code-review"
	event.Payload["recommended_skill_ids"] = []string{"code-review"}
	recorder.Record(event)
	mustFlush(t, recorder)

	database, err := openDatabase(context.Background(), recorder.config)
	if err != nil {
		t.Fatal(err)
	}
	_, insertErr := database.ExecContext(context.Background(), `INSERT INTO telemetry_events(id,occurred_at,kind,resolution_id,payload_json) VALUES(?,?,?,?,?)`,
		"evt_malformed", "2026-09-29T08:00:00Z", EventResolutionCompleted, "res_malformed", `{"resolution_id":`)
	if insertErr != nil {
		t.Fatal(insertErr)
	}
	redacted := storedEnvelope{
		Version: EventVersion, ID: "evt_redacted", Type: EventResolutionCompleted, OccurredAt: "2026-09-29T08:00:01Z",
		ResolutionID: "res_malformed", CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy",
		Client: Client{Name: "test"}, Privacy: privacyEnvelope{ContentMode: "redacted", RedactionVersion: RedactionVersion},
		Payload: map[string]any{"status": "resolved", "top_skill_id": "architecture-review"},
	}
	redactedJSON, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	_, insertErr = database.ExecContext(context.Background(), `INSERT INTO telemetry_events(id,occurred_at,kind,resolution_id,payload_json) VALUES(?,?,?,?,?)`,
		redacted.ID, redacted.OccurredAt, redacted.Type, redacted.ResolutionID, string(redactedJSON))
	closeErr := database.Close()
	if insertErr != nil {
		t.Fatal(insertErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	draft, err := recorder.PromotionDraft(context.Background(), "res_malformed")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Sanitization.MalformedEventsSkipped != 1 || draft.Sanitization.UnsupportedContentModeEventsSkipped != 1 || strings.Join(draft.SourceEventIDs, ",") != "evt_valid" {
		t.Fatalf("invalid rows were not safely skipped: %+v", draft)
	}
	encoded, err := draft.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"evt_malformed", "evt_redacted", "architecture-review"} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("invalid source data %q leaked: %s", prohibited, encoded)
		}
	}
}

func TestPromotionDraftBoundsResolutionEvents(t *testing.T) {
	recorder := newTestRecorder(t, Config{BufferSize: maxPromotionEvents + 2})
	for index := 0; index <= maxPromotionEvents; index++ {
		event := validEvent(EventResolutionCompleted)
		event.ID = fmt.Sprintf("evt_bounded_%03d", index)
		event.ResolutionID = "res_too_large"
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	if _, err := recorder.PromotionDraft(context.Background(), "res_too_large"); !errors.Is(err, ErrPromotionTooLarge) {
		t.Fatalf("oversized resolution error = %v", err)
	}
}

func TestRecordFeedbackMapsFunnelEventsAndDerivesPinnedEnvelope(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	resolution := validEvent(EventResolutionCompleted)
	resolution.ID = "evt_resolution_source"
	resolution.ResolutionID = "res_feedback"
	resolution.Client = Client{Name: "resolver-client", Version: "2"}
	resolution.Payload["top_skill_id"] = "recommended-skill"
	resolution.Payload["recommended_skill_ids"] = []string{"recommended-skill"}
	recorder.Record(resolution)

	expectedTypes := map[string]string{
		"activated": EventSkillActivated,
		"rejected":  EventActivationRejected,
		"used":      EventSkillUsed,
		"abandoned": EventSkillAbandoned,
		"completed": EventTaskOutcomeReported,
		"failed":    EventTaskOutcomeReported,
	}
	for outcome := range expectedTypes {
		feedback := Feedback{
			EventID: "evt_feedback_" + outcome, ResolutionID: "res_feedback", Outcome: outcome,
			ReasonCode: FeedbackReasonHostReport, SkillID: "recommended-skill",
		}
		if outcome == "completed" {
			feedback.Utility, feedback.Basis = "helpful", "user"
		}
		result, err := recorder.RecordFeedback(t.Context(), feedback)
		if err != nil || result.Deduplicated {
			t.Fatalf("record %s feedback = %+v, %v", outcome, result, err)
		}
		if want := 1; feedback.Utility != "" {
			want = 2
			if result.Events != want {
				t.Fatalf("%s event count = %d, want %d", outcome, result.Events, want)
			}
		}
	}

	preview, err := recorder.Preview(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]storedEnvelope)
	for _, line := range bytes.Split(bytes.TrimSpace(preview.JSONL), []byte{'\n'}) {
		var event storedEnvelope
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		seen[event.ID] = event
		if event.Type == "skill.feedback" {
			t.Fatal("legacy skill.feedback row was exported")
		}
	}
	for outcome, eventType := range expectedTypes {
		event := seen["evt_feedback_"+outcome]
		if event.Type != eventType || event.ResolutionID != "res_feedback" || event.CatalogSnapshot != resolution.CatalogSnapshot || event.PolicyRevision != resolution.PolicyRevision || event.Client != resolution.Client {
			t.Fatalf("%s feedback envelope = %+v", outcome, event)
		}
		if event.Payload["status"] != outcome || event.Payload["skill_id"] != "recommended-skill" {
			t.Fatalf("%s feedback payload = %#v", outcome, event.Payload)
		}
	}
	utility := seen["evt_feedback_completed:utility"]
	if utility.Type != EventSkillUtilityReported || utility.Payload["utility"] != "helpful" || utility.Payload["basis"] != "user" {
		t.Fatalf("utility event = %+v", utility)
	}

	retry, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID: "evt_feedback_completed", ResolutionID: "res_feedback", Outcome: "completed",
		ReasonCode: FeedbackReasonHostReport, SkillID: "recommended-skill", Utility: "helpful", Basis: "user",
	})
	if err != nil || !retry.Deduplicated {
		t.Fatalf("deduplicated feedback = %+v, %v", retry, err)
	}
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID: "evt_feedback_completed", ResolutionID: "res_feedback", Outcome: "completed",
		ReasonCode: FeedbackReasonHostReport, SkillID: "recommended-skill",
	}); !errors.Is(err, ErrFeedbackConflict) {
		t.Fatalf("utility semantic conflict = %v", err)
	}
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{
		EventID: "evt_feedback_used", ResolutionID: "res_feedback", Outcome: "failed",
		ReasonCode: FeedbackReasonHostReport, SkillID: "recommended-skill",
	}); !errors.Is(err, ErrFeedbackConflict) {
		t.Fatalf("outcome semantic conflict = %v", err)
	}

	draft, err := recorder.PromotionDraft(t.Context(), "res_feedback")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"evt_feedback_activated", "evt_feedback_completed", "evt_feedback_completed:utility"} {
		if !contains(draft.SourceEventIDs, id) {
			t.Fatalf("promotion omitted indexed feedback %q: %v", id, draft.SourceEventIDs)
		}
	}
}

func TestRecordFeedbackRequiresValidatedExactResolution(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{EventID: "evt_unknown", ResolutionID: "res_unknown", Outcome: "used"}); !errors.Is(err, ErrFeedbackResolutionNotFound) {
		t.Fatalf("unknown resolution error = %v", err)
	}

	mustFlush(t, recorder)
	insertRawEvent(t, recorder.config, storedEnvelope{
		Version: EventVersion, ID: "evt_mismatched", Type: EventResolutionCompleted,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano), ResolutionID: "res_other",
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: Client{Name: "test"},
		Privacy: privacyEnvelope{ContentMode: ContentModeNone, RedactionVersion: RedactionVersion},
		Payload: map[string]any{"status": "resolved"},
	})
	database, err := openDatabase(t.Context(), recorder.config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), `UPDATE telemetry_events SET resolution_id = 'res_exact' WHERE id = 'evt_mismatched'`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.RecordFeedback(t.Context(), Feedback{EventID: "evt_invalid_source", ResolutionID: "res_exact", Outcome: "used"}); !errors.Is(err, ErrFeedbackResolutionNotFound) {
		t.Fatalf("mismatched indexed envelope error = %v", err)
	}
}

func TestConcurrentRecordAndClose(t *testing.T) {
	recorder := newTestRecorder(t, Config{BufferSize: 64})
	var wait sync.WaitGroup
	for worker := 0; worker < 20; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for item := 0; item < 200; item++ {
				event := validEvent(EventSkillUsed)
				event.ID = fmt.Sprintf("evt_%02d_%03d", worker, item)
				event.Payload = validPayload(EventSkillUsed)
				recorder.Record(event)
			}
		}(worker)
	}
	wait.Wait()
	mustClose(t, recorder)
	health := recorder.Health()
	if health.State != "closed" {
		t.Fatalf("state = %s", health.State)
	}
	if health.Accepted != health.Written {
		t.Fatalf("accepted=%d written=%d dropped=%d", health.Accepted, health.Written, health.Dropped)
	}
	droppedBefore := health.Dropped
	recorder.Record(validEvent(EventResolutionStarted))
	if recorder.Health().Dropped != droppedBefore+1 {
		t.Fatal("Record after close was not counted as dropped")
	}
}

func TestLegacyRawFeedbackRowsAreRemoved(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	mustFlush(t, recorder)
	database, err := openDatabase(t.Context(), recorder.config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), `DELETE FROM telemetry_meta WHERE key = 'legacy_skill_feedback_removed'`); err != nil {
		t.Fatal(err)
	}
	_, insertErr := database.ExecContext(t.Context(), `INSERT INTO telemetry_events(id,occurred_at,kind,resolution_id,payload_json) VALUES(?,?,?,?,?)`,
		"evt_legacy", time.Now().UTC().Format(time.RFC3339Nano), "skill.feedback", "res_legacy", `{"resolution_id":"res_legacy","outcome":"used","raw":"content"}`)
	closeErr := database.Close()
	if insertErr != nil || closeErr != nil {
		t.Fatalf("insert legacy row: %v; close: %v", insertErr, closeErr)
	}
	preview, err := recorder.Preview(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(preview.JSONL), "evt_legacy") {
		t.Fatalf("legacy feedback was exported: %s", preview.JSONL)
	}
	direct, err := sql.Open("sqlite", sqliteURL(recorder.config.Path))
	if err != nil {
		t.Fatal(err)
	}
	defer direct.Close()
	var count int
	if err := direct.QueryRowContext(t.Context(), `SELECT count(*) FROM telemetry_events WHERE kind = 'skill.feedback'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy raw feedback rows = %d, err=%v", count, err)
	}
}

func TestDeleteAndRecreateDatabase(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	first := validEvent(EventResolutionCompleted)
	first.ID = "evt_first"
	recorder.Record(first)
	mustFlush(t, recorder)
	for _, suffix := range []string{"-wal", "-shm", ""} {
		if err := os.Remove(recorder.config.Path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	second := validEvent(EventResolutionCompleted)
	second.ID = "evt_second"
	recorder.Record(second)
	mustFlush(t, recorder)
	preview, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Events != 1 || !strings.Contains(string(preview.JSONL), "evt_second") {
		t.Fatalf("database was not recreated: %s", preview.JSONL)
	}
}

func TestIDFailureAndInvalidConfiguration(t *testing.T) {
	if _, err := Open(Config{}); err == nil {
		t.Fatal("accepted empty path")
	}
	recorder := newTestRecorder(t, Config{ID: func() (string, error) { return "", errors.New("no entropy") }})
	recorder.Record(validEvent(EventResolutionStarted))
	if health := recorder.Health(); health.Errors == 0 || health.Dropped == 0 {
		t.Fatalf("unexpected health: %+v", health)
	}
}

func newTestRecorder(t *testing.T, config Config) *Recorder {
	t.Helper()
	if config.Path == "" {
		config.Path = filepath.Join(t.TempDir(), "telemetry.db")
	}
	if config.BufferSize == 0 {
		config.BufferSize = 128
	}
	if config.ID == nil {
		var sequence atomic.Uint64
		config.ID = func() (string, error) { return fmt.Sprintf("evt_auto_%d", sequence.Add(1)), nil }
	}
	recorder, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close(context.Background()) })
	return recorder
}

func validEvent(eventType string) Event {
	return Event{Type: eventType, CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: Client{Name: "test", Version: "1.0"}, Payload: validPayload(eventType)}
}

func validPayload(eventType string) map[string]any {
	switch eventType {
	case EventResolutionStarted, EventResolutionRecommended, EventResolutionCompleted, EventResolutionFailed:
		return map[string]any{"status": "resolved", "candidate_count": 2, "recommended_skill_ids": []string{}, "reason_codes": []string{"trigger_match"}, "stage_ms": map[string]int64{"total": 3}}
	case EventSkillActivated, EventActivationApproved, EventActivationRejected, EventSkillLoaded, EventSkillUsed, EventSkillAbandoned:
		return map[string]any{"skill_id": "code-review", "status": "completed"}
	case EventTaskCompleted:
		return map[string]any{"status": "completed", "skill_count": 1}
	case EventSkillUtilityReported:
		return map[string]any{"skill_id": "code-review", "utility": "helpful", "basis": "user"}
	case EventCurationSessionCompleted:
		return map[string]any{"status": "completed", "basis": CurationBasisHostReported, "turns_to_next_action": 1, "auto_finalized": true, "routine_git_noise": 0}
	case EventDistillRunFinalized:
		return map[string]any{"run_id": "run_1", "status": "finalized", "cursor_advanced": true, "observation_count": 2}
	case EventEvaluationRunCompleted:
		return map[string]any{"run_id": "eval_1", "suite_id": "core-v1", "variant": "bm25", "status": "completed", "case_count": 10, "seed": 42}
	default:
		return map[string]any{}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func insertRawEvent(t *testing.T, config Config, event storedEnvelope) {
	t.Helper()
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	database, err := openDatabase(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	_, insertErr := database.ExecContext(t.Context(), `INSERT INTO telemetry_events(id,occurred_at,kind,resolution_id,payload_json) VALUES(?,?,?,?,?)`, event.ID, event.OccurredAt, event.Type, nullIfEmpty(event.ResolutionID), string(encoded))
	closeErr := database.Close()
	if insertErr != nil {
		t.Fatal(insertErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

func rawEventCount(t *testing.T, config Config, id string) int {
	t.Helper()
	database, err := openDatabase(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM telemetry_events WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func rawLogicalSize(t *testing.T, config Config) int64 {
	t.Helper()
	database, err := openDatabase(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var size int64
	if err := database.QueryRowContext(t.Context(), `SELECT COALESCE(SUM(length(id)+length(occurred_at)+length(kind)+COALESCE(length(resolution_id),0)+length(payload_json)),0) FROM telemetry_events`).Scan(&size); err != nil {
		t.Fatal(err)
	}
	return size
}

func mustFlush(t *testing.T, recorder *Recorder) {
	t.Helper()
	if err := recorder.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func mustClose(t *testing.T, recorder *Recorder) {
	t.Helper()
	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHealthReportsCumulativeCountersAcrossRecorders(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "runtime", "telemetry.db")
	config := Config{Path: path, WorkspaceRoot: workspace, BufferSize: 8}
	var sequence atomic.Uint64
	config.ID = func() (string, error) { return fmt.Sprintf("evt_cum_%d", sequence.Add(1)), nil }

	first, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		first.Record(validEvent(EventResolutionCompleted))
	}
	bad := validEvent(EventResolutionCompleted)
	bad.Payload["task"] = "raw"
	first.Record(bad)
	mustFlush(t, first)
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	second, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	second.Record(validEvent(EventResolutionCompleted))
	mustFlush(t, second)
	health := second.Health()
	if health.Written != 4 || health.Rejected != 1 || health.Accepted != 4 {
		t.Fatalf("cumulative health with live recorder = %+v", health)
	}
	if err := second.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	third, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close(context.Background())
	mustFlush(t, third)
	health = third.Health()
	if health.Written != 4 || health.Rejected != 1 || health.Dropped != 0 {
		t.Fatalf("cumulative health after close = %+v", health)
	}
}

func TestRecorderRecoversAfterRuntimeDirectoryIsRecreated(t *testing.T) {
	workspace := t.TempDir()
	runtimeDir := filepath.Join(workspace, "runtime")
	recorder := newTestRecorder(t, Config{Path: filepath.Join(runtimeDir, "telemetry.db"), WorkspaceRoot: workspace})
	first := validEvent(EventResolutionCompleted)
	first.ID = "evt_before"
	recorder.Record(first)
	mustFlush(t, recorder)

	if err := os.RemoveAll(runtimeDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	mustFlush(t, recorder)
	second := validEvent(EventResolutionCompleted)
	second.ID = "evt_after"
	recorder.Record(second)
	mustFlush(t, recorder)
	preview, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Events != 1 || !strings.Contains(string(preview.JSONL), "evt_after") {
		t.Fatalf("recorder did not recover: %s", preview.JSONL)
	}
	if health := recorder.Health(); health.State != "healthy" {
		t.Fatalf("health = %+v", health)
	}
}

func TestStoredTimestampsAreFixedWidthAndRetentionIsExactWithinSecond(t *testing.T) {
	second := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	now := second.Add(time.Hour + 500*time.Millisecond)
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return now }, Retention: time.Hour})
	whole := validEvent(EventResolutionCompleted)
	whole.ID, whole.OccurredAt = "evt_whole", second
	fractional := validEvent(EventResolutionCompleted)
	fractional.ID, fractional.OccurredAt = "evt_fraction", second.Add(700*time.Millisecond)
	recorder.Record(whole)
	recorder.Record(fractional)
	mustFlush(t, recorder)

	preview, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	text := string(preview.JSONL)
	if strings.Contains(text, "evt_whole") || !strings.Contains(text, "evt_fraction") {
		t.Fatalf("retention kept the wrong events: %s", text)
	}
	if !strings.Contains(text, `"occurred_at":"2026-09-15T10:00:00.700000000Z"`) {
		t.Fatalf("timestamp is not fixed width: %s", text)
	}
}

func TestPreviewAcceptsRowsWrittenWithVariablePrecisionTimestamps(t *testing.T) {
	recorder := newTestRecorder(t, Config{Clock: func() time.Time { return time.Date(2026, 9, 15, 11, 0, 0, 0, time.UTC) }})
	event := validEvent(EventResolutionCompleted)
	event.ID, event.OccurredAt = "evt_legacy", time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	recorder.Record(event)
	mustFlush(t, recorder)

	database, err := sql.Open("sqlite", sqliteURL(recorder.config.Path))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	const legacy = "2026-09-15T10:00:00Z"
	if _, err := database.Exec(`UPDATE telemetry_events SET occurred_at = ?, payload_json = replace(payload_json, '2026-09-15T10:00:00.000000000Z', ?) WHERE id = 'evt_legacy'`, legacy, legacy); err != nil {
		t.Fatal(err)
	}
	preview, err := recorder.Preview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Events != 1 || preview.Skipped != 0 {
		t.Fatalf("legacy row not accepted: %+v %s", preview, preview.JSONL)
	}
}
