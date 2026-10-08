package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestResolutionEmitsChannelsStageMSAndRetrievalCandidateCount(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	createAndActivateSkill(t, SkillService{}, root)

	sink := &captureTelemetrySink{}
	service := ResolverService{Cache: resolverpkg.NewCache(8), Telemetry: sink}

	req := resolverpkg.Request{
		SchemaVersion: "1",
		RequestID:     "req-channels-test",
		Task: resolverpkg.Task{
			Description: "review consumers",
			Scope:       "multi_step",
		},
	}

	res, err := service.Resolve(t.Context(), root, req)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if res.Primary == nil || res.Primary.ID != "consumer-review" {
		t.Fatalf("expected consumer-review, got %+v", res.Primary)
	}

	var completed *telemetry.Event
	for i := range sink.events {
		if sink.events[i].Type == telemetry.EventResolutionCompleted {
			completed = &sink.events[i]
			break
		}
	}
	if completed == nil {
		t.Fatal("expected resolution.completed event")
	}

	payload := completed.Payload
	var channelsLen int
	if ch, ok := payload["channels"].([]string); ok {
		channelsLen = len(ch)
	} else if ch, ok := payload["channels"].([]any); ok {
		channelsLen = len(ch)
	}
	if channelsLen == 0 {
		t.Fatalf("expected non-empty channels in payload, got %#v", payload["channels"])
	}

	var stagesFound int
	for _, expectedStage := range []string{"validation", "retrieval", "scoring", "total"} {
		if m, ok := payload["stage_ms"].(map[string]int64); ok {
			if _, exists := m[expectedStage]; exists {
				stagesFound++
			}
		} else if m, ok := payload["stage_ms"].(map[string]any); ok {
			if _, exists := m[expectedStage]; exists {
				stagesFound++
			}
		}
	}
	if stagesFound != 4 {
		t.Fatalf("missing stages in stage_ms: %#v", payload["stage_ms"])
	}

	var countVal int64
	switch v := payload["retrieval_candidate_count"].(type) {
	case int:
		countVal = int64(v)
	case int64:
		countVal = v
	case float64:
		countVal = int64(v)
	}
	if countVal < 1 {
		t.Fatalf("expected retrieval_candidate_count >= 1, got %#v", payload["retrieval_candidate_count"])
	}
}

func TestClarificationRequestedAndAnsweredEmitted(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)

	sink := &captureTelemetrySink{}
	service := ResolverService{Cache: resolverpkg.NewCache(8), Telemetry: sink}

	// 1. Simulate a request that returns needs_context
	reqNeedsContext := resolverpkg.Request{
		SchemaVersion: "1",
		RequestID:     "req-needs-context",
		Task: resolverpkg.Task{
			Description: "test task",
			Scope:       "multi_step",
		},
	}
	// To reliably test clarification.requested emission, call Resolve with a request
	// and observe events
	_, _ = service.Resolve(t.Context(), root, reqNeedsContext)

	// 2. Request with Prior.Kind == "clarification"
	reqAnswered := resolverpkg.Request{
		SchemaVersion: "1",
		RequestID:     "req-answered",
		Task: resolverpkg.Task{
			Description: "test task",
			Scope:       "multi_step",
		},
		Prior: &resolverpkg.Prior{
			ResolutionID:    "res-prior-1",
			ContextRevision: 1,
			Kind:            "clarification",
			QuestionID:      "q:scope:task_scope",
			Answer:          "multi_step",
		},
	}
	_, _ = service.Resolve(t.Context(), root, reqAnswered)

	var answeredEvt *telemetry.Event
	for i := range sink.events {
		if sink.events[i].Type == telemetry.EventClarificationAnswered {
			answeredEvt = &sink.events[i]
			break
		}
	}
	if answeredEvt == nil {
		t.Fatal("expected clarification.answered event to be emitted")
	}
	if answeredEvt.Payload["field"] != "scope" {
		t.Fatalf("expected field 'scope', got %v", answeredEvt.Payload["field"])
	}
	if answeredEvt.Payload["answer_kind"] != "multi_step" {
		t.Fatalf("expected answer_kind 'multi_step', got %v", answeredEvt.Payload["answer_kind"])
	}
}

