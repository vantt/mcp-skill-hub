package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInsightValidationErrorsAreInvalidRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("empty rationale", func(t *testing.T) {
		t.Parallel()
		root, _, _, item := workspaceWithPendingInsight(t)
		service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}}
		_, err := service.DecideInsight(ctx, root, item.ID, InsightDecisionInput{Decision: "plan", Rationale: ""})
		if err == nil {
			t.Fatal("expected error on empty rationale")
		}
		appErr := ClassifyError(err)
		if appErr.Code != ErrorInvalidRequest {
			t.Fatalf("expected code %s, got %s", ErrorInvalidRequest, appErr.Code)
		}
		if !strings.Contains(err.Error(), "insight decision requires a rationale") {
			t.Fatalf("expected message to contain original text, got: %v", err)
		}
	})

	t.Run("plan when rejected", func(t *testing.T) {
		t.Parallel()
		root, _, _, item := workspaceWithPendingInsight(t)
		service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}}
		_, err := service.DecideInsight(ctx, root, item.ID, InsightDecisionInput{Decision: "reject", Rationale: "Not needed."})
		if err != nil {
			t.Fatalf("reject failed: %v", err)
		}
		_, err = service.DecideInsight(ctx, root, item.ID, InsightDecisionInput{Decision: "plan", Rationale: "Try planning."})
		if err == nil {
			t.Fatal("expected error planning rejected insight")
		}
		appErr := ClassifyError(err)
		if appErr.Code != ErrorInvalidRequest {
			t.Fatalf("expected code %s, got %s", ErrorInvalidRequest, appErr.Code)
		}
		if !strings.Contains(err.Error(), "only a pending insight can be planned") {
			t.Fatalf("expected message to contain original text, got: %v", err)
		}
	})

	t.Run("reopen when pending", func(t *testing.T) {
		t.Parallel()
		root, _, _, item := workspaceWithPendingInsight(t)
		service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}}
		_, err := service.DecideInsight(ctx, root, item.ID, InsightDecisionInput{Decision: "reopen", Rationale: "Reopening."})
		if err == nil {
			t.Fatal("expected error reopening pending insight")
		}
		appErr := ClassifyError(err)
		if appErr.Code != ErrorInvalidRequest {
			t.Fatalf("expected code %s, got %s", ErrorInvalidRequest, appErr.Code)
		}
		if !strings.Contains(err.Error(), "only a rejected insight can be reopened") {
			t.Fatalf("expected message to contain original text, got: %v", err)
		}
	})

	t.Run("empty changes", func(t *testing.T) {
		t.Parallel()
		root, _, _, item := workspaceWithPendingInsight(t)
		service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}}
		_, err := service.PreviewInsightApplication(ctx, root, item.ID, PreviewInsightInput{Changes: []ApplicationChange{}})
		if err == nil {
			t.Fatal("expected error with empty changes")
		}
		appErr := ClassifyError(err)
		if appErr.Code != ErrorInvalidRequest {
			t.Fatalf("expected code %s, got %s", ErrorInvalidRequest, appErr.Code)
		}
		if !strings.Contains(err.Error(), "application preview requires at least one changed skill path") {
			t.Fatalf("expected message to contain original text, got: %v", err)
		}
	})

	t.Run("unchanged file content", func(t *testing.T) {
		t.Parallel()
		root, _, _, item := workspaceWithPendingInsight(t)
		service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}}
		path := "skills/software/consumer-review/SKILL.md"
		current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("failed reading current file: %v", err)
		}
		_, err = service.PreviewInsightApplication(ctx, root, item.ID, PreviewInsightInput{
			Changes:  []ApplicationChange{{Path: path, Contents: string(current)}},
			Mappings: []ApplicationMapping{{ObservationID: item.ObservationIDs[0], ArtifactPath: path, Concept: "retry-review"}},
		})
		if err == nil {
			t.Fatal("expected error when file is unchanged")
		}
		appErr := ClassifyError(err)
		if appErr.Code != ErrorInvalidRequest {
			t.Fatalf("expected code %s, got %s", ErrorInvalidRequest, appErr.Code)
		}
		if !strings.Contains(err.Error(), "application path skills/software/consumer-review/SKILL.md is unchanged") {
			t.Fatalf("expected message to contain original text, got: %v", err)
		}
	})
}
