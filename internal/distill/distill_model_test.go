package distill

import (
	"strings"
	"testing"
	"time"
)

func TestDistillDocumentValidation(t *testing.T) {
	t.Parallel()

	validSha := strings.Repeat("a", 40)
	doc := &Document{
		Goal: "Improve error handling",
		Cursors: []Cursor{
			{SourceID: "src-1", Commit: validSha, SyncedAt: "2026-10-09T08:00:00Z"},
		},
		Coverage: []CoverageItem{
			{Resource: "retry.go", Status: "analyzed", Reason: "Complete review"},
		},
		Lessons: []Lesson{
			{
				Key:     "retry-jitter",
				What:    "Full jitter exponential backoff",
				Notable: "Prevents thundering herd on service recovery",
				Where:   []string{"github.com/org/repo@" + validSha + ":retry.go#L10-L20"},
				Decision: Decision{
					Status: "candidate",
				},
			},
		},
	}

	if err := ValidateDocument(doc); err != nil {
		t.Fatalf("expected valid document, got: %v", err)
	}

	// Missing goal
	docNoGoal := *doc
	docNoGoal.Goal = ""
	if err := ValidateDocument(&docNoGoal); err == nil {
		t.Fatal("expected error on empty goal")
	}

	// Invalid coverage status
	docBadCoverage := *doc
	docBadCoverage.Coverage = []CoverageItem{{Resource: "foo.go", Status: "unknown"}}
	if err := ValidateDocument(&docBadCoverage); err == nil {
		t.Fatal("expected error on invalid coverage status")
	}

	// Short SHA evidence
	docBadEvidence := *doc
	docBadEvidence.Lessons = []Lesson{
		{
			Key:     "bad-sha",
			What:    "Claim",
			Notable: "Reason",
			Where:   []string{"repo@abc:path.go"},
		},
	}
	if err := ValidateDocument(&docBadEvidence); err == nil {
		t.Fatal("expected error on short SHA evidence")
	}

	// Valid usage evidence
	docUsageEvidence := *doc
	docUsageEvidence.Lessons = []Lesson{
		{
			Key:     "usage-derived",
			What:    "Observed pattern",
			Notable: "Explanation",
			Where:   []string{"usage:cs_7f2a"},
		},
	}
	if err := ValidateDocument(&docUsageEvidence); err != nil {
		t.Fatalf("usage:<case_id> should be valid evidence, got: %v", err)
	}
}

func TestLessonReopenOnNewEvidence(t *testing.T) {
	t.Parallel()

	sha1 := strings.Repeat("1", 40)
	sha2 := strings.Repeat("2", 40)
	evidence1 := "github.com/org/repo@" + sha1 + ":docs/retry.md#L10"
	evidence2 := "github.com/org/repo@" + sha2 + ":docs/retry.md#L20"

	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	doc := &Document{
		Goal: "Test reopening",
		Lessons: []Lesson{
			{
				Key:     "backoff-jitter",
				What:    "Use jittered backoff",
				Notable: "Stops thundering herd",
				Where:   []string{evidence1},
				Decision: Decision{
					Status:    "rejected",
					Reason:    "Not needed currently",
					At:        now.Format(time.RFC3339),
					SeenWhere: []string{evidence1},
				},
			},
		},
	}

	// 1. Re-applying the exact same evidence leaves decision intact (rejected)
	sameLesson := Lesson{
		Key:     "backoff-jitter",
		What:    "Use jittered backoff updated",
		Notable: "Stops thundering herd",
		Where:   []string{evidence1},
	}
	if err := doc.ApplyLesson(sameLesson, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if doc.Lessons[0].Decision.Status != "rejected" {
		t.Fatalf("expected decision to remain rejected, got %s", doc.Lessons[0].Decision.Status)
	}

	// 2. Applying new evidence reopens the lesson back to candidate
	newLesson := Lesson{
		Key:     "backoff-jitter",
		What:    "Use jittered backoff converged",
		Notable: "Stops thundering herd across two sources",
		Where:   []string{evidence1, evidence2},
	}
	reopenTime := now.Add(2 * time.Hour)
	if err := doc.ApplyLesson(newLesson, reopenTime); err != nil {
		t.Fatal(err)
	}
	if doc.Lessons[0].Decision.Status != "candidate" {
		t.Fatalf("expected lesson to reopen to candidate, got %s", doc.Lessons[0].Decision.Status)
	}
	if !strings.Contains(doc.Lessons[0].Decision.Reason, "reopened: new evidence observed") {
		t.Fatalf("unexpected reason: %s", doc.Lessons[0].Decision.Reason)
	}
	if len(doc.Lessons[0].Where) != 2 {
		t.Fatalf("expected 2 converged where entries, got %d", len(doc.Lessons[0].Where))
	}
}

func TestDocumentRoundTripYAML(t *testing.T) {
	t.Parallel()

	validSha := strings.Repeat("f", 40)
	rel := 0.9
	doc := &Document{
		Goal: "Round-trip test",
		Cursors: []Cursor{
			{SourceID: "src-main", Commit: validSha, SyncedAt: "2026-10-09T08:00:00Z"},
		},
		Coverage: []CoverageItem{
			{Resource: "main.go", Status: "analyzed", Reason: "Inspected"},
		},
		Lessons: []Lesson{
			{
				Key:      "sample-lesson",
				What:     "Knowledge",
				Notable:  "Explanation of why it matters",
				Contrast: "Contrast with old way",
				Scores:   &Scores{Relevance: &rel},
				Where:    []string{"github.com/org/repo@" + validSha + ":main.go#L5"},
				Decision: Decision{
					Status:    "planned",
					Reason:    "High value",
					At:        "2026-10-09T09:00:00Z",
					SeenWhere: []string{"github.com/org/repo@" + validSha + ":main.go#L5"},
				},
			},
		},
	}

	data, err := MarshalDocument(doc)
	if err != nil {
		t.Fatalf("MarshalDocument: %v", err)
	}

	unmarshaled, err := UnmarshalDocument(data)
	if err != nil {
		t.Fatalf("UnmarshalDocument: %v", err)
	}

	if unmarshaled.Goal != doc.Goal {
		t.Errorf("goal = %q, want %q", unmarshaled.Goal, doc.Goal)
	}
	if len(unmarshaled.Lessons) != 1 || unmarshaled.Lessons[0].Key != "sample-lesson" {
		t.Fatalf("lessons mismatch: %#v", unmarshaled.Lessons)
	}
	if unmarshaled.Lessons[0].Notable != doc.Lessons[0].Notable {
		t.Errorf("notable = %q, want %q", unmarshaled.Lessons[0].Notable, doc.Lessons[0].Notable)
	}
}
