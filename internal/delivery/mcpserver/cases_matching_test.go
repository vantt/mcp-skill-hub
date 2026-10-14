package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
)

func TestCasesMatchTelemetryEvents(t *testing.T) {
	t.Parallel()

	root := newMCPWorkspace(t)
	// Enable case journal
	if err := telemetry.SetCaseJournalEnabled(root, true); err != nil {
		t.Fatal(err)
	}

	// Create and activate "other-skill"
	service := app.SkillService{}
	content := []byte("---\nname: other-skill\ndescription: Other skill description.\nlicense: Apache-2.0\n---\n\n# Other\n\nOther procedure.\n")
	created, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID: "other-skill", Collection: "core", Name: "Other Skill", Description: "Other skill description.", Content: content,
		Routing: skill.RoutingInput{Operations: []string{"other"}, Triggers: []string{"other procedure"}, NotFor: []string{"write prose"}, MinScope: "multi_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, created, created.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create = %#v, %v", result, err)
	}
	activated, err := service.PreviewActivate(context.Background(), root, "other-skill", false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, activated, activated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm activate = %#v, %v", result, err)
	}

	adapter, server, recorder, session := connectTelemetrySession(t, root)
	defer recorder.Close(context.Background())

	// 1. First resolution; the first phrasing carries a secret that must not survive redaction.
	const firstPhrasing = "initial review task"
	const secret = "sk-1234567890abcdef1234567890abcdef"
	res1 := callSkillResolve(t, session, map[string]any{
		"schema_version": "1",
		"request_id":     "REQ-match-1",
		"task":           map[string]any{"description": firstPhrasing + " " + secret, "scope": "multi_step"},
		"operation":      "review",
	})
	if res1.Resolution.ResolutionID == "" {
		t.Fatal("res1 has empty ResolutionID")
	}
	if res1.Resolution.Primary == nil {
		t.Fatal("res1 recommended nothing")
	}
	firstPrimary := res1.Resolution.Primary.ID

	// 2. Second resolution: verified reformulation with prior.kind: rejected
	res2 := callSkillResolve(t, session, map[string]any{
		"schema_version": "1",
		"request_id":     "REQ-match-2",
		"task":           map[string]any{"description": "reformulated review task", "scope": "multi_step"},
		"operation":      "review",
		"prior": map[string]any{
			"resolution_id":    res1.Resolution.ResolutionID,
			"kind":             "rejected",
			"context_revision": 1,
		},
	})
	if res2.Resolution.ResolutionID == "" {
		t.Fatal("res2 has empty ResolutionID")
	}

	// 3. Override: client loads other-skill instead of recommended review-skill
	_ = callSkillGet(t, session, "other-skill")

	// 4. Flush recorder
	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	// 5. Query raw events
	nowStr := time.Now().UTC().Format("2006-01-02")
	events, err := recorder.RawEvents(t.Context(), nowStr, nowStr)
	if err != nil {
		t.Fatal(err)
	}

	var reformResEvent *telemetry.Event
	var overrideLoadEvent *telemetry.Event
	for i := range events {
		ev := &events[i]
		if ev.Type == telemetry.EventResolutionCompleted && ev.ResolutionID == res2.Resolution.ResolutionID {
			reformResEvent = ev
		}
		if ev.Type == telemetry.EventSkillLoaded && ev.Payload["attribution"] == "override" {
			overrideLoadEvent = ev
		}
	}
	if reformResEvent == nil {
		t.Fatal("expected to find resolution.completed event for res2")
	}
	if overrideLoadEvent == nil {
		t.Fatal("expected to find skill.loaded event for override")
	}

	// 6. Query cases
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 2 {
		t.Fatalf("expected at least 2 cases, got %d", len(cases))
	}

	var reformCase *telemetry.CaseRecord
	var overrideCase *telemetry.CaseRecord
	for i := range cases {
		c := &cases[i]
		if c.Kind == "verified_reformulation" && c.ResolutionID == res2.Resolution.ResolutionID {
			reformCase = c
		}
		if c.Kind == "override" {
			overrideCase = c
		}
	}

	// 7. Assert verified-reformulation case matches telemetry event
	if reformCase == nil {
		t.Fatal("expected verified_reformulation case to be found")
	}
	if reformCase.SessionHash == "" {
		t.Fatal("reformCase has empty SessionHash")
	}
	if reformCase.SessionHash != reformResEvent.SessionIDHash {
		t.Fatalf("reformCase SessionHash %q != event SessionIDHash %q", reformCase.SessionHash, reformResEvent.SessionIDHash)
	}
	if reformCase.ResolutionID != reformResEvent.ResolutionID {
		t.Fatalf("reformCase ResolutionID %q != event ResolutionID %q", reformCase.ResolutionID, reformResEvent.ResolutionID)
	}
	if reformCase.EventID == "" {
		t.Fatal("reformCase has empty EventID")
	}
	if reformCase.EventID != reformResEvent.ID {
		t.Fatalf("reformCase EventID %q != event ID %q", reformCase.EventID, reformResEvent.ID)
	}

	// 8. Assert override case matches telemetry event
	if overrideCase == nil {
		t.Fatal("expected override case to be found")
	}
	if overrideCase.SessionHash == "" {
		t.Fatal("overrideCase has empty SessionHash")
	}
	if overrideCase.SessionHash != overrideLoadEvent.SessionIDHash {
		t.Fatalf("overrideCase SessionHash %q != event SessionIDHash %q", overrideCase.SessionHash, overrideLoadEvent.SessionIDHash)
	}
	if overrideCase.ResolutionID != overrideLoadEvent.ResolutionID {
		t.Fatalf("overrideCase ResolutionID %q != event ResolutionID %q", overrideCase.ResolutionID, overrideLoadEvent.ResolutionID)
	}
	if overrideCase.EventID == "" {
		t.Fatal("overrideCase has empty EventID")
	}
	if overrideCase.EventID != overrideLoadEvent.ID {
		t.Fatalf("overrideCase EventID %q != event ID %q", overrideCase.EventID, overrideLoadEvent.ID)
	}

	// 9. Links the suggestions rely on: the case names the skill it turned down.
	if got := overrideCase.Resolver["primary"]; got != res2.Resolution.Primary.ID {
		t.Fatalf("override resolver.primary = %v, want %q", got, res2.Resolution.Primary.ID)
	}
	if got := reformCase.Resolver["prior_primary"]; got != firstPrimary {
		t.Fatalf("reformulation resolver.prior_primary = %v, want %q", got, firstPrimary)
	}
	wantPrior := firstPhrasing + " [TOKEN]"
	if got := reformCase.Task["prior_description"]; got != wantPrior {
		t.Fatalf("reformulation task.prior_description = %v, want %q", got, wantPrior)
	}

	// 10. A rejection whose prior belongs to another session copies nothing from it.
	other := newClientSession(t, server)
	res3 := callSkillResolve(t, other, map[string]any{
		"schema_version": "1",
		"request_id":     "REQ-match-3",
		"task":           map[string]any{"description": "foreign review task", "scope": "multi_step"},
		"operation":      "review",
		"prior":          map[string]any{"resolution_id": res1.Resolution.ResolutionID, "kind": "rejected", "context_revision": 1},
	})

	// 11. A load after "needs context" is journalled with the empty primary.
	needsContext := resolver.Request{Operation: "review", Task: resolver.Task{Description: "ambiguous task", Scope: "multi_step"}}
	adapter.tracker.noteResolution(nil, needsContext, resolver.Response{ResolutionID: "res-needs-context", Status: resolver.StatusNeedsContext}, false)
	adapter.recordLoad(t.Context(), nil, app.SkillLoad{SkillID: "other-skill", ResourceKind: "entrypoint"})

	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	cases, err = recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	var rejectedCase, needsContextCase *telemetry.CaseRecord
	for i := range cases {
		switch c := &cases[i]; {
		case c.Kind == "rejected" && c.ResolutionID == res3.Resolution.ResolutionID:
			rejectedCase = c
		case c.Kind == "after_needs_context":
			needsContextCase = c
		}
	}
	if rejectedCase == nil {
		t.Fatal("expected a rejected case for the foreign prior")
	}
	if _, ok := rejectedCase.Resolver["prior_primary"]; ok {
		t.Fatalf("rejected case carries prior_primary: %#v", rejectedCase.Resolver)
	}
	if _, ok := rejectedCase.Task["prior_description"]; ok {
		t.Fatalf("rejected case carries prior_description: %#v", rejectedCase.Task)
	}
	if needsContextCase == nil {
		t.Fatal("expected an after_needs_context case")
	}
	if got, ok := needsContextCase.Resolver["primary"]; !ok || got != "" {
		t.Fatalf("after_needs_context resolver.primary = %v (present %v), want empty", got, ok)
	}
	if needsContextCase.Chosen != "other-skill" {
		t.Fatalf("after_needs_context chosen = %q, want other-skill", needsContextCase.Chosen)
	}
}
