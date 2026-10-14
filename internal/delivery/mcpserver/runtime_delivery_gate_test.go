package mcpserver

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// gateCase breaks one fixture skill so the runtime profile must refuse it.
type gateCase struct {
	name     string
	wantCode string
	break_   func(t *testing.T, root, id string)
}

func gateCases() []gateCase {
	setStatus := func(status string) func(*testing.T, string, string) {
		return func(t *testing.T, root, id string) {
			editFixtureMeta(t, root, id, func(doc map[string]any) { doc["status"] = status })
		}
	}
	return []gateCase{
		{"draft", "not_found", setStatus("draft")},
		{"deprecated", "not_found", setStatus("deprecated")},
		{"archived", "not_found", setStatus("archived")},
		{"active with an empty description", "not_servable", func(t *testing.T, root, id string) {
			path := filepath.Join(root, "skills", "default", id, "SKILL.md")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			blanked := regexp.MustCompile(`(?m)^description: .*$`).ReplaceAllString(string(data), `description: ""`)
			if blanked == string(data) {
				t.Fatal("fixture SKILL.md has no description to blank")
			}
			writeFixtureFile(t, path, []byte(blanked))
			rebuildFixtureCatalog(t, root)
		}},
	}
}

func skillGetOutcome(t *testing.T, session *mcp.ClientSession, id string) (*mcp.CallToolResult, toolOutcome[skillGetResult]) {
	t.Helper()
	response, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_get", Arguments: map[string]any{"skill_id": id}})
	if err != nil {
		t.Fatalf("skill_get(%s): %v", id, err)
	}
	var outcome toolOutcome[skillGetResult]
	decodeStructuredContent(t, response, &outcome)
	return response, outcome
}

func requireRefused(t *testing.T, response *mcp.CallToolResult, outcome toolOutcome[skillGetResult], wantCode, secret string) {
	t.Helper()
	if !response.IsError || outcome.Error == nil || outcome.Result != nil {
		t.Fatalf("skill_get must be refused with an error and no result: %#v", outcome)
	}
	if outcome.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q", outcome.Error.Code, wantCode)
	}
	if outcome.Error.SuggestedAction == "" {
		t.Fatal("a refusal needs a suggested action")
	}
	encoded := outcome.Error.Message + " " + outcome.Error.SuggestedAction
	if strings.Contains(encoded, secret) || strings.Contains(encoded, "local") {
		t.Fatalf("refusal leaks content or paths: %q", encoded)
	}
	for _, block := range response.Content {
		if text, ok := block.(*mcp.TextContent); ok && strings.Contains(text.Text, secret) {
			t.Fatalf("refusal text leaks content: %q", text.Text)
		}
	}
}

func TestRuntimeProfileRefusesSkillsItCannotResolve(t *testing.T) {
	t.Parallel()
	for _, test := range gateCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := newBundleHubFixture(t)
			const id = "test-audit"
			runtime := connectSkillsClient(t, root, ProfileRuntime)
			listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), runtime, "skills/list", &listSkillsParams{})
			if err != nil {
				t.Fatal(err)
			}
			var uri string
			for _, entry := range listed.Skills {
				if strings.Contains(entry.URI, "/"+id+"/") {
					uri = entry.URI
				}
			}
			if uri == "" {
				t.Fatalf("fixture skill %s is not listed before it is broken", id)
			}
			if response, outcome := skillGetOutcome(t, runtime, id); response.IsError || outcome.Result == nil || outcome.Result.Content == "" {
				t.Fatalf("an active servable skill must be delivered: %#v", outcome)
			}

			test.break_(t, root, id)
			runtime = connectSkillsClient(t, root, ProfileRuntime)

			response, outcome := skillGetOutcome(t, runtime, id)
			requireRefused(t, response, outcome, test.wantCode, "# "+id)

			relisted, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), runtime, "skills/list", &listSkillsParams{})
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range relisted.Skills {
				if strings.Contains(entry.URI, "/"+id+"/") {
					t.Fatalf("skills/list still offers %s: %s", id, entry.URI)
				}
			}
			if got, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), runtime, "skills/get", &getSkillParams{URI: uri}); err == nil {
				t.Fatalf("skills/get served %s: %#v", id, got)
			}
			if got, err := runtime.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri}); err == nil {
				t.Fatalf("resources/read served %s: %#v", id, got)
			}
		})
	}
}

func TestCombinedProfileStillReadsDrafts(t *testing.T) {
	t.Parallel()
	for _, profile := range []Profile{ProfileAll} {
		t.Run(string(profile), func(t *testing.T) {
			t.Parallel()
			root := newBundleHubFixture(t)
			editFixtureMeta(t, root, "test-audit", func(doc map[string]any) { doc["status"] = "draft" })
			session := connectSkillsClient(t, root, profile)
			response, outcome := skillGetOutcome(t, session, "test-audit")
			if response.IsError || outcome.Result == nil || outcome.Result.Content == "" || outcome.Result.LifecycleState != "draft" {
				t.Fatalf("%s profile must read a draft: %#v", profile, outcome)
			}
		})
	}
}

func TestRuntimeResolvableSkillIsGettable(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	session := connectSkillsClient(t, root, ProfileRuntime)
	resolved := callSkillResolve(t, session, map[string]any{
		"schema_version": "1",
		"request_id":     "REQ-runtime-gate",
		"task":           map[string]any{"description": "review changed code", "scope": "multi_step"},
		"operation":      "review",
	})
	if resolved.Resolution.Primary == nil {
		t.Fatalf("fixture skill must resolve: %#v", resolved.Resolution)
	}
	id := resolved.Resolution.Primary.ID
	if response, outcome := skillGetOutcome(t, session, id); response.IsError || outcome.Result == nil || outcome.Result.Content == "" {
		t.Fatalf("resolved skill %s is not gettable: %#v", id, outcome)
	}
}

func TestRuntimeGateAppliesOverHTTP(t *testing.T) {
	t.Parallel()
	root := newBundleHubFixture(t)
	editFixtureMeta(t, root, "test-audit", func(doc map[string]any) { doc["status"] = "draft" })
	hub := startHTTPTestHub(t, root)
	session := connectHTTPClient(t, hub.endpoint, hub.token)
	response, outcome := skillGetOutcome(t, session, "test-audit")
	requireRefused(t, response, outcome, "not_found", "# test-audit")
	if response, outcome := skillGetOutcome(t, session, bundleCheckSkill); response.IsError || outcome.Result == nil {
		t.Fatalf("an active skill must still be served over HTTP: %#v", outcome)
	}
}
