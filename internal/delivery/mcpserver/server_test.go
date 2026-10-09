package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
	contractschemas "github.com/vantt/mcp-skill-hub/schemas"
)

func findPropertySchema(schema *jsonschema.Schema, name string) *jsonschema.Schema {
	if schema == nil {
		return nil
	}
	if property := schema.Properties[name]; property != nil {
		return property
	}
	for _, collection := range []map[string]*jsonschema.Schema{schema.Properties, schema.Defs} {
		for _, child := range collection {
			if found := findPropertySchema(child, name); found != nil {
				return found
			}
		}
	}
	for _, child := range schema.AnyOf {
		if found := findPropertySchema(child, name); found != nil {
			return found
		}
	}
	return findPropertySchema(schema.Items, name)
}

func expectedToolAnnotations() map[string][4]bool {
	// Values are readOnly, destructive, idempotent, openWorld.
	return map[string][4]bool{
		"skill_resolve":            {true, false, true, false},
		"skill_feedback":           {false, false, true, false},
		"source_list":              {true, false, false, false},
		"source_check":             {false, false, false, true},
		"source_import_preview":    {false, false, false, false},
		"source_import_confirm":    {false, true, true, false},
		"source_watch_preview":     {false, false, false, true},
		"source_watch_confirm":     {false, true, true, false},
		"skill_upstream_status":    {true, false, false, false},
		"skill_create_preview":     {false, false, false, false},
		"skill_create_confirm":     {false, true, true, false},
		"skill_transition_preview": {false, false, false, false},
		"skill_transition_confirm": {false, true, true, false},
		"skill_list":               {true, false, false, false},
		"skill_get":                {true, false, false, false},
		"skill_add_preview":        {false, false, false, true},
		"skill_add_confirm":        {false, true, true, false},
		"skill_review":             {true, false, false, false},
		"skill_update_preview":     {false, false, false, false},
		"skill_update_confirm":     {false, true, true, false},
		"routing_evaluate":         {true, false, true, false},
		"curation_session_record":  {false, false, true, false},
		"hub_status":               {true, false, false, false},
		"workspace_validate":       {true, false, false, false},
		"workspace_rebuild":        {false, false, false, false},
		"workspace_diff":           {true, false, false, false},
	}
}

