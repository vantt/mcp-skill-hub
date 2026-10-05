package schemas

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"
)

func TestCommittedSchemasResolve(t *testing.T) {
	constructors := map[string]func() (*jsonschema.Schema, error){
		"resolver request":    ResolverRequest,
		"resolver response":   ResolverResponse,
		"telemetry event":     TelemetryEvent,
		"evaluation case":     EvaluationCase,
		"evaluation suite":    EvaluationSuite,
		"experiment manifest": ExperimentManifest,
	}
	for name, constructor := range constructors {
		t.Run(name, func(t *testing.T) {
			schema, err := constructor()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := schema.Resolve(nil); err != nil {
				t.Fatalf("resolve committed schema: %v", err)
			}
		})
	}
}

func TestTelemetryEventSchemaPrivacyAndAllowlists(t *testing.T) {
	valid := `{
		"event_version":"1","event_id":"evt_1","event_type":"curation.session_completed",
		"occurred_at":"2026-09-29T10:00:00.123Z","session_id_hash":"session_hash",
		"catalog_snapshot":"sha256:catalog","policy_revision":"sha256:policy",
		"client":{"name":"skillhub-cli","version":"1.0.0"},
		"privacy":{"content_mode":"none","redaction_version":"redact-v1"},
		"payload":{"status":"completed","basis":"host-reported","turns_to_next_action":2,"prompts_per_batch":1,"auto_finalized":true,"routine_git_noise":0}
	}`
	validateJSON(t, TelemetryEvent, valid, true)

	invalid := []string{
		replaceJSON(valid, `"content_mode":"none"`, `"content_mode":"raw"`),
		replaceJSON(valid, `"routine_git_noise":0`, `"routine_git_noise":0,"conversation":"raw text"`),
		replaceJSON(valid, `"payload":{`, `"task":"raw task","payload":{`),
		replaceJSON(valid, `"turns_to_next_action":2`, `"turns_to_next_action":-1`),
		replaceJSON(valid, `"turns_to_next_action":2`, `"turns_to_next_action":10001`),
		replaceJSON(valid, `"basis":"host-reported",`, ``),
		replaceJSON(valid, `"basis":"host-reported"`, `"basis":"guessed"`),
		replaceJSON(valid, `"routine_git_noise":0`, `"routine_git_noise":0,"error_code":"made_up"`),
		replaceJSON(valid, `"event_type":"curation.session_completed"`, `"event_type":"unknown.event"`),
	}
	for index, document := range invalid {
		t.Run(string(rune('a'+index)), func(t *testing.T) { validateJSON(t, TelemetryEvent, document, false) })
	}
}

func TestTelemetryFeedbackFunnelRequiresExplicitUtilityBasis(t *testing.T) {
	utility := `{
		"event_version":"1","event_id":"evt_feedback:utility","event_type":"skill.utility_reported",
		"occurred_at":"2026-09-29T10:00:00Z","resolution_id":"res_feedback",
		"catalog_snapshot":"sha256:catalog","policy_revision":"sha256:policy",
		"client":{"name":"skillhub"},"privacy":{"content_mode":"none","redaction_version":"redact-v1"},
		"payload":{"skill_id":"code-review","utility":"helpful","basis":"user","reason_codes":["host_report"]}
	}`
	validateJSON(t, TelemetryEvent, utility, true)
	validateJSON(t, TelemetryEvent, replaceJSON(utility, `,"basis":"user"`, ``), false)
	validateJSON(t, TelemetryEvent, replaceJSON(utility, `"utility":"helpful"`, `"utility":"accepted"`), false)
	validateJSON(t, TelemetryEvent, replaceJSON(utility, `"host_report"`, `"sk_live_token_shaped_secret"`), false)

	outcome := replaceJSON(utility, `"event_id":"evt_feedback:utility","event_type":"skill.utility_reported"`, `"event_id":"evt_feedback","event_type":"task.outcome_reported"`)
	outcome = replaceJSON(outcome, `"payload":{"skill_id":"code-review","utility":"helpful","basis":"user","reason_codes":["host_report"]}`, `"payload":{"status":"failed","skill_id":"code-review","reason_codes":["host_report"]}`)
	validateJSON(t, TelemetryEvent, outcome, true)
	validateJSON(t, TelemetryEvent, replaceJSON(outcome, `"status":"failed",`, ``), false)
	validateJSON(t, TelemetryEvent, replaceJSON(outcome, `"host_report"`, `"setup_failed"`), true)
}

