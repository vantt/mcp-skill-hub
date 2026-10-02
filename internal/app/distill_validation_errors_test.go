package app

import (
	"context"
	"strings"
	"testing"
)

func TestDistillValidationErrorsAreInvalidRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("duplicate submitted observation", func(t *testing.T) {
		t.Parallel()
		root, service, adapter := newDistillWorkspace(t, "source-a", false)
		run := prepareAndStart(t, root, service, "source-a")
		sub := validSubmission(run, adapter)
		// Duplicate finding with same StableKey
		sub.Findings = append(sub.Findings, sub.Findings[0])
		_, err := service.SubmitDistillRun(ctx, root, run.ID, sub)
		if err == nil {
			t.Fatal("expected error with duplicate findings")
		}
		appErr := ClassifyError(err)
		if appErr.Code != ErrorInvalidRequest {
			t.Fatalf("expected code %s, got %s", ErrorInvalidRequest, appErr.Code)
		}
		if !strings.Contains(err.Error(), "duplicate submitted observation") {
			t.Fatalf("expected message to contain original text, got: %v", err)
		}
		// Assert persisted run state and failure
		persisted, err := service.GetDistillRun(ctx, root, run.ID)
		if err != nil {
			t.Fatalf("failed loading distill run: %v", err)
		}
		if persisted.Run.State != "failed" {
			t.Fatalf("expected run state 'failed', got %q", persisted.Run.State)
		}
		if persisted.Run.Failure != "duplicate submitted observation" {
			t.Fatalf("expected failure %q, got %q", "duplicate submitted observation", persisted.Run.Failure)
		}
	})

	t.Run("invalid run ID", func(t *testing.T) {
		t.Parallel()
		root, service, _ := newDistillWorkspace(t, "source-a", false)
		_, err := service.GetDistillRun(ctx, root, "../bad")
		if err == nil {
			t.Fatal("expected error with invalid run ID")
		}
		appErr := ClassifyError(err)
		if appErr.Code != ErrorInvalidRequest {
			t.Fatalf("expected code %s, got %s", ErrorInvalidRequest, appErr.Code)
		}
		if !strings.Contains(err.Error(), "invalid run ID") {
			t.Fatalf("expected message to contain original text, got: %v", err)
		}
	})
}