func TestNewDoesNotOpenTelemetry(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	databasePath := filepath.Join(root, "runtime", "telemetry.db")
	if err := os.RemoveAll(databasePath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New(root, nil); err != nil {
		t.Fatal(err)
	}
	_, err := os.Stat(databasePath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("New created a telemetry lifecycle for an in-memory caller: %v", err)
	}
}

func TestModernAndLegacySDKContracts(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	for _, version := range []string{"2026-07-28", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			_, server, err := New(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			clientCaps := &mcp.ClientCapabilities{}
			clientCaps.AddExtension("io.modelcontextprotocol/skills", map[string]any{})
			client := mcp.NewClient(&mcp.Implementation{Name: "skillhub-test", Version: "1"}, &mcp.ClientOptions{Capabilities: clientCaps})
			if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
				t.Fatal(err)
			}
			if err := mcp.AddSendingCustomMethod[*getSkillParams, *getSkillResult](client, "skills/get"); err != nil {
				t.Fatal(err)
			}
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(t.Context(), serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = serverSession.Close() })
			session, err := client.Connect(t.Context(), clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: version})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = session.Close() })

			if got := session.InitializeResult().ProtocolVersion; got != version {
				t.Fatalf("protocol = %q, want %q", got, version)
			}
			settings, ok := session.InitializeResult().Capabilities.Extensions["io.modelcontextprotocol/skills"].(map[string]any)
			if !ok || settings["directoryRead"] != false {
				t.Fatalf("skills capability = %#v", session.InitializeResult().Capabilities.Extensions)
			}
			listedTools, err := session.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			wantAnnotations := expectedToolAnnotations()
			if len(listedTools.Tools) != len(wantAnnotations) {
				for _, tool := range listedTools.Tools {
					if _, ok := wantAnnotations[tool.Name]; !ok {
						t.Logf("EXTRA TOOL: %s", tool.Name)
					}
				}
				t.Fatalf("tool count = %d, want %d", len(listedTools.Tools), len(wantAnnotations))
			}
			for _, tool := range listedTools.Tools {
				inputSchema, _ := json.Marshal(tool.InputSchema)
				if !strings.Contains(string(inputSchema), `"additionalProperties":false`) {
					t.Fatalf("tool %s input schema is not strict: %s", tool.Name, inputSchema)
				}
				outputSchema, _ := json.Marshal(tool.OutputSchema)
				for schemaKind, encoded := range map[string][]byte{"input": inputSchema, "output": outputSchema} {
					if strings.Contains(string(encoded), `"type":[`) {
						t.Fatalf("tool %s %s schema uses a non-portable multi-type declaration: %s", tool.Name, schemaKind, encoded)
					}
				}
				if tool.Annotations == nil || tool.Annotations.OpenWorldHint == nil || tool.Annotations.DestructiveHint == nil {
					t.Fatalf("tool %s annotations are incomplete", tool.Name)
				}
				want, exists := wantAnnotations[tool.Name]
				if !exists {
					t.Fatalf("unexpected registration %s", tool.Name)
				}
				got := [4]bool{tool.Annotations.ReadOnlyHint, *tool.Annotations.DestructiveHint, tool.Annotations.IdempotentHint, *tool.Annotations.OpenWorldHint}
				if got != want {
					t.Fatalf("tool %s annotations = %v, want %v", tool.Name, got, want)
				}
				delete(wantAnnotations, tool.Name)
				if tool.Name == "skill_feedback" {
					encoded := string(inputSchema)
					for _, required := range []string{`"loaded"`, `"scope_mismatch"`, `"helpful"`, `"controlled-benchmark"`} {
						if !strings.Contains(encoded, required) {
							t.Fatalf("feedback discovery schema omits enum value %s: %s", required, encoded)
						}
					}
				}
				if tool.Name == "curation_session_record" {
					encoded := string(inputSchema)
					for _, prohibited := range []string{`"task"`, `"path"`, `"content"`, `"conversation"`} {
						if strings.Contains(encoded, prohibited) {
							t.Fatalf("curation telemetry schema accepts content field %s: %s", prohibited, encoded)
						}
					}
					for _, required := range []string{`"schema_version"`, `"event_id"`, `"status"`, `"basis"`, `"routine_git_noise"`} {
						if !strings.Contains(encoded, required) {
							t.Fatalf("curation telemetry schema omits %s: %s", required, encoded)
						}
					}
					if !strings.Contains(encoded, `"maximum":10000`) || !strings.Contains(encoded, `"host-reported"`) || !strings.Contains(encoded, `"controlled-benchmark"`) {
						t.Fatalf("curation telemetry schema lacks measurement bounds or basis enum: %s", encoded)
					}
				}
				if tool.Name == "skill_resolve" || tool.Name == "routing_evaluate" {
					committed, schemaErr := contractschemas.ResolverRequest()
					if schemaErr != nil {
						t.Fatal(schemaErr)
					}
					wantJSON, _ := json.Marshal(committed)
					var gotContract, wantContract any
					if json.Unmarshal(inputSchema, &gotContract) != nil || json.Unmarshal(wantJSON, &wantContract) != nil || !reflect.DeepEqual(gotContract, wantContract) {
						t.Fatalf("tool %s discovery schema differs from committed resolver request schema\n got: %s\nwant: %s", tool.Name, inputSchema, wantJSON)
					}
					committedResponse, schemaErr := contractschemas.ResolverResponse()
					if schemaErr != nil {
						t.Fatal(schemaErr)
					}
					var output jsonschema.Schema
					if err := json.Unmarshal(outputSchema, &output); err != nil {
						t.Fatalf("decode %s output schema: %v", tool.Name, err)
					}
					discoveredResponse := findPropertySchema(&output, "resolution")
					gotJSON, _ := json.Marshal(discoveredResponse)
					wantJSON, _ = json.Marshal(committedResponse)
					if json.Unmarshal(gotJSON, &gotContract) != nil || json.Unmarshal(wantJSON, &wantContract) != nil || !reflect.DeepEqual(gotContract, wantContract) {
						t.Fatalf("tool %s output resolution schema differs from committed response schema", tool.Name)
					}
				}
			}
			if len(wantAnnotations) != 0 {
				t.Fatalf("missing tool registrations: %#v", wantAnnotations)
			}
			for _, tool := range listedTools.Tools {
				malformed, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool.Name, Arguments: map[string]any{"unexpected": true}})
				if callErr != nil || !malformed.IsError {
					t.Fatalf("tool %s did not reject an unknown input field: result=%#v err=%v", tool.Name, malformed, callErr)
				}
			}
			status, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "hub_status", Arguments: map[string]any{}})
			if err != nil || status.IsError || status.StructuredContent == nil {
				t.Fatalf("hub_status = %#v, %v", status, err)
			}
			listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
			if err != nil || listed.ResultType != "complete" || listed.TTLMS <= 0 || listed.CacheScope == "" {
				t.Fatalf("skills/list = %#v, %v", listed, err)
			}
			entry := findSkillEntryByName(t, listed.Skills, "review-skill")
			if len(entry.Resources) != 2 {
				t.Fatalf("workspace skill entry = %#v", entry)
			}
			curator := findSkillEntryByName(t, listed.Skills, systemskills.CuratorSkillID)
			if curator.Frontmatter["version"] != systemskills.CuratorSkillVersion || curator.Frontmatter["contract-version"] != systemskills.CuratorContractVersion || len(curator.Resources) != 1 {
				t.Fatalf("bundled curator entry = %#v", curator)
			}
			resolved, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_resolve", Arguments: map[string]any{
				"schema_version": "1", "request_id": "REQ-contract-" + version,
				"task": map[string]any{"description": "review changed code", "scope": "multi_step"}, "operation": "review",
			}})
			if err != nil || resolved.IsError {
				t.Fatalf("skill_resolve = %#v, %v", resolved, err)
			}
			var resolution toolOutcome[resolveResult]
			decodeStructuredContent(t, resolved, &resolution)
			if resolution.Result == nil || resolution.Result.Resolution.Primary == nil || resolution.Result.Resolution.Primary.URI != entry.URI || resolution.Result.Resolution.Primary.Version == "" {
				t.Fatalf("resolved distribution pins = %#v", resolution)
			}
			for _, listedEntry := range listed.Skills {
				if listedEntry.Local != nil {
					t.Fatalf("skills/list must not export local snapshots: %#v", listedEntry)
				}
			}
			got, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), session, "skills/get", &getSkillParams{URI: entry.URI})
			if err != nil || got.Skill.URI != entry.URI {
				t.Fatalf("skills/get = %#v, %v", got, err)
			}
			snapshotRoot := filepath.Join(root, "runtime", "cache", "skills") + string(filepath.Separator)
			if got.Skill.Local == nil || got.Skill.Local.Status != app.LocalStatusReady || !filepath.IsAbs(got.Skill.Local.Path) || !strings.HasPrefix(got.Skill.Local.Path, snapshotRoot) {
				t.Fatalf("skills/get local = %#v", got.Skill.Local)
			}
			resource, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: entry.URI})
			if err != nil || len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, "Review carefully") {
				t.Fatalf("workspace resources/read = %#v, %v", resource, err)
			}
			if localPath, _ := resource.Contents[0].Meta["io.skillhub/local_path"].(string); localPath != filepath.Join(got.Skill.Local.Path, "SKILL.md") {
				t.Fatalf("resources/read _meta = %#v", resource.Contents[0].Meta)
			}
			gotCurator, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), session, "skills/get", &getSkillParams{URI: curator.URI})
			if err != nil || gotCurator.Skill.URI != curator.URI || gotCurator.Skill.Frontmatter["name"] != systemskills.CuratorSkillID || gotCurator.Skill.Local != nil {
				t.Fatalf("bundled curator skills/get = %#v, %v", gotCurator, err)
			}
			curatorResource, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: curator.URI})
			if err != nil || len(curatorResource.Contents) != 1 || curatorResource.Contents[0].Text != systemskills.CuratorSkill || curatorResource.Contents[0].Meta != nil {
				t.Fatalf("bundled curator resources/read = %#v, %v", curatorResource, err)
			}
		})
	}
}

