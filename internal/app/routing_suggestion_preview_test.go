package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
)

func previewFixture(t *testing.T, alphaExamples, betaExamples []string, phrase string) (string, SkillService, RoutingSuggestion) {
	t.Helper()
	cases := &suggestionCases{base: time.Now().UTC()}
	root, suggestions := seedSuggestionWorkspace(t,
		[]suggestionSkill{{id: "alpha", examples: alphaExamples}, {id: "beta", examples: betaExamples}, {id: "gamma"}},
		[]telemetry.CaseRecord{cases.loadCase("p1", "after_no_skill", "sess-one", "alpha", phrase)}, nil, nil)
	// Store alpha's metadata the way a skill edit writes it, so a preview's diff
	// shows only the change.
	canonical := "description: Handle alpha work for the team.\nid: alpha\nname: alpha\nrouting:\n    min_scope: single_step\n    not_for:\n        - compose unrelated poetry\n    operations: []\n    triggers:\n        - alpha work\nschema_version: 1\nstatus: active\n"
	if len(alphaExamples) > 0 {
		canonical = strings.Replace(canonical, "    min_scope", "    examples:\n        - "+strings.Join(alphaExamples, "\n        - ")+"\n    min_scope", 1)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "alpha", "skill.meta.yaml"), []byte(canonical), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	report, err := suggestions.Suggest(t.Context(), root, RoutingSuggestionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	return root, SkillService{}, findSuggestion(t, report, "alpha", "examples")
}

func readSkillMeta(t *testing.T, root string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "skills", "core", "alpha", "skill.meta.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPreviewRoutingSuggestion(t *testing.T) {
	const phrase = "tidy the shared drive"

	t.Run("previews without writing and confirms as a normal edit", func(t *testing.T) {
		root, service, suggestion := previewFixture(t, []string{"alpha work today"}, []string{"beta work today"}, phrase)
		before := readSkillMeta(t, root)

		preview, err := service.PreviewRoutingSuggestion(t.Context(), root, suggestion.ID)
		if err != nil {
			t.Fatal(err)
		}
		pins := preview.Confirmation.Confirmation.Pins
		if pins.ProposalID == "" || pins.ProposalDigest == "" || pins.BaseVersion == "" {
			t.Fatalf("pins = %+v", pins)
		}
		if preview.Suggestion.ID != suggestion.ID || preview.Suggestion.Evidence.Reason == "" {
			t.Fatalf("suggestion = %+v", preview.Suggestion)
		}
		added := "+        - " + phrase
		var addedLines int
		for _, line := range strings.Split(preview.FullDiff, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				addedLines++
				if line != added {
					t.Fatalf("unexpected added line %q in\n%s", line, preview.FullDiff)
				}
			}
		}
		if addedLines != 1 {
			t.Fatalf("want exactly one added line (%q), got %d in\n%s", added, addedLines, preview.FullDiff)
		}
		if preview.RoutingImpact == nil || !strings.HasPrefix(preview.RoutingImpact.Summary, "Routing check") {
			t.Fatalf("routing impact = %+v", preview.RoutingImpact)
		}
		if after := readSkillMeta(t, root); string(after) != string(before) {
			t.Fatal("the preview changed the skill")
		}

		result, err := service.ConfirmProposal(t.Context(), root, pins.ProposalID, &pins)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(readSkillMeta(t, root)), phrase) {
			t.Fatal("the confirmed edit did not write the example")
		}
		if result.Commit == nil && !result.GitDirty {
			t.Fatalf("a confirmed edit is neither committed nor dirty: %+v", result)
		}
		report, err := RoutingSuggestionService{}.Suggest(t.Context(), root, RoutingSuggestionQuery{})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range report.Suggestions {
			if s.ID == suggestion.ID {
				t.Fatal("an applied suggestion is still listed")
			}
		}
		if report.Skipped["already_present"] == 0 {
			t.Fatalf("skipped = %v", report.Skipped)
		}
	})

	t.Run("an example another skill holds is a regression naming that skill", func(t *testing.T) {
		root, service, suggestion := previewFixture(t, []string{"alpha work today"}, []string{"beta work today", phrase + " now"}, phrase)
		preview, err := service.PreviewRoutingSuggestion(t.Context(), root, suggestion.ID)
		if err != nil {
			t.Fatal(err)
		}
		impact := preview.RoutingImpact
		if impact == nil || !strings.Contains(strings.Join(impact.Warnings, "\n"), "beta") {
			t.Fatalf("want a warning naming beta, got %+v", impact)
		}
	})

	t.Run("unknown, malformed and dismissed IDs store nothing", func(t *testing.T) {
		root, service, suggestion := previewFixture(t, nil, nil, phrase)
		if err := (RoutingSuggestionService{}).Dismiss(t.Context(), root, suggestion.ID); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{suggestion.ID, "rs_00000000000000000000", "not an id"} {
			_, err := service.PreviewRoutingSuggestion(t.Context(), root, id)
			var appErr *Error
			if !errors.As(err, &appErr) || appErr.Code != ErrorInvalidRequest {
				t.Fatalf("id %q: err = %v", id, err)
			}
		}
		if entries, _ := os.ReadDir(filepath.Join(root, "runtime", "proposals")); len(entries) != 0 {
			t.Fatalf("stored %d proposals", len(entries))
		}
	})

	t.Run("a changed catalog skips the check", func(t *testing.T) {
		root, service, suggestion := previewFixture(t, nil, nil, phrase)
		preview, err := service.PreviewRoutingSuggestion(t.Context(), root, suggestion.ID)
		if err != nil {
			t.Fatal(err)
		}
		writeSuggestionSkill(t, root, suggestionSkill{id: "delta"})
		if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
			t.Fatal(err)
		}
		hook := routingCheckHook{field: "examples", phrase: phrase}
		impact, err := hook.Evaluate(t.Context(), root, "alpha", preview.Confirmation.Confirmation.Pins.BaseVersion, nil, readSkillMeta(t, root))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(impact.Summary, "Routing check skipped: the catalog changed") || len(impact.Warnings) != 0 {
			t.Fatalf("impact = %+v", impact)
		}
	})
}