func TestTelemetryResolutionSchemaRequiresContentFreeRecommendationIDs(t *testing.T) {
	valid := `{
		"event_version":"1","event_id":"evt_resolution","event_type":"resolution.completed",
		"occurred_at":"2026-09-29T10:00:00Z","resolution_id":"res_feedback",
		"catalog_snapshot":"sha256:catalog","policy_revision":"sha256:policy",
		"client":{"name":"skillhub"},"privacy":{"content_mode":"none","redaction_version":"redact-v1"},
		"payload":{"status":"resolved","top_skill_id":"code-review","recommended_skill_ids":["code-review","test-runner"]}
	}`
	validateJSON(t, TelemetryEvent, valid, true)
	validateJSON(t, TelemetryEvent, replaceJSON(valid, `,"recommended_skill_ids":["code-review","test-runner"]`, ``), false)
	validateJSON(t, TelemetryEvent, replaceJSON(valid, `"code-review","test-runner"`, `"code-review","code-review"`), false)
	validateJSON(t, TelemetryEvent, replaceJSON(valid, `"recommended_skill_ids":["code-review","test-runner"]`, `"recommended_skill_ids":["code-review"],"raw_recommendation":"private"`), false)
}
func TestTelemetryServerObservedBlockedLoadSchema(t *testing.T) {
	blocked := `{
		"event_version":"1","event_id":"evt_load:blocked","event_type":"skill.loaded",
		"occurred_at":"2026-09-29T10:00:00Z","resolution_id":"res_1",
		"catalog_snapshot":"sha256:catalog","policy_revision":"sha256:policy",
		"client":{"name":"skillhub"},"privacy":{"content_mode":"none","redaction_version":"redact-v1"},
		"payload":{"skill_id":"code-review","basis":"server-observed","status":"review_required","resource_kind":"entrypoint","surface":"skill_get","attribution":"recommended","reason_codes":["content_review_required"]}
	}`
	validateJSON(t, TelemetryEvent, blocked, true)
	// Rejects status: ready on server-observed load
	validateJSON(t, TelemetryEvent, replaceJSON(blocked, `"status":"review_required"`, `"status":"ready"`), false)
	// Rejects first_activation: true on blocked load
	validateJSON(t, TelemetryEvent, replaceJSON(blocked, `"attribution":"recommended"`, `"attribution":"recommended","first_activation":true`), false)
	// Accepts first_activation: false on blocked load
	validateJSON(t, TelemetryEvent, replaceJSON(blocked, `"attribution":"recommended"`, `"attribution":"recommended","first_activation":false`), true)
}


func TestEvaluationCaseSchemaSupportsMultipleOutcomesBranchesAndCounters(t *testing.T) {
	valid := `{
		"schema_version":1,"id":"routing-with-clarification","partition":"calibration",
		"tags":["routing","multiple-valid"],
		"request":{"schema_version":"1","request_id":"case-1","task":{"description":"review and test this change","constraints":["local only"]}},
		"expected":{
			"acceptable_statuses":["resolved","needs_context"],
			"acceptable_primary":["code-review","review-pr"],
			"unacceptable_primary":["deployment"],
			"question":{"id":"scope","field":"task.scope"},
			"branches":{
				"single":{"acceptable_statuses":["resolved"],"acceptable_primary":["code-review","review-pr"]},
				"none":{"acceptable_statuses":["no_skill"],"no_skill":true}
			}
		},
		"counters":{"curation.turns_to_next_action":2,"invocation.correct_reuse":1},
		"provenance":{"source":"sanitized-fixture","reviewed_by":["maintainer"]}
	}`
	validateJSON(t, EvaluationCase, valid, true)

	invalid := []string{
		replaceJSON(valid, `"partition":"calibration"`, `"partition":"private"`),
		replaceJSON(valid, `"invocation.correct_reuse":1`, `"unknown.counter":1`),
		replaceJSON(valid, `"curation.turns_to_next_action":2`, `"curation.turns_to_next_action":-1`),
		replaceJSON(valid, `"provenance":{`, `"unexpected":true,"provenance":{`),
		replaceJSON(valid, `"field":"task.scope"`, `"field":"task.scope","prompt":"raw question"`),
		replaceJSON(valid, `"description":"review and test this change"`, `"description":"`+strings.Repeat("x", 4097)+`"`),
		replaceJSON(valid, `"unacceptable_primary":["deployment"]`, `"unacceptable_primary":["deployment","deployment"]`),
		replaceJSON(valid, `"none":{"acceptable_statuses":["no_skill"],"no_skill":true}`, `"none":{"acceptable_statuses":["resolved"],"acceptable_primary":["code-review"],"no_skill":true}`),
		replaceJSON(valid, `"acceptable_statuses":["resolved","needs_context"]`, `"status":"resolved","acceptable_statuses":["resolved","needs_context"]`),
	}
	for index, document := range invalid {
		t.Run(string(rune('a'+index)), func(t *testing.T) { validateJSON(t, EvaluationCase, document, false) })
	}
}