func TestResolverLoadsOnlySelectedDistributionEntries(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	badDirectory := filepath.Join(root, "skills", "core", "unrelated-bad")
	if err := os.MkdirAll(badDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := "schema_version: 1\nid: unrelated-bad\nname: Unrelated Bad\nstatus: active\ndescription: A malformed distribution fixture.\nrouting:\n  operations: [research]\n  triggers: [unrelated catalog research]\n  not_for: [review code]\n  min_scope: multi_step\nquality:\n  reviewed: true\n"
	if err := os.WriteFile(filepath.Join(badDirectory, "skill.meta.yaml"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDirectory, "SKILL.md"), []byte("# Missing frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	response, err := (app.ResolverService{}).Resolve(t.Context(), root, resolver.Request{
		SchemaVersion: "1", RequestID: "REQ-targeted", Operation: "review",
		Task: resolver.Task{Description: "review changed code carefully", Scope: "multi_step"},
	})
	if err != nil || response.Primary == nil || response.Primary.ID != "review-skill" || response.Primary.URI == "" || response.Primary.Version == "" {
		t.Fatalf("targeted resolution = %#v, %v", response, err)
	}
	entry, err := (app.DistributionService{}).GetSkill(t.Context(), root, response.Primary.URI)
	if err != nil || entry.Version != response.Primary.Version {
		t.Fatalf("resolved distribution entry = %#v, %v", entry, err)
	}
}

func TestDistributionUsesTopLevelSkillEntrypoint(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	nested := filepath.Join(root, "skills", "core", "review-skill", "references", "SKILL.md")
	if err := os.WriteFile(nested, []byte("---\nname: nested-skill\ndescription: Supporting content only.\n---\n\n# Nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	entries, _, err := (app.DistributionService{}).ListSkills(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	entry := findDistributedSkillByID(t, entries, "review-skill")
	if entry.Frontmatter["name"] != "review-skill" || len(entry.Resources) != 3 {
		t.Fatalf("distributed workspace entry = %#v", entry)
	}
	findDistributedSkillByID(t, entries, systemskills.CuratorSkillID)
	content, err := (app.DistributionService{}).ReadResource(t.Context(), root, entry.URI)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content.Bytes), "Review carefully") {
		t.Fatalf("entrypoint selected nested SKILL.md: %q", content.Bytes)
	}
}

func TestPaginationAndConfirmationBoundaries(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	adapter, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := (app.TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter.resolver.Telemetry = recorder
	adapter.feedback.Recorder = recorder
	adapter.curationUX.Recorder = recorder
	t.Cleanup(func() { _ = recorder.Close(context.Background()) })
	client := mcp.NewClient(&mcp.Implementation{Name: "boundary-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	oversizedPage, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "source_list", Arguments: map[string]any{"limit": paging.MaximumLimit + 1}})
	if err != nil || !oversizedPage.IsError {
		t.Fatalf("oversized page = %#v, %v", oversizedPage, err)
	}
	stalePage, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "source_list", Arguments: map[string]any{"cursor": paging.EncodeCursor("stale", "source_list", "last")}})
	if err != nil || !stalePage.IsError {
		t.Fatalf("stale page = %#v, %v", stalePage, err)
	}
	var staleOutcome toolOutcome[paging.Page[app.SourceListItem]]
	decodeStructuredContent(t, stalePage, &staleOutcome)
	if staleOutcome.Error == nil || staleOutcome.Error.Code != "snapshot_expired" {
		t.Fatalf("stale cursor outcome = %#v", staleOutcome)
	}

	previewResult, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_update_preview", Arguments: map[string]any{
		"skill_id":    "review-skill",
		"description": "Updated only after exact confirmation.",
	}})
	if err != nil || previewResult.IsError {
		t.Fatalf("update preview = %#v, %v", previewResult, err)
	}
	var previewOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, previewResult, &previewOutcome)
	if previewOutcome.Result == nil {
		t.Fatal("update preview omitted structured result")
	}
	pins := previewOutcome.Result.Confirmation.Confirmation.Pins
	wrongConfirm, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_update_confirm", Arguments: map[string]any{
		"proposal_id":     pins.ProposalID,
		"proposal_digest": "sha256:" + strings.Repeat("0", 64),
		"base_version":    pins.BaseVersion,
	}})
	if err != nil || !wrongConfirm.IsError {
		t.Fatalf("wrong confirmation = %#v, %v", wrongConfirm, err)
	}
	current, err := (app.SkillService{}).ReadSkill(t.Context(), root, "review-skill")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(current.Content), "Updated only after exact confirmation.") {
		t.Fatal("wrong confirmation pins changed canonical skill content")
	}

	request := map[string]any{
		"schema_version": "1", "request_id": "REQ-evaluate",
		"task":      map[string]any{"description": "review changed code", "scope": "multi_step"},
		"operation": "review",
	}
	evaluated, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_resolve", Arguments: request})
	if err != nil || evaluated.IsError {
		t.Fatalf("skill_resolve = %#v, %v", evaluated, err)
	}
	var evaluation toolOutcome[resolveResult]
	decodeStructuredContent(t, evaluated, &evaluation)
	if evaluation.Result == nil || evaluation.Result.Resolution.CatalogSnapshot == "" {
		t.Fatalf("routing evaluation = %#v", evaluation)
	}

	feedbackArgs := map[string]any{"schema_version": "1", "resolution_id": evaluation.Result.Resolution.ResolutionID, "event_id": "EV-test", "outcome": "used"}
	for attempt := 0; attempt < 2; attempt++ {
		feedback, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_feedback", Arguments: feedbackArgs})
		if callErr != nil || feedback.IsError {
			t.Fatalf("skill_feedback attempt %d = %#v, %v", attempt, feedback, callErr)
		}
		var outcome toolOutcome[app.FeedbackResult]
		decodeStructuredContent(t, feedback, &outcome)
		if outcome.Result == nil || outcome.Result.PolicyMutated || outcome.Result.Deduplicated != (attempt == 1) {
			t.Fatalf("skill_feedback attempt %d = %#v", attempt, outcome)
		}
	}
	invalidFeedback, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_feedback", Arguments: map[string]any{
		"schema_version": "1", "resolution_id": "RES-test", "event_id": "EV-invalid", "outcome": "helpful",
	}})
	if err != nil || !invalidFeedback.IsError {
		t.Fatalf("invalid feedback = %#v, %v", invalidFeedback, err)
	}

	curationArgs := map[string]any{
		"schema_version": "1", "event_id": "evt_curation_boundary", "status": "completed", "basis": "host-reported",
		"turns_to_next_action": 1, "unnecessary_confirmations": 0, "prompts_per_batch": 1,
		"batch_size": 2, "auto_finalized": true, "recovery_completed": false,
		"routine_git_noise": 0, "duration_ms": 800,
	}
	for attempt := 0; attempt < 2; attempt++ {
		recorded, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "curation_session_record", Arguments: curationArgs})
		if callErr != nil || recorded.IsError {
			t.Fatalf("curation_session_record attempt %d = %#v, %v", attempt, recorded, callErr)
		}
		var outcome toolOutcome[app.CurationSessionResult]
		decodeStructuredContent(t, recorded, &outcome)
		if outcome.Result == nil || outcome.Result.CanonicalMutated || outcome.Result.PolicyMutated || outcome.Result.Deduplicated != (attempt == 1) {
			t.Fatalf("curation_session_record attempt %d = %#v", attempt, outcome)
		}
	}
	conflictingArgs := make(map[string]any, len(curationArgs))
	for key, value := range curationArgs {
		conflictingArgs[key] = value
	}
	conflictingArgs["batch_size"] = 3
	conflict, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "curation_session_record", Arguments: conflictingArgs})
	if err != nil || !conflict.IsError {
		t.Fatalf("curation measurement conflict = %#v, %v", conflict, err)
	}
}

