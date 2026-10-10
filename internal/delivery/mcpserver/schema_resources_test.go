package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listProfileTools(t *testing.T, profile Profile) *mcp.ListToolsResult {
	t.Helper()
	_, server, err := New(newMCPWorkspace(t), nil, profile)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "schema-consumer", Version: "1"}, nil)
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
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if listed.NextCursor != "" {
		t.Fatalf("unexpected incomplete tools/list: cursor %q", listed.NextCursor)
	}
	return listed
}

func decodeToolSchema(t *testing.T, value any) *jsonschema.Schema {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "null" {
		t.Fatal("tool schema is absent")
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	return &schema
}

func TestToolSchemasResolveForEveryProfile(t *testing.T) {
	t.Parallel()
	for _, profile := range []Profile{ProfileAll, ProfileRuntime, ProfileCuration} {
		t.Run(string(profile), func(t *testing.T) {
			t.Parallel()
			listed := listProfileTools(t, profile)
			want := expectedToolAnnotations()
			for name := range want {
				runtime := name == "skill_resolve" || name == "skill_get" || name == "skill_feedback"
				if (profile == ProfileRuntime && !runtime) || (profile == ProfileCuration && runtime) {
					delete(want, name)
				}
			}
			for _, tool := range listed.Tools {
				if _, ok := want[tool.Name]; !ok {
					t.Errorf("unexpected or duplicate tool %q for profile %s", tool.Name, profile)
				}
				delete(want, tool.Name)
				for kind, value := range map[string]any{"input": tool.InputSchema, "output": tool.OutputSchema} {
					if _, err := decodeToolSchema(t, value).Resolve(nil); err != nil {
						t.Errorf("%s %s schema cannot resolve: %v", tool.Name, kind, err)
					}
				}
			}
			if len(want) != 0 {
				t.Errorf("missing tools for profile %s: %v", profile, want)
			}
		})
	}
}

func TestResolverOutputSchemaSurvivesShrinking(t *testing.T) {
	t.Parallel()
	listed := listProfileTools(t, ProfileAll)
	for _, tool := range listed.Tools {
		if tool.Name != "skill_resolve" && tool.Name != "routing_evaluate" {
			continue
		}
		t.Run(tool.Name, func(t *testing.T) {
			schema := decodeToolSchema(t, tool.OutputSchema)
			// A later metadata/profile pass must not invalidate embedded resources.
			shrinkOutputSchema(schema)
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatalf("shrunk advertised output schema cannot resolve: %v", err)
			}
			digest := "sha256:" + strings.Repeat("a", 64)
			setup := map[string]any{"state": "ready"}
			primary := map[string]any{
				"id": "review-skill", "version": digest,
				"uri":           "skill://skillhub/" + strings.Repeat("a", 64) + "/review-skill/SKILL.md",
				"applicability": "Review changes", "confidence": "high", "setup": setup,
			}
			resolution := map[string]any{
				"schema_version": "1", "resolution_id": "RES-1", "request_id": "REQ-1",
				"context_revision": 1, "status": "resolved", "catalog_snapshot": digest,
				"policy_revision": digest, "reason_codes": []any{},
				"valid_for": map[string]any{"scope_fingerprint": digest},
				"primary":   primary, "supporting": []any{},
			}
			result := map[string]any{"resolution": resolution}
			if tool.Name == "skill_resolve" {
				result["schema_major"] = "1"
				result["resource_version_pinning"] = true
			} else {
				result["mutation"] = false
			}
			output := map[string]any{"schema_version": "1", "result": result}
			if err := resolved.Validate(output); err != nil {
				t.Fatalf("valid resolver response rejected: %v", err)
			}
			setup["state"] = "not-a-setup-state"
			if err := resolved.Validate(output); err == nil {
				t.Fatal("invalid setup state accepted after shrinking")
			}
			delete(setup, "state")
			if err := resolved.Validate(output); err == nil {
				t.Fatal("setup without required state accepted after shrinking")
			}
			setup["state"] = "ready"
			delete(resolution, "supporting")
			if err := resolved.Validate(output); err == nil {
				t.Fatal("resolved response without required supporting accepted after shrinking")
			}
		})
	}
}
