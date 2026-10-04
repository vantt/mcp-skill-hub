package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"github.com/vantt/mcp-skill-hub/schemas"
)

type panickingTelemetrySink struct{}

func (panickingTelemetrySink) Record(telemetry.Event) { panic("telemetry unavailable") }

type fullTelemetrySink struct{ queue chan telemetry.Event }

func (sink fullTelemetrySink) Record(event telemetry.Event) {
	select {
	case sink.queue <- event:
	default:
	}
}

type captureTelemetrySink struct{ events []telemetry.Event }

func (sink *captureTelemetrySink) Record(event telemetry.Event) {
	encoded, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	var copied telemetry.Event
	if err := json.Unmarshal(encoded, &copied); err != nil {
		panic(err)
	}
	sink.events = append(sink.events, copied)
}

func TestResolverResultIsIndependentOfNilFailingAndFullTelemetry(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	request := privateResolverRequest()
	baseline, baselineErr := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)

	blockingParent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockingParent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	degraded, err := telemetry.Open(telemetry.Config{Path: filepath.Join(blockingParent, "telemetry.db"), BufferSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = degraded.Close(context.Background()) })

	sinks := []struct {
		name string
		sink TelemetrySink
	}{
		{name: "panic", sink: panickingTelemetrySink{}},
		{name: "storage-failure", sink: degraded},
		{name: "full", sink: fullTelemetrySink{queue: make(chan telemetry.Event, 1)}},
	}
	for _, test := range sinks {
		t.Run(test.name, func(t *testing.T) {
			actual, actualErr := (ResolverService{Cache: resolverpkg.NewCache(8), Telemetry: test.sink}).Resolve(t.Context(), root, request)
			if !reflect.DeepEqual(actual, baseline) || errorText(actualErr) != errorText(baselineErr) {
				t.Fatalf("telemetry changed resolution:\nbaseline=%#v, %v\nactual=%#v, %v", baseline, baselineErr, actual, actualErr)
			}
		})
	}
}

func TestResolverTelemetryIsMinimizedAndSeparatesRecommendation(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	createAndActivateSkill(t, SkillService{}, root)
	sink := &captureTelemetrySink{}
	request := privateResolverRequest()
	request.Task.Description = "raw private task: review consumers using secret-token"
	request.Task.Constraints = []string{"never persist this private constraint"}
	request.Task.Scope = "multi_step"
	request.Context.ActiveArtifact = &resolverpkg.Artifact{Kind: "source-file", PathHint: "private/customer.go", Language: "go"}
	request.Context.Facts = []resolverpkg.Fact{{Key: "dependency", Value: "private-fact-value", Basis: "user"}}

	response, err := (ResolverService{Cache: resolverpkg.NewCache(8), Telemetry: sink}).Resolve(t.Context(), root, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Primary == nil {
		t.Fatalf("expected a recommendation, got %#v", response)
	}
	if len(sink.events) != 3 {
		t.Fatalf("event count = %d, want started, recommended, completed", len(sink.events))
	}
	wantTypes := []string{telemetry.EventResolutionStarted, telemetry.EventResolutionRecommended, telemetry.EventResolutionCompleted}
	for index, want := range wantTypes {
		if sink.events[index].Type != want {
			t.Fatalf("event[%d].Type = %q, want %q", index, sink.events[index].Type, want)
		}
	}
	completed := sink.events[2]
	if completed.CatalogSnapshot != response.CatalogSnapshot || completed.PolicyRevision != response.PolicyRevision ||
		completed.RequestID != response.RequestID || completed.ResolutionID != response.ResolutionID {
		t.Fatalf("event IDs do not match returned response: %#v", completed)
	}
	if completed.Payload["status"] != string(response.Status) || completed.Payload["top_skill_id"] != response.Primary.ID ||
		completed.Payload["candidate_count"] != float64(len(response.Supporting)+1) {
		t.Fatalf("event payload does not describe returned response: %#v", completed.Payload)
	}
	encoded, err := json.Marshal(sink.events)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		request.Task.Description, request.Task.Constraints[0], request.Context.Facts[0].Value,
		request.Context.ActiveArtifact.PathHint, "secret-token", "private-fact-value",
	} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("telemetry contains prohibited content %q: %s", prohibited, encoded)
		}
	}
	for _, event := range sink.events {
		if event.Payload["constraint_count"] != float64(1) {
			t.Fatalf("constraint_count missing from %s: %#v", event.Type, event.Payload)
		}
		keys, ok := event.Payload["fact_keys"].([]any)
		if !ok || len(keys) != 1 || keys[0] != "dependency" {
			t.Fatalf("fact keys missing from %s: %#v", event.Type, event.Payload)
		}
	}
}