func TestEvaluationSuiteSchemaSupportsPartitionsAndStrictNestedCases(t *testing.T) {
	valid := `{
		"schema_version":1,"id":"core-v1","sanitization":"synthetic",
		"skills":[{"id":"code-review","collection_id":"core","name":"Code Review","description":"review code","status":"active","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","triggers":["review code"],"not_for":["write code"],"min_scope":"multi_step","reviewed":true}],
		"cases":[
			{"id":"dev","partition":"development","request":{"schema_version":"1","request_id":"dev","task":{"description":"review code"}},"expected":{"acceptable_statuses":["resolved"],"acceptable_primary":["code-review"]}},
			{"schema_version":1,"id":"held","split":"held_out","request":{"schema_version":"1","request_id":"held","task":{"description":"find a skill"}},"expected":{"acceptable_statuses":["no_skill"],"no_skill":true},"counters":{"distillation.completed":1}}
		]
	}`
	validateJSON(t, EvaluationSuite, valid, true)
	validateJSON(t, EvaluationSuite, replaceJSON(valid, `"description":"find a skill"`, `"description":"find a skill","raw_task":"secret"`), false)
	validateJSON(t, EvaluationSuite, replaceJSON(valid, `"distillation.completed":1`, `"distillation.completed":1,"other":2`), false)

	minimalCase := `{"id":"bounded","partition":"development","request":{"schema_version":"1","request_id":"bounded","task":{"description":"bounded"}},"expected":{"acceptable_statuses":["no_skill"],"no_skill":true}}`
	tooManyCases := `{"schema_version":1,"id":"large","cases":[` + strings.Repeat(minimalCase+",", 1000) + minimalCase + `]}`
	validateJSON(t, EvaluationSuite, tooManyCases, false)
}

func TestExperimentManifestSchemaPinsReplayIdentities(t *testing.T) {
	valid := `{
		"schema_version":1,"experiment_id":"exp-1",
		"suite_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"catalog_snapshot":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"policy_revision":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"protocol_schema":"1","normalization_version":"normalize-v1","index_version":"index-v1",
		"fact_provider_fixture":"facts-v1","fact_provider_version":"1","seed":42,
		"binary":{"version":"1.0.0","commit":"abc123","digest":"sha256:binary"},
		"model":{"provider":"local","model":"reranker-v1","configuration":"temperature-0"},
		"variant":"candidate"
	}`
	validateJSON(t, ExperimentManifest, valid, true)

	invalid := []string{
		replaceJSON(valid, `"protocol_schema":"1",`, ``),
		replaceJSON(valid, `"suite_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",`, ``),
		replaceJSON(valid, `"configuration":"temperature-0"`, `"configuration":""`),
		replaceJSON(valid, `"variant":"candidate"`, `"variant":"candidate","latest":true`),
		replaceJSON(valid, `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`, `catalog-latest`),
	}
	for index, document := range invalid {
		t.Run(string(rune('a'+index)), func(t *testing.T) { validateJSON(t, ExperimentManifest, document, false) })
	}
}