func TestPlainNewCurationRecorderOwnsShortLivedLifecycle(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	_, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "plain-new-curation", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "curation_session_record", Arguments: map[string]any{
		"schema_version": "1", "event_id": "evt_plain_new", "status": "partial", "basis": "controlled-benchmark",
		"turns_to_next_action": 0, "error_code": "partial_failure",
	}})
	if err != nil || result.IsError {
		t.Fatalf("plain New curation record = %#v, %v", result, err)
	}
	preview, err := (app.TelemetryService{}).Preview(t.Context(), root, 0)
	if err != nil || preview.Events != 1 || !bytes.Contains(preview.JSONL, []byte(`"event_id":"evt_plain_new"`)) {
		t.Fatalf("plain New telemetry preview = %+v, %v", preview, err)
	}
}

func TestPlainNewFeedbackHasNoImplicitTelemetryLifecycle(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	_, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "plain-new", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_feedback", Arguments: map[string]any{
		"schema_version": "1", "resolution_id": "res_absent", "event_id": "evt_absent", "outcome": "used",
	}})
	if err != nil || !result.IsError {
		t.Fatalf("plain New feedback = %#v, %v", result, err)
	}
	var outcome toolOutcome[app.FeedbackResult]
	decodeStructuredContent(t, result, &outcome)
	if outcome.Error == nil || outcome.Error.Code != "unknown_resolution" {
		t.Fatalf("plain New feedback outcome = %#v", outcome)
	}
}

