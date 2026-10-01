package schemas

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
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