func loadSkillMetadataSchema() (*jsonschema.Schema, error) {
	data, err := os.ReadFile("skill-metadata.schema.json")
	if err != nil {
		return nil, err
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

func loadErrorEnvelopeSchema() (*jsonschema.Schema, error) {
	data, err := os.ReadFile("error-envelope.schema.json")
	if err != nil {
		return nil, err
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

func TestSkillMetadataProvenanceAndStructuredOrigin(t *testing.T) {
	legacy := `{
		"schema_version": 1,
		"id": "my-skill",
		"name": "My Skill",
		"status": "draft",
		"description": "Legacy provenance test",
		"routing": {},
		"provenance": {
			"created_by": "source_import",
			"source_id": "gh-source",
			"revision": "v1.0",
			"path": "skills/my-skill"
		}
	}`
	validateJSON(t, loadSkillMetadataSchema, legacy, true)

	validGithub := `{
		"schema_version": 1,
		"id": "my-skill",
		"name": "My Skill",
		"status": "draft",
		"description": "GitHub origin test",
		"routing": {},
		"provenance": {
			"origin": {
				"kind": "github",
				"repository": "https://github.com/anthropics/skills",
				"ref": "main",
				"commit": "8a1541c8a1541c8a1541c8a1541c8a1541c8a154",
				"path": "skills/pdf"
			}
		}
	}`
	validateJSON(t, loadSkillMetadataSchema, validGithub, true)

	validLocal := `{
		"schema_version": 1,
		"id": "my-skill",
		"name": "My Skill",
		"status": "draft",
		"description": "Local origin test",
		"routing": {},
		"provenance": {
			"origin": {
				"kind": "local",
				"name": "pdf",
				"path": "skills/pdf",
				"folder_digest": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"content_digest": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"transformations": ["strip_license"],
				"added_at": "2026-10-01T12:00:00Z"
			}
		}
	}`
	validateJSON(t, loadSkillMetadataSchema, validLocal, true)

	invalidCases := []struct {
		name string
		json string
	}{
		{
			name: "local origin with repository",
			json: replaceJSON(validLocal, `"name": "pdf",`, `"name": "pdf", "repository": "https://github.com/foo/bar",`),
		},
		{
			name: "local origin with absolute path",
			json: replaceJSON(validLocal, `"path": "skills/pdf"`, `"path": "/home/user/skills/pdf"`),
		},
		{
			name: "local origin with tilde path",
			json: replaceJSON(validLocal, `"path": "skills/pdf"`, `"path": "~/skills/pdf"`),
		},
		{
			name: "local origin with path traversal",
			json: replaceJSON(validLocal, `"path": "skills/pdf"`, `"path": "skills/../pdf"`),
		},
		{
			name: "local origin with path in name",
			json: replaceJSON(validLocal, `"name": "pdf"`, `"name": "dir/pdf"`),
		},
		{
			name: "local origin with unknown property",
			json: replaceJSON(validLocal, `"name": "pdf",`, `"name": "pdf", "extra": "unsafe",`),
		},
		{
			name: "invalid digest",
			json: replaceJSON(validLocal, `"folder_digest": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`, `"folder_digest": "md5:bad"`),
		},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			validateJSON(t, loadSkillMetadataSchema, tc.json, false)
		})
	}
}

func TestErrorEnvelopeSchemaAcceptsNewErrorCodes(t *testing.T) {
	codes := []string{
		"ambiguous_locator",
		"ambiguous_ref",
		"skill_selection_required",
		"skill_conflict",
		"source_conflict",
		"resource_limits_exceeded",
		"source_changed",
		"edit_conflict",
		"stale_proposal",
		"validation_failed",
		"local_watch_unsupported",
		"resource_content_unavailable",
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			doc := `{"code":"` + code + `","render":{"ERROR":"failed","WHY":"because","FIX":"retry"}}`
			validateJSON(t, loadErrorEnvelopeSchema, doc, true)
		})
	}
}

func validateJSON(t *testing.T, constructor func() (*jsonschema.Schema, error), document string, wantValid bool) {
	t.Helper()
	schema, err := constructor()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	var value any
	if err := json.Unmarshal([]byte(document), &value); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	err = resolved.Validate(value)
	if wantValid && err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	if !wantValid && err == nil {
		t.Fatal("invalid document accepted")
	}
}

func replaceJSON(document, old, replacement string) string {
	for index := 0; index+len(old) <= len(document); index++ {
		if document[index:index+len(old)] == old {
			return document[:index] + replacement + document[index+len(old):]
		}
	}
	return document
}

