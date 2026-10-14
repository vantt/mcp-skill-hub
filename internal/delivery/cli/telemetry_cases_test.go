package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
)

func TestTelemetryCLICases(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)

	var stdout, stderr bytes.Buffer

	// 1. Initial status: disabled
	if code := Run([]string{"telemetry", "cases", "status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("status code=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Case journal: disabled") {
		t.Fatalf("expected disabled, got %s", stdout.String())
	}

	// 2. Enable case journal
	stdout.Reset()
	if code := Run([]string{"telemetry", "cases", "enable", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("enable code=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Case journal enabled.") {
		t.Fatalf("expected enabled confirmation, got %s", stdout.String())
	}

	// 3. Status after enable
	stdout.Reset()
	if code := Run([]string{"telemetry", "cases", "status", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("status code=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Case journal: enabled") {
		t.Fatalf("expected enabled, got %s", stdout.String())
	}

	// 4. Record a case and list it
	telService := app.TelemetryService{}
	recorder, err := telService.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close(context.Background()) })
	err = recorder.RecordCase(t.Context(), telemetry.CaseRecord{
		ResolutionID:    "res_cli_case_1",
		SessionHash:     "sess_cli_case",
		OccurredAt:      time.Now().UTC(),
		Kind:            "override",
		Client:          telemetry.Client{Name: "claude-code"},
		CatalogSnapshot: "sha256:cat",
		Task:            map[string]any{"description": "my cli test task description"},
		Chosen:          "review-skill",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Close(t.Context())

	// 5. Query cases via CLI with --json
	stdout.Reset()
	if code := Run([]string{"telemetry", "cases", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("cases --json code=%d: %s", code, stderr.String())
	}
	var cases []telemetry.CaseRecord
	if err := json.Unmarshal(stdout.Bytes(), &cases); err != nil {
		t.Fatalf("unmarshal cases json: %v\nOutput: %s", err, stdout.String())
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if cases[0].CaseID == "" {
		t.Fatal("expected non-empty case_id in json output")
	}
	if cases[0].Kind != "override" {
		t.Fatalf("expected kind override, got %q", cases[0].Kind)
	}
	if task, _ := cases[0].Task["description"].(string); task != "my cli test task description" {
		t.Fatalf("expected task description, got %q", task)
	}

	// 6. Disable case journal
	stdout.Reset()
	if code := Run([]string{"telemetry", "cases", "disable", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("disable code=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Case journal disabled.") {
		t.Fatalf("expected disabled confirmation, got %s", stdout.String())
	}
}

// seedSuggestionCases records one task phrasing per after_no_skill case, each in
// its own session, with code-review loaded afterwards.
func seedSuggestionCases(t *testing.T, root string, phrasings ...string) {
	t.Helper()
	recorder, err := app.TelemetryService{}.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for i, phrasing := range phrasings {
		id := "sugg" + string(rune('a'+i))
		err := recorder.RecordCase(t.Context(), telemetry.CaseRecord{
			CaseID: "case_" + id, EventID: "evt_case_" + id, ResolutionID: "res_" + id,
			SessionHash: "sess-" + id, OccurredAt: time.Now().UTC().Add(-time.Duration(i+1) * time.Minute),
			Kind: "after_no_skill", Client: telemetry.Client{Name: "test-client"},
			Request: map[string]any{}, Resolver: map[string]any{"primary": ""},
			Task: map[string]any{"description": phrasing}, Chosen: "code-review",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryCLICasesSuggestAndDismiss(t *testing.T) {
	t.Parallel()
	root, _ := telemetryCLIWorkspace(t)
	run := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := Run(append(args, "--workspace", root), &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	code, out, errOut := run("telemetry", "cases", "suggest")
	if code != 0 || !strings.Contains(out, "The case journal is off") || !strings.Contains(out, "skillhub telemetry cases enable") {
		t.Fatalf("journal off: code=%d out=%q err=%q", code, out, errOut)
	}

	if code, _, errOut := run("telemetry", "cases", "enable"); code != 0 {
		t.Fatalf("enable code=%d: %s", code, errOut)
	}
	seedSuggestionCases(t, root, "audit the diff for hidden bugs", "look over my patch for mistakes", "check\x1b[31m the branch")

	code, out, errOut = run("telemetry", "cases", "suggest")
	if code != 0 {
		t.Fatalf("suggest code=%d: %s", code, errOut)
	}
	for _, want := range []string{`"audit the diff for hidden bugs"`, "Why:", "Evidence:", "1 event(s) in 1 session(s)", "Next: skillhub skill edit code-review --from-suggestion rs_", "Skipped: unsafe_characters 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("suggest output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("suggest printed a raw escape:\n%q", out)
	}

	code, out, errOut = run("telemetry", "cases", "suggest", "--json")
	var report struct {
		Suggestions []struct {
			ID, Text string
			Evidence struct{ Events int }
		}
	}
	if code != 0 || json.Unmarshal([]byte(out), &report) != nil || len(report.Suggestions) != 2 || report.Suggestions[0].Evidence.Events != 1 {
		t.Fatalf("suggest --json: code=%d err=%s out=%s", code, errOut, out)
	}
	if code, _, _ := run("telemetry", "cases", "suggest", "--min-events", "0"); code != 2 {
		t.Fatalf("--min-events 0 code=%d", code)
	}

	dismissed, kept := report.Suggestions[0], report.Suggestions[1]
	if code, out, errOut := run("telemetry", "cases", "dismiss", dismissed.ID); code != 0 || !strings.Contains(out, "Suggestion "+dismissed.ID+" dismissed; it will not be suggested again on this machine.") {
		t.Fatalf("dismiss code=%d out=%q err=%q", code, out, errOut)
	}
	if code, out, _ := run("telemetry", "cases", "suggest"); code != 0 || strings.Contains(out, dismissed.ID) || !strings.Contains(out, kept.ID) {
		t.Fatalf("dismissed suggestion still listed: code=%d\n%s", code, out)
	}
	if code, _, _ := run("telemetry", "cases", "dismiss", "not-an-id"); code != 2 {
		t.Fatalf("malformed dismiss code=%d", code)
	}

	// Editing from a suggestion: refused with other edit flags or another skill,
	// previewed without writing, and applied only by confirm.
	metaPath := filepath.Join(root, "skills", "core", "code-review", "skill.meta.yaml")
	before, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run("skill", "edit", "code-review", "--from-suggestion", kept.ID, "--example", "another phrase"); code != 2 {
		t.Fatalf("--from-suggestion with --example code=%d", code)
	}
	if code, _, _ := run("skill", "edit", "other-skill", "--from-suggestion", kept.ID); code != 2 {
		t.Fatalf("--from-suggestion for another skill code=%d", code)
	}
	code, out, errOut = run("skill", "edit", "code-review", "--from-suggestion", kept.ID)
	if code != 0 {
		t.Fatalf("preview code=%d: %s %s", code, out, errOut)
	}
	for _, want := range []string{"Suggestion:", "Why:", "Evidence:", "Proposal:", "Routing impact:", "skillhub skill confirm --proposal "} {
		if !strings.Contains(out, want) {
			t.Fatalf("preview lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "+++") {
		t.Fatalf("diff shown without --diff:\n%s", out)
	}
	if after, _ := os.ReadFile(metaPath); string(after) != string(before) {
		t.Fatal("the preview changed the skill")
	}
	if code, out, _ := run("skill", "edit", "code-review", "--from-suggestion", kept.ID, "--diff"); code != 0 || !strings.Contains(out, "+++") {
		t.Fatalf("--diff: code=%d\n%s", code, out)
	}
	confirm := regexp.MustCompile(`skill confirm --proposal (\S+) --proposal-digest (\S+) --base-version (\S+)`).FindStringSubmatch(out)
	if confirm == nil {
		t.Fatalf("no confirm command in\n%s", out)
	}
	if code, out, errOut := run("skill", "confirm", "--proposal", confirm[1], "--proposal-digest", confirm[2], "--base-version", confirm[3], "--no-commit"); code != 0 {
		t.Fatalf("confirm code=%d: %s %s", code, out, errOut)
	}
	if after, _ := os.ReadFile(metaPath); !strings.Contains(string(after), kept.Text) {
		t.Fatalf("the confirmed edit did not add %q:\n%s", kept.Text, after)
	}
}