// Runs serially: it asserts on the process-global MCP diagnostics logger that New replaces.
func TestSafeErrorMappingDoesNotLeakUnknownDetails(t *testing.T) {
	root := newMCPWorkspace(t)
	var diagnostics bytes.Buffer
	if _, _, err := New(root, &diagnostics); err != nil {
		t.Fatal(err)
	}
	known := safeToolError(errors.New("event_id is required"))
	if known.Code != "invalid_request" || known.CorrelationID != "" {
		t.Fatalf("known validation mapping = %#v", known)
	}
	locked := safeToolError(mutation.ErrWorkspaceBusy)
	if locked.Code != "internal_error" || !locked.Retryable || locked.CorrelationID == "" {
		t.Fatalf("lock mapping = %#v", locked)
	}
	unknownText := "database is locked at /private/workspace/secret.db"
	unknown := safeToolError(errors.New(unknownText))
	if unknown.Code != "internal_error" || !unknown.Retryable || unknown.CorrelationID == "" {
		t.Fatalf("unknown mapping = %#v", unknown)
	}
	encoded, _ := json.Marshal(unknown)
	if strings.Contains(string(encoded), unknownText) || strings.Contains(string(encoded), "/private/") {
		t.Fatalf("unknown details leaked in response: %s", encoded)
	}
	if !strings.Contains(diagnostics.String(), unknown.CorrelationID) || strings.Contains(diagnostics.String(), unknownText) || strings.Contains(diagnostics.String(), "/private/") {
		t.Fatalf("stderr diagnostics are unsafe or uncorrelated: %q", diagnostics.String())
	}
}