const skillMetadataWithRuntimeYAML = `schema_version: 1
id: owner
name: Owner
status: active
description: Route work.
runtime:
  requires:
    bins:
      - python3
      - {name: node, version: ">=18"}
    env: [OPENAI_API_KEY]
    platforms: [linux, darwin]
  setup:
    command: "pip install -r requirements.txt"
    check: "python3 scripts/check_env.py"
routing:
  triggers: [route work]
  not_for: [write prose]
  min_scope: multi_step
  examples: [route this request to the owner skill]
  counter_examples: [write a poem]
quality:
  content_reviewed_digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`

// skillMetadataJSON converts a skill.meta.yaml document to the JSON form the
// committed schema validates.
func skillMetadataJSON(t *testing.T, document string) string {
	t.Helper()
	var value any
	if err := yaml.Unmarshal([]byte(document), &value); err != nil {
		t.Fatalf("parse yaml fixture: %v", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return string(encoded)
}

func TestSkillMetadataSchemaAcceptsRuntimeExamplesAndScriptsReview(t *testing.T) {
	validateJSON(t, loadSkillMetadataSchema, skillMetadataJSON(t, skillMetadataWithRuntimeYAML), true)

	tooMany := make([]string, 11)
	for index := range tooMany {
		tooMany[index] = "example " + strings.Repeat("x", index+1)
	}
	invalid := map[string][2]string{
		"bad bin name":         {"      - python3\n", "      - python 3\n"},
		"bad version":          {`version: ">=18"`, `version: "~18"`},
		"unknown bin field":    {`version: ">=18"}`, `version: ">=18", path: /bin/node}`},
		"env with value":       {"env: [OPENAI_API_KEY]", "env: [OPENAI_API_KEY=secret]"},
		"unknown platform":     {"platforms: [linux, darwin]", "platforms: [plan9]"},
		"multi-line command":   {`command: "pip install -r requirements.txt"`, `command: "pip install\nrm -rf /"`},
		"unknown runtime key":  {"  setup:\n", "  network: true\n  setup:\n"},
		"too many examples":    {"examples: [route this request to the owner skill]", "examples: [" + strings.Join(tooMany, ", ") + "]"},
		"duplicate examples":   {"examples: [route this request to the owner skill]", "examples: [same, same]"},
		"long counter example": {"counter_examples: [write a poem]", "counter_examples: [" + strings.Repeat("x", 301) + "]"},
		"malformed digest":     {"content_reviewed_digest: sha256:0123", "content_reviewed_digest: md5:0123"},
	}
	for name, edit := range invalid {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(skillMetadataWithRuntimeYAML, edit[0]) {
				t.Fatalf("fixture does not contain %q", edit[0])
			}
			document := strings.Replace(skillMetadataWithRuntimeYAML, edit[0], edit[1], 1)
			validateJSON(t, loadSkillMetadataSchema, skillMetadataJSON(t, document), false)
		})
	}
}