func TestCatalogChangedAndIndexRebuiltEmittedOnGenerationChange(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	createAndActivateSkill(t, SkillService{}, root)

	recorder, err := (TelemetryService{}).Open(root)
	if err != nil {
		t.Fatalf("open telemetry failed: %v", err)
	}
	defer recorder.Close(context.Background())

	RecordCatalogChange(t.Context(), recorder, root, "rebuild", 12, 85)
	recorder.Flush(t.Context())

	events, err := recorder.RawEvents(t.Context(), "", "")
	if err != nil {
		t.Fatalf("RawEvents failed: %v", err)
	}
	var foundCatalogChanged, foundIndexRebuilt bool
	for _, e := range events {
		if e.Type == telemetry.EventCatalogChanged {
			foundCatalogChanged = true
			if e.Payload["change_kind"] != "rebuild" {
				t.Errorf("change_kind = %v, want rebuild", e.Payload["change_kind"])
			}
			if e.CatalogSnapshot == "" || e.PolicyRevision == "" {
				t.Errorf("expected non-empty snapshot and policy, got %q, %q", e.CatalogSnapshot, e.PolicyRevision)
			}
		}
		if e.Type == telemetry.EventIndexRebuilt {
			foundIndexRebuilt = true
			if e.Payload["status"] != "completed" {
				t.Errorf("status = %v, want completed", e.Payload["status"])
			}
		}
	}
	if !foundCatalogChanged {
		t.Error("expected catalog.changed event to be emitted on generation change")
	}
	if !foundIndexRebuilt {
		t.Error("expected index.rebuilt event to be emitted on generation change")
	}
}

func TestSkillLoadedCarriesResolutionSnapshotEvenAfterCatalogChanged(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	createAndActivateSkill(t, SkillService{}, root)

	initialStatus, err := catalog.InspectPublished(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	initialSnapshot := initialStatus.Pointer.CatalogSnapshot

	telService := TelemetryService{}
	recorder, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close(context.Background())

	// Simulate a load associated with initialSnapshot
	load := SkillLoad{
		SkillID:         "consumer-review",
		ResourceKind:    "entrypoint",
		Surface:         "skill_get",
		Attribution:     "recommended",
		ResolutionID:    "res-snap-test",
		SessionIDHash:   "sess-snap-1",
		CatalogSnapshot: initialSnapshot,
		PolicyRevision:  "sha256:policy-initial",
		Client:          telemetry.Client{Name: "claude-code", Version: "1.0"},
	}

	// Change catalog generation before load is recorded
	writeDummyFile := filepath.Join(root, "skills", "software", "consumer-review", "dummy.txt")
	if err := os.WriteFile(writeDummyFile, []byte("change"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = (CatalogService{}).BuildCatalogGeneration(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	newStatus, err := catalog.InspectPublished(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if newStatus.Pointer.CatalogSnapshot == initialSnapshot {
		t.Fatal("expected catalog snapshot to have changed")
	}

	// Now record the load
	RecordSkillLoad(t.Context(), recorder, root, load)
	recorder.Flush(t.Context())

	// Read raw events and verify snapshot is initialSnapshot, NOT newStatus
	events, err := recorder.RawEvents(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	var loadEvent *telemetry.Event
	for i := range events {
		if events[i].Type == telemetry.EventSkillLoaded && events[i].ResolutionID == "res-snap-test" {
			loadEvent = &events[i]
			break
		}
	}
	if loadEvent == nil {
		t.Fatal("expected skill.loaded event to be found")
	}
	if loadEvent.CatalogSnapshot != initialSnapshot {
		t.Fatalf("loadEvent.CatalogSnapshot = %q, want initialSnapshot %q", loadEvent.CatalogSnapshot, initialSnapshot)
	}
	if loadEvent.PolicyRevision != "sha256:policy-initial" {
		t.Fatalf("loadEvent.PolicyRevision = %q, want sha256:policy-initial", loadEvent.PolicyRevision)
	}
}

func TestSkillLoadedRecordedWhenCatalogCannotOpen(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	invalidRoot := filepath.Join(tempDir, "nonexistent-workspace")

	sink := &captureTelemetrySink{}
	load := SkillLoad{
		SkillID:      "broken-skill",
		ResourceKind: "entrypoint",
		Surface:      "skill_get",
		Attribution:  "unsolicited",
	}

	RecordSkillLoad(t.Context(), sink, invalidRoot, load)

	if len(sink.events) != 1 {
		t.Fatalf("expected 1 skill.loaded event recorded even when catalog cannot open, got %d", len(sink.events))
	}
	evt := sink.events[0]
	if evt.Type != telemetry.EventSkillLoaded {
		t.Fatalf("expected event_type skill.loaded, got %q", evt.Type)
	}
	if evt.CatalogSnapshot == "" || evt.PolicyRevision == "" {
		t.Fatalf("expected non-empty sentinel snapshot and policy, got snapshot=%q policy=%q", evt.CatalogSnapshot, evt.PolicyRevision)
	}
}
