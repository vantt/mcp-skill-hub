package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestOverrideAttachesTopKToLoadEventEvenWhenCaseJournalOff(t *testing.T) {
	t.Parallel()

	root := newMCPWorkspace(t)
	// Ensure case journal is off by default
	if telemetry.IsCaseJournalEnabled(root) {
		t.Fatal("expected case journal to be disabled by default")
	}

	// Create and activate a second skill "other-skill"
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

	_, _, recorder, session := connectTelemetrySession(t, root)
	defer recorder.Close(context.Background())

	// 1. Resolve for a task -> returns review-skill
	resolveArgs := map[string]any{
		"schema_version": "1",
		"request_id":     "REQ-override-test",
		"task":           map[string]any{"description": "review this code", "scope": "multi_step"},
		"operation":      "review",
	}
	resolveRes := callSkillResolve(t, session, resolveArgs)
	if resolveRes.Resolution.Primary == nil {
		t.Fatal("expected primary recommendation")
	}
	primaryID := resolveRes.Resolution.Primary.ID

	// 2. Client activates another skill (not the recommended primary) -> this is an OVERRIDE
	otherSkill := "other-skill"
	if primaryID == "other-skill" {
		otherSkill = "review-skill"
	}
	_ = callSkillGet(t, session, otherSkill)

	// 3. Flush recorder
	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	// 4. Inspect raw events from recorder
	nowStr := time.Now().UTC().Format("2006-01-02")
	events, err := recorder.RawEvents(t.Context(), nowStr, nowStr)
	if err != nil {
		t.Fatal(err)
	}

	var foundOverrideLoad bool
	for _, ev := range events {
		if ev.Type == telemetry.EventSkillLoaded && ev.Payload["skill_id"] == otherSkill {
			if ev.Payload["attribution"] == "override" {
				foundOverrideLoad = true
				// Check topk_skill_ids
				topkIDsAny, isAny := ev.Payload["topk_skill_ids"].([]any)
				topkIDsStr, isStr := ev.Payload["topk_skill_ids"].([]string)
				if (!isAny || len(topkIDsAny) == 0) && (!isStr || len(topkIDsStr) == 0) {
					t.Fatalf("expected topk_skill_ids attached to override load event, got %v", ev.Payload["topk_skill_ids"])
				}
				if ev.Payload["topk_matched"] == nil {
					t.Fatalf("expected topk_matched on override load event")
				}
			}
		}
	}
	if !foundOverrideLoad {
		t.Fatal("expected override skill.loaded event to be found")
	}

	// 5. Verify NO case was written to telemetry_cases because case journal is off!
	cases, err := recorder.Cases(t.Context(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Fatalf("expected 0 cases when case journal is disabled, got %d", len(cases))
	}
}