func findSkillEntryByName(t *testing.T, entries []skillEntry, name string) skillEntry {
	t.Helper()
	var found *skillEntry
	for index := range entries {
		if entries[index].Frontmatter["name"] != name {
			continue
		}
		if found != nil {
			t.Fatalf("skills/list returned duplicate %q entries: %#v", name, entries)
		}
		found = &entries[index]
	}
	if found == nil {
		t.Fatalf("skills/list omitted %q: %#v", name, entries)
	}
	return *found
}

func findDistributedSkillByID(t *testing.T, entries []app.DistributedSkill, id string) app.DistributedSkill {
	t.Helper()
	var found *app.DistributedSkill
	for index := range entries {
		if entries[index].SkillID != id {
			continue
		}
		if found != nil {
			t.Fatalf("distribution returned duplicate %q entries: %#v", id, entries)
		}
		found = &entries[index]
	}
	if found == nil {
		t.Fatalf("distribution omitted %q: %#v", id, entries)
	}
	return *found
}

func decodeStructuredContent(t *testing.T, result *mcp.CallToolResult, target any) {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode structured content: %v; payload=%s", err, data)
	}
}

func TestSkillSnapshotExpiryAndMalformedCursor(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	adapter := app.DistributionService{}
	entries, _, err := adapter.ListSkills(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	oldURI := findDistributedSkillByID(t, entries, "review-skill").URI
	path := filepath.Join(root, "skills", "core", "review-skill", "SKILL.md")
	updated := "---\nname: review-skill\ndescription: Review changed code safely.\nlicense: Apache-2.0\n---\n\n# Review\n\nReview carefully and verify tests.\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetSkill(t.Context(), root, oldURI); !errors.Is(err, skill.ErrSnapshotExpired) {
		t.Fatalf("old URI error = %v", err)
	}

	_, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, _ := server.Connect(t.Context(), st, nil)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	_, err = mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), cs, "skills/list", &listSkillsParams{Cursor: "not-a-cursor"})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "snapshot_expired") {
		t.Fatalf("malformed cursor error = %#v", err)
	}
}

