package distill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistillValidatorAcceptsDistillLabTestAudit(t *testing.T) {
	t.Parallel()

	testdataPath := filepath.Join("..", "migration", "testdata", "test-audit-distill.yaml")
	data, err := os.ReadFile(testdataPath)
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	doc, err := UnmarshalDocument(data)
	if err != nil {
		t.Fatalf("UnmarshalDocument failed: %v", err)
	}

	if doc.Goal.Status != "confirmed" {
		t.Fatalf("expected goal.status 'confirmed', got %q", doc.Goal.Status)
	}
	if len(doc.Cursors) != 2 {
		t.Fatalf("expected 2 cursors, got %d", len(doc.Cursors))
	}
	if doc.Cursors["openclaw"] == "" || doc.Cursors["superpowers"] == "" {
		t.Fatalf("missing expected cursors: %#v", doc.Cursors)
	}
	if len(doc.Lessons) != 42 {
		t.Fatalf("expected 42 lessons, got %d", len(doc.Lessons))
	}

	for i, l := range doc.Lessons {
		if l.Key == "" {
			t.Fatalf("lesson[%d] missing key", i)
		}
		if l.Layer == "" {
			t.Fatalf("lesson[%d] (%s) missing layer", i, l.Key)
		}
		if l.Score.Why == "" {
			t.Fatalf("lesson[%d] (%s) missing score.why", i, l.Key)
		}
		if l.Decision.State == "" {
			t.Fatalf("lesson[%d] (%s) missing decision.state", i, l.Key)
		}
	}
}

func TestImpactOfAndFinalScore(t *testing.T) {
	t.Parallel()

	// Score without 'a' has impact 0
	scoreNoA := Score{
		Relevance: 3,
		Facts:     []string{"b", "c"},
		Evidence:  2,
		Effort:    1,
		Why:       "Test",
	}
	if got := ImpactOf(scoreNoA.Facts); got != 0 {
		t.Fatalf("expected impact 0 without fact 'a', got %d", got)
	}
	if got := FinalScore(scoreNoA); got != 0 {
		t.Fatalf("expected final score 0, got %f", got)
	}

	// Score with 'a' has impact = len(facts)
	scoreWithA := Score{
		Relevance: 3,
		Facts:     []string{"a", "b", "c", "d", "e"},
		Evidence:  2,
		Effort:    1,
		Why:       "Full facts",
	}
	if got := ImpactOf(scoreWithA.Facts); got != 5 {
		t.Fatalf("expected impact 5, got %d", got)
	}
	// 3 * 5 * 2 / 1 = 30.0
	if got := FinalScore(scoreWithA); got != 30.0 {
		t.Fatalf("expected final score 30.0, got %f", got)
	}
}

func TestDistillDocumentValidation(t *testing.T) {
	t.Parallel()

	sha := strings.Repeat("a", 40)
	doc := &Document{
		Goal: Goal{
			Status:             "draft",
			Purpose:            "Test purpose",
			InScope:            []string{"gate"},
			OutOfScope:         []string{"features"},
			FailuresItPrevents: []string{"junk"},
		},
		Cursors: map[string]string{
			"source-a": sha,
		},
		Coverage: map[string]CoverageSource{
			"source-a": {
				Read: []string{"file1.go"},
				NotRead: []CoverageNotReadItem{
					{Path: "file2.go", Reason: "out of scope"},
				},
			},
		},
		Lessons: []Lesson{
			{
				Key:      "sample-key",
				Layer:    "content",
				What:     "Sample knowledge",
				Notable:  "Sample note",
				Where:    []string{"source-a@" + sha[:7]},
				Contrast: "new",
				Score: Score{
					Relevance: 3,
					Facts:     []string{"a"},
					Impact:    1,
					Evidence:  2,
					Effort:    1,
					Why:       "High value",
				},
				Decision: Decision{
					State: "candidate",
				},
			},
		},
	}

	if err := ValidateDocument(doc); err != nil {
		t.Fatalf("expected valid document, got: %v", err)
	}

	// 1. Missing goal status
	badGoal := *doc
	badGoal.Goal.Status = "invalid"
	if err := ValidateDocument(&badGoal); err == nil {
		t.Fatal("expected error on invalid goal.status")
	}

	// 2. Empty cursors
	badCursors := *doc
	badCursors.Cursors = map[string]string{}
	if err := ValidateDocument(&badCursors); err == nil {
		t.Fatal("expected error on empty cursors")
	}

	// 3. Where cites unknown source
	badWhere := *doc
	badWhere.Lessons = []Lesson{
		{
			Key:      "sample-key",
			Layer:    "content",
			What:     "Sample knowledge",
			Notable:  "Sample note",
			Where:    []string{"unknown-source@" + sha},
			Contrast: "new",
			Score: Score{
				Relevance: 2,
				Facts:     []string{"a"},
				Evidence:  2,
				Effort:    1,
				Why:       "Why",
			},
			Decision: Decision{State: "candidate"},
		},
	}
	if err := ValidateDocument(&badWhere); err == nil || !strings.Contains(err.Error(), "cites unknown source") {
		t.Fatalf("expected unknown source error, got: %v", err)
	}

	// 4. Rejected decision requires reason
	badDecision := *doc
	badDecision.Lessons = []Lesson{
		{
			Key:      "sample-key",
			Layer:    "content",
			What:     "Sample knowledge",
			Notable:  "Sample note",
			Where:    []string{"source-a@" + sha},
			Contrast: "new",
			Score: Score{
				Relevance: 2,
				Facts:     []string{"a"},
				Evidence:  2,
				Effort:    1,
				Why:       "Why",
			},
			Decision: Decision{State: "rejected", Reason: ""},
		},
	}
	if err := ValidateDocument(&badDecision); err == nil || !strings.Contains(err.Error(), "rejected decision needs a reason") {
		t.Fatalf("expected rejected decision reason error, got: %v", err)
	}
}
