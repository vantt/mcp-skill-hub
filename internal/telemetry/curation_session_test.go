package telemetry

import (
	"bytes"
	"errors"
	"testing"
)

func TestRecordCurationSessionIsSynchronousIdempotentAndContentFree(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	turns, confirmations, prompts, batch, duration := int64(2), int64(0), int64(1), int64(4), int64(1250)
	auto, recovered, gitNoise := true, false, int64(0)
	report := CurationSession{
		EventID: "evt_curation_1", Status: "completed", Basis: CurationBasisHostReported,
		TurnsToNextAction: &turns, UnnecessaryConfirmations: &confirmations,
		PromptsPerBatch: &prompts, BatchSize: &batch, AutoFinalized: &auto,
		RecoveryCompleted: &recovered, RoutineGitNoise: &gitNoise, DurationMS: &duration,
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy",
		Client: Client{Name: "skillhub-mcp", Version: "1"},
	}
	first, err := recorder.RecordCurationSession(t.Context(), report)
	if err != nil || first.Deduplicated {
		t.Fatalf("first record = %+v, %v", first, err)
	}
	second, err := recorder.RecordCurationSession(t.Context(), report)
	if err != nil || !second.Deduplicated {
		t.Fatalf("duplicate record = %+v, %v", second, err)
	}
	preview, err := recorder.Preview(t.Context(), 0)
	if err != nil || preview.Events != 1 || preview.Skipped != 0 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	for _, prohibited := range [][]byte{[]byte(`"task"`), []byte(`"conversation"`), []byte(`"path"`), []byte(`"content"`)} {
		if bytes.Contains(preview.JSONL, prohibited) {
			t.Fatalf("export contains prohibited data %q: %s", prohibited, preview.JSONL)
		}
	}
	if !bytes.Contains(preview.JSONL, []byte(`"routine_git_noise":0`)) || !bytes.Contains(preview.JSONL, []byte(`"basis":"host-reported"`)) {
		t.Fatalf("export omitted observed measurements: %s", preview.JSONL)
	}

	changed := report
	changed.TurnsToNextAction = pointerInt64(3)
	if _, err := recorder.RecordCurationSession(t.Context(), changed); !errors.Is(err, ErrCurationSessionConflict) {
		t.Fatalf("measurement conflict = %v", err)
	}
}

func TestRecordCurationSessionRejectsFabricatedOrUnboundedValues(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	base := CurationSession{
		EventID: "evt_curation_invalid", Status: "completed", Basis: CurationBasisHostReported,
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: Client{Name: "test"},
	}
	cases := map[string]CurationSession{
		"missing status":  func() CurationSession { value := base; value.Status = ""; return value }(),
		"guessed basis":   func() CurationSession { value := base; value.Basis = "guessed"; return value }(),
		"oversized count": func() CurationSession { value := base; value.BatchSize = pointerInt64(10_001); return value }(),
		"unallowlisted error": func() CurationSession {
			value := base
			value.Status = "failed"
			code := "raw_error_message"
			value.ErrorCode = &code
			return value
		}(),
		"completed with error": func() CurationSession { value := base; code := "internal_error"; value.ErrorCode = &code; return value }(),
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := recorder.RecordCurationSession(t.Context(), report); err == nil {
				t.Fatal("invalid report was accepted")
			}
		})
	}
}

func pointerInt64(value int64) *int64 { return &value }
