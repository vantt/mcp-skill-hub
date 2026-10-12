package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestHubStatusCarriesTheHubRelativeLessonsCommand(t *testing.T) {
	root := newMCPWorkspace(t)
	fixture, err := os.ReadFile(filepath.Join("..", "..", "migration", "testdata", "test-audit-distill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	file := "skills/core/review-skill/.meta/distill.yaml"
	path := filepath.Join(root, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	session := connectDistributionSession(t, root)
	status, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "hub_status", Arguments: map[string]any{}})
	if err != nil || status.IsError {
		t.Fatalf("hub_status = %#v, %v", status, err)
	}
	var outcome struct {
		Result struct {
			Actions []map[string]any `json:"actions"`
		} `json:"result"`
	}
	decodeStructuredContent(t, status, &outcome)
	for _, action := range outcome.Result.Actions {
		if action["kind"] != "review_lessons" {
			continue
		}
		if action["script"] != ".agents/skills/distill-lab/scripts/distill.mjs" || action["file"] != file || action["hub_relative"] != true ||
			action["command"] != "node .agents/skills/distill-lab/scripts/distill.mjs list "+file {
			t.Fatalf("review_lessons over MCP = %v", action)
		}
		return
	}
	t.Fatalf("no review_lessons action: %v", outcome.Result.Actions)
}