func TestResolutionTelemetryPayloadRetainsPrimaryAndSupportingRecommendationIDs(t *testing.T) {
	t.Parallel()
	response := resolverpkg.Response{
		Status:     resolverpkg.StatusResolved,
		Primary:    &resolverpkg.Recommendation{ID: "primary-skill", Confidence: "high"},
		Supporting: []resolverpkg.Supporting{{ID: "supporting-one"}, {ID: "supporting-two"}},
	}
	payload := resolutionTelemetryPayload(resolverpkg.Request{}, response, time.Millisecond)
	got, ok := payload["recommended_skill_ids"].([]string)
	if !ok || !reflect.DeepEqual(got, []string{"primary-skill", "supporting-one", "supporting-two"}) {
		t.Fatalf("recommended_skill_ids = %#v", payload["recommended_skill_ids"])
	}

	withoutRecommendation := resolutionTelemetryPayload(resolverpkg.Request{}, resolverpkg.Response{Status: resolverpkg.StatusNoSkill}, 0)
	got, ok = withoutRecommendation["recommended_skill_ids"].([]string)
	if !ok || len(got) != 0 {
		t.Fatalf("no-skill recommended_skill_ids = %#v", withoutRecommendation["recommended_skill_ids"])
	}
}

func TestResolverRecordsSanitizedFailureWithoutChangingError(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	request := privateResolverRequest()
	request.SchemaVersion = "unsupported"
	baseline, baselineErr := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)
	if baselineErr == nil {
		t.Fatal("invalid request unexpectedly resolved")
	}
	sink := &captureTelemetrySink{}
	actual, actualErr := (ResolverService{Cache: resolverpkg.NewCache(8), Telemetry: sink}).Resolve(t.Context(), root, request)
	if !reflect.DeepEqual(actual, baseline) || errorText(actualErr) != errorText(baselineErr) {
		t.Fatalf("telemetry changed validation error: baseline=%v actual=%v", baselineErr, actualErr)
	}
	if len(sink.events) != 2 || sink.events[0].Type != telemetry.EventResolutionStarted || sink.events[1].Type != telemetry.EventResolutionFailed {
		t.Fatalf("failure events = %#v", sink.events)
	}
	if sink.events[1].Payload["status"] != "failed" || sink.events[1].Payload["error_code"] != "resolution_failed" {
		t.Fatalf("failure payload = %#v", sink.events[1].Payload)
	}
}

