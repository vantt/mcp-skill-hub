package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

type suggestionSkill struct {
	id       string
	status   string
	examples []string
	counters []string
	notFor   []string
}

func writeSuggestionSkill(t *testing.T, root string, skill suggestionSkill) {
	t.Helper()
	status := skill.status
	if status == "" {
		status = "active"
	}
	if skill.notFor == nil {
		skill.notFor = []string{"compose unrelated poetry"}
	}
	list := func(values []string) string {
		quoted := make([]string, len(values))
		for i, value := range values {
			quoted[i] = fmt.Sprintf("%q", value)
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	}
	dir := filepath.Join(root, "skills", "core", skill.id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := fmt.Sprintf(`schema_version: 1
id: %s
name: %s
status: %s
description: Handle %s work for the team.
routing:
  triggers: [%s work]
  not_for: %s
  min_scope: single_step
  examples: %s
  counter_examples: %s
`, skill.id, skill.id, status, skill.id, skill.id, list(skill.notFor), list(skill.examples), list(skill.counters))
	if err := os.WriteFile(filepath.Join(dir, "skill.meta.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("---\nname: %s\ndescription: Handle %s work for the team.\n---\n# %s\n", skill.id, skill.id, skill.id)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

type suggestionCases struct {
	base time.Time
	n    int
}

func (c *suggestionCases) record(id, kind, session string) telemetry.CaseRecord {
	c.n++
	return telemetry.CaseRecord{
		CaseID:       "case_" + id,
		EventID:      "evt_case_" + id,
		OccurredAt:   c.base.Add(-time.Duration(c.n) * time.Minute),
		Kind:         kind,
		SessionHash:  session,
		ResolutionID: "res_" + id,
		Client:       telemetry.Client{Name: "test-client"},
		Request:      map[string]any{},
		Task:         map[string]any{},
		Resolver:     map[string]any{},
	}
}

func (c *suggestionCases) loadCase(id, kind, session, chosen, text string) telemetry.CaseRecord {
	record := c.record(id, kind, session)
	record.Task["description"] = text
	record.Resolver["primary"] = ""
	record.Chosen = chosen
	return record
}

func (c *suggestionCases) overrideCase(id, session, primary, chosen, text string) telemetry.CaseRecord {
	record := c.loadCase(id, "override", session, chosen, text)
	record.Resolver["primary"] = primary
	return record
}

func (c *suggestionCases) reformulationCase(id, session, prior, chosen, text string) telemetry.CaseRecord {
	record := c.record(id, "verified_reformulation", session)
	record.Task["description"] = "the new phrasing"
	record.Task["prior_description"] = text
	record.Resolver["prior_primary"] = prior
	record.Request["prior"] = map[string]any{"resolution_id": "res_prior_" + id, "kind": "rejected"}
	record.Chosen = chosen
	return record
}

func suggestionEvent(id, eventType, resolutionID, skillID string) telemetry.Event {
	payload := map[string]any{"skill_id": skillID}
	if eventType == telemetry.EventSkillLoaded {
		payload["resource_kind"] = "entrypoint"
		payload["surface"] = "skills_get"
		payload["basis"] = telemetry.LoadBasisServerObserved
	}
	return telemetry.Event{
		Version: telemetry.EventVersion, ID: id, Type: eventType, OccurredAt: time.Now().UTC(),
		ResolutionID: resolutionID, CatalogSnapshot: "test-snapshot", PolicyRevision: "test-policy",
		Client: telemetry.Client{Name: "test-client", Version: "1"}, Payload: payload,
	}
}

type wantSuggestion struct{ skill, field, text string }

func suggestionKeys(report RoutingSuggestionReport) []wantSuggestion {
	keys := []wantSuggestion{}
	for _, suggestion := range report.Suggestions {
		keys = append(keys, wantSuggestion{suggestion.SkillID, suggestion.Field, suggestion.Text})
	}
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
	return keys
}

func findSuggestion(t *testing.T, report RoutingSuggestionReport, skill, field string) RoutingSuggestion {
	t.Helper()
	for _, suggestion := range report.Suggestions {
		if suggestion.SkillID == skill && suggestion.Field == field {
			return suggestion
		}
	}
	t.Fatalf("no %s suggestion for %s in %+v", field, skill, report.Suggestions)
	return RoutingSuggestion{}
}

func manyExamples() []string {
	values := make([]string, 10)
	for i := range values {
		values[i] = fmt.Sprintf("existing example number %d", i)
	}
	return values
}

func overlongText() string {
	return strings.Repeat("word ", 61)
}

func TestRoutingSuggestions(t *testing.T) {
	cases := &suggestionCases{base: time.Now().UTC()}
	sales := []string{"summarize the quarterly sales report", "summarize quarterly sales report pdf", "please summarize quarterly sales report now"}
	rows := []struct {
		name     string
		skills   []suggestionSkill
		cases    []telemetry.CaseRecord
		events   []telemetry.Event
		query    RoutingSuggestionQuery
		journal  *bool
		want     []wantSuggestion
		skipped  map[string]int
		evidence func(t *testing.T, report RoutingSuggestionReport)
	}{
		{
			name:  "missed then chosen gives an examples entry with full evidence",
			cases: []telemetry.CaseRecord{cases.loadCase("a1", "after_no_skill", "sess-one", "alpha", "tidy the shared drive")},
			want:  []wantSuggestion{{"alpha", FieldExamples, "tidy the shared drive"}},
			evidence: func(t *testing.T, report RoutingSuggestionReport) {
				evidence := findSuggestion(t, report, "alpha", FieldExamples).Evidence
				today := time.Now().UTC().Format("2006-01-02")
				if evidence.Events != 1 || evidence.Sessions != 1 || evidence.FirstSeen != today || evidence.LastSeen != today {
					t.Fatalf("evidence counts or dates wrong: %+v", evidence)
				}
				if !reflect.DeepEqual(evidence.Kinds, map[string]int{"after_no_skill": 1}) || !reflect.DeepEqual(evidence.Clients, []string{"test-client"}) {
					t.Fatalf("evidence kinds or clients wrong: %+v", evidence)
				}
				if !reflect.DeepEqual(evidence.ResolutionIDs, []string{"res_a1"}) || !reflect.DeepEqual(evidence.CaseIDs, []string{"case_a1"}) {
					t.Fatalf("evidence ids wrong: %+v", evidence)
				}
				encoded, _ := json.Marshal(report)
				if strings.Contains(string(encoded), "sess-one") || strings.Contains(evidence.Reason, "tidy") || evidence.Reason == "" {
					t.Fatalf("report leaks a session hash or the phrasing, or has no reason: %s", encoded)
				}
			},
		},
		{
			name:  "needs context then chosen is mined like no skill",
			cases: []telemetry.CaseRecord{cases.loadCase("a2", "after_needs_context", "sess-one", "beta", "reconcile the ledger")},
			want:  []wantSuggestion{{"beta", FieldExamples, "reconcile the ledger"}},
		},
		{
			name:   "verified reformulation with a load of the other skill gives both sides and rejection feedback",
			cases:  []telemetry.CaseRecord{cases.reformulationCase("r1", "sess-one", "alpha", "beta", "archive the old invoices")},
			events: []telemetry.Event{suggestionEvent("evt_load_r1", telemetry.EventSkillLoaded, "res_r1", "beta"), suggestionEvent("evt_rej_r1", telemetry.EventActivationRejected, "res_prior_r1", "alpha")},
			want:   []wantSuggestion{{"alpha", FieldCounterExamples, "archive the old invoices"}, {"beta", FieldExamples, "archive the old invoices"}},
			evidence: func(t *testing.T, report RoutingSuggestionReport) {
				x := findSuggestion(t, report, "alpha", FieldCounterExamples).Evidence
				y := findSuggestion(t, report, "beta", FieldExamples).Evidence
				if x.OtherSkill != "beta" || y.OtherSkill != "alpha" || x.RejectedFeedback != 1 || y.RejectedFeedback != 0 {
					t.Fatalf("other skill or rejection feedback wrong: x=%+v y=%+v", x, y)
				}
				if report.RawEventsSince == "" {
					t.Fatal("raw_events_since was not reported")
				}
			},
		},
		{
			name:  "verified reformulation without a load gives only the rejected side",
			cases: []telemetry.CaseRecord{cases.reformulationCase("r2", "sess-one", "alpha", "beta", "archive the old invoices")},
			want:  []wantSuggestion{{"alpha", FieldCounterExamples, "archive the old invoices"}},
		},
		{
			name:  "same skill on both sides gives nothing",
			cases: []telemetry.CaseRecord{cases.reformulationCase("r3", "sess-one", "alpha", "alpha", "archive the old invoices")},
		},
		{
			name:  "override gives both sides",
			cases: []telemetry.CaseRecord{cases.overrideCase("o1", "sess-one", "alpha", "beta", "rotate the signing keys")},
			want:  []wantSuggestion{{"alpha", FieldCounterExamples, "rotate the signing keys"}, {"beta", FieldExamples, "rotate the signing keys"}},
		},
		{
			name: "three overlapping rejections from two sessions give one not_for entry",
			cases: []telemetry.CaseRecord{
				cases.reformulationCase("c1", "sess-one", "alpha", "beta", sales[0]),
				cases.reformulationCase("c2", "sess-two", "alpha", "beta", sales[1]),
				cases.reformulationCase("c3", "sess-one", "alpha", "beta", sales[2]),
			},
			want: []wantSuggestion{{"alpha", FieldNotFor, "summarize quarterly sales report"}},
			evidence: func(t *testing.T, report RoutingSuggestionReport) {
				evidence := findSuggestion(t, report, "alpha", FieldNotFor).Evidence
				if evidence.Events != 3 || evidence.Sessions != 2 || evidence.OtherSkill != "beta" || len(evidence.CaseIDs) != 3 {
					t.Fatalf("not_for evidence wrong: %+v", evidence)
				}
			},
		},
		{
			name: "the same rejections from one session stay counter examples",
			cases: []telemetry.CaseRecord{
				cases.reformulationCase("d1", "sess-one", "alpha", "beta", sales[0]),
				cases.reformulationCase("d2", "sess-one", "alpha", "beta", sales[1]),
				cases.reformulationCase("d3", "sess-one", "alpha", "beta", sales[2]),
			},
			want: []wantSuggestion{{"alpha", FieldCounterExamples, sales[0]}, {"alpha", FieldCounterExamples, sales[1]}, {"alpha", FieldCounterExamples, sales[2]}},
		},
		{
			name:  "ignored case kinds give nothing",
			cases: []telemetry.CaseRecord{cases.loadCase("k1", "rejected", "sess-one", "alpha", "something"), cases.loadCase("k2", "needs_context_resolved", "sess-one", "alpha", "something"), cases.loadCase("k3", "repeated_gap", "sess-one", "alpha", "something")},
		},
		{
			name:    "skill filter keeps one skill",
			cases:   []telemetry.CaseRecord{cases.overrideCase("f1", "sess-one", "alpha", "beta", "rotate the signing keys")},
			query:   RoutingSuggestionQuery{SkillID: "beta"},
			want:    []wantSuggestion{{"beta", FieldExamples, "rotate the signing keys"}},
			skipped: map[string]int{},
		},
		{
			name:    "no text counts a missing phrasing and a missing earlier field",
			cases:   []telemetry.CaseRecord{cases.loadCase("n1", "after_no_skill", "sess-one", "alpha", "   "), cases.reformulationCase("n2", "sess-one", "", "beta", "some phrasing")},
			skipped: map[string]int{SkipNoText: 2},
		},
		{
			name:    "redaction markers are dropped",
			cases:   []telemetry.CaseRecord{cases.loadCase("x1", "after_no_skill", "sess-one", "alpha", "mail the report to [EMAIL] today"), cases.loadCase("x2", "after_no_skill", "sess-one", "alpha", "open ~/notes and tidy")},
			skipped: map[string]int{SkipRedacted: 2},
		},
		{
			name:    "control and bidi characters are dropped",
			cases:   []telemetry.CaseRecord{cases.loadCase("u1", "after_no_skill", "sess-one", "alpha", "tidy\u202e the drive"), cases.loadCase("u2", "after_no_skill", "sess-one", "alpha", "tidy\x1b[31m the drive")},
			skipped: map[string]int{SkipUnsafeCharacters: 2},
		},
		{
			name:    "text over the canonical limit is dropped",
			cases:   []telemetry.CaseRecord{cases.loadCase("l1", "after_no_skill", "sess-one", "alpha", overlongText())},
			skipped: map[string]int{SkipTooLong: 1},
		},
		{
			name:    "missing and inactive skills are unavailable",
			skills:  []suggestionSkill{{id: "alpha"}, {id: "beta"}, {id: "gamma", status: "draft"}},
			cases:   []telemetry.CaseRecord{cases.loadCase("s1", "after_no_skill", "sess-one", "ghost", "tidy the drive"), cases.loadCase("s2", "after_no_skill", "sess-one", "gamma", "tidy the drive")},
			skipped: map[string]int{SkipSkillUnavailable: 2},
		},
		{
			name:    "text already in the field ignores case",
			skills:  []suggestionSkill{{id: "alpha", examples: []string{"Tidy The Drive"}}, {id: "beta"}},
			cases:   []telemetry.CaseRecord{cases.loadCase("p1", "after_no_skill", "sess-one", "alpha", "tidy  the drive")},
			skipped: map[string]int{SkipAlreadyPresent: 1},
		},
		{
			name:   "text in a contradicting field is reported as a conflict",
			skills: []suggestionSkill{{id: "alpha", counters: []string{"tidy the drive"}}, {id: "beta"}},
			cases:  []telemetry.CaseRecord{cases.loadCase("q1", "after_no_skill", "sess-one", "alpha", "tidy the drive")},
			evidence: func(t *testing.T, report RoutingSuggestionReport) {
				want := []RoutingConflict{{SkillID: "alpha", Field: FieldExamples, Text: "tidy the drive", ConflictsWith: FieldCounterExamples}}
				if !reflect.DeepEqual(report.Conflicts, want) {
					t.Fatalf("conflicts = %+v", report.Conflicts)
				}
			},
			skipped: map[string]int{SkipConflict: 1},
		},
		{
			name:    "a full list takes no more entries",
			skills:  []suggestionSkill{{id: "alpha", examples: manyExamples()}, {id: "beta"}},
			cases:   []telemetry.CaseRecord{cases.loadCase("m1", "after_no_skill", "sess-one", "alpha", "tidy the drive")},
			skipped: map[string]int{SkipFieldFull: 1},
		},
		{
			name:    "too few events stay below the threshold",
			cases:   []telemetry.CaseRecord{cases.loadCase("t1", "after_no_skill", "sess-one", "alpha", "tidy the drive")},
			query:   RoutingSuggestionQuery{MinEvents: 2},
			skipped: map[string]int{SkipBelowThreshold: 1},
		},
		{
			name:    "the case journal disabled gives no suggestions",
			cases:   []telemetry.CaseRecord{cases.loadCase("j1", "after_no_skill", "sess-one", "alpha", "tidy the drive")},
			journal: boolPointer(false),
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			root, service := seedSuggestionWorkspace(t, row.skills, row.cases, row.events, row.journal)
			report, err := service.Suggest(t.Context(), root, row.query)
			if err != nil {
				t.Fatal(err)
			}
			if row.journal != nil {
				if report.CaseJournalEnabled != *row.journal || len(report.Suggestions) != 0 {
					t.Fatalf("journal off report = %+v", report)
				}
				return
			}
			if !report.CaseJournalEnabled {
				t.Fatal("case journal should be reported enabled")
			}
			if got := suggestionKeys(report); !reflect.DeepEqual(got, sortedWants(row.want)) {
				t.Fatalf("suggestions = %+v, want %+v", got, row.want)
			}
			if row.skipped != nil && !reflect.DeepEqual(report.Skipped, row.skipped) {
				t.Fatalf("skipped = %+v, want %+v", report.Skipped, row.skipped)
			}
			if row.evidence != nil {
				row.evidence(t, report)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }

func sortedWants(wants []wantSuggestion) []wantSuggestion {
	out := append([]wantSuggestion{}, wants...)
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]) < fmt.Sprint(out[j]) })
	return out
}

// seedSuggestionWorkspace builds a temp workspace with the skills, records the
// cases and events in its telemetry store, and returns the root and a service.
// A non-nil journal switches the journal to that value after recording.
func seedSuggestionWorkspace(t *testing.T, skills []suggestionSkill, cases []telemetry.CaseRecord, events []telemetry.Event, journal *bool) (string, RoutingSuggestionService) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if skills == nil {
		skills = []suggestionSkill{{id: "alpha"}, {id: "beta"}, {id: "gamma"}}
	}
	for _, skill := range skills {
		writeSuggestionSkill(t, root, skill)
	}
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := telemetry.SetCaseJournalEnabled(root, true); err != nil {
		t.Fatal(err)
	}
	service := RoutingSuggestionService{}
	recorder, err := service.Telemetry.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range cases {
		if err := recorder.RecordCase(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range events {
		recorder.Record(event)
	}
	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if journal != nil {
		if err := telemetry.SetCaseJournalEnabled(root, *journal); err != nil {
			t.Fatal(err)
		}
	}
	return root, service
}

func TestRoutingSuggestionsDismissAndStableIDs(t *testing.T) {
	cases := &suggestionCases{base: time.Now().UTC()}
	root, service := seedSuggestionWorkspace(t, nil, []telemetry.CaseRecord{
		cases.loadCase("z1", "after_no_skill", "sess-one", "alpha", "tidy the drive"),
		cases.loadCase("z2", "after_no_skill", "sess-two", "beta", "reconcile the ledger"),
	}, nil, nil)

	first, err := service.Suggest(t.Context(), root, RoutingSuggestionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Suggest(t.Context(), root, RoutingSuggestionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Suggestions) != 2 || !reflect.DeepEqual(first.Suggestions, second.Suggestions) {
		t.Fatalf("suggestions differ across runs: %+v vs %+v", first.Suggestions, second.Suggestions)
	}
	target := first.Suggestions[0]
	if target.ID != suggestionID(target.SkillID, target.Field, strings.ToUpper(target.Text)) {
		t.Fatalf("ID %s is not stable for the same skill, field and text", target.ID)
	}

	if err := service.Dismiss(t.Context(), root, "not-an-id"); err == nil {
		t.Fatal("a malformed ID was accepted")
	}
	if err := service.Dismiss(t.Context(), root, target.ID); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "runtime", "routing_suggestions.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// File modes are not meaningful on Windows; elsewhere the file stays private.
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("dismissal file mode = %v", info.Mode().Perm())
	}
	if !strings.Contains(string(before), target.SkillID) {
		t.Fatalf("dismissal did not record the skill: %s", before)
	}
	if err := service.Dismiss(t.Context(), root, target.ID); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatalf("a second dismissal changed the file: %s", after)
	}
	if err := service.Dismiss(t.Context(), root, "rs_00000000000000000000"); err != nil {
		t.Fatalf("dismissing an ID that is not suggested must be allowed: %v", err)
	}

	report, err := service.Suggest(t.Context(), root, RoutingSuggestionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Suggestions) != 1 || report.Suggestions[0].ID == target.ID || report.Skipped[SkipDismissed] != 1 {
		t.Fatalf("dismissed suggestion still listed: %+v skipped=%+v", report.Suggestions, report.Skipped)
	}
}

func TestRoutingSuggestionDismissalsKeepTheNewestThousand(t *testing.T) {
	state := routingDismissals{Version: 1, Dismissed: map[string]routingDismissal{}}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < maxDismissals+5; i++ {
		addRoutingDismissal(&state, fmt.Sprintf("rs_%020x", i), routingDismissal{}, base.Add(time.Duration(i)*time.Second))
	}
	trimRoutingDismissals(&state)
	if len(state.Dismissed) != maxDismissals {
		t.Fatalf("kept %d entries", len(state.Dismissed))
	}
	if _, ok := state.Dismissed[fmt.Sprintf("rs_%020x", 0)]; ok {
		t.Fatal("the oldest entry was kept")
	}
	if _, ok := state.Dismissed[fmt.Sprintf("rs_%020x", maxDismissals+4)]; !ok {
		t.Fatal("the newest entry was dropped")
	}
}