func TestDistributionRPCErrorSplitsNotFoundFromSnapshotExpired(t *testing.T) {
	t.Parallel()

	adapter := &Server{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// 1. skill.ErrNotFound returns skill_not_found code
	errNotFound := adapter.distributionRPCError(context.Background(), nil, skill.ErrNotFound)
	var rpcErr *jsonrpc.Error
	if !errors.As(errNotFound, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "skill_not_found") {
		t.Fatalf("expected skill_not_found error, got: %#v", errNotFound)
	}

	// 2. skill.ErrSnapshotExpired returns snapshot_expired code
	errExpired := adapter.distributionRPCError(context.Background(), nil, skill.ErrSnapshotExpired)
	if !errors.As(errExpired, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "snapshot_expired") {
		t.Fatalf("expected snapshot_expired error, got: %#v", errExpired)
	}
}

func TestGetSkillAndReadResourceInvalidURI(t *testing.T) {
	t.Parallel()

	adapter := &Server{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// 1. getSkill with nil params
	_, err := adapter.getSkill(context.Background(), nil, nil)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "invalid_uri") {
		t.Fatalf("expected invalid_uri for nil params, got: %#v", err)
	}

	// 2. getSkill with empty URI
	_, err = adapter.getSkill(context.Background(), nil, &getSkillParams{URI: ""})
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "invalid_uri") {
		t.Fatalf("expected invalid_uri for empty URI, got: %#v", err)
	}

	// 3. getSkill with oversized URI (>4096 bytes)
	_, err = adapter.getSkill(context.Background(), nil, &getSkillParams{URI: strings.Repeat("x", 4097)})
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "invalid_uri") {
		t.Fatalf("expected invalid_uri for oversized URI, got: %#v", err)
	}

	// 4. readResource with nil request
	_, err = adapter.readResource(context.Background(), nil)
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "invalid_uri") {
		t.Fatalf("expected invalid_uri for nil ReadResourceRequest, got: %#v", err)
	}

	// 5. readResource with empty URI
	_, err = adapter.readResource(context.Background(), &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: ""}})
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "invalid_uri") {
		t.Fatalf("expected invalid_uri for empty URI, got: %#v", err)
	}

	// 6. readResource with oversized URI
	_, err = adapter.readResource(context.Background(), &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: strings.Repeat("y", 4097)}})
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams || !strings.Contains(string(rpcErr.Data), "invalid_uri") {
		t.Fatalf("expected invalid_uri for oversized URI, got: %#v", err)
	}
}

func newMCPWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (app.WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	service := app.SkillService{}
	content := []byte("---\nname: review-skill\ndescription: Review changed code safely.\nlicense: Apache-2.0\n---\n\n# Review\n\nReview carefully.\n")
	created, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID: "review-skill", Collection: "core", Name: "Review Skill", Description: "Review changed code safely.", Content: content,
		Routing: skill.RoutingInput{Operations: []string{"review"}, Triggers: []string{"review changed code"}, NotFor: []string{"write prose"}, MinScope: "multi_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, created, created.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create = %#v, %v", result, err)
	}
	activated, err := service.PreviewActivate(context.Background(), root, "review-skill", false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, activated, activated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm activate = %#v, %v", result, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "core", "review-skill", "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "review-skill", "references", "checks.md"), []byte("# Checks\n\nRun tests.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return root
}