func TestTelemetryPurgeAndDeletionDoNotAlterResolution(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	request := privateResolverRequest()
	service := TelemetryService{Config: telemetry.Config{BufferSize: 8}}

	validateBefore, err := (WorkspaceService{}).ValidateWorkspace(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	firstRecorder, err := service.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	first, firstErr := (ResolverService{Cache: resolverpkg.NewCache(8), Telemetry: firstRecorder}).Resolve(t.Context(), root, request)
	if err := firstRecorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := service.Purge(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm", ""} {
		if err := os.Remove(filepath.Join(root, "runtime", "telemetry.db") + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}

	secondRecorder, err := service.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	second, secondErr := (ResolverService{Cache: resolverpkg.NewCache(8), Telemetry: secondRecorder}).Resolve(t.Context(), root, request)
	if err := secondRecorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || errorText(firstErr) != errorText(secondErr) {
		t.Fatalf("purge/delete changed resolution:\nfirst=%#v, %v\nsecond=%#v, %v", first, firstErr, second, secondErr)
	}
	validateAfter, err := (WorkspaceService{}).ValidateWorkspace(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := (CatalogService{}).BuildCatalogGeneration(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(validateBefore, validateAfter) {
		t.Fatalf("purge/delete changed validation behavior: before=%#v after=%#v", validateBefore, validateAfter)
	}
	var rebuiltSnapshot string
	for _, item := range rebuilt.Items {
		if item.ID == "catalog_snapshot" {
			rebuiltSnapshot = item.Summary
		}
	}
	if rebuiltSnapshot == "" || rebuiltSnapshot != first.CatalogSnapshot {
		t.Fatalf("full rebuild snapshot = %q, want %q; result=%#v", rebuiltSnapshot, first.CatalogSnapshot, rebuilt)
	}
	third, thirdErr := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)
	if !reflect.DeepEqual(first, third) || errorText(firstErr) != errorText(thirdErr) {
		t.Fatalf("full rebuild after purge changed resolution:\nfirst=%#v, %v\nthird=%#v, %v", first, firstErr, third, thirdErr)
	}
}

func newResolverWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	return root
}

func privateResolverRequest() resolverpkg.Request {
	return resolverpkg.Request{
		SchemaVersion: resolverpkg.SchemaVersion,
		RequestID:     "req-telemetry-test",
		Task: resolverpkg.Task{
			Description: "private raw task with customer details",
			Constraints: []string{"private raw constraint"},
			Scope:       "multi_step",
		},
		Operation: "review",
		Context: resolverpkg.RequestContext{
			ActiveArtifact: &resolverpkg.Artifact{Kind: "source-file", PathHint: "private/customer.go", Language: "go"},
			Facts:          []resolverpkg.Fact{{Key: "dependency", Value: "private raw fact value", Basis: "user"}},
		},
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func setupResolverRequest() resolverpkg.Request {
	return resolverpkg.Request{
		SchemaVersion: resolverpkg.SchemaVersion,
		RequestID:     "req-setup-annotation",
		Task:          resolverpkg.Task{Description: "review the mutable workspace fixture", Scope: "multi_step"},
		Operation:     "review",
	}
}

func withoutSetup(response resolverpkg.Response) resolverpkg.Response {
	if response.Primary != nil {
		primary := *response.Primary
		primary.Setup = nil
		response.Primary = &primary
	}
	supporting := make([]resolverpkg.Supporting, len(response.Supporting))
	for index, entry := range response.Supporting {
		entry.Setup = nil
		supporting[index] = entry
	}
	response.Supporting = supporting
	return response
}

func validateResolveResponse(t *testing.T, response resolverpkg.Response) {
	t.Helper()
	schema, err := schemas.ResolverResponse()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(value); err != nil {
		t.Fatalf("response does not match the committed schema: %v\n%s", err, encoded)
	}
}

func TestResolverOmitsSetupForSkillsWithoutRuntime(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "setup-plain", "Setup Plain")
	response, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, setupResolverRequest())
	if err != nil {
		t.Fatal(err)
	}
	if response.Primary == nil || response.Primary.ID != "setup-plain" {
		t.Fatalf("expected setup-plain as primary, got %#v", response)
	}
	if response.Primary.Setup != nil {
		t.Fatalf("setup must be absent without a runtime block: %#v", response.Primary.Setup)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"setup"`) {
		t.Fatalf("serialized response contains setup: %s", encoded)
	}
}

func TestResolverSetupAnnotationFollowsPlatformAndDoctorHintWithoutChangingRanking(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "setup-runtime", "Setup Runtime")
	writeSkillFile(t, root, "setup-runtime", "scripts/check.py", "print('ok')\n")
	updateSkillMeta(t, root, "setup-runtime", func(document map[string]any) {
		withRuntime(document)
		document["runtime"].(map[string]any)["requires"].(map[string]any)["platforms"] = []any{"linux"}
	})
	request := setupResolverRequest()
	linux := runtimeProbe{goos: "linux"}
	windows := runtimeProbe{goos: "windows"}

	resolve := func(probe runtimeProbe, sink TelemetrySink) resolverpkg.Response {
		t.Helper()
		response, err := (ResolverService{Cache: resolverpkg.NewCache(8), Telemetry: sink, probe: probe}).Resolve(t.Context(), root, request)
		if err != nil {
			t.Fatal(err)
		}
		if response.Primary == nil || response.Primary.ID != "setup-runtime" || response.Primary.Setup == nil {
			t.Fatalf("expected an annotated setup-runtime primary, got %#v", response)
		}
		validateResolveResponse(t, response)
		return response
	}

	sink := &captureTelemetrySink{}
	unknown := resolve(linux, sink)
	if unknown.Primary.Setup.State != "unknown" || unknown.Primary.Setup.CheckedAt != "" || unknown.Primary.Setup.Basis != "" || !reflect.DeepEqual(unknown.Primary.Setup.ReasonCodes, []string{"doctor_not_run"}) {
		t.Fatalf("no doctor cache setup = %#v", unknown.Primary.Setup)
	}
	completed := sink.events[len(sink.events)-1]
	if completed.Type != telemetry.EventResolutionCompleted || completed.Payload["setup_state"] != "unknown" {
		t.Fatalf("resolution telemetry lacks setup_state: %#v", completed)
	}

	unsupported := resolve(windows, nil)
	if unsupported.Primary.Setup.State != "unsupported_platform" || !reflect.DeepEqual(unsupported.Primary.Setup.ReasonCodes, []string{"platform_unsupported"}) {
		t.Fatalf("wrong platform setup = %#v", unsupported.Primary.Setup)
	}

	spec, _, err := skillruntime.ParseSpec(readCatalogContentJSON(t, root, "setup-runtime"))
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := doctorFingerprint(unknown.Primary.Version, spec)
	checkedAt := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	if err := skillruntime.WriteCache(root, "setup-runtime", fingerprint, skillruntime.Result{State: skillruntime.StateReady, Basis: skillruntime.BasisTerminal, CheckedAt: checkedAt}); err != nil {
		t.Fatal(err)
	}
	ready := resolve(linux, nil)
	if ready.Primary.Setup.State != "ready" || ready.Primary.Setup.Basis != "terminal" || ready.Primary.Setup.CheckedAt != "2026-10-04T10:00:00Z" || len(ready.Primary.Setup.ReasonCodes) != 0 {
		t.Fatalf("passing doctor cache setup = %#v", ready.Primary.Setup)
	}

	if err := skillruntime.WriteCache(root, "setup-runtime", fingerprint, skillruntime.Result{State: skillruntime.StateSetupRequired, Basis: skillruntime.BasisTerminal, CheckedAt: checkedAt}); err != nil {
		t.Fatal(err)
	}
	failing := resolve(linux, nil)
	if failing.Primary.Setup.State != "setup_required" || failing.Primary.Setup.Basis != "terminal" || !reflect.DeepEqual(failing.Primary.Setup.ReasonCodes, []string{"doctor_setup_required"}) {
		t.Fatalf("failing doctor cache setup = %#v", failing.Primary.Setup)
	}

	baseline := withoutSetup(unknown)
	for name, response := range map[string]resolverpkg.Response{"unsupported": unsupported, "ready": ready, "failing": failing} {
		if !reflect.DeepEqual(withoutSetup(response), baseline) {
			t.Fatalf("%s setup changed the resolution:\nbaseline=%#v\nactual=%#v", name, baseline, withoutSetup(response))
		}
	}
}

func TestResolverAnnotatesUnapprovedThirdPartySkillAsReviewRequired(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "setup-third", "Setup Third")
	updateSkillMeta(t, root, "setup-third", markThirdParty)
	request := setupResolverRequest()

	response, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Primary == nil || response.Primary.ID != "setup-third" || response.Primary.Setup == nil {
		t.Fatalf("an unapproved third-party skill must still be recommended with setup: %#v", response)
	}
	if setup := response.Primary.Setup; setup.State != "review_required" || !reflect.DeepEqual(setup.ReasonCodes, []string{skillruntime.ReasonContentReviewRequired}) {
		t.Fatalf("setup = %#v", setup)
	}
	validateResolveResponse(t, response)

	// Approving the exact content digest clears the annotation without
	// changing the recommendation.
	review, err := (SkillService{}).ReviewSkill(t.Context(), root, "setup-third")
	if err != nil {
		t.Fatal(err)
	}
	updateSkillMeta(t, root, "setup-third", setReviewedDigest(review.ContentTrust.ContentDigest))
	approved, err := (ResolverService{Cache: resolverpkg.NewCache(8)}).Resolve(t.Context(), root, request)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Primary == nil || approved.Primary.Setup != nil {
		t.Fatalf("approved skill without a runtime block must have no setup: %#v", approved.Primary)
	}
	before, after := withoutSetup(response).Primary, withoutSetup(approved).Primary
	if before.ID != after.ID || before.Version != after.Version || before.URI != after.URI || before.Applicability != after.Applicability || before.Confidence != after.Confidence {
		t.Fatalf("approval changed the recommendation:\n%#v\n%#v", before, after)
	}
}