func TestTelemetryMeasurementEventsSchema(t *testing.T) {
	envelope := func(eventType, payload string) string {
		return `{
		"event_version":"1","event_id":"evt_measure","event_type":"` + eventType + `",
		"occurred_at":"2026-10-04T10:00:00Z","resolution_id":"res_measure",
		"catalog_snapshot":"sha256:catalog","policy_revision":"sha256:policy",
		"client":{"name":"skillhub"},"privacy":{"content_mode":"none","redaction_version":"redact-v1"},
		"payload":` + payload + `}`
	}

	load := envelope("skill.loaded", `{"skill_id":"code-review","basis":"server-observed","resource_kind":"entrypoint","surface":"skill_get","attribution":"recommended","first_activation":true}`)
	validateJSON(t, TelemetryEvent, load, true)
	validateJSON(t, TelemetryEvent, replaceJSON(load, `"resource_kind":"entrypoint"`, `"resource_kind":"binary"`), false)
	validateJSON(t, TelemetryEvent, replaceJSON(load, `"surface":"skill_get"`, `"surface":"shell"`), false)
	validateJSON(t, TelemetryEvent, replaceJSON(load, `"attribution":"recommended"`, `"attribution":"guessed"`), false)
	validateJSON(t, TelemetryEvent, replaceJSON(load, `"resource_kind":"entrypoint",`, ``), false)
	validateJSON(t, TelemetryEvent, replaceJSON(load, `"attribution":"recommended",`, ``), false)
	validateJSON(t, TelemetryEvent, replaceJSON(load, `"first_activation":true`, `"first_activation":true,"path":"/home/user"`), false)

	setup := envelope("resolution.completed", `{"status":"resolved","top_skill_id":"code-review","recommended_skill_ids":["code-review"],"setup_state":"setup_required"}`)
	validateJSON(t, TelemetryEvent, setup, true)
	validateJSON(t, TelemetryEvent, replaceJSON(setup, `"setup_required"`, `"review_required"`), true)
	validateJSON(t, TelemetryEvent, replaceJSON(setup, `"setup_required"`, `"broken"`), false)

	doctor := envelope("skill.doctor_checked", `{"skill_id":"code-review","status":"setup_required","reason_codes":["missing_bin"],"duration_ms":12}`)
	validateJSON(t, TelemetryEvent, doctor, true)
	validateJSON(t, TelemetryEvent, replaceJSON(doctor, `"status":"setup_required"`, `"status":"maybe"`), false)
	validateJSON(t, TelemetryEvent, replaceJSON(doctor, `"duration_ms":12`, `"duration_ms":12,"output":"raw"`), false)

	transcript := envelope("transcript.tool_observed", `{"tool":"Skill","skill_id":"code-review","source":"claude-code","basis":"transcript","resolved_before":false}`)
	validateJSON(t, TelemetryEvent, transcript, true)
	validateJSON(t, TelemetryEvent, replaceJSON(transcript, `"source":"claude-code"`, `"source":"other"`), false)
	validateJSON(t, TelemetryEvent, replaceJSON(transcript, `"basis":"transcript"`, `"basis":"user"`), false)
	validateJSON(t, TelemetryEvent, replaceJSON(transcript, `"resolved_before":false`, `"resolved_before":false,"prompt":"raw"`), false)

	utility := envelope("skill.utility_reported", `{"skill_id":"code-review","utility":"harmful","basis":"user","after_load":true}`)
	validateJSON(t, TelemetryEvent, utility, true)
	validateJSON(t, TelemetryEvent, replaceJSON(utility, `"after_load":true`, `"after_load":"yes"`), false)
}

func TestResolverResponseAcceptsOptionalSetupOnRecommendations(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	uri := "skill://skillhub/" + strings.Repeat("b", 64) + "/demo/SKILL.md"
	response := func(primarySetup, supportingSetup string) string {
		primary := `{"id":"demo","version":"` + digest + `","uri":"` + uri + `","applicability":"Demo.","confidence":"high"` + primarySetup + `}`
		supporting := `{"id":"demo","version":"` + digest + `","uri":"` + uri + `","role":"review","activation":"on-demand"` + supportingSetup + `}`
		return `{"schema_version":"1","resolution_id":"res-1","request_id":"req-1","context_revision":1,"status":"resolved","catalog_snapshot":"` + digest + `","policy_revision":"` + digest + `","reason_codes":[],"valid_for":{"scope_fingerprint":"` + digest + `"},"primary":` + primary + `,"supporting":[` + supporting + `]}`
	}
	validateJSON(t, ResolverResponse, response("", ""), true)
	validateJSON(t, ResolverResponse, response(`,"setup":{"state":"setup_required","reason_codes":["bin_not_found"],"checked_at":"2026-10-04T10:00:00Z"}`, `,"setup":{"state":"unknown"}`), true)
	validateJSON(t, ResolverResponse, response(`,"setup":{"state":"review_required","reason_codes":["content_review_required"]}`, ""), true)
	validateJSON(t, ResolverResponse, response(`,"setup":{"state":"ready","basis":"terminal","checked_at":"2026-10-04T10:00:00Z"}`, ""), true)
	validateJSON(t, ResolverResponse, response(`,"setup":{"state":"ready","basis":"shell"}`, ""), false)
	validateJSON(t, ResolverResponse, response(`,"setup":{"state":"broken"}`, ""), false)
	validateJSON(t, ResolverResponse, response(`,"setup":{"state":"ready","extra":true}`, ""), false)
	validateJSON(t, ResolverResponse, response("", `,"setup":{"reason_codes":[]}`), false)
}
