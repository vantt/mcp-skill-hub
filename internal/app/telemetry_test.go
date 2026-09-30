package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func TestTelemetryServiceUsesWorkspaceLocalContentFreeStore(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.db")
	service := TelemetryService{Config: telemetry.Config{Path: outside, ContentMode: "debug", BufferSize: 8}}

	recorder, err := service.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(telemetry.Event{
		Type: telemetry.EventResolutionCompleted, CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy",
		Client: telemetry.Client{Name: "test"}, Payload: map[string]any{"status": "resolved"},
	})
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("configured external path was used: %v", err)
	}
	databasePath := filepath.Join(root, "runtime", "telemetry.db")
	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("telemetry mode = %o, want 600", info.Mode().Perm())
	}

	health, err := service.Health(t.Context(), root)
	if err != nil || health.State != "healthy" {
		t.Fatalf("health = %+v, %v", health, err)
	}
	preview, err := service.Preview(t.Context(), root, 10)
	if err != nil || preview.Events != 1 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	output := filepath.Join(root, "runtime", "artifacts", "telemetry.jsonl")
	exported, err := service.Export(t.Context(), root, output)
	if err != nil || exported.Events != 1 || exported.Path != output {
		t.Fatalf("export = %+v, %v", exported, err)
	}
	if err := service.Purge(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	preview, err = service.Preview(t.Context(), root, 0)
	if err != nil || preview.Events != 0 || len(preview.JSONL) != 0 {
		t.Fatalf("preview after purge = %+v, %v", preview, err)
	}
}

func TestTelemetryServiceRejectsSymlinkedRuntime(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "runtime")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := (TelemetryService{}).Open(root); err == nil {
		t.Fatal("Open accepted a symlinked runtime directory")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside directory changed: entries=%v err=%v", entries, err)
	}
}

func TestTelemetryServicePromotionDraftIsSanitizedAndDeterministic(t *testing.T) {
	root := t.TempDir()
	service := TelemetryService{}
	recorder, err := service.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	const rawTask = "review the confidential customer transcript"
	recorder.Record(telemetry.Event{
		ID: "evt-raw-rejected", Type: telemetry.EventResolutionCompleted, ResolutionID: "res-promotion",
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: telemetry.Client{Name: "test"},
		Payload: map[string]any{"status": "resolved", "task": rawTask},
	})
	recorder.Record(telemetry.Event{
		ID: "evt-sanitized", Type: telemetry.EventResolutionCompleted, RequestID: "req-private", ResolutionID: "res-promotion",
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: telemetry.Client{Name: "test"},
		Payload: map[string]any{"status": "resolved", "top_skill_id": "code-review", "reason_codes": []string{"trigger_match"}, "candidate_count": 2},
	})
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}

	first, err := service.PromotionDraft(t.Context(), root, "res-promotion")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.PromotionDraft(t.Context(), root, "res-promotion")
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
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("promotion draft is nondeterministic:\n%s\n%s", firstJSON, secondJSON)
	}
	if bytes.Contains(firstJSON, []byte(rawTask)) || bytes.Contains(firstJSON, []byte(`"task":"`)) {
		t.Fatalf("promotion draft leaked raw task content: %s", firstJSON)
	}
	if !first.ReviewRequired || !first.CaseTemplate.Incomplete || len(first.CaseTemplate.RequiredHumanFields) == 0 {
		t.Fatalf("draft does not require human completion: %+v", first)
	}
	if first.Observed.TopSkillID != "code-review" || first.Observed.Status != "resolved" {
		t.Fatalf("sanitized observation = %+v", first.Observed)
	}
	if !containsString(first.Sanitization.RemovedFields, "request_id") || !containsString(first.Sanitization.RemovedFields, "payload.candidate_count") || len(first.Sanitization.UnavailableFields) == 0 {
		t.Fatalf("sanitization report = %+v", first.Sanitization)
	}
}

func TestTelemetryServicePromotionDraftReportsMissingAndAmbiguousResolution(t *testing.T) {
	root := t.TempDir()
	service := TelemetryService{}
	if _, err := service.PromotionDraft(t.Context(), root, "res-missing"); !errors.Is(err, telemetry.ErrPromotionNotFound) {
		t.Fatalf("missing resolution error = %v", err)
	}
	recorder, err := service.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for index, skillID := range []string{"code-review", "architecture-review"} {
		recorder.Record(telemetry.Event{
			ID: []string{"evt-conflict-one", "evt-conflict-two"}[index], Type: telemetry.EventResolutionCompleted, ResolutionID: "res-ambiguous",
			CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: telemetry.Client{Name: "test"},
			Payload: map[string]any{"status": "resolved", "top_skill_id": skillID},
		})
	}
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PromotionDraft(t.Context(), root, "res-ambiguous"); !errors.Is(err, telemetry.ErrPromotionAmbiguous) {
		t.Fatalf("ambiguous resolution error = %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestTelemetryServiceReportsConfigurationAndContextFailures(t *testing.T) {
	root := t.TempDir()
	service := TelemetryService{Config: telemetry.Config{BufferSize: -1}}
	if _, err := service.Open(root); err == nil {
		t.Fatal("Open accepted a negative buffer size")
	}

	if _, err := (TelemetryService{}).Preview(t.Context(), root, -1); err == nil {
		t.Fatal("Preview accepted a negative limit")
	}
}
